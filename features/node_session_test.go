//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["node_session"] = func(sc *godog.ScenarioContext, _ fixtures) { nodeSessionSteps(sc) }
}

type nodeSessionWorld struct {
	ctx        context.Context
	cleanup    func(func())
	now        time.Time
	ice        []proto.ICEServer
	db         *store.Store
	logs       *node.Logs
	sessions   *node.Sessions
	nodeKey    identity.Key
	nodePeer   *peer.Peer
	pairingDev string
	mu         sync.Mutex
	routes     map[string]*peer.Peer

	b1Key, otherKey identity.Key
	b1Conn          *peer.Connection
	lastWelcome     proto.Welcome
	cmdRuns         int
}

func nodeSessionSteps(sc *godog.ScenarioContext) {
	w := &nodeSessionWorld{
		cleanup: afterScenario(sc),
		now:     time.UnixMilli(1763600000000),
		routes:  make(map[string]*peer.Peer),
	}
	sc.Step(`^an unowned node and a pairing browser connected over ctl$`, w.setupUnownedPairing)
	sc.Step(`^the browser sends hello before any device log arrives$`, func() error { return w.sendHello(w.b1Conn) })
	sc.Step(`^welcome reports owner (true|false)$`, w.checkWelcomeOwner)
	sc.Step(`^host\.info and host commands on the pairing connection are refused with "([^"]*)"$`, w.checkPairingForbidden)
	sc.Step(`^the browser sends a device log whose add entry for the node was signed by another device$`, w.sendWrongSignerLog)
	sc.Step(`^the node still has no owner and refuses host\.info with "([^"]*)"$`, w.checkStillUnowned)
	sc.Step(`^the browser sends a device log whose add entry for the node is signed by the pairing browser$`, w.sendValidAdoptionLog)
	sc.Step(`^the browser sends hello again$`, func() error { return w.sendHello(w.b1Conn) })
	sc.Step(`^host\.info and a host command succeed with a stored receipt$`, w.checkAdoptedSurfaceAndReceipt)
}

func (w *nodeSessionWorld) initRelayAndNode() error {
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
	w.nodeKey = exampleKey("n2-node")
	w.b1Key = exampleKey("n2-browser-1")
	w.otherKey = exampleKey("n2-other")
	if err := w.initNodeServices(); err != nil {
		return err
	}
	return w.startNodePeer()
}

func (w *nodeSessionWorld) initNodeServices() error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db

	logs, err := node.OpenLogs(db, w.nodeKey.ID())
	if err != nil {
		return err
	}
	w.logs = logs
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: w.nodeKey.ID(), Owner: logs.Owner(),
		Logs: logs.Log, Tables: home.NodeTables(), Now: func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	w.cleanup(loop.Close)
	h, _, err := host.New(db, w.nodeKey, func() time.Time { return w.now })
	if err != nil {
		return err
	}
	w.sessions = node.NewSessions(node.SessionConfig{
		ID: w.nodeKey.ID(), Version: "v0.3.1", Store: db, Logs: logs, Home: loop, Host: h,
		Name: func() string { return "workstation" },
		Now:  func() time.Time { return w.now },
	})
	w.sessions.Requests["host.info"] = node.RequestHandler{
		Type: "host.info",
		Run: func(context.Context, *home.Conn, node.Ctl, string, int64, json.RawMessage) (any, error) {
			return map[string]any{"device": w.nodeKey.ID(), "name": "workstation"}, nil
		},
	}
	w.sessions.Commands["settings.set"] = func(context.Context, node.Ctl, string, json.RawMessage) (any, error) {
		w.mu.Lock()
		w.cmdRuns++
		w.mu.Unlock()
		return nil, nil
	}
	return nil
}

func (w *nodeSessionWorld) startNodePeer() error {
	w.pairingDev = w.b1Key.ID()
	nodePeer, err := w.makePeer(w.nodeKey, func(dev string) bool {
		w.mu.Lock()
		logs, pDev := w.logs, w.pairingDev
		w.mu.Unlock()
		return logs.Known(dev) || (pDev != "" && dev == pDev)
	}, func(c *peer.Connection) {
		w.mu.Lock()
		sessions, pDev := w.sessions, w.pairingDev
		w.mu.Unlock()
		go sessions.Serve(w.ctx, c.Device, pDev != "" && c.Device == pDev, c)
	})
	if err != nil {
		return err
	}
	w.nodePeer = nodePeer
	return nil
}

func (w *nodeSessionWorld) makePeer(key identity.Key, known func(string) bool, inbound func(*peer.Connection)) (*peer.Peer, error) {
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
	paths := func(string) []peer.Route {
		return []peer.Route{{ID: "loopback-relay", ICE: w.ice, Send: func(ctx context.Context, to string, env proto.Envelope) error {
			return w.deliverSignal(ctx, key.ID(), to, env)
		}}}
	}
	p := peer.New(key, socks, db, known, paths, inbound, nil)
	w.mu.Lock()
	w.routes[key.ID()] = p
	w.mu.Unlock()
	return p, nil
}

func (w *nodeSessionWorld) deliverSignal(ctx context.Context, from, to string, env proto.Envelope) error {
	var sig struct {
		Type string `json:"t"`
		Cand string `json:"cand"`
	}
	if json.Unmarshal([]byte(env.Payload), &sig) == nil && sig.Type == "ice" && !isRelayCand(sig.Cand) {
		return nil
	}
	w.mu.Lock()
	target := w.routes[to]
	w.mu.Unlock()
	rawEnv, err := json.Marshal(env)
	if err != nil {
		return err
	}
	route := peer.Route{ID: "loopback-relay", ICE: w.ice, Send: func(ctx context.Context, backTo string, ans proto.Envelope) error {
		return w.deliverSignal(ctx, to, backTo, ans)
	}}
	return target.Receive(ctx, route, proto.Route{Type: "from", Key: from, Env: rawEnv})
}

func isRelayCand(cand string) bool {
	return len(cand) >= 10 && containsRelay(cand)
}

func containsRelay(s string) bool {
	for i := 0; i+len(" typ relay") <= len(s); i++ {
		if s[i:i+len(" typ relay")] == " typ relay" {
			return true
		}
	}
	return false
}

func (w *nodeSessionWorld) dialNode(clientKey identity.Key) (*peer.Peer, *peer.Connection, error) {
	cp, err := w.makePeer(clientKey, func(id string) bool { return id == w.nodeKey.ID() }, func(*peer.Connection) {})
	if err != nil {
		return nil, nil, err
	}
	conn, err := cp.Offer(w.ctx, w.nodeKey.ID())
	if err != nil {
		return nil, nil, err
	}
	w.cleanup(func() { conn.Close() })
	if _, err := recvCtx(w.ctx, conn.Ready(), "the connection to be ready", nil); err != nil {
		return nil, nil, err
	}
	go drainBrowserConn(w.ctx, conn)
	return cp, conn, nil
}

func drainBrowserConn(ctx context.Context, conn *peer.Connection) {
	for {
		if _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (w *nodeSessionWorld) setupUnownedPairing() error {
	if err := w.initRelayAndNode(); err != nil {
		return err
	}
	_, conn, err := w.dialNode(w.b1Key)
	w.b1Conn = conn
	return err
}

func (w *nodeSessionWorld) sendHello(conn *peer.Connection) error {
	welc, err := requestBrowserHello(w.ctx, conn)
	w.lastWelcome = welc
	return err
}

func (w *nodeSessionWorld) checkWelcomeOwner(want string) error {
	if got := fmt.Sprint(w.lastWelcome.Owner); got != want {
		return fmt.Errorf("welcome.owner = %s, want %s", got, want)
	}
	return nil
}

func (w *nodeSessionWorld) checkPairingForbidden(wantCode string) error {
	if err := w.expectRequestError(w.b1Conn, "host.info", wantCode); err != nil {
		return err
	}
	env := w.b1Key.Sign(fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q,"offline":true}`, exampleID("pair-cmd"), w.now.UnixMilli(), w.nodeKey.ID()))
	raw, err := requestWait(w.ctx, w.b1Conn, nil, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func (w *nodeSessionWorld) sendWrongSignerLog() error {
	ts := w.now.UnixMilli()
	e1 := nodetest.SignDeviceAdd(w.b1Key, w.b1Key.ID(), 1, "browser", "b1", w.b1Key.ID(), ts)
	e2 := nodetest.SignDeviceAdd(w.b1Key, w.b1Key.ID(), 2, "browser", "other", w.otherKey.ID(), ts)
	e3 := nodetest.SignDeviceAdd(w.otherKey, w.b1Key.ID(), 3, "node", "workstation", w.nodeKey.ID(), ts)
	return w.b1Conn.Send(proto.Devlog{Type: "devlog", User: w.b1Key.ID(), Entries: []proto.Envelope{e1, e2, e3}})
}

func (w *nodeSessionWorld) checkStillUnowned(wantCode string) error {
	if err := w.expectRequestError(w.b1Conn, "host.info", wantCode); err != nil {
		return err
	}
	if w.logs.Owner() != "" {
		return fmt.Errorf("owner = %q, want empty", w.logs.Owner())
	}
	return nil
}

func (w *nodeSessionWorld) sendValidAdoptionLog() error {
	ts := w.now.UnixMilli()
	e1 := nodetest.SignDeviceAdd(w.b1Key, w.b1Key.ID(), 1, "browser", "b1", w.b1Key.ID(), ts)
	e2 := nodetest.SignDeviceAdd(w.b1Key, w.b1Key.ID(), 2, "node", "workstation", w.nodeKey.ID(), ts)
	return w.b1Conn.Send(proto.Devlog{Type: "devlog", User: w.b1Key.ID(), Entries: []proto.Envelope{e1, e2}})
}

func (w *nodeSessionWorld) checkAdoptedSurfaceAndReceipt() error {
	w.mu.Lock()
	w.pairingDev = ""
	w.mu.Unlock()
	if err := w.verifyAdoptedHostInfo(); err != nil {
		return err
	}
	return w.verifyIdempotentSettingsCmd()
}

func (w *nodeSessionWorld) verifyAdoptedHostInfo() error {
	rawInfo, err := requestWait(w.ctx, w.b1Conn, nil, func(id int64) any {
		return proto.Request{Type: "host.info", ID: id}
	})
	if err != nil {
		return err
	}
	var ans struct {
		Type string     `json:"t"`
		OK   proto.True `json:"ok"`
	}
	if err := proto.Decode(rawInfo, &ans); err != nil || ans.Type != "host.info" {
		return fmt.Errorf("host.info = %s (%v)", rawInfo, err)
	}
	return nil
}

func (w *nodeSessionWorld) verifyIdempotentSettingsCmd() error {
	env := w.b1Key.Sign(fmt.Sprintf(`{"t":"settings.set","n":%q,"ts":%d,"to":%q,"offline":true}`, exampleID("set-1"), w.now.UnixMilli(), w.nodeKey.ID()))
	for range 2 {
		rawCmd, err := requestWait(w.ctx, w.b1Conn, nil, func(id int64) any {
			return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
		})
		if err != nil {
			return err
		}
		var ok proto.Success
		if err := proto.Decode(rawCmd, &ok); err != nil {
			return fmt.Errorf("cmd = %s (%v)", rawCmd, err)
		}
	}
	w.mu.Lock()
	runs := w.cmdRuns
	w.mu.Unlock()
	if runs != 1 {
		return fmt.Errorf("host command ran %d times, want 1", runs)
	}
	return nil
}

func (w *nodeSessionWorld) expectRequestError(conn *peer.Connection, typ, wantCode string) error {
	raw, err := requestWait(w.ctx, conn, nil, func(id int64) any {
		return proto.Request{Type: typ, ID: id}
	})
	if err != nil {
		return err
	}
	return checkWireError(raw, wantCode)
}

func checkWireError(raw json.RawMessage, wantCode string) error {
	var er proto.ErrorResponse
	if err := proto.Decode(raw, &er); err != nil {
		return fmt.Errorf("decode error %s: %v", raw, err)
	}
	if er.Err.Code != wantCode {
		return fmt.Errorf("got code %q, want %q", er.Err.Code, wantCode)
	}
	return nil
}
