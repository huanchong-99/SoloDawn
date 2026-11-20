package rendezvous

import (
	"net"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Each UTC day gets a fresh table. An allocation outlives the table and
// looks up its device again for every datagram.
type ledger struct {
	mu    sync.Mutex
	day   time.Time
	used  map[string]int64
	limit int64
}

func (l *ledger) today() map[string]int64 {
	day := time.Now().UTC().Truncate(24 * time.Hour)
	if !day.Equal(l.day) {
		l.day = day
		l.used = make(map[string]int64)
	}
	return l.used
}

func (l *ledger) quota(id string) proto.Quota {
	l.mu.Lock()
	defer l.mu.Unlock()
	return proto.Quota{Type: "quota", Used: l.today()[id], Limit: l.limit}
}

func (l *ledger) admit(id string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	today := l.today()
	if int64(n) > l.limit-today[id] {
		return false
	}
	today[id] += int64(n)
	return true
}

// A UDP write either sends one whole datagram or fails. Only a successful
// send counts. No blocking client TCP/TLS I/O runs under the ledger lock.
func (l *ledger) write(id string, conn net.PacketConn, b []byte, dst net.Addr) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	today := l.today()
	if int64(len(b)) > l.limit-today[id] {
		return len(b), nil
	}
	n, err := conn.WriteTo(b, dst)
	today[id] += int64(n)
	return n, err
}
