package home_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func inboxDecode(t *testing.T, raw string) proto.Inbox {
	t.Helper()
	var frame proto.Inbox
	if err := proto.Decode([]byte(raw), &frame); err != nil {
		t.Fatalf("proto.Decode(%s): %v", raw, err)
	}
	return frame
}

func inboxCleanTask(id string, seq int64, title string) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id, Seq: seq}, Group: groupID, Num: 1,
		Title: title, Origin: "brief", State: "active", Step: "review",
		Report: "m1", Assignee: aiID, Depends: []string{}, Created: clock,
	}
}

func inboxGoldenToHome(want proto.Item) home.Item {
	return home.Item{
		AI: string(want.AI), Why: want.Why, Task: string(want.Task), Msg: string(want.Msg),
		Code: want.Code, Detail: want.Detail,
	}
}

func inboxGoldenNames() []string {
	return []string{
		"item-mention", "item-reply", "item-assign", "item-review", "item-reviewed",
		"item-resume", "item-decided", "item-confirmed", "item-notice",
	}
}

// Every frozen item-*.json shape round-trips through the home's inbox store
// and wire frame, and an InboxItem copies live msg and task rows when sent
// while omitting missing or gone ones.
func TestInboxGoldensAndRowCopies(t *testing.T) {
	for _, name := range inboxGoldenNames() {
		t.Run(name, func(t *testing.T) {
			inboxCheckGolden(t, name)
		})
	}
	t.Run("row copies and updates", inboxCheckRowCopies)
}

func inboxCheckGolden(t *testing.T, name string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "proto", "testdata", name+".json"))
	equal(t, err, nil)
	var want proto.Item
	equal(t, proto.Decode(raw, &want), nil)

	tables := runTables()
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		member := runAI(0)
		member.ID = string(want.AI)
		res := home.Result{Rows: []home.Row{home.RowOf(member)}, Items: []home.Item{inboxGoldenToHome(want)}}
		if want.Jump {
			res.Jump = []int64{want.Item}
		}
		return res
	})
	w, p := runFounded(t, tables)
	runExec(t, w.db, "INSERT INTO sqlite_sequence(name, seq) VALUES('inbox', ?)", want.Item-1)
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)
	p.does(groupID, "jump", ``)

	rawFrame := host.next()
	equal(t, proto.UnexpectedNulls([]byte(rawFrame)), []string{})
	frame := inboxDecode(t, rawFrame)
	equal(t, frame, proto.Inbox{Type: "inbox", Group: proto.ID(groupID), Items: []proto.InboxItem{{Item: want}}})
	stored := runQuery[string](t, w.db, "SELECT env FROM inbox WHERE id=?", want.Item)
	var compact map[string]any
	var gold map[string]any
	equal(t, json.Unmarshal([]byte(stored), &compact), nil)
	equal(t, json.Unmarshal(raw, &gold), nil)
	equal(t, compact, gold)
}

func inboxCheckRowCopies(t *testing.T) {
	msgID, goneID, taskID, missingID := id(60), id(61), id(62), id(63)
	env := browser.Sign(`{"t":"post","kind":"text"}`)
	msg := runMsg(msgID, 0, "check <a & b>")
	msg.Group, msg.Env = groupID, &env
	goneMsg := runMsg(goneID, 0, "forgotten")
	goneMsg.Group = groupID

	tables := runTables()
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		return home.Result{
			Rows: []home.Row{
				home.RowOf(msg), home.RowOf(goneMsg), home.RowOf(runGone("msg", goneID, 0)),
				home.RowOf(inboxCleanTask(taskID, 0, "Title <1 & 2>")),
			},
			Items: []home.Item{
				{AI: aiID, Why: "assign", Task: taskID, Msg: msgID},
				{AI: aiID, Why: "reviewed", Task: missingID, Msg: goneID},
				{AI: aiID, Why: "notice", Code: "quiet"},
			},
		}
	})
	tables.Commands["push"] = runFixed(func(s *home.State, c home.Command) home.Result {
		updated := inboxCleanTask(taskID, 0, "Updated title")
		updated.Step = "work"
		return home.Result{Rows: []home.Row{home.RowOf(updated)}}
	})
	w, p := runFounded(t, tables)
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)
	p.does(groupID, "jump", ``)

	raw := host.next()
	equal(t, strings.Contains(raw, "<a & b>") && strings.Contains(raw, "<1 & 2>"), true)
	frame := inboxDecode(t, raw)
	equal(t, len(frame.Items), 3)
	var copiedMsg proto.MsgRow
	equal(t, proto.Decode(frame.Items[0].Msg, &copiedMsg), nil)
	equal(t, []any{copiedMsg.ID, copiedMsg.Text, copiedMsg.Env, frame.Items[0].Task.Title}, []any{msgID, "check <a & b>", &env, "Title <1 & 2>"})
	equal(t, []any{frame.Items[1].Msg, frame.Items[1].Task}, []any{json.RawMessage(nil), (*proto.TaskRow)(nil)})
	equal(t, []any{frame.Items[2].Msg, frame.Items[2].Task}, []any{json.RawMessage(nil), (*proto.TaskRow)(nil)})

	// A later resend keeps the item decided when queued and copies the updated task row.
	p.does(groupID, "push", ``)
	w.loop.Inbox(host.conn)
	resent := inboxDecode(t, host.next())
	equal(t, []any{resent.Items[0].Item, resent.Items[0].Task.Step, resent.Items[0].Task.Seq}, []any{frame.Items[0].Item, "work", int64(8)})
}

// Multiple due items for one host are packed into the fewest inbox frames that
// fit in runRoom (61,376 bytes), in queue order, each within 61,440 bytes.
func TestInboxBatchesItemsToTheCtlLimit(t *testing.T) {
	inboxCheckBatchSplit(t, 61_376, []int{4, 1})
	inboxCheckBatchSplit(t, 61_377, []int{3, 2})
}

func inboxCheckBatchSplit(t *testing.T, quadBytes int, wantCounts []int) {
	t.Helper()
	ids := []string{id(71), id(72), id(73), id(74), id(75)}
	pad := 15_500
	firstThree := inboxSingleEntrySize(t, ids[0], 4, strings.Repeat("a", pad)) + 1 +
		inboxSingleEntrySize(t, ids[1], 5, strings.Repeat("a", pad)) + 1 +
		inboxSingleEntrySize(t, ids[2], 6, strings.Repeat("a", pad)) + 1
	pad4 := quadBytes - firstThree - inboxSingleEntrySize(t, ids[3], 7, "")

	tables := runTables()
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		texts := []string{strings.Repeat("a", pad), strings.Repeat("a", pad), strings.Repeat("a", pad), strings.Repeat("b", pad4), "e"}
		var res home.Result
		for i, msgID := range ids {
			m := runMsg(msgID, 0, texts[i])
			m.Group = groupID
			res.Rows = append(res.Rows, home.RowOf(m))
			res.Items = append(res.Items, home.Item{AI: aiID, Why: "mention", Msg: msgID})
		}
		return res
	})
	w, p := runFounded(t, tables)
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)
	p.does(groupID, "jump", ``)
	inboxAssertBatches(t, host, wantCounts)
}

func inboxAssertBatches(t *testing.T, host *runPeer, wantCounts []int) {
	t.Helper()
	var counts []int
	var order []int64
	for range len(wantCounts) {
		raw := host.next()
		if len(raw) > 61_440 {
			t.Fatalf("inbox frame of %d bytes exceeds ctl limit", len(raw))
		}
		frame := inboxDecode(t, raw)
		counts = append(counts, len(frame.Items))
		for _, entry := range frame.Items {
			order = append(order, entry.Item.Item)
		}
	}
	host.quiet()
	equal(t, counts, wantCounts)
	equal(t, order, []int64{1, 2, 3, 4, 5})
}

func inboxSingleEntrySize(t *testing.T, msgID string, seq int64, text string) int {
	t.Helper()
	msg := runMsg(msgID, seq, text)
	msg.Group = groupID
	entry := proto.InboxItem{
		Item: proto.Item{Item: seq - 3, AI: proto.ID(aiID), Why: "mention", Msg: proto.ID(msgID)},
		Msg:  json.RawMessage(runFrame(msg)),
	}
	return len(runFrame(entry))
}

// Ack deletes an item only when sent by the host of that item in a group homed here.
func TestInboxAckDeletesOnlyTheHostsOwnItem(t *testing.T) {
	otherAI := id(80)
	tables := runTables()
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		second := runAI(0)
		second.ID, second.Host = otherAI, node.ID()
		return home.Result{
			Rows:  []home.Row{home.RowOf(second)},
			Items: []home.Item{{AI: aiID, Why: "notice", Code: "quiet"}, {AI: aiID, Why: "notice", Code: "conflict"}, {AI: otherAI, Why: "notice", Code: "quiet"}},
		}
	})
	w, p := runFounded(t, tables)
	host, nodePeer := w.attach(laptop), w.attach(node)
	w.loop.Inbox(host.conn)
	p.does(groupID, "jump", ``)
	equal(t, len(inboxDecode(t, host.next()).Items), 2)

	// Non-homed group and another host leave item 1 intact.
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(id(99)), Item: 1})
	w.loop.Ack(nodePeer.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	w.idle()
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox"), 3)

	// Host acknowledges item 1; repeat ack is harmless; node acknowledges its own item 3.
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	w.loop.Ack(nodePeer.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 3})
	w.loop.Inbox(host.conn)
	resent := inboxDecode(t, host.next())
	equal(t, []any{len(resent.Items), resent.Items[0].Item.Item}, []any{1, int64(2)})
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox"), 1)

	// A store failure during Ack is reported, and Ack on a closed loop is ignored.
	runExec(t, w.db, "DROP TABLE inbox")
	w.loop.Ack(nodePeer.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 3})
	equal(t, strings.Contains(w.reported(), "group "+groupID+", its inbox: "), true)
	w.loop.Close()
	w.loop.Ack(nodePeer.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 3})
}

// The hourly wake-up cap holds items past wakes_per_hour until the oldest
// counted wake ages out, per AI, without holding or counting watch or reply items.
func TestInboxWakeUpCapHoldsAndReleasesItems(t *testing.T) {
	t.Run("cap holds excess items and exempts watch", inboxCheckCapAndWatch)
	t.Run("exact hour expiry and unsorted holds", inboxCheckHourBoundary)
	t.Run("rollback and restart", inboxCheckCapRollbackAndRestart)
	t.Run("people and replies are not held", inboxCheckCapSparesPeopleAndReplies)
}

func inboxCappedTables(capPerHour int, ai2 string) home.Tables {
	tables := runTables()
	tables.Commands["group/create"] = runFixed(func(s *home.State, c home.Command) home.Result {
		grp := runGroupRow(s.ID, "Website", 0)
		grp.Limits.WakesPerHour = capPerHour
		return home.Result{GroupsChanged: true, Rows: []home.Row{home.RowOf(grp), home.RowOf(runPerson(0)), home.RowOf(runAI(0))}}
	})
	tables.Commands["enroll"] = runFixed(func(s *home.State, c home.Command) home.Result {
		second := runAI(0)
		second.ID = ai2
		return home.Result{Rows: []home.Row{home.RowOf(second)}}
	})
	tables.Commands["jump"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		var req struct {
			Items []home.Item `json:"items"`
			Fail  bool        `json:"fail"`
		}
		if err := json.Unmarshal([]byte(c.Env.Payload), &req); err != nil {
			return home.Result{}, &home.Refusal{Code: "bad_request", Msg: err.Error()}
		}
		var writes []home.Write
		if req.Fail {
			writes = []home.Write{setting{"k", "v"}, setting{"k", "v"}}
		}
		return home.Result{Items: req.Items, Writes: writes}, nil
	}
	return tables
}

func inboxCheckCapAndWatch(t *testing.T) {
	ai2, taskID := id(81), id(82)
	w, p := runFounded(t, inboxCappedTables(2, ai2))
	p.does(groupID, "enroll", ``)
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)

	// A watch item before reaching the cap does not consume a wake slot: two mentions after it still go at once.
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"watch","Task":%q}]`, aiID, taskID))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Why, "watch")
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"mention","Msg":%q}]`, aiID, id(1)))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(2))

	w.clock.Store(clock + 1000)
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"mention","Msg":%q}]`, aiID, id(2)))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(3))

	// At the cap: a watch item and another AI still go immediately; two mentions for aiID wait in turn.
	w.clock.Store(clock + 2000)
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"watch","Task":%q},{"AI":%q,"Why":"mention","Msg":%q},{"AI":%q,"Why":"mention","Msg":%q},{"AI":%q,"Why":"mention","Msg":%q}]`,
		aiID, taskID, ai2, id(3), aiID, id(4), aiID, id(5)))
	immediate := inboxDecode(t, host.next())
	equal(t, []any{len(immediate.Items), immediate.Items[0].Item.Item, immediate.Items[1].Item.Item}, []any{2, int64(4), int64(5)})

	// Held at clock + 3,600,000 and clock + 3,601,000; not sent 1 ms early.
	w.loop.Tick(time.UnixMilli(clock + 3_600_000 - 1))
	host.quiet()
	w.loop.Tick(time.UnixMilli(clock + 3_600_000))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(6))
	w.loop.Tick(time.UnixMilli(clock + 3_601_000 - 1))
	host.quiet()
	w.loop.Tick(time.UnixMilli(clock + 3_601_000))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(7))
}

func inboxCheckHourBoundary(t *testing.T) {
	ai2 := id(81)
	w, _ := runFounded(t, inboxCappedTables(2, ai2))
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)

	// Explicit NotBefore out of order: first due at clock+5000, second due at clock+1000.
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet","NotBefore":%d}]`, aiID, clock+5000))
	w.clock.Store(clock + 1000)
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet"}]`, aiID))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(2))

	// 1 ms before clock+1000 ages out, a 3rd item is held until clock+1000+3,600,000; a longer caller hold stays longer.
	w.clock.Store(clock + 1000 + 3_600_000 - 1)
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet"},{"AI":%q,"Why":"notice","Code":"quiet","NotBefore":%d}]`,
		aiID, aiID, clock+9_000_000))
	equal(t, runQuery[int64](t, w.db, "SELECT not_before FROM inbox WHERE id=3"), int64(clock+1000+3_600_000))
	equal(t, runQuery[int64](t, w.db, "SELECT not_before FROM inbox WHERE id=4"), int64(clock+9_000_000))

	// At exact hour expiry (at == now - 3,600,000), the aged wake drops and a new item goes out immediately.
	w2, _ := runFounded(t, inboxCappedTables(1, ai2))
	host2 := w2.attach(laptop)
	w2.loop.Inbox(host2.conn)
	host2.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet"}]`, aiID))
	equal(t, inboxDecode(t, host2.next()).Items[0].Item.Item, int64(1))
	w2.clock.Store(clock + 3_600_000)
	host2.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet"}]`, aiID))
	equal(t, inboxDecode(t, host2.next()).Items[0].Item.Item, int64(2))
}

func inboxCheckCapRollbackAndRestart(t *testing.T) {
	ai2 := id(81)
	db := runOpen(t)
	tables := inboxCappedTables(1, ai2)
	w := runStart(t, db, tables)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)

	// A failed transaction leaves Wakes untouched.
	re := host.send(groupID, "jump", fmt.Sprintf(`,"fail":true,"items":[{"AI":%q,"Why":"notice","Code":"quiet"}]`, aiID))
	equal(t, strings.HasPrefix(host.next(), fmt.Sprintf(`{"re":%d,"err":{"code":"internal"`, re)), true)
	w.reported()

	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet"}]`, aiID))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(1))
	w.loop.Ack(host.conn, proto.Ack{Type: "ack", Group: proto.ID(groupID), Item: 1})
	w.idle()

	// Restarting the loop resets the in-memory wake count.
	w.loop.Close()
	w = runStart(t, db, tables)
	w.nonce.Store(10)
	host = w.attach(laptop)
	w.loop.Inbox(host.conn)
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"notice","Code":"quiet"}]`, aiID))
	equal(t, inboxDecode(t, host.next()).Items[0].Item.Item, int64(2))
}

// At the cap, a person's items and an AI's reply go at once and take no slot: a
// mention an AI causes is held, and goes out when the one counted mention ages out.
func inboxCheckCapSparesPeopleAndReplies(t *testing.T) {
	w, p := runFounded(t, inboxCappedTables(1, id(81)))
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)
	mention := func(n byte) string {
		return fmt.Sprintf(`,"items":[{"AI":%q,"Why":"mention","Msg":%q}]`, aiID, id(n))
	}
	gets := func(why string, item int64) {
		t.Helper()
		got := inboxDecode(t, host.next())
		equal(t, []any{len(got.Items), got.Items[0].Item.Why, got.Items[0].Item.Item}, []any{1, why, item})
	}

	p.does(groupID, "jump", mention(1))
	gets("mention", 1)
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"reply","Msg":%q}]`, aiID, id(2)))
	gets("reply", 2)
	w.clock.Store(clock + 1000)
	host.does(groupID, "jump", mention(3)) // the first that counts
	gets("mention", 3)

	w.clock.Store(clock + 2000)
	host.does(groupID, "jump", mention(4)) // held
	for _, why := range []string{"mention", "decided", "confirmed", "assign"} {
		p.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":%q,"Msg":%q}]`, aiID, why, id(5)))
		equal(t, inboxDecode(t, host.next()).Items[0].Item.Why, why)
	}
	host.does(groupID, "jump", fmt.Sprintf(`,"items":[{"AI":%q,"Why":"reply","Msg":%q}]`, aiID, id(6)))
	gets("reply", 9)

	w.loop.Tick(time.UnixMilli(clock + 1000 + 3_600_000 - 1))
	host.quiet()
	w.loop.Tick(time.UnixMilli(clock + 1000 + 3_600_000))
	gets("mention", 4)
}
