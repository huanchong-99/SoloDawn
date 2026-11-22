//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_tools_ask"] = func(sc *godog.ScenarioContext, _ fixtures) {
		hostAskToolSteps(sc)
	}
}

type askStepHome struct {
	mu     sync.Mutex
	done   chan struct{}
	calls  map[string]json.RawMessage
	posts  []proto.Post[json.RawMessage]
	onCall func(callID string)
}

func newAskStepHome() *askStepHome {
	return &askStepHome{
		done:  make(chan struct{}),
		calls: map[string]json.RawMessage{},
	}
}

func (m *askStepHome) Send(any) error               { return nil }
func (m *askStepHome) Path() string                 { return "local" }
func (m *askStepHome) PathChanged() <-chan struct{} { return nil }
func (m *askStepHome) Done() <-chan struct{}        { return m.done }

func (m *askStepHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	frame := build(1)
	switch f := frame.(type) {
	case proto.Cmd:
		return m.recordCmd(f)
	case proto.FetchRow:
		return m.answerFetch(f), nil
	default:
		return nil, fmt.Errorf("unexpected frame %T", frame)
	}
}

func (m *askStepHome) recordCmd(cmd proto.Cmd) (json.RawMessage, error) {
	var post proto.Post[json.RawMessage]
	if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
		return nil, err
	}
	if refused := m.refuseDeepCall(post); refused != nil {
		return refused, nil
	}
	if hook := m.storeRecordedPost(post, cmd.Env); hook != nil {
		hook(string(post.Nonce))
	}
	return json.RawMessage(`{"re":1,"ok":true}`), nil
}

func (m *askStepHome) refuseDeepCall(post proto.Post[json.RawMessage]) json.RawMessage {
	if post.Kind != "call" {
		return nil
	}
	var body proto.CallBody
	if proto.Decode(post.Body, &body) == nil && body.Depth > 4 {
		return json.RawMessage(accJSON(proto.ErrorResponse{
			Re:  1,
			Err: proto.WireError{Code: "limit", Message: fmt.Sprintf("call depth %d exceeds ask_depth limit 4", body.Depth)},
		}))
	}
	return nil
}

func (m *askStepHome) storeRecordedPost(post proto.Post[json.RawMessage], env proto.Envelope) func(string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.posts = append(m.posts, post)
	if post.Kind == "call" {
		m.calls[string(post.Nonce)] = m.callRowBytes(post, env)
		return m.onCall
	}
	return nil
}

func (m *askStepHome) callRowBytes(post proto.Post[json.RawMessage], env proto.Envelope) json.RawMessage {
	var thread *string
	if post.Thread != nil {
		tid := string(*post.Thread)
		thread = &tid
	}
	return json.RawMessage(accJSON(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(post.Nonce), Seq: 1},
		Pos:       1,
		Group:     string(post.Group),
		Thread:    thread,
		By:        string(post.As),
		Kind:      "call",
		Timestamp: post.Timestamp,
		Text:      post.Text,
		Mentions:  []string{},
		Body:      post.Body,
		State:     &proto.CardState{Status: "waiting"},
		Env:       &env,
	}))
}

func (m *askStepHome) answerFetch(req proto.FetchRow) json.RawMessage {
	m.mu.Lock()
	row, ok := m.calls[req.RowID]
	m.mu.Unlock()
	rows := []json.RawMessage{}
	if ok {
		rows = append(rows, row)
	}
	return json.RawMessage(accJSON(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: 1, OK: true},
		Rows:    rows,
	}))
}

type askStepWorld struct {
	h          *host.Host
	home       *askStepHome
	ids        seqIDGen
	groupID    string
	taskID     string
	ownerUID   string
	members    map[string]string
	lastResult map[string]any
	lastErr    error
	receipt    string
}

func (w *askStepWorld) setup(cleanup func(func()), waitSec int) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize)))
	w.h, _, err = host.New(db, key, func() time.Time { return time.UnixMilli(1_763_600_000_000) })
	if err != nil {
		return err
	}
	w.home = newAskStepHome()
	w.h.Attach("home-device", w.home)
	w.groupID = string(w.ids.next())
	w.taskID = string(w.ids.next())
	w.ownerUID = string(w.ids.next())
	w.members = map[string]string{
		"Claude Code": string(w.ids.next()),
		"Codex":       string(w.ids.next()),
	}
	w.seedRows(waitSec)
	return nil
}

func (w *askStepWorld) seedRows(waitSec int) {
	w.h.Rows.Put(w.groupID, "group", w.groupID, json.RawMessage(accJSON(proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: w.groupID, Seq: 1},
		Name:           "Website",
		Kind:           "project",
		Repo:           "https://git.example/team/website.git",
		Base:           "main",
		Forge:          "github",
		Home:           "home-device",
		Checks:         []string{},
		Pass:           90,
		PassFoundation: 70,
		Limits:         proto.Limits{AskDepth: 4, AskWaitS: waitSec},
		Created:        1_763_600_000_000,
	})))
	for name, id := range w.members {
		w.h.Rows.Put(w.groupID, "member", id, json.RawMessage(accJSON(proto.AIMemberRow{
			MemberRow: proto.MemberRow{
				RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 2},
				Group:     w.groupID,
				Kind:      "ai",
				User:      w.ownerUID,
				Name:      name,
				LastSeen:  1_763_600_000_000,
			},
			CLI:       "claude",
			Host:      w.h.Key.ID(),
			HostName:  "workstation",
			Account:   "personal",
			Allow:     []string{},
			Approvers: []string{w.ownerUID},
			State:     "working",
			Since:     1_763_600_000_000,
			Path:      "local",
		})))
	}
}

func (w *askStepWorld) caller(name string, depth int) host.Caller {
	return host.Caller{
		Session: host.Session{
			SessionKey: host.SessionKey{Group: w.groupID, Member: w.members[name], Task: w.taskID},
			Kind:       host.TaskSession,
			Folder:     "wt/12-path",
			Agent:      "claude",
			Home:       "home-device",
			Name:       name,
			State:      host.Working,
		},
		Level: 2,
		Turn:  host.Turn{Depth: depth, Thread: w.taskID},
	}
}

func (w *askStepWorld) callTool(name string, c host.Caller, args map[string]any) error {
	val, err := host.Tools[name].Call(context.Background(), w.h, c, json.RawMessage(accJSON(args)))
	w.lastErr = err
	w.lastResult = nil
	if err == nil {
		return json.Unmarshal([]byte(accJSON(val)), &w.lastResult)
	}
	return nil
}

func (w *askStepWorld) replyInboxItem(callID, byName, text string) json.RawMessage {
	replyID := w.ids.next()
	msg := proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(replyID), Seq: 5},
		Pos:       5,
		Group:     w.groupID,
		Thread:    &w.taskID,
		By:        w.members[byName],
		Kind:      "reply",
		Timestamp: 1_763_600_000_500,
		Text:      text,
		Mentions:  []string{},
		Re:        &callID,
		Body:      proto.EmptyBody{},
	}
	return json.RawMessage(accJSON(proto.InboxItem{
		Item: proto.Item{Item: 1, AI: proto.ID(w.members["Claude Code"]), Why: "reply", Task: proto.ID(w.taskID), Msg: replyID},
		Msg:  json.RawMessage(accJSON(msg)),
	}))
}

func hostAskToolSteps(sc *godog.ScenarioContext) {
	w := &askStepWorld{}
	cleanup := afterScenario(sc)

	sc.Step(`^a host group with AIs "Claude Code" and "Codex" on task "T-12"$`, func() error {
		return w.setup(cleanup, 45)
	})
	sc.Step(`^a host group with AIs "Claude Code" and "Codex" on task "T-12" and ask wait (\d+) second$`, func(sec int) error {
		return w.setup(cleanup, sec)
	})
	sc.Step(`^"([^"]+)" calls ask to "([^"]+)" with "([^"]+)" and "([^"]+)" replies "([^"]+)"$`, func(caller, callee, q, replier, ans string) error {
		var replyErr error
		w.home.onCall = func(callID string) {
			rawArgs := json.RawMessage(accJSON(map[string]any{"call": callID, "text": ans}))
			_, replyErr = host.Tools["reply"].Call(context.Background(), w.h, w.caller(replier, 1), rawArgs)
			if _, handErr := w.h.Hand(callID, w.replyInboxItem(callID, replier, ans)); replyErr == nil {
				replyErr = handErr
			}
		}
		if err := w.callTool("ask", w.caller(caller, 0), map[string]any{"to": callee, "text": q}); err != nil {
			return err
		}
		return replyErr
	})
	sc.Step(`^the open ask call returns answer "([^"]+)"$`, func(want string) error {
		if w.lastErr != nil || w.lastResult["answer"] != want {
			return fmt.Errorf("got %v (%v), want answer %q", w.lastResult, w.lastErr, want)
		}
		return nil
	})
	sc.Step(`^the home received a call post at depth (\d+) and a reply post in task "T-12"$`, w.verifyCallAndReplyPosts)
	sc.Step(`^"([^"]+)" calls ask to "([^"]+)" with "([^"]+)" and "([^"]+)" does not reply in time$`, func(caller, callee, q, _ string) error {
		return w.callTool("ask", w.caller(caller, 0), map[string]any{"to": callee, "text": q})
	})
	registerAskReceiptAndLimitSteps(sc, w)
}

func registerAskReceiptAndLimitSteps(sc *godog.ScenarioContext, w *askStepWorld) {
	sc.Step(`^the ask call returns a pending receipt for the call$`, w.checkPendingReceipt)
	sc.Step(`^when "([^"]+)" replies "([^"]+)" later the reply item is not taken by an open call$`, w.checkLateReplyNotTaken)
	sc.Step(`^"([^"]+)" calls ask to "([^"]+)" while answering a call of depth (\d+) without waiting$`, func(caller, callee string, depth int) error {
		return w.callTool("ask", w.caller(caller, depth), map[string]any{"to": callee, "text": "Nested call", "wait": false})
	})
	sc.Step(`^the home received a call post at depth (\d+)$`, w.checkLastCallDepth)
	sc.Step(`^"([^"]+)" calls ask to "([^"]+)" while answering a call of depth (\d+)$`, func(caller, callee string, depth int) error {
		return w.callTool("ask", w.caller(caller, depth), map[string]any{"to": callee, "text": "Too deep"})
	})
	sc.Step(`^"([^"]+)" calls ask to "([^"]+)" containing a GitHub personal access token$`, func(caller, callee string) error {
		return w.callTool("ask", w.caller(caller, 0), map[string]any{
			"to":   callee,
			"text": "token ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		})
	})
	sc.Step(`^the ask call fails with "([^"]+)"$`, w.checkAskFailedWith)
	sc.Step(`^no command was sent to the home$`, w.checkNoPostsSent)
}

func (w *askStepWorld) checkPendingReceipt() error {
	receipt, ok := w.lastResult["receipt"].(string)
	if !ok || w.lastErr != nil || w.lastResult["pending"] != true || len(receipt) != 22 {
		return fmt.Errorf("got %v (%v), want pending receipt", w.lastResult, w.lastErr)
	}
	w.receipt = receipt
	return nil
}

func (w *askStepWorld) checkLateReplyNotTaken(replier, ans string) error {
	rawArgs := json.RawMessage(accJSON(map[string]any{"call": w.receipt, "text": ans}))
	if _, err := host.Tools["reply"].Call(context.Background(), w.h, w.caller(replier, 1), rawArgs); err != nil {
		return err
	}
	if waited, err := w.h.Hand(w.receipt, w.replyInboxItem(w.receipt, replier, ans)); waited || err != nil {
		return fmt.Errorf("expected waited=false err=nil, got %v, %v", waited, err)
	}
	return nil
}

func (w *askStepWorld) checkLastCallDepth(wantDepth int) error {
	if w.lastErr != nil || len(w.home.posts) == 0 {
		return fmt.Errorf("err=%v posts=%d", w.lastErr, len(w.home.posts))
	}
	var body proto.CallBody
	last := w.home.posts[len(w.home.posts)-1]
	if proto.Decode(last.Body, &body) != nil || body.Depth != wantDepth {
		return fmt.Errorf("got call body %+v, want depth %d", body, wantDepth)
	}
	return nil
}

func (w *askStepWorld) checkAskFailedWith(want string) error {
	if w.lastErr == nil || w.lastErr.Error() != want {
		return fmt.Errorf("got %v, want %q", w.lastErr, want)
	}
	return nil
}

func (w *askStepWorld) checkNoPostsSent() error {
	if len(w.home.posts) != 0 {
		return fmt.Errorf("home received %d posts, want 0", len(w.home.posts))
	}
	return nil
}

func (w *askStepWorld) verifyCallAndReplyPosts(wantDepth int) error {
	if len(w.home.posts) != 2 {
		return fmt.Errorf("got %d posts, want 2", len(w.home.posts))
	}
	callPost, replyPost := w.home.posts[0], w.home.posts[1]
	if err := w.checkSentCallPost(callPost, wantDepth); err != nil {
		return err
	}
	return w.checkSentReplyPost(replyPost, callPost.Nonce)
}

func (w *askStepWorld) checkSentCallPost(callPost proto.Post[json.RawMessage], wantDepth int) error {
	var callBody proto.CallBody
	if proto.Decode(callPost.Body, &callBody) != nil || callBody.Depth != wantDepth {
		return fmt.Errorf("call body = %+v, want depth %d", callBody, wantDepth)
	}
	if !sameThreadAndMentions(callPost, w.taskID, w.members["Codex"]) {
		return fmt.Errorf("unexpected call post: %+v", callPost)
	}
	return nil
}

func (w *askStepWorld) checkSentReplyPost(replyPost proto.Post[json.RawMessage], callID proto.ID) error {
	if replyPost.Kind != "reply" || replyPost.Re == nil || *replyPost.Re != callID {
		return fmt.Errorf("unexpected reply post: %+v", replyPost)
	}
	if !sameThreadAndMentions(replyPost, w.taskID, w.members["Claude Code"]) {
		return fmt.Errorf("unexpected reply post: %+v", replyPost)
	}
	return nil
}

func sameThreadAndMentions(post proto.Post[json.RawMessage], taskID, mentionID string) bool {
	return post.Thread != nil && string(*post.Thread) == taskID && slices.Equal(post.Mentions, []proto.ID{proto.ID(mentionID)})
}
