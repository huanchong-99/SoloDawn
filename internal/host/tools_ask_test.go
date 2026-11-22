package host

import (
	"context"
	"encoding/json"
	"io"
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

const (
	askGroupID     = "rZNvy-1jH6Z-BcPqA5U5BQ"
	askTaskID      = "DrtCn6htSBwmMPrFPbHJHA"
	askOtherTaskID = "hVhh0SQDDWIzdLF3fDLiBA"
	askCallerID    = "1PC8WineBrUQ-apCjx7tug"
	askCalleeID    = "LXCZmuGAXkvO-bSrOkuCfw"
	askSecID       = "qBSFMsr2hHYKOMblEA_kdA"
	askHumanID     = "4xq2Q8RPeg7IJLWdEZTWDQ"
	askCallMsgID   = "Zk3m8vN2pQ5rT7wX9yB1cA"
	askMainCallID  = "KaiCW9JC8UOG7lKNduDo8Q"
	askReplyMsgID  = "Aa0000000000000000000A"
	askGoneID      = "9kbEU81-A3uJPqD1MdM1WA"
	askUserID      = "akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs"
	askTestSecret  = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
)

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
	}
	h.Rows.Put(group, "member", id, json.RawMessage(marshalWire(row)))
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
	h.Rows.Put(group, "member", id, json.RawMessage(marshalWire(row)))
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
	h.Rows.Put(group, "group", group, json.RawMessage(marshalWire(row)))
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
		By:        askCalleeID,
		Kind:      "reply",
		Timestamp: seamStart + 250,
		Text:      answer,
		Mentions:  []string{askCallerID},
		Re:        &callID,
		Body:      proto.EmptyBody{},
	}
	inboxItem := proto.InboxItem{
		Item: proto.Item{
			Item: 9,
			AI:   proto.ID(askCallerID),
			Why:  "reply",
			Msg:  proto.ID(askReplyMsgID),
		},
		Msg: json.RawMessage(marshalWire(replyRow)),
	}
	return json.RawMessage(marshalWire(inboxItem))
}

func askBuildCallMsgRow(callID, callerID, calleeID, status string, thread *string) json.RawMessage {
	var state *proto.CardState
	if status != "" {
		state = &proto.CardState{Status: status}
	}
	row := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: callID, Seq: 10},
		Pos:       10,
		Group:     askGroupID,
		Thread:    thread,
		By:        callerID,
		Kind:      "call",
		Timestamp: seamStart,
		Text:      "Which event reports a path change?",
		Mentions:  []string{calleeID},
		Body:      json.RawMessage(marshalWire(proto.CallBody{To: calleeID, Depth: 1, Wait: true})),
		State:     state,
	}
	return json.RawMessage(marshalWire(row))
}

func assertToolBadRequests(t *testing.T, h *Host, c Caller, toolName string, inputs []string) {
	t.Helper()
	for _, raw := range inputs {
		_, err := Tools[toolName].Call(t.Context(), h, c, json.RawMessage(raw))
		seamCode(t, err, "bad_request")
	}
}

func assertToolExactError(t *testing.T, h *Host, c Caller, toolName, raw, want string) {
	t.Helper()
	_, err := Tools[toolName].Call(t.Context(), h, c, json.RawMessage(raw))
	if err == nil || err.Error() != want {
		t.Fatalf("%s(%s) = %v, want %q", toolName, raw, err, want)
	}
}

func assertToolErrorCode(t *testing.T, h *Host, c Caller, toolName, raw, code string) {
	t.Helper()
	_, err := Tools[toolName].Call(t.Context(), h, c, json.RawMessage(raw))
	seamCode(t, err, code)
}

func checkAskSchemaInGoldenList(t *testing.T, label string, list proto.ToolList, toolName string) {
	t.Helper()
	reg, ok := Tools[toolName]
	if !ok || reg.For != nil || reg.Description == "" {
		t.Fatalf("%s missing or restricted: %+v", toolName, reg)
	}
	idx := slices.IndexFunc(list.Tools, func(entry proto.Tool) bool { return entry.Name == toolName })
	if idx < 0 {
		t.Fatalf("%s missing from %s", toolName, label)
	}
	got := decodeStrictToolInput(t, toolName, reg.Schema)
	if !reflect.DeepEqual(got, list.Tools[idx].InputSchema) {
		t.Fatalf("%s schema in %s = %+v, want %+v", toolName, label, got, list.Tools[idx].InputSchema)
	}
}

func checkAskListedForCaller(t *testing.T, c Caller) {
	t.Helper()
	listed := ToolsFor(c)
	if _, ok := listed["ask"]; !ok {
		t.Fatalf("ask not listed for %+v", c)
	}
	if _, ok := listed["reply"]; !ok {
		t.Fatalf("reply not listed for %+v", c)
	}
}

func TestAskAndReplySchemasMatchToolsGolden(t *testing.T) {
	lists := loadGoldenToolLists(t)
	named := map[string]proto.ToolList{
		"secretary":      lists.Secretary,
		"group":          lists.Group,
		"review":         lists.Review,
		"task_level_1":   lists.TaskLevel1,
		"task_level_2_3": lists.TaskLevel23,
	}
	for label, list := range named {
		checkAskSchemaInGoldenList(t, label, list, "ask")
		checkAskSchemaInGoldenList(t, label, list, "reply")
	}
	for _, c := range []Caller{
		{Session: Session{Kind: GroupSession, Secretary: true}, Level: 1},
		{Session: Session{Kind: GroupSession}, Level: 1},
		{Session: Session{Kind: ReviewSession}, Level: 1},
		{Session: Session{Kind: TaskSession}, Level: 1},
		{Session: Session{Kind: TaskSession}, Level: 2},
	} {
		checkAskListedForCaller(t, c)
	}
}

func TestAskAndReplyArgumentAndSecretValidation(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(TaskSession, askTaskID)

	assertToolBadRequests(t, h, c, "ask", []string{
		`null`,
		`[]`,
		`{}`,
		`{"to":"Codex"}`,
		`{"text":"hi"}`,
		`{"to":123,"text":"hi"}`,
		`{"to":"Codex","text":"hi","wait":"yes"}`,
		`{"to":"Codex","text":"hi","wait":null}`,
	})
	assertToolBadRequests(t, h, c, "reply", []string{
		`null`,
		`{}`,
		`{"call":"` + askCallMsgID + `"}`,
		`{"text":"hi"}`,
		`{"call":"","text":"hi"}`,
	})

	wantSecretErr := "secret: this looks like a secret; remove it and send again"
	assertToolExactError(t, h, c, "ask", marshalWire(map[string]any{"to": "Unknown", "text": "token " + askTestSecret}), wantSecretErr)
	assertToolExactError(t, h, c, "reply", marshalWire(map[string]any{"call": "", "text": "token " + askTestSecret}), wantSecretErr)
	if len(home.frames()) != 0 {
		t.Fatalf("secret or invalid arguments sent %d frames", len(home.frames()))
	}
}

func TestAskCalleeResolutionCases(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutHuman(h, askGroupID, askHumanID, "owner")
	gone := proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: askGoneID, Seq: 9}, Gone: true}
	h.Rows.Put(askGroupID, "member", askGoneID, json.RawMessage(marshalWire(gone)))
	c := askBaseCaller(GroupSession, "")

	assertToolExactError(t, h, c, "ask", `{"to":"Ghost","text":"hi"}`, `not_found: unknown AI member "Ghost" (valid: none)`)

	// Insert IDs in reverse order of names ("Zeta" at "aaaa...", "Alpha" at "zzzz...") to pin sorting.
	askPutAI(h, askGroupID, "aaaaaaaaaaaaaaaaaaaaaA", "Zeta", true)
	askPutAI(h, askGroupID, "zzzzzzzzzzzzzzzzzzzzzA", "Alpha", false)
	assertToolExactError(t, h, c, "ask", `{"to":"Ghost","text":"hi"}`, `not_found: unknown AI member "Ghost" (valid: Alpha, Zeta)`)
	assertToolExactError(t, h, c, "ask", `{"to":"Claude Code","text":"hi"}`, "bad_request: an AI cannot call itself")
	assertToolExactError(t, h, c, "ask", `{"to":"owner","text":"hi"}`, `bad_request: "owner" is not an AI member`)
}

func decodeVerifiedCmdPost[B any](t *testing.T, h *Host, rawFrame any, allowedNulls ...string) proto.Post[B] {
	t.Helper()
	cmd := rawFrame.(proto.Cmd)
	if _, err := identity.OpenCommand(cmd.Env, "", func(dev string) bool { return dev == h.Key.ID() }, h.Now()); err != nil {
		t.Fatalf("command envelope invalid: %v", err)
	}
	if nulls := proto.UnexpectedNulls([]byte(cmd.Env.Payload), allowedNulls...); len(nulls) != 0 {
		t.Fatalf("unexpected nulls %v in %s", nulls, cmd.Env.Payload)
	}
	var post proto.Post[B]
	if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
		t.Fatal(err)
	}
	return post
}

func mustCallAsk(t *testing.T, h *Host, c Caller, raw string) any {
	t.Helper()
	got, err := Tools["ask"].Call(t.Context(), h, c, json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertCallPost(t *testing.T, post proto.Post[proto.CallBody], want proto.Post[proto.CallBody]) {
	t.Helper()
	secMustEq(t, post, want)
}

func TestAskDepthLimitsAndThreadPlacement(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askSecID, "Secretary", true)

	c := askBaseCaller(TaskSession, askTaskID)
	c.Turn = Turn{Depth: 3, Thread: askOtherTaskID}
	pending := mustCallAsk(t, h, c, `{"to":"Secretary","text":"a < b & c","wait":false}`).(askPending)
	secMustTrue(t, pending.Pending && len(pending.Receipt) == 22, "expected pending receipt")
	firstFrame := home.frames()[0]
	secMustTrue(t, strings.Contains(firstFrame.(proto.Cmd).Env.Payload, `"a < b & c"`), "HTML was escaped")
	post := decodeVerifiedCmdPost[proto.CallBody](t, h, firstFrame, "/re")
	wantTask := proto.ID(askTaskID)
	assertCallPost(t, post, proto.Post[proto.CallBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(pending.Receipt), Timestamp: seamStart, Group: askGroupID, As: askCallerID},
		Kind:        "call", Thread: &wantTask, Text: "a < b & c", Mentions: []proto.ID{askSecID},
		Body: proto.CallBody{To: askSecID, Depth: 4, Wait: false},
	})

	c.Turn.Depth = 4
	assertToolExactError(t, h, c, "ask", `{"to":"Secretary","text":"too deep","wait":false}`, "limit: call depth 5 exceeds ask_depth limit 4")
	secMustEq(t, len(home.frames()), 1)

	verifyGroupSessionAskThreads(t, h, home)
}

func verifyGroupSessionAskThreads(t *testing.T, h *Host, home *askFakeHome) {
	t.Helper()
	grpCaller := askBaseCaller(GroupSession, "")
	grpCaller.Turn = Turn{Thread: askOtherTaskID}
	grpPending := mustCallAsk(t, h, grpCaller, `{"to":"Secretary","text":"in thread","wait":false}`).(askPending)
	grpThreadPost := decodeVerifiedCmdPost[proto.CallBody](t, h, home.frames()[1], "/re")
	otherTask := proto.ID(askOtherTaskID)
	assertCallPost(t, grpThreadPost, proto.Post[proto.CallBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(grpPending.Receipt), Timestamp: seamStart + 2000, Group: askGroupID, As: askCallerID},
		Kind:        "call", Thread: &otherTask, Text: "in thread", Mentions: []proto.ID{askSecID},
		Body: proto.CallBody{To: askSecID, Depth: 1, Wait: false},
	})

	grpCaller.Turn = Turn{}
	mainPending := mustCallAsk(t, h, grpCaller, `{"to":"Secretary","text":"in main","wait":false}`).(askPending)
	mainPost := decodeVerifiedCmdPost[proto.CallBody](t, h, home.frames()[2], "/thread", "/re")
	assertCallPost(t, mainPost, proto.Post[proto.CallBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: proto.ID(mainPending.Receipt), Timestamp: seamStart + 4000, Group: askGroupID, As: askCallerID},
		Kind:        "call", Text: "in main", Mentions: []proto.ID{askSecID},
		Body: proto.CallBody{To: askSecID, Depth: 1, Wait: false},
	})
}

func assertAskGroupLimits(t *testing.T, h *Host, wantDepth int, wantWait time.Duration) {
	t.Helper()
	if maxDepth, waitDur := groupAskLimits(h, askGroupID); maxDepth != wantDepth || waitDur != wantWait {
		t.Fatalf("got (%d, %v), want (%d, %v)", maxDepth, waitDur, wantDepth, wantWait)
	}
}

func TestAskCustomGroupLimitsAndFallbacks(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(GroupSession, "")

	h.Rows.Put(askGroupID, "group", askGroupID, json.RawMessage(`{"r":"group"}`))
	assertAskGroupLimits(t, h, 4, 45*time.Second)

	askPutGroup(h, askGroupID, proto.Limits{AskDepth: 0, AskWaitS: 0})
	assertAskGroupLimits(t, h, 4, 45*time.Second)
	mustCallAsk(t, h, c, `{"to":"Codex","text":"depth 1","wait":false}`)

	askPutGroup(h, askGroupID, proto.Limits{AskDepth: 1, AskWaitS: 1})
	assertAskGroupLimits(t, h, 1, 1*time.Second)
	mustCallAsk(t, h, c, `{"to":"Codex","text":"ok at 1","wait":false}`)
	c.Turn.Depth = 1
	assertToolExactError(t, h, c, "ask", `{"to":"Codex","text":"fail at 2","wait":false}`, "limit: call depth 2 exceeds ask_depth limit 1")
}

func installSyncReplyHook(t *testing.T, h, otherHost *Host, home *askFakeHome) {
	t.Helper()
	home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
		var post proto.Post[proto.CallBody]
		if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
			t.Errorf("decode post: %v", err)
		}
		verifySyncHandDuringCmd(t, h, otherHost, string(post.Nonce), post.Body.Wait)
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
}

func verifySyncHandDuringCmd(t *testing.T, h, otherHost *Host, callID string, wait bool) {
	t.Helper()
	thread := askTaskID
	replyItem := askBuildReplyInboxItem(callID, "Use PathChanged.", &thread)
	if waited, err := otherHost.Hand(callID, replyItem); waited || err != nil {
		t.Errorf("otherHost got waited=%v err=%v", waited, err)
	}
	waited, err := h.Hand(callID, replyItem)
	if waited != wait || err != nil {
		t.Errorf("Hand(wait=%v) got waited=%v err=%v", wait, waited, err)
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
	installSyncReplyHook(t, h, otherHost, home)

	for _, args := range []string{
		`{"to":"Codex","text":"Which signal?"}`,
		`{"to":"Codex","text":"Which signal?","wait":true}`,
	} {
		secMustEq(t, mustCallAsk(t, h, c, args), askAnswered{Answer: "Use PathChanged."})
	}

	pending := mustCallAsk(t, h, c, `{"to":"Codex","text":"Large question","wait":false}`).(askPending)
	secMustTrue(t, pending.Pending && len(pending.Receipt) == 22, "expected 22-char pending receipt")
}

type askAsyncOutcome struct {
	val any
	err error
}

func startAsyncAsk(t *testing.T, ctx context.Context, h *Host, c Caller, raw string) <-chan askAsyncOutcome {
	t.Helper()
	// Single reader: caller of startAsyncAsk.
	doneCh := make(chan askAsyncOutcome, 1)
	go func() {
		val, err := Tools["ask"].Call(ctx, h, c, json.RawMessage(raw))
		doneCh <- askAsyncOutcome{val, err}
	}()
	return doneCh
}

func TestAskTimeoutBoundaryAndLateReplyInSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _ := seamHost(t, seamStore(t))
		home := newAskFakeHome()
		h.Attach("home-device", home)
		askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
		askPutAI(h, askGroupID, askCalleeID, "Codex", false)
		c := askBaseCaller(TaskSession, askTaskID)

		// Single reader: callID receive below.
		callIDCh := make(chan string, 1)
		home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
			var post proto.Post[proto.CallBody]
			if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
				t.Errorf("decode post: %v", err)
			}
			callIDCh <- string(post.Nonce)
			return json.RawMessage(`{"re":1,"ok":true}`), nil
		}

		doneCh := startAsyncAsk(t, t.Context(), h, c, `{"to":"Codex","text":"Busy?"}`)
		callID := <-callIDCh
		synctest.Wait()

		verifyTimeoutBoundaryInBubble(t, h, doneCh, callID)
	})
}

func verifyTimeoutBoundaryInBubble(t *testing.T, h *Host, doneCh <-chan askAsyncOutcome, callID string) {
	t.Helper()
	time.Sleep(45*time.Second - time.Nanosecond)
	synctest.Wait()
	select {
	case res := <-doneCh:
		t.Fatalf("ask returned before 45s: %+v", res)
	default:
	}

	time.Sleep(time.Nanosecond)
	synctest.Wait()
	res := <-doneCh
	if res.err != nil || res.val != (askPending{Pending: true, Receipt: callID}) {
		t.Fatalf("at 45s got %+v, %v", res.val, res.err)
	}

	thread := askTaskID
	if waited, err := h.Hand(callID, askBuildReplyInboxItem(callID, "Late answer", &thread)); waited || err != nil {
		t.Fatalf("late reply after timeout got waited=%v err=%v", waited, err)
	}
}

func assertNoOpenWait(t *testing.T, h *Host, callID string) {
	t.Helper()
	if waited, err := h.Hand(callID, askBuildReplyInboxItem(callID, "ans", nil)); waited || err != nil {
		t.Fatalf("open wait remained for %s: waited=%v err=%v", callID, waited, err)
	}
}

func TestAskCommandFailuresAndCancelCleanUpWait(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	askPutAI(h, askGroupID, askCallerID, "Claude Code", false)
	askPutAI(h, askGroupID, askCalleeID, "Codex", false)
	c := askBaseCaller(GroupSession, "")

	assertToolErrorCode(t, h, c, "ask", `{"to":"Codex","text":"hi","wait":false}`, "home_unreachable")
	assertToolErrorCode(t, h, c, "ask", `{"to":"Codex","text":"hi","wait":true}`, "home_unreachable")

	home := newAskFakeHome()
	h.Attach("home-device", home)

	var failedCallID string
	home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
		var post proto.Post[proto.CallBody]
		if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
			t.Errorf("decode post: %v", err)
		}
		failedCallID = string(post.Nonce)
		return json.RawMessage(`{"re":1,"err":{"code":"limit","msg":"ask_depth"}}`), nil
	}
	assertToolErrorCode(t, h, c, "ask", `{"to":"Codex","text":"hi"}`, "limit")
	assertNoOpenWait(t, h, failedCallID)

	verifyAskContextCancelAndDrain(t, h, home, c)
}

func verifyAskContextCancelAndDrain(t *testing.T, h *Host, home *askFakeHome, c Caller) {
	t.Helper()
	// Single reader: canceledID receive below.
	called := make(chan string, 1)
	home.onCmd = func(_ int64, cmd proto.Cmd) (json.RawMessage, error) {
		var post proto.Post[proto.CallBody]
		if err := proto.Decode([]byte(cmd.Env.Payload), &post); err != nil {
			t.Errorf("decode post: %v", err)
		}
		called <- string(post.Nonce)
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	doneCh := startAsyncAsk(t, ctx, h, c, `{"to":"Codex","text":"cancel me"}`)
	canceledID := <-called
	cancel()
	if res := <-doneCh; res.err != context.Canceled {
		t.Fatalf("canceled ask returned %v, want context.Canceled", res.err)
	}
	assertNoOpenWait(t, h, canceledID)

	ch, done := h.Await(askCallMsgID, nil)
	if waited, err := h.Hand(askCallMsgID, askBuildReplyInboxItem(askCallMsgID, "Just in time", nil)); !waited || err != nil {
		t.Fatalf("Hand got %v, %v", waited, err)
	}
	if ans := finishAskTimeout(ch, done, askCallMsgID); ans != (askAnswered{Answer: "Just in time"}) {
		t.Fatalf("finishAskTimeout got %+v", ans)
	}
}

func assertReplyPost(t *testing.T, post proto.Post[proto.EmptyBody], wantTS int64, wantThread *proto.ID, wantCallID, wantText string) {
	t.Helper()
	re := proto.ID(wantCallID)
	secMustTrue(t, len(post.Nonce) == 22, "expected 22-char reply nonce")
	secMustEq(t, post, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     post.Nonce,
			Timestamp: wantTS,
			Group:     askGroupID,
			As:        askCalleeID,
		},
		Kind:     "reply",
		Thread:   wantThread,
		Text:     wantText,
		Mentions: []proto.ID{askCallerID},
		Re:       &re,
		Body:     proto.EmptyBody{},
	})
}

func TestReplyFetchesOrReadsCallAndSendsPost(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newAskFakeHome()
	h.Attach("home-device", home)
	callee := Caller{
		Session: Session{
			SessionKey: SessionKey{Group: askGroupID, Member: askCalleeID, Task: askTaskID},
			Kind:       TaskSession, Folder: "wt/12-path", Agent: "codex", Home: "home-device", Name: "Codex", State: Working,
		},
		Level: 2,
	}

	thread := askTaskID
	callRow := askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "waiting", &thread)
	var seenFetch proto.FetchRow
	home.onFetch = func(id int64, req proto.FetchRow) (json.RawMessage, error) {
		seenFetch = req
		resp := proto.Fetched{Type: "fetched", Success: proto.Success{Re: id, OK: true}, Rows: []json.RawMessage{callRow}}
		return json.RawMessage(marshalWire(resp)), nil
	}

	got, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askCallMsgID+`","text":"Use PathChanged()."}`))
	if err != nil {
		t.Fatal(err)
	}
	secMustEq(t, got, secOKReply{OK: true})
	secMustEq(t, seenFetch, proto.FetchRow{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 1}, Group: askGroupID, Limit: 1}, Row: "msg", RowID: askCallMsgID})
	post := decodeVerifiedCmdPost[proto.EmptyBody](t, h, home.frames()[1])
	wantTask := proto.ID(askTaskID)
	assertReplyPost(t, post, seamStart, &wantTask, askCallMsgID, "Use PathChanged().")

	verifyCachedMainLineReply(t, h, home, callee)
}

func verifyCachedMainLineReply(t *testing.T, h *Host, home *askFakeHome, callee Caller) {
	t.Helper()
	h.Rows.Put(askGroupID, "msg", askMainCallID, askBuildCallMsgRow(askMainCallID, askCallerID, askCalleeID, "waiting", nil))
	got, err := Tools["reply"].Call(t.Context(), h, callee, json.RawMessage(`{"call":"`+askMainCallID+`","text":"Main line answer."}`))
	if err != nil {
		t.Fatal(err)
	}
	secMustEq(t, got, secOKReply{OK: true})
	secMustEq(t, len(home.frames()), 3)
	mainReply := decodeVerifiedCmdPost[proto.EmptyBody](t, h, home.frames()[2], "/thread")
	assertReplyPost(t, mainReply, seamStart+2000, nil, askMainCallID, "Main line answer.")
}

func TestReplyRefusalAndErrorCases(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	callee := Caller{
		Session: Session{SessionKey: SessionKey{Group: askGroupID, Member: askCalleeID}, Kind: GroupSession, Home: "home-device", Name: "Codex"},
		Level:   1,
	}
	callArgs := `{"call":"` + askCallMsgID + `","text":"hi"}`

	assertToolErrorCode(t, h, callee, "reply", callArgs, "home_unreachable")

	home := newAskFakeHome()
	h.Attach("home-device", home)

	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) { return nil, io.EOF }
	assertToolErrorCode(t, h, callee, "reply", callArgs, "home_unreachable")

	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"forbidden","msg":"not a member"}}`), nil
	}
	assertToolErrorCode(t, h, callee, "reply", callArgs, "forbidden")

	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) {
		return json.RawMessage(`{"t":"fetched"}`), nil
	}
	assertToolErrorCode(t, h, callee, "reply", callArgs, "bad_request")

	home.onFetch = func(int64, proto.FetchRow) (json.RawMessage, error) {
		return json.RawMessage(`{"t":"fetched","re":1,"ok":true,"rows":[],"more":false}`), nil
	}
	assertToolErrorCode(t, h, callee, "reply", callArgs, "not_found")

	textRow := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: askCallMsgID, Seq: 3},
		Pos:       3, Group: askGroupID, By: askCallerID, Kind: "text", Timestamp: seamStart,
		Text: "plain text", Mentions: []string{}, Body: json.RawMessage(`{}`),
	}
	h.Rows.Put(askGroupID, "msg", askCallMsgID, json.RawMessage(marshalWire(textRow)))
	assertToolErrorCode(t, h, callee, "reply", callArgs, "conflict")

	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askSecID, "waiting", nil))
	assertToolErrorCode(t, h, callee, "reply", callArgs, "conflict")

	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "", nil))
	assertToolErrorCode(t, h, callee, "reply", callArgs, "conflict")

	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "answered", nil))
	assertToolErrorCode(t, h, callee, "reply", callArgs, "conflict")

	h.Rows.Put(askGroupID, "msg", askCallMsgID, askBuildCallMsgRow(askCallMsgID, askCallerID, askCalleeID, "waiting", nil))
	home.onCmd = func(int64, proto.Cmd) (json.RawMessage, error) {
		return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"already answered"}}`), nil
	}
	assertToolErrorCode(t, h, callee, "reply", callArgs, "conflict")
}
