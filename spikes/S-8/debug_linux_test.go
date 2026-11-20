//go:build netsim

package s8

import (
	"os/exec"
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

// One run of the answerer-mapped matrix, a restricted-cone offerer and a
// port-restricted answerer, took the relay. This repeats that pair with the
// ICE trace and the gateways' state, with and without TURN.
func TestMappedRestrictedToPortRestricted(t *testing.T) {
	for _, relay := range []bool{true, false} {
		for i := range 3 {
			name := map[bool]string{true: "relay", false: "no-relay"}[relay]
			t.Run(name+"/"+string(rune('0'+i)), func(t *testing.T) {
				o := punch(t, netsim.Restricted, netsim.PortRestricted, variant{relay: relay, mapAnswer: true, debug: true})
				t.Logf("restricted -> port-restricted, answerer mapped, TURN %v: path %s at %d ms; offerer %s; answerer %s; %s%s",
					relay, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
			})
		}
	}
}
