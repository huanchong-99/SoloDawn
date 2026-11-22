package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	talkGroupID = "rZNvy-1jH6Z-BcPqA5U5BQ"
	talkTask1ID = "Aa0000000000000000000A"
	talkTask2ID = "DrtCn6htSBwmMPrFPbHJHA"
	talkSecID   = "qBSFMsr2hHYKOMblEA_kdA"
	talkAI1ID   = "1PC8WineBrUQ-apCjx7tug"
	talkAI2ID   = "LXCZmuGAXkvO-bSrOkuCfw"
	talkOwnerID = "4xq2Q8RPeg7IJLWdEZTWDQ"
)

type talkHomeStub struct {
	done    chan struct{}
	asked   []any
	replies []json.RawMessage
	err     error
}

func newTalkHomeStub(replies ...string) *talkHomeStub {
	raw := make([]json.RawMessage, len(replies))
	for i, r := range replies {
		raw[i] = json.RawMessage(r)
	}
	return &talkHomeStub{done: make(chan struct{}), replies: raw}
}

func (f *talkHomeStub) Send(any) error               { return nil }
func (f *talkHomeStub) Path() string                 { return "local" }
func (f *talkHomeStub) PathChanged() <-chan struct{} { return nil }
func (f *talkHomeStub) Done() <-chan struct{}        { return f.done }

func (f *talkHomeStub) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	idx := len(f.asked)
	reqID := int64(50 + idx)
	f.asked = append(f.asked, build(reqID))
	if f.err != nil {
		return nil, f.err
	}
	if idx < len(f.replies) {
		return f.replies[idx], nil
	}
	return json.RawMessage(`{"re":50,"ok":true}`), nil
}

func talkPutRow(t *testing.T, h *Host, r, id string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	h.Rows.Put(talkGroupID, r, id, data)
}

func seedTalkGroup(t *testing.T, h *Host) {
	t.Helper()
	winPct := 42.5
	talkPutRow(t, h, "member", talkOwnerID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: talkOwnerID, Seq: 1},
			Group:     talkGroupID, Kind: "human", User: "u1", Name: "owner", LastSeen: seamStart,
		},
		Role:   "owner",
		Online: true,
	})
	talkPutRow(t, h, "member", talkSecID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: talkSecID, Seq: 2},
			Group:     talkGroupID, Kind: "ai", User: "u1", Name: "Secretary", LastSeen: seamStart,
		},
		CLI: "claude", Host: seamKey().ID(), HostName: "workstation", Secretary: true,
		Account: "personal", Allow: []string{}, Approvers: []string{"u1"}, State: "waiting", Path: "local",
	})
	talkPutRow(t, h, "member", talkAI1ID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: talkAI1ID, Seq: 3},
			Group:     talkGroupID, Kind: "ai", User: "u1", Name: "Claude Code", LastSeen: seamStart,
		},
		CLI: "claude", Host: seamKey().ID(), HostName: "workstation", Secretary: false,
		Account: "personal", Allow: []string{}, Approvers: []string{"u1"}, State: "working", Task: talkTask2ID, Path: "local",
		Usage: proto.Usage{WindowPct: &winPct},
	})
	talkPutRow(t, h, "member", talkAI2ID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: talkAI2ID, Seq: 4},
			Group:     talkGroupID, Kind: "ai", User: "u1", Name: "Codex", LastSeen: seamStart,
		},
		CLI: "codex", Host: seamKey().ID(), HostName: "", Secretary: false,
		Account: "team", Allow: []string{}, Approvers: []string{"u1"}, State: "dormant", Path: "direct",
	})
	talkPutRow(t, h, "task", talkTask2ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: talkTask2ID, Seq: 5},
		Group:     talkGroupID, Num: 12, Title: "Show path", Slug: "show-path", Origin: "brief", Brief: "KaiCW9JC8UOG7lKNduDo8Q",
		State: "active", Step: "review", Assignee: talkAI1ID, Reviewer: talkAI2ID, Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: seamStart - 120_000}, Created: seamStart - 180_000,
	})
	talkPutRow(t, h, "task", talkTask1ID, proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: talkTask1ID, Seq: 6},
		Group:     talkGroupID, Num: 1, Title: "First task", Slug: "first-task", Origin: "direct",
		State: "queued", Step: "branch", Assignee: talkAI2ID, Pass: 90, Depends: []string{},
		Wait:  &proto.TaskWait{Kind: "depends", Since: seamStart - 60_000},
		Stats: proto.TaskStats{Started: seamStart - 60_000}, Created: seamStart - 60_000,
	})
}

func talkCaller(kind Kind, member, task string, secretary bool, turn Turn) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: talkGroupID, Member: member, Task: task},
			Kind:       kind,
			Folder:     "repo",
			Agent:      "claude",
			Home:       "home-device",
			Name:       "AI",
			Secretary:  secretary,
		},
		Level: 2,
		Turn:  turn,
	}
}

func assertErrorCode(t *testing.T, err error, code string) *Error {
	t.Helper()
	got, ok := errors.AsType[*Error](err)
	if !ok || got.Code != code {
		t.Fatalf("got %v, want *Error with code %q", err, code)
	}
	return got
}

func assertEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func assertNoNulls(t *testing.T, raw []byte, allow ...string) {
	t.Helper()
	if nulls := proto.UnexpectedNulls(raw, allow...); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}
}

func mustCallTalk[T any](t *testing.T, h *Host, c Caller, name, raw string) T {
	t.Helper()
	out, err := Tools[name].Call(t.Context(), h, c, json.RawMessage(raw))
	if err != nil {
		t.Fatalf("%s(%s): %v", name, raw, err)
	}
	return out.(T)
}

func mustDecodeCmd[T any](t *testing.T, asked any, allowNulls ...string) T {
	t.Helper()
	cmd := asked.(proto.Cmd)
	assertNoNulls(t, []byte(cmd.Env.Payload), allowNulls...)
	var dst T
	if err := proto.Decode([]byte(cmd.Env.Payload), &dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func decodeJSONValue(t *testing.T, raw []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func findGoldenTool(t *testing.T, list proto.ToolList, name string) proto.Tool {
	t.Helper()
	for _, entry := range list.Tools {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("tool %q not in golden list", name)
	return proto.Tool{}
}

func checkRegisteredTalkTool(t *testing.T, sec proto.ToolList, name string) {
	t.Helper()
	reg, ok := Tools[name]
	assertEqual(t, ok && reg.Description != "" && reg.Call != nil && reg.For == nil, true)
	golden := findGoldenTool(t, sec, name)
	goldenRaw, _ := json.Marshal(golden.InputSchema)
	assertEqual(t, decodeJSONValue(t, reg.Schema), decodeJSONValue(t, goldenRaw))
	sdec := json.NewDecoder(bytes.NewReader(reg.Schema))
	sdec.DisallowUnknownFields()
	var parsed proto.ToolInput
	if err := sdec.Decode(&parsed); err != nil {
		t.Fatalf("%s schema decode: %v", name, err)
	}
	assertEqual(t, parsed, golden.InputSchema)
}

func checkInvariantSchemas(t *testing.T, lists proto.ToolLists) {
	t.Helper()
	allLists := []proto.ToolList{lists.Secretary, lists.Group, lists.Review, lists.TaskLevel1, lists.TaskLevel23}
	for _, list := range allLists {
		for _, name := range []string{"list_members", "list_tasks", "note_put", "note_list"} {
			goldenRaw, _ := json.Marshal(findGoldenTool(t, list, name).InputSchema)
			assertEqual(t, decodeJSONValue(t, Tools[name].Schema), decodeJSONValue(t, goldenRaw))
		}
	}
}

func TestTalkToolsSchemasAndVisibilityMatchToolsJSON(t *testing.T) {
	lists := loadGoldenToolLists(t)
	talkNames := []string{"post", "read_thread", "list_members", "list_tasks", "note_put", "note_list"}
	for _, name := range talkNames {
		checkRegisteredTalkTool(t, lists.Secretary, name)
	}
	checkInvariantSchemas(t, lists)

	callers := []Caller{
		talkCaller(GroupSession, talkSecID, "", true, Turn{}),
		talkCaller(GroupSession, talkAI1ID, "", false, Turn{}),
		talkCaller(ReviewSession, talkAI2ID, talkTask2ID, false, Turn{}),
		{Session: Session{SessionKey: SessionKey{Task: talkTask2ID}, Kind: TaskSession}, Level: 1},
		{Session: Session{SessionKey: SessionKey{Task: talkTask2ID}, Kind: TaskSession}, Level: 2},
	}
	for _, c := range callers {
		listed := ToolsFor(c)
		for _, name := range talkNames {
			if _, ok := listed[name]; !ok {
				t.Fatalf("tool %q missing for caller %+v", name, c)
			}
		}
	}
}

func TestPostToolMainAndMentions(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	grpCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn})
	idOut := mustCallTalk[idResult](t, h, grpCaller, "post", `{"text":"<ok> & done","mentions":["Codex","owner","Codex"]}`)
	if len(idOut.ID) != 22 || len(home.asked) != 1 {
		t.Fatalf("got %+v and %d requests", idOut, len(home.asked))
	}
	cmd := home.asked[0].(proto.Cmd)
	known := func(dev string) bool { return dev == seamKey().ID() }
	if _, err := identity.OpenCommand(cmd.Env, "", known, h.Now()); err != nil {
		t.Fatalf("signed command rejected: %v", err)
	}
	if strings.Contains(cmd.Env.Payload, `\u003c`) {
		t.Fatalf("payload escaped HTML: %s", cmd.Env.Payload)
	}
	sentText := mustDecodeCmd[proto.Post[proto.EmptyBody]](t, home.asked[0], "/thread", "/re")
	assertEqual(t, sentText, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(idOut.ID), Timestamp: seamStart, Group: talkGroupID, As: talkAI1ID},
		Kind:        "text",
		Text:        "<ok> & done",
		Mentions:    []proto.ID{talkAI2ID, talkOwnerID},
		Body:        proto.EmptyBody{},
	})
}

func TestPostToolThreadPlacesAndSummary(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	tid1 := proto.ID(talkTask1ID)
	tid2 := proto.ID(talkTask2ID)
	inThreadCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn, Thread: talkTask2ID})
	mustCallTalk[idResult](t, h, inThreadCaller, "post", `{"text":"In thread","kind":"question","mentions":[]}`)
	sentQ := mustDecodeCmd[proto.Post[proto.EmptyBody]](t, home.asked[0], "/re")
	assertEqual(t, sentQ.Kind, "question")
	assertEqual(t, sentQ.Thread, &tid2)
	assertEqual(t, sentQ.Main, false)
	assertEqual(t, sentQ.Mentions, []proto.ID{})

	mustCallTalk[idResult](t, h, inThreadCaller, "post", `{"text":"To main","where":"main"}`)
	sentMain := mustDecodeCmd[proto.Post[proto.EmptyBody]](t, home.asked[1], "/thread", "/re")
	assertEqual(t, sentMain.Thread, (*proto.ID)(nil))
	assertEqual(t, sentMain.Main, false)

	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: ItemsTurn, Thread: talkTask2ID})
	mustCallTalk[idResult](t, h, taskCaller, "post", `{"text":"Step 1","kind":"progress","where":"thread"}`)
	sentProg := mustDecodeCmd[proto.Post[proto.EmptyBody]](t, home.asked[2], "/re")
	assertEqual(t, sentProg.Kind, "progress")
	assertEqual(t, sentProg.Thread, &tid2)
	assertEqual(t, sentProg.Main, false)

	secCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: ItemsTurn})
	mustCallTalk[idResult](t, h, secCaller, "post", `{"text":"Status?","kind":"question","task":"T-1"}`)
	secAsk := mustDecodeCmd[proto.Post[proto.EmptyBody]](t, home.asked[3], "/re")
	assertEqual(t, secAsk.Thread, &tid1)
	assertEqual(t, secAsk.Main, false)

	mustCallTalk[idResult](t, h, secCaller, "post", `{"text":"","kind":"summary","task":"T-12"}`)
	secSum := mustDecodeCmd[proto.Post[proto.EmptyBody]](t, home.asked[4], "/re")
	assertEqual(t, secSum.Kind, "summary")
	assertEqual(t, secSum.Thread, &tid2)
	assertEqual(t, secSum.Main, true)
	assertEqual(t, secSum.Text, "")
}

func TestPostToolRefusalsAndSecretCheck(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	grpCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn})
	secCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: ItemsTurn})
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: ItemsTurn, Thread: talkTask2ID})

	secrets := []string{
		`{"text":"token sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"}`,
		`{"text":"-----BEGIN OPENSSH PRIVATE KEY-----\nabc"}`,
		"{\"text\":\"A=1\\nAPI_SECRET=xyz\\nB=2\"}",
	}
	for _, raw := range secrets {
		_, err := Tools["post"].Call(t.Context(), h, taskCaller, json.RawMessage(raw))
		got := assertErrorCode(t, err, "secret")
		assertEqual(t, got.Error(), "secret: this looks like a secret; remove it and send again")
	}
	assertEqual(t, len(home.asked), 0)

	badInputs := []struct {
		caller Caller
		raw    string
	}{
		{grpCaller, ``},
		{grpCaller, `[]`},
		{grpCaller, `null`},
		{grpCaller, `{}`},
		{grpCaller, `{"text":null}`},
		{grpCaller, `{"text":"hi","kind":null}`},
		{taskCaller, `{"text":"hi","kind":"report"}`},
		{taskCaller, `{"text":"hi","kind":"handback"}`},
		{secCaller, `{"text":"hi","kind":"summary"}`},
		{secCaller, `{"text":"hi","where":"main","task":"T-12"}`},
		{grpCaller, `{"text":"hi","where":"thread"}`},
	}
	for _, tc := range badInputs {
		_, err := Tools["post"].Call(t.Context(), h, tc.caller, json.RawMessage(tc.raw))
		assertErrorCode(t, err, "bad_request")
	}
	assertEqual(t, len(home.asked), 0)
}

func TestPostToolUnknownNamesAndHomeErrors(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	grpCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn})
	secCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: ItemsTurn})

	_, err := Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi","mentions":["Ghost"]}`))
	assertEqual(t, assertErrorCode(t, err, "not_found").Message, `unknown member "Ghost" (valid: Claude Code, Codex, Secretary, owner)`)

	_, err = Tools["post"].Call(t.Context(), h, secCaller, json.RawMessage(`{"text":"hi","kind":"summary","task":"T-99"}`))
	assertEqual(t, assertErrorCode(t, err, "not_found").Message, `unknown task "T-99" (valid: T-1, T-12)`)

	_, err = Tools["post"].Call(t.Context(), h, secCaller, json.RawMessage(`{"text":"hi","task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	h.Rows.Reset(talkGroupID)
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi","mentions":["Ghost"]}`))
	assertEqual(t, assertErrorCode(t, err, "not_found").Message, `unknown member "Ghost" (valid: none)`)

	talkPutRow(t, h, "member", talkOwnerID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: talkOwnerID, Seq: 1}, Group: talkGroupID, Kind: "human", User: "u1", Name: "owner", LastSeen: seamStart},
		Role:      "owner", Online: true,
	})
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi","mentions":["Ghost"]}`))
	assertEqual(t, assertErrorCode(t, err, "not_found").Message, `unknown member "Ghost" (valid: owner)`)
	assertEqual(t, len(home.asked), 0)

	home.replies = []json.RawMessage{json.RawMessage(`{"re":50,"err":{"code":"forbidden","msg":"denied"}}`)}
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi"}`))
	assertErrorCode(t, err, "forbidden")

	close(home.done)
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi"}`))
	assertErrorCode(t, err, "home_unreachable")
}

func TestReadThreadTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)

	threadStr := talkTask2ID
	t1Str := talkTask1ID
	r1, _ := json.Marshal(proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m1____________________A", Seq: 20},
		Pos:       20, Group: talkGroupID, Thread: &threadStr, By: talkAI2ID, Kind: "text", Timestamp: seamStart, Text: "Review ready", Mentions: []string{}, Body: proto.EmptyBody{},
	})
	r2, _ := json.Marshal(proto.MessageRow[proto.SystemCode, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m2____________________A", Seq: 19},
		Pos:       19, Group: talkGroupID, Thread: &threadStr, By: "system", Kind: "system", Timestamp: seamStart - 1000, Text: "", Mentions: []string{}, Body: proto.SystemCode{Code: "no_secretary"},
	})
	r3, _ := json.Marshal(proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m3____________________A", Seq: 18},
		Pos:       18, Group: talkGroupID, Thread: &threadStr, By: "goneMember000000000000A", Kind: "question", Timestamp: seamStart - 2000, Text: "Old?", Mentions: []string{}, Body: proto.EmptyBody{},
	})
	page1, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 50, OK: true}, Rows: []json.RawMessage{r1, r2, r3}, More: true})
	pageEmpty, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 51, OK: true}, Rows: []json.RawMessage{}, More: false})

	home := newTalkHomeStub(string(page1), string(pageEmpty), string(pageEmpty))
	h.Attach("home-device", home)

	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})
	got := mustCallTalk[readThreadResult](t, h, taskCaller, "read_thread", `{}`)
	assertEqual(t, got, readThreadResult{
		Messages: []threadMsgItem{
			{ID: "m1____________________A", Pos: 20, By: "Codex", Kind: "text", At: seamStart, Text: "Review ready"},
			{ID: "m2____________________A", Pos: 19, By: "system", Kind: "system", At: seamStart - 1000, Text: ""},
			{ID: "m3____________________A", Pos: 18, By: "goneMember000000000000A", Kind: "question", At: seamStart - 2000, Text: "Old?"},
		},
		More: true,
	})
	assertEqual(t, home.asked[0].(proto.FetchThread), proto.FetchThread{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 50}, Group: talkGroupID, Before: 0, Limit: 30},
		Thread:    &threadStr,
	})

	grpCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	outMain := mustCallTalk[readThreadResult](t, h, grpCaller, "read_thread", `{"task":"main","limit":1,"before":1}`)
	encodedEmpty, _ := json.Marshal(outMain)
	assertNoNulls(t, encodedEmpty)
	assertEqual(t, home.asked[1].(proto.FetchThread), proto.FetchThread{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 51}, Group: talkGroupID, Before: 1, Limit: 1},
		Thread:    nil,
	})

	mustCallTalk[readThreadResult](t, h, grpCaller, "read_thread", `{"task":"T-1","limit":100}`)
	assertEqual(t, home.asked[2].(proto.FetchThread), proto.FetchThread{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 52}, Group: talkGroupID, Before: 0, Limit: 100},
		Thread:    &t1Str,
	})

	for _, bad := range []string{`{"task":"main","limit":1.5}`, `[]`} {
		_, err := Tools["read_thread"].Call(t.Context(), h, grpCaller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}
	_, err := Tools["read_thread"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	home.replies = append(home.replies, json.RawMessage(`{"re":53,"err":{"code":"not_found","msg":"missing thread"}}`))
	_, err = Tools["read_thread"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "not_found")

	home.err = io.EOF
	_, err = Tools["read_thread"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "home_unreachable")

	close(home.done)
	_, err = Tools["read_thread"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "home_unreachable")
}

func TestListMembersTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	caller := talkCaller(GroupSession, talkSecID, "", true, Turn{})

	emptyOut := mustCallTalk[listMembersResult](t, h, caller, "list_members", `{}`)
	rawEmpty, _ := json.Marshal(emptyOut)
	assertEqual(t, string(rawEmpty), `{"members":[]}`)

	seedTalkGroup(t, h)
	talkPutRow(t, h, "member", "hum2__________________A", proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "hum2__________________A", Seq: 7}, Group: talkGroupID, Kind: "human", User: "u2", Name: "alice", LastSeen: seamStart},
		Role:      "admin", Online: false,
	})
	talkPutRow(t, h, "member", "ai3___________________A", proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "ai3___________________A", Seq: 8}, Group: talkGroupID, Kind: "ai", User: "u1", Name: "Worker", LastSeen: seamStart},
		CLI:       "codex", Host: "dev-raw", HostName: "", Account: "api_key", Allow: []string{}, Approvers: []string{"u1"}, State: "working", Task: "agedOutTask00000000000A", Path: "relay",
	})
	talkPutRow(t, h, "member", "gone__________________A", proto.GoneRow{
		RowHeader: proto.RowHeader{Type: "member", ID: "gone__________________A", Seq: 9}, Gone: true,
	})
	h.Rows.Put(talkGroupID, "member", "bad_ai", json.RawMessage(`{"r":"member","id":"bad_ai","seq":10,"g":"g","kind":"ai","user":"u","name":"BadAI","last_seen":1}`))
	h.Rows.Put(talkGroupID, "member", "bad_hum", json.RawMessage(`{"r":"member","id":"bad_hum","seq":11,"g":"g","kind":"human","user":"u","name":"BadHum","last_seen":1}`))

	out := mustCallTalk[listMembersResult](t, h, caller, "list_members", `{}`)
	winPct := 42.5
	emptyUsage := proto.Usage{}
	withPct := proto.Usage{WindowPct: &winPct}
	assertEqual(t, out.Members, []memberListItem{
		{Name: "Claude Code", Kind: "ai", State: "working", Host: "workstation", Account: "personal", Task: "T-12", Usage: &withPct},
		{Name: "Codex", Kind: "ai", State: "dormant", Host: seamKey().ID(), Account: "team", Usage: &emptyUsage},
		{Name: "Secretary", Kind: "ai", State: "waiting", Host: "workstation", Account: "personal", Usage: &emptyUsage},
		{Name: "Worker", Kind: "ai", State: "working", Host: "dev-raw", Account: "api_key", Task: "agedOutTask00000000000A", Usage: &emptyUsage},
		{Name: "alice", Kind: "human", State: "offline"},
		{Name: "owner", Kind: "human", State: "online"},
	})
	encoded, _ := json.Marshal(out)
	assertNoNulls(t, encoded)
	if strings.Contains(string(encoded), `"name":"alice","kind":"human","state":"offline","host"`) || strings.Contains(string(encoded), `"name":"Codex","kind":"ai","state":"dormant","host":"`+seamKey().ID()+`","account":"team","task"`) {
		t.Fatalf("unexpected omitted fields in %s", encoded)
	}

	_, err := Tools["list_members"].Call(t.Context(), h, caller, json.RawMessage(`[]`))
	assertErrorCode(t, err, "bad_request")
}

func TestListTasksToolAndIdleMinutesBoundaries(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	fixedNow := int64(seamStart + 180_000)
	h.Now = func() time.Time { return time.UnixMilli(fixedNow) }

	endedAt := fixedNow - 600_000
	talkPutRow(t, h, "task", "t3____________________A", proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: "t3____________________A", Seq: 10},
		Group:     talkGroupID, Num: 3, Title: "Thread timed", Slug: "thread-timed", Origin: "direct",
		State: "paused", Step: "local_check", Assignee: "unknownAssignee0000000A", Reviewer: "unknownReviewer0000000A", Pass: 90, Depends: []string{},
		Wait:   &proto.TaskWait{Kind: "manual", Since: fixedNow - 60_000},
		Thread: proto.ThreadSummary{Count: 1, Last: &proto.ThreadMessage{By: talkAI1ID, Kind: "progress", Text: "wip", Timestamp: fixedNow - 60_000}},
		Stats:  proto.TaskStats{Started: fixedNow - 600_000}, Created: fixedNow - 600_000,
	})
	talkPutRow(t, h, "task", "t4____________________A", proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: "t4____________________A", Seq: 11},
		Group:     talkGroupID, Num: 4, Title: "Merged task", Slug: "merged-task", Origin: "direct",
		State: "merged", Step: "merge", Assignee: talkAI1ID, Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: fixedNow - 600_000, Ended: &endedAt}, Created: fixedNow - 600_000,
	})
	talkPutRow(t, h, "task", "goneTask______________A", proto.GoneRow{
		RowHeader: proto.RowHeader{Type: "task", ID: "goneTask______________A", Seq: 12}, Gone: true,
	})

	caller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	outAll := mustCallTalk[listTasksResult](t, h, caller, "list_tasks", `{}`)
	dependsKind := "depends"
	manualKind := "manual"
	assertEqual(t, outAll.Tasks, []taskListItem{
		{Task: "T-1", Title: "First task", State: "queued", Step: "branch", Assignee: "Codex", Wait: &dependsKind, IdleMinutes: 4},
		{Task: "T-3", Title: "Thread timed", State: "paused", Step: "local_check", Assignee: "unknownAssignee0000000A", Reviewer: "unknownReviewer0000000A", Wait: &manualKind, IdleMinutes: 1},
		{Task: "T-4", Title: "Merged task", State: "merged", Step: "merge", Assignee: "Claude Code", Wait: nil, IdleMinutes: 0},
		{Task: "T-12", Title: "Show path", State: "active", Step: "review", Assignee: "Claude Code", Reviewer: "Codex", Wait: nil, IdleMinutes: 5},
	})
	encoded, _ := json.Marshal(outAll)
	assertNoNulls(t, encoded, "/tasks/2/wait", "/tasks/3/wait")

	outPaused := mustCallTalk[listTasksResult](t, h, caller, "list_tasks", `{"state":"paused"}`)
	assertEqual(t, len(outPaused.Tasks), 1)
	assertEqual(t, outPaused.Tasks[0].Task, "T-3")
	outFailed := mustCallTalk[listTasksResult](t, h, caller, "list_tasks", `{"state":"failed"}`)
	assertEqual(t, len(outFailed.Tasks), 0)

	for _, bad := range []string{`{"state":null}`, `[]`} {
		_, err := Tools["list_tasks"].Call(t.Context(), h, caller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}

	ended := int64(100)
	for _, tc := range []struct {
		row  proto.TaskRow
		now  int64
		want int
	}{
		{proto.TaskRow{State: "active", Stats: proto.TaskStats{Started: 0, Ended: &ended}}, 300_000, 0},
		{proto.TaskRow{State: "merged", Stats: proto.TaskStats{Started: 0}}, 300_000, 0},
		{proto.TaskRow{State: "cancelled", Stats: proto.TaskStats{Started: 0}}, 300_000, 0},
		{proto.TaskRow{State: "active", Stats: proto.TaskStats{Started: 100_000}}, 159_999, 0},
		{proto.TaskRow{State: "active", Stats: proto.TaskStats{Started: 100_000}}, 160_000, 1},
		{proto.TaskRow{State: "active", Created: 100_000}, 220_000, 2},
		{proto.TaskRow{State: "active", Stats: proto.TaskStats{Started: 200_000}}, 100_000, 0},
	} {
		assertEqual(t, taskIdleMinutes(tc.row, tc.now), tc.want)
	}
}

func TestNotePutTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	grpCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})
	revCaller := talkCaller(ReviewSession, talkAI2ID, talkTask1ID, false, Turn{})

	_, err := Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"ghp_0123456789abcdefghijklmnopqrstuvwxyz"}`))
	assertErrorCode(t, err, "secret")
	assertEqual(t, len(home.asked), 0)

	res1 := mustCallTalk[idResult](t, h, grpCaller, "note_put", `{"text":"Group convention"}`)
	noteCmd1 := mustDecodeCmd[proto.Note](t, home.asked[0], "/task")
	assertEqual(t, string(noteCmd1.ID), res1.ID)
	assertEqual(t, noteCmd1.ID != noteCmd1.Nonce, true)
	assertEqual(t, noteCmd1.Task, (*proto.ID)(nil))
	assertEqual(t, string(noteCmd1.As), talkSecID)
	assertEqual(t, noteCmd1.Text, "Group convention")

	tid2 := proto.ID(talkTask2ID)
	mustCallTalk[idResult](t, h, grpCaller, "note_put", `{"text":"Task convention","task":"T-12"}`)
	noteCmd2 := mustDecodeCmd[proto.Note](t, home.asked[1])
	assertEqual(t, noteCmd2.Task, &tid2)

	for i, c := range []Caller{taskCaller, revCaller} {
		mustCallTalk[idResult](t, h, c, "note_put", `{"text":"Session note"}`)
		nc := mustDecodeCmd[proto.Note](t, home.asked[2+i])
		wantTask := proto.ID(c.Task)
		assertEqual(t, nc.Task, &wantTask)
	}

	for _, bad := range []string{`{}`, `{"text":null}`, `[]`} {
		_, err := Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}
	_, err = Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"x","task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	close(home.done)
	_, err = Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"offline"}`))
	assertErrorCode(t, err, "home_unreachable")
}

func TestNoteListTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)

	grpCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})

	t12 := talkTask2ID
	tAged := "agedTask00000000000000A"
	nRow1, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "n1____________________A", Seq: 30}, Pos: 30, Group: talkGroupID, Task: &t12, Text: "First", By: talkAI1ID, Timestamp: seamStart})
	nRow2, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "n2____________________A", Seq: 25}, Pos: 25, Group: talkGroupID, Task: nil, Text: "Global", By: "goneAuthor000000000000A", Timestamp: seamStart - 1000})
	nRow3, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "n3____________________A", Seq: 20}, Pos: 20, Group: talkGroupID, Task: &tAged, Text: "Old task", By: talkOwnerID, Timestamp: seamStart - 2000})

	p1, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 60, OK: true}, Rows: []json.RawMessage{nRow1, nRow2}, More: true})
	p2, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 61, OK: true}, Rows: []json.RawMessage{nRow3}, More: false})
	pEmptyMore, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 62, OK: true}, Rows: []json.RawMessage{}, More: true})
	prRow, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "pr1___________________A", Seq: 40}, Pos: 40, Group: talkGroupID, Task: nil, NoteType: "preset", Name: "Calm dashboard", Text: "One accent", By: talkOwnerID, Timestamp: seamStart})
	pPresets, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 63, OK: true}, Rows: []json.RawMessage{prRow}, More: false})

	listHome := newTalkHomeStub(string(p1), string(p2), string(p1), string(p2), string(pEmptyMore), string(pPresets))
	h.Attach("home-device", listHome)

	allOut := mustCallTalk[noteListResult](t, h, taskCaller, "note_list", `{}`)
	lbl12 := "T-12"
	wantNotes := []noteListItem{
		{ID: "n1____________________A", Task: &lbl12, Text: "First", By: "Claude Code", At: seamStart},
		{ID: "n2____________________A", Task: nil, Text: "Global", By: "goneAuthor000000000000A", At: seamStart - 1000},
		{ID: "n3____________________A", Task: &tAged, Text: "Old task", By: "owner", At: seamStart - 2000},
	}
	assertEqual(t, allOut.Notes, wantNotes)
	assertEqual(t, listHome.asked[0].(proto.FetchKind), proto.FetchKind{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 50}, Group: talkGroupID, Before: 0, Limit: 100},
		Kind:      "note",
	})
	assertEqual(t, listHome.asked[1].(proto.FetchKind).Before, int64(25))
	encodedAll, _ := json.Marshal(allOut)
	assertNoNulls(t, encodedAll, "/notes/1/task")
	if strings.Contains(string(encodedAll), `"name"`) {
		t.Fatalf("context notes unexpectedly included name: %s", encodedAll)
	}

	filteredOut := mustCallTalk[noteListResult](t, h, grpCaller, "note_list", `{"task":"T-12"}`)
	assertEqual(t, filteredOut.Notes, wantNotes[:1])

	emptyMoreOut := mustCallTalk[noteListResult](t, h, grpCaller, "note_list", `{}`)
	assertEqual(t, len(emptyMoreOut.Notes), 0)

	presetOut := mustCallTalk[noteListResult](t, h, grpCaller, "note_list", `{"type":"preset"}`)
	assertEqual(t, presetOut.Notes, []noteListItem{
		{ID: "pr1___________________A", Task: nil, Name: "Calm dashboard", Text: "One accent", By: "owner", At: seamStart},
	})
	assertEqual(t, listHome.asked[5].(proto.FetchKind), proto.FetchKind{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 55}, Group: talkGroupID, Before: 0, Limit: 100},
		Kind:      "preset",
	})
	encodedPresets, _ := json.Marshal(presetOut)
	assertNoNulls(t, encodedPresets, "/notes/0/task")
	if !strings.Contains(string(encodedPresets), `"name":"Calm dashboard"`) {
		t.Fatalf("preset output missing name: %s", encodedPresets)
	}

	for _, bad := range []string{`[]`, `{"type":"kb"}`, `{"type":""}`, `{"type":null}`} {
		_, err := Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}
	_, err := Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	close(listHome.done)
	_, err = Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "home_unreachable")
}

func TestPostToolHandbackCarriesTheFacts(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)
	tid2 := proto.ID(talkTask2ID)
	facts := proto.HandbackBody{Files: []string{"a.go"}, Commits: []proto.CommitSubject{{SHA: "9f2c1ab", Subject: "Fix"}}, Added: 4, Removed: 1}
	t2 := &takeover{facts: facts}
	takeoverState(h).claim(aiSlot{group: talkGroupID, member: talkAI1ID}, t2, true)

	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: HandbackTurn, Thread: talkTask2ID})
	mustCallTalk[idResult](t, h, taskCaller, "post", `{"text":"Owner asked for X.","kind":"handback"}`)
	sent := mustDecodeCmd[proto.Post[proto.HandbackBody]](t, home.asked[0], "/re")
	assertEqual(t, [4]any{sent.Kind, sent.Thread, sent.Main, sent.Text}, [4]any{"handback", &tid2, false, "Owner asked for X."})
	assertEqual(t, sent.Body, facts)
	assertEqual(t, t2.posted, true)

	secCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: HandbackTurn})
	mustCallTalk[idResult](t, h, secCaller, "post", `{"text":"Read the logs.","kind":"handback"}`)
	sentSec := mustDecodeCmd[proto.Post[proto.HandbackBody]](t, home.asked[1], "/thread", "/re")
	assertEqual(t, sentSec.Thread, (*proto.ID)(nil))
	assertEqual(t, sentSec.Body, proto.HandbackBody{Files: []string{}, Commits: []proto.CommitSubject{}})
}
