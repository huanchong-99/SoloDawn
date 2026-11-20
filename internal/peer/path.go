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
	"github.com/pion/stun/v4"
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
	config proto.ICEConfig // its ICE holds only the entries pion accepts
	renew  *time.Timer     // set by Serve; asks for fresh ICE servers
	pace   pacer
}

// pacer keeps one rendezvous connection at 90 frames in any 10 seconds, well
// below the 120 at which the server closes it (contracts/rendezvous.md
// section 2), so that a node answering many reconnects at once stays
// connected. A frame over the limit waits for its turn.
type pacer struct {
	mu   sync.Mutex // held from a frame's turn until it is written: frames queue in order
	sent [90]time.Time
	next int
}

// turn waits until the frame 90 places back is 10 seconds old, then counts
// this one. A frame given up on, before or while it waits, is not counted.
func (q *pacer) turn(ctx context.Context) error {
	wait := time.NewTimer(time.Until(q.sent[q.next].Add(10 * time.Second)))
	defer wait.Stop()
	select {
	case <-ctx.Done():
	case <-wait.C:
	}
	if err := ctx.Err(); err != nil {
		return err
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
	p.config.ICE = usable(p.config.ICE)
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
	// The library closes the whole connection when the context of a write
	// ends; only Close may end this path.
	return p.ws.Write(context.WithoutCancel(ctx), websocket.MessageText, data)
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
	p.startRenewal(ctx)
	defer p.renew.Stop()
	for {
		kind, data, err := p.read(ctx)
		if err != nil {
			return err
		}
		if err := p.handle(ctx, kind, data, node, event); err != nil {
			return err
		}
	}
}

// startRenewal asks for fresh ICE servers when the current ones are half
// their ttl old (contracts/rendezvous.md section 2.1).
func (p *Path) startRenewal(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.renew = time.AfterFunc(p.half(), func() { p.ask(ctx) })
}

// ask requests fresh ICE servers, and again every 10 seconds until they
// arrive: an answer can be lost to a full queue at the server. A request that
// cannot be written means the path has ended, and the asking stops.
func (p *Path) ask(ctx context.Context) {
	p.mu.Lock()
	p.renew.Reset(10 * time.Second)
	p.mu.Unlock()
	if p.Write(ctx, proto.RVFrame{Type: "ice"}) != nil {
		p.renew.Stop()
	}
}
func (p *Path) handle(ctx context.Context, kind string, data []byte, node *Peer, event func(string, []byte) error) error {
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
		return p.refresh(data)
	}
	return event(kind, data)
}
func (p *Path) from(ctx context.Context, node *Peer, data []byte) error {
	var f proto.Route
	if err := proto.Decode(data, &f); err != nil {
		return err
	}
	// A signal the node refuses or cannot take is dropped; the path goes on.
	node.Receive(ctx, p.Route(), f)
	return nil
}
func (p *Path) refresh(data []byte) error {
	var c proto.ICEConfig
	if err := proto.Decode(data, &c); err != nil {
		return err
	}
	if err := validateTTL(c.TTL); err != nil {
		return err
	}
	c.ICE = usable(c.ICE)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = c
	p.renew.Reset(p.half())
	return nil
}
func validateTTL(ttl int) error {
	if ttl < 1 || int64(ttl) > int64(math.MaxInt64/time.Second) {
		return errors.New("invalid ICE credential lifetime")
	}
	return nil
}

// half is half the ttl of the current ICE servers; p.mu is held.
func (p *Path) half() time.Duration {
	return time.Duration(p.config.TTL) * time.Second / 2
}

// usable keeps the entries that pion accepts, so that one bad entry from one
// path breaks neither the other entries nor the other paths.
func usable(servers []proto.ICEServer) []proto.ICEServer {
	var ok []proto.ICEServer
	for _, s := range servers {
		if accepted(s) {
			ok = append(ok, s)
		}
	}
	return ok
}

// accepted is pion's own check of an entry: every URL parses, and a TURN
// URL comes with a username (iceserver.go of pion/webrtc).
func accepted(s proto.ICEServer) bool {
	for _, u := range s.URLs {
		uri, err := stun.ParseURI(u)
		if err != nil || relayed(uri) && s.Username == "" {
			return false
		}
	}
	return true
}
func relayed(uri *stun.URI) bool {
	return uri.Scheme == stun.SchemeTypeTURN || uri.Scheme == stun.SchemeTypeTURNS
}
