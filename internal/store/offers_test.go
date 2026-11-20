package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateOffersAndExpireStrictlyAfterWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	db, err := sql.Open("sqlite", path)
	check(t, err)
	check(t, migrate(db, []string{schema}))
	check(t, db.Close())
	s := openDB(t, path)
	for _, r := range []struct {
		key, conn string
		first     bool
	}{{"a", "1", true}, {"a", "2", true}, {"b", "1", true}, {"a", "1", false}} {
		first, err := s.RememberOffer(r.key, r.conn, 220000, 100000)
		check(t, err)
		equal(t, first, r.first)
	}
	for _, tc := range []struct {
		now   int64
		found bool
	}{{339999, true}, {340000, true}, {340001, false}} {
		found, err := s.HasOffer("a", "1", tc.now)
		check(t, err)
		equal(t, found, tc.found)
		check(t, s.Write(func(tx *Tx) error { return tx.Retain(time.UnixMilli(tc.now), 90) }))
		n := scalar[int](t, s.db, "SELECT COUNT(*) FROM processed_offer")
		if tc.found {
			equal(t, n, 3)
		} else {
			equal(t, n, 0)
		}
	}
}

func TestRememberOfferFailures(t *testing.T) {
	s := openDB(t, ":memory:")
	_, err := s.db.Exec("CREATE TRIGGER refuse BEFORE INSERT ON processed_offer BEGIN SELECT RAISE(ABORT, 'refused'); END")
	check(t, err)
	first, err := s.RememberOffer("a", "1", 0, 0)
	equal(t, first, false)
	equal(t, err.Error(), "constraint failed: refused (1811)")
	_, err = s.db.Exec("DROP TABLE processed_offer")
	check(t, err)
	_, err = s.RememberOffer("a", "1", 0, 0)
	equal(t, err.Error(), "SQL logic error: no such table: processed_offer (1)")
}
