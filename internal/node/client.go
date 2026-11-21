package node

import (
	"context"
	"encoding/json"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var _ ClientCtl = (*peer.Connection)(nil)

const cliHelloTimeout = 10 * time.Second

// ClientCtl is the client side of an authenticated ctl data channel;
// *peer.Connection satisfies it.
type ClientCtl interface {
	Ctl
	host.Home
}

// DialFunc keeps an outgoing ctl connection to device open until ctx ends,
// running session for each opened connection.
type DialFunc func(ctx context.Context, device string, session func(ClientCtl))

// ConnectHome returns a Host.Connect function that ignores this node's own
// device and starts dial in the background to run ServeClient on each opened
// connection until ctx ends.
func (s *Sessions) ConnectHome(dial DialFunc) func(ctx context.Context, device string) {
	return func(ctx context.Context, device string) {
		if device == s.cfg.ID {
			return
		}
		run := func(raw ClientCtl) { s.ServeClient(ctx, device, raw) }
		go dial(ctx, device, run)
	}
}

// ServeClient runs the client side of a host ctl session to device on raw: it
// sends hello with kind "host", checks welcome, sends this node's device log,
// attaches raw to Host, and dispatches incoming pushes in read order until ctx
// ends or raw closes.
func (s *Sessions) ServeClient(ctx context.Context, device string, raw ClientCtl) error {
	c := &sesConn{device: device, raw: raw}
	s.mu.Lock()
	s.conns[c] = true
	s.mu.Unlock()
	defer s.cliLeave(c)
	if err := s.cliHandshake(ctx, c, raw); err != nil {
		return err
	}
	s.cfg.Host.Attach(device, raw)
	return s.cliReadLoop(ctx, c, raw)
}

func (s *Sessions) cliLeave(c *sesConn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	c.raw.Close()
}

func (s *Sessions) cliHandshake(ctx context.Context, c *sesConn, raw ClientCtl) error {
	helloCtx, cancel := context.WithTimeout(ctx, cliHelloTimeout)
	defer cancel()
	build := func(id int64) any {
		return proto.Hello{
			Request: proto.Request{Type: "hello", ID: id},
			V:       1,
			Kind:    "host",
			Name:    s.cfg.Name(),
			Addrs:   s.cfg.Addrs(),
		}
	}
	ans, err := raw.Request(helloCtx, build)
	if err != nil {
		return err
	}
	if err := cliCheckWelcome(ans); err != nil {
		return err
	}
	return s.cliSendLogs(c)
}

func cliCheckWelcome(ans json.RawMessage) error {
	var refused proto.ErrorResponse
	if proto.Decode(ans, &refused) == nil {
		return &host.Error{Code: refused.Err.Code, Message: refused.Err.Message}
	}
	var w proto.Welcome
	if proto.Decode(ans, &w) != nil || w.Type != "welcome" || w.V != 1 {
		return &host.Error{Code: "bad_request", Message: "invalid welcome frame"}
	}
	return nil
}

func (s *Sessions) cliSendLogs(c *sesConn) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	s.mu.Lock()
	c.welcomed = true
	s.mu.Unlock()
	for _, dl := range s.cfg.Logs.frames(s.cfg.Logs.Owner()) {
		if err := c.raw.Send(dl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sessions) cliReadLoop(ctx context.Context, c *sesConn, raw ClientCtl) error {
	for {
		data, err := raw.Read(ctx)
		if err != nil {
			return err
		}
		s.cliDispatch(c, data)
	}
}

func (s *Sessions) cliDispatch(c *sesConn, data json.RawMessage) {
	typ, id, ok := sesOpenFrame(c, data)
	if !ok {
		return
	}
	if id != nil {
		c.sendErr(*id, "bad_request", "unknown ctl request")
		return
	}
	if typ == "devlog" {
		s.sesDevlog(c, data)
		return
	}
	s.cfg.Host.Push(c.device, data)
}
