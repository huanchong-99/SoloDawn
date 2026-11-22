package node

import (
	"context"
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const (
	legacyVersion = 1
	wireVersion   = 2
)

func sesDecodeHello(data json.RawMessage) (proto.Hello, *host.Error) {
	var ver struct {
		proto.Request
		V int `json:"v"`
	}
	if proto.Decode(data, &ver) != nil {
		return proto.Hello{}, &host.Error{Code: "bad_request", Message: "invalid hello frame"}
	}
	switch ver.V {
	case wireVersion, legacyVersion:
	default:
		return proto.Hello{}, &host.Error{Code: "version", Message: "unsupported protocol version"}
	}
	var h proto.Hello
	if proto.Decode(data, &h) != nil || (h.Kind != "browser" && h.Kind != "host") {
		return proto.Hello{}, &host.Error{Code: "bad_request", Message: "invalid hello fields"}
	}
	return h, nil
}

func (s *Sessions) cliHello(ctx context.Context, raw clientCtl, version int) (json.RawMessage, error) {
	return raw.Request(ctx, func(id int64) any {
		return proto.Hello{
			Request: proto.Request{Type: "hello", ID: id},
			V:       version,
			Kind:    "host",
			Name:    s.cfg.Name(),
			Addrs:   s.cfg.Addrs(),
		}
	})
}

func cliVersionRefusal(ans json.RawMessage) bool {
	var refusal proto.ErrorResponse
	return proto.Decode(ans, &refusal) == nil && refusal.Err.Code == "version"
}
