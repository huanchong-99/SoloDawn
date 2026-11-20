//go:build netsim

package s8

import (
	"fmt"
	"io"
	"net"
	"os"
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
	out.send(msg{T: "cand", Cand: fmt.Sprintf("candidate:4207374052 1 tcp 1694498815 %s %d typ srflx raddr %s rport %d tcptype passive",
		ip, r.MappedExternalPort, netsim.ClientIP, iceTCPPort)})
}

// The node maps its passive ICE-TCP listener too (rendezvous.md 5.1, point
// 8). With UDP between the peers blocked and no relay, only a mapped TCP
// endpoint can connect two symmetric NATs.
func TestICETCPMapped(t *testing.T) {
	for _, mapped := range []bool{false, true} {
		t.Run(fmt.Sprintf("mapped=%v", mapped), func(t *testing.T) {
			o := punch(t, netsim.Symmetric, netsim.Symmetric, variant{tcp: true, mapTCP: mapped, blockUDP: true})
			t.Logf("UDP blocked, ICE-TCP mapped %v: path %s at %d ms; offerer %s; answerer %s; %s%s",
				mapped, o.path(), o.ms, o.offer, o.answer, o.mapped, o.failure)
			if want := map[bool]string{false: "none", true: "direct"}[mapped]; o.path() != want || (mapped && !strings.Contains(o.offer, "tcp")) {
				t.Errorf("path %s over %q, want %s", o.path(), o.offer, want)
			}
		})
	}
}
