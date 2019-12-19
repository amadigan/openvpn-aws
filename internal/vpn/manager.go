package vpn

import (
	"errors"
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/ca"
	"github.com/amadigan/openvpn-aws/internal/config"
	"github.com/amadigan/openvpn-aws/internal/dns"
	"github.com/amadigan/openvpn-aws/internal/fw"
	"github.com/amadigan/openvpn-aws/internal/log"
	"io/ioutil"
	"net"
	"path/filepath"
	"sync"
	"time"
)

var logger = log.New("manager")

type VPNManager struct {
	Server          *OpenVPN
	Firewall        *fw.Firewall
	backend         config.ConfigurationBackend
	users           *userManager
	dnsproxy        *dns.DNSProxy
	lock            sync.RWMutex
	clients         map[uint64]*clientConnection
	userConnections map[string]*clientConnection
	tunnelIP        net.IP
	tunnelDevice    string
	timer           time.Timer
	timerChannel    time.Timer
}

type clientConnection struct {
	user     string
	clientId uint64
	address  *net.IPNet
	key      string
	conf     *config.UserConfig
}

func BootVPN(conf config.ConfigurationBackend, root string) (rv *VPNManager, err error) {
	metric := log.StartMetric()
	vpn := &VPNManager{
		clients:         make(map[uint64]*clientConnection),
		userConnections: make(map[string]*clientConnection),
		backend:         conf,
	}

	file, err := conf.FetchFile("vpn.conf")

	if file == nil {
		if err == nil {
			err = errors.New("Unable to load vpn configuration")
		}
		return nil, err
	}

	configFile, err := config.ParseConfig(file)
	file.Close()

	if err != nil {
		return nil, err
	}

	cert, key, err := vpn.fetchKeys(configFile.DomainName)

	if err != nil {
		return nil, err
	}

	vpn.users, err = initUserManager(conf, root)

	if err != nil {
		return nil, err
	}

	userConfigs, err := vpn.users.buildUserConfigs(configFile)

	if err != nil {
		return nil, err
	}

	network := configFile.Network

	if network == nil {
		network = &net.IPNet{IP: net.IPv4(169, 254, 120, 0), Mask: net.CIDRMask(24, 32)}
	}

	vpn.Server, err = StartOpenVPN(filepath.Join(root, "socket"), filepath.Join(root, "openvpn.conf"), cert, key, *network)

	if err != nil {
		return nil, err
	}

	for {
		event := <-vpn.Server.StateChannel
		if event == nil {
			return nil, errors.New("State channel unexpectedly closed")
		}
		if event.State == "CONNECTED" {
			iface, err := findInterfaceByAddress(event.IPv4)

			if err != nil {
				return nil, err
			}

			if iface != nil {
				vpn.tunnelIP = event.IPv4
				vpn.tunnelDevice = iface.Name
				break
			}
		}
	}

	logger.Infof("Tunnel: %s on %s", vpn.tunnelIP, vpn.tunnelDevice)

	vpn.Firewall, err = fw.InitFirewall(vpn.tunnelDevice)

	if err != nil {
		return nil, err
	}

	vpn.dnsproxy, err = dns.StartProxy(vpn.tunnelIP.String())

	if err != nil {
		return nil, err
	}

	logger.Debugf("Total users found: %d", len(userConfigs))

	for userName, userConf := range userConfigs {
		err = vpn.updateFirewall(userName, userConf.config)

		if err != nil {
			vpn.Shutdown()
			return nil, err
		}
	}

	go vpn.handleEvents()

	if configFile.Route53Zone != "" && configFile.DomainName != "" {
		err = conf.RegisterDNS(configFile.Route53Zone, configFile.DomainName, configFile.Route53Weighted)

		if err != nil {
			vpn.Shutdown()
			return nil, fmt.Errorf("Failed to register DNS: %w", err)
		}
	}

	metric.Stop()
	logger.Infof("Server startup in %s", metric)

	timerDuration := configFile.WatchTime
	if timerDuration == nil {
		duration, _ := time.ParseDuration("30s")
		timerDuration = &duration
	}

	time.AfterFunc(*timerDuration, vpn.updateConfig)

	return vpn, nil
}

func (m *VPNManager) fetchKeys(name string) (cert, key []byte, err error) {
	file, err := m.backend.FetchFile("server.crt")

	if err != nil {
		return nil, nil, err
	}

	if file == nil {
		logger.Info("Generating a new server certificate")

		bundle, err := ca.MakeServerCertificate(name)

		if err != nil {
			return nil, nil, fmt.Errorf("Failed to generate server certificate: %w", err)
		}

		err = m.backend.PutFile("server.crt", bundle.Certificate)

		if err != nil {
			return nil, nil, err
		}

		err = m.backend.PutFile("server.key", bundle.Key)

		if err != nil {
			return nil, nil, err
		}

		err = m.backend.PutFile("serverca.crt", bundle.CACertificate)

		if err != nil {
			return nil, nil, err
		}

		return bundle.Certificate, bundle.Key, nil
	}

	cert, err = ioutil.ReadAll(file)
	file.Close()
	if err != nil {
		return nil, nil, err
	}

	file, err = m.backend.FetchFile("server.key")

	if file == nil {
		if err == nil {
			err = errors.New("Unable to load server key")
		}
		return nil, nil, err
	}

	key, err = ioutil.ReadAll(file)
	file.Close()
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

func (m *VPNManager) updateConfig() {
	metric := log.StartMetric()
	users, timerDuration, err := m.users.update()

	if timerDuration == nil {
		duration, _ := time.ParseDuration("5m")
		timerDuration = &duration
	}

	if users != nil && err != nil {
		for user, info := range users {
			m.updateFirewall(user, info.config)

			m.lock.RLock()
			userConn := m.userConnections[user]
			killConn := false

			if userConn != nil {
				if !info.keys[userConn.key] {
					killConn = true
				} else if (userConn.conf.DNSSetting == config.OFF) != (info.config.DNSSetting == config.OFF) {
					killConn = true
				} else {
					if len(userConn.conf.Routes) != len(info.config.Routes) {
						killConn = true
					} else {
						for i, route := range userConn.conf.Routes {
							otherNet := info.config.Routes[i].Network
							if !otherNet.IP.Equal(route.Network.IP) {
								killConn = true
								break
							}

							mask, _ := route.Network.Mask.Size()
							otherMask, _ := otherNet.Mask.Size()

							if mask != otherMask {
								killConn = true
								break
							}
						}
					}
				}
			}

			m.lock.RUnlock()

			if killConn {
				m.DisconnectUser(user)
			}
		}
	}

	if err != nil {
		logger.Warnf("Warning: Failed to update configuration, %s", err)
	}

	time.AfterFunc(*timerDuration, m.updateConfig)
	metric.Stop()

	logger.Infof("Configuration updated in %s", metric)
}

func (m *VPNManager) updateFirewall(user string, conf *config.UserConfig) error {
	rules := make([]fw.FirewallRule, 0, len(conf.Routes))

	for _, route := range conf.Routes {
		if len(route.Ports) == 0 {
			rules = append(rules, fw.FirewallRule{Network: route.Network})
		} else {
			for _, port := range route.Ports {
				rules = append(rules, fw.FirewallRule{Network: route.Network, Port: port})
			}
		}
	}

	return m.Firewall.UpdateUser(user, rules)
}

func (m *VPNManager) handleEvents() {
	for {
		select {
		case event := <-m.Server.ClientChannel:
			if event == nil {
				return
			}
			m.processClientEvent(event)
			break
		}
	}

}

func findInterfaceByAddress(addr net.IP) (*net.Interface, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()

		if err == nil {
			for _, ifaceAddr := range addrs {
				ifaceIP, _, err := net.ParseCIDR(ifaceAddr.String())

				if err != nil {
					return nil, err
				}

				if ifaceIP.Equal(addr) {
					return &iface, nil
				}
			}
		}
	}

	return nil, nil
}

func (m *VPNManager) authorizeClient(clientId, keyId uint64, env map[string]string, reauth bool) error {
	userName := env["X509_1_CN"]
	keyHash := env["X509_1_OU"]

	if _, exists := env["tls_digest_sha256_3"]; exists {
		errString := fmt.Sprintf("Denying user %s with key hash %s, depth too high", userName, keyHash)
		logger.Warn(errString)
		return m.Server.ExecCommand(fmt.Sprintf("client-deny %d %d \"%s\"", clientId, keyId, errString), true)
	}

	conf, keyAlias, err := m.users.authenticateUser(userName, keyHash)

	if err != nil {
		logger.Errorf("Authentication error %s", err)
		return m.Server.ExecCommand(fmt.Sprintf("client-deny %d %d \"%s\"", clientId, keyId, err), true)
	}

	command := fmt.Sprintf("client-auth %d %d\n", clientId, keyId)

	if conf.DNSSetting != config.OFF {
		command += fmt.Sprintf("push \"dhcp-option DNS %s\"\n", m.tunnelIP)
	}

	rootMask := net.IPv4(255, 255, 255, 255)

	for _, route := range conf.Routes {
		command += fmt.Sprintf("push \"route %s %s\"\n", route.Network.IP, rootMask.Mask(route.Network.Mask))
	}

	command += "END"

	m.lock.Lock()
	var oldConnection *clientConnection

	if !reauth {
		oldConnection = m.clients[clientId]

		if oldConnection != nil {
			delete(m.clients, clientId)
			oldUserConnection := m.userConnections[oldConnection.user]
			if oldUserConnection != nil && oldUserConnection.clientId == clientId {
				delete(m.userConnections, oldConnection.user)
			}
		}

		oldConnection = m.userConnections[userName]
	}

	connection := &clientConnection{user: userName, clientId: clientId, key: keyAlias, conf: conf}

	m.clients[clientId] = connection
	m.userConnections[userName] = connection

	m.lock.Unlock()

	if oldConnection != nil {
		logger.Infof("Killing old connection %d for user %s", oldConnection.clientId, userName)
		m.Server.ExecCommand(fmt.Sprintf("client-kill %d", oldConnection.clientId), false)
	}

	if log.LogLevel <= log.DEBUG {
		logger.Debugf("Authorizing client %d for user %s with command %s", clientId, userName, command)
	} else {
		logger.Infof("Authorizing client %d for user %s", clientId, userName)
	}

	return m.Server.ExecCommand(command, true)
}

func (m *VPNManager) DisconnectUser(user string) error {
	m.lock.Lock()
	connection := m.userConnections[user]

	if connection != nil {
		delete(m.userConnections, user)
		delete(m.clients, connection.clientId)
	}

	m.lock.Unlock()

	if connection != nil {
		return m.Server.ExecCommand(fmt.Sprintf("client-kill %d", connection.clientId), false)
	}

	return nil
}

func (m *VPNManager) processClientEvent(e *VPNClientEvent) {
	logger.Debugf("%s event for client %d", e.Type, e.ClientId)

	if e.Environment != nil {
		logger.Debug(e.Environment)
	}

	if e.Type == "CONNECT" || e.Type == "REAUTH" {
		m.authorizeClient(e.ClientId, e.KeyId, e.Environment, e.Type == "REAUTH")
	} else if e.Type == "ADDRESS" {
		m.lock.Lock()
		conn := m.clients[e.ClientId]
		conn.address = &e.Address
		m.lock.Unlock()
		m.Firewall.ConnectUser(conn.user, e.Address)
	} else if e.Type == "DISCONNECT" {
		m.lock.Lock()
		conn := m.clients[e.ClientId]

		if conn != nil {
			delete(m.clients, e.ClientId)

			userConn := m.userConnections[conn.user]

			if userConn.clientId == e.ClientId {
				delete(m.userConnections, conn.user)
			}
		}

		m.lock.Unlock()

		if conn != nil && conn.address != nil {
			m.Firewall.DisconnectUser(conn.user, *conn.address)
		}

	}
}

func (m *VPNManager) Shutdown() {
	logger.Infof("Initiating server shutdown")
	err := m.backend.UnregisterDNS()

	if err != nil {
		logger.Warnf("Failed to unregister DNS: %w", err)
	}

	m.Server.Shutdown()
	m.dnsproxy.Stop()
}
