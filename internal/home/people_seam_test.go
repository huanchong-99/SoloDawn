package home_test

import (
	"errors"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type seamInviter struct {
	open   bool
	err    error
	link   home.InviteLink
	calls  int
	onCall func() error
}

func (f *seamInviter) Invite(group, by, signer, nonce string) (home.InviteLink, error) {
	f.calls++
	if f.onCall != nil {
		if err := f.onCall(); err != nil {
			return home.InviteLink{}, err
		}
	}
	if f.err != nil {
		return home.InviteLink{}, f.err
	}
	f.open = true
	f.link = home.InviteLink{Link: "https://app.example/#join=SECRET&g=" + group + "&by=" + by + "&n=" + nonce + "&k=" + signer, Expires: clock + 600_000}
	return f.link, nil
}

func (f *seamInviter) ReplayInvite(_, _ string) (home.InviteLink, bool) {
	if f.onCall != nil {
		_ = f.onCall()
	}
	if !f.open {
		return home.InviteLink{}, false
	}
	return f.link, true
}

type seamClient chan any

func (c seamClient) Send(frame any) error  { c <- frame; return nil }
func (c seamClient) Done() <-chan struct{} { return nil }

func TestApplyJoiningChecksAndPopulatesCommand(t *testing.T) {
	var got home.Command
	table := home.Table{
		"join": func(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
			got = c
			return home.Result{InviteUsed: c.Who.Human == ""}, nil
		},
		"note": func(*home.State, home.Command) (home.Result, *home.Refusal) {
			return home.Result{}, nil
		},
	}

	stranger := nobody
	joinLog := logOf(t, stranger)
	join := &home.Joining{Group: groupID, By: ownerID, Open: true, Log: joinLog}

	// Bad signature and wrong group fail before receipts on a joining connection.
	bad := signed(stranger, "join", 1, `,"name":"Quinn"`)
	bad.Payload += " "
	held := receipts{}
	res, err := table.ApplyJoining(group(t), disk(nil), held, bad, join, now)
	equal(t, err, nil)
	equal(t, res.Answer, store.Outcome{Code: "bad_sig"})
	equal(t, len(held), 0)

	wrongGroup := stranger.Sign(`{"t":"join","n":"` + id(2) + `","ts":1763600000000,"g":"` + id(8) + `","name":"Quinn"}`)
	res, err = table.ApplyJoining(group(t), disk(nil), held, wrongGroup, join, now)
	equal(t, err, nil)
	equal(t, res.Answer, store.Outcome{Code: "bad_request"})
	equal(t, len(held), 0)

	// Non-join command, mismatched invitation group (even on an empty group), missing group, and as are refused.
	for name, tc := range map[string]struct {
		state *home.State
		join  *home.Joining
		env   proto.Envelope
		code  string
	}{
		"non-join command":       {group(t), join, signed(stranger, "note", 3, `,"text":"x"`), "forbidden"},
		"invitation other group": {empty(t), &home.Joining{Group: id(8), Open: true, Log: joinLog}, signed(stranger, "join", 4, `,"name":"Q"`), "forbidden"},
		"group does not exist":   {empty(t), join, signed(stranger, "join", 5, `,"name":"Q"`), "not_found"},
		"as is not a string":     {group(t), join, signed(stranger, "join", 6, `,"name":"Q","as":5`), "bad_request"},
		"as names an AI":         {group(t), join, signed(stranger, "join", 7, `,"name":"Q","as":"`+aiID+`"`), "forbidden"},
	} {
		t.Run(name, func(t *testing.T) {
			r, applyErr := table.ApplyJoining(tc.state, disk(nil), receipts{}, tc.env, tc.join, now)
			equal(t, applyErr, nil)
			equal(t, r.Answer, store.Outcome{Code: tc.code})
		})
	}

	// Without a valid log entry for the signer, Who carries only Device.
	res, err = table.ApplyJoining(group(t), disk(nil), receipts{}, signed(stranger, "join", 8, `,"name":"Q"`), &home.Joining{Group: groupID, Open: true}, now)
	equal(t, err, nil)
	equal(t, res.Answer, store.Outcome{OK: true})
	equal(t, got.Who, home.Who{Device: stranger.ID()})

	res, err = table.ApplyJoining(group(t), disk(nil), receipts{}, signed(stranger, "join", 9, `,"name":"Q"`), &home.Joining{Group: groupID, Open: true, Log: logOf(t, guest)}, now)
	equal(t, err, nil)
	equal(t, got.Who, home.Who{Device: stranger.ID()})

	// With a valid presented log, Who carries User, and Human when already a member (restore).
	res, err = table.ApplyJoining(group(t), disk(nil), receipts{}, signed(stranger, "join", 10, `,"name":"Q"`), join, now)
	equal(t, err, nil)
	equal(t, res.InviteUsed, true)
	equal(t, got.Joining, join)
	equal(t, got.Who, home.Who{Device: stranger.ID(), User: stranger.ID()})

	restored := group(t)
	restored.People[id(15)] = person(id(15), stranger, "member")
	res, err = table.ApplyJoining(restored, disk(nil), receipts{}, signed(stranger, "join", 11, `,"name":"Q"`), join, now)
	equal(t, err, nil)
	equal(t, res.InviteUsed, false)
	equal(t, got.Who, home.Who{Device: stranger.ID(), User: stranger.ID(), Human: id(15)})
}

func TestApplyInviterHookAndReplay(t *testing.T) {
	table := home.Table{
		"invite": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
			if !s.Admin(c.Who) {
				return home.Result{}, &home.Refusal{Code: "forbidden", Msg: "admin only"}
			}
			return home.Result{InviteBy: c.Who.Human}, nil
		},
	}

	// Without an Inviter on State, an accepted invite still succeeds; replay after restart answers conflict.
	sNoHook, heldNoHook := group(t), receipts{}
	env := signed(browser, "invite", 1, ``)
	first := apply(t, table, sNoHook, heldNoHook, env)
	equal(t, first.Answer, store.Outcome{OK: true})
	equal(t, first.InviteBy, ownerID)
	replayNoHook := apply(t, table, sNoHook, heldNoHook, env)
	equal(t, replayNoHook.Answer, store.Outcome{Code: "conflict"})

	// An Inviter error leaves no receipt so a retry runs again.
	inv := &seamInviter{err: errors.New("no rendezvous")}
	s, held := group(t), receipts{}
	s.Invite = inv
	_, err := table.Apply(s, disk(nil), held, env, now)
	equal(t, err, inv.err)
	equal(t, len(held), 0)

	// Succeeded invite keeps only {OK: true} in receipts, frames link and expires, and replays while open.
	inv.err = nil
	res := apply(t, table, s, held, env)
	equal(t, held[[2]string{browser.ID(), id(1)}], receipt{clock, store.Outcome{OK: true}})
	equal(t, res.Frame(7), any(proto.CmdResult{
		Success: proto.Success{Re: 7, OK: true}, Link: inv.link.Link, Expires: inv.link.Expires,
	}))

	again := apply(t, table, s, held, env)
	equal(t, again.Frame(8), any(proto.CmdResult{
		Success: proto.Success{Re: 8, OK: true}, Link: inv.link.Link, Expires: inv.link.Expires,
	}))
	equal(t, inv.calls, 2)

	inv.open = false
	closed := apply(t, table, s, held, env)
	equal(t, closed.Answer, store.Outcome{Code: "conflict"})

	// A refused invite replays its original refusal rather than conflict.
	s.People[id(12)] = person(id(12), guest, "member")
	deniedEnv := signed(guest, "invite", 2, ``)
	denied := apply(t, table, s, held, deniedEnv)
	equal(t, denied.Answer, store.Outcome{Code: "forbidden"})
	equal(t, apply(t, table, s, held, deniedEnv).Answer, store.Outcome{Code: "forbidden"})
}

func TestLoopRunsInviterOutsideStoreWriteAndAdmitsJoiningConn(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var reported []error
	inv := &seamInviter{
		// Calling db.Write inside Invite/ReplayInvite proves Store.Write is not held.
		onCall: func() error {
			return db.Write(func(tx *store.Tx) error {
				_, qErr := tx.Exec("SELECT 1")
				return qErr
			})
		},
	}
	tables := home.Tables{
		Commands: home.Table{
			"group/create": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
				return home.Result{
					Rows: []home.Row{
						home.RowOf(proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "G", Home: s.Home}),
						home.RowOf(person(ownerID, browser, "owner")),
					},
					GroupsChanged: true,
				}, nil
			},
			"invite": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
				if !s.Admin(c.Who) {
					return home.Result{}, &home.Refusal{Code: "forbidden", Msg: "admin only"}
				}
				return home.Result{InviteBy: c.Who.Human}, nil
			},
			"join": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
				if c.Joining == nil || !c.Joining.Open || c.Who.User == "" {
					return home.Result{}, &home.Refusal{Code: "forbidden", Msg: "cannot join"}
				}
				if c.Who.Human != "" {
					return home.Result{GroupsChanged: true}, nil
				}
				return home.Result{
					Rows: []home.Row{
						home.RowOf(person(string(c.Nonce), guest, "member")),
						home.RowOf(deviceRow(guest, guest, 1, "add", guest)),
					},
					GroupsChanged: true,
					InviteUsed:    true,
				}, nil
			},
			"note": func(*home.State, home.Command) (home.Result, *home.Refusal) {
				return home.Result{}, nil
			},
		},
	}
	ownerLog := logOf(t, browser, node)
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db,
		Home:  node.ID(),
		Owner: browser.ID(),
		Logs: func(u string) *identity.Log {
			if u == browser.ID() {
				return ownerLog
			}
			return nil
		},
		Tables: tables,
		Invite: inv,
		Now:    func() time.Time { return now },
		Report: func(e error) { reported = append(reported, e) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	adminOut := make(seamClient, 16)
	adminConn := loop.Attach(browser.ID(), adminOut)
	loop.Cmd(adminConn, 1, signed(browser, "group", 9, `,"op":"create"`))
	equal(t, (<-adminOut).(proto.CmdResult).OK, proto.True(true))

	// Failed Invite reports internal and writes no receipt; retry succeeds.
	inv.err = errors.New("rv down")
	invEnv := signed(browser, "invite", 2, ``)
	loop.Cmd(adminConn, 2, invEnv)
	equal(t, (<-adminOut).(proto.ErrorResponse).Err.Code, "internal")
	equal(t, len(reported), 1)

	// Receipt write error after Invite is reported as internal.
	inv.err = nil
	dupEnv := signed(browser, "invite", 3, ``)
	inv.onCall = func() error {
		return db.Write(func(tx *store.Tx) error {
			return tx.PutReceipt(browser.ID(), id(3), clock, store.Outcome{OK: true})
		})
	}
	loop.Cmd(adminConn, 3, dupEnv)
	equal(t, (<-adminOut).(proto.ErrorResponse).Err.Code, "internal")
	equal(t, len(reported), 2)

	// Clean Invite succeeds, replays while open, and answers conflict once closed.
	inv.onCall = func() error {
		return db.Write(func(*store.Tx) error { return nil })
	}
	loop.Cmd(adminConn, 4, invEnv)
	gotInvite := (<-adminOut).(proto.CmdResult)
	equal(t, gotInvite.Link, inv.link.Link)
	equal(t, gotInvite.Expires, inv.link.Expires)

	loop.Cmd(adminConn, 5, invEnv)
	equal(t, (<-adminOut).(proto.CmdResult).Link, inv.link.Link)

	inv.open = false
	loop.Cmd(adminConn, 6, invEnv)
	equal(t, (<-adminOut).(proto.ErrorResponse).Err.Code, "conflict")

	// Normal command replay on a loop with Inviter still returns its receipt.
	noteEnv := signed(browser, "note", 4, ``)
	loop.Cmd(adminConn, 7, noteEnv)
	equal(t, (<-adminOut).(proto.CmdResult).OK, proto.True(true))
	loop.Cmd(adminConn, 8, noteEnv)
	equal(t, (<-adminOut).(proto.CmdResult).OK, proto.True(true))

	// Joining connection: stray command without g gets bad_request, refused join leaves Admit uncalled,
	// accepted join calls Admit(true) before ok, and restore join calls Admit(false).
	joinOut := make(seamClient, 16)
	joinConn := loop.Attach(guest.ID(), joinOut)
	var admitted []bool
	openFlag := false
	guestLog := logOf(t, guest)
	var joinHook func() *home.Joining
	joinHook = func() *home.Joining {
		return &home.Joining{
			Group: groupID,
			By:    ownerID,
			Open:  openFlag,
			Log:   guestLog,
			Admit: func(used bool) { admitted = append(admitted, used) },
		}
	}
	joinConn.SetJoining(joinHook)

	loop.Cmd(joinConn, 10, guest.Sign(`{"t":"join","n":"`+id(5)+`","ts":1763600000000}`))
	equal(t, (<-joinOut).(proto.ErrorResponse).Err.Code, "bad_request")

	loop.Cmd(joinConn, 11, signed(guest, "join", 6, ``))
	equal(t, (<-joinOut).(proto.ErrorResponse).Err.Code, "forbidden")
	equal(t, len(admitted), 0)

	openFlag = true
	loop.Cmd(joinConn, 12, signed(guest, "join", 7, ``))
	equal(t, (<-joinOut).(proto.CmdResult).OK, proto.True(true))
	equal(t, admitted, []bool{true})

	// Existing member rejoining after restore gets Admit(false); SetJoining(nil) clears joining mode.
	loop.Cmd(joinConn, 13, signed(guest, "join", 8, ``))
	equal(t, (<-joinOut).(proto.CmdResult).OK, proto.True(true))
	equal(t, admitted, []bool{true, false})

	joinConn.SetJoining(nil)
	loop.Cmd(joinConn, 14, signed(guest, "join", 12, ``))
	equal(t, (<-joinOut).(proto.ErrorResponse).Err.Code, "forbidden")
}
