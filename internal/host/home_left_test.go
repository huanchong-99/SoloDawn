package host

import (
	"encoding/json"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const leftMember = "owner-member"

// newLeftRig makes a host that reads a team group and a project group, with a
// session of its own AI in the latter, at one home.
func newLeftRig(t *testing.T) *deliverRig {
	r := newDeliverRig(t, false)
	home := r.homeKey.ID()
	r.setSetting("owner", r.ownerKey.ID())
	r.h.Attach(home, r.home)
	for _, g := range []proto.ID{testGroup1, testGroup2} {
		joinAssertEqual(t, "remember group", r.h.RecordHome(proto.Home{Group: g, Home: home, Name: "Project"}, nil), nil)
		r.waitGroupLive(string(g))
		r.leftPush(string(g), leftHuman(string(g), r.ownerKey.ID()))
	}
	r.leftPush(testGroup1, proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: testAI1, Seq: 11}, Group: testGroup1, Kind: "ai", User: r.ownerKey.ID(), Name: "C"},
		CLI:       "claude", Host: r.hostKey.ID(), Allow: []string{}, Approvers: []string{},
	})
	key := SessionKey{Group: testGroup1, Member: testAI1}
	r.h.Sessions.Create(Session{SessionKey: key, Kind: GroupSession, Folder: "repo", Agent: "claude", Home: home, Name: "C"})
	r.waitTeams(1)
	return r
}

func leftHuman(group, user string) proto.HumanMemberRow {
	return proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: leftMember, Seq: 10}, Group: group, Kind: "human", User: user, Name: "Owner"}, Role: "member"}
}

func (r *deliverRig) leftPush(group string, rows ...any) {
	raws := []json.RawMessage{json.RawMessage(marshalWire(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group, Seq: 9}, Name: "G", Kind: map[string]string{testGroup1: "project", testGroup2: "team"}[group],
		Base: "main", Forge: "none", Home: r.homeKey.ID(), Checks: []string{},
	}))}
	for _, row := range rows {
		raws = append(raws, json.RawMessage(marshalWire(row)))
	}
	r.h.Push(r.homeKey.ID(), json.RawMessage(marshalWire(proto.Rows{Type: "rows", Group: group, Rows: raws})))
}

// waitTeams waits until the secretary's line would name n team groups.
func (r *deliverRig) waitTeams(n int) {
	r.t.Helper()
	r.waitUntil("team groups", func() bool {
		known, err := bulletinHomes(r.h.Store)
		return err == nil && len(teamGroups(r.h, known)) == n
	})
}

func (r *deliverRig) waitHomes(n int) {
	r.t.Helper()
	r.waitUntil("remembered homes", func() bool {
		homes, err := joinSavedHomes(r.h.Store)
		return err == nil && len(homes) == n
	})
}

func (r *deliverRig) wantKnown(group string, want bool) {
	r.t.Helper()
	_, rows := joinCachedGroup(r.h, group)
	joinAssertEqual(r.t, "rows of "+group, rows, want)
	joinAssertEqual(r.t, "home of "+group, deliv(r.h).groupHome(group) != "", want)
	joinAssertEqual(r.t, "session in "+group, joinGroupHasSession(r.h, group), want && group == testGroup1)
}

var leftGone = proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: leftMember, Seq: 20}, Gone: true}

// A home that no longer lists a group, after rows that show the node's owner
// is no member, ends what the node has of it; the connection may be gone by
// then. The other group stays.
func TestGroupItsOwnerWasRemovedFromIsLeft(t *testing.T) {
	r := newLeftRig(t)
	pre := &preemption{session: SessionKey{Group: testGroup1, Member: testAI1}}
	deliv(r.h).openPreemption(aiGroupKey{group: testGroup1, member: testAI1}, pre)
	w := watchState(r.h)
	w.mu.Lock()
	w.grants[aiSlot{testGroup1, testAI1}] = map[string]string{"watcher": "another-member"}
	w.mu.Unlock()
	r.leftPush(testGroup1, leftGone)
	r.home.closeConn()
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"groups","groups":[{"g":"`+testGroup2+`","name":"G","seq":10}]}`))
	r.waitHomes(1)
	r.h.Settle()
	r.wantKnown(testGroup1, false)
	r.wantKnown(testGroup2, true)
	joinAssertEqual(t, "owed turns", len(deliv(r.h).heldKeys()), 0)
	joinAssertEqual(t, "closed CLIs", len(r.runtime.closed) > 0, true)
	joinAssertEqual(t, "watchers", len(r.h.Watchers(testGroup1, testAI1)), 0)
}

// A group that is syncing when its home stops listing it is looked at once the
// sync ends, though the connection is gone by then.
func TestGroupItsOwnerWasRemovedFromWhileItSyncedIsLeft(t *testing.T) {
	r := newLeftRig(t)
	d := deliv(r.h)
	r.home.mu.Lock()
	r.home.syncMode = true
	r.home.mu.Unlock()
	job, _ := d.claimOrWaitGroupSync(r.homeKey.ID(), testGroup1)
	synced := make(chan error, 1)
	go func() { synced <- syncOneGroup(t.Context(), r.h, d, r.home, r.homeKey.ID(), job) }()
	<-r.home.started
	r.leftPush(testGroup1, leftGone)
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"groups","groups":[{"g":"`+testGroup2+`","name":"G","seq":10}]}`))
	r.home.closeConn()
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.Synced{Type: "synced", Success: proto.Success{Re: 1, OK: true}, Group: testGroup1, Seq: 20}))}
	joinAssertEqual(t, "sync", <-synced, nil)
	r.h.Settle()
	r.wantKnown(testGroup1, false)
	r.wantKnown(testGroup2, true)
}

// A team group has no AI here: its line and its home go all the same.
func TestTeamGroupItsOwnerLeftIsNoLongerNamed(t *testing.T) {
	r := newLeftRig(t)
	r.leftPush(testGroup2, leftGone)
	r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"groups","groups":[{"g":"`+testGroup1+`","name":"G","seq":10}]}`))
	r.waitHomes(1)
	r.waitTeams(0)
	r.h.Settle()
	r.wantKnown(testGroup1, true)
}

// A group that moved, or whose home goes out of reach, still has the owner in
// its rows: the node keeps it. So it does when it cannot read who the owner is.
func TestGroupThatIsNotListedIsKeptWhileItsRowsHaveTheOwner(t *testing.T) {
	for name, spoil := range map[string]func(r *deliverRig){
		"unreachable": func(r *deliverRig) { r.home.closeConn() },
		"moved": func(r *deliverRig) {
			r.home.mu.Lock()
			r.home.syncMode = true
			r.home.mu.Unlock()
			for range 2 {
				r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.ErrorResponse{
					Re: 1, Err: proto.WireError{Code: "moved", Message: "The group has moved.", Home: r.home2Key.ID()},
				}))}
			}
		},
		"no owner read": func(r *deliverRig) {
			r.home.closeConn()
			r.leftPush(testGroup1, leftGone)
			r.leftPush(testGroup2, leftGone)
			r.db.Write(func(tx *store.Tx) error {
				_, err := tx.Exec("UPDATE setting SET value = 'not json' WHERE key = 'owner'")
				return err
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newLeftRig(t)
			spoil(r)
			r.h.Push(r.homeKey.ID(), json.RawMessage(`{"t":"groups","groups":[]}`))
			if name == "moved" {
				r.waitUntil("both moved", func() bool {
					d := deliv(r.h)
					return d.groupHome(testGroup1) == r.home2Key.ID() && d.groupHome(testGroup2) == r.home2Key.ID()
				})
			}
			r.h.Settle()
			homes, err := joinSavedHomes(r.h.Store)
			joinAssertEqual(t, "remembered homes", len(homes), 2)
			joinAssertEqual(t, "read homes", err, nil)
			joinAssertEqual(t, "session", joinGroupHasSession(r.h, testGroup1), true)
		})
	}
}

// A node that was away at the removal learns it from the refused sync, from
// the group's own home only.
func TestGroupWhoseHomeRefusesItIsLeft(t *testing.T) {
	r := newLeftRig(t)
	d := deliv(r.h)
	forbidden := json.RawMessage(marshalWire(proto.ErrorResponse{Re: 1, Err: proto.WireError{Code: "forbidden", Message: "not a member"}}))
	joinAssertEqual(t, "another home", settleRefusedSync(r.h, d, r.home2Key.ID(), testGroup1, forbidden), false)
	r.wantKnown(testGroup1, true)
	settleHomeSync(r.h, d, r.homeKey.ID(), testGroup1, forbidden, false, true)
	r.wantKnown(testGroup1, false)
	waiter, _ := d.claimOrWaitGroupSync(r.homeKey.ID(), testGroup1)
	_, wait := d.claimOrWaitGroupSync(r.homeKey.ID(), testGroup1)
	d.forget(testGroup1)
	if _, open := <-wait; open || waiter.group != testGroup1 {
		t.Fatal("a waiting sync was not woken")
	}
	joinAssertEqual(t, "unreachable", settleRefusedSync(r.h, d, r.homeKey.ID(), testGroup2, nil), false)
}
