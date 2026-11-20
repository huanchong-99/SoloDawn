package host

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func init() {
	Register[Driver](Drivers, "codex", codexDriver{})
}

type codexDriver struct{}

var errCodexClosed = errors.New("codex: process closed")

// Interactive returns the codex resume command line for a terminal takeover
// (contracts/drivers.md 1.3). Codex requires an existing thread id.
func (codexDriver) Interactive(l Launch) ([]string, string, error) {
	if l.Session == "" {
		return nil, "", errors.New("codex: interactive takeover requires an existing thread id")
	}
	flags, err := codexBaseFlags(l)
	if err != nil {
		return nil, "", err
	}
	sandbox, approval, err := codexLevel(l.Level)
	if err != nil {
		return nil, "", err
	}
	argv := []string{
		"codex", "resume", l.Session,
		"--no-alt-screen", "--no-daemon",
		"-C", l.Folder,
	}
	argv = append(argv, flags...)
	argv = append(argv,
		"-s", sandbox,
		"-a", approval,
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true",
		"-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
	)
	if !l.RepoConfig {
		argv = append(argv, "-c", "project_doc_max_bytes=0")
	}
	return argv, l.Session, nil
}

// Start launches codex app-server over stdio JSON-RPC (contracts/drivers.md 1.2).
func (codexDriver) Start(ctx context.Context, l Launch) (Process, error) {
	flags, err := codexBaseFlags(l)
	if err != nil {
		return nil, err
	}
	sandbox, approval, err := codexLevel(l.Level)
	if err != nil {
		return nil, err
	}
	attrs := new(syscall.SysProcAttr)
	cmd := codexCommand(ctx, l.Folder, append([]string{"app-server"}, flags...), runtime.GOOS == "windows", reflect.ValueOf(attrs).Elem())
	cmd.SysProcAttr = attrs
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	kill := killCheckTree(cmd)
	p := &codexProcess{
		launch:   l,
		sandbox:  sandbox,
		approval: approval,
		stdin:    stdin,
		events:   make(chan Event, 16),
		ready:    make(chan struct{}),
		nextID:   3,
		asks:     map[string]json.RawMessage{},
		files:    map[string]codexFileItem{},
	}
	p.send(codexRPCRequest{
		ID:     1,
		Method: "initialize",
		Params: map[string]any{"clientInfo": map[string]string{"name": "solodawn", "version": "1"}},
	})
	go p.readLoop(stdout, func() {
		cmd.Wait()
		kill()
	})
	return p, nil
}

func codexBaseFlags(l Launch) ([]string, error) {
	if len(l.Shim) == 0 || l.Shim[0] == "" {
		return nil, errors.New("codex: missing MCP shim command")
	}
	timeout := strconv.FormatInt(int64(l.ToolTimeout.Seconds()), 10)
	return []string{
		"-c", "mcp_servers.solodawn.command=" + string(codexJSON(l.Shim[0])),
		"-c", "mcp_servers.solodawn.args=" + string(codexJSON(l.Shim[1:])),
		"-c", `mcp_servers.solodawn.default_tools_approval_mode="approve"`,
		"-c", "mcp_servers.solodawn.tool_timeout_sec=" + timeout,
		"-c", "notify=[]",
	}, nil
}

func codexLevel(level int) (sandbox, approval string, err error) {
	switch level {
	case 1:
		return "read-only", "never", nil
	case 2:
		return "workspace-write", "on-request", nil
	case 3:
		return "danger-full-access", "never", nil
	default:
		return "", "", fmt.Errorf("codex: invalid permission level %d", level)
	}
}

func codexThreadConfig(l Launch) map[string]any {
	cfg := map[string]any{}
	if l.Level == 2 {
		cfg["sandbox_workspace_write"] = map[string]bool{
			"exclude_slash_tmp":      true,
			"exclude_tmpdir_env_var": true,
		}
	}
	if !l.RepoConfig {
		cfg["project_doc_max_bytes"] = 0
	}
	return cfg
}

// codexCommand configures process-group termination on Unix and taskkill tree
// termination on Windows.
func codexCommand(ctx context.Context, dir string, args []string, windows bool, fields reflect.Value) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	if windows {
		cmd.Cancel = func() error {
			return exec.Command("taskkill.exe", "/f", "/t", "/pid", strconv.Itoa(cmd.Process.Pid)).Run()
		}
		return cmd
	}
	fields.FieldByName("Setpgid").SetBool(true)
	cmd.Cancel = func() error {
		group, _ := os.FindProcess(-cmd.Process.Pid)
		return group.Kill()
	}
	return cmd
}

func codexJSON(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// codexProcess drives one running codex app-server child.
// wmu guards stdin and closed; mu guards protocol, turn, approval and usage
// state. The two locks are never nested.
type codexProcess struct {
	launch   Launch
	sandbox  string
	approval string

	wmu    sync.Mutex
	stdin  io.WriteCloser
	closed bool

	// events is written only by readLoop and read only by the caller of Start.
	events chan Event
	// ready is closed once by finishReady; Turn is its single waiter per turn.
	ready     chan struct{}
	readyOnce sync.Once

	mu               sync.Mutex
	threadID         string
	startErr         error
	nextID           int64
	turnReqID        int64
	turnActive       bool
	turnID           string
	interruptPending bool
	turnTokens       *int64
	windowPct        *float64
	windowResets     *int64
	asks             map[string]json.RawMessage
	files            map[string]codexFileItem
}

func (p *codexProcess) Events() <-chan Event { return p.events }

func (p *codexProcess) Close() error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return p.stdin.Close()
}

func (p *codexProcess) send(v any) error {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if p.closed {
		return errCodexClosed
	}
	line := append(codexJSON(v), '\n')
	_, err := p.stdin.Write(line)
	return err
}

func (p *codexProcess) finishReady(threadID string, err error) {
	p.readyOnce.Do(func() {
		p.mu.Lock()
		p.threadID = threadID
		p.startErr = err
		p.mu.Unlock()
		close(p.ready)
	})
}

func (p *codexProcess) Turn(text string) error {
	<-p.ready
	id, threadID, err := p.beginTurn()
	if err != nil {
		return err
	}
	req := codexRPCRequest{
		ID:     id,
		Method: "turn/start",
		Params: map[string]any{
			"threadId": threadID,
			"input":    []map[string]string{{"type": "text", "text": text}},
		},
	}
	if err := p.send(req); err != nil {
		p.abortTurn()
		return err
	}
	return nil
}

func (p *codexProcess) beginTurn() (int64, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.threadID == "" {
		return 0, "", p.startErr
	}
	if p.turnActive {
		return 0, "", errors.New("codex: a turn is already running")
	}
	id := p.nextID
	p.nextID++
	p.turnReqID = id
	p.turnActive = true
	p.turnID = ""
	p.interruptPending = false
	p.turnTokens = nil
	clear(p.files)
	return id, p.threadID, nil
}

func (p *codexProcess) abortTurn() {
	p.mu.Lock()
	p.resetTurnLocked()
	p.mu.Unlock()
}

func (p *codexProcess) resetTurnLocked() {
	p.turnReqID = 0
	p.turnActive = false
	p.turnID = ""
	p.interruptPending = false
	p.turnTokens = nil
	clear(p.asks)
	clear(p.files)
}

func (p *codexProcess) Interrupt() error {
	p.mu.Lock()
	if !p.turnActive {
		p.mu.Unlock()
		return errors.New("codex: no turn is running")
	}
	if p.turnID == "" {
		p.interruptPending = true
		p.mu.Unlock()
		return nil
	}
	id := p.nextID
	p.nextID++
	threadID, turnID := p.threadID, p.turnID
	p.mu.Unlock()
	return p.sendInterrupt(id, threadID, turnID)
}

func (p *codexProcess) sendInterrupt(id int64, threadID, turnID string) error {
	return p.send(codexRPCRequest{
		ID:     id,
		Method: "turn/interrupt",
		Params: map[string]string{"threadId": threadID, "turnId": turnID},
	})
}

func (p *codexProcess) Answer(ask string, allow bool) error {
	p.mu.Lock()
	rawID, ok := p.asks[ask]
	delete(p.asks, ask)
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("codex: unknown approval request %q", ask)
	}
	decision := "decline"
	if allow {
		decision = "accept"
	}
	return p.send(codexRPCResult{ID: rawID, Result: map[string]string{"decision": decision}})
}

func (p *codexProcess) readLoop(stdout io.Reader, wait func()) {
	defer close(p.events)
	defer wait()
	defer p.finishReady("", errCodexClosed)
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		p.handleLine(line)
		if err != nil {
			return
		}
	}
}

func (p *codexProcess) handleLine(line []byte) {
	var msg codexMessage
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	if msg.Method != "" {
		if len(msg.ID) != 0 {
			p.handleRequest(msg)
			return
		}
		p.handleNotification(msg)
		return
	}
	if len(msg.ID) != 0 {
		p.handleResponse(msg)
	}
}

func (p *codexProcess) handleResponse(msg codexMessage) {
	var id int64
	if json.Unmarshal(msg.ID, &id) != nil {
		return
	}
	if msg.Error != nil {
		p.handleResponseError(id, msg.Error.Message)
		return
	}
	switch id {
	case 1:
		p.sendThreadOpen()
	case 2:
		p.onThreadReady(msg.Result)
	default:
		p.onTurnStarted(id, msg.Result)
	}
}

func (p *codexProcess) sendThreadOpen() {
	p.send(map[string]string{"method": "initialized"})
	params := map[string]any{
		"cwd":                   p.launch.Folder,
		"sandbox":               p.sandbox,
		"approvalPolicy":        p.approval,
		"developerInstructions": p.launch.Instructions,
		"config":                codexThreadConfig(p.launch),
	}
	method := "thread/start"
	if p.launch.Session != "" {
		method = "thread/resume"
		params["threadId"] = p.launch.Session
	}
	p.send(codexRPCRequest{ID: 2, Method: method, Params: params})
}

func (p *codexProcess) onThreadReady(raw json.RawMessage) {
	var res struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &res) != nil || res.Thread.ID == "" {
		p.finishReady("", errors.New("codex: missing thread id"))
		return
	}
	p.finishReady(res.Thread.ID, nil)
	p.events <- Event{Kind: Ready, Session: res.Thread.ID}
}

func (p *codexProcess) onTurnStarted(id int64, raw json.RawMessage) {
	var res struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(raw, &res) != nil || res.Turn.ID == "" {
		return
	}
	reqID, threadID, interrupt := p.recordTurnID(id, res.Turn.ID)
	if interrupt {
		p.sendInterrupt(reqID, threadID, res.Turn.ID)
	}
}

func (p *codexProcess) recordTurnID(id int64, turnID string) (int64, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id != p.turnReqID {
		return 0, "", false
	}
	p.turnID = turnID
	if !p.interruptPending {
		return 0, "", false
	}
	p.interruptPending = false
	reqID := p.nextID
	p.nextID++
	return reqID, p.threadID, true
}

func (p *codexProcess) handleResponseError(id int64, text string) {
	if id <= 2 {
		p.handleStartupError(id, text)
		return
	}
	if p.clearTurnOnError(id) {
		p.events <- Event{Kind: TurnEnded, Status: "failed", Text: text}
	}
}

func (p *codexProcess) handleStartupError(id int64, text string) {
	p.finishReady("", errors.New(text))
	if id == 2 && p.launch.Session != "" {
		p.events <- Event{Kind: NoSession, Text: text}
		return
	}
	if isCodexAuthText(text) {
		p.events <- Event{Kind: LoginInvalid, Text: text}
	}
}

func isCodexAuthText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "auth") || strings.Contains(lower, "login")
}

func (p *codexProcess) clearTurnOnError(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id != p.turnReqID {
		return false
	}
	p.resetTurnLocked()
	return true
}

func (p *codexProcess) isTurnActive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.turnActive
}

func (p *codexProcess) handleNotification(msg codexMessage) {
	if msg.Method == "turn/completed" {
		p.onTurnCompleted(msg.Params)
		return
	}
	if p.isTurnActive() {
		p.events <- Event{Kind: Active}
	}
	switch msg.Method {
	case "thread/tokenUsage/updated":
		p.onTokenUsage(msg.Params)
	case "account/rateLimits/updated":
		p.onRateLimits(msg.Params)
	default:
		p.onItemNotification(msg.Method, msg.Params)
	}
}

func (p *codexProcess) onItemNotification(method string, raw json.RawMessage) {
	if method != "item/started" && method != "item/completed" {
		return
	}
	var note codexItemParams
	if json.Unmarshal(raw, &note) != nil || note.Item.Type != "fileChange" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if method == "item/completed" {
		delete(p.files, note.Item.ID)
		return
	}
	p.files[note.Item.ID] = note.Item
}

func (p *codexProcess) onTokenUsage(raw json.RawMessage) {
	var note codexTokenParams
	if json.Unmarshal(raw, &note) != nil {
		return
	}
	usage, ok := p.addTurnTokens(note.TurnID, note.TokenUsage.Last.TotalTokens)
	if ok {
		p.events <- Event{Kind: Used, Usage: usage}
	}
}

func (p *codexProcess) addTurnTokens(turnID string, delta int64) (Usage, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.turnID == "" || turnID != p.turnID {
		return Usage{}, false
	}
	var next int64
	if p.turnTokens != nil {
		next = *p.turnTokens
	}
	next += delta
	p.turnTokens = new(next)
	return Usage{
		Tokens:       new(next),
		WindowPct:    cloneCodexPtr(p.windowPct),
		WindowResets: cloneCodexPtr(p.windowResets),
	}, true
}

func (p *codexProcess) onRateLimits(raw json.RawMessage) {
	var note struct {
		RateLimits codexRateLimits `json:"rateLimits"`
	}
	if json.Unmarshal(raw, &note) != nil {
		return
	}
	usage, resets, hasWindow, limited := p.updateRateLimits(note.RateLimits)
	if hasWindow {
		p.events <- Event{Kind: Used, Usage: usage}
	}
	if limited {
		p.events <- Event{Kind: RateLimited, Resets: resets}
	}
}

func (p *codexProcess) updateRateLimits(rl codexRateLimits) (Usage, int64, bool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	win := selectRateWindow(rl.Primary, rl.Secondary)
	var usage Usage
	if win != nil {
		p.windowPct = new(win.UsedPercent)
		p.windowResets = codexWindowResetsMs(win.ResetsAt)
		usage = Usage{
			WindowPct:    cloneCodexPtr(p.windowPct),
			WindowResets: cloneCodexPtr(p.windowResets),
		}
	}
	var resets int64
	if p.windowResets != nil {
		resets = *p.windowResets
	}
	return usage, resets, win != nil, codexRateLimited(rl, win)
}

func selectRateWindow(primary, secondary *codexLimitWindow) *codexLimitWindow {
	if primary == nil {
		return secondary
	}
	if secondary != nil && secondary.UsedPercent > primary.UsedPercent {
		return secondary
	}
	return primary
}

func codexWindowResetsMs(sec *int64) *int64 {
	if sec == nil {
		return nil
	}
	return new(*sec * 1000)
}

func codexRateLimited(rl codexRateLimits, win *codexLimitWindow) bool {
	if win != nil && win.UsedPercent >= 100 {
		return true
	}
	return rl.RateLimitReachedType != "" || rl.SpendControlReached
}

func (p *codexProcess) onTurnCompleted(raw json.RawMessage) {
	var note codexTurnCompleted
	if json.Unmarshal(raw, &note) != nil {
		return
	}
	usage, resets := p.finishTurn()
	var errText string
	if note.Turn.Error != nil {
		errText = note.Turn.Error.Message
		p.emitTurnError(note.Turn.Error.CodexErrorInfo, errText, resets)
	}
	p.events <- Event{Kind: TurnEnded, Status: note.Turn.Status, Usage: usage, Text: errText}
}

func (p *codexProcess) finishTurn() (Usage, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	usage := Usage{
		Tokens:       cloneCodexPtr(p.turnTokens),
		WindowPct:    cloneCodexPtr(p.windowPct),
		WindowResets: cloneCodexPtr(p.windowResets),
	}
	var resets int64
	if p.windowResets != nil {
		resets = *p.windowResets
	}
	p.resetTurnLocked()
	return usage, resets
}

func (p *codexProcess) emitTurnError(info json.RawMessage, text string, resets int64) {
	switch codexTurnErrorKind(info) {
	case LoginInvalid:
		p.events <- Event{Kind: LoginInvalid, Text: text}
	case RateLimited:
		p.events <- Event{Kind: RateLimited, Resets: resets}
	}
}

func codexTurnErrorKind(info json.RawMessage) EventKind {
	var code string
	if json.Unmarshal(info, &code) == nil {
		if code == "unauthorized" {
			return LoginInvalid
		}
		if code == "usageLimitExceeded" {
			return RateLimited
		}
		return ""
	}
	if codexHTTPStatus(info) == 401 {
		return LoginInvalid
	}
	return ""
}

func codexHTTPStatus(info json.RawMessage) int {
	var detail map[string]struct {
		Status int `json:"httpStatusCode"`
	}
	if json.Unmarshal(info, &detail) != nil {
		return 0
	}
	for _, v := range detail {
		return v.Status
	}
	return 0
}

func (p *codexProcess) handleRequest(msg codexMessage) {
	switch msg.Method {
	case "item/commandExecution/requestApproval":
		p.onCommandApproval(msg.ID, msg.Params)
	case "item/fileChange/requestApproval":
		p.onFileApproval(msg.ID, msg.Params)
	default:
		p.declineRequest(msg.ID, msg.Method)
	}
}

func (p *codexProcess) onCommandApproval(rawID, raw json.RawMessage) {
	var req struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
		Reason  string `json:"reason"`
	}
	if json.Unmarshal(raw, &req) != nil {
		p.declineRequest(rawID, "")
		return
	}
	input := codexJSON(codexCommandInput{Command: req.Command, Cwd: req.Cwd})
	ask := p.registerAsk(rawID, "commandExecution", "command", req.Command, req.Reason, input)
	p.events <- Event{Kind: Asked, Ask: ask}
}

func (p *codexProcess) onFileApproval(rawID, raw json.RawMessage) {
	var req struct {
		ItemID string `json:"itemId"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(raw, &req) != nil {
		p.declineRequest(rawID, "")
		return
	}
	item := p.lookupFileItem(req.ItemID)
	input := item.RawChanges
	if len(input) == 0 {
		input = json.RawMessage("[]")
	}
	var changes []codexFileChange
	json.Unmarshal(input, &changes)
	ask := p.registerAsk(rawID, "fileChange", "file", formatFileChanges(changes), req.Reason, input)
	p.events <- Event{Kind: Asked, Ask: ask}
}

func (p *codexProcess) lookupFileItem(itemID string) codexFileItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.files[itemID]
}

func (p *codexProcess) registerAsk(rawID json.RawMessage, tool, category, command, why string, input json.RawMessage) *Ask {
	id := rand.Text()
	p.mu.Lock()
	p.asks[id] = rawID
	p.mu.Unlock()
	return &Ask{
		ID:       id,
		Tool:     tool,
		Category: category,
		Command:  command,
		Why:      why,
		Input:    input,
	}
}

func formatFileChanges(changes []codexFileChange) string {
	lines := make([]string, 0, len(changes))
	for _, ch := range changes {
		head := ch.Kind.Type + " " + ch.Path
		if ch.Kind.MovePath != "" {
			head += " -> " + ch.Kind.MovePath
		}
		if ch.Diff != "" {
			head += "\n" + strings.TrimSuffix(ch.Diff, "\n")
		}
		lines = append(lines, head)
	}
	return strings.Join(lines, "\n")
}

func (p *codexProcess) declineRequest(rawID json.RawMessage, method string) {
	switch method {
	case "mcpServer/elicitation/request":
		p.send(codexRPCResult{ID: rawID, Result: map[string]string{"action": "decline"}})
	case "item/permissions/requestApproval":
		p.send(codexRPCResult{ID: rawID, Result: map[string]any{"permissions": map[string]any{}}})
	case "item/tool/requestUserInput":
		p.send(codexRPCResult{ID: rawID, Result: map[string]any{"answers": map[string]any{}}})
	case "item/tool/call":
		p.send(codexRPCResult{ID: rawID, Result: map[string]any{"contentItems": []any{}, "success": false}})
	default:
		p.send(codexRPCErrorReply{ID: rawID, Error: codexRPCError{Code: -32601, Message: "unsupported server request"}})
	}
}

func cloneCodexPtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	return new(*v)
}

type codexRPCRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type codexRPCResult struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result"`
}

type codexRPCErrorReply struct {
	ID    json.RawMessage `json:"id"`
	Error codexRPCError   `json:"error"`
}

type codexRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type codexMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *codexRPCError  `json:"error"`
}

type codexCommandInput struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

type codexItemParams struct {
	Item codexFileItem `json:"item"`
}

type codexFileItem struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	RawChanges json.RawMessage `json:"changes"`
}

type codexFileChange struct {
	Path string `json:"path"`
	Kind struct {
		Type     string `json:"type"`
		MovePath string `json:"move_path"`
	} `json:"kind"`
	Diff string `json:"diff"`
}

type codexTokenParams struct {
	TurnID     string `json:"turnId"`
	TokenUsage struct {
		Last struct {
			TotalTokens int64 `json:"totalTokens"`
		} `json:"last"`
	} `json:"tokenUsage"`
}

type codexRateLimits struct {
	Primary              *codexLimitWindow `json:"primary"`
	Secondary            *codexLimitWindow `json:"secondary"`
	SpendControlReached  bool              `json:"spendControlReached"`
	RateLimitReachedType string            `json:"rateLimitReachedType"`
}

type codexLimitWindow struct {
	UsedPercent float64 `json:"usedPercent"`
	ResetsAt    *int64  `json:"resetsAt"`
}

type codexTurnCompleted struct {
	Turn struct {
		Status string          `json:"status"`
		Error  *codexTurnError `json:"error"`
	} `json:"turn"`
}

type codexTurnError struct {
	Message        string          `json:"message"`
	CodexErrorInfo json.RawMessage `json:"codexErrorInfo"`
}
