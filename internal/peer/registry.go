package peer

import (
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

// The dispatch of an answering node: the kinds of offer it answers
// (contracts/rendezvous.md section 2.3) and the data channels it takes
// (contracts/datachannel.md section 1). An offer of a kind nobody registered
// is refused with bad_request, and a channel with a label nobody handles is
// closed.

// An offerKind answers the first application of an offer of its kind. It
// runs under the peer lock, after the envelope, the description and the
// once-only rule have passed; paths is the signer's, taken before the lock.
type offerKind func(p *Peer, k link, desc webrtc.SessionDescription, paths []Route) (*proto.Envelope, error)

// offerKinds is keyed by the kind kindOf names: "ordinary" or "probe". The
// file of each kind registers it from its own init.
var offerKinds = map[string]offerKind{}

func kindOf(o proto.Offer) string {
	if o.Probe {
		return "probe"
	}
	return "ordinary"
}

// Channels are a node's handlers of the data channels other than ctl, which
// is built in. A key is a whole label without a colon, or a family of labels
// up to and including their first colon, such as "pty:" for every
// "pty:<g>:<member>"; a key "ctl" is never used.
//
// A handler runs on a goroutine of its own, not under the peer lock. Its
// stream is set up before the channel opens, so it misses no message.
type Channels map[string]func(*Stream)

func (p *Peer) dispatch(c *Connection, dc *webrtc.DataChannel) {
	key := dc.Label()
	if key == "ctl" {
		c.acceptCtl(dc, p.inbound)
		return
	}
	if family, _, found := strings.Cut(key, ":"); found {
		key = family + ":"
	}
	if take := p.channels[key]; take != nil {
		// Before pion opens the channel, which it does once dispatch returns.
		s := newStream(c, dc)
		go take(s)
		return
	}
	dc.Close()
}
