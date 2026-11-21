package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestFetchSnapshotRowReadsLiveRowsOnly(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
	liveMsg := msg("m1", 1, nil, true, "text")
	liveMsg.Text = "a < b & c > d"
	sameIDNote := proto.NoteRow{RowHeader: header("note", "m1", 2), Pos: 2, Group: "g", Text: "note with m1 id"}
	task := proto.TaskRow{RowHeader: header("task", "t1", 3), Group: "g", Num: 1, Depends: []string{}}
	otherGroup := msg("m1", 1, nil, true, "text")
	otherGroup.Group = "other"
	otherGroup.Text = "in another group"

	put(t, s, "g", 10, liveMsg)
	put(t, s, "g", 10, sameIDNote)
	put(t, s, "g", 10, task)
	put(t, s, "g", 10, proto.NoteRow{RowHeader: header("note", "erased", 4), Pos: 4, Group: "g", Text: "temporary"})
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("note", "erased", 5), Gone: true})
	put(t, s, "other", 10, otherGroup)

	v := snapshot(t, s, "g")
	fetAssertRow(t, v, "msg", "m1", liveMsg)
	fetAssertRow(t, v, "note", "m1", sameIDNote)
	fetAssertRow(t, v, "task", "t1", task)

	for _, missing := range [][2]string{
		{"note", "erased"},
		{"msg", "absent"},
		{"member", "m1"},
	} {
		_, err := v.Row(missing[0], missing[1])
		equal(t, errors.Is(err, sql.ErrNoRows), true)
	}

	other := snapshot(t, s, "other")
	fetAssertRow(t, other, "msg", "m1", otherGroup)

	empty := snapshot(t, s, "none")
	_, err := empty.Row("msg", "m1")
	equal(t, errors.Is(err, sql.ErrNoRows), true)
}

func TestFetchSnapshotRowPinsViewAndReportsClosedSnapshot(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
	note := proto.NoteRow{RowHeader: header("note", "n1", 1), Pos: 1, Group: "g", Text: "pinned text"}
	put(t, s, "g", 10, note)

	pinned := snapshot(t, s, "g")
	put(t, s, "g", 20, proto.GoneRow{RowHeader: header("note", "n1", 2), Gone: true})
	later := snapshot(t, s, "g")

	fetAssertRow(t, pinned, "note", "n1", note)
	_, err := later.Row("note", "n1")
	equal(t, errors.Is(err, sql.ErrNoRows), true)

	closed, err := s.Snapshot("g")
	check(t, err)
	check(t, closed.Close())
	_, err = closed.Row("note", "n1")
	equal(t, err, sql.ErrTxDone)
}

func fetAssertRow[R wireRow](t *testing.T, v *Snapshot, r, id string, want R) {
	t.Helper()
	got, err := v.Row(r, id)
	check(t, err)
	encoded, err := encode(want)
	check(t, err)
	equal(t, string(got), encoded)
}
