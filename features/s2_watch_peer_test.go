//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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
	net     *m7NetControl
	saw     trail // the frames read from ctl
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
	pc, err := p.peerConnection(withTURN)
	if err != nil {
		return nil, err
	}
	if err := p.connectRTC(pc, p.signalHeader, p.routeSignal); err != nil {
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

// peerConnection has the M7 browser's sockets, on 127.0.0.1 only: a firewall may refuse other addresses.
func (p *s2TerminalPeer) peerConnection(withTURN bool) (*webrtc.PeerConnection, error) {
	b := newM7Browser(p.ctx, p.key, p.target, m7CandDirect, "", nil)
	if withTURN {
		turn := strings.TrimSuffix(strings.TrimPrefix(p.ice.ICE[0].URLs[0], "turn:"), "?transport=udp")
		b = newM7Browser(p.ctx, p.key, p.target, m7CandSrflxAndRelay, turn, p.ice.ICE)
	}
	p.net = b.ctrl
	return b.buildPeerConnection(net.IPv4(127, 0, 0, 1))
}

// moveToRelay makes ICE renominate. Pion rates a pair that answers no more by its last round trip, and a fast dead direct pair
// above a relayed one, so the direct path first becomes slow and is then cut. Pion also rates a round trip of 0 as none
// measured, below the slow direct pair; the relay's round trip on 127.0.0.1 reads 0 on Windows, whose clock moves in steps
// of about 0.5 ms, so the relay is given 5 ms.
func (p *s2TerminalPeer) moveToRelay() error {
	p.net.relayNs.Store(int64(5 * time.Millisecond))
	p.net.delayNs.Store(int64(3 * time.Second))
	if err := waitUntilCtx(p.ctx, 40*time.Millisecond, "the direct round trip to be measured as slow", p.directIsSlow); err != nil {
		return err
	}
	p.net.relayOnly.Store(true)
	return waitUntilCtx(p.ctx, 40*time.Millisecond, "the connection to select the TURN relay", func() error { return p.checkPath(true) })
}

func (p *s2TerminalPeer) directIsSlow() error {
	stats, ok := p.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePairStats()
	if !ok || stats.CurrentRoundTripTime < 3 {
		return fmt.Errorf("selected pair ok=%t, round trip %.3f s", ok, stats.CurrentRoundTripTime)
	}
	return nil
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
	return p.nextCtl(fmt.Sprintf("the answer to %v", frame["t"]), func(raw json.RawMessage) bool {
		var r struct {
			Re int64 `json:"re"`
		}
		return json.Unmarshal(raw, &r) == nil && r.Re == p.seq
	})
}

// waitClosed waits for the node to close the channel label.
func (p *s2TerminalPeer) waitOpened(label, what string) error {
	_, err := recvCtx(p.ctx, p.opened[label], what, &p.saw)
	return err
}

func (p *s2TerminalPeer) waitClosed(label, what string) error {
	_, err := recvCtx(p.ctx, p.closed[label], what, &p.saw)
	return err
}

func (p *s2TerminalPeer) nextCtl(what string, match func(json.RawMessage) bool) (json.RawMessage, error) {
	start := time.Now()
	for {
		select {
		case raw := <-p.ctl:
			p.saw.see(frameBrief(raw))
			if match(raw) {
				return raw, nil
			}
		case <-p.closed["ctl"]:
			return nil, fmt.Errorf("ctl closed")
		case <-p.ctx.Done():
			return nil, ctxEnded(p.ctx, start, what, &p.saw)
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
