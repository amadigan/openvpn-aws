package dns

import (
	"context"
	"github.com/amadigan/openvpn-aws/internal/log"
	"github.com/miekg/dns"
	"time"
)

var logger = log.New("dns")

type DNSProxy struct {
	udp *handler
	tcp *handler
}

func StartProxy(addr string) (*DNSProxy, error) {
	addr = addr + ":53"
	clientConfig, err := dns.ClientConfigFromFile("/etc/resolv.conf")

	if err != nil {
		return nil, err
	}

	proxy := new(DNSProxy)

	proxy.udp = &handler{servers: make([]string, len(clientConfig.Servers)), client: new(dns.Client)}

	for k, v := range clientConfig.Servers {
		proxy.udp.servers[k] = v + ":53"
	}

	proxy.tcp = &handler{servers: proxy.udp.servers, client: &dns.Client{Net: "tcp"}}

	err = proxy.udp.serve(addr, "udp")

	if err != nil {
		return nil, err
	}

	err = proxy.tcp.serve(addr, "tcp")

	if err != nil {
		proxy.udp.stop()
		return nil, err
	}

	return proxy, nil
}

func (proxy *DNSProxy) Stop() error {
	udpErr := proxy.udp.stop()
	tcpErr := proxy.tcp.stop()

	if udpErr != nil {
		return udpErr
	}

	return tcpErr
}

type handler struct {
	servers []string
	client  *dns.Client
	server  *dns.Server
	channel chan state
}

type state struct {
	started bool
	err     error
}

func (h *handler) ServeDNS(w dns.ResponseWriter, msg *dns.Msg) {
	for _, addr := range h.servers {
		r, _, err := h.client.Exchange(msg, addr)

		if err == nil {
			w.WriteMsg(r)
			break
		} else {
			logger.Error("Error: %s", err)
		}
	}

	m := new(dns.Msg)
	m.SetRcode(msg, dns.RcodeServerFailure)
	w.WriteMsg(m)
}

func (h *handler) started() {
	h.channel <- state{started: true}
}

func (h *handler) serve(addr, net string) error {
	h.server = &dns.Server{Addr: addr, Net: net, Handler: h, NotifyStartedFunc: h.started}
	h.channel = make(chan state)

	go h.start()

	state := <-h.channel

	return state.err
}

func (h *handler) start() {
	err := h.server.ListenAndServe()

	if err != nil {
		h.channel <- state{err: err}
	}

	close(h.channel)
}

func (h *handler) stop() error {
	d := time.Now().Add(100 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), d)
	defer cancel()
	err := h.server.ShutdownContext(ctx)

	if err != nil {
		return err
	}

	state := <-h.channel

	return state.err
}
