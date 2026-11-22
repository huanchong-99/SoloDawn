package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

const kbSchema = `CREATE VIRTUAL TABLE kb USING fts5(
  g UNINDEXED, id UNINDEXED, name, text, tokenize = 'unicode61'
);
CREATE UNIQUE INDEX row_named ON row (g, data->>'type', data->>'name')
  WHERE r = 'note' AND gone = 0 AND data->>'type' IS NOT NULL;`

type kbEntry struct {
	g, id, name, text string
}

func afterMigration(tx *Tx, migration string) error {
	if migration != kbSchema {
		return nil
	}
	return tx.backfillKB()
}

func (tx *Tx) backfillKB() error {
	rows, err := tx.Query(`SELECT g, id, data->>'name', data->>'text' FROM row
		WHERE r='note' AND gone=0 AND data->>'type'='kb'`)
	items, err := collect(rows, err, scanKBEntry)
	if err != nil {
		return err
	}
	for _, e := range items {
		if err := insertKB(tx, e.g, e.id, e.name, e.text); err != nil {
			return err
		}
	}
	return nil
}

func scanKBEntry(rows *sql.Rows) (kbEntry, error) {
	var e kbEntry
	err := rows.Scan(&e.g, &e.id, &e.name, &e.text)
	return e, err
}

func syncKB(tx *Tx, group string, row any) error {
	switch r := row.(type) {
	case proto.NoteRow:
		return putKBNote(tx, group, r)
	case proto.GoneRow:
		if r.Type == "note" {
			return deleteKB(tx, group, r.ID)
		}
	}
	return nil
}

func putKBNote(tx *Tx, group string, r proto.NoteRow) error {
	if r.NoteType != "kb" {
		return nil
	}
	if err := deleteKB(tx, group, r.ID); err != nil {
		return err
	}
	return insertKB(tx, group, r.ID, r.Name, r.Text)
}

func insertKB(tx *Tx, group, id, name, text string) error {
	_, err := tx.Exec("INSERT INTO kb(g, id, name, text) VALUES (?, ?, ?, ?)",
		group, id, spreadCJK(name), spreadCJK(text))
	return err
}

func deleteKB(tx *Tx, group, id string) error {
	_, err := tx.Exec("DELETE FROM kb WHERE g=? AND id=?", group, id)
	return err
}

// NamedNote returns the live note of group with noteType and name, or
// sql.ErrNoRows when none exists.
func (tx *Tx) NamedNote(group, noteType, name string) (json.RawMessage, error) {
	var data string
	err := tx.QueryRow(`SELECT data FROM row
		WHERE g=? AND r='note' AND gone=0 AND data->>'type' IS NOT NULL
		  AND data->>'type'=? AND data->>'name'=?`, group, noteType, name).Scan(&data)
	return json.RawMessage(data), err
}

// SearchKB searches the group's knowledge-base documents by query, best match
// first (planning/contracts/step-3/knowledge.md section 6), returning at most
// limit stored note rows.
func (v *Snapshot) SearchKB(query string, limit int) ([]json.RawMessage, error) {
	fts := buildKBQuery(query)
	if fts == "" {
		return []json.RawMessage{}, nil
	}
	return v.rows(`SELECT (SELECT data FROM row WHERE g=?2 AND r='note' AND id=kb.id)
		FROM kb WHERE kb MATCH ?1 AND g=?2 ORDER BY bm25(kb) LIMIT ?3`, fts, v.group, limit)
}

func spreadCJK(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			b.WriteByte(' ')
			b.WriteRune(r)
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func buildKBQuery(q string) string {
	terms := strings.Fields(q)
	phrases := make([]string, len(terms))
	for i, term := range terms {
		phrases[i] = `"` + strings.ReplaceAll(spreadCJK(term), `"`, `""`) + `"`
	}
	return strings.Join(phrases, " OR ")
}
