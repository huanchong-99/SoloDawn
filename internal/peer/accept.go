package peer

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"
)

// steadyListener accepts until it is closed or ctx ends. Any other Accept
// error, such as too many open files, is logged, and after a wait, from 5 ms
// doubling to 1 s, it goes on.
type steadyListener struct {
	net.Listener
	ctx  context.Context
	diag *slog.Logger
	name string
}

// Steady is l for an accept loop that ends at the first error: only a closed
// l or the end of ctx is one. name is l's in the accept.fail lines of diag.
func Steady(ctx context.Context, l net.Listener, diag *slog.Logger, name string) net.Listener {
	return steadyListener{l, ctx, diag, name}
}

func (l steadyListener) Accept() (net.Conn, error) {
	delay := 5 * time.Millisecond
	for {
		conn, err := l.Listener.Accept()
		if err == nil || l.ended(err) {
			return conn, err
		}
		l.diag.Error("accept.fail", "name", l.name, "err", err)
		select {
		case <-l.ctx.Done():
		case <-time.After(delay):
		}
		delay = min(2*delay, time.Second)
	}
}

func (l steadyListener) ended(err error) bool {
	return errors.Is(err, net.ErrClosed) || l.ctx.Err() != nil
}
