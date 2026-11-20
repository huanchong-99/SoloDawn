package rendezvous

import (
	"bytes"
	"errors"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
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
		const user = "946688400:device"
		const password = "7ZUC+m0DOLVWukPUPjqFfOz+vDw="
		check("ready", user, password)
		time.Sleep(123 * time.Second)
		check("ice", "946688523:device", "ay+89oSv4H/J4DZABOURaF+mr80=")
		// The verifier is the library's: it accepts until the end of the
		// expiry second and refuses from the next one.
		req := &turn.RequestAttributes{Username: user, Realm: "rv"}
		auth := turn.LongTermTURNRESTAuthHandler("secret", nil)
		time.Sleep(3478*time.Second - time.Nanosecond)
		id, k, ok := auth(req)
		if !ok || id != "device" || !bytes.Equal(k, turn.GenerateAuthKey(user, "rv", password)) {
			t.Fatalf("before expiry: %q %x %v", id, k, ok)
		}
		time.Sleep(time.Nanosecond)
		if id, k, ok := auth(req); ok || k != nil || id != "" {
			t.Fatalf("after expiry: %q %x %v", id, k, ok)
		}
	})
}

func TestDailyLedger(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := ledger{limit: 10}
		if !l.admit("a", 4) || l.admit("a", 7) || !l.admit("a", 6) || l.admit("a", 1) || !l.admit("b", 3) {
			t.Fatal("wrong admissions")
		}
		for _, tc := range []struct {
			id   string
			used int64
		}{{"a", 10}, {"b", 3}, {"c", 0}} {
			if got := l.quota(tc.id); got != (proto.Quota{Type: "quota", Used: tc.used, Limit: 10}) {
				t.Fatal(got)
			}
		}
		time.Sleep(24*time.Hour - time.Nanosecond)
		if got := l.quota("a").Used; got != 10 {
			t.Fatal(got)
		}
		time.Sleep(time.Nanosecond)
		if got := l.quota("a").Used; got != 0 {
			t.Fatal(got)
		}
		if !l.admit("b", 10) || l.quota("b").Used != 10 {
			t.Fatal("budget did not reset")
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

func TestMeteredSocket(t *testing.T) {
	l := ledger{limit: 5}
	g := generator{ledger: &l, bind: net.IPv4(127, 0, 0, 1), public: []net.IP{net.IPv4(127, 0, 0, 1)}}
	raw, endpoint, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp4", UserID: "device"})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	peer := udpLocal(t)
	readOne := func(c net.PacketConn, want string) {
		t.Helper()
		c.SetReadDeadline(time.Now().Add(time.Second))
		var b [32]byte
		n, _, err := c.ReadFrom(b[:])
		if err != nil || string(b[:n]) != want {
			t.Fatalf("read %q, %v; want %q", b[:n], err, want)
		}
	}
	// An unpermitted packet queued before the permission must not spend quota.
	peer.WriteTo([]byte("bad"), endpoint)
	raw.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	var b [32]byte
	if n, src, err := raw.ReadFrom(b[:]); n != 0 || src != nil || err == nil {
		t.Fatalf("unpermitted packet: %d %v %v", n, src, err)
	}
	if got := l.quota("device").Used; got != 0 {
		t.Fatal(got)
	}
	ip := peer.LocalAddr().(*net.UDPAddr).IP
	g.events().OnPermissionCreated(nil, nil, "", "", "", endpoint, ip)
	// Never forward or charge a prefix when Pion's destination buffer is smaller.
	peer.WriteTo([]byte("large"), endpoint)
	peer.WriteTo([]byte("hi"), endpoint)
	raw.SetReadDeadline(time.Now().Add(time.Second))
	if n, _, err := raw.ReadFrom(b[:2]); err != nil || n != 2 || string(b[:n]) != "hi" {
		t.Fatal(n, err, b[:n])
	}
	if got := l.quota("device").Used; got != 2 {
		t.Fatal(got)
	}
	// A failed send is not charged.
	if n, err := raw.WriteTo([]byte("x"), &net.UDPAddr{IP: ip, Port: -1}); n != 0 || !errors.Is(err, syscall.EINVAL) {
		t.Fatal(n, err)
	}
	raw.WriteTo([]byte("drop"), peer.LocalAddr())
	raw.WriteTo([]byte("yes"), peer.LocalAddr())
	readOne(peer, "yes")
	if got := l.quota("device").Used; got != 5 {
		t.Fatal(got)
	}
	g.events().OnPermissionDeleted(nil, nil, "", "", "", endpoint, ip)
	if _, ok := g.permissions.Load(permission{endpoint, ip.String()}); ok {
		t.Fatal("permission not removed")
	}
}

func TestGenerator(t *testing.T) {
	v4, v6 := net.ParseIP("192.0.2.1"), net.ParseIP("2001:db8::1")
	g := generator{public: []net.IP{v4, v6}}
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
	if c, a, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp4", RequestedPort: -1}); c != nil || a != nil || err == nil || err.Error() != "listen udp4: address -1: invalid port" {
		t.Fatalf("invalid port: %v, %v, %v", c, a, err)
	}
	if c, a, err := g.AllocateListener(turn.AllocateListenerConfig{}); c != nil || a != nil || !errors.Is(err, errTransport) {
		t.Fatal(c, a, err)
	}
	if c, err := g.AllocateConn(turn.AllocateConnConfig{}); c != nil || !errors.Is(err, errTransport) {
		t.Fatal(c, err)
	}
	for _, tc := range []struct {
		bind, peer string
		want       bool
	}{
		{"0.0.0.0", "8.8.8.8", true}, {"0.0.0.0", "10.0.0.1", false}, {"0.0.0.0", "127.0.0.1", false}, {"::", "fe80::1", false}, {"127.0.0.1", "127.0.0.1", true}, {"127.0.0.1", "8.8.8.8", false},
		{"::", "100.64.0.0", false}, {"::", "100.127.255.255", false}, {"::", "100.128.0.0", true},
	} {
		g.bind = net.ParseIP(tc.bind)
		if got := g.permitPeer(nil, net.ParseIP(tc.peer)); got != tc.want {
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
