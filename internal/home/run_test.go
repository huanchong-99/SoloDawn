package home_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// runWorld is a loop over a store, with a clock the test moves and the
// failures the loop reported.
type runWorld struct {
	t       *testing.T
	db      *store.Store
	loop    *home.Loop
	clock   atomic.Int64
	nonce   atomic.Int64
	reports chan error
	ghost   *home.Conn // a connection that is gone: Sync sends it nothing, after the group's jobs
}

func runOpen(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// runLogs is the node's device logs: the owner has the browser, the node and
// the laptop, the guest one device. Each call makes a log of its own.
func runLogs(t *testing.T) func(user string) *identity.Log {
	return func(user string) *identity.Log {
		switch user {
		case browser.ID():
			return logOf(t, browser, node, laptop)
		case guest.ID():
			return logOf(t, guest)
		}
		return nil
	}
}

func runStart(t *testing.T, db *store.Store, tables home.Tables, change ...func(*home.LoopConfig)) *runWorld {
	t.Helper()
	w := &runWorld{t: t, db: db, reports: make(chan error, 64)}
	w.clock.Store(clock)
	cfg := home.LoopConfig{Store: db, Home: node.ID(), Owner: browser.ID(), Logs: runLogs(t), Tables: tables,
		Now: func() time.Time { return time.UnixMilli(w.clock.Load()) }, Report: func(err error) { w.reports <- err }}
	for _, set := range change {
		set(&cfg)
	}
	loop, err := home.StartLoop(cfg)
	if err != nil {
		t.Fatal(err)
	}
	w.loop = loop
	t.Cleanup(loop.Close)
	w.ghost = loop.Attach("a ghost", runNewClient())
	loop.Detach(w.ghost)
	return w
}

// idle waits until the two groups of the tests have done every job they
// were given so far.
func (w *runWorld) idle() {
	for _, group := range []string{groupID, runOther} {
		w.loop.Sync(w.ghost, proto.Sync{Group: group})
	}
}

// runID is a random id that follows from a number.
func runID(n int64) string {
	b := make([]byte, 16)
	b[0] = 0xFF // after the ids of id() in the order of the texts
	binary.BigEndian.PutUint64(b[8:], uint64(n))
	return base64.RawURLEncoding.EncodeToString(b)
}

// The second group of the tests.
var runOther = runID(1 << 40)

// cmd signs a command for a group with a nonce of its own, at the home's time.
func (w *runWorld) cmd(by identity.Key, group, t, more string) proto.Envelope {
	return by.Sign(fmt.Sprintf(`{"t":%q,"n":%q,"ts":%d,"g":%q%s}`, t, runID(w.nonce.Add(1)), w.clock.Load(), group, more))
}

// reported returns the next failure the loop reported.
func (w *runWorld) reported() string {
	w.t.Helper()
	select {
	case err := <-w.reports:
		return err.Error()
	case <-time.After(10 * time.Second):
		w.t.Fatal(timedOut(10*time.Second, "the loop to report a failure", nil))
		return ""
	}
}

// query reads one value from the store.
func runQuery[T any](t *testing.T, db *store.Store, query string, args ...any) (v T) {
	t.Helper()
	err := db.Write(func(tx *store.Tx) error { return tx.QueryRow(query, args...).Scan(&v) })
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func runExec(t *testing.T, db *store.Store, query string, args ...any) {
	t.Helper()
	err := db.Write(func(tx *store.Tx) error {
		_, err := tx.Exec(query, args...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// runClient is a client of the tests. It keeps every frame it is sent.
type runClient struct {
	got     chan any
	done    chan struct{}
	started chan struct{} // takes a value when a Send begins
	hold    chan struct{} // a Send waits until it is closed; nil lets every Send through
	refuse  atomic.Bool   // a Send fails
	during  func(frame any)
}

func runNewClient() *runClient {
	return &runClient{got: make(chan any, 8192), done: make(chan struct{}), started: make(chan struct{}, 8192)}
}

func (c *runClient) Send(frame any) error {
	c.started <- struct{}{}
	if c.hold != nil {
		<-c.hold
	}
	if c.refuse.Load() {
		return errors.New("the channel is closed")
	}
	if c.during != nil {
		c.during(frame)
	}
	c.got <- frame
	return nil
}

func (c *runClient) Done() <-chan struct{} { return c.done }

// runClosing is a client that can be closed, as a peer.Connection can.
type runClosing struct {
	*runClient
	closed chan struct{}
	once   sync.Once
}

func (c *runClosing) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

// runPeer is a device with a connection to the loop.
type runPeer struct {
	w      *runWorld
	key    identity.Key
	conn   *home.Conn
	client *runClient
	re     int64
}

// attach connects a device and takes its welcome.
func (w *runWorld) attach(key identity.Key) *runPeer {
	client := runNewClient()
	p := w.attachClient(key, client, client)
	w.loop.Welcome(p.conn, runWelcome)
	p.next()
	return p
}

// runWelcome is the welcome of the tests: its groups and nothing else.
func runWelcome(groups []proto.GroupSummary) any {
	return proto.Groups{Type: "welcome", Groups: groups}
}

func runWelcomeFrame(entries ...proto.GroupSummary) string {
	return runFrame(runWelcome(append([]proto.GroupSummary{}, entries...)))
}

func (w *runWorld) attachClient(key identity.Key, client home.Client, inner *runClient) *runPeer {
	return &runPeer{w: w, key: key, conn: w.loop.Attach(key.ID(), client), client: inner}
}

// runFrame is a frame as its JSON, the way a peer connection encodes it.
func runFrame(frame any) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(frame); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// next returns the next frame the peer was sent.
func (p *runPeer) next() string {
	p.w.t.Helper()
	select {
	case frame := <-p.client.got:
		return runFrame(frame)
	case <-time.After(10 * time.Second):
		p.w.t.Fatal(timedOut(10*time.Second, "a frame", nil))
		return ""
	}
}

// gets checks the next frames of the peer, in order.
func (p *runPeer) gets(want ...string) {
	p.w.t.Helper()
	for _, frame := range want {
		equal(p.w.t, p.next(), frame)
	}
}

// send hands in a command on the peer's connection and returns its request id.
func (p *runPeer) send(group, t, more string) int64 {
	p.re++
	p.w.loop.Cmd(p.conn, p.re, p.w.cmd(p.key, group, t, more))
	return p.re
}

// does sends a command and checks that the next frame is its success.
func (p *runPeer) does(group, t, more string) {
	p.w.t.Helper()
	re := p.send(group, t, more)
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, re))
}

// fails sends a command and checks that the next frame is that error.
func (p *runPeer) fails(group, t, more, code string) {
	p.w.t.Helper()
	re := p.send(group, t, more)
	got := p.next()
	if !strings.HasPrefix(got, fmt.Sprintf(`{"re":%d,"err":{"code":%q,"msg":"`, re, code)) {
		p.w.t.Fatalf("answered %s; want the error %s", got, code)
	}
}

// quiet checks that the peer was sent nothing: once the groups have done
// their jobs, the answer to a command that names no group is the next frame
// it gets.
func (p *runPeer) quiet() {
	p.w.t.Helper()
	p.w.idle()
	p.re++
	p.w.loop.Cmd(p.conn, p.re, p.key.Sign(`{"t":"note","n":"`+runID(p.w.nonce.Add(1))+`","ts":`+fmt.Sprint(p.w.clock.Load())+`}`))
	got := p.next()
	if !strings.HasPrefix(got, fmt.Sprintf(`{"re":%d,"err":{"code":"`, p.re)) {
		p.w.t.Fatalf("got %s; want nothing before the answer to request %d", got, p.re)
	}
}

// runWait waits for a channel of the test to be closed or to give a value.
func runWait[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal(timedOut(10*time.Second, fmt.Sprintf("a %T from a channel", *new(T)), nil))
	}
}

// watch subscribes the peer to a group at its current seq through Sync.
func (p *runPeer) watch(group string, stateOnly bool) int64 {
	p.w.t.Helper()
	var since int64
	for _, g := range p.w.loop.Groups(p.conn) {
		if g.Group == group {
			since = g.Seq
		}
	}
	p.re++
	p.w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: p.re}, Group: group, Since: since, StateOnly: stateOnly})
	var synced proto.Synced
	equal(p.w.t, proto.Decode([]byte(p.next()), &synced), nil)
	equal(p.w.t, []any{synced.Re, synced.Group, synced.Seq}, []any{p.re, group, since})
	return synced.Seq
}

func (p *runPeer) syncRefused(group, code string) {
	p.w.t.Helper()
	p.re++
	p.w.loop.Sync(p.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: p.re}, Group: group})
	var resp proto.ErrorResponse
	equal(p.w.t, proto.Decode([]byte(p.next()), &resp), nil)
	equal(p.w.t, []any{resp.Re, resp.Err.Code}, []any{p.re, code})
}

func runRows(group string, rows ...any) string {
	frame := proto.Rows{Type: "rows", Group: group}
	for _, row := range rows {
		frame.Rows = append(frame.Rows, json.RawMessage(runFrame(row)))
	}
	return runFrame(frame)
}

func runGroupsFrame(entries ...proto.GroupSummary) string {
	return runFrame(proto.Groups{Type: "groups", Groups: append([]proto.GroupSummary{}, entries...)})
}

// The rows of the test commands.
func runGroupRow(group, name string, seq int64) proto.GroupRow {
	return proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: group, Seq: seq}, Name: name, Kind: "project", Home: node.ID(), Secretary: aiID}
}

func runPerson(seq int64) proto.HumanMemberRow {
	return proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: ownerID, Seq: seq}, Kind: "human", User: browser.ID()}, Role: "owner"}
}

func runAI(seq int64) proto.AIMemberRow {
	return proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: aiID, Seq: seq}, Kind: "ai", User: browser.ID()}, Host: laptop.ID()}
}

func runNote(id string, seq int64, text string) proto.NoteRow {
	return proto.NoteRow{RowHeader: proto.RowHeader{Type: "note", ID: id, Seq: seq}, Pos: seq, Text: text, By: ownerID, Timestamp: clock}
}

func runMsg(id string, seq int64, text string) proto.MsgRow {
	return proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: id, Seq: seq}, Pos: seq, By: ownerID, Kind: "text", Timestamp: clock,
		Text: text, Mentions: []string{}, Body: json.RawMessage(`{}`)}
}

// runCreate is the group create of the tests: the group, its owner and one AI
// on the laptop, and the groups push.
func runCreate(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Name string `json:"name"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	if s.Group.ID != "" {
		return home.Result{}, &home.Refusal{Code: "conflict", Msg: "the group exists"}
	}
	return home.Result{GroupsChanged: true, Rows: []home.Row{
		home.RowOf(runGroupRow(s.ID, p.Name, 0)), home.RowOf(runPerson(0)), home.RowOf(runAI(0))}}, nil
}

// runWriteNote writes a note with the text of the payload and its nonce as
// id; it refuses a note without text.
func runWriteNote(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Text string `json:"text"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	if p.Text == "" {
		return home.Result{}, &home.Refusal{Code: "conflict", Msg: "a note has a text"}
	}
	return home.Result{Rows: []home.Row{home.RowOf(runNote(string(c.Nonce), 0, p.Text))}}, nil
}

// runPost writes a main-line text message.
func runPost(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p struct {
		Text string `json:"text"`
	}
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	return home.Result{Rows: []home.Row{home.RowOf(runMsg(string(c.Nonce), 0, p.Text))}}, nil
}

// runTables are the commands most tests send; a test adds its own.
func runTables() home.Tables {
	return home.Tables{
		Commands: home.Table{"group/create": runCreate, "note": runWriteNote, "post/text": runPost},
		Timers:   home.Rules[home.Timer]{}, Events: home.Rules[home.Handler]{}, After: home.Rules[home.Hook]{},
	}
}

const runCreateWebsite = `,"op":"create","name":"Website"`

// founded is a world with the group of the tests, created by the owner's
// browser, whose connection it returns after the frames of the creation.
func runFounded(t *testing.T, tables home.Tables, change ...func(*home.LoopConfig)) (*runWorld, *runPeer) {
	t.Helper()
	w := runStart(t, runOpen(t), tables, change...)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.gets(runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 3}))
	return w, p
}

// runRecord keeps what rules and hooks saw, from whichever goroutine.
type runRecord struct {
	mu    sync.Mutex
	lines []string
}

func (r *runRecord) add(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *runRecord) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.lines...)
}

func TestRunPushesChangedRowsBeforeCommandReply(t *testing.T) {
	w, p := runFounded(t, runTables())
	p.watch(groupID, false)

	re := p.send(groupID, "note", `,"text":"before the reply"`)
	p.gets(runRows(groupID, runNote(runID(w.nonce.Load()), 4, "before the reply")),
		fmt.Sprintf(`{"re":%d,"ok":true}`, re))
}

// A2.1, the home half: a group created on one connection shows up on a
// second connection of the same person, and a message posted on one reaches
// the other. The sender gets the rows before its answer.
func TestRunTellsEveryClientOfAGroup(t *testing.T) {
	w := runStart(t, runOpen(t), runTables())
	first, second := w.attach(browser), w.attach(laptop)
	equal(t, w.loop.Groups(second.conn), []proto.GroupSummary{})
	second.syncRefused(groupID, "not_found")

	first.does(groupID, "group", runCreateWebsite)
	listed := runGroupsFrame(proto.GroupSummary{Group: groupID, Name: "Website", Seq: 3})
	first.gets(listed)
	second.gets(listed)
	equal(t, w.loop.Groups(second.conn), []proto.GroupSummary{{Group: groupID, Name: "Website", Seq: 3}})

	equal(t, second.watch(groupID, false), int64(3))
	equal(t, first.watch(groupID, false), int64(3))

	re := first.send(groupID, "post", `,"kind":"text","text":"hello"`)
	posted := runRows(groupID, runMsg(runID(w.nonce.Load()), 4, "hello"))
	first.gets(posted, fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	second.gets(posted)
	re = second.send(groupID, "note", `,"text":"seen"`)
	noted := runRows(groupID, runNote(runID(w.nonce.Load()), 5, "seen"))
	second.gets(noted, fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	first.gets(noted)

	// The rows went through to the store as they were pushed.
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row WHERE g=?", groupID), 5)
	equal(t, runQuery[string](t, w.db, "SELECT data FROM row WHERE g=? AND seq=4", groupID), runFrame(runMsg(runID(w.nonce.Load()-1), 4, "hello")))
	equal(t, runQuery[int64](t, w.db, "SELECT mt FROM row WHERE g=? AND seq=4", groupID), int64(clock))
	equal(t, runQuery[int64](t, w.db, "SELECT seq FROM grp WHERE g=?", groupID), int64(5))
}

// A welcome holds the groups as they are when it is queued, and the pushes
// of the list start with it: a group created before the welcome is in it and
// is not pushed before it, and one created after it follows it.
func TestRunWelcomesAConnectionBeforeItsGroupsPushes(t *testing.T) {
	w := runStart(t, runOpen(t), runTables())
	p := w.attach(browser)
	client := runNewClient()
	late := w.attachClient(laptop, client, client)
	p.does(groupID, "group", runCreateWebsite)
	website := proto.GroupSummary{Group: groupID, Name: "Website", Seq: 3}
	p.gets(runGroupsFrame(website))
	w.loop.Welcome(late.conn, runWelcome)
	p.does(runOther, "group", `,"op":"create","name":"Other"`)
	both := runGroupsFrame(website, proto.GroupSummary{Group: runOther, Name: "Other", Seq: 3})
	p.gets(both)
	late.gets(runWelcomeFrame(website), both)
	// A second hello is answered with the list as it is then.
	w.loop.Welcome(late.conn, runWelcome)
	late.gets(runWelcomeFrame(website, proto.GroupSummary{Group: runOther, Name: "Other", Seq: 3}))
	// A device that may read no group here is welcomed with none.
	stranger := w.attachClient(guest, client, client)
	w.loop.Welcome(stranger.conn, runWelcome)
	stranger.gets(runWelcomeFrame())
}

// A device row that is written again has a later sequence number than the
// entry after it. A loop that starts holds the device rows in the order of
// their versions, so a device that was revoked stays revoked.
func TestRunLoadsTheDeviceRowsInTheOrderOfTheirVersions(t *testing.T) {
	tablet := device(8)
	tables := runTables()
	tables.Commands["member/device"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		var p struct {
			Version int    `json:"ver"`
			Change  string `json:"change"`
		}
		c.Decode(&p)
		return home.Result{Rows: []home.Row{home.RowOf(deviceRow(browser, browser, p.Version, p.Change, tablet))}}, nil
	}
	db := runOpen(t)
	w := runStart(t, db, tables)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()
	p.does(groupID, "member", `,"op":"device","ver":4,"change":"add"`)
	p.does(groupID, "member", `,"op":"device","ver":5,"change":"revoke"`)
	p.does(groupID, "member", `,"op":"device","ver":4,"change":"add"`)
	equal(t, runQuery[string](t, db, "SELECT group_concat(data->>'ver', ' ' ORDER BY seq) FROM row WHERE r='device'"), "5 4")
	w.attach(tablet).fails(groupID, "note", `,"text":"x"`, "unknown_device")
	w.loop.Close()

	w = runStart(t, db, tables)
	w.attach(tablet).fails(groupID, "note", `,"text":"x"`, "unknown_device")
	w.attach(laptop).does(groupID, "note", `,"text":"x"`)
}

// A loop that starts on a store reads its groups from it: the rows, the two
// counters and the device logs are what the last run left.
func TestRunLoadsItsGroupsAfterARestart(t *testing.T) {
	tablet := device(8)
	tables := runTables()
	var seen runRecord
	tables.Commands["task/start"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		seen.add("num %d, %d tasks, secretary %s, host %s", s.Num, len(s.Tasks), s.Group.Secretary, s.AIs[aiID].Host)
		task := proto.TaskRow{RowHeader: proto.RowHeader{Type: "task", ID: string(c.Nonce)}, Num: s.Num + 1, State: "merged"}
		return home.Result{Rows: []home.Row{home.RowOf(task)}, Answer: store.Outcome{Num: task.Num}}, nil
	}
	tables.Commands["member/device"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		return home.Result{Rows: []home.Row{home.RowOf(deviceRow(browser, browser, 4, "add", tablet))}}, nil
	}
	db := runOpen(t)
	w := runStart(t, db, tables)
	p := w.attach(browser)
	p.does(groupID, "group", runCreateWebsite)
	p.next()
	p.does(runOther, "group", `,"op":"create","name":"Other"`)
	p.next()
	p.does(groupID, "member", `,"op":"device"`)
	re := p.send(groupID, "task", `,"op":"start"`)
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true,"num":1}`, re))
	re = p.send(groupID, "task", `,"op":"start"`)
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true,"num":2}`, re))
	p.does(groupID, "note", `,"text":"kept on disk only"`)
	// Retention removes a finished task; its number is not given out again.
	runExec(t, db, "DELETE FROM row WHERE g=? AND r='task' AND seq=6", groupID)
	w.loop.Close()

	w = runStart(t, db, tables)
	p = w.attach(tablet)
	// The tablet is known only through the device row of the first group.
	equal(t, w.loop.Groups(p.conn), []proto.GroupSummary{{Group: groupID, Name: "Website", Seq: 7}})
	equal(t, w.loop.Groups(w.attach(browser).conn), []proto.GroupSummary{{Group: groupID, Name: "Website", Seq: 7}, {Group: runOther, Name: "Other", Seq: 3}})
	equal(t, p.watch(groupID, false), int64(7))
	re = p.send(groupID, "task", `,"op":"start"`)
	p.next()
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true,"num":3}`, re))
	equal(t, seen.all()[2], "num 2, 1 tasks, secretary "+aiID+", host "+laptop.ID())
	equal(t, runQuery[int64](t, db, "SELECT num FROM grp WHERE g=?", groupID), int64(3))
	// The next row follows the last one of the run before.
	equal(t, runQuery[int64](t, db, "SELECT seq FROM row WHERE g=? AND r='task' AND id=?", groupID, runID(w.nonce.Load())), int64(8))
	p.fails(runOther, "note", `,"text":"x"`, "unknown_device")
}

// A store that cannot be read, or that holds a state row no rule could have
// written, starts no loop.
func TestRunDoesNotStartOnAStoreItCannotRead(t *testing.T) {
	start := func(db *store.Store) error {
		loop, err := home.StartLoop(home.LoopConfig{Store: db, Home: node.ID(), Owner: browser.ID(), Logs: runLogs(t), Tables: runTables()})
		if err == nil {
			loop.Close()
		}
		return err
	}
	founded := func() *store.Store {
		w, _ := runFounded(t, runTables())
		w.loop.Close()
		return w.db
	}
	for name, spoil := range map[string]string{
		"no row table":       "DROP TABLE row",
		"no counters":        "DROP TABLE grp",
		"a member of a kind": `UPDATE row SET data=json_set(data, '$.kind', 'robot') WHERE id='` + ownerID + `'`,
		"a row that is none": `UPDATE row SET data='[]' WHERE r='task' OR id='` + aiID + `'`,
		"a field of a type":  `UPDATE row SET data=json_set(data, '$.name', 7) WHERE r='group'`,
	} {
		t.Run(name, func(t *testing.T) {
			db := founded()
			equal(t, start(db), nil)
			runExec(t, db, spoil)
			if err := start(db); err == nil {
				t.Fatal("the loop started")
			}
		})
	}
}

// The commands of a group run strictly in the order they were handed in,
// whichever connection they came on.
func TestRunKeepsTheOrderOfAGroup(t *testing.T) {
	w, first := runFounded(t, runTables())
	second, watcher := w.attach(laptop), w.attach(browser)
	watcher.watch(groupID, false)
	var want []string
	for i := range 60 {
		p := []*runPeer{first, second}[i%2]
		p.send(groupID, "note", fmt.Sprintf(`,"text":"note %d"`, i))
		note := runNote(runID(w.nonce.Load()), int64(4+i), fmt.Sprintf("note %d", i))
		note.By = ownerID
		want = append(want, runRows(groupID, note))
	}
	watcher.gets(want...)
	for i := range 30 {
		first.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, i+2))
		second.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, i+1))
	}
}

// Each group has a goroutine of its own: one that is busy does not hold
// another.
func TestRunGroupsDoNotWaitForEachOther(t *testing.T) {
	tables := runTables()
	entered, release := make(chan struct{}), make(chan struct{})
	tables.After.Add("wait", func(s *home.State, res home.Result) {
		if _, noted := res.Rows[0].Value().(proto.NoteRow); noted && s.ID == groupID {
			entered <- struct{}{}
			<-release
		}
	})
	w, p := runFounded(t, tables)
	p.does(runOther, "group", `,"op":"create","name":"Other"`)
	p.next()
	slow := p.send(groupID, "note", `,"text":"held in a hook"`)
	runWait(t, entered)
	// The first group is inside its hook; the second answers meanwhile.
	for range 5 {
		p.does(runOther, "note", `,"text":"not held"`)
	}
	close(release)
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, slow))
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row WHERE r='note'"), 6)
}

// A refused command leaves its receipt and its answer, and nothing else: no
// row, no hook, no push, and the state as it was.
func TestRunARefusedCommandLeavesOnlyItsReceipt(t *testing.T) {
	tables := runTables()
	var hooks runRecord
	tables.After.Add("seen", func(s *home.State, res home.Result) { hooks.add("%d rows at %d", len(res.Rows), s.Seq) })
	w, p := runFounded(t, tables)
	p.watch(groupID, false)
	receipts := func() int { return runQuery[int](t, w.db, "SELECT count(*) FROM receipt") }

	refused := w.cmd(browser, groupID, "note", `,"text":""`)
	w.loop.Cmd(p.conn, 7, refused)
	p.gets(`{"re":7,"err":{"code":"conflict","msg":"a note has a text"}}`)
	equal(t, receipts(), 2)
	equal(t, runQuery[string](t, w.db, "SELECT answer FROM receipt WHERE n=?", runID(w.nonce.Load())), `{"code":"conflict"}`)
	// The same command again is answered from the receipt.
	w.loop.Cmd(p.conn, 8, refused)
	p.gets(`{"re":8,"err":{"code":"conflict","msg":"the command was refused when it first arrived"}}`)

	p.quiet()
	equal(t, hooks.all(), []string{"3 rows at 3"})
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row"), 3)
	// The next command gets the next sequence number.
	re := p.send(groupID, "note", `,"text":"x"`)
	p.gets(runRows(groupID, runNote(runID(w.nonce.Load()), 4, "x")), fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	// An accepted command sent again has no second effect and reaches no hook.
	again := w.cmd(browser, groupID, "note", `,"text":"once"`)
	w.loop.Cmd(p.conn, 20, again)
	w.loop.Cmd(p.conn, 21, again)
	p.gets(runRows(groupID, runNote(runID(w.nonce.Load()), 5, "once")), `{"re":20,"ok":true}`, `{"re":21,"ok":true}`)
	p.quiet()
	equal(t, hooks.all(), []string{"3 rows at 3", "1 rows at 4", "1 rows at 5"})
}

// A command that names no group is answered by the first check that fails,
// in the order of every command, without a group and without a receipt.
func TestRunAnswersACommandWithoutAGroup(t *testing.T) {
	w, p := runFounded(t, runTables())
	stranger := w.attach(nobody)
	head := func(more string) string {
		return fmt.Sprintf(`{"t":"note","n":%q,"ts":%d%s}`, runID(w.nonce.Add(1)), clock, more)
	}
	for _, c := range []struct {
		on   *runPeer
		env  proto.Envelope
		code string
	}{
		{p, browser.Sign(head(``)), "bad_request"},
		{p, proto.Envelope{Payload: head(``), Key: browser.ID(), Signature: "AA"}, "bad_sig"},
		{stranger, nobody.Sign(head(``)), "unknown_device"},
		{p, browser.Sign(head(`,"g":"no id"`)), "bad_request"},
		{p, browser.Sign(`not JSON`), "bad_request"},
		{p, browser.Sign(head(`,"g":7`)), "bad_request"},
	} {
		c.on.re++
		w.loop.Cmd(c.on.conn, c.on.re, c.env)
		got := c.on.next()
		if !strings.HasPrefix(got, fmt.Sprintf(`{"re":%d,"err":{"code":%q,"msg":"`, c.on.re, c.code)) {
			t.Fatalf("answered %s; want %s", got, c.code)
		}
	}
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM receipt"), 1)
}

// A node whose owner has no log knows no device.
func TestRunKnowsNoDeviceWithoutTheOwnersLog(t *testing.T) {
	w := runStart(t, runOpen(t), runTables(), func(cfg *home.LoopConfig) {
		cfg.Logs = func(string) *identity.Log { return nil }
	})
	p := w.attach(browser)
	p.fails(groupID, "group", runCreateWebsite, "unknown_device")
}

// A command for a group that does not exist is answered and leaves nothing
// behind but its receipt; two commands for one such group still run in order.
func TestRunAGroupThatDoesNotExistTakesCommandsInOrder(t *testing.T) {
	tables := runTables()
	entered, release := make(chan struct{}), make(chan struct{})
	create := tables.Commands["group/create"]
	tables.Commands["group/create"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		entered <- struct{}{}
		<-release
		return create(s, c)
	}
	w := runStart(t, runOpen(t), tables)
	p := w.attach(browser)
	p.fails(groupID, "note", `,"text":"too early"`, "not_found")
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM receipt"), 1)

	first := p.send(groupID, "group", runCreateWebsite)
	runWait(t, entered)
	// The second command waits behind the first, which is still running.
	second := p.send(groupID, "note", `,"text":"right after"`)
	close(release)
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, first))
	p.next()
	p.gets(fmt.Sprintf(`{"re":%d,"ok":true}`, second))
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row"), 4)
}

// A write that fails drops the whole transaction: the answer is internal,
// the failure is reported, and neither the store nor the state has changed.
func TestRunAFailedWriteChangesNothing(t *testing.T) {
	tables := runTables()
	var hooks runRecord
	tables.After.Add("seen", func(s *home.State, res home.Result) { hooks.add("hook") })
	note := func(c home.Command) home.Row { return home.RowOf(runNote(string(c.Nonce), 0, "lost")) }
	results := map[string]func(c home.Command) home.Result{
		"write": func(c home.Command) home.Result {
			return home.Result{Rows: []home.Row{note(c)}, Writes: []home.Write{setting{"k", "v"}, setting{"k", "v"}}}
		},
		"row": func(c home.Command) home.Result {
			broken := runMsg(string(c.Nonce), 0, "x")
			broken.Body = json.RawMessage(`{`)
			return home.Result{Rows: []home.Row{note(c), home.RowOf(broken)}}
		},
		"item": func(c home.Command) home.Result {
			return home.Result{Rows: []home.Row{note(c)}, Items: []home.Item{{AI: ownerID, Why: "notice"}}}
		},
		"disk": func(c home.Command) home.Result {
			c.Disk.Msg("\x00")
			c.Disk.Pending()
			return home.Result{Rows: []home.Row{note(c)}}
		},
		"queue": func(c home.Command) home.Result {
			return home.Result{Rows: []home.Row{note(c)}, Items: []home.Item{{AI: aiID, Why: "notice"}}}
		},
		"retire": func(c home.Command) home.Result { return home.Result{Retire: []int64{1}} },
		"jump":   func(c home.Command) home.Result { return home.Result{Jump: []int64{1}} },
	}
	tables.Commands["jump"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		var p struct {
			How string `json:"how"`
		}
		c.Decode(&p)
		return results[p.How](c), nil
	}
	w, p := runFounded(t, tables)
	p.watch(groupID, false)
	const internal = `,"err":{"code":"internal","msg":"the home could not apply the command; the cause is in its log"}}`
	fails := func(how, cause string) {
		t.Helper()
		re := p.send(groupID, "jump", `,"how":"`+how+`"`)
		p.gets(fmt.Sprintf(`{"re":%d`, re) + internal)
		if got := w.reported(); !strings.HasPrefix(got, "group "+groupID+", a command of "+browser.ID()+": ") || !strings.Contains(got, cause) {
			t.Fatalf("reported %q; want %q in it", got, cause)
		}
	}
	fails("write", "UNIQUE constraint failed: setting.key")
	fails("row", "unexpected end of JSON input")
	fails("item", `an item "notice" is for "`+ownerID+`", which is no AI of the group`)
	// A stored row that does not decode fails the Disk that reads it.
	runExec(t, w.db, `INSERT INTO row(g,r,id,seq,pos,ts,mt,pending,data) VALUES(?, 'msg', char(0), 90, 90, 1, 1, 1, '{"r":"msg","seq":"x"}')`, groupID)
	fails("disk", "cannot unmarshal string into Go struct field")
	runExec(t, w.db, `DELETE FROM row WHERE seq=90`)

	// Nothing of the four commands is in the store, and the state is where it was.
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM row"), 3)
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM receipt"), 1)
	equal(t, runQuery[int](t, w.db, "SELECT count(*) FROM setting"), 0)
	equal(t, runQuery[int64](t, w.db, "SELECT seq FROM grp WHERE g=?", groupID), int64(3))
	re := p.send(groupID, "note", `,"text":"x"`)
	p.gets(runRows(groupID, runNote(runID(w.nonce.Load()), 4, "x")), fmt.Sprintf(`{"re":%d,"ok":true}`, re))
	equal(t, hooks.all(), []string{"hook", "hook"})

	// The store's counter and the state's must move together.
	runExec(t, w.db, "UPDATE grp SET seq=seq+1")
	re = p.send(groupID, "note", `,"text":"y"`)
	p.gets(fmt.Sprintf(`{"re":%d`, re) + internal)
	equal(t, strings.HasSuffix(w.reported(), ": a row is numbered 5 where the store gives out 6"), true)
	runExec(t, w.db, "UPDATE grp SET seq=seq-1")
	re = p.send(groupID, "note", `,"text":"y"`)
	p.gets(runRows(groupID, runNote(runID(w.nonce.Load()), 5, "y")), fmt.Sprintf(`{"re":%d,"ok":true}`, re))

	// A store without its tables fails each step that needs one.
	equal(t, hooks.all(), []string{"hook", "hook", "hook"})
	runExec(t, w.db, "DROP TABLE inbox")
	fails("queue", "no such table: inbox")
	fails("retire", "no such table: inbox")
	fails("jump", "no such table: inbox")
	runExec(t, w.db, "DROP TABLE grp")
	fails("retire", "no such table: grp")
	fails("row", "no such table: grp")
	runExec(t, w.db, "DROP TABLE receipt")
	fails("retire", "no such table: receipt")
	p.quiet()
	equal(t, hooks.all(), []string{"hook", "hook", "hook"})
}

// Without a Report the loop writes its failures to the standard logger, and
// without a Now it reads the clock of the machine.
func TestRunDefaults(t *testing.T) {
	var out bytes.Buffer
	var mu sync.Mutex
	defer log.SetOutput(log.Writer())
	log.SetOutput(writerFunc(func(b []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(b)
	}))
	tables := runTables()
	var at atomic.Int64
	tables.Commands["note"] = func(s *home.State, c home.Command) (home.Result, *home.Refusal) {
		at.Store(c.Now)
		return home.Result{Writes: []home.Write{setting{"k", "v"}, setting{"k", "v"}}}, nil
	}
	db := runOpen(t)
	loop, err := home.StartLoop(home.LoopConfig{Store: db, Home: node.ID(), Owner: browser.ID(), Logs: runLogs(t), Tables: tables})
	equal(t, err, nil)
	defer loop.Close()
	client := runNewClient()
	conn := loop.Attach(browser.ID(), client)
	loop.Welcome(conn, runWelcome)
	<-client.got
	before := time.Now().UnixMilli()
	loop.Cmd(conn, 1, browser.Sign(fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q%s}`, runID(1), before, groupID, runCreateWebsite)))
	loop.Cmd(conn, 2, browser.Sign(fmt.Sprintf(`{"t":"note","n":%q,"ts":%d,"g":%q}`, runID(2), before, groupID)))
	equal(t, runFrame(<-client.got), `{"re":1,"ok":true}`)
	<-client.got
	equal(t, strings.HasPrefix(runFrame(<-client.got), `{"re":2,"err":{"code":"internal"`), true)
	if got := at.Load(); got < before || got > time.Now().UnixMilli() {
		t.Fatalf("the command ran at %d, after %d", got, before)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "home: group "+groupID+", a command of "+browser.ID()+": ") {
		t.Fatalf("logged %q", out.String())
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

// The tables of a node are the ones the files of the package add to.
func TestRunNodeTables(t *testing.T) {
	tables := home.NodeTables()
	same := func(a, b any) bool { return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer() }
	equal(t, []bool{same(tables.Commands, home.Commands), same(tables.Timers, home.Timers), same(tables.Events, home.Events), same(tables.After, home.After)},
		[]bool{true, true, true, true})
}

func TestRunDynamicOwnerFn(t *testing.T) {
	db := runOpen(t)
	var owner atomic.Value
	owner.Store("")
	loop, err := home.StartLoop(home.LoopConfig{
		Store: db, Home: node.ID(),
		OwnerFn: func() string { return owner.Load().(string) },
		Logs:    runLogs(t), Tables: runTables(),
	})
	equal(t, err, nil)
	defer loop.Close()

	client := runNewClient()
	conn := loop.Attach(browser.ID(), client)
	owner.Store(browser.ID())
	now := time.Now().UnixMilli()
	loop.Cmd(conn, 1, browser.Sign(fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q%s}`, runID(1), now, groupID, runCreateWebsite)))
	equal(t, runFrame(<-client.got), `{"re":1,"ok":true}`)
}
