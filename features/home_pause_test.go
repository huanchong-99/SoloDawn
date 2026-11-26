//go:build acceptance

package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
	w := newWait(2*time.Second, "a frame", p.client.saw)
	defer w.stop()
	return recvWait(w, p.client.ch)
}

type homePauseWorld struct {
	now     time.Time
	db      *store.Store
	loop    *home.Loop
	group   string
	members map[string]string
	tasks   map[string]string
	peers   map[string]*homePausePeer
	watcher *homePausePeer
	host    *homePausePeer
	nonce   int
}

func homePauseSteps(sc *godog.ScenarioContext) {
	w := &homePauseWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a group for pause and resume with an owner, an admin and three working AIs$`, func() error {
		return w.start(cleanup)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)"$`, func(name, step, ai string) error {
		return w.putTask(name, step, ai, nil)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)" waiting for an approval$`, func(name, step, ai string) error {
		return w.putTask(name, step, ai, &proto.TaskWait{Kind: "approval", Since: w.now.UnixMilli() - 3000})
	})
	sc.Step(`^"([^"]+)" sends group "(pause|resume)"$`, w.sendGroup)
	sc.Step(`^"([^"]+)" posts a text mentioning "([^"]+)" while the group is paused$`, w.postMention)
	sc.Step(`^the group is paused by "([^"]+)" with system message "([^"]+)"$`, w.pausedBy)
	sc.Step(`^the group is unpaused with system message "([^"]+)" by "([^"]+)"$`, w.unpausedBy)
	sc.Step(`^the pause home has queued "([^"]*)"$`, w.checkQueued)
}

func (w *homePauseWorld) start(cleanup func(func())) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db
	w.members, w.tasks, w.nonce = map[string]string{}, map[string]string{}, 0
	for _, name := range []string{"owner", "admin", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("pause " + name))
	}
	w.group = string(exampleID("pause group"))

	ownerDev, watchDev := exampleKey("pause owner"), exampleKey("pause watcher")
	nodeDev, laptopDev, adminDev := exampleKey("pause node"), exampleKey("pause laptop"), exampleKey("pause admin")
	if err := w.seedStore(ownerDev, nodeDev, laptopDev, adminDev); err != nil {
		return err
	}
	return w.bootLoop(cleanup, ownerDev, watchDev, nodeDev, laptopDev, adminDev)
}

func (w *homePauseWorld) seedStore(ownerDev, nodeDev, laptopDev, adminDev identity.Key) error {
	adminEntry, err := logEntry(adminDev, adminDev, 1, "add", adminDev)
	if err != nil {
		return err
	}
	ms := w.now.UnixMilli()
	rows := []home.Row{
		home.RowOf(proto.GroupRow{
			RowHeader: proto.RowHeader{Type: "group", ID: w.group}, Name: "Project",
			Home: nodeDev.ID(), Secretary: w.members["secretary"],
			Pass: 90, PassFoundation: 70, Limits: proto.Limits{Rounds: 10, WakesPerHour: 20}, Created: ms,
		}),
		w.humanRow("owner", ownerDev, "owner"),
		w.humanRow("admin", adminDev, "admin"),
		home.RowOf(proto.DeviceRow{
			RowHeader:    proto.RowHeader{Type: "device", ID: adminDev.ID() + ":1"},
			DeviceChange: proto.DeviceChange{User: adminDev.ID(), Version: 1, Op: "add", Device: adminDev.ID(), Kind: "browser", Name: "browser"},
			Env:          adminEntry,
		}),
	}
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		rows = append(rows, home.RowOf(proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members[name]}, Group: w.group, Kind: "ai", User: ownerDev.ID(), Name: name},
			CLI:       "claude", Host: laptopDev.ID(), Workspace: "/work/" + name, Secretary: name == "secretary", Account: "personal",
			Allow: []string{}, Approvers: []string{ownerDev.ID()}, State: "waiting", Path: "direct",
		}))
	}
	sealed := home.NewState(w.group, "", "").Seal(home.Result{Rows: rows})
	return w.db.Write(func(tx *store.Tx) error {
		for _, r := range sealed.Rows {
			if _, err := tx.NextSeq(w.group); err != nil {
				return err
			}
			if err := r.Put(tx, w.group, ms); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w *homePauseWorld) humanRow(name string, dev identity.Key, role string) home.Row {
	return home.RowOf(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members[name]}, Group: w.group, Kind: "human", User: dev.ID(), Name: name},
		Role:      role,
	})
}

func (w *homePauseWorld) bootLoop(cleanup func(func()), ownerDev, watchDev, nodeDev, laptopDev, adminDev identity.Key) error {
	ownerLog, err := homeLog([]identity.Key{ownerDev, watchDev, nodeDev, laptopDev})
	if err != nil {
		return err
	}
	logs := func(user string) *identity.Log {
		if user != ownerDev.ID() {
			return nil
		}
		return ownerLog
	}
	tables := home.NodeTables()
	tables.Events = home.Rules[home.Handler]{"seed": func(_ *home.State, _ home.Disk, _ int64, ev any) home.Result {
		return home.Result{Rows: []home.Row{home.RowOf(ev.(proto.TaskRow))}}
	}}
	loop, err := home.StartLoop(home.LoopConfig{
		Store: w.db, Home: nodeDev.ID(), Owner: ownerDev.ID(), Logs: logs, Tables: tables,
		Now: func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
	w.peers = map[string]*homePausePeer{"the owner": w.attach(ownerDev), "the admin": w.attach(adminDev)}
	w.watcher, w.host = w.attach(watchDev), w.attach(laptopDev)
	loop.Inbox(w.host.conn)
	w.watcher.re++
	loop.Sync(w.watcher.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: w.watcher.re}, Group: w.group})
	_, err1 := w.watcher.next() // reset
	_, err2 := w.readRows(8)    // 1 group + 2 humans + 1 device + 4 AIs
	_, err3 := w.watcher.next() // synced
	return errors.Join(err1, err2, err3)
}

func (w *homePauseWorld) attach(key identity.Key) *homePausePeer {
	c := newHomeFrameClient(64)
	return &homePausePeer{key: key, conn: w.loop.Attach(key.ID(), c), client: c}
}

func (w *homePauseWorld) nextID() proto.ID {
	w.nonce++
	return exampleID(fmt.Sprintf("pause nonce %d", w.nonce))
}

func (w *homePauseWorld) putTask(name, step, ai string, wait *proto.TaskWait) error {
	id := string(exampleID("pause task " + name))
	w.tasks[name] = id
	w.loop.Event("seed", proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id}, Group: w.group, Num: len(w.tasks),
		Title: name, Origin: "brief", State: "active", Step: step,
		Assignee: w.members[ai], Wait: wait, Pass: 90, Depends: []string{}, Created: w.now.UnixMilli(),
	})
	_, err := w.readRows(1)
	return err
}

func (w *homePauseWorld) sendGroup(who, op string) error {
	payload := fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":%q}`, w.nextID(), w.now.UnixMilli()-500, w.group, op)
	return w.apply(who, w.peers[who].key.Sign(payload))
}

func (w *homePauseWorld) postMention(who, ai string) error {
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli() - 400, Group: proto.ID(w.group)},
		Kind:        "text", Text: "Hold until resume.", Mentions: []proto.ID{proto.ID(w.members[ai])}, Body: proto.EmptyBody{},
	}
	raw, err := marshalNoEscape(post)
	if err != nil {
		return err
	}
	if err := w.apply(who, w.peers[who].key.Sign(string(raw))); err != nil {
		return err
	}
	_, err = w.readRows(1)
	return err
}

func (w *homePauseWorld) apply(who string, env proto.Envelope) error {
	p := w.peers[who]
	p.re++
	w.loop.Cmd(p.conn, p.re, env)
	resp, err := p.next()
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(resp, &res); err != nil || !res.OK {
		return fmt.Errorf("answered %s (%v); want ok", resp, err)
	}
	return nil
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

func (w *homePauseWorld) pausedBy(who, code string) error {
	by := w.members[who]
	return w.checkGroupAndSystem(&proto.GroupPause{By: by, At: w.now.UnixMilli()}, code, by)
}

func (w *homePauseWorld) unpausedBy(code, who string) error {
	return w.checkGroupAndSystem(nil, code, w.members[who])
}

func (w *homePauseWorld) checkGroupAndSystem(wantPause *proto.GroupPause, code, by string) error {
	rows, err := w.readRows(2)
	if err != nil {
		return err
	}
	var grp proto.GroupRow
	var msg proto.MsgRow
	if err := errors.Join(json.Unmarshal(rows[0], &grp), json.Unmarshal(rows[1], &msg)); err != nil {
		return err
	}
	if !reflect.DeepEqual(grp.Paused, wantPause) {
		return fmt.Errorf("group.paused is %+v; want %+v", grp.Paused, wantPause)
	}
	got := []any{msg.Thread, msg.By, msg.Kind, msg.Timestamp, string(msg.Body)}
	want := []any{(*string)(nil), "system", "system", w.now.UnixMilli(), fmt.Sprintf(`{"code":%q,"by":%q}`, code, by)}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("got message %v; want %v", got, want)
	}
	return nil
}

func (w *homePauseWorld) checkQueued(spec string) error {
	raw, err := w.host.next()
	if err != nil {
		return err
	}
	var push proto.Inbox
	if err := json.Unmarshal(raw, &push); err != nil {
		return err
	}
	var got []string
	for _, entry := range push.Items {
		got = append(got, w.formatItem(entry.Item))
	}
	if want := strings.Split(spec, ", "); !slices.Equal(got, want) {
		return fmt.Errorf("queued %q; want %q", got, want)
	}
	return nil
}

func (w *homePauseWorld) formatItem(it proto.Item) string {
	parts := []string{it.Why, homePauseName(w.members, string(it.AI))}
	if it.Task != "" {
		parts = append(parts, "task", homePauseName(w.tasks, string(it.Task)))
	}
	return strings.Join(parts, " ")
}

func homePauseName(names map[string]string, id string) string {
	for name, v := range names {
		if v == id {
			return name
		}
	}
	return id
}
