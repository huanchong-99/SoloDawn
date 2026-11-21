package home_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type fetItem struct {
	Type   string  `json:"row_type"`
	ID     string  `json:"row_id"`
	Text   string  `json:"text"`
	Thread *string `json:"thread"`
	Main   bool    `json:"main"`
	Kind   string  `json:"msg_kind"`
	Gone   bool    `json:"gone"`
}

func fetTables() home.Tables {
	tables := runTables()
	tables.Commands["post/item"] = fetApplyItem
	return tables
}

func fetApplyItem(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p fetItem
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	if p.Gone {
		return home.Result{Rows: []home.Row{home.RowOf(proto.GoneRow{
			RowHeader: proto.RowHeader{Type: p.Type, ID: p.ID},
			Gone:      true,
		})}}, nil
	}
	if p.Type == "note" {
		return home.Result{Rows: []home.Row{home.RowOf(proto.NoteRow{
			RowHeader: proto.RowHeader{Type: "note", ID: p.ID},
			Group:     s.ID,
			Text:      p.Text,
			By:        ownerID,
			Timestamp: clock,
		})}}, nil
	}
	return home.Result{Rows: []home.Row{home.RowOf(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: p.ID},
		Group:     s.ID,
		Thread:    p.Thread,
		Main:      p.Main,
		By:        ownerID,
		Kind:      p.Kind,
		Timestamp: clock,
		Text:      p.Text,
		Mentions:  []string{},
		Body:      json.RawMessage(`{}`),
	})}}, nil
}

func fetWrite(p *runPeer, item fetItem) {
	p.w.t.Helper()
	raw, _ := json.Marshal(item)
	p.does(groupID, "post", `,"kind":"item",`+string(raw[1:len(raw)-1]))
}

func fetThread(id int64, group string, thread *string, before int64, limit int) home.Fetch {
	return home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group, Before: before, Limit: limit},
		Thread:    thread,
	}
}

func fetKind(id int64, group, kind string, before int64, limit int) home.Fetch {
	return home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group, Before: before, Limit: limit},
		Kind:      kind,
	}
}

func fetRow(id int64, group, r, rowID string) home.Fetch {
	return home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: group},
		Row:       r,
		RowID:     rowID,
	}
}

func fetCall(p *runPeer, req home.Fetch) string {
	p.w.t.Helper()
	p.w.loop.Fetch(p.conn, req)
	return p.next()
}

func fetParsed(t *testing.T, re int64, got string) ([]string, bool) {
	t.Helper()
	var frame proto.Fetched
	if err := proto.Decode([]byte(got), &frame); err != nil {
		t.Fatalf("decode fetched %s: %v", got, err)
	}
	equal(t, []any{frame.Type, frame.Re, bool(frame.OK)}, []any{"fetched", re, true})
	ids := []string{}
	for _, r := range frame.Rows {
		var h proto.RowHeader
		if err := json.Unmarshal(r, &h); err != nil {
			t.Fatalf("unmarshal row header %s: %v", r, err)
		}
		ids = append(ids, h.ID)
	}
	return ids, frame.More
}

func fetError(t *testing.T, re int64, got, code string) {
	t.Helper()
	var frame proto.ErrorResponse
	if err := proto.Decode([]byte(got), &frame); err != nil {
		t.Fatalf("decode error %s: %v", got, err)
	}
	if frame.Re != re || frame.Err.Code != code || frame.Err.Message == "" {
		t.Fatalf("got %+v; want re %d and code %s", frame, re, code)
	}
}

func TestFetchPagesMainLineAndThreadsNewestFirst(t *testing.T) {
	_, p := runFounded(t, fetTables())
	taskA, taskB, absent, empty := id(21), id(22), id(99), ""
	fetWrite(p, fetItem{Type: "msg", ID: "m1", Kind: "text", Text: "first"})
	fetWrite(p, fetItem{Type: "msg", ID: "m2", Thread: &taskA, Main: true, Kind: "decision", Text: "raised card"})
	fetWrite(p, fetItem{Type: "msg", ID: "m3", Thread: &taskA, Main: false, Kind: "text", Text: "in thread A"})
	fetWrite(p, fetItem{Type: "msg", ID: "m4", Thread: &taskB, Main: false, Kind: "text", Text: "in thread B"})
	fetWrite(p, fetItem{Type: "msg", ID: "m5", Kind: "text", Text: "fifth"})
	fetWrite(p, fetItem{Type: "msg", ID: "m6", Kind: "text", Text: "erased"})
	fetWrite(p, fetItem{Type: "msg", ID: "m6", Gone: true})

	ids, more := fetParsed(t, 11, fetCall(p, fetThread(11, groupID, nil, 0, 1)))
	equal(t, []any{ids, more}, []any{[]string{"m5"}, true})

	ids, more = fetParsed(t, 12, fetCall(p, fetThread(12, groupID, nil, 8, 100)))
	equal(t, []any{ids, more}, []any{[]string{"m2", "m1"}, false})

	ids, more = fetParsed(t, 13, fetCall(p, fetThread(13, groupID, nil, 4, 10)))
	equal(t, []any{ids, more}, []any{[]string{}, false})

	ids, more = fetParsed(t, 14, fetCall(p, fetThread(14, groupID, &taskA, 0, 1)))
	equal(t, []any{ids, more}, []any{[]string{"m3"}, true})

	ids, more = fetParsed(t, 15, fetCall(p, fetThread(15, groupID, &taskA, 6, 1)))
	equal(t, []any{ids, more}, []any{[]string{"m2"}, false})

	ids, more = fetParsed(t, 16, fetCall(p, fetThread(16, groupID, &absent, 0, 10)))
	equal(t, []any{ids, more}, []any{[]string{}, false})

	ids, more = fetParsed(t, 17, fetCall(p, fetThread(17, groupID, &empty, 0, 10)))
	equal(t, []any{ids, more}, []any{[]string{}, false})
}

func TestFetchPagesNotesAndReviewsByKind(t *testing.T) {
	_, p := runFounded(t, fetTables())
	taskA, taskB := id(31), id(32)
	fetWrite(p, fetItem{Type: "note", ID: "n1", Text: "note 1"})
	fetWrite(p, fetItem{Type: "msg", ID: "r1", Thread: &taskA, Kind: "review", Text: "review 1"})
	fetWrite(p, fetItem{Type: "msg", ID: "t1", Thread: &taskA, Kind: "text", Text: "not in ledger"})
	fetWrite(p, fetItem{Type: "note", ID: "n2", Text: "note 2"})
	fetWrite(p, fetItem{Type: "note", ID: "n3", Text: "removed note"})
	fetWrite(p, fetItem{Type: "note", ID: "n3", Gone: true})
	fetWrite(p, fetItem{Type: "msg", ID: "r2", Thread: &taskB, Kind: "review", Text: "review 2"})
	fetWrite(p, fetItem{Type: "msg", ID: "r3", Thread: &taskB, Kind: "review", Text: "removed review"})
	fetWrite(p, fetItem{Type: "msg", ID: "r3", Gone: true})

	ids, more := fetParsed(t, 21, fetCall(p, fetKind(21, groupID, "note", 0, 1)))
	equal(t, []any{ids, more}, []any{[]string{"n2"}, true})

	ids, more = fetParsed(t, 22, fetCall(p, fetKind(22, groupID, "note", 7, 2)))
	equal(t, []any{ids, more}, []any{[]string{"n1"}, false})

	ids, more = fetParsed(t, 23, fetCall(p, fetKind(23, groupID, "review", 0, 1)))
	equal(t, []any{ids, more}, []any{[]string{"r2"}, true})

	ids, more = fetParsed(t, 24, fetCall(p, fetKind(24, groupID, "review", 10, 100)))
	equal(t, []any{ids, more}, []any{[]string{"r1"}, false})

	ids, more = fetParsed(t, 25, fetCall(p, fetKind(25, groupID, "text", 0, 10)))
	equal(t, []any{ids, more}, []any{[]string{}, false})

	// Kind takes precedence over Row when both are populated.
	mixed := fetKind(26, groupID, "note", 0, 10)
	mixed.Row, mixed.RowID = "msg", "t1"
	ids, more = fetParsed(t, 26, fetCall(p, mixed))
	equal(t, []any{ids, more}, []any{[]string{"n2", "n1"}, false})
}

func TestFetchOneRowReturnsLiveRowAndOmitsMissingOrGone(t *testing.T) {
	_, p := runFounded(t, fetTables())
	fetWrite(p, fetItem{Type: "msg", ID: "m1", Kind: "text", Text: "main line"})
	fetWrite(p, fetItem{Type: "note", ID: "n1", Text: "live <note> & text"})
	fetWrite(p, fetItem{Type: "note", ID: "n2", Text: "soon gone"})
	fetWrite(p, fetItem{Type: "note", ID: "n2", Gone: true})

	got := fetCall(p, fetRow(31, groupID, "note", "n1"))
	ids, more := fetParsed(t, 31, got)
	equal(t, []any{ids, more}, []any{[]string{"n1"}, false})
	if !strings.Contains(got, `"text":"live <note> & text"`) {
		t.Fatalf("html characters were escaped in %s", got)
	}

	// Single-row reads ignore Before and Limit.
	ignored := home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 32}, Group: groupID, Before: -5, Limit: 999},
		Row:       "group",
		RowID:     groupID,
	}
	ids, more = fetParsed(t, 32, fetCall(p, ignored))
	equal(t, []any{ids, more}, []any{[]string{groupID}, false})

	for i, spec := range [][2]string{
		{"note", "n2"},
		{"note", "absent"},
		{"msg", "n1"},
		{"", "n1"},
		{"note", ""},
	} {
		re := int64(40 + i)
		ids, more = fetParsed(t, re, fetCall(p, fetRow(re, groupID, spec[0], spec[1])))
		equal(t, []any{ids, more}, []any{[]string{}, false})
	}
}

func TestFetchPacksRowsToTheExactByteLimit(t *testing.T) {
	// With re=9, {"t":"fetched","re":9,"ok":true,"rows":[r0,r1,r2,r3],"more":true}
	// has 54 bytes of wrapper plus 3 commas = 57 bytes around the 4 rows, and 58 bytes
	// when more is false. We build 4 notes whose encoded JSON lengths sum to 61,383 + extraByte:
	// with a 5th older note present, more is true and 61,383 fits in 61,440 bytes while 61,384 drops p1;
	// without a 5th note, more is false so 61,382 fits in 61,440 bytes while 61,383 drops p1 (more true).
	fetCheckExactBoundary(t, true, 0, 61_440, []string{"p4", "p3", "p2", "p1"}, true)
	fetCheckExactBoundary(t, true, 1, 0, []string{"p4", "p3", "p2"}, true)
	fetCheckExactBoundary(t, false, -1, 61_440, []string{"p4", "p3", "p2", "p1"}, false)
	fetCheckExactBoundary(t, false, 0, 0, []string{"p4", "p3", "p2"}, true)
}

func fetCheckExactBoundary(t *testing.T, withFifth bool, extraByte int, wantLen int, wantIDs []string, wantMore bool) {
	t.Helper()
	w, p := runFounded(t, fetTables())
	if withFifth {
		fetWrite(p, fetItem{Type: "note", ID: "p0", Text: "older fifth note"})
	}
	base := fetNoteOverhead(w.db, groupID)
	sizes := []int{15_345 + extraByte, 15_346, 15_346, 15_346}
	for i, target := range sizes {
		idName := fmt.Sprintf("p%d", i+1)
		textLen := target - base
		fetWrite(p, fetItem{Type: "note", ID: idName, Text: strings.Repeat("<", textLen)})
	}
	got := fetCall(p, fetKind(9, groupID, "note", 0, 4))
	if wantLen > 0 {
		equal(t, len(got), wantLen)
	}
	if len(got) > 61_440 {
		t.Fatalf("frame length %d exceeds 61,440", len(got))
	}
	ids, more := fetParsed(t, 9, got)
	equal(t, []any{ids, more}, []any{wantIDs, wantMore})
}

func fetNoteOverhead(db *store.Store, group string) int {
	var size int
	err := db.Write(func(tx *store.Tx) error {
		n := proto.NoteRow{
			RowHeader: proto.RowHeader{Type: "note", ID: "p1", Seq: 5},
			Pos:       5,
			Group:     group,
			By:        ownerID,
			Timestamp: clock,
		}
		if err := store.Put(tx, group, clock, n); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT length(data) FROM row WHERE g=? AND r='note' AND id='p1'", group).Scan(&size); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM row WHERE g=? AND r='note' AND id='p1'", group)
		return err
	})
	if err != nil {
		panic(err)
	}
	return size
}

func TestFetchStopsAtFirstOverflowingRowWithoutSkipping(t *testing.T) {
	_, p := runFounded(t, fetTables())
	// Newest to oldest: n5 (16,000), n4 (16,000), n3 (16,000), n2 (16,000), n1 (tiny).
	// n5+n4+n3 fits (~48 KB); adding n2 overflows 61,440 bytes; n1 would fit if n2 were skipped.
	fetWrite(p, fetItem{Type: "note", ID: "n1", Text: "tiny oldest"})
	for _, name := range []string{"n2", "n3", "n4", "n5"} {
		fetWrite(p, fetItem{Type: "note", ID: name, Text: strings.Repeat("a", 15_900)})
	}
	got := fetCall(p, fetKind(50, groupID, "note", 0, 5))
	ids, more := fetParsed(t, 50, got)
	equal(t, []any{ids, more}, []any{[]string{"n5", "n4", "n3"}, true})
}

func TestFetchRefusesOutOfBoundsPageWithBadRequest(t *testing.T) {
	_, p := runFounded(t, fetTables())
	for i, req := range []home.Fetch{
		fetThread(100, groupID, nil, -1, 1),
		fetThread(101, groupID, nil, 0, 0),
		fetThread(102, groupID, nil, 0, -1),
		fetThread(103, groupID, nil, 0, 101),
		fetKind(104, groupID, "note", -1, 1),
		fetKind(105, groupID, "note", 0, 0),
		fetKind(106, groupID, "note", 0, 101),
	} {
		re := int64(100 + i)
		fetError(t, re, fetCall(p, req), "bad_request")
	}
}

func TestFetchGroupAccess(t *testing.T) {
	w, p := runFounded(t, fetTables())

	// Unknown or empty group answers not_found.
	for i, g := range []string{id(77), ""} {
		re := int64(61 + i)
		fetError(t, re, fetCall(p, fetThread(re, g, nil, 0, 10)), "not_found")
	}

	// Unserved devices (another person's device and an unknown device) get forbidden.
	for i, peer := range []*runPeer{w.attach(guest), w.attach(nobody)} {
		re := int64(70 + i)
		fetError(t, re, fetCall(peer, fetThread(re, groupID, nil, 0, 10)), "forbidden")
	}
}

func TestFetchDetachedConnAndStoreFailures(t *testing.T) {
	t.Run("query failure after snapshot", func(t *testing.T) {
		w, p := runFounded(t, fetTables())
		runExec(t, w.db, "DROP TABLE row")
		for _, req := range []home.Fetch{
			fetThread(80, groupID, nil, 0, 10),
			fetKind(81, groupID, "note", 0, 10),
			fetRow(82, groupID, "note", "n1"),
		} {
			fetError(t, req.ID, fetCall(p, req), "internal")
			if got := w.reported(); !strings.Contains(got, "group "+groupID+", a fetch of "+browser.ID()) || !strings.Contains(got, "no such table: row") {
				t.Fatalf("reported %q; want row table error", got)
			}
		}
	})

	t.Run("snapshot failure and detached connection", func(t *testing.T) {
		w, p := runFounded(t, fetTables())
		runExec(t, w.db, "DROP TABLE grp")

		// A detached connection returns before touching the broken store, so nothing is reported.
		w.loop.Fetch(w.ghost, fetThread(90, groupID, nil, 0, 10))

		fetError(t, 91, fetCall(p, fetThread(91, groupID, nil, 0, 10)), "internal")
		if got := w.reported(); !strings.Contains(got, "group "+groupID+", a fetch of "+browser.ID()) || !strings.Contains(got, "no such table: grp") {
			t.Fatalf("reported %q; want grp table error", got)
		}
		select {
		case extra := <-w.reports:
			t.Fatalf("unexpected extra report: %v", extra)
		default:
		}
	})
}
