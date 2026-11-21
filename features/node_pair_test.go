//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["node_pair"] = func(sc *godog.ScenarioContext, _ fixtures) { nodePairSteps(sc) }
}

type pairStepBrowser struct {
	key    identity.Key
	path   *peer.Path
	peer   *peer.Peer
	conn   *peer.Connection
	pushes chan json.RawMessage // single reader: pairStepBrowser.nextPush in scenario steps
}

type nodePairWorld struct {
	ctx     context.Context
	cleanup func(func())

	mu       sync.Mutex
	now      time.Time
	rvURL    string
	ice      []proto.ICEServer
	db       *store.Store
	nodeKey  identity.Key
	logs     *node.Logs
	sessions *node.Sessions
	surface  *node.Surface
	pairer   *node.Pairer
	nodePeer *peer.Peer

	b1, b2       *pairStepBrowser
	otherKey     identity.Key
	secret       identity.Secret
	roomPeer     proto.Pair
	pairDone     chan pairCallOut // single reader: checkPairCompleted
	lastWelcome  proto.Welcome
	linksByNonce map[string]proto.CmdResult
	lastCmdRaw   json.RawMessage
}

type pairCallOut struct {
	res node.PairResult
	err error
}

func nodePairSteps(sc *godog.ScenarioContext) {
	w := &nodePairWorld{
		cleanup:      afterScenario(sc),
		now:          time.UnixMilli(1763600000000),
		linksByNonce: map[string]proto.CmdResult{},
	}
	sc.Step(`^an in-process rendezvous and an unowned pairing node$`, func() error { return w.setupNode(false) })
	sc.Step(`^a fresh browser creates a pairing secret and joins its room$`, w.browserCreatesSecret)
	sc.Step(`^the pairing node runs pair with that secret and label "([^"]*)"$`, w.nodeRunsPair)
	sc.Step(`^the node fingerprint seen in the room equals the node key fingerprint$`, w.checkRoomFingerprint)
	sc.Step(`^the pairing browser opens ctl and sends hello before any device log$`, w.browserOpensCtlAndSaysHello)
	sc.Step(`^the pairing welcome reports owner (true|false)$`, w.checkWelcomeOwner)
	sc.Step(`^host\.info before enrolment is refused with "([^"]*)"$`, w.checkPreEnrolmentForbidden)
	sc.Step(`^the pairing browser sends a log whose add entry for the node was signed by another device$`, w.sendWrongSignerLog)
	sc.Step(`^the pairing node still has no owner and refuses host\.info with "([^"]*)"$`, w.checkUnownedForbidden)
	sc.Step(`^the pairing browser sends a valid device log adding the node signed by the pairing browser$`, w.sendValidAdoptionLog)
	sc.Step(`^pair completes with the node fingerprint and active path$`, w.checkPairCompleted)
	sc.Step(`^the pairing browser sends hello again$`, func() error { return w.sendHello(w.b1.conn) })
	sc.Step(`^host\.info succeeds for the owner browser$`, w.checkOwnerSurface)

	sc.Step(`^an owned node with a connected owner browser over the rendezvous$`, w.setupOwnedWithBrowser)
	sc.Step(`^the owner browser sends pair\.browser with nonce "([^"]*)"$`, w.sendPairBrowser)
	sc.Step(`^the owner browser repeats pair\.browser with nonce "([^"]*)" while the link is valid$`, w.repeatPairBrowserValid)
	sc.Step(`^both answers return ok with the same #pair link and expiry$`, w.checkSamePairLink)
	sc.Step(`^a second browser pairs using that #pair link$`, w.secondBrowserPairs)
	sc.Step(`^the second browser receives the device log, adopts the owner and reads owner true$`, w.checkSecondBrowserAdopted)
	sc.Step(`^the owner browser repeats pair\.browser with nonce "([^"]*)" after the link was used$`, w.sendPairBrowserRaw)
	sc.Step(`^10 minutes pass so the second link expires$`, w.advanceTenMinutes)
	sc.Step(`^the owner browser repeats pair\.browser with nonce "([^"]*)"$`, w.sendPairBrowserRaw)
	sc.Step(`^the pair\.browser repeat is refused with "([^"]*)"$`, w.checkLastRefused)
}

func (w *nodePairWorld) nowTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

func (w *nodePairWorld) setupNode(owned bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)

	relay := &peersWorld{}
	err := relay.relayOnly()
	for _, undo := range relay.cleanup {
		w.cleanup(undo)
	}
	if err != nil {
		return err
	}
	w.ice = relay.relay

	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(srv.Close)
	w.rvURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"

	w.nodeKey = exampleKey("n3-node")
	b1Key := exampleKey("n3-b1")
	w.otherKey = exampleKey("n3-other")

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db

	if err := w.initLogs(owned, b1Key); err != nil {
		return err
	}
	if err := w.initNodeStack(owned); err != nil {
		return err
	}
	b1, err := w.newBrowser(b1Key)
	w.b1 = b1
	return err
}

func (w *nodePairWorld) initLogs(owned bool, ownerKey identity.Key) error {
	if owned {
		ownerJSON, err := json.Marshal(ownerKey.ID())
		if err != nil {
			return err
		}
		e1 := w.signDeviceEntry(ownerKey, ownerKey.ID(), 1, ownerKey.ID(), "browser", "b1")
		e2 := w.signDeviceEntry(ownerKey, ownerKey.ID(), 2, w.nodeKey.ID(), "node", "workstation")
		raw1, _ := json.Marshal(e1)
		raw2, _ := json.Marshal(e2)
		seed := func(tx *store.Tx) error {
			if _, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?), ('name', '\"workstation\"')", string(ownerJSON)); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO device_log(user, ver, env) VALUES(?, 1, ?), (?, 2, ?)", ownerKey.ID(), string(raw1), ownerKey.ID(), string(raw2)); err != nil {
				return err
			}
			return nil
		}
		if err := w.db.Write(seed); err != nil {
			return err
		}
	}
	logs, err := node.OpenLogs(w.db, w.nodeKey.ID())
	w.logs = logs
	return err
}

func (w *nodePairWorld) initNodeStack(owned bool) error {
	rawRV, _ := json.Marshal([]string{w.rvURL})
	putRV := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('rendezvous', ?)", string(rawRV))
		return err
	}
	if err := w.db.Write(putRV); err != nil {
		return err
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: w.nodeKey.ID(), Owner: w.logs.Owner(),
		Logs: w.logs.Log, Tables: home.NodeTables(), Now: w.nowTime,
	})
	if err != nil {
		return err
	}
	w.cleanup(loop.Close)
	h, _, err := host.New(w.db, w.nodeKey, w.nowTime)
	if err != nil {
		return err
	}
	w.sessions = node.NewSessions(node.SessionConfig{
		ID: w.nodeKey.ID(), Version: "v0.3.1", Store: w.db, Logs: w.logs, Home: loop, Host: h,
		Name:  func() string { return node.ReadName(w.db) },
		Addrs: func() []string { return []string{w.rvURL} },
		Now:   w.nowTime,
	})
	w.surface = node.NewSurface(node.SurfaceConfig{
		ID: w.nodeKey.ID(), OS: "darwin/arm64", Version: "v0.3.1",
		Store: w.db, Sessions: w.sessions, Host: h,
		Agents: func(context.Context) ([]proto.HostAgent, error) { return []proto.HostAgent{}, nil },
	})

	var nodePath *peer.Path
	w.pairer = node.NewPairer(node.PairConfig{
		Key: w.nodeKey, Store: w.db, Logs: w.logs, Sessions: w.sessions,
		Paths: func() []*peer.Path {
			if nodePath != nil {
				return []*peer.Path{nodePath}
			}
			return nil
		},
		Now: w.nowTime,
	})
	w.cleanup(w.pairer.Close)
	return w.initNodePeer(owned, &nodePath)
}

func (w *nodePairWorld) initNodePeer(owned bool, nodePath **peer.Path) error {
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	w.cleanup(func() { socks.Close() })

	known := func(dev string) bool { return w.logs.Known(dev) || w.pairer.Known(dev) }
	paths := func(dev string) []peer.Route {
		var out []peer.Route
		if *nodePath != nil {
			out = append(out, w.withRelay((*nodePath).Route()))
		}
		for _, r := range w.pairer.Routes(dev) {
			out = append(out, w.withRelay(r))
		}
		return out
	}
	inbound := func(c *peer.Connection) { go w.pairer.Serve(w.ctx, c.Device, c) }
	w.nodePeer = peer.New(w.nodeKey, socks, w.db, known, paths, inbound, nil)
	w.pairer.Peer = w.nodePeer
	if !owned {
		return nil
	}
	p, err := peer.Dial(w.ctx, w.rvURL, w.nodeKey)
	if err != nil {
		return err
	}
	*nodePath = p
	w.cleanup(func() { p.Close() })
	event := func(kind string, data []byte) error {
		w.pairer.HandleEvent(w.ctx, p, kind, data)
		return nil
	}
	go p.Serve(w.ctx, w.nodePeer, event)
	return nil
}

func (w *nodePairWorld) withRelay(r peer.Route) peer.Route {
	orig := r.Send
	r.ICE = w.ice
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		var sig struct {
			Type string `json:"t"`
			Cand string `json:"cand"`
		}
		if json.Unmarshal([]byte(env.Payload), &sig) == nil && sig.Type == "ice" && !isRelayCand(sig.Cand) {
			return nil
		}
		return orig(ctx, to, env)
	}
	return r
}

func (w *nodePairWorld) newBrowser(key identity.Key) (*pairStepBrowser, error) {
	path, err := peer.Dial(w.ctx, w.rvURL, key)
	if err != nil {
		return nil, err
	}
	w.cleanup(func() { path.Close() })
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return nil, err
	}
	w.cleanup(func() { socks.Close(); db.Close() })

	routes := func(string) []peer.Route { return []peer.Route{w.withRelay(path.Route())} }
	p := peer.New(key, socks, db, identity.IsDevice, routes, func(*peer.Connection) {}, nil)
	return &pairStepBrowser{key: key, path: path, peer: p}, nil
}

func (w *nodePairWorld) browserCreatesSecret() error {
	w.secret = identity.NewSecret()
	return nil
}

func (w *nodePairWorld) nodeRunsPair(label string) error {
	w.pairDone = make(chan pairCallOut, 1)
	go func() {
		res, err := w.pairer.Pair(w.ctx, node.PairRequest{Secret: w.secret.String(), Name: label})
		w.pairDone <- pairCallOut{res: res, err: err}
	}()
	peerMsg, err := w.joinAndExchange(w.b1, w.secret, "a", "b")
	if err != nil {
		return err
	}
	w.roomPeer = peerMsg
	return w.awaitKnown(w.b1.key.ID())
}

func (w *nodePairWorld) awaitKnown(device string) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w.pairer.Known(device) {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for node to record pairing peer %q", device)
}

func (w *nodePairWorld) joinAndExchange(b *pairStepBrowser, secret identity.Secret, myRole, wantRole string) (proto.Pair, error) {
	room := secret.Room()
	self := proto.Pair{Type: "pair", Key: b.key.ID(), Kind: "browser", Name: "Browser", Role: myRole}
	self.MAC = secret.MAC(self)
	rawSelf, _ := json.Marshal(self)

	got := make(chan proto.Pair, 1) // single reader: joinAndExchange
	var once atomic.Bool
	event := func(kind string, data []byte) error {
		if kind == "joined" {
			b.path.Write(w.ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: room}, Data: rawSelf})
		}
		if kind == "room" {
			if m, ok := matchStepRoomPeer(data, room, secret, wantRole); ok && once.CompareAndSwap(false, true) {
				got <- m
			}
		}
		return nil
	}
	go b.path.Serve(w.ctx, b.peer, event)
	if err := b.path.Write(w.ctx, proto.RoomID{Type: "join", Room: room}); err != nil {
		return proto.Pair{}, err
	}
	select {
	case m := <-got:
		return m, nil
	case <-w.ctx.Done():
		return proto.Pair{}, w.ctx.Err()
	}
}

func matchStepRoomPeer(data []byte, room string, secret identity.Secret, wantRole string) (proto.Pair, bool) {
	var rd proto.RoomData
	var m proto.Pair
	if proto.Decode(data, &rd) != nil || rd.Room != room || proto.Decode(rd.Data, &m) != nil {
		return proto.Pair{}, false
	}
	return m, secret.Verify(m, wantRole)
}

func (w *nodePairWorld) openBrowserCtl(b *pairStepBrowser) error {
	conn, err := b.peer.Offer(w.ctx, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.cleanup(func() { conn.Close() })
	select {
	case <-conn.Ready():
		b.conn = conn
		b.pushes = make(chan json.RawMessage, 32)
		go pumpStepPushes(w.ctx, conn, b.pushes)
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func pumpStepPushes(ctx context.Context, conn *peer.Connection, pushes chan<- json.RawMessage) {
	defer close(pushes)
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		pushes <- msg
	}
}

func (b *pairStepBrowser) nextPush(ctx context.Context) (json.RawMessage, error) {
	select {
	case msg, ok := <-b.pushes:
		if !ok {
			return nil, fmt.Errorf("ctl connection closed before push")
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *nodePairWorld) checkRoomFingerprint() error {
	if w.roomPeer.Key != w.nodeKey.ID() {
		return fmt.Errorf("room peer key = %q, want %q", w.roomPeer.Key, w.nodeKey.ID())
	}
	return nil
}

func (w *nodePairWorld) browserOpensCtlAndSaysHello() error {
	if err := w.openBrowserCtl(w.b1); err != nil {
		return err
	}
	return w.sendHello(w.b1.conn)
}

func (w *nodePairWorld) sendHello(conn *peer.Connection) error {
	raw, err := conn.Request(w.ctx, func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Browser", Addrs: []string{}}
	})
	if err != nil {
		return err
	}
	return proto.Decode(raw, &w.lastWelcome)
}

func (w *nodePairWorld) checkWelcomeOwner(want string) error {
	if got := fmt.Sprint(w.lastWelcome.Owner); got != want {
		return fmt.Errorf("welcome.owner = %s, want %s", got, want)
	}
	return nil
}

func (w *nodePairWorld) checkPreEnrolmentForbidden(wantCode string) error {
	rawInfo, err := w.b1.conn.Request(w.ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return err
	}
	return checkWireError(rawInfo, wantCode)
}

func (w *nodePairWorld) sendWrongSignerLog() error {
	e1 := w.signDeviceEntry(w.b1.key, w.b1.key.ID(), 1, w.b1.key.ID(), "browser", "b1")
	e2 := w.signDeviceEntry(w.b1.key, w.b1.key.ID(), 2, w.otherKey.ID(), "browser", "other")
	e3 := w.signDeviceEntry(w.otherKey, w.b1.key.ID(), 3, w.nodeKey.ID(), "node", "workstation")
	return w.b1.conn.Send(proto.Devlog{Type: "devlog", User: w.b1.key.ID(), Entries: []proto.Envelope{e1, e2, e3}})
}

func (w *nodePairWorld) checkUnownedForbidden(wantCode string) error {
	rawInfo, err := w.b1.conn.Request(w.ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return err
	}
	if err := checkWireError(rawInfo, wantCode); err != nil {
		return err
	}
	if w.logs.Owner() != "" {
		return fmt.Errorf("owner = %q, want empty", w.logs.Owner())
	}
	return nil
}

func (w *nodePairWorld) sendValidAdoptionLog() error {
	e1 := w.signDeviceEntry(w.b1.key, w.b1.key.ID(), 1, w.b1.key.ID(), "browser", "b1")
	e2 := w.signDeviceEntry(w.b1.key, w.b1.key.ID(), 2, w.nodeKey.ID(), "node", "workstation")
	return w.b1.conn.Send(proto.Devlog{Type: "devlog", User: w.b1.key.ID(), Entries: []proto.Envelope{e1, e2}})
}

func (w *nodePairWorld) checkPairCompleted() error {
	select {
	case out := <-w.pairDone:
		if out.err != nil {
			return out.err
		}
		if out.res.Fingerprint != w.nodeKey.Fingerprint() || out.res.Path == "" {
			return fmt.Errorf("pair result = %+v", out.res)
		}
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *nodePairWorld) checkOwnerSurface() error {
	rawInfo, err := w.b1.conn.Request(w.ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	if err != nil {
		return err
	}
	var info proto.HostInfo
	if err := proto.Decode(rawInfo, &info); err != nil || info.Device != w.nodeKey.ID() || info.Name != "workstation" {
		return fmt.Errorf("host.info = %+v (%v)", info, err)
	}
	return nil
}

func (w *nodePairWorld) setupOwnedWithBrowser() error {
	if err := w.setupNode(true); err != nil {
		return err
	}
	go w.b1.path.Serve(w.ctx, w.b1.peer, func(string, []byte) error { return nil })
	if err := w.openBrowserCtl(w.b1); err != nil {
		return err
	}
	if err := w.sendHello(w.b1.conn); err != nil {
		return err
	}
	_, err := w.b1.nextPush(w.ctx) // drain initial devlog push
	return err
}

func (w *nodePairWorld) sendPairBrowser(nonceTag string) error {
	raw, err := w.callPairBrowser(nonceTag)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("pair.browser %s: %v", raw, err)
	}
	w.linksByNonce[nonceTag] = res
	return nil
}

func (w *nodePairWorld) repeatPairBrowserValid(nonceTag string) error {
	raw, err := w.callPairBrowser(nonceTag)
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("pair.browser repeat %s: %v", raw, err)
	}
	w.linksByNonce[nonceTag+":repeat"] = res
	return nil
}

func (w *nodePairWorld) sendPairBrowserRaw(nonceTag string) error {
	raw, err := w.callPairBrowser(nonceTag)
	w.lastCmdRaw = raw
	return err
}

func (w *nodePairWorld) callPairBrowser(nonceTag string) (json.RawMessage, error) {
	ts := time.UnixMilli(1763600000000).UnixMilli()
	env := w.b1.key.Sign(fmt.Sprintf(`{"t":"pair.browser","n":%q,"ts":%d,"to":%q}`, exampleID("pb-"+nonceTag), ts, w.nodeKey.ID()))
	return w.b1.conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
}

func (w *nodePairWorld) checkSamePairLink() error {
	first := w.linksByNonce["first"]
	second := w.linksByNonce["first:repeat"]
	if first.Link == "" || first.Link != second.Link || first.Expires != second.Expires || first.Re == second.Re {
		return fmt.Errorf("first=%+v repeat=%+v", first, second)
	}
	return nil
}

func (w *nodePairWorld) secondBrowserPairs() error {
	sec, err := parsePairSecretLink(w.linksByNonce["first"].Link)
	if err != nil {
		return err
	}
	b2, err := w.newBrowser(exampleKey("n3-b2"))
	if err != nil {
		return err
	}
	w.b2 = b2
	if _, err := w.joinAndExchange(w.b2, sec, "b", "a"); err != nil {
		return err
	}
	if err := w.awaitKnown(w.b2.key.ID()); err != nil {
		return err
	}
	return w.openBrowserCtl(w.b2)
}

func parsePairSecretLink(link string) (identity.Secret, error) {
	_, code, ok := strings.Cut(link, "/#pair=")
	if !ok {
		return "", fmt.Errorf("missing #pair= in %q", link)
	}
	return identity.ParseSecret(code)
}

func (w *nodePairWorld) checkSecondBrowserAdopted() error {
	if err := w.sendHello(w.b2.conn); err != nil || w.lastWelcome.Owner {
		return fmt.Errorf("unexpected first welcome owner=%v err=%v", w.lastWelcome.Owner, err)
	}
	if err := w.verifyAdoptedDevlog(w.b2); err != nil {
		return err
	}
	if err := w.sendHello(w.b2.conn); err != nil || !w.lastWelcome.Owner {
		return fmt.Errorf("expected second welcome owner=true, got %v (%v)", w.lastWelcome.Owner, err)
	}
	return nil
}

func (w *nodePairWorld) verifyAdoptedDevlog(b *pairStepBrowser) error {
	rawDL, err := b.nextPush(w.ctx)
	if err != nil {
		return err
	}
	var dl proto.Devlog
	if err := proto.Decode(rawDL, &dl); err != nil {
		return err
	}
	cand, err := identity.Replay(w.b1.key.ID(), dl.Entries)
	if err != nil || !cand.Adopts(b.key.ID(), w.nodeKey.ID()) {
		return fmt.Errorf("second browser log did not adopt: %v", err)
	}
	return nil
}

func (w *nodePairWorld) advanceTenMinutes() error {
	w.mu.Lock()
	w.now = w.now.Add(10 * time.Minute)
	w.mu.Unlock()
	return nil
}

func (w *nodePairWorld) checkLastRefused(wantCode string) error {
	return checkWireError(w.lastCmdRaw, wantCode)
}

func (w *nodePairWorld) signDeviceEntry(signer identity.Key, user string, ver int, dev, kind, name string) proto.Envelope {
	p, err := json.Marshal(proto.DeviceEntry{
		PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: exampleID(fmt.Sprintf("n3-%s-%d", dev, ver)), Timestamp: 1763600000000},
		DeviceChange:  proto.DeviceChange{User: user, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
	})
	if err != nil {
		panic(err)
	}
	return signer.Sign(string(p))
}
