package peer

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/pion/webrtc/v4"
)

// remote is a channel the offering side opened, as a browser opens one.
type remote struct {
	dc           *webrtc.DataChannel
	msgs         chan []byte
	open, closed chan struct{}
}

func openRemote(t *testing.T, c *Connection, label string) remote {
	t.Helper()
	dc, err := c.pc.CreateDataChannel(label, nil)
	check(t, err)
	r := remote{dc, make(chan []byte, 512), make(chan struct{}), make(chan struct{})}
	dc.OnOpen(func() { close(r.open) })
	dc.OnClose(func() { close(r.closed) })
	dc.OnMessage(func(m webrtc.DataChannelMessage) { r.msgs <- m.Data })
	return r
}

// A write waits for the open channel, then while more than limit is queued;
// a closed channel ends the wait.
func TestStreamWaitsForRoom(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var queued atomic.Uint64
		s := &Stream{buffered: queued.Load, limit: 10, ctx: context.Background(), open: make(chan struct{}), low: make(chan struct{}, 1), done: make(chan struct{})}
		ready := func() chan error {
			result := make(chan error, 1)
			go func() { result <- s.ready() }()
			synctest.Wait()
			return result
		}
		waiting := ready()
		eq(t, len(waiting), 0)
		close(s.open)
		synctest.Wait()
		eq(t, <-waiting, nil)
		queued.Store(10)
		eq(t, <-ready(), nil)
		queued.Store(11)
		waiting = ready()
		s.signal() // a signal that comes too early is checked again
		synctest.Wait()
		eq(t, len(waiting), 0)
		queued.Store(10)
		s.signal()
		synctest.Wait()
		eq(t, <-waiting, nil)
		// Signals nobody waits for leave one pending.
		s.signal()
		s.signal()
		eq(t, len(s.low), 1)
		<-s.low
		queued.Store(11)
		waiting = ready()
		close(s.done)
		synctest.Wait()
		eq(t, <-waiting, io.ErrClosedPipe)
	})
}

// A channel that closed before it opened, or between the wait and the send,
// ends the write with what was sent.
func TestStreamWriteStopsWhenClosed(t *testing.T) {
	closed := make(chan struct{})
	close(closed)
	s := &Stream{ctx: context.Background(), open: make(chan struct{}), done: closed}
	sent, err := writeWithin(t, s, []byte("never sent"))
	eq(t, []any{sent, err}, []any{0, io.ErrClosedPipe})
	// An empty write sends nothing and waits for nothing.
	sent, err = writeWithin(t, s, nil)
	eq(t, []any{sent, err}, []any{0, nil})

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	dc, err := pc.CreateDataChannel("test:", nil)
	check(t, err)
	s = &Stream{dc: dc, buffered: dc.BufferedAmount, limit: streamLimit, ctx: context.Background(), open: closed, done: make(chan struct{})}
	sent, err = writeWithin(t, s, []byte("never sent"))
	eq(t, []any{sent, err}, []any{0, io.ErrClosedPipe})
}

// readError reads one byte from s, failing the test after five seconds.
func readError(t *testing.T, s io.Reader) error {
	t.Helper()
	got := make(chan error, 1)
	go func() {
		_, err := s.Read(make([]byte, 1))
		got <- err
	}()
	return recv(t, got)
}

// Pion fires neither OnOpen nor OnClose for a channel closed while it is
// being opened. A write that waits for the open still ends with Close or
// with the connection, and the end of the connection ends a read with EOF.
func TestStreamEndsWithoutPionEvents(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	check(t, err)
	t.Cleanup(func() { pc.Close() })
	stream := func(ctx context.Context) *Stream {
		dc, err := pc.CreateDataChannel("test:", nil)
		check(t, err)
		return newStream(&Connection{Device: "device", ctx: ctx}, dc)
	}
	s := stream(t.Context())
	wrote := make(chan error, 1)
	go func() { _, err := s.Write([]byte("never sent")); wrote <- err }()
	check(t, s.Close())
	eq(t, recv(t, wrote), io.ErrClosedPipe)
	eq(t, readError(t, s), io.ErrClosedPipe)

	ctx, end := context.WithCancel(t.Context())
	s = stream(ctx)
	go func() { _, err := s.Write([]byte("never sent")); wrote <- err }()
	end()
	eq(t, recv(t, wrote), io.ErrClosedPipe)
	eq(t, readError(t, s), io.EOF)
}

func TestStreamOverAConnection(t *testing.T) {
	f := newPair(t)
	handed := make(chan *Stream, 1)
	f.b.channels = Channels{"test:": func(s *Stream) { handed <- s }}
	n := f.open(t, 0)
	a := openRemote(t, n.client, "test:a")
	s := recv(t, handed)
	// A write made before the channel opens waits for it; messages carry at
	// most 16 KiB each, in order.
	data := make([]byte, 2*streamChunk+100)
	for i := range data {
		data[i] = byte(i % 251)
	}
	sent, err := writeWithin(t, s, data)
	check(t, err)
	eq(t, sent, len(data))
	var sizes []int
	var got []byte
	for len(got) < len(data) {
		m := recv(t, a.msgs)
		sizes, got = append(sizes, len(m)), append(got, m...)
	}
	eq(t, sizes, []int{streamChunk, streamChunk, 100})
	eq(t, bytes.Equal(got, data), true)
	// Three MiB pass the 1 MiB queue intact.
	big := bytes.Repeat(data, 3<<20/len(data))
	wrote := make(chan error, 1)
	go func() { _, err := s.Write(big); wrote <- err }()
	for got = nil; len(got) < len(big); {
		got = append(got, recv(t, a.msgs)...)
	}
	check(t, recv(t, wrote))
	eq(t, bytes.Equal(got, big), true)
	check(t, a.dc.Send([]byte("ab")))
	check(t, a.dc.Send([]byte("cd")))
	eq(t, readWithin(t, s, 4), "abcd")
	// Closed by the other side: the stream ends.
	check(t, a.dc.Close())
	recv(t, s.done)
	eq(t, readError(t, s), io.EOF)
	_, err = writeWithin(t, s, []byte("late"))
	eq(t, err, io.ErrClosedPipe)

	// Closed by this side: the other side sees it.
	b := openRemote(t, n.client, "test:b")
	s = recv(t, handed)
	recv(t, b.open)
	check(t, s.Close())
	recv(t, b.closed)
	eq(t, readError(t, s), io.ErrClosedPipe)

	// A message nobody reads does not keep the channel from closing with its
	// connection.
	c := openRemote(t, n.client, "test:c")
	s = recv(t, handed)
	recv(t, c.open)
	check(t, c.dc.Send([]byte("never read")))
	n.info(t)
	check(t, n.server.Close())
	recv(t, s.done)
	eq(t, readError(t, s), io.EOF)
}
