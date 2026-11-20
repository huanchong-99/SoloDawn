package peer

import "time"

// waitForAnswer marks an offered connection as unanswered, under the peer
// lock, until its answer is applied (flows/connect.md step 15): pending holds
// the paths that have not reported the target offline. Its deadline is made
// stopped; Offer starts it, for 10 seconds, once a copy of the offer is out.
func (p *Peer) waitForAnswer(c *Connection, paths []Route) {
	c.pending = make(map[string]bool)
	for _, path := range paths {
		c.pending[path.ID] = true
	}
	c.deadline = time.AfterFunc(time.Hour, func() { p.retirePending(c) })
	c.deadline.Stop()
}

func (c *Connection) endWait() {
	if c.pending != nil {
		c.deadline.Stop()
		c.pending = nil
	}
}

func (p *Peer) retirePending(c *Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c.pending != nil {
		c.Close()
	}
}

// Offline records that the path pathID reported target offline. An offer
// still unanswered when every path it was sent on did so is retired at once.
func (p *Peer) Offline(pathID, target string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.live {
		if c.Device == target && c.pending != nil {
			delete(c.pending, pathID)
			if len(c.pending) == 0 {
				c.Close()
			}
		}
	}
}
