package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
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
	if !ok {
		t.Fatalf("got %T, want proto.Cmd", req)
	}
	trusted := func(dev string) bool { return dev == h.Key.ID() }
	if _, err := identity.OpenCommand(cmd.Env, "", trusted, h.Now()); err != nil {
		t.Fatalf("envelope verification failed: %v", err)
	}
	var payload T
	if err := proto.Decode([]byte(cmd.Env.Payload), &payload); err != nil {
		t.Fatalf("decode payload %s: %v", cmd.Env.Payload, err)
	}
	return payload
}

func secMsgJSON(pos int64, thread *string, by, kind, text string, body any) json.RawMessage {
	rawBody, _ := json.Marshal(body)
	var state *proto.CardState
	if kind == "brief" || kind == "approval" || kind == "decision" {
		state = &proto.CardState{Status: "pending"}
	}
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: fmt.Sprintf("m%020dA", pos), Seq: pos},
		Pos:       pos,
		Group:     secGroupID,
		Thread:    thread,
		By:        by,
		Kind:      kind,
		Timestamp: seamStart,
		Text:      text,
		Mentions:  []string{},
		Body:      rawBody,
		State:     state,
	}
	out, _ := json.Marshal(msg)
	return out
}

func secFetchedJSON(id int64, rows ...json.RawMessage) json.RawMessage {
	if rows == nil {
		rows = []json.RawMessage{}
	}
	raw, _ := json.Marshal(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: id, OK: true},
		Rows:    rows,
	})
	return raw
}

func TestSecToolsMatchGoldenSchemasAndSessionVisibility(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lists proto.ToolLists
	if err := json.Unmarshal(data, &lists); err != nil {
		t.Fatal(err)
	}
	golden := func(list proto.ToolList, name string) any {
		idx := slices.IndexFunc(list.Tools, func(tl proto.Tool) bool { return tl.Name == name })
		if idx < 0 {
			t.Fatalf("missing %s in tools.json", name)
		}
		raw, _ := json.Marshal(list.Tools[idx].InputSchema)
		var v any
		json.Unmarshal(raw, &v)
		return v
	}
	for name, want := range map[string]any{
		"propose_brief":    golden(lists.Secretary, "propose_brief"),
		"set_task":         golden(lists.Secretary, "set_task"),
		"request_decision": golden(lists.Group, "request_decision"),
	} {
		tool, ok := Tools[name]
		if !ok || tool.Description == "" || tool.Call == nil {
			t.Fatalf("tool %s not registered completely: %+v", name, tool)
		}
		var got any
		if err := json.Unmarshal(tool.Schema, &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s schema mismatch:\ngot  %v\nwant %v", name, got, want)
		}
	}

	secListed := ToolsFor(secSecretaryCaller(""))
	if _, ok := secListed["propose_brief"]; !ok {
		t.Fatal("propose_brief not listed to secretary")
	}
	if _, ok := secListed["set_task"]; !ok {
		t.Fatal("set_task not listed to secretary")
	}
	if _, ok := secListed["request_decision"]; !ok {
		t.Fatal("request_decision not listed to secretary")
	}

	for _, c := range []Caller{
		secWorkerCaller(GroupSession, secClaudeID, ""),
		secWorkerCaller(TaskSession, secClaudeID, secTask12ID),
		secWorkerCaller(ReviewSession, secCodexID, secTask12ID),
	} {
		listed := ToolsFor(c)
		if _, ok := listed["propose_brief"]; ok {
			t.Fatalf("propose_brief listed to non-secretary %+v", c)
		}
		if _, ok := listed["set_task"]; ok {
			t.Fatalf("set_task listed to non-secretary %+v", c)
		}
		if _, ok := listed["request_decision"]; !ok {
			t.Fatalf("request_decision not listed to %+v", c)
		}
	}
}

func TestOptionIDBoundaries(t *testing.T) {
	cases := []struct {
		index int
		want  string
	}{
		{0, "A"},
		{1, "B"},
		{25, "Z"},
		{26, "AA"},
		{27, "AB"},
		{51, "AZ"},
		{52, "BA"},
		{702, "AAA"},
		{703, "AAB"},
	}
	for _, tc := range cases {
		if got := optionID(tc.index); got != tc.want {
			t.Fatalf("optionID(%d) = %q, want %q", tc.index, got, tc.want)
		}
	}
}

func TestProposeBriefBuildsSignedPostWithPassAndMentions(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	// Explicit requester, replaces, foundation true/default, depends [0]/default.
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
		"replaces": "KaiCW9JC8UOG7lKNduDo8Q"
	}`)
	out, err := Tools["propose_brief"].Call(t.Context(), h, c, rawArgs)
	if err != nil {
		t.Fatalf("propose_brief failed: %v", err)
	}
	reply := out.(secIDReply)
	if len(reply.ID) != 22 || len(home.requests) != 1 {
		t.Fatalf("got reply %+v and %d requests", reply, len(home.requests))
	}
	post := secOpenCmd[proto.Post[proto.BriefBody]](t, h, home.requests[0])
	if string(post.Nonce) != reply.ID || post.Timestamp != seamStart || string(post.Group) != secGroupID || string(post.As) != secSecID {
		t.Fatalf("unexpected header: %+v", post.HomeCommand)
	}
	if post.Kind != "brief" || post.Thread != nil || post.Main || post.Text != "" || post.Re != nil {
		t.Fatalf("unexpected post envelope fields: %+v", post)
	}
	if !slices.Equal(post.Mentions, []proto.ID{secAliceMID}) {
		t.Fatalf("mentions = %v, want [%s]", post.Mentions, secAliceMID)
	}
	if post.Body.Pass != 85 || post.Body.Requester != secAliceMID || post.Body.Replaces == nil || *post.Body.Replaces != secBriefID {
		t.Fatalf("unexpected brief body metadata: %+v", post.Body)
	}
	if !slices.Equal(post.Body.Scope.In, []string{"host", "web"}) || !slices.Equal(post.Body.Scope.Out, []string{"relay"}) {
		t.Fatalf("unexpected scope: %+v", post.Body.Scope)
	}
	wantTasks := []proto.BriefTask{
		{Title: "Host events", Assignee: secClaudeID, Reviewer: secCodexID, Foundation: true, Depends: []int{}},
		{Title: "Web badge", Assignee: secCodexID, Reviewer: secClaudeID, Foundation: false, Depends: []int{0}},
	}
	if !reflect.DeepEqual(post.Body.Tasks, wantTasks) {
		t.Fatalf("tasks = %+v, want %+v", post.Body.Tasks, wantTasks)
	}

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
	if err != nil {
		t.Fatalf("second propose_brief failed: %v", err)
	}
	cmd2 := home.requests[0].(proto.Cmd)
	if !strings.Contains(cmd2.Env.Payload, `"replaces":null`) || !strings.Contains(cmd2.Env.Payload, `"depends":[]`) {
		t.Fatalf("wire payload missing null replaces or [] depends: %s", cmd2.Env.Payload)
	}
	post2 := secOpenCmd[proto.Post[proto.BriefBody]](t, h, cmd2)
	if out2.(secIDReply).ID == reply.ID || post2.Body.Replaces != nil || post2.Body.Requester != secOwnerMID {
		t.Fatalf("unexpected second post: %+v", post2)
	}
}

func TestProposeBriefDefaultsRequesterFromRowsAndFetch(t *testing.T) {
	const minimalBrief = `{
		"title": "Brief",
		"goal": "Goal",
		"criteria": ["Done"],
		"scope_in": [],
		"scope_out": [],
		"tasks": [{"title": "Work", "assignee": "Claude Code", "reviewer": "Codex"}]
	}`

	// 1. Cached msg rows in h.Rows: picks newest Pos in matching thread, ignores other threads and corrupt JSON.
	h, home := newSecFixture(t)
	otherThread := secTask12ID
	h.Rows.Put(secGroupID, "msg", "bad-msg", json.RawMessage(`{"r":"msg"`))
	h.Rows.Put(secGroupID, "msg", "other-thread", secMsgJSON(50, &otherThread, secOwnerMID, "text", "In thread", proto.EmptyBody{}))
	h.Rows.Put(secGroupID, "msg", "a-older-main", secMsgJSON(10, nil, secOwnerMID, "text", "Older", proto.EmptyBody{}))
	h.Rows.Put(secGroupID, "msg", "z-newer-main", secMsgJSON(20, nil, secAliceMID, "text", "Newer", proto.EmptyBody{}))

	if _, err := Tools["propose_brief"].Call(t.Context(), h, secSecretaryCaller(""), json.RawMessage(minimalBrief)); err != nil {
		t.Fatal(err)
	}
	if len(home.requests) != 1 {
		t.Fatalf("got %d requests, want 1 (no fetch needed when cached)", len(home.requests))
	}
	post := secOpenCmd[proto.Post[proto.BriefBody]](t, h, home.requests[0])
	if post.Body.Requester != secAliceMID || !slices.Equal(post.Mentions, []proto.ID{secAliceMID}) {
		t.Fatalf("got requester %s mentions %v, want %s", post.Body.Requester, post.Mentions, secAliceMID)
	}

	// Same in a task thread turn: picks newest message in that thread, ignoring main-line messages.
	home.requests = nil
	h.Rows.Put(secGroupID, "msg", "z-thread-alice", secMsgJSON(60, &otherThread, secAliceMID, "text", "Thread newer", proto.EmptyBody{}))
	if _, err := Tools["propose_brief"].Call(t.Context(), h, secSecretaryCaller(secTask12ID), json.RawMessage(minimalBrief)); err != nil {
		t.Fatal(err)
	}
	postThread := secOpenCmd[proto.Post[proto.BriefBody]](t, h, home.requests[0])
	if postThread.Body.Requester != secAliceMID {
		t.Fatalf("got thread requester %s, want %s", postThread.Body.Requester, secAliceMID)
	}

	// 2. Fetch from home when h.Rows holds no msg rows: sorts by Pos desc and skips system, AI card, and secretary's own post.
	h2, home2 := newSecFixture(t)
	home2.onReq = func(id int64, req any) (json.RawMessage, error) {
		if ft, ok := req.(proto.FetchThread); ok {
			if ft.Thread != nil || ft.Before != 0 || ft.Limit != 50 || ft.Group != secGroupID {
				t.Fatalf("unexpected main FetchThread: %+v", ft)
			}
			return secFetchedJSON(id,
				secMsgJSON(10, nil, secAliceMID, "text", "Earlier", proto.EmptyBody{}),
				secMsgJSON(15, nil, secOwnerMID, "text", "Build it.", proto.EmptyBody{}),
				secMsgJSON(20, nil, secSecID, "question", "Which page?", proto.EmptyBody{}),
				secMsgJSON(25, nil, secClaudeID, "question", "Clarify?", proto.EmptyBody{}),
				secMsgJSON(30, nil, "system", "task", "", proto.TaskBody{Task: secTask12ID}),
			), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	if _, err := Tools["propose_brief"].Call(t.Context(), h2, secSecretaryCaller(""), json.RawMessage(minimalBrief)); err != nil {
		t.Fatal(err)
	}
	if len(home2.requests) != 2 {
		t.Fatalf("got %d requests, want fetch + cmd", len(home2.requests))
	}
	if got := secOpenCmd[proto.Post[proto.BriefBody]](t, h2, home2.requests[1]).Body.Requester; got != secOwnerMID {
		t.Fatalf("got %s, want %s", got, secOwnerMID)
	}

	// 3. Fetch in task thread turn, resolving requester from an earlier revised brief (skipping invalid/stale briefs).
	home2.requests = nil
	home2.onReq = func(id int64, req any) (json.RawMessage, error) {
		if ft, ok := req.(proto.FetchThread); ok {
			if ft.Thread == nil || *ft.Thread != secTask12ID {
				t.Fatalf("expected thread %s, got %+v", secTask12ID, ft.Thread)
			}
			staleBrief := proto.BriefBody{
				Title: "Old", Goal: "G", Criteria: []string{"C"}, Scope: proto.BriefScope{In: []string{}, Out: []string{}},
				Tasks: []proto.BriefTask{{Title: "T", Assignee: secClaudeID, Reviewer: secCodexID, Depends: []int{}}}, Pass: 85, Requester: "gone-member",
			}
			validBrief := staleBrief
			validBrief.Requester = secAliceMID
			badBrief := secMsgJSON(28, ft.Thread, secSecID, "brief", "", proto.EmptyBody{})
			return secFetchedJSON(id,
				badBrief,
				secMsgJSON(26, ft.Thread, secSecID, "brief", "", staleBrief),
				secMsgJSON(24, ft.Thread, secSecID, "brief", "", validBrief),
			), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	if _, err := Tools["propose_brief"].Call(t.Context(), h2, secSecretaryCaller(secTask12ID), json.RawMessage(minimalBrief)); err != nil {
		t.Fatal(err)
	}
	if got := secOpenCmd[proto.Post[proto.BriefBody]](t, h2, home2.requests[1]).Body.Requester; got != secAliceMID {
		t.Fatalf("got %s from brief requester, want %s", got, secAliceMID)
	}

	// 4. Fallback to non-secretary AI author when no human or brief exists (skipping system and self).
	home2.requests = nil
	home2.onReq = func(id int64, req any) (json.RawMessage, error) {
		if _, ok := req.(proto.FetchThread); ok {
			return secFetchedJSON(id,
				secMsgJSON(12, nil, "system", "task", "", proto.TaskBody{Task: secTask12ID}),
				secMsgJSON(11, nil, secSecID, "text", "Self message", proto.EmptyBody{}),
				secMsgJSON(10, nil, secCodexID, "text", "AI request", proto.EmptyBody{}),
			), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	if _, err := Tools["propose_brief"].Call(t.Context(), h2, secSecretaryCaller(""), json.RawMessage(minimalBrief)); err != nil {
		t.Fatal(err)
	}
	if got := secOpenCmd[proto.Post[proto.BriefBody]](t, h2, home2.requests[1]).Body.Requester; got != secCodexID {
		t.Fatalf("got %s, want fallback AI %s", got, secCodexID)
	}

	// 5. No candidate author -> bad_request.
	home2.requests = nil
	home2.onReq = func(id int64, req any) (json.RawMessage, error) {
		if _, ok := req.(proto.FetchThread); ok {
			return secFetchedJSON(id, secMsgJSON(11, nil, secSecID, "text", "Only self", proto.EmptyBody{})), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	_, err := Tools["propose_brief"].Call(t.Context(), h2, secSecretaryCaller(""), json.RawMessage(minimalBrief))
	seamCode(t, err, "bad_request")
}

func TestProposeBriefRefusalsAndSecretGuard(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	// Secret guard runs before home connection or state lookups on every text field.
	close(home.done)
	baseMap := func() map[string]any {
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
	secretMutations := []func(map[string]any){
		func(m map[string]any) { m["title"] = secTestToken },
		func(m map[string]any) { m["goal"] = secTestToken },
		func(m map[string]any) { m["criteria"] = []string{"ok", secTestToken} },
		func(m map[string]any) { m["scope_in"] = []string{secTestToken} },
		func(m map[string]any) { m["scope_out"] = []string{secTestToken} },
		func(m map[string]any) {
			m["tasks"].([]map[string]any)[0]["title"] = secTestToken
		},
		func(m map[string]any) { m["replaces"] = secTestToken },
	}
	for i, mutate := range secretMutations {
		m := baseMap()
		mutate(m)
		raw, _ := json.Marshal(m)
		_, err := Tools["propose_brief"].Call(t.Context(), h, c, raw)
		if err == nil || err.Error() != "secret: this looks like a secret; remove it and send again" {
			t.Fatalf("case %d: got %v, want secret error", i, err)
		}
	}
	if len(home.requests) != 0 {
		t.Fatalf("secret guard sent %d requests", len(home.requests))
	}

	// Reattach home for argument, member, group, fetch and command error paths.
	home = newSecTestHome(nil)
	h.Attach("home-dev", home)

	badInputs := []string{
		`{`,
		`{"title":"T"}`,
		`{"title":"T","goal":"G","criteria":["C"],"scope_in":[],"scope_out":[],"tasks":[],"extra":1}`,
		`{"title":"T","goal":"G","criteria":["C"],"scope_in":[],"scope_out":[],"tasks":[{"title":"X","assignee":"Claude Code","reviewer":"Codex","extra":1}]}`,
		`{"title":"T","goal":"G","criteria":["C"],"scope_in":[],"scope_out":[],"tasks":[{"title":"X","assignee":"Claude Code","reviewer":"Codex","depends":null}]}`,
		`{"title":"T","goal":"G","criteria":["C"],"scope_in":[],"scope_out":[],"tasks":[{"title":"X","assignee":"Claude Code","reviewer":"Codex","depends":[-1]}],"requester":"owner"}`,
	}
	for _, raw := range badInputs {
		_, err := Tools["propose_brief"].Call(t.Context(), h, c, json.RawMessage(raw))
		seamCode(t, err, "bad_request")
	}

	// Unknown assignee, reviewer, and explicit requester list sorted valid member names.
	for _, field := range []string{"assignee", "reviewer", "requester"} {
		m := baseMap()
		if field == "requester" {
			m["requester"] = "Ghost"
		} else {
			m["tasks"].([]map[string]any)[0][field] = "Ghost"
		}
		raw, _ := json.Marshal(m)
		_, err := Tools["propose_brief"].Call(t.Context(), h, c, raw)
		seamCode(t, err, "not_found")
		wantSub := `unknown member "Ghost" (valid: Alice, Claude Code, Codex, Secretary, owner)`
		if !strings.Contains(err.Error(), wantSub) {
			t.Fatalf("%s error = %q, want %q", field, err.Error(), wantSub)
		}
	}

	// Fetch errors when requester is omitted: wire error, malformed fetched, malformed msg row, request failure, disconnected.
	noReqMap := baseMap()
	delete(noReqMap, "requester")
	noReqRaw, _ := json.Marshal(noReqMap)

	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"err":{"code":"forbidden","msg":"not allowed"}}`, id)), nil
	}
	_, err := Tools["propose_brief"].Call(t.Context(), h, c, noReqRaw)
	seamCode(t, err, "forbidden")

	home.onReq = func(int64, any) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, noReqRaw)
	seamCode(t, err, "bad_request")

	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		return secFetchedJSON(id, json.RawMessage(`{"r":"msg"}`)), nil
	}
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, noReqRaw)
	seamCode(t, err, "bad_request")

	home.onReq = func(int64, any) (json.RawMessage, error) { return nil, io.EOF }
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, noReqRaw)
	seamCode(t, err, "home_unreachable")

	close(home.done)
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, noReqRaw)
	seamCode(t, err, "home_unreachable")

	home = newSecTestHome(nil)
	h.Attach("home-dev", home)

	// Command failure propagation.
	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"err":{"code":"conflict","msg":"bad brief"}}`, id)), nil
	}
	cleanRaw, _ := json.Marshal(baseMap())
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, cleanRaw)
	seamCode(t, err, "conflict")

	// Corrupt group row and missing group row.
	h.Rows.Put(secGroupID, "group", secGroupID, json.RawMessage(`{"r":"group"}`))
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, cleanRaw)
	seamCode(t, err, "bad_request")

	h.Rows.Reset(secGroupID)
	_, err = Tools["propose_brief"].Call(t.Context(), h, c, cleanRaw)
	seamCode(t, err, "not_found")
}

func TestSetTaskAssignsAndTransitions(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	// 1. Assignee alone.
	out, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-12","assignee":"Codex"}`))
	if err != nil || out != (secOKReply{OK: true}) || len(home.requests) != 1 {
		t.Fatalf("got %v, %v, %d requests", out, err, len(home.requests))
	}
	cmd1 := home.requests[0].(proto.Cmd)
	if strings.Contains(cmd1.Env.Payload, `"reviewer"`) {
		t.Fatalf("unexpected reviewer in payload: %s", cmd1.Env.Payload)
	}
	assign1 := secOpenCmd[proto.TaskAssign](t, h, cmd1)
	if assign1.Op != "assign" || string(assign1.ID) != secTask12ID || string(assign1.Assignee) != secCodexID || assign1.Reviewer != "" || string(assign1.As) != secSecID {
		t.Fatalf("unexpected assign1: %+v", assign1)
	}

	// 2. Reviewer alone.
	home.requests = nil
	if _, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","reviewer":"Claude Code"}`)); err != nil {
		t.Fatal(err)
	}
	cmd2 := home.requests[0].(proto.Cmd)
	if strings.Contains(cmd2.Env.Payload, `"assignee"`) {
		t.Fatalf("unexpected assignee in payload: %s", cmd2.Env.Payload)
	}
	assign2 := secOpenCmd[proto.TaskAssign](t, h, cmd2)
	if string(assign2.ID) != secTask2ID || string(assign2.Reviewer) != secClaudeID || assign2.Assignee != "" {
		t.Fatalf("unexpected assign2: %+v", assign2)
	}

	// 3. Action alone: pause, resume, cancel.
	for _, action := range []string{"pause", "resume", "cancel"} {
		home.requests = nil
		raw := json.RawMessage(fmt.Sprintf(`{"task":"T-12","action":%q}`, action))
		if _, err := Tools["set_task"].Call(t.Context(), h, c, raw); err != nil {
			t.Fatalf("%s failed: %v", action, err)
		}
		if len(home.requests) != 1 {
			t.Fatalf("%s sent %d requests", action, len(home.requests))
		}
		tc := secOpenCmd[proto.TaskCommand](t, h, home.requests[0])
		if tc.Op != action || string(tc.ID) != secTask12ID || string(tc.As) != secSecID {
			t.Fatalf("unexpected task command for %s: %+v", action, tc)
		}
	}

	// 4. Both roles and action together: sends task assign first, then task action.
	home.requests = nil
	if _, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Claude Code","reviewer":"Codex","action":"resume"}`)); err != nil {
		t.Fatal(err)
	}
	if len(home.requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(home.requests))
	}
	bothAssign := secOpenCmd[proto.TaskAssign](t, h, home.requests[0])
	bothResume := secOpenCmd[proto.TaskCommand](t, h, home.requests[1])
	if bothAssign.Op != "assign" || string(bothAssign.Assignee) != secClaudeID || string(bothAssign.Reviewer) != secCodexID {
		t.Fatalf("unexpected first command: %+v", bothAssign)
	}
	if bothResume.Op != "resume" || string(bothResume.ID) != secTask2ID || bothAssign.Nonce == bothResume.Nonce {
		t.Fatalf("unexpected second command: %+v", bothResume)
	}
}

func TestSetTaskRefusals(t *testing.T) {
	h, home := newSecFixture(t)
	c := secSecretaryCaller("")

	for _, bad := range []string{
		`{}`,
		`{"task":"T-12"}`,
		`{"task":"T-12","action":"archive"}`,
		`{"task":"T-12","action":null}`,
		`{"task":"T-12","action":"pause","extra":true}`,
	} {
		_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	// Unknown task lists sorted valid T-<num> names.
	_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-99","action":"pause"}`))
	seamCode(t, err, "not_found")
	if want := `unknown task "T-99" (valid: T-2, T-12)`; !strings.Contains(err.Error(), want) {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}

	// Unknown assignee or reviewer refuses before sending any command, even when action is set.
	for _, raw := range []string{
		`{"task":"T-12","assignee":"Nobody","action":"pause"}`,
		`{"task":"T-12","reviewer":"Nobody","action":"pause"}`,
	} {
		_, err := Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(raw))
		seamCode(t, err, "not_found")
	}
	if len(home.requests) != 0 {
		t.Fatalf("refused set_task sent %d requests", len(home.requests))
	}

	// Command failure on second command of a combined assign+action.
	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		if id == 2 {
			return json.RawMessage(`{"re":2,"err":{"code":"conflict","msg":"cannot resume"}}`), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Claude Code","action":"resume"}`))
	seamCode(t, err, "conflict")
	if len(home.requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(home.requests))
	}

	// Empty group lists "valid: none" for unknown task and unknown member.
	h.Rows.Reset(secGroupID)
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-1","action":"pause"}`))
	if err == nil || !strings.Contains(err.Error(), "valid: none") {
		t.Fatalf("got %v, want valid: none", err)
	}
	secPutJSON(h, "task", secTask2ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: secTask2ID, Seq: 1},
		Group:     secGroupID, Num: 2, Title: "T", Slug: "t", Origin: "direct",
		State: "active", Step: "branch", Assignee: secCodexID, Pass: 85, Depends: []string{},
	})
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-9","action":"pause"}`))
	if err == nil || !strings.Contains(err.Error(), "valid: T-2)") {
		t.Fatalf("got %v, want valid: T-2)", err)
	}
	_, err = Tools["set_task"].Call(t.Context(), h, c, json.RawMessage(`{"task":"T-2","assignee":"Nobody"}`))
	if err == nil || !strings.Contains(err.Error(), "valid: none") {
		t.Fatalf("got %v, want valid: none for members", err)
	}
}

func TestRequestDecisionInGroupAndTaskSessions(t *testing.T) {
	h, home := newSecFixture(t)

	// 1. GroupSession with explicit recipients (deduplicated), options, recommended index 1 -> "B", context.
	groupCaller := secWorkerCaller(GroupSession, secClaudeID, "")
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
	if err != nil {
		t.Fatalf("group request_decision failed: %v", err)
	}
	reply := out.(secIDReply)
	post := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0])
	if string(post.Nonce) != reply.ID || post.Kind != "decision" || post.Thread != nil || !post.Main || post.Text != "" || post.Re != nil {
		t.Fatalf("unexpected group decision post: %+v", post)
	}
	if !slices.Equal(post.Mentions, []proto.ID{secOwnerMID, secAliceMID}) {
		t.Fatalf("mentions = %v", post.Mentions)
	}
	if post.Body.Task != nil || post.Body.Context == nil || *post.Body.Context != "Space is tight." {
		t.Fatalf("unexpected body fields: %+v", post.Body)
	}
	if post.Body.Recommended == nil || *post.Body.Recommended != "B" || !slices.Equal(post.Body.To, []string{secOwnerUID, secAliceUID}) {
		t.Fatalf("unexpected recommended/to: %+v", post.Body)
	}
	wantOpts := []proto.DecisionOption{
		{ID: "A", Label: "Top bar", Effect: "Always visible."},
		{ID: "B", Label: "Status dot", Effect: "Subtle."},
	}
	if !reflect.DeepEqual(post.Body.Options, wantOpts) {
		t.Fatalf("options = %+v, want %+v", post.Body.Options, wantOpts)
	}

	// 2. Empty options and empty context ("") omit context and recommended on wire, recommended 0 -> "A" when 1 option.
	home.requests = nil
	if _, err := Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{
		"question": "What label?",
		"context": "",
		"options": [],
		"to": ["Alice"]
	}`)); err != nil {
		t.Fatal(err)
	}
	rawFree := home.requests[0].(proto.Cmd).Env.Payload
	if strings.Contains(rawFree, `"context"`) || strings.Contains(rawFree, `"recommended"`) || !strings.Contains(rawFree, `"options":[]`) {
		t.Fatalf("unexpected wire encoding: %s", rawFree)
	}

	// Boundary recommended = 0 on single option -> "A".
	home.requests = nil
	if _, err := Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{
		"question": "Confirm?",
		"options": [{"label": "Yes", "effect": "Proceed."}],
		"recommended": 0,
		"to": ["owner"]
	}`)); err != nil {
		t.Fatal(err)
	}
	if rec := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0]).Body.Recommended; rec == nil || *rec != "A" {
		t.Fatalf("recommended = %v, want A", rec)
	}

	// 3. TaskSession with omitted to on brief-origin task fetches brief via FetchRow.
	home.requests = nil
	home.onReq = func(id int64, req any) (json.RawMessage, error) {
		if fr, ok := req.(proto.FetchRow); ok {
			if fr.Row != "msg" || fr.RowID != secBriefID || fr.Before != 0 || fr.Limit != 1 || fr.Group != secGroupID {
				t.Fatalf("unexpected FetchRow: %+v", fr)
			}
			brief := proto.BriefBody{
				Title: "B", Goal: "G", Criteria: []string{"C"}, Scope: proto.BriefScope{In: []string{}, Out: []string{}},
				Tasks: []proto.BriefTask{{Title: "T", Assignee: secClaudeID, Reviewer: secCodexID, Depends: []int{}}}, Pass: 85, Requester: secAliceMID,
			}
			return secFetchedJSON(id, secMsgJSON(1, nil, secSecID, "brief", "", brief)), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	taskCaller := secWorkerCaller(TaskSession, secClaudeID, secTask12ID)
	if _, err := Tools["request_decision"].Call(t.Context(), h, taskCaller, json.RawMessage(`{
		"question": "Retry timeout?",
		"options": [{"label": "5s", "effect": "Fast"}]
	}`)); err != nil {
		t.Fatal(err)
	}
	if len(home.requests) != 2 {
		t.Fatalf("got %d requests, want FetchRow + Cmd", len(home.requests))
	}
	taskPost := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[1])
	if taskPost.Thread == nil || string(*taskPost.Thread) != secTask12ID || taskPost.Body.Task == nil || *taskPost.Body.Task != secTask12ID {
		t.Fatalf("task thread missing: %+v", taskPost)
	}
	if !slices.Equal(taskPost.Body.To, []string{secAliceUID}) || !slices.Equal(taskPost.Mentions, []proto.ID{secAliceMID}) {
		t.Fatalf("got to=%v mentions=%v, want Alice", taskPost.Body.To, taskPost.Mentions)
	}

	// 4. ReviewSession with cached brief in h.Rows (AI requester resolves to its human owner) sends 0 fetch requests.
	home.requests = nil
	home.onReq = nil
	aiReqBrief := proto.BriefBody{
		Title: "B", Goal: "G", Criteria: []string{"C"}, Scope: proto.BriefScope{In: []string{}, Out: []string{}},
		Tasks: []proto.BriefTask{{Title: "T", Assignee: secClaudeID, Reviewer: secCodexID, Depends: []int{}}}, Pass: 85, Requester: secCodexID,
	}
	h.Rows.Put(secGroupID, "msg", secBriefID, secMsgJSON(2, nil, secSecID, "brief", "", aiReqBrief))
	reviewCaller := secWorkerCaller(ReviewSession, secCodexID, secTask12ID)
	if _, err := Tools["request_decision"].Call(t.Context(), h, reviewCaller, json.RawMessage(`{
		"question": "Accept fallback?",
		"options": []
	}`)); err != nil {
		t.Fatal(err)
	}
	if len(home.requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(home.requests))
	}
	revPost := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0])
	if !slices.Equal(revPost.Body.To, []string{secOwnerUID}) || !slices.Equal(revPost.Mentions, []proto.ID{secOwnerMID}) {
		t.Fatalf("got to=%v mentions=%v, want owner", revPost.Body.To, revPost.Mentions)
	}
}

func TestRequestDecisionFallbacksAndRefusals(t *testing.T) {
	h, home := newSecFixture(t)

	// 1. Non-brief task (origin: direct, even if Brief field were non-empty) falls back to AI member's owner.
	secPutJSON(h, "task", secTask2ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: secTask2ID, Seq: 20},
		Group:     secGroupID, Num: 2, Title: "Direct", Slug: "direct", Origin: "direct", Brief: secBriefID,
		State: "active", Step: "branch", Assignee: secClaudeID, Pass: 85, Depends: []string{},
	})
	directCaller := secWorkerCaller(TaskSession, secClaudeID, secTask2ID)
	if _, err := Tools["request_decision"].Call(t.Context(), h, directCaller, json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}
	if len(home.requests) != 1 {
		t.Fatalf("direct task fetched brief: %d requests", len(home.requests))
	}
	if got := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0]).Body.To; !slices.Equal(got, []string{secOwnerUID}) {
		t.Fatalf("got %v, want owner", got)
	}

	// 2. Corrupt task row JSON, missing task row, empty FetchRow result, or stale brief requester fall back cleanly.
	h.Rows.Put(secGroupID, "task", "bad-task", json.RawMessage(`{"r":"task"}`))
	if _, err := Tools["request_decision"].Call(t.Context(), h, secWorkerCaller(TaskSession, secClaudeID, "bad-task"), json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := Tools["request_decision"].Call(t.Context(), h, secWorkerCaller(TaskSession, secClaudeID, "absent-task"), json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}

	home.onReq = func(id int64, req any) (json.RawMessage, error) {
		if _, ok := req.(proto.FetchRow); ok {
			return secFetchedJSON(id), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	if _, err := Tools["request_decision"].Call(t.Context(), h, secWorkerCaller(TaskSession, secClaudeID, secTask12ID), json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}

	home.onReq = func(id int64, req any) (json.RawMessage, error) {
		if _, ok := req.(proto.FetchRow); ok {
			stale := proto.BriefBody{
				Title: "B", Goal: "G", Criteria: []string{"C"}, Scope: proto.BriefScope{In: []string{}, Out: []string{}},
				Tasks: []proto.BriefTask{{Title: "T", Assignee: secClaudeID, Reviewer: secCodexID, Depends: []int{}}}, Pass: 85, Requester: "gone-member",
			}
			return secFetchedJSON(id, secMsgJSON(3, nil, secSecID, "brief", "", stale)), nil
		}
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
	}
	if _, err := Tools["request_decision"].Call(t.Context(), h, secWorkerCaller(TaskSession, secClaudeID, secTask12ID), json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}

	// 3. Fallback chain when AI member row is absent: first human member in h.Rows, then store setting 'owner', then not_found.
	home.onReq = nil
	h.Rows.Reset(secGroupID)
	secPutJSON(h, "member", secAliceMID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: secAliceMID, Seq: 1}, Group: secGroupID, Kind: "human", User: secAliceUID, Name: "Alice", LastSeen: seamStart},
		Role:      "owner",
	})
	home.requests = nil
	if _, err := Tools["request_decision"].Call(t.Context(), h, directCaller, json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}
	if got := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0]).Body.To; !slices.Equal(got, []string{secAliceUID}) {
		t.Fatalf("got %v, want human fallback %s", got, secAliceUID)
	}

	h.Rows.Reset(secGroupID)
	writeOwner := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES('owner', ?)`, `"`+secOwnerUID+`"`)
		return err
	}
	if err := h.Store.Write(writeOwner); err != nil {
		t.Fatal(err)
	}
	home.requests = nil
	if _, err := Tools["request_decision"].Call(t.Context(), h, directCaller, json.RawMessage(`{"question":"Q?","options":[]}`)); err != nil {
		t.Fatal(err)
	}
	storePost := secOpenCmd[proto.Post[proto.DecisionBody]](t, h, home.requests[0])
	if !slices.Equal(storePost.Body.To, []string{secOwnerUID}) || len(storePost.Mentions) != 0 {
		t.Fatalf("got to=%v mentions=%v, want store owner with [] mentions", storePost.Body.To, storePost.Mentions)
	}

	clearOwner := func(tx *store.Tx) error {
		_, err := tx.Exec(`DELETE FROM setting WHERE key = 'owner'`)
		return err
	}
	if err := h.Store.Write(clearOwner); err != nil {
		t.Fatal(err)
	}
	_, err := Tools["request_decision"].Call(t.Context(), h, directCaller, json.RawMessage(`{"question":"Q?","options":[]}`))
	seamCode(t, err, "not_found")
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
	if len(home.requests) != 0 {
		t.Fatalf("secret guard sent %d requests", len(home.requests))
	}

	home = newSecTestHome(nil)
	h.Attach("home-dev", home)

	// Bad arguments, recommended bounds (-1, len, 0 on empty), missing/empty to in GroupSession.
	for _, tc := range []struct {
		caller Caller
		raw    string
	}{
		{groupCaller, `{`},
		{groupCaller, `{"question":"Q?","options":[],"to":["owner"],"extra":1}`},
		{groupCaller, `{"question":"Q?","options":[{"label":"L","effect":"E","extra":1}],"to":["owner"]}`},
		{groupCaller, `{"question":"Q?","options":null,"to":["owner"]}`},
		{groupCaller, `{"question":"Q?","options":[]}`},
		{groupCaller, `{"question":"Q?","options":[],"to":[]}`},
		{taskCaller, `{"question":"Q?","options":[],"to":[]}`},
		{groupCaller, `{"question":"Q?","options":[{"label":"A","effect":"E"}],"recommended":-1,"to":["owner"]}`},
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
		if want := `valid: Alice, owner)`; !strings.Contains(err.Error(), want) {
			t.Fatalf("for %q got %q, want %q", badName, err.Error(), want)
		}
	}

	// FetchRow errors and malformed brief rows when resolving default recipient.
	home.onReq = func(int64, any) (json.RawMessage, error) { return nil, io.EOF }
	_, err := Tools["request_decision"].Call(t.Context(), h, taskCaller, json.RawMessage(`{"question":"Q?","options":[]}`))
	seamCode(t, err, "home_unreachable")

	for _, badBriefRow := range []json.RawMessage{
		json.RawMessage(`{"r":"msg"}`),
		secMsgJSON(1, nil, secOwnerMID, "text", "Not a brief", proto.BriefBody{
			Title: "B", Goal: "G", Criteria: []string{"C"}, Scope: proto.BriefScope{In: []string{}, Out: []string{}},
			Tasks: []proto.BriefTask{{Title: "T", Assignee: secClaudeID, Reviewer: secCodexID, Depends: []int{}}}, Pass: 85, Requester: secAliceMID,
		}),
		json.RawMessage(`{"r":"msg","id":"KaiCW9JC8UOG7lKNduDo8Q","seq":1,"pos":1,"g":"rZNvy-1jH6Z-BcPqA5U5BQ","thread":null,"main":false,"by":"qBSFMsr2hHYKOMblEA_kdA","kind":"brief","ts":1,"text":"","mentions":[],"re":null,"body":{},"state":{"status":"pending"}}`),
	} {
		rowCopy := badBriefRow
		home.onReq = func(id int64, _ any) (json.RawMessage, error) {
			return secFetchedJSON(id, rowCopy), nil
		}
		_, err := Tools["request_decision"].Call(t.Context(), h, taskCaller, json.RawMessage(`{"question":"Q?","options":[]}`))
		seamCode(t, err, "bad_request")
	}

	// Command refusal propagated.
	home.onReq = func(id int64, _ any) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"re":%d,"err":{"code":"conflict","msg":"task closed"}}`, id)), nil
	}
	_, err = Tools["request_decision"].Call(t.Context(), h, groupCaller, json.RawMessage(`{"question":"Q?","options":[],"to":["owner"]}`))
	seamCode(t, err, "conflict")
}
