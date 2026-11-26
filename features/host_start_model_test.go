//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["host_start_model"] = hostStartModelSteps }

func TestHostStartModelFeature(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command("go", "build", "-o", dir+string(os.PathSeparator), "../internal/host/fakecli/claude", "../internal/host/fakecli/codex").CombinedOutput()
	if err != nil {
		t.Fatalf("build fake CLIs: %s: %v", out, err)
	}
	suite := godog.TestSuite{ScenarioInitializer: func(sc *godog.ScenarioContext) { hostStartModelSteps(sc, fixtures{t: t, fakeCLIs: dir}) }, Options: &godog.Options{Format: "pretty", Paths: []string{"host_start_model.feature"}, Strict: true, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("host model choice scenarios failed")
	}
}

type choiceDriver struct {
	host.Driver
	mu       sync.Mutex
	launches []host.Launch
}

func (d *choiceDriver) Start(ctx context.Context, l host.Launch) (host.Process, error) {
	d.mu.Lock()
	d.launches = append(d.launches, l)
	d.mu.Unlock()
	return d.Driver.Start(ctx, l)
}

func (d *choiceDriver) snapshot() []host.Launch {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]host.Launch{}, d.launches...)
}

type choiceHome struct {
	*joinScenarioHome
	mu      sync.Mutex
	adds    []proto.MemberAddAI
	notices chan json.RawMessage
}

func (h *choiceHome) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	frame := build(11)
	if cmd, ok := frame.(proto.Cmd); ok {
		return h.command(ctx, frame, cmd)
	}
	return h.joinScenarioHome.Request(ctx, func(int64) any { return frame })
}

func (h *choiceHome) command(ctx context.Context, frame any, cmd proto.Cmd) (json.RawMessage, error) {
	var header struct {
		Type string `json:"t"`
		Op   string `json:"op"`
	}
	json.Unmarshal([]byte(cmd.Env.Payload), &header)
	if header.Type != "member" || header.Op == "status" {
		h.notices <- json.RawMessage(cmd.Env.Payload)
		return json.RawMessage(accJSON(proto.CmdResult{Success: proto.Success{Re: 11, OK: true}})), nil
	}
	answer, err := h.joinScenarioHome.Request(ctx, func(int64) any { return frame })
	if header.Op == "add_ai" && err == nil {
		h.record(cmd.Env.Payload)
	}
	return answer, err
}

func (h *choiceHome) record(payload string) {
	var add proto.MemberAddAI
	json.Unmarshal([]byte(payload), &add)
	h.mu.Lock()
	h.adds = append(h.adds, add)
	h.mu.Unlock()
}

type choiceWorld struct {
	joinScenarioWorld
	home     *choiceHome
	drivers  map[string]*choiceDriver
	profiles map[string]proto.Profile
	members  map[string]string
	key      host.SessionKey
	cliID    string
}

func hostStartModelSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &choiceWorld{profiles: map[string]proto.Profile{}, members: map[string]string{}, drivers: map[string]*choiceDriver{}}
	cleanup := afterScenario(sc)
	sc.Step(`^a model-choice host with fake Claude Code and Codex$`, func() error { return w.setup(f, cleanup, "claude", "codex") })
	sc.Step(`^a model-choice host with fake OpenCode and Qwen Code$`, func() error {
		server, err := serverProfileFixtures(f, cleanup)
		if err != nil {
			return err
		}
		return w.setup(server, cleanup, "opencode", "qwen")
	})
	sc.Step(`^the model-choice host joins "([^"]*)" as "([^"]*)" with model "([^"]*)", effort "([^"]*)" and tier "([^"]*)"$`, w.joinProfile)
	sc.Step(`^both model-choice profiles are forwarded unchanged to the home$`, w.forwarded)
	sc.Step(`^model-choice host.info lists both members under claude$`, w.hostInfo)
	sc.Step(`^model-choice leave without member is bad_request$`, func() error {
		err := w.leave("claude", "Website")
		var coded *host.Error
		if !errors.As(err, &coded) {
			return fmt.Errorf("leave: %v", err)
		}
		return hostModelsEqual(coded.Code, "bad_request")
	})
	sc.Step(`^model-choice leave selects "([^"]*)"$`, w.leaveMember)
	sc.Step(`^only model-choice member "([^"]*)" remains$`, func(name string) error {
		all := w.h.Sessions.All()
		if len(all) != 1 {
			return fmt.Errorf("sessions: %+v", all)
		}
		return hostModelsEqual(all[0].Member, w.members[name])
	})
	sc.Step(`^model-choice member "([^"]*)" starts a turn$`, func(name string) error { return w.run(name, false, false) })
	sc.Step(`^the model-choice process received model "([^"]*)" and effort "([^"]*)"$`, w.startedWith)
	sc.Step(`^the home publishes model "([^"]*)" and effort "([^"]*)" for model-choice member "([^"]*)"$`, w.publish)
	sc.Step(`^the model-choice running process is unchanged$`, w.unchanged)
	sc.Step(`^model-choice member "([^"]*)" resumes in a new process$`, func(name string) error { return w.run(name, true, false) })
	sc.Step(`^the model-choice session retained its CLI id$`, w.keptID)
	sc.Step(`^model-choice member "([^"]*)" starts a task whose model is refused$`, func(name string) error { return w.run(name, false, true) })
	sc.Step(`^model-choice reports a crash and task fail with the original session id$`, w.crashed)
}

func (w *choiceWorld) setup(f fixtures, cleanup func(func()), first, second string) error {
	if err := w.initHostOnDisk(first, second, "home-remote", cleanup); err != nil {
		return err
	}
	cleanup(w.h.Settle)
	if err := w.createRemoteGroup("home-remote", "Website"); err != nil {
		return err
	}
	w.h.Shim = []string{"solodawn", "mcp"}
	w.home = &choiceHome{joinScenarioHome: w.remoteHome, notices: make(chan json.RawMessage, 64)}
	w.h.Connect = func(_ context.Context, device string) { w.h.Attach(device, w.home) }
	w.h.Attach("home-remote", w.home)
	for _, agent := range []string{first, second} {
		original := host.Drivers[agent]
		w.drivers[agent] = &choiceDriver{Driver: original}
		host.Drivers[agent] = w.drivers[agent]
		cleanup(func() { host.Drivers[agent] = original })
	}
	old, had := os.LookupEnv("PATH")
	if err := os.Setenv("PATH", f.fakeCLIs+string(os.PathListSeparator)+old); err != nil {
		return err
	}
	cleanup(func() { restoreVar("PATH", old, had) })
	log, logHad := os.LookupEnv("SOLODAWN_FAKECLI_LOG")
	if err := os.Setenv("SOLODAWN_FAKECLI_LOG", filepath.Join(w.root, "fake-cli.log")); err != nil {
		return err
	}
	cleanup(func() { restoreVar("SOLODAWN_FAKECLI_LOG", log, logHad) })
	return nil
}

func (w *choiceWorld) joinProfile(agent, name, model, effort, tier string) error {
	profile := proto.Profile{Model: model, Effort: effort, Tier: tier, Skills: "Go"}
	workspace, err := aiWorkspace(w.root, joinGroupIDs["Website"], name)
	if err != nil {
		return err
	}
	p := proto.HostJoin{HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.nextNonce(), Timestamp: time.Now().UnixMilli(), To: w.h.Key.ID()}, Group: joinGroupIDs["Website"], Home: "home-remote", Agent: agent, Name: name, Workspace: workspace, ProfileSet: proto.ProfileSet{Model: &model, Effort: &effort, Tier: &tier, Skills: &profile.Skills}}
	if _, err := host.Surface["host.join"](context.Background(), w.h, nil, "owner-browser", json.RawMessage(accJSON(p))); err != nil {
		return err
	}
	w.home.mu.Lock()
	add := w.home.adds[len(w.home.adds)-1]
	w.home.mu.Unlock()
	w.profiles[name], w.members[name] = profile, string(add.ID)
	m := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: string(add.ID)}, Group: string(p.Group), Kind: "ai", Name: name}, CLI: agent, Host: w.h.Key.ID(), Allow: []string{}, Approvers: []string{}, Profile: profile}
	w.h.Rows.Put(string(p.Group), "member", string(add.ID), json.RawMessage(accJSON(m)))
	return nil
}

func (w *choiceWorld) forwarded() error {
	w.home.mu.Lock()
	defer w.home.mu.Unlock()
	if len(w.home.adds) != 2 {
		return fmt.Errorf("adds: %v", w.home.adds)
	}
	for _, add := range w.home.adds {
		got := proto.Profile{Model: *add.Model, Effort: *add.Effort, Tier: *add.Tier, Skills: *add.Skills}
		if err := hostModelsEqual(got, w.profiles[add.Name]); err != nil {
			return err
		}
	}
	return nil
}

func (w *choiceWorld) hostInfo() error {
	logs, err := node.OpenLogs(w.db, w.h.Key.ID())
	if err != nil {
		return err
	}
	sessions := node.NewSessions(node.SessionConfig{Store: w.db, Logs: logs, Host: w.h})
	surface := node.NewSurface(node.SurfaceConfig{Store: w.db, Host: w.h, Sessions: sessions, Agents: w.h.Agents})
	state, err := surface.HostState(context.Background())
	if err != nil {
		return err
	}
	for _, agent := range state.Agents {
		if agent.Agent == "claude" {
			ids := map[string]bool{}
			for _, group := range agent.Groups {
				ids[string(group.Member)] = true
			}
			return hostModelsEqual(ids, map[string]bool{w.members["One"]: true, w.members["Two"]: true})
		}
	}
	return fmt.Errorf("claude missing in host.info")
}

func (w *choiceWorld) leaveMember(name string) error {
	p := proto.HostLeave{HostCommand: proto.HostCommand{Type: "host.leave", Nonce: w.nextNonce(), Timestamp: time.Now().UnixMilli(), To: w.h.Key.ID()}, Group: joinGroupIDs["Website"], Agent: "claude", Member: proto.ID(w.members[name])}
	_, err := host.Surface["host.leave"](context.Background(), w.h, nil, "owner-browser", json.RawMessage(accJSON(p)))
	return err
}

func (w *choiceWorld) publish(model, effort, name string) error {
	p := w.profiles[name]
	p.Model, p.Effort = model, effort
	w.profiles[name] = p
	raw, _ := w.h.Rows.Get(string(joinGroupIDs["Website"]), "member", w.members[name])
	var row proto.AIMemberRow
	if err := proto.Decode(raw, &row); err != nil {
		return err
	}
	row.Profile = p
	w.h.Rows.Put(row.Group, "member", row.ID, json.RawMessage(accJSON(row)))
	return nil
}

func (w *choiceWorld) current() (host.Session, *choiceDriver) {
	s, _ := w.h.Sessions.Get(w.key)
	return s, w.drivers[s.Agent]
}

func (w *choiceWorld) startedWith(model, effort string) error {
	_, d := w.current()
	launches := d.snapshot()
	l := launches[len(launches)-1]
	return hostModelsEqual([]string{l.Model, l.Effort}, []string{model, effort})
}

func (w *choiceWorld) unchanged() error {
	s, d := w.current()
	return hostModelsEqual([]any{len(d.snapshot()), s.State}, []any{1, host.Waiting})
}

func (w *choiceWorld) keptID() error {
	s, _ := w.current()
	return hostModelsEqual(s.CLISession, w.cliID)
}

func (w *choiceWorld) notice() (json.RawMessage, error) {
	wt := newWait(10*time.Second, "the model-choice host notice", nil)
	defer wt.stop()
	return recvWait(wt, w.home.notices)
}

func (w *choiceWorld) run(name string, resume, refused bool) error {
	if err := w.prepareRun(name, resume, refused); err != nil {
		return err
	}
	s, _ := w.current()
	restore, err := w.setScript(s, w.profiles[name], refused)
	if err != nil {
		return err
	}
	defer restore()
	turnErr := host.Runner.Turn(context.Background(), w.h, w.key, "run", host.Turn{Kind: host.ItemsTurn, Thread: w.key.Task})
	if refused {
		return w.status("working")
	}
	if turnErr != nil {
		return turnErr
	}
	return w.completedRun(resume)
}

func (w *choiceWorld) prepareRun(name string, resume, refused bool) error {
	w.key = host.SessionKey{Group: string(joinGroupIDs["Website"]), Member: w.members[name]}
	if refused {
		if err := w.taskSession(); err != nil {
			return err
		}
	}
	if resume {
		host.Runner.Close(w.h, w.key)
		return w.status("dormant")
	}
	return nil
}

func (w *choiceWorld) setScript(s host.Session, profile proto.Profile, refused bool) (func(), error) {
	path := filepath.Join(w.root, s.Agent+"-choice.json")
	script := choiceScript(s, refused)
	switch s.Agent {
	case "opencode", "qwen":
		script = serverChoiceScript(s, profile, refused)
	}
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		return nil, err
	}
	key := "SOLODAWN_FAKE_" + strings.ToUpper(s.Agent) + "_SCRIPT"
	previous, had := os.LookupEnv(key)
	if err := os.Setenv(key, path); err != nil {
		return nil, err
	}
	return func() { restoreVar(key, previous, had) }, nil
}

func (w *choiceWorld) completedRun(resume bool) error {
	if err := w.status("working"); err != nil {
		return err
	}
	if err := w.status("waiting"); err != nil {
		return err
	}
	if !resume {
		fresh, _ := w.current()
		w.cliID = fresh.CLISession
	}
	return nil
}

func (w *choiceWorld) status(want string) error {
	raw, err := w.notice()
	if err != nil {
		return err
	}
	var status proto.MemberStatus[proto.SystemCode]
	if err := proto.Decode(raw, &status); err != nil {
		return err
	}
	return hostModelsEqual(status.State, want)
}

func (w *choiceWorld) taskSession() error {
	s, _ := w.current()
	w.key.Task = hrTask1
	s.SessionKey, s.Kind, s.CLISession = w.key, host.TaskSession, "existing"
	if s.Agent == "qwen" {
		s.CLISession = ""
	}
	w.cliID = s.CLISession
	return w.h.Sessions.Create(s)
}

func choiceScript(s host.Session, refused bool) string {
	if s.Agent == "claude" {
		if refused {
			return `{"session":"existing","steps":[{"dir":"stdin","msg":{"type":"user"}}],"exit":1}`
		}
		return `{"session":"session","steps":[{"dir":"stdin","msg":{"type":"user"}},{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},{"dir":"stdout","msg":{"type":"result","subtype":"success","is_error":false}},{"dir":"eof"}]}`
	}
	result := `{"id":2,"result":{"thread":{"id":"session"}}}`
	if refused {
		result = `{"id":2,"error":{"code":-32600,"message":"model refused"}}`
	}
	steps := `{"session":"session","steps":[{"dir":"stdin","msg":{"method":"initialize"}},{"dir":"stdout","msg":{"id":1,"result":{}}},{"dir":"stdin","msg":{"method":"initialized"}},{"dir":"stdin","msg":{"id":2}},{"dir":"stdout","msg":` + result + `}`
	if !refused {
		steps += `,{"dir":"stdin","msg":{"method":"turn/start"}},{"dir":"stdout","msg":{"method":"turn/completed","params":{"threadId":"session","turn":{"id":"turn","status":"completed"}}}}`
	}
	return steps + `,{"dir":"eof"}]}`
}

func (w *choiceWorld) crashed() error {
	raw, err := w.notice()
	if err != nil {
		return err
	}
	if err := choiceCrashStatus(raw); err != nil {
		return err
	}
	raw, err = w.notice()
	if err != nil {
		return err
	}
	if err := choiceTaskFail(raw); err != nil {
		return err
	}
	if err := w.keptID(); err != nil {
		return err
	}
	s, d := w.current()
	return hostModelsEqual([]any{s.State, len(d.snapshot())}, []any{host.Dormant, 1})
}

func choiceCrashStatus(raw json.RawMessage) error {
	var status proto.MemberStatus[proto.TurnInterruptedEvent]
	if err := proto.Decode(raw, &status); err != nil {
		return err
	}
	return hostModelsEqual([]string{status.State, status.Event.Code, status.Event.Reason}, []string{"dormant", "turn_interrupted", "crash"})
}

func choiceTaskFail(raw json.RawMessage) error {
	var fail proto.TaskFail
	if err := proto.Decode(raw, &fail); err != nil {
		return err
	}
	return hostModelsEqual([]string{fail.Type, fail.Op, fail.Reason, string(fail.ID)}, []string{"task", "fail", "crash", hrTask1})
}
