//go:build netsim

package s8

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/features/netsim"
	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/pion/webrtc/v4"
)

const iceTCPPort = answerPort + 1 // the passive ICE-TCP listener, both roles

// iceTCP adds the passive ICE-TCP listener as internal/peer does (S8_TCP=1).
// With S8_MAP_TCP=pmp the answerer maps it by NAT-PMP and trickles the
// granted endpoint as a passive server-reflexive TCP candidate.
func iceTCP(e *webrtc.SettingEngine, out *emitter, answer bool) (io.Closer, error) {
	l, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", netsim.ClientIP, iceTCPPort))
	if err != nil {
		return nil, err
	}
	e.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4})
	mux := webrtc.NewICETCPMux(nil, l, 8192)
	e.SetICETCPMux(mux)
	if answer && os.Getenv("S8_MAP_TCP") == "pmp" {
		go publishTCP(out)
	}
	return mux, nil
}

func publishTCP(out *emitter) {
	start := time.Now()
	fail := func(err error) { out.send(msg{T: "mapped", Proto: "tcp", Err: err.Error()}) }
	gw, err := gateway()
	if err != nil {
		fail(err)
		return
	}
	c := natpmp.NewClientWithTimeout(gw, 2*time.Second)
	r, err := c.AddPortMapping("tcp", iceTCPPort, iceTCPPort, 120)
	if err != nil {
		fail(err)
		return
	}
	ext, err := c.GetExternalAddress()
	if err != nil {
		fail(err)
		return
	}
	ip := net.IP(ext.ExternalIPAddress[:]).String()
	out.send(msg{T: "mapped", Proto: "tcp", IP: ip, Port: int(r.MappedExternalPort), Internal: iceTCPPort, Ms: since(start)})
	// pion v4.4.8 keeps tcptype only on host candidates when it parses one
	// (candidate_base.go, UnmarshalCandidate), so S8_TCP_TYP=host publishes
	// the mapped endpoint as a host candidate.
	typ := "srflx raddr " + netsim.ClientIP + " rport " + strconv.Itoa(iceTCPPort)
	if os.Getenv("S8_TCP_TYP") == "host" {
		typ = "host"
	}
	out.send(msg{T: "cand", Cand: fmt.Sprintf("candidate:4207374052 1 tcp 1015022079 %s %d typ %s tcptype passive", ip, r.MappedExternalPort, typ)})
}

// The node maps its passive ICE-TCP listener too (rendezvous.md 5.1, point
// 8). With UDP between the peers blocked and no relay, only a mapped TCP
// endpoint can connect two symmetric NATs.
func TestICETCPMapped(t *testing.T) {
	for _, c := range []struct{ published, want string }{{"", "none"}, {"srflx", "none"}, {"host", "direct"}} {
		t.Run("published="+c.published, func(t *testing.T) {
			o := punch(t, netsim.Symmetric, netsim.Symmetric, variant{tcp: true, mapTCP: c.published != "", tcpType: c.published, blockUDP: true, debug: true})
			t.Logf("UDP blocked, ICE-TCP mapping published as %q: path %s at %d ms; offerer %s; answerer %s; %s%s",
				c.published, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
			if o.path() != c.want || (c.want == "direct" && !strings.Contains(o.offer, " tcp ")) {
				t.Errorf("path %s over %q, want %s", o.path(), o.offer, c.want)
			}
		})
	}
}
