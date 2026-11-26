package home

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Confirmation cards across owners: when to ask for mentions and calls, post of
// kind confirm (watch), act confirm and deny on confirm cards, card expiry, and
// confirm reminders (planning/contracts/step-2/confirm.md sections 1 to 5;
// planning/contracts/step-2/authority.md sections 1 and 2;
// planning/contracts/step-2/watch-team.md section 2).

func init() {
	Commands.Add("post/confirm", cfmPost)
	Commands["act/confirm"] = cfmRoute(Commands["act/confirm"], briefConfirmed)
	Commands["act/deny"] = cfmRoute(Commands["act/deny"], cardStatusDenied)
	Timers.Add("confirm", cfmExpire)
	wpushExtractors[cfmKind] = cfmReminder
}

const (
	cfmKind       = "confirm"
	cfmWhatDirect = "direct"
	cfmWhatAsk    = "ask"
	cfmWhatWatch  = "watch"
	cfmDeniedCode = "confirm_denied"
)

func (s *State) cfmGateItems(c Command, what string, msg proto.MsgRow, rows []Row, items []Item) Result {
	var kept []Item
	var cards []proto.MsgRow
	for _, it := range items {
		if s.cfmAllowed(c.Disk, c.Who.User, msg.Thread, it.AI) {
			kept = append(kept, it)
			continue
		}
		cards = append(cards, s.cfmNewCard(c, what, msg, s.AIs[it.AI]))
	}
	return Result{Rows: cfmAppendCards(rows, cards), Items: kept}
}

func (s *State) cfmAllowed(d Disk, user string, thread *string, aiID string) bool {
	if cfmEnough(user, s.AIs[aiID]) {
		return true
	}
	if thread == nil {
		return false
	}
	task := s.Tasks[*thread]
	return !cftHeld(cftCard(d, task, aiID)) && cfmTakesPart(task, aiID)
}

func cfmEnough(user string, ai proto.AIMemberRow) bool {
	if user == ai.User {
		return true
	}
	return (ai.Account == "team" || ai.Account == "api_key") && slices.Contains(ai.Allow, user)
}

func cfmTakesPart(task proto.TaskRow, ai string) bool {
	if !cfmTaskRunning(task.State) {
		return false
	}
	if ai == task.Assignee {
		return true
	}
	return ai == task.Reviewer && cfmReviewerStarted(task)
}

func cfmTaskRunning(state string) bool {
	return state != "" && state != taskQueued
}

func cfmReviewerStarted(task proto.TaskRow) bool {
	return task.Step == stepReview || (task.Review != nil && task.Review.By == task.Reviewer)
}

func (s *State) cfmNewCard(c Command, what string, req proto.MsgRow, ai proto.AIMemberRow) proto.MsgRow {
	re := req.ID
	body := proto.ConfirmBody{What: what, AI: ai.ID, To: []string{ai.User}}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: cfmCardID(proto.ID(req.ID), ai.ID)}, Group: s.ID,
		Thread: req.Thread, Main: true, By: "system", Kind: cfmKind, Timestamp: c.Now,
		Mentions: []string{}, Re: &re, Body: briefJSON(body), State: &proto.CardState{Status: cardStatusPending},
	}
}

func cfmCardID(nonce proto.ID, ai string) string {
	sum := sha256.Sum256([]byte("SoloDawn confirm " + string(nonce) + " " + ai))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func cfmAppendCards(rows []Row, cards []proto.MsgRow) []Row {
	task, inThread := rows[len(rows)-1].Value().(proto.TaskRow)
	if !inThread {
		return cfmAddMsgRows(rows, cards)
	}
	out := cfmAddMsgRows(rows[:len(rows)-1], cards)
	for _, card := range cards {
		task = postAdvanceThread(task, card)
	}
	return append(out, RowOf(task))
}

func cfmAddMsgRows(rows []Row, cards []proto.MsgRow) []Row {
	out := slices.Clone(rows)
	for _, card := range cards {
		out = append(out, RowOf(card))
	}
	return out
}

func cfmPost(s *State, c Command) (Result, *Refusal) {
	if c.Who.AI != "" {
		return Result{}, &Refusal{"forbidden", "only a person posts a confirm card"}
	}
	var p proto.Post[proto.ConfirmSigned]
	if refusal := c.Decode(&p); refusal != nil {
		return Result{}, refusal
	}
	if !cfmWatchShape(p) {
		return Result{}, &Refusal{"bad_request", "the confirm card does not match the watch contract"}
	}
	ai, refusal := s.cfmWatchTarget(c.Who.User, string(p.Body.AI))
	if refusal != nil {
		return Result{}, refusal
	}
	body := proto.ConfirmBody{What: cfmWhatWatch, AI: ai.ID, To: []string{ai.User}}
	row := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, Main: true,
		By: c.Who.Human, Kind: cfmKind, Timestamp: c.Timestamp, Mentions: []string{},
		Body: briefJSON(body), State: &proto.CardState{Status: cardStatusPending}, Env: &c.Env,
	}
	return Result{Rows: []Row{RowOf(row)}}, nil
}

func cfmWatchShape(p proto.Post[proto.ConfirmSigned]) bool {
	return p.Body.What == cfmWhatWatch && len(p.Mentions) == 0
}

func (s *State) cfmWatchTarget(user, aiID string) (proto.AIMemberRow, *Refusal) {
	ai, ok := s.AIs[aiID]
	if !ok || ai.User == user {
		return proto.AIMemberRow{}, &Refusal{"conflict", "watch names an AI member owned by another person"}
	}
	return ai, nil
}

func cfmRoute(prev Func, status string) Func {
	return func(s *State, c Command) (Result, *Refusal) {
		msg, _ := c.field("msg")
		if card, ok := c.Disk.Msg(msg); ok && card.Kind == cfmKind {
			return s.cfmDecide(c, status)
		}
		return prev(s, c)
	}
}

func (s *State) cfmDecide(c Command, status string) (Result, *Refusal) {
	_, card, refusal := cardOpenAct(c, cfmKind, briefBare)
	if refusal != nil {
		return Result{}, refusal
	}
	var body proto.ConfirmBody
	json.Unmarshal(card.Body, &body)
	if !cfmMayDecide(c.Who, body.To) {
		return Result{}, &Refusal{"forbidden", "only the AI's owner decides a confirm card"}
	}
	if briefStatus(card) != cardStatusPending || s.cfmDead(c.Disk, card, body) {
		return Result{}, &Refusal{"conflict", "the confirm card is no longer pending"}
	}
	decided := cardActed(card, c, status, nil, nil)
	return s.cfmApplyDecision(c, decided, body, status), nil
}

func cfmMayDecide(w Who, to []string) bool {
	return w.AI == "" && slices.Contains(to, w.User)
}

func (s *State) cfmApplyDecision(c Command, card proto.MsgRow, body proto.ConfirmBody, status string) Result {
	if body.What == cfmWhatDirect {
		req, _ := c.Disk.Msg(*card.Re)
		if cftRoleRequest(req) {
			return s.cftDecide(c, card, body)
		}
	}
	if status == briefConfirmed {
		return Result{Rows: []Row{RowOf(card)}, Items: s.cfmConfirmedItems(c.Disk, card, body)}
	}
	return s.cfmDeniedResult(c, card, body)
}

func (s *State) cfmConfirmedItems(d Disk, card proto.MsgRow, body proto.ConfirmBody) []Item {
	if body.What == cfmWhatWatch {
		return []Item{{AI: body.AI, Why: cfmWhatWatch, Msg: card.ID}}
	}
	req, _ := d.Msg(*card.Re)
	return []Item{{AI: body.AI, Why: postMentionWhy, Task: postTaskID(req.Thread), Msg: req.ID}}
}

func (s *State) cfmDeniedResult(c Command, card proto.MsgRow, body proto.ConfirmBody) Result {
	switch body.What {
	case cfmWhatDirect:
		req, _ := c.Disk.Msg(*card.Re)
		if _, byAI := s.AIs[req.By]; byAI {
			return Result{Rows: []Row{RowOf(card)}, Items: s.cfmNotice(card)}
		}
		return Result{Rows: []Row{RowOf(card)}}
	case cfmWhatAsk:
		call, _ := c.Disk.Msg(*card.Re)
		at := c.Now
		call.State = &proto.CardState{Status: cardStatusDenied, At: &at}
		items := append(s.cardItems([]TaskItem{{Why: calWhyReply, AI: call.By}}, postTaskID(call.Thread), card.ID), s.cfmNotice(card)...)
		return Result{Rows: []Row{RowOf(card), RowOf(call)}, Items: items}
	default:
		return Result{Rows: []Row{RowOf(card)}}
	}
}

func (s *State) cfmNotice(card proto.MsgRow) []Item {
	ctx := tskItemCtx{taskID: postTaskID(card.Thread), secretary: s.Group.Secretary, noticeMsg: card.ID}
	return ctx.items([]TaskItem{{Why: itemNotice, Code: cfmDeniedCode}})
}

func cfmExpire(s *State, d Disk, now int64) Result {
	var rows []Row
	for _, card := range d.Pending() {
		if s.cfmShouldExpire(d, card) {
			at := now
			card.State = &proto.CardState{Status: "expired", At: &at}
			rows = append(rows, RowOf(card))
		}
	}
	return Result{Rows: rows}
}

func (s *State) cfmShouldExpire(d Disk, card proto.MsgRow) bool {
	if card.Kind != cfmKind {
		return false
	}
	var body proto.ConfirmBody
	json.Unmarshal(card.Body, &body)
	return s.cfmDead(d, card, body)
}

func (s *State) cfmDead(d Disk, card proto.MsgRow, body proto.ConfirmBody) bool {
	if _, ok := s.AIs[body.AI]; !ok {
		return true
	}
	if body.What == cfmWhatWatch {
		_, ok := s.People[card.By]
		return !ok
	}
	req, ok := d.Msg(*card.Re)
	if !ok {
		return true
	}
	return cftRoleRequest(req) && !s.cftHasRole(req.ID, body.AI)
}

func cfmReminder(s *State, msg proto.MsgRow) wpushEvent {
	var b proto.ConfirmBody
	json.Unmarshal(msg.Body, &b)
	return wpushEvent{kind: cfmKind, who: s.AIs[b.AI].Name, users: b.To}
}
