package store

import "encoding/json"

// StateRows returns the group, member, device and task rows of the group at
// the snapshot in sequence order, without gone markers.
func (v *Snapshot) StateRows() ([]json.RawMessage, error) {
	return v.rows(`SELECT data FROM row WHERE g=? AND gone=0
		AND r IN ('group','member','device','task') ORDER BY seq`, v.group)
}

// Pending returns the cards of the group that are still pending at the
// snapshot, oldest first.
func (v *Snapshot) Pending() ([]json.RawMessage, error) {
	return v.rows("SELECT data FROM row WHERE g=? AND pending=1 ORDER BY pos", v.group)
}

// DeltaCount returns how many rows of the group have seq in (after, upTo],
// gone markers included.
func (v *Snapshot) DeltaCount(after, upTo int64) (int, error) {
	var n int
	err := v.tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND seq>? AND seq<=?", v.group, after, upTo).Scan(&n)
	return n, err
}

// StateAfterSeqUpTo returns the group, member, device and task rows of the
// group in (after, upTo] in sequence order, gone markers included.
func (v *Snapshot) StateAfterSeqUpTo(after, upTo int64) ([]json.RawMessage, error) {
	return v.rows(`SELECT data FROM row WHERE g=? AND seq>? AND seq<=?
		AND r IN ('group','member','device','task') ORDER BY seq`, v.group, after, upTo)
}
