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
	peer   net.PacketConn
	allocs []net.PacketConn
}

func relaySteps(sc *godog.ScenarioContext, rv *rvWorld) {
	w := &relayWorld{rvWorld: rv}
	sc.Step(`^a local TLS rendezvous with a (\d+) MiB relay quota$`, w.startTLS)
	sc.Step(`^the browser signs the challenge and allocates a UDP and a TLS relay$`, w.allocateBoth)
	sc.Step(`^each allocation forwards (\d+) KiB to a peer and (\d+) KiB back$`, w.transfer)
	sc.Step(`^neither open allocation forwards another byte in either direction$`, w.cutOff)
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
	w.tcp = l.Addr().String()
	l.Close()
	cmd := exec.Command(w.bin, "rendezvous", "--listen", w.tcp, "--domain", "localhost", "--turn-domain", "turn.localhost", "--cert", filepath.Join(dir, "cert"), "--key", filepath.Join(dir, "key"), "--relay-quota-mb", strconv.Itoa(mib), "--udp-port", "0")
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { cmd.Process.Kill(); cmd.Wait() })
	w.httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: w.roots, ServerName: "localhost"}}}
	w.ws = "wss://" + w.tcp + "/rv"
	return rvListening(w.tcp)
}

func (w *relayWorld) allocateBoth() error {
	if err := w.answerChallenge("browser", "its signature of that challenge"); err != nil {
		return err
	}
	_, b, err := rvRead(w.conns["browser"])
	if err != nil {
		return err
	}
	var ready proto.ICEConfig
	if err := proto.Decode(b, &ready); err != nil {
		return err
	}
	if ready.Type != "ready" || ready.TTL != 3600 || len(ready.ICE) != 2 || len(ready.ICE[1].URLs) != 2 {
		return fmt.Errorf("invalid ready ICE configuration")
	}
	cred := ready.ICE[1]
	addr := strings.TrimSuffix(strings.TrimPrefix(cred.URLs[0], "turn:"), "?transport=udp")
	port := w.tcp[strings.LastIndex(w.tcp, ":"):]
	if cred.URLs[1] != "turns:turn.localhost"+port+"?transport=tcp" {
		return fmt.Errorf("unexpected TLS endpoint %s", cred.URLs[1])
	}
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
	secure, err := tls.Dial("tcp", w.tcp, &tls.Config{ServerName: "turn.localhost", RootCAs: w.roots})
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

func (w *relayWorld) transfer(out, in int) error {
	for i, a := range w.allocs {
		for j := range out {
			if err := relayPacket(a, w.peer, bytes.Repeat([]byte{byte(i + j)}, 1024), false); err != nil {
				return err
			}
		}
		for j := range in {
			if err := relayPacket(w.peer, a, bytes.Repeat([]byte{byte(i + j + 1)}, 1024), false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *relayWorld) cutOff() error {
	for _, a := range w.allocs {
		if err := relayPacket(a, w.peer, []byte("!"), true); err != nil {
			return err
		}
		if err := relayPacket(w.peer, a, []byte("?"), true); err != nil {
			return err
		}
	}
	return nil
}
