package rendezvous

import (
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

// packetCost is what a relayed packet costs beside its payload: about its IP
// and UDP headers, so that tiny packets are not nearly free.
const packetCost = 40

func (l *ledger) admit(id string, n int) bool {
	_, ok := l.reserve(id, n)
	return ok
}

// reserve charges a packet of n bytes to a device for today. It refuses a
// packet that does not fit, an empty one included once the quota is spent.
func (l *ledger) reserve(id string, n int) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	today, cost := l.today(), int64(n)+packetCost
	if cost > l.limit-today[id] {
		return time.Time{}, false
	}
	today[id] += cost
	return l.day, true
}

// refund gives back the charge of a packet that was not sent, unless a new
// day has started since.
func (l *ledger) refund(id string, n int, day time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if day.Equal(l.day) {
		l.used[id] -= int64(n) + packetCost
	}
}
