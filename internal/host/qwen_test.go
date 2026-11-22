package host

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func qwenLaunch(t *testing.T, session string, level int) Launch {
	return Launch{Folder: t.TempDir(), Session: session, Level: level, Shim: []string{"solodawn", "mcp"}, Instructions: "Be brief.", ToolTimeout: time.Minute}
}

func TestQwenTakesTurnsAndAnswersInOneDaemon(t *testing.T) {
	proc := startFakeServer(t, "qwen", qwenDriver{}, `{"session":"qs_1","steps":[
		{"dir":"http","msg":{"call":"POST /session","body":{},"res":{"sessionId":"$session"}}},
		{"dir":"http","msg":{"call":"POST /session/qs_1/prompt","body":{"prompt":[{"type":"text","text":"Be brief."},{"type":"text","text":"hello"}]}}},
		{"dir":"sse","msg":{"type":"session_update","data":{"sessionUpdate":"agent_message_chunk"}}},
		{"dir":"sse","msg":{"type":"permission_request","data":{"requestId":"req_1","toolCall":{"kind":"execute","rawInput":{"command":"git status"},"_meta":{"toolName":"run_shell_command"}}}}},
		{"dir":"http","msg":{"call":"POST /session/qs_1/permission/req_1","body":{"outcome":{"outcome":"selected","optionId":"cancel"}}}},
		{"dir":"sse","msg":{"type":"permission_request","data":{"requestId":"req_2","toolCall":{"kind":"think"}}}},
		{"dir":"http","msg":{"call":"POST /session/qs_1/permission/req_2","body":{"outcome":{"optionId":"proceed_once"}}}},
		{"dir":"http","msg":{"call":"POST /session/qs_1/cancel","body":{}}},
		{"dir":"sse","msg":{"type":"turn_complete","data":{"stopReason":"cancelled"}}},
		{"dir":"http","msg":{"call":"POST /session/qs_1/prompt","body":{"prompt":[{"type":"text","text":"again"}]}}},
		{"dir":"sse","msg":{"type":"turn_error","data":{"message":"401 Incorrect API key provided"}}},
		{"dir":"http","msg":{"call":"POST /session/qs_1/prompt","body":{"prompt":[{"type":"text","text":"third"}]}}},
		{"dir":"sse","msg":{"type":"turn_error","data":{"message":"model overloaded"}}},
		{"dir":"sse","msg":{"type":"heartbeat","data":{}}},
		{"dir":"sse","msg":{"type":"turn_complete","data":{"stopReason":"end_turn"}}},
		{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, qwenLaunch(t, "", 1))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "qs_1"})
	codexNoErr(t, proc.Turn("hello"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Active})
	asked := nextCodexEvent(t, proc.Events())
	codexEqual(t, []string{asked.Ask.ID, asked.Ask.Tool, asked.Ask.Category, asked.Ask.Command}, []string{"req_1", "run_shell_command", "command", "git status"})
	codexNoErr(t, proc.Answer("req_1", false))
	codexEqual(t, nextCodexEvent(t, proc.Events()).Ask.Category, "other")
	codexNoErr(t, proc.Answer("req_2", true))
	codexNoErr(t, proc.Interrupt())
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "interrupted"})
	codexNoErr(t, proc.Turn("again"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: LoginInvalid, Text: "401 Incorrect API key provided"})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "failed", Text: "401 Incorrect API key provided"})
	codexNoErr(t, proc.Turn("third"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "failed", Text: "model overloaded"})
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "completed"})
	codexNoErr(t, proc.Close())
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
}

func TestQwenResumedSessionGetsNoInstructions(t *testing.T) {
	proc := startFakeServer(t, "qwen", qwenDriver{}, `{"session":"qs_old","steps":[
		{"dir":"http","msg":{"call":"POST /session/qs_old/resume","body":{}}},
		{"dir":"http","msg":{"call":"POST /session/qs_old/prompt","body":{"prompt":[{"type":"text","text":"hello"}]}}},
		{"dir":"sse","msg":{"type":"turn_complete","data":{"stopReason":"end_turn"}}},
		{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, qwenLaunch(t, "qs_old", 3))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: Ready, Session: "qs_old"})
	codexNoErr(t, proc.Turn("hello"))
	codexEqual(t, nextCodexEvent(t, proc.Events()), Event{Kind: TurnEnded, Status: "completed"})
	codexNoErr(t, proc.Close())
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
}

func TestQwenStartPinsSettingsAndArguments(t *testing.T) {
	mcp := `"mcpServers":{"solodawn":{"args":["mcp"],"command":"solodawn","timeout":60000,"trust":true}}`
	group := qwenLaunch(t, "", 1)
	task := qwenLaunch(t, "", 3)
	task.RepoConfig = true
	for _, c := range []struct {
		l    Launch
		mode string
	}{{group, "plan"}, {task, "yolo"}} {
		settings, args := qwenStart(c.l, "tk")
		codexEqual(t, settings, `{`+mcp+`,"tools":{"approvalMode":"`+c.mode+`"}}`)
		codexEqual(t, args, []string{"serve", "--hostname", "127.0.0.1", "--port", "0", "--token", "tk", "--workspace", c.l.Folder})
	}
}

func TestQwenReportsASessionItCannotResume(t *testing.T) {
	proc := startFakeServer(t, "qwen", qwenDriver{}, `{"session":"qs_old","steps":[
		{"dir":"http","msg":{"call":"POST /session/qs_old/resume","body":{},"status":404}},
		{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`, qwenLaunch(t, "qs_old", 3))
	ev := nextCodexEvent(t, proc.Events())
	codexCheck(t, ev.Kind == NoSession && strings.Contains(ev.Text, "POST /session/qs_old/resume: 404"), ev.Text)
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
}

func TestQwenWhoseDaemonFailsEndsWithoutASession(t *testing.T) {
	proc := startFakeServer(t, "qwen", qwenDriver{}, `{`, qwenLaunch(t, "", 3))
	codexEqual(t, len(waitEventsClosed(t, proc.Events())), 0)
	codexWantErr(t, proc.Turn("hello"))
	codexWantErr(t, proc.Answer("\x00", true))
}

func TestQwenStartRefusals(t *testing.T) {
	_, _, err := qwenDriver{}.Interactive(Launch{})
	codexWantErr(t, err)
	_, err = qwenDriver{}.Start(context.Background(), qwenLaunch(t, "", 2))
	codexWantErr(t, err)
	t.Setenv("PATH", t.TempDir())
	_, err = qwenDriver{}.Start(context.Background(), qwenLaunch(t, "", 1))
	codexWantErr(t, err)
	missing := filepath.Join(t.TempDir(), "missing")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}
	_, err = qwenDriver{}.Start(context.Background(), qwenLaunch(t, "", 1))
	codexEqual(t, errors.Is(err, fs.ErrNotExist), true)
}
