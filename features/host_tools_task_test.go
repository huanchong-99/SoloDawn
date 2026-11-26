//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_tools_task"] = hostToolsTaskSteps
}

const (
	tskAccGroupID  = "rZNvy-1jH6Z-BcPqA5U5BQ"
	tskAccTaskID   = "DrtCn6htSBwmMPrFPbHJHA"
	tskAccWorkerID = "1PC8WineBrUQ-apCjx7tug"
	tskAccReviewID = "LXCZmuGAXkvO-bSrOkuCfw"
)

type tskAccHome struct {
	mu sync.Mutex
	// Single reader: Host.Home via Done.
	done     chan struct{}
	h        *host.Host
	hostDev  string
	requests []proto.Cmd
	handlers map[string]func(int64, string) (json.RawMessage, error)
	queue    bool // task start makes a task that waits as busy
}

func newTskAccHome(h *host.Host) *tskAccHome {
	m := &tskAccHome{
		done:    make(chan struct{}),
		h:       h,
		hostDev: h.Key.ID(),
	}
	m.handlers = map[string]func(int64, string) (json.RawMessage, error){
		"task/start": m.recordStart,
	}
	return m
}

func (m *tskAccHome) Send(any) error               { return nil }
func (m *tskAccHome) Path() string                 { return "local" }
func (m *tskAccHome) PathChanged() <-chan struct{} { return nil }
func (m *tskAccHome) Done() <-chan struct{}        { return m.done }

func (m *tskAccHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := int64(len(m.requests) + 1)
	req := build(id)
	cmd, ok := req.(proto.Cmd)
	if !ok {
		return nil, fmt.Errorf("unexpected frame %T", req)
	}
	known := func(dev string) bool { return dev == m.hostDev }
	if _, err := identity.OpenCommand(cmd.Env, "", known, m.h.Now()); err != nil {
		return nil, err
	}
	m.requests = append(m.requests, cmd)
	return m.applyCmd(id, cmd.Env.Payload)
}

func (m *tskAccHome) applyCmd(id int64, payload string) (json.RawMessage, error) {
	var hdr struct {
		Type string `json:"t"`
		Op   string `json:"op,omitempty"`
		Kind string `json:"kind,omitempty"`
	}
	if err := json.Unmarshal([]byte(payload), &hdr); err != nil {
		return nil, err
	}
	if fn := m.handlers[hdr.Type+"/"+hdr.Op+hdr.Kind]; fn != nil {
		return fn(id, payload)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

// recordStart answers task start; when queue is set it first pushes the row
// of a task that waits for its folder, as the home does before it answers.
func (m *tskAccHome) recordStart(id int64, payload string) (json.RawMessage, error) {
	if m.queue {
		var start proto.TaskStart
		if err := proto.Decode([]byte(payload), &start); err != nil {
			return nil, err
		}
		task := tskTaskRow(string(start.ID), "queued", "work")
		task.Wait = &proto.TaskWait{Kind: "busy"}
		raw, _ := json.Marshal(task)
		m.h.Rows.Put(tskAccGroupID, "task", string(start.ID), raw)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true,"num":1}`, id)), nil
}

func (m *tskAccHome) currentTask() proto.TaskRow {
	raw, _ := m.h.Rows.Get(tskAccGroupID, "task", tskAccTaskID)
	var task proto.TaskRow
	proto.Decode(raw, &task)
	return task
}

func (m *tskAccHome) putTask(task proto.TaskRow) {
	task.Seq++
	raw, _ := json.Marshal(task)
	m.h.Rows.Put(tskAccGroupID, "task", tskAccTaskID, raw)
}

func (m *tskAccHome) allCmds() []proto.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.requests)
}

type tskScenario struct {
	t         *testing.T
	cleanup   func(func())
	workspace string
	h         *host.Host
	home      *tskAccHome
	caller    host.Caller
	lastOut   any
	lastErr   error
	levels    []host.Caller
	summary   string
}

func hostToolsTaskSteps(sc *godog.ScenarioContext, f fixtures) {
	st := &tskScenario{t: f.t, cleanup: afterScenario(sc)}

	sc.Step(`^a working AI group session with an owner request in the main line$`, st.givenGroupSession)
	sc.Step(`^the group session lists "([^"]+)" and none of "([^"]+)"$`, st.thenCallerLists)
	sc.Step(`^the AI calls start_task with title "([^"]+)" and plan "([^"]+)"$`, st.whenStartTask)
	sc.Step(`^start_task returns task "([^"]+)" with moved true and sends a direct task start$`, st.thenStartTaskSucceeded)
	sc.Step(`^the host admits the task session of that task at levels 1 and 3$`, st.admitTaskLevels)
	sc.Step(`^at both levels the task session lists "([^"]+)" and none of "([^"]+)"$`, st.thenLevelsList)
	sc.Step(`^the home queues a new task of that AI behind another task in its folder$`, func() { st.home.queue = true })
	sc.Step(`^start_task returns task "([^"]+)" with queued true and sends a direct task start$`, st.thenStartTaskQueued)
	sc.Step(`^a task session of "Claude Code" in its workspace for an active task at step work$`, st.givenTaskSession)
	sc.Step(`^the AI calls submit_task with summary "([^"]+)"$`, st.whenSubmit)
	sc.Step(`^the task tool fails with code "([^"]+)" and nothing was sent to the home$`, st.thenFailedQuietly)
	sc.Step(`^submit_task returns ok and the home received only that summary, as a post of kind report in the task's thread$`, st.thenReported)
}

func (st *tskScenario) initHost() error {
	dataDir := st.t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "node.db"))
	if err != nil {
		return err
	}
	st.cleanup(func() { db.Close() })
	key := identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize)))
	h, _, err := host.New(db, key, time.Now)
	if err != nil {
		return err
	}
	st.h = h
	st.home = newTskAccHome(h)
	h.Attach("home-dev", st.home)
	st.seedGroupRow()
	st.workspace, err = aiWorkspace(dataDir, tskAccGroupID, "Claude Code")
	return err
}

func (st *tskScenario) seedGroupRow() {
	grpRaw, _ := json.Marshal(proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: tskAccGroupID, Seq: 1},
		Name:           "Website",
		Home:           "home-dev",
		Pass:           90,
		PassFoundation: 70,
		Limits:         proto.Limits{TurnMinutes: 1},
		Created:        1_763_600_000_000,
	})
	st.h.Rows.Put(tskAccGroupID, "group", tskAccGroupID, grpRaw)
}

func (st *tskScenario) givenGroupSession() error {
	if err := st.initHost(); err != nil {
		return err
	}
	sessKey := host.SessionKey{Group: tskAccGroupID, Member: tskAccWorkerID}
	if err := st.h.Sessions.Create(host.Session{
		SessionKey: sessKey,
		Kind:       host.GroupSession,
		Folder:     st.workspace,
		Agent:      "claude",
		Home:       "home-dev",
		Name:       "Claude Code",
	}); err != nil {
		return err
	}
	st.h.Sessions.SetTurn(sessKey, host.Turn{Kind: host.ItemsTurn})
	tok := st.h.Sessions.Admit(sessKey, 1, false)
	st.caller, _ = st.h.Sessions.Caller(tok)
	return nil
}

func (st *tskScenario) thenCallerLists(mustHave, mustNotCSV string) error {
	listed := host.ToolsFor(st.caller)
	if _, ok := listed[mustHave]; !ok {
		return fmt.Errorf("missing %q in %v", mustHave, slices.Sorted(maps.Keys(listed)))
	}
	for name := range strings.SplitSeq(mustNotCSV, ",") {
		trimmed := strings.TrimSpace(name)
		if _, ok := listed[trimmed]; ok {
			return fmt.Errorf("unexpected %q in %v", trimmed, slices.Sorted(maps.Keys(listed)))
		}
	}
	return nil
}

func (st *tskScenario) callTool(name string, args any) {
	raw, _ := json.Marshal(args)
	tool, ok := host.ToolsFor(st.caller)[name]
	if !ok {
		st.lastOut, st.lastErr = nil, fmt.Errorf("tool %q is not listed", name)
		return
	}
	st.lastOut, st.lastErr = tool.Call(st.t.Context(), st.h, st.caller, raw)
}

func (st *tskScenario) expectResultJSON(want string) error {
	if st.lastErr != nil {
		return st.lastErr
	}
	raw, _ := json.Marshal(st.lastOut)
	if string(raw) != want {
		return fmt.Errorf("got %s, want %s", raw, want)
	}
	return nil
}

func (st *tskScenario) whenStartTask(title, plan string) {
	st.callTool("start_task", map[string]string{"title": title, "plan": plan})
}

func (st *tskScenario) thenStartTaskSucceeded(wantTask string) error {
	return st.thenStartTask(fmt.Sprintf(`{"task":%q,"moved":true}`, wantTask))
}

func (st *tskScenario) thenStartTaskQueued(wantTask string) error {
	return st.thenStartTask(fmt.Sprintf(`{"task":%q,"queued":true}`, wantTask))
}

func (st *tskScenario) thenStartTask(want string) error {
	if err := st.expectResultJSON(want); err != nil {
		return err
	}
	cmds := st.home.allCmds()
	if len(cmds) != 1 {
		return fmt.Errorf("got %d cmds, want 1", len(cmds))
	}
	return tskVerifyStartPayload(cmds[0].Env.Payload)
}

func tskVerifyStartPayload(payload string) error {
	var p proto.TaskStart
	if err := proto.Decode([]byte(payload), &p); err != nil {
		return err
	}
	if p.Re != nil || p.Origin != "direct" {
		return fmt.Errorf("unexpected TaskStart: %+v", p)
	}
	if string(p.As) != tskAccWorkerID {
		return fmt.Errorf("unexpected TaskStart signer %q", p.As)
	}
	return nil
}

func tskTaskRow(id, state, step string) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id, Seq: 2},
		Group:     tskAccGroupID, Num: 1, Title: "Fix path", Origin: "direct", Plan: "Add path status and tests.",
		State: state, Step: step, Assignee: tskAccWorkerID, Reviewer: tskAccReviewID, Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: 1_763_600_000_000}, Created: 1_763_600_000_000,
	}
}

// admitTask admits the task session of tskAccTaskID in the workspace at level.
func (st *tskScenario) admitTask(level int) (host.Caller, error) {
	key := host.SessionKey{Group: tskAccGroupID, Member: tskAccWorkerID, Task: tskAccTaskID}
	if _, ok := st.h.Sessions.Get(key); !ok {
		s := host.Session{SessionKey: key, Kind: host.TaskSession, Folder: st.workspace, Agent: "claude", Home: "home-dev", Name: "Claude Code"}
		if err := st.h.Sessions.Create(s); err != nil {
			return host.Caller{}, err
		}
	}
	caller, ok := st.h.Sessions.Caller(st.h.Sessions.Admit(key, level, false))
	if !ok {
		return caller, fmt.Errorf("the task session at level %d was not admitted", level)
	}
	return caller, nil
}

func (st *tskScenario) admitTaskLevels() error {
	st.home.putTask(tskTaskRow(tskAccTaskID, "active", "work"))
	st.levels = nil
	for _, level := range []int{1, 3} {
		caller, err := st.admitTask(level)
		if err != nil {
			return err
		}
		st.levels = append(st.levels, caller)
	}
	return nil
}

func (st *tskScenario) thenLevelsList(mustHave, mustNotCSV string) error {
	for _, caller := range st.levels {
		st.caller = caller
		if err := st.thenCallerLists(mustHave, mustNotCSV); err != nil {
			return fmt.Errorf("level %d: %w", caller.Level, err)
		}
	}
	return nil
}

func (st *tskScenario) givenTaskSession() (err error) {
	if err := st.initHost(); err != nil {
		return err
	}
	st.home.putTask(tskTaskRow(tskAccTaskID, "active", "work"))
	st.caller, err = st.admitTask(3)
	return err
}

func (st *tskScenario) whenSubmit(summary string) {
	st.summary = summary
	st.callTool("submit_task", map[string]string{"summary": summary})
}

func (st *tskScenario) thenFailedQuietly(code string) error {
	if err := checkHostErrorCode(st.lastErr, code); err != nil {
		return err
	}
	if cmds := st.home.allCmds(); len(cmds) != 0 {
		return fmt.Errorf("the home received %d commands", len(cmds))
	}
	return nil
}

// thenReported checks that the one command the home received is the report:
// a post of kind report in the task's thread, by the assignee, whose text is
// the summary as the AI wrote it and whose body is empty.
func (st *tskScenario) thenReported() error {
	if err := st.expectResultJSON(`{"ok":true}`); err != nil {
		return err
	}
	cmds := st.home.allCmds()
	if len(cmds) != 1 {
		return fmt.Errorf("the home received %d commands, want 1", len(cmds))
	}
	var post proto.Post[json.RawMessage]
	if err := proto.Decode([]byte(cmds[0].Env.Payload), &post); err != nil {
		return err
	}
	got := fmt.Sprintf("%s|%s|%s|%s|%s|%d", post.Type, post.Kind, threadOf(post.Thread), post.As, post.Body, len(post.Mentions))
	if want := fmt.Sprintf("post|report|%s|%s|{}|0", tskAccTaskID, tskAccWorkerID); got != want || post.Text != st.summary {
		return fmt.Errorf("got %s with text %q, want %s with text %q", got, post.Text, want, st.summary)
	}
	return nil
}

// threadOf is the thread a post names, empty for the main line.
func threadOf(thread *proto.ID) string {
	if thread == nil {
		return ""
	}
	return string(*thread)
}
