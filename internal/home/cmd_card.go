package home

import (
	"encoding/json"
	"reflect"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Approval and decision cards: post of kinds approval and decision, act
// approve, deny and choose on them, and the pending-card helpers shared by
// task transitions (planning/contracts/commands.md 1.3 and 1.4;
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

	cardStatusPending    = "pending"
	cardStatusApproved   = "approved"
	cardStatusDenied     = "denied"
	cardStatusDecided    = "decided"
	cardStatusExpired    = "expired"
	cardStatusReassigned = "reassigned"

	// cardCommandLimit and cardWhyLimit are the most bytes command and why of
	// an approval body may have as JSON strings, quotes included
	// (planning/contracts/messages.md 2.3).
	cardCommandLimit = 3072
	cardWhyLimit     = 256
	cardPreviewLimit = 200
)

var cardCategories = []string{"command", "file", "network", "other"}

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
	mentions, refusal := s.cardMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	spec := cardPostSpec{
		kind: cardKindApproval, on: OnApproval, taskID: p.Body.Task, who: p.Body.Approvers,
		text: p.Text, mentions: mentions, re: p.Re, body: p.Body,
	}
	return s.cardPostOnTask(c, spec)
}

func cardApprovalShape(p proto.Post[proto.ApprovalBody], ai string) bool {
	if p.Thread == nil || string(*p.Thread) != p.Body.Task || p.Body.AI != ai {
		return false
	}
	return cardApprovalBody(p.Body)
}

func cardApprovalBody(b proto.ApprovalBody) bool {
	if !slices.Contains(cardCategories, b.Category) || !cardUsers(b.Approvers) {
		return false
	}
	if len(briefJSON(b.Command)) > cardCommandLimit || !cardFullOK(b.Full, b.Command) {
		return false
	}
	return cardWhyOK(b.Why)
}

func cardFullOK(full *int, command string) bool {
	return full == nil || *full > len(command)
}

func cardWhyOK(why *string) bool {
	if why == nil {
		return true
	}
	return *why != "" && len(briefJSON(*why)) <= cardWhyLimit
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
	mentions, refusal := s.cardMentions(p.Mentions)
	if refusal != nil {
		return Result{}, refusal
	}
	return s.cardWriteDecision(c, p, mentions)
}

func cardDecisionShape(p proto.Post[proto.DecisionBody]) bool {
	if !cardSameTask(p.Thread, p.Body.Task) || !cardUsers(p.Body.To) {
		return false
	}
	if p.Body.Context != nil && *p.Body.Context == "" {
		return false
	}
	return cardOptionsOK(p.Body.Options, p.Body.Recommended)
}

func cardSameTask(thread *proto.ID, task *string) bool {
	if thread == nil {
		return task == nil
	}
	return task != nil && string(*thread) == *task
}

func cardOptionsOK(options []proto.DecisionOption, recommended *string) bool {
	for i, opt := range options {
		if opt.ID == "" || cardHasOption(options[:i], opt.ID) {
			return false
		}
	}
	return recommended == nil || cardHasOption(options, *recommended)
}

func cardHasOption(options []proto.DecisionOption, id string) bool {
	for _, opt := range options {
		if opt.ID == id {
			return true
		}
	}
	return false
}

func (s *State) cardWriteDecision(c Command, p proto.Post[proto.DecisionBody], mentions []string) (Result, *Refusal) {
	if p.Body.Task == nil {
		card, refusal := s.cardNewMsg(c, cardKindDecision, nil, p.Text, mentions, p.Re, p.Body)
		if refusal != nil {
			return Result{}, refusal
		}
		return Result{Rows: []Row{RowOf(card)}}, nil
	}
	spec := cardPostSpec{
		kind: cardKindDecision, on: OnDecision, taskID: *p.Body.Task, who: p.Body.To,
		text: p.Text, mentions: mentions, re: p.Re, body: p.Body,
	}
	return s.cardPostOnTask(c, spec)
}

func (s *State) cardPostOnTask(c Command, spec cardPostSpec) (Result, *Refusal) {
	task, ok := s.Tasks[spec.taskID]
	if !ok {
		return Result{}, &Refusal{"not_found", "the card names no task"}
	}
	id := string(c.Nonce)
	ev := Event{
		Kind: spec.on, By: ByAI, Member: c.Who.AI, Now: c.Now,
		Card:    PendingCard{Kind: spec.kind, Msg: id, Who: spec.who, Since: c.Timestamp},
		Pending: cardPending(c.Disk, task.ID),
	}
	moved, _, err := Step(task, ev)
	if err != nil {
		return Result{}, &Refusal{err.Error(), "the task machine refused the card"}
	}
	card, refusal := s.cardNewMsg(c, spec.kind, &task.ID, spec.text, spec.mentions, spec.re, spec.body)
	if refusal != nil {
		return Result{}, refusal
	}
	return Result{Rows: []Row{RowOf(card), RowOf(cardThread(moved, card))}}, nil
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

func cardThread(task proto.TaskRow, msg proto.MsgRow) proto.TaskRow {
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{
		By: msg.By, Kind: msg.Kind, Text: cardPreview(msg.Text), Timestamp: msg.Timestamp,
	}
	return task
}

func cardPreview(text string) string {
	runes := []rune(text)
	return string(runes[:min(len(runes), cardPreviewLimit)])
}

func cardUsers(users []string) bool {
	for i, u := range users {
		if u == "" || slices.Contains(users[:i], u) {
			return false
		}
	}
	return len(users) > 0
}

func (s *State) cardMentions(ids []proto.ID) ([]string, *Refusal) {
	list, unique := cardDistinct(ids)
	if !unique {
		return nil, &Refusal{"bad_request", "a card mentions the same member twice"}
	}
	for _, m := range list {
		if !s.cardHasMember(m) {
			return nil, &Refusal{"conflict", "a card mentions no member of the group"}
		}
	}
	return list, nil
}

func cardDistinct(ids []proto.ID) ([]string, bool) {
	seen := make(map[proto.ID]bool, len(ids))
	list := make([]string, len(ids))
	for i, id := range ids {
		if seen[id] {
			return nil, false
		}
		seen[id], list[i] = true, string(id)
	}
	return list, true
}

func (s *State) cardHasMember(id string) bool {
	if _, ok := s.People[id]; ok {
		return true
	}
	_, ok := s.AIs[id]
	return ok
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
	if !cardMayApprove(c.Who, body.Approvers) {
		return Result{}, &Refusal{"forbidden", "only a listed approver decides an approval card"}
	}
	if briefStatus(card) != cardStatusPending {
		return Result{}, &Refusal{"conflict", "the approval card is no longer pending"}
	}
	return s.cardFinishApproval(c, card, body, status)
}

func cardMayApprove(w Who, approvers []string) bool {
	return w.AI == "" && slices.Contains(approvers, w.User)
}

func (s *State) cardFinishApproval(c Command, card proto.MsgRow, body proto.ApprovalBody, status string) (Result, *Refusal) {
	task := s.Tasks[body.Task]
	if !slices.Contains(statesOpen, task.State) || task.Assignee != body.AI {
		return Result{}, &Refusal{"conflict", "the task of the approval card is no longer open for that AI"}
	}
	decided := cardActed(card, c, status, nil, nil)
	ev := Event{
		Kind: OnApprove, By: ByPerson, Now: c.Now,
		Card:    PendingCard{Kind: cardKindApproval, Msg: card.ID, Who: body.Approvers, Since: card.Timestamp},
		Asker:   body.AI,
		Pending: cardPending(c.Disk, task.ID),
	}
	return s.cardApplyStep(c, decided, task, ev), nil
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
	return s.cardFinishDecision(c, act, card, body)
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
	if len(options) == 0 {
		return arg == ""
	}
	return cardHasOption(options, arg)
}

func (s *State) cardMayChoose(w Who, to []string) bool {
	return w.AI == "" && (slices.Contains(to, w.User) || s.Admin(w))
}

func (s *State) cardFinishDecision(c Command, act proto.Act, card proto.MsgRow, body proto.DecisionBody) (Result, *Refusal) {
	decided := cardActed(card, c, cardStatusDecided, cardOpt(act.Arg), cardOpt(act.Text))
	if body.Task == nil {
		items := s.cardItems([]TaskItem{{Why: itemDecided, AI: card.By}}, "", card.ID, &c.Env)
		return Result{Rows: []Row{RowOf(decided)}, Items: items}, nil
	}
	task := s.Tasks[*body.Task]
	if !slices.Contains(statesOpen, task.State) {
		return Result{}, &Refusal{"conflict", "the task of the decision card is no longer open"}
	}
	ev := Event{
		Kind: OnChoose, By: ByPerson, Now: c.Now,
		Card:    PendingCard{Kind: cardKindDecision, Msg: card.ID, Who: body.To, Since: card.Timestamp},
		Asker:   card.By,
		Pending: cardPending(c.Disk, task.ID),
	}
	return s.cardApplyStep(c, decided, task, ev), nil
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

func (s *State) cardApplyStep(c Command, card proto.MsgRow, task proto.TaskRow, ev Event) Result {
	moved, fx, _ := Step(task, ev)
	rows := []Row{RowOf(card)}
	if !reflect.DeepEqual(moved, task) {
		rows = append(rows, RowOf(moved))
	}
	return Result{Rows: rows, Items: s.cardItems(fx.Items, task.ID, card.ID, &c.Env)}
}

func (s *State) cardItems(items []TaskItem, task, msg string, cause *proto.Envelope) []Item {
	var out []Item
	for _, it := range items {
		if _, ok := s.AIs[it.AI]; ok {
			out = append(out, Item{AI: it.AI, Why: it.Why, Task: task, Msg: msg, Cause: cause})
		}
	}
	return out
}

// cardPending returns the pending approval and decision cards of task, oldest
// first, as Event.Pending reads them.
func cardPending(d Disk, task string) []PendingCard {
	var out []PendingCard
	for _, msg := range d.Pending() {
		if cardForTask(msg, task) {
			out = append(out, PendingCard{Kind: msg.Kind, Msg: msg.ID, Who: cardWho(msg), Since: msg.Timestamp})
		}
	}
	return out
}

func cardForTask(msg proto.MsgRow, task string) bool {
	if msg.Thread == nil || *msg.Thread != task {
		return false
	}
	return msg.Kind == cardKindApproval || msg.Kind == cardKindDecision
}

func cardWho(msg proto.MsgRow) []string {
	var body struct {
		Approvers []string `json:"approvers"`
		To        []string `json:"to"`
	}
	json.Unmarshal(msg.Body, &body)
	if msg.Kind == cardKindApproval {
		return body.Approvers
	}
	return body.To
}

// cardClose marks the pending cards of task as Effects.Cards asks: expired on
// a merge or cancel (approval and decision cards), reassigned when the task's
// assignee changes (approval cards only).
func cardClose(d Disk, task, status string, now int64) []Row {
	if status == "" {
		return nil
	}
	var rows []Row
	for _, msg := range d.Pending() {
		if cardCloses(msg, task, status) {
			at := now
			msg.State = &proto.CardState{Status: status, At: &at}
			rows = append(rows, RowOf(msg))
		}
	}
	return rows
}

func cardCloses(msg proto.MsgRow, task, status string) bool {
	if !cardForTask(msg, task) {
		return false
	}
	return status == cardStatusExpired || msg.Kind == cardKindApproval
}
