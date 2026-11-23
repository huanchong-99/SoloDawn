package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

const ctlLimit = 61_440 // 60 KiB

type Connection struct {
	Device      string
	pc          *webrtc.PeerConnection
	ctx         context.Context
	close       func() error
	ready       chan struct{}
	pathChanged chan struct{}
	incoming    chan json.RawMessage // single reader: Read
	mu          sync.Mutex
	dc          *webrtc.DataChannel
	next        int64
	waiting     map[int64]chan json.RawMessage // each channel's single reader: its Request
	answer      *proto.Envelope
	pending     map[string]bool // see waitForAnswer
	deadline    *time.Timer
	candidates  *batch
}

func newConnection(pc *webrtc.PeerConnection, device string) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connection{
		Device: device, pc: pc, ctx: ctx, ready: make(chan struct{}), pathChanged: make(chan struct{}, 1),
		incoming: make(chan json.RawMessage, 16), waiting: make(map[int64]chan json.RawMessage),
		close: sync.OnceValue(func() error { cancel(); return pc.Close() }),
	}
}
func (c *Connection) Ready() <-chan struct{} { return c.ready }
func (c *Connection) Done() <-chan struct{}  { return c.ctx.Done() }
func (c *Connection) Close() error           { return c.close() }
func (c *Connection) isClosed() bool         { return c.ctx.Err() != nil }

func (c *Connection) NegotiationState() string {
	pair, err := c.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	return fmt.Sprintf("ICE=%s gathering=%s signaling=%s selected=%v selectedError=%v",
		c.pc.ICEConnectionState(), c.pc.ICEGatheringState(), c.pc.SignalingState(), pair, err)
}

// acceptCtl takes the channel labelled ctl; dispatch has checked the label.
func (c *Connection) acceptCtl(dc *webrtc.DataChannel, onOpen func(*Connection)) {
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
		if m.IsString { // a binary message on ctl is dropped
			c.deliver(m.Data)
		}
	})
}

type framing struct {
	Type *string `json:"t,omitempty"`
	ID   *int64  `json:"id,omitempty"`
	Re   *int64  `json:"re,omitempty"`
}

// deliver queues a valid request, push or response for Read, and refuses
// anything else while keeping the connection open
// (contracts/datachannel.md section 2).
func (c *Connection) deliver(data []byte) {
	if !validFrame(data) {
		c.refuse(data)
		return
	}
	c.queue(data)
}

func validFrame(data []byte) bool {
	f, err := readFraming(data)
	if err != nil {
		return false
	}
	if f.Re != nil {
		return f.ID == nil && validResult(f.Type, data) == nil
	}
	return f.Type != nil
}

// queue holds a valid ctl frame until Read takes it.
func (c *Connection) queue(data []byte) {
	select {
	case c.incoming <- data:
	case <-c.Done():
	}
}

// refuse answers bad_request to a message that carries a request id and
// drops any other.
func (c *Connection) refuse(data []byte) {
	var r struct {
		ID *int64 `json:"id,omitempty"`
	}
	if proto.Decode(data, &r) == nil && r.ID != nil {
		c.Send(proto.ErrorResponse{Re: *r.ID, Err: proto.WireError{Code: "bad_request", Message: "the message is not a ctl frame of the contract"}})
	}
}
func readFraming(data []byte) (f framing, err error) {
	if len(data) > ctlLimit || !utf8.Valid(data) {
		return f, errors.New("invalid ctl text")
	}
	err = proto.Decode(data, &f)
	return f, err
}

// response hands a queued response to the request waiting for it.
func (c *Connection) response(re int64, data json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch := c.waiting[re]; ch != nil {
		delete(c.waiting, re)
		ch <- data
	}
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

// Read returns the next request or push in channel order, handing any queued
// response to its waiting Request before reading on.
func (c *Connection) Read(ctx context.Context) (json.RawMessage, error) {
	for {
		msg, err := c.dequeue(ctx)
		if err != nil {
			return nil, err
		}
		if re := replyID(msg); re != nil {
			c.response(*re, msg)
			continue
		}
		return msg, nil
	}
}

func (c *Connection) dequeue(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.Done():
		return nil, io.EOF
	case msg := <-c.incoming:
		return msg, nil
	}
}

func replyID(msg json.RawMessage) *int64 {
	var f framing
	json.Unmarshal(msg, &f)
	return f.Re
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
