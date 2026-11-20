package rendezvous

import (
	"context"
	"crypto/tls"
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
	manager := &autocert.Manager{Prompt: autocert.AcceptTOS, Cache: autocert.DirCache(filepath.Join(dir, "solodawn", "certs")), HostPolicy: autocert.HostWhitelist(tlsName(o.Domain), tlsName(o.TurnDomain))}
	config.GetCertificate = manager.GetCertificate
	config.NextProtos = append(config.NextProtos, acme.ALPNProto)
	return config, nil
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

// httpsServer serves HTTPS on the shared port: a request's headers within 10
// seconds, a keep-alive connection closed after 2 minutes idle. An upgraded
// WebSocket belongs to its handler and is cut by neither.
func httpsServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
}

// sharedPort hands each TLS connection on the signalling port to HTTPS or
// TURN: by ALPN stun.turn, otherwise by server name.
type sharedPort struct {
	config *tls.Config
	names  map[string]*endpoint
	turn   *endpoint // nil without the relay
}

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

func (p *sharedPort) serve(listener net.Listener) error {
	for {
		c, err := listener.Accept()
		if err != nil {
			return err
		}
		go p.route(c)
	}
}
