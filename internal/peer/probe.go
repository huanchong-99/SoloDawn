package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/pion/stun/v4"
	"github.com/pion/webrtc/v4"
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

// offerProbe sends a relay-free offer with probe: true and gives ctl 15
// seconds from the sent offer to open (flows/connect.md steps 10 and 11).
func (p *Peer) offerProbe(ctx context.Context, device string) (*Connection, error) {
	c, err := p.startOffer(ctx, device, true)
	if err != nil {
		return nil, err
	}
	go watchProbeOpen(c, probeOpenLimit)
	return c, nil
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
	client      *Client
	c           *Connection
	ctx         context.Context
	retire      context.CancelFunc
	pathChanged chan struct{} // read only by the caller of Session.PathChanged
	done        chan struct{} // closed once when the Session is retired
	close       func()
}

func newSession(cl *Client, c *Connection) *Session {
	ctx, retire := context.WithCancel(cl.ctx)
	done := make(chan struct{})
	s := &Session{
		client: cl, c: c, ctx: ctx, retire: retire,
		pathChanged: make(chan struct{}, 1), done: done,
		close: sync.OnceFunc(func() { close(done) }),
	}
	context.AfterFunc(c.ctx, retire)
	return s
}

func mergeCancel(a, b context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(a)
	stop := context.AfterFunc(b, cancel)
	return ctx, func() { stop(); cancel() }
}

// Read returns the next incoming request or push on s, or io.EOF once s has
// been retired by a move or connection loss.
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

// Send writes a push or response on s while s is active, or returns io.EOF
// once s has been retired.
func (s *Session) Send(frame any) error {
	if s.ctx.Err() != nil {
		return io.EOF
	}
	return s.c.Send(frame)
}

// Path reports "direct", "relay", or "" for s's underlying connection.
func (s *Session) Path() string { return s.client.pathOf(s.c) }

// PathChanged returns a channel signalled whenever s.Path may have changed.
// It is distinct from Client.PathChanged and the Client's own serve loop, so
// a single reader on s observes every signal.
func (s *Session) PathChanged() <-chan struct{} { return s.pathChanged }

// Done is closed once s has been retired.
func (s *Session) Done() <-chan struct{} { return s.done }

// Request sends the request built by build and returns its response. A sync
// request belongs to s alone and ends with io.EOF if s ends; any other request
// is sent again under a fresh id if a move or reconnect intervenes
// (flows/connect.md steps 12 and 18). build must be pure: type and size checks
// call it with synthetic ids before each wire send.
func (s *Session) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if isSync(build) {
		return s.syncRequest(ctx, build)
	}
	return s.client.request(ctx, build)
}

func (s *Session) syncRequest(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if err := preflightReq(ctx, build); err != nil {
		return nil, err
	}
	if s.ctx.Err() != nil {
		return nil, io.EOF
	}
	merged, stop := mergeCancel(ctx, s.ctx)
	defer stop()
	reply, err := s.c.Request(merged, build)
	if err != nil && ctx.Err() == nil {
		return nil, io.EOF
	}
	return reply, err
}

func preflightReq(ctx context.Context, build func(int64) any) error {
	if len(encode(build(math.MaxInt64))) > ctlLimit {
		return fmt.Errorf("ctl frame exceeds 60 KiB")
	}
	return ctx.Err()
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
	build  func(int64) any
	reply  chan reqResult // read only by Client.request
	done   chan struct{}  // closed once by finishReq
	active *Connection
}

// A Client keeps one offered direction open, says hello, probes without TURN
// while relayed, moves on welcome, and sends unanswered non-sync requests
// again (flows/connect.md sections A, B and C).
type Client struct {
	device      string
	present     <-chan struct{} // read only by Client.run via backoff
	hello       func() proto.Hello
	onWelcome   func(*Session, proto.Welcome)
	offer       func(context.Context, string) (*Connection, error)
	probe       func(context.Context, string) (*Connection, error)
	pathOf      func(*Connection) string
	firstProbe  time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{} // closed once when Client.run returns
	pathChanged chan struct{} // read only by the caller of Client.PathChanged
	sendMu      sync.Mutex
	mu          sync.Mutex
	current     *Session
	pending     []*pendingReq
}

// Client starts maintaining a connection to device until ctx ends or Close is
// called. onWelcome sets up each welcomed Session before unanswered non-sync
// requests are sent again.
func (p *Peer) Client(ctx context.Context, device string, present <-chan struct{}, hello func() proto.Hello, onWelcome func(*Session, proto.Welcome)) *Client {
	return newClient(ctx, device, present, hello, onWelcome, p.Offer, p.offerProbe, (*Connection).Path, probeFirstWait)
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
	firstProbe time.Duration,
) *Client {
	ctx, cancel := context.WithCancel(parent)
	cl := &Client{
		device: device, present: present, hello: hello, onWelcome: onWelcome,
		offer: offer, probe: probe, pathOf: pathOf, firstProbe: firstProbe,
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		pathChanged: make(chan struct{}, 1),
	}
	go cl.run()
	return cl
}

// Close stops reconnects and probes, closes the current Session if any, and
// waits until the Client loop has exited.
func (cl *Client) Close() error {
	cl.cancel()
	<-cl.done
	return nil
}

// Done is closed once the Client has stopped.
func (cl *Client) Done() <-chan struct{} { return cl.done }

// PathChanged returns a channel signalled whenever Client.Path may have
// changed. Only one caller should read from it.
func (cl *Client) PathChanged() <-chan struct{} { return cl.pathChanged }

// Path reports the current Session's path ("direct", "relay", or ""), or ""
// while no Session is active.
func (cl *Client) Path() string {
	cl.mu.Lock()
	s := cl.current
	cl.mu.Unlock()
	if s == nil {
		return ""
	}
	return cl.pathOf(s.c)
}

// Send writes a push or response on the current Session, or returns io.EOF
// while no Session is active.
func (cl *Client) Send(frame any) error {
	cl.mu.Lock()
	s := cl.current
	cl.mu.Unlock()
	if s == nil {
		return io.EOF
	}
	return s.Send(frame)
}

func notify(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
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
	if err != nil {
		c.Close()
		return w, false
	}
	w, ok = decodeWelcome(raw)
	if !ok {
		c.Close()
	}
	return w, ok
}

func decodeWelcome(raw json.RawMessage) (w proto.Welcome, ok bool) {
	if proto.Decode(raw, &w) != nil {
		return w, false
	}
	return w, w.Type == "welcome" && w.V == 1
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
	cl.mu.Unlock()
	if prev != nil {
		prev.retire()
		prev.c.Close()
	}
	notify(cl.pathChanged)
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
	cl.mu.Unlock()
	if prev != nil {
		prev.retire()
		prev.c.Close()
		prev.close()
	}
}

type serveState struct {
	s           *Session
	opened      time.Time
	wait        time.Duration
	scheduled   bool
	timer       *time.Timer // C read only by stepServe
	pending     *Connection
	probeResult <-chan bool // read only by stepServe
}

func (cl *Client) serve(s *Session, opened time.Time) time.Duration {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	st := &serveState{
		s: s, opened: opened, wait: cl.firstProbe,
		timer: timer,
	}
	st.syncPath(cl.pathOf(s.c))
	for !cl.stepServe(st) {
	}
	return time.Since(st.opened)
}

func (cl *Client) stepServe(st *serveState) bool {
	select {
	case <-st.s.ctx.Done():
		return cl.endServe(st)
	case <-st.s.c.PathChanged(): // Connection.pathChanged is read only here while served
		notify(st.s.pathChanged)
		notify(cl.pathChanged)
		st.syncPath(cl.pathOf(st.s.c))
		return false
	case <-st.timer.C:
		st.startProbe(cl)
		return false
	case opened := <-st.probeResult:
		return cl.finishProbe(st, opened)
	}
}

func (cl *Client) endServe(st *serveState) bool {
	st.stop()
	cl.retireCurrent()
	notify(cl.pathChanged)
	return true
}

func (st *serveState) syncPath(path string) {
	if path == "relay" {
		st.armIfIdle()
		return
	}
	st.stop()
	st.wait = st.s.client.firstProbe
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
	now := time.Now()
	welcome, ok := sayHello(cl.ctx, cand, cl.hello)
	if ok {
		st.s = cl.adopt(cand, welcome)
		st.opened = now
		st.wait = cl.firstProbe
		st.syncPath(cl.pathOf(st.s.c))
		return false
	}
	if cl.ctx.Err() != nil || st.s.c.isClosed() {
		return cl.endServe(st)
	}
	st.scheduleNext(cl.pathOf(st.s.c))
	return false
}

// Request sends the request built by build on the current Session. A sync
// request runs only on the Session open now and is never replayed; any other
// request is sent again under a fresh id after the next welcome if the
// connection moves or reconnects (flows/connect.md steps 12 and 18). build
// must be pure: type and size checks call it with synthetic ids before each
// wire send.
func (cl *Client) Request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if isSync(build) {
		cl.mu.Lock()
		s := cl.current
		cl.mu.Unlock()
		if s == nil {
			return nil, io.EOF
		}
		return s.syncRequest(ctx, build)
	}
	return cl.request(ctx, build)
}

func (cl *Client) request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	if err := preflightReq(ctx, build); err != nil {
		return nil, err
	}
	req, ok := cl.enqueueReq(build)
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

func (cl *Client) enqueueReq(build func(int64) any) (*pendingReq, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.ctx.Err() != nil || cl.current == nil {
		return nil, false
	}
	req := &pendingReq{build: build, reply: make(chan reqResult, 1), done: make(chan struct{})}
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
		if req.active == nil || req.active.isClosed() {
			req.active = c
			todo = append(todo, req)
		}
	}
	return c, todo
}

func (cl *Client) flushOne(c *Connection, req *pendingReq) {
	id, ch := c.reserve()
	if err := c.Send(req.build(id)); err != nil {
		c.unwait(id)
		c.Close()
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
		return
	case <-req.done:
		return
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
