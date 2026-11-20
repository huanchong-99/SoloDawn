package peer

import (
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

type handover struct {
	p  *Peer
	c  *Connection
	dc *webrtc.DataChannel
}

// handed receives every channel of the family "test:", which this file
// registers as a terminal or watch file will register its own.
var handed = make(chan handover, 8)

func init() {
	Channels["test:"] = func(p *Peer, c *Connection, dc *webrtc.DataChannel) { handed <- handover{p, c, dc} }
}

func TestChannelDispatchByLabel(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	p := &Peer{}
	c := &Connection{close: func() error { t.Error("closed the connection"); return nil }}
	for _, label := range []string{"test:g:member", "test:"} {
		dc, err := pc.CreateDataChannel(label, nil)
		check(t, err)
		p.dispatch(c, dc)
		eq(t, recv(t, handed), handover{p, c, dc})
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

// On an open connection a registered label reaches its handler with the
// connection it arrived on, an unknown one is closed, and ctl goes on.
func TestChannelsHandedOverBesideCtl(t *testing.T) {
	f := newPair(t)
	n := f.open(t, 0)
	extra, err := n.client.pc.CreateDataChannel("test:g:member", nil)
	check(t, err)
	opened := make(chan struct{})
	extra.OnOpen(func() { close(opened) })
	got := recv(t, handed)
	eq(t, got.p, f.b)
	eq(t, got.c, n.server)
	eq(t, got.dc.Label(), "test:g:member")
	recv(t, opened)
	unknown, err := n.client.pc.CreateDataChannel("unknown", nil)
	check(t, err)
	closed := make(chan struct{})
	unknown.OnClose(func() { close(closed) })
	recv(t, closed)
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
