package fakecli

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQwenServesItsScriptBehindTheBearerToken(t *testing.T) {
	dir := t.TempDir()
	script := `{"session":"qs_1","steps":[
		{"dir":"sse","msg":{"type":"turn_complete","data":{}}},
		{"dir":"mcp<-cli","msg":{"jsonrpc":"2.0","id":0,"method":"initialize"}}]}`
	os.WriteFile(filepath.Join(dir, "script.json"), []byte(script), 0600)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"mcpServers":{"solodawn":{"command":"`+filepath.ToSlash(filepath.Join(dir, "none"))+`"}}}`), 0600)
	t.Setenv("SOLODAWN_FAKE_QWEN_SCRIPT", filepath.Join(dir, "script.json"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(dir, "log"))
	t.Setenv("QWEN_CODE_SYSTEM_SETTINGS_PATH", filepath.Join(dir, "settings.json"))
	out, stdout, _ := os.Pipe()
	exit, stderr := make(chan int, 1), &strings.Builder{}
	go func() {
		exit <- Run("qwen", "unused", []string{"serve", "--token", "tk", "--workspace", "/w"}, strings.NewReader(""), stdout, stderr)
	}()
	line, _ := bufio.NewReader(out).ReadString('\n')
	base, ok := strings.CutPrefix(line, "qwen serve listening on ")
	base, rest, _ := strings.Cut(base, " ")
	openCodeCheck(t, ok && rest == "(mode=http-bridge, workspace=/w)\n", "address line %q", line)
	post, _ := http.NewRequest("POST", base+"/session", strings.NewReader("{}"))
	post.Header.Set("Authorization", "Bearer tk")
	code, _ := openCodeDo(t, post)
	openCodeCheck(t, code == http.StatusUnsupportedMediaType, "status %d without Content-Type", code)
	req, _ := http.NewRequest("GET", base+"/session/qs_1/events", nil)
	code, _ = openCodeDo(t, req)
	openCodeCheck(t, code == http.StatusUnauthorized, "status %d without the token", code)
	req.Header.Set("Authorization", "Bearer tk")
	stream, err := http.DefaultClient.Do(req)
	openCodeCheck(t, err == nil, "event stream: %v", err)
	defer stream.Body.Close()
	event, _ := bufio.NewReader(stream.Body).ReadString('\n')
	openCodeCheck(t, event == `data: {"type":"turn_complete","data":{}}`+"\n", "event %q", event)
	code = <-exit
	openCodeCheck(t, code == 1 && strings.HasPrefix(stderr.String(), "fake qwen: step 2 (mcp<-cli): "), "exit %d, stderr %q", code, stderr)
}
