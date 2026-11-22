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
	origCaps := maps.Clone(host.DriverCaps)
	cleanup(func() { host.DriverCaps = origCaps })

	w := &hostCapsWorld{f: f, cleanup: cleanup}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.m3 != nil && w.m3.activeHost != nil {
			w.m3.waitAllDormant(w.m3.activeHost)
		}
		return ctx, nil
	})

	registerHostCapsContractSteps(sc)
	registerHostCapsSwitchSteps(sc, w)
	registerHostCapsM3Steps(sc, w)
}

func registerHostCapsContractSteps(sc *godog.ScenarioContext) {
	sc.Step(`^every registered host driver declares a valid capability table$`, checkAllDriverDeclarations)
	sc.Step(`^the Claude Code and Codex capability declarations match section 8 of the step 3 driver contract$`, checkStep1DriverDeclarations)
}

func registerHostCapsSwitchSteps(sc *godog.ScenarioContext, w *hostCapsWorld) {
	sc.Step(`^a synthetic CLI "([^"]+)" with accounts "([^"]+)", approvals "([^"]+)", and takeover "([^"]+)"$`, w.givenSyntheticCLI)
	sc.Step(`^on the capability host, owner device "([^"]+)" sends agent\.set for "([^"]+)" with account "([^"]+)"$`, w.whenAgentSet)
	sc.Step(`^the capability host agent\.set ends with "([^"]+)"$`, w.thenAgentSetEnds)
	sc.Step(`^MCP tool "([^"]+)" for "([^"]+)" is "([^"]+)" in a structured session and "([^"]+)" in a terminal session$`, w.thenMCPToolListing)
	sc.Step(`^CheckTakeover for "([^"]+)" ends with "([^"]+)"$`, thenCheckTakeoverOutcome)
}

func registerHostCapsM3Steps(sc *godog.ScenarioContext, w *hostCapsWorld) {
	sc.Step(`^a capability test node where "([^"]+)" has a synthetic API-key-only declaration without takeover$`, w.givenRestrictedM3Node)
	sc.Step(`^on the capability test node, configuring AI "([^"]+)" on "([^"]+)" refuses personal naming accounts and accepts api_key$`, w.whenConfigureAPIKeyAI)
	sc.Step(`^on the capability test node, "([^"]+)" is scripted on thread "([^"]+)" to reply "([^"]+)" and "([^"]+)"$`, w.m3ScriptCodexTwoTurns)
	sc.Step(`^on the capability test node, mentioning "([^"]+)" twice in "([^"]+)" gets "([^"]+)" then "([^"]+)" in 1 "([^"]+)" process on session "([^"]+)"$`, w.thenTwoTurnMentionInOneProcess)
	sc.Step(`^structured sessions on the capability test node for "([^"]+)" omit MCP tool "([^"]+)" and CheckTakeover refuses naming takeover$`, w.thenRestrictedSwitchesHold)
}

func checkAllDriverDeclarations() error {
	drvKeys := slices.Sorted(maps.Keys(host.Drivers))
	capKeys := slices.Sorted(maps.Keys(host.DriverCaps))
	if !slices.Equal(drvKeys, capKeys) {
		return fmt.Errorf("Drivers keys %v != DriverCaps keys %v", drvKeys, capKeys)
	}
	for _, name := range capKeys {
		if err := validateCapsEntry(name, host.DriverCaps[name]); err != nil {
			return err
		}
	}
	return nil
}

func validateCapsEntry(name string, c host.Capabilities) error {
	if len(c.Accounts) == 0 {
		return fmt.Errorf("%s has empty accounts", name)
	}
	if !slices.Contains(c.Levels, 1) {
		return fmt.Errorf("%s levels %v missing level 1", name, c.Levels)
	}
	if c.Approvals != host.ApprovalTool && c.Approvals != host.ApprovalConn {
		return fmt.Errorf("%s invalid approvals %q", name, c.Approvals)
	}
	if c.Takeover != host.TakeoverNew && c.Takeover != host.TakeoverResume && c.Takeover != host.TakeoverNone {
		return fmt.Errorf("%s invalid takeover %q", name, c.Takeover)
	}
	if c.Usage == nil {
		return fmt.Errorf("%s nil usage slice", name)
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
	next := maps.Clone(host.DriverCaps)
	next[agent] = host.Capabilities{
		Accounts:  strings.Split(accountsCSV, ","),
		Levels:    []int{1, 2, 3},
		Approvals: approvals,
		Takeover:  takeover,
		Usage:     []string{},
	}
	host.DriverCaps = next

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
	_, err = h.Agents(context.Background())
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
	// Single reader: wait before pipeMCPToolState decodes tools/list.
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeMCP(context.Background(), token, bufio.NewReader(serverConn), serverConn)
	}()
	clientConn.SetDeadline(time.Now().Add(5 * time.Second))
	dec := json.NewDecoder(clientConn)
	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	var initResp map[string]any
	if err := dec.Decode(&initResp); err != nil {
		return "", err
	}
	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	fmt.Fprintln(clientConn, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := dec.Decode(&listResp); err != nil {
		return "", err
	}
	clientConn.Close()
	<-done
	for _, tl := range listResp.Result.Tools {
		if tl.Name == toolName {
			return "listed", nil
		}
	}
	return "omitted", nil
}

func thenCheckTakeoverOutcome(agent, want string) error {
	assignFirst, err := host.CheckTakeover(agent)
	got := "start_in_terminal"
	if err != nil {
		got = err.Error()
	} else if assignFirst {
		got = "assign_first"
	}
	if got != want {
		return fmt.Errorf("CheckTakeover(%q) = %q, want %q", agent, got, want)
	}
	return nil
}

func (w *hostCapsWorld) givenRestrictedM3Node(agent string) error {
	next := maps.Clone(host.DriverCaps)
	next[agent] = host.Capabilities{
		Accounts:  []string{host.AccountAPIKey},
		Levels:    []int{1},
		Approvals: host.ApprovalConn,
		Takeover:  host.TakeoverNone,
		Usage:     []string{},
	}
	host.DriverCaps = next
	m3 := &m3World{
		f:        w.f,
		cleanup:  w.cleanup,
		tb:       scenarioTB{TB: w.f.t, cleanup: w.cleanup},
		groups:   map[string]string{},
		aiByName: map[string]proto.AIMemberRow{},
	}
	w.m3 = m3
	if err := m3.startCluster(); err != nil {
		return err
	}
	return m3.createGroup("Website")
}

func (w *hostCapsWorld) whenConfigureAPIKeyAI(aiName, agent string) error {
	ctx := context.Background()
	n := w.m3.node()
	if _, err := n.Host.Agents(ctx); err != nil {
		return err
	}
	by := w.m3.cluster.Owner.ID()
	badRaw, err := marshalNoEscape(proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.m3.ids.next(), Timestamp: time.Now().UnixMilli(), To: n.Key.ID()},
		Agent:       agent,
		Account:     "personal",
	})
	if err != nil {
		return err
	}
	_, badErr := host.Surface["agent.set"](ctx, n.Host, n.Link, by, badRaw)
	var hostErr *host.Error
	if !errors.As(badErr, &hostErr) || hostErr.Code != "bad_request" || !strings.Contains(hostErr.Message, "accounts") {
		return fmt.Errorf("personal agent.set got %v, want bad_request naming accounts", badErr)
	}
	okRaw, err := marshalNoEscape(proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.m3.ids.next(), Timestamp: time.Now().UnixMilli(), To: n.Key.ID()},
		Agent:       agent,
		Account:     "api_key",
	})
	if err != nil {
		return err
	}
	if _, err := host.Surface["agent.set"](ctx, n.Host, n.Link, by, okRaw); err != nil {
		return err
	}
	joinRaw, err := marshalNoEscape(proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.m3.ids.next(), Timestamp: time.Now().UnixMilli(), To: n.Key.ID()},
		Group:       proto.ID(w.m3.groups["Website"]),
		Home:        n.Key.ID(),
		Agent:       agent,
		Name:        aiName,
	})
	if err != nil {
		return err
	}
	if _, err := host.Surface["host.join"](ctx, n.Host, n.Link, by, joinRaw); err != nil {
		return err
	}
	return w.m3.drainBrowserUntil(5*time.Second, func() bool {
		_, ok := w.m3.aiByName[aiName]
		return ok
	}, nil)
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
	if err != nil {
		return err
	}
	if state != "omitted" {
		return fmt.Errorf("tool %s was %s, want omitted", toolName, state)
	}
	_, takeErr := host.CheckTakeover(agent)
	var hostErr *host.Error
	if !errors.As(takeErr, &hostErr) || hostErr.Code != "conflict" || !strings.Contains(hostErr.Message, "takeover") {
		return fmt.Errorf("CheckTakeover(%q) = %v, want conflict naming takeover", agent, takeErr)
	}
	return nil
}
