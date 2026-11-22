package home_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestMoveIsRememberedAndCanBeImportedBack(t *testing.T) {
	w, p := runFounded(t, runTables())
	file := filepath.Join(t.TempDir(), "move.db")
	equal(t, w.loop.ExportGroup(groupID, file, node.ID()).Error(), "the group is already on this node")
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
	r.gets(runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 3}))
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
