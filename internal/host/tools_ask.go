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

var (
	askToolSchema   = json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"to":{"type":"string"},"wait":{"type":"boolean","default":true}},"required":["to","text"],"additionalProperties":false}`)
	replyToolSchema = json.RawMessage(`{"type":"object","properties":{"call":{"type":"string"},"text":{"type":"string"}},"required":["call","text"],"additionalProperties":false}`)
)

func init() {
	Register(Tools, "ask", Tool{
		Description: "Ask another AI in the group; wait for the answer up to 45 s, or get a receipt.",
		Schema:      askToolSchema,
		Call:        runAskTool,
	})
	Register(Tools, "reply", Tool{
		Description: "Reply to an open call from another AI in the group.",
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

type askDenied struct {
	Denied bool `json:"denied"`
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
	return sendAskCall(ctx, h, c, callee, in, c.Turn.Depth+1, askWaitDuration(h, c.Group))
}

func resolveAskCallee(rows *Rows, group, selfID, target string) (proto.ID, error) {
	var valid []string
	for _, m := range liveMembers(rows, group) {
		if m.Kind == "ai" && m.ID != selfID {
			if m.Name == target {
				return proto.ID(m.ID), nil
			}
			valid = append(valid, m.Name)
		}
	}
	slices.Sort(valid)
	return "", missingTargetError("AI member", target, valid)
}

func askWaitDuration(h *Host, group string) time.Duration {
	grp, _ := lookupRow[proto.GroupRow](h, group, "group", group)
	return time.Duration(grp.Limits.AskWaitS) * time.Second
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
	itemCh, done := h.Await(string(callID), nil)
	defer done()
	if _, err := h.Command(ctx, home, payload); err != nil {
		return nil, err
	}
	raw, ok := awaitApprovalItem(ctx, waitDur, itemCh, done)
	if !ok {
		return askPending{Pending: true, Receipt: string(callID)}, nil
	}
	return decodeReplyItem(raw), nil
}

func decodeReplyItem(raw json.RawMessage) any {
	var item proto.InboxItem
	var msg proto.MsgRow
	proto.Decode(raw, &item)
	proto.Decode(item.Msg, &msg)
	if msg.Kind == "confirm" {
		return askDenied{Denied: true}
	}
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
	msg, err := fetchCallMsg(ctx, h, c, in.Call)
	if err != nil {
		return nil, err
	}
	callID := proto.ID(msg.ID)
	payload := marshalWire(proto.Post[proto.EmptyBody]{
		HomeCommand: homeHeader(h, c.Group, c.Member, "post"),
		Kind:        "reply", Thread: (*proto.ID)(msg.Thread), Text: in.Text,
		Mentions: []proto.ID{proto.ID(msg.By)}, Re: &callID, Body: proto.EmptyBody{},
	})
	if _, err := h.Command(ctx, c.Home, payload); err != nil {
		return nil, err
	}
	return secOKReply{OK: true}, nil
}

func fetchCallMsg(ctx context.Context, h *Host, c Caller, callID string) (proto.MsgRow, error) {
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
