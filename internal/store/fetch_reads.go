package store

import "encoding/json"

// Row returns one live row of the snapshot's group by its type and id, or
// sql.ErrNoRows when no such row exists or only its gone marker remains.
func (v *Snapshot) Row(r, id string) (json.RawMessage, error) {
	var raw string
	err := v.tx.QueryRow("SELECT data FROM row WHERE g=? AND r=? AND id=? AND gone=0", v.group, r, id).Scan(&raw)
	return json.RawMessage(raw), err
}
