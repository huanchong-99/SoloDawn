package proto

import (
	"strings"
	"testing"
)

// hostState is a node with two CLIs: Claude Code, declared and working on a
// task in the examples' group, and Codex, found but not declared yet.
func hostState() HostState {
	return HostState{
		Device: host, Name: "workstation", OS: "macOS 15.2", Version: "v0.3.1", Open: 2, MaxOpen: 4,
		Homes: []Home{{ID(group), host, "Website"}},
		Relay: &RelayUse{3250000, 52428800},
		Agents: []HostAgent{
			{"claude", "2.1.311", new("personal"), true, Caps{Day: &Cap{"turns", 200}, Task: &Cap{"usd", 12.5}},
				Driver{[]string{"personal", "team", "api_key"}, "new"},
				[]AgentGroup{{ID(group), ID(agent), 2, "working", ID(taskID), 1}}},
			{"codex", "0.164.0", nil, false, Caps{},
				Driver{[]string{"personal", "team", "api_key"}, "resume"},
				[]AgentGroup{}},
		},
		Settings: HostSettings{90, 15, 2, false, []string{}},
	}
}

func TestHostSurfaceGoldens(t *testing.T) {
	golden(t, "groups", Groups{"groups", []GroupSummary{{group, "Website", 812}}})

	golden(t, "host-info-request", Request{"host.info", 9})
	golden(t, "host-info-answer", HostInfo{"host.info", Success{9, true}, hostState()})
	// The owner took all AIs offline after an update was installed, and the
	// rendezvous is not reachable: what the answer showed changes in a push.
	state := hostState()
	state.Open, state.Offline = 0, true
	state.Update = &Update{"v0.3.2", "pending"}
	state.Relay = nil
	state.Agents[0].Groups[0].State, state.Agents[0].Groups[0].Queued = "offline", 3
	state.Settings.Approvers = []string{user}
	golden(t, "host-info-push", HostInfoPush{"host.info", state})

	// The next page after rows down to 413: the rows whose id is lower.
	golden(t, "audit-request", Audit{Request{"audit", 10}, 413, 50})
	golden(t, "audit-answer", AuditAnswer{Success{10, true}, []AuditRow{
		{412, ts - 1000, ID(group), "claude", "approval", "git push origin other-branch", user, "approved"},
		{ID: 411, TS: ts - 1000, What: "setting", Detail: "max_open 4", By: user},
	}})

	golden(t, "pty-resize", PTYResize{"pty.resize", ID(group), ID(agent), 120, 32})
	golden(t, "beacon", Beacon{"beacon", ID(id("beacon")), ts, 47800})
}

func TestHostSettingsAndTakeoverGoldens(t *testing.T) {
	to := func(kind string) HostCommand { return HostCommand{kind, ID(id(kind)), ts, host} }
	golden(t, "settings-set", SettingsSet{to("settings.set"), new(30), new(20), new(3), new(2), new(false), &[]string{user},
		&AgentCaps{Claude: &Caps{&Cap{"turns", 200}, &Cap{"turns", 60}}, Codex: &Caps{}}})
	golden(t, "agent-set", AgentSet{to("agent.set"), "codex", "personal"})
	golden(t, "perm-set", PermSet{to("perm.set"), "claude", ID(group), 3})
	golden(t, "pair-browser", to("pair.browser"))
	golden(t, "host-offline", to("host.offline"))
	golden(t, "host-online", to("host.online"))
	golden(t, "takeover-start", TakeoverStart{Takeover{to("takeover.start"), ID(group), ID(agent)}, ID(taskID)})
	golden(t, "takeover-end", Takeover{to("takeover.end"), ID(group), ID(agent)})
}

// A set that sends a setting as 0, false or an empty list changes it to that;
// one that leaves a setting out keeps it.
func TestSettingsSetTellsAZeroFromNothing(t *testing.T) {
	header := `"t":"settings.set","n":"` + id("n") + `","ts":1,"to":"` + host + `"`
	var zero, none SettingsSet
	if err := Decode([]byte(`{`+header+`,"retention_days":0,"all_level3":false,"approvers":[],"caps":{"codex":{}}}`), &zero); err != nil {
		t.Fatal(err)
	}
	if *zero.RetentionDays != 0 || *zero.AllLevel3 || len(*zero.Approvers) != 0 || zero.Caps.Claude != nil || *zero.Caps.Codex != (Caps{}) {
		t.Errorf("zero values read as %+v", zero)
	}
	if err := Decode([]byte(`{`+header+`}`), &none); err != nil || none != (SettingsSet{HostCommand: none.HostCommand}) {
		t.Errorf("nothing changed read as %+v, %v", none, err)
	}
	for _, bad := range []string{`"max_open":null`, `"caps":{"claude":null}`, `"caps":{"claude":{"day":null}}`, `"caps":{"codex":{"day":{"unit":"turns"}}}`} {
		if err := Decode([]byte(`{`+header+`,`+bad+`}`), new(SettingsSet)); err == nil {
			t.Errorf("%s is read", bad)
		}
	}
	// An agent whose name holds an upper-case letter is another name, ignored.
	var other SettingsSet
	if err := Decode([]byte(`{`+header+`,"caps":{"Claude":{"day":null}}}`), &other); err != nil || other.Caps.Claude != nil {
		t.Errorf("an upper-case agent read as %+v, %v", other.Caps, err)
	}
}

// An agent that is not declared yet has the account null, and a node that has
// no quota answer the relay null; anything else there is a value.
func TestHostInfoNullsAreTheContractsNulls(t *testing.T) {
	for name, allowed := range map[string][]string{
		"host-info-answer": {"/agents/1/account", "/update"},
		"host-info-push":   {"/agents/1/account", "/relay"},
	} {
		if nulls := UnexpectedNulls(example(t, name), allowed...); len(nulls) != 0 {
			t.Errorf("%s has null at %q", name, nulls)
		}
		if nulls := UnexpectedNulls(example(t, name)); len(nulls) != len(allowed) {
			t.Errorf("%s has the nulls %q, want %q", name, nulls, allowed)
		}
	}
}

// What the host surface answers is told from what it pushes by re: the answer
// to host.info has it, the push does not.
func TestHostInfoAnswerAndPushDiffer(t *testing.T) {
	if err := Decode(example(t, "host-info-push"), new(HostInfo)); err != errMissing {
		t.Errorf("a push read as an answer: %v, want %v", err, errMissing)
	}
	answer := string(example(t, "host-info-answer"))
	if !strings.Contains(answer, `"re": 9`) || strings.Contains(string(example(t, "host-info-push")), `"re"`) {
		t.Errorf("the answer has no re, or the push has one")
	}
}
