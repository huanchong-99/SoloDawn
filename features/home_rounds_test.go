//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
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
	stepSets["home_rounds"] = func(sc *godog.ScenarioContext, _ fixtures) { homeRoundsSteps(sc) }
}

type homeRoundsPeer struct {
	conn *home.Conn
	// frames has a single reader: nextFrame or checkHostQuiet on the scenario goroutine.
	frames chan any
	saw    trail
	re     int64
}

func (p *homeRoundsPeer) Send(frame any) error  { p.frames <- frame; return nil }
func (p *homeRoundsPeer) Done() <-chan struct{} { return nil }

func (p *homeRoundsPeer) nextFrame() (any, error) {
	return recvFrame(p.frames, 10*time.Second, &p.saw, briefAny)
}

type homeRoundsWorld struct {
	clock        atomic.Int64
	db           *store.Store
	loop         *home.Loop
	ghost        *home.Conn
	keys         map[string]identity.Key
	peers        map[string]*homeRoundsPeer
	members      map[string]string
	groupID      string
	taskID       string
	directMsg    string
	guestMsg     string
	threadMsg    string
	tenthMsg     string
	firstItemID  int64
	threadItemID int64
	lastQuietID  int64
	midState     string
	midWait      string
	lastErrCode  string
	receivedPush []proto.InboxItem
	nonceSeq     int
}

func homeRoundsSteps(sc *godog.ScenarioContext) {
	w := &homeRoundsWorld{}
	cleanup := afterScenario(sc)
	homeRoundsGivenSteps(sc, w, cleanup)
	homeRoundsWhenSteps(sc, w)
	homeRoundsThenSteps(sc, w)
}

func homeRoundsGivenSteps(sc *godog.ScenarioContext, w *homeRoundsWorld, cleanup func(func())) {
	sc.Step(`^a rounds home with a secretary, two working AIs and an active task for "([^"]*)"$`, func(aiName string) error {
		return w.setup(cleanup, aiName, 30)
	})
	sc.Step(`^a rounds home with idle_notice_minutes (\d+) and an active task with no thread message$`, func(idleMin int) error {
		return w.setup(cleanup, "ai-one", idleMin)
	})
	sc.Step(`^"([^"]*)" and "([^"]*)" (?:exchange|have exchanged) (\d+) thread mentions$`, w.exchangeRounds)
	sc.Step(`^the owner has posted a direct request mentioning "([^"]*)" whose item was (unacknowledged|acknowledged)$`, w.seedDirectRequest)
}

func homeRoundsWhenSteps(sc *godog.ScenarioContext, w *homeRoundsWorld) {
	sc.Step(`^the owner posts "([^"]*)" in the rounds task thread$`, func(text string) error {
		_, err := w.sendPost("owner", "text", &w.taskID, text, nil)
		return err
	})
	sc.Step(`^the (secretary|owner) (?:tries to resume|resumes) the rounds task$`, w.resumeTask)
	sc.Step(`^the owner posts a direct request mentioning "([^"]*)" in the main line$`, func(aiName string) error {
		id, err := w.sendPost("owner", "text", nil, "Please check this.", []string{w.members[aiName]})
		if err != nil {
			return err
		}
		w.directMsg = id
		items, err := w.drainHostItems(1)
		if err == nil && len(items) == 1 {
			w.firstItemID = items[0].Item.Item
		}
		return err
	})
	sc.Step(`^the owner posts a newer message mentioning "([^"]*)" in the main line$`, func(aiName string) error {
		if _, err := w.sendPost("owner", "text", nil, "And later this.", []string{w.members[aiName]}); err != nil {
			return err
		}
		_, err := w.drainHostItems(1)
		return err
	})
	sc.Step(`^the owner jumps the queue on that direct request$`, func() error {
		w.receivedPush = nil
		return w.sendJump("owner", w.directMsg, true)
	})
	sc.Step(`^the host of "([^"]*)" reports the rounds task pre-empted by that direct request and then resumes it$`, w.preemptAndResume)
	sc.Step(`^the owner posts in the rounds task thread mentioning "([^"]*)" and jumps the queue on that thread message$`, w.threadPostAndJump)
	sc.Step(`^(\d+) (?:more )?seconds? pass(?:es)? on the rounds home and it ticks$`, func(sec int) error {
		return w.advanceAndTick(int64(sec)*1000, 1)
	})
	sc.Step(`^the rounds host acknowledges that quiet notice and (\d+) minutes pass with ticks$`, func(mins int) error {
		w.ackItem(w.lastQuietID)
		w.receivedPush = nil
		return w.advanceAndTick(60_000, mins)
	})
	sc.Step(`^"([^"]*)" posts progress in the rounds task thread and (\d+) minute passes with a tick$`, func(aiName string, mins int) error {
		w.receivedPush = nil
		if _, err := w.sendPost(aiName, "progress", &w.taskID, "Updated branch.", nil); err != nil {
			return err
		}
		return w.advanceAndTick(60_000, mins)
	})
	sc.Step(`^("ai-one"|a guest member|the owner) sends jump on (the direct request|a guest's message|a missing message)$`, w.sendJumpCase)
}

func homeRoundsThenSteps(sc *godog.ScenarioContext, w *homeRoundsWorld) {
	sc.Step(`^the rounds task is "([^"]*)" waiting for "([^"]*)" at (\d+) rounds$`, w.checkTaskRounds)
	sc.Step(`^the main line of the rounds home holds a "([^"]*)" system message for the task$`, w.checkMainSystemMsg)
	sc.Step(`^the rounds host has received the 10th mention with the task "([^"]*)" and "([^"]*)"$`, w.checkTenthPush)
	sc.Step(`^the rounds host receives the first mention item again as a jump mention for "([^"]*)"$`, w.checkDirectJump)
	sc.Step(`^the rounds task was "([^"]*)" waiting for "([^"]*)" meanwhile and is "([^"]*)" waiting for "([^"]*)" afterwards$`, w.checkPreemptCycle)
	sc.Step(`^the rounds host receives a jump mention for "([^"]*)" on the task$`, w.checkThreadJump)
	sc.Step(`^the rounds host has nothing new$`, w.checkHostQuiet)
	sc.Step(`^the rounds host receives "([^"]*)"$`, w.checkSingleItem)
	sc.Step(`^the rounds home refuses with "([^"]*)"$`, func(code string) error {
		if w.lastErrCode != code {
			return fmt.Errorf("got refusal %q, want %q", w.lastErrCode, code)
		}
		return nil
	})
}

func (w *homeRoundsWorld) nextID() proto.ID {
	w.nonceSeq++
	return exampleID("rounds " + strconv.Itoa(w.nonceSeq))
}

func (w *homeRoundsWorld) setup(cleanup func(func()), assignee string, idleMin int) error {
	w.clock.Store(1763600000000)
	w.nonceSeq, w.lastErrCode, w.receivedPush = 0, "", nil
	browser, node, laptop, guest := exampleKey("rnd browser"), exampleKey("rnd node"), exampleKey("rnd laptop"), exampleKey("rnd guest")
	w.keys = map[string]identity.Key{"owner": browser, "node": node, "host": laptop, "guest": guest}
	w.members = map[string]string{
		"owner":     string(exampleID("rnd member owner")),
		"guest":     string(exampleID("rnd member guest")),
		"secretary": string(exampleID("rnd member secretary")),
		"ai-one":    string(exampleID("rnd member ai-one")),
		"ai-two":    string(exampleID("rnd member ai-two")),
	}
	w.groupID = string(exampleID("rnd group"))
	w.taskID = string(exampleID("rnd task 1"))
	w.guestMsg = string(exampleID("rnd guest msg"))

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db

	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: node.ID(), Owner: browser.ID(),
		Logs: func(user string) *identity.Log {
			switch user {
			case browser.ID():
				log, _ := homeLog([]identity.Key{browser, node, laptop})
				return log
			case guest.ID():
				log, _ := homeLog([]identity.Key{guest})
				return log
			}
			return nil
		},
		Tables: w.buildTables(assignee, idleMin),
		Now:    func() time.Time { return time.UnixMilli(w.clock.Load()) },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
	w.ghost = loop.Attach("ghost", &homeRoundsPeer{frames: make(chan any, 1)})
	loop.Detach(w.ghost)
	return w.attachPeers()
}

func (w *homeRoundsWorld) attachPeers() error {
	w.peers = map[string]*homeRoundsPeer{}
	for _, who := range []string{"owner", "host", "guest"} {
		p := &homeRoundsPeer{frames: make(chan any, 128)}
		p.conn = w.loop.Attach(w.keys[who].ID(), p)
		w.loop.Welcome(p.conn, func(groups []proto.GroupSummary) any {
			return proto.Groups{Type: "welcome", Groups: groups}
		})
		if _, err := p.nextFrame(); err != nil {
			return err
		}
		w.peers[who] = p
	}
	if err := w.execCmd("owner", "group", map[string]any{"op": "create"}, true); err != nil {
		return err
	}
	for _, p := range w.peers {
		if _, err := p.nextFrame(); err != nil {
			return err
		}
	}
	w.loop.Inbox(w.peers["host"].conn)
	return nil
}

func (w *homeRoundsWorld) buildTables(assignee string, idleMin int) home.Tables {
	base := home.NodeTables()
	cmds := maps.Clone(base.Commands)
	cmds["group/create"] = w.seedGroupCmd(assignee, idleMin)
	base.Commands = cmds
	return base
}

func (w *homeRoundsWorld) seedGroupCmd(assignee string, idleMin int) home.Func {
	return func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		grp := proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Rounds",
			Home: s.Home, Secretary: w.members["secretary"], Pass: 90, PassFoundation: 70,
			Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5, IdleNoticeMinutes: idleMin},
		}
		own := proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["owner"]}, Group: s.ID, Kind: "human", User: w.keys["owner"].ID()},
			Role:      "owner",
		}
		gst := proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["guest"]}, Group: s.ID, Kind: "human", User: w.keys["guest"].ID()},
			Role:      "member",
		}
		guestKey := w.keys["guest"]
		guestChange := proto.DeviceChange{User: guestKey.ID(), Version: 1, Op: "add", Device: guestKey.ID(), Kind: "browser", Name: "guest"}
		guestPayload, _ := json.Marshal(proto.DeviceEntry{
			PayloadHeader: proto.PayloadHeader{Type: "device", Nonce: exampleID("rnd guest dev"), Timestamp: c.Now},
			DeviceChange:  guestChange,
		})
		guestDev := proto.DeviceRow{
			RowHeader:    proto.RowHeader{Type: "device", ID: guestKey.ID() + ":1"},
			DeviceChange: guestChange,
			Env:          guestKey.Sign(string(guestPayload)),
		}
		rows := []home.Row{home.RowOf(grp), home.RowOf(own), home.RowOf(gst), home.RowOf(guestDev)}
		for _, role := range []string{"secretary", "ai-one", "ai-two"} {
			id := w.members[role]
			ai := proto.AIMemberRow{
				MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: s.ID, Kind: "ai", User: w.keys["owner"].ID(), Name: role},
				Host:      w.keys["host"].ID(), Workspace: "/work/" + role, Secretary: role == "secretary",
			}
			rows = append(rows, home.RowOf(ai))
		}
		task := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: w.taskID}, Group: s.ID, Num: 1,
			Title: "Task 1", Origin: "brief", State: "active", Step: "work",
			Assignee: w.members[assignee], Reviewer: w.members["ai-two"], Pass: 90, Depends: []string{},
			Created: c.Now, Stats: proto.TaskStats{Started: c.Now},
		}
		guestRow := proto.MsgRow{
			RowHeader: proto.RowHeader{Type: "msg", ID: w.guestMsg}, Group: s.ID, By: w.members["guest"],
			Kind: "text", Timestamp: c.Now, Text: "Guest note", Mentions: []string{}, Body: json.RawMessage(`{}`),
		}
		rows = append(rows, home.RowOf(task), home.RowOf(guestRow))
		return home.Result{Rows: rows, GroupsChanged: true}, nil
	}
}

func (w *homeRoundsWorld) execCmd(peerName, t string, extra map[string]any, wantOK bool) error {
	withNonce := map[string]any{"n": w.nextID()}
	maps.Copy(withNonce, extra)
	return w.execCmdWithNonce(peerName, t, withNonce, wantOK)
}

func (w *homeRoundsWorld) sendPost(actor, kind string, thread *string, text string, mentions []string) (string, error) {
	if mentions == nil {
		mentions = []string{}
	}
	nonce := w.nextID()
	peerName, extra := w.postActor(actor, nonce, kind, thread, text, mentions)
	if err := w.execCmdWithNonce(peerName, "post", extra, true); err != nil {
		return "", err
	}
	return string(nonce), nil
}

func (w *homeRoundsWorld) postActor(actor string, nonce proto.ID, kind string, thread *string, text string, mentions []string) (string, map[string]any) {
	extra := map[string]any{
		"n": nonce, "kind": kind, "thread": thread, "main": false,
		"text": text, "mentions": mentions, "re": nil, "body": map[string]any{},
	}
	if actor == "owner" || actor == "guest" {
		return actor, extra
	}
	extra["as"] = w.members[actor]
	return "host", extra
}

func (w *homeRoundsWorld) execCmdWithNonce(peerName, t string, extra map[string]any, wantOK bool) error {
	payload := map[string]any{"t": t, "ts": w.clock.Load(), "g": w.groupID}
	maps.Copy(payload, extra)
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	p := w.peers[peerName]
	p.re++
	w.loop.Cmd(p.conn, p.re, w.keys[peerName].Sign(string(raw)))
	frame, err := p.nextFrame()
	if err != nil {
		return err
	}
	return w.recordOutcome(t, frame, wantOK)
}

func (w *homeRoundsWorld) recordOutcome(t string, frame any, wantOK bool) error {
	errResp, ok := frame.(proto.ErrorResponse)
	if !ok {
		w.lastErrCode = ""
		return nil
	}
	w.lastErrCode = errResp.Err.Code
	if wantOK {
		return fmt.Errorf("command %s failed with %s: %s", t, errResp.Err.Code, errResp.Err.Message)
	}
	return nil
}

func (w *homeRoundsWorld) drainHostItems(count int) ([]proto.InboxItem, error) {
	var out []proto.InboxItem
	for len(out) < count {
		frame, err := w.peers["host"].nextFrame()
		if err != nil {
			return nil, err
		}
		push, ok := frame.(proto.Inbox)
		if !ok {
			return nil, fmt.Errorf("unexpected host frame %T", frame)
		}
		out = append(out, push.Items...)
	}
	return out, nil
}

func (w *homeRoundsWorld) exchangeRounds(aiA, aiB string, total int) error {
	w.receivedPush = nil
	for i := 1; i <= total; i++ {
		if err := w.exchangeOneRound(aiA, aiB, i, total); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeRoundsWorld) exchangeOneRound(aiA, aiB string, i, total int) error {
	from, to := aiA, aiB
	if i%2 == 0 {
		from, to = aiB, aiA
	}
	msgID, err := w.sendPost(from, "question", &w.taskID, fmt.Sprintf("Round %d", i), []string{w.members[to]})
	if err != nil {
		return err
	}
	if i < total {
		items, err := w.drainHostItems(1)
		if err != nil {
			return err
		}
		w.ackItem(items[0].Item.Item)
		return nil
	}
	items, err := w.drainHostItems(2)
	if err != nil {
		return err
	}
	w.tenthMsg, w.receivedPush = msgID, items
	return nil
}

func (w *homeRoundsWorld) ackItem(id int64) {
	w.loop.Ack(w.peers["host"].conn, proto.Ack{Type: "ack", Group: proto.ID(w.groupID), Item: id})
	w.syncLoop()
}

func (w *homeRoundsWorld) readTask() (proto.TaskRow, error) {
	var task proto.TaskRow
	err := w.db.Write(func(tx *store.Tx) error {
		raw, err := tx.Row(w.groupID, "task", w.taskID)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &task)
	})
	return task, err
}

func (w *homeRoundsWorld) syncLoop() {
	w.loop.Sync(w.ghost, proto.Sync{Group: w.groupID})
}

func (w *homeRoundsWorld) resumeTask(who string) error {
	w.receivedPush = nil
	peerName := "owner"
	extra := map[string]any{"op": "resume", "id": w.taskID}
	wantOK := true
	if who == "secretary" {
		peerName = "host"
		extra["as"] = w.members["secretary"]
		wantOK = false
	}
	return w.execCmd(peerName, "task", extra, wantOK)
}

func (w *homeRoundsWorld) sendJump(peerName, msgID string, wantOK bool) error {
	return w.execCmd(peerName, "jump", map[string]any{"msg": msgID}, wantOK)
}

func (w *homeRoundsWorld) preemptAndResume(_ string) error {
	if err := w.execCmd("host", "task", map[string]any{"op": "preempted", "id": w.taskID, "msg": w.directMsg}, true); err != nil {
		return err
	}
	tMid, err := w.readTask()
	if err != nil {
		return err
	}
	w.midState, w.midWait = tMid.State, taskWaitKind(tMid)
	return w.execCmd("host", "task", map[string]any{"op": "resume", "id": w.taskID}, true)
}

func (w *homeRoundsWorld) threadPostAndJump(aiName string) error {
	msgID, err := w.sendPost("owner", "text", &w.taskID, "Check this first.", []string{w.members[aiName]})
	if err != nil {
		return err
	}
	w.threadMsg = msgID
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	w.threadItemID = items[0].Item.Item
	w.receivedPush = nil
	return w.sendJump("owner", w.threadMsg, true)
}

func (w *homeRoundsWorld) advanceAndTick(stepMS int64, steps int) error {
	for range steps {
		next := w.clock.Add(stepMS)
		w.loop.Tick(time.UnixMilli(next))
		w.syncLoop()
	}
	return nil
}

func (w *homeRoundsWorld) seedDirectRequest(aiName, state string) error {
	msgID, err := w.sendPost("owner", "text", nil, "Direct request", []string{w.members[aiName]})
	if err != nil {
		return err
	}
	w.directMsg = msgID
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	if state == "acknowledged" {
		w.ackItem(items[0].Item.Item)
	}
	return nil
}

func (w *homeRoundsWorld) sendJumpCase(actor, target string) error {
	msgID := w.directMsg
	switch target {
	case "a guest's message":
		msgID = w.guestMsg
	case "a missing message":
		msgID = string(exampleID("rnd missing msg"))
	}
	switch actor {
	case `"ai-one"`:
		return w.execCmd("host", "jump", map[string]any{"as": w.members["ai-one"], "msg": msgID}, false)
	case "a guest member":
		return w.sendJump("guest", msgID, false)
	default:
		return w.sendJump("owner", msgID, false)
	}
}

func taskWaitKind(task proto.TaskRow) string {
	if task.Wait == nil {
		return "nothing"
	}
	return task.Wait.Kind
}

func (w *homeRoundsWorld) checkTaskRounds(state, wait string, rounds int) error {
	w.syncLoop()
	task, err := w.readTask()
	if err != nil {
		return err
	}
	got := []any{task.State, taskWaitKind(task), task.Thread.Rounds}
	want := []any{state, wait, rounds}
	if !slices.Equal(got, want) {
		return fmt.Errorf("task is %v; want %v", got, want)
	}
	return nil
}

func (w *homeRoundsWorld) checkMainSystemMsg(code string) error {
	var rows []json.RawMessage
	err := w.db.Write(func(tx *store.Tx) error {
		var txErr error
		rows, txErr = tx.Thread(w.groupID, nil, 0, 50)
		return txErr
	})
	if err != nil {
		return err
	}
	for _, raw := range rows {
		if homeRoundsMatchesLimit(raw, code, w.taskID) {
			return nil
		}
	}
	return fmt.Errorf("no %s main-line system message found for task %s", code, w.taskID)
}

func homeRoundsMatchesLimit(raw []byte, code, taskID string) bool {
	var m proto.MsgRow
	if json.Unmarshal(raw, &m) != nil || m.Kind != "system" {
		return false
	}
	var body proto.RoundsLimitBody
	if json.Unmarshal(m.Body, &body) != nil {
		return false
	}
	return body.Code == code && body.Task == taskID && body.N == 10
}

func (w *homeRoundsWorld) checkTenthPush(wantState, wantNotice string) error {
	if len(w.receivedPush) != 2 || w.receivedPush[0].Task == nil {
		return fmt.Errorf("host received %+v; want 10th mention and notice", w.receivedPush)
	}
	first := w.receivedPush[0]
	got := []string{first.Item.Why, string(first.Item.Msg), first.Task.State, taskWaitKind(*first.Task), w.formatItem(w.receivedPush[1].Item)}
	want := []string{"mention", w.tenthMsg, wantState, "rounds", wantNotice}
	if !slices.Equal(got, want) {
		return fmt.Errorf("got %v; want %v", got, want)
	}
	return w.checkHostQuiet()
}

func (w *homeRoundsWorld) checkDirectJump(aiName string) error {
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	got := items[0].Item
	want := proto.Item{Item: w.firstItemID, AI: proto.ID(w.members[aiName]), Why: "mention", Msg: proto.ID(w.directMsg), Jump: true}
	if got != want {
		return fmt.Errorf("jump item = %+v, want %+v", got, want)
	}
	return nil
}

func (w *homeRoundsWorld) checkPreemptCycle(midState, midWait, endState, endWait string) error {
	if w.midState != midState || w.midWait != midWait {
		return fmt.Errorf("meanwhile task was %s/%s, want %s/%s", w.midState, w.midWait, midState, midWait)
	}
	return w.checkTaskRounds(endState, endWait, 0)
}

func (w *homeRoundsWorld) checkThreadJump(aiName string) error {
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	got := items[0].Item
	want := proto.Item{Item: w.threadItemID, AI: proto.ID(w.members[aiName]), Why: "mention", Task: proto.ID(w.taskID), Msg: proto.ID(w.threadMsg), Jump: true}
	if got != want {
		return fmt.Errorf("thread jump item = %+v, want %+v", got, want)
	}
	return nil
}

func (w *homeRoundsWorld) checkHostQuiet() error {
	w.syncLoop()
	select {
	case frame := <-w.peers["host"].frames:
		return fmt.Errorf("expected no host frame, got %+v", frame)
	default:
		return nil
	}
}

func (w *homeRoundsWorld) checkSingleItem(want string) error {
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	w.lastQuietID = items[0].Item.Item
	if got := w.formatItem(items[0].Item); got != want {
		return fmt.Errorf("host received %q, want %q", got, want)
	}
	return nil
}

func (w *homeRoundsWorld) formatItem(it proto.Item) string {
	name := string(it.AI)
	for role, id := range w.members {
		if id == string(it.AI) {
			name = role
			break
		}
	}
	parts := []string{it.Why, name}
	if it.Code != "" {
		parts = append(parts, it.Code)
	}
	return strings.Join(parts, " ")
}
