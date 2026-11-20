package peer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"maps"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/pion/webrtc/v4"
)

type link struct{ device, conn string }

// A Peer answers and offers the connections of one device.
//
// Lock order, for the node that wires it: the Peer holds its own lock only
// around its own decisions, and never while it calls known, paths, inbound or
// a channel handler, or sends on a Route. known and paths run on the caller's
// goroutine during Receive and Offer, and paths and Route.Send also on pion's
// goroutines; inbound and the handlers run on pion's. A callback may call back
// into the Peer, but a caller of Receive, Offer or Connect must not hold a
// lock that known or paths takes.
type Peer struct {
	key      identity.Key
	sockets  *Sockets
	db       *store.Store
	known    func(string) bool
	paths    func(string) []Route
	inbound  func(*Connection)
	channels Channels
	now      func() time.Time
	mu       sync.Mutex // decisions and description/candidate application, never network writes
	live     map[link]*Connection
	slot     map[string]*Connection
	early    map[link]*batch
	retired  map[link]int64
}

// New keeps its own copy of channels: what the caller changes later does not
// reach it.
func New(key identity.Key, sockets *Sockets, db *store.Store, known func(string) bool, paths func(string) []Route, inbound func(*Connection), channels Channels) *Peer {
	return &Peer{key: key, sockets: sockets, db: db, known: known, paths: paths, inbound: inbound, channels: maps.Clone(channels), now: time.Now, live: make(map[link]*Connection), slot: make(map[string]*Connection), early: make(map[link]*batch), retired: make(map[link]int64)}
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
// An answer is sent on path from a goroutine of its own, after Receive returns.
func (p *Peer) Receive(ctx context.Context, path Route, frame proto.Route) error {
	now := p.now()
	env, hdr, err := p.open(frame, now)
	if err != nil {
		return err
	}
	paths := p.paths(env.Key) // before the lock: the callback may take locks of its own
	answer, err := p.apply(link{env.Key, hdr.Conn}, hdr.Type, []byte(env.Payload), now.UnixMilli(), paths)
	if err != nil || answer == nil {
		return err
	}
	// The answer waits for its turn on the path, as candidates do, while
	// the path's reader goes on.
	go path.Send(ctx, env.Key, *answer)
	return nil
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
func (p *Peer) apply(k link, kind string, data []byte, now int64, paths []Route) (*proto.Envelope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(now)
	switch kind {
	case "offer":
		return p.receiveOffer(k, data, now, paths)
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

func (p *Peer) receiveOffer(k link, data []byte, now int64, paths []Route) (*proto.Envelope, error) {
	var o proto.Offer
	if proto.Decode(data, &o) != nil {
		return nil, identity.BadRequest
	}
	kind := offerKinds[kindOf(o)]
	if kind == nil {
		return nil, identity.BadRequest
	}
	if c := p.live[k]; c != nil {
		if c.isClosed() {
			return nil, nil
		}
		return c.answer, nil
	}
	return p.firstOffer(k, o, now, kind, paths)
}
func (p *Peer) firstOffer(k link, o proto.Offer, now int64, kind offerKind, paths []Route) (*proto.Envelope, error) {
	desc := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: o.SDP}
	if _, err := desc.Unmarshal(); err != nil {
		return nil, identity.BadRequest
	}
	first, err := p.db.RememberOffer(k.device, k.conn, o.Timestamp, now)
	if err != nil || !first {
		return nil, err
	}
	return kind(p, k, desc, paths)
}

func init() { offerKinds["ordinary"] = (*Peer).ordinaryOffer }

// ordinaryOffer closes the signer's earlier inbound connection before the new
// checks start (contracts/rendezvous.md section 2.3).
func (p *Peer) ordinaryOffer(k link, desc webrtc.SessionDescription, paths []Route) (*proto.Envelope, error) {
	if old := p.slot[k.device]; old != nil {
		// Close returns once pion has released the old mux registration; its
		// error concerns only the old connection.
		old.Close()
	}
	return p.answer(k, desc, paths)
}
func (p *Peer) answer(k link, desc webrtc.SessionDescription, paths []Route) (*proto.Envelope, error) {
	c, err := p.create(k, p.sockets.answer, paths)
	if err != nil {
		return nil, err
	}
	p.slot[k.device] = c
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) { p.dispatch(c, dc) })
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
	if c == nil || c.pending == nil || c.isClosed() {
		return nil
	}
	return c.remote(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: a.SDP})
}
func (c *Connection) remote(desc webrtc.SessionDescription) error {
	if err := c.pc.SetRemoteDescription(desc); err != nil {
		return identity.BadRequest
	}
	c.endWait()
	c.candidates.flush(c.addCandidate)
	return nil
}

// addCandidate applies a remote candidate; one the stack refuses is dropped.
func (c *Connection) addCandidate(i webrtc.ICECandidateInit) {
	c.pc.AddICECandidate(i)
	c.reclassify()
}

// Offer starts gathering without waiting for it to finish. Ready signals when
// ctl opens; a closed Done before Ready means negotiation failed.
func (p *Peer) Offer(ctx context.Context, device string) (*Connection, error) {
	k := link{device, randomID()}
	paths := p.paths(device)
	c, offer, err := p.makeOffer(k, paths)
	if err != nil {
		return nil, err
	}
	// ctx ends the attempt while the offer is being sent; the copies on slower
	// paths go on for as long as the connection lives.
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	if err := broadcast(c.ctx, paths, device, offer); err != nil {
		c.Close()
		return nil, err
	}
	c.deadline.Reset(10 * time.Second)
	return c, nil
}
func (p *Peer) makeOffer(k link, paths []Route) (*Connection, proto.Envelope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, err := p.create(k, p.sockets.offer, paths)
	if err != nil {
		return nil, proto.Envelope{}, err
	}
	// A new connection refuses none of these.
	dc, err := c.pc.CreateDataChannel("ctl", nil)
	must(err)
	c.bind(dc, func(*Connection) {})
	offer, err := c.pc.CreateOffer(nil)
	must(err)
	must(c.pc.SetLocalDescription(offer))
	env := p.sign(proto.Offer{SignalHeader: p.header(k, "offer"), SDP: offer.SDP})
	p.waitForAnswer(c, paths)
	return c, env, nil
}

// create makes a connection with the ICE servers of paths, a snapshot taken
// before the peer lock.
func (p *Peer) create(k link, api *webrtc.API, paths []Route) (*Connection, error) {
	pc, err := api.NewPeerConnection(iceConfig(paths))
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
	pc.SCTP().Transport().ICETransport().OnSelectedCandidatePairChange(func(*webrtc.ICECandidatePair) { c.reclassify() })
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
	c.endWait()
	if p.slot[k.device] == c {
		delete(p.slot, k.device)
	}
	p.retired[k] = p.now().Add(2 * time.Minute).UnixMilli()
}
