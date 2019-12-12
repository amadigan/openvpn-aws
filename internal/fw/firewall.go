package fw

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/log"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var logger = log.New("firewall")

type Firewall struct {
	vpnInterface string
	wanInterface string
	userChains   map[string][]chainRule
	chainlock    sync.RWMutex
	connections  map[userAddress]string
	connlock     sync.Mutex
}

type FirewallRule struct {
	Network net.IPNet
	Port    uint16
}

type userAddress struct {
	ip   [16]byte
	size int // Size of the IP in bytes, 4 or 16
	mask int // Bits of the mask
}

type chainRule struct {
	ip   [16]byte
	size int    // Size of the IP in bytes, 4 or 16
	mask int    // Bits of the mask
	port uint16 // 0 for all
}

func InitFirewall(vpnInterface string) (*Firewall, error) {
	err := iptables("--policy", "FORWARD", "DROP")

	if err != nil {
		return nil, err
	}

	bs, err := ioutil.ReadFile("/proc/sys/net/ipv4/ip_forward")

	if err != nil {
		return nil, err
	}

	if len(bs) == 0 || bs[0] != '1' {
		err = ioutil.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644)

		if err != nil {
			return nil, err
		}
	}

	wanInterface, err := getDefaultRoute()

	if err != nil {
		return nil, err
	}

	err = iptables("--table", "nat", "--append", "POSTROUTING", "--out-interface", *wanInterface, "--jump", "MASQUERADE")

	if err != nil {
		return nil, err
	}

	err = iptables("--append", "FORWARD", "--match", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "--jump", "ACCEPT")

	if err != nil {
		return nil, err
	}

	fw := new(Firewall)
	fw.vpnInterface = vpnInterface
	fw.wanInterface = *wanInterface
	fw.userChains = make(map[string][]chainRule)
	fw.connections = make(map[userAddress]string)

	return fw, nil
}

func to16(ip net.IP) (ip16 [16]byte) {
	copy(ip16[:], ip.To16())
	return ip16
}

func (fw *Firewall) ConnectUser(user string, ip net.IPNet) error {
	addr := userAddress{ip: to16(ip.IP)}

	addr.mask, addr.size = ip.Mask.Size()

	fw.connlock.Lock()
	defer fw.connlock.Unlock()

	existingUser, exists := fw.connections[addr]

	if exists {
		err := iptables("--delete", "FORWARD", "--in-interface", fw.vpnInterface, "--source", ip.String(), "--jump", "user-"+existingUser)

		if err != nil {
			return err
		}
	}

	fw.connections[addr] = user

	return iptables("--append", "FORWARD", "--in-interface", fw.vpnInterface, "--source", ip.String(), "--jump", "user-"+user)
}

func (fw *Firewall) DisconnectUser(user string, ip net.IPNet) error {
	addr := userAddress{ip: to16(ip.IP)}

	addr.mask, addr.size = ip.Mask.Size()

	fw.connlock.Lock()
	defer fw.connlock.Unlock()

	existingUser, _ := fw.connections[addr]

	if existingUser == user {
		delete(fw.connections, addr)
		return iptables("--delete", "FORWARD", "--in-interface", fw.vpnInterface, "--source", ip.String(), "--jump", "user-"+user)
	}

	return nil
}

func (fw *Firewall) UpdateUser(user string, rules []FirewallRule) error {
	chainRules := make(map[chainRule]bool)

	for _, rule := range rules {
		entry := chainRule{ip: to16(rule.Network.IP)}
		entry.size, entry.mask = rule.Network.Mask.Size()
		entry.port = rule.Port
		chainRules[entry] = true
	}

	fw.chainlock.RLock()
	current := fw.userChains[user]

	update := (current == nil && len(rules) != 0) || len(current) != len(rules)

	if !update {
		for _, entry := range current {
			if !chainRules[entry] {
				update = true
				break
			}
		}
	}

	fw.chainlock.RUnlock()

	if !update {
		return nil
	}

	if current == nil {
		logger.Infof("Adding user %s", user)
	} else if len(rules) == 0 {
		logger.Infof("Deleting user %s", user)
	} else {
		logger.Infof("Updating rules for user %s", user)
	}

	fw.chainlock.Lock()
	defer fw.chainlock.Unlock()

	current = fw.userChains[user]

	chain := "user-" + user

	if rules == nil {
		if current != nil {
			delete(fw.userChains, user)
			return fw.dropChain(chain)
		} else {
			return nil
		}
	} else if current == nil {
		err := fw.createChain(chain)

		if err != nil {
			return err
		}

		current = make([]chainRule, len(chainRules), len(chainRules))
		var index int

		for entry, _ := range chainRules {
			current[index] = entry
			index++
			err = fw.addRule(chain, entry)

			if err != nil {
				return err
			}
		}

		fw.userChains[user] = current
	} else {
		toDelete := make([]int, 0, len(current))

		for index, entry := range current {
			if chainRules[entry] {
				delete(chainRules, entry)
			} else {
				toDelete = append(toDelete, index)
			}
		}

		var deletePointer int

		for entry, _ := range chainRules {
			if deletePointer < len(toDelete) {
				rulenum := toDelete[deletePointer]
				deletePointer++
				fw.replaceRule(chain, rulenum, entry)
				current[rulenum] = entry
			} else {
				fw.addRule(chain, entry)
				current = append(current, entry)
			}
		}

		for i := len(toDelete) - 1; i >= deletePointer; i-- {
			rulenum := toDelete[deletePointer]
			fw.deleteRule(chain, rulenum)
			copy(current[rulenum:], current[rulenum+1:])
		}

		fw.userChains[user] = current
	}

	return nil
}

func (fw *Firewall) createChain(chain string) error {
	return iptables("--new-chain", chain)
}

func (fw *Firewall) dropChain(chain string) error {
	err := iptables("--flush", chain)

	if err != nil {
		return err
	}

	return iptables("--delete-chain", chain)
}

func (fw *Firewall) addRule(chain string, rule chainRule) error {
	network := net.IPNet{IP: rule.ip[:], Mask: net.CIDRMask(rule.size, rule.mask)}

	if rule.port == 0 {
		return iptables(
			"--append", chain,
			"--destination", network.String(),
			"--in-interface", fw.vpnInterface,
			"--match", "conntrack",
			"--ctstate", "NEW",
			"--jump", "ACCEPT")
	} else {
		return iptables(
			"--append", chain,
			"--destination", network.String(),
			"--in-interface", fw.vpnInterface,
			"--protocol", "tcp",
			"--match", "tcp",
			"--dport", strconv.FormatUint(uint64(rule.port), 10),
			"--match", "conntrack", "--ctstate", "NEW",
			"--jump", "ACCEPT")
	}
}

func (fw *Firewall) deleteRule(chain string, rule int) error {
	return iptables("--delete", chain, strconv.Itoa(rule))
}

func (fw *Firewall) replaceRule(chain string, index int, rule chainRule) error {
	network := net.IPNet{IP: rule.ip[:], Mask: net.CIDRMask(rule.size, rule.mask)}

	if rule.port == 0 {
		return iptables(
			"--replace", chain, strconv.Itoa(index),
			"--destination", network.String(),
			"--in-interface", fw.vpnInterface,
			"--match", "conntrack",
			"--ctstate", "NEW",
			"--jump", "ACCEPT")
	} else {
		return iptables(
			"--append", chain, strconv.Itoa(index),
			"--destination", network.String(),
			"--in-interface", fw.vpnInterface,
			"--protocol", "tcp",
			"--match", "tcp",
			"--dport", strconv.FormatUint(uint64(rule.port), 10),
			"--match", "conntrack", "--ctstate", "NEW",
			"--jump", "ACCEPT")
	}
}

func iptables(a ...string) (err error) {
	logger.Debugf("Running %s", strings.Join(a, " "))
	out, err := exec.Command("iptables", a...).CombinedOutput()

	if err != nil {
		return fmt.Errorf("Failed to execute %s: %s (%w)", strings.Join(a, " "), out, err)
	}

	return nil
}

func getDefaultRoute() (*string, error) {
	routeTable, err := os.Open("/proc/net/route")

	if err != nil {
		return nil, err
	}

	defer routeTable.Close()

	reader := bufio.NewReader(routeTable)

	_, err = reader.ReadBytes('\n')

	if err != nil {
		return nil, err
	}

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			return nil, err
		}

		fields := strings.Fields(line)

		if fields[1] == "00000000" {
			return &fields[0], nil
		}
	}

	return nil, errors.New("Unable to find default route")
}
