package host_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
)

func mergeEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func mergeOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mergeGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mergeFile(t *testing.T, repo, file, contents string) {
	t.Helper()
	mergeOK(t, os.WriteFile(filepath.Join(repo, file), []byte(contents), 0600))
}

func mergeFixture(t *testing.T) host.ReviewedMerge {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("LC_ALL", "C")
	mergeGit(t, repo, "init", "-q", "-b", "base")
	mergeGit(t, repo, "config", "user.name", "fixture")
	mergeGit(t, repo, "config", "user.email", "fixture")
	mergeFile(t, repo, "item", "base\n")
	mergeGit(t, repo, "add", "item")
	mergeGit(t, repo, "commit", "-qm", "start")
	mergeGit(t, repo, "switch", "-qc", "task/7")
	mergeFile(t, repo, "item", "reviewed\n")
	mergeGit(t, repo, "commit", "-qam", "edit")
	commit := mergeGit(t, repo, "rev-parse", "HEAD")
	mergeGit(t, repo, "switch", "-q", "base")
	return host.ReviewedMerge{Repo: repo, Base: "base", Branch: "task/7", Task: "t7", Commit: commit}
}

// noHooks is the setting the host's local merge passes to Git with -c.
func noHooks(m host.ReviewedMerge) string {
	return "core.hooksPath=" + filepath.Join(m.Repo, ".git", "solodawn-no-hooks")
}

func TestLocalReviewedMerge(t *testing.T) {
	m := mergeFixture(t)
	base := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	head := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	mergeEqual(t, result, host.MergeResult{MergeCommit: head})
	mergeEqual(t, mergeGit(t, m.Repo, "log", "-1", "--format=%P%n%B"),
		base+"\nMerge task t7 at "+m.Commit)
	mergeEqual(t, mergeGit(t, m.Repo, "show", "base:item"), "reviewed")
	mergeEqual(t, mergeGit(t, m.Repo, "rev-parse", "task/7"), m.Commit)
	// Another checkout is active on restart. Inspect the base ref, not HEAD.
	mergeGit(t, m.Repo, "switch", "-q", "task/7")
	observed, err := m.Inspect(t.Context())
	mergeOK(t, err)
	mergeEqual(t, observed, result)
	m.Task = "t71"
	observed, err = m.Inspect(t.Context())
	mergeOK(t, err)
	mergeEqual(t, observed, host.MergeResult{Conflict: "no commit on base records Merge task t71 at " + m.Commit})
}

func TestLocalMergeRefusal(t *testing.T) {
	m := mergeFixture(t)
	base := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	check := func(detail string) {
		t.Helper()
		result, err := m.Apply(t.Context())
		mergeOK(t, err)
		mergeEqual(t, result, host.MergeResult{Conflict: detail})
		mergeEqual(t, mergeGit(t, m.Repo, "rev-parse", "base"), base)
	}
	mergeGit(t, m.Repo, "switch", "-q", "task/7")
	check(`git symbolic-ref HEAD: got "refs/heads/task/7", expected "refs/heads/base"`)
	mergeGit(t, m.Repo, "commit", "--allow-empty", "-qm", "after review")
	moved := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	mergeGit(t, m.Repo, "switch", "-q", "base")
	check(fmt.Sprintf("git rev-parse --verify refs/heads/task/7: got %q, expected %q", moved, m.Commit))
}

// gitFailure runs a Git command that must fail and returns its error as the
// host words it, with Git's own text, which differs between Git versions.
func gitFailure(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	mergeEqual(t, err != nil, true)
	return fmt.Sprintf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)+stderr.String()))
}

func TestLocalConflictAndCommitFailureResetIndex(t *testing.T) {
	for _, stage := range []string{"merge", "commit"} {
		t.Run(stage, func(t *testing.T) {
			m := mergeFixture(t)
			if stage == "merge" {
				mergeFile(t, m.Repo, "item", "base advanced\n")
				mergeGit(t, m.Repo, "commit", "-qam", "overlapping change")
			} else {
				// No identity: the commit fails after the squash has applied.
				mergeGit(t, m.Repo, "config", "--unset", "user.name")
				mergeGit(t, m.Repo, "config", "--unset", "user.email")
				mergeGit(t, m.Repo, "config", "user.useConfigOnly", "true")
			}
			base := mergeGit(t, m.Repo, "rev-parse", "HEAD")
			result, err := m.Apply(t.Context())
			mergeOK(t, err)
			mergeEqual(t, mergeGit(t, m.Repo, "status", "--porcelain"), "")
			mergeEqual(t, mergeGit(t, m.Repo, "rev-parse", "HEAD"), base)
			var want string
			if stage == "merge" {
				want = gitFailure(t, m.Repo, "-c", noHooks(m), "merge", "--squash", "--", m.Commit)
			} else {
				mergeGit(t, m.Repo, "merge", "--squash", "--", m.Commit)
				want = gitFailure(t, m.Repo, "-c", noHooks(m), "commit", "--allow-empty", "--no-verify", "--no-gpg-sign", "-m", "Merge task t7 at "+m.Commit)
			}
			mergeEqual(t, result, host.MergeResult{Conflict: want})
		})
	}
}

// A restart between the squash and its commit: the base does not record the
// merge, and the interrupted squash is undone.
func TestInterruptedLocalSquash(t *testing.T) {
	m := mergeFixture(t)
	base := mergeGit(t, m.Repo, "rev-parse", "base")
	mergeGit(t, m.Repo, "merge", "--squash", m.Commit)
	result, err := m.Inspect(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{Conflict: "no commit on base records Merge task t7 at " + m.Commit})
	mergeEqual(t, mergeGit(t, m.Repo, "rev-parse", "base"), base)
	mergeEqual(t, mergeGit(t, m.Repo, "status", "--porcelain"), "")
	result, err = m.Apply(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{MergeCommit: mergeGit(t, m.Repo, "rev-parse", "base")})
}

func TestInterruptedLocalSquashResetFailure(t *testing.T) {
	m := mergeFixture(t)
	mergeGit(t, m.Repo, "merge", "--squash", m.Commit)
	mergeFile(t, m.Repo, filepath.Join(".git", "index.lock"), "")
	result, err := m.Inspect(t.Context())
	mergeEqual(t, result, host.MergeResult{})
	mergeEqual(t, err.Error(), gitFailure(t, m.Repo, "-c", noHooks(m), "reset", "--merge"))
}

func TestMergeChangeAlreadyOnBase(t *testing.T) {
	m := mergeFixture(t)
	mergeFile(t, m.Repo, "item", "reviewed\n")
	mergeGit(t, m.Repo, "commit", "-qam", "same edit on base")
	base := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	head := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	mergeEqual(t, result, host.MergeResult{MergeCommit: head})
	mergeEqual(t, mergeGit(t, m.Repo, "log", "-1", "--format=%P%n%B"), base+"\nMerge task t7 at "+m.Commit)
}

func TestInspectSignedLocalMerge(t *testing.T) {
	m := mergeFixture(t)
	_, err := m.Apply(t.Context())
	mergeOK(t, err)
	// Include a signature header without requiring a local signing key.
	// With log.showSignature, Git includes its verification diagnostics.
	raw := mergeGit(t, m.Repo, "cat-file", "commit", "HEAD")
	signed := strings.Replace(raw, "\n\n", "\ngpgsig -----BEGIN PGP SIGNATURE-----\n fixture\n -----END PGP SIGNATURE-----\n\n", 1)
	dir := t.TempDir()
	mergeFile(t, dir, "signed", signed+"\n")
	oid := mergeGit(t, m.Repo, "hash-object", "-w", "-t", "commit", filepath.Join(dir, "signed"))
	mergeGit(t, m.Repo, "update-ref", "refs/heads/base", oid)
	mergeGit(t, m.Repo, "config", "log.showSignature", "true")
	result, err := m.Inspect(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{MergeCommit: oid})
}

func TestMergeMissingRefs(t *testing.T) {
	m := mergeFixture(t)
	m.Branch = "absent"
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{Conflict: "git rev-parse --verify refs/heads/absent: exit status 128: fatal: Needed a single revision"})
	m.Base = "absent"
	result, err = m.Inspect(t.Context())
	mergeEqual(t, result, host.MergeResult{})
	mergeEqual(t, err.Error(), gitFailure(t, m.Repo, "log", "-z", "--first-parent", "--no-show-signature", "--fixed-strings",
		"--grep=Merge task t7 at "+m.Commit, "--format=%H%n%B", "refs/heads/absent", "--"))
}

// Re-execute this test binary as gh. Each invocation records the folder it
// runs in and its argv in the log the variable names, so the tests cover
// both the command and where it runs.
func init() {
	log := os.Getenv("SOLODAWN_TEST_MERGE_GH")
	if log == "" {
		return
	}
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(f, dir+" | "+strings.Join(os.Args[1:], " "))
	f.Close()
	if os.Args[2] == os.Getenv("FAIL_GH_COMMAND") {
		fmt.Fprintln(os.Stderr, "forge refused")
		os.Exit(1)
	}
	fmt.Println(os.Getenv("GH_FIXTURE_OUTPUT"))
	os.Exit(0)
}

func mergeForgeFixture(t *testing.T) host.ReviewedMerge {
	t.Helper()
	exe, err := os.Executable()
	mergeOK(t, err)
	bin := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(exe)
	mergeOK(t, err)
	mergeOK(t, os.WriteFile(filepath.Join(bin, name), data, 0700))
	t.Setenv("SOLODAWN_TEST_MERGE_GH", filepath.Join(bin, "calls"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := filepath.Join(t.TempDir(), "repo")
	mergeOK(t, os.Mkdir(repo, 0700))
	return host.ReviewedMerge{Repo: repo, Task: "t7", Commit: "abc", PR: 32, GitHubRepo: "team/website"}
}

// checkMergeCalls compares the gh calls with want, a list of "merge" (run
// with --repo from the folder that holds the clone) and "view" (in the clone).
func checkMergeCalls(t *testing.T, m host.ReviewedMerge, want ...string) {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("SOLODAWN_TEST_MERGE_GH"))
	mergeOK(t, err)
	calls := map[string]string{
		"merge": filepath.Dir(m.Repo) + " | pr merge 32 --repo team/website --squash --delete-branch --match-head-commit abc\n",
		"view":  m.Repo + " | pr view 32 --json state,mergeCommit,headRefOid\n",
	}
	var expected string
	for _, call := range want {
		expected += calls[call]
	}
	mergeEqual(t, string(data), expected)
}

func TestMergeReviewedPR(t *testing.T) {
	m := mergeForgeFixture(t)
	t.Setenv("GH_FIXTURE_OUTPUT", `{"state":"MERGED","mergeCommit":{"oid":"def"},"headRefOid":"abc"}`)
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{MergeCommit: "def"})
	checkMergeCalls(t, m, "merge", "view")
	observed, err := m.Inspect(t.Context())
	mergeOK(t, err)
	mergeEqual(t, observed, result)
	checkMergeCalls(t, m, "merge", "view", "view")
}

func TestMergePRCommandFailure(t *testing.T) {
	m := mergeForgeFixture(t)
	t.Setenv("FAIL_GH_COMMAND", "merge")
	t.Setenv("GH_FIXTURE_OUTPUT", `{"state":"OPEN"}`)
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{Conflict: "gh pr merge 32 --repo team/website --squash --delete-branch --match-head-commit abc: exit status 1: forge refused"})
	checkMergeCalls(t, m, "merge", "view")
}

func TestMergePRBranchDeletionFailure(t *testing.T) {
	m := mergeForgeFixture(t)
	t.Setenv("FAIL_GH_COMMAND", "merge")
	t.Setenv("GH_FIXTURE_OUTPUT", `{"state":"MERGED","mergeCommit":{"oid":"def"},"headRefOid":"abc"}`)
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{MergeCommit: "def"})
	checkMergeCalls(t, m, "merge", "view")
}

func TestMergePRInspection(t *testing.T) {
	for _, tc := range []struct{ out, fail, conflict, problem string }{
		{out: `{"state":"OPEN","mergeCommit":null}`, conflict: "merge still pending after 60 minutes (merge queue or auto-merge)"},
		{out: `{"state":"CLOSED","mergeCommit":null}`, conflict: "pull request 32 is CLOSED"},
		{out: `{"state":"MERGED","mergeCommit":{"oid":"def"},"headRefOid":"abc1"}`, conflict: "pull request 32 merged head abc1, expected abc"},
		// A read that fails counts as unfinished; the limit has passed.
		{out: "{", conflict: "merge still pending after 60 minutes (merge queue or auto-merge)"},
		{fail: "view", conflict: "merge still pending after 60 minutes (merge queue or auto-merge)"},
	} {
		t.Run(tc.out+tc.fail, func(t *testing.T) {
			m := mergeForgeFixture(t)
			// The merge was taken long ago: a PR still OPEN is read once.
			m.Taken = time.Unix(1, 0)
			t.Setenv("GH_FIXTURE_OUTPUT", tc.out)
			t.Setenv("FAIL_GH_COMMAND", tc.fail)
			result, err := m.Inspect(t.Context())
			mergeEqual(t, result, host.MergeResult{Conflict: tc.conflict})
			if tc.problem == "" {
				mergeOK(t, err)
			} else {
				mergeEqual(t, err.Error(), tc.problem)
			}
			checkMergeCalls(t, m, "view")
		})
	}
}

// The branch is deleted only while it is still the merged commit; a branch
// that has moved since keeps its unreviewed commits.
func TestDeleteBranchOnlyAtTheMergedCommit(t *testing.T) {
	m := mergeFixture(t)
	_, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeGit(t, m.Repo, "switch", "-q", m.Branch)
	mergeGit(t, m.Repo, "commit", "--allow-empty", "-qm", "unreviewed")
	moved := mergeGit(t, m.Repo, "rev-parse", "HEAD")
	mergeGit(t, m.Repo, "switch", "-q", "base")
	deleted, err := m.DeleteBranch(t.Context())
	mergeOK(t, err)
	mergeEqual(t, deleted, false)
	mergeEqual(t, mergeGit(t, m.Repo, "rev-parse", m.Branch), moved)
	mergeGit(t, m.Repo, "branch", "-f", m.Branch, m.Commit)
	deleted, err = m.DeleteBranch(t.Context())
	mergeOK(t, err)
	mergeEqual(t, deleted, true)
	mergeEqual(t, mergeGit(t, m.Repo, "for-each-ref", "--format=%(refname)", "refs/heads/task"), "")
}

func TestDeleteBranchFailures(t *testing.T) {
	m := mergeFixture(t)
	mergeFile(t, m.Repo, filepath.Join(".git", "refs", "heads", "task", "7.lock"), "")
	deleted, err := m.DeleteBranch(t.Context())
	mergeEqual(t, deleted, false)
	mergeEqual(t, err.Error(), gitFailure(t, m.Repo, "update-ref", "-d", "refs/heads/task/7", m.Commit))
	mergeEqual(t, mergeGit(t, m.Repo, "rev-parse", m.Branch), m.Commit)
	m.Branch = "absent"
	deleted, err = m.DeleteBranch(t.Context())
	mergeEqual(t, deleted, false)
	mergeEqual(t, err.Error(), gitFailure(t, m.Repo, "rev-parse", "--verify", "refs/heads/absent"))
}

// The local merge runs no hook of the project and does not sign. Husky, for
// example, sets core.hooksPath in the clone's config once a check has run
// npm install.
func TestLocalMergeRunsNoProjectHooks(t *testing.T) {
	for name, hook := range map[string]string{
		"prepare-commit-msg": "#!/bin/sh\nprintf '\\nTrailer: added\\n' >> \"$1\"\n",
		"commit-msg":         "#!/bin/sh\necho rejected >&2\nexit 1\n",
		"pre-commit":         "#!/bin/sh\nexit 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			m := mergeFixture(t)
			hooks := t.TempDir()
			mergeOK(t, os.WriteFile(filepath.Join(hooks, name), []byte(hook), 0700))
			mergeGit(t, m.Repo, "config", "core.hooksPath", hooks)
			mergeGit(t, m.Repo, "config", "commit.gpgsign", "true")
			mergeGit(t, m.Repo, "config", "gpg.program", "missing-fixture-program")
			result, err := m.Apply(t.Context())
			mergeOK(t, err)
			head := mergeGit(t, m.Repo, "rev-parse", "base")
			mergeEqual(t, result, host.MergeResult{MergeCommit: head})
			mergeEqual(t, mergeGit(t, m.Repo, "log", "-1", "--format=%B", "base"), "Merge task t7 at "+m.Commit)
			observed, err := m.Inspect(t.Context())
			mergeOK(t, err)
			mergeEqual(t, observed, result)
		})
	}
}

// The empty hooks folder cannot be made: the merge is a conflict with the
// error, and an inspection after a restart returns it.
func TestLocalMergeWithoutHooksFolder(t *testing.T) {
	m := mergeFixture(t)
	mergeFile(t, m.Repo, filepath.Join(".git", "solodawn-no-hooks"), "")
	// The system's own text, which differs between systems.
	want := os.MkdirAll(filepath.Join(m.Repo, ".git", "solodawn-no-hooks"), 0o700).Error()
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeEqual(t, result, host.MergeResult{Conflict: want})
	result, err = m.Inspect(t.Context())
	mergeEqual(t, result, host.MergeResult{})
	mergeEqual(t, err.Error(), want)
}

// After a restart the merge is found on the base's first-parent history even
// when another task has merged after it.
func TestInspectFindsAnEarlierLocalMerge(t *testing.T) {
	m := mergeFixture(t)
	first, err := m.Apply(t.Context())
	mergeOK(t, err)
	mergeGit(t, m.Repo, "switch", "-qc", "task/8")
	mergeFile(t, m.Repo, "other", "eight\n")
	mergeGit(t, m.Repo, "add", "other")
	mergeGit(t, m.Repo, "commit", "-qm", "eight")
	second := host.ReviewedMerge{Repo: m.Repo, Base: "base", Branch: "task/8", Task: "t8", Commit: mergeGit(t, m.Repo, "rev-parse", "HEAD")}
	mergeGit(t, m.Repo, "switch", "-q", "base")
	_, err = second.Apply(t.Context())
	mergeOK(t, err)
	observed, err := m.Inspect(t.Context())
	mergeOK(t, err)
	mergeEqual(t, observed, first)
}

// The detail of task conflict goes to the group, so a secret in Git's output
// is redacted. A conflict in a file named like a token puts one there.
func TestConflictDetailIsRedacted(t *testing.T) {
	m := mergeFixture(t)
	token := "ghp_" + strings.Repeat("a", 36)
	mergeGit(t, m.Repo, "switch", "-q", m.Branch)
	mergeFile(t, m.Repo, token, "task\n")
	mergeGit(t, m.Repo, "add", token)
	mergeGit(t, m.Repo, "commit", "-qm", "task file")
	m.Commit = mergeGit(t, m.Repo, "rev-parse", "HEAD")
	mergeGit(t, m.Repo, "switch", "-q", "base")
	mergeFile(t, m.Repo, token, "base\n")
	mergeGit(t, m.Repo, "add", token)
	mergeGit(t, m.Repo, "commit", "-qm", "base file")
	result, err := m.Apply(t.Context())
	mergeOK(t, err)
	raw := gitFailure(t, m.Repo, "-c", noHooks(m), "merge", "--squash", "--", m.Commit)
	mergeEqual(t, strings.Contains(raw, token), true)
	mergeEqual(t, result, host.MergeResult{Conflict: strings.ReplaceAll(raw, token, "[redacted]")})
}
