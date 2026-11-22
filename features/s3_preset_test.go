//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["s3_preset"] = func(sc *godog.ScenarioContext, _ fixtures) { s3PresetSteps(sc) }
}

type s3PresetHomeBridge struct {
	loop *home.Loop
	conn *home.Conn
	ch   chan json.RawMessage
	done chan struct{}
}

func newS3PresetBridge(loop *home.Loop, device string) *s3PresetHomeBridge {
	b := &s3PresetHomeBridge{loop: loop, ch: make(chan json.RawMessage, 64), done: make(chan struct{})}
	b.conn = loop.Attach(device, b)
	return b
}

func (b *s3PresetHomeBridge) Send(frame any) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	var push struct {
		Type string `json:"t"`
	}
	if json.Unmarshal(raw, &push) == nil && push.Type != "" && push.Type != "fetched" && push.Type != "synced" {
		return nil
	}
	b.ch <- raw
	return nil
}

func (b *s3PresetHomeBridge) Done() <-chan struct{}        { return b.done }
func (b *s3PresetHomeBridge) Path() string                 { return "local" }
func (b *s3PresetHomeBridge) PathChanged() <-chan struct{} { return nil }

func (b *s3PresetHomeBridge) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	const reqID = 77
	req := build(reqID)
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var cmd proto.Cmd
	if proto.Decode(raw, &cmd) == nil && cmd.Type == "cmd" {
		b.loop.Cmd(b.conn, cmd.ID, cmd.Env)
		return <-b.ch, nil
	}
	var fk home.Fetch
	if proto.Decode(raw, &fk) == nil && fk.Type == "fetch" {
		b.loop.Fetch(b.conn, fk)
		return <-b.ch, nil
	}
	return nil, fmt.Errorf("unsupported bridge request: %s", raw)
}

type s3PresetWorld struct {
	today     time.Time
	now       time.Time
	db        *store.Store
	hostDB    *store.Store
	loop      *home.Loop
	h         *host.Host
	bridge    *s3PresetHomeBridge
	ownerConn *homeRunPeer
	hostConn  *homeRunPeer
	watchConn *homeRunPeer
	cleanup   func(func())
	browser   identity.Key
	node      identity.Key
	group     string
	ownerMID  string
	secID     string
	claudeID  string
	codexID   string
	nonce     int
	presetIDs map[string]string
	firstID   string
	lastBrief string
	lastTasks []proto.TaskRow
	oldMsgID  string
	forgotten string
}

func s3PresetSteps(sc *godog.ScenarioContext) {
	w := &s3PresetWorld{
		today:     time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
		presetIDs: map[string]string{},
	}
	w.cleanup = afterScenario(sc)
	sc.Step(`^a preset test group with retention set to (\d+) days$`, w.openGroup)
	sc.Step(`^the owner writes the preset "([^"]+)" with text "([^"]+)"$`, w.ownerWritesPreset)
	sc.Step(`^an AI writes the context note "([^"]+)"$`, w.aiWritesContextNote)
	sc.Step(`^note_list with type "([^"]+)" returns only "([^"]+)" with its updated text and original id$`, w.checkNoteListPreset)
	sc.Step(`^note_list without type returns only the context note "([^"]+)"$`, w.checkNoteListContext)
	sc.Step(`^the secretary proposes a two-task brief naming preset "([^"]+)" and the owner confirms it$`, w.proposeAndConfirmTwoTasks)
	sc.Step(`^the assign turn of every task of that brief holds preset "([^"]+)" unchanged and the review turn names it$`, w.checkTwoTaskTurns)
	sc.Step(`^the secretary proposes a brief without a preset and the owner confirms it$`, w.proposeAndConfirmNoPreset)
	sc.Step(`^the assign turn of that brief's task holds no style preset$`, w.checkNoPresetTurn)
	sc.Step(`^the secretary proposes a brief naming preset "([^"]+)"$`, w.proposeSingleBriefWithPreset)
	sc.Step(`^the owner forgets the preset "([^"]+)"$`, w.ownerForgetsPreset)
	sc.Step(`^the owner confirms the pending brief$`, w.confirmPendingBrief)
	sc.Step(`^the assign turn of that brief's task states that preset "([^"]+)" was deleted since the brief was proposed$`, w.checkDeletedPresetTurn)
	sc.Step(`^proposing a brief with unknown preset "([^"]+)" fails with "([^"]+)" listing "([^"]+)" and writes no brief row$`, w.checkUnknownPresetRefused)
	sc.Step(`^(\d+) days ago the owner wrote the preset "([^"]+)" and the text message "([^"]+)"$`, w.seedOldPresetAndMessage)
	sc.Step(`^an AI attempt to write preset "([^"]+)" is refused with "([^"]+)"$`, w.checkAIPresetForbidden)
	sc.Step(`^daily retention runs on the preset group today$`, w.runRetentionToday)
	sc.Step(`^the old text message "([^"]+)" is purged while preset "([^"]+)" remains$`, w.checkRetentionOutcome)
	sc.Step(`^a browser watches the preset group and the owner forgets the preset "([^"]+)"$`, w.watchAndForgetPreset)
	sc.Step(`^the browser receives a gone marker for that preset, fetch kind "preset" is empty, and the name "([^"]+)" can be created again$`, w.checkGoneMarkerAndRecreate)
}

func (w *s3PresetWorld) nextID(tag string) string {
	w.nonce++
	return string(exampleID(fmt.Sprintf("s3p-%s-%d", tag, w.nonce)))
}

func (w *s3PresetWorld) openGroup(days int) error {
	w.browser = exampleKey("s3p browser")
	w.node = exampleKey("s3p node")
	w.group = string(exampleID("s3p group"))
	w.secID = string(exampleID("s3p secretary"))
	w.claudeID = string(exampleID("s3p claude"))
	w.codexID = string(exampleID("s3p codex"))
	w.now = w.today

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { db.Close() })
	w.db = db
	if err := w.db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('retention_days', ?)", strconv.Itoa(days))
		return err
	}); err != nil {
		return err
	}

	loop, err := home.StartLoop(home.LoopConfig{
		Store:  w.db,
		Home:   w.node.ID(),
		Owner:  w.browser.ID(),
		Tables: home.NodeTables(),
		Logs: func(user string) *identity.Log {
			log, _ := homeLog([]identity.Key{w.browser, w.node})
			if log == nil || log.User != user {
				return nil
			}
			return log
		},
		Now: func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	w.cleanup(func() { loop.Close() })
	w.loop = loop
	w.ownerConn = s3AttachPeer(loop, w.browser)
	w.hostConn = s3AttachPeer(loop, w.node)

	hostDB, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	w.cleanup(func() { hostDB.Close() })
	w.hostDB = hostDB
	ownerVal, _ := json.Marshal(w.browser.ID())
	if err := hostDB.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", string(ownerVal))
		return err
	}); err != nil {
		return err
	}
	h, _, err := host.New(hostDB, w.node, func() time.Time { return w.now })
	if err != nil {
		return err
	}
	w.h = h
	w.bridge = newS3PresetBridge(loop, w.node.ID())
	h.Attach(w.node.ID(), w.bridge)

	return w.foundGroupMembers()
}

func (w *s3PresetWorld) foundGroupMembers() error {
	create := fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":"create","name":"Presets","kind":"project","repo":"git@example/repo","base":"main","forge":"none","checks":[],"member_name":"Owner"}`,
		w.group, w.now.UnixMilli(), w.group)
	if err := w.cmdOK(w.ownerConn, w.browser, create); err != nil {
		return err
	}
	for _, spec := range []struct {
		id, cli, name string
		sec           bool
	}{
		{w.secID, "claude", "Secretary", true},
		{w.claudeID, "claude", "Claude Code", false},
		{w.codexID, "codex", "Codex", false},
	} {
		add := fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"add_ai","id":%q,"cli":%q,"name":%q,"account":"personal","secretary":%v,"approvers":[%q]}`,
			w.nextID("mem"), w.now.UnixMilli(), w.group, spec.id, spec.cli, spec.name, spec.sec, w.browser.ID())
		if err := w.cmdOK(w.hostConn, w.node, add); err != nil {
			return err
		}
	}
	return w.syncHostState()
}

func (w *s3PresetWorld) syncHostState() error {
	snap, err := w.db.Snapshot(w.group)
	if err != nil {
		return err
	}
	defer snap.Close()
	rows, err := snap.StateRows()
	if err != nil {
		return err
	}
	w.h.Rows.Reset(w.group)
	w.lastTasks = nil
	for _, raw := range rows {
		var hdr proto.RowHeader
		if err := json.Unmarshal(raw, &hdr); err != nil {
			return err
		}
		w.h.Rows.Put(w.group, hdr.Type, hdr.ID, raw)
		if hdr.Type == "member" {
			var m proto.MemberRow
			if json.Unmarshal(raw, &m) == nil && m.Kind == "human" {
				w.ownerMID = m.ID
			}
		}
		if hdr.Type == "task" {
			var tr proto.TaskRow
			if json.Unmarshal(raw, &tr) == nil {
				w.lastTasks = append(w.lastTasks, tr)
			}
		}
	}
	return nil
}

func s3AttachPeer(loop *home.Loop, dev identity.Key) *homeRunPeer {
	p := &homeRunPeer{frames: make(chan any, 128)}
	p.conn = loop.Attach(dev.ID(), p)
	return p
}

func (w *s3PresetWorld) cmdResult(peer *homeRunPeer, signer identity.Key, payload string) (proto.CmdResult, *proto.ErrorResponse, error) {
	peer.re++
	re := peer.re
	w.loop.Cmd(peer.conn, re, signer.Sign(payload))
	for {
		raw, err := peer.next()
		if err != nil {
			return proto.CmdResult{}, nil, err
		}
		var errResp proto.ErrorResponse
		if proto.Decode([]byte(raw), &errResp) == nil && errResp.Re == re {
			return proto.CmdResult{}, &errResp, nil
		}
		var okResp proto.CmdResult
		if proto.Decode([]byte(raw), &okResp) == nil && okResp.Re == re {
			return okResp, nil, nil
		}
	}
}

func (w *s3PresetWorld) cmdOK(peer *homeRunPeer, signer identity.Key, payload string) error {
	_, errResp, err := w.cmdResult(peer, signer, payload)
	if err != nil {
		return err
	}
	if errResp != nil {
		return fmt.Errorf("unexpected cmd error %s: %s", errResp.Err.Code, errResp.Err.Message)
	}
	return nil
}

func (w *s3PresetWorld) ownerWritesPreset(name, text string) error {
	givenID := w.nextID("preset")
	cmd := proto.Note{
		HomeCommand: proto.HomeCommand{Type: "note", Nonce: proto.ID(w.nextID("n")), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.group)},
		ID:          proto.ID(givenID),
		NoteType:    "preset",
		Name:        name,
		Text:        text,
	}
	raw, _ := json.Marshal(cmd)
	res, errResp, err := w.cmdResult(w.ownerConn, w.browser, string(raw))
	if err != nil || errResp != nil {
		return fmt.Errorf("write preset err=%v resp=%+v", err, errResp)
	}
	if res.Replaced == nil {
		return fmt.Errorf("missing replaced field in result: %+v", res)
	}
	if prev, seen := w.presetIDs[name]; !seen {
		if *res.Replaced || res.ID != givenID {
			return fmt.Errorf("expected create result id=%s replaced=false, got %+v", givenID, res)
		}
		w.firstID = res.ID
		w.presetIDs[name] = res.ID
	} else if !*res.Replaced || res.ID != prev {
		return fmt.Errorf("expected replace result id=%s replaced=true, got %+v", prev, res)
	}
	return nil
}

func (w *s3PresetWorld) aiWritesContextNote(text string) error {
	caller := host.Caller{
		Session: host.Session{SessionKey: host.SessionKey{Group: w.group, Member: w.secID}, Kind: host.GroupSession, Home: w.node.ID(), Name: "Secretary", Secretary: true},
		Level:   1,
	}
	args, _ := json.Marshal(map[string]any{"text": text})
	_, err := host.Tools["note_put"].Call(context.Background(), w.h, caller, args)
	return err
}

func (w *s3PresetWorld) secCaller() host.Caller {
	return host.Caller{
		Session: host.Session{SessionKey: host.SessionKey{Group: w.group, Member: w.secID}, Kind: host.GroupSession, Home: w.node.ID(), Name: "Secretary", Secretary: true},
		Level:   1,
	}
}

func (w *s3PresetWorld) callNoteListJSON(rawArgs string) ([]map[string]any, error) {
	out, err := host.Tools["note_list"].Call(context.Background(), w.h, w.secCaller(), json.RawMessage(rawArgs))
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(out)
	var parsed struct {
		Notes []map[string]any `json:"notes"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return parsed.Notes, nil
}

func (w *s3PresetWorld) checkNoteListPreset(kind, name string) error {
	notes, err := w.callNoteListJSON(fmt.Sprintf(`{"type":%q}`, kind))
	if err != nil {
		return err
	}
	if len(notes) != 1 || notes[0]["name"] != name || notes[0]["id"] != w.firstID || notes[0]["text"] != "Principles: one accent colour; no motion longer than 200 ms." {
		return fmt.Errorf("unexpected preset note_list result: %+v", notes)
	}
	return nil
}

func (w *s3PresetWorld) checkNoteListContext(wantText string) error {
	notes, err := w.callNoteListJSON(`{}`)
	if err != nil {
		return err
	}
	if len(notes) != 1 || notes[0]["text"] != wantText {
		return fmt.Errorf("unexpected context note_list result: %+v", notes)
	}
	if _, hasName := notes[0]["name"]; hasName {
		return fmt.Errorf("context note unexpectedly carried name field: %+v", notes[0])
	}
	return nil
}

func (w *s3PresetWorld) proposeBriefViaTool(preset string, twoTasks bool) (string, error) {
	tasks := []map[string]any{
		{"title": "Build calm cards", "assignee": "Claude Code", "reviewer": "Codex"},
	}
	if twoTasks {
		tasks = append(tasks, map[string]any{"title": "Polish calm header", "assignee": "Codex", "reviewer": "Claude Code"})
	}
	input := map[string]any{
		"title":     "Dashboard polish",
		"goal":      "Keep the dashboard calm.",
		"criteria":  []string{"Uses one accent colour"},
		"scope_in":  []string{"web"},
		"scope_out": []string{"cli"},
		"tasks":     tasks,
	}
	if preset != "" {
		input["preset"] = preset
	}
	rawArgs, _ := json.Marshal(input)
	out, err := host.Tools["propose_brief"].Call(context.Background(), w.h, w.secCaller(), rawArgs)
	if err != nil {
		return "", err
	}
	rawOut, _ := json.Marshal(out)
	var idOut struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rawOut, &idOut)
	w.lastBrief = idOut.ID
	return idOut.ID, nil
}

func (w *s3PresetWorld) confirmPendingBrief() error {
	act := fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":"confirm"}`,
		w.nextID("act"), w.now.UnixMilli(), w.group, w.lastBrief)
	if err := w.cmdOK(w.ownerConn, w.browser, act); err != nil {
		return err
	}
	return w.syncHostState()
}

func (w *s3PresetWorld) proposeAndConfirmTwoTasks(preset string) error {
	if _, err := w.proposeBriefViaTool(preset, true); err != nil {
		return err
	}
	return w.confirmPendingBrief()
}

func (w *s3PresetWorld) composeTaskAssign(task proto.TaskRow) (string, error) {
	sess := host.Session{
		SessionKey: host.SessionKey{Group: w.group, Member: task.Assignee, Task: task.ID},
		Kind:       host.TaskSession,
		Home:       w.node.ID(),
		Name:       "Assignee",
	}
	itemRaw, _ := json.Marshal(proto.InboxItem{
		Item: proto.Item{Item: 1, AI: proto.ID(task.Assignee), Why: "assign", Task: proto.ID(task.ID), Msg: proto.ID(task.ID)},
	})
	text, _, err := host.Compose(w.h, sess, host.ItemsTurn, []json.RawMessage{itemRaw})
	return text, err
}

func (w *s3PresetWorld) checkTwoTaskTurns(preset string) error {
	var briefTasks []proto.TaskRow
	for _, t := range w.lastTasks {
		if t.Brief == w.lastBrief {
			briefTasks = append(briefTasks, t)
		}
	}
	if len(briefTasks) != 2 {
		return fmt.Errorf("expected 2 tasks for brief %s, got %d", w.lastBrief, len(briefTasks))
	}
	wantBlock := fmt.Sprintf("Style preset %q:\nPrinciples: one accent colour; no motion longer than 200 ms.\nNotes of this group:\n- Keep routes flat.", preset)
	for _, t := range briefTasks {
		turnText, err := w.composeTaskAssign(t)
		if err != nil {
			return err
		}
		if !strings.Contains(turnText, wantBlock) {
			return fmt.Errorf("task T-%d assign turn missing preset block:\n%s", t.Num, turnText)
		}
		revSess := host.Session{
			SessionKey: host.SessionKey{Group: w.group, Member: t.Reviewer, Task: t.ID},
			Kind:       host.ReviewSession,
			Home:       w.node.ID(),
			Name:       "Reviewer",
		}
		revRaw, _ := json.Marshal(proto.InboxItem{
			Item: proto.Item{Item: 2, AI: proto.ID(t.Reviewer), Why: "review", Task: proto.ID(t.ID), Commit: "abc1234"},
		})
		revText, _, err := host.Compose(w.h, revSess, host.ItemsTurn, []json.RawMessage{revRaw})
		if err != nil {
			return err
		}
		if !strings.HasSuffix(revText, fmt.Sprintf("Style preset: %q", preset)) {
			return fmt.Errorf("task T-%d review turn missing preset name:\n%s", t.Num, revText)
		}
	}
	return nil
}

func (w *s3PresetWorld) proposeAndConfirmNoPreset() error {
	if _, err := w.proposeBriefViaTool("", false); err != nil {
		return err
	}
	return w.confirmPendingBrief()
}

func (w *s3PresetWorld) singleBriefTask() (proto.TaskRow, error) {
	for _, t := range w.lastTasks {
		if t.Brief == w.lastBrief {
			return t, nil
		}
	}
	return proto.TaskRow{}, fmt.Errorf("no task found for brief %s", w.lastBrief)
}

func (w *s3PresetWorld) checkNoPresetTurn() error {
	t, err := w.singleBriefTask()
	if err != nil {
		return err
	}
	turnText, err := w.composeTaskAssign(t)
	if err != nil {
		return err
	}
	if strings.Contains(turnText, "Style preset") {
		return fmt.Errorf("brief without preset produced Style preset in turn:\n%s", turnText)
	}
	return nil
}

func (w *s3PresetWorld) proposeSingleBriefWithPreset(preset string) error {
	_, err := w.proposeBriefViaTool(preset, false)
	return err
}

func (w *s3PresetWorld) ownerForgetsPreset(name string) error {
	id, ok := w.presetIDs[name]
	if !ok {
		return fmt.Errorf("unknown preset %q", name)
	}
	w.forgotten = id
	delete(w.presetIDs, name)
	forget := fmt.Sprintf(`{"t":"forget","n":%q,"ts":%d,"g":%q,"r":"note","id":%q}`,
		w.nextID("forget"), w.now.UnixMilli(), w.group, id)
	return w.cmdOK(w.ownerConn, w.browser, forget)
}

func (w *s3PresetWorld) checkDeletedPresetTurn(preset string) error {
	t, err := w.singleBriefTask()
	if err != nil {
		return err
	}
	turnText, err := w.composeTaskAssign(t)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("Style preset %q: deleted since the brief was proposed.", preset)
	if !strings.Contains(turnText, want) {
		return fmt.Errorf("expected %q in turn:\n%s", want, turnText)
	}
	return nil
}

func (w *s3PresetWorld) checkUnknownPresetRefused(preset, code, validList string) error {
	_, err := w.proposeBriefViaTool(preset, false)
	he, ok := err.(*host.Error)
	if !ok || he.Code != code || !strings.Contains(he.Message, validList) {
		return fmt.Errorf("got err %#v; want code %q listing %q", err, code, validList)
	}
	var count int
	if err := w.db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND r='msg' AND data->>'kind'='brief'", w.group).Scan(&count)
	}); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("expected 0 brief rows, found %d", count)
	}
	return nil
}

func (w *s3PresetWorld) seedOldPresetAndMessage(daysAgo int, presetName, msgText string) error {
	w.now = w.today.Add(-time.Duration(daysAgo) * homeRetDay)
	if err := w.ownerWritesPreset(presetName, "Durable style rules."); err != nil {
		return err
	}
	msgID := w.nextID("oldmsg")
	w.oldMsgID = msgID
	post := fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"kind":"text","thread":null,"main":false,"text":%q,"mentions":[],"re":null,"body":{}}`,
		msgID, w.now.UnixMilli(), w.group, msgText)
	return w.cmdOK(w.ownerConn, w.browser, post)
}

func (w *s3PresetWorld) checkAIPresetForbidden(name, code string) error {
	cmd := fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q,"as":%q,"id":%q,"task":null,"type":"preset","name":%q,"text":"bad"}`,
		w.nextID("ainote"), w.now.UnixMilli(), w.group, w.claudeID, w.nextID("pid"), name)
	_, errResp, err := w.cmdResult(w.hostConn, w.node, cmd)
	if err != nil {
		return err
	}
	if errResp == nil || errResp.Err.Code != code {
		return fmt.Errorf("expected %s, got %+v", code, errResp)
	}
	return nil
}

func (w *s3PresetWorld) runRetentionToday() error {
	w.now = w.today
	return w.db.Write(func(tx *store.Tx) error {
		return home.Retain(tx, w.now.UnixMilli())
	})
}

func (w *s3PresetWorld) checkRetentionOutcome(_, presetName string) error {
	snap, err := w.db.Snapshot(w.group)
	if err != nil {
		return err
	}
	defer snap.Close()
	if _, err := snap.Row("msg", w.oldMsgID); err == nil {
		return fmt.Errorf("expected old message %s to be purged by retention", w.oldMsgID)
	}
	presets, err := snap.PageKind("preset", 0, 10)
	if err != nil {
		return err
	}
	if len(presets) != 1 || !strings.Contains(string(presets[0]), fmt.Sprintf(`"name":%q`, presetName)) {
		return fmt.Errorf("expected preset %q to survive retention, got %s", presetName, presets)
	}
	return nil
}

func (w *s3PresetWorld) watchAndForgetPreset(name string) error {
	w.watchConn = s3AttachPeer(w.loop, w.browser)
	w.loop.Sync(w.watchConn.conn, proto.Sync{
		Request: proto.Request{Type: "sync", ID: 10},
		Group:   w.group,
		Since:   0,
	})
	for {
		raw, err := w.watchConn.next()
		if err != nil {
			return err
		}
		if strings.Contains(raw, `"t":"synced"`) {
			break
		}
	}
	return w.ownerForgetsPreset(name)
}

func (w *s3PresetWorld) checkGoneMarkerAndRecreate(name string) error {
	pushed, err := w.watchConn.next()
	if err != nil {
		return err
	}
	var rowsPush proto.Rows
	if err := proto.Decode([]byte(pushed), &rowsPush); err != nil {
		return err
	}
	if len(rowsPush.Rows) != 1 {
		return fmt.Errorf("expected 1 pushed row, got %s", pushed)
	}
	var gone proto.GoneRow
	if err := proto.Decode(rowsPush.Rows[0], &gone); err != nil {
		return err
	}
	if gone.Type != "note" || gone.ID != w.forgotten || !gone.Gone {
		return fmt.Errorf("unexpected gone row: %+v", gone)
	}

	w.loop.Fetch(w.watchConn.conn, home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: 11}, Group: w.group, Before: 0, Limit: 10},
		Kind:      "preset",
	})
	fetchedRaw, err := w.watchConn.next()
	if err != nil {
		return err
	}
	var fetched proto.Fetched
	if err := proto.Decode([]byte(fetchedRaw), &fetched); err != nil {
		return err
	}
	if len(fetched.Rows) != 0 || fetched.More {
		return fmt.Errorf("expected empty preset fetch after forget, got %s", fetchedRaw)
	}

	return w.ownerWritesPreset(name, "Recreated after forget.")
}
