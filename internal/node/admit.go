package node

import (
	"context"
	"sync"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
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

func (p *Pairer) serveJoining(ctx context.Context, a *pairAttempt, device string, raw Ctl) {
	c := &sesConn{device: device, raw: raw, joining: &joinConn{group: a.invitation.group}}
	p.cfg.Sessions.serve(ctx, c, func() *home.Joining { return p.joinSnapshot(a, c) })
	p.mu.Lock()
	delete(a.joining, raw)
	p.mu.Unlock()
}

func (j *joinConn) accepts(payload string) bool {
	var cmd proto.Join
	return proto.Decode([]byte(payload), &cmd) == nil && cmd.Type == "join" && string(cmd.Group) == j.group
}

func (p *Pairer) joinSnapshot(a *pairAttempt, c *sesConn) *home.Joining {
	now := p.cfg.Now().UnixMilli()
	p.mu.Lock()
	open := pairLive(a, now)
	p.mu.Unlock()
	log := p.cfg.Logs.dvlWith(c.joining.presented())
	return &home.Joining{
		Group: a.invitation.group, By: a.invitation.by, Open: open, Log: log,
		Admit: func(used bool) { p.admit(a, c, log, used) },
	}
}

func (p *Pairer) admit(a *pairAttempt, c *sesConn, log *identity.Log, used bool) {
	if err := p.cfg.Logs.dvlKeep(log, c); err != nil {
		p.cfg.Sessions.cfg.Report(err)
	}
	p.mu.Lock()
	delete(a.joining, c.raw)
	if used {
		a.used, a.secret = true, ""
		delete(p.links, a.invitation.key)
	}
	p.mu.Unlock()
	c.home.SetJoining(nil)
	c.joining.mu.Lock()
	c.joining.admitted = true
	c.joining.mu.Unlock()
}

func (n *nodeRuntime) remembersHome(device string) bool {
	var found bool
	err := n.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM setting, json_each(setting.value) AS entry
			WHERE setting.key = 'homes' AND json_extract(entry.value, '$.home') = ?)`, device).Scan(&found)
	})
	return err == nil && found
}
