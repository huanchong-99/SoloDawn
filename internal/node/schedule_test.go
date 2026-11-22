package node

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestRunRetentionStartupRunUsesHomeRetain(t *testing.T) {
	db := newRetentionStore(t)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).Truncate(time.Millisecond)
	seedDefaultRetentionRows(t, db, now)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	runRetention(ctx, db, now, nil)
	verifyDefaultRetentionRows(t, db)
}

func newRetentionStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "node.db"))
	mustNoErr(t, err)
	t.Cleanup(func() { db.Close() })
	return db
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

func TestRunRetentionLogsErrorsAndKeepsRunning(t *testing.T) {
	db := newRetentionStore(t)
	setRetentionRaw(t, db, `"bad"`)
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{}) // single reader: TestRunRetentionLogsErrorsAndKeepsRunning
	go func() {
		runRetention(ctx, db, time.Now(), tick)
		close(done)
	}()
	tick <- time.Now()
	cancel()
	<-done
	if got := strings.Count(buf.String(), "node: retention: "); got != 2 {
		t.Fatalf("logged %d retention errors, want 2: %q", got, buf.String())
	}
}

func TestRunRetentionRunsAtEveryTick(t *testing.T) {
	db := newRetentionStore(t)
	setRetentionRaw(t, db, `7`)
	day := 24 * time.Hour
	base := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	cut1, cut2 := base.Add(-7*day).UnixMilli(), base.Add(-6*day).UnixMilli()
	seed := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO audit(ts, what, detail) VALUES(?, 'delivered', 'first'), (?, 'delivered', 'second'), (?, 'delivered', 'keep')`, cut1-1, cut2-1, cut2)
		return err
	}
	mustNoErr(t, db.Write(seed))

	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{}) // single reader: TestRunRetentionRunsAtEveryTick
	go func() {
		runRetention(ctx, db, base, tick)
		close(done)
	}()
	tick <- base.Add(day)
	cancel()
	<-done
	checkSingleString(t, db, `SELECT detail FROM audit`, "keep")
}

func TestOpenNodeRunsRetentionAtStartup(t *testing.T) {
	dir := seedNodeDB(t, func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT INTO audit(ts, what, detail) VALUES(1, 'delivered', 'startup-aged')`)
		return err
	})
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	n.Close()
	db, err := store.Open(filepath.Join(dir, "node.db"))
	mustNoErr(t, err)
	defer db.Close()
	if got := queryInt(t, db, `SELECT count(*) FROM audit`); got != 0 {
		t.Fatalf("audit rows after start-up = %d, want 0", got)
	}
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
