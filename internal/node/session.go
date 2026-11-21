package node

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Error is a wire failure carrying one of the codes of datachannel.md section 9.
type Error = host.Error

// Ctl is an authenticated ctl data channel; *peer.Connection satisfies it.
type Ctl interface {
	host.Owner
	Read(ctx context.Context) (json.RawMessage, error)
	Close() error
}

var _ Ctl = (*peer.Connection)(nil)

type (
	// RequestHandler answers a ctl request; Async handlers write their own reply.
	RequestHandler struct {
		Owner, Async bool
		Type         string
		Run          func(ctx context.Context, conn *home.Conn, from Ctl, by string, id int64, data json.RawMessage) (any, error)
	}
	// PushHandler handles a ctl push after enrolment.
	PushHandler struct {
		Owner bool
		Run   func(ctx context.Context, conn *home.Conn, from Ctl, by string, data json.RawMessage)
	}
	// CmdFunc executes a host command after envelope checks and receipt lookup.
	CmdFunc func(ctx context.Context, from Ctl, by string, payload json.RawMessage) (any, error)
	// ReplayFunc answers a repeated once-in-memory host command (pair.browser or takeover.start).
	ReplayFunc func(ctx context.Context, from Ctl, by, nonce string, payload json.RawMessage) (any, error)
)

var sesKnownHostCmds = map[string]bool{
	"agent.set": true, "perm.set": true, "settings.set": true,
	"host.join": true, "host.leave": true, "takeover.start": true,
	"takeover.end": true, "host.offline": true, "host.online": true, "pair.browser": true,
}

// SessionConfig configures the ctl session hub of a node.
type SessionConfig struct {
	ID, Version string
	Store       *store.Store
	Logs        *Logs
	Home        *home.Loop
	Host        *host.Host
	Name        func() string
	Addrs       func() []string
	Now         func() time.Time
	Report      func(error)
	OnAdopt     func(owner string)
}

// Sessions serves ctl connections, dispatches home and host surfaces, forwards
// device logs and records receipts for host commands.
// Lock order: cmdMu -> Store.Write / handlers / mu; mu is a leaf lock guarding
// conns; sesConn.sendMu is a leaf lock serializing writes on one Ctl connection.
type Sessions struct {
	Requests  map[string]RequestHandler
	Pushes    map[string]PushHandler
	Commands  map[string]CmdFunc
	Replays   map[string]ReplayFunc
	cfg       SessionConfig
	cmdMu, mu sync.Mutex
	conns     map[*sesConn]bool
}

type sesConn struct {
	device           string
	paired, welcomed bool
	raw              Ctl
	sendMu           sync.Mutex
	home             *home.Conn
	staging          map[string]*identity.Log
}

type sesWelcome struct {
	welcome proto.Welcome
	build   func([]proto.GroupSummary) []proto.Devlog
	sent    chan struct{} // single reader: sesHello
}

// NewSessions constructs a ctl session hub and wires device-log notifications.
func NewSessions(cfg SessionConfig) *Sessions {
	s := &Sessions{
		Requests: map[string]RequestHandler{"host.info": {Owner: true, Type: "host.info"}, "audit": {Owner: true}},
		Pushes:   map[string]PushHandler{}, Commands: map[string]CmdFunc{}, Replays: map[string]ReplayFunc{},
		cfg: sesDefaultConfig(cfg), conns: map[*sesConn]bool{},
	}
	if cfg.Host != nil {
		s.Requests["audit"] = RequestHandler{Owner: true, Run: s.sesRunAudit}
		s.Pushes["pty.resize"] = PushHandler{Owner: true, Run: s.sesRunResize}
	}
	cfg.Logs.mu.Lock()
	cfg.Logs.onChange = s.sesLogChanged
	cfg.Logs.mu.Unlock()
	return s
}
func sesDefaultConfig(cfg SessionConfig) SessionConfig {
	cfg.Version = cmp.Or(cfg.Version, Version)
	if cfg.Name == nil {
		cfg.Name = func() string { return "" }
	}
	if cfg.Addrs == nil {
		cfg.Addrs = func() []string { return []string{} }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Report == nil {
		cfg.Report = func(err error) { log.Printf("node: %v", err) }
	}
	return cfg
}

// Serve runs the ctl session of raw until ctx ends or raw closes.
func (s *Sessions) Serve(ctx context.Context, device string, paired bool, raw Ctl) {
	if !paired && !s.cfg.Logs.Known(device) {
		raw.Close()
		return
	}
	c := &sesConn{device: device, paired: paired, raw: raw, staging: map[string]*identity.Log{}}
	if s.cfg.Home != nil {
		c.home = s.cfg.Home.Attach(device, c)
	}
	s.mu.Lock()
	s.conns[c] = true
	s.mu.Unlock()
	defer s.sesLeave(c)
	for {
		data, err := raw.Read(ctx)
		if err != nil {
			return
		}
		s.sesDispatch(ctx, c, data)
	}
}
func (s *Sessions) sesLeave(c *sesConn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	if c.home != nil {
		s.cfg.Home.Detach(c.home)
	}
	c.raw.Close()
}

func (s *Sessions) sesDispatch(ctx context.Context, c *sesConn, data json.RawMessage) {
	var f struct {
		Type *string `json:"t,omitempty"`
		ID   *int64  `json:"id,omitempty"`
		Re   *int64  `json:"re,omitempty"`
	}
	if proto.Decode(data, &f) != nil || f.Type == nil || f.Re != nil {
		c.sesRefuse(data)
		return
	}
	if f.ID == nil {
		s.sesPush(ctx, c, *f.Type, data)
		return
	}
	s.sesRequest(ctx, c, *f.ID, *f.Type, data)
}
func (s *Sessions) sesPush(ctx context.Context, c *sesConn, typ string, data json.RawMessage) {
	if typ == "devlog" {
		s.sesDevlog(c, data)
		return
	}
	if h, ok := s.Pushes[typ]; ok && s.sesEnrolled(c) && (!h.Owner || s.cfg.Logs.IsOwner(c.device)) {
		h.Run(ctx, c.home, c.raw, c.device, data)
	}
}
func (s *Sessions) sesRequest(ctx context.Context, c *sesConn, id int64, typ string, data json.RawMessage) {
	if typ == "hello" {
		s.sesHello(ctx, c, id, data)
		return
	}
	if !s.sesEnrolled(c) {
		c.sendErr(id, "forbidden", "only hello and devlog are accepted before enrolment")
		return
	}
	if typ == "cmd" {
		s.sesCmd(ctx, c, id, data)
		return
	}
	s.sesRunRequest(ctx, c, id, typ, data)
}
func (s *Sessions) sesRunRequest(ctx context.Context, c *sesConn, id int64, typ string, data json.RawMessage) {
	h, ok := s.Requests[typ]
	if !ok {
		c.sendErr(id, "bad_request", "unknown ctl request")
		return
	}
	if h.Owner && !s.cfg.Logs.IsOwner(c.device) {
		c.sendErr(id, "forbidden", "host surface requires an owner device")
		return
	}
	if h.Run == nil {
		c.sendErr(id, "bad_request", "request handler is unavailable")
		return
	}
	res, err := h.Run(ctx, c.home, c.raw, c.device, id, data)
	if !h.Async {
		s.sesReply(c, id, h.Type, res, err)
	}
}
func (s *Sessions) sesEnrolled(c *sesConn) bool {
	s.mu.Lock()
	welcomed := c.welcomed
	s.mu.Unlock()
	return welcomed && s.cfg.Logs.Known(c.device)
}

func (s *Sessions) sesHello(ctx context.Context, c *sesConn, id int64, data json.RawMessage) {
	h, refusal := sesDecodeHello(data)
	if refusal != nil {
		c.sendErr(id, refusal.Code, refusal.Message)
		return
	}
	s.mu.Lock()
	c.welcomed = true
	s.mu.Unlock()
	sent := make(chan struct{})
	base := s.sesWelcomeBase(id, c.device)
	wrap := func(gs []proto.GroupSummary) any {
		w := base
		w.Groups = gs
		return sesWelcome{welcome: w, build: func(g []proto.GroupSummary) []proto.Devlog { return s.sesWelcomeLogs(c, g) }, sent: sent}
	}
	if c.home == nil {
		c.Send(wrap([]proto.GroupSummary{}))
		return
	}
	s.cfg.Home.Welcome(c.home, wrap)
	sesWaitWelcome(ctx, sent, c.home.Gone())
	if h.Kind == "host" {
		s.cfg.Home.Inbox(c.home)
	}
}
func sesDecodeHello(data json.RawMessage) (proto.Hello, *Error) {
	var ver struct {
		proto.Request
		V int `json:"v"`
	}
	if proto.Decode(data, &ver) != nil {
		return proto.Hello{}, &Error{Code: "bad_request", Message: "invalid hello frame"}
	}
	if ver.V != 1 {
		return proto.Hello{}, &Error{Code: "version", Message: "unsupported protocol version"}
	}
	var h proto.Hello
	if proto.Decode(data, &h) != nil || (h.Kind != "browser" && h.Kind != "host") {
		return proto.Hello{}, &Error{Code: "bad_request", Message: "invalid hello fields"}
	}
	return h, nil
}
func (s *Sessions) sesWelcomeBase(id int64, device string) proto.Welcome {
	addrs := s.cfg.Addrs()
	if addrs == nil {
		addrs = []string{}
	}
	return proto.Welcome{
		Type: "welcome", Success: proto.Success{Re: id, OK: true}, V: 1,
		Device: s.cfg.ID, Name: s.cfg.Name(), Version: s.cfg.Version,
		Time: s.cfg.Now().UnixMilli(), Addrs: addrs, Owner: s.cfg.Logs.IsOwner(device),
	}
}
func sesWaitWelcome(ctx context.Context, sent, gone <-chan struct{}) {
	select {
	case <-sent:
		return
	case <-gone:
		return
	case <-ctx.Done():
		return
	}
}

func (s *Sessions) sesCmd(ctx context.Context, c *sesConn, id int64, data json.RawMessage) {
	var req proto.Cmd
	if proto.Decode(data, &req) != nil {
		c.sendErr(id, "bad_request", "invalid cmd frame")
		return
	}
	if req.Env.Key != c.device {
		c.sendErr(id, "forbidden", "command signer does not match connection device")
		return
	}
	if s.sesIsHostCmd(req.Env.Payload) {
		s.sesHostCmd(ctx, c, id, req.Env)
		return
	}
	if c.home == nil {
		c.sendErr(id, "not_found", "this node is not home to the group")
		return
	}
	s.cfg.Home.Cmd(c.home, id, req.Env)
}
func (s *Sessions) sesIsHostCmd(payload string) bool {
	var p struct {
		Type  string    `json:"t"`
		Group *proto.ID `json:"g,omitempty"`
		To    *string   `json:"to,omitempty"`
	}
	if proto.Decode([]byte(payload), &p) != nil {
		return false
	}
	return sesKnownHostCmds[p.Type] || s.sesHostCmdFn(p.Type) != nil || (p.To != nil && p.Group == nil)
}

func (s *Sessions) sesHostCmd(ctx context.Context, c *sesConn, id int64, env proto.Envelope) {
	hdr, err := sesOpenHostCmd(env, s.cfg.ID, s.cfg.Logs.Known, s.cfg.Now())
	if err != nil {
		c.sendErr(id, err.Error(), "the envelope of the command was refused")
		return
	}
	if !s.cfg.Logs.IsOwner(env.Key) {
		c.sendErr(id, "forbidden", "host commands require an owner device")
		return
	}
	s.cmdMu.Lock()
	defer s.cmdMu.Unlock()
	held, found, err := s.sesReadReceipt(env.Key, string(hdr.Nonce))
	if err != nil {
		s.cfg.Report(err)
		c.sendErr(id, "internal", "could not read command receipt")
		return
	}
	if found {
		s.sesReplayHostCmd(ctx, c, id, hdr.Type, env, string(hdr.Nonce), held)
		return
	}
	s.sesExecHostCmd(ctx, c, id, hdr, env)
}
func sesOpenHostCmd(env proto.Envelope, me string, known func(string) bool, now time.Time) (proto.PayloadHeader, error) {
	var p struct {
		To *string `json:"to"`
	}
	json.Unmarshal([]byte(env.Payload), &p)
	hdr, err := identity.OpenCommand(env, me, known, now)
	if (err == nil || errors.Is(err, identity.Stale)) && p.To == nil {
		return proto.PayloadHeader{}, identity.NotForMe
	}
	return hdr, err
}
func (s *Sessions) sesReadReceipt(signer, nonce string) (store.Outcome, bool, error) {
	var held store.Outcome
	var err error
	read := func(tx *store.Tx) error {
		held, err = tx.Receipt(signer, nonce)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	dbErr := s.cfg.Store.Write(read)
	return held, err == nil && dbErr == nil, dbErr
}
func (s *Sessions) sesReplayHostCmd(ctx context.Context, c *sesConn, id int64, typ string, env proto.Envelope, nonce string, held store.Outcome) {
	if held.Code != "" {
		c.sendErr(id, held.Code, "the command was refused when it first arrived")
		return
	}
	if replay := s.Replays[typ]; replay != nil {
		res, err := replay(ctx, c.raw, env.Key, nonce, []byte(env.Payload))
		s.sesReply(c, id, "", res, err)
		return
	}
	if typ == "pair.browser" || typ == "takeover.start" {
		c.sendErr(id, "conflict", "the one-time resource is no longer active")
		return
	}
	c.send(proto.Success{Re: id, OK: true})
}
func (s *Sessions) sesExecHostCmd(ctx context.Context, c *sesConn, id int64, hdr proto.PayloadHeader, env proto.Envelope) {
	var res any
	err := error(&Error{Code: "bad_request", Message: "unknown host command"})
	if fn := s.sesHostCmdFn(hdr.Type); fn != nil {
		res, err = fn(ctx, c.raw, env.Key, []byte(env.Payload))
	}
	outcome, ok := sesOutcome(err)
	if !ok {
		s.cfg.Report(err)
		c.sendErr(id, "internal", "host command failed")
		return
	}
	write := func(tx *store.Tx) error { return tx.PutReceipt(env.Key, string(hdr.Nonce), hdr.Timestamp, outcome) }
	if dbErr := s.cfg.Store.Write(write); dbErr != nil {
		s.cfg.Report(dbErr)
		c.sendErr(id, "internal", "could not store command receipt")
		return
	}
	s.sesReply(c, id, "", res, err)
}
func sesOutcome(err error) (store.Outcome, bool) {
	if err == nil {
		return store.Outcome{OK: true}, true
	}
	var wireErr *Error
	if errors.As(err, &wireErr) {
		return store.Outcome{Code: wireErr.Code}, true
	}
	return store.Outcome{}, false
}

func (s *Sessions) sesReply(c *sesConn, id int64, typ string, res any, err error) {
	var wireErr *Error
	if errors.As(err, &wireErr) {
		c.sendErr(id, wireErr.Code, wireErr.Message)
		return
	}
	if err != nil {
		s.cfg.Report(err)
		c.sendErr(id, "internal", "request failed")
		return
	}
	frame, encErr := sesBuildAnswer(id, typ, res)
	if encErr != nil {
		s.cfg.Report(encErr)
		c.sendErr(id, "internal", "could not encode answer")
		return
	}
	c.send(frame)
}
func sesBuildAnswer(re int64, typ string, res any) (any, error) {
	if res == nil && typ == "" {
		return proto.Success{Re: re, OK: true}, nil
	}
	fields, err := sesAnswerFields(res)
	if err != nil {
		return nil, err
	}
	fields["re"], fields["ok"] = json.RawMessage(strconv.FormatInt(re, 10)), json.RawMessage("true")
	delete(fields, "t")
	if typ != "" {
		fields["t"] = dvlMustEncode(typ)
	}
	return fields, nil
}
func sesAnswerFields(res any) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if res == nil {
		return fields, nil
	}
	raw, err := dvlEncode(res)
	if err != nil {
		return nil, err
	}
	return fields, json.Unmarshal(raw, &fields)
}

func (s *Sessions) sesRunAudit(ctx context.Context, _ *home.Conn, from Ctl, by string, _ int64, data json.RawMessage) (any, error) {
	if fn := host.Surface["audit"]; fn != nil {
		return fn(ctx, s.cfg.Host, from, by, data)
	}
	return nil, &Error{Code: "bad_request", Message: "audit is unavailable"}
}
func (s *Sessions) sesRunResize(ctx context.Context, _ *home.Conn, from Ctl, by string, data json.RawMessage) {
	if fn := host.Surface["pty.resize"]; fn != nil {
		fn(ctx, s.cfg.Host, from, by, data)
	}
}
func (s *Sessions) sesHostCmdFn(t string) CmdFunc {
	if fn := s.Commands[t]; fn != nil {
		return fn
	}
	if s.cfg.Host != nil && sesKnownHostCmds[t] && host.Surface[t] != nil {
		surf := host.Surface[t]
		return func(ctx context.Context, from Ctl, by string, payload json.RawMessage) (any, error) {
			return surf(ctx, s.cfg.Host, from, by, payload)
		}
	}
	return nil
}

func (c *sesConn) Done() <-chan struct{} { return c.raw.Done() }
func (c *sesConn) Close() error          { return c.raw.Close() }
func (c *sesConn) send(frame any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.raw.Send(frame)
}
func (c *sesConn) sendErr(re int64, code, msg string) {
	c.send(proto.ErrorResponse{Re: re, Err: proto.WireError{Code: code, Message: msg}})
}
func (c *sesConn) Send(frame any) error {
	w, ok := frame.(sesWelcome)
	if !ok {
		return c.send(frame)
	}
	defer close(w.sent)
	logs := w.build(w.welcome.Groups)
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if err := c.raw.Send(w.welcome); err != nil {
		return err
	}
	for _, dl := range logs {
		if err := c.raw.Send(dl); err != nil {
			return err
		}
	}
	return nil
}
func (c *sesConn) sesRefuse(data []byte) {
	var r struct {
		ID *int64 `json:"id,omitempty"`
	}
	if proto.Decode(data, &r) == nil && r.ID != nil {
		c.sendErr(*r.ID, "bad_request", "the message is not a ctl frame of the contract")
	}
}
