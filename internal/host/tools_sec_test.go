package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	secGroupID   = "rZNvy-1jH6Z-BcPqA5U5BQ"
	secSecID     = "qBSFMsr2hHYKOMblEA_kdA"
	secClaudeID  = "1PC8WineBrUQ-apCjx7tug"
	secCodexID   = "LXCZmuGAXkvO-bSrOkuCfw"
	secOwnerMID  = "4xq2Q8RPeg7IJLWdEZTWDQ"
	secAliceMID  = "9kbEU81-A3uJPqD1MdM1WA"
	secBriefID   = "KaiCW9JC8UOG7lKNduDo8Q"
	secTask12ID  = "DrtCn6htSBwmMPrFPbHJHA"
	secTask2ID   = "hVhh0SQDDWIzdLF3fDLiBA"
	secOwnerUID  = "akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs"
	secAliceUID  = "J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q"
	secTestToken = "sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"
)

type secTestHome struct {
	done     chan struct{}
	requests []any
	onReq    func(id int64, req any) (json.RawMessage, error)
}

func newSecTestHome(onReq func(id int64, req any) (json.RawMessage, error)) *secTestHome {
	return &secTestHome{done: make(chan struct{}), onReq: onReq}
}

func (f *secTestHome) Send(any) error               { return nil }
func (f *secTestHome) Path() string                 { return "local" }
func (f *secTestHome) PathChanged() <-chan struct{} { return nil }
func (f *secTestHome) Done() <-chan struct{}        { return f.done }

func (f *secTestHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	id := int64(len(f.requests) + 1)
	req := build(id)
	f.requests = append(f.requests, req)
	if f.onReq != nil {
		return f.onReq(id, req)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func secPutJSON(h *Host, r, id string, v any) {
	raw, _ := json.Marshal(v)
	h.Rows.Put(secGroupID, r, id, raw)
}

func secMustEq(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func secMustTrue(t *testing.T, ok bool, detail string) {
	t.Helper()
	if !ok {
		t.Fatal(detail)
	}
}

func newSecFixture(t *testing.T) (*Host, *secTestHome) {
	t.Helper()
	h, _ := seamHost(t, seamStore(t))
	home := newSecTestHome(nil)
	h.Attach("home-dev", home)
	secPutJSON(h, "group", secGroupID, proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: secGroupID, Seq: 1},
		Name:           "Website",
		Kind:           "project",
		Base:           "main",
		Forge:          "none",
		Home:           "home-dev",
		Secretary:      secSecID,
		Checks:         []string{},
		Pass:           85,
		PassFoundation: 70,
		Created:        seamStart,
	})
	secPutJSON(h, "member", secOwnerMID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: secOwnerMID, Seq: 2}, Group: secGroupID, Kind: "human", User: secOwnerUID, Name: "owner", LastSeen: seamStart},
		Role:      "owner",
		Online:    true,
	})
	secPutJSON(h, "member", secAliceMID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: secAliceMID, Seq: 3}, Group: secGroupID, Kind: "human", User: secAliceUID, Name: "Alice", LastSeen: seamStart},
		Role:      "member",
		Online:    false,
	})
	secPutJSON(h, "member", secSecID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: secSecID, Seq: 4}, Group: secGroupID, Kind: "ai", User: secOwnerUID, Name: "Secretary", LastSeen: seamStart},
		CLI:       "claude", Host: seamKey().ID(), HostName: "ws", Secretary: true, Account: "personal", Allow: []string{}, Approvers: []string{secOwnerUID}, State: "working", Path: "local",
	})
	secPutJSON(h, "member", secClaudeID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: secClaudeID, Seq: 5}, Group: secGroupID, Kind: "ai", User: secOwnerUID, Name: "Claude Code", LastSeen: seamStart},
		CLI:       "claude", Host: seamKey().ID(), HostName: "ws", Account: "personal", Allow: []string{}, Approvers: []string{secOwnerUID}, State: "working", Path: "local",
	})
	secPutJSON(h, "member", secCodexID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: secCodexID, Seq: 6}, Group: secGroupID, Kind: "ai", User: secOwnerUID, Name: "Codex", LastSeen: seamStart},
		CLI:       "codex", Host: seamKey().ID(), HostName: "ws", Account: "personal", Allow: []string{}, Approvers: []string{secOwnerUID}, State: "waiting", Path: "local",
	})
	h.Rows.Put(secGroupID, "member", "gone-member", json.RawMessage(`{"r":"member","id":"gone-member","seq":7,"gone":true}`))
	secPutJSON(h, "task", secTask12ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: secTask12ID, Seq: 8},
		Group:     secGroupID, Num: 12, Title: "Show path", Slug: "show-path", Origin: "brief", Brief: secBriefID,
		State: "active", Step: "local_check", Assignee: secClaudeID, Reviewer: secCodexID, Pass: 85, Depends: []string{},
		Stats: proto.TaskStats{Started: seamStart}, Created: seamStart,
	})
	secPutJSON(h, "task", secTask2ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: secTask2ID, Seq: 9},
		Group:     secGroupID, Num: 2, Title: "Direct fix", Slug: "direct-fix", Origin: "direct",
		State: "paused", Step: "branch", Assignee: secCodexID, Pass: 85, Depends: []string{},
		Stats: proto.TaskStats{Started: seamStart}, Created: seamStart,
	})
	h.Rows.Put(secGroupID, "task", "gone-task", json.RawMessage(`{"r":"task","id":"gone-task","seq":10,"gone":true}`))
	return h, home
}

func secSecretaryCaller(thread string) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: secGroupID, Member: secSecID},
			Kind:       GroupSession,
			Home:       "home-dev",
			Name:       "Secretary",
			Secretary:  true,
		},
		Level: 1,
		Turn:  Turn{Kind: ItemsTurn, Thread: thread},
	}
}

func secWorkerCaller(kind Kind, member, task string) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: secGroupID, Member: member, Task: task},
			Kind:       kind,
			Home:       "home-dev",
			Name:       "Claude Code",
		},
		Level: 2,
		Turn:  Turn{Kind: ItemsTurn, Thread: task},
	}
}

func secOpenCmd[T any](t *testing.T, h *Host, req any) T {
	t.Helper()
	cmd, ok := req.(proto.Cmd)
	secMustTrue(t, ok, "expected proto.Cmd")
	trusted := func(dev string) bool { return dev == h.Key.ID() }
	_, err := identity.OpenCommand(cmd.Env, "", trusted, h.Now())
	secMustEq(t, err, nil)
	var payload T
	secMustEq(t, proto.Decode([]byte(cmd.Env.Payload), &payload), nil)
	return payload
}

func secGoldenSchema(t *testing.T, list proto.ToolList, name string) any {
	t.Helper()
	idx := slices.IndexFunc(list.Tools, func(tl proto.Tool) bool { return tl.Name == name })
	secMustTrue(t, idx >= 0, "missing tool in tools.json: "+name)
	raw, _ := json.Marshal(list.Tools[idx].InputSchema)
	var v any
	secMustEq(t, json.Unmarshal(raw, &v), nil)
	return v
}

func TestSecToolsMatchGoldenSchemas(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	secMustEq(t, err, nil)
	var lists proto.ToolLists
	secMustEq(t, json.Unmarshal(data, &lists), nil)

	wantSchemas := map[string]any{
		"propose_brief":    secGoldenSchema(t, lists.Secretary, "propose_brief"),
		"set_task":         secGoldenSchema(t, lists.Secretary, "set_task"),
		"request_decision": secGoldenSchema(t, lists.Group, "request_decision"),
	}
	for name, want := range wantSchemas {
		tool, ok := Tools[name]
		secMustTrue(t, ok, "missing registration: "+name)
		secMustTrue(t, tool.Description != "", "empty description: "+name)
		secMustTrue(t, tool.Call != nil, "nil Call: "+name)
		var got any
		secMustEq(t, json.Unmarshal(tool.Schema, &got), nil)
		secMustEq(t, got, want)
	}
}

func TestSecToolsSessionVisibility(t *testing.T) {
	secListed := ToolsFor(secSecretaryCaller(""))
	for _, name := range []string{"propose_brief", "set_task", "request_decision"} {
		_, ok := secListed[name]
		secMustTrue(t, ok, name+" not listed to secretary")
	}
	for _, c := range []Caller{
		secWorkerCaller(GroupSession, secClaudeID, ""),
		secWorkerCaller(TaskSession, secClaudeID, secTask12ID),
		secWorkerCaller(ReviewSession, secCodexID, secTask12ID),
	} {
		listed := ToolsFor(c)
		_, hasBrief := listed["propose_brief"]
		_, hasSet := listed["set_task"]
		_, hasDec := listed["request_decision"]
		secMustEq(t, hasBrief, false)
		secMustEq(t, hasSet, false)
		secMustEq(t, hasDec, true)
	}
}

func TestProposeBriefBuildsSignedPostWithPassAndMentions(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	// Explicit requester, replaces (home id, not scanned as free text), foundation true/default, depends [0]/default.
	rawArgs := json.RawMessage(`{
		"title": "Connection indicator",
		"goal": "Show path state.",
		"criteria": ["Updates within 1 s"],
		"scope_in": ["host", "web"],
		"scope_out": ["relay"],
		"tasks": [
			{"title": "Host events", "assignee": "Claude Code", "reviewer": "Codex", "foundation": true, "depends": []},
			{"title": "Web badge", "assignee": "Codex", "reviewer": "Claude Code", "depends": [0]}
		],
		"requester": "Alice",
		"replaces": "` + secTestToken + `"
	}`)
	out, err := Tools["propose_brief"].Call(t.Context(), h, c, rawArgs)
	secMustEq(t, err, nil)
	reply := out.(secIDReply)
	secMustEq(t, len(reply.ID), 22)
	secMustEq(t, len(home.requests), 1)

	replaces := secTestToken
	want := proto.Post[proto.BriefBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(reply.ID), Timestamp: seamStart, Group: secGroupID, As: secSecID,
		},
		Kind:     "brief",
		Mentions: []proto.ID{secAliceMID},
		Body: proto.BriefBody{
			Title:    "Connection indicator",
			Goal:     "Show path state.",
			Criteria: []string{"Updates within 1 s"},
			Scope:    proto.BriefScope{In: []string{"host", "web"}, Out: []string{"relay"}},
			Tasks: []proto.BriefTask{
				{Title: "Host events", Assignee: secClaudeID, Reviewer: secCodexID, Foundation: true, Depends: []int{}},
				{Title: "Web badge", Assignee: secCodexID, Reviewer: secClaudeID, Foundation: false, Depends: []int{0}},
			},
			Pass:      85,
			Requester: secAliceMID,
			Replaces:  &replaces,
		},
	}
	secMustEq(t, secOpenCmd[proto.Post[proto.BriefBody]](t, h, home.requests[0]), want)

	// Omitted replaces and omitted depends encode as null and [] on the wire.
	home.requests = nil
	out2, err := Tools["propose_brief"].Call(t.Context(), h, c, json.RawMessage(`{
		"title": "Second brief",
		"goal": "Keep defaults.",
		"criteria": ["Clean"],
		"scope_in": [],
		"scope_out": [],
		"tasks": [{"title": "Only task", "assignee": "Claude Code", "reviewer": "Codex"}],
		"requester": "owner"
	}`))
	secMustEq(t, err, nil)
	cmd2 := home.requests[0].(proto.Cmd)
	secMustTrue(t, strings.Contains(cmd2.Env.Payload, `"replaces":null`), cmd2.Env.Payload)
	secMustTrue(t, strings.Contains(cmd2.Env.Payload, `"depends":[]`), cmd2.Env.Payload)
	post2 := secOpenCmd[proto.Post[proto.BriefBody]](t, h, cmd2)
	secMustTrue(t, out2.(secIDReply).ID != reply.ID, "expected distinct nonces")
	secMustEq(t, post2.Body.Replaces, (*string)(nil))
	secMustEq(t, post2.Body.Requester, secOwnerMID)
}

func TestProposeBriefDefaultRequesterSingleMultiAndZeroPeople(t *testing.T) {
	const minimalBrief = `{
		"title": "Brief",
		"goal": "Goal",
		"criteria": ["Done"],
		"scope_in": [],
		"scope_out": [],
		"tasks": [{"title": "Work", "assignee": "Claude Code", "reviewer": "Codex"}]
	}`

	// 1. Two people in fixture (Alice, owner): omitted requester is refused naming both.
	h, home := newSecFixture(t)
	_, err := Tools["propose_brief"].Call(t.Context(), h, secSecretaryCaller(""), json.RawMessage(minimalBrief))
	seamCode(t, err, "bad_request")
	secMustTrue(t, strings.Contains(err.Error(), "more than one person (Alice, owner)"), err.Error())
	secMustEq(t, len(home.requests), 0)

	// 2. One person in group: omitted requester uses that person without fetching.
	h.Rows.Put(secGroupID, "member", secAliceMID, json.RawMessage(`{"r":"member","id":"`+secAliceMID+`","seq":11,"gone":true}`))
	_, err = Tools["propose_brief"].Call(t.Context(), h, secSecretaryCaller(""), json.RawMessage(minimalBrief))
	secMustEq(t, err, nil)
	secMustEq(t, len(home.requests), 1)
	post := secOpenCmd[proto.Post[proto.BriefBody]](t, h, home.requests[0])
	secMustEq(t, post.Body.Requester, secOwnerMID)
	secMustEq(t, post.Mentions, []proto.ID{secOwnerMID})

	// 3. Zero people in group: omitted requester is refused with bad_request.
	h.Rows.Put(secGroupID, "member", secOwnerMID, json.RawMessage(`{"r":"member","id":"`+secOwnerMID+`","seq":12,"gone":true}`))
	_, err = Tools["propose_brief"].Call(t.Context(), h, secSecretaryCaller(""), json.RawMessage(minimalBrief))
	seamCode(t, err, "bad_request")
	secMustTrue(t, strings.Contains(err.Error(), "no person"), err.Error())
}

func secBaseBriefMap() map[string]any {
	return map[string]any{
		"title":     "Clean title",
		"goal":      "Clean goal",
		"criteria":  []string{"Clean criterion"},
		"scope_in":  []string{"in"},
		"scope_out": []string{"out"},
		"tasks": []map[string]any{
			{"title": "Clean task", "assignee": "Claude Code", "reviewer": "Codex"},
		},
		"requester": "owner",
	}
}

func TestProposeBriefSecretGuard(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")
	close(home.done)

	mutations := []func(map[string]any){
		func(m map[string]any) { m["title"] = secTestToken },
		func(m map[string]any) { m["goal"] = secTestToken },
		func(m map[string]any) { m["criteria"] = []string{"ok", secTestToken} },
		func(m map[string]any) { m["scope_in"] = []string{secTestToken} },
		func(m map[string]any) { m["scope_out"] = []string{secTestToken} },
		func(m map[string]any) { m["tasks"].([]map[string]any)[0]["title"] = secTestToken },
	}
	for _, mutate := range mutations {
		m := secBaseBriefMap()
		mutate(m)
		raw, _ := json.Marshal(m)
		_, err := Tools["propose_brief"].Call(t.Context(), h, c, raw)
		seamCode(t, err, "secret")
		secMustEq(t, err.Error(), "secret: this looks like a secret; remove it and send again")
	}
	secMustEq(t, len(home.requests), 0)
}

func TestProposeBriefRefusals(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	for _, raw := range []string{
		`{`,
		`{"title":"T"}`,
		`{"title":"T","goal":"G","criteria":["C"],"scope_in":[],"scope_out":[],"tasks":[{"title":"X","assignee":"Claude Code","reviewer":"Codex","depends":null}]}`,
	} {
		_, err := Tools["propose_brief"].Call(t.Context(), h, c, json.RawMessage(raw))
		seamCode(t, err, "bad_request")
	}

	// Unknown assignee, reviewer, and explicit requester list sorted valid member names.
	wantSub := `unknown member "Ghost" (valid: Alice, Claude Code, Codex, Secretary, owner)`
	for _, mutate := range []func(map[string]any){
		func(m map[string]any) { m["tasks"].([]map[string]any)[0]["assignee"] = "Ghost" },
		func(m map[string]any) { m["tasks"].([]map[string]any)[0]["reviewer"] = "Ghost" },
		func(m map[string]any) { m["requester"] = "Ghost" },
	} {
		m := secBaseBriefMap()
		mutate(m)
		raw, _ := json.Marshal(m)
		_, err := Tools["propose_brief"].Call(t.Context(), h, c, raw)
		seamCode(t, err, "not_found")
		secMustTrue(t, strings.Contains(err.Error(), wantSub), err.Error())
	}

	// Command failure, corrupt group row, and missing group row.
	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"err":{"code":"conflict","msg":"bad brief"}}`, id)), nil
	}
	cleanRaw, _ := json.Marshal(secBaseBriefMap())
	_, err := Tools["propose_brief"].Call(t.Context(), h, c, cleanRaw)
	seamCode(t, err, "conflict")

	h.Rows.Put(secGroupID, "group", secGroupID, json.RawMessage(`{"r":"group"}`))
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, cleanRaw)
	seamCode(t, err, "bad_request")

	h.Rows.Reset(secGroupID)
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, cleanRaw)
	seamCode(t, err, "not_found")
}

func TestSetTaskAssignsRoles(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	// 1. Assignee alone omits reviewer on wire.
	out, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-12","assignee":"Codex"}`))
	secMustEq(t, err, nil)
	secMustEq(t, out, secOKReply{OK: true})
	secMustEq(t, len(home.requests), 1)
	cmd1 := home.requests[0].(proto.Cmd)
	secMustTrue(t, !strings.Contains(cmd1.Env.Payload, `"reviewer"`), cmd1.Env.Payload)
	assign1 := secOpenCmd[proto.TaskAssign](t, h, cmd1)
	secMustEq(t, assign1.Op, "assign")
	secMustEq(t, string(assign1.ID), secTask12ID)
	secMustEq(t, string(assign1.Assignee), secCodexID)
	secMustEq(t, string(assign1.Reviewer), "")
	secMustEq(t, string(assign1.As), secSecID)

	// 2. Reviewer alone omits assignee on wire.
	home.requests = nil
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","reviewer":"Claude Code"}`))
	secMustEq(t, err, nil)
	cmd2 := home.requests[0].(proto.Cmd)
	secMustTrue(t, !strings.Contains(cmd2.Env.Payload, `"assignee"`), cmd2.Env.Payload)
	assign2 := secOpenCmd[proto.TaskAssign](t, h, cmd2)
	secMustEq(t, string(assign2.ID), secTask2ID)
	secMustEq(t, string(assign2.Reviewer), secClaudeID)
	secMustEq(t, string(assign2.Assignee), "")
}

func TestSetTaskTransitionsAndCombinedOrder(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	for _, action := range []string{"pause", "resume", "cancel"} {
		home.requests = nil
		raw := json.RawMessage(fmt.Sprintf(`{"task":"T-12","action":%q}`, action))
		_, err := Tools["set_task"].Call(t.Context(), h, c, raw)
		secMustEq(t, err, nil)
		secMustEq(t, len(home.requests), 1)
		tc := secOpenCmd[proto.TaskCommand](t, h, home.requests[0])
		secMustEq(t, tc.Op, action)
		secMustEq(t, string(tc.ID), secTask12ID)
		secMustEq(t, string(tc.As), secSecID)
	}

	// Both roles and action together: sends task assign first, then task action.
	home.requests = nil
	_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Claude Code","reviewer":"Codex","action":"resume"}`))
	secMustEq(t, err, nil)
	secMustEq(t, len(home.requests), 2)
	bothAssign := secOpenCmd[proto.TaskAssign](t, h, home.requests[0])
	bothResume := secOpenCmd[proto.TaskCommand](t, h, home.requests[1])
	secMustEq(t, bothAssign.Op, "assign")
	secMustEq(t, string(bothAssign.Assignee), secClaudeID)
	secMustEq(t, string(bothAssign.Reviewer), secCodexID)
	secMustEq(t, bothResume.Op, "resume")
	secMustEq(t, string(bothResume.ID), secTask2ID)
	secMustTrue(t, bothAssign.Nonce != bothResume.Nonce, "expected distinct nonces")
}

func TestSetTaskValidationRefusals(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	for _, bad := range []string{`{}`, `{"task":"T-12"}`, `{"task":"T-12","action":null}`} {
		_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	// Unknown task lists sorted valid T-<num> names.
	_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-99","action":"pause"}`))
	seamCode(t, err, "not_found")
	secMustTrue(t, strings.Contains(err.Error(), `unknown task "T-99" (valid: T-2, T-12)`), err.Error())

	// Unknown assignee or reviewer refuses before sending any command.
	for _, raw := range []string{
		`{"task":"T-12","assignee":"Nobody","action":"pause"}`,
		`{"task":"T-12","reviewer":"Nobody","action":"pause"}`,
	} {
		_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(raw))
		seamCode(t, err, "not_found")
	}
	secMustEq(t, len(home.requests), 0)

	// Empty group lists "valid: none" for unknown task and unknown member.
	h.Rows.Reset(secGroupID)
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-1","action":"pause"}`))
	secMustTrue(t, strings.Contains(err.Error(), "valid: none"), err.Error())

	secPutJSON(h, "task", secTask2ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: secTask2ID, Seq: 1},
		Group:     secGroupID, Num: 2, Title: "T", Slug: "t", Origin: "direct",
		State: "active", Step: "branch", Assignee: secCodexID, Pass: 85, Depends: []string{},
	})
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-9","action":"pause"}`))
	secMustTrue(t, strings.Contains(err.Error(), "valid: T-2)"), err.Error())

	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Nobody"}`))
	secMustTrue(t, strings.Contains(err.Error(), "valid: none"), err.Error())
}

func TestSetTaskCommandErrorsAndPartialSuccess(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	// 1. Failure on task assign itself does not claim assign succeeded and stops before action.
	home.onReq = func(int64, any) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"cannot assign"}}`), nil
	}
	_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Claude Code","action":"resume"}`))
	seamCode(t, err, "conflict")
	secMustEq(t, err.Error(), "conflict: cannot assign")
	secMustEq(t, len(home.requests), 1)

	// 2. Failure on action alone returns the home error unchanged.
	home.requests = nil
	home.onReq = func(int64, any) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"cannot pause"}}`), nil
	}
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","action":"pause"}`))
	seamCode(t, err, "conflict")
	secMustEq(t, err.Error(), "conflict: cannot pause")

	// 3. Failure on second command of combined assign+action notes that task assign succeeded first.
	home.requests = nil
	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		if id == 2 {
			return json.RawMessage(`{"re":2,"err":{"code":"conflict","msg":"cannot resume"}}`), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Claude Code","action":"resume"}`))
	seamCode(t, err, "conflict")
	secMustEq(t, err.Error(), "conflict: task assign succeeded before task resume failed: cannot resume")
	secMustEq(t, len(home.requests), 2)
}

func TestRequestDecisionInGroupSession(t *testing.T) {
	h, home := newSecFixture(t)
	groupCaller := secWorkerCaller(GroupSession, secClaudeID, "")

	// 1. Deduplicated recipients, options A/B, recommended 1 -> "B", context, zero Main/Thread/Text/Re.
	out, err := Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{
		"question": "Where should the banner appear?",
		"context": "Space is tight.",
		"options": [
			{"label": "Top bar", "effect": "Always visible."},
			{"label": "Status dot", "effect": "Subtle."}
		],
		"recommended": 1,
		"to": ["owner", "Alice", "owner"]
	}`))
	secMustEq(t, err, nil)
	reply := out.(secIDReply)
	ctx := "Space is tight."
	recB := "B"
	want := proto.Post[proto.DecisionBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(reply.ID), Timestamp: seamStart, Group: secGroupID, As: secClaudeID,
		},
		Kind:     "decision",
		Mentions: []proto.ID{secOwnerMID, secAliceMID},
		Body: proto.DecisionBody{
			Question: "Where should the banner appear?",
			Context:  &ctx,
			Options: []proto.DecisionOption{
				{ID: "A", Label: "Top bar", Effect: "Always visible."},
				{ID: "B", Label: "Status dot", Effect: "Subtle."},
			},
			Recommended: &recB,
			To:          []string{secOwnerUID, secAliceUID},
		},
	}
	secMustEq(t, secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0]), want)

	// 2. Empty options and empty context ("") omit context and recommended on wire; single explicit recipient.
	home.requests = nil
	_, err = Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{
		"question": "What label?",
		"context": "",
		"options": [],
		"to": ["Alice"]
	}`))
	secMustEq(t, err, nil)
	rawFree := home.requests[0].(proto.Cmd).Env.Payload
	secMustTrue(t, !strings.Contains(rawFree, `"context"`), rawFree)
	secMustTrue(t, !strings.Contains(rawFree, `"recommended"`), rawFree)
	secMustTrue(t, strings.Contains(rawFree, `"options":[]`), rawFree)
	singlePost := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0])
	secMustEq(t, singlePost.Body.To, []string{secAliceUID})
	secMustEq(t, singlePost.Mentions, []proto.ID{secAliceMID})

	// 3. Boundary recommended = 0 on single option -> "A".
	home.requests = nil
	_, err = Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{
		"question": "Confirm?",
		"options": [{"label": "Yes", "effect": "Proceed."}],
		"recommended": 0,
		"to": ["owner"]
	}`))
	secMustEq(t, err, nil)
	recA := "A"
	secMustEq(t, secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0]).Body.Recommended, &recA)
}

func TestRequestDecisionDefaultsToGroupPeopleInTaskAndReviewSessions(t *testing.T) {
	h, home := newSecFixture(t)
	tid := proto.ID(secTask12ID)
	task := secTask12ID

	for _, caller := range []Caller{
		secWorkerCaller(TaskSession, secClaudeID, secTask12ID),
		secWorkerCaller(ReviewSession, secCodexID, secTask12ID),
	} {
		home.requests = nil
		_, err := Tools["request_decision"].Call(t.Context(), h, caller, json.RawMessage(`{
			"question": "Retry timeout?",
			"options": [{"label": "5s", "effect": "Fast"}]
		}`))
		secMustEq(t, err, nil)
		secMustEq(t, len(home.requests), 1)
		got := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0])
		secMustEq(t, got.Main, false)
		secMustEq(t, got.Thread, &tid)
		secMustEq(t, got.Body.Task, &task)
		secMustEq(t, got.Body.To, []string{secAliceUID, secOwnerUID})
		secMustEq(t, got.Mentions, []proto.ID{secAliceMID, secOwnerMID})
	}
}

func TestRequestDecisionValidationAndSecretGuard(t *testing.T) {
	h, home := newSecFixture(t)
	groupCaller := secWorkerCaller(GroupSession, secClaudeID, "")
	taskCaller := secWorkerCaller(TaskSession, secClaudeID, secTask12ID)

	// Secret guard runs before any network call on question, context, option.label and option.effect.
	close(home.done)
	for _, raw := range []string{
		fmt.Sprintf(`{"question":%q,"options":[]}`, secTestToken),
		fmt.Sprintf(`{"question":"Q?","context":%q,"options":[]}`, secTestToken),
		fmt.Sprintf(`{"question":"Q?","options":[{"label":%q,"effect":"E"}]}`, secTestToken),
		fmt.Sprintf(`{"question":"Q?","options":[{"label":"L","effect":%q}]}`, secTestToken),
	} {
		_, err := Tools["request_decision"].Call(t.Context(), h, taskCaller, json.RawMessage(raw))
		seamCode(t, err, "secret")
	}
	secMustEq(t, len(home.requests), 0)

	home = newSecTestHome(nil)
	h.Attach("home-dev", home)

	// Bad arguments, recommended bounds (== len, 0 on empty), empty explicit to.
	for _, tc := range []struct {
		caller Caller
		raw    string
	}{
		{groupCaller, `{`},
		{groupCaller, `{"question":"Q?","options":null,"to":["owner"]}`},
		{groupCaller, `{"question":"Q?","options":[],"to":[]}`},
		{taskCaller, `{"question":"Q?","options":[],"to":[]}`},
		{groupCaller, `{"question":"Q?","options":[{"label":"A","effect":"E"},{"label":"B","effect":"E"}],"recommended":2,"to":["owner"]}`},
		{groupCaller, `{"question":"Q?","options":[],"recommended":0,"to":["owner"]}`},
	} {
		_, err := Tools["request_decision"].Call(t.Context(), h, tc.caller, json.RawMessage(tc.raw))
		seamCode(t, err, "bad_request")
	}

	// Non-human AI member or unknown name in to fails with not_found listing only people.
	for _, badName := range []string{"Codex", "Stranger"} {
		raw := json.RawMessage(fmt.Sprintf(`{"question":"Q?","options":[],"to":[%q]}`, badName))
		_, err := Tools["request_decision"].Call(t.Context(), h, groupCaller, raw)
		seamCode(t, err, "not_found")
		secMustTrue(t, strings.Contains(err.Error(), `valid: Alice, owner)`), err.Error())
	}

	// Command refusal propagated, and omitted to when group has no people fails with bad_request.
	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"err":{"code":"conflict","msg":"task closed"}}`, id)), nil
	}
	_, err := Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{"question":"Q?","options":[],"to":["owner"]}`))
	seamCode(t, err, "conflict")

	home.onReq = nil
	h.Rows.Reset(secGroupID)
	_, err = Tools["request_decision"].Call(t.Context(), h, taskCaller, json.RawMessage(`{"question":"Q?","options":[]}`))
	seamCode(t, err, "bad_request")
}
