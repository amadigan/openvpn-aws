package vpn

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/amadigan/openvpn-aws/internal/log"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type OpenVPN struct {
	StateChannel  chan *VPNStateEvent
	ClientChannel chan *VPNClientEvent
	cmd           *exec.Cmd
	cmdChannel    chan vpnCommand
	respChannel   chan error
}

type VPNStateEvent struct {
	Time          int64
	State         string
	Description   string
	IPv4          net.IP
	RemoteAddress net.IP
	RemotePort    int
	LocalAddress  net.IP
	LocalPort     int
	IPv6          net.IP
}

type VPNClientEvent struct {
	Type        string // CONNECT,REAUTH,ESTABLISHED,DISCONNECT,ADDRESS
	ClientId    uint64
	KeyId       uint64            // only for CONNECT and REAUTH
	Primary     bool              // only for ADDRESS
	Address     net.IPNet         // only for ADDRESS
	Environment map[string]string // nil for ADDRESS
}

type vpnCommand struct {
	command        string
	expectResponse bool
}

func StartOpenVPN(socketPath, config string, cert, key []byte, network net.IPNet) (*OpenVPN, error) {
	_, err := os.Stat("/dev/net/tun")

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll("/dev/net", 0755)

			if err != nil {
				return nil, err
			}

			syscall.Mknod("/dev/net/tun", syscall.S_IFCHR, int(unix.Mkdev(10, 200)))
		} else {
			return nil, err
		}
	}

	var verbosity int

	switch log.LogLevel {
	case log.DEBUG:
		verbosity = 2
		break
	case log.INFO:
	case log.WARN:
		verbosity = 1
		break
	case log.ERROR:
		verbosity = 0
		break
	}

	confFile, err := os.OpenFile(config, os.O_APPEND|os.O_WRONLY, 0644)

	if err != nil {
		return nil, err
	}

	defer confFile.Close()

	confWriter := bufio.NewWriter(confFile)

	confWriter.WriteString(fmt.Sprintf("\nverb %d\n", verbosity))

	confWriter.WriteString(fmt.Sprintf("\nserver %s %s\n", network.IP.String(), net.IPv4(255, 255, 255, 255).Mask(network.Mask).String()))

	confWriter.WriteString("\n<cert>\n")
	confWriter.Write(cert)
	confWriter.WriteString("\n</cert>\n")

	confWriter.WriteString("\n<key>\n")
	confWriter.Write(key)
	confWriter.WriteString("\n</key>\n")

	confWriter.Flush()
	confFile.Close()

	addr, err := net.ResolveUnixAddr("unix", socketPath)

	if err != nil {
		return nil, err
	}

	listener, err := net.ListenUnix("unix", addr)

	if err != nil {
		return nil, err
	}

	vpnman := new(OpenVPN)

	vpnman.cmdChannel = make(chan vpnCommand)
	vpnman.respChannel = make(chan error)
	vpnman.StateChannel = make(chan *VPNStateEvent)
	vpnman.ClientChannel = make(chan *VPNClientEvent)

	cmd := exec.Command("openvpn", "--config", config)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()

	if err != nil {
		return nil, err
	}

	vpnman.cmd = cmd

	go vpnman.run(listener)

	return vpnman, nil
}

func (m *OpenVPN) run(listener *net.UnixListener) {
	defer func() {
		listener.Close()
		close(m.respChannel)
		m.cmd.Wait()
	}()

	conn, err := listener.AcceptUnix()

	if err != nil {
		panic(err)
	}

	defer conn.Close()
	listener.Close()

	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			panic(err)
		}

		if strings.HasPrefix(line, ">HOLD:") {
			break
		}
	}

	readChannel := make(chan string)
	go m.readerChannel(reader, readChannel)

	err = sendCommand("state on", true, readChannel, conn)

	if err != nil {
		panic(err)
	}

	err = sendCommand("hold release", true, readChannel, conn)

	if err != nil {
		panic(err)
	}

	for {
		command, more := <-m.cmdChannel

		if !more {
			break
		}

		response := sendCommand(command.command, command.expectResponse, readChannel, conn)

		if command.expectResponse {
			m.respChannel <- response
		}

	}
}

func sendCommand(command string, expectResponse bool, readChannel chan string, conn *net.UnixConn) error {
	_, err := io.WriteString(conn, command+"\n")

	if err != nil {
		return err
	}

	if expectResponse {
		response, more := <-readChannel

		if !more || strings.HasPrefix(response, "SUCCESS:") {
			return nil
		} else {
			return errors.New(response)
		}
	} else {
		return nil
	}
}

func (m *OpenVPN) readEvent(line string, reader *bufio.Reader) error {
	line = strings.TrimSpace(line)
	logger.Debug("EVENT:", line)

	if strings.HasPrefix(line, ">STATE:") {

		event, err := parseStateEvent(line)

		if err != nil {
			return err
		}

		m.StateChannel <- event
	} else if strings.HasPrefix(line, ">CLIENT:") {
		event, err := parseClientEvent(line, reader)

		if err != nil {
			return err
		}

		m.ClientChannel <- event
	}
	return nil
}

func parseStateEvent(line string) (*VPNStateEvent, error) {
	parts := strings.Split(line[len(">STATE:"):], ",")

	event := new(VPNStateEvent)

	time, err := strconv.ParseInt(parts[0], 10, 64)

	if err != nil {
		return nil, err
	}

	event.Time = time
	event.State = parts[1]
	event.Description = parts[2]
	event.IPv4 = net.ParseIP(parts[3])
	event.RemoteAddress = net.ParseIP(parts[4])

	if parts[5] != "" {
		num, err := strconv.ParseInt(parts[5], 10, 32)

		if err != nil {
			return nil, err
		}

		event.RemotePort = int(num)
	}

	event.LocalAddress = net.ParseIP(parts[6])

	if parts[6] != "" {
		num, err := strconv.ParseInt(parts[7], 10, 32)

		if err != nil {
			return nil, err
		}

		event.LocalPort = int(num)
	}

	if len(parts) > 8 {
		event.IPv6 = net.ParseIP(parts[8])
	}

	return event, nil
}

func parseClientEvent(line string, reader *bufio.Reader) (*VPNClientEvent, error) {
	parts := strings.Split(line[len(">CLIENT:"):], ",")

	event := new(VPNClientEvent)

	event.Type = parts[0]

	num, err := strconv.ParseUint(parts[1], 10, 64)

	if err != nil {
		return nil, err
	}

	event.ClientId = num

	if event.Type == "CONNECT" || event.Type == "REAUTH" {
		num, err := strconv.ParseUint(parts[2], 10, 64)

		if err != nil {
			return nil, err
		}

		event.KeyId = num
	}

	if event.Type == "ADDRESS" {
		addrString := parts[2]
		slash := strings.IndexRune(addrString, '/')

		var address net.IPNet

		if slash != -1 {
			address.IP = net.ParseIP(addrString[:slash])

			maskIp := net.ParseIP(addrString[slash+1:])

			address.Mask = net.IPv4Mask(maskIp[0], maskIp[1], maskIp[2], maskIp[3])
		} else {
			address.IP = net.ParseIP(addrString)
			address.Mask = net.CIDRMask(32, 32)
		}

		event.Address = address

		if parts[3] == "1" {
			event.Primary = true
		}
	} else {
		event.Environment = make(map[string]string)

		for {
			line, err = reader.ReadString('\n')

			if err != nil {
				return nil, err
			}

			line = strings.TrimSpace(line)

			if line == ">CLIENT:ENV,END" {
				break
			}

			if !strings.HasPrefix(line, ">CLIENT:ENV,") {
				return nil, fmt.Errorf("Unable to read client event line %s", line)
			}

			varline := line[len(">CLIENT:ENV,"):]

			equal := strings.IndexRune(varline, '=')

			if equal == -1 {
				return nil, fmt.Errorf("Unable to read client event line %s", line)
			}

			varname := varline[:equal]

			varval := varline[equal+1:]

			event.Environment[varname] = varval
		}
	}

	return event, nil
}

func (m *OpenVPN) readerChannel(reader *bufio.Reader, readChannel chan string) {
	defer func() {
		close(readChannel)
		close(m.StateChannel)
		close(m.ClientChannel)
	}()

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if errors.Is(io.EOF, err) || errors.Is(os.ErrClosed, err) {
				break
			}

			panic(err)
		}

		if strings.HasPrefix(line, ">") {
			err = m.readEvent(line, reader)

			if err != nil {
				logger.Error(err)
			}
		} else {
			readChannel <- line
		}

	}
}

func (m *OpenVPN) ExecCommand(command string, expectResponse bool) error {
	defer recover() // Ignore panics caused by writing to a closed channel
	m.cmdChannel <- vpnCommand{command: command, expectResponse: expectResponse}

	if expectResponse {
		response := <-m.respChannel
		return response
	} else {
		return nil
	}
}

func (m *OpenVPN) Shutdown() {
	close(m.cmdChannel)
	m.cmd.Wait()
}
