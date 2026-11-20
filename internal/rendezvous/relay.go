package rendezvous

import (
	"crypto/rand"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/turn/v5"
)

const ttl = time.Hour

type relay struct {
	stun   string
	urls   []string
	secret string
	ledger
}

func (r *relay) ice(kind, id string) proto.ICEConfig {
	frame := proto.ICEConfig{Type: kind, ICE: []proto.ICEServer{}, TTL: int(ttl / time.Second)}
	if r.stun == "" {
		return frame
	}
	frame.ICE = append(frame.ICE, proto.ICEServer{URLs: []string{r.stun}})
	if len(r.urls) != 0 {
		user, pass, err := turn.GenerateLongTermTURNRESTCredentials(r.secret, id, ttl)
		if err != nil {
			panic(err) // hash.Hash.Write never fails.
		}
		frame.ICE = append(frame.ICE, proto.ICEServer{URLs: r.urls, Username: user, Credential: pass})
	}
	return frame
}

func (o Options) start(udp net.PacketConn, tlsListener net.Listener, tlsPort int) (*relay, *turn.Server, error) {
	local := udp.LocalAddr().(*net.UDPAddr)
	addr := net.JoinHostPort(o.Domain, strconv.Itoa(local.Port))
	r := &relay{stun: "stun:" + addr}
	cfg := turn.ServerConfig{Realm: o.Domain, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp}}}
	if tlsListener != nil {
		ips, err := net.LookupIP(o.Domain)
		if err != nil {
			return nil, nil, err
		}
		r.limit, r.secret = o.QuotaMB<<20, rand.Text()
		r.urls = []string{"turn:" + addr + "?transport=udp", "turns:" + net.JoinHostPort(tlsName(o.TurnDomain), strconv.Itoa(tlsPort)) + "?transport=tcp"}
		g := &generator{public: ips, bind: local.IP, ledger: &r.ledger}
		// Use the RFC IPv4 default when available. On IPv6-only hosts let
		// Pion use the transport family for clients that omit the attribute.
		cfg.StrictAddressFamily = g.publicIP("udp4") != nil && (local.IP.To4() != nil || local.IP.IsUnspecified())
		cfg.PacketConnConfigs[0].RelayAddressGenerator = g
		cfg.PacketConnConfigs[0].PermissionHandler = g.permitPeer
		cfg.ListenerConfigs = []turn.ListenerConfig{{Listener: tlsListener, RelayAddressGenerator: g, PermissionHandler: g.permitPeer}}
		cfg.AuthHandler = restAuth(r.secret)
		cfg.EventHandler = g.events()
	}
	s, err := turn.NewServer(cfg)
	return r, s, err
}

func restAuth(secret string) turn.AuthHandler {
	verify := turn.LongTermTURNRESTAuthHandler(secret, nil)
	return func(a *turn.RequestAttributes) (string, []byte, bool) {
		expiry, _, _ := strings.Cut(a.Username, ":")
		// Pion refuses earlier timestamps but accepts the expiry second itself.
		if expiry == strconv.FormatInt(time.Now().Unix(), 10) {
			return "", nil, false
		}
		return verify(a)
	}
}

type generator struct {
	public []net.IP
	bind   net.IP
	*ledger
	permissions sync.Map // (relay endpoint, peer IP) installed by Pion
}

type permission struct {
	relay net.Addr
	peer  string
}

func (g *generator) events() turn.EventHandler {
	return turn.EventHandler{
		OnPermissionCreated: func(_, _ net.Addr, _, _, _ string, relay net.Addr, peer net.IP) {
			g.permissions.Store(permission{relay, peer.String()}, true)
		},
		OnPermissionDeleted: func(_, _ net.Addr, _, _, _ string, relay net.Addr, peer net.IP) {
			g.permissions.Delete(permission{relay, peer.String()})
		},
	}
}

func (g *generator) AllocatePacketConn(c turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	ip := g.publicIP(c.Network)
	if ip == nil {
		return nil, nil, errors.New("no unambiguous relay address for requested address family")
	}
	conn, err := net.ListenPacket(c.Network, net.JoinHostPort(g.bind.String(), strconv.Itoa(c.RequestedPort)))
	if err != nil {
		return nil, nil, err
	}
	addr := &net.UDPAddr{IP: ip, Port: conn.LocalAddr().(*net.UDPAddr).Port}
	return &allocation{PacketConn: conn, g: g, addr: addr, device: c.UserID}, addr, nil
}

func (*generator) Validate() error { return nil } // Addresses were resolved at startup.

func (g *generator) publicIP(network string) net.IP {
	var matches []net.IP
	for _, ip := range g.public {
		if (network == "udp4") != (ip.To4() != nil) {
			continue
		}
		if ip.Equal(g.bind) {
			return ip
		}
		matches = append(matches, ip)
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return nil
}

// A public TURN credential must not grant access to the relay's private
// network. A loopback-only service can be exercised with a loopback peer.
func (g *generator) permitPeer(_ net.Addr, peer net.IP) bool {
	if g.bind.IsLoopback() {
		return peer.IsLoopback()
	}
	ip, _ := netip.AddrFromSlice(peer)
	return peer.IsGlobalUnicast() && !peer.IsPrivate() && !sharedIPv4.Contains(ip.Unmap())
}

var sharedIPv4 = netip.MustParsePrefix("100.64.0.0/10") // RFC 6598 carrier-grade NAT

// The client transport can be TLS/TCP; ICE requests a UDP relay socket.
// Do not offer an unmetered RFC 6062 TCP relay.
var errTransport = errors.New("only UDP relay allocations are supported")

func (*generator) AllocateListener(turn.AllocateListenerConfig) (net.Listener, net.Addr, error) {
	return nil, nil, errTransport
}
func (*generator) AllocateConn(turn.AllocateConnConfig) (net.Conn, error) { return nil, errTransport }

type allocation struct {
	net.PacketConn
	g      *generator
	addr   net.Addr
	device string
	packet [65535]byte // receive whole UDP datagrams, never an undetectably truncated prefix
}

func (a *allocation) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, src, err := a.PacketConn.ReadFrom(a.packet[:])
		if err != nil {
			return n, src, err
		}
		_, allowed := a.g.permissions.Load(permission{a.addr, src.(*net.UDPAddr).IP.String()})
		if n <= len(b) && allowed && a.g.admit(a.device, n) {
			copy(b, a.packet[:n])
			return n, src, nil
		}
	}
}

func (a *allocation) WriteTo(b []byte, dst net.Addr) (int, error) {
	return a.g.write(a.device, a.PacketConn, b, dst)
}
