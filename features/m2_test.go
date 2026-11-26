//go:build acceptance

package features

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["m2"] = m2Steps
}

type m2Entry struct {
	seq    int64
	pos    int64
	gone   bool
	kind   string
	text   string
	status string
}

type m2Cache struct {
	cursor      int64
	wasReset    bool
	beforeSync  []string
	entries     map[[2]string]m2Entry
	cursorSaved int64
}

type m2Browser struct {
	homeFrameClient
	name   string
	key    identity.Key
	conn   *home.Conn
	re     int64
	groups []proto.GroupSummary
	caches map[string]*m2Cache
}

func m2NewBrowser(name string, key identity.Key) *m2Browser {
	return &m2Browser{
		homeFrameClient: newHomeFrameClient(2048),
		name:            name,
		key:             key,
		caches:          map[string]*m2Cache{},
	}
}

func (b *m2Browser) cacheOf(group string) *m2Cache {
	c := b.caches[group]
	if c == nil {
		c = &m2Cache{entries: map[[2]string]m2Entry{}}
		b.caches[group] = c
	}
	return c
}

func (b *m2Browser) nextFrame(timeout time.Duration, what string) ([]byte, error) {
	w := b.wait(timeout, what)
	defer w.stop()
	return recvWait(w, b.ch)
}

func (b *m2Browser) wait(timeout time.Duration, what string) *wait {
	return newWait(timeout, b.name+": "+what, b.saw)
}

func (b *m2Browser) connect(loop *home.Loop, homeID string, now int64) error {
	b.conn = loop.Attach(b.key.ID(), b)
	b.re++
	re := b.re
	build := func(gs []proto.GroupSummary) any {
		return proto.Welcome{
			Type: "welcome", Success: proto.Success{Re: re, OK: true}, V: 3,
			Device: homeID, Time: now, Addrs: []string{}, Groups: gs, Owner: true,
		}
	}
	loop.Welcome(b.conn, build)
	raw, err := b.readResponse(re, 5*time.Second)
	if err != nil {
		return err
	}
	var w proto.Welcome
	if err := proto.Decode(raw, &w); err != nil {
		return err
	}
	b.groups = w.Groups
	return nil
}

func (b *m2Browser) disconnect(loop *home.Loop) {
	if b.conn != nil {
		loop.Detach(b.conn)
		b.conn = nil
	}
}

func (b *m2Browser) readResponse(re int64, timeout time.Duration) ([]byte, error) {
	w := b.wait(timeout, fmt.Sprintf("the answer to request %d", re))
	defer w.stop()
	for {
		raw, err := recvWait(w, b.ch)
		if err != nil {
			return nil, err
		}
		matched, err := b.handleOrMatch(raw, re)
		if err != nil || matched {
			return raw, err
		}
	}
}

func (b *m2Browser) handleOrMatch(raw []byte, re int64) (bool, error) {
	var hdr struct {
		Re *int64 `json:"re,omitempty"`
	}
	if err := json.Unmarshal(raw, &hdr); err != nil {
		return false, err
	}
	if hdr.Re != nil {
		return *hdr.Re == re, nil
	}
	return false, b.applyPush(raw)
}

func (b *m2Browser) applyPush(raw []byte) error {
	var hdr struct {
		Type string `json:"t"`
	}
	if err := json.Unmarshal(raw, &hdr); err != nil {
		return err
	}
	switch hdr.Type {
	case "groups":
		return b.applyGroupsPush(raw)
	case "rows":
		return b.applyRowsPush(raw, true, nil)
	default:
		return nil
	}
}

func (b *m2Browser) applyGroupsPush(raw []byte) error {
	var g proto.Groups
	if err := proto.Decode(raw, &g); err != nil {
		return err
	}
	b.groups = g.Groups
	return nil
}

func (b *m2Browser) applyRowsPush(raw []byte, moveCursor bool, track *[]string) error {
	var batch proto.Rows
	if err := proto.Decode(raw, &batch); err != nil {
		return err
	}
	return b.applyRowList(batch.Group, batch.Rows, moveCursor, track)
}

func (b *m2Browser) applyRowList(group string, list []json.RawMessage, moveCursor bool, track *[]string) error {
	c := b.cacheOf(group)
	for _, raw := range list {
		if err := m2ApplyOne(c, raw, moveCursor, track); err != nil {
			return err
		}
	}
	return nil
}

func m2ApplyOne(c *m2Cache, raw json.RawMessage, moveCursor bool, track *[]string) error {
	var row struct {
		Type  string          `json:"r"`
		ID    string          `json:"id"`
		Seq   int64           `json:"seq"`
		Pos   int64           `json:"pos"`
		Gone  bool            `json:"gone"`
		Kind  string          `json:"kind"`
		Text  string          `json:"text"`
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return err
	}
	status, err := m2CardStatus(row.Type, row.State)
	if err != nil {
		return err
	}
	if track != nil {
		*track = append(*track, row.Text)
	}
	entry := m2Entry{seq: row.Seq, pos: row.Pos, gone: row.Gone, kind: row.Kind, text: row.Text, status: status}
	m2StoreEntry(c, row.Type, row.ID, entry, moveCursor)
	return nil
}

func m2CardStatus(typ string, raw json.RawMessage) (string, error) {
	if typ != "msg" || len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var st struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return "", err
	}
	return st.Status, nil
}

func m2StoreEntry(c *m2Cache, typ, id string, next m2Entry, moveCursor bool) {
	key := [2]string{typ, id}
	prev, ok := c.entries[key]
	if ok && next.seq <= prev.seq {
		return
	}
	c.entries[key] = next
	if moveCursor {
		c.cursor = next.seq
	}
}

func (b *m2Browser) sync(loop *home.Loop, group string) error {
	c := b.cacheOf(group)
	c.wasReset = false
	c.beforeSync = nil
	b.re++
	re := b.re
	loop.Sync(b.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: re}, Group: group, Since: c.cursor})
	return b.collectSync(group, re)
}

func (b *m2Browser) collectSync(group string, re int64) error {
	for {
		raw, err := b.nextFrame(10*time.Second, "the sync of "+group)
		if err != nil {
			return err
		}
		done, err := b.stepSync(group, re, raw)
		if err != nil || done {
			return err
		}
	}
}

func (b *m2Browser) stepSync(group string, re int64, raw []byte) (bool, error) {
	var hdr struct {
		Type string `json:"t"`
	}
	if err := json.Unmarshal(raw, &hdr); err != nil {
		return false, err
	}
	switch hdr.Type {
	case "reset":
		b.resetGroup(group)
		return false, nil
	case "rows":
		return false, b.applyRowsPush(raw, false, &b.cacheOf(group).beforeSync)
	case "synced":
		return true, b.finishSync(group, re, raw)
	default:
		return false, b.applyPush(raw)
	}
}

func (b *m2Browser) resetGroup(group string) {
	c := b.cacheOf(group)
	c.cursor = 0
	c.wasReset = true
	c.entries = map[[2]string]m2Entry{}
}

func (b *m2Browser) finishSync(group string, re int64, raw []byte) error {
	var s proto.Synced
	if err := proto.Decode(raw, &s); err != nil {
		return err
	}
	if s.Re != re || s.Group != group {
		return fmt.Errorf("unexpected synced %+v for re %d", s, re)
	}
	b.cacheOf(group).cursor = s.Seq
	return nil
}

func (b *m2Browser) fetchMain(loop *home.Loop, group string, before int64, limit int) (proto.Fetched, error) {
	b.re++
	re := b.re
	req := home.Fetch{
		FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: re}, Group: group, Before: before, Limit: limit},
		Thread:    nil,
	}
	loop.Fetch(b.conn, req)
	raw, err := b.readResponse(re, 5*time.Second)
	if err != nil {
		return proto.Fetched{}, err
	}
	var page proto.Fetched
	if err := proto.Decode(raw, &page); err != nil {
		return proto.Fetched{}, err
	}
	return page, b.applyRowList(group, page.Rows, false, nil)
}

func (b *m2Browser) findGroup(name string) (string, bool) {
	for _, g := range b.groups {
		if g.Name == name {
			return g.Group, true
		}
	}
	return "", false
}

func (b *m2Browser) awaitGroup(name string, timeout time.Duration) (string, error) {
	w := b.wait(timeout, fmt.Sprintf("group %q in its list", name))
	defer w.stop()
	for {
		if id, ok := b.findGroup(name); ok {
			return id, nil
		}
		raw, err := recvWait(w, b.ch)
		if err != nil {
			return "", err
		}
		if err := b.applyPush(raw); err != nil {
			return "", err
		}
	}
}

func (b *m2Browser) mainTextEntries(group string) []m2Entry {
	var list []m2Entry
	for k, e := range b.cacheOf(group).entries {
		if k[0] == "msg" && !e.gone && e.kind == "text" {
			list = append(list, e)
		}
	}
	slices.SortFunc(list, func(a, c m2Entry) int { return cmp.Compare(a.pos, c.pos) })
	return list
}

func (b *m2Browser) mainTexts(group string) []string {
	entries := b.mainTextEntries(group)
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.text
	}
	return out
}

func (b *m2Browser) awaitText(group, text string, timeout time.Duration) error {
	w := b.wait(timeout, fmt.Sprintf("text %q in the main line", text))
	defer w.stop()
	for !slices.Contains(b.mainTexts(group), text) {
		raw, err := recvWait(w, b.ch)
		if err != nil {
			return err
		}
		if err := b.applyPush(raw); err != nil {
			return err
		}
	}
	return nil
}

func (b *m2Browser) hasPendingCard(group, id string) bool {
	e, ok := b.cacheOf(group).entries[[2]string{"msg", id}]
	return ok && !e.gone && e.status == "pending"
}

type m2World struct {
	clock    atomic.Int64
	cluster  *nodetest.Cluster
	browserA *m2Browser
	browserB *m2Browser
	browserC *m2Browser
	outsider *m2Browser
	groups   map[string]string
	ais      map[string]string
	cardID   string
	answers  [][]byte
	ids      seqIDGen
}

func m2Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m2World{}
	cleanup := afterScenario(sc)
	m2RegisterSetupSteps(sc, w, cleanup, f)
	m2RegisterSyncSteps(sc, w)
	m2RegisterCommandSteps(sc, w)
	m2RegisterRestartSteps(sc, w)
}

func m2RegisterSetupSteps(sc *godog.ScenarioContext, w *m2World, cleanup func(func()), f fixtures) {
	sc.Step(`^an M2 node with connected browser A and browser B of its owner$`, func() error {
		return w.startEmpty(cleanup, f)
	})
	sc.Step(`^an M2 node where browser A and browser B have synced "([^"]*)"$`, func(name string) error {
		return w.startSynced(cleanup, f, name)
	})
	sc.Step(`^an M2 node where browser A and browser B have synced "([^"]*)" with worker AI "([^"]*)"$`, func(group, ai string) error {
		return w.startWithWorker(cleanup, f, group, ai)
	})
	sc.Step(`^browser A creates the project group "([^"]*)" on the M2 home$`, w.createGroup)
	sc.Step(`^browser B sees "([^"]*)" without reloading and syncs its state$`, w.secondSeesAndSyncs)
	sc.Step(`^the local host adds the worker AI "([^"]*)" to "([^"]*)"$`, w.hostAddsWorker)
	sc.Step(`^browser (A|B) posts "([^"]*)" in "([^"]*)"$`, w.browserPosts)
	sc.Step(`^browser (A|B) receives "([^"]*)" in "([^"]*)" within 1 second$`, w.browserReceivesWithinOneSecond)
}

func m2RegisterSyncSteps(sc *godog.ScenarioContext, w *m2World) {
	sc.Step(`^browser B disconnects from the M2 home$`, func() { w.browserB.disconnect(w.homeLoop()) })
	sc.Step(`^browser A posts (\d+) numbered messages in "([^"]*)"$`, w.postNumbered)
	sc.Step(`^browser B reconnects and syncs "([^"]*)" from its cursor$`, w.reconnectAndSyncB)
	sc.Step(`^browser B was not reset on "([^"]*)" and received those (\d+) messages in order$`, w.assertDeltaInOrder)
	sc.Step(`^"([^"]*)" has posted a pending decision card in "([^"]*)" synced by browser B$`, w.postPendingCardAndSyncB)
	sc.Step(`^browser B was reset on "([^"]*)" and holds the pending decision card and newest messages (\d+) to (\d+)$`, func(group string, from, to int) error {
		return w.assertColdState(w.browserB, group, true, from, to)
	})
	sc.Step(`^browser B scrolls back two pages of (\d+) messages in "([^"]*)"$`, w.scrollBackTwoPages)
	sc.Step(`^browser B holds messages (\d+) to (\d+) in "([^"]*)" in creation order with its sync cursor unchanged$`, w.assertScrolledRange)
	sc.Step(`^a cold browser C connects and syncs "([^"]*)" from scratch$`, w.coldConnectC)
	sc.Step(`^browser C holds the pending decision card and newest messages (\d+) to (\d+) in "([^"]*)"$`, func(from, to int, group string) error {
		return w.assertColdState(w.browserC, group, true, from, to)
	})
}

func m2RegisterCommandSteps(sc *godog.ScenarioContext, w *m2World) {
	sc.Step(`^browser A sends a text post command twice with the same nonce in "([^"]*)"$`, w.sendDuplicateNonce)
	sc.Step(`^both M2 command answers are the same success and "([^"]*)" holds (\d+) message rows?$`, w.assertSameSuccessAndCount)
	sc.Step(`^a non-member device sends a text post command to "([^"]*)"$`, w.sendFromOutsider)
	sc.Step(`^the M2 home refuses that command with "([^"]*)" and "([^"]*)" still holds (\d+) message rows?$`, func(code, group string, count int) error {
		return errors.Join(w.assertLastError(code), w.assertStoredMessages(group, count))
	})
	sc.Step(`^the M2 home refuses sync and fetch from that non-member device on "([^"]*)" with "([^"]*)"$`, w.assertOutsiderReadsRefused)
}

func m2RegisterRestartSteps(sc *godog.ScenarioContext, w *m2World) {
	sc.Step(`^the M2 home stops$`, func() { w.node().StopHome() })
	sc.Step(`^browser B's connection to the home is gone and it still holds "([^"]*)" in "([^"]*)"$`, w.assertDisconnectedAndHolding)
	sc.Step(`^the M2 home restarts from its disk store$`, func() { w.node().RestartHome() })
	sc.Step(`^browser A reconnects, syncs "([^"]*)", and posts "([^"]*)"$`, w.reconnectSyncAndPostA)
	sc.Step(`^browser B reconnects and syncs "([^"]*)" without reloading$`, w.reconnectAndSyncB)
	sc.Step(`^browser B was not reset on "([^"]*)"$`, w.assertResumedB)
	sc.Step(`^both browsers hold "([^"]*)" in "([^"]*)" in creation order$`, w.assertBothHoldTexts)
}

func (w *m2World) now() time.Time          { return time.UnixMilli(w.clock.Load()) }
func (w *m2World) node() *nodetest.Node    { return w.cluster.Nodes[0] }
func (w *m2World) homeLoop() *home.Loop    { return w.node().Home }
func (w *m2World) groupID(g string) string { return w.groups[g] }

func (w *m2World) browserByLabel(which string) *m2Browser {
	if which == "A" {
		return w.browserA
	}
	return w.browserB
}

func (w *m2World) startEmpty(cleanup func(func()), f fixtures) error {
	w.clock.Store(1763600000000)
	w.groups, w.ais, w.answers, w.ids = map[string]string{}, map[string]string{}, nil, seqIDGen{}
	root, err := os.MkdirTemp("", "solodawn-m2-*")
	if err != nil {
		return err
	}
	cleanup(func() { os.RemoveAll(root) })
	keyB, keyC := exampleKey("m2 browser B"), exampleKey("m2 browser C")
	w.cluster = nodetest.Start(scenarioTB{TB: f.t, cleanup: cleanup}, nodetest.Config{
		Root:          root,
		BinDir:        f.fakeCLIs,
		Now:           w.now,
		ExtraBrowsers: []identity.Key{keyB, keyC},
	})
	w.browserA = m2NewBrowser("browser A", w.cluster.Owner)
	w.browserB = m2NewBrowser("browser B", keyB)
	w.browserC = m2NewBrowser("browser C", keyC)
	w.outsider = m2NewBrowser("outsider", exampleKey("m2 outsider"))
	return errors.Join(
		w.browserA.connect(w.homeLoop(), w.node().Key.ID(), w.clock.Load()),
		w.browserB.connect(w.homeLoop(), w.node().Key.ID(), w.clock.Load()),
	)
}

func (w *m2World) startSynced(cleanup func(func()), f fixtures, name string) error {
	if err := w.startEmpty(cleanup, f); err != nil {
		return err
	}
	if err := w.createGroup(name); err != nil {
		return err
	}
	return w.secondSeesAndSyncs(name)
}

func (w *m2World) startWithWorker(cleanup func(func()), f fixtures, group, ai string) error {
	if err := w.startSynced(cleanup, f, group); err != nil {
		return err
	}
	return w.hostAddsWorker(ai, group)
}

func (w *m2World) sendCmd(b *m2Browser, env proto.Envelope) ([]byte, error) {
	b.re++
	re := b.re
	w.homeLoop().Cmd(b.conn, re, env)
	raw, err := b.readResponse(re, 5*time.Second)
	if err == nil {
		w.answers = append(w.answers, raw)
	}
	return raw, err
}

func (w *m2World) execOK(b *m2Browser, payload any) error {
	rawPayload, err := marshalNoEscape(payload)
	if err != nil {
		return err
	}
	raw, err := w.sendCmd(b, b.key.Sign(string(rawPayload)))
	if err != nil {
		return err
	}
	return m2CheckOK(raw)
}

func m2CheckOK(raw []byte) error {
	var ok proto.CmdResult
	if err := proto.Decode(raw, &ok); err != nil || !ok.OK {
		return fmt.Errorf("expected ok response, got %s (%v)", raw, err)
	}
	return nil
}

func (w *m2World) createGroup(name string) error {
	gid := w.ids.next()
	w.groups[name] = string(gid)
	cmd := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: w.clock.Load(), Group: gid},
			Op:          "create",
		},
		Name:       name,
		MemberName: "alex",
	}
	if err := w.execOK(w.browserA, cmd); err != nil {
		return err
	}
	return w.browserA.sync(w.homeLoop(), string(gid))
}

func (w *m2World) secondSeesAndSyncs(name string) error {
	gid, err := w.browserB.awaitGroup(name, time.Second)
	if err != nil {
		return err
	}
	if gid != w.groupID(name) {
		return fmt.Errorf("browser B saw group %s, want %s", gid, w.groupID(name))
	}
	return w.browserB.sync(w.homeLoop(), gid)
}

func (w *m2World) hostAddsWorker(aiName, groupName string) error {
	aiID := w.ids.next()
	w.ais[aiName] = string(aiID)
	cmd := proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.ids.next(), Timestamp: w.clock.Load(), Group: proto.ID(w.groupID(groupName))},
			Op:          "add_ai",
			ID:          aiID,
		},
		CLI: "claude", Name: aiName, Workspace: "/work/" + aiName, Account: "personal", Approvers: []string{w.cluster.Owner.ID()},
	}
	raw, err := marshalNoEscape(cmd)
	if err != nil {
		return err
	}
	_, err = w.node().Host.Command(context.Background(), w.node().Key.ID(), string(raw))
	return err
}

func (w *m2World) makeTextPost(group, text string, nonce proto.ID) proto.Post[proto.EmptyBody] {
	return proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: nonce, Timestamp: w.clock.Load(), Group: proto.ID(w.groupID(group))},
		Kind:        "text", Text: text, Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
}

func (w *m2World) browserPosts(which, text, group string) error {
	return w.execOK(w.browserByLabel(which), w.makeTextPost(group, text, w.ids.next()))
}

func (w *m2World) browserReceivesWithinOneSecond(which, text, group string) error {
	start := time.Now()
	if err := w.browserByLabel(which).awaitText(w.groupID(group), text, time.Second); err != nil {
		return err
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		return fmt.Errorf("browser %s received %q in %v, above 1 second", which, text, elapsed)
	}
	return nil
}

func (w *m2World) postNumbered(count int, group string) error {
	for i := 1; i <= count; i++ {
		if err := w.browserPosts("A", m2MsgLabel(i), group); err != nil {
			return err
		}
	}
	return nil
}

func m2MsgLabel(i int) string { return fmt.Sprintf("msg-%04d", i) }

func (w *m2World) reconnectAndSyncB(group string) error {
	if err := w.browserB.connect(w.homeLoop(), w.node().Key.ID(), w.clock.Load()); err != nil {
		return err
	}
	return w.browserB.sync(w.homeLoop(), w.groupID(group))
}

func (w *m2World) assertDeltaInOrder(group string, count int) error {
	c := w.browserB.cacheOf(w.groupID(group))
	if c.wasReset {
		return errors.New("browser B was told to reset during delta sync")
	}
	want := m2ExpectedRange(1, count)
	if !slices.Equal(c.beforeSync, want) {
		return fmt.Errorf("got delta %v, want %v", c.beforeSync, want)
	}
	return nil
}

func m2ExpectedRange(from, to int) []string {
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, m2MsgLabel(i))
	}
	return out
}

func (w *m2World) postPendingCardAndSyncB(aiName, group string) error {
	cardID := w.ids.next()
	w.cardID = string(cardID)
	cmd := proto.Post[proto.DecisionBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: cardID, Timestamp: w.clock.Load(),
			Group: proto.ID(w.groupID(group)), As: proto.ID(w.ais[aiName]),
		},
		Kind: "decision", Main: true, Text: "Choose deployment target", Mentions: []proto.ID{},
		Body: proto.DecisionBody{
			Question: "Where should we deploy?",
			Options:  []proto.DecisionOption{{ID: "A", Label: "Fly", Effect: "Deploy to Fly"}},
			To:       []string{w.cluster.Owner.ID()},
		},
	}
	raw, err := marshalNoEscape(cmd)
	if err != nil {
		return err
	}
	if _, err := w.node().Host.Command(context.Background(), w.node().Key.ID(), string(raw)); err != nil {
		return err
	}
	return w.browserB.sync(w.homeLoop(), w.groupID(group))
}

func (w *m2World) assertColdState(b *m2Browser, group string, wantReset bool, from, to int) error {
	gid := w.groupID(group)
	c := b.cacheOf(gid)
	if c.wasReset != wantReset {
		return fmt.Errorf("%s wasReset = %v, want %v", b.name, c.wasReset, wantReset)
	}
	if !b.hasPendingCard(gid, w.cardID) {
		return fmt.Errorf("%s is missing older pending card %s", b.name, w.cardID)
	}
	got := b.mainTexts(gid)
	want := m2ExpectedRange(from, to)
	if !slices.Equal(got, want) {
		return fmt.Errorf("%s holds %d messages (first/last %v), want %d..%d", b.name, len(got), m2Ends(got), from, to)
	}
	return nil
}

func m2Ends(list []string) []string {
	if len(list) <= 2 {
		return list
	}
	return []string{list[0], list[len(list)-1]}
}

func (w *m2World) scrollBackTwoPages(limit int, group string) error {
	gid := w.groupID(group)
	c := w.browserB.cacheOf(gid)
	c.cursorSaved = c.cursor
	for range 2 {
		before := w.browserB.mainTextEntries(gid)[0].pos
		page, err := w.browserB.fetchMain(w.homeLoop(), gid, before, limit)
		if err != nil || !page.More {
			return fmt.Errorf("scroll back fetch err=%v more=%v", err, page.More)
		}
	}
	return nil
}

func (w *m2World) assertScrolledRange(from, to int, group string) error {
	gid := w.groupID(group)
	c := w.browserB.cacheOf(gid)
	if c.cursor != c.cursorSaved {
		return fmt.Errorf("cursor moved during fetch from %d to %d", c.cursorSaved, c.cursor)
	}
	got := w.browserB.mainTexts(gid)
	want := m2ExpectedRange(from, to)
	if !slices.Equal(got, want) {
		return fmt.Errorf("scrolled messages mismatch: len=%d ends=%v, want %d..%d", len(got), m2Ends(got), from, to)
	}
	return nil
}

func (w *m2World) coldConnectC(group string) error {
	if err := w.browserC.connect(w.homeLoop(), w.node().Key.ID(), w.clock.Load()); err != nil {
		return err
	}
	return w.browserC.sync(w.homeLoop(), w.groupID(group))
}

func (w *m2World) sendDuplicateNonce(group string) error {
	rawCmd, err := marshalNoEscape(w.makeTextPost(group, "Idempotent message", w.ids.next()))
	if err != nil {
		return err
	}
	env := w.browserA.key.Sign(string(rawCmd))
	w.answers = nil
	_, err1 := w.sendCmd(w.browserA, env)
	_, err2 := w.sendCmd(w.browserA, env)
	return errors.Join(err1, err2)
}

func (w *m2World) assertSameSuccessAndCount(group string, wantCount int) error {
	var first, second proto.CmdResult
	err1 := proto.Decode(w.answers[0], &first)
	err2 := proto.Decode(w.answers[1], &second)
	if err := errors.Join(err1, err2); err != nil || !first.OK || first.Re == second.Re {
		return fmt.Errorf("unexpected answers %s and %s (%v, %v)", w.answers[0], w.answers[1], err1, err2)
	}
	second.Re = first.Re
	if !reflect.DeepEqual(first, second) {
		return fmt.Errorf("answers differ: %+v vs %+v", first, second)
	}
	return w.assertStoredMessages(group, wantCount)
}

func (w *m2World) sendFromOutsider(group string) error {
	w.outsider.conn = w.homeLoop().Attach(w.outsider.key.ID(), w.outsider)
	defer w.outsider.disconnect(w.homeLoop())
	rawCmd, err := marshalNoEscape(w.makeTextPost(group, "Unauthorized post", w.ids.next()))
	if err != nil {
		return err
	}
	_, err = w.sendCmd(w.outsider, w.outsider.key.Sign(string(rawCmd)))
	return err
}

func (w *m2World) assertOutsiderReadsRefused(group, code string) error {
	gid := w.groupID(group)
	w.outsider.conn = w.homeLoop().Attach(w.outsider.key.ID(), w.outsider)
	defer w.outsider.disconnect(w.homeLoop())
	w.outsider.re++
	syncRe := w.outsider.re
	w.homeLoop().Sync(w.outsider.conn, proto.Sync{Request: proto.Request{Type: "sync", ID: syncRe}, Group: gid})
	syncRaw, err1 := w.outsider.readResponse(syncRe, 5*time.Second)
	w.outsider.re++
	fetchRe := w.outsider.re
	w.homeLoop().Fetch(w.outsider.conn, home.Fetch{FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: fetchRe}, Group: gid, Limit: 10}})
	fetchRaw, err2 := w.outsider.readResponse(fetchRe, 5*time.Second)
	return errors.Join(err1, err2, m2CheckError(syncRaw, code), m2CheckError(fetchRaw, code))
}

func (w *m2World) assertLastError(code string) error {
	return m2CheckError(w.answers[len(w.answers)-1], code)
}

func m2CheckError(raw []byte, code string) error {
	var resp proto.ErrorResponse
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Err.Code != code || resp.Err.Message == "" {
		return fmt.Errorf("got %s (%v), want error %q", raw, err, code)
	}
	return nil
}

func (w *m2World) assertStoredMessages(group string, want int) error {
	gid := w.groupID(group)
	var count int
	query := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM row WHERE g = ? AND r = 'msg'`, gid).Scan(&count)
	}
	if err := w.node().Store.Write(query); err != nil {
		return err
	}
	if count != want {
		return fmt.Errorf("stored msg rows = %d, want %d", count, want)
	}
	return nil
}

func (w *m2World) assertDisconnectedAndHolding(heldText, group string) error {
	select {
	case <-w.browserB.conn.Gone():
	default:
		return errors.New("expected browser B connection Gone to be closed")
	}
	gid := w.groupID(group)
	if !slices.Equal(w.browserB.mainTexts(gid), []string{heldText}) {
		return fmt.Errorf("browser B cached texts = %v, want [%q]", w.browserB.mainTexts(gid), heldText)
	}
	return nil
}

func (w *m2World) reconnectSyncAndPostA(group, text string) error {
	if err := w.browserA.connect(w.homeLoop(), w.node().Key.ID(), w.clock.Load()); err != nil {
		return err
	}
	if err := w.browserA.sync(w.homeLoop(), w.groupID(group)); err != nil {
		return err
	}
	return w.browserPosts("A", text, group)
}

func (w *m2World) assertResumedB(group string) error {
	c := w.browserB.cacheOf(w.groupID(group))
	if c.wasReset {
		return errors.New("browser B was reset after home restart")
	}
	return nil
}

func (w *m2World) assertBothHoldTexts(csv, group string) error {
	want := strings.Split(csv, ", ")
	gid := w.groupID(group)
	last := want[len(want)-1]
	if err := errors.Join(w.browserA.awaitText(gid, last, time.Second), w.browserB.awaitText(gid, last, time.Second)); err != nil {
		return err
	}
	gotA, gotB := w.browserA.mainTexts(gid), w.browserB.mainTexts(gid)
	if !slices.Equal(gotA, want) || !slices.Equal(gotB, want) {
		return fmt.Errorf("browser A=%v, browser B=%v, want %v", gotA, gotB, want)
	}
	return nil
}
