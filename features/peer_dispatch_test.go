//go:build acceptance

package features

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

type handedChannel struct {
	conn  *peer.Connection
	label string
}

// handedOver receives every channel of the family "test:", registered here
// from init as a terminal or watch file registers its own.
var handedOver = make(chan handedChannel, 4)

func init() {
	peer.Channels["test:"] = func(_ *peer.Peer, c *peer.Connection, dc *webrtc.DataChannel) {
		handedOver <- handedChannel{c, dc.Label()}
	}
	stepSets["peer_dispatch"] = func(sc *godog.ScenarioContext, _ fixtures) { dispatchSteps(sc) }
}

// dispatchWorld is a node and a browser made of a bare WebRTC stack, which
// opens channels that a node never opens. They reach each other through a
// TURN server on 127.0.0.1 only, so that the scenario runs where a firewall
// refuses other addresses.
type dispatchWorld struct {
	ctx      context.Context
	browser  identity.Key
	node     identity.Key
	peer     *peer.Peer
	ice      []proto.ICEServer
	accepted chan *peer.Connection
	served   *peer.Connection
	toPeer   chan proto.Envelope // the node's relay candidates, for the browser
	pc       *webrtc.PeerConnection
	ctl      *webrtc.DataChannel
	opened   map[string]chan struct{}
	closed   map[string]chan struct{}
	cleanup  []func()
}

func dispatchSteps(sc *godog.ScenarioContext) {
	w := &dispatchWorld{}
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for _, undo := range slices.Backward(w.cleanup) {
			undo()
		}
		return ctx, err
	})
	sc.Step(`^a node with a test handler registered for the channel family "([^"]*)"$`, w.startNode)
	sc.Step(`^a browser connected to the node through a relay on this machine$`, w.connect)
	sc.Step(`^the browser opens the channels "([^"]*)" and "([^"]*)" beside ctl$`, w.openChannels)
	sc.Step(`^the node hands "([^"]*)" to the handler with the browser's connection, and it opens$`, w.handed)
	sc.Step(`^the node closes "([^"]*)"$`, func(label string) error { return w.wait(w.closed[label]) })
	sc.Step(`^the node still reads the browser's hello on ctl$`, w.hello)
}

func (w *dispatchWorld) startNode(family string) error {
	if peer.Channels[family] == nil {
		return fmt.Errorf("nothing registered for %q", family)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	w.ctx = ctx
	w.cleanup = append(w.cleanup, cancel)
	// The relay of the peer steps: its sockets take datagrams only from relay sockets.
	relay := &peersWorld{}
	if err := relay.relayOnly(); err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, relay.cleanup...)
	w.ice, w.browser, w.node = relay.relay, exampleKey("browser"), exampleKey("node")
	w.accepted, w.toPeer = make(chan *peer.Connection, 1), make(chan proto.Envelope, 64)
	socks, err := peer.Listen(0, 0)
	if err != nil {
		return err
	}
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup = append(w.cleanup, func() { socks.Close(); db.Close() })
	w.peer = peer.New(w.node, socks, db, func(id string) bool { return id == w.browser.ID() }, func(string) []peer.Route {
		return []peer.Route{{ID: "local", Send: w.relayed, ICE: w.ice}}
	}, func(c *peer.Connection) { w.accepted <- c })
	for len(handedOver) > 0 {
		<-handedOver
	}
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
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: ice, ICETransportPolicy: webrtc.ICETransportPolicyRelay})
	if err != nil {
		return err
	}
	w.pc, w.opened, w.closed = pc, map[string]chan struct{}{}, map[string]chan struct{}{}
	w.cleanup = append(w.cleanup, func() { pc.Close() })
	if w.ctl, err = w.channel("ctl"); err != nil {
		return err
	}
	conn := randomConn()
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			w.send(proto.ICECandidate{SignalHeader: w.header("ice", conn), Candidate: i.ToJSON().Candidate, MID: i.SDPMid}, nil)
		}
	})
	offer, err := pc.CreateOffer(nil)
	if err == nil {
		err = pc.SetLocalDescription(offer)
	}
	var answer proto.Answer
	err = errors.Join(err, w.send(proto.Offer{SignalHeader: w.header("offer", conn), SDP: offer.SDP}, &answer))
	if err == nil {
		err = pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP})
	}
	if err != nil {
		return err
	}
	go w.candidates()
	if err := w.wait(w.opened["ctl"]); err != nil {
		return err
	}
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case w.served = <-w.accepted:
		return nil
	}
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
// answer, if any, is decoded into answer.
func (w *dispatchWorld) send(payload any, answer *proto.Answer) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var reply []proto.Envelope
	path := peer.Route{ID: "local", Send: func(_ context.Context, _ string, env proto.Envelope) error { reply = append(reply, env); return nil }}
	if err := w.peer.Receive(w.ctx, path, signal(w.browser.Sign(string(data)))); err != nil || answer == nil {
		return err
	}
	if len(reply) != 1 {
		return fmt.Errorf("the node answered %d times", len(reply))
	}
	return proto.Decode([]byte(reply[0].Payload), answer)
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

// channel opens a data channel from the browser and tracks its state.
func (w *dispatchWorld) channel(label string) (*webrtc.DataChannel, error) {
	dc, err := w.pc.CreateDataChannel(label, nil)
	if err != nil {
		return nil, err
	}
	opened, closed := make(chan struct{}), make(chan struct{})
	w.opened[label], w.closed[label] = opened, closed
	dc.OnOpen(func() { close(opened) })
	dc.OnClose(func() { close(closed) })
	return dc, nil
}

func (w *dispatchWorld) openChannels(registered, unknown string) error {
	for _, label := range []string{registered, unknown} {
		if _, err := w.channel(label); err != nil {
			return err
		}
	}
	return nil
}

func (w *dispatchWorld) wait(ch <-chan struct{}) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case <-ch:
		return nil
	}
}

func (w *dispatchWorld) handed(label string) error {
	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case got := <-handedOver:
		if got.label != label || got.conn != w.served || got.conn.Device != w.browser.ID() {
			return fmt.Errorf("handed %q on another connection or for another device", got.label)
		}
	}
	return w.wait(w.opened[label])
}

// hello sends the first message of a session on ctl; the node reads it as sent.
func (w *dispatchWorld) hello() error {
	const hello = `{"t":"hello","id":1,"v":1,"kind":"browser","name":"phone browser","addrs":[]}`
	if err := w.ctl.SendText(hello); err != nil {
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
