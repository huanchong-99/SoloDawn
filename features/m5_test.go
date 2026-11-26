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
	"io/fs"
	"maps"
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

	mu            sync.Mutex
	turns         int
	composedTurns map[host.SessionKey]m5TurnTrace
	turnTrace     []m5TurnTrace
	turnsAtReview map[string]int
	turnsAtDone   map[string]int
	workspaces    map[string]string
	sawTaskSess   map[int]host.SessionKey
	sawTaskDir    map[int]string
	sawRevSess    map[int]host.SessionKey
	sawRevDir     map[int]string
	tools         []m5ToolRecord
	listedTools   map[host.SessionKey][]string
	beforeHooks   map[string][]m5HookFunc
	heldMembers   map[string]bool
	closing       bool
	hookErr       error

	confirmAns1 proto.CmdResult
	confirmAns2 proto.CmdResult
	failAns1    proto.CmdResult
	failAns2    proto.CmdResult
	refuseCode1 string
	refuseCode2 string
	report1     string
	report2     string
	refusedItem int64
}

func newM5World(sc *godog.ScenarioContext, f fixtures) *m5World {
	w := &m5World{
		f:             f,
		composedTurns: map[host.SessionKey]m5TurnTrace{},
		turnsAtReview: map[string]int{},
		turnsAtDone:   map[string]int{},
		workspaces:    map[string]string{},
		sawTaskSess:   map[int]host.SessionKey{},
		sawTaskDir:    map[int]string{},
		sawRevSess:    map[int]host.SessionKey{},
		sawRevDir:     map[int]string{},
		listedTools:   map[host.SessionKey][]string{},
		beforeHooks:   map[string][]m5HookFunc{},
		heldMembers:   map[string]bool{},
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
	w.cluster = nodetest.Start(tb, nodetest.Config{BinDir: w.binDir, Nodes: 1, Tables: w.homeTables()})
	if err := w.startMCPBridge(); err != nil {
		return err
	}
	return w.createGroupAndJoinAIs(level2)
}

// homeTables are the home's tables with a hook that notes when it wrote
// reviews and task_done rows.
func (w *m5World) homeTables() home.Tables {
	tables := home.NodeTables()
	tables.After = maps.Clone(tables.After)
	tables.After.Add("m5_done_order", w.recordDoneOrder)
	return tables
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
	w.workspaces[name] = workspace
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
	m5RegisterReviewSteps(sc, w)
	m5RegisterDepAndDirectSteps(sc, w)
	m5RegisterRepeatAndLeaveSteps(sc, w)
	m5RegisterJumpSteps(sc, w)
	m5RegisterFailureSteps(sc, w)
}

func m5RegisterFullFlowSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a group with a secretary and working AIs "Claude" and "Codex", each in a workspace of its own$`, func() error {
		return w.setupCluster(true)
	})
	sc.Step(`^the owner posts a feature request in the main line and the secretary proposes a two-task brief$`, w.stepRequestAndTwoTaskBrief)
	sc.Step(`^before confirm no assign item was queued on the home and no task session exists$`, w.stepVerifyBeforeConfirm)
	sc.Step(`^the owner confirms the brief and both tasks are submitted by report, reviewed by the other's AI and summarized$`, w.stepConfirmAndRunTwoTasks)
	sc.Step(`^both tasks are done with two summary cards in the main line and no approval card$`, w.stepVerifyTwoTasksDone)
	sc.Step(`^the owner saw no confirm card \(A9\.40\)$`, w.verifyNoConfirmCard)
	sc.Step(`^each passing review made its task done with no AI turn in between and a task_done row in the thread$`, w.stepVerifyNoTurnBeforeDone)
	sc.Step(`^each task ran in its own session in the workspace of its assignee, after it was done its sessions are gone, and the node's data directory holds no project folder$`, m5FinalStep(w, w.stepVerifySessionsGone))
}

func (w *m5World) stepRequestAndTwoTaskBrief() error {
	turns := append(m5BriefTurns(m5TwoTaskBriefArgs()), m5SummaryTurns("Alpha done.", "Beta done.")...)
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

func (w *m5World) stepVerifyBeforeConfirm() error {
	items, err := w.homeQueuedItems()
	if err != nil {
		return err
	}
	if i := slices.IndexFunc(items, func(it store.Item) bool { return it.Why == "assign" }); i >= 0 {
		return fmt.Errorf("an assign item before confirm: %+v", items[i])
	}
	if keys := w.taskSessionKeys(); len(keys) != 0 {
		return fmt.Errorf("task sessions before confirm: %+v", keys)
	}
	return nil
}

func (w *m5World) taskSessionKeys() []host.SessionKey {
	var keys []host.SessionKey
	for _, s := range w.cluster.Nodes[0].Host.Sessions.All() {
		if s.Kind == host.TaskSession {
			keys = append(keys, s.SessionKey)
		}
	}
	return keys
}

// stepConfirmAndRunTwoTasks runs the brief of two tasks: Claude does task 1
// and Codex reviews it; Codex does task 2, which depends on task 1, and
// Claude reviews it. Codex is held from its review of task 1 until the
// secretary has summarized task 1, so that the secretary gets one notice per
// turn.
func (w *m5World) stepConfirmAndRunTwoTasks() error {
	w.writeScriptFile("claude", m5ClaudeScript(m5WorkerCalls("Alpha is in alpha.go in my folder; go test passes.")))
	w.writeScriptFile("codex", m5CodexScript("codex-rev-1", m5PassingReview(95)))
	w.addBeforeHook("submit_review", func(host.Caller) error {
		w.writeScriptFile("codex", m5CodexScript("codex-task-2", m5WorkerCalls("Beta is in beta.go in my folder; go test passes.")))
		w.writeScriptFile("claude", m5ClaudeScript(m5PassingReview(96)))
		w.setMemberHeld(w.codexM, true)
		return nil
	})
	if err := w.confirmTwoTaskBrief(); err != nil {
		return w.twoTaskFailure(err)
	}
	if err := w.waitFor("first summary card", func() bool { return len(w.summaryCards()) >= 1 }); err != nil {
		return w.twoTaskFailure(err)
	}
	w.setMemberHeld(w.codexM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.codexM)
	if err := w.waitFor("two summary cards", func() bool { return len(w.summaryCards()) == 2 }); err != nil {
		return w.twoTaskFailure(err)
	}
	return nil
}

func (w *m5World) confirmTwoTaskBrief() error {
	// The fake secretary consumes one event per turn: confirmation, then each task_done notice.
	w.setMemberHeld(w.claudeM, true)
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	if err := w.waitFor("secretary confirmed turn started", func() bool { return w.turnCount() >= 2 }); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return nil
}

func (w *m5World) twoTaskFailure(cause error) error {
	var tasks, queued, deliveries string
	var summaries int
	n := w.cluster.Nodes[0]
	err := n.Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT
			(SELECT json_group_array(json_object('id',id,'num',data->>'num','state',data->>'state','step',data->>'step'))
				FROM row WHERE g=? AND r='task' AND gone=0),
			(SELECT json_group_array(json_object('id',id,'why',why,'task',task,'ai',ai,'not_before',not_before))
				FROM inbox WHERE g=?),
			(SELECT json_group_array(json_object('item',item,'what',what,'agent',agent,'detail',detail))
				FROM (SELECT item,what,agent,detail FROM audit
					WHERE g=? AND what IN ('delivered','refused') ORDER BY id DESC LIMIT 12)),
			(SELECT count(*) FROM row WHERE g=? AND r='msg' AND gone=0 AND data->>'kind'='summary')`,
			w.groupID, w.groupID, w.groupID, w.groupID).Scan(&tasks, &queued, &deliveries, &summaries)
	})
	log, logErr := os.ReadFile(w.cluster.LogPath)
	w.mu.Lock()
	turns := slices.Clone(w.turnTrace)
	w.mu.Unlock()
	return fmt.Errorf("%w; summaries browser=%d home=%d; home tasks=%s queued=%s; host item audit=%s; db=%v; hooks=%v; turns=%+v; fake CLI log err=%v:\n%s",
		cause, len(w.summaryCards()), summaries, tasks, queued, deliveries, err, w.checkHookErr(), turns, logErr, log)
}

func (w *m5World) summaryCards() []proto.MsgRow {
	var out []proto.MsgRow
	for _, m := range w.browserMessages() {
		if m.Kind == "summary" && m.Main {
			out = append(out, m)
		}
	}
	return out
}

func (w *m5World) verifyZeroApprovalCards() error {
	for _, m := range w.browserMessages() {
		if m.Kind == "approval" {
			return fmt.Errorf("unexpected approval card %s", m.ID)
		}
	}
	return nil
}

func (w *m5World) stepVerifyTwoTasksDone() error {
	if err := w.verifyZeroApprovalCards(); err != nil {
		return err
	}
	tasks := w.browserTasks()
	if len(tasks) != 2 || tasks[0].State != "done" || tasks[1].State != "done" {
		return fmt.Errorf("expected 2 done tasks, got %s", w.tasksBrief())
	}
	return w.verifySummaries()
}

func (w *m5World) verifySummaries() error {
	cards := w.summaryCards()
	if len(cards) != 2 {
		return fmt.Errorf("expected 2 summary cards, got %d", len(cards))
	}
	for _, c := range cards {
		var body proto.SummaryBody
		if json.Unmarshal(c.Body, &body) != nil || c.By != w.secID {
			return fmt.Errorf("unexpected summary card by=%q body=%s", c.By, string(c.Body))
		}
	}
	return nil
}

// recordDoneOrder notes the number of AI turns begun when the home wrote each
// task's last review and when it wrote its task_done row.
func (w *m5World) recordDoneOrder(_ *home.State, result home.Result) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, row := range result.Rows {
		msg, _ := row.Value().(proto.MsgRow)
		if msg.Kind == "review" && msg.Thread != nil {
			w.turnsAtReview[*msg.Thread] = w.turns
		}
		if task := m5DoneThread(msg); task != "" {
			w.turnsAtDone[task] = w.turns
		}
	}
}

// m5DoneThread is the task of a task_done row, which the client shows under
// the secretary's name (contracts/messages.md section 4).
func m5DoneThread(m proto.MsgRow) string {
	var body proto.TaskDoneBody
	if m.Kind != "system" || m.Thread == nil || json.Unmarshal(m.Body, &body) != nil {
		return ""
	}
	want := proto.TaskDoneBody{SystemCode: proto.SystemCode{Code: "task_done"}, Task: *m.Thread}
	if m.By != "system" || body != want {
		return ""
	}
	return body.Task
}

func (w *m5World) stepVerifyNoTurnBeforeDone() error {
	for _, t := range w.browserTasks() {
		w.mu.Lock()
		atReview, ok1 := w.turnsAtReview[t.ID]
		atDone, ok2 := w.turnsAtDone[t.ID]
		w.mu.Unlock()
		if !ok1 || !ok2 || atReview != atDone {
			return w.twoTaskFailure(fmt.Errorf("task %s turns at its review=%d (ok=%v), at done=%d (ok=%v)", t.ID, atReview, ok1, atDone, ok2))
		}
		if !slices.ContainsFunc(w.browserMessages(), func(m proto.MsgRow) bool { return m5DoneThread(m) == t.ID }) {
			return fmt.Errorf("task %s has no task_done row in its thread", t.ID)
		}
	}
	return nil
}

func m5DistinctPair[T comparable](a, b, zero T) bool {
	return a != zero && b != zero && a != b
}

// stepVerifySessionsGone checks where each task and review ran: task 1 by
// Claude reviewed by Codex, task 2 by Codex reviewed by Claude.
func (w *m5World) stepVerifySessionsGone() error {
	w.mu.Lock()
	keys := []host.SessionKey{w.sawTaskSess[1], w.sawTaskSess[2], w.sawRevSess[1], w.sawRevSess[2]}
	dirs := []string{w.sawTaskDir[1], w.sawTaskDir[2], w.sawRevDir[1], w.sawRevDir[2]}
	w.mu.Unlock()
	if !m5DistinctPair(keys[0], keys[1], host.SessionKey{}) || !m5DistinctPair(keys[2], keys[3], host.SessionKey{}) {
		return fmt.Errorf("tasks and reviews did not run in sessions of their own: %+v", keys)
	}
	want := []string{w.workspaces["Claude"], w.workspaces["Codex"], w.workspaces["Codex"], w.workspaces["Claude"]}
	if !slices.Equal(dirs, want) {
		return fmt.Errorf("task 1, task 2 and their reviews ran in %q, want %q", dirs, want)
	}
	if err := w.waitFor("task and review sessions gone after done", func() bool { return m5SessionsGone(w.cluster.Nodes[0].Host, keys) }); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(w.cluster.Nodes[0].Dir, "projects")); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the node's data directory has a project folder: %v", err)
	}
	return nil
}

func m5SessionsGone(h *host.Host, keys []host.SessionKey) bool {
	for _, k := range keys {
		if _, ok := h.Sessions.Get(k); ok {
			return false
		}
	}
	return true
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

func (w *m5World) turnCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.turns
}

// homeQueuedItems is the inbox of the home for the group.
func (w *m5World) homeQueuedItems() ([]store.Item, error) {
	var items []store.Item
	read := func(tx *store.Tx) error {
		var err error
		items, err = tx.Queued(w.groupID)
		return err
	}
	err := w.cluster.Nodes[0].Store.Write(read)
	return items, err
}

// release lets the host deliver to member again and delivers what it holds.
func (w *m5World) release(member string) {
	w.setMemberHeld(member, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, member)
}

func m5RegisterRepeatAndLeaveSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a group with a secretary, working AIs and a pending one-task brief$`, w.stepGivenPendingOneTaskBrief)
	sc.Step(`^the owner sends the same signed confirm twice and the host sends the same signed task fail twice$`, w.stepSendDuplicateConfirmAndFail)
	sc.Step(`^both confirms answered ok creating one task, and both task fails answered ok counting one failure$`, m5FinalStep(w, w.stepVerifyDuplicateConfirmAndFail))
	sc.Step(`^removing "Claude" or "Codex" from the group is refused with "conflict"$`, m5FinalStep(w, w.stepVerifyBusyAICannotLeave))
}

func (w *m5World) stepGivenPendingOneTaskBrief() error {
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
		if code := m5ErrCode(w.ownerRemoveMember(memberID)); code != "conflict" {
			return fmt.Errorf("removing member %s answered %q, want conflict", memberID, code)
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
