package host

import (
	"encoding/json"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestGroupAnnouncementsFollowMoveAndRetryAfterImport(t *testing.T) {
	r := newDeliverRig(t, false)
	old, next := r.homeKey.ID(), r.ownerKey.ID()
	entry := proto.Home{Group: testGroup1, Home: old, Name: "Project"}
	if err := r.h.RecordHome(entry, []string{"wss://example.test/rv"}); err != nil {
		t.Fatal(err)
	}
	r.home.syncMode = true
	r.h.Attach(old, r.home)
	<-r.home.started
	r.home.answers <- syncAnswer{raw: moveTestSynced(1)}
	r.waitGroupLive(testGroup1)

	r.h.Push(old, json.RawMessage(`{"t":"groups"}`))
	r.h.Push(old, json.RawMessage(`{"t":"groups","groups":[]}`))
	<-r.home.started
	r.home.answers <- syncAnswer{raw: json.RawMessage(marshalWire(proto.ErrorResponse{
		Re: 2, Err: proto.WireError{Code: "moved", Message: "The group has moved.", Home: next},
	}))}
	r.waitUntil("saved destination", func() bool {
		return deliv(r.h).groupHome(testGroup1) == next
	})
	addrs, err := movedHomeAddrs(r.h.Store, next)
	joinAssertEqual(t, "address read", err, nil)
	joinAssertEqual(t, "address count", len(addrs), 1)
	joinAssertEqual(t, "remembered signalling address", addrs[0], "wss://example.test/rv")

	destination := newFakeDeliverHome()
	destination.syncMode = true
	defer destination.closeConn()
	r.h.Attach(next, destination)
	<-destination.started
	present := proto.Groups{Type: "groups", Groups: []proto.GroupSummary{{Group: testGroup1, Name: "Project", Seq: 5}}}
	r.h.Push(next, json.RawMessage(marshalWire(present)))
	destination.answers <- syncAnswer{raw: json.RawMessage(`{"re":1,"err":{"code":"not_found","msg":"not imported"}}`)}
	req := <-destination.started
	joinAssertEqual(t, "retry after import", req.Since, int64(0))
	destination.answers <- syncAnswer{raw: moveTestSynced(2)}
	r.waitGroupLive(testGroup1)
}

func moveTestSynced(re int64) json.RawMessage {
	return json.RawMessage(marshalWire(proto.Synced{
		Type: "synced", Success: proto.Success{Re: re, OK: true}, Group: testGroup1, Seq: 5,
	}))
}

func TestFollowMovedHomeKeepsTheCurrentHomeOnErrors(t *testing.T) {
	r := newDeliverRig(t, false)
	old, next := r.homeKey.ID(), r.ownerKey.ID()
	joinAssertEqual(t, "invalid destination", movedHome(json.RawMessage(`{"re":1,"err":{"code":"moved","msg":"moved","home":"invalid"}}`)), "")
	joinAssertEqual(t, "unknown group", followMovedHome(r.h, old, testGroup1, next), nil)
	joinAssertEqual(t, "remember group", r.h.RecordHome(proto.Home{Group: testGroup1, Home: old, Name: "Project"}, nil), nil)
	err := r.h.Store.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE peer")
		return err
	})
	joinAssertEqual(t, "drop the address table", err, nil)
	err = followMovedHome(r.h, old, testGroup1, next)
	if err == nil {
		t.Fatal("missing address table did not fail")
	}
	d := deliv(r.h)
	settleHomeSync(r.h, d, old, testGroup1, json.RawMessage(marshalWire(proto.ErrorResponse{
		Re: 1, Err: proto.WireError{Code: "moved", Message: "Moved", Home: next},
	})), false, true)
	homes, err := joinSavedHomes(r.h.Store)
	joinAssertEqual(t, "read current homes", err, nil)
	joinAssertEqual(t, "keep current home", homes[0].Home, old)
	r.setSetting("homes", "not a list")
	if err := followMovedHome(r.h, old, testGroup1, next); err == nil {
		t.Fatal("invalid homes setting did not fail")
	}
}

func TestHomeAnnouncementKeepsUnrelatedAndCurrentGroups(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	d := deliv(h)
	d.groupLocked("current", "home").live = true
	d.groupLocked("unrelated", "other").live = false
	joinAssertEqual(t, "no sync needed", len(d.announcedGroups("home", map[string]bool{"current": true})), 0)
}

func TestMovedHomeRejectsMalformedAddressStorage(t *testing.T) {
	db := seamStore(t)
	err := db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("DROP TABLE peer; CREATE TABLE peer(device TEXT, url TEXT); INSERT INTO peer VALUES('home', NULL)")
		return err
	})
	joinAssertEqual(t, "malformed address storage", err, nil)
	_, err = movedHomeAddrs(db, "home")
	joinAssertEqual(t, "reject null address", err != nil, true)
}
