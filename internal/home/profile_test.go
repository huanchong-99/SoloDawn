package home_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestProfileValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    proto.ProfileSet
	}{
		{"model too long", proto.ProfileSet{Model: new(strings.Repeat("文", 101))}},
		{"model control", proto.ProfileSet{Model: new("bad\u007f")}},
		{"effort too long", proto.ProfileSet{Effort: new(strings.Repeat("文", 33))}},
		{"effort control", proto.ProfileSet{Effort: new("bad\t")}},
		{"invalid tier", proto.ProfileSet{Tier: new("expert")}},
		{"empty tier", proto.ProfileSet{Tier: new("")}},
		{"skills too long", proto.ProfileSet{Skills: new(strings.Repeat("文", 201))}},
		{"skills newline", proto.ProfileSet{Skills: new("docs\nGo")}},
		{"skills carriage return", proto.ProfileSet{Skills: new("docs\rGo")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := grpState(t)
			add := memAddCmd(120, id(120), "codex", "Profile AI", "personal", false)
			add.ProfileSet = tc.p
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, laptop, add), "bad_request")
			set := memSetCmd(121, grpWorkerA)
			set.ProfileSet = tc.p
			briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, browser, set), "bad_request")
		})
	}
}

func TestProfileAddAndSet(t *testing.T) {
	s := grpState(t)
	add := memAddCmd(120, id(120), "codex", "Profile AI", "personal", false)
	model, effort, skills := strings.Repeat("文", 100), strings.Repeat("文", 32), strings.Repeat("文", 200)
	add.ProfileSet = proto.ProfileSet{Model: &model, Effort: &effort, Tier: new("strong"), Skills: &skills}
	res := grpApply(t, s, memDisk{}, receipts{}, laptop, add)
	want := proto.Profile{Model: model, Effort: effort, Tier: "strong", Skills: skills}
	equal(t, res.Rows[0].Value().(proto.AIMemberRow).Profile, want)
	s.Commit(res)

	set := memSetCmd(121, id(120))
	set.ProfileSet = proto.ProfileSet{Model: new(""), Effort: new(""), Tier: new("light")}
	res = grpApply(t, s, memDisk{}, receipts{}, browser, set)
	equal(t, res.Rows[0].Value().(proto.AIMemberRow).Profile, proto.Profile{Tier: "light", Skills: skills})

	set = memSetCmd(122, id(120))
	set.ProfileSet = proto.ProfileSet{Tier: new("standard")}
	briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, guest, set), "forbidden")
	briefRefused(t, grpApply(t, s, memDisk{}, receipts{}, laptop, set), "forbidden")
}

func TestProfileAssignmentAndRowFive(t *testing.T) {
	base := machineTask().set(func(t *proto.TaskRow) {
		t.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	})
	machineRun(t, []machineCase{
		{name: "row 5 refuses while asking",
			in: base, ev: machineEvent(home.OnReport, home.ByAI, home.Event{Member: mcA, Report: "m2"}), err: home.RefusedConflict},
		{name: "row 11 clears on reassignment",
			in:  base,
			ev:  machineEvent(home.OnAssign, home.ByAdmin, home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee, t.Stuck = mcB, nil },
			fx:  home.Effects{Cards: "reassigned", Retire: []home.ItemFilter{{AI: mcA}}, Items: []home.TaskItem{{Why: "assign", AI: mcB}}}},
	})
}

func TestProfileCountSameFailures(t *testing.T) {
	base := machineTask().at("active", "review").set(func(t *proto.TaskRow) {
		t.Stuck = &proto.Stuck{Kind: "review", N: 1}
	})
	without := machineEvent(home.OnReview, home.ByAI, home.Event{Member: mcR, Report: "m1", Score: 85})
	with := without
	with.Stronger = true
	tasks, _ := mcPlay(t, base.TaskRow, without)
	equal(t, tasks[0].Stuck, &proto.Stuck{Kind: "review", N: 2})
	tasks, _ = mcPlay(t, base.TaskRow, with)
	equal(t, tasks[0].Stuck, &proto.Stuck{Kind: "review", N: 2, Ask: true})
}

func TestProfileHomeRefusesSubmission(t *testing.T) {
	s, d := revState(t)
	c := s.AIs[aiID]
	c.Tier, c.State, c.Secretary = "strong", "offline", true
	s.AIs[aiID] = c
	for n := byte(121); n <= 123; n += 2 {
		res := revApply(t, s, d, laptop, revReportPayload(n, revAssignee, "Done."))
		equal(t, res.Answer, store.Outcome{OK: true})
		s.Commit(res)
		res = revApply(t, s, d, revReviewBox, revReviewPayload(n+1, revReviewer, id(n), 10, []proto.ReviewItem{}))
		equal(t, res.Answer.OK, true)
		s.Commit(res)
	}
	stuck := s.Tasks[revTaskID].Stuck
	equal(t, stuck, &proto.Stuck{Kind: "review", N: 2, Ask: true})
	res := revApply(t, s, d, laptop, revReportPayload(127, revAssignee, "Done."))
	briefRefused(t, res, "conflict")
	equal(t, res.Msg, "ask a stronger AI before submitting again")
	equal(t, len(res.Rows), 0)
}

func TestProfileReplyClearsOnlyTheAssigneesCallToAStrongerAI(t *testing.T) {
	for _, tc := range []struct {
		name, caller, tier, thread string
		wantAsk                    bool
	}{
		{"stronger", calA, "strong", calTaskID, false},
		{"equal", calA, "standard", calTaskID, true},
		{"weaker", calA, "light", calTaskID, true},
		{"other caller", calC, "strong", calTaskID, true},
		{"other thread", calA, "strong", calOtherT, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := calState(t)
			ai := s.AIs[calB]
			ai.Tier = tc.tier
			s.AIs[calB] = ai
			task := s.Tasks[calTaskID]
			task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
			s.Tasks[calTaskID] = task
			call := calCallCmd(122, calCallTS, tc.caller, &tc.thread, "Question?", proto.CallBody{To: calB, Depth: 1, Wait: true})
			_, sent := calSend(t, s, disk{}, receipts{}, laptop, call)
			equal(t, sent.Answer, store.Outcome{OK: true})
			s.Commit(sent)
			msg := sent.Rows[0].Value().(proto.MsgRow)
			reply := calReplyCmd(123, calReplyTS, calB, &tc.thread, msg.ID, "Answer.")
			_, res := calSend(t, s, disk{msg.ID: msg}, receipts{}, laptop, reply)
			equal(t, res.Answer, store.Outcome{OK: true})
			equal(t, s.Tasks[calTaskID].Stuck, &proto.Stuck{Kind: "review", N: 2, Ask: true})
			s.Commit(res)
			equal(t, s.Tasks[calTaskID].Stuck, &proto.Stuck{Kind: "review", N: 2, Ask: tc.wantAsk})
		})
	}
}

func TestProfileEditSurvivesHomeRestart(t *testing.T) {
	tables := runTables()
	tables.Commands["member/add_ai"] = home.Commands["member/add_ai"]
	tables.Commands["member/set"] = home.Commands["member/set"]
	var seen proto.Profile
	tables.Commands["note"] = func(s *home.State, _ home.Command) (home.Result, *home.Refusal) {
		seen = s.AIs[id(120)].Profile
		return home.Result{}, nil
	}

	w, owner := runFounded(t, tables)
	host := w.attach(laptop)
	host.does(groupID, "member", fmt.Sprintf(`,"op":"add_ai","id":%q,"cli":"claude","name":"Opus","workspace":"/home/dev/website","account":"personal","secretary":false,"approvers":[],"model":"opus","tier":"strong"`, id(120)))
	owner.does(groupID, "member", fmt.Sprintf(`,"op":"set","id":%q,"tier":"light","skills":"docs"`, id(120)))
	const query = "SELECT data FROM row WHERE g=? AND r='member' AND id=? AND gone=0 ORDER BY seq DESC LIMIT 1"
	before := runQuery[string](t, w.db, query, groupID, id(120))
	w.loop.Close()

	restarted := runStart(t, w.db, tables)
	restarted.nonce.Store(w.nonce.Load())
	restarted.attach(browser).does(groupID, "note", "")
	equal(t, seen, proto.Profile{Model: "opus", Tier: "light", Skills: "docs"})
	equal(t, runQuery[string](t, w.db, query, groupID, id(120)), before)
}
