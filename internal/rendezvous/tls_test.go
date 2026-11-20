package rendezvous

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/turn/v5"
)

// The TLS handshake on the shared port must finish within 10 seconds.
func TestTLSHandshakeDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := net.Pipe()
		start := time.Now()
		go (&sharedPort{config: &tls.Config{}}).route(a)
		if n, err := b.Read(make([]byte, 1)); n != 0 || err != io.EOF || time.Since(start) != 10*time.Second {
			t.Fatal(n, err, time.Since(start))
		}
	})
}

// closedAfter reports how long after it is called the other end closes c.
func closedAfter(c net.Conn) <-chan time.Duration {
	start, done := time.Now(), make(chan time.Duration, 1)
	go func() {
		io.Copy(io.Discard, c)
		done <- time.Since(start)
	}()
	return done
}

// HTTPS on the shared port and plain HTTP with --listen alone wait 10 seconds
// for a request's headers and close a keep-alive connection idle for 120
// seconds. An upgraded WebSocket is cut by neither.
func TestHTTPTimeouts(t *testing.T) {
	web := fstest.MapFS{"index.html": {Data: []byte("x")}}
	for name, serve := range map[string]func(net.Listener){
		"shared TLS port": func(l net.Listener) { httpServer(Handler(web)).Serve(l) },
		"plain HTTP":      func(l net.Listener) { Options{}.serve(web, l) },
	} {
		t.Run(name, func(t *testing.T) { testHTTPTimeouts(t, serve) })
	}
}

func testHTTPTimeouts(t *testing.T, serve func(net.Listener)) {
	synctest.Test(t, func(t *testing.T) {
		e := newEndpoint(&net.TCPAddr{})
		go serve(e)
		dial := func() net.Conn {
			a, b := net.Pipe()
			go e.deliver(a)
			return b
		}

		slow := dial()
		slow.Write([]byte("GET / HTTP/1.1\r\n"))
		if d := <-closedAfter(slow); d != 10*time.Second {
			t.Fatalf("headers: closed after %v", d)
		}

		idle := dial()
		idle.Write([]byte("GET / HTTP/1.1\r\nHost: rv\r\n\r\n"))
		resp, err := http.ReadResponse(bufio.NewReader(idle), nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatal(resp, err)
		}
		if d := <-closedAfter(idle); d != 2*time.Minute {
			t.Fatalf("keep-alive: closed after %v", d)
		}

		tr := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) { return dial(), nil }}
		ws, _, err := websocket.Dial(context.Background(), "ws://rv/rv", &websocket.DialOptions{HTTPClient: &http.Client{Transport: tr}})
		if err != nil {
			t.Fatal(err)
		}
		var challenge proto.Challenge
		if _, b, err := ws.Read(context.Background()); err != nil || proto.Decode(b, &challenge) != nil {
			t.Fatal(b, err)
		}
		send(t, ws, proto.Auth{Type: "auth", Key: key(1).ID(), Signature: key(1).SignChallenge(challenge.Nonce)})
		expect(t, ws, proto.ICEConfig{Type: "ready", ICE: []proto.ICEServer{}, TTL: 3600})
		time.Sleep(time.Hour)
		send(t, ws, proto.RVFrame{Type: "ice"})
		expect(t, ws, proto.ICEConfig{Type: "ice", ICE: []proto.ICEServer{}, TTL: 3600})
		ws.CloseNow()
		tr.CloseIdleConnections()
		e.Close()
	})
}

// A TURN stream that receives nothing for 10 minutes is closed; each read
// starts the 10 minutes again.
func TestTURNReadIdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, b := net.Pipe()
		stream := idleStream{a}
		defer b.Close()
		start := time.Now()
		go func() { time.Sleep(9 * time.Minute); b.Write([]byte("x")) }()
		buf := make([]byte, 1)
		if n, err := stream.Read(buf); err != nil || n != 1 || string(buf) != "x" {
			t.Fatal(n, err, buf)
		}
		if n, err := stream.Read(buf); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) || time.Since(start) != 19*time.Minute {
			t.Fatal(n, err, time.Since(start))
		}
		a.Close()
		if n, err := stream.Read(buf); n != 0 || !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(n, err)
		}
	})
}

// A connection that negotiates ALPN stun.turn (RFC 7443) is TURN whatever its
// server name; without the relay it is closed.
func TestTURNByALPN(t *testing.T) {
	o, roots := testCert(t)
	for _, noRelay := range []bool{false, true} {
		o.NoRelay, o.Domain, o.TurnDomain = noRelay, "localhost", "turn.example.com"
		l := listenLocal(t)
		done := make(chan error, 1)
		go func() { done <- o.serveICE(fstest.MapFS{}, l) }()
		c, err := tls.Dial("tcp", l.Addr().String(), &tls.Config{ServerName: "example.com", RootCAs: roots, NextProtos: []string{"stun.turn"}})
		if err != nil {
			t.Fatal(err)
		}
		if p := c.ConnectionState().NegotiatedProtocol; p != "stun.turn" {
			t.Fatalf("ALPN %q", p)
		}
		if noRelay {
			c.SetReadDeadline(time.Now().Add(time.Second))
			if n, err := c.Read(make([]byte, 1)); n != 0 || err != io.EOF {
				t.Fatalf("stun.turn without relay: %d, %v", n, err)
			}
		} else {
			client, err := turn.NewClient(&turn.ClientConfig{Conn: turn.NewSTUNConn(c), STUNServerAddr: l.Addr().String()})
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Listen(); err != nil {
				t.Fatal(err)
			}
			if addr, err := client.SendBindingRequest(); err != nil || addr.String() != c.LocalAddr().String() {
				t.Fatal(addr, err)
			}
			client.Close()
		}
		c.Close()
		l.Close()
		if err := <-done; !errors.Is(err, net.ErrClosed) {
			t.Fatal(err)
		}
	}
}

// --acme-directory and --acme-ca choose the ACME server of the automatic
// certificates and the roots its HTTPS is checked against.
func TestACMEServer(t *testing.T) {
	requests := make(chan string, 4)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(name, dir) // the certificate cache
	}
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ca := write("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	o := Options{Domain: "rv.example", TurnDomain: "turn.rv.example", ACMEDirectory: srv.URL + "/dir"}
	hello := &tls.ClientHelloInfo{ServerName: "rv.example"}
	for _, trusted := range []bool{false, true} {
		if trusted {
			o.ACMECA = ca
		}
		cfg, err := o.certificates()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cfg.GetCertificate(hello); err == nil {
			t.Fatal("certificate from a directory that has none")
		}
		select {
		case got := <-requests:
			if !trusted || got != "GET /dir" {
				t.Fatalf("trusted %v: request %q", trusted, got)
			}
		default:
			if trusted {
				t.Fatal("the ACME server was not asked")
			}
		}
	}
	o.ACMECA = filepath.Join(dir, "missing.pem")
	if _, err := o.certificates(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	o.ACMECA = write("empty.pem", []byte("no certificate"))
	if _, err := o.certificates(); err == nil || err.Error() != "--acme-ca holds no certificate" {
		t.Fatal(err)
	}
}
