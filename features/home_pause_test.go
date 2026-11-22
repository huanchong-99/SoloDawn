//go:build acceptance

package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	stepSets["home_pause"] = func(sc *godog.ScenarioContext, _ fixtures) { homePauseSteps(sc) }
}

type homePausePeer struct {
	key    identity.Key
	conn   *home.Conn
	client homeFrameClient
	re     int64
}

func (p *homePausePeer) next() ([]byte, error) {
	select {
	case raw := <-p.client.ch:
		return raw, nil
	case <-time.After(2 * time.Second):
		return nil, errors.New("timed out waiting for frame")
	}
}

type homePauseSeed struct {
	task   proto.TaskRow
	review *home.Item
}

type homePauseWorld struct {
	now       time.Time
	db        *store.Store
	loop      *home.Loop
	group     string
	members   map[string]string
	tasks     map[string]string
	peers     map[string]*homePausePeer
	watcher   *homePausePeer
	lastResp  []byte
	lastRows  []json.RawMessage
	lastItems []proto.InboxItem
	rowDelta  int
	itemDelta int
	nonce     int
}

func homePauseSteps(sc *godog.ScenarioContext) {
	w := &homePauseWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a group for pause and resume with an owner, an admin, a member and three working AIs$`, func() error {
		return w.start(cleanup)
	})
	sc.Step(`^"([^"]+)" has paused the group$`, func(who string) error {
		if err := w.sendGroup(who, "pause"); err != nil {
			return err
		}
		return w.answered("ok")
	})
	homePauseTaskSteps(sc, w)
	sc.Step(`^"([^"]+)" sends group "(pause|resume)"$`, w.sendGroup)
	sc.Step(`^"([^"]+)" posts a text mentioning "([^"]+)" while the group is paused$`, w.postMention)
	sc.Step(`^the pause home answers "([^"]+)"$`, w.answered)
	sc.Step(`^the pause home answers "([^"]+)" with no rows and no items$`, w.answeredIdle)
	sc.Step(`^the group is paused by "([^"]+)" with system message "([^"]+)"$`, w.pausedBy)
	sc.Step(`^the group is unpaused with system message "([^"]+)" by "([^"]+)"$`, w.unpausedBy)
	sc.Step(`^the pause home has queued "([^"]*)"$`, w.checkQueued)
}

func homePauseTaskSteps(sc *godog.ScenarioContext, w *homePauseWorld) {
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)"$`, func(name, step, ai string) error {
		return w.putTask(name, "active", step, ai, "", "", nil, false)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)" waiting for an approval$`, func(name, step, ai string) error {
		return w.putTask(name, "active", step, ai, "", "", &proto.TaskWait{Kind: "approval", Since: w.now.UnixMilli() - 3000}, false)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)" for commit "([^"]+)"$`, func(name, step, ai, commit string) error {
		return w.putTask(name, "active", step, ai, "", commit, nil, false)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)"$`, func(name, ai, reviewer, commit string) error {
		return w.putTask(name, "active", "review", ai, reviewer, commit, nil, false)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)" with its review already queued$`, func(name, ai, reviewer, commit string) error {
		return w.putTask(name, "active", "review", ai, reviewer, commit, nil, true)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)" waiting for an approval$`, func(name, ai, reviewer, commit string) error {
		return w.putTask(name, "active", "review", ai, reviewer, commit, &proto.TaskWait{Kind: "approval", Since: w.now.UnixMilli() - 3000}, false)
	})
	sc.Step(`^a paused pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)"$`, func(name, ai, reviewer, commit string) error {
		return w.putTask(name, "paused", "review", ai, reviewer, commit, &proto.TaskWait{Kind: "manual", Since: w.now.UnixMilli() - 3000}, false)
	})
}

func (w *homePauseWorld) start(cleanup func(func())) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db
	w.members, w.tasks, w.nonce = map[string]string{}, map[string]string{}, 0
	for _, name := range []string{"owner", "admin", "member", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("pause " + name))
	}
	w.group = string(exampleID("pause group"))

	ownerDev, watchDev := exampleKey("pause owner"), exampleKey("pause watcher")
	nodeDev, laptopDev := exampleKey("pause node"), exampleKey("pause laptop")
	adminDev, memberDev := exampleKey("pause admin"), exampleKey("pause member")
	if err := w.seedStore(ownerDev, nodeDev, laptopDev, adminDev, memberDev); err != nil {
		return err
	}
	return w.bootLoop(cleanup, ownerDev, watchDev, nodeDev, laptopDev, adminDev, memberDev)
}

func (w *homePauseWorld) seedStore(ownerDev, nodeDev, laptopDev, adminDev, memberDev identity.Key) error {
	adminEntry, err1 := logEntry(adminDev, adminDev, 1, "add", adminDev)
	memberEntry, err2 := logEntry(memberDev, memberDev, 1, "add", memberDev)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}
	ms := w.now.UnixMilli()
	rows := []home.Row{
		home.RowOf(proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: w.group}, Name: "Project", Kind: "project",
			Home: nodeDev.ID(), Secretary: w.members["secretary"], Forge: "none", Checks: []string{},
			Pass: 90, PassFoundation: 70, Limits: proto.Limits{Rounds: 10, WakesPerHour: 20}, Created: ms,
		}),
		w.humanRow("owner", ownerDev, "owner"),
		w.humanRow("admin", adminDev, "admin"),
		w.humanRow("member", memberDev, "member"),
		w.deviceRow(adminDev, adminEntry),
		w.deviceRow(memberDev, memberEntry),
	}
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		rows = append(rows, w.aiRow(name, ownerDev, laptopDev))
	}
	write := func(tx *store.Tx) error { return w.putInitialRows(tx, rows, ms) }
	return w.db.Write(write)
}

func (w *homePauseWorld) humanRow(name string, dev identity.Key, role string) home.Row {
	return home.RowOf(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members[name]}, Group: w.group, Kind: "human", User: dev.ID(), Name: name},
		Role:      role,
	})
}

func (w *homePauseWorld) deviceRow(dev identity.Key, env proto.Envelope) home.Row {
	return home.RowOf(proto.DeviceRow{
		RowHeader:    proto.RowHeader{Type: "device", ID: dev.ID() + ":1"},
		DeviceChange: proto.DeviceChange{User: dev.ID(), Version: 1, Op: "add", Device: dev.ID(), Kind: "browser", Name: "browser"},
		Env:          env,
	})
}

func (w *homePauseWorld) aiRow(name string, ownerDev, laptopDev identity.Key) home.Row {
	return home.RowOf(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members[name]}, Group: w.group, Kind: "ai", User: ownerDev.ID(), Name: name},
		CLI:       "claude", Host: laptopDev.ID(), Secretary: name == "secretary", Account: "personal",
		Allow: []string{}, Approvers: []string{ownerDev.ID()}, State: "waiting", Path: "direct",
	})
}

func (w *homePauseWorld) putInitialRows(tx *store.Tx, rows []home.Row, ms int64) error {
	sealed := home.NewState(w.group, "", "").Seal(home.Result{Rows: rows})
	for _, r := range sealed.Rows {
		if _, err := tx.NextSeq(w.group); err != nil {
			return err
		}
		if err := r.Put(tx, w.group, ms); err != nil {
			return err
		}
	}
	return nil
}

func (w *homePauseWorld) bootLoop(cleanup func(func()), ownerDev, watchDev, nodeDev, laptopDev, adminDev, memberDev identity.Key) error {
	logs := func(user string) *identity.Log {
		if user != ownerDev.ID() {
			return nil
		}
		log, err := homeLog([]identity.Key{ownerDev, watchDev, nodeDev, laptopDev})
		if err != nil {
			return nil
		}
		return log
	}
	tables := home.NodeTables()
	tables.Events = home.Rules[home.Handler]{"seed": w.seedEvent}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: nodeDev.ID(), Owner: ownerDev.ID(), Logs: logs, Tables: tables,
		Now: func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
	w.peers = map[string]*homePausePeer{
		"the owner":  w.attach(ownerDev),
		"the admin":  w.attach(adminDev),
		"the member": w.attach(memberDev),
		"laptop":     w.attach(laptopDev),
	}
	w.watcher = w.attach(watchDev)
	loop.Inbox(w.peers["laptop"].conn)
	return w.syncWatcher()
}

func (w *homePauseWorld) attach(key identity.Key) *homePausePeer {
	c := newHomeFrameClient(64)
	return &homePausePeer{key: key, conn: w.loop.Attach(key.ID(), c), client: c}
}

func (w *homePauseWorld) syncWatcher() error {
	w.watcher.re++
	w.loop.Sync(w.watcher.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: w.watcher.re}, Group: w.group})
	_, err1 := w.watcher.next() // reset
	_, err2 := w.readRows(10)   // 1 group + 3 humans + 2 devices + 4 AIs
	_, err3 := w.watcher.next() // synced
	return errors.Join(err1, err2, err3)
}

func (w *homePauseWorld) seedEvent(_ *home.State, _ home.Disk, _ int64, ev any) home.Result {
	s := ev.(homePauseSeed)
	var items []home.Item
	if s.review != nil {
		items = []home.Item{*s.review}
	}
	return home.Result{Rows: []home.Row{home.RowOf(s.task)}, Items: items}
}

func (w *homePauseWorld) nextID() proto.ID {
	w.nonce++
	return exampleID("pause nonce " + strconv.Itoa(w.nonce))
}

func (w *homePauseWorld) putTask(name, state, step, ai, reviewer, commit string, wait *proto.TaskWait, queueReview bool) error {
	id := w.orderedTaskID(name)
	w.tasks[name] = id
	revID := w.members[reviewer]
	task := proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id}, Group: w.group, Num: len(w.tasks),
		Title: name, Slug: proto.MakeSlug(name), Origin: "brief", State: state, Step: step,
		Assignee: w.members[ai], Reviewer: revID, Commit: commit, Wait: wait,
		Pass: 90, Depends: []string{}, Created: w.now.UnixMilli(),
	}
	seed := homePauseSeed{task: task}
	if queueReview {
		seed.review = &home.Item{AI: revID, Why: "review", Task: id, Commit: commit}
	}
	w.loop.Event("seed", seed)
	if _, err := w.readRows(1); err != nil {
		return err
	}
	if queueReview {
		_, err := w.peers["laptop"].next()
		return err
	}
	return nil
}

func (w *homePauseWorld) orderedTaskID(name string) string {
	// Prefix with padded name characters in ASCII base64url so sorted ids follow name order.
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		return '-'
	}, name)
	for len(clean) < 21 {
		clean += "a"
	}
	return clean[:21] + "A"
}

func (w *homePauseWorld) sendGroup(who, op string) error {
	peerName, p, more := w.signer(who)
	payload := fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":%q%s}`, w.nextID(), w.now.UnixMilli()-500, w.group, op, more)
	return w.apply(peerName, p.key.Sign(payload))
}

func (w *homePauseWorld) signer(who string) (string, *homePausePeer, string) {
	if p, ok := w.peers[who]; ok {
		return who, p, ""
	}
	ai := strings.TrimPrefix(who, "the ")
	return "laptop", w.peers["laptop"], fmt.Sprintf(`,"as":%q`, w.members[ai])
}

func (w *homePauseWorld) postMention(who, ai string) error {
	p := w.peers[who]
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli() - 400, Group: proto.ID(w.group)},
		Kind:        "text", Text: "Hold until resume.", Mentions: []proto.ID{proto.ID(w.members[ai])}, Body: proto.EmptyBody{},
	}
	raw, err := marshalNoEscape(post)
	if err != nil {
		return err
	}
	return w.apply(who, p.key.Sign(string(raw)))
}

func (w *homePauseWorld) apply(who string, env proto.Envelope) error {
	seq0, q0, err := w.counts()
	if err != nil {
		return err
	}
	p := w.peers[who]
	p.re++
	w.loop.Cmd(p.conn, p.re, env)
	resp, err := p.next()
	if err != nil {
		return err
	}
	seq1, q1, err := w.counts()
	if err != nil {
		return err
	}
	w.lastResp, w.lastRows, w.lastItems = resp, nil, nil
	w.rowDelta, w.itemDelta = int(seq1-seq0), int(q1-q0)
	return w.collectPushes()
}

func (w *homePauseWorld) collectPushes() error {
	var err error
	if w.rowDelta > 0 {
		if w.lastRows, err = w.readRows(w.rowDelta); err != nil {
			return err
		}
	}
	if w.itemDelta > 0 {
		w.lastItems, err = w.readInbox(w.itemDelta)
	}
	return err
}

func (w *homePauseWorld) counts() (seq, queued int64, err error) {
	read := func(tx *store.Tx) error {
		if err := tx.QueryRow(`SELECT seq FROM grp WHERE g = ?`, w.group).Scan(&seq); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT COUNT(*) FROM inbox WHERE g = ?`, w.group).Scan(&queued)
	}
	err = w.db.Write(read)
	return seq, queued, err
}

func (w *homePauseWorld) readRows(want int) ([]json.RawMessage, error) {
	raw, err := w.watcher.next()
	if err != nil {
		return nil, err
	}
	var push proto.Rows
	if err := json.Unmarshal(raw, &push); err != nil || push.Type != "rows" || len(push.Rows) != want {
		return nil, fmt.Errorf("got %s (%v); want %d rows", raw, err, want)
	}
	return push.Rows, nil
}

func (w *homePauseWorld) readInbox(want int) ([]proto.InboxItem, error) {
	raw, err := w.peers["laptop"].next()
	if err != nil {
		return nil, err
	}
	var push proto.Inbox
	if err := json.Unmarshal(raw, &push); err != nil {
		return nil, err
	}
	if push.Group != proto.ID(w.group) || len(push.Items) != want {
		return nil, fmt.Errorf("got inbox %s; want %d items", raw, want)
	}
	return push.Items, homePauseVerifyCopies(push.Items)
}

func homePauseVerifyCopies(items []proto.InboxItem) error {
	for _, entry := range items {
		if !homePauseCopiesMatch(entry) {
			return fmt.Errorf("missing row copy on inbox item %+v", entry)
		}
	}
	return nil
}

func homePauseCopiesMatch(entry proto.InboxItem) bool {
	if entry.Item.Task != "" && (entry.Task == nil || entry.Task.ID != string(entry.Item.Task)) {
		return false
	}
	return entry.Item.Msg == "" || len(entry.Msg) > 0
}

func (w *homePauseWorld) answered(want string) error {
	if want == "ok" {
		return w.answeredOK()
	}
	var res proto.ErrorResponse
	if err := proto.Decode(w.lastResp, &res); err != nil {
		return err
	}
	if res.Err.Code != want || res.Err.Message == "" {
		return fmt.Errorf("answered %s; want error %s", w.lastResp, want)
	}
	return nil
}

func (w *homePauseWorld) answeredOK() error {
	var res proto.CmdResult
	if err := proto.Decode(w.lastResp, &res); err != nil || !res.OK {
		return fmt.Errorf("answered %s (%v); want ok", w.lastResp, err)
	}
	return nil
}

func (w *homePauseWorld) answeredIdle(want string) error {
	if err := w.answered(want); err != nil {
		return err
	}
	if w.rowDelta != 0 || w.itemDelta != 0 || len(w.watcher.client.ch) != 0 || len(w.peers["laptop"].client.ch) != 0 {
		return fmt.Errorf("wrote %d rows and %d items", w.rowDelta, w.itemDelta)
	}
	return nil
}

func (w *homePauseWorld) pausedBy(who, code string) error {
	by := w.members[who]
	return w.checkGroupAndSystem(&proto.GroupPause{By: by, At: w.now.UnixMilli()}, code, by)
}

func (w *homePauseWorld) unpausedBy(code, who string) error {
	return w.checkGroupAndSystem(nil, code, w.members[who])
}

func (w *homePauseWorld) checkGroupAndSystem(wantPause *proto.GroupPause, code, by string) error {
	if len(w.lastRows) != 2 {
		return fmt.Errorf("expected 2 pushed rows, got %d", len(w.lastRows))
	}
	var grp proto.GroupRow
	var msg proto.MsgRow
	if err := errors.Join(json.Unmarshal(w.lastRows[0], &grp), json.Unmarshal(w.lastRows[1], &msg)); err != nil {
		return err
	}
	if !reflect.DeepEqual(grp.Paused, wantPause) {
		return fmt.Errorf("group.paused is %+v; want %+v", grp.Paused, wantPause)
	}
	wantMsg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msg.ID, Seq: msg.Seq}, Pos: msg.Seq, Group: w.group,
		By: "system", Kind: "system", Timestamp: w.now.UnixMilli(), Mentions: []string{},
		Body: json.RawMessage(fmt.Sprintf(`{"code":%q,"by":%q}`, code, by)),
	}
	if !reflect.DeepEqual(msg, wantMsg) {
		return fmt.Errorf("got message %+v; want %+v", msg, wantMsg)
	}
	return nil
}

func (w *homePauseWorld) checkQueued(spec string) error {
	var got []string
	for _, entry := range w.lastItems {
		got = append(got, w.formatItem(entry.Item))
	}
	want := strings.Split(spec, ", ")
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %q; want %q", got, want)
	}
	w.lastItems = nil
	return nil
}

func (w *homePauseWorld) formatItem(it proto.Item) string {
	parts := []string{it.Why, w.memberName(string(it.AI))}
	if it.Task != "" {
		parts = append(parts, "task", w.taskName(string(it.Task)))
	}
	if it.Commit != "" {
		parts = append(parts, it.Commit)
	}
	return strings.Join(parts, " ")
}

func (w *homePauseWorld) memberName(id string) string {
	for name, mid := range w.members {
		if mid == id {
			return name
		}
	}
	return id
}

func (w *homePauseWorld) taskName(id string) string {
	for name, tid := range w.tasks {
		if tid == id {
			return name
		}
	}
	return id
}
