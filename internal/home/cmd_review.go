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
	revMaxScore     = 100
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
	if !revCheckPayloadOK(p) {
		return Result{}, &Refusal{"bad_request", "task check requires a commit and non-empty item fields"}
	}
	moved, fx, refusal := s.revHostStep(c, p.ID, Event{Kind: kind, Commit: p.Commit, OK: p.OK})
	if refusal != nil {
		return Result{}, refusal
	}
	body := proto.ChecksBody{Stage: stage, OK: p.OK, Commit: p.Commit, Items: p.Items}
	msg := s.revSystemMsg(string(c.Nonce), moved.ID, revKindChecks, c.Now, []string{}, body)
	return s.revFinishThread(c, moved, fx, msg, store.Outcome{})
}

func revCheckPayloadOK(p proto.TaskChecked) bool {
	if p.Commit == "" {
		return false
	}
	for _, it := range p.Items {
		if !revCheckItemOK(it) {
			return false
		}
	}
	return true
}

func revCheckItemOK(it proto.CheckItem) bool {
	if it.Name == "" {
		return false
	}
	return it.Detail == nil || *it.Detail != ""
}

// revSubmitted records a submitted commit, pull request and diff line counts
// through row 5 of the task machine, retiring any queued review item for an
// earlier commit.
func revSubmitted(s *State, c Command) (Result, *Refusal) {
	var p proto.TaskSubmitted
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	remote := s.Group.Forge != "none"
	if !revSubmitPayloadOK(c.Env.Payload, p, remote) {
		return Result{}, &Refusal{"bad_request", "task submitted fields do not match the group's forge or bounds"}
	}
	moved, fx, refusal := s.revHostStep(c, p.ID, Event{Kind: OnSubmitted, Commit: p.Commit})
	if refusal != nil {
		return Result{}, refusal
	}
	moved.PR, moved.PRURL = p.PR, p.PRURL
	moved.Stats.Added, moved.Stats.Removed = p.Stats.Added, p.Stats.Removed
	retire := revRetire(c.Disk.Queued(), moved.ID, fx.Retire)
	items := s.revMakeItems(fx.Items, moved, "", "")
	return Result{Rows: []Row{RowOf(moved)}, Items: items, Retire: retire}, nil
}

func revSubmitPayloadOK(raw string, p proto.TaskSubmitted, remote bool) bool {
	if p.Commit == "" || p.Stats.Added < 0 || p.Stats.Removed < 0 {
		return false
	}
	return revSubmitPROK(raw, p, remote)
}

func revSubmitPROK(raw string, p proto.TaskSubmitted, remote bool) bool {
	if remote {
		return p.PR >= 1 && p.PRURL != ""
	}
	var fields map[string]json.RawMessage
	proto.Decode([]byte(raw), &fields)
	_, hasPR := fields["pr"]
	_, hasURL := fields["pr_url"]
	return !hasPR && !hasURL
}

// revPost is post of kind review: computes the review outcome, steps the task
// through row 7 (and row 14 on the sixth failed review), writes the completed
// review row in the thread, and answers with passed and round.
func revPost(s *State, c Command) (Result, *Refusal) {
	if !s.briefWorking(c.Who.AI) {
		return Result{}, &Refusal{"forbidden", "only a working AI posts a review"}
	}
	var p proto.Post[proto.ReviewSigned]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if !revPostPayloadOK(p) {
		return Result{}, &Refusal{"bad_request", "the review post does not match the contract"}
	}
	return s.revApplyReview(c, p)
}

func revPostPayloadOK(p proto.Post[proto.ReviewSigned]) bool {
	if p.Main || p.Thread == nil || *p.Thread != p.Body.Task {
		return false
	}
	return revSignedBodyOK(p.Body) && revUniqueIDs(p.Mentions)
}

func revSignedBodyOK(b proto.ReviewSigned) bool {
	if b.Commit == "" || !revScoreOK(b.Score) {
		return false
	}
	for _, it := range b.Items {
		if !revReviewItemOK(it) {
			return false
		}
	}
	return true
}

func revScoreOK(score int) bool {
	return score >= 0 && score <= revMaxScore
}

func revReviewItemOK(it proto.ReviewItem) bool {
	if it.Text == "" || (it.File != nil && *it.File == "") {
		return false
	}
	return it.Line == nil || *it.Line >= 1
}

func revUniqueIDs(ids []proto.ID) bool {
	for i, id := range ids {
		if slices.Contains(ids[:i], id) {
			return false
		}
	}
	return true
}

func (s *State) revApplyReview(c Command, p proto.Post[proto.ReviewSigned]) (Result, *Refusal) {
	task, ok := s.Tasks[string(p.Body.Task)]
	if !ok {
		return Result{}, &Refusal{"not_found", "the review names no task"}
	}
	score, passed := revOutcome(p.Body, task.Pass)
	ev := Event{Kind: OnReview, By: ByAI, Member: c.Who.AI, Commit: p.Body.Commit, OK: passed, Score: score}
	moved, fx, refusal := s.revStepTask(c, task, ev)
	if refusal != nil {
		return Result{}, refusal
	}
	mentions, refusal := s.revCheckMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	msg := s.revBuildMsg(c, p, moved, mentions, score, passed)
	return s.revFinishThread(c, moved, fx, msg, store.Outcome{Passed: &passed, Round: moved.Review.Round})
}

func revOutcome(b proto.ReviewSigned, pass int) (score int, ok bool) {
	for _, it := range b.Items {
		if it.Veto {
			return 0, false
		}
	}
	return b.Score, b.Score >= pass
}

func (s *State) revCheckMentions(ids []proto.ID) ([]string, *Refusal) {
	out := make([]string, len(ids))
	for i, id := range ids {
		m := string(id)
		if !s.revIsMember(m) {
			return nil, &Refusal{"conflict", "the review mentions someone who is no member of the group"}
		}
		out[i] = m
	}
	return out, nil
}

func (s *State) revIsMember(id string) bool {
	_, human := s.People[id]
	_, ai := s.AIs[id]
	return human || ai
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

func (s *State) revHostStep(c Command, id proto.ID, ev Event) (proto.TaskRow, Effects, *Refusal) {
	task, ok := s.Tasks[string(id)]
	if !ok {
		return proto.TaskRow{}, Effects{}, &Refusal{"not_found", "the group has no such task"}
	}
	if s.Hosts(c.Who, task.Assignee) {
		ev.By = ByHost
	}
	return s.revStepTask(c, task, ev)
}

func (s *State) revStepTask(c Command, task proto.TaskRow, ev Event) (proto.TaskRow, Effects, *Refusal) {
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

func (s *State) revFinishThread(c Command, moved proto.TaskRow, fx Effects, primary proto.MsgRow, answer store.Outcome) (Result, *Refusal) {
	if _, taken := c.Disk.Msg(primary.ID); taken {
		return Result{}, &Refusal{"conflict", "a message with the id of the command exists"}
	}
	moved = revAddThread(moved, primary)
	rows := []Row{RowOf(primary)}
	noticeMsg := ""
	if fx.System != "" {
		failed, refusal := s.revFailedMsg(c, moved, fx)
		if refusal != nil {
			return Result{}, refusal
		}
		moved = revAddThread(moved, failed)
		rows = append(rows, RowOf(failed))
		noticeMsg = failed.ID
	}
	rows = append(rows, RowOf(moved))
	items := s.revMakeItems(fx.Items, moved, primary.ID, noticeMsg)
	return Result{Rows: rows, Items: items, Answer: answer}, nil
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

func (s *State) revFailedMsg(c Command, task proto.TaskRow, fx Effects) (proto.MsgRow, *Refusal) {
	id := revFailedID(c.Nonce)
	if _, taken := c.Disk.Msg(id); taken {
		return proto.MsgRow{}, &Refusal{"conflict", "a message with the derived id of task_failed exists"}
	}
	body := proto.TaskFailedBody{SystemCode: proto.SystemCode{Code: fx.System}, Task: task.ID, Reason: fx.Reason}
	return s.revSystemMsg(id, task.ID, revKindSystem, c.Now, s.revFailedMentions(c.Disk, task), body), nil
}

// revFailedID derives the id of a task_failed system message from the command's
// nonce when that nonce already names the command's checks or review message.
func revFailedID(nonce proto.ID) string {
	sum := sha256.Sum256([]byte("SoloDawn task failed " + string(nonce)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func (s *State) revFailedMentions(d Disk, task proto.TaskRow) []string {
	mentions := []string{}
	if req := revBriefRequester(d, task.Brief); s.revIsMember(req) {
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
