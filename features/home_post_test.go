//go:build acceptance

package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
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
	stepSets["home_post"] = func(sc *godog.ScenarioContext, _ fixtures) { homePostSteps(sc) }
}

type homePostPeer struct {
	key    identity.Key
	conn   *home.Conn
	frames chan any
	re     int64
}

func (p *homePostPeer) Send(frame any) error {
	p.frames <- frame
	return nil
}

func (p *homePostPeer) Done() <-chan struct{} { return nil }

func (p *homePostPeer) next() (string, error) {
	select {
	case frame := <-p.frames:
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		err := enc.Encode(frame)
		return strings.TrimSuffix(b.String(), "\n"), err
	case <-time.After(time.Second):
		return "", fmt.Errorf("no frame arrived within 1 second")
	}
}

type homePostWorld struct {
	clock     atomic.Int64
	db        *store.Store
	loop      *home.Loop
	group     string
	task      string
	members   map[string]string
	peers     map[string]*homePostPeer
	oldText   string
	oldReview string
	oldNote   string
	lastMsg   string
	lastErr   string
	nonce     int
}

func homePostSteps(sc *godog.ScenarioContext) {
	w := &homePostWorld{}
	cleanup := afterScenario(sc)
	sc.Step(`^a group home with two connected browsers of its owner, a secretary and two working AIs on its host$`, func() error { return w.start(cleanup) })
	sc.Step(`^the first browser posts the text "([^"]*)" in the main line$`, w.browserPostsText)
	sc.Step(`^the second browser receives that "([^"]*)" message in a live push$`, w.secondGetsMainMsg)
	sc.Step(`^the host receives "([^"]*)" for that message$`, w.hostGetsItem)
	sc.Step(`^"([^"]*)" posts the progress "([^"]*)" in the thread of task 1 mentioning "([^"]*)"$`, w.aiPostsProgress)
	sc.Step(`^the second browser receives that "([^"]*)" message and task 1 with (\d+) thread messages?$`, w.secondGetsThreadMsg)
	sc.Step(`^"([^"]*)" posts a completion report for commit "([^"]*)" in the thread of task 1$`, w.aiPostsReport)
	sc.Step(`^the group holds an old text message, an old review message and an old note$`, w.seedMemory)
	sc.Step(`^(\d+) days pass and daily retention runs for a (\d+)-day window$`, w.runRetention)
	sc.Step(`^the old text message is gone from the store while the old review message and the old note remain$`, w.checkRetained)
	sc.Step(`^the first browser forgets the old (note|review message)$`, w.forgetOld)
	sc.Step(`^the second browser receives a gone marker for that "(note|msg)"$`, w.secondGetsGone)
	sc.Step(`^(a non-admin member tries to forget the old note|the first browser tries to forget the old text message|"ai-two" tries to post a completion report in the thread of task 1|"ai-one" tries to post progress in the main line)$`, w.attemptRefusal)
	sc.Step(`^the post home answers "([^"]*)"$`, w.checkRefusal)
}

func (w *homePostWorld) now() time.Time { return time.UnixMilli(w.clock.Load()) }

func (w *homePostWorld) nextID() proto.ID {
	w.nonce++
	return exampleID("home_post nonce " + strconv.Itoa(w.nonce))
}

func (w *homePostWorld) start(cleanup func(func())) error {
	w.clock.Store(1763600000000)
	w.nonce, w.lastErr = 0, ""
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db

	b1, b2 := exampleKey("post browser 1"), exampleKey("post browser 2")
	node, host, guest := exampleKey("post node"), exampleKey("post host"), exampleKey("post guest")
	if err := w.seedGroup(b1, node, host, guest); err != nil {
		return err
	}
	return w.bootLoop(cleanup, b1, b2, node, host, guest)
}

func (w *homePostWorld) seedGroup(b1, node, host, guest identity.Key) error {
	w.group = string(exampleID("post group"))
	w.task = string(exampleID("post task 1"))
	w.members = map[string]string{}
	for _, name := range []string{"owner", "guest", "secretary", "ai-one", "ai-two"} {
		w.members[name] = string(exampleID("post member " + name))
	}
	guestEntry, err := logEntry(guest, guest, 1, "add", guest)
	if err != nil {
		return err
	}
	return w.db.Write(func(tx *store.Tx) error { return w.writeInitialRows(tx, b1, node, host, guest, guestEntry) })
}

func (w *homePostWorld) writeInitialRows(tx *store.Tx, b1, node, host, guest identity.Key, guestEntry proto.Envelope) error {
	ms := w.clock.Load()
	rows := []home.Row{
		home.RowOf(proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: w.group}, Name: "Website", Kind: "project",
			Home: node.ID(), Secretary: w.members["secretary"], Forge: "none", Pass: 90, PassFoundation: 70,
			Checks: []string{}, Created: ms,
		}),
		home.RowOf(proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["owner"]}, Group: w.group, Kind: "human", User: b1.ID(), Name: "owner"},
			Role:      "owner",
		}),
		home.RowOf(proto.HumanMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["guest"]}, Group: w.group, Kind: "human", User: guest.ID(), Name: "guest"},
			Role:      "member",
		}),
		home.RowOf(proto.DeviceRow{
			RowHeader:    proto.RowHeader{Type: "device", ID: guest.ID() + ":1"},
			DeviceChange: proto.DeviceChange{User: guest.ID(), Version: 1, Op: "add", Device: guest.ID(), Kind: "browser", Name: "guest"},
			Env:          guestEntry,
		}),
	}
	for _, name := range []string{"secretary", "ai-one", "ai-two"} {
		rows = append(rows, home.RowOf(proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members[name]}, Group: w.group, Kind: "ai", User: b1.ID(), Name: name},
			CLI:       "claude", Host: host.ID(), Account: "personal", Allow: []string{}, Approvers: []string{b1.ID()}, State: "waiting", Path: "direct",
		}))
	}
	rows = append(rows, home.RowOf(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.task}, Group: w.group, Num: 1, Title: "Show path",
		Slug: proto.MakeSlug("Show path"), Origin: "brief", Brief: string(exampleID("post brief")),
		State: "active", Step: "local_check", Assignee: w.members["ai-one"], Reviewer: w.members["ai-two"],
		Pass: 90, Depends: []string{}, Stats: proto.TaskStats{Started: ms}, Created: ms,
	}))
	return w.putSeeded(tx, rows, ms)
}

func (w *homePostWorld) putSeeded(tx *store.Tx, rows []home.Row, ms int64) error {
	s := home.NewState(w.group, "", "")
	sealed := s.Seal(home.Result{Rows: rows})
	for _, r := range sealed.Rows {
		if _, err := tx.NextSeq(w.group); err != nil {
			return err
		}
		if err := r.Put(tx, w.group, ms); err != nil {
			return err
		}
	}
	return tx.RaiseNum(w.group, 1)
}

func (w *homePostWorld) bootLoop(cleanup func(func()), b1, b2, node, host, guest identity.Key) error {
	logs := func(user string) *identity.Log {
		if user != b1.ID() {
			return nil
		}
		log, _ := homeLog([]identity.Key{b1, b2, node, host})
		return log
	}
	tables := home.NodeTables()
	tables.Events = home.Rules[home.Handler]{"seed_review": w.seedReviewEvent}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: node.ID(), Owner: b1.ID(), Logs: logs, Tables: tables, Now: w.now,
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
	w.peers = map[string]*homePostPeer{}
	for name, key := range map[string]identity.Key{"b1": b1, "b2": b2, "host": host, "guest": guest} {
		p := &homePostPeer{key: key, frames: make(chan any, 64)}
		p.conn = loop.Attach(key.ID(), p)
		w.peers[name] = p
	}
	loop.Watch(w.peers["b1"].conn, w.group, false)
	loop.Watch(w.peers["b2"].conn, w.group, false)
	loop.Inbox(w.peers["host"].conn)
	return nil
}

func (w *homePostWorld) sendOK(who string, payload any) error {
	raw, err := w.sendRaw(who, payload)
	if err != nil {
		return err
	}
	want := fmt.Sprintf(`{"re":%d,"ok":true}`, w.peers[who].re)
	if raw != want {
		return fmt.Errorf("%s answered %s; want %s", who, raw, want)
	}
	if who == "b1" {
		_, err = w.peers["b1"].next()
	}
	return err
}

func (w *homePostWorld) sendRaw(who string, payload any) (string, error) {
	text, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	p := w.peers[who]
	p.re++
	w.loop.Cmd(p.conn, p.re, p.key.Sign(string(text)))
	return p.next()
}

func (w *homePostWorld) browserPostsText(text string) error {
	id := w.nextID()
	w.lastMsg = string(id)
	cmd := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: id, Timestamp: w.clock.Load(), Group: proto.ID(w.group)},
		Kind:        "text", Text: text, Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
	return w.sendOK("b1", cmd)
}

func (w *homePostWorld) aiPostsProgress(ai, text, mentioned string) error {
	id, thread := w.nextID(), proto.ID(w.task)
	w.lastMsg = string(id)
	cmd := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: id, Timestamp: w.clock.Load(), Group: proto.ID(w.group), As: proto.ID(w.members[ai])},
		Kind:        "progress", Thread: &thread, Text: text, Mentions: []proto.ID{proto.ID(w.members[mentioned])}, Body: proto.EmptyBody{},
	}
	return w.sendOK("host", cmd)
}

func (w *homePostWorld) aiPostsReport(ai, commit string) error {
	id, thread := w.nextID(), proto.ID(w.task)
	w.lastMsg = string(id)
	cmd := proto.Post[proto.ReportBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: id, Timestamp: w.clock.Load(), Group: proto.ID(w.group), As: proto.ID(w.members[ai])},
		Kind:        "report", Thread: &thread, Mentions: []proto.ID{}, Body: proto.ReportBody{Commit: commit, Files: 2, Added: 20, Removed: 4},
	}
	return w.sendOK("host", cmd)
}

func (w *homePostWorld) secondGetsMainMsg(kind string) error {
	rows, err := w.readRows("b2", 1)
	if err != nil {
		return err
	}
	var msg proto.MsgRow
	if err := json.Unmarshal(rows[0], &msg); err != nil {
		return err
	}
	if msg.ID != w.lastMsg || msg.Kind != kind || msg.Thread != nil {
		return fmt.Errorf("got message %+v; want %s (%s) in main line", msg, w.lastMsg, kind)
	}
	return nil
}

func (w *homePostWorld) secondGetsThreadMsg(kind string, count int) error {
	rows, err := w.readRows("b2", 2)
	if err != nil {
		return err
	}
	var msg proto.MsgRow
	var task proto.TaskRow
	if err := json.Unmarshal(rows[0], &msg); err != nil {
		return err
	}
	if err := json.Unmarshal(rows[1], &task); err != nil {
		return err
	}
	lastKind := ""
	if task.Thread.Last != nil {
		lastKind = task.Thread.Last.Kind
	}
	if !reflect.DeepEqual([]any{msg.ID, msg.Kind, task.Thread.Count, lastKind}, []any{w.lastMsg, kind, count, kind}) {
		return fmt.Errorf("got msg %+v and task thread %+v", msg, task.Thread)
	}
	return nil
}

func (w *homePostWorld) readRows(who string, want int) ([]json.RawMessage, error) {
	raw, err := w.peers[who].next()
	if err != nil {
		return nil, err
	}
	var push proto.Rows
	if err := json.Unmarshal([]byte(raw), &push); err != nil || push.Type != "rows" || len(push.Rows) != want {
		return nil, fmt.Errorf("got %s (%v); want %d rows", raw, err, want)
	}
	return push.Rows, nil
}

func (w *homePostWorld) hostGetsItem(spec string) error {
	raw, err := w.peers["host"].next()
	if err != nil {
		return err
	}
	var push proto.Inbox
	if err := proto.Decode([]byte(raw), &push); err != nil {
		return err
	}
	if len(push.Items) != 1 {
		return fmt.Errorf("host got %s", raw)
	}
	it := push.Items[0].Item
	if !reflect.DeepEqual([]string{w.itemSpec(it), string(it.Msg)}, []string{spec, w.lastMsg}) {
		return fmt.Errorf("host item %+v; want %q for %s", it, spec, w.lastMsg)
	}
	return nil
}

func (w *homePostWorld) itemSpec(it proto.Item) string {
	got := it.Why + " " + w.memberName(string(it.AI))
	if string(it.Task) == w.task {
		got += " task 1"
	}
	return got
}

func (w *homePostWorld) memberName(id string) string {
	for name, mid := range w.members {
		if mid == id {
			return name
		}
	}
	return id
}

func (w *homePostWorld) seedMemory() error {
	if err := w.browserPostsText("Old discussion."); err != nil {
		return err
	}
	if err := w.secondGetsMainMsg("text"); err != nil {
		return err
	}
	if _, err := w.peers["host"].next(); err != nil {
		return err
	}
	w.oldText = w.lastMsg

	noteID := w.nextID()
	w.oldNote = string(noteID)
	noteCmd := proto.Note{
		HomeCommand: proto.HomeCommand{Type: "note", Nonce: w.nextID(), Timestamp: w.clock.Load(), Group: proto.ID(w.group)},
		ID:          noteID, Text: "Path events carry the connection id.",
	}
	if err := w.sendOK("b1", noteCmd); err != nil {
		return err
	}
	if _, err := w.readRows("b2", 1); err != nil {
		return err
	}
	return w.putOldReview()
}

func (w *homePostWorld) putOldReview() error {
	w.oldReview = string(w.nextID())
	w.loop.Event("seed_review", w.oldReview)
	if _, err := w.readRows("b1", 1); err != nil {
		return err
	}
	_, err := w.readRows("b2", 1)
	return err
}

func (w *homePostWorld) seedReviewEvent(_ *home.State, _ home.Disk, ms int64, ev any) home.Result {
	thread := w.task
	row := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: ev.(string)}, Group: w.group,
		Thread: &thread, By: w.members["ai-two"], Kind: "review", Timestamp: ms, Mentions: []string{},
		Body: json.RawMessage(`{"task":"` + w.task + `","num":1,"title":"Show path","commit":"c1","round":1,"score":95,"pass":90,"ok":true,"items":[]}`),
	}
	return home.Result{Rows: []home.Row{home.RowOf(row)}}
}

func (w *homePostWorld) runRetention(elapsedDays, windowDays int) error {
	w.clock.Add(int64(elapsedDays) * 24 * int64(time.Hour/time.Millisecond))
	return w.db.Write(func(tx *store.Tx) error { return tx.Retain(w.now(), windowDays) })
}

func (w *homePostWorld) checkRetained() error {
	return w.db.Write(func(tx *store.Tx) error {
		_, errText := tx.Row(w.group, "msg", w.oldText)
		_, errReview := tx.Row(w.group, "msg", w.oldReview)
		_, errNote := tx.Row(w.group, "note", w.oldNote)
		if errText == nil || errReview != nil || errNote != nil {
			return fmt.Errorf("after retention: text=%v, review=%v, note=%v", errText, errReview, errNote)
		}
		return nil
	})
}

func (w *homePostWorld) forgetOld(which string) error {
	r, id := "note", w.oldNote
	if which == "review message" {
		r, id = "msg", w.oldReview
	}
	w.lastMsg = id
	cmd := proto.Forget{
		HomeCommand: proto.HomeCommand{Type: "forget", Nonce: w.nextID(), Timestamp: w.clock.Load(), Group: proto.ID(w.group)},
		Row:         r, ID: proto.ID(id),
	}
	return w.sendOK("b1", cmd)
}

func (w *homePostWorld) secondGetsGone(r string) error {
	rows, err := w.readRows("b2", 1)
	if err != nil {
		return err
	}
	var gone proto.GoneRow
	if err := json.Unmarshal(rows[0], &gone); err != nil {
		return err
	}
	if gone.Type != r || gone.ID != w.lastMsg || !gone.Gone {
		return fmt.Errorf("got marker %+v; want %s %s gone", gone, r, w.lastMsg)
	}
	return nil
}

func (w *homePostWorld) attemptRefusal(attempt string) error {
	who, payload := w.refusalCommand(attempt)
	raw, err := w.sendRaw(who, payload)
	w.lastErr = raw
	return err
}

func (w *homePostWorld) refusalCommand(attempt string) (string, any) {
	ts, g, thread := w.clock.Load(), proto.ID(w.group), proto.ID(w.task)
	switch attempt {
	case "a non-admin member tries to forget the old note":
		return "guest", proto.Forget{
			HomeCommand: proto.HomeCommand{Type: "forget", Nonce: w.nextID(), Timestamp: ts, Group: g},
			Row:         "note", ID: proto.ID(w.oldNote),
		}
	case "the first browser tries to forget the old text message":
		return "b1", proto.Forget{
			HomeCommand: proto.HomeCommand{Type: "forget", Nonce: w.nextID(), Timestamp: ts, Group: g},
			Row:         "msg", ID: proto.ID(w.oldText),
		}
	case `"ai-two" tries to post a completion report in the thread of task 1`:
		return "host", proto.Post[proto.ReportBody]{
			HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: ts, Group: g, As: proto.ID(w.members["ai-two"])},
			Kind:        "report", Thread: &thread, Mentions: []proto.ID{}, Body: proto.ReportBody{Commit: "c1", Files: 1},
		}
	default:
		return "host", proto.Post[proto.EmptyBody]{
			HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: ts, Group: g, As: proto.ID(w.members["ai-one"])},
			Kind:        "progress", Mentions: []proto.ID{}, Body: proto.EmptyBody{},
		}
	}
}

func (w *homePostWorld) checkRefusal(code string) error {
	var resp proto.ErrorResponse
	if err := json.Unmarshal([]byte(w.lastErr), &resp); err != nil || resp.Err.Code != code || resp.Err.Message == "" {
		return fmt.Errorf("got %s (%v); want error %s", w.lastErr, err, code)
	}
	return nil
}
