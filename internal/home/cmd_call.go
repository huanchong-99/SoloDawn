package home

import (
	"encoding/json"
	"fmt"
	"slices"

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
	calWhyMention    = "mention"
	calWhyReply      = "reply"
	calMaxPreview    = 200
)

// calCall stores a waiting call from an AI to another AI member and queues a
// mention item for the callee.
func calCall(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI == "" {
		return Result{}, &Refusal{Code: "forbidden", Msg: "only an AI posts a call"}
	}
	var calPost proto.Post[proto.CallBody]
	if calRefusal := c.Decode(&calPost); calRefusal != nil {
		return Result{}, calRefusal
	}
	if !calCallShape(calPost, c.Who.AI) {
		return Result{}, &Refusal{Code: "bad_request", Msg: "the call post does not match the contract"}
	}
	return s.calApplyCall(c, calPost)
}

func calCallShape(calPost proto.Post[proto.CallBody], caller string) bool {
	if calPost.Main || calPost.Body.Depth < 1 || calPost.Body.To == caller {
		return false
	}
	return calDistinct(calPost.Mentions)
}

func (s *State) calApplyCall(c Command, calPost proto.Post[proto.CallBody]) (Result, *Refusal) {
	if !s.calThreadExists(calPost.Thread) {
		return Result{}, &Refusal{Code: "not_found", Msg: "the call names no task"}
	}
	calList, calRefusal := s.calCallMembers(calPost.Body.To, calPost.Mentions)
	if calRefusal != nil {
		return Result{}, calRefusal
	}
	if calPost.Body.Depth > s.Group.Limits.AskDepth {
		return Result{}, &Refusal{
			Code: "limit",
			Msg:  fmt.Sprintf("call depth %d exceeds the ask_depth limit of %d", calPost.Body.Depth, s.Group.Limits.AskDepth),
		}
	}
	return s.calWriteCall(c, calPost, calList)
}

func (s *State) calCallMembers(callee string, raw []proto.ID) ([]string, *Refusal) {
	if _, ok := s.AIs[callee]; !ok {
		return nil, &Refusal{Code: "conflict", Msg: "the callee is no AI member of the group"}
	}
	return s.calMentions(raw)
}

func (s *State) calWriteCall(c Command, calPost proto.Post[proto.CallBody], calList []string) (Result, *Refusal) {
	calID := string(c.Nonce)
	if _, taken := c.Disk.Msg(calID); taken {
		return Result{}, &Refusal{Code: "conflict", Msg: "a message with the id of the call exists"}
	}
	calThread := (*string)(calPost.Thread)
	calMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: calID}, Group: s.ID, Thread: calThread,
		By: c.Who.AI, Kind: calKindCall, Timestamp: c.Timestamp, Text: calPost.Text,
		Mentions: calList, Re: (*string)(calPost.Re), Body: briefJSON(calPost.Body),
		State: &proto.CardState{Status: calStatusWaiting}, Env: &c.Env,
	}
	calItem := Item{AI: calPost.Body.To, Why: calWhyMention, Task: calOptText(calThread), Msg: calID}
	return Result{Rows: s.calAppendTask([]Row{RowOf(calMsg)}, calMsg), Items: []Item{calItem}}, nil
}

// calReply answers a waiting call addressed to the signing AI, records the
// elapsed milliseconds on the call, and queues a reply item for the caller.
func calReply(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI == "" {
		return Result{}, &Refusal{Code: "forbidden", Msg: "only an AI posts a reply"}
	}
	var calPost proto.Post[proto.EmptyBody]
	if calRefusal := c.Decode(&calPost); calRefusal != nil {
		return Result{}, calRefusal
	}
	if !calReplyShape(calPost) {
		return Result{}, &Refusal{Code: "bad_request", Msg: "the reply post does not match the contract"}
	}
	return s.calApplyReply(c, calPost)
}

func calReplyShape(calPost proto.Post[proto.EmptyBody]) bool {
	if calPost.Main || calPost.Re == nil {
		return false
	}
	return calDistinct(calPost.Mentions)
}

func (s *State) calApplyReply(c Command, calPost proto.Post[proto.EmptyBody]) (Result, *Refusal) {
	calTarget, calRefusal := calLoadWaiting(c.Disk, string(*calPost.Re), c.Who.AI)
	if calRefusal != nil {
		return Result{}, calRefusal
	}
	if calRefusal := s.calReplyThread(calPost.Thread, calTarget.Thread); calRefusal != nil {
		return Result{}, calRefusal
	}
	calList, calRefusal := s.calMentions(calPost.Mentions)
	if calRefusal != nil {
		return Result{}, calRefusal
	}
	return s.calWriteReply(c, calPost, calTarget, calList)
}

func calLoadWaiting(d Disk, calID, responder string) (proto.MsgRow, *Refusal) {
	calTarget, ok := d.Msg(calID)
	if !ok {
		return proto.MsgRow{}, &Refusal{Code: "not_found", Msg: "the reply names no call message"}
	}
	if !calWaitingFor(calTarget, responder) {
		return proto.MsgRow{}, &Refusal{Code: "conflict", Msg: "the call is not waiting for this AI"}
	}
	return calTarget, nil
}

func calWaitingFor(calTarget proto.MsgRow, responder string) bool {
	if calTarget.Kind != calKindCall || briefStatus(calTarget) != calStatusWaiting {
		return false
	}
	var calBody proto.CallBody
	json.Unmarshal(calTarget.Body, &calBody)
	return calBody.To == responder
}

func (s *State) calReplyThread(got *proto.ID, want *string) *Refusal {
	if calOptText((*string)(got)) != calOptText(want) {
		return &Refusal{Code: "bad_request", Msg: "a reply belongs in the same thread or main line as its call"}
	}
	if !s.calThreadExists(got) {
		return &Refusal{Code: "not_found", Msg: "the reply names no task"}
	}
	return nil
}

func (s *State) calWriteReply(c Command, calPost proto.Post[proto.EmptyBody], calTarget proto.MsgRow, calList []string) (Result, *Refusal) {
	calReplyID := string(c.Nonce)
	if _, taken := c.Disk.Msg(calReplyID); taken {
		return Result{}, &Refusal{Code: "conflict", Msg: "a message with the id of the reply exists"}
	}
	calElapsed := max(int64(0), c.Timestamp-calTarget.Timestamp)
	calTarget.State = &proto.CardState{Status: calStatusAnswer, Reply: &calReplyID, MS: &calElapsed}
	calThread := (*string)(calPost.Thread)
	calMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: calReplyID}, Group: s.ID, Thread: calThread,
		By: c.Who.AI, Kind: calKindReply, Timestamp: c.Timestamp, Text: calPost.Text,
		Mentions: calList, Re: (*string)(calPost.Re), Body: briefJSON(calPost.Body), Env: &c.Env,
	}
	calRows := s.calAppendTask([]Row{RowOf(calTarget), RowOf(calMsg)}, calMsg)
	return Result{Rows: calRows, Items: s.calReplyItems(calTarget.By, calOptText(calThread), calReplyID)}, nil
}

func (s *State) calReplyItems(caller, calTask, calMsg string) []Item {
	if _, ok := s.AIs[caller]; !ok {
		return nil
	}
	return []Item{{AI: caller, Why: calWhyReply, Task: calTask, Msg: calMsg}}
}

func (s *State) calThreadExists(calThread *proto.ID) bool {
	if calThread == nil {
		return true
	}
	_, ok := s.Tasks[string(*calThread)]
	return ok
}

func (s *State) calMentions(calIDs []proto.ID) ([]string, *Refusal) {
	calOut := make([]string, len(calIDs))
	for calIdx, calID := range calIDs {
		calMember := string(calID)
		if !s.calIsMember(calMember) {
			return nil, &Refusal{Code: "conflict", Msg: "the post mentions someone who is no member of the group"}
		}
		calOut[calIdx] = calMember
	}
	return calOut, nil
}

func (s *State) calIsMember(calID string) bool {
	_, calHuman := s.People[calID]
	_, calAI := s.AIs[calID]
	return calHuman || calAI
}

func calDistinct(calIDs []proto.ID) bool {
	for calIdx, calID := range calIDs {
		if slices.Contains(calIDs[:calIdx], calID) {
			return false
		}
	}
	return true
}

func (s *State) calAppendTask(calRows []Row, calMsg proto.MsgRow) []Row {
	if calMsg.Thread == nil {
		return calRows
	}
	calTask := s.Tasks[*calMsg.Thread]
	calTask.Thread.Count++
	calTask.Thread.Last = &proto.ThreadMessage{
		By: calMsg.By, Kind: calMsg.Kind, Text: calPreview(calMsg.Text), Timestamp: calMsg.Timestamp,
	}
	return append(calRows, RowOf(calTask))
}

func calPreview(calText string) string {
	calRunes := []rune(calText)
	return string(calRunes[:min(len(calRunes), calMaxPreview)])
}

func calOptText(calPtr *string) string {
	if calPtr == nil {
		return ""
	}
	return *calPtr
}
