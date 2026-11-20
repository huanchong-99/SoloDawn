// Package peer opens authenticated WebRTC data channels between devices.
package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const signalLimit = 16 * 1024

func encode(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

// Route is one applicable signalling path (a WebSocket or node-local delivery).
type Route struct {
	Send func(context.Context, string, proto.Envelope) error
	ICE  []proto.ICEServer
}

type Path struct {
	ws     *websocket.Conn
	mu     sync.Mutex
	config proto.ICEConfig
}

// Dial authenticates this device before returning any route or ICE servers.
func Dial(ctx context.Context, url string, key identity.Key) (*Path, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(signalLimit)
	p := &Path{ws: ws}
	if err := p.handshake(ctx, key); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *Path) Close() error { return p.ws.CloseNow() }

func (p *Path) handshake(ctx context.Context, key identity.Key) error {
	var challenge proto.Challenge
	if err := p.expect(ctx, "challenge", &challenge); err != nil {
		return err
	}
	auth := proto.Auth{Type: "auth", Key: key.ID(), Signature: key.SignChallenge(challenge.Nonce)}
	if err := p.Write(ctx, auth); err != nil {
		return err
	}
	if err := p.expect(ctx, "ready", &p.config); err != nil {
		return err
	}
	return validateTTL(p.config.TTL)
}

func (p *Path) expect(ctx context.Context, kind string, dest any) error {
	t, data, err := p.read(ctx)
	if err != nil {
		return err
	}
	if t != kind {
		return errors.New("unexpected signalling frame: " + t)
	}
	return proto.Decode(data, dest)
}

func (p *Path) read(ctx context.Context) (string, []byte, error) {
	typ, data, err := p.ws.Read(ctx)
	if err != nil {
		return "", nil, err
	}
	if typ != websocket.MessageText || !utf8.Valid(data) {
		return "", nil, errors.New("signalling frame is not text")
	}
	var frame proto.RVFrame
	err = proto.Decode(data, &frame)
	return frame.Type, data, err
}

func (p *Path) Write(ctx context.Context, frame any) error {
	data := encode(frame)
	if len(data) > signalLimit {
		return errors.New("signalling frame exceeds 16 KiB")
	}
	return p.ws.Write(ctx, websocket.MessageText, data)
}

func (p *Path) Route() Route {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Route{p.send, p.config.ICE}
}
func (p *Path) send(ctx context.Context, to string, env proto.Envelope) error {
	return p.Write(ctx, proto.Route{Type: "to", Key: to, Env: encode(env)})
}

// Serve reads this path until closure. Bad envelopes are dropped in isolation;
// non-signalling events (presence, pairing, quota) belong to the caller.
func (p *Path) Serve(ctx context.Context, node *Peer, event func(string, []byte) error) error {
	timer := time.AfterFunc(p.lifetime(), func() { _ = p.Write(ctx, proto.RVFrame{Type: "ice"}) })
	defer timer.Stop()
	for {
		kind, data, err := p.read(ctx)
		if err != nil {
			return err
		}
		if err := p.handle(ctx, kind, data, node, event, timer); err != nil {
			return err
		}
	}
}
func (p *Path) handle(ctx context.Context, kind string, data []byte, node *Peer, event func(string, []byte) error, timer *time.Timer) error {
	switch kind {
	case "from":
		return p.from(ctx, node, data)
	case "ice":
		if err := p.refresh(data); err != nil {
			return err
		}
		timer.Reset(p.lifetime())
		return nil
	default:
		return event(kind, data)
	}
}
func (p *Path) from(ctx context.Context, node *Peer, data []byte) error {
	var f proto.Route
	if err := proto.Decode(data, &f); err != nil {
		return err
	}
	err := node.Receive(ctx, p.Route(), f)
	var rejected identity.Code
	if errors.As(err, &rejected) {
		return nil
	}
	return err
}
func (p *Path) refresh(data []byte) error {
	var c proto.ICEConfig
	if err := proto.Decode(data, &c); err != nil {
		return err
	}
	if err := validateTTL(c.TTL); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = c
	return nil
}
func validateTTL(ttl int) error {
	if ttl < 1 || int64(ttl) > int64((1<<63-1)/time.Second) {
		return errors.New("invalid ICE credential lifetime")
	}
	return nil
}
func (p *Path) lifetime() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Duration(p.config.TTL) * time.Second
}
