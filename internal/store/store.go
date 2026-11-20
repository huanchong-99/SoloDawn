// Package store persists node data in SQLite. Writes run on one goroutine;
// a sync reader keeps its own transaction pinned to the writer's cut.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type Store struct {
	db   *sql.DB
	jobs chan func()
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		// Each connection to :memory: would otherwise see a different database.
		db.SetMaxOpenConns(1)
	}
	if err := initialize(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, jobs: make(chan func())}
	go func() {
		for job := range s.jobs {
			job()
		}
	}()
	return s, nil
}

func initialize(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}
	return migrate(db, []string{schema})
}

func migrate(db *sql.DB, migrations []string) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version < 0 || version > len(migrations) {
		return fmt.Errorf("unsupported store schema version %d", version)
	}
	for _, migration := range migrations[version:] {
		version++
		err := transaction(db, func(tx *Tx) error {
			_, err := tx.Exec(migration + fmt.Sprintf("; PRAGMA user_version=%d", version))
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Close is called after all callers have finished with the store.
func (s *Store) Close() error {
	close(s.jobs)
	return s.db.Close()
}

type Tx struct{ *sql.Tx }

func transaction(db *sql.DB, run func(*Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := run(&Tx{tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) onWriter(run func() error) error {
	done := make(chan error, 1)
	s.jobs <- func() { done <- run() }
	return <-done
}

// Write commits all the effects of one command, or none if run returns an error.
func (s *Store) Write(run func(*Tx) error) error {
	return s.onWriter(func() error { return transaction(s.db, run) })
}

func (tx *Tx) NextSeq(group string) (int64, error) {
	return tx.counter(`INSERT INTO grp(g, seq, num) VALUES (?, 1, 0)
		ON CONFLICT(g) DO UPDATE SET seq=seq+1 RETURNING seq`, group)
}

func (tx *Tx) NextNum(group string) (int64, error) {
	return tx.counter(`INSERT INTO grp(g, seq, num) VALUES (?, 0, 1)
		ON CONFLICT(g) DO UPDATE SET num=num+1 RETURNING num`, group)
}

func (tx *Tx) counter(query, group string) (int64, error) {
	var n int64
	err := tx.QueryRow(query, group).Scan(&n)
	return n, err
}
