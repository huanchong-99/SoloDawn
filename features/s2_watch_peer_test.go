//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/webrtc/v4"
)

type s2TerminalPeer struct {
	dispatchWorld
	path     *websocket.Conn
	ctl      chan json.RawMessage
	key      identity.Key
	target   string
	connID   string
	seq      int64
	earlyICE []webrtc.ICECandidateInit
	ice      proto.ICEConfig
}

func openS2TerminalPeer(c *s2Cast, person *s2Person, browser *peerBrowser, target *s2Node, withTURN bool) (*s2TerminalPeer, error) {
	ctx, cancel := context.WithCancel(c.Ctx)
	browser.path.Close()
	p := &s2TerminalPeer{key: browser.key, target: target.ID(), connID: randomConn(), ctl: make(chan json.RawMessage, 128)}
	p.dispatchWorld = dispatchWorld{ctx: ctx, cleanup: c.addUndo, channels: map[string]*webrtc.DataChannel{},
		opened: map[string]chan struct{}{}, closed: map[string]chan struct{}{}, received: make(chan []byte, 32)}
	c.addUndo(cancel)
	if err := p.dial(c.RVURL); err != nil {
		return nil, err
	}
	if err := p.connect(p.config(withTURN)); err != nil {
		return nil, err
	}
	if err := p.checkPath(false); err != nil {
		return nil, err
	}
	return p, p.helloPerson(person)
}

func (p *s2TerminalPeer) checkPath(relay bool) error {
	pair, err := p.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil {
		return err
	}
	if pair == nil {
		return fmt.Errorf("terminal connection has no selected ICE pair")
	}
	if got := pair.Local.Typ == webrtc.ICECandidateTypeRelay || pair.Remote.Typ == webrtc.ICECandidateTypeRelay; got != relay {
		return fmt.Errorf("expected selected pair with relay=%t, got %v", relay, pair)
	}
	return nil
}

func (p *s2TerminalPeer) dial(url string) error {
	ws, _, err := websocket.Dial(p.ctx, url, nil)
	if err != nil {
		return err
	}
	p.path = ws
	p.cleanup(func() { ws.CloseNow() })
	var challenge proto.Challenge
	if err := wsjson.Read(p.ctx, ws, &challenge); err != nil {
		return err
	}
	auth := proto.Auth{Type: "auth", Key: p.key.ID(), Signature: p.key.SignChallenge(challenge.Nonce)}
	if err := wsjson.Write(p.ctx, ws, auth); err != nil {
		return err
	}
	return wsjson.Read(p.ctx, ws, &p.ice)
}

func (p *s2TerminalPeer) config(withTURN bool) webrtc.Configuration {
	var cfg webrtc.Configuration
	if withTURN {
		for _, server := range p.ice.ICE {
			cfg.ICEServers = append(cfg.ICEServers, webrtc.ICEServer{URLs: server.URLs, Username: server.Username, Credential: server.Credential})
		}
	}
	return cfg
}

func (p *s2TerminalPeer) connect(cfg webrtc.Configuration) error {
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return err
	}
	p.pc = pc
	p.cleanup(func() { pc.Close() })
	if err := p.channel("ctl"); err != nil {
		return err
	}
	p.channels["ctl"].OnMessage(func(m webrtc.DataChannelMessage) {
		select {
		case p.ctl <- json.RawMessage(m.Data):
		case <-p.ctx.Done():
		}
	})
	if err := p.offer(); err != nil {
		return err
	}
	return p.waitForCtl()
}

func (p *s2TerminalPeer) waitForCtl() error {
	failed := make(chan error, 1)
	go func() {
		failed <- p.readSignals()
		p.pc.Close()
	}()
	select {
	case err := <-failed:
		return fmt.Errorf("terminal signaling: %w", err)
	case <-p.opened["ctl"]:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *s2TerminalPeer) offer() error {
	p.pc.OnICECandidate(p.sendCandidate)
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := p.sendSignal(proto.Offer{SignalHeader: p.signalHeader("offer"), SDP: offer.SDP}); err != nil {
		return err
	}
	return p.pc.SetLocalDescription(offer)
}

func (p *s2TerminalPeer) sendCandidate(i *webrtc.ICECandidate) {
	if i == nil {
		return
	}
	_ = p.sendSignal(proto.ICECandidate{SignalHeader: p.signalHeader("ice"), Candidate: i.ToJSON().Candidate, MID: i.SDPMid})
}

func (p *s2TerminalPeer) signalHeader(kind string) proto.SignalHeader {
	return proto.SignalHeader{PayloadHeader: proto.PayloadHeader{Type: kind, Nonce: proto.NewNonce(),
		Timestamp: time.Now().UnixMilli(), To: p.target}, Conn: p.connID}
}

func (p *s2TerminalPeer) sendSignal(payload any) error {
	env := p.key.Sign(string(mustMarshalNoEscape(payload)))
	return wsjson.Write(p.ctx, p.path, proto.Route{Type: "to", Key: p.target, Env: mustMarshalNoEscape(env)})
}

func (p *s2TerminalPeer) readSignals() error {
	for {
		var route proto.Route
		if err := wsjson.Read(p.ctx, p.path, &route); err != nil {
			return err
		}
		if route.Type == "from" && route.Key == p.target {
			if err := p.applySignal(route.Env); err != nil {
				return err
			}
		}
	}
}

func (p *s2TerminalPeer) applySignal(raw json.RawMessage) error {
	var env proto.Envelope
	var header proto.SignalHeader
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if err := proto.Decode([]byte(env.Payload), &header); err != nil || header.Conn != p.connID {
		return err
	}
	return p.applyDescription(header.Type, []byte(env.Payload))
}

func (p *s2TerminalPeer) applyDescription(kind string, raw []byte) error {
	switch kind {
	case "answer":
		var answer proto.Answer
		if err := proto.Decode(raw, &answer); err != nil {
			return err
		}
		return p.applyAnswer(answer.SDP)
	case "ice":
		var ice proto.ICECandidate
		if err := proto.Decode(raw, &ice); err != nil {
			return err
		}
		candidate := webrtc.ICECandidateInit{Candidate: ice.Candidate, SDPMid: &ice.MID}
		if p.pc.RemoteDescription() == nil {
			p.earlyICE = append(p.earlyICE, candidate)
			return nil
		}
		return p.pc.AddICECandidate(candidate)
	}
	return nil
}

func (p *s2TerminalPeer) applyAnswer(sdp string) error {
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}); err != nil {
		return err
	}
	for _, candidate := range p.earlyICE {
		if err := p.pc.AddICECandidate(candidate); err != nil {
			return err
		}
	}
	p.earlyICE = nil
	return nil
}

func (p *s2TerminalPeer) send(frame any) error {
	return p.channels["ctl"].SendText(string(mustMarshalNoEscape(frame)))
}

func (p *s2TerminalPeer) request(frame map[string]any) (json.RawMessage, error) {
	p.seq++
	frame["id"] = p.seq
	if err := p.send(frame); err != nil {
		return nil, err
	}
	return p.nextCtl(func(raw json.RawMessage) bool {
		var r struct {
			Re int64 `json:"re"`
		}
		return json.Unmarshal(raw, &r) == nil && r.Re == p.seq
	})
}

func (p *s2TerminalPeer) nextCtl(match func(json.RawMessage) bool) (json.RawMessage, error) {
	for {
		select {
		case raw := <-p.ctl:
			if match(raw) {
				return raw, nil
			}
		case <-p.closed["ctl"]:
			return nil, fmt.Errorf("ctl closed")
		case <-p.ctx.Done():
			return nil, p.ctx.Err()
		}
	}
}

func (p *s2TerminalPeer) helloPerson(person *s2Person) error {
	frame := map[string]any{"t": "hello", "v": 2, "kind": "browser", "name": person.Name + " browser", "addrs": []string{}}
	raw, err := p.request(frame)
	if err != nil {
		return err
	}
	var welcome proto.Welcome
	if err := proto.Decode(raw, &welcome); err != nil {
		return fmt.Errorf("hello replied %s: %w", raw, err)
	}
	return p.send(proto.Devlog{Type: "devlog", User: person.ID(), Entries: person.Entries})
}

func (p *s2TerminalPeer) command(fields map[string]any, want string) error {
	fields["n"], fields["ts"] = proto.NewNonce(), time.Now().UnixMilli()
	env := p.key.Sign(string(mustMarshalNoEscape(fields)))
	raw, err := p.request(map[string]any{"t": "cmd", "env": env})
	if err != nil {
		return err
	}
	return s2WatchReply(raw, want)
}

func (p *s2TerminalPeer) terminalBytes(want string) (string, error) {
	var text []byte
	for {
		select {
		case data := <-p.received:
			text = append(text, data...)
			if i := strings.Index(string(text), want); i >= 0 {
				return string(text[:i+len(want)]), nil
			}
		case <-p.ctx.Done():
			return "", fmt.Errorf("terminal waiting for %q in %q: %w", want, text, p.ctx.Err())
		}
	}
}
