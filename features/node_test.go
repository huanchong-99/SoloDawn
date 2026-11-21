//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["node"] = nodeFeatureSteps
}

type runningNodeProc struct {
	root   string
	sock   string
	env    []string
	cmd    *exec.Cmd
	output bytes.Buffer
}

type listedNode struct {
	state proto.HostState
	path  string
}

type nodeScenarioWorld struct {
	bin      string
	cleanup  func(func())
	ctx      context.Context
	nodes    []*runningNodeProc
	statuses []string
	rvURL    string
	bKey     identity.Key
	bPath    *peer.Path
	bPeer    *peer.Peer
	conns    []*peer.Connection
	entries  []proto.Envelope
	listed   []listedNode

	roomMu   sync.Mutex
	roomWait map[string]roomExpectation
}

type roomExpectation struct {
	secret identity.Secret
	rawMsg json.RawMessage
	found  chan proto.Pair // single reader: pairRunningNode
}

func nodeFeatureSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &nodeScenarioWorld{
		bin:      f.bin,
		cleanup:  afterScenario(sc),
		roomWait: map[string]roomExpectation{},
	}
	sc.Step(`^two foreground nodes running in separate home directories on this machine$`, w.startTwoNodes)
	sc.Step(`^the owner runs "solodawn status" in each node's environment$`, w.runStatusOnEach)
	sc.Step(`^each status prints its node's version and distinct device fingerprint$`, w.checkStatuses)
	sc.Step(`^two foreground nodes running in separate home directories and a shared rendezvous$`, w.startTwoNodesWithRendezvous)
	sc.Step(`^a fresh browser pairs the first node as "([^"]*)" and adds the second node as "([^"]*)"$`, w.pairBothNodes)
	sc.Step(`^the browser lists both nodes with their names, system, version and active path$`, w.checkBothNodesListed)
}

func (w *nodeScenarioWorld) startTwoNodes() error {
	w.nodes = nil
	w.statuses = nil
	for range 2 {
		np, err := startAcceptanceNode(w.bin, w.cleanup)
		if err != nil {
			return err
		}
		w.nodes = append(w.nodes, np)
	}
	return nil
}

func (w *nodeScenarioWorld) runStatusOnEach() error {
	for _, np := range w.nodes {
		out, err := runNodeCLI(w.bin, np.env, "status")
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

func (w *nodeScenarioWorld) startTwoNodesWithRendezvous() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	rvURL, err := startAcceptanceRelayRV(w.cleanup)
	if err != nil {
		return err
	}
	w.rvURL = rvURL
	if err := w.startTwoNodes(); err != nil {
		return err
	}
	return w.openSimulatedBrowser()
}

func (w *nodeScenarioWorld) openSimulatedBrowser() error {
	w.bKey = exampleKey("a12-browser")
	path, err := peer.Dial(w.ctx, w.rvURL, w.bKey)
	if err != nil {
		return err
	}
	w.cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	w.cleanup(func() { socks.Close() })
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	routes := func(string) []peer.Route { return []peer.Route{path.Route()} }
	w.bPath = path
	w.bPeer = peer.New(w.bKey, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	go path.Serve(w.ctx, w.bPeer, w.onBrowserRVEvent)
	w.entries = []proto.Envelope{signAcceptanceAdd(w.bKey, w.bKey.ID(), 1, w.bKey.ID(), "browser", "Owner Browser")}
	return nil
}

func (w *nodeScenarioWorld) onBrowserRVEvent(kind string, data []byte) error {
	var rm proto.RoomID
	if proto.Decode(data, &rm) != nil {
		return nil
	}
	w.roomMu.Lock()
	exp, ok := w.roomWait[rm.Room]
	w.roomMu.Unlock()
	if !ok {
		return nil
	}
	w.handleMatchedRoomEvent(kind, rm.Room, data, exp)
	return nil
}

func (w *nodeScenarioWorld) handleMatchedRoomEvent(kind, room string, data []byte, exp roomExpectation) {
	if kind == "joined" {
		w.bPath.Write(w.ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: exp.rawMsg})
		return
	}
	var rd proto.RoomData
	var m proto.Pair
	if kind == "room" && proto.Decode(data, &rd) == nil && proto.Decode(rd.Data, &m) == nil && exp.secret.Verify(m, "b") {
		w.roomMu.Lock()
		delete(w.roomWait, room)
		w.roomMu.Unlock()
		exp.found <- m
	}
}

func (w *nodeScenarioWorld) pairBothNodes(firstName, secondName string) error {
	if runtime.GOOS == "windows" {
		w.cleanup(func() { exec.Command(w.bin, "service", "remove").Run() })
	}
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
	if err := c0.Send(proto.Devlog{Type: "devlog", User: w.bKey.ID(), Entries: w.entries[len(w.entries)-1:]}); err != nil {
		return err
	}
	return w.collectListedNodes()
}

func (w *nodeScenarioWorld) pairRunningNode(np *runningNodeProc, label string) (*peer.Connection, error) {
	secret := identity.NewSecret()
	room := secret.Room()
	self := proto.Pair{Type: "pair", Key: w.bKey.ID(), Kind: "browser", Name: "Owner Browser", Role: "a"}
	self.MAC = secret.MAC(self)
	rawSelf, _ := json.Marshal(self)
	found := make(chan proto.Pair, 1)
	w.roomMu.Lock()
	w.roomWait[room] = roomExpectation{secret: secret, rawMsg: rawSelf, found: found}
	w.roomMu.Unlock()
	if err := w.bPath.Write(w.ctx, proto.RoomID{Type: "join", Room: room}); err != nil {
		return nil, err
	}

	pairDone := make(chan error, 1) // single reader: finishNodeEnrolment
	go func() {
		_, err := runNodeCLI(w.bin, np.env, "pair", secret.String(), "--name", label, "--rendezvous", w.rvURL)
		pairDone <- err
	}()
	return w.finishNodeEnrolment(found, pairDone, label)
}

func (w *nodeScenarioWorld) finishNodeEnrolment(found <-chan proto.Pair, pairDone <-chan error, label string) (*peer.Connection, error) {
	select {
	case m := <-found:
		conn, err := w.openBrowserConn(m.Key)
		if err != nil {
			return nil, err
		}
		return conn, w.enrollNodeOverCtl(conn, m.Key, label, pairDone)
	case err := <-pairDone:
		return nil, fmt.Errorf("pair exited before room match: %w", err)
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	}
}

func (w *nodeScenarioWorld) openBrowserConn(nodeID string) (*peer.Connection, error) {
	conn, err := w.bPeer.Offer(w.ctx, nodeID)
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { conn.Close() })
	select {
	case <-conn.Ready():
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	}
	go func() {
		for {
			if _, err := conn.Read(w.ctx); err != nil {
				return
			}
		}
	}()
	return conn, nil
}

func (w *nodeScenarioWorld) enrollNodeOverCtl(conn *peer.Connection, nodeID, label string, pairDone <-chan error) error {
	if _, err := requestBrowserHello(w.ctx, conn); err != nil {
		return err
	}
	ver := len(w.entries) + 1
	w.entries = append(w.entries, signAcceptanceAdd(w.bKey, w.bKey.ID(), ver, nodeID, "node", label))
	if err := conn.Send(proto.Devlog{Type: "devlog", User: w.bKey.ID(), Entries: w.entries}); err != nil {
		return err
	}
	if err := w.waitPairDone(pairDone); err != nil {
		return err
	}
	welc, err := requestBrowserHello(w.ctx, conn)
	if err != nil || !welc.Owner {
		return fmt.Errorf("post-adopt welcome owner=%v err=%v", welc.Owner, err)
	}
	return nil
}

func (w *nodeScenarioWorld) waitPairDone(pairDone <-chan error) error {
	select {
	case err := <-pairDone:
		return err
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func requestBrowserHello(ctx context.Context, conn *peer.Connection) (proto.Welcome, error) {
	raw, err := conn.Request(ctx, func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Owner Browser", Addrs: []string{}}
	})
	if err != nil {
		return proto.Welcome{}, err
	}
	var welc proto.Welcome
	return welc, proto.Decode(raw, &welc)
}

func (w *nodeScenarioWorld) collectListedNodes() error {
	w.listed = nil
	for _, conn := range w.conns {
		raw, err := conn.Request(w.ctx, func(id int64) any {
			return proto.Request{Type: "host.info", ID: id}
		})
		if err != nil {
			return err
		}
		var ans struct {
			proto.Success
			proto.HostState
		}
		if err := proto.Decode(raw, &ans); err != nil {
			return err
		}
		w.listed = append(w.listed, listedNode{state: ans.HostState, path: conn.Path()})
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

func signAcceptanceAdd(signer identity.Key, user string, ver int, dev, kind, name string) proto.Envelope {
	var b [16]byte
	rand.Read(b[:])
	nonce := proto.ID(base64.RawURLEncoding.EncodeToString(b[:]))
	raw, _ := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: nonce, Timestamp: time.Now().UnixMilli()},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	})
	return signer.Sign(string(raw))
}

func startAcceptanceRelayRV(cleanup func(func())) (string, error) {
	relay := &peersWorld{}
	err := relay.relayOnly()
	for _, undo := range relay.cleanup {
		cleanup(undo)
	}
	if err != nil {
		return "", err
	}
	iceRaw, _ := json.Marshal(relay.relay)
	back := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	cleanup(back.Close)
	backURL := "ws" + strings.TrimPrefix(back.URL, "http") + "/rv"
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
	bConn, _, err := websocket.Dial(r.Context(), backURL, nil)
	if err != nil {
		return
	}
	defer bConn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go pumpAcceptanceWS(ctx, cancel, cConn, bConn, nil)
	pumpAcceptanceWS(ctx, cancel, bConn, cConn, func(b []byte) []byte {
		return bytes.Replace(b, []byte(`"ice":[]`), []byte(`"ice":`+iceJSON), 1)
	})
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
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil || stderr.Len() != 0 {
		return "", fmt.Errorf("%v (stdout=%q stderr=%q)", err, out.String(), stderr.String())
	}
	return out.String(), nil
}

func startAcceptanceNode(bin string, cleanup func(func())) (*runningNodeProc, error) {
	base, rel := os.TempDir(), "solodawn"
	if runtime.GOOS == "darwin" {
		base, rel = "/tmp", filepath.Join("Library", "Application Support", "solodawn")
	}
	root, err := os.MkdirTemp(base, "sdn")
	if err != nil {
		return nil, err
	}
	cleanup(func() { os.RemoveAll(root) })
	sock := filepath.Join(root, rel, "sock")
	env := append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "XDG_CONFIG_HOME="+root, "LocalAppData="+root)
	np := &runningNodeProc{root: root, sock: sock, env: env}
	cmd := exec.Command(bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.output, &np.output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	np.cmd = cmd
	cleanup(func() { stopAcceptanceNodeProc(cmd) })
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return nil, fmt.Errorf("node did not bind %s: %w (%s)", sock, err, np.output.String())
	}
	return np, nil
}

func stopAcceptanceNodeProc(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	done := make(chan struct{}) // single reader: stopAcceptanceNodeProc
	go func() {
		cmd.Wait()
		close(done)
	}()
	if cmd.Process.Signal(os.Interrupt) != nil {
		cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		<-done
	}
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
