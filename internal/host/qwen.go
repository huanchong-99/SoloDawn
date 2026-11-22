package host

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	Register(Drivers, "qwen", Driver(qwenDriver{}))
	Register(DriverCaps, "qwen", qwenCapabilities(runtime.GOOS))
}

func qwenCapabilities(goos string) Capabilities {
	levels := []int{1, 3}
	if goos == "darwin" {
		levels = []int{1, 2, 3}
	}
	return Capabilities{
		Accounts:  []string{AccountAPIKey},
		Levels:    levels,
		Approvals: ApprovalConn,
		Takeover:  TakeoverNone,
		Usage:     []string{},
	}
}

type qwenDriver struct{}

var errQwenClosed = errors.New("qwen: process closed")

// Interactive refuses terminal takeover because Qwen Code's same-id handoff
// has not been verified (planning/contracts/step-3/drivers.md section 8).
func (qwenDriver) Interactive(Launch) ([]string, string, error) {
	return nil, "", &Error{Code: "conflict", Message: "qwen takeover is none"}
}

// Start launches qwen serve over loopback HTTP and SSE
// (planning/contracts/step-3/drivers.md sections 3 and 8).
func (qwenDriver) Start(ctx context.Context, l Launch) (Process, error) {
	cfgDir, mode, env, err := qwenPrepareConfig(l, runtime.GOOS)
	if err != nil {
		return nil, err
	}
	token := newQwenToken()
	cmd := exec.CommandContext(ctx, "qwen", qwenServeArgs(l.Folder, token)...)
	cmd.Dir = l.Folder
	cmd.Env = env
	detachTerminal(cmd)
	cmd.WaitDelay = time.Second
	stdin, stdout, stderr, err := spawnQwenDaemon(cmd)
	if err != nil {
		_ = os.RemoveAll(cfgDir)
		return nil, err
	}
	killTree := killCheckTree(cmd)
	p := newQwenProcess(l, mode, token, stdin, cmd.Process)
	go p.run(cmd, stdout, stderr, killTree, cfgDir)
	return p, nil
}

func qwenServeArgs(folder, token string) []string {
	return []string{
		"serve",
		"--hostname", "127.0.0.1",
		"--port", "0",
		"--token", token,
		"--no-web",
		"--no-open",
		"--workspace", folder,
		"--permission-response-timeout-ms", "0",
	}
}

func newQwenToken() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func spawnQwenDaemon(cmd *exec.Cmd) (*os.File, *os.File, *checkTail, error) {
	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	var stderr checkTail
	cmd.Stdin = inR
	cmd.Stdout = outW
	cmd.Stderr = &stderr
	err := cmd.Start()
	_ = inR.Close()
	_ = outW.Close()
	if err != nil {
		_ = inW.Close()
		_ = outR.Close()
		return nil, nil, nil, err
	}
	return inW, outR, &stderr, nil
}

type qwenLevelSpec struct {
	mode    string
	deny    []string
	sandbox any
	env     []string
}

func qwenPrepareConfig(l Launch, goos string) (string, string, []string, error) {
	if len(l.Shim) == 0 || l.Shim[0] == "" {
		return "", "", nil, &Error{Code: "bad_request", Message: "missing MCP shim command"}
	}
	spec, err := qwenLevel(l.Level, goos)
	if err != nil {
		return "", "", nil, err
	}
	cfgDir, env := writeQwenConfigFiles(l, spec)
	return cfgDir, spec.mode, env, nil
}

func qwenLevel(level int, goos string) (qwenLevelSpec, error) {
	switch level {
	case 1:
		return qwenLevelSpec{
			mode: "plan", deny: []string{"run_shell_command", "write_file", "edit"},
			sandbox: false, env: []string{"QWEN_SANDBOX=false"},
		}, nil
	case 2:
		if goos != "darwin" {
			return qwenLevelSpec{}, &Error{Code: "conflict", Message: "Qwen Code has no native sandbox for level 2 on " + goos}
		}
		return qwenLevelSpec{
			mode: "auto-edit", sandbox: "sandbox-exec",
			env: []string{"QWEN_SANDBOX=sandbox-exec", "SEATBELT_PROFILE=permissive-open"},
		}, nil
	case 3:
		return qwenLevelSpec{
			mode: "yolo", sandbox: false,
			env: []string{"QWEN_SANDBOX=false", "QWEN_CODE_SUPPRESS_YOLO_WARNING=1"},
		}, nil
	default:
		return qwenLevelSpec{}, &Error{Code: "bad_request", Message: fmt.Sprintf("invalid permission level %d", level)}
	}
}

type qwenSettingsFile struct {
	MCPServers      map[string]qwenMCPServerCfg `json:"mcpServers"`
	MCP             qwenMCPAllowCfg             `json:"mcp"`
	Tools           qwenToolsCfg                `json:"tools"`
	Permissions     qwenPermissionsCfg          `json:"permissions"`
	Security        qwenSecurityCfg             `json:"security"`
	Context         *qwenContextCfg             `json:"context,omitempty"`
	DisableAllHooks bool                        `json:"disableAllHooks,omitempty"`
}

type qwenMCPServerCfg struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int64    `json:"timeout,omitempty"`
	Trust   bool     `json:"trust"`
}

type qwenMCPAllowCfg struct {
	Allowed []string `json:"allowed"`
}

type qwenToolsCfg struct {
	ApprovalMode string `json:"approvalMode"`
	Sandbox      any    `json:"sandbox"`
}

type qwenPermissionsCfg struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny,omitempty"`
}

type qwenSecurityCfg struct {
	FolderTrust qwenFolderTrustCfg `json:"folderTrust"`
}

type qwenFolderTrustCfg struct {
	Enabled bool `json:"enabled"`
}

type qwenContextCfg struct {
	FileName string `json:"fileName"`
}

func writeQwenConfigFiles(l Launch, spec qwenLevelSpec) (string, []string) {
	cfgDir, _ := os.MkdirTemp("", "solodawn-qwen-*")
	cfg := qwenSettingsFile{
		MCPServers: map[string]qwenMCPServerCfg{
			"solodawn": {
				Command: l.Shim[0],
				Args:    slices.Clone(l.Shim[1:]),
				Timeout: max(l.ToolTimeout.Milliseconds(), 0),
				Trust:   true,
			},
		},
		MCP:         qwenMCPAllowCfg{Allowed: []string{"solodawn"}},
		Tools:       qwenToolsCfg{ApprovalMode: spec.mode, Sandbox: spec.sandbox},
		Permissions: qwenPermissionsCfg{Allow: []string{"mcp__solodawn"}, Deny: spec.deny},
		Security:    qwenSecurityCfg{FolderTrust: qwenFolderTrustCfg{Enabled: !l.RepoConfig}},
	}
	if !l.RepoConfig {
		cfg.DisableAllHooks = true
		cfg.Context = &qwenContextCfg{FileName: "__solodawn_none__"}
	}
	settingsPath := filepath.Join(cfgDir, "settings.json")
	_ = os.WriteFile(settingsPath, []byte(marshalWire(cfg)), 0o600)
	var sysPath string
	if l.Instructions != "" {
		sysPath = filepath.Join(cfgDir, "system.md")
		_ = os.WriteFile(sysPath, []byte(l.Instructions), 0o600)
	}
	return cfgDir, qwenMergeEnv(os.Environ(), settingsPath, sysPath, spec.env)
}

func qwenMergeEnv(base []string, settingsPath, sysPath string, extra []string) []string {
	overrides := append([]string{"QWEN_CODE_SYSTEM_SETTINGS_PATH=" + settingsPath}, extra...)
	if sysPath != "" {
		overrides = append(overrides, "QWEN_SYSTEM_MD="+sysPath)
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		if !qwenHasOverride(overrides, entry) {
			out = append(out, entry)
		}
	}
	return append(out, overrides...)
}

func qwenHasOverride(overrides []string, entry string) bool {
	key, _, _ := strings.Cut(entry, "=")
	for _, ov := range overrides {
		if strings.HasPrefix(ov, key+"=") {
			return true
		}
	}
	return false
}

type qwenPendingAsk struct {
	reqID     string
	allowOpt  string
	rejectOpt string
}

// qwenProcess drives one running qwen serve daemon child.
// Lock order: mu is a leaf lock guarding in-memory state only; nothing under mu
// performs I/O, waits on channels, or acquires any other lock.
type qwenProcess struct {
	launch     Launch
	mode       string
	token      string
	startedNs  int64
	stdin      *os.File
	osProc     *os.Process
	httpClient *http.Client
	httpCtx    context.Context
	httpCancel context.CancelFunc

	// events is written only by run and read only by the caller of Start.
	events chan Event
	// ready is closed once by finishReady; Turn waits on it before sending prompts.
	ready     chan struct{}
	readyOnce sync.Once

	mu          sync.Mutex
	baseURL     string
	sessionID   string
	startErr    error
	closed      bool
	turnActive  bool
	interrupted bool
	turnTokens  *int64
	asks        map[string]qwenPendingAsk
	questions   map[string]qwenPendingAsk
}

func newQwenProcess(l Launch, mode, token string, stdin *os.File, osProc *os.Process) *qwenProcess {
	httpCtx, httpCancel := context.WithCancel(context.Background())
	return &qwenProcess{
		launch:     l,
		mode:       mode,
		token:      token,
		startedNs:  time.Now().UnixNano(),
		stdin:      stdin,
		osProc:     osProc,
		httpClient: &http.Client{},
		httpCtx:    httpCtx,
		httpCancel: httpCancel,
		// Single reader: the caller of Process.Events (the session runtime).
		events: make(chan Event, 16),
		// Single closer: finishReady via readyOnce.
		ready:     make(chan struct{}),
		asks:      map[string]qwenPendingAsk{},
		questions: map[string]qwenPendingAsk{},
	}
}

func (p *qwenProcess) Events() <-chan Event { return p.events }

func (p *qwenProcess) finishReady(baseURL, sessionID string, err error) {
	p.readyOnce.Do(func() {
		p.mu.Lock()
		p.baseURL = baseURL
		p.sessionID = sessionID
		p.startErr = err
		p.mu.Unlock()
		close(p.ready)
	})
}

func (p *qwenProcess) Close() error {
	baseURL, sid, ok := p.claimClose()
	if !ok {
		return nil
	}
	if baseURL != "" && sid != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_, _, _ = p.doHTTP(ctx, http.MethodDelete, baseURL+"/session/"+sid, nil)
		cancel()
	}
	p.stopDaemon()
	return nil
}

func (p *qwenProcess) stopDaemon() {
	p.httpCancel()
	_ = p.stdin.Close()
	signalQwenStop(p.osProc)
}

func (p *qwenProcess) claimClose() (string, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return "", "", false
	}
	p.closed = true
	return p.baseURL, p.sessionID, true
}

func (p *qwenProcess) Turn(text string) error {
	<-p.ready
	baseURL, sid, err := p.beginTurn()
	if err != nil {
		return err
	}
	body := map[string]any{
		"prompt": []map[string]string{{"type": "text", "text": text}},
	}
	if _, err := p.postJSON(baseURL+"/session/"+sid+"/prompt", body); err != nil {
		p.clearTurn()
		return err
	}
	return nil
}

func (p *qwenProcess) beginTurn() (string, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return "", "", errQwenClosed
	}
	if p.sessionID == "" {
		return "", "", p.startErr
	}
	if p.turnActive {
		return "", "", errors.New("qwen: a turn is already running")
	}
	p.turnActive = true
	p.interrupted = false
	p.turnTokens = nil
	return p.baseURL, p.sessionID, nil
}

func (p *qwenProcess) clearTurn() (bool, *int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	wasInterrupted := p.interrupted
	tokens := p.turnTokens
	p.turnActive = false
	p.interrupted = false
	p.turnTokens = nil
	clear(p.asks)
	clear(p.questions)
	return wasInterrupted, tokens
}

func (p *qwenProcess) Interrupt() error {
	baseURL, sid, err := p.markInterrupted()
	if err != nil {
		return err
	}
	_, err = p.postJSON(baseURL+"/session/"+sid+"/cancel", nil)
	return err
}

func (p *qwenProcess) markInterrupted() (string, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.turnActive {
		return "", "", errors.New("qwen: no turn is running")
	}
	p.interrupted = true
	return p.baseURL, p.sessionID, nil
}

func (p *qwenProcess) Answer(ask string, allow bool) error {
	pending, baseURL, sid, ok := p.takePending(p.asks, ask)
	if !ok {
		return fmt.Errorf("qwen: unknown approval request %q", ask)
	}
	_, err := p.postJSON(baseURL+"/session/"+sid+"/permission/"+pending.reqID, qwenVoteBody(pending, allow, nil))
	return err
}

func (p *qwenProcess) replyQuestion(askID string, answers map[string][]string) error {
	pending, baseURL, sid, ok := p.takePending(p.questions, askID)
	if !ok {
		return fmt.Errorf("qwen: unknown question request %q", askID)
	}
	flat := flattenQwenAnswers(answers)
	_, err := p.postJSON(baseURL+"/session/"+sid+"/permission/"+pending.reqID, qwenVoteBody(pending, len(flat) > 0, flat))
	return err
}

func (p *qwenProcess) takePending(table map[string]qwenPendingAsk, id string) (qwenPendingAsk, string, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pending, ok := table[id]
	delete(table, id)
	return pending, p.baseURL, p.sessionID, ok
}

func (p *qwenProcess) recordPending(table map[string]qwenPendingAsk, item qwenPendingAsk) string {
	askID := fmt.Sprintf("%x:%s", p.startedNs, item.reqID)
	p.mu.Lock()
	table[askID] = item
	p.mu.Unlock()
	return askID
}

func flattenQwenAnswers(answers map[string][]string) map[string]string {
	if len(answers) == 0 {
		return nil
	}
	out := make(map[string]string, len(answers))
	for k, vals := range answers {
		if len(vals) > 0 {
			out[k] = strings.Join(vals, "\n")
		}
	}
	return out
}

func qwenVoteBody(pending qwenPendingAsk, allow bool, answers map[string]string) map[string]any {
	opt := pending.rejectOpt
	if allow {
		opt = pending.allowOpt
	}
	if opt == "" {
		return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}
	}
	body := map[string]any{
		"outcome": map[string]string{"outcome": "selected", "optionId": opt},
	}
	if len(answers) > 0 {
		body["answers"] = answers
	}
	return body
}

func (p *qwenProcess) run(cmd *exec.Cmd, stdout *os.File, stderr *checkTail, killTree func(), cfgDir string) {
	reportedDiag := p.serveSession(stdout)
	_ = stdout.Close()
	waitErr := cmd.Wait()
	killTree()
	p.httpCancel()
	_ = p.stdin.Close()
	_ = os.RemoveAll(cfgDir)
	p.finishReady("", "", errQwenClosed)
	if waitErr != nil && !reportedDiag {
		p.emitExitDiag(strings.TrimSpace(string(*stderr)))
	}
	close(p.events)
}

func (p *qwenProcess) serveSession(stdout *os.File) bool {
	reader := bufio.NewReader(stdout)
	baseURL, ok := readQwenListenURL(reader)
	if !ok {
		return false
	}
	go io.Copy(io.Discard, reader)
	sid, diag, err := p.openSession(baseURL)
	if err != nil {
		p.finishReady("", "", err)
		p.stopDaemon()
		return diag
	}
	sseBody, err := p.openEventStream(baseURL, sid)
	if err != nil {
		p.finishReady("", "", err)
		p.stopDaemon()
		return false
	}
	p.events <- Event{Kind: Ready, Session: sid}
	p.finishReady(baseURL, sid, nil)
	return p.readEventStream(sseBody)
}

func readQwenListenURL(reader *bufio.Reader) (string, bool) {
	for {
		line, err := reader.ReadString('\n')
		if url, ok := extractLoopbackURL(line); ok {
			return url, true
		}
		if err != nil {
			return "", false
		}
	}
}

func extractLoopbackURL(line string) (string, bool) {
	idx := strings.Index(line, "http://127.0.0.1:")
	if idx < 0 {
		return "", false
	}
	rest := line[idx:]
	end := strings.IndexAny(rest, " \t\r\n)")
	if end > 0 {
		rest = rest[:end]
	}
	return rest, true
}

func (p *qwenProcess) openSession(baseURL string) (string, bool, error) {
	if p.launch.Session != "" {
		return p.resumeSession(baseURL)
	}
	body := map[string]any{"cwd": p.launch.Folder, "sessionScope": "thread"}
	status, raw, err := p.doHTTP(p.httpCtx, http.MethodPost, baseURL+"/session", body)
	if err != nil || status >= 300 {
		return "", p.reportStartupHTTPFailure(status, raw, false), cmp.Or(err, errors.New(qwenErrorMessage(raw, status)))
	}
	sid := parseQwenSessionID(raw, "")
	if sid == "" {
		return "", false, errors.New("qwen: missing sessionId")
	}
	_, _, _ = p.doHTTP(p.httpCtx, http.MethodPost, baseURL+"/session/"+sid+"/approval-mode", map[string]any{"mode": p.mode, "persist": false})
	return sid, false, nil
}

func (p *qwenProcess) resumeSession(baseURL string) (string, bool, error) {
	body := map[string]any{"cwd": p.launch.Folder, "approvalMode": p.mode}
	status, raw, err := p.doHTTP(p.httpCtx, http.MethodPost, baseURL+"/session/"+p.launch.Session+"/resume", body)
	if err != nil || status >= 300 {
		return "", p.reportStartupHTTPFailure(status, raw, true), cmp.Or(err, errors.New(qwenErrorMessage(raw, status)))
	}
	return parseQwenSessionID(raw, p.launch.Session), false, nil
}

func parseQwenSessionID(raw []byte, fallback string) string {
	var res struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.Unmarshal(raw, &res)
	return cmp.Or(res.SessionID, fallback)
}

func (p *qwenProcess) reportStartupHTTPFailure(status int, raw []byte, resumed bool) bool {
	msg := qwenErrorMessage(raw, status)
	if resumed && status == http.StatusNotFound {
		p.events <- Event{Kind: NoSession, Text: msg}
		return true
	}
	if status == http.StatusUnauthorized || isQwenAuthText(msg) {
		p.events <- Event{Kind: LoginInvalid, Text: msg}
		return true
	}
	return false
}

func qwenErrorMessage(raw []byte, status int) string {
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &body)
	return cmp.Or(strings.TrimSpace(body.Error), strings.TrimSpace(body.Message), strings.TrimSpace(string(raw)), fmt.Sprintf("HTTP %d", status))
}

func (p *qwenProcess) postJSON(url string, body any) ([]byte, error) {
	status, raw, err := p.doHTTP(p.httpCtx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("qwen: %s", qwenErrorMessage(raw, status))
	}
	return raw, nil
}

func (p *qwenProcess) doHTTP(ctx context.Context, method, url string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(marshalWire(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, nil
}

func (p *qwenProcess) openEventStream(baseURL, sid string) (io.ReadCloser, error) {
	req, _ := http.NewRequestWithContext(p.httpCtx, http.MethodGet, baseURL+"/session/"+sid+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("qwen events: %s", qwenErrorMessage(raw, resp.StatusCode))
	}
	return resp.Body, nil
}

type qwenSSEEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (p *qwenProcess) readEventStream(body io.ReadCloser) bool {
	defer body.Close()
	reportedDiag := false
	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		stop, diag := p.handleSSELine(line)
		reportedDiag = reportedDiag || diag
		if stop || err != nil {
			if stop {
				p.stopDaemon()
			}
			return reportedDiag
		}
	}
}

func (p *qwenProcess) handleSSELine(line string) (bool, bool) {
	payload, ok := strings.CutPrefix(strings.TrimSpace(line), "data:")
	if !ok {
		return false, false
	}
	var env qwenSSEEnvelope
	if json.Unmarshal([]byte(strings.TrimSpace(payload)), &env) != nil {
		return false, false
	}
	return p.dispatchQwenEvent(env.Type, env.Data)
}

func (p *qwenProcess) dispatchQwenEvent(typ string, data json.RawMessage) (bool, bool) {
	switch typ {
	case "session_update":
		p.onSessionUpdate(data)
	case "permission_request":
		p.onPermissionRequest(data)
	case "turn_complete":
		p.onTurnComplete(data)
	case "turn_error":
		return false, p.onTurnError(data)
	case "session_died", "stream_error":
		return true, false
	}
	return false, false
}

func (p *qwenProcess) onSessionUpdate(data json.RawMessage) {
	tokens := extractQwenTokens(data)
	p.mu.Lock()
	active := p.turnActive
	if tokens != nil {
		p.turnTokens = tokens
	}
	p.mu.Unlock()
	if active {
		p.events <- Event{Kind: Active}
	}
}

func extractQwenTokens(data json.RawMessage) *int64 {
	var msg struct {
		Usage struct {
			TotalTokens *int64 `json:"totalTokens"`
			SnakeTokens *int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &msg) != nil {
		return nil
	}
	return cmp.Or(msg.Usage.TotalTokens, msg.Usage.SnakeTokens)
}

func (p *qwenProcess) onTurnComplete(data json.RawMessage) {
	var msg struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(data, &msg)
	wasInterrupted, prevTokens := p.clearTurn()
	tokens := cmp.Or(extractQwenTokens(data), prevTokens)
	status := "completed"
	if wasInterrupted || msg.StopReason == "cancelled" {
		status = "interrupted"
	}
	p.events <- Event{Kind: TurnEnded, Status: status, Usage: Usage{Tokens: tokens}}
}

func (p *qwenProcess) onTurnError(data json.RawMessage) bool {
	var msg struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal(data, &msg)
	p.clearTurn()
	text := strings.TrimSpace(msg.Message)
	diag := isQwenAuthText(msg.Code + " " + text)
	if diag {
		p.events <- Event{Kind: LoginInvalid, Text: text}
	}
	p.events <- Event{Kind: TurnEnded, Status: "failed", Text: text}
	return diag
}

type qwenPermRequest struct {
	RequestID string           `json:"requestId"`
	ToolCall  qwenToolCall     `json:"toolCall"`
	Options   []qwenPermOption `json:"options"`
}

type qwenPermOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
}

type qwenToolCall struct {
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	RawInput json.RawMessage `json:"rawInput"`
	Meta     struct {
		ToolName string `json:"toolName"`
	} `json:"_meta"`
}

func (p *qwenProcess) onPermissionRequest(data json.RawMessage) {
	var req qwenPermRequest
	if json.Unmarshal(data, &req) != nil || req.RequestID == "" {
		return
	}
	allowOpt, rejectOpt := selectQwenPermOptions(req.Options)
	pending := qwenPendingAsk{reqID: req.RequestID, allowOpt: allowOpt, rejectOpt: rejectOpt}
	tool := cmp.Or(req.ToolCall.Meta.ToolName, req.ToolCall.Name, req.ToolCall.Kind, "tool")
	if tool == "ask_user_question" {
		p.onQuestionPermission(pending, req.ToolCall.RawInput)
		return
	}
	ask := buildQwenToolAsk(tool, req.ToolCall.Kind, req.ToolCall.RawInput)
	ask.ID = p.recordPending(p.asks, pending)
	p.events <- Event{Kind: Asked, Ask: &ask}
}

func selectQwenPermOptions(opts []qwenPermOption) (allowOpt, rejectOpt string) {
	for _, opt := range opts {
		if strings.HasPrefix(opt.Kind, "allow") && allowOpt == "" {
			allowOpt = opt.OptionID
		}
		if strings.HasPrefix(opt.Kind, "reject") && rejectOpt == "" {
			rejectOpt = opt.OptionID
		}
	}
	return allowOpt, rejectOpt
}

func (p *qwenProcess) onQuestionPermission(pending qwenPendingAsk, rawInput json.RawMessage) {
	normalized, ok := normalizeQwenQuestions(rawInput)
	if !ok {
		p.cancelInvalidQuestion(pending)
		return
	}
	askID := p.recordPending(p.questions, pending)
	reply := func(answers map[string][]string) error {
		return p.replyQuestion(askID, answers)
	}
	p.events <- Event{Kind: Asked, Ask: &Ask{
		ID:    askID,
		Tool:  "ask_user_question",
		Input: normalized,
		Reply: reply,
	}}
}

func (p *qwenProcess) cancelInvalidQuestion(pending qwenPendingAsk) {
	p.mu.Lock()
	baseURL, sid := p.baseURL, p.sessionID
	p.mu.Unlock()
	_, _ = p.postJSON(baseURL+"/session/"+sid+"/permission/"+pending.reqID, qwenVoteBody(pending, false, nil))
}

type qwenRawQuestion struct {
	ID          string           `json:"id"`
	AnswerKey   string           `json:"answerKey"`
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []cliQuestionOpt `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
	IsSecret    bool             `json:"isSecret"`
}

func normalizeQwenQuestions(raw json.RawMessage) (json.RawMessage, bool) {
	var in struct {
		Questions []qwenRawQuestion `json:"questions"`
	}
	if json.Unmarshal(raw, &in) != nil || len(in.Questions) == 0 {
		return nil, false
	}
	out := make([]cliQuestion, len(in.Questions))
	for i, q := range in.Questions {
		if q.Question == "" {
			return nil, false
		}
		out[i] = cliQuestion{
			ID: cmp.Or(q.AnswerKey, q.ID, strconv.Itoa(i)), Question: q.Question,
			Header: q.Header, Options: q.Options, MultiSelect: q.MultiSelect, IsSecret: q.IsSecret,
		}
	}
	return json.RawMessage(marshalWire(cliAskInput{Questions: out})), true
}

func buildQwenToolAsk(tool, kind string, rawInput json.RawMessage) Ask {
	var compact bytes.Buffer
	if len(rawInput) == 0 || json.Compact(&compact, rawInput) != nil {
		compact.Reset()
		compact.WriteString("{}")
	}
	inputBytes := json.RawMessage(compact.Bytes())
	var fields struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(inputBytes, &fields)
	return Ask{
		Tool:     tool,
		Category: qwenToolCategory(tool, kind),
		Command:  cmp.Or(fields.Command, string(inputBytes)),
		Why:      fields.Description,
		Input:    inputBytes,
	}
}

func qwenToolCategory(tool, kind string) string {
	switch {
	case tool == "run_shell_command" || kind == "execute":
		return "command"
	case tool == "write_file" || tool == "edit" || tool == "read_file" || kind == "edit":
		return "file"
	case tool == "web_fetch" || tool == "web_search" || kind == "fetch":
		return "network"
	default:
		return "other"
	}
}

func (p *qwenProcess) emitExitDiag(stderr string) {
	if isQwenAuthText(stderr) {
		p.events <- Event{Kind: LoginInvalid, Text: stderr}
	}
}

func isQwenAuthText(text string) bool {
	lower := strings.ToLower(text)
	for _, sub := range []string{"401", "unauthorized", "invalid api key", "invalid_api_key", "authentication", "not logged in"} {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
