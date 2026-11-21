package home

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Checks, submissions and reviews of tasks: task checked, submitted and cloud,
// and post of kind review (planning/contracts/commands.md 1.3, 1.5 and 1.8;
// planning/contracts/messages.md 2.8, 2.9 and 2.11;
// planning/contracts/task-machine.md rows 4 to 7 and 14).

func init() {
	Commands.Add("task/checked", revChecked)
	Commands.Add("task/submitted", revSubmitted)
	Commands.Add("task/cloud", revCloud)
	Commands.Add("post/review", revPost)
}

const (
	revStageLocal   = "local"
	revStageCloud   = "cloud"
	revKindChecks   = "checks"
	revKindReview   = "review"
	revKindSystem   = "system"
	revAuthorSystem = "system"
	revPreviewMax   = 200
)

// revChecked records local check results through row 4 of the task machine and
// writes a checks message in the task's thread.
func revChecked(s *State, c Command) (Result, *Refusal) {
	return s.revRunChecks(c, revStageLocal, OnChecked)
}

// revCloud records hosted check results through row 6 of the task machine,
// writes a checks message in the task's thread, and writes task_failed when
// the failure limit stops the task (row 14).
func revCloud(s *State, c Command) (Result, *Refusal) {
	return s.revRunChecks(c, revStageCloud, OnCloud)
}

func (s *State) revRunChecks(c Command, stage string, kind EventKind) (Result, *Refusal) {
	var p proto.TaskChecked
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	moved, fx, refusal := s.revStep(c, p.ID, Event{Kind: kind, Commit: p.Commit, OK: p.OK})
	if refusal != nil {
		return Result{}, refusal
	}
	body := proto.ChecksBody{Stage: stage, OK: p.OK, Commit: p.Commit, Items: p.Items}
	msg := s.revSystemMsg(string(c.Nonce), moved.ID, revKindChecks, c.Now, []string{}, body)
	return s.revFinishThread(c, moved, fx, msg, store.Outcome{}), nil
}

// revSubmitted records a submitted commit, pull request and diff line counts
// through row 5 of the task machine, retiring any queued review item for an
// earlier commit.
func revSubmitted(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskSubmitted
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	moved, fx, refusal := s.revStep(c, p.ID, Event{Kind: OnSubmitted, Commit: p.Commit})
	if refusal != nil {
		return Result{}, refusal
	}
	moved.PR, moved.PRURL = p.PR, p.PRURL
	moved.Stats.Added, moved.Stats.Removed = p.Stats.Added, p.Stats.Removed
	retire := revRetire(c.Disk.Queued(), moved.ID, fx.Retire)
	items := s.revMakeItems(fx.Items, moved, "", "")
	return Result{Rows: []Row{RowOf(moved)}, Items: items, Retire: retire}, nil
}

// revPost is post of kind review: computes the review outcome, steps the task
// through row 7 (and row 14 on the sixth failed review), writes the completed
// review row in the thread, and answers with passed and round.
func revPost(s *State, c Command) (Result, *Refusal) {
	var p proto.Post[proto.ReviewSigned]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Thread == nil || *p.Thread != p.Body.Task {
		return Result{}, &Refusal{"bad_request", "a review post belongs in the thread of its task"}
	}
	return s.revApplyReview(c, p)
}

func (s *State) revApplyReview(c Command, p proto.Post[proto.ReviewSigned]) (Result, *Refusal) {
	score, passed := revOutcome(p.Body, s.Tasks[string(p.Body.Task)].Pass)
	ev := Event{Kind: OnReview, Member: c.Who.AI, Commit: p.Body.Commit, OK: passed, Score: score}
	moved, fx, refusal := s.revStep(c, p.Body.Task, ev)
	if refusal != nil {
		return Result{}, refusal
	}
	mentions, refusal := s.revMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	msg := s.revBuildMsg(c, p, moved, mentions, score, passed)
	return s.revFinishThread(c, moved, fx, msg, store.Outcome{Passed: &passed, Round: moved.Review.Round}), nil
}

func revOutcome(b proto.ReviewSigned, pass int) (score int, ok bool) {
	for _, it := range b.Items {
		if it.Veto {
			return 0, false
		}
	}
	return b.Score, b.Score >= pass
}

func (s *State) revMentions(ids []proto.ID) ([]string, *Refusal) {
	for i, id := range ids {
		if slices.Contains(ids[:i], id) {
			return nil, &Refusal{"bad_request", "the review mentions a member twice"}
		}
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		m := string(id)
		if !s.memExists(m) {
			return nil, &Refusal{"conflict", "the review mentions someone who is no member of the group"}
		}
		out[i] = m
	}
	return out, nil
}

func (s *State) revBuildMsg(c Command, p proto.Post[proto.ReviewSigned], task proto.TaskRow, mentions []string, score int, ok bool) proto.MsgRow {
	tid := task.ID
	body := proto.ReviewBody{
		Task: task.ID, Num: task.Num, Title: task.Title, Commit: p.Body.Commit,
		Round: task.Review.Round, Score: score, Pass: task.Pass, OK: ok, Items: p.Body.Items,
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, Thread: &tid,
		By: c.Who.Member(), Kind: revKindReview, Timestamp: c.Timestamp, Text: p.Text,
		Mentions: mentions, Re: (*string)(p.Re), Body: briefJSON(body), Env: &c.Env,
	}
}

func (s *State) revStep(c Command, id proto.ID, ev Event) (proto.TaskRow, Effects, *Refusal) {
	task, ok := s.Tasks[string(id)]
	if !ok {
		return proto.TaskRow{}, Effects{}, &Refusal{"not_found", "the group has no such task"}
	}
	if s.Hosts(c.Who, task.Assignee) {
		ev.By = ByHost
	}
	ev.Now = c.Now
	ev.Limits = s.Group.Limits
	ev.Remote = s.Group.Forge != "none"
	ev.Pending = revTaskPending(c.Disk, task.ID)
	moved, fx, err := Step(task, ev)
	if err != nil {
		return proto.TaskRow{}, Effects{}, &Refusal{err.Error(), "the task machine refused the command"}
	}
	return moved, fx, nil
}

func (s *State) revFinishThread(c Command, moved proto.TaskRow, fx Effects, primary proto.MsgRow, answer store.Outcome) Result {
	moved = revAddThread(moved, primary)
	rows := []Row{RowOf(primary)}
	var noticeMsg string
	if fx.System != "" {
		failed := s.revFailedMsg(c, moved, fx)
		moved = revAddThread(moved, failed)
		rows = append(rows, RowOf(failed))
		noticeMsg = failed.ID
	}
	rows = append(rows, RowOf(moved))
	items := s.revMakeItems(fx.Items, moved, primary.ID, noticeMsg)
	return Result{Rows: rows, Items: items, Answer: answer}
}

func revAddThread(task proto.TaskRow, msg proto.MsgRow) proto.TaskRow {
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{
		By: msg.By, Kind: msg.Kind, Text: revPreview(msg.Text), Timestamp: msg.Timestamp,
	}
	return task
}

func revPreview(text string) string {
	runes := []rune(text)
	return string(runes[:min(len(runes), revPreviewMax)])
}

func (s *State) revSystemMsg(id, taskID, kind string, ts int64, mentions []string, body any) proto.MsgRow {
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id}, Group: s.ID, Thread: &taskID,
		By: revAuthorSystem, Kind: kind, Timestamp: ts, Mentions: mentions, Body: briefJSON(body),
	}
}

func (s *State) revFailedMsg(c Command, task proto.TaskRow, fx Effects) proto.MsgRow {
	body := proto.TaskFailedBody{SystemCode: proto.SystemCode{Code: fx.System}, Task: task.ID, Reason: fx.Reason}
	return s.revSystemMsg(revFailedID(c.Nonce), task.ID, revKindSystem, c.Now, s.revFailedMentions(c.Disk, task), body)
}

// revFailedID derives the id of a task_failed system message from the command's
// nonce when that nonce already names the command's checks or review message.
func revFailedID(nonce proto.ID) string {
	sum := sha256.Sum256([]byte("SoloDawn task failed " + string(nonce)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func (s *State) revFailedMentions(d Disk, task proto.TaskRow) []string {
	mentions := []string{}
	if req := revBriefRequester(d, task.Brief); s.memExists(req) {
		mentions = append(mentions, req)
	}
	owner := s.human(s.AIs[task.Assignee].User)
	if owner != "" && !slices.Contains(mentions, owner) {
		mentions = append(mentions, owner)
	}
	return mentions
}

func revBriefRequester(d Disk, briefID string) string {
	card, _ := d.Msg(briefID)
	var b proto.BriefBody
	json.Unmarshal(card.Body, &b)
	return b.Requester
}

func revTaskPending(d Disk, taskID string) []PendingCard {
	var list []PendingCard
	for _, m := range d.Pending() {
		if pc, ok := revPendingOf(m, taskID); ok {
			list = append(list, pc)
		}
	}
	return list
}

func revPendingOf(m proto.MsgRow, taskID string) (PendingCard, bool) {
	var b struct {
		Task      string   `json:"task"`
		Approvers []string `json:"approvers"`
		To        []string `json:"to"`
	}
	json.Unmarshal(m.Body, &b)
	if b.Task != taskID {
		return PendingCard{}, false
	}
	who := b.To
	if m.Kind == waitApproval {
		who = b.Approvers
	}
	return PendingCard{Kind: m.Kind, Msg: m.ID, Who: who, Since: m.Timestamp}, true
}

func revRetire(queued []store.Item, taskID string, filters []ItemFilter) []int64 {
	var out []int64
	for _, q := range queued {
		if q.Task != nil && *q.Task == taskID && revMatches(q.Why, filters) {
			out = append(out, q.ID)
		}
	}
	return out
}

func revMatches(why string, filters []ItemFilter) bool {
	for _, f := range filters {
		if f.Why == why {
			return true
		}
	}
	return false
}

func (s *State) revMakeItems(list []TaskItem, task proto.TaskRow, feedbackMsg, noticeMsg string) []Item {
	var out []Item
	for _, raw := range list {
		if it, ok := s.revOneItem(raw, task, feedbackMsg, noticeMsg); ok {
			out = append(out, it)
		}
	}
	return out
}

func (s *State) revOneItem(raw TaskItem, task proto.TaskRow, feedbackMsg, noticeMsg string) (Item, bool) {
	if raw.Why == itemNotice {
		return s.revNoticeItem(raw.Code, task.ID, noticeMsg)
	}
	return s.revWorkItem(raw, task, feedbackMsg), true
}

func (s *State) revNoticeItem(code, taskID, msgID string) (Item, bool) {
	if s.Group.Secretary == "" {
		return Item{}, false
	}
	return Item{AI: s.Group.Secretary, Why: itemNotice, Task: taskID, Msg: msgID, Code: code}, true
}

func (s *State) revWorkItem(raw TaskItem, task proto.TaskRow, feedbackMsg string) Item {
	it := Item{AI: raw.AI, Why: raw.Why, Task: task.ID, Commit: raw.Commit}
	switch raw.Why {
	case itemChecks, itemReviewed:
		it.Msg = feedbackMsg
	case itemMerge:
		if s.Group.Forge != "none" {
			it.PR = task.PR
		}
	}
	return it
}
