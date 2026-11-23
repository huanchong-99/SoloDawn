//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/rendezvous"
	"github.com/pion/ice/v4"
	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
)

func init() {
	stepSets["m7"] = m7Steps
}

const (
	m7GroupName   = "Website"
	m7CodexThread = "019a7f10-4b2c-7aa1-9d34-70f1b2c3d4e5"
)

type m7CandMode int

const (
	m7CandDirect m7CandMode = iota
	m7CandRelayOnly
	m7CandSrflxAndRelay
)

type m7NetControl struct {
	turnAddr  string
	cut       atomic.Bool
	relayOnly atomic.Bool
}

func (c *m7NetControl) drops(addr net.Addr) bool {
	if c.cut.Load() {
		return true
	}
	if c.relayOnly.Load() {
		return addr.String() != c.turnAddr
	}
	return !addr.(*net.UDPAddr).IP.IsLoopback()
}

type m7UDPConn struct {
	*net.UDPConn
	ctrl *m7NetControl
}

func (u *m7UDPConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if u.ctrl.drops(addr) {
		return len(p), nil
	}
	return u.UDPConn.WriteTo(p, addr)
}

func (u *m7UDPConn) WriteToUDP(p []byte, addr *net.UDPAddr) (int, error) {
	return u.WriteTo(p, addr)
}

func (u *m7UDPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := u.UDPConn.ReadFrom(p)
		if err != nil || !u.ctrl.drops(addr) {
			return n, addr, err
		}
	}
}

func (u *m7UDPConn) ReadFromUDP(p []byte) (int, *net.UDPAddr, error) {
	for {
		n, addr, err := u.UDPConn.ReadFromUDP(p)
		if err != nil || !u.ctrl.drops(addr) {
			return n, addr, err
		}
	}
}

type m7TransportNet struct {
	*stdnet.Net
	ctrl *m7NetControl
}

func (n *m7TransportNet) ListenPacket(network, address string) (net.PacketConn, error) {
	c, err := n.Net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}
	if uc, ok := c.(*net.UDPConn); ok {
		return &m7UDPConn{UDPConn: uc, ctrl: n.ctrl}, nil
	}
	return c, nil
}

func (n *m7TransportNet) ListenUDP(network string, locAddr *net.UDPAddr) (transport.UDPConn, error) {
	uc, err := n.Net.ListenUDP(network, locAddr)
	if err != nil {
		return nil, err
	}
	if raw, ok := uc.(*net.UDPConn); ok {
		return &m7UDPConn{UDPConn: raw, ctrl: n.ctrl}, nil
	}
	return uc, nil
}

type m7PTY struct {
	dc     *webrtc.DataChannel
	opened chan struct{} // closed once by dc.OnOpen
	closed chan struct{} // closed once by dc.OnClose
	notify chan struct{} // single reader: waitOutput
	mu     sync.Mutex
	buf    bytes.Buffer
}

func newM7PTY(dc *webrtc.DataChannel) *m7PTY {
	p := &m7PTY{
		dc:     dc,
		opened: make(chan struct{}),
		closed: make(chan struct{}),
		notify: make(chan struct{}, 1),
	}
	dc.OnOpen(func() { close(p.opened) })
	dc.OnClose(func() {
		close(p.closed)
		p.signalNotify()
	})
	dc.OnMessage(func(m webrtc.DataChannelMessage) { p.appendData(m.Data) })
	return p
}

func (p *m7PTY) appendData(data []byte) {
	p.mu.Lock()
	p.buf.Write(data)
	p.mu.Unlock()
	p.signalNotify()
}

func (p *m7PTY) signalNotify() {
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

func (p *m7PTY) text() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}

func (p *m7PTY) waitOpen(ctx context.Context) error {
	select {
	case <-p.opened:
		return nil
	case <-p.closed:
		return errors.New("pty channel closed before opening")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *m7PTY) waitClosed(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.closed:
		return nil
	case <-timer.C:
		return fmt.Errorf("pty channel did not close within %s", timeout)
	}
}

func (p *m7PTY) waitOutput(timeout time.Duration, sub string) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if strings.Contains(p.text(), sub) {
			return nil
		}
		if !p.waitStep(timer.C) {
			return fmt.Errorf("timed out waiting for %q in pty output %q", sub, p.text())
		}
	}
}

func (p *m7PTY) waitStep(timeout <-chan time.Time) bool {
	select {
	case <-p.notify:
		return true
	case <-timeout:
		return false
	}
}

type m7Browser struct {
	ctx        context.Context
	key        identity.Key
	nodeID     string
	ctrl       *m7NetControl
	mode       m7CandMode
	ice        []proto.ICEServer
	pc         *webrtc.PeerConnection
	ctl        *webrtc.DataChannel
	ctlOpened  chan struct{}           // closed once by ctl.OnOpen
	ctlClosed  chan struct{}           // closed once by ctl.OnClose
	answers    chan proto.Answer       // single reader: awaitAnswer
	remoteICE  chan proto.ICECandidate // single reader: applyRemoteICE
	cutDirect  chan struct{}
	connID     string
	offersSent atomic.Int32

	mu       sync.Mutex
	nextID   int64
	waiting  map[int64]chan json.RawMessage // each channel's single reader: awaitReqReply
	changed  chan struct{}                  // closed under mu by applyRows; reader: m7WaitChan
	aiByName map[string]proto.AIMemberRow
	tasks    map[string]proto.TaskRow
	msgs     map[string]proto.MsgRow
	msgOrder []string
}

func newM7Browser(ctx context.Context, key identity.Key, nodeID string, mode m7CandMode, turnAddr string, iceServers []proto.ICEServer) *m7Browser {
	return &m7Browser{
		ctx:       ctx,
		key:       key,
		nodeID:    nodeID,
		ctrl:      &m7NetControl{turnAddr: turnAddr},
		mode:      mode,
		ice:       iceServers,
		ctlOpened: make(chan struct{}),
		ctlClosed: make(chan struct{}),
		answers:   make(chan proto.Answer, 4),
		remoteICE: make(chan proto.ICECandidate, 64),
		cutDirect: make(chan struct{}),
		waiting:   map[int64]chan json.RawMessage{},
		changed:   make(chan struct{}),
		aiByName:  map[string]proto.AIMemberRow{},
		tasks:     map[string]proto.TaskRow{},
		msgs:      map[string]proto.MsgRow{},
	}
}

func m7KeepCandidate(mode m7CandMode, cand string) bool {
	switch mode {
	case m7CandRelayOnly:
		return strings.Contains(cand, " typ relay")
	case m7CandSrflxAndRelay:
		return strings.Contains(cand, " typ srflx") || strings.Contains(cand, " typ relay")
	default:
		return !strings.Contains(cand, " typ relay")
	}
}

func (b *m7Browser) connect(cleanup func(func()), nodePeer *peer.Peer, relayIP net.IP) error {
	pc, err := b.buildPeerConnection(relayIP)
	if err != nil {
		return err
	}
	b.pc = pc
	cleanup(func() { _ = pc.Close() })
	if err := b.initCtlChannel(); err != nil {
		return err
	}
	b.connID = randomConn()
	b.wireLocalCandidates(nodePeer)
	if err := b.exchangeSDP(nodePeer); err != nil {
		return err
	}
	go b.applyRemoteICE()
	return b.waitCtlOpen()
}

func (b *m7Browser) buildPeerConnection(relayIP net.IP) (*webrtc.PeerConnection, error) {
	baseNet, err := stdnet.NewNet()
	if err != nil {
		return nil, err
	}
	var se webrtc.SettingEngine
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	se.SetIncludeLoopbackCandidate(true)
	se.SetIPFilter(net.IP.IsLoopback)
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	se.SetNet(&m7TransportNet{Net: baseNet, ctrl: b.ctrl})
	se.SetRemoteIPFilter(func(ip net.IP) bool { return ip.IsLoopback() || ip.Equal(relayIP) })
	if b.mode == m7CandSrflxAndRelay {
		if err := se.SetICERenomination(webrtc.WithRenominationInterval(100 * time.Millisecond)); err != nil {
			return nil, err
		}
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	return api.NewPeerConnection(m7WebRTCConfig(b.ice, b.mode))
}

func m7WebRTCConfig(servers []proto.ICEServer, mode m7CandMode) webrtc.Configuration {
	var out []webrtc.ICEServer
	for _, s := range servers {
		out = append(out, webrtc.ICEServer{URLs: s.URLs, Username: s.Username, Credential: s.Credential})
	}
	cfg := webrtc.Configuration{ICEServers: out}
	switch mode {
	case m7CandRelayOnly:
		cfg.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	case m7CandSrflxAndRelay:
		cfg.ICETransportPolicy = webrtc.ICETransportPolicyNoHost
	}
	return cfg
}

func (b *m7Browser) initCtlChannel() error {
	dc, err := b.pc.CreateDataChannel("ctl", nil)
	if err != nil {
		return err
	}
	b.ctl = dc
	dc.OnOpen(func() { close(b.ctlOpened) })
	dc.OnClose(func() { close(b.ctlClosed) })
	dc.OnMessage(func(m webrtc.DataChannelMessage) {
		if m.IsString {
			b.onCtlFrame(m.Data)
		}
	})
	return nil
}

func (b *m7Browser) signalHeader(kind string) proto.SignalHeader {
	return proto.SignalHeader{
		PayloadHeader: proto.PayloadHeader{
			Type:      kind,
			Nonce:     proto.ID(randomConn()),
			Timestamp: time.Now().UnixMilli(),
			To:        b.nodeID,
		},
		Conn: b.connID,
	}
}

func (b *m7Browser) nodeReplyRoute() peer.Route {
	return peer.Route{
		ID:   "m7-local",
		ICE:  m7NodeICEServers(b.ice, b.mode),
		Send: b.receiveFromNode,
	}
}

func m7NodeICEServers(servers []proto.ICEServer, mode m7CandMode) []proto.ICEServer {
	if mode == m7CandSrflxAndRelay && len(servers) > 1 {
		return servers[:1]
	}
	return servers
}

func (b *m7Browser) receiveFromNode(_ context.Context, _ string, env proto.Envelope) error {
	var hdr struct {
		Type string `json:"t"`
	}
	if proto.Decode([]byte(env.Payload), &hdr) != nil {
		return nil
	}
	if hdr.Type == "answer" {
		b.queueAnswer(env.Payload)
	}
	if hdr.Type == "ice" {
		b.queueRemoteICE(env.Payload)
	}
	return nil
}

func (b *m7Browser) queueAnswer(payload string) {
	var ans proto.Answer
	if proto.Decode([]byte(payload), &ans) == nil {
		select {
		case b.answers <- ans:
		default:
		}
	}
}

func (b *m7Browser) queueRemoteICE(payload string) {
	var cand proto.ICECandidate
	if proto.Decode([]byte(payload), &cand) != nil || !b.keepRemoteCandidate(cand.Candidate) {
		return
	}
	select {
	case b.remoteICE <- cand:
	default:
	}
}

func (b *m7Browser) keepRemoteCandidate(candidate string) bool {
	return m7KeepCandidate(b.mode, candidate) || b.mode == m7CandSrflxAndRelay && strings.Contains(candidate, " typ host")
}

func (b *m7Browser) sendSignal(nodePeer *peer.Peer, payload any) error {
	raw, err := marshalNoEscape(payload)
	if err != nil {
		return err
	}
	return nodePeer.Receive(b.ctx, b.nodeReplyRoute(), signal(b.key.Sign(string(raw))))
}

func (b *m7Browser) wireLocalCandidates(nodePeer *peer.Peer) {
	b.pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		if !m7KeepCandidate(b.mode, init.Candidate) {
			return
		}
		_ = b.sendSignal(nodePeer, proto.ICECandidate{
			SignalHeader: b.signalHeader("ice"),
			Candidate:    init.Candidate,
			MID:          c.SDPMid,
		})
	})
}

func (b *m7Browser) exchangeSDP(nodePeer *peer.Peer) error {
	offer, err := b.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := b.pc.SetLocalDescription(offer); err != nil {
		return err
	}
	b.offersSent.Add(1)
	err = b.sendSignal(nodePeer, proto.Offer{SignalHeader: b.signalHeader("offer"), SDP: offer.SDP})
	if err != nil {
		return err
	}
	ans, err := b.awaitAnswer()
	if err != nil {
		return err
	}
	return b.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: ans.SDP})
}

func (b *m7Browser) awaitAnswer() (proto.Answer, error) {
	select {
	case ans := <-b.answers:
		return ans, nil
	case <-b.ctx.Done():
		return proto.Answer{}, b.ctx.Err()
	}
}

func (b *m7Browser) applyRemoteICE() {
	cut := b.cutDirect
	var held []proto.ICECandidate
	for {
		select {
		case c := <-b.remoteICE:
			if b.holdHostICE(c, cut) {
				held = append(held, c)
			} else {
				b.addICE(c)
			}
		case <-cut:
			cut = nil
			b.addHeldICE(held)
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *m7Browser) addHeldICE(held []proto.ICECandidate) {
	for _, c := range held {
		b.addICE(c)
	}
}

func (b *m7Browser) holdHostICE(c proto.ICECandidate, cut <-chan struct{}) bool {
	return b.mode == m7CandSrflxAndRelay && cut != nil && strings.Contains(c.Candidate, " typ host")
}

func (b *m7Browser) addICE(c proto.ICECandidate) {
	_ = b.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: c.Candidate, SDPMid: &c.MID})
}

func (b *m7Browser) waitCtlOpen() error {
	select {
	case <-b.ctlOpened:
		return nil
	case <-b.ctlClosed:
		return errors.New("ctl closed before opening")
	case <-b.ctx.Done():
		return b.ctx.Err()
	}
}

func (b *m7Browser) onCtlFrame(data []byte) {
	var hdr struct {
		Re   *int64 `json:"re,omitempty"`
		Type string `json:"t,omitempty"`
	}
	if json.Unmarshal(data, &hdr) != nil {
		return
	}
	if hdr.Re != nil {
		b.deliverReply(*hdr.Re, bytes.Clone(data))
		return
	}
	if hdr.Type == "rows" {
		b.applyRows(data)
	}
}

func (b *m7Browser) deliverReply(re int64, raw json.RawMessage) {
	b.mu.Lock()
	ch := b.waiting[re]
	delete(b.waiting, re)
	b.mu.Unlock()
	if ch != nil {
		ch <- raw
	}
}

func (b *m7Browser) applyRows(data []byte) {
	var batch proto.Rows
	if proto.Decode(data, &batch) != nil || batch.Type != "rows" {
		return
	}
	b.mu.Lock()
	for _, elem := range batch.Rows {
		b.storeRowLocked(elem)
	}
	close(b.changed)
	b.changed = make(chan struct{})
	b.mu.Unlock()
}

func (b *m7Browser) storeRowLocked(elem json.RawMessage) {
	var hdr proto.RowHeader
	if proto.Decode(elem, &hdr) != nil {
		return
	}
	switch hdr.Type {
	case "member":
		b.storeMemberLocked(elem)
	case "task":
		b.storeTaskLocked(elem)
	case "msg":
		b.storeMsgLocked(elem)
	}
}

func (b *m7Browser) storeMemberLocked(elem json.RawMessage) {
	var ai proto.AIMemberRow
	if proto.Decode(elem, &ai) == nil && ai.Kind == "ai" {
		b.aiByName[ai.Name] = ai
	}
}

func (b *m7Browser) storeTaskLocked(elem json.RawMessage) {
	var t proto.TaskRow
	if proto.Decode(elem, &t) == nil {
		b.tasks[t.ID] = t
	}
}

func (b *m7Browser) storeMsgLocked(elem json.RawMessage) {
	var m proto.MsgRow
	if proto.Decode(elem, &m) != nil {
		return
	}
	if _, seen := b.msgs[m.ID]; !seen {
		b.msgOrder = append(b.msgOrder, m.ID)
	}
	b.msgs[m.ID] = m
}

func (b *m7Browser) waitRows(timeout time.Duration, pred func() bool) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		b.mu.Lock()
		ok := pred()
		ch := b.changed
		b.mu.Unlock()
		if ok {
			return nil
		}
		if !m7WaitChan(ch, timer.C) {
			return errors.New("timed out waiting on M7 rows")
		}
	}
}

func m7WaitChan(ch <-chan struct{}, timeout <-chan time.Time) bool {
	select {
	case <-ch:
		return true
	case <-timeout:
		return false
	}
}

func (b *m7Browser) reserveReq() (int64, chan json.RawMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	// Single reader: b.request.
	ch := make(chan json.RawMessage, 1)
	b.waiting[b.nextID] = ch
	return b.nextID, ch
}

func (b *m7Browser) clearReq(id int64) {
	b.mu.Lock()
	delete(b.waiting, id)
	b.mu.Unlock()
}

func (b *m7Browser) request(ctx context.Context, build func(int64) any) (json.RawMessage, error) {
	id, ch := b.reserveReq()
	defer b.clearReq(id)
	rawReq, err := marshalNoEscape(build(id))
	if err != nil {
		return nil, err
	}
	if err := b.ctl.SendText(string(rawReq)); err != nil {
		return nil, err
	}
	return b.awaitReqReply(ctx, ch)
}

func (b *m7Browser) awaitReqReply(ctx context.Context, ch <-chan json.RawMessage) (json.RawMessage, error) {
	select {
	case msg := <-ch:
		return msg, nil
	case <-b.ctlClosed:
		return nil, errors.New("ctl closed before response")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *m7Browser) hello() error {
	build := func(id int64) any {
		return proto.Hello{Request: proto.Request{Type: "hello", ID: id}, V: 1, Kind: "browser", Name: "Owner Browser", Addrs: []string{}}
	}
	raw, err := b.request(b.ctx, build)
	if err != nil {
		return err
	}
	var w proto.Welcome
	if err := proto.Decode(raw, &w); err != nil {
		return err
	}
	if !w.Owner || w.Device != b.nodeID {
		return fmt.Errorf("unexpected welcome: %+v", w)
	}
	return nil
}

func (b *m7Browser) hostInfo() (proto.HostInfo, error) {
	build := func(id int64) any {
		return proto.Request{Type: "host.info", ID: id}
	}
	raw, err := b.request(b.ctx, build)
	if err != nil {
		return proto.HostInfo{}, err
	}
	var info proto.HostInfo
	return info, proto.Decode(raw, &info)
}

func (b *m7Browser) syncGroup(gid string) error {
	build := func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: gid}
	}
	raw, err := b.request(b.ctx, build)
	if err != nil {
		return err
	}
	var synced proto.Synced
	return proto.Decode(raw, &synced)
}

func (b *m7Browser) sendEnv(env proto.Envelope) (string, error) {
	build := func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	}
	raw, err := b.request(b.ctx, build)
	if err != nil {
		return "", err
	}
	var wireErr proto.ErrorResponse
	if proto.Decode(raw, &wireErr) == nil {
		return wireErr.Err.Code, nil
	}
	var ok proto.Success
	if err := proto.Decode(raw, &ok); err != nil {
		return "", err
	}
	return "", nil
}

func (b *m7Browser) signCmd(v any) (proto.Envelope, error) {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return proto.Envelope{}, err
	}
	return b.key.Sign(string(raw)), nil
}

func (b *m7Browser) execCmd(v any) error {
	env, err := b.signCmd(v)
	if err != nil {
		return err
	}
	code, err := b.sendEnv(env)
	if err != nil {
		return err
	}
	if code != "" {
		return fmt.Errorf("command refused with code %q", code)
	}
	return nil
}

func (b *m7Browser) openPTY(gid, mid string) (*m7PTY, error) {
	dc, err := b.pc.CreateDataChannel("pty:"+gid+":"+mid, nil)
	if err != nil {
		return nil, err
	}
	p := newM7PTY(dc)
	if err := p.waitOpen(b.ctx); err != nil {
		return nil, err
	}
	resize := proto.PTYResize{Type: "pty.resize", Group: proto.ID(gid), AI: proto.ID(mid), Cols: 120, Rows: 32}
	raw, err := marshalNoEscape(resize)
	if err != nil {
		return nil, err
	}
	return p, b.ctl.SendText(string(raw))
}

func (b *m7Browser) fetchThread(gid, taskID string) ([]proto.MsgRow, error) {
	build := func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: gid, Limit: 20},
			Thread:    &taskID,
		}
	}
	raw, err := b.request(b.ctx, build)
	if err != nil {
		return nil, err
	}
	var fetched proto.Fetched
	if err := proto.Decode(raw, &fetched); err != nil {
		return nil, err
	}
	return m7DecodeMsgRows(fetched.Rows), nil
}

func m7DecodeMsgRows(rawRows []json.RawMessage) []proto.MsgRow {
	var out []proto.MsgRow
	for _, r := range rawRows {
		var m proto.MsgRow
		if proto.Decode(r, &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (b *m7Browser) selectedCandidateTypes() (webrtc.ICECandidateType, webrtc.ICECandidateType, bool) {
	pair, ok := b.pc.SCTP().Transport().ICETransport().GetSelectedCandidatePairStats()
	if !ok || pair.State != webrtc.StatsICECandidatePairStateSucceeded {
		return 0, 0, false
	}
	report := b.pc.GetStats()
	if _, hasPair := report[pair.ID].(webrtc.ICECandidatePairStats); !hasPair {
		return 0, 0, false
	}
	return m7CandidateTypesFromReport(report, pair.LocalCandidateID, pair.RemoteCandidateID)
}

func m7CandidateTypesFromReport(report webrtc.StatsReport, locID, remID string) (webrtc.ICECandidateType, webrtc.ICECandidateType, bool) {
	loc, okL := report[locID].(webrtc.ICECandidateStats)
	rem, okR := report[remID].(webrtc.ICECandidateStats)
	return loc.CandidateType, rem.CandidateType, okL && okR
}

func (b *m7Browser) statsShowRelay() (bool, bool) {
	loc, rem, ok := b.selectedCandidateTypes()
	if !ok {
		return false, false
	}
	relayed := loc == webrtc.ICECandidateTypeRelay || rem == webrtc.ICECandidateTypeRelay
	return relayed, true
}

type m7World struct {
	f           fixtures
	cleanup     func(func())
	ctx         context.Context
	root        string
	nodeDir     string
	repoDir     string
	cliLogPath  string
	rvURL       string
	turnAddr    string
	relayIP     net.IP
	iceServers  []proto.ICEServer
	pn          *peerNode
	accepted    chan *peer.Connection // single reader: acceptServed
	served      *peer.Connection
	browserKey  identity.Key
	browserMu   sync.Mutex
	browser     *m7Browser
	ids         seqIDGen
	scriptSeq   int
	groupID     proto.ID
	claudeTask  string
	codexTask   string
	activePTY   *m7PTY
	lastStart   proto.Envelope
	replayCode  string
	refusalCode string
	handbackDur time.Duration
	firstTakeID string
	secTakeID   string
	commitSHA   string
}

func m7Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m7World{
		f:       f,
		cleanup: afterScenario(sc),
	}
	registerM7GivenSteps(sc, w)
	registerM7TakeoverSteps(sc, w)
	registerM7RelayAndTaskSteps(sc, w)
}

func registerM7GivenSteps(sc *godog.ScenarioContext, w *m7World) {
	sc.Step(`^an M7 project group on a direct path with configured AI "([^"]+)" on an active task$`, w.givenDirectSingleAITask)
	sc.Step(`^an M7 project group on a direct path with configured AIs "([^"]+)" and "([^"]+)" on active tasks$`, w.givenDirectTwoAITasks)
	sc.Step(`^an M7 project group with configured AI "([^"]+)" accessed over a relayed WebRTC path$`, w.givenRelayedSingleAI)
	sc.Step(`^an M7 project group on a direct path with configured AI "([^"]+)" and no tasks$`, w.givenDirectSingleAINoTask)
	sc.Step(`^an M7 project group with configured AI "([^"]+)" on an active task over a direct path with TURN reachable$`, w.givenDirectWithTURNTask)
}

func registerM7TakeoverSteps(sc *godog.ScenarioContext, w *m7World) {
	sc.Step(`^the M7 owner opens terminal takeover on "([^"]+)" and attaches the pty channel$`, w.whenOpenTakeoverAndAttach)
	sc.Step(`^the M7 owner posts mentions "([^"]+)", "([^"]+)" and "([^"]+)" to "([^"]+)" in that task thread$`, w.whenPostThreeMentions)
	sc.Step(`^"([^"]+)" stays in state "([^"]+)" with (\d+) queued items in host\.info and no delivered reply$`, w.thenStateAndQueuedDuringTakeover)
	sc.Step(`^the M7 owner hands back "([^"]+)"'s terminal and "([^"]+)" posts handback "([^"]+)" and answers the held mentions$`, w.whenHandbackAndAnswerHeld)
	sc.Step(`^"([^"]+)" posts handback "([^"]+)" before posting "([^"]+)" in that task thread$`, w.thenHandbackBeforeOrderedReply)
	sc.Step(`^the M7 owner switches "([^"]+)" ten times between structured turns and interactive PTY takeover$`, w.whenTenSwitches)
	sc.Step(`^the M7 fake CLI log records every "([^"]+)" start and exit on one session id with no start before the previous exit$`, w.thenLogAlternatesOneSession)
	sc.Step(`^the M7 owner starts takeover on "([^"]+)", replays that takeover\.start while open, and sends takeover\.end$`, w.whenStartReplayAndEnd)
	sc.Step(`^the open replay succeeds, hand-back finishes within 2 seconds, and replaying that takeover\.start afterwards fails with "([^"]+)"$`, w.thenOpenReplayAndFastHandback)
	sc.Step(`^the M7 owner starts a second takeover on "([^"]+)" and closes the pty data channel in order$`, w.whenSecondTakeoverClosePTY)
	sc.Step(`^hand-back finishes within 2 seconds and replaying the second takeover\.start fails with "([^"]+)"$`, w.thenFastHandbackAndConflict)
	sc.Step(`^the M7 owner starts a third takeover on "([^"]+)" and drops all packets on that browser connection$`, w.whenThirdTakeoverDropPackets)
	sc.Step(`^the node hands back once its WebRTC stack reports the connection lost, and replaying the third takeover\.start on a reconnected ctl fails with "([^"]+)"$`, w.thenLostConnHandbackAndConflict)
}

func registerM7RelayAndTaskSteps(sc *godog.ScenarioContext, w *m7World) {
	sc.Step(`^the M7 owner sends takeover\.start for "([^"]+)" over that relayed ctl connection$`, w.whenSendTakeoverOnRelay)
	sc.Step(`^takeover\.start is refused with error code "([^"]+)" and "([^"]+)" starts (\d+) CLI processes$`, w.thenRefusedWithNoCLIStart)
	sc.Step(`^the M7 owner opens terminal takeover on "([^"]+)" with no task named$`, w.whenOpenNoTaskTakeover)
	sc.Step(`^an active "([^"]+)" task card exists for "([^"]+)" and its pty channel opens in the task worktree$`, w.thenOwnerTakeoverCardExists)
	sc.Step(`^the M7 owner hands back "([^"]+)"'s terminal with nothing changed in the worktree$`, w.whenHandbackUnchanged)
	sc.Step(`^that takeover task is "([^"]+)" with no open task session and (\d+) handback messages$`, w.thenTakeoverTaskCancelled)
	sc.Step(`^the M7 owner opens terminal takeover on "([^"]+)" again, commits "([^"]+)" with subject "([^"]+)" in the task worktree, and hands back$`, w.whenSecondNoTaskCommitAndHandback)
	sc.Step(`^the second takeover task remains "([^"]+)" and its handback message lists commit "([^"]+)" and file "([^"]+)"$`, w.thenSecondTaskActiveWithCommit)
	sc.Step(`^the M7 owner opens terminal takeover on "([^"]+)" and verifies a non-relay selected pair in WebRTC statistics$`, w.whenOpenTakeoverVerifyDirectStats)
	sc.Step(`^direct UDP is cut while TURN stays reachable so the same PeerConnection selects a relayed pair$`, w.whenCutDirectToRelay)
	sc.Step(`^WebRTC selected-pair statistics report a relay candidate, the pty channel closes, and the node hands back "([^"]+)"$`, w.thenStatsRelayClosesPTYAndHandsBack)
	sc.Step(`^host\.info still succeeds on the original M7 ctl connection without sending a new offer$`, w.thenHostInfoOriginalCtl)
}

func (w *m7World) relayGenerator(mode m7CandMode) (turn.RelayAddressGenerator, error) {
	ip := net.IPv4(127, 0, 0, 1)
	if mode == m7CandSrflxAndRelay {
		var err error
		ip, err = m7RelayHostIP()
		if err != nil {
			return nil, err
		}
	}
	w.relayIP = ip
	base := turn.RelayAddressGeneratorStatic{RelayAddress: ip, Address: ip.String()}
	if mode == m7CandRelayOnly {
		return &relaySockets{RelayAddressGeneratorStatic: base, ports: map[int]bool{}}, nil
	}
	return &base, nil
}

func m7RelayHostIP() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		ip := addr.(*net.IPNet).IP
		if ip.To4() != nil && !ip.IsLoopback() && ip.IsGlobalUnicast() {
			return ip, nil
		}
	}
	return nil, errors.New("no non-loopback IPv4 address for the TURN relay")
}

func m7ICEServersForMode(addr string, mode m7CandMode) []proto.ICEServer {
	stunSrv := proto.ICEServer{URLs: []string{"stun:" + addr}}
	turnSrv := testTURNServer(addr)
	switch mode {
	case m7CandDirect:
		return []proto.ICEServer{stunSrv}
	case m7CandRelayOnly:
		return []proto.ICEServer{turnSrv}
	default:
		return []proto.ICEServer{stunSrv, turnSrv}
	}
}

// m7Event bounds every wait for one event (a CLI turn, a row, a process start).
// Passing runs are not slower: each wait returns at the first match.
const m7Event = 15 * time.Second

// m7Scenario is only a guard against a hang. The node, the browser and every
// reconnect share the one context made in bootEnvAndNode; it is never replaced.
const m7Scenario = 5 * time.Minute

func (w *m7World) bootEnvAndNode(mode m7CandMode) error {
	ctx, cancel := context.WithTimeout(context.Background(), m7Scenario)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := w.prepareDirsAndEnv(); err != nil {
		return err
	}
	gen, err := w.relayGenerator(mode)
	if err != nil {
		return err
	}
	turnAddr, err := startTestTURN(w.cleanup, gen)
	if err != nil {
		return err
	}
	w.turnAddr, w.iceServers = turnAddr, m7ICEServersForMode(turnAddr, mode)
	if err := w.startNode(mode); err != nil {
		return err
	}
	return w.connectBrowser(mode)
}

func (w *m7World) prepareDirsAndEnv() error {
	rawRoot, err := shortTempRoot(w.f.t, w.cleanup, "sdm7")
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(rawRoot)
	if err != nil {
		return err
	}
	w.root = root
	w.nodeDir = filepath.Join(root, "node")
	w.repoDir = filepath.Join(root, "repo")
	w.cliLogPath = filepath.Join(root, "fakecli.log")
	if err := initLocalGitRepo(w.repoDir); err != nil {
		return err
	}
	return errors.Join(
		setScenarioEnv(w.cleanup, "SOLODAWN_FAKECLI_LOG", w.cliLogPath),
		setScenarioEnv(w.cleanup, "SOLODAWN_FAKE_CLAUDE_SCRIPT", filepath.Join(root, "claude-init.json")),
		setScenarioEnv(w.cleanup, "SOLODAWN_FAKE_CODEX_SCRIPT", filepath.Join(root, "codex-init.json")),
	)
}

func (w *m7World) startNode(mode m7CandMode) error {
	srv := httptest.NewServer(rendezvous.Handler(fstest.MapFS{}))
	w.cleanup(srv.Close)
	w.rvURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/rv"
	w.browserKey = exampleKey("m7-owner-browser")
	w.accepted = make(chan *peer.Connection, 8)
	pn, err := startPeerNode(w.f.t, w.cleanup, peerNodeConfig{
		Ctx:        w.ctx,
		Dir:        w.nodeDir,
		Key:        exampleKey("m7-node"),
		Name:       "m7-workstation",
		Version:    "v0.1.0",
		Owner:      w.browserKey,
		Rendezvous: w.rvURL,
		MapRoute:   func(r peer.Route) peer.Route { return w.mapNodeRoute(r, mode) },
		OnInbound:  func(c *peer.Connection) { w.accepted <- c },
	})
	if err != nil {
		return err
	}
	w.pn = pn
	if err := w.attachMCPSocket(); err != nil {
		return err
	}
	w.cleanup(func() { w.settleAndClose() })
	return nil
}

func (w *m7World) setBrowser(b *m7Browser) {
	w.browserMu.Lock()
	w.browser = b
	w.browserMu.Unlock()
}

func (w *m7World) currentBrowser() *m7Browser {
	w.browserMu.Lock()
	defer w.browserMu.Unlock()
	return w.browser
}

func (w *m7World) settleAndClose() {
	if b := w.currentBrowser(); b != nil && b.pc != nil {
		_ = b.pc.Close()
	}
	if w.pn != nil {
		w.pn.Close()
	}
}

func (w *m7World) mapNodeRoute(r peer.Route, mode m7CandMode) peer.Route {
	r.ICE = m7NodeICEServers(w.iceServers, mode)
	send := r.Send
	r.Send = func(ctx context.Context, to string, env proto.Envelope) error {
		if to == w.browserKey.ID() {
			return w.forwardNodeSignal(ctx, to, env)
		}
		if m7DropSignalledICE(env.Payload, mode) {
			return nil
		}
		return send(ctx, to, env)
	}
	return r
}

func (w *m7World) forwardNodeSignal(ctx context.Context, to string, env proto.Envelope) error {
	if b := w.currentBrowser(); b != nil {
		return b.receiveFromNode(ctx, to, env)
	}
	return nil
}

func m7DropSignalledICE(payload string, mode m7CandMode) bool {
	var sig struct {
		Type string `json:"t"`
		Cand string `json:"cand"`
	}
	if json.Unmarshal([]byte(payload), &sig) != nil || sig.Type != "ice" {
		return false
	}
	return !m7KeepCandidate(mode, sig.Cand)
}

func (w *m7World) attachMCPSocket() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	mcpDir := filepath.Join(w.nodeDir, "mcp")
	local, err := node.ListenLocal(mcpDir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go serveTestMCP(ctx, local, w.pn.Host, &wg)
	stop := sync.OnceFunc(func() {
		cancel()
		_ = local.Close()
		wg.Wait()
	})
	registerBothCleanups(w.f.t, w.cleanup, stop)
	w.pn.Host.Shim = []string{exe, "-test.run=^TestM3MCPShim$", "--", filepath.Join(mcpDir, "sock")}
	return nil
}

func (w *m7World) connectBrowser(mode m7CandMode) error {
	b := newM7Browser(w.ctx, w.browserKey, w.pn.Key.ID(), mode, w.turnAddr, w.iceServers)
	w.setBrowser(b)
	if err := b.connect(w.cleanup, w.pn.Peer, w.relayIP); err != nil {
		return err
	}
	served, err := w.acceptServed()
	if err != nil {
		return err
	}
	w.served = served
	wantPath := "direct"
	if mode == m7CandRelayOnly {
		wantPath = "relay"
	}
	if err := waitConnPath(served, wantPath, m7Event); err != nil {
		return err
	}
	return b.hello()
}

func (w *m7World) acceptServed() (*peer.Connection, error) {
	select {
	case c := <-w.accepted:
		return c, nil
	case <-w.ctx.Done():
		return nil, w.ctx.Err()
	}
}

func (w *m7World) createProjectGroup() error {
	gid := w.ids.next()
	w.groupID = gid
	cmd := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: time.Now().UnixMilli(), Group: gid},
			Op:          "create",
		},
		Name:       m7GroupName,
		Kind:       "project",
		Repo:       w.repoDir,
		Base:       "main",
		Forge:      "none",
		Checks:     []string{},
		MemberName: "Owner",
	}
	if err := w.browser.execCmd(cmd); err != nil {
		return err
	}
	return w.browser.syncGroup(string(gid))
}

func m7VendorOf(aiName string) string {
	if strings.EqualFold(aiName, "codex") {
		return "codex"
	}
	return "claude"
}

func (w *m7World) joinAI(aiName string) error {
	if _, err := w.browser.hostInfo(); err != nil {
		return err
	}
	agent := m7VendorOf(aiName)
	setCmd := proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: w.pn.Key.ID()},
		Agent:       agent,
		Account:     "personal",
	}
	if err := w.browser.execCmd(setCmd); err != nil {
		return err
	}
	joinCmd := proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: w.pn.Key.ID()},
		Group:       w.groupID,
		Home:        w.pn.Key.ID(),
		Agent:       agent,
		Name:        aiName,
	}
	if err := w.browser.execCmd(joinCmd); err != nil {
		return err
	}
	return w.browser.waitRows(m7Event, func() bool {
		_, ok := w.browser.aiByName[aiName]
		return ok
	})
}

func (w *m7World) setScript(vendor string, doc map[string]any) error {
	w.scriptSeq++
	path := filepath.Join(w.root, fmt.Sprintf("%s-%d.json", vendor, w.scriptSeq))
	raw, err := marshalNoEscape(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Setenv("SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", path)
}

func m7ClaudeWaitingDoc(postText string) map[string]any {
	steps := append(claudeTurnSteps(1, true, "", postText, ""), map[string]any{"dir": "eof"})
	return map[string]any{"steps": steps}
}

func m7HandbackPostSteps(callID int, text string) []map[string]any {
	return mcpCallSteps(callID, "post", map[string]any{"kind": "handback", "text": text}, map[string]any{"isError": false})
}

func m7ClaudeHandbackDoc(handbackText string, extraTurn []map[string]any) map[string]any {
	steps := claudeToolTurn(true, "", "", m7HandbackPostSteps(1, handbackText))
	steps = append(steps, extraTurn...)
	steps = append(steps, map[string]any{"dir": "eof"})
	return map[string]any{"steps": steps}
}

func m7CodexFirstTaskDoc(postText string) map[string]any {
	steps := append(codexStartupSteps(), codexTurnSteps("turn-1", 1, "", postText, false)...)
	steps = append(steps, map[string]any{"dir": "eof"})
	return map[string]any{"session": m7CodexThread, "steps": steps}
}

func m7CodexResumeHandbackDoc(turnID, handbackText string) map[string]any {
	steps := codexThreadSteps(map[string]any{"method": "thread/resume", "params": map[string]any{"threadId": "$session"}})
	steps = append(steps, codexToolTurn(turnID, "", m7HandbackPostSteps(1, handbackText))...)
	steps = append(steps, map[string]any{"dir": "eof"})
	return map[string]any{"session": m7CodexThread, "steps": steps}
}

func m7InteractiveHoldDoc() map[string]any {
	return map[string]any{"steps": []map[string]any{
		{"dir": "stdout", "msg": map[string]any{"ready": "$session"}},
		{"dir": "eof"},
	}}
}

func m7InteractiveExitDoc() map[string]any {
	return map[string]any{"steps": []map[string]any{
		{"dir": "stdout", "msg": map[string]any{"ready": "$session"}},
		{"dir": "stdin", "msg": map[string]any{"op": "exit"}},
		{"dir": "stdout", "msg": map[string]any{"bye": "$session"}},
	}}
}

func (w *m7World) startActiveTask(aiName string) (string, error) {
	vendor := m7VendorOf(aiName)
	readyPost := aiName + " ready on task."
	doc := m7ClaudeWaitingDoc(readyPost)
	if vendor == "codex" {
		doc = m7CodexFirstTaskDoc(readyPost)
	}
	if err := w.setScript(vendor, doc); err != nil {
		return "", err
	}
	aiID := w.aiID(aiName)
	tid := w.ids.next()
	cmd := proto.TaskStart{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: tid, Timestamp: time.Now().UnixMilli(), Group: w.groupID, As: proto.ID(aiID),
			},
			Op: "start", ID: tid,
		},
		Title: aiName + " task", Plan: "Initial work", Origin: "direct",
	}
	raw, err := marshalNoEscape(cmd)
	if err != nil {
		return "", err
	}
	if _, err := w.pn.Host.Command(w.ctx, w.pn.Key.ID(), string(raw)); err != nil {
		return "", err
	}
	err = w.waitMsgAndAIState(aiName, readyPost, "waiting", m7Event)
	return string(tid), err
}

func (w *m7World) aiID(aiName string) string {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	return w.browser.aiByName[aiName].ID
}

func (w *m7World) waitMsgAndAIState(aiName, wantText, wantState string, timeout time.Duration) error {
	return w.browser.waitRows(timeout, func() bool {
		ai := w.browser.aiByName[aiName]
		return ai.State == wantState && w.hasMsgByLocked(ai.ID, wantText)
	})
}

func (w *m7World) hasMsgByLocked(authorID, text string) bool {
	for _, m := range w.browser.msgs {
		if m.By == authorID && m.Text == text {
			return true
		}
	}
	return false
}

func (w *m7World) givenDirectSingleAITask(aiName string) error {
	if err := w.bootEnvAndNode(m7CandDirect); err != nil {
		return err
	}
	if err := w.createProjectGroup(); err != nil {
		return err
	}
	if err := w.joinAI(aiName); err != nil {
		return err
	}
	tid, err := w.startActiveTask(aiName)
	w.claudeTask = tid
	return err
}

func (w *m7World) givenDirectTwoAITasks(ai1, ai2 string) error {
	if err := w.bootEnvAndNode(m7CandDirect); err != nil {
		return err
	}
	if err := w.createProjectGroup(); err != nil {
		return err
	}
	if err := errors.Join(w.joinAI(ai1), w.joinAI(ai2)); err != nil {
		return err
	}
	t1, err := w.startActiveTask(ai1)
	if err != nil {
		return err
	}
	t2, err := w.startActiveTask(ai2)
	w.claudeTask, w.codexTask = t1, t2
	return err
}

func (w *m7World) givenRelayedSingleAI(aiName string) error {
	if err := w.bootEnvAndNode(m7CandRelayOnly); err != nil {
		return err
	}
	if err := w.createProjectGroup(); err != nil {
		return err
	}
	return w.joinAI(aiName)
}

func (w *m7World) givenDirectSingleAINoTask(aiName string) error {
	if err := w.bootEnvAndNode(m7CandDirect); err != nil {
		return err
	}
	if err := w.createProjectGroup(); err != nil {
		return err
	}
	return w.joinAI(aiName)
}

func (w *m7World) givenDirectWithTURNTask(aiName string) error {
	if err := w.bootEnvAndNode(m7CandSrflxAndRelay); err != nil {
		return err
	}
	if err := w.createProjectGroup(); err != nil {
		return err
	}
	if err := w.joinAI(aiName); err != nil {
		return err
	}
	tid, err := w.startActiveTask(aiName)
	w.claudeTask = tid
	return err
}

func (w *m7World) buildTakeoverStart(aiName, taskID string) (proto.Envelope, error) {
	cmd := proto.TakeoverStart{
		Takeover: proto.Takeover{
			HostCommand: proto.HostCommand{
				Type: "takeover.start", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: w.pn.Key.ID(),
			},
			Group:  w.groupID,
			Member: proto.ID(w.aiID(aiName)),
		},
		Task: proto.ID(taskID),
	}
	return w.browser.signCmd(cmd)
}

func (w *m7World) sendTakeoverEnd(aiName string) error {
	cmd := proto.Takeover{
		HostCommand: proto.HostCommand{
			Type: "takeover.end", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: w.pn.Key.ID(),
		},
		Group:  w.groupID,
		Member: proto.ID(w.aiID(aiName)),
	}
	return w.browser.execCmd(cmd)
}

func (w *m7World) openTakeoverSession(aiName, taskID string, doc map[string]any) error {
	vendor := m7VendorOf(aiName)
	if err := w.setScript(vendor, doc); err != nil {
		return err
	}
	env, err := w.buildTakeoverStart(aiName, taskID)
	if err != nil {
		return err
	}
	w.lastStart = env
	code, err := w.browser.sendEnv(env)
	if err != nil {
		return err
	}
	if code != "" {
		return fmt.Errorf("takeover.start refused with %q", code)
	}
	ptyStream, err := w.browser.openPTY(string(w.groupID), w.aiID(aiName))
	if err != nil {
		return err
	}
	w.activePTY = ptyStream
	return ptyStream.waitOutput(m7Event, `"ready":`)
}

func (w *m7World) whenOpenTakeoverAndAttach(aiName string) error {
	if err := w.openTakeoverSession(aiName, w.claudeTask, m7InteractiveHoldDoc()); err != nil {
		return err
	}
	return w.browser.waitRows(m7Event, func() bool {
		return w.browser.aiByName[aiName].State == "takeover"
	})
}

func (w *m7World) whenPostThreeMentions(m1, m2, m3, aiName string) error {
	aiID := proto.ID(w.aiID(aiName))
	tid := proto.ID(w.claudeTask)
	for _, text := range []string{m1, m2, m3} {
		msgID := w.ids.next()
		cmd := proto.Post[proto.EmptyBody]{
			HomeCommand: proto.HomeCommand{Type: "post", Nonce: msgID, Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Kind:        "text",
			Text:        text,
			Thread:      &tid,
			Mentions:    []proto.ID{aiID},
		}
		if err := w.browser.execCmd(cmd); err != nil {
			return err
		}
	}
	return nil
}

func (w *m7World) waitHostQueued(aiName string, wantQueued int) error {
	deadline := time.Now().Add(m7Event)
	aiID := proto.ID(w.aiID(aiName))
	for time.Now().Before(deadline) {
		info, err := w.browser.hostInfo()
		if err != nil {
			return err
		}
		if got, state, ok := m7LookupAgentGroup(info, w.groupID, aiID); ok && got == wantQueued && state == "takeover" {
			return nil
		}
		time.Sleep(40 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s queued=%d in takeover", aiName, wantQueued)
}

func m7LookupAgentGroup(info proto.HostInfo, gid, mid proto.ID) (int, string, bool) {
	for _, a := range info.Agents {
		if q, st, ok := m7FindGroupEntry(a.Groups, gid, mid); ok {
			return q, st, true
		}
	}
	return 0, "", false
}

func m7FindGroupEntry(groups []proto.AgentGroup, gid, mid proto.ID) (int, string, bool) {
	for _, g := range groups {
		if g.Group == gid && g.Member == mid {
			return g.Queued, g.State, true
		}
	}
	return 0, "", false
}

func (w *m7World) thenStateAndQueuedDuringTakeover(aiName, wantState string, wantQueued int) error {
	if err := w.waitHostQueued(aiName, wantQueued); err != nil {
		return err
	}
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	ai := w.browser.aiByName[aiName]
	if ai.State != wantState {
		return fmt.Errorf("member state = %q, want %q", ai.State, wantState)
	}
	if w.countAIMessagesLocked(ai.ID) != 1 {
		return fmt.Errorf("AI posted unexpected reply during takeover")
	}
	return nil
}

func (w *m7World) countAIMessagesLocked(aiID string) int {
	n := 0
	for _, m := range w.browser.msgs {
		if m.By == aiID {
			n++
		}
	}
	return n
}

func (w *m7World) whenHandbackAndAnswerHeld(aiName, _, handbackText string) error {
	wantTurn := strings.Join([]string{
		fmt.Sprintf("[SoloDawn] group %q · you are %q · task session T-1", m7GroupName, aiName),
		`== 1 of 3 · mention · from "Owner" in T-1 · instruction`,
		"first held",
		`== 2 of 3 · mention · from "Owner" in T-1 · instruction`,
		"second held",
		`== 3 of 3 · mention · from "Owner" in T-1 · instruction`,
		"third held",
	}, "\n")
	extra := claudeTurnSteps(2, false, wantTurn, "Ordered: first held, second held, third held", "")
	if err := w.setScript("claude", m7ClaudeHandbackDoc(handbackText, extra)); err != nil {
		return err
	}
	return w.sendTakeoverEnd(aiName)
}

func (w *m7World) closeOpenSessions() {
	if w.pn == nil || w.pn.Host == nil {
		return
	}
	for _, s := range w.pn.Host.Sessions.All() {
		host.Runner.Close(w.pn.Host, s.SessionKey)
	}
}

func (w *m7World) thenHandbackBeforeOrderedReply(aiName, handbackText, orderedText string) error {
	if err := w.waitMsgAndAIState(aiName, orderedText, "waiting", m7Event); err != nil {
		return err
	}
	w.browser.mu.Lock()
	hbIdx, replyIdx := w.msgOrderIndicesLocked(w.browser.aiByName[aiName].ID, handbackText, orderedText)
	w.browser.mu.Unlock()
	w.closeOpenSessions()
	if hbIdx < 0 || replyIdx < 0 || hbIdx >= replyIdx {
		return fmt.Errorf("handback index %d and reply index %d not in order", hbIdx, replyIdx)
	}
	return nil
}

func (w *m7World) msgOrderIndicesLocked(aiID, hbText, replyText string) (int, int) {
	return w.msgIndexLocked(aiID, "handback", hbText), w.msgIndexLocked(aiID, "text", replyText)
}

func (w *m7World) msgIndexLocked(aiID, kind, text string) int {
	for i, id := range w.browser.msgOrder {
		if w.matchesThreadMsg(w.browser.msgs[id], aiID, kind, text) {
			return i
		}
	}
	return -1
}

func (w *m7World) matchesThreadMsg(m proto.MsgRow, aiID, kind, text string) bool {
	if m.Thread == nil || *m.Thread != w.claudeTask {
		return false
	}
	return m.By == aiID && m.Kind == kind && m.Text == text
}

func (w *m7World) taskForAI(aiName string) string {
	if m7VendorOf(aiName) == "codex" {
		return w.codexTask
	}
	return w.claudeTask
}

func (w *m7World) whenTenSwitches(aiName string) error {
	vendor := m7VendorOf(aiName)
	taskID := w.taskForAI(aiName)
	// Each cycle is two switches (structured to terminal, terminal back), so
	// 5 cycles are the ten switches.
	for i := range 5 {
		hbText := fmt.Sprintf("%s handback %d", aiName, i+1)
		if err := w.runOneSwitchCycle(aiName, vendor, taskID, i+2, hbText); err != nil {
			return err
		}
	}
	host.Runner.Close(w.pn.Host, host.SessionKey{Group: string(w.groupID), Member: w.aiID(aiName), Task: taskID})
	return nil
}

func (w *m7World) runOneSwitchCycle(aiName, vendor, taskID string, turnNum int, hbText string) error {
	if err := w.openTakeoverSession(aiName, taskID, m7InteractiveExitDoc()); err != nil {
		return err
	}
	resumeDoc := m7ClaudeHandbackDoc(hbText, nil)
	if vendor == "codex" {
		resumeDoc = m7CodexResumeHandbackDoc("turn-"+strconv.Itoa(turnNum), hbText)
	}
	if err := w.setScript(vendor, resumeDoc); err != nil {
		return err
	}
	if err := w.activePTY.dc.Send([]byte("{\"op\":\"exit\"}\r")); err != nil {
		return err
	}
	// The CLI's "bye" line is not awaited: when the CLI exits right after
	// printing it, Windows ConPTY can drop its last frame (the echo and "bye"
	// together), so the line may never reach the channel. The hand-back post and
	// the exit entry in the CLI log (thenLogAlternatesOneSession) prove the
	// exit; on a timeout the error carries what the terminal showed.
	if err := w.waitMsgAndAIState(aiName, hbText, "waiting", m7Event); err != nil {
		return fmt.Errorf("%w; terminal output %q", err, w.activePTY.text())
	}
	return nil
}

func (w *m7World) thenLogAlternatesOneSession(vendor string) error {
	raw, err := os.ReadFile(w.cliLogPath)
	if err != nil {
		return err
	}
	lines := m7VendorLogLines(string(raw), vendor)
	if len(lines) != 22 {
		return fmt.Errorf("%s log has %d entries (%v), want 22 across 10 switches", vendor, len(lines), lines)
	}
	return m7VerifyAlternatingSession(lines, vendor)
}

func m7VendorLogLines(content, vendor string) []string {
	var out []string
	suffix := " " + vendor + " "
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if strings.Contains(line, suffix) {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func m7VerifyAlternatingSession(lines []string, vendor string) error {
	var sessionID string
	for i, line := range lines {
		wantAction := "start"
		if i%2 == 1 {
			wantAction = "exit"
		}
		id, err := m7ParseLogEntry(line, wantAction, vendor)
		if err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		if sessionID == "" {
			sessionID = id
		} else if id != sessionID {
			return fmt.Errorf("session id changed from %q to %q", sessionID, id)
		}
	}
	return nil
}

func m7ParseLogEntry(line, wantAction, wantVendor string) (string, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 || fields[0] != wantAction || fields[1] != wantVendor || fields[2] == "" {
		return "", fmt.Errorf("got %q, want %q %q <session>", line, wantAction, wantVendor)
	}
	return fields[2], nil
}

func (w *m7World) whenSendTakeoverOnRelay(aiName string) error {
	if err := w.setScript("claude", m7InteractiveHoldDoc()); err != nil {
		return err
	}
	env, err := w.buildTakeoverStart(aiName, "")
	if err != nil {
		return err
	}
	code, err := w.browser.sendEnv(env)
	w.refusalCode = code
	return err
}

func (w *m7World) thenRefusedWithNoCLIStart(wantCode, aiName string, wantStarts int) error {
	if w.refusalCode != wantCode {
		return fmt.Errorf("takeover.start code = %q, want %q", w.refusalCode, wantCode)
	}
	raw, err := os.ReadFile(w.cliLogPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	got := len(m7VendorLogLines(string(raw), m7VendorOf(aiName)))
	if got != wantStarts {
		return fmt.Errorf("logged %d CLI lines, want %d", got, wantStarts)
	}
	return nil
}

func (w *m7World) whenStartReplayAndEnd(aiName string) error {
	if err := w.openTakeoverSession(aiName, w.claudeTask, m7InteractiveHoldDoc()); err != nil {
		return err
	}
	code, err := w.browser.sendEnv(w.lastStart)
	if err != nil {
		return err
	}
	w.replayCode = code
	if err := w.setScript("claude", m7ClaudeHandbackDoc("Orderly button handback.", nil)); err != nil {
		return err
	}
	started := time.Now()
	if err := w.sendTakeoverEnd(aiName); err != nil {
		return err
	}
	return w.waitLeftTakeover(aiName, "Orderly button handback.", started)
}

// waitLeftTakeover records how long the node took to close the terminal and
// leave the "takeover" state (the contract's "within 2 seconds"), then waits
// the usual event bound for the hand-back turn's post. The turn starts CLI
// and helper processes, so it is not part of the 2 second bound.
func (w *m7World) waitLeftTakeover(aiName, hbText string, started time.Time) error {
	err := w.browser.waitRows(m7Event, func() bool {
		return w.browser.aiByName[aiName].State != "takeover"
	})
	w.handbackDur = time.Since(started)
	if err != nil {
		return err
	}
	return w.waitMsgAndAIState(aiName, hbText, "waiting", m7Event)
}

func (w *m7World) thenOpenReplayAndFastHandback(wantCode string) error {
	if w.replayCode != "" {
		return fmt.Errorf("replay while open returned %q, want ok", w.replayCode)
	}
	return w.thenFastHandbackAndConflict(wantCode)
}

func (w *m7World) thenFastHandbackAndConflict(wantCode string) error {
	if w.handbackDur > 2*time.Second {
		return fmt.Errorf("hand-back took %s, want within 2s", w.handbackDur)
	}
	code, err := w.browser.sendEnv(w.lastStart)
	if err != nil {
		return err
	}
	if code != wantCode {
		return fmt.Errorf("replay after hand-back returned %q, want %q", code, wantCode)
	}
	return nil
}

func (w *m7World) whenSecondTakeoverClosePTY(aiName string) error {
	if err := w.openTakeoverSession(aiName, w.claudeTask, m7InteractiveHoldDoc()); err != nil {
		return err
	}
	if err := w.setScript("claude", m7ClaudeHandbackDoc("Orderly tab close handback.", nil)); err != nil {
		return err
	}
	started := time.Now()
	if err := w.activePTY.dc.Close(); err != nil {
		return err
	}
	return w.waitLeftTakeover(aiName, "Orderly tab close handback.", started)
}

func (w *m7World) whenThirdTakeoverDropPackets(aiName string) error {
	if err := w.openTakeoverSession(aiName, w.claudeTask, m7InteractiveHoldDoc()); err != nil {
		return err
	}
	if err := w.setScript("claude", m7ClaudeHandbackDoc("Lost connection handback.", nil)); err != nil {
		return err
	}
	w.browser.ctrl.cut.Store(true)
	return nil
}

func (w *m7World) thenLostConnHandbackAndConflict(wantCode string) error {
	if err := w.waitServedClosed(50 * time.Second); err != nil {
		return err
	}
	if err := w.reconnectAndSync(); err != nil {
		return err
	}
	if err := w.waitLostHandbackFetched("Claude", "Lost connection handback."); err != nil {
		return err
	}
	err := w.checkReplayCode(wantCode)
	w.closeOpenSessions()
	return err
}

func (w *m7World) waitLostHandbackFetched(aiName, wantText string) error {
	err := w.browser.waitRows(m7Event, func() bool {
		return w.browser.aiByName[aiName].State == "waiting"
	})
	if err != nil {
		return err
	}
	return w.pollThreadHandback(w.aiID(aiName), wantText, m7Event)
}

func (w *m7World) pollThreadHandback(aiID, wantText string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rows, err := w.browser.fetchThread(string(w.groupID), w.claudeTask)
		if err == nil && m7HasHandbackRow(rows, aiID, wantText) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("thread %s missing handback %q", w.claudeTask, wantText)
}

func m7HasHandbackRow(rows []proto.MsgRow, aiID, wantText string) bool {
	for _, m := range rows {
		if m.By == aiID && m.Kind == "handback" && m.Text == wantText {
			return true
		}
	}
	return false
}

func (w *m7World) waitServedClosed(timeout time.Duration) error {
	select {
	case <-w.served.Done():
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("node WebRTC stack did not close lost connection within %s", timeout)
	}
}

func (w *m7World) reconnectAndSync() error {
	if err := w.connectBrowser(m7CandDirect); err != nil {
		return err
	}
	return w.browser.syncGroup(string(w.groupID))
}

func (w *m7World) checkReplayCode(wantCode string) error {
	code, err := w.browser.sendEnv(w.lastStart)
	if err != nil {
		return err
	}
	if code != wantCode {
		return fmt.Errorf("replayed takeover.start returned %q, want %q", code, wantCode)
	}
	return nil
}

func (w *m7World) whenOpenNoTaskTakeover(aiName string) error {
	return w.openTakeoverSession(aiName, "", m7InteractiveHoldDoc())
}

func (w *m7World) findTakeoverTaskLocked(excludeID string) (proto.TaskRow, bool) {
	for _, t := range w.browser.tasks {
		if t.ID != excludeID && t.Origin == "takeover" {
			return t, true
		}
	}
	return proto.TaskRow{}, false
}

func (w *m7World) hasTaskCardMsgLocked(taskID string) bool {
	m, ok := w.browser.msgs[taskID]
	if !ok || m.Kind != "task" {
		return false
	}
	var body proto.TaskBody
	return proto.Decode(m.Body, &body) == nil && body.Task == taskID
}

func (w *m7World) thenOwnerTakeoverCardExists(wantTitle, aiName string) error {
	err := w.browser.waitRows(m7Event, func() bool {
		t, ok := w.findTakeoverTaskLocked("")
		return ok && t.State == "active" && t.Title == wantTitle && w.hasTaskCardMsgLocked(t.ID)
	})
	if err != nil {
		return err
	}
	w.browser.mu.Lock()
	t, _ := w.findTakeoverTaskLocked("")
	w.browser.mu.Unlock()
	w.firstTakeID = t.ID
	wtDir := w.taskWorktreePath(t)
	if _, statErr := os.Stat(wtDir); statErr != nil {
		return fmt.Errorf("expected takeover worktree %s for %s: %w", wtDir, aiName, statErr)
	}
	return nil
}

func (w *m7World) taskWorktreePath(t proto.TaskRow) string {
	return filepath.Join(w.nodeDir, "projects", string(w.groupID), "wt", fmt.Sprintf("%d-%s", t.Num, t.Slug))
}

func (w *m7World) whenHandbackUnchanged(aiName string) error {
	return w.sendTakeoverEnd(aiName)
}

func (w *m7World) thenTakeoverTaskCancelled(wantState string, wantHandbacks int) error {
	err := w.browser.waitRows(m7Event, func() bool {
		return w.browser.tasks[w.firstTakeID].State == wantState
	})
	if err != nil {
		return err
	}
	if err := w.waitNoTaskSession(w.firstTakeID); err != nil {
		return err
	}
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	if got := w.countHandbackMsgsLocked(); got != wantHandbacks {
		return fmt.Errorf("handback message count = %d, want %d", got, wantHandbacks)
	}
	return nil
}

func (w *m7World) waitNoTaskSession(taskID string) error {
	deadline := time.Now().Add(m7Event)
	for time.Now().Before(deadline) {
		if !w.hasOpenTaskSession(taskID) {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return fmt.Errorf("task session %s remained present after cancel", taskID)
}

func (w *m7World) hasOpenTaskSession(taskID string) bool {
	for _, s := range w.pn.Host.Sessions.All() {
		if s.Task == taskID {
			return true
		}
	}
	return false
}

func (w *m7World) countHandbackMsgsLocked() int {
	n := 0
	for _, m := range w.browser.msgs {
		if m.Kind == "handback" {
			n++
		}
	}
	return n
}

func (w *m7World) whenSecondNoTaskCommitAndHandback(aiName, fileName, subject string) error {
	if err := w.openTakeoverSession(aiName, "", m7InteractiveHoldDoc()); err != nil {
		return err
	}
	err := w.browser.waitRows(m7Event, func() bool {
		t, ok := w.findTakeoverTaskLocked(w.firstTakeID)
		return ok && t.State == "active"
	})
	if err != nil {
		return err
	}
	w.browser.mu.Lock()
	t, _ := w.findTakeoverTaskLocked(w.firstTakeID)
	w.browser.mu.Unlock()
	w.secTakeID = t.ID
	if err := w.commitInWorktree(w.taskWorktreePath(t), fileName, subject); err != nil {
		return err
	}
	if err := w.setScript("claude", m7ClaudeHandbackDoc("Committed "+fileName+".", nil)); err != nil {
		return err
	}
	return w.sendTakeoverEnd(aiName)
}

func (w *m7World) commitInWorktree(wtDir, fileName, subject string) error {
	if err := os.WriteFile(filepath.Join(wtDir, fileName), []byte("owner takeover line\n"), 0o600); err != nil {
		return err
	}
	if err := runGitIn(wtDir, "add", fileName); err != nil {
		return err
	}
	if err := runGitIn(wtDir, "-c", "user.name=Owner", "-c", "user.email=owner@example.com", "commit", "-m", subject); err != nil {
		return err
	}
	out, err := exec.Command("git", "-C", wtDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	w.commitSHA = strings.TrimSpace(string(out))
	return nil
}

func (w *m7World) thenSecondTaskActiveWithCommit(wantState, wantSubject, wantFile string) error {
	if err := w.waitMsgAndAIState("Claude", "Committed "+wantFile+".", "waiting", m7Event); err != nil {
		return err
	}
	w.browser.mu.Lock()
	t := w.browser.tasks[w.secTakeID]
	hb, ok := w.findHandbackForTaskLocked(w.secTakeID)
	w.browser.mu.Unlock()
	w.closeOpenSessions()
	if t.State != wantState || !ok {
		return fmt.Errorf("second task state=%q handbackFound=%v", t.State, ok)
	}
	return w.verifyHandbackBody(hb.Body, wantSubject, wantFile)
}

func (w *m7World) findHandbackForTaskLocked(taskID string) (proto.MsgRow, bool) {
	for _, m := range w.browser.msgs {
		if m.Kind == "handback" && m.Thread != nil && *m.Thread == taskID {
			return m, true
		}
	}
	return proto.MsgRow{}, false
}

func (w *m7World) verifyHandbackBody(raw json.RawMessage, wantSubject, wantFile string) error {
	var body proto.HandbackBody
	if err := proto.Decode(raw, &body); err != nil {
		return err
	}
	if err := m7CheckHandbackFiles(body, wantFile); err != nil {
		return err
	}
	return m7CheckHandbackCommit(body.Commits, w.commitSHA, wantSubject)
}

func m7CheckHandbackFiles(body proto.HandbackBody, wantFile string) error {
	if !slices.Contains(body.Files, wantFile) || body.Added != 1 || body.Removed != 0 {
		return fmt.Errorf("unexpected handback files/diff: %+v", body)
	}
	return nil
}

func m7CheckHandbackCommit(commits []proto.CommitSubject, headSHA, wantSubject string) error {
	if len(commits) != 1 || commits[0].Subject != wantSubject || !m7SHAMatches(headSHA, commits[0].SHA) {
		return fmt.Errorf("unexpected handback commits %+v for head %s", commits, headSHA)
	}
	return nil
}

func m7SHAMatches(fullSHA, reported string) bool {
	return reported != "" && strings.HasPrefix(fullSHA, reported)
}

func (w *m7World) whenOpenTakeoverVerifyDirectStats(aiName string) error {
	if err := w.openTakeoverSession(aiName, w.claudeTask, m7InteractiveHoldDoc()); err != nil {
		return err
	}
	relayed, ok := w.browser.statsShowRelay()
	if !ok || relayed {
		return fmt.Errorf("initial selected pair stats ok=%v relayed=%v, want non-relay", ok, relayed)
	}
	return nil
}

func (w *m7World) whenCutDirectToRelay() error {
	if err := w.setScript("claude", m7ClaudeHandbackDoc("Relay transition handback.", nil)); err != nil {
		return err
	}
	relay := make(chan struct{}, 1)
	w.browser.pc.SCTP().Transport().ICETransport().OnSelectedCandidatePairChange(func(pair *webrtc.ICECandidatePair) {
		if pair.Local.Typ == webrtc.ICECandidateTypeRelay {
			select {
			case relay <- struct{}{}:
			default:
			}
		}
	})
	w.browser.ctrl.relayOnly.Store(true)
	close(w.browser.cutDirect)
	if !m7WaitChan(relay, time.After(m7Event)) {
		return errors.New("browser did not select its TURN allocation")
	}
	return nil
}

func (w *m7World) thenStatsRelayClosesPTYAndHandsBack(aiName string) error {
	relayed, ok := w.browser.statsShowRelay()
	if !ok || !relayed {
		return fmt.Errorf("selected-pair stats ok=%v relayed=%v, want relay", ok, relayed)
	}
	if err := w.activePTY.waitClosed(m7Event); err != nil {
		return err
	}
	if got := w.served.Path(); got != "relay" {
		return fmt.Errorf("served path = %q, want relay", got)
	}
	return w.waitMsgAndAIState(aiName, "Relay transition handback.", "waiting", m7Event)
}

func (w *m7World) thenHostInfoOriginalCtl() error {
	defer w.closeOpenSessions()
	if got := w.browser.offersSent.Load(); got != 1 {
		return fmt.Errorf("browser sent %d offers, want 1", got)
	}
	info, err := w.browser.hostInfo()
	if err != nil {
		return err
	}
	if info.Device != w.pn.Key.ID() {
		return fmt.Errorf("host.info device = %q, want %q", info.Device, w.pn.Key.ID())
	}
	return nil
}
