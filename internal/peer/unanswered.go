package peer

import "time"

// answerWait is set only on an offered connection, under the peer lock, until
// its answer is applied (flows/connect.md step 15).
type answerWait struct {
	timer *time.Timer
	paths map[string]bool // the paths that have not reported the target offline
}

func (p *Peer) waitForAnswer(c *Connection, paths []Route) {
	w := &answerWait{paths: make(map[string]bool)}
	for _, path := range paths {
		w.paths[path.ID] = true
	}
	w.timer = time.AfterFunc(10*time.Second, func() { p.retirePending(c) })
	c.pending = w
}

func (c *Connection) endWait() {
	if c.pending != nil {
		c.pending.timer.Stop()
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
			delete(c.pending.paths, pathID)
			if len(c.pending.paths) == 0 {
				c.Close()
			}
		}
	}
}
