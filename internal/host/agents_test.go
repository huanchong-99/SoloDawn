package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// When SOLODAWN_TEST_AGENTS is set and this test binary is invoked as a
// stand-in CLI with a matching .spec file in that folder, it runs the spec.
func init() {
	if dir := os.Getenv("SOLODAWN_TEST_AGENTS"); dir != "" {
		name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if _, err := os.Stat(filepath.Join(dir, name+".spec")); err == nil {
			os.Exit(agtRunStandIn(dir, name))
		}
	}
}

type agtStandInSpec struct {
	Stdout  string
	Stderr  string
	Code    int
	SleepMS int
}

func agtOK(err error) {
	if err != nil {
		panic(err)
	}
}

func agtVal[T any](v T, err error) T {
	agtOK(err)
	return v
}

func agtRunStandIn(dir, name string) int {
	if len(os.Args) != 2 || os.Args[1] != "--version" {
		return 1
	}
	if len(agtVal(io.ReadAll(os.Stdin))) != 0 {
		return 2
	}
	logFile := agtVal(os.OpenFile(filepath.Join(dir, name+".calls"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600))
	fmt.Fprintln(logFile, strings.Join(os.Args[1:], " "))
	agtOK(logFile.Close())
	var spec agtStandInSpec
	agtOK(json.Unmarshal(agtVal(os.ReadFile(filepath.Join(dir, name+".spec"))), &spec))
	time.Sleep(time.Duration(spec.SleepMS) * time.Millisecond)
	fmt.Fprint(os.Stdout, spec.Stdout)
	fmt.Fprint(os.Stderr, spec.Stderr)
	return spec.Code
}

func agtCheck(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func agtEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func agtErrText(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func agtReadAgents(t *testing.T, h *Host) []proto.HostAgent {
	t.Helper()
	got, err := h.Agents(t.Context(), false)
	agtCheck(t, err)
	return got
}

func agtCallSet(t *testing.T, h *Host, by string, raw json.RawMessage) {
	t.Helper()
	res, err := Surface["agent.set"](t.Context(), h, nil, by, raw)
	agtCheck(t, err)
	agtEqual(t, res, nil)
}

func agtIsolatedPath(dir string) string {
	if runtime.GOOS == "windows" {
		if sysRoot := os.Getenv("SystemRoot"); sysRoot != "" {
			return dir + string(os.PathListSeparator) + filepath.Join(sysRoot, "System32")
		}
	}
	return dir
}

func agtExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

type agtStandInHarness struct {
	t       *testing.T
	dir     string
	exePath string
}

func agtLinkOrCopy(src, dst string) error {
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	return os.WriteFile(dst, agtVal(os.ReadFile(src)), 0700)
}

func newAgtStandIns(t *testing.T, specs map[string]agtStandInSpec) *agtStandInHarness {
	t.Helper()
	exePath := agtVal(os.Executable())
	dir := t.TempDir()
	t.Setenv("PATH", agtIsolatedPath(dir))
	t.Setenv("SOLODAWN_TEST_AGENTS", dir)
	h := &agtStandInHarness{t: t, dir: dir, exePath: exePath}
	for name, spec := range specs {
		h.install(name, spec)
	}
	return h
}

func (h *agtStandInHarness) install(name string, spec agtStandInSpec) {
	h.t.Helper()
	binPath := filepath.Join(h.dir, agtExeName(name))
	if _, err := os.Lstat(binPath); err != nil {
		agtCheck(h.t, agtLinkOrCopy(h.exePath, binPath))
	}
	agtCheck(h.t, os.WriteFile(filepath.Join(h.dir, name+".spec"), agtVal(json.Marshal(spec)), 0600))
}

func (h *agtStandInHarness) calls(name string) int {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.dir, name+".calls"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	agtCheck(h.t, err)
	lines := strings.FieldsFunc(string(raw), func(r rune) bool { return r == '\n' || r == '\r' })
	for _, line := range lines {
		agtEqual(h.t, strings.TrimSpace(line), "--version")
	}
	return len(lines)
}

func agtAgentRows(t *testing.T, db *store.Store) string {
	t.Helper()
	var out string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT coalesce(group_concat(concat_ws('|', agent, cli_version, coalesce(account, '-'), caps), char(10)), '')
			FROM (SELECT agent, cli_version, account, caps FROM agent ORDER BY agent)`).Scan(&out)
	}
	agtCheck(t, db.Write(read))
	return out
}

func agtSetPayload(agent, account string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"t":"agent.set","n":"STwCAvPRTFkRNCvXYEgsMw","ts":1763600000000,"to":"host-device","agent":%q,"account":%q}`, agent, account))
}

func TestAgentsCleanVersionFormats(t *testing.T) {
	cases := []struct {
		agent string
		raw   string
		want  string
	}{
		{"claude", "2.1.311 (Claude Code)\n", "2.1.311"},
		{"claude", "claude 2.1.311 (Claude Code)\n", "2.1.311"},
		{"claude", "claude-cli 2.1.311\n", "2.1.311"},
		{"codex", "codex-cli 0.164.0\n", "0.164.0"},
		{"codex", "codex 0.164.0\r\nextra line\r\n", "0.164.0"},
		{"claude", "claude\n", "claude"},
		{"codex", "0.164.0\n", "0.164.0"},
		{"claude", "   \n\t\n", ""},
	}
	for _, tc := range cases {
		agtEqual(t, agtCleanVersion(tc.agent, tc.raw), tc.want)
	}
}

func TestAgentsDetectsFakeCLIsAndPreservesStoredFields(t *testing.T) {
	binDir := t.TempDir()
	cmd := exec.Command("go", "build", "-o", binDir+string(os.PathSeparator), "./fakecli/claude", "./fakecli/codex", "./fakecli/opencode")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake CLIs: %v\n%s", err, out)
	}
	t.Setenv("PATH", agtIsolatedPath(binDir))

	db := seamStore(t)
	seamSQL(t, db, `INSERT INTO agent(agent, cli_version, account, caps)
		VALUES('claude', '2.0.0', 'team', '{"day":{"unit":"turns","max":200},"task":{"unit":"usd","max":12.5}}')`)

	h, _ := seamHost(t, db)
	got := agtReadAgents(t, h)
	team := "team"
	want := []proto.HostAgent{
		{
			Agent:      "claude",
			CLIVersion: "2.1.311",
			Account:    &team,
			LoginOK:    true,
			Caps: proto.Caps{
				Day:  &proto.Cap{Unit: "turns", Max: 200},
				Task: &proto.Cap{Unit: "usd", Max: 12.5},
			},
			Driver: proto.Driver{
				Accounts: []string{"personal", "team", "api_key"},
				Takeover: "new",
			},
			Groups: []proto.AgentGroup{},
			Models: []proto.Model{},
		},
		{
			Agent:      "codex",
			CLIVersion: "0.164.0",
			Account:    nil,
			LoginOK:    true,
			Caps:       proto.Caps{},
			Driver: proto.Driver{
				Accounts: []string{"personal", "team", "api_key"},
				Takeover: "resume",
			},
			Groups: []proto.AgentGroup{},
			Models: []proto.Model{},
		},
		{
			Agent:      "opencode",
			CLIVersion: "1.19.6",
			Account:    nil,
			LoginOK:    true,
			Caps:       proto.Caps{},
			Driver: proto.Driver{
				Accounts: []string{"api_key"},
				Takeover: "none",
			},
			Groups: []proto.AgentGroup{},
			Models: []proto.Model{},
		},
		externalHostAgent(),
	}
	agtEqual(t, got, want)
	encoded := agtVal(json.Marshal(got))
	agtEqual(t, proto.UnexpectedNulls(encoded, "/1/account", "/2/account", "/3/account"), []string{})

	wantRows := strings.Join([]string{
		`claude|2.1.311|team|{"day":{"unit":"turns","max":200},"task":{"unit":"usd","max":12.5}}`,
		`codex|0.164.0|-|{}`,
		`opencode|1.19.6|-|{}`,
	}, "\n")
	agtEqual(t, agtAgentRows(t, db), wantRows)
}

func TestAgentsEmptyNonZeroExitAndTimeout(t *testing.T) {
	t.Run("none on PATH", func(t *testing.T) {
		standIns := newAgtStandIns(t, nil)
		h, _ := seamHost(t, seamStore(t))
		now := time.UnixMilli(seamStart)
		h.Now = func() time.Time { return now }

		got := agtReadAgents(t, h)
		agtEqual(t, got, []proto.HostAgent{externalHostAgent()})
		agtEqual(t, string(agtVal(json.Marshal(got))), `[{"agent":"external","cli_version":"","account":null,"login_ok":true,"caps":{},"driver":{"accounts":["personal","team","api_key"],"takeover":"none"},"groups":[],"models":[]}]`)

		// Install claude after the empty probe: still cached within 60 s.
		standIns.install("claude", agtStandInSpec{Stdout: "2.1.311 (Claude Code)\n"})
		now = now.Add(30 * time.Second)
		agtEqual(t, agtReadAgents(t, h), []proto.HostAgent{externalHostAgent()})
		agtEqual(t, standIns.calls("claude"), 0)
		agtEqual(t, agtAgentRows(t, h.Store), "")
	})

	t.Run("non-zero exit excluded", func(t *testing.T) {
		newAgtStandIns(t, map[string]agtStandInSpec{
			"claude": {Stderr: "broken install\n", Code: 1},
			"codex":  {Stdout: "codex-cli 0.164.0\n", Code: 0},
		})
		h, _ := seamHost(t, seamStore(t))
		got := agtReadAgents(t, h)
		agtEqual(t, len(got), 2)
		agtEqual(t, got[0].Agent, "codex")
		agtEqual(t, got[0].CLIVersion, "0.164.0")
	})

	t.Run("hung version command times out without failing the other CLI", func(t *testing.T) {
		newAgtStandIns(t, map[string]agtStandInSpec{
			"claude": {Stdout: "2.1.311 (Claude Code)\n", SleepMS: 10000},
			"codex":  {Stdout: "codex-cli 0.164.0\n"},
		})
		h, _ := seamHost(t, seamStore(t))
		start := time.Now()
		got := agtReadAgents(t, h)
		agtEqual(t, len(got), 2)
		agtEqual(t, got[0].Agent, "codex")
		if elapsed := time.Since(start); elapsed < 4*time.Second || elapsed >= 9*time.Second {
			t.Fatalf("probe took %v, want bounded by 5s agtProbeSeconds", elapsed)
		}
	})
}

func TestAgentsCacheBoundary60SecondsAndHostIsolation(t *testing.T) {
	standIns := newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
	})
	start := time.UnixMilli(seamStart)
	now := start
	h, _ := seamHost(t, seamStore(t))
	h.Now = func() time.Time { return now }

	first := agtReadAgents(t, h)
	agtEqual(t, first[0].CLIVersion, "2.1.311")
	agtEqual(t, standIns.calls("claude"), 1)

	standIns.install("claude", agtStandInSpec{Stdout: "2.2.0 (Claude Code)\n"})

	// One nanosecond short of 60 s still reuses the cached answer.
	now = start.Add(60*time.Second - time.Nanosecond)
	beforeExpiry := agtReadAgents(t, h)
	agtEqual(t, beforeExpiry[0].CLIVersion, "2.1.311")
	agtEqual(t, standIns.calls("claude"), 1)

	// Another Host in the same process keeps its own Part and probes at once.
	other, _ := seamHost(t, seamStore(t))
	otherGot := agtReadAgents(t, other)
	agtEqual(t, otherGot[0].CLIVersion, "2.2.0")
	agtEqual(t, standIns.calls("claude"), 2)

	// At exact 60 s elapsed (now - start == 60s), h re-probes and updates the store.
	now = start.Add(60 * time.Second)
	atExpiry := agtReadAgents(t, h)
	agtEqual(t, atExpiry[0].CLIVersion, "2.2.0")
	agtEqual(t, standIns.calls("claude"), 3)
	agtEqual(t, agtAgentRows(t, h.Store), "claude|2.2.0|-|{}")
}

func TestAgentsCancelledContextDoesNotCache(t *testing.T) {
	standIns := newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
	})
	h, _ := seamHost(t, seamStore(t))
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := h.Agents(cancelled, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Agents err = %v, want context.Canceled", err)
	}
	agtEqual(t, got, []proto.HostAgent(nil))

	live := agtReadAgents(t, h)
	agtEqual(t, len(live), 2)
	agtEqual(t, live[0].Agent, "claude")
	agtEqual(t, standIns.calls("claude"), 1)
}

func TestAgentsLoginStatusTransitions(t *testing.T) {
	newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
		"codex":  {Stdout: "codex-cli 0.164.0\n"},
	})
	h, _ := seamHost(t, seamStore(t))
	claudeKey := SessionKey{Group: "g1", Member: "m1", Task: "t1"}
	codexKey := SessionKey{Group: "g1", Member: "m2", Task: "t2"}
	agtCheck(t, h.Sessions.Create(Session{SessionKey: claudeKey, Kind: TaskSession, Folder: "wt/1-a", Agent: "claude", Home: "home-1", Name: "Claude"}))
	agtCheck(t, h.Sessions.Create(Session{SessionKey: codexKey, Kind: TaskSession, Folder: "wt/2-b", Agent: "codex", Home: "home-1", Name: "Codex"}))

	assertLogin := func(wantClaude, wantCodex bool) {
		t.Helper()
		got := agtReadAgents(t, h)
		agtEqual(t, [2]bool{got[0].LoginOK, got[1].LoginOK}, [2]bool{wantClaude, wantCodex})
	}

	assertLogin(true, true)

	// Events for unknown sessions and neutral events leave login_ok unchanged.
	h.Report(SessionKey{Group: "missing"}, Event{Kind: LoginInvalid})
	h.Report(claudeKey, Event{Kind: TurnBegan})
	assertLogin(true, true)

	// LoginInvalid marks only that session's agent invalid; a second agent is tracked independently.
	h.Report(claudeKey, Event{Kind: LoginInvalid, Text: "not signed in"})
	assertLogin(false, true)
	h.Report(codexKey, Event{Kind: LoginInvalid, Text: "expired token"})
	assertLogin(false, false)

	// Neutral events (Ready included) and a failed TurnEnded keep login_ok false.
	h.Report(claudeKey, Event{Kind: Ready, Session: "sess-1"})
	h.Report(claudeKey, Event{Kind: Active})
	h.Report(claudeKey, Event{Kind: RateLimited, Resets: seamStart})
	h.Report(claudeKey, Event{Kind: TurnEnded, Status: "failed"})
	assertLogin(false, false)

	// TurnEnded with status completed or interrupted restores login_ok for that agent only.
	h.Report(claudeKey, Event{Kind: TurnEnded, Status: "completed"})
	assertLogin(true, false)

	h.Report(codexKey, Event{Kind: TurnEnded, Status: "interrupted"})
	assertLogin(true, true)
}

func TestAgentSetAccountsAndAudit(t *testing.T) {
	if Surface["agent.set"] == nil {
		t.Fatal("Surface[\"agent.set\"] is not registered")
	}

	standIns := newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
		"codex":  {Stdout: "codex-cli 0.164.0\n"},
	})
	db := seamStore(t)
	seamSQL(t, db, `INSERT INTO agent(agent, cli_version, caps) VALUES('claude', '2.1.311', '{"day":{"unit":"turns","max":50}}')`)
	h, _ := seamHost(t, db)
	now := time.UnixMilli(seamStart)
	h.Now = func() time.Time { return now }

	claudeKey := SessionKey{Group: "g1", Member: "m1"}
	agtCheck(t, h.Sessions.Create(Session{SessionKey: claudeKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: "home-1", Name: "Claude"}))
	agtReadAgents(t, h)
	agtEqual(t, standIns.calls("claude"), 1)
	agtEqual(t, standIns.calls("codex"), 1)
	h.Report(claudeKey, Event{Kind: LoginInvalid})

	// All three valid account types (including setting the same value twice) update the row without probing PATH, preserve caps, and restore login_ok.
	for _, account := range []string{"personal", "personal", "team", "api_key"} {
		agtCallSet(t, h, "owner-dev", agtSetPayload("claude", account))
	}
	agtEqual(t, standIns.calls("claude"), 1)
	agtEqual(t, standIns.calls("codex"), 1)

	goldenRaw := agtVal(os.ReadFile(filepath.Join("..", "proto", "testdata", "agent-set.json")))
	now = now.Add(5 * time.Second)
	agtCallSet(t, h, "owner-dev", goldenRaw)

	agents := agtReadAgents(t, h)
	apiKey := "api_key"
	personal := "personal"
	agtEqual(t, agents[0].Account, &apiKey)
	agtEqual(t, agents[0].LoginOK, true)
	agtEqual(t, agents[0].Caps.Day, &proto.Cap{Unit: "turns", Max: 50})
	agtEqual(t, agents[1].Account, &personal)

	wantAudit := strings.Join([]string{
		"1763600000000|-|claude|setting|-|-|account personal|owner-dev|-",
		"1763600000000|-|claude|setting|-|-|account personal|owner-dev|-",
		"1763600000000|-|claude|setting|-|-|account team|owner-dev|-",
		"1763600000000|-|claude|setting|-|-|account api_key|owner-dev|-",
		"1763600005000|-|codex|setting|-|-|account personal|owner-dev|-",
	}, "\n")
	agtEqual(t, seamRecord(t, h), wantAudit)
}

func TestAgentSetRefusals(t *testing.T) {
	handle := Surface["agent.set"]
	newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
	})
	h, _ := seamHost(t, seamStore(t))
	agtReadAgents(t, h)

	_, decodeErr := handle(t.Context(), h, nil, "owner-dev", json.RawMessage(`{"t":"agent.set"}`))
	seamCode(t, decodeErr, "bad_request")

	for _, bad := range []struct {
		raw  json.RawMessage
		want string
	}{
		{agtSetPayload("gemini", "personal"), "bad_request: unknown agent: gemini"},
		{agtSetPayload("", "personal"), "bad_request: unknown agent: "},
		{agtSetPayload("claude", "enterprise"), "bad_request: account enterprise is not in claude accounts"},
		{agtSetPayload("claude", ""), "bad_request: account  is not in claude accounts"},
		{agtSetPayload("codex", "personal"), "conflict: codex is not found on this machine"},
	} {
		res, err := handle(t.Context(), h, nil, "owner-dev", bad.raw)
		agtEqual(t, res, nil)
		agtErrText(t, err, bad.want)
	}
	agtEqual(t, seamRecord(t, h), "")
}

func TestAgentsStoreErrorsAndTransactionAtomicity(t *testing.T) {
	newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
	})

	t.Run("cold probe fails when agent table is missing", func(t *testing.T) {
		db := seamStore(t)
		seamSQL(t, db, "DROP TABLE agent")
		h, _ := seamHost(t, db)
		if _, err := h.Agents(t.Context(), false); err == nil || !strings.Contains(err.Error(), "no such table: agent") {
			t.Fatalf("got %v, want no such table: agent", err)
		}
	})

	t.Run("invalid caps JSON fails Agents", func(t *testing.T) {
		db := seamStore(t)
		h, _ := seamHost(t, db)
		agtReadAgents(t, h)
		seamSQL(t, db, "UPDATE agent SET caps = '{' WHERE agent = 'claude'")
		if _, err := h.Agents(t.Context(), false); err == nil {
			t.Fatal("expected JSON unmarshal error for malformed caps")
		}
	})

	t.Run("audit failure rolls back account write and keeps login_invalid", func(t *testing.T) {
		db := seamStore(t)
		h, _ := seamHost(t, db)
		key := SessionKey{Group: "g1", Member: "m1"}
		agtCheck(t, h.Sessions.Create(Session{SessionKey: key, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: "home-1", Name: "Claude"}))
		agtReadAgents(t, h)
		h.Report(key, Event{Kind: LoginInvalid})
		seamSQL(t, db, "DROP TABLE audit")

		if _, err := Surface["agent.set"](t.Context(), h, nil, "owner-dev", agtSetPayload("claude", "personal")); err == nil {
			t.Fatal("expected error when audit table is missing")
		}
		got := agtReadAgents(t, h)
		agtEqual(t, got[0].Account, (*string)(nil))
		agtEqual(t, got[0].LoginOK, false)
	})

	t.Run("missing agent table after warm cache fails Agents and agent.set", func(t *testing.T) {
		db := seamStore(t)
		h, _ := seamHost(t, db)
		agtReadAgents(t, h)
		seamSQL(t, db, "DROP TABLE agent")
		if _, err := h.Agents(t.Context(), false); err == nil || !strings.Contains(err.Error(), "no such table: agent") {
			t.Fatalf("Agents got %v, want no such table: agent", err)
		}
		if _, err := Surface["agent.set"](t.Context(), h, nil, "owner-dev", agtSetPayload("claude", "personal")); err == nil || !strings.Contains(err.Error(), "no such table: agent") {
			t.Fatalf("agent.set got %v, want no such table: agent", err)
		}
	})
}

func TestAgentsConcurrentAccess(t *testing.T) {
	newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
		"codex":  {Stdout: "codex-cli 0.164.0\n"},
	})
	h, _ := seamHost(t, seamStore(t))
	h.Now = time.Now
	key := SessionKey{Group: "g1", Member: "m1"}
	agtCheck(t, h.Sessions.Create(Session{SessionKey: key, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: "home-1", Name: "Claude"}))

	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, err := h.Agents(t.Context(), false); err != nil {
				t.Errorf("Agents: %v", err)
			}
			if n%2 == 0 {
				h.Report(key, Event{Kind: LoginInvalid})
			} else {
				h.Report(key, Event{Kind: TurnEnded, Status: "completed"})
			}
			if _, err := Surface["agent.set"](t.Context(), h, nil, "owner-dev", agtSetPayload("claude", "personal")); err != nil {
				t.Errorf("agent.set: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestAgentsDefaultsInHostInfo(t *testing.T) {
	newAgtStandIns(t, map[string]agtStandInSpec{
		"claude": {Stdout: "2.1.311 (Claude Code)\n"},
		"codex":  {Stdout: "codex-cli 0.164.0\n"},
	})
	db := seamStore(t)
	h, _ := seamHost(t, db)

	before := agtReadAgents(t, h)
	agtEqual(t, before[0].Defaults, (*proto.Profile)(nil))
	agtEqual(t, before[1].Defaults, (*proto.Profile)(nil))
	agtEqual(t, before[2].Defaults, (*proto.Profile)(nil))
	if strings.Contains(string(agtVal(json.Marshal(before))), `"defaults"`) {
		t.Fatalf("unexpected defaults field before agent.defaults: %s", agtVal(json.Marshal(before)))
	}

	_, err := Surface["agent.defaults"](t.Context(), h, nil, "owner-dev", defPayload(t, "claude", "opus", "high", "strong", "Go backend"))
	agtCheck(t, err)

	// Even if setting contains an external entry manually, externalHostAgent never shows Defaults.
	seamSQL(t, db, `UPDATE setting SET value = json_set(value, '$.external', json('{"model":"m","effort":"","tier":"strong","skills":""}')) WHERE key = 'defaults'`)
	after := agtReadAgents(t, h)
	want := &proto.Profile{Model: "opus", Effort: "high", Tier: "strong", Skills: "Go backend"}
	agtEqual(t, after[0].Defaults, want)
	agtEqual(t, after[1].Defaults, (*proto.Profile)(nil))
	agtEqual(t, after[2].Defaults, (*proto.Profile)(nil))

	seamSQL(t, db, `INSERT OR REPLACE INTO setting(key, value) VALUES('defaults', '{')`)
	if _, err := h.Agents(t.Context(), false); err == nil {
		t.Fatal("expected error for corrupt defaults JSON")
	}
}
