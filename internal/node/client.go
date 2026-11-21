package node

import (
	"context"
	"encoding/json"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var _ clientCtl = (*peer.Connection)(nil)

const cliHelloTimeout = 10 * time.Second

type clientCtl interface {
	Ctl
	host.Home
}

type dialFunc func(ctx context.Context, device string, session func(clientCtl))

func (s *Sessions) connectHome(dial dialFunc) func(ctx context.Context, device string) {
	return func(ctx context.Context, device string) {
		if device == s.cfg.ID {
			return
		}
		run := func(raw clientCtl) { s.serveClient(ctx, device, raw) }
		go dial(ctx, device, run)
	}
}

func (s *Sessions) serveClient(ctx context.Context, device string, raw clientCtl) {
	runCtx, cancel := context.WithCancel(ctx)
	c := &sesConn{device: device, raw: raw}
	s.mu.Lock()
	s.conns[c] = true
	s.mu.Unlock()
	ready := make(chan struct{})
	readDone := make(chan struct{}) // single reader: serveClient
	go func() {
		defer close(readDone)
		s.cliReadLoop(runCtx, c, raw, ready)
	}()
	if s.cliHandshake(runCtx, c, raw) {
		s.cfg.Host.Attach(device, raw)
		close(ready)
		<-readDone
	}
	cancel()
	s.cliLeave(c)
	<-readDone
}

func (s *Sessions) cliLeave(c *sesConn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	c.raw.Close()
}

func (s *Sessions) cliHandshake(ctx context.Context, c *sesConn, raw clientCtl) bool {
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
	if err != nil || !cliWelcomeOK(ans) {
		return false
	}
	return s.cliSendLogs(c)
}

func cliWelcomeOK(ans json.RawMessage) bool {
	var refused proto.ErrorResponse
	return proto.Decode(ans, &refused) != nil
}

func (s *Sessions) cliSendLogs(c *sesConn) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	s.mu.Lock()
	c.welcomed = true
	s.mu.Unlock()
	for _, dl := range s.cfg.Logs.frames(s.cfg.Logs.Owner()) {
		if c.raw.Send(dl) != nil {
			return false
		}
	}
	return true
}

func (s *Sessions) cliReadLoop(ctx context.Context, c *sesConn, raw clientCtl, ready <-chan struct{}) {
	for {
		data, err := raw.Read(ctx)
		if err != nil {
			return
		}
		s.cliDispatch(ctx, c, data, ready)
	}
}

func (s *Sessions) cliDispatch(ctx context.Context, c *sesConn, data json.RawMessage, ready <-chan struct{}) {
	typ, _, ok := sesOpenFrame(c, data)
	if !ok {
		return
	}
	if typ == "devlog" {
		s.sesDevlog(c, data)
		return
	}
	select {
	case <-ready:
		s.cfg.Host.Push(c.device, data)
	case <-ctx.Done():
		return
	}
}
