package fakecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The configured MCP child bridges stdio to the test, which decides exactly
// when to reply. No vendor, model, shell or production node runs.
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

type pipe struct {
	in  io.Writer
	out *json.Decoder
}

func (p pipe) send(t *testing.T, line string) {
	t.Helper()
	if _, err := fmt.Fprintln(p.in, line); err != nil {
		t.Fatal(err)
	}
}

// Each call consumes exactly the next message, not the next matching one.
func (p pipe) next(t *testing.T, pairs ...any) {
	t.Helper()
	var obj map[string]any
	if err := p.out.Decode(&obj); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(pairs); i += 2 {
		var value any = obj
		for _, part := range strings.Split(pairs[i].(string), ".") {
			value = value.(map[string]any)[part]
		}
		if !reflect.DeepEqual(value, pairs[i+1]) {
			t.Fatalf("%s: got %#v, want %#v", pairs[i], value, pairs[i+1])
		}
	}
}

type cli struct {
	pipe
	cmd      *exec.Cmd
	stdin    io.Closer
	stderr   bytes.Buffer
	listener *net.TCPListener
	token    string
}

func start(t *testing.T, vendor, script, token string, args ...string) *cli {
	t.Helper()
	l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	program, _ := os.Executable()
	srv := server{program, []string{"mcp", "--token", token}}
	if vendor == "claude" {
		path := filepath.Join(t.TempDir(), "mcp.json")
		data, _ := json.Marshal(map[string]any{"mcpServers": map[string]server{"solodawn": srv}})
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--mcp-config", path)
	} else {
		command, _ := json.Marshal(program)
		argv, _ := json.Marshal(srv.Args)
		args = append(args, "-c", "mcp_servers.solodawn.command="+string(command), "-c", "mcp_servers.solodawn.args="+string(argv))
	}
	c := &cli{cmd: exec.CommandContext(ctx, vendor, args...), listener: l, token: token}
	c.cmd.Env = append(os.Environ(), "SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT="+script, "TEST_MCP_PEER="+l.Addr().String())
	c.cmd.Stderr = &c.stderr
	input, err := c.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	c.stdin, c.in = input, input
	output, err := c.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	c.out = json.NewDecoder(output)
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c.cmd.ProcessState == nil {
			input.Close()
			cancel()
			c.cmd.Wait()
		}
		if t.Failed() {
			t.Log(c.stderr.String())
		}
	})
	return c
}

func (c *cli) mcp(t *testing.T, version string) pipe {
	t.Helper()
	c.listener.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := c.listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	want := "mcp --token " + c.token + "\n"
	line := make([]byte, len(want))
	if _, err := io.ReadFull(conn, line); err != nil || string(line) != want {
		t.Fatalf("MCP argv %q: %v", line, err)
	}
	p := pipe{conn, json.NewDecoder(conn)}
	p.next(t, "method", "initialize", "id", 0.0, "params.protocolVersion", version)
	p.send(t, `{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"`+version+`","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1"}}}`)
	p.next(t, "method", "notifications/initialized")
	p.next(t, "method", "tools/list", "id", 1.0)
	p.send(t, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)
	return p
}

func (c *cli) end(t *testing.T, code int) {
	t.Helper()
	var extra json.RawMessage
	if err := c.out.Decode(&extra); err != io.EOF {
		t.Fatalf("unexpected output %s (%v)", extra, err)
	}
	c.cmd.Wait()
	if got := c.cmd.ProcessState.ExitCode(); got != code {
		t.Fatalf("exit %d, want %d: %s", got, code, c.stderr.String())
	}
}

func (p pipe) result(t *testing.T, id int, text string) {
	b, _ := json.Marshal(text)
	p.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":%s}],"isError":false}}`, id, b))
}

func TestVendorConversations(t *testing.T) {
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", bin+string(os.PathSeparator), "./claude", "./codex").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(bin, "lifecycle"))
	t.Run("Claude approval and resume", claudeTurns)
	t.Run("Codex approval", codexTurns)
	t.Run("overlapping sessions and mismatched decisions", wrongDecision)
	t.Run("decision after process exit", exitDuringApproval)
	t.Run("late reply after tool timeout", lateReply)
}

const claudeID = "7dc7f589-d9e4-45ba-9974-b043cf19969a"
const turn = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"try the write"}]}}`

func claudeTurns(t *testing.T) {
	c := start(t, "claude", "testdata/claude-approval.json", "tok-first", "-p", "--session-id", claudeID)
	c.send(t, turn)
	m := c.mcp(t, "2025-11-25")
	c.next(t, "type", "system", "subtype", "init", "session_id", claudeID, "apiKeySource", "none")
	for i, id := range []string{"toolu_011Z6XgToqHgh6BUPTAbQ1VW", "toolu_01Pdq3rAZ9sYnEtyVMcfR6Aq"} {
		c.next(t, "type", "assistant", "session_id", claudeID)
		m.next(t, "method", "tools/call", "id", float64(i+2), "params.name", "approve", "params.arguments.tool_use_id", id, "params.arguments.input.content", "retry-test-1")
		switch i {
		case 0:
			m.result(t, 2, `{"message":"No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided.","behavior":"deny"}`)
			c.next(t, "type", "rate_limit_event", "rate_limit_info.status", "allowed", "rate_limit_info.resetsAt", 1795615200.0)
			c.next(t, "type", "user", "uuid", "d5c7add1-6edd-482d-9af8-bb30e95eeff9")
			c.next(t, "type", "result", "session_id", claudeID, "usage.output_tokens", 741.0)
			c.send(t, `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Approved. Retry."}]}}`)
		case 1:
			m.result(t, 3, `{"behavior":"allow"}`)
			c.next(t, "type", "user", "tool_use_result.type", "create")
			c.next(t, "type", "result", "session_id", claudeID, "usage.output_tokens", 260.0)
		}
	}
	c.stdin.Close()
	c.end(t, 0)
	c = start(t, "claude", "testdata/claude-resume.json", "tok-resumed", "-p", "--resume", claudeID)
	c.send(t, turn)
	m = c.mcp(t, "2025-11-25")
	c.next(t, "type", "system", "session_id", claudeID)
	c.next(t, "type", "assistant", "uuid", "7ac36541-1e0c-47c0-b175-f5377bdef4fa")
	m.next(t, "method", "tools/call", "params.name", "whoami", "id", 2.0)
	m.result(t, 2, `{"token": "tok-resumed"}`)
	c.next(t, "type", "user", "session_id", claudeID)
	c.next(t, "type", "result", "session_id", claudeID, "usage.output_tokens", 173.0)
	c.stdin.Close()
	c.end(t, 0)
}

func wrongDecision(t *testing.T) {
	log := filepath.Join(t.TempDir(), "overlap")
	t.Setenv("SOLODAWN_FAKECLI_LOG", log)
	a := start(t, "claude", "testdata/claude-approval.json", "tok-first", "-p", "--session-id", claudeID)
	a.send(t, turn)
	m := a.mcp(t, "2025-11-25")
	a.next(t, "type", "system", "session_id", claudeID)
	a.next(t, "type", "assistant")
	m.next(t, "id", 2.0, "params.name", "approve")
	b := start(t, "codex", "testdata/codex-approval.json", "tok-first", "app-server")
	firstApproval(t, b)
	starts := "start claude " + claudeID + "\nstart codex " + thread + "\n"
	check := func(want string) {
		t.Helper()
		got, err := os.ReadFile(log)
		if err != nil || string(got) != want {
			t.Fatalf("log %q, want %q (%v)", got, want, err)
		}
	}
	check(starts)
	// Neither the wrong decision nor the right decision on a different
	// request may release a scripted result.
	m.result(t, 2, `{"behavior": "allow"}`)
	a.end(t, 1)
	b.send(t, `{"id":1,"result":{"decision":"decline"}}`)
	b.end(t, 1)
	check(starts + "exit claude " + claudeID + "\nexit codex " + thread + "\n")
}
