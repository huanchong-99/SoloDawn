package host

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// startOpenCode starts the driver on the fake OpenCode replaying script.
func startOpenCode(t *testing.T, script string, l Launch) Process {
	return startFakeServer(t, "opencode", openCodeDriver{}, script, l)
}

// startFakeServer starts d on the fake vendor CLI, built for this test,
// replaying script.
func startFakeServer(t *testing.T, vendor string, d Driver, script string, l Launch) Process {
	t.Helper()
	fakeServerCLI(t, vendor, script)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	proc, err := d.Start(ctx, l)
	codexNoErr(t, err)
	return proc
}

func fakeServerCLI(t *testing.T, vendor, script string) {
	t.Helper()
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", bin+string(os.PathSeparator), "./fakecli/"+vendor).CombinedOutput(); err != nil {
		t.Fatalf("build fake %s: %v\n%s", vendor, err, out)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(t.TempDir(), "script.json")
	codexNoErr(t, os.WriteFile(path, []byte(script), 0600))
	t.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", path)
	t.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(t.TempDir(), "log"))
}

func TestOpenCodeModelDiscovery(t *testing.T) {
	script, err := os.ReadFile("fakecli/testdata/opencode-models.json")
	codexNoErr(t, err)
	fakeServerCLI(t, "opencode", string(script))
	codexEqual(t, (openCodeDriver{}).Models(t.Context()), []proto.Model{
		{Model: "anthropic/claude-opus-5", Name: "Opus", Efforts: []string{"low", "high"}, Tier: "strong"},
		{Model: "openai/gpt-6-mini", Name: "Mini", Efforts: []string{}, Tier: "light"},
	})
	modelScript(t, "opencode", `{"session":"models","steps":[{"dir":"http","msg":{"call":"GET /api/model","body":null,"status":503}},{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`)
	codexEqual(t, (openCodeDriver{}).Models(t.Context()), []proto.Model{})
	t.Setenv("PATH", t.TempDir())
	codexEqual(t, (openCodeDriver{}).Models(t.Context()), []proto.Model{})
}

func TestOpenCodeModelListingBoundary(t *testing.T) {
	for _, data := range []string{
		`{`, `null`, `{}`, `{"data":null}`, `{"data":{}}`,
		`{"data":[{"modelID":"opus","variants":[]}]}`,
		`{"data":[{"providerID":"p","variants":[]}]}`,
		`{"data":[{"providerID":"p","modelID":"opus"}]}`,
		`{"data":[{"providerID":"p","modelID":"opus","variants":[{}]}]}`,
		`{"data":[{"providerID":"p","modelID":"opus","variants":[],"name":1}]}`,
	} {
		codexEqual(t, openCodeModels([]byte(data)), []proto.Model{})
	}
	codexEqual(t, openCodeModels([]byte(`{"data":[]}`)), []proto.Model{})
	codexEqual(t, openCodeModels([]byte(`{"data":[{"providerID":"p","modelID":"haiku","variants":[]}]}`)), []proto.Model{{Model: "p/haiku", Name: "p/haiku", Efforts: []string{}, Tier: "light"}})
}

func TestOpenCodeChoicePrecedesOnlyTheFirstPromptOfEachProcess(t *testing.T) {
	for _, session := range []string{"", "ses_choice"} {
		for _, effort := range []string{"", "custom"} {
			l := openCodeLaunch(t, session, 3)
			l.Model, l.Effort = "provider/model/part", effort
			open := `{"dir":"http","msg":{"call":"POST /session","body":{},"res":{"id":"$session"}}},`
			if session != "" {
				open = `{"dir":"http","msg":{"call":"GET /session/ses_choice","body":null,"res":{"id":"$session"}}},`
			}
			model := map[string]string{"providerID": "provider", "id": "model/part"}
			if effort != "" {
				model["variant"] = effort
			}
			script := `{"session":"ses_choice","steps":[` + open + `{"dir":"http","msg":{"call":"POST /api/session/ses_choice/model","body":{"model":` + marshalWire(model) + `},"status":204}},`
			for range 2 {
				script += `{"dir":"http","msg":{"call":"POST /session/ses_choice/prompt_async","body":{"parts":[{"type":"text","text":"hello"}]}}},{"dir":"sse","msg":{"type":"session.idle","properties":{"sessionID":"$session"}}},`
			}
			proc := startOpenCode(t, script+`{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, l)
			codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "ses_choice"})
			assertOpenCodeTwoTurns(t, proc)
		}
	}
}

func assertOpenCodeTwoTurns(t *testing.T, proc Process) {
	t.Helper()
	for range 2 {
		codexNoErr(t, proc.Turn("hello"))
		codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "completed"})
	}
	codexNoErr(t, proc.Close())
	codexEqual(t, waitEventsClosed(t, proc.Events()), []Event(nil))
}

func TestOpenCodeSkipsEffortWithoutModel(t *testing.T) {
	l := openCodeLaunch(t, "", 3)
	l.Effort = "high"
	proc := startOpenCode(t, `{"session":"ses_choice","steps":[{"dir":"http","msg":{"call":"POST /session","body":{},"res":{"id":"$session"}}},{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, l)
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "ses_choice"})
	codexNoErr(t, proc.Close())
	codexEqual(t, waitEventsClosed(t, proc.Events()), []Event(nil))
}

func TestOpenCodeRefusedChoiceClosesWithoutLosingSession(t *testing.T) {
	l := openCodeLaunch(t, "ses_choice", 3)
	l.Model, l.Effort = "p/refused", "high"
	proc := startOpenCode(t, `{"session":"ses_choice","steps":[{"dir":"http","msg":{"call":"GET /session/ses_choice","body":null,"res":{"id":"$session"}}},{"dir":"http","msg":{"call":"POST /api/session/ses_choice/model","body":{"model":{"providerID":"p","id":"refused","variant":"high"}},"status":400,"res":{"error":"refused"}}},{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, l)
	codexEqual(t, waitEventsClosed(t, proc.Events()), []Event(nil))
	codexEqual(t, fmt.Sprint(proc.Turn("never")), `opencode: POST /api/session/ses_choice/model: 400 Bad Request {"error":"refused"}`)
}

func TestOpenCodeModelPayloadOmitsEmptyVariant(t *testing.T) {
	for _, tc := range []struct{ effort, body string }{
		{"", `{"model":{"id":"x","providerID":"p"}}`},
		{"custom", `{"model":{"id":"x","providerID":"p","variant":"custom"}}`},
	} {
		calls := make(chan string, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			calls <- r.Method + " " + r.URL.Path + " " + string(data)
			w.WriteHeader(http.StatusNoContent)
		}))
		p := openCodeProcess{base: server.URL, session: "saved", launch: Launch{Model: "p/x", Effort: tc.effort}}
		codexNoErr(t, p.selectModel())
		codexEqual(t, <-calls, "POST /api/session/saved/model "+tc.body)
		server.Close()
	}
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
