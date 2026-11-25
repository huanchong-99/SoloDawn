package host

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	Register(Drivers, "claude", Driver(claudeDriver{}))
	Register(DriverCaps, "claude", claudeCapabilities(runtime.GOOS))
}

func claudeCapabilities(goos string) Capabilities {
	levels := []int{1, 2, 3}
	if goos == "windows" {
		levels = []int{1, 3}
	}
	return Capabilities{
		Accounts:  []string{AccountPersonal, AccountTeam, AccountAPIKey},
		Levels:    levels,
		Approvals: ApprovalTool,
		Takeover:  TakeoverNew,
		Usage:     []string{},
	}
}

var errClaudeAnswer = errors.New("claude: permission requests use the MCP approve tool")

const (
	claudeSandboxSettings = `{"sandbox":{"enabled":true,"autoAllowBashIfSandboxed":true,"failIfUnavailable":true}}`
	claudeBypassSettings  = `{"skipDangerousModePermissionPrompt":true}`
)

type claudeMCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type claudeMCPConfig struct {
	MCPServers map[string]claudeMCPServer `json:"mcpServers"`
}

type claudeDriver struct{}

func (claudeDriver) Models(ctx context.Context) []proto.Model {
	return probeModels(ctx, "claude", []string{"-p", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json", "--no-session-persistence", "--strict-mcp-config", "--settings", `{"disableAllHooks":true}`}, readClaudeModels)
}

func (claudeDriver) Start(ctx context.Context, l Launch) (Process, error) {
	argv, _, err := claudeCommandArgs(l, false, runtime.GOOS == "windows")
	if err != nil {
		return nil, err
	}
	return startClaudeProcess(ctx, l, argv[1:])
}

func (claudeDriver) Interactive(l Launch) ([]string, string, error) {
	return claudeCommandArgs(l, true, runtime.GOOS == "windows")
}

func claudeCommandArgs(l Launch, interactive, windows bool) ([]string, string, error) {
	levelFlags, err := claudeLevelFlags(l.Level, interactive, windows)
	if err != nil {
		return nil, "", err
	}
	mcpJSON, err := writeClaudeMCPConfig(l.Shim)
	if err != nil {
		return nil, "", err
	}
	session, sessionFlags := claudeSessionFlags(l.Session)
	argv := claudeBaseArgs(interactive, l.RepoConfig, l.Instructions, mcpJSON, sessionFlags)
	argv = append(argv, claudeModelFlags(l)...)
	return append(argv, levelFlags...), session, nil
}

func writeClaudeMCPConfig(shim []string) (string, error) {
	if len(shim) == 0 {
		return "", &Error{"bad_request", "missing MCP shim command"}
	}
	cfg := claudeMCPConfig{MCPServers: map[string]claudeMCPServer{"solodawn": {Command: shim[0], Args: append([]string{}, shim[1:]...)}}}
	return marshalWire(cfg), nil
}

func newClaudeUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] | 0x40) & 0x4f
	b[8] = (b[8] | 0x80) & 0xbf
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func claudeSessionFlags(session string) (string, []string) {
	if session == "" {
		id := newClaudeUUID()
		return id, []string{"--session-id", id}
	}
	return session, []string{"--resume", session}
}

func claudeBaseArgs(interactive, repoConfig bool, instructions, mcpJSON string, sessionFlags []string) []string {
	argv := []string{"claude"}
	if !interactive {
		argv = append(argv, "-p", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json")
	}
	argv = append(argv, sessionFlags...)
	argv = append(argv, "--mcp-config", mcpJSON, "--strict-mcp-config")
	if !interactive {
		argv = append(argv, "--permission-prompt-tool", "mcp__solodawn__approve")
	}
	sources := "user"
	if repoConfig {
		sources = "user,project"
	}
	argv = append(argv, "--setting-sources", sources, "--allowedTools", "mcp__solodawn")
	if instructions != "" {
		argv = append(argv, "--append-system-prompt", instructions)
	}
	return argv
}

func claudeLevelFlags(level int, interactive, windows bool) ([]string, error) {
	switch level {
	case 1:
		return []string{"--permission-mode", "default", "--tools", "Read,Glob,Grep,AskUserQuestion"}, nil
	case 2:
		if windows {
			return nil, &Error{"conflict", "Claude Code has no sandbox for level 2 on Windows"}
		}
		return []string{"--permission-mode", "acceptEdits", "--settings", claudeSandboxSettings}, nil
	case 3:
		return claudeBypassFlags(interactive), nil
	default:
		return nil, &Error{"bad_request", fmt.Sprintf("invalid permission level %d", level)}
	}
}

func claudeBypassFlags(interactive bool) []string {
	flags := []string{"--permission-mode", "bypassPermissions"}
	if interactive {
		return append(flags, "--settings", claudeBypassSettings)
	}
	return flags
}

func startClaudeProcess(ctx context.Context, l Launch, args []string) (Process, error) {
	ctx, kill := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, "claude", args...)
	detachTerminal(cmd)
	cmd.WaitDelay = time.Second
	cmd.Dir = l.Folder
	stdinRead, stdinWrite, _ := os.Pipe()
	cmd.Stdin = stdinRead
	stdout, _ := cmd.StdoutPipe()
	var stderr checkTail
	cmd.Stderr = &stderr
	err := cmd.Start()
	stdinRead.Close()
	if err != nil {
		kill()
		stdinWrite.Close()
		stdout.Close()
		return nil, err
	}
	killTree := killCheckTree(cmd)
	p := &claudeProcess{
		in:      stdinWrite,
		closeIn: sync.OnceValue(stdinWrite.Close),
		// Single reader: the caller of Process.Events (the session runtime).
		events:  make(chan Event),
		resumed: l.Session != "",
		kill:    kill,
	}
	go p.readLoop(cmd, stdout, &stderr, killTree)
	return p, nil
}

// claudeProcess owns one structured Claude Code subprocess.
// Lock order: mu guards stdin writes, closeIn, reqSeq and interrupted; nothing
// inside mu waits on events or the child process.
type claudeProcess struct {
	mu          sync.Mutex
	in          *os.File
	closeIn     func() error
	reqSeq      int64
	interrupted bool
	resumed     bool
	kill        func() // ends the context of the process, which kills its tree
	noTools     bool   // used by readLoop only
	// Single reader: the caller of Process.Events (the session runtime).
	events chan Event
}

type claudeTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeUserBody struct {
	Role    string            `json:"role"`
	Content []claudeTextBlock `json:"content"`
}

type claudeUserMsg struct {
	Type    string         `json:"type"`
	Message claudeUserBody `json:"message"`
}

type claudeControlSubtype struct {
	Subtype string `json:"subtype"`
}

type claudeControlReq struct {
	Type      string               `json:"type"`
	RequestID string               `json:"request_id"`
	Request   claudeControlSubtype `json:"request"`
}

func (p *claudeProcess) Events() <-chan Event { return p.events }

func (p *claudeProcess) Answer(string, bool) error { return errClaudeAnswer }

func (p *claudeProcess) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeIn()
}

func (p *claudeProcess) Turn(text string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interrupted = false
	return p.writeJSON(claudeUserMsg{
		Type:    "user",
		Message: claudeUserBody{Role: "user", Content: []claudeTextBlock{{Type: "text", Text: text}}},
	})
}

func (p *claudeProcess) Interrupt() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reqSeq++
	p.interrupted = true
	return p.writeJSON(claudeControlReq{
		Type:      "control_request",
		RequestID: "req-" + strconv.FormatInt(p.reqSeq, 10),
		Request:   claudeControlSubtype{Subtype: "interrupt"},
	})
}

func (p *claudeProcess) writeJSON(msg any) error {
	_, err := p.in.WriteString(marshalWire(msg) + "\n")
	return err
}

func (p *claudeProcess) takeInterrupted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	was := p.interrupted
	p.interrupted = false
	return was
}

func (p *claudeProcess) readLoop(cmd *exec.Cmd, stdout io.Reader, stderr *checkTail, killTree func()) {
	reportedDiag := p.readLines(bufio.NewReader(stdout))
	waitErr := cmd.Wait()
	killTree()
	p.kill()
	p.Close()
	if waitErr != nil && !reportedDiag {
		p.emitExitDiag(strings.TrimSpace(string(*stderr)))
	}
	close(p.events)
}

// readLines handles stdout to its end, or to the line that shows that the
// process has no tools.
func (p *claudeProcess) readLines(reader *bufio.Reader) (reportedDiag bool) {
	for !p.noTools {
		line, err := reader.ReadBytes('\n')
		if p.handleLine(bytes.TrimSpace(line)) {
			reportedDiag = true
		}
		if err != nil {
			break
		}
	}
	return reportedDiag
}

type claudeHeader struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	SessionID  string `json:"session_id"`
	MCPServers []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"mcp_servers"`
}

// toolsStatus is the status of the solodawn server that system/init lists;
// another than connected means a session without the node's tools.
func (hdr claudeHeader) toolsStatus() string {
	for _, srv := range hdr.MCPServers {
		if srv.Name == "solodawn" {
			return srv.Status
		}
	}
	return "connected"
}

func (p *claudeProcess) handleLine(line []byte) bool {
	var hdr claudeHeader
	if json.Unmarshal(line, &hdr) != nil || hdr.Type == "" {
		return false
	}
	switch hdr.Type {
	case "rate_limit_event":
		p.handleRateLimit(line)
		return false
	case "result":
		return p.handleResult(line)
	default:
		return p.handleOther(hdr)
	}
}

// handleOther kills a process that works without the node's tools
// (contracts/drivers.md 1.1) and reports any other line.
func (p *claudeProcess) handleOther(hdr claudeHeader) bool {
	if status := hdr.toolsStatus(); status != "connected" {
		p.noTools = true
		p.kill()
		p.events <- Event{Kind: Active, Text: "claude: MCP server solodawn: " + status}
		return true
	}
	p.events <- claudeOtherEvent(hdr)
	return false
}

func claudeOtherEvent(hdr claudeHeader) Event {
	if hdr.Type == "system" && hdr.Subtype == "init" && hdr.SessionID != "" {
		return Event{Kind: Ready, Session: hdr.SessionID}
	}
	return Event{Kind: Active}
}

type claudeRateLimitInfo struct {
	Status         string                      `json:"status"`
	RateLimitType  string                      `json:"rateLimitType"`
	Utilization    *float64                    `json:"utilization"`
	ResetsAt       *int64                      `json:"resetsAt"`
	UnifiedWindows map[string]claudeRateWindow `json:"unifiedWindows"`
}

type claudeRateWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *int64   `json:"resetsAt"`
}

func (p *claudeProcess) handleRateLimit(line []byte) {
	for _, ev := range claudeRateEvents(line) {
		p.events <- ev
	}
}

// claudeRateEvents reads a rate_limit_event: Used with the window, and
// RateLimited when the limit is hit; Active when it says neither.
func claudeRateEvents(line []byte) []Event {
	var msg struct {
		Info claudeRateLimitInfo `json:"rate_limit_info"`
	}
	json.Unmarshal(line, &msg)
	usage, resets := claudeRateUsage(msg.Info)
	var out []Event
	if usage != (Usage{}) {
		out = append(out, Event{Kind: Used, Usage: usage})
	}
	if isClaudeRateLimited(msg.Info.Status) {
		out = append(out, Event{Kind: RateLimited, Resets: resets})
	}
	if len(out) == 0 {
		return []Event{{Kind: Active}}
	}
	return out
}

func claudeRateUsage(info claudeRateLimitInfo) (Usage, int64) {
	win := info.UnifiedWindows[info.RateLimitType]
	pct := cmp.Or(info.Utilization, win.Utilization)
	resets := cmp.Or(info.ResetsAt, win.ResetsAt)
	var u Usage
	var resetsMS int64
	if pct != nil {
		v := *pct * 100
		u.WindowPct = &v
	}
	if resets != nil {
		resetsMS = *resets * 1000
		u.WindowResets = &resetsMS
	}
	return u, resetsMS
}

func isClaudeRateLimited(status string) bool {
	return status != "" && status != "allowed" && status != "allowed_warning"
}

type claudeResultMsg struct {
	Subtype        string   `json:"subtype"`
	IsError        bool     `json:"is_error"`
	TerminalReason string   `json:"terminal_reason"`
	Result         string   `json:"result"`
	Errors         []string `json:"errors"`
	APIErrorStatus int      `json:"api_error_status"`
}

func (p *claudeProcess) handleResult(line []byte) bool {
	var msg claudeResultMsg
	if json.Unmarshal(line, &msg) != nil {
		p.events <- Event{Kind: Active}
		return false
	}
	ev, diag := claudeResultEvent(msg, p.takeInterrupted(), p.resumed)
	p.events <- ev
	return diag
}

func claudeResultEvent(msg claudeResultMsg, interrupted, resumed bool) (Event, bool) {
	failed := msg.IsError || msg.Subtype != "success"
	if failed && !claudeTurnAborted(msg, interrupted) {
		if diag, ok := claudeDiagEvent(msg.APIErrorStatus, claudeErrorText(msg), resumed); ok {
			return diag, true
		}
	}
	return Event{
		Kind:   TurnEnded,
		Status: claudeTurnStatus(msg, interrupted, failed),
	}, false
}

func claudeTurnAborted(msg claudeResultMsg, interrupted bool) bool {
	return interrupted || strings.HasPrefix(msg.TerminalReason, "aborted_")
}

func claudeTurnStatus(msg claudeResultMsg, interrupted, failed bool) string {
	if !failed {
		return "completed"
	}
	if claudeTurnAborted(msg, interrupted) {
		return "interrupted"
	}
	return "failed"
}

func claudeErrorText(msg claudeResultMsg) string {
	return strings.TrimSpace(cmp.Or(msg.Result, strings.Join(msg.Errors, "\n")))
}

func claudeDiagEvent(status int, text string, resumed bool) (Event, bool) {
	if resumed && containsClaudeFold(text, "no conversation found", "session not found") {
		return Event{Kind: NoSession, Text: text}, true
	}
	if status == 401 || containsClaudeFold(text, "/login", "not logged in", "invalid api key", "authentication") {
		return Event{Kind: LoginInvalid, Text: text}, true
	}
	return Event{}, false
}

func (p *claudeProcess) emitExitDiag(stderr string) {
	if ev, ok := claudeDiagEvent(0, stderr, p.resumed); ok {
		p.events <- ev
	}
}

func containsClaudeFold(text string, subs ...string) bool {
	lower := strings.ToLower(text)
	for _, sub := range subs {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}
