package store

import (
	"bytes"
	"encoding/json"
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
