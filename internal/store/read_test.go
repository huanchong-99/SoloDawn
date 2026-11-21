package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func header(r, id string, seq int64) proto.RowHeader {
	return proto.RowHeader{Type: r, ID: id, Seq: seq}
}

func msg(id string, seq int64, thread *string, main bool, kind string) proto.MsgRow {
	return proto.MsgRow{RowHeader: header("msg", id, seq), Group: "g", Pos: seq, Timestamp: 10, Kind: kind, Thread: thread, Main: main,
		Mentions: []string{}, Body: json.RawMessage("{}")}
}

func put[R wireRow](t *testing.T, s *Store, g string, mt int64, row R) {
	t.Helper()
	check(t, s.Write(func(tx *Tx) error { return Put(tx, g, mt, row) }))
}

func snapshot(t *testing.T, s *Store, g string) *Snapshot {
	t.Helper()
	v, err := s.Snapshot(g)
	check(t, err)
	t.Cleanup(func() { check(t, v.Close()) })
	return v
}

func ids(t *testing.T, rows []json.RawMessage, err error) []string {
	t.Helper()
	check(t, err)
	got := []string{}
	for _, row := range rows {
		var h proto.RowHeader
		check(t, json.Unmarshal(row, &h))
		got = append(got, h.ID)
	}
	return got
}

func TestPagesAndDelta(t *testing.T) {
	s := openDB(t, ":memory:")
	thread := "t"
	other := "t2"
	card := msg("card", 2, &thread, true, "decision")
	put(t, s, "g", 10, msg("main", 1, nil, false, "text"))
	put(t, s, "g", 10, card)
	put(t, s, "g", 10, msg("thread", 3, &thread, false, "text"))
	put(t, s, "g", 10, msg("review", 4, &other, false, "review"))
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "note", 5), Pos: 5, Group: "g"})
	put(t, s, "g", 10, msg("erased", 6, &thread, true, "text"))
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("msg", "erased", 7), Gone: true})
	card.Seq = 8
	card.State = &proto.CardState{Status: "decided"}
	put(t, s, "g", 20, card)
	put(t, s, "other", 10, msg("elsewhere", 1, nil, true, "review"))
	put(t, s, "other", 10, proto.NoteRow{RowHeader: header("note", "elsewhere-note", 2), Pos: 2, Group: "other"})

	v := snapshot(t, s, "g")
	rows, err := v.AfterSeqUpTo(3, 7)
	equal(t, ids(t, rows, err), []string{"review", "note", "erased"})
	rows, err = v.AfterSeqUpTo(7, 8)
	equal(t, ids(t, rows, err), []string{"card"})
	var updated proto.MsgRow
	check(t, json.Unmarshal(rows[0], &updated))
	equal(t, updated, card)
	for _, c := range []struct {
		thread *string
		before int64
		limit  int
		want   []string
	}{
		{nil, 0, 10, []string{"card", "main"}},
		{nil, 9, 2, []string{"card", "main"}},
		{nil, 2, 1, []string{"main"}},
		{&thread, 0, 10, []string{"thread", "card"}},
		{&thread, 9, 1, []string{"thread"}},
		{&thread, 3, 10, []string{"card"}},
	} {
		rows, err = v.PageThread(c.thread, c.before, c.limit)
		equal(t, ids(t, rows, err), c.want)
	}
	for _, c := range []struct {
		kind   string
		before int64
		want   []string
	}{
		{"review", 0, []string{"review"}},
		{"review", 5, []string{"review"}},
		{"review", 4, []string{}},
		{"note", 0, []string{"note"}},
		{"note", 6, []string{"note"}},
		{"note", 5, []string{}},
		{"text", 0, []string{}},
	} {
		rows, err = v.PageKind(c.kind, c.before, 10)
		equal(t, ids(t, rows, err), c.want)
	}
	rows, err = v.PageKind("note", 5, 10)
	check(t, err)
	b, err := json.Marshal(rows)
	check(t, err)
	equal(t, string(b), "[]")
}

func TestSnapshotPinsReplacedRow(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
	equal(t, s.db.Stats().MaxOpenConnections, 0)
	set := func(text string) {
		check(t, s.Write(func(tx *Tx) error {
			seq, err := tx.NextSeq("g")
			if err != nil {
				return err
			}
			r := msg("one", seq, nil, true, "text")
			r.Text = text
			r.Pos = 1
			return Put(tx, "g", seq, r)
		}))
	}
	set("first")
	old := snapshot(t, s, "g")
	set("second")
	fresh := snapshot(t, s, "g")
	for _, c := range []struct {
		view *Snapshot
		seq  int64
		text string
	}{{old, 1, "first"}, {fresh, 2, "second"}} {
		equal(t, c.view.Seq, c.seq)
		equal(t, c.view.Purged, int64(0))
		rows, err := c.view.AfterSeqUpTo(0, c.seq)
		equal(t, ids(t, rows, err), []string{"one"})
		var r proto.MsgRow
		check(t, json.Unmarshal(rows[0], &r))
		equal(t, r.Text, c.text)
		equal(t, r.Seq, c.seq)
		equal(t, r.Pos, int64(1))
	}
}

func TestSnapshotRowPinsView(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
	note := proto.NoteRow{RowHeader: header("note", "n1", 1), Pos: 1, Group: "g", Text: "pinned text"}
	put(t, s, "g", 10, note)
	pinned := snapshot(t, s, "g")
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("note", "n1", 2), Gone: true})
	later := snapshot(t, s, "g")

	got, err := pinned.Row("note", "n1")
	check(t, err)
	want, err := encode(note)
	check(t, err)
	equal(t, string(got), want)
	_, err = later.Row("note", "n1")
	equal(t, errors.Is(err, sql.ErrNoRows), true)
}

func roundTripGolden[R wireRow](t *testing.T, file string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "row-"+file+".json"))
	check(t, err)
	var row R
	check(t, json.Unmarshal(data, &row))
	s := openDB(t, ":memory:")
	put(t, s, "g", 1, row)
	v := snapshot(t, s, "g")
	rows, err := v.AfterSeqUpTo(0, 1000)
	check(t, err)
	equal(t, len(rows), 1)
	var want, got map[string]any
	check(t, json.Unmarshal(data, &want))
	check(t, json.Unmarshal(rows[0], &got))
	equal(t, got, want)
}

func TestStoredWireRows(t *testing.T) {
	roundTripGolden[proto.GroupRow](t, "group")
	roundTripGolden[proto.HumanMemberRow](t, "member-human")
	roundTripGolden[proto.AIMemberRow](t, "member-ai")
	roundTripGolden[proto.DeviceRow](t, "device")
	roundTripGolden[proto.TaskRow](t, "task-waiting")
	roundTripGolden[proto.MsgRow](t, "msg-decision-decided")
	roundTripGolden[proto.NoteRow](t, "note")
	roundTripGolden[proto.GoneRow](t, "gone")
}

func TestInvalidBodyNotWritten(t *testing.T) {
	s := openDB(t, ":memory:")
	r := msg("m", 1, nil, true, "text")
	r.Body = json.RawMessage("{")
	err := s.Write(func(tx *Tx) error { return Put(tx, "g", 1, r) })
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("unexpected end of JSON input")) {
		t.Fatalf("put bad body: %v", err)
	}
	equal(t, scalar[int](t, s.db, "SELECT count(*) FROM row"), 0)
}

func TestPutStoresWithoutHTMLEscapes(t *testing.T) {
	s := openDB(t, ":memory:")
	text := strings.Repeat("<>&", 5000)
	note := proto.NoteRow{RowHeader: header("note", "n", 1), Pos: 1, Group: "g", Text: text}
	body := json.RawMessage(`{"a":"<b>&"}`)
	card := msg("card", 2, nil, false, "text")
	card.Body = body
	put(t, s, "g", 10, note)
	put(t, s, "g", 10, card)
	want, err := encode(note)
	check(t, err)
	if len(want) != len(text)+len(`{"r":"note","id":"n","seq":1,"pos":1,"g":"g","task":null,"text":"","by":"","ts":0}`) {
		t.Fatalf("reference encoding is escaped: %d bytes", len(want))
	}
	var size int
	var stored, storedCard string
	check(t, s.db.QueryRow("SELECT length(data), data FROM row WHERE id='n'").Scan(&size, &stored))
	check(t, s.db.QueryRow("SELECT data FROM row WHERE id='card'").Scan(&storedCard))
	equal(t, stored, want)
	equal(t, size, len(want))
	if !strings.Contains(storedCard, `"body":{"a":"<b>&"}`) {
		t.Fatalf("body was escaped: %s", storedCard)
	}
	var back proto.NoteRow
	check(t, json.Unmarshal([]byte(stored), &back))
	equal(t, back, note)
}

func TestPutKeepsStoredEscapesOfRawJSON(t *testing.T) {
	s := openDB(t, ":memory:")
	card := msg("card", 1, nil, false, "text")
	card.Body = json.RawMessage(`{"a":"<b>"}`)
	put(t, s, "g", 10, card)
	var stored string
	check(t, s.db.QueryRow("SELECT data FROM row WHERE id='card'").Scan(&stored))
	if !strings.Contains(stored, `"body":{"a":"<b>"}`) {
		t.Fatalf("escapes were rewritten: %s", stored)
	}
}

func TestSnapshotAndQueryFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	s := openDB(t, path+"?_txlock=immediate")
	unlock := lockDB(t, path)
	v, err := s.Snapshot("g")
	equal(t, v, (*Snapshot)(nil))
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("locked snapshot begin: %v", err)
	}
	unlock()

	v, err = s.Snapshot("g")
	check(t, err)
	check(t, v.Close())
	rows, err := v.AfterSeqUpTo(0, 1)
	equal(t, rows, ([]json.RawMessage)(nil))
	equal(t, err, sql.ErrTxDone)

	mem := openDB(t, ":memory:")
	_, err = mem.db.Exec("DROP TABLE grp")
	check(t, err)
	v, err = mem.Snapshot("g")
	equal(t, v, (*Snapshot)(nil))
	equal(t, err.Error(), "SQL logic error: no such table: grp (1)")
	equal(t, mem.db.Stats().InUse, 0)
}

func emptyJSON(t *testing.T, rows []json.RawMessage, err error) {
	t.Helper()
	check(t, err)
	raw, err := json.Marshal(rows)
	check(t, err)
	equal(t, string(raw), "[]")
}

func TestSnapshotStateRowsAndPending(t *testing.T) {
	s := openDB(t, ":memory:")
	thread := "t1"
	put(t, s, "g", 10, proto.GroupRow{RowHeader: header("group", "g", 1)})
	put(t, s, "g", 10, member("m1", 2))
	put(t, s, "g", 10, proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: header("member", "ai1", 3), Kind: "ai"}})
	put(t, s, "g", 10, proto.DeviceRow{RowHeader: header("device", "d1", 4)})
	put(t, s, "g", 10, proto.TaskRow{RowHeader: header("task", "t1", 5)})
	put(t, s, "g", 10, msg("m6", 6, nil, false, "text"))
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "n7", 7), Pos: 7})
	c8 := card("c8", 8, &thread, "pending")
	c8.Main = false
	put(t, s, "g", 10, c8)
	put(t, s, "g", 10, card("c9", 9, nil, "pending"))
	put(t, s, "g", 10, card("c10", 10, nil, "decided"))
	put(t, s, "g", 10, member("m11", 11))
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("member", "m11", 12), Gone: true})
	put(t, s, "g", 10, card("c13", 13, nil, "pending"))
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("msg", "c13", 14), Gone: true})
	put(t, s, "g", 30, member("m1", 15))
	c8.Seq = 16
	put(t, s, "g", 30, c8)
	put(t, s, "other", 10, proto.GroupRow{RowHeader: header("group", "other", 1)})
	put(t, s, "other", 10, card("other-card", 2, nil, "pending"))

	v, err := s.Snapshot("g")
	check(t, err)
	rows, err := v.StateRows()
	equal(t, ids(t, rows, err), []string{"g", "ai1", "d1", "t1", "m1"})
	rows, err = v.Pending()
	equal(t, ids(t, rows, err), []string{"c8", "c9"})
	check(t, v.Close())

	empty := snapshot(t, s, "absent")
	rows, err = empty.StateRows()
	emptyJSON(t, rows, err)
	rows, err = empty.Pending()
	emptyJSON(t, rows, err)
}

func TestSnapshotDeltaCount(t *testing.T) {
	s := openDB(t, ":memory:")
	put(t, s, "g", 10, proto.GroupRow{RowHeader: header("group", "g", 1)})
	put(t, s, "g", 10, msg("m2", 2, nil, false, "text"))
	put(t, s, "g", 10, member("mem3", 3))
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "n4", 4), Pos: 4})
	put(t, s, "g", 10, proto.TaskRow{RowHeader: header("task", "t5", 5)})
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("msg", "gone-msg", 6), Gone: true})
	put(t, s, "g", 20, proto.DeviceRow{RowHeader: header("device", "dev7", 7)})
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("member", "gone-mem", 8), Gone: true})
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("note", "gone-note", 9), Gone: true})
	put(t, s, "g", 20, proto.TaskRow{RowHeader: header("task", "t10", 10)})
	put(t, s, "other", 10, proto.TaskRow{RowHeader: header("task", "other-task", 4)})

	v := snapshot(t, s, "g")
	for _, c := range []struct {
		after, upTo int64
		want        int
	}{
		{0, 10, 10},
		{1, 10, 9},
		{0, 9, 9},
		{5, 8, 3},
		{10, 10, 0},
		{12, 10, 0},
	} {
		got, err := v.DeltaCount(c.after, c.upTo)
		check(t, err)
		equal(t, got, c.want)
	}
}

func TestSnapshotSyncIsolationAndClosedErrors(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
	seedSnapshotStep(t, s, proto.GroupRow{RowHeader: header("group", "g", 1)}, card("c2", 2, nil, "pending"))
	old := snapshot(t, s, "g")
	seedSnapshotStep(t, s, card("c2", 3, nil, "decided"), proto.TaskRow{RowHeader: header("task", "t4", 4)})
	fresh := snapshot(t, s, "g")

	rows, err := old.StateRows()
	equal(t, ids(t, rows, err), []string{"g"})
	rows, err = old.Pending()
	equal(t, ids(t, rows, err), []string{"c2"})
	n, err := old.DeltaCount(0, old.Seq)
	equal(t, []any{n, err}, []any{2, nil})

	rows, err = fresh.StateRows()
	equal(t, ids(t, rows, err), []string{"g", "t4"})
	rows, err = fresh.Pending()
	equal(t, ids(t, rows, err), []string{})
	n, err = fresh.DeltaCount(0, fresh.Seq)
	equal(t, []any{n, err}, []any{3, nil})

	closed, err := s.Snapshot("g")
	check(t, err)
	check(t, closed.Close())
	assertClosedSnapshot(t, closed)
}

func seedSnapshotStep[A, B wireRow](t *testing.T, s *Store, first A, second B) {
	t.Helper()
	check(t, s.Write(func(tx *Tx) error {
		for range 2 {
			if _, err := tx.NextSeq("g"); err != nil {
				return err
			}
		}
		if err := Put(tx, "g", 10, first); err != nil {
			return err
		}
		return Put(tx, "g", 10, second)
	}))
}

func assertClosedSnapshot(t *testing.T, v *Snapshot) {
	t.Helper()
	rows, err := v.StateRows()
	equal(t, []any{rows, err}, []any{[]json.RawMessage(nil), sql.ErrTxDone})
	rows, err = v.Pending()
	equal(t, []any{rows, err}, []any{[]json.RawMessage(nil), sql.ErrTxDone})
	n, err := v.DeltaCount(0, 1)
	equal(t, []any{n, err}, []any{0, sql.ErrTxDone})
}
