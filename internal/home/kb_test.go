package home_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type kbTestDisk struct {
	noteDisk
}

func (d kbTestDisk) NamedNote(noteType, name string) (proto.NoteRow, bool) {
	for _, n := range d.notes {
		if n.NoteType == noteType && n.Name == name {
			return n, true
		}
	}
	return proto.NoteRow{}, false
}

func TestKBNoteCreateReplaceAndReplay(t *testing.T) {
	s := noteState(t)
	d := kbTestDisk{noteDisk{msgs: map[string]proto.MsgRow{}, notes: map[string]proto.NoteRow{}}}
	r := receipts{}

	// Plain Disk without NamedNoteDisk falls back cleanly to creating the note.
	plainPayload := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":"Plain disk","text":"v0"}`, id(140), noteTS, groupID, id(141))
	resPlain := noteApply(t, noteState(t), d.noteDisk, receipts{}, browser, plainPayload)
	f := false
	equal(t, resPlain.Answer, store.Outcome{OK: true, ID: id(141), Replaced: &f})

	// Create by AI (80-rune name boundary).
	name80 := strings.Repeat("决", 80)
	createCmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"as":%q,"id":%q,"task":null,"type":"kb","name":%q,"text":"First body"}`,
		id(142), noteTS, groupID, noteWorker, id(143), name80)
	res := noteApply(t, s, d, r, laptop, createCmd)
	equal(t, res.Answer, store.Outcome{OK: true, ID: id(143), Replaced: &f})
	wantCreated := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(143), Seq: 11}, Pos: 11, Group: groupID,
		NoteType: "kb", Name: name80, Text: "First body", By: noteWorker, Timestamp: noteTS,
	}
	equal(t, []any{res.Rows[0].Value()}, []any{wantCreated})
	noteNulls(t, res.Rows[0].Value(), "/task")
	frameRaw, _ := json.Marshal(res.Frame(9))
	equal(t, string(frameRaw), fmt.Sprintf(`{"re":9,"ok":true,"id":%q,"replaced":false}`, id(143)))
	s.Commit(res)
	d.notes[wantCreated.ID] = wantCreated

	// Replay returns the same id and replaced: false without writing a second row.
	replay := noteApply(t, s, d, r, laptop, createCmd)
	equal(t, replay, home.Result{Answer: store.Outcome{OK: true, ID: id(143), Replaced: &f}})

	// Advance seq, then replace by person: id and pos stay, command's own id is ignored.
	s.Seq = 20
	replaceCmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":%q,"text":"Updated body"}`,
		id(144), noteTS+50, groupID, id(143), name80)
	resRep := noteApply(t, s, d, r, browser, replaceCmd)
	tr := true
	equal(t, resRep.Answer, store.Outcome{OK: true, ID: id(143), Replaced: &tr})
	wantReplaced := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: id(143), Seq: 21}, Pos: 11, Group: groupID,
		NoteType: "kb", Name: name80, Text: "Updated body", By: ownerID, Timestamp: noteTS + 50,
	}
	equal(t, []any{resRep.Rows[0].Value()}, []any{wantReplaced})
	noteNulls(t, resRep.Rows[0].Value(), "/task")
}

func TestKBNoteRefusals(t *testing.T) {
	tooLong81 := strings.Repeat("a", 81)
	hugeText := strings.Repeat("x", 16_400)
	cases := map[string]struct {
		by      identity.Key
		payload string
		code    string
		wantSub string
	}{
		"type without name": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","text":"x"}`, id(150), noteTS, groupID, id(151)),
			code:    "bad_request",
		},
		"name without type": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"name":"Doc","text":"x"}`, id(150), noteTS, groupID, id(151)),
			code:    "bad_request",
		},
		"unknown type": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"other","name":"Doc","text":"x"}`, id(150), noteTS, groupID, id(151)),
			code:    "bad_request",
		},
		"name over 80 runes": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":%q,"text":"x"}`, id(150), noteTS, groupID, id(151), tooLong81),
			code:    "bad_request",
		},
		"control char newline in name": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":"a\nb","text":"x"}`, id(150), noteTS, groupID, id(151)),
			code:    "bad_request",
		},
		"control char C1 in name": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":"a`+"\u0085"+`b","text":"x"}`, id(150), noteTS, groupID, id(151)),
			code:    "bad_request",
		},
		"non-null task": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":%q,"type":"kb","name":"Doc","text":"x"}`, id(150), noteTS, groupID, id(151), noteTask),
			code:    "bad_request",
		},
		"row above 16 KiB limit": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":"Big","text":%q}`, id(150), noteTS, groupID, id(151), hugeText),
			code:    "bad_request",
			wantSub: "16384",
		},
		"id already used by another note": {
			by:      browser,
			payload: fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"id":%q,"task":null,"type":"kb","name":"Fresh","text":"x"}`, id(150), noteTS, groupID, id(121)),
			code:    "conflict",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := noteState(t)
			d := kbTestDisk{noteDisk{
				msgs: map[string]proto.MsgRow{},
				notes: map[string]proto.NoteRow{
					id(121): {RowHeader: proto.RowHeader{Type: "note", ID: id(121)}},
				},
			}}
			res := noteApply(t, s, d, receipts{}, tc.by, tc.payload)
			noteRefused(t, res, tc.code)
			if tc.wantSub != "" && !strings.Contains(res.Msg, tc.wantSub) {
				t.Fatalf("refusal msg %q lacks %q", res.Msg, tc.wantSub)
			}
		})
	}
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
