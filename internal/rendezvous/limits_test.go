package rendezvous

import (
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/turn/v5"
)

// loopbackRelay serves TURN on 127.0.0.1, where loopback peers are allowed.
func loopbackRelay(t *testing.T) (*relay, *turn.Server, net.PacketConn) {
	t.Helper()
	udp := udpLocal(t)
	r, s, err := Options{Domain: "localhost", TurnDomain: "turn.localhost", QuotaMB: 50}.start(udp, listenLocal(t), 443)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return r, s, udp
}

// turnClient signs in to the relay as a device from a socket of its own.
func turnClient(t *testing.T, r *relay, udp net.PacketConn, device string) *turn.Client {
	t.Helper()
	cred := r.ice("ready", device).ICE[1]
	c, err := turn.NewClient(&turn.ClientConfig{Conn: udpLocal(t), TURNServerAddr: udp.LocalAddr().String(), Username: cred.Username, Password: cred.Credential})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err := c.Listen(); err != nil {
		t.Fatal(err)
	}
	return c
}

func allocate(t *testing.T, r *relay, udp net.PacketConn, device string) (net.PacketConn, error) {
	t.Helper()
	a, err := turnClient(t, r, udp, device).Allocate()
	if err == nil {
		t.Cleanup(func() { a.Close() })
	}
	return a, err
}

func waitAllocations(t *testing.T, s *turn.Server, n int) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); s.AllocationCount() != n; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%d allocations; want %d", s.AllocationCount(), n)
		}
	}
}

// A device has at most 8 allocations and a source IP at most 32. A closed
// allocation gives its place back.
func TestAllocationLimits(t *testing.T) {
	r, s, udp := loopbackRelay(t)
	const refusal = "Allocate error response (error 486: )"
	var first net.PacketConn
	for i := range 8 {
		a, err := allocate(t, r, udp, "a")
		if err != nil {
			t.Fatalf("allocation %d: %v", i, err)
		}
		first = a
	}
	if _, err := allocate(t, r, udp, "a"); err == nil || err.Error() != refusal || r.refused.Load() != 1 {
		t.Fatalf("a ninth allocation: %v, %d refused", err, r.refused.Load())
	}
	first.Close()
	waitAllocations(t, s, 7)
	for _, device := range []string{"a", "b", "b", "b", "b", "b", "b", "b", "b", "c", "c", "c", "c", "c", "c", "c", "c", "d", "d", "d", "d", "d", "d", "d", "d"} {
		if _, err := allocate(t, r, udp, device); err != nil {
			t.Fatalf("%s: %v", device, err)
		}
	}
	if _, err := allocate(t, r, udp, "e"); err == nil || err.Error() != refusal || r.refused.Load() != 2 || s.AllocationCount() != 32 {
		t.Fatalf("a 33rd allocation from one IP: %v, %d refused, %d allocations", err, r.refused.Load(), s.AllocationCount())
	}
}

// A place taken for an Allocate that fails is given back when the client asks
// again from the same address, or after placeTTL. An allocation that took
// longer than that takes its place again.
func TestAllocationPlaces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := newAllocations()
		src := func(port int) net.Addr { return &net.UDPAddr{IP: net.IPv4(198, 51, 100, 1), Port: port} }
		for port := range maxDeviceAllocations {
			if !a.take("d", src(port)) {
				t.Fatalf("place %d", port)
			}
		}
		if a.take("d", src(100)) || !a.take("d", src(0)) {
			t.Fatal("the ninth place, or the same address asking again")
		}
		a.created("d", src(0))
		time.Sleep(placeTTL)
		if a.take("d", src(100)) {
			t.Fatal("a pending place expired before placeTTL")
		}
		time.Sleep(time.Nanosecond)
		if !a.take("d", src(100)) {
			t.Fatal("expired places were not given back")
		}
		if want := (map[string]int{"d": 2}); !reflect.DeepEqual(a.devices, want) || !reflect.DeepEqual(a.sources, map[string]int{"198.51.100.1": 2}) {
			t.Fatal(a.devices, a.sources)
		}
		time.Sleep(placeTTL + time.Nanosecond)
		if !a.take("e", src(200)) || !reflect.DeepEqual(a.devices, map[string]int{"d": 1, "e": 1}) {
			t.Fatal("the place of a slow Allocate did not expire", a.devices)
		}
		a.created("d", src(100)) // late: its place had expired
		a.created("e", src(200))
		a.deleted("d", src(0))
		if !reflect.DeepEqual(a.devices, map[string]int{"d": 1, "e": 1}) || len(a.pending) != 0 {
			t.Fatal(a.devices, a.pending)
		}
		a.deleted("d", src(100))
		a.deleted("e", src(200))
		if len(a.devices) != 0 || len(a.sources) != 0 {
			t.Fatal(a.devices, a.sources)
		}
	})
}

// A client transport address has permissions for at most 64 peer IPs.
// Refreshing one it has is always allowed, and a removed one frees its place.
func TestPermissionLimit(t *testing.T) {
	r, _, udp := loopbackRelay(t)
	c := turnClient(t, r, udp, "a")
	if _, err := c.Allocate(); err != nil {
		t.Fatal(err)
	}
	peer := func(i int) net.Addr { return &net.UDPAddr{IP: net.IPv4(127, 0, 1, byte(i)), Port: 9} }
	for i := range maxPeers {
		if err := c.CreatePermission(peer(i)); err != nil {
			t.Fatalf("peer %d: %v", i, err)
		}
	}
	if err := c.CreatePermission(peer(maxPeers)); err == nil || err.Error() != "CreatePermission error response (error 403: )" {
		t.Fatal(err)
	}
	if err := c.CreatePermission(peer(0)); err != nil {
		t.Fatal(err)
	}
	p := peers{by: map[string]map[string]bool{}}
	src := &net.UDPAddr{IP: net.IPv4(198, 51, 100, 1), Port: 1}
	for i := range maxPeers {
		p.add(src, net.IPv4(203, 0, 113, byte(i)))
	}
	p.remove(src, net.IPv4(203, 0, 113, 0))
	if !p.allow(src, net.IPv4(203, 0, 113, 200)) || len(p.by[src.String()]) != maxPeers-1 {
		t.Fatal("a removed permission kept its place")
	}
	for i := 1; i < maxPeers; i++ {
		p.remove(src, net.IPv4(203, 0, 113, byte(i)))
	}
	if len(p.by) != 0 {
		t.Fatal(p.by)
	}
}

// A datagram to one of the server's own addresses goes only to one of its
// relay allocations. A relay bound to loopback, used in tests, has no own
// addresses.
func TestOwnAddresses(t *testing.T) {
	defer func(f func() ([]net.Addr, error)) { interfaceAddrs = f }(interfaceAddrs)
	interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.IPv4(127, 0, 0, 1), Mask: net.CIDRMask(8, 32)}, &net.IPNet{IP: net.ParseIP("2001:db8::7"), Mask: net.CIDRMask(64, 128)}}, nil
	}
	own, err := ownAddrs(net.IPv4zero, []net.IP{net.ParseIP("::ffff:192.0.2.10")})
	want := map[netip.Addr]bool{netip.MustParseAddr("127.0.0.1"): true, netip.MustParseAddr("2001:db8::7"): true, netip.MustParseAddr("192.0.2.10"): true}
	if err != nil || !reflect.DeepEqual(own, want) {
		t.Fatal(own, err)
	}
	if own, err := ownAddrs(net.IPv4(127, 0, 0, 1), nil); err != nil || len(own) != 0 {
		t.Fatal(own, err)
	}
	failure := errors.New("no interfaces")
	interfaceAddrs = func() ([]net.Addr, error) { return nil, failure }
	if own, err := ownAddrs(net.IPv4zero, nil); own != nil || err != failure {
		t.Fatal(own, err)
	}
	if g, err := (&relay{}).generator("localhost", net.IPv4zero); g != nil || err != failure {
		t.Fatal(g, err)
	}
	// An allocation on 127.0.0.1 of a relay that counts 127.0.0.1 as its own.
	l := ledger{limit: 1000}
	g := testGenerator(&l, net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 1))
	g.own = want
	relayed, _, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp4", UserID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := g.AllocatePacketConn(turn.AllocateListenerConfig{Network: "udp4", UserID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	defer relayed.Close()
	peer := udpLocal(t)
	relayed.WriteTo([]byte("to a service"), peer.LocalAddr())
	relayed.WriteTo([]byte("to a relay"), other.LocalAddr())
	other.SetReadDeadline(time.Now().Add(time.Second))
	var b [32]byte
	if n, _, err := other.(*allocation).PacketConn.ReadFrom(b[:]); err != nil || string(b[:n]) != "to a relay" {
		t.Fatal(n, err)
	}
	peer.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if n, _, err := peer.ReadFrom(b[:]); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("a datagram to the server's own service: %q", b[:n])
	}
	if got := l.quota("d").Used; got != 50 {
		t.Fatal(got)
	}
	port := other.LocalAddr().(*net.UDPAddr).Port
	other.Close()
	if g.toSelf(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}) != true || g.toSelf(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 11), Port: port}) {
		t.Fatal("a closed allocation's port, or another host")
	}
}

// Pion prints nothing for refused requests; refused Allocates are counted.
func TestQuietRelay(t *testing.T) {
	stdout, stderr := os.Stdout, os.Stderr
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = write, write
	r, _, udp := loopbackRelay(t)
	user, pass, _ := turn.GenerateLongTermTURNRESTCredentials(r.secret, "d", -time.Hour)
	c, err := turn.NewClient(&turn.ClientConfig{Conn: udpLocal(t), TURNServerAddr: udp.LocalAddr().String(), Username: user, Password: pass})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Listen()
	_, refusal := c.Allocate()
	os.Stdout, os.Stderr = stdout, stderr
	write.Close()
	printed, _ := io.ReadAll(read)
	if refusal == nil || len(printed) != 0 || r.refused.Load() != 1 {
		t.Fatalf("refusal %v, %d refused, printed %q", refusal, r.refused.Load(), printed)
	}
}

// The quota is kept by alias; a device asks for its own on signalling.
func TestQuotaByAlias(t *testing.T) {
	r := &relay{secret: "secret", ledger: ledger{limit: 100}}
	k := key(1)
	r.admit(r.alias(k.ID()), 10)
	srv := fixtureWith(t, r)
	c := login(t, srv, k)
	send(t, c, proto.RVFrame{Type: "quota"})
	expect(t, c, proto.Quota{Type: "quota", Used: 50, Limit: 100})
	if alias := r.alias(k.ID()); len(alias) != 22 || strings.Contains(alias, k.ID()) || alias == r.alias(key(2).ID()) {
		t.Fatal(alias)
	}
}

// blockingConn holds a WriteTo until it is released.
type blockingConn struct {
	net.PacketConn
	entered, release chan struct{}
}

func (c blockingConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	close(c.entered)
	<-c.release
	return len(b), nil
}

// A relayed send does not hold the ledger's lock: another device's quota is
// read while it blocks.
func TestSendOutsideTheLedgerLock(t *testing.T) {
	l := ledger{limit: 1000}
	g := testGenerator(&l, net.IPv4(127, 0, 0, 1))
	conn := blockingConn{entered: make(chan struct{}), release: make(chan struct{})}
	a := &allocation{PacketConn: conn, g: g, addr: &net.UDPAddr{}, device: "d"}
	go a.WriteTo([]byte("x"), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 9})
	<-conn.entered
	read := make(chan proto.Quota)
	go func() { read <- l.quota("d") }()
	select {
	case q := <-read:
		if q.Used != 41 {
			t.Fatal(q)
		}
	case <-time.After(time.Second):
		t.Fatal("the quota is locked during a send")
	}
	close(conn.release)
}

func fixtureWith(t *testing.T, r *relay) string {
	t.Helper()
	s := httptest.NewServer(handler(fstest.MapFS{}, time.Second, r))
	t.Cleanup(s.Close)
	return s.URL
}
