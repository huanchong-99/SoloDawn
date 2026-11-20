//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/turn/v5"
)

type relayWorld struct {
	*rvWorld
	roots  *x509.CertPool
	tcp    string
	domain string // the signalling name; TURN over TLS is "turn." + domain
	peer   net.PacketConn
	allocs []net.PacketConn
}

func init() {
	stepSets["relay"] = func(sc *godog.ScenarioContext, _ fixtures) {
		relaySteps(sc)
	}
}

// The relay steps drive the browser and the rendezvous of the rendezvous steps.
func relaySteps(sc *godog.ScenarioContext) {
	w := &relayWorld{}
	sc.Step(`^a local TLS rendezvous with a (\d+) MiB relay quota$`, func(ctx context.Context, mib int) error {
		w.rvWorld = ctx.Value(rvKey{}).(*rvWorld)
		return w.startTLS(mib)
	})
	sc.Step(`^the browser signs the challenge and allocates a UDP and a TLS relay$`, w.allocateBoth)
	sc.Step(`^each allocation sends (\d+) packets of (\d+) bytes to a peer and receives as many back$`, w.transfer)
	sc.Step(`^neither open allocation forwards another packet in either direction, not even an empty one$`, w.cutOff)
}

func localCert(dir string) (*x509.CertPool, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost", "turn.localhost"}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	if err != nil {
		return nil, err
	}
	pair := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "cert"), pair, 0600); err != nil {
		return nil, err
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pair)
	return roots, os.WriteFile(filepath.Join(dir, "key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0600)
}

func (w *relayWorld) startTLS(mib int) error {
	dir, err := os.MkdirTemp("", "relay-cert")
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { os.RemoveAll(dir) })
	if w.roots, err = localCert(dir); err != nil {
		return err
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	w.tcp, w.domain = l.Addr().String(), "localhost"
	l.Close()
	cmd := exec.Command(w.bin, "rendezvous", "--listen", w.tcp, "--domain", "localhost", "--turn-domain", "TURN.localhost.", "--cert", filepath.Join(dir, "cert"), "--key", filepath.Join(dir, "key"), "--relay-quota-mb", strconv.Itoa(mib), "--udp-port", "0")
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { cmd.Process.Kill(); cmd.Wait() })
	w.httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: w.roots, ServerName: "localhost"}}}
	w.ws = "wss://" + w.tcp + "/rv"
	return rvListening(w.tcp)
}

// The ready frame offers STUN and TURN on one UDP port and TURN over TLS on
// the signalling port under the canonical relay name. The credential is an
// expiry one hour ahead and the device id, with a base64 HMAC-SHA1.
func (w *relayWorld) allocateBoth() error {
	issued := time.Now().Unix()
	if err := w.answerChallenge("browser", "its signature of that challenge"); err != nil {
		return err
	}
	_, b, err := rvRead(w.conns["browser"])
	if err != nil {
		return err
	}
	name := regexp.QuoteMeta(w.domain)
	frame := regexp.MustCompile(`^\{"t":"ready","ice":\[\{"urls":\["stun:` + name + `:(\d+)"\]\},` +
		`\{"urls":\["turn:` + name + `:(\d+)\?transport=udp","turns:turn\.` + name + regexp.QuoteMeta(w.tcp[strings.LastIndex(w.tcp, ":"):]) + `\?transport=tcp"\],` +
		`"username":"(\d+):[A-Za-z0-9_-]{22}","credential":"[A-Za-z0-9+/]{27}="\}\],"ttl":3600\}$`)
	m := frame.FindStringSubmatch(string(b))
	if m == nil || m[1] != m[2] {
		return fmt.Errorf("ready frame %s", b)
	}
	if expiry, _ := strconv.ParseInt(m[3], 10, 64); expiry < issued+3600 || expiry > time.Now().Unix()+3600 {
		return fmt.Errorf("credential expiry %d issued at %d", expiry, issued)
	}
	var ready proto.ICEConfig
	if err := proto.Decode(b, &ready); err != nil {
		return err
	}
	cred, addr := ready.ICE[1], net.JoinHostPort(w.domain, m[1])
	if w.peer, err = net.ListenPacket("udp4", "127.0.0.1:0"); err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { w.peer.Close() })
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	if err := w.allocate(udp, addr, cred); err != nil {
		return err
	}
	secure, err := tls.Dial("tcp", w.tcp, &tls.Config{ServerName: "turn." + w.domain, RootCAs: w.roots})
	if err != nil {
		return err
	}
	return w.allocate(turn.NewSTUNConn(secure), w.tcp, cred)
}

func (w *relayWorld) allocate(conn net.PacketConn, addr string, cred proto.ICEServer) error {
	w.cleanup = append(w.cleanup, func() { conn.Close() })
	c, err := turn.NewClient(&turn.ClientConfig{TURNServerAddr: addr, Conn: conn, Username: cred.Username, Password: cred.Credential})
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, c.Close)
	if err := c.Listen(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a, err := c.AllocateWithContext(ctx)
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { a.Close() })
	w.allocs = append(w.allocs, a)
	return c.CreatePermission(w.peer.LocalAddr())
}

func relayPacket(src, dst net.PacketConn, b []byte, discard bool) error {
	wait := 2 * time.Second
	if discard {
		wait = 120 * time.Millisecond
	}
	if err := dst.SetReadDeadline(time.Now().Add(wait)); err != nil {
		return err
	}
	n, err := src.WriteTo(b, dst.LocalAddr())
	if err != nil || n != len(b) {
		return fmt.Errorf("write %d of %d: %v", n, len(b), err)
	}
	buf := make([]byte, 2048)
	n, addr, err := dst.ReadFrom(buf)
	var timeout net.Error
	if discard && errors.As(err, &timeout) && timeout.Timeout() {
		return nil
	}
	if err != nil || discard || addr.String() != src.LocalAddr().String() || !bytes.Equal(buf[:n], b) {
		return fmt.Errorf("received %d bytes from %v, discard=%v: %v", n, addr, discard, err)
	}
	return nil
}

// transfer sends packets of size bytes from each allocation to the peer and
// as many back. The quota charges each packet its size and 40 bytes.
func (w *relayWorld) transfer(packets, size int) error {
	for i, a := range w.allocs {
		for j := range packets {
			if err := relayPacket(a, w.peer, bytes.Repeat([]byte{byte(i + j)}, size), false); err != nil {
				return err
			}
			if err := relayPacket(w.peer, a, bytes.Repeat([]byte{byte(i + j + 1)}, size), false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *relayWorld) cutOff() error {
	for _, a := range w.allocs {
		for _, b := range [][]byte{[]byte("!"), nil} {
			if err := relayPacket(a, w.peer, b, true); err != nil {
				return err
			}
			if err := relayPacket(w.peer, a, b, true); err != nil {
				return err
			}
		}
	}
	return nil
}
