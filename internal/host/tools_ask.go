package host

import (
	"cmp"
	"context"
	"crypto/rand"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	askproto "github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	defaultAskDepth   = 4
	defaultAskWaitSec = 45
)

// askWaitUnit scales limits.ask_wait_s into a time.Duration.
var askWaitUnit = time.Second

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

type replyAck struct {
	OK bool `json:"ok"`
}

func parseCallArgs[T any](raw json.RawMessage, allowed ...string) (T, error) {
	var out T
	if err := askproto.Decode(raw, &out); err != nil {
		return out, &Error{Code: "bad_request", Message: err.Error()}
	}
	var obj map[string]json.RawMessage
	json.Unmarshal(raw, &obj)
	for key := range obj {
		if !slices.Contains(allowed, key) {
			return out, &Error{Code: "bad_request", Message: "unexpected argument: " + key}
		}
	}
	return out, nil
}

func guardAskSecret(text string) error {
	if err := CheckOutboundSecret(text); err != nil {
		return &Error{Code: "secret", Message: "this looks like a secret; remove it and send again"}
	}
	return nil
}

func runAskTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	in, err := parseCallArgs[askArgs](raw, "to", "text", "wait")
	if err != nil {
		return nil, err
	}
	if err := guardAskSecret(in.Text); err != nil {
		return nil, err
	}
	callee, err := resolveAskCallee(h, c, in.To)
	if err != nil {
		return nil, err
	}
	maxDepth, waitDur := groupAskLimits(h, c.Group)
	depth := c.Turn.Depth + 1
	if depth > maxDepth {
		return nil, &Error{
			Code:    "limit",
			Message: fmt.Sprintf("call depth %d exceeds ask_depth limit %d", depth, maxDepth),
		}
	}
	return sendAskCall(ctx, h, c, callee, in, depth, waitDur)
}

func resolveAskCallee(h *Host, c Caller, target string) (askproto.ID, error) {
	members, hasSelf := groupMembers(h, c.Group, c.Member)
	for _, m := range members {
		if m.Name == target {
			return checkCalleeMember(m, c.Member)
		}
	}
	if isUnlistedSelf(hasSelf, c.Name, target) {
		return "", &Error{Code: "bad_request", Message: "an AI cannot call itself"}
	}
	return "", unknownAINameError(target, otherAINames(members, c.Member))
}

func isUnlistedSelf(hasSelf bool, selfName, target string) bool {
	return !hasSelf && selfName != "" && target == selfName
}

func groupMembers(h *Host, group, selfID string) ([]askproto.MemberRow, bool) {
	var list []askproto.MemberRow
	hasSelf := false
	for _, raw := range h.Rows.Of(group, "member") {
		var m askproto.MemberRow
		if askproto.Decode(raw, &m) == nil {
			list = append(list, m)
			hasSelf = hasSelf || m.ID == selfID
		}
	}
	return list, hasSelf
}

func checkCalleeMember(m askproto.MemberRow, selfID string) (askproto.ID, error) {
	if m.ID == selfID {
		return "", &Error{Code: "bad_request", Message: "an AI cannot call itself"}
	}
	if m.Kind != "ai" {
		return "", &Error{Code: "bad_request", Message: fmt.Sprintf("%q is not an AI member", m.Name)}
	}
	return askproto.ID(m.ID), nil
}

func otherAINames(members []askproto.MemberRow, selfID string) []string {
	var names []string
	for _, m := range members {
		if m.Kind == "ai" && m.ID != selfID {
			names = append(names, m.Name)
		}
	}
	slices.Sort(names)
	return names
}

func unknownAINameError(target string, valid []string) error {
	available := "none"
	if len(valid) != 0 {
		available = strings.Join(valid, ", ")
	}
	return &Error{
		Code:    "not_found",
		Message: fmt.Sprintf("unknown AI member %q; valid AI members: %s", target, available),
	}
}

func groupAskLimits(h *Host, group string) (int, time.Duration) {
	maxDepth, waitSec := defaultAskDepth, defaultAskWaitSec
	raw, ok := h.Rows.Get(group, "group", group)
	if !ok {
		return maxDepth, time.Duration(waitSec) * askWaitUnit
	}
	var grp askproto.GroupRow
	if askproto.Decode(raw, &grp) != nil {
		return maxDepth, time.Duration(waitSec) * askWaitUnit
	}
	if grp.Limits.AskDepth > 0 {
		maxDepth = grp.Limits.AskDepth
	}
	if grp.Limits.AskWaitS > 0 {
		waitSec = grp.Limits.AskWaitS
	}
	return maxDepth, time.Duration(waitSec) * askWaitUnit
}

func askThread(c Caller) *askproto.ID {
	tid := cmp.Or(c.Task, c.Turn.Thread)
	if tid == "" {
		return nil
	}
	id := askproto.ID(tid)
	return &id
}

func newCallID() askproto.ID {
	var raw [16]byte
	rand.Read(raw[:])
	return askproto.ID(b64.RawURLEncoding.EncodeToString(raw[:]))
}

func marshalCallCmd(v any) string {
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(out.String(), "\n")
}

func sendAskCall(ctx context.Context, h *Host, c Caller, callee askproto.ID, in askArgs, depth int, waitDur time.Duration) (any, error) {
	wait := in.Wait == nil || *in.Wait
	callID := newCallID()
	payload := buildCallPostPayload(h, c, callID, callee, in.Text, depth, wait)
	if !wait {
		if _, err := h.Command(ctx, c.Home, payload); err != nil {
			return nil, err
		}
		return askPending{Pending: true, Receipt: string(callID)}, nil
	}
	return sendWaitingCall(ctx, h, c.Home, callID, payload, waitDur)
}

func buildCallPostPayload(h *Host, c Caller, callID, callee askproto.ID, text string, depth int, wait bool) string {
	return marshalCallCmd(askproto.Post[askproto.CallBody]{
		HomeCommand: askproto.HomeCommand{
			Type:      "post",
			Nonce:     callID,
			Timestamp: h.Now().UnixMilli(),
			Group:     askproto.ID(c.Group),
			As:        askproto.ID(c.Member),
		},
		Kind:     "call",
		Thread:   askThread(c),
		Main:     false,
		Text:     text,
		Mentions: []askproto.ID{callee},
		Re:       nil,
		Body: askproto.CallBody{
			To:    string(callee),
			Depth: depth,
			Wait:  wait,
		},
	})
}

func sendWaitingCall(ctx context.Context, h *Host, home string, callID askproto.ID, payload string, waitDur time.Duration) (any, error) {
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
		return decodeReplyItem(raw)
	}
	return finishAskTimeout(itemCh, done, string(callID))
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

func finishAskTimeout(itemCh <-chan json.RawMessage, done func(), callID string) (any, error) {
	done()
	select {
	case raw := <-itemCh:
		return decodeReplyItem(raw)
	default:
		return askPending{Pending: true, Receipt: callID}, nil
	}
}

func decodeReplyItem(raw json.RawMessage) (any, error) {
	var item askproto.InboxItem
	var msg askproto.MsgRow
	if askproto.Decode(raw, &item) != nil || len(item.Msg) == 0 || askproto.Decode(item.Msg, &msg) != nil {
		return nil, &Error{Code: "bad_request", Message: "invalid reply item"}
	}
	return askAnswered{Answer: msg.Text}, nil
}

func runReplyTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	in, err := parseCallArgs[replyArgs](raw, "call", "text")
	if err != nil {
		return nil, err
	}
	if err := guardAskSecret(in.Text); err != nil {
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

func sendReplyPost(ctx context.Context, h *Host, c Caller, msg askproto.MsgRow, text string) (any, error) {
	thread, callerAI, err := verifyCallTarget(msg, c.Member)
	if err != nil {
		return nil, err
	}
	callID := askproto.ID(msg.ID)
	payload := marshalCallCmd(askproto.Post[askproto.EmptyBody]{
		HomeCommand: askproto.HomeCommand{
			Type:      "post",
			Nonce:     newCallID(),
			Timestamp: h.Now().UnixMilli(),
			Group:     askproto.ID(c.Group),
			As:        askproto.ID(c.Member),
		},
		Kind:     "reply",
		Thread:   thread,
		Main:     false,
		Text:     text,
		Mentions: []askproto.ID{callerAI},
		Re:       &callID,
		Body:     askproto.EmptyBody{},
	})
	if _, err := h.Command(ctx, c.Home, payload); err != nil {
		return nil, err
	}
	return replyAck{OK: true}, nil
}

func loadCallMsg(ctx context.Context, h *Host, c Caller, callID string) (askproto.MsgRow, error) {
	raw, ok := h.Rows.Get(c.Group, "msg", callID)
	if !ok {
		var err error
		raw, err = fetchCallRow(ctx, h, c.Home, c.Group, callID)
		if err != nil {
			return askproto.MsgRow{}, err
		}
	}
	var msg askproto.MsgRow
	if askproto.Decode(raw, &msg) != nil || msg.ID != callID {
		return askproto.MsgRow{}, &Error{Code: "bad_request", Message: "invalid call message"}
	}
	return msg, nil
}

func fetchCallRow(ctx context.Context, h *Host, device, group, callID string) (json.RawMessage, error) {
	conn, ok := h.Home(device)
	if !ok {
		return nil, &Error{Code: "home_unreachable", Message: "the home of that group is not connected"}
	}
	build := func(id int64) any {
		return askproto.FetchRow{
			FetchPage: askproto.FetchPage{
				Request: askproto.Request{Type: "fetch", ID: id},
				Group:   group,
				Before:  0,
				Limit:   1,
			},
			Row:   "msg",
			RowID: callID,
		}
	}
	ans, err := conn.Request(ctx, build)
	if err != nil {
		return nil, &Error{Code: "home_unreachable", Message: "the home of that group is not connected"}
	}
	return unpackFetchedRow(ans, callID)
}

func unpackFetchedRow(ans json.RawMessage, callID string) (json.RawMessage, error) {
	var wireErr askproto.ErrorResponse
	if askproto.Decode(ans, &wireErr) == nil {
		return nil, &Error{Code: wireErr.Err.Code, Message: wireErr.Err.Message}
	}
	var page askproto.Fetched
	if askproto.Decode(ans, &page) != nil {
		return nil, &Error{Code: "bad_request", Message: "invalid fetched response"}
	}
	if len(page.Rows) == 0 {
		return nil, &Error{Code: "not_found", Message: fmt.Sprintf("unknown call %q", callID)}
	}
	return page.Rows[0], nil
}

func verifyCallTarget(msg askproto.MsgRow, responder string) (*askproto.ID, askproto.ID, error) {
	if msg.Kind != "call" {
		return nil, "", &Error{Code: "conflict", Message: "the target message is not a call"}
	}
	var body askproto.CallBody
	if askproto.Decode(msg.Body, &body) != nil {
		return nil, "", &Error{Code: "bad_request", Message: "invalid call body"}
	}
	if body.To != responder || !isCallWaiting(msg.State) {
		return nil, "", &Error{Code: "conflict", Message: "the call is not waiting for this AI"}
	}
	return optionalThreadID(msg.Thread), askproto.ID(msg.By), nil
}

func isCallWaiting(state *askproto.CardState) bool {
	return state != nil && state.Status == "waiting"
}

func optionalThreadID(raw *string) *askproto.ID {
	if raw == nil {
		return nil
	}
	id := askproto.ID(*raw)
	return &id
}
