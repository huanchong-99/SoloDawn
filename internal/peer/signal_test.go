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

type pair struct {
	a, b         *Peer
	aMail, bMail mailbox
	opened       chan *Connection
	now          atomic.Int64
	path         string
}

func testKey(t *testing.T) identity.Key {
	k, err := identity.LoadKey(t.TempDir())
	check(t, err)
	return k
}
func sockets(t *testing.T) *Sockets {
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	check(t, err)
	port := l.LocalAddr().(*net.UDPAddr).Port
	check(t, l.Close())
	s, err := Listen(port, 0)
	check(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}
func newPair(t *testing.T) *pair {
	a, b := testKey(t), testKey(t)
	f := &pair{aMail: mail(), bMail: mail(), opened: make(chan *Connection, 8), path: filepath.Join(t.TempDir(), "node.db")}
	f.now.Store(time.Now().UnixMilli())
	dbA, err := store.Open(":memory:")
	check(t, err)
	t.Cleanup(func() { dbA.Close() })
	dbB, err := store.Open(f.path)
	check(t, err)
	f.a = New(a, sockets(t), dbA, func(id string) bool { return id == b.ID() }, func(string) []Route { return []Route{f.aMail.route()} }, func(*Connection) {})
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
func hostUDP(t *testing.T, m mailbox) (proto.Envelope, proto.ICECandidate) {
	t.Helper()
	for {
		env := recv(t, m["ice"])
		var c proto.ICECandidate
		check(t, proto.Decode([]byte(env.Payload), &c))
		if strings.Contains(strings.ToLower(c.Candidate), " udp ") && strings.Contains(c.Candidate, " typ host") {
			return env, c
		}
	}
}

type negotiation struct {
	client, server *Connection
	offer, answer  proto.Envelope
}

// Deliver duplicate copies of each real, local UDP candidate through an
// in-process fixture, before its description and again after the channel opens.
func (f *pair) open(t *testing.T, offset int64) negotiation {
	t.Helper()
	// Select candidates by conn: an earlier gather may still be completing.
	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	offer := recv(t, f.aMail["offer"])
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	o.Timestamp += offset
	offer = f.a.sign(o)
	var l, r proto.ICECandidate
	var li, ri proto.Envelope
	for l.Conn != o.Conn {
		li, l = hostUDP(t, f.aMail)
	}
	for range 2 {
		deliver(t, f.b, f.bMail.route(), li)
	}
	k := link{f.a.key.ID(), o.Conn}
	eq(t, f.b.early[k].pending, []candidate{{l.MID, l.Candidate}})
	deliver(t, f.b, f.bMail.route(), offer)
	answer := recv(t, f.bMail["answer"])
	for r.Conn != o.Conn {
		ri, r = hostUDP(t, f.bMail)
	}
	for range 2 {
		deliver(t, f.a, f.aMail.route(), ri)
	}
	eq(t, c.candidates.pending, []candidate{{r.MID, r.Candidate}})
	deliver(t, f.a, f.aMail.route(), answer)
	recv(t, c.Ready())
	s := recv(t, f.opened)
	t.Cleanup(func() { s.Close() })
	deliver(t, f.a, f.aMail.route(), answer)
	deliver(t, f.b, f.bMail.route(), li)
	deliver(t, f.a, f.aMail.route(), ri)
	for _, conn := range []*Connection{c, s} {
		eq(t, len(conn.candidates.pending), 0)
		eq(t, len(conn.candidates.seen), 1)
		path, err := conn.Path()
		check(t, err)
		eq(t, path, "direct")
	}
	remote, err := c.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	check(t, err)
	addr := f.b.sockets.udp.GetListenAddresses()[0].(*net.UDPAddr)
	eq(t, int(remote.Remote.Port), addr.Port)
	local := f.a.sockets.udp.GetListenAddresses()[0].(*net.UDPAddr)
	if int(remote.Local.Port) == local.Port {
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
func TestCandidateCopiesAndOfferLifetime(t *testing.T) {
	f := newPair(t)
	past := f.open(t, 120000)
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
		later.info(t)
	}
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
func TestEveryPathChecksEnvelope(t *testing.T) {
	f := newPair(t)
	data := proto.ICECandidate{SignalHeader: f.a.header(link{f.b.key.ID(), randomID()}, "ice"), Candidate: "candidate", MID: "0"}
	valid := f.a.sign(data)
	wrong := frame(valid)
	wrong.Key = f.b.key.ID()
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
	for _, tc := range []struct {
		from proto.Route
		want error
	}{
		{wrong, identity.BadSig}, {badSig, identity.BadSig}, {frame(f.b.sign(data)), identity.UnknownDevice}, {frame(f.a.sign(other)), identity.NotForMe}, {frame(f.a.sign(stale)), identity.Stale}, {frame(f.a.sign(badID)), identity.BadRequest},
	} {
		eq(t, f.b.Receive(t.Context(), Route{}, tc.from), tc.want)
	}
	eq(t, len(f.b.early), 0)
}
func TestCandidateAddOrder(t *testing.T) {
	for _, role := range []string{"offerer", "answerer"} {
		t.Run(role, func(t *testing.T) {
			b := newBatch()
			var added []candidate
			add := func(i webrtc.ICECandidateInit) error {
				added = append(added, candidate{*i.SDPMid, i.Candidate})
				return nil
			}
			c := candidate{"0", "synthetic-candidate"}
			for range 2 {
				check(t, b.receive(c, add))
			}
			eq(t, len(added), 0)
			check(t, b.flush(add))
			eq(t, added, []candidate{c})
			check(t, b.receive(c, add))
			eq(t, added, []candidate{c})
		})
	}
}
