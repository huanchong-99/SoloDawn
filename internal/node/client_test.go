package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type ctlPair struct {
	client *clientPipe
	server *serverPipe
	done   chan struct{}
	once   sync.Once
}

type clientPipe struct {
	pair       *ctlPair
	toServer   chan json.RawMessage // single reader: serverPipe.Read
	fromServer chan json.RawMessage // single reader: clientPipe.Read
	pathCh     chan struct{}        // single reader: caller of PathChanged

	mu        sync.Mutex
	next      int64
	waiting   map[int64]chan json.RawMessage
	failAt    int
	sentCnt   int
	intercept func(id int64, raw json.RawMessage) ([]json.RawMessage, bool)
}

type serverPipe struct {
	pair *ctlPair
	in   chan json.RawMessage // single reader: Sessions.Serve via Read
	out  chan json.RawMessage
}

func newCtlPair() *ctlPair {
	toSrv := make(chan json.RawMessage, 64)
	fromSrv := make(chan json.RawMessage, 64)
	p := &ctlPair{done: make(chan struct{})}
	p.client = &clientPipe{
		pair:       p,
		toServer:   toSrv,
		fromServer: fromSrv,
		pathCh:     make(chan struct{}, 1),
		waiting:    map[int64]chan json.RawMessage{},
	}
	p.server = &serverPipe{pair: p, in: toSrv, out: fromSrv}
	return p
}

func (p *ctlPair) close() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *ctlPair) Done() <-chan struct{} { return p.done }

func (c *clientPipe) Path() string                 { return "direct" }
func (c *clientPipe) PathChanged() <-chan struct{} { return c.pathCh }
func (c *clientPipe) Done() <-chan struct{}        { return c.pair.done }
func (c *clientPipe) Close() error                 { return c.pair.close() }
func (c *clientPipe) Finish()                      { c.pair.close() }

func (c *clientPipe) Send(frame any) error {
	raw, err := encodeCtl(frame)
	if err != nil {
		return err
	}
	if err := c.checkSendFail(); err != nil {
		return err
	}
	if frames, ok := c.applyIntercept(raw); ok {
		for _, f := range frames {
			if err := sendOrDone(c.fromServer, c.pair.done, f); err != nil {
				return err
			}
		}
		return nil
	}
	return sendOrDone(c.toServer, c.pair.done, raw)
}

func (c *clientPipe) checkSendFail() error {
	c.mu.Lock()
	c.sentCnt++
	fail := c.failAt > 0 && c.sentCnt == c.failAt
	c.mu.Unlock()
	if !fail {
		return nil
	}
	for len(c.fromServer) > 0 {
		runtime.Gosched()
	}
	time.Sleep(time.Millisecond)
	return errors.New("client send failure")
}

func (c *clientPipe) applyIntercept(raw json.RawMessage) ([]json.RawMessage, bool) {
	c.mu.Lock()
	fn := c.intercept
	c.mu.Unlock()
	if fn == nil {
		return nil, false
	}
	var req struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(raw, &req)
	if req.ID == 0 {
		return nil, false
	}
	return fn(req.ID, raw)
}

func (c *clientPipe) Request(ctx context.Context, build func(id int64) any) (json.RawMessage, error) {
	id, reply := c.reserve()
	defer c.forget(id)
	if err := c.Send(build(id)); err != nil {
		return nil, err
	}
	return waitPipeReply(ctx, reply, c.pair.done)
}

func (c *clientPipe) reserve() (int64, chan json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	ch := make(chan json.RawMessage, 1) // single reader: Request owning c.next
	c.waiting[c.next] = ch
	return c.next, ch
}

func (c *clientPipe) forget(id int64) {
	c.mu.Lock()
	delete(c.waiting, id)
	c.mu.Unlock()
}

func (c *clientPipe) Read(ctx context.Context) (json.RawMessage, error) {
	for {
		msg, err := readPipeMsg(ctx, c.fromServer, c.pair.done)
		if err != nil {
			return nil, err
		}
		if c.deliverReply(msg) {
			continue
		}
		return msg, nil
	}
}

func (c *clientPipe) deliverReply(msg json.RawMessage) bool {
	var hdr struct {
		ID *int64 `json:"id,omitempty"`
		Re *int64 `json:"re,omitempty"`
	}
	if proto.Decode(msg, &hdr) != nil || hdr.Re == nil || hdr.ID != nil {
		return false
	}
	c.mu.Lock()
	ch := c.waiting[*hdr.Re]
	delete(c.waiting, *hdr.Re)
	c.mu.Unlock()
	if ch != nil {
		ch <- msg
	}
	return true
}

func (s *serverPipe) Path() string                 { return "direct" }
func (s *serverPipe) PathChanged() <-chan struct{} { return nil }
func (s *serverPipe) Done() <-chan struct{}        { return s.pair.done }
func (s *serverPipe) Close() error                 { return s.pair.close() }
func (s *serverPipe) Finish()                      { s.pair.close() }

func (s *serverPipe) Send(frame any) error {
	raw, err := encodeCtl(frame)
	if err != nil {
		return err
	}
	return sendOrDone(s.out, s.pair.done, raw)
}

func (s *serverPipe) Read(ctx context.Context) (json.RawMessage, error) {
	return readPipeMsg(ctx, s.in, s.pair.done)
}

func sendOrDone(ch chan<- json.RawMessage, done <-chan struct{}, raw json.RawMessage) error {
	select {
	case ch <- raw:
		return nil
	case <-done:
		return io.EOF
	}
}

func waitPipeReply(ctx context.Context, reply <-chan json.RawMessage, done <-chan struct{}) (json.RawMessage, error) {
	select {
	case msg := <-reply:
		return msg, nil
	case <-done:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func readPipeMsg(ctx context.Context, in <-chan json.RawMessage, done <-chan struct{}) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		return nil, io.EOF
	case msg := <-in:
		return msg, nil
	}
}

type clientNodeRig struct {
	key      identity.Key
	db       *store.Store
	logs     *Logs
	loop     *home.Loop
	host     *host.Host
	sessions *Sessions
}

func newClientNodeRig(t *testing.T, name string, key identity.Key, owner identity.Key, entries []proto.Envelope, cmds home.Table) *clientNodeRig {
	t.Helper()
	now := time.UnixMilli(1763600000000)
	db := sesOpenStore(t)
	logs, err := OpenLogs(db, key.ID())
	mustNoErr(t, err)
	cand, err := identity.Replay(owner.ID(), entries)
	mustNoErr(t, err)
	ok, err := logs.dvlAdopt(owner.ID(), cand, nil)
	if err != nil || !ok {
		t.Fatalf("adopt %s: %v, %v", name, ok, err)
	}
	loop, h := newTestHomeAndHost(t, db, key, logs, now, cmds)
	s := NewSessions(SessionConfig{
		ID: key.ID(), Version: "v0.3.1", Store: db, Logs: logs, Home: loop, Host: h,
		Name: func() string { return name }, Addrs: func() []string { return []string{"ws://" + name + "/rv"} },
		Now: func() time.Time { return now }, Report: func(error) {},
	})
	return &clientNodeRig{key: key, db: db, logs: logs, loop: loop, host: h, sessions: s}
}

func TestClientHandshakeDevlogBeforeAttachAndLiveForwarding(t *testing.T) {
	owner := sesKey("cli-owner")
	homeKey := sesKey("cli-home")
	hostKey := sesKey("cli-host")
	extraKey := sesKey("cli-extra")
	groupID := string(sesID("cli-grp-1"))
	nowMs := int64(1763600000000)

	e1 := sesAddEntry(owner, owner.ID(), 1, owner.ID(), "browser", "owner")
	e2 := sesAddEntry(owner, owner.ID(), 2, homeKey.ID(), "node", "desktop")
	e3 := sesAddEntry(owner, owner.ID(), 3, hostKey.ID(), "node", "laptop")
	e4 := sesAddEntry(owner, owner.ID(), 4, extraKey.ID(), "browser", "tablet")

	cmds := home.Table{
		"ping": func(_ *home.State, _ home.Command) (home.Result, *home.Refusal) {
			return home.Result{Answer: store.Outcome{Num: 42}}, nil
		},
		"group/create": testGroupCreate,
	}
	homeNode := newClientNodeRig(t, "desktop", homeKey, owner, []proto.Envelope{e1, e2}, cmds)
	hostNode := newClientNodeRig(t, "laptop", hostKey, owner, []proto.Envelope{e1, e2, e3}, nil)

	homeLocal := AttachLocal(homeNode.host, homeNode.loop)
	t.Cleanup(func() { homeLocal.Close() })
	createPayload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q,"name":"Main"}`, sesID("cli-gc"), nowMs, groupID)
	_, err := homeNode.host.Command(t.Context(), homeKey.ID(), createPayload)
	mustNoErr(t, err)

	joinDone := installAttachPing(t, hostNode, homeKey.ID(), groupID, nowMs)
	pair := startPairedSession(t, homeNode, hostNode)

	mustNoErr(t, recvFrame(t, joinDone))
	if !homeNode.logs.Known(hostKey.ID()) {
		t.Fatal("home did not learn host device from client devlog")
	}
	checkLiveDevlogBothWays(t, owner, homeNode, hostNode, e4)
	pair.close()
}

func installAttachPing(t *testing.T, hostNode *clientNodeRig, homeID, groupID string, nowMs int64) <-chan error {
	t.Helper()
	prev := maps.Clone(host.Attached)
	t.Cleanup(func() { host.Attached = prev })
	done := make(chan error, 1) // single reader: TestClientHandshakeDevlogBeforeAttachAndLiveForwarding
	host.Attached["join_probe"] = func(h *host.Host, dev string) {
		if dev != homeID {
			return
		}
		go func() {
			p := fmt.Sprintf(`{"t":"ping","n":%q,"ts":%d,"g":%q}`, sesID("cli-ping"), nowMs, groupID)
			_, err := h.Command(context.Background(), dev, p)
			done <- err
		}()
	}
	return done
}

func startPairedSession(t *testing.T, homeNode, hostNode *clientNodeRig) *ctlPair {
	t.Helper()
	pair := newCtlPair()
	ctx, cancel := context.WithCancel(t.Context())
	srvDone := make(chan struct{}) // single reader: cleanup
	cliDone := make(chan struct{}) // single reader: cleanup
	go func() {
		defer close(srvDone)
		homeNode.sessions.Serve(ctx, hostNode.key.ID(), false, pair.server)
	}()
	go func() {
		defer close(cliDone)
		hostNode.sessions.serveClient(ctx, homeNode.key.ID(), pair.client)
	}()
	t.Cleanup(func() {
		cancel()
		pair.close()
		<-srvDone
		<-cliDone
	})
	return pair
}

func checkLiveDevlogBothWays(t *testing.T, owner identity.Key, homeNode, hostNode *clientNodeRig, e4 proto.Envelope) {
	t.Helper()
	browser := serveConn(t, hostNode.sessions, owner.ID(), false)
	browser.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "b", Addrs: []string{}})
	browser.next(t)
	browser.next(t)

	// Home learns v4 -> forwards over client ctl to hostNode, which forwards to its inbound browser.
	mustNoErr(t, homeNode.logs.dvlReceive(proto.Devlog{Type: "devlog", User: owner.ID(), Entries: []proto.Envelope{e4}}, nil))
	var fwd proto.Devlog
	mustNoErr(t, proto.Decode(browser.next(t), &fwd))
	if len(fwd.Entries) != 1 || !hostNode.logs.Known(dvlRowOf(e4).Device) {
		t.Fatalf("host did not apply and forward home devlog: %+v", fwd)
	}

	// Browser pushes v5 to hostNode -> hostNode forwards over outgoing client ctl to homeNode.
	d5 := sesKey("cli-d5")
	e5 := sesAddEntry(owner, owner.ID(), 5, d5.ID(), "browser", "watch")
	browser.push(proto.Devlog{Type: "devlog", User: owner.ID(), Entries: []proto.Envelope{e5}})
	waitKnown(t, homeNode.logs, d5.ID())
}

func waitKnown(t *testing.T, logs *Logs, dev string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if logs.Known(dev) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("device %s never became known", dev)
}

func TestClientSynchronousPushOrderAndFraming(t *testing.T) {
	owner := sesKey("sync-owner")
	homeKey := sesKey("sync-home")
	hostKey := sesKey("sync-host")
	groupID := string(sesID("sync-grp"))
	entries := []proto.Envelope{
		sesAddEntry(owner, owner.ID(), 1, owner.ID(), "browser", "owner"),
		sesAddEntry(owner, owner.ID(), 2, homeKey.ID(), "node", "home"),
		sesAddEntry(owner, owner.ID(), 3, hostKey.ID(), "node", "host"),
	}
	homeNode := newClientNodeRig(t, "home", homeKey, owner, entries, nil)
	hostNode := newClientNodeRig(t, "host", hostKey, owner, entries, nil)

	var pushOrder []string
	var pushMu sync.Mutex
	savePushes(t)
	host.Pushes["rows"] = func(h *host.Host, from string, frame json.RawMessage) {
		var rf proto.Rows
		if proto.Decode(frame, &rf) != nil || from != homeKey.ID() {
			return
		}
		for _, r := range rf.Rows {
			h.Rows.Put(rf.Group, "group", rf.Group, r)
		}
		pushMu.Lock()
		pushOrder = append(pushOrder, "rows")
		pushMu.Unlock()
	}
	host.Pushes["inbox"] = func(_ *host.Host, _ string, _ json.RawMessage) {
		pushMu.Lock()
		pushOrder = append(pushOrder, "inbox")
		pushMu.Unlock()
	}

	registerFakeSyncOnHome(homeNode.sessions, groupID)
	attached := waitAttachedHome(t, hostNode.host, homeKey.ID())
	pair := startPairedSession(t, homeNode, hostNode)
	remoteHome := recvFrame(t, attached)

	checkSyncRowsBeforeAnswer(t, hostNode.host, remoteHome, groupID)
	pair.close()

	pushMu.Lock()
	gotOrder := slices.Clone(pushOrder)
	pushMu.Unlock()
	mustEqual(t, gotOrder, []string{"rows", "inbox"})

	checkPreAttachPushHeldUntilAttachReturns(t, hostNode, homeKey.ID(), owner)
	checkClientInboundFraming(t, hostNode, homeKey.ID(), owner)
}

func savePushes(t *testing.T) {
	t.Helper()
	prev := maps.Clone(host.Pushes)
	t.Cleanup(func() { host.Pushes = prev })
}

func registerFakeSyncOnHome(s *Sessions, groupID string) {
	s.Requests["sync"] = RequestHandler{
		Type: "synced",
		Run: func(_ context.Context, _ *home.Conn, from Ctl, _ string, _ int64, _ json.RawMessage) (any, error) {
			row := json.RawMessage(fmt.Sprintf(`{"r":"group","id":%q,"seq":1,"name":"SyncGrp"}`, groupID))
			from.Send(proto.Rows{Type: "rows", Group: groupID, Rows: []json.RawMessage{row}})
			from.Send(map[string]any{"t": "inbox", "g": groupID, "items": []any{}})
			return map[string]any{"g": groupID, "seq": 1}, nil
		},
	}
}

func waitAttachedHome(t *testing.T, h *host.Host, wantDev string) <-chan host.Home {
	t.Helper()
	prev := maps.Clone(host.Attached)
	t.Cleanup(func() { host.Attached = prev })
	ch := make(chan host.Home, 4) // single reader: caller awaiting Attach
	host.Attached["wait_test"] = func(got *host.Host, dev string) {
		if dev == wantDev {
			if hm, ok := got.Home(dev); ok {
				ch <- hm
			}
		}
	}
	return ch
}

func checkSyncRowsBeforeAnswer(t *testing.T, h *host.Host, remoteHome host.Home, groupID string) {
	t.Helper()
	build := func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: groupID, Since: 0, StateOnly: true}
	}
	ans, err := remoteHome.Request(t.Context(), build)
	mustNoErr(t, err)
	var synced proto.Synced
	mustNoErr(t, proto.Decode(ans, &synced))
	if _, ok := h.Rows.Get(groupID, "group", groupID); !ok {
		t.Fatal("group row was not in Host.Rows when sync Request returned")
	}
}

func checkPreAttachPushHeldUntilAttachReturns(t *testing.T, hostNode *clientNodeRig, homeID string, owner identity.Key) {
	t.Helper()
	dPre := sesKey("sync-pre-dev")
	ePre := sesAddEntry(owner, owner.ID(), 4, dPre.ID(), "browser", "pre-attach")
	dlRaw, err := encodeCtl(proto.Devlog{Type: "devlog", User: owner.ID(), Entries: []proto.Envelope{ePre}})
	mustNoErr(t, err)

	var events []string
	var evMu sync.Mutex
	pushed := make(chan struct{}, 1) // single reader: checkPreAttachPushHeldUntilAttachReturns
	prevAtt := maps.Clone(host.Attached)
	defer func() { host.Attached = prevAtt }()
	host.Attached["order_check"] = func(_ *host.Host, dev string) {
		if dev != homeID {
			return
		}
		waitKnown(t, hostNode.logs, dPre.ID())
		time.Sleep(5 * time.Millisecond)
		evMu.Lock()
		events = append(events, "attached")
		evMu.Unlock()
	}
	host.Pushes["inbox"] = func(_ *host.Host, _ string, _ json.RawMessage) {
		evMu.Lock()
		events = append(events, "inbox")
		evMu.Unlock()
		pushed <- struct{}{}
	}

	pair := newCtlPair()
	pair.client.intercept = func(id int64, _ json.RawMessage) ([]json.RawMessage, bool) {
		w := fmt.Sprintf(`{"t":"welcome","re":%d,"ok":true,"v":1,"device":%q,"name":"h","version":"v1","time":1,"addrs":[],"groups":[],"owner":true}`, id, homeID)
		return []json.RawMessage{dlRaw, json.RawMessage(w), json.RawMessage(`{"t":"inbox","g":"g1","items":[]}`)}, true
	}
	done := make(chan struct{}) // single reader: checkPreAttachPushHeldUntilAttachReturns
	go func() {
		defer close(done)
		hostNode.sessions.serveClient(t.Context(), homeID, pair.client)
	}()
	recvFrame(t, pushed)
	pair.close()
	<-done

	evMu.Lock()
	got := slices.Clone(events)
	evMu.Unlock()
	mustEqual(t, got, []string{"attached", "inbox"})
}

func checkClientInboundFraming(t *testing.T, hostNode *clientNodeRig, homeID string, owner identity.Key) {
	t.Helper()
	var dbErrs atomic.Int32
	hostNode.sessions.cfg.Report = func(error) { dbErrs.Add(1) }
	host.Pushes[""] = func(*host.Host, string, json.RawMessage) {
		t.Error("malformed frame reached Host.Push")
	}

	var gotHello proto.Hello
	pair := newCtlPair()
	pair.client.intercept = func(id int64, raw json.RawMessage) ([]json.RawMessage, bool) {
		var req struct {
			Type string `json:"t"`
		}
		json.Unmarshal(raw, &req)
		if req.Type != "hello" {
			return nil, false
		}
		proto.Decode(raw, &gotHello)
		w := fmt.Sprintf(`{"t":"welcome","re":%d,"ok":true,"v":1,"device":%q,"name":"h","version":"v1","time":1,"addrs":[],"groups":[],"owner":true}`, id, homeID)
		return []json.RawMessage{json.RawMessage(w)}, true
	}
	done := make(chan struct{}) // single reader: checkClientInboundFraming cleanup
	go func() {
		defer close(done)
		hostNode.sessions.serveClient(t.Context(), homeID, pair.client)
	}()
	defer func() {
		pair.close()
		<-done
	}()

	checkInboundRefusalsAndDevlogErr(t, pair, hostNode, owner, &dbErrs)
	mustEqual(t, gotHello.Kind, "host")
	mustEqual(t, gotHello.Name, "host")
	mustEqual(t, gotHello.Addrs, []string{"ws://host/rv"})
}

func checkInboundRefusalsAndDevlogErr(t *testing.T, pair *ctlPair, hostNode *clientNodeRig, owner identity.Key, dbErrs *atomic.Int32) {
	t.Helper()
	// Drain initial devlog frame sent by serveClient after welcome.
	recvFrame(t, pair.server.in)

	// Malformed JSON, missing t, stray re with id -> refused or dropped on client ctl without closing.
	for _, raw := range []string{`[]`, `{}`, `{"t":"x","id":70,"re":1}`} {
		mustNoErr(t, sendOrDone(pair.client.fromServer, pair.done, json.RawMessage(raw)))
	}
	expectErr(t, recvFrame(t, pair.server.in), 70, "bad_request")
	mustNoErr(t, sendOrDone(pair.client.fromServer, pair.done, json.RawMessage(`{"id":71}`)))
	expectErr(t, recvFrame(t, pair.server.in), 71, "bad_request")

	// Unknown frame types (even with id) go to Host.Push and are ignored without sending an error.
	mustNoErr(t, sendOrDone(pair.client.fromServer, pair.done, json.RawMessage(`{"t":"sync","id":72}`)))

	// Invalid devlog push is ignored; devlog DB failure is reported via Report.
	mustNoErr(t, sendOrDone(pair.client.fromServer, pair.done, json.RawMessage(`{"t":"devlog"}`)))
	drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
	mustNoErr(t, hostNode.db.Write(drop))
	d6 := sesKey("sync-d6")
	e6 := sesAddEntry(owner, owner.ID(), 5, d6.ID(), "browser", "bad-db")
	dlRaw, err := encodeCtl(proto.Devlog{Type: "devlog", User: owner.ID(), Entries: []proto.Envelope{e6}})
	mustNoErr(t, err)
	mustNoErr(t, sendOrDone(pair.client.fromServer, pair.done, dlRaw))

	// Send one more refused frame to synchronize after the devlog frame on the read loop.
	mustNoErr(t, sendOrDone(pair.client.fromServer, pair.done, json.RawMessage(`{"id":73}`)))
	expectErr(t, recvFrame(t, pair.server.in), 73, "bad_request")
	if dbErrs.Load() != 1 {
		t.Fatalf("expected 1 reported db error, got %d", dbErrs.Load())
	}
}

func TestConnectHomeSelfIgnoreReconnectAndMemberStatus(t *testing.T) {
	owner := sesKey("rc-owner")
	homeKey := sesKey("rc-home")
	hostKey := sesKey("rc-host")
	groupID := string(sesID("rc-grp"))
	aiID := string(sesID("rc-ai"))
	entries := []proto.Envelope{
		sesAddEntry(owner, owner.ID(), 1, owner.ID(), "browser", "owner"),
		sesAddEntry(owner, owner.ID(), 2, homeKey.ID(), "node", "home"),
		sesAddEntry(owner, owner.ID(), 3, hostKey.ID(), "node", "host"),
	}

	statuses := make(chan map[string]any, 8) // single reader: TestConnectHomeSelfIgnoreReconnectAndMemberStatus
	seedCmd := func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "RC", Checks: []string{}, Created: c.Now}
		own := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m-own"}, Group: s.ID, Kind: "human", Name: "O", User: s.Owner}, Role: "admin"}
		ai := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: aiID}, Group: s.ID, Kind: "ai", Name: "Claude"}, CLI: "claude", Host: hostKey.ID(), Allow: []string{}, Approvers: []string{s.Owner}, State: "offline"}
		return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(ai)}, GroupsChanged: true}, nil
	}
	cmds := home.Table{
		"group/create": seedCmd,
		"member/status": func(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
			var fields map[string]any
			json.Unmarshal([]byte(c.Env.Payload), &fields)
			statuses <- fields
			return home.Result{}, nil
		},
	}
	homeNode := newClientNodeRig(t, "home", homeKey, owner, entries, cmds)
	hostNode := newClientNodeRig(t, "host", hostKey, owner, entries, nil)
	seedGroupWithHostedAI(t, homeNode, groupID)

	// Create two sessions for the same AI on homeKey and one on another home: only one member status per connect.
	mustNoErr(t, hostNode.host.Sessions.Create(host.Session{
		SessionKey: host.SessionKey{Group: groupID, Member: aiID},
		Kind:       host.GroupSession, Folder: "repo", Agent: "claude", Home: homeKey.ID(), Name: "Claude",
	}))
	mustNoErr(t, hostNode.host.Sessions.Create(host.Session{
		SessionKey: host.SessionKey{Group: groupID, Member: aiID, Task: string(sesID("rc-task"))},
		Kind:       host.TaskSession, Folder: "wt/1-t", Agent: "claude", Home: homeKey.ID(), Name: "Claude",
	}))
	mustNoErr(t, hostNode.host.Sessions.Create(host.Session{
		SessionKey: host.SessionKey{Group: groupID, Member: string(sesID("rc-other"))},
		Kind:       host.GroupSession, Folder: "repo", Agent: "claude", Home: "other-home", Name: "Other",
	}))

	verifyConnectHomeAndReconnect(t, homeNode, hostNode, groupID, aiID, statuses)
}

func seedGroupWithHostedAI(t *testing.T, homeNode *clientNodeRig, groupID string) {
	t.Helper()
	nowMs := int64(1763600000000)
	local := AttachLocal(homeNode.host, homeNode.loop)
	defer local.Close()
	payload := fmt.Sprintf(`{"t":"group","op":"create","n":%q,"ts":%d,"g":%q}`, sesID("rc-seed"), nowMs, groupID)
	_, err := homeNode.host.Command(t.Context(), homeNode.key.ID(), payload)
	mustNoErr(t, err)
}

func verifyConnectHomeAndReconnect(t *testing.T, homeNode, hostNode *clientNodeRig, groupID, aiID string, statuses <-chan map[string]any) {
	t.Helper()
	var dialCalls atomic.Int32
	nextConn := make(chan *ctlPair, 2) // single reader: dial loop
	dial := func(ctx context.Context, device string, session func(clientCtl)) {
		dialCalls.Add(1)
		runTestDialLoop(ctx, homeNode, hostNode.key.ID(), device, nextConn, session)
	}
	connectFn := hostNode.sessions.connectHome(dial)

	// Self device is ignored synchronously without invoking dial.
	connectFn(t.Context(), hostNode.key.ID())
	mustEqual(t, dialCalls.Load(), int32(0))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	connectFn(ctx, homeNode.key.ID())

	// Connection 1 opens -> member status sent with state dormant and path direct.
	p1 := newCtlPair()
	nextConn <- p1
	assertMemberStatus(t, recvFrame(t, statuses), aiID, groupID, "dormant", "direct")

	// Drop connection 1 -> reconnects on connection 2 and sends member status again.
	p1.close()
	p2 := newCtlPair()
	nextConn <- p2
	assertMemberStatus(t, recvFrame(t, statuses), aiID, groupID, "dormant", "direct")
	expectNoExtraStatus(t, statuses)

	// Canceling ctx closes the active connection.
	cancel()
	<-p2.Done()
	mustEqual(t, dialCalls.Load(), int32(1))
}

func assertMemberStatus(t *testing.T, st map[string]any, wantID, wantGroup, wantState, wantPath string) {
	t.Helper()
	mustEqual(t, st["id"], wantID)
	mustEqual(t, st["g"], wantGroup)
	mustEqual(t, st["state"], wantState)
	mustEqual(t, st["path"], wantPath)
}

func runTestDialLoop(ctx context.Context, homeNode *clientNodeRig, hostID, wantHomeID string, nextConn <-chan *ctlPair, session func(clientCtl)) {
	if wantHomeID != homeNode.key.ID() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-nextConn:
			srvDone := make(chan struct{}) // single reader: runTestDialLoop
			go func() {
				defer close(srvDone)
				homeNode.sessions.Serve(ctx, hostID, false, p.server)
			}()
			session(p.client)
			<-srvDone
		}
	}
}

func expectNoExtraStatus(t *testing.T, statuses <-chan map[string]any) {
	t.Helper()
	select {
	case extra := <-statuses:
		t.Fatalf("unexpected extra member status: %+v", extra)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestClientHandshakeErrorsAndTimeout(t *testing.T) {
	owner := sesKey("err-owner")
	homeKey := sesKey("err-home")
	hostKey := sesKey("err-host")
	entries := []proto.Envelope{
		sesAddEntry(owner, owner.ID(), 1, owner.ID(), "browser", "owner"),
		sesAddEntry(owner, owner.ID(), 2, homeKey.ID(), "node", "home"),
		sesAddEntry(owner, owner.ID(), 3, hostKey.ID(), "node", "host"),
	}
	hostNode := newClientNodeRig(t, "host", hostKey, owner, entries, nil)

	checkWelcomeErrorResponseCase(t, hostNode, homeKey.ID())
	checkDevlogSendErrorCase(t, hostNode, homeKey.ID())
	checkHelloTimeoutInBubble(t, hostNode, homeKey.ID())
}

func checkWelcomeErrorResponseCase(t *testing.T, hostNode *clientNodeRig, homeID string) {
	t.Helper()
	p := newCtlPair()
	var versions []int
	p.client.intercept = func(id int64, raw json.RawMessage) ([]json.RawMessage, bool) {
		var h proto.Hello
		mustNoErr(t, proto.Decode(raw, &h))
		versions = append(versions, h.V)
		reply := fmt.Sprintf(`{"re":%d,"err":{"code":"version","msg":"unsupported"}}`, id)
		return []json.RawMessage{json.RawMessage(reply)}, true
	}
	hostNode.sessions.serveClient(t.Context(), homeID, p.client)
	if !slices.Equal(versions, []int{2, 1}) {
		t.Fatalf("attempted hello versions = %v, want [2 1]", versions)
	}
	if _, ok := hostNode.host.Home(homeID); ok {
		t.Fatal("refused handshake attached home")
	}
	if len(p.server.in) != 0 {
		t.Fatal("refused handshake sent devlog")
	}
	<-p.Done()
}

func checkDevlogSendErrorCase(t *testing.T, hostNode *clientNodeRig, homeID string) {
	t.Helper()
	savePushes(t)
	host.Pushes["inbox"] = func(*host.Host, string, json.RawMessage) {
		t.Error("push reached Host.Push when handshake failed before Attach")
	}
	p := newCtlPair()
	p.client.failAt = 2
	p.client.intercept = func(id int64, _ json.RawMessage) ([]json.RawMessage, bool) {
		w := fmt.Sprintf(`{"t":"welcome","re":%d,"ok":true,"v":1,"device":"d","name":"n","version":"v1","time":1,"addrs":[],"groups":[],"owner":true}`, id)
		return []json.RawMessage{json.RawMessage(w), json.RawMessage(`{"t":"inbox","g":"g1","items":[]}`)}, true
	}
	hostNode.sessions.serveClient(t.Context(), homeID, p.client)
	if _, ok := hostNode.host.Home(homeID); ok {
		t.Fatal("failed devlog send attached home")
	}
	<-p.Done()
}

func checkHelloTimeoutInBubble(t *testing.T, hostNode *clientNodeRig, homeID string) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		p := newCtlPair()
		done := make(chan struct{}) // single reader: checkHelloTimeoutInBubble
		start := time.Now()
		go func() {
			defer close(done)
			hostNode.sessions.serveClient(t.Context(), homeID, p.client)
		}()
		<-done
		if time.Since(start) != cliHelloTimeout {
			t.Fatalf("got elapsed=%v, want %v", time.Since(start), cliHelloTimeout)
		}
		if _, ok := hostNode.host.Home(homeID); ok {
			t.Fatal("timed-out handshake attached home")
		}
		<-p.Done()
	})
}
