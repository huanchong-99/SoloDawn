package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

func openDB(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	check(t, err)
	t.Cleanup(func() { check(t, s.Close()) })
	equal(t, s.db.Stats().InUse, 0)
	return s
}

func scalar[T any](t *testing.T, db *sql.DB, query string, args ...any) T {
	t.Helper()
	var v T
	check(t, db.QueryRow(query, args...).Scan(&v))
	return v
}

func TestSchemaAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	s, err := Open(path)
	check(t, err)
	equal(t, scalar[string](t, s.db, "PRAGMA journal_mode"), "wal")
	equal(t, s.db.Stats().MaxOpenConnections, 0)
	equal(t, scalar[int](t, s.db, "PRAGMA user_version"), 2)
	equal(t, scalar[string](t, s.db, `SELECT group_concat(name,',' ORDER BY name) FROM pragma_table_list
		WHERE name NOT LIKE 'sqlite_%' AND strict=1`), "agent,approval,audit,device_log,grp,inbox,peer,perm,processed_offer,push_sub,receipt,row,session,setting,usage")
	equal(t, scalar[string](t, s.db, `SELECT group_concat(name,',' ORDER BY name) FROM sqlite_schema
		WHERE type='index' AND name NOT LIKE 'sqlite_%'`), "audit_by_ts,audit_item,inbox_by_host,row_by_seq,row_by_thread,row_keep,row_main,row_pending")
	_, err = s.db.Exec("INSERT INTO setting VALUES('name','test')")
	check(t, err)
	check(t, s.Close())
	s = openDB(t, path)
	equal(t, scalar[string](t, s.db, "SELECT value FROM setting WHERE key='name'"), "test")
}

func TestMemoryStoreUsesOneConnection(t *testing.T) {
	s := openDB(t, ":memory:")
	equal(t, s.db.Stats().MaxOpenConnections, 1)
}

func TestClosedDBMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	check(t, err)
	check(t, db.Close())
	equal(t, migrate(db, []string{schema}).Error(), "sql: database is closed")
}

func TestMigrationsAreOrderedAndAtomic(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	check(t, err)
	defer db.Close()
	steps := []string{"CREATE TABLE m(n INTEGER)", "INSERT INTO m VALUES(2)", "INSERT INTO m VALUES(3); broken"}
	check(t, migrate(db, steps[:1]))
	err = migrate(db, steps)
	if err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("bad migration: %v", err)
	}
	equal(t, scalar[int](t, db, "PRAGMA user_version"), 2)
	equal(t, scalar[string](t, db, "SELECT group_concat(n) FROM m"), "2")
	_, err = db.Exec("PRAGMA user_version=4")
	check(t, err)
	equal(t, migrate(db, steps).Error(), "unsupported store schema version 4")
}

func TestOpenFailure(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "absent", "node.db"))
	equal(t, s, (*Store)(nil))
	if err == nil || !strings.Contains(err.Error(), "unable to open database") {
		t.Fatalf("open: %v", err)
	}
}

func TestCountersAndRollback(t *testing.T) {
	s := openDB(t, ":memory:")
	check(t, s.Write(func(tx *Tx) error {
		for _, step := range []struct {
			f    func(string) (int64, error)
			g    string
			want int64
		}{
			{tx.NextNum, "a", 1}, {tx.NextSeq, "a", 1}, {tx.NextSeq, "a", 2}, {tx.NextNum, "a", 2},
			{tx.NextSeq, "b", 1}, {tx.NextSeq, "b", 2}, {tx.NextNum, "b", 1},
		} {
			got, err := step.f(step.g)
			if err != nil {
				return err
			}
			if got != step.want {
				return fmt.Errorf("counter %s: got %d; want %d", step.g, got, step.want)
			}
		}
		return nil
	}))
	stop := errors.New("command rejected")
	err := s.Write(func(tx *Tx) error {
		if _, err := tx.NextSeq("a"); err != nil {
			return err
		}
		if err := tx.PutReceipt("key", "nonce", 2, Outcome{OK: true}); err != nil {
			return err
		}
		return stop
	})
	equal(t, err, stop)
	equal(t, scalar[string](t, s.db, "SELECT group_concat(g||':'||seq||':'||num,',' ORDER BY g) FROM grp"), "a:2:2,b:2:1")
	equal(t, scalar[int](t, s.db, "SELECT count(*) FROM receipt"), 0)
}

func lockDB(t *testing.T, path string) func() {
	t.Helper()
	lock, err := sql.Open("sqlite", path)
	check(t, err)
	_, err = lock.Exec("BEGIN IMMEDIATE")
	check(t, err)
	return func() {
		t.Helper()
		_, err := lock.Exec("ROLLBACK")
		check(t, err)
		check(t, lock.Close())
	}
}

func TestWriteBeginFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	s := openDB(t, path+"?_txlock=immediate")
	unlock := lockDB(t, path)
	ran := false
	err := s.Write(func(*Tx) error {
		ran = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("locked write begin: %v", err)
	}
	equal(t, ran, false)
	unlock()
}
