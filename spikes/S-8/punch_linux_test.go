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
	relay               bool // TURN offered to both ends (a node's ordinary connection)
	mapOffer, mapAnswer bool // that end maps its host candidate's port by NAT-PMP
	occupied            bool // another mapping holds 47800/UDP on the right gateway
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
	env := []string{"S8_TURN=0"}
	if v.relay {
		env[0] = "S8_TURN=1"
	}
	if v.mapAnswer || v.occupied {
		n.Right.StartUPnP(t)
	}
	if v.mapOffer {
		n.Left.StartUPnP(t)
	}
	if v.occupied {
		command(t, "ip", "netns", "exec", n.Right.Client, "upnpc", "-m", netsim.ClientIP, "-a", netsim.ClientIP, "47899", strconv.Itoa(answerPort), "UDP")
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
		if side == "answer" {
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
	runs map[[2]netsim.NAT]outcome
}

func (x *matrix) put(l, r netsim.NAT, o outcome) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.runs[[2]netsim.NAT{l, r}] = o
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
			o := x.runs[[2]netsim.NAT{l, r}]
			cell := o.path()
			if o.open {
				cell += fmt.Sprintf(" %d ms", o.ms)
			}
			b.WriteString(" " + cell + " |")
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

// Question 2: every pair of NET-1's NAT types, four ways.
func TestPunch(t *testing.T) {
	variants := []variant{
		{name: "direct"},
		{name: "relay", relay: true},
		{name: "answerer-mapped", relay: true, mapAnswer: true},
		{name: "offerer-mapped", relay: true, mapOffer: true},
	}
	for _, v := range variants {
		x := &matrix{runs: map[[2]netsim.NAT]outcome{}}
		t.Run(v.name, func(t *testing.T) {
			for _, l := range nats {
				for _, r := range nats {
					t.Run(string(l)+"/"+string(r), func(t *testing.T) {
						t.Parallel()
						o := punch(t, l, r, v)
						x.put(l, r, o)
						t.Logf("%s offerer %s answerer %s: path %s at %d ms; offerer %s; answerer %s; %s%s",
							v.name, l, r, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
						if want := expected(v, l, r); o.path() != want {
							t.Errorf("path %s, want %s", o.path(), want)
						}
					})
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

// The contract's open point (rendezvous.md 5.1, point 8): the router grants
// another port than the one asked for, and the node publishes the granted one.
func TestGrantedPortPublished(t *testing.T) {
	for _, l := range []netsim.NAT{netsim.PortRestricted, netsim.Symmetric} {
		t.Run(string(l)+"/symmetric", func(t *testing.T) {
			o := punch(t, l, netsim.Symmetric, variant{relay: true, mapAnswer: true, occupied: true})
			t.Logf("offerer %s answerer symmetric, 47800 occupied: path %s at %d ms; offerer %s; answerer %s; %s%s",
				l, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
			if o.path() != "direct" || strings.HasSuffix(o.granted, ":47800") || !strings.HasSuffix(o.offer, "srflx "+o.granted) {
				t.Errorf("want a direct path through the granted port")
			}
		})
	}
}
