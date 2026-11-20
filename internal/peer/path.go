// Package peer opens authenticated WebRTC data channels between devices.
package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const signalLimit = 16_384 // 16 KiB

func encode(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	must(enc.Encode(v))
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

// must stops the node on an error that only a programming mistake can cause.
func must(err error) {
	if err != nil {
		panic(err)
	}
}

// Route is one applicable signalling path (a WebSocket or node-local delivery).
type Route struct {
	ID   string // Stable for the life of one signalling path, including local delivery.
	Send func(context.Context, string, proto.Envelope) error
	ICE  []proto.ICEServer
}

type Path struct {
	id     string
	ws     *websocket.Conn
	mu     sync.Mutex
	config proto.ICEConfig
	pace   pacer
}

// pacer keeps one rendezvous connection under 100 frames in any 10 seconds,
// below the 120 at which the server closes it (contracts/rendezvous.md
// section 2), so that a node answering many reconnects at once stays
// connected. A frame over the limit waits for its turn.
type pacer struct {
	mu   sync.Mutex // held from a frame's turn until it is written: frames queue in order
	sent [99]time.Time
	next int
}

// turn waits until the frame 99 places back is 10 seconds old, then counts
// this one. A frame given up on is not counted.
func (q *pacer) turn(ctx context.Context) error {
	wait := time.NewTimer(time.Until(q.sent[q.next].Add(10 * time.Second)))
	defer wait.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait.C:
	}
	q.sent[q.next] = time.Now()
	q.next = (q.next + 1) % len(q.sent)
	return nil
}

// Dial authenticates this device before returning any route or ICE servers.
func Dial(ctx context.Context, url string, key identity.Key) (*Path, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(signalLimit)
	p := &Path{id: randomID(), ws: ws}
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
	// A write that fails leaves the connection unusable: reading ready reports it.
	p.Write(ctx, proto.Auth{Type: "auth", Key: key.ID(), Signature: key.SignChallenge(challenge.Nonce)})
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
	p.pace.mu.Lock()
	defer p.pace.mu.Unlock()
	if err := p.pace.turn(ctx); err != nil {
		return err
	}
	return p.ws.Write(ctx, websocket.MessageText, data)
}

func (p *Path) Route() Route {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Route{ID: p.id, Send: p.send, ICE: p.config.ICE}
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
	case "offline":
		var f proto.Offline
		if err := proto.Decode(data, &f); err != nil {
			return err
		}
		node.Offline(p.id, f.Key)
	case "ice":
		if err := p.refresh(data); err != nil {
			return err
		}
		timer.Reset(p.lifetime())
		return nil
	}
	return event(kind, data)
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
	if ttl < 1 || int64(ttl) > int64(math.MaxInt64/time.Second) {
		return errors.New("invalid ICE credential lifetime")
	}
	return nil
}
func (p *Path) lifetime() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Duration(p.config.TTL) * time.Second
}
