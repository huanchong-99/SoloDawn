package node

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type invitation struct {
	group, by, key string
}

func (p *Pairer) Invite(group, by, signer, nonce string) (home.InviteLink, error) {
	urls := p.cfg.Rendezvous()
	webURL := ""
	if p.cfg.WebURL != nil {
		webURL = p.cfg.WebURL()
	}
	base, err := pairWebBase(webURL, urls)
	if err != nil {
		return home.InviteLink{}, err
	}
	secret := identity.NewSecret()
	a := &pairAttempt{
		role: "a", wantRole: "b", secret: secret, label: p.cfg.Name(),
		invitation: invitation{group, by, inviteKey(signer, nonce)},
		joining:    map[Ctl]bool{},
		link:       PairLink{Link: base + "/#join=" + secret.String() + "&g=" + group + "&home=" + p.cfg.Key.ID()},
	}
	if err := p.openBrowser(context.Background(), a, urls); err != nil {
		return home.InviteLink{}, err
	}
	return home.InviteLink(a.link), nil
}

func inviteKey(signer, nonce string) string { return "invite\x00" + signer + "\x00" + nonce }

func (p *Pairer) ReplayInvite(signer, nonce string) (home.InviteLink, bool) {
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	defer p.mu.Unlock()
	a := p.links[inviteKey(signer, nonce)]
	if !pairLive(a, now) {
		return home.InviteLink{}, false
	}
	return home.InviteLink(a.link), true
}

type joinConn struct {
	group    string
	mu       sync.Mutex
	admitted bool
	log      *identity.Log
}

func (j *joinConn) pending() bool {
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return !j.admitted
}

func (j *joinConn) stage(dl proto.Devlog) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.log == nil {
		j.log = &identity.Log{User: dl.User}
	}
	for _, env := range dl.Entries {
		if _, err := j.log.Apply(env); err != nil {
			j.log = nil
			return
		}
	}
}

func (j *joinConn) presented() *identity.Log {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.log == nil {
		return nil
	}
	log, _ := identity.Replay(j.log.User, j.log.Entries)
	return log
}

// joinVia is what a joining connection came by, an invitation or a code: the
// group, where the connection is kept until it is admitted or ends, to be
// closed with what it came by (under Pairer.mu), and what the home is told
// about it when a command arrives.
type joinVia struct {
	group    string
	conns    *map[Ctl]bool
	snapshot func(c *sesConn, log *identity.Log) *home.Joining
}

func (p *Pairer) serveJoining(ctx context.Context, via joinVia, device string, raw Ctl) {
	c := &sesConn{device: device, raw: raw, joining: &joinConn{group: via.group}}
	p.cfg.Sessions.serve(ctx, c, func() *home.Joining {
		return via.snapshot(c, p.cfg.Logs.dvlWith(c.joining.presented()))
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(*via.conns, raw)
}

func (j *joinConn) accepts(payload string) bool {
	var cmd proto.Join
	return proto.Decode([]byte(payload), &cmd) == nil && cmd.Type == "join" && string(cmd.Group) == j.group
}

func (p *Pairer) inviteVia(a *pairAttempt) joinVia {
	return joinVia{a.invitation.group, &a.joining, func(c *sesConn, log *identity.Log) *home.Joining {
		return &home.Joining{
			Group: a.invitation.group, By: a.invitation.by, Open: p.attemptOpen(a), Log: log,
			Admit: func(used bool) { p.admit(a, c, log, used) },
		}
	}}
}

func (p *Pairer) attemptOpen(a *pairAttempt) bool {
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	defer p.mu.Unlock()
	return pairLive(a, now)
}

func (p *Pairer) admit(a *pairAttempt, c *sesConn, log *identity.Log, used bool) {
	p.admitted(c, log, func() {
		delete(a.joining, c.raw)
		if used {
			a.used, a.secret = true, ""
			delete(p.links, a.invitation.key)
		}
	})
}

// admitted makes c the connection of a member's device, once the home has
// committed its join: the person's log is kept, and forget, under p.mu, takes
// c from the connections that are closed with what it came by. The device is
// no peer of a code any more, whichever way it joined.
func (p *Pairer) admitted(c *sesConn, log *identity.Log, forget func()) {
	if err := p.cfg.Logs.dvlKeep(log, c); err != nil {
		p.cfg.Sessions.cfg.Report(err)
	}
	if rec := p.forgetAdmitted(c.device, forget); rec != nil {
		p.dropPeer(c.device, rec)
	}
	c.home.SetJoining(nil)
	c.joining.mu.Lock()
	defer c.joining.mu.Unlock()
	c.joining.admitted = true
}

func (p *Pairer) forgetAdmitted(device string, forget func()) *codePeer {
	p.mu.Lock()
	defer p.mu.Unlock()
	forget()
	return p.codePeers[device]
}

// A code is an open invitation code of a group homed here, from the moment
// the node reads it in the setting invites until it finds it there no longer
// (planning/contracts/step-2/people.md 2.6). Unlike a pairAttempt it has no
// outcome and any number of peers, and it dials nothing: its room is hosted
// on the node's own signalling paths. Nothing of it is written to a log.
type code struct {
	group, room string
	secret      identity.Secret
	card        json.RawMessage // this node's pair message, the data of the hosted room
	ended       chan struct{}   // closed when the code ends
}

// codePeer is a device whose pair message under a code was accepted: a
// joining peer for 10 minutes from that message, or until the code ends or
// its person is a member. A later message of the device is a new codePeer,
// which has the same set of connections when it comes straight after one
// under the same code.
type codePeer struct {
	code    *code
	route   peer.Route
	joining *map[Ctl]bool // its joining connections that are not admitted, under Pairer.mu
}

// CodeChanged is LoopConfig.CodeChanged of the home, and the node calls it
// when it starts: it reads the open codes, ends each one in memory that is
// open no longer, and hosts the room of each new one on the node's paths. A
// call when nothing changed does nothing. codeMu is held from the read to the
// last frame, so the frames about a room go out in the order of its changes.
func (p *Pairer) CodeChanged(string) {
	p.codeMu.Lock()
	defer p.codeMu.Unlock()
	now := p.cfg.Now().UnixMilli()
	var open map[string]store.Invite
	err := p.cfg.Store.Write(func(tx *store.Tx) (err error) {
		open, err = tx.Invites(now)
		return err
	})
	if err != nil {
		p.cfg.Sessions.cfg.Report(err)
		return
	}
	began, ended := p.takeCodes(open, p.cfg.Name())
	paths := p.sharedPaths()
	for _, c := range ended {
		close(c.ended)
		tellRoom(paths, proto.RoomID{Type: "leave", Room: c.room})
	}
	for _, c := range began {
		go p.watchCode(c, open[c.group].Expires)
		tellRoom(paths, c.hosting())
	}
}

// takeCodes makes the codes in memory those that are open, and returns the
// ones that began and the ones that ended.
func (p *Pairer) takeCodes(open map[string]store.Invite, label string) (began, ended []*code) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for group, c := range p.codes {
		if string(c.secret) != open[group].Secret {
			delete(p.codes, group)
			ended = append(ended, c)
		}
	}
	for group, entry := range open {
		if p.codes[group] == nil {
			p.codes[group] = p.newCode(group, label, entry)
			began = append(began, p.codes[group])
		}
	}
	return began, ended
}

// newCode builds a code with its card, whose exp is when the code ends
// (planning/contracts/envelope.md section 6).
func (p *Pairer) newCode(group, label string, entry store.Invite) *code {
	secret := identity.Secret(entry.Secret)
	self := proto.Pair{Type: "pair", Key: p.cfg.Key.ID(), Kind: "node", Name: label, Role: "a", Expires: entry.Expires}
	self.MAC = secret.MAC(self)
	return &code{group: group, room: secret.Room(), secret: secret, card: dvlMustEncode(self), ended: make(chan struct{})}
}

func (c *code) hosting() proto.RoomData {
	return proto.RoomData{RoomID: proto.RoomID{Type: "join", Room: c.room}, Data: c.card}
}

// tellRoom sends a frame about a code's room, the hosting join or leave, on
// each path whose server has hosted rooms. An older rendezvous is sent
// neither (planning/contracts/rendezvous.md 2.1).
func tellRoom(paths []*peer.Path, frame any) {
	for _, path := range paths {
		if path.Hosts() {
			path.Write(context.Background(), frame)
		}
	}
}

// HostCodes hosts the room of every open code on a path that has connected.
// A full that answers one, because another device hosts the room or this
// connection hosts 16, ends nothing: the code is hosted on the other paths,
// and here when this path connects again.
func (p *Pairer) HostCodes(path *peer.Path) {
	p.codeMu.Lock()
	defer p.codeMu.Unlock()
	for _, c := range p.openCodes() {
		tellRoom([]*peer.Path{path}, c.hosting())
	}
}

func (p *Pairer) openCodes() []*code {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Collect(maps.Values(p.codes))
}

// watchCode ends a code when the home's clock has reached expires: the entry
// then counts as none, which CodeChanged finds. A timer does not run on the
// clock that expires is read against, so one that ran out early is set again
// for the time that is left, a second at least.
func (p *Pairer) watchCode(c *code, expires int64) {
	for {
		left := time.Duration(expires-p.cfg.Now().UnixMilli()) * time.Millisecond
		select {
		case <-c.ended:
			return
		case <-p.cfg.After(max(left, time.Second)):
			p.CodeChanged("")
		}
	}
}

// onCodeMsg takes a frame for the room of no attempt. A pair message under
// the open code of that room makes its device a joining peer from now, as
// often as one comes (planning/contracts/step-2/people.md section 3, point 3).
func (p *Pairer) onCodeMsg(path *peer.Path, room string, data []byte) {
	c := p.codeOfRoom(room)
	m, ok := roomPair(data)
	if c == nil || !ok || !c.secret.Verify(m, "b") {
		return
	}
	rec := p.keepCodePeer(c, m.Key, path.Route())
	go p.watchPeer(m.Key, rec)
}

func (p *Pairer) keepCodePeer(c *code, device string, route peer.Route) *codePeer {
	rec := &codePeer{code: c, route: route, joining: &map[Ctl]bool{}}
	p.mu.Lock()
	defer p.mu.Unlock()
	if old := p.codePeers[device]; old != nil && old.code == c {
		rec.joining = old.joining
	}
	p.codePeers[device] = rec
	return rec
}

func (p *Pairer) codeOfRoom(room string) *code {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.codes {
		if c.room == room {
			return c
		}
	}
	return nil
}

// watchPeer ends a peer's time on a code 10 minutes after its pair message,
// or when the code ends.
func (p *Pairer) watchPeer(device string, rec *codePeer) {
	select {
	case <-rec.code.ended:
	case <-p.cfg.After(pairWindow):
	}
	p.dropPeer(device, rec)
}

// dropPeer forgets a peer of a code and closes its joining connections that
// were not admitted. After a later pair message of the device rec is not its
// peer any more: when the device's peer now has the set of connections of
// rec, nothing is done; otherwise the set was left with rec, which closes it.
func (p *Pairer) dropPeer(device string, rec *codePeer) {
	for _, ctl := range p.takeDropped(device, rec) {
		ctl.Close()
	}
}

func (p *Pairer) takeDropped(device string, rec *codePeer) []Ctl {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.codePeers[device]
	if cur == rec {
		delete(p.codePeers, device)
	}
	if cur == rec || cur == nil || cur.joining != rec.joining {
		return slices.Collect(maps.Keys(*rec.joining))
	}
	return nil
}

func (p *Pairer) codeVia(rec *codePeer) joinVia {
	return joinVia{rec.code.group, rec.joining, func(c *sesConn, log *identity.Log) *home.Joining {
		return &home.Joining{Group: rec.code.group, Code: rec.code.secret, Log: log, Admit: func(bool) {
			p.admitted(c, log, func() { delete(*rec.joining, c.raw) })
		}}
	}}
}

func (n *nodeRuntime) remembersHome(device string) bool {
	var found bool
	err := n.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM setting, json_each(setting.value) AS entry
			WHERE setting.key = 'homes' AND json_extract(entry.value, '$.home') = ?)`, device).Scan(&found)
	})
	return err == nil && found
}
