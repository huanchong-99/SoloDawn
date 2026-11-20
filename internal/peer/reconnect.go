package peer

import (
	"context"
	"time"
)

// after is the back-off that follows an attempt whose ctl stayed open for
// open: 1 second again after 60 seconds open, delay otherwise
// (flows/connect.md step 15).
func after(delay, open time.Duration) time.Duration {
	if open >= time.Minute {
		return time.Second
	}
	return delay
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
