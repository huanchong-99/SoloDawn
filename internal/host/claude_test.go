package host

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var claudeUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func claudeNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func claudeErrText(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func claudeEqual[T any](t *testing.T, label string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s:\ngot  %+v\nwant %+v", label, got, want)
	}
}

func claudeTrue(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}

func TestClaudeMCPShim(t *testing.T) {
	addr := os.Getenv("SOLODAWN_CLAUDE_TEST_MCP")
	if addr == "" {
		return
	}
	idx := slices.Index(os.Args, "--")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprintln(conn, strings.Join(os.Args[idx+1:], " "))
	go io.Copy(conn, os.Stdin)
	io.Copy(os.Stdout, conn)
	os.Exit(0)
}

func TestClaudeDriverRegistered(t *testing.T) {
	claudeTrue(t, Drivers["claude"] != nil, "claude driver is not registered")
}

func TestNewClaudeUUIDFormatAndBits(t *testing.T) {
	seen := map[string]bool{}
	lowNibbles := map[byte]bool{}
	variants := map[byte]bool{}
	for range 128 {
		id := newClaudeUUID()
		claudeTrue(t, claudeUUIDPattern.MatchString(id), "invalid UUID v4: "+id)
		seen[id] = true
		lowNibbles[id[15]] = true
		variants[id[19]] = true
	}
	claudeEqual(t, "unique UUIDs", len(seen), 128)
	claudeTrue(t, len(lowNibbles) >= 4, "insufficient version-byte low nibble entropy")
	claudeEqual(t, "variant nibble count", len(variants), 4)
}

func TestClaudeCommandArgsStructured(t *testing.T) {
	groupLaunch := Launch{
		Folder:       t.TempDir(),
		Level:        1,
		Shim:         []string{"/bin/solodawn", "mcp", "--token", "tok-1"},
		Instructions: "Speak through <post> & reply.",
	}
	argv, session, err := claudeCommandArgs(groupLaunch, false, false)
	claudeNoErr(t, err)
	claudeTrue(t, claudeUUIDPattern.MatchString(session), "generated session is not UUID v4")
	mcpJSON := claudeArgAfter(t, argv, "--mcp-config")
	checkClaudeMCPJSON(t, mcpJSON, "/bin/solodawn", []string{"mcp", "--token", "tok-1"})
	want := []string{
		"claude", "-p", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json",
		"--session-id", session, "--mcp-config", mcpJSON, "--strict-mcp-config",
		"--permission-prompt-tool", "mcp__solodawn__approve",
		"--setting-sources", "user", "--allowedTools", "mcp__solodawn",
		"--append-system-prompt", "Speak through <post> & reply.",
		"--permission-mode", "default", "--tools", "Read,Glob,Grep,AskUserQuestion",
	}
	claudeEqual(t, "structured group argv", argv, want)

	taskLaunch := groupLaunch
	taskLaunch.RepoConfig = true
	taskArgv, _, err := claudeCommandArgs(taskLaunch, false, false)
	claudeNoErr(t, err)
	claudeEqual(t, "structured task setting-sources", claudeArgAfter(t, taskArgv, "--setting-sources"), "user,project")
}

func TestClaudeCommandArgsInteractiveResume(t *testing.T) {
	const session = "797fa93e-af8a-4768-b847-42582d5583a0"
	resumed := Launch{
		Folder:     t.TempDir(),
		Session:    session,
		Level:      3,
		Shim:       []string{"solodawn"},
		RepoConfig: true,
	}
	argv, gotSession, err := claudeCommandArgs(resumed, true, false)
	claudeNoErr(t, err)
	claudeEqual(t, "interactive session", gotSession, session)
	mcpJSON := claudeArgAfter(t, argv, "--mcp-config")
	checkClaudeMCPJSON(t, mcpJSON, "solodawn", []string{})
	want := []string{
		"claude", "--resume", session, "--mcp-config", mcpJSON, "--strict-mcp-config",
		"--setting-sources", "user,project", "--allowedTools", "mcp__solodawn",
		"--permission-mode", "bypassPermissions", "--settings", claudeBypassSettings,
	}
	claudeEqual(t, "interactive argv", argv, want)
}

func TestClaudeInteractiveFirstStartWithInstructions(t *testing.T) {
	argv, session, err := Drivers["claude"].Interactive(Launch{
		Folder:       t.TempDir(),
		Level:        1,
		Shim:         []string{"solodawn", "mcp", "--token", "tok-int"},
		Instructions: "Group instructions.",
	})
	claudeNoErr(t, err)
	claudeTrue(t, claudeUUIDPattern.MatchString(session), "session is not UUID v4")
	mcpJSON := claudeArgAfter(t, argv, "--mcp-config")
	checkClaudeMCPJSON(t, mcpJSON, "solodawn", []string{"mcp", "--token", "tok-int"})
	want := []string{
		"claude", "--session-id", session, "--mcp-config", mcpJSON, "--strict-mcp-config",
		"--setting-sources", "user", "--allowedTools", "mcp__solodawn",
		"--append-system-prompt", "Group instructions.",
		"--permission-mode", "default", "--tools", "Read,Glob,Grep,AskUserQuestion",
	}
	claudeEqual(t, "interactive first-start argv", argv, want)
}

func TestClaudeLevelFlagsAllModesAndRefusals(t *testing.T) {
	cases := []struct {
		level       int
		interactive bool
		windows     bool
		want        []string
		errText     string
	}{
		{1, false, true, []string{"--permission-mode", "default", "--tools", "Read,Glob,Grep,AskUserQuestion"}, ""},
		{2, false, false, []string{"--permission-mode", "acceptEdits", "--settings", claudeSandboxSettings}, ""},
		{2, true, false, []string{"--permission-mode", "acceptEdits", "--settings", claudeSandboxSettings}, ""},
		{2, false, true, nil, "conflict: Claude Code has no sandbox for level 2 on Windows"},
		{3, false, true, []string{"--permission-mode", "bypassPermissions"}, ""},
		{3, true, true, []string{"--permission-mode", "bypassPermissions", "--settings", claudeBypassSettings}, ""},
		{0, false, false, nil, "bad_request: invalid permission level 0"},
		{4, true, false, nil, "bad_request: invalid permission level 4"},
	}
	for _, tc := range cases {
		got, err := claudeLevelFlags(tc.level, tc.interactive, tc.windows)
		if tc.errText != "" {
			claudeErrText(t, err, tc.errText)
			continue
		}
		claudeNoErr(t, err)
		claudeEqual(t, "level flags", got, tc.want)
	}
}

func TestClaudeStartAndInteractiveRefusals(t *testing.T) {
	d := Drivers["claude"]
	base := Launch{Folder: t.TempDir(), Level: 1, Shim: []string{"solodawn", "mcp"}}

	badLevel := base
	badLevel.Level = 9
	_, _, err := d.Interactive(badLevel)
	claudeErrText(t, err, "bad_request: invalid permission level 9")
	_, err = d.Start(t.Context(), badLevel)
	claudeErrText(t, err, "bad_request: invalid permission level 9")

	noShim := base
	noShim.Shim = nil
	_, _, err = d.Interactive(noShim)
	claudeErrText(t, err, "bad_request: missing MCP shim command")

	missingFolder := base
	missingFolder.Folder = filepath.Join(t.TempDir(), "does-not-exist")
	_, err = d.Start(t.Context(), missingFolder)
	claudeTrue(t, err != nil, "Start succeeded in a missing folder")
}

func TestClaudeStreamLineAndRateEvents(t *testing.T) {
	// Single reader: this test draining events below.
	events := make(chan Event, 16)
	p := &claudeProcess{events: events}
	for _, ignored := range []string{"", "not-json", `{}`, `{"type":"system","subtype":123}`} {
		claudeTrue(t, !p.handleLine([]byte(ignored)), "unexpected diag on "+ignored)
		claudeEqual(t, "events len", len(events), 0)
	}
	cases := []struct {
		line string
		want []Event
	}{
		{`{"type":"system","subtype":"init","session_id":"s1"}`, []Event{{Kind: Ready, Session: "s1"}}},
		{`{"type":"system","subtype":"init","session_id":"s1","mcp_servers":[{"name":"other","status":"failed"},{"name":"solodawn","status":"connected"}]}`, []Event{{Kind: Ready, Session: "s1"}}},
		{`{"type":"assistant","subtype":"init","session_id":"s1"}`, []Event{{Kind: Active}}},
		{`{"type":"system","subtype":"thinking_tokens","session_id":"s1"}`, []Event{{Kind: Active}}},
		{`{"type":"system","subtype":"init","session_id":""}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":"bad"}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event"}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning"}}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":""}}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected"}}`, []Event{{Kind: RateLimited, Resets: 0}}},
		{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","utilization":0.75,"resetsAt":1700000}}`,
			[]Event{
				{Kind: Used, Usage: Usage{WindowPct: new(75.0), WindowResets: new(int64(1700000000))}},
				{Kind: RateLimited, Resets: 1700000000},
			},
		},
		{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","rateLimitType":"seven_day","unifiedWindows":{"seven_day":{"utilization":0.25,"resetsAt":1800000}}}}`,
			[]Event{{Kind: Used, Usage: Usage{WindowPct: new(25.0), WindowResets: new(int64(1800000000))}}},
		},
		{`{"type":"result","is_error":"bad"}`, []Event{{Kind: Active}}},
	}
	for _, tc := range cases {
		claudeTrue(t, !p.handleLine([]byte(tc.line)), "unexpected diagnostic return")
		for _, want := range tc.want {
			claudeEqual(t, tc.line, <-events, want)
		}
		claudeEqual(t, "remaining events", len(events), 0)
	}
}

// The line after a failed init, such as a sign-in error, is not read: the
// process is a crash that names the server.
func TestClaudeReadsNothingAfterAFailedInit(t *testing.T) {
	events := make(chan Event, 16)
	killed := false
	p := &claudeProcess{events: events, kill: func() { killed = true }}
	stdout := `{"type":"system","subtype":"init","session_id":"s1","mcp_servers":[{"name":"solodawn","status":"failed"}]}` + "\n" +
		`{"type":"result","subtype":"success","is_error":true,"result":"Not logged in"}` + "\n"
	claudeTrue(t, p.readLines(bufio.NewReader(strings.NewReader(stdout))), "no diagnosis")
	claudeTrue(t, killed, "the process was not killed")
	claudeEqual(t, "event", <-events, Event{Kind: Active, Text: "claude: MCP server solodawn: failed"})
	claudeEqual(t, "remaining events", len(events), 0)
}

func TestClaudeResultAndDiagnosticMapping(t *testing.T) {
	checkClaudeStatuses(t)
	checkClaudeDiagnostics(t)
}

func checkClaudeStatuses(t *testing.T) {
	t.Helper()
	cases := []struct {
		msg         claudeResultMsg
		interrupted bool
		want        Event
	}{
		{claudeResultMsg{Subtype: "success"}, false, Event{Kind: TurnEnded, Status: "completed"}},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "  boom  "}, false, Event{Kind: TurnEnded, Status: "failed", Text: "boom"}},
		{claudeResultMsg{Subtype: "error_max_turns"}, false, Event{Kind: TurnEnded, Status: "failed"}},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true}, true, Event{Kind: TurnEnded, Status: "interrupted"}},
		{claudeResultMsg{Subtype: "error_during_execution", TerminalReason: "aborted_tools"}, false, Event{Kind: TurnEnded, Status: "interrupted"}},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "Please run /login"}, true, Event{Kind: TurnEnded, Status: "interrupted"}},
	}
	for _, tc := range cases {
		ev, diag := claudeResultEvent(tc.msg, tc.interrupted, true)
		claudeTrue(t, !diag, "unexpected diagnostic result")
		claudeEqual(t, "turn status", ev, tc.want)
	}
}

func checkClaudeDiagnostics(t *testing.T) {
	t.Helper()
	cases := []struct {
		msg     claudeResultMsg
		resumed bool
		want    Event
		isDiag  bool
	}{
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Errors: []string{"No conversation found with session ID: x"}}, true, Event{Kind: NoSession, Text: "No conversation found with session ID: x"}, true},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "Session not found"}, true, Event{Kind: NoSession, Text: "Session not found"}, true},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "No conversation found"}, false, Event{Kind: TurnEnded, Status: "failed", Text: "No conversation found"}, false},
		{claudeResultMsg{Subtype: "success", IsError: true, APIErrorStatus: 400, Result: "HTTP 400"}, false, Event{Kind: TurnEnded, Status: "failed", Text: "HTTP 400"}, false},
		{claudeResultMsg{Subtype: "success", IsError: true, APIErrorStatus: 401, Result: "HTTP 401"}, false, Event{Kind: LoginInvalid, Text: "HTTP 401"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, APIErrorStatus: 402, Result: "HTTP 402"}, false, Event{Kind: TurnEnded, Status: "failed", Text: "HTTP 402"}, false},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "  \n", Errors: []string{"first line\nsecond"}}, false, Event{Kind: TurnEnded, Status: "failed", Text: "first line\nsecond"}, false},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Please run /login"}, false, Event{Kind: LoginInvalid, Text: "Please run /login"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Not logged in"}, false, Event{Kind: LoginInvalid, Text: "Not logged in"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Invalid API key"}, false, Event{Kind: LoginInvalid, Text: "Invalid API key"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "authentication_error"}, false, Event{Kind: LoginInvalid, Text: "authentication_error"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "authentication_failed"}, false, Event{Kind: LoginInvalid, Text: "authentication_failed"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Authentication failed"}, false, Event{Kind: LoginInvalid, Text: "Authentication failed"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: `Failed to authenticate. API Error: 401 {"error":{"message":"OAuth token has expired"}}`}, false, Event{Kind: LoginInvalid, Text: `Failed to authenticate. API Error: 401 {"error":{"message":"OAuth token has expired"}}`}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "OAuth session expired"}, false, Event{Kind: LoginInvalid, Text: "OAuth session expired"}, true},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, APIErrorStatus: 401, Result: "Unauthorized request"}, false, Event{Kind: LoginInvalid, Text: "Unauthorized request"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Unauthorized request"}, false, Event{Kind: TurnEnded, Status: "failed", Text: "Unauthorized request"}, false},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, APIErrorStatus: 500, Result: "HTTP 401 Unauthorized from upstream proxy"}, false, Event{Kind: TurnEnded, Status: "failed", Text: "HTTP 401 Unauthorized from upstream proxy"}, false},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "third-party oauth callback failed"}, false, Event{Kind: TurnEnded, Status: "failed", Text: "third-party oauth callback failed"}, false},
		{claudeResultMsg{Subtype: "success", APIErrorStatus: 401, Result: "Not logged in"}, false, Event{Kind: TurnEnded, Status: "completed"}, false},
	}
	for _, tc := range cases {
		ev, diag := claudeResultEvent(tc.msg, false, tc.resumed)
		claudeEqual(t, "isDiag", diag, tc.isDiag)
		claudeEqual(t, "diag event", ev, tc.want)
	}
}

func TestClaudeDriverWithFakeCLI(t *testing.T) {
	buildFakeClaudeBinary(t)
	t.Run("FirstStartTwoTurnsAndResume", testClaudeApprovalAndResume)
	t.Run("InterruptAndSecondTurn", testClaudeInterruptAndContinue)
	t.Run("MidTurnExitAndStderrDiagnostics", testClaudeCrashAndStderrDiag)
	t.Run("ContextCancelKillsProcessTree", testClaudeContextCancelKillsTree)
}

func claudeTestStartLevel() int {
	if runtime.GOOS == "windows" {
		return 1
	}
	return 2
}

func testClaudeApprovalAndResume(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "lifecycle.log")
	t.Setenv("SOLODAWN_FAKECLI_LOG", logFile)
	script1, err := filepath.Abs(filepath.Join("fakecli", "testdata", "claude-approval.json"))
	claudeNoErr(t, err)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", script1)
	peer, shim := startTestMCPPeer(t, "tok-first")

	_, _, err = Drivers["claude"].Interactive(Launch{
		Folder: t.TempDir(),
		Level:  claudeTestStartLevel(),
		Shim:   shim,
	})
	claudeNoErr(t, err)

	proc, err := Drivers["claude"].Start(t.Context(), Launch{
		Folder:       t.TempDir(),
		Level:        claudeTestStartLevel(),
		Shim:         shim,
		Instructions: "Follow rules.",
	})
	claudeNoErr(t, err)
	t.Cleanup(func() { proc.Close() })
	claudeTrue(t, errors.Is(proc.Answer("ask-1", true), errClaudeAnswer), "Answer did not return errClaudeAnswer")
	claudeNoErr(t, proc.Turn("try the write"))
	mcpDone := serveClaudeApprovalMCP(t, peer, "tok-first")
	events := drainUntilTurnEnded(t, proc.Events())
	session := events[0].Session
	claudeEqual(t, "first kind", events[0].Kind, Ready)
	claudeTrue(t, claudeUUIDPattern.MatchString(session), "session is not UUID v4")
	checkFirstApprovalTurnEvents(t, events)

	claudeNoErr(t, proc.Turn("Approved. Run exactly the same request again."))
	events2 := drainUntilTurnEnded(t, proc.Events())
	claudeNoErr(t, <-mcpDone)
	claudeEqual(t, "second turn end", events2[len(events2)-1], Event{Kind: TurnEnded, Status: "completed"})

	claudeNoErr(t, proc.Close())
	_, open := <-proc.Events()
	claudeTrue(t, !open, "expected Events closed after Close")
	runClaudeResumeCheck(t, session, logFile)
}

func checkFirstApprovalTurnEvents(t *testing.T, events []Event) {
	t.Helper()
	claudeTrue(t, slices.Contains(events, Event{Kind: Active}), "missing Active event")
	wantUsed := Event{Kind: Used, Usage: Usage{WindowPct: new(44.0), WindowResets: new(int64(1795615200000))}}
	claudeTrue(t, slices.ContainsFunc(events, func(e Event) bool { return reflect.DeepEqual(e, wantUsed) }), "missing rate_limit_event Used")
	claudeEqual(t, "first turn end", events[len(events)-1], Event{Kind: TurnEnded, Status: "completed"})
}

func runClaudeResumeCheck(t *testing.T, session, logFile string) {
	t.Helper()
	script2, err := filepath.Abs(filepath.Join("fakecli", "testdata", "claude-resume.json"))
	claudeNoErr(t, err)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", script2)
	peer, shim := startTestMCPPeer(t, "tok-resumed")
	proc, err := Drivers["claude"].Start(t.Context(), Launch{
		Folder:  t.TempDir(),
		Session: session,
		Level:   1,
		Shim:    shim,
	})
	claudeNoErr(t, err)
	t.Cleanup(func() { proc.Close() })
	claudeNoErr(t, proc.Turn("check token"))
	mcpDone := serveClaudeResumeMCP(t, peer, "tok-resumed")
	events := drainUntilTurnEnded(t, proc.Events())
	claudeNoErr(t, <-mcpDone)
	claudeEqual(t, "resumed Ready", events[0], Event{Kind: Ready, Session: session})
	claudeEqual(t, "resumed end", events[len(events)-1], Event{Kind: TurnEnded, Status: "completed"})
	claudeNoErr(t, proc.Close())
	for range proc.Events() {
	}
	gotLog, err := os.ReadFile(logFile)
	claudeNoErr(t, err)
	wantLog := fmt.Sprintf("start claude %s\nexit claude %s\nstart claude %s\nexit claude %s\n", session, session, session, session)
	claudeEqual(t, "lifecycle log", string(gotLog), wantLog)
}

func testClaudeInterruptAndContinue(t *testing.T) {
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	script := writeClaudeScript(t, `[
		{"dir":"stdin","msg":{"type":"control_request","request_id":"req-1","request":{"subtype":"interrupt"}}},
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"turn <1> & go"}]}}},
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},
		{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true}},
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"turn 2"}]}}},
		{"dir":"stdin","msg":{"type":"control_request","request_id":"req-2","request":{"subtype":"interrupt"}}},
		{"dir":"stdout","msg":{"type":"control_response","response":{"subtype":"success","request_id":"req-2"}}},
		{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true}},
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"turn 3"}]}}},
		{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true}},
		{"dir":"eof"}
	]`, 0)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", script)

	proc, err := Drivers["claude"].Start(t.Context(), Launch{
		Folder: t.TempDir(),
		Level:  1,
		Shim:   []string{"solodawn", "mcp"},
	})
	claudeNoErr(t, err)
	t.Cleanup(func() { proc.Close() })
	claudeNoErr(t, proc.Interrupt())
	claudeNoErr(t, proc.Turn("turn <1> & go"))
	claudeEqual(t, "init kind", (<-proc.Events()).Kind, Ready)
	ev1 := drainUntilTurnEnded(t, proc.Events())
	claudeEqual(t, "turn 1 status", ev1[len(ev1)-1].Status, "failed")

	claudeNoErr(t, proc.Turn("turn 2"))
	claudeNoErr(t, proc.Interrupt())
	ev2 := drainUntilTurnEnded(t, proc.Events())
	claudeEqual(t, "turn 2 status", ev2[len(ev2)-1].Status, "interrupted")

	claudeNoErr(t, proc.Turn("turn 3"))
	ev3 := drainUntilTurnEnded(t, proc.Events())
	claudeEqual(t, "turn 3 status", ev3[len(ev3)-1].Status, "failed")
	claudeNoErr(t, proc.Close())
	for range proc.Events() {
	}
}

func testClaudeCrashAndStderrDiag(t *testing.T) {
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	cases := []struct {
		session string
		steps   string
		exit    int
		want    []Event
	}{
		{
			"",
			`[{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"s-crash"}},{"dir":"stdout","msg":{"type":"assistant"}}]`,
			1,
			[]Event{{Kind: Ready, Session: "s-crash"}, {Kind: Active}},
		},
		{
			"",
			`[{"dir":"stderr","msg":"No conversation found with session ID: s-fresh"}]`,
			1,
			nil,
		},
		{
			"missing-id",
			`[{"dir":"stderr","msg":"No conversation found with session ID: missing-id"}]`,
			1,
			[]Event{{Kind: NoSession, Text: "No conversation found with session ID: missing-id"}},
		},
		{
			"missing-id",
			`[{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["No conversation found with session ID: missing-id"]}},{"dir":"stderr","msg":"No conversation found with session ID: missing-id"}]`,
			1,
			[]Event{{Kind: NoSession, Text: "No conversation found with session ID: missing-id"}},
		},
		{
			"",
			`[{"dir":"stdout","msg":{"type":"result","subtype":"success"}},{"dir":"stderr","msg":"Not logged in"}]`,
			0,
			[]Event{{Kind: TurnEnded, Status: "completed"}},
		},
		{
			"",
			`[{"dir":"stderr","msg":"Not logged in"}]`,
			1,
			[]Event{{Kind: LoginInvalid, Text: "Not logged in"}},
		},
	}
	for _, tc := range cases {
		runClaudeDiagCase(t, tc.session, tc.steps, tc.exit, tc.want)
	}
}

func runClaudeDiagCase(t *testing.T, session, steps string, exit int, want []Event) {
	t.Helper()
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", writeClaudeScript(t, steps, exit))
	proc, err := Drivers["claude"].Start(t.Context(), Launch{
		Folder:  t.TempDir(),
		Session: session,
		Level:   1,
		Shim:    []string{"solodawn", "mcp"},
	})
	claudeNoErr(t, err)
	var got []Event
	for ev := range proc.Events() {
		got = append(got, ev)
	}
	claudeEqual(t, "diag case events", got, want)
	claudeTrue(t, errors.Is(proc.Turn("after exit"), os.ErrClosed), "expected os.ErrClosed after process exit")
}

// The captured CLI works on after its MCP server failed. The runtime reports
// the crash, and the first start leaves no CLI session id.
func TestClaudeWithoutItsToolsIsACrash(t *testing.T) {
	buildFakeClaudeBinary(t)
	log := filepath.Join(t.TempDir(), "log")
	t.Setenv("SOLODAWN_FAKECLI_LOG", log)
	script, err := filepath.Abs(filepath.Join("fakecli", "testdata", "claude-mcp-failed.json"))
	claudeNoErr(t, err)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", script)
	h, rh, _, _ := setupRunEnv(t)
	Listeners["agents"] = agtListen
	Drivers["claude"] = claudeDriver{}
	key := SessionKey{Group: runGroup1, Member: runAI1, Task: runTask1}
	claudeNoErr(t, h.Sessions.Create(makeRunSession(key, TaskSession, t.TempDir())))
	claudeNoErr(t, Runner.Turn(t.Context(), h, key, "go", Turn{Kind: ItemsTurn, Thread: runTask1}))
	rh.waitCmd(t)
	crash, fail := rh.waitCmd(t), rh.waitCmd(t)
	const detail = "claude: MCP server solodawn: failed"
	var status proto.MemberStatus[proto.TurnInterruptedEvent]
	claudeNoErr(t, proto.Decode([]byte(crash.raw), &status))
	claudeEqual(t, "reason", status.Event.Reason, "crash")
	claudeEqual(t, "detail", status.Event.Detail, detail)
	var failed proto.TaskFail
	claudeNoErr(t, proto.Decode([]byte(fail.raw), &failed))
	claudeEqual(t, "task fail", failed.Reason+" "+failed.Detail, "crash "+detail)
	claudeEqual(t, "CLI session", waitForState(t, h, key, Dormant).CLISession, "")
	written, err := os.ReadFile(log)
	claudeNoErr(t, err)
	claudeTrue(t, !strings.Contains(string(written), "exit claude"), "the CLI ended by itself")
}

// A Claude OAuth expiration failure moves the session to dormant, sets limited
// + login_invalid (with login_ok=false), and a non-auth failed result keeps
// the open CLI in Waiting while emitting turn_interrupted (crash) with no task fail.
func TestClaudeOAuthAndFailedTurnInRuntime(t *testing.T) {
	buildFakeClaudeBinary(t)
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	oauthMsg := `Failed to authenticate. API Error: 401 {"error":{"message":"OAuth token has expired"}}`
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", writeClaudeScript(t, fmt.Sprintf(`[
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"s-oauth"}},
		{"dir":"stdin","msg":{"type":"user"}},
		{"dir":"stdout","msg":{"type":"result","subtype":"success","is_error":true,"result":%q}}
	]`, oauthMsg), 1))
	h, rh, _, _ := setupRunEnv(t)
	Listeners["agents"] = agtListen
	Drivers["claude"] = claudeDriver{}
	var loginChanges atomic.Int32
	h.LoginChanged = func() { loginChanges.Add(1) }

	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	claudeNoErr(t, h.Sessions.Create(makeRunSession(grpKey, GroupSession, t.TempDir())))
	claudeNoErr(t, Runner.Turn(t.Context(), h, grpKey, "hello", Turn{Kind: ItemsTurn}))
	rh.waitCmd(t) // working

	var stLim proto.MemberStatus[proto.LoginInvalidEvent]
	claudeNoErr(t, proto.Decode([]byte(rh.waitCmd(t).raw), &stLim))
	claudeEqual(t, "limited state", stLim.State, "limited")
	claudeEqual(t, "login invalid code", stLim.Event.Code, "login_invalid")
	claudeEqual(t, "oauth detail", stLim.Event.Detail, oauthMsg)
	waitForState(t, h, grpKey, Dormant)
	claudeEqual(t, "login changes", loginChanges.Load(), int32(1))

	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", writeClaudeScript(t, `[
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"s-fail"}},
		{"dir":"stdin","msg":{"type":"user"}},
		{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true,"api_error_status":400,"result":"API error 400\nline two"}},
		{"dir":"stdin","msg":{"type":"user"}},
		{"dir":"stdout","msg":{"type":"result","subtype":"success","session_id":"s-fail","result":"recovered"}},
		{"dir":"eof"}
	]`, 0))
	taskKey := SessionKey{Group: runGroup2, Member: runAI2, Task: runTask1}
	claudeNoErr(t, h.Sessions.Create(makeRunSession(taskKey, TaskSession, t.TempDir())))
	claudeNoErr(t, Runner.Turn(t.Context(), h, taskKey, "work", Turn{Kind: ItemsTurn, Thread: runTask1}))
	rh.waitCmd(t) // working

	var stCrash proto.MemberStatus[proto.TurnInterruptedEvent]
	claudeNoErr(t, proto.Decode([]byte(rh.waitCmd(t).raw), &stCrash))
	claudeEqual(t, "waiting state", stCrash.State, "waiting")
	claudeEqual(t, "crash task", string(stCrash.Event.Task), runTask1)
	claudeEqual(t, "crash reason", stCrash.Event.Reason, "crash")
	claudeEqual(t, "crash detail", stCrash.Event.Detail, "API error 400")
	waitForState(t, h, taskKey, Waiting)
	for _, cmd := range rh.snapshotCmds() {
		claudeTrue(t, cmd.payload["op"] != "fail", "stay-open failed turn must not fail task: "+cmd.raw)
	}

	claudeNoErr(t, Runner.Turn(t.Context(), h, taskKey, "follow-up", Turn{Kind: ItemsTurn, Thread: runTask1}))
	rh.waitCmd(t) // working on same open CLI
	var stOK proto.MemberStatus[proto.TurnInterruptedEvent]
	claudeNoErr(t, proto.Decode([]byte(rh.waitCmd(t).raw), &stOK))
	claudeEqual(t, "waiting after recovery", stOK.State, "waiting")
	claudeTrue(t, stOK.Event == nil, "unexpected event after recovery")
	waitForState(t, h, taskKey, Waiting)
}

// Foreign 401 Unauthorized or third-party oauth errors (without APIErrorStatus 401)
// do not emit LoginInvalid or flip login_ok.
func TestClaudeForeignAuthDoesNotFlipLoginOK(t *testing.T) {
	buildFakeClaudeBinary(t)
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", writeClaudeScript(t, `[
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"s-foreign"}},
		{"dir":"stdin","msg":{"type":"user"}},
		{"dir":"stderr","msg":"proxy reported 401 Unauthorized on third-party oauth endpoint"},
		{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true,"api_error_status":500,"result":"401 Unauthorized from external oauth proxy\nretry"}},
		{"dir":"eof"}
	]`, 1))
	h, rh, _, _ := setupRunEnv(t)
	Listeners["agents"] = agtListen
	Drivers["claude"] = claudeDriver{}
	var loginChanges atomic.Int32
	h.LoginChanged = func() { loginChanges.Add(1) }

	grpKey := SessionKey{Group: runGroup1, Member: runAI1}
	claudeNoErr(t, h.Sessions.Create(makeRunSession(grpKey, GroupSession, t.TempDir())))
	claudeNoErr(t, Runner.Turn(t.Context(), h, grpKey, "hello", Turn{Kind: ItemsTurn}))
	rh.waitCmd(t) // working

	var stCrash proto.MemberStatus[proto.TurnInterruptedEvent]
	claudeNoErr(t, proto.Decode([]byte(rh.waitCmd(t).raw), &stCrash))
	claudeEqual(t, "crash code", stCrash.Event.Code, "turn_interrupted")
	claudeEqual(t, "crash detail", stCrash.Event.Detail, "401 Unauthorized from external oauth proxy")
	claudeEqual(t, "login changes", loginChanges.Load(), int32(0))
	st, _ := h.MemberState(runGroup1, runAI1)
	claudeTrue(t, st != "limited", "foreign 401 must not enter limited state")
}

func testClaudeContextCancelKillsTree(t *testing.T) {
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	script := writeClaudeScript(t, `[
		{"dir":"mcp<-cli","msg":{"jsonrpc":"2.0","id":1,"method":"ping"}},
		{"dir":"mcp->cli","msg":{"jsonrpc":"2.0","id":1,"result":{}}}
	]`, 0)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", script)
	peer, shim := startTestMCPPeer(t, "tok-tree")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proc, err := Drivers["claude"].Start(ctx, Launch{
		Folder: t.TempDir(),
		Level:  1,
		Shim:   shim,
	})
	claudeNoErr(t, err)
	peer.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := peer.Accept()
	claudeNoErr(t, err)
	t.Cleanup(func() { conn.Close() })
	cancel()

	for range proc.Events() {
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, readErr := io.ReadAll(conn)
	var netErr net.Error
	timedOut := errors.As(readErr, &netErr) && netErr.Timeout()
	claudeTrue(t, !timedOut, "child MCP process survived context cancellation")
}

func checkClaudeMCPJSON(t *testing.T, raw, wantCmd string, wantArgs []string) {
	t.Helper()
	claudeTrue(t, strings.HasPrefix(raw, "{"), "MCP config is not inline JSON: "+raw)
	var cfg claudeMCPConfig
	claudeNoErr(t, json.Unmarshal([]byte(raw), &cfg))
	srv := cfg.MCPServers["solodawn"]
	claudeEqual(t, "MCP command", srv.Command, wantCmd)
	claudeTrue(t, srv.Args != nil, "MCP args slice is nil")
	claudeEqual(t, "MCP args", srv.Args, wantArgs)
}

func claudeArgAfter(t *testing.T, argv []string, flag string) string {
	t.Helper()
	i := slices.Index(argv, flag)
	claudeTrue(t, i >= 0 && i+1 < len(argv), "missing flag "+flag)
	return argv[i+1]
}

func buildFakeClaudeBinary(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, name), "./fakecli/claude").CombinedOutput()
	if err != nil {
		t.Fatalf("build fake claude: %v\n%s", err, out)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeClaudeScript(t *testing.T, steps string, exit int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.json")
	content := fmt.Sprintf(`{"steps":%s,"exit":%d}`, steps, exit)
	claudeNoErr(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

func startTestMCPPeer(t *testing.T, token string) (*net.TCPListener, []string) {
	t.Helper()
	peer, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	claudeNoErr(t, err)
	t.Cleanup(func() { peer.Close() })
	t.Setenv("SOLODAWN_CLAUDE_TEST_MCP", peer.Addr().String())
	exe, err := os.Executable()
	claudeNoErr(t, err)
	return peer, []string{exe, "-test.run=^TestClaudeMCPShim$", "--", "mcp", "--token", token}
}

func acceptTestMCPConn(peer *net.TCPListener, token string) (net.Conn, *json.Decoder, error) {
	peer.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := peer.Accept()
	if err != nil {
		return nil, nil, err
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	want := "mcp --token " + token + "\n"
	header := make([]byte, len(want))
	if _, err := io.ReadFull(conn, header); err != nil || string(header) != want {
		conn.Close()
		return nil, nil, fmt.Errorf("MCP header %q (%v), want %q", header, err, want)
	}
	return conn, json.NewDecoder(conn), nil
}

func handshakeClaudeMCP(conn net.Conn, dec *json.Decoder) error {
	var msg map[string]any
	steps := []string{
		`{"jsonrpc":"2.0","id":"server-discover-probe-1","error":{"code":-32601,"message":"method not found"}}`,
		`{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1"}}}`,
		"",
		`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`,
	}
	for _, reply := range steps {
		if err := dec.Decode(&msg); err != nil {
			return err
		}
		if reply != "" {
			fmt.Fprintln(conn, reply)
		}
	}
	return nil
}

func serveClaudeApprovalMCP(t *testing.T, peer *net.TCPListener, token string) <-chan error {
	t.Helper()
	// Single reader: caller awaiting <-done in testClaudeApprovalAndResume.
	done := make(chan error, 1)
	go func() {
		conn, dec, err := acceptTestMCPConn(peer, token)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		if err := handshakeClaudeMCP(conn, dec); err != nil {
			done <- err
			return
		}
		const denyText = `{"behavior": "deny", "message": "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."}`
		done <- replyTwoMCPCalls(conn, dec, denyText, `{"behavior": "allow"}`)
	}()
	return done
}

func replyTwoMCPCalls(conn net.Conn, dec *json.Decoder, first, second string) error {
	for i, text := range []string{first, second} {
		var call map[string]any
		if err := dec.Decode(&call); err != nil {
			return err
		}
		quoted, _ := json.Marshal(text)
		fmt.Fprintf(conn, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":%s}],"isError":false}}`+"\n", i+2, quoted)
	}
	return nil
}

func serveClaudeResumeMCP(t *testing.T, peer *net.TCPListener, token string) <-chan error {
	t.Helper()
	// Single reader: caller awaiting <-done in runClaudeResumeCheck.
	done := make(chan error, 1)
	go func() {
		conn, dec, err := acceptTestMCPConn(peer, token)
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		if err := handshakeClaudeMCP(conn, dec); err != nil {
			done <- err
			return
		}
		var call map[string]any
		if err := dec.Decode(&call); err != nil {
			done <- err
			return
		}
		quoted, _ := json.Marshal(`{"token": "` + token + `"}`)
		fmt.Fprintf(conn, `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":%s}],"isError":false}}`+"\n", quoted)
		done <- nil
	}()
	return done
}

func drainUntilTurnEnded(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var got []Event
	for ev := range ch {
		got = append(got, ev)
		if ev.Kind == TurnEnded {
			return got
		}
	}
	t.Fatalf("Events closed before TurnEnded: %+v", got)
	return nil
}

// The stderr tail stays the last 64 KiB however the writes are cut.
func TestOutputTailKeepsTheLast64KiB(t *testing.T) {
	var tail outputTail
	tail.Write([]byte(strings.Repeat("a", outputTailSize-1) + "b"))
	tail.Write([]byte("cd"))
	want := strings.Repeat("a", outputTailSize-3) + "bcd"
	if string(tail) != want {
		t.Fatalf("tail of %d bytes ending %q, want %d ending %q", len(tail), tail[max(0, len(tail)-4):], len(want), want[len(want)-4:])
	}
}
