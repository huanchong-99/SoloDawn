//go:build acceptance

package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_inbox"] = func(sc *godog.ScenarioContext, _ fixtures) { homeInboxSteps(sc) }
}

const homeInboxClock = int64(1_763_600_000_000)

type homeInboxWorld struct {
	clock     atomic.Int64
	db        *store.Store
	loop      *home.Loop
	browserK  identity.Key
	nodeK     identity.Key
	laptopK   identity.Key
	browser   *homeInboxPeer
	laptop    *homeInboxPeer
	group     string
	ownerID   string
	secID     string
	workerID  string
	reviewID  string
	taskID    string
	nonce     int
	lastBrief []proto.InboxItem
}

type homeInboxPeer struct {
	conn *home.Conn
	// frames has a single reader: the scenario step reading from this peer.
	frames chan string
	re     int64
}

func (p *homeInboxPeer) Send(frame any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(frame); err != nil {
		return err
	}
	p.frames <- strings.TrimSuffix(b.String(), "\n")
	return nil
}

func (p *homeInboxPeer) Done() <-chan struct{} { return nil }

func (p *homeInboxPeer) next() (string, error) {
	select {
	case raw := <-p.frames:
		return raw, nil
	case <-time.After(5 * time.Second):
		return "", fmt.Errorf("timed out waiting for frame")
	}
}

func homeInboxSteps(sc *godog.ScenarioContext) {
	w := &homeInboxWorld{}
	cleanup := afterScenario(sc)
	sc.Step(`^a group home with a secretary and a working AI on the owner's laptop$`, func() error {
		return w.setup(cleanup, 20, true, false, true)
	})
	sc.Step(`^a group home with a wake-up cap of (\d+) per hour and a working AI on the owner's laptop$`, func(capPerHour int) error {
		return w.setup(cleanup, capPerHour, false, false, true)
	})
	sc.Step(`^a group home with an assignee and a reviewer on the owner's laptop, while the laptop is away$`, func() error {
		return w.setup(cleanup, 20, true, true, false)
	})
	sc.Step(`^a brief of 1 task for the working AI is posted and confirmed$`, w.postAndConfirmBrief)
	sc.Step(`^the laptop receives the confirmed rows and then an inbox frame with "assign" and "confirmed", carrying their task and message rows$`, w.checkBriefInbox)
	sc.Step(`^the laptop reconnects and says hello(?: before acknowledging the items)?$`, w.reconnectLaptop)
	sc.Step(`^the laptop connects and says hello$`, w.reconnectLaptop)
	sc.Step(`^the laptop receives the same "assign" and "confirmed" items again in 1 inbox frame$`, w.checkResentBriefInbox)
	sc.Step(`^the owner's browser sends an ack for item (\d+)$`, func(item int64) {
		w.loop.Ack(w.browser.conn, proto.Ack{Type: "ack", Group: proto.ID(w.group), Item: item})
	})
	sc.Step(`^the laptop sends an ack for item (\d+) and item (\d+)$`, func(a, b int64) {
		w.loop.Ack(w.laptop.conn, proto.Ack{Type: "ack", Group: proto.ID(w.group), Item: a})
		w.loop.Ack(w.laptop.conn, proto.Ack{Type: "ack", Group: proto.ID(w.group), Item: b})
	})
	sc.Step(`^the laptop receives no inbox frame$`, w.laptopQuiet)
	homeInboxCapAndRetireSteps(sc, w)
}

func homeInboxCapAndRetireSteps(sc *godog.ScenarioContext, w *homeInboxWorld) {
	sc.Step(`^(\d+) mentions are queued for the working AI$`, w.queueMentions)
	sc.Step(`^the laptop receives (\d+) "mention" items$`, w.expectMentionItems)
	sc.Step(`^the laptop receives (\d+) "mention" items in 1 inbox frame$`, w.expectMentionBatch)
	sc.Step(`^another mention is queued for the working AI (\d+) seconds later$`, func(sec int64) error {
		w.clock.Add(sec * 1000)
		return w.queueMentions(1)
	})
	sc.Step(`^the laptop receives no inbox frame before 1 hour has passed since the 1st mention$`, func() error {
		w.loop.Tick(time.UnixMilli(homeInboxClock + 3_600_000 - 1))
		return w.laptopQuiet()
	})
	sc.Step(`^1 hour has passed since the 1st mention$`, func() {
		w.clock.Store(homeInboxClock + 3_600_000)
		w.loop.Tick(time.UnixMilli(homeInboxClock + 3_600_000))
	})
	sc.Step(`^the laptop receives item (\d+)$`, w.expectSingleItem)
	sc.Step(`^the task is (?:submitted|resubmitted) with commit "([^"]*)"$`, w.submitCommit)
	sc.Step(`^the laptop receives 1 inbox item, a "review" for commit "([^"]*)" with the task at that commit$`, w.checkReviewCommit)
}

func (w *homeInboxWorld) id() proto.ID {
	w.nonce++
	return exampleID(fmt.Sprint("home inbox ", w.nonce))
}

func (w *homeInboxWorld) setup(cleanup func(func()), capPerHour int, withSec, withReviewer, laptopOnline bool) error {
	w.clock.Store(homeInboxClock)
	w.browserK, w.nodeK, w.laptopK = exampleKey("inbox browser"), exampleKey("inbox node"), exampleKey("inbox laptop")
	w.group = string(w.id())
	w.secID, w.workerID, w.reviewID = string(exampleID("inbox sec")), string(exampleID("inbox worker")), string(exampleID("inbox reviewer"))
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db

	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: w.nodeK.ID(), Owner: w.browserK.ID(),
		Logs:   w.logs,
		Tables: w.tables(),
		Now:    func() time.Time { return time.UnixMilli(w.clock.Load()) },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
	w.browser, w.laptop = w.attach(w.browserK), w.attach(w.laptopK)
	return w.bootstrap(capPerHour, withSec, withReviewer, laptopOnline)
}

func (w *homeInboxWorld) logs(user string) *identity.Log {
	log, _ := homeLog([]identity.Key{w.browserK, w.nodeK, w.laptopK})
	if log == nil || log.User != user {
		return nil
	}
	return log
}

func (w *homeInboxWorld) tables() home.Tables {
	tables := home.NodeTables()
	cmds := home.Table{}
	for k, fn := range tables.Commands {
		cmds[k] = fn
	}
	cmds["post/text"] = w.mentionCmd
	cmds["task/submitted"] = w.submitCmd
	tables.Commands = cmds
	return tables
}

func (w *homeInboxWorld) mentionCmd(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID,
		By: c.Who.Member(), Kind: "text", Timestamp: c.Now, Text: "ping",
		Mentions: []string{w.workerID}, Body: json.RawMessage(`{}`), Env: &c.Env,
	}
	return home.Result{Rows: []home.Row{home.RowOf(msg)}, Items: []home.Item{{AI: w.workerID, Why: "mention", Msg: msg.ID}}}, nil
}

func (w *homeInboxWorld) submitCmd(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var req struct {
		proto.PayloadHeader
		Op     string `json:"op"`
		Commit string `json:"commit"`
	}
	if refusal := c.Decode(&req); refusal != nil {
		return home.Result{}, refusal
	}
	base := s.Tasks[w.taskID]
	if base.Step == "branch" {
		base.Step = "local_check"
	}
	base.Checks.Local = &proto.Check{OK: true, At: c.Now, Commit: req.Commit}
	moved, fx, err := home.Step(base, home.Event{Kind: home.OnSubmitted, By: home.ByHost, Now: c.Now, Commit: req.Commit})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "refused"}
	}
	var retire []int64
	for _, q := range c.Disk.Queued() {
		if q.Why == fx.Retire[0].Why {
			retire = append(retire, q.ID)
		}
	}
	it := fx.Items[0]
	return home.Result{
		Rows:   []home.Row{home.RowOf(moved)},
		Items:  []home.Item{{AI: it.AI, Why: it.Why, Task: w.taskID, Commit: it.Commit}},
		Retire: retire,
	}, nil
}

func (w *homeInboxWorld) attach(key identity.Key) *homeInboxPeer {
	p := &homeInboxPeer{frames: make(chan string, 64)}
	p.conn = w.loop.Attach(key.ID(), p)
	return p
}

func (w *homeInboxWorld) cmd(p *homeInboxPeer, key identity.Key, payload map[string]any) error {
	if _, has := payload["n"]; !has {
		payload["n"] = w.id()
	}
	payload["ts"] = w.clock.Load()
	payload["g"] = w.group
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	p.re++
	w.loop.Cmd(p.conn, p.re, key.Sign(string(raw)))
	got, err := p.next()
	if want := fmt.Sprintf(`{"re":%d,"ok":true}`, p.re); err != nil || got != want {
		return fmt.Errorf("cmd got %s (%v), want %s", got, err, want)
	}
	return nil
}

func (w *homeInboxWorld) bootstrap(capPerHour int, withSec, withReviewer, laptopOnline bool) error {
	createNonce := proto.ID(w.group)
	err := w.cmd(w.browser, w.browserK, map[string]any{
		"t": "group", "op": "create", "n": createNonce, "name": "Website", "kind": "project",
		"repo": "https://git.example/r.git", "base": "main", "forge": "none", "checks": []string{}, "member_name": "Owner",
	})
	if err != nil {
		return err
	}
	if err := w.enrollAIs(withSec, withReviewer); err != nil {
		return err
	}
	if err := w.configureCapAndTasks(capPerHour, withReviewer); err != nil {
		return err
	}
	if !laptopOnline {
		w.loop.Detach(w.laptop.conn)
		return nil
	}
	w.loop.Watch(w.laptop.conn, w.group, true)
	w.loop.Inbox(w.laptop.conn)
	return nil
}

func (w *homeInboxWorld) enrollAIs(withSec, withReviewer bool) error {
	if err := w.addAI(w.workerID, "claude", "Worker", false); err != nil {
		return err
	}
	if err := w.addAI(w.reviewID, "codex", "Reviewer", false); err != nil {
		return err
	}
	if !withSec {
		return nil
	}
	return w.addAI(w.secID, "claude", "Secretary", true)
}

func (w *homeInboxWorld) addAI(id, cli, name string, secretary bool) error {
	return w.cmd(w.laptop, w.laptopK, map[string]any{
		"t": "member", "op": "add_ai", "id": id, "cli": cli, "name": name,
		"account": "personal", "secretary": secretary, "approvers": []string{},
	})
}

func (w *homeInboxWorld) configureCapAndTasks(capPerHour int, withReviewer bool) error {
	if capPerHour != 20 {
		err := w.cmd(w.browser, w.browserK, map[string]any{
			"t": "group", "op": "set",
			"limits": proto.Limits{Rounds: 10, WakesPerHour: capPerHour, TurnMinutes: 60, Failures: 3, ReviewRounds: 5, AskDepth: 4, AskWaitS: 45, ApprovalWaitS: 600, IdleNoticeMinutes: 30},
		})
		if err != nil {
			return err
		}
	}
	var ownerID string
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT id FROM row WHERE g=? AND r='member' AND data->>'kind'='human'", w.group).Scan(&ownerID)
	})
	w.ownerID = ownerID
	if err != nil || !withReviewer {
		return err
	}
	if err := w.postAndConfirmBrief(); err != nil {
		return err
	}
	w.loop.Ack(w.laptop.conn, proto.Ack{Type: "ack", Group: proto.ID(w.group), Item: 1})
	w.loop.Ack(w.laptop.conn, proto.Ack{Type: "ack", Group: proto.ID(w.group), Item: 2})
	return nil
}

func (w *homeInboxWorld) postAndConfirmBrief() error {
	briefNonce := w.id()
	body := proto.BriefBody{
		Title: "Path indicator", Goal: "Show connection path.", Criteria: []string{"Updates within 1 s."},
		Scope: proto.BriefScope{In: []string{"ui"}, Out: []string{}},
		Tasks: []proto.BriefTask{{Title: "Show path", Assignee: w.workerID, Reviewer: w.reviewID, Depends: []int{}}},
		Pass:  90, Requester: w.ownerID,
	}
	err := w.cmd(w.laptop, w.laptopK, map[string]any{
		"t": "post", "n": briefNonce, "as": w.secID, "kind": "brief", "text": "Plan",
		"thread": nil, "main": false, "mentions": []string{}, "re": nil, "body": body,
	})
	if err != nil {
		return err
	}
	if err := w.cmd(w.browser, w.browserK, map[string]any{"t": "act", "msg": briefNonce, "action": "confirm"}); err != nil {
		return err
	}
	return w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT id FROM row WHERE g=? AND r='task'", w.group).Scan(&w.taskID)
	})
}

func (w *homeInboxWorld) decodeInbox(raw string) (proto.Inbox, error) {
	var frame proto.Inbox
	return frame, proto.Decode([]byte(raw), &frame)
}

func (w *homeInboxWorld) checkBriefInbox() error {
	if err := w.expectStateRows(1); err != nil {
		return err
	}
	inboxRaw, err := w.laptop.next()
	if err != nil {
		return err
	}
	frame, err := w.decodeInbox(inboxRaw)
	if err != nil || !validBriefItems(frame.Items) {
		return fmt.Errorf("inbox frame: %s (%v)", inboxRaw, err)
	}
	w.lastBrief = frame.Items
	return nil
}

func (w *homeInboxWorld) expectStateRows(count int) error {
	rowsRaw, err := w.laptop.next()
	if err != nil {
		return err
	}
	var rows proto.Rows
	if err := proto.Decode([]byte(rowsRaw), &rows); err != nil || len(rows.Rows) != count {
		return fmt.Errorf("expected %d state row before inbox, got %s (%v)", count, rowsRaw, err)
	}
	return nil
}

func validBriefItems(items []proto.InboxItem) bool {
	if len(items) != 2 {
		return false
	}
	return validAssignItem(items[0]) && validConfirmedItem(items[1])
}

func validAssignItem(it proto.InboxItem) bool {
	return it.Item.Why == "assign" && it.Task != nil && len(it.Msg) > 0
}

func validConfirmedItem(it proto.InboxItem) bool {
	return it.Item.Why == "confirmed" && len(it.Msg) > 0 && it.Task == nil
}

func (w *homeInboxWorld) reconnectLaptop() error {
	w.loop.Detach(w.laptop.conn)
	w.laptop = w.attach(w.laptopK)
	w.loop.Inbox(w.laptop.conn)
	return nil
}

func (w *homeInboxWorld) checkResentBriefInbox() error {
	raw, err := w.laptop.next()
	if err != nil {
		return err
	}
	frame, err := w.decodeInbox(raw)
	if err != nil || !reflect.DeepEqual(frame.Items, w.lastBrief) {
		return fmt.Errorf("resent %+v (%v), want %+v", frame.Items, err, w.lastBrief)
	}
	return nil
}

func (w *homeInboxWorld) laptopQuiet() error {
	// Synchronise with the group goroutine, then verify no frame arrived on laptop.
	w.loop.Watch(w.browser.conn, w.group, true)
	select {
	case got := <-w.laptop.frames:
		return fmt.Errorf("laptop unexpectedly received %s", got)
	default:
		return nil
	}
}

func (w *homeInboxWorld) queueMentions(count int) error {
	for range count {
		if err := w.cmd(w.browser, w.browserK, map[string]any{"t": "post", "kind": "text", "text": "ping"}); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeInboxWorld) expectMentionItems(count int) error {
	for range count {
		raw, err := w.laptop.next()
		if err != nil {
			return err
		}
		frame, err := w.decodeInbox(raw)
		if err != nil || len(frame.Items) != 1 || frame.Items[0].Item.Why != "mention" {
			return fmt.Errorf("expected 1 mention item, got %s (%v)", raw, err)
		}
	}
	return nil
}

func (w *homeInboxWorld) expectMentionBatch(count int) error {
	raw, err := w.laptop.next()
	if err != nil {
		return err
	}
	frame, err := w.decodeInbox(raw)
	if err != nil || len(frame.Items) != count {
		return fmt.Errorf("expected %d mention items in 1 frame, got %s (%v)", count, raw, err)
	}
	return nil
}

func (w *homeInboxWorld) expectSingleItem(wantID int64) error {
	raw, err := w.laptop.next()
	if err != nil {
		return err
	}
	frame, err := w.decodeInbox(raw)
	if err != nil || len(frame.Items) != 1 || frame.Items[0].Item.Item != wantID {
		return fmt.Errorf("expected item %d, got %s (%v)", wantID, raw, err)
	}
	return nil
}

func (w *homeInboxWorld) submitCommit(commit string) error {
	// A second host connection sends the submission while the AI's session connection is away.
	sender := w.attach(w.laptopK)
	defer w.loop.Detach(sender.conn)
	return w.cmd(sender, w.laptopK, map[string]any{"t": "task", "op": "submitted", "commit": commit})
}

func (w *homeInboxWorld) checkReviewCommit(commit string) error {
	raw, err := w.laptop.next()
	if err != nil {
		return err
	}
	frame, err := w.decodeInbox(raw)
	if err != nil {
		return err
	}
	reviews := reviewItems(frame.Items)
	if !singleReviewAt(reviews, commit) {
		return fmt.Errorf("got review items %+v, want 1 review at %s", reviews, commit)
	}
	return nil
}

func reviewItems(items []proto.InboxItem) []proto.InboxItem {
	var reviews []proto.InboxItem
	for _, it := range items {
		if it.Item.Why == "review" {
			reviews = append(reviews, it)
		}
	}
	return reviews
}

func singleReviewAt(reviews []proto.InboxItem, commit string) bool {
	if len(reviews) != 1 {
		return false
	}
	it := reviews[0]
	return it.Item.Commit == commit && it.Task != nil && it.Task.Commit == commit
}
