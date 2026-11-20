//go:build netsim

package s8

import (
	"os/exec"
	"strconv"
	"testing"

	"github.com/huanchong-99/SoloDawn/features/netsim"
)

// dump logs what the two gateways hold after a run: their UDP conntrack
// entries, the cone filter sets and the right gateway's miniupnpd rules.
func dump(t *testing.T, n netsim.Network) {
	for _, gw := range []string{n.Left.Gateway, n.Right.Gateway} {
		for _, args := range [][]string{
			{"conntrack", "-L", "-p", "udp"},
			{"nft", "list", "set", "ip", "netsim", "allowed"},
			{"nft", "list", "chain", "ip", "netsim", "prerouting_miniupnpd"},
		} {
			out, err := exec.Command("ip", append([]string{"netns", "exec", gw}, args...)...).CombinedOutput()
			t.Logf("%s %v (err %v):\n%s", gw, args, err, out)
		}
	}
}

// The answerer-mapped matrix took the relay in some runs of a pair with a
// restricted-cone offerer. This repeats one such pair ten times in parallel
// with the ICE trace and the gateways' state; the trace of a relayed run
// shows the pion race that raced describes.
func TestMappedRestrictedToPortRestricted(t *testing.T) {
	for i := range 10 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			o := punch(t, netsim.Restricted, netsim.PortRestricted, variant{relay: true, mapAnswer: true, debug: true})
			t.Logf("restricted -> port-restricted, answerer mapped: path %s at %d ms; offerer %s; answerer %s; %s%s",
				o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
			if o.path() == "none" {
				t.Errorf("no connection")
			}
		})
	}
}
