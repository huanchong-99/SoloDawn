package home_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const retDay = int64(86_400_000)

func retSetSetting(t *testing.T, db *store.Store, key, value string) {
	t.Helper()
	runExec(t, db, "INSERT INTO setting(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
}

func retRun(t *testing.T, db *store.Store, now int64) {
	t.Helper()
	run := func(tx *store.Tx) error { return home.Retain(tx, now) }
	if err := db.Write(run); err != nil {
		t.Fatal(err)
	}
}

func retPutRow[R interface {
	proto.GroupRow | proto.HumanMemberRow | proto.AIMemberRow | proto.DeviceRow |
		proto.TaskRow | proto.MsgRow | proto.NoteRow | proto.GoneRow
}](t *testing.T, db *store.Store, group string, mt int64, row R) {
	t.Helper()
	writeRow := func(tx *store.Tx) error {
		if _, err := tx.NextSeq(group); err != nil {
			return err
		}
		return store.Put(tx, group, mt, row)
	}
	if err := db.Write(writeRow); err != nil {
		t.Fatal(err)
	}
}

func retRowIDs(t *testing.T, db *store.Store, group string) string {
	t.Helper()
	return runQuery[string](t, db, "SELECT coalesce(group_concat(id, ',' ORDER BY seq), '') FROM row WHERE g=? AND r NOT IN ('group', 'member')", group)
}

func retPutMsgAt(t *testing.T, db *store.Store, group, msgID string, seq, mt, ts int64) {
	t.Helper()
	retPutRow(t, db, group, mt, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: seq}, Pos: seq, Group: group,
		By: ownerID, Kind: "text", Timestamp: ts, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
}

// Retain uses the default 90 days when retention_days is absent and surfaces
// errors from reading setting and from tx.Retain.
func TestRetentionDefaultDaysAndStoreErrors(t *testing.T) {
	const now = int64(1_763_600_000_000)
	w, _ := runFounded(t, runTables())
	cut90 := time.UnixMilli(now).AddDate(0, 0, -90).UnixMilli()
	retPutMsgAt(t, w.db, groupID, "old-90", 4, cut90, cut90-1)
	retPutMsgAt(t, w.db, groupID, "edge-90", 5, cut90, cut90)

	retRun(t, w.db, now)
	equal(t, retRowIDs(t, w.db, groupID), "edge-90")

	dbNoInbox := runOpen(t)
	retSetSetting(t, dbNoInbox, "retention_days", `7`)
	runExec(t, dbNoInbox, "DROP TABLE inbox")
	err := dbNoInbox.Write(func(tx *store.Tx) error { return home.Retain(tx, now) })
	if err == nil || !strings.Contains(err.Error(), "no such table: inbox") {
		t.Fatalf("got %v; want missing inbox error", err)
	}

	dbNoSetting := runOpen(t)
	runExec(t, dbNoSetting, "DROP TABLE setting")
	err = dbNoSetting.Write(func(tx *store.Tx) error { return home.Retain(tx, now) })
	if err == nil || !strings.Contains(err.Error(), "no such table: setting") {
		t.Fatalf("got %v; want missing setting error", err)
	}
}

// A2.5 and A5.14: Retain respects configured retention_days, keeps pending
// cards, notes, review messages and boundary rows, deletes older messages and
// finished tasks, and advances grp.purged so Sync resets cursors below purged.
func TestRetentionConfiguredDaysProtectedRowsAndSyncReset(t *testing.T) {
	db := runOpen(t)
	retSetSetting(t, db, "retention_days", `7`)
	w := runStart(t, db, runTables())
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()

	day8 := int64(clock) + 8*retDay
	cut8 := time.UnixMilli(day8).AddDate(0, 0, -7).UnixMilli()
	oldMarkerSeq, freshMarkerSeq := retSeedBoundaryRows(t, db, cut8)
	retRun(t, db, day8)

	view, err := db.Snapshot(groupID)
	equal(t, err, nil)
	equal(t, view.Purged, oldMarkerSeq)
	view.Close()

	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id IN ('msg-old','card-decided','gone-old','t-merged','t-cancelled')", groupID), 0)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id IN ('msg-cut','card-pending','msg-review','note-keep','gone-cut','t-active','t-edge')", groupID), 7)
	equal(t, retSyncResets(t, w, oldMarkerSeq-1), true)
	equal(t, retSyncResets(t, w, oldMarkerSeq), false)

	retRun(t, db, day8+retDay)
	equal(t, runQuery[int64](t, db, "SELECT purged FROM grp WHERE g=?", groupID), freshMarkerSeq)
}

func retSeedBoundaryRows(t *testing.T, db *store.Store, cut8 int64) (oldMarkerSeq, freshMarkerSeq int64) {
	t.Helper()
	old := cut8 - 1
	nextSeq := runQuery[int64](t, db, "SELECT seq FROM grp WHERE g=?", groupID)
	addMsg := func(msgID, kind, status string, ts int64) {
		nextSeq++
		var state *proto.CardState
		if status != "" {
			state = &proto.CardState{Status: status}
		}
		retPutRow(t, db, groupID, ts, proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: nextSeq}, Pos: nextSeq, Group: groupID,
			By: ownerID, Kind: kind, Timestamp: ts, Mentions: []string{}, Body: json.RawMessage(`{}`), State: state,
		})
	}
	addMsg("msg-old", "text", "", old)
	addMsg("msg-cut", "text", "", cut8)
	addMsg("card-pending", "decision", "pending", old)
	addMsg("card-decided", "decision", "decided", old)
	addMsg("msg-review", "review", "", old)

	for _, task := range []struct {
		id, state string
		mt        int64
	}{
		{"t-done", "done", old},
		{"t-cancelled", "cancelled", old},
		{"t-active", "active", old},
		{"t-edge", "done", cut8},
	} {
		nextSeq++
		retPutRow(t, db, groupID, task.mt, proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: task.id, Seq: nextSeq}, Group: groupID,
			Num: int(nextSeq), Title: task.id, Origin: "direct", State: task.state,
			Assignee: aiID, Reviewer: aiID, Pass: 90, Depends: []string{}, Created: old,
		})
	}

	nextSeq++
	retPutRow(t, db, groupID, old, proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: "note-keep", Seq: nextSeq}, Pos: nextSeq,
		Group: groupID, Text: "remember", By: ownerID, Timestamp: old,
	})
	nextSeq++
	oldMarkerSeq = nextSeq
	retPutRow(t, db, groupID, old, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-old", Seq: oldMarkerSeq}, Gone: true})
	nextSeq++
	freshMarkerSeq = nextSeq
	retPutRow(t, db, groupID, cut8, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-cut", Seq: freshMarkerSeq}, Gone: true})
	return oldMarkerSeq, freshMarkerSeq
}

func retSyncResets(t *testing.T, w *runWorld, since int64) bool {
	t.Helper()
	reader := w.attach(browser)
	defer w.loop.Detach(reader.conn)
	reader.re++
	w.loop.Sync(reader.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: reader.re}, Group: groupID, Since: since})
	reset := false
	for {
		var head struct {
			Type string `json:"t"`
			Re   int64  `json:"re"`
		}
		if err := json.Unmarshal([]byte(reader.next()), &head); err != nil {
			t.Fatal(err)
		}
		if head.Type == "reset" {
			reset = true
		}
		if head.Type == "synced" && head.Re == reader.re {
			return reset
		}
	}
}

// A finished task that a queued task depends on survives retention, so the
// restarted home still sees the dependency as done.
func TestRetentionKeepsADependencyAcrossARestart(t *testing.T) {
	tables := runTables()
	var seen []string
	tables.Commands["note"] = func(s *home.State, _ home.Command) (home.Result, *home.Refusal) {
		for _, id := range []string{"a", "b"} {
			seen = append(seen, id+":"+s.Tasks[id].State)
		}
		return home.Result{}, nil
	}
	w, _ := runFounded(t, tables)
	w.loop.Close()
	old := int64(1)
	retSetSetting(t, w.db, "retention_days", `7`)
	retPutRow(t, w.db, groupID, old, proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: "a", Seq: 4}, Group: groupID, State: "done"})
	retPutRow(t, w.db, groupID, old, proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: "b", Seq: 5}, Group: groupID, State: "queued", Depends: []string{"a"}})

	retRun(t, w.db, clock+10*retDay)
	restarted := runStart(t, w.db, tables)
	restarted.attach(browser).does(groupID, "note", "")
	equal(t, seen, []string{"a:done", "b:queued"})
}
