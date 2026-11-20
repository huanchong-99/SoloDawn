//go:build netsim

package s8

import (
	"testing"
	"time"
)

// gone polls a mapped port until a datagram no longer arrives. Each failed
// probe waits 700 ms, so the result is late by up to about a second.
func gone(t *testing.T, r *router, port int, start time.Time, limit time.Duration) time.Duration {
	t.Helper()
	for time.Since(start) < limit {
		if !r.reach(t, port) {
			return time.Since(start).Round(100 * time.Millisecond)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return -1
}

// Lifetimes as the gateway enforces them, and renewal at half the lifetime.
func TestLease(t *testing.T) {
	r := startRouter(t)
	start := time.Now()
	p := r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 4})
	pmpGone := gone(t, r, p.Port, start, 15*time.Second)
	t.Logf("NAT-PMP lifetime 4 s (granted %d s, err %q): unreachable %s after the request", p.Lifetime, p.Err, pmpGone)

	p = r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 4})
	ports := []int{p.Port}
	for range 5 {
		time.Sleep(2 * time.Second)
		ports = append(ports, r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, External: p.Port, Lifetime: 4}).Port)
	}
	renewed := r.reach(t, p.Port)
	r.call(t, msg{T: "pmp", Proto: "udp", Internal: answerPort, Lifetime: 0})
	t.Logf("NAT-PMP lifetime 4 s renewed every 2 s for 10 s: ports %v, reachable at the end %v", ports, renewed)

	r.call(t, msg{T: "igd", Kind: "discover", Ms: 8000})
	start = time.Now()
	a := r.call(t, msg{T: "igd", Kind: "any", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 4})
	entry := r.call(t, msg{T: "igd", Kind: "get", Proto: "udp", External: a.Port})
	igdGone := gone(t, r, a.Port, start, 15*time.Second)
	t.Logf("IGDv2 lease 4 s on %d (err %q, entry says %d s left): unreachable %s after the request", a.Port, a.Err, entry.Lifetime, igdGone)

	if pmpGone < 3*time.Second || pmpGone > 7*time.Second || !renewed || igdGone < 3*time.Second || igdGone > 7*time.Second {
		t.Errorf("want expiry near 4 s for both protocols and a renewed mapping that stays")
	}
}

// go-nat's NAT-PMP calls ignore the context: when the gateway stops
// answering, AddPortMapping retries for about 128 s (go-nat-pmp NewClient).
func TestUnansweredGateway(t *testing.T) {
	r := startRouter(t)
	r.call(t, msg{T: "discover", Ms: 8000})
	first := r.call(t, msg{T: "map", NAT: "NAT-PMP", Proto: "udp", Internal: answerPort, Lifetime: 60})
	command(t, "ip", "netns", "exec", r.n.Right.Gateway, "nft", "insert", "rule", "ip", "netsim", "input",
		"iifname", `"lan"`, "udp", "dport", "5351", "drop")
	direct := spawn(t, r.n.Right.Client, "mapper").call(t,
		msg{T: "pmp", Proto: "udp", Internal: answerPort, External: answerPort, Lifetime: 60}, 20*time.Second)
	r.m.in.send(msg{T: "map", NAT: "NAT-PMP", Proto: "udp", Internal: answerPort, Lifetime: 60, Ms: 2000})
	_, replied := r.m.next(t, 10*time.Second)
	t.Logf("go-nat NAT-PMP mapped %d (err %q); gateway silenced; go-nat AddPortMapping with a 2 s context replied within 10 s: %v; go-nat-pmp with a 2 s timeout: err %q after %d ms",
		first.Port, first.Err, replied, direct.Err, direct.Ms)
	if replied || direct.Err == "" || direct.Ms > 5000 {
		t.Errorf("want go-nat blocked past its context and go-nat-pmp bounded by its timeout")
	}
}
