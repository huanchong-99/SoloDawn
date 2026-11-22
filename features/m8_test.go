//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["m8"] = m8Steps
}

type m8InstallWorld struct {
	f           fixtures
	cleanup     func(func())
	ctx         context.Context
	home        string
	dlDir       string
	asset       string
	image       []byte
	binPath     string
	sockPath    string
	env         []string
	rvURL       string
	browser     *peerBrowser
	nodeID      string
	fingerprint string
}

type m8PeerEntry struct {
	name string
	pn   *nodetest.PeerNode
	mu   sync.Mutex
	inb  *peer.Connection
}

func (e *m8PeerEntry) setInbound(c *peer.Connection) {
	e.mu.Lock()
	e.inb = c
	e.mu.Unlock()
}

func (e *m8PeerEntry) inbound() *peer.Connection {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.inb
}

type m8RVWorld struct {
	f          fixtures
	cleanup    func(func())
	ctx        context.Context
	rvURL      string
	relayICE   []proto.ICEServer
	blocked    atomic.Bool
	browser    *peerBrowser
	logEntries []proto.Envelope
	nodes      []*m8PeerEntry
	roomFP     string
	statusFP   string
	nodeConns  map[string]*peer.Connection
}

type m8CLIResult struct {
	stdout string
	stderr string
	err    error
}

type m8NodePairOut struct {
	res node.PairResult
	err error
}

func m8Steps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	iw := &m8InstallWorld{f: f, cleanup: cleanup}
	rw := &m8RVWorld{f: f, cleanup: cleanup, nodeConns: map[string]*peer.Connection{}}

	sc.Step(`^a clean user home with release "([^"]+)" and a local rendezvous$`, iw.setupCleanHome)
	sc.Step(`^the install script rejects a mismatching SHA256SUMS and installs "([^"]+)" once SHA256SUMS matches$`, iw.verifyAndInstall)
	sc.Step(`^a fresh browser pairs with the installed node via "solodawn pair" as "([^"]+)"$`, iw.pairInstalledNode)
	sc.Step(`^the current user has a login service registration and status matches the browser fingerprint$`, iw.checkServiceAndStatus)
	sc.Step(`^the installed node stops and starts again as after a reboot$`, iw.restartInstalledNode)
	sc.Step(`^the restarted node keeps its fingerprint, owner enrolment, and connected status$`, iw.checkPersistedNode)

	sc.Step(`^a fresh server running "solodawn rendezvous" with TLS, STUN, and TURN in one command$`, rw.startOneCommandRV)
	sc.Step(`^a fresh browser and first node "([^"]+)" pair over that rendezvous checking bootstrap refusals$`, rw.pairFirstNodeA11)
	sc.Step(`^the room fingerprint matches "solodawn status" and the first node connects on path "([^"]+)"$`, rw.checkFirstNodeDirect)
	sc.Step(`^a second node "([^"]+)" pairs with the browser over that rendezvous$`, rw.pairSecondNodeA12)
	sc.Step(`^the browser lists both nodes with system, version, and path "([^"]+)"$`, rw.checkBothNodesListed)
	sc.Step(`^UDP and ICE-TCP between the browser and the first node are blocked$`, rw.blockDirectTraffic)
	sc.Step(`^the reconnected path is "([^"]+)" within 1 second and "host\.info" succeeds every second$`, rw.checkRelayFallbackA13)
}

func (w *m8InstallWorld) setupCleanHome(tag string) error {
	if err := w.initDirs(tag); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := checkLoginRegistration(w.home, false); err != nil {
		return err
	}
	registerBothCleanups(w.f.t, w.cleanup, w.removeServiceIfPresent)
	registerBothCleanups(w.f.t, w.cleanup, func() { _ = m8StopExeNodes(w.binPath, w.sockPath) })
	rvURL, err := startAcceptanceRelayRV(w.cleanup)
	w.rvURL = rvURL
	return err
}

func (w *m8InstallWorld) initDirs(tag string) error {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sd8i")
	if err != nil {
		return err
	}
	w.home = filepath.Join(root, "home")
	w.dlDir = filepath.Join(root, "dl")
	for _, sub := range []string{w.home, w.dlDir} {
		if err := os.MkdirAll(sub, 0o700); err != nil {
			return err
		}
	}
	image, err := stampedRelease(w.f, tag)
	if err != nil {
		return err
	}
	w.image, w.asset = image, m8AssetName(tag)
	if err := os.WriteFile(filepath.Join(w.dlDir, w.asset), image, 0o644); err != nil {
		return err
	}
	w.binPath, w.sockPath, w.env = m8ProfileLayout(w.home, w.f.fakeCLIs)
	return nil
}

func (w *m8InstallWorld) removeServiceIfPresent() {
	ok, err := loginRegistration(w.home)
	if err != nil || !ok {
		return
	}
	if runtime.GOOS == "windows" {
		_ = node.RemoveService()
		return
	}
	cmd := exec.Command(w.f.bin, "service", "remove")
	cmd.Env = w.env
	_ = cmd.Run()
}

func (w *m8InstallWorld) verifyAndInstall(tag string) error {
	goodSum := fmt.Sprintf("%x", sha256.Sum256(w.image))
	if err := w.writeSumsAndRunInstall(m8FlipHex(goodSum)); err == nil {
		return errors.New("install script succeeded with mismatching SHA256SUMS")
	}
	if _, err := os.Stat(w.binPath); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("binary installed despite checksum mismatch: %v", err)
	}
	if err := w.writeSumsAndRunInstall(goodSum); err != nil {
		return fmt.Errorf("install with matching SHA256SUMS: %w", err)
	}
	out, err := runNodeCLI(w.binPath, w.env, "version")
	if err != nil || out != tag+"\n" {
		return fmt.Errorf("solodawn version: out=%q err=%v", out, err)
	}
	return nil
}

func m8FlipHex(sum string) string {
	if strings.HasPrefix(sum, "0") {
		return "f" + sum[1:]
	}
	return "0" + sum[1:]
}

func (w *m8InstallWorld) writeSumsAndRunInstall(hexDigest string) error {
	sums := fmt.Sprintf("%s  %s\n", hexDigest, w.asset)
	if err := os.WriteFile(filepath.Join(w.dlDir, "SHA256SUMS"), []byte(sums), 0o644); err != nil {
		return err
	}
	cmd, err := m8InstallCommand(w.asset)
	if err != nil {
		return err
	}
	cmd.Dir, cmd.Env = w.dlDir, w.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

func (w *m8InstallWorld) pairInstalledNode(label string) error {
	b, err := newPeerBrowser(w.ctx, w.cleanup, exampleKey("m8-install-browser"), w.rvURL, nil)
	if err != nil {
		return err
	}
	w.browser = b
	secret := identity.NewSecret()
	done := w.startPairCLI(secret.String(), label)
	roomMsg, err := b.exchangeRoom(w.ctx, secret, "a", "b")
	if err != nil {
		return err
	}
	if err := w.completeBrowserAdopt(b, roomMsg, label); err != nil {
		return err
	}
	return w.awaitPairCLI(done, label)
}

func (w *m8InstallWorld) startPairCLI(secret, label string) <-chan m8CLIResult {
	pairCtx, cancel := context.WithCancel(w.ctx)
	// Single reader: awaitPairCLI (or waitDone on early failure).
	done := make(chan m8CLIResult, 1)
	var settled sync.WaitGroup
	settled.Add(1)
	go func() {
		defer settled.Done()
		out, stderr, err := m8ExecCLIContext(pairCtx, w.binPath, w.env, "pair", secret, "--name", label, "--rendezvous", w.rvURL)
		done <- m8CLIResult{stdout: out, stderr: stderr, err: err}
	}()
	registerBothCleanups(w.f.t, w.cleanup, func() {
		cancel()
		settled.Wait()
	})
	return done
}

func (w *m8InstallWorld) completeBrowserAdopt(b *peerBrowser, roomMsg proto.Pair, label string) error {
	fp, err := deviceFingerprint(roomMsg.Key)
	if err != nil {
		return err
	}
	w.nodeID, w.fingerprint = roomMsg.Key, fp
	bc, err := b.openCtl(w.ctx, w.cleanup, w.nodeID)
	if err != nil {
		return err
	}
	if _, err := requestBrowserHello(w.ctx, bc.conn); err != nil {
		return err
	}
	ts := time.Now().UnixMilli()
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(b.key, b.key.ID(), 1, "browser", "Owner Browser", b.key.ID(), ts),
		nodetest.SignDeviceAdd(b.key, b.key.ID(), 2, "node", label, w.nodeID, ts),
	}
	if err := bc.conn.Send(proto.Devlog{Type: "devlog", User: b.key.ID(), Entries: entries}); err != nil {
		return err
	}
	return checkOwnerWelcome(w.ctx, bc.conn, w.nodeID)
}

func (w *m8InstallWorld) awaitPairCLI(done <-chan m8CLIResult, label string) error {
	select {
	case res := <-done:
		return w.verifyPairCLIResult(res, label)
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *m8InstallWorld) verifyPairCLIResult(res m8CLIResult, label string) error {
	if res.err != nil || res.stderr != "" {
		return fmt.Errorf("solodawn pair: err=%v stderr=%q stdout=%q", res.err, res.stderr, res.stdout)
	}
	if !strings.Contains(res.stdout, label) || !strings.Contains(res.stdout, w.fingerprint) {
		return fmt.Errorf("solodawn pair output %q missing %q or %q", res.stdout, label, w.fingerprint)
	}
	return nil
}

func (w *m8InstallWorld) checkServiceAndStatus() error {
	if err := checkLoginRegistration(w.home, true); err != nil {
		return err
	}
	return w.assertStatusConnected()
}

func (w *m8InstallWorld) assertStatusConnected() error {
	out, err := runNodeCLI(w.binPath, w.env, "status")
	if err != nil {
		return err
	}
	if !w.statusMatches(out) {
		return fmt.Errorf("unexpected status output: %q", out)
	}
	return nil
}

func (w *m8InstallWorld) statusMatches(out string) bool {
	hasVer := strings.Contains(out, "version v1.0.0\n")
	hasFP := strings.Contains(out, "fingerprint "+w.fingerprint+"\n")
	hasConn := strings.Contains(out, "connection "+w.browser.key.ID()+" ")
	return hasVer && hasFP && hasConn
}

func (w *m8InstallWorld) restartInstalledNode() error {
	if err := m8StopExeNodes(w.binPath, w.sockPath); err != nil {
		return err
	}
	if _, err := runNodeCLI(w.binPath, w.env, "status"); err == nil {
		return errors.New("status succeeded while node was stopped")
	}
	return w.startForegroundNode()
}

func (w *m8InstallWorld) startForegroundNode() error {
	var logBuf syncBuffer
	cmd := exec.Command(w.binPath, "node")
	cmd.Env = w.env
	cmd.Stdout, cmd.Stderr = &logBuf, &logBuf
	if _, err := startManagedCmd(w.f.t, w.cleanup, cmd); err != nil {
		return err
	}
	if err := waitAcceptanceSocket(w.sockPath, 10*time.Second); err != nil {
		return fmt.Errorf("restarted node socket: %w (%s)", err, logBuf.String())
	}
	return nil
}

func (w *m8InstallWorld) checkPersistedNode() error {
	if err := w.browser.waitOnline(w.ctx, w.nodeID); err != nil {
		return err
	}
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, w.nodeID)
	if err != nil {
		return err
	}
	if err := w.verifyReconnectedNode(bc.conn); err != nil {
		return err
	}
	return w.assertStatusConnected()
}

func (w *m8InstallWorld) verifyReconnectedNode(conn *peer.Connection) error {
	if err := checkOwnerWelcome(w.ctx, conn, w.nodeID); err != nil {
		return err
	}
	info, err := requestHostInfo(w.ctx, conn)
	if err != nil {
		return err
	}
	if info.Device != w.nodeID || info.Name != "workstation" || info.Version != "v1.0.0" {
		return fmt.Errorf("restarted host.info = %+v", info)
	}
	return nil
}

func (w *m8RVWorld) startOneCommandRV() error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	relay := &peersWorld{}
	err := relay.relayOnly()
	for _, undo := range relay.cleanup {
		w.cleanup(undo)
	}
	if err != nil {
		return err
	}
	w.relayICE = relay.relay
	return w.setupTLSServer()
}

func (w *m8RVWorld) setupTLSServer() error {
	rvDir, err := shortTempRoot(w.f.t, w.cleanup, "sd8rv")
	if err != nil {
		return err
	}
	roots, err := localCert(rvDir)
	if err != nil {
		return err
	}
	w.installRootCA(roots)
	return w.launchTLSServer(rvDir)
}

func (w *m8RVWorld) installRootCA(roots *x509.CertPool) {
	prev := http.DefaultClient.Transport
	w.cleanup(func() { http.DefaultClient.Transport = prev })
	http.DefaultClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"},
	}
}

func (w *m8RVWorld) launchTLSServer(rvDir string) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	addr := "127.0.0.1:" + port
	rvHome := filepath.Join(rvDir, "home")
	if err := os.MkdirAll(rvHome, 0o700); err != nil {
		return err
	}
	cmd := exec.Command(
		w.f.bin, "rendezvous",
		"--listen", addr,
		"--domain", "localhost",
		"--turn-domain", "turn.localhost",
		"--cert", filepath.Join(rvDir, "cert"),
		"--key", filepath.Join(rvDir, "key"),
		"--udp-port", "0",
	)
	cmd.Env = m8CleanEnv(map[string]string{"HOME": rvHome, "XDG_CACHE_HOME": rvHome, "APPDATA": rvHome, "LOCALAPPDATA": rvHome})
	if err := killManagedCmd(w.f.t, w.cleanup, cmd, nil); err != nil {
		return err
	}
	w.rvURL = "wss://" + addr + "/rv"
	return rvListening(addr)
}

func (w *m8RVWorld) mapRoute(r peer.Route) peer.Route {
	return m8FilterRoute(r, &w.blocked, w.relayICE)
}

func (w *m8RVWorld) pairFirstNodeA11(label string) error {
	b, err := newPeerBrowser(w.ctx, w.cleanup, exampleKey("m8-rv-browser"), w.rvURL, w.mapRoute)
	if err != nil {
		return err
	}
	if err := m8CheckReadyICE(b.path.Route().ICE); err != nil {
		return err
	}
	w.browser = b
	n1, err := w.openFirstNodeWithRealStatus(label)
	if err != nil {
		return err
	}
	return w.runFirstPairFlow(n1, label)
}

func (w *m8RVWorld) openFirstNodeWithRealStatus(label string) (*m8PeerEntry, error) {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sd8n1")
	if err != nil {
		return nil, err
	}
	_, sock, env := m8ProfileLayout(root, w.f.fakeCLIs)
	statusFP, err := w.captureNodeStatusFP(sock, env)
	if err != nil {
		return nil, err
	}
	w.statusFP = statusFP
	return w.newPeerEntry(label, filepath.Dir(sock)), nil
}

func (w *m8RVWorld) captureNodeStatusFP(sock string, env []string) (string, error) {
	var logBuf syncBuffer
	cmd := exec.Command(w.f.bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &logBuf, &logBuf
	stop, err := startManagedCmd(w.f.t, w.cleanup, cmd)
	if err != nil {
		return "", err
	}
	defer stop()
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return "", fmt.Errorf("node socket: %w (%s)", err, logBuf.String())
	}
	out, err := runNodeCLI(w.f.bin, env, "status")
	if err != nil {
		return "", err
	}
	fp := extractFingerprintLine(out)
	if fp == "" {
		return "", fmt.Errorf("status missing fingerprint: %q", out)
	}
	return fp, nil
}

func (w *m8RVWorld) newPeerEntry(name, dir string) *m8PeerEntry {
	entry := &m8PeerEntry{name: name}
	tb := scenarioTB{TB: w.f.t, cleanup: w.cleanup}
	entry.pn = nodetest.StartPeerNode(tb, nodetest.PeerConfig{
		Ctx:        w.ctx,
		Dir:        dir,
		Name:       name,
		Version:    "v1.0.0",
		Rendezvous: w.rvURL,
		MapRoute:   w.mapRoute,
		OnInbound:  entry.setInbound,
	})
	w.nodes = append(w.nodes, entry)
	return entry
}

func (w *m8RVWorld) runFirstPairFlow(n1 *m8PeerEntry, label string) error {
	secret := identity.NewSecret()
	pairDone := startMemPair(w.ctx, n1.pn.Pairer, secret.String(), label)
	roomMsg, err := w.browser.exchangeRoom(w.ctx, secret, "a", "b")
	if err != nil {
		return err
	}
	fp, err := deviceFingerprint(roomMsg.Key)
	if err != nil {
		return err
	}
	w.roomFP = fp
	if err := waitNodeKnown(n1, roomMsg.Key, w.browser.key.ID()); err != nil {
		return err
	}
	return w.exerciseBootstrapAndAdopt(n1, label, pairDone)
}

func (w *m8RVWorld) exerciseBootstrapAndAdopt(n1 *m8PeerEntry, label string, pairDone <-chan m8NodePairOut) error {
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, n1.pn.Key.ID())
	if err != nil {
		return err
	}
	welcome, err := requestBrowserHello(w.ctx, bc.conn)
	if err != nil || welcome.Owner {
		return fmt.Errorf("pre-log welcome = %+v, err = %v", welcome, err)
	}
	if err := m8ExpectForbiddenHostInfo(w.ctx, bc.conn); err != nil {
		return err
	}
	if err := m8ExpectForbiddenCmd(w.ctx, w.browser.key, n1.pn.Key.ID(), bc.conn); err != nil {
		return err
	}
	return w.adoptFirstNode(n1, label, bc.conn, pairDone)
}

func (w *m8RVWorld) adoptFirstNode(n1 *m8PeerEntry, label string, conn *peer.Connection, pairDone <-chan m8NodePairOut) error {
	ts := time.Now().UnixMilli()
	w.logEntries = []proto.Envelope{
		nodetest.SignDeviceAdd(w.browser.key, w.browser.key.ID(), 1, "browser", "Owner Browser", w.browser.key.ID(), ts),
		nodetest.SignDeviceAdd(w.browser.key, w.browser.key.ID(), 2, "node", label, n1.pn.Key.ID(), ts),
	}
	if err := conn.Send(proto.Devlog{Type: "devlog", User: w.browser.key.ID(), Entries: w.logEntries}); err != nil {
		return err
	}
	if err := m8AwaitNodePair(w.ctx, pairDone, w.roomFP); err != nil {
		return err
	}
	if err := checkOwnerWelcome(w.ctx, conn, n1.pn.Key.ID()); err != nil {
		return err
	}
	w.nodeConns[label] = conn
	return nil
}

func (w *m8RVWorld) checkFirstNodeDirect(wantPath string) error {
	if w.roomFP == "" || w.roomFP != w.statusFP {
		return fmt.Errorf("room fingerprint %q does not match status fingerprint %q", w.roomFP, w.statusFP)
	}
	n1 := w.nodes[0]
	conn := w.nodeConns[n1.name]
	if err := m8CheckBothPaths(conn, n1.inbound(), wantPath); err != nil {
		return err
	}
	info, err := requestHostInfo(w.ctx, conn)
	if err != nil || info.Device != n1.pn.Key.ID() {
		return fmt.Errorf("first node host.info = %+v, err = %v", info, err)
	}
	return nil
}

func (w *m8RVWorld) pairSecondNodeA12(label string) error {
	dir, err := shortTempRoot(w.f.t, w.cleanup, "sd8n2")
	if err != nil {
		return err
	}
	n2 := w.newPeerEntry(label, dir)
	secret := identity.NewSecret()
	pairDone := startMemPair(w.ctx, n2.pn.Pairer, secret.String(), label)
	if _, err := w.browser.exchangeRoom(w.ctx, secret, "a", "b"); err != nil {
		return err
	}
	if err := waitNodeKnown(n2, n2.pn.Key.ID(), w.browser.key.ID()); err != nil {
		return err
	}
	return w.adoptSecondNode(n2, label, pairDone)
}

func (w *m8RVWorld) adoptSecondNode(n2 *m8PeerEntry, label string, pairDone <-chan m8NodePairOut) error {
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, n2.pn.Key.ID())
	if err != nil {
		return err
	}
	if err := w.sendSecondNodeLogs(bc.conn, n2, label); err != nil {
		return err
	}
	if err := m8AwaitNodePair(w.ctx, pairDone, n2.pn.Key.Fingerprint()); err != nil {
		return err
	}
	if err := checkOwnerWelcome(w.ctx, bc.conn, n2.pn.Key.ID()); err != nil {
		return err
	}
	w.nodeConns[label] = bc.conn
	return nil
}

func (w *m8RVWorld) sendSecondNodeLogs(conn *peer.Connection, n2 *m8PeerEntry, label string) error {
	if _, err := requestBrowserHello(w.ctx, conn); err != nil {
		return err
	}
	addN2 := nodetest.SignDeviceAdd(w.browser.key, w.browser.key.ID(), 3, "node", label, n2.pn.Key.ID(), time.Now().UnixMilli())
	w.logEntries = append(w.logEntries, addN2)
	if err := conn.Send(proto.Devlog{Type: "devlog", User: w.browser.key.ID(), Entries: w.logEntries}); err != nil {
		return err
	}
	firstConn := w.nodeConns[w.nodes[0].name]
	return firstConn.Send(proto.Devlog{Type: "devlog", User: w.browser.key.ID(), Entries: []proto.Envelope{addN2}})
}

func (w *m8RVWorld) checkBothNodesListed(wantPath string) error {
	if len(w.nodes) != 2 {
		return fmt.Errorf("got %d nodes, want 2", len(w.nodes))
	}
	for _, n := range w.nodes {
		if err := w.verifyNodeEntry(n, wantPath); err != nil {
			return err
		}
	}
	if w.nodes[0].pn.Key.ID() == w.nodes[1].pn.Key.ID() {
		return errors.New("both nodes share the same device id")
	}
	return nil
}

func (w *m8RVWorld) verifyNodeEntry(n *m8PeerEntry, wantPath string) error {
	conn := w.nodeConns[n.name]
	if err := m8CheckBothPaths(conn, n.inbound(), wantPath); err != nil {
		return err
	}
	info, err := requestHostInfo(w.ctx, conn)
	if err != nil {
		return err
	}
	wantOS := runtime.GOOS + "/" + runtime.GOARCH
	if info.Name != n.name || info.OS != wantOS || info.Version != "v1.0.0" {
		return fmt.Errorf("node %s host.info = %+v", n.name, info)
	}
	return nil
}

func (w *m8RVWorld) blockDirectTraffic() error {
	w.blocked.Store(true)
	if old := w.nodeConns[w.nodes[0].name]; old != nil {
		old.Close()
	}
	return nil
}

func (w *m8RVWorld) checkRelayFallbackA13(wantPath string) error {
	n1 := w.nodes[0]
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, n1.pn.Key.ID())
	if err != nil {
		return err
	}
	w.nodeConns[n1.name] = bc.conn
	if err := m8CheckBothPaths(bc.conn, n1.inbound(), wantPath); err != nil {
		return err
	}
	if err := checkOwnerWelcome(w.ctx, bc.conn, n1.pn.Key.ID()); err != nil {
		return err
	}
	return w.pollHostInfoEverySecond(bc.conn, n1.pn.Key.ID(), 3)
}

func (w *m8RVWorld) pollHostInfoEverySecond(conn *peer.Connection, wantDevice string, count int) error {
	for i := range count {
		if i > 0 {
			time.Sleep(time.Second)
		}
		info, err := requestHostInfo(w.ctx, conn)
		if err != nil || info.Device != wantDevice {
			return fmt.Errorf("tick %d host.info = %+v, err = %v", i, info, err)
		}
	}
	return nil
}

func startMemPair(ctx context.Context, p *node.Pairer, secret, label string) <-chan m8NodePairOut {
	// Single reader: m8AwaitNodePair.
	done := make(chan m8NodePairOut, 1)
	go func() {
		res, err := p.Pair(ctx, node.PairRequest{Secret: secret, Name: label})
		done <- m8NodePairOut{res: res, err: err}
	}()
	return done
}

func waitNodeKnown(n *m8PeerEntry, expectedNodeID, browserID string) error {
	if expectedNodeID != n.pn.Key.ID() {
		return fmt.Errorf("room node key = %q, want %q", expectedNodeID, n.pn.Key.ID())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if n.pn.Pairer.Known(browserID) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for node %s to record pairing peer", n.name)
}

func m8AwaitNodePair(ctx context.Context, done <-chan m8NodePairOut, wantFP string) error {
	select {
	case out := <-done:
		if out.err != nil {
			return out.err
		}
		if out.res.Fingerprint != wantFP || out.res.Path == "" {
			return fmt.Errorf("pair result = %+v, want fingerprint %q", out.res, wantFP)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func m8CheckReadyICE(servers []proto.ICEServer) error {
	raw, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	s := string(raw)
	for _, want := range []string{"stun:localhost:", "turn:localhost:", "turns:turn.localhost:"} {
		if !strings.Contains(s, want) {
			return fmt.Errorf("ready ICE %s missing %q", s, want)
		}
	}
	return nil
}

func m8FilterRoute(r peer.Route, blocked *atomic.Bool, relayICE []proto.ICEServer) peer.Route {
	if blocked.Load() {
		r.ICE = relayICE
	} else {
		r.ICE = m8RewriteRVICE(r.ICE)
	}
	orig := r.Send
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		if blocked.Load() && isNonRelayICECandidate(env.Payload) {
			return nil
		}
		return orig(ctx, to, env)
	}
	return r
}

func m8RewriteRVICE(servers []proto.ICEServer) []proto.ICEServer {
	var out []proto.ICEServer
	for _, s := range servers {
		if urls := m8DirectICEURLs(s.URLs); len(urls) > 0 {
			cp := s
			cp.URLs = urls
			out = append(out, cp)
		}
	}
	return out
}

func m8DirectICEURLs(urls []string) []string {
	var out []string
	for _, u := range urls {
		if strings.HasPrefix(u, "stun:") {
			out = append(out, strings.Replace(u, "localhost:", "127.0.0.1:", 1))
		}
	}
	return out
}

func m8ExpectForbiddenHostInfo(ctx context.Context, conn *peer.Connection) error {
	raw, err := conn.Request(ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return err
	}
	return checkWireError(raw, "forbidden")
}

func m8ExpectForbiddenCmd(ctx context.Context, signer identity.Key, nodeID string, conn *peer.Connection) error {
	payload := fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, exampleID("m8-pre"), time.Now().UnixMilli(), nodeID)
	env := signer.Sign(payload)
	raw, err := conn.Request(ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	return checkWireError(raw, "forbidden")
}

func m8CheckBothPaths(browserConn, nodeConn *peer.Connection, want string) error {
	if err := waitConnPath(browserConn, want, time.Second); err != nil {
		return fmt.Errorf("browser path: %w", err)
	}
	if err := waitConnPath(nodeConn, want, time.Second); err != nil {
		return fmt.Errorf("node path: %w", err)
	}
	return nil
}

func m8AssetName(tag string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return "solodawn_" + tag + "_" + runtime.GOOS + "_" + runtime.GOARCH + ext
}

func m8ProfileLayout(home, fakeCLIs string) (binPath, sockPath string, env []string) {
	dataHome := filepath.Join(home, "data")
	appData := filepath.Join(home, "appdata")
	binDir, binName, sockDir := m8PlatformDirs(home, dataHome, appData)
	pathVal := strings.Join([]string{binDir, fakeCLIs, os.Getenv("PATH")}, string(os.PathListSeparator))
	env = m8CleanEnv(map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CONFIG_HOME": home,
		"XDG_DATA_HOME":   dataHome,
		"APPDATA":         appData,
		"LOCALAPPDATA":    appData,
		"PATH":            pathVal,
	})
	return filepath.Join(binDir, binName), filepath.Join(sockDir, "sock"), env
}

func m8PlatformDirs(home, dataHome, appData string) (binDir, binName, sockDir string) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(appData, `Programs\SoloDawn`), "solodawn.exe", filepath.Join(appData, "solodawn")
	case "darwin":
		return filepath.Join(home, ".local", "bin"), "solodawn", filepath.Join(home, "Library", "Application Support", "solodawn")
	default:
		return filepath.Join(home, ".local", "bin"), "solodawn", filepath.Join(dataHome, "solodawn")
	}
}

func m8CleanEnv(overrides map[string]string) []string {
	var out []string
	for _, entry := range os.Environ() {
		k, _, _ := strings.Cut(entry, "=")
		if !m8HasKeyFold(overrides, k) {
			out = append(out, entry)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(overrides)) {
		out = append(out, k+"="+overrides[k])
	}
	return out
}

func m8HasKeyFold(m map[string]string, target string) bool {
	for k := range m {
		if strings.EqualFold(k, target) {
			return true
		}
	}
	return false
}

func m8InstallCommand(asset string) (*exec.Cmd, error) {
	raw, err := os.ReadFile(filepath.Join("..", "install.md"))
	if err != nil {
		return nil, err
	}
	doc := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if runtime.GOOS == "windows" {
		script, err := m8LoadFencedInstall(doc, "powershell", "REPLACE_WITH_BINARY_FILENAME.exe", asset)
		if err != nil {
			return nil, err
		}
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script), nil
	}
	script, err := m8LoadFencedInstall(doc, "sh", "REPLACE_WITH_BINARY_FILENAME", asset)
	if err != nil {
		return nil, err
	}
	return exec.Command("sh", "-c", script), nil
}

func m8LoadFencedInstall(doc, lang, placeholder, asset string) (string, error) {
	fence := "```" + lang + "\n"
	_, rest, ok := strings.Cut(doc, fence)
	if !ok {
		return "", fmt.Errorf("install.md missing %s block", lang)
	}
	block, _, ok := strings.Cut(rest, "\n```")
	if !ok || !strings.Contains(block, placeholder) {
		return "", fmt.Errorf("install.md %s block missing %s", lang, placeholder)
	}
	return strings.Replace(block, placeholder, asset, 1) + "\n", nil
}

func m8ExecCLIContext(ctx context.Context, bin string, env []string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func m8StopExeNodes(exe, sock string) error {
	if runtime.GOOS == "windows" {
		return m8StopExeWindows(exe, sock)
	}
	return m8StopExeUnix(exe, sock)
}

func m8StopExeWindows(exe, sock string) error {
	script := `Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq $env:SD_STOP_EXE } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "SD_STOP_EXE="+exe)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stop %s: %w (%s)", exe, err, bytes.TrimSpace(out))
	}
	return m8WaitSocketGone(sock, 5*time.Second)
}

func m8StopExeUnix(exe, sock string) error {
	m8SignalUnixPIDs(m8FindUnixPIDs(exe), os.Interrupt)
	if m8WaitUnixStopped(exe, sock, 3*time.Second) {
		return nil
	}
	m8SignalUnixPIDs(m8FindUnixPIDs(exe), os.Kill)
	if m8WaitUnixStopped(exe, sock, 2*time.Second) {
		return nil
	}
	return fmt.Errorf("processes for %s did not stop", exe)
}

func m8SignalUnixPIDs(pids []int, sig os.Signal) {
	for _, pid := range pids {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(sig)
		}
	}
}

func m8WaitUnixStopped(exe, sock string, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if len(m8FindUnixPIDs(exe)) == 0 && !m8CanDialUnix(sock) {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

func m8WaitSocketGone(sock string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if !m8CanDialUnix(sock) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("socket %s stayed reachable", sock)
}

func m8CanDialUnix(sock string) bool {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func m8FindUnixPIDs(exe string) []int {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,stat=,command=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		if pid, ok := m8MatchLivePID(line, exe); ok {
			pids = append(pids, pid)
		}
	}
	return pids
}

func m8MatchLivePID(line, exe string) (int, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || strings.HasPrefix(fields[1], "Z") {
		return 0, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || fields[2] != exe {
		return 0, false
	}
	return pid, true
}
