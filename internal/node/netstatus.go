package node

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

var netStatusSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

func init() {
	host.Register(host.Tools, "net_status", host.Tool{
		Description: "Report peer connection paths, local NAT capabilities, and rendezvous reachability.",
		Schema:      netStatusSchema,
		Call:        callNetStatus,
	})
}

type (
	netStatusResult struct {
		Paths      []netPath       `json:"paths"`
		NAT        netNAT          `json:"nat"`
		Rendezvous []netRendezvous `json:"rendezvous"`
	}
	netPath struct {
		Peer string `json:"peer"`
		Path string `json:"path"`
	}
	netNAT struct {
		UDP         bool `json:"udp"`
		MappedPorts bool `json:"mapped_ports"`
		IPv6        bool `json:"ipv6"`
	}
	netRendezvous struct {
		URL string `json:"url"`
		OK  bool   `json:"ok"`
	}
	sesPeer struct {
		device string
		raw    Ctl
	}
	netStatusPart struct {
		status func() netStatusResult
	}
)

func callNetStatus(_ context.Context, h *host.Host, _ host.Caller, raw json.RawMessage) (any, error) {
	if proto.Decode(raw, &struct{}{}) != nil {
		return nil, &host.Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	if fn := host.Part[netStatusPart](h).status; fn != nil {
		return fn(), nil
	}
	return netStatusResult{Paths: []netPath{}, Rendezvous: []netRendezvous{}}, nil
}

func (n *nodeRuntime) netStatus() netStatusResult {
	return netStatusResult{
		Paths:      n.sessions.netPaths(),
		NAT:        netNAT{UDP: true, IPv6: hasIPv6(localIPs())},
		Rendezvous: buildNetRendezvous(ReadRendezvous(n.db), n.connectedRVSet()),
	}
}

func (n *nodeRuntime) connectedRVSet() map[string]bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make(map[string]bool, len(n.rvPaths))
	for u := range n.rvPaths {
		out[u] = true
	}
	return out
}

func (s *Sessions) welcomedPeers() []sesPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sesPeer
	for c := range s.conns {
		if c.welcomed {
			out = append(out, sesPeer{device: c.device, raw: c.raw})
		}
	}
	return out
}

func (s *Sessions) netPaths() []netPath {
	out := []netPath{}
	for _, c := range s.welcomedPeers() {
		if p, ok := buildNetPath(c.device, c.raw); ok {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, compareNetPath)
	return out
}

func buildNetPath(device string, raw Ctl) (netPath, bool) {
	path := raw.Path()
	if path == "" {
		return netPath{}, false
	}
	return netPath{Peer: device, Path: path}, true
}

func compareNetPath(a, b netPath) int {
	return cmp.Or(cmp.Compare(a.Peer, b.Peer), cmp.Compare(a.Path, b.Path))
}

func buildNetRendezvous(urls []string, connected map[string]bool) []netRendezvous {
	out := make([]netRendezvous, len(urls))
	for i, u := range urls {
		out[i] = netRendezvous{URL: u, OK: connected[u]}
	}
	return out
}

func hasIPv6(ips []string) bool {
	return slices.ContainsFunc(ips, isIPv6Text)
}

func isIPv6Text(ip string) bool {
	return strings.Contains(ip, ":")
}
