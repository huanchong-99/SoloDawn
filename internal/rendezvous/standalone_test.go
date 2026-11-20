package rendezvous

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pion/turn/v5"
)

func TestStandaloneOptions(t *testing.T) {
	noDomain := "--domain is required for TLS and STUN"
	distinct := "relay needs a distinct --turn-domain"
	pair := "--cert and --key must be supplied together"
	quota := "--relay-quota-mb is outside the wire integer range"
	for _, tc := range []struct {
		o   Options
		msg string
	}{
		{Options{}, ""}, {Options{Domain: "localhost", NoRelay: true}, ""},
		{Options{Domain: "localhost", TurnDomain: "turn.localhost", Cert: "c", Key: "k", QuotaMB: 1<<33 - 1}, ""},
		{Options{Cert: "c"}, pair}, {Options{Key: "k"}, pair}, {Options{Cert: "c", Key: "k"}, noDomain},
		{Options{TurnDomain: "turn.localhost"}, noDomain}, {Options{NoRelay: true}, noDomain},
		{Options{Domain: "localhost"}, distinct}, {Options{Domain: "localhost", TurnDomain: "LOCALHOST"}, distinct},
		{Options{Domain: "localhost.", TurnDomain: "localhost"}, distinct},
		{Options{QuotaMB: -1}, quota}, {Options{QuotaMB: 1 << 33}, quota},
	} {
		err := tc.o.validate()
		if tc.msg == "" {
			if err != nil {
				t.Fatal(tc.o, err)
			}
		} else if err == nil || err.Error() != tc.msg {
			t.Fatal(tc.o, err)
		}
	}
	if got := tlsName("Rv.Example."); got != "rv.example" {
		t.Fatal(got)
	}
	for _, tc := range []struct {
		o    Options
		addr string
	}{{Options{}, "127.0.0.1:8080"}, {Options{Domain: "rv"}, ":443"}, {Options{Domain: "rv", Listen: "localhost:9000"}, "localhost:9000"}} {
		if got := tc.o.address(); got != tc.addr {
			t.Errorf("%+v: %q", tc.o, got)
		}
	}
}

func testCert(t *testing.T) (Options, *x509.CertPool) {
	t.Helper()
	s := httptest.NewTLSServer(nil)
	defer s.Close()
	c := s.TLS.Certificates[0]
	der, err := x509.MarshalPKCS8PrivateKey(c.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(typ string, data []byte) string {
		path := filepath.Join(dir, typ)
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: data}), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	return Options{Domain: "example.com", NoRelay: true, Cert: write("CERTIFICATE", c.Certificate[0]), Key: write("PRIVATE KEY", der)}, roots
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestStandaloneTLS(t *testing.T) {
	web := fstest.MapFS{"index.html": {Data: []byte("static beside signalling")}}
	o, roots := testCert(t)
	for _, noRelay := range []bool{true, false} {
		o.NoRelay = noRelay
		l := listenLocal(t)
		host := o.Domain
		if !noRelay {
			o.Domain = "localhost"
			o.TurnDomain = "TURN.example.com."
			host = "turn.example.com"
		}
		done := make(chan error, 1)
		go func() { done <- o.serveICE(web, l) }()
		cfg := &tls.Config{ServerName: host, RootCAs: roots}
		if noRelay {
			tr := &http.Transport{TLSClientConfig: cfg}
			c := http.Client{Transport: tr, Timeout: 2 * time.Second}
			resp, err := c.Get("https://" + l.Addr().String() + "/")
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			tr.CloseIdleConnections()
			if err != nil || string(b) != "static beside signalling" {
				t.Fatalf("HTTPS: %s %v", b, err)
			}
		} else {
			c, err := tls.Dial("tcp", l.Addr().String(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			pc := turn.NewSTUNConn(c)
			client, err := turn.NewClient(&turn.ClientConfig{Conn: pc, STUNServerAddr: l.Addr().String()})
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Listen(); err != nil {
				t.Fatal(err)
			}
			addr, err := client.SendBindingRequest()
			if err != nil || addr.String() != c.LocalAddr().String() {
				t.Fatal(addr, err)
			}
			client.Close()
			c.Close()
		}
		c, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{ServerName: "unserved.example.com", RootCAs: roots})
		if err != nil {
			t.Fatal(err)
		}
		c.SetReadDeadline(time.Now().Add(time.Second))
		if n, err := c.Read(make([]byte, 1)); n != 0 || err != io.EOF {
			t.Fatalf("unknown SNI: %d, %v", n, err)
		}
		c.Close()
		l.Close()
		if err := <-done; !errors.Is(err, net.ErrClosed) {
			t.Fatal(err)
		}
	}
}

func TestPlainHTTP(t *testing.T) {
	l := listenLocal(t)
	done := make(chan error, 1)
	go func() { done <- Options{}.serve(fstest.MapFS{"index.html": {Data: []byte("local")}}, l) }()
	resp, err := http.Get("http://" + l.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" || string(b) != "local" {
		t.Fatalf("plain HTTP: %s %v", b, err)
	}
	l.Close()
	if err := <-done; !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}

func TestServeFailures(t *testing.T) {
	web := fstest.MapFS{}
	// Misconfigured services must return, not enter a serving loop.
	start := func(o Options) error {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- o.Serve(web) }()
		select {
		case err := <-done:
			return err
		case <-time.After(time.Second):
			return errors.New("invalid service did not stop")
		}
	}
	if err := start(Options{NoRelay: true}); err == nil || err.Error() != "--domain is required for TLS and STUN" {
		t.Fatal(err)
	}
	if err := start(Options{Domain: "rv", NoRelay: true, Listen: "bad"}); err == nil || err.Error() != "listen tcp: address bad: missing port in address" {
		t.Fatal(err)
	}
	o := Options{Domain: "localhost", TurnDomain: "turn.localhost", Cert: "bad", Key: "bad", Listen: "127.0.0.1:0"}
	if err := start(o); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	o.Cert, o.Key = "", ""
	udp := udpLocal(t)
	o.UDPPort = udp.LocalAddr().(*net.UDPAddr).Port
	l := listenLocal(t)
	// Windows answers WSAEADDRINUSE (10048), which syscall.EADDRINUSE does not match there.
	if err := o.serveICE(web, l); !errors.Is(err, syscall.EADDRINUSE) && !errors.Is(err, syscall.Errno(10048)) {
		t.Fatal(err)
	}
	o.Domain = "invalid domain%"
	o.UDPPort = 0
	var dns *net.DNSError
	if err := o.serveICE(web, l); !errors.As(err, &dns) || dns.Name != o.Domain {
		t.Fatal(err)
	}
}

func TestCertificatePolicy(t *testing.T) {
	o := Options{Domain: "rv.example", TurnDomain: "turn.rv.example"}
	cfg, err := o.certificates()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != tls.VersionTLS12 || !reflect.DeepEqual(cfg.NextProtos, []string{"http/1.1", "stun.turn", "acme-tls/1"}) {
		t.Fatal(cfg.MinVersion, cfg.NextProtos)
	}
	_, err = cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "unconfigured.example"})
	if err == nil || err.Error() != `acme/autocert: host "unconfigured.example" not configured in HostWhitelist` {
		t.Fatal(err)
	}
	for _, name := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(name, "")
	}
	_, want := os.UserCacheDir()
	if _, err := o.certificates(); err == nil || err.Error() != want.Error() {
		t.Fatal(err, want)
	}
}

func TestClosedEndpoint(t *testing.T) {
	e := newEndpoint(&net.TCPAddr{Port: 443})
	if e.Addr().String() != ":443" {
		t.Fatal(e.Addr())
	}
	e.Close()
	a, b := net.Pipe()
	defer b.Close()
	e.deliver(a)
	if n, err := b.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatal(n, err)
	}
	if c, err := e.Accept(); c != nil || !errors.Is(err, net.ErrClosed) {
		t.Fatal(c, err)
	}
}
