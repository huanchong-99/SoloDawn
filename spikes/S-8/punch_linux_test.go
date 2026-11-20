//go:build netsim

package s8

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/features/netsim"
)

var nats = []netsim.NAT{netsim.FullCone, netsim.Restricted, netsim.PortRestricted, netsim.Symmetric}

// variant is one way of opening the connection. The offerer is always
// behind the left gateway and the answerer behind the right.
type variant struct {
	name                string
	relay               bool   // TURN offered to both ends (a node's ordinary connection)
	mapOffer, mapAnswer bool   // that end maps its host candidate's port by NAT-PMP
	occupied            bool   // another mapping holds 47800/UDP on the right gateway
	tcp, mapTCP         bool   // both ends listen for ICE-TCP; the answerer maps that port
	tcpType             string // how the mapped ICE-TCP endpoint is published: srflx or host
	blockUDP            bool   // NET-1 drops UDP between the two gateways
	runs                int    // repetitions of each pair (1 if zero)
	debug               bool   // ICE trace and the gateways' conntrack tables
}

// outcome is what one run saw within the deadline.
type outcome struct {
	open          bool   // ctl echoed
	ms            int64  // offerer's clock at the echo
	offer, answer string // each side's selected pair, "local -> remote"
	mapped        string
	granted       string // the answerer's granted external endpoint
	failure       string
}

func (o outcome) path() string {
	switch {
	case !o.open:
		return "none"
	case strings.Contains(o.offer, "relay") || strings.Contains(o.answer, "relay"):
		return "relay"
	}
	return "direct"
}

const deadline = 15 * time.Second

// punch opens one connection in a fresh network and reports what happened.
func punch(t *testing.T, left, right netsim.NAT, v variant) outcome {
	n := netsim.Start(t, left, right)
	env := []string{"S8_TURN=" + flag(v.relay), "S8_TCP=" + flag(v.tcp), "S8_MAP_TCP=" + on(v.mapTCP), "S8_TCP_TYP=" + v.tcpType}
	if v.blockUDP {
		n.SetBlocked(t, true, 0)
	}
	if v.mapAnswer || v.occupied || v.mapTCP {
		n.Right.StartUPnP(t)
	}
	if v.mapOffer {
		n.Left.StartUPnP(t)
	}
	if v.occupied {
		occupy(t, n.Right.Client)
	}
	if v.debug {
		env = append(env, "S8_DEBUG=1")
		t.Cleanup(func() { dump(t, n) }) // after the processes stop, before the namespaces go
	}
	relay := spawn(t, n.WAN, "relay")
	if m, ok := relay.next(t, 5*time.Second); !ok || m.T != "ready" {
		t.Fatalf("relay not ready: %+v", m)
	}
	ans := spawn(t, n.Right.Client, "peer", append(env, "S8_SIDE=answer", "S8_MAP="+on(v.mapAnswer))...)
	off := spawn(t, n.Left.Client, "peer", append(env, "S8_SIDE=offer", "S8_MAP="+on(v.mapOffer))...)
	var o outcome
	answered := false
	timer := time.After(deadline)
	for !o.open || !answered {
		select {
		case m, ok := <-off.out:
			if !ok {
				t.Fatal("offerer exited")
			}
			o.observe(t, "offer", m, ans)
		case m, ok := <-ans.out:
			if !ok {
				t.Fatal("answerer exited")
			}
			answered = answered || m.T == "pair"
			o.observe(t, "answer", m, off)
		case <-timer:
			if o.open {
				o.failure = "answerer reported no pair"
			}
			return o
		}
	}
	return o
}

func flag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func on(b bool) string {
	if b {
		return "pmp"
	}
	return ""
}

// observe forwards signalling to the other end and records the rest.
func (o *outcome) observe(t *testing.T, side string, m msg, other *child) {
	switch m.T {
	case "sdp", "cand":
		other.in.send(m)
	case "echo":
		o.open, o.ms, o.offer = true, m.Ms, m.Local+" -> "+m.Remote
	case "pair":
		o.answer = m.Local + " -> " + m.Remote
	case "mapped":
		o.mapped += fmt.Sprintf("%s %d->%s:%d (%d ms, err %q) ", side, m.Internal, m.IP, m.Port, m.Ms, m.Err)
		if side == "answer" && m.Proto != "tcp" {
			o.granted = net.JoinHostPort(m.IP, strconv.Itoa(m.Port))
		}
	case "error":
		t.Logf("%s: %s", side, m.Err)
	case "state":
		t.Logf("%s ICE %s at %d ms", side, m.Kind, m.Ms)
	}
}

// matrix holds the outcomes of one variant for the summary table.
type matrix struct {
	mu   sync.Mutex
	runs map[[2]netsim.NAT][]outcome
}

func (x *matrix) put(l, r netsim.NAT, o outcome) {
	x.mu.Lock()
	defer x.mu.Unlock()
	k := [2]netsim.NAT{l, r}
	x.runs[k] = append(x.runs[k], o)
}

// cell counts the paths of one pair, with the offerer's time to the echo.
func cell(runs []outcome) string {
	count := map[string]int{}
	lo, hi := int64(-1), int64(0)
	for _, o := range runs {
		count[o.path()]++
		if o.open && (lo < 0 || o.ms < lo) {
			lo = o.ms
		}
		hi = max(hi, o.ms)
	}
	var parts []string
	for _, p := range []string{"direct", "relay", "none"} {
		if count[p] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d/%d", p, count[p], len(runs)))
		}
	}
	switch {
	case lo == hi:
		parts = append(parts, fmt.Sprintf("%d ms", lo))
	case lo >= 0:
		parts = append(parts, fmt.Sprintf("%d-%d ms", lo, hi))
	}
	return strings.Join(parts, ", ")
}

func (x *matrix) table() string {
	var b strings.Builder
	b.WriteString("| offerer \\ answerer |")
	for _, r := range nats {
		b.WriteString(" " + string(r) + " |")
	}
	b.WriteString("\n| --- | --- | --- | --- | --- |\n")
	for _, l := range nats {
		b.WriteString("| " + string(l) + " |")
		for _, r := range nats {
			b.WriteString(" " + cell(x.runs[[2]netsim.NAT{l, r}]) + " |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// relayOnly are the pairs that NET-1's NATs leave without a direct path:
// symmetric against port-restricted or symmetric.
func relayOnly(l, r netsim.NAT) bool {
	hard := func(n netsim.NAT) bool { return n == netsim.PortRestricted || n == netsim.Symmetric }
	return (l == netsim.Symmetric && hard(r)) || (r == netsim.Symmetric && hard(l))
}

// Question 2: every pair of NET-1's NAT types, four ways; the variants that
// can only connect run each pair three times.
func TestPunch(t *testing.T) {
	variants := []variant{
		{name: "direct", runs: 1},
		{name: "relay", relay: true, runs: 3},
		{name: "answerer-mapped", relay: true, mapAnswer: true, runs: 3},
		{name: "offerer-mapped", relay: true, mapOffer: true, runs: 3},
	}
	for _, v := range variants {
		x := &matrix{runs: map[[2]netsim.NAT][]outcome{}}
		t.Run(v.name, func(t *testing.T) {
			for _, l := range nats {
				for _, r := range nats {
					for i := range v.runs {
						t.Run(fmt.Sprintf("%s/%s/%d", l, r, i), func(t *testing.T) {
							t.Parallel()
							o := punch(t, l, r, v)
							x.put(l, r, o)
							t.Logf("%s offerer %s answerer %s: path %s at %d ms; offerer %s; answerer %s; %s%s",
								v.name, l, r, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
							switch want := expected(v, l, r); {
							case o.path() == want:
							case raced(v, l, r, o):
								t.Logf("outlier: relay although a direct pair was possible (pion prflx race, results.md)")
							default:
								t.Errorf("path %s, want %s", o.path(), want)
							}
						})
					}
				}
			}
		})
		t.Logf("%s:\n%s", v.name, x.table())
	}
}

func expected(v variant, l, r netsim.NAT) string {
	switch {
	case !relayOnly(l, r) || v.mapOffer || v.mapAnswer:
		return "direct"
	case v.relay:
		return "relay"
	}
	return "none"
}

// raced reports the one known outlier: with TURN offered, a pair that has a
// direct path selects the relay because the remote relay candidate was first
// learned as peer-reflexive and kept that priority (pion ice v4.4.8,
// agent.go:1247-1262 and selection.go:74-84). The pairs that only a mapping
// makes direct must stay direct.
func raced(v variant, l, r netsim.NAT, o outcome) bool {
	return v.relay && !relayOnly(l, r) && o.path() == "relay"
}

// The contract's open point (rendezvous.md 5.1, point 8): the router grants
// another port than the one asked for, and the node publishes the granted one.
func TestGrantedPortPublished(t *testing.T) {
	for _, l := range []netsim.NAT{netsim.PortRestricted, netsim.Symmetric} {
		t.Run(string(l)+"/symmetric", func(t *testing.T) {
			o := punch(t, l, netsim.Symmetric, variant{relay: true, mapAnswer: true, occupied: true})
			t.Logf("offerer %s answerer symmetric, 47800 occupied: path %s at %d ms; offerer %s; answerer %s; %s%s",
				l, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
			if o.path() != "direct" || strings.HasSuffix(o.granted, ":47800") || !strings.HasSuffix(o.offer, "srflx udp "+o.granted) {
				t.Errorf("want a direct path through the granted port")
			}
		})
	}
}
