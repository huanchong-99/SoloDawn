//go:build netsim

package s8

import (
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/features/netsim"
)

const answerPort = 47800 // the node's UDP port and TCP signalling port

// Gateway permission rules under which miniupnpd grants only 30000 to 30009,
// so every request for another external port gets a different one.
var narrow = []string{"allow 30000-30009 0.0.0.0/0 1024-65535", "deny 0-65535 0.0.0.0/0 0-65535"}

// router is the right-hand port-restricted gateway with miniupnpd, a socket
// on the client's 47800 and a sender in the WAN. Without a mapping no
// datagram from the WAN reaches the client. The gateway's conntrack keeps
// translating a flow it has seen after its rule is gone, so reach probes
// from a new source port each time and established probes from one port.
type router struct {
	n           netsim.Network
	client, wan *net.UDPConn
	public      netip.Addr
	m           *child
}

func startRouter(t *testing.T, config ...string) *router {
	t.Helper()
	n := netsim.Start(t, netsim.PortRestricted, netsim.PortRestricted)
	n.Right.StartUPnP(t, config...)
	return &router{
		n:      n,
		client: udpSocket(t, n.Right.Client, fmt.Sprintf("%s:%d", netsim.ClientIP, answerPort)),
		wan:    udpSocket(t, n.WAN, netsim.WANIP+":3478"),
		public: netip.MustParseAddr(n.Right.PublicIP),
		m:      spawn(t, n.Right.Client, "mapper"),
	}
}

func (r *router) reach(t *testing.T, port int) bool {
	t.Helper()
	return arrives(t, udpSocket(t, r.n.WAN, netsim.WANIP+":0"), r.client, netip.AddrPortFrom(r.public, uint16(port)))
}

func (r *router) established(t *testing.T, port int) bool {
	t.Helper()
	return arrives(t, r.wan, r.client, netip.AddrPortFrom(r.public, uint16(port)))
}

// occupy has another device on the LAN, 10.0.0.3, map 47800/UDP by NAT-PMP
// before the node asks.
func occupy(t *testing.T, client string) {
	t.Helper()
	command(t, "ip", "-n", client, "address", "add", "10.0.0.3/24", "dev", "eth0")
	c := udpSocket(t, client, "10.0.0.3:0")
	// Version 0, opcode 1 (UDP), internal and external port 47800, 3600 s.
	req := []byte{0, 1, 0, 0, 0xba, 0xb8, 0xba, 0xb8, 0, 0, 0x0e, 0x10}
	_, err := c.WriteToUDPAddrPort(req, netip.MustParseAddrPort("10.0.0.1:5351"))
	must(t, err)
	must(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
	var resp [16]byte
	n, _, err := c.ReadFromUDPAddrPort(resp[:])
	must(t, err)
	if n != 16 || resp[3] != 0 || resp[10] != 0xba || resp[11] != 0xb8 {
		t.Fatalf("10.0.0.3 did not get 47800: % x", resp[:n])
	}
}

func (r *router) call(t *testing.T, req msg) msg {
	t.Helper()
	return r.m.call(t, req, 20*time.Second)
}

var dnat = regexp.MustCompile(`dport (\d+) .*dnat (?:ip )?to [0-9.]+:(\d+)`)

// mappings lists the gateway's DNAT rules from miniupnpd as "external->internal".
func (r *router) mappings(t *testing.T) []string {
	t.Helper()
	out := command(t, "ip", "netns", "exec", r.n.Right.Gateway, "nft", "list", "chain", "ip", "netsim", "prerouting_miniupnpd")
	var list []string
	for _, m := range dnat.FindAllStringSubmatch(out, -1) {
		list = append(list, m[1]+"->"+m[2])
	}
	if len(list) == 0 {
		t.Logf("no DNAT rule read from:\n%s", out)
	}
	return list
}

func TestVersions(t *testing.T) {
	for _, args := range [][]string{{"miniupnpd", "--version"}, {"nft", "--version"}, {"conntrack", "--version"}, {"uname", "-srm"}} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput() // miniupnpd may exit 1 here
		t.Logf("%v (err %v):\n%s", args, err, out)
	}
}

// Question 1 with go-nat as X-1 added it: discovery, external address,
// mapping, reachability, renewal and deletion, for each protocol it finds.
func TestLibrary(t *testing.T) {
	r := startRouter(t)
	d := r.call(t, msg{T: "discover", Ms: 8000})
	t.Logf("DiscoverNATs: %v arriving after %v ms; channel closed after %d ms", d.Types, d.After, d.Ms)
	g := r.call(t, msg{T: "gateway", Ms: 8000})
	t.Logf("DiscoverGateway: %q after %d ms, err %q", g.Kind, g.Ms, g.Err)
	if len(d.Types) != 2 || d.Types[0] != "NAT-PMP" || d.Types[1] != "UPNP (IG2-IP2)" {
		t.Errorf("found %v, want NAT-PMP then UPNP (IG2-IP2)", d.Types)
	}
	for _, typ := range d.Types {
		ext := r.call(t, msg{T: "external", NAT: typ})
		first := r.call(t, msg{T: "map", NAT: typ, Proto: "udp", Internal: answerPort, Lifetime: 60})
		reached := r.reach(t, first.Port)
		again := r.call(t, msg{T: "map", NAT: typ, Proto: "udp", Internal: answerPort, Lifetime: 60})
		flow := r.established(t, first.Port)
		rules := r.mappings(t)
		del := r.call(t, msg{T: "delete", NAT: typ, Proto: "udp", Internal: answerPort})
		after, still := r.reach(t, first.Port), r.established(t, first.Port)
		t.Logf("%s: external %s (%d ms, err %q); mapped %d in %d ms (err %q), reachable %v; renewed to %d (err %q); gateway rules %v; delete err %q; after delete reachable from a new port %v, from the established flow %v",
			typ, ext.IP, ext.Ms, ext.Err, first.Port, first.Ms, first.Err, reached, again.Port, again.Err, rules, del.Err, after, still)
		if ext.IP != r.public.String() || first.Err != "" || !reached || again.Port != first.Port || !flow || !still {
			t.Errorf("%s: mapping cycle failed", typ)
		}
		if want := typ == "NAT-PMP"; after != want { // go-nat's NAT-PMP delete sends nothing
			t.Errorf("%s: reachable after DeletePortMapping = %v, want %v", typ, after, want)
		}
	}
}

// A gateway that grants only 30000 to 30009: what go-nat reports, and what
// the protocols report when asked directly.
func TestGrantedPortDiffers(t *testing.T) {
	r := startRouter(t, narrow...)
	r.call(t, msg{T: "discover", Ms: 8000})
	pmp := r.call(t, msg{T: "map", NAT: "NAT-PMP", Proto: "udp", Internal: answerPort, Lifetime: 60})
	rules := r.mappings(t)
	t.Logf("go-nat NAT-PMP: reported %d (err %q), reachable there %v; gateway rules %v", pmp.Port, pmp.Err, r.reach(t, pmp.Port), rules)
	if len(rules) != 1 || r.reach(t, pmp.Port) || !r.reach(t, external(rules[0])) {
		t.Errorf("go-nat NAT-PMP: want an unreachable reported port and a reachable granted one")
	}
	upnp := r.call(t, msg{T: "map", NAT: "UPNP (IG2-IP2)", Proto: "udp", Internal: answerPort, Lifetime: 60})
	t.Logf("go-nat UPnP: port %d, err %q after %d ms", upnp.Port, upnp.Err, upnp.Ms)
	if upnp.Err == "" {
		t.Errorf("go-nat UPnP mapped %d outside the allowed range", upnp.Port)
	}

	direct := r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60})
	t.Logf("NAT-PMP asked for %d: granted %d for %d s (err %q), reachable %v", answerPort, direct.Port, direct.Lifetime, direct.Err, r.reach(t, direct.Port))
	r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, Lifetime: 0})
	add := r.call(t, msg{T: "igd", Kind: "add", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60})
	anyPort := r.call(t, msg{T: "igd", Kind: "any", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60})
	t.Logf("IGDv2 AddPortMapping(%d): err %q; AddAnyPortMapping(%d): %d (err %q), reachable %v",
		answerPort, add.Err, answerPort, anyPort.Port, anyPort.Err, r.reach(t, anyPort.Port))
	if direct.Port < 30000 || direct.Port > 30009 || add.Err == "" || anyPort.Port < 30000 || anyPort.Port > 30009 {
		t.Errorf("direct protocols: want granted ports in 30000-30009")
	}
}

// external is the external port of a rule as mappings lists it.
func external(rule string) int {
	port, _, _ := strings.Cut(rule, "->")
	n, _ := strconv.Atoi(port)
	return n
}

// The protocols used directly when another device holds 47800/UDP, as when
// a second node behind the same router asked first.
func TestProtocols(t *testing.T) {
	r := startRouter(t)
	occupy(t, r.n.Right.Client)
	ext := r.call(t, msg{T: "pmp", Kind: "external"})
	p := r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60})
	reached := r.reach(t, p.Port)
	renew := r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, External: p.Port, Lifetime: 60})
	del := r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, Lifetime: 0})
	t.Logf("NAT-PMP: external %s in %d ms; asked %d, granted %d for %d s in %d ms, reachable %v; renewed %d (err %q); delete %q, reachable after %v",
		ext.IP, ext.Ms, answerPort, p.Port, p.Lifetime, p.Ms, reached, renew.Port, renew.Err, del.Err, r.reach(t, p.Port))
	if ext.IP != r.public.String() || p.Port == answerPort || !reached || renew.Port != p.Port || r.reach(t, p.Port) {
		t.Errorf("NAT-PMP cycle: want another port granted, reachable, renewed in place and gone after delete")
	}

	d := r.call(t, msg{T: "igd", Kind: "discover", Ms: 8000})
	add := r.call(t, msg{T: "igd", Kind: "add", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60})
	anyPort := r.call(t, msg{T: "igd", Kind: "any", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60})
	reached = r.reach(t, anyPort.Port)
	get := r.call(t, msg{T: "igd", Kind: "get", Proto: "udp", External: anyPort.Port})
	renewed := r.call(t, msg{T: "igd", Kind: "add", Proto: "udp", Internal: answerPort, External: anyPort.Port, Lifetime: 60})
	gone := r.call(t, msg{T: "igd", Kind: "delete", Proto: "udp", External: anyPort.Port})
	t.Logf("IGDv2 at %s found in %d ms (err %q); AddPortMapping(%d) err %q; AddAnyPortMapping %d in %d ms (err %q), reachable %v; entry %s:%d lease %d s; renew err %q; delete err %q, reachable after %v",
		d.IP, d.Ms, d.Err, answerPort, add.Err, anyPort.Port, anyPort.Ms, anyPort.Err, reached, get.IP, get.Port, get.Lifetime, renewed.Err, gone.Err, r.reach(t, anyPort.Port))
	if d.Err != "" || add.Err == "" || anyPort.Err != "" || anyPort.Port == answerPort || !reached || renewed.Err != "" || r.reach(t, anyPort.Port) {
		t.Errorf("IGDv2 cycle: want a conflict, another port reserved, reachable, renewed and gone after delete")
	}
	tcp(t, r)
}

// TCP: the signalling listener by NAT-PMP and the ICE-TCP listener by IGDv2.
func tcp(t *testing.T, r *router) {
	t.Helper()
	sig := r.call(t, msg{T: "pmp", Proto: "tcp", Internal: answerPort, External: answerPort, Lifetime: 60})
	ice := r.call(t, msg{T: "igd", Kind: "any", Proto: "tcp", Internal: answerPort + 1, External: answerPort + 1, Lifetime: 60})
	for _, c := range []struct {
		internal int
		m        msg
	}{{answerPort, sig}, {answerPort + 1, ice}} {
		var l *net.TCPListener
		inNamespace(t, r.n.Right.Client, func() {
			var err error
			l, err = net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP(netsim.ClientIP), Port: c.internal})
			must(t, err)
		})
		ok := false
		inNamespace(t, r.n.WAN, func() {
			conn, err := net.DialTimeout("tcp4", netip.AddrPortFrom(r.public, uint16(c.m.Port)).String(), time.Second)
			if ok = err == nil; ok {
				conn.Close()
			}
		})
		l.Close()
		t.Logf("TCP %d mapped to %d (err %q): connect from WAN %v", c.internal, c.m.Port, c.m.Err, ok)
		if !ok {
			t.Errorf("TCP %d not reachable through its mapping", c.internal)
		}
	}
}
