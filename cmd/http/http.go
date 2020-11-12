package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	httpbundle "github.com/amadigan/openvpn-aws/internal/http"
	"golang.org/x/net/http2"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
)

type testHandler struct {
	root string
}

func (t *testHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	method := request.Method
	log.Printf("Request: %s %s", method, path)

	headers := writer.Header()

	headers.Set("server", "openvpn-aws")

	if method != "OPTIONS" && method != "GET" && method != "POST" {
		if method == "POST" {
			writer.WriteHeader(405)
		} else {
			writer.WriteHeader(501)
		}
		return
	}

	if method == "OPTIONS" && path == "*" {
		headers.Set("Allow", "OPTIONS, HEAD, GET")
		writer.WriteHeader(204)
		return
	}

	file, exist := httpbundle.UIBundle[request.URL.Path]

	if !exist {
		log.Printf("Not found: %s", request.URL.Path)
		writer.WriteHeader(404)
		return
	}

	if method == "OPTIONS" {
		headers.Set("Allow", "OPTIONS, HEAD, GET")
		writer.WriteHeader(204)
		return
	}

	for key, value := range file.Headers {
		headers.Set(key, value)
	}

	if file.Headers["content-type"] == "application/javascript" {
		headers.Set("cache-control", "immutable,max-age=31536000")
	}

	tag := request.Header.Get("if-none-match")

	if tag == file.Headers["etag"] {
		writer.WriteHeader(304)
		headers.Del("content-length")
		return
	}

	writer.WriteHeader(200)

	if method != "HEAD" {
		writer.Write(file.Content)
	}
}

func parsePEM(filename string) ([]byte, error) {
	raw, err := ioutil.ReadFile(filename)

	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(raw)

	return block.Bytes, nil
}

func main() {
	certBytes, err := parsePEM("../../web/.local/tls/ui.crt")

	if err != nil {
		panic(err)
	}

	cert, err := x509.ParseCertificate(certBytes)

	if err != nil {
		panic(err)
	}

	keyBytes, err := parsePEM("../../web/.local/tls/ui.key")

	if err != nil {
		panic(err)
	}

	key, err := x509.ParseECPrivateKey(keyBytes)

	if err != nil {
		panic(err)
	}

	h2Server := &http2.Server{
		MaxConcurrentStreams:         16,
		MaxUploadBufferPerConnection: 65535,
		MaxReadFrameSize:             16 * 1024,
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{
			tls.Certificate{
				Certificate: [][]byte{certBytes},
				Leaf:        cert,
				PrivateKey:  key,
			},
		},
		NextProtos:       []string{http2.NextProtoTLS},
		CipherSuites:     []uint16{tls.TLS_AES_256_GCM_SHA384},
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP384, tls.X25519, tls.CurveP256, tls.CurveP521},
	}

	tcp, err := net.Listen("tcp", ":8400")

	if err != nil {
		panic(err)
	}

	listener := tls.NewListener(tcp, config)

	path, err := filepath.Abs("../../web/dist/openvpn-aws")

	if err != nil {
		panic(err)
	}

	handler := &testHandler{root: path}

	for {
		conn, err := listener.Accept()

		if err != nil {
			panic(err)
		}

		tlsConn := conn.(*tls.Conn)

		err = tlsConn.Handshake()

		if err != nil {
			log.Print(err)
		} else {
			state := tlsConn.ConnectionState()

			if state.NegotiatedProtocol != http2.NextProtoTLS {
				log.Printf("Did not negotiate http2 connection")
				conn.Close()
			} else {
				go h2Server.ServeConn(conn, &http2.ServeConnOpts{
					Handler: handler,
				})
			}
		}
	}

}
