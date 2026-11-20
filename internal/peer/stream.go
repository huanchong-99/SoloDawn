package peer

import (
	"context"
	"io"

	"github.com/pion/webrtc/v4"
)

const (
	streamChunk = 16_384  // bytes per message at most
	streamLimit = 1 << 20 // contracts/datachannel.md section 7: 1 MiB
)

// A Stream is a data channel other than ctl as a stream of bytes: the
// terminal (contracts/datachannel.md section 1) and, from step 2, watch.
//
// Read returns the bytes of the channel's messages in order, and io.EOF once
// the channel or its connection has ended. Its handler keeps reading until
// Read returns an error, and one that needs no input runs
// io.Copy(io.Discard, s): a message waits until it is read, so a single
// unread message keeps both sides from seeing the channel close, and the
// connection's channels share one receive window, so unread messages hold
// back the others, ctl included. Ctl recovers only seconds after the stream
// is read again, the later the longer it was held back (0.2 to 5 s measured).
//
// Write waits until the channel is open, sends p as messages of at most
// 16 KiB, and waits while more than 1 MiB is queued, so that a writer fed
// from a pseudo-terminal stops reading it (contracts/datachannel.md
// section 7). One goroutine writes at a time.
type Stream struct {
	label, device string
	dc            *webrtc.DataChannel
	buffered      func() uint64
	limit         uint64
	ctx           context.Context // ends with the connection or with Close
	cancel        context.CancelFunc
	r             *io.PipeReader
	open          chan struct{}
	low           chan struct{} // signalled when the queue falls to limit
	done          chan struct{} // closed when the channel has closed
}

// newStream hooks every callback of dc before the channel opens; pion opens
// it only after the call that announced it has returned.
func newStream(c *Connection, dc *webrtc.DataChannel) *Stream {
	r, w := io.Pipe()
	ctx, cancel := context.WithCancel(c.ctx)
	s := &Stream{label: dc.Label(), device: c.Device, dc: dc, buffered: dc.BufferedAmount, limit: streamLimit, ctx: ctx, cancel: cancel, r: r,
		open: make(chan struct{}), low: make(chan struct{}, 1), done: make(chan struct{})}
	dc.SetBufferedAmountLowThreshold(s.limit)
	dc.OnBufferedAmountLow(s.signal)
	dc.OnOpen(func() { close(s.open) })
	// The end of the connection ends Read with io.EOF, and frees pion's read
	// loop from a message nobody reads, so that the channel can close.
	context.AfterFunc(c.ctx, func() { w.Close() })
	dc.OnMessage(func(m webrtc.DataChannelMessage) { w.Write(m.Data) })
	dc.OnClose(func() { w.Close(); close(s.done) })
	return s
}

// signal wakes a waiting write; a signal while one is pending adds nothing.
func (s *Stream) signal() {
	select {
	case s.low <- struct{}{}:
	default:
	}
}

// Label is the channel's label, such as "pty:<g>:<member>".
func (s *Stream) Label() string { return s.label }

// Device is the device the connection was authenticated for.
func (s *Stream) Device() string { return s.device }

func (s *Stream) Read(p []byte) (int, error) { return s.r.Read(p) }

func (s *Stream) Write(p []byte) (int, error) {
	sent := 0
	for sent < len(p) {
		if err := s.ready(); err != nil {
			return sent, err
		}
		n := min(len(p)-sent, streamChunk)
		if err := s.dc.Send(p[sent : sent+n]); err != nil {
			return sent, err
		}
		sent += n
	}
	return sent, nil
}

// ready waits until the channel is open and holds at most limit bytes not yet
// sent.
func (s *Stream) ready() error {
	err := s.await(s.open)
	for err == nil && s.buffered() > s.limit {
		err = s.await(s.low)
	}
	return err
}

// await ends with ch, or with the channel, the connection or Close: pion fires
// neither OnOpen nor OnClose for a channel closed while it is being opened.
func (s *Stream) await(ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-s.done:
		return io.ErrClosedPipe
	case <-s.ctx.Done():
		return io.ErrClosedPipe
	}
}

// Close closes the channel; a Read or Write after it returns
// io.ErrClosedPipe.
func (s *Stream) Close() error {
	s.r.Close()
	s.cancel()
	return s.dc.Close()
}
