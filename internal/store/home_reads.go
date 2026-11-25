package store

import (
	"database/sql"
	"encoding/json"
)

// What the run loop of a home (internal/home/run.go) reads and writes in a
// transaction of the writer: the groups and their state rows at the start,
// and what a rule of the home reads from disk while its transaction is open.
// A read here sees what the same transaction has written.

// Groups returns the ids of the groups that have a group row, in order.
func (tx *Tx) Groups() ([]string, error) {
	rows, err := tx.Query("SELECT g FROM row WHERE r='group' AND gone=0 ORDER BY g")
	return collect(rows, err, scanOne[string])
}

// Counters returns the last sequence number and the last task number given
// out in a group; both are 0 for a group that has neither.
func (tx *Tx) Counters(group string) (seq, num int64, err error) {
	err = tx.QueryRow("SELECT coalesce(max(seq),0), coalesce(max(num),0) FROM grp WHERE g=?", group).Scan(&seq, &num)
	return seq, num, err
}

// RaiseNum moves the last task number of a group up to num, and never down.
func (tx *Tx) RaiseNum(group string, num int) error {
	_, err := tx.Exec("UPDATE grp SET num=max(num, ?) WHERE g=?", num, group)
	return err
}

// StateRows returns the group, member, device and task rows of a group in
// the order of their sequence numbers, without markers.
func (tx *Tx) StateRows(group string) ([]json.RawMessage, error) {
	return tx.data(`SELECT data FROM row WHERE g=? AND gone=0
		AND r IN ('group','member','device','task') ORDER BY seq`, group)
}

// LackingEntries returns the stored device-log entries of the group's people
// that have no device row in the group, by person and version.
func (tx *Tx) LackingEntries(group string) ([]json.RawMessage, error) {
	return tx.data(`SELECT env FROM device_log AS l WHERE user IN (SELECT data->>'user' FROM row
		WHERE g=?1 AND r='member' AND gone=0 AND data->>'kind'='human')
		AND NOT EXISTS (SELECT 1 FROM row WHERE g=?1 AND r='device' AND gone=0 AND id=l.user||':'||l.ver)
		ORDER BY user, ver`, group)
}

// Row returns one row of a group, or sql.ErrNoRows when there is none or
// only its marker.
func (tx *Tx) Row(group, r, id string) (json.RawMessage, error) {
	var data string
	err := tx.QueryRow("SELECT data FROM row WHERE g=? AND r=? AND id=? AND gone=0", group, r, id).Scan(&data)
	return json.RawMessage(data), err
}

// TypedNotes returns the live notes of a type in a group, sorted by name.
func (tx *Tx) TypedNotes(group, noteType string) ([]json.RawMessage, error) {
	return tx.data(`SELECT data FROM row WHERE g=? AND r='note' AND gone=0
		AND data->>'type' IS NOT NULL AND data->>'type'=? ORDER BY data->>'name'`, group, noteType)
}

// Thread returns a page of the main line (nil) or of a task's thread, newest
// first: the messages below pos before, and the newest ones for before 0.
func (tx *Tx) Thread(group string, thread *string, before int64, limit int) ([]json.RawMessage, error) {
	if thread == nil {
		return tx.data(`SELECT data FROM row WHERE g=? AND r='msg' AND in_main=1 AND pos<?
			ORDER BY pos DESC LIMIT ?`, group, below(before), limit)
	}
	return tx.data(`SELECT data FROM row WHERE g=? AND r='msg' AND thread=? AND pos<?
		ORDER BY pos DESC LIMIT ?`, group, *thread, below(before), limit)
}

// Pending returns the cards of a group that are still pending, oldest first.
func (tx *Tx) Pending(group string) ([]json.RawMessage, error) {
	return tx.data("SELECT data FROM row WHERE g=? AND pending=1 ORDER BY pos", group)
}

// Queued returns the inbox items of a group that no host has acknowledged,
// oldest first, those held back by not_before included.
func (tx *Tx) Queued(group string) ([]Item, error) {
	rows, err := tx.Query(selectInbox+"WHERE g=? ORDER BY id", group)
	return collect(rows, err, scanItem)
}

// Retire deletes a queued item of a group that a later change made
// pointless. An item that is gone already is no error.
func (tx *Tx) Retire(group string, id int64) error {
	_, err := tx.Exec("DELETE FROM inbox WHERE g=? AND id=?", group, id)
	return err
}

// Jump has a queued item of a group jump the queue under its number, held back
// no longer. An item that is gone already is no error.
func (tx *Tx) Jump(group string, id int64) error {
	_, err := tx.Exec("UPDATE inbox SET env=json_set(env,'$.jump',json('true')), not_before=0 WHERE g=? AND id=?", group, id)
	return err
}

// data reads the one JSON column of a query on the row table.
func (tx *Tx) data(query string, args ...any) ([]json.RawMessage, error) {
	rows, err := tx.Query(query, args...)
	return collect(rows, err, func(rows *sql.Rows) (json.RawMessage, error) {
		data, err := scanOne[string](rows)
		return json.RawMessage(data), err
	})
}

// scanOne reads a result that has one column.
func scanOne[T any](rows *sql.Rows) (v T, err error) {
	err = rows.Scan(&v)
	return v, err
}

// collect reads every line of a result with scan, or returns the error of
// the query that made it. A list without lines is empty, not nil.
func collect[T any](rows *sql.Rows, err error, scan func(*sql.Rows) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
