//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
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
	"github.com/pion/turn/v5"
)

func init() {
	stepSets["m6"] = m6Steps
}

const (
	m6GroupName = "Project"
	m6OwnerName = "Alice"
)

type m6NodeProc struct {
	name     string
	root     string
	data     string
	key      identity.Key
	cmd      *exec.Cmd
	stopOnce sync.Once
	done     chan struct{} // Single reader: m6NodeProc.terminate waiting for cmd.Wait.
	out      bytes.Buffer
}

type m6OwnerLink struct {
	conn   *peer.Connection
	pushes chan json.RawMessage // Single reader: scenario steps draining pushes on conn.
}

type m6World struct {
	bin        string
	cleanup    func(func())
	ctx        context.Context
	root       string
	repoDir    string
	cliLogPath string
	rvURL      string
	ice        []proto.ICEServer
	ownerKey   identity.Key
	ownerPath  *peer.Path
	ownerPeer  *peer.Peer
	homeNode   *m6NodeProc
	aiNode     *m6NodeProc
	homeLink   *m6OwnerLink
	aiLink     *m6OwnerLink
	groupID    proto.ID
	aiMemberID string
	seenMsgs   map[string]bool
	idSeq      uint64

	mu     sync.Mutex
	online map[string]bool
	ready  chan struct{} // Closed once when both nodes report online on the rendezvous; single reader: waitBothNodesOnline.
}

func m6Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m6World{
		bin:      f.bin,
		cleanup:  afterScenario(sc),
		seenMsgs: map[string]bool{},
		online:   map[string]bool{},
		ready:    make(chan struct{}),
	}
	sc.Step(`^two owned nodes over ctl and a project group on the first node$`, w.givenTwoNodesAndGroup)
	sc.Step(`^"([^"]+)" on the second node is scripted to answer "([^"]+)" with "([^"]+)" and "([^"]+)" with "([^"]+)" as "([^"]+)"$`, w.givenTwoTurnScript)
	sc.Step(`^the owner joins "([^"]+)" on the second node to the group as "([^"]+)"$`, w.givenJoinedRemoteAI)
	sc.Step(`^the owner mentions "([^"]+)" in the group with "([^"]+)"$`, w.whenOwnerMentionsAI)
	sc.Step(`^"([^"]+)" posts "([^"]+)" from the second node and enters waiting$`, w.thenAIPostsAndWaits)
	sc.Step(`^one "([^"]+)" CLI process existed throughout on the second node$`, w.thenOneCLIProcessRan)
}

func (w *m6World) nextID() proto.ID {
	w.idSeq++
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], w.idSeq)
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

func (w *m6World) givenTwoNodesAndGroup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := w.prepareRootAndRepo(); err != nil {
		return err
	}
	if err := w.startSignalling(); err != nil {
		return err
	}
	if err := w.prepareAndStartNodes(); err != nil {
		return err
	}
	if err := w.connectOwnerToNodes(); err != nil {
		return err
	}
	return w.createAndSyncGroup()
}

func (w *m6World) prepareRootAndRepo() error {
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	rawRoot, err := os.MkdirTemp(base, "sdm6")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(rawRoot) })
	realRoot, err := filepath.EvalSymlinks(rawRoot)
	if err != nil {
		return err
	}
	w.root = realRoot
	w.cliLogPath = filepath.Join(realRoot, "fakecli.log")
	w.repoDir = filepath.Join(realRoot, "repo")
	return initM6GitRepo(realRoot, w.repoDir)
}

func initM6GitRepo(parentDir, repoDir string) error {
	steps := [][]string{
		{"init", "-b", "main", repoDir},
		{"-C", repoDir, "config", "user.email", "owner@example.com"},
		{"-C", repoDir, "config", "user.name", m6OwnerName},
		{"-C", repoDir, "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = parentDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w (%s)", args, err, out)
		}
	}
	return nil
}

func (w *m6World) startSignalling() error {
	ice, err := startM6LoopbackTURN(w.cleanup)
	if err != nil {
		return err
	}
	w.ice = ice
	w.rvURL = startM6RVProxy(w.ctx, ice, w.cleanup)
	return nil
}

func startM6LoopbackTURN(cleanup func(func())) ([]proto.ICEServer, error) {
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ip := net.IPv4(127, 0, 0, 1)
	gen := &turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()}
	auth := func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "test", "secret"), r.Username == "peer"
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:             "test",
		AuthHandler:       auth,
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen}},
	})
	if err != nil {
		udp.Close()
		return nil, err
	}
	cleanup(func() { srv.Close() })
	entry := proto.ICEServer{
		URLs:       []string{"turn:" + udp.LocalAddr().String() + "?transport=udp"},
		Username:   "peer",
		Credential: "secret",
	}
	return []proto.ICEServer{entry}, nil
}

func startM6RVProxy(ctx context.Context, ice []proto.ICEServer, cleanup func(func())) string {
	upstream := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	cleanup(upstream.Close)
	upWS := "ws" + strings.TrimPrefix(upstream.URL, "http") + "/rv"
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		serveM6RVConn(ctx, upWS, ice, rw, r)
	})
	proxy := httptest.NewServer(handler)
	cleanup(proxy.Close)
	return "ws" + strings.TrimPrefix(proxy.URL, "http") + "/rv"
}

func serveM6RVConn(ctx context.Context, upWS string, ice []proto.ICEServer, rw http.ResponseWriter, r *http.Request) {
	down, err := websocket.Accept(rw, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer down.CloseNow()
	up, _, err := websocket.Dial(ctx, upWS, nil)
	if err != nil {
		return
	}
	defer up.CloseNow()
	pairCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		pumpM6ClientToRV(pairCtx, down, up)
		cancel()
	}()
	pumpM6RVToClient(pairCtx, up, down, ice)
}

func pumpM6ClientToRV(ctx context.Context, down, up *websocket.Conn) {
	for {
		_, data, err := down.Read(ctx)
		if err != nil {
			return
		}
		if m6DropNonRelayICE(data) {
			continue
		}
		if up.Write(ctx, websocket.MessageText, data) != nil {
			return
		}
	}
}

func m6DropNonRelayICE(data []byte) bool {
	var route proto.Route
	if proto.Decode(data, &route) != nil || route.Type != "to" {
		return false
	}
	var env proto.Envelope
	if proto.Decode(route.Env, &env) != nil {
		return false
	}
	var sig struct {
		Type string `json:"t"`
		Cand string `json:"cand"`
	}
	if json.Unmarshal([]byte(env.Payload), &sig) != nil || sig.Type != "ice" {
		return false
	}
	return !strings.Contains(sig.Cand, " typ relay")
}

func pumpM6RVToClient(ctx context.Context, up, down *websocket.Conn, ice []proto.ICEServer) {
	for {
		_, data, err := up.Read(ctx)
		if err != nil {
			return
		}
		out := m6RewriteICEFrame(data, ice)
		if down.Write(ctx, websocket.MessageText, out) != nil {
			return
		}
	}
}

func m6RewriteICEFrame(data []byte, ice []proto.ICEServer) []byte {
	var hdr proto.RVFrame
	if proto.Decode(data, &hdr) != nil || (hdr.Type != "ready" && hdr.Type != "ice") {
		return data
	}
	var cfg proto.ICEConfig
	if proto.Decode(data, &cfg) != nil {
		return data
	}
	cfg.ICE = ice
	return m6MarshalJSON(cfg)
}

func (w *m6World) prepareAndStartNodes() error {
	w.ownerKey = m6NewKey()
	homeNode, err := w.initNodeDir("home-node")
	if err != nil {
		return err
	}
	aiNode, err := w.initNodeDir("ai-node")
	if err != nil {
		return err
	}
	w.homeNode, w.aiNode = homeNode, aiNode
	entries := w.buildDeviceLog(homeNode, aiNode)
	if err := w.seedNodeDB(homeNode, entries, false); err != nil {
		return err
	}
	if err := w.seedNodeDB(aiNode, entries, true); err != nil {
		return err
	}
	if err := w.startNodeProc(homeNode, nil); err != nil {
		return err
	}
	extraEnv := []string{
		"SOLODAWN_FAKECLI_LOG=" + w.cliLogPath,
		"SOLODAWN_FAKE_CLAUDE_SCRIPT=" + filepath.Join(w.root, "claude.json"),
		"SOLODAWN_FAKE_CODEX_SCRIPT=" + filepath.Join(w.root, "codex.json"),
	}
	return w.startNodeProc(aiNode, extraEnv)
}

func (w *m6World) initNodeDir(name string) (*m6NodeProc, error) {
	nodeRoot := filepath.Join(w.root, name)
	data := filepath.Join(nodeRoot, "solodawn")
	if runtime.GOOS == "darwin" {
		data = filepath.Join(nodeRoot, "Library", "Application Support", "solodawn")
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return nil, err
	}
	key, err := identity.LoadKey(data)
	if err != nil {
		return nil, err
	}
	return &m6NodeProc{name: name, root: nodeRoot, data: data, key: key}, nil
}

func (w *m6World) buildDeviceLog(n1, n2 *m6NodeProc) []proto.Envelope {
	ts := time.Now().UnixMilli()
	ownerID := w.ownerKey.ID()
	return []proto.Envelope{
		w.signDeviceAdd(ownerID, 1, "browser", "owner", ownerID, ts),
		w.signDeviceAdd(ownerID, 2, "node", n1.name, n1.key.ID(), ts),
		w.signDeviceAdd(ownerID, 3, "node", n2.name, n2.key.ID(), ts),
	}
}

func (w *m6World) signDeviceAdd(user string, ver int, kind, name, dev string, ts int64) proto.Envelope {
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: w.nextID(), Timestamp: ts},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	}
	return w.ownerKey.Sign(string(m6MarshalJSON(entry)))
}

func (w *m6World) seedNodeDB(np *m6NodeProc, entries []proto.Envelope, withAgents bool) error {
	db, err := store.Open(filepath.Join(np.data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	ownerJSON := string(m6MarshalJSON(w.ownerKey.ID()))
	nameJSON := string(m6MarshalJSON(np.name))
	rvJSON := string(m6MarshalJSON([]string{w.rvURL}))
	write := func(tx *store.Tx) error {
		if err := seedM6Settings(tx, ownerJSON, nameJSON, rvJSON, withAgents); err != nil {
			return err
		}
		return seedM6LogEntries(tx, w.ownerKey.ID(), entries)
	}
	return db.Write(write)
}

func seedM6Settings(tx *store.Tx, ownerJSON, nameJSON, rvJSON string, withAgents bool) error {
	_, err := tx.Exec(
		`INSERT INTO setting(key, value) VALUES('owner', ?), ('name', ?), ('rendezvous', ?)`,
		ownerJSON, nameJSON, rvJSON,
	)
	if err != nil || !withAgents {
		return err
	}
	_, err = tx.Exec(`INSERT INTO agent(agent, cli_version) VALUES('claude', '2.1.311'), ('codex', '0.164.0')`)
	return err
}

func seedM6LogEntries(tx *store.Tx, user string, entries []proto.Envelope) error {
	for i, env := range entries {
		if _, err := tx.Exec(`INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)`, user, i+1, string(m6MarshalJSON(env))); err != nil {
			return err
		}
	}
	return nil
}

func (w *m6World) startNodeProc(np *m6NodeProc, extraEnv []string) error {
	env := append(os.Environ(), "HOME="+np.root, "XDG_DATA_HOME="+np.root, "LocalAppData="+np.root)
	env = append(env, extraEnv...)
	cmd := exec.Command(w.bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.out, &np.out
	if err := cmd.Start(); err != nil {
		return err
	}
	np.cmd = cmd
	np.done = make(chan struct{})
	go func() {
		cmd.Wait()
		close(np.done)
	}()
	w.cleanup(np.stop)
	sock := filepath.Join(np.data, "sock")
	if err := waitM6Socket(sock, 10*time.Second); err != nil {
		return fmt.Errorf("%s socket %s: %w (%s)", np.name, sock, err, np.out.String())
	}
	return nil
}

func (np *m6NodeProc) stop() {
	np.stopOnce.Do(np.terminate)
}

func (np *m6NodeProc) terminate() {
	if np.cmd.Process.Signal(os.Interrupt) != nil {
		np.cmd.Process.Kill()
	}
	select {
	case <-np.done:
	case <-time.After(3 * time.Second):
		np.cmd.Process.Kill()
		<-np.done
	}
}

func waitM6Socket(sock string, wait time.Duration) error {
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

func (w *m6World) connectOwnerToNodes() error {
	if err := w.openOwnerPeer(); err != nil {
		return err
	}
	if err := w.waitBothNodesOnline(); err != nil {
		return err
	}
	homeLink, err := w.dialOwnerCtl(w.homeNode.key.ID())
	if err != nil {
		return err
	}
	aiLink, err := w.dialOwnerCtl(w.aiNode.key.ID())
	if err != nil {
		return err
	}
	w.homeLink, w.aiLink = homeLink, aiLink
	return nil
}

func (w *m6World) openOwnerPeer() error {
	path, err := peer.Dial(w.ctx, w.rvURL, w.ownerKey)
	if err != nil {
		return err
	}
	w.cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return err
	}
	w.cleanup(func() { socks.Close(); db.Close() })
	routes := func(string) []peer.Route { return []peer.Route{path.Route()} }
	w.ownerPath = path
	w.ownerPeer = peer.New(w.ownerKey, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	go path.Serve(w.ctx, w.ownerPeer, w.onOwnerRVEvent)
	return path.Write(w.ctx, proto.Watch{Type: "watch", Keys: []string{w.homeNode.key.ID(), w.aiNode.key.ID()}})
}

func (w *m6World) onOwnerRVEvent(kind string, data []byte) error {
	if kind != "presence" {
		return nil
	}
	var p proto.Presence
	if proto.Decode(data, &p) != nil || !p.On {
		return nil
	}
	w.recordNodeOnline(p.Key)
	return nil
}

func (w *m6World) recordNodeOnline(device string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	wasBoth := w.bothOnlineLocked()
	w.online[device] = true
	if !wasBoth && w.bothOnlineLocked() {
		close(w.ready)
	}
}

func (w *m6World) bothOnlineLocked() bool {
	return w.online[w.homeNode.key.ID()] && w.online[w.aiNode.key.ID()]
}

func (w *m6World) waitBothNodesOnline() error {
	select {
	case <-w.ready:
		return nil
	case <-w.ctx.Done():
		return fmt.Errorf("timed out waiting for nodes on rendezvous: %w", w.ctx.Err())
	}
}

func (w *m6World) dialOwnerCtl(device string) (*m6OwnerLink, error) {
	conn, err := w.ownerPeer.Offer(w.ctx, device)
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { conn.Close() })
	select {
	case <-conn.Ready():
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	}
	link := &m6OwnerLink{conn: conn, pushes: make(chan json.RawMessage, 64)}
	go pumpM6CtlPushes(w.ctx, conn, link.pushes)
	if err := w.handshakeOwnerCtl(conn); err != nil {
		return nil, err
	}
	return link, nil
}

func pumpM6CtlPushes(ctx context.Context, conn *peer.Connection, pushes chan<- json.RawMessage) {
	for {
		raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case pushes <- raw:
		case <-ctx.Done():
			return
		}
	}
}

func (w *m6World) handshakeOwnerCtl(conn *peer.Connection) error {
	build := func(id int64) any {
		return proto.Hello{
			Request: proto.Request{Type: "hello", ID: id},
			V:       1,
			Kind:    "browser",
			Name:    "owner",
			Addrs:   []string{},
		}
	}
	raw, err := conn.Request(w.ctx, build)
	if err != nil {
		return err
	}
	var welcome proto.Welcome
	if err := proto.Decode(raw, &welcome); err != nil || !welcome.Owner {
		return fmt.Errorf("welcome = %s (%v)", raw, err)
	}
	return nil
}

func (w *m6World) createAndSyncGroup() error {
	w.groupID = w.nextID()
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{
				Type:      "group",
				Nonce:     w.groupID,
				Timestamp: time.Now().UnixMilli(),
				Group:     w.groupID,
			},
			Op: "create",
		},
		Name:       m6GroupName,
		Kind:       "project",
		Repo:       w.repoDir,
		Base:       "main",
		Forge:      "none",
		Checks:     []string{},
		MemberName: m6OwnerName,
	}
	if err := w.sendSignedCmd(w.homeLink.conn, create); err != nil {
		return err
	}
	return w.syncGroupOnHome()
}

func (w *m6World) syncGroupOnHome() error {
	build := func(id int64) any {
		return proto.Sync{
			Request:   proto.Request{Type: "sync", ID: id},
			Group:     string(w.groupID),
			Since:     0,
			StateOnly: false,
		}
	}
	raw, err := w.homeLink.conn.Request(w.ctx, build)
	if err != nil {
		return err
	}
	var synced proto.Synced
	if err := proto.Decode(raw, &synced); err != nil {
		return fmt.Errorf("sync group = %s (%v)", raw, err)
	}
	return nil
}

func (w *m6World) sendSignedCmd(conn *peer.Connection, payload any) error {
	env := w.ownerKey.Sign(string(m6MarshalJSON(payload)))
	build := func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	}
	raw, err := conn.Request(w.ctx, build)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("cmd failed: %s (%v)", raw, err)
	}
	return nil
}

func (w *m6World) givenTwoTurnScript(agent, m1, a1, m2, a2, member string) error {
	turn1 := formatM6MentionTurn(member, m1)
	turn2 := formatM6MentionTurn(member, m2)
	var body []byte
	if agent == "claude" {
		body = buildM6ClaudeScript(turn1, a1, turn2, a2)
	} else {
		body = buildM6CodexScript(turn1, a1, turn2, a2)
	}
	return os.WriteFile(filepath.Join(w.root, agent+".json"), body, 0o600)
}

func formatM6MentionTurn(member, body string) string {
	header := fmt.Sprintf("[SoloDawn] group %q · you are %q · group session", m6GroupName, member)
	block := fmt.Sprintf("== 1 of 1 · mention · from %q in main · instruction", m6OwnerName)
	return header + "\n" + block + "\n" + body
}

func buildM6ClaudeScript(turn1, ans1, turn2, ans2 string) []byte {
	steps := []map[string]any{
		m6ClaudeStdinStep(turn1),
		{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
		m6MCPInitStep("claude"),
		{"dir": "mcp->cli", "msg": map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{}}},
		{"dir": "mcp<-cli", "msg": map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}},
		m6MCPPostCallStep(1, ans1),
		m6MCPPostResultStep(1),
		{"dir": "stdout", "msg": map[string]any{"type": "result", "subtype": "success"}},
		m6ClaudeStdinStep(turn2),
		{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
		m6MCPPostCallStep(2, ans2),
		m6MCPPostResultStep(2),
		{"dir": "stdout", "msg": map[string]any{"type": "result", "subtype": "success"}},
		{"dir": "eof"},
	}
	return m6MarshalJSON(map[string]any{"steps": steps})
}

func m6ClaudeStdinStep(text string) map[string]any {
	content := []map[string]string{{"type": "text", "text": text}}
	return map[string]any{
		"dir": "stdin",
		"msg": map[string]any{
			"type":    "user",
			"message": map[string]any{"role": "user", "content": content},
		},
	}
}

func buildM6CodexScript(turn1, ans1, turn2, ans2 string) []byte {
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"id": 1, "method": "initialize"}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{}}},
		{"dir": "stdin", "msg": map[string]any{"method": "initialized"}},
		{"dir": "stdin", "msg": map[string]any{"id": 2, "method": "thread/start"}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"thread": map[string]any{"id": "$session"}}}},
		m6CodexTurnStartStep(3, turn1),
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": "turn-1"}}}},
		m6MCPInitStep("codex"),
		{"dir": "mcp->cli", "msg": map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{}}},
		{"dir": "mcp<-cli", "msg": map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}},
		m6MCPPostCallStep(1, ans1),
		m6MCPPostResultStep(1),
		m6CodexTurnDoneStep(),
		m6CodexTurnStartStep(4, turn2),
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": "turn-2"}}}},
		m6MCPPostCallStep(2, ans2),
		m6MCPPostResultStep(2),
		m6CodexTurnDoneStep(),
		{"dir": "eof"},
	}
	return m6MarshalJSON(map[string]any{"session": "thr-m6-codex-1", "steps": steps})
}

func m6CodexTurnStartStep(id int, text string) map[string]any {
	input := []map[string]string{{"type": "text", "text": text}}
	return map[string]any{
		"dir": "stdin",
		"msg": map[string]any{
			"id":     id,
			"method": "turn/start",
			"params": map[string]any{"threadId": "$session", "input": input},
		},
	}
}

func m6CodexTurnDoneStep() map[string]any {
	return map[string]any{
		"dir": "stdout",
		"msg": map[string]any{
			"method": "turn/completed",
			"params": map[string]any{"turn": map[string]any{"status": "completed", "error": nil}},
		},
	}
}

func m6MCPInitStep(name string) map[string]any {
	return map[string]any{
		"dir": "mcp<-cli",
		"msg": map[string]any{
			"jsonrpc": "2.0",
			"id":      0,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]string{"name": name, "version": "1"},
			},
		},
	}
}

func m6MCPPostCallStep(id int, text string) map[string]any {
	return map[string]any{
		"dir": "mcp<-cli",
		"msg": map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "post",
				"arguments": map[string]string{"text": text},
			},
		},
	}
}

func m6MCPPostResultStep(id int) map[string]any {
	return map[string]any{
		"dir": "mcp->cli",
		"msg": map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  map[string]any{"isError": false},
		},
	}
}

func (w *m6World) givenJoinedRemoteAI(agent, memberName string) error {
	setCmd := proto.AgentSet{
		HostCommand: proto.HostCommand{
			Type:      "agent.set",
			Nonce:     w.nextID(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.aiNode.key.ID(),
		},
		Agent:   agent,
		Account: "personal",
	}
	if err := w.sendSignedCmd(w.aiLink.conn, setCmd); err != nil {
		return err
	}
	joinCmd := proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type:      "host.join",
			Nonce:     w.nextID(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.aiNode.key.ID(),
		},
		Group:     w.groupID,
		Home:      w.homeNode.key.ID(),
		Agent:     agent,
		Name:      memberName,
		Secretary: false,
	}
	if err := w.sendSignedCmd(w.aiLink.conn, joinCmd); err != nil {
		return err
	}
	return w.awaitJoinedMember(agent, memberName)
}

func (w *m6World) awaitJoinedMember(agent, memberName string) error {
	match := func(raw json.RawMessage) (bool, error) {
		var ai proto.AIMemberRow
		if proto.Decode(raw, &ai) != nil || ai.Kind != "ai" || ai.Name != memberName {
			return false, nil
		}
		return w.verifyJoinedRow(ai, agent)
	}
	return w.readHomeRowsUntil(match)
}

func (w *m6World) verifyJoinedRow(ai proto.AIMemberRow, agent string) (bool, error) {
	if ai.CLI != agent || ai.Host != w.aiNode.key.ID() || ai.Host == w.homeNode.key.ID() {
		return false, fmt.Errorf("unexpected AI member row: %+v", ai)
	}
	w.aiMemberID = ai.ID
	return true, nil
}

func (w *m6World) whenOwnerMentionsAI(_ string, text string) error {
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     w.nextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.groupID,
		},
		Kind:     "text",
		Thread:   nil,
		Main:     false,
		Text:     text,
		Mentions: []proto.ID{proto.ID(w.aiMemberID)},
		Re:       nil,
		Body:     proto.EmptyBody{},
	}
	return w.sendSignedCmd(w.homeLink.conn, post)
}

func (w *m6World) thenAIPostsAndWaits(_ string, wantText string) error {
	var sawAnswer bool
	match := func(raw json.RawMessage) (bool, error) {
		if !sawAnswer {
			sawAnswer = w.matchesNewAIPost(raw, wantText)
			return false, nil
		}
		return w.matchesWaitingAI(raw), nil
	}
	return w.readHomeRowsUntil(match)
}

func (w *m6World) matchesNewAIPost(raw json.RawMessage, wantText string) bool {
	var msg proto.MsgRow
	if proto.Decode(raw, &msg) != nil || w.seenMsgs[msg.ID] {
		return false
	}
	if msg.By != w.aiMemberID || msg.Kind != "text" || msg.Text != wantText {
		return false
	}
	w.seenMsgs[msg.ID] = true
	return true
}

func (w *m6World) matchesWaitingAI(raw json.RawMessage) bool {
	var ai proto.AIMemberRow
	if proto.Decode(raw, &ai) != nil || ai.ID != w.aiMemberID {
		return false
	}
	return ai.Host == w.aiNode.key.ID() && ai.State == "waiting"
}

func (w *m6World) readHomeRowsUntil(match func(json.RawMessage) (bool, error)) error {
	for {
		rawFrame, err := w.nextHomePush()
		if err != nil {
			return err
		}
		done, err := w.scanRowsFrame(rawFrame, match)
		if err != nil || done {
			return err
		}
	}
}

func (w *m6World) nextHomePush() (json.RawMessage, error) {
	select {
	case raw := <-w.homeLink.pushes:
		return raw, nil
	case <-w.ctx.Done():
		return nil, fmt.Errorf("timed out waiting for home push: %w\nhome=%s\nai=%s", w.ctx.Err(), w.homeNode.out.String(), w.aiNode.out.String())
	}
}

func (w *m6World) scanRowsFrame(rawFrame json.RawMessage, match func(json.RawMessage) (bool, error)) (bool, error) {
	var batch proto.Rows
	if proto.Decode(rawFrame, &batch) != nil || batch.Group != string(w.groupID) {
		return false, nil
	}
	for _, row := range batch.Rows {
		done, err := match(row)
		if err != nil || done {
			return done, err
		}
	}
	return false, nil
}

func (w *m6World) thenOneCLIProcessRan(agent string) error {
	sessionID, err := readSingleStartLine(w.cliLogPath, agent)
	if err != nil {
		return err
	}
	w.aiNode.stop()
	return waitSingleStartExitLog(w.cliLogPath, agent, sessionID)
}

func readSingleStartLine(path, agent string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		return "", fmt.Errorf("expected one live start line before shutdown, got %q", string(raw))
	}
	prefix := "start " + agent + " "
	sessionID, ok := strings.CutPrefix(lines[0], prefix)
	if !ok || sessionID == "" {
		return "", fmt.Errorf("unexpected start line %q for %s", lines[0], agent)
	}
	return sessionID, nil
}

func waitSingleStartExitLog(path, agent, sessionID string) error {
	want := fmt.Sprintf("start %s %s\nexit %s %s\n", agent, sessionID, agent, sessionID)
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && string(raw) == want {
			return nil
		}
		got = string(raw)
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("lifecycle log = %q, want %q", got, want)
}

func m6NewKey() identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}

func m6MarshalJSON(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
