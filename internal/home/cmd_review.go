package home

import (
	"crypto/sha256"
	"encoding/base64"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Submissions and reviews of tasks: post of kinds report and review
// (planning/contracts/commands.md 1.3 and 1.8; planning/contracts/messages.md
// 2.7, 2.9 and 2.11; planning/contracts/task-machine.md rows 5, 7 and 14).

func init() {
	Commands.Add("post/report", revReport)
	Commands.Add("post/review", revPost)
}

const revKindReview = "review"

// revReport is post of kind report, the submission of its task: row 5 of the
// task machine makes the message the report under review and retires any
// queued review item for an earlier one.
func revReport(s *State, c Command) (Result, *Refusal) {
	_, msg, refusal := postWrite(s, c, postKindReport, true, true)
	if refusal != nil {
		return Result{}, refusal
	}
	moved, fx, refusal := s.tskStep(c, proto.ID(*msg.Thread), Event{Kind: OnReport, Report: msg.ID})
	if refusal != nil {
		return Result{}, refusal
	}
	retire, _ := tskSettleQueue(c.Disk.Queued(), moved.ID, fx.Retire)
	items := tskItemCtx{taskID: moved.ID, secretary: s.Group.Secretary}.items(fx.Items)
	return Result{Rows: []Row{RowOf(msg), RowOf(postAdvanceThread(moved, msg))}, Items: items, Retire: retire}, nil
}

// revPost is post of kind review: computes the review outcome, steps the task
// through row 7 (and row 14 on the sixth failed review), writes the completed
// review row in the thread, and answers with passed and round. A review that
// passes ends the task: nothing is merged and no host acts.
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
	ev := Event{Kind: OnReview, Report: string(p.Body.Report), OK: passed, Score: score}
	moved, fx, refusal := s.tskStep(c, p.Body.Task, ev)
	if refusal != nil {
		return Result{}, refusal
	}
	mentions, refusal := s.postMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	msg := s.revBuildMsg(c, p, moved, mentions, score, passed)
	moved = postAdvanceThread(moved, msg)
	return s.revFinish(c, moved, fx, msg, store.Outcome{Passed: &passed, Round: moved.Review.Round}), nil
}

func revOutcome(b proto.ReviewSigned, pass int) (score int, ok bool) {
	for _, it := range b.Items {
		if it.Veto {
			return 0, false
		}
	}
	return b.Score, b.Score >= pass
}

func (s *State) revBuildMsg(c Command, p proto.Post[proto.ReviewSigned], task proto.TaskRow, mentions []string, score int, ok bool) proto.MsgRow {
	tid := task.ID
	body := proto.ReviewBody{
		Task: task.ID, Num: task.Num, Title: task.Title, Report: string(p.Body.Report),
		Round: task.Review.Round, Score: score, Pass: task.Pass, OK: ok, Items: p.Body.Items,
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, Thread: &tid,
		By: c.Who.Member(), Kind: revKindReview, Timestamp: c.Timestamp, Text: p.Text,
		Mentions: mentions, Re: (*string)(p.Re), Body: briefJSON(body), Env: &c.Env,
	}
}

// revFinish is what a review leaves: its message; the system message of a
// task that is done or stopped, which the secretary's notice names; the task;
// the cards a done task expires; and the queued tasks that start now.
func (s *State) revFinish(c Command, moved proto.TaskRow, fx Effects, review proto.MsgRow, answer store.Outcome) Result {
	rows := []Row{RowOf(review)}
	ctx := tskItemCtx{taskID: moved.ID, secretary: s.Group.Secretary, feedbackMsg: review.ID}
	if fx.System != "" {
		sys := s.revSystem(c, &moved, fx)
		rows, ctx.noticeMsg = append(rows, RowOf(sys)), sys.ID
	}
	rows = append(append(rows, RowOf(moved)), tskCardRows(c, moved.ID, fx.Cards)...)
	return s.tskRelease(c, Result{Rows: rows, Items: ctx.items(fx.Items), Answer: answer})
}

// revSystem is the task_done or task_failed message of a review, in the
// task's thread.
func (s *State) revSystem(c Command, task *proto.TaskRow, fx Effects) proto.MsgRow {
	id := revDerivedID("SoloDawn "+fx.System+" ", c.Nonce)
	if fx.Reason != "" {
		return s.tskFailedSystem(c, id, task, fx)
	}
	body := proto.TaskDoneBody{SystemCode: proto.SystemCode{Code: fx.System}, Task: task.ID}
	return s.tskThreadSystem(c, id, "system", task, []string{}, body)
}

// revDerivedID derives a 16-byte id from prefix and nonce when that nonce
// already names another message row of the command.
func revDerivedID(prefix string, nonce proto.ID) string {
	sum := sha256.Sum256([]byte(prefix + string(nonce)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}
