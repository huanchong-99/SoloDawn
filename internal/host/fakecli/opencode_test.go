package fakecli

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startOpenCode runs one fake OpenCode in this process and returns the
// address it listens on.
func startOpenCode(t *testing.T, script string) (string, chan int, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "script.json"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOLODAWN_FAKE_OPENCODE_SCRIPT", filepath.Join(dir, "script.json"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(dir, "log"))
	t.Setenv("OPENCODE_SERVER_PASSWORD", "pw")
	out, stdout, _ := os.Pipe()
	exit, stderr := make(chan int, 1), &bytes.Buffer{}
	go func() {
		exit <- Run("opencode", "unused", []string{"serve"}, strings.NewReader(""), stdout, stderr)
		stdout.Close()
	}()
	line, _ := bufio.NewReader(out).ReadString('\n')
	_, base, _ := strings.Cut(strings.TrimSpace(line), "listening on ")
	return base, exit, stderr
}

func openCodeRequest(method, url, password, body string) *http.Request {
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.SetBasicAuth("opencode", password)
	return req
}

func openCodeDo(t *testing.T, req *http.Request) (int, string) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(data)
}

func openCodeCheck(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

func TestOpenCodeServesItsScriptOverHTTP(t *testing.T) {
	base, exit, stderr := startOpenCode(t, `{"session":"ses_1","exit":2,"steps":[
		{"dir":"http","msg":{"call":"POST /session","body":{},"res":{"id":"$session"}}},
		{"dir":"sse","msg":{ "type": "session.idle", "properties": {"sessionID":"$session"} }},
		{"dir":"http","msg":{"call":"GET /session/ses_1","body":null,"status":404}}]}`)
	code, _ := openCodeDo(t, openCodeRequest("GET", base+"/event", "wrong", ""))
	openCodeCheck(t, code == http.StatusUnauthorized, "status %d without the password", code)
	stream, err := http.DefaultClient.Do(openCodeRequest("GET", base+"/event", "pw", ""))
	openCodeCheck(t, err == nil, "event stream: %v", err)
	defer stream.Body.Close()
	code, body := openCodeDo(t, openCodeRequest("POST", base+"/session", "pw", "{}"))
	openCodeCheck(t, code == http.StatusOK && body == `{"id":"ses_1"}`, "status %d, body %q", code, body)
	line, _ := bufio.NewReader(stream.Body).ReadString('\n')
	openCodeCheck(t, line == `data: {"type":"session.idle","properties":{"sessionID":"ses_1"}}`+"\n", "event %q", line)
	code, _ = openCodeDo(t, openCodeRequest("GET", base+"/session/ses_1", "pw", ""))
	openCodeCheck(t, code == http.StatusNotFound, "status %d", code)
	code = <-exit
	openCodeCheck(t, code == 2 && stderr.String() == "", "exit %d, stderr %q", code, stderr)
}

func TestOpenCodeStopsAtAStepThatDoesNotHold(t *testing.T) {
	for _, tt := range []struct{ name, step, config, path, body, want string }{
		{"other request", `{"dir":"http","msg":{"call":"POST /session","body":{}}}`, "{}", "/other", "{}",
			"fake opencode: step 1 (http): expected POST /session, got POST /other\n"},
		{"other body", `{"dir":"http","msg":{"call":"POST /session","body":{"a":1}}}`, "{}", "/session", `{"a":2}`,
			`fake opencode: step 1 (http): expected {"a":1}, got {"a":2}` + "\n"},
		{"broken step", `{"dir":"http","msg":[]}`, "{}", "", "", "fake opencode: step 1 (http): json: cannot unmarshal"},
		{"broken configuration", `{"dir":"mcp<-cli","msg":{}}`, "{", "", "", "fake opencode: step 1 (mcp<-cli): unexpected end of JSON input\n"},
		{"missing server", `{"dir":"mcp<-cli","msg":{}}`, `{"mcp":{"solodawn":{"command":["` + filepath.ToSlash(filepath.Join(t.TempDir(), "none")) + `","--token","t"]}}}`, "", "",
			"fake opencode: step 1 (mcp<-cli): "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENCODE_CONFIG_CONTENT", tt.config)
			base, exit, stderr := startOpenCode(t, `{"session":"ses_1","steps":[`+tt.step+`]}`)
			if tt.path != "" {
				go http.DefaultClient.Do(openCodeRequest("POST", base+tt.path, "pw", tt.body))
			}
			if code := <-exit; code != 1 || !strings.HasPrefix(stderr.String(), tt.want) {
				t.Fatalf("exit %d, stderr %q, want %q", code, stderr, tt.want)
			}
		})
	}
}
