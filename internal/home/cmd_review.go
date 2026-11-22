package home

import (
	"crypto/sha256"
	"encoding/base64"

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
	revStageLocal = "local"
	revStageCloud = "cloud"
	revKindChecks = "checks"
	revKindReview = "review"
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
	moved, fx, refusal := s.tskStep(c, p.ID, Event{Kind: kind, Commit: p.Commit, OK: p.OK})
	if refusal != nil {
		return Result{}, refusal
	}
	body := proto.ChecksBody{Stage: stage, OK: p.OK, Commit: p.Commit, Items: p.Items}
	msg := s.tskThreadSystem(c, string(c.Nonce), revKindChecks, &moved, []string{}, body)
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
	moved, fx, refusal := s.tskStep(c, p.ID, Event{Kind: OnSubmitted, Commit: p.Commit})
	if refusal != nil {
		return Result{}, refusal
	}
	moved.PR, moved.PRURL = p.PR, p.PRURL
	moved.Stats.Added, moved.Stats.Removed = p.Stats.Added, p.Stats.Removed
	retire, _ := tskSettleQueue(c.Disk.Queued(), moved.ID, fx.Retire)
	items := tskItemCtx{taskID: moved.ID, secretary: s.Group.Secretary}.items(fx.Items)
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

func (s *State) revFinishThread(c Command, moved proto.TaskRow, fx Effects, primary proto.MsgRow, answer store.Outcome) Result {
	rows := []Row{RowOf(primary)}
	var noticeMsg string
	if fx.System != "" {
		failed := s.tskFailedSystem(c, revFailedID(c.Nonce), &moved, fx)
		rows = append(rows, RowOf(failed))
		noticeMsg = failed.ID
	}
	rows = append(rows, RowOf(moved))
	var pr int
	if s.Group.Forge != "none" {
		pr = moved.PR
	}
	items := tskItemCtx{
		taskID: moved.ID, secretary: s.Group.Secretary, feedbackMsg: primary.ID, noticeMsg: noticeMsg, pr: pr,
	}.items(fx.Items)
	return Result{Rows: rows, Items: items, Answer: answer}
}

// revFailedID derives the id of a task_failed system message from the command's
// nonce when that nonce already names the command's checks or review message.
func revFailedID(nonce proto.ID) string {
	sum := sha256.Sum256([]byte("SoloDawn task failed " + string(nonce)))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}
