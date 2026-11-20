package home

import (
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The task machine: planning/contracts/task-machine.md as one pure function.
// Step reads a task row and an event and returns the row as the event leaves
// it, with what the home must do beside writing it. It reads no clock, no
// store and no other row: what a row of the table needs from outside the task
// (the group's limits, the pending cards, whether a dependency is merged) the
// caller puts into the event.
//
// What stays with the caller: who the sender is (Sender), the checks a table
// row makes on a card or on the group, the fields a command only copies into
// the row (branch, pr, pr_url, stats.added, stats.removed, thread.count,
// thread.last), the bodies of the messages, and the fan-out over other tasks:
// OnReleased for each queued task that depends on a merged one, OnOrphaned
// for each that depends on a cancelled one, OnGroupResume for every task.

// Task is a task row as the machine reads and writes it.
type Task struct {
	proto.TaskRow
	// review.failed and wait.kept of planning/contracts/rows.md section 4.
	// They are held beside the row until P-1 freezes them in proto.TaskRow.
	ReviewFailed int
	Kept         string
}

// EventKind names the row of the table an event asks for.
type EventKind string

const (
	OnConfirm     EventKind = "confirm"      // row 1: one entry of a confirmed brief; the row is new
	OnStart       EventKind = "start"        // row 2: task start; the row is new
	OnBranch      EventKind = "branch"       // row 3
	OnChecked     EventKind = "checked"      // row 4
	OnSubmitted   EventKind = "submitted"    // row 5
	OnCloud       EventKind = "cloud"        // row 6
	OnReview      EventKind = "review"       // row 7: post of kind review
	OnMerged      EventKind = "merged"       // row 8, for the merged task
	OnReleased    EventKind = "released"     // row 8, for a queued task that depends on the merged one
	OnConflict    EventKind = "conflict"     // row 9
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
	Member string // the AI the command is signed for (`as`), when there is one
	Now    int64  // the home's clock, in milliseconds

	Limits proto.Limits // the group's limits
	Remote bool         // the group has a remote (`forge` is not `none`)

	Commit   string // rows 4 to 9: the commit the command names
	OK       bool   // rows 4 and 6: the checks passed; row 7: the review passed (commands.md 1.8)
	Score    int    // row 7: the score the home wrote
	Reason   string // row 10: crash or timeout
	Assignee string // row 11: the assignee to set, or "" to keep it
	Reviewer string // row 11: the reviewer to set, or "" to keep it
	Blocked  bool   // rows 1 and 8: a dependency of the task is not merged

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
// The caller adds the message and the cause its row names.
type TaskItem struct {
	Why    string
	AI     string // the member it is for; a notice has none, it is the secretary's
	Code   string // why notice: the case (task-machine.md section 5)
	Commit string // why review and merge
	IfNone bool   // why review: queue it only if none is queued for this AI and commit
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
	taskMerged    = "merged"
	taskCancelled = "cancelled"

	stepBranch = "branch"
	stepLocal  = "local_check"
	stepCloud  = "cloud_check"
	stepReview = "review"
	stepMerge  = "merge"

	waitDepends   = "depends"
	waitApproval  = "approval"
	waitDecision  = "decision"
	waitReviewer  = "reviewer"
	waitConflict  = "conflict"
	waitManual    = "manual"
	waitRounds    = "rounds"
	waitPreempted = "preempted"

	itemAssign   = "assign"
	itemChecks   = "checks"
	itemReviewed = "reviewed"
	itemReview   = "review"
	itemResume   = "resume"
	itemMerge    = "merge"
	itemDecided  = "decided"
	itemNotice   = "notice"
)

var (
	statesNew     = []string{""} // the row has no state yet
	statesRunning = []string{taskActive, taskPaused}
	statesOpen    = []string{taskQueued, taskActive, taskPaused, taskFailed} // not final
	statesActive  = []string{taskActive}

	stepsChecked     = []string{stepLocal, stepCloud, stepReview}
	stepsBeforeMerge = []string{stepBranch, stepLocal, stepCloud, stepReview}

	waitsOfCards = []string{"", waitApproval, waitDecision} // no wait, or a card wait
	waitsKept    = []string{waitReviewer, waitConflict}
	workItems    = []string{itemAssign, itemChecks, itemReviewed, itemReview, itemResume, itemMerge}
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
	OnConfirm:   {{by: ByAdmin | ByPerson, states: statesNew, do: (*taskMove).create}},
	OnStart:     {{by: ByAI, states: statesNew, do: (*taskMove).start}},
	OnBranch:    {{by: ByHost, states: statesRunning, steps: []string{stepBranch}, do: (*taskMove).branch}},
	OnChecked:   {{by: ByHost, states: statesRunning, steps: stepsChecked, do: (*taskMove).checked}},
	OnSubmitted: {{by: ByHost, states: statesRunning, steps: stepsChecked, also: (*taskMove).checkedCommit, do: (*taskMove).submitted}},
	OnCloud:     {{by: ByHost, states: statesRunning, steps: []string{stepCloud}, also: (*taskMove).sameCommit, do: (*taskMove).cloud}},
	OnReview:    {{by: byReviewer, states: statesActive, steps: []string{stepReview}, also: (*taskMove).reviewable, do: (*taskMove).reviewed, orPaused: true}},
	OnMerged:    {{by: ByHost, states: statesActive, steps: []string{stepMerge}, also: (*taskMove).sameCommit, do: (*taskMove).merged}},
	OnReleased:  {{by: ByHome, states: []string{taskQueued}, do: (*taskMove).released}},
	OnConflict:  {{by: ByHost, states: statesActive, steps: []string{stepMerge}, also: (*taskMove).sameCommit, do: (*taskMove).conflicted}},
	OnFail:      {{by: ByHost, states: statesActive, steps: stepsBeforeMerge, do: (*taskMove).crashed, orPaused: true}},
	OnAssign: {
		{by: BySecretary, states: []string{taskQueued, taskActive, taskPaused}, steps: stepsBeforeMerge, do: (*taskMove).assign},
		{by: ByAdmin, states: statesOpen, steps: stepsBeforeMerge, do: (*taskMove).assign},
	},
	OnPause: {{by: BySecretary | ByAdmin, states: statesActive, steps: stepsBeforeMerge, do: (*taskMove).pauseByHand, orPaused: true}},
	OnResume: {
		{by: BySecretary, states: []string{taskPaused}, also: (*taskMove).notByRounds, do: (*taskMove).resume},
		{by: ByAdmin, states: []string{taskPaused, taskFailed}, do: (*taskMove).resume},
		{by: ByHost, states: []string{taskPaused}, also: (*taskMove).preempted, do: (*taskMove).activate},
	},
	OnCancel: {
		{by: ByAdmin, states: statesOpen, do: (*taskMove).cancel},
		{by: BySecretary, states: statesOpen, steps: stepsBeforeMerge, do: (*taskMove).cancel},
		{by: ByHost, states: statesOpen, steps: stepsBeforeMerge, also: (*taskMove).takeover, do: (*taskMove).cancel},
	},
	OnOrphaned:    {{by: ByHome, states: []string{taskQueued}, do: (*taskMove).orphaned}},
	OnPreempted:   {{by: ByHost, states: statesActive, steps: stepsBeforeMerge, do: (*taskMove).preempt, orPaused: true}},
	OnApproval:    {{by: byAssignee, states: statesActive, do: (*taskMove).approval, orPaused: true}},
	OnApprove:     {{by: ByAdmin | ByPerson, do: (*taskMove).decided}},
	OnDecision:    {{by: BySecretary | ByAI, states: statesOpen, do: (*taskMove).decision}},
	OnChoose:      {{by: ByAdmin | ByPerson, do: (*taskMove).chosen}},
	OnMention:     {{by: BySecretary | ByAI, do: (*taskMove).mention}},
	OnText:        {{by: ByAdmin | ByPerson, do: (*taskMove).text}},
	OnQuiet:       {{by: ByHome, states: statesActive, steps: stepsBeforeMerge, also: (*taskMove).idle, do: (*taskMove).quiet}},
	OnGroupResume: {{by: ByHome, do: (*taskMove).groupResume}},
}

// taskMove is one step in the making: the row, changed in place, and its effects.
type taskMove struct {
	t       Task
	e       Event
	x       Effects
	by      Sender        // e.By, with the roles e.Member holds in the task
	pending []PendingCard // the cards still pending once this event is applied
}

// Step applies an event to a task row. A refused event changes nothing and
// returns a TaskRefusal.
func Step(t Task, e Event) (Task, Effects, error) {
	m := taskMove{t: t, e: e, by: taskSender(t, e), pending: e.Pending}
	rule, err := m.rule()
	if err != nil {
		return t, Effects{}, err
	}
	rule.do(&m)
	// Roles (section 2): assignee and reviewer are never the same AI.
	if m.t.Assignee == m.t.Reviewer {
		return t, Effects{}, RefusedConflict
	}
	return m.t, m.x, nil
}

// taskSender is who sent the event, with the roles its member holds in the task.
func taskSender(t Task, e Event) Sender {
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

func (m *taskMove) sameCommit() bool { return m.e.Commit == m.t.Commit }

func (m *taskMove) checkedCommit() bool {
	c := m.t.Checks.Local
	return c != nil && c.OK && c.Commit == m.e.Commit
}

func (m *taskMove) reviewable() bool { return m.waitKind() != waitReviewer && m.sameCommit() }

func (m *taskMove) notByRounds() bool { return m.waitKind() != waitRounds }

func (m *taskMove) preempted() bool { return m.waitKind() == waitPreempted }

func (m *taskMove) takeover() bool { return m.t.Origin == "takeover" }

func (m *taskMove) idle() bool { return m.t.Wait == nil }

// moving says whether the rows that act only on an active task before step
// merge may act (section 2, the point of no return).
func (m *taskMove) moving() bool { return m.t.State == taskActive && m.t.Step != stepMerge }

// cardsDecide says whether a card may set or clear the wait (section 2).
func (m *taskMove) cardsDecide() bool {
	return m.moving() && slices.Contains(waitsOfCards, m.waitKind())
}

func (m *taskMove) waitKind() string {
	if m.t.Wait == nil {
		return ""
	}
	return m.t.Wait.Kind
}

// The building blocks.

func (m *taskMove) waitFor(kind string) *proto.TaskWait {
	return &proto.TaskWait{Kind: kind, Since: m.e.Now}
}

// cardWait is the wait of a task that waits for nothing else: its oldest
// pending approval card, else its oldest pending decision card, else none.
func (m *taskMove) cardWait() *proto.TaskWait {
	for _, kind := range []string{waitApproval, waitDecision} {
		if i := slices.IndexFunc(m.pending, func(c PendingCard) bool { return c.Kind == kind }); i >= 0 {
			return taskCardWait(m.pending[i])
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
	if m.t.Kept != "" {
		return m.waitFor(m.t.Kept)
	}
	return m.cardWait()
}

// settle ends a wait of one of these kinds: an active task's wait is
// recomputed, a paused task no longer keeps it.
func (m *taskMove) settle(kinds ...string) {
	if slices.Contains(kinds, m.t.Kept) {
		m.t.Kept = ""
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
// the reviewer gets the commit unless it is queued for them already.
func (m *taskMove) activate() {
	m.t.State, m.t.Wait, m.t.Kept = taskActive, m.restored(), ""
	if m.t.Step == stepReview {
		m.askReview(true)
	}
}

// askReview gives the commit to the reviewer. Without one an active task
// waits for a reviewer; a paused one does when it becomes active.
func (m *taskMove) askReview(ifNone bool) {
	if m.t.Reviewer != "" {
		m.queue(TaskItem{Why: itemReview, AI: m.t.Reviewer, Commit: m.t.Commit, IfNone: ifNone})
		return
	}
	if m.t.State == taskActive {
		m.t.Wait = m.waitFor(waitReviewer)
	}
}

// pause is the pause of rows 12, 16 and 21: it keeps a reviewer or conflict wait it replaces.
func (m *taskMove) pause(kind string) {
	if k := m.waitKind(); slices.Contains(waitsKept, k) {
		m.t.Kept = k
	}
	m.t.State, m.t.Wait = taskPaused, m.waitFor(kind)
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
	m.t.State, m.t.Wait, m.t.Kept = taskFailed, nil, ""
	m.x.System, m.x.Reason = "task_failed", reason
	m.notice("task_failed")
}

// passed is row 6 for checks that passed, and row 5 without a remote.
func (m *taskMove) passed() {
	m.t.Step, m.t.Failures = stepReview, 0
	if m.t.Reviewer == "" {
		m.notice("no_reviewer")
	}
	m.askReview(false)
}

// end makes the task final (rows 8 and 15).
func (m *taskMove) end(state string) {
	now := m.e.Now
	m.t.State, m.t.Wait, m.t.Kept, m.t.Stats.Ended = state, nil, "", &now
	m.x.Cards = "expired"
}

// The rows.

// create is row 1: queued, and released at once unless a dependency is not merged.
func (m *taskMove) create() {
	m.t.State, m.t.Step, m.t.Wait = taskQueued, stepBranch, m.waitFor(waitDepends)
	m.released()
}

// start is row 2.
func (m *taskMove) start() {
	m.t.Step, m.t.Assignee, m.t.Reviewer, m.t.Foundation = stepBranch, m.e.Member, "", false
	m.activate()
	if !m.takeover() {
		m.queue(TaskItem{Why: itemAssign, AI: m.t.Assignee})
	}
	m.notice("task_started")
}

// branch is row 3.
func (m *taskMove) branch() { m.t.Step = stepLocal }

// checked is row 4.
func (m *taskMove) checked() {
	m.t.Checks.Local = &proto.Check{OK: m.e.OK, At: m.e.Now, Commit: m.e.Commit}
}

// submitted is row 5. Any review item still queued is about the old commit.
func (m *taskMove) submitted() {
	m.t.Commit = m.e.Commit
	m.retire(ItemFilter{Why: itemReview})
	m.settle(waitsKept...)
	if m.e.Remote {
		m.t.Step = stepCloud
		return
	}
	m.passed()
}

// cloud is row 6.
func (m *taskMove) cloud() {
	m.t.Checks.Cloud = &proto.Check{OK: m.e.OK, At: m.e.Now, Commit: m.e.Commit}
	if m.e.OK {
		m.passed()
		return
	}
	m.t.Step = stepLocal
	m.strike(TaskItem{Why: itemChecks, AI: m.t.Assignee})
}

// reviewed is row 7.
func (m *taskMove) reviewed() {
	r := proto.Review{Score: m.e.Score, OK: m.e.OK, By: m.e.Member, Commit: m.e.Commit, At: m.e.Now}
	if m.t.Review != nil {
		r.Round = m.t.Review.Round
	}
	r.Round++
	m.t.Review = &r
	if m.e.OK {
		m.t.Step = stepMerge
		m.queue(TaskItem{Why: itemMerge, AI: m.t.Assignee, Commit: m.t.Commit})
		return
	}
	m.t.ReviewFailed++
	if m.t.ReviewFailed > m.e.Limits.ReviewRounds {
		m.stop("review_rounds")
		return
	}
	m.t.Step = stepLocal
	m.queue(TaskItem{Why: itemReviewed, AI: m.t.Assignee})
}

// merged is row 8 for the merged task.
func (m *taskMove) merged() {
	m.end(taskMerged)
	m.x.System = "task_merged"
	m.notice("task_merged")
}

// released is row 8 for a task that waited for the merged one.
func (m *taskMove) released() {
	if m.e.Blocked {
		return
	}
	m.activate()
	m.queue(TaskItem{Why: itemAssign, AI: m.t.Assignee})
}

// conflicted is row 9.
func (m *taskMove) conflicted() {
	m.t.Step, m.t.Wait = stepLocal, m.waitFor(waitConflict)
	m.notice("conflict")
}

// crashed is row 10.
func (m *taskMove) crashed() { m.strike(TaskItem{Why: itemNotice, Code: m.e.Reason}) }

// assign is row 11.
func (m *taskMove) assign() {
	was := *m
	m.setAssignee()
	m.setReviewer()
	m.settle(waitConflict)
	if was.t.State == taskFailed {
		m.t.Failures, m.t.ReviewFailed = 0, 0
		m.activate()
	}
	if m.t.Assignee != was.t.Assignee || was.t.State == taskFailed || was.waitKind() == waitConflict {
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
	m.x.Cards = "reassigned"
	m.retire(ItemFilter{AI: old})
	m.forget(func(c PendingCard) bool { return c.Kind == waitApproval })
	m.settle(waitApproval)
}

// setReviewer names another reviewer: the old one's queued review is retired.
// At step review a reviewer wait ends and the new reviewer gets the commit;
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

// pauseByHand is row 12.
func (m *taskMove) pauseByHand() { m.pause(waitManual) }

// resume is row 13; row 13b is activate alone.
func (m *taskMove) resume() {
	if m.t.State == taskFailed {
		m.t.Failures, m.t.ReviewFailed = 0, 0
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
	if m.moving() && m.t.Wait == nil {
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
	if m.t.Thread.Rounds < m.e.Limits.Rounds || !m.moving() {
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
	if m.moving() && m.t.Wait == nil {
		m.queue(TaskItem{Why: itemResume, AI: m.t.Assignee})
	}
	if m.t.Step == stepReview && m.t.Reviewer != "" {
		m.queue(TaskItem{Why: itemReview, AI: m.t.Reviewer, Commit: m.t.Commit, IfNone: true})
	}
}
