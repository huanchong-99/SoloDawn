package home_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// runFixed is a command that returns what make gives for it.
func runFixed(make func(s *home.State, c home.Command) home.Result) home.Func {
	return func(s *home.State, c home.Command) (home.Result, *home.Refusal) { return make(s, c), nil }
}

func runTask(id string, seq int64) proto.TaskRow {
	return proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: id, Seq: seq}, Num: 1, State: "active"}
}

func runGone(r, id string, seq int64) proto.GoneRow {
	return proto.GoneRow{RowHeader: proto.RowHeader{Type: r, ID: id, Seq: seq}, Gone: true}
}

// A client that reads with state_only is pushed the group, member, device
// and task rows, markers of those included, and no others; a client that
// does not watch the group is pushed no rows.
func TestRunPushesTheRowsEachClientAskedFor(t *testing.T) {
	tables := runTables()
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		return home.Result{Rows: []home.Row{
			home.RowOf(runTask("t1", 0)), home.RowOf(runNote("n1", 0, "a note")), home.RowOf(runGone("member", "m9", 0)),
			home.RowOf(runGone("note", "n0", 0)), home.RowOf(deviceRow(browser, browser, 4, "add", device(8))),
			home.RowOf(runMsg("m1", 0, "a message")), home.RowOf(runAI(0)), home.RowOf(runGroupRow(s.ID, "Site", 0)),
		}}
	})
	w, full := runFounded(t, tables)
	host, idle, guestPeer := w.attach(laptop), w.attach(laptop), w.attach(guest)
	guestPeer.syncRefused(groupID, "forbidden")
	full.watch(groupID, false)
	equal(t, host.watch(groupID, true), int64(3))

	re := full.send(groupID, "jump", ``)
	added := deviceRow(browser, browser, 4, "add", device(8))
	added.Seq = 8
	full.gets(runRows(groupID, runTask("t1", 4), runNote("n1", 5, "a note"), runGone("member", "m9", 6), runGone("note", "n0", 7),
		added, runMsg("m1", 9, "a message"), runAI(10), runGroupRow(groupID, "Site", 11)), fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	host.gets(runRows(groupID, runTask("t1", 4), runGone("member", "m9", 6), added, runAI(10), runGroupRow(groupID, "Site", 11)))

	// A result without a state row is no frame for a state_only reader.
	re = full.send(groupID, "note", `,"text":"only a note"`)
	full.gets(runRows(groupID, runNote(runID(w.nonce.Load()), 12, "only a note")), fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	host.quiet()
	idle.quiet()
	// The list of groups is pushed only for a result that says it changed,
	// and then to every connection, with the groups its device may read.
	guestPeer.quiet()
	full.does(runOther, "group", `,"op":"create","name":"Other"`)
	both := runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Site", Seq: 12}, proto.GroupSummary{Group: runOther, Name: "Other", Seq: 3})
	full.gets(both)
	host.gets(both)
	idle.gets(both)
	guestPeer.gets(runGroupsFrame())
}

// A connection is pushed rows and sent items only while its group serves
// its device (datachannel section 8). The result that ends that is not
// pushed to it either, and it does not watch again by itself when the group
// serves the device once more.
func TestRunStopsPushingToADeviceTheGroupNoLongerServes(t *testing.T) {
	tables := runTables()
	visitor := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: "visitor"}, Kind: "human", User: guest.ID()}, Role: "member"}
	results := map[string]home.Result{
		"joins":   {Rows: []home.Row{home.RowOf(visitor), home.RowOf(deviceRow(guest, guest, 1, "add", guest))}},
		"leaves":  {Rows: []home.Row{home.RowOf(runGone("member", "visitor", 0))}},
		"returns": {Rows: []home.Row{home.RowOf(visitor), home.RowOf(deviceRow(guest, guest, 1, "add", guest))}},
		"revokes": {Rows: []home.Row{home.RowOf(deviceRow(browser, browser, 4, "revoke", laptop))}},
		"wakes":   {Items: []home.Item{{AI: aiID, Why: "notice"}}},
	}
	tables.Commands["jump"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		var p struct {
			What string `json:"what"`
		}
		c.Decode(&p)
		return results[p.What], nil
	}
	w, p := runFounded(t, tables)
	host, visit := w.attach(laptop), w.attach(guest)
	host.watch(groupID, false)
	w.loop.Inbox(host.conn)
	if users := w.loop.LogUsers(visit.conn); len(users) != 0 {
		t.Fatalf("nonmember log users = %v", users)
	}
	p.does(groupID, "jump", `,"what":"joins"`)
	host.next()
	equal(t, visit.watch(groupID, false), int64(5))
	if users := w.loop.LogUsers(visit.conn); len(users) != 2 || !users[browser.ID()] || !users[guest.ID()] {
		t.Fatalf("member log users = %v", users)
	}
	p.does(groupID, "note", `,"text":"for everyone"`)
	note := runRows(groupID, runNote(runID(w.nonce.Load()), 6, "for everyone"))
	host.gets(note)
	visit.gets(note)

	// The visitor leaves the group: its connection is not pushed the row
	// that says so, nor anything after it, also not as a member again.
	p.does(groupID, "jump", `,"what":"leaves"`)
	host.gets(runRows(groupID, runGone("member", "visitor", 7)))
	visit.quiet()
	p.does(groupID, "jump", `,"what":"returns"`)
	host.next()
	p.does(groupID, "note", `,"text":"not for a connection that did not ask again"`)
	host.next()
	visit.quiet()
	equal(t, visit.watch(groupID, false), int64(10))

	// The laptop is revoked: its open connection gets no row and no item.
	p.does(groupID, "jump", `,"what":"wakes"`)
	equal(t, strings.HasPrefix(host.next(), `{"t":"inbox","g":"`+groupID+`","items":[{"item":{"item":1,`), true)
	p.does(groupID, "jump", `,"what":"revokes"`)
	visit.next()
	p.does(groupID, "note", `,"text":"not for a revoked device"`)
	visit.next()
	p.does(groupID, "jump", `,"what":"wakes"`)
	w.loop.Inbox(host.conn)
	host.quiet()
	host.syncRefused(groupID, "forbidden")
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox"), 2)
}

// A rows frame is filled only as far as a ctl message goes; the rest of a
// result follows in the next frame.
func TestRunSplitsTheRowsOfALargeResult(t *testing.T) {
	tables := runTables()
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		var res home.Result
		for i := range 5 {
			res.Rows = append(res.Rows, home.RowOf(runNote(fmt.Sprint("n", i), 0, strings.Repeat("<", 16000))))
		}
		return res
	})
	_, p := runFounded(t, tables)
	p.watch(groupID, false)
	re := p.send(groupID, "jump", ``)
	var counts []int
	for range 2 {
		frame := p.next()
		if len(frame) > 61440 {
			t.Fatalf("a rows frame of %d bytes", len(frame))
		}
		var rows proto.Rows
		equal(t, json.Unmarshal([]byte(frame), &rows), nil)
		counts = append(counts, len(rows.Rows))
	}
	equal(t, counts, []int{3, 2})
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	p.quiet()
}

// runWake is a command that queues one item for the AI of the tests, held
// back for hold milliseconds, beside a task row.
func runWake(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Hold int64  `json:"hold"`
		Why  string `json:"why"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	item := home.Item{AI: aiID, Why: p.Why, Task: "t1", Msg: string(c.Nonce), Commit: "9f2c1ab", PR: 7, Jump: true, Code: "c", Detail: "d"}
	if p.Hold != 0 {
		item.NotBefore = c.Now + p.Hold
	}
	return home.Result{Rows: []home.Row{home.RowOf(runTask("t1", 0))}, Items: []home.Item{item}}, nil
}

// An inbox item is queued in the transaction of its result and sent to the
// connection that takes the items of its host: after the row change that
// caused it, once per connection, again when the host says hello again, and
// not before its hold has ended.
func TestRunQueuesAndSendsInboxItems(t *testing.T) {
	tables := runTables()
	tables.Commands["jump"] = runWake
	tables.Commands["forget"] = runFixed(func(s *home.State, c home.Command) home.Result { return home.Result{Retire: []int64{2}} })
	w, p := runFounded(t, tables)
	host := w.attach(laptop)
	host.watch(groupID, true)
	items := map[int64]proto.Item{}
	inbox := func(seq int64, ids ...int64) string {
		task := runTask("t1", seq)
		batch := make([]proto.InboxItem, len(ids))
		for i, id := range ids {
			batch[i] = proto.InboxItem{Item: items[id], Task: &task}
		}
		return runFrame(proto.Inbox{Type: "inbox", Group: proto.ID(groupID), Items: batch})
	}
	wake := func(hold int64, seq int64) string {
		p.does(groupID, "jump", fmt.Sprintf(`,"hold":%d,"why":"mention"`, hold))
		n := int64(len(items) + 1)
		items[n] = proto.Item{
			Item: n, AI: proto.ID(aiID), Why: "mention", Task: "t1", Msg: proto.ID(runID(w.nonce.Load())),
			Commit: "9f2c1ab", PR: 7, Jump: true, Code: "c", Detail: "d",
		}
		return inbox(seq, n)
	}

	// Nobody takes the items yet: the first one waits in the store.
	wake(0, 4)
	host.gets(runRows(groupID, runTask("t1", 4)))
	var stored struct {
		g, ai, host, why, msg, task, commit string
		notBefore, created                  int64
	}
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT g,ai,host,why,msg,task,commit_id,not_before,created FROM inbox WHERE id=1").Scan(
			&stored.g, &stored.ai, &stored.host, &stored.why, &stored.msg, &stored.task, &stored.commit, &stored.notBefore, &stored.created)
	})
	equal(t, err, nil)
	equal(t, []any{stored.g, stored.ai, stored.host, stored.why, stored.msg, stored.task, stored.commit, stored.notBefore, stored.created},
		[]any{groupID, aiID, laptop.ID(), "mention", runID(w.nonce.Load()), "t1", "9f2c1ab", int64(0), int64(clock)})
	host.quiet()

	// The host says hello: it is sent what waits, and from then on each item
	// after the rows of its result. A connection of another device gets none.
	w.loop.Inbox(host.conn)
	w.loop.Inbox(p.conn)
	host.gets(inbox(4, 1))
	second := wake(0, 5)
	host.gets(runRows(groupID, runTask("t1", 5)), second)
	p.quiet()

	// A held item is sent when the clock reaches its time, and once.
	held := wake(1000, 6)
	host.gets(runRows(groupID, runTask("t1", 6)))
	w.loop.Tick(time.UnixMilli(clock + 999))
	host.quiet()
	w.loop.Tick(time.UnixMilli(clock + 1000))
	host.gets(held)
	w.loop.Tick(time.UnixMilli(clock + 2000))
	host.quiet()

	// On a hello every unacknowledged item that is due goes out again in one
	// inbox frame, oldest first; one that was acknowledged or retired does not.
	w.loop.Inbox(host.conn)
	host.gets(inbox(6, 1, 2))
	host.quiet()
	w.clock.Store(clock + 1000)
	w.loop.Inbox(host.conn)
	host.gets(inbox(6, 1, 2, 3))
	runExec(t, w.db, "DELETE FROM inbox WHERE id=1")
	p.does(groupID, "forget", ``)
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox"), 1)
	w.loop.Inbox(host.conn)
	host.gets(inbox(6, 3))
	// A second connection of the host takes them as well; the first is not
	// sent an item twice for that.
	again := w.attach(laptop)
	w.loop.Inbox(again.conn)
	again.gets(inbox(6, 3))
	host.quiet()

	// An item in the store that cannot be read is reported and left out, once
	// for a connection; the others are sent.
	last := wake(0, 7)
	host.gets(runRows(groupID, runTask("t1", 7)), last)
	again.gets(last)
	env := runQuery[string](t, w.db, "SELECT env FROM inbox WHERE id=3")
	runExec(t, w.db, "UPDATE inbox SET env='x' WHERE id=3")
	w.loop.Inbox(host.conn)
	equal(t, strings.HasPrefix(w.reported(), "group "+groupID+", its inbox: item 3: invalid character 'x'"), true)
	host.gets(last)
	w.loop.Tick(time.UnixMilli(clock + 1000))
	host.quiet()
	again.quiet()
	equal(t, len(w.reports), 0)
	runExec(t, w.db, "UPDATE inbox SET env=? WHERE id=3", env)
	w.loop.Inbox(host.conn)
	host.gets(inbox(7, 3, 4))

	// A queue that cannot be read is reported, and read again on the next tick.
	runExec(t, w.db, "DROP TABLE inbox")
	w.loop.Inbox(host.conn)
	equal(t, strings.Contains(w.reported(), "group "+groupID+", its inbox: "), true)
	w.loop.Tick(time.UnixMilli(clock + 999))
	host.quiet()
	equal(t, len(w.reports), 0)
	w.loop.Tick(time.UnixMilli(clock + 1000))
	equal(t, strings.Contains(w.reported(), "group "+groupID+", its inbox: "), true)
}

// A held item whose time has come waits for the tick: a result that queues
// nothing starts no delivery.
func TestRunSendsAHeldItemOnATick(t *testing.T) {
	tables := runTables()
	tables.Commands["jump"] = runWake
	w, p := runFounded(t, tables)
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)
	p.does(groupID, "jump", `,"hold":500,"why":"mention"`)
	w.clock.Store(clock + 600)
	p.does(groupID, "note", `,"text":"no item"`)
	host.quiet()
	w.loop.Tick(time.UnixMilli(clock + 600))
	equal(t, strings.HasPrefix(host.next(), `{"t":"inbox","g":"`+groupID+`","items":[{"item":{"item":1,"ai":"`+aiID+`","why":"mention"`), true)
}

// An item without the fields its why does not have is stored without them,
// and an item for an AI that its own result adds goes to that AI's host.
func TestRunStoresAnItemAsItsResultLeavesIt(t *testing.T) {
	tables := runTables()
	hired, tablet := runID(77), device(8)
	tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result {
		ai := runAI(0)
		ai.ID, ai.Host = hired, tablet.ID()
		other := runAI(0)
		other.ID, other.Host = runID(78), guest.ID()
		return home.Result{Rows: []home.Row{home.RowOf(ai), home.RowOf(other)}, Items: []home.Item{{AI: hired, Why: "notice"}}}
	})
	w, p := runFounded(t, tables)
	p.does(groupID, "jump", ``)
	var empty int
	var host string
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT host, (msg IS NULL)+(task IS NULL)+(commit_id IS NULL) FROM inbox").Scan(&host, &empty)
	})
	equal(t, []any{err, host, empty}, []any{nil, tablet.ID(), 3})
}

// State.Wakes holds, by AI, when each item of the last hour is due: the
// home's time when its result was committed, or its hold when that is later.
func TestRunCountsTheItemsOfTheLastHour(t *testing.T) {
	tables := runTables()
	var seen runRecord
	tables.Commands["jump"] = runWake
	tables.Commands["forget"] = runFixed(func(s *home.State, c home.Command) home.Result {
		seen.add("%v", s.Wakes)
		return home.Result{}
	})
	w, p := runFounded(t, tables)
	host := w.attach(laptop)
	w.loop.Inbox(host.conn)
	p.does(groupID, "jump", `,"hold":0,"why":"mention"`)
	host.next()
	p.does(groupID, "jump", `,"hold":1000,"why":"mention"`)
	// Sending an item again counts nothing.
	w.loop.Inbox(host.conn)
	host.next()
	w.clock.Store(clock + 3600_000 - 1)
	p.does(groupID, "forget", ``)
	p.does(groupID, "jump", `,"hold":0,"why":"mention"`)
	p.does(groupID, "forget", ``)
	// What is an hour old is dropped when the next item is counted.
	w.clock.Store(clock + 3600_000)
	p.does(groupID, "jump", `,"hold":0,"why":"mention"`)
	p.does(groupID, "forget", ``)
	equal(t, seen.all(), []string{
		fmt.Sprintf("map[%s:[%d %d]]", aiID, clock, clock+1000),
		fmt.Sprintf("map[%s:[%d %d %d]]", aiID, clock, clock+1000, clock+3600_000-1),
		fmt.Sprintf("map[%s:[%d %d %d]]", aiID, clock+1000, clock+3600_000-1, clock+3600_000),
	})
}

// A client that stops reading never holds the group: the frames wait for
// it, up to 1024 of them, and then the connection is given up and closed,
// whichever frame finds no room.
func TestRunGivesUpAClientThatStopsReading(t *testing.T) {
	for name, last := range map[string]func(w *runWorld, p, slow *runPeer){
		"a rows frame": func(w *runWorld, p, slow *runPeer) {
			re := p.send(groupID, "note", `,"text":"x"`)
			p.next()
			p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, re))
		},
		"a groups frame": func(w *runWorld, p, slow *runPeer) {
			p.does(groupID, "jump", ``)
			p.next()
		},
		"a welcome": func(w *runWorld, p, slow *runPeer) { w.loop.Welcome(slow.conn, runWelcome) },
	} {
		t.Run(name, func(t *testing.T) {
			tables := runTables()
			var online runRecord
			tables.After.Add("online", func(s *home.State, res home.Result) { online.add("%v", s.Online[laptop.ID()]) })
			tables.Commands["jump"] = runFixed(func(s *home.State, c home.Command) home.Result { return home.Result{GroupsChanged: true} })
			w, p := runFounded(t, tables)
			inner := runNewClient()
			client := &runClosing{runClient: inner, closed: make(chan struct{})}
			slow := w.attachClient(laptop, client, inner)
			slow.watch(groupID, false)
			<-inner.started
			inner.hold = make(chan struct{})
			p.watch(groupID, false)
			note := func(n int) {
				for range n {
					re := p.send(groupID, "note", `,"text":"x"`)
					p.next()
					p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, re))
				}
			}
			w.loop.Welcome(slow.conn, runWelcome)
			runWait(t, inner.started) // the welcome is with the client, which does not return
			note(1024)
			select {
			case <-slow.conn.Gone():
				t.Fatal("the connection was given up while its frames still had room")
			default:
			}
			last(w, p, slow)
			runWait(t, slow.conn.Gone())
			runWait(t, client.closed)
			note(1)
			seen := online.all()
			equal(t, []string{seen[1024], seen[len(seen)-1]}, []string{"true", "false"})
			// The frames that waited are dropped.
			close(inner.hold)
			slow.gets(runWelcomeFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 3}))
			time.Sleep(20 * time.Millisecond)
			equal(t, len(inner.got), 0)
			w.loop.Sync(slow.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 99}, Group: groupID, Since: 3})
			equal(t, len(inner.got), 0)
		})
	}
}

// A client that refuses a frame is given up as well, whether it can be
// closed or not. A client that is done, or that the node detaches, is not
// closed by the loop.
func TestRunEndsAConnection(t *testing.T) {
	tables := runTables()
	var online runRecord
	tables.After.Add("online", func(s *home.State, res home.Result) { online.add("%v", s.Online[laptop.ID()]) })
	w, p := runFounded(t, tables)
	closing := func() (*runPeer, *runClosing) {
		c := &runClosing{runClient: runNewClient(), closed: make(chan struct{})}
		peer := w.attachClient(laptop, c, c.runClient)
		peer.watch(groupID, false)
		return peer, c
	}
	open := func(c *runClosing) {
		t.Helper()
		time.Sleep(20 * time.Millisecond)
		select {
		case <-c.closed:
			t.Fatal("the loop closed the client")
		default:
		}
	}
	note := func() {
		p.does(groupID, "note", `,"text":"x"`)
	}

	refusing, refused := closing()
	plain := w.attach(laptop)
	plain.watch(groupID, false)
	refused.refuse.Store(true)
	plain.client.refuse.Store(true)
	note()
	runWait(t, refusing.conn.Gone())
	runWait(t, refused.closed)
	runWait(t, plain.conn.Gone())

	// Two connections of one device: the device is online until both are gone.
	done, finished := closing()
	left, detached := closing()
	note()
	close(finished.done)
	runWait(t, done.conn.Gone())
	open(finished)
	note()
	left.next()
	left.next()
	w.loop.Detach(left.conn)
	w.loop.Detach(left.conn)
	runWait(t, left.conn.Gone())
	note()
	w.loop.Inbox(left.conn)
	w.idle()
	open(detached)
	equal(t, online.all()[1:], []string{"true", "true", "true", "false"})
	// Nothing was sent after the detach.
	equal(t, len(detached.got), 0)
}

// The timers run for every group on a tick, in the order of their names,
// each in a transaction of its own; an event runs its handler. Their
// results are written, committed and pushed like a command's.
func TestRunRunsTheTimersAndTheEvents(t *testing.T) {
	tables := runTables()
	var seen, hooks runRecord
	var mode atomic.Value
	mode.Store("")
	tables.After.Add("seen", func(s *home.State, res home.Result) { hooks.add("%d rows at %d", len(res.Rows), s.Seq) })
	tables.Timers.Add("b-reads", func(s *home.State, d home.Disk, now int64) home.Result {
		_, found := d.Note("n-" + fmt.Sprint(now))
		seen.add("b %s at %d: seq %d, note %v", s.Group.Name, now, s.Seq, found)
		return home.Result{}
	})
	tables.Timers.Add("a-writes", func(s *home.State, d home.Disk, now int64) home.Result {
		seen.add("a %s at %d: seq %d", s.Group.Name, now, s.Seq)
		switch mode.Load() {
		case "note":
			return home.Result{Rows: []home.Row{home.RowOf(runNote("n-"+fmt.Sprint(now), 0, "from a timer"))}}
		case "large":
			return home.Result{Rows: []home.Row{home.RowOf(runNote("large", 0, strings.Repeat("x", 17000)))}}
		case "broken":
			return home.Result{Rows: []home.Row{home.RowOf(runNote("lost", 0, "x"))}, Writes: []home.Write{setting{"k", "v"}, setting{"k", "v"}}}
		}
		return home.Result{}
	})
	tables.Events.Add("closed", func(s *home.State, d home.Disk, now int64, event any) home.Result {
		_, found := d.Note("n-" + fmt.Sprint(clock+1))
		seen.add("event %v at %d, note %v", event, now, found)
		return home.Result{Rows: []home.Row{home.RowOf(runNote("from-event", 0, fmt.Sprint(event)))}}
	})
	w, p := runFounded(t, tables)
	p.watch(groupID, false)

	w.loop.Tick(time.UnixMilli(clock))
	p.quiet()
	equal(t, hooks.all(), []string{"3 rows at 3"})

	mode.Store("note")
	w.loop.Tick(time.UnixMilli(clock + 1))
	note := runNote("n-"+fmt.Sprint(clock+1), 4, "from a timer")
	p.gets(runRows(groupID, note))
	equal(t, runQuery[int64](t, w.db, "SELECT mt FROM row WHERE id=?", note.ID), int64(clock+1))

	// A result that is refused, and one that cannot be written, change nothing.
	mode.Store("large")
	w.loop.Tick(time.UnixMilli(clock + 2))
	equal(t, w.reported(), "group "+groupID+", a rule: bad_request: the command would write a row of 17119 bytes, above its limit of 16384")
	mode.Store("broken")
	w.loop.Tick(time.UnixMilli(clock + 3))
	equal(t, strings.HasPrefix(w.reported(), "group "+groupID+", a rule: "), true)
	mode.Store("")
	p.quiet()

	equal(t, w.loop.Event("opened", "x"), false)
	equal(t, w.loop.Event("closed", "the laptop"), true)
	late := runNote("from-event", 5, "the laptop")
	p.gets(runRows(groupID, late))
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row"), 5)
	equal(t, hooks.all(), []string{"3 rows at 3", "1 rows at 4", "1 rows at 5"})
	equal(t, seen.all(), []string{
		fmt.Sprintf("a Website at %d: seq 3", clock), fmt.Sprintf("b Website at %d: seq 3, note false", clock),
		fmt.Sprintf("a Website at %d: seq 3", clock+1), fmt.Sprintf("b Website at %d: seq 4, note true", clock+1),
		fmt.Sprintf("a Website at %d: seq 4", clock+2), fmt.Sprintf("b Website at %d: seq 4, note false", clock+2),
		fmt.Sprintf("a Website at %d: seq 4", clock+3), fmt.Sprintf("b Website at %d: seq 4, note false", clock+3),
		fmt.Sprintf("event the laptop at %d, note true", clock),
	})
}

// A group that does not exist has no timers and no events.
func TestRunAGroupThatDoesNotExistHasNoRules(t *testing.T) {
	tables := runTables()
	var seen runRecord
	entered, release := make(chan struct{}), make(chan struct{})
	tables.Commands["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		entered <- struct{}{}
		<-release
		return home.Result{}, &home.Refusal{Code: "conflict", Msg: "not this time"}
	}
	tables.Timers.Add("timer", func(s *home.State, d home.Disk, now int64) home.Result { seen.add("timer"); return home.Result{} })
	tables.Events.Add("event", func(s *home.State, d home.Disk, now int64, event any) home.Result {
		seen.add("event")
		return home.Result{}
	})
	w := runStart(t, runOpen(t), tables)
	p, other := w.attach(browser), w.attach(laptop)
	re := p.send(groupID, "group", runCreateWebsite)
	runWait(t, entered)
	w.loop.Tick(time.UnixMilli(clock))
	w.loop.Event("event", nil)
	w.loop.Detach(other.conn)
	close(release)
	p.gets(fmt.Sprintf(`{"re":%d,"err":{"code":"conflict","msg":"not this time"}}`, re))
	// The tick and the event were handed in before this command.
	re = p.send(groupID, "group", runCreateWebsite)
	runWait(t, entered)
	p.gets(fmt.Sprintf(`{"re":%d,"err":{"code":"conflict","msg":"not this time"}}`, re))
	equal(t, seen.all(), []string{})
	equal(t, w.loop.Groups(p.conn), []proto.GroupSummary{})
}

// With Every the loop runs the timers by itself.
func TestRunTicksByItsClock(t *testing.T) {
	tables := runTables()
	var once atomic.Bool
	tables.Timers.Add("once", func(s *home.State, d home.Disk, now int64) home.Result {
		if once.Swap(true) {
			return home.Result{}
		}
		return home.Result{Rows: []home.Row{home.RowOf(runNote("ticked", 0, "by the clock"))}}
	})
	once.Store(true)
	_, p := runFounded(t, tables, func(cfg *home.LoopConfig) { cfg.Every = time.Millisecond })
	p.watch(groupID, false)
	once.Store(false)
	p.gets(runRows(groupID, runNote("ticked", 4, "by the clock")))
}

// The hooks run in the order of their names, after the store and the state
// hold the result and before the answer is sent. A hook may hand work to the
// loop.
func TestRunCallsTheHooksAfterTheCommit(t *testing.T) {
	tables := runTables()
	var seen runRecord
	var w *runWorld
	var p *runPeer
	tables.Events.Add("noted", func(s *home.State, d home.Disk, now int64, event any) home.Result {
		seen.add("event %v", event)
		return home.Result{}
	})
	tables.After.Add("b", func(s *home.State, res home.Result) {
		note, ok := res.Rows[0].Value().(proto.NoteRow)
		if !ok {
			return
		}
		seen.add("b: seq %d, stored %d, online %v", s.Seq, runQuery[int](w.t, w.db, "SELECT count(*) FROM row WHERE id=?", note.ID), s.Online[browser.ID()])
		if note.Text == "first" {
			seen.add("%d frames sent", len(p.client.got))
			w.loop.Cmd(p.conn, 50, w.cmd(browser, groupID, "note", `,"text":"from a hook"`))
			w.loop.Event("noted", note.Text)
			w.loop.Tick(time.UnixMilli(clock))
			w.loop.Inbox(p.conn)
			seen.add("groups %d", len(w.loop.Groups(p.conn)))
		}
	})
	tables.After.Add("a", func(s *home.State, res home.Result) { seen.add("a: %d rows", len(res.Rows)) })
	w, p = runFounded(t, tables)
	p.does(groupID, "note", `,"text":"first"`)
	p.gets(`{"re":50,"ok":true}`)
	w.idle()
	equal(t, seen.all(), []string{
		"a: 3 rows", "a: 1 rows", "b: seq 4, stored 1, online true", "0 frames sent", "groups 1",
		"a: 1 rows", "b: seq 5, stored 1, online true", "event first",
	})
}

// A result is not idle for any one thing it holds: each reaches the hooks.
func TestRunAResultThatHoldsAnythingIsCommitted(t *testing.T) {
	tables := runTables()
	var hooks runRecord
	tables.After.Add("seen", func(s *home.State, res home.Result) {
		hooks.add("%d %d %d %d %v", len(res.Rows), len(res.Items), len(res.Retire), len(res.Writes), res.GroupsChanged)
	})
	results := map[string]home.Result{
		"nothing": {}, "an item": {Items: []home.Item{{AI: aiID, Why: "notice"}}}, "a retirement": {Retire: []int64{9}},
		"a write": {Writes: []home.Write{setting{"k", "v"}}}, "the groups": {GroupsChanged: true},
	}
	tables.Commands["jump"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		var p struct {
			What string `json:"what"`
		}
		c.Decode(&p)
		return results[p.What], nil
	}
	w, p := runFounded(t, tables)
	for _, what := range []string{"nothing", "an item", "a retirement", "a write"} {
		p.does(groupID, "jump", `,"what":"`+what+`"`)
	}
	p.does(groupID, "jump", `,"what":"the groups"`)
	p.gets(runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 3}))
	equal(t, hooks.all(), []string{"3 0 0 0 true", "0 1 0 0 false", "0 0 1 0 false", "0 0 0 1 false", "0 0 0 0 true"})
	equal(t, runQuery[string](t, w.db, "SELECT value FROM setting WHERE key='k'"), "v")
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM inbox"), 1)
}

// A client may call the loop and the store while it is sent a frame: the
// frames of a connection are sent by a goroutine of its own, which holds
// nothing the group or the store's writer needs.
func TestRunAClientMayCallBackWhileItIsSent(t *testing.T) {
	w, p := runFounded(t, runTables())
	client := runNewClient()
	var left atomic.Int64
	left.Store(40)
	var reader *runPeer
	client.during = func(frame any) {
		r, rows := frame.(proto.Rows)
		if !rows || left.Add(-1) < 0 {
			return
		}
		runExec(t, w.db, "INSERT INTO setting(key, value) VALUES(?, 'x')", fmt.Sprint(left.Load()))
		w.loop.Groups(reader.conn)
		var last proto.RowHeader
		equal(t, json.Unmarshal(r.Rows[len(r.Rows)-1], &last), nil)
		w.loop.Sync(reader.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 99}, Group: groupID, Since: last.Seq})
		w.loop.Cmd(reader.conn, 100, w.cmd(laptop, groupID, "note", `,"text":"again"`))
	}
	reader = w.attachClient(laptop, client, client)
	reader.watch(groupID, false)
	p.does(groupID, "note", `,"text":"first"`)
	equal(t, strings.HasPrefix(reader.next(), `{"t":"rows"`), true)
	for range 40 {
		synced, rows, answer := reader.next(), reader.next(), reader.next()
		equal(t, []bool{strings.HasPrefix(synced, `{"t":"synced","re":99,`), strings.HasPrefix(rows, `{"t":"rows"`), answer == `{"re":100,"ok":true}`}, []bool{true, true, true})
	}
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row WHERE r='note'"), 41)
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM setting"), 40)
}

// A closed loop takes nothing more: a connection attached to it is gone at
// once, and its entries do nothing.
func TestRunACloseEndsEverything(t *testing.T) {
	w, p := runFounded(t, runTables())
	w.idle()
	p.send(groupID, "note", `,"text":"before the close"`)
	w.loop.Close()
	w.loop.Close()
	runWait(t, p.conn.Gone())
	// The group finished what it was given.
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row WHERE r='note'"), 1)

	client := runNewClient()
	late := w.attachClient(browser, client, client)
	runWait(t, late.conn.Gone())
	w.loop.Welcome(late.conn, runWelcome)
	late.send(groupID, "note", `,"text":"after the close"`)
	late.send(runOther, "group", runCreateWebsite)
	w.loop.Tick(time.UnixMilli(clock))
	w.loop.Inbox(late.conn)
	w.loop.Detach(late.conn)
	equal(t, w.loop.Event("none", nil), false)
	w.loop.Sync(late.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 99}, Group: groupID})
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row"), 4)
	equal(t, len(late.client.got), 0)
}

// Close waits for an in-flight Client.Send on attached and detached connections.
func TestRunCloseWaitsForConnectionSends(t *testing.T) {
	checkCloseWaitsForSend(t, false)
	checkCloseWaitsForSend(t, true)
}

func checkCloseWaitsForSend(t *testing.T, detachFirst bool) {
	t.Helper()
	db, err := store.Open(":memory:")
	equal(t, err, nil)
	loop, err := home.StartLoop(home.LoopConfig{Store: db, Home: node.ID(), Owner: browser.ID(), Logs: runLogs(t), Tables: runTables()})
	equal(t, err, nil)

	var sentDone atomic.Bool
	client := runNewClient()
	client.hold = make(chan struct{})
	client.during = func(any) {
		runExec(t, db, "INSERT INTO setting(key, value) VALUES('during', '1')")
		sentDone.Store(true)
	}
	conn := loop.Attach(browser.ID(), client)
	idle := loop.Attach(laptop.ID(), runNewClient())
	loop.Welcome(conn, runWelcome)
	runWait(t, client.started)
	if detachFirst {
		loop.Detach(conn)
	}

	closed := make(chan struct{}) // single reader: checkCloseWaitsForSend
	go func() {
		loop.Close()
		db.Close()
		close(closed)
	}()
	runWait(t, idle.Gone())
	select {
	case <-closed:
		t.Fatal("Close returned while Client.Send was still running")
	default:
	}
	close(client.hold)
	runWait(t, closed)
	equal(t, sentDone.Load(), true)
}
