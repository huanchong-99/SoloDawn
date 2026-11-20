package proto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

// command starts a home command signed by a person, or by a host for itself.
func command(kind, label string) HomeCommand {
	return HomeCommand{Type: kind, Nonce: ID(id(label)), Timestamp: ts, Group: ID(group)}
}

// The bytes of a key in an example: reproducible, and no key of anything.
func keyBytes(label string, size int) string {
	h := sha256.Sum256([]byte("SoloDawn protocol example: " + label))
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat(h[:], 3)[:size])
}

var limits = Limits{10, 20, 60, 3, 5, 4, 45, 600, 30}

func TestGroupCommandGoldens(t *testing.T) {
	// The new group's id is the command's nonce, and g names it like any group.
	create := GroupCommand{HomeCommand{Type: "group", Nonce: ID(group), Timestamp: ts - 5000, Group: ID(group)}, "create"}
	golden(t, "group-create", GroupCreate{create, "Website", "project", "https://git.example/team/website.git", "main", "github",
		[]string{"go vet ./...", "go test ./..."}, "owner"})
	golden(t, "group-set", GroupSet{GroupCommand{command("group", "group set"), "set"},
		new("Website"), &[]string{"go vet ./...", "go test ./..."}, new(90), new(70), &limits, ID(secretary)})
	golden(t, "group-pause", GroupCommand{command("group", "group pause"), "pause"})
	golden(t, "group-resume", GroupCommand{command("group", "group resume"), "resume"})
}

func member(op string) MemberCommand {
	return MemberCommand{command("member", "member "+op), op, ID(agent)}
}

func TestMemberCommandGoldens(t *testing.T) {
	golden(t, "member-add_ai", MemberAddAI{member("add_ai"), "codex", "Codex", "personal", false, []string{user}})
	golden(t, "member-status", MemberStatus[SystemCode]{MemberCommand: member("status"), State: "working", Task: ID(taskID), Path: "local",
		Usage: Usage{WindowPct: new(62.5), WindowResets: new(ts + 10000), DayCapPct: new(80.0),
			SpendToday: new(0.0), SpendCap: new(10.5), TurnsToday: new(14)}})
	golden(t, "member-set", MemberSet{member("set"), new("Codex 2"), new("team"), &[]string{}, &[]string{user}})
	golden(t, "member-remove", member("remove"))
}

// status is a member status that reports a fact; the home writes the fact as
// a system message, with the AI's id in ai.
func status[E any](state string, event E) MemberStatus[E] {
	s := MemberStatus[E]{MemberCommand: member("status"), State: state, Path: "local", Usage: Usage{}, Event: &event}
	s.Nonce = ID(id("member status " + state + " " + reflect.TypeFor[E]().Name()))
	return s
}

func TestHostEventGoldens(t *testing.T) {
	golden(t, "member-status-turn_interrupted", status("dormant",
		TurnInterruptedEvent{SystemCode{"turn_interrupted"}, "timeout", ID(taskID), "Turn deadline reached."}))
	golden(t, "member-status-cap_reached", status("limited", CapReachedEvent{SystemCode{"cap_reached"}, "task", ID(taskID)}))
	golden(t, "member-status-rate_limited", status("limited", RateLimitedEvent{SystemCode{"rate_limited"}, ts + 3600000}))
	golden(t, "member-status-login_invalid", status("limited", LoginInvalidEvent{SystemCode{"login_invalid"}, "Signed out of the CLI."}))
	golden(t, "member-status-session_replaced", status("working", SessionReplacedEvent{SystemCode{"session_replaced"}, ID(taskID)}))
}

// A host event is the system body of its code without ai, which the home adds:
// with ai, it is read as that body, and nothing of it is lost.
func TestAHostEventIsItsSystemBodyWithoutAI(t *testing.T) {
	for _, tc := range []struct {
		event, body any
	}{
		{TurnInterruptedEvent{SystemCode{"turn_interrupted"}, "crash", ID(taskID), "exit status 1"}, new(TurnInterruptedBody)},
		{CapReachedEvent{SystemCode{"cap_reached"}, "day", ID(taskID)}, new(CapReachedBody)},
		{RateLimitedEvent{SystemCode{"rate_limited"}, 42}, new(RateLimitedBody)},
		{LoginInvalidEvent{SystemCode{"login_invalid"}, "signed out"}, new(LoginInvalidBody)},
		{SessionReplacedEvent{SystemCode{"session_replaced"}, ID(taskID)}, new(SessionReplacedBody)},
	} {
		fact := jsonValue(t, wireJSON(t, tc.event)).(map[string]any)
		if err := Decode(wireJSON(t, fact), tc.body); err != errMissing {
			t.Errorf("%T read without ai: %v, want %v", tc.body, err, errMissing)
		}
		fact["ai"] = agent
		if err := Decode(wireJSON(t, fact), tc.body); err != nil || !reflect.DeepEqual(jsonValue(t, wireJSON(t, tc.body)), any(fact)) {
			t.Errorf("%T read %+v, %v\nwant %v", tc.body, tc.body, err, fact)
		}
	}
}

// A status with nothing to report has no task and no event, and usage is {}.
func TestMemberStatusWithNothingToReport(t *testing.T) {
	quiet := MemberStatus[SystemCode]{MemberCommand: member("status"), State: "dormant", Path: "relay", Usage: Usage{}}
	encoded := wireJSON(t, quiet)
	if nulls := UnexpectedNulls(encoded); len(nulls) != 0 {
		t.Errorf("nulls at %q in %s", nulls, encoded)
	}
	var read MemberStatus[SystemCode]
	if err := Decode(encoded, &read); err != nil || !reflect.DeepEqual(read, quiet) {
		t.Errorf("read %+v, %v\nwant %+v", read, err, quiet)
	}
	fields := jsonValue(t, encoded).(map[string]any)
	if _, there := fields["task"]; there || fields["event"] != nil || !reflect.DeepEqual(fields["usage"], map[string]any{}) {
		t.Errorf("encoded %s", encoded)
	}
}

func task(op string) TaskCommand {
	return TaskCommand{command("task", "task "+op), op, ID(taskID)}
}

func TestTaskCommandGoldens(t *testing.T) {
	start := TaskCommand{command("task", "task start"), "start", ID(id("local task"))}
	start.As = ID(agent)
	golden(t, "task-start", TaskStart{start, "Choose a label", "Ask the owner which label the path gets.", new(ID(id("text"))), "direct"})
	golden(t, "task-branch", TaskBranch{task("branch"), taskExample().Branch})
	items := []CheckItem{{"go test ./...", true, new("42 tests")}}
	golden(t, "task-checked", TaskChecked{task("checked"), true, items, commit})
	golden(t, "task-submitted", TaskSubmitted{task("submitted"), commit, 31, "https://git.example/team/website/pull/31", DiffStats{212, 18}})
	golden(t, "task-cloud", TaskChecked{task("cloud"), true, []CheckItem{{Name: "ci / test", OK: true}}, commit})
	golden(t, "task-merged", TaskMerged{task("merged"), commit, "c1d9e7a2f0b34455aa8f0e6d7c2b1a9988776655", DiffStats{212, 18}})
	golden(t, "task-conflict", TaskConflict{task("conflict"), commit, "CONFLICT (content): Merge conflict in path.go"})
	golden(t, "task-fail", TaskFail{task("fail"), "timeout", "Turn deadline reached."})
	golden(t, "task-preempted", TaskPreempted{task("preempted"), ID(id("text"))})
	golden(t, "task-assign", TaskAssign{task("assign"), ID(agent), ID(reviewer)})
	golden(t, "task-pause", task("pause"))
	golden(t, "task-resume", task("resume"))
	golden(t, "task-cancel", task("cancel"))
}

// Without a remote a submission has no pull request, and an assign may name one role only.
func TestTaskCommandsLeaveOutWhatDoesNotApply(t *testing.T) {
	for _, tc := range []struct {
		command any
		want    string
	}{
		{TaskSubmitted{TaskCommand: task("submitted"), Commit: "abc", Stats: DiffStats{1, 2}}, `"commit":"abc","stats":{"added":1,"removed":2}}`},
		{TaskAssign{TaskCommand: task("assign"), Reviewer: ID(reviewer)}, `"id":"` + taskID + `","reviewer":"` + reviewer + `"}`},
		{TaskAssign{TaskCommand: task("assign"), Assignee: ID(agent)}, `"id":"` + taskID + `","assignee":"` + agent + `"}`},
		{GroupSet{GroupCommand: GroupCommand{command("group", "group set"), "set"}, Checks: &[]string{}}, `"op":"set","checks":[]}`},
		{MemberSet{MemberCommand: member("set"), Approvers: &[]string{}}, `"id":"` + agent + `","approvers":[]}`},
	} {
		encoded := string(wireJSON(t, tc.command))
		if got := encoded[len(encoded)-len(tc.want):]; got != tc.want {
			t.Errorf("%T ends on %s, want %s", tc.command, got, tc.want)
		}
		read := reflect.New(reflect.TypeOf(tc.command))
		if err := Decode([]byte(encoded), read.Interface()); err != nil || !reflect.DeepEqual(read.Elem().Interface(), tc.command) {
			t.Errorf("read %+v, %v\nwant %+v", read.Elem(), err, tc.command)
		}
	}
}

func TestOtherHomeCommandGoldens(t *testing.T) {
	golden(t, "jump", Jump{command("jump", "jump"), ID(id("text"))})
	golden(t, "note", Note{command("note", "note command"), ID(id("note")), nil, "Path events include the connection id."})
	golden(t, "forget", Forget{command("forget", "forget"), "note", ID(id("note"))})
	on := PushOn{command("push", "push on"),
		PushSubscription{"https://push.example/send/" + id("subscription"), keyBytes("p256dh", 65), keyBytes("auth", 16)},
		VAPIDKeys{keyBytes("vapid public", 65), keyBytes("vapid private", 32)},
		[]string{"mention", "approval", "decision", "brief", "done"}}
	golden(t, "push-on", on)
	golden(t, "push-off", PushOff{command("push", "push off"), true})
	golden(t, "push-reminder", Reminder{ID(group), ID(id("approval")), "Website", "approval", "Codex", "Time the path change in a browser."})
}

// A push whose off is true ends the reminders, whatever else it holds; any
// other push is read as a subscription (contracts/commands.md 1.7).
func TestPushIsOffOnlyWhenOffIsTrue(t *testing.T) {
	on := jsonValue(t, example(t, "push-on")).(map[string]any)
	for _, tc := range []struct {
		off   any
		isOff bool
	}{{true, true}, {false, false}, {"true", false}, {1, false}} {
		on["off"] = tc.off
		var off PushOff
		if err := Decode(wireJSON(t, on), &off); (err == nil) != tc.isOff {
			t.Errorf("off %v read as an end of reminders: %v", tc.off, err)
		}
		var sub PushOn
		if err := Decode(wireJSON(t, on), &sub); err != nil || sub.Sub.Endpoint == "" {
			t.Errorf("off %v beside a subscription: %+v, %v", tc.off, sub, err)
		}
	}
	var off PushOff
	if err := Decode([]byte(`{"t":"push","n":"`+id("n")+`","ts":1,"g":"`+group+`","off":false}`), &off); err != errNotTrue {
		t.Errorf("off false alone: %v, want %v", err, errNotTrue)
	}
}

func TestHostCommandGoldens(t *testing.T) {
	to := func(kind string) HostCommand { return HostCommand{kind, ID(id(kind)), ts, host} }
	golden(t, "host-join", HostJoin{to("host.join"), ID(group), host, "codex", "Codex", false})
	golden(t, "host-leave", HostLeave{to("host.leave"), ID(group), "codex"})
}

// The example of each command names what a receiver reads to choose its type:
// t, and the op, kind or action beside it.
func TestCommandExamplesAreNamedByWhatSelectsThem(t *testing.T) {
	for _, name := range exampleNames(t) {
		fields, _ := jsonValue(t, example(t, name)).(map[string]any)
		kind, _ := fields["t"].(string)
		var want string
		switch kind {
		case "group", "member", "task":
			want = kind + "-" + fields["op"].(string)
			if event, there := fields["event"].(map[string]any); there {
				want += "-" + event["code"].(string)
			}
		case "post":
			want = "post-" + fields["kind"].(string)
		case "act":
			want = "act-" + fields["action"].(string)
		case "item":
			want = "item-" + fields["why"].(string)
		case "jump", "note", "forget", "ack", "inbox":
			want = kind
		case "host.join", "host.leave", "host.offline", "host.online", "settings.set", "pair.browser", "agent.set", "perm.set",
			"takeover.start", "takeover.end", "pty.resize":
			want = strings.ReplaceAll(kind, ".", "-")
		case "groups", "beacon":
			want = kind
		case "host.info", "audit": // a request, its answer, a push
			if want = strings.ReplaceAll(kind, ".", "-") + "-"; strings.HasPrefix(name, want) {
				continue
			}
		default:
			continue
		}
		if name != want {
			t.Errorf("%s holds the example of %s", name, want)
		}
	}
}
