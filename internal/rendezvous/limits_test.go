package rendezvous

import (
	"errors"
	"fmt"
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

// A device has at most 32 allocations and a source at most 128. A closed
// allocation gives its place back.
func TestAllocationLimits(t *testing.T) {
	r, s, udp := loopbackRelay(t)
	const refusal = "Allocate error response (error 486: )"
	var first net.PacketConn
	for i := range maxDeviceAllocations {
		a, err := allocate(t, r, udp, "a")
		if err != nil {
			t.Fatalf("allocation %d: %v", i, err)
		}
		first = a
	}
	if _, err := allocate(t, r, udp, "a"); err == nil || err.Error() != refusal || r.refused.Load() != 1 {
		t.Fatalf("a 33rd allocation of one device: %v, %d refused", err, r.refused.Load())
	}
	first.Close()
	waitAllocations(t, s, maxDeviceAllocations-1)
	devices := []string{"a"}
	for _, d := range []string{"b", "c", "d"} {
		for range maxDeviceAllocations {
			devices = append(devices, d)
		}
	}
	for _, device := range devices {
		if _, err := allocate(t, r, udp, device); err != nil {
			t.Fatalf("%s: %v", device, err)
		}
	}
	if _, err := allocate(t, r, udp, "e"); err == nil || err.Error() != refusal || r.refused.Load() != 2 || s.AllocationCount() != maxSourceAllocations {
		t.Fatalf("a 129th allocation from one IP: %v, %d refused, %d allocations", err, r.refused.Load(), s.AllocationCount())
	}
}

// The per-source limits count an IPv4 address, or an IPv6 /64.
func TestSources(t *testing.T) {
	for _, tc := range []struct{ addr, source string }{
		{"192.0.2.1:9", "192.0.2.1"}, {"[::ffff:192.0.2.1]:9", "192.0.2.1"},
		{"[2001:db8:1:2:3:4:5:6]:9", "2001:db8:1:2::/64"}, {"[2001:db8:1:2::ffff]:9", "2001:db8:1:2::/64"}, {"[2001:db8:1:3::1]:9", "2001:db8:1:3::/64"},
	} {
		addr, _ := net.ResolveUDPAddr("udp", tc.addr)
		if got := source(addr); got != tc.source {
			t.Errorf("%s: %s", tc.addr, got)
		}
	}
	a := newAllocations()
	for i := range maxSourceAllocations {
		src := &net.UDPAddr{IP: net.ParseIP(fmt.Sprintf("2001:db8:0:1::%x", i+1)), Port: 9}
		if !a.take(fmt.Sprint(i%5), src) {
			t.Fatalf("place %d", i)
		}
	}
	if a.take("other", &net.UDPAddr{IP: net.ParseIP("2001:db8:0:1:ffff::1"), Port: 9}) || !a.take("other", &net.UDPAddr{IP: net.ParseIP("2001:db8:0:2::1"), Port: 9}) {
		t.Fatal("the /64 is not one source")
	}
	counts := map[string]int{}
	decrement(counts, "never counted")
	if len(counts) != 0 {
		t.Fatal(counts)
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

func TestLateAllocationCountsTowardSource(t *testing.T) {
	a := newAllocations()
	src := &net.UDPAddr{IP: net.IPv4(198, 51, 100, 1), Port: 9}
	a.created("d", src)
	if got := a.sources["198.51.100.1"]; got != 1 {
		t.Fatalf("source count = %d; want 1", got)
	}
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
	if g, err := (&relay{}).generator("localhost", net.IPv4zero, DefaultRelayPorts); g != nil || err != failure {
		t.Fatal(g, err)
	}
	// An allocation on 127.0.0.1 of a relay that counts 127.0.0.1 as its own.
	l := ledger{limit: 1000}
	g := testGenerator(&l, net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 1))
	g.own.Store(&ownSet{want, time.Now()})
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

// freeRange finds two free consecutive UDP ports on 127.0.0.1.
func freeRange(t *testing.T) PortRange {
	t.Helper()
	for range 100 {
		c, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := c.LocalAddr().(*net.UDPAddr).Port
		next, err := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", port+1))
		c.Close()
		if err == nil {
			next.Close()
			return PortRange{port, port + 1}
		}
	}
	t.Fatal("no two free ports")
	return PortRange{}
}

// Relay sockets bind ports of the relay range only, which caps them; a
// closed one frees its port.
func TestRelayPortRange(t *testing.T) {
	ports := freeRange(t)
	g := testGenerator(&ledger{}, net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 1))
	g.portRange = ports
	cfg := turn.AllocateListenerConfig{Network: "udp4", UserID: "d"}
	first, _, err := g.AllocatePacketConn(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := g.AllocatePacketConn(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if a, b := first.LocalAddr().(*net.UDPAddr).Port, second.LocalAddr().(*net.UDPAddr).Port; a != ports.First || b != ports.Last {
		t.Fatal(a, b)
	}
	if c, _, err := g.AllocatePacketConn(cfg); c != nil || err != errNoPort {
		t.Fatal(c, err)
	}
	first.Close()
	again, _, err := g.AllocatePacketConn(cfg)
	if err != nil || again.LocalAddr().(*net.UDPAddr).Port != ports.First {
		t.Fatal(again, err)
	}
	again.Close()
}

func TestRelaySkipsReservedPort(t *testing.T) {
	g := testGenerator(&ledger{}, net.IPv4(127, 0, 0, 1))
	g.portRange = freeRange(t)
	g.holdPort(portKey{false, g.portRange.First})
	c, err := g.listen("udp4")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.LocalAddr().(*net.UDPAddr).Port; got != g.portRange.Last {
		t.Fatalf("bound port %d; want %d", got, g.portRange.Last)
	}
}

// A relay socket of the other family than the bind address fails at once,
// with the reason, instead of trying every port of the range.
func TestRelayPortFamily(t *testing.T) {
	g := testGenerator(&ledger{}, net.IPv4(127, 0, 0, 1))
	g.portRange = DefaultRelayPorts
	c, err := g.listen("udp6")
	var addrErr *net.AddrError
	if c != nil || !errors.As(err, &addrErr) {
		t.Fatal(c, err)
	}
}

// A relay port is named by family and port, and counted: an IPv6 socket on
// the number of a live IPv4 allocation does not end that allocation's role.
func TestRelayPortsByFamily(t *testing.T) {
	g := testGenerator(&ledger{}, net.IPv6unspecified)
	g.own.Store(&ownSet{map[netip.Addr]bool{netip.MustParseAddr("192.0.2.10"): true, netip.MustParseAddr("2001:db8::10"): true}, time.Now()})
	v4 := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 20000}
	v6 := &net.UDPAddr{IP: net.ParseIP("2001:db8::10"), Port: 20000}
	g.holdPort(portKey{false, 20000})
	g.holdPort(portKey{true, 20000})
	g.freePort(portKey{true, 20000})
	if g.toSelf(v4) || !g.toSelf(v6) {
		t.Fatal("the IPv6 socket's close ended the IPv4 allocation's port")
	}
	g.holdPort(portKey{false, 20000})
	g.freePort(portKey{false, 20000})
	if g.toSelf(v4) {
		t.Fatal("a port held twice was freed by one close")
	}
	g.freePort(portKey{false, 20000})
	if !g.toSelf(v4) || len(g.ports) != 0 {
		t.Fatal(g.ports)
	}
}

// The server reads its own addresses again after a minute; a failed read
// keeps those it has.
func TestOwnAddressRefresh(t *testing.T) {
	defer func(f func() ([]net.Addr, error)) { interfaceAddrs = f }(interfaceAddrs)
	synctest.Test(t, func(t *testing.T) {
		old, fresh := netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")
		g := testGenerator(&ledger{}, net.IPv4zero)
		g.own.Store(&ownSet{map[netip.Addr]bool{old: true}, time.Now()})
		addrs := []net.Addr{&net.IPNet{IP: fresh.AsSlice(), Mask: net.CIDRMask(24, 32)}}
		var failure error
		interfaceAddrs = func() ([]net.Addr, error) { return addrs, failure }
		time.Sleep(ownTTL - time.Nanosecond)
		if !g.ownAddr(old) || g.ownAddr(fresh) {
			t.Fatal("read again before a minute")
		}
		time.Sleep(time.Nanosecond)
		if g.ownAddr(old) || !g.ownAddr(fresh) {
			t.Fatal("not read again after a minute")
		}
		failure = errors.New("no interfaces")
		time.Sleep(ownTTL)
		if !g.ownAddr(fresh) {
			t.Fatal("a failed read lost the addresses")
		}
	})
}

// The count of refused Allocates is logged once a minute when it changed.
func TestRefusedReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var r relay
		lines := make(chan string, 4)
		logf := func(format string, args ...any) { lines <- fmt.Sprintf(format, args...) }
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() { r.report(stop, logf); close(done) }()
		r.refused.Add(2)
		time.Sleep(time.Minute - time.Nanosecond)
		synctest.Wait()
		if len(lines) != 0 {
			t.Fatal(<-lines)
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		time.Sleep(time.Minute) // unchanged
		synctest.Wait()
		r.refused.Add(1)
		time.Sleep(time.Minute)
		synctest.Wait()
		close(stop)
		<-done
		close(lines)
		var got []string
		for line := range lines {
			got = append(got, line)
		}
		if !reflect.DeepEqual(got, []string{"relay: 2 allocations refused since start", "relay: 3 allocations refused since start"}) {
			t.Fatal(got)
		}
	})
}

// --relay-ports takes first-last, two ports in order; an invalid range in
// Options means the default one.
func TestPortRange(t *testing.T) {
	var r PortRange
	for _, bad := range []string{"", "20000", "20000-", "-29999", "0-10", "30000-20000", "20000-65536", "a-b"} {
		if err := r.Set(bad); err != errPortRange || r != (PortRange{}) {
			t.Fatalf("%q: %v, %v", bad, err, r)
		}
	}
	if err := r.Set("1-65535"); err != nil || r.String() != "1-65535" {
		t.Fatal(err, r)
	}
	if got := (Options{}).relayPorts(); got != DefaultRelayPorts || DefaultRelayPorts.String() != "20000-29999" {
		t.Fatal(got)
	}
	if got := (Options{RelayPorts: PortRange{5, 5}}).relayPorts(); got != (PortRange{5, 5}) {
		t.Fatal(got)
	}
}
