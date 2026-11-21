package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func synAI(id string, seq int64) proto.AIMemberRow {
	return proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: header("member", id, seq), Kind: "ai"}}
}

func synEmptyJSON(t *testing.T, rows []json.RawMessage, err error) {
	t.Helper()
	check(t, err)
	raw, err := json.Marshal(rows)
	check(t, err)
	equal(t, string(raw), "[]")
}

func TestSyncStateRowsAndPending(t *testing.T) {
	s := openDB(t, ":memory:")
	thread := "t1"
	put(t, s, "g", 10, proto.GroupRow{RowHeader: header("group", "g", 1)})
	put(t, s, "g", 10, member("m1", 2))
	put(t, s, "g", 10, synAI("ai1", 3))
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
	synEmptyJSON(t, rows, err)
	rows, err = empty.Pending()
	synEmptyJSON(t, rows, err)
}

func TestSyncDeltaCountAndStateDelta(t *testing.T) {
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

	rows, err := v.StateAfterSeqUpTo(0, 10)
	equal(t, ids(t, rows, err), []string{"g", "mem3", "t5", "dev7", "gone-mem", "t10"})
	rows, err = v.StateAfterSeqUpTo(1, 8)
	equal(t, ids(t, rows, err), []string{"mem3", "t5", "dev7", "gone-mem"})
	rows, err = v.StateAfterSeqUpTo(8, 9)
	synEmptyJSON(t, rows, err)
}

func TestSyncSnapshotIsolationAndClosedErrors(t *testing.T) {
	s := openDB(t, filepath.Join(t.TempDir(), "node.db"))
	putStep := func(run func(tx *Tx) error) {
		check(t, s.Write(run))
	}
	putStep(func(tx *Tx) error {
		for range 2 {
			if _, err := tx.NextSeq("g"); err != nil {
				return err
			}
		}
		if err := Put(tx, "g", 10, proto.GroupRow{RowHeader: header("group", "g", 1)}); err != nil {
			return err
		}
		return Put(tx, "g", 10, card("c2", 2, nil, "pending"))
	})
	old := snapshot(t, s, "g")
	putStep(func(tx *Tx) error {
		for range 2 {
			if _, err := tx.NextSeq("g"); err != nil {
				return err
			}
		}
		if err := Put(tx, "g", 20, card("c2", 3, nil, "decided")); err != nil {
			return err
		}
		return Put(tx, "g", 20, proto.TaskRow{RowHeader: header("task", "t4", 4)})
	})
	fresh := snapshot(t, s, "g")

	rows, err := old.StateRows()
	equal(t, ids(t, rows, err), []string{"g"})
	rows, err = old.Pending()
	equal(t, ids(t, rows, err), []string{"c2"})
	n, err := old.DeltaCount(0, old.Seq)
	equal(t, []any{n, err}, []any{2, nil})
	rows, err = old.StateAfterSeqUpTo(0, 10)
	equal(t, ids(t, rows, err), []string{"g"})

	rows, err = fresh.StateRows()
	equal(t, ids(t, rows, err), []string{"g", "t4"})
	rows, err = fresh.Pending()
	equal(t, ids(t, rows, err), []string{})
	n, err = fresh.DeltaCount(0, fresh.Seq)
	equal(t, []any{n, err}, []any{3, nil})

	closed, err := s.Snapshot("g")
	check(t, err)
	check(t, closed.Close())
	synAssertClosedSnapshot(t, closed)
}

func synAssertClosedSnapshot(t *testing.T, v *Snapshot) {
	t.Helper()
	rows, err := v.StateRows()
	equal(t, []any{rows, err}, []any{[]json.RawMessage(nil), sql.ErrTxDone})
	rows, err = v.Pending()
	equal(t, []any{rows, err}, []any{[]json.RawMessage(nil), sql.ErrTxDone})
	n, err := v.DeltaCount(0, 1)
	equal(t, []any{n, err}, []any{0, sql.ErrTxDone})
	rows, err = v.StateAfterSeqUpTo(0, 1)
	equal(t, []any{rows, err}, []any{[]json.RawMessage(nil), sql.ErrTxDone})
}
