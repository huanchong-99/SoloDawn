package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func header(r, id string, seq int64) proto.RowHeader {
	return proto.RowHeader{Type: r, ID: id, Seq: seq}
}

func msg(id string, seq int64, thread *string, main bool, kind string) proto.MsgRow {
	return proto.MsgRow{RowHeader: header("msg", id, seq), Group: "g", Pos: seq, Timestamp: 10, Kind: kind, Thread: thread, Main: main}
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
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "note", 5), Group: "g"})
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("msg", "erased", 6), Gone: true})
	card.Seq = 7
	card.State = &proto.CardState{Status: "decided"}
	put(t, s, "g", 20, card)
	put(t, s, "other", 10, msg("elsewhere", 1, nil, true, "review"))

	v := snapshot(t, s, "g")
	rows, err := v.AfterSeqUpTo(3, 6)
	equal(t, ids(t, rows, err), []string{"review", "note", "erased"})
	rows, err = v.AfterSeqUpTo(6, 7)
	equal(t, ids(t, rows, err), []string{"card"})
	var updated proto.MsgRow
	check(t, json.Unmarshal(rows[0], &updated))
	card.Mentions = []string{}
	card.Body = json.RawMessage("{}")
	equal(t, updated, card)
	rows, err = v.PageThread(nil, 8, 2)
	equal(t, ids(t, rows, err), []string{"card", "main"})
	rows, err = v.PageThread(nil, 2, 1)
	equal(t, ids(t, rows, err), []string{"main"})
	rows, err = v.PageThread(&thread, 8, 1)
	equal(t, ids(t, rows, err), []string{"thread"})
	rows, err = v.PageThread(&thread, 3, 10)
	equal(t, ids(t, rows, err), []string{"card"})
	rows, err = v.PageKind("review", 5, 10)
	equal(t, ids(t, rows, err), []string{"review"})
	rows, err = v.PageKind("note", 6, 10)
	equal(t, ids(t, rows, err), []string{"note"})
	rows, err = v.PageKind("note", 5, 10)
	check(t, err)
	b, err := json.Marshal(rows)
	check(t, err)
	equal(t, string(b), "[]")
}

func TestSnapshotPinsReplacedRow(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
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
	roundTripGolden[proto.MsgRow](t, "msg")
	roundTripGolden[proto.NoteRow](t, "note")
	roundTripGolden[proto.GoneRow](t, "gone")
}

func TestEmptyCollections(t *testing.T) {
	s := openDB(t, ":memory:")
	put(t, s, "g", 1, proto.GroupRow{RowHeader: header("group", "g", 1)})
	put(t, s, "g", 2, proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: header("member", "m", 2)}})
	put(t, s, "g", 3, proto.TaskRow{RowHeader: header("task", "t", 3)})
	put(t, s, "g", 4, msg("m", 4, nil, true, "text"))
	rows, err := snapshot(t, s, "g").AfterSeqUpTo(0, 4)
	check(t, err)
	equal(t, len(rows), 4)
	for i, fields := range []map[string]string{{"checks": "[]", "paused": "null"}, {"allow": "[]", "approvers": "[]", "usage": "{}"}, {"depends": "[]", "checks": "{}", "wait": "null"}, {"mentions": "[]", "body": "{}", "state": "null", "thread": "null", "re": "null"}} {
		var got map[string]json.RawMessage
		check(t, json.Unmarshal(rows[i], &got))
		for k, want := range fields {
			equal(t, string(got[k]), want)
		}
	}
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
