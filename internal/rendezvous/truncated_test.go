package rendezvous

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

// datagramRead is one answer of scriptedConn.ReadFrom.
type datagramRead struct {
	data string
	src  net.Addr
	err  error
}

// scriptedConn answers its reads from a script, one entry per read.
type scriptedConn struct {
	net.PacketConn
	script []datagramRead
	reads  int
}

func (c *scriptedConn) ReadFrom(b []byte) (int, net.Addr, error) {
	r := c.script[c.reads]
	c.reads++
	return copy(b, r.data), r.src, r.err
}

var errCut = errors.New("message too long")

// Reads that fail with the skipped error, wrapped or not, are dropped with
// their bytes; the first other read is returned as it is.
func TestReadSkippingDropsSkippedReads(t *testing.T) {
	src := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 9}
	c := &scriptedConn{script: []datagramRead{
		{data: "x", err: errCut},
		{data: "yy", err: fmt.Errorf("read: %w", errCut)},
		{data: "hi", src: src},
	}}
	b := make([]byte, 16)
	n, from, err := readSkipping(c, b, errCut)
	if err != nil || n != 2 || from != src || string(b[:n]) != "hi" || c.reads != 3 {
		t.Fatal(n, from, err, c.reads)
	}
}

// Another error ends the read with its count and source, and nothing more
// is read.
func TestReadSkippingReturnsOtherErrors(t *testing.T) {
	src := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 3), Port: 7}
	closed := errors.New("use of closed network connection")
	c := &scriptedConn{script: []datagramRead{{data: "abc", src: src, err: closed}}}
	b := make([]byte, 16)
	if n, from, err := readSkipping(c, b, errCut); err != closed || n != 3 || from != src || c.reads != 1 {
		t.Fatal(n, from, err, c.reads)
	}
}
