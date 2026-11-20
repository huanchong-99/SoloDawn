package rendezvous

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/logging"
	"github.com/pion/stun/v4"
	"github.com/pion/turn/v5"
)

const ttl = time.Hour

// quiet is Pion's logger: a line for every refused request would let anyone
// flood the log. Refused Allocates are counted in relay.refused instead.
var quiet = &logging.DefaultLoggerFactory{Writer: io.Discard, DefaultLogLevel: logging.LogLevelDisabled}

type relay struct {
	stun    string
	urls    []string
	secret  string
	refused atomic.Int64 // Allocates refused by their credential or by a limit
	ledger
}

func (r *relay) ice(kind, id string) proto.ICEConfig {
	frame := proto.ICEConfig{Type: kind, ICE: []proto.ICEServer{}, TTL: int(ttl / time.Second)}
	if r.stun == "" {
		return frame
	}
	frame.ICE = append(frame.ICE, proto.ICEServer{URLs: []string{r.stun}})
	if len(r.urls) != 0 {
		// The only error is that of hash.Hash.Write, which never fails.
		user, pass, _ := turn.GenerateLongTermTURNRESTCredentials(r.secret, r.alias(id), ttl)
		frame.ICE = append(frame.ICE, proto.ICEServer{URLs: r.urls, Username: user, Credential: pass})
	}
	return frame
}

// alias stands for a device in TURN usernames and in the quota: the first 16
// bytes of an HMAC-SHA256 of its id under the process's secret, base64url, so
// that a username does not carry the device id.
func (r *relay) alias(id string) string {
	mac := hmac.New(sha256.New, []byte(r.secret))
	mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

func (o Options) start(udp net.PacketConn, tlsListener net.Listener, tlsPort int) (*relay, *turn.Server, error) {
	local := udp.LocalAddr().(*net.UDPAddr)
	addr := net.JoinHostPort(o.Domain, strconv.Itoa(local.Port))
	r := &relay{stun: "stun:" + addr}
	cfg := turn.ServerConfig{Realm: o.Domain, LoggerFactory: quiet, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp}}}
	if tlsListener != nil {
		r.limit, r.secret = o.QuotaMB<<20, rand.Text()
		g, err := r.generator(o.Domain, local.IP)
		if err != nil {
			return nil, nil, err
		}
		r.urls = []string{"turn:" + addr + "?transport=udp", "turns:" + net.JoinHostPort(tlsName(o.TurnDomain), strconv.Itoa(tlsPort)) + "?transport=tcp"}
		// Use the RFC IPv4 default when the relay serves IPv4. On IPv6-only
		// hosts let Pion use the transport family for clients that omit it.
		cfg.StrictAddressFamily = g.publicIP("udp4") != nil
		cfg.PacketConnConfigs[0].RelayAddressGenerator = g
		cfg.PacketConnConfigs[0].PermissionHandler = g.permitPeer
		cfg.ListenerConfigs = []turn.ListenerConfig{{Listener: tlsListener, RelayAddressGenerator: g, PermissionHandler: g.permitPeer}}
		cfg.AuthHandler = restAuth(r.secret, &r.refused)
		cfg.QuotaHandler = g.quota
		cfg.EventHandler = g.events()
	}
	s, err := turn.NewServer(cfg)
	return r, s, err
}

// generator resolves the relay's public addresses and its own.
func (r *relay) generator(domain string, bind net.IP) (*generator, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil, err
	}
	own, err := ownAddrs(bind, ips)
	if err != nil {
		return nil, err
	}
	return newGenerator(ips, bind, own, &r.ledger, &r.refused), nil
}

func newGenerator(public []net.IP, bind net.IP, own map[netip.Addr]bool, l *ledger, refused *atomic.Int64) *generator {
	return &generator{public: public, bind: bind, own: own, ledger: l, refused: refused, allocations: newAllocations(), peers: peers{by: map[string]map[string]bool{}}}
}

// interfaceAddrs lists this host's interface addresses.
var interfaceAddrs = net.InterfaceAddrs

// ownAddrs are this server's addresses: those of its interfaces and those its
// name resolves to. A relay bound to loopback, used in tests, has none.
func ownAddrs(bind net.IP, public []net.IP) (map[netip.Addr]bool, error) {
	own := map[netip.Addr]bool{}
	if bind.IsLoopback() {
		return own, nil
	}
	addrs, err := interfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		ip, _, _ := net.ParseCIDR(a.String()) // an interface address is a prefix
		public = append(public, ip)
	}
	for _, ip := range public {
		addr, _ := netip.AddrFromSlice(ip)
		own[addr.Unmap()] = true
	}
	return own, nil
}

// restAuth checks a credential's expiry only when it opens an allocation.
// Later requests on that allocation (refresh, permission, channel binding)
// are accepted while the HMAC matches, so a relayed connection outlives its
// credential; the daily quota bounds what it relays.
func restAuth(secret string, refused *atomic.Int64) turn.AuthHandler {
	allocate := turn.LongTermTURNRESTAuthHandler(secret, quiet.NewLogger("turn"))
	return func(a *turn.RequestAttributes) (string, []byte, bool) {
		if a.Method != stun.MethodAllocate {
			mac := hmac.New(sha1.New, []byte(secret))
			mac.Write([]byte(a.Username))
			_, device, _ := strings.Cut(a.Username, ":")
			return device, turn.GenerateAuthKey(a.Username, a.Realm, base64.StdEncoding.EncodeToString(mac.Sum(nil))), true
		}
		device, key, ok := allocate(a)
		if !ok {
			refused.Add(1)
		}
		return device, key, ok
	}
}

type generator struct {
	public []net.IP
	bind   net.IP
	own    map[netip.Addr]bool
	*ledger
	refused     *atomic.Int64
	allocations *allocations
	peers       peers
	ports       sync.Map // the ports of this server's relay allocations
	permissions sync.Map // (relay endpoint, peer IP) installed by Pion
}

type permission struct {
	relay net.Addr
	peer  string
}

func (g *generator) events() turn.EventHandler {
	return turn.EventHandler{
		OnAllocationCreated: func(src, _ net.Addr, _, device, _ string, _ net.Addr, _ int) {
			g.allocations.created(device, src)
		},
		OnAllocationDeleted: func(src, _ net.Addr, _, device, _ string) {
			g.allocations.deleted(device, src)
		},
		OnPermissionCreated: func(src, _ net.Addr, _, _, _ string, relay net.Addr, peer net.IP) {
			g.permissions.Store(permission{relay, peer.String()}, true)
			g.peers.add(src, peer)
		},
		OnPermissionDeleted: func(src, _ net.Addr, _, _, _ string, relay net.Addr, peer net.IP) {
			g.permissions.Delete(permission{relay, peer.String()})
			g.peers.remove(src, peer)
		},
	}
}

// quota is Pion's check of a new allocation: at most maxDeviceAllocations
// per device and maxSourceAllocations per source IP.
func (g *generator) quota(device, _ string, src net.Addr) bool {
	ok := g.allocations.take(device, src)
	if !ok {
		g.refused.Add(1)
	}
	return ok
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
	g.ports.Store(addr.Port, true)
	return &allocation{PacketConn: conn, g: g, addr: addr, device: c.UserID}, addr, nil
}

func (*generator) Validate() error { return nil } // Addresses were resolved at startup.

// publicIP is the relay address of a family: the bound address when the name
// resolves to it, otherwise the one address of that family it resolves to.
// A family the bound address cannot serve has none.
func (g *generator) publicIP(network string) net.IP {
	if !g.serves(network) {
		return nil
	}
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

// serves reports whether a socket of the family can bind the bound address:
// "::" serves both families, any other address its own.
func (g *generator) serves(network string) bool {
	return g.bind.Equal(net.IPv6unspecified) || (network == "udp4") == (g.bind.To4() != nil)
}

// A public TURN credential must not grant access to the relay's private
// network or to special-purpose addresses. A loopback-only service can be
// exercised with a loopback peer. Each client transport address has
// permissions for at most maxPeers peer IPs.
func (g *generator) permitPeer(src net.Addr, peer net.IP) bool {
	return g.allowedPeer(peer) && g.peers.allow(src, peer)
}

func (g *generator) allowedPeer(peer net.IP) bool {
	if g.bind.IsLoopback() {
		return peer.IsLoopback()
	}
	ip, _ := netip.AddrFromSlice(peer)
	return peer.IsGlobalUnicast() && !peer.IsPrivate() && !slices.ContainsFunc(deniedPeers, func(p netip.Prefix) bool { return p.Contains(ip.Unmap()) })
}

// deniedPeers are special-purpose ranges that IsGlobalUnicast and IsPrivate
// leave in (contracts/rendezvous.md section 3).
var deniedPeers = prefixes(
	"100.64.0.0/10",   // carrier-grade NAT
	"0.0.0.0/8",       // this network
	"192.0.0.0/24",    // IETF protocol assignments
	"198.18.0.0/15",   // benchmarking
	"240.0.0.0/4",     // reserved
	"192.0.2.0/24",    // documentation
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"64:ff9b::/96",    // NAT64
	"64:ff9b:1::/48",  // local NAT64
	"::/96",           // IPv4-compatible
	"2002::/16",       // 6to4
	"2001::/32",       // Teredo
	"fec0::/10",       // site-local
	"2001:db8::/32",   // documentation
)

func prefixes(texts ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, text := range texts {
		out = append(out, netip.MustParsePrefix(text))
	}
	return out
}

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
	addr   *net.UDPAddr
	device string
	packet []byte // one byte more than Pion reads, to see a datagram it would cut
}

func (a *allocation) Close() error {
	a.g.ports.Delete(a.addr.Port)
	return a.PacketConn.Close()
}

func (a *allocation) ReadFrom(b []byte) (int, net.Addr, error) {
	if len(a.packet) != len(b)+1 {
		a.packet = make([]byte, len(b)+1)
	}
	for {
		n, src, err := a.PacketConn.ReadFrom(a.packet)
		if err != nil {
			return n, src, err
		}
		if a.forward(n, len(b), src) {
			copy(b, a.packet[:n])
			return n, src, nil
		}
	}
}

// forward admits a datagram from a peer: whole, from a permitted peer, and
// within the quota.
func (a *allocation) forward(n, room int, src net.Addr) bool {
	_, allowed := a.g.permissions.Load(permission{a.addr, src.(*net.UDPAddr).IP.String()})
	return n <= room && allowed && a.g.admit(a.device, n)
}

// WriteTo relays a datagram to a peer. One to this server goes only to one of
// its relay allocations. The quota is reserved first and given back if the
// send fails; no socket write happens under the ledger's lock.
func (a *allocation) WriteTo(b []byte, dst net.Addr) (int, error) {
	if a.g.toSelf(dst) {
		return len(b), nil
	}
	day, ok := a.g.reserve(a.device, len(b))
	if !ok {
		return len(b), nil
	}
	n, err := a.PacketConn.WriteTo(b, dst)
	if err != nil {
		a.g.refund(a.device, len(b), day)
	}
	return n, err
}

// toSelf reports a datagram to one of this server's addresses that is not for
// one of its relay allocations.
func (g *generator) toSelf(dst net.Addr) bool {
	u := dst.(*net.UDPAddr)
	ip, _ := netip.AddrFromSlice(u.IP)
	_, relay := g.ports.Load(u.Port)
	return g.own[ip.Unmap()] && !relay
}
