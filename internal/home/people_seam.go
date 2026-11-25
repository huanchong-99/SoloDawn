package home

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Seam for step 2 people commands (planning/contracts/step-2/people.md;
// planning/steps/parallel-plan-step-2-3.md row X2-1): the joining context of a
// connection on Command, and the Inviter hook the home calls for invite.

const (
	keyInvite = "invite"
	keyJoin   = "join"
)

// Joining is what Command carries on a joining connection
// (planning/contracts/step-2/people.md sections 2.2, 4 and 5): a non-nil
// Joining says the connection is joining.
type Joining struct {
	Group string        // group id the invitation is for
	By    string        // member id of the admin who created the invitation
	Open  bool          // whether the invitation is open when the command runs
	Log   *identity.Log // verified device log presented on this connection, or nil
	// Admit, when non-nil, is called by the home after an accepted join
	// commits; used is Result.InviteUsed (true when a new member joined,
	// false when an existing member rejoined after a restore).
	Admit func(used bool)
}

// InviteLink is the link and expiry returned by an invite command
// (planning/contracts/step-2/people.md section 2.1).
type InviteLink struct {
	Link    string
	Expires int64
}

// Inviter is the hook the home calls for invite and its replays
// (planning/contracts/step-2/people.md section 2.1). On a Loop its methods run
// on the group's goroutine outside Store.Write, so they may read node settings
// and join rendezvous rooms.
type Inviter interface {
	Invite(group, by, signer, nonce string) (InviteLink, error)
	ReplayInvite(signer, nonce string) (InviteLink, bool)
}

type seamDeferred struct {
	replay, create bool
	hdr            proto.PayloadHeader
	held           store.Outcome
}

// SetJoining sets or clears (with nil) the snapshot function that marks c as a
// joining connection when commands arrive on it.
func (c *Conn) SetJoining(fn func() *Joining) {
	if fn == nil {
		c.joining.Store(nil)
		return
	}
	c.joining.Store(&fn)
}

func (c *Conn) seamJoining() *Joining {
	fn := c.joining.Load()
	if fn == nil {
		return nil
	}
	return (*fn)()
}

// applyJoining is Apply on a connection whose joining state is join (nil on an
// ordinary connection).
func (t Table) applyJoining(s *State, disk Disk, receipts Receipts, env proto.Envelope, join *Joining, now time.Time) (Result, error) {
	res, _, err := t.seamRunTx(s, disk, receipts, env, join, now, false)
	return res, err
}

func (t Table) seamRunTx(s *State, disk Disk, receipts Receipts, env proto.Envelope, join *Joining, now time.Time, deferInvite bool) (Result, seamDeferred, error) {
	hdr, refusal := s.open(env, join, now)
	if refusal != nil {
		return refused(refusal), seamDeferred{}, nil
	}
	return t.seamAfterOpen(s, disk, receipts, hdr, env, join, now, deferInvite)
}

func (t Table) seamAfterOpen(s *State, disk Disk, receipts Receipts, hdr proto.PayloadHeader, env proto.Envelope, join *Joining, now time.Time, deferInvite bool) (Result, seamDeferred, error) {
	held, found, err := seamReceipt(receipts, env.Key, string(hdr.Nonce))
	if err != nil {
		return Result{}, seamDeferred{}, err
	}
	if found {
		res, def := seamHeld(hdr, env.Key, held, deferInvite)
		return res, def, nil
	}
	res := t.run(s, Command{PayloadHeader: hdr, Env: env, Now: now.UnixMilli(), Disk: disk, Joining: join})
	if deferInvite && res.InviteBy != "" {
		return res, seamDeferred{create: true, hdr: hdr}, nil
	}
	return res, seamDeferred{}, receipts.PutReceipt(env.Key, string(hdr.Nonce), hdr.Timestamp, res.Answer)
}

func seamReceipt(receipts Receipts, signer, nonce string) (store.Outcome, bool, error) {
	held, err := receipts.Receipt(signer, nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Outcome{}, false, nil
	}
	return held, err == nil, err
}

func seamHeld(hdr proto.PayloadHeader, signer string, held store.Outcome, deferInvite bool) (Result, seamDeferred) {
	if deferInvite && held.OK && hdr.Type == keyInvite {
		return Result{}, seamDeferred{replay: true, hdr: hdr, held: held}
	}
	return seamReplayWith(nil, hdr.Type, signer, string(hdr.Nonce), held), seamDeferred{}
}

func seamReplayWith(inv Inviter, typ, signer, nonce string, held store.Outcome) Result {
	if !held.OK || typ != keyInvite {
		return repeated(held)
	}
	if inv != nil {
		if link, ok := inv.ReplayInvite(signer, nonce); ok {
			return Result{Answer: held, Link: link.Link, Expires: link.Expires}
		}
	}
	return refused(&Refusal{"conflict", "the invitation is no longer open"})
}

func (s *State) seamKnown(join *Joining) func(string) bool {
	if join == nil {
		return s.known
	}
	return func(string) bool { return true }
}

func (s *State) seamFindWho(key string, c *Command) *Refusal {
	if c.Joining != nil {
		return s.seamJoiningWho(key, c)
	}
	if s.Group.ID == "" && key != keyCreate {
		return &Refusal{"not_found", "this node is not home to the group"}
	}
	as, ok := c.field("as")
	if !ok {
		return &Refusal{"bad_request", "as is not a member id"}
	}
	who, refusal := s.who(c.Env.Key, as)
	c.Who = who
	return refusal
}

func (s *State) seamJoiningWho(key string, c *Command) *Refusal {
	if key != keyJoin || c.Joining.Group != s.ID {
		return &Refusal{"forbidden", "a joining connection may only send join for its group"}
	}
	if s.Group.ID == "" {
		return &Refusal{"not_found", "this node is not home to the group"}
	}
	if refusal := seamNoAs(c); refusal != nil {
		return refusal
	}
	c.Who = s.seamJoiner(c.Env.Key, c.Joining.Log)
	return nil
}

func seamNoAs(c *Command) *Refusal {
	as, ok := c.field("as")
	if !ok {
		return &Refusal{"bad_request", "as is not a member id"}
	}
	if as != "" {
		return &Refusal{"forbidden", "as names no AI that the signer hosts"}
	}
	return nil
}

func (s *State) seamJoiner(device string, log *identity.Log) Who {
	w := Who{Device: device}
	if log != nil && log.Valid(device) {
		w.User = log.User
		w.Human = s.human(log.User)
	}
	return w
}

func seamAdmit(join *Joining, res Result) {
	if join != nil && join.Admit != nil && res.Answer.OK {
		join.Admit(res.InviteUsed)
	}
}

func (g *runGroup) seamCommand(c *Conn, re int64, env proto.Envelope) {
	now, join := g.loop.cfg.Now(), c.seamJoining()
	var def seamDeferred
	res, err := g.transact(now, func(d Disk, tx *store.Tx) (Result, error) {
		var out Result
		var txErr error
		out, def, txErr = g.loop.cfg.Tables.Commands.seamRunTx(g.state, d, tx, env, join, now, g.loop.cfg.Invite != nil)
		out.byPerson = inboxByPerson(g.state, env.Key)
		return out, txErr
	})
	res, err = g.seamResolveInvite(env.Key, res, def, err)
	if err != nil {
		g.loop.cfg.Report(fmt.Errorf("group %s, a command of %s: %w", g.state.ID, c.device, err))
		g.loop.diagReply(c, g.state.ID, re, env, "internal")
		c.send(runErr(re, "internal", "the home could not apply the command; the cause is in its log"))
		return
	}
	g.settle(res, now, func() {
		seamAdmit(join, res)
		g.loop.diagReply(c, g.state.ID, re, env, res.Answer.Code)
		c.send(res.Frame(re))
	})
}

func (g *runGroup) seamResolveInvite(signer string, res Result, def seamDeferred, err error) (Result, error) {
	if err != nil {
		return res, err
	}
	if def.replay {
		return seamReplayWith(g.loop.cfg.Invite, keyInvite, signer, string(def.hdr.Nonce), def.held), nil
	}
	if !def.create {
		return res, nil
	}
	return g.seamCreateInvite(signer, def.hdr, res)
}

func (g *runGroup) seamCreateInvite(signer string, hdr proto.PayloadHeader, res Result) (Result, error) {
	link, err := g.loop.cfg.Invite.Invite(g.state.ID, res.InviteBy, signer, string(hdr.Nonce))
	if err != nil {
		return Result{}, err
	}
	res.Link, res.Expires = link.Link, link.Expires
	err = g.loop.cfg.Store.Write(func(tx *store.Tx) error {
		return tx.PutReceipt(signer, string(hdr.Nonce), hdr.Timestamp, res.Answer)
	})
	return res, err
}
