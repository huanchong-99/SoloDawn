package home

import (
	"encoding/json"
	"fmt"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// AI-to-AI calls and their answers: post of kinds call and reply
// (planning/contracts/commands.md section 1.3;
// planning/contracts/messages.md sections 1, 2.6 and 3.4;
// planning/flows/ask.md).

func init() {
	Commands.Add("post/call", calCall)
	Commands.Add("post/reply", calReply)
}

const (
	calKindCall      = "call"
	calKindReply     = "reply"
	calStatusWaiting = "waiting"
	calStatusAnswer  = "answered"
	calWhyReply      = "reply"
)

// calCall stores a waiting call from an AI to another AI member and queues a
// mention item for the callee.
func calCall(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI == "" {
		return Result{}, &Refusal{Code: "forbidden", Msg: "only an AI posts a call"}
	}
	var p proto.Post[proto.CallBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Body.To == c.Who.AI {
		return Result{}, &Refusal{Code: "bad_request", Msg: "an AI cannot call itself"}
	}
	return s.calApplyCall(c, p)
}

func (s *State) calApplyCall(c Command, p proto.Post[proto.CallBody]) (Result, *Refusal) {
	mentions, refusal := s.postCheck(c.Who, calKindCall, false, p.Thread, p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	if _, ok := s.AIs[p.Body.To]; !ok {
		return Result{}, &Refusal{Code: "conflict", Msg: "the callee is no AI member of the group"}
	}
	if p.Body.Depth > s.Group.Limits.AskDepth {
		return Result{}, &Refusal{
			Code: "limit",
			Msg:  fmt.Sprintf("call depth %d exceeds the ask_depth limit of %d", p.Body.Depth, s.Group.Limits.AskDepth),
		}
	}
	return s.calWriteCall(c, p, mentions), nil
}

func (s *State) calWriteCall(c Command, p proto.Post[proto.CallBody], mentions []string) Result {
	id := string(c.Nonce)
	thread := (*string)(p.Thread)
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: id}, Group: s.ID, Thread: thread,
		By: c.Who.AI, Kind: calKindCall, Timestamp: c.Timestamp, Text: p.Text,
		Mentions: mentions, Re: (*string)(p.Re), Body: briefJSON(p.Body),
		State: &proto.CardState{Status: calStatusWaiting}, Env: &c.Env,
	}
	item := Item{AI: p.Body.To, Why: postMentionWhy, Task: postTaskID(thread), Msg: id}
	return Result{Rows: s.postRows(msg), Items: []Item{item}}
}

// calReply answers a waiting call addressed to the signing AI, records the
// elapsed milliseconds on the call, and queues a reply item for the caller.
func calReply(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI == "" {
		return Result{}, &Refusal{Code: "forbidden", Msg: "only an AI posts a reply"}
	}
	var p proto.Post[proto.EmptyBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if p.Re == nil {
		return Result{}, &Refusal{Code: "bad_request", Msg: "a reply names the call in re"}
	}
	return s.calApplyReply(c, p)
}

func (s *State) calApplyReply(c Command, p proto.Post[proto.EmptyBody]) (Result, *Refusal) {
	target, refusal := calLoadWaiting(c.Disk, string(*p.Re), c.Who.AI)
	if refusal != nil {
		return Result{}, refusal
	}
	if postTaskID((*string)(p.Thread)) != postTaskID(target.Thread) {
		return Result{}, &Refusal{Code: "bad_request", Msg: "a reply belongs in the same thread or main line as its call"}
	}
	mentions, refusal := s.postCheck(c.Who, calKindReply, false, p.Thread, p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	return s.calWriteReply(c, p, target, mentions), nil
}

func calLoadWaiting(d Disk, id, responder string) (proto.MsgRow, *Refusal) {
	target, ok := d.Msg(id)
	if !ok {
		return proto.MsgRow{}, &Refusal{Code: "not_found", Msg: "the reply names no call message"}
	}
	if !calWaitingFor(target, responder) {
		return proto.MsgRow{}, &Refusal{Code: "conflict", Msg: "the call is not waiting for this AI"}
	}
	return target, nil
}

func calWaitingFor(target proto.MsgRow, responder string) bool {
	if target.Kind != calKindCall || briefStatus(target) != calStatusWaiting {
		return false
	}
	var body proto.CallBody
	json.Unmarshal(target.Body, &body)
	return body.To == responder
}

func (s *State) calWriteReply(c Command, p proto.Post[proto.EmptyBody], target proto.MsgRow, mentions []string) Result {
	replyID := string(c.Nonce)
	elapsed := max(int64(0), c.Timestamp-target.Timestamp)
	target.State = &proto.CardState{Status: calStatusAnswer, Reply: &replyID, MS: &elapsed}
	thread := (*string)(p.Thread)
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: replyID}, Group: s.ID, Thread: thread,
		By: c.Who.AI, Kind: calKindReply, Timestamp: c.Timestamp, Text: p.Text,
		Mentions: mentions, Re: (*string)(p.Re), Body: briefJSON(p.Body), Env: &c.Env,
	}
	rows := append([]Row{RowOf(target)}, s.postRows(msg)...)
	return Result{Rows: rows, Items: s.calReplyItems(target.By, postTaskID(thread), replyID)}
}

func (s *State) calReplyItems(caller, task, msg string) []Item {
	if _, ok := s.AIs[caller]; !ok {
		return nil
	}
	return []Item{{AI: caller, Why: calWhyReply, Task: task, Msg: msg}}
}
