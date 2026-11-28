package host

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// OpenCode runs as `opencode serve`, one server per session process: turns
// go over its session API and come back on its event stream
// (planning/contracts/step-3/drivers.md section 8, [documented]).
func init() {
	Register[Driver](Drivers, "opencode", openCodeDriver{})
	Register(DriverCaps, "opencode", Capabilities{
		Accounts:  []string{AccountAPIKey},
		Levels:    []int{1, 3},
		Approvals: ApprovalConn,
		Takeover:  TakeoverNone,
		Usage:     []string{},
	})
}

// openCodeLevels holds OpenCode's permission settings by level: at level 1 it
// edits nothing and runs nothing, at level 3 it asks nothing.
var openCodeLevels = map[int]map[string]string{
	1: {"edit": "deny", "bash": "deny"},
	3: {"edit": "allow", "bash": "allow", "webfetch": "allow", "external_directory": "allow", "doom_loop": "allow"},
}

var openCodeCategories = map[string]string{"bash": "command", "edit": "file", "webfetch": "network"}

type openCodeDriver struct{}

func (openCodeDriver) Models(ctx context.Context) []proto.Model {
	models := []proto.Model{}
	openCodeServer(ctx, func(get func(path string) ([]byte, error)) {
		if data, err := get("/api/model"); err == nil {
			models = openCodeModels(data)
		}
	})
	return models
}

// openCodeServer gives use the GET of a server of its own, started in no
// folder, which the end of the probe time stops.
func openCodeServer(ctx context.Context, use func(get func(path string) ([]byte, error))) {
	ctx, cancel := context.WithTimeout(ctx, agtProbeSeconds*time.Second)
	defer cancel()
	p := &openCodeProcess{password: rand.Text()}
	out, wait, err := serve(ctx, "", []string{"OPENCODE_SERVER_PASSWORD=" + p.password}, "opencode", "serve", "--hostname", "127.0.0.1", "--port", "0")
	if err != nil {
		return
	}
	defer func() { cancel(); wait() }()
	p.base = serverAddress(out)
	use(func(path string) ([]byte, error) { return p.call(http.MethodGet, path, nil) })
}

type openCodeModel struct {
	ProviderID, ModelID, Name string
	Variants                  []struct{ ID string }
}

func openCodeModels(data []byte) []proto.Model {
	var reply struct{ Data []openCodeModel }
	if json.Unmarshal(data, &reply) != nil || reply.Data == nil {
		return []proto.Model{}
	}
	models := []proto.Model{}
	for _, m := range reply.Data {
		model, ok := m.listed()
		if !ok {
			return []proto.Model{}
		}
		models = append(models, model)
	}
	return models
}

func (m openCodeModel) listed() (proto.Model, bool) {
	if m.ProviderID == "" || m.ModelID == "" || m.Variants == nil {
		return proto.Model{}, false
	}
	efforts := []string{}
	for _, variant := range m.Variants {
		if variant.ID == "" {
			return proto.Model{}, false
		}
		efforts = append(efforts, variant.ID)
	}
	id := m.ProviderID + "/" + m.ModelID
	return proto.Model{Model: id, Name: cmp.Or(m.Name, id), Efforts: efforts, Tier: modelTier(id)}, true
}

func (openCodeDriver) Interactive(Launch) ([]string, string, error) {
	return nil, "", fmt.Errorf("opencode: no terminal takeover")
}

func (openCodeDriver) Start(ctx context.Context, l Launch) (Process, error) {
	if _, ok := openCodeLevels[l.Level]; !ok {
		return nil, fmt.Errorf("opencode: level %d is not declared", l.Level)
	}
	p := &openCodeProcess{launch: l, password: rand.Text(), events: make(chan Event, 16), ready: make(chan struct{})}
	ctx, p.stop = context.WithCancel(ctx)
	out, wait, err := serve(ctx, l.Folder, openCodeEnv(l, p.password), "opencode", "serve", "--hostname", "127.0.0.1", "--port", "0")
	if err != nil {
		p.stop()
		return nil, err
	}
	go p.run(out, func() {
		p.stop()
		wait()
	})
	return p, nil
}

// openCodeEnv is the environment of the server of l: the password, the
// level's permission settings and the solodawn MCP server, and without
// RepoConfig the switch that keeps the repository's configuration out.
func openCodeEnv(l Launch, password string) []string {
	config := marshalWire(map[string]any{
		"permission": openCodeLevels[l.Level],
		"mcp":        map[string]any{"solodawn": map[string]any{"type": "local", "command": l.Shim}},
	})
	env := []string{"OPENCODE_SERVER_PASSWORD=" + password, "OPENCODE_CONFIG_CONTENT=" + config}
	if !l.RepoConfig {
		env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=1")
	}
	return env
}

// openCodeProcess drives one opencode server. base and session are written
// before ready is closed and only read after it.
type openCodeProcess struct {
	launch   Launch
	password string
	events   chan Event
	ready    chan struct{}
	stop     context.CancelFunc
	base     string
	session  string
	status   string // how the running turn ends, from session.error
	errText  string // error text carried by a failed turn
	startErr error
}

type openCodeEvent struct {
	Type       string
	Properties struct {
		SessionID  string `json:"sessionID"`
		ID         string
		Permission string
		Patterns   []string
		Error      struct {
			Name string
			Data struct{ Message string }
		}
	}
}

func (p *openCodeProcess) Events() <-chan Event { return p.events }

func (p *openCodeProcess) Turn(text string) error {
	<-p.ready
	if p.startErr != nil {
		return p.startErr
	}
	return p.post("/session/"+p.session+"/prompt_async", map[string]any{
		"system": p.launch.Instructions,
		"parts":  []map[string]string{{"type": "text", "text": text}},
	})
}

func (p *openCodeProcess) Interrupt() error {
	<-p.ready
	return p.post("/session/"+p.session+"/abort", struct{}{})
}

func (p *openCodeProcess) Answer(ask string, allow bool) error {
	<-p.ready
	return p.post("/permission/"+ask+"/reply", map[string]string{"reply": map[bool]string{true: "once", false: "reject"}[allow]})
}

// Close stops the server: it does not exit when its input closes.
func (p *openCodeProcess) Close() error {
	p.stop()
	return nil
}

func (p *openCodeProcess) post(path string, body any) error {
	_, err := p.call(http.MethodPost, path, body)
	return err
}

func (p *openCodeProcess) request(method, path string, body any) (*http.Response, error) {
	req, err := http.NewRequest(method, p.base+path, wireBody(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("opencode", p.password)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func (p *openCodeProcess) call(method, path string, body any) ([]byte, error) {
	res, err := p.request(method, path, body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if res.StatusCode >= http.StatusMultipleChoices {
		err = fmt.Errorf("opencode: %s %s: %s %s", method, path, res.Status, data)
	}
	return data, err
}

// run finds the server's address, then reads its event stream until the
// server exits.
func (p *openCodeProcess) run(out io.Reader, wait func()) {
	defer close(p.events)
	defer wait()
	p.base = serverAddress(out)
	stream, err := p.request(http.MethodGet, "/event", nil)
	if err == nil {
		defer stream.Body.Close()
		err = p.open()
	}
	if err == nil {
		err = p.selectModel()
	}
	p.startErr = err
	close(p.ready)
	if err != nil {
		return
	}
	p.events <- Event{Kind: Ready, Session: p.session}
	readSSE(stream.Body, func(data []byte) {
		var e openCodeEvent
		if json.Unmarshal(data, &e) == nil {
			p.handle(e, data)
		}
	})
}

func (p *openCodeProcess) selectModel() error {
	if p.launch.Model == "" {
		return nil
	}
	provider, id, _ := strings.Cut(p.launch.Model, "/")
	model := map[string]string{"id": id, "providerID": provider}
	if p.launch.Effort != "" {
		model["variant"] = p.launch.Effort
	}
	return p.post("/api/session/"+p.session+"/model", map[string]any{"model": model})
}

// open resumes the launch's session, or creates one.
func (p *openCodeProcess) open() error {
	if p.launch.Session != "" {
		p.session = p.launch.Session
		_, err := p.call(http.MethodGet, "/session/"+p.session, nil)
		if err != nil {
			p.events <- Event{Kind: NoSession, Text: err.Error()}
		}
		return err
	}
	data, err := p.call(http.MethodPost, "/session", struct{}{})
	var s struct{ ID string }
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	p.session = s.ID
	return err
}

func (p *openCodeProcess) handle(e openCodeEvent, raw json.RawMessage) {
	props := e.Properties
	switch e.Type {
	case "message.part.updated":
		p.events <- Event{Kind: Active}
	case "permission.asked":
		p.events <- Event{Kind: Asked, Ask: &Ask{
			ID:       props.ID,
			Tool:     props.Permission,
			Category: cmp.Or(openCodeCategories[props.Permission], "other"),
			Command:  strings.Join(props.Patterns, " "),
			Input:    raw,
		}}
	case "session.error":
		p.failed(props.SessionID, props.Error.Name, props.Error.Data.Message)
	case "session.idle":
		p.idle(props.SessionID)
	}
}

// failed records how the turn ends. An error of another session, such as a
// child session, is not the turn's; an error without a session is.
func (p *openCodeProcess) failed(session, name, text string) {
	if cmp.Or(session, p.session) != p.session {
		return
	}
	aborted := name == "MessageAbortedError"
	p.status = map[bool]string{true: "interrupted", false: "failed"}[aborted]
	if !aborted {
		p.errText = cmp.Or(strings.TrimSpace(text), name)
	}
	if name == "ProviderAuthError" {
		p.events <- Event{Kind: LoginInvalid, Text: text}
	}
}

func (p *openCodeProcess) idle(session string) {
	if session == p.session {
		p.events <- Event{Kind: TurnEnded, Status: cmp.Or(p.status, "completed"), Text: p.errText}
		p.status = ""
		p.errText = ""
	}
}
