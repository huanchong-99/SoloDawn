package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
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
	f.sends++
	fail := f.failAfter > 0 && f.sends >= f.failAfter
	f.mu.Unlock()
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

func serveConn(t *testing.T, s *Sessions, device string, paired bool) *fakeCtl {
	t.Helper()
	fc := newFakeCtl()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Serve(t.Context(), device, paired, fc)
	}()
	t.Cleanup(func() {
		fc.Close()
		wg.Wait()
	})
	return fc
}

func TestSessionDefaultsAndUnadmittedConnection(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, stranger := sesKey("node"), sesKey("stranger")
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs})
	s.cfg.Report(errors.New("logged default error"))

	unadmitted := newFakeCtl()
	s.Serve(t.Context(), stranger.ID(), false, unadmitted)
	select {
	case <-unadmitted.Done():
	default:
		t.Fatal("expected unadmitted connection to close")
	}

	sNilAddrs := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Store: db, Logs: logs,
		Addrs: func() []string { return nil },
	})
	fc := serveConn(t, sNilAddrs, stranger.ID(), true)
	fc.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
	var w proto.Welcome
	if err := proto.Decode(fc.next(t), &w); err != nil {
		t.Fatal(err)
	}
	if w.Version != Version || w.Name != "" || w.Addrs == nil || len(w.Addrs) != 0 || w.Owner {
		t.Fatalf("unexpected welcome: %+v", w)
	}
}

func TestFramingHelloAndBootstrapRules(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, otherKey := sesKey("node"), sesKey("browser"), sesKey("other")
	logs, err := OpenLogs(db, nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	var adoptedOwner string
	now := time.UnixMilli(1763600000000)
	s := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Version: "v0.3.1", Store: db, Logs: logs,
		Name:    func() string { return "workstation" },
		Addrs:   func() []string { return []string{"wss://rv.example/rv"} },
		Now:     func() time.Time { return now },
		OnAdopt: func(owner string) { adoptedOwner = owner },
	})
	s.Requests["host.info"] = RequestHandler{
		Owner: true, Type: "host.info",
		Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return map[string]any{"name": "workstation"}, nil
		},
	}

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
	fc.pushRaw(`{"t":"pty.resize","g":"` + string(sesID("g")) + `","ai":"` + string(sesID("ai")) + `","cols":80,"rows":24}`)
	fc.expectNone(t)

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

	fc.pushRaw(`{"t":"devlog","user":123}`)
	fc.push(proto.Devlog{Type: "devlog", User: "bad", Entries: []proto.Envelope{{}}})
	fc.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{{Payload: "{}", Key: browserKey.ID(), Signature: "bad"}}})

	// Non-paired connection on an unowned node cannot trigger adoption.
	logs.Keep(otherKey.ID())
	o1 := sesAddEntry(otherKey, otherKey.ID(), 1, otherKey.ID(), "browser", "other")
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: otherKey.ID(), Entries: []proto.Envelope{o1}}); err != nil {
		t.Fatal(err)
	}
	unpaired := serveConn(t, s, otherKey.ID(), false)
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
	if !w2.Owner || adoptedOwner != browserKey.ID() {
		t.Fatalf("expected owner=true after adoption: %+v, adopted=%s", w2, adoptedOwner)
	}
	var pushedLog proto.Devlog
	if err := proto.Decode(fc.next(t), &pushedLog); err != nil || len(pushedLog.Entries) != 3 {
		t.Fatalf("expected 3-entry devlog after welcome, got %+v (%v)", pushedLog, err)
	}

	fc.pushRaw(`{"t":"unknown_push"}`)
	fc.pushRaw(`{"t":"host.info","id":10}`)
	raw := fc.next(t)
	var hi struct {
		Type string     `json:"t"`
		Re   int64      `json:"re"`
		OK   proto.True `json:"ok"`
		Name string     `json:"name"`
	}
	if err := proto.Decode(raw, &hi); err != nil || hi.Type != "host.info" || hi.Re != 10 || hi.Name != "workstation" {
		t.Fatalf("host.info answer = %s (%v)", raw, err)
	}
}

func TestHomeLoopIntegrationForwardingAndRevocation(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, b1Key, b2Key, memberKey, hostKey := sesKey("node"), sesKey("b1"), sesKey("b2"), sesKey("member"), sesKey("aihost")
	logs := seedOwnedLogs(t, db, nodeKey, b1Key)
	if _, err := logs.Append(sesAddEntry(b1Key, b1Key.ID(), 3, b2Key.ID(), "browser", "tablet")); err != nil {
		t.Fatal(err)
	}
	logs.Keep(memberKey.ID())
	m1 := sesAddEntry(memberKey, memberKey.ID(), 1, memberKey.ID(), "browser", "m-browser")
	m2 := sesAddEntry(memberKey, memberKey.ID(), 2, hostKey.ID(), "node", "m-host")
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: memberKey.ID(), Entries: []proto.Envelope{m1, m2}}); err != nil {
		t.Fatal(err)
	}

	now := time.UnixMilli(1763600000000)
	groupID := string(sesID("group1"))
	tables := home.Tables{Commands: home.Table{}, Timers: home.Rules[home.Timer]{}, Events: home.Events, After: home.Rules[home.Hook]{}}
	tables.Commands["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Proj", Kind: "project", Home: s.Home, Checks: []string{}, Created: c.Now}
		own := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-own"}, Group: s.ID, Kind: "human", User: b1Key.ID(), Name: "owner"}, Role: "owner"}
		mem := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-mem"}, Group: s.ID, Kind: "human", User: memberKey.ID(), Name: "ally"}, Role: "member"}
		ai := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-ai"}, Group: s.ID, Kind: "ai", User: memberKey.ID(), Name: "Codex"}, CLI: "codex", Host: hostKey.ID(), Account: "personal", Allow: []string{}, Approvers: []string{b1Key.ID()}, State: "waiting", Path: "direct"}
		return home.Result{
			Rows:          []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(mem), home.RowOf(dvlRowOf(m1)), home.RowOf(dvlRowOf(m2)), home.RowOf(ai)},
			Items:         []home.Item{{AI: "m-ai", Why: "mention"}},
			GroupsChanged: true,
		}, nil
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: nodeKey.ID(), Owner: b1Key.ID(),
		Logs: logs.Log, Tables: tables, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(loop.Close)

	acked := make(chan int64, 1) // single reader: this test
	s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Home: loop, Now: func() time.Time { return now }})
	s.Requests["sync"] = RequestHandler{
		Async: true,
		Run: func(_ context.Context, conn *home.Conn, from Ctl, _ string, id int64, data json.RawMessage) (any, error) {
			var q proto.Sync
			if proto.Decode(data, &q) != nil {
				from.Send(proto.ErrorResponse{Re: id, Err: proto.WireError{Code: "bad_request", Message: "bad sync"}})
				return nil, nil
			}
			seq, ok := loop.Watch(conn, q.Group, q.StateOnly)
			if !ok {
				from.Send(proto.ErrorResponse{Re: id, Err: proto.WireError{Code: "forbidden", Message: "not served"}})
				return nil, nil
			}
			from.Send(proto.Synced{Type: "synced", Success: proto.Success{Re: id, OK: true}, Group: q.Group, Seq: seq})
			return nil, nil
		},
	}
	s.Pushes["ack"] = PushHandler{
		Run: func(_ context.Context, _ *home.Conn, _ Ctl, _ string, data json.RawMessage) {
			var a proto.Ack
			if proto.Decode(data, &a) == nil {
				acked <- a.Item
			}
		},
	}

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

	// Non-owner host before group exists gets only its own person's log (groups: []).
	chEarly := serveConn(t, s, hostKey.ID(), false)
	chEarly.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "early", Addrs: []string{}})
	var wEarly proto.Welcome
	if err := proto.Decode(chEarly.next(t), &wEarly); err != nil || wEarly.Owner || len(wEarly.Groups) != 0 {
		t.Fatalf("early welcome: %+v (%v)", wEarly, err)
	}
	var dlEarly proto.Devlog
	if err := proto.Decode(chEarly.next(t), &dlEarly); err != nil || dlEarly.User != memberKey.ID() {
		t.Fatalf("early devlog: %+v (%v)", dlEarly, err)
	}
	chEarly.Close()

	cmdPayload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q}`, groupID, now.UnixMilli(), groupID)
	c1.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: b1Key.Sign(cmdPayload)})
	var createAns proto.Success
	if err := proto.Decode(c1.next(t), &createAns); err != nil || createAns.Re != 2 {
		t.Fatalf("create answer: %v", err)
	}
	c1.next(t)
	c2.next(t)

	ch := serveConn(t, s, hostKey.ID(), false)
	ch.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "host", Name: "m-host", Addrs: []string{}})
	var wh proto.Welcome
	if err := proto.Decode(ch.next(t), &wh); err != nil || wh.Owner || len(wh.Groups) != 1 || wh.Groups[0].Group != groupID {
		t.Fatalf("host welcome: %+v (%v)", wh, err)
	}
	ch.next(t)
	ch.next(t)
	inboxRaw := ch.next(t)
	if !strings.Contains(string(inboxRaw), `"Why":"mention"`) {
		t.Fatalf("expected inbox item on host hello, got %s", inboxRaw)
	}

	ch.push(proto.Sync{Request: proto.Request{Type: "sync", ID: 2}, Group: groupID, Since: 0, StateOnly: true})
	var synced proto.Synced
	if err := proto.Decode(ch.next(t), &synced); err != nil || synced.Re != 2 {
		t.Fatalf("host sync = %+v (%v)", synced, err)
	}
	ch.push(proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	ch.pushRaw(`{"t":"host.info","id":3}`)
	expectErr(t, ch.next(t), 3, "forbidden")
	ch.pushRaw(`{"t":"audit","id":4,"before":0,"limit":10}`)
	expectErr(t, ch.next(t), 4, "forbidden")
	hostCmdEnv := hostKey.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, sesID("hc"), now.UnixMilli(), nodeKey.ID()))
	ch.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 5}, Env: hostCmdEnv})
	expectErr(t, ch.next(t), 5, "forbidden")
	if got := <-acked; got != 1 {
		t.Fatalf("acked = %d, want 1", got)
	}

	// Pairing peer on an owned node receives only the owner's log after welcome.
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
	cPair.Close()

	// Unwelcomed connection does not receive forwarded devlogs; renaming b2 keeps c2 open.
	cUnwelcomed := serveConn(t, s, b1Key.ID(), false)
	renameB2 := sesAddEntry(b1Key, b1Key.ID(), 4, b2Key.ID(), "browser", "tablet-renamed")
	c1.push(proto.Devlog{Type: "devlog", User: b1Key.ID(), Entries: []proto.Envelope{renameB2}})
	var fwdB2 proto.Devlog
	if err := proto.Decode(c2.next(t), &fwdB2); err != nil || len(fwdB2.Entries) != 1 {
		t.Fatalf("expected rename devlog on c2, got %+v (%v)", fwdB2, err)
	}
	cUnwelcomed.expectNone(t)
	cUnwelcomed.Close()
	// Drain ch's forwarded devlog and device row from renameB2.
	ch.next(t)
	ch.next(t)

	rev := sesRevokeEntry(b1Key, b1Key.ID(), 5, b2Key.ID())
	c1.push(proto.Devlog{Type: "devlog", User: b1Key.ID(), Entries: []proto.Envelope{rev}})
	select {
	case <-c2.Done():
	case <-time.After(time.Second):
		t.Fatal("revoked browser connection did not close within 1s")
	}
	if got := string(c1.next(t)); !strings.Contains(got, `"t":"groups"`) {
		t.Fatalf("expected groups push on c1, got %s", got)
	}
	c1.expectNone(t)
	gotDevlog, gotDeviceRow, gotGroups := false, false, false
	for range 3 {
		msg := string(ch.next(t))
		if strings.Contains(msg, `"t":"devlog"`) {
			gotDevlog = true
		}
		if strings.Contains(msg, `"r":"device"`) {
			gotDeviceRow = true
		}
		if strings.Contains(msg, `"t":"groups"`) {
			gotGroups = true
		}
	}
	if !gotDevlog || !gotDeviceRow || !gotGroups {
		t.Fatalf("host got devlog=%v deviceRow=%v groups=%v", gotDevlog, gotDeviceRow, gotGroups)
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

func TestHostCommandsReceiptsAndHostSurfaceBridge(t *testing.T) {
	db := sesOpenStore(t)
	nodeKey, browserKey, b2Key, memberKey := sesKey("node"), sesKey("browser"), sesKey("b2"), sesKey("member")
	logs := seedOwnedLogs(t, db, nodeKey, browserKey)
	if _, err := logs.Append(sesAddEntry(browserKey, browserKey.ID(), 3, b2Key.ID(), "browser", "b2")); err != nil {
		t.Fatal(err)
	}
	logs.Keep(memberKey.ID())
	if err := logs.Receive(proto.Devlog{Type: "devlog", User: memberKey.ID(), Entries: []proto.Envelope{sesAddEntry(memberKey, memberKey.ID(), 1, memberKey.ID(), "browser", "m")}}); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1763600000000)
	h, _, err := host.New(db, nodeKey, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	var resized int
	var agentSetCalls int
	host.Surface["audit"] = func(_ context.Context, _ *host.Host, _ host.Owner, _ string, _ json.RawMessage) (any, error) {
		return proto.AuditAnswer{Rows: []proto.AuditRow{{ID: 1, TS: now.UnixMilli(), What: "setting", Detail: "x"}}}, nil
	}
	host.Surface["pty.resize"] = func(_ context.Context, _ *host.Host, _ host.Owner, _ string, _ json.RawMessage) (any, error) {
		resized++
		return nil, nil
	}
	host.Surface["agent.set"] = func(_ context.Context, _ *host.Host, _ host.Owner, _ string, _ json.RawMessage) (any, error) {
		agentSetCalls++
		return nil, nil
	}
	t.Cleanup(func() {
		delete(host.Surface, "audit")
		delete(host.Surface, "pty.resize")
		delete(host.Surface, "agent.set")
	})

	var reported []error
	s := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Store: db, Logs: logs, Host: h,
		Now:    func() time.Time { return now },
		Report: func(err error) { reported = append(reported, err) },
	})

	linkValid := true
	pairCalls := 0
	s.Commands["pair.browser"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		pairCalls++
		return map[string]any{"link": "https://example/#pair=SECRET", "expires": now.UnixMilli() + 600_000}, nil
	}
	s.Replays["pair.browser"] = func(context.Context, Ctl, string, string, json.RawMessage) (any, error) {
		if !linkValid {
			return nil, &Error{Code: "conflict", Message: "link expired"}
		}
		return map[string]any{"link": "https://example/#pair=SECRET", "expires": now.UnixMilli() + 600_000}, nil
	}
	s.Commands["takeover.start"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		return nil, nil
	}
	refusalCalls := 0
	s.Commands["settings.set"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		refusalCalls++
		return nil, &Error{Code: "bad_request", Message: "no field set"}
	}
	s.Commands["host.offline"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) {
		return nil, errors.New("boom")
	}

	c := serveConn(t, s, browserKey.ID(), false)
	c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b1", Addrs: []string{}})
	c.next(t)
	c.next(t)

	c.pushRaw(`{"t":"bogus","id":2}`)
	expectErr(t, c.next(t), 2, "bad_request")
	c.pushRaw(`{"t":"host.info","id":3}`)
	expectErr(t, c.next(t), 3, "bad_request")
	c.push(proto.Audit{Request: proto.Request{Type: "audit", ID: 4}, Before: 0, Limit: 10})
	var auditAns proto.AuditAnswer
	rawAudit := c.next(t)
	if strings.Contains(string(rawAudit), `"t":`) {
		t.Fatalf("audit answer must have no t field: %s", rawAudit)
	}
	if err := proto.Decode(rawAudit, &auditAns); err != nil || auditAns.Re != 4 || len(auditAns.Rows) != 1 {
		t.Fatalf("audit answer = %s (%v)", rawAudit, err)
	}
	c.push(proto.PTYResize{Type: "pty.resize", Group: sesID("g"), AI: sesID("ai"), Cols: 120, Rows: 40})

	c.pushRaw(`{"t":"cmd","id":10}`)
	expectErr(t, c.next(t), 10, "bad_request")
	wrongSigner := b2Key.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("ws"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 11}, Env: wrongSigner})
	expectErr(t, c.next(t), 11, "forbidden")
	badSig := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":0}`, sesID("bs")))
	badSig.Signature = strings.Repeat("A", 86)
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 12}, Env: badSig})
	expectErr(t, c.next(t), 12, "bad_sig")
	badPayload := browserKey.Sign(`{"t":"agent.set","n":"not-an-id","ts":0}`)
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 13}, Env: badPayload})
	expectErr(t, c.next(t), 13, "bad_request")
	nonJSONPayload := browserKey.Sign(`not-json`)
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 130}, Env: nonJSONPayload})
	expectErr(t, c.next(t), 130, "not_found")
	missingToFresh := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d}`, sesID("mtf"), now.UnixMilli()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 139}, Env: missingToFresh})
	expectErr(t, c.next(t), 139, "not_for_me")
	missingTo := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":0}`, sesID("mt")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 14}, Env: missingTo})
	expectErr(t, c.next(t), 14, "not_for_me")
	wrongTo := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":0,"to":%q}`, sesID("wt"), b2Key.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 15}, Env: wrongTo})
	expectErr(t, c.next(t), 15, "not_for_me")
	staleEnv := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("stale"), now.Add(-24*time.Hour-time.Millisecond).UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 16}, Env: staleEnv})
	expectErr(t, c.next(t), 16, "stale")

	exactBoundaryEnv := browserKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("exact24h"), now.Add(-24*time.Hour).UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 17}, Env: exactBoundaryEnv})
	var ok17 proto.Success
	if err := proto.Decode(c.next(t), &ok17); err != nil || ok17.Re != 17 || agentSetCalls != 1 {
		t.Fatalf("exact 24h = %+v (%v), calls=%d", ok17, err, agentSetCalls)
	}
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 18}, Env: exactBoundaryEnv})
	var ok18 proto.Success
	if err := proto.Decode(c.next(t), &ok18); err != nil || ok18.Re != 18 || agentSetCalls != 1 {
		t.Fatalf("replayed agent.set = %+v (%v), calls=%d", ok18, err, agentSetCalls)
	}

	refEnv := browserKey.Sign(fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q}`, sesID("ref"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 19}, Env: refEnv})
	expectErr(t, c.next(t), 19, "bad_request")
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 20}, Env: refEnv})
	expectErr(t, c.next(t), 20, "bad_request")
	if refusalCalls != 1 {
		t.Fatalf("refusalCalls = %d, want 1", refusalCalls)
	}

	auditAsCmd := browserKey.Sign(fmt.Sprintf(`{"t":"audit","n":%q,"ts":%d,"to":%q}`, sesID("audcmd"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 21}, Env: auditAsCmd})
	expectErr(t, c.next(t), 21, "bad_request")
	permUnreg := browserKey.Sign(fmt.Sprintf(`{"t":"perm.set","n":%q,"ts":%d,"to":%q,"g":%q}`, sesID("perm1"), now.UnixMilli(), nodeKey.ID(), sesID("g")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 210}, Env: permUnreg})
	expectErr(t, c.next(t), 210, "bad_request")
	s.Commands["custom.host"] = func(context.Context, Ctl, string, json.RawMessage) (any, error) { return nil, nil }
	customHost := browserKey.Sign(fmt.Sprintf(`{"t":"custom.host","n":%q,"ts":%d,"to":%q,"g":%q}`, sesID("ch1"), now.UnixMilli(), nodeKey.ID(), sesID("g")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 211}, Env: customHost})
	var okCustom proto.Success
	if err := proto.Decode(c.next(t), &okCustom); err != nil || okCustom.Re != 211 {
		t.Fatalf("custom.host = %+v (%v)", okCustom, err)
	}
	homeWithTo := browserKey.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"to":%q,"g":%q}`, sesID("hwt"), now.UnixMilli(), nodeKey.ID(), sesID("g")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 212}, Env: homeWithTo})
	expectErr(t, c.next(t), 212, "not_found")

	// Non-owner enrolled device cannot trigger owner push (pty.resize) or host commands.
	cm := serveConn(t, s, memberKey.ID(), false)
	cm.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "m", Addrs: []string{}})
	cm.next(t)
	cm.next(t)
	cm.push(proto.PTYResize{Type: "pty.resize", Group: sesID("g"), AI: sesID("ai"), Cols: 80, Rows: 24})
	cm.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: memberKey.Sign(fmt.Sprintf(`{"t":"agent.set","n":%q,"ts":%d,"to":%q}`, sesID("mhc"), now.UnixMilli(), nodeKey.ID()))})
	expectErr(t, cm.next(t), 2, "forbidden")
	cm.Close()

	// Missing audit/pty.resize surface entries fail cleanly without panicking.
	delete(host.Surface, "audit")
	delete(host.Surface, "pty.resize")
	c.push(proto.PTYResize{Type: "pty.resize", Group: sesID("g"), AI: sesID("ai"), Cols: 80, Rows: 24})
	c.push(proto.Audit{Request: proto.Request{Type: "audit", ID: 213}, Before: 0, Limit: 10})
	expectErr(t, c.next(t), 213, "bad_request")

	homeNoLoop := browserKey.Sign(fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q}`, sesID("hnl"), now.UnixMilli(), sesID("g")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 22}, Env: homeNoLoop})
	expectErr(t, c.next(t), 22, "not_found")

	boomEnv := browserKey.Sign(fmt.Sprintf(`{"t":"host.offline","n":%q,"ts":%d,"to":%q}`, sesID("boom"), now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 23}, Env: boomEnv})
	expectErr(t, c.next(t), 23, "internal")

	pairNonce := sesID("pair1")
	pairEnv := browserKey.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, pairNonce, now.UnixMilli(), nodeKey.ID()))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 24}, Env: pairEnv})
	var pr1 proto.CmdResult
	if err := proto.Decode(c.next(t), &pr1); err != nil || pr1.Link != "https://example/#pair=SECRET" || pairCalls != 1 {
		t.Fatalf("pair.browser 1 = %+v (%v)", pr1, err)
	}
	var storedAnswer string
	checkReceipt := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT answer FROM receipt WHERE k=? AND n=?", browserKey.ID(), string(pairNonce)).Scan(&storedAnswer)
	}
	if err := db.Write(checkReceipt); err != nil || strings.Contains(storedAnswer, "SECRET") || storedAnswer != `{"ok":true}` {
		t.Fatalf("stored receipt leaked secret or wrong shape: %q (%v)", storedAnswer, err)
	}
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 25}, Env: pairEnv})
	var pr2 proto.CmdResult
	if err := proto.Decode(c.next(t), &pr2); err != nil || pr2.Re != 25 || pr2.Link != pr1.Link || pairCalls != 1 {
		t.Fatalf("pair.browser replay = %+v (%v), calls=%d", pr2, err, pairCalls)
	}
	linkValid = false
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 26}, Env: pairEnv})
	expectErr(t, c.next(t), 26, "conflict")

	tkEnv := browserKey.Sign(fmt.Sprintf(`{"t":"takeover.start","n":%q,"ts":%d,"to":%q,"g":%q,"member":%q}`, sesID("tk1"), now.UnixMilli(), nodeKey.ID(), sesID("g"), sesID("m")))
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 27}, Env: tkEnv})
	var tkOK proto.Success
	if err := proto.Decode(c.next(t), &tkOK); err != nil || tkOK.Re != 27 {
		t.Fatalf("takeover.start 1 = %+v (%v)", tkOK, err)
	}
	c.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 28}, Env: tkEnv})
	expectErr(t, c.next(t), 28, "conflict")

	if resized != 1 || len(reported) == 0 {
		t.Fatalf("resized=%d reported=%d", resized, len(reported))
	}
}

func TestSessionErrorAndEdgePaths(t *testing.T) {
	nodeKey, browserKey := sesKey("node"), sesKey("browser")
	now := time.UnixMilli(1763600000000)

	t.Run("reply and answer encoding errors", func(t *testing.T) {
		db := sesOpenStore(t)
		logs := seedOwnedLogs(t, db, nodeKey, browserKey)
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Now: func() time.Time { return now }, Report: func(error) {}})
		s.Requests["wire_err"] = RequestHandler{Run: func(context.Context, *home.Conn, Ctl, string, int64, json.RawMessage) (any, error) {
			return nil, &Error{Code: "busy", Message: "busy now"}
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
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Now: func() time.Time { return now }, Report: func(error) {}})
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
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Now: func() time.Time { return now }, Report: func(error) {}})

		buildLogs := func([]proto.GroupSummary) []proto.Devlog { return logs.Frames(browserKey.ID()) }
		fc1 := newFakeCtl()
		fc1.failAfter = 1
		sc1 := &sesConn{device: browserKey.ID(), raw: fc1}
		if err := sc1.Send(sesWelcome{welcome: proto.Welcome{Type: "welcome"}, build: buildLogs, sent: make(chan struct{})}); err == nil {
			t.Fatal("expected welcome send error")
		}

		fc2 := newFakeCtl()
		fc2.failAfter = 2
		sc2 := &sesConn{device: browserKey.ID(), raw: fc2}
		if err := sc2.Send(sesWelcome{welcome: proto.Welcome{Type: "welcome"}, build: buildLogs, sent: make(chan struct{})}); err == nil {
			t.Fatal("expected devlog send error")
		}

		c := serveConn(t, s, browserKey.ID(), false)
		c.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
		c.next(t)
		c.next(t)
		c.mu.Lock()
		c.failAfter = c.sends + 1
		c.mu.Unlock()
		if _, err := logs.Append(sesAddEntry(browserKey, browserKey.ID(), 3, sesKey("d3").ID(), "browser", "d3")); err != nil {
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
		var errs int
		s := NewSessions(SessionConfig{ID: nodeKey.ID(), Store: db, Logs: logs, Report: func(error) { errs++ }})
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
		logs.owner, logs.kept[browserKey.ID()] = browserKey.ID(), true
		logs.mu.Unlock()
		c.push(proto.Devlog{Type: "devlog", User: browserKey.ID(), Entries: []proto.Envelope{e1}})
		c.pushRaw(`{"t":"hello","id":99,"v":2}`)
		expectErr(t, c.next(t), 99, "version")
		if errs != 2 {
			t.Fatalf("expected 2 reported db errors, got %d", errs)
		}
	})
}
