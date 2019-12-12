package ca

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/log"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var logger = log.New("ca")

type CertificateFile struct {
	hash  string
	index int
}

type CertificateManager struct {
	Path        string
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	counter     int64
	byHash      map[string][]*CertificateFile
	byName      map[string]*CertificateFile
	lock        sync.Mutex
}

type ServerCertificate struct {
	CACertificate []byte
	Certificate   []byte
	Key           []byte
}

type basicConstraints struct {
	IsCA       bool `asn1:"optional"`
	MaxPathLen int  `asn1:"optional,default:-1"`
}

func CreateCertificateManager(path string) (*CertificateManager, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)

	if err != nil {
		return nil, err
	}

	basicCon := basicConstraints{IsCA: true, MaxPathLen: -1}
	basicConBits, err := asn1.Marshal(basicCon)

	if err != nil {
		return nil, err
	}

	tempCATemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "fakeca",
		},
		NotBefore:          time.Now(),
		NotAfter:           time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
		SignatureAlgorithm: x509.ECDSAWithSHA384,
		IsCA:               true,
		ExtraExtensions: []pkix.Extension{
			pkix.Extension{
				Id:       oidExtensionBasicConstraints,
				Critical: true,
				Value:    basicConBits,
			},
		},
	}

	caCertDer, err := x509.CreateCertificate(rand.Reader, &tempCATemplate, &tempCATemplate, &caKey.PublicKey, caKey)

	if err != nil {
		return nil, err
	}

	caCert, err := x509.ParseCertificate(caCertDer)

	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(path, 0755)

	if err != nil {
		return nil, err
	}

	cm := new(CertificateManager)
	cm.Path = path
	cm.certificate = caCert
	cm.key = caKey
	cm.byHash = make(map[string][]*CertificateFile)
	cm.byName = make(map[string]*CertificateFile)

	err = cm.addCertificate(caCert, caCertDer, nil)

	if err != nil {
		return nil, err
	}

	return cm, nil
}

func ParsePublicKey(content []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(content)

	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("Input is not a PEM public key")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)

	if err != nil {
		return nil, err
	}

	rv := key.(*rsa.PublicKey)

	if rv == nil {
		return nil, errors.New("Unable to convert key to RSA public key")
	}

	return rv, nil
}

var (
	oidCountry                   = []int{2, 5, 4, 6}
	oidOrganization              = []int{2, 5, 4, 10}
	oidOrganizationalUnit        = []int{2, 5, 4, 11}
	oidCommonName                = []int{2, 5, 4, 3}
	oidSerialNumber              = []int{2, 5, 4, 5}
	oidLocality                  = []int{2, 5, 4, 7}
	oidProvince                  = []int{2, 5, 4, 8}
	oidStreetAddress             = []int{2, 5, 4, 9}
	oidPostalCode                = []int{2, 5, 4, 17}
	oidExtensionBasicConstraints = []int{2, 5, 29, 19}
)

func (m *CertificateManager) Add(user string, alias string, key *rsa.PublicKey) (hash string, err error) {
	logger.Infof("Adding key %s for user %s", alias, user)
	spki, err := x509.MarshalPKIXPublicKey(key)

	if err != nil {
		return "", err
	}

	hashArr := sha256.Sum256(spki)
	hash = fmt.Sprintf("%x", hashArr)

	basicCon := basicConstraints{IsCA: true, MaxPathLen: -1}
	basicConBits, err := asn1.Marshal(basicCon)

	if err != nil {
		return hash, err
	}

	clientTemplate := x509.Certificate{
		SerialNumber: big.NewInt(atomic.AddInt64(&m.counter, 1)),
		Subject: pkix.Name{
			ExtraNames: []pkix.AttributeTypeAndValue{
				pkix.AttributeTypeAndValue{Type: oidCommonName, Value: user},
				pkix.AttributeTypeAndValue{Type: oidOrganizationalUnit, Value: hash},
			},
		},
		NotBefore:          time.Now(),
		NotAfter:           time.Date(2200, time.December, 31, 23, 59, 59, 0, time.UTC),
		SignatureAlgorithm: x509.ECDSAWithSHA384,
		IsCA:               true,
		ExtraExtensions: []pkix.Extension{
			pkix.Extension{
				Id:       oidExtensionBasicConstraints,
				Critical: true,
				Value:    basicConBits,
			},
		},
	}

	certDer, err := x509.CreateCertificate(rand.Reader, &clientTemplate, m.certificate, key, m.key)

	if err != nil {
		return hash, err
	}

	err = m.addCertificate(&clientTemplate, certDer, &alias)

	if err != nil {
		return hash, err
	}

	return hash, nil
}

func (m *CertificateManager) addCertificate(cert *x509.Certificate, der []byte, alias *string) error {
	nameBytes, err := getNameBytes(cert.Subject)

	if err != nil {
		return err
	}

	certHash := certificateHash(nameBytes)

	m.lock.Lock()
	defer m.lock.Unlock()
	certs := m.byHash[certHash]

	certFile := new(CertificateFile)

	certFile.hash = certHash
	certFile.index = len(certs)

	m.byHash[certHash] = append(certs, certFile)

	if alias != nil {
		m.byName[*alias] = certFile
	}

	return writePEM(filepath.Join(m.Path, certFile.hash+"."+strconv.Itoa(certFile.index)), "CERTIFICATE", der)
}

func (m *CertificateManager) Remove(alias string) error {
	m.lock.Lock()

	certFile := m.byName[alias]
	certs := m.byHash[certFile.hash]

	var err error

	last := len(certs) - 1

	if certFile.index != last {
		err = os.Rename(filepath.Join(m.Path, certFile.hash, strconv.Itoa(last)), filepath.Join(m.Path, certFile.hash, strconv.Itoa(certFile.index)))
		certs[certFile.index] = certs[last]
		certs[certFile.index].index = certFile.index
	} else {
		err = os.Remove(filepath.Join(m.Path, certFile.hash, strconv.Itoa(certFile.index)))
	}

	m.byHash[certFile.hash] = certs[:len(certs)-1]

	m.lock.Unlock()

	return err
}

func writePEM(filename, pemType string, bs []byte) error {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)

	if err != nil {
		return err
	}

	err = pem.Encode(f, &pem.Block{Type: pemType, Bytes: bs})

	if err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

func MakeServerCertificate(vpnName string) (*ServerCertificate, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)

	if err != nil {
		return nil, err
	}

	basicCon := basicConstraints{IsCA: true, MaxPathLen: -1}
	basicConBits, err := asn1.Marshal(basicCon)

	if err != nil {
		return nil, err
	}

	tempCATemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "CA for " + vpnName,
		},
		NotBefore:          time.Now(),
		NotAfter:           time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
		SignatureAlgorithm: x509.ECDSAWithSHA384,
		IsCA:               true,
		ExtraExtensions: []pkix.Extension{
			pkix.Extension{
				Id:       oidExtensionBasicConstraints,
				Critical: true,
				Value:    basicConBits,
			},
		},
	}

	caCertDer, err := x509.CreateCertificate(rand.Reader, &tempCATemplate, &tempCATemplate, &caKey.PublicKey, caKey)

	if err != nil {
		return nil, err
	}

	caCert, err := x509.ParseCertificate(caCertDer)

	if err != nil {
		return nil, err
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)

	if err != nil {
		return nil, err
	}

	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: vpnName,
		},
		NotBefore:          time.Now(),
		NotAfter:           time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
		SignatureAlgorithm: x509.ECDSAWithSHA384,
		KeyUsage:           x509.KeyUsageDigitalSignature | x509.KeyUsageKeyAgreement,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverCertDer, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverKey.PublicKey, caKey)

	if err != nil {
		return nil, err
	}

	serverKeyDer, err := x509.MarshalPKCS8PrivateKey(serverKey)

	if err != nil {
		return nil, err
	}

	ret := new(ServerCertificate)

	ret.CACertificate = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDer})
	ret.Key = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDer})
	ret.Certificate = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDer})

	return ret, nil
}

func getNameBytes(name pkix.Name) ([]byte, error) {
	type attributeTypeAndValue struct {
		Type  asn1.ObjectIdentifier
		Value interface{} `asn1:"utf8"`
	}

	type RDNSET []attributeTypeAndValue

	seq := name.ToRDNSequence()

	var bs []byte

	for i := 0; i < len(seq); i++ {
		entry := RDNSET{attributeTypeAndValue{Type: seq[i][0].Type, Value: strings.ToLower(seq[i][0].Value.(string))}}
		bytes, err := asn1.Marshal(entry)

		if err != nil {
			return nil, err
		}

		bs = append(bs, bytes...)
	}

	return bs, nil
}

func certificateHash(nameBytes []byte) string {
	hashBytes := sha1.Sum(nameBytes)

	return fmt.Sprintf("%08x", binary.LittleEndian.Uint32(hashBytes[:4]))
}

func CheckCertificate(capath, certpath string) (bool, error) {
	content, err := ioutil.ReadFile(certpath)

	if err != nil {
		return false, err
	}

	block, _ := pem.Decode(content)

	if block == nil || block.Type != "CERTIFICATE" {
		return false, errors.New("Input is not a PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)

	if err != nil {
		return false, err
	}

	issuer, err := getNameBytes(pkix.Name{
		ExtraNames: []pkix.AttributeTypeAndValue{
			pkix.AttributeTypeAndValue{Type: oidCommonName, Value: cert.Issuer.CommonName},
			pkix.AttributeTypeAndValue{Type: oidOrganizationalUnit, Value: cert.Issuer.OrganizationalUnit[0]},
		},
	})

	if err != nil {
		return false, err
	}

	hash := certificateHash(issuer)

	for i := 0; true; i++ {
		content, err = ioutil.ReadFile(filepath.Join(capath, fmt.Sprintf("%s.%d", hash, i)))

		if err != nil {
			return false, err
		}

		block, _ = pem.Decode(content)

		if block == nil || block.Type != "CERTIFICATE" {
			return false, errors.New("Input is not a PEM certificate")
		}

		caCert, err := x509.ParseCertificate(block.Bytes)

		if err != nil {
			return false, err
		}

		if cert.Subject.CommonName == caCert.Subject.CommonName &&
			cert.Issuer.CommonName == caCert.Subject.CommonName &&
			cert.Issuer.OrganizationalUnit[0] == caCert.Subject.OrganizationalUnit[0] {
			return bytes.Equal(cert.RawSubjectPublicKeyInfo, caCert.RawSubjectPublicKeyInfo), nil
		}
	}

	return false, nil
}
