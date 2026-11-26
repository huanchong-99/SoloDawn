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
	"sync"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	tskTestGroupID  = "rZNvy-1jH6Z-BcPqA5U5BQ"
	tskTestTaskID   = "DrtCn6htSBwmMPrFPbHJHA"
	tskTestWorkerID = "1PC8WineBrUQ-apCjx7tug"
	tskTestReviewID = "LXCZmuGAXkvO-bSrOkuCfw"
)

type tskReplyStep struct {
	answer string
	err    error
}

type tskScriptHome struct {
	mu sync.Mutex
	// Single reader: Host.Home via Done.
	done  chan struct{}
	steps []tskReplyStep
	asked []any
	// before runs for each request before its answer, as a home pushes the
	// rows a command changed before it answers.
	before func(req any)
}

func newTskScriptHome(steps ...tskReplyStep) *tskScriptHome {
	return &tskScriptHome{done: make(chan struct{}), steps: steps}
}

func (f *tskScriptHome) Send(any) error               { return nil }
func (f *tskScriptHome) Path() string                 { return "local" }
func (f *tskScriptHome) PathChanged() <-chan struct{} { return nil }
func (f *tskScriptHome) Done() <-chan struct{}        { return f.done }

func (f *tskScriptHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.asked) + 1)
	req := build(id)
	f.asked = append(f.asked, req)
	step, before := f.popStepLocked(id), f.before
	f.mu.Unlock()
	if before != nil {
		before(req)
	}
	return json.RawMessage(step.answer), step.err
}

func (f *tskScriptHome) popStepLocked(id int64) tskReplyStep {
	if len(f.steps) == 0 {
		return tskReplyStep{answer: fmt.Sprintf(`{"re":%d,"ok":true}`, id)}
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step
}

func (f *tskScriptHome) snapshotAsked() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

func tskEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func tskRequire(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}

func tskNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func tskPutRow(t *testing.T, h *Host, kind, id string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	tskNoErr(t, err)
	h.Rows.Put(tskTestGroupID, kind, id, raw)
}

func tskOpenSignedCmd[T any](t *testing.T, h *Host, raw any) T {
	t.Helper()
	cmd, ok := raw.(proto.Cmd)
	tskRequire(t, ok, "expected proto.Cmd")
	known := func(dev string) bool { return dev == h.Key.ID() }
	_, err := identity.OpenCommand(cmd.Env, "", known, h.Now())
	tskNoErr(t, err)
	var decoded T
	tskNoErr(t, proto.Decode([]byte(cmd.Env.Payload), &decoded))
	return decoded
}

func tskTaskCaller() Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: tskTestGroupID, Member: tskTestWorkerID, Task: tskTestTaskID},
			Kind:       TaskSession,
			Home:       "home-device",
		},
		Level: 3,
	}
}

func tskListedOwned(listed map[string]Tool, owned []string) []string {
	var got []string
	for _, name := range owned {
		if _, ok := listed[name]; ok {
			got = append(got, name)
		}
	}
	slices.Sort(got)
	return got
}

func tskAssertGoldenSubset(t *testing.T, listed map[string]Tool, golden proto.ToolList, owned []string) {
	t.Helper()
	var want []string
	for _, tool := range golden.Tools {
		if slices.Contains(owned, tool.Name) {
			want = append(want, tool.Name)
			tskAssertToolSchema(t, listed[tool.Name], tool.InputSchema)
		}
	}
	slices.Sort(want)
	tskEqual(t, tskListedOwned(listed, owned), want)
}

func tskAssertToolSchema(t *testing.T, got Tool, wantInput proto.ToolInput) {
	t.Helper()
	tskRequire(t, got.Description != "", "empty tool description")
	var gotVal, wantVal any
	tskNoErr(t, json.Unmarshal(got.Schema, &gotVal))
	rawWant, err := json.Marshal(wantInput)
	tskNoErr(t, err)
	tskNoErr(t, json.Unmarshal(rawWant, &wantVal))
	tskEqual(t, gotVal, wantVal)
}

func TestTaskToolsMatchContractLists(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	tskNoErr(t, err)
	var lists proto.ToolLists
	tskNoErr(t, json.Unmarshal(data, &lists))
	owned := []string{"start_task", "submit_task", "submit_review"}
	cases := []struct {
		caller Caller
		golden proto.ToolList
	}{
		{Caller{Session: Session{Kind: GroupSession, Secretary: true}, Level: 1}, lists.Secretary},
		{Caller{Session: Session{Kind: GroupSession, Secretary: false}, Level: 1}, lists.Group},
		{Caller{Session: Session{Kind: ReviewSession}, Level: 1}, lists.Review},
		{Caller{Session: Session{Kind: TaskSession}, Level: 1}, lists.Task},
		{Caller{Session: Session{Kind: TaskSession}, Level: 3}, lists.Task},
		{Caller{Session: Session{Kind: GroupSession}, Level: 3, External: true}, lists.External},
	}
	for _, tc := range cases {
		tskAssertGoldenSubset(t, ToolsFor(tc.caller), tc.golden, owned)
	}
}

func TestTaskToolForPredicates(t *testing.T) {
	tskEqual(t, tskForWorkingGroup(Caller{Session: Session{Kind: GroupSession, Secretary: false}}), true)
	tskEqual(t, tskForWorkingGroup(Caller{Session: Session{Kind: GroupSession, Secretary: true}}), false)
	tskEqual(t, tskForWorkingGroup(Caller{Session: Session{Kind: TaskSession, Secretary: false}}), false)
	tskEqual(t, tskForTask(Caller{Session: Session{Kind: TaskSession}, Level: 1}), true)
	tskEqual(t, tskForTask(Caller{Session: Session{Kind: ReviewSession}, Level: 3}), false)
	tskEqual(t, tskForReview(Caller{Session: Session{Kind: ReviewSession}}), true)
	tskEqual(t, tskForReview(Caller{Session: Session{Kind: TaskSession}}), false)
}

func tskGroupCaller(turn Turn) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: tskTestGroupID, Member: tskTestWorkerID},
			Kind:       GroupSession,
			Home:       "home-device",
		},
		Level: 1,
		Turn:  turn,
	}
}

func TestStartTaskWithoutTurn(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newTskScriptHome(tskReplyStep{answer: `{"re":1,"ok":true,"num":14}`})
	h.Attach("home-device", home)
	res, err := Tools["start_task"].Call(t.Context(), h, tskGroupCaller(Turn{Kind: ItemsTurn}), json.RawMessage(`{"title":"Add badge","plan":"Update TaskCard."}`))
	tskNoErr(t, err)
	tskEqual(t, marshalWire(res), `{"task":"T-14","moved":true}`)
	asked := home.snapshotAsked()
	tskEqual(t, len(asked), 1)
	tskRequire(t, !strings.HasSuffix(asked[0].(proto.Cmd).Env.Payload, "\n"), "payload has trailing newline")
	started := tskOpenSignedCmd[proto.TaskStart](t, h, asked[0])
	tskEqual(t, started.Op, "start")
	tskEqual(t, string(started.As), tskTestWorkerID)
	tskEqual(t, started.Origin, "direct")
	tskEqual(t, started.Re, (*proto.ID)(nil))
	tskEqual(t, started.Title, "Add badge")
	tskEqual(t, started.Plan, "Update TaskCard.")
	tskRequire(t, started.ID != "" && started.ID != started.Nonce, "expected fresh task id distinct from nonce")
}

func TestStartTaskErrorsAndBoundaries(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	call := Tools["start_task"].Call
	caller := tskGroupCaller(Turn{Kind: ItemsTurn})

	_, err := call(t.Context(), h, caller, json.RawMessage(`{"title":"T","plan":"P"}`))
	seamCode(t, err, "home_unreachable")

	home := newTskScriptHome()
	h.Attach("home-device", home)

	for _, bad := range []string{
		`[]`,
		`{"title":"only"}`,
		`{"title":1,"plan":"c"}`,
	} {
		_, err := call(t.Context(), h, caller, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	const secretKey = "-----BEGIN " + "PRIVATE KEY-----"
	for _, secretArg := range []string{
		fmt.Sprintf(`{"title":%q,"plan":"clean"}`, secretKey),
		fmt.Sprintf(`{"title":"clean","plan":%q}`, secretKey),
	} {
		_, err := call(t.Context(), h, caller, json.RawMessage(secretArg))
		seamCode(t, err, "secret")
	}
	tskEqual(t, len(home.snapshotAsked()), 0)
}

// The home pushes the row a task start made before it answers. A task that is
// queued behind another one in its folder is told as queued, not as moved.
func TestStartTaskQueuedBehindTheFolder(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newTskScriptHome(tskReplyStep{answer: `{"re":1,"ok":true,"num":15}`})
	home.before = func(req any) {
		started := tskOpenSignedCmd[proto.TaskStart](t, h, req)
		tskPutRow(t, h, "task", string(started.ID), proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: string(started.ID), Seq: 9}, Group: tskTestGroupID, Num: 15, Title: started.Title,
			Origin: "direct", Plan: started.Plan, State: "queued", Step: "work", Assignee: tskTestWorkerID, Depends: []string{},
			Wait: &proto.TaskWait{Kind: "busy", Since: seamStart},
		})
	}
	h.Attach("home-device", home)
	res, err := Tools["start_task"].Call(t.Context(), h, tskGroupCaller(Turn{Kind: ItemsTurn}), json.RawMessage(`{"title":"Add badge","plan":"Update TaskCard."}`))
	tskNoErr(t, err)
	tskEqual(t, marshalWire(res), `{"task":"T-15","queued":true}`)
}

// submit_task is one command: the summary, as the AI wrote it, posted as a
// report in the task's thread. The node adds nothing and reads no folder.
func TestSubmitTaskPostsTheReport(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newTskScriptHome()
	h.Attach("home-device", home)
	const summary = "Path events are sent on every switch.\nRan go test ./...: 42 passed. Commit 9f2c1ab on branch t12-path-events."
	res, err := Tools["submit_task"].Call(t.Context(), h, tskTaskCaller(), json.RawMessage(marshalWire(tskSubmitArgs{Summary: summary})))
	tskNoErr(t, err)
	tskEqual(t, marshalWire(res), `{"ok":true}`)

	asked := home.snapshotAsked()
	tskEqual(t, len(asked), 1)
	tskRequire(t, strings.HasSuffix(asked[0].(proto.Cmd).Env.Payload, `"body":{}}`), "the report's body is not {}")
	report := tskOpenSignedCmd[proto.Post[proto.EmptyBody]](t, h, asked[0])
	wantTaskID := proto.ID(tskTestTaskID)
	report.HomeCommand = proto.HomeCommand{Type: report.Type, Group: report.Group, As: report.As}
	tskEqual(t, report, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Group: tskTestGroupID, As: tskTestWorkerID},
		Kind:        "report", Thread: &wantTaskID, Text: summary, Mentions: []proto.ID{},
	})
}

func TestSubmitTaskRefusals(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	call := Tools["submit_task"].Call
	caller := tskTaskCaller()
	_, err := call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "home_unreachable")

	home := newTskScriptHome()
	h.Attach("home-device", home)
	for _, bad := range []string{`{}`, `[]`, `{"summary":1}`} {
		_, err = call(t.Context(), h, caller, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"}`))
	seamCode(t, err, "secret")

	// A task whose last failures say to ask a stronger AI is not submitted before that.
	tskPutRow(t, h, "member", tskTestWorkerID, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: tskTestWorkerID, Seq: 2}, Group: tskTestGroupID, Kind: "ai", User: "u", Name: "Claude Code"},
		CLI:       "claude", Host: h.Key.ID(), HostName: "workstation", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "working", Path: "local",
	})
	stuck := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: tskTestTaskID, Seq: 3}, Group: tskTestGroupID, Num: 12, Title: "Fix path check", Origin: "brief",
		State: "active", Step: "work", Assignee: tskTestWorkerID, Reviewer: tskTestReviewID, Depends: []string{}, Stuck: &proto.Stuck{Kind: "review", N: 2, Ask: true},
	}
	tskPutRow(t, h, "task", tskTestTaskID, stuck)
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	tskEqual(t, err, error(&Error{Code: "conflict", Message: "Ask a stronger AI before submitting again. Candidates: none currently listed."}))
	tskEqual(t, len(home.snapshotAsked()), 0)

	// What the home answers to the post is the tool's answer.
	stuck.Stuck = nil
	tskPutRow(t, h, "task", tskTestTaskID, stuck)
	home.mu.Lock()
	home.steps = []tskReplyStep{{answer: `{"re":1,"err":{"code":"conflict","msg":"the task is not yours to submit now"}}`}}
	home.mu.Unlock()
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	tskEqual(t, err, error(&Error{Code: "conflict", Message: "the task is not yours to submit now"}))
}

// submit_review names the report its session was given when the review item
// was delivered, as the session's row holds it now.
func TestSubmitReviewNamesTheReportOfItsSession(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newTskScriptHome(
		tskReplyStep{answer: `{"re":1,"ok":true,"passed":false,"round":1}`},
		tskReplyStep{answer: `{"re":2,"ok":true,"passed":true,"round":2}`},
	)
	h.Attach("home-device", home)
	key := SessionKey{Group: tskTestGroupID, Member: tskTestReviewID, Task: tskTestTaskID}
	tskNoErr(t, h.Sessions.Create(Session{SessionKey: key, Kind: ReviewSession, Folder: t.TempDir(), Agent: "codex", Home: "home-device", Name: "Codex"}))
	token := h.Sessions.Admit(key, 3, false)
	review := func(report proto.ID, args string) any {
		t.Helper()
		tskNoErr(t, h.Sessions.Update(key, func(s *Session) { s.Report = string(report) }))
		caller, _ := h.Sessions.Caller(token)
		res, err := Tools["submit_review"].Call(t.Context(), h, caller, json.RawMessage(args))
		tskNoErr(t, err)
		return res
	}

	first, second := proto.ID(testMsg1), proto.ID(testMsg2)
	res := review(first, `{"score":0,"items":[{"text":"Missing test"},{"text":"Check line","file":"x.go","line":1,"veto":true}]}`)
	tskEqual(t, res, tskReviewResult{Passed: false, Round: 1})
	rev0 := tskOpenSignedCmd[proto.Post[proto.ReviewSigned]](t, h, home.snapshotAsked()[0])
	tskEqual(t, rev0.Kind, "review")
	tskEqual(t, string(rev0.As), tskTestReviewID)
	wantTaskID := proto.ID(tskTestTaskID)
	tskEqual(t, rev0.Thread, &wantTaskID)
	tskEqual(t, rev0.Main, false)
	tskEqual(t, rev0.Text, "")
	wantFile := "x.go"
	wantLine := 1
	tskEqual(t, rev0.Body, proto.ReviewSigned{
		Task:   wantTaskID,
		Report: first,
		Score:  0,
		Items: []proto.ReviewItem{
			{Text: "Missing test", Veto: false},
			{Text: "Check line", Veto: true, File: &wantFile, Line: &wantLine},
		},
	})

	// The task was submitted again, and the session was given the new report.
	res = review(second, `{"score":100,"items":[{"text":"Clean","veto":false}]}`)
	tskEqual(t, res, tskReviewResult{Passed: true, Round: 2})
	rev100 := tskOpenSignedCmd[proto.Post[proto.ReviewSigned]](t, h, home.snapshotAsked()[1])
	tskEqual(t, rev100.Body.Report, second)
	tskEqual(t, rev100.Body.Score, 100)
	tskEqual(t, rev100.Body.Items[0].Veto, false)
}

func TestSubmitReviewRefusalsAndSecrets(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newTskScriptHome(tskReplyStep{err: os.ErrClosed})
	h.Attach("home-device", home)
	call := Tools["submit_review"].Call
	caller := Caller{Session: Session{
		SessionKey: SessionKey{Group: tskTestGroupID, Member: tskTestReviewID, Task: tskTestTaskID},
		Kind:       ReviewSession, Home: "home-device", Report: testMsg1,
	}, Level: 3}

	for _, bad := range []string{
		`[]`,
		`{"score":50,"items":[123]}`,
	} {
		_, err := call(t.Context(), h, caller, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	secretTok := "AKIA" + strings.Repeat("B", 16)
	_, err := call(t.Context(), h, caller, json.RawMessage(fmt.Sprintf(`{"score":80,"items":[{"text":%q}]}`, secretTok)))
	seamCode(t, err, "secret")
	_, err = call(t.Context(), h, caller, json.RawMessage(fmt.Sprintf(`{"score":80,"items":[{"text":"clean","file":%q}]}`, secretTok)))
	seamCode(t, err, "secret")
	tskEqual(t, len(home.snapshotAsked()), 0)

	_, err = call(t.Context(), h, caller, json.RawMessage(`{"score":90,"items":[]}`))
	seamCode(t, err, "home_unreachable")
}
