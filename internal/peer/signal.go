package peer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/webrtc/v4"
)

type link struct{ device, conn string }

type Peer struct {
	key     identity.Key
	sockets *Sockets
	db      *store.Store
	known   func(string) bool
	paths   func(string) []Route
	inbound func(*Connection)
	now     func() time.Time
	mu      sync.Mutex // decisions and description/candidate application, never network writes
	live    map[link]*Connection
	slot    map[string]*Connection
	early   map[link]*batch
	retired map[link]int64
}

func New(key identity.Key, sockets *Sockets, db *store.Store, known func(string) bool, paths func(string) []Route, inbound func(*Connection)) *Peer {
	return &Peer{key: key, sockets: sockets, db: db, known: known, paths: paths, inbound: inbound, now: time.Now, live: make(map[link]*Connection), slot: make(map[string]*Connection), early: make(map[link]*batch), retired: make(map[link]int64)}
}
func randomID() string {
	var b [16]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func (p *Peer) header(k link, kind string) proto.SignalHeader {
	return proto.SignalHeader{PayloadHeader: proto.PayloadHeader{Type: kind, Nonce: proto.ID(randomID()), Timestamp: p.now().UnixMilli(), To: k.device}, Conn: k.conn}
}
func (p *Peer) sign(msg any) proto.Envelope { return p.key.Sign(string(encode(msg))) }

func broadcast(ctx context.Context, paths []Route, to string, env proto.Envelope) error {
	if len(paths) == 0 {
		return errors.New("no signalling path")
	}
	results := make(chan error, len(paths))
	for _, path := range paths {
		go func() { results <- path.Send(ctx, to, env) }()
	}
	var err error
	for range paths {
		if err = <-results; err == nil {
			return nil
		}
	}
	return err
}

// Receive is shared by every transport, including node-local delivery. The
// outer sender must match the signer, and every copy is independently checked.
func (p *Peer) Receive(ctx context.Context, path Route, frame proto.Route) error {
	now := p.now()
	env, hdr, err := p.open(frame, now)
	if err != nil {
		return err
	}
	answer, err := p.apply(link{env.Key, hdr.Conn}, hdr.Type, []byte(env.Payload), now.UnixMilli())
	if err != nil || answer == nil {
		return err
	}
	return path.Send(ctx, env.Key, *answer)
}
func (p *Peer) open(frame proto.Route, now time.Time) (env proto.Envelope, hdr proto.SignalHeader, err error) {
	if proto.Decode(frame.Env, &env) != nil || frame.Key != env.Key {
		return env, hdr, identity.BadSig
	}
	hdr.PayloadHeader, err = identity.OpenSignal(env, p.key.ID(), p.known, now)
	if err != nil {
		return env, hdr, err
	}
	var id struct {
		Conn proto.ID `json:"conn"`
	}
	if proto.Decode([]byte(env.Payload), &id) != nil {
		return env, hdr, identity.BadRequest
	}
	hdr.Conn = string(id.Conn)
	return env, hdr, nil
}
func (p *Peer) apply(k link, kind string, data []byte, now int64) (*proto.Envelope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(now)
	switch kind {
	case "offer":
		return p.receiveOffer(k, data, now)
	case "answer":
		return nil, p.receiveAnswer(k, data)
	case "ice":
		return nil, p.receiveCandidate(k, data, now)
	default:
		return nil, identity.BadRequest
	}
}
func (p *Peer) prune(now int64) {
	for k, b := range p.early {
		if b.expires < now {
			delete(p.early, k)
		}
	}
	for k, until := range p.retired {
		if until < now {
			delete(p.retired, k)
		}
	}
}

func (p *Peer) receiveOffer(k link, data []byte, now int64) (*proto.Envelope, error) {
	var o proto.Offer
	if proto.Decode(data, &o) != nil || o.Probe {
		return nil, identity.BadRequest
	}
	if c := p.live[k]; c != nil {
		if c.isClosed() {
			return nil, nil
		}
		return c.answer, nil
	}
	return p.firstOffer(k, o, now)
}
func (p *Peer) firstOffer(k link, o proto.Offer, now int64) (*proto.Envelope, error) {
	desc := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: o.SDP}
	if _, err := desc.Unmarshal(); err != nil {
		return nil, identity.BadRequest
	}
	first, err := p.db.RememberOffer(k.device, k.conn, o.Timestamp, now)
	if err != nil || !first {
		return nil, err
	}
	if err := p.closeInbound(k.device); err != nil {
		return nil, err
	}
	return p.answer(k, desc)
}
func (p *Peer) closeInbound(device string) error {
	if old := p.slot[device]; old != nil {
		return old.Close()
	}
	return nil
}
func (p *Peer) answer(k link, desc webrtc.SessionDescription) (*proto.Envelope, error) {
	c, err := p.create(k, p.sockets.answer)
	if err != nil {
		return nil, err
	}
	p.slot[k.device] = c
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) { c.acceptCtl(dc, p.inbound) })
	answer, err := c.makeAnswer(desc)
	if err != nil {
		c.Close()
		return nil, err
	}
	env := p.sign(proto.Answer{SignalHeader: p.header(k, "answer"), SDP: answer.SDP})
	c.answer = &env
	return &env, nil
}
func (c *Connection) makeAnswer(desc webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := c.remote(desc); err != nil {
		return webrtc.SessionDescription{}, err
	}
	answer, err := c.pc.CreateAnswer(nil)
	if err == nil {
		err = c.pc.SetLocalDescription(answer)
	}
	return answer, err
}

func (p *Peer) receiveAnswer(k link, data []byte) error {
	var a proto.Answer
	if proto.Decode(data, &a) != nil {
		return identity.BadRequest
	}
	c := p.live[k]
	if c == nil || c.answer != nil || c.candidates.ready {
		return nil
	}
	return c.remote(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: a.SDP})
}
func (c *Connection) remote(desc webrtc.SessionDescription) error {
	if err := c.pc.SetRemoteDescription(desc); err != nil {
		return identity.BadRequest
	}
	return c.candidates.flush(c.pc.AddICECandidate)
}

// Offer starts gathering without waiting for it to finish. Ready signals when
// ctl opens; a closed Done before Ready means negotiation failed.
func (p *Peer) Offer(ctx context.Context, device string) (*Connection, error) {
	k := link{device, randomID()}
	c, offer, err := p.makeOffer(k)
	if err != nil {
		return nil, err
	}
	if err := broadcast(ctx, p.paths(device), device, offer); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}
func (p *Peer) makeOffer(k link) (*Connection, proto.Envelope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, err := p.create(k, p.sockets.offer)
	if err != nil {
		return nil, proto.Envelope{}, err
	}
	dc, err := c.pc.CreateDataChannel("ctl", nil)
	if err != nil {
		c.Close()
		return nil, proto.Envelope{}, err
	}
	c.bind(dc, func(*Connection) {})
	offer, err := c.pc.CreateOffer(nil)
	if err == nil {
		err = c.pc.SetLocalDescription(offer)
	}
	if err != nil {
		c.Close()
		return nil, proto.Envelope{}, err
	}
	return c, p.sign(proto.Offer{SignalHeader: p.header(k, "offer"), SDP: offer.SDP}), nil
}

func (p *Peer) create(k link, api *webrtc.API) (*Connection, error) {
	pc, err := api.NewPeerConnection(iceConfig(p.paths(k.device)))
	if err != nil {
		return nil, err
	}
	c := newConnection(pc, k.device)
	c.candidates = p.take(k)
	p.live[k] = c
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i != nil {
			msg := proto.ICECandidate{SignalHeader: p.header(k, "ice"), Candidate: i.ToJSON().Candidate, MID: i.SDPMid}
			_ = broadcast(c.ctx, p.paths(k.device), k.device, p.sign(msg))
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			go c.Close()
		}
	})
	go p.forget(k, c)
	return c, nil
}

func (p *Peer) forget(k link, c *Connection) {
	<-c.Done()
	c.Close() // Wait for Pion to release the mux registration before freeing the slot.
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.live, k)
	if p.slot[k.device] == c {
		delete(p.slot, k.device)
	}
	p.retired[k] = p.now().Add(2 * time.Minute).UnixMilli()
}
