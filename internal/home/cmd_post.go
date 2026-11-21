package home

import (
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Messages in the main line and in task threads: post of kinds text, progress,
// question, report and handback (planning/contracts/commands.md 1.3;
// planning/contracts/messages.md 1, 2.7 and 2.10). A message in a thread
// advances the task's thread summary (planning/contracts/rows.md section 4).

func init() {
	Commands.Add("post/text", postText)
	Commands.Add("post/progress", postProgress)
	Commands.Add("post/question", postQuestion)
	Commands.Add("post/report", postReport)
	Commands.Add("post/handback", postHandback)
}

const (
	postKindText     = "text"
	postKindProgress = "progress"
	postKindQuestion = "question"
	postKindReport   = "report"
	postKindHandback = "handback"

	postMentionWhy   = "mention"
	postPreviewLimit = 200
)

type postPlace uint8

const (
	postAnywhere postPlace = iota
	postInThread
	postMainOnly
)

func postText(s *State, c Command) (Result, *Refusal) {
	rows, msg, refusal := postWrite[proto.EmptyBody](s, c, postKindText, false, postAnywhere)
	if refusal != nil {
		return Result{}, refusal
	}
	items := s.postWakes(msg)
	if len(items) == 0 && s.postReachSecretary(c.Who, msg) {
		items = []Item{{AI: s.Group.Secretary, Why: postMentionWhy, Msg: msg.ID}}
	}
	return Result{Rows: rows, Items: items}, nil
}

// postReachSecretary reports whether a message without AI mentions goes to the
// secretary: a main-line message from a person in a group that has one.
func (s *State) postReachSecretary(w Who, msg proto.MsgRow) bool {
	return msg.Thread == nil && w.AI == "" && s.Group.Secretary != ""
}

func postProgress(s *State, c Command) (Result, *Refusal) {
	return postTalk(s, c, postKindProgress, postInThread)
}

func postQuestion(s *State, c Command) (Result, *Refusal) {
	return postTalk(s, c, postKindQuestion, postAnywhere)
}

func postTalk(s *State, c Command, kind string, place postPlace) (Result, *Refusal) {
	rows, msg, refusal := postWrite[proto.EmptyBody](s, c, kind, true, place)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: rows, Items: s.postWakes(msg)}, nil
}

func postReport(s *State, c Command) (Result, *Refusal) {
	rows, _, refusal := postWrite[proto.ReportBody](s, c, postKindReport, true, postInThread)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: rows}, nil
}

func postHandback(s *State, c Command) (Result, *Refusal) {
	place := postInThread
	if s.Secretary(c.Who) {
		place = postMainOnly
	}
	rows, msg, refusal := postWrite[proto.HandbackBody](s, c, postKindHandback, true, place)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: rows, Items: s.postHandbackNotice(msg)}, nil
}

func (s *State) postHandbackNotice(msg proto.MsgRow) []Item {
	if s.Group.Secretary == "" || msg.By == s.Group.Secretary {
		return nil
	}
	return []Item{{AI: s.Group.Secretary, Why: itemNotice, Task: postTaskID(msg.Thread), Msg: msg.ID, Code: postKindHandback}}
}

func postWrite[B any](s *State, c Command, kind string, aiOnly bool, place postPlace) ([]Row, proto.MsgRow, *Refusal) {
	if aiOnly && c.Who.AI == "" {
		return nil, proto.MsgRow{}, &Refusal{"forbidden", "only an AI posts this kind of message"}
	}
	var p proto.Post[B]
	if refusal := c.Decode(&p); refusal != nil {
		return nil, proto.MsgRow{}, refusal
	}
	mentions, refusal := s.postCheck(c.Who, kind, place, p.Thread, p.Mentions)
	if refusal != nil {
		return nil, proto.MsgRow{}, refusal
	}
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, Thread: (*string)(p.Thread),
		By: c.Who.Member(), Kind: kind, Timestamp: c.Timestamp, Text: p.Text, Mentions: mentions,
		Re: (*string)(p.Re), Body: briefJSON(p.Body), Env: &c.Env,
	}
	return s.postRows(msg), msg, nil
}

func (s *State) postCheck(w Who, kind string, place postPlace, thread *proto.ID, raw []proto.ID) ([]string, *Refusal) {
	if !postAllows(thread != nil, place) {
		return nil, &Refusal{"bad_request", "the post is not where its kind allows"}
	}
	if refusal := s.postThread(w, kind, thread); refusal != nil {
		return nil, refusal
	}
	return s.postMentions(raw)
}

func postAllows(inThread bool, place postPlace) bool {
	switch place {
	case postInThread:
		return inThread
	case postMainOnly:
		return !inThread
	default:
		return true
	}
}

func (s *State) postThread(w Who, kind string, thread *proto.ID) *Refusal {
	if thread == nil {
		return nil
	}
	task, ok := s.Tasks[string(*thread)]
	if !ok {
		return &Refusal{"not_found", "the post names no task"}
	}
	if kind == postKindReport && task.Assignee != w.AI {
		return &Refusal{"forbidden", "only the assignee of the task posts a report"}
	}
	return nil
}

// postMentions checks the members a post mentions and returns their ids: each
// is named once (bad_request) and is a member of the group (conflict).
func (s *State) postMentions(ids []proto.ID) ([]string, *Refusal) {
	for i, id := range ids {
		if slices.Contains(ids[:i], id) {
			return nil, &Refusal{"bad_request", "the post mentions a member twice"}
		}
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		if !s.postMember(string(id)) {
			return nil, &Refusal{"conflict", "the post mentions someone who is no member of the group"}
		}
		out[i] = string(id)
	}
	return out, nil
}

func (s *State) postMember(id string) bool {
	_, person := s.People[id]
	_, ai := s.AIs[id]
	return person || ai
}

func (s *State) postRows(msg proto.MsgRow) []Row {
	if msg.Thread == nil {
		return []Row{RowOf(msg)}
	}
	task := s.Tasks[*msg.Thread]
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{
		By: msg.By, Kind: msg.Kind, Text: postPreview(msg.Text), Timestamp: msg.Timestamp,
	}
	return []Row{RowOf(msg), RowOf(task)}
}

func postPreview(text string) string {
	runes := []rune(text)
	return string(runes[:min(len(runes), postPreviewLimit)])
}

func (s *State) postWakes(msg proto.MsgRow) []Item {
	var items []Item
	task := postTaskID(msg.Thread)
	for _, m := range msg.Mentions {
		if _, ai := s.AIs[m]; ai && m != msg.By {
			items = append(items, Item{AI: m, Why: postMentionWhy, Task: task, Msg: msg.ID})
		}
	}
	return items
}

func postTaskID(thread *string) string {
	if thread == nil {
		return ""
	}
	return *thread
}
