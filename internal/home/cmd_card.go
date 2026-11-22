package home

import (
	"encoding/json"
	"reflect"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Approval and decision cards: post of kinds approval and decision, act
// approve, deny and choose on them (planning/contracts/commands.md 1.3 and 1.4;
// planning/contracts/messages.md 2.3, 2.4, 3.2 and 3.3;
// planning/contracts/task-machine.md rows 17 to 20).

func init() {
	Commands.Add("post/approval", cardPostApproval)
	Commands.Add("post/decision", cardPostDecision)
	Commands.Add("act/approve", cardApprove)
	Commands.Add("act/deny", cardDeny)
	Commands.Add("act/choose", cardChoose)
}

const (
	cardKindApproval = "approval"
	cardKindDecision = "decision"

	cardStatusPending  = "pending"
	cardStatusApproved = "approved"
	cardStatusDenied   = "denied"
	cardStatusDecided  = "decided"
)

type cardPostSpec struct {
	kind     string
	on       EventKind
	taskID   string
	who      []string
	text     string
	mentions []string
	re       *proto.ID
	body     any
}

func cardPostApproval(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI == "" {
		return Result{}, &Refusal{"forbidden", "only an AI posts an approval card"}
	}
	var p proto.Post[proto.ApprovalBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if !cardApprovalShape(p, c.Who.AI) {
		return Result{}, &Refusal{"bad_request", "the approval card does not match the contract"}
	}
	mentions, refusal := s.postMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	spec := cardPostSpec{
		kind: cardKindApproval, on: OnApproval, taskID: p.Body.Task, who: p.Body.Approvers,
		text: p.Text, mentions: mentions, re: p.Re, body: p.Body,
	}
	return s.cardWritePost(c, spec)
}

func cardApprovalShape(p proto.Post[proto.ApprovalBody], ai string) bool {
	return cardApprovalThread(p.Thread, p.Body.Task) && p.Body.AI == ai && cardUsers(p.Body.Approvers)
}

func cardApprovalThread(thread *proto.ID, task string) bool {
	if thread == nil {
		return task == ""
	}
	return string(*thread) == task
}

func cardPostDecision(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI == "" {
		return Result{}, &Refusal{"forbidden", "only an AI posts a decision card"}
	}
	var p proto.Post[proto.DecisionBody]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if !cardDecisionShape(p) {
		return Result{}, &Refusal{"bad_request", "the decision card does not match the contract"}
	}
	mentions, refusal := s.postMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	spec := cardPostSpec{
		kind: cardKindDecision, on: OnDecision, taskID: cardTaskStr(p.Body.Task), who: p.Body.To,
		text: p.Text, mentions: mentions, re: p.Re, body: p.Body,
	}
	return s.cardWritePost(c, spec)
}

func cardDecisionShape(p proto.Post[proto.DecisionBody]) bool {
	return cardSameTask(p.Thread, p.Body.Task) && cardUsers(p.Body.To)
}

func cardSameTask(thread *proto.ID, task *string) bool {
	if thread == nil {
		return task == nil
	}
	return task != nil && string(*thread) == *task
}

func cardTaskStr(task *string) string {
	if task == nil {
		return ""
	}
	return *task
}

func cardUsers(users []string) bool {
	return len(users) > 0
}

func cardHasOption(options []proto.DecisionOption, id string) bool {
	for _, opt := range options {
		if opt.ID == id {
			return true
		}
	}
	return false
}

func (s *State) cardWritePost(c Command, spec cardPostSpec) (Result, *Refusal) {
	if spec.taskID == "" {
		return s.cardPostMain(c, spec)
	}
	return s.cardPostOnTask(c, spec)
}

func (s *State) cardPostMain(c Command, spec cardPostSpec) (Result, *Refusal) {
	card, refusal := s.cardNewMsg(c, spec.kind, nil, spec.text, spec.mentions, spec.re, spec.body)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: []Row{RowOf(card)}}, nil
}

func (s *State) cardPostOnTask(c Command, spec cardPostSpec) (Result, *Refusal) {
	task, ok := s.Tasks[spec.taskID]
	if !ok {
		return Result{}, &Refusal{"not_found", "the card names no task"}
	}
	id := string(c.Nonce)
	ev := Event{
		Kind: spec.on, By: s.cardPoster(c.Who), Member: c.Who.AI, Now: c.Now,
		Card:    PendingCard{Kind: spec.kind, Msg: id, Who: spec.who, Since: c.Timestamp},
		Pending: tskPending(c.Disk, task.ID),
	}
	moved, _, err := Step(task, ev)
	if err != nil {
		return Result{}, &Refusal{err.Error(), "the task machine refused the card"}
	}
	card, refusal := s.cardNewMsg(c, spec.kind, &task.ID, spec.text, spec.mentions, spec.re, spec.body)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: []Row{RowOf(card), RowOf(postAdvanceThread(moved, card))}}, nil
}

func (s *State) cardPoster(w Who) Sender {
	if s.Secretary(w) {
		return BySecretary
	}
	return ByAI
}

func (s *State) cardNewMsg(c Command, kind string, thread *string, text string, mentions []string, re *proto.ID, body any) (proto.MsgRow, *Refusal) {
	if _, taken := c.Disk.Msg(string(c.Nonce)); taken {
		return proto.MsgRow{}, &Refusal{"conflict", "a message with the id of the card exists"}
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, Thread: thread, Main: true,
		By: c.Who.Member(), Kind: kind, Timestamp: c.Timestamp, Text: text, Mentions: mentions,
		Re: (*string)(re), Body: briefJSON(body), State: &proto.CardState{Status: cardStatusPending}, Env: &c.Env,
	}, nil
}

func cardApprove(s *State, c Command) (Result, *Refusal) {
	return s.cardDecideApproval(c, cardStatusApproved)
}

func cardDeny(s *State, c Command) (Result, *Refusal) {
	return s.cardDecideApproval(c, cardStatusDenied)
}

func (s *State) cardDecideApproval(c Command, status string) (Result, *Refusal) {
	_, card, refusal := cardOpenAct(c, cardKindApproval, briefBare)
	if refusal != nil {
		return Result{}, refusal
	}
	var body proto.ApprovalBody
	json.Unmarshal(card.Body, &body)
	if !s.cardMayApprove(c.Who, body) {
		return Result{}, &Refusal{"forbidden", "only a listed approver decides an approval card"}
	}
	if briefStatus(card) != cardStatusPending {
		return Result{}, &Refusal{"conflict", "the approval card is no longer pending"}
	}
	decided := cardActed(card, c, status, nil, nil)
	return s.cardFinishAct(c, decided, body.Task, OnApprove, body.Approvers, body.AI), nil
}

func (s *State) cardMayApprove(w Who, body proto.ApprovalBody) bool {
	return w.AI == "" && slices.Contains(body.Approvers, w.User) && slices.Contains(s.AIs[body.AI].Approvers, w.User)
}

func cardChoose(s *State, c Command) (Result, *Refusal) {
	act, card, refusal := cardOpenAct(c, cardKindDecision, cardChooseFields)
	if refusal != nil {
		return Result{}, refusal
	}
	var body proto.DecisionBody
	json.Unmarshal(card.Body, &body)
	if !cardChoiceOK(body.Options, act.Arg) {
		return Result{}, &Refusal{"bad_request", "choose must match the options of the decision card"}
	}
	if !s.cardMayChoose(c.Who, body.To) {
		return Result{}, &Refusal{"forbidden", "only a listed person or an admin decides a decision card"}
	}
	if briefStatus(card) != cardStatusPending {
		return Result{}, &Refusal{"conflict", "the decision card is no longer pending"}
	}
	decided := cardActed(card, c, cardStatusDecided, cardOpt(act.Arg), cardOpt(act.Text))
	return s.cardFinishAct(c, decided, cardTaskStr(body.Task), OnChoose, body.To, card.By), nil
}

func cardChooseFields(c Command) bool {
	arg, argOK := c.field("arg")
	text, textOK := c.field("text")
	if !argOK || !textOK || (arg == "" && text == "") {
		return false
	}
	return len(briefJSON(text)) <= briefNoteLimit
}

func cardChoiceOK(options []proto.DecisionOption, arg string) bool {
	return arg == "" || cardHasOption(options, arg)
}

func (s *State) cardMayChoose(w Who, to []string) bool {
	return w.AI == "" && (slices.Contains(to, w.User) || s.Admin(w))
}

func (s *State) cardFinishAct(c Command, decided proto.MsgRow, taskID string, on EventKind, who []string, asker string) Result {
	if taskID == "" {
		items := s.cardItems([]TaskItem{{Why: itemDecided, AI: asker}}, "", decided.ID)
		return Result{Rows: []Row{RowOf(decided)}, Items: items}
	}
	task := s.Tasks[taskID]
	ev := Event{
		Kind: on, By: ByPerson, Now: c.Now,
		Card:    PendingCard{Kind: decided.Kind, Msg: decided.ID, Who: who, Since: decided.Timestamp},
		Asker:   asker,
		Pending: tskPending(c.Disk, task.ID),
	}
	return s.cardApplyStep(decided, task, ev)
}

func cardOpenAct(c Command, kind string, validArgs func(Command) bool) (proto.Act, proto.MsgRow, *Refusal) {
	var act proto.Act
	if refusal := c.Decode(&act); refusal != nil {
		return act, proto.MsgRow{}, refusal
	}
	if !validArgs(c) {
		return act, proto.MsgRow{}, &Refusal{"bad_request", "the act carries fields its action does not allow"}
	}
	card, ok := c.Disk.Msg(string(act.Msg))
	if !ok {
		return act, card, &Refusal{"not_found", "the act names no message"}
	}
	if card.Kind != kind {
		return act, card, &Refusal{"bad_request", "the action does not apply to this kind of message"}
	}
	return act, card, nil
}

func cardActed(card proto.MsgRow, c Command, status string, choice, text *string) proto.MsgRow {
	by, at, act := c.Who.Human, c.Now, c.Env
	card.State = &proto.CardState{Status: status, By: &by, At: &at, Choice: choice, Text: text, Act: &act}
	return card
}

func cardOpt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *State) cardApplyStep(card proto.MsgRow, task proto.TaskRow, ev Event) Result {
	moved, fx, _ := Step(task, ev)
	rows := []Row{RowOf(card)}
	if !reflect.DeepEqual(moved, task) {
		rows = append(rows, RowOf(moved))
	}
	return Result{Rows: rows, Items: s.cardItems(fx.Items, task.ID, card.ID)}
}

func (s *State) cardItems(items []TaskItem, task, msg string) []Item {
	var out []Item
	for _, it := range items {
		if _, ok := s.AIs[it.AI]; ok {
			out = append(out, Item{AI: it.AI, Why: it.Why, Task: task, Msg: msg})
		}
	}
	return out
}
