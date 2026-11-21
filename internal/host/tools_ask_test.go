package host

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func askTestID(label string) string {
	sum := sha256.Sum256([]byte("SoloDawn HS-12 test: " + label))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

var (
	askGroupID     = askTestID("group")
	askTaskID      = askTestID("task")
	askOtherTaskID = askTestID("other-task")
	askCallerID    = askTestID("caller-ai")
	askCalleeID    = askTestID("callee-ai")
	askSecID       = askTestID("secretary-ai")
	askHumanID     = askTestID("owner-human")
	askCallMsgID   = askTestID("call-msg")
	askReplyMsgID  = askTestID("reply-msg")
	askUserID      = askTestID("owner-user")
)

const askTestSecret = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"

type askFakeHome struct {
	mu      sync.Mutex
	done    chan struct{}
	onCmd   func(id int64, cmd proto.Cmd) (json.RawMessage, error)
	onFetch func(id int64, req proto.FetchRow) (json.RawMessage, error)
	sent    []any
}

func newAskFakeHome() *askFakeHome {
	return &askFakeHome{done: make(chan struct{})}
}

func (f *askFakeHome) Send(any) error               { return nil }
func (f *askFakeHome) Path() string                 { return "local" }
func (f *askFakeHome) PathChanged() <-chan struct{} { return nil }
func (f *askFakeHome) Done() <-chan struct{}        { return f.done }

func (f *askFakeHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.sent) + 1)
	frame := build(id)
	f.sent = append(f.sent, frame)
	onCmd, onFetch := f.onCmd, f.onFetch
	f.mu.Unlock()

	switch typed := frame.(type) {
	case proto.Cmd:
		if onCmd != nil {
			return onCmd(id, typed)
		}
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	case proto.FetchRow:
		if onFetch != nil {
			return onFetch(id, typed)
		}
		return json.RawMessage(`{"t":"fetched","re":1,"ok":true,"rows":[],"more":false}`), nil
	default:
		return nil, io.EOF
	}
}

func (f *askFakeHome) frames() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}

func askPutAI(h *Host, group, id, name string, secretary bool) {
	row := proto.AIMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 2},
			Group:     group,
			Kind:      "ai",
			User:      askUserID,
			Name:      name,
			LastSeen:  seamStart,
		},
		CLI:       "claude",
		Host:      h.Key.ID(),
		HostName:  "workstation",
		Secretary: secretary,
		Account:   "personal",
		Allow:     []string{},
		Approvers: []string{askUserID},
		State:     "working",
		Since:     seamStart,
		Path:      "local",
		Usage:     proto.Usage{},
	}
	raw, _ := json.Marshal(row)
	h.Rows.Put(group, "member", id, raw)
}

func askPutHuman(h *Host, group, id, name string) {
	row := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{
			RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 1},
			Group:     group,
			Kind:      "human",
			User:      askUserID,
			Name:      name,
			LastSeen:  seamStart,
		},
		Role:   "owner",
		Online: true,
	}
	raw, _ := json.Marshal(row)
	h.Rows.Put(group, "member", id, raw)
}

func askPutGroup(h *Host, group string, limits proto.Limits) {
	row := proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: group, Seq: 1},
		Name:           "Website",
		Kind:           "project",
		Repo:           "https://git.example/team/website.git",
		Base:           "main",
		Forge:          "github",
		Home:           "home-device",
		Checks:         []string{},
		Pass:           90,
		PassFoundation: 70,
		Limits:         limits,
		Created:        seamStart,
	}
	raw, _ := json.Marshal(row)
	h.Rows.Put(group, "group", group, raw)
}

func askBaseCaller(kind Kind, task string) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: askGroupID, Member: askCallerID, Task: task},
			Kind:       kind,
			Folder:     "repo",
			Agent:      "claude",
			Home:       "home-device",
			Name:       "Claude Code",
			State:      Working,
		},
		Level: 2,
	}
}

func askBuildReplyInboxItem(callID, answer string, thread *string) json.RawMessage {
	replyRow := proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: askReplyMsgID, Seq: 20},
		Pos:       20,
		Group:     askGroupID,
		Thread:    thread,
		Main:      false,
		By:        askCalleeID,
		Kind:      "reply",
		Timestamp: seamStart + 250,
		Text:      answer,
		Mentions:  []string{askCallerID},
		Re:        &callID,
		Body:      proto.EmptyBody{},
		State:     nil,
	}
	msgBytes, _ := json.Marshal(replyRow)
	itemPayload := marshalCallCmd(proto.Item{
		Type:      "item",
		Nonce:     proto.ID(askTestID("item-nonce")),
		Timestamp: seamStart + 250,
		Group:     proto.ID(askGroupID),
		To:        seamKey().ID(),
		Item:      9,
		AI:        proto.ID(askCallerID),
		Why:       "reply",
		Msg:       proto.ID(askReplyMsgID),
	})
	inboxItem := proto.InboxItem{
		Env: seamKey().Sign(itemPayload),
		Msg: msgBytes,
	}
	raw, _ := json.Marshal(inboxItem)
	return raw
}

func askBuildCallMsgRow(callID, callerID, calleeID, status string, thread *string) json.RawMessage {
	bodyBytes, _ := json.Marshal(proto.CallBody{To: calleeID, Depth: 1, Wait: true})
	var state *proto.CardState
	if status != "" {
		state = &proto.CardState{Status: status}
	}
	row := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: callID, Seq: 10},
		Pos:       10,
		Group:     askGroupID,
		Thread:    thread,
		Main:      false,
		By:        callerID,
		Kind:      "call",
		Timestamp: seamStart,
		Text:      "Which event reports a path change?",
		Mentions:  []string{calleeID},
		Re:        nil,
		Body:      bodyBytes,
		State:     state,
	}
	raw, _ := json.Marshal(row)
	return raw
}

func TestAskAndReplySchemasMatchToolsGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lists proto.ToolLists
	if err := json.Unmarshal(data, &lists); err != nil {
		t.Fatal(err)
	}
	for label, list := range map[string]proto.ToolList{
		"secretary":      lists.Secretary,
		"group":          lists.Group,
		"review":         lists.Review,
		"task_level_1":   lists.TaskLevel1,
		"task_level_2_3": lists.TaskLevel23,
	} {
		for _, toolName := range []string{"ask", "reply"} {
			reg, ok := Tools[toolName]
			if !ok || reg.For != nil || reg.Description == "" {
				t.Fatalf("%s missing or restricted: %+v", toolName, reg)
			}
			idx := slices.IndexFunc(list.Tools, func(entry proto.Tool) bool { return entry.Name == toolName })
			if idx < 0 {
				t.Fatalf("%s missing from %s", toolName, label)
			}
			var got, want any
			json.Unmarshal(reg.Schema, &got)
			goldenSchema, _ := json.Marshal(list.Tools[idx].InputSchema)
			json.Unmarshal(goldenSchema, &want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s schema in %s = %v, want %v", toolName, label, got, want)
			}
		}
	}
	for _, c := range []Caller{
		{Session: Session{Kind: GroupSession, Secretary: true}, Level: 1},
		{Session: Session{Kind: GroupSession}, Level: 1},
		{Session: Session{Kind: ReviewSession}, Level: 1},
		{Session: Session{Kind: TaskSession}, Level: 1},
		{Session: Session{Kind: TaskSession}, Level: 2},
	} {
		listed := ToolsFor(c)
		if _, ok := listed["ask"]; !ok {
			t.Fatalf("ask not listed for %+v", c)
		}
		if _, ok := listed["reply"]; !ok {
			t.Fatalf("reply not listed for %+v", c)
		}
	}
}

func TestAskAndReplyArgumentAndSecretValidation(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(TaskSession, askTaskID)

	for _, raw := range []string{
		`null`,
		`[]`,
		`{}`,
		`{"to":"Codex"}`,
		`{"text":"hi"}`,
		`{"to":123,"text":"hi"}`,
		`{"to":"Codex","text":"hi","wait":"yes"}`,
		`{"to":"Codex","text":"hi","wait":null}`,
		`{"to":"Codex","text":"hi","extra":true}`,
		`{"to":"Codex","text":"hi","WAIT":false}`,
	} {
		if _, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(raw)); err == nil {
			t.Fatalf("ask accepted %s", raw)
		} else {
			seamCode(t, err, "bad_request")
		}
	}

	for _, raw := range []string{
		`null`,
		`{}`,
		`{"call":"` + askCallMsgID + `"}`,
		`{"text":"hi"}`,
		`{"call":"","text":"hi"}`,
		`{"call":"` + askCallMsgID + `","text":"hi","extra":1}`,
	} {
		if _, err := Tools["reply"].Call(t.Context(), h, c, json.RawMessage(raw)); err == nil {
			t.Fatalf("reply accepted %s", raw)
		} else {
			seamCode(t, err, "bad_request")
		}
	}

	wantSecretErr := "secret: this looks like a secret; remove it and send again"
	askSecretArgs, _ := json.Marshal(map[string]any{"to": "Unknown", "text": "token " + askTestSecret})
	if _, err := Tools["ask"].Call(t.Context(), h, c, askSecretArgs); err == nil || err.Error() != wantSecretErr {
		t.Fatalf("ask secret error = %v, want %q", err, wantSecretErr)
	}
	replySecretArgs, _ := json.Marshal(map[string]any{"call": "", "text": "token " + askTestSecret})
	if _, err := Tools["reply"].Call(t.Context(), h, c, replySecretArgs); err == nil || err.Error() != wantSecretErr {
		t.Fatalf("reply secret error = %v, want %q", err, wantSecretErr)
	}
	if len(home.frames()) != 0 {
		t.Fatalf("secret or invalid arguments sent %d frames", len(home.frames()))
	}
}

func TestAskCalleeResolutionCases(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutHuman(h, askGroupID, askHumanID, "owner")
	goneRaw, _ := json.Marshal(proto.GoneRow{
		RowHeader: proto.RowHeader{Type: "member", ID: askTestID("gone-ai"), Seq: 9},
		Gone:      true,
	})
	h.Rows.Put(askGroupID, "member", askTestID("gone-ai"), goneRaw)
	c := askBaseCaller(GroupSession, "")

	// No other AI members exist yet: valid AI members is "none".
	_, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Ghost","text":"hi"}`))
	if err == nil || err.Error() != `not_found: unknown AI member "Ghost"; valid AI members: none` {
		t.Fatalf("got %v", err)
	}

	// Add two AI members with IDs in reverse order of their names to pin sorting.
	askPutAI(h, askGroupID, "zzzzzzzzzzzzzzzzzzzzzA", "Alpha", false)
	askPutAI(h, askGroupID, "aaaaaaaaaaaaaaaaaaaaaA", "Secretary", true)

	_, err = Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Ghost","text":"hi"}`))
	if err == nil || err.Error() != `not_found: unknown AI member "Ghost"; valid AI members: Alpha, Secretary` {
		t.Fatalf("got %v", err)
	}

	// Self-call when caller's member row is in Rows.
	_, err = Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Claude Code","text":"hi"}`))
	if err == nil || err.Error() != "bad_request: an AI cannot call itself" {
		t.Fatalf("self-call got %v", err)
	}

	// Human member is refused with bad_request.
	_, err = Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"owner","text":"hi"}`))
	if err == nil || err.Error() != `bad_request: "owner" is not an AI member` {
		t.Fatalf("human call got %v", err)
	}

	// If caller was renamed in Rows ("Claude 2") while Session.Name still says "Claude Code",
	// hasSelf is true so the old name "Claude Code" is unknown (not_found), not a self-call.
	askPutAI(h, askGroupID, askCallerID, "Claude 2", false)
	_, err = Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Claude Code","text":"hi"}`))
	seamCode(t, err, "not_found")

	// Unlisted caller (no self row in Rows): calling c.Name is refused as self-call,
	// empty c.Name with empty target is not_found, and another unknown name is not_found.
	h2, _ := seamHost(t, seamStore(t))
	askPutAI(h2, askGroupID, askCalleeID, "Codex", false)
	_, err = Tools["ask"].Call(t.Context(), h2, c, json.RawMessage(`{"to":"Claude Code","text":"hi"}`))
	if err == nil || err.Error() != "bad_request: an AI cannot call itself" {
		t.Fatalf("unlisted self-call got %v", err)
	}
	_, err = Tools["ask"].Call(t.Context(), h2, c, json.RawMessage(`{"to":"Other","text":"hi"}`))
	seamCode(t, err, "not_found")
	unnamedCaller := c
	unnamedCaller.Name = ""
	_, err = Tools["ask"].Call(t.Context(), h2, unnamedCaller, json.RawMessage(`{"to":"","text":"hi"}`))
	seamCode(t, err, "not_found")
}

func TestAskDepthLimitsAndThreadPlacement(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askSecID, "Secretary", true)

	// Depth 4 (c.Turn.Depth == 3) is accepted under default ask_depth 4; depth 5 is refused with limit.
	c := askBaseCaller(TaskSession, askTaskID)
	c.Turn = Turn{Depth: 3, Thread: askOtherTaskID}
	res, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Secretary","text":"a < b & c","wait":false}`))
	if err != nil {
		t.Fatal(err)
	}
	pending, ok := res.(askPending)
	if !ok || !pending.Pending || pending.Receipt == "" {
		t.Fatalf("got %+v", res)
	}
	sent := home.frames()
	if len(sent) != 1 {
		t.Fatalf("sent %d frames, want 1", len(sent))
	}
	cmd := sent[0].(proto.Cmd)
	if _, err := identity.OpenCommand(cmd.Env, "", func(dev string) bool { return dev == h.Key.ID() }, h.Now()); err != nil {
		t.Fatalf("command envelope invalid: %v", err)
	}
	if !strings.Contains(cmd.Env.Payload, `"a < b & c"`) {
		t.Fatalf("HTML was escaped in payload: %s", cmd.Env.Payload)
	}
	if nulls := proto.UnexpectedNulls([]byte(cmd.Env.Payload), "/re"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls %v in %s", nulls, cmd.Env.Payload)
	}
	var post proto.Post[proto.CallBody]
	if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
		t.Fatal(err)
	}
	if string(post.Nonce) != pending.Receipt || post.Timestamp != seamStart || string(post.Group) != askGroupID || string(post.As) != askCallerID {
		t.Fatalf("unexpected header: %+v", post.HomeCommand)
	}
	// c.Task takes precedence over c.Turn.Thread.
	if post.Kind != "call" || post.Thread == nil || string(*post.Thread) != askTaskID || post.Main || post.Re != nil {
		t.Fatalf("unexpected post fields: %+v", post)
	}
	if !slices.Equal(post.Mentions, []proto.ID{proto.ID(askSecID)}) {
		t.Fatalf("mentions = %v, want [%s]", post.Mentions, askSecID)
	}
	if post.Body != (proto.CallBody{To: askSecID, Depth: 4, Wait: false}) {
		t.Fatalf("body = %+v", post.Body)
	}

	// Exact boundary: c.Turn.Depth == 4 => depth 5 > 4 => limit error, nothing sent.
	c.Turn.Depth = 4
	_, err = Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Secretary","text":"too deep","wait":false}`))
	if err == nil || err.Error() != "limit: call depth 5 exceeds ask_depth limit 4" {
		t.Fatalf("depth 5 got %v", err)
	}
	if len(home.frames()) != 1 {
		t.Fatal("refused call was sent to home")
	}

	// GroupSession uses c.Turn.Thread when set, and nil when empty.
	grpCaller := askBaseCaller(GroupSession, "")
	grpCaller.Turn = Turn{Thread: askOtherTaskID}
	if _, err := Tools["ask"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"to":"Secretary","text":"in thread","wait":false}`)); err != nil {
		t.Fatal(err)
	}
	var grpThreadPost proto.Post[proto.CallBody]
	proto.Decode([]byte(home.frames()[1].(proto.Cmd).Env.Payload), &grpThreadPost)
	if grpThreadPost.Thread == nil || string(*grpThreadPost.Thread) != askOtherTaskID || grpThreadPost.Body.Depth != 1 {
		t.Fatalf("group thread post = %+v", grpThreadPost)
	}

	grpCaller.Turn = Turn{}
	if _, err := Tools["ask"].Call(t.Context(), h, grpCaller, json.RawMessage(`{"to":"Secretary","text":"in main","wait":false}`)); err != nil {
		t.Fatal(err)
	}
	mainPayload := home.frames()[2].(proto.Cmd).Env.Payload
	if nulls := proto.UnexpectedNulls([]byte(mainPayload), "/thread", "/re"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls %v in %s", nulls, mainPayload)
	}
	var mainPost proto.Post[proto.CallBody]
	proto.Decode([]byte(mainPayload), &mainPost)
	if mainPost.Thread != nil {
		t.Fatalf("expected nil thread in main line, got %v", *mainPost.Thread)
	}
}

func TestAskCustomGroupLimitsAndFallbacks(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(GroupSession, "")

	// Invalid group row falls back to defaults (4, 45s).
	h.Rows.Put(askGroupID, "group", askGroupID, json.RawMessage(`{"r":"group"}`))
	if maxDepth, waitDur := groupAskLimits(h, askGroupID); maxDepth != 4 || waitDur != 45*time.Second {
		t.Fatalf("got %d and %v", maxDepth, waitDur)
	}

	// Zero AskDepth and zero AskWaitS fall back to defaults (pins > 0 vs >= 0).
	askPutGroup(h, askGroupID, proto.Limits{AskDepth: 0, AskWaitS: 0})
	if maxDepth, waitDur := groupAskLimits(h, askGroupID); maxDepth != 4 || waitDur != 45*time.Second {
		t.Fatalf("zero limits got %d and %v", maxDepth, waitDur)
	}
	if _, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Codex","text":"depth 1","wait":false}`)); err != nil {
		t.Fatalf("zero AskDepth rejected depth 1: %v", err)
	}

	// AskDepth = 1 and AskWaitS = 1 override defaults (pins > 0 vs > 1).
	askPutGroup(h, askGroupID, proto.Limits{AskDepth: 1, AskWaitS: 1})
	if maxDepth, waitDur := groupAskLimits(h, askGroupID); maxDepth != 1 || waitDur != 1*time.Second {
		t.Fatalf("limits=1 got %d and %v", maxDepth, waitDur)
	}
	if _, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Codex","text":"ok at 1","wait":false}`)); err != nil {
		t.Fatalf("AskDepth=1 rejected depth 1: %v", err)
	}
	c.Turn.Depth = 1
	_, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Codex","text":"fail at 2","wait":false}`))
	if err == nil || err.Error() != "limit: call depth 2 exceeds ask_depth limit 1" {
		t.Fatalf("AskDepth=1 depth 2 got %v", err)
	}
}

func TestAskAwaitBeforeCommandAndWaitFalseLeavesNoOpenWait(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	otherHost, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(TaskSession, askTaskID)

	// Synchronous Hand inside Home.Request proves Await was registered BEFORE h.Command.
	home.onCmd = func(id int64, cmd proto.Cmd) (json.RawMessage, error) {
		var post proto.Post[proto.CallBody]
		if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
			t.Errorf("decode post: %v", err)
		}
		callID := string(post.Nonce)
		thread := askTaskID
		replyItem := askBuildReplyInboxItem(callID, "Use PathChanged.", &thread)
		// Another host in the same process never shares this host's open-call table.
		if waited, _ := otherHost.Hand(callID, replyItem); waited {
			t.Error("otherHost claimed host's open call")
		}
		if post.Body.Wait {
			waited, err := h.Hand(callID, replyItem)
			if !waited || err != nil {
				t.Errorf("expected open wait during h.Command, got waited=%v err=%v", waited, err)
			}
		} else {
			if waited, _ := h.Hand(callID, replyItem); waited {
				t.Error("wait:false registered an open wait")
			}
		}
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}

	// Default wait (omitted => true) and explicit wait: true both receive the answer.
	for _, args := range []string{
		`{"to":"Codex","text":"Which signal?"}`,
		`{"to":"Codex","text":"Which signal?","wait":true}`,
	} {
		got, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(args))
		if err != nil || got != (askAnswered{Answer: "Use PathChanged."}) {
			t.Fatalf("args %s got %+v, %v", args, got, err)
		}
	}

	// wait: false returns pending receipt immediately and never claims h.Hand.
	got, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Codex","text":"Large question","wait":false}`))
	if err != nil {
		t.Fatal(err)
	}
	pending := got.(askPending)
	if !pending.Pending || len(pending.Receipt) != 22 {
		t.Fatalf("unexpected pending result: %+v", pending)
	}
}

func TestAskTimeoutBoundaryAndLateReplyInSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := seamHost(t, seamStore(t))
		home := newAskFakeHome()
		h.Attach("home-device", home)
		askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
		askPutAI(h, askGroupID, askCalleeID, "Codex", false)
		c := askBaseCaller(TaskSession, askTaskID)

		callIDCh := make(chan string, 1)
		home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
			var post proto.Post[proto.CallBody]
			proto.Decode([]byte(cmd.Env.Payload), &post)
			callIDCh <- string(post.Nonce)
			return json.RawMessage(`{"re":1,"ok":true}`), nil
		}

		type outcome struct {
			val any
			err error
		}
		doneCh := make(chan outcome, 1)
		go func() {
			val, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Codex","text":"Busy?"}`))
			doneCh <- outcome{val, err}
		}()

		callID := <-callIDCh
		synctest.Wait()

		// At 45s - 1ns the ask call must still be open.
		time.Sleep(45*time.Second - time.Nanosecond)
		synctest.Wait()
		select {
		case res := <-doneCh:
			t.Fatalf("ask returned before 45s: %+v", res)
		default:
		}

		// Advancing the final 1ns reaches 45s and returns the pending receipt.
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		res := <-doneCh
		if res.err != nil || res.val != (askPending{Pending: true, Receipt: callID}) {
			t.Fatalf("at 45s got %+v, %v", res.val, res.err)
		}

		// A late reply arriving after the wait elapsed finds no open call (waited == false).
		thread := askTaskID
		if waited, err := h.Hand(callID, askBuildReplyInboxItem(callID, "Late answer", &thread)); waited || err != nil {
			t.Fatalf("late reply after timeout got waited=%v err=%v", waited, err)
		}
	})
}

func TestAskFailureContextCancelAndDrainEdgeCases(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(GroupSession, "")

	// Unreachable home fails both wait:false and wait:true, cleaning up the open wait.
	for _, raw := range []string{
		`{"to":"Codex","text":"hi","wait":false}`,
		`{"to":"Codex","text":"hi","wait":true}`,
	} {
		if _, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(raw)); err == nil {
			t.Fatalf("expected home_unreachable for %s", raw)
		} else {
			seamCode(t, err, "home_unreachable")
		}
	}

	home := newAskFakeHome()
	h.Attach("home-device", home)

	// Home refusal during wait:true cleans up the registered wait in h.waits.
	var failedCallID string
	home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
		var post proto.Post[proto.CallBody]
		proto.Decode([]byte(cmd.Env.Payload), &post)
		failedCallID = string(post.Nonce)
		return json.RawMessage(`{"re":1,"err":{"code":"limit","msg":"ask_depth"}}`), nil
	}
	if _, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(`{"to":"Codex","text":"hi"}`)); err == nil {
		t.Fatal("expected limit refusal")
	} else {
		seamCode(t, err, "limit")
	}
	if waited, _ := h.Hand(failedCallID, askBuildReplyInboxItem(failedCallID, "ans", nil)); waited {
		t.Fatal("failed h.Command leaked open wait")
	}

	// Context cancellation during waitAskReply releases the wait and returns ctx.Err().
	called := make(chan string, 1)
	home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
		var post proto.Post[proto.CallBody]
		proto.Decode([]byte(cmd.Env.Payload), &post)
		called <- string(post.Nonce)
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		_, err := Tools["ask"].Call(ctx, h, c, json.RawMessage(`{"to":"Codex","text":"cancel me"}`))
		errCh <- err
	}()
	canceledID := <-called
	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("canceled ask returned %v, want context.Canceled", err)
	}
	if waited, _ := h.Hand(canceledID, askBuildReplyInboxItem(canceledID, "ans", nil)); waited {
		t.Fatal("canceled ask leaked open wait")
	}

	// finishAskTimeout reads itemCh once more after done() so an item buffered just before timeout is returned.
	ch, done := h.Await(askCallMsgID, nil)
	if waited, err := h.Hand(askCallMsgID, askBuildReplyInboxItem(askCallMsgID, "Just in time", nil)); !waited || err != nil {
		t.Fatalf("Hand got %v, %v", waited, err)
	}
	ans, err := finishAskTimeout(ch, done, askCallMsgID)
	if err != nil || ans != (askAnswered{Answer: "Just in time"}) {
		t.Fatalf("finishAskTimeout got %+v, %v", ans, err)
	}

	// decodeReplyItem rejects malformed item, missing msg, and malformed msg.
	emptyEnv, _ := json.Marshal(proto.InboxItem{Env: seamKey().Sign("{}")})
	badMsgEnv, _ := json.Marshal(proto.InboxItem{Env: seamKey().Sign("{}"), Msg: json.RawMessage(`{"r":"msg"}`)})
	for _, bad := range []json.RawMessage{
		json.RawMessage(`{}`),
		emptyEnv,
		badMsgEnv,
	} {
		if _, err := decodeReplyItem(bad); err == nil {
			t.Fatalf("decodeReplyItem accepted %s", bad)
		} else {
			seamCode(t, err, "bad_request")
		}
	}

	if id1, id2 := newCallID(), newCallID(); id1 == id2 || len(id1) != 22 {
		t.Fatalf("newCallID produced %q and %q", id1, id2)
	}
}

func TestReplyFetchesOrReadsCallAndSendsPost(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)

	callee := Caller{
		Session: Session{
			SessionKey: SessionKey{Group: askGroupID, Member: askCalleeID, Task: askTaskID},
			Kind:       TaskSession,
			Folder:     "wt/12-path",
			Agent:      "codex",
			Home:       "home-device",
			Name:       "Codex",
			State:      Working,
		},
		Level: 2,
	}

	// 1. Call message fetched from the home (in a task thread).
	thread := askTaskID
	callRow := askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "waiting", &thread)
	var seenFetch proto.FetchRow
	home.onFetch = func(id int64, req proto.FetchRow) (json.RawMessage, error) {
		seenFetch = req
		resp := proto.Fetched{
			Type:    "fetched",
			Success: proto.Success{Re: id, OK: true},
			Rows:    []json.RawMessage{callRow},
			More:    false,
		}
		raw, _ := json.Marshal(resp)
		return raw, nil
	}

	got, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"Use PathChanged()."}`))
	if err != nil || got != (replyAck{OK: true}) {
		t.Fatalf("reply got %+v, %v", got, err)
	}
	if seenFetch.Type != "fetch" || seenFetch.Group != askGroupID || seenFetch.Before != 0 || seenFetch.Limit != 1 || seenFetch.Row != "msg" || seenFetch.RowID != askCallMsgID {
		t.Fatalf("unexpected fetch request: %+v", seenFetch)
	}
	frames := home.frames()
	if len(frames) != 2 {
		t.Fatalf("sent %d frames, want 2 (fetch + cmd)", len(frames))
	}
	cmd := frames[1].(proto.Cmd)
	if nulls := proto.UnexpectedNulls([]byte(cmd.Env.Payload)); len(nulls) != 0 {
		t.Fatalf("unexpected nulls %v in %s", nulls, cmd.Env.Payload)
	}
	var post proto.Post[proto.EmptyBody]
	if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
		t.Fatal(err)
	}
	if post.Kind != "reply" || string(post.Group) != askGroupID || string(post.As) != askCalleeID || post.Main {
		t.Fatalf("unexpected reply post: %+v", post)
	}
	if post.Thread == nil || string(*post.Thread) != askTaskID || post.Re == nil || string(*post.Re) != askCallMsgID {
		t.Fatalf("unexpected thread/re: %+v", post)
	}
	if !slices.Equal(post.Mentions, []proto.ID{proto.ID(askCallerID)}) || post.Text != "Use PathChanged()." {
		t.Fatalf("unexpected mentions/text: %+v", post)
	}

	// 2. Call message already cached in h.Rows (main line, thread == nil) sends no fetch.
	mainCallID := askTestID("main-call")
	h.Rows.Put(askGroupID, "msg", mainCallID, askBuildCallMsgRow(mainCallID, askCallerID, askCalleeID, "waiting", nil))
	got, err = Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+mainCallID+`","text":"Main line answer."}`))
	if err != nil || got != (replyAck{OK: true}) {
		t.Fatalf("cached main reply got %+v, %v", got, err)
	}
	if len(home.frames()) != 3 {
		t.Fatalf("expected only 1 cmd frame without fetch, got %d total frames", len(home.frames()))
	}
	mainPayload := home.frames()[2].(proto.Cmd).Env.Payload
	if nulls := proto.UnexpectedNulls([]byte(mainPayload), "/thread"); len(nulls) != 0 {
		t.Fatalf("unexpected nulls %v in %s", nulls, mainPayload)
	}
	var mainReply proto.Post[proto.EmptyBody]
	proto.Decode([]byte(mainPayload), &mainReply)
	if mainReply.Thread != nil || mainReply.Re == nil || string(*mainReply.Re) != mainCallID {
		t.Fatalf("unexpected main reply: %+v", mainReply)
	}
}

func TestReplyRefusalAndErrorCases(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	callee := Caller{
		Session: Session{
			SessionKey: SessionKey{Group: askGroupID, Member: askCalleeID},
			Kind:       GroupSession,
			Home:       "home-device",
			Name:       "Codex",
		},
		Level: 1,
	}

	// Home disconnected before fetch.
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected home_unreachable")
	} else {
		seamCode(t, err, "home_unreachable")
	}

	home := newAskFakeHome()
	h.Attach("home-device", home)

	// Fetch transport error => home_unreachable.
	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) { return nil, io.EOF }
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected home_unreachable on transport failure")
	} else {
		seamCode(t, err, "home_unreachable")
	}

	// Fetch returns wire ErrorResponse => propagated.
	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"forbidden","msg":"not a member"}}`), nil
	}
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected forbidden")
	} else {
		seamCode(t, err, "forbidden")
	}

	// Fetch returns malformed frame => bad_request.
	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) {
		return json.RawMessage(`{"t":"fetched"}`), nil
	}
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected bad_request")
	} else {
		seamCode(t, err, "bad_request")
	}

	// Fetch returns empty rows => not_found.
	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) {
		return json.RawMessage(`{"t":"fetched","re":1,"ok":true,"rows":[],"more":false}`), nil
	}
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected not_found")
	} else {
		seamCode(t, err, "not_found")
	}

	// Fetched row fails MsgRow decode OR has mismatched ID => bad_request (pins both sides of ||).
	for _, badRow := range []json.RawMessage{
		json.RawMessage(`{"r":"msg"}`),
		askBuildCallMsgRow(askTestID("wrong-id"), askCallerID, askCalleeID, "waiting", nil),
	} {
		home.onFetch = func(id int64, _ proto.FetchRow) (json.RawMessage, error) {
			raw, _ := json.Marshal(proto.Fetched{Type: "fetched", Success: proto.Success{Re: id, OK: true}, Rows: []json.RawMessage{badRow}})
			return raw, nil
		}
		if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
			t.Fatalf("expected bad_request for row %s", badRow)
		} else {
			seamCode(t, err, "bad_request")
		}
	}

	// Non-call message kind => conflict.
	textRow, _ := json.Marshal(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: askCallMsgID, Seq: 3},
		Pos:       3, Group: askGroupID, By: askCallerID, Kind: "text", Timestamp: seamStart,
		Text: "plain text", Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
	h.Rows.Put(askGroupID, "msg", askCallMsgID, textRow)
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected conflict for text msg")
	} else {
		seamCode(t, err, "conflict")
	}

	// Call message with malformed body => bad_request.
	badCallBody, _ := json.Marshal(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: askCallMsgID, Seq: 4},
		Pos:       4, Group: askGroupID, By: askCallerID, Kind: "call", Timestamp: seamStart,
		Text: "q", Mentions: []string{}, Body: json.RawMessage(`{}`), State: &proto.CardState{Status: "waiting"},
	})
	h.Rows.Put(askGroupID, "msg", askCallMsgID, badCallBody)
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected bad_request for malformed call body")
	} else {
		seamCode(t, err, "bad_request")
	}

	// Call addressed to another AI while waiting => conflict (pins left side of ||).
	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askSecID, "waiting", nil))
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected conflict for wrong callee")
	} else {
		seamCode(t, err, "conflict")
	}

	// Call addressed to this AI with nil state => conflict.
	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "", nil))
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected conflict for nil state")
	} else {
		seamCode(t, err, "conflict")
	}

	// Call addressed to this AI already answered => conflict (pins state.Status == "waiting").
	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "answered", nil))
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected conflict for answered call")
	} else {
		seamCode(t, err, "conflict")
	}

	// Valid call in Rows, then h.Command fails on home refusal => propagated.
	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "waiting", nil))
	home.onCmd = func(int64, proto.Cmd) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"already answered"}}`), nil
	}
	if _, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"hi"}`)); err == nil {
		t.Fatal("expected conflict from h.Command")
	} else {
		seamCode(t, err, "conflict")
	}
}
