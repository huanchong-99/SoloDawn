package peer

import (
	"io"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

func TestChannelDispatchByLabel(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	handed := make(chan *Stream, 2)
	p := &Peer{channels: Channels{"test:": func(s *Stream) { handed <- s }, "ctl": func(*Stream) { t.Error("ctl handed over") }}}
	c := &Connection{Device: "device", ctx: t.Context(), close: func() error { t.Error("closed the connection"); return nil }}
	for _, label := range []string{"test:g:member", "test:"} {
		dc, err := pc.CreateDataChannel(label, nil)
		check(t, err)
		p.dispatch(c, dc)
		s := recv(t, handed)
		eq(t, []any{s.Label(), s.Device(), s.dc}, []any{label, "device", dc})
		eq(t, dc.ReadyState(), webrtc.DataChannelStateConnecting)
	}
	// A family key takes no single label, and a single label no family.
	for _, label := range []string{"unknown", "test", "ctl:x", ""} {
		dc, err := pc.CreateDataChannel(label, nil)
		check(t, err)
		p.dispatch(c, dc)
		eq(t, dc.ReadyState(), webrtc.DataChannelStateClosing)
	}
	dc, err := pc.CreateDataChannel("ctl", nil)
	check(t, err)
	p.dispatch(c, dc)
	eq(t, c.dc, dc)
	eq(t, len(handed), 0)
}

func TestNewKeepsItsOwnChannels(t *testing.T) {
	channels := Channels{"test:": func(*Stream) {}}
	p := New(identity.Key{}, nil, nil, nil, nil, nil, channels)
	channels["other:"] = func(*Stream) {}
	eq(t, len(p.channels), 1)
}

// readWithin reads n bytes from s, failing the test after five seconds.
func readWithin(t *testing.T, s io.Reader, n int) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		b := make([]byte, n)
		k, _ := io.ReadFull(s, b)
		got <- string(b[:k])
	}()
	return recv(t, got)
}

// writeWithin writes p to s, failing the test after five seconds.
func writeWithin(t *testing.T, s io.Writer, p []byte) (int, error) {
	t.Helper()
	type result struct {
		n   int
		err error
	}
	got := make(chan result, 1)
	go func() {
		n, err := s.Write(p)
		got <- result{n, err}
	}()
	r := recv(t, got)
	return r.n, r.err
}

// On an open connection a handled label reaches its handler as a stream of
// the connection's device that carries bytes both ways; an unknown label is
// closed, and ctl goes on.
func TestChannelsHandedOverBesideCtl(t *testing.T) {
	f := newPair(t)
	handed := make(chan *Stream, 1)
	f.b.channels = Channels{"test:": func(s *Stream) { handed <- s }}
	n := f.open(t, 0)
	extra := openRemote(t, n.client, "test:g:member")
	s := recv(t, handed)
	eq(t, []any{s.Label(), s.Device()}, []any{"test:g:member", f.a.key.ID()})
	recv(t, extra.open)
	check(t, extra.dc.Send([]byte("to the node")))
	eq(t, readWithin(t, s, 11), "to the node")
	_, err := writeWithin(t, s, []byte("to the client"))
	check(t, err)
	eq(t, string(recv(t, extra.msgs)), "to the client")
	unknown := openRemote(t, n.client, "unknown")
	recv(t, unknown.closed)
	eq(t, len(handed), 0)
	n.info(t)
}

// probeSDP parses; only the kind handler of a test reads it.
const probeSDP = "v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=-\r\nt=0 0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\nc=IN IP4 0.0.0.0\r\na=mid:0\r\n"

func TestProbeOfferKind(t *testing.T) {
	f := newPair(t)
	k := link{f.a.key.ID(), randomID()}
	probe := f.a.sign(proto.Offer{SignalHeader: f.a.header(link{f.b.key.ID(), k.conn}, "offer"), SDP: probeSDP, Probe: true})
	// Unregistered, a probe is refused before anything is remembered.
	eq(t, f.b.Receive(t.Context(), Route{}, frame(probe)), identity.BadRequest)
	seen, err := f.b.db.HasOffer(k.device, k.conn, f.now.Load())
	check(t, err)
	eq(t, seen, false)

	type call struct {
		k    link
		desc webrtc.SessionDescription
	}
	calls := make(chan call, 2)
	answer := f.b.sign(proto.Answer{SignalHeader: f.b.header(k, "answer"), SDP: "probe answer"})
	offerKinds["probe"] = func(p *Peer, k link, desc webrtc.SessionDescription) (*proto.Envelope, error) {
		eq(t, p, f.b)
		calls <- call{k, desc}
		return &answer, nil
	}
	t.Cleanup(func() { delete(offerKinds, "probe") })
	copies := mail()
	deliver(t, f.b, copies.route(), probe)
	got := recv(t, calls)
	eq(t, got.k, k)
	eq(t, []any{got.desc.Type, got.desc.SDP}, []any{webrtc.SDPTypeOffer, probeSDP})
	eq(t, recv(t, copies["answer"]), answer)
	// The once-only rule comes first: a copy reaches no handler.
	deliver(t, f.b, copies.route(), probe)
	eq(t, len(calls), 0)
	eq(t, len(copies["answer"]), 0)
	eq(t, len(f.b.slot), 0)
}
