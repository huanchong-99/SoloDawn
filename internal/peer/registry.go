package peer

import (
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

// The dispatch of an answering node: the kinds of offer it answers
// (contracts/rendezvous.md section 2.3) and the data channels it takes
// (contracts/datachannel.md section 1). Each file that handles one registers
// it from its own init; an offer of a kind nobody registered is refused with
// bad_request, and a channel with a label nobody registered is closed.

// An offerKind answers the first application of an offer of its kind. It
// runs under the peer lock, after the envelope, the description and the
// once-only rule have passed.
type offerKind func(p *Peer, k link, desc webrtc.SessionDescription) (*proto.Envelope, error)

// offerKinds is keyed by the kind kindOf names: "ordinary" or "probe".
var offerKinds = map[string]offerKind{}

func kindOf(o proto.Offer) string {
	if o.Probe {
		return "probe"
	}
	return "ordinary"
}

// A Channel takes a data channel that the offering side opened on c.
type Channel func(p *Peer, c *Connection, dc *webrtc.DataChannel)

// Channels is keyed by a whole label without a colon, such as "ctl", or by the
// family of labels that share the text up to and including their first colon,
// such as "pty:" for every "pty:<g>:<member>".
var Channels = map[string]Channel{}

func (p *Peer) dispatch(c *Connection, dc *webrtc.DataChannel) {
	key := dc.Label()
	if family, _, found := strings.Cut(key, ":"); found {
		key = family + ":"
	}
	if take := Channels[key]; take != nil {
		take(p, c, dc)
		return
	}
	dc.Close()
}
