//go:build netsim

package netsim

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/turn/v5"
)

const (
	helperEnv      = "SOLODAWN_NETSIM_ROLE"
	answerNodePort = 47800
	offerNodePort  = 47900
)

type helperMsg struct {
	T         string          `json:"t"`
	Mapped    bool            `json:"mapped,omitempty"`
	SignalURL string          `json:"signal_url,omitempty"`
	Path      string          `json:"path,omitempty"`
	Key       string          `json:"key,omitempty"`
	Env       json.RawMessage `json:"env,omitempty"`
	Err       string          `json:"err,omitempty"`
}

type helperWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (w *helperWriter) send(m helperMsg) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(m)
}

// TestPortMapHelper runs inside a child network namespace when invoked by
// spawnHelper; during normal test discovery it returns immediately.
func TestPortMapHelper(t *testing.T) {
	role := os.Getenv(helperEnv)
	if role == "" {
		return
	}
	out := &helperWriter{enc: json.NewEncoder(os.NewFile(3, "events"))}
	var err error
	switch role {
	case "mapper":
		err = serveMapper(os.Stdin, out)
	case "turn":
		err = serveTURN(os.Stdin, out)
	case "peer":
		err = servePeer(os.Stdin, out)
	default:
		err = fmt.Errorf("unknown helper role %q", role)
	}
	if err != nil {
		out.send(helperMsg{T: "fatal", Err: err.Error()})
		os.Exit(1)
	}
}

func serveMapper(in io.Reader, out *helperWriter) error {
	sig, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", ClientIP, answerNodePort))
	if err != nil {
		return err
	}
	defer sig.Close()
	go func() {
		for {
			c, err := sig.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("rv"))
			c.Close()
		}
	}()
	socks, err := peer.Listen(answerNodePort, answerNodePort+1)
	if err != nil {
		return err
	}
	defer socks.Close()
	pm := socks.MapPorts(answerNodePort)
	<-pm.Ready()
	out.send(helperMsg{T: "ready", Mapped: pm.Mapped(), SignalURL: pm.SignalURL()})
	lines := bufio.NewScanner(in)
	for lines.Scan() {
		var req helperMsg
		if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
			return err
		}
		switch req.T {
		case "refresh":
			pm.Refresh()
			out.send(helperMsg{T: "refreshed", Mapped: pm.Mapped(), SignalURL: pm.SignalURL()})
		case "close":
			_ = socks.Close()
			out.send(helperMsg{T: "closed", Mapped: pm.Mapped(), SignalURL: pm.SignalURL()})
			return nil
		}
	}
	return lines.Err()
}

func serveTURN(in io.Reader, out *helperWriter) error {
	udp, err := net.ListenPacket("udp4", WANIP+":3478")
	if err != nil {
		return err
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "netsim",
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			return "u", turn.GenerateAuthKey("u", "netsim", "p"), r.Username == "u"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn: udp,
			RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
				RelayAddress: net.ParseIP(WANIP), Address: WANIP,
			},
		}},
	})
	if err != nil {
		return err
	}
	out.send(helperMsg{T: "ready"})
	_, _ = io.Copy(io.Discard, in)
	return srv.Close()
}

func servePeer(in io.Reader, out *helperWriter) error {
	dir, err := os.MkdirTemp("", "solodawn-peer-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	key, err := identity.LoadKey(dir)
	if err != nil {
		return err
	}
	basePort := answerNodePort
	if os.Getenv("NETSIM_SIDE") == "offer" {
		basePort = offerNodePort
	}
	socks, err := peer.Listen(basePort, basePort+1)
	if err != nil {
		return err
	}
	defer socks.Close()
	if os.Getenv("NETSIM_MAP") == "1" {
		pm := socks.MapPorts(basePort)
		if os.Getenv("NETSIM_WAIT_MAP") == "1" {
			<-pm.Ready()
		}
	}
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	defer db.Close()

	var iceServers []proto.ICEServer
	if os.Getenv("NETSIM_STUN") == "1" {
		iceServers = []proto.ICEServer{{URLs: []string{"stun:" + WANIP + ":3478"}}}
	}
	route := peer.Route{
		ID:  "pipe",
		ICE: iceServers,
		Send: func(_ context.Context, _ string, env proto.Envelope) error {
			raw, err := json.Marshal(env)
			if err != nil {
				return err
			}
			out.send(helperMsg{T: "sig", Key: key.ID(), Env: raw})
			return nil
		},
	}

	var remoteMu sync.Mutex
	var remoteID string
	accepted := make(chan *peer.Connection, 1)
	node := peer.New(
		key, socks, db,
		func(id string) bool { remoteMu.Lock(); defer remoteMu.Unlock(); return id == remoteID },
		func(string) []peer.Route { return []peer.Route{route} },
		func(c *peer.Connection) { accepted <- c },
		nil,
	)
	out.send(helperMsg{T: "ready", Key: key.ID()})
	return drivePeerLoop(in, out, node, route, accepted, &remoteMu, &remoteID)
}

func drivePeerLoop(in io.Reader, out *helperWriter, node *peer.Peer, route peer.Route, accepted <-chan *peer.Connection, remoteMu *sync.Mutex, remoteID *string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 64<<10), 1<<20)
	for lines.Scan() {
		var m helperMsg
		if err := json.Unmarshal(lines.Bytes(), &m); err != nil {
			return err
		}
		switch m.T {
		case "peer":
			remoteMu.Lock()
			*remoteID = m.Key
			remoteMu.Unlock()
			if os.Getenv("NETSIM_SIDE") == "answer" {
				go serveAnswerConn(ctx, out, accepted)
			} else {
				go runOfferConn(ctx, out, node, m.Key)
			}
		case "sig":
			_ = node.Receive(ctx, route, proto.Route{Type: "from", Key: m.Key, Env: m.Env})
		case "close":
			cancel()
			return nil
		}
	}
	return lines.Err()
}

func serveAnswerConn(ctx context.Context, out *helperWriter, accepted <-chan *peer.Connection) {
	select {
	case <-ctx.Done():
		return
	case c := <-accepted:
		defer c.Close()
		data, err := c.Read(ctx)
		if err != nil {
			out.send(helperMsg{T: "done", Err: err.Error()})
			return
		}
		var q proto.Request
		if err := proto.Decode(data, &q); err != nil {
			out.send(helperMsg{T: "done", Err: err.Error()})
			return
		}
		_ = c.Send(proto.Success{Re: q.ID, OK: true})
		waitDirectPath(ctx, c)
		out.send(helperMsg{T: "done", Path: c.Path()})
		<-ctx.Done()
	}
}

func runOfferConn(ctx context.Context, out *helperWriter, node *peer.Peer, target string) {
	c, err := node.Offer(ctx, target)
	if err != nil {
		out.send(helperMsg{T: "done", Err: err.Error()})
		return
	}
	defer c.Close()
	select {
	case <-ctx.Done():
		out.send(helperMsg{T: "done", Err: ctx.Err().Error()})
		return
	case <-c.Done():
		out.send(helperMsg{T: "done", Err: "connection closed before Ready"})
		return
	case <-c.Ready():
	}
	if _, err := c.Request(ctx, func(id int64) any { return proto.Request{Type: "ping", ID: id} }); err != nil {
		out.send(helperMsg{T: "done", Err: err.Error()})
		return
	}
	waitDirectPath(ctx, c)
	out.send(helperMsg{T: "done", Path: c.Path()})
	<-ctx.Done()
}

func waitDirectPath(ctx context.Context, c *peer.Connection) {
	for c.Path() == "" {
		select {
		case <-ctx.Done():
			return
		case <-c.PathChanged():
		}
	}
}

type helperProc struct {
	name string
	in   *helperWriter
	out  chan helperMsg
}

func spawnHelper(t *testing.T, ns, role string, extraEnv ...string) *helperProc {
	t.Helper()
	exe, err := os.Executable()
	must(t, err)
	cmd := exec.Command("ip", "netns", "exec", ns, exe, "-test.run=^TestPortMapHelper$")
	cmd.Env = append(append(os.Environ(), helperEnv+"="+role), extraEnv...)
	cmd.Stdout, cmd.Stderr = t.Output(), t.Output()
	stdin, err := cmd.StdinPipe()
	must(t, err)
	r, w, err := os.Pipe()
	must(t, err)
	cmd.ExtraFiles = []*os.File{w}
	must(t, cmd.Start())
	w.Close()
	done := make(chan struct{})
	h := &helperProc{name: role, in: &helperWriter{enc: json.NewEncoder(stdin)}, out: make(chan helperMsg, 64)}
	go func() {
		defer close(h.out)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var m helperMsg
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				select {
				case h.out <- m:
				case <-done:
					return
				}
			}
		}
	}()
	t.Cleanup(func() {
		close(done)
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		r.Close()
	})
	return h
}

func (h *helperProc) recv(t *testing.T, d time.Duration) helperMsg {
	t.Helper()
	select {
	case m, ok := <-h.out:
		if !ok {
			t.Fatalf("%s exited unexpectedly", h.name)
		}
		if m.T == "fatal" {
			t.Fatalf("%s failed: %s", h.name, m.Err)
		}
		return m
	case <-time.After(d):
		t.Fatalf("%s timed out after %s", h.name, d)
	}
	panic("unreachable")
}

// occupyPort maps 47800/UDP and 47800/TCP from a second LAN address 10.0.0.3 so
// the gateway grants 10.0.0.2 different external ports than requested.
func occupyPort(t *testing.T, clientNS string) {
	t.Helper()
	run(t, "", "ip", "-n", clientNS, "address", "add", "10.0.0.3/24", "dev", "eth0")
	c := socket(t, clientNS, "10.0.0.3", 0)
	for _, opcode := range []byte{1, 2} { // 1 = UDP, 2 = TCP
		req := []byte{0, opcode, 0, 0, 0xba, 0xb8, 0xba, 0xb8, 0, 0, 0x0e, 0x10}
		_, err := c.WriteToUDPAddrPort(req, netip.MustParseAddrPort("10.0.0.1:5351"))
		must(t, err)
		must(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
		var resp [16]byte
		n, _, err := c.ReadFromUDPAddrPort(resp[:])
		must(t, err)
		if n != 16 || resp[3] != 0 {
			t.Fatalf("occupy opcode %d failed: % x", opcode, resp[:n])
		}
	}
}

var clientDNAT = regexp.MustCompile(`dport (\d+) .*dnat (?:ip )?to 10\.0\.0\.2(?::\d+)?`)

func clientMappings(t *testing.T, gwNS string) []string {
	t.Helper()
	out, err := exec.Command("ip", "netns", "exec", gwNS, "nft", "list", "chain", "ip", "netsim", "prerouting_miniupnpd").CombinedOutput()
	must(t, err)
	var rules []string
	for _, m := range clientDNAT.FindAllStringSubmatch(string(out), -1) {
		rules = append(rules, m[1])
	}
	return rules
}

func dialSignalFromWAN(t *testing.T, wanNS, hostPort string, want bool) {
	t.Helper()
	inNamespace(t, wanNS, func() {
		c, err := net.DialTimeout("tcp4", hostPort, 500*time.Millisecond)
		if !want {
			if err == nil {
				c.Close()
				t.Fatalf("expected %s unreachable after close", hostPort)
			}
			return
		}
		must(t, err)
		defer c.Close()
		must(t, c.SetReadDeadline(time.Now().Add(time.Second)))
		var buf [2]byte
		_, err = io.ReadFull(c, buf[:])
		must(t, err)
		if string(buf[:]) != "rv" {
			t.Fatalf("got %q, want rv", buf)
		}
	})
}

func TestPortMapGatewayProtocolsAndRemoval(t *testing.T) {
	for _, mode := range []string{"natpmp", "upnp", "no-gateway"} {
		removed(t, mode, func(t *testing.T) Network {
			n := Start(t, PortRestricted, PortRestricted)
			if mode == "no-gateway" {
				m := spawnHelper(t, n.Right.Client, "mapper")
				ready := m.recv(t, 8*time.Second)
				if ready.Mapped || ready.SignalURL != "" {
					t.Fatalf("unexpected mapping without gateway: %+v", ready)
				}
				m.in.send(helperMsg{T: "close"})
				m.recv(t, 5*time.Second)
				return n
			}

			n.Right.StartUPnP(t)
			occupyPort(t, n.Right.Client)
			if mode == "upnp" {
				// Silence NAT-PMP (UDP 5351) to verify bounded timeout + UPnP IGDv2 fallback.
				run(t, "", "ip", "netns", "exec", n.Right.Gateway, "nft", "insert", "rule", "ip", "netsim", "input",
					"iifname", `"lan"`, "udp", "dport", "5351", "drop")
			}

			m := spawnHelper(t, n.Right.Client, "mapper")
			ready := m.recv(t, 10*time.Second)
			if !ready.Mapped {
				t.Fatalf("%s: expected Mapped() == true", mode)
			}
			u, err := url.Parse(ready.SignalURL)
			must(t, err)
			if u.Hostname() != n.Right.PublicIP || u.Port() == "47800" || u.Path != "/rv" {
				t.Fatalf("%s: unexpected SignalURL %q (occupied 47800 must yield another port)", mode, ready.SignalURL)
			}
			if rules := clientMappings(t, n.Right.Gateway); len(rules) != 3 {
				t.Fatalf("%s: want 3 DNAT rules for 10.0.0.2, got %v", mode, rules)
			}
			dialSignalFromWAN(t, n.WAN, u.Host, true)

			m.in.send(helperMsg{T: "close"})
			closed := m.recv(t, 5*time.Second)
			if closed.Mapped || closed.SignalURL != "" {
				t.Fatalf("%s: mapping not withdrawn on close: %+v", mode, closed)
			}
			if rules := clientMappings(t, n.Right.Gateway); len(rules) != 0 {
				t.Fatalf("%s: DNAT rules survived close: %v", mode, rules)
			}
			dialSignalFromWAN(t, n.WAN, u.Host, false)
			return n
		})
	}
}

func TestPortMapDirectPeerAcrossSymmetricNAT(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mapOffer bool
		mapAns   bool
		upnpOnly bool
		blockUDP bool
		withSTUN bool
	}{
		{name: "answerer-udp-natpmp-occupied", mapAns: true, withSTUN: true},
		{name: "answerer-icetcp-upnp-udp-blocked", mapAns: true, upnpOnly: true, blockUDP: true},
		{name: "offerer-icetcp-udp-blocked", mapOffer: true, blockUDP: true},
	} {
		removed(t, tc.name, func(t *testing.T) Network {
			n := Start(t, Symmetric, Symmetric)
			if tc.mapAns {
				n.Right.StartUPnP(t)
				occupyPort(t, n.Right.Client)
				if tc.upnpOnly {
					run(t, "", "ip", "netns", "exec", n.Right.Gateway, "nft", "insert", "rule", "ip", "netsim", "input",
						"iifname", `"lan"`, "udp", "dport", "5351", "drop")
				}
			}
			if tc.mapOffer {
				n.Left.StartUPnP(t)
			}
			if tc.blockUDP {
				n.SetBlocked(t, true, 0)
			}
			if tc.withSTUN {
				stunSrv := spawnHelper(t, n.WAN, "turn")
				stunSrv.recv(t, 5*time.Second)
			}

			boolStr := func(v bool) string {
				if v {
					return "1"
				}
				return "0"
			}
			// Notice NETSIM_WAIT_MAP=0 on UPnP test exercises trickling mapped
			// candidates after the offer is already in flight!
			offer := spawnHelper(t, n.Left.Client, "peer", "NETSIM_SIDE=offer", "NETSIM_MAP="+boolStr(tc.mapOffer), "NETSIM_WAIT_MAP="+boolStr(tc.mapOffer), "NETSIM_STUN="+boolStr(tc.withSTUN))
			answer := spawnHelper(t, n.Right.Client, "peer", "NETSIM_SIDE=answer", "NETSIM_MAP="+boolStr(tc.mapAns), "NETSIM_WAIT_MAP="+boolStr(!tc.upnpOnly), "NETSIM_STUN="+boolStr(tc.withSTUN))

			oReady := offer.recv(t, 10*time.Second)
			aReady := answer.recv(t, 10*time.Second)
			answer.in.send(helperMsg{T: "peer", Key: oReady.Key})
			offer.in.send(helperMsg{T: "peer", Key: aReady.Key})

			bridge := func(from, to *helperProc, result chan<- helperMsg) {
				for m := range from.out {
					if m.T == "sig" {
						to.in.send(m)
						continue
					}
					result <- m
					return
				}
				result <- helperMsg{Err: from.name + " closed"}
			}
			oDone, aDone := make(chan helperMsg, 1), make(chan helperMsg, 1)
			go bridge(offer, answer, oDone)
			go bridge(answer, offer, aDone)

			for _, ch := range []chan helperMsg{oDone, aDone} {
				select {
				case res := <-ch:
					if res.Err != "" || res.Path != "direct" {
						t.Fatalf("%s: got %+v, want direct path", tc.name, res)
					}
				case <-time.After(20 * time.Second):
					t.Fatalf("%s: peer connection timed out", tc.name)
				}
			}
			offer.in.send(helperMsg{T: "close"})
			answer.in.send(helperMsg{T: "close"})
			return n
		})
	}
}
