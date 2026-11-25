package rendezvous

import (
	"bytes"
	"errors"
	"net"
	"net/http/httptest"
	"net/netip"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/stun/v4"
	"github.com/pion/turn/v5"
)

func TestRESTLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := relay{secret: "secret", stun: "stun:rv:3478", urls: []string{"turn:rv:3478?transport=udp", "turns:trv:443?transport=tcp"}}
		check := func(kind, user, pass string) {
			t.Helper()
			want := proto.ICEConfig{Type: kind, TTL: 3600, ICE: []proto.ICEServer{{URLs: []string{"stun:rv:3478"}}, {URLs: r.urls, Username: user, Credential: pass}}}
			if got := r.ice(kind, "device"); !reflect.DeepEqual(got, want) {
				t.Fatalf("ice = %+v; want %+v", got, want)
			}
		}
		// The username carries the device's alias, never its id.
		const alias = "ePJYJhd6mUJpL1AoUNXQmg"
		const user = "946688400:" + alias
		const password = "28hFL6b2YhaUPCt5AFkCzIMsnGg="
		check("ready", user, password)
		time.Sleep(123 * time.Second)
		check("ice", "946688523:"+alias, "J2UC04a0O5bBRjiTqrhQTU0jVAA=")
		// An allocation is opened with the library's verifier: it accepts
		// until the end of the expiry second and refuses from the next one.
		// Every other request keeps the same device and key after expiry.
		allocate := &turn.RequestAttributes{Username: user, Realm: "rv", Method: stun.MethodAllocate}
		refresh := &turn.RequestAttributes{Username: user, Realm: "rv", Method: stun.MethodRefresh}
		var refused atomic.Int64
		auth := restAuth("secret", &refused)
		key := turn.GenerateAuthKey(user, "rv", password)
		time.Sleep(3478*time.Second - time.Nanosecond)
		for _, req := range []*turn.RequestAttributes{allocate, refresh} {
			if id, k, ok := auth(req); !ok || id != alias || !bytes.Equal(k, key) {
				t.Fatalf("%v before expiry: %q %x %v", req.Method, id, k, ok)
			}
		}
		time.Sleep(time.Nanosecond)
		if id, k, ok := auth(allocate); ok || k != nil || id != "" || refused.Load() != 1 {
			t.Fatalf("allocate after expiry: %q %x %v, %d refused", id, k, ok, refused.Load())
		}
		if id, k, ok := auth(refresh); !ok || id != alias || !bytes.Equal(k, key) || refused.Load() != 1 {
			t.Fatalf("refresh after expiry: %q %x %v, %d refused", id, k, ok, refused.Load())
		}
	})
}

// Each packet costs its payload and 40 bytes; once the quota is spent nothing
// passes, an empty packet included. A failed send gives its charge back, on
// the day it was made.
func TestDailyLedger(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := ledger{limit: 100}
		if !l.admit("a", 4) || l.admit("a", 17) || !l.admit("a", 16) || l.admit("a", 0) || !l.admit("b", 3) {
			t.Fatal("wrong admissions")
		}
		for _, tc := range []struct {
			id   string
			used int64
		}{{"a", 100}, {"b", 43}, {"c", 0}} {
			if got := l.quota(tc.id); got != (proto.Quota{Type: "quota", Used: tc.used, Limit: 100}) {
				t.Fatal(got)
			}
		}
		day, ok := l.reserve("b", 7)
		if !ok || l.quota("b").Used != 90 {
			t.Fatal(ok, l.quota("b"))
		}
		l.refund("b", 7, day)
		if got := l.quota("b").Used; got != 43 {
			t.Fatal(got)
		}
		day, _ = l.reserve("b", 7)
		time.Sleep(24*time.Hour - time.Nanosecond)
		if got := l.quota("a").Used; got != 100 {
			t.Fatal(got)
		}
		time.Sleep(time.Nanosecond)
		if got := l.quota("a").Used; got != 0 {
			t.Fatal(got)
		}
		l.admit("b", 10)
		l.refund("b", 7, day) // yesterday's charge
		if got := l.quota("b").Used; got != 50 {
			t.Fatal(got)
		}
	})
}

func udpLocal(t *testing.T) net.PacketConn {
	t.Helper()
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func testGenerator(l *ledger, bind net.IP, public ...net.IP) *generator {
	return newGenerator(public, bind, map[netip.Addr]bool{}, DefaultRelayPorts, l, new(atomic.Int64))
}

func TestMeteredSocket(t *testing.T) {
	l := ledger{limit: 85} // "hi" and "yes": 2 + 40 + 3 + 40
	g := testGenerator(&l, net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 1))
	raw, endpoint, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp4", UserID: "device"})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	peer := udpLocal(t)
	ip := peer.LocalAddr().(*net.UDPAddr).IP
	var b [32]byte
	checkRelayReceive(t, g, raw, peer, endpoint, ip, b[:])
	checkRelaySend(t, &l, raw, peer, ip, b[:])
	// With the quota spent, empty packets pass in neither direction.
	if n, err := raw.WriteTo(nil, peer.LocalAddr()); n != 0 || err != nil {
		t.Fatal(n, err)
	}
	expectNoRelayPacket(t, peer, b[:])
	peer.WriteTo(nil, endpoint)
	expectNoRelayPacket(t, raw, b[:])
	g.events().OnPermissionDeleted(peer.LocalAddr(), nil, "", "", "", endpoint, ip)
	if _, ok := g.permissions.Load(permission{endpoint, ip.String()}); ok {
		t.Fatal("permission not removed")
	}
}

func expectNoRelayPacket(t *testing.T, c net.PacketConn, b []byte) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if n, src, err := c.ReadFrom(b); n != 0 || src != nil || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read %d from %v, %v; want nothing", n, src, err)
	}
}

func checkRelayReceive(t *testing.T, g *generator, raw, peer net.PacketConn, endpoint net.Addr, ip net.IP, b []byte) {
	t.Helper()
	// An unpermitted packet queued before the permission must not spend quota.
	peer.WriteTo([]byte("bad"), endpoint)
	expectNoRelayPacket(t, raw, b)
	if got := g.ledger.quota("device").Used; got != 0 {
		t.Fatal(got)
	}
	g.events().OnPermissionCreated(peer.LocalAddr(), nil, "", "", "", endpoint, ip)
	// Never forward or charge a prefix when Pion's destination buffer is
	// smaller; the socket reads one byte more than that buffer.
	peer.WriteTo([]byte("large"), endpoint)
	peer.WriteTo([]byte("hi"), endpoint)
	raw.SetReadDeadline(time.Now().Add(time.Second))
	if n, _, err := raw.ReadFrom(b[:2]); err != nil || n != 2 || string(b[:n]) != "hi" {
		t.Fatal(n, err, b[:n])
	}
	if got := g.ledger.quota("device").Used; got != 42 {
		t.Fatal(got)
	}
}

func checkRelaySend(t *testing.T, l *ledger, raw, peer net.PacketConn, ip net.IP, b []byte) {
	t.Helper()
	// A failed send is not charged.
	if n, err := raw.WriteTo([]byte("x"), &net.UDPAddr{IP: ip, Port: -1}); n != 0 || !errors.Is(err, syscall.EINVAL) {
		t.Fatal(n, err)
	}
	raw.WriteTo([]byte("drop"), peer.LocalAddr())
	raw.WriteTo([]byte("yes"), peer.LocalAddr())
	peer.SetReadDeadline(time.Now().Add(time.Second))
	if n, _, err := peer.ReadFrom(b); err != nil || string(b[:n]) != "yes" {
		t.Fatal(n, err)
	}
	if got := l.quota("device").Used; got != 85 {
		t.Fatal(got)
	}
}

func TestGenerator(t *testing.T) {
	v4, v6 := net.ParseIP("192.0.2.1"), net.ParseIP("2001:db8::1")
	g := testGenerator(&ledger{}, net.IPv6unspecified, v4, v6)
	if !g.publicIP("udp4").Equal(v4) || !g.publicIP("udp6").Equal(v6) {
		t.Fatal("wrong public address family")
	}
	g.public = []net.IP{v4}
	if c, a, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp6"}); c != nil || a != nil || err.Error() != "no unambiguous relay address for requested address family" {
		t.Fatalf("IPv6: %v, %v, %v", c, a, err)
	}
	g.bind = net.IPv4(127, 0, 0, 1)
	g.public = append(g.public, g.bind)
	if got := g.publicIP("udp4"); !got.Equal(g.bind) {
		t.Fatal(got)
	}
	g.bind = net.ParseIP("127.0.0.2")
	if got := g.publicIP("udp4"); got != nil {
		t.Fatal(got)
	}
	g.public = []net.IP{v4}
	g.portRange = PortRange{0, -1}
	if c, a, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp4"}); c != nil || a != nil || err != errNoPort {
		t.Fatalf("no relay ports: %v, %v, %v", c, a, err)
	}
	if c, a, err := g.AllocateListener(turn.AllocateListenerConfig{}); c != nil || a != nil || !errors.Is(err, errTransport) {
		t.Fatal(c, a, err)
	}
	if c, err := g.AllocateConn(turn.AllocateConnConfig{}); c != nil || !errors.Is(err, errTransport) {
		t.Fatal(c, err)
	}
}

// A bound address serves the families it can bind: "::" both, any other its
// own. A family it cannot serve has no relay address, whatever DNS says.
func TestServedFamilies(t *testing.T) {
	v4, v6 := net.ParseIP("192.0.2.1"), net.ParseIP("2001:db8::1")
	for _, tc := range []struct {
		bind     string
		ipv4, v6 bool
	}{{"0.0.0.0", true, false}, {"192.0.2.1", true, false}, {"::", true, true}, {"2001:db8::1", false, true}} {
		g := testGenerator(&ledger{}, net.ParseIP(tc.bind), v4, v6)
		if (g.publicIP("udp4") != nil) != tc.ipv4 || (g.publicIP("udp6") != nil) != tc.v6 {
			t.Errorf("bound to %s: IPv4 %v, IPv6 %v", tc.bind, g.publicIP("udp4"), g.publicIP("udp6"))
		}
	}
}

// Peers on the relay's private network and in special-purpose ranges are
// refused. A loopback-only relay, used in tests, allows loopback peers only.
func TestPeerFilter(t *testing.T) {
	g := testGenerator(&ledger{}, nil)
	src := &net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: 1}
	for _, tc := range []struct {
		bind, peer string
		want       bool
	}{
		{"0.0.0.0", "8.8.8.8", true}, {"0.0.0.0", "10.0.0.1", false}, {"0.0.0.0", "127.0.0.1", false}, {"::", "fe80::1", false}, {"127.0.0.1", "127.0.0.1", true}, {"127.0.0.1", "8.8.8.8", false},
		{"::", "100.64.0.0", false}, {"::", "100.127.255.255", false}, {"::", "100.128.0.0", true},
		{"::", "0.1.2.3", false}, {"::", "192.0.0.9", false}, {"::", "198.18.0.1", false}, {"::", "198.19.255.255", false}, {"::", "198.20.0.0", true},
		{"::", "240.0.0.1", false}, {"::", "255.255.255.254", false}, {"::", "192.0.2.1", false}, {"::", "198.51.100.1", false}, {"::", "203.0.113.1", false},
		{"::", "64:ff9b::808:808", false}, {"::", "64:ff9b:1::1", false}, {"::", "::808:808", false}, {"::", "2002:808:808::1", false},
		{"::", "2001:0:4136:e378::1", false}, {"::", "2001:1::1", true}, {"::", "fec0::1", false}, {"::", "2001:db8::1", false}, {"::", "2606:4700::1111", true},
		{"::", "::ffff:8.8.8.8", true}, {"::", "::ffff:10.0.0.1", false}, {"::", "::ffff:0:808:808", false},
	} {
		g.bind = net.ParseIP(tc.bind)
		if got := g.permitPeer(src, net.ParseIP(tc.peer)); got != tc.want {
			t.Errorf("%s to %s = %v", tc.bind, tc.peer, got)
		}
	}
}

func TestSTUNOnly(t *testing.T) {
	o := Options{Domain: "localhost", NoRelay: true, QuotaMB: 50}
	udp := udpLocal(t)
	addr := udp.LocalAddr().String()
	r, s, err := o.start(udp, nil, 443)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	clientSocket := udpLocal(t)
	c, err := turn.NewClient(&turn.ClientConfig{Conn: clientSocket, STUNServerAddr: addr, TURNServerAddr: addr, Username: "fake", Password: "fake", RTO: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	mapped, err := c.SendBindingRequest()
	if err != nil || mapped.String() != clientSocket.LocalAddr().String() {
		t.Fatalf("binding %v, %v; want %v", mapped, err, clientSocket.LocalAddr())
	}
	if a, err := c.AllocateWithContext(t.Context()); err == nil || a != nil {
		t.Fatalf("STUN-only allocation: %v, %v", a, err)
	}
	want := []proto.ICEServer{{URLs: []string{"stun:localhost:" + strings.Split(addr, ":")[1]}}}
	sig := httptest.NewServer(handler(fstest.MapFS{}, time.Second, r))
	defer sig.Close()
	ws, nonce := dial(t, sig.URL)
	k := key(4)
	send(t, ws, proto.Auth{Type: "auth", Key: k.ID(), Signature: k.SignChallenge(nonce)})
	expect(t, ws, proto.ICEConfig{Type: "ready", TTL: 3600, ICE: want})
	send(t, ws, proto.RVFrame{Type: "ice"})
	expect(t, ws, proto.ICEConfig{Type: "ice", TTL: 3600, ICE: want})
	send(t, ws, proto.RVFrame{Type: "quota"})
	expect(t, ws, proto.Quota{Type: "quota", Used: 0, Limit: 0})
}

func TestNodeQuota(t *testing.T) {
	c := login(t, fixture(t), key(1))
	send(t, c, proto.RVFrame{Type: "quota"})
	expect(t, c, proto.Quota{Type: "quota", Used: 0, Limit: 0})
}

func TestOmittedFamilyOnIPv6Stream(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) { testOmittedFamily(t, host) })
	}
}

func testOmittedFamily(t *testing.T, host string) {
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Domain: host, TurnDomain: "turn.localhost"}
	udp, err := net.ListenPacket("udp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	r, s, err := o.start(udp, l, l.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, err := net.Dial("tcp6", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cred := r.ice("ready", "device").ICE[1]
	// Pion omits REQUESTED-ADDRESS-FAMILY for an IPv4 allocation.
	c, err := turn.NewClient(&turn.ClientConfig{Conn: turn.NewSTUNConn(conn), TURNServerAddr: l.Addr().String(), Username: cred.Username, Password: cred.Credential, RequestedAddressFamily: turn.RequestedAddressFamilyIPv4})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	a, err := c.AllocateWithContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if got := a.LocalAddr().(*net.UDPAddr).IP; !got.Equal(net.ParseIP(host)) {
		t.Fatal(got)
	}
}

// A credential's expiry limits new allocations only. Once it has passed, the
// allocation it opened still refreshes and still creates permissions; a new
// allocation with it is refused.
func TestRefreshAfterCredentialExpiry(t *testing.T) {
	udp := udpLocal(t)
	r, s, err := Options{Domain: "localhost", TurnDomain: "turn.localhost"}.start(udp, listenLocal(t), 443)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	user, pass, _ := turn.GenerateLongTermTURNRESTCredentials(r.secret, "device", time.Second)
	expiry, _, _ := strings.Cut(user, ":")
	client := func() *turn.Client {
		c, err := turn.NewClient(&turn.ClientConfig{Conn: udpLocal(t), TURNServerAddr: udp.LocalAddr().String(), Username: user, Password: pass})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		if err := c.Listen(); err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := client()
	if _, err := c.Allocate(); err != nil {
		t.Fatal(err)
	}
	end, _ := strconv.ParseInt(expiry, 10, 64)
	time.Sleep(time.Until(time.Unix(end+1, 0)))

	if err := c.CreatePermission(udpLocal(t).LocalAddr()); err != nil {
		t.Errorf("permission after expiry: %v", err)
	}
	refresh := func(attrs ...stun.Setter) *stun.Message {
		t.Helper()
		msg := stun.MustBuild(append([]stun.Setter{stun.TransactionID, stun.NewType(stun.MethodRefresh, stun.ClassRequest)}, attrs...)...)
		res, err := c.PerformTransaction(msg, udp.LocalAddr(), false)
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	var nonce stun.Nonce
	var realm stun.Realm
	challenge := refresh()
	if nonce.GetFrom(challenge) != nil || realm.GetFrom(challenge) != nil || realm.String() != "localhost" {
		t.Fatalf("unauthenticated refresh: %v", challenge)
	}
	var code stun.ErrorCodeAttribute
	if res := refresh(stun.NewUsername(user), realm, nonce, stun.NewLongTermIntegrity(user, realm.String(), pass), stun.Fingerprint); res.Type.Class != stun.ClassSuccessResponse {
		code.GetFrom(res)
		t.Errorf("refresh after expiry: %v %v", res, code)
	}
	if a, err := client().Allocate(); a != nil || err == nil || err.Error() != "Allocate error response (error 401: )" {
		t.Fatalf("new allocation after expiry: %v, %v", a, err)
	}
}
