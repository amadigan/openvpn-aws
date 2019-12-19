package config

import (
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SectionType byte
type ConfigFlag byte

const (
	GLOBAL = iota
	GROUP  = iota
	USER   = iota
)

const (
	NOT_SET = 0
	OFF     = 1
	ON      = 2
	ALL     = 3
)

type Route struct {
	Network net.IPNet
	Ports   []uint16
}

type Subnet struct {
	Name  string
	Ports []uint16
}

type SectionConfig struct {
	Type       SectionType
	Name       string
	Order      int
	Subnets    []Subnet
	Routes     []Route
	NATRoutes  []Route
	NATSetting ConfigFlag
	DNSSetting ConfigFlag
}

type ConfigFile struct {
	WatchTime       *time.Duration
	Network         *net.IPNet
	Route53Zone     string
	DomainName      string
	Route53Weighted bool
	KeyStrength     int
	GlobalConfig    *SectionConfig
	Groups          map[string]*SectionConfig
	Users           map[string]*SectionConfig
}

type UserRoute struct {
	Network net.IPNet
	Ports   []uint16
}

type UserConfig struct {
	DNSSetting ConfigFlag
	Routes     []UserRoute
}

type networkKey struct {
	ip   [16]byte
	size int // Size of the IP in bytes, 4 or 16
	mask int // Bits of the mask
}

func (config *ConfigFile) GetUserConfig(user string, groups []string, netinfo *NetworkInfo) (*UserConfig, error) {
	var dns ConfigFlag
	allSubnets := true
	sections := make([]*SectionConfig, 1, len(groups)+2)

	sections[0] = config.GlobalConfig

	for _, group := range groups {
		groupConfig, exists := config.Groups[group]

		if exists {
			sections = append(sections, groupConfig)
		}
	}

	userConfig, exists := config.Users[user]

	if !exists && len(sections) == 1 {
		return nil, fmt.Errorf("User %s does not have access", user)
	}

	sort.Slice(sections, func(i int, j int) bool {
		if sections[i].Type != sections[j].Type {
			return sections[i].Type < sections[j].Type
		} else {
			return sections[i].Order < sections[j].Order
		}
	})

	if exists {
		sections = append(sections, userConfig)
	}

	routes := make(map[networkKey]map[uint16]bool)
	var natRoutes []Route

	var nat ConfigFlag
	nat = ALL

	for _, section := range sections {
		if section.DNSSetting != NOT_SET {
			dns = section.DNSSetting
		}

		if section.NATSetting != NOT_SET {
			nat = section.NATSetting
		}

		natRoutes = append(natRoutes, section.NATRoutes...)

		for _, route := range section.Routes {
			addRoute(route.Network, route.Ports, routes)
			key := toNetworkKey(route.Network)
			ports := routes[key]
			if ports == nil {
				ports = make(map[uint16]bool, len(route.Ports))
				routes[key] = ports
			}

			for _, port := range route.Ports {
				ports[port] = true
			}
		}

		if len(section.Subnets) != 0 {
			allSubnets = false
		}

		for _, subnet := range section.Subnets {
			network, exists := netinfo.Subnets[subnet.Name]

			if exists {
				addRoute(network, subnet.Ports, routes)
			}
		}
	}

	if nat != OFF {
		if nat == ALL {
			for _, network := range netinfo.NAT {
				addRoute(network, nil, routes)
			}
		}

		for _, route := range natRoutes {
			addRoute(route.Network, route.Ports, routes)
		}
	}

	if allSubnets {
		for _, subnet := range netinfo.Subnets {
			addRoute(subnet, nil, routes)
		}
	}

	result := &UserConfig{
		DNSSetting: dns,
		Routes:     make([]UserRoute, 0, len(routes)),
	}

	for key, ports := range routes {
		routePorts := make([]uint16, 0, len(ports))

		for port, _ := range ports {
			routePorts = append(routePorts, port)
		}

		result.Routes = append(result.Routes, UserRoute{
			Network: toIPNet(key),
			Ports:   routePorts,
		})
	}

	sort.Slice(result.Routes, func(i int, j int) bool { return result.Routes[i].Network.String() < result.Routes[j].Network.String() })

	return result, nil
}

func (config *ConfigFile) String() string {
	rv := config.GlobalConfig.String()

	if config.WatchTime != nil && config.WatchTime.Seconds() > 0 {
		rv += fmt.Sprintf("\twatch %ds\n", int(config.WatchTime.Seconds()))
	}

	if config.DomainName != "" {
		mode := "simple"
		if config.Route53Weighted {
			mode = "weighted"
		}
		rv += fmt.Sprintf("\troute53 %s %s %s\n", config.Route53Zone, config.DomainName, mode)
	}

	if config.Network != nil {
		rv += fmt.Sprintf("\tnet %s\n", config.Network.String())
	}

	if config.KeyStrength != 0 {
		rv += fmt.Sprintf("\tkey-strength %d\n", config.KeyStrength)
	}

	for _, section := range config.Groups {
		rv += section.String()
	}

	for _, section := range config.Users {
		rv += section.String()
	}

	return rv
}

func (section *SectionConfig) String() string {
	var rv string
	switch section.Type {
	case GLOBAL:
		rv = "global\n"
		break
	case GROUP:
		rv = fmt.Sprintf("group %s\n", section.Name)
		break
	case USER:
		rv = fmt.Sprintf("user %s\n", section.Name)
		break
	}

	if section.DNSSetting != NOT_SET {
		rv += fmt.Sprintf("\tdns %s\n", section.DNSSetting.String())
	}

	if section.NATSetting == ON || section.NATSetting == OFF {
		rv += fmt.Sprintf("\tnat %s\n", section.NATSetting.String())
	}

	for _, subnet := range section.Subnets {
		rv += fmt.Sprintf("\t%s\n", subnet.String())
	}

	for _, nat := range section.NATRoutes {
		rv += fmt.Sprintf("\tnat %s\n", nat.String())
	}

	for _, route := range section.Routes {
		rv += fmt.Sprintf("\troute %s\n", route.String())
	}

	return rv
}

func (subnet Subnet) String() string {
	if len(subnet.Ports) == 0 {
		return fmt.Sprintf("%s", subnet.Name)
	} else {
		ports := ""

		for _, port := range subnet.Ports {
			if ports != "" {
				ports += " "
			}

			ports += strconv.Itoa(int(port))
		}

		return fmt.Sprintf("%s %s", subnet.Name, ports)
	}
}

func (route Route) String() string {
	if len(route.Ports) == 0 {
		return fmt.Sprintf("%s", route.Network.String())
	} else {
		ports := ""

		for _, port := range route.Ports {
			if ports != "" {
				ports += " "
			}

			ports += strconv.Itoa(int(port))
		}

		return fmt.Sprintf("route %s %s", route.Network.String(), ports)
	}
}

func (flag ConfigFlag) String() string {
	switch flag {
	case NOT_SET:
		return ""
	case ON:
		return "on"
	case OFF:
		return "off"
	case ALL:
		return "all"
	default:
		return ""
	}
}

func addRoute(network net.IPNet, ports []uint16, routes map[networkKey]map[uint16]bool) {
	key := toNetworkKey(network)
	currentPorts := routes[key]
	if currentPorts == nil {
		currentPorts = make(map[uint16]bool, len(ports))
		routes[key] = currentPorts
	}

	for _, port := range ports {
		currentPorts[port] = true
	}
}

func toNetworkKey(network net.IPNet) networkKey {
	var key networkKey

	copy(key.ip[:], network.IP.To16())
	key.mask, key.size = network.Mask.Size()
	return key
}

func toIPNet(network networkKey) (rv net.IPNet) {
	rv.IP = network.ip[:]
	rv.Mask = net.CIDRMask(network.mask, network.size)
	return rv
}

func ParseConfig(reader io.Reader) (*ConfigFile, error) {
	parser := Open(reader)

	configFile := &ConfigFile{
		GlobalConfig: &SectionConfig{
			Type: GLOBAL,
		},
		Groups: make(map[string]*SectionConfig),
		Users:  make(map[string]*SectionConfig),
	}

	var err error
	var stmt *ConfigStatement

	for stmt, err = parser.Read(); stmt != nil; stmt, err = parser.Read() {
		var section *SectionConfig

		if stmt.SectionType == "global" {
			isGlobal, err := parseGlobal(configFile, stmt)

			if err != nil {
				return nil, err
			}

			if isGlobal {
				continue
			}

			section = configFile.GlobalConfig
		} else if stmt.SectionType == "group" {
			section = configFile.Groups[stmt.SectionName]

			if section == nil {
				section = &SectionConfig{
					Type:  GROUP,
					Name:  stmt.SectionName,
					Order: len(configFile.Groups),
				}

				configFile.Groups[section.Name] = section
			}
		} else if stmt.SectionType == "user" {
			section = configFile.Users[stmt.SectionName]

			if section == nil {
				section = &SectionConfig{
					Type: USER,
					Name: stmt.SectionName,
				}

				configFile.Users[section.Name] = section
			}
		} else {
			return nil, fmt.Errorf("config:%d unrecognized section type %s", stmt.Line, stmt.SectionType)
		}

		err = parseSection(section, stmt)
	}

	if err != nil {
		return nil, err
	}

	return configFile, nil
}

func parseSection(section *SectionConfig, stmt *ConfigStatement) (err error) {
	if strings.HasPrefix(stmt.Word, "subnet-") || strings.HasPrefix(stmt.Word, "pcx-") {
		subnet := Subnet{Name: stmt.Word}

		subnet.Ports, err = parsePorts(stmt.Line, stmt.Fields)

		if err != nil {
			return err
		}

		section.Subnets = append(section.Subnets, subnet)
	} else if stmt.Word == "nat" {
		if len(stmt.Fields) == 0 {
			return fmt.Errorf("config:%d nat must have at least one argument", stmt.Line)
		}

		switch stmt.Fields[0] {
		case "on":
			if len(stmt.Fields) > 1 {
				return fmt.Errorf("config:%d nat cannot have arguments after 'on'", stmt.Line)
			}
			section.NATSetting = ALL
			break
		case "off":
			if len(stmt.Fields) > 1 {
				return fmt.Errorf("config:%d nat cannot have arguments after 'off'", stmt.Line)
			}
			section.NATSetting = OFF
			break
		default:
			route, err := parseRoute(stmt)

			if err != nil {
				return err
			}

			section.NATSetting = ON
			section.NATRoutes = append(section.NATRoutes, route)
		}
	} else if stmt.Word == "route" {
		route, err := parseRoute(stmt)

		if err != nil {
			return err
		}

		section.Routes = append(section.Routes, route)
	} else if stmt.Word == "dns" {
		if len(stmt.Fields) != 1 {
			return fmt.Errorf("config:%d dns must have exactly one argument", stmt.Line)
		}

		switch stmt.Fields[0] {
		case "on":
			section.DNSSetting = ON
			break
		case "off":
			section.DNSSetting = OFF
			break
		default:
			return fmt.Errorf("config:%d dns setting must be 'on' or 'off'", stmt.Line)
		}
	}

	return nil
}

func parseGlobal(configFile *ConfigFile, stmt *ConfigStatement) (isglobal bool, err error) {
	switch stmt.Word {
	case "watch":
		if len(stmt.Fields) != 1 {
			return true, fmt.Errorf("config:%d watch must have exactly one argument", stmt.Line)
		}
		watchTime, err := time.ParseDuration(stmt.Fields[0])

		if err != nil {
			return true, fmt.Errorf("config:%d cannot parse watch duration %s: %w", stmt.Line, stmt.Fields[0], err)
		}

		configFile.WatchTime = &watchTime

		return true, nil
	case "net":
		if len(stmt.Fields) != 1 {
			return true, fmt.Errorf("config:%d net must have exactly one argument", stmt.Line)
		}

		netString := stmt.Fields[0]

		if strings.IndexRune(netString, '/') == -1 {
			ip := net.ParseIP(netString)

			if ip == nil {
				return true, fmt.Errorf("config:%d cannot parse net %s", stmt.Line, netString)
			}

			configFile.Network = &net.IPNet{IP: ip, Mask: net.CIDRMask(24, 32)}
		} else {
			_, configFile.Network, err = net.ParseCIDR(netString)

			if err != nil {
				return true, fmt.Errorf("config:%d cannot parse net %s", stmt.Line, netString)
			}
		}

		return true, nil

	case "route53":
		fields := len(stmt.Fields)

		if fields != 2 && fields != 3 {
			return true, fmt.Errorf("config:%d route53 must have 2 or 3 arguments", stmt.Line)
		}

		configFile.Route53Zone = stmt.Fields[0]
		configFile.DomainName = stmt.Fields[1]

		if fields == 3 {
			if stmt.Fields[2] == "weighted" {
				configFile.Route53Weighted = true
			} else if stmt.Fields[2] != "simple" {
				return true, fmt.Errorf("config:%d invalid route53 entry type %s", stmt.Line, stmt.Fields[2])
			}
		}

		return true, nil

	case "key-strength":
		if len(stmt.Fields) != 1 {
			return true, fmt.Errorf("config:%d key-strength must have exactly 1 argument", stmt.Line)
		}

		strength, err := strconv.Atoi(stmt.Fields[0])

		if err != nil {
			return true, fmt.Errorf("config:%d invalid key-strength %s", stmt.Line, stmt.Fields[0])
		}

		configFile.KeyStrength = strength
		return true, nil
	}

	return false, nil
}

func parsePorts(line int, fields []string) ([]uint16, error) {
	var ports []uint16

	for _, portStr := range fields {
		port, err := strconv.ParseUint(portStr, 10, 16)

		if err != nil {
			return nil, fmt.Errorf("config:%d unable to parse port number %s: %w", line, portStr, err)
		}

		ports = append(ports, uint16(port))
	}

	return ports, nil
}

func parseRoute(stmt *ConfigStatement) (route Route, err error) {
	if len(stmt.Fields) == 0 {
		return route, fmt.Errorf("config:%d %s must have at least one argument", stmt.Line, stmt.Word)
	}

	netString := stmt.Fields[0]
	if strings.IndexRune(netString, '/') == -1 {
		ip := net.ParseIP(netString)

		if ip == nil {
			return route, fmt.Errorf("config:%d cannot parse net %s", stmt.Line, netString)
		}

		route.Network = net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
	} else {
		_, network, err := net.ParseCIDR(netString)

		if err != nil {
			return route, fmt.Errorf("config:%d cannot parse net %s", stmt.Line, netString)
		}

		route.Network = *network
	}

	route.Ports, err = parsePorts(stmt.Line, stmt.Fields[1:])

	return route, err
}
