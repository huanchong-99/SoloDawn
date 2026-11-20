package peer

import "time"

const answerTimeout = 10 * time.Second

// pending is set only on an outbound offer, under the peer lock.
type answerWait struct {
	until time.Time
	timer *time.Timer
	paths map[string]bool
}

func (p *Peer) waitForAnswer(c *Connection, paths []Route) {
	w := &answerWait{paths: make(map[string]bool)}
	for _, path := range paths {
		w.paths[path.ID] = true
	}
	c.pending = w
	w.until = time.Now().Add(answerTimeout)
	w.timer = time.AfterFunc(time.Until(w.until), func() { p.retirePending(c) })
}

func (c *Connection) endWait() {
	if c.pending != nil {
		c.pending.timer.Stop()
		c.pending = nil
	}
}

func (p *Peer) retirePending(c *Connection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c.pending == nil {
		return false
	}
	c.Close()
	return true
}

// Offline records the target-only report from this particular path.
func (p *Peer) Offline(pathID, target string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.live {
		if c.Device == target && c.pending != nil {
			c.pending.offline(c, pathID)
		}
	}
}

func (w *answerWait) offline(c *Connection, pathID string) {
	delete(w.paths, pathID)
	if len(w.paths) == 0 {
		c.Close()
	}
}
