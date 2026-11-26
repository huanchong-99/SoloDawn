//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"maps"
	"os"
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

// tskNoChecks is what the steps that MODEL-1126 took away say when they fail.
const tskNoChecks = "a submission is a post of kind report and a review names the report: no worktree, checks, push, pull request or level lists (MODEL-1126 plan 1.6; host_tools_task.feature scenarios 2 and 3 become one)"

func init() {
	stepSets["host_tools_task"] = hostToolsTaskSteps
}

const (
	tskAccGroupID  = "rZNvy-1jH6Z-BcPqA5U5BQ"
	tskAccTaskID   = "DrtCn6htSBwmMPrFPbHJHA"
	tskAccWorkerID = "1PC8WineBrUQ-apCjx7tug"
)

type tskAccHome struct {
	mu sync.Mutex
	// Single reader: Host.Home via Done.
	done     chan struct{}
	h        *host.Host
	hostDev  string
	requests []proto.Cmd
	handlers map[string]func(int64, string) (json.RawMessage, error)
}

func newTskAccHome(h *host.Host) *tskAccHome {
	m := &tskAccHome{
		done:    make(chan struct{}),
		h:       h,
		hostDev: h.Key.ID(),
	}
	m.handlers = map[string]func(int64, string) (json.RawMessage, error){
		"task/start":  m.recordStart,
		"post/review": m.recordReview,
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

func (m *tskAccHome) recordStart(id int64, _ string) (json.RawMessage, error) {
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true,"num":1}`, id)), nil
}

func (m *tskAccHome) recordReview(id int64, _ string) (json.RawMessage, error) {
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true,"passed":true,"round":1}`, id)), nil
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
}

func hostToolsTaskSteps(sc *godog.ScenarioContext, f fixtures) {
	st := &tskScenario{t: f.t, cleanup: afterScenario(sc)}

	sc.Step(`^a working AI group session with an owner request in the main line$`, st.givenGroupSession)
	sc.Step(`^the group session lists "([^"]+)" and none of "([^"]+)"$`, st.thenCallerLists)
	sc.Step(`^the AI calls start_task with title "([^"]+)" and plan "([^"]+)"$`, st.whenStartTask)
	sc.Step(`^start_task returns task "([^"]+)" with moved true and sends a direct task start$`, st.thenStartTaskSucceeded)
	sc.Step(`^the host creates the task worktree and admits the task session at levels 1 and 2$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^level 1 lists none of "([^"]+)" and level 2 lists all three without "([^"]+)"$`, rewritten("AC-2", tskNoChecks))

	sc.Step(`^a level-2 task session on a local repository with a project check$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the task worktree has an uncommitted file and the AI calls run_checks$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the task tool fails with code "([^"]+)"$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^no task checked command was sent to the home$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the AI commits a failing change with a test file and runs run_checks then submit_task$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^run_checks reports ok false and submit_task fails with code "([^"]+)"$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the AI commits a passing fix and runs run_checks then submit_task with summary "([^"]+)"$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^run_checks reports ok true and submit_task sends report and task submitted without a pull request$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the AI commits one more change without re-running checks and calls submit_task$`, rewritten("AC-2", tskNoChecks))

	sc.Step(`^a level-2 task session with a bare Git origin and a fake gh$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the AI commits a passing change with a test file and calls push_branch$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^push_branch pushes the assigned branch to origin and returns its commit$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the AI runs run_checks and submit_task with summary "([^"]+)"$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^submit_task opens pull request 31, sends report and task submitted, and reports task cloud$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^the reviewer opens the review checkout for that commit and submits a review with score (\d+)$`, rewritten("AC-2", tskNoChecks))
	sc.Step(`^submit_review sends a signed review post for that commit and returns passed true in round 1$`, rewritten("AC-2", tskNoChecks))
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
	if err := st.expectResultJSON(fmt.Sprintf(`{"task":%q,"moved":true}`, wantTask)); err != nil {
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

func (st *tskScenario) setScenarioEnv(key, val string) {
	old, had := os.LookupEnv(key)
	st.cleanup(func() {
		if had {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
	os.Setenv(key, val)
}
