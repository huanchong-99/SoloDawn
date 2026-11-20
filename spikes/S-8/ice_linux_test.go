//go:build netsim

package s8

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/features/netsim"
	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
)

// runRelay is the rendezvous in the WAN: STUN and TURN on 11.0.0.1:3478.
func runRelay(in io.Reader, out *emitter) error {
	udp, err := net.ListenPacket("udp4", netsim.WANIP+":3478")
	if err != nil {
		return err
	}
	s, err := turn.NewServer(turn.ServerConfig{
		Realm: "s8",
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			return "s8", turn.GenerateAuthKey("s8", "s8", "s8"), r.Username == "s8"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
			RelayAddress: net.ParseIP(netsim.WANIP), Address: netsim.WANIP,
		}}},
	})
	if err != nil {
		return err
	}
	out.send(msg{T: "ready"})
	_, _ = io.Copy(io.Discard, in) // until the test closes stdin
	return s.Close()
}

// peer is one end of a connection as internal/peer opens it: the answerer's
// host candidates on the UDP mux of port 47800, the offerer's on sockets of
// its own (contracts/rendezvous.md 5.1, point 3), mDNS query only, a ctl
// data channel. IPv4 UDP only. S8_MAP=pmp maps the socket of the host
// candidate by NAT-PMP and trickles the granted endpoint as srflx.
type peer struct {
	out    *emitter
	pc     *webrtc.PeerConnection
	start  time.Time
	mu     sync.Mutex
	held   []webrtc.ICECandidateInit
	ready  bool
	mapped bool
}

func runPeer(in io.Reader, out *emitter) error {
	offer := os.Getenv("S8_SIDE") == "offer"
	e := webrtc.SettingEngine{LoggerFactory: &logging.DefaultLoggerFactory{Writer: os.Stderr, DefaultLogLevel: logging.LogLevelWarn}}
	e.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	e.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)
	if offer {
		// Keep offering ports apart from symmetric NAT's 40000-40999.
		if err := e.SetEphemeralUDPPortRange(50000, 59999); err != nil {
			return err
		}
	} else {
		mux, err := ice.NewMultiUDPMuxFromPort(answerPort, ice.UDPMuxFromPortWithNetworks(ice.NetworkTypeUDP4))
		if err != nil {
			return err
		}
		defer mux.Close()
		e.SetICEUDPMux(mux)
	}
	servers := []webrtc.ICEServer{{URLs: []string{"stun:" + netsim.WANIP + ":3478"}}}
	if os.Getenv("S8_TURN") == "1" {
		servers = append(servers, webrtc.ICEServer{URLs: []string{"turn:" + netsim.WANIP + ":3478?transport=udp"}, Username: "s8", Credential: "s8"})
	}
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(e)).NewPeerConnection(webrtc.Configuration{ICEServers: servers})
	if err != nil {
		return err
	}
	defer pc.Close()
	p := &peer{out: out, pc: pc, start: time.Now()}
	pc.OnICECandidate(p.candidate)
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) { out.send(msg{T: "state", Kind: s.String(), Ms: since(p.start)}) })
	if offer {
		if err := p.offer(); err != nil {
			return err
		}
	} else {
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			dc.OnOpen(func() { p.pair("pair") })
			dc.OnMessage(func(m webrtc.DataChannelMessage) { _ = dc.Send(m.Data) })
		})
	}
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 64<<10), 1<<20)
	for lines.Scan() {
		var m msg
		if err := json.Unmarshal(lines.Bytes(), &m); err != nil {
			return err
		}
		if err := p.receive(m); err != nil {
			out.send(msg{T: "error", Err: err.Error()})
		}
	}
	return lines.Err()
}

func (p *peer) offer() error {
	dc, err := p.pc.CreateDataChannel("ctl", nil)
	if err != nil {
		return err
	}
	dc.OnOpen(func() { _ = dc.SendText("ping") })
	dc.OnMessage(func(webrtc.DataChannelMessage) { p.pair("echo") })
	o, err := p.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := p.pc.SetLocalDescription(o); err != nil {
		return err
	}
	p.out.send(msg{T: "sdp", Kind: o.Type.String(), SDP: o.SDP})
	return nil
}

// pair reports the selected candidate pair as this side sees it.
func (p *peer) pair(event string) {
	pair, err := p.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	m := msg{T: event, Ms: since(p.start), Err: errText(err)}
	if pair != nil {
		m.Local = pair.Local.Typ.String() + " " + net.JoinHostPort(pair.Local.Address, strconv.Itoa(int(pair.Local.Port)))
		m.Remote = pair.Remote.Typ.String() + " " + net.JoinHostPort(pair.Remote.Address, strconv.Itoa(int(pair.Remote.Port)))
	}
	p.out.send(m)
}

func (p *peer) candidate(c *webrtc.ICECandidate) {
	if c == nil {
		return
	}
	p.out.send(msg{T: "cand", Cand: c.ToJSON().Candidate})
	p.mu.Lock()
	first := c.Typ == webrtc.ICECandidateTypeHost && !p.mapped && os.Getenv("S8_MAP") == "pmp"
	p.mapped = p.mapped || first
	p.mu.Unlock()
	if first {
		go p.publish(int(c.Port))
	}
}

// publish asks the gateway for the host candidate's port and trickles the
// external address and granted port as a server-reflexive candidate.
func (p *peer) publish(internal int) {
	start := time.Now()
	gw, err := gateway()
	if err != nil {
		p.out.send(msg{T: "mapped", Err: err.Error()})
		return
	}
	c := natpmp.NewClientWithTimeout(gw, 2*time.Second)
	want, _ := strconv.Atoi(os.Getenv("S8_ASK"))
	if want == 0 {
		want = internal
	}
	r, err := c.AddPortMapping("udp", internal, want, 120)
	if err != nil {
		p.out.send(msg{T: "mapped", Err: err.Error()})
		return
	}
	ext, err := c.GetExternalAddress()
	if err != nil {
		p.out.send(msg{T: "mapped", Err: err.Error()})
		return
	}
	ip := net.IP(ext.ExternalIPAddress[:]).String()
	cand, err := ice.NewCandidateServerReflexive(&ice.CandidateServerReflexiveConfig{
		Network: "udp", Address: ip, Port: int(r.MappedExternalPort), Component: 1,
		RelAddr: netsim.ClientIP, RelPort: internal,
	})
	if err != nil {
		p.out.send(msg{T: "mapped", Err: err.Error()})
		return
	}
	p.out.send(msg{T: "mapped", IP: ip, Port: int(r.MappedExternalPort), Internal: internal, External: want, Ms: since(start)})
	p.out.send(msg{T: "cand", Cand: "candidate:" + cand.Marshal()})
}

// receive applies a description or candidate from the other end; candidates
// that arrive before the description are held (contracts/rendezvous.md 2.3).
func (p *peer) receive(m msg) error {
	switch m.T {
	case "sdp":
		return p.describe(m)
	case "cand":
		c := webrtc.ICECandidateInit{Candidate: m.Cand}
		p.mu.Lock()
		if !p.ready {
			p.held = append(p.held, c)
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		return p.pc.AddICECandidate(c)
	}
	return fmt.Errorf("unexpected %q", m.T)
}

func (p *peer) describe(m msg) error {
	err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.NewSDPType(m.Kind), SDP: m.SDP})
	if err != nil {
		return err
	}
	if m.Kind == "offer" {
		a, err := p.pc.CreateAnswer(nil)
		if err != nil {
			return err
		}
		if err := p.pc.SetLocalDescription(a); err != nil {
			return err
		}
		p.out.send(msg{T: "sdp", Kind: a.Type.String(), SDP: a.SDP})
	}
	p.mu.Lock()
	held := p.held
	p.held, p.ready = nil, true
	p.mu.Unlock()
	for _, c := range held {
		if err := p.pc.AddICECandidate(c); err != nil {
			return err
		}
	}
	return nil
}
