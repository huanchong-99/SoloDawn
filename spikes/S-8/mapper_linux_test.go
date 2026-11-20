//go:build netsim

package s8

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway2"
	natpmp "github.com/jackpal/go-nat-pmp"
	nat "github.com/libp2p/go-nat"
	"github.com/libp2p/go-netroute"
)

// mapper answers requests in a client namespace with three port-mapping
// clients: go-nat (X-1's library) and, beneath it, go-nat-pmp and goupnp
// used directly. Each request gets exactly one reply.
type mapper struct {
	lib map[string]nat.NAT // go-nat, by Type()
	pmp *natpmp.Client
	igd *internetgateway2.WANIPConnection2
}

func runMapper(in io.Reader, out *emitter) error {
	m := &mapper{lib: map[string]nat.NAT{}}
	lines := bufio.NewScanner(in)
	for lines.Scan() {
		var req msg
		if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
			return err
		}
		start := time.Now()
		reply := m.handle(req)
		reply.T, reply.Ms = req.T, since(start)
		out.send(reply)
	}
	return lines.Err()
}

func (m *mapper) handle(req msg) msg {
	timeout := time.Duration(req.Ms) * time.Millisecond
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	switch req.T {
	case "discover":
		return m.discover(ctx)
	case "gateway":
		g, err := nat.DiscoverGateway(ctx)
		if err != nil {
			return msg{Err: err.Error()}
		}
		return msg{Kind: g.Type()}
	case "external", "map", "delete":
		return m.library(ctx, req)
	case "pmp":
		return m.natpmp(req)
	case "igd":
		return m.upnp(ctx, req)
	}
	return msg{Err: "unknown request " + req.T}
}

// discover lists every gateway go-nat finds and when each arrived.
func (m *mapper) discover(ctx context.Context) msg {
	start := time.Now()
	var r msg
	for g := range nat.DiscoverNATs(ctx) {
		m.lib[g.Type()] = g
		r.Types = append(r.Types, g.Type())
		r.After = append(r.After, since(start))
	}
	return r
}

func (m *mapper) library(ctx context.Context, req msg) msg {
	g := m.lib[req.NAT]
	if g == nil {
		return msg{Err: "not discovered: " + req.NAT}
	}
	switch req.T {
	case "external":
		ip, err := g.GetExternalAddress()
		return msg{IP: ip.String(), Err: errText(err)}
	case "map":
		port, err := g.AddPortMapping(ctx, req.Proto, req.Internal, "S-8", time.Duration(req.Lifetime)*time.Second)
		return msg{Port: port, Err: errText(err)}
	}
	return msg{Err: errText(g.DeletePortMapping(ctx, req.Proto, req.Internal))}
}

// gateway is the next hop of the default route, as go-nat finds it.
func gateway() (net.IP, error) {
	r, err := netroute.New()
	if err != nil {
		return nil, err
	}
	_, gw, _, err := r.Route(net.IPv4zero)
	return gw, err
}

// natpmp maps with go-nat-pmp directly, with a bounded retry time; a zero
// lifetime deletes. External 0 asks for the internal port (RFC 6886).
func (m *mapper) natpmp(req msg) msg {
	if m.pmp == nil {
		gw, err := gateway()
		if err != nil {
			return msg{Err: err.Error()}
		}
		m.pmp = natpmp.NewClientWithTimeout(gw, 2*time.Second)
	}
	if req.Kind == "external" {
		r, err := m.pmp.GetExternalAddress()
		if err != nil {
			return msg{Err: err.Error()}
		}
		a := r.ExternalIPAddress
		return msg{IP: net.IPv4(a[0], a[1], a[2], a[3]).String()}
	}
	r, err := m.pmp.AddPortMapping(req.Proto, req.Internal, req.External, req.Lifetime)
	if err != nil {
		return msg{Err: err.Error()}
	}
	return msg{Port: int(r.MappedExternalPort), Lifetime: int(r.PortMappingLifetimeInSeconds)}
}

// upnp uses the IGDv2 WANIPConnection:2 service directly.
func (m *mapper) upnp(ctx context.Context, req msg) msg {
	if req.Kind == "discover" || m.igd == nil {
		clients, errs, err := internetgateway2.NewWANIPConnection2ClientsCtx(ctx)
		if err == nil && len(clients) == 0 {
			err = errors.Join(append(errs, errors.New("no WANIPConnection:2 service"))...)
		}
		if err != nil {
			return msg{Err: err.Error()}
		}
		m.igd = clients[0]
		if req.Kind == "discover" {
			return msg{IP: m.igd.Location.Host}
		}
	}
	proto, client := strings.ToUpper(req.Proto), net.IPv4(10, 0, 0, 2).String()
	ext, in, lease := uint16(req.External), uint16(req.Internal), uint32(req.Lifetime)
	switch req.Kind {
	case "external":
		ip, err := m.igd.GetExternalIPAddressCtx(ctx)
		return msg{IP: ip, Err: errText(err)}
	case "add":
		return msg{Port: req.External, Err: errText(m.igd.AddPortMappingCtx(ctx, "", ext, proto, in, client, true, "S-8", lease))}
	case "any":
		port, err := m.igd.AddAnyPortMappingCtx(ctx, "", ext, proto, in, client, true, "S-8", lease)
		return msg{Port: int(port), Err: errText(err)}
	case "get":
		port, host, _, _, left, err := m.igd.GetSpecificPortMappingEntryCtx(ctx, "", ext, proto)
		return msg{Port: int(port), IP: host, Lifetime: int(left), Err: errText(err)}
	case "delete":
		return msg{Err: errText(m.igd.DeletePortMappingCtx(ctx, "", ext, proto))}
	}
	return msg{Err: "unknown igd request " + req.Kind}
}
