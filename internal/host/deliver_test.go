package host

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	err     error
	keepRun bool
	onTurn  func(h *Host, key SessionKey)
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
	composeMu sync.Mutex
	composeE  error
	onCompose func(s Session, items []json.RawMessage)
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
		stopDeliverySyncs(h)
	})
	return rig
}

func stopDeliverySyncs(h *Host) {
	d := deliv(h)
	d.mu.Lock()
	for dev, entry := range d.cancels {
		entry.cancel()
		delete(d.cancels, dev)
	}
	d.mu.Unlock()
}

func (r *deliverRig) installHooks() {
	prevRunner, prevCompose, prevHolds := Runner, Compose, Holds
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
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

func (r *deliverRig) waitUntil(desc string, ok func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	r.t.Fatalf("timed out waiting for %s", desc)
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

func (r *deliverRig) waitNoActiveMerges() {
	r.t.Helper()
	d := deliv(r.h)
	r.waitUntil("merges unclaimed", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.merging) == 0
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
		Name:      "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{},
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
	checks := []bool{Attached["deliver"] != nil, Pushes["inbox"] != nil, Pushes["rows"] != nil, Pushes["reset"] != nil, Deliver != nil}
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
	t.Cleanup(func() { stopDeliverySyncs(h2) })
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
	// Push inbox for unsynced testGroup1 on an open connection whose d.cancels entry is nil.
	it := proto.Item{Item: 10, AI: testAI1, Why: "assign", Task: testTask1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil)
	req := <-r.home.started
	wantSync := proto.Sync{Request: req.Request, Group: testGroup1, Since: 0, StateOnly: true}
	if req != wantSync || len(r.home.ackList()) != 0 || len(r.audits()) != 0 {
		t.Fatalf("unexpected pre-sync state: req=%+v acks=%v audits=%v", req, r.home.ackList(), r.audits())
	}
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "P1", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}}))
	aiRaw := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	tRaw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "T", Slug: "t", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1, Depends: []string{}}))
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
	watchHomeConn(d, r.homeKey.ID(), stale, &homeSync{ctx: staleCtx, cancel: staleCancel})
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
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 4}, Name: "P", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}}))
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
	mergeIt := proto.Item{Item: 31, AI: testAI2, Why: "merge", Task: testTask2, Commit: "c1"}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, assignIt, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, mergeIt, nil)
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
	t1Raw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 8}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1, Depends: []string{}}))
	t2Raw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 9}, Group: testGroup1, Num: 2, Title: "T2", Slug: "t2", Origin: "brief", State: "active", Step: "merge", Commit: "c1", Assignee: testAI2, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{t1Raw, t2Raw}})))
	r.waitUntil("both delayed items acknowledged", func() bool { return len(third.ackList()) == 2 && len(r.runtime.recorded()) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.waitNoActiveMerges()
	assertLiveCursorAndFailedDelta(t, r)
}

func assertLiveCursorAndFailedDelta(t *testing.T, r *deliverRig) {
	t.Helper()
	// Reconnect must ask since 10 (not regressed to 9); wrong group in Synced keeps 10; live row seq 12 advances to 12; dup 12 and 11 ignored.
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
	assertMismatchAndMalformedSynced(t, r)
}

func assertMismatchAndMalformedSynced(t *testing.T, r *deliverRig) {
	t.Helper()
	fifth := newFakeDeliverHome()
	fifth.syncMode = true
	r.h.Attach(r.homeKey.ID(), fifth)
	t.Cleanup(func() { fifth.closeConn() })
	if req5 := <-fifth.started; req5.Since != 12 {
		t.Fatalf("got since=%d, want 12", req5.Since)
	}
	fifth.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup2, Seq: 50}))}
	r.waitSyncStopped(testGroup1)
	sixth := newFakeDeliverHome()
	sixth.syncMode = true
	r.h.Attach(r.homeKey.ID(), sixth)
	t.Cleanup(func() { sixth.closeConn() })
	if req6 := <-sixth.started; req6.Since != 12 {
		t.Fatalf("after failed delta got since=%d, want 12", req6.Since)
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 39, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "wait-sync"))
	sixth.answers <- syncAnswer{raw: json.RawMessage(`{"t":"synced","re":1,"g":"` + testGroup1 + `","seq":99}`)}
	r.waitSyncStopped(testGroup1)
	r.waitAIUnbusy(testGroup1, testAI1)
	d := deliv(r.h)
	d.mu.Lock()
	cur, live := d.groups[testGroup1].cursor, d.groups[testGroup1].live
	d.mu.Unlock()
	if cur != 12 || live || len(r.runtime.recorded()) != 1 {
		t.Fatalf("invalid synced frame accepted: cursor=%d live=%v turns=%d", cur, live, len(r.runtime.recorded()))
	}
}

func TestGroupResyncAfterPostSyncedResetAndFailedSync(t *testing.T) {
	// 1) A reset handled after synced, followed by group + member rows and a mention, starts a new sync and delivers the turn.
	r := newDeliverRig(t, false)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "Proj"}})
	r.home.syncMode = true
	r.h.Attach(r.homeKey.ID(), r.home)
	req1 := <-r.home.started
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: req1.Group, Seq: 7}))}
	r.waitGroupLive(testGroup1)

	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Reset{Type: "reset", Group: testGroup1})))
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "Proj", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}}))
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

func TestDuplicateWhileTakingSessionAndMergeItems(t *testing.T) {
	// (a) Duplicate mention arriving while the first copy is inside Compose -> 1 turn, 1 delivered row, 2 acks.
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
	verifyDuplicateMergeWhileApplyingAndPreAudit(t)
}

func verifyDuplicateMergeWhileApplyingAndPreAudit(t *testing.T) {
	t.Helper()
	r := newDeliverRig(t, true)
	originDir, pDir, _ := initTestOriginAndGroupRepo(t, r, "none")
	ctx := t.Context()
	if _, err := EnsureGroupWorkspace(ctx, pDir, originDir, "main"); err != nil {
		t.Fatal(err)
	}
	wt, err := CreateTaskWorkspace(ctx, pDir, 1, "dup-merge", testAI1, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, wt, "config", "user.name", "AI")
	mustGit(t, ctx, wt, "config", "user.email", "ai@example.invalid")
	os.WriteFile(filepath.Join(wt, "f1.txt"), []byte("one\n"), 0o600)
	mustGit(t, ctx, wt, "add", "f1.txt")
	mustGit(t, ctx, wt, "commit", "-m", "c1")
	c1 := strings.TrimSpace(mustGit(t, ctx, wt, "rev-parse", "HEAD"))
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "M1", Slug: "dup-merge", Origin: "brief", State: "active", Step: "merge", Commit: c1, Assignee: testAI1, Stats: proto.TaskStats{Added: 1}})

	// 1) Block ReviewedMerge.Apply on cloneLock while pushing duplicate merge item 905 -> 1 Apply, 2 acks.
	repoLock := cloneLock(filepath.Join(pDir, "repo"))
	repoLock.Lock()
	mIt1 := proto.Item{Item: 905, AI: testAI1, Why: "merge", Task: testTask1, Commit: c1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, mIt1, nil)
	r.waitUntil("first ack while Apply is blocked", func() bool { return r.home.ackCountFor(905) == 1 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, mIt1, nil)
	r.waitUntil("second ack while Apply is still blocked", func() bool { return r.home.ackCountFor(905) == 2 })
	repoLock.Unlock()
	r.waitUntil("single TaskMerged command", func() bool { return len(r.home.cmdPayloads()) == 1 })
	r.waitNoActiveMerges()

	// 2) Duplicate merge item 906 arriving in the pre-audit window so recordMergeTaken returns written == false -> 1 Apply, 2 acks.
	verifyMergePreAuditWrittenFalse(t, r, pDir)
}

func verifyMergePreAuditWrittenFalse(t *testing.T, r *deliverRig, pDir string) {
	t.Helper()
	ctx := t.Context()
	wt2, err := CreateTaskWorkspace(ctx, pDir, 2, "pre-audit", testAI1, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, ctx, wt2, "config", "user.name", "AI")
	mustGit(t, ctx, wt2, "config", "user.email", "ai@example.invalid")
	os.WriteFile(filepath.Join(wt2, "f2.txt"), []byte("two\n"), 0o600)
	mustGit(t, ctx, wt2, "add", "f2.txt")
	mustGit(t, ctx, wt2, "commit", "-m", "c2")
	c2 := strings.TrimSpace(mustGit(t, ctx, wt2, "rev-parse", "HEAD"))
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 11}, Group: testGroup1, Num: 2, Title: "M2", Slug: "pre-audit", Origin: "brief", State: "active", Step: "merge", Commit: c2, Assignee: testAI1, Stats: proto.TaskStats{Added: 1}})

	mIt2 := proto.Item{Item: 906, AI: testAI1, Why: "merge", Task: testTask2, Commit: c2}
	var first atomic.Bool
	Holds["dup"] = func(*Host, SessionKey) (string, bool) {
		if first.CompareAndSwap(false, true) {
			r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, mIt2, nil)
			r.waitUntil("inner merge 906 completed", func() bool { return len(r.home.cmdPayloads()) == 2 })
		}
		return "", false
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, mIt2, nil)
	r.waitUntil("inner merge 906 acked", func() bool { return r.home.ackCountFor(906) == 1 && len(r.home.cmdPayloads()) == 2 })
	r.waitNoActiveMerges()
	delete(Holds, "dup")
	// Also exercise recordMergeTaken returning written == false when an already-audited merge entry reaches takeOneMerge: acked again, 0 extra Apply.
	d := deliv(r.h)
	d.enqueueMerge(verifiedItem{home: r.homeKey.ID(), group: testGroup1, wire: proto.InboxItem{Item: mIt2}, item: mIt2})
	Deliver(r.h, "", "")
	r.waitUntil("second ack for already-audited merge 906", func() bool {
		return r.home.ackCountFor(906) == 2 && len(d.mergeGroups()) == 0
	})
	r.waitNoActiveMerges()
	if len(r.home.cmdPayloads()) != 2 {
		t.Fatalf("expected 2 total merge commands, got %d", len(r.home.cmdPayloads()))
	}
}

func TestNonBlockingPushHandlersAndLockOrder(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1})

	cmdEntered := make(chan struct{}, 1) // single reader: main test goroutine
	pushDone := make(chan struct{})      // closed by main test goroutine once later pushes finish
	var hasDeadline bool
	var remaining time.Duration
	r.home.onCmd = func(ctx context.Context, _ proto.Cmd) (json.RawMessage, error) {
		dl, ok := ctx.Deadline()
		hasDeadline = ok
		remaining = time.Until(dl)
		cmdEntered <- struct{}{}
		<-pushDone
		return json.RawMessage(`{"re":1,"ok":true}`), nil
	}
	// Push assign (triggers TaskBranch Request on background goroutine); Push must return immediately.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 40, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	<-cmdEntered
	// While Request is blocked, pushing rows and another inbox item must return without deadlocking.
	noteRaw := json.RawMessage(`{"r":"note","id":"n1","seq":11}`)
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{noteRaw}})))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 41, AI: testSec, Why: "notice", Code: "ping"}, nil)
	close(pushDone)

	r.waitUntil("both turns completed", func() bool { return len(r.runtime.recorded()) == 2 })
	if !hasDeadline || remaining <= 30*time.Second || remaining > time.Minute {
		t.Fatalf("expected 1m bounded context on h.Command, got ok=%v remaining=%v", hasDeadline, remaining)
	}
	verifyHoldLockNotUnderDeliveryMutex(t, r)
}

func verifyHoldLockNotUnderDeliveryMutex(t *testing.T, r *deliverRig) {
	t.Helper()
	var extMu sync.Mutex
	var first atomic.Bool
	Holds["lockorder"] = func(*Host, SessionKey) (string, bool) {
		extMu.Lock()
		defer extMu.Unlock()
		if first.CompareAndSwap(false, true) {
			r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 42, AI: testAI2, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "reentrant"))
		}
		return "", false
	}
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
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 103, AI: goneID, Why: "merge", Task: testTask1, Commit: "c1"}, nil)
	r.waitUntil("3 refused acks", func() bool { return len(r.home.ackList()) == 3 })
	r.waitAIUnbusy(testGroup1, testAI2)
	r.waitAIUnbusy(testGroup1, testHuman)
	for _, a := range r.audits() {
		if a.what != AuditRefused || a.detail != "not_hosted_here" {
			t.Fatalf("unexpected refusal audit: %+v", a)
		}
	}
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
	r.h.Sessions.Create(Session{SessionKey: gKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
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
	r.home.syncMode = true
	r.h.Attach(r.homeKey.ID(), r.home)
	<-r.home.started

	waitCh, doneWait := r.h.Await(testCard1, nil)
	defer doneWait()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 330, AI: testAI1, Why: "decided", Msg: testCard1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 331, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "held"))
	grpPaused := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Paused: &proto.GroupPause{By: testHuman, At: testStartMS}, Checks: []string{}}))
	aiRow := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpPaused, aiRow}})))
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 2}))}
	r.waitUntil("open wait drained at take", func() bool { return len(waitCh) == 1 && r.home.ackCountFor(330) == 1 })
	if len(r.runtime.recorded()) != 0 || len(r.audits()) != 1 || r.audits()[0].agent != "claude" {
		t.Fatalf("unexpected cold-start drain state: turns=%d audits=%+v", len(r.runtime.recorded()), r.audits())
	}
	grpUnpaused := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 3}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpUnpaused}})))
	r.waitUntil("mention delivered after unpause", func() bool { return len(r.runtime.recorded()) == 1 })
}

func TestRoutingHoldsJumpAndBatchSelection(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "review", Commit: "c1", Assignee: testAI1, Reviewer: testAI2})

	// Causeless confirmed to secretary -> GroupSession; assign -> TaskSession; review -> ReviewSession; causeless decided -> GroupSession/TaskSession.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 501, AI: testSec, Why: "confirmed", Msg: testMsg1}, nil)
	r.waitUntil("sec turn", func() bool { return len(r.runtime.recorded()) == 1 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 502, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("assign turn", func() bool { return len(r.runtime.recorded()) == 2 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 503, AI: testAI2, Why: "review", Task: testTask1, Commit: "c1"}, nil)
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
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 16}, Name: "Proj", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, 17)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 509, AI: testAI1, Why: "decided", Task: testTask1, Msg: testCard1}, nil)
	r.waitUntil("ai1 dormant task decided turn", func() bool { return len(r.runtime.recorded()) == 9 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 510, AI: testAI1, Why: "decided", Task: testTask2, Msg: testCard1}, nil)
	r.waitUntil("ai1 uncreated task decided fallback", func() bool { return len(r.runtime.recorded()) == 10 })
	r.waitAIUnbusy(testGroup1, testAI1)

	assertRoutedTurns(t, r.runtime.recorded())
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
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "paused", Step: "local_check", Assignee: testAI1})

	// Paused/failed task holds work items while notice delivers immediately.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 601, AI: testAI1, Why: "checks", Task: testTask1, Msg: testMsg1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 602, AI: testAI1, Why: "reviewed", Task: testTask1, Msg: testMsg2}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 603, AI: testSec, Why: "notice", Task: testTask1, Code: "quiet"}, nil)
	r.waitUntil("sec notice turn", func() bool { return len(r.runtime.recorded()) == 1 })
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 11}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "failed", Step: "local_check", Assignee: testAI1})
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("failed task delivered")
	}

	// Holds["cap"] holds AI1 and AI2; also queue a GroupSession mention for AI1 to verify selectEligibleBatch batches 601+602 into TaskSession (task:2) separately from GroupSession (group:1).
	var heldFlag atomic.Bool
	var holdChecks atomic.Int32
	heldFlag.Store(true)
	Holds["cap"] = func(*Host, SessionKey) (string, bool) {
		holdChecks.Add(1)
		return "cap", heldFlag.Load()
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 12}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 604, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "grp"))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 605, AI: testAI2, Why: "mention", Msg: testMsg2}, r.makeTextMsg(testMsg2, nil, "ai2"))
	r.waitUntil("held items checked", func() bool { return holdChecks.Load() >= 2 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.waitAIUnbusy(testGroup1, testAI2)
	gKey := SessionKey{Group: testGroup1, Member: testAI1}
	r.h.Sessions.Create(Session{SessionKey: gKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
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
	// Move AI1 from Takeover to Working: non-jump items stay held; jump item interrupts immediately while keepRun stays true.
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
	r.waitUntil("jump turn delivered", func() bool { return len(r.runtime.recorded()) == 3 && r.home.ackCountFor(606) == 1 })
	r.waitAIUnbusy(testGroup1, testAI1)
	if !r.runtime.recorded()[2].turn.Jump || r.runtime.recorded()[2].text != "group:1" {
		t.Fatalf("unexpected jump turn: %+v", r.runtime.recorded()[2])
	}
	verifyReentrantDrain(t, r, gKey)
}

func verifyReentrantDrain(t *testing.T, r *deliverRig, gKey SessionKey) {
	t.Helper()
	// Finish Working and test re-entrant Deliver draining TaskSession batch of 2 (601+602) then GroupSession batch of 1 (604).
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
	r.waitUntil("5 turns drained", func() bool { return len(r.runtime.recorded()) == 5 })
	turns := r.runtime.recorded()
	if turns[3].text != "task:2" || turns[4].text != "group:1" {
		t.Fatalf("expected task:2 then group:1 turns, got %+v", turns)
	}
}

func verifyEnqueueHeldOrdering(t *testing.T) {
	t.Helper()
	mk := func(id int64, jump bool) verifiedItem {
		var j proto.True
		if jump {
			j = true
		}
		return verifiedItem{item: proto.Item{Item: id, Jump: j}}
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

func TestStillCurrentTaskRoleStepCommitMismatches(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	var held atomic.Bool
	var holdChecks atomic.Int32
	held.Store(true)
	Holds["off"] = func(*Host, SessionKey) (string, bool) {
		holdChecks.Add(1)
		return "offline", held.Load()
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "review", Commit: "c1", Assignee: testAI1, Reviewer: testAI2})

	// Mixed batch on TaskSession: stale assign + valid thread mention on existing TaskSession, plus GroupSession decided queued behind an all-stale batch.
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, Kind: TaskSession, Folder: "wt/1-t1", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 701, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 702, AI: testAI1, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "still valid"))
	r.waitUntil("held task items checked", func() bool { return holdChecks.Load() >= 2 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 11}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "review", Commit: "c1", Assignee: testAI2, Reviewer: testSec})
	held.Store(false)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("mixed batch turn", func() bool { return len(r.runtime.recorded()) == 1 })
	if r.runtime.recorded()[0].text != "task:1" {
		t.Fatalf("expected mixed batch to deliver 1 valid item, got %+v", r.runtime.recorded())
	}

	// Check every role/step/commit mismatch and merged-task refusal.
	activeTask := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 20}, Group: testGroup1, Num: 2, Title: "T2", Slug: "t2", Origin: "brief", State: "active", Step: "review", Commit: "c-good", Assignee: testAI1, Reviewer: testAI2}
	r.putTask(activeTask)
	staleItems := []proto.Item{
		{Why: "assign", AI: testAI2, Task: testTask2},
		{Why: "review", AI: testAI1, Task: testTask2, Commit: "c-good"},
		{Why: "review", AI: testAI2, Task: testTask2, Commit: "c-old"},
		{Why: "merge", AI: testAI1, Task: testTask2, Commit: "c-good"},
	}
	for i, it := range staleItems {
		it.Item = int64(710 + i)
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil)
	}
	r.waitUntil("6 acks after first stale wave", func() bool { return len(r.home.ackList()) == 6 })
	activeTask.Step = "merge"
	r.putTask(activeTask)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 720, AI: testAI2, Why: "review", Task: testTask2, Commit: "c-good"}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 721, AI: testAI2, Why: "merge", Task: testTask2, Commit: "c-good"}, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 722, AI: testAI1, Why: "merge", Task: testTask2, Commit: "c-wrong"}, nil)
	activeTask.State = "merged"
	r.putTask(activeTask)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 723, AI: testAI1, Why: "resume", Task: testTask2}, nil)
	r.waitUntil("10 acks after second stale wave", func() bool { return len(r.home.ackList()) == 10 })
	if len(r.runtime.recorded()) != 1 {
		t.Fatalf("stale items ran turns: %d", len(r.runtime.recorded()))
	}
}

func initTestOriginAndGroupRepo(t *testing.T, r *deliverRig, forge string) (originDir, projectDir, baseCommit string) {
	t.Helper()
	ctx := t.Context()
	originDir = filepath.Join(t.TempDir(), "team", "website.git")
	seed := t.TempDir()
	mustGit(t, ctx, "", "init", "--bare", "--initial-branch=main", originDir)
	mustGit(t, ctx, "", "init", "--initial-branch=main", seed)
	mustGit(t, ctx, seed, "config", "user.name", "Test")
	mustGit(t, ctx, seed, "config", "user.email", "test@example.invalid")
	os.WriteFile(filepath.Join(seed, "README.md"), []byte("hello\nline2\n"), 0o600)
	mustGit(t, ctx, seed, "add", "README.md")
	mustGit(t, ctx, seed, "commit", "-m", "initial")
	mustGit(t, ctx, seed, "push", originDir, "refs/heads/main:refs/heads/main")
	baseCommit = strings.TrimSpace(mustGit(t, ctx, seed, "rev-parse", "HEAD"))
	cfg, err := loadHostConfig(r.h)
	if err != nil {
		t.Fatal(err)
	}
	projectDir = cfg.projectDir(testGroup1)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "Website"}})
	r.putGroup(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1},
		Name:      "Website", Kind: "project", Repo: originDir, Base: "main", Forge: forge,
		Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{},
	})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, 2)
	r.putAIMember(testAI2, "Codex", r.hostKey.ID(), false, 3)
	r.putAIMember(testSec, "Sec", r.hostKey.ID(), true, 4)
	r.h.Attach(r.homeKey.ID(), r.home)
	r.waitGroupLive(testGroup1)
	return originDir, projectDir, baseCommit
}

func mustGit(t *testing.T, ctx context.Context, dir string, args ...string) string {
	t.Helper()
	out, err := workspaceGit(ctx, dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func TestWorkspacesBranchRetryMergeStatsAndRestartInspect(t *testing.T) {
	logs := captureLogs(t)
	r := newDeliverRig(t, true)
	_, pDir, _ := initTestOriginAndGroupRepo(t, r, "none")
	ctx := t.Context()

	// 1) First TaskBranch fails -> error logged, session row exists, item held; retry sends TaskBranch again and delivers turn.
	r.home.mu.Lock()
	r.home.cmdErr = errors.New("temp_down")
	r.home.mu.Unlock()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 801, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("first failed TaskBranch logged", func() bool {
		return len(r.home.cmdPayloads()) == 1 && strings.Contains(logs.String(), "host: command to")
	})
	r.waitAIUnbusy(testGroup1, testAI1)
	if len(r.runtime.recorded()) != 0 {
		t.Fatalf("failed TaskBranch should hold turn: turns=%d", len(r.runtime.recorded()))
	}
	r.home.mu.Lock()
	r.home.cmdErr = nil
	r.home.mu.Unlock()
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("retried TaskBranch and turn", func() bool {
		return len(r.runtime.recorded()) == 1 && len(r.home.cmdPayloads()) == 2
	})
	if !strings.Contains(r.home.cmdPayloads()[1], `"branch":"task/1-add-feature"`) {
		t.Fatalf("expected retried TaskBranch and turn, got cmds=%v", r.home.cmdPayloads())
	}
	// Second item at local_check does not resend TaskBranch.
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 6}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 802, AI: testAI1, Why: "resume", Task: testTask1}, nil)
	r.waitUntil("second turn", func() bool { return len(r.runtime.recorded()) == 2 })
	if len(r.home.cmdPayloads()) != 2 {
		t.Fatal("sent duplicate TaskBranch after local_check")
	}
	wt := filepath.Join(pDir, "wt", "1-add-feature")
	mustGit(t, ctx, wt, "config", "user.name", "AI")
	mustGit(t, ctx, wt, "config", "user.email", "ai@example.invalid")
	os.WriteFile(filepath.Join(wt, "README.md"), []byte("hello\nchanged\nadded\n"), 0o600)
	mustGit(t, ctx, wt, "add", "README.md")
	mustGit(t, ctx, wt, "commit", "-m", "c1")
	c1 := strings.TrimSpace(mustGit(t, ctx, wt, "rev-parse", "HEAD"))
	verifyReviewRoundsAndMergeStats(t, r, pDir, wt, c1)
	verifyConflictRestartInspectAndRemoteForge(t)
}

func verifyReviewRoundsAndMergeStats(t *testing.T, r *deliverRig, pDir, wt, c1 string) {
	t.Helper()
	ctx := t.Context()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 7}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "review", Commit: c1, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 803, AI: testAI2, Why: "review", Task: testTask1, Commit: c1}, nil)
	r.waitUntil("review 1", func() bool { return len(r.runtime.recorded()) == 3 })
	if _, err := os.Stat(filepath.Join(pDir, "review", "1", reviewDiff)); err != nil {
		t.Fatalf("review diff missing: %v", err)
	}
	// Mention in existing ReviewSession (commit == "" in batch) reuses review/1 without PrepareReviewWorkspace.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 804, AI: testAI2, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "note"))
	r.waitUntil("review mention", func() bool { return len(r.runtime.recorded()) == 4 })

	os.WriteFile(filepath.Join(wt, "extra.txt"), []byte("more\n"), 0o600)
	mustGit(t, ctx, wt, "add", "extra.txt")
	mustGit(t, ctx, wt, "commit", "-m", "c2")
	c2 := strings.TrimSpace(mustGit(t, ctx, wt, "rev-parse", "HEAD"))
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 8}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "review", Commit: c2, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 805, AI: testAI2, Why: "review", Task: testTask1, Commit: c2}, nil)
	r.waitUntil("review 2", func() bool { return len(r.runtime.recorded()) == 5 })

	// Merge held while Holds reports "offline"; lifted when Holds reports "cap"; TaskMerged uses TaskRow.Stats directly.
	var holdReason atomic.Value
	var offChecks atomic.Int32
	holdReason.Store("offline")
	Holds["off"] = func(*Host, SessionKey) (string, bool) {
		offChecks.Add(1)
		return holdReason.Load().(string), true
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 9}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "merge", Commit: c2, Assignee: testAI1, Reviewer: testAI2, Stats: proto.TaskStats{Added: 17, Removed: 5}})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 806, AI: testAI1, Why: "merge", Task: testTask1, Commit: c2}, nil)
	r.waitUntil("offline merge hold checked", func() bool { return offChecks.Load() >= 1 })
	Deliver(r.h, "", "")
	if len(r.home.cmdPayloads()) != 2 {
		t.Fatal("merge ran while offline")
	}
	holdReason.Store("cap")
	Deliver(r.h, "", "")
	r.waitUntil("TaskMerged after offline lifted", func() bool { return len(r.home.cmdPayloads()) == 3 })
	cmds := r.home.cmdPayloads()
	if !strings.Contains(cmds[2], `"op":"merged"`) || !strings.Contains(cmds[2], `"added":17`) || !strings.Contains(cmds[2], `"removed":5`) {
		t.Fatalf("expected TaskMerged with TaskRow.Stats 17/5, got %v", cmds)
	}
}

func verifyConflictRestartInspectAndRemoteForge(t *testing.T) {
	t.Helper()
	r := newDeliverRig(t, true)
	originDir, pDir, baseCommit := initTestOriginAndGroupRepo(t, r, "none")
	if _, err := EnsureGroupWorkspace(t.Context(), pDir, originDir, "main"); err != nil {
		t.Fatal(err)
	}
	CreateTaskWorkspace(t.Context(), pDir, 1, "conflict", testAI1, "main", false)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "C", Slug: "conflict", Origin: "brief", State: "active", Step: "merge", Commit: "0000000000000000000000000000000000000000", Branch: "task/1-conflict", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 820, AI: testAI1, Why: "merge", Task: testTask1, Commit: "0000000000000000000000000000000000000000"}, nil)
	r.waitUntil("TaskConflict command", func() bool { return len(r.home.cmdPayloads()) == 1 })
	if !strings.Contains(r.home.cmdPayloads()[0], `"op":"conflict"`) {
		t.Fatalf("expected TaskConflict, got %v", r.home.cmdPayloads())
	}

	// Simulate restart with AuditMerge written; resumeGroupMerges inspects via d.groups[group].home and claimMerge.
	write := func(tx *store.Tx) error {
		_, err := r.h.Audit(tx, AuditRow{Group: testGroup1, Agent: "claude", What: AuditMerge, Detail: testTask2 + " " + baseCommit})
		return err
	}
	r.db.Write(write)
	r2 := newDeliverRigOnStore(t, r.db)
	r2.h.Attach(r2.homeKey.ID(), r2.home)
	r2.waitGroupLive(testGroup1)
	grpPaused := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 10}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r2.homeKey.ID(), Secretary: testSec, Paused: &proto.GroupPause{By: testHuman, At: testStartMS}, Checks: []string{}}))
	grpRaw := json.RawMessage(marshalWire(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 11}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r2.homeKey.ID(), Secretary: testSec, Checks: []string{}}))
	aiRaw := json.RawMessage(marshalWire(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 12}, Group: testGroup1, Kind: "ai", User: r2.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r2.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	t2Raw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 13}, Group: testGroup1, Num: 2, Title: "Restart", Slug: "restart", Origin: "brief", State: "active", Step: "merge", Commit: baseCommit, Assignee: testAI1, Depends: []string{}}))
	noAuditRaw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(testCanonicalID(98)), Seq: 14}, Group: testGroup1, Num: 3, Title: "N", Slug: "no-audit", Origin: "brief", State: "active", Step: "merge", Commit: baseCommit, Assignee: testAI1, Depends: []string{}}))
	remoteAIRaw := json.RawMessage(marshalWire(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(testCanonicalID(97)), Seq: 15}, Group: testGroup1, Num: 4, Title: "R", Slug: "remote-ai", Origin: "brief", State: "active", Step: "merge", Commit: baseCommit, Assignee: testAI2, Depends: []string{}}))
	r2.h.Push(r2.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpPaused, aiRaw, t2Raw, noAuditRaw, remoteAIRaw}})))
	afterGroupState(r2.h, deliv(r2.h), testGroup1)
	if len(r2.home.cmdPayloads()) != 0 {
		t.Fatal("restart inspect ran while group paused")
	}
	d2 := deliv(r2.h)
	mKey2 := testGroup1 + "\x00" + testTask2 + "\x00" + baseCommit
	d2.claimMerge(mKey2)
	r2.h.Rows.Put(testGroup1, "group", testGroup1, grpRaw)
	resumeOneMerge(r2.h, d2, r2.homeKey.ID(), testGroup1, proto.TaskRow{RowHeader: proto.RowHeader{ID: testTask2}, Num: 2, Slug: "restart", State: "active", Step: "merge", Commit: baseCommit, Assignee: testAI1})
	if len(r2.home.cmdPayloads()) != 0 {
		t.Fatal("restart inspect ran while merge already claimed")
	}
	d2.unclaimMerge(mKey2)
	r2.h.Push(r2.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpRaw}})))
	r2.waitUntil("restart inspect reported", func() bool { return len(r2.home.cmdPayloads()) == 1 })
	r2.waitNoActiveMerges()
	verifyRemoteForgeAndGitHubRepo(t)
}

func verifyRemoteForgeAndGitHubRepo(t *testing.T) {
	t.Helper()
	cases := map[string]string{
		"https://github.com/team/website.git": "team/website",
		"git@github.com:team/website.git/":    "team/website",
		`C:\repos\team\website.git`:           "team/website",
		"team/website":                        "team/website",
		"onlyone":                             "",
		"":                                    "",
	}
	for in, want := range cases {
		if got := githubRepo(in); got != want {
			t.Fatalf("githubRepo(%q)=%q, want %q", in, got, want)
		}
	}
	r := newDeliverRig(t, true)
	initTestOriginAndGroupRepo(t, r, "github")
	// GroupSession wake with forge=github runs RefreshGroupWorkspace on create and when Dormant, and skips when Waiting.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 850, AI: testSec, Why: "notice", Code: "ping"}, nil)
	r.waitUntil("sec turn 1", func() bool { return len(r.runtime.recorded()) == 1 })
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 851, AI: testSec, Why: "notice", Code: "pong"}, nil)
	r.waitUntil("sec turn 2", func() bool { return len(r.runtime.recorded()) == 2 })
	r.h.Sessions.Move(SessionKey{Group: testGroup1, Member: testSec}, Waiting, Dormant)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 852, AI: testSec, Why: "notice", Code: "dormant-wake"}, nil)
	r.waitUntil("sec turn 3", func() bool { return len(r.runtime.recorded()) == 3 })

	mergedJSON := `{"state":"MERGED","headRefOid":"deadbeef","mergeCommit":{"oid":"merged-oid"}}` + "\n"
	fakeForgeGH(t,
		forgeAnswer{},
		forgeAnswer{Stdout: mergedJSON},
		forgeAnswer{Stderr: "down", Code: 1},
		forgeAnswer{Stderr: "down", Code: 1},
	)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 20}, Group: testGroup1, Num: 1, Title: "Remote", Slug: "remote", Origin: "brief", State: "active", Step: "merge", Commit: "deadbeef", PR: 42, Assignee: testAI1, Stats: proto.TaskStats{Added: 9, Removed: 4}})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 853, AI: testAI1, Why: "merge", Task: testTask1, Commit: "deadbeef", PR: 42}, nil)
	r.waitUntil("remote TaskMerged", func() bool { return len(r.home.cmdPayloads()) == 1 })
	if !strings.Contains(r.home.cmdPayloads()[0], `"merge_commit":"merged-oid"`) || !strings.Contains(r.home.cmdPayloads()[0], `"added":9`) {
		t.Fatalf("unexpected remote TaskMerged: %v", r.home.cmdPayloads())
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 21}, Group: testGroup1, Num: 2, Title: "FailGH", Slug: "failgh", Origin: "brief", State: "active", Step: "merge", Commit: "fail-gh", PR: 99, Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 854, AI: testAI1, Why: "merge", Task: testTask2, Commit: "fail-gh", PR: 99}, nil)
	r.waitUntil("item 854 acked", func() bool { return r.home.ackCountFor(854) == 1 })
	r.waitNoActiveMerges()
	if len(r.home.cmdPayloads()) != 1 {
		t.Fatal("failed gh inspection reported outcome")
	}
}

func TestGroupSessionGitFailureKeepsItemsHeld(t *testing.T) {
	r := newDeliverRig(t, true)
	originDir, pDir, _ := initTestOriginAndGroupRepo(t, r, "github")
	var attempts atomic.Int32
	Holds["obs"] = func(*Host, SessionKey) (string, bool) {
		attempts.Add(1)
		return "", false
	}

	// 1) Initial clone failure for a group session keeps the item held with 0 acks and 0 audit rows, then delivers once Repo is reachable.
	missingOrigin := filepath.Join(t.TempDir(), "unreachable.git")
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 10}, Name: "Website", Kind: "project", Repo: missingOrigin, Base: "main", Forge: "github", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 860, AI: testSec, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "wake-clone"))
	r.waitUntil("clone failure attempt started", func() bool { return attempts.Load() >= 1 })
	r.waitAIUnbusy(testGroup1, testSec)
	if r.home.ackCountFor(860) != 0 || len(r.audits()) != 0 || len(r.home.cmdPayloads()) != 0 || len(r.runtime.recorded()) != 0 {
		t.Fatalf("group clone failure consumed item: acks=%d audits=%+v cmds=%v", r.home.ackCountFor(860), r.audits(), r.home.cmdPayloads())
	}
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 11}, Name: "Website", Kind: "project", Repo: originDir, Base: "main", Forge: "github", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	Deliver(r.h, testGroup1, testSec)
	r.waitUntil("held clone item delivered", func() bool { return r.home.ackCountFor(860) == 1 && len(r.runtime.recorded()) == 1 })
	r.waitAIUnbusy(testGroup1, testSec)

	verifyGroupRefreshFailureHoldsAndRecovers(t, r, originDir, pDir, &attempts)
}

func verifyGroupRefreshFailureHoldsAndRecovers(t *testing.T, r *deliverRig, originDir, pDir string, attempts *atomic.Int32) {
	t.Helper()
	// 2) RefreshGroupWorkspace failure when waking a Dormant group session keeps the item held and delivers on the next wake once origin recovers.
	secKey := SessionKey{Group: testGroup1, Member: testSec}
	r.h.Sessions.Move(secKey, Waiting, Dormant)
	mustGit(t, t.Context(), filepath.Join(pDir, "repo"), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "dropped.git"))
	attempts.Store(0)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 861, AI: testSec, Why: "notice", Code: "ping"}, nil)
	r.waitUntil("refresh failure attempt started", func() bool { return attempts.Load() >= 1 })
	r.waitAIUnbusy(testGroup1, testSec)
	if r.home.ackCountFor(861) != 0 || len(r.audits()) != 1 || len(r.home.cmdPayloads()) != 0 || len(r.runtime.recorded()) != 1 {
		t.Fatalf("group refresh failure consumed item: acks=%d audits=%+v cmds=%v", r.home.ackCountFor(861), r.audits(), r.home.cmdPayloads())
	}
	mustGit(t, t.Context(), filepath.Join(pDir, "repo"), "remote", "set-url", "origin", originDir)
	Deliver(r.h, testGroup1, testSec)
	r.waitUntil("held refresh item delivered", func() bool { return r.home.ackCountFor(861) == 1 && len(r.runtime.recorded()) == 2 })
	r.waitAIUnbusy(testGroup1, testSec)
}

func TestWorkspaceFailureReportingConfigUnmarshalAndStoreErrors(t *testing.T) {
	logs := captureLogs(t)
	r := newDeliverRig(t, true)
	r.seedGroupAndMembers()
	cfg, err := loadHostConfig(r.h)
	if err != nil {
		t.Fatal(err)
	}
	pDir := cfg.projectDir(testGroup1)

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
	checkMergeAndHandAuditFailure(t, r)
	dropAuditBlockTrigger(t, r.db)
	Deliver(r.h, testGroup1, testAI1)
	r.waitUntil("re-queued 901 acked", func() bool { return r.home.ackCountFor(901) == 1 })
	checkWorkspaceFailuresReportedOnce(t, r, pDir, logs)
	checkConfigUnmarshalAndDroppedTables(t, r)
}

func checkMergeAndHandAuditFailure(t *testing.T, r *deliverRig) {
	t.Helper()
	ch, done := r.h.Await(testCall1, nil)
	defer done()
	callRef := testCall1
	replyMsg := &proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg2, Seq: 40}, Pos: 40, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 902, AI: testAI1, Why: "reply", Msg: testMsg2}, replyMsg)
	if len(ch) != 1 || r.home.ackCountFor(902) != 0 {
		t.Fatal("expected handed item without ack on audit failure")
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 41}, Group: testGroup1, Num: 1, Title: "M", Slug: "m", Origin: "brief", State: "active", Step: "merge", Commit: "c1", Assignee: testAI1})
	mIt := proto.Item{Item: 903, AI: testAI1, Why: "merge", Task: testTask1, Commit: "c1"}
	d := deliv(r.h)
	v := verifiedItem{home: r.homeKey.ID(), group: testGroup1, wire: proto.InboxItem{Item: mIt}, item: mIt}
	d.enqueueMerge(v)
	d.enqueueMerge(v)
	mKey := testGroup1 + "\x00" + testTask1 + "\x00c1"
	d.claimMerge(mKey)
	if d.claimMerge(mKey) || d.takeQueuedMerge(testGroup1, 99999) {
		t.Fatal("expected duplicate claimMerge and absent takeQueuedMerge to fail")
	}
	stepMerges(r.h, d, testGroup1)
	d.unclaimMerge(mKey)
	// With audit trigger active, refuseItem skips ack and recordMergeTaken fails and re-queues item 903.
	humanID := testCanonicalID(89)
	r.h.Rows.Put(testGroup1, "member", string(humanID), json.RawMessage(marshalWire(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: string(humanID), Seq: 42}, Gone: true})))
	refuseItem(r.h, verifiedItem{home: r.homeKey.ID(), group: testGroup1, item: proto.Item{Item: 904}}, "", "not_hosted_here")
	if r.home.ackCountFor(904) != 0 {
		t.Fatal("refusal ack sent when audit write failed")
	}
	d.enqueueMerge(v)
	stepMerges(r.h, d, testGroup1)
	d.mu.Lock()
	delete(d.merges, testGroup1)
	d.mu.Unlock()
}

func checkWorkspaceFailuresReportedOnce(t *testing.T, r *deliverRig, pDir string, logs *safeLogBuffer) {
	t.Helper()
	// Put an open Waiting session on AI1 so idleMemberState returns "waiting" on AI1's TaskSession failure, while AI2 has none ("dormant").
	secretRepo := "https://ghp_1234567890abcdefghijklmnopqrstuvwxyzABCD@github.com/team/bad.git"
	os.MkdirAll(filepath.Join(pDir, "repo"), 0o700)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 50}, Name: "W", Kind: "project", Repo: secretRepo, Base: "main", Forge: "github", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 51}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 910, AI: testAI1, Why: "assign", Task: testTask1}, nil)
	r.waitUntil("member status and task fail commands", func() bool { return len(r.home.cmdPayloads()) == 2 })
	cmds := r.home.cmdPayloads()
	if !strings.Contains(cmds[0], `"code":"turn_interrupted"`) || !strings.Contains(cmds[0], `"state":"waiting"`) || !strings.Contains(cmds[1], `"op":"fail"`) {
		t.Fatalf("unexpected workspace failure commands: %v", cmds)
	}
	// Delivering again must not retry the failed item.
	r.waitAIUnbusy(testGroup1, testAI1)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.home.cmdPayloads()) != 2 {
		t.Fatalf("failed workspace item retried on later push: cmds=%d", len(r.home.cmdPayloads()))
	}
	checkRemainingWorkspaceAndMergeEdges(t, r, pDir, logs)
}

func checkRemainingWorkspaceAndMergeEdges(t *testing.T, r *deliverRig, pDir string, logs *safeLogBuffer) {
	t.Helper()
	// Merge clone failure logs and returns; RefreshGroupWorkspace, CreateTaskWorkspace, PrepareReviewWorkspace, invalid task Num==0, disconnected homePath, and failTargetSession audit failure.
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 52}, Group: testGroup1, Num: 2, Title: "M", Slug: "m", Origin: "brief", State: "active", Step: "merge", Commit: "c1", Assignee: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 911, AI: testAI2, Why: "merge", Task: testTask2, Commit: "c1"}, nil)
	r.waitUntil("merge 911 clone failure logged", func() bool {
		return r.home.ackCountFor(911) == 1 && strings.Contains(logs.String(), "host: ensure merge clone")
	})
	r.waitNoActiveMerges()

	os.RemoveAll(filepath.Join(pDir, "repo"))
	// Clone a valid repo without remote "origin": Waiting session skips RefreshGroupWorkspace and runs a turn, while Dormant session runs RefreshGroupWorkspace and holds the item.
	mustGit(t, t.Context(), "", "init", "--initial-branch=main", filepath.Join(pDir, "repo"))
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 53}, Name: "W", Kind: "project", Base: "main", Forge: "github", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	secKey := SessionKey{Group: testGroup1, Member: testSec}
	r.h.Sessions.Move(secKey, Dormant, Working)
	r.h.Sessions.Move(secKey, Working, Waiting)
	beforeTurns := len(r.runtime.recorded())
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 916, AI: testSec, Why: "notice", Code: "c"}, nil)
	r.waitUntil("sec waiting turn without refresh", func() bool { return len(r.runtime.recorded()) == beforeTurns+1 })
	r.waitAIUnbusy(testGroup1, testSec)
	var secChecks atomic.Int32
	Holds["secobs"] = func(*Host, SessionKey) (string, bool) {
		secChecks.Add(1)
		return "", false
	}
	r.h.Sessions.Move(secKey, Waiting, Dormant)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 912, AI: testSec, Why: "notice", Code: "c"}, nil)
	r.waitUntil("sec refresh attempt started", func() bool { return secChecks.Load() >= 1 })
	r.waitAIUnbusy(testGroup1, testSec)
	delete(Holds, "secobs")
	if r.home.ackCountFor(912) != 0 {
		t.Fatal("dormant group refresh failure acked item 912")
	}

	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 54}, Group: testGroup1, Num: 2, Title: "Bad", Slug: "bad", Origin: "brief", State: "active", Step: "review", Commit: "ghp_1234567890abcdefghijklmnopqrstuvwxyzABCD", Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 913, AI: testAI1, Why: "assign", Task: testTask2}, nil)
	r.waitUntil("913 failure commands", func() bool { return r.home.ackCountFor(913) == 1 && len(r.home.cmdPayloads()) == 4 })
	r.waitAIUnbusy(testGroup1, testAI1)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 55}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 914, AI: testAI2, Why: "review", Task: testTask2, Commit: "ghp_1234567890abcdefghijklmnopqrstuvwxyzABCD"}, nil)
	r.waitUntil("914 review failure and 912 group recovery", func() bool {
		return r.home.ackCountFor(912) == 1 && r.home.ackCountFor(914) == 1 && len(r.home.cmdPayloads()) == 5
	})
	r.waitAIUnbusy(testGroup1, testSec)
	r.waitAIUnbusy(testGroup1, testAI2)
	cmds := r.home.cmdPayloads()
	allCmds := strings.Join(cmds, "\n")
	if strings.Contains(allCmds, "ghp_") || !strings.Contains(allCmds, "[redacted]") || !strings.Contains(cmds[4], `"state":"dormant"`) || strings.Contains(cmds[4], `"op":"fail"`) {
		t.Fatalf("unexpected redaction or review failure report: %s", allCmds)
	}
	os.RemoveAll(filepath.Join(pDir, "repo"))

	// Invalid Num == 0 with closed home (homePath == "") and audit trigger blocking failTargetSession commit.
	r.home.closeConn()
	tZero := testCanonicalID(220)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(tZero), Seq: 56}, Group: testGroup1, Num: 0, Title: "Z", Slug: "z", Origin: "brief", State: "active", Step: "branch", Assignee: testAI2})
	addAuditBlockTrigger(t, r.db)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 915, AI: testAI2, Why: "assign", Task: tZero}, nil)
	r.waitUntil("915 blocked audit logged", func() bool { return strings.Contains(logs.String(), "host: commit failed-workspace items") })
	r.waitAIUnbusy(testGroup1, testAI2)
	dropAuditBlockTrigger(t, r.db)
	Deliver(r.h, testGroup1, testAI2)
	r.waitAIUnbusy(testGroup1, testAI2)
}

func checkConfigUnmarshalAndDroppedTables(t *testing.T, r *deliverRig) {
	t.Helper()
	logs := captureLogs(t)
	mem := newDeliverRig(t, false)
	runReviewedMerge(mem.h, mem.homeKey.ID(), testGroup1, proto.TaskRow{RowHeader: proto.RowHeader{ID: testTask1}, Num: 1, Slug: "m"}, "c1", 0, false, time.Time{})
	resumeGroupMerges(mem.h, deliv(mem.h), "unknown-group")

	// Session INSERT trigger fails ensureSessionRow; non-array 'homes' JSON returns json.Unmarshal error in loadHostConfig.
	writeTrigger := func(tx *store.Tx) error {
		_, err := tx.Exec(`CREATE TRIGGER block_session BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
		return err
	}
	r.db.Write(writeTrigger)
	r.home = newFakeDeliverHome()
	r.h.Attach(r.homeKey.ID(), r.home)
	r.waitGroupLive(testGroup1)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 949, AI: testAI2, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "hi"))
	r.waitUntil("session create failure logged", func() bool { return strings.Contains(logs.String(), "host: create session") })
	r.waitAIUnbusy(testGroup1, testAI2)

	r.setSetting("homes", 123)
	if _, err := loadHostConfig(r.h); err == nil {
		t.Fatal("expected loadHostConfig unmarshal error for non-array homes")
	}
	r.h.Attach(r.homeKey.ID(), r.home)
	Deliver(r.h, testGroup1, testAI2)
	runReviewedMerge(r.h, r.homeKey.ID(), testGroup1, proto.TaskRow{RowHeader: proto.RowHeader{ID: testTask1}, Num: 1, Slug: "m"}, "c1", 0, false, time.Time{})

	// Drop audit to cover hasAuditItem error, then drop setting to cover SQLite error in loadHostConfig.
	dropAudit := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE audit")
		return err
	}
	r.db.Write(dropAudit)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: 960, AI: testAI1, Why: "notice", Code: "c"}, nil)
	dropSetting := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE setting")
		return err
	}
	r.db.Write(dropSetting)
	if _, err := loadHostConfig(r.h); err == nil {
		t.Fatal("expected loadHostConfig error when setting table dropped")
	}
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

func TestResumeHostedChecksAfterRestart(t *testing.T) {
	r1 := newDeliverRig(t, true)
	tskStopAllCloud(t, r1.h)
	originDir, pDir, head := initTestOriginAndGroupRepo(t, r1, "github")
	_, err := EnsureGroupWorkspace(t.Context(), pDir, originDir, "main")
	tskNoErr(t, err)
	_, err = CreateTaskWorkspace(t.Context(), pDir, 1, "cloud", testAI1, "main", true)
	tskNoErr(t, err)
	rowsAt := func(seq int64) json.RawMessage { return cloudCheckRows(r1, head, seq) }

	// Group with no home entry skips hosted-check resume; an already-running check is left alone.
	r1.h.Rows.Put("unknown-group", "member", testAI1, json.RawMessage(marshalWire(cloudCheckAI(r1, testAI1, r1.hostKey.ID()))))
	r1.h.Rows.Put("unknown-group", "task", testTask1, json.RawMessage(marshalWire(cloudCheckTask("unknown-group", testTask1, 1, testAI1, head, 2))))
	noCalls := fakeForgeGH(t)
	afterGroupState(r1.h, deliv(r1.h), "unknown-group")

	key := tskCloudKey{group: testGroup1, task: testTask1}
	ctx1, run1 := tskBeginCloud(Part[tskPart](r1.h), key, head)
	r1.h.Push(r1.homeKey.ID(), rowsAt(10))
	afterGroupState(r1.h, deliv(r1.h), testGroup1)
	startHostedCheck(r1.h, r1.homeKey.ID(), testGroup1, HostedCheck{Task: testTask1, Branch: "task/1-cloud", Commit: head, PR: 31})
	tskRequire(t, ctx1.Err() == nil && len(r1.home.cmdPayloads()) == 0, "running check disturbed")
	tskFinishCloud(Part[tskPart](r1.h), key, run1)
	noCalls()

	verifyRestartCloudPollOnce(t, r1.db, head, rowsAt)
}

func verifyRestartCloudPollOnce(t *testing.T, db *store.Store, head string, rowsAt func(int64) json.RawMessage) {
	t.Helper()
	r2 := newDeliverRigOnStore(t, db)
	tskStopAllCloud(t, r2.h)
	calls := fakeForgeGH(t, hostedView(head, passedRun), hostedView(head, passedRun))

	// First resume loses task cloud to home_unreachable; reconnect retries it once.
	r2.home.cmdErr = errors.New("unreachable")
	r2.h.Attach(r2.homeKey.ID(), r2.home)
	r2.waitGroupLive(testGroup1)
	r2.h.Push(r2.homeKey.ID(), rowsAt(20))
	r2.waitUntil("failed cloud command", func() bool { return len(r2.home.cmdPayloads()) == 1 })
	waitCloudIdle(r2)

	var secondRowsChecks atomic.Int32
	Holds["obs"] = func(*Host, SessionKey) (string, bool) {
		secondRowsChecks.Add(1)
		return "cap", true
	}
	r2.pushItem(r2.homeKey, r2.homeKey.ID(), testGroup1, proto.Item{Item: 980, AI: testAI1, Why: "notice", Code: "c"}, nil)
	r2.waitAIUnbusy(testGroup1, testAI1)

	inFlight := make(chan struct{}, 1) // single reader: verifyRestartCloudPollOnce
	release := make(chan struct{})     // single reader: r3.onCmd
	r3 := newFakeDeliverHome()
	t.Cleanup(r3.closeConn)
	r3.onCmd = func(ctx context.Context, _ proto.Cmd) (json.RawMessage, error) {
		inFlight <- struct{}{}
		<-release
		done := cloudCheckTask(testGroup1, testTask1, 1, testAI1, head, 30)
		done.Step = "review"
		r2.putTask(done)
		return json.RawMessage(`{"re":1,"ok":true}`), ctx.Err()
	}
	r2.h.Attach(r2.homeKey.ID(), r3)
	<-inFlight
	secondRowsChecks.Store(0)
	r2.h.Push(r2.homeKey.ID(), rowsAt(25))
	r2.waitUntil("second rows push evaluated", func() bool { return secondRowsChecks.Load() >= 1 })
	close(release)
	waitCloudIdle(r2)

	view := []string{"pr", "view", "31", "--json", "headRefOid,statusCheckRollup"}
	calls(view, view)
	tskEqual(t, len(r3.cmdPayloads()), 1)
	var cloud proto.TaskChecked
	tskNoErr(t, proto.Decode([]byte(r3.cmdPayloads()[0]), &cloud))
	tskEqual(t, cloud.Op, "cloud")
	tskEqual(t, cloud.OK, true)
	tskEqual(t, cloud.Commit, head)
}

func waitCloudIdle(r *deliverRig) {
	r.t.Helper()
	p := Part[tskPart](r.h)
	r.waitUntil("cloud polls idle", func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.cancels) == 0
	})
}

func cloudCheckAI(r *deliverRig, id, hostID string) proto.AIMemberRow {
	return proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 7}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "AI"}, CLI: "claude", Host: hostID, HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}
}

func cloudCheckTask(group, id string, num int, assignee, commit string, seq int64) proto.TaskRow {
	return proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id, Seq: seq}, Group: group, Num: num, Title: "Cloud", Slug: "cloud", Origin: "brief", State: "active", Step: "cloud_check", Assignee: assignee, Commit: commit, PR: 31, Depends: []string{}, Checks: proto.TaskChecks{Local: &proto.Check{OK: true, At: time.Now().UnixMilli(), Commit: commit}}}
}

func cloudCheckRows(r *deliverRig, head string, seq int64) json.RawMessage {
	raw := func(v any) json.RawMessage { return json.RawMessage(marshalWire(v)) }
	human := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testHuman, Seq: 5}, Group: testGroup1, Kind: "human", User: r.ownerKey.ID(), Name: "Owner"}, Role: "owner"}
	gone := proto.GoneRow{RowHeader: proto.RowHeader{Type: "task", ID: string(testCanonicalID(99)), Seq: 6}, Gone: true}
	return raw(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{
		raw(cloudCheckAI(r, testAI1, r.hostKey.ID())), raw(cloudCheckAI(r, testAI2, r.extraKey.ID())),
		raw(human), raw(gone), raw(cloudCheckTask(testGroup1, testTask2, 2, testAI2, head, 9)),
		raw(cloudCheckTask(testGroup1, testTask1, 1, testAI1, head, seq)),
	}})
}
