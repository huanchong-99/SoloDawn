package home

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type seamTestReceipts map[[2]string]store.Outcome

func (r seamTestReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	if held, ok := r[[2]string{signer, nonce}]; ok {
		return held, nil
	}
	return store.Outcome{}, sql.ErrNoRows
}

func (r seamTestReceipts) PutReceipt(signer, nonce string, _ int64, held store.Outcome) error {
	r[[2]string{signer, nonce}] = held
	return nil
}

func TestApplyJoiningChecksAndPopulatesCommand(t *testing.T) {
	const clock int64 = 1763600000000
	now := time.UnixMilli(clock)
	id := func(seed byte) string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 16)) }
	key := func(seed byte) identity.Key {
		return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize)))
	}
	browser, guest, nobody := key(1), key(4), key(5)
	groupID, ownerID, aiID := id(9), id(10), id(11)
	logOf := func(t *testing.T, first identity.Key) *identity.Log {
		t.Helper()
		log := &identity.Log{User: first.ID()}
		env := first.Sign(fmt.Sprintf(`{"t":"device","user":%q,"ver":1,"op":"add","dev":%q,"kind":"node","name":"device"}`, first.ID(), first.ID()))
		if _, err := log.Apply(env); err != nil {
			t.Fatal(err)
		}
		return log
	}
	empty := func(t *testing.T) *State {
		s := NewState(groupID, browser.ID(), browser.ID())
		s.Logs[browser.ID()], s.Logs[guest.ID()] = logOf(t, browser), logOf(t, guest)
		return s
	}
	person := func(id string, user identity.Key, role string) proto.HumanMemberRow {
		return proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, User: user.ID()}, Role: role}
	}
	group := func(t *testing.T) *State {
		s := empty(t)
		s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: groupID}}
		s.People[ownerID] = person(ownerID, browser, "owner")
		return s
	}
	signed := func(by identity.Key, typ string, n byte, more string) proto.Envelope {
		return by.Sign(fmt.Sprintf(`{"t":%q,"n":%q,"ts":%d,"g":%q%s}`, typ, id(n), clock, groupID, more))
	}

	var got Command
	table := Table{
		"join": func(_ *State, c Command) (Result, *Refusal) {
			got = c
			return Result{InviteUsed: c.Who.Human == ""}, nil
		},
		"note": func(*State, Command) (Result, *Refusal) {
			return Result{}, nil
		},
	}

	stranger := nobody
	joinLog := logOf(t, stranger)
	join := &Joining{Group: groupID, By: ownerID, Open: true, Log: joinLog}

	// Bad signature and wrong group fail before receipts on a joining connection.
	bad := signed(stranger, "join", 1, `,"name":"Quinn"`)
	bad.Payload += " "
	held := seamTestReceipts{}
	res, err := table.applyJoining(group(t), cardTestDisk(nil), held, bad, join, now)
	runSame(t, err, nil)
	runSame(t, res.Answer, store.Outcome{Code: "bad_sig"})
	runSame(t, len(held), 0)

	wrongGroup := stranger.Sign(`{"t":"join","n":"` + id(2) + `","ts":1763600000000,"g":"` + id(8) + `","name":"Quinn"}`)
	res, err = table.applyJoining(group(t), cardTestDisk(nil), held, wrongGroup, join, now)
	runSame(t, err, nil)
	runSame(t, res.Answer, store.Outcome{Code: "bad_request"})
	runSame(t, len(held), 0)

	// Non-join command, mismatched invitation group (even on an empty group), missing group, and as are refused.
	for name, tc := range map[string]struct {
		state *State
		join  *Joining
		env   proto.Envelope
		code  string
	}{
		"non-join command":       {group(t), join, signed(stranger, "note", 3, `,"text":"x"`), "forbidden"},
		"invitation other group": {empty(t), &Joining{Group: id(8), Open: true, Log: joinLog}, signed(stranger, "join", 4, `,"name":"Q"`), "forbidden"},
		"group does not exist":   {empty(t), join, signed(stranger, "join", 5, `,"name":"Q"`), "not_found"},
		"as is not a string":     {group(t), join, signed(stranger, "join", 6, `,"name":"Q","as":5`), "bad_request"},
		"as names an AI":         {group(t), join, signed(stranger, "join", 7, `,"name":"Q","as":"`+aiID+`"`), "forbidden"},
	} {
		t.Run(name, func(t *testing.T) {
			r, applyErr := table.applyJoining(tc.state, cardTestDisk(nil), seamTestReceipts{}, tc.env, tc.join, now)
			runSame(t, applyErr, nil)
			runSame(t, r.Answer, store.Outcome{Code: tc.code})
		})
	}

	// Without a valid log entry for the signer, Who carries only Device.
	res, err = table.applyJoining(group(t), cardTestDisk(nil), seamTestReceipts{}, signed(stranger, "join", 8, `,"name":"Q"`), &Joining{Group: groupID, Open: true}, now)
	runSame(t, err, nil)
	runSame(t, res.Answer, store.Outcome{OK: true})
	runSame(t, got.Who, Who{Device: stranger.ID()})

	res, err = table.applyJoining(group(t), cardTestDisk(nil), seamTestReceipts{}, signed(stranger, "join", 9, `,"name":"Q"`), &Joining{Group: groupID, Open: true, Log: logOf(t, guest)}, now)
	runSame(t, err, nil)
	runSame(t, got.Who, Who{Device: stranger.ID()})

	// With a valid presented log, Who carries User, and Human when already a member (restore).
	res, err = table.applyJoining(group(t), cardTestDisk(nil), seamTestReceipts{}, signed(stranger, "join", 10, `,"name":"Q"`), join, now)
	runSame(t, err, nil)
	runSame(t, res.InviteUsed, true)
	runSame(t, got.Joining, join)
	runSame(t, got.Who, Who{Device: stranger.ID(), User: stranger.ID()})

	restored := group(t)
	restored.People[id(15)] = person(id(15), stranger, "member")
	res, err = table.applyJoining(restored, cardTestDisk(nil), seamTestReceipts{}, signed(stranger, "join", 11, `,"name":"Q"`), join, now)
	runSame(t, err, nil)
	runSame(t, res.InviteUsed, false)
	runSame(t, got.Who, Who{Device: stranger.ID(), User: stranger.ID(), Human: id(15)})
}
