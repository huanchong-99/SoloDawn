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
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
)

func setScenarioEnv(cleanup func(func()), key, val string) error {
	old, had := os.LookupEnv(key)
	if err := os.Setenv(key, val); err != nil {
		return err
	}
	cleanup(func() { restoreVar(key, old, had) })
	return nil
}

func startTestTURN(cleanup func(func()), gen turn.RelayAddressGenerator) (string, error) {
	pkt, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:         "test",
		LoggerFactory: pionLog,
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			return "peer", turn.GenerateAuthKey("peer", "test", "secret"), r.Username == "peer"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: pkt, RelayAddressGenerator: gen}},
	})
	if err != nil {
		_ = pkt.Close()
		return "", err
	}
	cleanup(func() { _ = srv.Close() })
	return pkt.LocalAddr().String(), nil
}

func testTURNServer(addr string) proto.ICEServer {
	return proto.ICEServer{URLs: []string{"turn:" + addr + "?transport=udp"}, Username: "peer", Credential: "secret"}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var (
	activeCleanupsMu sync.Mutex
	activeCleanups   []activeCleanupEntry
	nextCleanupID    uint64
	deadlineFired    bool
)

type activeCleanupEntry struct {
	id uint64
	fn func()
}

func armDeadlineCleanups(t *testing.T) func() {
	activeCleanupsMu.Lock()
	deadlineFired = false
	activeCleanupsMu.Unlock()
	d, ok := t.Deadline()
	if !ok {
		return func() {}
	}
	rem := time.Until(d)
	lead := min(30*time.Second, max(800*time.Millisecond, rem/6))
	timer := time.AfterFunc(max(0, rem-lead), runDeadlineCleanups)
	return func() {
		timer.Stop()
		activeCleanupsMu.Lock()
		deadlineFired = false
		activeCleanupsMu.Unlock()
	}
}

func runDeadlineCleanups() {
	activeCleanupsMu.Lock()
	deadlineFired = true
	pending := slices.Clone(activeCleanups)
	activeCleanups = nil
	activeCleanupsMu.Unlock()
	for _, entry := range slices.Backward(pending) {
		entry.fn()
	}
}

func isDeadlineCleanup() bool {
	activeCleanupsMu.Lock()
	defer activeCleanupsMu.Unlock()
	return deadlineFired
}

func trackOrRunCleanup(idOut *uint64, fn func()) bool {
	activeCleanupsMu.Lock()
	defer activeCleanupsMu.Unlock()
	if deadlineFired {
		return true
	}
	nextCleanupID++
	*idOut = nextCleanupID
	activeCleanups = append(activeCleanups, activeCleanupEntry{id: *idOut, fn: fn})
	return false
}

func removeActiveCleanup(id uint64) {
	activeCleanupsMu.Lock()
	defer activeCleanupsMu.Unlock()
	activeCleanups = slices.DeleteFunc(activeCleanups, func(e activeCleanupEntry) bool {
		return e.id == id
	})
}

func registerBothCleanups(t testing.TB, cleanup func(func()), fn func()) {
	var id uint64
	once := sync.OnceFunc(func() {
		fn()
		removeActiveCleanup(id)
	})
	if trackOrRunCleanup(&id, once) {
		once()
		return
	}
	if cleanup != nil {
		cleanup(once)
	}
	if t != nil {
		t.Cleanup(once)
	}
}

func startManagedCmd(t testing.TB, cleanup func(func()), cmd *exec.Cmd) (func(), error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	sigProc, killProc := attachManagedProc(cmd)
	done := make(chan struct{}) // single reader: stop closure below
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	stop := sync.OnceFunc(func() {
		if sigProc(os.Interrupt) != nil {
			killProc()
		}
		waitOrKillProc(done, killProc)
	})
	registerBothCleanups(t, cleanup, stop)
	return stop, nil
}

func killManagedCmd(t testing.TB, cleanup func(func()), cmd *exec.Cmd, extra func()) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	_, killProc := attachManagedProc(cmd)
	done := make(chan struct{}) // single reader: stop closure below
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	stop := sync.OnceFunc(func() {
		killProc()
		<-done
		if extra != nil {
			extra()
		}
	})
	registerBothCleanups(t, cleanup, stop)
	return nil
}

func waitOrKillProc(done <-chan struct{}, killProc func()) {
	grace := 3 * time.Second
	if isDeadlineCleanup() {
		grace = 300 * time.Millisecond
	}
	select {
	case <-done:
	case <-time.After(grace):
		killProc()
		<-done
	}
}

func cleanWindowsLoginTaskIfAbsent(t testing.TB, cleanup func(func())) {
	if runtime.GOOS != "windows" {
		return
	}
	if had, err := loginRegistration(""); err == nil && !had {
		registerBothCleanups(t, cleanup, func() { _ = node.RemoveService() })
	}
}

func shortTempRoot(t testing.TB, cleanup func(func()), prefix string) (string, error) {
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, prefix)
	if err != nil {
		return "", err
	}
	registerBothCleanups(t, cleanup, func() { _ = os.RemoveAll(root) })
	return root, nil
}

func sharedRVURL(ctx context.Context) string {
	if rv, ok := ctx.Value(rvKey{}).(*rvWorld); ok && rv != nil {
		return rv.ws
	}
	return ""
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func mustMarshalNoEscape(v any) []byte {
	raw, err := marshalNoEscape(v)
	if err != nil {
		panic(err)
	}
	return raw
}

type seqIDGen struct {
	seq uint64
}

func (g *seqIDGen) next() proto.ID {
	g.seq++
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], g.seq)
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

type homeFrameClient struct {
	ch  chan []byte // single reader: nextRaw (m2Browser) or drainBrowserUntil (m3World)
	saw *trail      // the frames sent, as frameBrief names them
}

func newHomeFrameClient(buf int) homeFrameClient {
	return homeFrameClient{ch: make(chan []byte, buf), saw: new(trail)}
}

func (c homeFrameClient) Done() <-chan struct{} { return nil }

func (c homeFrameClient) Send(frame any) error {
	raw, err := marshalNoEscape(frame)
	if err != nil {
		return err
	}
	c.saw.see(frameBrief(raw))
	c.ch <- raw
	return nil
}

func runGitIn(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w (%s)", args, err, out)
	}
	return nil
}

// initBareRepo makes dir a bare repository whose maintenance ends with the
// push that starts it, and does not go on in the folder when it is removed.
func initBareRepo(dir, base string) error {
	if err := runGitIn("", "init", "--bare", "-b", base, dir); err != nil {
		return err
	}
	return runGitIn(dir, "config", "gc.autoDetach", "false")
}

func initLocalGitRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := runGitIn(dir, "init", "-b", "main"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o600); err != nil {
		return err
	}
	if err := runGitIn(dir, "add", "README.md"); err != nil {
		return err
	}
	return runGitIn(dir, "-c", "user.name=Owner", "-c", "user.email=owner@example.com", "commit", "-m", "init")
}

func claudeInitMCP(startID int) []map[string]any {
	return []map[string]any{
		{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": startID, "method": "initialize",
			"params": map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "claude-code", "version": "1"}},
		}},
		{"dir": "mcp->cli", "msg": map[string]any{"jsonrpc": "2.0", "id": startID, "result": map[string]any{}}},
		{"dir": "mcp<-cli", "msg": map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}},
	}
}

func mcpPostSteps(callID int, text string) []map[string]any {
	return mcpCallSteps(callID, "post", map[string]any{"text": text}, map[string]any{"isError": false})
}

func mcpCallSteps(callID int, name string, args, result map[string]any) []map[string]any {
	return []map[string]any{
		{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": callID, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		}},
		{"dir": "mcp->cli", "msg": map[string]any{"jsonrpc": "2.0", "id": callID, "result": result}},
	}
}

func claudeUserStdinStep(stdinText string) map[string]any {
	msg := map[string]any{"role": "user"}
	if stdinText != "" {
		msg["content"] = []map[string]string{{"type": "text", "text": stdinText}}
	}
	return map[string]any{"dir": "stdin", "msg": map[string]any{"type": "user", "message": msg}}
}

func claudeTurnSteps(callID int, first bool, stdinText, postText, outText string) []map[string]any {
	var body []map[string]any
	if postText != "" {
		body = mcpPostSteps(callID, postText)
	}
	return claudeToolTurn(first, stdinText, outText, body)
}

func claudeToolTurn(first bool, stdinText, outText string, body []map[string]any) []map[string]any {
	steps := []map[string]any{claudeUserStdinStep(stdinText)}
	if first {
		steps = append(steps, claudeInitMCP(0)...)
		steps = append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}})
	}
	if outText != "" {
		steps = append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{
			"type": "assistant", "session_id": "$session",
			"message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": outText}}},
		}})
	}
	steps = append(steps, body...)
	res := map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": "$session", "result": outText}
	return append(steps, map[string]any{"dir": "stdout", "msg": res})
}

func codexStartupSteps() []map[string]any {
	return codexThreadSteps(map[string]any{"method": "thread/start"})
}

func codexThreadSteps(thread map[string]any) []map[string]any {
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"method": "initialize"}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{}}},
		{"dir": "stdin", "msg": map[string]any{"method": "initialized"}},
		{"dir": "stdin", "msg": thread},
	}
	steps = append(steps, claudeInitMCP(0)...)
	return append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"thread": map[string]any{"id": "$session"}}}})
}

func codexTurnSteps(turnID string, callID int, stdinText, postText string, askApproval bool) []map[string]any {
	var body []map[string]any
	if askApproval {
		body = codexApprovalSteps()
	}
	return codexToolTurn(turnID, stdinText, append(body, mcpPostSteps(callID, postText)...))
}

func codexToolTurn(turnID, stdinText string, body []map[string]any) []map[string]any {
	params := map[string]any{"threadId": "$session"}
	if stdinText != "" {
		params["input"] = []map[string]string{{"type": "text", "text": stdinText}}
	}
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"method": "turn/start", "params": params}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}}},
	}
	steps = append(steps, body...)
	return append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": "$session", "turn": map[string]any{"id": turnID, "status": "completed", "error": nil}},
	}})
}

func codexApprovalSteps() []map[string]any {
	return []map[string]any{
		{"dir": "stdout", "msg": map[string]any{
			"id": 99, "method": "item/commandExecution/requestApproval",
			"params": map[string]any{"command": "echo hold-slot", "cwd": "/tmp", "reason": "hold slot"},
		}},
		{"dir": "stdin", "msg": map[string]any{"id": 99, "result": map[string]any{"decision": "accept"}}},
	}
}

type peerNodeConfig struct {
	Ctx           context.Context
	Dir           string
	Key           identity.Key
	Name          string
	Version       string
	Owner         identity.Key
	ExtraBrowsers []identity.Key
	Rendezvous    string
	Now           func() time.Time
	MapRoute      func(peer.Route) peer.Route
	OnInbound     func(*peer.Connection)
}

type peerNode struct {
	Key      identity.Key
	Store    *store.Store
	Logs     *node.Logs
	Home     *home.Loop
	Host     *host.Host
	Link     *node.LocalLink
	Sessions *node.Sessions
	Surface  *node.Surface
	Pairer   *node.Pairer
	Sockets  *peer.Sockets
	Peer     *peer.Peer
	Path     *peer.Path
	once     sync.Once
	cancel   context.CancelFunc
	serving  sync.WaitGroup
	mu       sync.Mutex
	closing  bool
}

func startPeerNode(t testing.TB, cleanup func(func()), cfg peerNodeConfig) (*peerNode, error) {
	cfg = withPeerNodeDefaults(cfg)
	key, db, err := openPeerNodeStore(cfg)
	if err != nil {
		return nil, err
	}
	if err := seedPeerNodeData(db, key, cfg); err != nil {
		db.Close()
		return nil, err
	}
	pn := &peerNode{Key: key, Store: db}
	if err := pn.initCore(cfg); err != nil {
		db.Close()
		return nil, err
	}
	if err := pn.initTransport(cfg); err != nil {
		pn.closeCore()
		return nil, err
	}
	registerBothCleanups(t, cleanup, pn.Close)
	return pn, nil
}

func (n *peerNode) closeCore() {
	n.Link.Close()
	n.Home.Close()
	n.Host.Settle()
	n.Store.Close()
}

func (n *peerNode) Close() {
	n.once.Do(func() {
		n.Path.Close()
		n.Pairer.Close()
		n.Sockets.Close()
		n.mu.Lock()
		n.closing = true
		n.mu.Unlock()
		n.cancel()
		n.serving.Wait()
		n.closeCore()
	})
}

func withPeerNodeDefaults(cfg peerNodeConfig) peerNodeConfig {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.Name == "" {
		cfg.Name = "node"
	}
	if cfg.MapRoute == nil {
		cfg.MapRoute = func(r peer.Route) peer.Route { return r }
	}
	return cfg
}

func openPeerNodeStore(cfg peerNodeConfig) (identity.Key, *store.Store, error) {
	if cfg.Dir == "" {
		key := cfg.Key
		if key == nil {
			key = newRandomPeerKey()
		}
		db, err := store.Open(":memory:")
		return key, db, err
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, nil, err
	}
	key, err := resolvePeerNodeKey(cfg.Dir, cfg.Key)
	if err != nil {
		return nil, nil, err
	}
	db, err := store.Open(filepath.Join(cfg.Dir, "node.db"))
	return key, db, err
}

func resolvePeerNodeKey(dir string, key identity.Key) (identity.Key, error) {
	if key != nil {
		return key, nil
	}
	return identity.LoadKey(dir)
}

func newRandomPeerKey() identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}

func seedPeerNodeData(db *store.Store, nodeKey identity.Key, cfg peerNodeConfig) error {
	rawRV, _ := json.Marshal([]string{cfg.Rendezvous})
	writeRV := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES('rendezvous', ?)`, string(rawRV))
		return err
	}
	if err := db.Write(writeRV); err != nil || cfg.Owner == nil {
		return err
	}
	entries := buildPeerOwnerLog(cfg.Owner, nodeKey, cfg.Name, cfg.ExtraBrowsers, cfg.Now().UnixMilli())
	return seedPeerOwnerEntries(db, cfg.Owner.ID(), cfg.Name, entries)
}

func buildPeerOwnerLog(owner, nodeKey identity.Key, name string, browsers []identity.Key, ts int64) []proto.Envelope {
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(owner, owner.ID(), 1, "browser", "owner", owner.ID(), ts),
		nodetest.SignDeviceAdd(owner, owner.ID(), 2, "node", name, nodeKey.ID(), ts),
	}
	for i, key := range browsers {
		entries = append(entries, nodetest.SignDeviceAdd(owner, owner.ID(), len(entries)+1, "browser", fmt.Sprintf("browser-%d", i+2), key.ID(), ts))
	}
	return entries
}

func seedPeerOwnerEntries(db *store.Store, owner, name string, entries []proto.Envelope) error {
	ownerJSON, _ := json.Marshal(owner)
	nameJSON, _ := json.Marshal(name)
	write := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES('owner', ?), ('name', ?)`, string(ownerJSON), string(nameJSON))
		for i, env := range entries {
			raw, _ := json.Marshal(env)
			_, next := tx.Exec(`INSERT OR REPLACE INTO device_log(user, ver, env) VALUES(?, ?, ?)`, owner, i+1, string(raw))
			err = errors.Join(err, next)
		}
		return err
	}
	return db.Write(write)
}

func (n *peerNode) initCore(cfg peerNodeConfig) error {
	logs, err := node.OpenLogs(n.Store, n.Key.ID())
	if err != nil {
		return err
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: n.Store, Home: n.Key.ID(), OwnerFn: logs.Owner,
		Logs: logs.Log, Tables: home.NodeTables(), Invite: peerNodeInviter{n}, Now: cfg.Now,
	})
	if err != nil {
		return err
	}
	h, _, err := host.New(n.Store, n.Key, cfg.Now)
	if err != nil {
		loop.Close()
		return err
	}
	n.Logs, n.Home, n.Host, n.Link = logs, loop, h, node.AttachLocal(h, loop)
	nameFn := func() string { return node.ReadName(n.Store) }
	addrsFn := func() []string { return []string{cfg.Rendezvous} }
	n.Sessions = node.NewSessions(node.SessionConfig{
		ID: n.Key.ID(), Version: cfg.Version, Store: n.Store, Logs: logs,
		Home: loop, Host: h, Name: nameFn, Addrs: addrsFn, Now: cfg.Now,
	})
	n.Surface = node.NewSurface(node.SurfaceConfig{
		ID: n.Key.ID(), OS: runtime.GOOS + "/" + runtime.GOARCH, Version: cfg.Version,
		Store: n.Store, Sessions: n.Sessions, Host: h, Name: nameFn, Agents: h.Agents,
	})
	return nil
}

// peerNodeInviter gives the home loop the node's Pairer, which needs the loop's Sessions and so is built after it.
type peerNodeInviter struct{ n *peerNode }

func (i peerNodeInviter) Invite(group, by, signer, nonce string) (home.InviteLink, error) {
	return i.n.Pairer.Invite(group, by, signer, nonce)
}

func (i peerNodeInviter) ReplayInvite(signer, nonce string) (home.InviteLink, bool) {
	return i.n.Pairer.ReplayInvite(signer, nonce)
}

func (n *peerNode) initTransport(cfg peerNodeConfig) error {
	socks, err := peer.Listen(0, 0, "test")
	if err != nil {
		return err
	}
	rvPath, err := peer.Dial(cfg.Ctx, cfg.Rendezvous, n.Key)
	if err != nil {
		socks.Close()
		return err
	}
	ctx, cancel := context.WithCancel(cfg.Ctx)
	n.cancel = cancel
	n.Sockets, n.Path = socks, rvPath
	n.Pairer = node.NewPairer(node.PairConfig{
		Key: n.Key, Store: n.Store, Logs: n.Logs, Sessions: n.Sessions,
		Paths: func() []*peer.Path { return []*peer.Path{rvPath} },
		Now:   cfg.Now,
	})
	known := func(dev string) bool { return n.Logs.Known(dev) || n.Pairer.Known(dev) }
	routes := func(dev string) []peer.Route { return n.routesFor(dev, cfg.MapRoute) }
	inbound := func(c *peer.Connection) {
		if cfg.OnInbound != nil {
			cfg.OnInbound(c)
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		if !n.closing {
			n.serving.Go(func() { n.Pairer.Serve(ctx, c.Device, c) })
		}
	}
	channels := peer.Channels{"pty:": func(s *peer.Stream) { n.Host.ServeTerminal(s) }}
	n.Peer = peer.New(n.Key, socks, n.Store, known, routes, inbound, channels)
	n.Pairer.Peer = n.Peer
	event := func(kind string, data []byte) error {
		n.Pairer.HandleEvent(cfg.Ctx, rvPath, kind, data)
		return nil
	}
	go rvPath.Serve(cfg.Ctx, n.Peer, event)
	return nil
}

func (n *peerNode) routesFor(dev string, mapRoute func(peer.Route) peer.Route) []peer.Route {
	own := n.Path.Route()
	out := []peer.Route{mapRoute(own)}
	for _, r := range n.Pairer.Routes(dev) {
		if r.ID != own.ID {
			out = append(out, mapRoute(r))
		}
	}
	return out
}

func deviceFingerprint(id string) (string, error) {
	if !identity.IsDevice(id) {
		return "", fmt.Errorf("invalid device id %q", id)
	}
	pub, _ := base64.RawURLEncoding.DecodeString(id)
	raw := append(make([]byte, ed25519.SeedSize), pub...)
	return identity.Key(raw).Fingerprint(), nil
}

func waitConnPath(conn *peer.Connection, want string, limit time.Duration) error {
	if conn == nil {
		return errors.New("connection is nil")
	}
	w := newWait(limit, fmt.Sprintf("connection path %q", want), nil)
	defer w.stop()
	for conn.Path() != want {
		w.saw.read(conn.Path())
		if !waitConnPathSignal(conn, w.timer.C) {
			return w.fail()
		}
	}
	return nil
}

func waitConnPathSignal(conn *peer.Connection, timeout <-chan time.Time) bool {
	select {
	case <-conn.PathChanged():
		return true
	case <-conn.Done():
		return false
	case <-timeout:
		return false
	}
}

type peerRoomEvent struct {
	kind string
	data []byte
}

type peerBrowser struct {
	key        identity.Key
	path       *peer.Path
	peer       *peer.Peer
	presence   chan string        // single reader: waitOnline
	roomEvents chan peerRoomEvent // single reader: exchangeRoom
}

type peerBrowserConn struct {
	conn   *peer.Connection
	saw    trail
	pushes chan json.RawMessage // single reader: nextPush in scenario steps
}

func newPeerBrowser(ctx context.Context, cleanup func(func()), key identity.Key, rvURL string, mapRoute func(peer.Route) peer.Route) (*peerBrowser, error) {
	dialStart := time.Now()
	path, err := peer.Dial(ctx, rvURL, key)
	if err != nil {
		return nil, ctxCallErr(ctx, dialStart, "rendezvous dial", nil, err)
	}
	cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0, "test")
	if err != nil {
		return nil, err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return nil, err
	}
	cleanup(func() { socks.Close(); db.Close() })
	if mapRoute == nil {
		mapRoute = func(r peer.Route) peer.Route { return r }
	}
	routes := func(string) []peer.Route { return []peer.Route{mapRoute(path.Route())} }
	p := peer.New(key, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	b := &peerBrowser{
		key:        key,
		path:       path,
		peer:       p,
		presence:   make(chan string, 32),
		roomEvents: make(chan peerRoomEvent, 32),
	}
	go path.Serve(ctx, p, b.onEvent)
	return b, nil
}

func (b *peerBrowser) onEvent(kind string, data []byte) error {
	if kind == "presence" {
		b.recordPresence(data)
		return nil
	}
	if kind == "joined" || kind == "room" || kind == "full" {
		select {
		case b.roomEvents <- peerRoomEvent{kind: kind, data: slices.Clone(data)}:
		default:
		}
	}
	return nil
}

func (b *peerBrowser) recordPresence(data []byte) {
	var pr proto.Presence
	if proto.Decode(data, &pr) == nil && pr.On {
		select {
		case b.presence <- pr.Key:
		default:
		}
	}
}

func (b *peerBrowser) waitOnline(ctx context.Context, devices ...string) error {
	if err := b.path.Write(ctx, proto.Watch{Type: "watch", Keys: devices}); err != nil {
		return err
	}
	remaining := map[string]bool{}
	for _, dev := range devices {
		remaining[dev] = true
	}
	var saw trail
	start := time.Now()
	for len(remaining) > 0 {
		select {
		case k := <-b.presence:
			delete(remaining, k)
			saw.see(k)
		case <-ctx.Done():
			return ctxEnded(ctx, start, fmt.Sprintf("devices %v to come online", devices), &saw)
		}
	}
	return nil
}

func (b *peerBrowser) exchangeRoom(ctx context.Context, secret identity.Secret, myRole, wantRole string) (proto.Pair, error) {
	room, rawSelf := secret.Room(), b.pairSelf(secret, myRole)
	if err := b.path.Write(ctx, proto.RoomID{Type: "join", Room: room}); err != nil {
		return proto.Pair{}, err
	}
	return b.awaitRoomPeer(ctx, secret, room, wantRole, rawSelf)
}

func (b *peerBrowser) pairSelf(secret identity.Secret, role string) []byte {
	self := proto.Pair{Type: "pair", Key: b.key.ID(), Kind: "browser", Name: "Owner Browser", Role: role}
	self.MAC = secret.MAC(self)
	return mustMarshalNoEscape(self)
}

func (b *peerBrowser) awaitRoomPeer(ctx context.Context, secret identity.Secret, room, wantRole string, rawSelf []byte) (proto.Pair, error) {
	var saw trail
	start := time.Now()
	for {
		select {
		case ev := <-b.roomEvents:
			saw.see(ev.kind)
			m, ok, err := b.handleRoomEvent(ctx, ev, secret, room, wantRole, rawSelf)
			if err != nil || ok {
				return m, err
			}
		case <-ctx.Done():
			return proto.Pair{}, ctxEnded(ctx, start, "the pairing partner in the room", &saw)
		}
	}
}

func (b *peerBrowser) handleRoomEvent(ctx context.Context, ev peerRoomEvent, secret identity.Secret, room, wantRole string, rawSelf []byte) (proto.Pair, bool, error) {
	switch ev.kind {
	case "joined":
		return proto.Pair{}, false, b.sendRoomSelf(ctx, ev.data, room, rawSelf)
	case "full":
		return proto.Pair{}, false, errors.New("pairing room is full")
	case "room":
		m, ok := matchPeerRoomMsg(ev.data, secret, room, wantRole)
		return m, ok, nil
	default:
		return proto.Pair{}, false, nil
	}
}

func (b *peerBrowser) sendRoomSelf(ctx context.Context, data []byte, room string, rawSelf []byte) error {
	var id proto.RoomID
	if proto.Decode(data, &id) != nil || id.Room != room {
		return nil
	}
	return b.path.Write(ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: rawSelf})
}

func matchPeerRoomMsg(data []byte, secret identity.Secret, room, wantRole string) (proto.Pair, bool) {
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

func (b *peerBrowser) openCtl(ctx context.Context, cleanup func(func()), nodeID string) (*peerBrowserConn, error) {
	const tries, wait = 2, 8 * time.Second
	var saw trail
	for range tries {
		bc, ok, err := b.tryOpenCtl(ctx, cleanup, nodeID, wait, &saw)
		if err != nil || ok {
			return bc, err
		}
	}
	return nil, saw.timedOut(tries*wait, "ctl connection to "+nodeID)
}

func (b *peerBrowser) tryOpenCtl(ctx context.Context, cleanup func(func()), nodeID string, wait time.Duration, saw *trail) (*peerBrowserConn, bool, error) {
	offerStart := time.Now()
	conn, err := b.peer.Offer(ctx, nodeID)
	if err != nil {
		return nil, false, ctxCallErr(ctx, offerStart, "ctl offer to "+nodeID, saw, err)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-conn.Ready():
		cleanup(func() { conn.Close() })
		bc := &peerBrowserConn{conn: conn, pushes: make(chan json.RawMessage, 128)}
		go pumpBrowserPushes(ctx, conn, bc.pushes)
		return bc, true, nil
	case <-conn.Done():
		saw.see("connection ended before it was ready")
		conn.Close()
		return nil, false, nil
	case <-timer.C:
		saw.see("not ready after " + wait.String())
		conn.Close()
		return nil, false, nil
	case <-ctx.Done():
		conn.Close()
		return nil, false, ctxCallErr(ctx, offerStart, "ctl connection to "+nodeID, saw, ctx.Err())
	}
}

func pumpBrowserPushes(ctx context.Context, conn *peer.Connection, pushes chan<- json.RawMessage) {
	defer close(pushes)
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case pushes <- msg:
		case <-ctx.Done():
			return
		}
	}
}

func (c *peerBrowserConn) request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	return requestWait(ctx, c.conn, &c.saw, build)
}

func (c *peerBrowserConn) nextPush(ctx context.Context) (json.RawMessage, error) {
	msg, err := recvCtx(ctx, c.pushes, "a push on ctl", &c.saw)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, errors.New("ctl connection closed before push")
	}
	c.saw.see(frameBrief(msg))
	return msg, nil
}

func requestBrowserHello(ctx context.Context, conn *peer.Connection) (proto.Welcome, error) {
	raw, err := requestWait(ctx, conn, nil, func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Owner Browser", Addrs: []string{}}
	})
	if err != nil {
		return proto.Welcome{}, err
	}
	var welc proto.Welcome
	return welc, proto.Decode(raw, &welc)
}

func requestHostInfo(ctx context.Context, conn *peer.Connection) (proto.HostInfo, error) {
	raw, err := requestWait(ctx, conn, nil, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return proto.HostInfo{}, err
	}
	var info proto.HostInfo
	return info, proto.Decode(raw, &info)
}

func checkOwnerWelcome(ctx context.Context, conn *peer.Connection, wantDevice string) error {
	w, err := requestBrowserHello(ctx, conn)
	if err != nil {
		return err
	}
	if !w.Owner || w.Device != wantDevice {
		return fmt.Errorf("welcome = %+v, want owner true for %q", w, wantDevice)
	}
	return nil
}

func isNonRelayICECandidate(payload string) bool {
	var sig struct {
		Type string `json:"t"`
		Cand string `json:"cand"`
	}
	if json.Unmarshal([]byte(payload), &sig) != nil || sig.Type != "ice" {
		return false
	}
	return !strings.Contains(sig.Cand, " typ relay")
}

func rewriteDirectICE(servers []proto.ICEServer) []proto.ICEServer {
	var out []proto.ICEServer
	for _, s := range servers {
		if urls := rewriteDirectURLs(s.URLs); len(urls) > 0 {
			cp := s
			cp.URLs = urls
			out = append(out, cp)
		}
	}
	return out
}

func rewriteDirectURLs(urls []string) []string {
	var out []string
	for _, u := range urls {
		if strings.HasPrefix(u, "stun:") {
			out = append(out, strings.Replace(u, "localhost:", "127.0.0.1:", 1))
		}
	}
	return out
}

// TestM3MCPShim runs host.RunShim when invoked as a child MCP server process
// with the socket path and --token arguments after "--". During a regular go
// test run flag.Args() is empty and the test returns at once.
func TestM3MCPShim(_ *testing.T) {
	args := flag.Args()
	if len(args) == 0 {
		return
	}
	dial := func() (net.Conn, error) {
		return net.Dial("unix", args[0])
	}
	os.Exit(host.RunShim(args[1:], dial, os.Stdin, os.Stdout, os.Stderr))
}

func attachMCPSocket(t testing.TB, cleanup func(func()), h *host.Host, dir string) (string, func(), error) {
	exe, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	local, err := node.ListenLocal(dir)
	if err != nil {
		return "", nil, err
	}
	sockPath := filepath.Join(dir, "sock")
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go serveTestMCP(ctx, local, h, &wg)
	stop := sync.OnceFunc(func() {
		cancel()
		local.Close()
		wg.Wait()
	})
	registerBothCleanups(t, cleanup, stop)
	h.Shim = []string{exe, "-test.run=^TestM3MCPShim$", "--", sockPath}
	return sockPath, stop, nil
}

func serveTestMCP(ctx context.Context, local *node.Local, h *host.Host, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		conn, err := local.Accept()
		if err != nil {
			return
		}
		wg.Add(1)
		go handleTestMCPConn(ctx, conn, h, wg)
	}
}

func handleTestMCPConn(ctx context.Context, conn net.Conn, h *host.Host, wg *sync.WaitGroup) {
	defer wg.Done()
	open, _, r, err := node.AcceptOpening(conn)
	if err != nil {
		return
	}
	if open.MCP == "" {
		conn.Close()
		return
	}
	h.ServeMCP(ctx, open.MCP, r, conn)
}

func decodeReplyMatch(raw json.RawMessage, reqID int64) (bool, error) {
	var hdr struct {
		Re *int64 `json:"re,omitempty"`
	}
	if proto.Decode(raw, &hdr) != nil || hdr.Re == nil || *hdr.Re != reqID {
		return false, nil
	}
	var wireErr proto.ErrorResponse
	if proto.Decode(raw, &wireErr) == nil {
		return true, fmt.Errorf("%s: %s", wireErr.Err.Code, wireErr.Err.Message)
	}
	return true, nil
}

func decodeReplyCode(raw json.RawMessage, reqID int64) (bool, string) {
	matched, err := decodeReplyMatch(raw, reqID)
	if !matched || err == nil {
		return matched, ""
	}
	code, _, _ := strings.Cut(err.Error(), ":")
	return true, code
}

type homeViewEvent struct {
	kind   string
	member string
	state  string
	text   string
}

type homeView struct {
	group    proto.GroupRow
	aiByName map[string]proto.AIMemberRow
	ownerMem string
	tasks    map[string]proto.TaskRow
	msgs     map[string]proto.MsgRow
	msgOrder []string
	events   []homeViewEvent
}

func newHomeView() homeView {
	return homeView{
		aiByName: map[string]proto.AIMemberRow{},
		tasks:    map[string]proto.TaskRow{},
		msgs:     map[string]proto.MsgRow{},
	}
}

func applyRowsFrame(v *homeView, raw json.RawMessage) {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Type != "rows" {
		return
	}
	for _, elem := range batch.Rows {
		recordViewRow(v, elem)
	}
}

func recordViewRow(v *homeView, elem json.RawMessage) {
	var hdr proto.RowHeader
	if proto.Decode(elem, &hdr) != nil {
		return
	}
	if hdr.Type == "msg" {
		recordViewMsg(v, elem)
		return
	}
	recordViewStateRow(v, hdr.Type, elem)
}

func recordViewStateRow(v *homeView, kind string, elem json.RawMessage) {
	switch kind {
	case "group":
		recordViewGroup(v, elem)
	case "member":
		recordViewMember(v, elem)
	case "task":
		recordViewTask(v, elem)
	}
}

func recordViewGroup(v *homeView, elem json.RawMessage) {
	var grp proto.GroupRow
	if proto.Decode(elem, &grp) == nil {
		v.group = grp
	}
}

func recordViewMember(v *homeView, elem json.RawMessage) {
	var ai proto.AIMemberRow
	if proto.Decode(elem, &ai) == nil && ai.Kind == "ai" {
		v.aiByName[ai.Name] = ai
		v.events = append(v.events, homeViewEvent{kind: "member", member: ai.Name, state: ai.State})
		return
	}
	var hum proto.HumanMemberRow
	if proto.Decode(elem, &hum) == nil && hum.Kind == "human" {
		v.ownerMem = hum.ID
	}
}

func recordViewTask(v *homeView, elem json.RawMessage) {
	var t proto.TaskRow
	if proto.Decode(elem, &t) == nil {
		v.tasks[t.ID] = t
	}
}

// noConfirmCardAfter fails a scenario whose one-person group shows a confirm
// card (A9.40). msgs is read after the last step.
func noConfirmCardAfter(sc *godog.ScenarioContext, msgs func() map[string]proto.MsgRow) {
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		for id, m := range msgs() {
			if m.Kind == "confirm" {
				return ctx, fmt.Errorf("a one-person group has the confirm card %s: %s", id, m.Body)
			}
		}
		return ctx, nil
	})
}

func recordViewMsg(v *homeView, elem json.RawMessage) {
	var m proto.MsgRow
	if proto.Decode(elem, &m) != nil {
		return
	}
	if _, seen := v.msgs[m.ID]; !seen {
		v.msgOrder = append(v.msgOrder, m.ID)
		v.events = append(v.events, homeViewEvent{kind: "msg", member: viewAuthorName(v, m.By), text: m.Text})
	}
	v.msgs[m.ID] = m
}

func viewAuthorName(v *homeView, id string) string {
	for name, ai := range v.aiByName {
		if ai.ID == id {
			return name
		}
	}
	return id
}

func viewHasMsgFrom(v *homeView, by, want string) bool {
	for _, m := range v.msgs {
		if m.By == by && m.Text == want {
			return true
		}
	}
	return false
}

func viewHasInterruptedMsg(v *homeView, aiID, wantReason string) bool {
	for _, m := range v.msgs {
		if isInterruptedMsg(m, aiID, wantReason) {
			return true
		}
	}
	return false
}

func isInterruptedMsg(m proto.MsgRow, aiID, wantReason string) bool {
	if m.Kind != "system" {
		return false
	}
	var body proto.TurnInterruptedBody
	if proto.Decode(m.Body, &body) != nil {
		return false
	}
	return body.Code == "turn_interrupted" && body.AI == aiID && body.Reason == wantReason
}

func writeClusterScript(tb testing.TB, cluster *nodetest.Cluster, root string, seq *int, vendor string, doc map[string]any) error {
	*seq++
	path := filepath.Join(root, fmt.Sprintf("%s-%d.json", vendor, *seq))
	data, err := marshalNoEscape(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	cluster.SetScript(tb, vendor, path)
	return nil
}

func newHostCmd(ids *seqIDGen, to, typ string) proto.HostCommand {
	return proto.HostCommand{
		Type: typ, Nonce: ids.next(), Timestamp: time.Now().UnixMilli(), To: to,
	}
}

func callHostSurface(h *host.Host, from host.Owner, by, name string, payload any) error {
	raw, err := marshalNoEscape(payload)
	if err != nil {
		return err
	}
	_, err = host.Surface[name](context.Background(), h, from, by, raw)
	return err
}

func configureHostAI(h *host.Host, from host.Owner, ids *seqIDGen, ownerID, homeID string, groupID proto.ID, aiName, agent string) error {
	if _, err := h.Agents(context.Background(), false); err != nil {
		return err
	}
	setCmd := proto.AgentSet{
		HostCommand: newHostCmd(ids, h.Key.ID(), "agent.set"),
		Agent:       agent,
		Account:     "personal",
	}
	if err := callHostSurface(h, from, ownerID, "agent.set", setCmd); err != nil {
		return err
	}
	joinCmd := proto.HostJoin{
		HostCommand: newHostCmd(ids, h.Key.ID(), "host.join"),
		Group:       groupID,
		Home:        homeID,
		Agent:       agent,
		Name:        aiName,
	}
	return callHostSurface(h, from, ownerID, "host.join", joinCmd)
}

func awaitHostGroupSynced(h *host.Host, gid string) error {
	return waitUntil(5*time.Second, "host group sync of "+gid, func() error {
		if _, ok := h.Rows.Get(gid, "group", gid); !ok {
			return errors.New("no group row")
		}
		return nil
	})
}

func hasNonDormant(h *host.Host) bool {
	isOpen := func(s host.Session) bool { return s.State != host.Dormant }
	return slices.ContainsFunc(h.Sessions.All(), isOpen)
}

func waitHostDormant(h *host.Host) {
	_ = waitUntil(5*time.Second, "every host session dormant", func() error { return sessionsDormant(h) })
}

func sessionsDormant(h *host.Host) error {
	var open []string
	for _, s := range h.Sessions.All() {
		if s.State != host.Dormant {
			open = append(open, fmt.Sprintf("%v=%s", s.SessionKey, s.State))
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("not dormant: %s", strings.Join(open, " "))
	}
	return nil
}

func isSettledMemberState(state string) bool {
	return state == "dormant" || state == "offline"
}

func allMembersSettled(h *host.Host, ais map[string]proto.AIMemberRow) bool {
	for _, ai := range ais {
		if !isSettledMemberState(ai.State) {
			return false
		}
	}
	return !hasNonDormant(h)
}

func readFakeCLILog(logPath string) []string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func matchingFakeCLILogs(logPath, prefix string) []string {
	var out []string
	for _, line := range readFakeCLILog(logPath) {
		if strings.HasPrefix(line, prefix) {
			out = append(out, line)
		}
	}
	return out
}

func waitFakeCLILogs(logPath, prefix string, want int, timeout time.Duration) ([]string, error) {
	var got []string
	err := waitUntil(timeout, fmt.Sprintf("%d %q fake CLI log lines", want, prefix), func() error {
		if got = matchingFakeCLILogs(logPath, prefix); len(got) < want {
			return fmt.Errorf("%d: %v", len(got), readFakeCLILog(logPath))
		}
		return nil
	})
	return got, err
}

func homeInboxCount(db *store.Store) (int, error) {
	var count int
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM inbox`).Scan(&count)
	}
	return count, db.Write(read)
}

func awaitHomeInboxCount(db *store.Store, want int) error {
	return waitUntil(5*time.Second, fmt.Sprintf("home inbox count %d", want), func() error {
		n, err := homeInboxCount(db)
		if err != nil {
			return abort(err)
		}
		if n != want {
			return fmt.Errorf("count %d", n)
		}
		return nil
	})
}
