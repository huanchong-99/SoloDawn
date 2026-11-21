package node

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const pairWindow = 10 * time.Minute

var (
	errNoRendezvous = errors.New("no rendezvous URL configured")
	errRoomFull     = errors.New("pairing room is full")
	errPairExpired  = errors.New("pairing secret expired")
	errPairReplaced = errors.New("pairing superseded by a newer attempt")
	pairSchemes     = map[string]string{"wss": "https", "https": "https", "ws": "http", "http": "http"}
)

type (
	// PairLink is the answer to pair.browser and PairBrowser (flows/pairing.md C).
	PairLink struct {
		Link    string `json:"link"`
		Expires int64  `json:"expires"`
	}
	// PairResult reports the peer and path enrolled by Pair (flows/pairing.md A.13).
	PairResult struct {
		Device, Name, Fingerprint, Path string
	}
	// PairConfig configures room pairing and pair.browser on a node.
	PairConfig struct {
		Key          identity.Key
		Store        *store.Store
		Logs         *Logs
		Sessions     *Sessions
		Peer         *peer.Peer
		Paths        func() []*peer.Path
		Name, WebURL func() string
		Rendezvous   func() []string
		Now          func() time.Time
		After        func(time.Duration) <-chan time.Time
	}
	// Pairer runs pairing rooms in both roles and serves pair.browser.
	// Lock order: mu is a leaf lock guarding attempts and links; it is never held
	// across Store.Write, Logs, Sessions, Peer or WebSocket calls.
	Pairer struct {
		Peer     *peer.Peer
		cfg      PairConfig
		mu       sync.Mutex
		attempts map[*pairAttempt]bool
		links    map[string]*pairAttempt
	}
	pairAttempt struct {
		role, wantRole, room, label, pathUsed string
		secret                                identity.Secret
		link                                  PairLink
		rawSelf                               json.RawMessage
		cancel                                context.CancelFunc
		ownedPaths                            []*peer.Path
		routes                                []peer.Route
		peer                                  proto.Pair
		conn                                  Ctl
		done                                  bool
		pathsLeft                             int
		err                                   error
		outcome                               chan error // single reader: waitPair (role "b") or watchBrowser (role "a")
	}
)

// NewPairer constructs a Pairer and wires pair.browser into cfg.Sessions.
func NewPairer(cfg PairConfig) *Pairer {
	if cfg.Name == nil {
		cfg.Name = func() string { return ReadName(cfg.Store) }
	}
	if cfg.Rendezvous == nil {
		cfg.Rendezvous = func() []string { return ReadRendezvous(cfg.Store) }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.After == nil {
		cfg.After = time.After
	}
	p := &Pairer{Peer: cfg.Peer, cfg: cfg, attempts: map[*pairAttempt]bool{}, links: map[string]*pairAttempt{}}
	cfg.Sessions.Commands["pair.browser"] = p.cmdPairBrowser
	cfg.Sessions.Replays["pair.browser"] = p.replayPairBrowser
	cfg.Sessions.onPairHello, cfg.Sessions.onPairAdopt = p.onPairHello, p.onPairAdopt
	return p
}

// Close ends every open pairing attempt and discards its secret.
func (p *Pairer) Close() {
	p.mu.Lock()
	open := slices.Collect(maps.Keys(p.attempts))
	p.mu.Unlock()
	for _, a := range open {
		p.finish(a, net.ErrClosed)
	}
}

// Known reports whether device completed a room exchange on an active attempt.
func (p *Pairer) Known(device string) bool { return len(p.Routes(device)) > 0 }

// Routes returns the active signalling routes for a pairing peer, or nil.
func (p *Pairer) Routes(device string) []peer.Route {
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	defer p.mu.Unlock()
	if a := p.findPeerLocked(device, now); a != nil {
		return slices.Clone(a.routes)
	}
	return nil
}

// Serve runs a ctl session on Sessions, marking it paired when device matches
// an active room exchange.
func (p *Pairer) Serve(ctx context.Context, device string, raw Ctl) {
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	a := p.findPeerLocked(device, now)
	if a != nil {
		a.conn = raw
	}
	p.mu.Unlock()
	p.cfg.Sessions.Serve(ctx, device, a != nil, raw)
	if a != nil {
		p.finish(a, io.EOF)
	}
}

// HandleEvent processes a non-signalling rendezvous frame ("joined", "full" or
// "room") received on path.
func (p *Pairer) HandleEvent(ctx context.Context, path *peer.Path, kind string, data []byte) {
	var f proto.RoomID
	proto.Decode(data, &f)
	a := p.findRoom(f.Room)
	if a == nil {
		return
	}
	switch kind {
	case "joined":
		path.Write(ctx, proto.RoomData{RoomID: proto.RoomID{Type: "room", Room: f.Room}, Data: a.rawSelf})
	case "full":
		p.finish(a, errRoomFull)
	case "room":
		p.onPairMsg(a, data)
	}
}

// Pair joins the room of req.Secret in role "b" and waits for the browser to
// adopt this unowned node (flows/pairing.md A, B).
func (p *Pairer) Pair(ctx context.Context, req PairRequest) (PairResult, error) {
	if p.cfg.Logs.Owner() != "" {
		return PairResult{}, ErrOwned
	}
	a, err := p.openRoleB(ctx, req)
	if err != nil {
		return PairResult{}, err
	}
	if err := p.waitPair(ctx, a); err != nil {
		return PairResult{}, err
	}
	p.mu.Lock()
	res := PairResult{Device: a.peer.Key, Name: a.label, Fingerprint: p.cfg.Key.Fingerprint(), Path: a.pathUsed}
	p.mu.Unlock()
	return res, nil
}

func (p *Pairer) openRoleB(ctx context.Context, req PairRequest) (*pairAttempt, error) {
	secret, err := identity.ParseSecret(req.Secret)
	if err != nil {
		return nil, err
	}
	label, err := p.prepareLabel(req.Name)
	if err != nil {
		return nil, err
	}
	p.cancelRoleB()
	var shared []*peer.Path
	urls := req.Rendezvous
	if len(urls) == 0 {
		shared, urls = p.sharedPaths(), p.cfg.Rendezvous()
	}
	return p.openAttempt(ctx, "b", "a", secret, label, "", shared, urls)
}

// PairBrowser creates a single-use secret, joins its room in role "a", and
// returns the 10-minute #pair=<secret> link (flows/pairing.md C).
func (p *Pairer) PairBrowser(ctx context.Context, webURL string) (PairLink, error) {
	a, err := p.startBrowser(ctx, webURL)
	if err != nil {
		return PairLink{}, err
	}
	return a.link, nil
}

func (p *Pairer) cmdPairBrowser(ctx context.Context, _ Ctl, by string, payload json.RawMessage) (any, error) {
	var cmd proto.HostCommand
	if err := proto.Decode(payload, &cmd); err != nil {
		return nil, &host.Error{Code: "bad_request", Message: err.Error()}
	}
	webURL := ""
	if p.cfg.WebURL != nil {
		webURL = p.cfg.WebURL()
	}
	a, err := p.startBrowser(ctx, webURL)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.links[by+"\x00"+string(cmd.Nonce)] = a
	p.mu.Unlock()
	return a.link, nil
}

func (p *Pairer) replayPairBrowser(_ context.Context, _ Ctl, by string, payload json.RawMessage) (any, error) {
	var cmd proto.HostCommand
	proto.Decode(payload, &cmd)
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	a := p.links[by+"\x00"+string(cmd.Nonce)]
	valid := pairLive(a, now)
	p.mu.Unlock()
	if !valid {
		if a != nil {
			p.finish(a, errPairExpired)
		}
		return nil, &host.Error{Code: "conflict", Message: "pairing link is no longer valid"}
	}
	return a.link, nil
}

func (p *Pairer) startBrowser(ctx context.Context, webURL string) (*pairAttempt, error) {
	if p.cfg.Logs.Owner() == "" {
		return nil, &host.Error{Code: "conflict", Message: "node has no owner"}
	}
	urls := p.cfg.Rendezvous()
	base, err := pairWebBase(webURL, urls)
	if err != nil {
		return nil, err
	}
	secret := identity.NewSecret()
	a, err := p.openAttempt(ctx, "a", "b", secret, p.cfg.Name(), base+"/#pair="+secret.String(), p.sharedPaths(), urls)
	if err == nil {
		go p.watchBrowser(a)
	}
	return a, err
}

func (p *Pairer) sharedPaths() []*peer.Path {
	if p.cfg.Paths != nil {
		return p.cfg.Paths()
	}
	return nil
}

func (p *Pairer) watchBrowser(a *pairAttempt) {
	select {
	case <-a.outcome:
		return
	case <-p.cfg.After(pairWindow):
		p.finish(a, errPairExpired)
	}
}

func (p *Pairer) waitPair(ctx context.Context, a *pairAttempt) error {
	select {
	case err := <-a.outcome:
		return err
	case <-p.cfg.After(pairWindow):
		return p.finish(a, errPairExpired)
	case <-ctx.Done():
		return p.finish(a, ctx.Err())
	}
}

func (p *Pairer) prepareLabel(name string) (string, error) {
	if name == "" {
		return p.cfg.Name(), nil
	}
	raw := string(dvlMustEncode(name))
	return name, p.cfg.Store.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('name', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", raw)
		return err
	})
}

func (p *Pairer) cancelRoleB() {
	p.mu.Lock()
	var old []*pairAttempt
	for a := range p.attempts {
		if a.role == "b" {
			old = append(old, a)
		}
	}
	p.mu.Unlock()
	for _, a := range old {
		p.finish(a, errPairReplaced)
	}
}

func (p *Pairer) openAttempt(dialCtx context.Context, role, wantRole string, secret identity.Secret, label, linkURL string, shared []*peer.Path, urls []string) (*pairAttempt, error) {
	paths, dialed, err := shared, len(shared) == 0, error(nil)
	if dialed {
		paths, err = p.dialPaths(dialCtx, urls)
		if len(paths) == 0 {
			return nil, cmp.Or(err, errNoRendezvous)
		}
	}
	runCtx, cancel := context.WithCancel(context.Background())
	a := p.registerAttempt(role, wantRole, secret, label, linkURL, cancel, dialed, paths)
	for _, path := range paths {
		path.Write(runCtx, proto.RoomID{Type: "join", Room: a.room})
		if dialed {
			go p.servePath(runCtx, a, path)
		}
	}
	return a, nil
}

func (p *Pairer) registerAttempt(role, wantRole string, secret identity.Secret, label, linkURL string, cancel context.CancelFunc, dialed bool, paths []*peer.Path) *pairAttempt {
	routes := make([]peer.Route, len(paths))
	for i, path := range paths {
		routes[i] = path.Route()
	}
	self := proto.Pair{Type: "pair", Key: p.cfg.Key.ID(), Kind: "node", Name: label, Role: role}
	self.MAC = secret.MAC(self)
	a := &pairAttempt{
		role: role, wantRole: wantRole, secret: secret, room: secret.Room(), label: label,
		link:    PairLink{Link: linkURL, Expires: p.cfg.Now().Add(pairWindow).UnixMilli()},
		rawSelf: dvlMustEncode(self), cancel: cancel, routes: routes, outcome: make(chan error, 1),
	}
	if dialed {
		a.ownedPaths, a.pathsLeft = paths, len(paths)
	}
	p.mu.Lock()
	p.attempts[a] = true
	p.mu.Unlock()
	return a
}

func (p *Pairer) dialPaths(ctx context.Context, urls []string) ([]*peer.Path, error) {
	var paths []*peer.Path
	var firstErr error
	for _, u := range urls {
		path, err := peer.Dial(ctx, u, p.cfg.Key)
		if err != nil {
			firstErr = cmp.Or(firstErr, err)
			continue
		}
		paths = append(paths, path)
	}
	return paths, firstErr
}

func (p *Pairer) servePath(ctx context.Context, a *pairAttempt, path *peer.Path) {
	err := path.Serve(ctx, p.Peer, func(kind string, data []byte) error {
		p.HandleEvent(ctx, path, kind, data)
		return nil
	})
	p.mu.Lock()
	a.pathsLeft--
	fail := a.pathsLeft == 0 && a.conn == nil
	p.mu.Unlock()
	if fail {
		p.finish(a, err)
	}
}

func (p *Pairer) findRoom(room string) *pairAttempt {
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	defer p.mu.Unlock()
	for a := range p.attempts {
		if pairLive(a, now) && a.room == room {
			return a
		}
	}
	return nil
}

func (p *Pairer) onPairMsg(a *pairAttempt, data []byte) {
	var rd proto.RoomData
	var m proto.Pair
	if proto.Decode(data, &rd) != nil || proto.Decode(rd.Data, &m) != nil {
		return
	}
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	defer p.mu.Unlock()
	if pairLive(a, now) && a.peer.Key == "" && a.secret.Verify(m, a.wantRole) {
		a.peer = m
	}
}

func (p *Pairer) onPairHello(c *sesConn) {
	now := p.cfg.Now().UnixMilli()
	a := p.takeRole("a", c.device, "", now)
	if a == nil {
		return
	}
	if err := p.cfg.Logs.dvlAppendAdd(p.cfg.Key, a.peer.Key, a.peer.Kind, a.peer.Name, now, c); err != nil {
		p.cfg.Sessions.cfg.Report(err)
	}
}

func (p *Pairer) onPairAdopt(c *sesConn) {
	if c != nil {
		p.takeRole("b", c.device, c.raw.Path(), p.cfg.Now().UnixMilli())
	}
}

func (p *Pairer) takeRole(role, device, pathUsed string, now int64) *pairAttempt {
	p.mu.Lock()
	a := p.findPeerLocked(device, now)
	if a == nil || a.role != role {
		p.mu.Unlock()
		return nil
	}
	a.pathUsed = pathUsed
	p.mu.Unlock()
	p.finish(a, nil)
	return a
}

func (p *Pairer) findPeerLocked(device string, now int64) *pairAttempt {
	for a := range p.attempts {
		if pairLive(a, now) && a.peer.Key == device {
			return a
		}
	}
	return nil
}

func pairLive(a *pairAttempt, now int64) bool {
	return a != nil && !a.done && now < a.link.Expires
}

func (p *Pairer) finish(a *pairAttempt, err error) error {
	p.mu.Lock()
	if a.done {
		first := a.err
		p.mu.Unlock()
		return first
	}
	a.done, a.err, a.secret = true, err, ""
	delete(p.attempts, a)
	paths := a.ownedPaths
	a.ownedPaths, a.routes = nil, nil
	p.mu.Unlock()
	a.cancel()
	for _, path := range paths {
		path.Close()
	}
	a.outcome <- err
	return err
}

func pairWebBase(webURL string, rvURLs []string) (string, error) {
	if webURL != "" {
		return strings.TrimRight(webURL, "/"), nil
	}
	if len(rvURLs) == 0 {
		return "", errNoRendezvous
	}
	u, err := url.Parse(rvURLs[0])
	if err != nil || u.Host == "" {
		return "", errors.New("invalid rendezvous URL")
	}
	scheme, ok := pairSchemes[u.Scheme]
	if !ok {
		return "", errors.New("unsupported rendezvous URL scheme")
	}
	return scheme + "://" + u.Host, nil
}
