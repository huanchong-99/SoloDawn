package fakecli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const session = "7dc7f589-d9e4-45ba-9974-b043cf19969a"

// replay runs one start in this process against a script given as text.
func replay(t *testing.T, script, stdin string, args ...string) (code int, stdout, stderr, log string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "script.json"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", filepath.Join(dir, "script.json"))
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(dir, "log"))
	var out, errs bytes.Buffer
	code = Run("claude", "9.9.9 (Test)", args, strings.NewReader(stdin), &out, &errs)
	written, _ := os.ReadFile(filepath.Join(dir, "log"))
	return code, out.String(), errs.String(), string(written)
}

func TestVersionNeedsNoScript(t *testing.T) {
	var out, errs bytes.Buffer
	code := Run("claude", "9.9.9 (Test)", []string{"--version"}, strings.NewReader(""), &out, &errs)
	if code != 0 || out.String() != "9.9.9 (Test)\n" || errs.String() != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errs.String())
	}
}

func TestAScriptIsReplayedStepByStep(t *testing.T) {
	code, stdout, stderr, log := replay(t, `{"session":"unused","exit":3,"steps":[
		{"dir":"stdin","msg":{"method":"turn","params":{"input":[{"text":"go"}]}}},
		{"dir":"stdout","msg":{ "id" : "$id", "session_id":"$session", "text":"<user> & 检查", "cost":1.50 }},
		{"dir":"stderr","msg":"\u001b[31mERROR\u001b[0m rejected"},
		{"dir":"stdin","msg":{"id":"reply-1"}},
		{"dir":"stdout","msg":{"id":"$id","result":{}}},
		{"dir":"eof"}]}`,
		`{"id":7,"method":"turn","params":{"cwd":"here","input":[{"text":"go"}]}}`+"\n"+`{"id":"reply-1","result":null}`+"\n",
		"-p", "--session-id", session)
	if code != 3 || stderr != "\x1b[31mERROR\x1b[0m rejected\n" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if want := `{"id":7,"session_id":"` + session + `","text":"<user> & 检查","cost":1.50}` + "\n" + `{"id":"reply-1","result":{}}` + "\n"; stdout != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
	if want := "start claude " + session + "\nexit claude " + session + "\n"; log != want {
		t.Fatalf("log %q, want %q", log, want)
	}
}

func TestTheSessionIdComesFromTheStartOrTheScript(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"-p", "--session-id", "first"}, "first"},
		{[]string{"-p", "--resume", "again"}, "again"},
		{[]string{"--resume", "terminal"}, "terminal"},
		{[]string{"resume", "thread"}, "thread"},
		{[]string{"app-server"}, "scripted"},
	} {
		code, stdout, stderr, log := replay(t, `{"session":"scripted","steps":[{"dir":"eof"}]}`, "", tt.args...)
		if want := "start claude " + tt.want + "\nexit claude " + tt.want + "\n"; code != 0 || stdout != "" || stderr != "" || log != want {
			t.Errorf("%v: exit %d, stdout %q, stderr %q, log %q", tt.args, code, stdout, stderr, log)
		}
	}
}

func TestAScriptStopsAtTheFirstStepThatDoesNotHold(t *testing.T) {
	const released = `,{"dir":"stdout","msg":{"released":true}}]}`
	for _, tt := range []struct {
		name, steps, stdin, want string
	}{
		{"another decision", `{"dir":"stdin","msg":{"id":0,"result":{"decision":"decline"}}}`, `{"id":0,"result":{"decision":"accept"}}` + "\n",
			`step 1 (stdin): expected {"id":0,"result":{"decision":"decline"}}, got {"id":0,"result":{"decision":"accept"}}`},
		{"closed before the line", `{"dir":"eof"},{"dir":"stdin","msg":{}}`, "", "step 2 (stdin): EOF"},
		{"a line that is not JSON", `{"dir":"stdin","msg":{}}`, "nope\n", "step 1 (stdin): invalid character 'o' in literal null (expecting 'u')"},
		{"input where the end is due", `{"dir":"eof"}`, "{}\n", "step 1 (eof): expected closed stdin"},
		{"unknown direction", `{"dir":"eof"},{"dir":"sideways","msg":{}}`, "", `step 2 (sideways): unknown direction "sideways"`},
		{"no message", `{"dir":"stdout"}`, "", "step 1 (stdout): unexpected end of JSON input"},
		{"no expectation", `{"dir":"stdin"}`, "{}\n", "step 1 (stdin): unexpected end of JSON input"},
		{"reply without a request", `{"dir":"stdout","msg":{"id":"$id"}}`, "", "step 1 (stdout): invalid character '}' looking for beginning of value"},
		{"stderr that is no string", `{"dir":"stderr","msg":5}`, "", "step 1 (stderr): json: cannot unmarshal number into Go value of type string"},
		{"no MCP server", `{"dir":"mcp<-cli","msg":{}}`, "", "step 1 (mcp<-cli): exec: no command"},
	} {
		code, stdout, stderr, log := replay(t, `{"steps":[`+tt.steps+released, tt.stdin, "--resume", session)
		if want := "start claude " + session + "\nexit claude " + session + "\n"; code != 1 || stdout != "" || stderr != "fake claude: "+tt.want+"\n" || log != want {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, log %q", tt.name, code, stdout, stderr, log)
		}
	}
}

func TestAStartThatCannotRunSaysWhy(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.json")
	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	const mcp = `{"steps":[{"dir":"mcp->cli","msg":{}}]}`
	started := "start claude " + session + "\nexit claude " + session + "\n"
	for _, tt := range []struct {
		name, script, env, value string
		args                     []string
		want, log                string
	}{
		{"no script", `{}`, "SOLODAWN_FAKE_CLAUDE_SCRIPT", "", []string{"--resume", session},
			"SOLODAWN_FAKE_CLAUDE_SCRIPT: open : no such file or directory", ""},
		{"broken script", `{`, "", "", []string{"--resume", session},
			"SOLODAWN_FAKE_CLAUDE_SCRIPT: unexpected end of JSON input", ""},
		{"no session id", `{}`, "", "", []string{"-p"}, "no session id in the arguments or the script", ""},
		{"session id cut off", `{}`, "", "", []string{"-p", "--resume"}, "no session id in the arguments or the script", ""},
		{"no log", `{}`, "SOLODAWN_FAKECLI_LOG", "", []string{"--resume", session},
			"SOLODAWN_FAKECLI_LOG: open : no such file or directory", ""},
		{"no MCP file", mcp, "", "", []string{"--resume", session, "--mcp-config", missing},
			"step 1 (mcp->cli): open " + missing + ": no such file or directory", started},
		{"broken MCP file", mcp, "", "", []string{"--resume", session, "--mcp-config", broken},
			"step 1 (mcp->cli): unexpected end of JSON input", started},
		{"MCP command that is no string", mcp, "", "", []string{"resume", session, "-c", "mcp_servers.solodawn.command=7"},
			"step 1 (mcp->cli): json: cannot unmarshal number into Go value of type string", started},
		{"MCP arguments that are no list", mcp, "", "", []string{"resume", session, "-c", "mcp_servers.solodawn.args=7"},
			"step 1 (mcp->cli): json: cannot unmarshal number into Go value of type []string", started},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SOLODAWN_FAKE_CLAUDE_SCRIPT", filepath.Join(dir, "script.json"))
			t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(dir, "log"))
			if err := os.WriteFile(filepath.Join(dir, "script.json"), []byte(tt.script), 0600); err != nil {
				t.Fatal(err)
			}
			if tt.env != "" {
				t.Setenv(tt.env, tt.value)
			}
			var out, errs bytes.Buffer
			code := Run("claude", "9.9.9 (Test)", tt.args, strings.NewReader(""), &out, &errs)
			log, _ := os.ReadFile(filepath.Join(dir, "log"))
			if code != 1 || out.String() != "" || errs.String() != "fake claude: "+tt.want+"\n" || string(log) != tt.log {
				t.Fatalf("exit %d, stdout %q, stderr %q, log %q", code, out.String(), errs.String(), log)
			}
		})
	}
}

func TestAnExpectationNamesTheFieldsThatMatter(t *testing.T) {
	const line = `{"id":2,"result":{"content":[{"type":"text","text":"allow"}],"isError":false}}`
	for _, tt := range []struct {
		want string
		ok   bool
	}{
		{`{}`, true},
		{`{"id":2}`, true},
		{`{"result":{"isError":false}}`, true},
		{`{"result":{"content":[{"type":"text","text":"allow"}]}}`, true},
		{line, true},
		{`{"id":3}`, false},
		{`{"id":"2"}`, false},
		{`{"error":{}}`, false},
		{`{"id":{}}`, false},
		{`{"result":{"content":[{"type":"text"}]}}`, false},
		{`{"result":{"content":[{"text":"allow","type":"text"},{}]}}`, false},
		{`[]`, false},
		{`2`, false},
	} {
		var got, want any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
			t.Fatal(err)
		}
		if contains(got, want) != tt.ok {
			t.Errorf("%s: got %t, want %t", tt.want, !tt.ok, tt.ok)
		}
	}
}
