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
	queuedID   string
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
	sc.Step(`^a cold sync of the group delivers "([^"]*)"$`, w.coldSyncDelivers)
	sc.Step(`^a second retention tick on the same day changes nothing$`, w.secondTickNoOp)
	sc.Step(`^(\d+) days ago the group received the note "([^"]*)" and the review message "([^"]*)"$`, w.seedMemoryRows)
	sc.Step(`^a note "([^"]*)" from (\d+) days ago was forgotten (\d+) days ago$`, w.seedForgottenNote)
	sc.Step(`^the store still holds the note "([^"]*)" and the review message "([^"]*)"$`, w.storeHoldsMemoryRows)
	sc.Step(`^the deletion marker for "([^"]*)" is gone while the deletion marker for "([^"]*)" remains$`, w.checkMarkers)
	sc.Step(`^syncing from before the removed marker for "([^"]*)" resets the client while syncing from that marker sends a delta without reset$`, w.checkSyncAgainstPurgeMark)
	sc.Step(`^(\d+) days ago the owner confirmed a brief with a queued dependent task and a second brief without a queued task$`, w.seedConfirmedBriefs)
	sc.Step(`^the store still holds the brief with the queued task and has deleted the brief without a queued task$`, w.checkBriefsAfterFirstRun)
	sc.Step(`^the dependency merges and releases the queued task$`, w.mergeAndRelease)
	sc.Step(`^the host receives the released task's assign item and can fetch its confirmed brief$`, w.checkReleasedAssignAndFetchBrief)
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
	events := maps.Clone(home.Events)
	events["ret.inspect"] = w.inspectState
	tables := home.Tables{
		Commands: home.Commands,
		Timers:   home.Timers,
		Events:   events,
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
	w.ownerConn = &homeRunPeer{frames: make(chan any, 256)}
	w.ownerConn.conn = loop.Attach(w.browser.ID(), w.ownerConn)
	w.hostConn = &homeRunPeer{frames: make(chan any, 256)}
	w.hostConn.conn = loop.Attach(w.laptop.ID(), w.hostConn)
	loop.Inbox(w.hostConn.conn)
	return nil
}

func (w *homeRetWorld) inspectState(s *home.State, _ home.Disk, _ int64, _ any) home.Result {
	w.memTasks = len(s.Tasks)
	_, w.memActive = s.Tasks[string(exampleID("ret task-active"))]
	w.memNum = s.Num
	return home.Result{}
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
	return w.waitAnswer(peer, peer.re)
}

func (w *homeRetWorld) waitAnswer(peer *homeRunPeer, re int64) error {
	for {
		got, err := peer.next()
		if err != nil {
			return err
		}
		var ans struct {
			Re  int64            `json:"re"`
			OK  bool             `json:"ok"`
			Err *proto.WireError `json:"err"`
		}
		if err := json.Unmarshal([]byte(got), &ans); err != nil {
			return err
		}
		if ans.Re == re {
			if !ans.OK {
				return fmt.Errorf("cmd %d refused: %+v", re, ans.Err)
			}
			return nil
		}
	}
}

func (w *homeRetWorld) foundGroup() error {
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: proto.ID(w.group), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
			Op:          "create",
		},
		Name: "Website", Kind: "project", Repo: "https://example.invalid/website",
		Base: "main", Forge: "none", Checks: []string{}, MemberName: "owner",
	}
	if err := w.sendCmd(w.ownerConn, w.browser, create); err != nil {
		return err
	}
	if err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT id FROM row WHERE g=? AND r='member' AND data->>'kind'='human'", w.group).Scan(&w.ownerID)
	}); err != nil {
		return err
	}
	for _, ai := range []struct {
		id   string
		name string
		sec  bool
	}{
		{w.secID, "secretary", true},
		{w.aiOne, "ai-one", false},
		{w.aiTwo, "ai-two", false},
	} {
		add := proto.MemberAddAI{
			MemberCommand: proto.MemberCommand{
				HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
				Op:          "add_ai", ID: proto.ID(ai.id),
			},
			CLI: "claude", Name: ai.name, Account: "personal", Secretary: ai.sec, Approvers: []string{},
		}
		if err := w.sendCmd(w.hostConn, w.laptop, add); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeRetWorld) seedMessagesAndCards(daysAgo int, textOld, pendingTitle, decidedTitle string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	if err := w.postText(textOld); err != nil {
		return err
	}
	if _, err := w.postDecision(pendingTitle); err != nil {
		return err
	}
	decidedID, err := w.postDecision(decidedTitle)
	if err != nil {
		return err
	}
	choose := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		Msg:         decidedID, Action: "choose", Arg: "opt1",
	}
	return w.sendCmd(w.ownerConn, w.browser, choose)
}

func (w *homeRetWorld) postText(text string) error {
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		Kind:        "text", Text: text, Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
	return w.sendCmd(w.ownerConn, w.browser, post)
}

func (w *homeRetWorld) postDecision(title string) (proto.ID, error) {
	id := w.nextID()
	post := proto.Post[proto.DecisionBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: id, Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.group), As: proto.ID(w.secID),
		},
		Kind: "decision", Text: title, Mentions: []proto.ID{},
		Body: proto.DecisionBody{
			Question: title,
			Options:  []proto.DecisionOption{{ID: "opt1", Label: "Proceed", Effect: "continue"}},
			To:       []string{w.browser.ID()},
		},
	}
	return id, w.sendCmd(w.hostConn, w.laptop, post)
}

func (w *homeRetWorld) seedTextMessage(daysAgo int, text string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	return w.postText(text)
}

func (w *homeRetWorld) tickAt(at time.Time) error {
	w.now = at
	w.loop.Tick(at)
	_, _, err := w.syncFrom(0)
	return err
}

func (w *homeRetWorld) syncFrom(since int64) (bool, []json.RawMessage, error) {
	peer := &homeRunPeer{frames: make(chan any, 128)}
	peer.conn = w.loop.Attach(w.browser.ID(), peer)
	defer w.loop.Detach(peer.conn)
	w.loop.Sync(peer.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 1}, Group: w.group, Since: since})
	return collectSyncFrames(peer, 1)
}

type syncFrameHead struct {
	Type string            `json:"t"`
	Re   int64             `json:"re"`
	Rows []json.RawMessage `json:"rows"`
}

func nextSyncFrame(peer *homeRunPeer) (syncFrameHead, error) {
	var head syncFrameHead
	raw, err := peer.next()
	if err != nil {
		return head, err
	}
	return head, json.Unmarshal([]byte(raw), &head)
}

func collectSyncFrames(peer *homeRunPeer, re int64) (bool, []json.RawMessage, error) {
	reset := false
	var rows []json.RawMessage
	for {
		head, err := nextSyncFrame(peer)
		if err != nil {
			return false, nil, err
		}
		reset = reset || head.Type == "reset"
		rows = append(rows, head.Rows...)
		if head.Type == "synced" && head.Re == re {
			return reset, rows, nil
		}
	}
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

func (w *homeRetWorld) coldSyncDelivers(wantCSV string) error {
	reset, rows, err := w.syncFrom(0)
	if err != nil || !reset {
		return fmt.Errorf("cold sync reset=%v err=%v", reset, err)
	}
	got := strings.Join(msgTextsFromRaw(rows), ", ")
	if got != wantCSV {
		return fmt.Errorf("cold sync delivered %q; want %q", got, wantCSV)
	}
	return nil
}

func msgTextsFromRaw(rows []json.RawMessage) []string {
	var texts []string
	for _, raw := range rows {
		var m proto.MsgRow
		if json.Unmarshal(raw, &m) == nil && m.Type == "msg" {
			texts = append(texts, m.Text)
		}
	}
	return texts
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
	note := proto.Note{
		HomeCommand: proto.HomeCommand{Type: "note", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		ID:          w.nextID(), Text: noteText,
	}
	if err := w.sendCmd(w.ownerConn, w.browser, note); err != nil {
		return err
	}
	taskID, num, err := w.startDirectTask("review-target")
	if err != nil {
		return err
	}
	return w.submitAndReviewTask(taskID, num, "review-target", w.aiTwo, reviewText)
}

func (w *homeRetWorld) startDirectTask(label string) (proto.ID, int, error) {
	tid := exampleID("ret " + label)
	start := proto.TaskStart{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
				Group: proto.ID(w.group), As: proto.ID(w.aiOne),
			},
			Op: "start", ID: tid,
		},
		Title: label, Plan: "Ship " + label, Origin: "direct",
	}
	if err := w.sendCmd(w.hostConn, w.laptop, start); err != nil {
		return "", 0, err
	}
	assign := proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
				Group: proto.ID(w.group), As: proto.ID(w.secID),
			},
			Op: "assign", ID: tid,
		},
		Reviewer: proto.ID(w.aiTwo),
	}
	if err := w.sendCmd(w.hostConn, w.laptop, assign); err != nil {
		return "", 0, err
	}
	var num int
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT data->>'num' FROM row WHERE g=? AND r='task' AND id=?", w.group, string(tid)).Scan(&num)
	})
	return tid, num, err
}

func (w *homeRetWorld) submitAndReviewTask(tid proto.ID, num int, title, reviewer, reviewText string) error {
	slug := proto.MakeSlug(title)
	branch := proto.TaskBranch{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
			Op:          "branch", ID: tid,
		},
		Branch: fmt.Sprintf("task/%d-%s", num, slug),
	}
	if err := w.sendCmd(w.hostConn, w.laptop, branch); err != nil {
		return err
	}
	checked := proto.TaskChecked{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
			Op:          "checked", ID: tid,
		},
		OK: true, Items: []proto.CheckItem{}, Commit: "c1",
	}
	if err := w.sendCmd(w.hostConn, w.laptop, checked); err != nil {
		return err
	}
	sub := proto.TaskSubmitted{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
			Op:          "submitted", ID: tid,
		},
		Commit: "c1", Stats: proto.DiffStats{Added: 4, Removed: 1},
	}
	if err := w.sendCmd(w.hostConn, w.laptop, sub); err != nil {
		return err
	}
	thread := tid
	rev := proto.Post[proto.ReviewSigned]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.group), As: proto.ID(reviewer),
		},
		Kind: "review", Thread: &thread, Text: reviewText, Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: tid, Commit: "c1", Score: 92, Items: []proto.ReviewItem{}},
	}
	return w.sendCmd(w.hostConn, w.laptop, rev)
}

func (w *homeRetWorld) seedForgottenNote(label string, createdDaysAgo, forgottenDaysAgo int) error {
	noteID := w.nextID()
	w.now = w.today.Add(-time.Duration(createdDaysAgo) * homeRetDay)
	note := proto.Note{
		HomeCommand: proto.HomeCommand{Type: "note", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		ID:          noteID, Text: label,
	}
	if err := w.sendCmd(w.ownerConn, w.browser, note); err != nil {
		return err
	}
	w.now = w.today.Add(-time.Duration(forgottenDaysAgo) * homeRetDay)
	forget := proto.Forget{
		HomeCommand: proto.HomeCommand{Type: "forget", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		Row:         "note", ID: noteID,
	}
	if err := w.sendCmd(w.ownerConn, w.browser, forget); err != nil {
		return err
	}
	var seq int64
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT seq FROM row WHERE g=? AND r='note' AND id=?", w.group, string(noteID)).Scan(&seq)
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

func (w *homeRetWorld) checkSyncAgainstPurgeMark(label string) error {
	mark := w.markers[label]
	resetBelow, _, err := w.syncFrom(mark - 1)
	if err != nil {
		return err
	}
	resetAt, _, err := w.syncFrom(mark)
	if err != nil {
		return err
	}
	if !resetBelow || resetAt {
		return fmt.Errorf("sync reset at %d=%v, at %d=%v; want true and false", mark-1, resetBelow, mark, resetAt)
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
	actPayload, err := json.Marshal(proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		Msg:         briefID, Action: "confirm",
	})
	if err != nil {
		return "", proto.Envelope{}, err
	}
	actEnv := w.browser.Sign(string(actPayload))
	w.ownerConn.re++
	w.loop.Cmd(w.ownerConn.conn, w.ownerConn.re, actEnv)
	return string(briefID), actEnv, w.waitAnswer(w.ownerConn, w.ownerConn.re)
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
	var firstID string
	var firstNum int
	err := w.db.Write(func(tx *store.Tx) error {
		if err := tx.QueryRow("SELECT id, data->>'num' FROM row WHERE g=? AND r='task' AND data->>'brief'=? AND data->>'state'='active'", w.group, w.briefKept).Scan(&firstID, &firstNum); err != nil {
			return err
		}
		return tx.QueryRow("SELECT id FROM row WHERE g=? AND r='task' AND data->>'brief'=? AND data->>'state'='queued'", w.group, w.briefKept).Scan(&w.queuedID)
	})
	if err != nil {
		return err
	}
	drainAvailable(w.hostConn)
	if err := w.submitAndReviewTask(proto.ID(firstID), firstNum, "Base work", w.aiTwo, "Ready to merge"); err != nil {
		return err
	}
	merged := proto.TaskMerged{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
			Op:          "merged", ID: proto.ID(firstID),
		},
		Commit: "c1", MergeCommit: "m1", Stats: proto.DiffStats{Added: 4, Removed: 1},
	}
	return w.sendCmd(w.hostConn, w.laptop, merged)
}

func drainAvailable(peer *homeRunPeer) {
	for {
		select {
		case <-peer.frames:
		default:
			return
		}
	}
}

func (w *homeRetWorld) checkReleasedAssignAndFetchBrief() error {
	if err := w.waitAssignFor(w.queuedID); err != nil {
		return err
	}
	return w.fetchAndCheckBrief(w.briefKept)
}

func (w *homeRetWorld) waitAssignFor(taskID string) error {
	for {
		raw, err := w.hostConn.next()
		if err != nil {
			return err
		}
		if inboxHasAssign(raw, taskID) {
			return nil
		}
	}
}

func inboxHasAssign(raw, taskID string) bool {
	var push proto.Inbox
	if json.Unmarshal([]byte(raw), &push) != nil || push.Type != "inbox" {
		return false
	}
	return hasAssignItem(push.Items, taskID)
}

func hasAssignItem(items []proto.InboxItem, taskID string) bool {
	for _, it := range items {
		if it.Item.Why == "assign" && string(it.Item.Task) == taskID {
			return true
		}
	}
	return false
}

func (w *homeRetWorld) fetchAndCheckBrief(briefID string) error {
	raw, err := w.fetchSingleRow("msg", briefID)
	if err != nil {
		return err
	}
	var card proto.MsgRow
	if err := json.Unmarshal(raw, &card); err != nil {
		return err
	}
	if !confirmedBriefMatches(card.State, w.confirmAct) {
		return fmt.Errorf("fetched brief state=%+v; want confirmed with original act", card.State)
	}
	return nil
}

func (w *homeRetWorld) fetchSingleRow(kind, rowID string) (json.RawMessage, error) {
	peer := &homeRunPeer{frames: make(chan any, 16)}
	peer.conn = w.loop.Attach(w.laptop.ID(), peer)
	defer w.loop.Detach(peer.conn)
	w.loop.Fetch(peer.conn, home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 1}, Group: w.group},
		Row:       kind, RowID: rowID,
	})
	raw, err := peer.next()
	if err != nil {
		return nil, err
	}
	var fetched proto.Fetched
	if err := json.Unmarshal([]byte(raw), &fetched); err != nil || len(fetched.Rows) != 1 {
		return nil, fmt.Errorf("fetch row got %s (%v)", raw, err)
	}
	return fetched.Rows[0], nil
}

func confirmedBriefMatches(state *proto.CardState, want proto.Envelope) bool {
	return state != nil && state.Status == "confirmed" && state.Act != nil && *state.Act == want
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
	if _, _, err := w.startDirectTask("task-active"); err != nil {
		return err
	}
	mergedID, mergedNum, err := w.startDirectTask("task-merged")
	if err != nil {
		return err
	}
	if err := w.submitAndReviewTask(mergedID, mergedNum, "task-merged", w.aiTwo, "Pass"); err != nil {
		return err
	}
	mergeCmd := proto.TaskMerged{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
			Op:          "merged", ID: mergedID,
		},
		Commit: "c1", MergeCommit: "m1", Stats: proto.DiffStats{Added: 2, Removed: 0},
	}
	if err := w.sendCmd(w.hostConn, w.laptop, mergeCmd); err != nil {
		return err
	}
	cancelID, _, err := w.startDirectTask("task-cancelled")
	if err != nil {
		return err
	}
	cancelCmd := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		Op:          "cancel", ID: cancelID,
	}
	return w.sendCmd(w.ownerConn, w.browser, cancelCmd)
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
	want := []string{string(exampleID("ret task-active"))}
	if !slices.Equal(ids, want) {
		return fmt.Errorf("store task rows=%v; want %v", ids, want)
	}
	return nil
}

func (w *homeRetWorld) memoryHoldsCount(want int) error {
	w.loop.Event("ret.inspect", nil)
	if _, _, err := w.syncFrom(0); err != nil {
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
	w.loop.Event("ret.inspect", nil)
	if _, _, err := w.syncFrom(0); err != nil {
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
