package peer

import (
	"errors"
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

func Listen(udpPort, tcpPort int) (*Sockets, error) {
	udp, err := ice.NewMultiUDPMuxFromPort(udpPort)
	if err != nil {
		return nil, err
	}
	tcp, err := net.Listen("tcp", ":"+strconv.Itoa(tcpPort))
	if err != nil {
		udp.Close()
		return nil, err
	}
	s := &Sockets{udp: udp, tcp: webrtc.NewICETCPMux(nil, tcp, 8192)}
	var engine webrtc.SettingEngine
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

func (c *Connection) Path() (string, error) {
	pair, err := c.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil {
		return "", err
	}
	return classify(pair.Local.Typ, pair.Remote.Typ), nil
}
func classify(local, remote webrtc.ICECandidateType) string {
	if local == webrtc.ICECandidateTypeRelay || remote == webrtc.ICECandidateTypeRelay {
		return "relay"
	}
	return "direct"
}
