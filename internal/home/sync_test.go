package home_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

type synGot struct {
	reset  bool
	frames int
	rows   []json.RawMessage
	ids    []string
	seqs   []int64
	synced proto.Synced
}

func synPutRows(t *testing.T, db *store.Store, group string, rows ...home.Row) int64 {
	t.Helper()
	var last int64
	run := func(tx *store.Tx) error {
		for _, r := range rows {
			seq, err := tx.NextSeq(group)
			if err != nil {
				return err
			}
			last = seq
			if err := synPutNumbered(tx, group, seq, r); err != nil {
				return err
			}
		}
		return nil
	}
	if err := db.Write(run); err != nil {
		t.Fatal(err)
	}
	return last
}

func synPutNumbered(tx *store.Tx, group string, seq int64, r home.Row) error {
	s := &home.State{Seq: seq - 1}
	sealed := s.Seal(home.Result{Rows: []home.Row{r}})
	return sealed.Rows[0].Put(tx, group, clock)
}

func synCollect(t *testing.T, p *runPeer, re int64) synGot {
	t.Helper()
	var out synGot
	for {
		raw := p.next()
		if len(raw) > 61_440 {
			t.Fatalf("frame exceeds ctl limit: %d bytes", len(raw))
		}
		if synReadFrame(t, raw, re, &out) {
			return out
		}
	}
}

func synReadFrame(t *testing.T, raw string, re int64, out *synGot) bool {
	t.Helper()
	var kind struct {
		Type string `json:"t"`
	}
	equal(t, json.Unmarshal([]byte(raw), &kind), nil)
	switch kind.Type {
	case "reset":
		var rst proto.Reset
		equal(t, proto.Decode([]byte(raw), &rst), nil)
		equal(t, []any{rst.Group, out.reset, out.frames}, []any{groupID, false, 0})
		out.reset = true
	case "rows":
		synAppendBatch(t, raw, out)
	case "synced":
		equal(t, proto.UnexpectedNulls([]byte(raw)), []string{})
		equal(t, proto.Decode([]byte(raw), &out.synced), nil)
		equal(t, []any{out.synced.Re, out.synced.Group}, []any{re, groupID})
		return true
	default:
		t.Fatalf("unexpected frame during sync: %s", raw)
	}
	return false
}

func synAppendBatch(t *testing.T, raw string, out *synGot) {
	t.Helper()
	var batch proto.Rows
	equal(t, proto.Decode([]byte(raw), &batch), nil)
	if len(batch.Rows) == 0 {
		t.Fatalf("empty rows batch: %s", raw)
	}
	out.frames++
	for _, r := range batch.Rows {
		var h proto.RowHeader
		equal(t, json.Unmarshal(r, &h), nil)
		out.rows = append(out.rows, r)
		out.ids = append(out.ids, h.ID)
		out.seqs = append(out.seqs, h.Seq)
	}
}

func synStrictlyIncreasing(t *testing.T, seqs []int64) {
	t.Helper()
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("seqs not strictly increasing at %d: %v", i, seqs)
		}
	}
}

func synCard(id string, pos int64, thread *string, main bool, status string) proto.MsgRow {
	m := runMsg(id, 0, "card "+id)
	m.Pos = pos
	m.Thread = thread
	m.Main = main
	m.Kind = "decision"
	m.State = &proto.CardState{Status: status}
	return m
}

func synTask(id string, num int) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id}, Group: groupID, Num: num,
		Title: "Task", Origin: "direct", State: "active", Step: "work",
		Assignee: aiID, Pass: 90, Depends: []string{}, Stats: proto.TaskStats{Started: clock}, Created: clock,
	}
}

func TestSyncWireFramesMatchGoldens(t *testing.T) {
	w, p := runFounded(t, runTables())
	p.does(groupID, "post", `,"kind":"text","text":"hi"`)
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 2}, Group: groupID, Since: 0})
	frames := []string{p.next(), p.next(), p.next()}
	for i, name := range []string{"reset", "rows", "synced"} {
		example, err := os.ReadFile(filepath.Join("..", "proto", "testdata", name+".json"))
		equal(t, err, nil)
		var want, got map[string]any
		equal(t, json.Unmarshal(example, &want), nil)
		equal(t, json.Unmarshal([]byte(frames[i]), &got), nil)
		equal(t, synKeys(got), synKeys(want))
	}
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 8}, Group: "", Since: 0})
	var errResp map[string]any
	equal(t, json.Unmarshal([]byte(p.next()), &errResp), nil)
	equal(t, synKeys(errResp), []string{"err", "re"})
}

func synKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

func TestSyncColdContentAndLiveSubscription(t *testing.T) {
	db := runOpen(t)
	thread := id(30)
	synSeedColdGroup(t, db, thread)
	w := runStart(t, db, runTables())
	reader, writer := w.attach(browser), w.attach(laptop)

	w.loop.Sync(reader.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 2}, Group: groupID, Since: 0})
	got := synCollect(t, reader, 2)
	equal(t, got.reset, true)
	synStrictlyIncreasing(t, got.seqs)
	synVerifyColdSet(t, got, w.db)

	writer.does(groupID, "post", `,"kind":"text","text":"after sync"`)
	live := runMsg(runID(w.nonce.Load()), got.synced.Seq+1, "after sync")
	reader.gets(runRows(groupID, live))
	writer.does(groupID, "note", `,"text":"live note"`)
	reader.gets(runRows(groupID, runNote(runID(w.nonce.Load()), got.synced.Seq+2, "live note")))
}

func synSeedColdGroup(t *testing.T, db *store.Store, thread string) {
	t.Helper()
	dev := deviceRow(browser, browser, 2, "add", node)
	synPutRows(t, db, groupID,
		home.RowOf(runGroupRow(groupID, "Website", 0)),
		home.RowOf(runPerson(0)),
		home.RowOf(runAI(0)),
		home.RowOf(dev),
		home.RowOf(synTask(thread, 1)),
		home.RowOf(person(id(31), guest, "member")),
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: id(31)}, Gone: true}),
		home.RowOf(synCard("old-thread-pending", 0, &thread, false, "pending")),
		home.RowOf(synCard("old-main-pending", 0, nil, true, "pending")),
		home.RowOf(synCard("old-decided", 0, nil, true, "decided")),
		home.RowOf(synThreadMsg("thread-msg", thread)),
		home.RowOf(runNote("note-1", 0, "n")),
	)
	synSeedMainMessages(t, db, thread)
}

func synThreadMsg(id, thread string) proto.MsgRow {
	m := runMsg(id, 0, "in thread")
	m.Thread = &thread
	return m
}

func synSeedMainMessages(t *testing.T, db *store.Store, thread string) {
	t.Helper()
	var earlyCard proto.MsgRow
	for i := 1; i <= 56; i++ {
		name := fmt.Sprintf("main-%02d", i)
		row := runMsg(name, 0, name)
		if i == 20 {
			row = synCard(name, 0, nil, true, "pending")
			earlyCard = row
		}
		if i == 40 {
			row = synCard(name, 0, nil, true, "pending")
		}
		synPutRows(t, db, groupID, home.RowOf(row))
		if i == 20 {
			earlyCard.Pos = runQuery[int64](t, db, "SELECT pos FROM row WHERE g=? AND id=?", groupID, name)
		}
	}
	// Delete main-30 so 55 live main-line messages remain (main-06..main-56 without main-30 -> newest 50 starts at main-06).
	synPutRows(t, db, groupID, home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "msg", ID: "main-30"}, Gone: true}))
	// Decide main-20 later so its seq moves past main-56 while its pos stays early in the newest 50.
	earlyCard.State = &proto.CardState{Status: "decided"}
	synPutRows(t, db, groupID, home.RowOf(earlyCard), home.RowOf(synTask(thread, 1)))
}

func synVerifyColdSet(t *testing.T, got synGot, db *store.Store) {
	t.Helper()
	wantSeq := runQuery[int64](t, db, "SELECT seq FROM grp WHERE g=?", groupID)
	equal(t, got.synced.Seq, wantSeq)
	seen := map[string]int{}
	for _, id := range got.ids {
		seen[id]++
	}
	// 5 state rows + 2 old pending cards + 50 newest live main-line messages (main-40 is both pending and in newest 50, counted once).
	equal(t, len(got.ids), 57)
	for _, must := range []string{groupID, ownerID, aiID, browser.ID() + ":2", id(30), "old-thread-pending", "old-main-pending", "main-06", "main-20", "main-40", "main-56"} {
		equal(t, seen[must], 1)
	}
	for _, excluded := range []string{id(31), "old-decided", "thread-msg", "note-1", "main-01", "main-05", "main-30"} {
		equal(t, seen[excluded], 0)
	}
}

func TestSyncDeltaAndPurgeBoundaries(t *testing.T) {
	w, p := runFounded(t, runTables())
	thread := id(40)
	freshMsg := runMsg("m-last", 0, "stay")
	freshMsg.Timestamp = clock + 3*86_400_000
	end := synPutRows(t, w.db, groupID,
		home.RowOf(synThreadMsg("t-msg", thread)),
		home.RowOf(runNote("n-keep", 0, "kept")),
		home.RowOf(runMsg("m-gone", 0, "bye")),
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "msg", ID: "m-gone"}, Gone: true}),
		home.RowOf(freshMsg),
	)
	// Exact boundary since == 1 (first positive cursor): delta without reset.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 9}, Group: groupID, Since: 1})
	got := synCollect(t, p, 9)
	equal(t, []any{got.reset, got.ids, got.synced.Seq}, []any{false, []string{ownerID, aiID, "t-msg", "n-keep", "m-gone", "m-last"}, end})

	// Negative since resets like 0.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 8}, Group: groupID, Since: -1})
	got = synCollect(t, p, 8)
	equal(t, []any{got.reset, got.ids, got.synced.Seq}, []any{true, []string{groupID, ownerID, aiID, "m-last"}, end})

	// Exact boundary since == S: delta with zero rows, no reset and no rows frame.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 10}, Group: groupID, Since: end})
	got = synCollect(t, p, 10)
	equal(t, []any{got.reset, got.frames, len(got.ids), got.synced.Seq}, []any{false, 0, 0, end})

	// Delta (3, end]: includes thread msg, note, gone marker and main msg in seq order.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 11}, Group: groupID, Since: 3})
	got = synCollect(t, p, 11)
	equal(t, []any{got.reset, got.ids, got.synced.Seq}, []any{false, []string{"t-msg", "n-keep", "m-gone", "m-last"}, end})
	synStrictlyIncreasing(t, got.seqs)

	// Boundary since == S + 1 (ahead of store): reset.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 12}, Group: groupID, Since: end + 1})
	got = synCollect(t, p, 12)
	equal(t, []any{got.reset, got.ids, got.synced.Seq}, []any{true, []string{groupID, ownerID, aiID, "m-last"}, end})

	// Advance purge mark to 7 via Retain (the gone marker of m-gone is at seq 7).
	synRetainGone(t, w.db)
	equal(t, runQuery[int64](t, w.db, "SELECT purged FROM grp WHERE g=?", groupID), int64(7))

	// Exact boundary since == purged (7): delta.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 13}, Group: groupID, Since: 7})
	got = synCollect(t, p, 13)
	equal(t, []any{got.reset, got.ids}, []any{false, []string{"m-last"}})

	// Boundary since == purged - 1 (6): reset.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 14}, Group: groupID, Since: 6})
	got = synCollect(t, p, 14)
	equal(t, []any{got.reset, got.ids}, []any{true, []string{groupID, ownerID, aiID, "m-last"}})
}

func synRetainGone(t *testing.T, db *store.Store) {
	t.Helper()
	err := db.Write(func(tx *store.Tx) error {
		return tx.Retain(time.UnixMilli(clock).AddDate(0, 0, 2), 1)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncThousandRowAndBatchBoundaries(t *testing.T) {
	w, p := runFounded(t, runTables())
	bulk := make([]home.Row, 1001)
	for i := range bulk {
		bulk[i] = home.RowOf(runMsg(fmt.Sprintf("b-%04d", i+1), 0, "x"))
	}
	end := synPutRows(t, w.db, groupID, bulk...)
	equal(t, end, int64(1004))

	// Exact boundary count == 1000 (since = 4, rows 5..1004): delta without reset.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 20}, Group: groupID, Since: 4})
	got := synCollect(t, p, 20)
	equal(t, []any{got.reset, len(got.ids), got.ids[0], got.ids[999], got.synced.Seq}, []any{false, 1000, "b-0002", "b-1001", int64(1004)})
	synStrictlyIncreasing(t, got.seqs)

	// Exact boundary count == 1001 (since = 3, rows 4..1004): reset with 3 state rows + 50 newest main-line messages.
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 21}, Group: groupID, Since: 3})
	got = synCollect(t, p, 21)
	equal(t, []any{got.reset, len(got.ids), got.ids[3], got.ids[52], got.synced.Seq}, []any{true, 53, "b-0952", "b-1001", int64(1004)})
	synStrictlyIncreasing(t, got.seqs)

	// Large rows exceeding one 60 KiB ctl frame are split across multiple rows frames.
	bigText := strings.Repeat("a", 15_000)
	bigEnd := synPutRows(t, w.db, groupID,
		home.RowOf(runMsg("big-1", 0, bigText)),
		home.RowOf(runMsg("big-2", 0, bigText)),
		home.RowOf(runMsg("big-3", 0, bigText)),
		home.RowOf(runMsg("big-4", 0, bigText)),
		home.RowOf(runMsg("big-5", 0, bigText)),
	)
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{ID: 22}, Group: groupID, Since: end})
	got = synCollect(t, p, 22)
	equal(t, []any{got.reset, got.frames, got.ids, got.synced.Seq}, []any{false, 2, []string{"big-1", "big-2", "big-3", "big-4", "big-5"}, bigEnd})
}

func TestSyncStateOnly(t *testing.T) {
	tables := runTables()
	tables.Commands["task/touch"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		return home.Result{Rows: []home.Row{home.RowOf(synTask(string(c.Nonce), 1))}}, nil
	}
	w, owner := runFounded(t, tables)
	host := w.attach(laptop)
	end := synPutRows(t, w.db, groupID,
		home.RowOf(synCard("pending-card", 0, nil, true, "pending")),
		home.RowOf(runMsg("chat-msg", 0, "hello")),
		home.RowOf(runNote("chat-note", 0, "note")),
		home.RowOf(person("temp-member", guest, "member")),
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "member", ID: "temp-member"}, Gone: true}),
		home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-note"}, Gone: true}),
		home.RowOf(synTask("task-1", 1)),
	)

	// Cold sync with state_only: reset + live state rows only (no gone marker, pending card, msg or note).
	w.loop.Sync(host.conn, proto.Sync{Request: proto.Request{ID: 30}, Group: groupID, Since: 0, StateOnly: true})
	got := synCollect(t, host, 30)
	equal(t, []any{got.reset, got.ids, got.synced.Seq}, []any{true, []string{groupID, ownerID, aiID, "task-1"}, end})

	// Delta sync with state_only: no reset, state rows and state gone markers only.
	w.loop.Sync(host.conn, proto.Sync{Request: proto.Request{ID: 31}, Group: groupID, Since: 3, StateOnly: true})
	got = synCollect(t, host, 31)
	equal(t, []any{got.reset, got.ids, got.synced.Seq}, []any{false, []string{"temp-member", "task-1"}, end})

	// Keep the group's in-memory Seq aligned before live commands.
	w.loop.Close()
	w = runStart(t, w.db, tables)
	w.nonce.Store(10)
	owner, host = w.attach(browser), w.attach(laptop)
	w.loop.Sync(host.conn, proto.Sync{Request: proto.Request{ID: 32}, Group: groupID, Since: end, StateOnly: true})
	got = synCollect(t, host, 32)
	equal(t, []any{got.reset, got.frames}, []any{false, 0})

	// Live msg and note are omitted for state_only; task row is pushed.
	owner.does(groupID, "post", `,"kind":"text","text":"ignored by host"`)
	owner.does(groupID, "note", `,"text":"also ignored"`)
	owner.does(groupID, "task", `,"op":"touch"`)
	host.gets(runRows(groupID, synNumberedTask(runID(w.nonce.Load()), end+3)))

	// Re-syncing with state_only false switches the subscription to receive notes and messages.
	w.loop.Sync(host.conn, proto.Sync{Request: proto.Request{ID: 33}, Group: groupID, Since: end + 3, StateOnly: false})
	synCollect(t, host, 33)
	owner.does(groupID, "note", `,"text":"now delivered"`)
	host.gets(runRows(groupID, runNote(runID(w.nonce.Load()), end+4, "now delivered")))
}

func synNumberedTask(id string, seq int64) proto.TaskRow {
	r := synTask(id, 1)
	r.Seq = seq
	return r
}

// A2.7: a row written while a first read is being sent is neither missing at
// synced nor applied twice.
func TestSyncConcurrentWriteDuringFirstRead(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "node.db"))
	equal(t, err, nil)
	t.Cleanup(func() { db.Close() })
	w := runStart(t, db, runTables())
	writer := w.attach(laptop)
	writer.does(groupID, "group", runCreateWebsite)
	writer.next()
	writer.does(groupID, "post", `,"kind":"text","text":"before sync"`)

	slow := runNewClient()
	slow.hold = make(chan struct{})
	reader := w.attachClient(browser, slow, slow)
	w.loop.Sync(reader.conn, proto.Sync{Request: proto.Request{ID: 40}, Group: groupID, Since: 0})
	// Wait until pump has picked up the first frame (reset) and is blocked in Send.
	runWait(t, slow.started)
	// Write two rows while the initial sync frames are still waiting to be sent.
	writer.does(groupID, "post", `,"kind":"text","text":"during 1"`)
	id1 := runID(w.nonce.Load())
	writer.does(groupID, "post", `,"kind":"text","text":"during 2"`)
	id2 := runID(w.nonce.Load())
	close(slow.hold)

	got := synCollect(t, reader, 40)
	equal(t, []any{got.reset, got.synced.Seq, got.seqs}, []any{true, int64(4), []int64{1, 2, 3, 4}})
	reader.gets(runRows(groupID, runMsg(id1, 5, "during 1")), runRows(groupID, runMsg(id2, 6, "during 2")))
	reader.quiet()
}

func TestSyncRefusals(t *testing.T) {
	w, owner := runFounded(t, runTables())
	guestPeer, stranger := w.attach(guest), w.attach(nobody)

	for _, c := range []struct {
		peer *runPeer
		req  proto.Sync
		code string
	}{
		{owner, proto.Sync{Request: proto.Request{ID: 50}, Group: "", Since: 0}, "not_found"},
		{owner, proto.Sync{Request: proto.Request{ID: 52}, Group: id(99), Since: 0}, "not_found"},
		{guestPeer, proto.Sync{Request: proto.Request{ID: 53}, Group: groupID, Since: 0}, "forbidden"},
		{stranger, proto.Sync{Request: proto.Request{ID: 54}, Group: groupID, Since: 0}, "forbidden"},
	} {
		w.loop.Sync(c.peer.conn, c.req)
		synExpectErr(t, c.peer, c.req.ID, c.code)
	}
	// Neither unauthorized connection was subscribed.
	owner.does(groupID, "post", `,"kind":"text","text":"private"`)
	guestPeer.quiet()
}

func synExpectErr(t *testing.T, p *runPeer, re int64, code string) {
	t.Helper()
	var resp proto.ErrorResponse
	raw := p.next()
	equal(t, proto.Decode([]byte(raw), &resp), nil)
	equal(t, []any{resp.Re, resp.Err.Code, resp.Err.Message != ""}, []any{re, code, true})
}

func TestSyncDetachedAndStoreFailures(t *testing.T) {
	w, owner := runFounded(t, runTables())
	detached := w.attach(browser)
	w.loop.Detach(detached.conn)
	w.idle()

	// Spoil the row table and verify a detached owner connection returns before touching the store.
	runExec(t, w.db, "DROP TABLE row")
	w.loop.Sync(detached.conn, proto.Sync{Request: proto.Request{ID: 60}, Group: groupID, Since: 0})
	equal(t, len(w.reports), 0)

	// On a live connection, missing row table fails both cold (since 0) and delta (since 1) without subscribing.
	for _, req := range []proto.Sync{
		{Request: proto.Request{ID: 61}, Group: groupID, Since: 0},
		{Request: proto.Request{ID: 62}, Group: groupID, Since: 1},
	} {
		w.loop.Sync(owner.conn, req)
		synExpectErr(t, owner, req.ID, "internal")
		if got := w.reported(); !strings.Contains(got, "group "+groupID+", a sync of "+browser.ID()) || !strings.Contains(got, "no such table: row") {
			t.Fatalf("unexpected report: %s", got)
		}
	}

	// Missing grp table fails Snapshot itself without subscribing.
	w2, owner2 := runFounded(t, runTables())
	runExec(t, w2.db, "DROP TABLE grp")
	w2.loop.Sync(owner2.conn, proto.Sync{Request: proto.Request{ID: 63}, Group: groupID, Since: 0})
	synExpectErr(t, owner2, 63, "internal")
	if got := w2.reported(); !strings.Contains(got, "no such table: grp") {
		t.Fatalf("unexpected snapshot report: %s", got)
	}
	// Restore grp and verify owner2 was not subscribed by the failed sync.
	runExec(t, w2.db, "CREATE TABLE grp (g TEXT PRIMARY KEY, seq INTEGER NOT NULL, num INTEGER NOT NULL, purged INTEGER NOT NULL DEFAULT 0) STRICT")
	runExec(t, w2.db, "INSERT INTO grp(g, seq, num) VALUES (?, 3, 0)", groupID)
	other := w2.attach(laptop)
	other.does(groupID, "post", `,"kind":"text","text":"unseen"`)
	owner2.quiet()
}
