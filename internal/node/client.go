package node

import (
	"context"
	"encoding/json"
	"fmt"
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
	ans, err := s.cliHello(helloCtx, raw)
	if err != nil || !s.cliWelcomeOK(c, ans) {
		return false
	}
	return s.cliSendLogs(c)
}

// cliWelcomeOK takes a welcome of wireVersion and nothing else (datachannel.md section 3).
func (s *Sessions) cliWelcomeOK(c *sesConn, ans json.RawMessage) bool {
	var w proto.Welcome
	err := proto.Decode(ans, &w)
	if err == nil && w.V == wireVersion {
		return true
	}
	if err == nil || cliVersionRefusal(ans) {
		s.cfg.Report(fmt.Errorf("the home %s speaks another protocol version: update this node and that computer together", c.device))
	}
	return false
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
