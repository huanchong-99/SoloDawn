package rendezvous

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"strconv"
	"strings"
)

// Options names the endpoints of the standalone service, not a node's handler.
// ACMEDirectory and ACMECA choose the ACME server of the automatic certificates
// (Let's Encrypt when empty) and the roots its HTTPS is checked against.
// RelayPorts is the range of relay allocation ports, DefaultRelayPorts when
// it is not a valid range.
type Options struct {
	Listen, Domain, TurnDomain, Cert, Key string
	ACMEDirectory, ACMECA                 string
	UDPPort                               int
	RelayPorts                            PortRange
	QuotaMB                               int64
	NoRelay                               bool
}

// PortRange is a range of ports, written first-last.
type PortRange struct{ First, Last int }

// DefaultRelayPorts lies outside the ephemeral port ranges of Linux, macOS
// and Windows.
var DefaultRelayPorts = PortRange{20000, 29999}

var errPortRange = errors.New("want first-last, two ports from 1 to 65535 in order")

func (r *PortRange) String() string { return fmt.Sprintf("%d-%d", r.First, r.Last) }

func (r *PortRange) Set(text string) error {
	first, last, _ := strings.Cut(text, "-")
	a, errFirst := strconv.Atoi(first)
	b, errLast := strconv.Atoi(last)
	if errors.Join(errFirst, errLast) != nil || !(PortRange{a, b}).valid() {
		return errPortRange
	}
	*r = PortRange{a, b}
	return nil
}

func (r PortRange) valid() bool { return 0 < r.First && r.First <= r.Last && r.Last <= 65535 }

func (o Options) relayPorts() PortRange {
	if o.RelayPorts.valid() {
		return o.RelayPorts
	}
	return DefaultRelayPorts
}

func (o Options) validate() error {
	if (o.Cert == "") != (o.Key == "") {
		return errors.New("--cert and --key must be supplied together")
	}
	if o.QuotaMB < 0 || o.QuotaMB >= 1<<33 {
		return errors.New("--relay-quota-mb is outside the wire integer range")
	}
	return o.validateNames()
}

func (o Options) validateNames() error {
	if o.Domain == "" {
		if o.Cert != "" || o.TurnDomain != "" || o.NoRelay {
			return errors.New("--domain is required for TLS and STUN")
		}
		return nil
	}
	return o.validateRelayName()
}

func (o Options) validateRelayName() error {
	if !o.NoRelay && (o.TurnDomain == "" || tlsName(o.Domain) == tlsName(o.TurnDomain)) {
		return errors.New("relay needs a distinct --turn-domain")
	}
	return nil
}

func (o Options) address() string {
	if o.Listen != "" {
		return o.Listen
	}
	if o.Domain != "" {
		return ":443"
	}
	return "127.0.0.1:8080"
}

func (o Options) Serve(web fs.FS) error {
	if err := o.validate(); err != nil {
		return err
	}
	l, err := net.Listen("tcp", o.address())
	if err != nil {
		return err
	}
	defer l.Close()
	return o.serve(web, l)
}

// serve answers on l until it is closed: plain HTTP without a domain,
// otherwise TLS signalling, STUN and, unless disabled, TURN.
func (o Options) serve(web fs.FS, l net.Listener) error {
	if o.Domain == "" {
		return httpServer(Handler(web)).Serve(l)
	}
	return o.serveICE(web, l)
}

func (o Options) serveICE(web fs.FS, l net.Listener) error {
	cfg, err := o.certificates()
	if err != nil {
		return err
	}
	host := l.Addr().(*net.TCPAddr).IP.String()
	udp, err := net.ListenPacket("udp", net.JoinHostPort(host, strconv.Itoa(o.UDPPort)))
	if err != nil {
		return err
	}
	defer udp.Close()
	httpEndpoint := newEndpoint(l.Addr())
	defer httpEndpoint.Close()
	port := &sharedPort{config: cfg, names: map[string]*endpoint{tlsName(o.Domain): httpEndpoint}, conns: map[string]int{}}
	var turnListener net.Listener
	if !o.NoRelay {
		port.turn = newEndpoint(l.Addr())
		port.names[tlsName(o.TurnDomain)] = port.turn
		turnListener = turnStreams{port.turn}
	}
	r, server, err := o.start(udp, turnListener, l.Addr().(*net.TCPAddr).Port)
	if err != nil {
		return err
	}
	defer server.Close()
	stop := make(chan struct{})
	defer close(stop)
	go r.report(stop, log.Printf)
	go httpServer(r.handler(web)).Serve(httpEndpoint)
	return port.serve(l)
}
