package home

import "github.com/huanchong-99/SoloDawn/internal/proto"

// The secretary's summary of a done task: post of kind summary
// (planning/contracts/commands.md 1.3; planning/contracts/messages.md 2.5).

const (
	sumKindSummary = "summary"
	sumMinuteMS    = 60_000
)

func init() { Commands.Add("post/summary", sumSummary) }

// sumSummary writes the secretary's completion summary for a done task in its
// thread and in the main line, filling the summary body from the task row.
func sumSummary(s *State, c Command) (Result, *Refusal) {
	if !s.Secretary(c.Who) {
		return Result{}, &Refusal{Code: "forbidden", Msg: "only the secretary posts a summary"}
	}
	var p proto.Post[proto.EmptyBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	task, mentions, refusal := s.sumSummaryTarget(p)
	if refusal != nil {
		return Result{}, refusal
	}
	msg := s.sumSummaryMsg(c, p, task, mentions)
	return Result{Rows: []Row{RowOf(msg), RowOf(postAdvanceThread(task, msg))}}, nil
}

func (s *State) sumSummaryTarget(p proto.Post[proto.EmptyBody]) (proto.TaskRow, []string, *Refusal) {
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
	if task.State != taskDone {
		return proto.TaskRow{}, nil, &Refusal{Code: "conflict", Msg: "summary requires a done task"}
	}
	return task, mentions, nil
}

func (s *State) sumSummaryMsg(c Command, p proto.Post[proto.EmptyBody], task proto.TaskRow, mentions []string) proto.MsgRow {
	tid := task.ID
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)},
		Group:     s.ID,
		Thread:    &tid,
		Main:      true,
		By:        c.Who.Member(),
		Kind:      sumKindSummary,
		Timestamp: c.Timestamp,
		Text:      p.Text,
		Mentions:  mentions,
		Re:        (*string)(p.Re),
		Body:      briefJSON(sumSummaryBody(task)),
		Env:       &c.Env,
	}
}

func sumSummaryBody(task proto.TaskRow) proto.SummaryBody {
	return proto.SummaryBody{
		Task:    task.ID,
		Score:   task.Review.Score,
		Rounds:  task.Review.Round,
		Minutes: int((*task.Stats.Ended - task.Stats.Started) / sumMinuteMS),
	}
}
