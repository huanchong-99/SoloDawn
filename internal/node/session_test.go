package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type fakeCtl struct {
	in        chan json.RawMessage // single reader: Serve via Read
	out       chan json.RawMessage // single reader: test caller via next
	done      chan struct{}
	pathCh    chan struct{}
	once      sync.Once
	mu        sync.Mutex
	failAfter int
	sends     int
	beforeOut func(frame any)
}

func newFakeCtl() *fakeCtl {
	return &fakeCtl{
		in:     make(chan json.RawMessage, 32),
		out:    make(chan json.RawMessage, 64),
		done:   make(chan struct{}),
		pathCh: make(chan struct{}, 1),
	}
}

func (f *fakeCtl) Read(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-f.done:
		return nil, io.EOF
	case msg := <-f.in:
		return msg, nil
	}
}

func (f *fakeCtl) Send(frame any) error {
	f.mu.Lock()
	hook := f.beforeOut
	f.sends++
	fail := f.failAfter > 0 && f.sends >= f.failAfter
	f.mu.Unlock()
	if hook != nil {
		hook(frame)
	}
	if fail {
		return errors.New("send failed")
	}
	raw, err := dvlEncode(frame)
	if err != nil {
		return err
	}
	f.out <- raw
	return nil
}

func (f *fakeCtl) Path() string                 { return "direct" }
func (f *fakeCtl) PathChanged() <-chan struct{} { return f.pathCh }
func (f *fakeCtl) Done() <-chan struct{}        { return f.done }
func (f *fakeCtl) Close() error                 { f.once.Do(func() { close(f.done) }); return nil }

func (f *fakeCtl) pushRaw(s string) { f.in <- json.RawMessage(s) }

func (f *fakeCtl) push(v any) {
	raw, err := dvlEncode(v)
	if err != nil {
		panic(err)
	}
	f.in <- raw
}

func (f *fakeCtl) next(t *testing.T) json.RawMessage {
	t.Helper()
	select {
	case msg := <-f.out:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for frame")
		return nil
	}
}

func (f *fakeCtl) expectNone(t *testing.T) {
	t.Helper()
	select {
	case msg := <-f.out:
		t.Fatalf("unexpected frame: %s", msg)
	case <-time.After(30 * time.Millisecond):
	}
}

func expectErr(t *testing.T, raw json.RawMessage, wantRe int64, wantCode string) {
	t.Helper()
	var er proto.ErrorResponse
	if err := proto.Decode(raw, &er); err != nil {
		t.Fatalf("decode error response %s: %v", raw, err)
	}
	if er.Re != wantRe || er.Err.Code != wantCode {
		t.Fatalf("got re=%d code=%s (%s), want re=%d code=%s", er.Re, er.Err.Code, er.Err.Message, wantRe, wantCode)
	}
}

func seedOwnedLogs(t *testing.T, db *store.Store, nodeKey, browserKey identity.Key) *Logs {
	t.Helper()
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "phone")
	e2 := sesAddEntry(browserKey, browserKey.ID(), 2, nodeKey.ID(), "node", "workstation")
	cand, err := identity.Replay(browserKey.ID(), []proto.Envelope{e1, e2})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := logs.dvlAdopt(browserKey.ID(), cand, nil); err != nil || !ok {
		t.Fatalf("seed adopt: %v, %v", ok, err)
	}
	return logs
}

func newTestHomeAndHost(t *testing.T, db *store.Store, nodeKey identity.Key, logs *Logs, now time.Time, cmds home.Table) (*home.Loop, *host.Host) {
	t.Helper()
	if cmds == nil {
		cmds = home.Table{}
	}
	tables := home.Tables{Commands: cmds, Timers: home.Rules[home.Timer]{}, Events: home.Events, After: home.Rules[home.Hook]{}}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: nodeKey.ID(), Owner: logs.Owner(),
		Logs: logs.Log, Tables: tables, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(loop.Close)
	h, _, err := host.New(db, nodeKey, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return loop, h
}

func serveConnCtx(t *testing.T, ctx context.Context, s *Sessions, device string, paired bool) *fakeCtl {
	t.Helper()
	fc := newFakeCtl()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Serve(ctx, device, paired, fc)
	}()
	t.Cleanup(func() {
		fc.Close()
		wg.Wait()
	})
	return fc
}

func serveConn(t *testing.T, s *Sessions, device string, paired bool) *fakeCtl {
	t.Helper()
	return serveConnCtx(t, t.Context(), s, device, paired)
}

func TestSessionDefaultsAndWelcomeOrdering(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, b2Key := sesKey("node"), sesKey("browser"), sesKey("b2")
	logs := seedOwnedLogs(t, db, nodeKey, browserKey)
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)

	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h})
	s.cfg.Report(errors.New("logged default error"))

	fc := newFakeCtl()
	e3 := sesAddEntry(browserKey, browserKey.ID(), 3, b2Key.ID(), "browser", "b2")
	var injected sync.Once
	fc.beforeOut = func(frame any) {
		if _, isWelcome := frame.(proto.Welcome); isWelcome {
			injected.Do(func() {
				if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3}}, nil); err != nil {
					t.Error(err)
				}
			})
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Serve(t.Context(), browserKey.ID(), false, fc)
	}()
	t.Cleanup(func() {
		fc.Close()
		wg.Wait()
	})

	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
	var w proto.Welcome
	if err := proto.Decode(fc.next(t), &w); err != nil {
		t.Fatalf("first frame was not welcome: %v", err)
	}
	if w.Version != Version || w.Name != "" || w.Addrs == nil || len(w.Addrs) != 0 || !w.Owner || w.Time <= 0 {
		t.Fatalf("unexpected welcome: %+v", w)
	}
	var dl proto.Devlog
	if err := proto.Decode(fc.next(t), &dl); err != nil || len(dl.Entries) != 3 {
		t.Fatalf("expected 3-entry devlog after welcome, got %+v (%v)", dl, err)
	}
}

func TestFramingHelloAndBootstrapRules(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, otherKey := sesKey("node"), sesKey("browser"), sesKey("other")
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
	var adoptCalls int
	var adoptedOwner string
	s := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Version: "v0.3.1", Store: db, Logs: logs, Home: loop, Host: h,
		Name:    func() string { return "workstation" },
		Addrs:   func() []string { return []string{"wss://rv.example/rv"} },
		Now:     func() time.Time { return now },
		OnAdopt: func(owner string) { adoptCalls++; adoptedOwner = owner },
	})
	s.Requests["host.info"] = RequestHandler{
		Type: "host.info",
		Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return map[string]any{"name": "workstation"}, nil
		},
	}
	var pushSeen int
	s.Pushes["pty.resize"] = func(context.Context, *home.Conn, Ctl, string, json.RawMessage) { pushSeen++ }

	fc := serveConn(t, s, browserKey.ID(), true)

	fc.pushRaw(`123`)
	fc.pushRaw(`{"id":91}`)
	expectErr(t, fc.next(t), 91, "bad_request")
	fc.pushRaw(`{"x":1}`)
	fc.pushRaw(`{"re":5,"ok":true}`)
	fc.pushRaw(`{"t":"welcome","re":5,"id":92,"ok":true}`)
	expectErr(t, fc.next(t), 92, "bad_request")

	fc.pushRaw(`{"t":"host.info","id":1}`)
	expectErr(t, fc.next(t), 1, "forbidden")
	fc.pushRaw(`{"t":"pty.resize"}`)
	fc.expectNone(t)
	if pushSeen != 0 {
		t.Fatal("push ran before enrolment")
	}

	fc.pushRaw(`{"t":"hello","id":2}`)
	expectErr(t, fc.next(t), 2, "bad_request")
	fc.pushRaw(`{"t":"hello","id":3,"v":0,"kind":"browser","name":"p","addrs":[]}`)
	expectErr(t, fc.next(t), 3, "version")
	fc.pushRaw(`{"t":"hello","id":4,"v":2}`)
	expectErr(t, fc.next(t), 4, "version")
	fc.pushRaw(`{"t":"hello","id":5,"v":1,"kind":"browser"}`)
	expectErr(t, fc.next(t), 5, "bad_request")
	fc.pushRaw(`{"t":"hello","id":6,"v":1,"kind":"tablet","name":"p","addrs":[]}`)
	expectErr(t, fc.next(t), 6, "bad_request")

	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 7}, V: 1, Kind: "browser", Name: "phone", Addrs: []string{}})
	var w1 proto.Welcome
	if err := proto.Decode(fc.next(t), &w1); err != nil {
		t.Fatal(err)
	}
	if w1.Re != 7 || w1.Owner || w1.Device != nodeKey.ID() || w1.Name != "workstation" || w1.Version != "v0.3.1" || w1.Time != now.UnixMilli() {
		t.Fatalf("unexpected welcome 1: %+v", w1)
	}
	fc.expectNone(t)

	fc.pushRaw(`{"t":"host.info","id":8}`)
	expectErr(t, fc.next(t), 8, "forbidden")
	fc.pushRaw(`{"t":"pty.resize"}`)
	if pushSeen != 0 {
		t.Fatal("push ran while welcomed but not known in log")
	}

	fc.pushRaw(`{"t":"devlog","user":123}`)
	fc.push(proto.Devlog{Type: "devlog", User: "bad", Entries: []proto.Envelope{{}}})
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{{Payload: "{}", Key: browserKey.ID(), Signature: "bad"}}})

	// Non-paired connection on an unowned node cannot trigger adoption.
	unpaired := serveConn(t, s, otherKey.ID(), false)
	o1 := sesAddEntry(otherKey, otherKey.ID(), 1, otherKey.ID(), "browser", "other")
	o2 := sesAddEntry(otherKey, otherKey.ID(), 2, nodeKey.ID(), "node", "workstation")
	unpaired.push(proto.Devlog{Type: "devlog", User: otherKey.ID(), Entries: []proto.Envelope{o1, o2}})
	unpaired.pushRaw(`{"t":"hello","id":1,"v":2}`)
	expectErr(t, unpaired.next(t), 1, "version")
	if logs.Owner() != "" {
		t.Fatal("unpaired connection adopted node")
	}

	e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "phone")
	e2Other := sesAddEntry(browserKey, browserKey.ID(), 2, otherKey.ID(), "browser", "other")
	e3WrongSigner := sesAddEntry(otherKey, browserKey.ID(), 3, nodeKey.ID(), "node", "workstation")
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1, e2Other, e3WrongSigner}})
	if logs.Owner() != "" || adoptedOwner != "" {
		t.Fatalf("adopted wrong signer log: %s", logs.Owner())
	}

	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1, e2Other}})
	if logs.Owner() != "" {
		t.Fatal("adopted incomplete log")
	}
	e3Valid := sesAddEntry(browserKey, browserKey.ID(), 3, nodeKey.ID(), "node", "workstation")
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3Valid}})

	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 9}, V: 1, Kind: "browser", Name: "phone", Addrs: []string{}})
	var w2 proto.Welcome
	if err := proto.Decode(fc.next(t), &w2); err != nil {
		t.Fatal(err)
	}
	if !w2.Owner || adoptedOwner != browserKey.ID() || adoptCalls != 1 {
		t.Fatalf("expected owner=true after adoption: %+v, adopted=%s calls=%d", w2, adoptedOwner, adoptCalls)
	}
	var pushedLog proto.Devlog
	if err := proto.Decode(fc.next(t), &pushedLog); err != nil || len(pushedLog.Entries) != 3 {
		t.Fatalf("expected 3-entry devlog after welcome, got %+v (%v)", pushedLog, err)
	}

	// Subsequent devlog entry does not call OnAdopt a second time.
	e4 := sesAddEntry(browserKey, browserKey.ID(), 4, sesKey("extra").ID(), "browser", "extra")
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e4}})

	fc.pushRaw(`{"t":"unknown_push"}`)
	fc.pushRaw(`{"t":"pty.resize"}`)
	fc.pushRaw(`{"t":"host.info","id":10}`)
	raw := fc.next(t)
	var hi struct {
		Type string     `json:"t"`
		Re   int64      `json:"re"`
		OK   proto.True `json:"ok"`
		Name string     `json:"name"`
	}
	if err := proto.Decode(raw, &hi); err != nil || hi.Type != "host.info" || hi.Re != 10 || hi.Name != "workstation" || adoptCalls != 1 || pushSeen != 1 {
		t.Fatalf("host.info answer = %s (%v), adoptCalls=%d pushSeen=%d", raw, err, adoptCalls, pushSeen)
	}
}

func TestHomeLoopIntegrationAndDevlogForwarding(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, b1Key, b2Key, hostKey := sesKey("node"), sesKey("b1"), sesKey("b2"), sesKey("aihost")
	logs := seedOwnedLogs(t, db, nodeKey, b1Key)
	e3 := sesAddEntry(b1Key, b1Key.ID(), 3, b2Key.ID(), "browser", "tablet")
	e4 := sesAddEntry(b1Key, b1Key.ID(), 4, hostKey.ID(), "node", "ai-host")
	if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: b1Key.ID(), Entries: []proto.Envelope{e3, e4}}, nil); err != nil {
		t.Fatal(err)
	}

	now := time.UnixMilli(1763600000000)
	groupID := string(sesID("group1"))
	cmds := home.Table{
		"group/create": func(st *home.State, c home.Command) (home.Result, *home.Refusal) {
			grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: st.ID}, Name: "Proj", Kind: "project", Home: st.Home, Checks: []string{}, Created: c.Now}
			own := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-own"}, Group: st.ID, Kind: "human", User: b1Key.ID(), Name: "owner"}, Role: "owner"}
			ai := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-ai"}, Group: st.ID, Kind: "ai", User: b1Key.ID(), Name: "Codex"}, CLI: "codex", Host: hostKey.ID(), Account: "personal", Allow: []string{}, Approvers: []string{b1Key.ID()}, State: "waiting", Path: "direct"}
			return home.Result{
				Rows:          []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(ai)},
				Items:         []home.Item{{AI: "m-ai", Why: "mention"}},
				GroupsChanged: true,
			}, nil
		},
	}
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, cmds)

	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }})

	c1 := serveConn(t, s, b1Key.ID(), false)
	c1.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	var wb1 proto.Welcome
	if err := proto.Decode(c1.next(t), &wb1); err != nil || !wb1.Owner {
		t.Fatalf("b1 welcome: %+v (%v)", wb1, err)
	}
	c1.next(t)

	c2 := serveConn(t, s, b2Key.ID(), false)
	c2.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b2", Addrs: []string{}})
	c2.next(t)
	c2.next(t)

	cmdPayload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q}`, groupID, now.UnixMilli(), groupID)
	c1.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: b1Key.Sign(cmdPayload)})
	var createAns proto.Success
	if err := proto.Decode(c1.next(t), &createAns); err != nil || createAns.Re != 2 {
		t.Fatalf("create answer: %v", err)
	}
	c1.next(t)
	c2.next(t)

	// Unwelcomed host connection cannot acknowledge queued items before hello.
	cUnwHost := serveConn(t, s, hostKey.ID(), false)
	cUnwHost.push(proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	cUnwHost.pushRaw(`{"t":"sync","id":1,"g":"` + groupID + `","since":0,"state_only":true}`)
	expectErr(t, cUnwHost.next(t), 1, "forbidden")
	cUnwHost.Close()

	// Host hello receives welcome, owner devlog and queued inbox items; browser hello does not trigger Inbox.
	ch := serveConn(t, s, hostKey.ID(), false)
	ch.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "host", Name: "ai-host", Addrs: []string{}})
	var wh proto.Welcome
	if err := proto.Decode(ch.next(t), &wh); err != nil || !wh.Owner || len(wh.Groups) != 1 || wh.Groups[0].Group != groupID {
		t.Fatalf("host welcome: %+v (%v)", wh, err)
	}
	ch.next(t)
	inboxRaw := ch.next(t)
	if !strings.Contains(string(inboxRaw), `"why":"mention"`) {
		t.Fatalf("expected inbox item on host hello, got %s", inboxRaw)
	}
	ch.pushRaw(`{"t":"sync","id":20,"g":123}`)
	expectErr(t, ch.next(t), 20, "bad_request")
	ch.push(proto.Sync{Request: proto.Request{Type: "sync", ID: 2}, Group: groupID, Since: 3, StateOnly: true})
	var synced proto.Synced
	if err := proto.Decode(ch.next(t), &synced); err != nil || synced.Re != 2 {
		t.Fatalf("host sync = %+v (%v)", synced, err)
	}
	sesCheckHostAck(t, ch, groupID)

	// Pairing peer on an owned node receives the owner's log after welcome and on subsequent updates.
	newPairKey := sesKey("newpair")
	cPair := serveConn(t, s, newPairKey.ID(), true)
	cPair.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "np", Addrs: []string{}})
	var wPair proto.Welcome
	if err := proto.Decode(cPair.next(t), &wPair); err != nil || wPair.Owner || len(wPair.Groups) != 0 {
		t.Fatalf("pair welcome: %+v (%v)", wPair, err)
	}
	var dlPair proto.Devlog
	if err := proto.Decode(cPair.next(t), &dlPair); err != nil || dlPair.User != b1Key.ID() {
		t.Fatalf("pair devlog: %+v (%v)", dlPair, err)
	}
	cPair.expectNone(t)

	// Unwelcomed owner connection is refused requests and gets no forwarded devlogs; welcomed peers get them.
	cUnwelcomed := serveConn(t, s, b1Key.ID(), false)
	cUnwelcomed.pushRaw(`{"t":"sync","id":77,"g":"` + groupID + `","since":0,"state_only":false}`)
	expectErr(t, cUnwelcomed.next(t), 77, "forbidden")
	renameB2 := sesAddEntry(b1Key, b1Key.ID(), 5, b2Key.ID(), "browser", "tablet-renamed")
	c1.push(proto.Devlog{Type: "devlog", User: b1Key.ID(), Entries: []proto.Envelope{renameB2}})
	var fwdB2, fwdPair proto.Devlog
	if err := proto.Decode(c2.next(t), &fwdB2); err != nil || len(fwdB2.Entries) != 1 {
		t.Fatalf("expected rename devlog on c2, got %+v (%v)", fwdB2, err)
	}
	if err := proto.Decode(cPair.next(t), &fwdPair); err != nil || len(fwdPair.Entries) != 1 {
		t.Fatalf("expected rename devlog on cPair, got %+v (%v)", fwdPair, err)
	}
	c1.expectNone(t)
	cUnwelcomed.expectNone(t)
	cUnwelcomed.Close()
	cPair.Close()
	gotHostDevlog, gotHostRow := false, false
	for range 2 {
		msg := string(ch.next(t))
		if strings.Contains(msg, `"t":"devlog"`) {
			gotHostDevlog = true
		}
		if strings.Contains(msg, `"r":"device"`) {
			gotHostRow = true
		}
	}
	if !gotHostDevlog || !gotHostRow {
		t.Fatalf("host frames: devlog=%v row=%v", gotHostDevlog, gotHostRow)
	}

	// Revoking b2 leaves c2 open (rule 10), stops forwarding devlogs to c2, and refuses requests on c2.
	rev := sesRevokeEntry(b1Key, b1Key.ID(), 6, b2Key.ID())
	c1.push(proto.Devlog{Type: "devlog", User: b1Key.ID(), Entries: []proto.Envelope{rev}})
	if got := string(c2.next(t)); !strings.Contains(got, `"groups":[]`) {
		t.Fatalf("expected empty groups push on revoked c2, got %s", got)
	}
	c2.expectNone(t)
	c2.pushRaw(`{"t":"sync","id":9,"g":"` + groupID + `","since":0,"state_only":false}`)
	expectErr(t, c2.next(t), 9, "forbidden")
	c2.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 10}, V: 1, Kind: "browser", Name: "b2", Addrs: []string{}})
	var wRev proto.Welcome
	if err := proto.Decode(c2.next(t), &wRev); err != nil || wRev.Owner || len(wRev.Groups) != 0 {
		t.Fatalf("revoked welcome: %+v (%v)", wRev, err)
	}
	c2.expectNone(t)
	select {
	case <-c2.Done():
		t.Fatal("revoked connection should remain open")
	default:
	}

	// Failing Send on attached home connection causes home.Conn.watch to call (*sesConn).Close.
	cFail := serveConn(t, s, b1Key.ID(), false)
	cFail.mu.Lock()
	cFail.failAfter = 1
	cFail.mu.Unlock()
	cFail.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "fail", Addrs: []string{}})
	select {
	case <-cFail.Done():
	case <-time.After(time.Second):
		t.Fatal("broken connection did not close")
	}
}

func sesCheckHostAck(t *testing.T, ch *fakeCtl, groupID string) {
	t.Helper()
	// Malformed ack is ignored; a fresh host hello resends item 1.
	ch.pushRaw(`{"t":"ack","g":"bad","item":1}`)
	ch.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 3}, V: 1, Kind: "host", Name: "ai-host", Addrs: []string{}})
	ch.next(t)
	ch.next(t)
	if got := string(ch.next(t)); !strings.Contains(got, `"why":"mention"`) {
		t.Fatalf("expected resent inbox item after bad ack, got %s", got)
	}
	// Valid ack routes to Loop.Ack and deletes item 1, so the next host hello sends no inbox frame.
	ch.push(proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	ch.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 4}, V: 1, Kind: "host", Name: "ai-host", Addrs: []string{}})
	ch.next(t)
	ch.next(t)
	ch.push(proto.Sync{Request: proto.Request{Type: "sync", ID: 5}, Group: groupID, Since: 3, StateOnly: true})
	var synced proto.Synced
	if err := proto.Decode(ch.next(t), &synced); err != nil || synced.Re != 5 {
		t.Fatalf("sync after ack = %+v (%v)", synced, err)
	}
	ch.expectNone(t)
}

func TestHostCommandsReceiptsAndSurfaceBridge(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, b2Key := sesKey("node"), sesKey("browser"), sesKey("b2")
	logs := seedOwnedLogs(t, db, nodeKey, browserKey)
	e3 := sesAddEntry(browserKey, browserKey.ID(), 3, b2Key.ID(), "browser", "b2")
	if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3}}, nil); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1763600000000)
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)

	var agentSetCalls atomic.Int32
	host.Surface["agent.set"] = func(_ context.Context, _ *host.Host, _ host.Owner, _ string, _ json.RawMessage) (any, error) {
		agentSetCalls.Add(1)
		return nil, nil
	}
	t.Cleanup(func() { delete(host.Surface, "agent.set") })

	var reportMu sync.Mutex
	var reported []error
	s := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h,
		Now:    func() time.Time { return now },
		Report: func(err error) { reportMu.Lock(); reported = append(reported, err); reportMu.Unlock() },
	})

	var refusalCalls atomic.Int32
	s.Commands["settings.set"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		refusalCalls.Add(1)
		return nil, &host.Error{Code: "bad_request", Message: "no field set"}
	}
	s.Commands["host.offline"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		return nil, errors.New("boom")
	}
	s.Commands["pair.browser"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		return map[string]any{"t": "ignored", "link": "https://example/#pair=SECRET", "expires": now.UnixMilli() + 600_000}, nil
	}

	c := serveConn(t, s, browserKey.ID(), false)
	c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	c.next(t)
	c.next(t)

	c.pushRaw(`{"t":"bogus","id":2}`)
	expectErr(t, c.next(t), 2, "bad_request")
	c.pushRaw(`{"t":"host.info","id":3}`)
	expectErr(t, c.next(t), 3, "bad_request")

	c.pushRaw(`{"t":"cmd","id":10}`)
	expectErr(t, c.next(t), 10, "bad_request")
	wrongSigner := b2Key.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("ws"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 11}, Env: wrongSigner})
	expectErr(t, c.next(t), 11, "forbidden")
	badSig := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":0,"to":%q}`, sesID("bs"), nodeKey.ID()))
	badSig.Signature = strings.Repeat("A", 86)
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 12}, Env: badSig})
	expectErr(t, c.next(t), 12, "bad_sig")
	badPayload := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":"not-an-id","ts":0,"to":%q}`, nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 13}, Env: badPayload})
	expectErr(t, c.next(t), 13, "bad_request")
	nonJSONPayload := browserKey.Sign(`not-json`)
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 130}, Env: nonJSONPayload})
	expectErr(t, c.next(t), 130, "bad_request")
	noGroupNoTo := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d}`, sesID("ngnt"), now.UnixMilli()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 14}, Env: noGroupNoTo})
	expectErr(t, c.next(t), 14, "bad_request")
	wrongTo := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":0,"to":%q}`, sesID("wt"), b2Key.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 15}, Env: wrongTo})
	expectErr(t, c.next(t), 15, "not_for_me")
	staleEnv := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("stale"), now.Add(-24*time.Hour-time.Millisecond).UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 16}, Env: staleEnv})
	expectErr(t, c.next(t), 16, "stale")

	exactBoundaryEnv := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("exact24h"), now.Add(-24*time.Hour).UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 17}, Env: exactBoundaryEnv})
	var ok17 proto.Success
	if err := proto.Decode(c.next(t), &ok17); err != nil || ok17.Re != 17 || agentSetCalls.Load() != 1 {
		t.Fatalf("exact 24h = %+v (%v), calls=%d", ok17, err, agentSetCalls.Load())
	}
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 18}, Env: exactBoundaryEnv})
	var ok18 proto.Success
	if err := proto.Decode(c.next(t), &ok18); err != nil || ok18.Re != 18 || agentSetCalls.Load() != 1 {
		t.Fatalf("replayed agent.set = %+v (%v), calls=%d", ok18, err, agentSetCalls.Load())
	}

	refEnv := browserKey.Sign(fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q}`, sesID("ref"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 19}, Env: refEnv})
	expectErr(t, c.next(t), 19, "bad_request")
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 20}, Env: refEnv})
	expectErr(t, c.next(t), 20, "bad_request")
	if refusalCalls.Load() != 1 {
		t.Fatalf("refusalCalls = %d, want 1", refusalCalls.Load())
	}

	unknownHostCmd := browserKey.Sign(fmt.Sprintf(`{"t":"perm.set","n":%q,"ts":%d,"to":%q,"g":%q}`, sesID("perm1"), now.UnixMilli(), nodeKey.ID(), sesID("g")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 21}, Env: unknownHostCmd})
	expectErr(t, c.next(t), 21, "bad_request")

	homeMissingGroup := browserKey.Sign(fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q}`, sesID("hnl"), now.UnixMilli(), sesID("g")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 22}, Env: homeMissingGroup})
	expectErr(t, c.next(t), 22, "bad_request")

	boomEnv := browserKey.Sign(fmt.Sprintf(`{"t":"host.offline","n":%q,"ts":%d,"to":%q}`, sesID("boom"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 23}, Env: boomEnv})
	expectErr(t, c.next(t), 23, "internal")
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 231}, Env: boomEnv})
	expectErr(t, c.next(t), 231, "internal")

	pairEnv := browserKey.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, sesID("pair1"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 24}, Env: pairEnv})
	rawPair := c.next(t)
	var pr proto.CmdResult
	if err := proto.Decode(rawPair, &pr); err != nil || pr.Re != 24 || pr.Link != "https://example/#pair=SECRET" || strings.Contains(string(rawPair), `"t":`) {
		t.Fatalf("pair.browser = %s (%v)", rawPair, err)
	}

	reportMu.Lock()
	numReported := len(reported)
	reportMu.Unlock()
	if numReported == 0 {
		t.Fatal("expected internal error to be reported")
	}
}

func TestBlockingHostCmdDoesNotHoldUpHomeCmdOrInflightDuplicate(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, b1Key, b2Key := sesKey("node"), sesKey("b1"), sesKey("b2")
	logs := seedOwnedLogs(t, db, nodeKey, b1Key)
	e3 := sesAddEntry(b1Key, b1Key.ID(), 3, b2Key.ID(), "browser", "b2")
	if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: b1Key.ID(), Entries: []proto.Envelope{e3}}, nil); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1763600000000)
	groupID := string(sesID("grp-async"))
	cmds := home.Table{
		"group/create": func(st *home.State, c home.Command) (home.Result, *home.Refusal) {
			grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: st.ID}, Name: "Async", Kind: "project", Home: st.Home, Checks: []string{}, Created: c.Now}
			own := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-own"}, Group: st.ID, Kind: "human", User: b1Key.ID(), Name: "owner"}, Role: "owner"}
			return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(own)}}, nil
		},
	}
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, cmds)
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }})

	started := make(chan struct{}) // single reader: this test
	release := make(chan struct{})
	var joinRuns atomic.Int32
	s.Commands["host.join"] = func(_ context.Context, _ Ctl, by string, _ json.RawMessage) (any, error) {
		if by == b1Key.ID() && joinRuns.Add(1) == 1 {
			close(started)
			<-release
		}
		return nil, nil
	}
	var fastRuns atomic.Int32
	s.Commands["settings.set"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		fastRuns.Add(1)
		return nil, nil
	}

	c1 := serveConn(t, s, b1Key.ID(), false)
	c1.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	c1.next(t)
	c1.next(t)

	// 1. Send blocking host.join (id 2), wait until its handler is running.
	sharedNonce := sesID("join-nonce")
	joinEnv := b1Key.Sign(fmt.Sprintf(`{"t":"host.join","n":%q,"ts":%d,"to":%q}`, sharedNonce, now.UnixMilli(), nodeKey.ID()))
	c1.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: joinEnv})
	<-started

	// 2. While host.join is blocked, send a home cmd (id 3) and a distinct host cmd (id 4) on the same connection.
	createEnv := b1Key.Sign(fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q}`, groupID, now.UnixMilli(), groupID))
	c1.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: createEnv})
	var homeAns proto.Success
	if err := proto.Decode(c1.next(t), &homeAns); err != nil || homeAns.Re != 3 {
		t.Fatalf("home cmd blocked behind host cmd: %+v (%v)", homeAns, err)
	}

	fastEnv := b1Key.Sign(fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q}`, sesID("fast-nonce"), now.UnixMilli(), nodeKey.ID()))
	c1.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 4}, Env: fastEnv})
	var fastAns proto.Success
	if err := proto.Decode(c1.next(t), &fastAns); err != nil || fastAns.Re != 4 || fastRuns.Load() != 1 {
		t.Fatalf("distinct host cmd blocked behind inflight cmd: %+v (%v)", fastAns, err)
	}

	// 3. Different signer (b2) with the same nonce does not wait on b1's inflight entry.
	c2 := serveConn(t, s, b2Key.ID(), false)
	c2.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b2", Addrs: []string{}})
	c2.next(t)
	c2.next(t)
	b2SameNonce := b2Key.Sign(fmt.Sprintf(`{"t":"host.join","n":%q,"ts":%d,"to":%q}`, sharedNonce, now.UnixMilli(), nodeKey.ID()))
	c2.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 5}, Env: b2SameNonce})
	var b2Ans proto.Success
	if err := proto.Decode(c2.next(t), &b2Ans); err != nil || b2Ans.Re != 5 {
		t.Fatalf("different signer with same nonce blocked: %+v (%v)", b2Ans, err)
	}

	// 4. Duplicate (b1, sharedNonce) on a cancellable context exits cleanly on ctx cancellation.
	cancelCtx, cancel := context.WithCancel(t.Context())
	cCancel := serveConnCtx(t, cancelCtx, s, b1Key.ID(), false)
	cCancel.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "bc", Addrs: []string{}})
	cCancel.next(t)
	cCancel.next(t)
	cCancel.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 6}, Env: joinEnv})
	time.Sleep(15 * time.Millisecond)
	cancel()
	cCancel.expectNone(t)

	// 5. Duplicate (b1, sharedNonce) on c1 waits for the first run, then gets the stored receipt without re-running.
	c1.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 7}, Env: joinEnv})
	c1.expectNone(t)
	close(release)

	gotRe := map[int64]bool{}
	for range 2 {
		var ans proto.Success
		if err := proto.Decode(c1.next(t), &ans); err != nil {
			t.Fatal(err)
		}
		gotRe[ans.Re] = true
	}
	if !gotRe[2] || !gotRe[7] || joinRuns.Load() != 1 {
		t.Fatalf("gotRe=%v joinRuns=%d, want {2,7} and 1 run", gotRe, joinRuns.Load())
	}
}

func TestSessionErrorAndEdgePaths(t *testing.T) {
	nodeKey, browserKey := sesKey("node"), sesKey("browser")
	now := time.UnixMilli(1763600000000)

	t.Run("reply and answer encoding errors", func(t *testing.T) {
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, browserKey)
		loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }, Report: func(error) {}})
		s.Requests["wire_err"] = RequestHandler{Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return nil, &host.Error{Code: "busy", Message: "busy now"}
		}}
		s.Requests["plain_err"] = RequestHandler{Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return nil, errors.New("unexpected")
		}}
		s.Requests["bad_marshal"] = RequestHandler{Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return make(chan int), nil
		}}
		s.Requests["non_obj"] = RequestHandler{Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return "not-an-object", nil
		}}
		s.Requests["typed_nil"] = RequestHandler{Type: "custom", Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return nil, nil
		}}

		c := serveConn(t, s, browserKey.ID(), false)
		c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
		c.next(t)
		c.next(t)

		c.pushRaw(`{"t":"wire_err","id":2}`)
		expectErr(t, c.next(t), 2, "busy")
		c.pushRaw(`{"t":"plain_err","id":3}`)
		expectErr(t, c.next(t), 3, "internal")
		c.pushRaw(`{"t":"bad_marshal","id":4}`)
		expectErr(t, c.next(t), 4, "internal")
		c.pushRaw(`{"t":"non_obj","id":5}`)
		expectErr(t, c.next(t), 5, "internal")
		c.pushRaw(`{"t":"typed_nil","id":6}`)
		if got := string(c.next(t)); !strings.Contains(got, `"t":"custom"`) {
			t.Fatalf("typed_nil = %s", got)
		}
	})

	t.Run("receipt read and write failures", func(t *testing.T) {
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, browserKey)
		loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }, Report: func(error) {}})
		s.Commands["agent.set"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
			drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE receipt"); return err }
			if err := db.Write(drop); err != nil {
				return nil, err
			}
			return nil, nil
		}
		c := serveConn(t, s, browserKey.ID(), false)
		c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
		c.next(t)
		c.next(t)

		env1 := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("rw1"), now.UnixMilli(), nodeKey.ID()))
		c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: env1})
		expectErr(t, c.next(t), 2, "internal")

		env2 := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("rw2"), now.UnixMilli(), nodeKey.ID()))
		c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: env2})
		expectErr(t, c.next(t), 3, "internal")
	})

	t.Run("send failures on welcome and devlog", func(t *testing.T) {
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, browserKey)
		loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Now: func() time.Time { return now }, Report: func(error) {}})

		onSent := func() []proto.Devlog { return logs.frames(browserKey.ID()) }
		fc1 := newFakeCtl()
		fc1.failAfter = 1
		sc1 := &sesConn{device: browserKey.ID(), raw: fc1}
		if err := sc1.Send(sesWelcome{welcome: proto.Welcome{Type: "welcome"}, onSent: onSent, sent: make(chan struct{})}); err == nil {
			t.Fatal("expected welcome send error")
		}

		fc2 := newFakeCtl()
		fc2.failAfter = 2
		sc2 := &sesConn{device: browserKey.ID(), raw: fc2}
		if err := sc2.Send(sesWelcome{welcome: proto.Welcome{Type: "welcome"}, onSent: onSent, sent: make(chan struct{})}); err == nil {
			t.Fatal("expected devlog send error")
		}

		c := serveConn(t, s, browserKey.ID(), false)
		c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
		c.next(t)
		c.next(t)
		c.mu.Lock()
		c.failAfter = c.sends + 1
		c.mu.Unlock()
		e3 := sesAddEntry(browserKey, browserKey.ID(), 3, sesKey("d3").ID(), "browser", "d3")
		if err := logs.dvlReceive(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e3}}, nil); err != nil {
			t.Fatal(err)
		}

		gone := make(chan struct{})
		close(gone)
		sesWaitWelcome(t.Context(), make(chan struct{}), gone)
		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		sesWaitWelcome(canceled, make(chan struct{}), make(chan struct{}))
	})

	t.Run("adoption and receive db errors are reported", func(t *testing.T) {
		db := sesOpenStore(t)
		logs, err := OpenLogs(db, nodeKey.ID())
		if err != nil {
			t.Fatal(err)
		}
		loop, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
		var errs atomic.Int32
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Host: h, Report: func(error) { errs.Add(1) }})
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
		if err := db.Write(drop); err != nil {
			t.Fatal(err)
		}
		c := serveConn(t, s, browserKey.ID(), true)
		e1 := sesAddEntry(browserKey, browserKey.ID(), 1, browserKey.ID(), "browser", "b")
		e2 := sesAddEntry(browserKey, browserKey.ID(), 2, nodeKey.ID(), "node", "n")
		c.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1, e2}})
		c.pushRaw(`{"t":"hello","id":98,"v":2}`)
		expectErr(t, c.next(t), 98, "version")
		logs.mu.Lock()
		logs.owner = browserKey.ID()
		logs.mu.Unlock()
		c.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1}})
		c.pushRaw(`{"t":"hello","id":99,"v":2}`)
		expectErr(t, c.next(t), 99, "version")
		if errs.Load() != 2 {
			t.Fatalf("expected 2 reported db errors, got %d", errs.Load())
		}
	})

	t.Run("SetHome reattaches open connections and statusLines lists welcomed ones", func(t *testing.T) {
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, browserKey)
		loop1, h := newTestHomeAndHost(t, db, nodeKey, logs, now, nil)
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop1, Host: h, Now: func() time.Time { return now }})
		unw := serveConn(t, s, browserKey.ID(), false)
		defer unw.Close()
		c := serveConn(t, s, browserKey.ID(), false)
		c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
		c.next(t)
		c.next(t)
		if got := s.statusLines(); len(got) != 1 || got[0] != "connection "+browserKey.ID()+" direct" {
			t.Fatalf("statusLines = %v", got)
		}
		loop2, err := home.StartLoop(home.LoopConfig{Store: db, Home: nodeKey.ID(), Owner: browserKey.ID(), Logs: logs.Log, Tables: home.NodeTables()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(loop2.Close)
		s.SetHome(loop2)
		loop1.Close()
		c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 2}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
		var w proto.Welcome
		if err := proto.Decode(c.next(t), &w); err != nil || w.Re != 2 || !w.Owner {
			t.Fatalf("welcome after SetHome = %+v (%v)", w, err)
		}
		c.next(t)
	})
}
