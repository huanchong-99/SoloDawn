package host

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	testGroup1  = "AAAAAAAAAAAAAAAAAAAAAQ"
	testGroup2  = "AAAAAAAAAAAAAAAAAAAAAg"
	testAI1     = "BBBBBBBBBBBBBBBBBBBBBQ"
	testAI2     = "BBBBBBBBBBBBBBBBBBBBBg"
	testSec     = "CCCCCCCCCCCCCCCCCCCCCQ"
	testHuman   = "CCCCCCCCCCCCCCCCCCCCCg"
	testTask1   = "DDDDDDDDDDDDDDDDDDDDDQ"
	testTask2   = "DDDDDDDDDDDDDDDDDDDDDg"
	testMsg1    = "EEEEEEEEEEEEEEEEEEEEEQ"
	testMsg2    = "EEEEEEEEEEEEEEEEEEEEEg"
	testCall1   = "FFFFFFFFFFFFFFFFFFFFFQ"
	testCard1   = "GGGGGGGGGGGGGGGGGGGGGQ"
	testStartMS = int64(1763600000000)
)

func testSeedKey(b byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize)))
}

func testCanonicalID(b byte) proto.ID {
	var raw [16]byte
	raw[0] = b
	return proto.ID(base64.RawURLEncoding.EncodeToString(raw[:]))
}

type recordedTurn struct {
	host *Host
	key  SessionKey
	text string
	turn Turn
}

type fakeDeliverRuntime struct {
	mu      sync.Mutex
	turns   []recordedTurn
	closed  []SessionKey
	err     error
	keepRun bool
	onTurn  func(h *Host, key SessionKey)
	onClose func(h *Host, key SessionKey)
}

func (r *fakeDeliverRuntime) Turn(_ context.Context, h *Host, key SessionKey, text string, turn Turn) error {
	r.mu.Lock()
	err, keep, hook := r.err, r.keepRun, r.onTurn
	r.mu.Unlock()
	if err == nil {
		r.advanceState(h, key, keep)
		if hook != nil {
			hook(h, key)
		}
	}
	r.mu.Lock()
	r.turns = append(r.turns, recordedTurn{host: h, key: key, text: text, turn: turn})
	r.mu.Unlock()
	return err
}

func (r *fakeDeliverRuntime) advanceState(h *Host, key SessionKey, keepWorking bool) {
	s, ok := h.Sessions.Get(key)
	if !ok {
		return
	}
	if s.State == Dormant {
		h.Sessions.Move(key, Dormant, Working)
	}
	if !keepWorking {
		if cur, ok := h.Sessions.Get(key); ok && cur.State == Working {
			h.Sessions.Move(key, Working, Waiting)
		}
	}
}

func (r *fakeDeliverRuntime) Answer(*Host, SessionKey, string, bool) error { return nil }

func (r *fakeDeliverRuntime) Close(h *Host, key SessionKey) {
	r.mu.Lock()
	hook := r.onClose
	r.closed = append(r.closed, key)
	r.mu.Unlock()
	if hook != nil {
		hook(h, key)
	}
	if s, ok := h.Sessions.Get(key); ok && s.State != Dormant {
		h.Sessions.Move(key, s.State, Dormant)
	}
}

func (r *fakeDeliverRuntime) closedKeys() []SessionKey {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.closed)
}

func (r *fakeDeliverRuntime) recorded() []recordedTurn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.turns)
}

type syncAnswer struct {
	raw json.RawMessage
	err error
}

type fakeDeliverHome struct {
	mu       sync.Mutex
	done     chan struct{}
	acks     []proto.Ack
	cmds     []proto.Cmd
	syncs    []proto.Sync
	cmdAns   string
	cmdErr   error
	onCmd    func(ctx context.Context, cmd proto.Cmd) (json.RawMessage, error)
	syncMode bool
	started  chan proto.Sync // single reader: test sync expectation helpers
	answers  chan syncAnswer // single reader: waitSyncAnswer
}

func newFakeDeliverHome() *fakeDeliverHome {
	return &fakeDeliverHome{
		done:    make(chan struct{}),
		cmdAns:  `{"re":1,"ok":true}`,
		started: make(chan proto.Sync, 8),
		answers: make(chan syncAnswer, 8),
	}
}

func (f *fakeDeliverHome) Send(frame any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ack, ok := frame.(proto.Ack); ok {
		f.acks = append(f.acks, ack)
	}
	return nil
}

func (f *fakeDeliverHome) Path() string                 { return "local" }
func (f *fakeDeliverHome) PathChanged() <-chan struct{} { return nil }
func (f *fakeDeliverHome) Done() <-chan struct{}        { return f.done }

func (f *fakeDeliverHome) closeConn() {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
}

func (f *fakeDeliverHome) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.cmds) + len(f.syncs) + 1)
	req := build(id)
	syncMode := f.syncMode
	ansText, ansErr := f.cmdAns, f.cmdErr
	cmdHook := f.onCmd
	f.recordReqLocked(req)
	f.mu.Unlock()
	if syncReq, ok := req.(proto.Sync); ok && syncMode {
		return f.waitSyncAnswer(ctx, syncReq)
	}
	if syncReq, ok := req.(proto.Sync); ok {
		raw := marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: id, OK: true}, Group: syncReq.Group, Seq: syncReq.Since})
		return json.RawMessage(raw), nil
	}
	if cmdReq, ok := req.(proto.Cmd); ok && cmdHook != nil {
		return cmdHook(ctx, cmdReq)
	}
	return json.RawMessage(ansText), ansErr
}

func (f *fakeDeliverHome) recordReqLocked(req any) {
	switch v := req.(type) {
	case proto.Sync:
		f.syncs = append(f.syncs, v)
	case proto.Cmd:
		f.cmds = append(f.cmds, v)
	}
}

func (f *fakeDeliverHome) waitSyncAnswer(ctx context.Context, req proto.Sync) (json.RawMessage, error) {
	f.started <- req
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case ans := <-f.answers:
		return ans.raw, ans.err
	}
}

func (f *fakeDeliverHome) ackList() []proto.Ack {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.acks)
}

func (f *fakeDeliverHome) ackCountFor(item int64) int {
	n := 0
	for _, a := range f.ackList() {
		if a.Item == item {
			n++
		}
	}
	return n
}

func (f *fakeDeliverHome) cmdPayloads() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.cmds))
	for i, c := range f.cmds {
		out[i] = c.Env.Payload
	}
	return out
}

func (f *fakeDeliverHome) taskPayloads() []string {
	return slices.DeleteFunc(f.cmdPayloads(), func(p string) bool { return !strings.HasPrefix(p, `{"t":"task"`) })
}

func (f *fakeDeliverHome) syncSinces() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int64, len(f.syncs))
	for i, s := range f.syncs {
		out[i] = s.Since
	}
	return out
}

type deliverRig struct {
	t         *testing.T
	db        *store.Store
	h         *Host
	ownerKey  identity.Key
	hostKey   identity.Key
	homeKey   identity.Key
	home2Key  identity.Key
	extraKey  identity.Key
	home      *fakeDeliverHome
	runtime   *fakeDeliverRuntime
	folder    string // the workspace of every AI the rig joins
	composeMu sync.Mutex
	composeE  error
	onCompose func(s Session, items []json.RawMessage)
	hold      atomic.Pointer[func(*Host, SessionKey) (string, bool)]
}

func newDeliverRig(t *testing.T, disk bool) *deliverRig {
	t.Helper()
	path := ":memory:"
	if disk {
		path = filepath.Join(t.TempDir(), "node.db")
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return newDeliverRigOnStore(t, db)
}

func newDeliverRigOnStore(t *testing.T, db *store.Store) *deliverRig {
	t.Helper()
	rig := &deliverRig{
		t:        t,
		db:       db,
		ownerKey: testSeedKey(11),
		hostKey:  testSeedKey(12),
		homeKey:  testSeedKey(13),
		home2Key: testSeedKey(14),
		extraKey: testSeedKey(15),
		home:     newFakeDeliverHome(),
		runtime:  &fakeDeliverRuntime{},
		folder:   t.TempDir(),
	}
	rig.installHooks()
	clock := func() time.Time { return time.UnixMilli(testStartMS) }
	h, _, err := New(db, rig.hostKey, clock)
	if err != nil {
		t.Fatal(err)
	}
	rig.h = h
	t.Cleanup(func() {
		rig.home.closeConn()
		h.Settle()
	})
	return rig
}

func (r *deliverRig) installHooks() {
	prevRunner, prevCompose, prevHolds := Runner, Compose, Holds
	Holds = map[string]func(*Host, SessionKey) (string, bool){"rig": r.held}
	Runner = r.runtime
	Compose = func(_ *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
		r.composeMu.Lock()
		hook, cErr := r.onCompose, r.composeE
		r.composeMu.Unlock()
		if hook != nil {
			hook(s, items)
		}
		if cErr != nil {
			return "", Turn{}, cErr
		}
		return string(s.Kind) + ":" + strconv.Itoa(len(items)), Turn{Kind: kind, Thread: s.Task}, nil
	}
	r.t.Cleanup(func() {
		Runner = prevRunner
		Compose = prevCompose
		Holds = prevHolds
	})
}

// held is the one entry of Holds, there before the host starts: its goroutines
// read that map without a lock. A test sets what it answers with setHold.
func (r *deliverRig) held(h *Host, key SessionKey) (string, bool) {
	if hold := r.hold.Load(); hold != nil {
		return (*hold)(h, key)
	}
	return "", false
}

func (r *deliverRig) setHold(hold func(*Host, SessionKey) (string, bool)) { r.hold.Store(&hold) }

func (r *deliverRig) waitUntil(desc string, ok func() bool) {
	r.t.Helper()
	poll(r.t, 5*time.Second, desc, func() string {
		if ok() {
			return ""
		}
		return fmt.Sprintf("turns=%d acks=%d sessions=%s", len(r.runtime.recorded()), len(r.home.ackList()), sessionStates(r.h))
	})
}

func (r *deliverRig) waitGroupLive(group string) {
	r.t.Helper()
	d := deliv(r.h)
	r.waitUntil("group "+group+" live", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		gs := d.groups[group]
		return gs != nil && gs.live && !gs.syncing
	})
}

func (r *deliverRig) waitSyncStopped(group string) {
	r.t.Helper()
	d := deliv(r.h)
	r.waitUntil("group "+group+" sync stopped", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		gs := d.groups[group]
		return gs != nil && !gs.syncing
	})
}

func (r *deliverRig) waitAIUnbusy(group, member string) {
	r.t.Helper()
	d := deliv(r.h)
	key := aiGroupKey{group: group, member: member}
	r.waitUntil("ai "+member+" unbusy", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return !d.busy[key]
	})
}

func (r *deliverRig) setSetting(key string, val any) {
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)", key, marshalWire(val))
		return err
	}
	if err := r.db.Write(write); err != nil {
		r.t.Fatal(err)
	}
}

func (r *deliverRig) seedGroupAndMembers() {
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "Proj"}})
	r.putGroup(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1},
		Name:      "Proj",
		Home:      r.homeKey.ID(), Secretary: testSec,
	})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, 2)
	r.putAIMember(testAI2, "Codex", r.hostKey.ID(), false, 3)
	r.putAIMember(testSec, "Sec", r.hostKey.ID(), true, 4)
	r.h.Attach(r.homeKey.ID(), r.home)
	r.waitGroupLive(testGroup1)
}

func (r *deliverRig) putGroup(grp proto.GroupRow) {
	r.h.Rows.Put(grp.ID, "group", grp.ID, json.RawMessage(marshalWire(grp)))
}

func (r *deliverRig) putAIMember(id, name, hostID string, sec bool, seq int64) {
	m := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: seq}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: name},
		CLI:       "claude", Host: hostID, HostName: "n", Secretary: sec, Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local",
	}
	r.h.Rows.Put(testGroup1, "member", id, json.RawMessage(marshalWire(m)))
	if hostID == r.hostKey.ID() {
		r.joined(id, name, sec)
	}
}

// joined records the group session of a local AI in the rig's folder, as
// host.join does.
func (r *deliverRig) joined(member, name string, sec bool) {
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: member}, Kind: GroupSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: name, Secretary: sec})
}

func (r *deliverRig) putTask(task proto.TaskRow) {
	if task.Depends == nil {
		task.Depends = []string{}
	}
	r.h.Rows.Put(task.Group, "task", task.ID, json.RawMessage(marshalWire(task)))
}

func (r *deliverRig) makeTextMsg(id proto.ID, thread *string, text string) *proto.MsgRow {
	return &proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(id), Seq: 10},
		Pos:       10, Group: testGroup1, Thread: thread, Main: thread == nil,
		By: testHuman, Kind: "text", Timestamp: testStartMS, Text: text, Mentions: []string{}, Body: json.RawMessage(`{}`),
	}
}

func (r *deliverRig) pushItem(_ identity.Key, connHome string, group proto.ID, it proto.Item, msg *proto.MsgRow) {
	wire := proto.InboxItem{Item: it}
	if msg != nil {
		wire.Msg = json.RawMessage(marshalWire(msg))
	}
	frame := marshalWire(proto.Inbox{Type: "inbox", Group: group, Items: []proto.InboxItem{wire}})
	r.h.Push(connHome, json.RawMessage(frame))
}

type auditSummary struct {
	what   string
	item   int64
	home   string
	agent  string
	detail string
	by     string
}

func (r *deliverRig) audits() []auditSummary {
	var list []auditSummary
	read := func(tx *store.Tx) error {
		var raw string
		err := tx.QueryRow(`SELECT coalesce(json_group_array(json_object(
			'what', what, 'item', coalesce(item, 0), 'home', coalesce(home, ''),
			'agent', coalesce(agent, ''), 'detail', detail, 'by', coalesce(by, '')
		)), '[]') FROM (SELECT * FROM audit ORDER BY id)`).Scan(&raw)
		if err != nil {
			return err
		}
		var rows []struct {
			What   string `json:"what"`
			Item   int64  `json:"item"`
			Home   string `json:"home"`
			Agent  string `json:"agent"`
			Detail string `json:"detail"`
			By     string `json:"by"`
		}
		if err := json.Unmarshal([]byte(raw), &rows); err != nil {
			return err
		}
		for _, row := range rows {
			list = append(list, auditSummary{row.What, row.Item, row.Home, row.Agent, row.Detail, row.By})
		}
		return nil
	}
	if err := r.db.Write(read); err != nil {
		r.t.Fatal(err)
	}
	return list
}

type safeLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeLogBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeLogBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func captureLogs(t *testing.T) *safeLogBuffer {
	t.Helper()
	prevOut, prevFlags := log.Writer(), log.Flags()
	out := &safeLogBuffer{}
	log.SetOutput(out)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return out
}

func assertDeliverRegistrations(t *testing.T) {
	t.Helper()
	checks := []bool{Attached["deliver"] != nil, Settled["deliver"] != nil, Pushes["inbox"] != nil, Pushes["rows"] != nil, Pushes["reset"] != nil, Deliver != nil}
	if slices.Contains(checks, false) {
		t.Fatal("deliver registrations missing")
	}
}

func TestRegistrationsAndHostIsolation(t *testing.T) {
	assertDeliverRegistrations(t)
	r1 := newDeliverRig(t, false)
	db2, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	h2, _, err := New(db2, testSeedKey(19), r1.h.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h2.Settle() })
	r1.seedGroupAndMembers()
	msg := r1.makeTextMsg(testMsg1, nil, "hello")
	it := proto.Item{Item: 1, AI: testAI1, Why: "mention", Msg: testMsg1}
	r1.pushItem(r1.homeKey, r1.homeKey.ID(), testGroup1, it, msg)
	r1.waitUntil("1 turn and ack on r1", func() bool { return len(r1.runtime.recorded()) == 1 && r1.home.ackCountFor(1) == 1 })
	r1.waitAIUnbusy(testGroup1, testAI1)
	turns := r1.runtime.recorded()
	if turns[0].host != r1.h || len(deliv(h2).groups) != 0 || len(r1.home.syncSinces()) != 1 {
		t.Fatalf("isolation or live sync check broken: turns=%+v h2Groups=%d syncs=%v", turns, len(deliv(h2).groups), r1.home.syncSinces())
	}
	r1.h.Settle()
}

func TestAttachGroupsCancelsCleanupAndUnsyncedPush(t *testing.T) {
	r := newDeliverRig(t, false)
	closed := newFakeDeliverHome()
	closed.closeConn()
	r.h.Attach(r.homeKey.ID(), closed)
	r.h.Attach(r.homeKey.ID(), r.home)
	d := deliv(r.h)
	d.mu.Lock()
	cancelsAfterEmpty := len(d.cancels)
	d.mu.Unlock()
	if len(closed.syncSinces()) != 0 || len(r.home.syncSinces()) != 0 || cancelsAfterEmpty != 0 {
		t.Fatalf("closed or empty attach started sync or cancel entry: cancels=%d", cancelsAfterEmpty)
	}
	verifyUnsyncedInboxTriggersSyncAndDelivers(t, r)
	verifySecondUnsyncedGroupAndCancelCleanup(t, r)
}

func verifyUnsyncedInboxTriggersSyncAndDelivers(t *testing.T, r *deliverRig) {
	t.Helper()
	r.home.syncMode = true
	r.joined(testAI1, "C", false)
	// Push inbox for unsynced testGroup1 on an open connection whose d.cancels entry is nil.
	it := proto.Item{Item: 10, AI: testAI1, Why: "assign", Task: testTask1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil)
	req := <-r.home.started
	wantSync := proto.Sync{Request: req.Request, Group: testGroup1, Since: 0, StateOnly: true}
	if req != wantSync || len(r.home.ackList()) != 0 || len(r.audits()) != 0 {
		t.Fatalf("unexpected pre-sync state: req=%+v acks=%v audits=%v", req, r.home.ackList(), r.audits())
	}
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "P1", Home: r.homeKey.ID()}))
	aiRaw := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	tRaw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "T", Origin: "brief", State: "active", Step: "work", Assignee: testAI1, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpRaw, aiRaw, tRaw}})))
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 5}))}
	r.waitUntil("item 10 delivered", func() bool { return r.home.ackCountFor(10) == 1 && len(r.runtime.recorded()) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)
	if len(r.home.syncSinces()) != 1 {
		t.Fatalf("expected single sync while syncing, got %v", r.home.syncSinces())
	}
}

func verifySecondUnsyncedGroupAndCancelCleanup(t *testing.T, r *deliverRig) {
	t.Helper()
	// Push rows for unsynced testGroup2 on the same connection while d.cancels[home] already exists.
	m2 := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup2, Kind: "ai", User: r.ownerKey.ID(), Name: "C2"},
		CLI:       "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local",
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup2, Rows: []json.RawMessage{json.RawMessage(marshalWire(m2))}})))
	req2 := <-r.home.started
	if req2.Group != testGroup2 {
		t.Fatalf("expected sync for %s, got %+v", testGroup2, req2)
	}
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 2, OK: true}, Group: testGroup2, Seq: 2}))}
	r.waitGroupLive(testGroup2)

	// Attach a second connection (superseding r.home), then close r.home.done: d.cancels[home] must retain second's entry until second closes.
	d := deliv(r.h)
	d.mu.Lock()
	d.groupLocked(string(testCanonicalID(96)), r.home2Key.ID())
	d.mu.Unlock()
	r.h.Push(r.extraKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: string(testCanonicalID(95)), Rows: []json.RawMessage{}})))
	r.setSetting("homes", []proto.Home{
		{Group: testGroup2, Home: r.homeKey.ID(), Name: "Two"},
		{Group: testCanonicalID(99), Home: r.home2Key.ID(), Name: "Other"},
	})
	second := newFakeDeliverHome()
	second.syncMode = true
	r.h.Attach(r.homeKey.ID(), second)
	s1 := <-second.started
	second.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: s1.Group, Seq: s1.Since}))}
	s2 := <-second.started
	second.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 2, OK: true}, Group: s2.Group, Seq: s2.Since}))}
	if s1.Group != testGroup1 || s2.Group != testGroup2 {
		t.Fatalf("unexpected synced groups: %s, %s", s1.Group, s2.Group)
	}
	r.waitGroupLive(testGroup2)
	stale := newFakeDeliverHome()
	stale.closeConn()
	staleCtx, staleCancel := context.WithCancel(context.Background())
	watchHomeConn(r.h, d, r.homeKey.ID(), stale, &homeSync{ctx: staleCtx, cancel: staleCancel})
	d.mu.Lock()
	stillPresent := d.cancels[r.homeKey.ID()] != nil
	d.mu.Unlock()
	if staleCtx.Err() == nil || !stillPresent {
		t.Fatal("closing superseded connection failed to cancel stale ctx or deleted active cancel entry")
	}
	second.closeConn()
	r.waitUntil("cancels cleaned up on conn close", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.cancels) == 0
	})
}

func TestSyncCursorResetDeltaAndOvertakenRows(t *testing.T) {
	r := newDeliverRig(t, false)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "One"}})
	r.joined(testAI1, "C1", false)
	r.joined(testAI2, "C2", false)
	first := newFakeDeliverHome()
	first.syncMode = true
	r.h.Attach(r.homeKey.ID(), first)
	<-first.started
	t.Cleanup(func() { first.closeConn() })

	second := newFakeDeliverHome()
	second.syncMode = true
	r.h.Attach(r.homeKey.ID(), second)
	<-second.started
	t.Cleanup(func() { second.closeConn() })
	first.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 99}))}

	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Reset{Type: "reset", Group: testGroup1})))
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 4}, Name: "P", Home: r.homeKey.ID()}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpRaw}})))
	second.answers <- syncAnswer{err: errors.New("broken")}
	r.waitSyncStopped(testGroup1)

	verifyOvertakenSyncedAndMonotonicCursor(t, r)
}

func verifyOvertakenSyncedAndMonotonicCursor(t *testing.T, r *deliverRig) {
	t.Helper()
	// Third attach asks since 0; synced{10} arrives before member rows (seq 6, 7) and task rows (seq 8, 9 < 10).
	third := newFakeDeliverHome()
	third.syncMode = true
	r.h.Attach(r.homeKey.ID(), third)
	t.Cleanup(func() { third.closeConn() })
	req3 := <-third.started
	if req3.Since != 0 {
		t.Fatalf("after interrupted reset got since=%d, want 0", req3.Since)
	}
	assignIt := proto.Item{Item: 30, AI: testAI1, Why: "assign", Task: testTask1}
	resumeIt := proto.Item{Item: 31, AI: testAI2, Why: "resume", Task: testTask2}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, assignIt, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, resumeIt, nil)
	third.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 10}))}
	r.waitGroupLive(testGroup1)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.audits()) != 0 || len(third.ackList()) != 0 {
		t.Fatalf("absent member rows caused refusal or ack: audits=%+v", r.audits())
	}
	// Push member rows (seq 6, 7) while task rows are still absent: checkStillCurrent must hold, not refuse.
	ai1Raw := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 6}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C1"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	ai2Raw := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI2, Seq: 7}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C2"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{ai1Raw, ai2Raw}})))
	Deliver(r.h, testGroup1, testAI1)
	if len(r.audits()) != 0 || len(third.ackList()) != 0 {
		t.Fatalf("absent task rows caused refusal or ack: audits=%+v", r.audits())
	}
	// Push task rows with seq 8, 9 (< 10): items now process, and cursor remains at 10.
	t1Raw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 8}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "work", Assignee: testAI1, Depends: []string{}}))
	t2Raw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 9}, Group: testGroup1, Num: 2, Title: "T2", Origin: "brief", State: "active", Step: "work", Assignee: testAI2, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{t1Raw, t2Raw}})))
	r.waitUntil("both delayed items acknowledged", func() bool { return len(third.ackList()) == 2 && len(r.runtime.recorded()) == 2 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.waitAIUnbusy(testGroup1, testAI2)
	assertLiveCursorAndFailedDelta(t, r)
}

func assertLiveCursorAndFailedDelta(t *testing.T, r *deliverRig) {
	t.Helper()
	// Reconnect must ask since 10 (not regressed to 9); live row seq 12 advances to 12; dup 12 and 11 ignored.
	fourth := newFakeDeliverHome()
	fourth.syncMode = true
	r.h.Attach(r.homeKey.ID(), fourth)
	t.Cleanup(func() { fourth.closeConn() })
	if req4 := <-fourth.started; req4.Since != 10 {
		t.Fatalf("cursor regressed below synced S: got since=%d, want 10", req4.Since)
	}
	fourth.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 10}))}
	r.waitGroupLive(testGroup1)

	putSeq := func(seq int64, name string) {
		m := proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: seq}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: name},
			CLI:       "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local",
		}
		r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{json.RawMessage(marshalWire(m))}})))
	}
	putSeq(12, "first-12")
	putSeq(12, "dup-12")
	putSeq(11, "old-11")
	var got proto.AIMemberRow
	raw, _ := r.h.Rows.Get(testGroup1, "member", testAI1)
	proto.Decode(raw, &got)
	if got.Name != "first-12" {
		t.Fatalf("row overwritten by equal or lower seq: %q", got.Name)
	}
	assertMalformedSynced(t, r)
}

func assertMalformedSynced(t *testing.T, r *deliverRig) {
	t.Helper()
	fifth := newFakeDeliverHome()
	fifth.syncMode = true
	r.h.Attach(r.homeKey.ID(), fifth)
	t.Cleanup(func() { fifth.closeConn() })
	if req5 := <-fifth.started; req5.Since != 12 {
		t.Fatalf("after live rows got since=%d, want 12", req5.Since)
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 39, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "wait-sync"))
	fifth.answers <- syncAnswer{raw: json.RawMessage(`{"t":"synced","re":1,"g":"` + testGroup1 + `","seq":99}`)}
	r.waitSyncStopped(testGroup1)
	r.waitAIUnbusy(testGroup1, testAI1)
	d := deliv(r.h)
	d.mu.Lock()
	cur, live := d.groups[testGroup1].cursor, d.groups[testGroup1].live
	d.mu.Unlock()
	if cur != 12 || live || len(r.runtime.recorded()) != 2 {
		t.Fatalf("invalid synced frame accepted: cursor=%d live=%v turns=%d", cur, live, len(r.runtime.recorded()))
	}
}

func TestGroupResyncAfterPostSyncedResetAndFailedSync(t *testing.T) {
	// 1) A reset handled after synced, followed by group + member rows and a mention, starts a new sync and delivers the turn.
	r := newDeliverRig(t, false)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "Proj"}})
	r.joined(testAI1, "C1", false)
	r.home.syncMode = true
	r.h.Attach(r.homeKey.ID(), r.home)
	req1 := <-r.home.started
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: req1.Group, Seq: 7}))}
	r.waitGroupLive(testGroup1)

	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Reset{Type: "reset", Group: testGroup1})))
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "Proj", Home: r.homeKey.ID()}))
	aiRaw := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C1"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpRaw, aiRaw}})))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 401, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "after-reset"))
	req2 := <-r.home.started
	if req2.Since != 0 || len(r.home.syncSinces()) != 2 {
		t.Fatalf("expected single resync from 0 after post-synced reset, got req=%+v sinces=%v", req2, r.home.syncSinces())
	}
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 2, OK: true}, Group: testGroup1, Seq: 2}))}
	r.waitUntil("post-reset mention delivered", func() bool { return len(r.runtime.recorded()) == 1 && r.home.ackCountFor(401) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)

	verifyResyncAfterFailedSync(t, r)
}

func verifyResyncAfterFailedSync(t *testing.T, r *deliverRig) {
	t.Helper()
	// 2) A sync answered with an error, followed by a mention item, sends a fresh sync from the kept cursor and delivers once answered.
	second := newFakeDeliverHome()
	second.syncMode = true
	r.h.Attach(r.homeKey.ID(), second)
	t.Cleanup(func() { second.closeConn() })
	s1 := <-second.started
	second.answers <- syncAnswer{err: errors.New("temporary sync failure")}
	r.waitSyncStopped(testGroup1)

	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 402, AI: testAI1, Why: "mention", Msg: testMsg2}, r.makeTextMsg(testMsg2, nil, "after-failed-sync"))
	s2 := <-second.started
	if s1.Since != 2 || s2.Since != 2 {
		t.Fatalf("expected both syncs to ask since kept cursor 2, got %d then %d", s1.Since, s2.Since)
	}
	second.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 2, OK: true}, Group: testGroup1, Seq: 2}))}
	r.waitUntil("post-error mention delivered", func() bool { return len(r.runtime.recorded()) == 2 && second.ackCountFor(402) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)
}

// A batch that cannot go is held again after its step. A push for another AI
// while that step runs must still make the step look again: the item would
// otherwise wait for the next push (CI flake of 11-26, item 912).
func TestSweepRechecksAnAIWhoseBatchIsInFlight(t *testing.T) {
	logs := captureDiag(t)
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	// The sweep after the attach would otherwise recheck the secretary.
	logs.wait(t, "bg.stop node="+r.hostKey.ID()[:8]+" name=group_state g="+testGroup1)
	var composing atomic.Bool
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	r.composeMu.Lock()
	r.composeE = errors.New("compose failed")
	r.onCompose = func(Session, []json.RawMessage) {
		composing.Store(true)
		<-release
	}
	r.composeMu.Unlock()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 920, AI: testSec, Why: "notice", Code: "c"}, nil)
	r.waitUntil("item 920 in compose", composing.Load)
	r.composeMu.Lock()
	r.composeE, r.onCompose = nil, nil
	r.composeMu.Unlock()

	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 921, AI: testAI1, Why: "notice", Code: "c"}, nil)
	d := deliv(r.h)
	secKey := aiGroupKey{group: testGroup1, member: testSec}
	r.waitUntil("the sweep after item 921 to recheck the secretary", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.recheck[secKey]
	})
	unblock()
	r.waitUntil("item 920 delivered", func() bool { return r.home.ackCountFor(920) == 1 && r.home.ackCountFor(921) == 1 })
	r.waitAIUnbusy(testGroup1, testSec)
}

func TestDuplicateWhileTakingSessionItems(t *testing.T) {
	// Duplicate mention arriving while the first copy is inside Compose -> 1 turn, 1 delivered row, 2 acks.
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	msg := r.makeTextMsg(testMsg1, nil, "once")
	it := proto.Item{Item: 900, AI: testAI1, Why: "mention", Msg: testMsg1}
	resent := false
	r.onCompose = func(_ Session, _ []json.RawMessage) {
		if !resent {
			resent = true
			r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, msg)
		}
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, msg)
	r.waitUntil("2 acks and 1 turn for item 900", func() bool { return r.home.ackCountFor(900) == 2 && len(r.runtime.recorded()) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)
	if len(r.audits()) != 1 || r.audits()[0].what != AuditDelivered || len(deliv(r.h).heldKeys()) != 0 {
		t.Fatalf("duplicate session item result: turns=%d audits=%+v held=%d", len(r.runtime.recorded()), r.audits(), len(deliv(r.h).heldKeys()))
	}
}

func TestNonBlockingPushHandlersAndLockOrder(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "work", Assignee: testAI1})

	composing := make(chan struct{}, 1) // single reader: main test goroutine
	pushDone := make(chan struct{})     // closed by main test goroutine once later pushes finish
	r.composeMu.Lock()
	r.onCompose = func(s Session, _ []json.RawMessage) {
		if s.Kind == TaskSession {
			composing <- struct{}{}
			<-pushDone
		}
	}
	r.composeMu.Unlock()
	// Push assign (Compose, which asks the home, blocks on a background goroutine); Push must return immediately.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 40, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	<-composing
	// While it is blocked, pushing rows and another inbox item must return without deadlocking.
	noteRaw := json.RawMessage(`{"r":"note","id":"n1","seq":11}`)
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{noteRaw}})))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 41, AI: testSec, Why: "notice", Code: "ping"}, nil)
	close(pushDone)

	r.waitUntil("both turns completed", func() bool { return len(r.runtime.recorded()) == 2 })
	verifyHostCommandIsBounded(t, r)
	verifyHoldLockNotUnderDeliveryMutex(t, r)
}

// A command the host sends by itself waits for its home one minute at most,
// and only a failed one is logged.
func verifyHostCommandIsBounded(t *testing.T, r *deliverRig) {
	t.Helper()
	logs := captureLogs(t)
	var hasDeadline bool
	var remaining time.Duration
	r.home.mu.Lock()
	r.home.onCmd = func(ctx context.Context, cmd proto.Cmd) (json.RawMessage, error) {
		if strings.HasPrefix(cmd.Env.Payload, `{"t":"task"`) {
			dl, ok := ctx.Deadline()
			hasDeadline, remaining = ok, time.Until(dl)
		}
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	r.home.mu.Unlock()
	err := sendHostCommand(context.Background(), r.h, r.homeKey.ID(), marshalWire(hostTaskCommand(r.h, testGroup1, "resume", testTask1)))
	if err != nil || !hasDeadline || remaining <= 30*time.Second || remaining > time.Minute {
		t.Fatalf("expected 1m bounded context on h.Command, got %v ok=%v remaining=%v", err, hasDeadline, remaining)
	}
	if strings.Contains(logs.String(), "host: command to") {
		t.Fatalf("a command the home took was logged: %q", logs.String())
	}
	r.home.mu.Lock()
	r.home.onCmd = func(context.Context, proto.Cmd) (json.RawMessage, error) { return nil, errors.New("home down") }
	r.home.mu.Unlock()
	err = sendHostCommand(context.Background(), r.h, r.homeKey.ID(), marshalWire(hostTaskCommand(r.h, testGroup1, "resume", testTask1)))
	if err == nil || !strings.Contains(logs.String(), "host: command to "+r.homeKey.ID()+": "+err.Error()) {
		t.Fatalf("a failed command = %v, logs %q", err, logs.String())
	}
	r.home.mu.Lock()
	r.home.onCmd = nil
	r.home.mu.Unlock()
}

func verifyHoldLockNotUnderDeliveryMutex(t *testing.T, r *deliverRig) {
	t.Helper()
	var extMu sync.Mutex
	var first atomic.Bool
	r.setHold(func(*Host, SessionKey) (string, bool) {
		extMu.Lock()
		defer extMu.Unlock()
		if first.CompareAndSwap(false, true) {
			r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 42, AI: testAI2, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "reentrant"))
		}
		return "", false
	})
	extMu.Lock()
	done := make(chan struct{})
	go func() {
		Deliver(r.h, testGroup1, testAI2)
		close(done)
	}()
	extMu.Unlock()
	<-done
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 43, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "check"))
	r.waitUntil("4 turns total", func() bool { return len(r.runtime.recorded()) == 4 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.waitAIUnbusy(testGroup1, testAI2)
}

func TestArrivalRefusalsNotHostedAndOpenWaitsWithoutCause(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	// Malformed frames, empty groups, and invalid row headers are ignored.
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"reset","g":1}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"reset","g":""}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"rows","g":1}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"rows","g":"","rows":[]}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"inbox","g":"not-canonical","items":[]}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"inbox","g":"`+testGroup1+`","items":[123]}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"inbox","g":"`+testGroup1+`","items":[{"item":{"item":4,"why":"mention"}}]}`))
	checkRowHeaderEdges(t, r)

	// Remote AI, human, and gone member are refused as not_hosted_here.
	r.putAIMember(testAI2, "RemoteAI", r.extraKey.ID(), false, 10)
	human := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testHuman, Seq: 11}, Group: testGroup1, Kind: "human", User: r.ownerKey.ID(), Name: "Owner"}, Role: "owner", Online: true}
	r.h.Rows.Put(testGroup1, "member", testHuman, json.RawMessage(marshalWire(human)))
	goneID := testCanonicalID(88)
	r.h.Rows.Put(testGroup1, "member", string(goneID), json.RawMessage(marshalWire(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: string(goneID), Seq: 12}, Gone: true})))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 101, AI: testAI2, Why: "mention", Msg: testMsg1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 102, AI: testHuman, Why: "mention", Msg: testMsg1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 103, AI: goneID, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("3 refused acks", func() bool { return len(r.home.ackList()) == 3 })
	r.waitAIUnbusy(testGroup1, testAI2)
	r.waitAIUnbusy(testGroup1, testHuman)
	for _, a := range r.audits() {
		if a.what != AuditRefused || a.detail != "not_hosted_here" {
			t.Fatalf("unexpected refusal audit: %+v", a)
		}
	}
	r.h.Settle()
	verifyOpenWaitsAndCauselessDecisions(t)
}

func checkRowHeaderEdges(t *testing.T, r *deliverRig) {
	t.Helper()
	badRows := []json.RawMessage{
		json.RawMessage(`"not-object"`),
		json.RawMessage(`{"r":"","id":"x","seq":1}`),
		json.RawMessage(`{"r":"note","id":"","seq":1}`),
		json.RawMessage(`{"r":"note","id":"x","seq":0}`),
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: badRows})))
	d := deliv(r.h)
	d.mu.Lock()
	emptyGrp := d.groups[""]
	d.mu.Unlock()
	if _, ok := r.h.Rows.Get(testGroup1, "note", "x"); ok || len(r.h.Rows.Of(testGroup1, "")) != 0 || emptyGrp != nil {
		t.Fatal("invalid row header or empty group was stored")
	}
	checkCorruptRowReplacement(t, r)
}

func checkCorruptRowReplacement(t *testing.T, r *deliverRig) {
	t.Helper()
	r.h.Rows.Put(testGroup1, "note", "corrupt", json.RawMessage(`{bad`))
	validRows := []json.RawMessage{
		json.RawMessage(`{"r":"note","id":"n1","seq":1}`),
		json.RawMessage(`{"r":"note","id":"corrupt","seq":1}`),
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: validRows})))
	var replaced proto.RowHeader
	raw, ok := r.h.Rows.Get(testGroup1, "note", "corrupt")
	if !ok || proto.Decode(raw, &replaced) != nil || replaced.Seq != 1 {
		t.Fatalf("corrupt cached row not replaced: %s", raw)
	}
}

func verifyOpenWaitsAndCauselessDecisions(t *testing.T) {
	t.Helper()
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	gKey := SessionKey{Group: testGroup1, Member: testAI1}
	r.h.Sessions.Move(gKey, Dormant, Working)

	askCh, doneAsk := r.h.Await(testCall1, nil)
	defer doneAsk()
	checkReplyNonMatchingCases(t, r)
	callRef := testCall1
	replyMsg := &proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: testMsg1, Seq: 15},
		Pos:       15, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS,
		Text: "here", Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`),
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 310, AI: testAI1, Why: "reply", Msg: testMsg1}, replyMsg)
	if len(askCh) != 1 {
		t.Fatal("open ask did not receive reply")
	}

	// Open wait on decided item WITH NO cause: first attempt refused by check, second attempt accepted; audit By is home.
	attempts := 0
	appCh, doneApp := r.h.Await(testCard1, func(json.RawMessage) error {
		attempts++
		if attempts == 1 {
			return errors.New("bad_choice")
		}
		return nil
	})
	defer doneApp()
	dec1 := proto.Item{Item: 320, AI: testAI1, Why: "decided", Msg: testCard1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, dec1, nil)
	dec2 := dec1
	dec2.Item = 321
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, dec2, nil)
	if len(appCh) != 1 {
		t.Fatal("causeless decided item did not reach open wait")
	}
	wantAudits := []auditSummary{
		{what: AuditDelivered, item: 310, home: r.homeKey.ID(), agent: "claude", detail: "reply", by: r.homeKey.ID()},
		{what: AuditRefused, item: 320, home: r.homeKey.ID(), agent: "claude", detail: "bad_choice", by: r.homeKey.ID()},
		{what: AuditDelivered, item: 321, home: r.homeKey.ID(), agent: "claude", detail: "decided", by: r.homeKey.ID()},
	}
	if !slices.Equal(r.audits(), wantAudits) {
		t.Fatalf("audits=%+v, want %+v", r.audits(), wantAudits)
	}
	r.h.Sessions.Move(gKey, Working, Waiting)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("queued non-matching replies drained", func() bool {
		return r.home.ackCountFor(300) == 1 && r.home.ackCountFor(309) == 1 && len(deliv(r.h).heldKeys()) == 0
	})
	r.waitAIUnbusy(testGroup1, testAI1)
	r.h.Settle()
	verifyColdStartDrainOpenWait(t)
}

func checkReplyNonMatchingCases(t *testing.T, r *deliverRig) {
	t.Helper()
	callRef := testCall1
	cases := []*proto.MsgRow{
		nil,
		{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg2, Seq: 11}, Pos: 11, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)},
		{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg1, Seq: 12}, Pos: 12, Group: testGroup1, By: testSec, Kind: "text", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)},
		{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg1, Seq: 13}, Pos: 13, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: nil, Body: json.RawMessage(`{}`)},
	}
	for i, msg := range cases {
		it := proto.Item{Item: int64(300 + i), AI: testAI1, Why: "reply", Msg: testMsg1}
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, msg)
	}
	// Malformed Msg JSON is treated as no message and queued instead of refused.
	badMsgIt := proto.Item{Item: 309, AI: testAI1, Why: "reply", Msg: testMsg1}
	badWire := proto.InboxItem{Item: badMsgIt, Msg: json.RawMessage(`{"invalid":true}`)}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Inbox{Type: "inbox", Group: testGroup1, Items: []proto.InboxItem{badWire}})))
}

func verifyColdStartDrainOpenWait(t *testing.T) {
	t.Helper()
	// Decided item arriving before member row is cached is drained to open wait at take time even when group is paused.
	r := newDeliverRig(t, false)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "W"}})
	r.joined(testAI1, "C", false)
	r.home.syncMode = true
	r.h.Attach(r.homeKey.ID(), r.home)
	<-r.home.started

	waitCh, doneWait := r.h.Await(testCard1, nil)
	defer doneWait()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 330, AI: testAI1, Why: "decided", Msg: testCard1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 331, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "held"))
	grpPaused := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "W", Home: r.homeKey.ID(), Paused: &proto.GroupPause{By: testHuman, At: testStartMS}}))
	aiRow := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpPaused, aiRow}})))
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 2}))}
	r.waitUntil("open wait drained at take", func() bool { return len(waitCh) == 1 && r.home.ackCountFor(330) == 1 })
	if len(r.runtime.recorded()) != 0 || len(r.audits()) != 1 || r.audits()[0].agent != "claude" {
		t.Fatalf("unexpected cold-start drain state: turns=%d audits=%+v", len(r.runtime.recorded()), r.audits())
	}
	grpUnpaused := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 3}, Name: "W", Home: r.homeKey.ID()}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpUnpaused}})))
	r.waitUntil("mention delivered after unpause", func() bool { return len(r.runtime.recorded()) == 1 })
}

func TestRoutingHoldsJumpAndBatchSelection(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "review", Report: testMsg2, Assignee: testAI1, Reviewer: testAI2})

	// Causeless confirmed to secretary -> GroupSession; assign -> TaskSession; review -> ReviewSession; causeless decided -> GroupSession/TaskSession.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 501, AI: testSec, Why: "confirmed", Msg: testMsg1}, nil)
	r.waitUntil("sec turn", func() bool { return len(r.runtime.recorded()) == 1 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 502, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("assign turn", func() bool { return len(r.runtime.recorded()) == 2 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 503, AI: testAI2, Why: "review", Task: testTask1, Msg: testMsg2}, nil)
	r.waitUntil("review turn", func() bool { return len(r.runtime.recorded()) == 3 })

	tid := testTask1
	otherTid := testTask2
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 504, AI: testAI2, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, &tid, "in review thread"))
	r.waitUntil("thread mention turn", func() bool { return len(r.runtime.recorded()) == 4 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 505, AI: testAI2, Why: "mention", Msg: testMsg2}, r.makeTextMsg(testMsg2, &otherTid, "other thread"))
	r.waitUntil("other thread turn", func() bool { return len(r.runtime.recorded()) == 5 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 506, AI: testAI2, Why: "decided", Task: testTask1, Msg: testCard1}, nil)
	r.waitUntil("ai2 open review decided turn", func() bool { return len(r.runtime.recorded()) == 6 })
	r.waitAIUnbusy(testGroup1, testAI2)

	// Dormant ReviewSession falls back to GroupSession on decided, while thread mention still wakes ReviewSession.
	revKey := SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}
	r.h.Sessions.Move(revKey, Waiting, Dormant)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 507, AI: testAI2, Why: "decided", Task: testTask1, Msg: testCard1}, nil)
	r.waitUntil("ai2 dormant review decided fallback", func() bool { return len(r.runtime.recorded()) == 7 })
	r.waitAIUnbusy(testGroup1, testAI2)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 508, AI: testAI2, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "wake review"))
	r.waitUntil("ai2 dormant review mention", func() bool { return len(r.runtime.recorded()) == 8 })
	r.waitAIUnbusy(testGroup1, testAI2)

	// Dormant TaskSession still receives decided for its task; task without a session falls back to GroupSession.
	r.h.Sessions.Move(SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, Waiting, Dormant)
	r.h.Rows.Reset(testGroup1)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 16}, Name: "Proj", Home: r.homeKey.ID(), Secretary: testSec})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, 17)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 509, AI: testAI1, Why: "decided", Task: testTask1, Msg: testCard1}, nil)
	r.waitUntil("ai1 dormant task decided turn", func() bool { return len(r.runtime.recorded()) == 9 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 510, AI: testAI1, Why: "decided", Task: testTask2, Msg: testCard1}, nil)
	r.waitUntil("ai1 uncreated task decided fallback", func() bool { return len(r.runtime.recorded()) == 10 })
	r.waitAIUnbusy(testGroup1, testAI1)

	assertRoutedTurns(t, r.runtime.recorded())
	r.h.Settle()
	verifyHoldsBatchingAndJump(t)
	verifyEnqueueHeldOrdering(t)
}

func assertRoutedTurns(t *testing.T, turns []recordedTurn) {
	t.Helper()
	type wantTurn struct {
		key  SessionKey
		text string
	}
	want := []wantTurn{
		{SessionKey{Group: testGroup1, Member: testSec}, "group:1"},
		{SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, "task:1"},
		{SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}, "review:1"},
		{SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}, "review:1"},
		{SessionKey{Group: testGroup1, Member: testAI2}, "group:1"},
		{SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}, "review:1"},
		{SessionKey{Group: testGroup1, Member: testAI2}, "group:1"},
		{SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}, "review:1"},
		{SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, "task:1"},
		{SessionKey{Group: testGroup1, Member: testAI1}, "group:1"},
	}
	for i, w := range want {
		if turns[i].key != w.key || turns[i].text != w.text {
			t.Fatalf("turn %d got (%+v, %q), want (%+v, %q)", i, turns[i].key, turns[i].text, w.key, w.text)
		}
	}
}

func verifyHoldsBatchingAndJump(t *testing.T) {
	t.Helper()
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "paused", Step: "work", Assignee: testAI1})

	// Paused/failed task holds work items while notice delivers immediately.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 601, AI: testAI1, Why: "resume", Task: testTask1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 602, AI: testAI1, Why: "reviewed", Task: testTask1, Msg: testMsg2}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 603, AI: testSec, Why: "notice", Task: testTask1, Code: "quiet"}, nil)
	r.waitUntil("sec notice turn", func() bool { return len(r.runtime.recorded()) == 1 })
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 11}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "failed", Step: "work", Assignee: testAI1})
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("failed task delivered")
	}

	// Holds["cap"] holds AI1 and AI2; also queue a GroupSession mention for AI1 to verify selectEligibleBatch batches 601+602 into TaskSession (task:2) separately from GroupSession (group:1).
	var heldFlag atomic.Bool
	var holdChecks atomic.Int32
	heldFlag.Store(true)
	r.setHold(func(*Host, SessionKey) (string, bool) {
		holdChecks.Add(1)
		return "cap", heldFlag.Load()
	})
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 12}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "work", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 604, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "grp"))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 605, AI: testAI2, Why: "mention", Msg: testMsg2}, r.makeTextMsg(testMsg2, nil, "ai2"))
	r.waitUntil("held items checked", func() bool { return holdChecks.Load() >= 2 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.waitAIUnbusy(testGroup1, testAI2)
	gKey := SessionKey{Group: testGroup1, Member: testAI1}
	r.h.Sessions.Move(gKey, Dormant, Takeover)
	heldFlag.Store(false)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("AI2 woken", func() bool { return len(r.runtime.recorded()) == 2 })
	// AI1 is in Takeover so AI1 stays held, while Deliver walks other held keys and wakes AI2.
	if r.runtime.recorded()[1].key.Member != testAI2 {
		t.Fatalf("expected AI2 woken while AI1 in takeover, got %+v", r.runtime.recorded())
	}
	verifyWorkingJumpAndTakeoverRelease(t, r, gKey)
}

func verifyWorkingJumpAndTakeoverRelease(t *testing.T, r *deliverRig, gKey SessionKey) {
	t.Helper()
	// Move AI1 from Takeover to Working: every item stays held and unacknowledged, a jump item first in the queue.
	r.runtime.mu.Lock()
	r.runtime.keepRun = true
	r.runtime.mu.Unlock()
	r.h.Sessions.Move(gKey, Takeover, Working)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 2 {
		t.Fatal("delivered non-jump item while Working")
	}
	jumpIt := proto.Item{Item: 606, AI: testAI1, Why: "mention", Msg: testMsg2, Jump: true}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, jumpIt, r.makeTextMsg(testMsg2, nil, "jump"))
	r.waitUntil("jump item held first", func() bool {
		d := deliv(r.h)
		d.mu.Lock()
		defer d.mu.Unlock()
		queue := d.held[aiGroupKey{group: testGroup1, member: testAI1}]
		return len(queue) == 4 && queue[0].item.Item == 606
	})
	r.waitAIUnbusy(testGroup1, testAI1)
	if len(r.runtime.recorded()) != 2 || r.home.ackCountFor(606) != 0 {
		t.Fatal("a jump item was delivered or acknowledged while the AI was working")
	}
	verifyReentrantDrain(t, r, gKey)
}

func verifyReentrantDrain(t *testing.T, r *deliverRig, gKey SessionKey) {
	t.Helper()
	// Finish Working and test re-entrant Deliver draining the GroupSession batch of the jump (606+604) then the TaskSession batch (601+602).
	var reentered atomic.Bool
	r.runtime.mu.Lock()
	r.runtime.keepRun = false
	r.runtime.onTurn = func(h *Host, key SessionKey) {
		if reentered.CompareAndSwap(false, true) {
			Deliver(h, key.Group, key.Member)
		}
	}
	r.runtime.mu.Unlock()
	r.h.Sessions.Move(gKey, Working, Waiting)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("4 turns drained", func() bool { return len(r.runtime.recorded()) == 4 })
	turns := r.runtime.recorded()
	if turns[2].text != "group:2" || !turns[2].turn.Jump || turns[3].text != "task:2" || r.home.ackCountFor(606) != 1 {
		t.Fatalf("expected the jump's group:2 then task:2 turns, got %+v", turns)
	}
}

func verifyEnqueueHeldOrdering(t *testing.T) {
	t.Helper()
	mk := func(id int64, jump bool) verifiedItem {
		var j proto.True
		if jump {
			j = true
		}
		return verifiedItem{item: proto.Item{Item: id, Why: "mention", Msg: testCanonicalID(byte(id)), Jump: j}}
	}
	var q []verifiedItem
	q = enqueueHeld(q, mk(10, true))
	q = enqueueHeld(q, mk(11, true))
	q = enqueueHeld(q, mk(10, true))
	q = enqueueHeld(q, mk(12, false))
	q = enqueueHeld(q, mk(12, false))
	q = enqueueHeld(q, mk(13, false))
	q = enqueueHeld(q, mk(13, true))
	var got []int64
	for _, e := range q {
		got = append(got, e.item.Item)
	}
	if !slices.Equal(got, []int64{10, 11, 13, 12}) {
		t.Fatalf("got queue %v, want [10 11 13 12]", got)
	}
	q2 := enqueueHeld(enqueueHeld(nil, mk(20, false)), mk(21, false))
	q2 = enqueueHeld(q2, mk(20, true))
	if len(q2) != 2 || q2[0].item.Item != 20 || !bool(q2[0].item.Jump) {
		t.Fatalf("index 0 upgrade failed: %+v", q2)
	}
}

func TestStillCurrentTaskRoleStepReportMismatches(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	var held atomic.Bool
	var holdChecks atomic.Int32
	held.Store(true)
	r.setHold(func(*Host, SessionKey) (string, bool) {
		holdChecks.Add(1)
		return "offline", held.Load()
	})
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "review", Report: testMsg1, Assignee: testAI1, Reviewer: testAI2})

	// Mixed batch on TaskSession: stale assign + valid thread mention on existing TaskSession, plus GroupSession decided queued behind an all-stale batch.
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, Kind: TaskSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Claude"})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 701, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 702, AI: testAI1, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "still valid"))
	r.waitUntil("held task items checked", func() bool { return holdChecks.Load() >= 2 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 11}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "review", Report: testMsg1, Assignee: testAI2, Reviewer: testSec})
	held.Store(false)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("mixed batch turn", func() bool { return len(r.runtime.recorded()) == 1 })
	if r.runtime.recorded()[0].text != "task:1" {
		t.Fatalf("expected mixed batch to deliver 1 valid item, got %+v", r.runtime.recorded())
	}

	// Check every role, step and report mismatch, and the refusal for a task that is done.
	activeTask := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 20}, Group: testGroup1, Num: 2, Title: "T2", Origin: "brief", State: "active", Step: "review", Report: testMsg2, Assignee: testAI1, Reviewer: testAI2}
	r.putTask(activeTask)
	staleItems := []proto.Item{
		{Why: "assign", AI: testAI2, Task: testTask2},
		{Why: "review", AI: testAI1, Task: testTask2, Msg: testMsg2},
		{Why: "review", AI: testAI2, Task: testTask2, Msg: testMsg1},
	}
	for i, it := range staleItems {
		it.Item = int64(710 + i)
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil)
	}
	r.waitUntil("5 acks after first stale wave", func() bool { return len(r.home.ackList()) == 5 })
	activeTask.Step = "work"
	r.putTask(activeTask)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 720, AI: testAI2, Why: "review", Task: testTask2, Msg: testMsg2}, nil)
	r.waitUntil("6 acks after the review of a task at step work", func() bool { return len(r.home.ackList()) == 6 })
	activeTask.State = "done"
	r.putTask(activeTask)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 723, AI: testAI1, Why: "resume", Task: testTask2}, nil)
	r.waitUntil("7 acks after second stale wave", func() bool { return len(r.home.ackList()) == 7 })
	if len(r.runtime.recorded()) != 1 {
		t.Fatalf("stale items ran turns: %d", len(r.runtime.recorded()))
	}
	for _, a := range r.audits()[2:] {
		if a.what != AuditRefused || a.detail != "stale_task" {
			t.Fatalf("a stale item was not refused: %+v", a)
		}
	}
}

func TestTurnComposeAuditAndStoreErrors(t *testing.T) {
	logs := captureLogs(t)
	r := newDeliverRig(t, true)
	r.seedGroupAndMembers()

	// 1) Runner.Turn error is logged; Compose error logs and re-queues without acking.
	r.runtime.mu.Lock()
	r.runtime.err = errors.New("cli_failed")
	r.runtime.mu.Unlock()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 900, AI: testSec, Why: "notice", Code: "c"}, nil)
	r.waitUntil("item 900 turn error logged", func() bool {
		return r.home.ackCountFor(900) == 1 && strings.Contains(logs.String(), "cli_failed")
	})
	r.waitAIUnbusy(testGroup1, testSec)
	r.runtime.mu.Lock()
	r.runtime.err = nil
	r.runtime.mu.Unlock()

	r.composeMu.Lock()
	r.composeE = errors.New("fetch failed")
	r.composeMu.Unlock()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 901, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "retry"))
	r.waitUntil("compose error logged", func() bool { return strings.Contains(logs.String(), "host: compose") })
	r.waitAIUnbusy(testGroup1, testAI1)
	if r.home.ackCountFor(901) != 0 {
		t.Fatal("compose error still acked")
	}
	r.composeMu.Lock()
	r.composeE = nil
	r.composeMu.Unlock()
	addAuditBlockTrigger(t, r.db)
	Deliver(r.h, testGroup1, testAI1)
	if r.home.ackCountFor(901) != 0 {
		t.Fatal("audit write failure still acked")
	}
	checkHandAndRefusalAuditFailure(t, r)
	dropAuditBlockTrigger(t, r.db)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("re-queued 901 acked", func() bool { return r.home.ackCountFor(901) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)
	checkConfigUnmarshalAndDroppedTables(t, r)
}

func checkHandAndRefusalAuditFailure(t *testing.T, r *deliverRig) {
	t.Helper()
	ch, done := r.h.Await(testCall1, nil)
	defer done()
	callRef := testCall1
	replyMsg := &proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg2, Seq: 40}, Pos: 40, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 902, AI: testAI1, Why: "reply", Msg: testMsg2}, replyMsg)
	if len(ch) != 1 || r.home.ackCountFor(902) != 0 {
		t.Fatal("expected handed item without ack on audit failure")
	}
	// With the audit trigger active, refuseItem skips the ack.
	refuseItem(r.h, verifiedItem{home: r.homeKey.ID(), group: testGroup1, item: proto.Item{Item: 904}}, "", "not_hosted_here")
	if r.home.ackCountFor(904) != 0 {
		t.Fatal("refusal ack sent when audit write failed")
	}
}

func checkConfigUnmarshalAndDroppedTables(t *testing.T, r *deliverRig) {
	t.Helper()
	logs := captureLogs(t)

	// Session INSERT trigger fails createSession; non-array 'homes' JSON logs and aborts attachHome.
	writeTrigger := func(tx *store.Tx) error {
		_, err := tx.Exec(`CREATE TRIGGER block_session BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
		return err
	}
	r.db.Write(writeTrigger)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 60}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "work", Assignee: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 949, AI: testAI2, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("session create failure logged", func() bool { return strings.Contains(logs.String(), "host: create session") })
	r.waitAIUnbusy(testGroup1, testAI2)
	if r.home.ackCountFor(949) != 0 || r.h.QueuedCount(testGroup1, testAI2) != 1 {
		t.Fatal("an item whose session could not be recorded was not held")
	}

	r.setSetting("homes", 123)
	r.h.Attach(r.homeKey.ID(), r.home)

	// Drop audit to cover hasAuditItem error.
	dropAudit := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE audit")
		return err
	}
	r.db.Write(dropAudit)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 960, AI: testAI1, Why: "notice", Code: "c"}, nil)
	r.h.Settle()
}

func addAuditBlockTrigger(t *testing.T, db *store.Store) {
	t.Helper()
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("CREATE TRIGGER block_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT, 'no audit'); END")
		return err
	}
	if err := db.Write(write); err != nil {
		t.Fatal(err)
	}
}

func dropAuditBlockTrigger(t *testing.T, db *store.Store) {
	t.Helper()
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TRIGGER block_audit")
		return err
	}
	if err := db.Write(write); err != nil {
		t.Fatal(err)
	}
}

func pushTaskState(r *deliverRig, row proto.TaskRow) {
	if row.Depends == nil {
		row.Depends = []string{}
	}
	frame := proto.Rows{Type: "rows", Group: row.Group, Rows: []json.RawMessage{json.RawMessage(marshalWire(row))}}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(frame)))
}

func hasSession(h *Host, key SessionKey) bool {
	_, ok := h.Sessions.Get(key)
	return ok
}

func TestRouteThreadReplyAndNonThreadItems(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10},
		Group:     testGroup1, Num: 1, Title: "T1", Origin: "brief",
		State: "active", Step: "review", Report: testMsg2, Assignee: testAI1, Reviewer: testAI2,
	})
	taskKey := SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}
	revKey := SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}
	grpKey := SessionKey{Group: testGroup1, Member: testAI1}
	tskNoErr(t, r.h.Sessions.Create(Session{SessionKey: taskKey, Kind: TaskSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Claude"}))
	tskNoErr(t, r.h.Sessions.Create(Session{SessionKey: revKey, Kind: ReviewSession, Folder: r.folder, Agent: "codex", Home: r.homeKey.ID(), Name: "Codex"}))

	tid := testTask1
	deliverSingleAndWait(r, 951, proto.Item{Item: 951, AI: testAI1, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, &tid, "mention"), 1)
	deliverSingleAndWait(r, 952, proto.Item{Item: 952, AI: testAI1, Why: "reply", Task: testTask1, Msg: testMsg2}, r.makeTextMsg(testMsg2, &tid, "reply"), 2)
	deliverSingleAndWait(r, 953, proto.Item{Item: 953, AI: testAI1, Why: "notice", Task: testTask1, Code: "quiet"}, nil, 3)
	deliverSingleAndWait(r, 955, proto.Item{Item: 955, AI: testAI2, Why: "reply", Msg: testMsg2}, r.makeTextMsg(testMsg2, &tid, "review reply"), 4)

	wantKeys := []SessionKey{taskKey, taskKey, grpKey, revKey}
	wantTexts := []string{"task:1", "task:1", "group:1", "review:1"}
	assertTurnTargets(t, r.runtime.recorded(), wantKeys, wantTexts)
}

func deliverSingleAndWait(r *deliverRig, id int64, it proto.Item, msg *proto.MsgRow, wantTurns int) {
	r.t.Helper()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, msg)
	r.waitUntil("turn "+strconv.Itoa(wantTurns), func() bool {
		return len(r.runtime.recorded()) == wantTurns && r.home.ackCountFor(id) == 1
	})
	r.waitAIUnbusy(testGroup1, string(it.AI))
}

func assertTurnTargets(t *testing.T, turns []recordedTurn, wantKeys []SessionKey, wantTexts []string) {
	t.Helper()
	if len(turns) != len(wantKeys) {
		t.Fatalf("got %d turns, want %d", len(turns), len(wantKeys))
	}
	for i := range wantKeys {
		if turns[i].key != wantKeys[i] || turns[i].text != wantTexts[i] {
			t.Fatalf("turn %d got (%+v, %q), want (%+v, %q)", i, turns[i].key, turns[i].text, wantKeys[i], wantTexts[i])
		}
	}
}

func TestRecordAuditLogging(t *testing.T) {
	logs := captureLogs(t)
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()

	refuseItem(r.h, verifiedItem{home: r.homeKey.ID(), group: testGroup1, item: proto.Item{Item: 970}}, "claude", "stale_task")
	if r.home.ackCountFor(970) != 1 || strings.Contains(logs.String(), "host: write audit") {
		t.Fatalf("unexpected log or missing ack on valid audit write: acks=%d logs=%q", r.home.ackCountFor(970), logs.String())
	}

	addAuditBlockTrigger(t, r.db)
	refuseItem(r.h, verifiedItem{home: r.homeKey.ID(), group: testGroup1, item: proto.Item{Item: 971}}, "claude", "stale_task")
	if r.home.ackCountFor(971) != 0 || !strings.Contains(logs.String(), "host: write audit refused:") {
		t.Fatalf("missing audit failure log or unexpected ack: acks=%d logs=%q", r.home.ackCountFor(971), logs.String())
	}
	dropAuditBlockTrigger(t, r.db)
}

// Every session of an AI runs in the folder of its group session. A review
// item writes the report it carries into the row of its session, a later one
// in place of the one before, and the row keeps it over a restart.
func TestSessionsCopyTheWorkspaceAndReviewsNoteTheirReport(t *testing.T) {
	r := newDeliverRig(t, true)
	r.seedGroupAndMembers()
	task := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "review", Report: testMsg1, Assignee: testAI1, Reviewer: testAI2}
	r.putTask(task)
	deliverSingleAndWait(r, 1001, proto.Item{Item: 1001, AI: testAI1, Why: "assign", Task: testTask1}, nil, 1)
	deliverSingleAndWait(r, 1002, proto.Item{Item: 1002, AI: testAI2, Why: "review", Task: testTask1, Msg: testMsg1}, nil, 2)
	kT := SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}
	kR := SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}
	sT, _ := r.h.Sessions.Get(kT)
	sR, _ := r.h.Sessions.Get(kR)
	if sT.Kind != TaskSession || sT.Folder != r.folder || sT.Report != "" || sR.Kind != ReviewSession || sR.Folder != r.folder || sR.Report != testMsg1 {
		t.Fatalf("task session %+v, review session %+v, want both in %s and the report on the review session", sT, sR, r.folder)
	}

	task.Seq, task.Report = 11, testMsg2
	r.putTask(task)
	deliverSingleAndWait(r, 1003, proto.Item{Item: 1003, AI: testAI2, Why: "review", Task: testTask1, Msg: testMsg2}, nil, 3)
	deliverSingleAndWait(r, 1004, proto.Item{Item: 1004, AI: testAI2, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "a question"), 4)
	r.h.Settle()
	_, saved, err := loadSessions(r.db, time.Now)
	tskNoErr(t, err)
	for _, s := range saved {
		if want := map[SessionKey]string{kR: testMsg2}[s.SessionKey]; s.Report != want || s.Folder != r.folder {
			t.Fatalf("saved session %+v, want report %q in %s", s, want, r.folder)
		}
	}
	tskEqual(t, len(saved), 5)
}

// The record of a delivery and the report of a review are written together or
// not at all: a review item whose session has ended meanwhile is not recorded.
func TestReviewItemOfAnEndedSessionIsNotRecorded(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	review := proto.Item{Item: 1040, AI: testAI2, Why: "review", Task: testTask1, Msg: testMsg1}
	batch := []verifiedItem{
		{home: r.homeKey.ID(), group: testGroup1, item: proto.Item{Item: 1039, AI: testAI2, Why: "mention", Msg: testMsg2}},
		{home: r.homeKey.ID(), group: testGroup1, item: review},
	}
	err := commitTurnItems(r.h, "claude", SessionKey{Group: testGroup1, Member: testAI2, Task: testTask1}, batch)
	if seamCode(t, err, "not_found"); len(r.audits()) != 0 {
		t.Fatalf("items of an ended session were recorded: %+v", r.audits())
	}
}

// An AI that has no group session on this node has no folder: nothing is
// recorded for it, and its items wait.
func TestItemsWaitForTheGroupSession(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	tskNoErr(t, r.h.Sessions.End(SessionKey{Group: testGroup1, Member: testAI1}))
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "work", Assignee: testAI1})
	var looks atomic.Int32
	r.setHold(func(*Host, SessionKey) (string, bool) {
		looks.Add(1)
		return "", false
	})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 1010, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "hello"))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 1011, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("both items looked at", func() bool { return looks.Load() >= 2 && r.h.QueuedCount(testGroup1, testAI1) == 2 })
	r.waitAIUnbusy(testGroup1, testAI1)
	if len(r.home.ackList()) != 0 || len(r.audits()) != 0 || len(memberSessions(r.h, testGroup1, testAI1)) != 0 || len(r.runtime.recorded()) != 0 {
		t.Fatalf("an AI without a group session got something: acks=%v audits=%+v sessions=%s", r.home.ackList(), r.audits(), sessionStates(r.h))
	}
	r.joined(testAI1, "Claude", false)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("both items delivered", func() bool { return len(r.runtime.recorded()) == 2 && len(r.home.ackList()) == 2 })
}

// The first turn of a task session starts only after the host has closed the
// group's task sessions whose task is final or has another assignee: a folder
// never runs the last turn of one task beside the first turn of the next
// (contracts/task-machine.md section 4). The rows are put without a push, so
// that only the step that records the new session can close the old one.
func TestTaskSessionStartsAfterObsoleteSessionsAreClosed(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	first := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Origin: "brief", State: "active", Step: "work", Assignee: testAI1, Reviewer: testAI2}
	r.putTask(first)
	r.runtime.mu.Lock()
	r.runtime.keepRun = true
	r.runtime.mu.Unlock()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 1030, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	kFirst := SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}
	r.waitUntil("the first task's turn running", func() bool {
		s, _ := r.h.Sessions.Get(kFirst)
		return s.State == Working
	})
	r.waitAIUnbusy(testGroup1, testAI1)

	// The review passed while that turn still runs; the task that waited for the folder starts.
	first.Seq, first.State = 11, "done"
	r.putTask(first)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 12}, Group: testGroup1, Num: 2, Title: "T2", Origin: "brief", State: "active", Step: "work", Assignee: testAI2})
	var beside atomic.Bool
	r.runtime.mu.Lock()
	r.runtime.onTurn = func(h *Host, _ SessionKey) { beside.Store(hasSession(h, kFirst)) }
	r.runtime.mu.Unlock()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 1031, AI: testAI2, Why: "assign", Task: testTask2}, nil)
	r.waitUntil("the second task's turn", func() bool { return len(r.runtime.recorded()) == 2 })
	if beside.Load() || hasSession(r.h, kFirst) || !slices.Contains(r.runtime.closedKeys(), kFirst) {
		t.Fatalf("the second task started beside the first: closed=%v sessions=%s", r.runtime.closedKeys(), sessionStates(r.h))
	}
}

// A task session ends when its task is done or cancelled or has another
// assignee, a review session when its task is done or cancelled or has
// another reviewer, and every session of a member that is gone
// (flows/agent-turn.md step 17). Nothing in the folder is touched.
func TestSessionsEndWithTheirTaskRoleAndMember(t *testing.T) {
	r := newDeliverRig(t, true)
	r.seedGroupAndMembers()
	marker := filepath.Join(r.folder, "work.txt")
	tskNoErr(t, os.WriteFile(marker, []byte("uncommitted\n"), 0o600))
	seed := func(task string) (SessionKey, SessionKey) {
		kT := SessionKey{Group: testGroup1, Member: testAI1, Task: task}
		kR := SessionKey{Group: testGroup1, Member: testAI2, Task: task}
		tskNoErr(t, r.h.Sessions.Create(Session{SessionKey: kT, Kind: TaskSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Claude"}))
		tskNoErr(t, r.h.Sessions.Create(Session{SessionKey: kR, Kind: ReviewSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Codex"}))
		return kT, kR
	}
	push := func(row proto.TaskRow) {
		pushTaskState(r, row)
		settleDelivery(r.h)
	}
	kT1, kR1 := seed(testTask1)
	kUnseen := SessionKey{Group: testGroup1, Member: testAI1, Task: "task-unseen"}
	kOther := SessionKey{Group: testGroup2, Member: testAI1, Task: testTask1}
	for _, key := range []SessionKey{kUnseen, kOther} {
		tskNoErr(t, r.h.Sessions.Create(Session{SessionKey: key, Kind: TaskSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Claude"}))
	}
	remoteAI := string(testCanonicalID(77))
	row := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "Alpha", Origin: "brief", State: "active", Step: "review", Assignee: remoteAI, Reviewer: testAI2}

	// While the group's state is being read, nothing ends.
	d := deliv(r.h)
	d.mu.Lock()
	d.groupLocked(testGroup1, r.homeKey.ID()).live = false
	d.mu.Unlock()
	r.putTask(row)
	cleanupGroupState(r.h, d, testGroup1)
	d.mu.Lock()
	d.groupLocked(testGroup1, r.homeKey.ID()).live = true
	d.mu.Unlock()
	tskRequire(t, hasSession(r.h, kT1) && len(r.runtime.closedKeys()) == 0, "a session ended while the group's state was read")

	row.Seq, row.Assignee = 11, testAI1
	push(row)
	tskRequire(t, hasSession(r.h, kT1) && hasSession(r.h, kR1), "sessions of an active task ended")
	row.Seq, row.Assignee = 12, remoteAI
	push(row)
	tskRequire(t, !hasSession(r.h, kT1) && hasSession(r.h, kR1) && slices.Equal(r.runtime.closedKeys(), []SessionKey{kT1}), "another assignee: the task session alone ends")
	row.Seq, row.Reviewer = 13, remoteAI
	push(row)
	tskRequire(t, !hasSession(r.h, kR1) && hasSession(r.h, kUnseen) && hasSession(r.h, kOther), "another reviewer: the review session alone ends")
	tskRequire(t, hasSession(r.h, SessionKey{Group: testGroup1, Member: testAI1}), "a group session ended with a task")

	kT2, kR2 := seed(testTask2)
	push(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 20}, Group: testGroup1, Num: 2, Title: "Beta", Origin: "brief", State: "cancelled", Step: "work", Assignee: testAI1, Reviewer: testAI2})
	tskRequire(t, !hasSession(r.h, kT2) && !hasSession(r.h, kR2), "a cancelled task kept its sessions")

	task3 := string(testCanonicalID(63))
	kT3, kR3 := seed(task3)
	tskNoErr(t, r.h.Sessions.Move(kT3, Dormant, Takeover))
	done := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: task3, Seq: 30}, Group: testGroup1, Num: 3, Title: "Gamma", Origin: "brief", State: "done", Step: "review", Assignee: testAI1, Reviewer: testAI2}
	push(done)
	tskRequire(t, !hasSession(r.h, kT3) && !hasSession(r.h, kR3), "a task that is done kept its sessions")
	closedBefore := len(r.runtime.closedKeys())
	done.Seq = 31
	push(done)
	tskEqual(t, len(r.runtime.closedKeys()), closedBefore)

	gone1 := proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 41}, Gone: true}
	gone2 := proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI2, Seq: 42}, Gone: true}
	frame := proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{json.RawMessage(marshalWire(gone1)), json.RawMessage(marshalWire(gone2))}}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(frame)))
	settleDelivery(r.h)
	var left []SessionKey
	for _, s := range r.h.Sessions.All() {
		left = append(left, s.SessionKey)
	}
	tskRequire(t, slices.Equal(left, []SessionKey{{Group: testGroup1, Member: testSec}, kOther}), "a gone member's sessions: "+sessionStates(r.h))
	if text, err := os.ReadFile(marker); err != nil || string(text) != "uncommitted\n" {
		t.Fatalf("the folder was touched: %q, %v", text, err)
	}

	// A session whose task ended while the node was down ends when the node has read the group's state again.
	task5 := string(testCanonicalID(65))
	kT5 := SessionKey{Group: testGroup1, Member: testSec, Task: task5}
	tskNoErr(t, r.h.Sessions.Create(Session{SessionKey: kT5, Kind: TaskSession, Folder: r.folder, Agent: "claude", Home: r.homeKey.ID(), Name: "Sec"}))
	r.h.Settle()
	r2 := newDeliverRigOnStore(t, r.db)
	r2.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: task5, Seq: 50}, Group: testGroup1, Num: 5, Title: "Orphan", Origin: "brief", State: "done", Step: "review", Assignee: testSec})
	r2.h.Attach(r2.homeKey.ID(), r2.home)
	r2.waitUntil("the orphan session ended after the restart", func() bool { return !hasSession(r2.h, kT5) })
}
