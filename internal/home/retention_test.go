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
	err := db.Write(func(tx *store.Tx) error {
		if _, err := tx.NextSeq(group); err != nil {
			return err
		}
		return store.Put(tx, group, mt, row)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func retRowIDs(t *testing.T, db *store.Store, group string) string {
	t.Helper()
	return runQuery[string](t, db, "SELECT coalesce(group_concat(id, ',' ORDER BY seq), '') FROM row WHERE g=? AND r NOT IN ('group', 'member')", group)
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
	w.loop.Watch(p.conn, groupID, false)

	// A non-retention Write reaches After before the first tick and must not
	// mark retention as having run at time 0.
	p.does(groupID, "push", ``)

	cut90 := time.UnixMilli(start).AddDate(0, 0, -90).UnixMilli()
	retPutRow(t, w.db, groupID, cut90, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "old-90", Seq: 6}, Pos: 6, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cut90 - 1, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
	retPutRow(t, w.db, groupID, cut90, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "edge-90", Seq: 7}, Pos: 7, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cut90, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})

	w.loop.Tick(time.UnixMilli(start))
	p.quiet()
	equal(t, retRowIDs(t, w.db, groupID), "edge-90")

	// Before 24 hours have passed, Tick is a no-op; at 24 hours it runs again.
	nextDay := start + retDay
	cutNext := time.UnixMilli(nextDay).AddDate(0, 0, -90).UnixMilli()
	retPutRow(t, w.db, groupID, cutNext, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "wait-24h", Seq: 8}, Pos: 8, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cutNext - 1, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})

	w.loop.Tick(time.UnixMilli(start + retDay - 1))
	p.settled(groupID)
	equal(t, retRowIDs(t, w.db, groupID), "edge-90,wait-24h")

	w.loop.Tick(time.UnixMilli(nextDay))
	p.settled(groupID)
	equal(t, retRowIDs(t, w.db, groupID), "")

	// RetainEvent runs outside the 24-hour wait and advances the last-run mark.
	eventAt := nextDay + 5_000
	w.clock.Store(eventAt)
	cutEvent := time.UnixMilli(eventAt).AddDate(0, 0, -90).UnixMilli()
	retPutRow(t, w.db, groupID, cutEvent, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "event-expired", Seq: 9}, Pos: 9, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cutEvent - 1, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
	equal(t, w.loop.Event(home.RetainEvent, nil), true)
	p.settled(groupID)
	equal(t, retRowIDs(t, w.db, groupID), "")

	afterEvent := eventAt + retDay
	cutAfter := time.UnixMilli(afterEvent).AddDate(0, 0, -90).UnixMilli()
	retPutRow(t, w.db, groupID, cutAfter, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "after-event", Seq: 10}, Pos: 10, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cutAfter - 1, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
	w.loop.Tick(time.UnixMilli(eventAt + retDay - 1))
	p.settled(groupID)
	equal(t, retRowIDs(t, w.db, groupID), "after-event")

	w.loop.Tick(time.UnixMilli(afterEvent))
	p.settled(groupID)
	equal(t, retRowIDs(t, w.db, groupID), "")
}

// A failed retention run leaves the schedule unmarked so the next tick retries,
// refuses non-integer and non-positive retention_days values while accepting 1,
// and surfaces query and Retain errors.
func TestRetentionValidationAndRollbackPurity(t *testing.T) {
	const start = int64(1_000)
	w := runStart(t, runOpen(t), retTables(nil))
	w.clock.Store(start)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()

	cut1 := time.UnixMilli(start+500).AddDate(0, 0, -1).UnixMilli()
	retPutRow(t, w.db, groupID, cut1, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "old-1d", Seq: 6}, Pos: 6, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cut1 - 1, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
	retPutRow(t, w.db, groupID, cut1, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "edge-1d", Seq: 7}, Pos: 7, Group: groupID,
		By: ownerID, Kind: "text", Timestamp: cut1, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})

	for _, c := range []struct {
		raw  string
		want string
	}{
		{`null`, "setting retention_days:"},
		{`7.5`, "setting retention_days:"},
		{`"7"`, "setting retention_days:"},
		{`{`, "setting retention_days:"},
		{`0`, "setting retention_days must be at least 1, got 0"},
		{`-3`, "setting retention_days must be at least 1, got -3"},
	} {
		retSetSetting(t, w.db, "retention_days", c.raw)
		w.loop.Tick(time.UnixMilli(start))
		if got := w.reported(); !strings.Contains(got, c.want) {
			t.Fatalf("for %s reported %q; want %q", c.raw, got, c.want)
		}
		equal(t, retRowIDs(t, w.db, groupID), "old-1d,edge-1d")
	}

	// Fixing retention_days to 1 lets the next tick succeed within the same day
	// because none of the rolled-back runs reached After.
	retSetSetting(t, w.db, "retention_days", `1`)
	w.loop.Tick(time.UnixMilli(start + 500))
	p.settled(groupID)
	equal(t, retRowIDs(t, w.db, groupID), "edge-1d")
}

// RetainWrite propagates errors from reading setting and from tx.Retain.
func TestRetentionStoreErrors(t *testing.T) {
	const start = int64(1_000)
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

// A2.5, A5.14 and R3-4 across two groups: pending cards, notes, review
// messages and a confirmed brief with a queued task survive retention; the
// purge mark advances only after gone markers age out; finished tasks are
// deleted from the store and remain in State.Tasks until restart.
func TestRetentionKeepsProtectedRowsAdvancesPurgeMarkAndKeepsMemoryTasksUntilRestart(t *testing.T) {
	var seenTasks []string
	var seenBrief bool
	var seenCause proto.Envelope
	tables := retTables(map[string]home.Func{
		"task/inspect": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
			var p struct {
				Brief string `json:"brief"`
			}
			if refusal := c.Decode(&p); refusal != nil {
				return home.Result{}, refusal
			}
			seenTasks = slices.Sorted(maps.Keys(s.Tasks))
			card, ok := c.Disk.Msg(p.Brief)
			seenBrief = ok
			if ok && card.State != nil && card.State.Act != nil {
				seenCause = *card.State.Act
			}
			return home.Result{}, nil
		},
		"task/seed": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
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
		},
		"task/release": func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
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
		},
	})

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

	// Post and confirm two briefs at base: briefQueued has a dependent second
	// task (queued), briefActive has one active task only.
	briefQueued, confirmQueued := retPostAndConfirmBrief(t, w, p, host, true)
	briefActive, _ := retPostAndConfirmBrief(t, w, p, host, false)

	// Seed a merged, a cancelled and an active task at base so they are in
	// State.Tasks as well as SQLite.
	p.does(groupID, "task", `,"op":"seed","id":"t-merged","num":10,"state":"merged"`)
	p.does(groupID, "task", `,"op":"seed","id":"t-cancelled","num":11,"state":"cancelled"`)
	p.does(groupID, "task", `,"op":"seed","id":"t-active","num":12,"state":"active"`)
	p.settled(groupID)

	day8 := base + 8*retDay
	cut8 := time.UnixMilli(day8).AddDate(0, 0, -7).UnixMilli()
	old := cut8 - 1

	// Seed messages, cards, notes, review ledger and gone markers at exact
	// retention boundaries.
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
	oldMarkerSeq := nextSeq
	retPutRow(t, db, groupID, old, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-old", Seq: oldMarkerSeq}, Gone: true})
	nextSeq++
	freshMarkerSeq := nextSeq
	retPutRow(t, db, groupID, cut8, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "gone-cut", Seq: freshMarkerSeq}, Gone: true})

	// Second group also gets an expired message and marker so concurrent group
	// ticks exercise retMu under -race.
	retPutRow(t, db, runOther, old, proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "other-old", Seq: 6}, Pos: 6, Group: runOther,
		By: ownerID, Kind: "text", Timestamp: old, Mentions: []string{}, Body: json.RawMessage(`{}`),
	})
	retPutRow(t, db, runOther, old, proto.GoneRow{RowHeader: proto.RowHeader{Type: "note", ID: "other-gone", Seq: 7}, Gone: true})

	// Keep the loop's State.Seq synchronized with the direct store rows.
	w.loop.Close()
	w = runStart(t, db, tables)
	w.clock.Store(day8)
	w.nonce.Store(100)
	p = w.attach(browser)

	w.loop.Tick(time.UnixMilli(day8))
	w.idle()

	// Check purge marks on both groups: g advanced to oldMarkerSeq (not
	// freshMarkerSeq, whose mt == cut8); runOther advanced to 7.
	view, err := db.Snapshot(groupID)
	equal(t, err, nil)
	equal(t, view.Purged, oldMarkerSeq)
	view.Close()
	equal(t, retRowIDs(t, db, runOther), "")
	equal(t, runQuery[int64](t, db, "SELECT purged FROM grp WHERE g=?", runOther), int64(7))

	// Check retained rows in SQLite: briefQueued kept, briefActive deleted,
	// pending card / review / note / cut-boundary message / fresh marker /
	// active+queued tasks kept, and merged+cancelled tasks deleted from SQLite.
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id=?", groupID, briefQueued), 1)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id=?", groupID, briefActive), 0)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id IN ('msg-old','card-decided','gone-old','t-merged','t-cancelled')", groupID), 0)
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id IN ('msg-cut','card-pending','msg-review','note-keep','gone-cut','t-active')", groupID), 6)

	// While the loop is still running, finished tasks stay in State.Tasks, and
	// Disk.Msg still returns the confirmed brief with its exact signed act.
	p.does(groupID, "task", fmt.Sprintf(`,"op":"inspect","brief":%q`, briefQueued))
	equal(t, seenBrief, true)
	equal(t, seenCause, confirmQueued)
	equal(t, slices.Contains(seenTasks, "t-merged"), true)
	equal(t, slices.Contains(seenTasks, "t-cancelled"), true)
	equal(t, slices.Contains(seenTasks, "t-active"), true)

	// After restart, State.Tasks no longer holds the expired finished tasks,
	// while active and brief tasks remain and grp.num is preserved.
	w.loop.Close()
	w = runStart(t, db, tables)
	w.clock.Store(day8)
	w.nonce.Store(200)
	p = w.attach(browser)
	p.does(groupID, "task", fmt.Sprintf(`,"op":"inspect","brief":%q`, briefQueued))
	equal(t, slices.Contains(seenTasks, "t-merged"), false)
	equal(t, slices.Contains(seenTasks, "t-cancelled"), false)
	equal(t, slices.Contains(seenTasks, "t-active"), true)
	equal(t, runQuery[int64](t, db, "SELECT num FROM grp WHERE g=?", groupID), int64(12))

	// Release the queued dependent task; on the next day's run briefQueued has
	// no queued task left and is deleted, and gone-cut ages out and raises purged.
	var firstID, queuedID string
	for _, tid := range seenTasks {
		state := runQuery[string](t, db, "SELECT data->>'state' FROM row WHERE g=? AND r='task' AND id=?", groupID, tid)
		brief := runQuery[string](t, db, "SELECT coalesce(data->>'brief', '') FROM row WHERE g=? AND r='task' AND id=?", groupID, tid)
		if brief == briefQueued && state == "active" {
			firstID = tid
		}
		if brief == briefQueued && state == "queued" {
			queuedID = tid
		}
	}
	p.does(groupID, "task", fmt.Sprintf(`,"op":"release","first":%q,"queued":%q`, firstID, queuedID))

	day9 := day8 + retDay
	w.clock.Store(day9)
	w.loop.Tick(time.UnixMilli(day9))
	w.idle()
	equal(t, runQuery[int](t, db, "SELECT count(*) FROM row WHERE g=? AND id=?", groupID, briefQueued), 0)
	equal(t, runQuery[int64](t, db, "SELECT purged FROM grp WHERE g=?", groupID), freshMarkerSeq)
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
