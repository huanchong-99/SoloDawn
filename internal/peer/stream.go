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
// the channel has closed. A message waits until it is read, and the
// connection's channels share one receive window, so a stream that is not
// read until it ends, or closed, holds back the others, ctl included.
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
	r             *io.PipeReader
	open          chan struct{}
	low           chan struct{} // signalled when the queue falls to limit
	done          chan struct{} // closed when the channel has closed
}

// newStream hooks every callback of dc before the channel opens; pion opens
// it only after the channel's handler has returned.
func newStream(c *Connection, dc *webrtc.DataChannel) *Stream {
	r, w := io.Pipe()
	s := &Stream{label: dc.Label(), device: c.Device, dc: dc, buffered: dc.BufferedAmount, limit: streamLimit, r: r,
		open: make(chan struct{}), low: make(chan struct{}, 1), done: make(chan struct{})}
	dc.SetBufferedAmountLowThreshold(s.limit)
	dc.OnBufferedAmountLow(s.signal)
	dc.OnOpen(func() { close(s.open) })
	// A message nobody reads would keep pion's read loop, and with it the
	// closing of this channel, waiting after the connection has ended.
	context.AfterFunc(c.ctx, func() { r.Close() })
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
func (s *Stream) await(ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-s.done:
		return io.ErrClosedPipe
	}
}

// Close closes the channel; a Read after it returns io.ErrClosedPipe.
func (s *Stream) Close() error {
	s.r.Close()
	return s.dc.Close()
}
