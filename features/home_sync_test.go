//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_sync"] = func(sc *godog.ScenarioContext, _ fixtures) { homeSyncSteps(sc) }
}

type homeSyncWorld struct {
	now     time.Time
	db      *store.Store
	loop    *home.Loop
	browser identity.Key
	laptop  identity.Key
	node    identity.Key
	group   string
	ownerID string
	aiID    string
	nonce   int
	cursor  int64
	got     homeSyncRead
	laptopP *homeSyncPeer
	slowP   *homeSyncPeer
}

type homeSyncPeer struct {
	conn    *home.Conn
	frames  chan any
	saw     trail
	started chan struct{}
	hold    chan struct{}
}

func homeSyncNewPeer() *homeSyncPeer {
	return &homeSyncPeer{frames: make(chan any, 4096), started: make(chan struct{}, 4096)}
}

func (p *homeSyncPeer) Send(frame any) error {
	p.started <- struct{}{}
	if p.hold != nil {
		<-p.hold
	}
	p.frames <- frame
	return nil
}

func (p *homeSyncPeer) Done() <-chan struct{} { return nil }

func (p *homeSyncPeer) next() (string, error) {
	frame, err := recvFrame(p.frames, 10*time.Second, &p.saw, briefAny)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(frame)
	return string(raw), err
}

type homeSyncRead struct {
	reset  bool
	rows   []json.RawMessage
	ids    []string
	kinds  []string
	seqs   []int64
	synced proto.Synced
}

func homeSyncSteps(sc *godog.ScenarioContext) {
	w := &homeSyncWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a group home with the group "([^"]*)" read by the owner's browser$`, func(name string) error {
		return w.openFounded(cleanup, ":memory:", name)
	})
	sc.Step(`^a group home on disk with the group "([^"]*)" and (\d+) messages$`, func(name string, count int) error {
		return w.openDiskWithMessages(cleanup, name, count)
	})
	sc.Step(`^the owner's browser disconnects and (\d+) messages are posted$`, w.postBulkMessages)
	sc.Step(`^a pending decision card posted before (\d+) main-line messages$`, w.postPendingThenMessages)
	sc.Step(`^a deleted message ages past retention and raises the purge mark above the browser's cursor$`, w.purgePastCursor)
	sc.Step(`^the owner's browser reconnects and syncs from its cursor$`, w.reconnectAndSync)
	sc.Step(`^the owner's browser is not told to reset$`, func() error { return w.assertReset(false) })
	sc.Step(`^the owner's browser is told to reset first$`, func() error { return w.assertReset(true) })
	sc.Step(`^the owner's browser receives those (\d+) messages in order before synced$`, w.assertMissedMessages)
	sc.Step(`^the owner's browser receives the state rows, the pending card and the newest (\d+) messages in sequence order$`, w.assertColdReset)
	sc.Step(`^the owner's laptop syncs the group with state_only$`, w.syncLaptopStateOnly)
	sc.Step(`^the owner's laptop is told to reset first$`, func() error { return w.assertReset(true) })
	sc.Step(`^the owner's laptop receives only the group's state rows before synced$`, w.assertOnlyStateRows)
	sc.Step(`^a message and a task update are written to the group$`, w.writeMsgAndTask)
	sc.Step(`^the owner's laptop is pushed the task row and not the message row$`, w.assertLaptopTaskPushOnly)
	sc.Step(`^the owner's browser starts a cold sync held on its first frame while another message is posted$`, w.syncConcurrentFirstRead)
	sc.Step(`^the owner's browser receives the rows up to synced, then synced, and then the new message once$`, w.assertConcurrentFirstRead)
}

func (w *homeSyncWorld) openFounded(cleanup func(func()), path, name string) error {
	w.browser, w.node, w.laptop = exampleKey("browser"), exampleKey("node"), exampleKey("laptop")
	w.group = string(exampleID("sync-group"))
	w.ownerID = string(exampleID("sync-owner"))
	w.aiID = string(exampleID("sync-ai"))
	db, err := store.Open(path)
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db
	if err := w.startLoop(cleanup); err != nil {
		return err
	}
	return w.seedInitialRead(name)
}

func (w *homeSyncWorld) openDiskWithMessages(cleanup func(func()), name string, count int) error {
	dir, err := os.MkdirTemp("", "solodawn-sync-*")
	if err != nil {
		return err
	}
	cleanup(func() { os.RemoveAll(dir) })
	if err := w.openFounded(cleanup, filepath.Join(dir, "node.db"), name); err != nil {
		return err
	}
	return w.postBulkMessages(count)
}

func (w *homeSyncWorld) startLoop(cleanup func(func())) error {
	tables := home.Tables{Commands: home.Table{
		"group/create": w.cmdCreate,
		"post/text":    w.cmdPostText,
		"post/card":    w.cmdPostCard,
		"forget":       w.cmdForget,
		"task/touch":   w.cmdTouchTask,
	}}
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
	cleanup(loop.Close)
	w.loop = loop
	return nil
}

func (w *homeSyncWorld) seedInitialRead(name string) error {
	if err := w.execCmd(w.laptop, "group", fmt.Sprintf(`,"op":"create","name":%q`, name)); err != nil {
		return err
	}
	p := homeSyncNewPeer()
	p.conn = w.loop.Attach(w.browser.ID(), p)
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 1}, Group: w.group, Since: 0})
	got, err := homeSyncCollect(p, 1)
	w.loop.Detach(p.conn)
	if err != nil {
		return err
	}
	w.cursor = got.synced.Seq
	return nil
}

func (w *homeSyncWorld) cmdCreate(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Name string `json:"name"`
	}
	if r := c.Decode(&p); r != nil {
		return home.Result{}, r
	}
	grp := proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: s.ID}, Name: p.Name, Kind: "project", Home: s.Home, Secretary: w.aiID, Checks: []string{}, Created: c.Now}
	hum := proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.ownerID}, Group: s.ID, Kind: "human", User: c.Who.User, Name: "owner"}, Role: "owner"}
	ai := proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.aiID}, Group: s.ID, Kind: "ai", User: c.Who.User, Name: "secretary"}, Host: w.laptop.ID()}
	return home.Result{Rows: []home.Row{home.RowOf(grp), home.RowOf(hum), home.RowOf(ai)}, GroupsChanged: true}, nil
}

func (w *homeSyncWorld) cmdPostText(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		ID   string `json:"msg_id"`
		Text string `json:"text"`
	}
	if r := c.Decode(&p); r != nil {
		return home.Result{}, r
	}
	msg := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: p.ID}, Group: s.ID, By: c.Who.Member(), Kind: "text", Timestamp: c.Now, Text: p.Text, Mentions: []string{}, Body: json.RawMessage(`{}`)}
	return home.Result{Rows: []home.Row{home.RowOf(msg)}}, nil
}

func (w *homeSyncWorld) cmdPostCard(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: "pending-card"}, Group: s.ID, Main: true,
		By: c.Who.Member(), Kind: "decision", Timestamp: c.Now, Text: "Decide",
		Mentions: []string{}, Body: json.RawMessage(`{}`), State: &proto.CardState{Status: "pending"},
	}
	return home.Result{Rows: []home.Row{home.RowOf(msg)}}, nil
}

func (w *homeSyncWorld) cmdForget(_ *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		ID string `json:"target"`
	}
	if r := c.Decode(&p); r != nil {
		return home.Result{}, r
	}
	return home.Result{Rows: []home.Row{home.RowOf(proto.GoneRow{RowHeader: proto.RowHeader{Type: "msg", ID: p.ID}, Gone: true})}}, nil
}

func (w *homeSyncWorld) cmdTouchTask(s *home.State, _ home.Command) (home.Result, *home.Refusal) {
	task := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: "task-live"}, Group: s.ID, Num: 1,
		Title: "Sync", Slug: proto.MakeSlug("Sync"), Origin: "direct", State: "active", Step: "branch",
		Assignee: w.aiID, Pass: 90, Depends: []string{}, Stats: proto.TaskStats{Started: w.now.UnixMilli()}, Created: w.now.UnixMilli(),
	}
	return home.Result{Rows: []home.Row{home.RowOf(task)}}, nil
}

func (w *homeSyncWorld) execCmd(by identity.Key, t, more string) error {
	p := homeSyncNewPeer()
	p.conn = w.loop.Attach(by.ID(), p)
	defer w.loop.Detach(p.conn)
	w.nonce++
	payload := fmt.Sprintf(`{"t":%q,"n":%q,"ts":%d,"g":%q%s}`, t, exampleID(fmt.Sprint("sync-", w.nonce)), w.now.UnixMilli(), w.group, more)
	w.loop.Cmd(p.conn, 1, by.Sign(payload))
	got, err := p.next()
	if err != nil || got != `{"re":1,"ok":true}` {
		return fmt.Errorf("command %s answered %s (%v)", t, got, err)
	}
	return nil
}

func (w *homeSyncWorld) postBulkMessages(count int) error {
	for i := 1; i <= count; i++ {
		more := fmt.Sprintf(`,"kind":"text","msg_id":"msg-%04d","text":"message %d"`, i, i)
		if err := w.execCmd(w.laptop, "post", more); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeSyncWorld) postPendingThenMessages(count int) error {
	if err := w.execCmd(w.laptop, "post", `,"kind":"card"`); err != nil {
		return err
	}
	return w.postBulkMessages(count)
}

func (w *homeSyncWorld) purgePastCursor() error {
	if err := w.postBulkMessages(1); err != nil {
		return err
	}
	if err := w.execCmd(w.laptop, "forget", `,"target":"msg-0001"`); err != nil {
		return err
	}
	return w.db.Write(func(tx *store.Tx) error {
		return tx.Retain(w.now.AddDate(0, 0, 2), 1)
	})
}

func (w *homeSyncWorld) reconnectAndSync() error {
	p := homeSyncNewPeer()
	p.conn = w.loop.Attach(w.browser.ID(), p)
	defer w.loop.Detach(p.conn)
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 2}, Group: w.group, Since: w.cursor})
	got, err := homeSyncCollect(p, 2)
	w.got = got
	return err
}

func (w *homeSyncWorld) syncLaptopStateOnly() error {
	p := homeSyncNewPeer()
	p.conn = w.loop.Attach(w.laptop.ID(), p)
	w.laptopP = p
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 3}, Group: w.group, Since: 0, StateOnly: true})
	got, err := homeSyncCollect(p, 3)
	w.got = got
	return err
}

func (w *homeSyncWorld) assertReset(want bool) error {
	if w.got.reset != want {
		return fmt.Errorf("reset = %v, want %v", w.got.reset, want)
	}
	return nil
}

func (w *homeSyncWorld) assertMissedMessages(count int) error {
	if len(w.got.ids) != count {
		return fmt.Errorf("got %d rows, want %d", len(w.got.ids), count)
	}
	for i := 1; i <= count; i++ {
		want := fmt.Sprintf("msg-%04d", i)
		if w.got.ids[i-1] != want {
			return fmt.Errorf("row %d id = %q, want %q", i-1, w.got.ids[i-1], want)
		}
	}
	return homeSyncIncreasing(w.got.seqs)
}

func (w *homeSyncWorld) assertColdReset(newest int) error {
	// 3 state rows + 1 older pending card + newest 50 main-line messages.
	wantTotal := 4 + newest
	if len(w.got.ids) != wantTotal {
		return fmt.Errorf("got %d rows, want %d: %v", len(w.got.ids), wantTotal, w.got.ids)
	}
	if w.got.ids[3] != "pending-card" || w.got.ids[4] != "msg-1451" || w.got.ids[wantTotal-1] != "msg-1500" {
		return fmt.Errorf("unexpected cold reset ids: first5=%v last=%s", w.got.ids[:5], w.got.ids[wantTotal-1])
	}
	return homeSyncIncreasing(w.got.seqs)
}

func (w *homeSyncWorld) assertOnlyStateRows() error {
	if len(w.got.kinds) != 3 {
		return fmt.Errorf("got %d rows (%v), want 3 state rows", len(w.got.kinds), w.got.kinds)
	}
	for _, kind := range w.got.kinds {
		if kind != "group" && kind != "member" {
			return fmt.Errorf("unexpected row type %q in state_only sync", kind)
		}
	}
	return homeSyncIncreasing(w.got.seqs)
}

func (w *homeSyncWorld) writeMsgAndTask() error {
	if err := w.execCmd(w.browser, "post", `,"kind":"text","msg_id":"msg-after-host","text":"chat"`); err != nil {
		return err
	}
	return w.execCmd(w.browser, "task", `,"op":"touch"`)
}

func (w *homeSyncWorld) assertLaptopTaskPushOnly() error {
	h, err := homeSyncSinglePushedRow(w.laptopP)
	if err != nil {
		return err
	}
	if h.Type != "task" || h.ID != "task-live" {
		return fmt.Errorf("got pushed row %+v, want task-live", h)
	}
	return nil
}

func homeSyncSinglePushedRow(p *homeSyncPeer) (proto.RowHeader, error) {
	raw, err := p.next()
	if err != nil {
		return proto.RowHeader{}, err
	}
	var batch proto.Rows
	if err := proto.Decode([]byte(raw), &batch); err != nil {
		return proto.RowHeader{}, err
	}
	if len(batch.Rows) != 1 {
		return proto.RowHeader{}, fmt.Errorf("got %d rows in push: %s", len(batch.Rows), raw)
	}
	var h proto.RowHeader
	err = json.Unmarshal(batch.Rows[0], &h)
	return h, err
}

func (w *homeSyncWorld) syncConcurrentFirstRead() error {
	p := homeSyncNewPeer()
	p.hold = make(chan struct{})
	p.conn = w.loop.Attach(w.browser.ID(), p)
	w.slowP = p
	w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: 9}, Group: w.group, Since: 0})
	started := newWait(10*time.Second, "the first frame to start", nil)
	defer started.stop()
	if _, err := recvWait(started, p.started); err != nil {
		return err
	}
	err := w.execCmd(w.laptop, "post", `,"kind":"text","msg_id":"msg-concurrent","text":"during sync"`)
	close(p.hold)
	if err != nil {
		return err
	}
	got, err := homeSyncCollect(p, 9)
	w.got = got
	return err
}

func (w *homeSyncWorld) assertConcurrentFirstRead() error {
	if err := w.assertCutAtEight(); err != nil {
		return err
	}
	h, err := homeSyncSinglePushedRow(w.slowP)
	if err != nil {
		return err
	}
	if h.ID != "msg-concurrent" || h.Seq != 9 {
		return fmt.Errorf("got live row %+v, want msg-concurrent at seq 9", h)
	}
	return homeSyncNoExtraFrame(w.slowP, w.got.seqs)
}

func (w *homeSyncWorld) assertCutAtEight() error {
	if !w.got.reset || w.got.synced.Seq != 8 || len(w.got.ids) != 8 {
		return fmt.Errorf("unexpected cut: reset=%v seq=%d ids=%v", w.got.reset, w.got.synced.Seq, w.got.ids)
	}
	return nil
}

func homeSyncNoExtraFrame(p *homeSyncPeer, seqs []int64) error {
	select {
	case extra := <-p.frames:
		return fmt.Errorf("unexpected extra frame: %+v", extra)
	default:
		return homeSyncIncreasing(seqs)
	}
}

func homeSyncCollect(p *homeSyncPeer, re int64) (homeSyncRead, error) {
	var out homeSyncRead
	for {
		raw, err := p.next()
		if err != nil {
			return out, err
		}
		done, err := homeSyncStepFrame(raw, re, &out)
		if err != nil || done {
			return out, err
		}
	}
}

func homeSyncStepFrame(raw string, re int64, out *homeSyncRead) (bool, error) {
	var kind struct {
		Type string `json:"t"`
	}
	if err := json.Unmarshal([]byte(raw), &kind); err != nil {
		return false, err
	}
	switch kind.Type {
	case "reset":
		out.reset = true
		return false, nil
	case "rows":
		return false, homeSyncAddRows(raw, out)
	case "synced":
		return true, homeSyncDecodeSynced(raw, re, &out.synced)
	default:
		return false, fmt.Errorf("unexpected frame: %s", raw)
	}
}

func homeSyncDecodeSynced(raw string, re int64, dst *proto.Synced) error {
	if err := proto.Decode([]byte(raw), dst); err != nil {
		return err
	}
	if dst.Re != re {
		return fmt.Errorf("synced re = %d, want %d", dst.Re, re)
	}
	return nil
}

func homeSyncAddRows(raw string, out *homeSyncRead) error {
	var batch proto.Rows
	if err := proto.Decode([]byte(raw), &batch); err != nil {
		return err
	}
	for _, r := range batch.Rows {
		var h proto.RowHeader
		if err := json.Unmarshal(r, &h); err != nil {
			return err
		}
		out.rows = append(out.rows, r)
		out.ids = append(out.ids, h.ID)
		out.kinds = append(out.kinds, h.Type)
		out.seqs = append(out.seqs, h.Seq)
	}
	return nil
}

func homeSyncIncreasing(seqs []int64) error {
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			return fmt.Errorf("sequence numbers not strictly increasing: %v", seqs)
		}
	}
	return nil
}
