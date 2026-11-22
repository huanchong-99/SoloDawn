package host

import (
	"errors"
	"maps"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func saveDriverCaps(t *testing.T, next map[string]Capabilities) {
	t.Helper()
	before := maps.Clone(DriverCaps)
	DriverCaps = next
	t.Cleanup(func() { DriverCaps = before })
}

func assertValidCapabilities(t *testing.T, agent string, c Capabilities) {
	t.Helper()
	assertValidAccounts(t, agent, c.Accounts)
	assertValidLevels(t, agent, c.Levels)
	if c.Approvals != ApprovalTool && c.Approvals != ApprovalConn {
		t.Fatalf("%s approvals = %q, want tool or conn", agent, c.Approvals)
	}
	if c.Takeover != TakeoverNew && c.Takeover != TakeoverResume && c.Takeover != TakeoverNone {
		t.Fatalf("%s takeover = %q, want new, resume or none", agent, c.Takeover)
	}
	assertValidUsage(t, agent, c.Usage)
}

func assertValidAccounts(t *testing.T, agent string, accounts []string) {
	t.Helper()
	allowed := []string{AccountPersonal, AccountTeam, AccountAPIKey}
	if len(accounts) == 0 {
		t.Fatalf("%s accounts must not be empty", agent)
	}
	seen := map[string]bool{}
	for _, a := range accounts {
		if !slices.Contains(allowed, a) || seen[a] {
			t.Fatalf("%s invalid or duplicate account %q in %v", agent, a, accounts)
		}
		seen[a] = true
	}
}

func assertValidLevels(t *testing.T, agent string, levels []int) {
	t.Helper()
	if !slices.Contains(levels, 1) {
		t.Fatalf("%s levels %v must include level 1", agent, levels)
	}
	seen := map[int]bool{}
	for _, lvl := range levels {
		if lvl < 1 || lvl > 3 || seen[lvl] {
			t.Fatalf("%s invalid or duplicate level %d in %v", agent, lvl, levels)
		}
		seen[lvl] = true
	}
}

func assertValidUsage(t *testing.T, agent string, usage []string) {
	t.Helper()
	allowed := []string{"tokens", "usd", "window_pct"}
	if usage == nil {
		t.Fatalf("%s usage slice must be non-nil", agent)
	}
	seen := map[string]bool{}
	for _, u := range usage {
		if !slices.Contains(allowed, u) || seen[u] {
			t.Fatalf("%s invalid or duplicate usage unit %q in %v", agent, u, usage)
		}
		seen[u] = true
	}
}

func TestEveryDriverDeclaresValidCapabilities(t *testing.T) {
	drvKeys := slices.Sorted(maps.Keys(Drivers))
	capKeys := driverNames()
	agtEqual(t, capKeys, drvKeys)
	for _, name := range capKeys {
		assertValidCapabilities(t, name, DriverCaps[name])
	}
}

func TestStep1DriverDeclarationsMatchContract(t *testing.T) {
	agtEqual(t, DriverCaps["claude"], claudeCapabilities(runtime.GOOS))
	agtEqual(t, claudeCapabilities("darwin"), Capabilities{
		Accounts:  []string{"personal", "team", "api_key"},
		Levels:    []int{1, 2, 3},
		Approvals: "tool",
		Takeover:  "new",
		Usage:     []string{},
	})
	agtEqual(t, claudeCapabilities("linux"), Capabilities{
		Accounts:  []string{"personal", "team", "api_key"},
		Levels:    []int{1, 2, 3},
		Approvals: "tool",
		Takeover:  "new",
		Usage:     []string{},
	})
	agtEqual(t, claudeCapabilities("windows"), Capabilities{
		Accounts:  []string{"personal", "team", "api_key"},
		Levels:    []int{1, 3},
		Approvals: "tool",
		Takeover:  "new",
		Usage:     []string{},
	})
	agtEqual(t, DriverCaps["codex"], Capabilities{
		Accounts:  []string{"personal", "team", "api_key"},
		Levels:    []int{1, 2, 3},
		Approvals: "conn",
		Takeover:  "resume",
		Usage:     []string{},
	})
}

func TestSyntheticDriverAccountSwitch(t *testing.T) {
	next := maps.Clone(DriverCaps)
	next["api_only"] = Capabilities{
		Accounts:  []string{AccountAPIKey},
		Levels:    []int{1},
		Approvals: ApprovalConn,
		Takeover:  TakeoverNone,
		Usage:     []string{},
	}
	next["personal_only"] = Capabilities{
		Accounts:  []string{AccountPersonal},
		Levels:    []int{1, 2},
		Approvals: ApprovalTool,
		Takeover:  TakeoverNew,
		Usage:     []string{"tokens"},
	}
	next["team_only"] = Capabilities{
		Accounts:  []string{AccountTeam},
		Levels:    []int{1, 3},
		Approvals: ApprovalConn,
		Takeover:  TakeoverResume,
		Usage:     []string{"usd", "window_pct"},
	}
	saveDriverCaps(t, next)

	newAgtStandIns(t, map[string]agtStandInSpec{
		"api_only":      {Stdout: "api_only-cli 1.2.3\n"},
		"claude":        {Stdout: "2.1.311 (Claude Code)\n"},
		"personal_only": {Stdout: "0.9.0\n"},
		"team_only":     {Stdout: "team_only 2.0.0\n"},
	})
	h, _ := seamHost(t, seamStore(t))
	found := agtReadAgents(t, h)
	var names []string
	for _, a := range found {
		names = append(names, a.Agent)
	}
	agtEqual(t, names, []string{"api_only", "claude", "personal_only", "team_only"})

	assertAccountRefusal := func(agent, account string) {
		t.Helper()
		res, err := Surface["agent.set"](t.Context(), h, nil, "owner-dev", agtSetPayload(agent, account))
		agtEqual(t, res, nil)
		var hostErr *Error
		if !errors.As(err, &hostErr) || hostErr.Code != "bad_request" || !strings.Contains(hostErr.Message, "accounts") {
			t.Fatalf("(%s, %s) got %v, want bad_request naming accounts", agent, account, err)
		}
		agtErrText(t, err, "bad_request: account "+account+" is not in "+agent+" accounts")
	}

	assertAccountRefusal("api_only", "personal")
	assertAccountRefusal("api_only", "team")
	agtCallSet(t, h, "owner-dev", agtSetPayload("api_only", "api_key"))

	assertAccountRefusal("personal_only", "team")
	assertAccountRefusal("personal_only", "api_key")
	agtCallSet(t, h, "owner-dev", agtSetPayload("personal_only", "personal"))

	assertAccountRefusal("team_only", "personal")
	assertAccountRefusal("team_only", "api_key")
	agtCallSet(t, h, "owner-dev", agtSetPayload("team_only", "team"))

	after := agtReadAgents(t, h)
	apiKey, personal, team := "api_key", "personal", "team"
	agtEqual(t, after[0].Account, &apiKey)
	agtEqual(t, after[2].Account, &personal)
	agtEqual(t, after[3].Account, &team)
}

func TestSyntheticDriverApproveToolSwitch(t *testing.T) {
	next := maps.Clone(DriverCaps)
	next["synth_tool"] = Capabilities{
		Accounts:  []string{AccountAPIKey},
		Levels:    []int{1, 2, 3},
		Approvals: ApprovalTool,
		Takeover:  TakeoverNone,
		Usage:     []string{},
	}
	next["synth_conn"] = Capabilities{
		Accounts:  []string{AccountAPIKey},
		Levels:    []int{1, 2, 3},
		Approvals: ApprovalConn,
		Takeover:  TakeoverNone,
		Usage:     []string{},
	}
	saveDriverCaps(t, next)

	for agent, structured := range map[string]bool{"synth_tool": true, "synth_conn": false} {
		agtEqual(t, approveForCaller(Caller{Session: Session{Agent: agent}}), structured)
		agtEqual(t, approveForCaller(Caller{Session: Session{Agent: agent}, Interactive: true}), false)
	}
}

func TestSyntheticDriverTakeoverSwitch(t *testing.T) {
	next := maps.Clone(DriverCaps)
	next["synth_new"] = Capabilities{Takeover: TakeoverNew}
	next["synth_resume"] = Capabilities{Takeover: TakeoverResume}
	next["synth_none"] = Capabilities{Takeover: TakeoverNone}
	saveDriverCaps(t, next)
	for agent, want := range map[string][2]any{
		"synth_new":    {false, nil},
		"synth_resume": {true, nil},
		"synth_none":   {false, &Error{Code: "conflict", Message: "synth_none takeover is none"}},
	} {
		assignFirst, err := checkTakeover(agent)
		agtEqual(t, [2]any{assignFirst, err}, want)
	}
}

func TestTakeoverStartRefusesADriverWithoutTakeoverBeforeRelay(t *testing.T) {
	r := newTkRig(t)
	next := maps.Clone(DriverCaps)
	next["codex"] = Capabilities{Takeover: TakeoverNone}
	saveDriverCaps(t, next)
	tkCode(t, r.startFrom(newTkOwner("relay"), runAI2, ""), "conflict")
}
