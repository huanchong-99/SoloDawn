package home_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestMoveIsRememberedAndCanBeImportedBack(t *testing.T) {
	w, p := runFounded(t, runTables())
	file := filepath.Join(t.TempDir(), "move.db")
	exportStoreLogs(t, w.db, logOf(t, browser, node, laptop))
	equal(t, w.loop.ExportGroup(runOther, file, laptop.ID()).Error(), "this node is not home to the group")
	equal(t, w.loop.ExportGroup(groupID, file, ""), nil)
	equal(t, len(w.loop.Groups(p.conn)), 1)

	equal(t, w.loop.ExportGroup(groupID, file, laptop.ID()), nil)
	p.gets(runGroupsFrame())
	w.loop.Close()
	reopened := runStart(t, w.db, runTables())
	r := reopened.attach(browser)
	equal(t, len(reopened.loop.Groups(r.conn)), 0)
	moved := func(re int64) string {
		return runFrame(proto.ErrorResponse{Re: re, Err: proto.WireError{Code: "moved", Message: "The group has moved.", Home: laptop.ID()}})
	}
	reopened.loop.Fetch(r.conn, home.Fetch{FetchPage: proto.FetchPage{Request: proto.Request{ID: 11}, Group: groupID}})
	r.gets(moved(11))
	reopened.loop.Fetch(r.conn, home.Fetch{FetchPage: proto.FetchPage{Request: proto.Request{ID: 12}, Group: groupID, Limit: 1}})
	r.gets(moved(12))
	reopened.loop.Sync(r.conn, proto.Sync{Request: proto.Request{ID: 13}, Group: groupID})
	r.gets(moved(13))
	r.gets(moved(r.send(groupID, "group", runCreateWebsite)))

	load := func(*store.Tx) error { return nil }
	fail := func(*store.Tx) error { return errors.New("bad log") }
	equal(t, reopened.loop.ImportGroup(groupID, file, fail, func() { t.Error("installed failed import") }).Error(), "bad log")
	reopened.loop.Sync(r.conn, proto.Sync{Request: proto.Request{ID: 14}, Group: groupID})
	r.gets(moved(14))
	equal(t, reopened.loop.ImportGroup(groupID, file, load, func() {}), nil)
	r.gets(runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 6}))
	checkDevices := func(tx *store.Tx) error {
		rows, err := tx.StateRows(groupID)
		equal(t, string(rows[len(rows)-1]), string(briefRaw(t, grpOwnerDevices(t, 4)[2])))
		return err
	}
	equal(t, w.db.Write(checkDevices), nil)

	again := filepath.Join(t.TempDir(), "again.db")
	equal(t, reopened.loop.ExportGroup(groupID, again, laptop.ID()), nil)
	r.gets(runGroupsFrame())
	equal(t, reopened.loop.ImportGroup(groupID, again, load, func() {}), nil)
	r.gets(runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 6}))
	reopened.loop.Close()
	equal(t, reopened.loop.ImportGroup(groupID, file, load, func() {}).Error(), "the home is closed")
}

func TestInvalidMovedSettingAndMalformedGroup(t *testing.T) {
	w, _ := runFounded(t, runTables())
	file := filepath.Join(t.TempDir(), "bad.db")
	runExec(t, w.db, `UPDATE row SET data=json_set(data,'$.id','wrong') WHERE r='group'`)
	equal(t, w.loop.ExportGroup(groupID, file, ""), nil)
	target := runStart(t, runOpen(t), runTables())
	load := func(*store.Tx) error { return nil }
	equal(t, target.loop.ImportGroup(groupID, file, load, func() {}).Error(), "the file has no group row")

	runExec(t, target.db, "INSERT INTO setting VALUES('moved','{')")
	_, err := home.StartLoop(home.LoopConfig{Store: target.db})
	if err == nil || !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("opening a corrupted moved setting: %v", err)
	}
	runExec(t, target.db, "DROP TABLE setting")
	_, err = home.StartLoop(home.LoopConfig{Store: target.db})
	if err == nil || !strings.Contains(err.Error(), "no such table: setting") {
		t.Fatalf("opening a database without a setting table: %v", err)
	}
}

// exportStoreLogs stores logs in device_log, as the node keeps them.
func exportStoreLogs(t *testing.T, db *store.Store, logs ...*identity.Log) {
	t.Helper()
	for _, log := range logs {
		for i, env := range log.Entries {
			runExec(t, db, "INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)", log.User, i+1, string(briefRaw(t, env)))
		}
	}
}

// exportDevice is entry ver of log as the device row with seq.
func exportDevice(t *testing.T, log *identity.Log, ver int, seq int64) proto.DeviceRow {
	t.Helper()
	var change proto.DeviceChange
	equal(t, proto.Decode([]byte(log.Entries[ver-1].Payload), &change), nil)
	header := proto.RowHeader{Type: "device", ID: fmt.Sprintf("%s:%d", log.User, ver), Seq: seq}
	return proto.DeviceRow{RowHeader: header, DeviceChange: change, Env: log.Entries[ver-1]}
}

// An import adds the device rows that the group lacks of the stored logs of
// its people, in its own transaction, and rewrites none that the file holds.
func TestImportAddsTheMissingDeviceRows(t *testing.T) {
	w, _ := runFounded(t, runTables())
	owner, person, stranger := logOf(t, browser, node, laptop), logOf(t, guest, nobody), logOf(t, nobody)
	member := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id(12), Seq: 4},
		Group: groupID, Kind: "human", User: guest.ID(), Name: "Q"}, Role: "member"}
	file := []any{member, exportDevice(t, owner, 1, 5), exportDevice(t, person, 1, 6)}
	equal(t, w.db.Write(func(tx *store.Tx) error {
		_, err := tx.NextSeq(groupID)
		_, err2 := tx.NextSeq(groupID)
		_, err3 := tx.NextSeq(groupID)
		return errors.Join(err, err2, err3, store.Put(tx, groupID, clock, member),
			store.Put(tx, groupID, clock, file[1].(proto.DeviceRow)), store.Put(tx, groupID, clock, file[2].(proto.DeviceRow)))
	}), nil)
	exportStoreLogs(t, w.db, owner, person, stranger)
	path := filepath.Join(t.TempDir(), "rows.db")
	equal(t, w.loop.ExportGroup(groupID, path, laptop.ID()), nil)
	equal(t, w.loop.ImportGroup(groupID, path, func(*store.Tx) error { return nil }, func() {}), nil)

	added := []any{exportDevice(t, owner, 2, 7), exportDevice(t, owner, 3, 8), exportDevice(t, person, 2, 9)}
	if guest.ID() < browser.ID() {
		added = []any{exportDevice(t, person, 2, 7), exportDevice(t, owner, 2, 8), exportDevice(t, owner, 3, 9)}
	}
	var rows []json.RawMessage
	equal(t, w.db.Write(func(tx *store.Tx) (err error) { rows, err = tx.StateRows(groupID); return err }), nil)
	equal(t, len(rows), 9)
	for i, want := range append(file, added...) {
		equal(t, string(rows[i+3]), string(briefRaw(t, want)))
	}
}
