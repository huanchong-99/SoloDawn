package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
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

func TestTalkToolsSchemasAndVisibilityMatchToolsJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var lists proto.ToolLists
	if err := dec.Decode(&lists); err != nil {
		t.Fatal(err)
	}

	talkNames := []string{"post", "read_thread", "list_members", "list_tasks", "note_put", "note_list"}
	for _, name := range talkNames {
		reg, ok := Tools[name]
		if !ok || reg.Description == "" || reg.Call == nil || reg.For != nil {
			t.Fatalf("tool %q registration incomplete: %+v (ok=%v)", name, reg, ok)
		}
		golden := findGoldenTool(t, lists.Secretary, name)
		goldenRaw, err := json.Marshal(golden.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decodeJSONValue(t, reg.Schema), decodeJSONValue(t, goldenRaw)) {
			t.Fatalf("%s schema:\ngot  %s\nwant %s", name, reg.Schema, goldenRaw)
		}
		sdec := json.NewDecoder(bytes.NewReader(reg.Schema))
		sdec.DisallowUnknownFields()
		var parsed proto.ToolInput
		if err := sdec.Decode(&parsed); err != nil || !reflect.DeepEqual(parsed, golden.InputSchema) {
			t.Fatalf("%s schema decode: %v, got %+v, want %+v", name, err, parsed, golden.InputSchema)
		}
	}

	invariant := []string{"list_members", "list_tasks", "note_put", "note_list"}
	allLists := []proto.ToolList{lists.Secretary, lists.Group, lists.Review, lists.TaskLevel1, lists.TaskLevel23}
	for _, list := range allLists {
		for _, name := range invariant {
			golden := findGoldenTool(t, list, name)
			goldenRaw, _ := json.Marshal(golden.InputSchema)
			if !reflect.DeepEqual(decodeJSONValue(t, Tools[name].Schema), decodeJSONValue(t, goldenRaw)) {
				t.Fatalf("%s schema differs from session list", name)
			}
		}
	}

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

func TestPostToolKindsPlacesMentionsAndSigning(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	// Default kind ("text") in main line of a group session, with < and & unescaped.
	grpCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn})
	res, err := Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"<ok> & done","mentions":["Codex","owner"]}`))
	if err != nil {
		t.Fatal(err)
	}
	idOut := res.(idResult)
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
	if nulls := proto.UnexpectedNulls([]byte(cmd.Env.Payload), "/thread", "/re"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}
	var sentText proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(cmd.Env.Payload), &sentText); err != nil {
		t.Fatal(err)
	}
	wantMentions := []proto.ID{talkAI2ID, talkOwnerID}
	if string(sentText.Nonce) != idOut.ID || string(sentText.Group) != talkGroupID || string(sentText.As) != talkAI1ID ||
		sentText.Kind != "text" || sentText.Thread != nil || sentText.Main || sentText.Text != "<ok> & done" ||
		!slices.Equal(sentText.Mentions, wantMentions) || sentText.Re != nil {
		t.Fatalf("unexpected post payload: %+v", sentText)
	}

	// Group session turn whose last item was in a thread defaults to that thread; where="main" overrides to main.
	inThreadCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn, Thread: talkTask2ID})
	if _, err := Tools["post"].Call(t.Context(), h, inThreadCaller, json.RawMessage(`{"text":"In thread","kind":"question","mentions":[]}`)); err != nil {
		t.Fatal(err)
	}
	var sentQ proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(home.asked[1].(proto.Cmd).Env.Payload), &sentQ); err != nil {
		t.Fatal(err)
	}
	if sentQ.Kind != "question" || sentQ.Thread == nil || string(*sentQ.Thread) != talkTask2ID || sentQ.Main || len(sentQ.Mentions) != 0 {
		t.Fatalf("unexpected question post: %+v", sentQ)
	}
	if nulls := proto.UnexpectedNulls([]byte(home.asked[1].(proto.Cmd).Env.Payload), "/re"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}

	if _, err := Tools["post"].Call(t.Context(), h, inThreadCaller, json.RawMessage(`{"text":"To main","where":"main"}`)); err != nil {
		t.Fatal(err)
	}
	var sentMain proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(home.asked[2].(proto.Cmd).Env.Payload), &sentMain); err != nil || sentMain.Thread != nil || sentMain.Main {
		t.Fatalf("expected main-line post: %+v (%v)", sentMain, err)
	}

	// Task session with empty Turn.Thread falls back to Session.Task; explicit where="thread" works too.
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})
	if _, err := Tools["post"].Call(t.Context(), h, taskCaller, json.RawMessage(`{"text":"Step 1","kind":"progress","where":"thread"}`)); err != nil {
		t.Fatal(err)
	}
	var sentProg proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(home.asked[3].(proto.Cmd).Env.Payload), &sentProg); err != nil ||
		sentProg.Kind != "progress" || sentProg.Thread == nil || string(*sentProg.Thread) != talkTask2ID || sentProg.Main {
		t.Fatalf("unexpected progress post: %+v (%v)", sentProg, err)
	}

	// Secretary posts in a named task thread (non-summary) and posts a summary with main=true and empty text.
	secCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: ItemsTurn})
	if _, err := Tools["post"].Call(t.Context(), h, secCaller, json.RawMessage(`{"text":"Status?","kind":"question","task":"T-1"}`)); err != nil {
		t.Fatal(err)
	}
	var secAsk proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(home.asked[4].(proto.Cmd).Env.Payload), &secAsk); err != nil ||
		secAsk.Thread == nil || string(*secAsk.Thread) != talkTask1ID || secAsk.Main {
		t.Fatalf("unexpected secretary thread post: %+v (%v)", secAsk, err)
	}

	if _, err := Tools["post"].Call(t.Context(), h, secCaller, json.RawMessage(`{"text":"","kind":"summary","task":"T-12"}`)); err != nil {
		t.Fatal(err)
	}
	var secSum proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(home.asked[5].(proto.Cmd).Env.Payload), &secSum); err != nil ||
		secSum.Kind != "summary" || secSum.Thread == nil || string(*secSum.Thread) != talkTask2ID || !secSum.Main || secSum.Text != "" {
		t.Fatalf("unexpected summary post: %+v (%v)", secSum, err)
	}
}

func TestPostToolRefusalsAndSecretCheck(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	grpCaller := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: ItemsTurn})
	secCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: ItemsTurn})
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: ItemsTurn, Thread: talkTask2ID})

	// Outbound secret check runs before signing/sending, even when home is connected.
	secrets := []string{
		`{"text":"token sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"}`,
		`{"text":"-----BEGIN OPENSSH PRIVATE KEY-----\nabc"}`,
		"{\"text\":\"A=1\\nAPI_SECRET=xyz\\nB=2\"}",
	}
	for _, raw := range secrets {
		_, err := Tools["post"].Call(t.Context(), h, taskCaller, json.RawMessage(raw))
		got := assertErrorCode(t, err, "secret")
		if got.Error() != "secret: this looks like a secret; remove it and send again" {
			t.Fatalf("unexpected secret message: %q", got.Error())
		}
	}
	if len(home.asked) != 0 {
		t.Fatalf("secret post sent %d requests", len(home.asked))
	}

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
		{grpCaller, `{"text":"hi","extra":1}`},
		{grpCaller, `{"text":"hi","task":"T-12"}`},
		{secCaller, `{"text":"hi","extra":1}`},
		{taskCaller, `{"text":"hi","kind":"report"}`},
		{taskCaller, `{"text":"hi","kind":"summary"}`},
		{secCaller, `{"text":"hi","kind":"summary"}`},
		{secCaller, `{"text":"hi","where":"main","task":"T-12"}`},
		{grpCaller, `{"text":"hi","where":"sideways"}`},
		{grpCaller, `{"text":"hi","where":"thread"}`},
		{grpCaller, `{"text":"hi","kind":"progress"}`},
		{taskCaller, `{"text":"hi","kind":"progress","where":"main"}`},
	}
	for _, tc := range badInputs {
		_, err := Tools["post"].Call(t.Context(), h, tc.caller, json.RawMessage(tc.raw))
		assertErrorCode(t, err, "bad_request")
	}

	// Unknown member name lists sorted valid members; when none exist, says "none".
	_, err := Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi","mentions":["Ghost"]}`))
	got := assertErrorCode(t, err, "not_found")
	if got.Message != `unknown member "Ghost"; valid members: Claude Code, Codex, Secretary, owner` {
		t.Fatalf("got %q", got.Message)
	}

	_, err = Tools["post"].Call(t.Context(), h, secCaller, json.RawMessage(`{"text":"hi","kind":"summary","task":"T-99"}`))
	gotTask := assertErrorCode(t, err, "not_found")
	if gotTask.Message != `unknown task "T-99"; valid tasks: T-1, T-12` {
		t.Fatalf("got %q", gotTask.Message)
	}

	_, err = Tools["post"].Call(t.Context(), h, secCaller, json.RawMessage(`{"text":"hi","task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	// Empty group rows: valid list reads "none".
	h.Rows.Reset(talkGroupID)
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi","mentions":["Ghost"]}`))
	if got := assertErrorCode(t, err, "not_found"); got.Message != `unknown member "Ghost"; valid members: none` {
		t.Fatalf("got %q", got.Message)
	}
	// Single valid member pins len(valid) > 0 at boundary 1.
	talkPutRow(t, h, "member", talkOwnerID, proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: talkOwnerID, Seq: 1}, Group: talkGroupID, Kind: "human", User: "u1", Name: "owner", LastSeen: seamStart},
		Role:      "owner", Online: true,
	})
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi","mentions":["Ghost"]}`))
	if got := assertErrorCode(t, err, "not_found"); got.Message != `unknown member "Ghost"; valid members: owner` {
		t.Fatalf("got %q", got.Message)
	}

	if len(home.asked) != 0 {
		t.Fatalf("refused calls sent %d requests", len(home.asked))
	}

	// Home error propagation and unreachable home.
	home.replies = []json.RawMessage{json.RawMessage(`{"re":50,"err":{"code":"forbidden","msg":"denied"}}`)}
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi"}`))
	assertErrorCode(t, err, "forbidden")

	close(home.done)
	_, err = Tools["post"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"hi"}`))
	assertErrorCode(t, err, "home_unreachable")
}

func initTestRepo(t *testing.T, dir string) string {
	t.Helper()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "a.go")
	git("commit", "-m", "initial")
	return git("rev-parse", "HEAD")
}

func TestPostToolHandbackRefusalsAndGitFacts(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	// Pin every operand of checkHandbackCaller and checkKindPlace.
	interactiveHB := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: HandbackTurn, Thread: talkTask2ID})
	interactiveHB.Interactive = true
	_, err := Tools["post"].Call(t.Context(), h, interactiveHB, json.RawMessage(`{"text":"x","kind":"handback"}`))
	assertErrorCode(t, err, "bad_request")

	wrongTurn := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: ItemsTurn, Thread: talkTask2ID})
	_, err = Tools["post"].Call(t.Context(), h, wrongTurn, json.RawMessage(`{"text":"x","kind":"handback"}`))
	assertErrorCode(t, err, "bad_request")

	workingGroupHB := talkCaller(GroupSession, talkAI1ID, "", false, Turn{Kind: HandbackTurn})
	_, err = Tools["post"].Call(t.Context(), h, workingGroupHB, json.RawMessage(`{"text":"x","kind":"handback"}`))
	assertErrorCode(t, err, "bad_request")

	// Secretary handback: allowed in main, refused in thread.
	secHB := talkCaller(GroupSession, talkSecID, "", true, Turn{Kind: HandbackTurn})
	_, err = Tools["post"].Call(t.Context(), h, secHB, json.RawMessage(`{"text":"x","kind":"handback","task":"T-12"}`))
	assertErrorCode(t, err, "bad_request")

	if _, err := Tools["post"].Call(t.Context(), h, secHB, json.RawMessage(`{"text":"Summary","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}
	var secSent proto.Post[proto.HandbackBody]
	if err := proto.Decode([]byte(home.asked[0].(proto.Cmd).Env.Payload), &secSent); err != nil {
		t.Fatal(err)
	}
	if secSent.Thread != nil || secSent.Main || len(secSent.Body.Files) != 0 || len(secSent.Body.Commits) != 0 || secSent.Body.Added != 0 || secSent.Body.Removed != 0 {
		t.Fatalf("unexpected secretary handback: %+v", secSent)
	}
	if nulls := proto.UnexpectedNulls([]byte(home.asked[0].(proto.Cmd).Env.Payload), "/thread", "/re"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}

	// Review session handback with non-empty TakeoverAt and nonexistent folder still reports empty read-only facts.
	revHB := talkCaller(ReviewSession, talkAI2ID, talkTask2ID, false, Turn{Kind: HandbackTurn, Thread: talkTask2ID})
	revHB.Folder = "/nonexistent-review-folder"
	revHB.TakeoverAt = "deadbeef"
	_, err = Tools["post"].Call(t.Context(), h, revHB, json.RawMessage(`{"text":"x","kind":"handback","where":"main"}`))
	assertErrorCode(t, err, "bad_request")

	if _, err := Tools["post"].Call(t.Context(), h, revHB, json.RawMessage(`{"text":"Reviewed","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}
	var revSent proto.Post[proto.HandbackBody]
	if err := proto.Decode([]byte(home.asked[1].(proto.Cmd).Env.Payload), &revSent); err != nil ||
		revSent.Thread == nil || string(*revSent.Thread) != talkTask2ID || len(revSent.Body.Files) != 0 {
		t.Fatalf("unexpected review handback: %+v (%v)", revSent, err)
	}

	// Task session with empty TakeoverAt reports empty facts without touching nonexistent folder.
	emptyAtHB := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: HandbackTurn, Thread: talkTask2ID})
	emptyAtHB.Folder = "/nonexistent-task-folder"
	if _, err := Tools["post"].Call(t.Context(), h, emptyAtHB, json.RawMessage(`{"text":"No start","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}

	// Task session with non-empty TakeoverAt on missing folder returns git error (memory store, relative folder).
	missingHB := emptyAtHB
	missingHB.Folder = filepath.Join(t.TempDir(), "missing-wt")
	missingHB.TakeoverAt = "deadbeef"
	relMissingHB := emptyAtHB
	relMissingHB.Folder = "nonexistent-relative-wt-folder"
	relMissingHB.TakeoverAt = "deadbeef"
	if _, err := Tools["post"].Call(t.Context(), h, relMissingHB, json.RawMessage(`{"text":"err","kind":"handback"}`)); err == nil {
		t.Fatal("expected git error for missing relative folder")
	}

	// Real Git repository with commits, insertions, deletions and untracked files (absolute folder).
	repoDir := t.TempDir()
	startSHA := initTestRepo(t, repoDir)

	// Clean unchanged tree since startSHA -> 0 commits, 0 files, 0 added, 0 removed.
	cleanHB := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: HandbackTurn, Thread: talkTask2ID})
	cleanHB.Folder = repoDir
	cleanHB.TakeoverAt = startSHA
	if _, err := Tools["post"].Call(t.Context(), h, cleanHB, json.RawMessage(`{"text":"Nothing","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}
	var cleanSent proto.Post[proto.HandbackBody]
	if err := proto.Decode([]byte(home.asked[3].(proto.Cmd).Env.Payload), &cleanSent); err != nil ||
		len(cleanSent.Body.Files) != 0 || len(cleanSent.Body.Commits) != 0 || cleanSent.Body.Added != 0 || cleanSent.Body.Removed != 0 {
		t.Fatalf("unexpected clean handback: %+v (%v)", cleanSent, err)
	}

	// Add a commit and an untracked file.
	if err := os.WriteFile(filepath.Join(repoDir, "a.go"), []byte("line1\nchanged2\nadded3\nadded4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "b.go"), []byte("package b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repoDir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitRun("add", "a.go", "b.go")
	gitRun("commit", "-m", "Update path logic")
	if err := os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Tools["post"].Call(t.Context(), h, cleanHB, json.RawMessage(`{"text":"Done","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}
	var dirtySent proto.Post[proto.HandbackBody]
	if err := proto.Decode([]byte(home.asked[4].(proto.Cmd).Env.Payload), &dirtySent); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(dirtySent.Body.Files, []string{"a.go", "b.go", "untracked.txt"}) ||
		len(dirtySent.Body.Commits) != 1 || dirtySent.Body.Commits[0].Subject != "Update path logic" ||
		len(dirtySent.Body.Commits[0].SHA) < 4 || dirtySent.Body.Added != 4 || dirtySent.Body.Removed != 2 {
		t.Fatalf("unexpected dirty handback: %+v", dirtySent.Body)
	}

	// File-backed store resolves relative Session.Folder under <dataDir>/projects/<group>/<folder>.
	dataDir := t.TempDir()
	fileDB, err := store.Open(filepath.Join(dataDir, "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fileDB.Close() })
	fileHost, _ := seamHost(t, fileDB)
	fileHome := newTalkHomeStub()
	fileHost.Attach("home-device", fileHome)
	wtRel := filepath.Join("wt", "12-show-path")
	wtAbs := filepath.Join(dataDir, "projects", talkGroupID, wtRel)
	if err := os.MkdirAll(wtAbs, 0o700); err != nil {
		t.Fatal(err)
	}
	wtStart := initTestRepo(t, wtAbs)
	if err := os.WriteFile(filepath.Join(wtAbs, "note.md"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	relCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{Kind: HandbackTurn, Thread: talkTask2ID})
	relCaller.Folder = wtRel
	relCaller.TakeoverAt = wtStart
	if _, err := Tools["post"].Call(t.Context(), fileHost, relCaller, json.RawMessage(`{"text":"Rel","kind":"handback"}`)); err != nil {
		t.Fatal(err)
	}
	var relSent proto.Post[proto.HandbackBody]
	if err := proto.Decode([]byte(fileHome.asked[0].(proto.Cmd).Env.Payload), &relSent); err != nil || !slices.Equal(relSent.Body.Files, []string{"note.md"}) {
		t.Fatalf("unexpected file-backed handback: %+v (%v)", relSent.Body, err)
	}

	// Direct unit pins for parseShortstat and collectHandbackFiles edge cases (dedup, singular, CRLF).
	if add, rem := parseShortstat(" 1 file changed, 1 insertion(+)\n"); add != 1 || rem != 0 {
		t.Fatalf("got %d/%d", add, rem)
	}
	if add, rem := parseShortstat(" 1 file changed, 5 deletions(-)\n"); add != 0 || rem != 5 {
		t.Fatalf("got %d/%d", add, rem)
	}
	if got := collectHandbackFiles("a.go\r\nb.go\r\n", " M a.go\r\n?? a.go\r\n?? c.go\r\n"); !slices.Equal(got, []string{"a.go", "b.go", "c.go"}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadThreadTool(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)

	threadStr := talkTask2ID
	row1 := proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m1____________________A", Seq: 20},
		Pos:       20, Group: talkGroupID, Thread: &threadStr, By: talkAI2ID, Kind: "text", Timestamp: seamStart, Text: "Review ready", Mentions: []string{}, Body: proto.EmptyBody{},
	}
	row2 := proto.MessageRow[proto.SystemCode, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m2____________________A", Seq: 19},
		Pos:       19, Group: talkGroupID, Thread: &threadStr, By: "system", Kind: "system", Timestamp: seamStart - 1000, Text: "", Mentions: []string{}, Body: proto.SystemCode{Code: "no_secretary"},
	}
	row3 := proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: "m3____________________A", Seq: 18},
		Pos:       18, Group: talkGroupID, Thread: &threadStr, By: "goneMember000000000000A", Kind: "question", Timestamp: seamStart - 2000, Text: "Old?", Mentions: []string{}, Body: proto.EmptyBody{},
	}
	r1, _ := json.Marshal(row1)
	r2, _ := json.Marshal(row2)
	r3, _ := json.Marshal(row3)
	page1, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 50, OK: true}, Rows: []json.RawMessage{r1, r2, r3}, More: true})
	pageEmpty, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 51, OK: true}, Rows: []json.RawMessage{}, More: false})

	home := newTalkHomeStub(string(page1), string(pageEmpty), string(pageEmpty))
	h.Attach("home-device", home)

	// Task session omits task -> defaults to Session.Task, limit=30, before=0.
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})
	out, err := Tools["read_thread"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	got := out.(readThreadResult)
	want := readThreadResult{
		Messages: []threadMsgItem{
			{ID: row1.ID, Pos: 20, By: "Codex", Kind: "text", At: seamStart, Text: "Review ready"},
			{ID: row2.ID, Pos: 19, By: "system", Kind: "system", At: seamStart - 1000, Text: ""},
			{ID: row3.ID, Pos: 18, By: "goneMember000000000000A", Kind: "question", At: seamStart - 2000, Text: "Old?"},
		},
		More: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	req0 := home.asked[0].(proto.FetchThread)
	if req0.Group != talkGroupID || req0.Limit != 30 || req0.Before != 0 || req0.Thread == nil || *req0.Thread != talkTask2ID {
		t.Fatalf("unexpected FetchThread: %+v", req0)
	}

	// Group session reads "main" at exact boundary limit=1, before=1; then task "T-1" at boundary limit=100.
	grpCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	outMain, err := Tools["read_thread"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"main","limit":1,"before":1}`))
	if err != nil {
		t.Fatal(err)
	}
	encodedEmpty, _ := json.Marshal(outMain)
	if nulls := proto.UnexpectedNulls(encodedEmpty); len(nulls) != 0 {
		t.Fatalf("unexpected nulls in empty read_thread: %v", nulls)
	}
	req1 := home.asked[1].(proto.FetchThread)
	if req1.Thread != nil || req1.Limit != 1 || req1.Before != 1 {
		t.Fatalf("unexpected main FetchThread: %+v", req1)
	}

	if _, err := Tools["read_thread"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"T-1","limit":100}`)); err != nil {
		t.Fatal(err)
	}
	req2 := home.asked[2].(proto.FetchThread)
	if req2.Thread == nil || *req2.Thread != talkTask1ID || req2.Limit != 100 || req2.Before != 0 {
		t.Fatalf("unexpected T-1 FetchThread: %+v", req2)
	}

	// Refusals: missing task in group session, out-of-range limit/before boundaries, unknown arg, unknown task.
	for _, bad := range []string{
		`{}`,
		`{"task":"main","limit":0}`,
		`{"task":"main","limit":101}`,
		`{"task":"main","before":0}`,
		`{"task":"main","before":-1}`,
		`{"task":"main","limit":1.5}`,
		`{"task":"main","extra":true}`,
		`[]`,
	} {
		_, err := Tools["read_thread"].Call(t.Context(), h, grpCaller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}
	_, err = Tools["read_thread"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	// Malformed row, malformed fetched frame, wire ErrorResponse, Request error, disconnected home.
	home.replies = append(home.replies,
		json.RawMessage(`{"t":"fetched","re":53,"ok":true,"rows":[{"r":"msg"}],"more":false}`),
		json.RawMessage(`{"t":"not_fetched","re":54,"ok":true}`),
		json.RawMessage(`{"re":55,"err":{"code":"not_found","msg":"missing thread"}}`),
	)
	_, err = Tools["read_thread"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "bad_request")
	_, err = Tools["read_thread"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "bad_request")
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

	// Empty group returns [] without nulls.
	emptyOut, err := Tools["list_members"].Call(t.Context(), h, caller, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	rawEmpty, _ := json.Marshal(emptyOut)
	if string(rawEmpty) != `{"members":[]}` {
		t.Fatalf("got %s", rawEmpty)
	}

	seedTalkGroup(t, h)
	// Add offline human, AI on aged-out task ID, gone marker, and malformed member rows.
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

	out, err := Tools["list_members"].Call(t.Context(), h, caller, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	got := out.(listMembersResult).Members
	winPct := 42.5
	emptyUsage := proto.Usage{}
	withPct := proto.Usage{WindowPct: &winPct}
	want := []memberListItem{
		{Name: "Claude Code", Kind: "ai", State: "working", Host: "workstation", Account: "personal", Task: "T-12", Usage: &withPct},
		{Name: "Codex", Kind: "ai", State: "dormant", Host: seamKey().ID(), Account: "team", Usage: &emptyUsage},
		{Name: "Secretary", Kind: "ai", State: "waiting", Host: "workstation", Account: "personal", Usage: &emptyUsage},
		{Name: "Worker", Kind: "ai", State: "working", Host: "dev-raw", Account: "api_key", Task: "agedOutTask00000000000A", Usage: &emptyUsage},
		{Name: "alice", Kind: "human", State: "offline"},
		{Name: "owner", Kind: "human", State: "online"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	encoded, _ := json.Marshal(out)
	if nulls := proto.UnexpectedNulls(encoded); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}
	if strings.Contains(string(encoded), `"name":"alice","kind":"human","state":"offline","host"`) || strings.Contains(string(encoded), `"name":"Codex","kind":"ai","state":"dormant","host":"`+seamKey().ID()+`","account":"team","task"`) {
		t.Fatalf("unexpected omitted fields in %s", encoded)
	}

	_, err = Tools["list_members"].Call(t.Context(), h, caller, json.RawMessage(`{"extra":1}`))
	assertErrorCode(t, err, "bad_request")
	_, err = Tools["list_members"].Call(t.Context(), h, caller, json.RawMessage(`[]`))
	assertErrorCode(t, err, "bad_request")
}

func TestListTasksToolAndIdleMinutesBoundaries(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	// Fixed clock at seamStart + 180_000 for deterministic idle_minutes math.
	fixedNow := int64(seamStart + 180_000)
	h.Now = func() time.Time { return time.UnixMilli(fixedNow) }

	// Add tasks covering Thread.Last, Created fallback, merged, cancelled, Ended set, unknown member IDs, and GoneRow.
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
	outAll, err := Tools["list_tasks"].Call(t.Context(), h, caller, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	dependsKind := "depends"
	manualKind := "manual"
	wantAll := []taskListItem{
		{Task: "T-1", Title: "First task", State: "queued", Step: "branch", Assignee: "Codex", Wait: &dependsKind, IdleMinutes: 4},
		{Task: "T-3", Title: "Thread timed", State: "paused", Step: "local_check", Assignee: "unknownAssignee0000000A", Reviewer: "unknownReviewer0000000A", Wait: &manualKind, IdleMinutes: 1},
		{Task: "T-4", Title: "Merged task", State: "merged", Step: "merge", Assignee: "Claude Code", Wait: nil, IdleMinutes: 0},
		{Task: "T-12", Title: "Show path", State: "active", Step: "review", Assignee: "Claude Code", Reviewer: "Codex", Wait: nil, IdleMinutes: 5},
	}
	if got := outAll.(listTasksResult).Tasks; !reflect.DeepEqual(got, wantAll) {
		t.Fatalf("got %+v\nwant %+v", got, wantAll)
	}
	encoded, _ := json.Marshal(outAll)
	if nulls := proto.UnexpectedNulls(encoded, "/tasks/2/wait", "/tasks/3/wait"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}

	// Filter by each state.
	outPaused, err := Tools["list_tasks"].Call(t.Context(), h, caller, json.RawMessage(`{"state":"paused"}`))
	if err != nil || len(outPaused.(listTasksResult).Tasks) != 1 || outPaused.(listTasksResult).Tasks[0].Task != "T-3" {
		t.Fatalf("got %+v (%v)", outPaused, err)
	}
	outFailed, err := Tools["list_tasks"].Call(t.Context(), h, caller, json.RawMessage(`{"state":"failed"}`))
	if err != nil || len(outFailed.(listTasksResult).Tasks) != 0 {
		t.Fatalf("got %+v (%v)", outFailed, err)
	}

	for _, bad := range []string{`{"state":"running"}`, `{"state":null}`, `{"extra":1}`, `[]`} {
		_, err := Tools["list_tasks"].Call(t.Context(), h, caller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}

	// Pin every operand and millisecond boundary of taskIdleMinutes directly.
	ended := int64(100)
	idleCases := []struct {
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
	}
	for i, tc := range idleCases {
		if got := taskIdleMinutes(tc.row, tc.now); got != tc.want {
			t.Fatalf("case %d: got %d, want %d", i, got, tc.want)
		}
	}
}

func TestNotePutAndNoteListTools(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	seedTalkGroup(t, h)
	home := newTalkHomeStub()
	h.Attach("home-device", home)

	grpCaller := talkCaller(GroupSession, talkSecID, "", true, Turn{})
	taskCaller := talkCaller(TaskSession, talkAI1ID, talkTask2ID, false, Turn{})
	revCaller := talkCaller(ReviewSession, talkAI2ID, talkTask1ID, false, Turn{})

	// Secret check blocks note_put before sending.
	_, err := Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"ghp_0123456789abcdefghijklmnopqrstuvwxyz"}`))
	assertErrorCode(t, err, "secret")
	if len(home.asked) != 0 {
		t.Fatal("secret note was sent")
	}

	// Group session without task -> Note.Task is nil; with task -> resolved ID.
	res1, err := Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"Group convention"}`))
	if err != nil {
		t.Fatal(err)
	}
	var noteCmd1 proto.Note
	raw1 := []byte(home.asked[0].(proto.Cmd).Env.Payload)
	if err := proto.Decode(raw1, &noteCmd1); err != nil {
		t.Fatal(err)
	}
	if nulls := proto.UnexpectedNulls(raw1, "/task"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}
	if string(noteCmd1.ID) != res1.(idResult).ID || noteCmd1.ID == noteCmd1.Nonce || noteCmd1.Task != nil || string(noteCmd1.As) != talkSecID || noteCmd1.Text != "Group convention" {
		t.Fatalf("unexpected group note cmd: %+v", noteCmd1)
	}

	if _, err := Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"Task convention","task":"T-12"}`)); err != nil {
		t.Fatal(err)
	}
	var noteCmd2 proto.Note
	if err := proto.Decode([]byte(home.asked[1].(proto.Cmd).Env.Payload), &noteCmd2); err != nil || noteCmd2.Task == nil || string(*noteCmd2.Task) != talkTask2ID {
		t.Fatalf("unexpected named-task note cmd: %+v (%v)", noteCmd2, err)
	}

	// Task and review sessions default omitted task to Session.Task.
	for i, c := range []Caller{taskCaller, revCaller} {
		if _, err := Tools["note_put"].Call(t.Context(), h, c, json.RawMessage(`{"text":"Session note"}`)); err != nil {
			t.Fatal(err)
		}
		var nc proto.Note
		if err := proto.Decode([]byte(home.asked[2+i].(proto.Cmd).Env.Payload), &nc); err != nil || nc.Task == nil || string(*nc.Task) != c.Task {
			t.Fatalf("unexpected session note %d: %+v (%v)", i, nc, err)
		}
	}

	// note_put refusals.
	for _, bad := range []string{`{}`, `{"text":null}`, `{"text":"x","extra":1}`, `[]`} {
		_, err := Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(bad))
		assertErrorCode(t, err, "bad_request")
	}
	_, err = Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"x","task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	// note_list across multiple pages + task filtering + empty-page More=true guard.
	t12 := talkTask2ID
	tAged := "agedTask00000000000000A"
	nRow1, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "n1____________________A", Seq: 30}, Pos: 30, Group: talkGroupID, Task: &t12, Text: "First", By: talkAI1ID, Timestamp: seamStart})
	nRow2, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "n2____________________A", Seq: 25}, Pos: 25, Group: talkGroupID, Task: nil, Text: "Global", By: "goneAuthor000000000000A", Timestamp: seamStart - 1000})
	nRow3, _ := json.Marshal(proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: "n3____________________A", Seq: 20}, Pos: 20, Group: talkGroupID, Task: &tAged, Text: "Old task", By: talkOwnerID, Timestamp: seamStart - 2000})

	p1, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 60, OK: true}, Rows: []json.RawMessage{nRow1, nRow2}, More: true})
	p2, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 61, OK: true}, Rows: []json.RawMessage{nRow3}, More: false})
	pEmptyMore, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 62, OK: true}, Rows: []json.RawMessage{}, More: true})

	listHome := newTalkHomeStub(string(p1), string(p2), string(p1), string(p2), string(pEmptyMore))
	h.Attach("home-device", listHome)

	allOut, err := Tools["note_list"].Call(t.Context(), h, taskCaller, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	lbl12 := "T-12"
	wantNotes := []noteListItem{
		{ID: "n1____________________A", Task: &lbl12, Text: "First", By: "Claude Code", At: seamStart},
		{ID: "n2____________________A", Task: nil, Text: "Global", By: "goneAuthor000000000000A", At: seamStart - 1000},
		{ID: "n3____________________A", Task: &tAged, Text: "Old task", By: "owner", At: seamStart - 2000},
	}
	if got := allOut.(noteListResult).Notes; !reflect.DeepEqual(got, wantNotes) {
		t.Fatalf("got %+v\nwant %+v", got, wantNotes)
	}
	reqPage1 := listHome.asked[0].(proto.FetchKind)
	reqPage2 := listHome.asked[1].(proto.FetchKind)
	if reqPage1.Kind != "note" || reqPage1.Before != 0 || reqPage1.Limit != 100 || reqPage2.Before != 25 {
		t.Fatalf("unexpected note pagination: %+v then %+v", reqPage1, reqPage2)
	}
	encodedAll, _ := json.Marshal(allOut)
	if nulls := proto.UnexpectedNulls(encodedAll, "/notes/1/task"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls: %v", nulls)
	}

	// Filtered note_list returns only T-12 notes.
	filteredOut, err := Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"T-12"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := filteredOut.(noteListResult).Notes; !reflect.DeepEqual(got, wantNotes[:1]) {
		t.Fatalf("got %+v, want %+v", got, wantNotes[:1])
	}

	// Empty page with More=true terminates cleanly with [].
	emptyMoreOut, err := Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{}`))
	if err != nil || len(emptyMoreOut.(noteListResult).Notes) != 0 {
		t.Fatalf("got %+v (%v)", emptyMoreOut, err)
	}

	// Refusals and error paths on note_list and note_put.
	_, err = Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"extra":1}`))
	assertErrorCode(t, err, "bad_request")
	_, err = Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"task":"T-99"}`))
	assertErrorCode(t, err, "not_found")

	listHome.replies = append(listHome.replies, json.RawMessage(`{"t":"fetched","re":70,"ok":true,"rows":[{"r":"note"}],"more":false}`))
	_, err = Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "bad_request")

	close(listHome.done)
	_, err = Tools["note_list"].Call(t.Context(), h, grpCaller, json.RawMessage(`{}`))
	assertErrorCode(t, err, "home_unreachable")
	_, err = Tools["note_put"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"text":"offline"}`))
	assertErrorCode(t, err, "home_unreachable")
}
