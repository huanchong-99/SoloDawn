// Disposable transport bench. Run only inside the two isolated network namespaces.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/ice/v4"
	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
)

const a = "10.71.0.1"
const b = "10.71.0.2"

var key = mustKey()
var b64 = base64.RawURLEncoding
var pcs, senders sync.Map
var clients, answers sync.Map
var signalling *websocket.Conn
var mode atomic.Value
var ctx = context.Background()
var udp ice.UDPMux
var tcp ice.TCPMux
var ip = os.Getenv("S1_IP")
var ip6 = os.Getenv("S1_IP6")

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}
func mustKey() ed25519.PrivateKey  { _, k, err := ed25519.GenerateKey(nil); return must(k, err) }
func id() string                   { return b64.EncodeToString(key.Public().(ed25519.PublicKey)) }
func nonce() string                { p := make([]byte, 16); rand.Read(p); return b64.EncodeToString(p) }
func js(v any) []byte              { return must(json.Marshal(v)) }
func url(host, path string) string { return "http://" + host + ":8080" + path }
func get(u string) string {
	r := must(http.Get(u))
	defer r.Body.Close()
	return string(must(io.ReadAll(r.Body)))
}

type obj = map[string]any
type envelope struct {
	P string `json:"p"`
	K string `json:"k"`
	S string `json:"s"`
}
type frame struct {
	T   string   `json:"t"`
	K   string   `json:"k"`
	Env envelope `json:"env"`
}

func signed(p obj) frame {
	p["n"], p["ts"] = nonce(), time.Now().UnixMilli()
	body := string(js(p))
	return frame{"to", p["to"].(string), envelope{body, id(), b64.EncodeToString(ed25519.Sign(key, []byte("solodawn/1\n"+body)))}}
}
func payload(f frame) map[string]any {
	if !ed25519.Verify(must(b64.DecodeString(f.Env.K)), []byte("solodawn/1\n"+f.Env.P), must(b64.DecodeString(f.Env.S))) {
		panic("signature")
	}
	var p obj
	check(json.Unmarshal([]byte(f.Env.P), &p))
	if p["to"] != id() {
		panic("wrong peer")
	}
	return p
}

func peer(conn, mode string) *webrtc.PeerConnection {
	s := webrtc.SettingEngine{}
	if mode != "tcp" && mode != "ephemeral" {
		s.SetICEUDPMux(udp)
	}
	s.SetICETCPMux(tcp)
	nets := []webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeTCP4}
	if mode == "tcp" {
		nets = nets[1:]
	}
	if ip6 != "" {
		nets = append(nets, webrtc.NetworkTypeTCP6)
		if mode != "tcp" {
			nets = append(nets, webrtc.NetworkTypeUDP6)
		}
	}
	s.SetNetworkTypes(nets)
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeQueryOnly)
	if mode == "renom" {
		check(s.SetICERenomination(webrtc.WithRenominationInterval(time.Second)))
	}
	cfg := webrtc.Configuration{}
	if mode == "all" || mode == "renom" || mode == "probe" {
		cfg.ICEServers = []webrtc.ICEServer{{URLs: []string{"stun:" + b + ":3478"}}}
		if mode != "probe" {
			cfg.ICEServers = append(cfg.ICEServers, webrtc.ICEServer{URLs: []string{"turn:" + b + ":3478?transport=udp"}, Username: "s1", Credential: "s1"})
		}
	}
	pc := must(webrtc.NewAPI(webrtc.WithSettingEngine(s)).NewPeerConnection(cfg))
	pcs.Store(conn, pc)
	return pc
}
func local(conn string, pc *webrtc.PeerConnection, d webrtc.SessionDescription) string {
	done := webrtc.GatheringCompletePromise(pc)
	start := time.Now()
	check(pc.SetLocalDescription(d))
	<-done
	sdp := pc.LocalDescription().SDP
	log.Printf("%s %s gather=%.1fms candidates=%d", conn, d.Type, float64(time.Since(start).Microseconds())/1000, strings.Count(sdp, "a=candidate:"))
	return sdp
}
func answer(from string, p obj) {
	conn := p["conn"].(string)
	pc := peer(conn, mode.Load().(string))
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnMessage(func(m webrtc.DataChannelMessage) { check(dc.SendText(string(m.Data))) })
	})
	check(pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: p["sdp"].(string)}))
	sdp := local(conn, pc, must(pc.CreateAnswer(nil)))
	send(signalling, signed(obj{"t": "answer", "conn": conn, "to": from, "sdp": sdp}))
}

func send(ws *websocket.Conn, f frame) { check(ws.Write(ctx, websocket.MessageText, js(f))) }

// Only route signed envelopes. Device keys are prearranged by the test driver.
func relay(w http.ResponseWriter, r *http.Request) {
	ws := must(websocket.Accept(w, r, nil))
	defer ws.CloseNow()
	ws.SetReadLimit(16 * 1024)
	from := r.URL.Query().Get("k")
	clients.Store(from, ws)
	defer clients.Delete(from)
	check(ws.Write(ctx, websocket.MessageText, []byte(`{"t":"ready"}`)))
	for {
		_, msg, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var f frame
		check(json.Unmarshal(msg, &f))
		dest, _ := clients.Load(f.K)
		send(dest.(*websocket.Conn), frame{"from", from, f.Env})
	}
}
func join() {
	var err error
	signalling, _, err = websocket.Dial(ctx, "ws://"+a+":8080/rv?k="+id(), nil)
	check(err)
	_, _, err = signalling.Read(ctx)
	check(err)
	for {
		_, msg, err := signalling.Read(ctx)
		check(err)
		var f frame
		check(json.Unmarshal(msg, &f))
		p := payload(f)
		if p["t"] == "offer" {
			go answer(f.K, p)
		} else {
			c, _ := answers.Load(p["conn"])
			c.(chan obj) <- p
		}
	}
}

type sender struct {
	dc *webrtc.DataChannel
	rx chan string
}

func dial(w http.ResponseWriter, r *http.Request) {
	remote := a
	if ip == a {
		remote = b
	}
	to := get(url(remote, "/id"))
	conn := nonce()
	pc := peer(conn, r.URL.Query().Get("mode"))
	dc := must(pc.CreateDataChannel("ctl", nil))
	open := make(chan struct{})
	s := sender{dc, make(chan string, 1)}
	dc.OnOpen(func() { close(open) })
	dc.OnMessage(func(m webrtc.DataChannelMessage) { s.rx <- string(m.Data) })
	sdp := local(conn, pc, must(pc.CreateOffer(nil)))
	ch := make(chan obj, 1)
	answers.Store(conn, ch)
	send(signalling, signed(obj{"t": "offer", "to": to, "conn": conn, "probe": false, "sdp": sdp}))
	ans := <-ch
	check(pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ans["sdp"].(string)}))
	select {
	case <-open:
	case <-time.After(15 * time.Second):
		fmt.Fprint(w, "timeout")
		return
	}
	senders.Store(conn, s)
	fmt.Fprint(w, conn)
}
func turnServer() *turn.Server {
	return must(turn.NewServer(turn.ServerConfig{Realm: "s1", AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
		return "s1", turn.GenerateAuthKey("s1", "s1", "s1"), r.Username == "s1"
	}, PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: must(net.ListenPacket("udp4", b+":3478")), RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{RelayAddress: net.ParseIP(b), Address: b, MinPort: 22000, MaxPort: 22099}}}}))
}
func main() {
	udp = ice.NewUniversalUDPMuxDefault(ice.UniversalUDPMuxParams{UDPConn: must(net.ListenPacket("udp4", ip+":47800"))})
	if ip6 != "" {
		udp = ice.NewMultiUDPMuxDefault(udp, ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: must(net.ListenPacket("udp6", net.JoinHostPort(ip6, "47800")))}))
	}
	tcp = ice.NewTCPMuxDefault(ice.TCPMuxParams{Listener: must(net.Listen("tcp", ":47801")), ReadBufferSize: 8192})
	if ip == b {
		srv := turnServer()
		defer srv.Close()
		silent := must(net.ListenPacket("udp4", b+":3479"))
		defer silent.Close()
	}
	http.HandleFunc("/id", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, id()) })
	http.HandleFunc("/target", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, get(url(b, "/id"))) })
	mode.Store("direct")
	http.HandleFunc("/mode", func(w http.ResponseWriter, r *http.Request) { mode.Store(r.URL.Query().Get("v")) })
	if ip == a {
		http.HandleFunc("/rv", relay)
	}
	http.HandleFunc("/dial", dial)
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		x, _ := senders.Load(r.URL.Query().Get("conn"))
		s := x.(sender)
		msg := r.URL.Query().Get("msg")
		check(s.dc.SendText(msg))
		select {
		case got := <-s.rx:
			fmt.Fprint(w, got)
		case <-time.After(5 * time.Second):
			fmt.Fprint(w, "timeout")
		}
	})
	http.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
		conn := r.URL.Query().Get("conn")
		if ip == a && r.URL.Query().Has("node") {
			fmt.Fprint(w, get(url(b, "/pair?conn="+conn)))
			return
		}
		x, _ := pcs.Load(conn)
		pc := x.(*webrtc.PeerConnection)
		pair := must(pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair())
		w.Write(js(pair))
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<!doctype html><title>S-1</title>") })
	listener := must(net.Listen("tcp", ":8080"))
	go http.Serve(listener, nil)
	join()
}
