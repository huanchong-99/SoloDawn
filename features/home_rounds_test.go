//go:build acceptance

package features

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
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
	conn   *home.Conn
	frames chan any
	re     int64
}

func (p *homeRoundsPeer) Send(frame any) error  { p.frames <- frame; return nil }
func (p *homeRoundsPeer) Done() <-chan struct{} { return nil }

func (p *homeRoundsPeer) nextFrame() (any, error) {
	select {
	case frame := <-p.frames:
		return frame, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timed out waiting for frame")
	}
}

type homeRoundsWorld struct {
	clock        atomic.Int64
	db           *store.Store
	loop         *home.Loop
	keys         map[string]identity.Key
	peers        map[string]*homeRoundsPeer
	members      map[string]string
	groupID      string
	taskID       string
	directMsg    string
	guestMsg     string
	threadMsg    string
	firstItemID  int64
	lastQuietID  int64
	heldMsgID    string
	midState     string
	midWait      string
	lastErrCode  string
	receivedPush []home.QueuedItem
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
			w.firstItemID = items[0].ID
		}
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
		if err := w.ackItem(w.lastQuietID); err != nil {
			return err
		}
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
	sc.Step(`^the rounds host has received "([^"]*)" and the 10th mention is held$`, w.checkHeldAtLimit)
	sc.Step(`^the rounds host receives the released mention$`, w.checkReleasedMention)
	sc.Step(`^the rounds home refuses with "([^"]*)" and the 10th mention stays held$`, func(code string) error {
		if w.lastErrCode != code {
			return fmt.Errorf("got refusal %q, want %q", w.lastErrCode, code)
		}
		return w.assertMentionStillHeld()
	})
	sc.Step(`^the rounds host receives "([^"]*)" and the released mention$`, w.checkResumeAndReleased)
	sc.Step(`^the first mention item is retired and the rounds host receives a jump mention for "([^"]*)"$`, w.checkDirectJump)
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

	tables := w.buildTables(assignee, idleMin)
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
		Tables: tables,
		Now:    func() time.Time { return time.UnixMilli(w.clock.Load()) },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
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
	// Drain groups push on all welcomed connections.
	for _, p := range w.peers {
		if _, err := p.nextFrame(); err != nil {
			return err
		}
	}
	w.loop.Inbox(w.peers["host"].conn)
	return nil
}

// buildTables uses NodeTables() and supplies minimal command handlers only for
// sibling commands not yet merged into rebuild.
func (w *homeRoundsWorld) buildTables(assignee string, idleMin int) home.Tables {
	base := home.NodeTables()
	cmds := home.Table{}
	for k, v := range base.Commands {
		cmds[k] = v
	}
	cmds["group/create"] = w.seedGroupCmd(assignee, idleMin)
	if cmds["post/text"] == nil {
		cmds["post/text"] = homeRoundsFallbackPost
		cmds["post/progress"] = homeRoundsFallbackPost
		cmds["post/question"] = homeRoundsFallbackPost
	}
	if cmds["task/resume"] == nil {
		cmds["task/resume"] = homeRoundsFallbackResume
	}
	if cmds["task/preempted"] == nil {
		cmds["task/preempted"] = homeRoundsFallbackPreempt
	}
	base.Commands = cmds
	return base
}

func (w *homeRoundsWorld) seedGroupCmd(assignee string, idleMin int) home.Func {
	return func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		grp := proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: "Rounds", Kind: "project",
			Home: s.Home, Secretary: w.members["secretary"], Forge: "none", Pass: 90, PassFoundation: 70,
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
				Host:      w.keys["host"].ID(), Secretary: role == "secretary",
			}
			rows = append(rows, home.RowOf(ai))
		}
		task := proto.TaskRow{
			RowHeader: proto.RowHeader{Type: "task", ID: w.taskID}, Group: s.ID, Num: 1,
			Title: "Task 1", Slug: "task-1", Origin: "brief", State: "active", Step: "local_check",
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

func homeRoundsFallbackPost(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.Post[proto.EmptyBody]
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID,
		Thread: (*string)(p.Thread), By: c.Who.Member(), Kind: p.Kind,
		Timestamp: c.Timestamp, Text: p.Text, Mentions: homeRoundsIDStrings(p.Mentions), Body: json.RawMessage(`{}`), Env: &c.Env,
	}
	items := homeRoundsPostWakes(s, msg)
	if msg.Thread == nil {
		return home.Result{Rows: []home.Row{home.RowOf(msg)}, Items: items}, nil
	}
	return homeRoundsStepThreadPost(s, c, msg, items)
}

func homeRoundsIDStrings(ids []proto.ID) []string {
	var list []string
	for _, item := range ids {
		list = append(list, string(item))
	}
	return list
}

func homeRoundsPostWakes(s *home.State, msg proto.MsgRow) []home.Item {
	var threadID string
	if msg.Thread != nil {
		threadID = *msg.Thread
	}
	var items []home.Item
	for _, m := range msg.Mentions {
		if _, ok := s.AIs[m]; ok {
			items = append(items, home.Item{AI: m, Why: "mention", Task: threadID, Msg: msg.ID})
		}
	}
	return items
}

func homeRoundsStepThreadPost(s *home.State, c home.Command, msg proto.MsgRow, items []home.Item) (home.Result, *home.Refusal) {
	task := s.Tasks[*msg.Thread]
	task.Thread.Count++
	task.Thread.Last = &proto.ThreadMessage{By: msg.By, Kind: msg.Kind, Text: msg.Text, Timestamp: msg.Timestamp}
	res := home.Result{Rows: []home.Row{home.RowOf(msg), home.RowOf(task)}, Items: items}
	if c.Who.AI == "" {
		return homeRoundsFallbackPersonPost(s, c, task, res), nil
	}
	return homeRoundsFallbackAIPost(s, c, task, res), nil
}

func homeRoundsFallbackPersonPost(s *home.State, c home.Command, task proto.TaskRow, res home.Result) home.Result {
	wasRounds := taskWaitKind(task) == "rounds"
	moved, _, _ := home.Step(task, home.Event{Kind: home.OnText, By: home.ByPerson, Now: c.Now, Limits: s.Group.Limits})
	res.Rows[1] = home.RowOf(moved)
	if wasRounds {
		retire, released := homeRoundsReleaseQueued(c.Disk.Queued(), moved.ID)
		res.Retire = retire
		res.Items = append(released, res.Items...)
	}
	return res
}

func homeRoundsFallbackAIPost(s *home.State, c home.Command, task proto.TaskRow, res home.Result) home.Result {
	if len(res.Items) > 0 {
		moved, fx, _ := home.Step(task, home.Event{Kind: home.OnMention, By: home.ByAI, Member: c.Who.AI, Now: c.Now, Limits: s.Group.Limits})
		res.Rows[1] = home.RowOf(moved)
		task = moved
		if fx.System != "" {
			res = homeRoundsAppendLimit(s, c, moved.ID, res)
		}
	}
	if taskWaitKind(task) == "rounds" {
		homeRoundsHoldSlice(res.Items)
	}
	return res
}

func homeRoundsAppendLimit(s *home.State, c home.Command, taskID string, res home.Result) home.Result {
	sum := sha256.Sum256([]byte("SoloDawn rounds limit " + string(c.Nonce)))
	sysID := base64.RawURLEncoding.EncodeToString(sum[:16])
	body, _ := json.Marshal(proto.RoundsLimitBody{SystemCode: proto.SystemCode{Code: "rounds_limit"}, Task: taskID, N: s.Group.Limits.Rounds})
	sysMsg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: sysID}, Group: s.ID, By: "system", Kind: "system", Timestamp: c.Now, Mentions: []string{}, Body: body}
	res.Rows = append(res.Rows, home.RowOf(sysMsg))
	res.Items = append(res.Items, home.Item{AI: s.Group.Secretary, Why: "notice", Task: taskID, Msg: sysID, Code: "rounds_limit"})
	return res
}

func homeRoundsHoldSlice(items []home.Item) {
	for i := range items {
		if items[i].Why == "mention" {
			items[i].NotBefore = math.MaxInt64
		}
	}
}

func homeRoundsFallbackResume(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.TaskCommand
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	task := s.Tasks[string(p.ID)]
	by := homeRoundsResumeSender(s, c.Who, task.Assignee)
	moved, fx, err := home.Step(task, home.Event{Kind: home.OnResume, By: by, Now: c.Now, Limits: s.Group.Limits})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "refused"}
	}
	items, retire := homeRoundsResumeItems(fx.Items, taskWaitKind(task) == "rounds", c.Disk.Queued(), moved.ID)
	return home.Result{Rows: []home.Row{home.RowOf(moved)}, Items: items, Retire: retire}, nil
}

func homeRoundsResumeSender(s *home.State, w home.Who, assignee string) home.Sender {
	if s.Secretary(w) {
		return home.BySecretary
	}
	if s.Hosts(w, assignee) {
		return home.ByHost
	}
	if s.Admin(w) {
		return home.ByAdmin
	}
	return home.ByPerson
}

func homeRoundsResumeItems(fxItems []home.TaskItem, wasRounds bool, queued []store.Item, taskID string) ([]home.Item, []int64) {
	var items []home.Item
	for _, it := range fxItems {
		items = append(items, home.Item{AI: it.AI, Why: it.Why, Task: taskID})
	}
	if !wasRounds {
		return items, nil
	}
	retire, released := homeRoundsReleaseQueued(queued, taskID)
	return append(items, released...), retire
}

func homeRoundsFallbackPreempt(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.TaskPreempted
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	task := s.Tasks[string(p.ID)]
	moved, _, err := home.Step(task, home.Event{Kind: home.OnPreempted, By: home.ByHost, Now: c.Now, Limits: s.Group.Limits})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "refused"}
	}
	return home.Result{Rows: []home.Row{home.RowOf(moved)}}, nil
}

func homeRoundsReleaseQueued(queued []store.Item, taskID string) ([]int64, []home.Item) {
	var retire []int64
	var released []home.Item
	for _, q := range queued {
		if homeRoundsHeldForTask(q, taskID) {
			retire = append(retire, q.ID)
			released = append(released, home.Item{AI: q.AI, Why: "mention", Task: taskID, Msg: *q.Msg})
		}
	}
	return retire, released
}

func homeRoundsHeldForTask(q store.Item, taskID string) bool {
	if q.Why != "mention" || q.Task == nil {
		return false
	}
	return *q.Task == taskID && q.NotBefore == math.MaxInt64
}

func (w *homeRoundsWorld) execCmd(peerName, t string, extra map[string]any, wantOK bool) error {
	withNonce := map[string]any{"n": w.nextID()}
	for k, v := range extra {
		withNonce[k] = v
	}
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
	for k, v := range extra {
		payload[k] = v
	}
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

func (w *homeRoundsWorld) drainHostItems(count int) ([]home.QueuedItem, error) {
	var out []home.QueuedItem
	for len(out) < count {
		frame, err := w.peers["host"].nextFrame()
		if err != nil {
			return nil, err
		}
		push, ok := frame.(home.ItemsPush)
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
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	if i < total {
		return w.ackItem(items[0].ID)
	}
	w.heldMsgID = msgID
	w.receivedPush = items
	return nil
}

func (w *homeRoundsWorld) ackItem(id int64) error {
	return w.db.Write(func(tx *store.Tx) error {
		return tx.Ack(w.groupID, w.keys["host"].ID(), id)
	})
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
	w.loop.Watch(w.peers["owner"].conn, w.groupID, true)
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

func (w *homeRoundsWorld) preemptAndResume(aiName string) error {
	if err := w.execCmd("host", "task", map[string]any{"op": "preempted", "id": w.taskID, "msg": w.directMsg}, true); err != nil {
		return err
	}
	tMid, err := w.readTask()
	if err != nil {
		return err
	}
	w.midState = tMid.State
	w.midWait = "nothing"
	if tMid.Wait != nil {
		w.midWait = tMid.Wait.Kind
	}
	return w.execCmd("host", "task", map[string]any{"op": "resume", "id": w.taskID}, true)
}

func (w *homeRoundsWorld) threadPostAndJump(aiName string) error {
	msgID, err := w.sendPost("owner", "text", &w.taskID, "Check this first.", []string{w.members[aiName]})
	if err != nil {
		return err
	}
	w.threadMsg = msgID
	if _, err := w.drainHostItems(1); err != nil {
		return err
	}
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
		return w.ackItem(items[0].ID)
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
	json.Unmarshal(m.Body, &body)
	return body.Code == code && body.Task == taskID && body.N == 10
}

func (w *homeRoundsWorld) checkHeldAtLimit(want string) error {
	if len(w.receivedPush) != 1 || w.formatItem(w.receivedPush[0].Item) != want {
		return fmt.Errorf("host received %+v; want %q", w.receivedPush, want)
	}
	if err := w.checkHostQuiet(); err != nil {
		return err
	}
	return w.assertMentionStillHeld()
}

func (w *homeRoundsWorld) assertMentionStillHeld() error {
	var queued []store.Item
	err := w.db.Write(func(tx *store.Tx) error {
		var txErr error
		queued, txErr = tx.Queued(w.groupID)
		return txErr
	})
	if err != nil {
		return err
	}
	var held int
	for _, q := range queued {
		if homeRoundsHeldMsg(q, w.heldMsgID) {
			held++
		}
	}
	if held != 1 {
		return fmt.Errorf("held mention count = %d, want 1", held)
	}
	return nil
}

func homeRoundsHeldMsg(q store.Item, msgID string) bool {
	if q.Why != "mention" || q.Msg == nil {
		return false
	}
	return *q.Msg == msgID && q.NotBefore == math.MaxInt64
}

func (w *homeRoundsWorld) checkReleasedMention() error {
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	it := items[0].Item
	got := []any{it.Why, it.Msg, it.Task, it.NotBefore}
	want := []any{"mention", w.heldMsgID, w.taskID, int64(0)}
	if !slices.Equal(got, want) {
		return fmt.Errorf("released item = %+v, want %v", it, want)
	}
	return nil
}

func (w *homeRoundsWorld) checkResumeAndReleased(wantResume string) error {
	items, err := w.drainHostItems(2)
	if err != nil {
		return err
	}
	got := []string{w.formatItem(items[0].Item), items[1].Why + " " + items[1].Msg}
	want := []string{wantResume, "mention " + w.heldMsgID}
	if !slices.Equal(got, want) {
		return fmt.Errorf("host received %v, want %v", got, want)
	}
	return nil
}

func (w *homeRoundsWorld) checkDirectJump(aiName string) error {
	items, err := w.drainHostItems(1)
	if err != nil {
		return err
	}
	want := home.Item{AI: w.members[aiName], Why: "mention", Msg: w.directMsg, Jump: true}
	if items[0].Item != want {
		return fmt.Errorf("jump item = %+v, want %+v", items[0].Item, want)
	}
	return w.assertRetired(w.firstItemID)
}

func (w *homeRoundsWorld) assertRetired(id int64) error {
	var count int
	err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM inbox WHERE id=?", id).Scan(&count)
	})
	if err != nil || count != 0 {
		return fmt.Errorf("retired item %d still present (%d, %v)", id, count, err)
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
	want := home.Item{AI: w.members[aiName], Why: "mention", Task: w.taskID, Msg: w.threadMsg, Jump: true}
	if items[0].Item != want {
		return fmt.Errorf("thread jump item = %+v, want %+v", items[0].Item, want)
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
	w.lastQuietID = items[0].ID
	if got := w.formatItem(items[0].Item); got != want {
		return fmt.Errorf("host received %q, want %q", got, want)
	}
	return nil
}

func (w *homeRoundsWorld) formatItem(it home.Item) string {
	name := it.AI
	for role, id := range w.members {
		if id == it.AI {
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
