package fakecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const qwenTestSession = "550e8400-e29b-41d4-a716-446655440091"

type qwenTestRun struct {
	baseURL string
	token   string
	stdin   *os.File
	stderr  bytes.Buffer
	exit    chan int
	peer    *net.TCPListener
	mcpTok  string
}

func startQwenFake(t *testing.T, ctx context.Context, scriptPath, mcpTok string, extraArgs ...string) *qwenTestRun {
	t.Helper()
	peer, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_MCP_PEER", peer.Addr().String())
	t.Setenv("SOLODAWN_FAKE_QWEN_SCRIPT", scriptPath)
	prog, _ := os.Executable()
	settings := filepath.Join(t.TempDir(), "settings.json")
	cfg, _ := json.Marshal(map[string]any{
		"mcpServers": map[string]server{"solodawn": {Command: prog, Args: []string{"mcp", "--token", mcpTok}}},
	})
	_ = os.WriteFile(settings, cfg, 0o600)
	t.Setenv("QWEN_CODE_SYSTEM_SETTINGS_PATH", settings)

	inR, inW, _ := os.Pipe()
	outR, outW, _ := os.Pipe()
	args := append([]string{"serve", "--token", "bearer-tok", "--workspace", "/tmp/ws"}, extraArgs...)
	r := &qwenTestRun{token: "bearer-tok", stdin: inW, exit: make(chan int, 1), peer: peer, mcpTok: mcpTok}
	go func() {
		code := RunQwen(ctx, "0.28.0", args, inR, outW, &r.stderr)
		_ = inR.Close()
		_ = outW.Close()
		r.exit <- code
	}()
	t.Cleanup(func() {
		_ = inW.Close()
		_ = outR.Close()
		_ = peer.Close()
	})
	line, err := bufio.NewReader(outR).ReadString('\n')
	if err != nil {
		t.Fatalf("read listen line: %v (%s)", err, r.stderr.String())
	}
	idx := strings.Index(line, "http://127.0.0.1:")
	end := strings.Index(line[idx:], " ")
	r.baseURL = line[idx : idx+end]
	return r
}

func (r *qwenTestRun) do(t *testing.T, method, path, auth string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, r.baseURL+path, rdr)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *qwenTestRun) openSSE(t *testing.T, ctx context.Context, sid string) *bufio.Reader {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/session/"+sid+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+r.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("openSSE: %v %v", resp, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return bufio.NewReader(resp.Body)
}

func readSSEEnvelope(t *testing.T, rdr *bufio.Reader) map[string]any {
	t.Helper()
	for {
		line, err := rdr.ReadString('\n')
		if err != nil {
			t.Fatalf("readSSEEnvelope: %v", err)
		}
		if payload, ok := strings.CutPrefix(strings.TrimSpace(line), "data:"); ok {
			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &env); err != nil {
				t.Fatal(err)
			}
			return env
		}
	}
}

func acceptQwenMCP(t *testing.T, r *qwenTestRun) wire {
	t.Helper()
	_ = r.peer.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := r.peer.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	want := "mcp --token " + r.mcpTok + "\n"
	buf := make([]byte, len(want))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != want {
		t.Fatalf("MCP header %q (%v)", buf, err)
	}
	return wire{conn, json.NewDecoder(conn)}
}

func TestQwenVersionAndCLIValidation(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := RunQwen(t.Context(), "0.28.0", []string{"--version"}, strings.NewReader(""), &out, &errBuf); code != 0 || out.String() != "0.28.0\n" {
		t.Fatalf("version: code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
	errBuf.Reset()
	if code := RunQwen(t.Context(), "0.28.0", nil, strings.NewReader(""), &out, &errBuf); code != 1 || !strings.Contains(errBuf.String(), "expected serve") {
		t.Fatalf("missing serve: code=%d err=%q", code, errBuf.String())
	}
	t.Setenv("SOLODAWN_FAKE_QWEN_SCRIPT", filepath.Join(t.TempDir(), "missing.json"))
	errBuf.Reset()
	if code := RunQwen(t.Context(), "0.28.0", []string{"serve"}, strings.NewReader(""), &out, &errBuf); code != 1 {
		t.Fatalf("missing script expected exit 1, got %d", code)
	}
	bootPath := writeQwenScriptFile(t, `{"bootExit": 2, "bootStderr": "401 Invalid API key"}`)
	t.Setenv("SOLODAWN_FAKE_QWEN_SCRIPT", bootPath)
	errBuf.Reset()
	if code := RunQwen(t.Context(), "0.28.0", []string{"serve"}, strings.NewReader(""), &out, &errBuf); code != 2 || !strings.Contains(errBuf.String(), "Invalid API key") {
		t.Fatalf("bootExit: code=%d err=%q", code, errBuf.String())
	}
	noSessPath := writeQwenScriptFile(t, `{"session": ""}`)
	t.Setenv("SOLODAWN_FAKE_QWEN_SCRIPT", noSessPath)
	if code := RunQwen(t.Context(), "0.28.0", []string{"serve"}, strings.NewReader(""), &out, &errBuf); code != 1 {
		t.Fatalf("empty session expected exit 1, got %d", code)
	}
	validPath := writeQwenScriptFile(t, `{"session": "s1", "steps": []}`)
	t.Setenv("SOLODAWN_FAKE_QWEN_SCRIPT", validPath)
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "no-dir", "log"))
	if code := RunQwen(t.Context(), "0.28.0", []string{"serve"}, strings.NewReader(""), &out, &errBuf); code != 1 {
		t.Fatalf("bad log path expected exit 1, got %d", code)
	}
	_ = lifecycle(t)
	if code := RunQwen(t.Context(), "0.28.0", []string{"serve", "--hostname", "127.0.0.1", "--port", "99999"}, strings.NewReader(""), &out, &errBuf); code != 1 {
		t.Fatalf("invalid port expected exit 1, got %d", code)
	}
}

func writeQwenScriptFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "script.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestQwenApprovalAndResumeFixtures(t *testing.T) {
	log := lifecycle(t)
	r := startQwenFake(t, t.Context(), "testdata/qwen-approval.json", "tok-initial")
	if status, _ := r.do(t, http.MethodPost, "/session", "Bearer wrong", map[string]any{}); status != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", status)
	}
	if status, _ := r.do(t, http.MethodGet, "/unknown", "Bearer "+r.token, nil); status != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", status)
	}
	if status, _ := r.do(t, http.MethodPost, "/session/"+qwenTestSession+"/unknown", "Bearer "+r.token, nil); status != http.StatusNotFound {
		t.Fatalf("unknown session subpath status = %d", status)
	}
	status, created := r.do(t, http.MethodPost, "/session", "Bearer "+r.token, map[string]any{"cwd": "/tmp/ws"})
	if status != http.StatusOK || created["sessionId"] != qwenTestSession {
		t.Fatalf("create = %d %#v", status, created)
	}
	status, _ = r.do(t, http.MethodPost, "/session/"+qwenTestSession+"/approval-mode", "Bearer "+r.token, map[string]any{"mode": "yolo"})
	if status != http.StatusOK {
		t.Fatalf("approval-mode = %d", status)
	}
	sse := r.openSSE(t, t.Context(), qwenTestSession)
	status, _ = r.do(t, http.MethodPost, "/session/"+qwenTestSession+"/prompt", "Bearer "+r.token, map[string]any{
		"prompt": []map[string]string{{"type": "text", "text": "Try the write."}},
	})
	if status != http.StatusAccepted {
		t.Fatalf("prompt 1 = %d", status)
	}
	m := acceptQwenMCP(t, r)
	m.read(t, "initialize", "id", 0.0)
	m.send(t, `{"jsonrpc":"2.0","id":0,"result":{"capabilities":{"tools":{}}}}`)
	m.read(t, "tools/list", "id", 1.0)
	m.send(t, `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)

	if env := readSSEEnvelope(t, sse); env["type"] != "session_update" {
		t.Fatalf("expected session_update, got %#v", env)
	}
	if env := readSSEEnvelope(t, sse); env["type"] != "permission_request" {
		t.Fatalf("expected permission_request, got %#v", env)
	}
	r.do(t, http.MethodPost, "/session/"+qwenTestSession+"/permission/req-qwen-1", "Bearer "+r.token, map[string]any{
		"outcome": map[string]string{"outcome": "selected", "optionId": "cancel"},
	})
	if env := readSSEEnvelope(t, sse); env["type"] != "permission_resolved" {
		t.Fatalf("expected permission_resolved, got %#v", env)
	}
	if env := readSSEEnvelope(t, sse); env["type"] != "turn_complete" {
		t.Fatalf("expected turn_complete, got %#v", env)
	}
	r.do(t, http.MethodPost, "/session/"+qwenTestSession+"/prompt", "Bearer "+r.token, map[string]any{
		"prompt": []map[string]string{{"type": "text", "text": "Approved. Retry."}},
	})
	if env := readSSEEnvelope(t, sse); env["type"] != "permission_request" {
		t.Fatalf("expected second permission_request, got %#v", env)
	}
	r.do(t, http.MethodPost, "/session/"+qwenTestSession+"/permission/req-qwen-2", "Bearer "+r.token, map[string]any{
		"outcome": map[string]string{"outcome": "selected", "optionId": "proceed_once"},
	})
	if env := readSSEEnvelope(t, sse); env["type"] != "turn_complete" {
		t.Fatalf("expected second turn_complete, got %#v", env)
	}
	if status, _ := r.do(t, http.MethodDelete, "/session/"+qwenTestSession, "Bearer "+r.token, nil); status != http.StatusNoContent {
		t.Fatalf("delete = %d", status)
	}
	if code := <-r.exit; code != 0 {
		t.Fatalf("exit = %d (%s)", code, r.stderr.String())
	}
	log("start qwen "+qwenTestSession, "exit qwen "+qwenTestSession)

	r2 := startQwenFake(t, t.Context(), "testdata/qwen-resume.json", "tok-resumed")
	status, resumed := r2.do(t, http.MethodPost, "/session/"+qwenTestSession+"/resume", "Bearer "+r2.token, map[string]any{"cwd": "/tmp/ws"})
	if status != http.StatusOK || resumed["sessionId"] != qwenTestSession {
		t.Fatalf("resume = %d %#v", status, resumed)
	}
	sse2 := r2.openSSE(t, t.Context(), qwenTestSession)
	r2.do(t, http.MethodPost, "/session/"+qwenTestSession+"/prompt", "Bearer "+r2.token, map[string]any{
		"prompt": []map[string]string{{"type": "text", "text": "Check the token."}},
	})
	m2 := acceptQwenMCP(t, r2)
	m2.read(t, "initialize", "id", 0.0)
	m2.send(t, `{"jsonrpc":"2.0","id":0,"result":{"capabilities":{"tools":{}}}}`)
	m2.read(t, "tools/call", "id", 2.0, "params.name", "whoami")
	m2.result(t, 2, `{"token": "tok-resumed"}`)
	if env := readSSEEnvelope(t, sse2); env["type"] != "turn_complete" {
		t.Fatalf("expected resumed turn_complete, got %#v", env)
	}
	_ = r2.stdin.Close()
	if code := <-r2.exit; code != 0 {
		t.Fatalf("r2 exit = %d (%s)", code, r2.stderr.String())
	}
	log("start qwen "+qwenTestSession, "exit qwen "+qwenTestSession, "start qwen "+qwenTestSession, "exit qwen "+qwenTestSession)
}

func TestQwenStepErrorsCancelAndOpenFailures(t *testing.T) {
	_ = lifecycle(t)
	openFail := writeQwenScriptFile(t, `{"session": "s-err", "openStatus": 401, "openBody": {"error": "Unauthorized"}, "steps": [{"dir": "eof"}]}`)
	r1 := startQwenFake(t, t.Context(), openFail, "tok")
	if status, _ := r1.do(t, http.MethodPost, "/session", "Bearer "+r1.token, map[string]any{}); status != http.StatusUnauthorized {
		t.Fatalf("openFail status = %d", status)
	}
	if code := <-r1.exit; code != 0 {
		t.Fatalf("r1 code = %d", code)
	}

	resMiss := writeQwenScriptFile(t, `{"session": "s-known", "steps": [{"dir": "eof"}]}`)
	r2 := startQwenFake(t, t.Context(), resMiss, "tok")
	if status, _ := r2.do(t, http.MethodPost, "/session/s-other/resume", "Bearer "+r2.token, map[string]any{}); status != http.StatusNotFound {
		t.Fatalf("resume miss status = %d", status)
	}
	if code := <-r2.exit; code != 0 {
		t.Fatalf("r2 code = %d", code)
	}

	cancelScript := writeQwenScriptFile(t, `{
		"session": "s-cancel",
		"steps": [
			{"dir": "stderr", "msg": "warn line"},
			{"dir": "sse->node", "msg": {"v": 1, "type": "session_snapshot", "data": {}}},
			{"dir": "http<-node", "msg": {"route": "cancel", "sessionId": "$session"}},
			{"dir": "eof"}
		]
	}`)
	ctx, cancel := context.WithCancel(t.Context())
	r3 := startQwenFake(t, ctx, cancelScript, "tok")
	sseCtx, sseCancel := context.WithCancel(t.Context())
	sse := r3.openSSE(t, sseCtx, "s-cancel")
	_ = readSSEEnvelope(t, sse)
	sseCancel()
	if status, _ := r3.do(t, http.MethodPost, "/session/s-cancel/cancel", "Bearer "+r3.token, nil); status != http.StatusNoContent {
		t.Fatalf("cancel status = %d", status)
	}
	cancel()
	if code := <-r3.exit; code != 0 || !strings.Contains(r3.stderr.String(), "warn line") {
		t.Fatalf("r3 exit=%d stderr=%q", code, r3.stderr.String())
	}

	for _, rawScript := range []string{
		`{"session": "s-bad", "steps": [{"dir": "unknown"}]}`,
		`{"session": "s-bad", "steps": [{"dir": "http<-node", "msg": "bad-want"}]}`,
		`{"session": "s-bad", "steps": [{"dir": "http<-node", "msg": {"route": "prompt"}}]}`,
		`{"session": "s-bad", "steps": [{"dir": "sse->node", "msg": "{not-json"}]}`,
	} {
		p := writeQwenScriptFile(t, rawScript)
		rx := startQwenFake(t, t.Context(), p, "tok")
		if strings.Contains(rawScript, "sse->node") {
			_ = rx.openSSE(t, t.Context(), "s-bad")
		} else if strings.Contains(rawScript, `"route": "prompt"`) {
			_ = rx.stdin.Close()
		} else if strings.Contains(rawScript, `"bad-want"`) {
			rx.do(t, http.MethodPost, "/session/s-bad/cancel", "Bearer "+rx.token, nil)
		}
		if code := <-rx.exit; code != 1 {
			t.Fatalf("script %s expected exit 1, got %d (%s)", rawScript, code, rx.stderr.String())
		}
	}

	mismatch := writeQwenScriptFile(t, `{"session": "s-mis", "steps": [{"dir": "http<-node", "msg": {"route": "prompt"}}]}`)
	rm := startQwenFake(t, t.Context(), mismatch, "tok")
	rm.do(t, http.MethodPost, "/session/s-mis/cancel", "Bearer "+rm.token, nil)
	if code := <-rm.exit; code != 1 {
		t.Fatalf("mismatch expected exit 1, got %d", code)
	}
}
