package node

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Ctl is an authenticated ctl data channel; *peer.Connection satisfies it.
type Ctl interface {
	host.Owner
	Read(ctx context.Context) (json.RawMessage, error)
	Close() error
}

type (
	// RequestHandler answers a ctl request; Async handlers write their own reply.
	RequestHandler struct {
		Async bool
		Type  string
		Run   func(ctx context.Context, conn *home.Conn, from Ctl, by string, id int64, data json.RawMessage) (any, error)
	}
	// PushHandler handles a ctl push after enrolment.
	PushHandler func(ctx context.Context, conn *home.Conn, from Ctl, by string, data json.RawMessage)
	// CmdFunc executes a host command after envelope checks and receipt lookup.
	CmdFunc func(ctx context.Context, from Ctl, by string, payload json.RawMessage) (any, error)
)

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
	Spawn       func(func())
}

// Sessions serves ctl connections, dispatches home and host surfaces, forwards
// device logs and records receipts for host commands.
// Lock order: cmdMu -> Store.Write; sesConn.sendMu -> mu -> Logs.mu; mu is a
// leaf lock guarding conns and sesConn.welcomed.
type Sessions struct {
	Requests    map[string]RequestHandler
	Pushes      map[string]PushHandler
	Commands    map[string]CmdFunc
	Replays     map[string]CmdFunc
	cfg         SessionConfig
	cmdMu, mu   sync.Mutex
	inflight    map[string]chan struct{}
	conns       map[*sesConn]bool
	onPairHello func(*sesConn)
	onPairAdopt func(*sesConn)
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
	onSent  func() []proto.Devlog
	sent    chan struct{} // single reader: sesHello
}

// NewSessions constructs a ctl session hub and wires device-log notifications.
func NewSessions(cfg SessionConfig) *Sessions {
	s := &Sessions{
		Requests: map[string]RequestHandler{}, Pushes: map[string]PushHandler{},
		Commands: map[string]CmdFunc{}, Replays: map[string]CmdFunc{},
		cfg: sesDefaultConfig(cfg), inflight: map[string]chan struct{}{}, conns: map[*sesConn]bool{},
	}
	s.Requests["sync"] = RequestHandler{Async: true, Run: s.sesSync}
	s.Requests["fetch"] = RequestHandler{Async: true, Run: s.sesFetch}
	cfg.Logs.mu.Lock()
	cfg.Logs.onChange = s.sesLogChanged
	cfg.Logs.mu.Unlock()
	return s
}

func (s *Sessions) sesConns() []*sesConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Keys(s.conns))
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
	if cfg.Spawn == nil {
		cfg.Spawn = func(fn func()) { go fn() }
	}
	return cfg
}

// Serve runs the ctl session of raw until ctx ends or raw closes.
func (s *Sessions) Serve(ctx context.Context, device string, paired bool, raw Ctl) {
	c := &sesConn{device: device, paired: paired, raw: raw, staging: map[string]*identity.Log{}}
	c.home = s.cfg.Home.Attach(device, c)
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
	s.cfg.Home.Detach(c.home)
	c.raw.Close()
}

func (s *Sessions) statusLines() []string {
	s.mu.Lock()
	var lines []string
	for c := range s.conns {
		if c.welcomed {
			lines = append(lines, fmt.Sprintf("connection %s %s", c.device, cmp.Or(c.raw.Path(), "unknown")))
		}
	}
	s.mu.Unlock()
	slices.Sort(lines)
	return lines
}

func sesOpenFrame(c *sesConn, data json.RawMessage) (string, *int64, bool) {
	var f struct {
		Type *string `json:"t,omitempty"`
		ID   *int64  `json:"id,omitempty"`
		Re   *int64  `json:"re,omitempty"`
	}
	if proto.Decode(data, &f) != nil || f.Type == nil || f.Re != nil {
		c.sesRefuse(data)
		return "", nil, false
	}
	return *f.Type, f.ID, true
}

func (s *Sessions) sesDispatch(ctx context.Context, c *sesConn, data json.RawMessage) {
	typ, id, ok := sesOpenFrame(c, data)
	if !ok {
		return
	}
	if id == nil {
		s.sesPush(ctx, c, typ, data)
		return
	}
	s.sesRequest(ctx, c, *id, typ, data)
}

func (s *Sessions) sesPush(ctx context.Context, c *sesConn, typ string, data json.RawMessage) {
	if typ == "devlog" {
		s.sesDevlog(c, data)
		return
	}
	if !s.sesEnrolled(c) {
		return
	}
	if typ == "ack" {
		s.sesAck(c, data)
		return
	}
	if h := s.Pushes[typ]; h != nil {
		h(ctx, c.home, c.raw, c.device, data)
	}
}

func (s *Sessions) sesAck(c *sesConn, data json.RawMessage) {
	var ack proto.Ack
	if proto.Decode(data, &ack) == nil {
		s.cfg.Home.Ack(c.home, ack)
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
	res, err := h.Run(ctx, c.home, c.raw, c.device, id, data)
	if !h.Async || err != nil {
		s.sesReply(c, id, h.Type, res, err)
	}
}

func (s *Sessions) sesSync(_ context.Context, conn *home.Conn, _ Ctl, _ string, _ int64, data json.RawMessage) (any, error) {
	var req proto.Sync
	if proto.Decode(data, &req) != nil {
		return nil, &host.Error{Code: "bad_request", Message: "invalid sync frame"}
	}
	s.cfg.Home.Sync(conn, req)
	return nil, nil
}

func (s *Sessions) sesFetch(_ context.Context, conn *home.Conn, _ Ctl, _ string, _ int64, data json.RawMessage) (any, error) {
	req, ok := decodeFetch(data)
	if !ok {
		return nil, &host.Error{Code: "bad_request", Message: "invalid fetch frame"}
	}
	s.cfg.Home.Fetch(conn, req)
	return nil, nil
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
	sent := make(chan struct{})
	base := s.sesWelcomeBase(id, c.device)
	s.sesBeforeWelcome(c)
	onSent := func() []proto.Devlog {
		s.mu.Lock()
		c.welcomed = true
		s.mu.Unlock()
		return s.sesWelcomeLogs(c)
	}
	wrap := func(gs []proto.GroupSummary) any {
		w := base
		w.Groups = gs
		return sesWelcome{welcome: w, onSent: onSent, sent: sent}
	}
	s.cfg.Home.Welcome(c.home, wrap)
	sesWaitWelcome(ctx, sent, c.home.Gone())
	if h.Kind == "host" {
		s.cfg.Home.Inbox(c.home)
	}
}

func (s *Sessions) sesBeforeWelcome(c *sesConn) {
	if c.paired && s.onPairHello != nil {
		s.onPairHello(c)
	}
}

func sesDecodeHello(data json.RawMessage) (proto.Hello, *host.Error) {
	var ver struct {
		proto.Request
		V int `json:"v"`
	}
	if proto.Decode(data, &ver) != nil {
		return proto.Hello{}, &host.Error{Code: "bad_request", Message: "invalid hello frame"}
	}
	if ver.V != 1 {
		return proto.Hello{}, &host.Error{Code: "version", Message: "unsupported protocol version"}
	}
	var h proto.Hello
	if proto.Decode(data, &h) != nil || (h.Kind != "browser" && h.Kind != "host") {
		return proto.Hello{}, &host.Error{Code: "bad_request", Message: "invalid hello fields"}
	}
	return h, nil
}

func (s *Sessions) sesWelcomeBase(id int64, device string) proto.Welcome {
	return proto.Welcome{
		Type: "welcome", Success: proto.Success{Re: id, OK: true}, V: 1,
		Device: s.cfg.ID, Name: s.cfg.Name(), Version: s.cfg.Version,
		Time: s.cfg.Now().UnixMilli(), Addrs: s.cfg.Addrs(), Owner: s.cfg.Logs.IsOwner(device),
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
	if sesIsHostCmd(req.Env.Payload) {
		s.cfg.Spawn(func() { s.sesHostCmd(ctx, c, id, req.Env) })
		return
	}
	s.cfg.Home.Cmd(c.home, id, req.Env)
}

func sesIsHostCmd(payload string) bool {
	var p struct {
		To *string `json:"to,omitempty"`
	}
	proto.Decode([]byte(payload), &p)
	return p.To != nil
}

func (s *Sessions) sesHostCmd(ctx context.Context, c *sesConn, id int64, env proto.Envelope) {
	hdr, err := identity.OpenCommand(env, s.cfg.ID, s.cfg.Logs.Known, s.cfg.Now())
	if err != nil {
		c.sendErr(id, err.Error(), "the envelope of the command was refused")
		return
	}
	held, done, ok := s.sesBeginHostCmd(ctx, c, id, env.Key, string(hdr.Nonce))
	if !ok {
		return
	}
	if done == nil {
		s.sesReplayHostCmd(ctx, c, id, hdr.Type, env, held)
		return
	}
	s.sesExecHostCmd(ctx, c, id, hdr, env, done)
}

func (s *Sessions) sesBeginHostCmd(ctx context.Context, c *sesConn, id int64, signer, nonce string) (store.Outcome, chan struct{}, bool) {
	key := signer + "\x00" + nonce
	for {
		s.cmdMu.Lock()
		if wait := s.inflight[key]; wait != nil {
			s.cmdMu.Unlock()
			if !sesWaitInflight(ctx, wait) {
				return store.Outcome{}, nil, false
			}
			continue
		}
		held, done, err := s.sesClaimReceipt(key, signer, nonce)
		s.cmdMu.Unlock()
		if err != nil {
			s.cfg.Report(err)
			c.sendErr(id, "internal", "could not read command receipt")
			return store.Outcome{}, nil, false
		}
		return held, done, true
	}
}

func (s *Sessions) sesClaimReceipt(key, signer, nonce string) (store.Outcome, chan struct{}, error) {
	held, found, err := s.sesReadReceipt(signer, nonce)
	if err != nil || found {
		return held, nil, err
	}
	done := make(chan struct{})
	s.inflight[key] = done
	return store.Outcome{}, done, nil
}

func sesWaitInflight(ctx context.Context, wait <-chan struct{}) bool {
	select {
	case <-wait:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Sessions) sesReadReceipt(signer, nonce string) (store.Outcome, bool, error) {
	var held store.Outcome
	var found bool
	read := func(tx *store.Tx) error {
		out, err := tx.Receipt(signer, nonce)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		held, found = out, true
		return nil
	}
	return held, found, s.cfg.Store.Write(read)
}

func (s *Sessions) sesReplayHostCmd(ctx context.Context, c *sesConn, id int64, typ string, env proto.Envelope, held store.Outcome) {
	if held.Code != "" {
		c.sendErr(id, held.Code, "the command was refused when it first arrived")
		return
	}
	if fn := s.Replays[typ]; fn != nil {
		res, err := fn(ctx, c.raw, env.Key, []byte(env.Payload))
		s.sesReply(c, id, "", res, err)
		return
	}
	c.send(proto.Success{Re: id, OK: true})
}

func (s *Sessions) sesExecHostCmd(ctx context.Context, c *sesConn, id int64, hdr proto.PayloadHeader, env proto.Envelope, done chan struct{}) {
	var res any
	err := error(&host.Error{Code: "bad_request", Message: "unknown host command"})
	if fn := s.sesHostCmdFn(hdr.Type); fn != nil {
		res, err = fn(ctx, c.raw, env.Key, []byte(env.Payload))
	}
	if !s.sesFinishHostCmd(c, id, hdr, env.Key, err, done) {
		return
	}
	s.sesReply(c, id, "", res, err)
}

func (s *Sessions) sesFinishHostCmd(c *sesConn, id int64, hdr proto.PayloadHeader, signer string, cmdErr error, done chan struct{}) bool {
	key := signer + "\x00" + string(hdr.Nonce)
	outcome, ok := sesOutcome(cmdErr)
	var dbErr error
	s.cmdMu.Lock()
	if ok {
		write := func(tx *store.Tx) error { return tx.PutReceipt(signer, string(hdr.Nonce), hdr.Timestamp, outcome) }
		dbErr = s.cfg.Store.Write(write)
	}
	delete(s.inflight, key)
	close(done)
	s.cmdMu.Unlock()
	if dbErr != nil {
		s.cfg.Report(dbErr)
		c.sendErr(id, "internal", "could not store command receipt")
		return false
	}
	return true
}

func sesOutcome(err error) (store.Outcome, bool) {
	if err == nil {
		return store.Outcome{OK: true}, true
	}
	var wireErr *host.Error
	if errors.As(err, &wireErr) {
		return store.Outcome{Code: wireErr.Code}, true
	}
	return store.Outcome{}, false
}

func (s *Sessions) sesReply(c *sesConn, id int64, typ string, res any, err error) {
	var wireErr *host.Error
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
	raw, err := encodeCtl(res)
	if err != nil {
		return nil, err
	}
	return fields, json.Unmarshal(raw, &fields)
}

func (s *Sessions) sesHostCmdFn(t string) CmdFunc {
	if fn := s.Commands[t]; fn != nil {
		return fn
	}
	if surf := host.Surface[t]; surf != nil {
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
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if err := c.raw.Send(w.welcome); err != nil {
		return err
	}
	for _, dl := range w.onSent() {
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
	proto.Decode(data, &r)
	if r.ID != nil {
		c.sendErr(*r.ID, "bad_request", "the message is not a ctl frame of the contract")
	}
}
