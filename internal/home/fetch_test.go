package home_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func fetYield() { time.Sleep(5 * time.Millisecond) }

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

func fetCall(p *runPeer, re int64, raw string) string {
	p.w.t.Helper()
	p.w.loop.Fetch(p.conn, re, json.RawMessage(raw))
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
	taskA, taskB := id(21), id(22)
	fetWrite(p, fetItem{Type: "msg", ID: "m1", Kind: "text", Text: "first"})
	fetWrite(p, fetItem{Type: "msg", ID: "m2", Thread: &taskA, Main: true, Kind: "decision", Text: "raised card"})
	fetWrite(p, fetItem{Type: "msg", ID: "m3", Thread: &taskA, Main: false, Kind: "text", Text: "in thread A"})
	fetWrite(p, fetItem{Type: "msg", ID: "m4", Thread: &taskB, Main: false, Kind: "text", Text: "in thread B"})
	fetWrite(p, fetItem{Type: "msg", ID: "m5", Kind: "text", Text: "fifth"})
	fetWrite(p, fetItem{Type: "msg", ID: "m6", Kind: "text", Text: "erased"})
	fetWrite(p, fetItem{Type: "msg", ID: "m6", Gone: true})

	// Extra unknown fields and uppercase shadows are ignored at run time.
	got := fetCall(p, 11, fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":1,"extra":true,"KIND":"note"}`, groupID))
	ids, more := fetParsed(t, 11, got)
	equal(t, []any{ids, more}, []any{[]string{"m5"}, true})

	got = fetCall(p, 12, fmt.Sprintf(`{"t":"fetch","id":2,"g":%q,"thread":null,"before":8,"limit":100}`, groupID))
	ids, more = fetParsed(t, 12, got)
	equal(t, []any{ids, more}, []any{[]string{"m2", "m1"}, false})

	got = fetCall(p, 13, fmt.Sprintf(`{"t":"fetch","id":3,"g":%q,"thread":null,"before":4,"limit":10}`, groupID))
	ids, more = fetParsed(t, 13, got)
	equal(t, []any{ids, more}, []any{[]string{}, false})

	got = fetCall(p, 14, fmt.Sprintf(`{"t":"fetch","id":4,"g":%q,"thread":%q,"before":0,"limit":1}`, groupID, taskA))
	ids, more = fetParsed(t, 14, got)
	equal(t, []any{ids, more}, []any{[]string{"m3"}, true})

	got = fetCall(p, 15, fmt.Sprintf(`{"t":"fetch","id":5,"g":%q,"thread":%q,"before":6,"limit":1}`, groupID, taskA))
	ids, more = fetParsed(t, 15, got)
	equal(t, []any{ids, more}, []any{[]string{"m2"}, false})

	got = fetCall(p, 16, fmt.Sprintf(`{"t":"fetch","id":6,"g":%q,"thread":%q,"before":0,"limit":10}`, groupID, id(99)))
	ids, more = fetParsed(t, 16, got)
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

	got := fetCall(p, 21, fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"note","before":0,"limit":1}`, groupID))
	ids, more := fetParsed(t, 21, got)
	equal(t, []any{ids, more}, []any{[]string{"n2"}, true})

	got = fetCall(p, 22, fmt.Sprintf(`{"t":"fetch","id":2,"g":%q,"kind":"note","before":7,"limit":2}`, groupID))
	ids, more = fetParsed(t, 22, got)
	equal(t, []any{ids, more}, []any{[]string{"n1"}, false})

	got = fetCall(p, 23, fmt.Sprintf(`{"t":"fetch","id":3,"g":%q,"kind":"review","before":0,"limit":1}`, groupID))
	ids, more = fetParsed(t, 23, got)
	equal(t, []any{ids, more}, []any{[]string{"r2"}, true})

	got = fetCall(p, 24, fmt.Sprintf(`{"t":"fetch","id":4,"g":%q,"kind":"review","before":10,"limit":100}`, groupID))
	ids, more = fetParsed(t, 24, got)
	equal(t, []any{ids, more}, []any{[]string{"r1"}, false})
}

func TestFetchOneRowReturnsLiveRowAndOmitsMissingOrGone(t *testing.T) {
	_, p := runFounded(t, fetTables())
	fetWrite(p, fetItem{Type: "note", ID: "n1", Text: "live <note> & text"})
	fetWrite(p, fetItem{Type: "note", ID: "n2", Text: "soon gone"})
	fetWrite(p, fetItem{Type: "note", ID: "n2", Gone: true})

	got := fetCall(p, 31, fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"note","row":"n1","before":0,"limit":1}`, groupID))
	ids, more := fetParsed(t, 31, got)
	equal(t, []any{ids, more}, []any{[]string{"n1"}, false})
	if !strings.Contains(got, `"text":"live <note> & text"`) {
		t.Fatalf("html characters were escaped in %s", got)
	}

	got = fetCall(p, 32, fmt.Sprintf(`{"t":"fetch","id":2,"g":%q,"r":"group","row":%q,"before":0,"limit":1}`, groupID, groupID))
	ids, more = fetParsed(t, 32, got)
	equal(t, []any{ids, more}, []any{[]string{groupID}, false})

	for i, spec := range [][2]string{
		{"note", "n2"},
		{"note", "absent"},
		{"msg", "n1"},
	} {
		re := int64(40 + i)
		raw := fmt.Sprintf(`{"t":"fetch","id":%d,"g":%q,"r":%q,"row":%q,"before":0,"limit":1}`, re, groupID, spec[0], spec[1])
		ids, more = fetParsed(t, re, fetCall(p, re, raw))
		equal(t, []any{ids, more}, []any{[]string{}, false})
	}
}

func TestFetchPacksRowsToTheExactByteLimit(t *testing.T) {
	// With re=9, {"t":"fetched","re":9,"ok":true,"rows":[r0,r1,r2,r3],"more":true}
	// has 54 bytes of wrapper plus 3 commas = 57 bytes around the 4 rows, and 58 bytes
	// when more is false. We build 4 notes whose encoded JSON lengths sum to 61,383 bytes:
	// with a 5th older note present, more is true and all 4 fit in exactly 61,440 bytes;
	// without a 5th note, including the 4th would make more false (61,441 bytes), so the
	// page stops at 3 rows with more true.
	fetCheckExactBoundary(t, true, 0, 61_440, []string{"p4", "p3", "p2", "p1"}, true)
	fetCheckExactBoundary(t, false, 0, 0, []string{"p4", "p3", "p2"}, true)
	fetCheckExactBoundary(t, true, 1, 0, []string{"p4", "p3", "p2"}, true)
}

func fetCheckExactBoundary(t *testing.T, withFifth bool, extraByte int, wantLen int, wantIDs []string, wantMore bool) {
	t.Helper()
	w, p := runFounded(t, fetTables())
	if withFifth {
		fetWrite(p, fetItem{Type: "note", ID: "p0", Text: "older fifth note"})
	}
	base := fetNoteOverhead(w.db, groupID)
	// Four rows: three of 15,346 bytes and one of 15,345 + extraByte bytes -> sum = 61,383 + extraByte.
	sizes := []int{15_345 + extraByte, 15_346, 15_346, 15_346}
	for i, target := range sizes {
		idName := fmt.Sprintf("p%d", i+1)
		textLen := target - base
		fetWrite(p, fetItem{Type: "note", ID: idName, Text: strings.Repeat("<", textLen)})
	}
	got := fetCall(p, 9, fmt.Sprintf(`{"t":"fetch","id":9,"g":%q,"kind":"note","before":0,"limit":4}`, groupID))
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
	got := fetCall(p, 50, fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"note","before":0,"limit":5}`, groupID))
	ids, more := fetParsed(t, 50, got)
	equal(t, []any{ids, more}, []any{[]string{"n5", "n4", "n3"}, true})
}

func TestFetchRefusesInvalidRequestsWithBadRequest(t *testing.T) {
	_, p := runFounded(t, fetTables())
	for i, raw := range fetBadRequests() {
		re := int64(100 + i)
		fetError(t, re, fetCall(p, re, raw), "bad_request")
	}
}

func fetBadRequests() []string {
	g := groupID
	return []string{
		`not json`,
		`[]`,
		`null`,
		`7`,
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"THREAD":null,"before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"kind":"note","before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"r":"msg","row":"m1","before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"note","r":"note","row":"n1","before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"row":"m1","before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"sync","id":1,"g":%q,"thread":null,"before":0,"limit":10}`, g),
		`{"t":"fetch","id":1,"g":"","thread":null,"before":0,"limit":10}`,
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":"","before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":7,"before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":-1,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":0}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":-1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":101}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":1.5}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"","before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"text","before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":null,"before":0,"limit":10}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"note","before":-1,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"note","before":0,"limit":0}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"kind":"note","before":0,"limit":101}`, g),
		fmt.Sprintf(`{"t":"sync","id":1,"g":%q,"r":"msg","row":"m1","before":0,"limit":1}`, g),
		`{"t":"fetch","id":1,"g":"","r":"msg","row":"m1","before":0,"limit":1}`,
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"","row":"m1","before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","row":"","before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","row":null,"before":0,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","row":"m1","before":1,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","row":"m1","before":-1,"limit":1}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","row":"m1","before":0,"limit":0}`, g),
		fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"r":"msg","row":"m1","before":0,"limit":2}`, g),
	}
}

func TestFetchGroupAccessAndUncreatedGroup(t *testing.T) {
	tables := fetTables()
	entered, release := make(chan struct{}), make(chan struct{})
	tables.Commands["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		if s.ID == runOther {
			entered <- struct{}{}
			<-release
			return home.Result{}, &home.Refusal{Code: "conflict", Msg: "held"}
		}
		return runCreate(s, c)
	}
	w, p := runFounded(t, tables)

	// Non-existent group with no goroutine answers not_found.
	rawAbsent := fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":10}`, id(77))
	fetError(t, 61, fetCall(p, 61, rawAbsent), "not_found")

	// Non-existent group while its temporary goroutine is running also answers not_found.
	otherPeer := w.attach(laptop)
	p.send(runOther, "group", `,"op":"create","name":"Other"`)
	runWait(t, entered)
	started, done := make(chan struct{}), make(chan string, 1)
	go func() {
		close(started)
		rawOther := fmt.Sprintf(`{"t":"fetch","id":2,"g":%q,"thread":null,"before":0,"limit":10}`, runOther)
		done <- fetCall(otherPeer, 62, rawOther)
	}()
	<-started
	fetYield()
	close(release)
	p.next()
	fetError(t, 62, <-done, "not_found")

	// Unserved devices (another person's device and an unknown device) get forbidden.
	rawGroup := fmt.Sprintf(`{"t":"fetch","id":3,"g":%q,"thread":null,"before":0,"limit":10}`, groupID)
	for i, dev := range []struct {
		peer *runPeer
	}{{w.attach(guest)}, {w.attach(nobody)}} {
		re := int64(70 + i)
		fetError(t, re, fetCall(dev.peer, re, rawGroup), "forbidden")
	}
}

func TestFetchDetachedConnAndStoreFailures(t *testing.T) {
	t.Run("query failure after snapshot", func(t *testing.T) {
		w, p := runFounded(t, fetTables())
		runExec(t, w.db, "DROP TABLE row")
		for i, raw := range []string{
			fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":10}`, groupID),
			fmt.Sprintf(`{"t":"fetch","id":2,"g":%q,"kind":"note","before":0,"limit":10}`, groupID),
			fmt.Sprintf(`{"t":"fetch","id":3,"g":%q,"r":"note","row":"n1","before":0,"limit":1}`, groupID),
		} {
			re := int64(80 + i)
			fetError(t, re, fetCall(p, re, raw), "internal")
			if got := w.reported(); !strings.Contains(got, "group "+groupID+", a fetch of "+browser.ID()) || !strings.Contains(got, "no such table: row") {
				t.Fatalf("reported %q; want row table error", got)
			}
		}
	})

	t.Run("snapshot failure and detached connection", func(t *testing.T) {
		w, p := runFounded(t, fetTables())
		runExec(t, w.db, "DROP TABLE grp")

		// A detached connection returns before touching the broken store, so nothing is reported.
		raw := fmt.Sprintf(`{"t":"fetch","id":1,"g":%q,"thread":null,"before":0,"limit":10}`, groupID)
		w.loop.Fetch(w.ghost, 90, json.RawMessage(raw))

		fetError(t, 91, fetCall(p, 91, raw), "internal")
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
