package home_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func TestKBNoteByAnAI(t *testing.T) {
	s := noteState(t)
	d := noteDisk{msgs: map[string]proto.MsgRow{}, notes: map[string]proto.NoteRow{}}
	cmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"as":%q,"id":%q,"task":null,"type":"kb","name":"Storage","text":"First body"}`,
		id(142), noteTS, groupID, noteWorker, id(143))
	res := noteApply(t, s, d, receipts{}, laptop, cmd)
	f := false
	equal(t, res.Answer, store.Outcome{OK: true, ID: id(143), Replaced: &f})
	equal(t, []any{res.Rows[0].Value()}, []any{proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(143), Seq: 11}, Pos: 11, Group: groupID,
		NoteType: "kb", Name: "Storage", Text: "First body", By: noteWorker, Timestamp: noteTS,
	}})
}

func fetKBSearch(id int64, group, q string, before int64, limit int) home.Fetch {
	return home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group, Before: before, Limit: limit},
		Kind:      "kb",
		Query:     &q,
	}
}

func kbTables() home.Tables {
	tables := runTables()
	tables.Commands["note"] = home.Commands["note"]
	tables.Commands["forget"] = home.Commands["forget"]
	return tables
}

func kbPut(p *runPeer, id, name, text, wantID string, wantReplaced bool) {
	p.w.t.Helper()
	re := p.send(groupID, "note", fmt.Sprintf(`,"id":%q,"task":null,"type":"kb","name":%q,"text":%q`, id, name, text))
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true,"id":%q,"replaced":%t}`, re, wantID, wantReplaced))
}

func TestKBLoopPutReplaceForgetAndFetchSearch(t *testing.T) {
	w, p := runFounded(t, kbTables())

	// Context note vs KB documents.
	p.does(groupID, "note", fmt.Sprintf(`,"id":%q,"task":null,"text":"context only writer"`, id(160)))
	kbPut(p, id(161), "Storage", "SQLite WAL single writer", id(161), false)
	kbPut(p, id(162), "路径设计", "连接路径事件 single", id(162), false)

	// Replace first KB document: id(161) keeps pos < id(162)'s pos, while text updates.
	kbPut(p, id(163), "Storage", "single writer single writer", id(161), true)

	// kind: note excludes kb; kind: kb without q pages by pos newest first.
	ids, more := fetParsed(t, 1, fetCall(p, fetKind(1, groupID, "note", 0, 10)))
	equal(t, []any{ids, more}, []any{[]string{id(160)}, false})

	ids, more = fetParsed(t, 2, fetCall(p, fetKind(2, groupID, "kb", 0, 10)))
	equal(t, []any{ids, more}, []any{[]string{id(162), id(161)}, false})

	// Search ranks more/rarer matches first, supports CJK, limits with more: true, and returns [] on miss.
	ids, more = fetParsed(t, 3, fetCall(p, fetKBSearch(3, groupID, "single writer", 0, 10)))
	equal(t, []any{ids, more}, []any{[]string{id(161), id(162)}, false})

	ids, more = fetParsed(t, 4, fetCall(p, fetKBSearch(4, groupID, "single", 0, 1)))
	equal(t, []any{ids, more}, []any{[]string{id(161)}, true})

	ids, more = fetParsed(t, 5, fetCall(p, fetKBSearch(5, groupID, "路径", 0, 5)))
	equal(t, []any{ids, more}, []any{[]string{id(162)}, false})

	ids, more = fetParsed(t, 6, fetCall(p, fetKBSearch(6, groupID, "nonexistent", 0, 5)))
	equal(t, []any{ids, more}, []any{[]string{}, false})

	// Forget removes from index and frees the name.
	p.does(groupID, "forget", fmt.Sprintf(`,"r":"note","id":%q`, id(161)))
	ids, more = fetParsed(t, 7, fetCall(p, fetKBSearch(7, groupID, "writer", 0, 5)))
	equal(t, []any{ids, more}, []any{[]string{}, false})

	kbPut(p, id(164), "Storage", "reborn writer", id(164), false)
	ids, more = fetParsed(t, 8, fetCall(p, fetKBSearch(8, groupID, "reborn", 0, 5)))
	equal(t, []any{ids, more}, []any{[]string{id(164)}, false})

	// 60 KiB packing on search results: four ~15.9 KiB documents match, best 3 fit with more: true.
	bigBody := "sharedtoken " + strings.Repeat("b", 15_900)
	for i := range 4 {
		docID := id(byte(170 + i))
		kbPut(p, docID, fmt.Sprintf("Big %d", i), bigBody, docID, false)
	}
	gotBig := fetCall(p, fetKBSearch(9, groupID, "sharedtoken", 0, 10))
	if len(gotBig) > 61_440 {
		t.Fatalf("search frame %d exceeds 61,440", len(gotBig))
	}
	ids, more = fetParsed(t, 9, gotBig)
	equal(t, []any{len(ids), more}, []any{3, true})

	// Invalid search requests answer bad_request.
	empty, spaces, valid := "", " \t\n ", "ok"
	for i, bad := range []home.Fetch{
		fetKBSearch(20, groupID, empty, 0, 5),
		fetKBSearch(21, groupID, spaces, 0, 5),
		fetKBSearch(22, groupID, valid, 1, 5),
		fetKBSearch(23, groupID, valid, -1, 5),
		fetKBSearch(24, groupID, valid, 0, 0),
		fetKBSearch(25, groupID, valid, 0, 101),
		{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 26}, Group: groupID, Limit: 5}, Kind: "note", Query: &valid},
		{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 27}, Group: groupID, Limit: 5}, Kind: "preset", Query: &valid},
		{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 28}, Group: groupID, Limit: 1}, Row: "note", RowID: id(160), Query: &valid},
	} {
		fetError(t, int64(20+i), fetCall(p, bad), "bad_request")
	}

	// Store failure on search answers internal.
	runExec(t, w.db, "DROP TABLE kb")
	fetError(t, 40, fetCall(p, fetKBSearch(40, groupID, "reborn", 0, 5)), "internal")
	_ = w.reported()
}
