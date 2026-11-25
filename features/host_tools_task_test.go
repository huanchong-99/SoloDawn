//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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
	if dir := os.Getenv("SOLODAWN_TEST_TASK_GH"); dir != "" && strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "gh" {
		os.Exit(tskReplayGH(dir))
	}
	stepSets["host_tools_task"] = hostToolsTaskSteps
}

type tskGHReply struct {
	Stdout string
	Stderr string
	Code   int
}

func tskReplayGH(dir string) int {
	idx, ok := tskRecordGHCall(filepath.Join(dir, "calls"))
	if !ok {
		return 2
	}
	raw, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(idx)))
	if err != nil {
		return 1
	}
	var ans tskGHReply
	if err := json.Unmarshal(raw, &ans); err != nil {
		return 2
	}
	fmt.Fprint(os.Stdout, ans.Stdout)
	fmt.Fprint(os.Stderr, ans.Stderr)
	return ans.Code
}

func tskRecordGHCall(logPath string) (int, bool) {
	prev, err := os.ReadFile(logPath)
	if err != nil {
		return 0, false
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(os.Args[1:]); err != nil {
		return 0, false
	}
	return bytes.Count(prev, []byte{'\n'}), true
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
	remote   bool
	requests []proto.Cmd
	// Single reader: thenRemoteSubmitAndCloudSucceeded.
	cloudCh chan proto.TaskChecked
	// Single reader: awaitRowPush.
	pushedCh chan struct{}
	handlers map[string]func(int64, string) (json.RawMessage, error)
}

func newTskAccHome(h *host.Host, remote bool) *tskAccHome {
	m := &tskAccHome{
		done:     make(chan struct{}),
		h:        h,
		hostDev:  h.Key.ID(),
		remote:   remote,
		cloudCh:  make(chan proto.TaskChecked, 1),
		pushedCh: make(chan struct{}, 8),
	}
	m.handlers = map[string]func(int64, string) (json.RawMessage, error){
		"task/start":     m.recordStart,
		"task/checked":   m.recordLocalChecks,
		"task/submitted": m.recordSubmitted,
		"task/cloud":     m.recordCloud,
		"post/review":    m.recordReview,
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

func (m *tskAccHome) recordLocalChecks(id int64, payload string) (json.RawMessage, error) {
	var p proto.TaskChecked
	if err := proto.Decode([]byte(payload), &p); err != nil {
		return nil, err
	}
	at := m.h.Now().UnixMilli()
	go m.pushTaskAfterAnswer(func(task *proto.TaskRow) {
		task.Checks.Local = &proto.Check{OK: p.OK, At: at, Commit: p.Commit}
	})
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (m *tskAccHome) recordSubmitted(id int64, payload string) (json.RawMessage, error) {
	var p proto.TaskSubmitted
	if err := proto.Decode([]byte(payload), &p); err != nil {
		return nil, err
	}
	step := "review"
	if m.remote {
		step = "cloud_check"
	}
	go m.pushTaskAfterAnswer(func(task *proto.TaskRow) {
		task.Commit, task.PR, task.PRURL = p.Commit, p.PR, p.PRURL
		task.Stats.Added, task.Stats.Removed = p.Stats.Added, p.Stats.Removed
		task.Step = step
	})
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (m *tskAccHome) pushTaskAfterAnswer(edit func(*proto.TaskRow)) {
	m.mu.Lock()
	task := m.currentTask()
	edit(&task)
	m.putTask(task)
	m.mu.Unlock()
	m.pushedCh <- struct{}{}
}

func (m *tskAccHome) recordCloud(id int64, payload string) (json.RawMessage, error) {
	var p proto.TaskChecked
	if err := proto.Decode([]byte(payload), &p); err != nil {
		return nil, err
	}
	select {
	case m.cloudCh <- p:
	default:
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
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
	t          *testing.T
	cleanup    func(func())
	dataDir    string
	projectDir string
	remoteDir  string
	wtDir      string
	ghDir      string
	h          *host.Host
	home       *tskAccHome
	caller     host.Caller
	lvl1Caller host.Caller
	lvl2Caller host.Caller
	lastOut    any
	lastErr    error
	lastCheck  bool
	head       string
}

func hostToolsTaskSteps(sc *godog.ScenarioContext, f fixtures) {
	st := &tskScenario{t: f.t, cleanup: afterScenario(sc)}

	sc.Step(`^a working AI group session with an owner request in the main line$`, st.givenGroupSession)
	sc.Step(`^the group session lists "([^"]+)" and none of "([^"]+)"$`, st.thenCallerLists)
	sc.Step(`^the AI calls start_task with title "([^"]+)" and plan "([^"]+)"$`, st.whenStartTask)
	sc.Step(`^start_task returns task "([^"]+)" with moved true and sends a direct task start$`, st.thenStartTaskSucceeded)
	sc.Step(`^the host creates the task worktree and admits the task session at levels 1 and 2$`, st.whenCreateAndAdmitTaskSessions)
	sc.Step(`^level 1 lists none of "([^"]+)" and level 2 lists all three without "([^"]+)"$`, st.thenLevelListsMatch)

	sc.Step(`^a level-2 task session on a local repository with a project check$`, func() error {
		return st.givenTaskSession(false)
	})
	sc.Step(`^the task worktree has an uncommitted file and the AI calls run_checks$`, st.whenDirtyRunChecks)
	sc.Step(`^the task tool fails with code "([^"]+)"$`, st.thenToolFailedWithCode)
	sc.Step(`^no task checked command was sent to the home$`, st.thenNoCheckedSent)
	sc.Step(`^the AI commits a failing change with a test file and runs run_checks then submit_task$`, st.whenFailingCheckThenSubmit)
	sc.Step(`^run_checks reports ok false and submit_task fails with code "([^"]+)"$`, st.thenFailedCheckRefusesSubmit)
	sc.Step(`^the AI commits a passing fix and runs run_checks then submit_task with summary "([^"]+)"$`, st.whenPassingCheckThenSubmit)
	sc.Step(`^run_checks reports ok true and submit_task sends report and task submitted without a pull request$`, st.thenLocalSubmitSucceeded)
	sc.Step(`^the AI commits one more change without re-running checks and calls submit_task$`, st.whenCommitAfterCheckThenSubmit)

	sc.Step(`^a level-2 task session with a bare Git origin and a fake gh$`, func() error {
		return st.givenTaskSession(true)
	})
	sc.Step(`^the AI commits a passing change with a test file and calls push_branch$`, st.whenCommitAndPushBranch)
	sc.Step(`^push_branch pushes the assigned branch to origin and returns its commit$`, st.thenPushBranchSucceeded)
	sc.Step(`^the AI runs run_checks and submit_task with summary "([^"]+)"$`, st.whenRemoteChecksAndSubmit)
	sc.Step(`^submit_task opens pull request 31, sends report and task submitted, and reports task cloud$`, st.thenRemoteSubmitAndCloudSucceeded)
	sc.Step(`^the reviewer opens the review checkout for that commit and submits a review with score (\d+)$`, st.whenReviewerSubmits)
	sc.Step(`^submit_review sends a signed review post for that commit and returns passed true in round 1$`, st.thenReviewSucceeded)
}

func (st *tskScenario) initHost(remote bool, checks ...string) error {
	st.dataDir = st.t.TempDir()
	db, err := store.Open(filepath.Join(st.dataDir, "node.db"))
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
	st.home = newTskAccHome(h, remote)
	h.Attach("home-dev", st.home)
	st.seedGroupRow(remote, checks)
	return st.setupRepos(remote)
}

func (st *tskScenario) seedGroupRow(remote bool, checks []string) {
	forge := "none"
	if remote {
		forge = "github"
	}
	if checks == nil {
		checks = []string{}
	}
	grpRaw, _ := json.Marshal(proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: tskAccGroupID, Seq: 1},
		Name:           "Website",
		Kind:           "project",
		Base:           "base",
		Forge:          forge,
		Home:           "home-dev",
		Checks:         checks,
		Pass:           90,
		PassFoundation: 70,
		Limits:         proto.Limits{TurnMinutes: 1},
		Created:        1_763_600_000_000,
	})
	st.h.Rows.Put(tskAccGroupID, "group", tskAccGroupID, grpRaw)
}

func (st *tskScenario) setupRepos(remote bool) error {
	src := st.t.TempDir()
	if _, err := hostForgeGit(src, []string{"init", "-b", "base"}, hostForgeCommit("initial")); err != nil {
		return err
	}
	source := src
	if remote {
		if err := st.initBareOrigin(src); err != nil {
			return err
		}
		source = st.remoteDir
	}
	st.projectDir = filepath.Join(st.dataDir, "projects", tskAccGroupID)
	_, err := host.EnsureGroupWorkspace(st.t.Context(), st.projectDir, source, "base")
	return err
}

func (st *tskScenario) initBareOrigin(src string) error {
	st.remoteDir = st.t.TempDir()
	if err := initBareRepo(st.remoteDir, "base"); err != nil {
		return err
	}
	_, err := hostForgeGit(src, []string{"remote", "add", "origin", st.remoteDir}, []string{"push", "origin", "base"})
	return err
}

func (st *tskScenario) givenGroupSession() error {
	if err := st.initHost(false); err != nil {
		return err
	}
	sessKey := host.SessionKey{Group: tskAccGroupID, Member: tskAccWorkerID}
	if err := st.h.Sessions.Create(host.Session{
		SessionKey: sessKey,
		Kind:       host.GroupSession,
		Folder:     "repo",
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

func (st *tskScenario) whenCreateAndAdmitTaskSessions() error {
	if _, err := host.CreateTaskWorkspace(st.t.Context(), st.projectDir, 1, "fix-path-event", tskAccWorkerID, "base", false); err != nil {
		return err
	}
	sessKey := host.SessionKey{Group: tskAccGroupID, Member: tskAccWorkerID, Task: tskAccTaskID}
	if err := st.h.Sessions.Create(host.Session{
		SessionKey: sessKey,
		Kind:       host.TaskSession,
		Folder:     "wt/1-fix-path-event",
		Agent:      "claude",
		Home:       "home-dev",
		Name:       "Claude Code",
	}); err != nil {
		return err
	}
	st.lvl1Caller, _ = st.h.Sessions.Caller(st.h.Sessions.Admit(sessKey, 1, false))
	st.lvl2Caller, _ = st.h.Sessions.Caller(st.h.Sessions.Admit(sessKey, 2, false))
	return nil
}

func (st *tskScenario) thenLevelListsMatch(taskToolsCSV, groupTool string) error {
	l1 := host.ToolsFor(st.lvl1Caller)
	l2 := host.ToolsFor(st.lvl2Caller)
	for name := range strings.SplitSeq(taskToolsCSV, ",") {
		if err := tskCheckLevelTool(l1, l2, strings.TrimSpace(name)); err != nil {
			return err
		}
	}
	if _, ok := l2[groupTool]; ok {
		return fmt.Errorf("level 2 task session listed %q", groupTool)
	}
	return nil
}

func tskCheckLevelTool(l1, l2 map[string]host.Tool, name string) error {
	if _, ok := l1[name]; ok {
		return fmt.Errorf("level 1 unexpectedly listed %q", name)
	}
	if _, ok := l2[name]; !ok {
		return fmt.Errorf("level 2 did not list %q", name)
	}
	return nil
}

func tskPassMarkerCheck() string {
	if runtime.GOOS == "windows" {
		return "if not exist pass.ok exit /b 1"
	}
	return "test -f pass.ok"
}

func (st *tskScenario) givenTaskSession(remote bool) error {
	if err := st.initHost(remote, tskPassMarkerCheck()); err != nil {
		return err
	}
	wt, err := host.CreateTaskWorkspace(st.t.Context(), st.projectDir, 1, "fix-path", tskAccWorkerID, "base", remote)
	if err != nil {
		return err
	}
	st.wtDir = wt
	st.seedActiveTaskRow()
	if err := st.admitTaskCaller(); err != nil {
		return err
	}
	if remote {
		return st.installFakeGH()
	}
	return nil
}

func (st *tskScenario) seedActiveTaskRow() {
	taskRaw, _ := json.Marshal(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: tskAccTaskID, Seq: 2},
		Group:     tskAccGroupID,
		Num:       1,
		Title:     "Fix path",
		Slug:      "fix-path",
		Origin:    "brief",
		State:     "active",
		Step:      "local_check",
		Assignee:  tskAccWorkerID,
		Reviewer:  tskAccReviewID,
		Pass:      90,
		Depends:   []string{},
		Branch:    "wrong/row-branch",
		Stats:     proto.TaskStats{Started: 1_763_600_000_000},
		Created:   1_763_600_000_000,
	})
	st.h.Rows.Put(tskAccGroupID, "task", tskAccTaskID, taskRaw)
}

func (st *tskScenario) admitTaskCaller() error {
	sessKey := host.SessionKey{Group: tskAccGroupID, Member: tskAccWorkerID, Task: tskAccTaskID}
	if err := st.h.Sessions.Create(host.Session{
		SessionKey: sessKey,
		Kind:       host.TaskSession,
		Folder:     "wt/1-fix-path",
		Agent:      "claude",
		Home:       "home-dev",
		Name:       "Claude Code",
	}); err != nil {
		return err
	}
	st.caller, _ = st.h.Sessions.Caller(st.h.Sessions.Admit(sessKey, 2, false))
	return nil
}

func (st *tskScenario) installFakeGH() error {
	st.ghDir = st.t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := tskCopySelfBinary(filepath.Join(st.ghDir, name)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(st.ghDir, "calls"), nil, 0o600); err != nil {
		return err
	}
	st.setScenarioEnv("PATH", st.ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	st.setScenarioEnv("SOLODAWN_TEST_TASK_GH", st.ghDir)
	return nil
}

func tskCopySelfBinary(dst string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return m5CopyExec(exe, dst)
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

func (st *tskScenario) queueGHReplies(replies ...tskGHReply) error {
	if err := os.WriteFile(filepath.Join(st.ghDir, "calls"), nil, 0o600); err != nil {
		return err
	}
	for i, r := range replies {
		raw, _ := json.Marshal(r)
		if err := os.WriteFile(filepath.Join(st.ghDir, strconv.Itoa(i)), raw, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (st *tskScenario) commitWorktreeFiles(files map[string]string, msg string) error {
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(st.wtDir, name), []byte(content), 0o600); err != nil {
			return err
		}
		if _, err := hostForgeGit(st.wtDir, []string{"add", name}); err != nil {
			return err
		}
	}
	sha, err := hostForgeGit(st.wtDir, hostForgeCommit(msg), []string{"rev-parse", "HEAD"})
	st.head = sha
	return err
}

func (st *tskScenario) whenDirtyRunChecks() error {
	if err := os.WriteFile(filepath.Join(st.wtDir, "untracked.txt"), []byte("wip"), 0o600); err != nil {
		return err
	}
	st.callTool("run_checks", map[string]any{})
	return os.Remove(filepath.Join(st.wtDir, "untracked.txt"))
}

func (st *tskScenario) thenToolFailedWithCode(want string) error {
	e, ok := errors.AsType[*host.Error](st.lastErr)
	if !ok || e.Code != want {
		return fmt.Errorf("got %v, want code %q", st.lastErr, want)
	}
	return nil
}

func (st *tskScenario) thenNoCheckedSent() error {
	if n := len(st.home.allCmds()); n != 0 {
		return fmt.Errorf("got %d commands, want 0", n)
	}
	return nil
}

func (st *tskScenario) parseCheckOK() (bool, error) {
	if st.lastErr != nil {
		return false, st.lastErr
	}
	raw, _ := json.Marshal(st.lastOut)
	var parsed struct {
		OK bool `json:"ok"`
	}
	err := json.Unmarshal(raw, &parsed)
	return parsed.OK, err
}

func (st *tskScenario) awaitRowPush() error {
	w := newWait(5*time.Second, "a row push", nil)
	defer w.stop()
	_, err := recvWait(w, st.home.pushedCh)
	return err
}

func (st *tskScenario) whenFailingCheckThenSubmit() error {
	if err := st.commitWorktreeFiles(map[string]string{
		"app.go":      "package app\n",
		"app_test.go": "package app\n",
	}, "missing pass marker"); err != nil {
		return err
	}
	st.callTool("run_checks", map[string]any{})
	var err error
	if st.lastCheck, err = st.parseCheckOK(); err != nil {
		return err
	}
	if err := st.awaitRowPush(); err != nil {
		return err
	}
	st.callTool("submit_task", map[string]string{"summary": "Should be refused."})
	return nil
}

func (st *tskScenario) thenFailedCheckRefusesSubmit(wantCode string) error {
	if st.lastCheck {
		return errors.New("expected run_checks ok=false")
	}
	return st.thenToolFailedWithCode(wantCode)
}

func (st *tskScenario) whenPassingCheckThenSubmit(summary string) error {
	if err := st.commitWorktreeFiles(map[string]string{"pass.ok": "ok\n"}, "satisfy check"); err != nil {
		return err
	}
	st.callTool("run_checks", map[string]any{})
	var err error
	if st.lastCheck, err = st.parseCheckOK(); err != nil {
		return err
	}
	if err := st.awaitRowPush(); err != nil {
		return err
	}
	st.callTool("submit_task", map[string]string{"summary": summary})
	return st.awaitRowPush()
}

func (st *tskScenario) thenLocalSubmitSucceeded() error {
	if !st.lastCheck {
		return errors.New("expected passing run_checks")
	}
	if err := st.expectResultJSON(fmt.Sprintf(`{"ok":true,"commit":%q}`, st.head)); err != nil {
		return err
	}
	return tskVerifyLocalSubmitCmds(st.home.allCmds(), st.head)
}

func tskVerifyLocalSubmitCmds(cmds []proto.Cmd, head string) error {
	if len(cmds) != 4 {
		return fmt.Errorf("got %d cmds, want 4", len(cmds))
	}
	var report proto.Post[proto.ReportBody]
	if err := proto.Decode([]byte(cmds[2].Env.Payload), &report); err != nil {
		return err
	}
	var sub proto.TaskSubmitted
	if err := proto.Decode([]byte(cmds[3].Env.Payload), &sub); err != nil {
		return err
	}
	return tskMatchLocalReportAndSubmit(report, sub, head)
}

func tskMatchLocalReportAndSubmit(report proto.Post[proto.ReportBody], sub proto.TaskSubmitted, head string) error {
	if report.Body.Commit != head || report.Body.PR != nil {
		return fmt.Errorf("unexpected local report: %+v", report.Body)
	}
	if sub.Commit != head || sub.PR != 0 {
		return fmt.Errorf("unexpected local submitted: %+v", sub)
	}
	return nil
}

func (st *tskScenario) whenCommitAfterCheckThenSubmit() error {
	if err := st.commitWorktreeFiles(map[string]string{"extra.txt": "extra\n"}, "unchecked commit"); err != nil {
		return err
	}
	st.callTool("submit_task", map[string]string{"summary": "Unchecked."})
	return nil
}

func (st *tskScenario) whenCommitAndPushBranch() error {
	if err := st.commitWorktreeFiles(map[string]string{
		"pass.ok":     "ok\n",
		"app.go":      "package app\n",
		"app_test.go": "package app\n",
	}, "remote change"); err != nil {
		return err
	}
	st.callTool("push_branch", map[string]any{})
	return nil
}

func (st *tskScenario) thenPushBranchSucceeded() error {
	if err := st.expectResultJSON(fmt.Sprintf(`{"ok":true,"commit":%q}`, st.head)); err != nil {
		return err
	}
	refs, err := hostForgeGit(st.remoteDir, []string{"for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task"})
	if err != nil || refs != "refs/heads/task/1-fix-path "+st.head {
		return fmt.Errorf("remote refs = %q (%v)", refs, err)
	}
	return nil
}

func (st *tskScenario) whenRemoteChecksAndSubmit(summary string) error {
	st.callTool("run_checks", map[string]any{})
	if st.lastErr != nil {
		return st.lastErr
	}
	if err := st.awaitRowPush(); err != nil {
		return err
	}
	prURL := "https://example.invalid/team/website/pull/31"
	rollup := fmt.Sprintf(`{"headRefOid":%q,"statusCheckRollup":[{"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"SUCCESS"}]}`, st.head)
	if err := st.queueGHReplies(
		tskGHReply{Stdout: "[]\n"},
		tskGHReply{Stdout: prURL + "\n"},
		tskGHReply{Stdout: rollup},
	); err != nil {
		return err
	}
	st.callTool("submit_task", map[string]string{"summary": summary})
	if st.lastErr != nil {
		return st.lastErr
	}
	return st.awaitRowPush()
}

func (st *tskScenario) thenRemoteSubmitAndCloudSucceeded() error {
	if err := st.expectResultJSON(fmt.Sprintf(`{"ok":true,"commit":%q,"pr":31}`, st.head)); err != nil {
		return err
	}
	w := newWait(10*time.Second, "the task cloud command", nil)
	defer w.stop()
	cloud, err := recvWait(w, st.home.cloudCh)
	if err != nil {
		return err
	}
	return tskCheckCloudResult(cloud, st.head)
}

func tskCheckCloudResult(cloud proto.TaskChecked, head string) error {
	if !cloud.OK || cloud.Commit != head {
		return fmt.Errorf("unexpected task cloud outcome: %+v", cloud)
	}
	if len(cloud.Items) != 1 || cloud.Items[0].Name != "ci" {
		return fmt.Errorf("unexpected task cloud items: %+v", cloud.Items)
	}
	return nil
}

func (st *tskScenario) whenReviewerSubmits(score int) error {
	if _, err := host.PrepareReviewWorkspace(st.t.Context(), st.projectDir, 1, tskAccReviewID, st.head, "base", true); err != nil {
		return err
	}
	revKey := host.SessionKey{Group: tskAccGroupID, Member: tskAccReviewID, Task: tskAccTaskID}
	if err := st.h.Sessions.Create(host.Session{
		SessionKey: revKey,
		Kind:       host.ReviewSession,
		Folder:     "review/1",
		Agent:      "codex",
		Home:       "home-dev",
		Name:       "Codex",
	}); err != nil {
		return err
	}
	st.caller, _ = st.h.Sessions.Caller(st.h.Sessions.Admit(revKey, 1, false))
	st.callTool("submit_review", map[string]any{
		"score": score,
		"items": []map[string]any{{"text": "Solid test coverage.", "file": "app_test.go", "line": 1}},
	})
	return nil
}

func (st *tskScenario) thenReviewSucceeded() error {
	if err := st.expectResultJSON(`{"passed":true,"round":1}`); err != nil {
		return err
	}
	cmds := st.home.allCmds()
	return tskVerifyReviewPayload(cmds[len(cmds)-1].Env.Payload, st.head)
}

func tskVerifyReviewPayload(payload, head string) error {
	var rev proto.Post[proto.ReviewSigned]
	if err := proto.Decode([]byte(payload), &rev); err != nil {
		return err
	}
	if rev.Body.Commit != head || rev.Body.Score != 94 || string(rev.As) != tskAccReviewID {
		return fmt.Errorf("unexpected review post: %+v", rev)
	}
	return nil
}
