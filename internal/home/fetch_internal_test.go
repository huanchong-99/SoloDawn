package home

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestFetchReaderChecksSeatGroupAndAccess(t *testing.T) {
	ownerKey := identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize)))
	log := &identity.Log{User: ownerKey.ID()}
	entry := fmt.Sprintf(`{"t":"device","user":%q,"ver":1,"op":"add","dev":%q,"kind":"node","name":"owner"}`, ownerKey.ID(), ownerKey.ID())
	if _, err := log.Apply(ownerKey.Sign(entry)); err != nil {
		t.Fatal(err)
	}
	newConn := func(device string) *Conn {
		return &Conn{device: device, gone: make(chan struct{}), frames: runNewQueue[any](4)}
	}

	s := NewState("g1", "home", ownerKey.ID())
	s.Logs[ownerKey.ID()] = log
	g := &runGroup{state: s, seats: map[*Conn]*runSeat{}}

	// Unseated connection returns false and queues nothing.
	unseated := newConn(ownerKey.ID())
	runSame(t, []bool{g.fetReader(unseated, 10), unseated.frames.idle()}, []bool{false, true})

	// Seated connection on a group whose group row does not exist yet gets not_found.
	ownerConn := newConn(ownerKey.ID())
	g.seat(ownerConn)
	runSame(t, g.fetReader(ownerConn, 11), false)
	frame, ok := ownerConn.frames.pop()
	runSame(t, []any{ok, frame}, []any{true, fetErr(11, "not_found", "this node is not home to the group")})

	// Existing group refuses an unserved device with forbidden and admits a served one.
	s.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: "g1"}}
	s.People["m1"] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "m1"}, User: ownerKey.ID()}}
	strangerConn := newConn("stranger")
	g.seat(strangerConn)
	runSame(t, g.fetReader(strangerConn, 12), false)
	frame, ok = strangerConn.frames.pop()
	runSame(t, []any{ok, frame}, []any{true, fetErr(12, "forbidden", "the device may not read this group")})

	runSame(t, []bool{g.fetReader(ownerConn, 13), ownerConn.frames.idle()}, []bool{true, true})
}
