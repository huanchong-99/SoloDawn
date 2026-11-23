//go:build acceptance

package features

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["s3_drivers"] = s3DriverSteps }

type s3DriverWorld struct {
	f       fixtures
	cleanup func(func())
	caps    *hostCapsWorld
	agent   string
	mcp     s3MCPInput
}

func s3DriverSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s3DriverWorld{f: f}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.caps != nil && w.caps.m3.activeHost != nil {
			waitHostDormant(w.caps.m3.activeHost)
		}
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	sc.Step(`^the S3 declarations match the shipped OpenCode and Qwen contract$`, s3DriverDeclarations)
	sc.Step(`^an S3 project with an API-key-only "([^"]+)" member$`, w.setup)
	sc.Step(`^the named step-1 case "([^"]+)" runs for that S3 member$`, w.runCase)
	sc.Step(`^the S3 capability exclusions hold for that member$`, w.exclusions)
}

func s3DriverDeclarations() error {
	want := host.Capabilities{Accounts: []string{"api_key"}, Levels: []int{1, 3}, Approvals: "conn", Takeover: "none", Usage: []string{}}
	for _, agent := range []string{"opencode", "qwen"} {
		if !reflect.DeepEqual(host.DriverCaps[agent], want) {
			return fmt.Errorf("%s capabilities = %+v, want %+v", agent, host.DriverCaps[agent], want)
		}
	}
	return checkStep1DriverDeclarations()
}

func (w *s3DriverWorld) setup(agent string) error {
	w.agent = agent
	if _, err := os.Stat(filepath.Join(w.f.fakeCLIs, hostAgentsExe("opencode"))); os.IsNotExist(err) {
		f, err := serverProfileFixtures(w.f, w.cleanup)
		if err != nil {
			return err
		}
		w.f = f
	}
	m := &m3World{f: w.f, cleanup: w.cleanup, tb: scenarioTB{TB: w.f.t, cleanup: w.cleanup}, groups: map[string]string{}, homeView: newHomeView(), mcpInput: &w.mcp}
	w.caps = &hostCapsWorld{f: w.f, cleanup: w.cleanup, m3: m}
	return s3Run(
		m.startCluster,
		func() error { return m.createGroup("Website") },
		func() error { return w.caps.whenConfigureAPIKeyAI("Worker", agent) },
		func() error {
			err := w.caps.send("agent.set", s3AgentSet(w.caps, agent, "team"))
			if !refusedNaming(err, "bad_request", "accounts") {
				return fmt.Errorf("team agent.set = %v, want bad_request naming accounts", err)
			}
			return nil
		},
	)
}

func s3Run(steps ...func() error) error {
	for i, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

func (w *s3DriverWorld) runCase(name string) error {
	cases := map[string]func() error{
		"M3 tools":  w.tools,
		"A3.1/A3.2": w.memory,
		"A3.3":      w.remote,
		"A3.4":      w.idle,
		"A3.5":      w.maxOpen,
		"A3.6":      w.crash,
		"A3.7":      w.outside,
		"A3.9":      w.secret,
		"A3.10":     w.dedup,
		"A4.4":      w.readOnly,
		"A4.5":      w.levelThree,
		"A4.7":      w.pause,
		"A4.8":      w.offline,
		"A4.9":      w.timeout,
	}
	return cases[name]()
}

func (w *s3DriverWorld) exclusions() error {
	return s3Run(s3DriverDeclarations, func() error { return w.caps.thenRestrictedSwitchesHold(w.agent, "approve") })
}

func (w *s3DriverWorld) tools() error {
	m := w.caps.m3
	lists, err := loadM3GoldenTools()
	if err != nil {
		return err
	}
	for _, spec := range []struct {
		kind host.Kind
		sec  bool
		task string
		want []proto.Tool
	}{
		{host.GroupSession, true, "", lists.Secretary.Tools},
		{host.GroupSession, false, "", lists.Group.Tools},
		{host.ReviewSession, false, "review", lists.Review.Tools},
		{host.TaskSession, false, "task", lists.TaskLevel23.Tools},
	} {
		got, err := m.listSessionTools("tools", fmt.Sprint(spec.kind, spec.sec), spec.task, w.agent, spec.kind, spec.sec)
		if err != nil {
			return err
		}
		slices.SortFunc(spec.want, func(a, b proto.Tool) int { return cmp.Compare(a.Name, b.Name) })
		if !reflect.DeepEqual(got, spec.want) {
			return fmt.Errorf("%s tools = %+v, want %+v", spec.kind, got, spec.want)
		}
	}
	return nil
}

func (w *s3DriverWorld) maxOpen() error {
	m := w.caps.m3
	other := "qwen"
	if w.agent == other {
		other = "opencode"
	}
	second := &s3DriverWorld{f: w.f, cleanup: w.cleanup, caps: w.caps, agent: other}
	release := w.slotGate()
	body := append(mcpPostSteps(2, "Second working."), mcpCallSteps(3, "s3_gate", map[string]any{}, map[string]any{"isError": false})...)
	return s3Run(func() error { return m.writeSetting("max_open", "1") },
		func() error { return w.caps.whenConfigureAPIKeyAI("Second", other) },
		func() error { return w.script(false, true, s3PostTurn(1, "First waiting.")) },
		func() error { return second.script(false, true, s3PostTurn(1, "Second started."), s3Turn{body: body}) },
		w.mention, func() error { return w.posted("First waiting.") },
		func() error { return m.whenOwnerPosts("@Second begin", "Website") },
		func() error { return m.thenAIPosts("Second", "Second started.", "Website") },
		func() error { return m.thenAIHasState("Worker", "dormant", "Website") },
		func() error { return w.workingSlot(release) })
}

func (w *s3DriverWorld) mention() error {
	return w.caps.m3.whenOwnerPosts("@Worker remember alpha-7", "Website")
}
func (w *s3DriverWorld) posted(text string) error {
	return w.caps.m3.thenAIPostsAndEntersState("Worker", text, "Website", "waiting")
}

func (w *s3DriverWorld) memory() error {
	return s3Run(func() error {
		return w.script(false, false, s3PostTurn(1, "Noted alpha-7."), s3PostTurn(2, "First mention said alpha-7."))
	},
		w.mention, func() error { return w.posted("Noted alpha-7.") },
		func() error { return w.caps.m3.whenOwnerPosts("@Worker what did I say", "Website") },
		func() error { return w.caps.m3.thenAIPosts("Worker", "First mention said alpha-7.", "Website") },
		func() error { return w.caps.m3.thenOneSessionLogged(w.agent, "s3-session") })
}

func (w *s3DriverWorld) idle() error {
	m := w.caps.m3
	return s3Run(func() error { return m.writeSetting("idle_minutes", "1") },
		func() error { return w.script(false, true, s3PostTurn(1, "Before idle.")) }, w.mention,
		func() error { return w.posted("Before idle.") },
		func() error {
			return m.drainBrowserUntil(75*time.Second, func() bool { return m.aiByName["Worker"].State == "dormant" }, nil)
		},
		func() error { return m.thenAIHasState("Worker", "dormant", "Website") },
		func() error { return w.script(true, false, s3PostTurn(1, "Resumed alpha-7.")) }, w.mention,
		func() error { return m.thenAIPosts("Worker", "Resumed alpha-7.", "Website") },
		func() error {
			starts, err := m.waitLogs("start "+w.agent+" ", 2)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(starts, []string{"start " + w.agent + " s3-session", "start " + w.agent + " s3-session"}) {
				return fmt.Errorf("resume starts = %v", starts)
			}
			return nil
		})
}

func (w *s3DriverWorld) crash() error {
	m := w.caps.m3
	return s3Run(func() error { return w.script(false, false, s3Turn{crash: true}) }, w.mention,
		func() error { return m.thenTurnInterruptedRow("Worker", "crash", "Website") },
		func() error { return w.script(true, false, s3PostTurn(1, "Recovered.")) }, w.mention,
		func() error { return m.thenAIPosts("Worker", "Recovered.", "Website") })
}

func (w *s3DriverWorld) outside() error {
	m := w.caps.m3
	return s3Run(func() error {
		return w.script(false, false, s3Turn{body: mcpPostSteps(1, "Visible reply."), outside: "UNPOSTED_ALPHA"}, s3Turn{outside: "UNPOSTED_BETA"})
	},
		w.mention, func() error { return w.posted("Visible reply.") }, w.mention,
		func() error { return m.thenSilentTurnCompletes("Worker") },
		func() error { return m.thenNoMsgContains("Website", "UNPOSTED_ALPHA", "UNPOSTED_BETA") })
}

func (w *s3DriverWorld) secret() error {
	m := w.caps.m3
	body := mcpCallSteps(1, "post", map[string]any{"text": "leak " + m3SecretToken}, map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": m3SecretError}}})
	return s3Run(func() error { return w.script(false, false, s3Turn{body: body}) }, w.mention,
		func() error { return m.thenSecretTurnFinishes("Worker") },
		func() error { return m.thenNoMsgFromAIOnHome("Worker", "Website") })
}

func (w *s3DriverWorld) dedup() error {
	m := w.caps.m3
	m.node().Link.Close()
	m.holdLink = wrapM3HoldLink(m.activeHost, m.node().Home, true)
	w.cleanup(func() { m.holdLink.Close() })
	return s3Run(func() error { return awaitHostGroupSynced(m.activeHost, m.groups["Website"]) },
		func() error { return w.script(false, false, s3PostTurn(1, "Delivered once.")) }, w.mention,
		func() error { return m.thenPostAndOneHeldAck("Worker", "Delivered once.", "Website") }, m.whenHomeResendsInbox,
		func() error { return m.holdLink.awaitAcks(1) }, m.whenHostRestarts,
		func() error { return m.restartLink.awaitAcks(1) },
		func() error { return awaitHomeInboxCount(m.node().Store, 0) },
		func() error {
			if got := m.countMessagesBy(m.aiByName["Worker"].ID); got != 1 {
				return fmt.Errorf("messages = %d, want 1", got)
			}
			return m.thenOneProcessLogged(w.agent)
		})
}
