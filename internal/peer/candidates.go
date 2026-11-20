package peer

import (
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

// A batch holds the candidates of one (sender, conn) until the remote
// description is set, and applies each distinct candidate exactly once
// (contracts/rendezvous.md section 2.3, "Trickle"). A connection takes at
// most maxCandidates, held or applied; candidates that arrive before their
// description are held for at most earlyMillis from the first, for at most
// earlyAttempts connection attempts of one signer. The rest are dropped.
type candidate struct{ mid, text string }
type batch struct {
	seen    map[candidate]bool
	pending []candidate
	ready   bool
	expires int64
}

const (
	maxCandidates = 64
	earlyMillis   = 30000
	earlyAttempts = 8
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
	if b := p.batchFor(k, now); b != nil {
		b.receive(v, nil)
	}
	return nil
}

// batchFor is the batch that holds k's early candidates; a new one is made
// unless k's signer already has earlyAttempts.
func (p *Peer) batchFor(k link, now int64) *batch {
	if b := p.early[k]; b != nil {
		return b
	}
	n := 0
	for held := range p.early {
		if held.device == k.device {
			n++
		}
	}
	if n == earlyAttempts {
		return nil
	}
	b := newBatch()
	b.expires = now + earlyMillis
	p.early[k] = b
	return b
}
func (b *batch) receive(v candidate, add func(webrtc.ICECandidateInit)) {
	if b.seen[v] || len(b.seen) == maxCandidates {
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
