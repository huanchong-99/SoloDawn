//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
)

// Node-level host information is a fixture; the requests, answers, pushes,
// signatures, signalling sockets, relay and data channels here are real.
const peerInfo = `"device":"%s","name":"test node","os":"test","version":"1","open":0,"max_open":4,"offline":false,"update":null,"homes":[],"relay":{"used":0,"limit":0},"agents":[],"settings":{"retention_days":90,"idle_minutes":15,"default_level":2,"all_level3":false,"approvers":[]}`

// peersWorld is one scenario: the node (0) and the tablet (1).
type peersWorld struct {
	ctx      context.Context
	cancel   context.CancelFunc
	keys     [2]identity.Key
	nodes    [2]*peer.Peer
	accepted [2]chan *peer.Connection // inbound connections, in the order ctl opened
	conns    [2]*peer.Connection      // what each side offered last
	pushes   [2]chan json.RawMessage  // pushes read from conns[i], single reader: pushBoth
	served   [2]*peer.Connection      // the inbound connection each side accepted last
	relay    []proto.ICEServer
	blocked  bool // only relay candidates cross
	mu       sync.Mutex
	shift    int64                     // added to the ts of the node's next offer in transit
	dated    map[string]proto.Envelope // the node's offers as they left it, by conn
	offers   []proto.Envelope          // the same, in order
	local    [2]chan proto.Envelope
	replies  []chan proto.Envelope // what each replay path is sent
	first    [2]proto.Envelope     // the candidate of each side delivered before its description
	cleanup  []func()
}

func init() {
	stepSets["peer"] = func(sc *godog.ScenarioContext, _ fixtures) { peerSteps(sc) }
}

func peerSteps(sc *godog.ScenarioContext) {
	w := &peersWorld{dated: map[string]proto.Envelope{}}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		for _, undo := range slices.Backward(w.cleanup) {
			undo()
		}
		return ctx, nil
	})
	sc.Step("^two authorized nodes signed in on two local signalling paths$", w.signedIn)
	sc.Step("^two authorized nodes signed in on that rendezvous$", w.signedInShared)
	sc.Step("^two authorized nodes whose signalling the test delivers$", w.delivered)
	sc.Step("^each node offers the other a connection$", func() error { return errors.Join(w.offer(0), w.offer(1)) })
	sc.Step("^both directions are direct and answer host.info$", w.directBoth)
	sc.Step("^an ordered ctl push follows the reply in each direction$", w.pushBoth)

	sc.Step("^a relay on this machine, with UDP and ICE-TCP between the nodes blocked$", w.relayOnly)
	sc.Step("^the node offers the tablet a connection$", func() error { return w.offer(0) })
	sc.Step("^both ends report the path relay and the connection answers host.info$", w.relayed)
	sc.Step(`^host\.info on that connection succeeds every second$`, w.pollInfoEverySecond)

	sc.Step("^the node offers the tablet a connection dated 119 seconds ahead$", func() error { return w.dateOffer(119000) })
	sc.Step("^the node offers the tablet a newer connection$", w.newerOffer)
	sc.Step("^the tablet receives the first offer again$", func() error { return w.replay(0, 1) })
	sc.Step("^the tablet answers nothing and the newer connection answers host.info$", func() error { return w.replied(answeredNothing) })
	sc.Step("^the tablet receives the newer offer again on two other paths$", func() error { return w.replay(1, 2) })
	sc.Step("^each of those paths gets the same answer and the newer connection answers host.info$", func() error { return w.replied(answeredAlike) })

	sc.Step("^two copies of a candidate reach each side before the description it belongs to$", w.early)
	sc.Step("^the connection opens and answers host.info$", w.opened)
	sc.Step("^another copy of each of those candidates arrives$", w.again)
	sc.Step("^the connection answers host.info$", func() error { return w.info(0) })
}

// start makes both nodes; routes gives each node its signalling paths.
func (w *peersWorld) start(routes func(i int) []peer.Route) error {
	w.ctx, w.cancel = context.WithTimeout(context.Background(), 20*time.Second)
	w.cleanup = append(w.cleanup, w.cancel)
	for i, name := range []string{"node", "tablet"} {
		w.keys[i] = exampleKey(name)
		w.accepted[i] = make(chan *peer.Connection, 4)
		socks, err := peer.Listen(0, 0)
		if err != nil {
			return err
		}
		db, err := store.Open(":memory:")
		if err != nil {
			return err
		}
		w.cleanup = append(w.cleanup, func() { socks.Close(); db.Close() })
		other := exampleKey([]string{"tablet", "node"}[i]).ID()
		w.nodes[i] = peer.New(w.keys[i], socks, db, func(id string) bool { return id == other }, func(string) []peer.Route { return routes(i) }, func(c *peer.Connection) { w.accepted[i] <- c; w.serve(c, i) }, nil)
	}
	return nil
}

// signedIn signs both nodes in on two rendezvous instances.
func (w *peersWorld) signedIn() error {
	var paths [2][]*peer.Path
	err := w.start(func(i int) []peer.Route {
		var routes []peer.Route
		for _, p := range paths[i] {
			routes = append(routes, w.route(i, p.Route()))
		}
		return routes
	})
	if err != nil {
		return err
	}
	for range 2 {
		srv := httptest.NewServer(rendezvous.Handler(rvWeb))
		w.cleanup = append(w.cleanup, srv.Close)
		for i := range w.keys {
			p, err := peer.Dial(w.ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/rv", w.keys[i])
			if err != nil {
				return err
			}
			w.cleanup = append(w.cleanup, func() { p.Close() })
			paths[i] = append(paths[i], p)
			go p.Serve(w.ctx, w.nodes[i], func(string, []byte) error { return nil })
		}
	}
	return nil
}

func (w *peersWorld) signedInShared(ctx context.Context) error {
	var paths [2]*peer.Path
	err := w.start(func(i int) []peer.Route {
		return []peer.Route{w.route(i, paths[i].Route())}
	})
	if err != nil {
		return err
	}
	rvURL := sharedRVURL(ctx)
	for i := range w.keys {
		p, err := peer.Dial(w.ctx, rvURL, w.keys[i])
		if err != nil {
			return err
		}
		w.cleanup = append(w.cleanup, func() { p.Close() })
		paths[i] = p
		go p.Serve(w.ctx, w.nodes[i], func(string, []byte) error { return nil })
	}
	return nil
}

// route passes what node i sends through the conditions of the scenario.
func (w *peersWorld) route(i int, r peer.Route) peer.Route {
	send := r.Send
	r = w.withRouteICE(r)
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		var p struct {
			Type string `json:"t"`
			Cand string `json:"cand"`
		}
		json.Unmarshal([]byte(env.Payload), &p)
		if p.Type == "ice" && w.isBlocked() && !strings.Contains(p.Cand, " typ relay") {
			return nil
		}
		if p.Type == "offer" && i == 0 {
			env = w.sent(env)
		}
		return send(ctx, to, env)
	}
	return r
}

func (w *peersWorld) withRouteICE(r peer.Route) peer.Route {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.blocked {
		r.ICE = w.relay
		return r
	}
	r.ICE = rewriteDirectICE(r.ICE)
	return r
}

func (w *peersWorld) isBlocked() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.blocked
}

// sent signs the node's offer again with its ts moved by shift, and records
// it. Every copy of one offer leaves the same.
func (w *peersWorld) sent(env proto.Envelope) proto.Envelope {
	var o proto.Offer
	json.Unmarshal([]byte(env.Payload), &o)
	w.mu.Lock()
	defer w.mu.Unlock()
	if e, ok := w.dated[o.Conn]; ok {
		return e
	}
	o.Timestamp += w.shift
	data, _ := json.Marshal(o)
	env = w.keys[0].Sign(string(data))
	w.dated[o.Conn] = env
	w.offers = append(w.offers, env)
	return env
}

// serve answers every host.info request and pushes host.info once after the first answer.
func (w *peersWorld) serve(c *peer.Connection, i int) {
	fields := fmt.Sprintf(peerInfo, w.keys[i].ID())
	for n := 0; ; n++ {
		data, err := c.Read(w.ctx)
		if err != nil {
			return
		}
		var q proto.Request
		if proto.Decode(data, &q) != nil || q.Type != "host.info" {
			c.Close()
			return
		}
		c.Send(json.RawMessage(fmt.Sprintf(`{"t":"host.info","re":%d,"ok":true,%s}`, q.ID, fields)))
		if n == 0 {
			c.Send(json.RawMessage(`{"t":"host.info",` + fields + "}"))
		}
	}
}

// offer has node i offer the other node a connection.
func (w *peersWorld) offer(i int) error {
	c, err := w.nodes[i].Offer(w.ctx, w.keys[1-i].ID())
	if err != nil {
		return err
	}
	ch := make(chan json.RawMessage, 16) // single reader: pushBoth
	w.conns[i] = c
	w.pushes[i] = ch
	w.cleanup = append(w.cleanup, func() { c.Close() })
	go pumpBrowserPushes(w.ctx, c, ch)
	return nil
}

// open waits until ctl of what node i offered is open at both ends.
func (w *peersWorld) open(i int) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-w.conns[i].Ready():
	}
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case w.served[1-i] = <-w.accepted[1-i]:
		return nil
	}
}

// info sends host.info on what node i offered and checks the answer.
func (w *peersWorld) info(i int) error {
	var id int64
	data, err := w.conns[i].Request(w.ctx, func(n int64) any { id = n; return proto.Request{Type: "host.info", ID: n} })
	if err != nil {
		return err
	}
	want := fmt.Sprintf(`{"t":"host.info","re":%d,"ok":true,`+peerInfo+"}", id, w.keys[1-i].ID())
	if string(data) != want {
		return fmt.Errorf("answer: %s, want %s", data, want)
	}
	return nil
}
func (w *peersWorld) directBoth() error {
	for i, c := range w.conns {
		if err := w.open(i); err != nil {
			return err
		}
		if err := waitConnPath(c, "direct", time.Second); err != nil {
			return err
		}
		if err := w.info(i); err != nil {
			return err
		}
	}
	return nil
}
func (w *peersWorld) pushBoth() error {
	for i, ch := range w.pushes {
		var data json.RawMessage
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case data = <-ch:
		}
		if want := fmt.Sprintf(`{"t":"host.info",`+peerInfo+"}", w.keys[1-i].ID()); string(data) != want {
			return fmt.Errorf("push: %s, want %s", data, want)
		}
	}
	return nil
}

// relayOnly starts a TURN server on 127.0.0.1 whose relay sockets take
// datagrams only from relay sockets, and passes no other candidate between
// the nodes: UDP and ICE-TCP between them are blocked.
func (w *peersWorld) relayOnly() error {
	w.closePriorConns()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	ip := net.IPv4(127, 0, 0, 1)
	gen := &relaySockets{RelayAddressGeneratorStatic: turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()}, ports: map[int]bool{}}
	s, err := turn.NewServer(turn.ServerConfig{Realm: "test", AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "test", "secret"), r.Username == "peer"
	}, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: l, RelayAddressGenerator: gen}}})
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { s.Close() })
	w.mu.Lock()
	w.relay = []proto.ICEServer{{URLs: []string{"turn:" + l.LocalAddr().String() + "?transport=udp"}, Username: "peer", Credential: "secret"}}
	w.blocked = true
	w.mu.Unlock()
	return nil
}

func (w *peersWorld) closePriorConns() {
	for i, c := range w.conns {
		if c != nil {
			c.Close()
			w.conns[i] = nil
		}
	}
	for i, c := range w.served {
		if c != nil {
			c.Close()
			w.served[i] = nil
		}
	}
}

type relaySockets struct {
	turn.RelayAddressGeneratorStatic
	mu    sync.Mutex
	ports map[int]bool
}

func (g *relaySockets) AllocatePacketConn(conf turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	conn, addr, err := g.RelayAddressGeneratorStatic.AllocatePacketConn(conf)
	if err != nil {
		return nil, nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ports[addr.(*net.UDPAddr).Port] = true
	return relayConn{conn, g}, addr, nil
}
func (g *relaySockets) relayed(addr net.Addr) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ports[addr.(*net.UDPAddr).Port]
}

type relayConn struct {
	net.PacketConn
	g *relaySockets
}

func (c relayConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || c.g.relayed(addr) {
			return n, addr, err
		}
	}
}

func (w *peersWorld) relayed() error {
	if err := w.open(0); err != nil {
		return err
	}
	for _, c := range []*peer.Connection{w.conns[0], w.served[1]} {
		if err := waitConnPath(c, "relay", time.Second); err != nil {
			return err
		}
	}
	return w.info(0)
}

func (w *peersWorld) pollInfoEverySecond() error {
	for range 2 {
		time.Sleep(time.Second)
		if err := w.info(0); err != nil {
			return err
		}
	}
	return nil
}

// dateOffer opens a connection whose offer is dated shift milliseconds ahead.
func (w *peersWorld) dateOffer(shift int64) error {
	w.mu.Lock()
	w.shift = shift
	w.mu.Unlock()
	return errors.Join(w.offer(0), w.open(0))
}

// newerOffer opens a second connection; the tablet closes the first.
func (w *peersWorld) newerOffer() error {
	first := w.conns[0]
	if err := w.dateOffer(0); err != nil {
		return err
	}
	select {
	case <-w.ctx.Done():
		return errors.New("the first connection stayed open")
	case <-first.Done():
		return nil
	}
}

// replay gives the tablet the node's offer number n again on copies paths of
// node-local delivery; each path keeps what it is sent.
func (w *peersWorld) replay(n, copies int) error {
	w.mu.Lock()
	offer := w.offers[n]
	w.mu.Unlock()
	w.replies = nil
	for range copies {
		got := make(chan proto.Envelope, 2)
		path := peer.Route{ID: "replay", Send: func(_ context.Context, _ string, env proto.Envelope) error { got <- env; return nil }}
		if err := w.nodes[1].Receive(w.ctx, path, signal(offer)); err != nil {
			return err
		}
		w.replies = append(w.replies, got)
	}
	return nil
}

// answers is what each replay path was sent. The node sends an answer from a
// goroutine of its own after Receive returns, so a path that has nothing is
// given a second.
func (w *peersWorld) answers() [][]proto.Envelope {
	var all [][]proto.Envelope
	for _, got := range w.replies {
		var envs []proto.Envelope
		select {
		case env := <-got:
			envs = append(envs, env)
		case <-time.After(time.Second):
		}
		all = append(all, append(envs, drain(got)...))
	}
	return all
}
func drain(got chan proto.Envelope) []proto.Envelope {
	var envs []proto.Envelope
	for len(got) > 0 {
		envs = append(envs, <-got)
	}
	return envs
}

// replied checks what the replay paths were sent, that the tablet made no
// connection for them, and that the newer connection still works.
func (w *peersWorld) replied(want func([][]proto.Envelope) bool) error {
	if sent := w.answers(); !want(sent) {
		return fmt.Errorf("the replay paths were sent %v", sent)
	}
	if n := len(w.accepted[1]); n != 0 {
		return fmt.Errorf("%d more connections", n)
	}
	return w.info(0)
}
func answeredNothing(r [][]proto.Envelope) bool { return len(r) == 1 && len(r[0]) == 0 }
func answeredAlike(r [][]proto.Envelope) bool {
	return len(r) == 2 && len(r[0]) == 1 && slices.Equal(r[0], r[1])
}
func signal(env proto.Envelope) proto.Route {
	data, _ := json.Marshal(env)
	return proto.Route{Type: "from", Key: env.Key, Env: data}
}

// delivered makes both nodes send all signalling to the test, which delivers
// it in the order a step chooses.
func (w *peersWorld) delivered() error {
	for i := range w.local {
		w.local[i] = make(chan proto.Envelope, 256)
	}
	return w.start(func(i int) []peer.Route { return []peer.Route{w.localRoute(i)} })
}
func (w *peersWorld) localRoute(i int) peer.Route {
	return peer.Route{ID: "local", Send: func(ctx context.Context, _ string, env proto.Envelope) error {
		select {
		case w.local[i] <- env:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
}

// early delivers, for each side, two copies of one of its candidates before
// its description, and everything else it sends after that.
func (w *peersWorld) early() error {
	for i := range 2 {
		held, err := w.hold(i)
		if err != nil {
			return err
		}
		w.first[i] = held[0]
		for _, env := range slices.Concat(held[:1], held) {
			if err := w.nodes[1-i].Receive(w.ctx, w.localRoute(1-i), signal(env)); err != nil {
				return err
			}
		}
		go w.pump(i)
	}
	return nil
}

// hold collects what node i sent until it has a description and a candidate;
// the result starts with a candidate and ends with the rest.
func (w *peersWorld) hold(i int) ([]proto.Envelope, error) {
	var candidates, rest []proto.Envelope
	for len(candidates) == 0 || len(rest) == 0 {
		select {
		case <-w.ctx.Done():
			return nil, w.ctx.Err()
		case env := <-w.local[i]:
			if strings.Contains(env.Payload, `"t":"ice"`) {
				candidates = append(candidates, env)
			} else {
				rest = append(rest, env)
			}
		}
	}
	return slices.Concat(candidates[:1], rest, candidates[1:]), nil
}

// pump delivers what node i sends from now on to the other node.
func (w *peersWorld) pump(i int) {
	for {
		select {
		case <-w.ctx.Done():
			return
		case env := <-w.local[i]:
			w.nodes[1-i].Receive(w.ctx, w.localRoute(1-i), signal(env))
		}
	}
}
func (w *peersWorld) opened() error {
	if err := w.open(0); err != nil {
		return err
	}
	return w.info(0)
}

// again delivers one more copy of each early candidate.
func (w *peersWorld) again() error {
	for i, env := range w.first {
		if err := w.nodes[1-i].Receive(w.ctx, w.localRoute(1-i), signal(env)); err != nil {
			return err
		}
	}
	return nil
}
