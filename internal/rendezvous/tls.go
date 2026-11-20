package rendezvous

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// SNI omits a fully qualified name's final dot. Preserve that dot for DNS.
func tlsName(host string) string { return strings.ToLower(strings.TrimSuffix(host, ".")) }

// alpnTURN is the ALPN id of TURN over TLS (RFC 7443).
const alpnTURN = "stun.turn"

func (o Options) certificates() (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1", alpnTURN}}
	if o.Cert != "" {
		cert, err := tls.LoadX509KeyPair(o.Cert, o.Key)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
		return config, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	client, err := o.acmeClient()
	if err != nil {
		return nil, err
	}
	manager := &autocert.Manager{Prompt: autocert.AcceptTOS, Client: client, Cache: autocert.DirCache(filepath.Join(dir, "solodawn", "certs")), HostPolicy: autocert.HostWhitelist(tlsName(o.Domain), tlsName(o.TurnDomain))}
	config.GetCertificate = manager.GetCertificate
	config.NextProtos = append(config.NextProtos, acme.ALPNProto)
	return config, nil
}

// acmeClient talks to the ACME server of --acme-directory, Let's Encrypt when
// it is empty. With --acme-ca, that server's HTTPS is checked against those
// roots in place of the system's; nothing else trusts them.
func (o Options) acmeClient() (*acme.Client, error) {
	client := &acme.Client{DirectoryURL: o.ACMEDirectory}
	if o.ACMECA == "" {
		return client, nil
	}
	data, err := os.ReadFile(o.ACMECA)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("--acme-ca holds no certificate")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	client.HTTPClient = &http.Client{Transport: transport}
	return client, nil
}

// Both protocols have one TCP listener; only completed TLS handshakes
// with the configured SNI reach the HTTP or TURN parser.
type endpoint struct {
	addr  net.Addr
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newEndpoint(addr net.Addr) *endpoint {
	return &endpoint{addr: addr, conns: make(chan net.Conn), done: make(chan struct{})}
}
func (e *endpoint) Addr() net.Addr { return e.addr }
func (e *endpoint) Close() error   { e.once.Do(func() { close(e.done) }); return nil }
func (e *endpoint) Accept() (net.Conn, error) {
	select {
	case c := <-e.conns:
		return c, nil
	case <-e.done:
		return nil, net.ErrClosed
	}
}

func (e *endpoint) deliver(c net.Conn) {
	select {
	case e.conns <- c:
	case <-e.done:
		c.Close()
	}
}

// A live allocation sends refreshes before its ten-minute lifetime expires.
// An idle TURN stream must not hold a socket forever, even before allocation.
type turnStreams struct{ *endpoint }

func (s turnStreams) Accept() (net.Conn, error) {
	c, err := s.endpoint.Accept()
	if err != nil {
		return nil, err
	}
	return idleStream{c}, nil
}

type idleStream struct{ net.Conn }

func (c idleStream) Read(p []byte) (int, error) {
	if err := c.SetReadDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

// A client that stops reading must not hold a relay write for ever.
func (c idleStream) Write(p []byte) (int, error) {
	if err := c.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}

// httpServer serves HTTPS on the shared port and plain HTTP with --listen
// alone: a request's headers within 10 seconds, a keep-alive connection closed
// after 2 minutes idle. An upgraded WebSocket belongs to its handler and is
// cut by neither.
func httpServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
}

// sharedPort hands each TLS connection on the signalling port to HTTPS or
// TURN: by ALPN stun.turn, otherwise by server name. A source IP has at most
// maxConnsPerIP connections open on it.
type sharedPort struct {
	config *tls.Config
	names  map[string]*endpoint
	turn   *endpoint // nil without the relay
	mu     sync.Mutex
	conns  map[string]int // open connections by source IP
}

const maxConnsPerIP = 256

func (p *sharedPort) route(conn net.Conn) {
	c := tls.Server(conn, p.config)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if c.HandshakeContext(ctx) == nil {
		if e := p.endpoint(c.ConnectionState()); e != nil {
			e.deliver(c)
			return
		}
	}
	c.Close()
}

func (p *sharedPort) endpoint(state tls.ConnectionState) *endpoint {
	if state.NegotiatedProtocol == alpnTURN {
		return p.turn
	}
	return p.names[tlsName(state.ServerName)]
}

// serve accepts until the listener is closed. After any other error it waits,
// from 5 ms doubling to 1 s, and goes on, as net/http does.
func (p *sharedPort) serve(listener net.Listener) error {
	var delay time.Duration
	for {
		c, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return err
		}
		if err != nil {
			delay = min(max(2*delay, 5*time.Millisecond), time.Second)
			time.Sleep(delay)
			continue
		}
		delay = 0
		p.admit(c)
	}
}

// admit routes a connection unless its source IP already has maxConnsPerIP.
func (p *sharedPort) admit(c net.Conn) {
	ip := host(c.RemoteAddr())
	p.mu.Lock()
	full := p.conns[ip] == maxConnsPerIP
	if !full {
		p.conns[ip]++
	}
	p.mu.Unlock()
	if full {
		c.Close()
		return
	}
	go p.route(&counted{Conn: c, done: func() { p.closed(ip) }})
}

func (p *sharedPort) closed(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	decrement(p.conns, ip)
}

// counted calls done once, when the connection is first closed.
type counted struct {
	net.Conn
	once sync.Once
	done func()
}

func (c *counted) Close() error {
	c.once.Do(c.done)
	return c.Conn.Close()
}
