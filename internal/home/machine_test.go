package home_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The cast of every case: the task's assignee and reviewer, another working
// AI, and the secretary.
const (
	mcA   = "ai-assignee"
	mcR   = "ai-reviewer"
	mcB   = "ai-other"
	mcSec = "ai-secretary"
	mcNow = int64(100)
)

var (
	mcApproval1 = home.PendingCard{Kind: "approval", Msg: "A1", Who: []string{"u1"}, Since: 11}
	mcApproval2 = home.PendingCard{Kind: "approval", Msg: "A2", Who: []string{"u1", "u2"}, Since: 12}
	mcDecision  = home.PendingCard{Kind: "decision", Msg: "D1", Who: []string{"u3"}, Since: 10}
)

// machineRow builds the task a case starts from.
type machineRow struct{ proto.TaskRow }

// machineTask is an active task at step work whose latest report is m1, with
// one failure, one failed review and two rounds.
func machineTask() machineRow {
	return machineRow{proto.TaskRow{
		Origin: "brief", State: "active", Step: "work", Report: "m1",
		Assignee: mcA, Reviewer: mcR, Depends: []string{},
		Review:   mcReview(1),
		Failures: 1,
		Thread:   proto.ThreadSummary{Count: 4, Rounds: 2},
	}}
}

// mcReview is the one review machineTask has had, of an earlier report, with
// this many failed reviews counted.
func mcReview(failed int) *proto.Review {
	return &proto.Review{Round: 1, Failed: failed, Score: 60, By: mcR, Report: "m0", At: 3}
}

func (r machineRow) at(state, step string) machineRow {
	r.State, r.Step = state, step
	return r
}

func (r machineRow) wait(kind string) machineRow {
	r.Wait = mcWait(kind, 1)
	return r
}

// keep makes the wait of a paused task keep the wait it replaced.
func (r machineRow) keep(kind string) machineRow {
	wait := *r.Wait
	wait.Kept = kind
	r.Wait = &wait
	return r
}

func (r machineRow) reviewer(id string) machineRow {
	r.Reviewer = id
	return r
}

func (r machineRow) set(change func(*proto.TaskRow)) machineRow {
	change(&r.TaskRow)
	return r
}

func mcWait(kind string, since int64) *proto.TaskWait {
	return &proto.TaskWait{Kind: kind, Since: since}
}

// mcKept is the wait of a pause at mcNow that keeps the wait it replaced.
func mcKept(kind, kept string) *proto.TaskWait {
	return &proto.TaskWait{Kind: kind, Since: mcNow, Kept: kept}
}

func mcCardWait(c home.PendingCard) *proto.TaskWait {
	return &proto.TaskWait{Kind: c.Kind, Msg: c.Msg, Who: c.Who, Since: c.Since}
}

// machineEvent completes an event with the clock and the group's limits.
func machineEvent(kind home.EventKind, by home.Sender, e home.Event) home.Event {
	e.Kind, e.By, e.Now = kind, by, mcNow
	e.Limits = proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5}
	return e
}

type machineCase struct {
	name string
	in   machineRow
	ev   home.Event
	out  func(*proto.TaskRow) // what the event changes in the row; it never writes through a pointer of the row
	fx   home.Effects
	err  error
}

func mcJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mcSame(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s\n got %s\nwant %s", what, mcJSON(t, got), mcJSON(t, want))
	}
}

func machineRun(t *testing.T, cases []machineCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, c.check)
	}
}

// check runs the case, and sees that Step changed neither its task nor its event.
func (c machineCase) check(t *testing.T) {
	task, event := mcJSON(t, c.in.TaskRow), mcJSON(t, c.ev)
	want := c.in.TaskRow
	if c.out != nil {
		c.out(&want)
	}
	got, fx, err := home.Step(c.in.TaskRow, c.ev)
	mcSame(t, "error", err, c.err)
	mcSame(t, "task", got, want)
	mcSame(t, "effects", fx, c.fx)
	mcSame(t, "the task passed in", mcJSON(t, c.in.TaskRow), task)
	mcSame(t, "the event passed in", mcJSON(t, c.ev), event)
}

// mcPlay applies the events in turn and returns the task and the effects after each.
func mcPlay(t *testing.T, task proto.TaskRow, events ...home.Event) ([]proto.TaskRow, []home.Effects) {
	t.Helper()
	var tasks []proto.TaskRow
	var effects []home.Effects
	for i, ev := range events {
		next, fx, err := home.Step(task, ev)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		task = next
		tasks, effects = append(tasks, next), append(effects, fx)
	}
	return tasks, effects
}

// mcState is the task's state and what it waits for.
func mcState(task proto.TaskRow) string {
	if task.Wait == nil {
		return task.State
	}
	return task.State + " " + task.Wait.Kind
}

func mcItems(items ...home.TaskItem) home.Effects { return home.Effects{Items: items} }

func mcNotice(code string) home.TaskItem { return home.TaskItem{Why: "notice", Code: code} }

func mcStopped(reason string) home.Effects {
	return home.Effects{System: "task_failed", Reason: reason, Items: []home.TaskItem{mcNotice("task_failed")}}
}

// Rows 1 and 2: a task is created.
func TestMachineCreates(t *testing.T) {
	on := machineEvent
	fresh := machineTask().at("", "")
	direct := fresh.set(func(t *proto.TaskRow) { t.Origin, t.Assignee, t.Reviewer, t.Foundation = "direct", "", "stale", true })
	started := func(t *proto.TaskRow) {
		t.State, t.Step, t.Assignee, t.Reviewer, t.Foundation, t.Stats.Started = "active", "work", mcB, "", false, mcNow
	}
	waiting := func(kind string) func(*proto.TaskRow) {
		return func(t *proto.TaskRow) { t.State, t.Step, t.Wait = "queued", "work", mcWait(kind, mcNow) }
	}
	machineRun(t, []machineCase{
		{name: "row 1: an entry whose dependencies are done and whose folder is free starts",
			in: fresh, ev: on(home.OnConfirm, home.ByPerson, home.Event{}),
			out: func(t *proto.TaskRow) { t.State, t.Step, t.Stats.Started = "active", "work", mcNow },
			fx:  mcItems(home.TaskItem{Why: "assign", AI: mcA})},
		{name: "row 1: an entry with a dependency that is not done is queued",
			in: fresh, ev: on(home.OnConfirm, home.ByAdmin, home.Event{Blocked: true}), out: waiting("depends")},
		{name: "row 1: an entry whose folder is busy is queued",
			in: fresh, ev: on(home.OnConfirm, home.ByAdmin, home.Event{Busy: true}), out: waiting("busy")},
		{name: "row 1: the dependency is named before the folder",
			in: fresh, ev: on(home.OnConfirm, home.ByAdmin, home.Event{Blocked: true, Busy: true}), out: waiting("depends")},
		{name: "row 1: a task that exists is not created again",
			in: machineTask(), ev: on(home.OnConfirm, home.ByAdmin, home.Event{}), err: home.RefusedConflict},
		{name: "row 1: an AI confirms nothing",
			in: fresh, ev: on(home.OnConfirm, home.ByAI, home.Event{}), err: home.RefusedForbidden},
		{name: "roles: assignee and reviewer of a new task differ",
			in: fresh.reviewer(mcA), ev: on(home.OnConfirm, home.ByAdmin, home.Event{}), err: home.RefusedConflict},

		{name: "row 2: a direct task starts with its AI as assignee and no reviewer",
			in: direct, ev: on(home.OnStart, home.ByAI, home.Event{Member: mcB}),
			out: started,
			fx:  mcItems(home.TaskItem{Why: "assign", AI: mcB}, mcNotice("task_started"))},
		{name: "row 2: a direct task of an AI whose folder is busy is queued, and the secretary told",
			in: direct, ev: on(home.OnStart, home.ByAI, home.Event{Member: mcB, Busy: true}),
			out: func(t *proto.TaskRow) {
				t.State, t.Step, t.Assignee, t.Reviewer, t.Foundation, t.Wait = "queued", "work", mcB, "", false, mcWait("busy", mcNow)
			},
			fx: mcItems(mcNotice("task_started"))},
		{name: "row 2: the secretary starts no task",
			in: direct, ev: on(home.OnStart, home.BySecretary, home.Event{Member: mcSec}), err: home.RefusedForbidden},
		{name: "row 2: a task that exists is not started again",
			in: machineTask(), ev: on(home.OnStart, home.ByAI, home.Event{Member: mcB}), err: home.RefusedConflict},

		{name: "an event the table does not have is refused",
			in: machineTask(), ev: on("unknown", home.ByAdmin, home.Event{}), err: home.RefusedForbidden},
	})
}

// Row 5: the assignee's report is the submission.
func TestMachineSubmission(t *testing.T) {
	on := machineEvent
	by := func(member string, e home.Event) home.Event {
		e.Member, e.Report = member, "m2"
		return on(home.OnReport, home.ByAI, e)
	}
	base := machineTask()
	paused := base.at("paused", "work").wait("manual")
	review := home.TaskItem{Why: "review", AI: mcR, Report: "m2"}
	oldReview := []home.ItemFilter{{Why: "review"}}
	submitted := func(t *proto.TaskRow) { t.Report, t.Step, t.Failures = "m2", "review", 0 }
	machineRun(t, []machineCase{
		{name: "the report goes to the reviewer and the failures start again",
			in: base, ev: by(mcA, home.Event{}), out: submitted,
			fx: home.Effects{Retire: oldReview, Items: []home.TaskItem{review}}},
		{name: "a new report at step review takes the place of the earlier one",
			in: base.at("active", "review"), ev: by(mcA, home.Event{}), out: submitted,
			fx: home.Effects{Retire: oldReview, Items: []home.TaskItem{review}}},
		{name: "a card wait is left as it is",
			in: base.set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval2) }), ev: by(mcA, home.Event{}), out: submitted,
			fx: home.Effects{Retire: oldReview, Items: []home.TaskItem{review}}},
		{name: "without a reviewer the task waits for one and the secretary is told",
			in: base.reviewer("").set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval1) }), ev: by(mcA, home.Event{}),
			out: func(t *proto.TaskRow) {
				t.Report, t.Step, t.Failures, t.Wait = "m2", "review", 0, mcWait("reviewer", mcNow)
			},
			fx: home.Effects{Retire: oldReview, Items: []home.TaskItem{mcNotice("no_reviewer")}}},
		{name: "while paused the wait stays and no work item is queued",
			in: paused, ev: by(mcA, home.Event{}), out: submitted,
			fx: home.Effects{Retire: oldReview}},
		{name: "while paused without a reviewer the secretary is told and the wait stays",
			in: paused.reviewer("").keep("reviewer"), ev: by(mcA, home.Event{}), out: submitted,
			fx: home.Effects{Retire: oldReview, Items: []home.TaskItem{mcNotice("no_reviewer")}}},
		{name: "refused while the assignee must ask a stronger AI",
			in: base.set(func(t *proto.TaskRow) { t.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true} }), ev: by(mcA, home.Event{}),
			err: home.RefusedConflict},
		{name: "refused from the reviewer",
			in: base, ev: by(mcR, home.Event{}), err: home.RefusedForbidden},
		{name: "refused from another AI",
			in: base, ev: by(mcB, home.Event{}), err: home.RefusedForbidden},
		{name: "refused for a queued task",
			in: base.at("queued", "work").wait("busy"), ev: by(mcA, home.Event{}), err: home.RefusedConflict},
		{name: "refused for a failed task",
			in: base.at("failed", "work"), ev: by(mcA, home.Event{}), err: home.RefusedConflict},
		{name: "refused for a done task",
			in: base.at("done", "review"), ev: by(mcA, home.Event{}), err: home.RefusedConflict},
	})
}

// Row 7, with row 14 inside it.
func TestMachineReview(t *testing.T) {
	on := machineEvent
	by := func(member string, e home.Event) home.Event {
		e.Member = member
		return on(home.OnReview, home.ByAI, e)
	}
	base := machineTask().at("active", "review")
	scored := func(round, failed, score int, ok bool) *proto.Review {
		return &proto.Review{Round: round, Failed: failed, Score: score, OK: ok, By: mcR, Report: "m1", At: mcNow}
	}
	never := base.set(func(t *proto.TaskRow) { t.Review = nil })
	failedBefore := func(n int) func(*proto.TaskRow) {
		return func(t *proto.TaskRow) {
			t.Review = &proto.Review{Round: 7, Failed: n, Score: 10, By: "old", Report: "m0", At: 7}
		}
	}
	ended := mcNow
	done := home.Effects{Cards: "expired", System: "task_done", Items: []home.TaskItem{mcNotice("task_done")}}
	reviewed := mcItems(home.TaskItem{Why: "reviewed", AI: mcA})
	machineRun(t, []machineCase{
		{name: "row 7: a passed first review ends the task",
			in: never, ev: by(mcR, home.Event{Report: "m1", OK: true, Score: 93}),
			out: func(t *proto.TaskRow) { t.Review, t.State, t.Stats.Ended = scored(1, 0, 93, true), "done", &ended },
			fx:  done},
		{name: "row 7: a passed review ends a card wait and an escalation",
			in: base.wait("approval").set(func(t *proto.TaskRow) { t.Stuck = &proto.Stuck{Kind: "review", N: 1} }),
			ev: by(mcR, home.Event{Report: "m1", OK: true, Score: 93}),
			out: func(t *proto.TaskRow) {
				t.Review, t.State, t.Wait, t.Stats.Ended, t.Stuck = scored(2, 1, 93, true), "done", nil, &ended, nil
			},
			fx: done},
		{name: "row 7: a first review that fails is the first failed one",
			in: never, ev: by(mcR, home.Event{Report: "m1", Score: 86}),
			out: func(t *proto.TaskRow) {
				t.Review, t.Step = scored(1, 1, 86, false), "work"
				t.Stuck = &proto.Stuck{Kind: "review", N: 1}
			},
			fx: reviewed},
		{name: "row 7: a second failure in a row asks for a stronger AI when the group has one",
			in: base.set(func(t *proto.TaskRow) { t.Stuck = &proto.Stuck{Kind: "review", N: 1} }),
			ev: by(mcR, home.Event{Report: "m1", Score: 86, Stronger: true}),
			out: func(t *proto.TaskRow) {
				t.Review, t.Step = scored(2, 2, 86, false), "work"
				t.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
			},
			fx: reviewed},
		{name: "row 7: the fifth failed review still sends the work back",
			in: base.set(failedBefore(4)), ev: by(mcR, home.Event{Report: "m1", Score: 86}),
			out: func(t *proto.TaskRow) {
				t.Review, t.Step = scored(8, 5, 86, false), "work"
				t.Stuck = &proto.Stuck{Kind: "review", N: 1}
			},
			fx: reviewed},
		{name: "row 14: the sixth failed review stops the task",
			in: base.wait("decision").set(failedBefore(5)), ev: by(mcR, home.Event{Report: "m1", Score: 86}),
			out: func(t *proto.TaskRow) {
				t.Review, t.State, t.Wait = scored(8, 6, 86, false), "failed", nil
				t.Stuck = &proto.Stuck{Kind: "review", N: 1}
			},
			fx: mcStopped("review_rounds")},
		{name: "row 7: refused from the task's own assignee",
			in: base, ev: by(mcA, home.Event{Report: "m1", OK: true}), err: home.RefusedForbidden},
		{name: "row 7: refused from another AI",
			in: base, ev: by(mcB, home.Event{Report: "m1", OK: true}), err: home.RefusedForbidden},
		{name: "row 7: nobody is the reviewer of a task without one",
			in: base.reviewer(""), ev: by("", home.Event{Report: "m1", OK: true}), err: home.RefusedForbidden},
		{name: "row 7: refused for another report",
			in: base, ev: by(mcR, home.Event{Report: "m0", OK: true}), err: home.RefusedConflict},
		{name: "row 7: refused while the task waits for a reviewer",
			in: base.wait("reviewer"), ev: by(mcR, home.Event{Report: "m1", OK: true}), err: home.RefusedConflict},
		{name: "row 7: refused outside step review",
			in: machineTask(), ev: by(mcR, home.Event{Report: "m1", OK: true}), err: home.RefusedConflict},
		{name: "row 7: answered paused for a paused task",
			in: base.at("paused", "review").wait("manual"), ev: by(mcR, home.Event{Report: "m1", OK: true}), err: home.RefusedPaused},
		{name: "row 7: a paused task that kept a reviewer wait answers conflict",
			in: base.at("paused", "review").wait("manual").keep("reviewer"), ev: by(mcR, home.Event{Report: "m1", OK: true}), err: home.RefusedConflict},
		{name: "row 7: another report is a conflict while paused too",
			in: base.at("paused", "review").wait("manual"), ev: by(mcR, home.Event{Report: "m0", OK: true}), err: home.RefusedConflict},
		{name: "row 7: refused for a failed task",
			in: base.at("failed", "review"), ev: by(mcR, home.Event{Report: "m1", OK: true}), err: home.RefusedConflict},
		{name: "row 7: refused for a done task",
			in: base.at("done", "review"), ev: by(mcR, home.Event{Report: "m1", OK: true}), err: home.RefusedConflict},
	})
}

// Section 2, "Starting a queued task", and row 10 with row 14 inside it.
func TestMachineQueuedAndFailures(t *testing.T) {
	on := machineEvent
	host := func(kind home.EventKind, e home.Event) home.Event { return on(kind, home.ByHost, e) }
	look := func(e home.Event) home.Event { return on(home.OnReleased, home.ByHome, e) }
	base := machineTask()
	queued := base.at("queued", "work").wait("depends")
	behind := base.at("queued", "work").wait("busy")
	machineRun(t, []machineCase{
		{name: "a task whose dependencies are done and whose folder is free starts",
			in: queued, ev: look(home.Event{}),
			out: func(t *proto.TaskRow) { t.State, t.Wait, t.Stats.Started = "active", nil, mcNow },
			fx:  mcItems(home.TaskItem{Why: "assign", AI: mcA})},
		{name: "a task behind another starts when the folder is free",
			in: behind, ev: look(home.Event{}),
			out: func(t *proto.TaskRow) { t.State, t.Wait, t.Stats.Started = "active", nil, mcNow },
			fx:  mcItems(home.TaskItem{Why: "assign", AI: mcA})},
		{name: "a task that starts waits for a decision asked while it was queued",
			in: queued, ev: look(home.Event{Pending: []home.PendingCard{mcDecision}}),
			out: func(t *proto.TaskRow) { t.State, t.Wait, t.Stats.Started = "active", mcCardWait(mcDecision), mcNow },
			fx:  mcItems(home.TaskItem{Why: "assign", AI: mcA})},
		{name: "a task with a dependency that is not done stays as it is",
			in: queued, ev: look(home.Event{Blocked: true, Busy: true})},
		{name: "a task whose dependencies are done waits for its folder from then on",
			in: queued, ev: look(home.Event{Busy: true}),
			out: func(t *proto.TaskRow) { t.Wait = mcWait("busy", mcNow) }},
		{name: "a task whose folder is still busy stays as it is",
			in: behind, ev: look(home.Event{Busy: true})},
		{name: "only a queued task is looked at",
			in: base, ev: look(home.Event{}), err: home.RefusedConflict},
		{name: "a host starts nothing",
			in: queued, ev: host(home.OnReleased, home.Event{}), err: home.RefusedForbidden},

		{name: "row 10: a crash below the limit is counted and the secretary told",
			in: base, ev: host(home.OnFail, home.Event{Reason: "crash"}),
			out: func(t *proto.TaskRow) { t.Failures = 2 },
			fx:  mcItems(mcNotice("crash"))},
		{name: "row 10: a timeout is told as a timeout",
			in: base.at("active", "review"), ev: host(home.OnFail, home.Event{Reason: "timeout"}),
			out: func(t *proto.TaskRow) { t.Failures = 2 },
			fx:  mcItems(mcNotice("timeout"))},
		{name: "row 14: the third consecutive failure stops the task",
			in: base.wait("approval").set(func(t *proto.TaskRow) { t.Failures = 2 }), ev: host(home.OnFail, home.Event{Reason: "crash"}),
			out: func(t *proto.TaskRow) { t.Failures, t.State, t.Wait = 3, "failed", nil },
			fx:  mcStopped("failures")},
		{name: "row 10: answered paused for a paused task",
			in: base.at("paused", "work").wait("manual"), ev: host(home.OnFail, home.Event{Reason: "crash"}), err: home.RefusedPaused},
		{name: "row 10: refused for a queued task",
			in: queued, ev: host(home.OnFail, home.Event{Reason: "crash"}), err: home.RefusedConflict},
		{name: "row 10: refused for a failed task",
			in: base.at("failed", "work"), ev: host(home.OnFail, home.Event{Reason: "crash"}), err: home.RefusedConflict},
		{name: "row 10: only the host reports it",
			in: base, ev: on(home.OnFail, home.BySecretary, home.Event{Reason: "crash"}), err: home.RefusedForbidden},
	})
}

// Row 11.
func TestMachineAssign(t *testing.T) {
	on := machineEvent
	sec := func(e home.Event) home.Event { return on(home.OnAssign, home.BySecretary, e) }
	admin := func(e home.Event) home.Event { return on(home.OnAssign, home.ByAdmin, e) }
	base := machineTask()
	reviewing := base.at("active", "review")
	failed := base.at("failed", "work")
	moved := home.Effects{Cards: "reassigned", Retire: []home.ItemFilter{{AI: mcA}}}
	movedTo := func(items ...home.TaskItem) home.Effects {
		fx := moved
		fx.Items = items
		return fx
	}
	oldReviewer := []home.ItemFilter{{AI: mcR, Why: "review"}}
	assign := func(ai string) home.TaskItem { return home.TaskItem{Why: "assign", AI: ai} }
	machineRun(t, []machineCase{
		{name: "another assignee: its cards are reassigned, its items retired, the new one gets the task",
			in: base, ev: sec(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  movedTo(assign(mcB))},
		{name: "another assignee: an approval wait ends with its card, a pending decision takes its place",
			in:  base.set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval1) }),
			ev:  admin(home.Event{Assignee: mcB, Pending: []home.PendingCard{mcApproval1, mcDecision, mcApproval2}}),
			out: func(t *proto.TaskRow) { t.Assignee, t.Wait = mcB, mcCardWait(mcDecision) },
			fx:  movedTo(assign(mcB))},
		{name: "another assignee: an approval wait ends with nothing else pending",
			in: base.set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval1) }), ev: sec(home.Event{Assignee: mcB, Pending: []home.PendingCard{mcApproval1}}),
			out: func(t *proto.TaskRow) { t.Assignee, t.Wait = mcB, nil },
			fx:  movedTo(assign(mcB))},
		{name: "another assignee: a decision wait stays",
			in: base.set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcDecision) }), ev: sec(home.Event{Assignee: mcB, Pending: []home.PendingCard{mcDecision}}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  movedTo(assign(mcB))},
		{name: "the same assignee named again changes nothing",
			in: base, ev: sec(home.Event{Assignee: mcA})},
		{name: "another reviewer at step review gets the report, the old one's item is retired",
			in: reviewing, ev: sec(home.Event{Reviewer: mcB}),
			out: func(t *proto.TaskRow) { t.Reviewer = mcB },
			fx:  home.Effects{Retire: oldReviewer, Items: []home.TaskItem{{Why: "review", AI: mcB, Report: "m1"}}}},
		{name: "a first reviewer at step review ends the reviewer wait",
			in: reviewing.reviewer("").wait("reviewer"), ev: sec(home.Event{Reviewer: mcB, Pending: []home.PendingCard{mcApproval1}}),
			out: func(t *proto.TaskRow) { t.Reviewer, t.Wait = mcB, mcCardWait(mcApproval1) },
			fx:  home.Effects{Retire: []home.ItemFilter{{Why: "review"}}, Items: []home.TaskItem{{Why: "review", AI: mcB, Report: "m1"}}}},
		{name: "another reviewer before step review gets nothing yet",
			in: base, ev: sec(home.Event{Reviewer: mcB}),
			out: func(t *proto.TaskRow) { t.Reviewer = mcB },
			fx:  home.Effects{Retire: oldReviewer}},
		{name: "the same reviewer named again changes nothing",
			in: reviewing, ev: sec(home.Event{Reviewer: mcR})},
		{name: "another assignee at step review leaves the reviewer wait",
			in: reviewing.reviewer("").wait("reviewer"), ev: sec(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  movedTo(assign(mcB))},
		{name: "both named at once",
			in: reviewing, ev: sec(home.Event{Assignee: mcB, Reviewer: "ai-third"}),
			out: func(t *proto.TaskRow) { t.Assignee, t.Reviewer = mcB, "ai-third" },
			fx: home.Effects{Cards: "reassigned", Retire: []home.ItemFilter{{AI: mcA}, {AI: mcR, Why: "review"}},
				Items: []home.TaskItem{{Why: "review", AI: "ai-third", Report: "m1"}, assign(mcB)}}},
		{name: "a queued task is reassigned without a work item",
			in: base.at("queued", "work").wait("depends"), ev: sec(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  moved},
		{name: "a queued task goes to an assignee whose folder is busy, and waits there",
			in: base.at("queued", "work").wait("busy"), ev: sec(home.Event{Assignee: mcB, Busy: true}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  moved},
		{name: "a paused task is reassigned without a work item and stays paused",
			in: base.at("paused", "work").wait("manual"), ev: sec(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  moved},
		{name: "a paused task no longer keeps a reviewer wait once it has a reviewer",
			in: reviewing.at("paused", "review").reviewer("").wait("manual").keep("reviewer"), ev: sec(home.Event{Reviewer: mcB}),
			out: func(t *proto.TaskRow) { t.Reviewer, t.Wait = mcB, mcWait("manual", 1) },
			fx:  home.Effects{Retire: []home.ItemFilter{{Why: "review"}}}},
		{name: "a paused task keeps its reviewer wait when only the assignee changes",
			in: reviewing.at("paused", "review").reviewer("").wait("manual").keep("reviewer"), ev: sec(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee = mcB },
			fx:  moved},

		{name: "from failed: an admin gives it to another AI, active with the counters at zero",
			in: failed, ev: admin(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) { t.Assignee, t.State, t.Failures, t.Review = mcB, "active", 0, mcReview(0) },
			fx:  movedTo(assign(mcB))},
		{name: "from failed: the same assignee gets the task again",
			in: failed, ev: admin(home.Event{Assignee: mcA}),
			out: func(t *proto.TaskRow) { t.State, t.Failures, t.Review = "active", 0, mcReview(0) },
			fx:  mcItems(assign(mcA))},
		{name: "from failed: the task waits for a decision that is still pending",
			in: failed, ev: admin(home.Event{Assignee: mcA, Pending: []home.PendingCard{mcDecision}}),
			out: func(t *proto.TaskRow) {
				t.State, t.Failures, t.Review, t.Wait = "active", 0, mcReview(0), mcCardWait(mcDecision)
			},
			fx: mcItems(assign(mcA))},
		{name: "from failed at step review: another reviewer gets the report once",
			in: failed.at("failed", "review"), ev: admin(home.Event{Reviewer: mcB}),
			out: func(t *proto.TaskRow) { t.Reviewer, t.State, t.Failures, t.Review = mcB, "active", 0, mcReview(0) },
			fx: home.Effects{Retire: oldReviewer,
				Items: []home.TaskItem{{Why: "review", AI: mcB, Report: "m1", IfNone: true}, assign(mcA)}}},
		{name: "from failed at step review without a reviewer: the task waits for one",
			in: failed.at("failed", "review").reviewer(""), ev: admin(home.Event{Assignee: mcB}),
			out: func(t *proto.TaskRow) {
				t.Assignee, t.State, t.Failures, t.Review, t.Wait = mcB, "active", 0, mcReview(0), mcWait("reviewer", mcNow)
			},
			fx: movedTo(assign(mcB))},

		{name: "refused: an active task for an assignee whose folder is busy",
			in: base, ev: sec(home.Event{Assignee: mcB, Busy: true}), err: home.RefusedConflict},
		{name: "refused: a paused task for an assignee whose folder is busy",
			in: base.at("paused", "work").wait("manual"), ev: admin(home.Event{Assignee: mcB, Busy: true}), err: home.RefusedConflict},
		{name: "refused: a failed task for an assignee whose folder is busy, whoever asks",
			in: failed, ev: admin(home.Event{Assignee: mcB, Busy: true}), err: home.RefusedConflict},
		{name: "refused: the same for the secretary, which a free folder would not help",
			in: failed, ev: sec(home.Event{Assignee: mcB, Busy: true}), err: home.RefusedConflict},
		{name: "refused: the secretary does not reassign a failed task",
			in: failed, ev: sec(home.Event{Assignee: mcB}), err: home.RefusedForbidden},
		{name: "refused for a done task",
			in: base.at("done", "review"), ev: admin(home.Event{Assignee: mcB}), err: home.RefusedConflict},
		{name: "refused for a cancelled task",
			in: base.at("cancelled", "work"), ev: admin(home.Event{Assignee: mcB}), err: home.RefusedConflict},
		{name: "refused from a host",
			in: base, ev: on(home.OnAssign, home.ByHost, home.Event{Assignee: mcB}), err: home.RefusedForbidden},
		{name: "refused from a person who is no admin",
			in: base, ev: on(home.OnAssign, home.ByPerson, home.Event{Assignee: mcB}), err: home.RefusedForbidden},
		{name: "roles: the reviewer is not made assignee",
			in: base, ev: sec(home.Event{Assignee: mcR}), err: home.RefusedConflict},
		{name: "roles: the assignee is not made reviewer",
			in: base, ev: sec(home.Event{Reviewer: mcA}), err: home.RefusedConflict},
	})
}

// Rows 12, 13, 13b and 16, and the waits a pause keeps.
func TestMachinePauseAndResume(t *testing.T) {
	on := machineEvent
	sec := func(kind home.EventKind) home.Event { return on(kind, home.BySecretary, home.Event{Member: mcSec}) }
	admin := func(kind home.EventKind) home.Event { return on(kind, home.ByAdmin, home.Event{Member: "person"}) }
	host := func(kind home.EventKind) home.Event { return on(kind, home.ByHost, home.Event{Member: "person"}) }
	base := machineTask()
	reviewing := base.at("active", "review")
	paused := base.at("paused", "work").wait("manual")
	manual := func(by, kept string) *proto.TaskWait {
		return &proto.TaskWait{Kind: "manual", Since: mcNow, Kept: kept, By: by}
	}
	pre := mcWait("preempted", mcNow)
	resume := home.TaskItem{Why: "resume", AI: mcA}
	review := home.TaskItem{Why: "review", AI: mcR, Report: "m1", IfNone: true}
	machineRun(t, []machineCase{
		{name: "row 12: the secretary pauses a task, and the wait says so",
			in: base, ev: sec(home.OnPause),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "paused", manual(mcSec, "") }},
		{name: "row 12: a person's pause names the person and keeps a reviewer wait",
			in: reviewing.reviewer("").wait("reviewer"), ev: admin(home.OnPause),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "paused", manual("person", "reviewer") }},
		{name: "row 12: the pause keeps no card wait",
			in: base.wait("approval"), ev: sec(home.OnPause),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "paused", manual(mcSec, "") }},
		{name: "row 12: answered paused for a paused task",
			in: paused, ev: sec(home.OnPause), err: home.RefusedPaused},
		{name: "row 12: refused for a queued task",
			in: base.at("queued", "work").wait("depends"), ev: sec(home.OnPause), err: home.RefusedConflict},
		{name: "row 12: refused for a failed task",
			in: base.at("failed", "work"), ev: sec(home.OnPause), err: home.RefusedConflict},
		{name: "row 12: refused from a person who is no admin, whatever the task",
			in: paused, ev: on(home.OnPause, home.ByPerson, home.Event{}), err: home.RefusedForbidden},
		{name: "row 12: refused from a host",
			in: base, ev: host(home.OnPause), err: home.RefusedForbidden},

		{name: "row 16: the host pre-empted the task, and nobody chose that",
			in: base, ev: host(home.OnPreempted),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "paused", pre },
			fx:  home.Effects{System: "preempted"}},
		{name: "row 16: the pause keeps a reviewer wait",
			in: reviewing.reviewer("").wait("reviewer"), ev: host(home.OnPreempted),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "paused", mcKept("preempted", "reviewer") },
			fx:  home.Effects{System: "preempted"}},
		{name: "row 16: answered paused for a paused task",
			in: paused, ev: host(home.OnPreempted), err: home.RefusedPaused},
		{name: "row 16: only the host reports it",
			in: base, ev: sec(home.OnPreempted), err: home.RefusedForbidden},

		{name: "row 13: the secretary resumes a paused task",
			in: paused, ev: sec(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", nil },
			fx:  mcItems(resume)},
		{name: "row 13: a kept reviewer wait is restored and no review is queued",
			in: paused.at("paused", "review").reviewer("").keep("reviewer"), ev: sec(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", mcWait("reviewer", mcNow) },
			fx:  mcItems(resume)},
		{name: "row 13: at step review the reviewer gets the report unless it is queued",
			in: paused.at("paused", "review"), ev: sec(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", nil },
			fx:  mcItems(review, resume)},
		{name: "row 13: at step review without a reviewer the task waits for one again",
			in: paused.at("paused", "review").reviewer(""), ev: sec(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", mcWait("reviewer", mcNow) },
			fx:  mcItems(resume)},
		{name: "row 13: the wait is the oldest pending approval",
			in: paused, ev: on(home.OnResume, home.BySecretary, home.Event{Pending: []home.PendingCard{mcApproval1, mcApproval2}}),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", mcCardWait(mcApproval1) },
			fx:  mcItems(resume)},
		{name: "row 13: the secretary resumes a pre-empted task whose host never did",
			in: paused.wait("preempted"), ev: sec(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", nil },
			fx:  mcItems(resume)},
		{name: "row 13: an admin ends the round limit, and the rounds start again",
			in: paused.wait("rounds"), ev: admin(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait, t.Thread.Rounds = "active", nil, 0 },
			fx:  mcItems(resume)},
		{name: "row 13: an admin resumes a failed task with the counters at zero",
			in: base.at("failed", "work").set(func(t *proto.TaskRow) { t.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true} }), ev: admin(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Failures, t.Review, t.Stuck = "active", 0, mcReview(0), nil },
			fx:  mcItems(resume)},
		{name: "row 13: a failed task that was never reviewed has only its failures to forget",
			in: base.at("failed", "work").set(func(t *proto.TaskRow) { t.Review = nil }), ev: admin(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Failures = "active", 0 },
			fx:  mcItems(resume)},
		{name: "row 13: refused: the secretary does not end the round limit",
			in: paused.wait("rounds"), ev: sec(home.OnResume), err: home.RefusedForbidden},
		{name: "row 13: refused: the secretary does not resume a failed task",
			in: base.at("failed", "work"), ev: sec(home.OnResume), err: home.RefusedForbidden},
		{name: "row 13: refused for an active task",
			in: base, ev: sec(home.OnResume), err: home.RefusedConflict},
		{name: "row 13: refused for a queued task",
			in: base.at("queued", "work").wait("depends"), ev: admin(home.OnResume), err: home.RefusedConflict},
		{name: "row 13: refused for a done task",
			in: base.at("done", "review"), ev: admin(home.OnResume), err: home.RefusedConflict},
		{name: "row 13: refused from a working AI",
			in: paused, ev: on(home.OnResume, home.ByAI, home.Event{Member: mcA}), err: home.RefusedForbidden},

		{name: "row 13b: the host resumes its pre-empted task and no item is queued",
			in: paused.wait("preempted"), ev: host(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", nil }},
		{name: "row 13b: at step review the reviewer gets the report unless it is queued",
			in: paused.at("paused", "review").wait("preempted"), ev: host(home.OnResume),
			out: func(t *proto.TaskRow) { t.State, t.Wait = "active", nil },
			fx:  mcItems(review)},
		{name: "row 13b: refused: the host does not end a pause by hand",
			in: paused, ev: host(home.OnResume), err: home.RefusedForbidden},
		{name: "row 13b: refused: the host does not end the round limit",
			in: paused.wait("rounds"), ev: host(home.OnResume), err: home.RefusedForbidden},
		{name: "row 13b: refused for an active task",
			in: base, ev: host(home.OnResume), err: home.RefusedConflict},
	})
}

// Row 15.
func TestMachineCancel(t *testing.T) {
	on := machineEvent
	cancel := func(by home.Sender) home.Event { return on(home.OnCancel, by, home.Event{}) }
	base := machineTask()
	ended := mcNow
	cancelled := func(t *proto.TaskRow) { t.State, t.Wait, t.Stats.Ended = "cancelled", nil, &ended }
	gone := home.Effects{Cards: "expired", Retire: []home.ItemFilter{{}}}
	machineRun(t, []machineCase{
		{name: "the secretary cancels an active task",
			in: base.wait("approval"), ev: cancel(home.BySecretary), out: cancelled, fx: gone},
		{name: "an admin cancels a queued task",
			in: base.at("queued", "work").wait("depends"), ev: cancel(home.ByAdmin), out: cancelled, fx: gone},
		{name: "a paused task is cancelled and keeps no wait",
			in: base.at("paused", "review").wait("manual").keep("reviewer"), ev: cancel(home.BySecretary), out: cancelled, fx: gone},
		{name: "the secretary cancels a failed task",
			in: base.at("failed", "work"), ev: cancel(home.BySecretary), out: cancelled, fx: gone},
		{name: "refused from the host",
			in: base, ev: cancel(home.ByHost), err: home.RefusedForbidden},
		{name: "refused from a person who is no admin",
			in: base, ev: cancel(home.ByPerson), err: home.RefusedForbidden},
		{name: "refused for a done task",
			in: base.at("done", "review"), ev: cancel(home.ByAdmin), err: home.RefusedConflict},
		{name: "refused for a cancelled task",
			in: base.at("cancelled", "work"), ev: cancel(home.BySecretary), err: home.RefusedConflict},

		{name: "a queued task whose dependency was cancelled is reported",
			in: base.at("queued", "work").wait("depends"), ev: on(home.OnOrphaned, home.ByHome, home.Event{}),
			fx: mcItems(mcNotice("dependency_cancelled"))},
		{name: "only a queued task waits for a dependency",
			in: base, ev: on(home.OnOrphaned, home.ByHome, home.Event{}), err: home.RefusedConflict},
	})
}

// Rows 17 to 20.
func TestMachineCards(t *testing.T) {
	on := machineEvent
	posted := home.PendingCard{Msg: "A9", Who: []string{"u9"}, Since: 90}
	approval := func(member string, pending ...home.PendingCard) home.Event {
		return on(home.OnApproval, home.ByAI, home.Event{Member: member, Card: posted, Pending: pending})
	}
	decision := func(by home.Sender) home.Event { return on(home.OnDecision, by, home.Event{Member: mcB, Card: posted}) }
	approve := func(card string, pending ...home.PendingCard) home.Event {
		return on(home.OnApprove, home.ByPerson, home.Event{Card: home.PendingCard{Msg: card}, Asker: mcA, Pending: pending})
	}
	choose := func(asker string, pending ...home.PendingCard) home.Event {
		return on(home.OnChoose, home.ByAdmin, home.Event{Card: home.PendingCard{Msg: "D1"}, Asker: asker, Pending: pending})
	}
	base := machineTask()
	paused := base.at("paused", "work").wait("preempted")
	asked := func(kind string) *proto.TaskWait {
		return &proto.TaskWait{Kind: kind, Msg: "A9", Who: []string{"u9"}, Since: 90}
	}
	onA1 := base.set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval1) })
	onD1 := base.set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcDecision) })
	decided := func(ai string) home.TaskItem { return home.TaskItem{Why: "decided", AI: ai} }
	machineRun(t, []machineCase{
		{name: "row 17: the task waits for the approval",
			in: base, ev: approval(mcA),
			out: func(t *proto.TaskRow) { t.Wait = asked("approval") }},
		{name: "row 17: an approval takes the wait from a pending decision",
			in: onD1, ev: approval(mcA, mcDecision),
			out: func(t *proto.TaskRow) { t.Wait = asked("approval") }},
		{name: "row 17: an older pending approval keeps the wait",
			in: onA1, ev: approval(mcA, mcApproval1)},
		{name: "row 17: the oldest pending approval takes the wait from a decision",
			in: onD1, ev: approval(mcA, mcDecision, mcApproval2),
			out: func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval2) }},
		{name: "row 17: a card the caller already lists as pending is the wait once",
			in: base, ev: approval(mcA, home.PendingCard{Kind: "approval", Msg: "A9", Who: []string{"u9"}, Since: 90}),
			out: func(t *proto.TaskRow) { t.Wait = asked("approval") }},
		{name: "row 17: a reviewer wait stays",
			in: base.at("active", "review").reviewer("").wait("reviewer"), ev: approval(mcA)},
		{name: "row 17: the reviewer's approval sets the wait too",
			in: base, ev: approval(mcR),
			out: func(t *proto.TaskRow) { t.Wait = asked("approval") }},
		{name: "row 17: refused from another AI",
			in: base, ev: approval(mcB), err: home.RefusedForbidden},
		{name: "row 17: refused from the secretary",
			in: base, ev: on(home.OnApproval, home.BySecretary, home.Event{Member: mcSec, Card: posted}), err: home.RefusedForbidden},
		{name: "row 17: answered paused for a paused task",
			in: paused, ev: approval(mcA), err: home.RefusedPaused},
		{name: "row 17: refused for a queued task",
			in: base.at("queued", "work").wait("depends"), ev: approval(mcA), err: home.RefusedConflict},
		{name: "row 17: refused for a failed task",
			in: base.at("failed", "work"), ev: approval(mcA), err: home.RefusedConflict},

		{name: "row 18: the last card decided, the wait is cleared",
			in: onA1, ev: approve("A1", mcApproval1),
			out: func(t *proto.TaskRow) { t.Wait = nil },
			fx:  mcItems(decided(mcA))},
		{name: "row 18: the next approval takes the wait, before an older decision",
			in: onA1, ev: approve("A1", mcDecision, mcApproval1, mcApproval2),
			out: func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval2) },
			fx:  mcItems(decided(mcA))},
		{name: "row 18: with no approval left the pending decision takes the wait",
			in: onA1, ev: approve("A1", mcApproval1, mcDecision),
			out: func(t *proto.TaskRow) { t.Wait = mcCardWait(mcDecision) },
			fx:  mcItems(decided(mcA))},
		{name: "row 18: a card that was not the wait leaves the oldest approval waiting",
			in: onA1, ev: approve("A2", mcApproval1, mcApproval2),
			fx: mcItems(decided(mcA))},
		{name: "row 18: a reviewer wait stays",
			in: base.at("active", "review").reviewer("").wait("reviewer"), ev: approve("A1", mcApproval1),
			fx: mcItems(decided(mcA))},
		{name: "row 18: a paused task keeps its paused wait (R3-1)",
			in: paused, ev: approve("A1", mcApproval1),
			fx: mcItems(decided(mcA))},
		{name: "row 18: refused from an AI",
			in: onA1, ev: on(home.OnApprove, home.ByAI, home.Event{Member: mcA}), err: home.RefusedForbidden},

		{name: "row 19: an active task that waits for nothing waits for the decision",
			in: base, ev: decision(home.ByAI),
			out: func(t *proto.TaskRow) { t.Wait = asked("decision") }},
		{name: "row 19: the secretary asks too",
			in: base, ev: decision(home.BySecretary),
			out: func(t *proto.TaskRow) { t.Wait = asked("decision") }},
		{name: "row 19: an approval wait stays",
			in: onA1, ev: decision(home.ByAI)},
		{name: "row 19: an older decision wait stays",
			in: onD1, ev: decision(home.ByAI)},
		{name: "row 19: a paused task keeps its wait",
			in: paused, ev: decision(home.ByAI)},
		{name: "row 19: a queued task keeps its wait",
			in: base.at("queued", "work").wait("depends"), ev: decision(home.ByAI)},
		{name: "row 19: a failed task stays without a wait",
			in: base.at("failed", "work"), ev: decision(home.ByAI)},
		{name: "row 19: refused for a done task",
			in: base.at("done", "review"), ev: decision(home.ByAI), err: home.RefusedConflict},
		{name: "row 19: refused for a cancelled task",
			in: base.at("cancelled", "work"), ev: decision(home.ByAI), err: home.RefusedConflict},
		{name: "row 19: refused from a person",
			in: base, ev: decision(home.ByPerson), err: home.RefusedForbidden},

		{name: "row 20: the asking AI and the assignee are told",
			in: onD1, ev: choose(mcSec, mcDecision),
			out: func(t *proto.TaskRow) { t.Wait = nil },
			fx:  mcItems(decided(mcSec), decided(mcA))},
		{name: "row 20: an assignee that asked is told once",
			in: onD1, ev: choose(mcA, mcDecision),
			out: func(t *proto.TaskRow) { t.Wait = nil },
			fx:  mcItems(decided(mcA))},
		{name: "row 20: a paused task keeps its paused wait",
			in: paused, ev: choose(mcSec, mcDecision),
			fx: mcItems(decided(mcSec), decided(mcA))},
		{name: "row 20: refused from an AI",
			in: onD1, ev: on(home.OnChoose, home.BySecretary, home.Event{}), err: home.RefusedForbidden},
	})
}

// Rows 21 to 24.
func TestMachineRoundsAndTimers(t *testing.T) {
	on := machineEvent
	mention := on(home.OnMention, home.ByAI, home.Event{Member: mcB})
	text := on(home.OnText, home.ByPerson, home.Event{})
	quiet := on(home.OnQuiet, home.ByHome, home.Event{})
	group := on(home.OnGroupResume, home.ByHome, home.Event{})
	base := machineTask()
	rounds := func(n int) func(*proto.TaskRow) { return func(t *proto.TaskRow) { t.Thread.Rounds = n } }
	limit := home.Effects{System: "rounds_limit", Main: true, Items: []home.TaskItem{mcNotice("rounds_limit")}}
	held := base.at("paused", "work").wait("rounds")
	resume := home.TaskItem{Why: "resume", AI: mcA}
	review := home.TaskItem{Why: "review", AI: mcR, Report: "m1", IfNone: true}
	machineRun(t, []machineCase{
		{name: "row 21: a round below the limit is counted",
			in: base.set(rounds(8)), ev: mention, out: rounds(9)},
		{name: "row 21: the secretary's mention counts too",
			in: base, ev: on(home.OnMention, home.BySecretary, home.Event{Member: mcSec}), out: rounds(3)},
		{name: "row 21: the round that reaches the limit pauses the task",
			in: base.set(rounds(9)), ev: mention,
			out: func(t *proto.TaskRow) { t.Thread.Rounds, t.State, t.Wait = 10, "paused", mcWait("rounds", mcNow) },
			fx:  limit},
		{name: "row 21: a round above the limit pauses the task, and keeps a reviewer wait",
			in: base.at("active", "review").reviewer("").wait("reviewer").set(rounds(10)), ev: mention,
			out: func(t *proto.TaskRow) {
				t.Thread.Rounds, t.State, t.Wait = 11, "paused", mcKept("rounds", "reviewer")
			},
			fx: limit},
		{name: "row 21: a paused task counts the round and keeps its wait",
			in: base.at("paused", "work").wait("manual").set(rounds(9)), ev: mention, out: rounds(10)},
		{name: "row 21: a queued task counts the round",
			in: base.at("queued", "work").wait("depends").set(rounds(9)), ev: mention, out: rounds(10)},
		{name: "row 21: a done task counts the round and stays done",
			in: base.at("done", "review").set(rounds(9)), ev: mention, out: rounds(10)},
		{name: "row 21: a person's post is no round",
			in: base, ev: on(home.OnMention, home.ByPerson, home.Event{}), err: home.RefusedForbidden},

		{name: "row 22: a person's message starts the rounds again",
			in: base, ev: text, out: rounds(0)},
		{name: "row 22: it ends the round limit",
			in: held, ev: on(home.OnText, home.ByAdmin, home.Event{}),
			out: func(t *proto.TaskRow) { t.Thread.Rounds, t.State, t.Wait = 0, "active", nil }},
		{name: "row 22: a kept reviewer wait is restored",
			in: held.at("paused", "review").reviewer("").keep("reviewer"), ev: text,
			out: func(t *proto.TaskRow) {
				t.Thread.Rounds, t.State, t.Wait = 0, "active", mcWait("reviewer", mcNow)
			}},
		{name: "row 22: at step review the reviewer gets the report unless it is queued",
			in: held.at("paused", "review"), ev: text,
			out: func(t *proto.TaskRow) { t.Thread.Rounds, t.State, t.Wait = 0, "active", nil },
			fx:  mcItems(review)},
		{name: "row 22: a pause by hand stays",
			in: base.at("paused", "work").wait("manual"), ev: text, out: rounds(0)},
		{name: "row 22: an AI's post is not a person's",
			in: held, ev: on(home.OnText, home.ByAI, home.Event{Member: mcB}), err: home.RefusedForbidden},

		{name: "row 23: a quiet task is reported",
			in: base, ev: quiet, fx: mcItems(mcNotice("quiet"))},
		{name: "row 23: not while it waits for something",
			in: base.wait("approval"), ev: quiet, err: home.RefusedConflict},
		{name: "row 23: not while paused",
			in: base.at("paused", "work").wait("manual"), ev: quiet, err: home.RefusedConflict},
		{name: "row 23: not once it is done",
			in: base.at("done", "review"), ev: quiet, err: home.RefusedConflict},
		{name: "row 23: only the home's timer says so",
			in: base, ev: on(home.OnQuiet, home.ByHost, home.Event{}), err: home.RefusedForbidden},

		{name: "row 24: an active task that waits for nothing continues",
			in: base, ev: group, fx: mcItems(resume)},
		{name: "row 24: at step review the reviewer gets the report unless it is queued",
			in: base.at("active", "review"), ev: group, fx: mcItems(resume, review)},
		{name: "row 24: a task that waits for an approval is not woken, its reviewer is",
			in: base.at("active", "review").wait("approval"), ev: group, fx: mcItems(review)},
		{name: "row 24: a task that waits for a reviewer gets nothing",
			in: base.at("active", "review").reviewer("").wait("reviewer"), ev: group},
		{name: "row 24: a task that waits for an approval is not woken",
			in: base.wait("approval"), ev: group},
		{name: "row 24: a paused task gets nothing",
			in: base.at("paused", "review").wait("manual"), ev: group},
		{name: "row 24: a queued task gets nothing",
			in: base.at("queued", "work").wait("depends"), ev: group},
		{name: "row 24: a done task gets nothing",
			in: base.at("done", "review"), ev: group},
		{name: "row 24: the group command is the caller's, not a sender of this event",
			in: base, ev: on(home.OnGroupResume, home.ByAdmin, home.Event{}), err: home.RefusedForbidden},
	})
}

// Final is final: no event from any sender changes the state, step, wait,
// report, assignee or reviewer of a task that is done or cancelled.
func TestMachineFinalTaskIsFrozen(t *testing.T) {
	frozen := func(t proto.TaskRow) []any { return []any{t.State, t.Step, t.Wait, t.Report, t.Assignee, t.Reviewer} }
	for _, state := range []string{"done", "cancelled"} {
		in := machineTask().at(state, "review").TaskRow
		for _, ev := range mcEveryEvent() {
			if got, _, err := home.Step(in, ev); !reflect.DeepEqual(frozen(got), frozen(in)) {
				t.Errorf("%s by %d as %q for %s changed a %s task, error %v", ev.Kind, ev.By, ev.Member, ev.Report, state, err)
			}
		}
	}
}

// mcEveryEvent is every event of the table from every sender, signed for nobody
// and for each AI, about the task's report and about another.
func mcEveryEvent() []home.Event {
	kinds := []home.EventKind{
		home.OnConfirm, home.OnStart, home.OnReport, home.OnReview, home.OnReleased, home.OnFail, home.OnAssign,
		home.OnPause, home.OnResume, home.OnCancel, home.OnOrphaned, home.OnPreempted, home.OnApproval,
		home.OnApprove, home.OnDecision, home.OnChoose, home.OnMention, home.OnText, home.OnQuiet, home.OnGroupResume,
	}
	senders := []home.Sender{home.ByHost, home.BySecretary, home.ByAdmin, home.ByPerson, home.ByAI, home.ByHome}
	var events []home.Event
	for _, kind := range kinds {
		for _, by := range senders {
			for _, member := range []string{"", mcA, mcR, mcB} {
				for _, report := range []string{"m1", "m2"} {
					ev := machineEvent(kind, by, home.Event{
						Member: member, Report: report, OK: true, Reason: "crash", Assignee: mcB, Reviewer: "ai-third",
						Card: home.PendingCard{Msg: "A1", Who: []string{"u1"}, Since: 11}, Asker: mcSec,
						Pending: []home.PendingCard{mcApproval1, mcDecision},
					})
					ev.Limits.Rounds = 1
					events = append(events, ev)
				}
			}
		}
	}
	return events
}

// R3-1: deciding the last card of a pre-empted task leaves the pause for the host to end.
func TestMachineCardDecidedWhilePreempted(t *testing.T) {
	on := machineEvent
	task := machineTask().set(func(t *proto.TaskRow) { t.Wait = mcCardWait(mcApproval1) }).TaskRow
	tasks, _ := mcPlay(t, task,
		on(home.OnPreempted, home.ByHost, home.Event{}),
		on(home.OnApprove, home.ByPerson, home.Event{Card: mcApproval1, Asker: mcA, Pending: []home.PendingCard{mcApproval1}}),
		on(home.OnResume, home.ByHost, home.Event{}),
	)
	got := []string{mcState(tasks[0]), mcState(tasks[1]), mcState(tasks[2])}
	mcSame(t, "the task after each step", got, []string{"paused preempted", "paused preempted", "active"})
}

// R3-2: a reviewer named while the task is paused gets the review when the
// task is resumed, with no second reassignment.
func TestMachineReviewerNamedWhilePaused(t *testing.T) {
	on := machineEvent
	task := machineTask().at("active", "review").reviewer("").wait("reviewer").TaskRow
	tasks, fx := mcPlay(t, task,
		on(home.OnPause, home.BySecretary, home.Event{}),
		on(home.OnAssign, home.BySecretary, home.Event{Reviewer: mcB}),
		on(home.OnResume, home.BySecretary, home.Event{}),
	)
	got := []string{mcState(tasks[0]), mcState(tasks[1]), mcState(tasks[2])}
	mcSame(t, "the task after each step", got, []string{"paused manual", "paused manual", "active"})
	items := [][]home.TaskItem{fx[0].Items, fx[1].Items, fx[2].Items}
	mcSame(t, "the items of each step", items, [][]home.TaskItem{nil, nil, {
		{Why: "review", AI: mcB, Report: "m1", IfNone: true}, {Why: "resume", AI: mcA},
	}})
}

// A submission that arrives while the task is paused reaches the reviewer
// when the pause ends.
func TestMachineSubmissionWhilePaused(t *testing.T) {
	on := machineEvent
	tasks, fx := mcPlay(t, machineTask().TaskRow,
		on(home.OnPause, home.ByAdmin, home.Event{Member: "person"}),
		on(home.OnReport, home.ByAI, home.Event{Member: mcA, Report: "m2"}),
		on(home.OnResume, home.ByAdmin, home.Event{}),
	)
	got := []string{mcState(tasks[0]), mcState(tasks[1]), mcState(tasks[2])}
	mcSame(t, "the task after each step", got, []string{"paused manual", "paused manual", "active"})
	mcSame(t, "the wait of the pause", tasks[1].Wait, &proto.TaskWait{Kind: "manual", Since: mcNow, By: "person"})
	mcSame(t, "the items of the resume", fx[2].Items, []home.TaskItem{
		{Why: "review", AI: mcR, Report: "m2", IfNone: true}, {Why: "resume", AI: mcA},
	})
}

func TestMachineRefusalIsAnError(t *testing.T) {
	got := []string{home.RefusedConflict.Error(), home.RefusedPaused.Error(), home.RefusedForbidden.Error()}
	mcSame(t, "a refusal's text is its error code", got, []string{"conflict", "paused", "forbidden"})
}
