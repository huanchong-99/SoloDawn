package peer

import (
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/stun/v4"
	"github.com/pion/webrtc/v4"

	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

const (
	helloTimeout   = 10 * time.Second
	probeOpenLimit = 15 * time.Second
	probeFirstWait = 30 * time.Second
	probeMaxWait   = 5 * time.Minute
)

func init() { offerKinds["probe"] = (*Peer).probeOffer }

func peerICE(paths []Route, probe bool) webrtc.Configuration {
	if !probe {
		return iceConfig(paths)
	}
	var servers []webrtc.ICEServer
	for _, path := range paths {
		servers = appendDirect(servers, path.ICE)
	}
	return webrtc.Configuration{ICEServers: servers}
}

func appendDirect(servers []webrtc.ICEServer, list []proto.ICEServer) []webrtc.ICEServer {
	for _, s := range list {
		if urls := stunURLs(s.URLs); len(urls) > 0 {
			servers = append(servers, webrtc.ICEServer{URLs: urls})
		}
	}
	return servers
}

func stunURLs(all []string) []string {
	var urls []string
	for _, u := range all {
		if uri, err := stun.ParseURI(u); err != nil || !relayed(uri) {
			urls = append(urls, u)
		}
	}
	return urls
}

func (p *Peer) closeProbe(device string) {
	if old := p.probes[device]; old != nil {
		delete(p.probes, device)
		old.Close()
	}
}

func (p *Peer) probeOffer(k link, desc webrtc.SessionDescription, paths []Route) (*proto.Envelope, error) {
	p.closeProbe(k.device)
	return p.answer(k, desc, paths, true)
}

func (p *Peer) acceptInbound(device string, c *Connection, probe bool) {
	if !probe {
		p.slot[device] = c
		c.pc.OnDataChannel(func(dc *webrtc.DataChannel) { p.dispatch(c, dc) })
		return
	}
	var promoted atomic.Bool
	p.probes[device] = c
	context.AfterFunc(c.ctx, func() { p.forgetProbe(device, c) })
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) { p.dispatchProbe(c, dc, &promoted) })
}

func (p *Peer) forgetProbe(device string, c *Connection) {
	c.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probes[device] == c {
		delete(p.probes, device)
	}
}

func (p *Peer) dispatchProbe(c *Connection, dc *webrtc.DataChannel, promoted *atomic.Bool) {
	if promoted.Load() {
		p.dispatch(c, dc)
		return
	}
	if dc.Label() != "ctl" {
		dc.Close()
		return
	}
	c.acceptCtl(dc, func(*Connection) {})
	if c.ownsCtl(dc) {
		dc.OnMessage(func(m webrtc.DataChannelMessage) {
			if m.IsString {
				p.onProbeCtl(c, m.Data, promoted)
			}
		})
	}
}

func (c *Connection) ownsCtl(dc *webrtc.DataChannel) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dc == dc
}

func (p *Peer) onProbeCtl(c *Connection, data []byte, promoted *atomic.Bool) {
	if promoted.Load() {
		c.deliver(data)
		return
	}
	if p.unpromotedCtl(c, data) {
		promoted.Store(true)
	}
}

func (p *Peer) unpromotedCtl(c *Connection, data []byte) bool {
	if !decodeProbeHello(c, data) {
		return false
	}
	if !p.known(c.Device) || !p.promote(c) {
		go c.Close()
		return false
	}
	c.queue(data)
	go p.inbound(c)
	return true
}

func decodeProbeHello(c *Connection, data []byte) bool {
	h, ok := validHelloPacket(data)
	if !ok {
		c.refuse(data)
		return false
	}
	if h.V != 1 {
		_ = c.Send(proto.ErrorResponse{Re: h.ID, Err: proto.WireError{Code: "version", Message: "protocol version is not supported"}})
		return false
	}
	return true
}

func validHelloPacket(data []byte) (h proto.Hello, ok bool) {
	f, err := readFraming(data)
	if err != nil || f.Re != nil || proto.Decode(data, &h) != nil {
		return h, false
	}
	return h, validHelloKind(h)
}

func validHelloKind(h proto.Hello) bool {
	return h.Type == "hello" && (h.Kind == "browser" || h.Kind == "host")
}

func (p *Peer) promote(c *Connection) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probes[c.Device] != c || c.isClosed() {
		return false
	}
	delete(p.probes, c.Device)
	if old := p.slot[c.Device]; old != nil {
		old.Close()
	}
	p.slot[c.Device] = c
	return true
}

// OfferProbe sends a relay-free offer with probe: true and gives ctl 15
// seconds from the sent offer to open (flows/connect.md steps 10 and 11).
func (p *Peer) OfferProbe(ctx context.Context, device string) (*Connection, error) {
	k := link{device, randomID()}
	routes := p.paths(device)
	c, offer, err := p.newProbeOffer(k, routes)
	if err != nil {
		return nil, err
	}
	if err := sendOffer(ctx, c, routes, device, offer); err != nil {
		return nil, err
	}
	go watchProbeOpen(c, probeOpenLimit)
	return c, nil
}

func sendOffer(ctx context.Context, conn *Connection, routes []Route, target string, offer proto.Envelope) error {
	cancelOnCtx := context.AfterFunc(ctx, func() { conn.Close() })
	defer cancelOnCtx()
	err := broadcast(conn.ctx, routes, target, offer)
	if err != nil {
		conn.Close()
		return err
	}
	conn.deadline.Reset(helloTimeout)
	return nil
}

func (p *Peer) newProbeOffer(k link, routes []Route) (*Connection, proto.Envelope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, err := p.create(k, p.sockets.offer, routes, true)
	if err != nil {
		return nil, proto.Envelope{}, err
	}
	sdp := initOfferCtl(c)
	env := p.sign(proto.Offer{SignalHeader: p.header(k, "offer"), SDP: sdp, Probe: true})
	p.waitForAnswer(c, routes)
	return c, env, nil
}

func initOfferCtl(c *Connection) string {
	ctl, err := c.pc.CreateDataChannel("ctl", nil)
	must(err)
	c.bind(ctl, func(*Connection) {})
	desc, err := c.pc.CreateOffer(nil)
	must(err)
	must(c.pc.SetLocalDescription(desc))
	return desc.SDP
}

func watchProbeOpen(c *Connection, limit time.Duration) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-c.Ready():
		return
	case <-c.Done():
		return
	case <-timer.C:
		c.Close()
	}
}

// A Session is one welcomed connection of a Client. Its Read stops as soon as
// a move or a loss retires it (flows/connect.md step 11.3); its Request sends
// unanswered non-sync requests again after the next welcome (steps 12 and 18).
type Session struct {
	client *Client
	c      *Connection
	ctx    context.Context
	retire context.CancelFunc
	done   chan struct{}
	close  func()
}

func newSession(cl *Client, c *Connection) *Session {
	ctx, retire := context.WithCancel(cl.ctx)
	done := make(chan struct{})
	s := &Session{client: cl, c: c, ctx: ctx, retire: retire, done: done, close: sync.OnceFunc(func() { close(done) })}
	context.AfterFunc(c.ctx, retire)
	return s
}

func mergeCancel(a, b context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(a)
	stop := context.AfterFunc(b, cancel)
	return ctx, func() { stop(); cancel() }
}

func (s *Session) Read(ctx context.Context) (json.RawMessage, error) {
	if s.ctx.Err() != nil {
		return nil, io.EOF
	}
	merged, stop := mergeCancel(ctx, s.ctx)
	defer stop()
	msg, err := s.c.Read(merged)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, io.EOF
	}
	return msg, nil
}

func (s *Session) Send(frame any) error {
	if s.ctx.Err() != nil {
		return io.EOF
	}
	return s.c.Send(frame)
}

func (s *Session) Path() string                 { return s.client.pathOf(s.c) }
func (s *Session) PathChanged() <-chan struct{} { return s.c.PathChanged() }
func (s *Session) Done() <-chan struct{}        { return s.done }

func (s *Session) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if !isSync(build) {
		return s.client.Request(ctx, build)
	}
	merged, stop := mergeCancel(ctx, s.ctx)
	defer stop()
	reply, err := s.c.Request(merged, build)
	if err != nil && ctx.Err() == nil {
		return nil, io.EOF
	}
	return reply, err
}

func isSync(build func(int64) any) bool {
	var f struct {
		Type string `json:"t"`
	}
	return json.Unmarshal(encode(build(0)), &f) == nil && f.Type == "sync"
}

type reqResult struct {
	data json.RawMessage
	err  error
}

type pendingReq struct {
	ctx    context.Context
	build  func(int64) any
	reply  chan reqResult
	done   chan struct{}
	active *Connection
}

// A Client keeps one offered direction open, says hello, probes without TURN
// while relayed, moves on welcome, and sends unanswered requests again
// (flows/connect.md sections A, B and C).
type Client struct {
	device      string
	present     <-chan struct{}
	hello       func() proto.Hello
	onWelcome   func(*Session, proto.Welcome)
	offer       func(context.Context, string) (*Connection, error)
	probe       func(context.Context, string) (*Connection, error)
	pathOf      func(*Connection) string
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	pathChanged chan struct{}
	probeNow    chan bool
	sendMu      sync.Mutex
	mu          sync.Mutex
	current     *Session
	moving      bool
	pending     []*pendingReq
}

// Client starts maintaining a connection to device until ctx ends or Close is
// called. onWelcome sets up each welcomed Session before unanswered requests
// are sent again.
func (p *Peer) Client(ctx context.Context, device string, present <-chan struct{}, hello func() proto.Hello, onWelcome func(*Session, proto.Welcome)) *Client {
	return newClient(ctx, device, present, hello, onWelcome, p.Offer, p.OfferProbe, (*Connection).Path)
}

func newClient(
	parent context.Context,
	device string,
	present <-chan struct{},
	hello func() proto.Hello,
	onWelcome func(*Session, proto.Welcome),
	offer func(context.Context, string) (*Connection, error),
	probe func(context.Context, string) (*Connection, error),
	pathOf func(*Connection) string,
) *Client {
	ctx, cancel := context.WithCancel(parent)
	cl := &Client{
		device: device, present: present, hello: hello, onWelcome: onWelcome,
		offer: offer, probe: probe, pathOf: pathOf, ctx: ctx, cancel: cancel,
		done: make(chan struct{}), pathChanged: make(chan struct{}, 1), probeNow: make(chan bool, 1),
	}
	go cl.run()
	return cl
}

func (cl *Client) Close() error {
	cl.cancel()
	<-cl.done
	return nil
}

func (cl *Client) Done() <-chan struct{}        { return cl.done }
func (cl *Client) PathChanged() <-chan struct{} { return cl.pathChanged }

func (cl *Client) Path() string {
	cl.mu.Lock()
	s := cl.current
	cl.mu.Unlock()
	if s == nil {
		return ""
	}
	return cl.pathOf(s.c)
}

func (cl *Client) Send(frame any) error {
	cl.mu.Lock()
	s := cl.current
	cl.mu.Unlock()
	if s == nil {
		return io.EOF
	}
	return s.Send(frame)
}

// Probe starts an immediate probe attempt on the current connection.
func (cl *Client) Probe() { sendBool(cl.probeNow, true) }

func sendBool(ch chan<- bool, v bool) {
	select {
	case ch <- v:
		return
	default:
		return
	}
}

func (cl *Client) notifyPath() {
	select {
	case cl.pathChanged <- struct{}{}:
		return
	default:
		return
	}
}

func (cl *Client) run() {
	defer close(cl.done)
	defer cl.retireCurrent()
	delay := time.Second
	for cl.ctx.Err() == nil {
		s, opened := cl.openSession()
		var open time.Duration
		if s != nil {
			open = cl.serve(s, opened)
		}
		if cl.ctx.Err() != nil {
			return
		}
		delay = backoff(cl.ctx, cl.present, after(delay, open))
	}
}

func (cl *Client) openSession() (*Session, time.Time) {
	c, err := cl.offer(cl.ctx, cl.device)
	if err != nil {
		return nil, time.Time{}
	}
	stop := context.AfterFunc(cl.ctx, func() { c.Close() })
	defer stop()
	if !waitConnReady(c) {
		return nil, time.Time{}
	}
	opened := time.Now()
	welcome, ok := sayHello(cl.ctx, c, cl.hello)
	if !ok {
		return nil, time.Time{}
	}
	return cl.adopt(c, welcome), opened
}

func waitConnReady(c *Connection) bool {
	select {
	case <-c.Done():
		return false
	case <-c.Ready():
		return true
	}
}

func sayHello(ctx context.Context, c *Connection, hello func() proto.Hello) (w proto.Welcome, ok bool) {
	hCtx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	raw, err := c.Request(hCtx, func(id int64) any { return buildHello(hello(), id) })
	if err != nil || proto.Decode(raw, &w) != nil || !validWelcome(w) {
		c.Close()
		return w, false
	}
	return w, true
}

func validWelcome(w proto.Welcome) bool {
	return w.Type == "welcome" && w.V == 1
}

func buildHello(h proto.Hello, id int64) proto.Hello {
	h.Request = proto.Request{Type: "hello", ID: id}
	if h.V == 0 {
		h.V = 1
	}
	if h.Addrs == nil {
		h.Addrs = []string{}
	}
	return h
}

func (cl *Client) adopt(c *Connection, welcome proto.Welcome) *Session {
	next := newSession(cl, c)
	cl.mu.Lock()
	prev := cl.current
	cl.current = next
	cl.moving = false
	cl.mu.Unlock()
	if prev != nil {
		prev.retire()
		prev.c.Close()
	}
	cl.notifyPath()
	cl.onWelcome(next, welcome)
	if prev != nil {
		prev.close()
	}
	cl.flushPending()
	return next
}

func (cl *Client) retireCurrent() {
	cl.mu.Lock()
	prev := cl.current
	cl.current = nil
	cl.moving = false
	cl.mu.Unlock()
	if prev != nil {
		prev.retire()
		prev.c.Close()
		prev.close()
	}
}

func (cl *Client) setMoving(v bool) {
	cl.mu.Lock()
	cl.moving = v
	cl.mu.Unlock()
}

type serveState struct {
	s           *Session
	opened      time.Time
	wait        time.Duration
	scheduled   bool
	timer       *time.Timer
	pending     *Connection
	probeResult <-chan bool
}

func (cl *Client) serve(s *Session, opened time.Time) time.Duration {
	st := &serveState{
		s: s, opened: opened, wait: probeFirstWait,
		timer: time.AfterFunc(time.Hour, func() { sendBool(cl.probeNow, false) }),
	}
	st.timer.Stop()
	st.syncPath(cl.pathOf(s.c))
	for !cl.stepServe(st) {
	}
	return time.Since(st.opened)
}

func (cl *Client) stepServe(st *serveState) bool {
	select {
	case <-st.s.ctx.Done():
		return cl.endServe(st)
	case <-st.s.c.PathChanged():
		cl.notifyPath()
		st.syncPath(cl.pathOf(st.s.c))
		return false
	case forced := <-cl.probeNow:
		st.triggerProbe(cl, forced)
		return false
	case opened := <-st.probeResult:
		return cl.finishProbe(st, opened)
	}
}

func (st *serveState) triggerProbe(cl *Client, forced bool) {
	if !forced && (!st.scheduled || cl.pathOf(st.s.c) != "relay") {
		return
	}
	st.startProbe(cl)
}

func (cl *Client) endServe(st *serveState) bool {
	st.stop()
	cl.retireCurrent()
	cl.notifyPath()
	return true
}

func (st *serveState) syncPath(path string) {
	if path == "relay" {
		st.armIfIdle()
		return
	}
	st.stop()
	st.wait = probeFirstWait
}

func (st *serveState) armIfIdle() {
	if !st.scheduled && st.pending == nil {
		st.scheduled = true
		st.timer.Reset(st.wait)
	}
}

func (st *serveState) stop() {
	st.timer.Stop()
	st.scheduled = false
	if st.pending != nil {
		st.pending.Close()
		st.pending = nil
		st.probeResult = nil
	}
}

func (st *serveState) startProbe(cl *Client) {
	st.stop()
	c, err := cl.probe(cl.ctx, cl.device)
	if err != nil {
		st.scheduleNext(cl.pathOf(st.s.c))
		return
	}
	res := make(chan bool, 1)
	st.pending = c
	st.probeResult = res
	go awaitProbeOpen(c, res)
}

func awaitProbeOpen(c *Connection, res chan<- bool) {
	select {
	case <-c.Ready():
		res <- true
	case <-c.Done():
		res <- false
	}
}

func (st *serveState) scheduleNext(path string) {
	st.wait = min(2*st.wait, probeMaxWait)
	if path == "relay" {
		st.armIfIdle()
	}
}

func (cl *Client) finishProbe(st *serveState, opened bool) bool {
	cand := st.pending
	st.pending = nil
	st.probeResult = nil
	if !opened {
		cand.Close()
		st.scheduleNext(cl.pathOf(st.s.c))
		return false
	}
	return cl.move(st, cand)
}

func (cl *Client) move(st *serveState, cand *Connection) bool {
	cl.setMoving(true)
	now := time.Now()
	welcome, ok := sayHello(cl.ctx, cand, cl.hello)
	if ok {
		st.s = cl.adopt(cand, welcome)
		st.opened = now
		st.wait = probeFirstWait
		st.syncPath(cl.pathOf(st.s.c))
		return false
	}
	cl.setMoving(false)
	if cl.ctx.Err() != nil || st.s.c.isClosed() {
		return cl.endServe(st)
	}
	cl.flushPending()
	st.scheduleNext(cl.pathOf(st.s.c))
	return false
}

func (cl *Client) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if len(encode(build(math.MaxInt64))) > ctlLimit {
		return nil, errors.New("ctl frame exceeds 60 KiB")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, ok := cl.enqueueReq(ctx, build)
	if !ok {
		return nil, io.EOF
	}
	stopReq := context.AfterFunc(ctx, func() { cl.finishReq(req, reqResult{err: ctx.Err()}) })
	defer stopReq()
	stopClient := context.AfterFunc(cl.ctx, func() { cl.finishReq(req, reqResult{err: io.EOF}) })
	defer stopClient()
	cl.flushPending()
	res := <-req.reply
	return res.data, res.err
}

func (cl *Client) enqueueReq(ctx context.Context, build func(int64) any) (*pendingReq, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.ctx.Err() != nil || (cl.current == nil && !cl.moving) {
		return nil, false
	}
	req := &pendingReq{ctx: ctx, build: build, reply: make(chan reqResult, 1), done: make(chan struct{})}
	cl.pending = append(cl.pending, req)
	return req, true
}

func (cl *Client) flushPending() {
	cl.sendMu.Lock()
	defer cl.sendMu.Unlock()
	c, todo := cl.takeUnsent()
	for _, req := range todo {
		cl.flushOne(c, req)
	}
}

func (cl *Client) takeUnsent() (*Connection, []*pendingReq) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.current == nil || cl.current.c.isClosed() {
		return nil, nil
	}
	c := cl.current.c
	var todo []*pendingReq
	for _, req := range cl.pending {
		if needsSend(req) {
			req.active = c
			todo = append(todo, req)
		}
	}
	return c, todo
}

func needsSend(req *pendingReq) bool {
	return (req.active == nil || req.active.isClosed()) && req.ctx.Err() == nil
}

func (cl *Client) flushOne(c *Connection, req *pendingReq) {
	id, ch := c.reserve()
	if err := c.Send(req.build(id)); err != nil {
		c.unwait(id)
		go c.Close()
		cl.releaseReq(c, req)
		return
	}
	go cl.awaitReq(c, id, ch, req)
}

func (c *Connection) unwait(id int64) {
	c.mu.Lock()
	delete(c.waiting, id)
	c.mu.Unlock()
}

func (cl *Client) awaitReq(c *Connection, id int64, ch <-chan json.RawMessage, req *pendingReq) {
	defer c.unwait(id)
	select {
	case msg := <-ch:
		cl.finishReq(req, reqResult{data: msg})
	case <-c.Done():
		cl.releaseReq(c, req)
	case <-req.done:
		return
	}
}

func (cl *Client) releaseReq(c *Connection, req *pendingReq) {
	cl.mu.Lock()
	if req.active == c {
		req.active = nil
	}
	retry := cl.current != nil && cl.current.c != c && !cl.current.c.isClosed()
	cl.mu.Unlock()
	if retry {
		go cl.flushPending()
	}
}

func (cl *Client) finishReq(req *pendingReq, res reqResult) {
	cl.mu.Lock()
	idx := slices.Index(cl.pending, req)
	if idx >= 0 {
		cl.pending = slices.Delete(cl.pending, idx, idx+1)
	}
	cl.mu.Unlock()
	if idx >= 0 {
		close(req.done)
		req.reply <- res
	}
}
