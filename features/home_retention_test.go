//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
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
	today     time.Time
	now       time.Time
	db        *store.Store
	loop      *home.Loop
	cleanup   func(func())
	browser   identity.Key
	node      identity.Key
	laptop    identity.Key
	ownerConn *homeRunPeer
	hostConn  *homeRunPeer
	group     string
	secID     string
	aiOne     string
	aiTwo     string
	nonce     int
	markers   map[string]int64
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
	sc.Step(`^daily retention runs today$`, w.retainToday)
	sc.Step(`^the store holds the messages "([^"]*)"$`, w.storeHoldsMessages)
	sc.Step(`^a cold sync of the group delivers "([^"]*)"$`, w.coldSyncDelivers)
	sc.Step(`^(\d+) days ago the group received the note "([^"]*)" and the review message "([^"]*)"$`, w.seedMemoryRows)
	sc.Step(`^a note "([^"]*)" from (\d+) days ago was forgotten (\d+) days ago$`, w.seedForgottenNote)
	sc.Step(`^the store still holds the note "([^"]*)" and the review message "([^"]*)"$`, w.storeHoldsMemoryRows)
	sc.Step(`^the deletion marker for "([^"]*)" is gone while the deletion marker for "([^"]*)" remains$`, w.checkMarkers)
	sc.Step(`^syncing from before the removed marker for "([^"]*)" resets the client while syncing from that marker sends a delta without reset$`, w.checkSyncAgainstPurgeMark)
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
	setDays := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('retention_days', ?)", strconv.Itoa(days))
		return err
	}
	if err := w.db.Write(setDays); err != nil {
		return err
	}
	if err := w.startLoop(); err != nil {
		return err
	}
	return w.foundGroup()
}

func (w *homeRetWorld) startLoop() error {
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: w.node.ID(), Owner: w.browser.ID(),
		Logs: func(user string) *identity.Log {
			log, _ := homeLog([]identity.Key{w.browser, w.node, w.laptop})
			if log == nil || log.User != user {
				return nil
			}
			return log
		},
		Tables: home.NodeTables(),
		Now:    func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	w.cleanup(loop.Close)
	w.loop = loop
	w.ownerConn = &homeRunPeer{frames: make(chan any, 128)}
	w.ownerConn.conn = loop.Attach(w.browser.ID(), w.ownerConn)
	w.hostConn = &homeRunPeer{frames: make(chan any, 128)}
	w.hostConn.conn = loop.Attach(w.laptop.ID(), w.hostConn)
	return nil
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
		Name:       "Website",
		MemberName: "owner",
	}
	if err := w.sendCmd(w.ownerConn, w.browser, create); err != nil {
		return err
	}
	for _, ai := range []struct {
		id, name string
		sec      bool
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
			CLI: "claude", Name: ai.name, Workspace: "/work/" + ai.name, Account: "personal", Secretary: ai.sec, Approvers: []string{},
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

func (w *homeRetWorld) retainToday() error {
	w.now = w.today
	return w.db.Write(func(tx *store.Tx) error { return home.Retain(tx, w.today.UnixMilli()) })
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

func (w *homeRetWorld) seedMemoryRows(daysAgo int, noteText, reviewText string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	note := proto.Note{
		HomeCommand: proto.HomeCommand{Type: "note", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		ID:          w.nextID(), Text: noteText,
	}
	if err := w.sendCmd(w.ownerConn, w.browser, note); err != nil {
		return err
	}
	return w.postOldReview(reviewText)
}

func (w *homeRetWorld) postOldReview(reviewText string) error {
	tid := exampleID("ret review-target")
	start := proto.TaskStart{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
				Group: proto.ID(w.group), As: proto.ID(w.aiOne),
			},
			Op: "start", ID: tid,
		},
		Title: "review-target", Plan: "Ship review-target", Origin: "direct",
	}
	if err := w.sendCmd(w.hostConn, w.laptop, start); err != nil {
		return err
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
		return err
	}
	return w.submitAndReview(tid, reviewText)
}

func (w *homeRetWorld) submitAndReview(tid proto.ID, reviewText string) error {
	report := w.nextID()
	sub := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: report, Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.group), As: proto.ID(w.aiOne),
		},
		Kind: "report", Thread: &tid, Text: "Ready for review.", Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
	if err := w.sendCmd(w.hostConn, w.laptop, sub); err != nil {
		return err
	}
	rev := proto.Post[proto.ReviewSigned]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.group), As: proto.ID(w.aiTwo),
		},
		Kind: "review", Thread: &tid, Text: reviewText, Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: tid, Report: report, Score: 92, Items: []proto.ReviewItem{}},
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
