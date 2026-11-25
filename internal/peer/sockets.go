package peer

import (
	"errors"
	"log/slog"
	"net"
	"strconv"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

type Sockets struct {
	udp           ice.UDPMux
	tcp           ice.TCPMux
	offer, answer *webrtc.API
}

// Listen reserves udpPort on each eligible local address for the host
// candidates of answered connections, and opens the passive ICE-TCP listener
// on tcpPort for both roles (contracts/rendezvous.md section 5.1, point 3).
func Listen(udpPort, tcpPort int) (*Sockets, error) {
	return listen(udpPort, ":"+strconv.Itoa(tcpPort), webrtc.SettingEngine{})
}

// listen builds on engine and passes opts to the UDP mux, so that tests can
// keep every socket on the loopback address.
func listen(udpPort int, tcpAddr string, engine webrtc.SettingEngine, opts ...ice.UDPMuxFromPortOption) (*Sockets, error) {
	udp, err := ice.NewMultiUDPMuxFromPort(udpPort, opts...)
	if err != nil {
		return nil, err
	}
	tcp, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		udp.Close()
		return nil, err
	}
	s := &Sockets{udp: udp, tcp: webrtc.NewICETCPMux(nil, tcp, 8192)}
	engine.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6, webrtc.NetworkTypeTCP4, webrtc.NetworkTypeTCP6})
	engine.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)
	engine.SetICETCPMux(s.tcp)
	s.offer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	engine.SetICEUDPMux(udp)
	s.answer = webrtc.NewAPI(webrtc.WithSettingEngine(engine))
	return s, nil
}
func (s *Sockets) Close() error { return errors.Join(s.udp.Close(), s.tcp.Close()) }

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
