package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

// Messages that are not ctl frames of the contract and carry no request id
// are dropped; responses go to their request once via Read; Read ends with its
// context or the connection.
func TestFramingBoundaries(t *testing.T) {
	c := &Connection{ctx: t.Context(), incoming: make(chan json.RawMessage, 4), waiting: make(map[int64]chan json.RawMessage)}
	push := func(n int) string { return `{"t":"` + strings.Repeat("a", n-8) + `"}` }
	// A response without a valid result does not reach the request waiting for it.
	id, waiting := c.reserve()
	re := `{"re":` + fmt.Sprint(id)
	for _, msg := range []string{`[]`, `{}`, `{"t":"x"}{}`, `{"T":"push"}`, `{"t":null}`, re + `}`, re + `,"ok":false}`, re + `,"err":{"code":"bad_request"}}`, re + `,"t":"x","err":{"code":"bad_request","msg":"x"}}`, re + `,"ok":false,"err":{"code":"bad_request","msg":"x"}}`, string([]byte{255}), push(61441)} {
		c.deliver([]byte(msg))
		eq(t, len(c.incoming), 0)
		eq(t, len(waiting), 0)
	}
	c.deliver([]byte(re + `,"ok":true}`))
	eq(t, len(waiting), 0)
	c.deliver([]byte(push(61440)))
	eq(t, len(c.incoming), 2)
	got, err := c.Read(t.Context())
	check(t, err)
	eq(t, string(recv(t, waiting)), re+`,"ok":true}`)
	eq(t, string(got), push(61440))
	eq(t, c.Send(json.RawMessage(push(61441))).Error(), "ctl frame exceeds 60 KiB")
	for _, result := range []string{`"ok":true`, `"err":{"code":"bad_request","msg":"invalid"}`} {
		id, ch := c.reserve()
		data := `{"re":` + string(encode(id)) + "," + result + "}"
		c.deliver([]byte(data))
		c.deliver([]byte(data))
		c.deliver([]byte(`{"t":"marker"}`))
		marker, err := c.Read(t.Context())
		check(t, err)
		eq(t, string(marker), `{"t":"marker"}`)
		eq(t, string(recv(t, ch)), data)
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

// On an open connection a message that is not a ctl frame of the contract is
// answered bad_request when it carries a request id and dropped otherwise, a
// binary message is dropped, and the connection stays open
// (contracts/datachannel.md section 2).
func TestRefusedMessagesKeepTheConnection(t *testing.T) {
	f := newPair(t)
	n := f.open(t, 0)
	refused := func(id int64) []string {
		big := `{"t":"x","id":` + fmt.Sprint(id) + `,"data":"` + strings.Repeat("a", 61440) + `"}`
		return []string{`{"id":` + fmt.Sprint(id) + `}`, `{"id":` + fmt.Sprint(id) + `,"re":5,"ok":true}`, big}
	}
	stop := readReplies(t.Context(), n.client)
	for i := range 3 {
		id, ch := n.client.reserve()
		check(t, n.client.dc.SendText(refused(id)[i]))
		eq(t, string(recv(t, ch)), fmt.Sprintf(`{"re":%d,"err":{"code":"bad_request","msg":"the message is not a ctl frame of the contract"}}`, id))
	}
	stop()
	for _, msg := range []string{`[]`, `{}`, `{"re":99,"ok":false}`, string([]byte{255})} {
		check(t, n.client.dc.SendText(msg))
	}
	check(t, n.client.dc.Send([]byte(`{"t":"binary"}`)))
	check(t, n.client.Send(proto.RVFrame{Type: "after"}))
	data, err := n.server.Read(t.Context())
	check(t, err)
	eq(t, string(data), `{"t":"after"}`)
	n.info(t)
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
	stop := readReplies(t.Context(), n.client)
	waiting := make(chan error, 1)
	go func() {
		_, err := n.client.Request(t.Context(), func(id int64) any { return proto.Request{Type: "two", ID: id} })
		waiting <- err
	}()
	data, err = n.server.Read(t.Context())
	check(t, err)
	// A binary message on ctl is dropped; the request is still answered.
	check(t, n.server.dc.Send([]byte("binary")))
	var q proto.Request
	check(t, proto.Decode(data, &q))
	check(t, n.server.Send(proto.Success{Re: q.ID, OK: true}))
	check(t, recv(t, waiting))
	stop()
	n.info(t)
	stop = readReplies(t.Context(), n.client)
	defer stop()
	go func() {
		_, err := n.client.Request(t.Context(), func(id int64) any { return proto.Request{Type: "three", ID: id} })
		waiting <- err
	}()
	_, err = n.server.Read(t.Context())
	check(t, err)
	check(t, n.server.Close())
	eq(t, recv(t, waiting), io.EOF)
}

// Pushes sent before a request's answer are returned by Read before Request
// wakes, and Request wakes only once Read is called again after the push.
func TestPushesDeliveredBeforeAnswer(t *testing.T) {
	f := newPair(t)
	n := f.open(t, 0)
	reply := make(chan json.RawMessage, 1) // single reader: TestPushesDeliveredBeforeAnswer
	go func() {
		ans, err := n.client.Request(t.Context(), func(id int64) any { return proto.Request{Type: "sync", ID: id} })
		check(t, err)
		reply <- ans
	}()
	reqRaw, err := n.server.Read(t.Context())
	check(t, err)
	var q proto.Request
	check(t, proto.Decode(reqRaw, &q))
	check(t, n.server.Send(proto.RVFrame{Type: "rows"}))
	check(t, n.server.Send(proto.Success{Re: q.ID, OK: true}))
	waitQueued(t, n.client, 2)
	eq(t, len(reply), 0)
	push, err := n.client.Read(t.Context())
	check(t, err)
	eq(t, string(push), `{"t":"rows"}`)
	eq(t, len(reply), 0)
	stop := readReplies(t.Context(), n.client)
	defer stop()
	eq(t, string(recv(t, reply)), fmt.Sprintf(`{"re":%d,"ok":true}`, q.ID))
}

func waitQueued(t *testing.T, c *Connection, want int) {
	t.Helper()
	poll(t, 4*time.Second, fmt.Sprintf("%d queued frames", want), func() string {
		if n := len(c.incoming); n != want {
			return fmt.Sprintf("%d queued", n)
		}
		return ""
	})
}

func readReplies(ctx context.Context, c *Connection) func() {
	rctx, stop := context.WithCancel(ctx)
	done := make(chan struct{}) // single reader: returned stop function
	go func() {
		defer close(done)
		for {
			if _, err := c.Read(rctx); err != nil {
				return
			}
		}
	}()
	return func() {
		stop()
		<-done
	}
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
	eq(t, f.a.attempt(t.Context(), f.b.key.ID(), func(*Connection) { t.Error("called session with no route") }), time.Duration(0))
}

// rawOffer negotiates with f.b, signed as f.a, from a pion connection of the
// test that opens a data channel labelled ctl for each init. It returns the
// test's channels and f.b's side of the connection.
func (f *pair) rawOffer(t *testing.T, inits ...*webrtc.DataChannelInit) ([]*webrtc.DataChannel, *Connection) {
	t.Helper()
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(loopbackEngine())).NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	var chans []*webrtc.DataChannel
	for _, init := range inits {
		dc, err := pc.CreateDataChannel("ctl", init)
		check(t, err)
		chans = append(chans, dc)
	}
	k := link{f.a.key.ID(), randomID()}
	to := link{f.b.key.ID(), k.conn}
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			f.b.Receive(t.Context(), Route{}, frame(f.a.sign(proto.ICECandidate{SignalHeader: f.a.header(to, "ice"), Candidate: i.ToJSON().Candidate, MID: *i.ToJSON().SDPMid})))
		}
	})
	offer, err := pc.CreateOffer(nil)
	check(t, err)
	check(t, pc.SetLocalDescription(offer))
	deliver(t, f.b, f.bMail.route(), f.a.sign(proto.Offer{SignalHeader: f.a.header(to, "offer"), SDP: offer.SDP}))
	var a proto.Answer
	check(t, proto.Decode([]byte(recv(t, f.bMail["answer"]).Payload), &a))
	check(t, pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: a.SDP}))
	_, r := hostUDP(t, f.bMail, k.conn)
	check(t, pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: r.Candidate, SDPMid: &r.MID}))
	f.b.mu.Lock()
	defer f.b.mu.Unlock()
	return chans, f.b.live[k]
}

// A ctl channel that the offering side opened unordered or with partial
// reliability ends the connection; a second ctl channel is closed and the
// first goes on.
func TestRejectedCtlChannels(t *testing.T) {
	unordered := false
	limit := uint16(1)
	for _, init := range []*webrtc.DataChannelInit{
		{Ordered: &unordered},
		{MaxRetransmits: &limit},
		{MaxPacketLifeTime: &limit},
	} {
		f := newPair(t)
		_, c := f.rawOffer(t, init)
		recv(t, c.Done())
		select {
		case <-c.Ready():
			t.Fatal("a rejected ctl opened")
		default:
		}
		eq(t, len(f.opened), 0)
	}
	f := newPair(t)
	chans, c := f.rawOffer(t, nil, nil)
	closed := make(chan struct{})
	chans[1].OnClose(func() { close(closed) })
	recv(t, c.Ready())
	recv(t, closed)
	eq(t, chans[0].ReadyState(), webrtc.DataChannelStateOpen)
	eq(t, recv(t, f.opened), c)
}

// connectOnce answers the next offer of a's Connect and returns the
// connection once it is open at both ends.
func (f *pair) connectOnce(t *testing.T, opened chan *Connection) negotiation {
	t.Helper()
	offer := recv(t, f.aMail["offer"])
	o := offerLink(t, f.b.key.ID(), offer)
	deliver(t, f.b, f.bMail.route(), offer)
	answer := recv(t, f.bMail["answer"])
	deliver(t, f.a, f.aMail.route(), answer)
	e, _ := hostUDP(t, f.bMail, o.conn)
	deliver(t, f.a, f.aMail.route(), e)
	e, _ = hostUDP(t, f.aMail, o.conn)
	deliver(t, f.b, f.bMail.route(), e)
	return negotiation{recv(t, opened), recv(t, f.opened), offer, answer}
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
		n := f.connectOnce(t, opened)
		n.info(t)
		check(t, n.server.Close())
		recv(t, n.client.Done())
		wake <- struct{}{}
	}
	cancel()
	recv(t, done)
}

// A connection that closes before it was open for 60 seconds keeps the
// back-off it had: after a failed attempt (1 s, then 2 s) and a short
// connection, the next offer waits 2 seconds, not 1.
func TestBackoffKeptAfterShortConnection(t *testing.T) {
	f := newPair(t)
	var failed atomic.Bool
	mailRoute := f.aMail.route()
	f.a.paths = func(string) []Route {
		return []Route{{Send: func(ctx context.Context, to string, env proto.Envelope) error {
			if strings.Contains(env.Payload, `"t":"offer"`) && !failed.Swap(true) {
				return errors.New("first offer lost")
			}
			return mailRoute.Send(ctx, to, env)
		}}}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan *Connection, 1)
	done := make(chan struct{})
	go func() { f.a.Connect(ctx, f.b.key.ID(), nil, func(c *Connection) { opened <- c }); close(done) }()
	n := f.connectOnce(t, opened)
	check(t, n.server.Close())
	recv(t, n.client.Done())
	closed := time.Now()
	select {
	case <-f.aMail["offer"]:
		if time.Since(closed) < 1500*time.Millisecond {
			t.Fatalf("offered again after %v", time.Since(closed))
		}
	case <-time.After(5 * time.Second):
		t.Fatal(timedOut(5*time.Second, "a new offer", nil))
	}
	cancel()
	recv(t, done)
}

func TestBackoffAfterAnAttempt(t *testing.T) {
	eq(t, after(8*time.Second, 0), 8*time.Second)
	eq(t, after(8*time.Second, time.Minute-time.Nanosecond), 8*time.Second)
	eq(t, after(8*time.Second, time.Minute), time.Second)
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
