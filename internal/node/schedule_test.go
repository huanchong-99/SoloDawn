package node

import (
	"bytes"
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestRetainOnceDelegatesToHomeRetain(t *testing.T) {
	if retentionEvery != 24*time.Hour {
		t.Fatalf("retentionEvery = %v, want 24h", retentionEvery)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "node.db"))
	mustNoErr(t, err)
	t.Cleanup(func() { db.Close() })

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).Truncate(time.Millisecond)
	seedDefaultRetentionRows(t, db, now)
	mustNoErr(t, retainOnce(db, now))
	verifyDefaultRetentionRows(t, db)

	setRetentionRaw(t, db, `"bad"`)
	if err := retainOnce(db, now); err == nil {
		t.Fatal("expected error for non-integer retention_days")
	}
}

func seedDefaultRetentionRows(t *testing.T, db *store.Store, now time.Time) {
	t.Helper()
	cut90 := now.AddDate(0, 0, -90).UnixMilli()
	rcptCut := now.Add(-24 * time.Hour).UnixMilli()
	nowMS := now.UnixMilli()
	seed := func(tx *store.Tx) error {
		stmts := []struct {
			q    string
			args []any
		}{
			{`INSERT INTO grp(g, seq, num, purged) VALUES('g1', 6, 0, 0)`, nil},
			{`INSERT INTO row(g, r, id, seq, pos, ts, mt, gone, data) VALUES('g1', 'msg', 'm-old', 5, 5, 1, ?, 1, '{}')`, []any{cut90 - 1}},
			{`INSERT INTO row(g, r, id, seq, pos, ts, mt, gone, data) VALUES('g1', 'msg', 'm-keep', 6, 6, 1, ?, 1, '{}')`, []any{cut90}},
			{`INSERT INTO audit(ts, what, detail) VALUES(?, 'delivered', 'old'), (?, 'delivered', 'keep')`, []any{cut90 - 1, cut90}},
			{`INSERT INTO receipt(k, n, ts, answer) VALUES('k', 'n-old', ?, '{}'), ('k', 'n-keep', ?, '{}')`, []any{rcptCut - 1, rcptCut}},
			{`INSERT INTO peer(device, url, ok_at) VALUES('d1', 'ws://old', ?), ('d1', 'ws://keep', ?)`, []any{cut90 - 1, cut90}},
			{`INSERT INTO processed_offer(signer, conn, expires_ms) VALUES('s', 'c-old', ?), ('s', 'c-keep', ?)`, []any{nowMS - 1, nowMS}},
		}
		for _, s := range stmts {
			if _, err := tx.Exec(s.q, s.args...); err != nil {
				return err
			}
		}
		return nil
	}
	mustNoErr(t, db.Write(seed))
}

func verifyDefaultRetentionRows(t *testing.T, db *store.Store) {
	t.Helper()
	if got := queryInt(t, db, `SELECT purged FROM grp WHERE g = 'g1'`); got != 5 {
		t.Fatalf("grp.purged = %d, want 5", got)
	}
	checkSingleString(t, db, `SELECT id FROM row WHERE g = 'g1'`, "m-keep")
	checkSingleString(t, db, `SELECT detail FROM audit`, "keep")
	checkSingleString(t, db, `SELECT n FROM receipt`, "n-keep")
	checkSingleString(t, db, `SELECT url FROM peer`, "ws://keep")
	checkSingleString(t, db, `SELECT conn FROM processed_offer`, "c-keep")
}

func setRetentionRaw(t *testing.T, db *store.Store, raw string) {
	t.Helper()
	put := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('retention_days', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, raw)
		return err
	}
	mustNoErr(t, db.Write(put))
}

func TestRunRetentionScheduleAndNodeStartup(t *testing.T) {
	checkLogRetentionError(t)
	checkScheduleTicksAndErrors(t)
	checkOpenNodeRunsRetention(t)
}

func checkLogRetentionError(t *testing.T) {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)
	logRetentionError(errors.New("disk full"))
	if !strings.Contains(buf.String(), "node: retention: disk full") {
		t.Fatalf("log output = %q", buf.String())
	}
}

func checkScheduleTicksAndErrors(t *testing.T) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "node.db"))
	mustNoErr(t, err)
	t.Cleanup(func() { db.Close() })

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	day1Cut := base.AddDate(0, 0, -7).UnixMilli()
	day2Cut := base.AddDate(0, 0, -6).UnixMilli()
	seed := func(tx *store.Tx) error {
		if _, txErr := tx.Exec(`INSERT INTO setting(key, value) VALUES('retention_days', '"bad"')`); txErr != nil {
			return txErr
		}
		_, txErr := tx.Exec(`INSERT INTO audit(ts, what, detail) VALUES(?, 'delivered', 'first'), (?, 'delivered', 'second')`, day1Cut-1, day2Cut-1)
		return txErr
	}
	mustNoErr(t, db.Write(seed))

	var mu sync.Mutex
	cur := base
	var reported []error
	nowFn := func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	reportFn := func(e error) { mu.Lock(); reported = append(reported, e); mu.Unlock() }

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{}) // single reader: checkScheduleTicksAndErrors
	go func() {
		runRetention(ctx, db, 5*time.Millisecond, nowFn, reportFn)
		close(done)
	}()

	waitCond(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(reported) >= 1 })
	setRetentionRaw(t, db, `7`)
	waitCond(t, func() bool { return queryInt(t, db, `SELECT count(*) FROM audit`) == 1 })
	checkSingleString(t, db, `SELECT detail FROM audit`, "second")

	mu.Lock()
	cur = base.Add(24 * time.Hour)
	mu.Unlock()
	waitCond(t, func() bool { return queryInt(t, db, `SELECT count(*) FROM audit`) == 0 })
	cancel()
	<-done
}

func checkOpenNodeRunsRetention(t *testing.T) {
	t.Helper()
	dir := seedNodeDB(t, func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO audit(ts, what, detail) VALUES(1, 'delivered', 'startup-aged')`)
		return err
	})
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	waitCond(t, func() bool { return queryInt(t, n.db, `SELECT count(*) FROM audit`) == 0 })
	n.Close()
}

func queryInt(t *testing.T, db *store.Store, q string) int {
	t.Helper()
	var n int
	read := func(tx *store.Tx) error { return tx.QueryRow(q).Scan(&n) }
	mustNoErr(t, db.Write(read))
	return n
}

func checkSingleString(t *testing.T, db *store.Store, q, want string) {
	t.Helper()
	var vals []string
	read := func(tx *store.Tx) error {
		rows, err := tx.Query(q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			vals = append(vals, s)
		}
		return rows.Err()
	}
	mustNoErr(t, db.Write(read))
	if len(vals) != 1 || vals[0] != want {
		t.Fatalf("%s = %v, want [%s]", q, vals, want)
	}
}
