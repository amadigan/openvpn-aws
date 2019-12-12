package config

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/log"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
)

var logger = log.New("config")

type LocalConfig struct {
	Root string
}

func (c *LocalConfig) FetchFile(path string) (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(c.Root, path))

	if err != nil {
		logger.Errorf("Error retrieving %s: %s", path, err)

		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		} else {
			return nil, err
		}
	}

	return file, nil
}

func (c *LocalConfig) PutFile(path string, data []byte) error {
	path = filepath.Join(c.Root, path)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return fmt.Errorf("Unable to write to file %s: %w", path, err)
	}

	defer file.Close()

	_, err = file.Write(data)

	return err
}

func (c *LocalConfig) FetchNetworkInfo() (*NetworkInfo, error) {
	file, err := c.FetchFile("netinfo")

	if file == nil {
		return nil, err
	}

	defer file.Close()
	reader := bufio.NewReader(file)

	info := &NetworkInfo{
		Subnets: make(map[string]net.IPNet),
	}

	for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		fields := strings.Fields(line)

		if len(fields) != 2 {
			continue
		}

		var network *net.IPNet

		netStr := fields[1]

		if strings.IndexRune(netStr, '/') < 0 {
			network := new(net.IPNet)

			network.IP = net.ParseIP(netStr)
			bits := len(network.IP) * 8
			network.Mask = net.CIDRMask(bits, bits)
		} else {
			_, network, err = net.ParseCIDR(netStr)

			if err != nil {
				return info, err
			}
		}

		if fields[0] == "nat" {
			info.NAT = append(info.NAT, *network)
		} else {
			info.Subnets[fields[0]] = *network
		}
	}

	if errors.Is(err, io.EOF) {
		return info, nil
	}

	return info, err
}

func (c *LocalConfig) FetchGroup(name string) ([]string, error) {
	file, err := c.FetchFile("groups")

	if file == nil {
		logger.Errorf("Failed to open groups file: %s", err)
		return nil, err
	}

	defer file.Close()
	reader := bufio.NewReader(file)

	for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		fields := strings.Fields(line)
		if fields[0] == name {
			return fields[1:], nil
		}
	}

	if errors.Is(err, io.EOF) {
		return nil, nil
	}

	return nil, err
}

func (c *LocalConfig) FetchGroupsForUser(user string) ([]string, error) {
	file, err := c.FetchFile("groups")

	if file == nil {
		return nil, err
	}

	defer file.Close()
	reader := bufio.NewReader(file)

	var groups []string

	for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		fields := strings.Fields(line)

		for _, name := range fields[1:] {
			if name == user {
				groups = append(groups, fields[0])
				break
			}
		}
	}

	if errors.Is(err, io.EOF) {
		return groups, nil
	}

	return groups, err
}

func (c *LocalConfig) FetchKeys(user string) ([]string, error) {
	files, err := ioutil.ReadDir(filepath.Join(c.Root, "user", user))

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	keys := make([]string, 0, len(files))

	for _, file := range files {
		if file.Mode().IsRegular() {
			keys = append(keys, file.Name())
		}
	}

	return keys, nil
}

func (c *LocalConfig) FetchKey(user string, key string) ([]byte, error) {
	bs, err := ioutil.ReadFile(filepath.Join(c.Root, "user", user, key))

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return bs, nil
}

func (c *LocalConfig) RegisterDNS(zone, name string, weighted bool) error {
	return nil
}

func (c *LocalConfig) UnregisterDNS() error {
	return nil
}
