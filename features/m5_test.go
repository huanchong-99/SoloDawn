//go:build acceptance

package features

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// What the steps that MODEL-1126 took away say when they fail.
const (
	m5NoConflict = "merges and merge conflicts are removed (MODEL-1126 plan 3.3: A5.10 goes)"
	m5NoChecks   = "check commands and the hard checks are removed (MODEL-1126 plan 3.3: A5.3, A5.9, A5.17 go)"
	m5NoMerge    = "a task has no worktree, branch, checks or merge: it runs in the assignee's workspace, is submitted by report and is done when its review passes (MODEL-1126 plan 1.5, 1.6)"
	m5NoForge    = "forges, pull requests and the live forge run are removed (MODEL-1126 plan 3.3: A5.13, A9.26 go)"
)

var (
	m5CurrentWorld atomic.Pointer[m5World]
	m5ClosedHosts  sync.Map
)

func init() {
	if len(os.Args) > 2 && os.Args[1] == "m5-mcp-shim" {
		os.Exit(m5RunMCPShim(os.Args[2], os.Args[3:]))
	}
	host.Register(host.Listeners, "m5_turns", m5OnHostEvent)
	compose := host.Compose
	host.Compose = func(h *host.Host, s host.Session, kind host.TurnKind, items []json.RawMessage) (string, host.Turn, error) {
		text, turn, err := compose(h, s, kind, items)
		if w := m5CurrentWorld.Load(); w != nil && err == nil {
			w.recordComposedTurn(s, kind, items)
		}
		return text, turn, err
	}
	host.Register(host.Holds, "m5_hold", m5OnHostHold)
	stepSets["m5"] = m5Steps
}

func m5OnHostEvent(_ *host.Host, key host.SessionKey, e host.Event) {
	w := m5CurrentWorld.Load()
	if w != nil && e.Kind == host.TurnBegan {
		w.mu.Lock()
		w.turns++
		trace := w.composedTurns[key]
		trace.Key = key
		w.turnTrace = append(w.turnTrace, trace)
		delete(w.composedTurns, key)
		w.mu.Unlock()
	}
}

type m5TurnTrace struct {
	Vendor string
	Key    host.SessionKey
	Kind   host.TurnKind
	Items  []m5ItemTrace
}

type m5ItemTrace struct {
	ID   int64
	Why  string
	Task proto.ID
	Msg  proto.ID
}

func (w *m5World) recordComposedTurn(s host.Session, kind host.TurnKind, items []json.RawMessage) {
	trace := m5TurnTrace{Vendor: s.Agent, Key: s.SessionKey, Kind: kind}
	for _, raw := range items {
		var item proto.InboxItem
		if proto.Decode(raw, &item) == nil {
			i := item.Item
			trace.Items = append(trace.Items, m5ItemTrace{ID: i.Item, Why: i.Why, Task: i.Task, Msg: i.Msg})
		}
	}
	w.mu.Lock()
	w.composedTurns[s.SessionKey] = trace
	w.mu.Unlock()
}

func m5OnHostHold(h *host.Host, key host.SessionKey) (string, bool) {
	if _, closed := m5ClosedHosts.Load(h); closed {
		return "offline", true
	}
	w := m5CurrentWorld.Load()
	if w == nil {
		return "", false
	}
	w.mu.Lock()
	held := w.closing || w.heldMembers[key.Member]
	w.mu.Unlock()
	if held {
		return "offline", true
	}
	return "", false
}

func m5ProgName(arg0 string) string {
	return strings.TrimSuffix(filepath.Base(arg0), ".exe")
}

func m5RunMCPShim(addr string, args []string) int {
	dial := func() (net.Conn, error) { return net.Dial("tcp", addr) }
	return host.RunShim(args, dial, os.Stdin, os.Stdout, os.Stderr)
}

type m5ToolCallSpec struct {
	Name    string
	Args    any
	WantErr bool
}

func m5MustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func m5StepJSON(dir string, msg any) string {
	return m5MustJSON(map[string]any{"dir": dir, "msg": msg})
}

func m5ClaudeScript(turns ...[]m5ToolCallSpec) string {
	var steps []string
	rpcID := 2
	for i, calls := range turns {
		steps = append(steps, m5ClaudeTurnStart(i)...)
		callSteps, nextID := m5BuildMCPCallSteps(calls, rpcID)
		rpcID = nextID
		steps = append(steps, callSteps...)
		steps = append(steps, m5StepJSON("stdout", map[string]any{
			"type": "result", "subtype": "success", "is_error": false, "session_id": "$session",
		}))
	}
	return `{"steps":[` + strings.Join(steps, ",") + `]}`
}

func m5ClaudeTurnStart(turnIndex int) []string {
	stdin := m5StepJSON("stdin", map[string]any{"type": "user", "message": map[string]any{"role": "user"}})
	if turnIndex > 0 {
		return []string{stdin, m5StepJSON("stdout", map[string]any{"type": "assistant", "session_id": "$session"})}
	}
	return []string{
		stdin,
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "claude-code", "version": "2.1.311"}}}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{}}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{}}),
		m5StepJSON("stdout", map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}),
	}
}

func m5CodexScript(session string, turns ...[]m5ToolCallSpec) string {
	steps := m5CodexBootSteps()
	rpcID := 2
	for i, calls := range turns {
		turnID := fmt.Sprintf("turn-%d", i+1)
		steps = append(steps,
			m5StepJSON("stdin", map[string]any{"method": "turn/start", "params": map[string]any{"threadId": "$session"}}),
			m5StepJSON("stdout", map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}}),
			m5StepJSON("stdout", map[string]any{"method": "item/started", "params": map[string]any{"threadId": "$session", "turnId": turnID}}),
		)
		callSteps, nextID := m5BuildMCPCallSteps(calls, rpcID)
		rpcID = nextID
		steps = append(steps, callSteps...)
		steps = append(steps, m5StepJSON("stdout", map[string]any{
			"method": "turn/completed",
			"params": map[string]any{"threadId": "$session", "turn": map[string]any{"id": turnID, "status": "completed"}},
		}))
	}
	return fmt.Sprintf(`{"session":%q,"steps":[%s]}`, session, strings.Join(steps, ","))
}

func m5CodexBootSteps() []string {
	return []string{
		m5StepJSON("stdin", map[string]any{"method": "initialize"}),
		m5StepJSON("stdout", map[string]any{"id": "$id", "result": map[string]any{}}),
		m5StepJSON("stdin", map[string]any{"method": "initialized"}),
		m5StepJSON("stdin", map[string]any{"id": 2}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "codex-mcp-client", "version": "0.164.0"}}}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{}}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{}}),
		m5StepJSON("stdout", map[string]any{"id": "$id", "result": map[string]any{"thread": map[string]any{"id": "$session"}}}),
	}
}

func m5BuildMCPCallSteps(calls []m5ToolCallSpec, startID int) ([]string, int) {
	var out []string
	id := startID
	for _, c := range calls {
		args := c.Args
		if args == nil {
			args = map[string]any{}
		}
		out = append(out,
			m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": c.Name, "arguments": args}}),
			m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"isError": c.WantErr}}),
		)
		id++
	}
	return out, id
}

type m5ToolRecord struct {
	Session host.SessionKey
	Kind    host.Kind
	Tool    string
	IsError bool
	Text    string
}

type m5HookFunc func(caller host.Caller) error

type m5Browser struct {
	mu      sync.Mutex
	conn    *home.Conn
	nextReq int64
	waiting map[int64]chan json.RawMessage
	rows    map[string]map[string]json.RawMessage
}

func newM5Browser() *m5Browser {
	return &m5Browser{
		waiting: map[int64]chan json.RawMessage{},
		rows:    map[string]map[string]json.RawMessage{},
	}
}

func (b *m5Browser) Done() <-chan struct{} { return nil }

func (b *m5Browser) Send(frame any) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	var hdr struct {
		Re   *int64 `json:"re,omitempty"`
		Type string `json:"t,omitempty"`
	}
	if json.Unmarshal(raw, &hdr) != nil {
		return nil
	}
	if hdr.Re != nil {
		b.deliverReply(*hdr.Re, raw)
		return nil
	}
	if hdr.Type == "rows" {
		b.applyRows(raw)
	}
	return nil
}

func (b *m5Browser) deliverReply(re int64, raw json.RawMessage) {
	b.mu.Lock()
	ch := b.waiting[re]
	delete(b.waiting, re)
	b.mu.Unlock()
	if ch != nil {
		ch <- raw
	}
}

func (b *m5Browser) applyRows(raw json.RawMessage) {
	var batch proto.Rows
	if json.Unmarshal(raw, &batch) != nil {
		return
	}
	b.mu.Lock()
	for _, elem := range batch.Rows {
		b.storeRowLocked(elem)
	}
	b.mu.Unlock()
}

func (b *m5Browser) storeRowLocked(elem json.RawMessage) {
	var rh proto.RowHeader
	if json.Unmarshal(elem, &rh) != nil || rh.Type == "" {
		return
	}
	if b.rows[rh.Type] == nil {
		b.rows[rh.Type] = map[string]json.RawMessage{}
	}
	b.rows[rh.Type][rh.ID] = elem
}

func (b *m5Browser) openRequest() (int64, <-chan json.RawMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextReq++
	// Single reader: waitBrowserReply in the calling test step.
	ch := make(chan json.RawMessage, 1)
	b.waiting[b.nextReq] = ch
	return b.nextReq, ch
}

type m5World struct {
	f       fixtures
	cleanup func(func())
	cluster *nodetest.Cluster
	browser *m5Browser
	groupID string
	binDir  string
	secID   string
	claudeM string
	codexM  string
	seq     atomic.Int64
	scriptN atomic.Int64

	mu                sync.Mutex
	turns             int
	composedTurns     map[host.SessionKey]m5TurnTrace
	turnTrace         []m5TurnTrace
	turnsAtReviewPass map[string]int
	turnsAtMerged     map[string]int
	sawTaskSess       map[int]host.SessionKey
	sawTaskDir        map[int]string
	sawRevSess        map[int]host.SessionKey
	sawRevDir         map[int]string
	tools             []m5ToolRecord
	listedTools       map[host.SessionKey][]string
	beforeHooks       map[string][]m5HookFunc
	heldMembers       map[string]bool
	closing           bool
	hookErr           error

	confirmAns1 proto.CmdResult
	confirmAns2 proto.CmdResult
	failAns1    proto.CmdResult
	failAns2    proto.CmdResult
	refuseCode1 string
	refuseCode2 string
	commit1     string
	commit2     string
	unreviewed  string
	refusedItem int64
}

func newM5World(sc *godog.ScenarioContext, f fixtures) *m5World {
	w := &m5World{
		f:                 f,
		composedTurns:     map[host.SessionKey]m5TurnTrace{},
		turnsAtReviewPass: map[string]int{},
		turnsAtMerged:     map[string]int{},
		sawTaskSess:       map[int]host.SessionKey{},
		sawTaskDir:        map[int]string{},
		sawRevSess:        map[int]host.SessionKey{},
		sawRevDir:         map[int]string{},
		listedTools:       map[host.SessionKey][]string{},
		beforeHooks:       map[string][]m5HookFunc{},
		heldMembers:       map[string]bool{},
	}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.settleBeforeAfterHooks()
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.browser = newM5Browser()
	return w
}

func (w *m5World) nextID() string {
	n := w.seq.Add(1)
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], uint64(n))
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func (w *m5World) writeScriptFile(vendor, content string) string {
	idx := w.scriptN.Add(1)
	path := filepath.Join(w.cluster.Root, fmt.Sprintf("%s-%d.json", vendor, idx))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		w.recordHookErr(err)
	}
	w.recordHookErr(setScenarioEnv(w.cleanup, "SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", path))
	return path
}

func (w *m5World) recordHookErr(err error) {
	if err == nil {
		return
	}
	w.mu.Lock()
	if w.hookErr == nil {
		w.hookErr = err
	}
	w.mu.Unlock()
}

func (w *m5World) checkHookErr() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hookErr
}

func (w *m5World) addBeforeHook(tool string, fn m5HookFunc) {
	w.mu.Lock()
	w.beforeHooks[tool] = append(w.beforeHooks[tool], fn)
	w.mu.Unlock()
}

func (w *m5World) popBeforeHook(tool string) m5HookFunc {
	w.mu.Lock()
	defer w.mu.Unlock()
	list := w.beforeHooks[tool]
	if len(list) == 0 {
		return nil
	}
	w.beforeHooks[tool] = list[1:]
	return list[0]
}

func (w *m5World) setMemberHeld(member string, held bool) {
	w.mu.Lock()
	if held {
		w.heldMembers[member] = true
	} else {
		delete(w.heldMembers, member)
	}
	w.mu.Unlock()
}

func (w *m5World) setupCluster(level2 bool) error {
	if err := w.prepareHelperBinaries(); err != nil {
		return err
	}
	w.registerHostHooks()
	tb := scenarioTB{TB: w.f.t, cleanup: w.cleanup}
	w.cluster = nodetest.Start(tb, nodetest.Config{BinDir: w.binDir, Nodes: 1})
	if err := w.startMCPBridge(); err != nil {
		return err
	}
	return w.createGroupAndJoinAIs(level2)
}

func m5ExeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func (w *m5World) prepareHelperBinaries() error {
	w.binDir = w.f.t.TempDir()
	ext := m5ExeExt()
	for _, cli := range []string{"claude", "codex"} {
		if err := m5CopyExec(filepath.Join(w.f.fakeCLIs, cli+ext), filepath.Join(w.binDir, cli+ext)); err != nil {
			return err
		}
	}
	return nil
}

func m5CopyExec(src, dst string) error {
	if os.Link(src, dst) == nil {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o755)
}

func (w *m5World) registerHostHooks() {
	m5CurrentWorld.Store(w)
	w.cleanup(func() { m5CurrentWorld.Store(nil) })
}

func (w *m5World) settleBeforeAfterHooks() {
	if w.cluster == nil {
		return
	}
	w.mu.Lock()
	already := w.closing
	w.closing = true
	w.mu.Unlock()
	if already {
		return
	}
	n0 := w.cluster.Nodes[0]
	m5ClosedHosts.Store(n0.Host, true)
	n0.Link.Close()
	n0.Home.Close()
	w.recordHookErr(w.waitEndedTasksCleaned(n0.Host))
	w.recordHookErr(m5WaitAllDormant(n0.Host))
}

func (w *m5World) waitEndedTasksCleaned(h *host.Host) error {
	if w.groupID == "" {
		return nil
	}
	return w.waitFor("ended task sessions closed", func() bool {
		return m5EndedTasksCleaned(h, w.groupID)
	})
}

func m5IsObsoleteTask(raw json.RawMessage) (proto.TaskRow, bool) {
	var t proto.TaskRow
	ok := json.Unmarshal(raw, &t) == nil && (t.State == "done" || t.State == "cancelled")
	return t, ok
}

func m5EndedTasksCleaned(h *host.Host, group string) bool {
	for _, raw := range h.Rows.Of(group, "task") {
		if t, ok := m5IsObsoleteTask(raw); ok && slices.ContainsFunc(h.Sessions.All(), func(s host.Session) bool { return s.Task == t.ID }) {
			return false
		}
	}
	return true
}

func m5FinalStep(w *m5World, fn func() error) func() error {
	return func() error {
		err := fn()
		w.settleBeforeAfterHooks()
		if err != nil {
			return err
		}
		return w.checkHookErr()
	}
}

func m5WaitAllDormant(h *host.Host) error {
	return waitUntil(5*time.Second, "every host session dormant", func() error { return sessionsDormant(h) })
}

func (w *m5World) startMCPBridge() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	w.cleanup(func() {
		cancel()
		ln.Close()
		wg.Wait()
	})
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	n0 := w.cluster.Nodes[0]
	n0.Host.Shim = []string{exe, "m5-mcp-shim", ln.Addr().String()}
	wg.Add(1)
	go w.acceptMCPLoop(ctx, ln, n0.Host, &wg)
	return nil
}

func (w *m5World) acceptMCPLoop(ctx context.Context, ln net.Listener, h *host.Host, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		conn, err := ln.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			w.recordHookErr(fmt.Errorf("accept an MCP shim: %w", err))
			time.Sleep(10 * time.Millisecond) // the error may last; this is no busy loop
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.serveOneMCPConn(ctx, conn, h)
		}()
	}
}

func (w *m5World) serveOneMCPConn(ctx context.Context, conn net.Conn, h *host.Host) {
	open, _, reader, err := node.AcceptOpening(conn)
	if err != nil {
		return
	}
	defer context.AfterFunc(ctx, func() { conn.Close() })()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	var reqMu sync.Mutex
	methods := map[string]string{}
	tools := map[string]string{}
	go w.pumpMCPRequests(h, open.MCP, reader, inW, &reqMu, methods, tools)
	go w.pumpMCPResponses(h, open.MCP, outR, conn, &reqMu, methods, tools)
	h.ServeMCP(ctx, open.MCP, inR, outW)
}

func (w *m5World) pumpMCPRequests(h *host.Host, token string, r *bufio.Reader, dst *io.PipeWriter, mu *sync.Mutex, methods, tools map[string]string) {
	defer dst.Close()
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			w.inspectMCPRequest(h, token, bytes.TrimSpace(line), mu, methods, tools)
			if _, werr := dst.Write(line); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (w *m5World) inspectMCPRequest(h *host.Host, token string, line []byte, mu *sync.Mutex, methods, tools map[string]string) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &req) != nil || len(req.ID) == 0 {
		return
	}
	idKey := string(req.ID)
	mu.Lock()
	methods[idKey] = req.Method
	tools[idKey] = req.Params.Name
	mu.Unlock()
	caller, ok := h.Sessions.Caller(token)
	if ok && req.Method == "tools/call" {
		w.onBeforeToolCall(h, caller, req.Params.Name)
	}
}

func (w *m5World) onBeforeToolCall(h *host.Host, caller host.Caller, toolName string) {
	w.noteSessionWorkspace(h, caller)
	if fn := w.popBeforeHook(toolName); fn != nil {
		w.recordHookErr(fn(caller))
	}
}

func (w *m5World) noteSessionWorkspace(h *host.Host, caller host.Caller) {
	if caller.Task == "" {
		return
	}
	raw, ok := h.Rows.Get(caller.Group, "task", caller.Task)
	if !ok {
		return
	}
	var t proto.TaskRow
	if json.Unmarshal(raw, &t) != nil {
		return
	}
	w.recordObservedSession(caller, t.Num, caller.Folder)
}

func (w *m5World) recordObservedSession(caller host.Caller, num int, dir string) {
	if _, err := os.Stat(dir); err != nil {
		w.recordHookErr(err)
		return
	}
	w.mu.Lock()
	switch caller.Kind {
	case host.TaskSession:
		w.sawTaskSess[num] = caller.SessionKey
		w.sawTaskDir[num] = dir
	case host.ReviewSession:
		w.sawRevSess[num] = caller.SessionKey
		w.sawRevDir[num] = dir
	}
	w.mu.Unlock()
}

func (w *m5World) pumpMCPResponses(h *host.Host, token string, src *io.PipeReader, dst net.Conn, mu *sync.Mutex, methods, tools map[string]string) {
	defer dst.Close()
	r := bufio.NewReader(src)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			w.inspectMCPResponse(h, token, bytes.TrimSpace(line), mu, methods, tools)
			if _, werr := dst.Write(line); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (w *m5World) inspectMCPResponse(h *host.Host, token string, line []byte, mu *sync.Mutex, methods, tools map[string]string) {
	var resp struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(line, &resp) != nil || len(resp.ID) == 0 {
		return
	}
	idKey := string(resp.ID)
	mu.Lock()
	method, tool := methods[idKey], tools[idKey]
	delete(methods, idKey)
	delete(tools, idKey)
	mu.Unlock()
	caller, ok := h.Sessions.Caller(token)
	if !ok {
		return
	}
	if method == "tools/list" {
		w.recordListedTools(caller.SessionKey, resp.Result)
	}
	if method == "tools/call" {
		w.onAfterToolCall(h, caller, tool, resp.Result)
	}
}

func (w *m5World) recordListedTools(key host.SessionKey, raw json.RawMessage) {
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	names := make([]string, len(list.Tools))
	for i, item := range list.Tools {
		names[i] = item.Name
	}
	w.mu.Lock()
	w.listedTools[key] = names
	w.mu.Unlock()
}

func m5ParseToolCallResult(raw json.RawMessage) (string, bool) {
	var callRes struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(raw, &callRes) != nil {
		return "", false
	}
	if len(callRes.Content) > 0 {
		return callRes.Content[0].Text, callRes.IsError
	}
	return "", callRes.IsError
}

func (w *m5World) onAfterToolCall(h *host.Host, caller host.Caller, tool string, raw json.RawMessage) {
	text, isErr := m5ParseToolCallResult(raw)
	w.mu.Lock()
	w.tools = append(w.tools, m5ToolRecord{Session: caller.SessionKey, Kind: caller.Kind, Tool: tool, IsError: isErr, Text: text})
	w.mu.Unlock()
}

func m5Same[T comparable](what string, got, want T) error {
	if got != want {
		return fmt.Errorf("%s: got %v, want %v", what, got, want)
	}
	return nil
}

func (w *m5World) createGroupAndJoinAIs(level2 bool) error {
	n0 := w.cluster.Nodes[0]
	w.browser.conn = n0.Home.Attach(w.cluster.Owner.ID(), w.browser)
	w.cleanup(func() { n0.Home.Detach(w.browser.conn) })
	w.groupID = w.nextID()
	createPayload := map[string]any{
		"t": "group", "op": "create", "n": w.groupID, "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "name": "Proj", "member_name": "Owner",
	}
	if _, err := w.ownerCmd(createPayload); err != nil {
		return err
	}
	if err := w.syncBrowser(); err != nil {
		return err
	}
	return w.joinGroupMembers(level2)
}

func (w *m5World) declareAndJoinWorkingAIs() error {
	n0 := w.cluster.Nodes[0]
	if _, err := n0.Host.Agents(context.Background(), false); err != nil {
		return err
	}
	for _, agent := range []string{"claude", "codex"} {
		if err := w.sendHostCommand("agent.set", map[string]any{"agent": agent, "account": "personal"}); err != nil {
			return err
		}
	}
	return s3Run(func() error { return w.joinAI("claude", "Claude", false) },
		func() error { return w.joinAI("codex", "Codex", false) },
		func() error { return w.joinAI("claude", "Secretary", true) })
}

// joinAI adds an AI of the node to the group, in a workspace of its own.
func (w *m5World) joinAI(agent, name string, secretary bool) error {
	workspace, err := aiWorkspace(w.cluster.Root, proto.ID(w.groupID), name)
	if err != nil {
		return err
	}
	join := map[string]any{"g": w.groupID, "home": w.cluster.Nodes[0].Key.ID(), "agent": agent, "name": name, "secretary": secretary, "workspace": workspace}
	return w.sendHostCommand("host.join", join)
}

func (w *m5World) joinGroupMembers(level2 bool) error {
	if err := w.declareAndJoinWorkingAIs(); err != nil {
		return err
	}
	if err := w.configureLevels(level2); err != nil {
		return err
	}
	return w.waitMembersSynced()
}

func (w *m5World) sendHostCommand(name string, fields map[string]any) error {
	n0 := w.cluster.Nodes[0]
	fields["t"] = name
	fields["n"] = w.nextID()
	fields["ts"] = n0.Host.Now().UnixMilli()
	fields["to"] = n0.Key.ID()
	return callHostSurface(n0.Host, n0.Link, w.cluster.Owner.ID(), name, fields)
}

func (w *m5World) configureLevels(level2 bool) error {
	if !level2 {
		return nil
	}
	claudeLvl := 2
	if runtime.GOOS == "windows" {
		claudeLvl = 3
	}
	write := func(tx *store.Tx) error {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO perm(agent, g, level) VALUES('codex', ?, 2)`, w.groupID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO perm(agent, g, level) VALUES('claude', ?, ?)`, w.groupID, claudeLvl)
		return err
	}
	return w.cluster.Nodes[0].Store.Write(write)
}

func (w *m5World) waitMembersSynced() error {
	n0 := w.cluster.Nodes[0]
	check := func() bool {
		w.claudeM = w.findMemberByName("Claude")
		w.codexM = w.findMemberByName("Codex")
		w.secID = w.findMemberByName("Secretary")
		_, hasSec := n0.Host.Rows.Get(w.groupID, "member", w.secID)
		_, hasC := n0.Host.Rows.Get(w.groupID, "member", w.claudeM)
		_, hasX := n0.Host.Rows.Get(w.groupID, "member", w.codexM)
		return w.claudeM != "" && w.codexM != "" && w.secID != "" && hasSec && hasC && hasX
	}
	return w.waitFor("group members synced to host", check)
}

func (w *m5World) findMemberByName(name string) string {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	for id, raw := range w.browser.rows["member"] {
		var m proto.MemberRow
		if json.Unmarshal(raw, &m) == nil && m.Name == name {
			return id
		}
	}
	return ""
}

func (w *m5World) ownerCmd(payload any) (proto.CmdResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return proto.CmdResult{}, err
	}
	return w.sendSignedCmd(w.cluster.Owner, string(raw))
}

func (w *m5World) sendSignedCmd(signer identity.Key, payload string) (proto.CmdResult, error) {
	return w.sendEnvelopeCmd(signer.Sign(payload))
}

func (w *m5World) sendEnvelopeCmd(env proto.Envelope) (proto.CmdResult, error) {
	reqID, ch := w.browser.openRequest()
	w.cluster.Nodes[0].Home.Cmd(w.browser.conn, reqID, env)
	return w.waitCmdReply(ch)
}

func (w *m5World) waitCmdReply(ch <-chan json.RawMessage) (proto.CmdResult, error) {
	wt := newWait(10*time.Second, "the home command reply", nil)
	defer wt.stop()
	raw, err := recvWait(wt, ch)
	if err != nil {
		return proto.CmdResult{}, err
	}
	var errResp proto.ErrorResponse
	if proto.Decode(raw, &errResp) == nil {
		return proto.CmdResult{}, &host.Error{Code: errResp.Err.Code, Message: errResp.Err.Message}
	}
	var res proto.CmdResult
	return res, proto.Decode(raw, &res)
}

func (w *m5World) syncBrowser() error {
	reqID, ch := w.browser.openRequest()
	w.cluster.Nodes[0].Home.Sync(w.browser.conn, proto.Sync{
		Request: proto.Request{Type: "sync", ID: reqID},
		Group:   w.groupID,
	})
	wt := newWait(10*time.Second, "the browser sync", nil)
	defer wt.stop()
	raw, err := recvWait(wt, ch)
	if err != nil {
		return err
	}
	var synced proto.Synced
	return proto.Decode(raw, &synced)
}

func (w *m5World) waitFor(desc string, check func() bool) error {
	return w.waitSaw(desc, func() error {
		if !check() {
			return errors.New("not yet, tasks: " + w.tasksBrief())
		}
		return nil
	})
}

func (w *m5World) tasksBrief() string {
	var tasks []string
	for _, t := range w.browserTasks() {
		wait := ""
		if t.Wait != nil {
			wait = t.Wait.Kind
		}
		tasks = append(tasks, fmt.Sprintf("T-%d %s/%s wait=%q report=%s failures=%d", t.Num, t.State, t.Step, wait, t.Report, t.Failures))
	}
	return strings.Join(tasks, "; ")
}

func (w *m5World) waitSaw(desc string, check func() error) error {
	return waitUntilEvery(10*time.Millisecond, 15*time.Second, desc, func() error {
		if err := w.checkHookErr(); err != nil {
			return abort(err)
		}
		return check()
	})
}

func (w *m5World) ownerPostMain(text string, mentions []string) error {
	return w.ownerPost(text, "", mentions)
}

// ownerPost posts in the thread of a task, or in the main line without one.
func (w *m5World) ownerPost(text, thread string, mentions []string) error {
	var in any
	if thread != "" {
		in = thread
	}
	_, err := w.ownerCmd(map[string]any{
		"t": "post", "n": w.nextID(), "ts": w.cluster.Nodes[0].Host.Now().UnixMilli(), "g": w.groupID,
		"kind": "text", "text": text, "thread": in, "main": thread == "",
		"mentions": append([]string{}, mentions...), "re": nil, "body": map[string]any{},
	})
	return err
}

func (w *m5World) ownerConfirmBrief(briefID string) error {
	n0 := w.cluster.Nodes[0]
	_, err := w.ownerCmd(map[string]any{
		"t": "act", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "msg": briefID, "action": "confirm",
	})
	return err
}

func (w *m5World) pendingBriefID() string {
	for _, m := range w.browserMessages() {
		if m.Kind == "brief" && m.State != nil && m.State.Status == "pending" {
			return m.ID
		}
	}
	return ""
}

func (w *m5World) browserMessages() []proto.MsgRow {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	var list []proto.MsgRow
	for _, raw := range w.browser.rows["msg"] {
		var m proto.MsgRow
		if json.Unmarshal(raw, &m) == nil {
			list = append(list, m)
		}
	}
	slices.SortFunc(list, func(a, b proto.MsgRow) int { return int(a.Pos - b.Pos) })
	return list
}

func (w *m5World) browserTasks() []proto.TaskRow {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	var list []proto.TaskRow
	for _, raw := range w.browser.rows["task"] {
		var t proto.TaskRow
		if json.Unmarshal(raw, &t) == nil {
			list = append(list, t)
		}
	}
	slices.SortFunc(list, func(a, b proto.TaskRow) int { return a.Num - b.Num })
	return list
}

func (w *m5World) taskByNum(num int) (proto.TaskRow, bool) {
	for _, t := range w.browserTasks() {
		if t.Num == num {
			return t, true
		}
	}
	return proto.TaskRow{}, false
}

func m5BriefArgs(tasks []map[string]any) map[string]any {
	return map[string]any{
		"title":     "Build helpers",
		"goal":      "Add helper functions with tests",
		"criteria":  []string{"All checks pass"},
		"scope_in":  []string{"helpers"},
		"scope_out": []string{"cli"},
		"tasks":     tasks,
	}
}

func m5TwoTaskBriefArgs() map[string]any {
	return m5BriefArgs([]map[string]any{
		{"title": "Add alpha helper", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
		{"title": "Add beta helper", "assignee": "Codex", "reviewer": "Claude", "depends": []int{0}},
	})
}

func m5OneTaskBriefArgs() map[string]any {
	return m5BriefArgs([]map[string]any{
		{"title": "Add gamma helper", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
	})
}

func m5Steps(sc *godog.ScenarioContext, f fixtures) {
	w := newM5World(sc, f)
	m5RegisterFullFlowSteps(sc, w)
	m5RegisterCheckAndReviewSteps(sc, w)
	m5RegisterDepAndDirectSteps(sc, w)
	m5RegisterConflictAndEdgeSteps(sc, w)
	m5RegisterJumpSteps(sc, w)
	m5RegisterFailureSteps(sc, w)
}

func m5RegisterFullFlowSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a signed-in non-GitHub forge and its scratch repository$`, rewritten("AC-1", m5NoForge))
	sc.Step(`^both assigned branches were pushed without approval cards$`, rewritten("AC-1", m5NoForge))
	sc.Step(`^the forge reports both distinct requests merged at their reviewed heads$`, rewritten("AC-1", m5NoForge))
	sc.Step(`^a project group with forge "([^"]*)", a secretary and working AIs "Claude" and "Codex"$`, func(string) error {
		return w.setupCluster(true)
	})
	sc.Step(`^the owner posts a feature request in the main line and the secretary proposes a two-task brief$`, w.stepRequestAndTwoTaskBrief)
	sc.Step(`^before confirm no task worktree exists and no assign item is queued on the home$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the owner confirms the brief and both tasks run through checks, cross-AI review, merge and summary$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the owner confirms the brief and both tasks push their branches, pass local and hosted checks, review each other, merge and summarize$`, rewritten("AC-1", m5NoForge))
	sc.Step(`^both tasks are merged onto base with two summary cards in the main line and no approval card$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^push_branch pushed the assigned branch to origin with no approval card$`, rewritten("AC-1", m5NoForge))
	sc.Step(`^both tasks are merged on origin with two summary cards carrying pull-request numbers and no approval card$`, rewritten("AC-1", m5NoForge))
	sc.Step(`^the owner saw no confirm card \(A9\.40\)$`, w.verifyNoConfirmCard)
	sc.Step(`^each passing review was followed by its merge with no AI turn in between and a task_merged row in the thread$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^each task ran in its own session and worktree, and after the merge those sessions and folders are gone$`, rewritten("AC-1", m5NoMerge))
}

func (w *m5World) stepRequestAndTwoTaskBrief() error {
	turns := append(m5BriefTurns(m5TwoTaskBriefArgs()), m5SummaryTurns("Alpha merged.", "Beta merged.")...)
	secScript := m5ClaudeScript(turns...)
	w.writeScriptFile("claude", secScript)
	if err := w.ownerPostMain("Please add alpha and beta helpers.", nil); err != nil {
		return err
	}
	return w.waitFor("pending brief card", func() bool { return w.pendingBriefID() != "" })
}

func m5BriefTurns(args map[string]any) [][]m5ToolCallSpec {
	return [][]m5ToolCallSpec{{{Name: "propose_brief", Args: args}}, nil}
}

func m5SummaryTurns(texts ...string) [][]m5ToolCallSpec {
	var turns [][]m5ToolCallSpec
	for i, text := range texts {
		turns = append(turns, []m5ToolCallSpec{{Name: "post", Args: map[string]any{
			"kind": "summary", "task": fmt.Sprintf("T-%d", i+1), "text": text,
		}}})
	}
	return turns
}

func m5PassingReview(score int) []m5ToolCallSpec {
	return []m5ToolCallSpec{{Name: "submit_review", Args: map[string]any{"score": score, "items": []any{}}}}
}

func m5WorkerCalls(summary string) []m5ToolCallSpec {
	return []m5ToolCallSpec{{Name: "submit_task", Args: map[string]any{"summary": summary}}}
}

func m5RegisterCheckAndReviewSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^an active brief task on forge "([^"]*)" assigned to "Claude" and reviewed by "Codex"$`, func(string) error {
		return w.givenOneBriefTask(false)
	})
	sc.Step(`^"Claude" runs checks on a dirty worktree, submits after failing checks, submits after passing checks, and submits again after an unchecked commit$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^run_checks refused the dirty worktree, submit_task failed after failing checks, succeeded after passing checks, and failed after the extra commit$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the review passes while merge is held and a new commit advances the task branch before merge runs$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the unreviewed commit is not merged onto base and the task returns to local_check waiting on conflict$`, rewritten("AC-1", m5NoMerge))

	sc.Step(`^a submitted brief task on forge "([^"]*)" assigned to "Claude" and reviewed by "Codex"$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the assignee and a wrong-commit checkout attempt reviews on the submitted task$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the assignee review is refused with "forbidden" and the wrong-commit review is refused with "conflict"$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^"Codex" reviews with score 86, then with a veto at score 95, and then four more failing reviews on resubmissions$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the 86 review returned the task to local_check, the veto recorded score 0, and the sixth failed review made the task failed$`, rewritten("AC-1", m5NoMerge))

	sc.Step(`^"Claude" commits a test secret block with a test file and calls run_checks and submit_task$`, rewritten("AC-1", m5NoChecks))
	sc.Step(`^run_checks completes with ok false and a failing "secrets" hard-check item, and submit_task publishes nothing$`, rewritten("AC-1", m5NoChecks))

	sc.Step(`^a project group on forge "([^"]*)" with check "m5envcheck" and a test token variable set in the node environment$`, rewritten("AC-1", m5NoChecks))
	sc.Step(`^an active task on that group commits a change and calls run_checks$`, rewritten("AC-1", m5NoChecks))
	sc.Step(`^run_checks passes with "m5envcheck" seeing PATH and HOME and not seeing the test token variable$`, rewritten("AC-1", m5NoChecks))
}

func (w *m5World) waitPendingBrief() error {
	hasBrief := func() bool { return w.pendingBriefID() != "" }
	return w.waitFor("pending brief", hasBrief)
}

func (w *m5World) givenOneBriefTask(holdCodex bool) error {
	if err := w.setupCluster(true); err != nil {
		return err
	}
	if holdCodex {
		w.setMemberHeld(w.codexM, true)
	}
	w.writeScriptFile("claude", m5ClaudeScript(
		[]m5ToolCallSpec{{Name: "propose_brief", Args: m5OneTaskBriefArgs()}},
		nil,
	))
	if err := w.ownerPostMain("Add gamma helper.", nil); err != nil {
		return err
	}
	if err := w.waitPendingBrief(); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, true)
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	err := w.waitFor("active task 1 and dormant secretary", w.task1AndSecReady)
	w.setMemberHeld(w.secID, true)
	w.resetTools()
	return err
}

func (w *m5World) task1AndSecReady() bool {
	t, ok := w.taskByNum(1)
	state, _ := w.cluster.Nodes[0].Host.MemberState(w.groupID, w.secID)
	return ok && t.State == "active" && state == string(host.Dormant)
}

func (w *m5World) resetTools() {
	w.mu.Lock()
	w.tools = nil
	w.mu.Unlock()
}

func (w *m5World) toolCallCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.tools)
}

func m5ErrCode(err error) string {
	var he *host.Error
	if errors.As(err, &he) {
		return he.Code
	}
	return ""
}

func m5RegisterDepAndDirectSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a confirmed brief where task 2 depends on task 1 on forge "([^"]*)"$`, w.stepGivenConfirmedDepBrief)
	sc.Step(`^the secretary reassigns task 2 to "Codex" and reviewer "Claude" while task 2 is queued$`, w.stepReassignQueuedTask2)
	sc.Step(`^task 2 remains queued with no worktree and no task session$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^task 1 completes checks, passes review and merges$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^task 2 becomes active, creates its worktree, and runs its task session for "Codex"$`, rewritten("AC-1", m5NoMerge))

	sc.Step(`^the owner mentions "Claude" directly for a code change and "Claude" calls start_task$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the group session listed start_task without task submission tools, created a task card, notified the secretary, and committed the change in a new task session$`, rewritten("AC-1", m5NoMerge))
}

func (w *m5World) stepGivenConfirmedDepBrief(string) error {
	if err := w.setupCluster(true); err != nil {
		return err
	}
	brief := m5BriefArgs([]map[string]any{
		{"title": "Base unit", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
		{"title": "Dependent unit", "assignee": "Claude", "reviewer": "Codex", "depends": []int{0}},
	})
	w.writeScriptFile("claude", m5ClaudeScript(
		[]m5ToolCallSpec{{Name: "propose_brief", Args: brief}},
		nil,
		[]m5ToolCallSpec{{Name: "set_task", Args: map[string]any{"task": "T-2", "assignee": "Codex", "reviewer": "Claude"}}},
	))
	if err := w.ownerPostMain("Build base and dependent units.", nil); err != nil {
		return err
	}
	if err := w.waitPendingBrief(); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, true)
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	return w.waitFor("2 tasks created and secretary confirmed turn settled", w.depBriefConfirmedReady)
}

func (w *m5World) turnCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.turns
}

func (w *m5World) depBriefConfirmedReady() bool {
	state, _ := w.cluster.Nodes[0].Host.MemberState(w.groupID, w.secID)
	return len(w.browserTasks()) == 2 && w.turnCount() >= 2 && state == string(host.Waiting)
}

func (w *m5World) stepReassignQueuedTask2() error {
	if err := w.ownerPostMain("Give T-2 to Codex.", nil); err != nil {
		return err
	}
	return w.waitFor("task 2 reassigned to Codex", func() bool {
		t2, ok := w.taskByNum(2)
		return ok && t2.Assignee == w.codexM && t2.Reviewer == w.claudeM
	})
}

func m5RegisterConflictAndEdgeSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a submitted brief task on forge "([^"]*)" whose base branch gained a conflicting commit$`, rewritten("AC-1", m5NoConflict))
	sc.Step(`^"Codex" passes the review and the host attempts the merge$`, rewritten("AC-1", m5NoConflict))
	sc.Step(`^the task returns to local_check with wait "conflict" and the secretary receives a conflict notice$`, rewritten("AC-1", m5NoConflict))
	sc.Step(`^the secretary names "Claude" on the conflicted task and "Claude" resolves, re-checks, resubmits, passes review and merges$`, rewritten("AC-1", m5NoConflict))
	sc.Step(`^the task reaches state "merged" and base holds the resolved content$`, rewritten("AC-1", m5NoConflict))

	sc.Step(`^a project group with forge "([^"]*)", a secretary, working AIs and a pending one-task brief$`, w.stepGivenPendingOneTaskBrief)
	sc.Step(`^the owner sends the same signed confirm twice and the host sends the same signed task fail twice$`, w.stepSendDuplicateConfirmAndFail)
	sc.Step(`^both confirms answered ok creating one task, and both task fails answered ok counting one failure$`, m5FinalStep(w, w.stepVerifyDuplicateConfirmAndFail))

	sc.Step(`^removing "Claude" or "Codex" from the group is refused with "conflict"$`, w.stepVerifyBusyAICannotLeave)
	sc.Step(`^"Codex" passes the review while the host holds the merge item$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^reassigning the task and submitting a new commit at step merge are refused with "conflict"$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the owner cancels the task at step merge and the host releases and processes the held merge item$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the task is cancelled, base is unchanged, and the host recorded the merge item as refused$`, rewritten("AC-1", m5NoMerge))

	sc.Step(`^an active brief task on forge "([^"]*)" assigned to "Claude" with reviewer "Codex" held on the host$`, func(string) error {
		return w.givenOneBriefTask(true)
	})
	sc.Step(`^"Claude" submits commit 1 and then commits, checks and submits commit 2 before "Codex" is released$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the home has retired the review item for commit 1 and keeps only commit 2 queued$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the host hold on "Codex" is released and "Codex" runs its review turn$`, rewritten("AC-1", m5NoMerge))
	sc.Step(`^the host refused the held review item for commit 1 and delivered only commit 2 to "Codex"$`, rewritten("AC-1", m5NoMerge))
}

func (w *m5World) stepGivenPendingOneTaskBrief(string) error {
	if err := w.setupCluster(true); err != nil {
		return err
	}
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{Name: "propose_brief", Args: m5OneTaskBriefArgs()}}))
	if err := w.ownerPostMain("Add gamma helper.", nil); err != nil {
		return err
	}
	return w.waitFor("pending brief", func() bool { return w.pendingBriefID() != "" })
}

func (w *m5World) stepSendDuplicateConfirmAndFail() error {
	n0 := w.cluster.Nodes[0]
	w.setMemberHeld(w.secID, true)
	w.setMemberHeld(w.claudeM, true)
	confirmRaw := m5MustJSON(map[string]any{
		"t": "act", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "msg": w.pendingBriefID(), "action": "confirm",
	})
	confirmEnv := w.cluster.Owner.Sign(confirmRaw)
	var err error
	if w.confirmAns1, err = w.sendEnvelopeCmd(confirmEnv); err != nil {
		return err
	}
	if w.confirmAns2, err = w.sendEnvelopeCmd(confirmEnv); err != nil {
		return err
	}
	oneTask := func() bool { return len(w.browserTasks()) == 1 }
	if err := w.waitFor("one task created", oneTask); err != nil {
		return err
	}
	return w.sendDuplicateTaskFail()
}

func (w *m5World) sendDuplicateTaskFail() error {
	n0 := w.cluster.Nodes[0]
	t1, _ := w.taskByNum(1)
	failPayload := m5MustJSON(map[string]any{
		"t": "task", "op": "fail", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": t1.ID, "reason": "crash", "detail": "simulated exit",
	})
	env := n0.Key.Sign(failPayload)
	var err error
	if w.failAns1, err = w.sendHostEnvelope(env); err != nil {
		return err
	}
	if w.failAns2, err = w.sendHostEnvelope(env); err != nil {
		return err
	}
	return w.waitFor("task failure recorded", func() bool {
		cur, ok := w.taskByNum(1)
		return ok && cur.Failures >= 1
	})
}

func (w *m5World) sendHostEnvelope(env proto.Envelope) (proto.CmdResult, error) {
	n0 := w.cluster.Nodes[0]
	build := func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	}
	raw, err := requestWait(context.Background(), n0.Link, nil, build)
	if err != nil {
		return proto.CmdResult{}, err
	}
	var res proto.CmdResult
	return res, proto.Decode(raw, &res)
}

func m5BothOK(a, b proto.CmdResult) bool {
	return bool(a.OK && b.OK)
}

func (w *m5World) stepVerifyDuplicateConfirmAndFail() error {
	t1, ok := w.taskByNum(1)
	if !ok || !m5BothOK(w.confirmAns1, w.confirmAns2) || len(w.browserTasks()) != 1 {
		return fmt.Errorf("unexpected confirm results: %+v %+v tasks=%d", w.confirmAns1, w.confirmAns2, len(w.browserTasks()))
	}
	if !m5BothOK(w.failAns1, w.failAns2) || t1.Failures != 1 {
		return fmt.Errorf("unexpected fail results: %+v %+v failures=%d", w.failAns1, w.failAns2, t1.Failures)
	}
	return nil
}

func (w *m5World) stepVerifyBusyAICannotLeave() error {
	for _, memberID := range []string{w.claudeM, w.codexM} {
		if m5ErrCode(w.ownerRemoveMember(memberID)) != "conflict" {
			return fmt.Errorf("expected conflict removing member %s", memberID)
		}
	}
	return nil
}

func (w *m5World) ownerRemoveMember(memberID string) error {
	n0 := w.cluster.Nodes[0]
	_, err := w.ownerCmd(map[string]any{
		"t": "member", "op": "remove", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": memberID,
	})
	return err
}
