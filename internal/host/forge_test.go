package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// The test binary doubles as a portable gh. Each invocation appends its
// complete argv to the log and consumes the corresponding numbered answer.
func init() {
	if dir := os.Getenv("SOLODAWN_TEST_FORGE"); dir != "" && strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") == "gh" {
		os.Exit(replayForgeAnswer(dir))
	}
}

func mustForge[T any](value T, err error) T {
	forgeOK(err)
	return value
}

func forgeOK(err error) {
	if err != nil {
		panic(err)
	}
}

type forgeAnswer struct {
	Stdout, Stderr string
	Code           int
}

func replayForgeAnswer(dir string) int {
	log := filepath.Join(dir, "calls")
	previous := mustForge(os.ReadFile(log))
	file := mustForge(os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0600))
	forgeOK(json.NewEncoder(file).Encode(os.Args[1:]))
	forgeOK(file.Close())
	n := bytes.Count(previous, []byte{'\n'})
	data := mustForge(os.ReadFile(filepath.Join(dir, strconv.Itoa(n))))
	var answer forgeAnswer
	forgeOK(json.Unmarshal(data, &answer))
	fmt.Fprint(os.Stdout, answer.Stdout)
	fmt.Fprint(os.Stderr, answer.Stderr)
	return answer.Code
}

func fakeForgeGH(t *testing.T, answers ...forgeAnswer) func(...[]string) {
	t.Helper()
	dir := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := mustForge(os.Executable())
	forgeOK(os.WriteFile(filepath.Join(dir, name), mustForge(os.ReadFile(exe)), 0700))
	log := filepath.Join(dir, "calls")
	forgeOK(os.WriteFile(log, nil, 0600))
	for i, answer := range answers {
		data := mustForge(json.Marshal(answer))
		forgeOK(os.WriteFile(filepath.Join(dir, strconv.Itoa(i)), data, 0600))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_TEST_FORGE", dir)
	return func(want ...[]string) {
		t.Helper()
		var expected []byte
		for _, args := range want {
			expected = append(expected, mustForge(json.Marshal(args))...)
			expected = append(expected, '\n')
		}
		assertForge(t, string(mustForge(os.ReadFile(log))), string(expected))
	}
}

func assertForge(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func forgeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func forgeRepo(t *testing.T) (TaskForge, string, string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("LC_ALL", "C")
	worktree, remote := t.TempDir(), t.TempDir()
	forgeGit(t, remote, "init", "--bare", "-b", "base")
	forgeGit(t, worktree, "init", "-b", "base")
	forgeGit(t, worktree, "-c", "user.name=test", "-c", "user.email=test.invalid", "commit", "--allow-empty", "-m", "base")
	forgeGit(t, worktree, "remote", "add", "origin", remote)
	forgeGit(t, worktree, "push", "origin", "base")
	f := TaskForge{worktree, "task/4-fix"}
	forgeGit(t, worktree, "switch", "-c", f.Branch)
	forgeGit(t, worktree, "-c", "user.name=test", "-c", "user.email=test.invalid", "commit", "--allow-empty", "-m", "task")
	return f, remote, forgeGit(t, worktree, "rev-parse", "HEAD")
}

func forgeList(branch string) []string {
	return []string{"pr", "list", "--head", branch, "--base", "base", "--state", "open", "--json", "number,headRefOid,url,state,baseRefName"}
}

func forgeCreate(branch, title, body string) []string {
	return []string{"pr", "create", "--head", branch, "--base", "base", "--title", title, "--body", body}
}

func TestSubmitForge(t *testing.T) {
	f, remote, head := forgeRepo(t)
	url := "https://example.invalid/test/repo/pull/12"
	open := forgeAnswer{Stdout: fmt.Sprintf(`[{"url":%q,"state":"OPEN","baseRefName":"base"}]`, url)}
	calls := fakeForgeGH(t, forgeAnswer{Stdout: "[]\n"}, forgeAnswer{Stdout: url + "\n"}, open)
	title, summary := "Task title", "summary with \"quotes\"\nand a second line"
	for range 2 {
		pr, err := f.Submit(t.Context(), head, "base", title, summary)
		assertForge(t, err, nil)
		assertForge(t, pr, &ForgePullRequest{Number: 12, URL: url})
		assertForge(t, forgeGit(t, remote, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task"), "refs/heads/"+f.Branch+" "+head)
	}
	calls(forgeList(f.Branch), forgeCreate(f.Branch, title, summary), forgeList(f.Branch))
}

func TestPushIgnoresPushConfig(t *testing.T) {
	f, remote, head := forgeRepo(t)
	forgeGit(t, f.Worktree, "config", "remote.origin.push", "refs/heads/*:refs/heads/elsewhere/*")
	assertForge(t, f.PushBranch(t.Context()), nil)
	assertForge(t, forgeGit(t, remote, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task", "refs/heads/elsewhere"), "refs/heads/"+f.Branch+" "+head)
}

func TestSubmitForgePreconditions(t *testing.T) {
	f, remote, head := forgeRepo(t)
	calls := fakeForgeGH(t)
	submitError := func(commit string, want string) {
		t.Helper()
		pr, err := f.Submit(t.Context(), commit, "base", "title", "summary")
		assertForge(t, pr, (*ForgePullRequest)(nil))
		assertForge(t, err.Error(), want)
	}
	submitError("old", "task head differs from the checked commit")
	forgeGit(t, f.Worktree, "config", "status.showUntrackedFiles", "no")
	forgeOK(os.WriteFile(filepath.Join(f.Worktree, "untracked"), nil, 0600))
	submitError(head, "task worktree has uncommitted changes")
	forgeOK(os.Remove(filepath.Join(f.Worktree, "untracked")))
	forgeGit(t, f.Worktree, "switch", "base")
	submitError(forgeGit(t, f.Worktree, "rev-parse", "HEAD"), "task head differs from the checked commit")
	assertForge(t, forgeGit(t, remote, "for-each-ref", "--format=%(refname)"), "refs/heads/base")
	calls()
}

func TestForgeSubmitCommandFailures(t *testing.T) {
	f, _, head := forgeRepo(t)
	prError := func() string {
		t.Helper()
		pr, err := f.Submit(t.Context(), head, "base", "title", "body")
		assertForge(t, pr, (*ForgePullRequest)(nil))
		return err.Error()
	}
	calls := fakeForgeGH(t, forgeAnswer{Stderr: "login required\n", Code: 1}, forgeAnswer{Stdout: "["},
		forgeAnswer{Stdout: "[]"}, forgeAnswer{Stderr: "create denied\n", Code: 1},
		forgeAnswer{Stdout: "[]"}, forgeAnswer{Stdout: "https://example.invalid/test/repo/pull/\n"})
	assertForge(t, prError(), "login required: exit status 1")
	assertForge(t, prError(), "unexpected end of JSON input")
	assertForge(t, prError(), "create denied: exit status 1")
	assertForge(t, prError(), `strconv.Atoi: parsing "": invalid syntax`)
	create := forgeCreate(f.Branch, "title", "body")
	calls(forgeList(f.Branch), forgeList(f.Branch), forgeList(f.Branch), create, forgeList(f.Branch), create)
	forgeGit(t, f.Worktree, "remote", "remove", "origin")
	assertForge(t, prError(), "fatal: 'origin' does not appear to be a git repository\nfatal: Could not read from remote repository.\n\n"+
		"Please make sure you have the correct access rights\nand the repository exists.: exit status 128")
	f.Branch = "task/missing"
	assertForge(t, prError(), "fatal: ambiguous argument 'refs/heads/task/missing': unknown revision or path not in the working tree.\n"+
		"Use '--' to separate paths from revisions, like this:\n'git <command> [<revision>...] -- [<file>...]': exit status 128")
	f.Worktree = t.TempDir()
	assertForge(t, prError(), "fatal: not a git repository (or any of the parent directories): .git: exit status 128")
}

func TestForgeGHCannotStart(t *testing.T) {
	f, _, head := forgeRepo(t)
	calls := fakeForgeGH(t)
	gh := mustForge(exec.LookPath("gh"))
	forgeOK(os.WriteFile(gh, []byte("invalid executable"), 0700))
	pr, err := f.Submit(t.Context(), head, "base", "t", "s")
	assertForge(t, pr, (*ForgePullRequest)(nil))
	assertForge(t, err.Error(), "fork/exec "+gh+": exec format error")
	calls()
}

func hostedView(head, rollup string) forgeAnswer {
	return forgeAnswer{Stdout: fmt.Sprintf(`{"headRefOid":%q,"statusCheckRollup":[%s]}`, head, rollup)}
}

// waitHosted runs WaitHostedChecks on a virtual clock and checks the result,
// the time it took and that every read asked for the head and the rollup.
func waitHosted(t *testing.T, since time.Duration, answers []forgeAnswer, want *proto.ChecksBody, took time.Duration) {
	t.Helper()
	f := TaskForge{Worktree: t.TempDir()}
	calls := fakeForgeGH(t, answers...)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		result, err := f.WaitHostedChecks(t.Context(), 4, "new", start.Add(-since))
		assertForge(t, err, nil)
		assertForge(t, result, want)
		assertForge(t, time.Since(start), took)
	})
	views := make([][]string, len(answers))
	for i := range views {
		views[i] = []string{"pr", "view", "4", "--json", "headRefOid,statusCheckRollup"}
	}
	calls(views...)
}

const (
	queuedRun  = `{"__typename":"CheckRun","name":"build","status":"QUEUED"}`
	passedRun  = `{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS"}`
	failedRun  = `{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"FAILURE"}`
	lintStatus = `{"__typename":"StatusContext","context":"lint","state":"%s"}`
)

func TestHostedChecksReportFinishedChecks(t *testing.T) {
	waitHosted(t, 0, []forgeAnswer{hostedView("new", queuedRun), hostedView("new", failedRun+","+fmt.Sprintf(lintStatus, "SUCCESS"))},
		&proto.ChecksBody{Stage: "cloud", Commit: "new", OK: false, Items: []proto.CheckItem{
			{Name: "build", Detail: new("FAILURE")}, {Name: "lint", OK: true, Detail: new("SUCCESS")}}}, 30*time.Second)
}

// Checks the forge registers a few seconds late are not skipped.
func TestHostedChecksLateRegistration(t *testing.T) {
	waitHosted(t, 0, []forgeAnswer{hostedView("new", ""), hostedView("new", passedRun)},
		&proto.ChecksBody{Stage: "cloud", Commit: "new", OK: true, Items: []proto.CheckItem{{Name: "build", OK: true, Detail: new("SUCCESS")}}},
		30*time.Second)
}

// No checks pass on the second consecutive empty read at the submitted head;
// a failed read, another head or an unfinished check in between start again.
func TestHostedChecksNoChecks(t *testing.T) {
	waitHosted(t, 0, []forgeAnswer{hostedView("new", ""), {Stderr: "login required", Code: 1}, hostedView("new", ""),
		hostedView("stale", ""), hostedView("new", ""), {Stdout: "{"}, hostedView("new", ""), hostedView("new", queuedRun),
		hostedView("new", ""), hostedView("new", "")},
		&proto.ChecksBody{Stage: "cloud", Commit: "new", OK: true, Items: []proto.CheckItem{}}, 9*hostedPoll)
}

// After sixty minutes the last read's items stay as they were and the reason
// is added; a read that fails does not replace them.
func TestHostedChecksTimeOut(t *testing.T) {
	timedOut := proto.CheckItem{Name: "hosted checks", Detail: new("hosted checks timed out")}
	waitHosted(t, 59*time.Minute, []forgeAnswer{hostedView("new", queuedRun+","+fmt.Sprintf(lintStatus, "PENDING")),
		hostedView("new", passedRun+","+fmt.Sprintf(lintStatus, "EXPECTED")), {Stderr: "login required", Code: 1}},
		&proto.ChecksBody{Stage: "cloud", Commit: "new", OK: false, Items: []proto.CheckItem{
			{Name: "build", OK: true, Detail: new("SUCCESS")}, {Name: "lint", Detail: new("EXPECTED")}, timedOut}}, time.Minute)
	waitHosted(t, hostedLimit, []forgeAnswer{hostedView("stale", passedRun)},
		&proto.ChecksBody{Stage: "cloud", Commit: "new", OK: false, Items: []proto.CheckItem{timedOut}}, 0)
}

func TestHostedChecksStopOnCancel(t *testing.T) {
	f := TaskForge{Worktree: t.TempDir()}
	calls := fakeForgeGH(t, hostedView("new", queuedRun))
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		result, err := f.WaitHostedChecks(ctx, 4, "new", time.Now())
		assertForge(t, result, (*proto.ChecksBody)(nil))
		assertForge(t, err, context.DeadlineExceeded)
	})
	calls([]string{"pr", "view", "4", "--json", "headRefOid,statusCheckRollup"})
}

func TestForgeCheckItems(t *testing.T) {
	for _, tc := range []struct {
		check forgeCheck
		item  proto.CheckItem
		done  bool
	}{
		{forgeCheck{TypeName: "CheckRun", Name: "a", Status: "COMPLETED", Conclusion: "FAILURE"}, proto.CheckItem{Name: "a", Detail: new("FAILURE")}, true},
		{forgeCheck{TypeName: "CheckRun", Name: "a", Status: "COMPLETED", Conclusion: "SUCCESS"}, proto.CheckItem{Name: "a", OK: true, Detail: new("SUCCESS")}, true},
		{forgeCheck{TypeName: "CheckRun", Name: "a", Status: "COMPLETED", Conclusion: "NEUTRAL"}, proto.CheckItem{Name: "a", OK: true, Detail: new("NEUTRAL")}, true},
		{forgeCheck{TypeName: "CheckRun", Name: "a", Status: "COMPLETED", Conclusion: "SKIPPED"}, proto.CheckItem{Name: "a", OK: true, Detail: new("SKIPPED")}, true},
		{forgeCheck{TypeName: "CheckRun", Name: "a", Status: "IN_PROGRESS"}, proto.CheckItem{Name: "a", Detail: new("IN_PROGRESS")}, false},
		{forgeCheck{TypeName: "StatusContext", Context: "b", State: "SUCCESS"}, proto.CheckItem{Name: "b", OK: true, Detail: new("SUCCESS")}, true},
		{forgeCheck{TypeName: "StatusContext", Context: "b", State: "ERROR"}, proto.CheckItem{Name: "b", Detail: new("ERROR")}, true},
		{forgeCheck{TypeName: "StatusContext", Context: "b", State: "PENDING"}, proto.CheckItem{Name: "b", Detail: new("PENDING")}, false},
		{forgeCheck{TypeName: "StatusContext", Context: "b", State: "EXPECTED"}, proto.CheckItem{Name: "b", Detail: new("EXPECTED")}, false},
	} {
		item, done := tc.check.item()
		assertForge(t, item, tc.item)
		assertForge(t, done, tc.done)
	}
}

func TestHostedChecksToResume(t *testing.T) {
	task := func(id, state, step, assignee string) proto.TaskRow {
		return proto.TaskRow{RowHeader: proto.RowHeader{ID: id}, State: state, Step: step, Assignee: assignee, Branch: "task/4-fix",
			Commit: "new", PR: 12, Checks: proto.TaskChecks{Local: &proto.Check{OK: true, At: 1_700_000_000_123, Commit: "new"}}}
	}
	tasks := []proto.TaskRow{task("t1", "active", "cloud_check", "a1"), task("t2", "active", "cloud_check", "a9"),
		task("t3", "active", "review", "a1"), task("t4", "paused", "cloud_check", "a2"), task("t5", "active", "cloud_check", "a2")}
	assertForge(t, HostedChecksToResume(tasks, []string{"a1", "a2"}), []HostedCheck{
		{Task: "t1", Branch: "task/4-fix", Commit: "new", PR: 12, Since: time.UnixMilli(1_700_000_000_123)},
		{Task: "t5", Branch: "task/4-fix", Commit: "new", PR: 12, Since: time.UnixMilli(1_700_000_000_123)},
	})
	assertForge(t, HostedChecksToResume(tasks, nil), []HostedCheck(nil))
}
