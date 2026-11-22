package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// startOpenCode starts the driver on the fake OpenCode, built for this test,
// replaying script.
func startOpenCode(t *testing.T, script string, l Launch) Process {
	t.Helper()
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", bin+string(os.PathSeparator), "./fakecli/opencode").CombinedOutput(); err != nil {
		t.Fatalf("build fake opencode: %v\n%s", err, out)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(t.TempDir(), "script.json")
	codexNoErr(t, os.WriteFile(path, []byte(script), 0600))
	t.Setenv("SOLODAWN_FAKE_OPENCODE_SCRIPT", path)
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	proc, err := openCodeDriver{}.Start(ctx, l)
	codexNoErr(t, err)
	return proc
}

func openCodeLaunch(t *testing.T, session string, level int) Launch {
	return Launch{Folder: t.TempDir(), Session: session, Level: level, Shim: []string{"solodawn", "mcp"}, Instructions: "Be brief."}
}

func TestOpenCodeTakesTurnsAndAnswersInOneServer(t *testing.T) {
	proc := startOpenCode(t, `{"session":"ses_1","steps":[
		{"dir":"http","msg":{"call":"POST /session","body":{},"res":{"id":"$session"}}},
		{"dir":"http","msg":{"call":"POST /session/ses_1/prompt_async","body":{"system":"Be brief.","parts":[{"type":"text","text":"hello"}]},"status":204}},
		{"dir":"sse","msg":{"type":"message.part.updated","properties":{"part":{"text":"`+strings.Repeat("x", 70000)+`"}}}},
		{"dir":"sse","msg":{"type":"permission.asked","properties":{"id":"per_1","sessionID":"$session","permission":"bash","patterns":["git status"]}}},
		{"dir":"http","msg":{"call":"POST /permission/per_1/reply","body":{"reply":"reject"}}},
		{"dir":"http","msg":{"call":"POST /session/ses_1/abort","body":{}}},
		{"dir":"sse","msg":{"type":"session.error","properties":{"sessionID":"$session","error":{"name":"MessageAbortedError"}}}},
		{"dir":"sse","msg":{"type":"session.error","properties":{"sessionID":"ses_child","error":{"name":"ProviderAuthError"}}}},
		{"dir":"sse","msg":{"type":"session.idle","properties":{"sessionID":"ses_child"}}},
		{"dir":"sse","msg":{"type":"session.idle","properties":{"sessionID":"$session"}}},
		{"dir":"http","msg":{"call":"POST /session/ses_1/prompt_async","body":{"parts":[{"type":"text","text":"again"}]}}},
		{"dir":"sse","msg":{"type":"session.error","properties":{"error":{"name":"ProviderAuthError","data":{"message":"no API key"}}}}},
		{"dir":"sse","msg":{"type":"session.idle","properties":{"sessionID":"$session"}}},
		{"dir":"http","msg":{"call":"POST /session/ses_1/prompt_async","body":{"parts":[{"type":"text","text":"third"}]}}},
		{"dir":"sse","msg":{"type":"server.heartbeat","properties":{}}},
		{"dir":"sse","msg":{"type":"session.idle","properties":{"sessionID":"$session"}}},
		{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, openCodeLaunch(t, "", 1))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "ses_1"})
	codexNoErr(t, proc.Turn("hello"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Active})
	asked := nextCodexEvent(t, proc.Events())
	codexEqual(t, []string{asked.Ask.ID, asked.Ask.Tool, asked.Ask.Category, asked.Ask.Command}, []string{"per_1", "bash", "command", "git status"})
	codexNoErr(t, proc.Answer("per_1", false))
	codexNoErr(t, proc.Interrupt())
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "interrupted"})
	codexNoErr(t, proc.Turn("again"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: LoginInvalid, Text: "no API key"})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "failed"})
	codexNoErr(t, proc.Turn("third"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "completed"})
	codexNoErr(t, proc.Close())
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
}

func TestOpenCodeReportsASessionItCannotResume(t *testing.T) {
	l := openCodeLaunch(t, "ses_old", 3)
	l.RepoConfig = true
	proc := startOpenCode(t, `{"session":"ses_old","steps":[
		{"dir":"http","msg":{"call":"GET /session/ses_old","body":null,"status":404,"res":{"name":"NotFoundError"}}},
		{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, l)
	ev := nextCodexEvent(t, proc.Events())
	codexCheck(t, ev.Kind == NoSession && strings.Contains(ev.Text, "GET /session/ses_old: 404"), ev.Text)
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
}

func TestOpenCodeWhoseServerFailsEndsWithoutASession(t *testing.T) {
	proc := startOpenCode(t, `{`, openCodeLaunch(t, "", 1))
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
	codexWantErr(t, proc.Turn("hello"))
	codexWantErr(t, proc.Answer("\x00", true))
}

func TestOpenCodeStartRefusals(t *testing.T) {
	codexEqual(t, serverAddress(strings.NewReader("starting\n")), "")
	_, _, err := openCodeDriver{}.Interactive(Launch{})
	codexWantErr(t, err)
	_, err = openCodeDriver{}.Start(context.Background(), openCodeLaunch(t, "", 2))
	codexWantErr(t, err)
	t.Setenv("PATH", t.TempDir())
	_, err = openCodeDriver{}.Start(context.Background(), openCodeLaunch(t, "", 1))
	codexWantErr(t, err)
}

func TestOpenCodeEnvPinsLevelShimAndRepoConfig(t *testing.T) {
	mcp := `"mcp":{"solodawn":{"command":["solodawn","mcp"],"type":"local"}}`
	group := openCodeLaunch(t, "", 1)
	task := openCodeLaunch(t, "", 3)
	task.RepoConfig = true
	codexEqual(t, openCodeEnv(group, "pw"), []string{
		"OPENCODE_SERVER_PASSWORD=pw",
		`OPENCODE_CONFIG_CONTENT={` + mcp + `,"permission":{"bash":"deny","edit":"deny"}}`,
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
	})
	codexEqual(t, openCodeEnv(task, "pw"), []string{
		"OPENCODE_SERVER_PASSWORD=pw",
		`OPENCODE_CONFIG_CONTENT={` + mcp + `,"permission":{"bash":"allow","doom_loop":"allow","edit":"allow","external_directory":"allow","webfetch":"allow"}}`,
	})
}
