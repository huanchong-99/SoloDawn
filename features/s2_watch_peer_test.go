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
	path    *websocket.Conn
	key     identity.Key
	target  string
	connID  string
	seq     int64
	answers chan proto.Envelope
	ice     proto.ICEConfig
}

func openS2TerminalPeer(c *s2Cast, person *s2Person, key identity.Key, target *s2Node, withTURN bool) (*s2TerminalPeer, error) {
	ctx, cancel := context.WithCancel(c.Ctx)
	p := &s2TerminalPeer{key: key, target: target.ID(), connID: randomConn(), answers: make(chan proto.Envelope, 1)}
	p.dispatchWorld = dispatchWorld{ctx: ctx, cleanup: c.addUndo, toPeer: make(chan proto.Envelope, 64)}
	c.addUndo(cancel)
	if err := p.dial(c.RVURL); err != nil {
		return nil, err
	}
	go func() {
		defer cancel()
		p.readSignals()
	}()
	if err := p.connectRTC(p.config(withTURN), p.signalHeader, p.routeSignal); err != nil {
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

func (p *s2TerminalPeer) signalHeader(kind string) proto.SignalHeader {
	return proto.SignalHeader{PayloadHeader: proto.PayloadHeader{Type: kind, Nonce: proto.NewNonce(),
		Timestamp: time.Now().UnixMilli(), To: p.target}, Conn: p.connID}
}

func (p *s2TerminalPeer) routeSignal(payload any, answer *proto.Answer) error {
	env := p.key.Sign(string(mustMarshalNoEscape(payload)))
	if err := wsjson.Write(p.ctx, p.path, proto.Route{Type: "to", Key: p.target, Env: mustMarshalNoEscape(env)}); err != nil {
		return err
	}
	if answer == nil {
		return nil
	}
	env, err := recvCtx(p.ctx, p.answers, "the peer's answer", nil)
	if err != nil {
		return err
	}
	return proto.Decode([]byte(env.Payload), answer)
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
	switch header.Type {
	case "answer":
		p.answers <- env
	case "ice":
		p.toPeer <- env
	}
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
	var saw trail
	start := time.Now()
	for {
		select {
		case raw := <-p.ctl:
			if match(raw) {
				return raw, nil
			}
			saw.see(frameBrief(raw))
		case <-p.closed["ctl"]:
			return nil, fmt.Errorf("ctl closed")
		case <-p.ctx.Done():
			return nil, ctxEnded(p.ctx, start, "a matching frame on ctl", &saw)
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
	start := time.Now()
	for {
		select {
		case data := <-p.received:
			text = append(text, data...)
			if i := strings.Index(string(text), want); i >= 0 {
				return string(text[:i+len(want)]), nil
			}
		case <-p.ctx.Done():
			var saw trail
			saw.see(tailOf(string(text)))
			return "", ctxEnded(p.ctx, start, fmt.Sprintf("%q in the terminal", want), &saw)
		}
	}
}
