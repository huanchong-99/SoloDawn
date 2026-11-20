package peer

import (
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

// A batch holds the candidates of one (sender, conn) until the remote
// description is set, and applies each distinct candidate exactly once
// (contracts/rendezvous.md section 2.3, "Trickle"). It holds at most
// earlyCount, for at most earlyMillis from the first, and drops the rest.
type candidate struct{ mid, text string }
type batch struct {
	seen    map[candidate]bool
	pending []candidate
	ready   bool
	expires int64
}

const (
	earlyCount  = 64
	earlyMillis = 30000
)

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
		c.candidates.receive(v, c.addCandidate)
		return nil
	}
	return p.hold(k, v, now)
}
func (p *Peer) hold(k link, v candidate, now int64) error {
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
		b.expires = now + earlyMillis
		p.early[k] = b
	}
	b.receive(v, nil)
	return nil
}
func (b *batch) receive(v candidate, add func(webrtc.ICECandidateInit)) {
	if b.seen[v] || !b.ready && len(b.pending) == earlyCount {
		return
	}
	b.seen[v] = true
	if b.ready {
		v.add(add)
		return
	}
	b.pending = append(b.pending, v)
}
func (b *batch) flush(add func(webrtc.ICECandidateInit)) {
	b.ready = true
	for _, v := range b.pending {
		v.add(add)
	}
	b.pending = nil
}
func (v candidate) add(f func(webrtc.ICECandidateInit)) {
	f(webrtc.ICECandidateInit{Candidate: v.text, SDPMid: &v.mid})
}
