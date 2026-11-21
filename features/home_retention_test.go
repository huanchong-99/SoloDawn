//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_retention"] = func(sc *godog.ScenarioContext, _ fixtures) { homeRetentionSteps(sc) }
}

const homeRetDay = 24 * time.Hour

type homeRetWorld struct {
	today      time.Time
	now        time.Time
	db         *store.Store
	loop       *home.Loop
	cleanup    func(func())
	browser    identity.Key
	node       identity.Key
	laptop     identity.Key
	ownerConn  *homeRunPeer
	hostConn   *homeRunPeer
	group      string
	ownerID    string
	secID      string
	aiOne      string
	aiTwo      string
	nonce      int
	markers    map[string]int64
	briefKept  string
	briefGone  string
	confirmAct proto.Envelope
	released   []home.Item
	memTasks   int
	memActive  bool
	memNum     int
}

func homeRetentionSteps(sc *godog.ScenarioContext) {
	w := &homeRetWorld{
		today:   time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC),
		markers: map[string]int64{},
	}
	w.cleanup = afterScenario(sc)
	sc.Step(`^a group home with retention set to (\d+) days$`, w.open)
	sc.Step(`^(\d+) days ago the group received the text message "([^"]*)", the pending decision card "([^"]*)" and the decided decision card "([^"]*)"$`, w.seedMessagesAndCards)
	sc.Step(`^(\d+) days ago the group received the text message "([^"]*)"$`, w.seedTextMessage)
	sc.Step(`^the daily retention job runs today$`, func() error { return w.tickAt(w.today) })
	sc.Step(`^the store holds the messages "([^"]*)"$`, w.storeHoldsMessages)
	sc.Step(`^a second retention tick on the same day changes nothing$`, w.secondTickNoOp)
	sc.Step(`^(\d+) days ago the group received the note "([^"]*)" and the review message "([^"]*)"$`, w.seedMemoryRows)
	sc.Step(`^a note "([^"]*)" from (\d+) days ago was forgotten (\d+) days ago$`, w.seedForgottenNote)
	sc.Step(`^the store still holds the note "([^"]*)" and the review message "([^"]*)"$`, w.storeHoldsMemoryRows)
	sc.Step(`^the deletion marker for "([^"]*)" is gone while the deletion marker for "([^"]*)" remains$`, w.checkMarkers)
	sc.Step(`^the group's purge mark equals the sequence number of the removed marker for "([^"]*)"$`, w.checkPurgeMark)
	sc.Step(`^a client cursor older than that removed marker is below the purge mark while a cursor at it is not$`, w.checkCursorAgainstPurgeMark)
	sc.Step(`^(\d+) days ago the owner confirmed a brief with a queued dependent task and a second brief without a queued task$`, w.seedConfirmedBriefs)
	sc.Step(`^the store still holds the brief with the queued task and has deleted the brief without a queued task$`, w.checkBriefsAfterFirstRun)
	sc.Step(`^the dependency merges and releases the queued task$`, w.mergeAndRelease)
	sc.Step(`^the released task's assign item carries the exact confirmation of its brief$`, w.checkReleasedCause)
	sc.Step(`^another day passes and the daily retention job runs again$`, func() error { return w.tickAt(w.today.Add(homeRetDay)) })
	sc.Step(`^the store has deleted the brief that had the queued task$`, w.checkQueuedBriefDeleted)
	sc.Step(`^(\d+) days ago the group had an active task, a merged task and a cancelled task$`, w.seedThreeTasks)
	sc.Step(`^the store holds only the active task$`, w.storeHoldsOnlyActiveTask)
	sc.Step(`^the running home still holds all (\d+) tasks in memory$`, w.memoryHoldsCount)
	sc.Step(`^the home restarts on the same store$`, w.restart)
	sc.Step(`^the running home holds only the active task in memory and keeps the task counter at (\d+)$`, w.memoryAfterRestart)
}

func (w *homeRetWorld) open(days int) error {
	w.browser = exampleKey("ret browser")
	w.node = exampleKey("ret node")
	w.laptop = exampleKey("ret laptop")
	w.group = string(exampleID("ret group"))
	w.ownerID = string(exampleID("ret owner"))
	w.secID = string(exampleID("ret secretary"))
	w.aiOne = string(exampleID("ret ai one"))
	w.aiTwo = string(exampleID("ret ai two"))
	w.now = w.today.Add(-10 * homeRetDay)

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db
	if err := w.exec("INSERT INTO setting(key, value) VALUES('retention_days', ?)", strconv.Itoa(days)); err != nil {
		return err
	}
	if err := w.startLoop(); err != nil {
		return err
	}
	return w.foundGroup()
}

func (w *homeRetWorld) startLoop() error {
	tables := home.Tables{
		Commands: w.commands(),
		Timers:   home.Timers,
		Events:   home.Events,
		After:    home.After,
	}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: w.node.ID(), Owner: w.browser.ID(),
		Logs: func(user string) *identity.Log {
			log, _ := homeLog([]identity.Key{w.browser, w.node, w.laptop})
			if log == nil || log.User != user {
				return nil
			}
			return log
		},
		Tables: tables,
		Now:    func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	w.cleanup(loop.Close)
	w.loop = loop
	w.ownerConn = &homeRunPeer{frames: make(chan any, 64)}
	w.ownerConn.conn = loop.Attach(w.browser.ID(), w.ownerConn)
	w.hostConn = &homeRunPeer{frames: make(chan any, 64)}
	w.hostConn.conn = loop.Attach(w.laptop.ID(), w.hostConn)
	return nil
}

func (w *homeRetWorld) commands() home.Table {
	cmds := maps.Clone(home.Commands)
	cmds["group/create"] = w.cmdCreate
	cmds["post/seed"] = w.cmdSeedMsg
	cmds["note"] = w.cmdNote
	cmds["forget"] = w.cmdForget
	cmds["task/seed"] = w.cmdSeedTask
	cmds["task/release"] = w.cmdRelease
	cmds["task/inspect"] = w.cmdInspect
	return cmds
}

func (w *homeRetWorld) cmdCreate(s *home.State, _ home.Command) (home.Result, *home.Refusal) {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Website", Kind: "project",
		Home: s.Home, Secretary: w.secID, Forge: "none", Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	owner := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.ownerID}, Group: s.ID, Kind: "human", User: w.browser.ID(), Name: "owner"}, Role: "owner"}
	sec := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.secID}, Group: s.ID, Kind: "ai", User: w.browser.ID(), Name: "secretary"}, Host: w.laptop.ID()}
	one := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.aiOne}, Group: s.ID, Kind: "ai", User: w.browser.ID(), Name: "ai-one"}, Host: w.laptop.ID()}
	two := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.aiTwo}, Group: s.ID, Kind: "ai", User: w.browser.ID(), Name: "ai-two"}, Host: w.laptop.ID()}
	return home.Result{
		GroupsChanged: true,
		Rows:          []home.Row{home.RowOf(grp), home.RowOf(owner), home.RowOf(sec), home.RowOf(one), home.RowOf(two)},
	}, nil
}

func (w *homeRetWorld) cmdSeedMsg(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		ID      string `json:"id"`
		MsgKind string `json:"msg_kind"`
		Text    string `json:"text"`
		Status  string `json:"status"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	var state *proto.CardState
	if p.Status != "" {
		state = &proto.CardState{Status: p.Status}
	}
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: p.ID}, Group: s.ID, By: c.Who.Member(),
		Kind: p.MsgKind, Timestamp: c.Now, Text: p.Text, Mentions: []string{},
		Body: json.RawMessage(`{}`), State: state,
	}
	return home.Result{Rows: []home.Row{home.RowOf(msg)}}, nil
}

func (w *homeRetWorld) cmdNote(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	note := proto.NoteRow{
		RowHeader: proto.RowHeader{Type: "note", ID: p.ID}, Group: s.ID,
		Text: p.Text, By: c.Who.Member(), Timestamp: c.Now,
	}
	return home.Result{Rows: []home.Row{home.RowOf(note)}}, nil
}

func (w *homeRetWorld) cmdForget(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		R  string `json:"r"`
		ID string `json:"id"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	gone := proto.GoneRow{RowHeader: proto.RowHeader{Type: p.R, ID: p.ID}, Gone: true}
	return home.Result{Rows: []home.Row{home.RowOf(gone)}}, nil
}

func (w *homeRetWorld) cmdSeedTask(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		ID    string `json:"id"`
		Num   int    `json:"num"`
		State string `json:"state"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	row := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: p.ID}, Group: s.ID, Num: p.Num,
		Title: p.ID, Slug: proto.MakeSlug(p.ID), Origin: "direct", State: p.State,
		Assignee: w.aiOne, Reviewer: w.aiTwo, Pass: 90, Depends: []string{},
		Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
	}
	return home.Result{Rows: []home.Row{home.RowOf(row)}}, nil
}

func (w *homeRetWorld) cmdRelease(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	first, dependent := w.findBriefTasks(s)
	first.Step, first.Commit = "merge", "c1"
	merged, _, err := home.Step(first, home.Event{Kind: home.OnMerged, By: home.ByHost, Now: c.Now, Commit: "c1"})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "merge refused"}
	}
	released, fx, err := home.Step(dependent, home.Event{Kind: home.OnReleased, By: home.ByHome, Now: c.Now})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "release refused"}
	}
	items, refusal := releaseItems(c.Disk, released, fx.Items)
	if refusal != nil {
		return home.Result{}, refusal
	}
	w.released = items
	return home.Result{Rows: []home.Row{home.RowOf(merged), home.RowOf(released)}, Items: items}, nil
}

func (w *homeRetWorld) findBriefTasks(s *home.State) (first, dependent proto.TaskRow) {
	for _, task := range s.Tasks {
		if task.Brief != w.briefKept {
			continue
		}
		if task.State == "active" {
			first = task
		} else {
			dependent = task
		}
	}
	return first, dependent
}

func releaseItems(d home.Disk, released proto.TaskRow, raw []home.TaskItem) ([]home.Item, *home.Refusal) {
	card, ok := d.Msg(released.Brief)
	if !ok || card.State == nil || card.State.Act == nil {
		return nil, &home.Refusal{Code: "not_found", Msg: "confirmed brief missing"}
	}
	act := *card.State.Act
	var items []home.Item
	for _, it := range raw {
		items = append(items, home.Item{AI: it.AI, Why: it.Why, Task: released.ID, Msg: released.ID, Cause: &act})
	}
	return items, nil
}

func (w *homeRetWorld) cmdInspect(s *home.State, _ home.Command) (home.Result, *home.Refusal) {
	w.memTasks = len(s.Tasks)
	_, w.memActive = s.Tasks["task-active"]
	w.memNum = s.Num
	return home.Result{}, nil
}

func (w *homeRetWorld) nextID() proto.ID {
	w.nonce++
	return exampleID("ret " + strconv.Itoa(w.nonce))
}

func (w *homeRetWorld) sendCmd(peer *homeRunPeer, signer identity.Key, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	peer.re++
	w.loop.Cmd(peer.conn, peer.re, signer.Sign(string(raw)))
	got, err := peer.next()
	want := fmt.Sprintf(`{"re":%d,"ok":true}`, peer.re)
	if err != nil || got != want {
		return fmt.Errorf("got %s (%v); want %s", got, err, want)
	}
	return nil
}

func (w *homeRetWorld) foundGroup() error {
	return w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "group", "op": "create", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
	})
}

func (w *homeRetWorld) seedMessagesAndCards(daysAgo int, textOld, pendingTitle, decidedTitle string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	for _, m := range []struct{ kind, text, status string }{
		{"text", textOld, ""},
		{"decision", pendingTitle, "pending"},
		{"decision", decidedTitle, "decided"},
	} {
		err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
			"t": "post", "kind": "seed", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
			"id": string(w.nextID()), "msg_kind": m.kind, "text": m.text, "status": m.status,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *homeRetWorld) seedTextMessage(daysAgo int, text string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	return w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "post", "kind": "seed", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
		"id": string(w.nextID()), "msg_kind": "text", "text": text, "status": "",
	})
}

func (w *homeRetWorld) tickAt(at time.Time) error {
	w.now = at
	w.loop.Tick(at)
	w.loop.Watch(w.ownerConn.conn, w.group, false)
	return nil
}

func (w *homeRetWorld) storeHoldsMessages(wantCSV string) error {
	var got string
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT coalesce(group_concat(data->>'text', ', ' ORDER BY seq), '') FROM row WHERE g=? AND r='msg' AND gone=0", w.group).Scan(&got)
	})
	if err != nil {
		return err
	}
	if got != wantCSV {
		return fmt.Errorf("store holds %q; want %q", got, wantCSV)
	}
	return nil
}

func (w *homeRetWorld) secondTickNoOp() error {
	if err := w.exec("UPDATE setting SET value='1' WHERE key='retention_days'"); err != nil {
		return err
	}
	if err := w.tickAt(w.today.Add(time.Hour)); err != nil {
		return err
	}
	return w.storeHoldsMessages("Still waiting, Six days ago")
}

func (w *homeRetWorld) seedMemoryRows(daysAgo int, noteText, reviewText string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	if err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "note", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
		"id": string(w.nextID()), "text": noteText,
	}); err != nil {
		return err
	}
	return w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "post", "kind": "seed", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
		"id": string(w.nextID()), "msg_kind": "review", "text": reviewText, "status": "",
	})
}

func (w *homeRetWorld) seedForgottenNote(label string, createdDaysAgo, forgottenDaysAgo int) error {
	noteID := string(w.nextID())
	w.now = w.today.Add(-time.Duration(createdDaysAgo) * homeRetDay)
	if err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "note", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
		"id": noteID, "text": label,
	}); err != nil {
		return err
	}
	w.now = w.today.Add(-time.Duration(forgottenDaysAgo) * homeRetDay)
	if err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "forget", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
		"r": "note", "id": noteID,
	}); err != nil {
		return err
	}
	var seq int64
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT seq FROM row WHERE g=? AND r='note' AND id=?", w.group, noteID).Scan(&seq)
	})
	w.markers[label] = seq
	return err
}

func (w *homeRetWorld) storeHoldsMemoryRows(noteText, reviewText string) error {
	var noteCount, reviewCount int
	err := w.db.Write(func(tx *store.Tx) error {
		if err := tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND r='note' AND gone=0 AND data->>'text'=?", w.group, noteText).Scan(&noteCount); err != nil {
			return err
		}
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND r='msg' AND gone=0 AND data->>'kind'='review' AND data->>'text'=?", w.group, reviewText).Scan(&reviewCount)
	})
	if err != nil {
		return err
	}
	if noteCount != 1 || reviewCount != 1 {
		return fmt.Errorf("notes=%d reviews=%d; want 1 and 1", noteCount, reviewCount)
	}
	return nil
}

func (w *homeRetWorld) checkMarkers(goneLabel, keptLabel string) error {
	goneSeq, keptSeq := w.markers[goneLabel], w.markers[keptLabel]
	var goneCount, keptCount int
	err := w.db.Write(func(tx *store.Tx) error {
		if err := tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND seq=?", w.group, goneSeq).Scan(&goneCount); err != nil {
			return err
		}
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND seq=? AND gone=1", w.group, keptSeq).Scan(&keptCount)
	})
	if err != nil {
		return err
	}
	if goneCount != 0 || keptCount != 1 {
		return fmt.Errorf("removed marker count=%d, kept marker count=%d", goneCount, keptCount)
	}
	return nil
}

func (w *homeRetWorld) checkPurgeMark(label string) error {
	view, err := w.db.Snapshot(w.group)
	if err != nil {
		return err
	}
	defer view.Close()
	if view.Purged != w.markers[label] {
		return fmt.Errorf("purged=%d; want %d", view.Purged, w.markers[label])
	}
	return nil
}

func (w *homeRetWorld) checkCursorAgainstPurgeMark() error {
	view, err := w.db.Snapshot(w.group)
	if err != nil {
		return err
	}
	defer view.Close()
	older := view.Purged - 1
	at := view.Purged
	if older >= view.Purged || at < view.Purged {
		return fmt.Errorf("cursor check failed for purged=%d", view.Purged)
	}
	return nil
}

func (w *homeRetWorld) seedConfirmedBriefs(daysAgo int) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	var err error
	w.briefKept, w.confirmAct, err = w.postAndConfirm(true)
	if err != nil {
		return err
	}
	w.briefGone, _, err = w.postAndConfirm(false)
	return err
}

func (w *homeRetWorld) postAndConfirm(withDependent bool) (string, proto.Envelope, error) {
	tasks := []proto.BriefTask{{Title: "Base work", Assignee: w.aiOne, Reviewer: w.aiTwo, Depends: []int{}}}
	if withDependent {
		tasks = append(tasks, proto.BriefTask{Title: "Follow-up work", Assignee: w.aiTwo, Reviewer: w.aiOne, Depends: []int{0}})
	}
	briefID := w.nextID()
	post := proto.Post[proto.BriefBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: briefID, Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.group), As: proto.ID(w.secID),
		},
		Kind: "brief", Mentions: []proto.ID{},
		Body: proto.BriefBody{
			Title: "Feature plan", Goal: "Ship cleanly", Criteria: []string{"Tests pass"},
			Scope: proto.BriefScope{In: []string{"home"}, Out: []string{}},
			Tasks: tasks, Pass: 90, Requester: w.ownerID,
		},
	}
	if err := w.sendCmd(w.hostConn, w.laptop, post); err != nil {
		return "", proto.Envelope{}, err
	}
	actPayload, err := json.Marshal(map[string]any{
		"t": "act", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
		"msg": briefID, "action": "confirm",
	})
	if err != nil {
		return "", proto.Envelope{}, err
	}
	actEnv := w.browser.Sign(string(actPayload))
	w.ownerConn.re++
	w.loop.Cmd(w.ownerConn.conn, w.ownerConn.re, actEnv)
	got, err := w.ownerConn.next()
	want := fmt.Sprintf(`{"re":%d,"ok":true}`, w.ownerConn.re)
	if err != nil || got != want {
		return "", proto.Envelope{}, fmt.Errorf("confirm got %s (%v); want %s", got, err, want)
	}
	return string(briefID), actEnv, nil
}

func (w *homeRetWorld) checkBriefsAfterFirstRun() error {
	kept, err := w.rowCount(w.briefKept)
	if err != nil {
		return err
	}
	gone, err := w.rowCount(w.briefGone)
	if err != nil {
		return err
	}
	if kept != 1 || gone != 0 {
		return fmt.Errorf("kept brief=%d, non-queued brief=%d; want 1 and 0", kept, gone)
	}
	return nil
}

func (w *homeRetWorld) mergeAndRelease() error {
	return w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "task", "op": "release", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
	})
}

func (w *homeRetWorld) checkReleasedCause() error {
	if len(w.released) != 1 || w.released[0].Why != "assign" || w.released[0].Cause == nil || *w.released[0].Cause != w.confirmAct {
		return fmt.Errorf("released items=%+v; want 1 assign with original confirm act", w.released)
	}
	return nil
}

func (w *homeRetWorld) checkQueuedBriefDeleted() error {
	kept, err := w.rowCount(w.briefKept)
	if err != nil {
		return err
	}
	if kept != 0 {
		return fmt.Errorf("brief count=%d; want 0", kept)
	}
	return nil
}

func (w *homeRetWorld) seedThreeTasks(daysAgo int) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	for i, s := range []struct{ id, state string }{
		{"task-active", "active"},
		{"task-merged", "merged"},
		{"task-cancelled", "cancelled"},
	} {
		if err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
			"t": "task", "op": "seed", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
			"id": s.id, "num": i + 1, "state": s.state,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeRetWorld) storeHoldsOnlyActiveTask() error {
	var ids []string
	err := w.db.Write(func(tx *store.Tx) error {
		var raw string
		if err := tx.QueryRow("SELECT coalesce(group_concat(id, ',' ORDER BY seq), '') FROM row WHERE g=? AND r='task'", w.group).Scan(&raw); err != nil {
			return err
		}
		if raw != "" {
			ids = strings.Split(raw, ",")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !slices.Equal(ids, []string{"task-active"}) {
		return fmt.Errorf("store task rows=%v; want [task-active]", ids)
	}
	return nil
}

func (w *homeRetWorld) memoryHoldsCount(want int) error {
	if err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "task", "op": "inspect", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
	}); err != nil {
		return err
	}
	if w.memTasks != want {
		return fmt.Errorf("in-memory tasks=%d; want %d", w.memTasks, want)
	}
	return nil
}

func (w *homeRetWorld) restart() error {
	w.loop.Close()
	return w.startLoop()
}

func (w *homeRetWorld) memoryAfterRestart(wantNum int) error {
	if err := w.sendCmd(w.ownerConn, w.browser, map[string]any{
		"t": "task", "op": "inspect", "n": w.nextID(), "ts": w.now.UnixMilli(), "g": w.group,
	}); err != nil {
		return err
	}
	if w.memTasks != 1 || !w.memActive || w.memNum != wantNum {
		return fmt.Errorf("after restart: count=%d active=%v num=%d; want 1, true, %d", w.memTasks, w.memActive, w.memNum, wantNum)
	}
	return nil
}

func (w *homeRetWorld) rowCount(id string) (int, error) {
	var n int
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND id=?", w.group, id).Scan(&n)
	})
	return n, err
}

func (w *homeRetWorld) exec(query string, args ...any) error {
	return w.db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec(query, args...)
		return err
	})
}
