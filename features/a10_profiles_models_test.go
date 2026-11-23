//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type a10ModelsWorld struct {
	hostModelsWorld
	f          fixtures
	cleanup    func(func())
	cluster    *nodetest.Cluster
	ctl        *a10ModelCtl
	ctx        context.Context
	persist    *s3KBWorld
	before     string
	disconnect func()
}

func a10ModelSteps(sc *godog.ScenarioContext, f fixtures, cleanup func(func())) *a10ModelsWorld {
	w := &a10ModelsWorld{f: f, cleanup: cleanup, hostModelsWorld: hostModelsWorld{now: time.UnixMilli(1763600000000)}}
	sc.Step(`^a host with fake CLIs "claude,codex,opencode,qwen", with two models except none for qwen$`, func() error { return w.setupModels(true) })
	sc.Step(`^a host with the fake CLI "claude" scripted to list two models$`, func() error { return w.setupModels(false) })
	sc.Step(`^the first listed models of fake claude and codex are scripted with fast$`, w.fast)
	sc.Step(`^the profiles host reads its agents$`, func() error { return w.read(false) })
	sc.Step(`^host.info lists under each agent but qwen its two models with id, name, levels and fast as scripted$`, w.listedAll)
	sc.Step(`^each listed model has the tier of the table for its id's words, else "standard"$`, w.listedAll)
	sc.Step(`^the models of qwen are \[\]$`, func() error { return hostModelsEqual(w.state.Agents[3].Models, []proto.Model{}) })
	sc.Step(`^the listing of the fake codex fails$`, func() error { return w.script("codex", `{"session":"models","exit":1}`) })
	sc.Step(`^the host reads its agents with refresh$`, func() error { return w.read(true) })
	sc.Step(`^the models of codex are \[\] and host.info is otherwise as before$`, w.failedCodex)
	sc.Step(`^the suggested tier of "gemini-3-pro" is "standard" and of "gpt-6-mini" is "light"$`, w.suggestions)
	sc.Step(`^the claude entry "default" has the tier of its resolvedModel$`, func() error { return hostModelsEqual(w.state.Agents[0].Models[0].Tier, "strong") })
	sc.Step(`^the browser's dialog for adding an AI offers the listed models and their levels$`, func() error { return hostModelsEqual(w.state.Agents[0].Models[:2], w.first.Agents[0].Models) })
	sc.Step(`^for an agent with no models the dialog offers two text fields$`, func() error { return hostModelsEqual(w.state.Agents[3].Models, []proto.Model{}) })
	sc.Step(`^the fake claude's list gains a model$`, w.gain)
	sc.Step(`^the host reads its agents within 60 seconds$`, func() error { w.now = w.now.Add(59 * time.Second); return w.read(false) })
	sc.Step(`^host.info answers the old list$`, func() error { return hostModelsEqual(w.state, w.first) })
	sc.Step(`^host.info answers the new list and a push carries it$`, w.refreshed)
	sc.Step(`^the refresh button on the browser's AI member panel shows the new model without a restart of the node$`, w.newList)
	sc.Step(`^the profiles cast with a member "Opus" whose model "opus" is suggested "strong"$`, w.persistence)
	sc.Step(`^a refresh suggests "standard" for "opus"$`, w.changedSuggestion)
	sc.Step(`^a refresh lists no "opus"$`, w.removedModel)
	sc.Step(`^a refresh lists no models$`, w.emptyModels)
	sc.Step(`^the node restarts$`, w.restart)
	sc.Step(`^the profile of "Opus" is unchanged byte for byte since the member set$`, w.unchangedProfile)
	sc.Step(`^the browser's AI member panel of "Opus" shows tier "light", "not listed" for its model and no suggestion$`, w.finalProfile)
	return w
}

func (w *a10ModelsWorld) setupModels(all bool) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	f, err := w.modelBins(all)
	if err != nil {
		return err
	}
	if err := hostAgentsLinkOrCopy(git, filepath.Join(f.fakeCLIs, hostAgentsExe("git"))); err != nil {
		return err
	}
	if err := w.hostModelsWorld.setup(f, w.cleanup); err != nil {
		return err
	}
	w.f = f
	w.cluster = nodetest.Start(scenarioTB{TB: f.t, cleanup: w.cleanup}, nodetest.Config{Root: w.dir, BinDir: f.fakeCLIs, Nodes: 1, Now: func() time.Time { return w.now }})
	return w.connect()
}

func (w *a10ModelsWorld) modelBins(all bool) (fixtures, error) {
	f := w.f
	var err error
	if !all {
		f.fakeCLIs, err = hostAgentsSetBins(w.f.fakeCLIs, "claude")
		w.cleanup(func() { os.RemoveAll(f.fakeCLIs) })
		return f, err
	}
	f, err = serverProfileFixtures(f, w.cleanup)
	if err != nil {
		return f, err
	}
	for _, agent := range []string{"claude", "codex"} {
		if err := hostAgentsLinkOrCopy(filepath.Join(w.f.fakeCLIs, hostAgentsExe(agent)), filepath.Join(f.fakeCLIs, hostAgentsExe(agent))); err != nil {
			return f, err
		}
	}
	return f, nil
}

func (w *a10ModelsWorld) connect() error {
	n := w.cluster.Nodes[0]
	logs, err := node.OpenLogs(n.Store, n.Key.ID())
	if err != nil {
		return err
	}
	w.sessions = node.NewSessions(node.SessionConfig{ID: n.Key.ID(), Store: n.Store, Logs: logs, Host: n.Host, Home: n.Home})
	node.NewSurface(node.SurfaceConfig{ID: n.Key.ID(), OS: "test", Version: "test", Store: n.Store, Host: n.Host, Sessions: w.sessions, Agents: n.Host.Agents})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.ctx = ctx
	w.ctl = &a10ModelCtl{homeFrameClient: newHomeFrameClient(128), in: make(chan json.RawMessage, 8)}
	done := make(chan struct{})
	go func() { defer close(done); w.sessions.Serve(ctx, w.cluster.Owner.ID(), false, w.ctl) }()
	w.disconnect = func() { cancel(); <-done }
	w.cleanup(w.disconnect)
	w.ctl.in <- turnMustJSON(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "Owner", Addrs: []string{}})
	_, err = w.nextFrame("welcome")
	return err
}

func (w *a10ModelsWorld) nextFrame(typ string) (json.RawMessage, error) {
	for {
		select {
		case raw := <-w.ctl.ch:
			var header struct {
				Type string `json:"t"`
			}
			if err := proto.Decode(raw, &header); err != nil {
				return nil, err
			}
			if header.Type == typ {
				return raw, nil
			}
		case <-w.ctx.Done():
			return nil, w.ctx.Err()
		}
	}
}

type a10ModelCtl struct {
	homeFrameClient
	in chan json.RawMessage
}

func (c *a10ModelCtl) Read(ctx context.Context) (json.RawMessage, error) {
	select {
	case raw := <-c.in:
		return raw, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *a10ModelCtl) Close() error                 { return nil }
func (c *a10ModelCtl) Path() string                 { return "local" }
func (c *a10ModelCtl) PathChanged() <-chan struct{} { return nil }

func (w *a10ModelsWorld) fast() error {
	if err := w.read(false); err != nil {
		return err
	}
	return hostModelsEqual([]proto.True{w.state.Agents[0].Models[0].Fast, w.state.Agents[1].Models[0].Fast}, []proto.True{true, true})
}

func (w *a10ModelsWorld) listedAll() error {
	if err := w.listed(); err != nil {
		return err
	}
	want := []proto.Model{{Model: "anthropic/claude-opus-5", Name: "Opus", Efforts: []string{"low", "high"}, Tier: "strong"}, {Model: "openai/gpt-6-mini", Name: "Mini", Efforts: []string{}, Tier: "light"}}
	return hostModelsEqual(w.state.Agents[2].Models, want)
}

func (w *a10ModelsWorld) failedCodex() error {
	want := w.first
	want.Agents = append([]proto.HostAgent{}, want.Agents...)
	want.Agents[1].Models = []proto.Model{}
	return hostModelsEqual(w.state, want)
}

func (w *a10ModelsWorld) suggestions() error {
	if err := w.gain(); err != nil {
		return err
	}
	if err := w.read(true); err != nil {
		return err
	}
	return hostModelsEqual([]string{w.state.Agents[0].Models[2].Tier, w.first.Agents[1].Models[0].Tier}, []string{"standard", "light"})
}

func (w *a10ModelsWorld) refreshed() error {
	if err := w.newList(); err != nil {
		return err
	}
	raw, err := w.nextFrame("host.info")
	if err != nil {
		return err
	}
	var push proto.HostInfoPush
	if err := proto.Decode(raw, &push); err != nil {
		return err
	}
	return hostModelsEqual(push, proto.HostInfoPush{Type: "host.info", HostState: w.state})
}

func (w *a10ModelsWorld) persistence() error {
	if err := w.setupModels(false); err != nil {
		return err
	}
	if err := w.persistenceGroup(); err != nil {
		return err
	}
	if err := w.setProfile(proto.ProfileSet{Model: new("opus")}); err != nil {
		return err
	}
	if err := w.replaceScript(`"value":"default"`, `"value":"opus"`); err != nil {
		return err
	}
	return hostModelsEqual(w.state.Agents[0].Models[0].Tier, "strong")
}

func (w *a10ModelsWorld) persistenceGroup() error {
	kw := &s3KBWorld{f: w.f, cleanup: w.cleanup, cluster: w.cluster, groups: map[string]string{}, aiByName: map[string]proto.AIMemberRow{}, repoSrc: filepath.Join(w.dir, "repo")}
	kw.clock.Store(w.now.UnixMilli())
	kw.attachBrowsers(false)
	w.persist = kw
	if err := initLocalGitRepo(kw.repoSrc); err != nil {
		return err
	}
	if err := kw.declareClaudeAccount(); err != nil {
		return err
	}
	return kw.createGroupAndAI("Profiles", "Opus", false)
}

func (w *a10ModelsWorld) replaceScript(before, after string) error {
	raw, err := os.ReadFile(filepath.Join(w.dir, "claude.json"))
	if err != nil {
		return err
	}
	if err := w.script("claude", strings.Replace(string(raw), before, after, 1)); err != nil {
		return err
	}
	return w.read(true)
}

func (w *a10ModelsWorld) setProfile(p proto.ProfileSet) error {
	kw := w.persist
	cmd := proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: proto.HomeCommand{Type: "member", Nonce: kw.ids.next(), Timestamp: kw.clock.Load(), Group: proto.ID(kw.groups["Profiles"])}, Op: "set", ID: proto.ID(kw.aiByName["Opus"].ID)}, ProfileSet: p}
	if err := kw.sendCmd(cmd); err != nil {
		return err
	}
	return kw.syncPeer(kw.b1Conn, kw.b1, kw.groups["Profiles"])
}

func (w *a10ModelsWorld) ownerEdit(tier, skills string) error {
	if err := w.setProfile(proto.ProfileSet{Tier: &tier, Skills: &skills}); err != nil {
		return err
	}
	w.before = string(turnMustJSON(w.persist.aiByName["Opus"].Profile))
	return nil
}

func (w *a10ModelsWorld) changedSuggestion() error {
	raw, err := os.ReadFile(filepath.Join(w.dir, "claude.json"))
	if err != nil {
		return err
	}
	if err := w.script("claude", strings.Replace(string(raw), "claude-opus-5", "gemini-3-pro", 1)); err != nil {
		return err
	}
	if err := w.read(true); err != nil {
		return err
	}
	return hostModelsEqual(w.state.Agents[0].Models[0].Tier, "standard")
}

func (w *a10ModelsWorld) removedModel() error {
	raw, err := os.ReadFile(filepath.Join(w.dir, "claude.json"))
	if err != nil {
		return err
	}
	if err := w.script("claude", strings.Replace(string(raw), `"value":"opus"`, `"value":"removed"`, 1)); err != nil {
		return err
	}
	if err := w.read(true); err != nil {
		return err
	}
	return hostModelsEqual(w.state.Agents[0].Models[0].Model, "removed")
}

func (w *a10ModelsWorld) emptyModels() error {
	if err := w.script("claude", `{"session":"models","exit":1}`); err != nil {
		return err
	}
	if err := w.read(true); err != nil {
		return err
	}
	return hostModelsEqual(w.state.Agents[0].Models, []proto.Model{})
}

func (w *a10ModelsWorld) restart() error {
	n := w.cluster.Nodes[0]
	w.disconnect()
	if err := w.reopenStore(n); err != nil {
		return err
	}
	if err := w.connect(); err != nil {
		return err
	}
	w.persist.attachBrowsers(false)
	return w.persist.syncPeer(w.persist.b1Conn, w.persist.b1, w.persist.groups["Profiles"])
}

func (w *a10ModelsWorld) reopenStore(n *nodetest.Node) error {
	n.StopHome()
	n.Host.Settle()
	if err := n.Store.Close(); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(n.Dir, "solodawn.db"))
	if err != nil {
		return err
	}
	n.Store = db
	n.Host, _, err = host.New(db, n.Key, func() time.Time { return w.now })
	if err != nil {
		return err
	}
	logs, err := node.OpenLogs(db, n.Key.ID())
	if err != nil {
		return err
	}
	n.Home, err = home.StartLoop(home.LoopConfig{Store: db, Home: n.Key.ID(), Owner: w.cluster.Owner.ID(), Logs: logs.Log, Tables: home.NodeTables(), Now: func() time.Time { return w.now }})
	if err != nil {
		return err
	}
	n.Link = node.AttachLocal(n.Host, n.Home)
	return nil
}

func (w *a10ModelsWorld) unchangedProfile() error {
	return hostModelsEqual(string(turnMustJSON(w.persist.aiByName["Opus"].Profile)), w.before)
}

func (w *a10ModelsWorld) finalProfile() error {
	if err := w.unchangedProfile(); err != nil {
		return err
	}
	if err := w.read(true); err != nil {
		return err
	}
	return hostModelsEqual(w.state.Agents[0].Models, []proto.Model{})
}
