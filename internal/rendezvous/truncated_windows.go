//go:build windows

package rendezvous

import (
	"errors"
	"net"

	"golang.org/x/sys/windows"
)

// readDatagram reads one datagram. Windows fails the read of one longer than
// b with WSAEMSGSIZE, without a source address; it is skipped, as other
// systems skip a datagram they cut to fill b.
func readDatagram(c net.PacketConn, b []byte) (int, net.Addr, error) {
	for {
		n, src, err := c.ReadFrom(b)
		if !errors.Is(err, windows.WSAEMSGSIZE) {
			return n, src, err
		}
	}
}
