package host

import (
	"bytes"
	"context"
	ed25519 "crypto/ed25519"
	b64 "encoding/base64"
	"encoding/json"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
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

func mustApproveErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
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
		t.Fatalf("timed out waiting for %s", label)
		return zero
	}
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
	acks        []proto.Ack
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

func (f *approveFakeHome) Send(frame any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ack, ok := frame.(proto.Ack); ok {
		f.acks = append(f.acks, ack)
	}
	return nil
}

func (f *approveFakeHome) Path() string                 { return "local" }
func (f *approveFakeHome) PathChanged() <-chan struct{} { return nil }
func (f *approveFakeHome) Done() <-chan struct{}        { return f.done }

func (f *approveFakeHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.rawCmds) + len(f.fetches) + 1)
	req := build(id)
	if fetch, ok := req.(proto.FetchRow); ok {
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
		return json.RawMessage(marshalApproveJSON(proto.Fetched{
			Type:    "fetched",
			Success: proto.Success{Re: id, OK: true},
			Rows:    rows,
			More:    false,
		})), nil
	}
	cmd := req.(proto.Cmd)
	f.rawCmds = append(f.rawCmds, cmd)
	var kindOnly struct {
		Kind string `json:"kind"`
	}
	json.Unmarshal([]byte(cmd.Env.Payload), &kindOnly)
	var appPost proto.Post[proto.ApprovalBody]
	hasApp := kindOnly.Kind == "approval" && proto.Decode([]byte(cmd.Env.Payload), &appPost) == nil
	if hasApp {
		f.posts = append(f.posts, appPost)
	}
	var decPost proto.Post[proto.DecisionBody]
	hasDec := kindOnly.Kind == "decision" && proto.Decode([]byte(cmd.Env.Payload), &decPost) == nil
	if hasDec {
		f.decisions = append(f.decisions, decPost)
	}
	ans, reqErr, appHook, decHook := f.answer, f.err, f.onPost, f.onDecision
	f.mu.Unlock()
	if reqErr != nil {
		return nil, reqErr
	}
	if ans != "" {
		return json.RawMessage(ans), nil
	}
	if hasApp && appHook != nil {
		appHook(appPost)
	}
	if hasDec && decHook != nil {
		decHook(decPost)
	}
	return json.RawMessage(`{"re":1,"ok":true}`), nil
}

func (f *approveFakeHome) ackList() []proto.Ack {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.acks)
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
	f.cards[card.ID] = json.RawMessage(marshalApproveJSON(card))
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
	seamSQL(t, fx.db, "INSERT INTO setting(key, value) VALUES('owner', '"+marshalApproveJSON(fx.owner1.ID())+"')")
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
		Name:      "SoloDawn",
		Kind:      "project",
		Base:      "main",
		Forge:     "none",
		Home:      fx.homeKey.ID(),
		Checks:    []string{},
		Limits:    proto.Limits{ApprovalWaitS: 20},
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
		CLI:       "claude",
		Host:      fx.hostKey.ID(),
		Account:   "personal",
		Allow:     []string{},
		Approvers: []string{fx.owner1.ID()},
		State:     "working",
	}
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalApproveJSON(grp)))
	fx.h.Rows.Put(fx.groupID, "member", fx.human1ID, json.RawMessage(marshalApproveJSON(h1)))
	fx.h.Rows.Put(fx.groupID, "member", fx.human2ID, json.RawMessage(marshalApproveJSON(h2)))
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalApproveJSON(ai)))
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
		Pos:       1,
		Group:     fx.groupID,
		Thread:    thread,
		Main:      true,
		By:        fx.memberID,
		Kind:      kind,
		Timestamp: seamStart,
		Text:      "",
		Mentions:  []string{},
		Re:        nil,
		Body:      json.RawMessage(`{}`),
		State:     state,
	}
}

func (fx *approveFixture) buildDecidedItem(actSigner identity.Key, card proto.ID, status string) json.RawMessage {
	fx.itemSeq++
	actEnv := actSigner.Sign(`{"t":"act"}`)
	by := fx.human1ID
	at := int64(seamStart)
	msg := fx.makeCardMsg(card, "approval", fx.taskID, &proto.CardState{
		Status: status,
		By:     &by,
		At:     &at,
		Act:    &actEnv,
	})
	item := proto.Item{
		Type:      "item",
		Nonce:     approveTestID(byte(110 + fx.itemSeq%40)),
		Timestamp: seamStart,
		Group:     proto.ID(fx.groupID),
		To:        fx.hostKey.ID(),
		Item:      fx.itemSeq,
		AI:        proto.ID(fx.memberID),
		Why:       "decided",
		Task:      proto.ID(fx.taskID),
		Msg:       card,
	}
	env := fx.homeKey.Sign(marshalApproveJSON(item))
	return json.RawMessage(marshalApproveJSON(proto.InboxItem{
		Env: env,
		Msg: json.RawMessage(marshalApproveJSON(msg)),
	}))
}

func (fx *approveFixture) buildDecisionItem(actSigner identity.Key, s Session, card proto.ID, choice, text *string) json.RawMessage {
	fx.itemSeq++
	actEnv := actSigner.Sign(`{"t":"act"}`)
	by := fx.human1ID
	at := int64(seamStart)
	msg := fx.makeCardMsg(card, "decision", s.Task, &proto.CardState{
		Status: "decided",
		By:     &by,
		At:     &at,
		Choice: choice,
		Text:   text,
		Act:    &actEnv,
	})
	var itemTask proto.ID
	if s.Task != "" {
		itemTask = proto.ID(s.Task)
	}
	item := proto.Item{
		Type:      "item",
		Nonce:     approveTestID(byte(60 + fx.itemSeq%40)),
		Timestamp: seamStart,
		Group:     proto.ID(s.Group),
		To:        fx.hostKey.ID(),
		Item:      fx.itemSeq,
		AI:        proto.ID(s.Member),
		Why:       "decided",
		Task:      itemTask,
		Msg:       card,
	}
	env := fx.homeKey.Sign(marshalApproveJSON(item))
	return json.RawMessage(marshalApproveJSON(proto.InboxItem{
		Env: env,
		Msg: json.RawMessage(marshalApproveJSON(msg)),
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

func setFastWaitUnit(t *testing.T, unit time.Duration) {
	t.Helper()
	prev := approvalWaitUnit
	approvalWaitUnit = unit
	t.Cleanup(func() { approvalWaitUnit = prev })
}

func TestApproveToolRegistrationSchemaAndVisibility(t *testing.T) {
	tool, ok := Tools["approve"]
	mustApproveTrue(t, ok, "Tools[approve] missing")
	mustApproveTrue(t, tool.Description != "", "Tools[approve] description empty")
	mustApproveTrue(t, tool.Call != nil, "Tools[approve] Call nil")
	mustApproveTrue(t, tool.For != nil, "Tools[approve] For nil")
	mustApproveTrue(t, Listeners["approve"] != nil, "Listeners[approve] nil")

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
	mustApproveEqual(t, schema.AdditionalProperties, false, "schema.AdditionalProperties")
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
		`{"input":{},"tool_use_id":"u1"}`,
		`{"tool_name":null,"input":{},"tool_use_id":"u1"}`,
		`{"tool_name":"","input":{},"tool_use_id":"u1"}`,
		`{"tool_name":"Bash","input":{}}`,
		`{"tool_name":"Bash","input":{},"tool_use_id":null}`,
		`{"tool_name":"Bash","tool_use_id":"u1"}`,
		`{"tool_name":"Bash","input":null,"tool_use_id":"u1"}`,
		`{"tool_name":"Bash","input":[],"tool_use_id":"u1"}`,
		`{"tool_name":"Bash","input":"x","tool_use_id":"u1"}`,
		`{"tool_name":"Bash","input":{},"tool_use_id":"u1","extra":true}`,
		`{"tool_name":"Bash","input":{"command":123},"tool_use_id":"u1"}`,
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
		raw := fx.buildDecidedItem(fx.owner2, post.Nonce, "approved")
		_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
		mustApproveNoErr(t, err)
	}
	cases := []categoryMapCase{
		{raw: `{"tool_name":"Bash","input":{"command":"git push origin other","description":"Push feature"},"tool_use_id":"u1"}`, tool: "Bash", category: "command", command: "git push origin other", why: new("Push feature"), level: 3},
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
		mustApproveEqual(t, marshalApproveJSON(res), `{"behavior":"allow"}`, tc.tool)
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
	mustApproveTrue(t, got.Thread != nil, "post.Thread must be set")
	mustApproveEqual(t, string(*got.Thread), fx.taskID, "post.Thread")
	mustApproveEqual(t, string(got.As), fx.memberID, "post.As")
	mustApproveEqual(t, got.Body.Tool, tc.tool, "Body.Tool")
	mustApproveEqual(t, got.Body.Category, tc.category, "Body.Category")
	mustApproveEqual(t, got.Body.Command, tc.command, "Body.Command")
	mustApproveTrue(t, got.Body.Full == nil, "Body.Full must be nil")
	mustApproveDeepEqual(t, got.Body.Why, tc.why, "Body.Why")
}

func TestApproveLevelOneAndOutboundSecretCheck(t *testing.T) {
	fx := newApproveFixture(t)
	res1 := fx.mustCallTool(t.Context(), 1, `{"tool_name":"Bash","input":{"command":"touch x"},"tool_use_id":"u1"}`)
	mustApproveEqual(t, marshalApproveJSON(res1), `{"behavior":"deny","message":"Denied: this session is read only."}`, "level 1")
	mustApproveEqual(t, len(fx.home.postList()), 0, "level 1 posts")
	mustApproveEqual(t, fx.approvalRows(), "", "level 1 rows")
	mustApproveEqual(t, seamRecord(t, fx.h), "", "level 1 audit")

	secretTok := "ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD"
	longWithSecret := strings.Repeat("a", 4000) + " " + secretTok
	argCmdSecret := marshalApproveJSON(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": longWithSecret, "description": "Safe justification"},
		"tool_use_id": "u3",
	})
	resSec1 := fx.mustCallTool(t.Context(), 3, argCmdSecret)
	mustApproveDeepEqual(t, resSec1, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "command secret")
	mustApproveTrue(t, !strings.Contains(marshalApproveJSON(resSec1), secretTok), "deny leaked secret")

	argWhySecret := marshalApproveJSON(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": "echo ok", "description": "Using -----BEGIN PRIVATE KEY-----"},
		"tool_use_id": "u4",
	})
	resSec2 := fx.mustCallTool(t.Context(), 2, argWhySecret)
	mustApproveDeepEqual(t, resSec2, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "why secret")
	mustApproveEqual(t, len(fx.home.postList()), 0, "secret posts")
	mustApproveEqual(t, fx.approvalRows(), "", "secret rows")

	audit := seamRecord(t, fx.h)
	mustApproveTrue(t, !strings.Contains(audit, secretTok), "audit leaked github token")
	mustApproveTrue(t, !strings.Contains(audit, "PRIVATE KEY"), "audit leaked private key")
	wantAudit := strings.Join([]string{
		"1763600001000|" + fx.groupID + "|claude|approval|-|-|Bash|-|secret",
		"1763600002000|" + fx.groupID + "|claude|approval|-|-|Bash|-|secret",
	}, "\n")
	mustApproveEqual(t, audit, wantAudit, "secret audit rows")
}

func jsonEncodedLen(s string) int {
	return len(marshalApproveJSON(s))
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
	mustApproveTrue(t, fullOver != nil, "over 3072 full set")
	mustApproveEqual(t, *fullOver, 3071, "over 3072 full value")
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
	mustApproveTrue(t, fullUTF8 != nil, "utf8 full set")
	mustApproveEqual(t, *fullUTF8, 3072, "utf8 full bytes")
	mustApproveEqual(t, jsonEncodedLen(shortUTF8), 3071, "utf8 jsonLen")

	nlExact := strings.Repeat("\n", 1535)
	nlShort, nlFull := shortenCommand(nlExact)
	mustApproveEqual(t, nlShort, nlExact, "newline exact")
	mustApproveTrue(t, nlFull == nil, "newline exact full nil")

	nlOver := strings.Repeat("\n", 1536)
	nlOverShort, nlOverFull := shortenCommand(nlOver)
	mustApproveEqual(t, len(nlOverShort), 1535, "newline over cut")
	mustApproveTrue(t, nlOverFull != nil, "newline over full set")
	mustApproveEqual(t, *nlOverFull, 1536, "newline over full len")

	mustApproveTrue(t, optionalWhy("") == nil, "empty why nil")
	whyExact := strings.Repeat("w", 254)
	gotWhyExact := optionalWhy(whyExact)
	mustApproveTrue(t, gotWhyExact != nil, "why exact non-nil")
	mustApproveEqual(t, *gotWhyExact, whyExact, "why exact val")
	mustApproveEqual(t, jsonEncodedLen(*gotWhyExact), 256, "why exact jsonLen")

	gotWhyOver := optionalWhy(strings.Repeat("w", 255))
	mustApproveTrue(t, gotWhyOver != nil, "why over non-nil")
	mustApproveEqual(t, *gotWhyOver, whyExact, "why over val")
	mustApproveEqual(t, jsonEncodedLen(*gotWhyOver), 256, "why over jsonLen")
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
	fx.h.Rows.Put(fx.groupID, "member", human3ID, json.RawMessage(marshalApproveJSON(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: human3ID, Seq: 5}, Group: fx.groupID, Kind: "human", User: otherUser.ID(), Name: "Carol"},
		Role:      "member",
	})))
	fx.h.Rows.Put(fx.groupID, "member", "bad-member", json.RawMessage(`{"r":"member"`))
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalApproveJSON(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 6}, Group: fx.groupID, Kind: "ai", User: fx.owner1.ID(), Name: "Claude Worker"},
		CLI:       "claude",
		Host:      fx.hostKey.ID(),
		Account:   "personal",
		Allow:     []string{},
		Approvers: []string{fx.owner1.ID(), fx.delegate.ID()},
		State:     "working",
	})))
	return approveTestKey(8)
}

func assertTimelyDelegateApprove(t *testing.T, fx *approveFixture, delegateDev2 identity.Key) {
	t.Helper()
	// Approve inside Home.Request so the test proves Await opened before Command.
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(delegateDev2, post.Nonce, "approved")
		status, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
		mustApproveNoErr(t, err)
		mustApproveEqual(t, status, "approved", "timely delegate status")
	}
	resAllow := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"npm test","description":"Run suite"},"tool_use_id":"u1"}`)
	mustApproveDeepEqual(t, resAllow, ApprovalResult{Behavior: "allow"}, "timely allow")
	post := fx.home.postList()[0]
	wantMentions := []proto.ID{proto.ID(fx.human1ID), proto.ID(fx.human2ID)}
	slices.Sort(wantMentions)
	mustApproveDeepEqual(t, post.Mentions, wantMentions, "post.Mentions")
	mustApproveDeepEqual(t, post.Body.Approvers, []string{fx.owner1.ID(), fx.delegate.ID()}, "post.Body.Approvers")
	mustApproveEqual(t, fx.approvalRows(), "", "timely allow rows")
	mustApproveEqual(t, len(fx.home.ackList()), 1, "timely allow acks")
}

func assertTimelyOwnerDenyAndRefusals(t *testing.T, fx *approveFixture) {
	t.Helper()
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(fx.owner2, post.Nonce, "denied")
		status, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
		mustApproveNoErr(t, err)
		mustApproveEqual(t, status, "denied", "timely deny status")
	}
	resDeny := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"rm -rf /tmp/x"},"tool_use_id":"u2"}`)
	mustApproveEqual(t, marshalApproveJSON(resDeny), `{"behavior":"deny","message":"Denied by the owner."}`, "timely deny")
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
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalApproveJSON(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 9}, Group: fx.groupID, Kind: "human", User: fx.owner1.ID(), Name: "Human"},
		Role:      "owner",
	})))
	mustApproveTrue(t, memberApprovers(fx.h.Rows, fx.groupID, fx.memberID) == nil, "human member row nil")
	fx.h.Rows.Put(fx.groupID, "member", fx.memberID, json.RawMessage(marshalApproveJSON(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: fx.memberID, Seq: 10}, Group: fx.groupID, Kind: "ai", User: fx.owner1.ID(), Name: "AI"},
		CLI:       "claude", Host: fx.hostKey.ID(), Account: "personal", Allow: []string{}, Approvers: []string{}, State: "working",
	})))
	mustApproveTrue(t, memberApprovers(fx.h.Rows, fx.groupID, fx.memberID) == nil, "empty ai approvers nil")
}

func TestTimeoutLateDecisionFetchFallbackAndOneTimeGrant(t *testing.T) {
	fx := newApproveFixture(t)
	setFastWaitUnit(t, time.Millisecond)
	grpOne := assertWaitDurationAndSelectBranches(t, fx)
	assertLateApproveOneTimeGrantAndCleanup(t, fx, grpOne)
	assertFetchPendingApprovalFallback(t, fx)
}

func assertWaitDurationAndSelectBranches(t *testing.T, fx *approveFixture) proto.GroupRow {
	t.Helper()
	fx.h.Rows.Reset(fx.groupID)
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 600*time.Millisecond, "missing group wait")
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(`{"r":"group"`))
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 600*time.Millisecond, "corrupt group wait")

	fx.seedDefaultRows()
	grpZero := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: fx.groupID, Seq: 10},
		Name:      "SoloDawn", Kind: "project", Base: "main", Forge: "none", Home: fx.homeKey.ID(), Checks: []string{},
		Limits: proto.Limits{ApprovalWaitS: 0},
	}
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalApproveJSON(grpZero)))
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 600*time.Millisecond, "zero ApprovalWaitS")

	grpOne := grpZero
	grpOne.Seq = 11
	grpOne.Limits.ApprovalWaitS = 1
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalApproveJSON(grpOne)))
	mustApproveEqual(t, approvalWaitDuration(fx.h, fx.groupID), 1*time.Millisecond, "ApprovalWaitS=1")

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

func assertLateApproveOneTimeGrantAndCleanup(t *testing.T, fx *approveFixture, grpOne proto.GroupRow) {
	t.Helper()
	resTimeout := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_t1"}`)
	mustApproveDeepEqual(t, resTimeout, ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "timeout deny")
	card1 := fx.home.postList()[0].Nonce
	mustApproveTrue(t, strings.HasSuffix(fx.approvalRows(), "|Write|pending"), "pending row after timeout")

	restarted, _, err := New(fx.db, fx.hostKey, approveTestClock())
	mustApproveNoErr(t, err)
	fx.h = restarted
	// Create a non-matching session first so sessionRowAgent iterates past a non-matching entry.
	mustApproveNoErr(t, fx.h.Sessions.Create(Session{
		SessionKey: SessionKey{Group: "other-g", Member: fx.memberID, Task: "t"},
		Kind:       TaskSession, Folder: "wt/1-x", Agent: "codex", Home: fx.homeKey.ID(), Name: "Other",
	}))
	fx.h.Attach(fx.homeKey.ID(), fx.home)

	decidedRaw := fx.buildDecidedItem(fx.owner2, card1, "approved")
	st1, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), decidedRaw)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, st1, "approved", "late approve status")
	mustApproveTrue(t, strings.HasSuffix(fx.approvalRows(), "|Write|granted"), "granted row after late approve")

	ackCountBefore := len(fx.home.ackList())
	stDup, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), decidedRaw)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, stDup, "", "duplicate late approve status")
	mustApproveEqual(t, len(fx.home.ackList()), ackCountBefore+1, "duplicate ack count")

	fx.seedDefaultRows()
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalApproveJSON(grpOne)))
	resDiffTool := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Edit","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_diff_tool"}`)
	resDiffInput := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"changed"},"tool_use_id":"u_diff_in"}`)
	mustApproveEqual(t, resDiffTool.Behavior, "deny", "different tool must not match grant")
	mustApproveEqual(t, resDiffInput.Behavior, "deny", "different input must not match grant")
	mustApproveEqual(t, len(fx.home.postList()), 3, "posts after non-matching calls")

	postsBefore := len(fx.home.postList())
	resRetry := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{ "content": "<v1>&v2", "file_path": "/ext/a.txt" },"tool_use_id":"u_retry_ok"}`)
	mustApproveDeepEqual(t, resRetry, ApprovalResult{Behavior: "allow"}, "granted retry")
	mustApproveEqual(t, len(fx.home.postList()), postsBefore, "granted retry did not post")

	resSecondTry := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_retry_2"}`)
	mustApproveEqual(t, resSecondTry.Behavior, "deny", "second retry asks again")
	mustApproveEqual(t, len(fx.home.postList()), postsBefore+1, "second retry posted new card")

	latestCard := fx.home.postList()[len(fx.home.postList())-1].Nonce
	stDeny, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), fx.buildDecidedItem(fx.owner1, latestCard, "denied"))
	mustApproveNoErr(t, err)
	mustApproveEqual(t, stDeny, "denied", "late deny status")

	otherTask := string(approveTestID(99))
	seamSQL(t, fx.db, "INSERT INTO approval(card, g, member, task, tool, input, state) VALUES('other-card', '"+fx.groupID+"', '"+fx.memberID+"', '"+otherTask+"', 'Bash', 'abc', 'granted')")
	mustApproveNoErr(t, fx.h.DeleteTaskApprovals(fx.groupID, fx.taskID))
	mustApproveEqual(t, fx.approvalRows(), "other-card|"+fx.groupID+"|"+fx.memberID+"|"+otherTask+"|Bash|granted", "rows after DeleteTaskApprovals")
}

func assertFetchPendingApprovalFallback(t *testing.T, fx *approveFixture) {
	t.Helper()
	mustApproveNoErr(t, fx.h.DeleteTaskApprovals(fx.groupID, string(approveTestID(99))))
	s, ok := fx.h.Sessions.Get(fx.sessKey)
	mustApproveTrue(t, ok, "session must exist")
	inputJSON := json.RawMessage(`{"command":"npm publish"}`)
	digest := hashApprovalInput(inputJSON)

	cards := []proto.ID{approveTestID(61), approveTestID(62), approveTestID(63), approveTestID(64), approveTestID(65)}
	for _, c := range cards {
		mustApproveNoErr(t, storePendingApproval(fx.h, c, s, "Bash", digest))
	}

	// Disconnected home returns false without deleting pending rows.
	discHome := newApproveFakeHome()
	close(discHome.done)
	fx.h.Attach(fx.homeKey.ID(), discHome)
	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "npm publish"), false, "disconnected home fetch")
	fx.h.Attach(fx.homeKey.ID(), fx.home)

	// Network error and malformed Fetched / MsgRow paths leave pending rows intact.
	fx.home.err = io.EOF
	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "npm publish"), false, "fetch transport err")
	fx.home.err = nil

	for _, badAns := range []string{
		`{"re":1,"err":{"code":"not_found","msg":"missing"}}`,
		`{"t":"fetched","re":1,"ok":true,"rows":[],"more":false}`,
		`{"t":"fetched","re":1,"ok":true,"rows":[{}],"more":false}`,
		marshalApproveJSON(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 1, OK: true}, Rows: []json.RawMessage{json.RawMessage(marshalApproveJSON(fx.makeCardMsg(cards[0], "text", fx.taskID, nil)))}}),
		marshalApproveJSON(proto.Fetched{Type: "fetched", Success: proto.Success{Re: 1, OK: true}, Rows: []json.RawMessage{json.RawMessage(marshalApproveJSON(fx.makeCardMsg(cards[0], "approval", fx.taskID, nil)))}}),
	} {
		ansCopy := badAns
		fx.home.onFetchHook = func(proto.FetchRow) (json.RawMessage, error) {
			return json.RawMessage(ansCopy), nil
		}
		mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "npm publish"), false, "bad fetch payload")
	}
	fx.home.onFetchHook = nil

	// Stale statuses (denied, expired, reassigned) delete their rows and continue; pending stays; approved consumes and stops.
	actEnv := fx.owner2.Sign(`{"t":"act"}`)
	fx.home.putCard(fx.makeCardMsg(cards[0], "approval", fx.taskID, &proto.CardState{Status: "denied"}))
	fx.home.putCard(fx.makeCardMsg(cards[1], "approval", fx.taskID, &proto.CardState{Status: "expired"}))
	fx.home.putCard(fx.makeCardMsg(cards[2], "approval", fx.taskID, &proto.CardState{Status: "reassigned"}))
	fx.home.putCard(fx.makeCardMsg(cards[3], "approval", fx.taskID, &proto.CardState{Status: "pending"}))
	fx.home.putCard(fx.makeCardMsg(cards[4], "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actEnv}))

	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "npm publish"), true, "fetch consumes approved card")
	mustApproveEqual(t, fx.approvalRows(), string(cards[3])+"|"+fx.groupID+"|"+fx.memberID+"|"+fx.taskID+"|Bash|pending", "only still-pending card remains")
	fetches := fx.home.fetchList()
	lastFetch := fetches[len(fetches)-1]
	mustApproveEqual(t, lastFetch.Type, "fetch", "fetch.Type")
	mustApproveEqual(t, lastFetch.Group, fx.groupID, "fetch.Group")
	mustApproveEqual(t, lastFetch.Before, int64(0), "fetch.Before")
	mustApproveEqual(t, lastFetch.Limit, 1, "fetch.Limit")
	mustApproveEqual(t, lastFetch.Row, "msg", "fetch.Row")
	mustApproveEqual(t, lastFetch.RowID, string(cards[4]), "fetch.RowID")

	// Concurrent deletion between fetch and consumePendingCard yields false without an extra audit row.
	fx.home.putCard(fx.makeCardMsg(cards[3], "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actEnv}))
	fx.home.onFetchHook = func(req proto.FetchRow) (json.RawMessage, error) {
		deleteApprovalCard(fx.h, req.RowID)
		row := fx.home.cards[req.RowID]
		return json.RawMessage(marshalApproveJSON(proto.Fetched{
			Type:    "fetched",
			Success: proto.Success{Re: 1, OK: true},
			Rows:    []json.RawMessage{row},
		})), nil
	}
	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "npm publish"), false, "concurrent delete during fetch")
	fx.home.onFetchHook = nil
}

func TestClaudeAskUserQuestionDecisionCardsAndAnswers(t *testing.T) {
	fx := newApproveFixture(t)
	setFastWaitUnit(t, time.Millisecond)

	assertClaudeQuestionSecretChecks(t, fx)
	assertClaudeQuestionOptionIDsAndAnswers(t, fx)
	assertClaudeQuestionGroupSessionAndFailures(t, fx)
}

func assertClaudeQuestionSecretChecks(t *testing.T, fx *approveFixture) {
	t.Helper()
	secretTok := "ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD"
	secretInputs := []map[string]any{
		{"questions": []map[string]any{{"question": "Leak " + secretTok, "header": "H", "options": []map[string]string{{"label": "A", "description": "B"}}}}},
		{"questions": []map[string]any{{"question": "Clean", "header": secretTok, "options": []map[string]string{{"label": "A", "description": "B"}}}}},
		{"questions": []map[string]any{{"question": "Clean", "header": "H", "options": []map[string]string{{"label": secretTok, "description": "B"}}}}},
		{"questions": []map[string]any{{"question": "Clean", "header": "H", "options": []map[string]string{{"label": "A", "description": secretTok}}}}},
	}
	for _, in := range secretInputs {
		raw := marshalApproveJSON(map[string]any{"tool_name": "AskUserQuestion", "input": in, "tool_use_id": "u_sec"})
		res := fx.mustCallTool(t.Context(), 3, raw)
		mustApproveDeepEqual(t, res, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "AskUserQuestion secret")
	}
	mustApproveEqual(t, len(fx.home.decisionList()), 0, "no decision cards posted on secret")
}

func assertClaudeQuestionOptionIDsAndAnswers(t *testing.T, fx *approveFixture) {
	t.Helper()
	// Build 27 options so optionID pins 0 ("A"), 25 ("Z"), and 26 ("27").
	opts27 := make([]map[string]string, 27)
	for i := range opts27 {
		opts27[i] = map[string]string{"label": "Opt-" + strconv.Itoa(i), "description": "Desc-" + strconv.Itoa(i)}
	}
	s, _ := fx.h.Sessions.Get(fx.sessKey)
	step := 0
	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		step++
		var choice, note *string
		switch step {
		case 1:
			c := "Z"
			choice = &c
		case 2:
			c := "27"
			n := "With custom note"
			choice, note = &c, &n
		case 3:
			n := "Pure custom answer"
			note = &n
		case 4:
			c := "UNKNOWN"
			n := "Fallback when choice id unknown"
			choice, note = &c, &n
		}
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecisionItem(fx.owner2, s, post.Nonce, choice, note))
		mustApproveEqual(t, waited, true, "decision Hand waited")
		mustApproveNoErr(t, err)
	}

	raw := marshalApproveJSON(map[string]any{
		"tool_name": "AskUserQuestion",
		"input": map[string]any{
			"extraKeep": "kept",
			"questions": []map[string]any{
				{"question": "Q1?", "header": "H1", "options": opts27, "multiSelect": false},
			},
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
	mustApproveEqual(t, len(decCard.Body.Options), 27, "27 options count")
	mustApproveEqual(t, decCard.Body.Options[0].ID, "A", "option 0 ID")
	mustApproveEqual(t, decCard.Body.Options[25].ID, "Z", "option 25 ID")
	mustApproveEqual(t, decCard.Body.Options[26].ID, "27", "option 26 ID")
	mustApproveTrue(t, decCard.Thread != nil && string(*decCard.Thread) == fx.taskID, "task session decision Thread")
	mustApproveTrue(t, decCard.Body.Task != nil && *decCard.Body.Task == fx.taskID, "task session decision Body.Task")
	mustApproveTrue(t, decCard.Body.Context != nil && *decCard.Body.Context == "H1", "decision Context")

	rawMulti := marshalApproveJSON(map[string]any{
		"tool_name": "AskUserQuestion",
		"input": map[string]any{
			"questions": []map[string]any{
				{"question": "Q2?", "header": "", "options": opts27},
				{"question": "Q3?", "header": "", "options": []map[string]string{}},
				{"question": "Q4?", "header": "", "options": []map[string]string{{"label": "OnlyA", "description": "D"}}},
			},
		},
		"tool_use_id": "u_q_multi",
	})
	resMulti := fx.mustCallTool(t.Context(), 3, rawMulti)
	mustApproveEqual(t, resMulti.Behavior, "allow", "multi question allow")
	mustApproveDeepEqual(t, resMulti.UpdatedInput["answers"], map[string]string{
		"Q2?": "Opt-26",
		"Q3?": "Pure custom answer",
		"Q4?": "Fallback when choice id unknown",
	}, "multi question answers")
	mustApproveDeepEqual(t, resMulti.UpdatedInput["annotations"], map[string]map[string]string{
		"Q2?": {"notes": "With custom note"},
	}, "multi question annotations")
}

func assertClaudeQuestionGroupSessionAndFailures(t *testing.T, fx *approveFixture) {
	t.Helper()
	grpKey := SessionKey{Group: fx.groupID, Member: fx.memberID}
	mustApproveNoErr(t, fx.h.Sessions.Create(Session{
		SessionKey: grpKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: fx.homeKey.ID(), Name: "Sec", Secretary: true,
	}))
	grpSess, _ := fx.h.Sessions.Get(grpKey)

	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		mustApproveTrue(t, post.Thread == nil, "group session decision Thread nil")
		mustApproveTrue(t, post.Body.Task == nil, "group session decision Task nil")
		mustApproveTrue(t, post.Body.Context == nil, "empty header Context nil")
		mustApproveDeepEqual(t, post.Body.Options, []proto.DecisionOption{}, "empty options non-nil")
		txt := "Free answer"
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecisionItem(fx.owner1, grpSess, post.Nonce, nil, &txt))
		mustApproveTrue(t, waited && err == nil, "group decision Hand")
	}
	rawGrp := json.RawMessage(`{"tool_name":"AskUserQuestion","input":{"questions":[{"question":"Main question?"}]},"tool_use_id":"u_grp"}`)
	valGrp, err := Tools["approve"].Call(t.Context(), fx.h, Caller{Session: grpSess, Level: 3}, rawGrp)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, valGrp.(ApprovalResult).Behavior, "allow", "group session AskUserQuestion")

	// Timeout on decision wait.
	fx.home.onDecision = nil
	grpFast := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: fx.groupID, Seq: 30},
		Name:      "SoloDawn", Kind: "project", Base: "main", Forge: "none", Home: fx.homeKey.ID(), Checks: []string{},
		Limits: proto.Limits{ApprovalWaitS: 1},
	}
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalApproveJSON(grpFast)))
	resTimeout := fx.mustCallTool(t.Context(), 3, string(rawGrp))
	mustApproveDeepEqual(t, resTimeout, ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "AskUserQuestion timeout")

	// Home refusal on decision post.
	fx.home.answer = `{"re":1,"err":{"code":"forbidden","msg":"not member"}}`
	resRef := fx.mustCallTool(t.Context(), 3, string(rawGrp))
	mustApproveDeepEqual(t, resRef, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: forbidden: not member"}, "AskUserQuestion refused")
	fx.home.answer = ""
}

func TestDecisionDecodingValidationAndDeciderPrecedence(t *testing.T) {
	fx := newApproveFixture(t)
	assertCardDeciderAndPredicates(t, fx)
	assertOpenWaitAndRecordDecisionChecks(t, fx)
}

func assertCardDeciderAndPredicates(t *testing.T, fx *approveFixture) {
	t.Helper()
	cardID := approveTestID(70)
	actWithKey := proto.Envelope{Payload: `{}`, Key: "dev-act", Signature: "s"}
	actEmptyKey := proto.Envelope{Payload: `{}`, Key: "", Signature: "s"}
	byNonEmpty := "member-by"
	byEmpty := ""

	c1 := fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actWithKey, By: &byNonEmpty})
	mustApproveEqual(t, cardDecider(c1, "fb"), "dev-act", "Act.Key wins")

	c2 := fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved", Act: &actEmptyKey, By: &byNonEmpty})
	mustApproveEqual(t, cardDecider(c2, "fb"), "member-by", "empty Act.Key falls through to By")

	c3 := fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved", By: &byNonEmpty})
	mustApproveEqual(t, cardDecider(c3, "fb"), "member-by", "nil Act uses By")

	c4 := fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved", By: &byEmpty})
	mustApproveEqual(t, cardDecider(c4, "fb"), "fb", "empty By uses fallback")

	c5 := fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved"})
	mustApproveEqual(t, cardDecider(c5, "fb"), "fb", "nil By uses fallback")

	// Pin isDecidedItem exact boundaries including Item == 0 vs Item == 1.
	validItem := proto.Item{Type: "item", Item: 1, Why: "decided", Msg: cardID}
	mustApproveTrue(t, isDecidedItem(validItem), "Item=1 valid")
	zeroItem := validItem
	zeroItem.Item = 0
	mustApproveTrue(t, !isDecidedItem(zeroItem), "Item=0 invalid")
	wrongType := validItem
	wrongType.Type = "other"
	mustApproveTrue(t, !isDecidedItem(wrongType), "Type!=item invalid")
	wrongWhy := validItem
	wrongWhy.Why = "mention"
	mustApproveTrue(t, !isDecidedItem(wrongWhy), "Why!=decided invalid")
	emptyMsg := validItem
	emptyMsg.Msg = ""
	mustApproveTrue(t, !isDecidedItem(emptyMsg), "Msg empty invalid")

	// Pin isApprovalDecided and isDecisionDecided operands.
	mustApproveTrue(t, isApprovalDecided(fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved"})), "approval approved")
	mustApproveTrue(t, isApprovalDecided(fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "denied"})), "approval denied")
	mustApproveTrue(t, !isApprovalDecided(fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "pending"})), "approval pending")
	mustApproveTrue(t, !isApprovalDecided(fx.makeCardMsg(cardID, "approval", fx.taskID, nil)), "approval nil state")
	mustApproveTrue(t, !isApprovalDecided(fx.makeCardMsg(cardID, "decision", fx.taskID, &proto.CardState{Status: "approved"})), "wrong kind approval")

	mustApproveTrue(t, isDecisionDecided(fx.makeCardMsg(cardID, "decision", fx.taskID, &proto.CardState{Status: "decided"})), "decision decided")
	mustApproveTrue(t, !isDecisionDecided(fx.makeCardMsg(cardID, "decision", fx.taskID, &proto.CardState{Status: "pending"})), "decision pending")
	mustApproveTrue(t, !isDecisionDecided(fx.makeCardMsg(cardID, "decision", fx.taskID, nil)), "decision nil state")
	mustApproveTrue(t, !isDecisionDecided(fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "decided"})), "wrong kind decision")
}

func assertOpenWaitAndRecordDecisionChecks(t *testing.T, fx *approveFixture) {
	t.Helper()
	for _, bad := range []string{
		`{`,
		`{"env":{"p":"{","k":"k","s":"s"}}`,
		marshalApproveJSON(proto.InboxItem{Env: fx.homeKey.Sign(marshalApproveJSON(proto.Item{
			Type: "item", Nonce: approveTestID(71), Timestamp: seamStart, Group: proto.ID(fx.groupID),
			To: "wrong-host", Item: 1, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(72),
		})), Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(approveTestID(72), "approval", fx.taskID, &proto.CardState{Status: "approved"})))}),
		marshalApproveJSON(proto.InboxItem{Env: fx.homeKey.Sign(marshalApproveJSON(proto.Item{
			Type: "item", Nonce: approveTestID(71), Timestamp: seamStart, Group: proto.ID(fx.groupID),
			To: fx.hostKey.ID(), Item: 1, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(72),
		})), Msg: json.RawMessage(`{`)}),
		marshalApproveJSON(proto.InboxItem{Env: fx.homeKey.Sign(marshalApproveJSON(proto.Item{
			Type: "item", Nonce: approveTestID(71), Timestamp: seamStart, Group: proto.ID(fx.groupID),
			To: fx.hostKey.ID(), Item: 1, AI: proto.ID(fx.memberID), Why: "decided", Msg: approveTestID(72),
		})), Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(approveTestID(72), "decision", fx.taskID, &proto.CardState{Status: "decided"})))}),
	} {
		_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	// Late decision with no pending row returns not_found.
	_, errMissing := fx.h.RecordApprovalDecision(fx.homeKey.ID(), fx.buildDecidedItem(fx.owner1, approveTestID(73), "approved"))
	seamCode(t, errMissing, "not_found")

	// Open approval wait rejects mismatched session/card/state/pending-row items without consuming wait.
	// Single reader: test goroutine awaiting posted card ID via recvApproveChan.
	postedCh := make(chan proto.ID, 1)
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) { postedCh <- post.Nonce }
	// Single reader: test goroutine awaiting callTool completion via recvApproveChan.
	doneCh := make(chan ApprovalResult, 1)
	go func() {
		doneCh <- fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"curl https://api.example"},"tool_use_id":"u_wait"}`)
	}()
	cardID := recvApproveChan(t, postedCh, "posted card")

	s, _ := fx.h.Sessions.Get(fx.sessKey)
	mustApproveErr(t, checkOpenDecisionWait(s, cardID, json.RawMessage(`{`)))
	mustApproveErr(t, checkOpenDecisionWait(Session{SessionKey: SessionKey{Group: "other", Member: s.Member}}, cardID, fx.buildDecisionItem(fx.owner1, s, cardID, nil, nil)))
	mustApproveErr(t, checkOpenDecisionWait(s, cardID, fx.buildDecidedItem(fx.owner1, cardID, "approved")))

	wrongAIItem := proto.Item{
		Type: "item", Nonce: approveTestID(74), Timestamp: seamStart, Group: proto.ID(fx.groupID),
		To: fx.hostKey.ID(), Item: 1, AI: approveTestID(75), Why: "decided", Task: proto.ID(fx.taskID), Msg: cardID,
	}
	wrongAIRaw := json.RawMessage(marshalApproveJSON(proto.InboxItem{
		Env: fx.homeKey.Sign(marshalApproveJSON(wrongAIItem)),
		Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved"}))),
	}))
	_, errWrongAI := fx.h.RecordApprovalDecision(fx.homeKey.ID(), wrongAIRaw)
	seamCode(t, errWrongAI, "bad_request")
	waitedBad, errBad := fx.h.Hand(string(cardID), json.RawMessage(`{`))
	mustApproveTrue(t, waitedBad && errBad != nil, "Hand bad json rejected")

	// Valid item with Item == 1 resolves open wait.
	itemOne := wrongAIItem
	itemOne.AI = proto.ID(fx.memberID)
	itemOne.Item = 1
	rawOne := json.RawMessage(marshalApproveJSON(proto.InboxItem{
		Env: fx.homeKey.Sign(marshalApproveJSON(itemOne)),
		Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(cardID, "approval", fx.taskID, &proto.CardState{Status: "approved"}))),
	}))
	st, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), rawOne)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, st, "approved", "Item=1 open wait status")
	mustApproveDeepEqual(t, recvApproveChan(t, doneCh, "waiting tool result"), ApprovalResult{Behavior: "allow"}, "waiting call outcome")

	// Deleted pending row while wait is open rejects decision.
	ctx2, cancel2 := context.WithCancel(t.Context())
	// Single reader: test goroutine awaiting cancelled callTool completion via recvApproveChan.
	done2 := make(chan ApprovalResult, 1)
	go func() {
		done2 <- fx.mustCallTool(ctx2, 3, `{"tool_name":"Bash","input":{"command":"curl https://two.example"},"tool_use_id":"u_deleted"}`)
	}()
	card2 := recvApproveChan(t, postedCh, "second posted card")
	mustApproveNoErr(t, fx.h.DeleteTaskApprovals(fx.groupID, fx.taskID))
	_, errDeleted := fx.h.RecordApprovalDecision(fx.homeKey.ID(), fx.buildDecidedItem(fx.owner1, card2, "approved"))
	seamCode(t, errDeleted, "bad_request")
	cancel2()
	recvApproveChan(t, done2, "cancelled waiting call")

	// lookupApprovalAgent returns "" when neither Rows nor Sessions knows the AI.
	fx.h.Rows.Reset(fx.groupID)
	mustApproveEqual(t, lookupApprovalAgent(fx.h, fx.groupID, string(approveTestID(98))), "", "unknown agent empty")
}

func TestCodexAskedListenerLevelsQuestionsConcurrencyAndCancellation(t *testing.T) {
	fx := newApproveFixture(t)
	runner := newApproveFakeRunner()
	prevRunner := Runner
	Runner = runner
	t.Cleanup(func() { Runner = prevRunner })

	codexKey, groupKey, reviewKey := seedCodexSessions(t, fx)
	assertCodexDefaultLevelThreeAndOverrides(t, fx, runner, codexKey, groupKey, reviewKey)
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

func assertCodexDefaultLevelThreeAndOverrides(t *testing.T, fx *approveFixture, runner *approveFakeRunner, codexKey, groupKey, reviewKey SessionKey) {
	t.Helper()
	fx.h.Report(codexKey, Event{Kind: Active})
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: nil})
	fx.h.Report(SessionKey{Group: "none", Member: "none", Task: "none"}, Event{Kind: Asked, Ask: &Ask{ID: "x"}})

	// Default level is 3 across task, group, and review sessions.
	for _, k := range []SessionKey{codexKey, groupKey, reviewKey} {
		s, _ := fx.h.Sessions.Get(k)
		mustApproveEqual(t, resolveSessionLevel(fx.h, s), 3, "default level 3")
	}

	// default_level = 1 denies without posting a card across all session kinds.
	seamSQL(t, fx.db, `INSERT INTO setting(key, value) VALUES('default_level', '1')`)
	for _, k := range []SessionKey{codexKey, groupKey, reviewKey} {
		fx.h.Report(k, Event{Kind: Asked, Ask: &Ask{ID: "def1", Tool: "commandExecution", Category: "command", Command: "ls"}})
		mustApproveEqual(t, recvApproveChan(t, runner.ch, "def1"), recordedAnswer{key: k, askID: "def1", allow: false}, "default_level=1")
		mustApproveEqual(t, recvApproveChan(t, runner.inCall, "def1 inCall"), 1, "Calls inside Answer")
	}
	approveStateOf(fx.h).wg.Wait()
	mustApproveEqual(t, fx.h.Sessions.Calls(groupKey), 0, "groupKey Calls after Ask")

	// perm = 2 overrides default_level = 1 and goes through approval card flow.
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		it := proto.Item{
			Type: "item", Nonce: approveTestID(52), Timestamp: seamStart, Group: proto.ID(fx.groupID),
			To: fx.hostKey.ID(), Item: 601, AI: proto.ID(fx.memberID), Why: "decided", Task: proto.ID(codexKey.Task), Msg: post.Nonce,
		}
		raw := json.RawMessage(marshalApproveJSON(proto.InboxItem{
			Env: fx.homeKey.Sign(marshalApproveJSON(it)),
			Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(post.Nonce, "approval", codexKey.Task, &proto.CardState{Status: "approved"}))),
		}))
		fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
	}
	seamSQL(t, fx.db, `INSERT INTO perm(agent, g, level, repo_config) VALUES('codex', '`+fx.groupID+`', 2, 0)`)
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "perm2", Tool: "commandExecution", Category: "command", Command: "ls"}})
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "perm2"), recordedAnswer{key: codexKey, askID: "perm2", allow: true}, "perm=2 card approved")
	recvApproveChan(t, runner.inCall, "perm2 inCall")

	// all_level3 = true overrides perm = 1 and also goes through approval card flow.
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		it := proto.Item{
			Type: "item", Nonce: approveTestID(53), Timestamp: seamStart, Group: proto.ID(fx.groupID),
			To: fx.hostKey.ID(), Item: 602, AI: proto.ID(fx.memberID), Why: "decided", Task: proto.ID(codexKey.Task), Msg: post.Nonce,
		}
		raw := json.RawMessage(marshalApproveJSON(proto.InboxItem{
			Env: fx.homeKey.Sign(marshalApproveJSON(it)),
			Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(post.Nonce, "approval", codexKey.Task, &proto.CardState{Status: "approved"}))),
		}))
		fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
	}
	seamSQL(t, fx.db,
		`UPDATE perm SET level = 1 WHERE agent = 'codex' AND g = '`+fx.groupID+`'`,
		`INSERT INTO setting(key, value) VALUES('all_level3', 'true')`,
	)
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "all3", Tool: "commandExecution", Category: "command", Command: "ls"}})
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "all3"), recordedAnswer{key: codexKey, askID: "all3", allow: true}, "all_level3=true card approved")
	recvApproveChan(t, runner.inCall, "all3 inCall")
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

	// Invalid inputs and secrets call Ask.Reply(nil).
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

	// Multi-question decision flow: option only, free text only, option + free text, and empty both.
	grpSess, _ := fx.h.Sessions.Get(groupKey)
	qStep := 0
	fx.home.onDecision = func(post proto.Post[proto.DecisionBody]) {
		qStep++
		var choice, note *string
		switch qStep {
		case 1:
			c := "A"
			choice = &c
		case 2:
			n := "Custom text"
			note = &n
		case 3:
			c := "B"
			n := "Note text"
			choice, note = &c, &n
		case 4:
			// Neither choice nor text -> empty slice.
		}
		waited, err := fx.h.Hand(string(post.Nonce), fx.buildDecisionItem(fx.owner2, grpSess, post.Nonce, choice, note))
		mustApproveTrue(t, waited && err == nil, "Codex decision Hand")
	}
	validInput := marshalApproveJSON(map[string]any{
		"questions": []map[string]any{
			{"id": "q1", "header": "H1", "question": "First?", "options": []map[string]string{{"label": "L1", "description": "D1"}}},
			{"id": "q2", "header": "", "question": "Second?", "options": []map[string]string{{"label": "L2", "description": "D2"}}},
			{"id": "q3", "header": "H3", "question": "Third?", "options": []map[string]string{{"label": "A3", "description": "D"}, {"label": "B3", "description": "D"}}},
			{"id": "q4", "header": "", "question": "Fourth?", "options": []map[string]string{}},
		},
	})
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "ok-q", Input: json.RawMessage(validInput), Reply: replyFn}})
	gotAns := recvApproveChan(t, replyCh, "Codex multi-question Reply")
	wantAns := map[string][]string{
		"q1": {"L1"},
		"q2": {"Custom text"},
		"q3": {"B3", "Note text"},
		"q4": {},
	}
	mustApproveDeepEqual(t, gotAns, wantAns, "Codex multi-question answers")

	// Home refusal on Codex question calls Reply(nil).
	fx.home.onDecision = nil
	fx.home.answer = `{"re":1,"err":{"code":"conflict","msg":"refused"}}`
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "ref-q", Input: json.RawMessage(validInput), Reply: replyFn}})
	mustApproveTrue(t, recvApproveChan(t, replyCh, "refused Codex question") == nil, "refused Codex question nil")
	fx.home.answer = ""

	// Cancelled turn while Codex question waits does not call Reply.
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
	approveStateOf(fx.h).wg.Wait()
	mustApproveEqual(t, calledReply.Load(), false, "cancelled Codex question must not call Reply")
	fx.home.onDecision = nil
}

func pickCardByCmd(p1, p2 proto.Post[proto.ApprovalBody], cmd string) proto.ID {
	if p1.Body.Command == cmd {
		return p1.Nonce
	}
	return p2.Nonce
}

func assertCodexConcurrentWaitAndCancel(t *testing.T, fx *approveFixture, runner *approveFakeRunner, codexKey, groupKey SessionKey) {
	t.Helper()
	seamSQL(t, fx.db,
		`DELETE FROM setting WHERE key IN ('all_level3', 'default_level')`,
		`DELETE FROM perm WHERE agent = 'codex'`,
	)
	// Single reader: test goroutine awaiting two Codex approval posts via recvApproveChan.
	cards := make(chan proto.Post[proto.ApprovalBody], 2)
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) { cards <- post }

	seamSoon(t, "Report for first Codex Ask", func() {
		fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "c1", Tool: "commandExecution", Category: "command", Command: "make a"}})
	})
	seamSoon(t, "Report for second Codex Ask", func() {
		fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "c2", Tool: "fileChange", Category: "file", Command: "add a.txt", Input: json.RawMessage(`[{"path":"a.txt"}]`)}})
	})
	mustApproveEqual(t, fx.h.Sessions.Calls(codexKey), 2, "Calls right after two Reports")

	postA := recvApproveChan(t, cards, "first Codex card")
	postB := recvApproveChan(t, cards, "second Codex card")
	mustApproveTrue(t, postA.Nonce != postB.Nonce, "concurrent asks got distinct cards")
	cardC1 := pickCardByCmd(postA, postB, "make a")

	countBeforeC1 := runner.answerCount()
	itA := proto.Item{
		Type: "item", Nonce: approveTestID(55), Timestamp: seamStart,
		Group: proto.ID(fx.groupID), To: fx.hostKey.ID(), Item: 777,
		AI: proto.ID(fx.memberID), Why: "decided", Task: proto.ID(codexKey.Task), Msg: cardC1,
	}
	rawA := json.RawMessage(marshalApproveJSON(proto.InboxItem{
		Env: fx.homeKey.Sign(marshalApproveJSON(itA)),
		Msg: json.RawMessage(marshalApproveJSON(fx.makeCardMsg(cardC1, "approval", codexKey.Task, &proto.CardState{Status: "approved"}))),
	}))
	_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), rawA)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "c1 answer"), recordedAnswer{key: codexKey, askID: "c1", allow: true}, "c1 allow")
	recvApproveChan(t, runner.inCall, "c1 inCall")

	fx.h.Report(codexKey, Event{Kind: TurnEnded, Status: "interrupted"})
	approveStateOf(fx.h).wg.Wait()
	mustApproveEqual(t, fx.h.Sessions.Calls(codexKey), 0, "Calls after TurnEnded")
	mustApproveEqual(t, runner.answerCount(), countBeforeC1+1, "cancelled Ask did not call Runner.Answer")

	seamSQL(t, fx.db, `INSERT INTO setting(key, value) VALUES('default_level', '1')`)
	Runner = nil
	fx.h.Report(groupKey, Event{Kind: Asked, Ask: &Ask{ID: "nil-runner", Tool: "commandExecution", Category: "command", Command: "ls"}})
	approveStateOf(fx.h).wg.Wait()
}

func TestStoreFailurePathsAndNonJSONInputDigest(t *testing.T) {
	mustApproveEqual(t, len(hashApprovalInput(json.RawMessage("  not-json  "))), 64, "non-JSON digest len")

	fx := newApproveFixture(t)
	card := approveTestID(90)
	it := proto.Item{
		Type: "item", Nonce: approveTestID(91), Timestamp: seamStart,
		Group: proto.ID(fx.groupID), To: fx.hostKey.ID(), Item: 888,
		AI: proto.ID(fx.memberID), Why: "decided", Task: proto.ID(fx.taskID), Msg: card,
	}

	digest := hashApprovalInput(json.RawMessage(`{"command":"echo x"}`))
	seamSQL(t, fx.db, "INSERT INTO approval(card, g, member, task, tool, input, state) VALUES('"+string(card)+"', '"+fx.groupID+"', '"+fx.memberID+"', '"+fx.taskID+"', 'Bash', '"+digest+"', 'granted')")
	seamSQL(t, fx.db, "DROP TABLE audit")

	s, ok := fx.h.Sessions.Get(fx.sessKey)
	mustApproveTrue(t, ok, "session exists")
	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "echo x"), false, "consumeGrantedApproval without audit")

	seamSQL(t, fx.db, "UPDATE approval SET state = 'pending' WHERE card = '"+string(card)+"'")
	fx.home.putCard(fx.makeCardMsg(card, "approval", fx.taskID, &proto.CardState{Status: "approved"}))
	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "echo x"), false, "consumePendingCard without audit")

	_, errLate := applyLateApprovalDecision(fx.h, fx.homeKey.ID(), "claude", fx.owner1.ID(), it, "approved")
	mustApproveErr(t, errLate)

	seamSQL(t, fx.db, "DROP TABLE approval")
	mustApproveEqual(t, consumeGrantedApproval(t.Context(), fx.h, s, "Bash", digest, "echo x"), false, "consumeGrantedApproval without approval")

	res := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"echo x"},"tool_use_id":"u_no_tbl"}`)
	mustApproveDeepEqual(t, res, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: internal: failed to store approval record"}, "missing approval table")
	mustApproveErr(t, fx.h.DeleteTaskApprovals(fx.groupID, fx.taskID))
}
