package home

import (
	"encoding/json"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Merge outcomes and completion summaries: task merged, task conflict and post
// of kind summary (planning/contracts/commands.md 1.3 and 1.5;
// planning/contracts/messages.md 2.5, 2.8 and 2.11;
// planning/contracts/task-machine.md rows 8 and 9).

const (
	mrgKindChecks   = "checks"
	mrgKindSummary  = "summary"
	mrgKindSystem   = "system"
	mrgStageMerge   = "merge"
	mrgAuthorSystem = "system"
	mrgMinuteMS     = 60_000
	mrgPreviewRunes = 200
)

func init() {
	Commands.Add("task/merged", mrgMerged)
	Commands.Add("task/conflict", mrgConflict)
	Commands.Add("post/summary", mrgSummary)
}

// mrgMerged records a completed merge through row 8 of the task machine,
// expires the task's pending cards, writes the task_merged system message,
// notifies the secretary, and releases dependent queued tasks whose
// dependencies are now all merged.
func mrgMerged(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskMerged
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if !mrgMergedValid(p) {
		return Result{}, &Refusal{Code: "bad_request", Msg: "task merged requires commit, merge_commit and non-negative stats"}
	}
	moved, fx, refusal := s.mrgStepHost(c, p.ID, OnMerged, p.Commit)
	if refusal != nil {
		return Result{}, refusal
	}
	return s.mrgFinishMerged(c, p, moved, fx)
}

func mrgMergedValid(p proto.TaskMerged) bool {
	return p.Commit != "" && p.MergeCommit != "" && p.Stats.Added >= 0 && p.Stats.Removed >= 0
}

func (s *State) mrgFinishMerged(c Command, p proto.TaskMerged, moved proto.TaskRow, fx Effects) (Result, *Refusal) {
	body := proto.TaskMergedBody{
		SystemCode:  proto.SystemCode{Code: fx.System},
		Task:        moved.ID,
		Commit:      p.Commit,
		MergeCommit: p.MergeCommit,
	}
	sysMsg, refusal := s.mrgThreadMsg(c, moved.ID, mrgKindSystem, body)
	if refusal != nil {
		return Result{}, refusal
	}
	moved.Stats.Added, moved.Stats.Removed = p.Stats.Added, p.Stats.Removed
	moved = mrgAdvanceThread(moved, sysMsg)
	rows := append([]Row{RowOf(moved)}, mrgExpireCards(c.Disk, moved.ID, c.Now)...)
	rows = append(rows, RowOf(sysMsg))
	relRows, relItems := s.mrgReleaseDeps(c.Disk, moved.ID, c.Now)
	items := append(s.mrgNotice(fx.Items[0].Code, moved.ID, sysMsg.ID), relItems...)
	return Result{Rows: append(rows, relRows...), Items: items}, nil
}

// mrgConflict records a failed merge attempt through row 9 of the task machine,
// writes a checks message at stage merge in the task's thread, and notifies the
// secretary.
func mrgConflict(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskConflict
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Commit == "" {
		return Result{}, &Refusal{Code: "bad_request", Msg: "task conflict requires a commit"}
	}
	moved, fx, refusal := s.mrgStepHost(c, p.ID, OnConflict, p.Commit)
	if refusal != nil {
		return Result{}, refusal
	}
	checksMsg, refusal := s.mrgThreadMsg(c, moved.ID, mrgKindChecks, mrgConflictBody(p.Commit, p.Detail))
	if refusal != nil {
		return Result{}, refusal
	}
	moved = mrgAdvanceThread(moved, checksMsg)
	items := s.mrgNotice(fx.Items[0].Code, moved.ID, checksMsg.ID)
	return Result{Rows: []Row{RowOf(moved), RowOf(checksMsg)}, Items: items}, nil
}

func mrgConflictBody(commit, detail string) proto.ChecksBody {
	var itemDetail *string
	if detail != "" {
		itemDetail = &detail
	}
	return proto.ChecksBody{
		Stage:  mrgStageMerge,
		OK:     false,
		Commit: commit,
		Items:  []proto.CheckItem{{Name: mrgStageMerge, OK: false, Detail: itemDetail}},
	}
}

// mrgSummary writes the secretary's completion summary for a merged task in its
// thread and in the main line, filling the summary body from the task row.
func mrgSummary(s *State, c Command) (Result, *Refusal) {
	if !s.Secretary(c.Who) {
		return Result{}, &Refusal{Code: "forbidden", Msg: "only the secretary posts a summary"}
	}
	var p proto.Post[proto.EmptyBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, mentions, refusal := s.mrgSummaryTarget(c, p)
	if refusal != nil {
		return Result{}, refusal
	}
	msg := s.mrgSummaryMsg(c, p, task, mentions)
	return Result{Rows: []Row{RowOf(msg), RowOf(mrgAdvanceThread(task, msg))}}, nil
}

func (s *State) mrgSummaryTarget(c Command, p proto.Post[proto.EmptyBody]) (proto.TaskRow, []string, *Refusal) {
	mentions, unique := mrgStrings(p.Mentions)
	if p.Thread == nil || !unique {
		return proto.TaskRow{}, nil, &Refusal{Code: "bad_request", Msg: "summary post requires a task thread and distinct mentions"}
	}
	task, exists := s.Tasks[string(*p.Thread)]
	if !exists {
		return proto.TaskRow{}, nil, &Refusal{Code: "not_found", Msg: "summary names no task"}
	}
	if refusal := s.mrgSummaryReady(c, task, mentions); refusal != nil {
		return proto.TaskRow{}, nil, refusal
	}
	return task, mentions, nil
}

func (s *State) mrgSummaryReady(c Command, task proto.TaskRow, mentions []string) *Refusal {
	if task.State != taskMerged {
		return &Refusal{Code: "conflict", Msg: "summary requires a merged task"}
	}
	for _, m := range mentions {
		if !s.mrgMember(m) {
			return &Refusal{Code: "conflict", Msg: "summary mentions someone outside the group"}
		}
	}
	if _, taken := c.Disk.Msg(string(c.Nonce)); taken {
		return &Refusal{Code: "conflict", Msg: "a message with the id of the summary exists"}
	}
	return nil
}

func (s *State) mrgSummaryMsg(c Command, p proto.Post[proto.EmptyBody], task proto.TaskRow, mentions []string) proto.MsgRow {
	tid := task.ID
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)},
		Group:     s.ID,
		Thread:    &tid,
		Main:      true,
		By:        c.Who.Member(),
		Kind:      mrgKindSummary,
		Timestamp: c.Timestamp,
		Text:      p.Text,
		Mentions:  mentions,
		Re:        (*string)(p.Re),
		Body:      briefJSON(mrgSummaryBody(task)),
		Env:       &c.Env,
	}
}

func mrgSummaryBody(task proto.TaskRow) proto.SummaryBody {
	var pr *int
	if task.PR > 0 {
		num := task.PR
		pr = &num
	}
	return proto.SummaryBody{
		Task:    task.ID,
		Score:   task.Review.Score,
		Rounds:  task.Review.Round,
		Minutes: max(0, int((*task.Stats.Ended-task.Stats.Started)/mrgMinuteMS)),
		Added:   task.Stats.Added,
		Removed: task.Stats.Removed,
		PR:      pr,
	}
}

func (s *State) mrgStepHost(c Command, taskID proto.ID, kind EventKind, commit string) (proto.TaskRow, Effects, *Refusal) {
	current, exists := s.Tasks[string(taskID)]
	if !exists {
		return proto.TaskRow{}, Effects{}, &Refusal{Code: "not_found", Msg: "task not found in group"}
	}
	ev := Event{
		Kind:   kind,
		By:     s.mrgHost(c.Who, current.Assignee),
		Now:    c.Now,
		Limits: s.Group.Limits,
		Commit: commit,
	}
	next, fx, stepErr := Step(current, ev)
	if stepErr != nil {
		return proto.TaskRow{}, Effects{}, &Refusal{Code: stepErr.Error(), Msg: "task transition refused"}
	}
	return next, fx, nil
}

func (s *State) mrgHost(w Who, assignee string) Sender {
	if s.Hosts(w, assignee) {
		return ByHost
	}
	return 0
}

func (s *State) mrgThreadMsg(c Command, taskID, kind string, body any) (proto.MsgRow, *Refusal) {
	msgID := string(c.Nonce)
	if _, taken := c.Disk.Msg(msgID); taken {
		return proto.MsgRow{}, &Refusal{Code: "conflict", Msg: "message id already exists"}
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID},
		Group:     s.ID,
		Thread:    &taskID,
		By:        mrgAuthorSystem,
		Kind:      kind,
		Timestamp: c.Now,
		Mentions:  []string{},
		Body:      briefJSON(body),
	}, nil
}

func mrgAdvanceThread(task proto.TaskRow, msg proto.MsgRow) proto.TaskRow {
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{
		By:        msg.By,
		Kind:      msg.Kind,
		Text:      mrgPreview(msg.Text),
		Timestamp: msg.Timestamp,
	}
	return task
}

func mrgPreview(text string) string {
	chars := []rune(text)
	return string(chars[:min(len(chars), mrgPreviewRunes)])
}

func mrgExpireCards(d Disk, taskID string, at int64) []Row {
	var out []Row
	for _, card := range d.Pending() {
		if card.Thread != nil && *card.Thread == taskID {
			when := at
			card.State = &proto.CardState{Status: "expired", At: &when}
			out = append(out, RowOf(card))
		}
	}
	return out
}

func (s *State) mrgNotice(code, taskID, msgID string) []Item {
	if s.Group.Secretary == "" {
		return nil
	}
	return []Item{{
		AI:   s.Group.Secretary,
		Why:  itemNotice,
		Task: taskID,
		Msg:  msgID,
		Code: code,
	}}
}

func (s *State) mrgReleaseDeps(d Disk, mergedID string, now int64) ([]Row, []Item) {
	var rows []Row
	var items []Item
	for _, id := range mrgSortedTaskIDs(s.Tasks) {
		dep := s.Tasks[id]
		if dep.State != taskQueued || !slices.Contains(dep.Depends, mergedID) {
			continue
		}
		if row, rel, ok := s.mrgReleaseOne(d, dep, mergedID, now); ok {
			rows = append(rows, row)
			items = append(items, rel...)
		}
	}
	return rows, items
}

func mrgSortedTaskIDs(tasks map[string]proto.TaskRow) []string {
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (s *State) mrgReleaseOne(d Disk, dep proto.TaskRow, mergedID string, now int64) (Row, []Item, bool) {
	ev := Event{
		Kind:    OnReleased,
		By:      ByHome,
		Now:     now,
		Limits:  s.Group.Limits,
		Blocked: s.mrgBlocked(dep.Depends, mergedID),
		Pending: mrgPending(d, dep.ID),
	}
	released, fx, _ := Step(dep, ev)
	if released.State != taskActive {
		return nil, nil, false
	}
	released.Stats.Started = now
	return RowOf(released), mrgAssignItems(d, released, fx.Items), true
}

func (s *State) mrgBlocked(depends []string, mergedID string) bool {
	for _, depID := range depends {
		if depID != mergedID && s.Tasks[depID].State != taskMerged {
			return true
		}
	}
	return false
}

func mrgPending(d Disk, taskID string) []PendingCard {
	var list []PendingCard
	for _, m := range d.Pending() {
		if m.Thread != nil && *m.Thread == taskID {
			list = append(list, PendingCard{
				Kind:  m.Kind,
				Msg:   m.ID,
				Who:   mrgCardWho(m),
				Since: m.Timestamp,
			})
		}
	}
	return list
}

func mrgCardWho(m proto.MsgRow) []string {
	if m.Kind == waitApproval {
		var b proto.ApprovalBody
		json.Unmarshal(m.Body, &b)
		return b.Approvers
	}
	var b proto.DecisionBody
	json.Unmarshal(m.Body, &b)
	return b.To
}

func mrgAssignItems(d Disk, task proto.TaskRow, raw []TaskItem) []Item {
	msgID := mrgTaskMsg(d, task.ID)
	out := make([]Item, len(raw))
	for i, it := range raw {
		out[i] = Item{
			AI:   it.AI,
			Why:  it.Why,
			Task: task.ID,
			Msg:  msgID,
		}
	}
	return out
}

func mrgTaskMsg(d Disk, taskID string) string {
	if m, _ := d.Msg(taskID); m.Kind == "task" {
		return taskID
	}
	return ""
}

func mrgStrings(ids []proto.ID) ([]string, bool) {
	out := make([]string, len(ids))
	for i, id := range ids {
		if slices.Contains(ids[:i], id) {
			return nil, false
		}
		out[i] = string(id)
	}
	return out, true
}

func (s *State) mrgMember(id string) bool {
	_, human := s.People[id]
	_, ai := s.AIs[id]
	return human || ai
}
