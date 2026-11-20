package peer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

const ctlLimit = 61_440 // 60 KiB

type Connection struct {
	Device     string
	pc         *webrtc.PeerConnection
	ctx        context.Context
	close      func() error
	ready      chan struct{}
	incoming   chan json.RawMessage
	mu         sync.Mutex
	dc         *webrtc.DataChannel
	next       int64
	waiting    map[int64]chan json.RawMessage
	answer     *proto.Envelope
	candidates *batch
}

func newConnection(pc *webrtc.PeerConnection, device string) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connection{
		Device: device, pc: pc, ctx: ctx, ready: make(chan struct{}),
		incoming: make(chan json.RawMessage, 16), waiting: make(map[int64]chan json.RawMessage),
		close: sync.OnceValue(func() error { cancel(); return pc.Close() }),
	}
}
func (c *Connection) Ready() <-chan struct{} { return c.ready }
func (c *Connection) Done() <-chan struct{}  { return c.ctx.Done() }
func (c *Connection) Close() error           { return c.close() }
func (c *Connection) isClosed() bool         { return c.ctx.Err() != nil }

func (c *Connection) acceptCtl(dc *webrtc.DataChannel, onOpen func(*Connection)) {
	if dc.Label() != "ctl" {
		dc.Close()
		return
	}
	if !dc.Ordered() || dc.MaxRetransmits() != nil || dc.MaxPacketLifeTime() != nil {
		go c.Close()
		return
	}
	c.bind(dc, onOpen)
}
func (c *Connection) bind(dc *webrtc.DataChannel, onOpen func(*Connection)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dc != nil {
		dc.Close()
		return
	}
	c.dc = dc
	dc.OnClose(func() { go c.Close() })
	dc.OnOpen(func() { close(c.ready); onOpen(c) })
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		if !m.IsString || c.deliver(m.Data) != nil {
			go c.Close()
		}
	})
}

type framing struct {
	Type *string `json:"t,omitempty"`
	ID   *int64  `json:"id,omitempty"`
	Re   *int64  `json:"re,omitempty"`
}

func (c *Connection) deliver(data []byte) error {
	f, err := readFraming(data)
	if err != nil {
		return err
	}
	if f.Re != nil {
		return c.response(*f.Re, f, data)
	}
	if f.Type == nil {
		return errors.New("missing ctl type")
	}
	select {
	case c.incoming <- data:
	case <-c.Done():
	}
	return nil
}
func readFraming(data []byte) (f framing, err error) {
	if len(data) > ctlLimit || !utf8.Valid(data) {
		return f, errors.New("invalid ctl text")
	}
	err = proto.Decode(data, &f)
	return f, err
}
func (c *Connection) response(id int64, f framing, data []byte) error {
	if f.ID != nil {
		return errors.New("both id and re on ctl")
	}
	if err := validResult(f.Type, data); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch := c.waiting[id]; ch != nil {
		delete(c.waiting, id)
		ch <- data
	}
	return nil
}
func validResult(kind *string, data []byte) error {
	var fields struct {
		Err *proto.WireError `json:"err,omitempty"`
		OK  *bool            `json:"ok,omitempty"`
	}
	if err := proto.Decode(data, &fields); err != nil {
		return err
	}
	if fields.Err != nil {
		return checkErrorResult(fields.OK, kind)
	}
	if fields.OK == nil || !*fields.OK {
		return errors.New("ctl response without a result")
	}
	return nil
}

func checkErrorResult(ok *bool, kind *string) error {
	if ok != nil || kind != nil {
		return errors.New("invalid ctl error response")
	}
	return nil
}

// Read returns the next request or push, in channel order. Replies go to
// the corresponding Request call, not to this stream.
func (c *Connection) Read(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.Done():
		return nil, io.EOF
	case msg := <-c.incoming:
		return msg, nil
	}
}

// Send writes a response or push after ctl opens.
func (c *Connection) Send(frame any) error {
	data := encode(frame)
	if len(data) > ctlLimit {
		return errors.New("ctl frame exceeds 60 KiB")
	}
	return c.dc.SendText(string(data))
}
func (c *Connection) reserve() (int64, chan json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	ch := make(chan json.RawMessage, 1)
	c.waiting[c.next] = ch
	return c.next, ch
}
func (c *Connection) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	id, ch := c.reserve()
	defer func() { c.mu.Lock(); delete(c.waiting, id); c.mu.Unlock() }()
	if err := c.Send(build(id)); err != nil {
		return nil, err
	}
	select {
	case msg := <-ch:
		return msg, nil
	case <-c.Done():
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
