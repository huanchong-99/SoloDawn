//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestA10ProfilesLiveFeature(t *testing.T) {
	if os.Getenv("SOLODAWN_LIVE") != "1" {
		t.Skip("set SOLODAWN_LIVE=1 with real CLIs signed in; see a10_profiles_live.md")
	}
	input := json.NewDecoder(os.Stdin)
	input.DisallowUnknownFields()
	for _, agent := range []string{"claude", "codex", "opencode", "qwen"} {
		t.Run(agent, func(t *testing.T) {
			if _, err := exec.LookPath(agent); err != nil {
				t.Skipf("real CLI %s is absent from PATH: %v", agent, err)
			}
			suite := godog.TestSuite{
				ScenarioInitializer: func(sc *godog.ScenarioContext) { a10LiveSteps(sc, t, agent, input) },
				Options:             &godog.Options{Format: "pretty", Paths: []string{"a10_profiles.feature"}, Tags: "@live", Strict: true, TestingT: t},
			}
			if suite.Run() != 0 {
				t.Fatal("A10 live profile scenarios failed")
			}
		})
	}
}

type a10LiveChoice struct {
	Model         string `json:"model"`
	Effort        string `json:"effort"`
	ReportedModel string `json:"reportedModel"`
}

type a10LivePickerModel struct {
	Model   string   `json:"model"`
	Name    string   `json:"name"`
	Efforts []string `json:"efforts"`
	Fast    bool     `json:"fast"`
}

type a10LiveWorld struct {
	*choiceWorld
	t       *testing.T
	agent   string
	input   *json.Decoder
	cleanup func(func())
	models  []proto.Model
	choice  a10LiveChoice
}

func a10LiveSteps(sc *godog.ScenarioContext, t *testing.T, agent string, input *json.Decoder) {
	w := &a10LiveWorld{t: t, agent: agent, input: input, cleanup: afterScenario(sc)}
	sc.Step(`^a host with each real CLI signed in$`, w.setup)
	sc.Step(`^the host reads its agents$`, w.read)
	sc.Step(`^the models of each agent are what the CLI's own picker shows$`, w.picker)
	sc.Step(`^host.join adds each agent with a model from its list$`, w.joinChoice)
	sc.Step(`^the model each CLI reports is the one chosen$`, w.reported)
}

func (w *a10LiveWorld) setup() error {
	caps := host.DriverCaps
	host.DriverCaps = map[string]host.Capabilities{w.agent: caps[w.agent]}
	w.cleanup(func() { host.DriverCaps = caps })
	w.choiceWorld = &choiceWorld{profiles: map[string]proto.Profile{}, members: map[string]string{}}
	other := map[string]string{"claude": "codex", "codex": "claude", "opencode": "claude", "qwen": "claude"}[w.agent]
	if err := w.initHostOnDisk(w.agent, other, "home-remote", w.cleanup); err != nil {
		return err
	}
	w.cleanup(w.h.Settle)
	if err := w.db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("UPDATE agent SET account=? WHERE agent=?", caps[w.agent].Accounts[0], w.agent)
		return err
	}); err != nil {
		return err
	}
	return w.attachHome()
}

func (w *a10LiveWorld) attachHome() error {
	if err := w.createRemoteGroup("home-remote", "Website"); err != nil {
		return err
	}
	w.home = &choiceHome{joinScenarioHome: w.remoteHome, notices: make(chan json.RawMessage, 64)}
	w.h.Attach("home-remote", w.home)
	_, _, err := attachMCPSocket(w.t, w.cleanup, w.h, w.root)
	return err
}

func (w *a10LiveWorld) read() error {
	ctx, cancel := context.WithTimeout(w.t.Context(), time.Minute)
	defer cancel()
	agents, err := w.h.Agents(ctx, true)
	if err != nil {
		return err
	}
	if len(agents) != 1 || agents[0].Agent != w.agent {
		return fmt.Errorf("discovered agents = %+v; want %s", agents, w.agent)
	}
	w.models = agents[0].Models
	return nil
}

func (w *a10LiveWorld) observe(prompt string, target any) error {
	fmt.Fprintf(os.Stdout, "\n[%s] %s\n", w.agent, prompt)
	if err := w.input.Decode(target); err != nil {
		return fmt.Errorf("live observation: %w; run the compiled test with stdin as documented in a10_profiles_live.md", err)
	}
	return nil
}

func (w *a10LiveWorld) picker() error {
	var want []a10LivePickerModel
	if err := w.observe("Inspect the native picker. Enter its ordered models as JSON [{\"model\":\"id\",\"name\":\"label\",\"efforts\":[],\"fast\":false}] (use [] for no listing).", &want); err != nil {
		return err
	}
	got := make([]a10LivePickerModel, 0, len(w.models))
	for _, model := range w.models {
		got = append(got, a10LivePickerModel{model.Model, model.Name, model.Efforts, bool(model.Fast)})
	}
	return hostModelsEqual(got, want)
}

func (w *a10LiveWorld) joinChoice() error {
	if err := w.read(); err != nil {
		return err
	}
	if err := w.observe("Choose a native-picker model and note its expected resolved model before launch. Enter JSON {\"model\":\"id\",\"effort\":\"\",\"reportedModel\":\"resolved-id\"} (Qwen accepts a manual model id).", &w.choice); err != nil {
		return err
	}
	if err := a10ValidateLiveChoice(w.agent, w.models, w.choice); err != nil {
		return err
	}
	return w.joinProfile(w.agent, "Live", w.choice.Model, w.choice.Effort, "standard")
}

func a10ValidateLiveChoice(agent string, models []proto.Model, choice a10LiveChoice) error {
	if choice.Model == "" || choice.ReportedModel == "" {
		return fmt.Errorf("model and expected reportedModel must be non-empty")
	}
	if agent != "qwen" && !slices.ContainsFunc(models, func(model proto.Model) bool { return model.Model == choice.Model }) {
		return fmt.Errorf("chosen model %q is absent from %s's discovered list", choice.Model, agent)
	}
	return nil
}

func (w *a10LiveWorld) reported() error {
	ctx, cancel := context.WithTimeout(w.t.Context(), 5*time.Minute)
	defer cancel()
	profile := w.profiles["Live"]
	token := w.h.Sessions.Admit(host.SessionKey{Group: string(joinGroupIDs["Website"]), Member: w.members["Live"]}, 1, false)
	defer w.h.Sessions.Forget(token)
	folder := filepath.Join(w.root, "remotes", "website")
	p, err := host.Drivers[w.agent].Start(ctx, host.Launch{Model: profile.Model, Effort: profile.Effort, Folder: folder, Level: 1, Shim: append(slices.Clone(w.h.Shim), "--token", token)})
	if err != nil {
		return err
	}
	defer p.Close()
	return w.checkReport(ctx, p, folder)
}

func (w *a10LiveWorld) checkReport(ctx context.Context, p host.Process, folder string) error {
	if err := p.Turn("Reply with ready. Do not edit files, run commands, or call tools."); err != nil {
		return err
	}
	ready, err := a10AwaitLiveEvent(ctx, p.Events(), host.Ready)
	if err != nil {
		return err
	}
	if ready.Session == "" {
		return fmt.Errorf("CLI ready event has no session id")
	}
	if _, err := a10AwaitLiveEvent(ctx, p.Events(), host.TurnEnded); err != nil {
		return err
	}
	var reported string
	if err := w.observe(fmt.Sprintf("Inspect this new CLI session's own model report: session=%q, folder=%q. Enter the reported resolved model as a JSON string (not the launch arguments or the AI's prose answer).", ready.Session, folder), &reported); err != nil {
		return err
	}
	return hostModelsEqual(reported, w.choice.ReportedModel)
}

func a10AwaitLiveEvent(ctx context.Context, events <-chan host.Event, want host.EventKind) (host.Event, error) {
	for {
		select {
		case event, open := <-events:
			done, err := a10LiveEventResult(event, open, want)
			if done {
				return event, err
			}
		case <-ctx.Done():
			return host.Event{}, ctx.Err()
		}
	}
}

func a10LiveEventResult(event host.Event, open bool, want host.EventKind) (bool, error) {
	if !open {
		return true, fmt.Errorf("CLI exited before %s", want)
	}
	switch event.Kind {
	case host.LoginInvalid, host.NoSession:
		return true, fmt.Errorf("CLI %s: %s", event.Kind, event.Text)
	case host.TurnEnded:
		if event.Status != "completed" {
			return true, fmt.Errorf("CLI turn ended %q: %s", event.Status, event.Text)
		}
	}
	return event.Kind == want, nil
}
