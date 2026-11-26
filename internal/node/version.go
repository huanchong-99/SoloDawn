package node

import (
	"context"
	"encoding/json"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// wireVersion is the one protocol version there is: 1 and 2 are what
// v3.0.0-rc.1 spoke, whose rows this version would misread
// (contracts/datachannel.md section 3).
const wireVersion = 3

func sesDecodeHello(data json.RawMessage) (proto.Hello, *host.Error) {
	var ver struct {
		proto.Request
		V int `json:"v"`
	}
	if proto.Decode(data, &ver) != nil {
		return proto.Hello{}, &host.Error{Code: "bad_request", Message: "invalid hello frame"}
	}
	if ver.V != wireVersion {
		return proto.Hello{}, &host.Error{Code: "version", Message: "unsupported protocol version"}
	}
	var h proto.Hello
	if proto.Decode(data, &h) != nil || (h.Kind != "browser" && h.Kind != "host") {
		return proto.Hello{}, &host.Error{Code: "bad_request", Message: "invalid hello fields"}
	}
	return h, nil
}

func (s *Sessions) cliHello(ctx context.Context, raw clientCtl) (json.RawMessage, error) {
	return raw.Request(ctx, func(id int64) any {
		return proto.Hello{
			Request: proto.Request{Type: "hello", ID: id},
			V:       wireVersion,
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
