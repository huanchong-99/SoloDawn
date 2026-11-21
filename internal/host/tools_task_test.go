package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	tskTestGroupID  = "rZNvy-1jH6Z-BcPqA5U5BQ"
	tskTestTaskID   = "DrtCn6htSBwmMPrFPbHJHA"
	tskTestWorkerID = "1PC8WineBrUQ-apCjx7tug"
	tskTestReviewID = "LXCZmuGAXkvO-bSrOkuCfw"
)

type tskReplyStep struct {
	answer string
	err    error
}

type tskScriptHome struct {
	mu sync.Mutex
	// Single reader: Host.Home via Done.
	done  chan struct{}
	steps []tskReplyStep
	asked []any
	// Single reader: waitAsked in TestSubmitTaskRemoteAndHostedChecks.
	askedCh chan struct{}
}

func newTskScriptHome(steps ...tskReplyStep) *tskScriptHome {
	return &tskScriptHome{done: make(chan struct{}), steps: steps, askedCh: make(chan struct{}, 16)}
}

func (f *tskScriptHome) Send(any) error               { return nil }
func (f *tskScriptHome) Path() string                 { return "local" }
func (f *tskScriptHome) PathChanged() <-chan struct{} { return nil }
func (f *tskScriptHome) Done() <-chan struct{}        { return f.done }

func (f *tskScriptHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.asked) + 1)
	req := build(id)
	f.asked = append(f.asked, req)
	step := f.popStepLocked(id)
	f.mu.Unlock()
	select {
	case f.askedCh <- struct{}{}:
	default:
	}
	return json.RawMessage(step.answer), step.err
}

func (f *tskScriptHome) popStepLocked(id int64) tskReplyStep {
	if len(f.steps) == 0 {
		return tskReplyStep{answer: fmt.Sprintf(`{"re":%d,"ok":true}`, id)}
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step
}

func (f *tskScriptHome) snapshotAsked() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

func (f *tskScriptHome) waitAsked(t *testing.T, want int) []any {
	t.Helper()
	for {
		if got := f.snapshotAsked(); len(got) >= want {
			return got
		}
		select {
		case <-f.askedCh:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %d requests", want)
		}
	}
}

func tskEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func tskRequire(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}

func tskNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func tskStopAllCloud(t *testing.T, h *Host) {
	t.Helper()
	t.Cleanup(func() {
		p := Part[tskPart](h)
		p.mu.Lock()
		for _, run := range p.cancels {
			run.cancel()
		}
		p.mu.Unlock()
	})
}

func tskDiskHost(t *testing.T) (*Host, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "node.db"))
	tskNoErr(t, err)
	t.Cleanup(func() { db.Close() })
	h, _ := seamHost(t, db)
	tskStopAllCloud(t, h)
	return h, dataDir
}

func tskPutRow(t *testing.T, h *Host, kind, id string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	tskNoErr(t, err)
	h.Rows.Put(tskTestGroupID, kind, id, raw)
}

func tskGroupFixture(forge string, checks ...string) proto.GroupRow {
	if checks == nil {
		checks = []string{}
	}
	return proto.GroupRow{
		RowHeader:      proto.RowHeader{Type: "group", ID: tskTestGroupID, Seq: 1},
		Name:           "Website",
		Kind:           "project",
		Base:           "base",
		Forge:          forge,
		Home:           "home-device",
		Checks:         checks,
		Pass:           90,
		PassFoundation: 70,
		Limits:         proto.Limits{TurnMinutes: 1},
		Created:        seamStart,
	}
}

func tskTaskFixture(num int, slug proto.Slug, branch string, local *proto.Check) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: tskTestTaskID, Seq: 2},
		Group:     tskTestGroupID,
		Num:       num,
		Title:     "Fix path check",
		Slug:      slug,
		Origin:    "brief",
		State:     "active",
		Step:      "local_check",
		Assignee:  tskTestWorkerID,
		Reviewer:  tskTestReviewID,
		Pass:      90,
		Depends:   []string{},
		Branch:    branch,
		Checks:    proto.TaskChecks{Local: local},
		Stats:     proto.TaskStats{Started: seamStart},
		Created:   seamStart,
	}
}

func tskOpenSignedCmd[T any](t *testing.T, h *Host, raw any) T {
	t.Helper()
	cmd, ok := raw.(proto.Cmd)
	tskRequire(t, ok, "expected proto.Cmd")
	known := func(dev string) bool { return dev == h.Key.ID() }
	_, err := identity.OpenCommand(cmd.Env, "", known, h.Now())
	tskNoErr(t, err)
	var decoded T
	tskNoErr(t, proto.Decode([]byte(cmd.Env.Payload), &decoded))
	return decoded
}

func tskInitWorktree(t *testing.T, dataDir string, remote bool) (projectDir, remoteDir, wtDir, baseSHA string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("LC_ALL", "C")
	src := t.TempDir()
	forgeGit(t, src, "init", "-b", "base")
	tskNoErr(t, os.WriteFile(filepath.Join(src, "readme.md"), []byte("line1\nline2\n"), 0o600))
	forgeGit(t, src, "add", "readme.md")
	forgeGit(t, src, "-c", "user.name=test", "-c", "user.email=test@invalid", "commit", "-m", "base")
	baseSHA = forgeGit(t, src, "rev-parse", "HEAD")
	source := src
	if remote {
		remoteDir = t.TempDir()
		forgeGit(t, remoteDir, "init", "--bare", "-b", "base")
		forgeGit(t, src, "remote", "add", "origin", remoteDir)
		forgeGit(t, src, "push", "origin", "base")
		source = remoteDir
	}
	projectDir = filepath.Join(dataDir, "projects", tskTestGroupID)
	_, err := EnsureGroupWorkspace(t.Context(), projectDir, source, "base")
	tskNoErr(t, err)
	wtDir, err = CreateTaskWorkspace(t.Context(), projectDir, 12, "fix-path", tskTestWorkerID, "base", remote)
	tskNoErr(t, err)
	return projectDir, remoteDir, wtDir, baseSHA
}

func tskCommitFiles(t *testing.T, dir string, files map[string]string, message string) string {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(dir, name)
		tskNoErr(t, os.MkdirAll(filepath.Dir(full), 0o700))
		tskNoErr(t, os.WriteFile(full, []byte(content), 0o600))
		forgeGit(t, dir, "add", name)
	}
	forgeGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@invalid", "commit", "-m", message)
	return forgeGit(t, dir, "rev-parse", "HEAD")
}

func tskTaskCaller(folder string, level int) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: tskTestGroupID, Member: tskTestWorkerID, Task: tskTestTaskID},
			Kind:       TaskSession,
			Folder:     folder,
			Home:       "home-device",
		},
		Level: level,
	}
}

func tskListedOwned(listed map[string]Tool, owned []string) []string {
	var got []string
	for _, name := range owned {
		if _, ok := listed[name]; ok {
			got = append(got, name)
		}
	}
	slices.Sort(got)
	return got
}

func tskAssertGoldenSubset(t *testing.T, listed map[string]Tool, golden proto.ToolList, owned []string) {
	t.Helper()
	var want []string
	for _, tool := range golden.Tools {
		if slices.Contains(owned, tool.Name) {
			want = append(want, tool.Name)
			tskAssertToolSchema(t, listed[tool.Name], tool.InputSchema)
		}
	}
	slices.Sort(want)
	tskEqual(t, tskListedOwned(listed, owned), want)
}

func tskAssertToolSchema(t *testing.T, got Tool, wantInput proto.ToolInput) {
	t.Helper()
	tskRequire(t, got.Description != "", "empty tool description")
	var gotVal, wantVal any
	tskNoErr(t, json.Unmarshal(got.Schema, &gotVal))
	rawWant, err := json.Marshal(wantInput)
	tskNoErr(t, err)
	tskNoErr(t, json.Unmarshal(rawWant, &wantVal))
	tskEqual(t, gotVal, wantVal)
}

func TestTaskToolsMatchContractLists(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "proto", "testdata", "tools.json"))
	tskNoErr(t, err)
	var lists proto.ToolLists
	tskNoErr(t, json.Unmarshal(data, &lists))
	owned := []string{"start_task", "push_branch", "run_checks", "submit_task", "submit_review"}
	cases := []struct {
		caller Caller
		golden proto.ToolList
	}{
		{Caller{Session: Session{Kind: GroupSession, Secretary: true}, Level: 1}, lists.Secretary},
		{Caller{Session: Session{Kind: GroupSession, Secretary: false}, Level: 1}, lists.Group},
		{Caller{Session: Session{Kind: ReviewSession}, Level: 1}, lists.Review},
		{Caller{Session: Session{Kind: TaskSession}, Level: 1}, lists.TaskLevel1},
		{Caller{Session: Session{Kind: TaskSession}, Level: 2}, lists.TaskLevel23},
		{Caller{Session: Session{Kind: TaskSession}, Level: 3}, lists.TaskLevel23},
	}
	for _, tc := range cases {
		tskAssertGoldenSubset(t, ToolsFor(tc.caller), tc.golden, owned)
	}
}

func TestTaskToolForPredicates(t *testing.T) {
	tskEqual(t, tskForWorkingGroup(Caller{Session: Session{Kind: GroupSession, Secretary: false}}), true)
	tskEqual(t, tskForWorkingGroup(Caller{Session: Session{Kind: GroupSession, Secretary: true}}), false)
	tskEqual(t, tskForWorkingGroup(Caller{Session: Session{Kind: TaskSession, Secretary: false}}), false)
	tskEqual(t, tskForTaskAboveLevel1(Caller{Session: Session{Kind: TaskSession}, Level: 2}), true)
	tskEqual(t, tskForTaskAboveLevel1(Caller{Session: Session{Kind: TaskSession}, Level: 1}), false)
	tskEqual(t, tskForTaskAboveLevel1(Caller{Session: Session{Kind: TaskSession}, Level: 0}), false)
	tskEqual(t, tskForTaskAboveLevel1(Caller{Session: Session{Kind: ReviewSession}, Level: 2}), false)
	tskEqual(t, tskForReview(Caller{Session: Session{Kind: ReviewSession}}), true)
	tskEqual(t, tskForReview(Caller{Session: Session{Kind: TaskSession}}), false)
}

func tskGroupCaller(turn Turn) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: tskTestGroupID, Member: tskTestWorkerID},
			Kind:       GroupSession,
			Folder:     "repo",
			Home:       "home-device",
		},
		Level: 1,
		Turn:  turn,
	}
}

func TestStartTaskWithoutTurn(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	tskStopAllCloud(t, h)
	home := newTskScriptHome(tskReplyStep{answer: `{"re":1,"ok":true,"num":14}`})
	h.Attach("home-device", home)
	res, err := Tools["start_task"].Call(t.Context(), h, tskGroupCaller(Turn{Kind: ItemsTurn}), json.RawMessage(`{"title":"Add badge","plan":"Update TaskCard."}`))
	tskNoErr(t, err)
	tskEqual(t, res, tskStartResult{Task: "T-14", Moved: true})
	asked := home.snapshotAsked()
	tskEqual(t, len(asked), 1)
	tskRequire(t, !strings.HasSuffix(asked[0].(proto.Cmd).Env.Payload, "\n"), "payload has trailing newline")
	started := tskOpenSignedCmd[proto.TaskStart](t, h, asked[0])
	tskEqual(t, started.Op, "start")
	tskEqual(t, string(started.As), tskTestWorkerID)
	tskEqual(t, started.Origin, "direct")
	tskEqual(t, started.Re, (*proto.ID)(nil))
	tskEqual(t, started.Title, "Add badge")
	tskEqual(t, started.Plan, "Update TaskCard.")
	tskRequire(t, started.ID != "" && started.ID != started.Nonce, "expected fresh task id distinct from nonce")
}

func TestStartTaskErrorsAndBoundaries(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	tskStopAllCloud(t, h)
	call := Tools["start_task"].Call
	caller := tskGroupCaller(Turn{Kind: ItemsTurn})

	_, err := call(t.Context(), h, caller, json.RawMessage(`{"title":"T","plan":"P"}`))
	seamCode(t, err, "home_unreachable")

	home := newTskScriptHome()
	h.Attach("home-device", home)

	for _, bad := range []string{
		`[]`,
		`{"title":"only"}`,
		`{"title":1,"plan":"c"}`,
	} {
		_, err := call(t.Context(), h, caller, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	const secretKey = "-----BEGIN " + "PRIVATE KEY-----"
	for _, secretArg := range []string{
		fmt.Sprintf(`{"title":%q,"plan":"clean"}`, secretKey),
		fmt.Sprintf(`{"title":"clean","plan":%q}`, secretKey),
	} {
		_, err := call(t.Context(), h, caller, json.RawMessage(secretArg))
		seamCode(t, err, "secret")
	}
	tskEqual(t, len(home.snapshotAsked()), 0)
}

func TestPushBranchRefusals(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	tskInitWorktree(t, dataDir, true)
	call := Tools["push_branch"].Call
	caller := tskTaskCaller("wt/12-fix-path", 2)

	_, err := call(t.Context(), h, caller, json.RawMessage(`[]`))
	seamCode(t, err, "bad_request")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "not_found")

	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none"))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "conflict")

	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("github"))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "not_found")

	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(1, "fix-path", "refs/heads/main", nil))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "cli")
}

func TestPushBranchSuccessAndSecretRedaction(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, remoteDir, wtDir, _ := tskInitWorktree(t, dataDir, true)
	head := tskCommitFiles(t, wtDir, map[string]string{"a.go": "package a\n"}, "work")
	call := Tools["push_branch"].Call
	caller := tskTaskCaller("wt/12-fix-path", 2)

	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("github"))
	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "wrong/ignored-branch", nil))

	res, err := call(t.Context(), h, caller, json.RawMessage(`{}`))
	tskNoErr(t, err)
	tskEqual(t, res, tskPushResult{OK: true, Commit: head})
	gotRefs := forgeGit(t, remoteDir, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task")
	tskEqual(t, gotRefs, "refs/heads/task/12-fix-path "+head)

	secretToken := "ghp_" + strings.Repeat("A", 36)
	forgeGit(t, wtDir, "remote", "set-url", "origin", filepath.Join(t.TempDir(), secretToken+".git"))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "cli")
	tskRequire(t, !strings.Contains(err.Error(), secretToken) && strings.Contains(err.Error(), "[redacted]"), "expected redacted cli error")
}

func TestRunChecksRefusals(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, _, wtDir, _ := tskInitWorktree(t, dataDir, false)
	home := newTskScriptHome()
	h.Attach("home-device", home)
	call := Tools["run_checks"].Call
	caller := tskTaskCaller("wt/12-fix-path", 2)

	_, err := call(t.Context(), h, caller, json.RawMessage(`[]`))
	seamCode(t, err, "bad_request")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "not_found")
	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none", "echo checked"))
	_, err = call(t.Context(), h, tskTaskCaller("wt/99-absent", 2), json.RawMessage(`{}`))
	seamCode(t, err, "cli")

	tskNoErr(t, os.WriteFile(filepath.Join(wtDir, "dirty.txt"), []byte("wip"), 0o600))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "conflict")
	tskRequire(t, !strings.HasPrefix(err.Error(), "conflict: conflict:"), "duplicate conflict prefix")
	os.Remove(filepath.Join(wtDir, "dirty.txt"))

	forgeGit(t, wtDir, "update-index", "--skip-worktree", "readme.md")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "conflict")
	tskRequire(t, strings.Contains(err.Error(), "readme.md"), "expected skip-worktree path in error")
	forgeGit(t, wtDir, "update-index", "--no-skip-worktree", "readme.md")
	tskEqual(t, len(home.snapshotAsked()), 0)
}

func TestRunChecksSuccessAndUnreachable(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, _, wtDir, _ := tskInitWorktree(t, dataDir, false)
	home := newTskScriptHome()
	h.Attach("home-device", home)
	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none", "echo checked"))
	call := Tools["run_checks"].Call
	caller := tskTaskCaller("wt/12-fix-path", 2)

	head := tskCommitFiles(t, wtDir, map[string]string{
		"pkg.go":      "package p\n",
		"pkg_test.go": "package p\n",
	}, "add pkg and test")
	res, err := call(t.Context(), h, caller, json.RawMessage(`{}`))
	tskNoErr(t, err)
	gotRes := res.(tskChecksResult)
	tskEqual(t, gotRes.OK, true)
	tskEqual(t, gotRes.Commit, head)
	tskEqual(t, len(gotRes.Items), 3)

	checked := tskOpenSignedCmd[proto.TaskChecked](t, h, home.snapshotAsked()[0])
	tskEqual(t, string(checked.As), "")
	tskEqual(t, checked.Op, "checked")
	tskEqual(t, string(checked.ID), tskTestTaskID)
	tskEqual(t, checked.OK, true)
	tskEqual(t, checked.Commit, head)

	close(home.done)
	_, err = call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "home_unreachable")

	// Remote group compares against origin/<base> rather than local <base>.
	rh, rDataDir := tskDiskHost(t)
	rProjectDir, _, rWtDir, rBaseSHA := tskInitWorktree(t, rDataDir, true)
	rHome := newTskScriptHome()
	rh.Attach("home-device", rHome)
	tskPutRow(t, rh, "group", tskTestGroupID, tskGroupFixture("github"))
	tskCommitFiles(t, rWtDir, map[string]string{"legacy.go": "package p\n"}, "advance origin base")
	forgeGit(t, rWtDir, "push", "origin", "HEAD:base")
	forgeGit(t, filepath.Join(rProjectDir, "repo"), "fetch", "origin", "base")
	forgeGit(t, filepath.Join(rProjectDir, "repo"), "update-ref", "refs/heads/base", rBaseSHA)
	rHead := tskCommitFiles(t, rWtDir, map[string]string{"only_test.go": "package p\n"}, "tested relative to origin/base")
	rRes, err := call(t.Context(), rh, caller, json.RawMessage(`{}`))
	tskNoErr(t, err)
	tskEqual(t, rRes.(tskChecksResult).OK, true)
	tskEqual(t, rRes.(tskChecksResult).Commit, rHead)
}

func TestSubmitTaskPreconditions(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, _, wtDir, _ := tskInitWorktree(t, dataDir, false)
	home := newTskScriptHome()
	h.Attach("home-device", home)
	call := Tools["submit_task"].Call
	caller := tskTaskCaller("wt/12-fix-path", 2)

	_, err := call(t.Context(), h, caller, json.RawMessage(`{}`))
	seamCode(t, err, "bad_request")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"}`))
	seamCode(t, err, "secret")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "not_found")

	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none"))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "not_found")
	head := tskCommitFiles(t, wtDir, map[string]string{"f_test.go": "package p\n"}, "add test")

	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "ignored", nil))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "conflict")

	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "ignored", &proto.Check{OK: false, At: seamStart, Commit: head}))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "conflict")

	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "ignored", &proto.Check{OK: true, At: seamStart, Commit: head}))
	tskNoErr(t, os.WriteFile(filepath.Join(wtDir, "untracked"), []byte("x"), 0o600))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "conflict")
	os.Remove(filepath.Join(wtDir, "untracked"))

	// Detached HEAD at a newer commit while refs/heads/task/12-fix-path stays at head.
	forgeGit(t, wtDir, "checkout", "--detach", head)
	detachedNew := tskCommitFiles(t, wtDir, map[string]string{"det.txt": "1\n"}, "detached")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "conflict")

	// Detached HEAD at checked commit while refs/heads/task/12-fix-path moved to detachedNew.
	forgeGit(t, wtDir, "checkout", "--detach", head)
	forgeGit(t, wtDir, "update-ref", "refs/heads/task/12-fix-path", detachedNew)
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Done."}`))
	seamCode(t, err, "conflict")
	tskEqual(t, len(home.snapshotAsked()), 0)
}

func TestSubmitTaskLocalSuccessAndAddedCommit(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, _, wtDir, _ := tskInitWorktree(t, dataDir, false)
	home := newTskScriptHome()
	h.Attach("home-device", home)
	call := Tools["submit_task"].Call
	caller := tskTaskCaller("wt/12-fix-path", 2)

	head := tskCommitFiles(t, wtDir, map[string]string{
		"readme.md": "line1\nupdated\nextra\n",
		"f_test.go": "package p\n",
	}, "implement feature")
	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none"))
	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "ignored", &proto.Check{OK: true, At: seamStart, Commit: head}))

	res, err := call(t.Context(), h, caller, json.RawMessage(`{"summary":"Implemented feature."}`))
	tskNoErr(t, err)
	tskEqual(t, res, tskSubmitResult{OK: true, Commit: head})
	rawJSON, _ := json.Marshal(res)
	tskRequire(t, !strings.Contains(string(rawJSON), `"pr"`), "local submit must omit pr")

	asked := home.snapshotAsked()
	tskEqual(t, len(asked), 2)
	report := tskOpenSignedCmd[proto.Post[proto.ReportBody]](t, h, asked[0])
	tskEqual(t, report.Kind, "report")
	tskEqual(t, string(report.As), tskTestWorkerID)
	wantTaskID := proto.ID(tskTestTaskID)
	tskEqual(t, report.Thread, &wantTaskID)
	tskEqual(t, report.Main, false)
	tskEqual(t, report.Text, "Implemented feature.")
	tskEqual(t, report.Body, proto.ReportBody{Commit: head, Files: 2, Added: 3, Removed: 1})

	submitted := tskOpenSignedCmd[proto.TaskSubmitted](t, h, asked[1])
	tskEqual(t, string(submitted.As), "")
	tskEqual(t, submitted.Op, "submitted")
	tskEqual(t, string(submitted.ID), tskTestTaskID)
	tskEqual(t, submitted.Commit, head)
	tskEqual(t, submitted.PR, 0)
	tskEqual(t, submitted.PRURL, "")
	tskEqual(t, submitted.Stats, proto.DiffStats{Added: 3, Removed: 1})

	tskCommitFiles(t, wtDir, map[string]string{"later.txt": "more\n"}, "after checks")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"Again."}`))
	seamCode(t, err, "conflict")
}

func TestSubmitTaskErrorPaths(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, _, _, baseSHA := tskInitWorktree(t, dataDir, false)
	call := Tools["submit_task"].Call
	caller := tskTaskCaller("wt/12-fix-path", 3)

	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none"))
	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(99, "other", "", &proto.Check{OK: true, At: seamStart, Commit: baseSHA}))
	_, err := call(t.Context(), h, caller, json.RawMessage(`{"summary":"s"}`))
	seamCode(t, err, "cli")

	badBase := tskGroupFixture("none")
	badBase.Base = "nonexistent-base"
	tskPutRow(t, h, "group", tskTestGroupID, badBase)
	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "", &proto.Check{OK: true, At: seamStart, Commit: baseSHA}))
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"s"}`))
	seamCode(t, err, "cli")

	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("none"))
	home := newTskScriptHome(
		tskReplyStep{err: os.ErrClosed},
		tskReplyStep{answer: `{"re":2,"ok":true}`},
		tskReplyStep{err: os.ErrClosed},
	)
	h.Attach("home-device", home)
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"empty diff"}`))
	seamCode(t, err, "home_unreachable")
	_, err = call(t.Context(), h, caller, json.RawMessage(`{"summary":"empty diff"}`))
	seamCode(t, err, "home_unreachable")

	_, err = call(t.Context(), h, tskTaskCaller("wt/99-missing", 3), json.RawMessage(`{"summary":"s"}`))
	seamCode(t, err, "cli")
}

func TestSubmitTaskRemoteAndHostedChecks(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	_, remoteDir, wtDir, _ := tskInitWorktree(t, dataDir, true)
	head := tskCommitFiles(t, wtDir, map[string]string{"feature_test.go": "package p\n"}, "add test")
	nowMS := time.Now().UnixMilli()
	tskPutRow(t, h, "group", tskTestGroupID, tskGroupFixture("github"))
	tskPutRow(t, h, "task", tskTestTaskID, tskTaskFixture(12, "fix-path", "wrong/branch", &proto.Check{OK: true, At: nowMS, Commit: head}))

	caller := tskTaskCaller("wt/12-fix-path", 2)
	call := Tools["submit_task"].Call
	home := newTskScriptHome()
	h.Attach("home-device", home)

	fakeForgeGH(t, forgeAnswer{Stderr: "gh auth required\n", Code: 1})
	_, err := call(t.Context(), h, caller, json.RawMessage(`{"summary":"PR body"}`))
	seamCode(t, err, "cli")
	tskEqual(t, len(home.snapshotAsked()), 0)

	prURL := "https://example.invalid/org/repo/pull/31"
	fakeForgeGH(t,
		forgeAnswer{Stdout: "[]\n"},
		forgeAnswer{Stdout: prURL + "\n"},
		hostedView(head, passedRun),
	)
	res, err := call(t.Context(), h, caller, json.RawMessage(`{"summary":"PR body"}`))
	tskNoErr(t, err)
	tskEqual(t, res, tskSubmitResult{OK: true, Commit: head, PR: 31})

	asked := home.waitAsked(t, 3)
	gotRefs := forgeGit(t, remoteDir, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task")
	tskEqual(t, gotRefs, "refs/heads/task/12-fix-path "+head)

	tskEqual(t, len(asked), 3)
	report := tskOpenSignedCmd[proto.Post[proto.ReportBody]](t, h, asked[0])
	wantPR := 31
	tskEqual(t, report.Body, proto.ReportBody{Commit: head, Files: 1, Added: 1, Removed: 0, PR: &wantPR})

	submitted := tskOpenSignedCmd[proto.TaskSubmitted](t, h, asked[1])
	tskEqual(t, submitted.PR, 31)
	tskEqual(t, submitted.PRURL, prURL)

	cloud := tskOpenSignedCmd[proto.TaskChecked](t, h, asked[2])
	tskEqual(t, string(cloud.As), "")
	tskEqual(t, cloud.Op, "cloud")
	tskEqual(t, cloud.OK, true)
	tskEqual(t, cloud.Commit, head)
	tskEqual(t, len(cloud.Items), 1)
	tskEqual(t, cloud.Items[0].Name, "build")
}

func TestSubmitTaskSupersedesPreviousHostedChecks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &tskPart{}
		key := tskCloudKey{group: tskTestGroupID, task: tskTestTaskID}
		ctx1, run1 := tskBeginCloud(p, key, "c1")
		dupCtx, dupRun := tskBeginCloud(p, key, "c1")
		tskEqual(t, dupCtx, context.Context(nil))
		tskEqual(t, dupRun, (*tskCloudRun)(nil))
		tskRequire(t, ctx1.Err() == nil, "same-commit claim must leave running check alone")

		ctx2, run2 := tskBeginCloud(p, key, "c2")
		tskRequire(t, ctx1.Err() != nil, "first cloud context must be cancelled")
		tskRequire(t, ctx2.Err() == nil, "second cloud context must stay active")

		tskFinishCloud(p, key, run1)
		tskEqual(t, p.cancels[key] == run2, true)

		tskFinishCloud(p, key, run2)
		tskEqual(t, len(p.cancels), 0)
	})
}

func TestSubmitTaskCancelledHostedCheckDoesNotReportCloud(t *testing.T) {
	h, _ := seamHost(t, seamStore(t))
	home := newTskScriptHome()
	h.Attach("home-device", home)
	p := Part[tskPart](h)
	key := tskCloudKey{group: tskTestGroupID, task: tskTestTaskID}
	ctx, run := tskBeginCloud(p, key, "c1")
	run.cancel()
	fakeForgeGH(t, hostedView("c1", queuedRun))
	tskPollHostedChecks(ctx, p, h, "home-device", key, run, TaskForge{Worktree: t.TempDir()}, 7, "c1", time.Now())
	tskEqual(t, len(home.snapshotAsked()), 0)
}

func tskReviewCaller(folder string) Caller {
	return Caller{
		Session: Session{
			SessionKey: SessionKey{Group: tskTestGroupID, Member: tskTestReviewID, Task: tskTestTaskID},
			Kind:       ReviewSession,
			Folder:     folder,
			Home:       "home-device",
		},
		Level: 1,
	}
}

func TestSubmitReviewScoresAndItems(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	projectDir, _, wtDir, _ := tskInitWorktree(t, dataDir, false)
	head := tskCommitFiles(t, wtDir, map[string]string{"x.go": "package x\n"}, "change")
	_, err := PrepareReviewWorkspace(t.Context(), projectDir, 12, tskTestReviewID, head, "base", false)
	tskNoErr(t, err)
	home := newTskScriptHome(
		tskReplyStep{answer: `{"re":1,"ok":true,"passed":false,"round":1}`},
		tskReplyStep{answer: `{"re":2,"ok":true,"passed":true,"round":2}`},
	)
	h.Attach("home-device", home)
	call := Tools["submit_review"].Call
	caller := tskReviewCaller("review/12")

	res, err := call(t.Context(), h, caller, json.RawMessage(`{"score":0,"items":[{"text":"Missing test"},{"text":"Check line","file":"x.go","line":1,"veto":true}]}`))
	tskNoErr(t, err)
	tskEqual(t, res, tskReviewResult{Passed: false, Round: 1})

	rev0 := tskOpenSignedCmd[proto.Post[proto.ReviewSigned]](t, h, home.snapshotAsked()[0])
	tskEqual(t, rev0.Kind, "review")
	tskEqual(t, string(rev0.As), tskTestReviewID)
	wantTaskID := proto.ID(tskTestTaskID)
	tskEqual(t, rev0.Thread, &wantTaskID)
	tskEqual(t, rev0.Main, false)
	tskEqual(t, rev0.Text, "")
	wantFile := "x.go"
	wantLine := 1
	tskEqual(t, rev0.Body, proto.ReviewSigned{
		Task:   wantTaskID,
		Commit: head,
		Score:  0,
		Items: []proto.ReviewItem{
			{Text: "Missing test", Veto: false},
			{Text: "Check line", Veto: true, File: &wantFile, Line: &wantLine},
		},
	})

	res, err = call(t.Context(), h, caller, json.RawMessage(`{"score":100,"items":[{"text":"Clean","veto":false}]}`))
	tskNoErr(t, err)
	tskEqual(t, res, tskReviewResult{Passed: true, Round: 2})
	rev100 := tskOpenSignedCmd[proto.Post[proto.ReviewSigned]](t, h, home.snapshotAsked()[1])
	tskEqual(t, rev100.Body.Score, 100)
	tskEqual(t, rev100.Body.Items[0].Veto, false)
}

func TestSubmitReviewRefusalsAndSecrets(t *testing.T) {
	h, dataDir := tskDiskHost(t)
	projectDir, _, wtDir, _ := tskInitWorktree(t, dataDir, false)
	head := tskCommitFiles(t, wtDir, map[string]string{"x.go": "package x\n"}, "change")
	_, err := PrepareReviewWorkspace(t.Context(), projectDir, 12, tskTestReviewID, head, "base", false)
	tskNoErr(t, err)
	home := newTskScriptHome(tskReplyStep{err: os.ErrClosed})
	h.Attach("home-device", home)
	call := Tools["submit_review"].Call
	caller := tskReviewCaller("review/12")

	for _, bad := range []string{
		`[]`,
		`{"score":50,"items":[123]}`,
	} {
		_, err := call(t.Context(), h, caller, json.RawMessage(bad))
		seamCode(t, err, "bad_request")
	}

	secretTok := "AKIA" + strings.Repeat("B", 16)
	_, err = call(t.Context(), h, caller, json.RawMessage(fmt.Sprintf(`{"score":80,"items":[{"text":%q}]}`, secretTok)))
	seamCode(t, err, "secret")
	_, err = call(t.Context(), h, caller, json.RawMessage(fmt.Sprintf(`{"score":80,"items":[{"text":"clean","file":%q}]}`, secretTok)))
	seamCode(t, err, "secret")
	tskEqual(t, len(home.snapshotAsked()), 0)

	_, err = call(t.Context(), h, tskReviewCaller("review/99"), json.RawMessage(`{"score":90,"items":[]}`))
	seamCode(t, err, "cli")

	_, err = call(t.Context(), h, caller, json.RawMessage(`{"score":90,"items":[]}`))
	seamCode(t, err, "home_unreachable")
}
