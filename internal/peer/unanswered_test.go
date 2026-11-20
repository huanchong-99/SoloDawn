package peer

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestUnansweredOfferDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &Peer{}
		ctx, cancel := context.WithCancel(t.Context())
		c := &Connection{ctx: ctx, close: func() error { cancel(); return nil }}
		p.waitForAnswer(c, []Route{{ID: "local"}})
		time.Sleep(10*time.Second - time.Nanosecond)
		synctest.Wait()
		eq(t, c.isClosed(), false)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		eq(t, c.isClosed(), true)
	})
}

func offerLink(t *testing.T, device string, offer proto.Envelope) link {
	t.Helper()
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	return link{device, o.Conn}
}

func TestAnswerAtDeadlineDoesNotReachICE(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		c := &Connection{ctx: ctx, close: func() error { cancel(); return nil }}
		k := link{"device", randomID()}
		p := &Peer{live: map[link]*Connection{k: c}}
		p.waitForAnswer(c, []Route{{ID: "local"}})
		eq(t, p.receiveAnswer(k, []byte(`{"t":"answer"}`)), identity.BadRequest)
		c.pending.until = time.Now() // Deadline reached, but callback hasn't run yet.
		h := proto.SignalHeader{PayloadHeader: proto.PayloadHeader{Type: "answer", Nonce: proto.ID(randomID()), Timestamp: time.Now().UnixMilli(), To: "local"}, Conn: k.conn}
		check(t, p.receiveAnswer(k, encode(proto.Answer{SignalHeader: h, SDP: "late"})))
		eq(t, c.isClosed(), true)
		c.endWait()
	})
}

func TestOfflineReportsApplyOnlyToThisUnansweredOffer(t *testing.T) {
	f := newPair(t)
	a, _, err := testPath(t, challenge, ready)
	check(t, err)
	b, _, err := testPath(t, challenge, ready)
	check(t, err)
	left, right := a.Route(), b.Route()
	if left.ID == right.ID {
		t.Fatal("distinct paths share an ID")
	}
	second := mail()
	left.Send, right.Send = f.aMail.route().Send, second.route().Send
	f.a.paths = func(string) []Route { return []Route{left, right} }

	device := f.b.key.ID()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { f.a.Connect(ctx, device, nil, func(*Connection) { t.Error("unexpected open") }); close(done) }()
	defer func() { cancel(); recv(t, done) }()
	offer := recv(t, f.aMail["offer"])
	eq(t, recv(t, second["offer"]), offer)
	first := offerLink(t, device, offer)
	f.a.mu.Lock()
	c := f.a.live[first]
	f.a.mu.Unlock()
	deliver(t, f.b, f.bMail.route(), offer)
	late := recv(t, f.bMail["answer"])
	report := func(p *Path, key string) {
		t.Helper()
		data := encode(proto.Offline{Type: "offline", Key: key})
		check(t, p.handle(ctx, "offline", data, f.a, func(string, []byte) error { return nil }, nil))
	}
	eq(t, a.handle(ctx, "offline", []byte(`{"t":"offline"}`), f.a, nil, nil).Error(), "proto: a field that is always present is missing")
	eq(t, a.handle(ctx, "ice", []byte(`{"t":"ice"}`), f.a, nil, nil).Error(), "proto: a field that is always present is missing")
	report(a, device)
	report(a, device)
	report(b, f.a.key.ID())
	f.a.Offline("unused-path", device)
	eq(t, c.isClosed(), false)
	start := time.Now()
	report(b, device)
	eq(t, c.isClosed(), true)
	next := recv(t, second["offer"])
	if time.Since(start) < time.Second {
		t.Fatal("all-offline retry skipped back-off")
	}
	eq(t, recv(t, f.aMail["offer"]), next)
	k := offerLink(t, device, next)
	if first == k {
		t.Fatal("retry reused retired conn")
	}
	deliver(t, f.a, left, late)
	f.a.mu.Lock()
	current := f.a.live[k]
	f.a.mu.Unlock()
	if current.pc.RemoteDescription() != nil || c.pc.RemoteDescription() != nil {
		t.Fatal("applied late answer")
	}
	deliver(t, f.b, f.bMail.route(), next)
	deliver(t, f.a, right, recv(t, f.bMail["answer"]))
	report(a, device)
	report(b, device)
	eq(t, current.isClosed(), false)
	eq(t, f.a.retirePending(current), false)
}
