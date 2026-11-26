//go:build acceptance

package features

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_caps"] = hostCapsSteps
}

type hostCapsWorld struct {
	f       fixtures
	cleanup func(func())
	h       *host.Host
	setErr  error
	m3      *m3World
}

func hostCapsSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)

	w := &hostCapsWorld{f: f, cleanup: cleanup}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.m3 != nil && w.m3.activeHost != nil {
			waitHostDormant(w.m3.activeHost)
		}
		return ctx, nil
	})

	registerHostCapsContractSteps(sc)
	registerHostCapsSwitchSteps(sc, w)
	registerHostCapsM3Steps(sc, w)
}

func registerHostCapsContractSteps(sc *godog.ScenarioContext) {
	sc.Step(`^Drivers and DriverCaps have the same keys$`, checkDriverKeys)
	sc.Step(`^the Claude Code and Codex capability declarations match section 8 of the step 3 driver contract$`, checkStep1DriverDeclarations)
}

func registerHostCapsSwitchSteps(sc *godog.ScenarioContext, w *hostCapsWorld) {
	sc.Step(`^a synthetic CLI "([^"]+)" with accounts "([^"]+)", approvals "([^"]+)", and takeover "([^"]+)"$`, w.givenSyntheticCLI)
	sc.Step(`^on the capability host, owner device "([^"]+)" sends agent\.set for "([^"]+)" with account "([^"]+)"$`, w.whenAgentSet)
	sc.Step(`^the capability host agent\.set ends with "([^"]+)"$`, w.thenAgentSetEnds)
	sc.Step(`^MCP tool "([^"]+)" for "([^"]+)" is "([^"]+)" in a structured session and "([^"]+)" in a terminal session$`, w.thenMCPToolListing)
}

func registerHostCapsM3Steps(sc *godog.ScenarioContext, w *hostCapsWorld) {
	sc.Step(`^a capability test node where "([^"]+)" has a synthetic API-key-only declaration without takeover$`, w.givenRestrictedM3Node)
	sc.Step(`^on the capability test node, configuring AI "([^"]+)" on "([^"]+)" refuses personal naming accounts and accepts api_key$`, w.whenConfigureAPIKeyAI)
	sc.Step(`^on the capability test node, "([^"]+)" is scripted on thread "([^"]+)" to reply "([^"]+)" and "([^"]+)"$`, w.m3ScriptCodexTwoTurns)
	sc.Step(`^on the capability test node, mentioning "([^"]+)" twice in "([^"]+)" gets "([^"]+)" then "([^"]+)" in 1 "([^"]+)" process on session "([^"]+)"$`, w.thenTwoTurnMentionInOneProcess)
	sc.Step(`^structured sessions on the capability test node for "([^"]+)" omit MCP tool "([^"]+)" and takeover\.start refuses naming takeover$`, w.thenRestrictedSwitchesHold)
}

func checkDriverKeys() error {
	drvKeys := slices.Sorted(maps.Keys(host.Drivers))
	capKeys := slices.Sorted(maps.Keys(host.DriverCaps))
	if !slices.Equal(drvKeys, capKeys) {
		return fmt.Errorf("Drivers keys %v != DriverCaps keys %v", drvKeys, capKeys)
	}
	return nil
}

func checkStep1DriverDeclarations() error {
	wantClaudeLevels := []int{1, 2, 3}
	if runtime.GOOS == "windows" {
		wantClaudeLevels = []int{1, 3}
	}
	wantClaude := host.Capabilities{
		Accounts:  []string{"personal", "team", "api_key"},
		Levels:    wantClaudeLevels,
		Approvals: "tool",
		Takeover:  "new",
		Usage:     []string{},
	}
	wantCodex := host.Capabilities{
		Accounts:  []string{"personal", "team", "api_key"},
		Levels:    []int{1, 2, 3},
		Approvals: "conn",
		Takeover:  "resume",
		Usage:     []string{},
	}
	if !reflect.DeepEqual(host.DriverCaps["claude"], wantClaude) {
		return fmt.Errorf("claude caps = %+v, want %+v", host.DriverCaps["claude"], wantClaude)
	}
	if !reflect.DeepEqual(host.DriverCaps["codex"], wantCodex) {
		return fmt.Errorf("codex caps = %+v, want %+v", host.DriverCaps["codex"], wantCodex)
	}
	return nil
}

func (w *hostCapsWorld) givenSyntheticCLI(agent, accountsCSV, approvals, takeover string) error {
	w.setDriverCaps(agent, host.Capabilities{
		Accounts:  strings.Split(accountsCSV, ","),
		Levels:    []int{1, 2, 3},
		Approvals: approvals,
		Takeover:  takeover,
		Usage:     []string{},
	})

	dir, err := os.MkdirTemp("", "solodawn-caps-*")
	if err != nil {
		return err
	}
	w.cleanup(func() { os.RemoveAll(dir) })
	src := filepath.Join(w.f.fakeCLIs, hostAgentsExe("claude"))
	dst := filepath.Join(dir, hostAgentsExe(agent))
	if err := hostAgentsLinkOrCopy(src, dst); err != nil {
		return err
	}
	prev, had := os.LookupEnv("PATH")
	if err := os.Setenv("PATH", hostAgentsIsolatedPath(dir)); err != nil {
		return err
	}
	w.cleanup(func() { restoreVar("PATH", prev, had) })

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	h, _, err := host.New(db, key, func() time.Time { return time.UnixMilli(1_763_600_000_000) })
	if err != nil {
		return err
	}
	w.h = h
	_, err = h.Agents(context.Background(), false)
	return err
}

func (w *hostCapsWorld) whenAgentSet(by, agent, account string) {
	payload := fmt.Sprintf(`{"t":"agent.set","n":"STwCAvPRTFkRNCvXYEgsMw","ts":1763600000000,"to":"host-device","agent":%q,"account":%q}`, agent, account)
	_, w.setErr = host.Surface["agent.set"](context.Background(), w.h, nil, by, json.RawMessage(payload))
}

func (w *hostCapsWorld) thenAgentSetEnds(want string) error {
	got := "ok"
	if w.setErr != nil {
		got = w.setErr.Error()
	}
	if got != want {
		return fmt.Errorf("got %q, want %q", got, want)
	}
	return nil
}

func (w *hostCapsWorld) thenMCPToolListing(toolName, agent, wantStruct, wantInter string) error {
	sKey := host.SessionKey{Group: "g-caps", Member: "m-" + agent, Task: "t1"}
	_ = w.h.Sessions.Create(host.Session{
		SessionKey: sKey,
		Kind:       host.TaskSession,
		Folder:     "wt/1-caps",
		Agent:      agent,
		Home:       "home-1",
		Name:       agent,
	})
	tokStruct := w.h.Sessions.Admit(sKey, 2, false)
	tokInter := w.h.Sessions.Admit(sKey, 2, true)
	gotStruct, err := pipeMCPToolState(w.h, tokStruct, toolName)
	if err != nil {
		return err
	}
	gotInter, err := pipeMCPToolState(w.h, tokInter, toolName)
	if err != nil {
		return err
	}
	if gotStruct != wantStruct || gotInter != wantInter {
		return fmt.Errorf("structured=%s interactive=%s, want %s/%s", gotStruct, gotInter, wantStruct, wantInter)
	}
	return nil
}

func pipeMCPToolState(h *host.Host, token, toolName string) (string, error) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeMCP(context.Background(), token, bufio.NewReader(serverConn), serverConn)
	}()
	clientConn.SetDeadline(time.Now().Add(5 * time.Second))
	raw, err := exchangeToolsList(clientConn, clientConn)
	clientConn.Close()
	<-done
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &list)
	}
	for _, tl := range list.Tools {
		if tl.Name == toolName {
			return "listed", err
		}
	}
	return "omitted", err
}

func (w *hostCapsWorld) givenRestrictedM3Node(agent string) error {
	w.setDriverCaps(agent, host.Capabilities{
		Accounts:  []string{host.AccountAPIKey},
		Levels:    []int{1},
		Approvals: host.ApprovalConn,
		Takeover:  host.TakeoverNone,
		Usage:     []string{},
	})
	m3 := &m3World{
		f:        w.f,
		cleanup:  w.cleanup,
		tb:       scenarioTB{TB: w.f.t, cleanup: w.cleanup},
		groups:   map[string]string{},
		homeView: newHomeView(),
	}
	w.m3 = m3
	if err := m3.startCluster(); err != nil {
		return err
	}
	return m3.createGroup("Website")
}

func (w *hostCapsWorld) setDriverCaps(agent string, caps host.Capabilities) {
	prev := host.DriverCaps
	next := maps.Clone(prev)
	next[agent] = caps
	host.DriverCaps = next
	w.cleanup(func() { host.DriverCaps = prev })
}

func (w *hostCapsWorld) whenConfigureAPIKeyAI(aiName, agent string) error {
	n := w.m3.node()
	if _, err := n.Host.Agents(context.Background(), false); err != nil {
		return err
	}
	badErr := w.send("agent.set", proto.AgentSet{HostCommand: w.header("agent.set"), Agent: agent, Account: "personal"})
	if !refusedNaming(badErr, "bad_request", "accounts") {
		return fmt.Errorf("personal agent.set got %v, want bad_request naming accounts", badErr)
	}
	if err := w.send("agent.set", proto.AgentSet{HostCommand: w.header("agent.set"), Agent: agent, Account: "api_key"}); err != nil {
		return err
	}
	gid := proto.ID(w.m3.groups["Website"])
	workspace, err := aiWorkspace(w.m3.root, gid, aiName)
	if err != nil {
		return err
	}
	join := proto.HostJoin{HostCommand: w.header("host.join"), Group: gid, Home: n.Key.ID(), Agent: agent, Name: aiName, Workspace: workspace}
	if err := w.send("host.join", join); err != nil {
		return err
	}
	return w.m3.drainBrowserUntil(5*time.Second, func() bool {
		_, ok := w.m3.aiByName[aiName]
		return ok
	}, nil)
}

// header is the envelope of an owner command to the capability test node.
func (w *hostCapsWorld) header(typ string) proto.HostCommand {
	return proto.HostCommand{Type: typ, Nonce: w.m3.ids.next(), Timestamp: time.Now().UnixMilli(), To: w.m3.node().Key.ID()}
}

// send hands an owner command of type typ to the capability test node's host.
func (w *hostCapsWorld) send(typ string, cmd any) error {
	raw, err := marshalNoEscape(cmd)
	if err != nil {
		return err
	}
	n := w.m3.node()
	_, err = host.Surface[typ](context.Background(), n.Host, n.Link, w.m3.cluster.Owner.ID(), raw)
	return err
}

func refusedNaming(err error, code, word string) bool {
	hostErr, ok := errors.AsType[*host.Error](err)
	return ok && hostErr.Code == code && strings.Contains(hostErr.Message, word)
}

func (w *hostCapsWorld) m3ScriptCodexTwoTurns(ai, threadID, reply1, reply2 string) error {
	return w.m3.scriptCodexTwoTurns(ai, threadID, reply1, reply2)
}

func (w *hostCapsWorld) thenTwoTurnMentionInOneProcess(ai, group, reply1, reply2, vendor, sessionID string) error {
	if err := w.m3.whenOwnerPosts("@"+ai+" remember key-42", group); err != nil {
		return err
	}
	if err := w.m3.thenAIPostsAndEntersState(ai, reply1, group, "waiting"); err != nil {
		return err
	}
	if err := w.m3.whenOwnerPosts("@"+ai+" what did I say", group); err != nil {
		return err
	}
	if err := w.m3.thenAIPosts(ai, reply2, group); err != nil {
		return err
	}
	return w.m3.thenOneSessionLogged(vendor, sessionID)
}

func (w *hostCapsWorld) thenRestrictedSwitchesHold(agent, toolName string) error {
	sessions := w.m3.activeHost.Sessions.All()
	if len(sessions) == 0 {
		return errors.New("expected active group session")
	}
	tok := w.m3.activeHost.Sessions.Admit(sessions[0].SessionKey, 1, false)
	defer w.m3.activeHost.Sessions.Forget(tok)
	state, err := pipeMCPToolState(w.m3.activeHost, tok, toolName)
	if err == nil && state != "omitted" {
		err = fmt.Errorf("tool %s was %s, want omitted", toolName, state)
	}
	if err != nil {
		return err
	}
	start := proto.TakeoverStart{Takeover: proto.Takeover{HostCommand: w.header("takeover.start"), Group: proto.ID(sessions[0].Group), Member: proto.ID(sessions[0].Member)}}
	if takeErr := w.send("takeover.start", start); !refusedNaming(takeErr, "conflict", "takeover") {
		return fmt.Errorf("takeover.start for %s = %v, want conflict naming takeover", agent, takeErr)
	}
	return nil
}
