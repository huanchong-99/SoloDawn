//go:build acceptance

package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_fetch"] = func(sc *godog.ScenarioContext, _ fixtures) { homeFetchSteps(sc) }
}

type homeFetchWorld struct {
	now       time.Time
	db        *store.Store
	loop      *home.Loop
	browser   identity.Key
	conn      *home.Conn
	frames    chan any
	group     string
	task      string
	otherTask string
	sent      int
	re        int64
	lastRaw   string
	lastPage  proto.Fetched
	stalePage proto.Fetched
	cache     map[[2]string]homeFetchCached
}

type homeFetchCached struct {
	seq  int64
	gone bool
}

type homeFetchWrite struct {
	Op     string  `json:"op"`
	ID     string  `json:"id"`
	Thread *string `json:"thread,omitempty"`
	Main   bool    `json:"main,omitempty"`
	Kind   string  `json:"kind,omitempty"`
	Status string  `json:"status,omitempty"`
	Pos    int64   `json:"pos,omitempty"`
	Text   string  `json:"text,omitempty"`
}

func (w *homeFetchWorld) Send(frame any) error  { w.frames <- frame; return nil }
func (w *homeFetchWorld) Done() <-chan struct{} { return nil }

func homeFetchSteps(sc *godog.ScenarioContext) {
	w := &homeFetchWorld{now: time.UnixMilli(1763600000000)}
	cleanup := afterScenario(sc)
	sc.Step(`^a group home with main-line messages, a decided card and a task thread$`, func() error {
		return w.seedThreads(cleanup)
	})
	sc.Step(`^a group home with notes, review messages across threads and four 16 KiB notes$`, func() error {
		return w.seedKindsAndLargeNotes(cleanup)
	})
	sc.Step(`^a group home where the browser watches the group and holds notes "([^"]*)" and "([^"]*)"$`, func(a, b string) error {
		return w.seedWatchedNotes(cleanup, a, b)
	})
	sc.Step(`^the browser fetches the main line from 0 with limit (\d+)$`, func(limit int) error {
		return w.fetchThread(nil, 0, limit)
	})
	sc.Step(`^the browser fetches the main line below the last pos with limit (\d+)$`, func(limit int) error {
		return w.fetchThread(nil, w.lowestPos(), limit)
	})
	sc.Step(`^the browser fetches the task thread from 0 with limit (\d+)$`, func(limit int) error {
		return w.fetchThread(&w.task, 0, limit)
	})
	sc.Step(`^the browser fetches the task thread below the last pos with limit (\d+)$`, func(limit int) error {
		return w.fetchThread(&w.task, w.lowestPos(), limit)
	})
	sc.Step(`^the browser fetches kind "([^"]*)" from 0 with limit (\d+)$`, func(kind string, limit int) error {
		return w.fetchKind(kind, 0, limit)
	})
	sc.Step(`^the browser fetches kind "([^"]*)" below the last pos with limit (\d+)$`, func(kind string, limit int) error {
		return w.fetchKind(kind, w.lowestPos(), limit)
	})
	sc.Step(`^the browser fetches the one row "([^"]*)" "([^"]*)"$`, w.fetchOneRow)
	sc.Step(`^the fetched page holds the rows "([^"]*)" with more (true|false)$`, w.assertPage)
	sc.Step(`^the fetched frame is at most (\d+) bytes and holds "([^"]*)" with more (true|false)$`, w.assertBoundedPage)
	sc.Step(`^an older fetch page of notes is taken before "([^"]*)" is forgotten$`, w.takeStaleThenForget)
	sc.Step(`^the browser applies the pushed deletion marker and then the older fetch page$`, w.applyPushThenStalePage)
	sc.Step(`^the browser's cache omits "([^"]*)" and keeps "([^"]*)"$`, w.assertCache)
}

func (w *homeFetchWorld) boot(cleanup func(func())) error {
	w.browser = exampleKey("browser")
	node := exampleKey("node")
	w.group = string(exampleID("fetch group"))
	w.task = string(exampleID("fetch task 1"))
	w.otherTask = string(exampleID("fetch task 2"))
	w.frames = make(chan any, 128)
	w.cache = map[[2]string]homeFetchCached{}

	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.db = db

	loop, err := home.StartLoop(home.LoopConfig{
		Store: db,
		Home:  node.ID(),
		Owner: w.browser.ID(),
		Logs: func(user string) *identity.Log {
			log, _ := homeLog([]identity.Key{w.browser, node})
			if log == nil || log.User != user {
				return nil
			}
			return log
		},
		Tables: home.Tables{Commands: home.Table{"group/create": homeRunCreate, "note": homeFetchApply}},
		Now:    func() time.Time { return w.now },
	})
	if err != nil {
		return err
	}
	cleanup(loop.Close)
	w.loop = loop
	w.conn = loop.Attach(w.browser.ID(), w)
	return w.execCmd("group", map[string]any{"op": "create", "name": "Website"})
}

func homeFetchApply(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p homeFetchWrite
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	switch p.Op {
	case "forget":
		return home.Result{Rows: []home.Row{home.RowOf(proto.GoneRow{
			RowHeader: proto.RowHeader{Type: p.Kind, ID: p.ID},
			Gone:      true,
		})}}, nil
	case "note":
		return home.Result{Rows: []home.Row{home.RowOf(proto.NoteRow{
			RowHeader: proto.RowHeader{Type: "note", ID: p.ID},
			Pos:       p.Pos,
			Group:     s.ID,
			Text:      p.Text,
			By:        c.Who.Member(),
			Timestamp: c.Now,
		})}}, nil
	default:
		return homeFetchMsg(s.ID, c.Who.Member(), c.Now, p), nil
	}
}

func homeFetchMsg(group, by string, now int64, p homeFetchWrite) home.Result {
	var state *proto.CardState
	if p.Status != "" {
		state = &proto.CardState{Status: p.Status}
	}
	return home.Result{Rows: []home.Row{home.RowOf(proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: p.ID},
		Pos:       p.Pos,
		Group:     group,
		Thread:    p.Thread,
		Main:      p.Main,
		By:        by,
		Kind:      p.Kind,
		Timestamp: now,
		Text:      p.Text,
		Mentions:  []string{},
		Body:      json.RawMessage(`{}`),
		State:     state,
	})}}
}

func (w *homeFetchWorld) execCmd(t string, fields map[string]any) error {
	w.sent++
	w.re++
	payload := map[string]any{"t": t, "n": exampleID(fmt.Sprint("home fetch ", w.sent)), "ts": w.now.UnixMilli(), "g": w.group}
	for k, v := range fields {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.loop.Cmd(w.conn, w.re, w.browser.Sign(string(raw)))
	got, err := w.nextJSON()
	if want := fmt.Sprintf(`{"re":%d,"ok":true}`, w.re); err != nil || got != want {
		return fmt.Errorf("command %d got %s (%v); want %s", w.re, got, err, want)
	}
	return nil
}

func (w *homeFetchWorld) writeItem(item homeFetchWrite) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	return w.execCmd("note", fields)
}

func (w *homeFetchWorld) nextJSON() (string, error) {
	select {
	case frame := <-w.frames:
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.SetEscapeHTML(false)
		if err := e.Encode(frame); err != nil {
			return "", err
		}
		return strings.TrimSuffix(b.String(), "\n"), nil
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("timed out waiting for frame")
	}
}

func (w *homeFetchWorld) seedThreads(cleanup func(func())) error {
	if err := w.boot(cleanup); err != nil {
		return err
	}
	// Group create wrote seq 1 and 2. Next items get seq/pos 3, 4, 5, 6, 7, then c2 is decided at seq 8 keeping pos 4.
	items := []homeFetchWrite{
		{Op: "msg", ID: "m1", Kind: "text", Text: "first"},
		{Op: "msg", ID: "c2", Thread: &w.task, Main: true, Kind: "decision", Status: "pending", Text: "card"},
		{Op: "msg", ID: "t3", Thread: &w.task, Main: false, Kind: "text", Text: "thread reply"},
		{Op: "msg", ID: "m4", Kind: "text", Text: "fourth"},
		{Op: "msg", ID: "m5", Kind: "text", Text: "fifth"},
		{Op: "msg", ID: "c2", Pos: 4, Thread: &w.task, Main: true, Kind: "decision", Status: "decided", Text: "card"},
	}
	for _, it := range items {
		if err := w.writeItem(it); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeFetchWorld) seedKindsAndLargeNotes(cleanup func(func())) error {
	if err := w.boot(cleanup); err != nil {
		return err
	}
	items := []homeFetchWrite{
		{Op: "note", ID: "n1", Text: "first note"},
		{Op: "msg", ID: "rev1", Thread: &w.task, Kind: "review", Text: "first review"},
		{Op: "note", ID: "n2", Text: "second note"},
		{Op: "msg", ID: "rev2", Thread: &w.otherTask, Kind: "review", Text: "second review"},
	}
	for _, name := range []string{"big1", "big2", "big3", "big4"} {
		items = append(items, homeFetchWrite{Op: "note", ID: name, Text: strings.Repeat("x", 16_000)})
	}
	for _, it := range items {
		if err := w.writeItem(it); err != nil {
			return err
		}
	}
	return nil
}

func (w *homeFetchWorld) seedWatchedNotes(cleanup func(func()), first, second string) error {
	if err := w.boot(cleanup); err != nil {
		return err
	}
	for _, name := range []string{first, second} {
		if err := w.writeItem(homeFetchWrite{Op: "note", ID: name, Text: "content of " + name}); err != nil {
			return err
		}
	}
	if _, ok := w.loop.Watch(w.conn, w.group, false); !ok {
		return fmt.Errorf("watch refused")
	}
	return nil
}

func (w *homeFetchWorld) sendFetch(raw map[string]any) error {
	w.re++
	raw["t"] = "fetch"
	raw["id"] = w.re
	raw["g"] = w.group
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	w.loop.Fetch(w.conn, w.re, encoded)
	text, err := w.nextJSON()
	if err != nil {
		return err
	}
	w.lastRaw = text
	w.lastPage = proto.Fetched{}
	return proto.Decode([]byte(text), &w.lastPage)
}

func (w *homeFetchWorld) fetchThread(thread *string, before int64, limit int) error {
	return w.sendFetch(map[string]any{"thread": thread, "before": before, "limit": limit})
}

func (w *homeFetchWorld) fetchKind(kind string, before int64, limit int) error {
	return w.sendFetch(map[string]any{"kind": kind, "before": before, "limit": limit})
}

func (w *homeFetchWorld) fetchOneRow(r, row string) error {
	return w.sendFetch(map[string]any{"r": r, "row": row, "before": 0, "limit": 1})
}

func (w *homeFetchWorld) lowestPos() int64 {
	var last struct {
		Pos int64 `json:"pos"`
	}
	json.Unmarshal(w.lastPage.Rows[len(w.lastPage.Rows)-1], &last)
	return last.Pos
}

func (w *homeFetchWorld) assertPage(wantCSV string, wantMore string) error {
	want := homeFetchSplit(wantCSV)
	got, err := homeFetchRowIDs(w.lastPage.Rows)
	if err != nil {
		return err
	}
	more := wantMore == "true"
	if !reflect.DeepEqual(got, want) || w.lastPage.More != more {
		return fmt.Errorf("got rows %v (more=%v); want %v (more=%v)", got, w.lastPage.More, want, more)
	}
	return nil
}

func homeFetchSplit(csv string) []string {
	if csv == "" {
		return []string{}
	}
	return strings.Split(csv, ", ")
}

func homeFetchRowIDs(rows []json.RawMessage) ([]string, error) {
	got := []string{}
	for _, row := range rows {
		if strings.Contains(string(row), `"gone":true`) {
			return nil, fmt.Errorf("fetched page included gone marker: %s", row)
		}
		var h proto.RowHeader
		if err := json.Unmarshal(row, &h); err != nil {
			return nil, err
		}
		got = append(got, h.ID)
	}
	return got, nil
}

func (w *homeFetchWorld) assertBoundedPage(maxBytes int, wantCSV, wantMore string) error {
	if len(w.lastRaw) > maxBytes {
		return fmt.Errorf("frame is %d bytes, above %d", len(w.lastRaw), maxBytes)
	}
	return w.assertPage(wantCSV, wantMore)
}

func (w *homeFetchWorld) takeStaleThenForget(id string) error {
	if err := w.fetchKind("note", 0, 10); err != nil {
		return err
	}
	w.stalePage = w.lastPage
	return w.writeItem(homeFetchWrite{Op: "forget", Kind: "note", ID: id})
}

func (w *homeFetchWorld) applyPushThenStalePage() error {
	pushed, err := w.nextJSON()
	if err != nil {
		return err
	}
	var rows proto.Rows
	if err := proto.Decode([]byte(pushed), &rows); err != nil {
		return err
	}
	w.applyRows(rows.Rows)
	w.applyRows(w.stalePage.Rows)
	return nil
}

func (w *homeFetchWorld) applyRows(list []json.RawMessage) {
	for _, raw := range list {
		var entry struct {
			Type string `json:"r"`
			ID   string `json:"id"`
			Seq  int64  `json:"seq"`
			Gone bool   `json:"gone"`
		}
		if json.Unmarshal(raw, &entry) != nil {
			continue
		}
		key := [2]string{entry.Type, entry.ID}
		if cur, ok := w.cache[key]; !ok || entry.Seq > cur.seq {
			w.cache[key] = homeFetchCached{seq: entry.Seq, gone: entry.Gone}
		}
	}
}

func (w *homeFetchWorld) assertCache(omitted, kept string) error {
	del, hasDel := w.cache[[2]string{"note", omitted}]
	live, hasLive := w.cache[[2]string{"note", kept}]
	if !hasDel || !del.gone {
		return fmt.Errorf("expected gone marker cached for %s, got %+v (present=%v)", omitted, del, hasDel)
	}
	if !hasLive || live.gone {
		return fmt.Errorf("expected live row cached for %s, got %+v (present=%v)", kept, live, hasLive)
	}
	return nil
}
