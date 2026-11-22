package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func kbNote(group, id string, seq, pos int64, noteType, name, text string) proto.NoteRow {
	return proto.NoteRow{
		RowHeader: header("note", id, seq),
		Pos:       pos,
		Group:     group,
		NoteType:  noteType,
		Name:      name,
		Text:      text,
		By:        "m1",
		Timestamp: 100,
	}
}

func kbIDs(t *testing.T, v *Snapshot, q string, limit int) []string {
	t.Helper()
	rows, err := v.SearchKB(q, limit)
	return ids(t, rows, err)
}

func pageIDs(t *testing.T, v *Snapshot, kind string, before int64, limit int) []string {
	t.Helper()
	rows, err := v.PageKind(kind, before, limit)
	return ids(t, rows, err)
}

func queryPlan(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var id, parent, notused int
	var detail string
	check(t, db.QueryRow(query, args...).Scan(&id, &parent, &notused, &detail))
	return detail
}

func TestMigrateKBBackfillsLiveDocumentsAndRollsBackOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.db")
	db, err := sql.Open("sqlite", path)
	check(t, err)
	check(t, migrate(db, []string{schema, processedOffers}))
	check(t, transaction(db, func(tx *Tx) error {
		for i, r := range []struct {
			g, id, gone, data string
		}{
			{"g1", "k1", "0", `{"r":"note","id":"k1","seq":1,"pos":1,"g":"g1","task":null,"type":"kb","name":"Decision: single writer","text":"连接路径事件","by":"m1","ts":10}`},
			{"g2", "k2", "0", `{"r":"note","id":"k2","seq":2,"pos":1,"g":"g2","task":null,"type":"kb","name":"Other group","text":"连接路径事件","by":"m2","ts":10}`},
			{"g1", "k3", "1", `{"r":"note","id":"k3","seq":3,"gone":true}`},
			{"g1", "p1", "0", `{"r":"note","id":"p1","seq":4,"pos":3,"g":"g1","task":null,"type":"preset","name":"Calm","text":"single writer","by":"m1","ts":10}`},
			{"g1", "n1", "0", `{"r":"note","id":"n1","seq":5,"pos":4,"g":"g1","task":null,"text":"single writer","by":"m1","ts":10}`},
		} {
			if _, err := tx.Exec(`INSERT INTO row(g,r,id,seq,pos,ts,mt,keep,gone,data) VALUES(?,'note',?,?,1,10,10,1,?,?)`, r.g, r.id, i+1, r.gone, r.data); err != nil {
				return err
			}
		}
		return nil
	}))
	check(t, db.Close())

	s := openDB(t, path)
	equal(t, scalar[int](t, s.db, "PRAGMA user_version"), 3)
	v1 := snapshot(t, s, "g1")
	equal(t, kbIDs(t, v1, "路径", 10), []string{"k1"})
	equal(t, kbIDs(t, v1, "writer", 10), []string{"k1"})
	v2 := snapshot(t, s, "g2")
	equal(t, kbIDs(t, v2, "路径", 10), []string{"k2"})
}

func TestMigrateKBAndSyncErrorBranches(t *testing.T) {
	bad, err := sql.Open("sqlite", ":memory:")
	check(t, err)
	defer bad.Close()
	check(t, migrate(bad, []string{schema, processedOffers}))
	_, err = bad.Exec(`INSERT INTO row(g,r,id,seq,pos,ts,mt,keep,gone,data)
		VALUES('g','note','k1',1,1,1,1,1,0,'{"type":"kb","name":null,"text":"x"}')`)
	check(t, err)
	if err := migrate(bad, []string{schema, processedOffers, kbSchema}); err == nil {
		t.Fatal("expected scan error during kb backfill")
	}
	equal(t, scalar[int](t, bad, "PRAGMA user_version"), 2)

	s := openDB(t, ":memory:")
	put(t, s, "g", 10, kbNote("g", "k1", 1, 1, "kb", "Doc", "body"))
	check(t, s.Write(func(tx *Tx) error {
		_, err := tx.Exec("DROP TABLE kb")
		return err
	}))
	if err := s.Write(func(tx *Tx) error { return tx.backfillKB() }); err == nil || !strings.Contains(err.Error(), "no such table: kb") {
		t.Fatalf("backfill without kb table: %v", err)
	}
	if err := s.Write(func(tx *Tx) error { return Put(tx, "g", 20, kbNote("g", "k2", 2, 2, "kb", "Doc2", "body")) }); err == nil {
		t.Fatal("expected Put error without kb table")
	}
	if err := s.Write(func(tx *Tx) error {
		return Put(tx, "g", 20, proto.GoneRow{RowHeader: header("note", "k1", 2), Gone: true})
	}); err == nil {
		t.Fatal("expected GoneRow Put error without kb table")
	}
}

func TestKBVerifiedMatchingRankingCJKAndRetention(t *testing.T) {
	s := openDB(t, ":memory:")
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -30).UnixMilli()

	docs := []proto.NoteRow{
		kbNote("g", "d-both", 1, 1, "kb", "Retention policy", "The database retention rule keeps notes."),
		kbNote("g", "d-rare", 2, 2, "kb", "Purge", "Old messages leave by retention."),
		kbNote("g", "d-common1", 3, 3, "kb", "Schema", "One SQLite database per node."),
		kbNote("g", "d-common2", 4, 4, "kb", "WAL", "The database uses WAL mode."),
		kbNote("g", "d-accent", 5, 5, "kb", "Hiring", "Send a RÉSUMÉ to the café."),
		kbNote("g", "d-path", 6, 6, "kb", "Network", "Selected path and fallback paths."),
		kbNote("g", "d-onlypaths", 7, 7, "kb", "Routes", "Multiple paths exist."),
		kbNote("g", "d-punct", 8, 8, "kb", "Metrics", "Report rtt_ms on switch."),
		kbNote("g", "d-punct-rev", 9, 9, "kb", "Other", "Field ms_rtt is unused; rtt alone."),
		kbNote("g", "d-cjk", 10, 10, "kb", "路径规则", "连接路径事件与せつぞくしょり、ネットワークイベント、연결경로이벤트"),
		kbNote("g", "d-ops", 11, 11, "kb", "Plain title", `Text has a"b and literal NOT and name:star.`),
		kbNote("g", "p-preset", 12, 12, "preset", "Retention database", "Preset text is not indexed in kb."),
		kbNote("other", "d-other", 1, 1, "kb", "Isolation", "retention database 连接路径事件"),
	}
	for _, d := range docs {
		d.Timestamp = old
		put(t, s, d.Group, old, d)
	}

	v, err := s.Snapshot("g")
	check(t, err)
	// 1. Any term matches name or text; more and rarer matches rank higher; presets and other groups excluded.
	equal(t, kbIDs(t, v, "retention database", 10), []string{"d-both", "d-rare", "d-common1", "d-common2"})
	equal(t, kbIDs(t, v, "retention database", 2), []string{"d-both", "d-rare"})
	// 2. Letter case and accents do not matter.
	equal(t, kbIDs(t, v, "resume", 10), []string{"d-accent"})
	equal(t, kbIDs(t, v, "CAFE", 10), []string{"d-accent"})
	// 3. Whole words in order: path does not find d-onlypaths ("paths").
	equal(t, kbIDs(t, v, "path", 10), []string{"d-path"})
	// 4. Punctuation inside a term separates words in order.
	equal(t, kbIDs(t, v, "rtt_ms", 10), []string{"d-punct"})
	// 5. Han, Hiragana, Katakana and Hangul runs (including single-character queries).
	for _, q := range []string{"路径", "连", "规则", "せつぞく", "ぞ", "イベント", "ベ", "경로", "연"} {
		equal(t, kbIDs(t, v, q, 10), []string{"d-cjk"})
	}
	// 6. AND, NOT, *, name: and quotes are text, not operators.
	equal(t, kbIDs(t, v, "Hiring AND", 10), []string{"d-accent", "d-ops", "d-path"})
	equal(t, kbIDs(t, v, "NOT", 10), []string{"d-ops"})
	equal(t, kbIDs(t, v, "path*", 10), []string{"d-path"})
	equal(t, kbIDs(t, v, "name:star", 10), []string{"d-ops"})
	equal(t, kbIDs(t, v, "name: star", 10), []string{"d-ops"})
	equal(t, kbIDs(t, v, `a"b`, 10), []string{"d-ops"})
	// 7. Punctuation-only queries match nothing without error (the home refuses a blank q).
	for _, q := range []string{"---", "... //", `""`} {
		equal(t, kbIDs(t, v, q, 10), []string{})
	}
	check(t, v.Close())

	// Replace updates the index in one transaction; forget removes it; retention never touches live kb docs.
	replaced := kbNote("g", "d-accent", 13, 5, "kb", "Hiring", "Updated curriculum vitae only.")
	replaced.Timestamp = old
	put(t, s, "g", old, replaced)
	put(t, s, "g", old, proto.GoneRow{RowHeader: header("note", "d-punct", 14), Gone: true})
	put(t, s, "g", old, proto.GoneRow{RowHeader: header("msg", "d-cjk", 15), Gone: true})
	check(t, s.Write(func(tx *Tx) error { return tx.Retain(now, 7) }))

	after, err := s.Snapshot("g")
	check(t, err)
	equal(t, kbIDs(t, after, "resume", 10), []string{})
	equal(t, kbIDs(t, after, "curriculum", 10), []string{"d-accent"})
	equal(t, kbIDs(t, after, "rtt_ms", 10), []string{})
	equal(t, kbIDs(t, after, "路径", 10), []string{"d-cjk"})
	check(t, after.Close())

	rows, err := after.SearchKB("路径", 10)
	equal(t, []any{rows, err}, []any{[]json.RawMessage(nil), sql.ErrTxDone})
}
