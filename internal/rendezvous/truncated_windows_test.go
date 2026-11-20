//go:build windows

package rendezvous

import (
	"net"
	"testing"

	"golang.org/x/sys/windows"
)

// cutConn fails its first read as Windows does for a datagram longer than the
// buffer: some bytes, no source address, WSAEMSGSIZE. Then it reads a datagram.
type cutConn struct {
	net.PacketConn
	reads int
	src   net.Addr
}

func (c *cutConn) ReadFrom(b []byte) (int, net.Addr, error) {
	c.reads++
	if c.reads == 1 {
		return 1, nil, windows.WSAEMSGSIZE
	}
	return copy(b, "hi"), c.src, nil
}

// A cut datagram is skipped before its missing source is used: it used to
// end the process when its length fitted Pion's buffer.
func TestWindowsCutDatagram(t *testing.T) {
	src := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 9}
	g := testGenerator(&ledger{limit: 1000}, net.IPv4(127, 0, 0, 1), net.IPv4(127, 0, 0, 1))
	a := &allocation{PacketConn: &cutConn{src: src}, g: g, addr: &net.UDPAddr{}, device: "d"}
	g.events().OnPermissionCreated(src, nil, "", "", "", a.addr, src.IP)
	b := make([]byte, 1600)
	if n, from, err := a.ReadFrom(b); err != nil || n != 2 || from != src || string(b[:n]) != "hi" {
		t.Fatal(n, from, err)
	}
}
