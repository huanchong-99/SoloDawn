package host

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
)

// The test binary acts as codex, the MCP shim, or a sleeper child when invoked
// under those names or subcommands in unit tests.
func init() {
	if len(os.Args) > 1 && os.Args[1] == "test-codex-sleep" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "test-codex-mcp" {
		os.Exit(runTestCodexBridge(os.Getenv("SOLODAWN_TEST_MCP_PEER")))
	}
	if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "codex" {
		os.Exit(runTestCodexHelper())
	}
}

func runTestCodexHelper() int {
	if treeFile := os.Getenv("SOLODAWN_TEST_CODEX_TREE"); treeFile != "" {
		return runTestCodexTree(treeFile)
	}
	if peer := os.Getenv("SOLODAWN_TEST_CODEX_PEER"); peer != "" {
		return runTestCodexBridge(peer)
	}
	return fakecli.Run("codex", "codex-cli 0.164.0", os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func runTestCodexTree(treeFile string) int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}
	child := exec.Command(exe, "test-codex-sleep")
	if err := child.Start(); err != nil {
		return 1
	}
	if os.WriteFile(treeFile, []byte(strconv.Itoa(child.Process.Pid)), 0600) != nil {
		return 1
	}
	time.Sleep(30 * time.Second)
	return 0
}

type codexBridgeHeader struct {
	Args    []string `json:"args"`
	Dir     string   `json:"dir"`
	NoColor string   `json:"no_color"`
}

func runTestCodexBridge(addr string) int {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return 1
	}
	defer conn.Close()
	cwd, _ := os.Getwd()
	data, _ := json.Marshal(codexBridgeHeader{Args: os.Args[1:], Dir: cwd, NoColor: os.Getenv("NO_COLOR")})
	fmt.Fprintln(conn, string(data))
	go func() {
		io.Copy(conn, os.Stdin)
		if tcp, ok := conn.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	io.Copy(os.Stdout, conn)
	return 0
}

func codexNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func codexWantErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func codexEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func codexCheck(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}

func installFakeCodexBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	name := "codex"
	if runtime.GOOS == "windows" {
		name = "codex.exe"
	}
	exe, err := os.Executable()
	codexNoErr(t, err)
	linkOrCopyCodex(t, exe, filepath.Join(binDir, name))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDir
}

func linkOrCopyCodex(t *testing.T, src, dst string) {
	t.Helper()
	if os.Symlink(src, dst) == nil || os.Link(src, dst) == nil {
		return
	}
	raw, err := os.ReadFile(src)
	codexNoErr(t, err)
	codexNoErr(t, os.WriteFile(dst, raw, 0700))
}

type codexPeer struct {
	t      *testing.T
	ln     net.Listener
	conn   net.Conn
	reader *bufio.Reader
	header codexBridgeHeader
}

func listenCodexPeer(t *testing.T, envKey string) *codexPeer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	codexNoErr(t, err)
	t.Cleanup(func() { ln.Close() })
	t.Setenv(envKey, ln.Addr().String())
	return &codexPeer{t: t, ln: ln}
}

func (cp *codexPeer) accept() codexBridgeHeader {
	cp.t.Helper()
	if tcp, ok := cp.ln.(*net.TCPListener); ok {
		tcp.SetDeadline(time.Now().Add(10 * time.Second))
	}
	conn, err := cp.ln.Accept()
	codexNoErr(cp.t, err)
	cp.t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	cp.conn = conn
	cp.reader = bufio.NewReader(conn)
	line, err := cp.reader.ReadBytes('\n')
	codexNoErr(cp.t, err)
	codexNoErr(cp.t, json.Unmarshal(line, &cp.header))
	return cp.header
}

func (cp *codexPeer) readJSON() map[string]any {
	cp.t.Helper()
	line, err := cp.reader.ReadBytes('\n')
	codexNoErr(cp.t, err)
	var msg map[string]any
	codexNoErr(cp.t, json.Unmarshal(line, &msg))
	return msg
}

func (cp *codexPeer) writeLine(raw string) {
	cp.t.Helper()
	_, err := fmt.Fprintln(cp.conn, raw)
	codexNoErr(cp.t, err)
}

func (cp *codexPeer) expectEOF() {
	cp.t.Helper()
	buf := make([]byte, 16)
	_, err := cp.reader.Read(buf)
	codexEqual(cp.t, err, io.EOF)
}

func codexTestLaunch(t *testing.T, level int) Launch {
	t.Helper()
	exe, err := os.Executable()
	codexNoErr(t, err)
	return Launch{
		Folder:       t.TempDir(),
		Level:        level,
		Shim:         []string{exe, "test-codex-mcp", "--token", "tok-1"},
		Instructions: "Follow the rules.",
		ToolTimeout:  300 * time.Second,
	}
}

func nextCodexEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		codexCheck(t, ok, "events channel closed early")
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal(timedOut(10*time.Second, "an event", nil))
		return Event{}
	}
}

func nextNonActiveEvent(t *testing.T, ch <-chan Event) (Event, int) {
	t.Helper()
	actives := 0
	for {
		ev := nextCodexEvent(t, ch)
		if ev.Kind == Active {
			actives++
			continue
		}
		return ev, actives
	}
}

func waitEventsClosed(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var rest []Event
	var saw []string
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return rest
			}
			rest = append(rest, ev)
			saw = append(saw, string(ev.Kind))
		case <-timeout:
			t.Fatal(timedOut(10*time.Second, "the events channel to close", saw))
		}
	}
}

func TestCodexDriverRegistered(t *testing.T) {
	codexCheck(t, Drivers["codex"] != nil, "codex driver is missing from Drivers")
}

func TestCodexInteractiveErrors(t *testing.T) {
	drv := Drivers["codex"]
	for _, bad := range []Launch{
		{Folder: "/w", Level: 2, Shim: []string{"solodawn"}},
		{Folder: "/w", Session: "s", Level: 2},
		{Folder: "/w", Session: "s", Level: 2, Shim: []string{""}},
		{Folder: "/w", Session: "s", Level: 0, Shim: []string{"solodawn"}},
		{Folder: "/w", Session: "s", Level: 4, Shim: []string{"solodawn"}},
	} {
		argv, session, err := drv.Interactive(bad)
		codexWantErr(t, err)
		codexEqual(t, argv, []string(nil))
		codexEqual(t, session, "")
	}
}

func TestCodexInteractiveFlagsAcrossLevels(t *testing.T) {
	drv := Drivers["codex"]
	l2 := Launch{
		Folder:      "/work/task-1",
		Session:     "thr-123",
		Level:       2,
		Shim:        []string{"/bin/solodawn&node", "mcp", "--token", "tok-abc"},
		RepoConfig:  true,
		ToolTimeout: 600 * time.Second,
	}
	argv2, session2, err := drv.Interactive(l2)
	codexNoErr(t, err)
	codexEqual(t, session2, "thr-123")
	codexEqual(t, argv2, []string{
		"codex", "resume", "thr-123",
		"--no-alt-screen", "--no-daemon",
		"-C", "/work/task-1",
		"-c", `mcp_servers.solodawn.command="/bin/solodawn&node"`,
		"-c", `mcp_servers.solodawn.args=["mcp","--token","tok-abc"]`,
		"-c", `mcp_servers.solodawn.default_tools_approval_mode="approve"`,
		"-c", "mcp_servers.solodawn.tool_timeout_sec=600",
		"-c", "notify=[]",
		"-c", "features.default_mode_request_user_input=true",
		"-c", `projects={"/work/task-1"={trust_level="trusted"}}`,
		"-s", "workspace-write",
		"-a", "on-request",
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true",
		"-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
	})
	codexCheck(t, !slices.Contains(argv2, "--skip-git-repo-check"), "unexpected --skip-git-repo-check in resume argv")

	argv1, _, err := drv.Interactive(Launch{Folder: "/repo", Session: "thr-1", Level: 1, Shim: []string{"solodawn"}})
	codexNoErr(t, err)
	codexCheck(t, slices.Contains(argv1, `mcp_servers.solodawn.args=[]`), "expected empty JSON args array")
	codexCheck(t, !slices.ContainsFunc(argv1, func(a string) bool { return strings.HasPrefix(a, "projects=") }), "unexpected projects flag for group session")
	codexEqual(t, argv1[len(argv1)-2:], []string{"-c", "project_doc_max_bytes=0"})
	codexEqual(t, argv1[slices.Index(argv1, "-s")+1], "read-only")
	codexEqual(t, argv1[slices.Index(argv1, "-a")+1], "never")

	argv3, _, err := drv.Interactive(Launch{Folder: "/work/review-3", Session: "thr-3", Level: 3, Shim: []string{"solodawn"}, RepoConfig: true})
	codexNoErr(t, err)
	codexCheck(t, slices.Contains(argv3, `projects={"/work/review-3"={trust_level="trusted"}}`), "expected trust flag for review session")
	codexCheck(t, !slices.Contains(argv3, "project_doc_max_bytes=0"), "unexpected project_doc_max_bytes=0")
	codexEqual(t, argv3[slices.Index(argv3, "-s")+1], "danger-full-access")
	codexEqual(t, argv3[slices.Index(argv3, "-a")+1], "never")
}

func TestCodexStartValidationAndMissingExecutable(t *testing.T) {
	drv := Drivers["codex"]
	for _, bad := range []Launch{
		{Folder: t.TempDir(), Level: 2},
		{Folder: t.TempDir(), Level: 2, Shim: []string{""}},
		{Folder: t.TempDir(), Level: 9, Shim: []string{"solodawn"}},
	} {
		proc, err := drv.Start(t.Context(), bad)
		codexWantErr(t, err)
		codexEqual(t, proc, nil)
	}
	t.Setenv("PATH", t.TempDir())
	proc, err := drv.Start(t.Context(), codexTestLaunch(t, 1))
	codexWantErr(t, err)
	codexEqual(t, proc, nil)
}

func answerFakeCodexMCP(cp *codexPeer, token string, answerCall bool) {
	hdr := cp.accept()
	codexCheck(cp.t, slices.Contains(hdr.Args, token), "missing token in MCP args")
	codexEqual(cp.t, cp.readJSON()["method"], "initialize")
	cp.writeLine(`{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"solodawn","version":"1"}}}`)
	codexEqual(cp.t, cp.readJSON()["method"], "notifications/initialized")
	codexEqual(cp.t, cp.readJSON()["method"], "tools/list")
	cp.writeLine(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	if answerCall {
		codexEqual(cp.t, cp.readJSON()["method"], "tools/call")
		cp.writeLine(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"{\"token\": \"tok-resumed\"}"}],"isError":false}}`)
	}
}

func setupFakeCodexScript(t *testing.T, logPath, relScript string) {
	t.Helper()
	scriptPath, err := filepath.Abs(relScript)
	codexNoErr(t, err)
	t.Setenv("SOLODAWN_FAKECLI_LOG", logPath)
	t.Setenv("SOLODAWN_FAKE_CODEX_SCRIPT", scriptPath)
}

func TestCodexFakeCLIApprovalTwoTurnsOneProcess(t *testing.T) {
	installFakeCodexBinary(t)
	mcpPeer := listenCodexPeer(t, "SOLODAWN_TEST_MCP_PEER")
	logPath := filepath.Join(t.TempDir(), "fakecli.log")
	setupFakeCodexScript(t, logPath, "fakecli/testdata/codex-approval.json")

	proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 2))
	codexNoErr(t, err)
	t.Cleanup(func() { proc.Close() })

	mcpDone := make(chan struct{})
	go func() {
		defer close(mcpDone)
		answerFakeCodexMCP(mcpPeer, "tok-1", false)
	}()

	const wantThread = "01a1f63f-701c-78c1-bbf3-f1ac61c56b59"
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: wantThread})
	firstID := runApprovalTurnOne(t, proc)
	runApprovalTurnTwo(t, proc, firstID)
	<-mcpDone

	codexNoErr(t, proc.Close())
	codexNoErr(t, proc.Close())
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
	assertCodexLog(t, logPath, "start codex "+wantThread, "exit codex "+wantThread)
}

func runApprovalTurnOne(t *testing.T, proc Process) string {
	t.Helper()
	codexNoErr(t, proc.Turn("Try the write."))
	ev, actives := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, ev.Kind, Asked)
	codexCheck(t, actives > 0 && ev.Ask != nil && ev.Ask.ID != "", "expected active events and Ask ID")
	wantAsk := &Ask{
		ID:       ev.Ask.ID,
		Tool:     "commandExecution",
		Category: "command",
		Command:  "/bin/zsh -lc 'echo retry-codex > $WORK/outside/x3.txt'",
		Why:      "Allow writing retry-codex to the requested path outside the workspace?",
		Input:    json.RawMessage(`{"command":"/bin/zsh -lc 'echo retry-codex > $WORK/outside/x3.txt'","cwd":"$WORK/xproj3"}`),
	}
	codexEqual(t, ev.Ask, wantAsk)
	codexNoErr(t, proc.Answer(ev.Ask.ID, false))
	codexWantErr(t, proc.Answer(ev.Ask.ID, false))
	end := drainCodexUntilTurnEnd(t, proc.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "completed"})
	return ev.Ask.ID
}

func runApprovalTurnTwo(t *testing.T, proc Process, firstAskID string) {
	t.Helper()
	codexNoErr(t, proc.Turn("Approved. Retry."))
	ev, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, ev.Kind, Asked)
	codexCheck(t, ev.Ask.ID != firstAskID, "expected distinct Ask ID on retry")
	codexNoErr(t, proc.Answer(ev.Ask.ID, true))
	end := drainCodexUntilTurnEnd(t, proc.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "completed"})
}

func drainCodexUntilTurnEnd(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	sawUsed := false
	for {
		ev, _ := nextNonActiveEvent(t, ch)
		if ev.Kind == Used {
			sawUsed = true
			continue
		}
		codexEqual(t, ev.Kind, TurnEnded)
		codexCheck(t, sawUsed, "TurnEnded arrived without preceding Used events")
		return ev
	}
}

func assertCodexLog(t *testing.T, path string, lines ...string) {
	t.Helper()
	got, err := os.ReadFile(path)
	codexNoErr(t, err)
	codexEqual(t, string(got), strings.Join(lines, "\n")+"\n")
}

func TestCodexFakeCLIExitMidTurnAndResume(t *testing.T) {
	installFakeCodexBinary(t)
	logPath := filepath.Join(t.TempDir(), "fakecli.log")
	threadID := runMidTurnExitProcess(t, logPath)

	mcp2 := listenCodexPeer(t, "SOLODAWN_TEST_MCP_PEER")
	setupFakeCodexScript(t, logPath, "fakecli/testdata/codex-resume.json")
	exe, err := os.Executable()
	codexNoErr(t, err)
	l2 := codexTestLaunch(t, 2)
	l2.Session = threadID
	l2.Shim = []string{exe, "test-codex-mcp", "--token", "tok-resumed"}

	p2, err := Drivers["codex"].Start(t.Context(), l2)
	codexNoErr(t, err)
	t.Cleanup(func() { p2.Close() })
	mcpDone := make(chan struct{})
	go func() {
		defer close(mcpDone)
		answerFakeCodexMCP(mcp2, "tok-resumed", true)
	}()

	turnDone := make(chan error, 1)
	go func() { turnDone <- p2.Turn("Check the token.") }()

	codexEqual(t, nextCodexEvent(t, p2.Events()), Event{Kind: Ready, Session: threadID})
	codexNoErr(t, <-turnDone)
	end := drainCodexUntilTurnEnd(t, p2.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "completed"})
	<-mcpDone

	codexNoErr(t, p2.Close())
	waitEventsClosed(t, p2.Events())
	assertCodexLog(t, logPath, "start codex "+threadID, "exit codex "+threadID, "start codex "+threadID, "exit codex "+threadID)
}

func runMidTurnExitProcess(t *testing.T, logPath string) string {
	t.Helper()
	mcp1 := listenCodexPeer(t, "SOLODAWN_TEST_MCP_PEER")
	setupFakeCodexScript(t, logPath, "fakecli/testdata/codex-exit-mid-turn.json")
	p1, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 2))
	codexNoErr(t, err)
	t.Cleanup(func() { p1.Close() })
	go answerFakeCodexMCP(mcp1, "tok-1", false)

	ready := nextCodexEvent(t, p1.Events())
	codexNoErr(t, p1.Turn("Try the write."))
	asked, _ := nextNonActiveEvent(t, p1.Events())
	codexEqual(t, asked.Kind, Asked)
	codexEqual(t, len(waitEventsClosed(t, p1.Events())), 0)
	return ready.Session
}

func startPeerCodex(t *testing.T, l Launch) (Process, *codexPeer) {
	t.Helper()
	installFakeCodexBinary(t)
	peer := listenCodexPeer(t, "SOLODAWN_TEST_CODEX_PEER")
	proc, err := Drivers["codex"].Start(t.Context(), l)
	codexNoErr(t, err)
	t.Cleanup(func() { proc.Close() })
	return proc, peer
}

func completePeerHandshake(t *testing.T, peer *codexPeer, threadID string) (initMsg, threadMsg map[string]any) {
	t.Helper()
	peer.accept()
	initMsg = peer.readJSON()
	peer.writeLine(`{"id":1,"result":{}}`)
	codexEqual(t, peer.readJSON()["method"], "initialized")
	threadMsg = peer.readJSON()
	peer.writeLine(fmt.Sprintf(`{"id":2,"result":{"thread":{"id":%q}}}`, threadID))
	return initMsg, threadMsg
}

type codexHandshakeCase struct {
	level       int
	repoConfig  bool
	session     string
	wantMethod  string
	wantSand    string
	wantPol     string
	wantSWW     bool
	wantDocZero bool
}

func TestCodexHandshakeParametersAcrossLevelsAndRepoConfig(t *testing.T) {
	for _, tc := range []codexHandshakeCase{
		{1, false, "", "thread/start", "read-only", "never", false, true},
		{2, false, "", "thread/start", "workspace-write", "on-request", true, true},
		{2, true, "thr-9", "thread/resume", "workspace-write", "on-request", true, false},
		{3, true, "", "thread/start", "danger-full-access", "never", false, false},
	} {
		verifyHandshakeCase(t, tc)
	}
}

func verifyHandshakeCase(t *testing.T, tc codexHandshakeCase) {
	t.Helper()
	l := codexTestLaunch(t, tc.level)
	l.RepoConfig = tc.repoConfig
	l.Session = tc.session
	proc, peer := startPeerCodex(t, l)
	initMsg, threadMsg := completePeerHandshake(t, peer, "thr-9")
	codexEqual(t, peer.header.Args[0], "app-server")
	codexEqual(t, peer.header.NoColor, "1")
	wantTrust := "projects={" + marshalWire(l.Folder) + `={trust_level="trusted"}}`
	codexEqual(t, slices.Contains(peer.header.Args, wantTrust), tc.repoConfig)
	codexCheck(t, slices.Contains(peer.header.Args, "mcp_servers.solodawn.required=true"), "missing required on app-server")
	codexCheck(t, slices.Contains(peer.header.Args, "features.default_mode_request_user_input=true"), "missing default_mode_request_user_input flag on app-server")
	codexEqual(t, initMsg["method"], "initialize")
	codexEqual(t, threadMsg["method"], tc.wantMethod)
	assertThreadParams(t, threadMsg["params"].(map[string]any), l, tc)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "thr-9"})
	codexNoErr(t, proc.Close())
	peer.expectEOF()
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func assertThreadParams(t *testing.T, params map[string]any, l Launch, tc codexHandshakeCase) {
	t.Helper()
	codexEqual(t, params["cwd"], l.Folder)
	codexEqual(t, params["sandbox"], tc.wantSand)
	codexEqual(t, params["approvalPolicy"], tc.wantPol)
	codexEqual(t, params["developerInstructions"], l.Instructions)
	_, hasModel := params["model"]
	codexEqual(t, hasModel, false)
	codexEqual(t, params["threadId"] == "thr-9", tc.session != "")
	cfg := params["config"].(map[string]any)
	_, hasSWW := cfg["sandbox_workspace_write"]
	docVal, hasDoc := cfg["project_doc_max_bytes"]
	codexEqual(t, hasSWW, tc.wantSWW)
	codexEqual(t, hasDoc, tc.wantDocZero)
	if tc.wantDocZero {
		codexEqual(t, docVal, float64(0))
	}
}

func TestCodexFileChangeApprovalAndUnsupportedServerRequests(t *testing.T) {
	proc, peer := startPeerCodex(t, codexTestLaunch(t, 2))
	completePeerHandshake(t, peer, "thr-fc")
	nextCodexEvent(t, proc.Events())

	codexNoErr(t, proc.Turn("Apply file patch."))
	codexWantErr(t, proc.Turn("Concurrent turn."))
	turnReq := peer.readJSON()
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":"turn-fc"}}}`, int(turnReq["id"].(float64))))

	verifyFileChangeApproval(t, proc, peer)
	verifyCompletedFileItemFallback(t, proc, peer)
	verifyDeclinedServerRequests(t, peer)

	peer.writeLine(`{"method":"turn/completed","params":{"turn":{"id":"turn-fc","status":"completed","error":null}}}`)
	end, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "completed"})
	codexNoErr(t, proc.Close())
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func verifyFileChangeApproval(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	changesJSON := `[{"path":"a.go","kind":{"type":"add"},"diff":"+pkg\n"},{"path":"old.go","kind":{"type":"update","move_path":"new.go"},"diff":"-x\n+y"},{"path":"gone.go","kind":{"type":"delete"},"diff":""}]`
	peer.writeLine(`{"method":"item/started","params":"malformed"}`)
	peer.writeLine(`{"method":"item/started","params":{"item":{"type":"agentMessage","id":"m1"}}}`)
	peer.writeLine(`{"method":"item/started","params":{"item":{"type":"fileChange","id":"fc-keep","changes":` + changesJSON + `}}}`)
	peer.writeLine(`{"method":"item/started","params":{"item":{"type":"fileChange","id":"fc-done","changes":[{"path":"tmp","kind":{"type":"add"},"diff":"+1"}]}}}`)
	peer.writeLine(`{"method":"item/completed","params":{"item":{"type":"fileChange","id":"fc-done","changes":[]}}}`)
	peer.writeLine(`{"method":"thread/status/changed","params":{"status":{"type":"active"}}}`)
	peer.writeLine(`{"id":"req-str","method":"item/fileChange/requestApproval","params":{"itemId":"fc-keep","reason":"Update files outside root"}}`)

	ev, actives := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, actives, 6)
	codexEqual(t, ev.Ask, &Ask{
		ID:       ev.Ask.ID,
		Tool:     "fileChange",
		Category: "file",
		Command:  "add a.go\n+pkg\nupdate old.go -> new.go\n-x\n+y\ndelete gone.go",
		Why:      "Update files outside root",
		Input:    json.RawMessage(changesJSON),
	})
	codexNoErr(t, proc.Answer(ev.Ask.ID, true))
	ans := peer.readJSON()
	codexEqual(t, ans["id"], "req-str")
	codexEqual(t, ans["result"].(map[string]any)["decision"], "accept")
}

func verifyCompletedFileItemFallback(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	peer.writeLine(`{"id":11,"method":"item/fileChange/requestApproval","params":{"itemId":"fc-done","reason":null}}`)
	ev, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, ev.Ask, &Ask{
		ID:       ev.Ask.ID,
		Tool:     "fileChange",
		Category: "file",
		Input:    json.RawMessage("[]"),
	})
	codexNoErr(t, proc.Answer(ev.Ask.ID, false))
	codexEqual(t, peer.readJSON()["result"].(map[string]any)["decision"], "decline")
}

func verifyDeclinedServerRequests(t *testing.T, peer *codexPeer) {
	t.Helper()
	for i, line := range []string{
		`{"id":60,"method":"mcpServer/elicitation/request","params":{}}`,
		`{"id":61,"method":"item/permissions/requestApproval","params":{}}`,
		`{"id":62,"method":"item/tool/call","params":{}}`,
		`{"id":63,"method":"future/unknownRequest","params":{}}`,
		`{"id":64,"method":"item/commandExecution/requestApproval","params":"bad"}`,
		`{"id":65,"method":"item/fileChange/requestApproval","params":"bad"}`,
	} {
		peer.writeLine(line)
		reply := peer.readJSON()
		codexEqual(t, int(reply["id"].(float64)), 60+i)
		codexEqual(t, reply["error"].(map[string]any)["code"], -32601.0)
	}
}

func TestCodexUserInputRequestsAndStaleReplies(t *testing.T) {
	proc, peer := startPeerCodex(t, codexTestLaunch(t, 2))
	completePeerHandshake(t, peer, "thr-ui")
	nextCodexEvent(t, proc.Events())

	codexNoErr(t, proc.Turn("Ask the owner."))
	turnReq := peer.readJSON()
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":"turn-ui"}}}`, int(turnReq["id"].(float64))))

	verifyDeclinedUserInputRequests(t, peer)
	verifyAnsweredUserInputRequests(t, proc, peer)
	verifyStaleUserInputAfterTurnEnd(t, proc, peer)

	codexNoErr(t, proc.Close())
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func verifyDeclinedUserInputRequests(t *testing.T, peer *codexPeer) {
	t.Helper()
	for i, params := range []string{
		`"bad"`,
		`{"questions":[{"id":"q1","question":"Pick?","isSecret":"bad"}]}`,
		`{}`,
		`{"questions":[]}`,
		`{"questions":[{"id":"","question":"Pick?"}]}`,
		`{"questions":[{"id":"q1","question":""}]}`,
		`{"questions":[{"id":"q1","question":"First?"},{"id":"q2","question":""}]}`,
	} {
		peer.writeLine(fmt.Sprintf(`{"id":%d,"method":"item/tool/requestUserInput","params":%s}`, 200+i, params))
		reply := peer.readJSON()
		codexEqual(t, int(reply["id"].(float64)), 200+i)
		codexEqual(t, reply["error"].(map[string]any)["code"], -32601.0)
	}
}

func verifyAnsweredUserInputRequests(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	rawParams := `{"questions":[{"id":"q1","header":"Storage","question":"Which database?","isSecret":false,"options":[{"label":"SQLite","description":"Embedded"},{"label":"Postgres","description":"Server"}]}]}`
	peer.writeLine(`{"id":"ui-1","method":"item/tool/requestUserInput","params":` + rawParams + `}`)
	ev, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, ev.Kind, Asked)
	codexCheck(t, ev.Ask != nil && ev.Ask.ID != "" && ev.Ask.Reply != nil, "expected user-input Ask with Reply")
	codexEqual(t, ev.Ask.Tool, "requestUserInput")
	codexEqual(t, ev.Ask.Category, "")
	codexEqual(t, ev.Ask.Command, "")
	codexEqual(t, ev.Ask.Why, "")
	codexEqual(t, string(ev.Ask.Input), rawParams)
	codexWantErr(t, proc.Answer(ev.Ask.ID, true))
	codexNoErr(t, ev.Ask.Reply(map[string][]string{"q1": {"SQLite"}}))
	codexWantErr(t, ev.Ask.Reply(map[string][]string{"q1": {"SQLite"}}))
	codexEqual(t, peer.readJSON(), map[string]any{
		"id": "ui-1",
		"result": map[string]any{
			"answers": map[string]any{
				"q1": map[string]any{"answers": []any{"SQLite"}},
			},
		},
	})

	peer.writeLine(`{"id":220,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q2","question":"Optional note?"}]}}`)
	ev2, _ := nextNonActiveEvent(t, proc.Events())
	codexNoErr(t, ev2.Ask.Reply(nil))
	codexWantErr(t, ev2.Ask.Reply(nil))
	codexEqual(t, peer.readJSON(), map[string]any{
		"id":     220.0,
		"result": map[string]any{"answers": map[string]any{}},
	})
}

func verifyStaleUserInputAfterTurnEnd(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	peer.writeLine(`{"id":221,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q3","question":"Still open?"}]}}`)
	ev, _ := nextNonActiveEvent(t, proc.Events())
	peer.writeLine(`{"method":"turn/completed","params":{"turn":{"id":"turn-ui","status":"completed","error":null}}}`)
	end, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "completed"})
	codexWantErr(t, ev.Ask.Reply(map[string][]string{"q3": {"late"}}))
}

func TestCodexInterruptBeforeAndAfterTurnIDAndSilentTurn(t *testing.T) {
	proc, peer := startPeerCodex(t, codexTestLaunch(t, 2))
	completePeerHandshake(t, peer, "thr-int")
	nextCodexEvent(t, proc.Events())

	codexWantErr(t, proc.Interrupt())
	verifyDeferredInterruptOnSilentTurn(t, proc, peer)
	verifyInterruptClearsOpenAsk(t, proc, peer)

	codexNoErr(t, proc.Close())
	codexWantErr(t, proc.Turn("After close."))
	codexWantErr(t, proc.Interrupt())
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func verifyDeferredInterruptOnSilentTurn(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	codexNoErr(t, proc.Turn("Silent turn interrupted before turn id."))
	turn1 := peer.readJSON()
	codexNoErr(t, proc.Interrupt())
	peer.writeLine(`{"id":"non-numeric","result":{}}`)
	peer.writeLine(`{"id":999,"result":{"turn":{"id":"other"}}}`)
	peer.writeLine(`{"id":3,"result":"bad"}`)
	peer.writeLine(`{"id":3,"result":{"turn":{}}}`)
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":"turn-1"}}}`, int(turn1["id"].(float64))))

	intReq := peer.readJSON()
	codexEqual(t, intReq["id"], 4.0)
	codexEqual(t, intReq["method"], "turn/interrupt")
	codexEqual(t, intReq["params"], map[string]any{"threadId": "thr-int", "turnId": "turn-1"})
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{}}`, int(intReq["id"].(float64))))
	peer.writeLine(`{"method":"turn/completed","params":{"turn":{"id":"turn-1","status":"interrupted","error":null}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "interrupted"})
}

func verifyInterruptClearsOpenAsk(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	codexNoErr(t, proc.Turn("Turn interrupted while waiting on approval."))
	turn2 := peer.readJSON()
	codexEqual(t, turn2["id"], 5.0)
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":"turn-2"}}}`, int(turn2["id"].(float64))))
	peer.writeLine(`{"id":7,"method":"item/commandExecution/requestApproval","params":{"command":"ls","cwd":"/w","reason":"check"}}`)
	askEv := nextCodexEvent(t, proc.Events())
	codexEqual(t, askEv.Kind, Asked)
	codexNoErr(t, proc.Interrupt())
	intReq := peer.readJSON()
	codexEqual(t, intReq["id"], 6.0)
	codexEqual(t, intReq["method"], "turn/interrupt")
	codexNoErr(t, proc.Interrupt())
	intReq2 := peer.readJSON()
	codexEqual(t, intReq2["id"], 7.0)
	peer.writeLine(`{"method":"turn/completed","params":{"turn":{"id":"turn-2","status":"interrupted","error":null}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "interrupted"})
	codexWantErr(t, proc.Answer(askEv.Ask.ID, true))
}

func TestCodexTurnAuthAndRPCErrors(t *testing.T) {
	proc, peer := startPeerCodex(t, codexTestLaunch(t, 1))
	completePeerHandshake(t, peer, "thr-lim")
	nextCodexEvent(t, proc.Events())

	for _, ignored := range []string{
		"not-json",
		`{}`,
		`{"method":"thread/status/changed","params":{}}`,
		`{"method":"account/rateLimits/updated","params":"bad"}`,
		`{"method":"account/rateLimits/updated","params":{"rateLimits":{}}}`,
		`{"method":"turn/completed","params":"bad"}`,
	} {
		peer.writeLine(ignored)
	}
	peer.writeLine(`{"id":50,"method":"ping","params":{}}`)
	codexEqual(t, peer.readJSON()["id"], 50.0)
	exerciseCodexRateLimitNotifications(t, proc, peer)
	exerciseCodexTurnErrors(t, proc, peer)

	codexNoErr(t, proc.Close())
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func exerciseCodexRateLimitNotifications(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"secondary":{"usedPercent":40}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(40.0)}})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":50,"resetsAt":100},"secondary":{"usedPercent":75,"resetsAt":200},"rateLimitReachedType":"tokens"}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(75.0), WindowResets: new(int64(200000))}})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: RateLimited, Resets: 200000})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":100,"resetsAt":300},"secondary":{"usedPercent":60,"resetsAt":200}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(100.0), WindowResets: new(int64(300000))}})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: RateLimited, Resets: 300000})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":99,"resetsAt":350},"spendControlReached":true}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(99.0), WindowResets: new(int64(350000))}})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: RateLimited, Resets: 350000})
}

func exerciseCodexTurnErrors(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	codexNoErr(t, proc.Turn("rpc-fail"))
	req := peer.readJSON()
	codexEqual(t, req["id"], 3.0)
	peer.writeLine(`{"id":999,"error":{"code":-32000,"message":"unrelated"}}`)
	peer.writeLine(`{"id":3,"error":{"code":-32000,"message":"turn rejected"}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "failed", Text: "turn rejected"})

	for i, tc := range []struct {
		info   string
		wantEv Event
	}{
		{`"unauthorized"`, Event{Kind: LoginInvalid, Text: "err-msg"}},
		{`"usageLimitExceeded"`, Event{Kind: RateLimited, Resets: 350000}},
		{`"otherError"`, Event{}},
		{`{"stream":{"httpStatusCode":401}}`, Event{Kind: LoginInvalid, Text: "err-msg"}},
		{`{"stream":{"httpStatusCode":400}}`, Event{}},
		{`{"stream":{"httpStatusCode":402}}`, Event{}},
		{`{}`, Event{}},
		{`[1]`, Event{}},
	} {
		runFailedTurnErrorCase(t, proc, peer, i, tc.info, tc.wantEv)
	}
}

func runFailedTurnErrorCase(t *testing.T, proc Process, peer *codexPeer, idx int, info string, wantEv Event) {
	t.Helper()
	codexNoErr(t, proc.Turn(fmt.Sprintf("turn-%d", idx)))
	req := peer.readJSON()
	turnID := fmt.Sprintf("tid-%d", idx)
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":%q}}}`, int(req["id"].(float64)), turnID))
	peer.writeLine(fmt.Sprintf(`{"method":"turn/completed","params":{"turn":{"id":%q,"status":"failed","error":{"message":"err-msg","codexErrorInfo":%s}}}}`, turnID, info))
	if wantEv.Kind != "" {
		ev, _ := nextNonActiveEvent(t, proc.Events())
		codexEqual(t, ev, wantEv)
	}
	end, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "failed", Text: "err-msg"})
}

type codexStartupFailureCase struct {
	name     string
	session  string
	failID   int
	reply    string
	wantKind EventKind
	wantErr  string
}

func TestCodexStartupFailuresNoSessionAndLoginInvalid(t *testing.T) {
	for _, tc := range []codexStartupFailureCase{
		{"resume missing thread", "thr-missing", 2, `{"id":2,"error":{"code":-32600,"message":"no rollout found for thread id thr-missing"}}`, NoSession, "no rollout found for thread id thr-missing"},
		{"resume init auth error", "thr-missing", 1, `{"id":1,"error":{"code":-32000,"message":"Not Authenticated"}}`, LoginInvalid, "Not Authenticated"},
		{"start login error", "", 2, `{"id":2,"error":{"code":-32000,"message":"Login required"}}`, LoginInvalid, "Login required"},
		{"start non-auth error", "", 2, `{"id":2,"error":{"code":-32000,"message":"invalid configuration"}}`, "", "invalid configuration"},
		{"malformed thread result", "", 2, `{"id":2,"result":"bad"}`, "", "codex: missing thread id"},
		{"empty thread id", "", 2, `{"id":2,"result":{"thread":{}}}`, "", "codex: missing thread id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runStartupFailureCase(t, tc)
		})
	}
}

func runStartupFailureCase(t *testing.T, tc codexStartupFailureCase) {
	t.Helper()
	l := codexTestLaunch(t, 1)
	l.Session = tc.session
	proc, peer := startPeerCodex(t, l)
	peer.accept()
	peer.readJSON()
	if tc.failID != 1 {
		peer.writeLine(`{"id":1,"result":{}}`)
		peer.readJSON()
		peer.readJSON()
	}
	peer.writeLine(tc.reply)
	peer.conn.Close()
	events := waitEventsClosed(t, proc.Events())
	assertStartupEvents(t, events, tc.wantKind)
	err := proc.Turn("hello")
	codexWantErr(t, err)
	codexEqual(t, err.Error(), tc.wantErr)
}

func TestCodexTrustsFolderWithInlineTableForDotAndQuotedPaths(t *testing.T) {
	drv := Drivers["codex"]
	for _, tc := range []struct {
		name   string
		folder string
		want   string
	}{
		{"dot path", "/tmp/a.b/c", `projects={"/tmp/a.b/c"={trust_level="trusted"}}`},
		{"quoted dot path", `/tmp/a.b/"q"/dir`, `projects={"/tmp/a.b/\"q\"/dir"={trust_level="trusted"}}`},
		{"windows space dot path", `C:\Users\a.b\my proj`, `projects={"C:\\Users\\a.b\\my proj"={trust_level="trusted"}}`},
	} {
		t.Run("interactive "+tc.name, func(t *testing.T) {
			argv, _, err := drv.Interactive(Launch{Folder: tc.folder, Session: "thr-1", Level: 3, RepoConfig: true, Shim: []string{"solodawn"}})
			codexNoErr(t, err)
			codexCheck(t, slices.Contains(argv, tc.want), fmt.Sprintf("argv %q missing %q", argv, tc.want))
			codexCheck(t, !slices.Contains(argv, "--skip-git-repo-check"), "unexpected --skip-git-repo-check")
		})
	}

	emptyArgv, _, err := drv.Interactive(Launch{Folder: "", Session: "thr-1", Level: 3, RepoConfig: true, Shim: []string{"solodawn"}})
	codexNoErr(t, err)
	codexCheck(t, !slices.ContainsFunc(emptyArgv, func(a string) bool { return strings.HasPrefix(a, "projects=") }), "unexpected projects flag for empty folder")

	groupArgv, _, err := drv.Interactive(Launch{Folder: "/tmp/a.b/group", Session: "thr-1", Level: 3, RepoConfig: false, Shim: []string{"solodawn"}})
	codexNoErr(t, err)
	codexCheck(t, !slices.ContainsFunc(groupArgv, func(a string) bool { return strings.HasPrefix(a, "projects=") }), "unexpected projects flag for interactive group session")

	dotFolder := filepath.Join(t.TempDir(), "non.git", "work space.v1")
	codexNoErr(t, os.MkdirAll(dotFolder, 0o755))

	groupLaunch := codexTestLaunch(t, 1)
	groupLaunch.Folder = dotFolder
	groupLaunch.RepoConfig = false
	groupProc, groupPeer := startPeerCodex(t, groupLaunch)
	completePeerHandshake(t, groupPeer, "thr-group")
	codexCheck(t, !slices.ContainsFunc(groupPeer.header.Args, func(a string) bool { return strings.HasPrefix(a, "projects=") }), fmt.Sprintf("app-server group args %q carried projects trust", groupPeer.header.Args))
	codexEqual(t, nextCodexEvent(t, groupProc.Events()), Event{Kind: Ready, Session: "thr-group"})
	codexNoErr(t, groupProc.Close())
	groupPeer.expectEOF()
	groupPeer.conn.Close()
	waitEventsClosed(t, groupProc.Events())

	t.Setenv("NO_COLOR", "0")
	l := codexTestLaunch(t, 3)
	l.Folder = dotFolder
	l.RepoConfig = true
	proc, peer := startPeerCodex(t, l)
	completePeerHandshake(t, peer, "thr-dot")
	codexEqual(t, peer.header.NoColor, "1")
	wantTrust := "projects={" + marshalWire(dotFolder) + `={trust_level="trusted"}}`
	codexCheck(t, slices.Contains(peer.header.Args, wantTrust), fmt.Sprintf("app-server args %q missing %q", peer.header.Args, wantTrust))
	codexCheck(t, !slices.Contains(peer.header.Args, "--skip-git-repo-check"), "unexpected --skip-git-repo-check on app-server")
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "thr-dot"})
	codexNoErr(t, proc.Close())
	peer.expectEOF()
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func writeCodexScript(t *testing.T, steps string, exit int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-script.json")
	logPath := filepath.Join(t.TempDir(), "fakecli.log")
	body := fmt.Sprintf(`{"session":"thr-err","steps":%s,"exit":%d}`, steps, exit)
	codexNoErr(t, os.WriteFile(path, []byte(body), 0o600))
	t.Setenv("SOLODAWN_FAKECLI_LOG", logPath)
	t.Setenv("SOLODAWN_FAKE_CODEX_SCRIPT", path)
}

func TestCodexStartupStderrAndErrorNotificationsPreserved(t *testing.T) {
	t.Run("startup non-auth stderr", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[{"dir":"stderr","msg":"Error: unknown configuration field"}]`, 1)
		proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 3))
		codexNoErr(t, err)
		events := waitEventsClosed(t, proc.Events())
		codexEqual(t, events, []Event{{Kind: Active, Text: "Error: unknown configuration field"}})
		turnErr := proc.Turn("hello")
		codexWantErr(t, turnErr)
		codexEqual(t, turnErr.Error(), "Error: unknown configuration field")
	})

	t.Run("startup auth stderr", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[{"dir":"stderr","msg":"401 Unauthorized: login required"}]`, 1)
		proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 3))
		codexNoErr(t, err)
		events := waitEventsClosed(t, proc.Events())
		codexEqual(t, events, []Event{{Kind: LoginInvalid, Text: "401 Unauthorized: login required"}})
		turnErr := proc.Turn("hello")
		codexWantErr(t, turnErr)
		codexEqual(t, turnErr.Error(), "401 Unauthorized: login required")
	})

	t.Run("startup resume missing session stderr", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[{"dir":"stderr","msg":"no rollout found for thread id thr-old"}]`, 1)
		l := codexTestLaunch(t, 3)
		l.Session = "thr-old"
		proc, err := Drivers["codex"].Start(t.Context(), l)
		codexNoErr(t, err)
		events := waitEventsClosed(t, proc.Events())
		codexEqual(t, events, []Event{{Kind: NoSession, Text: "no rollout found for thread id thr-old"}})
		turnErr := proc.Turn("hello")
		codexWantErr(t, turnErr)
		codexEqual(t, turnErr.Error(), "no rollout found for thread id thr-old")
	})

	t.Run("startup silent exit", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[]`, 0)
		proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 3))
		codexNoErr(t, err)
		codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
		codexEqual(t, proc.Turn("hello"), errCodexClosed)
	})

	t.Run("error notification during active turn", func(t *testing.T) {
		proc, peer := startPeerCodex(t, codexTestLaunch(t, 3))
		completePeerHandshake(t, peer, "thr-note")
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "thr-note"})
		codexNoErr(t, proc.Turn("work"))
		turnReq := peer.readJSON()
		peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":"t1"}}}`, int(turnReq["id"].(float64))))
		peer.writeLine(`{"method":"error","params":"bad"}`)
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Active})
		peer.writeLine(`{"method":"error","params":{"error":{"message":"stream hiccup"}}}`)
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Active, Text: "stream hiccup"})
		peer.writeLine(`{"method":"turn/completed","params":{"turn":{"id":"t1","status":"completed","error":null}}}`)
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "completed"})
		codexNoErr(t, proc.Close())
		peer.expectEOF()
		peer.conn.Close()
		codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
	})

	t.Run("mid-turn crash falls back to error notification", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[
			{"dir":"stdin","msg":{"method":"initialize"}},
			{"dir":"stdout","msg":{"id":"$id","result":{}}},
			{"dir":"stdin","msg":{"method":"initialized"}},
			{"dir":"stdin","msg":{"method":"thread/start"}},
			{"dir":"stdout","msg":{"id":"$id","result":{"thread":{"id":"thr-err"}}}},
			{"dir":"stdin","msg":{"method":"turn/start"}},
			{"dir":"stdout","msg":{"id":"$id","result":{"turn":{"id":"t-1"}}}},
			{"dir":"stdout","msg":{"method":"error","params":{"error":{"message":"stream disconnected"}}}}
		]`, 1)
		proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 3))
		codexNoErr(t, err)
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "thr-err"})
		codexNoErr(t, proc.Turn("work"))
		codexEqual(t, waitEventsClosed(t, proc.Events()), []Event{
			{Kind: Active, Text: "stream disconnected"},
			{Kind: Active, Text: "stream disconnected"},
		})
	})

	t.Run("mid-turn crash preserves stderr", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[
			{"dir":"stdin","msg":{"method":"initialize"}},
			{"dir":"stdout","msg":{"id":"$id","result":{}}},
			{"dir":"stdin","msg":{"method":"initialized"}},
			{"dir":"stdin","msg":{"method":"thread/start"}},
			{"dir":"stdout","msg":{"id":"$id","result":{"thread":{"id":"thr-err"}}}},
			{"dir":"stdin","msg":{"method":"turn/start"}},
			{"dir":"stdout","msg":{"id":"$id","result":{"turn":{"id":"t-1"}}}},
			{"dir":"stderr","msg":"fatal worker stderr"}
		]`, 1)
		proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 3))
		codexNoErr(t, err)
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "thr-err"})
		codexNoErr(t, proc.Turn("work"))
		codexEqual(t, waitEventsClosed(t, proc.Events()), []Event{{Kind: Active, Text: "fatal worker stderr"}})
	})

	t.Run("earlier 401 stderr log does not override panic line", func(t *testing.T) {
		installFakeCodexBinary(t)
		writeCodexScript(t, `[
			{"dir":"stderr","msg":"ERROR codex_api::endpoint::responses_websocket: failed to connect to websocket: HTTP error: 401 Unauthorized"},
			{"dir":"stdin","msg":{"method":"initialize"}},
			{"dir":"stdout","msg":{"id":"$id","result":{}}},
			{"dir":"stdin","msg":{"method":"initialized"}},
			{"dir":"stdin","msg":{"method":"thread/start"}},
			{"dir":"stdout","msg":{"id":"$id","result":{"thread":{"id":"thr-err"}}}},
			{"dir":"stdin","msg":{"method":"turn/start"}},
			{"dir":"stdout","msg":{"id":"$id","result":{"turn":{"id":"t-1"}}}},
			{"dir":"stderr","msg":"thread 'main' panicked: out of memory"},
			{"dir":"stderr","msg":"   "}
		]`, 1)
		proc, err := Drivers["codex"].Start(t.Context(), codexTestLaunch(t, 3))
		codexNoErr(t, err)
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "thr-err"})
		codexNoErr(t, proc.Turn("work"))
		codexEqual(t, waitEventsClosed(t, proc.Events()), []Event{{Kind: Active, Text: "thread 'main' panicked: out of memory"}})
	})
}

func assertStartupEvents(t *testing.T, events []Event, wantKind EventKind) {
	t.Helper()
	if wantKind == "" {
		codexEqual(t, len(events), 0)
		return
	}
	codexEqual(t, len(events), 1)
	codexEqual(t, events[0].Kind, wantKind)
	codexCheck(t, events[0].Text != "", "expected non-empty error text")
}

func TestCodexContextCancelKillsProcessTree(t *testing.T) {
	installFakeCodexBinary(t)
	treeFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("SOLODAWN_TEST_CODEX_TREE", treeFile)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	proc, err := Drivers["codex"].Start(ctx, codexTestLaunch(t, 1))
	codexNoErr(t, err)
	pid := waitTreeChildPID(t, treeFile)

	cancel()
	waitEventsClosed(t, proc.Events())
	codexWantErr(t, proc.Turn("after-cancel"))
	waitCodexChildGone(t, pid)
}

func waitTreeChildPID(t *testing.T, treeFile string) int {
	t.Helper()
	var pid int
	poll(t, 5*time.Second, "the codex tree helper to record its child pid", func() string {
		raw, err := os.ReadFile(treeFile)
		if err != nil {
			return err.Error()
		}
		var convErr error
		if pid, convErr = strconv.Atoi(strings.TrimSpace(string(raw))); convErr != nil {
			return fmt.Sprintf("file %q", raw)
		}
		return ""
	})
	return pid
}

func waitCodexChildGone(t *testing.T, pid int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	defer func() {
		if proc, err := os.FindProcess(pid); err == nil && t.Failed() {
			proc.Kill()
		}
	}()
	poll(t, 2*time.Second, fmt.Sprintf("grandchild pid %d to end after context cancellation", pid), func() string {
		if proc, _ := os.FindProcess(pid); proc.Signal(syscall.Signal(0)) == nil {
			return "running"
		}
		return ""
	})
}
