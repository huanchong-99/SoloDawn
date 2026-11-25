package peer

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The 10 seconds of an unanswered offer run from when Offer has sent its
// first copy, not from when the connection was made.
func TestUnansweredOfferDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &Peer{}
		ctx, cancel := context.WithCancel(t.Context())
		c := &Connection{ctx: ctx, close: func(string) error { cancel(); return nil }}
		p.mu.Lock()
		p.waitForAnswer(c, []Route{{ID: "local"}})
		p.mu.Unlock()
		time.Sleep(time.Hour)
		synctest.Wait()
		eq(t, c.isClosed(), false)
		c.deadline.Reset(10 * time.Second) // what Offer does once a copy is out
		time.Sleep(10*time.Second - time.Nanosecond)
		synctest.Wait()
		eq(t, c.isClosed(), false)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		eq(t, c.isClosed(), true)
	})
}

// Offer starts the deadline only once a copy of the offer has been sent.
func TestDeadlineStartsAtTheFirstSend(t *testing.T) {
	f := newPair(t)
	sending, release := make(chan struct{}), make(chan struct{})
	f.a.paths = func(string) []Route {
		return []Route{{Send: func(_ context.Context, _ string, env proto.Envelope) error {
			if strings.Contains(env.Payload, `"t":"offer"`) {
				close(sending)
				<-release
			}
			return nil
		}}}
	}
	offered := make(chan *Connection)
	go func() {
		c, err := f.a.Offer(t.Context(), f.b.key.ID())
		check(t, err)
		offered <- c
	}()
	recv(t, sending)
	f.a.mu.Lock()
	var c *Connection
	for _, c = range f.a.live {
	}
	f.a.mu.Unlock()
	eq(t, c.deadline.Stop(), false)
	close(release)
	eq(t, recv(t, offered), c)
	t.Cleanup(func() { c.Close() })
	eq(t, c.deadline.Stop(), true)
}

func offerLink(t *testing.T, device string, offer proto.Envelope) link {
	t.Helper()
	var o proto.Offer
	check(t, proto.Decode([]byte(offer.Payload), &o))
	return link{device, o.Conn}
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
		check(t, p.handle(ctx, "offline", data, f.a, func(string, []byte) error { return nil }))
	}
	eq(t, a.handle(ctx, "offline", []byte(`{"t":"offline"}`), f.a, nil).Error(), "proto: a field that is always present is missing")
	eq(t, a.handle(ctx, "ice", []byte(`{"t":"ice"}`), f.a, nil).Error(), "proto: a field that is always present is missing")
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
	f.a.retirePending(current)
	eq(t, current.isClosed(), false)
}
