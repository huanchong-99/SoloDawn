package peer

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

func TestFramingBoundaries(t *testing.T) {
	c := &Connection{ctx: t.Context(), incoming: make(chan json.RawMessage, 1), waiting: make(map[int64]chan json.RawMessage)}
	for _, msg := range []string{`[]`, `{}`, `{"T":"push"}`, `{"t":null}`, `{"re":3}`, `{"re":3,"ok":false}`, `{"id":3,"re":3,"ok":true}`, `{"re":3,"err":{"code":"bad_request"}}`, `{"re":3,"t":"x","err":{"code":"bad_request","msg":"x"}}`, `{"re":3,"ok":false,"err":{"code":"bad_request","msg":"x"}}`, string([]byte{255})} {
		if err := c.deliver([]byte(msg)); err == nil {
			t.Fatalf("accepted %q", msg)
		}
	}
	push := func(n int) string { return `{"t":"` + strings.Repeat("a", n-8) + `"}` }
	check(t, c.deliver([]byte(push(61440))))
	got, err := c.Read(t.Context())
	check(t, err)
	eq(t, string(got), push(61440))
	eq(t, c.deliver([]byte(push(61441))).Error(), "invalid ctl text")
	eq(t, c.Send(json.RawMessage(push(61441))).Error(), "ctl frame exceeds 60 KiB")
	for _, result := range []string{`"ok":true`, `"err":{"code":"bad_request","msg":"invalid"}`} {
		id, ch := c.reserve()
		msg := string(encode(id))
		data := `{"re":` + msg + "," + result + "}"
		check(t, c.deliver([]byte(data)))
		eq(t, string(recv(t, ch)), data)
		check(t, c.deliver([]byte(data)))
		eq(t, len(ch), 0)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.Read(canceled)
	eq(t, err, context.Canceled)
	c.ctx = canceled
	_, err = c.Read(t.Context())
	eq(t, err, io.EOF)
}
func TestCtlSizeAndReplyRouting(t *testing.T) {
	f := newPair(t)
	n := f.open(t, 0)
	payload := map[string]string{"t": "push", "data": ""}
	payload["data"] = strings.Repeat("a", 61440-len(encode(payload)))
	oversize := map[string]string{"t": "push", "data": payload["data"] + "a"}
	check(t, n.client.Send(payload))
	data, err := n.server.Read(t.Context())
	check(t, err)
	eq(t, len(data), 61440)
	eq(t, string(data), string(encode(payload)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = n.client.Request(ctx, func(id int64) any { return proto.Request{Type: "one", ID: id} })
	eq(t, err, context.Canceled)
	data, err = n.server.Read(t.Context())
	check(t, err)
	eq(t, string(data), `{"t":"one","id":1}`)
	_, err = n.client.Request(t.Context(), func(int64) any { return oversize })
	eq(t, err.Error(), "ctl frame exceeds 60 KiB")
	waiting := make(chan error, 1)
	go func() {
		_, err := n.client.Request(t.Context(), func(id int64) any { return proto.Request{Type: "two", ID: id} })
		waiting <- err
	}()
	_, err = n.server.Read(t.Context())
	check(t, err)
	// A binary message on ctl ends the connection, and with it the request.
	check(t, n.server.dc.Send([]byte("binary")))
	eq(t, recv(t, waiting), io.EOF)
	recv(t, n.server.Done())
}
func TestBackoffSequenceAndWake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		delay := time.Second
		for _, seconds := range []int{1, 2, 4, 8, 16, 32, 60, 60} {
			start := time.Now()
			delay = backoff(t.Context(), nil, delay)
			eq(t, time.Since(start), time.Duration(seconds)*time.Second)
		}
		wake := make(chan struct{}, 1)
		wake <- struct{}{}
		start := time.Now()
		eq(t, backoff(t.Context(), wake, delay), time.Second)
		eq(t, time.Since(start), time.Duration(0))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		eq(t, backoff(ctx, nil, delay), time.Duration(0))
	})
}
func TestNoRouteFailsBeforeOpeningSession(t *testing.T) {
	f := newPair(t)
	f.a.paths = func(string) []Route { return nil }
	eq(t, f.a.attempt(t.Context(), f.b.key.ID(), func(*Connection) { t.Error("called session with no route") }), false)
}

func TestRejectedCtlChannels(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	unordered := false
	limit := uint16(1)
	for _, tc := range []struct {
		label  string
		init   *webrtc.DataChannelInit
		closes bool
	}{
		{"unrelated", nil, false},
		{"ctl", &webrtc.DataChannelInit{Ordered: &unordered}, true},
		{"ctl", &webrtc.DataChannelInit{MaxRetransmits: &limit}, true},
		{"ctl", &webrtc.DataChannelInit{MaxPacketLifeTime: &limit}, true},
	} {
		dc, err := pc.CreateDataChannel(tc.label, tc.init)
		check(t, err)
		closed := make(chan struct{})
		c := &Connection{close: func() error { close(closed); return nil }}
		c.acceptCtl(dc, func(*Connection) { t.Error("opened rejected channel") })
		if tc.closes {
			recv(t, closed)
		} else {
			eq(t, dc.ReadyState(), webrtc.DataChannelStateClosing)
		}
		eq(t, c.dc, (*webrtc.DataChannel)(nil))
	}
	c := &Connection{close: func() error { return nil }}
	for _, want := range []webrtc.DataChannelState{webrtc.DataChannelStateConnecting, webrtc.DataChannelStateClosing} {
		dc, err := pc.CreateDataChannel("ctl", nil)
		check(t, err)
		c.acceptCtl(dc, func(*Connection) {})
		eq(t, dc.ReadyState(), want)
	}
}

func TestReconnectAfterOpen(t *testing.T) {
	f := newPair(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan *Connection, 1)
	done := make(chan struct{})
	wake := make(chan struct{}, 1)
	go func() { f.a.Connect(ctx, f.b.key.ID(), wake, func(c *Connection) { opened <- c }); close(done) }()
	for range 2 {
		offer := recv(t, f.aMail["offer"])
		var o proto.Offer
		check(t, proto.Decode([]byte(offer.Payload), &o))
		deliver(t, f.b, f.bMail.route(), offer)
		answer := recv(t, f.bMail["answer"])
		deliver(t, f.a, f.aMail.route(), answer)
		for _, side := range []struct {
			node        *Peer
			mail, reply mailbox
		}{{f.a, f.bMail, f.aMail}, {f.b, f.aMail, f.bMail}} {
			e, _ := hostUDP(t, side.mail, o.Conn)
			deliver(t, side.node, side.reply.route(), e)
		}
		n := negotiation{recv(t, opened), recv(t, f.opened), offer, answer}
		n.info(t)
		check(t, n.server.Close())
		recv(t, n.client.Done())
		wake <- struct{}{}
	}
	cancel()
	recv(t, done)
}

func TestPathClassification(t *testing.T) {
	host, prflx, srflx, relay := webrtc.ICECandidateTypeHost, webrtc.ICECandidateTypePrflx, webrtc.ICECandidateTypeSrflx, webrtc.ICECandidateTypeRelay
	for _, tc := range []struct {
		local, remote webrtc.ICECandidateType
		want          string
	}{{relay, host, "relay"}, {host, relay, "relay"}, {relay, relay, "relay"}, {host, prflx, "direct"}, {srflx, host, "direct"}} {
		eq(t, classify(tc.local, tc.remote), tc.want)
	}
	c := &Connection{pathChanged: make(chan struct{}, 1)}
	c.reclassify()
	c.reclassify()
	eq(t, len(c.PathChanged()), 1)
}

// Each remote candidate applied after ctl opened signals PathChanged.
func TestPathChangedOnCandidate(t *testing.T) {
	f := newPair(t)
	n := f.open(t, 0)
	recv(t, n.client.PathChanged())
	k := offerLink(t, f.b.key.ID(), n.offer)
	for {
		env := recv(t, f.bMail["ice"])
		var c proto.ICECandidate
		check(t, proto.Decode([]byte(env.Payload), &c))
		if c.Conn == k.conn && strings.Contains(c.Candidate, " tcp ") {
			deliver(t, f.a, f.aMail.route(), env)
			break
		}
	}
	recv(t, n.client.PathChanged())
	eq(t, n.client.Path(), "direct")
}

func TestListenFailures(t *testing.T) {
	_, err := Listen(-1, 0)
	eq(t, err == nil, false)
	_, err = listen(0, "127.0.0.1:-1", loopbackEngine(), ice.UDPMuxFromPortWithLoopback(), ice.UDPMuxFromPortWithIPFilter(loopback))
	eq(t, err == nil, false)
}

// Ending the caller's context while the offer is being sent ends the attempt.
func TestOfferEndsWithItsContext(t *testing.T) {
	f := newPair(t)
	sending := make(chan struct{}, 1)
	f.a.paths = func(string) []Route {
		return []Route{{Send: func(ctx context.Context, _ string, _ proto.Envelope) error {
			select {
			case sending <- struct{}{}:
			default:
			}
			<-ctx.Done()
			return ctx.Err()
		}}}
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-sending; cancel() }()
	_, err := f.a.Offer(ctx, f.b.key.ID())
	eq(t, err, context.Canceled)
}
