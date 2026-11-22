package host

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Qwen Code runs as `qwen serve`, one daemon per session process: turns go
// over its session API and come back on the session's event stream
// (planning/contracts/step-3/drivers.md section 8, [documented]).
func init() {
	caps := Capabilities{Levels: []int{1, 3}, Approvals: ApprovalConn, Takeover: TakeoverNone}
	caps.Accounts, caps.Usage = []string{AccountAPIKey}, []string{}
	Register(DriverCaps, "qwen", caps)
	Register[Driver](Drivers, "qwen", qwenDriver{})
}

// qwenModes holds Qwen Code's approval mode by level: plan mode edits and
// runs nothing, yolo asks nothing.
var qwenModes = map[int]string{1: "plan", 3: "yolo"}

var qwenCategories = map[string]string{"execute": "command", "edit": "file", "fetch": "network"}

type qwenDriver struct{}

func (qwenDriver) Interactive(Launch) ([]string, string, error) {
	return nil, "", fmt.Errorf("qwen: no terminal takeover")
}

// Start writes the level's settings to a temporary folder that lives as
// long as the daemon. A new session gets the instructions with its first
// prompt; a resumed one has them in its history.
func (qwenDriver) Start(ctx context.Context, l Launch) (Process, error) {
	if _, ok := qwenModes[l.Level]; !ok {
		return nil, fmt.Errorf("qwen: level %d is not declared", l.Level)
	}
	dir, err := os.MkdirTemp("", "solodawn-qwen-")
	if err != nil {
		return nil, err
	}
	p := &qwenProcess{token: rand.Text(), resume: l.Session, events: make(chan Event, 16), ready: make(chan struct{})}
	if l.Session == "" {
		p.instructions = l.Instructions
	}
	settings, args := qwenStart(l, p.token)
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(settings), 0o600)
	ctx, p.stop = context.WithCancel(ctx)
	out, wait, err := serve(ctx, l.Folder, []string{"QWEN_CODE_SYSTEM_SETTINGS_PATH=" + path}, "qwen", args...)
	if err != nil {
		p.stop()
		os.RemoveAll(dir)
		return nil, err
	}
	go p.run(out, func() {
		p.stop()
		wait()
		os.RemoveAll(dir)
	})
	return p, nil
}

// qwenStart is what the daemon of l gets: its settings file (the level's
// approval mode and the trusted solodawn MCP server) and its arguments.
func qwenStart(l Launch, token string) (string, []string) {
	settings := marshalWire(map[string]any{
		"tools": map[string]string{"approvalMode": qwenModes[l.Level]},
		"mcpServers": map[string]any{"solodawn": map[string]any{
			"command": l.Shim[0], "args": l.Shim[1:], "timeout": l.ToolTimeout.Milliseconds(), "trust": true,
		}},
	})
	return settings, []string{"serve", "--hostname", "127.0.0.1", "--port", "0", "--token", token, "--workspace", l.Folder}
}

// qwenProcess drives one qwen daemon. base, session and url (the session's
// path) are written before ready is closed and only read after it.
// instructions are sent with the first prompt, then cleared.
type qwenProcess struct {
	token, resume      string
	instructions       string
	events             chan Event
	ready              chan struct{}
	stop               context.CancelFunc
	base, session, url string
}

type qwenEvent struct {
	Type string
	Data struct {
		SessionID  string `json:"sessionId"`
		RequestID  string `json:"requestId"`
		StopReason string `json:"stopReason"`
		Message    string
		ToolCall   struct {
			Kind     string
			RawInput json.RawMessage `json:"rawInput"`
			Meta     struct {
				ToolName string `json:"toolName"`
			} `json:"_meta"`
		} `json:"toolCall"`
	}
}

func (p *qwenProcess) Events() <-chan Event { return p.events }

func (p *qwenProcess) Turn(text string) error {
	<-p.ready
	prompt := []map[string]string{{"type": "text", "text": text}}
	if p.instructions != "" {
		prompt = append([]map[string]string{{"type": "text", "text": p.instructions}}, prompt...)
		p.instructions = ""
	}
	_, err := p.post("/prompt", map[string]any{"prompt": prompt})
	return err
}

func (p *qwenProcess) Interrupt() error {
	<-p.ready
	_, err := p.post("/cancel", struct{}{})
	return err
}

func (p *qwenProcess) Answer(ask string, allow bool) error {
	<-p.ready
	option := map[bool]string{true: "proceed_once", false: "cancel"}[allow]
	_, err := p.post("/permission/"+ask, map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": option}})
	return err
}

// Close stops the daemon: it does not exit when its input closes.
func (p *qwenProcess) Close() error {
	p.stop()
	return nil
}

// request sends one call to the daemon; a status of 300 or more is an error.
func (p *qwenProcess) request(method, path string, body any) (*http.Response, error) {
	req, err := http.NewRequest(method, p.base+path, strings.NewReader(marshalWire(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err == nil && res.StatusCode >= http.StatusMultipleChoices {
		res.Body.Close()
		err = fmt.Errorf("qwen: %s %s: %s", method, path, res.Status)
	}
	return res, err
}

// post calls a path under url and returns the session id of the answer.
func (p *qwenProcess) post(path string, body any) (string, error) {
	res, err := p.request(http.MethodPost, p.url+path, body)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var s struct {
		SessionID string `json:"sessionId"`
	}
	json.NewDecoder(res.Body).Decode(&s)
	return s.SessionID, nil
}

// run opens the session, then reads its event stream until the daemon exits.
func (p *qwenProcess) run(out *os.File, wait func()) {
	defer close(p.events)
	defer wait()
	p.base = serverAddress(out)
	err := p.open()
	var stream *http.Response
	if err == nil {
		stream, err = p.request(http.MethodGet, p.url+"/events", nil)
	}
	close(p.ready)
	if err != nil {
		return
	}
	defer stream.Body.Close()
	p.events <- Event{Kind: Ready, Session: p.session}
	readSSE(stream.Body, func(data []byte) {
		var e qwenEvent
		if json.Unmarshal(data, &e) == nil {
			p.handle(e, data)
		}
	})
}

// open resumes the launch's session, or creates one.
func (p *qwenProcess) open() error {
	if p.resume == "" {
		p.url = "/session"
		id, err := p.post("", map[string]any{})
		p.session, p.url = id, "/session/"+id
		return err
	}
	p.session, p.url = p.resume, "/session/"+p.resume
	_, err := p.post("/resume", map[string]any{})
	if err != nil {
		p.events <- Event{Kind: NoSession, Text: err.Error()}
	}
	return err
}

func (p *qwenProcess) handle(e qwenEvent, raw json.RawMessage) {
	switch e.Type {
	case "session_update":
		p.events <- Event{Kind: Active}
	case "permission_request":
		p.events <- Event{Kind: Asked, Ask: qwenAsk(e, raw)}
	case "turn_complete":
		p.events <- Event{Kind: TurnEnded, Status: map[bool]string{true: "interrupted", false: "completed"}[e.Data.StopReason == "cancelled"]}
	case "turn_error":
		if strings.Contains(e.Data.Message, "401") {
			p.events <- Event{Kind: LoginInvalid, Text: e.Data.Message}
		}
		p.events <- Event{Kind: TurnEnded, Status: "failed", Text: e.Data.Message}
	}
}

func qwenAsk(e qwenEvent, raw json.RawMessage) *Ask {
	call := e.Data.ToolCall
	category, ok := qwenCategories[call.Kind]
	if !ok {
		category = "other"
	}
	var input struct{ Command string }
	json.Unmarshal(call.RawInput, &input)
	return &Ask{ID: e.Data.RequestID, Tool: call.Meta.ToolName, Category: category, Command: input.Command, Input: raw}
}
