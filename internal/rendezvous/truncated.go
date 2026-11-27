package rendezvous

import (
	"errors"
	"net"
)

// readSkipping reads one datagram from c, skipping every read that fails
// with skip; any other result is returned as it is.
func readSkipping(c net.PacketConn, b []byte, skip error) (int, net.Addr, error) {
	for {
		n, src, err := c.ReadFrom(b)
		if !errors.Is(err, skip) {
			return n, src, err
		}
	}
}
