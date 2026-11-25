package peer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pion/ice/v4"
)

var errAccept = errors.New("too many open files")

// failingListener fails its first n Accepts, then is its Listener, or a
// closed one without it.
type failingListener struct {
	net.Listener
	n int
}

func (l *failingListener) Accept() (net.Conn, error) {
	if l.n > 0 {
		l.n--
		return nil, errAccept
	}
	if l.Listener == nil {
		return nil, fmt.Errorf("accept: %w", net.ErrClosed)
	}
	return l.Listener.Accept()
}

func TestAcceptWaitsBetweenErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logged bytes.Buffer
		failing := &failingListener{}
		l := Steady(t.Context(), failing, slog.New(slog.NewTextHandler(&logged, nil)), "local")
		for range 2 {
			failing.n = 10
			start := time.Now()
			if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
				t.Fatal(err)
			}
			// 5+10+20+40+80+160+320+640 ms, then 1 s twice.
			if got := time.Since(start); got != 3275*time.Millisecond {
				t.Fatal(got)
			}
		}
		if got := strings.Count(logged.String(), `level=ERROR msg=accept.fail name=local err="too many open files"`); got != 20 {
			t.Fatal(got)
		}
	})
}

func TestAcceptEndsWithItsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		l := Steady(ctx, &failingListener{n: 1 << 10}, slog.New(slog.DiscardHandler), "local")
		time.AfterFunc(10*time.Second+time.Millisecond, cancel)
		start := time.Now()
		if _, err := l.Accept(); err != errAccept {
			t.Fatal(err)
		}
		if got := time.Since(start); got != 10*time.Second+time.Millisecond {
			t.Fatal(got)
		}
	})
}

// The ICE-TCP port still takes connections after an Accept error.
func TestICETCPOutlivesAnAcceptError(t *testing.T) {
	saved := tcpListen
	t.Cleanup(func() { tcpListen = saved })
	taken := make(chan struct{}) // single reader: this test
	var addr net.Addr
	tcpListen = func(network, at string) (net.Listener, error) {
		ln, err := saved(network, at)
		addr = ln.Addr()
		return &takenListener{&failingListener{ln, 1}, taken}, err
	}
	s, err := listen(0, "127.0.0.1:0", "test", loopbackEngine(), ice.UDPMuxFromPortWithLoopback(), ice.UDPMuxFromPortWithIPFilter(loopback))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case <-taken:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out after 10s waiting for the connection; saw 1: an accept error")
	}
}

// takenListener closes taken at its first connection.
type takenListener struct {
	net.Listener
	taken chan struct{}
}

func (l *takenListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		close(l.taken)
	}
	return conn, err
}

// Closing the sockets ends the context of the ICE-TCP accept, so that its
// back-off does not outlast Close.
func TestCloseEndsTheICETCPBackoff(t *testing.T) {
	saved := steadyListen
	t.Cleanup(func() { steadyListen = saved })
	var ctx context.Context
	steadyListen = func(c context.Context, l net.Listener, diag *slog.Logger, name string) net.Listener {
		ctx = c
		return saved(c, l, diag, name)
	}
	s, err := listen(0, "127.0.0.1:0", "test", loopbackEngine(), ice.UDPMuxFromPortWithLoopback(), ice.UDPMuxFromPortWithIPFilter(loopback))
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("done before Close")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("not done after Close")
	}
}
