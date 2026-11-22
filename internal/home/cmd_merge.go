package home

import (
	"maps"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Merge outcomes and completion summaries: task merged, task conflict and post
// of kind summary (planning/contracts/commands.md 1.3 and 1.5;
// planning/contracts/messages.md 2.5, 2.8 and 2.11;
// planning/contracts/task-machine.md rows 8 and 9).

const (
	mrgKindChecks  = "checks"
	mrgKindSummary = "summary"
	mrgKindSystem  = "system"
	mrgStageMerge  = "merge"
	mrgMinuteMS    = 60_000
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
	moved, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnMerged, Commit: p.Commit})
	if refusal != nil {
		return Result{}, refusal
	}
	return s.mrgFinishMerged(c, p, moved, fx), nil
}

func (s *State) mrgFinishMerged(c Command, p proto.TaskMerged, moved proto.TaskRow, fx Effects) Result {
	body := proto.TaskMergedBody{
		SystemCode:  proto.SystemCode{Code: fx.System},
		Task:        moved.ID,
		Commit:      p.Commit,
		MergeCommit: p.MergeCommit,
	}
	moved.Stats.Added, moved.Stats.Removed = p.Stats.Added, p.Stats.Removed
	sysMsg := s.tskThreadSystem(c, string(c.Nonce), mrgKindSystem, &moved, []string{}, body)
	rows := append([]Row{RowOf(moved)}, tskCardRows(c.Disk, moved.ID, fx.Cards, c.Now)...)
	rows = append(rows, RowOf(sysMsg))
	relRows, relItems := s.mrgReleaseDeps(c.Disk, moved.ID, c.Now)
	items := tskItemCtx{taskID: moved.ID, secretary: s.Group.Secretary, noticeMsg: sysMsg.ID}.items(fx.Items)
	return Result{Rows: append(rows, relRows...), Items: append(items, relItems...)}
}

// mrgConflict records a failed merge attempt through row 9 of the task machine,
// writes a checks message at stage merge in the task's thread, and notifies the
// secretary.
func mrgConflict(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskConflict
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	moved, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnConflict, Commit: p.Commit})
	if refusal != nil {
		return Result{}, refusal
	}
	checksMsg := s.tskThreadSystem(c, string(c.Nonce), mrgKindChecks, &moved, []string{}, mrgConflictBody(p.Commit, p.Detail))
	items := tskItemCtx{taskID: moved.ID, secretary: s.Group.Secretary, noticeMsg: checksMsg.ID}.items(fx.Items)
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
	task, mentions, refusal := s.mrgSummaryTarget(p)
	if refusal != nil {
		return Result{}, refusal
	}
	msg := s.mrgSummaryMsg(c, p, task, mentions)
	return Result{Rows: []Row{RowOf(msg), RowOf(postAdvanceThread(task, msg))}}, nil
}

func (s *State) mrgSummaryTarget(p proto.Post[proto.EmptyBody]) (proto.TaskRow, []string, *Refusal) {
	if p.Thread == nil {
		return proto.TaskRow{}, nil, &Refusal{Code: "bad_request", Msg: "summary post requires a task thread"}
	}
	task, exists := s.Tasks[string(*p.Thread)]
	if !exists {
		return proto.TaskRow{}, nil, &Refusal{Code: "not_found", Msg: "summary names no task"}
	}
	mentions, refusal := s.postMentions(p.Mentions)
	if refusal != nil {
		return proto.TaskRow{}, nil, refusal
	}
	if task.State != taskMerged {
		return proto.TaskRow{}, nil, &Refusal{Code: "conflict", Msg: "summary requires a merged task"}
	}
	return task, mentions, nil
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
		Minutes: int((*task.Stats.Ended - task.Stats.Started) / mrgMinuteMS),
		Added:   task.Stats.Added,
		Removed: task.Stats.Removed,
		PR:      pr,
	}
}

func (s *State) mrgReleaseDeps(d Disk, mergedID string, now int64) ([]Row, []Item) {
	var rows []Row
	var items []Item
	for _, id := range slices.Sorted(maps.Keys(s.Tasks)) {
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

func (s *State) mrgReleaseOne(d Disk, dep proto.TaskRow, mergedID string, now int64) (Row, []Item, bool) {
	ev := Event{
		Kind:    OnReleased,
		By:      ByHome,
		Now:     now,
		Limits:  s.Group.Limits,
		Blocked: s.mrgBlocked(dep.Depends, mergedID),
		Pending: tskPending(d, dep.ID),
	}
	released, fx, _ := Step(dep, ev)
	if released.State != taskActive {
		return nil, nil, false
	}
	released.Stats.Started = now
	items := tskItemCtx{taskID: released.ID, assignMsg: released.ID}.items(fx.Items)
	return RowOf(released), items, true
}

func (s *State) mrgBlocked(depends []string, mergedID string) bool {
	for _, depID := range depends {
		if depID != mergedID && s.Tasks[depID].State != taskMerged {
			return true
		}
	}
	return false
}
