package home_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const retDay = int64(86_400_000)

var (
	retWorkOne = id(21)
	retWorkTwo = id(22)
)

// retTables copies the node's command table and adds test commands while
// keeping the package's real Timers, Events and After maps.
func retTables(extra map[string]home.Func) home.Tables {
	cmds := maps.Clone(home.Commands)
	cmds["group/create"] = retCreateGroup
	cmds["post/text"] = runPost
	cmds["note"] = runWriteNote
	for k, f := range extra {
		cmds[k] = f
	}
	return home.Tables{Commands: cmds, Timers: home.Timers, Events: home.Events, After: home.After}
}

func retCreateGroup(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Name string `json:"name"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Kind: "project",
		Home: s.Home, Secretary: aiID, Forge: "none", Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	one := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: retWorkOne}, Kind: "ai", User: browser.ID(), Name: "one"}, Host: laptop.ID()}
	two := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: retWorkTwo}, Kind: "ai", User: browser.ID(), Name: "two"}, Host: laptop.ID()}
	return home.Result{
		GroupsChanged: true,
		Rows:          []home.Row{home.RowOf(grp), home.RowOf(runPerson(0)), home.RowOf(runAI(0)), home.RowOf(one), home.RowOf(two)},
	}, nil
}

func retSetSetting(t *testing.T, db *store.Store, key, value string) {
	t.Helper()
	runExec(t, db, "INSERT INTO setting(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
}

func retPutRow[R interface {
	proto.GroupRow | proto.HumanMemberRow | proto.AIMemberRow | proto.DeviceRow |
		proto.TaskRow | proto.MsgRow | proto.NoteRow | proto.GoneRow
}](t *testing.T, db *store.Store, group string, mt int64, row R) {
	t.Helper()
	writeRow := func(tx *store.Tx) error {
		if _, err := tx.NextSeq(group); err != nil {
			return err
		}
		return store.Put(tx, group, mt, row)
	}
	if err := db.Write(writeRow); err != nil {
		t.Fatal(err)
	}
}

func retRowIDs(t *testing.T, db *store.Store, group string) string {
	t.Helper()
	return runQuery[string](t, db, "SELECT coalesce(group_concat(id, ',' ORDER BY seq), '') FROM row WHERE g=? AND r NOT IN ('group', 'member')", group)
}

func retPutMsgAt(t *testing.T, db *store.Store, group, msgID string, seq, mt, ts int64) {
	t.Helper()
	retPutRow(t, db, group, mt, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: seq}, Pos: seq, Group: group,
		By: ownerID, Kind: "text", Timestamp: ts, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
}

// The daily timer runs on the first tick of a state (including when now is
// below 24 hours from the epoch), uses the default 90 days when retention_days
// is absent, ignores non-retention writes in After, does nothing at +86,399,999
// ms, runs again at +86,400,000 ms, and runs on demand through RetainEvent.
func TestRetentionDailyBoundaryAndOnDemand(t *testing.T) {
	const start = int64(1_000)
	tables := retTables(map[string]home.Func{
		"push": func(_ *home.State, _ home.Command) (home.Result, *home.Refusal) {
			return home.Result{Writes: []home.Write{setting{"other_setting", `"kept"`}}}, nil
		},
	})
	w := runStart(t, runOpen(t), tables)
	w.clock.Store(start)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()
	p.watch(groupID, false)

	// A non-retention Write reaches After before the first tick and does not
	// mark retention as having run at time 0.
	p.does(groupID, "push", ``)

	cut90 := time.UnixMilli(start).AddDate(0, 0, -90).UnixMilli()
	retPutMsgAt(t, w.db, groupID, "old-90", 6, cut90, cut90-1)
	retPutMsgAt(t, w.db, groupID, "edge-90", 7, cut90, cut90)

	w.loop.Tick(time.UnixMilli(start))
	p.quiet()
	equal(t, retRowIDs(t, w.db, groupID), "edge-90")

	// Before 24 hours have passed, Tick is a no-op; at 24 hours it runs again.
	nextDay := start + retDay
	cutNext := time.UnixMilli(nextDay).AddDate(0, 0, -90).UnixMilli()
	retPutMsgAt(t, w.db, groupID, "wait-24h", 8, cutNext, cutNext-1)

	w.loop.Tick(time.UnixMilli(start + retDay - 1))
	w.idle()
	equal(t, retRowIDs(t, w.db, groupID), "edge-90,wait-24h")

	w.loop.Tick(time.UnixMilli(nextDay))
	w.idle()
	equal(t, retRowIDs(t, w.db, groupID), "")

	// RetainEvent runs outside the 24-hour wait and advances the last-run mark.
	eventAt := nextDay + 5_000
	w.clock.Store(eventAt)
	cutEvent := time.UnixMilli(eventAt).AddDate(0, 0, -90).UnixMilli()
	retPutMsgAt(t, w.db, groupID, "event-expired", 9, cutEvent, cutEvent-1)
	retPutMsgAt(t, w.db, groupID, "event-edge", 10, cutEvent, cutEvent)
	equal(t, w.loop.Event(home.RetainEvent, nil), true)
	w.idle()
	equal(t, retRowIDs(t, w.db, groupID), "event-edge")

	afterEvent := eventAt + retDay
	cutAfter := time.UnixMilli(afterEvent).AddDate(0, 0, -90).UnixMilli()
	retPutMsgAt(t, w.db, groupID, "after-event", 11, cutAfter, cutAfter-1)
	w.loop.Tick(time.UnixMilli(eventAt + retDay - 1))
	w.idle()
	equal(t, retRowIDs(t, w.db, groupID), "event-edge,after-event")

	w.loop.Tick(time.UnixMilli(afterEvent))
	w.idle()
	equal(t, retRowIDs(t, w.db, groupID), "")
}

// A rolled-back retention run leaves the schedule unmarked so the next tick
// retries within the same day, and RetainWrite propagates setting and Retain
// errors.
func TestRetentionRollbackAndStoreErrors(t *testing.T) {
	const start = int64(1_000)
	w := runStart(t, runOpen(t), retTables(nil))
	w.clock.Store(start)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()

	retSetSetting(t, w.db, "retention_days", `1`)
	cut1 := time.UnixMilli(start+500).AddDate(0, 0, -1).UnixMilli()
	retPutMsgAt(t, w.db, groupID, "old-1d", 6, cut1, cut1-1)
	retPutMsgAt(t, w.db, groupID, "edge-1d", 7, cut1, cut1)

	runExec(t, w.db, "ALTER TABLE peer RENAME TO peer_off")
	w.loop.Tick(time.UnixMilli(start))
	if got := w.reported(); !strings.Contains(got, "no such table: peer") {
		t.Fatalf("reported %q; want missing peer error", got)
	}
	equal(t, retRowIDs(t, w.db, groupID), "old-1d,edge-1d")

	// Restoring the table lets the next tick succeed within the same day because
	// the rolled-back run never reached After.
	runExec(t, w.db, "ALTER TABLE peer_off RENAME TO peer")
	w.loop.Tick(time.UnixMilli(start + 500))
	w.idle()
	equal(t, retRowIDs(t, w.db, groupID), "edge-1d")

	dbNoInbox := runOpen(t)
	retSetSetting(t, dbNoInbox, "retention_days", `7`)
	runExec(t, dbNoInbox, "DROP TABLE inbox")
	err := dbNoInbox.Write(home.RetainWrite(start).Put)
	if err == nil || !strings.Contains(err.Error(), "no such table: inbox") {
		t.Fatalf("got %v; want missing inbox error", err)
	}

	dbNoSetting := runOpen(t)
	runExec(t, dbNoSetting, "DROP TABLE setting")
	err = dbNoSetting.Write(home.RetainWrite(start).Put)
	if err == nil || !strings.Contains(err.Error(), "no such table: setting") {
		t.Fatalf("got %v; want missing setting error", err)
	}
}

type retInspectState struct {
	tasks []string
	brief bool
	cause proto.Envelope
}

func retInspectorTables(out *retInspectState) home.Tables {
	return retTables(map[string]home.Func{
		"task/inspect": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
			var p struct {
				Brief string `json:"brief"`
			}
			if refusal := c.Decode(&p); refusal != nil {
				return home.Result{}, refusal
			}
			out.tasks = slices.Sorted(maps.Keys(s.Tasks))
			card, ok := c.Disk.Msg(p.Brief)
			out.brief = ok
			if ok && card.State != nil && card.State.Act != nil {
				out.cause = *card.State.Act
			}
			return home.Result{}, nil
		},
		"task/seed":    retSeedTaskCmd,
		"task/release": retReleaseCmd,
	})
}

func retSeedTaskCmd(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		ID    string `json:"id"`
		Num   int    `json:"num"`
		State string `json:"state"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	row := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: p.ID}, Group: s.ID,
		Num: p.Num, Title: p.ID, Slug: proto.MakeSlug(p.ID), Origin: "direct",
		State: p.State, Assignee: retWorkOne, Reviewer: retWorkTwo, Pass: 90,
		Depends: []string{}, Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
	}
	return home.Result{Rows: []home.Row{home.RowOf(row)}}, nil
}

func retReleaseCmd(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		First  string `json:"first"`
		Queued string `json:"queued"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	first := s.Tasks[p.First]
	first.Step, first.Commit = "merge", "c1"
	merged, _, err := home.Step(first, home.Event{Kind: home.OnMerged, By: home.ByHost, Now: c.Now, Commit: "c1"})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "merge failed"}
	}
	released, _, err := home.Step(s.Tasks[p.Queued], home.Event{Kind: home.OnReleased, By: home.ByHome, Now: c.Now})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "release failed"}
	}
	return home.Result{Rows: []home.Row{home.RowOf(merged), home.RowOf(released)}}, nil
}

// A2.5, A5.14 and R3-4 across two groups: pending cards, notes, review
// messages and a confirmed brief with a queued task survive retention; the
// purge mark advances only after gone markers age out and resets a Sync below
// purged; finished tasks leave SQLite and remain in State.Tasks until restart.
func TestRetentionProtectedRowsSyncResetAndMemoryTasks(t *testing.T) {
	var seen retInspectState
	tables := retInspectorTables(&seen)
	db := runOpen(t)
	retSetSetting(t, db, "retention_days", `7`)
	w := runStart(t, db, tables)
	base := int64(clock)
	w.clock.Store(base)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()
	p.does(runOther, "group", `,"op":"create","name":"Second"`)
	p.next()
	host := w.attach(laptop)

	briefQueued, confirmQueued := retPostAndConfirmBrief(t, w, p, host, true)
	briefActive, _ := retPostAndConfirmBrief(t, w, p, host, false)
	p.does(groupID, "task", `,"op":"seed","id":"t-merged","num":10,"state":"merged"`)
	p.does(groupID, "task", `,"op":"seed","id":"t-cancelled","num":11,"state":"cancelled"`)
	p.does(groupID, "task", `,"op":"seed","id":"t-active","num":12,"state":"active"`)
	w.idle()

	day8 := base + 8*retDay
	cut8 := time.UnixMilli(day8).AddDate(0, 0, -7).UnixMilli()
	oldMarkerSeq, freshMarkerSeq := retSeedBoundaryRows(t, db, cut8)

	w.loop.Close()
	w = runStart(t, db, tables)
	w.clock.Store(day8)
	w.nonce.Store(100)
	p = w.attach(browser)

	w.loop.Tick(time.UnixMilli(day8))
	w.idle()

	retAssertDay8StoreAndSync(t, w, db, briefQueued, briefActive, oldMarkerSeq)

	// While the loop is running, finished tasks stay in State.Tasks, and
	// Disk.Msg still returns the confirmed brief with its exact signed act.
	p.does(groupID, "task", fmt.Sprintf(`,"op":"inspect","brief":%q`, briefQueued))
	equal(t, seen.brief, true)
	equal(t, seen.cause, confirmQueued)
	equal(t, slices.Contains(seen.tasks, "t-merged"), true)
	equal(t, slices.Contains(seen.tasks, "t-cancelled"), true)
	equal(t, slices.Contains(seen.tasks, "t-active"), true)

	// After restart, State.Tasks drops the expired finished tasks and keeps grp.num.
	w.loop.Close()
	w = runStart(t, db, tables)
	w.clock.Store(day8)
	w.nonce.Store(200)
	p = w.attach(browser)
	p.does(groupID, "task", fmt.Sprintf(`,"op":"inspect","brief":%q`, briefQueued))
	equal(t, slices.Contains(seen.tasks, "t-merged"), false)
	equal(t, slices.Contains(seen.tasks, "t-cancelled"), false)
	equal(t, slices.Contains(seen.tasks, "t-active"), true)
	equal(t, runQuery[int64](t, db, "SELECT num FROM grp WHERE g=?", groupID), int64(12))

	firstID, queuedID := retFindBriefPair(t, db, seen.tasks, briefQueued)
	p.does(groupID, "task", fmt.Sprintf(`,"op":"release","first":%q,"queued":%q`, firstID, queuedID))

	day9 := day8 + retDay
	w.clock.Store(day9)
	w.loop.Tick(time.UnixMilli(day9))
	w.idle()
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id=?", groupID, briefQueued), 0)
	equal(t, runQuery[int64](t, db, "SELECT purged FROM grp WHERE g=?", groupID), freshMarkerSeq)
}

func retSeedBoundaryRows(t *testing.T, db *store.Store, cut8 int64) (oldMarkerSeq, freshMarkerSeq int64) {
	t.Helper()
	old := cut8 - 1
	nextSeq := runQuery[int64](t, db, "SELECT seq FROM grp WHERE g=?", groupID)
	addMsg := func(msgID, kind, status string, ts int64) {
		nextSeq++
		var state *proto.CardState
		if status != "" {
			state = &proto.CardState{Status: status}
		}
		retPutRow(t, db, groupID, ts, proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: msgID, Seq: nextSeq}, Pos: nextSeq, Group: groupID,
			By: ownerID, Kind: kind, Timestamp: ts, Mentions: []string{}, Body: json.RawMessage(`{}`), State: state,
		})
	}
	addMsg("msg-old", "text", "", old)
	addMsg("msg-cut", "text", "", cut8)
	addMsg("card-pending", "decision", "pending", old)
	addMsg("card-decided", "decision", "decided", old)
	addMsg("msg-review", "review", "", old)

	nextSeq++
	retPutRow(t, db, groupID, old, proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: "note-keep", Seq: nextSeq}, Pos: nextSeq,
		Group: groupID, Text: "remember", By: ownerID, Timestamp: old,
	})
	nextSeq++
	oldMarkerSeq = nextSeq
	retPutRow(t, db, groupID, old, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-old", Seq: oldMarkerSeq}, Gone: true})
	nextSeq++
	freshMarkerSeq = nextSeq
	retPutRow(t, db, groupID, cut8, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-cut", Seq: freshMarkerSeq}, Gone: true})

	retPutMsgAt(t, db, runOther, "other-old", 6, old, old)
	retPutRow(t, db, runOther, old, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "other-gone", Seq: 7}, Gone: true})
	return oldMarkerSeq, freshMarkerSeq
}

func retAssertDay8StoreAndSync(t *testing.T, w *runWorld, db *store.Store, briefQueued, briefActive string, oldMarkerSeq int64) {
	t.Helper()
	view, err := db.Snapshot(groupID)
	equal(t, err, nil)
	equal(t, view.Purged, oldMarkerSeq)
	view.Close()
	equal(t, retRowIDs(t, db, runOther), "")
	equal(t, runQuery[int64](t, db, "SELECT purged FROM grp WHERE g=?", runOther), int64(7))

	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id=?", groupID, briefQueued), 1)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id=?", groupID, briefActive), 0)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id IN ('msg-old','card-decided','gone-old','t-merged','t-cancelled')", groupID), 0)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id IN ('msg-cut','card-pending','msg-review','note-keep','gone-cut','t-active')", groupID), 6)

	equal(t, retSyncResets(t, w, oldMarkerSeq-1), true)
	equal(t, retSyncResets(t, w, oldMarkerSeq), false)
}

func retSyncResets(t *testing.T, w *runWorld, since int64) bool {
	t.Helper()
	reader := w.attach(browser)
	defer w.loop.Detach(reader.conn)
	reader.re++
	w.loop.Sync(reader.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: reader.re}, Group: groupID, Since: since})
	reset := false
	for {
		var head struct {
			Type string `json:"t"`
			Re   int64  `json:"re"`
		}
		if err := json.Unmarshal([]byte(reader.next()), &head); err != nil {
			t.Fatal(err)
		}
		if head.Type == "reset" {
			reset = true
		}
		if head.Type == "synced" && head.Re == reader.re {
			return reset
		}
	}
}

func retFindBriefPair(t *testing.T, db *store.Store, taskIDs []string, briefID string) (firstID, queuedID string) {
	t.Helper()
	for _, tid := range taskIDs {
		state := runQuery[string](t, db, "SELECT data->>'state' FROM row WHERE g=? AND r='task' AND id=?", groupID, tid)
		brief := runQuery[string](t, db, "SELECT coalesce(data->>'brief', '') FROM row WHERE g=? AND r='task' AND id=?", groupID, tid)
		if brief == briefID && state == "active" {
			firstID = tid
		}
		if brief == briefID && state == "queued" {
			queuedID = tid
		}
	}
	return firstID, queuedID
}

func retPostAndConfirmBrief(t *testing.T, w *runWorld, owner, host *runPeer, withDep bool) (string, proto.Envelope) {
	t.Helper()
	tasks := []proto.BriefTask{{Title: "First step", Assignee: retWorkOne, Reviewer: retWorkTwo, Depends: []int{}}}
	if withDep {
		tasks = append(tasks, proto.BriefTask{Title: "Second step", Assignee: retWorkTwo, Reviewer: retWorkOne, Depends: []int{0}})
	}
	briefNonce := runID(w.nonce.Add(1))
	post := proto.Post[proto.BriefBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: proto.ID(briefNonce), Timestamp: w.clock.Load(),
			Group: proto.ID(groupID), As: proto.ID(aiID),
		},
		Kind: "brief", Mentions: []proto.ID{},
		Body: proto.BriefBody{
			Title: "Plan", Goal: "Goal", Criteria: []string{"Done"},
			Scope: proto.BriefScope{In: []string{"home"}, Out: []string{}},
			Tasks: tasks, Pass: 90, Requester: ownerID,
		},
	}
	payload, err := json.Marshal(post)
	if err != nil {
		t.Fatal(err)
	}
	host.re++
	w.loop.Cmd(host.conn, host.re, identity.Key(laptop).Sign(string(payload)))
	equal(t, host.next(), fmt.Sprintf(`{"re":%d,"ok":true}`, host.re))

	actEnv := browser.Sign(fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":"confirm"}`,
		runID(w.nonce.Add(1)), w.clock.Load(), groupID, briefNonce))
	owner.re++
	w.loop.Cmd(owner.conn, owner.re, actEnv)
	equal(t, owner.next(), fmt.Sprintf(`{"re":%d,"ok":true}`, owner.re))
	return briefNonce, actEnv
}
