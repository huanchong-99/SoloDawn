package peer

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
	"github.com/pion/webrtc/v4"
)

var (
	tcpListen    = net.Listen
	steadyListen = Steady
)

// PionLog, when set before Listen, makes the loggers of the pion stack that
// Listen builds; nil is pion's default. The acceptance harness sets it.
var PionLog logging.LoggerFactory

type Sockets struct {
	udp           ice.UDPMux
	tcp           ice.TCPMux
	offer, answer *webrtc.API
	stop          context.CancelFunc // ends the back-off of the ICE-TCP accept
}

// Listen reserves udpPort on each eligible local address, loopback included,
// for the host candidates of answered connections, and opens the passive
// ICE-TCP listener on tcpPort for both roles (contracts/rendezvous.md
// section 5.1, point 3). id is the node's device id.
func Listen(udpPort, tcpPort int, id string) (*Sockets, error) {
	engine := webrtc.SettingEngine{LoggerFactory: PionLog}
	engine.SetIncludeLoopbackCandidate(true)
	return listen(udpPort, ":"+strconv.Itoa(tcpPort), id, engine, ice.UDPMuxFromPortWithLoopback())
}

// listen builds on engine and passes opts to the UDP mux, so that tests can
// keep every socket on the loopback address.
func listen(udpPort int, tcpAddr, id string, engine webrtc.SettingEngine, opts ...ice.UDPMuxFromPortOption) (*Sockets, error) {
	udp, err := ice.NewMultiUDPMuxFromPort(udpPort, opts...)
	if err != nil {
		return nil, err
	}
	tcp, err := tcpListen("tcp", tcpAddr)
	if err != nil {
		udp.Close()
		return nil, err
	}
	// pion's accept loop ends at the first error; Close ends its back-off.
	ctx, stop := context.WithCancel(context.Background())
	diag := slog.With("node", diagDevice(id))
	steady := steadyListen(ctx, tcp, diag, "ice_tcp")
	s := &Sockets{udp: udp, tcp: webrtc.NewICETCPMux(nil, steady, 8192), stop: stop}
	engine.SetNet(&turnNet{diag: diag})
	engine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6, webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6})
	engine.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)
	engine.SetICETCPMux(s.tcp)
	s.offer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	engine.SetICEUDPMux(udp)
	s.answer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	return s, nil
}
func (s *Sockets) Close() error {
	s.stop()
	return errors.Join(s.udp.Close(), s.tcp.Close())
}

// turnNet is pion's standard network, except that the socket of a TURN
// client over UDP, the only socket pion ICE opens with ListenPacket, passes
// on datagrams from its TURN server only: pion's client stops reading for
// good at a STUN request from anyone (issues.md FLAKY-33).
type turnNet struct {
	stdnet.Net
	diag *slog.Logger
}

// Interfaces reads them again on each call, as pion's default network does
// for each connection.
func (*turnNet) Interfaces() ([]*transport.Interface, error) {
	n, err := stdnet.NewNet()
	ifaces, _ := n.Interfaces() // stdnet returns no error here
	return ifaces, err
}
func (n *turnNet) ListenPacket(network, address string) (net.PacketConn, error) {
	conn, err := n.Net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}
	return &serverOnly{PacketConn: conn, diag: n.diag}, nil
}

// serverOnly drops each datagram that is not from the address it last sent
// to: the TURN client sends to its server only.
type serverOnly struct {
	net.PacketConn
	diag   *slog.Logger
	server atomic.Value // string
}

func (c *serverOnly) WriteTo(b []byte, to net.Addr) (int, error) {
	c.server.Store(to.String())
	return c.PacketConn.WriteTo(b, to)
}
func (c *serverOnly) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, from, err := c.PacketConn.ReadFrom(b)
		if err != nil || from.String() == c.server.Load() {
			return n, from, err
		}
		c.diag.Warn("turn.stray", "from", from.String())
	}
}

func iceConfig(paths []Route) webrtc.Configuration {
	var servers []webrtc.ICEServer
	for _, path := range paths {
		for _, s := range path.ICE {
			servers = append(servers, iceServer(s))
		}
	}
	return webrtc.Configuration{ICEServers: servers}
}
func iceServer(s proto.ICEServer) webrtc.ICEServer {
	return webrtc.ICEServer{URLs: s.URLs, Username: s.Username, Credential: s.Credential}
}

// Path classifies the pair this side's ICE agent selected: "relay" when
// either candidate is a relay candidate, "direct" for any other pair, and ""
// while no pair is selected (flows/connect.md step 8). It reads the pair on
// every call, so it is current after each signal of PathChanged.
func (c *Connection) Path() string {
	pair, _ := c.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair() // fails only once closed, with no pair
	if pair == nil {
		return ""
	}
	return classify(pair.Local.Typ, pair.Remote.Typ)
}
func classify(local, remote webrtc.ICECandidateType) string {
	if local == webrtc.ICECandidateTypeRelay || remote == webrtc.ICECandidateTypeRelay {
		return "relay"
	}
	return "direct"
}

// PathChanged is signalled, without blocking and at most one signal pending,
// whenever Path may have changed: a newly selected pair, or a remote candidate
// applied, which can change the type of a selected candidate at an unchanged
// address (contracts/rendezvous-evidence.md, entry 11).
func (c *Connection) PathChanged() <-chan struct{} { return c.pathChanged }

func (c *Connection) selectedPairChanged(pair *webrtc.ICECandidatePair) {
	local, remote := pair.Local.Typ, pair.Remote.Typ
	c.diag.event(slog.LevelInfo, "ice.pair", "local", local.String(), "remote", remote.String(),
		"proto", pair.Local.Protocol.String(), "path", classify(local, remote))
	c.reclassify()
}

func (c *Connection) reclassify() {
	select {
	case c.pathChanged <- struct{}{}:
	default:
	}
}
