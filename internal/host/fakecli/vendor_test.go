package fakecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The MCP server a fake starts is this test binary. It hands its stdio to the
// test, which then decides when each reply is sent.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		c, err := net.Dial("tcp", os.Getenv("TEST_MCP_PEER"))
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(c, strings.Join(os.Args[1:], " "))
		go io.Copy(c, os.Stdin)
		io.Copy(os.Stdout, c)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type wire struct {
	in  io.Writer
	out *json.Decoder
}

func (w wire) send(t *testing.T, line string) {
	t.Helper()
	if _, err := fmt.Fprintln(w.in, line); err != nil {
		t.Fatal(err)
	}
}

// A message is named by its method, or by its type and subtype, or as a result.
func kind(msg map[string]any) string {
	name := "result"
	for _, key := range []string{"type", "method"} {
		if value, ok := msg[key].(string); ok {
			name = value
		}
	}
	if subtype, ok := msg["subtype"].(string); ok {
		name += "/" + subtype
	}
	return name
}

// read takes exactly the next message, not the next one that matches.
func (w wire) read(t *testing.T, name string, pairs ...any) {
	t.Helper()
	var msg map[string]any
	if err := w.out.Decode(&msg); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if got := kind(msg); got != name {
		t.Fatalf("got %s, want %s", got, name)
	}
	for i := 0; i < len(pairs); i += 2 {
		var value any = msg
		for _, part := range strings.Split(pairs[i].(string), ".") {
			if list, ok := value.([]any); ok {
				index, _ := strconv.Atoi(part)
				value = list[index]
			} else {
				value = value.(map[string]any)[part]
			}
		}
		if !reflect.DeepEqual(value, pairs[i+1]) {
			t.Fatalf("%s %s: got %#v, want %#v", name, pairs[i], value, pairs[i+1])
		}
	}
}

func (w wire) reads(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		w.read(t, name)
	}
}

func (w wire) result(t *testing.T, id int, text string) {
	t.Helper()
	quoted, _ := json.Marshal(text)
	w.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":%s}],"isError":false}}`, id, quoted))
}

type fake struct {
	wire
	stdin  *os.File
	stderr bytes.Buffer
	exit   chan int
	peer   *net.TCPListener
	token  string
}

// start runs one fake in this process on real pipes, with this test binary as
// the MCP server of its configuration.
func start(t *testing.T, vendor, script, token string, args ...string) *fake {
	t.Helper()
	peer, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_MCP_PEER", peer.Addr().String())
	t.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", script)
	program, _ := os.Executable()
	srv := server{program, []string{"mcp", "--token", token}}
	if vendor == "claude" {
		data, _ := json.Marshal(map[string]any{"mcpServers": map[string]server{"solodawn": srv}})
		args = append(args, "--mcp-config", string(data))
	} else {
		command, _ := json.Marshal(srv.Command)
		list, _ := json.Marshal(srv.Args)
		args = append(args, "-c", "mcp_servers.solodawn.command="+string(command), "-c", "mcp_servers.solodawn.args="+string(list))
	}
	stdin, in, _ := os.Pipe()
	out, stdout, _ := os.Pipe()
	f := &fake{wire: wire{in, json.NewDecoder(out)}, stdin: in, exit: make(chan int, 1), peer: peer, token: token}
	go func() {
		code := Run(vendor, "unused", args, stdin, stdout, &f.stderr)
		stdin.Close()
		stdout.Close()
		f.exit <- code
	}()
	t.Cleanup(func() {
		in.Close()
		peer.Close()
	})
	return f
}

// mcp accepts the server the fake started and checks how it was started.
func (f *fake) mcp(t *testing.T) wire {
	t.Helper()
	f.peer.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := f.peer.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	want := "mcp --token " + f.token + "\n"
	line := make([]byte, len(want))
	if _, err := io.ReadFull(conn, line); err != nil || string(line) != want {
		t.Fatalf("MCP argv %q: %v", line, err)
	}
	return wire{conn, json.NewDecoder(conn)}
}

// end checks that nothing else was printed, the exit status and stderr.
func (f *fake) end(t *testing.T, code int, stderr string) {
	t.Helper()
	var extra json.RawMessage
	if err := f.out.Decode(&extra); err != io.EOF {
		t.Fatalf("unexpected output %s (%v)", extra, err)
	}
	if got := <-f.exit; got != code || f.stderr.String() != stderr {
		t.Fatalf("exit %d, want %d; stderr %q, want %q", got, code, f.stderr.String(), stderr)
	}
}

func lifecycle(t *testing.T) func(want ...string) {
	path := filepath.Join(t.TempDir(), "lifecycle")
	t.Setenv("SOLODAWN_FAKECLI_LOG", path)
	return func(want ...string) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil || string(got) != strings.Join(want, "\n")+"\n" {
			t.Fatalf("log %q, want %q (%v)", got, want, err)
		}
	}
}

func times(n int, names ...string) []string { return slices.Repeat(names, n) }

const (
	deny   = `{"behavior": "deny", "message": "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."}`
	prompt = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"try the write"}]}}`
)

// Claude Code probes the server, then lists its tools, before its first event.
func (f *fake) claudeMCP(t *testing.T) wire {
	t.Helper()
	m := f.mcp(t)
	m.read(t, "server/discover", "id", "server-discover-probe-1")
	m.send(t, `{"jsonrpc":"2.0","id":"server-discover-probe-1","error":{"code":-32601,"message":"method not found"}}`)
	m.read(t, "initialize", "id", 0.0, "params.protocolVersion", "2025-11-25")
	m.send(t, `{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-11-25","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1"}}}`)
	m.read(t, "notifications/initialized")
	m.read(t, "tools/list", "id", 1.0)
	m.send(t, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	return m
}

func TestClaudeIsAskedTwiceAndResumedWithTheSameSession(t *testing.T) {
	log := lifecycle(t)
	c := start(t, "claude", "testdata/claude-approval.json", "tok-first", "-p", "--session-id", session)
	c.send(t, prompt)
	m := c.claudeMCP(t)
	c.read(t, "system/init", "session_id", session, "apiKeySource", "none")
	c.read(t, "assistant", "session_id", session, "message.content.0.id", "toolu_011Z6XgToqHgh6BUPTAbQ1VW")
	m.read(t, "tools/call", "id", 2.0, "params.name", "approve", "params.arguments.tool_use_id", "toolu_011Z6XgToqHgh6BUPTAbQ1VW", "params.arguments.input.content", "retry-test-1")
	m.result(t, 2, deny)
	c.read(t, "rate_limit_event", "rate_limit_info.status", "allowed", "rate_limit_info.resetsAt", 1795615200.0, "rate_limit_info.rateLimitType", "seven_day")
	c.read(t, "user", "uuid", "d5c7add1-6edd-482d-9af8-bb30e95eeff9", "message.content.0.is_error", true)
	c.reads(t, "system/thinking_tokens", "system/thinking_tokens", "system/thinking_tokens", "assistant", "assistant")
	c.read(t, "result/success", "session_id", session, "usage.output_tokens", 741.0)
	log("start claude " + session)

	c.send(t, `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Approved. Run exactly the same request again."}]}}`)
	c.read(t, "system/init", "session_id", session)
	c.read(t, "assistant", "session_id", session, "message.content.0.id", "toolu_01Pdq3rAZ9sYnEtyVMcfR6Aq")
	m.read(t, "tools/call", "id", 3.0, "params.name", "approve", "params.arguments.tool_use_id", "toolu_01Pdq3rAZ9sYnEtyVMcfR6Aq", "params.arguments.input.content", "retry-test-1")
	m.result(t, 3, `{"behavior": "allow"}`)
	c.read(t, "user", "tool_use_result.type", "create")
	c.reads(t, "assistant")
	c.read(t, "result/success", "session_id", session, "usage.output_tokens", 260.0)
	c.stdin.Close()
	c.end(t, 0, "")
	log("start claude "+session, "exit claude "+session)

	c = start(t, "claude", "testdata/claude-resume.json", "tok-resumed", "-p", "--resume", session)
	c.send(t, prompt)
	m = c.claudeMCP(t)
	c.read(t, "system/init", "session_id", session)
	c.reads(t, "assistant", "assistant", "user")
	c.read(t, "rate_limit_event", "rate_limit_info.status", "allowed")
	c.read(t, "assistant", "uuid", "7ac36541-1e0c-47c0-b175-f5377bdef4fa", "session_id", session)
	m.read(t, "tools/call", "id", 2.0, "params.name", "whoami")
	m.result(t, 2, `{"token": "tok-resumed"}`)
	c.read(t, "user", "session_id", session, "message.content.0.content.0.text", `{"token": "tok-resumed"}`)
	c.reads(t, "assistant")
	c.read(t, "result/success", "session_id", session, "usage.output_tokens", 173.0)
	c.stdin.Close()
	c.end(t, 0, "")
	log("start claude "+session, "exit claude "+session, "start claude "+session, "exit claude "+session)
}

func TestClaudeStopsWhenTheDecisionIsNotTheScripted(t *testing.T) {
	log := lifecycle(t)
	c := start(t, "claude", "testdata/claude-approval.json", "tok-first", "-p", "--session-id", session)
	c.send(t, prompt)
	m := c.claudeMCP(t)
	c.reads(t, "system/init", "assistant")
	m.read(t, "tools/call", "id", 2.0, "params.name", "approve")
	m.result(t, 2, `{"behavior": "allow"}`)
	quoted, _ := json.Marshal(deny)
	c.end(t, 1, `fake claude: step 12 (mcp->cli): expected {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":`+string(quoted)+
		`}],"isError":false}}, got {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"{\"behavior\": \"allow\"}"}],"isError":false}}`+"\n")
	log("start claude "+session, "exit claude "+session)
}

const (
	thread      = "01a1f63f-701c-78c1-bbf3-f1ac61c56b59"
	firstTurn   = "01a1f63f-7034-7c60-a028-83d6e6401838"
	secondTurn  = "01a1f63f-965e-7fc2-9513-6460299fd68d"
	resumedTurn = "01a1f63f-cec1-7370-9b17-5a693c5b6580"
	status      = "mcpServer/startupStatus/updated"
	usage       = "thread/tokenUsage/updated"
	limits      = "account/rateLimits/updated"
	rejected    = "\x1b[2m2026-11-19T23:58:43.620265Z\x1b[0m \x1b[31mERROR\x1b[0m \x1b[2mcodex_core::tools::router\x1b[0m\x1b[2m:\x1b[0m \x1b[3merror\x1b[0m\x1b[2m=\x1b[0mexec_command failed: CreateProcess { message: \"Rejected(\\\"rejected by user\\\")\" }\n"
)

func (f *fake) initialize(t *testing.T) {
	t.Helper()
	f.send(t, `{"method":"initialize","id":"hello-42","params":{"clientInfo":{"name":"test","version":"0"}}}`)
	f.read(t, "result", "id", "hello-42", "result.platformOs", "macos")
	f.send(t, `{"method":"initialized"}`)
	f.reads(t, "remoteControl/status/changed")
}

func (f *fake) turn(t *testing.T, id, text string) {
	t.Helper()
	f.send(t, fmt.Sprintf(`{"method":"turn/start","id":42,"params":{"threadId":%q,"input":[{"type":"text","text":%q}]}}`, id, text))
}

func (w wire) codexMCP(t *testing.T) {
	t.Helper()
	w.read(t, "initialize", "id", 0.0, "params.protocolVersion", "2025-06-18")
	w.send(t, `{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1"}}}`)
	w.read(t, "notifications/initialized")
	w.read(t, "tools/list", "id", 1.0)
	w.send(t, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
}

// The first turn of the thread, up to its request to run a command.
func firstApproval(t *testing.T, script string) *fake {
	t.Helper()
	c := start(t, "codex", script, "tok-first", "app-server")
	c.initialize(t)
	c.send(t, `{"method":"thread/start","id":12,"params":{"cwd":"test"}}`)
	c.read(t, "result", "id", 12.0, "result.thread.id", thread)
	c.read(t, "thread/started", "params.thread.sessionId", thread)
	c.turn(t, thread, "Try the write.")
	c.reads(t, status, status, status)
	c.read(t, "result", "id", 42.0, "result.turn.id", firstTurn)
	c.mcp(t).codexMCP(t)
	c.reads(t, status, "thread/status/changed")
	c.read(t, "turn/started", "params.turn.id", firstTurn)
	c.reads(t, status)
	c.read(t, "account/updated", "params.authMode", "chatgpt", "params.planType", "plus")
	c.reads(t, status, status, status, "item/started", "item/completed", "item/started", "item/completed")
	c.read(t, "thread/status/changed", "params.status.activeFlags.0", "waitingOnApproval")
	c.read(t, "item/started", "params.item.type", "commandExecution", "params.item.status", "inProgress")
	c.read(t, "item/commandExecution/requestApproval", "id", 0.0, "params.turnId", firstTurn, "params.threadId", thread)
	return c
}

func TestCodexIsDeclinedThenAccepted(t *testing.T) {
	log := lifecycle(t)
	c := firstApproval(t, "testdata/codex-approval.json")
	c.send(t, `{"id":0,"result":{"decision":"decline"}}`)
	c.read(t, "serverRequest/resolved", "params.requestId", 0.0)
	c.reads(t, "thread/status/changed")
	c.read(t, "item/completed", "params.item.type", "commandExecution", "params.item.status", "declined")
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 17960.0, "params.turnId", firstTurn)
	c.read(t, limits, "params.rateLimits.primary.windowDurationMins", 300.0, "params.rateLimits.primary.usedPercent", 0.0)
	c.reads(t, "item/started", "item/completed")
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 35980.0)
	c.reads(t, limits, "thread/status/changed")
	c.read(t, "turn/completed", "params.turn.id", firstTurn, "params.turn.status", "completed")

	c.turn(t, thread, "Approved. Retry.")
	c.read(t, "result", "id", 42.0, "result.turn.id", secondTurn)
	c.reads(t, "thread/status/changed")
	c.read(t, "turn/started", "params.turn.id", secondTurn)
	c.reads(t, "item/started", "item/completed", "item/started", "item/completed", "thread/status/changed")
	c.read(t, "item/started", "params.item.type", "commandExecution")
	c.read(t, "item/commandExecution/requestApproval", "id", 1.0, "params.turnId", secondTurn, "params.threadId", thread)
	c.send(t, `{"id":1,"result":{"decision":"accept"}}`)
	c.read(t, "serverRequest/resolved", "params.requestId", 1.0)
	c.reads(t, "thread/status/changed")
	c.read(t, "item/completed", "params.item.status", "completed", "params.item.exitCode", 0.0)
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 56740.0)
	c.reads(t, limits, "item/started", "item/completed")
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 77542.0)
	c.reads(t, limits, "thread/status/changed")
	c.read(t, "turn/completed", "params.turn.id", secondTurn, "params.turn.status", "completed")
	c.stdin.Close()
	c.end(t, 0, rejected)
	log("start codex "+thread, "exit codex "+thread)
}

func TestCodexStopsWhenTheDecisionIsNotTheScripted(t *testing.T) {
	log := lifecycle(t)
	c := firstApproval(t, "testdata/codex-approval.json")
	c.send(t, `{"id":0,"result":{"decision":"accept"}}`)
	c.end(t, 1, `fake codex: step 33 (stdin): expected {"id":0,"result":{"decision":"decline"}}, got {"id":0,"result":{"decision":"accept"}}`+"\n")
	log("start codex "+thread, "exit codex "+thread)
}

func TestCodexExitsInsideATurnAndIsResumedInANewProcess(t *testing.T) {
	log := lifecycle(t)
	c := firstApproval(t, "testdata/codex-exit-mid-turn.json")
	c.end(t, 0, "")
	log("start codex "+thread, "exit codex "+thread)
	if _, err := fmt.Fprintln(c.in, `{"id":0,"result":{"decision":"accept"}}`); err == nil {
		t.Fatal("a process that exited took a decision")
	}

	c = start(t, "codex", "testdata/codex-resume.json", "tok-resumed", "app-server")
	c.initialize(t)
	c.send(t, `{"method":"thread/resume","id":24,"params":{"threadId":"`+thread+`"}}`)
	m := c.mcp(t)
	m.codexMCP(t)
	c.reads(t, status, status, status, status, "thread/status/changed")
	c.read(t, "result", "id", 24.0, "result.thread.id", thread)
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 98471.0)
	c.turn(t, thread, "The owner decided after the exit. Check the token.")
	c.reads(t, "thread/goal/cleared")
	c.read(t, "result", "id", 42.0, "result.turn.id", resumedTurn)
	c.reads(t, "thread/status/changed")
	c.read(t, "turn/started", "params.turn.id", resumedTurn, "params.threadId", thread)
	c.reads(t, status)
	c.read(t, "account/updated", "params.authMode", "chatgpt")
	c.reads(t, status, status, status, "item/started", "item/completed", usage, limits)
	c.read(t, "item/started", "params.item.type", "mcpToolCall", "params.item.tool", "whoami")
	m.read(t, "tools/call", "id", 2.0, "params.name", "whoami", "params._meta.threadId", thread, "params._meta.x-codex-turn-metadata.turn_id", resumedTurn)
	m.result(t, 2, `{"token": "tok-resumed"}`)
	c.read(t, "item/completed", "params.item.status", "completed", "params.item.result.content.0.text", `{"token": "tok-resumed"}`)
	c.reads(t, usage, limits, "item/started", "item/completed")
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 165782.0)
	c.reads(t, limits, "thread/status/changed")
	c.read(t, "turn/completed", "params.turn.id", resumedTurn, "params.threadId", thread)
	c.stdin.Close()
	c.end(t, 0, "")
	log("start codex "+thread, "exit codex "+thread, "start codex "+thread, "exit codex "+thread)
}

func TestCodexGivesUpACallBeforeItsReplyArrives(t *testing.T) {
	const thread = "01a1f63e-dcce-7a02-a6cc-fc0d4c34bc1c"
	const turn = "01a1f63e-dcec-7621-b000-b2f5ffc70b34"
	log := lifecycle(t)
	c := start(t, "codex", "testdata/codex-late-call.json", "tok-checks", "app-server")
	c.send(t, `{"method":"initialize","id":1,"params":{"clientInfo":{"name":"test","version":"0"}}}`)
	c.read(t, "result", "id", 1.0)
	c.reads(t, "remoteControl/status/changed")
	c.send(t, `{"method":"initialized"}`)
	c.send(t, `{"method":"thread/start","id":3,"params":{}}`)
	c.read(t, "result", "id", 3.0, "result.thread.id", thread)
	c.reads(t, status)
	c.turn(t, thread, "Run checks.")
	c.read(t, "thread/started", "params.thread.id", thread)
	c.reads(t, status, status)
	c.read(t, "result", "id", 42.0, "result.turn.id", turn)
	m := c.mcp(t)
	m.codexMCP(t)
	c.reads(t, status, "thread/status/changed")
	c.read(t, "turn/started", "params.turn.id", turn)
	c.reads(t, status, "account/updated", status, status, status, "item/started", "item/completed", usage, limits)
	c.read(t, "item/started", "params.item.tool", "run_checks", "params.item.status", "inProgress")
	m.read(t, "tools/call", "id", 2.0, "params.name", "run_checks", "params._meta.threadId", thread)
	c.reads(t, times(9, usage, limits)...)
	c.read(t, "item/completed", "params.item.status", "failed", "params.item.durationMs", 300005.0)
	c.read(t, usage, "params.tokenUsage.total.totalTokens", 200547.0)
	c.reads(t, limits, "item/started", "item/completed", usage, limits, "thread/status/changed")
	c.read(t, "turn/completed", "params.turn.id", turn, "params.turn.status", "completed")
	m.result(t, 2, `{"ok": true, "commit": "0000000", "items": []}`)
	c.stdin.Close()
	c.end(t, 0, "")
	log("start codex "+thread, "exit codex "+thread)
}
