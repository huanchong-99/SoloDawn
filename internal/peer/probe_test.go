package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

func reqCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testHello() proto.Hello {
	return proto.Hello{Kind: "host", Name: "test-host"}
}

func testWelcome(re int64, device string) proto.Welcome {
	return proto.Welcome{
		Type: "welcome", Success: proto.Success{Re: re, OK: true},
		V: 1, Device: device, Name: "home", Version: "v0.3.1", Time: 1763600000000,
		Addrs: []string{}, Groups: []proto.GroupSummary{}, Owner: true,
	}
}

// TestProbeICEStripsTURN verifies that probes omit TURN on both ends while
// keeping STUN, and still surface malformed URLs.
func TestProbeICEStripsTURN(t *testing.T) {
	routes := []Route{
		{ICE: []proto.ICEServer{
			{URLs: []string{"stun:127.0.0.1:3478", "turn:127.0.0.1:3478?transport=udp"}, Username: "u", Credential: "c"},
			{URLs: []string{"turns:127.0.0.1:443?transport=tcp"}, Username: "u", Credential: "c"},
		}},
		{ICE: []proto.ICEServer{{URLs: []string{"stun:127.0.0.1:3479"}}}},
	}
	full := peerICE(routes, false)
	eq(t, len(full.ICEServers), 3)
	direct := peerICE(routes, true)
	eq(t, direct.ICEServers, []webrtc.ICEServer{
		{URLs: []string{"stun:127.0.0.1:3478"}},
		{URLs: []string{"stun:127.0.0.1:3479"}},
	})
	bad := peerICE([]Route{{ICE: []proto.ICEServer{{URLs: []string{"bogus"}}}}}, true)
	eq(t, bad.ICEServers, []webrtc.ICEServer{{URLs: []string{"bogus"}}})

	f := newPair(t)
	f.a.paths = func(string) []Route {
		r := f.aMail.route()
		r.ICE = []proto.ICEServer{{URLs: []string{"bogus"}}}
		return []Route{r}
	}
	_, err := f.a.offerProbe(t.Context(), f.b.key.ID())
	eq(t, err == nil, false)

	f2 := newPair(t)
	relay := localRelay(t)
	f2.a.paths = func(string) []Route { r := f2.aMail.route(); r.ICE = []proto.ICEServer{relay}; return []Route{r} }
	f2.b.paths = func(string) []Route { r := f2.bMail.route(); r.ICE = []proto.ICEServer{relay}; return []Route{r} }
	pr, err := f2.a.offerProbe(t.Context(), f2.b.key.ID())
	check(t, err)
	t.Cleanup(func() { pr.Close() })
	offer := recv(t, f2.aMail["offer"])
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	eq(t, o.Probe, true)
	deliver(t, f2.b, f2.bMail.route(), offer)
	answer := recv(t, f2.bMail["answer"])
	deliver(t, f2.a, f2.aMail.route(), answer)
	_, aCand := hostUDP(t, f2.aMail, o.Conn)
	_, bCand := hostUDP(t, f2.bMail, o.Conn)
	eq(t, strings.Contains(aCand.Candidate, "typ relay"), false)
	eq(t, strings.Contains(bCand.Candidate, "typ relay"), false)
	for _, m := range []mailbox{f2.aMail, f2.bMail} {
		for len(m["ice"]) > 0 {
			env := <-m["ice"]
			eq(t, strings.Contains(env.Payload, "typ relay"), false)
		}
	}
}

// dialProbe negotiates a probe from f.a to f.b over loopback UDP and returns
// the client and server *Connection once ctl is open on both ends.
func (f *pair) dialProbe(t *testing.T) (*Connection, *Connection, proto.Envelope, proto.Envelope) {
	t.Helper()
	c, err := f.a.offerProbe(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	offer := recv(t, f.aMail["offer"])
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	eq(t, o.Probe, true)
	deliver(t, f.b, f.bMail.route(), offer)
	answer := recv(t, f.bMail["answer"])
	deliver(t, f.a, f.aMail.route(), answer)
	li, _ := hostUDP(t, f.aMail, o.Conn)
	ri, _ := hostUDP(t, f.bMail, o.Conn)
	deliver(t, f.b, f.bMail.route(), li)
	deliver(t, f.a, f.aMail.route(), ri)
	recv(t, c.Ready())
	f.b.mu.Lock()
	srv := f.b.live[link{f.a.key.ID(), o.Conn}]
	f.b.mu.Unlock()
	t.Cleanup(func() { srv.Close() })
	recv(t, srv.Ready())
	return c, srv, offer, answer
}

// TestInboundProbePromotionAndRaces covers inbound probe slot isolation,
// supersession by a second probe or an ordinary offer, pre-promotion frame
// checks, and promotion on the first valid hello.
func TestInboundProbePromotionAndRaces(t *testing.T) {
	f := newPair(t)
	handed := make(chan *Stream, 2)
	f.b.channels = Channels{"test:": func(s *Stream) { handed <- s }}
	first := f.open(t, 0)
	first.info(t)

	// A probe with open ctl leaves Peer.slot untouched until hello.
	p1Client, p1Server, p1Offer, p1Answer := f.dialProbe(t)
	dup := mail()
	deliver(t, f.b, dup.route(), p1Offer)
	eq(t, recv(t, dup["answer"]), p1Answer)
	f.b.mu.Lock()
	eq(t, f.b.slot[f.a.key.ID()], first.server)
	eq(t, f.b.probes[f.a.key.ID()], p1Server)
	f.b.mu.Unlock()
	eq(t, len(f.opened), 0)
	first.info(t)

	// Non-ctl channels opened before promotion are closed.
	earlyExtra := openRemote(t, p1Client, "test:before")
	recv(t, earlyExtra.closed)
	eq(t, len(handed), 0)

	// A second probe supersedes p1 while leaving first.server in Peer.slot.
	p2Client, p2Server, _, _ := f.dialProbe(t)
	recv(t, p1Server.Done())
	recv(t, p1Client.Done())
	deliver(t, f.b, dup.route(), p1Offer)
	eq(t, len(dup["answer"]), 0)
	f.b.mu.Lock()
	eq(t, f.b.slot[f.a.key.ID()], first.server)
	eq(t, f.b.probes[f.a.key.ID()], p2Server)
	f.b.mu.Unlock()
	first.info(t)

	// Binary frames (even containing valid hello JSON), non-hello requests,
	// bad hello kinds, and unsupported versions leave p2 open and unpromoted.
	check(t, p2Client.dc.Send([]byte(`{"t":"hello","id":77,"v":1,"kind":"host","name":"n","addrs":[]}`)))
	for _, tmpl := range []string{
		`{"t":"host.info","id":%d}`,
		`{"t":"hello","id":%d,"re":1,"v":1,"kind":"host","name":"n","addrs":[]}`,
		`{"t":"hello","id":%d,"v":1,"kind":"other","name":"n","addrs":[]}`,
		`{"t":"other","id":%d,"v":1,"kind":"host","name":"n","addrs":[]}`,
		`{"t":"hello","id":%d,"v":1,"kind":"host","name":"n","addrs":null}`,
	} {
		raw, err := p2Client.Request(reqCtx(t), func(id int64) any { return json.RawMessage(fmt.Sprintf(tmpl, id)) })
		check(t, err)
		var e proto.ErrorResponse
		check(t, proto.Decode(raw, &e))
		eq(t, e.Err.Code, "bad_request")
	}
	check(t, p2Client.Send(proto.RVFrame{Type: "devlog"}))
	for _, ver := range []int{0, 2} {
		raw, err := p2Client.Request(reqCtx(t), func(id int64) any {
			return json.RawMessage(fmt.Sprintf(`{"t":"hello","id":%d,"v":%d,"kind":"host","name":"n","addrs":[]}`, id, ver))
		})
		check(t, err)
		var verErr proto.ErrorResponse
		check(t, proto.Decode(raw, &verErr))
		eq(t, verErr.Err.Code, "version")
	}
	eq(t, len(f.opened), 0)
	first.info(t)

	// Valid hello promotes p2, closes first.server, and hands p2 to inbound.
	helloSent := make(chan error, 1)
	go func() {
		reply, reqErr := p2Client.Request(t.Context(), func(id int64) any {
			return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "tab", Addrs: []string{}}
		})
		if reqErr == nil {
			var w proto.Welcome
			reqErr = proto.Decode(reply, &w)
		}
		helloSent <- reqErr
	}()
	eq(t, recv(t, f.opened), p2Server)
	recv(t, first.server.Done())
	msg, err := p2Server.Read(t.Context())
	check(t, err)
	var gotHello proto.Hello
	check(t, proto.Decode(msg, &gotHello))
	eq(t, gotHello.Kind, "browser")
	check(t, p2Server.Send(testWelcome(gotHello.ID, f.b.key.ID())))
	check(t, recv(t, helloSent))

	// After promotion, registered channels are handed over and repeated hello
	// frames pass through deliver without calling inbound again.
	laterExtra := openRemote(t, p2Client, "test:after")
	s := recv(t, handed)
	eq(t, s.Label(), "test:after")
	recv(t, laterExtra.open)
	check(t, p2Client.Send(proto.Hello{Request: proto.Request{Type: "hello", ID: 99}, V: 1, Kind: "host", Name: "n", Addrs: []string{}}))
	msg, err = p2Server.Read(t.Context())
	check(t, err)
	check(t, proto.Decode(msg, &gotHello))
	eq(t, gotHello.ID, int64(99))
	eq(t, len(f.opened), 0)

	// An ordinary offer closes both Peer.slot and any pending probe.
	_, p3Server, _, _ := f.dialProbe(t)
	second := f.open(t, 0)
	recv(t, p2Server.Done())
	recv(t, p3Server.Done())
	eq(t, f.b.promote(p3Server), false)
	f.b.mu.Lock()
	eq(t, f.b.slot[f.a.key.ID()], second.server)
	eq(t, len(f.b.probes), 0)
	f.b.mu.Unlock()
	second.info(t)
}

// TestInboundProbeEdgeRejections covers refused probe SDP, ctl size limits on
// validHelloPacket, promote/unpromotedCtl operand combinations, unordered or
// duplicate ctl channels, and signer revocation before hello.
func TestInboundProbeEdgeRejections(t *testing.T) {
	// Exact ctlLimit boundary on validHelloPacket (61,440 passes, 61,441 fails).
	base := `{"t":"hello","id":1,"v":1,"kind":"host","name":"`
	tail := `","addrs":[]}`
	exact := []byte(base + strings.Repeat("a", ctlLimit-len(base)-len(tail)) + tail)
	eq(t, len(exact), ctlLimit)
	_, ok := validHelloPacket(exact)
	eq(t, ok, true)
	over := []byte(base + strings.Repeat("a", ctlLimit+1-len(base)-len(tail)) + tail)
	eq(t, len(over), ctlLimit+1)
	_, ok = validHelloPacket(over)
	eq(t, ok, false)
	eq(t, validHelloKind(proto.Hello{Request: proto.Request{Type: "other"}, Kind: "host"}), false)
	eq(t, validHelloKind(proto.Hello{Request: proto.Request{Type: "hello"}, Kind: "other"}), false)

	f := newPair(t)
	badOffer := f.a.sign(proto.Offer{
		SignalHeader: f.a.header(link{f.b.key.ID(), randomID()}, "offer"),
		SDP:          probeSDP, Probe: true,
	})
	eq(t, f.b.Receive(t.Context(), Route{}, frame(badOffer)), identity.BadRequest)
	f.b.mu.Lock()
	eq(t, len(f.b.slot), 0)
	eq(t, len(f.b.probes), 0)
	f.b.mu.Unlock()

	// Both operands of promote: (not in probes, open) and (in probes, closed).
	openCtx, openCancel := context.WithCancel(t.Context())
	openUnregistered := &Connection{Device: f.a.key.ID(), ctx: openCtx, close: func() error { openCancel(); return nil }}
	eq(t, f.b.promote(openUnregistered), false)
	// unpromotedCtl with known == true and promote == false closes the connection.
	eq(t, f.b.unpromotedCtl(openUnregistered, exact), false)
	recv(t, openUnregistered.Done())

	closedCtx, closedCancel := context.WithCancel(t.Context())
	closedCancel()
	closedProbe := &Connection{Device: f.a.key.ID(), ctx: closedCtx, close: func() error { return nil }}
	f.b.probes[f.a.key.ID()] = closedProbe
	eq(t, f.b.promote(closedProbe), false)
	delete(f.b.probes, f.a.key.ID())

	// Promotion when Peer.slot has no predecessor.
	soloClient, soloServer, _, _ := f.dialProbe(t)
	check(t, soloClient.Send(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "host", Name: "n", Addrs: []string{}}))
	eq(t, recv(t, f.opened), soloServer)
	check(t, soloServer.Close())
	check(t, soloClient.Close())
	recv(t, soloServer.Done())

	// Closing an unpromoted probe removes it from Peer.probes via forgetProbe.
	dropClient, dropServer, _, _ := f.dialProbe(t)
	check(t, dropClient.Close())
	check(t, dropServer.Close())
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.b.mu.Lock()
		nProbes := len(f.b.probes)
		f.b.mu.Unlock()
		if nProbes == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("forgetProbe did not remove closed probe")
		}
		time.Sleep(time.Millisecond)
	}

	rawProbe := func(inits ...*webrtc.DataChannelInit) ([]*webrtc.DataChannel, *Connection) {
		pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(loopbackEngine())).NewPeerConnection(webrtc.Configuration{})
		check(t, err)
		t.Cleanup(func() { pc.Close() })
		var chans []*webrtc.DataChannel
		for _, init := range inits {
			dc, dcErr := pc.CreateDataChannel("ctl", init)
			check(t, dcErr)
			chans = append(chans, dc)
		}
		k := link{f.a.key.ID(), randomID()}
		to := link{f.b.key.ID(), k.conn}
		pc.OnICECandidate(func(i *webrtc.ICECandidate) {
			if i != nil {
				f.b.Receive(t.Context(), Route{}, frame(f.a.sign(proto.ICECandidate{SignalHeader: f.a.header(to, "ice"), Candidate: i.ToJSON().Candidate, MID: i.SDPMid})))
			}
		})
		offer, err := pc.CreateOffer(nil)
		check(t, err)
		check(t, pc.SetLocalDescription(offer))
		deliver(t, f.b, f.bMail.route(), f.a.sign(proto.Offer{SignalHeader: f.a.header(to, "offer"), SDP: offer.SDP, Probe: true}))
		var a proto.Answer
		check(t, proto.Decode([]byte(recv(t, f.bMail["answer"]).Payload), &a))
		check(t, pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: a.SDP}))
		_, r := hostUDP(t, f.bMail, k.conn)
		check(t, pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: r.Candidate, SDPMid: &r.MID}))
		f.b.mu.Lock()
		defer f.b.mu.Unlock()
		return chans, f.b.live[k]
	}
	unordered := false
	_, rejected := rawProbe(&webrtc.DataChannelInit{Ordered: &unordered})
	recv(t, rejected.Done())

	chans, dupCtl := rawProbe(nil, nil)
	dupClosed := make(chan struct{})
	chans[1].OnClose(func() { close(dupClosed) })
	recv(t, dupCtl.Ready())
	recv(t, dupClosed)
	dupCtl.mu.Lock()
	firstDC := dupCtl.dc
	dupCtl.mu.Unlock()
	eq(t, dupCtl.ownsCtl(firstDC), true)
	eq(t, dupCtl.ownsCtl(chans[1]), false)

	// Revoking the signer before hello arrives closes the probe without promoting.
	var allowed atomic.Bool
	allowed.Store(true)
	f.b.known = func(id string) bool { return id == f.a.key.ID() && allowed.Load() }
	cur := f.open(t, 0)
	prClient, prServer, _, _ := f.dialProbe(t)
	allowed.Store(false)
	check(t, prClient.Send(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "host", Name: "n", Addrs: []string{}}))
	recv(t, prServer.Done())
	eq(t, len(f.opened), 0)
	cur.info(t)
}

// TestOfferProbeDeadlinesAndSendFailure covers offerProbe broadcast errors,
// context cancellation, and all three cases of watchProbeOpen.
func TestOfferProbeDeadlinesAndSendFailure(t *testing.T) {
	f := newPair(t)
	f.a.paths = func(string) []Route { return nil }
	_, err := f.a.offerProbe(t.Context(), f.b.key.ID())
	eq(t, err.Error(), "no signalling path")

	fCancel := newPair(t)
	sending := make(chan struct{}, 1)
	fCancel.a.paths = func(string) []Route {
		return []Route{{Send: func(ctx context.Context, _ string, _ proto.Envelope) error {
			notify(sending)
			<-ctx.Done()
			return ctx.Err()
		}}}
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-sending; cancel() }()
	_, err = fCancel.a.offerProbe(ctx, fCancel.b.key.ID())
	eq(t, err, context.Canceled)

	synctest.Test(t, func(t *testing.T) {
		mk := func() (*Connection, chan struct{}, context.CancelFunc) {
			cCtx, cCancel := context.WithCancel(context.Background())
			ready := make(chan struct{})
			c := &Connection{ctx: cCtx, ready: ready, close: func() error { cCancel(); return nil }}
			return c, ready, cCancel
		}
		c1, _, _ := mk()
		go watchProbeOpen(c1, probeOpenLimit)
		time.Sleep(probeOpenLimit - time.Nanosecond)
		synctest.Wait()
		eq(t, c1.isClosed(), false)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		eq(t, c1.isClosed(), true)

		c2, ready2, cancel2 := mk()
		defer cancel2()
		go watchProbeOpen(c2, probeOpenLimit)
		close(ready2)
		time.Sleep(time.Hour)
		synctest.Wait()
		eq(t, c2.isClosed(), false)

		c3, _, cancel3 := mk()
		go watchProbeOpen(c3, probeOpenLimit)
		cancel3()
		synctest.Wait()
		eq(t, c3.isClosed(), true)
	})
}

// TestSessionPathChangedDoesNotCompeteWithServe verifies F1: a host-side reader
// of Session.PathChanged and a node-side reader of Client.PathChanged each
// observe every path change alongside the Client's own serve loop.
func TestSessionPathChangedDoesNotCompeteWithServe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var pathVal atomic.Value
		pathVal.Store("direct")
		cCtx, cCancel := context.WithCancel(context.Background())
		defer cCancel()
		srvConn := &Connection{
			ctx: cCtx, ready: make(chan struct{}), pathChanged: make(chan struct{}, 1),
			incoming: make(chan json.RawMessage, 4), waiting: make(map[int64]chan json.RawMessage),
			close: func() error { cCancel(); return nil },
		}
		cl := &Client{
			device: "dev", hello: testHello, onWelcome: func(*Session, proto.Welcome) {},
			probe:      func(context.Context, string) (*Connection, error) { return nil, errors.New("unused") },
			pathOf:     func(*Connection) string { return pathVal.Load().(string) },
			firstProbe: probeFirstWait, ctx: cCtx, cancel: cCancel,
			done: make(chan struct{}), pathChanged: make(chan struct{}, 1),
		}
		s := newSession(cl, srvConn)
		cl.current = s
		done := make(chan time.Duration, 1)
		go func() { done <- cl.serve(s, time.Now()) }()
		synctest.Wait()

		var sessionSeen, clientSeen atomic.Int32
		go func() {
			for {
				select {
				case <-s.Done():
					return
				case <-s.PathChanged():
					sessionSeen.Add(1)
				}
			}
		}()
		go func() {
			for {
				select {
				case <-cCtx.Done():
					return
				case <-cl.PathChanged():
					clientSeen.Add(1)
				}
			}
		}()
		synctest.Wait()

		steps := []string{"relay", "direct", "relay", "direct", "relay"}
		for i, next := range steps {
			pathVal.Store(next)
			srvConn.reclassify()
			synctest.Wait()
			eq(t, int(sessionSeen.Load()), i+1)
			eq(t, int(clientSeen.Load()), i+1)
			eq(t, s.Path(), next)
			eq(t, cl.Path(), next)
		}
		cCancel()
		synctest.Wait()
		<-done
	})
}

// TestEndToEndClientProbeMoveAndRequests exercises Client and Session over real
// WebRTC connections and a local TURN relay: initial relayed path, probe move,
// FIFO replay of unanswered non-sync requests without duplicating in-flight
// answered ones, sync isolation on both Session.Request and Client.Request,
// exact 60 KiB ctl frame boundaries, and retirement behavior.
func TestEndToEndClientProbeMoveAndRequests(t *testing.T) {
	fCheck := newPair(t)
	fCheck.a.paths = func(string) []Route { return nil }
	chkCtx, chkCancel := context.WithCancel(t.Context())
	chkCancel()
	defaultCl := fCheck.a.Client(chkCtx, fCheck.b.key.ID(), nil, testHello, func(*Session, proto.Welcome) {})
	eq(t, defaultCl.firstProbe, probeFirstWait)
	check(t, defaultCl.Close())

	f := newPair(t)
	relay := localRelay(t)
	var blocked atomic.Bool
	blocked.Store(true)
	wrapRoute := func(m mailbox) func(string) []Route {
		return func(string) []Route {
			base := m.route()
			send := base.Send
			return []Route{{
				ID:  "rv",
				ICE: []proto.ICEServer{relay},
				Send: func(ctx context.Context, to string, env proto.Envelope) error {
					if blocked.Load() && strings.Contains(env.Payload, `"t":"ice"`) && !strings.Contains(env.Payload, " typ relay") {
						return nil
					}
					return send(ctx, to, env)
				},
			}}
		}
	}
	f.a.paths = wrapRoute(f.aMail)
	f.b.paths = wrapRoute(f.bMail)

	pumpCtx, stopPump := context.WithCancel(t.Context())
	defer stopPump()
	pump := func(from mailbox, to *Peer, toMail mailbox) {
		for _, kind := range []string{"offer", "answer", "ice"} {
			go func(ch chan proto.Envelope) {
				for {
					select {
					case <-pumpCtx.Done():
						return
					case env := <-ch:
						_ = to.Receive(pumpCtx, toMail.route(), frame(env))
					}
				}
			}(from[kind])
		}
	}
	pump(f.aMail, f.b, f.bMail)
	pump(f.bMail, f.a, f.aMail)

	var holdFirst atomic.Bool
	holdFirst.Store(true)
	var syncDropsLeft atomic.Int32
	syncDropsLeft.Store(2)
	var fastCount atomic.Int32
	heldArrived := make(chan struct{}, 1)
	serverConns := make(chan *Connection, 8)
	var orderMu sync.Mutex
	var holdIDs []int64
	f.b.inbound = func(srv *Connection) {
		serverConns <- srv
		go func() {
			for {
				raw, err := srv.Read(pumpCtx)
				if err != nil {
					return
				}
				var q proto.Request
				if proto.Decode(raw, &q) != nil {
					continue
				}
				switch q.Type {
				case "hello":
					_ = srv.Send(testWelcome(q.ID, f.b.key.ID()))
				case "hold":
					orderMu.Lock()
					holdIDs = append(holdIDs, q.ID)
					orderMu.Unlock()
					if holdFirst.CompareAndSwap(true, false) {
						notify(heldArrived)
						continue
					}
					_ = srv.Send(proto.Success{Re: q.ID, OK: true})
				case "fast":
					fastCount.Add(1)
					_ = srv.Send(proto.Success{Re: q.ID, OK: true})
				case "sync":
					if syncDropsLeft.Add(-1) < 0 {
						_ = srv.Send(proto.Success{Re: q.ID, OK: true})
					}
				case "host.info":
					_ = srv.Send(proto.Success{Re: q.ID, OK: true})
				}
			}
		}()
	}

	sessions := make(chan *Session, 4)
	cl := newClient(
		t.Context(), f.b.key.ID(), nil, testHello,
		func(s *Session, _ proto.Welcome) { sessions <- s },
		f.a.Offer, f.a.offerProbe, (*Connection).Path, 50*time.Millisecond,
	)
	t.Cleanup(func() { cl.Close() })

	s1 := recv(t, sessions)
	srv1 := recv(t, serverConns)
	for range 32 {
		if cl.Path() == "relay" {
			break
		}
		recv(t, cl.PathChanged())
	}
	eq(t, cl.Path(), "relay")
	eq(t, s1.Path(), "relay")

	blockingReadErr := make(chan error, 1)
	go func() {
		_, rErr := s1.Read(t.Context())
		blockingReadErr <- rErr
	}()
	time.Sleep(20 * time.Millisecond)

	// Start sync on both Session.Request and Client.Request, plus a held non-sync
	// request ("hold"); while "hold" is active on s1, send "fast" so takeUnsent
	// sees hold.active != nil && !hold.active.isClosed() and does not duplicate it.
	sessSyncErr := make(chan error, 1)
	go func() {
		_, err := s1.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
		sessSyncErr <- err
	}()
	clientSyncErr := make(chan error, 1)
	go func() {
		_, err := cl.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
		clientSyncErr <- err
	}()
	holdRes := make(chan error, 1)
	go func() {
		_, err := s1.Request(t.Context(), func(id int64) any { return proto.Request{Type: "hold", ID: id} })
		holdRes <- err
	}()
	recv(t, heldArrived)

	fastReply, err := cl.Request(t.Context(), func(id int64) any { return proto.Request{Type: "fast", ID: id} })
	check(t, err)
	eq(t, strings.Contains(string(fastReply), `"ok":true`), true)
	eq(t, int(fastCount.Load()), 1)
	orderMu.Lock()
	eq(t, len(holdIDs), 1)
	orderMu.Unlock()

	// Unblock direct candidates; the next scheduled probe moves to s2.
	blocked.Store(false)

	s2 := recv(t, sessions)
	srv2 := recv(t, serverConns)
	eq(t, srv2 == srv1, false)
	recv(t, srv1.Done())
	recv(t, s1.Done())
	eq(t, recv(t, blockingReadErr), io.EOF)
	eq(t, recv(t, sessSyncErr), io.EOF)
	eq(t, recv(t, clientSyncErr), io.EOF)
	check(t, recv(t, holdRes))
	orderMu.Lock()
	eq(t, len(holdIDs), 2)
	orderMu.Unlock()

	// Completed sync succeeds on both Session.Request and Client.Request without replay.
	for _, fn := range []func(context.Context, func(int64) any) (json.RawMessage, error){s2.Request, cl.Request} {
		syncReply, sErr := fn(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
		check(t, sErr)
		eq(t, strings.Contains(string(syncReply), `"ok":true`), true)
	}

	// Retired s1 refuses Read and Send with io.EOF despite queued messages.
	s1.c.queue(json.RawMessage(`{"t":"rows"}`))
	_, err = s1.Read(t.Context())
	eq(t, err, io.EOF)
	eq(t, s1.Send(proto.RVFrame{Type: "ack"}), io.EOF)

	for range 32 {
		if cl.Path() == "direct" {
			break
		}
		recv(t, cl.PathChanged())
	}
	eq(t, cl.Path(), "direct")
	check(t, cl.Send(proto.RVFrame{Type: "push"}))
	check(t, srvPush(t, s2))

	// Exact 61,440-byte request passes preflightReq and succeeds; 61,441 fails.
	sizedFrame := func(target int) func(int64) any {
		probe := map[string]any{"t": "host.info", "id": int64(math.MaxInt64), "pad": ""}
		padLen := target - len(encode(probe))
		pad := strings.Repeat("x", padLen)
		return func(id int64) any {
			if id == math.MaxInt64 {
				return map[string]any{"t": "host.info", "id": id, "pad": pad}
			}
			return proto.Request{Type: "host.info", ID: id}
		}
	}
	eq(t, len(encode(sizedFrame(ctlLimit)(math.MaxInt64))), ctlLimit)
	reply, err := cl.Request(t.Context(), sizedFrame(ctlLimit))
	check(t, err)
	var okRes proto.Success
	check(t, proto.Decode(reply, &okRes))
	eq(t, bool(okRes.OK), true)

	eq(t, len(encode(sizedFrame(ctlLimit+1)(math.MaxInt64))), ctlLimit+1)
	_, err = cl.Request(t.Context(), sizedFrame(ctlLimit+1))
	eq(t, err.Error(), "ctl frame exceeds 60 KiB")
	_, err = s2.Request(t.Context(), func(id int64) any {
		return map[string]any{"t": "sync", "id": id, "pad": strings.Repeat("x", ctlLimit)}
	})
	eq(t, err.Error(), "ctl frame exceeds 60 KiB")

	cCtx, cCancel := context.WithCancel(t.Context())
	cCancel()
	_, err = cl.Request(cCtx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	eq(t, err, context.Canceled)
	_, err = s2.Request(cCtx, func(id int64) any { return proto.Request{Type: "sync", ID: id} })
	eq(t, err, context.Canceled)

	check(t, cl.Close())
	recv(t, cl.Done())
	eq(t, cl.Path(), "")
	eq(t, cl.Send(proto.RVFrame{Type: "push"}), io.EOF)
	_, err = cl.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	eq(t, err, io.EOF)
	_, err = cl.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
	eq(t, err, io.EOF)
}

func srvPush(t *testing.T, s *Session) error {
	t.Helper()
	s.c.queue(json.RawMessage(`{"t":"pushed"}`))
	msg, err := s.Read(t.Context())
	if err != nil {
		return err
	}
	eq(t, string(msg), `{"t":"pushed"}`)
	cCtx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.Read(cCtx)
	eq(t, err, context.Canceled)
	return nil
}

// TestClientTimingsAndMoveOutcomes tests the probe timer schedule in synctest:
//   - reclassify at +20s does not postpone the 30s first probe;
//   - reclassify at +32s while probe #1 is pending does not arm a premature timer;
//   - probe intervals double 30s -> 1m -> 2m -> 4m -> 5m -> 5m while relayed;
//   - switching to direct stops probes and resets the wait to 30s;
//   - closing the ordinary connection while a probe is pending closes the probe.
func TestClientTimingsAndMoveOutcomes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var probeTimes []time.Time
		var lastProbe *Connection
		countProbes := func() int {
			mu.Lock()
			defer mu.Unlock()
			return len(probeTimes)
		}
		var pathVal atomic.Value
		pathVal.Store("relay")
		cCtx, cCancel := context.WithCancel(context.Background())
		defer cCancel()
		srvConn := &Connection{
			ctx: cCtx, ready: make(chan struct{}), pathChanged: make(chan struct{}, 1),
			incoming: make(chan json.RawMessage, 4), waiting: make(map[int64]chan json.RawMessage),
			close: func() error { cCancel(); return nil },
		}
		var failOffer atomic.Bool
		cl := &Client{
			device: "dev", hello: testHello, onWelcome: func(*Session, proto.Welcome) {},
			pathOf:     func(*Connection) string { return pathVal.Load().(string) },
			firstProbe: probeFirstWait, ctx: cCtx, cancel: cCancel,
			done: make(chan struct{}), pathChanged: make(chan struct{}, 1),
		}
		cl.probe = func(context.Context, string) (*Connection, error) {
			mu.Lock()
			probeTimes = append(probeTimes, time.Now())
			n := len(probeTimes)
			mu.Unlock()
			if n > 10 {
				cCancel()
				t.Errorf("unexpected probe count %d", n)
				return nil, errors.New("probe limit exceeded")
			}
			if failOffer.Load() {
				return nil, errors.New("no path")
			}
			pCtx, pCancel := context.WithCancel(context.Background())
			pConn := &Connection{ctx: pCtx, ready: make(chan struct{}), close: func() error { pCancel(); return nil }}
			mu.Lock()
			lastProbe = pConn
			mu.Unlock()
			time.AfterFunc(5*time.Second, pCancel)
			return pConn, nil
		}
		s := newSession(cl, srvConn)
		cl.current = s
		start := time.Now()
		done := make(chan time.Duration, 1)
		go func() { done <- cl.serve(s, start) }()
		synctest.Wait()

		// Trickled candidate at +20s while relayed must not postpone the 30s probe.
		time.Sleep(20 * time.Second)
		srvConn.reclassify()
		synctest.Wait()
		eq(t, countProbes(), 0)

		want := []time.Duration{
			30 * time.Second,
			95 * time.Second,
			220 * time.Second,
			465 * time.Second,
			770 * time.Second,
			1075 * time.Second,
		}
		for i, target := range want {
			if i == 1 {
				// At +32s (2s into probe #1), reclassify while pending != nil must not arm st.timer.
				time.Sleep(2 * time.Second)
				srvConn.reclassify()
				synctest.Wait()
				eq(t, countProbes(), 1)
			}
			if i == 5 {
				failOffer.Store(true)
			}
			wait := time.Until(start.Add(target))
			time.Sleep(wait - time.Nanosecond)
			synctest.Wait()
			eq(t, countProbes(), i)
			time.Sleep(time.Nanosecond)
			synctest.Wait()
			eq(t, countProbes(), i+1)
		}

		// Direct path stops scheduled probes.
		pathVal.Store("direct")
		srvConn.reclassify()
		synctest.Wait()
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		eq(t, countProbes(), 6)

		// Returning to relay starts the schedule fresh from 30s.
		failOffer.Store(false)
		pathVal.Store("relay")
		srvConn.reclassify()
		synctest.Wait()
		time.Sleep(30*time.Second - time.Nanosecond)
		synctest.Wait()
		eq(t, countProbes(), 6)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		eq(t, countProbes(), 7)

		// Closing the ordinary connection while probe #7 is pending closes the probe.
		time.Sleep(2 * time.Second)
		srvConn.Close()
		synctest.Wait()
		mu.Lock()
		p7Closed := lastProbe.isClosed()
		mu.Unlock()
		eq(t, p7Closed, true)
		openDur := <-done
		if openDur < 10*time.Minute {
			t.Fatalf("unexpected open duration %v", openDur)
		}
	})
}

// TestClientErrorAndCancelBranches covers openSession failures, sayHello
// refusals, Session retirement while underlying Connection is open, flushOne
// Send failure, sync cancellation mid-flight, and request queue boundaries.
func TestClientErrorAndCancelBranches(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	unopenedDC, err := pc.CreateDataChannel("ctl", nil)
	check(t, err)

	// 1. openSession: offer error, Done before Ready, and refused hello.
	var step atomic.Int32
	wake := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(t.Context())
	cl := newClient(ctx, "dev", wake, func() proto.Hello { return proto.Hello{V: 0, Kind: "host", Name: "h"} },
		func(*Session, proto.Welcome) {},
		func(context.Context, string) (*Connection, error) {
			switch step.Add(1) {
			case 1:
				wake <- struct{}{}
				return nil, errors.New("offer failed")
			case 2:
				wake <- struct{}{}
				cCtx, cCancel := context.WithCancel(t.Context())
				cCancel()
				return &Connection{ctx: cCtx, ready: make(chan struct{}), close: func() error { return nil }}, nil
			default:
				cCtx, cCancel := context.WithCancel(t.Context())
				ready := make(chan struct{})
				close(ready)
				c := &Connection{
					ctx: cCtx, ready: ready, dc: unopenedDC,
					waiting: make(map[int64]chan json.RawMessage),
					close:   func() error { cCancel(); return nil },
				}
				cancel()
				return c, nil
			}
		},
		func(context.Context, string) (*Connection, error) { return nil, errors.New("unused") },
		func(*Connection) string { return "direct" },
		probeFirstWait,
	)
	recv(t, cl.Done())

	// 2. sayHello and decodeWelcome branches on real connections.
	for _, replyPayload := range []any{
		proto.ErrorResponse{Re: 1, Err: proto.WireError{Code: "version", Message: "bad"}},
		proto.Welcome{Type: "other", Success: proto.Success{Re: 1, OK: true}, V: 1, Device: "d", Name: "n", Version: "1", Time: 1, Addrs: []string{}, Groups: []proto.GroupSummary{}},
		proto.Welcome{Type: "welcome", Success: proto.Success{Re: 1, OK: true}, V: 2, Device: "d", Name: "n", Version: "1", Time: 1, Addrs: []string{}, Groups: []proto.GroupSummary{}},
	} {
		p := newPair(t).open(t, 0)
		go func(ans any) {
			raw, rErr := p.server.Read(t.Context())
			if rErr == nil {
				var q proto.Request
				_ = proto.Decode(raw, &q)
				switch v := ans.(type) {
				case proto.ErrorResponse:
					v.Re = q.ID
					_ = p.server.Send(v)
				case proto.Welcome:
					v.Re = q.ID
					_ = p.server.Send(v)
				}
			}
		}(replyPayload)
		_, ok := sayHello(t.Context(), p.client, testHello)
		eq(t, ok, false)
		eq(t, p.client.isClosed(), true)
	}

	// 3. Retired Session while s.c is still open refuses Read, Send, and sync Request.
	f := newPair(t)
	n := f.open(t, 0)
	cl2Ctx, cl2Cancel := context.WithCancel(t.Context())
	defer cl2Cancel()
	cl2 := &Client{
		device: f.b.key.ID(), hello: testHello, onWelcome: func(*Session, proto.Welcome) {},
		pathOf: (*Connection).Path, firstProbe: probeFirstWait, ctx: cl2Ctx, cancel: cl2Cancel,
		done: make(chan struct{}), pathChanged: make(chan struct{}, 1),
	}
	retiredSess := newSession(cl2, n.client)
	n.client.queue(json.RawMessage(`{"t":"buffered"}`))
	retiredSess.retire()
	for range 4 {
		_, err = retiredSess.Read(t.Context())
		eq(t, err, io.EOF)
	}
	eq(t, retiredSess.Send(proto.RVFrame{Type: "push"}), io.EOF)
	_, err = retiredSess.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
	eq(t, err, io.EOF)
	// Drain buffered message on n.client so later tests start clean.
	_, err = n.client.Read(t.Context())
	check(t, err)

	// 4. flushOne Send failure closes brokenConn and adopt replays req on n.client.
	c2Ctx, c2Cancel := context.WithCancel(t.Context())
	defer c2Cancel()
	brokenConn := &Connection{
		ctx: c2Ctx, ready: make(chan struct{}), pathChanged: make(chan struct{}, 1),
		dc: unopenedDC, waiting: make(map[int64]chan json.RawMessage),
		close: sync.OnceValue(func() error { c2Cancel(); return nil }),
	}
	cl2.current = newSession(cl2, brokenConn)
	context.AfterFunc(c2Ctx, func() {
		cl2.adopt(n.client, testWelcome(1, f.b.key.ID()))
	})
	go func() {
		raw, rErr := n.server.Read(t.Context())
		if rErr == nil {
			var q proto.Request
			_ = proto.Decode(raw, &q)
			eq(t, q.Type, "host.info")
			_ = n.server.Send(proto.Success{Re: q.ID, OK: true})
		}
	}()
	reply, err := cl2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	check(t, err)
	eq(t, strings.Contains(string(reply), `"ok":true`), true)

	// 5. Cancel sync Request mid-flight returns context.Canceled (not io.EOF),
	//    and cancel non-sync Request mid-flight wakes awaitReq via req.done.
	syncCtx, syncCancel := context.WithCancel(t.Context())
	syncCh := make(chan error, 1)
	go func() {
		_, sErr := cl2.current.Request(syncCtx, func(id int64) any { return proto.Request{Type: "sync", ID: id} })
		syncCh <- sErr
	}()
	_, err = n.server.Read(t.Context())
	check(t, err)
	syncCancel()
	eq(t, recv(t, syncCh), context.Canceled)

	reqCtx, reqCancel := context.WithCancel(t.Context())
	reqErrCh := make(chan error, 1)
	go func() {
		_, rErr := cl2.Request(reqCtx, func(id int64) any { return proto.Request{Type: "unanswered", ID: id} })
		reqErrCh <- rErr
	}()
	_, err = n.server.Read(t.Context())
	check(t, err)
	reqCancel()
	eq(t, recv(t, reqErrCh), context.Canceled)

	// 6. takeUnsent when current == nil and when current.c is closed;
	//    and stopClient finishing queued requests with io.EOF when cl2Ctx ends.
	closedSubCtx, closedSubCancel := context.WithCancel(t.Context())
	closedSubCancel()
	closedConn := &Connection{ctx: closedSubCtx, close: func() error { return nil }}
	cl2.mu.Lock()
	cl2.current = newSession(cl2, closedConn)
	cl2.mu.Unlock()
	eofCh := make(chan error, 1)
	go func() {
		_, rErr := cl2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "queued", ID: id} })
		eofCh <- rErr
	}()
	waitLimit := time.Now().Add(2 * time.Second)
	for {
		cl2.mu.Lock()
		nPending := len(cl2.pending)
		var active *Connection
		if nPending == 1 {
			active = cl2.pending[0].active
		}
		cl2.mu.Unlock()
		if nPending == 1 {
			eq(t, active == nil, true)
			break
		}
		if time.Now().After(waitLimit) {
			t.Fatal("request was not queued on closed current connection")
		}
		time.Sleep(time.Millisecond)
	}
	// Cancel cl2Ctx while cl2.current != nil: enqueueReq now refuses new requests.
	cl2Cancel()
	eq(t, recv(t, eofCh), io.EOF)
	_, ok := cl2.enqueueReq(func(id int64) any { return proto.Request{Type: "after-cancel", ID: id} })
	eq(t, ok, false)
	cl2.retireCurrent()
	cl2.flushPending()
	cl2.finishReq(&pendingReq{done: make(chan struct{}), reply: make(chan reqResult, 1)}, reqResult{err: io.EOF})

	// 7. buildHello, isSync, and decodeWelcome boundary checks.
	eq(t, isSync(func(int64) any { return "not-an-object" }), false)
	eq(t, isSync(func(id int64) any { return proto.Request{Type: "host.info", ID: id} }), false)
	eq(t, isSync(func(id int64) any { return proto.Request{Type: "sync", ID: id} }), true)
	eq(t, buildHello(proto.Hello{Kind: "host", Name: "n"}, 7), proto.Hello{
		Request: proto.Request{Type: "hello", ID: 7}, V: 1, Kind: "host", Name: "n", Addrs: []string{},
	})
	eq(t, buildHello(proto.Hello{V: 2, Kind: "host", Name: "n", Addrs: []string{"ws://127.0.0.1:47800/rv"}}, 8), proto.Hello{
		Request: proto.Request{Type: "hello", ID: 8}, V: 2, Kind: "host", Name: "n", Addrs: []string{"ws://127.0.0.1:47800/rv"},
	})
	_, ok = decodeWelcome(encode(proto.Welcome{Type: "welcome", Success: proto.Success{Re: 1, OK: true}, V: 0, Device: "d", Name: "n", Version: "1", Time: 1, Addrs: []string{}, Groups: []proto.GroupSummary{}}))
	eq(t, ok, false)
}

// TestA113MoveBoundaries tests all move outcomes on Client.move:
//  1. Old closes during hello wait, welcome arrives in time -> move succeeds;
//  2. Probe fails while old stays open -> probe discarded, old continues and schedules next probe;
//  3. Probe fails after old closed -> move ends serve to start one ordinary reconnect;
//  4. Probe fails after Client context ended while old stays open -> move ends serve.
func TestA113MoveBoundaries(t *testing.T) {
	f := newPair(t)
	old1 := f.open(t, 0)
	cand1Client, cand1Server, _, _ := f.dialProbe(t)
	clCtx, clCancel := context.WithCancel(t.Context())
	defer clCancel()
	cl := &Client{
		device: f.b.key.ID(), hello: testHello, onWelcome: func(*Session, proto.Welcome) {},
		pathOf: func(*Connection) string { return "relay" }, firstProbe: probeFirstWait,
		ctx: clCtx, cancel: clCancel, done: make(chan struct{}), pathChanged: make(chan struct{}, 1),
	}
	s1 := newSession(cl, old1.client)
	cl.current = s1
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	st := &serveState{
		s: s1, opened: time.Now().Add(-time.Minute), wait: 2 * probeFirstWait,
		timer: timer,
	}
	go func() {
		eq(t, recv(t, f.opened), cand1Server)
		recv(t, old1.server.Done())
		recv(t, old1.client.Done())
		raw, err := cand1Server.Read(t.Context())
		check(t, err)
		var h proto.Hello
		check(t, proto.Decode(raw, &h))
		_ = cand1Server.Send(testWelcome(h.ID, f.b.key.ID()))
	}()
	beforeMove := time.Now()
	ended := cl.move(st, cand1Client)
	eq(t, ended, false)
	eq(t, st.s.c, cand1Client)
	eq(t, st.wait, probeFirstWait)
	eq(t, st.scheduled, true)
	if st.opened.Before(beforeMove) {
		t.Fatalf("st.opened %v was not updated on move", st.opened)
	}
	recv(t, s1.Done())
	st.stop()

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	dc, err := pc.CreateDataChannel("ctl", nil)
	check(t, err)
	mkBrokenProbe := func() *Connection {
		pCtx, pCancel := context.WithCancel(t.Context())
		return &Connection{
			ctx: pCtx, ready: make(chan struct{}), dc: dc,
			waiting: make(map[int64]chan json.RawMessage),
			close:   sync.OnceValue(func() error { pCancel(); return nil }),
		}
	}

	// Probe fails while cand1Client is still open: old continues and schedules 2*wait.
	ended = cl.move(st, mkBrokenProbe())
	eq(t, ended, false)
	eq(t, st.s.c, cand1Client)
	eq(t, st.wait, 2*probeFirstWait)
	eq(t, st.scheduled, true)
	st.stop()
	st.scheduleNext("direct")
	eq(t, st.scheduled, false)

	// Probe fails after cand1Client closed: serve ends so Client.run reconnects.
	check(t, cand1Client.Close())
	ended = cl.move(st, mkBrokenProbe())
	eq(t, ended, true)
	recv(t, st.s.Done())

	// Probe fails when cl.ctx is cancelled while old connection is still open: serve ends.
	liveOld := newPair(t).open(t, 0)
	sLive := newSession(cl, liveOld.client)
	cl.current = sLive
	st.s = sLive
	clCancel()
	eq(t, cl.move(st, mkBrokenProbe()), true)
	recv(t, sLive.Done())
}
