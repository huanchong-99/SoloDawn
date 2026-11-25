package host

import (
	"bytes"
	"context"
	ed25519 "crypto/ed25519"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func mustApproveNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustApproveTrue(t *testing.T, cond bool, msg string) {
	t.Helper()
	if !cond {
		t.Fatal(msg)
	}
}

func mustApproveEqual[T comparable](t *testing.T, got, want T, label string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", label, got, want)
	}
}

func mustApproveDeepEqual(t *testing.T, got, want any, label string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %#v, want %#v", label, got, want)
	}
}

func recvApproveChan[T any](t *testing.T, ch <-chan T, label string) T {
	t.Helper()
	// Single reader: this helper drains one bounded timer or test channel value.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case val := <-ch:
		return val
	case <-timer.C:
		var zero T
		t.Fatal(timedOut(5*time.Second, label, nil))
		return zero
	}
}

func waitCallsZero(t *testing.T, h *Host, key SessionKey) {
	t.Helper()
	poll(t, 5*time.Second, fmt.Sprintf("Calls(%v) == 0", key), func() string {
		if n := h.Sessions.Calls(key); n != 0 {
			return fmt.Sprintf("Calls = %d", n)
		}
		return ""
	})
}

func approveTestClock() func() time.Time {
	var ms atomic.Int64
	ms.Store(seamStart - 1000)
	return func() time.Time {
		return time.UnixMilli(ms.Add(1000))
	}
}

func approveTestKey(seed byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)))
}

func approveTestID(seed byte) proto.ID {
	return proto.ID(b64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 16)))
}

type approveFakeHome struct {
	mu          sync.Mutex
	done        chan struct{}
	answer      string
	err         error
	posts       []proto.Post[proto.ApprovalBody]
	decisions   []proto.Post[proto.DecisionBody]
	fetches     []proto.FetchRow
	rawCmds     []proto.Cmd
	cards       map[string]json.RawMessage
	onPost      func(post proto.Post[proto.ApprovalBody])
	onDecision  func(post proto.Post[proto.DecisionBody])
	onFetchHook func(req proto.FetchRow) (json.RawMessage, error)
}

func newApproveFakeHome() *approveFakeHome {
	return &approveFakeHome{
		// Closed on disconnect; read via Done().
		done:  make(chan struct{}),
		cards: map[string]json.RawMessage{},
	}
}

func (f *approveFakeHome) Send(any) error               { return nil }
func (f *approveFakeHome) Path() string                 { return "local" }
func (f *approveFakeHome) PathChanged() <-chan struct{} { return nil }
func (f *approveFakeHome) Done() <-chan struct{}        { return f.done }

func (f *approveFakeHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	if cmd, ok := build(1).(proto.Cmd); ok && isMemberStatusCmd(cmd.Env.Payload) {
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	f.mu.Lock()
	id := int64(len(f.rawCmds) + len(f.fetches) + 1)
	req := build(id)
	if fetch, ok := req.(proto.FetchRow); ok {
		return f.handleFetchLocked(id, fetch)
	}
	return f.handleCmdLocked(req.(proto.Cmd))
}

func (f *approveFakeHome) handleFetchLocked(id int64, fetch proto.FetchRow) (json.RawMessage, error) {
	f.fetches = append(f.fetches, fetch)
	hook := f.onFetchHook
	rawCard, hasCard := f.cards[fetch.RowID]
	reqErr := f.err
	f.mu.Unlock()
	if hook != nil {
		return hook(fetch)
	}
	if reqErr != nil {
		return nil, reqErr
	}
	rows := []json.RawMessage{}
	if hasCard {
		rows = []json.RawMessage{rawCard}
	}
	return json.RawMessage(marshalWire(proto.Fetched{
		Type: "fetched", Success: proto.Success{Re: id, OK: true}, Rows: rows,
	})), nil
}

func (f *approveFakeHome) handleCmdLocked(cmd proto.Cmd) (json.RawMessage, error) {
	f.rawCmds = append(f.rawCmds, cmd)
	appPost, hasApp, decPost, hasDec := f.decodeAndRecordLocked([]byte(cmd.Env.Payload))
	ans, reqErr, appHook, decHook := f.answer, f.err, f.onPost, f.onDecision
	f.mu.Unlock()
	if reqErr != nil {
		return nil, reqErr
	}
	if ans != "" {
		return json.RawMessage(ans), nil
	}
	runPostHooks(hasApp, appPost, appHook, hasDec, decPost, decHook)
	return json.RawMessage(`{"re":1,"ok":true}`), nil
}

func runPostHooks(hasApp bool, appPost proto.Post[proto.ApprovalBody], appHook func(proto.Post[proto.ApprovalBody]), hasDec bool, decPost proto.Post[proto.DecisionBody], decHook func(proto.Post[proto.DecisionBody])) {
	if hasApp && appHook != nil {
		appHook(appPost)
	}
	if hasDec && decHook != nil {
		decHook(decPost)
	}
}

func (f *approveFakeHome) decodeAndRecordLocked(payload []byte) (proto.Post[proto.ApprovalBody], bool, proto.Post[proto.DecisionBody], bool) {
	var kindOnly struct {
		Kind string `json:"kind"`
	}
	json.Unmarshal(payload, &kindOnly)
	var appPost proto.Post[proto.ApprovalBody]
	hasApp := kindOnly.Kind == "approval" && proto.Decode(payload, &appPost) == nil
	if hasApp {
		f.posts = append(f.posts, appPost)
	}
	var decPost proto.Post[proto.DecisionBody]
	hasDec := kindOnly.Kind == "decision" && proto.Decode(payload, &decPost) == nil
	if hasDec {
		f.decisions = append(f.decisions, decPost)
	}
	return appPost, hasApp, decPost, hasDec
}

func (f *approveFakeHome) postList() []proto.Post[proto.ApprovalBody] {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.posts)
}

func (f *approveFakeHome) decisionList() []proto.Post[proto.DecisionBody] {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.decisions)
}

func (f *approveFakeHome) fetchList() []proto.FetchRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.fetches)
}

func (f *approveFakeHome) putCard(card proto.MsgRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cards[card.ID] = json.RawMessage(marshalWire(card))
}

type recordedAnswer struct {
	key   SessionKey
	askID string
	allow bool
}

type approveFakeRunner struct {
	mu      sync.Mutex
	answers []recordedAnswer
	ch      chan recordedAnswer
	inCall  chan int
}

func newApproveFakeRunner() *approveFakeRunner {
	return &approveFakeRunner{
		// Single reader: test goroutine waiting for Runner.Answer via recvApproveChan.
		ch: make(chan recordedAnswer, 16),
		// Single reader: test goroutine reading Sessions.Calls count via recvApproveChan.
		inCall: make(chan int, 16),
	}
}

func (r *approveFakeRunner) Turn(context.Context, *Host, SessionKey, string, Turn) error {
	return nil
}

func (r *approveFakeRunner) Answer(h *Host, key SessionKey, ask string, allow bool) error {
	rec := recordedAnswer{key: key, askID: ask, allow: allow}
	r.mu.Lock()
	r.answers = append(r.answers, rec)
	r.mu.Unlock()
	r.inCall <- h.Sessions.Calls(key)
	r.ch <- rec
	return nil
}

func (r *approveFakeRunner) Close(*Host, SessionKey) {}

func (r *approveFakeRunner) answerCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.answers)
}

type approveFixture struct {
	t        *testing.T
	db       *store.Store
	h        *Host
	home     *approveFakeHome
	owner1   identity.Key
	owner2   identity.Key
	delegate identity.Key
	homeKey  identity.Key
	hostKey  identity.Key
	groupID  string
	memberID string
	taskID   string
	human1ID string
	human2ID string
	sessKey  SessionKey
	itemSeq  int64
}

func newApproveFixture(t *testing.T) *approveFixture {
	t.Helper()
	db := seamStore(t)
	fx := &approveFixture{
		t:        t,
		db:       db,
		home:     newApproveFakeHome(),
		owner1:   approveTestKey(1),
		owner2:   approveTestKey(2),
		delegate: approveTestKey(3),
		homeKey:  approveTestKey(4),
		hostKey:  approveTestKey(5),
		groupID:  string(approveTestID(10)),
		memberID: string(approveTestID(11)),
		taskID:   string(approveTestID(12)),
		human1ID: string(approveTestID(13)),
		human2ID: string(approveTestID(14)),
		itemSeq:  100,
	}
	h, _, err := New(db, fx.hostKey, approveTestClock())
	mustApproveNoErr(t, err)
	fx.h = h
	fx.sessKey = SessionKey{Group: fx.groupID, Member: fx.memberID, Task: fx.taskID}
	seamSQL(t, fx.db, "INSERT INTO setting(key, value) VALUES('owner', '"+marshalWire(fx.owner1.ID())+"')")
	fx.h.Attach(fx.homeKey.ID(), fx.home)
	fx.seedDefaultRows()
	mustApproveNoErr(t, fx.h.Sessions.Create(Session{
		SessionKey: fx.sessKey,
		Kind:       TaskSession,
		Folder:     "wt/12-approve",
		Agent:      "claude",
		Home:       fx.homeKey.ID(),
		Name:       "Claude Worker",
	}))
	return fx
}

func (fx *approveFixture) seedDefaultRows() {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: fx.groupID, Seq: 1},
		Name:      "SoloDawn", Kind: "project", Base: "main", Forge: "none",
		Home: fx.homeKey.ID(), Checks: []string{}, Limits: proto.Limits{ApprovalWaitS: 20},
	}
	h1 := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.human1ID, Seq: 2}, Group: fx.groupID, Kind: "human", User: fx.owner1.ID(), Name: "Alice"},
		Role:      "owner",
	}
	h2 := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.human2ID, Seq: 3}, Group: fx.groupID, Kind: "human", User: fx.delegate.ID(), Name: "Bob"},
		Role:      "admin",
	}
	ai := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 4}, Group: fx.groupID, Kind: "ai", User: fx.owner1.ID(), Name: "Claude Worker"},
		CLI:       "claude", Host: fx.hostKey.ID(), Account: "personal",
		Allow: []string{}, Approvers: []string{fx.owner1.ID()}, State: "working",
	}
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalWire(grp)))
	fx.h.Rows.Put(fx.groupID, "member", fx.human1ID, json.RawMessage(marshalWire(h1)))
	fx.h.Rows.Put(fx.groupID, "member", fx.human2ID, json.RawMessage(marshalWire(h2)))
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalWire(ai)))
}

func (fx *approveFixture) caller(level int) Caller {
	s, ok := fx.h.Sessions.Get(fx.sessKey)
	mustApproveTrue(fx.t, ok, "missing test session")
	return Caller{Session: s, Level: level}
}

func (fx *approveFixture) callTool(ctx context.Context, level int, raw string) (ApprovalResult, error) {
	fx.t.Helper()
	val, err := Tools["approve"].Call(ctx, fx.h, fx.caller(level), json.RawMessage(raw))
	if err != nil {
		return ApprovalResult{}, err
	}
	return val.(ApprovalResult), nil
}

func (fx *approveFixture) mustCallTool(ctx context.Context, level int, raw string) ApprovalResult {
	fx.t.Helper()
	res, err := fx.callTool(ctx, level, raw)
	mustApproveNoErr(fx.t, err)
	return res
}

func (fx *approveFixture) makeCardMsg(card proto.ID, kind string, task string, state *proto.CardState) proto.MsgRow {
	var thread *string
	if task != "" {
		t := task
		thread = &t
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(card), Seq: 1},
		Pos:       1, Group: fx.groupID, Thread: thread, Main: true, By: fx.memberID,
		Kind: kind, Timestamp: seamStart, Mentions: []string{},
		Body: json.RawMessage(`{}`), State: state,
	}
}

func (fx *approveFixture) buildDecidedItem(actSigner identity.Key, card proto.ID, task, status string) json.RawMessage {
	fx.itemSeq++
	actEnv := actSigner.Sign(`{"t":"act"}`)
	by := fx.human1ID
	at := int64(seamStart)
	msg := fx.makeCardMsg(card, "approval", task, &proto.CardState{
		Status: status, By: &by, At: &at, Act: &actEnv,
	})
	return json.RawMessage(marshalWire(proto.InboxItem{
		Item: proto.Item{
			Item: fx.itemSeq,
			AI:   proto.ID(fx.memberID),
			Why:  "decided",
			Task: proto.ID(task),
			Msg:  card,
		},
		Msg: json.RawMessage(marshalWire(msg)),
	}))
}

func (fx *approveFixture) buildDecisionItem(actSigner identity.Key, s Session, card proto.ID, choice, text *string) json.RawMessage {
	fx.itemSeq++
	actEnv := actSigner.Sign(`{"t":"act"}`)
	by := fx.human1ID
	at := int64(seamStart)
	var task string
	if s.Kind == TaskSession || s.Kind == ReviewSession {
		task = s.Task
	}
	msg := fx.makeCardMsg(card, "decision", task, &proto.CardState{
		Status: "decided", By: &by, At: &at, Choice: choice, Text: text, Act: &actEnv,
	})
	return json.RawMessage(marshalWire(proto.InboxItem{
		Item: proto.Item{
			Item: fx.itemSeq,
			AI:   proto.ID(s.Member),
			Why:  "decided",
			Task: proto.ID(task),
			Msg:  card,
		},
		Msg: json.RawMessage(marshalWire(msg)),
	}))
}

func (fx *approveFixture) approvalRows() string {
	fx.t.Helper()
	var rows string
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT coalesce(group_concat(concat_ws('|', card, g, member, task, tool, state), char(10)), '')
			FROM (SELECT * FROM approval ORDER BY card)`).Scan(&rows)
	}
	mustApproveNoErr(fx.t, fx.db.Write(read))
	return rows
}

func TestApproveToolRegistrationSchemaAndVisibility(t *testing.T) {
	tool, ok := Tools["approve"]
	mustApproveTrue(t, ok, "Tools[approve] missing")
	mustApproveTrue(t, tool.Description != "", "Tools[approve] description empty")
	mustApproveTrue(t, tool.Call != nil, "Tools[approve] Call nil")
	mustApproveTrue(t, tool.For != nil, "Tools[approve] For nil")
	mustApproveTrue(t, Listeners["approve"] != nil, "Listeners[approve] nil")
	mustApproveTrue(t, !bytes.Contains(tool.Schema, []byte("additionalProperties")), "approveInputSchema must omit additionalProperties")

	dec := json.NewDecoder(bytes.NewReader(tool.Schema))
	dec.DisallowUnknownFields()
	var schema proto.ToolInput
	mustApproveNoErr(t, dec.Decode(&schema))
	wantProps := map[string]proto.ToolArg{
		"tool_name":   {Type: "string"},
		"input":       {Type: "object"},
		"tool_use_id": {Type: "string"},
	}
	mustApproveEqual(t, schema.Type, "object", "schema.Type")
	mustApproveDeepEqual(t, schema.Properties, wantProps, "schema.Properties")
	mustApproveDeepEqual(t, schema.Required, []string{"tool_name", "input", "tool_use_id"}, "schema.Required")

	mustApproveTrue(t, approveForCaller(Caller{Session: Session{Agent: "claude"}, Interactive: false}), "structured claude must see approve")
	mustApproveTrue(t, !approveForCaller(Caller{Session: Session{Agent: "codex"}, Interactive: false}), "codex must not see approve")
	mustApproveTrue(t, !approveForCaller(Caller{Session: Session{Agent: "claude"}, Interactive: true}), "interactive claude must not see approve")
}

type categoryMapCase struct {
	raw      string
	tool     string
	category string
	command  string
	why      *string
	level    int
}

func TestApproveArgumentValidationAndCategoryMapping(t *testing.T) {
	fx := newApproveFixture(t)
	assertBadApprovePayloads(t, fx)
	assertMappedToolCategories(t, fx)
}

func assertBadApprovePayloads(t *testing.T, fx *approveFixture) {
	t.Helper()
	badPayloads := []string{
		``,
		`{`,
		`[]`,
		`null`,
		`"str"`,
		`{"tool_name":"Bash","input":`,
		`{"input":{},"tool_use_id":"u1"}`,
		`{"tool_name":null,"input":{},"tool_use_id":"u1"}`,
		`{"tool_name":"","input":{},"tool_use_id":"u1"}`,
		`{"tool_name":"Bash","tool_use_id":"u1"}`,
		`{"tool_name":"Bash","input":{"command":123},"tool_use_id":"u1"}`,
		`{"tool_name":"Write","tool_use_id":"u1"}`,
		`{"tool_name":"AskUserQuestion","input":{"questions":"bad"},"tool_use_id":"u1"}`,
		`{"tool_name":"AskUserQuestion","input":{"questions":[]},"tool_use_id":"u1"}`,
		`{"tool_name":"AskUserQuestion","input":{},"tool_use_id":"u1"}`,
		`{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"ok"},{"question":""}]},"tool_use_id":"u1"}`,
	}
	for _, bad := range badPayloads {
		_, err := fx.callTool(t.Context(), 3, bad)
		seamCode(t, err, "bad_request")
	}
}

func assertMappedToolCategories(t *testing.T, fx *approveFixture) {
	t.Helper()
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(fx.owner2, post.Nonce, fx.taskID, "approved")
		waited, err := fx.h.Hand(string(post.Nonce), raw)
		mustApproveTrue(t, waited && err == nil, "Hand approved category case")
	}
	cases := []categoryMapCase{
		{raw: `{"tool_name":"Bash","input":{"command":"git push origin other","description":"Push feature"},"tool_use_id":"u1","permission_suggestions":[{"type":"addRules"}]}`, tool: "Bash", category: "command", command: "git push origin other", why: new("Push feature"), level: 3},
		{raw: `{"tool_name":"Bash","input":{"command":"ls -la"},"tool_use_id":""}`, tool: "Bash", category: "command", command: "ls -la", level: 2},
		{raw: "{\n  \"tool_name\": \"Write\",\n  \"input\": {\n    \"filePath\": \"/outside/a.txt\",\n    \"content\": \"x\"\n  },\n  \"tool_use_id\": \"u2\"\n}", tool: "Write", category: "file", command: `{"filePath":"/outside/a.txt","content":"x"}`, level: 3},
		{raw: `{"tool_name":"Edit","input":{"old_string":"a","new_string":"b"},"tool_use_id":"u3"}`, tool: "Edit", category: "file", command: `{"old_string":"a","new_string":"b"}`, level: 2},
		{raw: `{"tool_name":"NotebookEdit","input":{"notebook_path":"n.ipynb"},"tool_use_id":"u4"}`, tool: "NotebookEdit", category: "file", command: `{"notebook_path":"n.ipynb"}`, level: 3},
		{raw: `{"tool_name":"Read","input":{"file_path":"/etc/hosts"},"tool_use_id":"u5"}`, tool: "Read", category: "file", command: `{"file_path":"/etc/hosts"}`, level: 2},
		{raw: `{"tool_name":"WebFetch","input":{"url":"https://example.com"},"tool_use_id":"u6"}`, tool: "WebFetch", category: "network", command: `{"url":"https://example.com"}`, level: 3},
		{raw: `{"tool_name":"WebSearch","input":{"query":"golang"},"tool_use_id":"u7"}`, tool: "WebSearch", category: "network", command: `{"query":"golang"}`, level: 2},
		{raw: `{"tool_name":"CustomMCP","input":{"k":1},"tool_use_id":"u8"}`, tool: "CustomMCP", category: "other", command: `{"k":1}`, level: 3},
	}
	for _, tc := range cases {
		res := fx.mustCallTool(t.Context(), tc.level, tc.raw)
		mustApproveEqual(t, marshalWire(res), `{"behavior":"allow"}`, tc.tool)
		posts := fx.home.postList()
		assertPostedApprovalCard(t, fx, posts[len(posts)-1], tc)
	}
}

func assertPostedApprovalCard(t *testing.T, fx *approveFixture, got proto.Post[proto.ApprovalBody], tc categoryMapCase) {
	t.Helper()
	mustApproveEqual(t, got.Kind, "approval", "post.Kind")
	mustApproveEqual(t, got.Main, true, "post.Main")
	mustApproveEqual(t, got.Text, "", "post.Text")
	mustApproveTrue(t, got.Re == nil, "post.Re must be nil")
	mustApproveTrue(t, got.Thread != nil && string(*got.Thread) == fx.taskID, "post.Thread")
	mustApproveEqual(t, got.Body.Task, fx.taskID, "Body.Task")
	mustApproveEqual(t, string(got.As), fx.memberID, "post.As")
	mustApproveEqual(t, got.Body.Tool, tc.tool, "Body.Tool")
	mustApproveEqual(t, got.Body.Category, tc.category, "Body.Category")
	mustApproveEqual(t, got.Body.Command, tc.command, "Body.Command")
	mustApproveTrue(t, got.Body.Full == nil, "Body.Full must be nil")
	mustApproveDeepEqual(t, got.Body.Why, tc.why, "Body.Why")
}

func TestCardsAcrossTaskReviewAndGroupSessions(t *testing.T) {
	fx := newApproveFixture(t)
	reviewTask := string(approveTestID(45))
	reviewKey := SessionKey{Group: fx.groupID, Member: fx.memberID, Task: reviewTask}
	groupKey := SessionKey{Group: fx.groupID, Member: fx.memberID}
	mustApproveNoErr(t, fx.h.Sessions.Create(Session{
		SessionKey: reviewKey, Kind: ReviewSession, Folder: "review/45", Agent: "claude", Home: fx.homeKey.ID(), Name: "Reviewer",
	}))
	mustApproveNoErr(t, fx.h.Sessions.Create(Session{
		SessionKey: groupKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: fx.homeKey.ID(), Name: "Secretary", Secretary: true,
	}))

	assertSessionKindApproval(t, fx, fx.sessKey, fx.taskID)
	assertSessionKindApproval(t, fx, reviewKey, reviewTask)
	assertSessionKindApproval(t, fx, groupKey, "")

	assertSessionKindDecision(t, fx, fx.sessKey, fx.taskID, 3)
	assertSessionKindDecision(t, fx, reviewKey, reviewTask, 2)
	assertSessionKindDecision(t, fx, groupKey, "", 1)
}

func assertSessionKindApproval(t *testing.T, fx *approveFixture, key SessionKey, wantTask string) {
	t.Helper()
	s, _ := fx.h.Sessions.Get(key)
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecidedItem(fx.owner1, post.Nonce, wantTask, "approved"))
		mustApproveTrue(t, waited && err == nil, "session kind approval Hand")
	}
	val, err := Tools["approve"].Call(t.Context(), fx.h, Caller{Session: s, Level: 3}, json.RawMessage(`{"tool_name":"Bash","input":{"command":"echo ok"},"tool_use_id":"u_sk"}`))
	mustApproveNoErr(t, err)
	mustApproveEqual(t, val.(ApprovalResult).Behavior, "allow", string(s.Kind)+" approval allow")
	posts := fx.home.postList()
	got := posts[len(posts)-1]
	mustApproveEqual(t, got.Body.Task, wantTask, string(s.Kind)+" approval Body.Task")
	if wantTask == "" {
		mustApproveTrue(t, got.Thread == nil, "group approval Thread must be nil")
		return
	}
	mustApproveTrue(t, got.Thread != nil && string(*got.Thread) == wantTask, string(s.Kind)+" approval Thread")
}

func assertSessionKindDecision(t *testing.T, fx *approveFixture, key SessionKey, wantTask string, level int) {
	t.Helper()
	s, _ := fx.h.Sessions.Get(key)
	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		txt := "Picked"
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecisionItem(fx.owner1, s, post.Nonce, nil, &txt))
		mustApproveTrue(t, waited && err == nil, "session kind decision Hand")
	}
	raw := json.RawMessage(`{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"Proceed?"}]},"tool_use_id":"u_dq"}`)
	val, err := Tools["approve"].Call(t.Context(), fx.h, Caller{Session: s, Level: level}, raw)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, val.(ApprovalResult).Behavior, "allow", string(s.Kind)+" decision allow")
	decs := fx.home.decisionList()
	got := decs[len(decs)-1]
	if wantTask == "" {
		mustApproveTrue(t, got.Thread == nil && got.Body.Task == nil, "group decision Thread and Task must be nil")
		return
	}
	mustApproveTrue(t, got.Thread != nil && string(*got.Thread) == wantTask, string(s.Kind)+" decision Thread")
	mustApproveTrue(t, got.Body.Task != nil && *got.Body.Task == wantTask, string(s.Kind)+" decision Body.Task")
}

func TestApproveLevelOneAndOutboundSecretCheck(t *testing.T) {
	fx := newApproveFixture(t)
	res1 := fx.mustCallTool(t.Context(), 1, `{"tool_name":"Bash","input":{"command":"touch x"},"tool_use_id":"u1"}`)
	mustApproveEqual(t, marshalWire(res1), `{"behavior":"deny","message":"Denied: this session is read only."}`, "level 1")
	mustApproveEqual(t, len(fx.home.postList()), 0, "level 1 posts")
	mustApproveEqual(t, fx.approvalRows(), "", "level 1 rows")

	secretTok := "ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD"
	longWithSecret := strings.Repeat("a", 4000) + " " + secretTok
	argCmdSecret := marshalWire(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": longWithSecret, "description": "Safe justification"},
		"tool_use_id": "u3",
	})
	resSec1 := fx.mustCallTool(t.Context(), 3, argCmdSecret)
	mustApproveDeepEqual(t, resSec1, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "command secret")
	mustApproveTrue(t, !strings.Contains(marshalWire(resSec1), secretTok), "deny leaked secret")

	argWhySecret := marshalWire(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": "echo ok", "description": "Using -----BEGIN PRIVATE KEY-----"},
		"tool_use_id": "u4",
	})
	resSec2 := fx.mustCallTool(t.Context(), 2, argWhySecret)
	mustApproveDeepEqual(t, resSec2, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "why secret")
	mustApproveEqual(t, len(fx.home.postList()), 0, "secret posts")
	mustApproveEqual(t, fx.approvalRows(), "", "secret rows")
	mustApproveEqual(t, seamRecord(t, fx.h), "", "no audit rows written")
}

func jsonEncodedLen(s string) int {
	return len(marshalWire(s))
}

func TestShortenJSONTextAndCardLimitsBoundaries(t *testing.T) {
	exactCmd := strings.Repeat("a", 3070)
	short, full := shortenCommand(exactCmd)
	mustApproveEqual(t, short, exactCmd, "exact 3072 short")
	mustApproveTrue(t, full == nil, "exact 3072 full nil")
	mustApproveEqual(t, jsonEncodedLen(short), 3072, "exact 3072 jsonLen")

	overCmd := strings.Repeat("a", 3071)
	shortOver, fullOver := shortenCommand(overCmd)
	mustApproveEqual(t, len(shortOver), 3070, "over 3072 cut len")
	mustApproveTrue(t, fullOver != nil && *fullOver == 3071, "over 3072 full value")
	mustApproveEqual(t, jsonEncodedLen(shortOver), 3072, "over 3072 jsonLen")

	htmlExact := strings.Repeat("<&>", 1023) + "a"
	shortHTML, fullHTML := shortenCommand(htmlExact)
	mustApproveEqual(t, shortHTML, htmlExact, "html exact")
	mustApproveTrue(t, fullHTML == nil, "html full nil")
	mustApproveEqual(t, jsonEncodedLen(shortHTML), 3072, "html jsonLen")

	utf8Straddle := strings.Repeat("b", 3069) + "€"
	shortUTF8, fullUTF8 := shortenCommand(utf8Straddle)
	mustApproveEqual(t, shortUTF8, strings.Repeat("b", 3069), "utf8 straddle cut")
	mustApproveTrue(t, utf8.ValidString(shortUTF8), "utf8 valid")
	mustApproveTrue(t, fullUTF8 != nil && *fullUTF8 == 3072, "utf8 full bytes")
	mustApproveEqual(t, jsonEncodedLen(shortUTF8), 3071, "utf8 jsonLen")

	nlExact := strings.Repeat("\n", 1535)
	nlShort, nlFull := shortenCommand(nlExact)
	mustApproveEqual(t, nlShort, nlExact, "newline exact")
	mustApproveTrue(t, nlFull == nil, "newline exact full nil")

	nlOver := strings.Repeat("\n", 1536)
	nlOverShort, nlOverFull := shortenCommand(nlOver)
	mustApproveEqual(t, len(nlOverShort), 1535, "newline over cut")
	mustApproveTrue(t, nlOverFull != nil && *nlOverFull == 1536, "newline over full len")

	mustApproveTrue(t, optionalWhy("") == nil, "empty why nil")
	whyExact := strings.Repeat("w", 254)
	gotWhyExact := optionalWhy(whyExact)
	mustApproveTrue(t, gotWhyExact != nil && *gotWhyExact == whyExact, "why exact val")
	mustApproveEqual(t, jsonEncodedLen(*gotWhyExact), 256, "why exact jsonLen")

	gotWhyOver := optionalWhy(strings.Repeat("w", 255))
	mustApproveTrue(t, gotWhyOver != nil && *gotWhyOver == whyExact, "why over val")
	mustApproveEqual(t, jsonEncodedLen(*gotWhyOver), 256, "why over jsonLen")

	d1 := hashApprovalInput(json.RawMessage(`{"b":9007199254740993,"a":1}`))
	d2 := hashApprovalInput(json.RawMessage(`{ "a": 1, "b": 9007199254740993 }`))
	d3 := hashApprovalInput(json.RawMessage(`{"a":1,"b":9007199254740992}`))
	mustApproveEqual(t, d1, d2, "canonical JSON digest")
	mustApproveTrue(t, d1 != d3, "UseNumber 64-bit integer precision")
	mustApproveEqual(t, len(hashApprovalInput(json.RawMessage("  not-json  "))), 64, "non-JSON digest len")
}

func TestTimelyApproveDenyHomeRefusalAndApproverResolution(t *testing.T) {
	fx := newApproveFixture(t)
	delegateDev2 := seedDelegateAndExtraHuman(fx)
	assertTimelyDelegateApprove(t, fx, delegateDev2)
	assertTimelyOwnerDenyAndRefusals(t, fx)
	assertApproverFallbackChain(t, fx)
}

func seedDelegateAndExtraHuman(fx *approveFixture) identity.Key {
	otherUser := approveTestKey(9)
	human3ID := string(approveTestID(15))
	fx.h.Rows.Put(fx.groupID, "member", human3ID, json.RawMessage(marshalWire(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: human3ID, Seq: 5}, Group: fx.groupID, Kind: "human", User: otherUser.ID(), Name: "Carol"},
		Role:      "member",
	})))
	fx.h.Rows.Put(fx.groupID, "member", "bad-member", json.RawMessage(`{"r":"member"`))
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalWire(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 6}, Group: fx.groupID, Kind: "ai", User: fx.owner1.ID(), Name: "Claude Worker"},
		CLI:       "claude", Host: fx.hostKey.ID(), Account: "personal",
		Allow: []string{}, Approvers: []string{fx.owner1.ID(), fx.delegate.ID()}, State: "working",
	})))
	return approveTestKey(8)
}

func assertTimelyDelegateApprove(t *testing.T, fx *approveFixture, delegateDev2 identity.Key) {
	t.Helper()
	// Hand inside Home.Request so the test proves Await opened before Command.
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(delegateDev2, post.Nonce, fx.taskID, "approved")
		waited, err := fx.h.Hand(string(post.Nonce), raw)
		mustApproveTrue(t, waited && err == nil, "timely delegate Hand")
	}
	resAllow := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"npm test","description":"Run suite"},"tool_use_id":"u1"}`)
	mustApproveDeepEqual(t, resAllow, ApprovalResult{Behavior: "allow"}, "timely allow")
	post := fx.home.postList()[0]
	wantMentions := []proto.ID{proto.ID(fx.human1ID), proto.ID(fx.human2ID)}
	slices.Sort(wantMentions)
	mustApproveDeepEqual(t, post.Mentions, wantMentions, "post.Mentions")
	mustApproveDeepEqual(t, post.Body.Approvers, []string{fx.owner1.ID(), fx.delegate.ID()}, "post.Body.Approvers")
	mustApproveEqual(t, fx.approvalRows(), "", "timely allow rows")
	mustApproveEqual(t, seamRecord(t, fx.h), "", "timely allow writes no audit rows")
}

func assertTimelyOwnerDenyAndRefusals(t *testing.T, fx *approveFixture) {
	t.Helper()
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(fx.owner2, post.Nonce, fx.taskID, "denied")
		waited, err := fx.h.Hand(string(post.Nonce), raw)
		mustApproveTrue(t, waited && err == nil, "timely deny Hand")
	}
	resDeny := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"rm -rf /tmp/x"},"tool_use_id":"u2"}`)
	mustApproveEqual(t, marshalWire(resDeny), `{"behavior":"deny","message":"Denied by the owner."}`, "timely deny")
	mustApproveEqual(t, fx.approvalRows(), "", "timely deny rows")

	fx.home.onPost = nil
	fx.home.answer = `{"re":1,"err":{"code":"conflict","msg":"task is not active"}}`
	resRefused := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"echo hi"},"tool_use_id":"u3"}`)
	mustApproveDeepEqual(t, resRefused, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: conflict: task is not active"}, "home refusal")
	mustApproveEqual(t, fx.approvalRows(), "", "home refusal rows")

	fx.home.answer = ""
	fx.home.err = io.EOF
	resUnreach := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"echo hi"},"tool_use_id":"u4"}`)
	mustApproveDeepEqual(t, resUnreach, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: home_unreachable: the home of that group is not connected"}, "home unreachable")
	mustApproveEqual(t, fx.approvalRows(), "", "home unreachable rows")
}

func assertApproverFallbackChain(t *testing.T, fx *approveFixture) {
	t.Helper()
	fx.h.Rows.Reset(fx.groupID)
	seamSQL(t, fx.db, `INSERT INTO setting(key, value) VALUES('approvers', '["user-a","user-b"]')`)
	gotApp1, gotMen1 := resolveApproversAndMentions(fx.h, fx.groupID, fx.memberID)
	mustApproveDeepEqual(t, gotApp1, []string{"user-a", "user-b"}, "setting approvers fallback")
	mustApproveDeepEqual(t, gotMen1, []proto.ID{}, "empty mentions slice")

	seamSQL(t, fx.db, `UPDATE setting SET value = '[]' WHERE key = 'approvers'`)
	gotApp2, gotMen2 := resolveApproversAndMentions(fx.h, fx.groupID, fx.memberID)
	mustApproveDeepEqual(t, gotApp2, []string{fx.owner1.ID()}, "owner fallback")
	mustApproveDeepEqual(t, gotMen2, []proto.ID{}, "owner fallback mentions")

	seamSQL(t, fx.db, `DELETE FROM setting WHERE key = 'owner'`)
	gotApp3, gotMen3 := resolveApproversAndMentions(fx.h, fx.groupID, fx.memberID)
	mustApproveDeepEqual(t, gotApp3, []string{}, "empty approvers fallback")
	mustApproveDeepEqual(t, gotMen3, []proto.ID{}, "empty fallback mentions")

	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(`{"r":"member"`))
	mustApproveTrue(t, memberApprovers(fx.h.Rows, fx.groupID, fx.memberID) == nil, "corrupt member row nil")
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalWire(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 9}, Group: fx.groupID, Kind: "human", User: fx.owner1.ID(), Name: "Human"},
		Role:      "owner",
	})))
	mustApproveTrue(t, memberApprovers(fx.h.Rows, fx.groupID, fx.memberID) == nil, "human member row nil")
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalWire(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 10}, Group: fx.groupID, Kind: "ai", User: fx.owner1.ID(), Name: "AI"},
		CLI:       "claude", Host: fx.hostKey.ID(), Account: "personal", Allow: []string{}, Approvers: []string{}, State: "working",
	})))
	mustApproveTrue(t, memberApprovers(fx.h.Rows, fx.groupID, fx.memberID) == nil, "empty ai approvers nil")
}

func TestTimeoutNonDecidedItemsAndFetchRetry(t *testing.T) {
	fx := newApproveFixture(t)
	grpOne := assertWaitDurationAndSelectBranches(t, fx)
	assertNonDecidedHandedItemsCountAsNoDecision(t, fx)
	assertTimeoutRestartAndFetchRetry(t, fx, grpOne)
	assertFetchPendingApprovalFallback(t, fx)
	assertStoreFailurePaths(t)
}

func assertWaitDurationAndSelectBranches(t *testing.T, fx *approveFixture) proto.GroupRow {
	t.Helper()
	fx.h.Rows.Reset(fx.groupID)
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 600*time.Second, "missing group wait")
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(`{"r":"group"`))
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 600*time.Second, "corrupt group wait")

	fx.seedDefaultRows()
	grpZero := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: fx.groupID, Seq: 10},
		Name:      "SoloDawn", Kind: "project", Base: "main", Forge: "none", Home: fx.homeKey.ID(), Checks: []string{},
		Limits: proto.Limits{ApprovalWaitS: 0},
	}
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalWire(grpZero)))
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 600*time.Second, "zero ApprovalWaitS")

	grpOne := grpZero
	grpOne.Seq = 11
	grpOne.Limits.ApprovalWaitS = 1
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalWire(grpOne)))
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), time.Second, "ApprovalWaitS=1")

	cancelCtx, cancel := context.WithCancel(t.Context())
	cancel()
	// Single reader: awaitApprovalItem under test.
	emptyCh := make(chan json.RawMessage, 1)
	gotEmpty, okEmpty := awaitApprovalItem(cancelCtx, time.Hour, emptyCh, func() {})
	mustApproveEqual(t, okEmpty, false, "cancelled empty wait ok")
	mustApproveTrue(t, gotEmpty == nil, "cancelled empty wait raw")

	// Single reader: awaitApprovalItem post-done drain under test.
	raceCh := make(chan json.RawMessage, 1)
	raceItem := json.RawMessage(`{"ok":true}`)
	gotRace, okRace := awaitApprovalItem(cancelCtx, time.Hour, raceCh, func() { raceCh <- raceItem })
	mustApproveEqual(t, okRace, true, "post-done drain ok")
	mustApproveEqual(t, string(gotRace), string(raceItem), "post-done drain raw")
	return grpOne
}

func assertNonDecidedHandedItemsCountAsNoDecision(t *testing.T, fx *approveFixture) {
	t.Helper()
	validMsg := marshalWire(fx.makeCardMsg(approveTestID(80), "approval", fx.taskID, &proto.CardState{Status: "approved"}))
	nilStateMsg := marshalWire(fx.makeCardMsg(approveTestID(80), "approval", fx.taskID, nil))
	pendingAppMsg := marshalWire(fx.makeCardMsg(approveTestID(80), "approval", fx.taskID, &proto.CardState{Status: "pending"}))
	pendingDecMsg := marshalWire(fx.makeCardMsg(approveTestID(81), "decision", fx.taskID, &proto.CardState{Status: "pending"}))

	badItems := []string{
		`{`,
		`{"item":{"item":1,"ai":"not-an-id","why":"decided"},"msg":` + validMsg + `}`,
		marshalWire(proto.InboxItem{Item: proto.Item{Item: 1, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(80)}, Msg: json.RawMessage(`{`)}),
		marshalWire(proto.InboxItem{Item: proto.Item{Item: 1, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(80)}, Msg: json.RawMessage(nilStateMsg)}),
		marshalWire(proto.InboxItem{Item: proto.Item{Item: 1, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(80)}, Msg: json.RawMessage(pendingAppMsg)}),
	}
	for i, bad := range badItems {
		rawBad := json.RawMessage(bad)
		fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
			waited, err := fx.h.Hand(string(post.Nonce), rawBad)
			mustApproveTrue(t, waited && err == nil, "Hand non-decided approval item")
		}
		res := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"cmd-`+strconv.Itoa(i)+`"},"tool_use_id":"u_nd"}`)
		mustApproveDeepEqual(t, res, ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "non-decided approval item")
	}
	fx.home.onPost = nil
	mustApproveEqual(t, strings.Count(fx.approvalRows(), "|pending"), len(badItems), "pending rows kept on non-decided items")
	seamSQL(t, fx.db, "DELETE FROM approval")

	rawPendingDec := json.RawMessage(marshalWire(proto.InboxItem{
		Item: proto.Item{Item: 2, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(81)},
		Msg:  json.RawMessage(pendingDecMsg),
	}))
	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		waited, err := fx.h.Hand(string(post.Nonce), rawPendingDec)
		mustApproveTrue(t, waited && err == nil, "Hand non-decided decision item")
	}
	resDec := fx.mustCallTool(t.Context(), 3, `{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"Still pending?"}]},"tool_use_id":"u_pdec"}`)
	mustApproveDeepEqual(t, resDec, ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "non-decided decision item")
	fx.home.onDecision = nil
}

func assertTimeoutRestartAndFetchRetry(t *testing.T, fx *approveFixture, grpOne proto.GroupRow) {
	t.Helper()
	// Run the 1-second approval timeout and 1-second decision timeout concurrently.
	// Single reader: recvApproveChan reading approval timeout outcome.
	appOut := make(chan ApprovalResult, 1)
	// Single reader: recvApproveChan reading decision timeout outcome.
	decOut := make(chan ApprovalResult, 1)
	go func() {
		appOut <- fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_t1"}`)
	}()
	go func() {
		decOut <- fx.mustCallTool(t.Context(), 3, `{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"Timed out?"}]},"tool_use_id":"u_qt1"}`)
	}()
	mustApproveDeepEqual(t, recvApproveChan(t, appOut, "approval timeout"), ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "approval timeout deny")
	mustApproveDeepEqual(t, recvApproveChan(t, decOut, "decision timeout"), ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "decision timeout deny")

	posts := fx.home.postList()
	card1 := posts[len(posts)-1].Nonce
	mustApproveTrue(t, strings.HasSuffix(fx.approvalRows(), "|Write|pending"), "pending row after timeout")

	restarted, _, err := New(fx.db, fx.hostKey, approveTestClock())
	mustApproveNoErr(t, err)
	fx.h = restarted
	fx.h.Attach(fx.homeKey.ID(), fx.home)
	fx.seedDefaultRows()
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalWire(grpOne)))

	// Mark the pending card approved on the home; non-matching tool/input still ask, matching input consumes once.
	actEnv := fx.owner2.Sign(`{"t":"act"}`)
	fx.home.putCard(fx.makeCardMsg(card1, "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actEnv}))

	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecidedItem(fx.owner1, post.Nonce, fx.taskID, "denied"))
		mustApproveTrue(t, waited && err == nil, "non-matching deny Hand")
	}
	resDiffTool := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Edit","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_diff_tool"}`)
	resDiffInput := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"changed"},"tool_use_id":"u_diff_in"}`)
	mustApproveEqual(t, resDiffTool.Behavior, "deny", "different tool does not match pending card")
	mustApproveEqual(t, resDiffInput.Behavior, "deny", "different input does not match pending card")

	postsBefore := len(fx.home.postList())
	resRetry := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{ "content": "<v1>&v2", "file_path": "/ext/a.txt" },"tool_use_id":"u_retry_ok"}`)
	mustApproveDeepEqual(t, resRetry, ApprovalResult{Behavior: "allow"}, "fetched approved retry")
	mustApproveEqual(t, len(fx.home.postList()), postsBefore, "fetched retry posted no new card")
	mustApproveEqual(t, fx.approvalRows(), "", "consumed pending row deleted")

	resSecondTry := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_retry_2"}`)
	mustApproveEqual(t, resSecondTry.Behavior, "deny", "second retry asks again")
	mustApproveEqual(t, len(fx.home.postList()), postsBefore+1, "second retry posted new card")
	fx.home.onPost = nil
}

func assertFetchPendingApprovalFallback(t *testing.T, fx *approveFixture) {
	t.Helper()
	s, ok := fx.h.Sessions.Get(fx.sessKey)
	mustApproveTrue(t, ok, "session must exist")
	digest := hashApprovalInput(json.RawMessage(`{"command":"npm publish"}`))
	cards := []proto.ID{approveTestID(61), approveTestID(62), approveTestID(63), approveTestID(64), approveTestID(65)}
	for _, c := range cards {
		mustApproveNoErr(t, storePendingApproval(fx.h, c, s, "Bash", digest))
	}

	assertFetchErrorPathsLeavePending(t, fx, s, digest, cards[0])

	actEnv := fx.owner2.Sign(`{"t":"act"}`)
	fx.home.putCard(fx.makeCardMsg(cards[0], "approval", fx.taskID, &proto.CardState{Status: "denied"}))
	fx.home.putCard(fx.makeCardMsg(cards[1], "approval", fx.taskID, &proto.CardState{Status: "expired"}))
	fx.home.putCard(fx.makeCardMsg(cards[2], "approval", fx.taskID, &proto.CardState{Status: "reassigned"}))
	fx.home.putCard(fx.makeCardMsg(cards[3], "approval", fx.taskID, &proto.CardState{Status: "pending"}))
	fx.home.putCard(fx.makeCardMsg(cards[4], "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actEnv}))

	mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), true, "fetch consumes approved card")
	mustApproveEqual(t, fx.approvalRows(), string(cards[3])+"|"+fx.groupID+"|"+fx.memberID+"|"+fx.taskID+"|Bash|pending", "only still-pending card remains")
	fetches := fx.home.fetchList()
	lastFetch := fetches[len(fetches)-1]
	mustApproveEqual(t, lastFetch.Type, "fetch", "fetch.Type")
	mustApproveEqual(t, lastFetch.Group, fx.groupID, "fetch.Group")
	mustApproveEqual(t, lastFetch.Before, int64(0), "fetch.Before")
	mustApproveEqual(t, lastFetch.Limit, 1, "fetch.Limit")
	mustApproveEqual(t, lastFetch.Row, "msg", "fetch.Row")
	mustApproveEqual(t, lastFetch.RowID, string(cards[4]), "fetch.RowID")

	// Concurrent deletion between fetch and deletePendingCard yields false.
	fx.home.putCard(fx.makeCardMsg(cards[3], "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actEnv}))
	fx.home.onFetchHook = func(req proto.FetchRow) (json.RawMessage, error) {
		deleteApprovalCard(fx.h, req.RowID)
		row := fx.home.cards[req.RowID]
		return json.RawMessage(marshalWire(proto.Fetched{
			Type: "fetched", Success: proto.Success{Re: 1, OK: true}, Rows: []json.RawMessage{row},
		})), nil
	}
	mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), false, "concurrent delete during fetch")
	fx.home.onFetchHook = nil
}

func assertFetchErrorPathsLeavePending(t *testing.T, fx *approveFixture, s Session, digest string, sampleCard proto.ID) {
	t.Helper()
	discHome := newApproveFakeHome()
	close(discHome.done)
	fx.h.Attach(fx.homeKey.ID(), discHome)
	mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), false, "disconnected home fetch")
	fx.h.Attach(fx.homeKey.ID(), fx.home)

	fx.home.err = io.EOF
	mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), false, "fetch transport err")
	fx.home.err = nil

	for _, badAns := range []string{
		`{"re":1,"err":{"code":"not_found","msg":"missing"}}`,
		`{"t":"fetched","re":1,"ok":true,"rows":[],"more":false}`,
		`{"t":"fetched","re":1,"ok":true,"rows":[{}],"more":false}`,
		marshalWire(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 1, OK: true}, Rows: []json.RawMessage{json.RawMessage(marshalWire(fx.makeCardMsg(sampleCard, "text", fx.taskID, nil)))}}),
		marshalWire(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 1, OK: true}, Rows: []json.RawMessage{json.RawMessage(marshalWire(fx.makeCardMsg(sampleCard, "approval", fx.taskID, nil)))}}),
	} {
		ansCopy := badAns
		fx.home.onFetchHook = func(proto.FetchRow) (json.RawMessage, error) {
			return json.RawMessage(ansCopy), nil
		}
		mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), false, "bad fetch payload")
	}
	fx.home.onFetchHook = nil
}

func assertStoreFailurePaths(t *testing.T) {
	t.Helper()
	fx := newApproveFixture(t)
	s, _ := fx.h.Sessions.Get(fx.sessKey)
	card := approveTestID(90)
	digest := hashApprovalInput(json.RawMessage(`{"command":"echo x"}`))
	mustApproveNoErr(t, storePendingApproval(fx.h, card, s, "Bash", digest))

	rowJSON := json.RawMessage(marshalWire(fx.makeCardMsg(card, "approval", fx.taskID, &proto.CardState{Status: "approved"})))
	fx.home.onFetchHook = func(proto.FetchRow) (json.RawMessage, error) {
		seamSQL(t, fx.db, "DROP TABLE approval")
		return json.RawMessage(marshalWire(proto.Fetched{
			Type: "fetched", Success: proto.Success{Re: 1, OK: true}, Rows: []json.RawMessage{rowJSON},
		})), nil
	}
	mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), false, "deletePendingCard error after DROP TABLE")
	fx.home.onFetchHook = nil

	mustApproveEqual(t, consumePendingApproval(t.Context(), fx.h, s, "Bash", digest), false, "listPendingCards error after DROP TABLE")
	res := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"echo x"},"tool_use_id":"u_no_tbl"}`)
	mustApproveDeepEqual(t, res, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: internal: failed to store approval record"}, "missing approval table")
}

func TestClaudeAskUserQuestionDecisionCardsAndAnswers(t *testing.T) {
	fx := newApproveFixture(t)
	assertClaudeQuestionSecretChecks(t, fx)
	assertClaudeQuestionOptionIDsAndOneAtATime(t, fx)
}

func assertClaudeQuestionSecretChecks(t *testing.T, fx *approveFixture) {
	t.Helper()
	secretTok := "ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD"
	secretInputs := []map[string]any{
		{"questions": []map[string]any{{"question": "Safe?"}, {"question": "Leak " + secretTok, "header": "H", "options": []map[string]string{{"label": "A", "description": "B"}}}}},
		{"questions": []map[string]any{{"question": "Clean", "header": secretTok, "options": []map[string]string{{"label": "A", "description": "B"}}}}},
		{"questions": []map[string]any{{"question": "Clean", "header": "H", "options": []map[string]string{{"label": secretTok, "description": "B"}}}}},
		{"questions": []map[string]any{{"question": "Clean", "header": "H", "options": []map[string]string{{"label": "A", "description": secretTok}}}}},
	}
	for _, in := range secretInputs {
		raw := marshalWire(map[string]any{"tool_name": "AskUserQuestion", "input": in, "tool_use_id": "u_sec"})
		res := fx.mustCallTool(t.Context(), 3, raw)
		mustApproveDeepEqual(t, res, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "AskUserQuestion secret")
	}
	mustApproveEqual(t, len(fx.home.decisionList()), 0, "no decision cards posted on secret")
}

func assertClaudeQuestionOptionIDsAndOneAtATime(t *testing.T, fx *approveFixture) {
	t.Helper()
	opts26 := make([]map[string]string, 26)
	for i := range opts26 {
		opts26[i] = map[string]string{"label": "Opt-" + strconv.Itoa(i), "description": "Desc-" + strconv.Itoa(i)}
	}
	s, _ := fx.h.Sessions.Get(fx.sessKey)
	step := 0
	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		step++
		// Pin that multi-question calls post one card at a time.
		mustApproveEqual(t, len(fx.home.decisionList()), step, "one decision card posted at a time")
		choice, note := scriptedClaudeChoice(step)
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecisionItem(fx.owner2, s, post.Nonce, choice, note))
		mustApproveTrue(t, waited && err == nil, "decision Hand")
	}

	raw := marshalWire(map[string]any{
		"tool_name": "AskUserQuestion",
		"input": map[string]any{
			"extraKeep": "kept",
			"questions": []map[string]any{{"question": "Q1?", "header": "H1", "options": opts26, "multiSelect": true}},
		},
		"tool_use_id": "u_q1",
	})
	res1 := fx.mustCallTool(t.Context(), 3, raw)
	mustApproveEqual(t, res1.Behavior, "allow", "Q1 behavior")
	mustApproveEqual(t, res1.UpdatedInput["extraKeep"], "kept", "original input preserved")
	_, hasAnn1 := res1.UpdatedInput["annotations"]
	mustApproveEqual(t, hasAnn1, false, "no annotations without note")
	mustApproveDeepEqual(t, res1.UpdatedInput["answers"], map[string]string{"Q1?": "Opt-25"}, "Q1 answer Z")

	decCard := fx.home.decisionList()[0]
	mustApproveEqual(t, len(decCard.Body.Options), 26, "26 options count")
	mustApproveEqual(t, decCard.Body.Options[0].ID, "A", "option 0 ID")
	mustApproveEqual(t, decCard.Body.Options[25].ID, "Z", "option 25 ID")
	mustApproveTrue(t, decCard.Body.Context != nil && *decCard.Body.Context == "H1", "decision Context")

	rawMulti := marshalWire(map[string]any{
		"tool_name": "AskUserQuestion",
		"input": map[string]any{
			"questions": []map[string]any{
				{"question": "Q2?", "header": "", "options": opts26},
				{"question": "Q3?", "header": "", "options": []map[string]string{}},
				{"question": "Q4?", "header": "", "options": []map[string]string{{"label": "OnlyA", "description": "D"}}},
			},
		},
		"tool_use_id": "u_q_multi",
	})
	resMulti := fx.mustCallTool(t.Context(), 3, rawMulti)
	mustApproveEqual(t, resMulti.Behavior, "allow", "multi question allow")
	mustApproveDeepEqual(t, resMulti.UpdatedInput["answers"], map[string]string{
		"Q2?": "Opt-0",
		"Q3?": "Pure custom answer",
		"Q4?": "Fallback when choice id unknown",
	}, "multi question answers")
	mustApproveDeepEqual(t, resMulti.UpdatedInput["annotations"], map[string]map[string]string{
		"Q2?": {"notes": "With custom note"},
	}, "multi question annotations")

	fx.home.onDecision = nil
	fx.home.answer = `{"re":1,"err":{"code":"forbidden","msg":"not member"}}`
	resRef := fx.mustCallTool(t.Context(), 3, raw)
	mustApproveDeepEqual(t, resRef, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: forbidden: not member"}, "AskUserQuestion refused")
	fx.home.answer = ""
}

func scriptedClaudeChoice(step int) (*string, *string) {
	switch step {
	case 1:
		return new("Z"), nil
	case 2:
		return new("A"), new("With custom note")
	case 3:
		return nil, new("Pure custom answer")
	default:
		return new("UNKNOWN"), new("Fallback when choice id unknown")
	}
}

func TestCodexAskedListenerQuestionsConcurrencyAndCancellation(t *testing.T) {
	fx := newApproveFixture(t)
	runner := newApproveFakeRunner()
	prevRunner := Runner
	Runner = runner
	t.Cleanup(func() { Runner = prevRunner })

	codexKey, groupKey, reviewKey := seedCodexSessions(t, fx)
	assertCodexIgnoredEventsAndApprovalPath(t, fx, runner, codexKey, reviewKey)
	assertCodexQuestionsViaAskReply(t, fx, groupKey)
	assertCodexConcurrentWaitAndCancel(t, fx, runner, codexKey, groupKey)
}

func seedCodexSessions(t *testing.T, fx *approveFixture) (SessionKey, SessionKey, SessionKey) {
	t.Helper()
	codexKey := SessionKey{Group: fx.groupID, Member: fx.memberID, Task: string(approveTestID(50))}
	groupKey := SessionKey{Group: fx.groupID, Member: fx.memberID}
	reviewKey := SessionKey{Group: fx.groupID, Member: fx.memberID, Task: string(approveTestID(51))}
	for _, s := range []Session{
		{SessionKey: codexKey, Kind: TaskSession, Folder: "wt/50-codex", Agent: "codex", Home: fx.homeKey.ID(), Name: "Codex"},
		{SessionKey: groupKey, Kind: GroupSession, Folder: "repo", Agent: "codex", Home: fx.homeKey.ID(), Name: "Codex", Secretary: true},
		{SessionKey: reviewKey, Kind: ReviewSession, Folder: "review/51", Agent: "codex", Home: fx.homeKey.ID(), Name: "Codex"},
	} {
		mustApproveNoErr(t, fx.h.Sessions.Create(s))
	}
	return codexKey, groupKey, reviewKey
}

func assertCodexIgnoredEventsAndApprovalPath(t *testing.T, fx *approveFixture, runner *approveFakeRunner, codexKey, reviewKey SessionKey) {
	t.Helper()
	fx.h.Report(codexKey, Event{Kind: Active})
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: nil})
	fx.h.Report(SessionKey{Group: "none", Member: "none", Task: "none"}, Event{Kind: Asked, Ask: &Ask{ID: "x"}})
	mustApproveEqual(t, runner.answerCount(), 0, "ignored events answerCount")

	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(fx.owner1, post.Nonce, post.Body.Task, "approved")
		waited, err := fx.h.Hand(string(post.Nonce), raw)
		mustApproveTrue(t, waited && err == nil, "Codex approval Hand")
	}
	fx.h.Report(reviewKey, Event{Kind: Asked, Ask: &Ask{ID: "rev-ask", Tool: "commandExecution", Category: "command", Command: "go test"}})
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "rev-ask"), recordedAnswer{key: reviewKey, askID: "rev-ask", allow: true}, "review session Codex Ask approved")
	mustApproveEqual(t, recvApproveChan(t, runner.inCall, "rev-ask inCall"), 1, "Calls inside Answer")
	waitCallsZero(t, fx.h, reviewKey)
	fx.home.onPost = nil
}

func assertCodexQuestionsViaAskReply(t *testing.T, fx *approveFixture, groupKey SessionKey) {
	t.Helper()
	// Single reader: test goroutine awaiting Ask.Reply via recvApproveChan.
	replyCh := make(chan map[string][]string, 4)
	replyFn := func(ans map[string][]string) error {
		replyCh <- ans
		return nil
	}

	for _, bad := range []string{
		`{`,
		`{"questions":[]}`,
		`{"questions":[{"id":"","question":"Q?"}]}`,
		`{"questions":[{"id":"q1","question":""}]}`,
		`{"questions":[{"id":"q1","question":"Enter password","isSecret":true}]}`,
		`{"questions":[{"id":"q1","question":"ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD"}]}`,
	} {
		fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "bad-q", Input: json.RawMessage(bad), Reply: replyFn}})
		mustApproveTrue(t, recvApproveChan(t, replyCh, "invalid/secret Codex question") == nil, "expected nil Reply")
	}

	grpSess, _ := fx.h.Sessions.Get(groupKey)
	qStep := 0
	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		qStep++
		choice, note := scriptedCodexChoice(qStep)
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecisionItem(fx.owner2, grpSess, post.Nonce, choice, note))
		mustApproveTrue(t, waited && err == nil, "Codex decision Hand")
	}
	validInput := marshalWire(map[string]any{
		"questions": []map[string]any{
			{"id": "q1", "header": "H1", "question": "First?", "options": []map[string]string{{"label": "L1", "description": "D1"}}},
			{"id": "q2", "header": "", "question": "Second?", "options": []map[string]string{{"label": "L2", "description": "D2"}}},
			{"id": "q3", "header": "H3", "question": "Third?", "options": []map[string]string{{"label": "A3", "description": "D"}, {"label": "B3", "description": "D"}}},
			{"id": "q4", "header": "", "question": "Fourth?", "options": []map[string]string{}},
		},
	})
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "ok-q", Input: json.RawMessage(validInput), Reply: replyFn}})
	wantAns := map[string][]string{"q1": {"L1"}, "q2": {"Custom text"}, "q3": {"B3", "Note text"}, "q4": {}}
	mustApproveDeepEqual(t, recvApproveChan(t, replyCh, "Codex multi-question Reply"), wantAns, "Codex multi-question answers")

	fx.home.onDecision = nil
	fx.home.answer = `{"re":1,"err":{"code":"conflict","msg":"refused"}}`
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "ref-q", Input: json.RawMessage(validInput), Reply: replyFn}})
	mustApproveTrue(t, recvApproveChan(t, replyCh, "refused Codex question") == nil, "refused Codex question nil")
	fx.home.answer = ""

	// Single reader: test goroutine awaiting decision post signal via recvApproveChan.
	postedQ := make(chan struct{}, 1)
	fx.home.onDecision = func(proto.Post[proto.DecisionBody]) { postedQ <- struct{}{} }
	var calledReply atomic.Bool
	cancelReplyFn := func(map[string][]string) error {
		calledReply.Store(true)
		return nil
	}
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "cancel-q", Input: json.RawMessage(validInput), Reply: cancelReplyFn}})
	recvApproveChan(t, postedQ, "Codex question posted before cancel")
	fx.h.Report(groupKey, Event{Kind: TurnBegan})
	waitCallsZero(t, fx.h, groupKey)
	mustApproveEqual(t, calledReply.Load(), false, "cancelled Codex question must not call Reply")
	fx.home.onDecision = nil
}

func scriptedCodexChoice(step int) (*string, *string) {
	switch step {
	case 1:
		return new("A"), nil
	case 2:
		return nil, new("Custom text")
	case 3:
		return new("B"), new("Note text")
	default:
		return nil, nil
	}
}

func pickCardByCmd(cmd string, posts ...proto.Post[proto.ApprovalBody]) proto.ID {
	for _, p := range posts {
		if p.Body.Command == cmd {
			return p.Nonce
		}
	}
	return ""
}

func assertCodexConcurrentWaitAndCancel(t *testing.T, fx *approveFixture, runner *approveFakeRunner, codexKey, groupKey SessionKey) {
	t.Helper()
	// Single reader: test goroutine awaiting three Codex approval posts via recvApproveChan.
	cards := make(chan proto.Post[proto.ApprovalBody], 3)
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) { cards <- post }

	seamSoon(t, "Report for first Codex Ask", func() {
		fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "c1", Tool: "commandExecution", Category: "command", Command: "make a"}})
	})
	seamSoon(t, "Report for second Codex Ask", func() {
		fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "c2", Tool: "fileChange", Category: "file", Command: "add a.txt", Input: json.RawMessage(`[{"path":"a.txt"}]`)}})
	})
	seamSoon(t, "Report for groupKey Codex Ask", func() {
		fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "g1", Tool: "commandExecution", Category: "command", Command: "make g"}})
	})
	mustApproveEqual(t, fx.h.Sessions.Calls(codexKey), 2, "Calls right after two Reports on codexKey")
	mustApproveEqual(t, fx.h.Sessions.Calls(groupKey), 1, "Calls right after Report on groupKey")
	st := Part[approvalState](fx.h)
	st.mu.Lock()
	openCodexAsks := len(st.cancels[codexKey])
	st.mu.Unlock()
	mustApproveEqual(t, openCodexAsks, 2, "distinct cancel keys for concurrent asks on codexKey")

	postA := recvApproveChan(t, cards, "first card")
	postB := recvApproveChan(t, cards, "second card")
	postC := recvApproveChan(t, cards, "third card")
	cardC1 := pickCardByCmd("make a", postA, postB, postC)

	countBeforeC1 := runner.answerCount()
	waited, err := fx.h.Hand(string(cardC1), fx.buildDecidedItem(fx.owner1, cardC1, codexKey.Task, "approved"))
	mustApproveTrue(t, waited && err == nil, "c1 Hand")
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "c1 answer"), recordedAnswer{key: codexKey, askID: "c1", allow: true}, "c1 allow")
	recvApproveChan(t, runner.inCall, "c1 inCall")

	// TurnEnded cancels remaining codexKey ask without cancelling groupKey's ask.
	fx.h.Report(codexKey, Event{Kind: TurnEnded, Status: "interrupted"})
	waitCallsZero(t, fx.h, codexKey)
	mustApproveEqual(t, fx.h.Sessions.Calls(groupKey), 1, "groupKey ask still active")
	mustApproveEqual(t, runner.answerCount(), countBeforeC1+1, "cancelled Ask did not call Runner.Answer")

	fx.h.Report(groupKey, Event{Kind: TurnBegan})
	waitCallsZero(t, fx.h, groupKey)

	st.mu.Lock()
	leftover := len(st.cancels)
	st.mu.Unlock()
	mustApproveEqual(t, leftover, 0, "all cancel maps cleaned up")

	// Nil Runner does not panic when an Asked request finishes.
	Runner = nil
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		fx.h.Hand(string(post.Nonce), fx.buildDecidedItem(fx.owner1, post.Nonce, "", "denied"))
	}
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "nil-runner", Tool: "commandExecution", Category: "command", Command: "ls"}})
	waitCallsZero(t, fx.h, groupKey)
	fx.home.onPost = nil
}

// settledAskFixture is an approval fixture with a fake Runner and a recorder
// of the diagnostic lines, and the Asked event of the session's CLI.
func settledAskFixture(t *testing.T) (*approveFixture, *approveFakeRunner, *diagLines, Event) {
	t.Helper()
	d := captureDiag(t)
	fx, runner := newApproveFixture(t), newApproveFakeRunner()
	prevRunner := Runner
	Runner = runner
	t.Cleanup(func() { Runner = prevRunner })
	return fx, runner, d, Event{Kind: Asked, Ask: &Ask{ID: "a1", Tool: "commandExecution", Category: "command", Command: "make a"}}
}

func (fx *approveFixture) assertAskSettled(d *diagLines, runner *approveFakeRunner) {
	fx.t.Helper()
	node := "node=" + fx.h.Key.ID()[:8]
	d.ran(fx.t, 1, node, "ask g="+fx.groupID+" task="+fx.taskID)
	lines := d.all()
	mustApproveEqual(fx.t, lines[len(lines)-1], "settle.end "+node, "last line")
	mustApproveEqual(fx.t, runner.answerCount(), 0, "answers of the cancelled ask")
}

func TestSettleWaitsForAnAskWithTheHomeClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx, runner, d, asked := settledAskFixture(t)
		close(fx.home.done)
		release, settled := make(chan struct{}), make(chan struct{})
		go fx.db.Write(func(*store.Tx) error { <-release; return nil })
		synctest.Wait()
		fx.h.Report(fx.sessKey, asked)
		go func() {
			fx.h.Settle()
			close(settled)
		}()
		synctest.Wait()
		select {
		case <-settled:
			t.Error("Settle returned before the ask finished")
		default:
		}
		close(release)
		<-settled
		synctest.Wait()
		fx.assertAskSettled(d, runner)

		fx.h.Report(fx.sessKey, asked)
		synctest.Wait()
		fx.assertAskSettled(d, runner)
		mustApproveEqual(t, fx.approvalRows(), "", "approval rows")
	})
}

// heldHome holds each post until its context ends.
type heldHome struct {
	*approveFakeHome
	held chan struct{} // single reader: the test
}

func (f *heldHome) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if cmd, ok := build(1).(proto.Cmd); ok && isMemberStatusCmd(cmd.Env.Payload) {
		return f.approveFakeHome.Request(ctx, build)
	}
	close(f.held)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestSettleEndsAnAskWhoseCardIsBeingPosted(t *testing.T) {
	fx, runner, d, asked := settledAskFixture(t)
	home := &heldHome{fx.home, make(chan struct{})}
	fx.h.Attach(fx.homeKey.ID(), home)
	fx.h.Report(fx.sessKey, asked)
	recvApproveChan(t, home.held, "held post")
	seamSoon(t, "Settle", fx.h.Settle)
	fx.assertAskSettled(d, runner)
	mustApproveEqual(t, fx.approvalRows(), "", "approval rows")
}

// A CLI that asks on its connection and was started at level 1 is denied at
// once, without a card; at level 2 the request becomes a card.
func TestLevelOneRequestOfAConnectionIsDeniedWithoutACard(t *testing.T) {
	for level, want := range map[int]int{1: 0, 2: 1} {
		t.Run(strconv.Itoa(level), func(t *testing.T) {
			h, rh, drv, _ := setupRunEnv(t)
			Listeners["approve"] = listenApprove
			putSetting(t, h.Store, "default_level", strconv.Itoa(level))
			posts := func() int {
				return len(slices.DeleteFunc(rh.snapshotCmds(), func(c recordedCmd) bool { return c.payload["t"] != "post" }))
			}
			rh.cmdFn = func(p map[string]any) (json.RawMessage, error) {
				if p["t"] == "post" {
					return json.RawMessage(`{"re":1,"err":{"code":"conflict","msg":"refused"}}`), nil
				}
				return json.RawMessage(`{"re":1,"ok":true}`), nil
			}
			key := SessionKey{Group: runGroup1, Member: runAI1}
			mustApproveNoErr(t, h.Sessions.Create(makeRunSession(key, GroupSession, "repo")))
			mustApproveNoErr(t, Runner.Turn(t.Context(), h, key, "turn", Turn{Kind: ItemsTurn}))
			p := drv.proc(0)
			p.emit(Event{Kind: Asked, Ask: &Ask{ID: "a1", Tool: "commandExecution", Category: "command", Command: "make a"}})
			poll(t, 5*time.Second, "the answer", func() string {
				p.mu.Lock()
				defer p.mu.Unlock()
				if !slices.Equal(p.answers, []recordedAsk{{id: "a1", allow: false}}) {
					return fmt.Sprint(p.answers)
				}
				return ""
			})
			mustApproveEqual(t, posts(), want, "cards posted")
		})
	}
}
