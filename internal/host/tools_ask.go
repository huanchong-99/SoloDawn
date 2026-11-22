package host

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	defaultAskDepth   = 4
	defaultAskWaitSec = 45
)

var (
	askToolSchema   = json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"to":{"type":"string"},"wait":{"type":"boolean","default":true}},"required":["to","text"],"additionalProperties":false}`)
	replyToolSchema = json.RawMessage(`{"type":"object","properties":{"call":{"type":"string"},"text":{"type":"string"}},"required":["call","text"],"additionalProperties":false}`)
)

func init() {
	Register(Tools, "ask", Tool{
		Description: "Ask another AI member in the group and optionally wait for its reply.",
		Schema:      askToolSchema,
		Call:        runAskTool,
	})
	Register(Tools, "reply", Tool{
		Description: "Reply to an open call from another AI member.",
		Schema:      replyToolSchema,
		Call:        runReplyTool,
	})
}

type askArgs struct {
	To   string `json:"to"`
	Text string `json:"text"`
	Wait *bool  `json:"wait,omitempty"`
}

type replyArgs struct {
	Call string `json:"call"`
	Text string `json:"text"`
}

type askAnswered struct {
	Answer string `json:"answer"`
}

type askPending struct {
	Pending bool   `json:"pending"`
	Receipt string `json:"receipt"`
}

func runAskTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	in, err := decodeSecInput[askArgs](raw)
	if err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Text); err != nil {
		return nil, err
	}
	callee, err := resolveAskCallee(h.Rows, c.Group, c.Member, in.To)
	if err != nil {
		return nil, err
	}
	maxDepth, waitDur := groupAskLimits(h, c.Group)
	depth := c.Turn.Depth + 1
	if depth > maxDepth {
		return nil, &Error{Code: "limit", Message: fmt.Sprintf("call depth %d exceeds ask_depth limit %d", depth, maxDepth)}
	}
	return sendAskCall(ctx, h, c, callee, in, depth, waitDur)
}

func resolveAskCallee(rows *Rows, group, selfID, target string) (proto.ID, error) {
	members := liveMembers(rows, group)
	for _, m := range members {
		if m.Name == target {
			return checkCalleeMember(m, selfID)
		}
	}
	return "", missingTargetError("AI member", target, otherAINames(members, selfID))
}

func checkCalleeMember(m proto.MemberRow, selfID string) (proto.ID, error) {
	if m.ID == selfID {
		return "", &Error{Code: "bad_request", Message: "an AI cannot call itself"}
	}
	if m.Kind != "ai" {
		return "", &Error{Code: "bad_request", Message: fmt.Sprintf("%q is not an AI member", m.Name)}
	}
	return proto.ID(m.ID), nil
}

func otherAINames(members []proto.MemberRow, selfID string) []string {
	var names []string
	for _, m := range members {
		if m.Kind == "ai" && m.ID != selfID {
			names = append(names, m.Name)
		}
	}
	slices.Sort(names)
	return names
}

func groupAskLimits(h *Host, group string) (int, time.Duration) {
	maxDepth, waitSec := defaultAskDepth, defaultAskWaitSec
	if grp, ok := lookupRow[proto.GroupRow](h, group, "group", group); ok {
		if grp.Limits.AskDepth > 0 {
			maxDepth = grp.Limits.AskDepth
		}
		if grp.Limits.AskWaitS > 0 {
			waitSec = grp.Limits.AskWaitS
		}
	}
	return maxDepth, time.Duration(waitSec) * time.Second
}

func askThread(c Caller) *proto.ID {
	tid := cmp.Or(c.Task, c.Turn.Thread)
	if tid == "" {
		return nil
	}
	id := proto.ID(tid)
	return &id
}

func sendAskCall(ctx context.Context, h *Host, c Caller, callee proto.ID, in askArgs, depth int, waitDur time.Duration) (any, error) {
	wait := in.Wait == nil || *in.Wait
	hdr := homeHeader(h, c.Group, c.Member, "post")
	payload := marshalWire(proto.Post[proto.CallBody]{
		HomeCommand: hdr, Kind: "call", Thread: askThread(c), Text: in.Text, Mentions: []proto.ID{callee},
		Body: proto.CallBody{To: string(callee), Depth: depth, Wait: wait},
	})
	if !wait {
		if _, err := h.Command(ctx, c.Home, payload); err != nil {
			return nil, err
		}
		return askPending{Pending: true, Receipt: string(hdr.Nonce)}, nil
	}
	return sendWaitingCall(ctx, h, c.Home, hdr.Nonce, payload, waitDur)
}

func sendWaitingCall(ctx context.Context, h *Host, home string, callID proto.ID, payload string, waitDur time.Duration) (any, error) {
	// Single reader: waitAskReply while the wait is open, then finishAskTimeout after done().
	itemCh, done := h.Await(string(callID), nil)
	if _, err := h.Command(ctx, home, payload); err != nil {
		done()
		return nil, err
	}
	raw, got, err := waitAskReply(ctx, itemCh, waitDur)
	if err != nil {
		done()
		return nil, err
	}
	if got {
		done()
		return decodeReplyItem(raw), nil
	}
	return finishAskTimeout(itemCh, done, string(callID)), nil
}

func waitAskReply(ctx context.Context, itemCh <-chan json.RawMessage, waitDur time.Duration) (json.RawMessage, bool, error) {
	timer := time.NewTimer(waitDur)
	defer timer.Stop()
	select {
	case raw := <-itemCh:
		return raw, true, nil
	case <-timer.C:
		return nil, false, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

func finishAskTimeout(itemCh <-chan json.RawMessage, done func(), callID string) any {
	done()
	select {
	case raw := <-itemCh:
		return decodeReplyItem(raw)
	default:
		return askPending{Pending: true, Receipt: callID}
	}
}

func decodeReplyItem(raw json.RawMessage) askAnswered {
	var item proto.InboxItem
	var msg proto.MsgRow
	proto.Decode(raw, &item)
	proto.Decode(item.Msg, &msg)
	return askAnswered{Answer: msg.Text}
}

func runReplyTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	in, err := decodeSecInput[replyArgs](raw)
	if err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Text); err != nil {
		return nil, err
	}
	if in.Call == "" {
		return nil, &Error{Code: "bad_request", Message: "call id must not be empty"}
	}
	msg, err := loadCallMsg(ctx, h, c, in.Call)
	if err != nil {
		return nil, err
	}
	return sendReplyPost(ctx, h, c, msg, in.Text)
}

func loadCallMsg(ctx context.Context, h *Host, c Caller, callID string) (proto.MsgRow, error) {
	if msg, ok := lookupRow[proto.MsgRow](h, c.Group, "msg", callID); ok {
		return msg, nil
	}
	build := func(id int64) any {
		return proto.FetchRow{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Limit: 1}, Row: "msg", RowID: callID}
	}
	page, err := fetchFrom(ctx, h, c.Home, build)
	if err != nil {
		return proto.MsgRow{}, err
	}
	if len(page.Rows) == 0 {
		return proto.MsgRow{}, &Error{Code: "not_found", Message: fmt.Sprintf("unknown call %q", callID)}
	}
	var msg proto.MsgRow
	proto.Decode(page.Rows[0], &msg)
	return msg, nil
}

func sendReplyPost(ctx context.Context, h *Host, c Caller, msg proto.MsgRow, text string) (any, error) {
	thread, callerAI, err := verifyCallTarget(msg, c.Member)
	if err != nil {
		return nil, err
	}
	callID := proto.ID(msg.ID)
	payload := marshalWire(proto.Post[proto.EmptyBody]{
		HomeCommand: homeHeader(h, c.Group, c.Member, "post"),
		Kind:        "reply", Thread: thread, Text: text, Mentions: []proto.ID{callerAI}, Re: &callID,
		Body: proto.EmptyBody{},
	})
	if _, err := h.Command(ctx, c.Home, payload); err != nil {
		return nil, err
	}
	return secOKReply{OK: true}, nil
}

func verifyCallTarget(msg proto.MsgRow, responder string) (*proto.ID, proto.ID, error) {
	if msg.Kind != "call" {
		return nil, "", &Error{Code: "conflict", Message: "the target message is not a call"}
	}
	var body proto.CallBody
	proto.Decode(msg.Body, &body)
	if body.To != responder || !isCallWaiting(msg.State) {
		return nil, "", &Error{Code: "conflict", Message: "the call is not waiting for this AI"}
	}
	return optionalThreadID(msg.Thread), proto.ID(msg.By), nil
}

func isCallWaiting(state *proto.CardState) bool {
	return state != nil && state.Status == "waiting"
}

func optionalThreadID(raw *string) *proto.ID {
	if raw == nil {
		return nil
	}
	id := proto.ID(*raw)
	return &id
}
