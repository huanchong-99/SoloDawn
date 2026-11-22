//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

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

func startManagedCmd(t testing.TB, cleanup func(func()), cmd *exec.Cmd) (func(), error) {
	prepareManagedCmd(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	sigGroup, killGroup := attachManagedGroup(cmd)
	done := make(chan struct{}) // single reader: stop closure below
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	stop := sync.OnceFunc(func() {
		if sigGroup(os.Interrupt) != nil {
			killGroup()
		}
		waitOrKillGroup(done, killGroup)
	})
	registerBothCleanups(t, cleanup, stop)
	return stop, nil
}

func killManagedCmd(t testing.TB, cleanup func(func()), cmd *exec.Cmd, extra func()) error {
	prepareManagedCmd(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	_, killGroup := attachManagedGroup(cmd)
	done := make(chan struct{}) // single reader: stop closure below
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	stop := sync.OnceFunc(func() {
		killGroup()
		<-done
		if extra != nil {
			extra()
		}
	})
	registerBothCleanups(t, cleanup, stop)
	return nil
}

func waitOrKillGroup(done <-chan struct{}, killGroup func()) {
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		killGroup()
		<-done
	}
}

func registerBothCleanups(t testing.TB, cleanup func(func()), fn func()) {
	once := sync.OnceFunc(fn)
	if cleanup != nil {
		cleanup(once)
	}
	if t != nil {
		t.Cleanup(once)
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
	ch chan []byte // single reader: nextRaw (m2Browser) or drainBrowserUntil (m3World)
}

func newHomeFrameClient(buf int) homeFrameClient {
	return homeFrameClient{ch: make(chan []byte, buf)}
}

func (c homeFrameClient) Done() <-chan struct{} { return nil }

func (c homeFrameClient) Send(frame any) error {
	raw, err := marshalNoEscape(frame)
	if err != nil {
		return err
	}
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
	return []map[string]any{
		{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": callID, "method": "tools/call",
			"params": map[string]any{"name": "post", "arguments": map[string]any{"text": text}},
		}},
		{"dir": "mcp->cli", "msg": map[string]any{"jsonrpc": "2.0", "id": callID, "result": map[string]any{"isError": false}}},
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
	if postText != "" {
		steps = append(steps, mcpPostSteps(callID, postText)...)
	}
	res := map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": "$session", "result": outText}
	return append(steps, map[string]any{"dir": "stdout", "msg": res})
}

func codexStartupSteps() []map[string]any {
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"method": "initialize"}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{}}},
		{"dir": "stdin", "msg": map[string]any{"method": "initialized"}},
		{"dir": "stdin", "msg": map[string]any{"method": "thread/start"}},
	}
	steps = append(steps, claudeInitMCP(0)...)
	return append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"thread": map[string]any{"id": "$session"}}}})
}

func codexTurnSteps(turnID string, callID int, stdinText, postText string, askApproval bool) []map[string]any {
	params := map[string]any{"threadId": "$session"}
	if stdinText != "" {
		params["input"] = []map[string]string{{"type": "text", "text": stdinText}}
	}
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"method": "turn/start", "params": params}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}}},
	}
	if askApproval {
		steps = append(steps, codexApprovalSteps()...)
	}
	steps = append(steps, mcpPostSteps(callID, postText)...)
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
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		if conn.Path() == want {
			return nil
		}
		if !waitConnPathSignal(conn, timer.C) {
			return fmt.Errorf("got %q after %s, want %q", conn.Path(), limit, want)
		}
	}
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
	pushes chan json.RawMessage // single reader: nextPush in scenario steps
}

func newPeerBrowser(ctx context.Context, cleanup func(func()), key identity.Key, rvURL string, mapRoute func(peer.Route) peer.Route) (*peerBrowser, error) {
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
	for len(remaining) > 0 {
		select {
		case k := <-b.presence:
			delete(remaining, k)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (b *peerBrowser) exchangeRoom(ctx context.Context, secret identity.Secret, myRole, wantRole string) (proto.Pair, error) {
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
	return b.awaitRoomPeer(ctx, secret, room, wantRole, rawSelf)
}

func (b *peerBrowser) awaitRoomPeer(ctx context.Context, secret identity.Secret, room, wantRole string, rawSelf []byte) (proto.Pair, error) {
	for {
		select {
		case ev := <-b.roomEvents:
			m, ok, err := b.handleRoomEvent(ctx, ev, secret, room, wantRole, rawSelf)
			if err != nil || ok {
				return m, err
			}
		case <-ctx.Done():
			return proto.Pair{}, ctx.Err()
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
	for range 2 {
		bc, ok, err := b.tryOpenCtl(ctx, cleanup, nodeID, 8*time.Second)
		if err != nil || ok {
			return bc, err
		}
	}
	return nil, errors.New("timed out opening ctl connection")
}

func (b *peerBrowser) tryOpenCtl(ctx context.Context, cleanup func(func()), nodeID string, wait time.Duration) (*peerBrowserConn, bool, error) {
	conn, err := b.peer.Offer(ctx, nodeID)
	if err != nil {
		return nil, false, err
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

func pumpBrowserPushes(ctx context.Context, conn *peer.Connection, pushes chan<- json.RawMessage) {
	defer close(pushes)
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case pushes <- msg:
		default:
		}
	}
}

func (c *peerBrowserConn) nextPush(ctx context.Context) (json.RawMessage, error) {
	select {
	case msg, ok := <-c.pushes:
		if !ok {
			return nil, errors.New("ctl connection closed before push")
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func requestHostInfo(ctx context.Context, conn *peer.Connection) (proto.HostInfo, error) {
	raw, err := conn.Request(ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
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
