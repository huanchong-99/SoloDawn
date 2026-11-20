package peer

import (
	"context"
	"time"
)

// Connect keeps one offered direction open until ctx ends (flows/connect.md
// section C). Attempts back off 1, 2, 4 … 60 seconds; the back-off starts again
// from 1 second after a connection that opened, and a signal on present (the
// target reported present, or a change of this side's network) ends it at
// once. session runs for each opened connection: hello, sync and commands
// without a response belong to it, not to the transport.
func (p *Peer) Connect(ctx context.Context, device string, present <-chan struct{}, session func(*Connection)) {
	delay := time.Second
	for ctx.Err() == nil {
		if p.attempt(ctx, device, session) {
			delay = time.Second
		}
		delay = backoff(ctx, present, delay)
	}
}
func backoff(ctx context.Context, present <-chan struct{}, delay time.Duration) time.Duration {
	select {
	case <-ctx.Done():
		return 0
	case <-present:
		return time.Second
	case <-time.After(delay):
		return min(2*delay, time.Minute)
	}
}

// attempt offers one connection and reports whether its ctl opened. It
// returns once that connection has closed.
func (p *Peer) attempt(ctx context.Context, device string, session func(*Connection)) bool {
	c, err := p.Offer(ctx, device)
	if err != nil {
		return false
	}
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	select {
	case <-c.Done():
		return false
	case <-c.Ready():
	}
	go session(c)
	<-c.Done()
	return true
}
