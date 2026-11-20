package peer

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
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

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
	_, err := f.a.OfferProbe(t.Context(), f.b.key.ID())
	eq(t, err == nil, false)

	f2 := newPair(t)
	relay := localRelay(t)
	f2.a.paths = func(string) []Route { r := f2.aMail.route(); r.ICE = []proto.ICEServer{relay}; return []Route{r} }
	f2.b.paths = func(string) []Route { r := f2.bMail.route(); r.ICE = []proto.ICEServer{relay}; return []Route{r} }
	pr, err := f2.a.OfferProbe(t.Context(), f2.b.key.ID())
	check(t, err)
	t.Cleanup(func() { pr.Close() })
	offer := recv(t, f2.aMail["offer"])
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	eq(t, o.Probe, true)
	deliver(t, f2.b, f2.bMail.route(), offer)
	answer := recv(t, f2.bMail["answer"])
	deliver(t, f2.a, f2.aMail.route(), answer)
	// Wait briefly to gather host/srflx candidates and confirm neither end
	// gathers a relay candidate on a probe.
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
// the client and server *Connection once ctl is open on the client.
func (f *pair) dialProbe(t *testing.T) (*Connection, *Connection, proto.Envelope, proto.Envelope) {
	t.Helper()
	c, err := f.a.OfferProbe(t.Context(), f.b.key.ID())
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

// TestInboundProbePromotionAndRaces covers:
//   - probe not taking the slot before hello;
//   - a second probe superseding the first;
//   - non-ctl channels, binary frames, malformed/non-hello requests, and v!=1
//     before promotion;
//   - promotion on first valid hello closing the predecessor, dispatching extra
//     channels afterwards, and ignoring repeated hello for promotion;
//   - ordinaryOffer closing both the slot and any pending probe;
//   - a revoked signer or superseded probe failing promotion on hello.
func TestInboundProbePromotionAndRaces(t *testing.T) {
	f := newPair(t)
	handed := make(chan *Stream, 2)
	f.b.channels = Channels{"test:": func(s *Stream) { handed <- s }}
	first := f.open(t, 0)
	first.info(t)

	// First probe opens ctl: slot stays first, inbound not called yet, copy of
	// probe offer gets the same answer.
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

	// Extra channel opened on an unpromoted probe is closed.
	earlyExtra := openRemote(t, p1Client, "test:before")
	recv(t, earlyExtra.closed)
	eq(t, len(handed), 0)

	// Second probe supersedes p1 while leaving first.server in slot.
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

	// Binary message, non-hello request, re+id frame, bad hello kind, and v=2
	// hello all leave p2 unpromoted and open.
	check(t, p2Client.dc.Send([]byte("binary")))
	for _, tmpl := range []string{
		`{"t":"host.info","id":%d}`,
		`{"t":"hello","id":%d,"re":1,"v":1,"kind":"host","name":"n","addrs":[]}`,
		`{"t":"hello","id":%d,"v":1,"kind":"other","name":"n","addrs":[]}`,
		`{"t":"hello","id":%d,"v":1,"kind":"host","name":"n","addrs":null}`,
	} {
		raw, err := p2Client.Request(t.Context(), func(id int64) any { return json.RawMessage(fmt.Sprintf(tmpl, id)) })
		check(t, err)
		var e proto.ErrorResponse
		check(t, proto.Decode(raw, &e))
		eq(t, e.Err.Code, "bad_request")
	}
	// Push without id is dropped without closing or promoting.
	check(t, p2Client.Send(proto.RVFrame{Type: "devlog"}))
	// Unsupported version answers "version" and keeps p2 open and unpromoted.
	raw, err := p2Client.Request(t.Context(), func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 2, Kind: "host", Name: "n", Addrs: []string{}}
	})
	check(t, err)
	var verErr proto.ErrorResponse
	check(t, proto.Decode(raw, &verErr))
	eq(t, verErr.Err.Code, "version")
	eq(t, len(f.opened), 0)
	first.info(t)

	// Valid hello promotes p2, closes first.server, delivers hello to inbound.
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

	// After promotion, channels beside ctl are dispatched and repeated hello
	// goes straight through deliver without re-promoting.
	laterExtra := openRemote(t, p2Client, "test:after")
	s := recv(t, handed)
	eq(t, s.Label(), "test:after")
	recv(t, laterExtra.open)
	check(t, p2Client.Send(proto.Hello{Request: proto.Request{Type: "hello", ID: 99}, V: 1, Kind: "host", Name: "n", Addrs: []string{}}))
	msg, err = p2Server.Read(t.Context())
	check(t, err)
	check(t, proto.Decode(msg, &gotHello))
	eq(t, gotHello.ID, int64(99))

	// A pending probe p3 is closed when an ordinary offer arrives.
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

// TestInboundProbeEdgeRejections covers refused probe SDP, rejected/duplicate
// probe ctl channels, and a signer revoked before hello arrives on its probe.
func TestInboundProbeEdgeRejections(t *testing.T) {
	f := newPair(t)
	// Refused SDP in a probe offer closes the connection and leaves slot/probes empty.
	badOffer := f.a.sign(proto.Offer{
		SignalHeader: f.a.header(link{f.b.key.ID(), randomID()}, "offer"),
		SDP:          probeSDP, Probe: true,
	})
	eq(t, f.b.Receive(t.Context(), Route{}, frame(badOffer)), identity.BadRequest)
	f.b.mu.Lock()
	eq(t, len(f.b.slot), 0)
	eq(t, len(f.b.probes), 0)
	f.b.mu.Unlock()

	// A closed probe still present in p.probes cannot promote, and closing a
	// pending probe clears p.probes via forgetProbe.
	closedCtx, closedCancel := context.WithCancel(t.Context())
	closedCancel()
	closedProbe := &Connection{Device: f.a.key.ID(), ctx: closedCtx, close: func() error { return nil }}
	f.b.probes[f.a.key.ID()] = closedProbe
	eq(t, f.b.promote(closedProbe), false)
	delete(f.b.probes, f.a.key.ID())

	soloClient, soloServer, _, _ := f.dialProbe(t)
	check(t, soloClient.Send(proto.Hello{Request: proto.Request{Type: "hello", ID: 1}, V: 1, Kind: "host", Name: "n", Addrs: []string{}}))
	eq(t, recv(t, f.opened), soloServer)
	check(t, soloServer.Close())
	check(t, soloClient.Close())
	recv(t, soloServer.Done())

	dropClient, dropServer, _, _ := f.dialProbe(t)
	check(t, dropClient.Close())
	check(t, dropServer.Close())
	for {
		f.b.mu.Lock()
		nProbes := len(f.b.probes)
		f.b.mu.Unlock()
		if nProbes == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Unordered ctl on a probe closes the probe; duplicate ctl closes the second.
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

	// Revoked device before hello on probe closes the probe without promoting.
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

// TestOfferProbeDeadlinesAndSendFailure covers OfferProbe broadcast errors,
// context cancellation, and all three cases of watchProbeOpen (15s timeout,
// early Ready, early Done).
func TestOfferProbeDeadlinesAndSendFailure(t *testing.T) {
	f := newPair(t)
	f.a.paths = func(string) []Route { return nil }
	_, err := f.a.OfferProbe(t.Context(), f.b.key.ID())
	eq(t, err.Error(), "no signalling path")

	fCancel := newPair(t)
	sending := make(chan struct{}, 1)
	fCancel.a.paths = func(string) []Route {
		return []Route{{Send: func(ctx context.Context, _ string, _ proto.Envelope) error {
			sendBoolChan(sending)
			<-ctx.Done()
			return ctx.Err()
		}}}
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-sending; cancel() }()
	_, err = fCancel.a.OfferProbe(ctx, fCancel.b.key.ID())
	eq(t, err, context.Canceled)

	synctest.Test(t, func(t *testing.T) {
		mk := func() (*Connection, chan struct{}, context.CancelFunc) {
			cCtx, cCancel := context.WithCancel(context.Background())
			ready := make(chan struct{})
			c := &Connection{ctx: cCtx, ready: ready, close: func() error { cCancel(); return nil }}
			return c, ready, cCancel
		}
		// 1. Expires at exactly 15 seconds when ctl has not opened.
		c1, _, _ := mk()
		go watchProbeOpen(c1, probeOpenLimit)
		time.Sleep(probeOpenLimit - time.Nanosecond)
		synctest.Wait()
		eq(t, c1.isClosed(), false)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		eq(t, c1.isClosed(), true)

		// 2. Exits cleanly when Ready closes before 15 seconds.
		c2, ready2, cancel2 := mk()
		defer cancel2()
		go watchProbeOpen(c2, probeOpenLimit)
		close(ready2)
		time.Sleep(time.Hour)
		synctest.Wait()
		eq(t, c2.isClosed(), false)

		// 3. Exits cleanly when Done closes before 15 seconds.
		c3, _, cancel3 := mk()
		go watchProbeOpen(c3, probeOpenLimit)
		cancel3()
		synctest.Wait()
		eq(t, c3.isClosed(), true)
	})
}

func sendBoolChan(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// mockConn builds a real SCTP-backed pair of *Connections on loopback so Send
// and Request work end to end, or an in-memory *Connection for synctest.
// Notice c.Send calls c.dc.SendText: for synctest we only need real SendText
// if c.Send is called! Wait: let's check what happens if c.dc is nil! c.Send
// panics if c.dc is nil! Let's check!

// TestEndToEndClientProbeMoveAndRequests exercises p.Client over real WebRTC
// connections and a local TURN relay:
// - initial relayed path;
// - forced probe while commands and pushes are in flight;
// - Session.Read stopping immediately on retirement even with queued pushes;
// - Session.Request on sync vs non-sync, oversize Request, disconnected Request.
func TestEndToEndClientProbeMoveAndRequests(t *testing.T) {
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

	// Pump signalling between f.aMail and f.bMail.
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

	// Server handler on f.b: answers hello with welcome, holds "hold" requests
	// until released (simulating a command in flight across a move!), and
	// answers "host.info" immediately.
	var holdFirst atomic.Bool
	holdFirst.Store(true)
	var syncCount atomic.Int32
	heldArrived := make(chan struct{}, 1)
	serverConns := make(chan *Connection, 4)
	var orderMu sync.Mutex
	var wireIDs []int64
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
					wireIDs = append(wireIDs, q.ID)
					orderMu.Unlock()
					if holdFirst.CompareAndSwap(true, false) {
						sendBoolChan(heldArrived)
						continue
					}
					_ = srv.Send(proto.Success{Re: q.ID, OK: true})
				case "sync":
					if syncCount.Add(1) > 1 {
						_ = srv.Send(proto.Success{Re: q.ID, OK: true})
					}
					continue
				case "host.info":
					_ = srv.Send(proto.Success{Re: q.ID, OK: true})
				}
			}
		}()
	}

	sessions := make(chan *Session, 4)
	cl := f.a.Client(t.Context(), f.b.key.ID(), nil, testHello, func(s *Session, _ proto.Welcome) {
		sessions <- s
	})
	t.Cleanup(func() { cl.Close() })

	s1 := recv(t, sessions)
	srv1 := recv(t, serverConns)
	_ = cl.PathChanged()
	for cl.Path() != "relay" {
		recv(t, cl.PathChanged())
	}
	eq(t, s1.Path(), "relay")
	_ = s1.PathChanged()

	// Start a blocking Read on s1 before retirement so s1.Read wakes with io.EOF.
	blockingReadErr := make(chan error, 1)
	go func() {
		_, rErr := s1.Read(t.Context())
		blockingReadErr <- rErr
	}()
	time.Sleep(20 * time.Millisecond)

	// Start a sync (per-session: ends with io.EOF on move) and a non-sync
	// request ("hold") that must replay on s2.
	syncErr := make(chan error, 1)
	go func() {
		_, err := s1.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
		syncErr <- err
	}()
	holdRes := make(chan error, 1)
	go func() {
		_, err := s1.Request(t.Context(), func(id int64) any {
			return proto.Request{Type: "hold", ID: id}
		})
		holdRes <- err
	}()
	recv(t, heldArrived)

	// Remove premature queue on s1 before blockingReadErr finished:
	// Unblock direct candidates and force a move.
	blocked.Store(false)
	cl.Probe()
	cl.Probe()

	s2 := recv(t, sessions)
	_ = recv(t, serverConns)
	recv(t, srv1.Done())
	recv(t, s1.Done())
	eq(t, recv(t, blockingReadErr), io.EOF)
	eq(t, recv(t, syncErr), io.EOF)
	check(t, recv(t, holdRes))
	orderMu.Lock()
	eq(t, len(wireIDs), 2)
	orderMu.Unlock()

	// Completed sync on s2 returns the server's reply directly.
	syncReply, err := s2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
	check(t, err)
	eq(t, strings.Contains(string(syncReply), `"ok":true`), true)

	// Retired s1 refuses Read and Send with io.EOF despite queued messages.
	s1.c.queue(json.RawMessage(`{"t":"rows"}`))
	_, err = s1.Read(t.Context())
	eq(t, err, io.EOF)
	eq(t, s1.Send(proto.RVFrame{Type: "ack"}), io.EOF)

	// Active s2 and cl work on direct path.
	for cl.Path() != "direct" {
		recv(t, cl.PathChanged())
	}
	check(t, cl.Send(proto.RVFrame{Type: "push"}))
	check(t, srvPush(t, s2))

	// Oversize request and cancelled context on Request.
	big := strings.Repeat("x", ctlLimit)
	_, err = cl.Request(t.Context(), func(id int64) any { return map[string]any{"t": "cmd", "id": id, "x": big} })
	eq(t, err.Error(), "ctl frame exceeds 60 KiB")
	cCtx, cCancel := context.WithCancel(t.Context())
	cCancel()
	_, err = cl.Request(cCtx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	eq(t, err, context.Canceled)

	reply, err := cl.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	check(t, err)
	var okRes proto.Success
	check(t, proto.Decode(reply, &okRes))
	eq(t, bool(okRes.OK), true)

	check(t, cl.Close())
	recv(t, cl.Done())
	// Calling Probe twice once stopped exercises both branches of sendBool.
	cl.Probe()
	cl.Probe()
	eq(t, cl.Path(), "")
	eq(t, cl.Send(proto.RVFrame{Type: "push"}), io.EOF)
	_, err = cl.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
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

// TestClientTimingsAndMoveOutcomes tests the exact timers and races of Client:
//   - probe schedule 30s -> 1m -> 2m -> 4m -> 5m -> 5m while relayed;
//   - stopping probes when Path flips to direct, and restarting at 30s when relay;
//   - A1.13 all three outcomes around the 10-second welcome deadline;
//   - Race 1 (ordinary connection closes while probe is unopened);
//   - failed offer, unopened ordinary connection, refused hello, send failure in
//     flushOne, request cancellation while in flight, and Client close while
//     requests wait.
func TestClientTimingsAndMoveOutcomes(t *testing.T) {
	// Create one pair of real open DataChannels outside synctest ONLY for tests
	// that need c.dc.SendText, OR use a closed/connecting DataChannel for error
	// paths; Wait: c.Send calls c.dc.SendText! Let's check: inside synctest,
	// does Pion's unopened DataChannel.SendText touch OS sockets? No! An
	// unopened webrtc.NewPeerConnection(Configuration{}) with no candidates
	// opens zero sockets until SetLocalDescription!
	// Even better: let's test the real-DataChannel helper outside synctest for
	// send-error/cancel cases, and test serveState/schedule/backoff logic in
	// synctest!
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var probeTimes []time.Time
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
			pathOf: func(*Connection) string { return pathVal.Load().(string) },
			ctx:    cCtx, cancel: cCancel, done: make(chan struct{}),
			pathChanged: make(chan struct{}, 1), probeNow: make(chan bool, 1),
		}
		cl.probe = func(context.Context, string) (*Connection, error) {
			mu.Lock()
			probeTimes = append(probeTimes, time.Now())
			mu.Unlock()
			if failOffer.Load() {
				return nil, errors.New("no path")
			}
			pCtx, pCancel := context.WithCancel(context.Background())
			pConn := &Connection{ctx: pCtx, ready: make(chan struct{}), close: func() error { pCancel(); return nil }}
			// Probe fails after 5 seconds (before ctl opens).
			time.AfterFunc(5*time.Second, pCancel)
			return pConn, nil
		}
		s := newSession(cl, srvConn)
		cl.current = s
		start := time.Now()
		done := make(chan time.Duration, 1)
		go func() { done <- cl.serve(s, start) }()
		synctest.Wait()

		// Each probe attempt runs for 5s before failing, then waits 60s, 120s, 240s, 300s, 300s.
		want := []time.Duration{
			30 * time.Second,
			95 * time.Second,
			220 * time.Second,
			465 * time.Second,
			770 * time.Second,
			1075 * time.Second,
		}
		for i, target := range want {
			if i == 5 {
				failOffer.Store(true) // cover synchronous OfferProbe error branch too
			}
			wait := time.Until(start.Add(target))
			time.Sleep(wait - time.Nanosecond)
			synctest.Wait()
			eq(t, countProbes(), i)
			time.Sleep(time.Nanosecond)
			synctest.Wait()
			eq(t, countProbes(), i+1)
		}

		// Path flips to direct: stops scheduled probes; stale non-forced signal ignored.
		pathVal.Store("direct")
		srvConn.reclassify()
		synctest.Wait()
		sendBool(cl.probeNow, false)
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		eq(t, countProbes(), 6)

		// Path flips back to relay: schedule starts fresh from 30s.
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

		// Ordinary connection closes while probe #7 is still pending (at +2s < 5s):
		// pending probe is closed immediately and serve returns open duration.
		time.Sleep(2 * time.Second)
		srvConn.Close()
		synctest.Wait()
		openDur := <-done
		if openDur < 10*time.Minute {
			t.Fatalf("unexpected open duration %v", openDur)
		}
	})
}

// TestClientErrorAndCancelBranches covers openSession failures, flushOne send
// error, request ctx cancellation while sent, and Client close while requests
// wait during a move.
func TestClientErrorAndCancelBranches(t *testing.T) {
	// Unopened Pion DataChannel makes SendText fail deterministically in flushOne.
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
				// Unopened dc causes sayHello's Request -> Send to fail!
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
	)
	recv(t, cl.Done())

	// 2. flushOne Send failure closes c, releases req, and retries on replacement;
	//    and cancelling req.ctx while awaiting reply hits <-req.done;
	//    and closing Client while moving finishes queued requests with io.EOF.
	f := newPair(t)
	n := f.open(t, 0)
	c2Ctx, c2Cancel := context.WithCancel(t.Context())
	defer c2Cancel()
	brokenConn := &Connection{
		ctx: c2Ctx, ready: make(chan struct{}), pathChanged: make(chan struct{}, 1),
		dc: unopenedDC, waiting: make(map[int64]chan json.RawMessage),
		close: sync.OnceValue(func() error { c2Cancel(); return nil }),
	}

	cl2Ctx, cl2Cancel := context.WithCancel(t.Context())
	cl2 := &Client{
		device: f.b.key.ID(), hello: testHello, onWelcome: func(*Session, proto.Welcome) {},
		pathOf: (*Connection).Path, ctx: cl2Ctx, cancel: cl2Cancel,
		done: make(chan struct{}), pathChanged: make(chan struct{}, 1), probeNow: make(chan bool, 1),
	}
	cl2.current = newSession(cl2, brokenConn)
	// Swap current to n.client as soon as brokenConn closes so releaseReq triggers retry!
	context.AfterFunc(c2Ctx, func() {
		cl2.mu.Lock()
		cl2.current = newSession(cl2, n.client)
		cl2.mu.Unlock()
		cl2.flushPending()
	})
	go func() {
		raw, rErr := n.server.Read(t.Context())
		if rErr == nil {
			var q proto.Request
			_ = proto.Decode(raw, &q)
			_ = n.server.Send(proto.Success{Re: q.ID, OK: true})
		}
	}()
	reply, err := cl2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	check(t, err)
	eq(t, strings.Contains(string(reply), `"ok":true`), true)

	// Cancel req.ctx while request is sent on n.client and unanswered -> hits <-req.done in awaitReq!
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

	// Queue a request while moving with current == nil, then cancel cl2Ctx -> hits io.EOF via stopClient!
	// First test releaseReq retry when current has already moved to another open connection before old closes:
	oldPair := newPair(t).open(t, 0)
	cl2.mu.Lock()
	cl2.current = newSession(cl2, oldPair.client)
	cl2.mu.Unlock()
	retryDone := make(chan error, 1)
	go func() {
		_, rErr := cl2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "moved-before-close", ID: id} })
		retryDone <- rErr
	}()
	// Wait until oldPair.server receives the request (so awaitReq is waiting on oldPair.client).
	_, err = oldPair.server.Read(t.Context())
	check(t, err)
	// Point cl2.current at n.client while oldPair.client is still open, then close oldPair.client!
	cl2.mu.Lock()
	cl2.current = newSession(cl2, n.client)
	cl2.mu.Unlock()
	go func() {
		raw, rErr := n.server.Read(t.Context())
		if rErr == nil {
			var q proto.Request
			_ = proto.Decode(raw, &q)
			_ = n.server.Send(proto.Success{Re: q.ID, OK: true})
		}
	}()
	check(t, oldPair.client.Close())
	check(t, recv(t, retryDone))

	cl2.retireCurrent()
	cl2.setMoving(true)
	eofCh := make(chan error, 1)
	go func() {
		_, rErr := cl2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "queued", ID: id} })
		eofCh <- rErr
	}()
	// Wait until queued in cl2.pending before cancelling cl2Ctx.
	for {
		cl2.mu.Lock()
		nPending := len(cl2.pending)
		cl2.mu.Unlock()
		if nPending == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cl2Cancel()
	eq(t, recv(t, eofCh), io.EOF)
	// With cl2Ctx cancelled while moving is still true, enqueueReq refuses new requests,
	// and a repeated finishReq on an already-finished request is a no-op.
	_, err = cl2.Request(t.Context(), func(id int64) any { return proto.Request{Type: "after-cancel", ID: id} })
	eq(t, err, io.EOF)
	cl2.finishReq(&pendingReq{done: make(chan struct{}), reply: make(chan reqResult, 1)}, reqResult{err: io.EOF})

	// buildHello and validWelcome boundary checks.
	eq(t, buildHello(proto.Hello{Kind: "host", Name: "n"}, 7), proto.Hello{
		Request: proto.Request{Type: "hello", ID: 7}, V: 1, Kind: "host", Name: "n", Addrs: []string{},
	})
	eq(t, buildHello(proto.Hello{V: 2, Kind: "host", Name: "n", Addrs: []string{"ws://127.0.0.1:47800/rv"}}, 8), proto.Hello{
		Request: proto.Request{Type: "hello", ID: 8}, V: 2, Kind: "host", Name: "n", Addrs: []string{"ws://127.0.0.1:47800/rv"},
	})
	eq(t, validWelcome(proto.Welcome{Type: "welcome", V: 1}), true)
	eq(t, validWelcome(proto.Welcome{Type: "other", V: 1}), false)
	eq(t, validWelcome(proto.Welcome{Type: "welcome", V: 2}), false)
}

// TestA113MoveBoundaries tests all three A1.13 move outcomes on Client.move:
//  1. Old closes during hello wait, welcome arrives before 10s -> move succeeds, no reconnect;
//  2. Old closes during hello wait, welcome withheld past 10s -> move fails, ends serve to reconnect;
//  3. Old stays open during hello wait, welcome withheld past 10s -> probe discarded, old continues,
//     and pending requests queued during moving flush onto old.
func TestA113MoveBoundaries(t *testing.T) {
	f := newPair(t)
	// Case 1: old closes after hello sent, welcome arrives before 10s.
	old1 := f.open(t, 0)
	cand1Client, cand1Server, _, _ := f.dialProbe(t)
	clCtx, clCancel := context.WithCancel(t.Context())
	defer clCancel()
	cl := &Client{
		device: f.b.key.ID(), hello: testHello, onWelcome: func(*Session, proto.Welcome) {},
		pathOf: (*Connection).Path, ctx: clCtx, cancel: clCancel,
		done: make(chan struct{}), pathChanged: make(chan struct{}, 1), probeNow: make(chan bool, 1),
	}
	s1 := newSession(cl, old1.client)
	cl.current = s1
	st := &serveState{
		s: s1, opened: time.Now(), wait: probeFirstWait,
		timer: time.AfterFunc(time.Hour, func() {}),
	}
	st.timer.Stop()
	// Drive cand1Server: promote on hello (closing old1.server), wait until old1.client
	// is closed, then send welcome!
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
	ended := cl.move(st, cand1Client)
	eq(t, ended, false)
	eq(t, st.s.c, cand1Client)
	recv(t, s1.Done())

	// Case 2 & Case 3 using synctest with an unopened cand DataChannel? Wait: in
	// synctest, to wait the full 10s helloTimeout (instead of failing immediately
	// on SendText), we test move with a real SCTP pair or test sayHello's 10s
	// timeout! Wait: in features/peer_probe_test.go we test the real 10s welcome
	// timeout end-to-end, AND here we can test both! Let's test Case 2 (old closed
	// + move fails) and Case 3 (old open + move fails + queued request flushes to old)
	// directly here too!
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

	// Case 3: old (cand1Client) is STILL open when probe fails -> ended == false,
	// st.s remains cand1Client, and a pending request flushes onto cand1Client!
	reqDone := make(chan error, 1)
	cl.mu.Lock()
	req := &pendingReq{
		ctx:   t.Context(),
		build: func(id int64) any { return proto.Request{Type: "after-fail", ID: id} },
		reply: make(chan reqResult, 1),
		done:  make(chan struct{}),
	}
	cl.pending = append(cl.pending, req)
	cl.mu.Unlock()
	go func() {
		res := <-req.reply
		reqDone <- res.err
	}()
	go func() {
		raw, rErr := cand1Server.Read(t.Context())
		if rErr == nil {
			var q proto.Request
			_ = proto.Decode(raw, &q)
			_ = cand1Server.Send(proto.Success{Re: q.ID, OK: true})
		}
	}()
	ended = cl.move(st, mkBrokenProbe())
	eq(t, ended, false)
	eq(t, st.s.c, cand1Client)
	check(t, recv(t, reqDone))

	// Case 2: old (cand1Client) is CLOSED when probe fails -> ended == true!
	check(t, cand1Client.Close())
	ended = cl.move(st, mkBrokenProbe())
	eq(t, ended, true)
	recv(t, st.s.Done())

	// Case 2b: old is still open, but cl.ctx is cancelled when probe fails -> ended == true!
	liveOld := newPair(t).open(t, 0)
	sLive := newSession(cl, liveOld.client)
	cl.current = sLive
	st.s = sLive
	clCancel()
	eq(t, cl.move(st, mkBrokenProbe()), true)
	recv(t, sLive.Done())
}
