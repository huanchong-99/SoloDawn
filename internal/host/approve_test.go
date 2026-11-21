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
	mu      sync.Mutex
	done    chan struct{}
	answer  string
	err     error
	posts   []proto.Post[proto.ApprovalBody]
	rawCmds []proto.Cmd
	acks    []proto.Ack
	onPost  func(post proto.Post[proto.ApprovalBody])
}

func newApproveFakeHome() *approveFakeHome {
	return &approveFakeHome{
		// Closed on disconnect; read via Done().
		done: make(chan struct{}),
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

func (f *approveFakeHome) recordCmd(build func(int64) any) (proto.Post[proto.ApprovalBody], bool, string, error, func(proto.Post[proto.ApprovalBody])) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := int64(len(f.rawCmds) + 1)
	cmd := build(id).(proto.Cmd)
	f.rawCmds = append(f.rawCmds, cmd)
	var post proto.Post[proto.ApprovalBody]
	hasPost := proto.Decode([]byte(cmd.Env.Payload), &post) == nil
	if hasPost {
		f.posts = append(f.posts, post)
	}
	return post, hasPost, f.answer, f.err, f.onPost
}

func (f *approveFakeHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	post, hasPost, ans, reqErr, hook := f.recordCmd(build)
	if reqErr != nil {
		return nil, reqErr
	}
	if ans != "" {
		return json.RawMessage(ans), nil
	}
	if hasPost && hook != nil {
		hook(post)
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
		ch: make(chan recordedAnswer, 8),
		// Single reader: test goroutine reading Sessions.Calls count via recvApproveChan.
		inCall: make(chan int, 8),
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
	stranger identity.Key
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
		stranger: approveTestKey(6),
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
	fx.seedOwnerStoreLog()
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

func (fx *approveFixture) seedOwnerStoreLog() {
	fx.t.Helper()
	entries := []proto.Envelope{
		fx.devEntry(fx.owner1, fx.owner1.ID(), 1, "add", fx.owner1.ID(), "browser", "b1", 21),
		fx.devEntry(fx.owner1, fx.owner1.ID(), 2, "add", fx.owner2.ID(), "browser", "b2", 22),
		fx.devEntry(fx.owner1, fx.owner1.ID(), 3, "add", fx.homeKey.ID(), "node", "home", 23),
		fx.devEntry(fx.owner1, fx.owner1.ID(), 4, "add", fx.hostKey.ID(), "node", "host", 24),
	}
	work := func(tx *store.Tx) error {
		if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", marshalApproveJSON(fx.owner1.ID())); err != nil {
			return err
		}
		for i, env := range entries {
			if _, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", fx.owner1.ID(), i+1, marshalApproveJSON(env)); err != nil {
				return err
			}
		}
		return nil
	}
	mustApproveNoErr(fx.t, fx.db.Write(work))
}

func (fx *approveFixture) devEntry(signer identity.Key, user string, ver int, op, dev, kind, name string, nonceSeed byte) proto.Envelope {
	entry := proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: approveTestID(nonceSeed), Timestamp: seamStart},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: op, Device: dev, Kind: kind, Name: name},
	}
	return signer.Sign(marshalApproveJSON(entry))
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

func (fx *approveFixture) buildDecidedItem(homeSigner, actSigner identity.Key, card proto.ID, action string) json.RawMessage {
	fx.itemSeq++
	act := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: approveTestID(byte(60 + fx.itemSeq%40)), Timestamp: seamStart, Group: proto.ID(fx.groupID)},
		Msg:         card,
		Action:      action,
	}
	cause := actSigner.Sign(marshalApproveJSON(act))
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
	env := homeSigner.Sign(marshalApproveJSON(item))
	return json.RawMessage(marshalApproveJSON(proto.InboxItem{Env: env, Cause: &cause}))
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

	// Pin both operands of the structured Claude visibility filter.
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
	}
	for _, bad := range badPayloads {
		_, err := fx.callTool(t.Context(), 2, bad)
		seamCode(t, err, "bad_request")
	}
}

func assertMappedToolCategories(t *testing.T, fx *approveFixture) {
	t.Helper()
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(fx.homeKey, fx.owner2, post.Nonce, "approve")
		_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
		mustApproveNoErr(t, err)
	}
	cases := []categoryMapCase{
		{raw: `{"tool_name":"Bash","input":{"command":"git push origin other","description":"Push feature"},"tool_use_id":"u1"}`, tool: "Bash", category: "command", command: "git push origin other", why: new("Push feature")},
		{raw: `{"tool_name":"Bash","input":{"command":"ls -la"},"tool_use_id":""}`, tool: "Bash", category: "command", command: "ls -la"},
		{raw: "{\n  \"tool_name\": \"Write\",\n  \"input\": {\n    \"filePath\": \"/outside/a.txt\",\n    \"content\": \"x\"\n  },\n  \"tool_use_id\": \"u2\"\n}", tool: "Write", category: "file", command: `{"filePath":"/outside/a.txt","content":"x"}`},
		{raw: `{"tool_name":"Edit","input":{"old_string":"a","new_string":"b"},"tool_use_id":"u3"}`, tool: "Edit", category: "file", command: `{"old_string":"a","new_string":"b"}`},
		{raw: `{"tool_name":"NotebookEdit","input":{"notebook_path":"n.ipynb"},"tool_use_id":"u4"}`, tool: "NotebookEdit", category: "file", command: `{"notebook_path":"n.ipynb"}`},
		{raw: `{"tool_name":"Read","input":{"file_path":"/etc/hosts"},"tool_use_id":"u5"}`, tool: "Read", category: "file", command: `{"file_path":"/etc/hosts"}`},
		{raw: `{"tool_name":"WebFetch","input":{"url":"https://example.com"},"tool_use_id":"u6"}`, tool: "WebFetch", category: "network", command: `{"url":"https://example.com"}`},
		{raw: `{"tool_name":"WebSearch","input":{"query":"golang"},"tool_use_id":"u7"}`, tool: "WebSearch", category: "network", command: `{"query":"golang"}`},
		{raw: `{"tool_name":"CustomMCP","input":{"k":1},"tool_use_id":"u8"}`, tool: "CustomMCP", category: "other", command: `{"k":1}`},
	}
	for _, tc := range cases {
		res := fx.mustCallTool(t.Context(), 2, tc.raw)
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

func TestApproveLevelsAndOutboundSecretCheck(t *testing.T) {
	fx := newApproveFixture(t)
	assertApproveLevelsOneAndThree(t, fx)
	assertApproveOutboundSecrets(t, fx)
}

func assertApproveLevelsOneAndThree(t *testing.T, fx *approveFixture) {
	t.Helper()
	res1 := fx.mustCallTool(t.Context(), 1, `{"tool_name":"Bash","input":{"command":"touch x"},"tool_use_id":"u1"}`)
	mustApproveEqual(t, marshalApproveJSON(res1), `{"behavior":"deny","message":"Denied: this session is read only."}`, "level 1")

	res3 := fx.mustCallTool(t.Context(), 3, `{"tool_name":"Bash","input":{"command":"touch x"},"tool_use_id":"u2"}`)
	mustApproveEqual(t, marshalApproveJSON(res3), `{"behavior":"allow"}`, "level 3")
	mustApproveEqual(t, len(fx.home.postList()), 0, "level 1/3 posts")
	mustApproveEqual(t, fx.approvalRows(), "", "level 1/3 rows")
	mustApproveEqual(t, seamRecord(t, fx.h), "", "level 1/3 audit")
}

func assertApproveOutboundSecrets(t *testing.T, fx *approveFixture) {
	t.Helper()
	secretTok := "ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD"
	longWithSecret := strings.Repeat("a", 4000) + " " + secretTok
	argCmdSecret := marshalApproveJSON(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": longWithSecret, "description": "Safe justification"},
		"tool_use_id": "u3",
	})
	resSec1 := fx.mustCallTool(t.Context(), 2, argCmdSecret)
	mustApproveEqual(t, resSec1, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "command secret")
	mustApproveTrue(t, !strings.Contains(marshalApproveJSON(resSec1), secretTok), "deny leaked secret")

	argWhySecret := marshalApproveJSON(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": "echo ok", "description": "Using -----BEGIN PRIVATE KEY-----"},
		"tool_use_id": "u4",
	})
	resSec2 := fx.mustCallTool(t.Context(), 2, argWhySecret)
	mustApproveEqual(t, resSec2, ApprovalResult{Behavior: "deny", Message: denySecretMessage}, "why secret")
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
	delegateDev2 := approveTestKey(8)
	d1 := fx.devEntry(fx.delegate, fx.delegate.ID(), 1, "add", fx.delegate.ID(), "browser", "del-1", 31)
	d2 := fx.devEntry(fx.delegate, fx.delegate.ID(), 2, "add", delegateDev2.ID(), "browser", "del-2", 32)
	fx.h.Rows.Put(fx.groupID, "device", fx.delegate.ID()+":2", json.RawMessage(marshalApproveJSON(proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: fx.delegate.ID() + ":2", Seq: 7},
		DeviceChange: proto.DeviceChange{User: fx.delegate.ID(), Version: 2, Op: "add", Device: delegateDev2.ID(), Kind: "browser", Name: "del-2"},
		Env:          d2,
	})))
	fx.h.Rows.Put(fx.groupID, "device", fx.delegate.ID()+":1", json.RawMessage(marshalApproveJSON(proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: fx.delegate.ID() + ":1", Seq: 8},
		DeviceChange: proto.DeviceChange{User: fx.delegate.ID(), Version: 1, Op: "add", Device: fx.delegate.ID(), Kind: "browser", Name: "del-1"},
		Env:          d1,
	})))
	fx.h.Rows.Put(fx.groupID, "device", "corrupt", json.RawMessage(`{"r":"device"`))
	return delegateDev2
}

func assertTimelyDelegateApprove(t *testing.T, fx *approveFixture, delegateDev2 identity.Key) {
	t.Helper()
	// Approve inside Home.Request so the test proves Await opened before Command.
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) {
		raw := fx.buildDecidedItem(fx.homeKey, delegateDev2, post.Nonce, "approve")
		action, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
		mustApproveNoErr(t, err)
		mustApproveEqual(t, action, "approve", "timely delegate action")
	}
	resAllow := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Bash","input":{"command":"npm test","description":"Run suite"},"tool_use_id":"u1"}`)
	mustApproveEqual(t, resAllow, ApprovalResult{Behavior: "allow"}, "timely allow")
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
		raw := fx.buildDecidedItem(fx.homeKey, fx.owner2, post.Nonce, "deny")
		action, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), raw)
		mustApproveNoErr(t, err)
		mustApproveEqual(t, action, "deny", "timely deny action")
	}
	resDeny := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Bash","input":{"command":"rm -rf /tmp/x"},"tool_use_id":"u2"}`)
	mustApproveEqual(t, marshalApproveJSON(resDeny), `{"behavior":"deny","message":"Denied by the owner."}`, "timely deny")
	mustApproveEqual(t, fx.approvalRows(), "", "timely deny rows")

	fx.home.onPost = nil
	fx.home.answer = `{"re":1,"err":{"code":"conflict","msg":"task is not active"}}`
	resRefused := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Bash","input":{"command":"echo hi"},"tool_use_id":"u3"}`)
	mustApproveEqual(t, resRefused, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: conflict: task is not active"}, "home refusal")
	mustApproveEqual(t, fx.approvalRows(), "", "home refusal rows")

	fx.home.answer = ""
	fx.home.err = io.EOF
	resUnreach := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Bash","input":{"command":"echo hi"},"tool_use_id":"u4"}`)
	mustApproveEqual(t, resUnreach, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: home_unreachable: the home of that group is not connected"}, "home unreachable")
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
}

func TestTimeoutLateDecisionRestartAndOneTimeGrant(t *testing.T) {
	fx := newApproveFixture(t)
	setFastWaitUnit(t, time.Millisecond)
	grpOne := assertWaitDurationAndSelectBranches(t, fx)
	assertLateApproveOneTimeGrantAndCleanup(t, fx, grpOne)
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
	resTimeout := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_t1"}`)
	mustApproveEqual(t, resTimeout, ApprovalResult{Behavior: "deny", Message: denyTimeoutMessage}, "timeout deny")
	card1 := fx.home.postList()[0].Nonce
	mustApproveTrue(t, strings.HasSuffix(fx.approvalRows(), "|Write|pending"), "pending row after timeout")

	restarted, _, err := New(fx.db, fx.hostKey, approveTestClock())
	mustApproveNoErr(t, err)
	fx.h = restarted
	fx.h.Attach(fx.homeKey.ID(), fx.home)

	decidedRaw := fx.buildDecidedItem(fx.homeKey, fx.owner2, card1, "approve")
	act1, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), decidedRaw)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, act1, "approve", "late approve action")
	mustApproveTrue(t, strings.HasSuffix(fx.approvalRows(), "|Write|granted"), "granted row after late approve")

	ackCountBefore := len(fx.home.ackList())
	actDup, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), decidedRaw)
	mustApproveNoErr(t, err)
	mustApproveEqual(t, actDup, "", "duplicate late approve action")
	mustApproveEqual(t, len(fx.home.ackList()), ackCountBefore+1, "duplicate ack count")

	fx.seedDefaultRows()
	fx.h.Rows.Put(fx.groupID, "group", fx.groupID, json.RawMessage(marshalApproveJSON(grpOne)))
	resDiffTool := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Edit","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_diff_tool"}`)
	resDiffInput := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"changed"},"tool_use_id":"u_diff_in"}`)
	mustApproveEqual(t, resDiffTool.Behavior, "deny", "different tool must not match grant")
	mustApproveEqual(t, resDiffInput.Behavior, "deny", "different input must not match grant")
	mustApproveEqual(t, len(fx.home.postList()), 3, "posts after non-matching calls")

	postsBefore := len(fx.home.postList())
	resRetry := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Write","input":{ "content": "<v1>&v2", "file_path": "/ext/a.txt" },"tool_use_id":"u_retry_ok"}`)
	mustApproveEqual(t, resRetry, ApprovalResult{Behavior: "allow"}, "granted retry")
	mustApproveEqual(t, len(fx.home.postList()), postsBefore, "granted retry did not post")

	resSecondTry := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Write","input":{"file_path":"/ext/a.txt","content":"<v1>&v2"},"tool_use_id":"u_retry_2"}`)
	mustApproveEqual(t, resSecondTry.Behavior, "deny", "second retry asks again")
	mustApproveEqual(t, len(fx.home.postList()), postsBefore+1, "second retry posted new card")

	latestCard := fx.home.postList()[len(fx.home.postList())-1].Nonce
	actDeny, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), fx.buildDecidedItem(fx.homeKey, fx.owner1, latestCard, "deny"))
	mustApproveNoErr(t, err)
	mustApproveEqual(t, actDeny, "deny", "late deny action")

	otherTask := string(approveTestID(99))
	seamSQL(t, fx.db, "INSERT INTO approval(card, g, member, task, tool, input, state) VALUES('other-card', '"+fx.groupID+"', '"+fx.memberID+"', '"+otherTask+"', 'Bash', 'abc', 'granted')")
	mustApproveNoErr(t, fx.h.DeleteTaskApprovals(fx.groupID, fx.taskID))
	mustApproveEqual(t, fx.approvalRows(), "other-card|"+fx.groupID+"|"+fx.memberID+"|"+otherTask+"|Bash|granted", "rows after DeleteTaskApprovals")
}

func TestCauseVerificationRefusesEveryForgery(t *testing.T) {
	fx := newApproveFixture(t)
	revokedKey := seedRevokedDeviceAndBadGroupLog(t, fx)
	assertEnvelopeFailures(t, fx)
	assertLiveWaitRefusesForgeries(t, fx, revokedKey)
	assertDeletedOrMissingApprovalRefused(t, fx)
}

func seedRevokedDeviceAndBadGroupLog(t *testing.T, fx *approveFixture) identity.Key {
	t.Helper()
	revokedKey := approveTestKey(7)
	e5 := fx.devEntry(fx.owner1, fx.owner1.ID(), 5, "add", revokedKey.ID(), "browser", "stolen", 41)
	e6 := fx.devEntry(fx.owner1, fx.owner1.ID(), 6, "revoke", revokedKey.ID(), "", "", 42)
	seamSQL(t, fx.db,
		"INSERT INTO device_log(user, ver, env) VALUES('"+fx.owner1.ID()+"', 5, '"+marshalApproveJSON(e5)+"')",
		"INSERT INTO device_log(user, ver, env) VALUES('"+fx.owner1.ID()+"', 6, '"+marshalApproveJSON(e6)+"')",
	)
	fx.h.Rows.Put(fx.groupID, "device", "bad-genesis", json.RawMessage(marshalApproveJSON(proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: "bad-genesis", Seq: 20},
		DeviceChange: proto.DeviceChange{User: fx.delegate.ID(), Version: 1, Op: "revoke", Device: fx.delegate.ID()},
		Env:          fx.devEntry(fx.delegate, fx.delegate.ID(), 1, "revoke", fx.delegate.ID(), "", "", 43),
	})))
	return revokedKey
}

func assertEnvelopeFailures(t *testing.T, fx *approveFixture) {
	t.Helper()
	for _, raw := range []string{`{`, `{"env":{"p":"{}","k":"` + fx.homeKey.ID() + `","s":""}}`} {
		_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), json.RawMessage(raw))
		mustApproveErr(t, err)
	}
	cardDummy := approveTestID(70)
	validSample := fx.buildDecidedItem(fx.homeKey, fx.owner1, cardDummy, "approve")
	_, errHome := fx.h.RecordApprovalDecision("other-home", validSample)
	mustApproveErr(t, errHome)
	_, errStrangerHome := fx.h.RecordApprovalDecision(fx.stranger.ID(), fx.buildDecidedItem(fx.stranger, fx.owner1, cardDummy, "approve"))
	mustApproveErr(t, errStrangerHome)
}

func makeCustomApproveItem(homeKey identity.Key, it proto.Item, cause *proto.Envelope) json.RawMessage {
	env := homeKey.Sign(marshalApproveJSON(it))
	return json.RawMessage(marshalApproveJSON(proto.InboxItem{Env: env, Cause: cause}))
}

func assertLiveWaitRefusesForgeries(t *testing.T, fx *approveFixture, revokedKey identity.Key) {
	t.Helper()
	// Single reader: test goroutine awaiting posted card ID via recvApproveChan.
	postedCh := make(chan proto.ID, 1)
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) { postedCh <- post.Nonce }
	// Single reader: test goroutine awaiting callTool completion via recvApproveChan.
	doneCh := make(chan ApprovalResult, 1)
	go func() {
		doneCh <- fx.mustCallTool(t.Context(), 2, `{"tool_name":"Bash","input":{"command":"curl https://api.example"},"tool_use_id":"u_forged"}`)
	}()
	cardID := recvApproveChan(t, postedCh, "posted card")

	goodAct := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: approveTestID(71), Timestamp: seamStart, Group: proto.ID(fx.groupID)},
		Msg:         cardID,
		Action:      "approve",
	}
	goodCause := fx.owner1.Sign(marshalApproveJSON(goodAct))
	baseItem := proto.Item{
		Type: "item", Nonce: approveTestID(72), Timestamp: seamStart,
		Group: proto.ID(fx.groupID), To: fx.hostKey.ID(), Item: 901,
		AI: proto.ID(fx.memberID), Why: "decided", Task: proto.ID(fx.taskID), Msg: cardID,
	}
	assertDirectHandForgeries(t, fx, cardID, baseItem, goodCause)
	assertActForgeries(t, fx, baseItem, goodAct, goodCause, revokedKey)

	act, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), fx.buildDecidedItem(fx.homeKey, fx.owner2, cardID, "approve"))
	mustApproveNoErr(t, err)
	mustApproveEqual(t, act, "approve", "valid decision after refusals")
	mustApproveEqual(t, recvApproveChan(t, doneCh, "waiting tool result"), ApprovalResult{Behavior: "allow"}, "waiting call outcome")
}

func assertDirectHandForgeries(t *testing.T, fx *approveFixture, cardID proto.ID, baseItem proto.Item, goodCause proto.Envelope) {
	t.Helper()
	withItem := func(mod func(*proto.Item)) json.RawMessage {
		cp := baseItem
		mod(&cp)
		return makeCustomApproveItem(fx.homeKey, cp, &goodCause)
	}
	badDirect := []json.RawMessage{
		json.RawMessage(`{`),
		makeCustomApproveItem(fx.homeKey, baseItem, nil),
		withItem(func(x *proto.Item) { x.Type = "other" }),
		withItem(func(x *proto.Item) { x.Item = 0 }),
		withItem(func(x *proto.Item) { x.Why = "mention" }),
		withItem(func(x *proto.Item) { x.Group = approveTestID(81) }),
		withItem(func(x *proto.Item) { x.AI = approveTestID(82) }),
		withItem(func(x *proto.Item) { x.Msg = approveTestID(83) }),
	}
	for _, bad := range badDirect {
		waited, err := fx.h.Hand(string(cardID), bad)
		mustApproveEqual(t, waited, true, "Hand waited")
		mustApproveErr(t, err)
	}
}

func assertActForgeries(t *testing.T, fx *approveFixture, baseItem proto.Item, goodAct proto.Act, goodCause proto.Envelope, revokedKey identity.Key) {
	t.Helper()
	signAct := func(mod func(*proto.Act)) proto.Envelope {
		cp := goodAct
		mod(&cp)
		return fx.owner1.Sign(marshalApproveJSON(cp))
	}
	badActs := []proto.Envelope{
		{Payload: `{`, Key: fx.owner1.ID(), Signature: goodCause.Signature},
		signAct(func(a *proto.Act) { a.Type = "post" }),
		signAct(func(a *proto.Act) { a.Group = approveTestID(84) }),
		signAct(func(a *proto.Act) { a.Msg = approveTestID(85) }),
		signAct(func(a *proto.Act) { a.As = approveTestID(86) }),
		signAct(func(a *proto.Act) { a.Action = "confirm" }),
		signAct(func(a *proto.Act) { a.Arg = "A" }),
		signAct(func(a *proto.Act) { a.Text = "note" }),
		{Payload: goodCause.Payload, Key: fx.owner1.ID(), Signature: fx.stranger.Sign(goodCause.Payload).Signature},
		fx.stranger.Sign(marshalApproveJSON(goodAct)),
		revokedKey.Sign(marshalApproveJSON(goodAct)),
	}
	for i, badCause := range badActs {
		it := baseItem
		it.Item = int64(920 + i)
		c := badCause
		_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), makeCustomApproveItem(fx.homeKey, it, &c))
		mustApproveErr(t, err)
	}
}

func assertDeletedOrMissingApprovalRefused(t *testing.T, fx *approveFixture) {
	t.Helper()
	// Single reader: test goroutine awaiting second posted card ID via recvApproveChan.
	postedCh := make(chan proto.ID, 1)
	fx.home.onPost = func(post proto.Post[proto.ApprovalBody]) { postedCh <- post.Nonce }
	ctx2, cancel2 := context.WithCancel(t.Context())
	// Single reader: test goroutine awaiting cancelled callTool completion via recvApproveChan.
	done2 := make(chan ApprovalResult, 1)
	go func() {
		done2 <- fx.mustCallTool(ctx2, 2, `{"tool_name":"Bash","input":{"command":"curl https://two.example"},"tool_use_id":"u_deleted"}`)
	}()
	card2 := recvApproveChan(t, postedCh, "second posted card")
	mustApproveNoErr(t, fx.h.DeleteTaskApprovals(fx.groupID, fx.taskID))
	_, errDeleted := fx.h.RecordApprovalDecision(fx.homeKey.ID(), fx.buildDecidedItem(fx.homeKey, fx.owner1, card2, "approve"))
	mustApproveErr(t, errDeleted)
	cancel2()
	recvApproveChan(t, done2, "cancelled waiting call")

	unknownItem := proto.Item{
		Type: "item", Nonce: approveTestID(94), Timestamp: seamStart,
		Group: proto.ID(fx.groupID), To: fx.hostKey.ID(), Item: 999,
		AI: approveTestID(95), Why: "decided", Task: proto.ID(fx.taskID), Msg: approveTestID(96),
	}
	unknownCause := fx.owner1.Sign(marshalApproveJSON(proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: approveTestID(97), Timestamp: seamStart, Group: proto.ID(fx.groupID)},
		Msg:         unknownItem.Msg,
		Action:      "approve",
	}))
	_, errMissing := fx.h.RecordApprovalDecision(fx.homeKey.ID(), makeCustomApproveItem(fx.homeKey, unknownItem, &unknownCause))
	mustApproveErr(t, errMissing)

	extractAction, extractBy := extractDecisionAct(json.RawMessage(`{}`))
	mustApproveEqual(t, extractAction, "", "empty extract action")
	mustApproveEqual(t, extractBy, "", "empty extract by")
}

func TestCodexAskedListenerLevelsConcurrencyAndCancellation(t *testing.T) {
	fx := newApproveFixture(t)
	runner := newApproveFakeRunner()
	prevRunner := Runner
	Runner = runner
	t.Cleanup(func() { Runner = prevRunner })

	codexKey, groupKey, reviewKey := seedCodexSessions(t, fx)
	assertCodexReadOnlyAndPermLevels(t, fx, runner, codexKey, groupKey, reviewKey)
	assertCodexConcurrentWaitAndCancel(t, fx, runner, codexKey, groupKey)
}

func seedCodexSessions(t *testing.T, fx *approveFixture) (SessionKey, SessionKey, SessionKey) {
	t.Helper()
	codexKey := SessionKey{Group: fx.groupID, Member: fx.memberID, Task: string(approveTestID(50))}
	groupKey := SessionKey{Group: fx.groupID, Member: fx.memberID}
	reviewKey := SessionKey{Group: fx.groupID, Member: fx.memberID, Task: string(approveTestID(51))}
	for _, s := range []Session{
		{SessionKey: codexKey, Kind: TaskSession, Folder: "wt/50-codex", Agent: "codex", Home: fx.homeKey.ID(), Name: "Codex"},
		{SessionKey: groupKey, Kind: GroupSession, Folder: "repo", Agent: "codex", Home: fx.homeKey.ID(), Name: "Codex"},
		{SessionKey: reviewKey, Kind: ReviewSession, Folder: "review/51", Agent: "codex", Home: fx.homeKey.ID(), Name: "Codex"},
	} {
		mustApproveNoErr(t, fx.h.Sessions.Create(s))
	}
	return codexKey, groupKey, reviewKey
}

func assertCodexReadOnlyAndPermLevels(t *testing.T, fx *approveFixture, runner *approveFakeRunner, codexKey, groupKey, reviewKey SessionKey) {
	t.Helper()
	fx.h.Report(codexKey, Event{Kind: Active})
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: nil})
	fx.h.Report(SessionKey{Group: "none", Member: "none", Task: "none"}, Event{Kind: Asked, Ask: &Ask{ID: "x"}})

	for _, k := range []SessionKey{groupKey, reviewKey} {
		fx.h.Report(k, Event{Kind: Asked, Ask: &Ask{ID: "ro", Tool: "commandExecution", Category: "command", Command: "ls"}})
		got := recvApproveChan(t, runner.ch, "read-only answer")
		mustApproveEqual(t, got, recordedAnswer{key: k, askID: "ro", allow: false}, "read-only answer")
		mustApproveEqual(t, recvApproveChan(t, runner.inCall, "read-only Calls"), 1, "Calls inside Answer")
	}
	approveStateOf(fx.h).wg.Wait()
	mustApproveEqual(t, fx.h.Sessions.Calls(groupKey), 0, "groupKey Calls after Ask")
	mustApproveEqual(t, fx.h.Sessions.Calls(reviewKey), 0, "reviewKey Calls after Ask")

	seamSQL(t, fx.db, `INSERT INTO setting(key, value) VALUES('default_level', '1')`)
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "def1", Tool: "commandExecution", Category: "command", Command: "ls"}})
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "def1"), recordedAnswer{key: codexKey, askID: "def1", allow: false}, "default_level=1")
	recvApproveChan(t, runner.inCall, "def1 inCall")

	seamSQL(t, fx.db, `INSERT INTO perm(agent, g, level, repo_config) VALUES('codex', '`+fx.groupID+`', 3, 0)`)
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "perm3", Tool: "commandExecution", Category: "command", Command: "ls"}})
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "perm3"), recordedAnswer{key: codexKey, askID: "perm3", allow: true}, "perm=3")
	recvApproveChan(t, runner.inCall, "perm3 inCall")

	seamSQL(t, fx.db,
		`UPDATE perm SET level = 1 WHERE agent = 'codex' AND g = '`+fx.groupID+`'`,
		`INSERT INTO setting(key, value) VALUES('all_level3', 'true')`,
	)
	fx.h.Report(codexKey, Event{Kind: Asked, Ask: &Ask{ID: "all3", Tool: "commandExecution", Category: "command", Command: "ls"}})
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "all3"), recordedAnswer{key: codexKey, askID: "all3", allow: true}, "all_level3=true")
	recvApproveChan(t, runner.inCall, "all3 inCall")
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
	causeA := fx.owner2.Sign(marshalApproveJSON(proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: approveTestID(56), Timestamp: seamStart, Group: proto.ID(fx.groupID)},
		Msg:         cardC1,
		Action:      "approve",
	}))
	_, err := fx.h.RecordApprovalDecision(fx.homeKey.ID(), makeCustomApproveItem(fx.homeKey, itA, &causeA))
	mustApproveNoErr(t, err)
	mustApproveEqual(t, recvApproveChan(t, runner.ch, "c1 answer"), recordedAnswer{key: codexKey, askID: "c1", allow: true}, "c1 allow")
	recvApproveChan(t, runner.inCall, "c1 inCall")

	mustApproveTrue(t, isTurnBoundary(TurnBegan), "TurnBegan is boundary")
	mustApproveTrue(t, isTurnBoundary(TurnEnded), "TurnEnded is boundary")
	mustApproveTrue(t, !isTurnBoundary(Active), "Active is not boundary")

	fx.h.Report(codexKey, Event{Kind: TurnEnded, Status: "interrupted"})
	approveStateOf(fx.h).wg.Wait()
	mustApproveEqual(t, fx.h.Sessions.Calls(codexKey), 0, "Calls after TurnEnded")
	mustApproveEqual(t, runner.answerCount(), countBeforeC1+1, "cancelled Ask did not call Runner.Answer")

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
	mustApproveEqual(t, consumeGrantedApproval(fx.h, s, "Bash", digest, "echo x"), false, "consumeGrantedApproval without audit")

	seamSQL(t, fx.db, "UPDATE approval SET state = 'pending' WHERE card = '"+string(card)+"'")
	cause := fx.owner1.Sign(marshalApproveJSON(proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: approveTestID(92), Timestamp: seamStart, Group: proto.ID(fx.groupID)},
		Msg:         card,
		Action:      "approve",
	}))
	_, errLate := applyLateApprovalDecision(fx.h, fx.homeKey.ID(), "claude", proto.InboxItem{Cause: &cause}, it)
	mustApproveErr(t, errLate)

	seamSQL(t, fx.db, "DROP TABLE device_log", "DROP TABLE approval")
	mustApproveEqual(t, validInStoreLog(fx.db, fx.owner1.ID(), fx.owner1.ID()), false, "validInStoreLog without device_log")
	mustApproveEqual(t, consumeGrantedApproval(fx.h, s, "Bash", digest, "echo x"), false, "consumeGrantedApproval without approval")

	res := fx.mustCallTool(t.Context(), 2, `{"tool_name":"Bash","input":{"command":"echo x"},"tool_use_id":"u_no_tbl"}`)
	mustApproveEqual(t, res, ApprovalResult{Behavior: "deny", Message: "The approval request was refused: internal: failed to store approval record"}, "missing approval table")
	mustApproveErr(t, fx.h.DeleteTaskApprovals(fx.groupID, fx.taskID))
}
