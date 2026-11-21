package store

import (
	"encoding/json"
	"math"
)

// Snapshot is a read transaction at Seq. Close it when done reading.
type Snapshot struct {
	tx          *Tx
	group       string
	Seq, Purged int64
}

func (s *Store) Snapshot(group string) (*Snapshot, error) {
	v := &Snapshot{group: group}
	err := s.onWriter(func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		v.tx = &Tx{tx}
		// This first read pins the SQLite snapshot, including for a new group.
		err = tx.QueryRow("SELECT coalesce(max(seq),0), coalesce(max(purged),0) FROM grp WHERE g=?", group).Scan(&v.Seq, &v.Purged)
		if err != nil {
			tx.Rollback()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Snapshot) Close() error { return v.tx.Rollback() }

// StateRows returns the group, member, device and task rows of the group at
// the snapshot in sequence order, without gone markers.
func (v *Snapshot) StateRows() ([]json.RawMessage, error) {
	return v.tx.StateRows(v.group)
}

// Pending returns the cards of the group that are still pending at the
// snapshot, oldest first.
func (v *Snapshot) Pending() ([]json.RawMessage, error) {
	return v.tx.Pending(v.group)
}

// DeltaCount returns how many rows of the group have seq in (after, upTo],
// gone markers included.
func (v *Snapshot) DeltaCount(after, upTo int64) (int, error) {
	var n int
	err := v.tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND seq>? AND seq<=?", v.group, after, upTo).Scan(&n)
	return n, err
}

// AfterSeqUpTo returns (after, upTo] in sequence order, including gone markers.
func (v *Snapshot) AfterSeqUpTo(after, upTo int64) ([]json.RawMessage, error) {
	return v.rows("SELECT data FROM row WHERE g=? AND seq>? AND seq<=? ORDER BY seq", v.group, after, upTo)
}

// below is the pos bound of a page: rows with a lower pos, or all rows for
// before 0, which asks for the newest page (no pos is 0).
func below(before int64) int64 {
	if before == 0 {
		return math.MaxInt64
	}
	return before
}

// PageThread reads the main line (nil) or a task thread, newest first.
func (v *Snapshot) PageThread(thread *string, before int64, limit int) ([]json.RawMessage, error) {
	const selectMsg = "SELECT data FROM row WHERE g=? AND r='msg' AND pos<? AND "
	const newest = " ORDER BY pos DESC LIMIT ?"
	if thread == nil {
		return v.rows(selectMsg+"in_main=1"+newest, v.group, below(before), limit)
	}
	return v.rows(selectMsg+"thread=?"+newest, v.group, below(before), *thread, limit)
}

// PageKind reads notes ("note") or the review ledger ("review"), newest first.
func (v *Snapshot) PageKind(kind string, before int64, limit int) ([]json.RawMessage, error) {
	return v.rows(`SELECT data FROM row WHERE g=? AND keep=1
		AND r=CASE ? WHEN 'note' THEN 'note' WHEN 'review' THEN 'msg' END
		AND pos<? ORDER BY pos DESC LIMIT ?`, v.group, kind, below(before), limit)
}

func (v *Snapshot) rows(query string, args ...any) ([]json.RawMessage, error) {
	rows, err := v.tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		var data string
		// A Scan failure surfaces through rows.Err() on the next line, because row is STRICT and the driver reads every column before Next returns true.
		_ = rows.Scan(&data)
		result = append(result, json.RawMessage(data))
	}
	return result, rows.Err()
}
