//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() { stepSets["host_models"] = hostModelsSteps }

func TestHostModelsFeature(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command("go", "build", "-o", dir+string(os.PathSeparator), "../internal/host/fakecli/claude", "../internal/host/fakecli/codex").CombinedOutput()
	if err != nil {
		t.Fatalf("build fake CLIs: %s: %v", out, err)
	}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) { hostModelsSteps(sc, fixtures{t: t, fakeCLIs: dir}) },
		Options:             &godog.Options{Format: "pretty", Paths: []string{"host_models.feature"}, Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("host model scenarios failed")
	}
}

type hostModelsWorld struct {
	dir          string
	now          time.Time
	sessions     *node.Sessions
	state, first proto.HostState
}

func hostModelsSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &hostModelsWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a model-discovery host with scripted Claude Code and Codex$`, func() error { return w.setup(f, cleanup) })
	sc.Step(`^a model-discovery host with scripted OpenCode and Qwen Code$`, func() error {
		server, err := serverProfileFixtures(f, cleanup)
		if err != nil {
			return err
		}
		return w.setup(server, cleanup)
	})
	sc.Step(`^the model-discovery host lists OpenCode variants and tiers and no Qwen models$`, w.serverListed)
	sc.Step(`^the OpenCode listing has an unknown shape$`, func() error {
		return w.script("opencode", `{"session":"models","steps":[{"dir":"http","msg":{"call":"GET /api/model","body":null,"res":{"models":[]}}},{"dir":"http","msg":{"call":"POST /never","body":{}}}]}`)
	})
	sc.Step(`^the model-discovery host receives host.info$`, func() error { return w.read(false) })
	sc.Step(`^the model-discovery host receives host.info with refresh$`, func() error { return w.read(true) })
	sc.Step(`^the model-discovery host lists the scripted ids, names, efforts, fast flags and tiers$`, w.listed)
	sc.Step(`^the scripted codex model listing fails$`, func() error { return w.script("codex", `{"session":"models","exit":1}`) })
	sc.Step(`^the codex models are empty and the other host.info fields are unchanged$`, func() error {
		want := w.first
		want.Agents = append([]proto.HostAgent{}, want.Agents...)
		want.Agents[1].Models = []proto.Model{}
		return hostModelsEqual(w.state, want)
	})
	sc.Step(`^both model listings have an unknown shape$`, w.unknown)
	sc.Step(`^both model lists are empty$`, func() error {
		return hostModelsEqual([][]proto.Model{w.state.Agents[0].Models, w.state.Agents[1].Models}, [][]proto.Model{{}, {}})
	})
	sc.Step(`^the scripted claude list gains a model$`, w.gain)
	sc.Step(`^(\d+) seconds pass for model discovery$`, func(seconds int) { w.now = w.now.Add(time.Duration(seconds) * time.Second) })
	sc.Step(`^the model-discovery host answers its old list$`, func() error { return hostModelsEqual(w.state, w.first) })
	sc.Step(`^the model-discovery host answers the new list$`, w.newList)
}

func hostModelsEqual(got, want any) error {
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("got %#v, want %#v", got, want)
	}
	return nil
}

func (w *hostModelsWorld) setup(f fixtures, cleanup func(func())) error {
	dir, err := os.MkdirTemp("", "solodawn-models-*")
	if err != nil {
		return err
	}
	w.dir = dir
	cleanup(func() { os.RemoveAll(dir) })
	for key, value := range map[string]string{
		"PATH":                          hostAgentsIsolatedPath(f.fakeCLIs),
		"SOLODAWN_FAKE_CLAUDE_SCRIPT":   filepath.Join(dir, "claude.json"),
		"SOLODAWN_FAKE_CODEX_SCRIPT":    filepath.Join(dir, "codex.json"),
		"SOLODAWN_FAKE_OPENCODE_SCRIPT": filepath.Join(dir, "opencode.json"),
		"SOLODAWN_FAKECLI_LOG":          filepath.Join(dir, "log"),
	} {
		previous, had := os.LookupEnv(key)
		if err := os.Setenv(key, value); err != nil {
			return err
		}
		cleanup(func() { restoreVar(key, previous, had) })
	}
	return w.open(cleanup)
}

func (w *hostModelsWorld) open(cleanup func(func())) error {
	if err := w.copyScripts(); err != nil {
		return err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, func() time.Time { return w.now })
	if err != nil {
		return err
	}
	cleanup(h.Settle)
	logs, err := node.OpenLogs(db, key.ID())
	if err != nil {
		return err
	}
	w.sessions = node.NewSessions(node.SessionConfig{Store: db, Host: h, Logs: logs})
	node.NewSurface(node.SurfaceConfig{ID: key.ID(), OS: "test", Version: "test", Store: db, Host: h, Sessions: w.sessions, Agents: h.Agents})
	return nil
}

func (w *hostModelsWorld) copyScripts() error {
	for _, agent := range []string{"claude", "codex", "opencode"} {
		raw, err := os.ReadFile(filepath.Join("..", "internal", "host", "fakecli", "testdata", agent+"-models.json"))
		if err != nil {
			return err
		}
		if err := w.script(agent, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func (w *hostModelsWorld) script(agent, script string) error {
	return os.WriteFile(filepath.Join(w.dir, agent+".json"), []byte(script), 0600)
}

func (w *hostModelsWorld) read(refresh bool) error {
	data, _ := json.Marshal(proto.HostInfoRequest{Request: proto.Request{Type: "host.info", ID: 1}, Refresh: refresh})
	state, err := w.sessions.Requests["host.info"].Run(context.Background(), nil, nil, "", 1, data)
	if err != nil {
		return err
	}
	w.state = state.(proto.HostState)
	if w.first.Agents == nil {
		w.first = w.state
	}
	return nil
}

func (w *hostModelsWorld) listed() error {
	want := [][]proto.Model{
		{{Model: "default", Name: "Default", Efforts: []string{"low", "high"}, Fast: true, Tier: "strong"}, {Model: "haiku", Name: "Haiku", Efforts: []string{}, Tier: "light"}},
		{{Model: "gpt-6-mini", Name: "Mini", Efforts: []string{"medium", "high"}, Fast: true, Tier: "light"}, {Model: "gpt-6-luna", Name: "Luna", Efforts: []string{}, Tier: "standard"}},
	}
	return hostModelsEqual([][]proto.Model{w.state.Agents[0].Models, w.state.Agents[1].Models}, want)
}

func (w *hostModelsWorld) unknown() error {
	if err := w.script("claude", `{"session":"models","steps":[{"dir":"stdout","msg":{"type":"control_response","response":{"subtype":"success","request_id":"models","response":{"models":{}}}}}]}`); err != nil {
		return err
	}
	return w.script("codex", `{"session":"models","steps":[{"dir":"stdin","msg":{"method":"initialize"}},{"dir":"stdout","msg":{"id":1,"result":{}}},{"dir":"stdin","msg":{"method":"initialized"}},{"dir":"stdin","msg":{"method":"model/list"}},{"dir":"stdout","msg":{"id":2,"result":{"data":{}}}}]}`)
}

func (w *hostModelsWorld) gain() error {
	raw, err := os.ReadFile(filepath.Join(w.dir, "claude.json"))
	if err != nil {
		return err
	}
	text := strings.Replace(string(raw), `"supportsEffort":false}`, `"supportsEffort":false},{"value":"gemini-3-pro","resolvedModel":"gemini-3-pro","displayName":"Pro"}`, 1)
	return w.script("claude", text)
}

func (w *hostModelsWorld) newList() error {
	want := w.first
	want.Agents = append([]proto.HostAgent{}, want.Agents...)
	want.Agents[0].Models = append(append([]proto.Model{}, want.Agents[0].Models...), proto.Model{Model: "gemini-3-pro", Name: "Pro", Efforts: []string{}, Tier: "standard"})
	return hostModelsEqual(w.state, want)
}
