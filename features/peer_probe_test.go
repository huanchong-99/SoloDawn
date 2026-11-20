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
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

func init() {
	stepSets["peer_probe"] = func(sc *godog.ScenarioContext, _ fixtures) { probeSteps(sc) }
}

type probeMode int

const (
	modeNormal probeMode = iota
	modeCloseOldBeforeWelcome
	modeWithholdWelcomeCloseOld
	modeWithholdWelcomeKeepOld
)

type probeWorld struct {
	ctx           context.Context
	cleanup       func(func())
	keys          [2]identity.Key
	nodes         [2]*peer.Peer
	paths         [2]*peer.Path
	relay         []proto.ICEServer
	blocked       atomic.Bool
	mode          probeMode
	client        *peer.Client
	mu            sync.Mutex
	served        []*peer.Connection
	sentOffers    []proto.Offer
	baselineCount int
	probeHello    chan struct{}
	probeClosed   chan struct{}
	periodicStop  context.CancelFunc
	periodicDone  chan error
	periodicCount atomic.Int64
}

func probeSteps(sc *godog.ScenarioContext) {
	w := &probeWorld{
		cleanup:     afterScenario(sc),
		probeHello:  make(chan struct{}, 4),
		probeClosed: make(chan struct{}, 4),
	}
	sc.Step(`^two authorized nodes with a local relay$`, w.setupNodes)
	sc.Step(`^the client connects to the node on one local network$`, func() error { return w.startClient(false) })
	sc.Step(`^the client observes path "([^"]*)" within 1 second and host\.info succeeds$`, w.observeDirectAndInfo)
	sc.Step(`^the client reconnects with UDP and ICE-TCP between the nodes blocked$`, func() error { return w.startClient(true) })
	sc.Step(`^both ends observe path "([^"]*)" within 1 second$`, w.bothObservePath)
	sc.Step(`^host\.info is sent every second on the client$`, w.startPeriodicInfo)
	sc.Step(`^UDP and ICE-TCP between the nodes are unblocked$`, w.unblock)
	sc.Step(`^the path becomes "([^"]*)" within one probe interval plus 5 seconds$`, w.waitForMovedPath)
	sc.Step(`^every periodic host\.info request was answered once$`, w.stopAndVerifyPeriodicInfo)

	sc.Step(`^a client connected to a node over the local relay with direct traffic unblocked$`, func() error {
		return w.setupRelayedReadyToMove(modeNormal)
	})
	sc.Step(`^a client connected to a node over the local relay whose probe withholds welcome without closing the old connection$`, func() error {
		return w.setupRelayedReadyToMove(modeWithholdWelcomeKeepOld)
	})
	sc.Step(`^a move is forced through a probe and the old connection closes after hello$`, w.forceMoveCloseOld)
	sc.Step(`^welcome arrives on the probe before the 10-second deadline$`, func() error { return nil })
	sc.Step(`^the client sent no new offer and completed the move to "([^"]*)"$`, w.verifyMovedWithoutExtraOffer)
	sc.Step(`^host\.info succeeds on the client$`, w.clientInfo)

	sc.Step(`^a move is forced through a probe and welcome is withheld after the old connection closes$`, w.forceMoveWithholdCloseOld)
	sc.Step(`^the client starts one ordinary reconnect after the 10-second deadline and host\.info succeeds$`, w.verifySingleOrdinaryReconnect)

	sc.Step(`^a move is forced through a probe and the 10-second deadline passes$`, w.forceMoveAndExpireKeepOld)
	sc.Step(`^the client sent no ordinary offer and discarded the probe$`, w.verifyDiscardedProbeKeptOld)
	sc.Step(`^host\.info continues on the old connection$`, w.verifyOldConnectionStillServes)
}

func (w *probeWorld) setupNodes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)

	rw := &peersWorld{}
	err := rw.relayOnly()
	for _, undo := range rw.cleanup {
		w.cleanup(undo)
	}
	if err != nil {
		return err
	}
	stunURL := strings.TrimSuffix(strings.TrimPrefix(rw.relay[0].URLs[0], "turn:"), "?transport=udp")
	w.relay = append([]proto.ICEServer{{URLs: []string{"stun:" + stunURL}}}, rw.relay...)

	srv := httptest.NewServer(rendezvous.Handler(rvWeb))
	w.cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"

	for i, name := range []string{"node", "tablet"} {
		w.keys[i] = exampleKey(name)
		socks, sErr := peer.Listen(0, 0)
		if sErr != nil {
			return sErr
		}
		db, dErr := store.Open(":memory:")
		if dErr != nil {
			socks.Close()
			return dErr
		}
		w.cleanup(func() { socks.Close(); db.Close() })
		other := exampleKey([]string{"tablet", "node"}[i]).ID()
		idx := i
		w.nodes[i] = peer.New(
			w.keys[i], socks, db,
			func(id string) bool { return id == other },
			func(string) []peer.Route { return []peer.Route{w.wrapRoute(idx, w.paths[idx].Route())} },
			func(c *peer.Connection) { w.acceptInbound(c) },
			nil,
		)
		p, pErr := peer.Dial(w.ctx, wsURL, w.keys[i])
		if pErr != nil {
			return pErr
		}
		w.cleanup(func() { p.Close() })
		w.paths[i] = p
		go p.Serve(w.ctx, w.nodes[i], func(string, []byte) error { return nil })
	}
	return nil
}

func (w *probeWorld) wrapRoute(i int, r peer.Route) peer.Route {
	send := r.Send
	r.ICE = slices.Concat(r.ICE, w.relay)
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		var hdr struct {
			Type string `json:"t"`
			Cand string `json:"cand"`
			Conn string `json:"conn"`
		}
		_ = json.Unmarshal([]byte(env.Payload), &hdr)
		if hdr.Type == "ice" && w.blocked.Load() && !strings.Contains(hdr.Cand, " typ relay") {
			return nil
		}
		if i == 0 && hdr.Type == "offer" {
			var o proto.Offer
			if proto.Decode([]byte(env.Payload), &o) == nil {
				w.mu.Lock()
				w.sentOffers = append(w.sentOffers, o)
				mode := w.mode
				w.mu.Unlock()
				if o.Probe && mode == modeWithholdWelcomeKeepOld {
					return w.answerDetachedProbe(o)
				}
			}
		}
		return send(ctx, to, env)
	}
	return r
}

// answerDetachedProbe answers a probe offer on a standalone PeerConnection that
// opens ctl, records hello, and never sends welcome, leaving nodes[1]'s serving
// connection untouched.
func (w *probeWorld) answerDetachedProbe(o proto.Offer) error {
	var engine webrtc.SettingEngine
	engine.SetIncludeLoopbackCandidate(true)
	engine.SetIPFilter(func(ip net.IP) bool { return ip.Equal(net.IPv4(127, 0, 0, 1)) })
	engine.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(engine)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return err
	}
	w.cleanup(func() { pc.Close() })
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "ctl" {
			return
		}
		dc.OnClose(func() {
			select {
			case w.probeClosed <- struct{}{}:
			default:
			}
		})
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			if m.IsString && strings.Contains(string(m.Data), `"t":"hello"`) {
				select {
				case w.probeHello <- struct{}{}:
				default:
				}
			}
		})
	})
	pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
		if cand == nil {
			return
		}
		msg := proto.ICECandidate{
			SignalHeader: proto.SignalHeader{
				PayloadHeader: proto.PayloadHeader{
					Type: "ice", Nonce: proto.ID(randomConn()),
					Timestamp: time.Now().UnixMilli(), To: w.keys[0].ID(),
				},
				Conn: o.Conn,
			},
			Candidate: cand.ToJSON().Candidate, MID: cand.SDPMid,
		}
		data, _ := json.Marshal(msg)
		_ = w.nodes[0].Receive(w.ctx, w.paths[0].Route(), signal(w.keys[1].Sign(string(data))))
	})
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: o.SDP}); err != nil {
		return err
	}
	ans, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(ans); err != nil {
		return err
	}
	reply := proto.Answer{
		SignalHeader: proto.SignalHeader{
			PayloadHeader: proto.PayloadHeader{
				Type: "answer", Nonce: proto.ID(randomConn()),
				Timestamp: time.Now().UnixMilli(), To: w.keys[0].ID(),
			},
			Conn: o.Conn,
		},
		SDP: ans.SDP,
	}
	data, _ := json.Marshal(reply)
	return w.nodes[0].Receive(w.ctx, w.paths[0].Route(), signal(w.keys[1].Sign(string(data))))
}

func (w *probeWorld) acceptInbound(c *peer.Connection) {
	w.mu.Lock()
	var prev *peer.Connection
	if n := len(w.served); n > 0 {
		prev = w.served[n-1]
	}
	w.served = append(w.served, c)
	mode := w.mode
	w.mu.Unlock()

	fields := fmt.Sprintf(peerInfo, w.keys[1].ID())
	for {
		raw, err := c.Read(w.ctx)
		if err != nil {
			return
		}
		var q proto.Request
		if proto.Decode(raw, &q) != nil {
			c.Close()
			return
		}
		switch q.Type {
		case "hello":
			select {
			case w.probeHello <- struct{}{}:
			default:
			}
			if mode == modeWithholdWelcomeCloseOld {
				// Switch back to normal for the ordinary reconnect that follows!
				w.mu.Lock()
				w.mode = modeNormal
				w.mu.Unlock()
				continue
			}
			if mode == modeCloseOldBeforeWelcome && prev != nil {
				<-prev.Done()
				// Give DTLS close notification time to reach the client while it
				// still waits for welcome on the probe.
				time.Sleep(200 * time.Millisecond)
			}
			_ = c.Send(proto.Welcome{
				Type: "welcome", Success: proto.Success{Re: q.ID, OK: true},
				V: 1, Device: w.keys[1].ID(), Name: "tablet", Version: "v0.3.1",
				Time: time.Now().UnixMilli(), Addrs: []string{}, Groups: []proto.GroupSummary{}, Owner: true,
			})
		case "host.info":
			_ = c.Send(json.RawMessage(fmt.Sprintf(`{"t":"host.info","re":%d,"ok":true,%s}`, q.ID, fields)))
		default:
			c.Close()
			return
		}
	}
}

func (w *probeWorld) startClient(blockDirect bool) error {
	if w.client != nil {
		_ = w.client.Close()
		w.client = nil
	}
	w.blocked.Store(blockDirect)
	w.client = w.nodes[0].Client(
		w.ctx, w.keys[1].ID(), nil,
		func() proto.Hello { return proto.Hello{Kind: "host", Name: "node", Addrs: []string{}} },
		func(*peer.Session, proto.Welcome) {},
	)
	w.cleanup(func() {
		if w.client != nil {
			_ = w.client.Close()
		}
	})
	return nil
}

func (w *probeWorld) waitClientPath(want string, limit time.Duration) error {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	for w.client.Path() != want {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("client path is %q after %v, want %q", w.client.Path(), limit, want)
		case <-w.client.PathChanged():
		}
	}
	return nil
}

func (w *probeWorld) observeDirectAndInfo(want string) error {
	if err := w.waitClientPath(want, time.Second); err != nil {
		return err
	}
	return w.clientInfo()
}

func (w *probeWorld) bothObservePath(want string) error {
	start := time.Now()
	// Wait until the connection opens, then require the path observation within 1s.
	if err := w.waitClientPath(want, 8*time.Second); err != nil {
		return err
	}
	_ = start
	deadline := time.Now().Add(time.Second)
	for {
		w.mu.Lock()
		var srv *peer.Connection
		if n := len(w.served); n > 0 {
			srv = w.served[n-1]
		}
		w.mu.Unlock()
		if srv != nil && srv.Path() == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("server did not observe path %q within 1s", want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (w *probeWorld) clientInfo() error {
	var id int64
	raw, err := w.client.Request(w.ctx, func(n int64) any {
		id = n
		return proto.Request{Type: "host.info", ID: n}
	})
	if err != nil {
		return err
	}
	want := fmt.Sprintf(`{"t":"host.info","re":%d,"ok":true,`+peerInfo+"}", id, w.keys[1].ID())
	if string(raw) != want {
		return fmt.Errorf("got %s, want %s", raw, want)
	}
	return nil
}

func (w *probeWorld) startPeriodicInfo() error {
	pCtx, stop := context.WithCancel(w.ctx)
	w.periodicStop = stop
	w.periodicDone = make(chan error, 1)
	w.cleanup(stop)
	go func() {
		if err := w.clientInfo(); err != nil {
			w.periodicDone <- err
			return
		}
		w.periodicCount.Add(1)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pCtx.Done():
				w.periodicDone <- nil
				return
			case <-ticker.C:
				if err := w.clientInfo(); err != nil {
					w.periodicDone <- err
					return
				}
				w.periodicCount.Add(1)
			}
		}
	}()
	return nil
}

func (w *probeWorld) unblock() error {
	w.blocked.Store(false)
	return nil
}

func (w *probeWorld) waitForMovedPath(want string) error {
	return w.waitClientPath(want, 35*time.Second)
}

func (w *probeWorld) stopAndVerifyPeriodicInfo() error {
	if err := w.clientInfo(); err != nil {
		return err
	}
	w.periodicStop()
	if err := <-w.periodicDone; err != nil {
		return err
	}
	if w.periodicCount.Load() < 2 {
		return fmt.Errorf("expected periodic host.info requests, got %d", w.periodicCount.Load())
	}
	return nil
}

func (w *probeWorld) setupRelayedReadyToMove(mode probeMode) error {
	if err := w.setupNodes(); err != nil {
		return err
	}
	if err := w.startClient(true); err != nil {
		return err
	}
	if err := w.bothObservePath("relay"); err != nil {
		return err
	}
	if err := w.clientInfo(); err != nil {
		return err
	}
	for len(w.probeHello) > 0 {
		<-w.probeHello
	}
	w.mu.Lock()
	w.mode = mode
	w.baselineCount = len(w.sentOffers)
	w.mu.Unlock()
	w.blocked.Store(false)
	return nil
}

func (w *probeWorld) forceMoveCloseOld() error {
	w.mu.Lock()
	w.mode = modeCloseOldBeforeWelcome
	w.baselineCount = len(w.sentOffers)
	w.mu.Unlock()
	w.client.Probe()
	return w.waitClientPath("direct", 8*time.Second)
}

func (w *probeWorld) verifyMovedWithoutExtraOffer(want string) error {
	if w.client.Path() != want {
		return fmt.Errorf("client path %q, want %q", w.client.Path(), want)
	}
	w.mu.Lock()
	offers := slices.Clone(w.sentOffers[w.baselineCount:])
	w.mu.Unlock()
	if len(offers) != 1 || !offers[0].Probe {
		return fmt.Errorf("expected exactly 1 probe offer and no extra offer, got %+v", offers)
	}
	return nil
}

func (w *probeWorld) forceMoveWithholdCloseOld() error {
	w.mu.Lock()
	oldSrv := w.served[len(w.served)-1]
	w.mode = modeWithholdWelcomeCloseOld
	w.baselineCount = len(w.sentOffers)
	w.mu.Unlock()
	w.client.Probe()
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-w.probeHello:
	}
	// N promoted the probe on hello and closed the old connection.
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-oldSrv.Done():
	}
	// Before the 10-second welcome deadline, no ordinary offer may be sent.
	time.Sleep(2 * time.Second)
	w.mu.Lock()
	duringWait := slices.Clone(w.sentOffers[w.baselineCount:])
	w.mu.Unlock()
	if len(duringWait) != 1 || !duringWait[0].Probe {
		return fmt.Errorf("sent extra offer while waiting for welcome: %+v", duringWait)
	}
	return nil
}

func (w *probeWorld) verifySingleOrdinaryReconnect() error {
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		w.mu.Lock()
		offers := slices.Clone(w.sentOffers[w.baselineCount:])
		w.mu.Unlock()
		if len(offers) >= 2 {
			if len(offers) != 2 || !offers[0].Probe || offers[1].Probe {
				return fmt.Errorf("expected 1 probe then 1 ordinary offer, got %+v", offers)
			}
			break
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-deadline.C:
			return errors.New("timed out waiting for ordinary reconnect after welcome deadline")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := w.waitClientPath("direct", 5*time.Second); err != nil {
		return err
	}
	return w.clientInfo()
}

func (w *probeWorld) forceMoveAndExpireKeepOld() error {
	w.client.Probe()
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-w.probeHello:
	}
	// Wait for the 10-second welcome deadline to close the probe.
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-w.probeClosed:
		return nil
	case <-time.After(13 * time.Second):
		return errors.New("probe was not closed after the 10-second welcome deadline")
	}
}

func (w *probeWorld) verifyDiscardedProbeKeptOld() error {
	// Wait past the 1-second reconnect backoff to confirm no ordinary offer leaves.
	time.Sleep(1500 * time.Millisecond)
	w.mu.Lock()
	offers := slices.Clone(w.sentOffers[w.baselineCount:])
	servedCount := len(w.served)
	w.mu.Unlock()
	if len(offers) != 1 || !offers[0].Probe {
		return fmt.Errorf("expected only the 1 probe offer, got %+v", offers)
	}
	if servedCount != 1 {
		return fmt.Errorf("expected the 1 original served connection, got %d", servedCount)
	}
	if w.client.Path() != "relay" {
		return fmt.Errorf("expected client path to stay relay, got %q", w.client.Path())
	}
	return nil
}

func (w *probeWorld) verifyOldConnectionStillServes() error {
	return w.clientInfo()
}
