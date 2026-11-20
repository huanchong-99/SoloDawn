package peer

import (
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/turn/v5"
)

// relayOnly allocates relay sockets on 127.0.0.1 that take datagrams only
// from other relay sockets. With UDP and ICE-TCP between the two ends
// blocked, the relay is the only path, although every socket is local.
type relayOnly struct {
	turn.RelayAddressGeneratorStatic
	mu    sync.Mutex
	ports map[int]bool
}

func (g *relayOnly) AllocatePacketConn(conf turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	conn, addr, err := g.RelayAddressGeneratorStatic.AllocatePacketConn(conf)
	if err != nil {
		return nil, nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ports[addr.(*net.UDPAddr).Port] = true
	return relayConn{conn, g}, addr, nil
}
func (g *relayOnly) relayed(addr net.Addr) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ports[addr.(*net.UDPAddr).Port]
}

type relayConn struct {
	net.PacketConn
	g *relayOnly
}

func (c relayConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || c.g.relayed(addr) {
			return n, addr, err
		}
	}
}

// localRelay is a TURN server on 127.0.0.1 for this test only.
func localRelay(t *testing.T) proto.ICEServer {
	t.Helper()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	check(t, err)
	password := randomID()
	gen := &relayOnly{RelayAddressGeneratorStatic: turn.RelayAddressGeneratorStatic{RelayAddress: loopbackIP, Address: loopbackIP.String()}, ports: map[int]bool{}}
	s, err := turn.NewServer(turn.ServerConfig{Realm: "test", AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "peer", turn.GenerateAuthKey("peer", "test", password), r.Username == "peer" && r.Realm == "test"
	}, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: l, RelayAddressGenerator: gen}}})
	check(t, err)
	t.Cleanup(func() { s.Close() })
	return proto.ICEServer{URLs: []string{"turn:" + l.LocalAddr().String() + "?transport=udp"}, Username: "peer", Credential: password}
}

// A1.3 up to the relay: only relay candidates are exchanged and only relayed
// datagrams pass, so both ends select a relayed pair and ctl works over it.
func TestLocalRelayPath(t *testing.T) {
	f := newPair(t)
	server := localRelay(t)
	f.b.paths = func(string) []Route { r := f.bMail.route(); r.ICE = []proto.ICEServer{server}; return []Route{r} }
	f.a.paths = func(string) []Route { r := f.aMail.route(); r.ICE = []proto.ICEServer{server}; return []Route{r} }
	c, err := f.a.Offer(t.Context(), f.b.key.ID())
	check(t, err)
	t.Cleanup(func() { c.Close() })
	offer := recv(t, f.aMail["offer"])
	deliver(t, f.b, f.bMail.route(), offer)
	answer := recv(t, f.bMail["answer"])
	deliver(t, f.a, f.aMail.route(), answer)
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
		eq(t, end.Path(), "relay")
	}
	negotiation{c, b, offer, answer}.info(t)
}
