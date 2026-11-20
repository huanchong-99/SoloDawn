package peer

import (
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

type candidate struct{ mid, text string }
type batch struct {
	seen    map[candidate]bool
	pending []candidate
	ready   bool
	expires int64
}

func (p *Peer) take(k link) *batch {
	b := p.early[k]
	delete(p.early, k)
	if b == nil {
		return newBatch()
	}
	return b
}
func newBatch() *batch { return &batch{seen: make(map[candidate]bool)} }
func (p *Peer) receiveCandidate(k link, data []byte, now int64) error {
	var i proto.ICECandidate
	if proto.Decode(data, &i) != nil {
		return identity.BadRequest
	}
	v := candidate{i.MID, i.Candidate}
	if c := p.live[k]; c != nil {
		if c.isClosed() {
			return nil
		}
		return c.candidates.receive(v, c.pc.AddICECandidate)
	}
	return p.hold(k, v, i.Timestamp, now)
}
func (p *Peer) hold(k link, v candidate, ts, now int64) error {
	if _, old := p.retired[k]; old {
		return nil
	}
	seen, err := p.db.HasOffer(k.device, k.conn, now)
	if seen || err != nil {
		return err
	}
	b := p.early[k]
	if b == nil {
		b = newBatch()
		p.early[k] = b
	}
	b.expires = max(b.expires, ts+120000)
	return b.receive(v, nil)
}
func (b *batch) receive(v candidate, add func(webrtc.ICECandidateInit) error) error {
	if b.seen[v] {
		return nil
	}
	b.seen[v] = true
	if b.ready {
		return v.add(add)
	}
	b.pending = append(b.pending, v)
	return nil
}
func (b *batch) flush(add func(webrtc.ICECandidateInit) error) error {
	b.ready = true
	for _, v := range b.pending {
		if err := v.add(add); err != nil {
			return err
		}
	}
	b.pending = nil
	return nil
}
func (v candidate) add(f func(webrtc.ICECandidateInit) error) error {
	if err := f(webrtc.ICECandidateInit{Candidate: v.text, SDPMid: &v.mid}); err != nil {
		return identity.BadRequest
	}
	return nil
}
