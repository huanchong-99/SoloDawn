package host

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	testCard2   = "GGGGGGGGGGGGGGGGGGGGGg"
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
	err := r.err
	keep := r.keepRun
	hook := r.onTurn
	r.turns = append(r.turns, recordedTurn{host: h, key: key, text: text, turn: turn})
	r.mu.Unlock()
	if err != nil {
		return err
	}
	r.advanceState(h, key, keep)
	if hook != nil {
		hook(h, key)
	}
	return nil
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
	syncMode bool
	started  chan proto.Sync // read by waitSyncStart
	answers  chan syncAnswer // read by Request when syncMode is true
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

func (f *fakeDeliverHome) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.cmds) + len(f.syncs) + 1)
	req := build(id)
	syncMode := f.syncMode
	ansText, ansErr := f.cmdAns, f.cmdErr
	f.recordReqLocked(req)
	f.mu.Unlock()
	if syncReq, ok := req.(proto.Sync); ok && syncMode {
		return f.waitSyncAnswer(ctx, syncReq)
	}
	if syncReq, ok := req.(proto.Sync); ok {
		raw := encodeJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: id, OK: true}, Group: syncReq.Group, Seq: syncReq.Since})
		return json.RawMessage(raw), nil
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
	t        *testing.T
	db       *store.Store
	h        *Host
	ownerKey identity.Key
	hostKey  identity.Key
	homeKey  identity.Key
	home2Key identity.Key
	extraKey identity.Key
	home     *fakeDeliverHome
	runtime  *fakeDeliverRuntime
	composeE error
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
	rig.seedOwnerDeviceLog()
	clock := func() time.Time { return time.UnixMilli(testStartMS) }
	h, _, err := New(db, rig.hostKey, clock)
	if err != nil {
		t.Fatal(err)
	}
	rig.h = h
	t.Cleanup(func() { stopDeliverySyncs(h) })
	return rig
}

func stopDeliverySyncs(h *Host) {
	d := deliv(h)
	d.mu.Lock()
	for _, cancel := range d.cancels {
		cancel()
	}
	d.mu.Unlock()
	d.wg.Wait()
}

func (r *deliverRig) installHooks() {
	prevRunner, prevCompose, prevHolds := Runner, Compose, Holds
	Holds = map[string]func(*Host, SessionKey) (string, bool){}
	Runner = r.runtime
	Compose = func(_ *Host, s Session, kind TurnKind, items []json.RawMessage) (string, Turn, error) {
		if r.composeE != nil {
			return "", Turn{}, r.composeE
		}
		return string(s.Kind) + ":" + strconv.Itoa(len(items)), Turn{Kind: kind, Thread: s.Task}, nil
	}
	r.t.Cleanup(func() {
		Runner = prevRunner
		Compose = prevCompose
		Holds = prevHolds
	})
}

func (r *deliverRig) seedOwnerDeviceLog() {
	ownerID := r.ownerKey.ID()
	devs := []struct {
		ver  int
		id   string
		kind string
	}{
		{1, ownerID, "browser"},
		{2, r.hostKey.ID(), "node"},
		{3, r.homeKey.ID(), "node"},
		{4, r.home2Key.ID(), "node"},
	}
	write := func(tx *store.Tx) error {
		tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES('owner', ?)", encodeJSON(ownerID))
		for _, d := range devs {
			env := r.ownerKey.Sign(encodeJSON(map[string]any{
				"t": "device", "n": testCanonicalID(byte(d.ver)), "ts": testStartMS,
				"user": ownerID, "ver": d.ver, "op": "add", "dev": d.id, "kind": d.kind, "name": "d",
			}))
			if _, err := tx.Exec("INSERT OR REPLACE INTO device_log(user, ver, env) VALUES(?, ?, ?)", ownerID, d.ver, encodeJSON(env)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := r.db.Write(write); err != nil {
		r.t.Fatal(err)
	}
}

func (r *deliverRig) setSetting(key string, val any) {
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)", key, encodeJSON(val))
		return err
	}
	if err := r.db.Write(write); err != nil {
		r.t.Fatal(err)
	}
}

func (r *deliverRig) seedGroupAndMembers() {
	r.putGroup(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1},
		Name:      "Proj", Kind: "project", Base: "main", Forge: "none",
		Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{},
	})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, nil, 2)
	r.putAIMember(testAI2, "Codex", r.hostKey.ID(), false, nil, 3)
	r.putAIMember(testSec, "Sec", r.hostKey.ID(), true, nil, 4)
	r.h.Attach(r.homeKey.ID(), r.home)
}

func (r *deliverRig) putGroup(grp proto.GroupRow) {
	r.h.Rows.Put(grp.ID, "group", grp.ID, json.RawMessage(encodeJSON(grp)))
}

func (r *deliverRig) putAIMember(id, name, hostID string, sec bool, approvers []string, seq int64) {
	if approvers == nil {
		approvers = []string{}
	}
	m := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: seq}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: name},
		CLI:       "claude", Host: hostID, HostName: "n", Secretary: sec, Account: "personal", Allow: []string{}, Approvers: approvers, State: "dormant", Path: "local",
	}
	r.h.Rows.Put(testGroup1, "member", id, json.RawMessage(encodeJSON(m)))
}

func (r *deliverRig) putTask(task proto.TaskRow) {
	if task.Depends == nil {
		task.Depends = []string{}
	}
	r.h.Rows.Put(task.Group, "task", task.ID, json.RawMessage(encodeJSON(task)))
}

func (r *deliverRig) makeTextMsg(id proto.ID, thread *string, text string) *proto.MsgRow {
	return &proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(id), Seq: 10},
		Pos:       10, Group: testGroup1, Thread: thread, Main: thread == nil,
		By: testHuman, Kind: "text", Timestamp: testStartMS, Text: text, Mentions: []string{}, Body: json.RawMessage(`{}`),
	}
}

func (r *deliverRig) pushItem(signer identity.Key, connHome string, group proto.ID, it proto.Item, msg *proto.MsgRow, cause *proto.Envelope) {
	wire := proto.InboxItem{Env: signer.Sign(encodeJSON(it)), Cause: cause}
	if msg != nil {
		wire.Msg = json.RawMessage(encodeJSON(msg))
	}
	frame := encodeJSON(proto.Inbox{Type: "inbox", Group: group, Items: []proto.InboxItem{wire}})
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
		json.Unmarshal([]byte(raw), &rows)
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

func TestRegistrationsAndHostIsolation(t *testing.T) {
	assertRegisteredTables(t)
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
	it := proto.Item{Type: "item", Nonce: testCanonicalID(20), Timestamp: testStartMS, Group: testGroup1, To: r1.hostKey.ID(), Item: 1, AI: testAI1, Why: "mention", Msg: testMsg1}
	r1.pushItem(r1.homeKey, r1.homeKey.ID(), testGroup1, it, msg, nil)
	turns := r1.runtime.recorded()
	if len(turns) != 1 || turns[0].host != r1.h || len(deliv(h2).groups) != 0 {
		t.Fatalf("isolation broken: turns=%+v h2Groups=%d", turns, len(deliv(h2).groups))
	}
}

func assertRegisteredTables(t *testing.T) {
	t.Helper()
	if Attached["deliver"] == nil || Pushes["inbox"] == nil || Pushes["rows"] == nil {
		t.Fatal("deliver registrations missing")
	}
	if Pushes["reset"] == nil || Deliver == nil {
		t.Fatal("reset or Deliver hook missing")
	}
}

func TestAttachAndGroupsForHomeSources(t *testing.T) {
	r := newDeliverRig(t, false)
	closed := newFakeDeliverHome()
	close(closed.done)
	r.h.Attach(r.homeKey.ID(), closed)
	r.h.Attach(r.homeKey.ID(), r.home)
	if len(closed.syncSinces()) != 0 || len(r.home.syncSinces()) != 0 {
		t.Fatal("closed or empty host sent sync")
	}
	verifyAttachedHomeSyncs(t, r)
}

func verifyAttachedHomeSyncs(t *testing.T, r *deliverRig) {
	t.Helper()
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1}, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: "other-g", Member: testAI2}, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.home2Key.ID(), Name: "C"})
	r.setSetting("homes", []proto.Home{
		{Group: testGroup2, Home: r.homeKey.ID(), Name: "Two"},
		{Group: testCanonicalID(99), Home: r.home2Key.ID(), Name: "Other"},
	})
	r.home.syncMode = true
	r.h.Attach(r.homeKey.ID(), r.home)
	expectInitialSync(t, r.home, testGroup1, 5)
	expectInitialSync(t, r.home, testGroup2, 8)
}

func expectInitialSync(t *testing.T, home *fakeDeliverHome, wantGroup string, ansSeq int64) {
	t.Helper()
	req := <-home.started
	if req.Group != wantGroup || !req.StateOnly || req.Since != 0 {
		t.Fatalf("unexpected sync: %+v", req)
	}
	home.answers <- syncAnswer{raw: json.RawMessage(encodeJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: wantGroup, Seq: ansSeq}))}
}

func TestSyncCursorResetDeltaAndTokenSupersession(t *testing.T) {
	r := newDeliverRig(t, false)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "One"}})
	// First attach is superseded by a second attach before first Request answers.
	first := newFakeDeliverHome()
	first.syncMode = true
	r.h.Attach(r.homeKey.ID(), first)
	<-first.started

	second := newFakeDeliverHome()
	second.syncMode = true
	r.h.Attach(r.homeKey.ID(), second)
	<-second.started
	// Unblock stale first sync with seq 99: token mismatch must ignore it.
	first.answers <- syncAnswer{raw: json.RawMessage(encodeJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 99}))}
	// Second sync fails after a reset + partial row: cursor must stay 0.
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Reset{Type: "reset", Group: testGroup1})))
	grpRaw := json.RawMessage(encodeJSON(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 4}, Name: "P", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpRaw}})))
	second.answers <- syncAnswer{err: errors.New("broken")}
	waitForSyncSettled(t, r.h, testGroup1)

	// Third attach must ask since 0, then succeed at seq 10.
	third := newFakeDeliverHome()
	third.syncMode = true
	r.h.Attach(r.homeKey.ID(), third)
	req3 := <-third.started
	if req3.Since != 0 {
		t.Fatalf("after interrupted reset got since=%d, want 0", req3.Since)
	}
	third.answers <- syncAnswer{raw: json.RawMessage(encodeJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 10}))}
	waitForSyncSettled(t, r.h, testGroup1)
	assertLiveCursor(t, r)
}

func assertLiveCursor(t *testing.T, r *deliverRig) {
	t.Helper()
	// Live row seq 12 advances cursor; equal seq 12 and lower seq 11 do not move it.
	putRowSeq := func(seq int64, name string) {
		m := proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: seq}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: name},
			CLI:       "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local",
		}
		r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{json.RawMessage(encodeJSON(m))}})))
	}
	putRowSeq(12, "first-12")
	putRowSeq(12, "dup-12")
	putRowSeq(11, "old-11")
	var got proto.AIMemberRow
	raw, _ := r.h.Rows.Get(testGroup1, "member", testAI1)
	proto.Decode(raw, &got)
	if got.Name != "first-12" {
		t.Fatalf("row overwritten by equal/lower seq: %q", got.Name)
	}
	// Interrupted delta (wrong group in Synced) keeps cursor 12.
	fourth := newFakeDeliverHome()
	fourth.syncMode = true
	r.h.Attach(r.homeKey.ID(), fourth)
	req4 := <-fourth.started
	if req4.Since != 12 {
		t.Fatalf("got since=%d, want 12", req4.Since)
	}
	fourth.answers <- syncAnswer{raw: json.RawMessage(encodeJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup2, Seq: 50}))}
	waitForSyncSettled(t, r.h, testGroup1)

	fifth := newFakeDeliverHome()
	fifth.syncMode = true
	r.h.Attach(r.homeKey.ID(), fifth)
	req5 := <-fifth.started
	if req5.Since != 12 {
		t.Fatalf("after failed delta got since=%d, want 12", req5.Since)
	}
	fifth.answers <- syncAnswer{raw: json.RawMessage(`{"re":1,"err":{"code":"forbidden","msg":"no"}}`)}
	waitForSyncSettled(t, r.h, testGroup1)
}

func waitForSyncSettled(t *testing.T, h *Host, _ string) {
	t.Helper()
	deliv(h).wg.Wait()
}

func TestRowsAndResetValidationBoundaries(t *testing.T) {
	r := newDeliverRig(t, false)
	// Malformed frames and empty groups are dropped.
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"reset","g":1}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"reset","g":""}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"rows","g":1}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"rows","g":"","rows":[]}`))
	// Stranger device rejected when group home is unknown and when group home is known.
	r.h.Push(r.extraKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{}})))
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 2}, Name: "P", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}})
	r.h.Push(r.home2Key.ID(), json.RawMessage(encodeJSON(proto.Reset{Type: "reset", Group: testGroup1})))
	if _, ok := r.h.Rows.Get(testGroup1, "group", testGroup1); !ok {
		t.Fatal("wrong home reset dropped group row")
	}
	checkRowHeaderAndGroupValidation(t, r)
}

func checkRowHeaderAndGroupValidation(t *testing.T, r *deliverRig) {
	t.Helper()
	badRows := []json.RawMessage{
		json.RawMessage(`"not-object"`),
		json.RawMessage(`{"r":"","id":"x","seq":1}`),
		json.RawMessage(`{"r":"note","id":"","seq":1}`),
		json.RawMessage(`{"r":"note","id":"x","seq":0}`),
		json.RawMessage(`{"r":"note","id":"x","seq":-1}`),
		json.RawMessage(`{"r":"group","id":"` + testGroup2 + `","seq":5}`),
		json.RawMessage(encodeJSON(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: "wrong", Seq: 5}, Name: "P", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}})),
		json.RawMessage(encodeJSON(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 5}, Name: "P", Kind: "project", Base: "main", Forge: "none", Home: r.home2Key.ID(), Checks: []string{}})),
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: badRows})))
	if _, ok := r.h.Rows.Get(testGroup1, "note", "x"); ok {
		t.Fatal("invalid row header was stored")
	}
	// Seq == 1 is valid, and corrupt cached row is overwritten.
	r.h.Rows.Put(testGroup1, "note", "corrupt", json.RawMessage(`{bad`))
	validRows := []json.RawMessage{
		json.RawMessage(`{"r":"note","id":"n1","seq":1}`),
		json.RawMessage(`{"r":"note","id":"corrupt","seq":1}`),
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: validRows})))
	if _, ok := r.h.Rows.Get(testGroup1, "note", "n1"); !ok {
		t.Fatal("seq 1 row was rejected")
	}
	var replaced proto.RowHeader
	if raw, ok := r.h.Rows.Get(testGroup1, "note", "corrupt"); !ok || proto.Decode(raw, &replaced) != nil || replaced.Seq != 1 {
		t.Fatalf("corrupt cached row not replaced: %s", raw)
	}
}

func TestKnownHomePrecedenceAndRevokedHome(t *testing.T) {
	r := newDeliverRig(t, false)
	// Stored home from setting 'homes'.
	r.setSetting("homes", []proto.Home{{Group: testGroup2, Home: r.home2Key.ID(), Name: "Two"}})
	cfg, _ := loadHostConfig(r.h)
	d := deliv(r.h)
	if got := knownHome(r.h, d, cfg, testGroup2); got != r.home2Key.ID() {
		t.Fatalf("got %q from homes setting", got)
	}
	if got := knownHome(r.h, d, cfg, "unknown-group"); got != "" {
		t.Fatalf("got %q for unknown group", got)
	}
	// Session home takes precedence over setting 'homes'.
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup2, Member: testAI1}, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	if got := knownHome(r.h, d, cfg, testGroup2); got != r.homeKey.ID() {
		t.Fatalf("got %q from session", got)
	}
	// Revoking homeKey in owner's log causes acceptGroupPush to reject even when known == home.
	ownerID := r.ownerKey.ID()
	rev := r.ownerKey.Sign(encodeJSON(map[string]any{"t": "device", "n": testCanonicalID(50), "ts": testStartMS, "user": ownerID, "ver": 5, "op": "revoke", "dev": r.homeKey.ID()}))
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, 5, ?)", ownerID, encodeJSON(rev))
		return err
	}
	r.db.Write(write)
	if _, ok := acceptGroupPush(r.h, d, r.homeKey.ID(), testGroup2); ok {
		t.Fatal("revoked home device was accepted")
	}
}

func TestBoundaryRefusalsAndItemShapes(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	// Malformed inbox frame and malformed element ignored.
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"inbox","g":"not-canonical","items":[]}`))
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"inbox","g":"`+testGroup1+`","items":[123]}`))
	// Signer differs from push home and wrong expected home.
	it1 := proto.Item{Type: "item", Nonce: testCanonicalID(1), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 1, AI: testAI1, Why: "mention", Msg: testMsg1}
	r.pushItem(r.home2Key, r.homeKey.ID(), testGroup1, it1, nil, nil)
	it2 := it1
	it2.Item = 2
	r.pushItem(r.home2Key, r.home2Key.ID(), testGroup1, it2, nil, nil)
	// Invalid signature / wrong target host.
	it3 := it1
	it3.Item = 3
	it3.To = r.extraKey.ID()
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it3, nil, nil)
	checkPayloadAndMsgBoundaries(t, r)
	checkAllWhyShapeBoundaries(t, r)
}

func checkPayloadAndMsgBoundaries(t *testing.T, r *deliverRig) {
	t.Helper()
	// Missing required nonce passes OpenCommand? OpenCommand requires valid JSON; missing 'ai' fails strict proto.Decode.
	badPayload := r.homeKey.Sign(`{"t":"item","n":"` + string(testCanonicalID(5)) + `","ts":1763600000000,"g":"` + testGroup1 + `","to":"` + r.hostKey.ID() + `","item":4,"why":"mention"}`)
	frame := encodeJSON(proto.Inbox{Type: "inbox", Group: testGroup1, Items: []proto.InboxItem{{Env: badPayload}}})
	r.h.Push(r.homeKey.ID(), json.RawMessage(frame))
	// Item == 0 and Item == -1 write refused audit with item=0 and send NO ack for 0/-1.
	acksBefore := len(r.home.ackList())
	for _, num := range []int64{0, -1} {
		it := proto.Item{Type: "item", Nonce: testCanonicalID(6), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: num, AI: testAI1, Why: "mention", Msg: testMsg1}
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil, nil)
	}
	if len(r.home.ackList()) != acksBefore {
		t.Fatal("ack sent for item < 1")
	}
	// Wrong Type, wrong Group, and invalid Msg JSON.
	wrongType := proto.Item{Type: "not_item", Nonce: testCanonicalID(7), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 5, AI: testAI1, Why: "mention", Msg: testMsg1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, wrongType, nil, nil)
	wrongGrp := proto.Item{Type: "item", Nonce: testCanonicalID(8), Timestamp: testStartMS, Group: testGroup2, To: r.hostKey.ID(), Item: 6, AI: testAI1, Why: "mention", Msg: testMsg1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, wrongGrp, nil, nil)
	validIt := proto.Item{Type: "item", Nonce: testCanonicalID(9), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 7, AI: testAI1, Why: "mention", Msg: testMsg1}
	badMsgWire := proto.InboxItem{Env: r.homeKey.Sign(encodeJSON(validIt)), Msg: json.RawMessage(`{"invalid":true}`)}
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Inbox{Type: "inbox", Group: testGroup1, Items: []proto.InboxItem{badMsgWire}})))
}

func checkAllWhyShapeBoundaries(t *testing.T, r *deliverRig) {
	t.Helper()
	dummyCause := r.ownerKey.Sign(`{"t":"act"}`)
	badCases := []struct {
		item  proto.Item
		cause *proto.Envelope
	}{
		{proto.Item{Why: "mention"}, nil},
		{proto.Item{Why: "reply"}, nil},
		{proto.Item{Why: "assign"}, nil},
		{proto.Item{Why: "resume"}, nil},
		{proto.Item{Why: "review", Task: testTask1}, nil},
		{proto.Item{Why: "review", Commit: "abc"}, nil},
		{proto.Item{Why: "merge", Task: testTask1}, nil},
		{proto.Item{Why: "merge", Commit: "abc"}, nil},
		{proto.Item{Why: "reviewed", Task: testTask1}, nil},
		{proto.Item{Why: "reviewed", Msg: testMsg1}, nil},
		{proto.Item{Why: "checks", Task: testTask1}, nil},
		{proto.Item{Why: "checks", Msg: testMsg1}, nil},
		{proto.Item{Why: "decided", Msg: testMsg1}, nil},
		{proto.Item{Why: "decided"}, &dummyCause},
		{proto.Item{Why: "confirmed", Msg: testMsg1}, nil},
		{proto.Item{Why: "confirmed"}, &dummyCause},
		{proto.Item{Why: "notice"}, nil},
		{proto.Item{Why: "unknown_why", Task: testTask1, Msg: testMsg1, Commit: "abc", Code: "c"}, &dummyCause},
	}
	for i, tc := range badCases {
		it := tc.item
		it.Type, it.Nonce, it.Timestamp, it.Group, it.To, it.Item, it.AI = "item", testCanonicalID(byte(20+i)), testStartMS, testGroup1, r.hostKey.ID(), int64(100+i), testAI1
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil, tc.cause)
	}
	if len(r.runtime.recorded()) != 0 {
		t.Fatalf("bad shapes ran %d turns", len(r.runtime.recorded()))
	}
}

func TestNotHostedHereAndSessionFallbackLookup(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	// AI hosted on another node, human member, and absent member are refused.
	r.putAIMember(testAI2, "RemoteAI", r.extraKey.ID(), false, nil, 5)
	human := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testHuman, Seq: 6}, Group: testGroup1, Kind: "human", User: r.ownerKey.ID(), Name: "Owner"}, Role: "owner", Online: true}
	r.h.Rows.Put(testGroup1, "member", testHuman, json.RawMessage(encodeJSON(human)))
	for i, targetAI := range []proto.ID{testAI2, testHuman, testCanonicalID(88)} {
		it := proto.Item{Type: "item", Nonce: testCanonicalID(byte(30 + i)), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: int64(200 + i), AI: targetAI, Why: "mention", Msg: testMsg1}
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil, nil)
	}
	if len(r.runtime.recorded()) != 0 {
		t.Fatal("non-local member received turn")
	}
	// When h.Rows has no member row, lookupSessionAI resolves from Sessions.
	fallbackAI := testCanonicalID(77)
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: string(fallbackAI)}, Kind: GroupSession, Folder: "repo", Agent: "codex", Home: r.homeKey.ID(), Name: "Fallback"})
	it := proto.Item{Type: "item", Nonce: testCanonicalID(35), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 205, AI: fallbackAI, Why: "mention", Msg: testMsg1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, r.makeTextMsg(testMsg1, nil, "ping"), nil)
	if len(r.runtime.recorded()) != 1 {
		t.Fatalf("session fallback turns=%d, want 1", len(r.runtime.recorded()))
	}
}

func TestOpenToolWaitsReplyAndApproval(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	// AI is Working; open ask receives matching reply immediately without queueing a turn.
	gKey := SessionKey{Group: testGroup1, Member: testAI1}
	r.h.Sessions.Create(Session{SessionKey: gKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.h.Sessions.Move(gKey, Dormant, Working)

	askCh, doneAsk := r.h.Await(testCall1, nil)
	defer doneAsk()
	checkReplyNonMatchingCases(t, r)
	// Matching reply: msg.ID == item.Msg, Kind == "reply", Re == &testCall1.
	callRef := testCall1
	replyMsg := &proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: testMsg1, Seq: 15},
		Pos:       15, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS,
		Text: "here", Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`),
	}
	it := proto.Item{Type: "item", Nonce: testCanonicalID(49), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 310, AI: testAI1, Why: "reply", Msg: testMsg1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, replyMsg, nil)
	if len(askCh) != 1 {
		t.Fatal("open ask did not receive reply")
	}
	checkOpenApprovalWait(t, r)
}

func checkReplyNonMatchingCases(t *testing.T, r *deliverRig) {
	t.Helper()
	callRef := testCall1
	// Non-matching reply cases: nil msg, mismatched msg ID, non-reply kind, nil Re.
	cases := []*proto.MsgRow{
		nil,
		{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg2, Seq: 11}, Pos: 11, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)},
		{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg1, Seq: 12}, Pos: 12, Group: testGroup1, By: testSec, Kind: "text", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)},
		{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg1, Seq: 13}, Pos: 13, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: nil, Body: json.RawMessage(`{}`)},
	}
	for i, msg := range cases {
		it := proto.Item{Type: "item", Nonce: testCanonicalID(byte(40 + i)), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: int64(300 + i), AI: testAI1, Why: "reply", Msg: testMsg1}
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, msg, nil)
	}
}

func checkOpenApprovalWait(t *testing.T, r *deliverRig) {
	t.Helper()
	attempts := 0
	appCh, doneApp := r.h.Await(testCard1, func(json.RawMessage) error {
		attempts++
		if attempts == 1 {
			return errors.New("bad_approver")
		}
		return nil
	})
	defer doneApp()
	cause := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(51), Timestamp: testStartMS, Group: testGroup1}, Msg: testCard1, Action: "approve"}))
	// First attempt refused by check -> AuditRefused with cause.Key; wait remains open for second attempt.
	it1 := proto.Item{Type: "item", Nonce: testCanonicalID(52), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 320, AI: testAI1, Why: "decided", Msg: testCard1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it1, nil, &cause)
	if len(appCh) != 0 {
		t.Fatal("refused approval reached channel")
	}
	it2 := it1
	it2.Item = 321
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it2, nil, &cause)
	if len(appCh) != 1 {
		t.Fatal("accepted approval did not reach channel")
	}
	assertHandedAudits(t, r.audits(), r.homeKey.ID(), r.ownerKey.ID())
}

func assertHandedAudits(t *testing.T, got []auditSummary, homeID, ownerID string) {
	t.Helper()
	want := []auditSummary{
		{what: AuditDelivered, item: 310, home: homeID, agent: "claude", detail: "reply", by: homeID},
		{what: AuditRefused, item: 320, home: homeID, agent: "claude", detail: "bad_approver", by: ownerID},
		{what: AuditDelivered, item: 321, home: homeID, agent: "claude", detail: "decided", by: ownerID},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("audits=%+v, want %+v", got, want)
	}
}

func TestLateApprovalCauseValidationAndAtomicStateUpdate(t *testing.T) {
	r := newDeliverRig(t, false)
	approverKey := testSeedKey(60)
	approverUser := approverKey.ID()
	r.seedGroupAndMembers()
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, []string{approverUser}, 7)
	// Seed approver's 2-entry device log out of version order (2 then 1) plus a corrupt user's log to exercise buildUserLog.
	seedApproverDeviceRows(t, r, approverKey)
	insertPendingApproval(t, r.db, testCard1, testGroup1, testAI1, testTask1)
	insertPendingApproval(t, r.db, testCard2, testGroup1, testAI1, testTask1)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 8}, Group: testGroup1, Num: 1, Title: "T", Slug: "t", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1})

	checkInvalidApprovalCauses(t, r)
	// Spoofed msg.Kind == "decision" for an actual pending approval card must be refused.
	spoofMsg := &proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: testCard1, Seq: 9}, Pos: 9, Group: testGroup1, By: testSec, Kind: "decision", Timestamp: testStartMS, Mentions: []string{}, Body: json.RawMessage(`{}`)}
	validCause := approverKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(70), Timestamp: testStartMS, Group: testGroup1}, Msg: testCard1, Action: "approve"}))
	spoofIt := proto.Item{Type: "item", Nonce: testCanonicalID(71), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 450, AI: testAI1, Why: "decided", Task: testTask1, Msg: testCard1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, spoofIt, spoofMsg, &validCause)
	if approvalStateInDB(t, r.db, testCard1) != "pending" {
		t.Fatal("spoofed decision altered pending approval")
	}
	// Valid late approve by named approver marks card 'granted'; valid late deny deletes testCard2.
	appIt := spoofIt
	appIt.Item = 451
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, appIt, nil, &validCause)
	if approvalStateInDB(t, r.db, testCard1) != "granted" {
		t.Fatalf("card1 state=%q, want granted", approvalStateInDB(t, r.db, testCard1))
	}
	denyCause := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(72), Timestamp: testStartMS, Group: testGroup1}, Msg: testCard2, Action: "deny"}))
	denyIt := proto.Item{Type: "item", Nonce: testCanonicalID(73), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 452, AI: testAI1, Why: "decided", Task: testTask1, Msg: testCard2}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, denyIt, nil, &denyCause)
	if approvalStateInDB(t, r.db, testCard2) != "" {
		t.Fatalf("card2 not deleted: %q", approvalStateInDB(t, r.db, testCard2))
	}
	// Real decision card (no row in approval table) is delivered without approval state mutation.
	decID := testCanonicalID(74)
	decMsg := &proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(decID), Seq: 12}, Pos: 12, Group: testGroup1, By: testSec, Kind: "decision", Timestamp: testStartMS, Mentions: []string{}, Body: json.RawMessage(`{}`)}
	chooseCause := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(75), Timestamp: testStartMS, Group: testGroup1}, Msg: decID, Action: "choose", Arg: "A"}))
	decIt := proto.Item{Type: "item", Nonce: testCanonicalID(76), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 453, AI: testAI1, Why: "decided", Msg: decID}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, decIt, decMsg, &chooseCause)
	if len(r.runtime.recorded()) != 3 {
		t.Fatalf("turns=%d, want 3", len(r.runtime.recorded()))
	}
}

func seedApproverDeviceRows(t *testing.T, r *deliverRig, approverKey identity.Key) {
	t.Helper()
	u := approverKey.ID()
	dev2 := testSeedKey(61)
	badUser := testSeedKey(62).ID()
	r.setSetting("approvers", []string{badUser})
	e1 := approverKey.Sign(encodeJSON(map[string]any{"t": "device", "n": testCanonicalID(61), "ts": testStartMS, "user": u, "ver": 1, "op": "add", "dev": u, "kind": "browser", "name": "a1"}))
	e2 := approverKey.Sign(encodeJSON(map[string]any{"t": "device", "n": testCanonicalID(62), "ts": testStartMS, "user": u, "ver": 2, "op": "add", "dev": dev2.ID(), "kind": "browser", "name": "a2"}))
	bad := approverKey.Sign(encodeJSON(map[string]any{"t": "device", "n": testCanonicalID(63), "ts": testStartMS, "user": badUser, "ver": 2, "op": "add", "dev": badUser, "kind": "browser", "name": "gap"}))
	r.h.Rows.Put(testGroup1, "device", u+":2", json.RawMessage(encodeJSON(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: u + ":2", Seq: 20}, DeviceChange: proto.DeviceChange{User: u, Version: 2, Op: "add", Device: dev2.ID()}, Env: e2})))
	r.h.Rows.Put(testGroup1, "device", u+":1", json.RawMessage(encodeJSON(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: u + ":1", Seq: 21}, DeviceChange: proto.DeviceChange{User: u, Version: 1, Op: "add", Device: u}, Env: e1})))
	r.h.Rows.Put(testGroup1, "device", badUser+":2", json.RawMessage(encodeJSON(proto.DeviceRow{RowHeader: proto.RowHeader{Type: "device", ID: badUser + ":2", Seq: 22}, DeviceChange: proto.DeviceChange{User: badUser, Version: 2, Op: "add", Device: badUser}, Env: bad})))
}

func checkInvalidApprovalCauses(t *testing.T, r *deliverRig) {
	t.Helper()
	base := proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(80), Timestamp: testStartMS, Group: testGroup1}, Msg: testCard1, Action: "approve"}
	badPayloads := []struct {
		signer identity.Key
		raw    string
	}{
		{r.ownerKey, `{bad-json`},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Type = "post"; return a }())},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Group = testGroup2; return a }())},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.As = testAI1; return a }())},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Msg = testCard2; return a }())},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Action = "choose"; return a }())},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Arg = "extra"; return a }())},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Text = "extra"; return a }())},
		{r.extraKey, encodeJSON(base)},
		{r.ownerKey, encodeJSON(func() proto.Act { a := base; a.Msg = testCanonicalID(89); return a }())},
	}
	for i, tc := range badPayloads {
		cause := tc.signer.Sign(tc.raw)
		msgID := proto.ID(testCard1)
		if i == len(badPayloads)-1 {
			msgID = testCanonicalID(89)
		}
		it := proto.Item{Type: "item", Nonce: testCanonicalID(byte(90 + i)), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: int64(400 + i), AI: testAI1, Why: "decided", Task: testTask1, Msg: msgID}
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil, &cause)
	}
	if len(r.runtime.recorded()) != 0 {
		t.Fatalf("invalid approval causes ran %d turns", len(r.runtime.recorded()))
	}
}

func insertPendingApproval(t *testing.T, db *store.Store, card, group, member, task string) {
	t.Helper()
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO approval(card, g, member, task, tool, input, state) VALUES(?, ?, ?, ?, 'bash', 'sha', 'pending')", card, group, member, task)
		return err
	}
	if err := db.Write(write); err != nil {
		t.Fatal(err)
	}
}

func approvalStateInDB(t *testing.T, db *store.Store, card string) string {
	t.Helper()
	var st string
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce((SELECT state FROM approval WHERE card = ?), '')", card).Scan(&st)
	}
	if err := db.Write(read); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestRoutingSecretaryThreadsAndTaskDecisions(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "review", Commit: "c1", Assignee: testAI1, Reviewer: testAI2})

	// Secretary confirmed item goes to secretary GroupSession.
	confCause := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(101), Timestamp: testStartMS, Group: testGroup1}, Msg: testMsg1, Action: "confirm"}))
	confIt := proto.Item{Type: "item", Nonce: testCanonicalID(102), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 501, AI: testSec, Why: "confirmed", Msg: testMsg1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, confIt, nil, &confCause)

	// Assign creates AI1 TaskSession; review creates AI2 ReviewSession.
	assignIt := proto.Item{Type: "item", Nonce: testCanonicalID(103), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 502, AI: testAI1, Why: "assign", Task: testTask1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, assignIt, nil, nil)
	revIt := proto.Item{Type: "item", Nonce: testCanonicalID(104), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 503, AI: testAI2, Why: "review", Task: testTask1, Commit: "c1"}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, revIt, nil, nil)

	// Thread mention via msg.Thread (when item.Task is empty) routes to AI2's existing ReviewSession; unknown task thread routes to GroupSession.
	tid := testTask1
	otherTid := testTask2
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(105), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 504, AI: testAI2, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, &tid, "in review thread"), nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(106), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 505, AI: testAI2, Why: "mention", Msg: testMsg2}, r.makeTextMsg(testMsg2, &otherTid, "other thread"), nil)

	// Decision on task assigned to AI1 goes to AI2's GroupSession when addressed to AI2, and fallback to Session when TaskRow absent.
	decID := testCanonicalID(107)
	decMsg := &proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: string(decID), Seq: 15}, Pos: 15, Group: testGroup1, By: testAI1, Kind: "decision", Timestamp: testStartMS, Mentions: []string{}, Body: json.RawMessage(`{}`)}
	choose := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(108), Timestamp: testStartMS, Group: testGroup1}, Msg: decID, Action: "choose", Arg: "x"}))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(109), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 506, AI: testAI2, Why: "decided", Task: testTask1, Msg: decID}, decMsg, &choose)
	r.h.Rows.Reset(testGroup1)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 16}, Name: "Proj", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, nil, 17)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(110), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 507, AI: testAI1, Why: "decided", Task: testTask1, Msg: decID}, decMsg, &choose)

	turns := r.runtime.recorded()
	wantKeys := []SessionKey{
		{Group: testGroup1, Member: testSec},
		{Group: testGroup1, Member: testAI1, Task: testTask1},
		{Group: testGroup1, Member: testAI2, Task: testTask1},
		{Group: testGroup1, Member: testAI2, Task: testTask1},
		{Group: testGroup1, Member: testAI2},
		{Group: testGroup1, Member: testAI2},
		{Group: testGroup1, Member: testAI1, Task: testTask1},
	}
	if len(turns) != len(wantKeys) {
		t.Fatalf("got %d turns, want %d", len(turns), len(wantKeys))
	}
	for i, w := range wantKeys {
		if turns[i].key != w {
			t.Fatalf("turn %d key=%+v, want %+v", i, turns[i].key, w)
		}
	}
}

func TestHoldsMaxOpenJumpAndReentrantDeliver(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "paused", Step: "branch", Assignee: testAI1})

	// Paused task holds assign for AI1, while notice for secretary on the same paused task still delivers immediately.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(120), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 601, AI: testAI1, Why: "assign", Task: testTask1}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(121), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 602, AI: testSec, Why: "notice", Task: testTask1, Code: "quiet"}, nil, nil)
	if len(r.runtime.recorded()) != 1 || r.runtime.recorded()[0].key.Member != testSec {
		t.Fatalf("expected only secretary notice turn, got %+v", r.runtime.recorded())
	}
	// Failed task also stays held; offline setting, group pause, Holds table, and Takeover each hold items.
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 11}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "failed", Step: "branch", Assignee: testAI1})
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("failed task item was delivered")
	}
	checkOfflinePauseHoldAndTakeover(t, r)
	checkMaxOpenAndJumpQueue(t)
}

func checkOfflinePauseHoldAndTakeover(t *testing.T, r *deliverRig) {
	t.Helper()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 12}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1})
	r.setSetting("offline", true)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("delivered while offline")
	}
	r.setSetting("offline", false)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 13}, Name: "Proj", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Secretary: testSec, Paused: &proto.GroupPause{By: testHuman, At: testStartMS}, Checks: []string{}})
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("delivered while group paused")
	}
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 14}, Name: "Proj", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	heldFlag := true
	Holds["cap"] = func(*Host, SessionKey) (string, bool) { return "cap", heldFlag }
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("delivered while Holds reported cap")
	}
	heldFlag = false
	gKey := SessionKey{Group: testGroup1, Member: testAI1}
	r.h.Sessions.Create(Session{SessionKey: gKey, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.h.Sessions.Move(gKey, Dormant, Takeover)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("delivered while in takeover")
	}
	r.h.Sessions.Move(gKey, Takeover, Dormant)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 2 {
		t.Fatalf("expected delivery after lifting holds, got %d", len(r.runtime.recorded()))
	}
}

func checkMaxOpenAndJumpQueue(t *testing.T) {
	t.Helper()
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	r.setSetting("max_open", 1)
	r.runtime.keepRun = true
	// AI1 starts Working (open=1 == max_open, waiting=0).
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(130), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 610, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "1"), nil)
	// AI2 is Dormant and blocked by max_open=1 while AI1 is Working.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(131), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 611, AI: testAI2, Why: "mention", Msg: testMsg2}, r.makeTextMsg(testMsg2, nil, "2"), nil)
	if len(r.runtime.recorded()) != 1 {
		t.Fatalf("AI2 woke despite max_open=1 and 0 waiting: %d", len(r.runtime.recorded()))
	}
	checkJumpOrderingAndInterrupt(t, r)
}

func checkJumpOrderingAndInterrupt(t *testing.T, r *deliverRig) {
	t.Helper()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 15}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1})
	// Queue non-jump items 612 (group) and 613 (task) for AI1 while AI1 GroupSession is Working, plus duplicate 612.
	it612 := proto.Item{Type: "item", Nonce: testCanonicalID(132), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 612, AI: testAI1, Why: "mention", Msg: testMsg1}
	it613 := proto.Item{Type: "item", Nonce: testCanonicalID(133), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 613, AI: testAI1, Why: "assign", Task: testTask1}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it612, r.makeTextMsg(testMsg1, nil, "m"), nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it612, r.makeTextMsg(testMsg1, nil, "m"), nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it613, nil, nil)
	// Jump item 614 targeting AI1's non-existent TaskSession (while GroupSession is Working and max_open=1, waiting=0) wakes via interrupting=true.
	tid := testTask1
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, Kind: TaskSession, Folder: "wt/1-t1", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	it614 := proto.Item{Type: "item", Nonce: testCanonicalID(134), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 614, AI: testAI1, Why: "mention", Task: testTask1, Msg: testMsg2, Jump: true}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it614, r.makeTextMsg(testMsg2, &tid, "jump-other-session"), nil)
	turns := r.runtime.recorded()
	if len(turns) != 2 || !turns[1].turn.Jump || turns[1].key.Task != testTask1 || turns[1].text != "task:1" {
		t.Fatalf("expected cross-session jump turn of 1 item, got %+v", turns)
	}
	finishJumpDrain(t, r, it612)
}

func finishJumpDrain(t *testing.T, r *deliverRig, it612 proto.Item) {
	t.Helper()
	// Also upgrade 612 on GroupSession to jump:true while Working, then finish Working and drain 613 + AI2's 611 via re-entrant Deliver.
	it612.Jump = true
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it612, r.makeTextMsg(testMsg1, nil, "m"), nil)
	r.runtime.keepRun = false
	called := false
	r.runtime.onTurn = func(h *Host, key SessionKey) {
		if !called {
			called = true
			Deliver(h, key.Group, key.Member)
		}
	}
	r.h.Sessions.Move(SessionKey{Group: testGroup1, Member: testAI1}, Working, Waiting)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 5 {
		t.Fatalf("expected 5 total turns after AI1 and AI2 drained, got %d", len(r.runtime.recorded()))
	}
}

func TestEnqueueHeldUnitOrderingEdges(t *testing.T) {
	mk := func(id int64, jump bool) heldItem {
		var j proto.True
		if jump {
			j = true
		}
		return heldItem{verifiedItem: verifiedItem{item: proto.Item{Item: id, Jump: j}}}
	}
	// Empty queue + jump -> [10(J)], second jump -> [10(J), 11(J)], duplicate jump 10 -> unchanged [10(J), 11(J)].
	var q []heldItem
	q = enqueueHeld(q, mk(10, true))
	q = enqueueHeld(q, mk(11, true))
	q = enqueueHeld(q, mk(10, true))
	// Add non-jump 12 and 13 -> [10(J), 11(J), 12, 13]; upgrade 13 to jump -> [10(J), 11(J), 13(J), 12].
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
	// Upgrade index 0 when queue starts with non-jump -> [20(J), 21].
	q2 := enqueueHeld(enqueueHeld(nil, mk(20, false)), mk(21, false))
	q2 = enqueueHeld(q2, mk(20, true))
	if q2[0].item.Item != 20 || !bool(q2[0].item.Jump) {
		t.Fatalf("index 0 upgrade failed: %+v", q2)
	}
}

func TestStillCurrentRefusalsAndMixedBatch(t *testing.T) {
	r := newDeliverRig(t, false)
	r.seedGroupAndMembers()
	// Hold items while offline, then update task state to test every R3-3 role/step/commit mismatch.
	r.setSetting("offline", true)
	insertPendingApproval(t, r.db, testCard1, testGroup1, testAI1, testTask1)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "T1", Slug: "t1", Origin: "brief", State: "active", Step: "review", Commit: "c1", Assignee: testAI1, Reviewer: testAI2})

	cause := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(140), Timestamp: testStartMS, Group: testGroup1}, Msg: testCard1, Action: "approve"}))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(141), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 701, AI: testAI1, Why: "decided", Task: testTask1, Msg: testCard1}, nil, &cause)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(142), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 702, AI: testAI1, Why: "resume", Task: testTask1}, nil, nil)
	// Delete pending approval while held: 701 becomes stale_approval while 702 in the same TaskSession batch stays valid.
	deleteTaskApprovals(r.db, testGroup1, testTask1)
	r.setSetting("offline", false)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 || r.runtime.recorded()[0].text != "task:1" {
		t.Fatalf("expected mixed batch to deliver 1 valid item, got %+v", r.runtime.recorded())
	}
	checkRoleStepCommitMismatches(t, r)
}

func checkRoleStepCommitMismatches(t *testing.T, r *deliverRig) {
	t.Helper()
	// Check matchesTaskRole directly across assignee, reviewer, step, commit, and absent task.
	activeTask := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 20}, Group: testGroup1, Num: 2, Title: "T2", Slug: "t2", Origin: "brief", State: "active", Step: "review", Commit: "c-good", Assignee: testAI1, Reviewer: testAI2}
	r.putTask(activeTask)
	staleItems := []proto.Item{
		{Why: "assign", AI: testAI1, Task: testCanonicalID(95)},
		{Why: "assign", AI: testAI2, Task: testTask2},
		{Why: "review", AI: testAI1, Task: testTask2, Commit: "c-good"},
		{Why: "review", AI: testAI2, Task: testTask2, Commit: "c-old"},
		{Why: "merge", AI: testAI1, Task: testTask2, Commit: "c-good"},
	}
	for i, it := range staleItems {
		it.Type, it.Nonce, it.Timestamp, it.Group, it.To, it.Item = "item", testCanonicalID(byte(150+i)), testStartMS, testGroup1, r.hostKey.ID(), int64(710+i)
		r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, it, nil, nil)
	}
	// Wrong step for review, and wrong assignee/commit for merge.
	activeTask.Step = "merge"
	r.putTask(activeTask)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(160), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 720, AI: testAI2, Why: "review", Task: testTask2, Commit: "c-good"}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(161), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 721, AI: testAI2, Why: "merge", Task: testTask2, Commit: "c-good"}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(162), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 722, AI: testAI1, Why: "merge", Task: testTask2, Commit: "c-wrong"}, nil, nil)
	if len(r.runtime.recorded()) != 1 {
		t.Fatalf("stale items ran turns: %d", len(r.runtime.recorded()))
	}
}

func initTestOriginAndGroupRepo(t *testing.T, r *deliverRig, forge string) (originDir, projectDir string, baseCommit string) {
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
	r.putGroup(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1},
		Name:      "Website", Kind: "project", Repo: originDir, Base: "main", Forge: forge,
		Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{},
	})
	r.putAIMember(testAI1, "Claude", r.hostKey.ID(), false, nil, 2)
	r.putAIMember(testAI2, "Codex", r.hostKey.ID(), false, nil, 3)
	r.putAIMember(testSec, "Sec", r.hostKey.ID(), true, nil, 4)
	r.h.Attach(r.homeKey.ID(), r.home)
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

func TestWorkspacesTaskBranchReviewRoundsAndLocalMergeLifecycle(t *testing.T) {
	r := newDeliverRig(t, true)
	_, pDir, _ := initTestOriginAndGroupRepo(t, r, "none")
	ctx := t.Context()

	// 1) Assign at step 'branch' clones repo, creates wt/1-add-feature, and sends TaskBranch.
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(170), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 801, AI: testAI1, Why: "assign", Task: testTask1}, nil, nil)
	cmds := r.home.cmdPayloads()
	if len(cmds) != 1 || !strings.Contains(cmds[0], `"op":"branch"`) || !strings.Contains(cmds[0], `"branch":"task/1-add-feature"`) || strings.Contains(cmds[0], `"as":`) {
		t.Fatalf("unexpected task branch cmd: %v", cmds)
	}
	// Second item on existing TaskSession at step local_check does not send a second TaskBranch.
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 6}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(171), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 802, AI: testAI1, Why: "resume", Task: testTask1}, nil, nil)
	if len(r.home.cmdPayloads()) != 1 {
		t.Fatal("sent duplicate TaskBranch")
	}

	// Commit 2 commits in wt/1-add-feature and run two review rounds + thread mention.
	wt := filepath.Join(pDir, "wt", "1-add-feature")
	mustGit(t, ctx, wt, "config", "user.name", "AI")
	mustGit(t, ctx, wt, "config", "user.email", "ai@example.invalid")
	os.WriteFile(filepath.Join(wt, "README.md"), []byte("hello\nchanged\nadded\n"), 0o600)
	mustGit(t, ctx, wt, "add", "README.md")
	mustGit(t, ctx, wt, "commit", "-m", "c1")
	c1 := strings.TrimSpace(mustGit(t, ctx, wt, "rev-parse", "HEAD"))
	runReviewAndLocalMerge(t, r, pDir, wt, c1)
}

func runReviewAndLocalMerge(t *testing.T, r *deliverRig, pDir, wt, c1 string) {
	t.Helper()
	c2 := verifyReviewRounds(t, r, pDir, wt, c1)
	verifyLocalMergeAndCleanup(t, r, pDir, wt, c2)
}

func verifyReviewRounds(t *testing.T, r *deliverRig, pDir, wt, c1 string) string {
	t.Helper()
	ctx := t.Context()
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 7}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "review", Commit: c1, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(172), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 803, AI: testAI2, Why: "review", Task: testTask1, Commit: c1}, nil, nil)
	if _, err := os.Stat(filepath.Join(pDir, "review", "1", reviewDiff)); err != nil {
		t.Fatalf("review diff missing: %v", err)
	}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(173), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 804, AI: testAI2, Why: "mention", Task: testTask1, Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "note"), nil)
	os.WriteFile(filepath.Join(wt, "extra.txt"), []byte("more\n"), 0o600)
	mustGit(t, ctx, wt, "add", "extra.txt")
	mustGit(t, ctx, wt, "commit", "-m", "c2")
	c2 := strings.TrimSpace(mustGit(t, ctx, wt, "rev-parse", "HEAD"))
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 8}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "review", Commit: c2, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(174), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 805, AI: testAI2, Why: "review", Task: testTask1, Commit: c2}, nil, nil)
	if revHead := strings.TrimSpace(mustGit(t, ctx, filepath.Join(pDir, "review", "1"), "rev-parse", "HEAD")); revHead != c2 {
		t.Fatalf("review round 2 head=%s, want %s", revHead, c2)
	}
	return c2
}

func verifyLocalMergeAndCleanup(t *testing.T, r *deliverRig, pDir, wt, c2 string) {
	t.Helper()
	holdReason := "offline"
	Holds["off"] = func(*Host, SessionKey) (string, bool) { return holdReason, true }
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 9}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "active", Step: "merge", Commit: c2, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(175), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 806, AI: testAI1, Why: "merge", Task: testTask1, Commit: c2}, nil, nil)
	if len(r.home.cmdPayloads()) != 1 {
		t.Fatal("merge ran while host offline")
	}
	holdReason = "cap"
	Deliver(r.h, "", "")
	assertMergedStatsCmd(t, r.home.cmdPayloads())
	assertPostMergeCleanup(t, r, pDir, wt, c2)
}

func assertMergedStatsCmd(t *testing.T, cmds []string) {
	t.Helper()
	ok := len(cmds) == 2 && strings.Contains(cmds[1], `"op":"merged"`) && strings.Contains(cmds[1], `"added":3`)
	if !ok || !strings.Contains(cmds[1], `"removed":1`) {
		t.Fatalf("expected TaskMerged with stats +3/-1, got %v", cmds)
	}
}

func assertPostMergeCleanup(t *testing.T, r *deliverRig, pDir, wt, c2 string) {
	t.Helper()
	mergedRow := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 10}, Group: testGroup1, Num: 1, Title: "Add feature", Slug: "add-feature", Origin: "brief", State: "merged", Step: "merge", Commit: c2, Assignee: testAI1, Reviewer: testAI2, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{mergedRow}})))
	if len(r.h.Sessions.All()) != 0 {
		t.Fatalf("sessions remaining after merge: %+v", r.h.Sessions.All())
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("task worktree still exists after merge")
	}
	if branches := strings.TrimSpace(mustGit(t, t.Context(), filepath.Join(pDir, "repo"), "branch", "--list", "task/1-add-feature")); branches != "" {
		t.Fatalf("local task branch not deleted: %q", branches)
	}
}

func TestLocalMergeConflictAndRestartInspect(t *testing.T) {
	r := newDeliverRig(t, true)
	_, pDir, baseCommit := initTestOriginAndGroupRepo(t, r, "none")
	EnsureGroupWorkspace(t.Context(), pDir, "", "main")
	// Task 1 at merge with commit differing from branch head produces TaskConflict.
	CreateTaskWorkspace(t.Context(), pDir, 1, "conflict", testAI1, "main", false)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "C", Slug: "conflict", Origin: "brief", State: "active", Step: "merge", Commit: "0000000000000000000000000000000000000000", Branch: "task/1-conflict", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(180), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 820, AI: testAI1, Why: "merge", Task: testTask1, Commit: "0000000000000000000000000000000000000000"}, nil, nil)
	if cmds := r.home.cmdPayloads(); len(cmds) != 1 || !strings.Contains(cmds[0], `"op":"conflict"`) {
		t.Fatalf("expected TaskConflict, got %v", cmds)
	}
	// 2) Simulate restart after AuditMerge was written before TaskConflict/TaskMerged could be reported (home unreachable first, then connected).
	write := func(tx *store.Tx) error {
		_, err := r.h.Audit(tx, AuditRow{Group: testGroup1, Agent: "claude", What: AuditMerge, Detail: testTask2 + " " + baseCommit})
		return err
	}
	r.db.Write(write)
	r2 := newDeliverRigOnStore(t, r.db)
	r2.home.cmdErr = errors.New("disconnected")
	r2.h.Attach(r2.homeKey.ID(), r2.home)
	grpRaw := json.RawMessage(encodeJSON(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 11}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r2.homeKey.ID(), Secretary: testSec, Checks: []string{}}))
	aiRaw := json.RawMessage(encodeJSON(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 12}, Group: testGroup1, Kind: "ai", User: r2.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r2.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	t2Raw := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 13}, Group: testGroup1, Num: 2, Title: "Restart", Slug: "restart", Origin: "brief", State: "active", Step: "merge", Commit: baseCommit, Assignee: testAI1, Depends: []string{}}))
	r2.h.Push(r2.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpRaw, aiRaw, t2Raw}})))
	// First attempt failed to send command, so reported is false; reconnecting and pushing rows reports once and marks reported.
	r2.home.cmdErr = nil
	r2.h.Push(r2.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{}})))
	r2.h.Push(r2.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{}})))
	if len(r2.home.cmdPayloads()) != 2 {
		t.Fatalf("expected 1 failed + 1 succeeded inspect command, got %d", len(r2.home.cmdPayloads()))
	}
}

func TestRemoteForgeMergeGroupRefreshAndStep17Reconcile(t *testing.T) {
	// Verify githubRepo extraction across URL styles.
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
	originDir, pDir, baseCommit := initTestOriginAndGroupRepo(t, r, "github")
	// GroupSession wake with forge=github runs RefreshGroupWorkspace (fetch + ff-only); second turn while Waiting skips refresh.
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(190), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 850, AI: testSec, Why: "notice", Code: "ping"}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(191), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 851, AI: testSec, Why: "notice", Code: "pong"}, nil, nil)
	checkRemoteMergeWithFakeGH(t, r, baseCommit)
	checkReconcileCancelReassignAndGone(t, r, originDir, pDir, baseCommit)
}

func checkRemoteMergeWithFakeGH(t *testing.T, r *deliverRig, baseCommit string) {
	t.Helper()
	mergedJSON := `{"state":"MERGED","headRefOid":"deadbeef","mergeCommit":{"oid":"merged-oid"}}` + "\n"
	fakeForgeGH(t,
		forgeAnswer{},
		forgeAnswer{Stdout: mergedJSON},
		forgeAnswer{Stderr: "down", Code: 1},
		forgeAnswer{Stderr: "down", Code: 1},
	)
	r.putTask(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 20},
		Group:     testGroup1, Num: 1, Title: "Remote", Slug: "remote", Origin: "brief",
		State: "active", Step: "merge", Commit: "deadbeef", PR: 42, Assignee: testAI1,
		Stats: proto.TaskStats{Added: 9, Removed: 4},
	})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(192), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 852, AI: testAI1, Why: "merge", Task: testTask1, Commit: "deadbeef", PR: 42}, nil, nil)
	assertRemoteMergedAndFallback(t, r, baseCommit)
}

func assertRemoteMergedAndFallback(t *testing.T, r *deliverRig, baseCommit string) {
	t.Helper()
	cmds := r.home.cmdPayloads()
	if len(cmds) != 1 || !strings.Contains(cmds[0], `"merge_commit":"merged-oid"`) {
		t.Fatalf("unexpected remote TaskMerged: %v", cmds)
	}
	stats := computeMergeStats(t.Context(), filepath.Join(t.TempDir(), "missing"), proto.GroupRow{Base: "main", Forge: "github"}, baseCommit, proto.TaskStats{Added: 2, Removed: 1})
	if stats.Added != 2 || stats.Removed != 1 {
		t.Fatalf("fallback stats=%+v", stats)
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 21}, Group: testGroup1, Num: 2, Title: "FailGH", Slug: "failgh", Origin: "brief", State: "active", Step: "merge", Commit: "fail-gh", PR: 99, Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(193), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 853, AI: testAI1, Why: "merge", Task: testTask2, Commit: "fail-gh", PR: 99}, nil, nil)
	if len(r.home.cmdPayloads()) != 1 {
		t.Fatal("failed gh inspection should not report conflict or merged")
	}
}

func checkReconcileCancelReassignAndGone(t *testing.T, r *deliverRig, originDir, pDir, baseCommit string) {
	t.Helper()
	ctx := t.Context()
	// Create Task 3 for AI1 + Review for AI2, then reassign assignee to AI2 (pushes branch to origin) and change reviewer to Sec.
	t3 := testCanonicalID(200)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(t3), Seq: 30}, Group: testGroup1, Num: 3, Title: "Give", Slug: "give", Origin: "brief", State: "active", Step: "review", Commit: baseCommit, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(201), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 860, AI: testAI1, Why: "assign", Task: t3}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(202), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 861, AI: testAI2, Why: "review", Task: t3, Commit: baseCommit}, nil, nil)
	insertPendingApproval(t, r.db, testCard1, testGroup1, testAI1, string(t3))

	reassigned := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(t3), Seq: 31}, Group: testGroup1, Num: 3, Title: "Give", Slug: "give", Origin: "brief", State: "active", Step: "local_check", Commit: baseCommit, Assignee: testAI2, Reviewer: testSec, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{reassigned}})))
	if refs := strings.TrimSpace(mustGit(t, ctx, originDir, "for-each-ref", "--format=%(refname)", "refs/heads/task/3-give")); refs != "refs/heads/task/3-give" {
		t.Fatalf("reassignment did not push branch to origin: %q", refs)
	}
	if approvalStateInDB(t, r.db, testCard1) != "" {
		t.Fatal("task approval survived reassignment")
	}
	checkCancelAndGoneCleanup(t, r, baseCommit)
}

func checkCancelAndGoneCleanup(t *testing.T, r *deliverRig, baseCommit string) {
	t.Helper()
	// Task 4 cancelled ends both TaskSession and ReviewSession; Task 5 GoneRow ends both; Member GoneRow ends GroupSession & deletes only that member's approvals.
	t4, t5 := testCanonicalID(204), testCanonicalID(205)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(t4), Seq: 32}, Group: testGroup1, Num: 4, Title: "Cancel", Slug: "cancel", Origin: "brief", State: "active", Step: "review", Commit: baseCommit, Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(206), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 862, AI: testAI1, Why: "assign", Task: t4}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(207), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 863, AI: testAI2, Why: "review", Task: t4, Commit: baseCommit}, nil, nil)
	cancelled := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(t4), Seq: 33}, Group: testGroup1, Num: 4, Title: "Cancel", Slug: "cancel", Origin: "brief", State: "cancelled", Step: "review", Commit: baseCommit, Assignee: testAI1, Reviewer: testAI2, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{cancelled}})))

	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: string(t5)}, Kind: TaskSession, Folder: "wt/5-gone", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI2, Task: string(t5)}, Kind: ReviewSession, Folder: "review/5", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	// Also test reconcile when task row is absent and invalid Num == 0 cleanup guard.
	tMissing, tZero := testCanonicalID(208), testCanonicalID(209)
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: string(tMissing)}, Kind: TaskSession, Folder: "wt/m", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI2, Task: string(tMissing)}, Kind: ReviewSession, Folder: "review/m", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: string(tZero)}, Kind: TaskSession, Folder: "wt/0", Agent: "claude", Home: r.homeKey.ID(), Name: "C"})
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(tZero), Seq: 34}, Group: testGroup1, Num: 0, Title: "Z", Slug: "z", Origin: "brief", State: "cancelled", Step: "branch", Assignee: testAI1})

	insertPendingApproval(t, r.db, testCard1, testGroup1, testSec, "")
	insertPendingApproval(t, r.db, testCard2, testGroup1, testAI1, "")
	goneRows := []json.RawMessage{
		json.RawMessage(encodeJSON(proto.GoneRow{RowHeader: proto.RowHeader{Type: "task", ID: string(t5), Seq: 35}, Gone: true})),
		json.RawMessage(encodeJSON(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: testSec, Seq: 36}, Gone: true})),
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: goneRows})))
	if _, exists := r.h.Sessions.Get(SessionKey{Group: testGroup1, Member: testSec}); exists {
		t.Fatal("secretary GroupSession survived member GoneRow")
	}
	if approvalStateInDB(t, r.db, testCard1) != "" || approvalStateInDB(t, r.db, testCard2) != "pending" {
		t.Fatal("member GoneRow deleted wrong approvals")
	}
}

func TestErrorPathsStoreTriggersWorkspaceAndComposeFailures(t *testing.T) {
	r := newDeliverRig(t, true)
	r.seedGroupAndMembers()
	cfg, _ := loadHostConfig(r.h)
	pDir := cfg.projectDir(testGroup1)

	// 1) Compose failure re-queues item without auditing or acking.
	r.composeE = errors.New("fetch failed")
	msg := r.makeTextMsg(testMsg1, nil, "retry me")
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(210), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 901, AI: testAI1, Why: "mention", Msg: testMsg1}, msg, nil)
	if len(r.home.ackList()) != 0 || len(r.runtime.recorded()) != 0 {
		t.Fatal("compose error still acked or ran turn")
	}
	// 2) SQLite trigger blocking audit INSERT causes commitTurnItems, recordMergeTaken, and finishHandedItem to abort cleanly.
	r.composeE = nil
	addAuditBlockTrigger(t, r.db)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.home.ackList()) != 0 || len(r.runtime.recorded()) != 0 {
		t.Fatal("audit write failure still acked or ran turn")
	}
	checkMergeAndHandAuditFailure(t, r)
	dropAuditBlockTrigger(t, r.db)
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != 1 {
		t.Fatal("re-queued item did not run once audit trigger was removed")
	}
	checkWorkspaceAndStoreDropErrors(t, r, pDir)
}

func checkMergeAndHandAuditFailure(t *testing.T, r *deliverRig) {
	t.Helper()
	// Handed reply when audit fails does not ack; merge when recordMergeTaken fails re-queues merge; claimMerge / claimUnreportedMerge false branches.
	ch, done := r.h.Await(testCall1, nil)
	defer done()
	callRef := testCall1
	replyMsg := &proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: testMsg2, Seq: 40}, Pos: 40, Group: testGroup1, By: testSec, Kind: "reply", Timestamp: testStartMS, Mentions: []string{}, Re: &callRef, Body: json.RawMessage(`{}`)}
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(211), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 902, AI: testAI1, Why: "reply", Msg: testMsg2}, replyMsg, nil)
	if len(ch) != 1 || len(r.home.ackList()) != 0 {
		t.Fatal("expected handed item without ack on audit failure")
	}
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 41}, Group: testGroup1, Num: 0, Title: "M", Slug: "m", Origin: "brief", State: "active", Step: "merge", Commit: "c1", Assignee: testAI1})
	mIt := proto.Item{Type: "item", Nonce: testCanonicalID(212), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 903, AI: testAI1, Why: "merge", Task: testTask1, Commit: "c1"}
	// Duplicate enqueueMerge + claimMerge already claimed + recordMergeTaken failure.
	d := deliv(r.h)
	v := verifiedItem{home: r.homeKey.ID(), group: testGroup1, wire: proto.InboxItem{Env: r.homeKey.Sign(encodeJSON(mIt))}, item: mIt}
	d.enqueueMerge(v)
	d.enqueueMerge(v)
	mKey := mergeTrackKey(testGroup1, testTask1, "c1")
	d.claimMerge(mKey)
	if d.claimMerge(mKey) || d.claimUnreportedMerge(mKey) {
		t.Fatal("expected claim to fail while merging")
	}
	cfg, _ := loadHostConfig(r.h)
	stepMerges(r.h, d, cfg, testGroup1)
	d.releaseMerge(mKey)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, mIt, nil, nil)
}

func checkWorkspaceAndStoreDropErrors(t *testing.T, r *deliverRig, pDir string) {
	t.Helper()
	// Invalid Task.Num == 0 fails prepareTaskFolder; non-git repo folder fails EnsureGroupWorkspace; diverged repo fails RefreshGroupWorkspace; bad commit fails PrepareReviewWorkspace; pre-existing row in session table fails createSessionRow; approval update failure rolls back commitTurnItems.
	os.MkdirAll(filepath.Join(pDir, "repo"), 0o700)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 50}, Name: "W", Kind: "project", Repo: "/nonexistent/repo", Base: "main", Forge: "github", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(215), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 910, AI: testSec, Why: "notice", Code: "c"}, nil, nil)
	// Make repo/.git a directory without valid origin so RefreshGroupWorkspace, CreateTaskWorkspace, PrepareReviewWorkspace fail cleanly.
	os.MkdirAll(filepath.Join(pDir, "repo", ".git"), 0o700)
	r.putGroup(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 51}, Name: "W", Kind: "project", Base: "main", Forge: "github", Home: r.homeKey.ID(), Secretary: testSec, Checks: []string{}})
	Deliver(r.h, testGroup1, testSec)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 52}, Group: testGroup1, Num: 2, Title: "Bad", Slug: "bad", Origin: "brief", State: "active", Step: "review", Commit: "bad-oid", Assignee: testAI1, Reviewer: testAI2})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(216), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 911, AI: testAI1, Why: "assign", Task: testTask2}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(217), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 912, AI: testAI2, Why: "review", Task: testTask2, Commit: "bad-oid"}, nil, nil)
	os.RemoveAll(filepath.Join(pDir, "repo"))

	// Invalid Num == 0 in prepareTaskFolder, and session INSERT trigger failing createSessionRow.
	tBadNum := testCanonicalID(218)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(tBadNum), Seq: 53}, Group: testGroup1, Num: 0, Title: "Zero", Slug: "zero", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(219), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 913, AI: testAI1, Why: "assign", Task: tBadNum}, nil, nil)
	addSessionAndApprovalTriggers(t, r)
	checkDroppedTablesErrors(t, r)
}

func addSessionAndApprovalTriggers(t *testing.T, r *deliverRig) {
	t.Helper()
	write := func(tx *store.Tx) error {
		_, err := tx.Exec(`CREATE TRIGGER block_session BEFORE INSERT ON session BEGIN SELECT RAISE(ABORT, 'blocked'); END;
			CREATE TRIGGER block_approval BEFORE UPDATE ON approval BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
		return err
	}
	r.db.Write(write)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(221), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 949, AI: testAI2, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "hi"), nil)
	insertPendingApproval(t, r.db, testCard1, testGroup1, testAI1, "")
	v := verifiedItem{home: r.homeKey.ID(), group: testGroup1, item: proto.Item{Item: 950, AI: testAI1, Why: "decided", Msg: testCard1}, approval: "approve"}
	if err := commitTurnItems(r.h, "claude", []heldItem{{verifiedItem: v}}); err == nil {
		t.Fatal("expected commitTurnItems to fail when approval update fails")
	}
}

func checkDroppedTablesErrors(t *testing.T, r *deliverRig) {
	t.Helper()
	// Drop audit first (while setting still exists) to hit hasAuditItem error in handleInboxElem, then drop setting to hit loadHostConfig errors.
	dropAudit := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE audit")
		return err
	}
	r.db.Write(dropAudit)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(220), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 960, AI: testAI1, Why: "notice", Code: "c"}, nil, nil)

	dropSetting := func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE approval; DROP TABLE setting")
		return err
	}
	r.db.Write(dropSetting)
	if hasNoPendingApproval(r.db, testCard1, testGroup1, testAI1) {
		t.Fatal("expected fail-closed false when approval table is unreadable")
	}
	r.h.Attach(r.homeKey.ID(), r.home)
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Reset{Type: "reset", Group: testGroup1})))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Inbox{Type: "inbox", Group: testGroup1, Items: []proto.InboxItem{}})))
	afterGroupState(r.h, deliv(r.h), testGroup1)
	stepAI(r.h, deliv(r.h), aiGroupKey{group: testGroup1, member: testAI1})
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

func TestColdStartEarlyBufferAndRemainingBranches(t *testing.T) {
	// Submatch with no deletions hits len(m) < 2 -> 0; memory host runReviewedMerge & unknown home resumeGroupMerges return cleanly.
	if got := submatchInt(shortstatRemoved, " 1 file changed, 4 insertions(+)"); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
	mem := newDeliverRig(t, false)
	memCfg, _ := loadHostConfig(mem.h)
	runReviewedMerge(mem.h, deliv(mem.h), memCfg, mem.homeKey.ID(), testGroup1, proto.TaskRow{RowHeader: proto.RowHeader{ID: testTask1}, Num: 1, Slug: "m"}, "c1", 0, false, time.Time{})
	resumeGroupMerges(mem.h, deliv(mem.h), memCfg, "no-home-group")

	// Cold start: attach with syncMode=true, push decided (for open wait) + mention while syncing, then finish sync.
	r := newDeliverRig(t, false)
	r.setSetting("homes", []proto.Home{{Group: testGroup1, Home: r.homeKey.ID(), Name: "W"}})
	r.home.syncMode = true
	r.h.Attach(r.homeKey.ID(), r.home)
	<-r.home.started

	waitCh, doneWait := r.h.Await(testCard1, nil)
	defer doneWait()
	cause := r.ownerKey.Sign(encodeJSON(proto.Act{HomeCommand: proto.HomeCommand{Type: "act", Nonce: testCanonicalID(230), Timestamp: testStartMS, Group: testGroup1}, Msg: testCard1, Action: "approve"}))
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(231), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 970, AI: testAI1, Why: "decided", Msg: testCard1}, nil, &cause)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(232), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 971, AI: testAI1, Why: "mention", Msg: testMsg1}, r.makeTextMsg(testMsg1, nil, "buffered"), nil)
	if len(r.runtime.recorded()) != 0 || len(waitCh) != 0 {
		t.Fatal("early items processed before sync finished")
	}
	finishColdSyncAndCheckEdges(t, r, waitCh)
}

func finishColdSyncAndCheckEdges(t *testing.T, r *deliverRig, waitCh <-chan json.RawMessage) {
	t.Helper()
	// Push paused group row + AI1 + active task + remote-AI merge task during sync, then answer synced.
	grpPaused := json.RawMessage(encodeJSON(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 1}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Paused: &proto.GroupPause{By: testHuman, At: testStartMS}, Checks: []string{}}))
	aiRow := json.RawMessage(encodeJSON(proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 2}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C"}, CLI: "claude", Host: r.hostKey.ID(), HostName: "n", Account: "personal", Allow: []string{}, Approvers: []string{}, State: "dormant", Path: "local"}))
	remoteMerge := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 3}, Group: testGroup1, Num: 2, Title: "R", Slug: "r", Origin: "brief", State: "active", Step: "merge", Commit: "c", Assignee: testAI2, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpPaused, aiRow, remoteMerge}})))
	r.home.answers <- syncAnswer{raw: json.RawMessage(encodeJSON(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 3}))}
	waitForSyncSettled(t, r.h, testGroup1)
	if len(waitCh) != 1 {
		t.Fatal("early open-wait item not handed after sync")
	}
	// Group was paused, so 971 is now in d.held[testGroup1, testAI1]; unpausing via pushRows wakes it via groupHeldKeys.
	grpUnpaused := json.RawMessage(encodeJSON(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: testGroup1, Seq: 4}, Name: "W", Kind: "project", Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{grpUnpaused}})))
	if len(r.runtime.recorded()) != 1 {
		t.Fatalf("expected 1 turn after group unpause, got %d", len(r.runtime.recorded()))
	}
	checkAllStaleBatchAndActiveReconcile(t, r)
}

func checkAllStaleBatchAndActiveReconcile(t *testing.T, r *deliverRig) {
	t.Helper()
	// Hold task assign while offline, then mark task merged and bring online -> filterStillCurrent yields len(current)==0; active task reconcile keeps TaskSession.
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 5}, Group: testGroup1, Num: 1, Title: "A", Slug: "a", Origin: "brief", State: "active", Step: "branch", Assignee: testAI1})
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(233), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 972, AI: testAI1, Why: "assign", Task: testTask1}, nil, nil)
	stillActive := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 6}, Group: testGroup1, Num: 1, Title: "A", Slug: "a", Origin: "brief", State: "active", Step: "local_check", Assignee: testAI1, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{stillActive}})))
	if _, ok := r.h.Sessions.Get(SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}); !ok {
		t.Fatal("active task session ended prematurely")
	}
	r.setSetting("offline", true)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(234), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 973, AI: testAI1, Why: "resume", Task: testTask1}, nil, nil)
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(235), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 974, AI: testAI1, Why: "merge", Task: testTask1, Commit: "c"}, nil, nil)
	r.putTask(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 7}, Group: testGroup1, Num: 1, Title: "A", Slug: "a", Origin: "brief", State: "merged", Step: "merge", Assignee: testAI1})
	r.setSetting("offline", false)
	turnsBefore := len(r.runtime.recorded())
	Deliver(r.h, testGroup1, testAI1)
	if len(r.runtime.recorded()) != turnsBefore {
		t.Fatal("stale batch ran a turn")
	}
	// Duplicate already-audited item 972 is acked and skipped; pushRows at step merge without AuditMerge and merged on memory store.
	acksBefore := len(r.home.ackList())
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Type: "item", Nonce: testCanonicalID(236), Timestamp: testStartMS, Group: testGroup1, To: r.hostKey.ID(), Item: 972, AI: testAI1, Why: "assign", Task: testTask1}, nil, nil)
	if len(r.home.ackList()) != acksBefore+1 || len(r.runtime.recorded()) != turnsBefore {
		t.Fatal("already-audited item was not deduplicated with ack")
	}
	cmdsBefore := len(r.home.cmdPayloads())
	noAuditMerge := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask2, Seq: 8}, Group: testGroup1, Num: 2, Title: "R", Slug: "r", Origin: "brief", State: "active", Step: "merge", Commit: "c", Assignee: testAI1, Depends: []string{}}))
	mergedTask1 := json.RawMessage(encodeJSON(proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: 9}, Group: testGroup1, Num: 1, Title: "A", Slug: "a", Origin: "brief", State: "merged", Step: "merge", Assignee: testAI1, Depends: []string{}}))
	r.h.Push(r.homeKey.ID(), json.RawMessage(encodeJSON(proto.Rows{Type: "rows", Group: testGroup1, Rows: []json.RawMessage{noAuditMerge, mergedTask1}})))
	if len(r.home.cmdPayloads()) != cmdsBefore {
		t.Fatal("task without AuditMerge triggered merge inspect")
	}
}
