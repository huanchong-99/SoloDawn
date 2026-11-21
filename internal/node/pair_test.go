package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
)

type pairClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []chan time.Time
}

func newPairClock(t0 time.Time) *pairClock {
	return &pairClock{now: t0}
}

func (c *pairClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *pairClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

func (c *pairClock) After(_ time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1) // single reader: waitPair or watchBrowser
	c.mu.Lock()
	c.waiters = append(c.waiters, ch)
	c.mu.Unlock()
	return ch
}

func (c *pairClock) WaitWaiters(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.waiters)
		c.mu.Unlock()
		if n >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d clock waiters", want)
}

func (c *pairClock) FireAll() {
	c.mu.Lock()
	now := c.now
	list := c.waiters
	c.waiters = nil
	c.mu.Unlock()
	for _, ch := range list {
		ch <- now
	}
}

func pairTestRelay(t *testing.T) proto.ICEServer {
	t.Helper()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	lo := net.IPv4(127, 0, 0, 1)
	gen := &turn.RelayAddressGeneratorStatic{RelayAddress: lo, Address: "127.0.0.1"}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "test",
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			return "peer", turn.GenerateAuthKey("peer", "test", "pw"), r.Username == "peer"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: l, RelayAddressGenerator: gen}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return proto.ICEServer{URLs: []string{"turn:" + l.LocalAddr().String() + "?transport=udp"}, Username: "peer", Credential: "pw"}
}

func relayRoute(r peer.Route, ice proto.ICEServer) peer.Route {
	orig := r.Send
	r.ICE = []proto.ICEServer{ice}
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		var sig struct {
			Type string `json:"t"`
			Cand string `json:"cand"`
		}
		if json.Unmarshal([]byte(env.Payload), &sig) == nil && sig.Type == "ice" && !strings.Contains(sig.Cand, " typ relay") {
			return nil
		}
		return orig(ctx, to, env)
	}
	return r
}

type pairHarness struct {
	rvURL      string
	ice        proto.ICEServer
	db         *store.Store
	clock      *pairClock
	nodeKey    identity.Key
	logs       *Logs
	sessions   *Sessions
	surface    *Surface
	pairer     *Pairer
	nodePeer   *peer.Peer
	sharedPath *peer.Path
	reported   chan error // single reader: test assertion when checking reported errors
}

func newPairHarness(t *testing.T, ownedBy *identity.Key, withSharedPath bool) *pairHarness {
	t.Helper()
	ice := pairTestRelay(t)
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	t.Cleanup(srv.Close)
	rvURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"

	db := sesOpenStore(t)
	nodeKey := sesKey("pair-node")
	var logs *Logs
	if ownedBy != nil {
		logs = seedOwnedLogs(t, db, nodeKey, *ownedBy)
	} else {
		var err error
		logs, err = OpenLogs(db, nodeKey.ID())
		if err != nil {
			t.Fatal(err)
		}
	}

	clk := newPairClock(time.UnixMilli(1763600000000))
	loop, h := newTestHomeAndHost(t, db, nodeKey, logs, clk.Now(), nil)
	reported := make(chan error, 8) // single reader: test assertion
	sessions := NewSessions(SessionConfig{
		ID: nodeKey.ID(), Version: "v0.3.1", Store: db, Logs: logs, Home: loop, Host: h,
		Name:   func() string { return ReadName(db) },
		Addrs:  func() []string { return []string{rvURL} },
		Now:    clk.Now,
		Report: func(err error) { reported <- err },
	})
	surface := NewSurface(SurfaceConfig{
		ID: nodeKey.ID(), OS: "darwin/arm64", Version: "v0.3.1",
		Store: db, Sessions: sessions, Host: h,
		Agents: func(context.Context) ([]proto.HostAgent, error) { return []proto.HostAgent{}, nil },
	})
	putRV := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('rendezvous', ?)", string(dvlMustEncode([]string{rvURL})))
		return err
	}
	if err := db.Write(putRV); err != nil {
		t.Fatal(err)
	}

	var sharedPath *peer.Path
	pairer := NewPairer(PairConfig{
		Key: nodeKey, Store: db, Logs: logs, Sessions: sessions,
		Paths: func() []*peer.Path {
			if sharedPath != nil {
				return []*peer.Path{sharedPath}
			}
			return nil
		},
		Now: clk.Now, After: clk.After,
	})
	t.Cleanup(pairer.Close)

	socks, err := peer.Listen(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socks.Close() })
	known := func(dev string) bool { return logs.Known(dev) || pairer.Known(dev) }
	paths := func(dev string) []peer.Route {
		raw := pairer.Routes(dev)
		out := make([]peer.Route, 0, len(raw)+1)
		if sharedPath != nil {
			out = append(out, relayRoute(sharedPath.Route(), ice))
		}
		for _, r := range raw {
			out = append(out, relayRoute(r, ice))
		}
		return out
	}
	inbound := func(c *peer.Connection) { go pairer.Serve(t.Context(), c.Device, c) }
	nodePeer := peer.New(nodeKey, socks, db, known, paths, inbound, nil)
	pairer.Peer = nodePeer

	if withSharedPath {
		sharedPath, err = peer.Dial(t.Context(), rvURL, nodeKey)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { sharedPath.Close() })
		event := func(kind string, data []byte) error {
			pairer.HandleEvent(t.Context(), sharedPath, kind, data)
			return nil
		}
		go sharedPath.Serve(t.Context(), nodePeer, event)
	}

	return &pairHarness{
		rvURL: rvURL, ice: ice, db: db, clock: clk, nodeKey: nodeKey,
		logs: logs, sessions: sessions, surface: surface, pairer: pairer,
		nodePeer: nodePeer, sharedPath: sharedPath, reported: reported,
	}
}

type simBrowser struct {
	key  identity.Key
	path *peer.Path
	peer *peer.Peer
}

func dialSimBrowser(t *testing.T, ph *pairHarness, key identity.Key) *simBrowser {
	t.Helper()
	path, err := peer.Dial(t.Context(), ph.rvURL, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { path.Close() })

	socks, err := peer.Listen(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	db := sesOpenStore(t)
	t.Cleanup(func() { socks.Close() })

	routes := func(string) []peer.Route { return []peer.Route{relayRoute(path.Route(), ph.ice)} }
	bp := peer.New(key, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	return &simBrowser{key: key, path: path, peer: bp}
}

func exchangeBrowserRoom(t *testing.T, ctx context.Context, b *simBrowser, secret identity.Secret, myRole, wantRole string) proto.Pair {
	t.Helper()
	room := secret.Room()
	self := proto.Pair{Type: "pair", Key: b.key.ID(), Kind: "browser", Name: "Phone Browser", Role: myRole}
	self.MAC = secret.MAC(self)
	rawSelf := json.RawMessage(dvlMustEncode(self))

	gotPeer := make(chan proto.Pair, 1) // single reader: exchangeBrowserRoom
	var sentOnce atomic.Bool
	event := func(kind string, data []byte) error {
		if kind == "joined" {
			b.path.Write(ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: rawSelf})
		}
		if kind == "room" {
			var rd proto.RoomData
			var m proto.Pair
			if proto.Decode(data, &rd) == nil && rd.Room == room && proto.Decode(rd.Data, &m) == nil && secret.Verify(m, wantRole) && sentOnce.CompareAndSwap(false, true) {
				gotPeer <- m
			}
		}
		return nil
	}
	go b.path.Serve(ctx, b.peer, event)
	if err := b.path.Write(ctx, proto.RoomID{Type: "join", Room: room}); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-gotPeer:
		return m
	case <-ctx.Done():
		t.Fatalf("timed out waiting for room peer: %v", ctx.Err())
		return proto.Pair{}
	}
}

func waitKnown(t *testing.T, p *Pairer, device string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p.Known(device) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for Pairer.Known(%q)", device)
}

func TestPairRoleBEndToEndAndBootstrap(t *testing.T) {
	ph := newPairHarness(t, nil, false)
	bKey := sesKey("first-browser")
	otherKey := sesKey("stranger")
	b := dialSimBrowser(t, ph, bKey)

	secret := identity.NewSecret()
	badURL := "ws://127.0.0.1:1/rv"

	type out struct {
		res PairResult
		err error
	}
	done := make(chan out, 1) // single reader: TestPairRoleBEndToEndAndBootstrap
	go func() {
		r, err := ph.pairer.Pair(t.Context(), PairRequest{
			Secret:     strings.ToLower(secret.String()),
			Name:       "workstation",
			Rendezvous: []string{badURL, ph.rvURL},
		})
		done <- out{r, err}
	}()

	nodeMsg := exchangeBrowserRoom(t, t.Context(), b, secret, "a", "b")
	if nodeMsg.Key != ph.nodeKey.ID() || nodeMsg.Name != "workstation" || nodeMsg.Kind != "node" {
		t.Fatalf("unexpected node pair message: %+v", nodeMsg)
	}
	waitKnown(t, ph.pairer, bKey.ID())
	if ph.pairer.Known(otherKey.ID()) || len(ph.pairer.Routes(bKey.ID())) != 1 || ph.pairer.Routes(otherKey.ID()) != nil {
		t.Fatal("unexpected Known/Routes state after room exchange")
	}

	conn, err := b.peer.Offer(t.Context(), ph.nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	select {
	case <-conn.Ready():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for ctl data channel")
	}

	// Close the browser's room socket after ctl opened to verify closed paths are ignored once ctl is open.
	b.path.Close()

	helloReq := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Phone Browser", Addrs: []string{}}
	}
	rawW1, err := conn.Request(t.Context(), helloReq)
	if err != nil {
		t.Fatal(err)
	}
	var w1 proto.Welcome
	if err := proto.Decode(rawW1, &w1); err != nil || w1.Owner || w1.Name != "" {
		t.Fatalf("first welcome = %+v (%v)", w1, err)
	}

	// Before adoption, host.info is forbidden.
	rawInfoErr, err := conn.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		t.Fatal(err)
	}
	expectErr(t, rawInfoErr, 2, "forbidden")

	// A log whose add entry for the node was signed by another device is not adopted.
	w1e1 := sesAddEntry(bKey, bKey.ID(), 1, bKey.ID(), "browser", "Phone Browser")
	w1e2 := sesAddEntry(bKey, bKey.ID(), 2, otherKey.ID(), "browser", "other")
	w1e3 := sesAddEntry(otherKey, bKey.ID(), 3, ph.nodeKey.ID(), "node", "workstation")
	if err := conn.Send(proto.Devlog{Type: "devlog", User: bKey.ID(), Entries: []proto.Envelope{w1e1, w1e2, w1e3}}); err != nil {
		t.Fatal(err)
	}

	// Send valid adoption log signed by the pairing browser.
	e1 := sesAddEntry(bKey, bKey.ID(), 1, bKey.ID(), "browser", "Phone Browser")
	e2 := sesAddEntry(bKey, bKey.ID(), 2, ph.nodeKey.ID(), "node", "workstation")
	if err := conn.Send(proto.Devlog{Type: "devlog", User: bKey.ID(), Entries: []proto.Envelope{e1, e2}}); err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("Pair returned error: %v", got.err)
	}
	if got.res.Device != bKey.ID() || got.res.Name != "workstation" || got.res.Fingerprint != ph.nodeKey.Fingerprint() || got.res.Path == "" {
		t.Fatalf("PairResult = %+v", got.res)
	}
	// Secret was discarded on completion; logs now hold the browser permanently.
	if ph.pairer.Known(bKey.ID()) || !ph.logs.Known(bKey.ID()) {
		t.Fatal("pairing secret was not discarded or log did not enroll browser")
	}

	// Second hello reports owner true and host.info succeeds on the same open ctl connection.
	rawW2, err := conn.Request(t.Context(), helloReq)
	if err != nil {
		t.Fatal(err)
	}
	var w2 proto.Welcome
	if err := proto.Decode(rawW2, &w2); err != nil || !w2.Owner || w2.Name != "workstation" {
		t.Fatalf("second welcome = %+v (%v)", w2, err)
	}
	rawInfo, err := conn.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		t.Fatal(err)
	}
	var info proto.HostInfo
	if err := proto.Decode(rawInfo, &info); err != nil || info.Device != ph.nodeKey.ID() || info.Name != "workstation" {
		t.Fatalf("host.info = %+v (%v)", info, err)
	}
}

func TestPairBrowserRoleAAndNonceReplay(t *testing.T) {
	ownerKey := sesKey("owner-b1")
	ph := newPairHarness(t, &ownerKey, true)

	// Connect existing owner browser over fakeCtl to observe forwarded devlog and send pair.browser.
	b1Ctl := serveConn(t, ph.sessions, ownerKey.ID(), false)
	b1Ctl.push(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "browser", Name: "Desktop", Addrs: []string{}})
	b1Ctl.next(t) // welcome
	b1Ctl.next(t) // initial devlog

	nonce1 := sesID("pb-nonce-1")
	pbPayload := fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, nonce1, ph.clock.Now().UnixMilli(), ph.nodeKey.ID())
	pbEnv := ownerKey.Sign(pbPayload)
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 2}, Env: pbEnv})

	var link1 proto.CmdResult
	if err := proto.Decode(b1Ctl.next(t), &link1); err != nil || link1.Re != 2 || link1.Expires != ph.clock.Now().Add(pairWindow).UnixMilli() {
		t.Fatalf("pair.browser answer = %+v (%v)", link1, err)
	}
	_, code, found := strings.Cut(link1.Link, "/#pair=")
	if !found {
		t.Fatalf("unexpected link %q", link1.Link)
	}
	secret, err := identity.ParseSecret(code)
	if err != nil {
		t.Fatal(err)
	}

	// Replay at expires - 1ms returns the exact same link and expires under the new request id.
	ph.clock.Set(time.UnixMilli(link1.Expires - 1))
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 3}, Env: pbEnv})
	var replayOK proto.CmdResult
	if err := proto.Decode(b1Ctl.next(t), &replayOK); err != nil || replayOK.Re != 3 || replayOK.Link != link1.Link || replayOK.Expires != link1.Expires {
		t.Fatalf("replay while valid = %+v (%v)", replayOK, err)
	}
	ph.clock.Set(time.UnixMilli(1763600000000))

	// New browser joins the room in role "b", opens ctl, receives node-signed devlog, and becomes owner.
	b2Key := sesKey("phone-b2")
	b2 := dialSimBrowser(t, ph, b2Key)
	nodeMsg := exchangeBrowserRoom(t, t.Context(), b2, secret, "b", "a")
	if nodeMsg.Key != ph.nodeKey.ID() || nodeMsg.Role != "a" {
		t.Fatalf("unexpected role a pair msg: %+v", nodeMsg)
	}
	waitKnown(t, ph.pairer, b2Key.ID())

	conn2, err := b2.peer.Offer(t.Context(), ph.nodeKey.ID())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn2.Close() })
	select {
	case <-conn2.Ready():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for b2 ctl")
	}

	helloReq := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Phone", Addrs: []string{}}
	}
	rawW1, err := conn2.Request(t.Context(), helloReq)
	if err != nil {
		t.Fatal(err)
	}
	var w1 proto.Welcome
	if err := proto.Decode(rawW1, &w1); err != nil || w1.Owner {
		t.Fatalf("b2 first welcome = %+v (%v)", w1, err)
	}

	rawDL, err := conn2.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var dl proto.Devlog
	if err := proto.Decode(rawDL, &dl); err != nil || len(dl.Entries) != 3 {
		t.Fatalf("b2 devlog = %+v (%v)", dl, err)
	}
	cand, err := identity.Replay(ownerKey.ID(), dl.Entries)
	if err != nil || !cand.Adopts(b2Key.ID(), ph.nodeKey.ID()) {
		t.Fatalf("b2 log did not adopt: %v", err)
	}

	// Connected first browser also received the forwarded add entry.
	var fwd proto.Devlog
	if err := proto.Decode(b1Ctl.next(t), &fwd); err != nil || len(fwd.Entries) != 1 {
		t.Fatalf("forwarded devlog = %+v (%v)", fwd, err)
	}

	// Second hello on b2 reports owner true.
	rawW2, err := conn2.Request(t.Context(), helloReq)
	if err != nil {
		t.Fatal(err)
	}
	var w2 proto.Welcome
	if err := proto.Decode(rawW2, &w2); err != nil || !w2.Owner {
		t.Fatalf("b2 second welcome = %+v (%v)", w2, err)
	}

	// Replay with nonce1 after link was used returns conflict.
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 4}, Env: pbEnv})
	expectErr(t, b1Ctl.next(t), 4, "conflict")

	// Second pair.browser with nonce2: test exact expiry boundary (now == expires -> conflict and secret cleared).
	nonce2 := sesID("pb-nonce-2")
	pbEnv2 := ownerKey.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, nonce2, ph.clock.Now().UnixMilli(), ph.nodeKey.ID()))
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 5}, Env: pbEnv2})
	var link2 proto.CmdResult
	if err := proto.Decode(b1Ctl.next(t), &link2); err != nil {
		t.Fatal(err)
	}
	ph.pairer.mu.Lock()
	a2 := ph.pairer.links[ownerKey.ID()+"\x00"+string(nonce2)]
	ph.pairer.mu.Unlock()
	ph.clock.Set(time.UnixMilli(link2.Expires))
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 6}, Env: pbEnv2})
	expectErr(t, b1Ctl.next(t), 6, "conflict")
	ph.pairer.mu.Lock()
	cleared := a2.done && a2.secret == ""
	ph.pairer.mu.Unlock()
	if !cleared {
		t.Fatal("expected expired replay to finish attempt and clear secret")
	}

	// Replay of a nonce stored in SQLite receipt when Pairer has no in-memory entry (simulating restart) -> conflict.
	nonce3 := sesID("pb-nonce-3")
	putRec := func(tx *store.Tx) error {
		return tx.PutReceipt(ownerKey.ID(), string(nonce3), ph.clock.Now().UnixMilli(), store.Outcome{OK: true})
	}
	if err := ph.db.Write(putRec); err != nil {
		t.Fatal(err)
	}
	pbEnv3 := ownerKey.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, nonce3, ph.clock.Now().UnixMilli(), ph.nodeKey.ID()))
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 7}, Env: pbEnv3})
	expectErr(t, b1Ctl.next(t), 7, "conflict")

	// Replay of a command that originally failed returns the original refusal code.
	badEnv := ownerKey.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":123}`, sesID("pb-bad"), ph.clock.Now().UnixMilli()))
	b1Ctl.push(proto.Cmd{Request: proto.Request{Type: "cmd", ID: 8}, Env: badEnv})
	expectErr(t, b1Ctl.next(t), 8, "bad_request")
}

func TestPairRoomEdgeCasesAndFailures(t *testing.T) {
	t.Run("defaults, owned refusal, bad secret, dialed PairBrowser and no rendezvous", func(t *testing.T) {
		ownerKey := sesKey("owned")
		ph := newPairHarness(t, &ownerKey, false)
		defPairer := NewPairer(PairConfig{Key: ph.nodeKey, Store: ph.db, Logs: ph.logs, Sessions: ph.sessions})
		openA := &pairAttempt{secret: identity.NewSecret(), link: PairLink{Expires: time.Now().Add(pairWindow).UnixMilli()}, cancel: func() {}, outcome: make(chan error, 1)}
		defPairer.mu.Lock()
		defPairer.attempts[openA] = true
		defPairer.mu.Unlock()
		defPairer.Close()
		if !openA.done || !errors.Is(<-openA.outcome, net.ErrClosed) || openA.secret != "" {
			t.Fatalf("Close did not finish open attempt: done=%v secret=%q", openA.done, openA.secret)
		}
		if defPairer.cfg.Now == nil || defPairer.cfg.After == nil || defPairer.sharedPaths() != nil {
			t.Fatal("NewPairer left nil callbacks")
		}
		customPairer := NewPairer(PairConfig{
			Key: ph.nodeKey, Store: ph.db, Logs: ph.logs, Sessions: ph.sessions,
			Name: func() string { return "custom-name" }, Rendezvous: func() []string { return []string{"wss://custom.example/rv"} },
		})
		if customPairer.cfg.Name() != "custom-name" || len(customPairer.cfg.Rendezvous()) != 1 || customPairer.cfg.Rendezvous()[0] != "wss://custom.example/rv" {
			t.Fatal("NewPairer overwrote custom Name or Rendezvous")
		}
		if _, err := ph.pairer.Pair(t.Context(), PairRequest{Secret: "7K2M-9QXD-4F8T-WB3N"}); !errors.Is(err, ErrOwned) {
			t.Fatalf("got %v, want ErrOwned", err)
		}
		// Dialed PairBrowser (when Paths is nil) connects and expires via FireAll.
		link, err := ph.pairer.PairBrowser(t.Context(), "http://localhost:47800/")
		if err != nil || !strings.HasPrefix(link.Link, "http://localhost:47800/#pair=") {
			t.Fatalf("dialed PairBrowser = %+v, %v", link, err)
		}
		ph.clock.WaitWaiters(t, 1)
		ph.clock.FireAll()
		waitAttempts(t, ph.pairer, 0)

		phUnowned := newPairHarness(t, nil, false)
		if _, err := phUnowned.pairer.Pair(t.Context(), PairRequest{Secret: "not-valid"}); err == nil {
			t.Fatal("expected ParseSecret error")
		}
		clearRV := func(tx *store.Tx) error { _, err := tx.Exec("DELETE FROM setting"); return err }
		if err := phUnowned.db.Write(clearRV); err != nil {
			t.Fatal(err)
		}
		if _, err := phUnowned.pairer.Pair(t.Context(), PairRequest{Secret: identity.NewSecret().String()}); !errors.Is(err, errNoRendezvous) {
			t.Fatalf("got %v, want errNoRendezvous", err)
		}
		if _, err := phUnowned.pairer.PairBrowser(t.Context(), ""); err == nil {
			t.Fatal("expected unowned PairBrowser conflict")
		}
	})

	t.Run("saveLabel on success only, store error, preset name, explicit RV override and two-path survival", func(t *testing.T) {
		phShared := newPairHarness(t, nil, true)
		putPreset := func(tx *store.Tx) error {
			_, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('name', '"preset-box"')`)
			return err
		}
		if err := phShared.db.Write(putPreset); err != nil {
			t.Fatal(err)
		}
		// Explicit Rendezvous overrides shared Paths, returns the dial error, and does not save --name on failure.
		if _, err := phShared.pairer.Pair(t.Context(), PairRequest{Secret: identity.NewSecret().String(), Name: "aborted", Rendezvous: []string{"ws://127.0.0.1:1/rv"}}); err == nil || errors.Is(err, errNoRendezvous) {
			t.Fatalf("expected dial error from explicit Rendezvous, got %v", err)
		}
		if ReadName(phShared.db) != "preset-box" {
			t.Fatalf("failed Pair saved --name: %q", ReadName(phShared.db))
		}

		ctx, cancel := context.WithCancel(t.Context())
		errCh := make(chan error, 1) // single reader: subtest
		go func() {
			_, err := phShared.pairer.Pair(ctx, PairRequest{Secret: identity.NewSecret().String()})
			errCh <- err
		}()
		waitAttempts(t, phShared.pairer, 1)
		phShared.pairer.mu.Lock()
		var gotLabel string
		for a := range phShared.pairer.attempts {
			gotLabel = a.label
		}
		phShared.pairer.mu.Unlock()
		if gotLabel != "preset-box" {
			t.Fatalf("empty req.Name label = %q, want preset-box", gotLabel)
		}
		// Unrelated device adoption leaves Role B attempt open.
		phShared.pairer.onPairAdopt(&sesConn{device: sesKey("unrelated").ID(), raw: newFakeCtl()})
		waitAttempts(t, phShared.pairer, 1)
		cancel()
		if got := <-errCh; !errors.Is(got, context.Canceled) {
			t.Fatalf("shared role b cancel = %v", got)
		}

		// Succeeding without --name preserves existing setting('name').
		noSavePeer := sesKey("no-save-peer").ID()
		aNoSave := &pairAttempt{
			role: "b", wantRole: "a", secret: identity.NewSecret(), label: "preset-box",
			peer: proto.Pair{Key: noSavePeer}, link: PairLink{Expires: phShared.clock.Now().Add(pairWindow).UnixMilli()},
			cancel: func() {}, outcome: make(chan error, 1),
		}
		phShared.pairer.mu.Lock()
		phShared.pairer.attempts[aNoSave] = true
		phShared.pairer.mu.Unlock()
		phShared.pairer.onPairAdopt(&sesConn{device: noSavePeer, raw: newFakeCtl()})
		if err := <-aNoSave.outcome; err != nil || ReadName(phShared.db) != "preset-box" {
			t.Fatalf("empty saveName finish = %v, name = %q", err, ReadName(phShared.db))
		}

		// Two rendezvous paths: closing the first leaves pathsLeft == 1 and does not fail the attempt;
		// once conn is set, closing the second path (pathsLeft == 0) also leaves the attempt alive until adoption or ctl close.
		srv2 := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
		srv3 := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
		rvURL2 := "ws" + strings.TrimPrefix(srv2.URL, "http") + "/rv"
		rvURL3 := "ws" + strings.TrimPrefix(srv3.URL, "http") + "/rv"
		ctx2, cancel2 := context.WithCancel(t.Context())
		err2Ch := make(chan error, 1) // single reader: subtest
		go func() {
			_, err := phShared.pairer.Pair(ctx2, PairRequest{Secret: identity.NewSecret().String(), Name: "aborted-cancel", Rendezvous: []string{rvURL2, rvURL3}})
			err2Ch <- err
		}()
		waitAttempts(t, phShared.pairer, 1)
		srv2.Close()
		time.Sleep(30 * time.Millisecond)
		waitAttempts(t, phShared.pairer, 1)
		phShared.pairer.mu.Lock()
		for a := range phShared.pairer.attempts {
			a.conn = newFakeCtl()
		}
		phShared.pairer.mu.Unlock()
		srv3.Close()
		time.Sleep(30 * time.Millisecond)
		waitAttempts(t, phShared.pairer, 1)
		cancel2()
		<-err2Ch
		if ReadName(phShared.db) != "preset-box" {
			t.Fatalf("cancelled Pair overwrote setting('name'): %q", ReadName(phShared.db))
		}

		ph := newPairHarness(t, nil, false)
		drop := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE setting"); return err }
		if err := ph.db.Write(drop); err != nil {
			t.Fatal(err)
		}
		saveErrPeer := sesKey("save-err-peer").ID()
		aSaveErr := &pairAttempt{
			role: "b", wantRole: "a", secret: identity.NewSecret(), label: "x", saveName: "x",
			peer: proto.Pair{Key: saveErrPeer}, link: PairLink{Expires: ph.clock.Now().Add(pairWindow).UnixMilli()},
			cancel: func() {}, outcome: make(chan error, 1),
		}
		ph.pairer.mu.Lock()
		ph.pairer.attempts[aSaveErr] = true
		ph.pairer.mu.Unlock()
		ph.pairer.onPairAdopt(&sesConn{device: saveErrPeer, raw: newFakeCtl()})
		if err := <-aSaveErr.outcome; err == nil {
			t.Fatal("expected label store error in finish")
		}
	})

	t.Run("concurrent role b, expiry, context cancel and room full", func(t *testing.T) {
		ph := newPairHarness(t, nil, true)
		s1, s2 := identity.NewSecret(), identity.NewSecret()
		err1 := make(chan error, 1) // single reader: subtest
		go func() {
			_, err := ph.pairer.Pair(t.Context(), PairRequest{Secret: s1.String(), Name: "first"})
			err1 <- err
		}()
		err2 := make(chan error, 1) // single reader: subtest
		go func() {
			_, err := ph.pairer.Pair(t.Context(), PairRequest{Secret: s2.String(), Name: "second"})
			err2 <- err
		}()
		waitAttempts(t, ph.pairer, 2)
		ph.clock.WaitWaiters(t, 2)
		ph.clock.FireAll()
		if got := <-err1; !errors.Is(got, errPairExpired) {
			t.Fatalf("first attempt = %v, want errPairExpired", got)
		}
		if got := <-err2; !errors.Is(got, errPairExpired) {
			t.Fatalf("second attempt = %v, want errPairExpired", got)
		}
		if ReadName(ph.db) != "" {
			t.Fatalf("expired Pair saved --name: %q", ReadName(ph.db))
		}

		// Room already full (2 browsers in room before node joins).
		sFull := identity.NewSecret()
		bA := dialSimBrowser(t, ph, sesKey("full-a"))
		bB := dialSimBrowser(t, ph, sesKey("full-b"))
		joinedB := make(chan struct{}, 1) // single reader: subtest
		go bA.path.Serve(t.Context(), bA.peer, func(string, []byte) error { return nil })
		go bB.path.Serve(t.Context(), bB.peer, func(kind string, _ []byte) error {
			if kind == "joined" {
				select {
				case joinedB <- struct{}{}:
				default:
				}
			}
			return nil
		})
		bA.path.Write(t.Context(), proto.RoomID{Type: "join", Room: sFull.Room()})
		bB.path.Write(t.Context(), proto.RoomID{Type: "join", Room: sFull.Room()})
		<-joinedB
		if _, err := ph.pairer.Pair(t.Context(), PairRequest{Secret: sFull.String()}); !errors.Is(err, errRoomFull) {
			t.Fatalf("full room = %v, want errRoomFull", err)
		}
	})

	t.Run("room frame validation, path drop before ctl, ctl drop before adopt", func(t *testing.T) {
		var wsConns []*websocket.Conn
		var wsMu sync.Mutex
		customRV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			wsMu.Lock()
			wsConns = append(wsConns, c)
			wsMu.Unlock()
			c.Write(r.Context(), websocket.MessageText, dvlMustEncode(proto.Challenge{Type: "challenge", Nonce: "abc"}))
			c.Read(r.Context()) // auth
			c.Write(r.Context(), websocket.MessageText, dvlMustEncode(proto.ICEConfig{Type: "ready", ICE: []proto.ICEServer{}, TTL: 3600}))
			c.Read(r.Context()) // join
			// Send malformed room, unknown room, then close socket.
			c.Write(r.Context(), websocket.MessageText, []byte(`{"t":"joined","room":123}`))
			c.Write(r.Context(), websocket.MessageText, []byte(`{"t":"joined","room":"00000000000000000000000000000000"}`))
			c.CloseNow()
		}))
		t.Cleanup(customRV.Close)
		wsURL := "ws" + strings.TrimPrefix(customRV.URL, "http")

		ph := newPairHarness(t, nil, false)
		if _, err := ph.pairer.Pair(t.Context(), PairRequest{Secret: identity.NewSecret().String(), Rendezvous: []string{wsURL}}); err == nil {
			t.Fatal("expected error when room path closes before ctl opens")
		}
		wsMu.Lock()
		nConns := len(wsConns)
		wsMu.Unlock()
		if nConns != 1 {
			t.Fatalf("wsConns = %d, want 1", nConns)
		}

		roleA := &pairAttempt{
			role: "a", wantRole: "b", secret: identity.NewSecret(), room: "11111111111111111111111111111111",
			link:   PairLink{Expires: ph.clock.Now().Add(pairWindow).UnixMilli()},
			routes: []peer.Route{{}}, cancel: func() {}, outcome: make(chan error, 1),
		}
		ph.pairer.mu.Lock()
		ph.pairer.attempts[roleA] = true
		ph.pairer.mu.Unlock()
		// At exact expires, findRoom, onPairMsg and Known treat the attempt as expired.
		pExp := sesKey("p-exp")
		msgExp := proto.Pair{Type: "pair", Key: pExp.ID(), Kind: "browser", Name: "b", Role: "b"}
		msgExp.MAC = roleA.secret.MAC(msgExp)
		ph.clock.Set(time.UnixMilli(roleA.link.Expires))
		ph.pairer.HandleEvent(t.Context(), nil, "room", dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: roleA.room}, Data: dvlMustEncode(msgExp)}))
		ph.pairer.onPairMsg(roleA, dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: roleA.room}, Data: dvlMustEncode(msgExp)}))
		if roleA.peer.Key != "" {
			t.Fatal("expired attempt accepted pair message")
		}
		roleA.peer = msgExp
		if ph.pairer.Known(pExp.ID()) {
			t.Fatal("Known returned true at exact expires")
		}
		ph.clock.Set(time.UnixMilli(roleA.link.Expires - 1))
		if !ph.pairer.Known(pExp.ID()) {
			t.Fatal("Known returned false at expires - 1")
		}
		// onPairAdopt ignores a role "a" attempt even when its peer matches, and HandleEvent ignores malformed RoomID.
		ph.pairer.onPairAdopt(&sesConn{device: pExp.ID(), raw: newFakeCtl()})
		ph.pairer.HandleEvent(t.Context(), nil, "full", []byte(`{"t":"full","room":123}`))
		if roleA.done {
			t.Fatal("roleA finished by onPairAdopt or malformed full frame")
		}
		ph.pairer.finish(roleA, nil)
		ph.clock.Set(time.UnixMilli(1763600000000))
		secret := identity.NewSecret()
		a := &pairAttempt{
			role: "b", wantRole: "a", secret: secret, room: secret.Room(),
			link:   PairLink{Expires: ph.clock.Now().Add(pairWindow).UnixMilli()},
			routes: []peer.Route{{}}, cancel: func() {}, outcome: make(chan error, 1),
		}
		ph.pairer.mu.Lock()
		ph.pairer.attempts[a] = true
		ph.pairer.mu.Unlock()

		ph.pairer.onPairMsg(a, []byte(`not-json`))
		ph.pairer.onPairMsg(a, dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: a.room}, Data: json.RawMessage(`123`)}))
		ownEcho := proto.Pair{Type: "pair", Key: sesKey("p1").ID(), Kind: "browser", Name: "b", Role: "b"}
		ownEcho.MAC = secret.MAC(ownEcho)
		ph.pairer.onPairMsg(a, dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: a.room}, Data: dvlMustEncode(ownEcho)}))
		badMAC := proto.Pair{Type: "pair", Key: sesKey("p1").ID(), Kind: "browser", Name: "b", Role: "a", MAC: "wrong"}
		ph.pairer.onPairMsg(a, dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: a.room}, Data: dvlMustEncode(badMAC)}))
		if a.peer.Key != "" {
			t.Fatalf("accepted invalid pair message: %+v", a.peer)
		}

		p1Key, p2Key := sesKey("p1"), sesKey("p2")
		valid1 := proto.Pair{Type: "pair", Key: p1Key.ID(), Kind: "browser", Name: "b1", Role: "a"}
		valid1.MAC = secret.MAC(valid1)
		valid2 := proto.Pair{Type: "pair", Key: p2Key.ID(), Kind: "browser", Name: "b2", Role: "a"}
		valid2.MAC = secret.MAC(valid2)
		ph.pairer.HandleEvent(t.Context(), nil, "room", dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: a.room}, Data: dvlMustEncode(valid1)}))
		ph.pairer.onPairMsg(a, dvlMustEncode(proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: a.room}, Data: dvlMustEncode(valid2)}))
		if a.peer.Key != p1Key.ID() {
			t.Fatalf("peer = %q, want %q", a.peer.Key, p1Key.ID())
		}

		// Serve closing before adoption finishes the attempt with EOF; Serve on an unpaired device does not touch attempts.
		fc := newFakeCtl()
		fc.Close()
		ph.pairer.Serve(t.Context(), p1Key.ID(), fc)
		if err := <-a.outcome; !errors.Is(err, net.ErrClosed) && err == nil {
			t.Fatal("expected EOF when paired ctl closes before adoption")
		}
		ph.pairer.finish(a, errRoomFull)
		if len(a.outcome) != 0 {
			t.Fatal("second finish wrote another outcome")
		}
		fcUnpaired := newFakeCtl()
		fcUnpaired.Close()
		ph.pairer.Serve(t.Context(), sesKey("unpaired-peer").ID(), fcUnpaired)
		ph.pairer.onPairAdopt(nil)
		ph.pairer.onPairHello(&sesConn{device: p2Key.ID()})
	})

	t.Run("PairBrowser schemes, dial error, invalid cmd payload and append error", func(t *testing.T) {
		ownerKey := sesKey("owner-web")
		ph := newPairHarness(t, &ownerKey, false)

		if _, err := ph.pairer.cmdPairBrowser(t.Context(), nil, ownerKey.ID(), json.RawMessage(`{}`)); err == nil {
			t.Fatal("expected bad_request on invalid pair.browser payload")
		}
		ph.pairer.cfg.WebURL = func() string { return "https://console.example/" }
		resCustom, err := ph.pairer.cmdPairBrowser(t.Context(), nil, ownerKey.ID(), dvlMustEncode(proto.HostCommand{Type: "pair.browser", Nonce: sesID("custom-web"), Timestamp: 1, To: ph.nodeKey.ID()}))
		if err != nil || !strings.HasPrefix(resCustom.(PairLink).Link, "https://console.example/#pair=") {
			t.Fatalf("custom WebURL cmdPairBrowser = %+v, %v", resCustom, err)
		}
		ph.pairer.cfg.WebURL = func() string { return "" }
		ph.pairer.cfg.Rendezvous = func() []string { return []string{"ftp://bad/rv"} }
		if _, err := ph.pairer.cmdPairBrowser(t.Context(), nil, ownerKey.ID(), dvlMustEncode(proto.HostCommand{Type: "pair.browser", Nonce: sesID("x"), Timestamp: 1, To: ph.nodeKey.ID()})); err == nil {
			t.Fatal("expected unsupported scheme error")
		}
		ph.pairer.cfg.Rendezvous = func() []string { return []string{"ws://127.0.0.1:1/rv"} }
		if _, err := ph.pairer.PairBrowser(t.Context(), ""); err == nil {
			t.Fatal("expected dial error on unreachable rendezvous")
		}

		for _, tc := range []struct {
			web  string
			rvs  []string
			want string
			ok   bool
		}{
			{"https://app.example/", nil, "https://app.example", true},
			{"", []string{"wss://rv.example/rv"}, "https://rv.example", true},
			{"", []string{"https://rv.example/rv"}, "https://rv.example", true},
			{"", []string{"ws://127.0.0.1:8080/rv"}, "http://127.0.0.1:8080", true},
			{"", []string{"http://127.0.0.1:8080/rv"}, "http://127.0.0.1:8080", true},
			{"", nil, "", false},
			{"", []string{"://bad"}, "", false},
			{"", []string{"wss:///nohost"}, "", false},
			{"", []string{"ftp://rv.example"}, "", false},
		} {
			got, err := pairWebBase(tc.web, tc.rvs)
			if (err == nil) != tc.ok || got != tc.want {
				t.Fatalf("pairWebBase(%q, %v) = (%q, %v), want (%q, ok=%v)", tc.web, tc.rvs, got, err, tc.want, tc.ok)
			}
		}

		// Verify dvlAppendAdd error reporting when newcomer key was previously revoked, unowned, or DB fails.
		revKey := sesKey("revoked-browser")
		revEnv := sesRevokeEntry(ownerKey, ownerKey.ID(), 3, revKey.ID())
		if err := ph.logs.dvlReceive(proto.Devlog{Type: "devlog", User: ownerKey.ID(), Entries: []proto.Envelope{revEnv}}, nil); err != nil {
			t.Fatal(err)
		}
		revSecret := identity.NewSecret()
		aRev := &pairAttempt{
			role: "a", wantRole: "b", secret: revSecret, room: revSecret.Room(),
			peer:   proto.Pair{Key: revKey.ID(), Kind: "browser", Name: "rev"},
			link:   PairLink{Expires: ph.clock.Now().Add(pairWindow).UnixMilli()},
			routes: []peer.Route{{}}, cancel: func() {}, outcome: make(chan error, 1),
		}
		ph.pairer.mu.Lock()
		ph.pairer.attempts[aRev] = true
		ph.pairer.mu.Unlock()
		ph.pairer.onPairHello(&sesConn{device: revKey.ID(), paired: true})
		select {
		case repErr := <-ph.reported:
			if repErr == nil {
				t.Fatal("expected reported error")
			}
		case <-time.After(time.Second):
			t.Fatal("expected dvlAppendAdd error to be reported")
		}

		emptyLogs, err := OpenLogs(sesOpenStore(t), ph.nodeKey.ID())
		if err != nil {
			t.Fatal(err)
		}
		if err := emptyLogs.dvlAppendAdd(ph.nodeKey, sesKey("x").ID(), "browser", "x", 1, nil); err == nil {
			t.Fatal("expected error appending to unowned logs")
		}
		dbDrop := sesOpenStore(t)
		ownedDrop := seedOwnedLogs(t, dbDrop, ph.nodeKey, ownerKey)
		dropLog := func(tx *store.Tx) error { _, err := tx.Exec("DROP TABLE device_log"); return err }
		if err := dbDrop.Write(dropLog); err != nil {
			t.Fatal(err)
		}
		if err := ownedDrop.dvlAppendAdd(ph.nodeKey, sesKey("fresh").ID(), "browser", "x", 1, nil); err == nil {
			t.Fatal("expected store error on dropped device_log")
		}
	})
}

func waitAttempts(t *testing.T, p *Pairer, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		n := len(p.attempts)
		p.mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d attempts", want)
}
