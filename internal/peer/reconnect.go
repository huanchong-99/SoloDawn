package peer

import (
	"context"
	"time"
)

// Connect maintains one offered direction until cancellation. present is
// notified on target presence or a local network change. The session callback
// starts fresh on every open: hello, sync and unacknowledged commands belong
// to its caller, not the transport.
func (p *Peer) Connect(ctx context.Context, device string, present <-chan struct{}, session func(*Connection)) {
	delay := time.Second
	for ctx.Err() == nil {
		switch p.attempt(ctx, device, present, session) {
		case retryNow:
			delay = time.Second
			continue
		case wasOpen:
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

type attemptResult int

const (
	failed attemptResult = iota
	wasOpen
	retryNow
)

func (p *Peer) attempt(ctx context.Context, device string, present <-chan struct{}, session func(*Connection)) attemptResult {
	c, err := p.Offer(ctx, device)
	if err != nil {
		return failed
	}
	defer c.Close()
	select {
	case <-ctx.Done():
		return failed
	case <-c.Done():
		return failed
	case <-present:
		return retryNow
	case <-c.Ready():
	}
	return runSession(ctx, c, session)
}
func runSession(ctx context.Context, c *Connection, session func(*Connection)) attemptResult {
	go session(c)
	select {
	case <-ctx.Done():
	case <-c.Done():
	}
	return wasOpen
}
