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
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
}

func runTestCodexBridge(addr string) int {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return 1
	}
	defer conn.Close()
	cwd, _ := os.Getwd()
	data, _ := json.Marshal(codexBridgeHeader{Args: os.Args[1:], Dir: cwd})
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
		t.Fatal("timed out waiting for event")
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
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return rest
			}
			rest = append(rest, ev)
		case <-timeout:
			t.Fatal("timed out waiting for events channel to close")
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
		"-s", "workspace-write",
		"-a", "on-request",
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true",
		"-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"-c", "project_doc_max_bytes=0",
	})

	argv1, _, err := drv.Interactive(Launch{Folder: "/repo", Session: "thr-1", Level: 1, Shim: []string{"solodawn"}, RepoConfig: true})
	codexNoErr(t, err)
	codexCheck(t, slices.Contains(argv1, `mcp_servers.solodawn.args=[]`), "expected empty JSON args array")
	codexCheck(t, !slices.Contains(argv1, "project_doc_max_bytes=0"), "unexpected project_doc_max_bytes=0")
	codexEqual(t, argv1[slices.Index(argv1, "-s")+1], "read-only")
	codexEqual(t, argv1[slices.Index(argv1, "-a")+1], "never")

	argv3, _, err := drv.Interactive(Launch{Folder: "/repo", Session: "thr-3", Level: 3, Shim: []string{"solodawn"}, RepoConfig: true})
	codexNoErr(t, err)
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

func TestCodexWindowsCommandCancelUsesTaskkill(t *testing.T) {
	var attrs syscall.SysProcAttr
	cmd := codexCommand(t.Context(), t.TempDir(), []string{"app-server"}, true, reflect.ValueOf(&attrs).Elem())
	codexEqual(t, cmd.Args, []string{"codex", "app-server"})
	codexEqual(t, cmd.WaitDelay, time.Second)
	if runtime.GOOS == "windows" {
		return
	}
	checkTaskkillArgs(t, cmd)
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
	end := drainUntilTurnEnd(t, proc.Events())
	codexEqual(t, end, Event{
		Kind:   TurnEnded,
		Status: "completed",
		Usage:  Usage{Tokens: new(int64(35980)), WindowPct: new(float64(0)), WindowResets: new(int64(1795149937000))},
	})
	return ev.Ask.ID
}

func runApprovalTurnTwo(t *testing.T, proc Process, firstAskID string) {
	t.Helper()
	codexNoErr(t, proc.Turn("Approved. Retry."))
	ev, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, ev.Kind, Asked)
	codexCheck(t, ev.Ask.ID != firstAskID, "expected distinct Ask ID on retry")
	codexNoErr(t, proc.Answer(ev.Ask.ID, true))
	end := drainUntilTurnEnd(t, proc.Events())
	codexEqual(t, end.Status, "completed")
	codexEqual(t, *end.Usage.Tokens, int64(41562))
}

func drainUntilTurnEnd(t *testing.T, ch <-chan Event) Event {
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
	end := drainUntilTurnEnd(t, p2.Events())
	codexEqual(t, end.Status, "completed")
	codexEqual(t, *end.Usage.Tokens, int64(67311))
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
	_, hasDoc := cfg["project_doc_max_bytes"]
	codexEqual(t, hasSWW, tc.wantSWW)
	codexEqual(t, hasDoc, tc.wantDocZero)
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
	for i, tc := range []struct {
		method string
		want   map[string]any
	}{
		{"mcpServer/elicitation/request", map[string]any{"action": "decline"}},
		{"item/permissions/requestApproval", map[string]any{"permissions": map[string]any{}}},
		{"item/tool/requestUserInput", map[string]any{"answers": map[string]any{}}},
		{"item/tool/call", map[string]any{"contentItems": []any{}, "success": false}},
	} {
		peer.writeLine(fmt.Sprintf(`{"id":%d,"method":%q,"params":{}}`, 50+i, tc.method))
		reply := peer.readJSON()
		codexEqual(t, int(reply["id"].(float64)), 50+i)
		codexEqual(t, reply["result"], tc.want)
	}
	for i, line := range []string{
		`{"id":60,"method":"future/unknownRequest","params":{}}`,
		`{"id":61,"method":"item/commandExecution/requestApproval","params":"bad"}`,
		`{"id":62,"method":"item/fileChange/requestApproval","params":"bad"}`,
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
		codexEqual(t, reply["result"], map[string]any{"answers": map[string]any{}})
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

func TestCodexPermissionsApprovalRequests(t *testing.T) {
	proc, peer := startPeerCodex(t, codexTestLaunch(t, 2))
	completePeerHandshake(t, peer, "thr-perm")
	nextCodexEvent(t, proc.Events())

	codexNoErr(t, proc.Turn("Request extra permissions."))
	turnReq := peer.readJSON()
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":"turn-perm"}}}`, int(turnReq["id"].(float64))))

	verifyDeclinedPermissionsRequests(t, peer)
	verifyNetworkPermissionsApproval(t, proc, peer)
	verifyFilePermissionsApprovals(t, proc, peer)

	peer.writeLine(`{"method":"turn/completed","params":{"turn":{"id":"turn-perm","status":"completed","error":null}}}`)
	end, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, end, Event{Kind: TurnEnded, Status: "completed"})
	codexNoErr(t, proc.Close())
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func verifyDeclinedPermissionsRequests(t *testing.T, peer *codexPeer) {
	t.Helper()
	for i, params := range []string{
		`"bad"`,
		`{"permissions":{"network":{"enabled":true}},"reason":123}`,
		`{}`,
		`{"permissions":null}`,
		`{"permissions":["read"]}`,
		`{"permissions":"all"}`,
	} {
		peer.writeLine(fmt.Sprintf(`{"id":%d,"method":"item/permissions/requestApproval","params":%s}`, 300+i, params))
		reply := peer.readJSON()
		codexEqual(t, int(reply["id"].(float64)), 300+i)
		codexEqual(t, reply["result"], map[string]any{"permissions": map[string]any{}})
	}
}

func verifyNetworkPermissionsApproval(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	peer.writeLine(`{"id":"perm-net","method":"item/permissions/requestApproval","params":{"permissions":{ "network": { "enabled": true }, "fileSystem": null },"reason":"Fetch module"}}`)
	ev, _ := nextNonActiveEvent(t, proc.Events())
	wantCompact := `{"network":{"enabled":true},"fileSystem":null}`
	codexEqual(t, ev.Ask, &Ask{
		ID:       ev.Ask.ID,
		Tool:     "permissions",
		Category: "network",
		Command:  wantCompact,
		Why:      "Fetch module",
		Input:    json.RawMessage(wantCompact),
	})
	codexNoErr(t, proc.Answer(ev.Ask.ID, true))
	codexWantErr(t, proc.Answer(ev.Ask.ID, true))
	codexEqual(t, peer.readJSON(), map[string]any{
		"id": "perm-net",
		"result": map[string]any{
			"permissions": map[string]any{
				"network":    map[string]any{"enabled": true},
				"fileSystem": nil,
			},
		},
	})
}

func verifyFilePermissionsApprovals(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	peer.writeLine(`{"id":320,"method":"item/permissions/requestApproval","params":{"permissions":{ "fileSystem": { "write": ["/outside"] }, "network": null },"reason":"Write output"}}`)
	ev1, _ := nextNonActiveEvent(t, proc.Events())
	want1 := `{"fileSystem":{"write":["/outside"]},"network":null}`
	codexEqual(t, ev1.Ask, &Ask{
		ID:       ev1.Ask.ID,
		Tool:     "permissions",
		Category: "file",
		Command:  want1,
		Why:      "Write output",
		Input:    json.RawMessage(want1),
	})
	codexNoErr(t, proc.Answer(ev1.Ask.ID, false))
	codexEqual(t, peer.readJSON(), map[string]any{
		"id":     320.0,
		"result": map[string]any{"permissions": map[string]any{}},
	})

	peer.writeLine(`{"id":321,"method":"item/permissions/requestApproval","params":{"permissions":{ "fileSystem": { "read": ["/etc/hosts"] } },"reason":null}}`)
	ev2, _ := nextNonActiveEvent(t, proc.Events())
	want2 := `{"fileSystem":{"read":["/etc/hosts"]}}`
	codexEqual(t, ev2.Ask, &Ask{
		ID:       ev2.Ask.ID,
		Tool:     "permissions",
		Category: "file",
		Command:  want2,
		Input:    json.RawMessage(want2),
	})
	codexNoErr(t, proc.Answer(ev2.Ask.ID, true))
	codexEqual(t, peer.readJSON(), map[string]any{
		"id": 321.0,
		"result": map[string]any{
			"permissions": map[string]any{
				"fileSystem": map[string]any{"read": []any{"/etc/hosts"}},
			},
		},
	})
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

func TestCodexRateLimitsTokenBoundariesAndTurnErrors(t *testing.T) {
	proc, peer := startPeerCodex(t, codexTestLaunch(t, 1))
	completePeerHandshake(t, peer, "thr-lim")
	nextCodexEvent(t, proc.Events())

	exerciseCodexRateLimitWindows(t, proc, peer)
	exerciseCodexTurnErrors(t, proc, peer)

	codexNoErr(t, proc.Close())
	peer.conn.Close()
	waitEventsClosed(t, proc.Events())
}

func exerciseCodexRateLimitWindows(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	for _, ignored := range []string{
		"not-json",
		`{}`,
		`{"method":"thread/tokenUsage/updated","params":"bad"}`,
		`{"method":"account/rateLimits/updated","params":"bad"}`,
		`{"method":"turn/completed","params":"bad"}`,
		`{"method":"thread/tokenUsage/updated","params":{"turnId":"","tokenUsage":{"last":{"totalTokens":100}}}}`,
		`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":null,"secondary":null}}}`,
	} {
		peer.writeLine(ignored)
	}
	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":null,"secondary":{"usedPercent":25,"resetsAt":null}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(float64(25))}})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":50,"resetsAt":100},"secondary":{"usedPercent":50,"resetsAt":200}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(float64(50)), WindowResets: new(int64(100000))}})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":40,"resetsAt":100},"secondary":{"usedPercent":99,"resetsAt":300}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(float64(99)), WindowResets: new(int64(300000))}})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":100,"resetsAt":400},"secondary":{"usedPercent":40,"resetsAt":300}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(float64(100)), WindowResets: new(int64(400000))}})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: RateLimited, Resets: 400000})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":10,"resetsAt":500},"rateLimitReachedType":"five_hour"}}}`)
	nextCodexEvent(t, proc.Events())
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: RateLimited, Resets: 500000})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":10,"resetsAt":null},"spendControlReached":true}}}`)
	nextCodexEvent(t, proc.Events())
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: RateLimited, Resets: 0})

	peer.writeLine(`{"method":"account/rateLimits/updated","params":{"rateLimits":{"primary":{"usedPercent":15,"resetsAt":777}}}}`)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Used, Usage: Usage{WindowPct: new(float64(15)), WindowResets: new(int64(777000))}})
}

func exerciseCodexTurnErrors(t *testing.T, proc Process, peer *codexPeer) {
	t.Helper()
	for i, tc := range []struct {
		info   string
		wantEv Event
	}{
		{`"unauthorized"`, Event{Kind: LoginInvalid, Text: "err-msg"}},
		{`"usageLimitExceeded"`, Event{Kind: RateLimited, Resets: 777000}},
		{`"otherError"`, Event{}},
		{`{"stream":{"httpStatusCode":401}}`, Event{Kind: LoginInvalid, Text: "err-msg"}},
		{`{"stream":{"httpStatusCode":400}}`, Event{}},
		{`{"stream":{"httpStatusCode":402}}`, Event{}},
		{`{}`, Event{}},
		{`[1]`, Event{}},
	} {
		runFailedTurnErrorCase(t, proc, peer, i, tc.info, tc.wantEv)
	}
	codexNoErr(t, proc.Turn("rpc-fail"))
	req := peer.readJSON()
	peer.writeLine(`{"id":999,"error":{"code":-32000,"message":"unrelated"}}`)
	peer.writeLine(fmt.Sprintf(`{"id":%d,"error":{"code":-32000,"message":"turn rejected"}}`, int(req["id"].(float64))))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "failed", Text: "turn rejected"})
}

func runFailedTurnErrorCase(t *testing.T, proc Process, peer *codexPeer, idx int, info string, wantEv Event) {
	t.Helper()
	codexNoErr(t, proc.Turn(fmt.Sprintf("turn-%d", idx)))
	req := peer.readJSON()
	turnID := fmt.Sprintf("tid-%d", idx)
	peer.writeLine(fmt.Sprintf(`{"id":%d,"result":{"turn":{"id":%q}}}`, int(req["id"].(float64)), turnID))
	peer.writeLine(`{"method":"thread/tokenUsage/updated","params":{"turnId":"wrong","tokenUsage":{"last":{"totalTokens":999}}}}`)
	peer.writeLine(fmt.Sprintf(`{"method":"thread/tokenUsage/updated","params":{"turnId":%q,"tokenUsage":{"last":{"totalTokens":10}}}}`, turnID))
	used, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, *used.Usage.Tokens, int64(10))
	peer.writeLine(fmt.Sprintf(`{"method":"turn/completed","params":{"turn":{"id":%q,"status":"failed","error":{"message":"err-msg","codexErrorInfo":%s}}}}`, turnID, info))
	if wantEv.Kind != "" {
		ev, _ := nextNonActiveEvent(t, proc.Events())
		codexEqual(t, ev, wantEv)
	}
	end, _ := nextNonActiveEvent(t, proc.Events())
	codexEqual(t, end.Kind, TurnEnded)
	codexEqual(t, end.Status, "failed")
	codexEqual(t, end.Text, "err-msg")
	codexEqual(t, *end.Usage.Tokens, int64(10))
}

type codexStartupFailureCase struct {
	name     string
	session  string
	failID   int
	reply    string
	wantKind EventKind
}

func TestCodexStartupFailuresNoSessionAndLoginInvalid(t *testing.T) {
	for _, tc := range []codexStartupFailureCase{
		{"resume missing thread", "thr-missing", 2, `{"id":2,"error":{"code":-32600,"message":"no rollout found for thread id thr-missing"}}`, NoSession},
		{"resume init auth error", "thr-missing", 1, `{"id":1,"error":{"code":-32000,"message":"Not Authenticated"}}`, LoginInvalid},
		{"start login error", "", 2, `{"id":2,"error":{"code":-32000,"message":"Login required"}}`, LoginInvalid},
		{"start non-auth error", "", 2, `{"id":2,"error":{"code":-32000,"message":"invalid configuration"}}`, ""},
		{"malformed thread result", "", 2, `{"id":2,"result":"bad"}`, ""},
		{"empty thread id", "", 2, `{"id":2,"result":{"thread":{}}}`, ""},
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
	codexWantErr(t, proc.Turn("hello"))
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
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(treeFile)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("codex tree helper did not record child pid")
	return 0
}

func waitCodexChildGone(t *testing.T, pid int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		proc, _ := os.FindProcess(pid)
		if proc.Signal(syscall.Signal(0)) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
	t.Fatalf("grandchild pid %d survived context cancellation", pid)
}
