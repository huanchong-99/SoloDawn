package home

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Task commands: start, assign, pause, resume, cancel, fail and preempted
// (planning/contracts/commands.md section 1.5;
// planning/contracts/task-machine.md rows 2 and 10 to 16), and the rule that
// one folder takes one task at a time (section 2 of that file).

func init() {
	Commands.Add("task/start", tskStart)
	Commands.Add("task/assign", tskAssign)
	Commands.Add("task/pause", tskPause)
	Commands.Add("task/resume", tskResume)
	Commands.Add("task/cancel", tskCancel)
	Commands.Add("task/fail", tskFail)
	Commands.Add("task/preempted", tskPreempted)
}

const tskPage = 50

var tskReasons = []string{"crash", "timeout"}

// tskStart creates a direct task for a working AI through row 2 of the task
// machine, queued while the AI's folder is busy, writes its task message in
// the main line, and answers with its group task number.
func tskStart(s *State, c Command) (Result, *Refusal) {
	if !s.briefWorking(c.Who.AI) {
		return Result{}, &Refusal{"forbidden", "only a working AI starts a task"}
	}
	var p proto.TaskStart
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Origin != "direct" {
		return Result{}, &Refusal{"bad_request", "task start origin must be direct"}
	}
	return s.tskCreate(c, p)
}

func (s *State) tskCreate(c Command, p proto.TaskStart) (Result, *Refusal) {
	id := string(p.ID)
	_, hasTask := s.Tasks[id]
	_, hasMsg := c.Disk.Msg(id)
	if hasTask || hasMsg {
		return Result{}, &Refusal{"conflict", "the id of the task is taken"}
	}
	task := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id}, Group: s.ID, Num: s.Num + 1,
		Title: p.Title, Origin: p.Origin, Plan: p.Plan,
		Pass: s.Group.Pass, Depends: []string{}, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
	}
	task, fx, _ := Step(task, Event{Kind: OnStart, By: ByAI, Member: c.Who.AI, Now: c.Now, Busy: s.tskBusy(s.Tasks, id, c.Who.AI)})
	var re string
	if p.Re != nil {
		re = string(*p.Re)
	}
	items := tskItemCtx{taskID: id, secretary: s.Group.Secretary, assignMsg: re, noticeMsg: id}.items(fx.Items)
	rows := []Row{RowOf(task), RowOf(briefTaskMsg(s, c, id))}
	return Result{Rows: rows, Items: items, Answer: store.Outcome{Num: task.Num}}, nil
}

// tskAssign sets a task's assignee, reviewer, or both through row 11 of the
// task machine, and looks at the queued tasks: another assignee may free a
// folder, or take a queued task to a folder that is free.
func tskAssign(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskAssign
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Assignee == "" && p.Reviewer == "" {
		return Result{}, &Refusal{"bad_request", "task assign requires at least one of assignee and reviewer"}
	}
	busy := s.tskBusy(s.Tasks, string(p.ID), string(p.Assignee))
	task, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnAssign, Assignee: string(p.Assignee), Reviewer: string(p.Reviewer), Busy: busy})
	if refusal != nil {
		return Result{}, refusal
	}
	if !s.tskRolesOK(p) {
		return Result{}, &Refusal{"conflict", "assignee and reviewer must be working AIs of the group"}
	}
	return s.tskRelease(c, s.tskAssigned(c, task, fx)), nil
}

func (s *State) tskRolesOK(p proto.TaskAssign) bool {
	if p.Assignee != "" && !s.briefWorking(string(p.Assignee)) {
		return false
	}
	return p.Reviewer == "" || s.briefWorking(string(p.Reviewer))
}

func (s *State) tskAssigned(c Command, task proto.TaskRow, fx Effects) Result {
	retire, kept := tskSettleQueue(c.Disk.Queued(), task.ID, fx.Retire)
	rows := append([]Row{RowOf(task)}, tskCardRows(c, task.ID, fx.Cards)...)
	items := tskItemCtx{
		taskID: task.ID, secretary: s.Group.Secretary, assignMsg: tskLatestReview(c.Disk, task.ID), kept: kept,
	}.items(fx.Items)
	return Result{Rows: rows, Items: items, Retire: retire}
}

// tskRelease is "Starting a queued task" (task-machine.md section 2), after
// res made a task final or gave one another assignee. It judges the queued
// tasks in the order of num, each against the tasks as res and the ones
// before it leave them, so that a task it starts keeps its folder from the
// ones after it. It adds to res the rows that changed and the assign item of
// each task that started, which names its task message.
func (s *State) tskRelease(c Command, res Result) Result {
	tasks := tskAfter(s.Tasks, res.Rows)
	queued := slices.DeleteFunc(slices.Collect(maps.Values(tasks)), func(t proto.TaskRow) bool { return t.State != taskQueued })
	slices.SortFunc(queued, func(a, b proto.TaskRow) int { return a.Num - b.Num })
	for _, t := range queued {
		ev := Event{Kind: OnReleased, By: ByHome, Now: c.Now, Pending: tskPending(c.Disk, t.ID),
			Blocked: tskBlocked(tasks, t.Depends), Busy: s.tskBusy(tasks, t.ID, t.Assignee)}
		moved, fx, _ := Step(t, ev)
		if moved.Wait != t.Wait {
			tasks[t.ID] = moved
			cftPutTask(&res, moved)
			res.Items = append(res.Items, tskItemCtx{taskID: t.ID, assignMsg: t.ID}.items(fx.Items)...)
		}
	}
	return res
}

// tskAfter is the group's tasks as rows leave them.
func tskAfter(tasks map[string]proto.TaskRow, rows []Row) map[string]proto.TaskRow {
	after := maps.Clone(tasks)
	for _, row := range rows {
		if t, ok := row.Value().(proto.TaskRow); ok {
			after[t.ID] = t
		}
	}
	return after
}

// tskBlocked says whether a dependency is not done.
func tskBlocked(tasks map[string]proto.TaskRow, depends []string) bool {
	return slices.ContainsFunc(depends, func(id string) bool { return tasks[id].State != taskDone })
}

// tskBusy says whether the folder of that AI, the pair of its host and its
// workspace, is that of the assignee of a started task other than the one
// with that id.
func (s *State) tskBusy(tasks map[string]proto.TaskRow, id, ai string) bool {
	for _, t := range tasks {
		if t.ID != id && slices.Contains(statesStarted, t.State) && s.tskFolder(t.Assignee) == s.tskFolder(ai) {
			return true
		}
	}
	return false
}

func (s *State) tskFolder(ai string) [2]string {
	return [2]string{s.AIs[ai].Host, s.AIs[ai].Workspace}
}

// tskPause pauses an active task through row 12 of the task machine.
func tskPause(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskCommand
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, _, refusal := s.tskStep(c, p.ID, Event{Kind: OnPause})
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: []Row{RowOf(task)}}, nil
}

// tskResume activates a paused or failed task through row 13 or row 13b of the
// task machine.
func tskResume(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskCommand
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnResume})
	if refusal != nil {
		return Result{}, refusal
	}
	items := tskItemCtx{taskID: task.ID, kept: c.Disk.Queued()}.items(fx.Items)
	return Result{Rows: []Row{RowOf(task)}, Items: items}, nil
}

// tskCancel cancels an unfinished task through row 15 of the task machine,
// notifies the secretary of each queued task that depended on it, and looks
// at the queued tasks: the folder may be free now.
func tskCancel(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskCommand
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnCancel})
	if refusal != nil {
		return Result{}, refusal
	}
	retire, _ := tskSettleQueue(c.Disk.Queued(), task.ID, fx.Retire)
	rows := append([]Row{RowOf(task)}, tskCardRows(c, task.ID, fx.Cards)...)
	return s.tskRelease(c, Result{Rows: rows, Items: s.tskOrphaned(task.ID), Retire: retire}), nil
}

func (s *State) tskOrphaned(cancelled string) []Item {
	var items []Item
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
		dep := s.Tasks[id]
		if slices.Contains(dep.Depends, cancelled) {
			_, fx, _ := Step(dep, Event{Kind: OnOrphaned, By: ByHome})
			items = append(items, tskItemCtx{taskID: dep.ID, secretary: s.Group.Secretary}.items(fx.Items)...)
		}
	}
	return items
}

// tskFail counts a crashed or timed-out turn through row 10 of the task
// machine, stopping the task by row 14 when the group's failure limit is reached.
func tskFail(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskFail
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if !slices.Contains(tskReasons, p.Reason) {
		return Result{}, &Refusal{"bad_request", "task fail reason must be crash or timeout"}
	}
	task, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnFail, Reason: p.Reason})
	if refusal != nil {
		return Result{}, refusal
	}
	return s.tskFailed(c, p.Detail, task, fx)
}

func (s *State) tskFailed(c Command, detail string, task proto.TaskRow, fx Effects) (Result, *Refusal) {
	if fx.System == "" {
		items := tskItemCtx{taskID: task.ID, secretary: s.Group.Secretary, detail: detail}.items(fx.Items)
		return Result{Rows: []Row{RowOf(task)}, Items: items}, nil
	}
	if refusal := tskNonceFree(c); refusal != nil {
		return Result{}, refusal
	}
	msg := s.tskFailedSystem(c, string(c.Nonce), &task, fx)
	items := tskItemCtx{taskID: task.ID, secretary: s.Group.Secretary, noticeMsg: msg.ID}.items(fx.Items)
	return Result{Rows: []Row{RowOf(task), RowOf(msg)}, Items: items}, nil
}

// tskPreempted pauses an active task when a queue jump in another session
// interrupted its turn, writing the preempted system message in its thread.
func tskPreempted(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskPreempted
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnPreempted})
	if refusal != nil {
		return Result{}, refusal
	}
	jump, refusal := tskJumpAuthor(c.Disk, string(p.Msg))
	if refusal != nil {
		return Result{}, refusal
	}
	if refusal := tskNonceFree(c); refusal != nil {
		return Result{}, refusal
	}
	body := proto.PreemptedBody{SystemCode: proto.SystemCode{Code: fx.System}, Task: task.ID, By: jump}
	msg := s.tskThreadSystem(c, string(c.Nonce), "system", &task, []string{}, body)
	return Result{Rows: []Row{RowOf(task), RowOf(msg)}}, nil
}

func tskJumpAuthor(d Disk, id string) (string, *Refusal) {
	msg, ok := d.Msg(id)
	if !ok {
		return "", &Refusal{"not_found", "the message named by task preempted does not exist"}
	}
	return msg.By, nil
}

func tskNonceFree(c Command) *Refusal {
	if _, taken := c.Disk.Msg(string(c.Nonce)); taken {
		return &Refusal{"conflict", "a message with the id of the command exists"}
	}
	return nil
}

func (s *State) tskStep(c Command, id proto.ID, ev Event) (proto.TaskRow, Effects, *Refusal) {
	task, ok := s.Tasks[string(id)]
	if !ok {
		return proto.TaskRow{}, Effects{}, &Refusal{"not_found", "the group has no such task"}
	}
	ev.By = s.tskSender(c.Who, task.Assignee)
	ev.Member = c.Who.Member()
	ev.Now = c.Now
	ev.Limits = s.Group.Limits
	ev.Pending = tskPending(c.Disk, task.ID)
	ev.Stronger = s.profileHasStronger(task, ev)
	moved, fx, err := Step(task, ev)
	if err != nil {
		return proto.TaskRow{}, Effects{}, profileStepRefusal(task, ev, err)
	}
	return moved, fx, nil
}

func (s *State) tskSender(w Who, assignee string) Sender {
	switch {
	case s.Secretary(w):
		return BySecretary
	case w.AI != "":
		return ByAI
	case s.Hosts(w, assignee):
		return ByHost
	case s.Admin(w):
		return ByAdmin
	default:
		return ByPerson
	}
}

func (s *State) tskThreadSystem(c Command, id, kind string, task *proto.TaskRow, mentions []string, body any) proto.MsgRow {
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id}, Group: s.ID,
		By: "system", Kind: kind, Timestamp: c.Now, Mentions: mentions, Body: briefJSON(body),
	}
	if task != nil {
		tid := task.ID
		msg.Thread = &tid
		*task = postAdvanceThread(*task, msg)
	}
	return msg
}

func (s *State) tskFailedSystem(c Command, id string, task *proto.TaskRow, fx Effects) proto.MsgRow {
	body := proto.TaskFailedBody{SystemCode: proto.SystemCode{Code: fx.System}, Task: task.ID, Reason: fx.Reason}
	return s.tskThreadSystem(c, id, "system", task, s.tskFailedMentions(c.Disk, *task), body)
}

func (s *State) tskFailedMentions(d Disk, task proto.TaskRow) []string {
	out := []string{}
	if req := tskRequester(d, task.Brief); s.memExists(req) {
		out = append(out, req)
	}
	if owner := s.human(s.AIs[task.Assignee].User); owner != "" && !slices.Contains(out, owner) {
		out = append(out, owner)
	}
	return out
}

func tskRequester(d Disk, brief string) string {
	card, _ := d.Msg(brief)
	var b proto.BriefBody
	json.Unmarshal(card.Body, &b)
	return b.Requester
}

type tskCardBody struct {
	Task      string   `json:"task"`
	Approvers []string `json:"approvers"`
	To        []string `json:"to"`
}

func tskReadCard(card proto.MsgRow) tskCardBody {
	var b tskCardBody
	json.Unmarshal(card.Body, &b)
	return b
}

func tskPending(d Disk, taskID string) []PendingCard {
	var out []PendingCard
	for _, card := range d.Pending() {
		b := tskReadCard(card)
		if b.Task != taskID {
			continue
		}
		who := b.To
		if card.Kind == waitApproval {
			who = b.Approvers
		}
		out = append(out, PendingCard{Kind: card.Kind, Msg: card.ID, Who: who, Since: card.Timestamp})
	}
	return out
}

// tskCardRows ends the pending cards of a task: all of them as expired, or its
// approval cards as reassigned, by the sender of the command.
func tskCardRows(c Command, taskID, status string) []Row {
	if status == "" {
		return nil
	}
	state := proto.CardState{Status: status, At: &c.Now}
	if status == "reassigned" {
		by := c.Who.Member()
		state.By = &by
	}
	var rows []Row
	for _, card := range c.Disk.Pending() {
		if tskCardChanges(card, taskID, status) {
			card.State = &state
			rows = append(rows, RowOf(card))
		}
	}
	return rows
}

func tskCardChanges(card proto.MsgRow, taskID, status string) bool {
	if tskReadCard(card).Task != taskID {
		return false
	}
	return status == "expired" || card.Kind == waitApproval
}

// tskLatestReview is the latest review message of a task, or "".
func tskLatestReview(d Disk, taskID string) string {
	var before int64
	for {
		page := d.Thread(&taskID, before, tskPage)
		i := slices.IndexFunc(page, func(m proto.MsgRow) bool { return m.Kind == revKindReview })
		if i >= 0 {
			return page[i].ID
		}
		if len(page) < tskPage {
			return ""
		}
		before = page[len(page)-1].Pos
	}
}

func tskSettleQueue(queued []store.Item, taskID string, filters []ItemFilter) (retire []int64, kept []store.Item) {
	for _, q := range queued {
		if tskForTask(q, taskID) && tskAnyFilter(q, filters) {
			retire = append(retire, q.ID)
			continue
		}
		kept = append(kept, q)
	}
	return retire, kept
}

func tskForTask(q store.Item, taskID string) bool {
	return q.Task != nil && *q.Task == taskID
}

func tskAnyFilter(q store.Item, filters []ItemFilter) bool {
	for _, f := range filters {
		if tskMatchField(f.AI, q.AI) && tskMatchField(f.Why, q.Why) {
			return true
		}
	}
	return false
}

func tskMatchField(want, got string) bool {
	return want == "" || want == got
}

func tskHasQueued(kept []store.Item, taskID string, it TaskItem) bool {
	for _, q := range kept {
		if tskSameReview(q, taskID, it) {
			return true
		}
	}
	return false
}

func tskSameReview(q store.Item, taskID string, it TaskItem) bool {
	if !tskForTask(q, taskID) || q.Why != it.Why {
		return false
	}
	return q.AI == it.AI && postTaskID(q.Msg) == it.Report
}

type tskItemCtx struct {
	taskID      string
	secretary   string
	assignMsg   string
	feedbackMsg string
	noticeMsg   string
	detail      string
	kept        []store.Item
}

func (x tskItemCtx) items(raw []TaskItem) []Item {
	var out []Item
	for _, it := range raw {
		if item, ok := x.one(it); ok {
			out = append(out, item)
		}
	}
	return out
}

func (x tskItemCtx) one(it TaskItem) (Item, bool) {
	if it.IfNone && tskHasQueued(x.kept, x.taskID, it) {
		return Item{}, false
	}
	if it.Why == itemNotice {
		return x.notice(it)
	}
	return x.work(it), true
}

func (x tskItemCtx) work(it TaskItem) Item {
	item := Item{AI: it.AI, Why: it.Why, Task: x.taskID, Msg: it.Report}
	switch it.Why {
	case itemAssign:
		item.Msg = x.assignMsg
	case itemReviewed:
		item.Msg = x.feedbackMsg
	}
	return item
}

func (x tskItemCtx) notice(it TaskItem) (Item, bool) {
	if x.secretary == "" {
		return Item{}, false
	}
	return Item{AI: x.secretary, Why: itemNotice, Task: x.taskID, Msg: x.noticeMsg, Code: it.Code, Detail: x.detail}, true
}
