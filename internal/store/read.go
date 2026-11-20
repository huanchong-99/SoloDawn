package store

import (
	"database/sql"
	"encoding/json"
)

// Snapshot is a read transaction at Seq. Close it when done reading.
type Snapshot struct {
	tx          *sql.Tx
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
		v.tx = tx
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

// AfterSeqUpTo returns (after, upTo] in sequence order, including gone markers.
func (v *Snapshot) AfterSeqUpTo(after, upTo int64) ([]json.RawMessage, error) {
	return v.rows("SELECT data FROM row WHERE g=? AND seq>? AND seq<=? ORDER BY seq", v.group, after, upTo)
}

func (v *Snapshot) PageThread(thread *string, before int64, limit int) ([]json.RawMessage, error) {
	const selectMsg = "SELECT data FROM row WHERE g=? AND r='msg' AND gone=0 AND pos<? AND "
	const newest = " ORDER BY pos DESC LIMIT ?"
	if thread == nil {
		return v.rows(selectMsg+"in_main=1"+newest, v.group, before, limit)
	}
	return v.rows(selectMsg+"thread=?"+newest, v.group, before, *thread, limit)
}

// PageKind reads notes ("note") or the review ledger ("review").
func (v *Snapshot) PageKind(kind string, before int64, limit int) ([]json.RawMessage, error) {
	return v.rows(`SELECT data FROM row WHERE g=? AND keep=1 AND pos<?
		AND (CASE r WHEN 'note' THEN 'note' ELSE 'review' END)=?
		ORDER BY pos DESC LIMIT ?`, v.group, before, kind, limit)
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
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		result = append(result, json.RawMessage(data))
	}
	return result, rows.Err()
}
