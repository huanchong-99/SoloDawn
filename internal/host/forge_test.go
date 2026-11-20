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

func forgeView(branch string) []string {
	return []string{"pr", "view", branch, "--json", "number,headRefOid"}
}

func noForgePR(branch string) forgeAnswer {
	return forgeAnswer{Stderr: fmt.Sprintf("no pull requests found for branch %q\n", branch), Code: 1}
}

func TestSubmitForge(t *testing.T) {
	f, remote, head := forgeRepo(t)
	view := forgeAnswer{Stdout: fmt.Sprintf(`{"number":1,"headRefOid":%q}`, head)}
	calls := fakeForgeGH(t, noForgePR(f.Branch), forgeAnswer{Stdout: "https://example.invalid/test/repo/pull/1\n"}, view)
	title, summary := "Task title", "summary with \"quotes\"\nand a second line"
	for range 2 {
		pr, err := f.Submit(t.Context(), head, "base", title, summary)
		assertForge(t, err, nil)
		assertForge(t, pr, 1)
		assertForge(t, forgeGit(t, remote, "rev-parse", f.Branch), head)
	}
	calls(forgeView(f.Branch), []string{"pr", "create", "--head", f.Branch, "--base", "base", "--title", title, "--body", summary}, forgeView(f.Branch))
}

func TestSubmitForgePreconditions(t *testing.T) {
	f, remote, head := forgeRepo(t)
	calls := fakeForgeGH(t)
	submitError := func(commit string, want string) {
		t.Helper()
		pr, err := f.Submit(t.Context(), commit, "base", "title", "summary")
		assertForge(t, pr, 0)
		assertForge(t, err.Error(), want)
	}
	submitError("old", "task head differs from the checked commit")
	forgeGit(t, f.Worktree, "switch", "base")
	submitError(forgeGit(t, f.Worktree, "rev-parse", "HEAD"), "task head differs from the checked commit")
	forgeOK(os.WriteFile(filepath.Join(f.Worktree, "dirty"), nil, 0600))
	submitError(head, "task worktree has uncommitted changes")
	assertForge(t, forgeGit(t, remote, "for-each-ref", "--format=%(refname:short)"), "base")
	calls()
}

func TestForgeSubmitCommandFailures(t *testing.T) {
	f, _, head := forgeRepo(t)
	prError := func() string {
		t.Helper()
		pr, err := f.Submit(t.Context(), head, "base", "title", "body")
		assertForge(t, pr, 0)
		return err.Error()
	}
	calls := fakeForgeGH(t, forgeAnswer{Stderr: "login required\n", Code: 1}, noForgePR(f.Branch), forgeAnswer{Stderr: "create denied\n", Code: 1})
	assertForge(t, prError(), "login required: exit status 1")
	assertForge(t, prError(), "create denied: exit status 1")
	calls(forgeView(f.Branch), forgeView(f.Branch), []string{"pr", "create", "--head", f.Branch, "--base", "base", "--title", "title", "--body", "body"})
	forgeGit(t, f.Worktree, "remote", "remove", "origin")
	assertForge(t, strings.Contains(prError(), "'origin' does not appear to be a git repository"), true)
	f.Branch = "task/missing"
	assertForge(t, strings.Contains(prError(), "unknown revision"), true)
	f.Worktree = t.TempDir()
	assertForge(t, strings.Contains(prError(), "not a git repository"), true)
}

func TestForgeGHCannotStart(t *testing.T) {
	f, _, head := forgeRepo(t)
	calls := fakeForgeGH(t)
	gh := mustForge(exec.LookPath("gh"))
	forgeOK(os.WriteFile(gh, []byte("invalid executable"), 0700))
	pr, err := f.Submit(t.Context(), head, "base", "t", "s")
	assertForge(t, pr, 0)
	assertForge(t, err != nil, true)
	calls()
}

func TestForgePRReplies(t *testing.T) {
	for _, view := range []string{"not JSON", `{"number":0,"headRefOid":"new"}`, `{"number":2,"headRefOid":"old"}`} {
		pr, err := existingPR(view, "new")
		assertForge(t, pr, 0)
		assertForge(t, err != nil, true)
	}
	for _, out := range []string{"invalid", "/org/repo/pull/1", "https://example.invalid/issues/1", "https://example.invalid/pull/no", "https://example.invalid/pull/0"} {
		pr, err := createdPRNumber(out)
		assertForge(t, pr, 0)
		assertForge(t, err != nil, true)
	}
}

func TestForgeHostedPoll(t *testing.T) {
	f := TaskForge{Worktree: t.TempDir()}
	calls := fakeForgeGH(t,
		forgeAnswer{Stdout: `{"headRefOid":"stale","statusCheckRollup":[]}`},
		forgeAnswer{Stdout: `{"headRefOid":"new","statusCheckRollup":[{"__typename":"CheckRun","status":"QUEUED"}]}`},
		forgeAnswer{Stdout: `{"headRefOid":"new","statusCheckRollup":[]}`},
	)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		result, err := f.WaitHostedChecks(t.Context(), 4, "new")
		assertForge(t, err, nil)
		assertForge(t, result, &proto.ChecksBody{Stage: "cloud", Commit: "new", OK: true, Items: []proto.CheckItem{}})
		assertForge(t, time.Since(start), 60*time.Second)
	})
	view := []string{"pr", "view", "4", "--json", "headRefOid,statusCheckRollup"}
	calls(view, view, view)
}

func TestForgePollStops(t *testing.T) {
	f := TaskForge{Worktree: t.TempDir()}
	calls := fakeForgeGH(t, forgeAnswer{Stdout: `{"headRefOid":"stale","statusCheckRollup":[]}`}, forgeAnswer{Stderr: "login required", Code: 1})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		result, err := f.WaitHostedChecks(ctx, 4, "new")
		assertForge(t, result, (*proto.ChecksBody)(nil))
		assertForge(t, err, context.DeadlineExceeded)
	})
	result, err := f.WaitHostedChecks(t.Context(), 4, "new")
	assertForge(t, result, (*proto.ChecksBody)(nil))
	assertForge(t, err.Error(), "login required: exit status 1")
	view := []string{"pr", "view", "4", "--json", "headRefOid,statusCheckRollup"}
	calls(view, view)
}

func TestForgeCheckRollup(t *testing.T) {
	checks := []forgeCheck{{TypeName: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: "FAILURE"},
		{TypeName: "StatusContext", Context: "lint", State: "SUCCESS"}, {TypeName: "CheckRun", Name: "optional", Status: "COMPLETED", Conclusion: "SKIPPED"}}
	want := &proto.ChecksBody{Stage: "cloud", Commit: "new", OK: false, Items: []proto.CheckItem{
		{Name: "build", OK: false, Detail: new("FAILURE")}, {Name: "lint", OK: true, Detail: new("SUCCESS")}, {Name: "optional", OK: true, Detail: new("SKIPPED")},
	}}
	result, err := completedChecks("new", checks)
	assertForge(t, err, nil)
	assertForge(t, result, want)
	for _, state := range []string{"EXPECTED", "PENDING"} {
		checks[1].State = state
		result, err = completedChecks("new", checks)
		assertForge(t, err, nil)
		assertForge(t, result, (*proto.ChecksBody)(nil))
	}
}

func TestForgeMalformedChecks(t *testing.T) {
	for _, out := range []string{"bad", `{"statusCheckRollup":[]}`, `{"headRefOid":"new"}`,
		`{"headRefOid":"new","statusCheckRollup":[{}]}`, `{"headRefOid":"new","statusCheckRollup":[{"__typename":"CheckRun"}]}`,
		`{"headRefOid":"new","statusCheckRollup":[{"__typename":"StatusContext","state":"SUCCESS"}]}`,
		`{"headRefOid":"new","statusCheckRollup":[{"__typename":"StatusContext","state":"invalid"}]}`} {
		result, err := hostedResult(out, "new")
		assertForge(t, result, (*proto.ChecksBody)(nil))
		assertForge(t, err != nil, true)
	}
}
