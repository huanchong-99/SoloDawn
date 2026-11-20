package rendezvous

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

// The relay's resource limits (contracts/rendezvous.md section 3).
const (
	maxDeviceAllocations = 32
	maxSourceAllocations = 128
	maxPeers             = 64
)

// placeTTL is how long a place taken by an Allocate waits for its
// allocation. Pion creates it at once or fails; a failure gives the place
// back when the client asks again from the same address, or after this.
const placeTTL time.Duration = 5e9 // 5 s

// allocations counts the relay allocations of each device and of each
// source. The quota check of an Allocate takes a place before Pion creates the
// allocation, so concurrent Allocates cannot pass a limit together.
type allocations struct {
	mu      sync.Mutex
	devices map[string]int
	sources map[string]int
	pending map[string]place // by the client's transport address
}

type place struct {
	device, source string
	at             time.Time
}

func newAllocations() *allocations {
	return &allocations{devices: map[string]int{}, sources: map[string]int{}, pending: map[string]place{}}
}

// source is what the per-source limits count by: an IPv4 address, or the /64
// of an IPv6 address, which one host usually holds whole.
func source(addr net.Addr) string {
	ap, _ := netip.ParseAddrPort(addr.String()) // a transport address is ip:port
	ip := ap.Addr().Unmap()
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}

// take is Pion's quota check of an Allocate from src.
func (a *allocations) take(device string, src net.Addr) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	a.expire(src.String(), now)
	p := place{device, source(src), now}
	if a.devices[p.device] >= maxDeviceAllocations || a.sources[p.source] >= maxSourceAllocations {
		return false
	}
	a.devices[p.device]++
	a.sources[p.source]++
	a.pending[src.String()] = p
	return true
}

// expire gives back the places whose Allocate failed: the one of the address
// that asks again, and those older than placeTTL.
func (a *allocations) expire(again string, now time.Time) {
	for key, p := range a.pending {
		if key == again || now.Sub(p.at) > placeTTL {
			delete(a.pending, key)
			a.release(p.device, p.source)
		}
	}
}

func (a *allocations) release(device, source string) {
	decrement(a.devices, device)
	decrement(a.sources, source)
}

// decrement counts one less of a key, and never below zero.
func decrement[K comparable](counts map[K]int, key K) {
	if counts[key] > 1 {
		counts[key]--
		return
	}
	delete(counts, key)
}

// created keeps the place of a new allocation. One that took longer than
// placeTTL takes its place again.
func (a *allocations) created(device string, src net.Addr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.pending[src.String()]; ok {
		delete(a.pending, src.String())
		return
	}
	a.devices[device]++
	a.sources[source(src)]++
}

func (a *allocations) deleted(device string, src net.Addr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.release(device, source(src))
}

// peers holds the peer IPs that each client transport address has a
// permission for. A new peer is refused once there are maxPeers; refreshing
// a permission is always allowed.
type peers struct {
	mu sync.Mutex
	by map[string]map[string]bool
}

func (p *peers) allow(src net.Addr, peer net.IP) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := p.by[src.String()]
	return set[peer.String()] || len(set) < maxPeers
}

func (p *peers) add(src net.Addr, peer net.IP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := p.by[src.String()]
	if set == nil {
		set = map[string]bool{}
		p.by[src.String()] = set
	}
	set[peer.String()] = true
}

func (p *peers) remove(src net.Addr, peer net.IP) {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := p.by[src.String()]
	if delete(set, peer.String()); len(set) == 0 {
		delete(p.by, src.String())
	}
}
