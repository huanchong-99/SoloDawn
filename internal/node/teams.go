package node

import (
	"context"
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func (s *Sessions) connectHome(dial dialFunc) func(ctx context.Context, device string) {
	return func(ctx context.Context, device string) {
		if device == s.cfg.ID {
			return
		}
		run := func(raw clientCtl) { s.serveClient(ctx, device, raw) }
		go dial(ctx, device, run)
	}
}

func (s *Sessions) cmdHomeAdd(_ context.Context, _ Ctl, _ string, payload json.RawMessage) (any, error) {
	var cmd proto.HomeAdd
	if proto.Decode(payload, &cmd) != nil {
		return nil, &host.Error{Code: "bad_request", Message: "invalid home.add fields"}
	}
	entry := proto.Home{Group: cmd.Group, Home: cmd.Home, Name: cmd.Name}
	return nil, s.cfg.Host.RecordHome(entry, cmd.Addrs)
}
