//go:build acceptance

package features

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/webrtc/v4"
)

func init() {
	stepSets["peer_dispatch"] = func(sc *godog.ScenarioContext, _ fixtures) { dispatchSteps(sc) }
}

// dispatchWorld is a node and a browser made of a bare WebRTC stack, which
// opens channels that a node never opens. They reach each other through a
// TURN server on 127.0.0.1 only, so that the scenario runs where a firewall
// refuses other addresses.
type dispatchWorld struct {
	ctx      context.Context
	cleanup  func(func())
	browser  identity.Key
	node     identity.Key
	peer     *peer.Peer
	handed   chan *peer.Stream // what the node's own handler was given
	stream   *peer.Stream
	ice      []proto.ICEServer
	accepted chan *peer.Connection
	served   *peer.Connection
	toPeer   chan proto.Envelope // the node's relay candidates, for the browser
	pc       *webrtc.PeerConnection
	channels map[string]*webrtc.DataChannel
	opened   map[string]chan struct{}
	closed   map[string]chan struct{}
	received chan []byte // messages the browser got on any channel but ctl
	ctl      chan json.RawMessage
}

func dispatchSteps(sc *godog.ScenarioContext) {
	w := &dispatchWorld{cleanup: afterScenario(sc)}
	sc.Step(`^a node that hands channels of the family "([^"]*)" to a handler of its own$`, w.startNode)
	sc.Step(`^a browser connected to the node through a relay on this machine$`, w.connect)
	sc.Step(`^the browser opens the channels "([^"]*)" and "([^"]*)" beside ctl$`, w.openChannels)
	sc.Step(`^the node's handler gets "([^"]*)" as a stream of the browser's device$`, w.handedOver)
	sc.Step(`^bytes cross that stream in both directions$`, w.crossing)
	sc.Step(`^the node closes "([^"]*)"$`, func(label string) error { return w.wait(w.closed[label]) })
	sc.Step(`^the node still reads the browser's hello on ctl$`, w.hello)
}

func (w *dispatchWorld) startNode(family string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	// The relay of the peer steps: its sockets take datagrams only from relay sockets.
	relay := &peersWorld{}
	err := relay.relayOnly()
	for _, undo := range relay.cleanup {
		w.cleanup(undo)
	}
	if err != nil {
		return err
	}
	w.ice, w.browser, w.node = relay.relay, exampleKey("browser"), exampleKey("node")
	w.accepted, w.toPeer, w.handed = make(chan *peer.Connection, 1), make(chan proto.Envelope, 64), make(chan *peer.Stream, 2)
	socks, err := peer.Listen(0, 0, "test")
	if err != nil {
		return err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		socks.Close()
		return err
	}
	w.cleanup(func() { socks.Close(); db.Close() })
	paths := func(string) []peer.Route { return []peer.Route{{ID: "local", Send: w.relayed, ICE: w.ice}} }
	// The handler is this node's own, as node gives each of its peers the handlers of its host.
	channels := peer.Channels{family: func(s *peer.Stream) { w.handed <- s }}
	w.peer = peer.New(w.node, socks, db, func(id string) bool { return id == w.browser.ID() }, paths, func(c *peer.Connection) { w.accepted <- c }, channels)
	return nil
}

// relayed keeps the node's relay candidates for the browser.
func (w *dispatchWorld) relayed(_ context.Context, _ string, env proto.Envelope) error {
	if strings.Contains(env.Payload, " typ relay") {
		w.toPeer <- env
	}
	return nil
}

// connect offers the node a connection with ctl, as a browser does.
func (w *dispatchWorld) connect() error {
	ice := []webrtc.ICEServer{{URLs: w.ice[0].URLs, Username: w.ice[0].Username, Credential: w.ice[0].Credential}}
	conn := randomConn()
	header := func(kind string) proto.SignalHeader { return w.header(kind, conn) }
	pc, err := browserAPI(webrtc.SettingEngine{}).NewPeerConnection(webrtc.Configuration{ICEServers: ice, ICETransportPolicy: webrtc.ICETransportPolicyRelay})
	if err != nil {
		return err
	}
	if err := w.connectRTC(pc, header, w.send); err != nil {
		return err
	}
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case w.served = <-w.accepted:
		return nil
	}
}

func (w *dispatchWorld) connectRTC(pc *webrtc.PeerConnection, header func(string) proto.SignalHeader, send func(any, *proto.Answer) error) error {
	w.pc, w.channels, w.opened, w.closed, w.received = pc, map[string]*webrtc.DataChannel{}, map[string]chan struct{}{}, map[string]chan struct{}{}, make(chan []byte, 32)
	w.ctl = make(chan json.RawMessage, 128)
	w.cleanup(func() { pc.Close() })
	if err := w.channel("ctl"); err != nil {
		return err
	}
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			send(proto.ICECandidate{SignalHeader: header("ice"), Candidate: i.ToJSON().Candidate, MID: i.SDPMid}, nil)
		}
	})
	if err := w.negotiate(header, send); err != nil {
		return err
	}
	go w.candidates()
	return w.wait(w.opened["ctl"])
}

func (w *dispatchWorld) negotiate(header func(string) proto.SignalHeader, send func(any, *proto.Answer) error) error {
	offer, err := w.pc.CreateOffer(nil)
	if err == nil {
		err = w.pc.SetLocalDescription(offer)
	}
	var answer proto.Answer
	err = errors.Join(err, send(proto.Offer{SignalHeader: header("offer"), SDP: offer.SDP}, &answer))
	if err == nil {
		err = w.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP})
	}
	return err
}

func randomConn() string {
	var b [16]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func (w *dispatchWorld) header(kind, conn string) proto.SignalHeader {
	return proto.SignalHeader{PayloadHeader: proto.PayloadHeader{Type: kind, Nonce: proto.ID(randomConn()), Timestamp: time.Now().UnixMilli(), To: w.node.ID()}, Conn: conn}
}

// send signs payload as the browser and delivers it to the node; the node's
// answer, if any, is decoded into answer. The node sends its answer from a
// goroutine of its own, so it is waited for.
func (w *dispatchWorld) send(payload any, answer *proto.Answer) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	reply := make(chan proto.Envelope, 1)
	path := peer.Route{ID: "local", Send: func(_ context.Context, _ string, env proto.Envelope) error { reply <- env; return nil }}
	if err := w.peer.Receive(w.ctx, path, signal(w.browser.Sign(string(data)))); err != nil || answer == nil {
		return err
	}
	select {
	case <-w.ctx.Done():
		return errors.New("the node did not answer")
	case env := <-reply:
		return proto.Decode([]byte(env.Payload), answer)
	}
}

// candidates applies the node's relay candidates once its answer is set.
func (w *dispatchWorld) candidates() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case env := <-w.toPeer:
			var c proto.ICECandidate
			if proto.Decode([]byte(env.Payload), &c) == nil {
				w.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: c.Candidate, SDPMid: &c.MID})
			}
		}
	}
}

// channel opens a data channel from the browser and tracks it.
func (w *dispatchWorld) channel(label string) error {
	onMessage := func(data []byte, _ bool) { w.received <- data }
	if label == "ctl" {
		onMessage = func(data []byte, _ bool) {
			select {
			case w.ctl <- json.RawMessage(data):
			case <-w.ctx.Done():
			}
		}
	}
	c, err := openBrowserChannel(w.pc, label, onMessage)
	if err != nil {
		return err
	}
	w.channels[label], w.opened[label], w.closed[label] = c.DataChannel, c.opened, c.closed
	return nil
}

// browserChannel is a data channel a test browser opens, read as a browser
// reads it: no message is lost. pion drops a message that arrives before
// OnMessage is set, and on a connected PeerConnection CreateDataChannel opens
// the channel before it returns, so a node that writes as soon as the channel
// opens (an interactive CLI's first output, A7.4) could lose its first bytes.
// A detached channel keeps them in its stream until they are read; the
// PeerConnection's API must detach data channels (browserAPI).
type browserChannel struct {
	*webrtc.DataChannel
	opened chan struct{} // closed once the channel is detached
	closed chan struct{} // closed once reading the channel ends
}

func openBrowserChannel(pc *webrtc.PeerConnection, label string, onMessage func(data []byte, isString bool)) (*browserChannel, error) {
	dc, err := pc.CreateDataChannel(label, nil)
	if err != nil {
		return nil, err
	}
	c := &browserChannel{DataChannel: dc, opened: make(chan struct{}), closed: make(chan struct{})}
	dc.OnOpen(func() {
		rw, err := dc.Detach()
		if err != nil {
			panic(err) // the API does not detach: use browserAPI
		}
		close(c.opened)
		go c.read(rw, onMessage)
	})
	return c, nil
}

func (c *browserChannel) read(rw interface {
	ReadDataChannel([]byte) (int, bool, error)
}, onMessage func([]byte, bool)) {
	defer close(c.closed)
	buf := make([]byte, 1<<16)
	for {
		n, isString, err := rw.ReadDataChannel(buf)
		if err != nil {
			return
		}
		onMessage(slices.Clone(buf[:n]), isString)
	}
}

// browserAPI is a WebRTC stack whose data channels are read as browserChannel
// reads them.
func browserAPI(se webrtc.SettingEngine) *webrtc.API {
	se.DetachDataChannels()
	return webrtc.NewAPI(webrtc.WithSettingEngine(se))
}

func (w *dispatchWorld) openChannels(registered, unknown string) error {
	return errors.Join(w.channel(registered), w.channel(unknown))
}

func (w *dispatchWorld) wait(ch <-chan struct{}) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-ch:
		return nil
	}
}

func (w *dispatchWorld) handedOver(label string) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case w.stream = <-w.handed:
	}
	if w.stream.Label() != label || w.stream.Device() != w.browser.ID() {
		return fmt.Errorf("handed %q of device %s", w.stream.Label(), w.stream.Device())
	}
	return w.wait(w.opened[label])
}

// crossing sends bytes from the browser to the handler's stream and back.
func (w *dispatchWorld) crossing() error {
	const in, out = "keys from the browser", "output of the node"
	if err := w.channels[w.stream.Label()].Send([]byte(in)); err != nil {
		return err
	}
	read := make(chan string, 1)
	go func() {
		b := make([]byte, len(in))
		n, _ := io.ReadFull(w.stream, b)
		read <- string(b[:n])
	}()
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case got := <-read:
		if got != in {
			return fmt.Errorf("the stream read %q", got)
		}
	}
	return w.streamOutput(out)
}

func (w *dispatchWorld) streamOutput(out string) error {
	if _, err := w.stream.Write([]byte(out)); err != nil {
		return err
	}
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case got := <-w.received:
		if string(got) != out {
			return fmt.Errorf("the browser got %q", got)
		}
		return nil
	}
}

// hello sends the first message of a session on ctl; the node reads it as sent.
func (w *dispatchWorld) hello() error {
	const hello = `{"t":"hello","id":1,"v":1,"kind":"browser","name":"phone browser","addrs":[]}`
	if err := w.channels["ctl"].SendText(hello); err != nil {
		return err
	}
	got, err := w.served.Read(w.ctx)
	if err != nil {
		return err
	}
	if string(got) != hello {
		return fmt.Errorf("the node read %s", got)
	}
	return nil
}
