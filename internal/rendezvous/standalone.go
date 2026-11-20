package rendezvous

import (
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
)

// Options names the endpoints of the standalone service, not a node's handler.
type Options struct {
	Listen, Domain, TurnDomain, Cert, Key string
	UDPPort                               int
	QuotaMB                               int64
	NoRelay                               bool
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
		return http.Serve(l, Handler(web))
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
	port := &sharedPort{config: cfg, names: map[string]*endpoint{tlsName(o.Domain): httpEndpoint}}
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
	go httpsServer(r.handler(web)).Serve(httpEndpoint)
	return port.serve(l)
}
