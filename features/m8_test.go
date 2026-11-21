//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
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
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
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
	browser     *m8Browser
	nodeID      string
	fingerprint string
}

type m8RVWorld struct {
	f          fixtures
	cleanup    func(func())
	ctx        context.Context
	rvURL      string
	relayICE   []proto.ICEServer
	blocked    atomic.Bool
	browser    *m8Browser
	logEntries []proto.Envelope
	nodes      []*m8MemNode
	roomFP     string
	statusFP   string
	nodeConns  map[string]*peer.Connection
}

type m8Browser struct {
	key        identity.Key
	path       *peer.Path
	peer       *peer.Peer
	presence   chan string      // single reader: waitOnline
	roomEvents chan m8RoomEvent // single reader: exchangeRoom
}

type m8RoomEvent struct {
	kind string
	data []byte
}

type m8MemNode struct {
	name     string
	version  string
	key      identity.Key
	db       *store.Store
	logs     *node.Logs
	sessions *node.Sessions
	pairer   *node.Pairer
	peer     *peer.Peer
	path     *peer.Path

	mu      sync.Mutex
	loop    *home.Loop
	local   *node.LocalLink
	inbound *peer.Connection
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := w.initDirs(tag); err != nil {
		return err
	}
	if err := checkLoginRegistration(w.home, false); err != nil {
		return err
	}
	rvURL, err := startAcceptanceRelayRV(w.cleanup)
	w.rvURL = rvURL
	return err
}

func (w *m8InstallWorld) initDirs(tag string) error {
	root, err := m8MakeDirs("sd8i")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(root) })
	w.home = filepath.Join(root, "home")
	w.dlDir = filepath.Join(root, "dl")
	image, err := stampedRelease(w.f, tag)
	if err != nil {
		return err
	}
	w.image, w.asset = image, m8AssetName(tag)
	if err := os.WriteFile(filepath.Join(w.dlDir, w.asset), image, 0o644); err != nil {
		return err
	}
	w.binPath, w.sockPath, w.env = m8ProfileLayout(w.home, w.f.fakeCLIs)
	w.cleanup(w.removeServiceIfPresent)
	w.cleanup(func() { m8StopExeNodes(w.binPath, w.sockPath) })
	return nil
}

func m8MakeDirs(prefix string) (string, error) {
	root, err := m8TempRoot(prefix)
	if err != nil {
		return "", err
	}
	for _, sub := range []string{"home", "dl"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o700); err != nil {
			os.RemoveAll(root)
			return "", err
		}
	}
	return root, nil
}

func (w *m8InstallWorld) removeServiceIfPresent() {
	ok, err := loginRegistration(w.home)
	if err == nil && ok {
		cmd := exec.Command(w.f.bin, "service", "remove")
		cmd.Env = w.env
		cmd.Run()
	}
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
	cmd := m8InstallCommand(w.asset)
	cmd.Dir, cmd.Env = w.dlDir, w.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, bytes.TrimSpace(out))
	}
	return nil
}

func (w *m8InstallWorld) pairInstalledNode(label string) error {
	if _, err := runNodeCLI(w.binPath, w.env, "status"); err == nil {
		return errors.New("status succeeded before node was started")
	}
	b, err := newM8Browser(w.ctx, w.cleanup, exampleKey("m8-install-browser"), w.rvURL, nil, nil)
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
	// Single reader: awaitPairCLI.
	done := make(chan m8CLIResult, 1)
	go func() {
		out, stderr, err := m8ExecCLI(w.binPath, w.env, "pair", secret, "--name", label, "--rendezvous", w.rvURL)
		done <- m8CLIResult{stdout: out, stderr: stderr, err: err}
	}()
	return done
}

func (w *m8InstallWorld) completeBrowserAdopt(b *m8Browser, roomMsg proto.Pair, label string) error {
	fp, err := m8DeviceFingerprint(roomMsg.Key)
	if err != nil {
		return err
	}
	w.nodeID, w.fingerprint = roomMsg.Key, fp
	conn, err := b.openCtl(w.ctx, w.cleanup, w.nodeID)
	if err != nil {
		return err
	}
	if _, err := requestBrowserHello(w.ctx, conn); err != nil {
		return err
	}
	entries := []proto.Envelope{
		signAcceptanceAdd(b.key, b.key.ID(), 1, b.key.ID(), "browser", "Owner Browser"),
		signAcceptanceAdd(b.key, b.key.ID(), 2, w.nodeID, "node", label),
	}
	if err := conn.Send(proto.Devlog{Type: "devlog", User: b.key.ID(), Entries: entries}); err != nil {
		return err
	}
	return m8CheckOwnerWelcome(w.ctx, conn, w.nodeID)
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
	var logBuf bytes.Buffer
	cmd := exec.Command(w.binPath, "node")
	cmd.Env = w.env
	cmd.Stdout, cmd.Stderr = &logBuf, &logBuf
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cleanup(func() { stopAcceptanceNodeProc(cmd) })
	if err := waitAcceptanceSocket(w.sockPath, 10*time.Second); err != nil {
		return fmt.Errorf("restarted node socket: %w (%s)", err, logBuf.String())
	}
	return nil
}

func (w *m8InstallWorld) checkPersistedNode() error {
	if err := w.checkOwnedPairRefused(); err != nil {
		return err
	}
	if err := w.browser.waitOnline(w.ctx, w.nodeID); err != nil {
		return err
	}
	conn, err := w.browser.openCtl(w.ctx, w.cleanup, w.nodeID)
	if err != nil {
		return err
	}
	if err := w.verifyReconnectedNode(conn); err != nil {
		return err
	}
	return w.assertStatusConnected()
}

func (w *m8InstallWorld) verifyReconnectedNode(conn *peer.Connection) error {
	if err := m8CheckOwnerWelcome(w.ctx, conn, w.nodeID); err != nil {
		return err
	}
	info, err := m8RequestHostInfo(w.ctx, conn)
	if err != nil {
		return err
	}
	if info.Device != w.nodeID || info.Name != "workstation" || info.Version != "v1.0.0" {
		return fmt.Errorf("restarted host.info = %+v", info)
	}
	return nil
}

func (w *m8InstallWorld) checkOwnedPairRefused() error {
	_, stderr, err := m8ExecCLI(w.binPath, w.env, "pair", identity.NewSecret().String(), "--name", "workstation")
	if err == nil || !strings.Contains(stderr, "pair-browser") {
		return fmt.Errorf("owned pair: err=%v stderr=%q", err, stderr)
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
	rvDir, err := m8TempRoot("sd8rv")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(rvDir) })
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
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cleanup(func() { m8KillWait(cmd) })
	w.rvURL = "wss://" + addr + "/rv"
	return rvListening(addr)
}

func (w *m8RVWorld) pairFirstNodeA11(label string) error {
	b, err := newM8Browser(w.ctx, w.cleanup, exampleKey("m8-rv-browser"), w.rvURL, &w.blocked, w.relayICE)
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

func (w *m8RVWorld) openFirstNodeWithRealStatus(label string) (*m8MemNode, error) {
	root, err := m8TempRoot("sd8n1")
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { os.RemoveAll(root) })
	_, sock, env := m8ProfileLayout(root, w.f.fakeCLIs)
	statusFP, err := m8CaptureNodeStatusFP(w.f.bin, sock, env)
	if err != nil {
		return nil, err
	}
	w.statusFP = statusFP
	return w.newMemNode(label, "v1.0.0", filepath.Dir(sock))
}

func m8CaptureNodeStatusFP(bin, sock string, env []string) (string, error) {
	var logBuf bytes.Buffer
	cmd := exec.Command(bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &logBuf, &logBuf
	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer stopAcceptanceNodeProc(cmd)
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return "", fmt.Errorf("node socket: %w (%s)", err, logBuf.String())
	}
	out, err := runNodeCLI(bin, env, "status")
	if err != nil {
		return "", err
	}
	fp := extractFingerprintLine(out)
	if fp == "" {
		return "", fmt.Errorf("status missing fingerprint: %q", out)
	}
	return fp, nil
}

func (w *m8RVWorld) runFirstPairFlow(n1 *m8MemNode, label string) error {
	secret := identity.NewSecret()
	pairDone := n1.startPair(w.ctx, secret.String(), label)
	roomMsg, err := w.browser.exchangeRoom(w.ctx, secret, "a", "b")
	if err != nil {
		return err
	}
	fp, err := m8DeviceFingerprint(roomMsg.Key)
	if err != nil {
		return err
	}
	w.roomFP = fp
	if err := n1.waitKnown(roomMsg.Key, w.browser.key.ID()); err != nil {
		return err
	}
	return w.exerciseBootstrapAndAdopt(n1, label, pairDone)
}

func (w *m8RVWorld) exerciseBootstrapAndAdopt(n1 *m8MemNode, label string, pairDone <-chan m8NodePairOut) error {
	conn, err := w.browser.openCtl(w.ctx, w.cleanup, n1.key.ID())
	if err != nil {
		return err
	}
	welcome, err := requestBrowserHello(w.ctx, conn)
	if err != nil || welcome.Owner {
		return fmt.Errorf("pre-log welcome = %+v, err = %v", welcome, err)
	}
	if err := m8ExpectForbiddenHostInfo(w.ctx, conn); err != nil {
		return err
	}
	if err := m8ExpectForbiddenCmd(w.ctx, w.browser.key, n1.key.ID(), conn); err != nil {
		return err
	}
	return w.adoptFirstNode(n1, label, conn, pairDone)
}

func (w *m8RVWorld) adoptFirstNode(n1 *m8MemNode, label string, conn *peer.Connection, pairDone <-chan m8NodePairOut) error {
	w.logEntries = []proto.Envelope{
		signAcceptanceAdd(w.browser.key, w.browser.key.ID(), 1, w.browser.key.ID(), "browser", "Owner Browser"),
		signAcceptanceAdd(w.browser.key, w.browser.key.ID(), 2, n1.key.ID(), "node", label),
	}
	if err := conn.Send(proto.Devlog{Type: "devlog", User: w.browser.key.ID(), Entries: w.logEntries}); err != nil {
		return err
	}
	if err := m8AwaitNodePair(w.ctx, pairDone, w.roomFP); err != nil {
		return err
	}
	if err := m8CheckOwnerWelcome(w.ctx, conn, n1.key.ID()); err != nil {
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
	if err := m8CheckBothPaths(conn, n1.inboundConn(), wantPath); err != nil {
		return err
	}
	info, err := m8RequestHostInfo(w.ctx, conn)
	if err != nil || info.Device != n1.key.ID() {
		return fmt.Errorf("first node host.info = %+v, err = %v", info, err)
	}
	return nil
}

func (w *m8RVWorld) pairSecondNodeA12(label string) error {
	dir, err := m8TempRoot("sd8n2")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(dir) })
	n2, err := w.newMemNode(label, "v1.0.0", dir)
	if err != nil {
		return err
	}
	secret := identity.NewSecret()
	pairDone := n2.startPair(w.ctx, secret.String(), label)
	if _, err := w.browser.exchangeRoom(w.ctx, secret, "a", "b"); err != nil {
		return err
	}
	if err := n2.waitKnown(n2.key.ID(), w.browser.key.ID()); err != nil {
		return err
	}
	return w.adoptSecondNode(n2, label, pairDone)
}

func (w *m8RVWorld) adoptSecondNode(n2 *m8MemNode, label string, pairDone <-chan m8NodePairOut) error {
	conn, err := w.browser.openCtl(w.ctx, w.cleanup, n2.key.ID())
	if err != nil {
		return err
	}
	if err := w.sendSecondNodeLogs(conn, n2, label); err != nil {
		return err
	}
	if err := m8AwaitNodePair(w.ctx, pairDone, n2.key.Fingerprint()); err != nil {
		return err
	}
	if err := m8CheckOwnerWelcome(w.ctx, conn, n2.key.ID()); err != nil {
		return err
	}
	w.nodeConns[label] = conn
	return nil
}

func (w *m8RVWorld) sendSecondNodeLogs(conn *peer.Connection, n2 *m8MemNode, label string) error {
	if _, err := requestBrowserHello(w.ctx, conn); err != nil {
		return err
	}
	addN2 := signAcceptanceAdd(w.browser.key, w.browser.key.ID(), 3, n2.key.ID(), "node", label)
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
	if w.nodes[0].key.ID() == w.nodes[1].key.ID() {
		return errors.New("both nodes share the same device id")
	}
	return nil
}

func (w *m8RVWorld) verifyNodeEntry(n *m8MemNode, wantPath string) error {
	conn := w.nodeConns[n.name]
	if err := m8CheckBothPaths(conn, n.inboundConn(), wantPath); err != nil {
		return err
	}
	info, err := m8RequestHostInfo(w.ctx, conn)
	if err != nil {
		return err
	}
	wantOS := runtime.GOOS + "/" + runtime.GOARCH
	if info.Name != n.name || info.OS != wantOS || info.Version != n.version {
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
	conn, err := w.browser.openCtl(w.ctx, w.cleanup, n1.key.ID())
	if err != nil {
		return err
	}
	w.nodeConns[n1.name] = conn
	if err := m8CheckBothPaths(conn, n1.inboundConn(), wantPath); err != nil {
		return err
	}
	if err := m8CheckOwnerWelcome(w.ctx, conn, n1.key.ID()); err != nil {
		return err
	}
	return w.pollHostInfoEverySecond(conn, n1.key.ID(), 3)
}

func (w *m8RVWorld) pollHostInfoEverySecond(conn *peer.Connection, wantDevice string, count int) error {
	for i := range count {
		if i > 0 {
			time.Sleep(time.Second)
		}
		info, err := m8RequestHostInfo(w.ctx, conn)
		if err != nil || info.Device != wantDevice {
			return fmt.Errorf("tick %d host.info = %+v, err = %v", i, info, err)
		}
	}
	return nil
}

func (w *m8RVWorld) newMemNode(name, version, dir string) (*m8MemNode, error) {
	key, db, logs, err := m8OpenNodeStore(dir)
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { db.Close() })
	n := &m8MemNode{name: name, version: version, key: key, db: db, logs: logs}
	if err := n.initServices(w.cleanup, w.rvURL); err != nil {
		return nil, err
	}
	if err := n.initNetwork(w.ctx, w.cleanup, w.rvURL, &w.blocked, w.relayICE); err != nil {
		return nil, err
	}
	w.nodes = append(w.nodes, n)
	return n, nil
}

func m8OpenNodeStore(dir string) (identity.Key, *store.Store, *node.Logs, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, nil, err
	}
	key, err := identity.LoadKey(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	db, err := store.Open(filepath.Join(dir, "node.db"))
	if err != nil {
		return nil, nil, nil, err
	}
	logs, err := node.OpenLogs(db, key.ID())
	if err != nil {
		db.Close()
		return nil, nil, nil, err
	}
	return key, db, logs, nil
}

func (n *m8MemNode) initServices(cleanup func(func()), rvURL string) error {
	loop, err := home.StartLoop(home.LoopConfig{
		Store: n.db, Home: n.key.ID(), OwnerFn: n.logs.Owner,
		Logs: n.logs.Log, Tables: home.NodeTables(),
	})
	if err != nil {
		return err
	}
	h, _, err := host.New(n.db, n.key, time.Now)
	if err != nil {
		loop.Close()
		return err
	}
	n.loop, n.local = loop, node.AttachLocal(h, loop)
	cleanup(n.closeHome)
	n.wireSessionSurfaceAndPairer(cleanup, h, rvURL)
	return nil
}

func (n *m8MemNode) wireSessionSurfaceAndPairer(cleanup func(func()), h *host.Host, rvURL string) {
	n.sessions = node.NewSessions(node.SessionConfig{
		ID: n.key.ID(), Version: n.version, Store: n.db, Logs: n.logs,
		Home: n.loop, Host: h,
		Name:  func() string { return node.ReadName(n.db) },
		Addrs: func() []string { return []string{rvURL} },
	})
	node.NewSurface(node.SurfaceConfig{
		ID: n.key.ID(), OS: runtime.GOOS + "/" + runtime.GOARCH, Version: n.version,
		Store: n.db, Sessions: n.sessions, Host: h, Agents: h.Agents,
	})
	n.pairer = node.NewPairer(node.PairConfig{
		Key: n.key, Store: n.db, Logs: n.logs, Sessions: n.sessions,
		Paths:      n.sharedPaths,
		Rendezvous: func() []string { return []string{rvURL} },
	})
	cleanup(n.pairer.Close)
}

func (n *m8MemNode) closeHome() {
	n.mu.Lock()
	local, loop := n.local, n.loop
	n.local, n.loop = nil, nil
	n.mu.Unlock()
	if local != nil {
		local.Close()
	}
	if loop != nil {
		loop.Close()
	}
}

func (n *m8MemNode) sharedPaths() []*peer.Path {
	if n.path != nil {
		return []*peer.Path{n.path}
	}
	return nil
}

func (n *m8MemNode) initNetwork(ctx context.Context, cleanup func(func()), rvURL string, blocked *atomic.Bool, relayICE []proto.ICEServer) error {
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	cleanup(func() { socks.Close() })
	known := func(dev string) bool { return n.logs.Known(dev) || n.pairer.Known(dev) }
	routes := func(dev string) []peer.Route { return n.routesFor(dev, blocked, relayICE) }
	inbound := func(c *peer.Connection) {
		n.setInbound(c)
		go n.pairer.Serve(ctx, c.Device, c)
	}
	n.peer = peer.New(n.key, socks, n.db, known, routes, inbound, nil)
	n.pairer.Peer = n.peer
	path, err := peer.Dial(ctx, rvURL, n.key)
	if err != nil {
		return err
	}
	n.path = path
	cleanup(func() { path.Close() })
	event := func(kind string, data []byte) error {
		n.pairer.HandleEvent(ctx, path, kind, data)
		return nil
	}
	go path.Serve(ctx, n.peer, event)
	return nil
}

func (n *m8MemNode) routesFor(dev string, blocked *atomic.Bool, relayICE []proto.ICEServer) []peer.Route {
	var out []peer.Route
	if n.path != nil {
		out = append(out, m8FilterRoute(n.path.Route(), blocked, relayICE))
	}
	for _, r := range n.pairer.Routes(dev) {
		out = append(out, m8FilterRoute(r, blocked, relayICE))
	}
	return out
}

func (n *m8MemNode) setInbound(c *peer.Connection) {
	n.mu.Lock()
	n.inbound = c
	n.mu.Unlock()
}

func (n *m8MemNode) inboundConn() *peer.Connection {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.inbound
}

func (n *m8MemNode) startPair(ctx context.Context, secret, label string) <-chan m8NodePairOut {
	// Single reader: m8AwaitNodePair.
	done := make(chan m8NodePairOut, 1)
	go func() {
		res, err := n.pairer.Pair(ctx, node.PairRequest{Secret: secret, Name: label})
		done <- m8NodePairOut{res: res, err: err}
	}()
	return done
}

func (n *m8MemNode) waitKnown(expectedNodeID, browserID string) error {
	if expectedNodeID != n.key.ID() {
		return fmt.Errorf("room node key = %q, want %q", expectedNodeID, n.key.ID())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if n.pairer.Known(browserID) {
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

func newM8Browser(ctx context.Context, cleanup func(func()), key identity.Key, rvURL string, blocked *atomic.Bool, relayICE []proto.ICEServer) (*m8Browser, error) {
	path, err := peer.Dial(ctx, rvURL, key)
	if err != nil {
		return nil, err
	}
	cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return nil, err
	}
	cleanup(func() { socks.Close(); db.Close() })
	routes := func(string) []peer.Route { return []peer.Route{m8FilterRoute(path.Route(), blocked, relayICE)} }
	p := peer.New(key, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	b := &m8Browser{
		key:        key,
		path:       path,
		peer:       p,
		presence:   make(chan string, 16),
		roomEvents: make(chan m8RoomEvent, 16),
	}
	event := func(kind string, data []byte) error {
		b.onEvent(kind, data)
		return nil
	}
	go path.Serve(ctx, p, event)
	return b, nil
}

func (b *m8Browser) onEvent(kind string, data []byte) {
	switch kind {
	case "presence":
		b.onPresence(data)
	case "joined", "room", "full":
		cp := slices.Clone(data)
		select {
		case b.roomEvents <- m8RoomEvent{kind: kind, data: cp}:
		default:
		}
	}
}

func (b *m8Browser) onPresence(data []byte) {
	var pr proto.Presence
	if proto.Decode(data, &pr) == nil && pr.On {
		select {
		case b.presence <- pr.Key:
		default:
		}
	}
}

func (b *m8Browser) waitOnline(ctx context.Context, device string) error {
	if err := b.path.Write(ctx, proto.Watch{Type: "watch", Keys: []string{device}}); err != nil {
		return err
	}
	for {
		select {
		case k := <-b.presence:
			if k == device {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (b *m8Browser) exchangeRoom(ctx context.Context, secret identity.Secret, myRole, wantRole string) (proto.Pair, error) {
	room := secret.Room()
	self := proto.Pair{Type: "pair", Key: b.key.ID(), Kind: "browser", Name: "Owner Browser", Role: myRole}
	self.MAC = secret.MAC(self)
	rawSelf, err := json.Marshal(self)
	if err != nil {
		return proto.Pair{}, err
	}
	if err := b.path.Write(ctx, proto.RoomID{Type: "join", Room: room}); err != nil {
		return proto.Pair{}, err
	}
	return b.readRoomPeer(ctx, secret, room, wantRole, rawSelf)
}

func (b *m8Browser) readRoomPeer(ctx context.Context, secret identity.Secret, room, wantRole string, rawSelf []byte) (proto.Pair, error) {
	for {
		select {
		case ev := <-b.roomEvents:
			m, ok, err := b.stepRoomEvent(ctx, ev, secret, room, wantRole, rawSelf)
			if err != nil || ok {
				return m, err
			}
		case <-ctx.Done():
			return proto.Pair{}, ctx.Err()
		}
	}
}

func (b *m8Browser) stepRoomEvent(ctx context.Context, ev m8RoomEvent, secret identity.Secret, room, wantRole string, rawSelf []byte) (proto.Pair, bool, error) {
	switch ev.kind {
	case "joined":
		return proto.Pair{}, false, b.sendRoomPairIfMatch(ctx, ev.data, room, rawSelf)
	case "full":
		return proto.Pair{}, false, errors.New("pairing room is full")
	case "room":
		m, ok := m8MatchRoomPair(ev.data, secret, room, wantRole)
		return m, ok, nil
	default:
		return proto.Pair{}, false, nil
	}
}

func (b *m8Browser) sendRoomPairIfMatch(ctx context.Context, data []byte, room string, rawSelf []byte) error {
	var id proto.RoomID
	if proto.Decode(data, &id) != nil || id.Room != room {
		return nil
	}
	return b.path.Write(ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: rawSelf})
}

func m8MatchRoomPair(data []byte, secret identity.Secret, room, wantRole string) (proto.Pair, bool) {
	var rd proto.RoomData
	var m proto.Pair
	if proto.Decode(data, &rd) != nil || rd.Room != room {
		return proto.Pair{}, false
	}
	if proto.Decode(rd.Data, &m) != nil || !secret.Verify(m, wantRole) {
		return proto.Pair{}, false
	}
	return m, true
}

func (b *m8Browser) openCtl(ctx context.Context, cleanup func(func()), nodeID string) (*peer.Connection, error) {
	for range 2 {
		conn, ok, err := b.tryOpenCtl(ctx, cleanup, nodeID, 8*time.Second)
		if err != nil || ok {
			return conn, err
		}
	}
	return nil, errors.New("timed out opening ctl connection")
}

func (b *m8Browser) tryOpenCtl(ctx context.Context, cleanup func(func()), nodeID string, wait time.Duration) (*peer.Connection, bool, error) {
	conn, err := b.peer.Offer(ctx, nodeID)
	if err != nil {
		return nil, false, err
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-conn.Ready():
		cleanup(func() { conn.Close() })
		go m8DrainConn(ctx, conn)
		return conn, true, nil
	case <-conn.Done():
		conn.Close()
		return nil, false, nil
	case <-timer.C:
		conn.Close()
		return nil, false, nil
	case <-ctx.Done():
		conn.Close()
		return nil, false, ctx.Err()
	}
}

func m8DrainConn(ctx context.Context, conn *peer.Connection) {
	for {
		if _, err := conn.Read(ctx); err != nil {
			return
		}
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
	if blocked == nil {
		return r
	}
	if blocked.Load() {
		r.ICE = relayICE
	} else {
		r.ICE = m8RewriteRVICE(r.ICE)
	}
	orig := r.Send
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		if blocked.Load() && m8IsNonRelayICE(env.Payload) {
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

func m8IsNonRelayICE(payload string) bool {
	var sig struct {
		Type string `json:"t"`
		Cand string `json:"cand"`
	}
	if json.Unmarshal([]byte(payload), &sig) != nil || sig.Type != "ice" {
		return false
	}
	return !strings.Contains(sig.Cand, " typ relay")
}

func m8CheckOwnerWelcome(ctx context.Context, conn *peer.Connection, wantDevice string) error {
	w, err := requestBrowserHello(ctx, conn)
	if err != nil {
		return err
	}
	if !w.Owner || w.Device != wantDevice {
		return fmt.Errorf("welcome = %+v, want owner true for %q", w, wantDevice)
	}
	return nil
}

func m8RequestHostInfo(ctx context.Context, conn *peer.Connection) (proto.HostInfo, error) {
	build := func(id int64) any { return proto.Request{Type: "host.info", ID: id} }
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return proto.HostInfo{}, err
	}
	var info proto.HostInfo
	return info, proto.Decode(raw, &info)
}

func m8ExpectForbiddenHostInfo(ctx context.Context, conn *peer.Connection) error {
	build := func(id int64) any { return proto.Request{Type: "host.info", ID: id} }
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return err
	}
	return m8AssertWireError(raw, "forbidden")
}

func m8ExpectForbiddenCmd(ctx context.Context, signer identity.Key, nodeID string, conn *peer.Connection) error {
	payload := fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, exampleID("m8-pre"), time.Now().UnixMilli(), nodeID)
	env := signer.Sign(payload)
	build := func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	}
	raw, err := conn.Request(ctx, build)
	if err != nil {
		return err
	}
	return m8AssertWireError(raw, "forbidden")
}

func m8AssertWireError(raw json.RawMessage, wantCode string) error {
	var resp proto.ErrorResponse
	if err := proto.Decode(raw, &resp); err != nil {
		return fmt.Errorf("expected wire error %q, got %s (%v)", wantCode, raw, err)
	}
	if resp.Err.Code != wantCode {
		return fmt.Errorf("wire error code = %q, want %q", resp.Err.Code, wantCode)
	}
	return nil
}

func m8CheckBothPaths(browserConn, nodeConn *peer.Connection, want string) error {
	if err := m8WaitPath(browserConn, want, time.Second); err != nil {
		return fmt.Errorf("browser path: %w", err)
	}
	if err := m8WaitPath(nodeConn, want, time.Second); err != nil {
		return fmt.Errorf("node path: %w", err)
	}
	return nil
}

func m8WaitPath(conn *peer.Connection, want string, limit time.Duration) error {
	if conn == nil {
		return errors.New("connection is nil")
	}
	deadline := time.Now().Add(limit)
	for {
		got := conn.Path()
		if got == want {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("got %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func m8DeviceFingerprint(id string) (string, error) {
	pub, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid device id %q: %v", id, err)
	}
	h := sha256.Sum256(pub)
	return fmt.Sprintf("%X %X %X", h[0:2], h[2:4], h[4:6]), nil
}

func m8TempRoot(prefix string) (string, error) {
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	return os.MkdirTemp(base, prefix)
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

func m8InstallCommand(asset string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		script := "$ErrorActionPreference = 'Stop'\n" +
			"$asset = '" + asset + "'\n" +
			"$digest = (Get-FileHash -Algorithm SHA256 -LiteralPath $asset -ErrorAction Stop).Hash\n" +
			"if ((Get-Content -LiteralPath SHA256SUMS -ErrorAction Stop) -notcontains \"$digest  $asset\") {\n" +
			"    throw \"SHA256SUMS has no matching digest and filename\"\n" +
			"}\n" +
			"$bin = Join-Path $env:LOCALAPPDATA 'Programs\\SoloDawn'\n" +
			"New-Item -ItemType Directory -Force -Path $bin | Out-Null\n" +
			"Copy-Item -LiteralPath $asset -Destination (Join-Path $bin 'solodawn.exe')\n"
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	}
	script := "asset='" + asset + "'\n" +
		"if [ \"$(uname -s)\" = Darwin ]; then\n" +
		"  shasum -a 256 \"$asset\"\n" +
		"else\n" +
		"  sha256sum \"$asset\"\n" +
		"fi | grep -Fxf SHA256SUMS &&\n" +
		"  mkdir -p \"$HOME/.local/bin\" &&\n" +
		"  install -m 755 \"$asset\" \"$HOME/.local/bin/solodawn\" &&\n" +
		"  if [ \"$(uname -s)\" = Darwin ]; then\n" +
		"    xattr -d com.apple.quarantine \"$HOME/.local/bin/solodawn\" 2>/dev/null || true\n" +
		"  fi\n"
	return exec.Command("sh", "-c", script)
}

func m8ExecCLI(bin string, env []string, args ...string) (string, string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func m8KillWait(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
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
			proc.Signal(sig)
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
