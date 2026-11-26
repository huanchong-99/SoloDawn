package home

import (
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The task machine: planning/contracts/task-machine.md as one pure function.
// Step reads a task row and an event and returns the row as the event leaves
// it, with what the home must do beside writing it. It reads no clock, no
// store and no other row: what a row of the table needs from outside the task
// (the group's limits, the pending cards, whether a dependency is done and
// whether the folder is busy) the caller puts into the event.
//
// What stays with the caller: who the sender is (Sender), the checks a table
// row makes on a card or on the group, the fields a command only copies into
// the row (thread.count, thread.last), the bodies of the messages, and the
// fan-out over other tasks: OnReleased for each queued task when a row may
// have freed a dependency or a folder, OnOrphaned for each that depends on a
// cancelled one, OnGroupResume for every task.

// EventKind names the row of the table an event asks for.
type EventKind string

const (
	OnConfirm     EventKind = "confirm"      // row 1: one entry of a confirmed brief; the row is new
	OnStart       EventKind = "start"        // row 2: task start; the row is new
	OnReport      EventKind = "report"       // row 5: post of kind report
	OnReview      EventKind = "review"       // row 7: post of kind review
	OnReleased    EventKind = "released"     // section 2, "Starting a queued task": a queued task looked at
	OnFail        EventKind = "fail"         // row 10
	OnAssign      EventKind = "assign"       // row 11
	OnPause       EventKind = "pause"        // row 12
	OnResume      EventKind = "resume"       // rows 13 and 13b
	OnCancel      EventKind = "cancel"       // row 15, for the cancelled task
	OnOrphaned    EventKind = "orphaned"     // row 15, for a queued task that depends on the cancelled one
	OnPreempted   EventKind = "preempted"    // row 16
	OnApproval    EventKind = "approval"     // row 17: post of kind approval
	OnApprove     EventKind = "approve"      // row 18: act approve or deny
	OnDecision    EventKind = "decision"     // row 19: post of kind decision naming the task
	OnChoose      EventKind = "choose"       // row 20: act choose
	OnMention     EventKind = "mention"      // row 21: a post by an AI in the thread that mentions an AI
	OnText        EventKind = "text"         // row 22: a post of kind text by a person in the thread
	OnQuiet       EventKind = "quiet"        // row 23: the timer found no thread message for limits.idle_notice_minutes
	OnGroupResume EventKind = "group_resume" // row 24, for each task of the group
)

// Sender is who sent an event, as the caller has established it.
type Sender uint8

const (
	ByHost      Sender = 1 << iota // the node of the task's assignee, speaking for itself
	BySecretary                    // the group's secretary
	ByAdmin                        // a person with role owner or admin
	ByPerson                       // any other person
	ByAI                           // a working AI of the group; never the secretary, which is BySecretary and nothing else
	ByHome                         // the home itself: a timer, or what follows from another row
	byAssignee                     // Event.Member is the task's assignee
	byReviewer                     // Event.Member is the task's reviewer
)

// Event is one command or fact, with what its row needs from outside the task.
type Event struct {
	Kind   EventKind
	By     Sender
	Member string // the member the command is by: the AI it is signed for (`as`), else the person
	Now    int64  // the home's clock, in milliseconds

	Limits   proto.Limits // the group's limits
	Stronger bool         // the group has an AI stronger than the assignee

	Report   string // row 5: the report message; row 7: the report the review names
	OK       bool   // row 7: the review passed (commands.md 1.8)
	Score    int    // row 7: the score the home wrote
	Reason   string // row 10: crash or timeout
	Assignee string // row 11: the assignee to set, or "" to keep it
	Reviewer string // row 11: the reviewer to set, or "" to keep it
	Blocked  bool   // row 1 and OnReleased: a dependency of the task is not done
	Busy     bool   // rows 1, 2 and OnReleased: another started task has the folder of the assignee; row 11: of the assignee to set

	Card    PendingCard   // rows 17 and 19: the card posted; rows 18 and 20: the card decided
	Asker   string        // rows 18 and 20: the AI that posted the card
	Pending []PendingCard // the task's pending cards, oldest first; the card of rows 17, 18 and 20 may be among them
}

// PendingCard is an approval or decision card of the task that nobody has decided.
type PendingCard struct {
	Kind  string   // approval or decision
	Msg   string   // the card's message id
	Who   []string // the people asked: body.approvers, or body.to
	Since int64    // the card's ts
}

// Effects is what the home does beside writing the row: first Retire, then the rest.
type Effects struct {
	Retire []ItemFilter // queued inbox items of the task to delete
	Items  []TaskItem   // inbox items to queue, in this order
	Cards  string       // "expired": every pending card of the task expires; "reassigned": its pending approval cards do
	System string       // the code of the system message to write, or ""
	Main   bool         // the system message goes in the main line (row 21); otherwise in the task's thread
	Reason string       // with System task_failed: failures or review_rounds
}

// TaskItem is an inbox item about the task (planning/contracts/datachannel.md section 6).
// The caller adds the message its row names.
type TaskItem struct {
	Why    string
	AI     string // the member it is for; a notice has none, it is the secretary's
	Code   string // why notice: the case (task-machine.md section 5)
	Report string // why review: the report message under review
	IfNone bool   // why review: queue it only if none is queued for this AI and report
}

// ItemFilter selects queued inbox items of the task. An empty field selects every value.
type ItemFilter struct {
	AI  string
	Why string
}

// TaskRefusal is the error of a refused event; its text is the error code of
// the answer (planning/contracts/datachannel.md section 9).
type TaskRefusal string

func (r TaskRefusal) Error() string { return string(r) }

const (
	RefusedConflict  TaskRefusal = "conflict"  // the task is not in a state that allows this
	RefusedPaused    TaskRefusal = "paused"    // an active task would allow it, and this one is paused
	RefusedForbidden TaskRefusal = "forbidden" // this sender may not do this, or not while the task is as it is
)

const (
	taskQueued    = "queued"
	taskActive    = "active"
	taskPaused    = "paused"
	taskFailed    = "failed"
	taskDone      = "done"
	taskCancelled = "cancelled"

	stepWork   = "work"
	stepReview = "review"

	waitDepends   = "depends"
	waitBusy      = "busy"
	waitApproval  = "approval"
	waitDecision  = "decision"
	waitReviewer  = "reviewer"
	waitManual    = "manual"
	waitRounds    = "rounds"
	waitPreempted = "preempted"

	itemAssign   = "assign"
	itemReviewed = "reviewed"
	itemReview   = "review"
	itemResume   = "resume"
	itemDecided  = "decided"
	itemNotice   = "notice"
)

var (
	statesNew     = []string{""} // the row has no state yet
	statesRunning = []string{taskActive, taskPaused}
	statesStarted = []string{taskActive, taskPaused, taskFailed}             // the task has its folder
	statesOpen    = []string{taskQueued, taskActive, taskPaused, taskFailed} // not final
	statesActive  = []string{taskActive}

	waitsOfCards = []string{"", waitConfirm, waitApproval, waitDecision} // no wait, or a card wait
	workItems    = []string{itemAssign, itemReviewed, itemReview, itemResume}
)

// taskRule is one line of the table: who may send the event, when, and what it does.
type taskRule struct {
	by       Sender
	states   []string             // the states that allow it; none means any
	steps    []string             // the steps that allow it; none means any
	also     func(*taskMove) bool // what else must hold
	do       func(*taskMove)
	orPaused bool // answered paused, not conflict, when only the pause stands in its way (section 2)
}

// An event with several lines takes the first whose sender and condition hold.
var taskRules = map[EventKind][]taskRule{
	OnConfirm:  {{by: ByAdmin | ByPerson, states: statesNew, do: (*taskMove).create}},
	OnStart:    {{by: ByAI, states: statesNew, do: (*taskMove).start}},
	OnReport:   {{by: byAssignee, states: statesRunning, do: (*taskMove).submitted}},
	OnReview:   {{by: byReviewer, states: statesActive, steps: []string{stepReview}, also: (*taskMove).reviewable, do: (*taskMove).reviewed, orPaused: true}},
	OnReleased: {{by: ByHome, states: []string{taskQueued}, do: (*taskMove).judge}},
	OnFail:     {{by: ByHost, states: statesActive, do: (*taskMove).crashed, orPaused: true}},
	OnAssign: {
		{by: BySecretary, states: []string{taskQueued, taskActive, taskPaused}, also: (*taskMove).folderFree, do: (*taskMove).assign},
		{by: ByAdmin, states: statesOpen, also: (*taskMove).folderFree, do: (*taskMove).assign},
	},
	OnPause: {{by: BySecretary | ByAdmin, states: statesActive, do: (*taskMove).pauseByHand, orPaused: true}},
	OnResume: {
		{by: BySecretary, states: []string{taskPaused}, also: (*taskMove).notByRounds, do: (*taskMove).resume},
		{by: ByAdmin, states: []string{taskPaused, taskFailed}, do: (*taskMove).resume},
		{by: ByHost, states: []string{taskPaused}, also: (*taskMove).preempted, do: (*taskMove).activate},
	},
	OnCancel:      {{by: BySecretary | ByAdmin, states: statesOpen, do: (*taskMove).cancel}},
	OnOrphaned:    {{by: ByHome, states: []string{taskQueued}, do: (*taskMove).orphaned}},
	OnPreempted:   {{by: ByHost, states: statesActive, do: (*taskMove).preempt, orPaused: true}},
	OnApproval:    {{by: byAssignee | byReviewer, states: statesActive, do: (*taskMove).approval, orPaused: true}},
	OnApprove:     {{by: ByAdmin | ByPerson, do: (*taskMove).decided}},
	OnDecision:    {{by: BySecretary | ByAI, states: statesOpen, do: (*taskMove).decision}},
	OnChoose:      {{by: ByAdmin | ByPerson, do: (*taskMove).chosen}},
	OnMention:     {{by: BySecretary | ByAI, do: (*taskMove).mention}},
	OnText:        {{by: ByAdmin | ByPerson, do: (*taskMove).text}},
	OnQuiet:       {{by: ByHome, states: statesActive, also: (*taskMove).idle, do: (*taskMove).quiet}},
	OnGroupResume: {{by: ByHome, do: (*taskMove).groupResume}},
}

// taskMove is one step in the making: the row, changed in place, and its effects.
type taskMove struct {
	t       proto.TaskRow
	e       Event
	x       Effects
	by      Sender        // e.By, with the roles e.Member holds in the task
	pending []PendingCard // the cards still pending once this event is applied
}

// Step applies an event to a task row. A refused event changes nothing and
// returns a TaskRefusal.
func Step(t proto.TaskRow, e Event) (proto.TaskRow, Effects, error) {
	m := taskMove{t: t, e: e, by: taskSender(t, e), pending: e.Pending}
	rule, err := m.rule()
	if err != nil {
		return t, Effects{}, err
	}
	if e.Kind == OnReport && profileMustAsk(t) {
		return t, Effects{}, RefusedConflict
	}
	rule.do(&m)
	// Roles (section 2): assignee and reviewer are never the same AI.
	if m.t.Assignee == m.t.Reviewer {
		return t, Effects{}, RefusedConflict
	}
	return m.t, m.x, nil
}

// taskSender is who sent the event, with the roles its member holds in the task.
func taskSender(t proto.TaskRow, e Event) Sender {
	by := e.By
	if e.Member == "" {
		return by
	}
	if e.Member == t.Assignee {
		by |= byAssignee
	}
	if e.Member == t.Reviewer {
		by |= byReviewer
	}
	return by
}

// rule finds the line that allows the event, or how the event is refused.
// The sender comes first: an event no line gives to this sender is forbidden,
// and so is one the task allows from another sender only.
func (m *taskMove) rule() (taskRule, error) {
	rules := taskRules[m.e.Kind]
	if i := slices.IndexFunc(rules, m.allows); i >= 0 {
		return rules[i], nil
	}
	if !slices.ContainsFunc(rules, m.named) || slices.ContainsFunc(rules, m.holds) {
		return taskRule{}, RefusedForbidden
	}
	return taskRule{}, m.refusal(rules)
}

// refusal is paused when a line marked for it would allow the event without
// the pause, else conflict.
func (m *taskMove) refusal(rules []taskRule) TaskRefusal {
	active := *m
	active.t.State, active.t.Wait = taskActive, m.restored()
	if m.t.State == taskPaused && slices.ContainsFunc(rules, active.pausedOnly) {
		return RefusedPaused
	}
	return RefusedConflict
}

func (m *taskMove) pausedOnly(r taskRule) bool { return r.orPaused && m.allows(r) }

func (m *taskMove) allows(r taskRule) bool { return m.named(r) && m.holds(r) }

// named says whether the line gives the event to this sender.
func (m *taskMove) named(r taskRule) bool { return r.by&m.by != 0 }

// holds says whether the task is as the line asks.
func (m *taskMove) holds(r taskRule) bool {
	return taskIn(r.states, m.t.State) && taskIn(r.steps, m.t.Step) && (r.also == nil || r.also(m))
}

func taskIn(set []string, v string) bool { return len(set) == 0 || slices.Contains(set, v) }

// The conditions beside state and step.

func (m *taskMove) reviewable() bool {
	return m.waitKind() != waitReviewer && m.e.Report == m.t.Report
}

func (m *taskMove) notByRounds() bool { return m.waitKind() != waitRounds }

func (m *taskMove) preempted() bool { return m.waitKind() == waitPreempted }

func (m *taskMove) idle() bool { return m.t.Wait == nil }

// folderFree is the refusal of row 11: a started task is never given to an
// assignee whose folder is busy with another task.
func (m *taskMove) folderFree() bool { return !m.e.Busy || m.t.State == taskQueued }

// cardsDecide says whether a card may set or clear the wait (section 2).
func (m *taskMove) cardsDecide() bool {
	return m.t.State == taskActive && slices.Contains(waitsOfCards, m.waitKind())
}

func (m *taskMove) waitKind() string {
	if m.t.Wait == nil {
		return ""
	}
	return m.t.Wait.Kind
}

// kept is the wait the task's pause replaced and keeps, or "".
func (m *taskMove) kept() string {
	if m.t.Wait == nil {
		return ""
	}
	return m.t.Wait.Kept
}

// The building blocks.

func (m *taskMove) waitFor(kind string) *proto.TaskWait {
	return &proto.TaskWait{Kind: kind, Since: m.e.Now}
}

// cardWait is the wait of a task that waits for nothing else: its oldest
// pending approval card, else its oldest pending decision card, else none.
func (m *taskMove) cardWait() *proto.TaskWait {
	for _, kind := range []string{waitApproval, waitDecision} {
		for _, c := range m.pending {
			if c.Kind == kind {
				return taskCardWait(c)
			}
		}
	}
	return nil
}

func taskCardWait(c PendingCard) *proto.TaskWait {
	return &proto.TaskWait{Kind: c.Kind, Msg: c.Msg, Who: c.Who, Since: c.Since}
}

// posted is the card the event posts.
func (m *taskMove) posted(kind string) PendingCard {
	c := m.e.Card
	c.Kind = kind
	return c
}

// forget drops cards that stop being pending in this step.
func (m *taskMove) forget(gone func(PendingCard) bool) {
	m.pending = slices.DeleteFunc(slices.Clone(m.pending), gone)
}

// restored is the wait of a task that becomes active: the one its pause kept, else its cards.
func (m *taskMove) restored() *proto.TaskWait {
	if k := m.kept(); k != "" {
		return m.waitFor(k)
	}
	return m.cardWait()
}

// settle ends a wait of one of these kinds: an active task's wait is
// recomputed, a paused task no longer keeps it.
func (m *taskMove) settle(kinds ...string) {
	if slices.Contains(kinds, m.kept()) {
		wait := *m.t.Wait
		wait.Kept = ""
		m.t.Wait = &wait
	}
	if slices.Contains(kinds, m.waitKind()) {
		m.t.Wait = m.cardWait()
	}
}

// queue adds an inbox item. A work item goes to an active task only; the
// task gets it when it becomes active.
func (m *taskMove) queue(it TaskItem) {
	if m.t.State != taskActive && slices.Contains(workItems, it.Why) {
		return
	}
	m.x.Items = append(m.x.Items, it)
}

func (m *taskMove) notice(code string) { m.queue(TaskItem{Why: itemNotice, Code: code}) }

func (m *taskMove) retire(f ItemFilter) { m.x.Retire = append(m.x.Retire, f) }

// activate makes the task active with the wait it has then. At step review
// the reviewer gets the report unless it is queued for them already.
func (m *taskMove) activate() {
	m.t.State, m.t.Wait = taskActive, m.restored()
	if m.t.Step == stepReview {
		m.askReview(true)
	}
}

// askReview gives the report to the reviewer. Without one an active task
// waits for a reviewer; a paused one does when it becomes active.
func (m *taskMove) askReview(ifNone bool) {
	if m.t.Reviewer != "" {
		m.queue(TaskItem{Why: itemReview, AI: m.t.Reviewer, Report: m.t.Report, IfNone: ifNone})
		return
	}
	if m.t.State == taskActive {
		m.t.Wait = m.waitFor(waitReviewer)
	}
}

// pause is the pause of rows 12, 16 and 21: it keeps a reviewer wait it replaces.
func (m *taskMove) pause(kind string) {
	wait := m.waitFor(kind)
	if m.waitKind() == waitReviewer {
		wait.Kept = waitReviewer
	}
	m.t.State, m.t.Wait = taskPaused, wait
}

// strike counts a failure. At the limit the task stops (row 14) and the item is not sent.
func (m *taskMove) strike(it TaskItem) {
	m.t.Failures++
	if m.t.Failures >= m.e.Limits.Failures {
		m.stop("failures")
		return
	}
	m.queue(it)
}

// stop is row 14.
func (m *taskMove) stop(reason string) {
	m.t.State, m.t.Wait = taskFailed, nil
	m.x.System, m.x.Reason = "task_failed", reason
	m.notice("task_failed")
}

// forgive starts the counters of R-63 again (rows 11 and 13, from failed).
func (m *taskMove) forgive() {
	m.t.Failures = 0
	m.t.Stuck = nil
	if m.t.Review != nil {
		review := *m.t.Review
		review.Failed = 0
		m.t.Review = &review
	}
}

// end makes the task final (rows 7 and 15).
func (m *taskMove) end(state string) {
	now := m.e.Now
	m.t.State, m.t.Wait, m.t.Stats.Ended = state, nil, &now
	m.x.Cards = "expired"
}

// The rows.

// create is row 1.
func (m *taskMove) create() {
	m.t.State, m.t.Step = taskQueued, stepWork
	m.judge()
}

// start is row 2.
func (m *taskMove) start() {
	m.t.State, m.t.Step, m.t.Assignee, m.t.Reviewer, m.t.Foundation = taskQueued, stepWork, m.e.Member, "", false
	m.judge()
	m.notice("task_started")
}

// judge is "Starting a queued task" (section 2): the task starts unless a
// dependency is not done or its folder is busy, which its wait then says.
func (m *taskMove) judge() {
	switch {
	case m.e.Blocked:
		m.hold(waitDepends)
	case m.e.Busy:
		m.hold(waitBusy)
	default:
		m.t.Stats.Started = m.e.Now
		m.activate()
		m.queue(TaskItem{Why: itemAssign, AI: m.t.Assignee})
	}
}

// hold keeps a queued task waiting. A wait that says the same already stays
// as it is, with its since.
func (m *taskMove) hold(kind string) {
	if m.waitKind() != kind {
		m.t.Wait = m.waitFor(kind)
	}
}

// submitted is row 5. Any review item still queued is about an earlier report.
func (m *taskMove) submitted() {
	m.t.Report, m.t.Step, m.t.Failures = m.e.Report, stepReview, 0
	m.retire(ItemFilter{Why: itemReview})
	if m.t.Reviewer == "" {
		m.notice("no_reviewer")
	}
	m.askReview(false)
}

// reviewed is row 7.
func (m *taskMove) reviewed() {
	r := proto.Review{Score: m.e.Score, OK: m.e.OK, By: m.e.Member, Report: m.e.Report, At: m.e.Now}
	if last := m.t.Review; last != nil {
		r.Round, r.Failed = last.Round, last.Failed
	}
	r.Round++
	m.t.Review = &r
	if m.e.OK {
		m.t.Stuck = nil
		m.end(taskDone)
		m.x.System = "task_done"
		m.notice("task_done")
		return
	}
	m.t.Stuck = profileFailure(m.t.Stuck, m.e.Stronger)
	r.Failed++
	if r.Failed > m.e.Limits.ReviewRounds {
		m.stop("review_rounds")
		return
	}
	m.t.Step = stepWork
	m.queue(TaskItem{Why: itemReviewed, AI: m.t.Assignee})
}

// crashed is row 10.
func (m *taskMove) crashed() { m.strike(TaskItem{Why: itemNotice, Code: m.e.Reason}) }

// assign is row 11.
func (m *taskMove) assign() {
	was := *m
	m.setAssignee()
	m.setReviewer()
	if was.t.State == taskFailed {
		m.forgive()
		m.activate()
	}
	if m.t.Assignee != was.t.Assignee || was.t.State == taskFailed {
		m.queue(TaskItem{Why: itemAssign, AI: m.t.Assignee})
	}
}

// setAssignee gives the task to another AI: the cards the old one asked with
// are reassigned, so an approval wait ends, and its queued items are retired.
func (m *taskMove) setAssignee() {
	old := m.t.Assignee
	if m.e.Assignee == "" || m.e.Assignee == old {
		return
	}
	m.t.Assignee = m.e.Assignee
	m.t.Stuck = nil
	m.x.Cards = "reassigned"
	m.retire(ItemFilter{AI: old})
	m.forget(func(c PendingCard) bool { return c.Kind == waitApproval })
	m.settle(waitApproval)
}

// setReviewer names another reviewer: the old one's queued review is retired.
// At step review a reviewer wait ends and the new reviewer gets the report;
// a task that was failed gets it when assign makes it active.
func (m *taskMove) setReviewer() {
	old := m.t.Reviewer
	if m.e.Reviewer == "" || m.e.Reviewer == old {
		return
	}
	m.t.Reviewer = m.e.Reviewer
	m.retire(ItemFilter{AI: old, Why: itemReview})
	if m.t.Step == stepReview {
		m.settle(waitReviewer)
		m.askReview(false)
	}
}

// pauseByHand is row 12: the wait says who paused.
func (m *taskMove) pauseByHand() {
	m.pause(waitManual)
	m.t.Wait.By = m.e.Member
}

// resume is row 13; row 13b is activate alone.
func (m *taskMove) resume() {
	if m.t.State == taskFailed {
		m.forgive()
	}
	if m.waitKind() == waitRounds {
		m.t.Thread.Rounds = 0
	}
	m.activate()
	m.queue(TaskItem{Why: itemResume, AI: m.t.Assignee})
}

// cancel is row 15 for the cancelled task.
func (m *taskMove) cancel() {
	m.end(taskCancelled)
	m.retire(ItemFilter{})
}

// orphaned is row 15 for a queued task that depends on the cancelled one.
func (m *taskMove) orphaned() { m.notice("dependency_cancelled") }

// preempt is row 16.
func (m *taskMove) preempt() {
	m.pause(waitPreempted)
	m.x.System = "preempted"
}

// approval is row 17: the card is the newest pending one, and the wait is
// recomputed, so an older pending approval keeps it.
func (m *taskMove) approval() {
	if m.cardsDecide() {
		m.pending = append(slices.Clone(m.pending), m.posted(waitApproval))
		m.t.Wait = m.cardWait()
	}
}

// decided is row 18, and the part of row 20 that is the same.
func (m *taskMove) decided() {
	m.forget(func(c PendingCard) bool { return c.Msg == m.e.Card.Msg })
	if m.cardsDecide() {
		m.t.Wait = m.cardWait()
	}
	m.queue(TaskItem{Why: itemDecided, AI: m.e.Asker})
}

// decision is row 19.
func (m *taskMove) decision() {
	if m.t.State == taskActive && m.t.Wait == nil {
		m.t.Wait = taskCardWait(m.posted(waitDecision))
	}
}

// chosen is row 20.
func (m *taskMove) chosen() {
	m.decided()
	if m.t.Assignee != m.e.Asker {
		m.queue(TaskItem{Why: itemDecided, AI: m.t.Assignee})
	}
}

// mention is row 21.
func (m *taskMove) mention() {
	m.t.Thread.Rounds++
	if m.t.Thread.Rounds < m.e.Limits.Rounds || m.t.State != taskActive {
		return
	}
	m.pause(waitRounds)
	m.x.System, m.x.Main = "rounds_limit", true
	m.notice("rounds_limit")
}

// text is row 22.
func (m *taskMove) text() {
	m.t.Thread.Rounds = 0
	if m.waitKind() == waitRounds {
		m.activate()
	}
}

// quiet is row 23.
func (m *taskMove) quiet() { m.notice("quiet") }

// groupResume is row 24. The row does not change.
func (m *taskMove) groupResume() {
	if m.t.Wait == nil {
		m.queue(TaskItem{Why: itemResume, AI: m.t.Assignee})
	}
	if m.t.Step == stepReview && m.t.Reviewer != "" {
		m.queue(TaskItem{Why: itemReview, AI: m.t.Reviewer, Report: m.t.Report, IfNone: true})
	}
}
