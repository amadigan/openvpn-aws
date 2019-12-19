package config

import (
	"io"
	"net"
)

type NetworkInfo struct {
	Subnets map[string]net.IPNet
	NAT     []net.IPNet
}

type ConfigurationBackend interface {
	FetchFile(path string, ifNotTag string) (reader io.ReadCloser, tag string, err error)
	PutFile(path string, data []byte) error
	FetchNetworkInfo() (*NetworkInfo, error)
	FetchGroup(name string) ([]string, error)
	FetchGroupsForUser(user string) ([]string, error)
	FetchKeys(user string) ([]string, error)
	FetchKey(user, key string) ([]byte, error)
	RegisterDNS(zone, name string, weighted bool) error
	UnregisterDNS() error
}
