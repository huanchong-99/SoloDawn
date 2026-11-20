package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
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
	setClaudeTempDir(t)
	launch := Launch{
		Folder:       t.TempDir(),
		Level:        1,
		Shim:         []string{"/bin/solodawn", "mcp", "--token", "tok-1"},
		Instructions: "Speak through post.",
	}
	argv, session, mcpPath, err := claudeCommandArgs(launch, false, false)
	claudeNoErr(t, err)
	t.Cleanup(func() { os.Remove(mcpPath) })
	claudeTrue(t, claudeUUIDPattern.MatchString(session), "generated session is not UUID v4")
	checkClaudeMCPFile(t, mcpPath, "/bin/solodawn", []string{"mcp", "--token", "tok-1"})
	want := []string{
		"claude", "-p", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json",
		"--session-id", session, "--mcp-config", mcpPath, "--strict-mcp-config",
		"--permission-prompt-tool", "mcp__solodawn__approve",
		"--setting-sources", "user", "--allowedTools", "mcp__solodawn",
		"--append-system-prompt", "Speak through post.",
		"--permission-mode", "default", "--tools", "Read,Glob,Grep",
	}
	claudeEqual(t, "structured argv", argv, want)
}

func TestClaudeCommandArgsInteractiveResume(t *testing.T) {
	setClaudeTempDir(t)
	const session = "797fa93e-af8a-4768-b847-42582d5583a0"
	resumed := Launch{
		Folder:     t.TempDir(),
		Session:    session,
		Level:      3,
		Shim:       []string{"solodawn"},
		RepoConfig: true,
	}
	argv, gotSession, mcpPath, err := claudeCommandArgs(resumed, true, false)
	claudeNoErr(t, err)
	t.Cleanup(func() { os.Remove(mcpPath) })
	claudeEqual(t, "interactive session", gotSession, session)
	checkClaudeMCPFile(t, mcpPath, "solodawn", []string{})
	want := []string{
		"claude", "--resume", session, "--mcp-config", mcpPath, "--strict-mcp-config",
		"--setting-sources", "user,project", "--allowedTools", "mcp__solodawn",
		"--permission-mode", "bypassPermissions", "--settings", claudeBypassSettings,
	}
	claudeEqual(t, "interactive argv", argv, want)
}

func TestClaudeInteractiveFirstStartWithInstructions(t *testing.T) {
	setClaudeTempDir(t)
	d := newClaudeDriver()
	argv, session, err := d.Interactive(Launch{
		Folder:       t.TempDir(),
		Level:        1,
		Shim:         []string{"solodawn", "mcp", "--token", "tok-int"},
		Instructions: "Task instructions.",
	})
	claudeNoErr(t, err)
	claudeTrue(t, claudeUUIDPattern.MatchString(session), "session is not UUID v4")
	mcpPath := claudeArgAfter(t, argv, "--mcp-config")
	t.Cleanup(func() { os.Remove(mcpPath) })
	want := []string{
		"claude", "--session-id", session, "--mcp-config", mcpPath, "--strict-mcp-config",
		"--setting-sources", "user", "--allowedTools", "mcp__solodawn",
		"--append-system-prompt", "Task instructions.",
		"--permission-mode", "default", "--tools", "Read,Glob,Grep",
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
		{1, false, true, []string{"--permission-mode", "default", "--tools", "Read,Glob,Grep"}, ""},
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

func TestClaudeStartAndInteractiveErrorsCleanTempFiles(t *testing.T) {
	tempDir := setClaudeTempDir(t)
	d := newClaudeDriver()
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
	entries, readErr := os.ReadDir(tempDir)
	claudeNoErr(t, readErr)
	claudeEqual(t, "temp directory entries", len(entries), 0)

	missingTmp := filepath.Join(t.TempDir(), "missing-tmp")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, missingTmp)
	}
	_, _, err = d.Interactive(base)
	claudeTrue(t, err != nil, "Interactive succeeded with missing temp dir")
}

func TestClaudeInteractiveTempFileRotation(t *testing.T) {
	setClaudeTempDir(t)
	d := newClaudeDriver()
	l := Launch{Folder: t.TempDir(), Session: "797fa93e-af8a-4768-b847-42582d5583a0", Level: 1, Shim: []string{"solodawn", "mcp"}}
	argv1, _, err := d.Interactive(l)
	claudeNoErr(t, err)
	path1 := claudeArgAfter(t, argv1, "--mcp-config")
	_, statErr := os.Stat(path1)
	claudeNoErr(t, statErr)

	argv2, _, err := d.Interactive(l)
	claudeNoErr(t, err)
	path2 := claudeArgAfter(t, argv2, "--mcp-config")
	_, statErr = os.Stat(path1)
	claudeTrue(t, errors.Is(statErr, os.ErrNotExist), "first interactive config still exists")
	_, statErr = os.Stat(path2)
	claudeNoErr(t, statErr)

	d.swapInteractive(l.Session, "")
	_, statErr = os.Stat(path2)
	claudeTrue(t, errors.Is(statErr, os.ErrNotExist), "second interactive config still exists")
	claudeEqual(t, "interactive map len", len(d.interactive), 0)
}

func TestClaudeExecCmdUnixAndWindows(t *testing.T) {
	var unixAttrs struct{ Setpgid bool }
	unixCmd := claudeExecCmd(t.Context(), []string{"-p"}, false, reflect.ValueOf(&unixAttrs).Elem())
	claudeTrue(t, unixAttrs.Setpgid, "expected Setpgid on Unix")
	claudeEqual(t, "unix wait delay", unixCmd.WaitDelay, time.Second)
	claudeEqual(t, "unix args", unixCmd.Args, []string{"claude", "-p"})

	var winAttrs struct{ Setpgid bool }
	winCmd := claudeExecCmd(t.Context(), []string{"-p"}, true, reflect.ValueOf(&winAttrs).Elem())
	claudeTrue(t, !winAttrs.Setpgid, "unexpected Setpgid on Windows")
	claudeEqual(t, "windows wait delay", winCmd.WaitDelay, time.Second)
	if runtime.GOOS == "windows" {
		return
	}
	verifyFakeTaskkill(t, winCmd)
}

func verifyFakeTaskkill(t *testing.T, winCmd *exec.Cmd) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "killed")
	t.Setenv("CLAUDE_TASKKILL_OUT", out)
	script := "#!/bin/sh\nprintf '%s ' \"$@\" > \"$CLAUDE_TASKKILL_OUT\"\n"
	claudeNoErr(t, os.WriteFile(filepath.Join(dir, "taskkill.exe"), []byte(script), 0700))
	t.Setenv("PATH", dir)
	winCmd.Process = &os.Process{Pid: 43210}
	claudeNoErr(t, winCmd.Cancel())
	got, err := os.ReadFile(out)
	claudeNoErr(t, err)
	claudeEqual(t, "taskkill args", string(got), "/f /t /pid 43210 ")
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
		want Event
	}{
		{`{"type":"system","subtype":"init","session_id":"s1"}`, Event{Kind: Ready, Session: "s1"}},
		{`{"type":"assistant","subtype":"init","session_id":"s1"}`, Event{Kind: Active}},
		{`{"type":"system","subtype":"thinking_tokens","session_id":"s1"}`, Event{Kind: Active}},
		{`{"type":"system","subtype":"init","session_id":""}`, Event{Kind: Active}},
		{`{"type":"result","is_error":"bad"}`, Event{Kind: Active}},
	}
	for _, tc := range cases {
		claudeTrue(t, !p.handleLine([]byte(tc.line)), "unexpected diagnostic return")
		claudeEqual(t, tc.line, <-events, tc.want)
	}
	checkClaudeRateEvents(t)
}

func checkClaudeRateEvents(t *testing.T) {
	t.Helper()
	pct44, pct0, pct85 := 44.0, 0.0, 85.0
	resetsMS, altMS := int64(1795615200000), int64(1800000000000)
	cases := []struct {
		line string
		want []Event
	}{
		{`{"type":"rate_limit_event"}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":123}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":"bad"}}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`, []Event{{Kind: Active}}},
		{`{"type":"rate_limit_event","rate_limit_info":{"status":""}}`, []Event{{Kind: Active}}},
		{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed_warning","rateLimitType":"seven_day","unifiedWindows":{"seven_day":{"utilization":0.0,"resetsAt":1795615200}}}}`,
			[]Event{{Kind: Used, Usage: Usage{WindowPct: &pct0, WindowResets: &resetsMS}}},
		},
		{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","utilization":0.85,"resetsAt":1800000000,"rateLimitType":"seven_day","unifiedWindows":{"seven_day":{"utilization":0.44,"resetsAt":1795615200}}}}`,
			[]Event{{Kind: Used, Usage: Usage{WindowPct: &pct85, WindowResets: &altMS}}},
		},
		{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","resetsAt":1795615200,"rateLimitType":"seven_day","unifiedWindows":{"seven_day":{"utilization":0.44}}}}`,
			[]Event{{Kind: Used, Usage: Usage{WindowPct: &pct44, WindowResets: &resetsMS}}, {Kind: RateLimited, Resets: resetsMS}},
		},
		{
			`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected"}}`,
			[]Event{{Kind: RateLimited, Resets: 0}},
		},
	}
	for _, tc := range cases {
		claudeEqual(t, tc.line, claudeRateEvents([]byte(tc.line)), tc.want)
	}
}

func TestClaudeResultAndDiagnosticMapping(t *testing.T) {
	checkClaudeStatuses(t)
	checkClaudeDiagnostics(t)
	checkClaudeUsageMath(t)
}

func checkClaudeStatuses(t *testing.T) {
	t.Helper()
	cases := []struct {
		msg         claudeResultMsg
		interrupted bool
		wantStatus  string
	}{
		{claudeResultMsg{Subtype: "success"}, false, "completed"},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "boom"}, false, "failed"},
		{claudeResultMsg{Subtype: "error_max_turns"}, false, "failed"},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true}, true, "interrupted"},
		{claudeResultMsg{Subtype: "error_during_execution", TerminalReason: "aborted_tools"}, false, "interrupted"},
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "Please run /login"}, true, "interrupted"},
	}
	for _, tc := range cases {
		ev, diag := claudeResultEvent(tc.msg, tc.interrupted, true)
		claudeTrue(t, !diag, "unexpected diagnostic result")
		claudeEqual(t, "turn status", ev, Event{Kind: TurnEnded, Status: tc.wantStatus})
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
		{claudeResultMsg{Subtype: "error_during_execution", IsError: true, Result: "No conversation found"}, false, Event{Kind: TurnEnded, Status: "failed"}, false},
		{claudeResultMsg{Subtype: "success", IsError: true, APIErrorStatus: 401, Result: "HTTP 401"}, false, Event{Kind: LoginInvalid, Text: "HTTP 401"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Please run /login"}, false, Event{Kind: LoginInvalid, Text: "Please run /login"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Not logged in"}, false, Event{Kind: LoginInvalid, Text: "Not logged in"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "Invalid API key"}, false, Event{Kind: LoginInvalid, Text: "Invalid API key"}, true},
		{claudeResultMsg{Subtype: "success", IsError: true, Result: "authentication_failed"}, false, Event{Kind: LoginInvalid, Text: "authentication_failed"}, true},
		{claudeResultMsg{Subtype: "success", APIErrorStatus: 401, Result: "Not logged in"}, false, Event{Kind: TurnEnded, Status: "completed"}, false},
	}
	for _, tc := range cases {
		ev, diag := claudeResultEvent(tc.msg, false, tc.resumed)
		claudeEqual(t, "isDiag", diag, tc.isDiag)
		claudeEqual(t, "diag event", ev, tc.want)
	}
}

func checkClaudeUsageMath(t *testing.T) {
	t.Helper()
	claudeEqual(t, "empty usage", claudeResultUsage(claudeResultMsg{Subtype: "success"}), Usage{})
	fallbackCost := 0.0125
	fb := claudeResultUsage(claudeResultMsg{
		Usage:        &claudeTurnTokens{InputTokens: 10, CacheCreationInputTokens: 20, CacheReadInputTokens: 30, OutputTokens: 40},
		TotalCostUSD: &fallbackCost,
	})
	wantTokens := int64(100)
	claudeEqual(t, "fallback usage", fb, Usage{Tokens: &wantTokens, USD: &fallbackCost})

	delta := claudeResultUsage(claudeResultMsg{
		TotalCostUSD: &fallbackCost,
		SinceLast: map[string]claudeModelDelta{
			"haiku":  {CostUSD: 0.002},
			"sonnet": {CostUSD: 0.003},
		},
	})
	claudeTrue(t, delta.Tokens == nil, "unexpected tokens")
	claudeTrue(t, delta.USD != nil, "missing delta USD")
	claudeTrue(t, math.Abs(*delta.USD-0.005) <= 1e-12, "wrong delta USD sum")
}

func TestClaudeDriverWithFakeCLI(t *testing.T) {
	buildFakeClaudeBinary(t)
	t.Run("FirstStartTwoTurnsAndResume", testClaudeApprovalAndResume)
	t.Run("InterruptAndSecondTurn", testClaudeInterruptAndContinue)
	t.Run("MidTurnExitAndStderrDiagnostics", testClaudeCrashAndStderrDiag)
	t.Run("ContextCancelKillsProcessTree", testClaudeContextCancelKillsTree)
}

func testClaudeApprovalAndResume(t *testing.T) {
	tempDir := setClaudeTempDir(t)
	logFile := filepath.Join(t.TempDir(), "lifecycle.log")
	t.Setenv("SOLODAWN_FAKECLI_LOG", logFile)
	script1, err := filepath.Abs(filepath.Join("fakecli", "testdata", "claude-approval.json"))
	claudeNoErr(t, err)
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", script1)
	peer, shim := startTestMCPPeer(t, "tok-first")

	proc, err := Drivers["claude"].Start(t.Context(), Launch{
		Folder:       t.TempDir(),
		Level:        2,
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
	checkSecondApprovalTurnResult(t, events2[len(events2)-1])

	claudeNoErr(t, proc.Close())
	_, open := <-proc.Events()
	claudeTrue(t, !open, "expected Events closed after Close")
	entries, _ := os.ReadDir(tempDir)
	claudeEqual(t, "leaked temp files", len(entries), 0)
	runClaudeResumeCheck(t, session, logFile)
}

func checkFirstApprovalTurnEvents(t *testing.T, events []Event) {
	t.Helper()
	pct44, resetsMS := 44.0, int64(1795615200000)
	wantUsed := Event{Kind: Used, Usage: Usage{WindowPct: &pct44, WindowResets: &resetsMS}}
	claudeTrue(t, slices.ContainsFunc(events, func(e Event) bool { return reflect.DeepEqual(e, wantUsed) }), "missing Used event")
	claudeTrue(t, slices.Contains(events, Event{Kind: Active}), "missing Active event")
	last := events[len(events)-1]
	claudeEqual(t, "first turn status", last.Status, "completed")
	claudeEqual(t, "first turn tokens", *last.Usage.Tokens, int64(57229))
}

func checkSecondApprovalTurnResult(t *testing.T, last Event) {
	t.Helper()
	claudeEqual(t, "second turn status", last.Status, "completed")
	claudeEqual(t, "second turn tokens", *last.Usage.Tokens, int64(58510))
	claudeTrue(t, math.Abs(*last.Usage.USD-0.00088614) <= 1e-10, "wrong second turn delta USD")
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
	claudeEqual(t, "resumed status", events[len(events)-1].Status, "completed")
	claudeNoErr(t, proc.Close())
	for range proc.Events() {
	}
	gotLog, err := os.ReadFile(logFile)
	claudeNoErr(t, err)
	wantLog := fmt.Sprintf("start claude %s\nexit claude %s\nstart claude %s\nexit claude %s\n", session, session, session, session)
	claudeEqual(t, "lifecycle log", string(gotLog), wantLog)
}

func testClaudeInterruptAndContinue(t *testing.T) {
	setClaudeTempDir(t)
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	script := writeClaudeScript(t, `[
		{"dir":"stdin","msg":{"type":"control_request","request_id":"req-1","request":{"subtype":"interrupt"}}},
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"turn <1> & go"}]}}},
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},
		{"dir":"stdin","msg":{"type":"control_request","request_id":"req-2","request":{"subtype":"interrupt"}}},
		{"dir":"stdout","msg":{"type":"control_response","response":{"subtype":"success","request_id":"req-2"}}},
		{"dir":"stdout","msg":{"type":"result","subtype":"error_during_execution","is_error":true}},
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"turn 2"}]}}},
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
	claudeNoErr(t, proc.Interrupt())
	ev1 := drainUntilTurnEnded(t, proc.Events())
	claudeEqual(t, "turn 1 status", ev1[len(ev1)-1].Status, "interrupted")

	claudeNoErr(t, proc.Turn("turn 2"))
	ev2 := drainUntilTurnEnded(t, proc.Events())
	claudeEqual(t, "turn 2 status", ev2[len(ev2)-1].Status, "failed")
	claudeNoErr(t, proc.Close())
	for range proc.Events() {
	}
}

func testClaudeCrashAndStderrDiag(t *testing.T) {
	setClaudeTempDir(t)
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

func testClaudeContextCancelKillsTree(t *testing.T) {
	setClaudeTempDir(t)
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

func setClaudeTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}
	return dir
}

func checkClaudeMCPFile(t *testing.T, path, wantCmd string, wantArgs []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	claudeNoErr(t, err)
	var cfg claudeMCPConfig
	claudeNoErr(t, json.Unmarshal(raw, &cfg))
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
