package peer

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func eq(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	panic("unreachable")
}

type mailbox map[string]chan proto.Envelope

func mail() mailbox {
	return mailbox{"offer": make(chan proto.Envelope, 8), "answer": make(chan proto.Envelope, 8), "ice": make(chan proto.Envelope, 128)}
}
func (m mailbox) route() Route {
	return Route{Send: func(ctx context.Context, to string, env proto.Envelope) error {
		var h proto.SignalHeader
		if err := proto.Decode([]byte(env.Payload), &h); err != nil {
			return err
		}
		if to != h.To {
			return errors.New("wrong signal target")
		}
		select {
		case m[h.Type] <- env:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
}

// pair is two nodes whose signalling goes to mailboxes the test delivers from.
type pair struct {
	a, b            *Peer
	aMail, bMail    mailbox
	aOpened, opened chan *Connection
	now             atomic.Int64
	path            string
}

func testKey(t *testing.T) identity.Key {
	k, err := identity.LoadKey(t.TempDir())
	check(t, err)
	return k
}

var loopbackIP = net.IPv4(127, 0, 0, 1)

func loopback(ip net.IP) bool { return ip.Equal(loopbackIP) }

// loopbackEngine keeps gathering on 127.0.0.1: the tests need no other
// address, and a host firewall may refuse inbound traffic on the others.
func loopbackEngine() webrtc.SettingEngine {
	var engine webrtc.SettingEngine
	engine.SetIncludeLoopbackCandidate(true)
	engine.SetIPFilter(loopback)
	return engine
}
func sockets(t *testing.T) *Sockets {
	s, err := listen(0, "127.0.0.1:0", loopbackEngine(), ice.UDPMuxFromPortWithLoopback(), ice.UDPMuxFromPortWithIPFilter(loopback))
	check(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}
func newPair(t *testing.T) *pair {
	a, b := testKey(t), testKey(t)
	f := &pair{aMail: mail(), bMail: mail(), aOpened: make(chan *Connection, 8), opened: make(chan *Connection, 8), path: filepath.Join(t.TempDir(), "node.db")}
	f.now.Store(time.Now().UnixMilli())
	dbA, err := store.Open(":memory:")
	check(t, err)
	t.Cleanup(func() { dbA.Close() })
	dbB, err := store.Open(f.path)
	check(t, err)
	f.a = New(a, sockets(t), dbA, func(id string) bool { return id == b.ID() }, func(string) []Route { return []Route{f.aMail.route()} }, func(c *Connection) { f.aOpened <- c })
	f.b = New(b, sockets(t), dbB, func(id string) bool { return id == a.ID() }, func(string) []Route { return []Route{f.bMail.route()} }, func(c *Connection) { f.opened <- c })
	f.a.now = func() time.Time { return time.UnixMilli(f.now.Load()) }
	f.b.now = f.a.now
	t.Cleanup(func() { f.b.db.Close() })
	return f
}
func frame(e proto.Envelope) proto.Route {
	return proto.Route{Type: "from", Key: e.Key, Env: encode(e)}
}
func deliver(t *testing.T, n *Peer, path Route, e proto.Envelope) {
	t.Helper()
	check(t, n.Receive(t.Context(), path, frame(e)))
}

// hostUDP returns the next host UDP candidate of conn from m: an earlier
// connection may still be gathering.
func hostUDP(t *testing.T, m mailbox, conn string) (proto.Envelope, proto.ICECandidate) {
	t.Helper()
	for {
		env := recv(t, m["ice"])
		var c proto.ICECandidate
		check(t, proto.Decode([]byte(env.Payload), &c))
		if c.Conn == conn && strings.Contains(c.Candidate, " udp ") && strings.Contains(c.Candidate, " typ host") {
			return env, c
		}
	}
}

type negotiation struct {
	client, server *Connection
	offer, answer  proto.Envelope
}

func (f *pair) open(t *testing.T, offset int64) negotiation {
	t.Helper()
	return dial(t, f.a, f.b, f.aMail, f.bMail, f.opened, offset)
}

// dial opens from → to with the offer's ts moved by offset. It delivers two
// copies of a real candidate of each side before the description it belongs
// to, and another copy after ctl opens: each is applied once, after the
// description.
func dial(t *testing.T, from, to *Peer, fromMail, toMail mailbox, opened chan *Connection, offset int64) negotiation {
	t.Helper()
	c, err := from.Offer(t.Context(), to.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	eq(t, c.Path(), "")
	offer := recv(t, fromMail["offer"])
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	o.Timestamp += offset
	offer = from.sign(o)
	li, l := hostUDP(t, fromMail, o.Conn)
	for range 2 {
		deliver(t, to, toMail.route(), li)
	}
	eq(t, to.early[link{from.key.ID(), o.Conn}].pending, []candidate{{l.MID, l.Candidate}})
	deliver(t, to, toMail.route(), offer)
	answer := recv(t, toMail["answer"])
	ri, r := hostUDP(t, toMail, o.Conn)
	for range 2 {
		deliver(t, from, fromMail.route(), ri)
	}
	eq(t, c.candidates.pending, []candidate{{r.MID, r.Candidate}})
	deliver(t, from, fromMail.route(), answer)
	recv(t, c.Ready())
	s := recv(t, opened)
	t.Cleanup(func() { s.Close() })
	deliver(t, from, fromMail.route(), answer)
	deliver(t, to, toMail.route(), li)
	deliver(t, from, fromMail.route(), ri)
	for _, conn := range []*Connection{c, s} {
		eq(t, len(conn.candidates.pending), 0)
		eq(t, len(conn.candidates.seen), 1)
		eq(t, conn.Path(), "direct")
	}
	selected, err := c.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	check(t, err)
	eq(t, int(selected.Remote.Port), to.sockets.udp.GetListenAddresses()[0].(*net.UDPAddr).Port)
	if int(selected.Local.Port) == from.sockets.udp.GetListenAddresses()[0].(*net.UDPAddr).Port {
		t.Fatal("offer used answer mux")
	}
	return negotiation{c, s, offer, answer}
}
func (n negotiation) info(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		data, err := n.server.Read(ctx)
		if err == nil {
			var q proto.Request
			err = proto.Decode(data, &q)
			if err == nil && q.Type != "host.info" {
				err = errors.New("unexpected request")
			}
			if err == nil {
				err = n.server.Send(struct {
					proto.Success
					Type string `json:"t"`
				}{proto.Success{Re: q.ID, OK: true}, "host.info"})
			}
		}
		done <- err
	}()
	reply, err := n.client.Request(ctx, func(id int64) any { return proto.Request{Type: "host.info", ID: id} })
	check(t, err)
	check(t, <-done)
	var res proto.Success
	check(t, proto.Decode(reply, &res))
	eq(t, res, proto.Success{OK: true, Re: n.client.next})
}

// A1.10: a remembered offer neither starts checks nor closes anything, up to
// and including ts + 120000 on the receiver's clock.
func TestCandidateCopiesAndOfferLifetime(t *testing.T) {
	f := newPair(t)
	past := f.open(t, 120000) // future-dated: passed check 5 on receipt
	past.info(t)
	duplicate := mail()
	deliver(t, f.b, duplicate.route(), past.offer)
	eq(t, recv(t, duplicate["answer"]), past.answer)
	later := f.open(t, 0)
	recv(t, past.server.Done())
	for _, advance := range []int64{0, 120001, 119999} {
		f.now.Add(advance)
		deliver(t, f.b, duplicate.route(), past.offer)
		eq(t, len(duplicate["answer"]), 0)
		eq(t, len(f.opened), 0)
		later.info(t)
	}
	f.now.Add(1)
	eq(t, f.b.Receive(t.Context(), duplicate.route(), frame(past.offer)), identity.Stale)
}
func TestRestartDoesNotForgetFutureOffer(t *testing.T) {
	f := newPair(t)
	old := f.open(t, 120000)
	check(t, old.server.Close())
	check(t, old.client.Close())
	check(t, f.b.db.Close())
	db, err := store.Open(f.path)
	check(t, err)
	prev := f.b
	f.b = New(prev.key, sockets(t), db, prev.known, prev.paths, prev.inbound)
	f.b.now = prev.now
	f.now.Add(240000)
	current := f.open(t, 0)
	copies := mail()
	deliver(t, f.b, copies.route(), old.offer)
	eq(t, len(copies["answer"]), 0)
	current.info(t)
}

// An ordinary offer closes the signer's earlier inbound connection, whatever
// its close reports, and leaves the connection offered to that signer alone.
func TestCloseFirstLeavesOutboundAlone(t *testing.T) {
	f := newPair(t)
	closed := make(chan struct{})
	f.b.slot[f.a.key.ID()] = &Connection{close: func() error { close(closed); return errors.New("old transport") }}
	first := f.open(t, 0)
	recv(t, closed)
	back := dial(t, f.b, f.a, f.bMail, f.aMail, f.aOpened, 0)
	second := f.open(t, 0)
	recv(t, first.server.Done())
	recv(t, first.client.Done())
	f.b.mu.Lock()
	slot := f.b.slot[f.a.key.ID()]
	f.b.mu.Unlock()
	eq(t, slot, second.server)
	back.info(t)
	second.info(t)
}
func TestEveryPathChecksEnvelope(t *testing.T) {
	f := newPair(t)
	data := proto.ICECandidate{SignalHeader: f.a.header(link{f.b.key.ID(), randomID()}, "ice"), Candidate: "candidate", MID: "0"}
	valid := f.a.sign(data)
	wrong := frame(valid)
	wrong.Key = f.b.key.ID()
	unreadable := frame(valid)
	unreadable.Env = json.RawMessage(`[]`)
	badSig := frame(valid)
	badSig.Env = encode(f.a.key.Sign(string(encode(data)) + "x"))
	var e proto.Envelope
	check(t, json.Unmarshal(badSig.Env, &e))
	e.Payload = valid.Payload
	badSig.Env = encode(e)
	other := data
	other.To = f.a.key.ID()
	stale := data
	stale.Timestamp -= 120001
	badID := data
	badID.Conn = ""
	header := func(kind string) proto.SignalHeader { return f.a.header(link{f.b.key.ID(), randomID()}, kind) }
	for _, tc := range []struct {
		from proto.Route
		want error
	}{
		{wrong, identity.BadSig}, {unreadable, identity.BadSig}, {badSig, identity.BadSig}, {frame(f.b.sign(data)), identity.UnknownDevice},
		{frame(f.a.sign(other)), identity.NotForMe}, {frame(f.a.sign(stale)), identity.Stale}, {frame(f.a.sign(badID)), identity.BadRequest},
		{frame(f.a.sign(header("beacon"))), identity.BadRequest},
		{frame(f.a.sign(header("offer"))), identity.BadRequest},
		{frame(f.a.sign(proto.Offer{SignalHeader: header("offer"), SDP: "v=0", Probe: true})), identity.BadRequest},
		{frame(f.a.sign(proto.Offer{SignalHeader: header("offer"), SDP: "not a description"})), identity.BadRequest},
		{frame(f.a.sign(header("answer"))), identity.BadRequest},
		{frame(f.a.sign(header("ice"))), identity.BadRequest},
	} {
		eq(t, f.b.Receive(t.Context(), Route{}, tc.from), tc.want)
	}
	eq(t, len(f.b.early), 0)
	eq(t, len(f.b.live), 0)
}

// An offer whose description pion refuses is remembered and answered with
// nothing; its connection is closed again.
func TestRefusedDescriptions(t *testing.T) {
	f := newPair(t)
	sdp := "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=mid:0\r\n"
	k := link{f.a.key.ID(), randomID()}
	offer := f.a.sign(proto.Offer{SignalHeader: f.a.header(link{f.b.key.ID(), k.conn}, "offer"), SDP: sdp})
	eq(t, f.b.Receive(t.Context(), Route{}, frame(offer)), identity.BadRequest)
	copies := mail()
	deliver(t, f.b, copies.route(), offer)
	eq(t, len(copies["answer"]), 0)

	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	o := offerLink(t, f.b.key.ID(), recv(t, f.aMail["offer"]))
	answer := f.b.sign(proto.Answer{SignalHeader: f.b.header(link{f.a.key.ID(), o.conn}, "answer"), SDP: sdp})
	eq(t, f.a.Receive(t.Context(), Route{}, frame(answer)), identity.BadRequest)
	f.a.mu.Lock()
	waiting := c.pending != nil
	f.a.mu.Unlock()
	eq(t, waiting, true)
}

// ICE servers come from a signalling path; pion refuses a malformed one.
func TestMalformedICEServer(t *testing.T) {
	f := newPair(t)
	bad := func(m mailbox) func(string) []Route {
		return func(string) []Route {
			r := m.route()
			r.ICE = []proto.ICEServer{{URLs: []string{"bogus"}}}
			return []Route{r}
		}
	}
	f.a.paths = bad(f.aMail)
	_, err := f.a.Offer(t.Context(), f.b.key.ID())
	eq(t, err == nil, false)
	eq(t, len(f.a.live), 0)
	offer := f.b.sign(proto.Offer{SignalHeader: f.b.header(link{f.a.key.ID(), randomID()}, "offer"), SDP: "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\n"})
	err = f.a.Receive(t.Context(), Route{}, frame(offer))
	var code identity.Code
	eq(t, err != nil && !errors.As(err, &code), true)
}

// A copy of an offer whose connection has closed, and an answer for a conn
// that is not waiting for one, are ignored.
func TestCopiesForClosedConnections(t *testing.T) {
	f := newPair(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	k := link{f.a.key.ID(), randomID()}
	answer := f.b.sign(proto.Answer{SignalHeader: f.b.header(k, "answer"), SDP: "v=0"})
	f.b.live[k] = &Connection{ctx: ctx, answer: &answer}
	copies := mail()
	deliver(t, f.b, copies.route(), f.a.sign(proto.Offer{SignalHeader: f.a.header(link{f.b.key.ID(), k.conn}, "offer"), SDP: "v=0"}))
	eq(t, len(copies["answer"]), 0)
	deliver(t, f.a, copies.route(), f.b.sign(proto.Answer{SignalHeader: f.b.header(link{f.a.key.ID(), randomID()}, "answer"), SDP: "v=0"}))
	eq(t, len(f.a.live), 0)
}
func TestCertificateMismatchEndsPendingConnection(t *testing.T) {
	f := newPair(t)
	// Isolate the failure from an incidental CloseNotify from the other end.
	engine := loopbackEngine()
	engine.DisableCloseByDTLS(true)
	f.a.sockets.offer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	offer := recv(t, f.aMail["offer"])
	k := offerLink(t, f.b.key.ID(), offer)
	deliver(t, f.b, f.bMail.route(), offer)
	var a proto.Answer
	check(t, proto.Decode([]byte(recv(t, f.bMail["answer"]).Payload), &a))
	// A local signer advertises a certificate different from its DTLS certificate.
	lines := strings.Split(a.SDP, "\r\n")
	for j, line := range lines {
		if strings.HasPrefix(line, "a=fingerprint:sha-256 ") {
			lines[j] = "a=fingerprint:sha-256 " + strings.Repeat("00:", 31) + "00"
		}
	}
	a.SDP = strings.Join(lines, "\r\n")
	deliver(t, f.a, f.aMail.route(), f.b.sign(a))
	li, _ := hostUDP(t, f.aMail, k.conn)
	ri, _ := hostUDP(t, f.bMail, k.conn)
	deliver(t, f.b, f.bMail.route(), li)
	deliver(t, f.a, f.aMail.route(), ri)
	recv(t, c.Done())
	select {
	case <-c.Ready():
		t.Fatal("ctl opened with wrong certificate")
	default:
	}
}

// A1.11 at the call: before the description nothing is added, afterwards each
// distinct candidate exactly once.
func TestCandidateAddOrder(t *testing.T) {
	b := newBatch()
	var added []candidate
	add := func(i webrtc.ICECandidateInit) { added = append(added, candidate{*i.SDPMid, i.Candidate}) }
	first, second := candidate{"0", "first"}, candidate{"0", "second"}
	for range 2 {
		b.receive(first, add)
		b.receive(second, add)
	}
	eq(t, len(added), 0)
	b.flush(add)
	eq(t, added, []candidate{first, second})
	b.receive(second, add)
	eq(t, added, []candidate{first, second})
	later := candidate{"0", "later"}
	b.receive(later, add)
	eq(t, added, []candidate{first, second, later})
}

func TestEarlyAndRetiredCutoffs(t *testing.T) {
	f := newPair(t)
	p := f.b
	k := link{f.a.key.ID(), randomID()}
	v := candidate{"0", "local fixture"}
	check(t, p.hold(k, v, 110000, 100000))
	check(t, p.hold(k, v, 100000, 100000))
	eq(t, p.early[k].expires, int64(230000))
	p.prune(230000)
	eq(t, p.early[k].pending, []candidate{v})
	p.prune(230001)
	eq(t, len(p.early), 0)
	f.now.Store(800000)
	old := &Connection{ctx: context.Background(), close: func() error { return nil }}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	old.ctx = ctx
	current := &Connection{}
	p.live[k] = old
	p.slot[k.device] = current
	p.forget(k, old)
	eq(t, p.slot[k.device], current)
	eq(t, len(p.live), 0)
	eq(t, p.retired[k], int64(920000))
	p.prune(920000)
	check(t, p.hold(k, v, 920000, 920000))
	eq(t, len(p.early), 0)
	p.prune(920001)
	eq(t, len(p.retired), 0)
	p.slot[k.device] = old
	p.forget(k, old)
	eq(t, len(p.slot), 0)
	remembered := link{k.device, randomID()}
	first, err := p.db.RememberOffer(remembered.device, remembered.conn, 920000, 920000)
	check(t, err)
	eq(t, first, true)
	check(t, p.hold(remembered, v, 920000, 920000))
	eq(t, len(p.early), 0)
}
