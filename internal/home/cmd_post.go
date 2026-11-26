package home

import (
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Messages in the main line and in task threads: post of kinds text, progress,
// question and handback (planning/contracts/commands.md 1.3;
// planning/contracts/messages.md 1 and 2.10). A message in a thread advances
// the task's thread summary (planning/contracts/rows.md section 4). The body
// of each of these kinds is empty.

func init() {
	Commands.Add("post/text", postText)
	Commands.Add("post/progress", postProgress)
	Commands.Add("post/question", postQuestion)
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

func postText(s *State, c Command) (Result, *Refusal) {
	rows, msg, refusal := postWrite(s, c, postKindText, false, false)
	if refusal != nil {
		return Result{}, refusal
	}
	items := s.postWakes(msg)
	if len(items) == 0 && s.postReachSecretary(c.Who, msg) {
		items = []Item{{AI: s.Group.Secretary, Why: postMentionWhy, Msg: msg.ID}}
	}
	return s.cfmGateItems(c, cfmWhatDirect, msg, rows, items), nil
}

// postReachSecretary reports whether a message without AI mentions goes to the
// secretary: a main-line message from a person in a group that has one.
func (s *State) postReachSecretary(w Who, msg proto.MsgRow) bool {
	return msg.Thread == nil && w.AI == "" && s.Group.Secretary != ""
}

func postProgress(s *State, c Command) (Result, *Refusal) {
	return postTalk(s, c, postKindProgress, true)
}

func postQuestion(s *State, c Command) (Result, *Refusal) {
	return postTalk(s, c, postKindQuestion, false)
}

func postTalk(s *State, c Command, kind string, inThreadOnly bool) (Result, *Refusal) {
	rows, msg, refusal := postWrite(s, c, kind, true, inThreadOnly)
	if refusal != nil {
		return Result{}, refusal
	}
	return s.cfmGateItems(c, cfmWhatDirect, msg, rows, s.postWakes(msg)), nil
}

// postHandback writes an AI's hand-back account: in the thread of its task,
// or in the main line for a session without one. One in a thread is a notice
// for the secretary.
func postHandback(s *State, c Command) (Result, *Refusal) {
	rows, msg, refusal := postWrite(s, c, postKindHandback, true, false)
	if refusal != nil {
		return Result{}, refusal
	}
	var items []Item
	if msg.Thread != nil && s.Group.Secretary != "" {
		items = []Item{{AI: s.Group.Secretary, Why: "notice", Task: *msg.Thread, Msg: msg.ID, Code: postKindHandback}}
	}
	return Result{Rows: rows, Items: items}, nil
}

func postWrite(s *State, c Command, kind string, aiOnly, inThreadOnly bool) ([]Row, proto.MsgRow, *Refusal) {
	if aiOnly && c.Who.AI == "" {
		return nil, proto.MsgRow{}, &Refusal{"forbidden", "only an AI posts this kind of message"}
	}
	var p proto.Post[proto.EmptyBody]
	if refusal := c.Decode(&p); refusal != nil {
		return nil, proto.MsgRow{}, refusal
	}
	mentions, refusal := s.postCheck(inThreadOnly, p.Thread, p.Mentions)
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

func (s *State) postCheck(inThreadOnly bool, thread *proto.ID, raw []proto.ID) ([]string, *Refusal) {
	if inThreadOnly && thread == nil {
		return nil, &Refusal{"bad_request", "the post belongs in a task thread"}
	}
	if thread != nil && !s.postHasTask(*thread) {
		return nil, &Refusal{"not_found", "the post names no task"}
	}
	return s.postMentions(raw)
}

func (s *State) postHasTask(id proto.ID) bool {
	_, ok := s.Tasks[string(id)]
	return ok
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
		if !s.memExists(string(id)) {
			return nil, &Refusal{"conflict", "the post mentions someone who is no member of the group"}
		}
		out[i] = string(id)
	}
	return out, nil
}

func (s *State) postRows(msg proto.MsgRow) []Row {
	if msg.Thread == nil {
		return []Row{RowOf(msg)}
	}
	return []Row{RowOf(msg), RowOf(postAdvanceThread(s.Tasks[*msg.Thread], msg))}
}

func postAdvanceThread(task proto.TaskRow, msg proto.MsgRow) proto.TaskRow {
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{
		By: msg.By, Kind: msg.Kind, Text: postPreview(msg.Text), Timestamp: msg.Timestamp,
	}
	return task
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
