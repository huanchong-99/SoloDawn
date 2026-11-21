package home_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	calA      = id(80)
	calB      = id(81)
	calC      = id(82)
	calTaskID = id(83)
	calOtherT = id(84)
	calPrevID = id(85)
)

const (
	calCallTS  = clock - 48_500
	calReplyTS = clock - 500
)

func calState(t *testing.T) *home.State {
	t.Helper()
	s := group(t)
	s.Group.Limits = proto.Limits{
		Rounds: 10, WakesPerHour: 20, TurnMinutes: 60, Failures: 3, ReviewRounds: 5,
		AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30,
	}
	for _, m := range []string{calA, calB, calC} {
		s.AIs[m] = ai(m, laptop)
	}
	s.Tasks[calTaskID] = calMakeTask(calTaskID, 1)
	s.Tasks[calOtherT] = calMakeTask(calOtherT, 2)
	return s
}

func calMakeTask(taskID string, num int) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: taskID, Seq: int64(num + 5)},
		Group:     groupID, Num: num, Title: "Path events", Slug: proto.MakeSlug("Path events"),
		Origin: "brief", Brief: id(86), State: "active", Step: "local_check",
		Assignee: calA, Reviewer: calB, Pass: 90, Depends: []string{},
		Thread: proto.ThreadSummary{Count: 2, Rounds: 1},
		Stats:  proto.TaskStats{Started: clock - 100_000}, Created: clock - 100_000,
	}
}

func calIDList(members ...string) []proto.ID {
	out := make([]proto.ID, len(members))
	for i, m := range members {
		out[i] = proto.ID(m)
	}
	return out
}

func calCallCmd(n byte, ts int64, as string, thread *string, text string, body proto.CallBody) proto.Post[proto.CallBody] {
	return proto.Post[proto.CallBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(id(n)), Timestamp: ts, Group: proto.ID(groupID), As: proto.ID(as),
		},
		Kind: "call", Thread: (*proto.ID)(thread), Text: text, Mentions: []proto.ID{}, Body: body,
	}
}

func calReplyCmd(n byte, ts int64, as string, thread *string, callID, text string) proto.Post[proto.EmptyBody] {
	re := proto.ID(callID)
	return proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(id(n)), Timestamp: ts, Group: proto.ID(groupID), As: proto.ID(as),
		},
		Kind: "reply", Thread: (*proto.ID)(thread), Text: text, Mentions: []proto.ID{}, Re: &re, Body: proto.EmptyBody{},
	}
}

func calSend(t *testing.T, s *home.State, d home.Disk, r home.Receipts, by identity.Key, payload any) (proto.Envelope, home.Result) {
	t.Helper()
	env := briefSign(by, payload)
	return env, briefApply(t, s, d, r, by, env)
}

func calExpectedRounds(base int, mentionsAI bool) int {
	if _, wrapped := home.Commands["jump"]; wrapped && mentionsAI {
		return base + 1
	}
	return base
}

func calCheckNulls(t *testing.T, row any, allowed ...string) {
	t.Helper()
	equal(t, proto.UnexpectedNulls(briefRaw(t, row), allowed...), []string{})
}

// Golden files row-msg-call-waiting, row-msg-call-answered, row-msg-reply and
// item-reply match the exact shapes produced by post/call and post/reply.
func TestCallGoldenShapes(t *testing.T) {
	var waitGolden, ansGolden, replyGolden proto.MsgRow
	var itemGolden proto.Item
	for name, dst := range map[string]any{
		"row-msg-call-waiting":  &waitGolden,
		"row-msg-call-answered": &ansGolden,
		"row-msg-reply":         &replyGolden,
		"item-reply":            &itemGolden,
	} {
		raw, err := os.ReadFile(filepath.Join("..", "proto", "testdata", name+".json"))
		equal(t, err, nil)
		equal(t, proto.Decode(raw, dst), nil)
	}

	s := calState(t)
	thread := calTaskID
	callPost := calCallCmd(90, clock-1000, calA, &thread, waitGolden.Text, proto.CallBody{To: calB, Depth: 1, Wait: true})
	_, callRes := calSend(t, s, disk{}, receipts{}, laptop, callPost)
	equal(t, callRes.Answer, store.Outcome{OK: true})
	s.Commit(callRes)
	waiting := callRes.Rows[0].Value().(proto.MsgRow)
	equal(t, waiting.State, waitGolden.State)
	calCheckNulls(t, waiting, "/re")

	d := disk{waiting.ID: waiting}
	replyPost := calReplyCmd(91, clock-500, calB, &thread, waiting.ID, replyGolden.Text)
	_, replyRes := calSend(t, s, d, receipts{}, laptop, replyPost)
	equal(t, replyRes.Answer, store.Outcome{OK: true})

	answered := replyRes.Rows[0].Value().(proto.MsgRow)
	replyRow := replyRes.Rows[1].Value().(proto.MsgRow)
	equal(t, answered.State.Status, ansGolden.State.Status)
	equal(t, *answered.State.Reply, replyRow.ID)
	equal(t, *answered.State.MS, *ansGolden.State.MS)
	equal(t, string(replyRow.Body), "{}")
	equal(t, replyRow.State, (*proto.CardState)(nil))
	equal(t, replyRes.Items, []home.Item{{AI: calA, Why: itemGolden.Why, Task: thread, Msg: replyRow.ID}})
	calCheckNulls(t, answered, "/re")
	calCheckNulls(t, replyRow, "/state")
}

func TestCallPostInThreadAndMainLine(t *testing.T) {
	t.Run("thread call updates task thread summary and queues one mention for callee", func(t *testing.T) {
		s := calState(t)
		thread, re := calTaskID, calPrevID
		post := calCallCmd(40, calCallTS, calA, &thread, "Which <event> & path?", proto.CallBody{To: calB, Depth: 1, Wait: true})
		post.Mentions, post.Re = calIDList(calB, ownerID), (*proto.ID)(&re)
		env, res := calSend(t, s, disk{}, receipts{}, laptop, post)
		equal(t, res.Answer, store.Outcome{OK: true})

		wantMsg := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(40), Seq: 11}, Pos: 11, Group: groupID,
			Thread: &thread, By: calA, Kind: "call", Timestamp: calCallTS, Text: "Which <event> & path?",
			Mentions: []string{calB, ownerID}, Re: &re,
			Body:  json.RawMessage(`{"to":"` + calB + `","depth":1,"wait":true}`),
			State: &proto.CardState{Status: "waiting"}, Env: &env,
		}
		wantTask := s.Tasks[calTaskID]
		wantTask.Seq = 12
		wantTask.Thread = proto.ThreadSummary{
			Count:  3,
			Rounds: calExpectedRounds(1, true),
			Last:   &proto.ThreadMessage{By: calA, Kind: "call", Text: "Which <event> & path?", Timestamp: calCallTS},
		}
		equal(t, briefRows(res), []any{wantMsg, wantTask})
		equal(t, res.Items, []home.Item{{AI: calB, Why: "mention", Task: calTaskID, Msg: id(40)}})
		calCheckNulls(t, res.Rows[0].Value())
		calCheckNulls(t, res.Rows[1].Value(), "/wait", "/stats/ended")
	})

	t.Run("main-line call at ask_depth boundary between secretary and working AI", func(t *testing.T) {
		s := calState(t)
		// Secretary calls calA at depth 2, then calA calls secretary at exact ask_depth 4 with wait false.
		first := calCallCmd(41, calCallTS, aiID, nil, "Check module.", proto.CallBody{To: calA, Depth: 2, Wait: false})
		env1, res1 := calSend(t, s, disk{}, receipts{}, laptop, first)
		equal(t, res1.Answer, store.Outcome{OK: true})
		wantFirst := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: id(41), Seq: 11}, Pos: 11, Group: groupID,
			By: aiID, Kind: "call", Timestamp: calCallTS, Text: "Check module.", Mentions: []string{},
			Body:  json.RawMessage(`{"to":"` + calA + `","depth":2,"wait":false}`),
			State: &proto.CardState{Status: "waiting"}, Env: &env1,
		}
		equal(t, briefRows(res1), []any{wantFirst})
		equal(t, res1.Items, []home.Item{{AI: calA, Why: "mention", Msg: id(41)}})
		calCheckNulls(t, res1.Rows[0].Value(), "/thread", "/re")
		s.Commit(res1)

		second := calCallCmd(42, calCallTS+100, calA, nil, "Need context.", proto.CallBody{To: aiID, Depth: 4, Wait: false})
		_, res2 := calSend(t, s, disk{id(41): wantFirst}, receipts{}, laptop, second)
		equal(t, res2.Answer, store.Outcome{OK: true})
		equal(t, res2.Items, []home.Item{{AI: aiID, Why: "mention", Msg: id(42)}})
	})

	t.Run("configured ask_depth boundary is respected", func(t *testing.T) {
		s := calState(t)
		s.Group.Limits.AskDepth = 2
		_, okRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(43, calCallTS, calA, nil, "ok", proto.CallBody{To: calB, Depth: 2, Wait: true}))
		equal(t, okRes.Answer, store.Outcome{OK: true})
	})
}

func TestCallThreadPreviewTruncation(t *testing.T) {
	s := calState(t)
	thread := calTaskID
	exact200 := strings.Repeat("界", 200)
	over200 := exact200 + "尾"

	_, callRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(44, calCallTS, calA, &thread, over200, proto.CallBody{To: calB, Depth: 1, Wait: true}))
	equal(t, callRes.Answer, store.Outcome{OK: true})
	callMsg := callRes.Rows[0].Value().(proto.MsgRow)
	callTask := callRes.Rows[1].Value().(proto.TaskRow)
	equal(t, callMsg.Text, over200)
	equal(t, callTask.Thread.Last.Text, exact200)
	s.Commit(callRes)

	_, replyRes := calSend(t, s, disk{callMsg.ID: callMsg}, receipts{}, laptop, calReplyCmd(45, calReplyTS, calB, &thread, callMsg.ID, exact200))
	equal(t, replyRes.Answer, store.Outcome{OK: true})
	replyTask := replyRes.Rows[2].Value().(proto.TaskRow)
	equal(t, replyTask.Thread.Last.Text, exact200)
}

type calPostCase struct {
	by    identity.Key
	post  func(*proto.Post[proto.CallBody])
	raw   func(string) string
	state func(*home.State)
	disk  disk
	code  string
	inMsg string
}

func TestCallPostRefusals(t *testing.T) {
	existing := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(40)}, Kind: "text"}
	missing := id(98)
	unknown := id(99)
	thread := calTaskID

	for name, tc := range map[string]calPostCase{
		"by a person": {
			by: browser, post: func(p *proto.Post[proto.CallBody]) { p.As = "" }, code: "forbidden",
		},
		"malformed payload": {
			raw: func(s string) string { return strings.Replace(s, `"depth":1`, `"depth":"one"`, 1) }, code: "bad_request",
		},
		"with main true": {
			post: func(p *proto.Post[proto.CallBody]) { p.Main = true }, code: "bad_request",
		},
		"at depth 0": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.Depth = 0 }, code: "bad_request",
		},
		"at negative depth": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.Depth = -1 }, code: "bad_request",
		},
		"calling itself": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.To = calA }, code: "bad_request",
		},
		"with adjacent duplicate mention": {
			post: func(p *proto.Post[proto.CallBody]) { p.Mentions = calIDList(calB, calB) }, code: "bad_request",
		},
		"with non-adjacent duplicate mention": {
			post: func(p *proto.Post[proto.CallBody]) { p.Mentions = calIDList(calB, ownerID, calB) }, code: "bad_request",
		},
		"in a missing task thread": {
			post: func(p *proto.Post[proto.CallBody]) { p.Thread = (*proto.ID)(&missing) }, code: "not_found",
		},
		"calling a human member": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.To = ownerID }, code: "conflict",
		},
		"calling an unknown member": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.To = unknown }, code: "conflict",
		},
		"calling an empty id": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.To = "" }, code: "conflict",
		},
		"mentioning a non-member": {
			post: func(p *proto.Post[proto.CallBody]) { p.Mentions = calIDList(unknown) }, code: "conflict",
		},
		"at depth 5 above ask_depth 4": {
			post: func(p *proto.Post[proto.CallBody]) { p.Body.Depth = 5 }, code: "limit", inMsg: "ask_depth",
		},
		"at depth 3 above custom ask_depth 2": {
			state: func(s *home.State) { s.Group.Limits.AskDepth = 2 },
			post:  func(p *proto.Post[proto.CallBody]) { p.Body.Depth = 3 },
			code:  "limit", inMsg: "2",
		},
		"with nonce already on disk": {
			disk: disk{id(40): existing}, code: "conflict",
		},
		"exceeding the 12 KiB card creation limit": {
			post: func(p *proto.Post[proto.CallBody]) { p.Text = strings.Repeat("x", 6200) }, code: "bad_request",
		},
	} {
		t.Run(name, func(t *testing.T) {
			calRunPostRefusal(t, thread, tc)
		})
	}
}

func calRunPostRefusal(t *testing.T, thread string, tc calPostCase) {
	t.Helper()
	s := calState(t)
	if tc.state != nil {
		tc.state(s)
	}
	post := calCallCmd(40, calCallTS, calA, &thread, "Question?", proto.CallBody{To: calB, Depth: 1, Wait: true})
	if tc.post != nil {
		tc.post(&post)
	}
	res := calApplyRaw(t, s, tc.disk, tc.by, post, tc.raw)
	briefRefused(t, res, tc.code)
	if tc.inMsg != "" && !strings.Contains(res.Msg, tc.inMsg) {
		t.Fatalf("refusal message %q does not mention %q", res.Msg, tc.inMsg)
	}
}

func calApplyRaw(t *testing.T, s *home.State, d disk, override identity.Key, post any, raw func(string) string) home.Result {
	t.Helper()
	by := laptop
	if override != nil {
		by = override
	}
	payload, err := json.Marshal(post)
	equal(t, err, nil)
	if raw != nil {
		payload = []byte(raw(string(payload)))
	}
	return briefApply(t, s, d, receipts{}, by, by.Sign(string(payload)))
}

func TestCallReplyInThreadAndMainLine(t *testing.T) {
	t.Run("late thread reply marks call answered with elapsed ms and wakes caller", func(t *testing.T) {
		s := calState(t)
		thread := calTaskID
		_, callRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(50, calCallTS, calA, &thread, "Need event name.", proto.CallBody{To: calB, Depth: 1, Wait: true}))
		s.Commit(callRes)
		callMsg := callRes.Rows[0].Value().(proto.MsgRow)
		baseTask := s.Tasks[calTaskID]

		reply := calReplyCmd(51, calReplyTS, calB, &thread, callMsg.ID, "Use path_changed.")
		reply.Mentions = calIDList(calA)
		env, res := calSend(t, s, disk{callMsg.ID: callMsg}, receipts{}, laptop, reply)
		equal(t, res.Answer, store.Outcome{OK: true})

		replyID, elapsed := id(51), int64(48_000)
		wantCall := callMsg
		wantCall.Seq = 13
		wantCall.State = &proto.CardState{Status: "answered", Reply: &replyID, MS: &elapsed}
		wantReply := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: replyID, Seq: 14}, Pos: 14, Group: groupID,
			Thread: &thread, By: calB, Kind: "reply", Timestamp: calReplyTS, Text: "Use path_changed.",
			Mentions: []string{calA}, Re: &callMsg.ID, Body: json.RawMessage(`{}`), Env: &env,
		}
		wantTask := baseTask
		wantTask.Seq = 15
		wantTask.Thread = proto.ThreadSummary{
			Count:  baseTask.Thread.Count + 1,
			Rounds: calExpectedRounds(baseTask.Thread.Rounds, true),
			Last:   &proto.ThreadMessage{By: calB, Kind: "reply", Text: "Use path_changed.", Timestamp: calReplyTS},
		}
		equal(t, briefRows(res), []any{wantCall, wantReply, wantTask})
		equal(t, res.Items, []home.Item{{AI: calA, Why: "reply", Task: calTaskID, Msg: replyID}})
	})

	t.Run("main-line reply at equal timestamp records ms 0 and human mention", func(t *testing.T) {
		s := calState(t)
		_, callRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(52, calReplyTS, calA, nil, "Ping?", proto.CallBody{To: calB, Depth: 1, Wait: true}))
		s.Commit(callRes)
		callMsg := callRes.Rows[0].Value().(proto.MsgRow)

		reply := calReplyCmd(53, calReplyTS, calB, nil, callMsg.ID, "Pong.")
		reply.Mentions = calIDList(ownerID)
		_, res := calSend(t, s, disk{callMsg.ID: callMsg}, receipts{}, laptop, reply)
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, len(res.Rows), 2)
		answered := res.Rows[0].Value().(proto.MsgRow)
		equal(t, []int64{answered.Seq, answered.Pos, *answered.State.MS}, []int64{12, 11, 0})
		equal(t, res.Items, []home.Item{{AI: calA, Why: "reply", Msg: id(53)}})
		calCheckNulls(t, answered, "/thread", "/re")
		calCheckNulls(t, res.Rows[1].Value(), "/thread", "/state")
	})

	t.Run("clock skew where reply timestamp precedes call clamps ms to 0", func(t *testing.T) {
		s := calState(t)
		_, callRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(54, clock, calA, nil, "Skew?", proto.CallBody{To: calB, Depth: 1, Wait: true}))
		s.Commit(callRes)
		callMsg := callRes.Rows[0].Value().(proto.MsgRow)

		_, res := calSend(t, s, disk{callMsg.ID: callMsg}, receipts{}, laptop, calReplyCmd(55, clock-300, calB, nil, callMsg.ID, "Clamped."))
		equal(t, res.Answer, store.Outcome{OK: true})
		answered := res.Rows[0].Value().(proto.MsgRow)
		equal(t, *answered.State.MS, int64(0))
	})

	t.Run("reply when caller AI left the group answers the call without queuing an item", func(t *testing.T) {
		s := calState(t)
		_, callRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(56, calCallTS, calA, nil, "Still here?", proto.CallBody{To: calB, Depth: 1, Wait: false}))
		s.Commit(callRes)
		callMsg := callRes.Rows[0].Value().(proto.MsgRow)
		delete(s.AIs, calA)

		_, res := calSend(t, s, disk{callMsg.ID: callMsg}, receipts{}, laptop, calReplyCmd(57, calReplyTS, calB, nil, callMsg.ID, "Answered."))
		equal(t, res.Answer, store.Outcome{OK: true})
		equal(t, res.Rows[0].Value().(proto.MsgRow).State.Status, "answered")
		equal(t, res.Items, ([]home.Item)(nil))
	})
}

func TestCallReceiptsAndSingleAnswer(t *testing.T) {
	s := calState(t)
	r := receipts{}
	d := disk{}
	thread := calTaskID

	callEnv, firstCall := calSend(t, s, d, r, laptop, calCallCmd(60, calCallTS, calA, &thread, "Once?", proto.CallBody{To: calB, Depth: 1, Wait: true}))
	equal(t, firstCall.Answer, store.Outcome{OK: true})
	s.Commit(firstCall)
	callMsg := firstCall.Rows[0].Value().(proto.MsgRow)
	d[callMsg.ID] = callMsg

	repeatCall := briefApply(t, s, d, r, laptop, callEnv)
	equal(t, repeatCall, home.Result{Answer: store.Outcome{OK: true}})

	replyEnv, firstReply := calSend(t, s, d, r, laptop, calReplyCmd(61, calReplyTS, calB, &thread, callMsg.ID, "Yes."))
	equal(t, firstReply.Answer, store.Outcome{OK: true})
	s.Commit(firstReply)
	for _, row := range firstReply.Rows {
		if m, ok := row.Value().(proto.MsgRow); ok {
			d[m.ID] = m
		}
	}

	repeatReply := briefApply(t, s, d, r, laptop, replyEnv)
	equal(t, repeatReply, home.Result{Answer: store.Outcome{OK: true}})

	secondEnv, secondReply := calSend(t, s, d, r, laptop, calReplyCmd(62, calReplyTS+100, calB, &thread, callMsg.ID, "Again."))
	briefRefused(t, secondReply, "conflict")
	repeatRefused := briefApply(t, s, d, r, laptop, secondEnv)
	equal(t, repeatRefused.Answer, store.Outcome{Code: "conflict"})
}

type calReplyCase struct {
	by     identity.Key
	target func(*proto.MsgRow)
	post   func(*proto.Post[proto.EmptyBody])
	raw    func(string) string
	state  func(*home.State)
	disk   disk
	code   string
}

func TestCallReplyRefusals(t *testing.T) {
	thread, other := calTaskID, calOtherT
	missing := id(97)
	unknown := id(99)
	otherMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id(71)}, Kind: "text"}

	for name, tc := range map[string]calReplyCase{
		"by a person": {
			by: browser, post: func(p *proto.Post[proto.EmptyBody]) { p.As = "" }, code: "forbidden",
		},
		"malformed payload": {
			raw: func(s string) string { return strings.Replace(s, `"body":{}`, `"body":7`, 1) }, code: "bad_request",
		},
		"with main true": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Main = true }, code: "bad_request",
		},
		"without re": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Re = nil }, code: "bad_request",
		},
		"with duplicate mention": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Mentions = calIDList(calA, calA) }, code: "bad_request",
		},
		"for a missing call": {
			post: func(p *proto.Post[proto.EmptyBody]) { re := proto.ID(missing); p.Re = &re }, code: "not_found",
		},
		"on a non-call message with waiting status and matching to": {
			target: func(m *proto.MsgRow) { m.Kind = "decision" }, code: "conflict",
		},
		"on a call without state": {
			target: func(m *proto.MsgRow) { m.State = nil }, code: "conflict",
		},
		"on an already answered call": {
			target: func(m *proto.MsgRow) { m.State = &proto.CardState{Status: "answered"} }, code: "conflict",
		},
		"by an AI other than the callee": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.As = proto.ID(calC) }, code: "conflict",
		},
		"in main line when call is in a thread": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Thread = nil }, code: "bad_request",
		},
		"in a thread when call is in main line": {
			target: func(m *proto.MsgRow) { m.Thread = nil }, code: "bad_request",
		},
		"in another thread than its call": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Thread = (*proto.ID)(&other) }, code: "bad_request",
		},
		"when the shared task no longer exists": {
			state: func(s *home.State) { delete(s.Tasks, calTaskID) }, code: "not_found",
		},
		"mentioning a non-member": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Mentions = calIDList(unknown) }, code: "conflict",
		},
		"with reply nonce already on disk": {
			disk: disk{id(71): otherMsg}, code: "conflict",
		},
		"with reply nonce equal to call id": {
			post: func(p *proto.Post[proto.EmptyBody]) { p.Nonce = proto.ID(id(70)) }, code: "conflict",
		},
	} {
		t.Run(name, func(t *testing.T) {
			calRunReplyRefusal(t, thread, tc)
		})
	}
}

func calRunReplyRefusal(t *testing.T, thread string, tc calReplyCase) {
	t.Helper()
	s := calState(t)
	_, callRes := calSend(t, s, disk{}, receipts{}, laptop, calCallCmd(70, calCallTS, calA, &thread, "Ask?", proto.CallBody{To: calB, Depth: 1, Wait: true}))
	s.Commit(callRes)
	callMsg := callRes.Rows[0].Value().(proto.MsgRow)
	if tc.target != nil {
		tc.target(&callMsg)
	}
	d := calMergeDisk(callMsg, tc.disk)
	if tc.state != nil {
		tc.state(s)
	}
	post := calReplyCmd(71, calReplyTS, calB, &thread, callMsg.ID, "Answer.")
	if tc.post != nil {
		tc.post(&post)
	}
	briefRefused(t, calApplyRaw(t, s, d, tc.by, post, tc.raw), tc.code)
}

func calMergeDisk(base proto.MsgRow, extra disk) disk {
	d := disk{base.ID: base}
	for k, v := range extra {
		d[k] = v
	}
	return d
}
