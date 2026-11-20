package spike

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const (
	webName    = "rv.example.test"
	turnName   = "turn.rv.example.test"
	labIP      = "192.0.2.10"
	quotaLimit = 48
	turnURL    = "turns:" + turnName + ":443?transport=tcp"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

type branch struct {
	net.Listener
	conns chan net.Conn
}

func (b branch) Accept() (net.Conn, error) { return <-b.conns, nil }

func route(raw net.Listener, config *tls.Config, web, relay branch) {
	for {
		conn := must(raw.Accept())
		go func() {
			c := tls.Server(conn, config)
			if err := c.Handshake(); err != nil {
				fmt.Println("handshake:", err)
				c.Close()
				return
			}
			s := c.ConnectionState()
			fmt.Printf("443 SNI=%s ALPN=%q\n", s.ServerName, s.NegotiatedProtocol)
			switch {
			case s.NegotiatedProtocol == acme.ALPNProto:
				c.Close()
			case s.ServerName == webName:
				web.conns <- c
			case s.ServerName == turnName:
				relay.conns <- c
			default:
				c.Close()
			}
		}()
	}
}

func tlsSerial(name string) string {
	conn := must(tls.Dial("tcp", name+":443", &tls.Config{ServerName: name}))
	defer conn.Close()
	leaf := conn.ConnectionState().VerifiedChains[0][0]
	fmt.Printf("verified SAN=%v issuer=%s serial=%x\n", leaf.DNSNames, leaf.Issuer.CommonName, leaf.SerialNumber)
	return leaf.SerialNumber.Text(16)
}

func TestSpike(t *testing.T) {
	raw := must(net.Listen("tcp4", labIP+":443"))
	web, relay := branch{raw, make(chan net.Conn)}, branch{raw, make(chan net.Conn)}
	manager := &autocert.Manager{
		Prompt: autocert.AcceptTOS, Cache: autocert.DirCache(t.TempDir()),
		HostPolicy:  autocert.HostWhitelist(webName, turnName),
		Client:      &acme.Client{DirectoryURL: "https://localhost:14000/dir"},
		RenewBefore: 50 * time.Second,
	}
	config := manager.TLSConfig()
	config.NextProtos = []string{"http/1.1", acme.ALPNProto}
	go route(raw, config, web, relay)
	secret := rand.Text()
	g := &generator{
		RelayAddressGeneratorStatic: &turn.RelayAddressGeneratorStatic{RelayAddress: net.ParseIP(labIP), Address: labIP},
		accounts:                    map[string]*meter{quotaID: {}, "control": {}},
	}
	must(turn.NewServer(turn.ServerConfig{
		Realm:           webName,
		AuthHandler:     turn.LongTermTURNRESTAuthHandler(secret, nil),
		ListenerConfigs: []turn.ListenerConfig{{Listener: relay, RelayAddressGenerator: g}},
	}))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "S-6 HTTPS") })
	go http.Serve(web, mux)
	initial := map[string]string{}
	for _, name := range []string{webName, turnName} {
		initial[name] = tlsSerial(name)
	}
	resp := must(http.Get("https://" + webName + "/"))
	got := string(must(io.ReadAll(resp.Body)))
	check(resp.Body.Close())
	if resp.StatusCode != http.StatusOK || got != "S-6 HTTPS" {
		t.Fatal(resp.Status, got)
	}
	t.Logf("HTTPS GET / = %s %q", resp.Status, got)
	t.Run("quota", func(t *testing.T) { probeQuota(t, secret, g) })
	t.Run("clients", func(t *testing.T) { probeClients(t, secret, mux) })
	t.Run("certificate-renewal", func(t *testing.T) {
		// The lab CA gives each certificate 75 seconds. Poll the live endpoints.
		deadline := time.Now().Add(70 * time.Second)
		for name, serial := range initial {
			for tlsSerial(name) == serial {
				if time.Now().After(deadline) {
					t.Fatalf("%s still on first leaf", name)
				}
				time.Sleep(2 * time.Second)
			}
		}
	})
}

func issue(secret, id string, ttl time.Duration) webrtc.ICEServer {
	user, password, err := turn.GenerateLongTermTURNRESTCredentials(secret, id, ttl)
	check(err)
	return webrtc.ICEServer{URLs: []string{turnURL}, Username: user, Credential: password}
}
