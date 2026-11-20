//go:build !windows

package rendezvous

import "net"

// readDatagram reads one datagram. These systems cut a long one to fill b
// without an error; the caller drops a read that fills b.
func readDatagram(c net.PacketConn, b []byte) (int, net.Addr, error) { return c.ReadFrom(b) }
