//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
)

func init() {
	stepSets["node"] = nodeFeatureSteps
}

type runningNodeProc struct {
	root   string
	sock   string
	env    []string
	cmd    *exec.Cmd
	output syncBuffer
}

type listedNode struct {
	state proto.HostState
	path  string
}

type nodeScenarioWorld struct {
	f        fixtures
	cleanup  func(func())
	ctx      context.Context
	nodes    []*runningNodeProc
	statuses []string
	rvURL    string
	browser  *peerBrowser
	conns    []*peer.Connection
	entries  []proto.Envelope
	listed   []listedNode
}

func nodeFeatureSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &nodeScenarioWorld{
		f:       f,
		cleanup: afterScenario(sc),
	}
	sc.Step(`^two foreground nodes running in separate home directories on this machine$`, w.startTwoNodes)
	sc.Step(`^the owner runs "solodawn status" in each node's environment$`, w.runStatusOnEach)
	sc.Step(`^each status prints its node's version and distinct device fingerprint$`, w.checkStatuses)
	sc.Step(`^two foreground nodes running in separate home directories (?:and a shared|on that) rendezvous$`, func(ctx context.Context) error {
		return w.startTwoNodesWithRendezvous(sharedRVURL(ctx))
	})
	sc.Step(`^a fresh browser pairs the first node as "([^"]*)" and adds the second node as "([^"]*)"$`, w.pairBothNodes)
	sc.Step(`^the browser lists both nodes with their names, system, version and active path$`, w.checkBothNodesListed)
}

func (w *nodeScenarioWorld) startTwoNodes() error {
	w.nodes = nil
	w.statuses = nil
	for range 2 {
		np, err := startAcceptanceNode(w.f.t, w.f.bin, w.cleanup)
		if err != nil {
			return err
		}
		w.nodes = append(w.nodes, np)
	}
	return nil
}

func (w *nodeScenarioWorld) runStatusOnEach() error {
	for _, np := range w.nodes {
		out, err := runNodeCLI(w.f.bin, np.env, "status")
		if err != nil {
			return fmt.Errorf("status failed (%w): node=%q", err, np.output.String())
		}
		w.statuses = append(w.statuses, out)
	}
	return nil
}

func (w *nodeScenarioWorld) checkStatuses() error {
	if len(w.statuses) != 2 {
		return fmt.Errorf("got %d status outputs, want 2", len(w.statuses))
	}
	fp0, err := checkOneStatus(w.statuses[0])
	if err != nil {
		return err
	}
	fp1, err := checkOneStatus(w.statuses[1])
	if err != nil {
		return err
	}
	if fp0 == fp1 {
		return fmt.Errorf("both nodes reported the same fingerprint %q", fp0)
	}
	return nil
}

func checkOneStatus(out string) (string, error) {
	if !strings.Contains(out, "version dev\n") {
		return "", fmt.Errorf("status missing version: %q", out)
	}
	fp := extractFingerprintLine(out)
	if fp == "" {
		return "", fmt.Errorf("status missing fingerprint: %q", out)
	}
	return fp, nil
}

func (w *nodeScenarioWorld) startTwoNodesWithRendezvous(sharedURL string) error {
	cleanWindowsLoginTaskIfAbsent(w.f.t, w.cleanup)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	rvURL, err := startAcceptanceRelayRVWithBack(w.cleanup, sharedURL)
	if err != nil {
		return err
	}
	w.rvURL = rvURL
	if err := w.startTwoNodes(); err != nil {
		return err
	}
	b, err := newPeerBrowser(w.ctx, w.cleanup, exampleKey("a12-browser"), w.rvURL, nil)
	if err != nil {
		return err
	}
	w.browser = b
	w.entries = []proto.Envelope{
		nodetest.SignDeviceAdd(b.key, b.key.ID(), 1, "browser", "Owner Browser", b.key.ID(), time.Now().UnixMilli()),
	}
	return nil
}

func (w *nodeScenarioWorld) pairBothNodes(firstName, secondName string) error {
	c0, err := w.pairRunningNode(w.nodes[0], firstName)
	if err != nil {
		return err
	}
	w.conns = append(w.conns, c0)
	c1, err := w.pairRunningNode(w.nodes[1], secondName)
	if err != nil {
		return err
	}
	w.conns = append(w.conns, c1)
	if err := c0.Send(proto.Devlog{Type: "devlog", User: w.browser.key.ID(), Entries: w.entries[len(w.entries)-1:]}); err != nil {
		return err
	}
	return w.collectListedNodes()
}

func (w *nodeScenarioWorld) pairRunningNode(np *runningNodeProc, label string) (*peer.Connection, error) {
	secret := identity.NewSecret()
	pairDone := w.startNodePairCLI(np.env, secret.String(), label)
	roomMsg, err := w.browser.exchangeRoom(w.ctx, secret, "a", "b")
	if err != nil {
		return nil, err
	}
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, roomMsg.Key)
	if err != nil {
		return nil, err
	}
	if err := w.enrollNodeOverCtl(bc.conn, roomMsg.Key, label, pairDone); err != nil {
		return nil, err
	}
	if err := verifyNodeStatusFP(w.f.bin, np.env, roomMsg.Key); err != nil {
		return nil, err
	}
	return bc.conn, nil
}

func verifyNodeStatusFP(bin string, env []string, nodeID string) error {
	wantFP, err := deviceFingerprint(nodeID)
	if err != nil {
		return err
	}
	out, err := runNodeCLI(bin, env, "status")
	if err != nil {
		return err
	}
	if got := extractFingerprintLine(out); got != wantFP {
		return fmt.Errorf("status fingerprint = %q, want %q", got, wantFP)
	}
	return nil
}

func (w *nodeScenarioWorld) startNodePairCLI(env []string, secret, label string) <-chan error {
	pairCtx, cancel := context.WithCancel(w.ctx)
	// Single reader: waitPairDone.
	pairDone := make(chan error, 1)
	var settled sync.WaitGroup
	settled.Add(1)
	go func() {
		defer settled.Done()
		_, err := runNodeCLIContext(pairCtx, w.f.bin, env, "pair", secret, "--name", label, "--rendezvous", w.rvURL)
		pairDone <- err
	}()
	registerBothCleanups(w.f.t, w.cleanup, func() {
		cancel()
		settled.Wait()
	})
	return pairDone
}

func (w *nodeScenarioWorld) enrollNodeOverCtl(conn *peer.Connection, nodeID, label string, pairDone <-chan error) error {
	if _, err := requestBrowserHello(w.ctx, conn); err != nil {
		return err
	}
	ver := len(w.entries) + 1
	bKey := w.browser.key
	w.entries = append(w.entries, nodetest.SignDeviceAdd(bKey, bKey.ID(), ver, "node", label, nodeID, time.Now().UnixMilli()))
	if err := conn.Send(proto.Devlog{Type: "devlog", User: bKey.ID(), Entries: w.entries}); err != nil {
		return err
	}
	if err := w.waitPairDone(pairDone); err != nil {
		return err
	}
	return checkOwnerWelcome(w.ctx, conn, nodeID)
}

func (w *nodeScenarioWorld) waitPairDone(pairDone <-chan error) error {
	select {
	case err := <-pairDone:
		return err
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *nodeScenarioWorld) collectListedNodes() error {
	w.listed = nil
	for _, conn := range w.conns {
		info, err := requestHostInfo(w.ctx, conn)
		if err != nil {
			return err
		}
		w.listed = append(w.listed, listedNode{state: info.HostState, path: conn.Path()})
	}
	return nil
}

func (w *nodeScenarioWorld) checkBothNodesListed() error {
	if len(w.listed) != 2 {
		return fmt.Errorf("listed %d nodes, want 2", len(w.listed))
	}
	if w.listed[0].state.Device == w.listed[1].state.Device {
		return fmt.Errorf("duplicate device id %q", w.listed[0].state.Device)
	}
	if err := checkListedNodeEntry(w.listed[0], "alpha"); err != nil {
		return err
	}
	return checkListedNodeEntry(w.listed[1], "beta")
}

func checkListedNodeEntry(item listedNode, wantName string) error {
	wantOS := runtime.GOOS + "/" + runtime.GOARCH
	if item.state.Name != wantName || item.state.OS != wantOS || item.state.Version != "dev" {
		return fmt.Errorf("unexpected host.info for %s: %+v", wantName, item.state)
	}
	if item.path != "direct" && item.path != "relay" {
		return fmt.Errorf("unexpected path for %s: %q", wantName, item.path)
	}
	return nil
}

func startAcceptanceRelayRV(cleanup func(func())) (string, error) {
	return startAcceptanceRelayRVWithBack(cleanup, "")
}

func startAcceptanceRelayRVWithBack(cleanup func(func()), backURL string) (string, error) {
	relay := &peersWorld{}
	err := relay.relayOnly()
	for _, undo := range relay.cleanup {
		cleanup(undo)
	}
	if err != nil {
		return "", err
	}
	iceRaw, _ := json.Marshal(relay.relay)
	if backURL == "" {
		back := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
		cleanup(back.Close)
		backURL = "ws" + strings.TrimPrefix(back.URL, "http") + "/rv"
	}
	front := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		proxyAcceptanceRV(rw, r, backURL, string(iceRaw))
	}))
	cleanup(front.Close)
	return "ws" + strings.TrimPrefix(front.URL, "http") + "/rv", nil
}

func proxyAcceptanceRV(rw http.ResponseWriter, r *http.Request, backURL, iceJSON string) {
	cConn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer cConn.CloseNow()
	bConn, _, err := websocket.Dial(r.Context(), backURL, &websocket.DialOptions{HTTPClient: http.DefaultClient})
	if err != nil {
		return
	}
	defer bConn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go pumpAcceptanceWS(ctx, cancel, cConn, bConn, nil)
	pumpAcceptanceWS(ctx, cancel, bConn, cConn, func(b []byte) []byte {
		return rewriteAcceptanceRVFrame(b, iceJSON)
	})
}

func rewriteAcceptanceRVFrame(b []byte, fallbackICE string) []byte {
	if bytes.Contains(b, []byte(`"ice":[]`)) {
		return bytes.Replace(b, []byte(`"ice":[]`), []byte(`"ice":`+fallbackICE), 1)
	}
	if !bytes.Contains(b, []byte(`"stun:localhost:`)) {
		return b
	}
	var cfg proto.ICEConfig
	if proto.Decode(b, &cfg) != nil {
		return b
	}
	cfg.ICE = rewriteSubprocICE(cfg.ICE)
	out, err := marshalNoEscape(cfg)
	if err != nil {
		return b
	}
	return out
}

func rewriteSubprocICE(servers []proto.ICEServer) []proto.ICEServer {
	var out []proto.ICEServer
	for _, s := range servers {
		if urls := rewriteSubprocURLs(s.URLs); len(urls) > 0 {
			cp := s
			cp.URLs = urls
			out = append(out, cp)
		}
	}
	return out
}

func rewriteSubprocURLs(urls []string) []string {
	var out []string
	for _, u := range urls {
		if strings.HasPrefix(u, "stun:") || strings.HasPrefix(u, "turn:") {
			out = append(out, strings.Replace(u, "localhost:", "127.0.0.1:", 1))
		}
	}
	return out
}

func pumpAcceptanceWS(ctx context.Context, cancel context.CancelFunc, src, dst *websocket.Conn, mapFn func([]byte) []byte) {
	defer cancel()
	for {
		typ, msg, err := src.Read(ctx)
		if err != nil {
			return
		}
		if mapFn != nil {
			msg = mapFn(msg)
		}
		if dst.Write(ctx, typ, msg) != nil {
			return
		}
	}
}

func runNodeCLI(bin string, env []string, args ...string) (string, error) {
	return runNodeCLIContext(context.Background(), bin, env, args...)
}

func runNodeCLIContext(ctx context.Context, bin string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil || stderr.Len() != 0 {
		return "", fmt.Errorf("%v (stdout=%q stderr=%q)", err, out.String(), stderr.String())
	}
	return out.String(), nil
}

func startAcceptanceNode(t testing.TB, bin string, cleanup func(func())) (*runningNodeProc, error) {
	root, err := shortTempRoot(t, cleanup, "sdn")
	if err != nil {
		return nil, err
	}
	rel := "solodawn"
	if runtime.GOOS == "darwin" {
		rel = filepath.Join("Library", "Application Support", "solodawn")
	}
	sock := filepath.Join(root, rel, "sock")
	env := append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "XDG_CONFIG_HOME="+root, "LocalAppData="+root)
	np := &runningNodeProc{root: root, sock: sock, env: env}
	cmd := exec.Command(bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.output, &np.output
	if _, err := startNodeCmd(t, cleanup, cmd, "node", filepath.Dir(sock), &np.output); err != nil {
		return nil, err
	}
	np.cmd = cmd
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return nil, fmt.Errorf("node did not bind %s: %w (%s)", sock, err, np.output.String())
	}
	return np, nil
}

func waitAcceptanceSocket(sock string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			conn.Close()
			return nil
		}
		last = err
		time.Sleep(25 * time.Millisecond)
	}
	return last
}

func extractFingerprintLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if fp, ok := strings.CutPrefix(line, "fingerprint "); ok && fp != "" {
			return fp
		}
	}
	return ""
}
