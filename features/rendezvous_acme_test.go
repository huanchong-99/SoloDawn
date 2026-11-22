//go:build acceptance

package features

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/cucumber/godog"
)

// pebbleVersion is the Pebble release that spike S-6 measured.
const pebbleVersion = "v2.10.3"

// acmeWorld is one scenario's ACME test server and the rendezvous that uses
// it. Everything it starts listens on 127.0.0.1 and everything it writes is
// in dir, which the scenario removes.
type acmeWorld struct {
	*relayWorld
	dir      string
	acme     string // Pebble's ACME port
	manage   string // Pebble's management port
	lifetime time.Duration
	issuer   *x509.CertPool
	first    map[string]*x509.Certificate // the certificate each name presented first
}

func init() {
	stepSets["rendezvous_acme"] = func(sc *godog.ScenarioContext, _ fixtures) {
		acmeSteps(sc)
	}
}

// The ACME steps drive the browser and the rendezvous of the rendezvous steps.
func acmeSteps(sc *godog.ScenarioContext) {
	w := &acmeWorld{first: map[string]*x509.Certificate{}}
	sc.Step(`^an ACME test server whose certificates last (\d+) seconds$`, func(ctx context.Context, seconds int) error {
		w.relayWorld = &relayWorld{rvWorld: ctx.Value(rvKey{}).(*rvWorld)}
		return w.startPebble(seconds)
	})
	sc.Step(`^the command "solodawn rendezvous" starts for "([^"]+)" with that ACME server$`, w.startRendezvous)
	sc.Step(`^"([^"]+)" and "([^"]+)" present certificates that the ACME test server issued$`, w.issued)
	sc.Step(`^HTTPS on the shared port answers under the certificate of "([^"]+)"$`, w.https)
	sc.Step(`^the browser signs the challenge and relays data through TURN over TLS$`, w.relayData)
	sc.Step(`^two thirds of the certificates' lifetime have passed$`, w.twoThirds)
	sc.Step(`^both names present new certificates from the ACME test server before the first ones expire$`, w.renewed)
}

func freePort() (string, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	return port, err
}

func freePorts(n int) ([]string, error) {
	ports := make([]string, n)
	for i := range ports {
		port, err := freePort()
		if err != nil {
			return nil, err
		}
		ports[i] = port
	}
	return ports, nil
}

// start runs a test server until the scenario ends, its output in a log file.
func (w *acmeWorld) start(name string, env []string, args ...string) error {
	log, err := os.Create(filepath.Join(w.dir, name+".log"))
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(w.dir, "bin", name), args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = append(os.Environ(), env...), log, log
	if err := killManagedCmd(w.t, w.addCleanup, cmd, func() { _ = log.Close() }); err != nil {
		_ = log.Close()
		return err
	}
	return nil
}

// startPebble installs Pebble and its DNS server into the scenario's directory
// and starts them. The DNS server answers every name with 127.0.0.1, so Pebble
// checks every name on the rendezvous port chosen here.
func (w *acmeWorld) startPebble(seconds int) error {
	dir, err := os.MkdirTemp("", "solodawn-acme")
	if err != nil {
		return err
	}
	w.dir, w.lifetime = dir, time.Duration(seconds-1)*time.Second // Pebble's notAfter is notBefore + validity - 1 s
	w.cleanup = append(w.cleanup, func() { os.RemoveAll(dir) })
	install := exec.Command("go", "install", "github.com/letsencrypt/pebble/v2/cmd/pebble@"+pebbleVersion, "github.com/letsencrypt/pebble/v2/cmd/pebble-challtestsrv@"+pebbleVersion)
	install.Env = append(os.Environ(), "GOBIN="+filepath.Join(dir, "bin"))
	if out, err := install.CombinedOutput(); err != nil {
		return fmt.Errorf("install Pebble: %v\n%s", err, out)
	}
	// Pebble's own HTTPS uses a certificate for localhost, which is also the
	// root that --acme-ca names.
	roots, err := localCert(dir)
	if err != nil {
		return err
	}
	ports, err := freePorts(6)
	if err != nil {
		return err
	}
	w.acme, w.manage, w.tcp = ports[0], ports[1], "127.0.0.1:"+ports[2]
	dns := "127.0.0.1:" + ports[3]
	tlsPort, _ := strconv.Atoi(ports[2])
	httpPort, _ := strconv.Atoi(ports[4])
	config, err := json.Marshal(map[string]any{"pebble": map[string]any{
		"listenAddress":           "127.0.0.1:" + w.acme,
		"managementListenAddress": "127.0.0.1:" + w.manage,
		"certificate":             filepath.Join(dir, "cert"),
		"privateKey":              filepath.Join(dir, "key"),
		"httpPort":                httpPort,
		"tlsPort":                 tlsPort,
		"keyAlgorithm":            "ecdsa",
		"profiles":                map[string]any{"short": map[string]any{"description": "renewal test", "validityPeriod": seconds}},
	}})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "pebble.json"), config, 0600); err != nil {
		return err
	}
	if err := w.start("pebble-challtestsrv", nil, "-management", "127.0.0.1:"+ports[5], "-dnsserver", dns, "-http01", "", "-https01", "", "-tlsalpn01", "", "-doh", "", "-defaultIPv4", "127.0.0.1", "-defaultIPv6", ""); err != nil {
		return err
	}
	if err := w.start("pebble", []string{"PEBBLE_VA_NOSLEEP=1", "PEBBLE_WFE_NONCEREJECT=0", "PEBBLE_AUTHZREUSE=0"}, "-config", filepath.Join(dir, "pebble.json"), "-dnsserver", dns); err != nil {
		return err
	}
	return w.loadIssuer(roots)
}

// loadIssuer waits for Pebble and reads the root it issues under.
func (w *acmeWorld) loadIssuer(roots *x509.CertPool) error {
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	var body []byte
	for range 100 {
		resp, err := client.Get("https://localhost:" + w.manage + "/roots/0")
		if err == nil {
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil && resp.StatusCode == http.StatusOK {
				w.issuer = x509.NewCertPool()
				if !w.issuer.AppendCertsFromPEM(body) {
					return fmt.Errorf("Pebble's root: %s", body)
				}
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Pebble did not start: %s", w.log("pebble"))
}

func (w *acmeWorld) log(name string) []byte {
	b, _ := os.ReadFile(filepath.Join(w.dir, name+".log"))
	return b[max(0, len(b)-4096):]
}

// startRendezvous starts the real binary with one command. Its HOME, and with
// it the certificate cache, is in the scenario's directory.
func (w *acmeWorld) startRendezvous(domain string) error {
	w.domain, w.roots = domain, w.issuer
	home := filepath.Join(w.dir, "home")
	cmd := exec.Command(w.bin, "rendezvous", "--listen", w.tcp, "--domain", domain, "--turn-domain", "turn."+domain, "--udp-port", "0",
		"--acme-directory", "https://localhost:"+w.acme+"/dir", "--acme-ca", filepath.Join(w.dir, "cert"))
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CACHE_HOME="+home, "LocalAppData="+home)
	stderr, err := os.Create(filepath.Join(w.dir, "rendezvous.log"))
	if err != nil {
		return err
	}
	cmd.Stderr = stderr
	if err := killManagedCmd(w.t, w.addCleanup, cmd, func() { _ = stderr.Close() }); err != nil {
		_ = stderr.Close()
		return err
	}
	w.httpClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: w.issuer, ServerName: domain}}}
	w.ws = "wss://" + w.tcp + "/rv"
	if err := rvListening(w.tcp); err != nil {
		ips, lookup := net.LookupIP(domain)
		return fmt.Errorf("%v; the binary wrote %q; %s resolves to %v, %v", err, w.log("rendezvous"), domain, ips, lookup)
	}
	return nil
}

// leaf is the certificate that a name presents on the shared port, checked
// against the ACME test server's root. The first handshake for a name waits
// while the rendezvous obtains its certificate.
func (w *acmeWorld) leaf(name string) (*x509.Certificate, error) {
	c, err := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", w.tcp, &tls.Config{ServerName: name, RootCAs: w.issuer})
	if err != nil {
		return nil, fmt.Errorf("%s: %v\n%s", name, err, w.log("pebble"))
	}
	defer c.Close()
	return c.ConnectionState().PeerCertificates[0], nil
}

func (w *acmeWorld) issued(domain, turn string) error {
	for _, name := range []string{domain, turn} {
		cert, err := w.leaf(name)
		if err != nil {
			return err
		}
		if !slices.Equal(cert.DNSNames, []string{name}) || cert.NotAfter.Sub(cert.NotBefore) != w.lifetime {
			return fmt.Errorf("%s: certificate for %v valid %v", name, cert.DNSNames, cert.NotAfter.Sub(cert.NotBefore))
		}
		w.first[name] = cert
	}
	return nil
}

func (w *acmeWorld) https(name string) error {
	resp, err := w.httpClient.Get("https://" + w.tcp + "/")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if serial := resp.TLS.PeerCertificates[0].SerialNumber; resp.StatusCode != http.StatusOK || serial.Cmp(w.first[name].SerialNumber) != 0 {
		return fmt.Errorf("HTTPS %d under serial %x", resp.StatusCode, serial)
	}
	return nil
}

func (w *acmeWorld) relayData() error {
	w.allocs = nil
	if err := w.allocateBoth(); err != nil {
		return err
	}
	return w.transfer(4, 1024)
}

// twoThirds waits until a third of the lifetime of every first certificate is
// left: the rendezvous renews in the tenth of its lifetime that follows.
func (w *acmeWorld) twoThirds() error {
	for _, cert := range w.first {
		time.Sleep(time.Until(cert.NotBefore.Add(w.lifetime * 2 / 3)))
	}
	return nil
}

func (w *acmeWorld) renewed() error {
	for name, first := range w.first {
		for {
			cert, err := w.leaf(name)
			if err != nil {
				return err
			}
			if cert.SerialNumber.Cmp(first.SerialNumber) != 0 && cert.NotBefore.After(first.NotBefore) && slices.Equal(cert.DNSNames, []string{name}) {
				break
			}
			if time.Now().After(first.NotAfter) {
				return fmt.Errorf("%s: not renewed before %v\n%s", name, first.NotAfter, w.log("pebble"))
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
	return nil
}
