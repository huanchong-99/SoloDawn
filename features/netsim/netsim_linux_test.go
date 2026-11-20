//go:build netsim

package netsim

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Only socket creation needs to run on the namespace's OS thread. Go can
// read and write those descriptors normally after the original ns is restored.
// If the restore fails, the thread stays locked and Go discards it.
func inNamespace(t *testing.T, name string, fn func()) {
	t.Helper()
	runtime.LockOSThread()
	previous, err := os.Open("/proc/thread-self/ns/net")
	must(t, err)
	defer previous.Close()
	next, err := os.Open("/var/run/netns/" + name)
	must(t, err)
	defer next.Close()
	must(t, unix.Setns(int(next.Fd()), unix.CLONE_NEWNET))
	defer func() {
		must(t, unix.Setns(int(previous.Fd()), unix.CLONE_NEWNET))
		runtime.UnlockOSThread()
	}()
	fn()
}

func socket(t *testing.T, ns, ip string, port int) *net.UDPConn {
	t.Helper()
	var conn *net.UDPConn
	inNamespace(t, ns, func() {
		var err error
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(ip), Port: port})
		must(t, err)
	})
	t.Cleanup(func() { conn.Close() })
	return conn
}

func address(c *net.UDPConn) netip.AddrPort {
	return c.LocalAddr().(*net.UDPAddr).AddrPort()
}

func packet(t *testing.T, label string, from, to *net.UDPConn, dst netip.AddrPort, want bool) netip.AddrPort {
	t.Helper()
	_, err := from.WriteToUDPAddrPort([]byte(label), dst)
	must(t, err)
	delay := 2 * time.Second
	if !want {
		delay = 200 * time.Millisecond
	}
	must(t, to.SetReadDeadline(time.Now().Add(delay)))
	var buf [128]byte
	n, src, err := to.ReadFromUDPAddrPort(buf[:])
	if !want {
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("%s: expected drop, got %q from %s, err %v", label, buf[:n], src, err)
		}
		return netip.AddrPort{}
	}
	must(t, err)
	if string(buf[:n]) != label {
		t.Fatalf("%s: got %q from %s", label, buf[:n], src)
	}
	return src
}

// removed runs a test on a Network and then checks that its namespaces are gone.
func removed(t *testing.T, name string, test func(t *testing.T) Network) {
	var n Network
	if !t.Run(name, func(t *testing.T) { n = test(t) }) {
		return
	}
	for _, ns := range []string{n.WAN, n.Left.Client, n.Left.Gateway, n.Right.Client, n.Right.Gateway} {
		if _, err := os.Stat("/var/run/netns/" + ns); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("namespace %s outlived its test: %v", ns, err)
		}
	}
}

// mapped checks the public endpoint of a client socket: cones keep the client
// port, symmetric NAT draws one from 40000 to 59999.
func mapped(t *testing.T, mode NAT, got netip.AddrPort, public string, port uint16) {
	t.Helper()
	ok := got.Addr().String() == public && got.Port() == port
	if mode == Symmetric {
		ok = got.Addr().String() == public && got.Port() >= 40000 && got.Port() < 60000
	}
	if !ok {
		t.Fatalf("%s: client port %d mapped to %s, public IP %s", mode, port, got, public)
	}
}

func TestNAT(t *testing.T) {
	for _, mode := range []NAT{FullCone, Restricted, PortRestricted, Symmetric} {
		removed(t, string(mode), func(t *testing.T) Network { return checkNAT(t, mode) })
	}
}

func checkNAT(t *testing.T, mode NAT) Network {
	n := Start(t, mode, mode)
	a := socket(t, n.Left.Client, ClientIP, 47800)
	b := socket(t, n.Right.Client, ClientIP, 47800)
	other := socket(t, n.Right.Client, ClientIP, 47802)
	wan := socket(t, n.WAN, WANIP, 3478)
	cold := socket(t, n.Left.Client, ClientIP, 47803)
	packet(t, "no outbound mapping", cold, b, netip.AddrPortFrom(netip.MustParseAddr(n.Right.PublicIP), 47800), false)

	mapAtWAN := func(c *net.UDPConn) netip.AddrPort {
		return packet(t, "discover", c, wan, address(wan), true)
	}
	left, right, otherPort := mapAtWAN(a), mapAtWAN(b), mapAtWAN(other)
	mapped(t, mode, left, n.Left.PublicIP, 47800)
	mapped(t, mode, right, n.Right.PublicIP, 47800)
	mapped(t, mode, otherPort, n.Right.PublicIP, 47802)
	packet(t, "reply from discovery endpoint", wan, a, left, true)
	packet(t, "uncontacted peer IP", b, a, left, mode == FullCone)
	packet(t, "contact other port", a, other, otherPort, mode == FullCone)
	packet(t, "contacted IP, uncontacted port", b, a, left, mode == FullCone || mode == Restricted)
	packet(t, "punch", a, b, right, mode != Symmetric)
	packet(t, "reverse punch", b, a, left, mode != Symmetric)

	checkMapping(t, mode, a, left, socket(t, n.WAN, WANIP, 3479))
	checkMapping(t, mode, a, left, socket(t, n.WAN, "11.0.0.5", 3478))
	if again := mapAtWAN(a); again != left {
		t.Fatalf("mapping changed on an existing flow: %s then %s", left, again)
	}
	return n
}

func checkMapping(t *testing.T, mode NAT, a *net.UDPConn, first netip.AddrPort, alt *net.UDPConn) {
	t.Helper()
	second := packet(t, "other destination", a, alt, address(alt), true)
	if (first != second) != (mode == Symmetric) {
		t.Fatalf("%s: mappings to two destinations: %s, %s", mode, first, second)
	}
	packet(t, "reply on other mapping", alt, a, second, true)
}

type failedCommand struct {
	*testing.T
	message string
}

func (f *failedCommand) Fatalf(format string, args ...any) {
	f.message = fmt.Sprintf(format, args...)
}

func TestCommandFailure(t *testing.T) {
	f := &failedCommand{T: t}
	run(f, "", "sh", "-c", "exit 17")
	if want := "netsim [sh -c exit 17]: exit status 17"; f.message != want {
		t.Fatalf("got %q, want %q", f.message, want)
	}
}

func TestBlockingUDP(t *testing.T) {
	n := Start(t, FullCone, FullCone)
	a := socket(t, n.Left.Client, ClientIP, 47800)
	b := socket(t, n.Right.Client, ClientIP, 47800)
	wan := socket(t, n.WAN, WANIP, 3478)
	right := packet(t, "discover", b, wan, address(wan), true)
	left := packet(t, "direct", a, b, right, true)
	n.SetBlocked(t, true, 47801)
	packet(t, "blocked", a, b, right, false)
	packet(t, "reverse blocked", b, a, left, false)
	packet(t, "TURN stays reachable", a, wan, address(wan), true)
	n.SetBlocked(t, false, 47801)
	packet(t, "direct restored", a, b, right, true)
}

func TestUPnPAndBlockingTCP(t *testing.T) {
	removed(t, "port-restricted", checkUPnP)
}

func checkUPnP(t *testing.T) Network {
	n := Start(t, PortRestricted, PortRestricted)
	n.Right.StartUPnP(t)
	for _, port := range []string{"47801", "7000"} {
		run(t, "", "ip", "netns", "exec", n.Right.Client, "upnpc", "-m", ClientIP,
			"-a", ClientIP, port, port, "TCP")
	}
	listener := func(port int) *net.TCPListener {
		var l *net.TCPListener
		inNamespace(t, n.Right.Client, func() {
			var err error
			l, err = net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP(ClientIP), Port: port})
			must(t, err)
		})
		t.Cleanup(func() { l.Close() })
		return l
	}
	ice, signal := listener(47801), listener(7000)
	connect := func(l *net.TCPListener, want bool) {
		t.Helper()
		inNamespace(t, n.Left.Client, func() {
			port := l.Addr().(*net.TCPAddr).Port
			c, err := net.DialTimeout("tcp4", fmt.Sprintf("%s:%d", n.Right.PublicIP, port), 300*time.Millisecond)
			if !want {
				if !os.IsTimeout(err) {
					t.Fatalf("expected ICE-TCP timeout, got %v", err)
				}
				return
			}
			must(t, err)
			defer c.Close()
			must(t, l.SetDeadline(time.Now().Add(time.Second)))
			conn, err := l.Accept()
			must(t, err)
			conn.Close()
		})
	}
	connect(ice, true)
	n.SetBlocked(t, false, 47801)
	connect(ice, false)
	connect(signal, true)
	n.SetBlocked(t, true, 0)
	connect(ice, true)
	return n
}
