package node

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var (
	_ host.Home   = (*LocalLink)(nil)
	_ home.Client = localClient{}
)

// LocalLink connects the host and the home of the same node in memory through
// host.Home and home.Client (planning/architecture.md section 8.3).
//
// Lock order: mu guards next and waiting only; it is never held across calls
// into host or home. Each channel in waiting has a single reader, the Request
// call that registered its id.
type LocalLink struct {
	push   func(string, json.RawMessage)
	loop   *home.Loop
	conn   *home.Conn
	device string

	mu      sync.Mutex
	next    int64
	waiting map[int64]chan json.RawMessage
}

// AttachLocal connects h to loop over a new LocalLink for this node's device,
// registers the link on h, and marks it as the inbox taker on loop.
func AttachLocal(h *host.Host, loop *home.Loop) *LocalLink {
	l := &LocalLink{push: h.Push, loop: loop, device: h.Key.ID(), waiting: make(map[int64]chan json.RawMessage)}
	l.conn = loop.Attach(l.device, localClient{l})
	h.Attach(l.device, l)
	loop.Inbox(l.conn)
	return l
}

// Path returns "local", the path reported for an in-process home.
func (l *LocalLink) Path() string { return "local" }

// PathChanged returns nil because an in-process link never changes path.
func (l *LocalLink) PathChanged() <-chan struct{} { return nil }

// Done is closed when the link is closed or detached from the home loop.
func (l *LocalLink) Done() <-chan struct{} { return l.conn.Gone() }

// Close detaches the link from the home loop.
func (l *LocalLink) Close() error {
	l.loop.Detach(l.conn)
	return nil
}

// Watch subscribes the link to row pushes of group after the returned sequence
// number, state rows only when stateOnly is true.
func (l *LocalLink) Watch(group string, stateOnly bool) (int64, bool) {
	return l.loop.Watch(l.conn, group, stateOnly)
}

func (l *LocalLink) isClosed() bool {
	select {
	case <-l.Done():
		return true
	default:
		return false
	}
}

// Send encodes frame as a ctl message and hands it to the local home.
func (l *LocalLink) Send(frame any) error {
	if l.isClosed() {
		return io.EOF
	}
	data, err := encodeCtl(frame)
	if err != nil {
		return err
	}
	l.toHome(data)
	return nil
}

// Request assigns a request id, sends the frame build returns, and waits for
// the matching response, link closure or ctx cancellation.
func (l *LocalLink) Request(ctx context.Context, build func(id int64) any) (json.RawMessage, error) {
	id, reply := l.openWait()
	defer l.dropWait(id)
	if err := l.Send(build(id)); err != nil {
		return nil, err
	}
	select {
	case answer := <-reply:
		return answer, nil
	case <-l.Done():
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *LocalLink) openWait() (int64, chan json.RawMessage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	reply := make(chan json.RawMessage, 1) // single reader: the Request call owning l.next
	l.waiting[l.next] = reply
	return l.next, reply
}

func (l *LocalLink) dropWait(id int64) {
	l.mu.Lock()
	delete(l.waiting, id)
	l.mu.Unlock()
}

func decodeCmd(data []byte) (proto.Cmd, bool) {
	var cmd proto.Cmd
	return cmd, proto.Decode(data, &cmd) == nil && cmd.Type == "cmd"
}

func (l *LocalLink) toHome(data []byte) {
	if cmd, ok := decodeCmd(data); ok {
		l.loop.Cmd(l.conn, cmd.ID, cmd.Env)
		return
	}
	var req struct {
		ID *int64 `json:"id,omitempty"`
	}
	if proto.Decode(data, &req) == nil && req.ID != nil {
		l.badRequest(*req.ID)
	}
}

func (l *LocalLink) badRequest(re int64) {
	resp := proto.ErrorResponse{Re: re, Err: proto.WireError{Code: "bad_request", Message: "the message is not a supported home request"}}
	data, _ := encodeCtl(resp)
	l.deliver(re, data)
}

type localClient struct{ link *LocalLink }

func (c localClient) Done() <-chan struct{} { return nil }

func (c localClient) Send(frame any) error {
	data, err := encodeCtl(frame)
	if err != nil {
		return err
	}
	c.link.fromHome(data)
	return nil
}

func (l *LocalLink) fromHome(data []byte) {
	var hdr struct {
		Re *int64 `json:"re,omitempty"`
	}
	if proto.Decode(data, &hdr) != nil {
		return
	}
	if hdr.Re != nil {
		l.deliver(*hdr.Re, data)
		return
	}
	l.push(l.device, data)
}

func (l *LocalLink) deliver(re int64, data json.RawMessage) {
	l.mu.Lock()
	reply := l.waiting[re]
	delete(l.waiting, re)
	l.mu.Unlock()
	if reply != nil {
		reply <- data
	}
}

func encodeCtl(frame any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(frame); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
