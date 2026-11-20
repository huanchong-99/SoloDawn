package peer

import (
	"net"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/turn/v5"
)

// TURN is local to this test. Only the test credentials can allocate, and
// permissions name only this machine. No external STUN or TURN is configured.
func localRelay(t *testing.T, ip net.IP) proto.ICEServer {
	t.Helper()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	check(t, err)
	password := randomID()
	s, err := turn.NewServer(turn.ServerConfig{Realm: "test", AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "test", password), r.Username == "peer" && r.Realm == "test"
	}, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: l, PermissionHandler: func(_ net.Addr, peer net.IP) bool { return peer.Equal(ip) }, RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()}}}})
	check(t, err)
	t.Cleanup(func() { s.Close() })
	return proto.ICEServer{URLs: []string{"turn:" + l.LocalAddr().String() + "?transport=udp"}, Username: "peer", Credential: password}
}
func TestLocalRelayPath(t *testing.T) {
	f := newPair(t)
	ip := net.IPv4(127, 0, 0, 2)
	server := localRelay(t, ip)
	f.b.paths = func(string) []Route { r := f.bMail.route(); r.ICE = []proto.ICEServer{server}; return []Route{r} }
	f.a.paths = func(string) []Route { r := f.aMail.route(); r.ICE = []proto.ICEServer{server}; return []Route{r} }
	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	offer := recv(t, f.aMail["offer"])
	deliver(t, f.b, f.bMail.route(), offer)
	answer := recv(t, f.bMail["answer"])
	deliver(t, f.a, f.aMail.route(), answer)
	// Do not exchange direct endpoints. The only reachable remote candidate
	// given to the offerer is the answerer's local TURN allocation.
	relay := func(m mailbox) proto.Envelope {
		for {
			i := recv(t, m["ice"])
			var cand proto.ICECandidate
			check(t, proto.Decode([]byte(i.Payload), &cand))
			if strings.Contains(cand.Candidate, " typ relay") {
				return i
			}
		}
	}
	deliver(t, f.a, f.aMail.route(), relay(f.bMail))
	deliver(t, f.b, f.bMail.route(), relay(f.aMail))
	recv(t, c.Ready())
	b := recv(t, f.opened)
	t.Cleanup(func() { b.Close() })
	for _, end := range []*Connection{b, c} {
		p, err := end.Path()
		check(t, err)
		eq(t, p, "relay")
	}
	negotiation{c, b, offer, answer}.info(t)
}
