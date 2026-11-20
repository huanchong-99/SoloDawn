package host

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func workOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func workEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSuffix(string(out), "\n")
}

type workspaceFixture struct{ project, source, repo string }

func workspaceSetup(t *testing.T) workspaceFixture {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "fixture")
	t.Setenv("GIT_COMMITTER_NAME", "fixture")
	t.Setenv("GIT_AUTHOR_EMAIL", "fixture")
	t.Setenv("GIT_COMMITTER_EMAIL", "fixture")
	t.Setenv("LC_ALL", "C")
	// Git reports real paths; the temporary folder may be behind a symlink (macOS).
	dir, err := filepath.EvalSymlinks(t.TempDir())
	workOK(t, err)
	source := filepath.Join(dir, "source")
	project := filepath.Join(dir, "projects", "group")
	fixtureGit(t, dir, "init", "-b", "trunk", source)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "base")
	upstream := filepath.Join(dir, "origin.git")
	fixtureGit(t, dir, "clone", "--bare", source, upstream)
	fixtureGit(t, source, "remote", "add", "origin", upstream)
	repo, err := EnsureGroupWorkspace(project, upstream, "trunk")
	workOK(t, err)
	workEqual(t, repo, filepath.Join(project, "repo"))
	return workspaceFixture{project, source, repo}
}

func workspaceCommit(t *testing.T, dir, file, content string) string {
	t.Helper()
	workOK(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0644))
	fixtureGit(t, dir, "add", "--", file)
	fixtureGit(t, dir, "commit", "-m", "update "+file)
	return fixtureGit(t, dir, "rev-parse", "HEAD")
}

func TestWorkspaceCloneAndRemoteBase(t *testing.T) {
	f := workspaceSetup(t)
	old := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	newHead := workspaceCommit(t, f.source, "upstream", "new\n")
	fixtureGit(t, f.source, "push", "origin", "trunk")
	again, err := EnsureGroupWorkspace(f.project, "unused", "unused")
	workOK(t, err)
	workEqual(t, again, f.repo)
	workEqual(t, fixtureGit(t, again, "rev-parse", "HEAD"), old)
	tree, err := CreateTaskWorkspace(f.project, 12, "fix", "trunk", true, false)
	workOK(t, err)
	workEqual(t, tree, filepath.Join(f.project, "wt", "12-fix"))
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/12-fix")
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), newHead)
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "HEAD"), old)
	workOK(t, RefreshGroupWorkspace(f.project, "trunk", true))
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "HEAD"), newHead)
}

func TestWorkspaceLocalReassignment(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	workOK(t, RefreshGroupWorkspace(f.project, "trunk", false))
	tree, err := CreateTaskWorkspace(f.project, 2, "docs", "trunk", false, false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "guide", "draft")
	workOK(t, RemoveTaskWorkspace(f.project, 2, "docs", false))
	_, err = os.Stat(tree)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	resumed, err := CreateTaskWorkspace(f.project, 2, "docs", "trunk", false, true)
	workOK(t, err)
	workEqual(t, resumed, tree)
	workEqual(t, fixtureGit(t, resumed, "branch", "--show-current"), "task/2-docs")
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), head)
}

func TestWorkspaceCancelDiscardsUncommittedChanges(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(f.project, 5, "draft", "trunk", false, false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "kept", "committed\n")
	workOK(t, os.WriteFile(filepath.Join(tree, "kept"), []byte("edited\n"), 0644))
	workOK(t, os.WriteFile(filepath.Join(tree, "loose"), []byte("untracked\n"), 0644))
	requireGitExit(t, RemoveTaskWorkspace(f.project, 5, "draft", false), 128)
	data, err := os.ReadFile(filepath.Join(tree, "kept"))
	workOK(t, err)
	workEqual(t, string(data), "edited\n")
	workOK(t, RemoveTaskWorkspace(f.project, 5, "draft", true))
	_, err = os.Stat(tree)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "task/5-draft"), head)
}

func TestWorkspaceRemoteReassignment(t *testing.T) {
	f := workspaceSetup(t)
	base := workspaceCommit(t, f.source, "base", "updated")
	fixtureGit(t, f.source, "checkout", "-b", "task/3-fix")
	published := workspaceCommit(t, f.source, "progress", "published")
	fixtureGit(t, f.source, "push", "origin", "--all")
	tree, err := CreateTaskWorkspace(f.project, 3, "fix", "trunk", true, true)
	workOK(t, err)
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), published)
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/3-fix")
	tree, err = CreateTaskWorkspace(f.project, 4, "fix", "trunk", true, true)
	workOK(t, err)
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), base)
}

func TestWorkspaceRetainedBranchFastForward(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(f.project, 3, "fix", "trunk", true, false)
	workOK(t, err)
	workOK(t, RemoveTaskWorkspace(f.project, 3, "fix", false))
	fixtureGit(t, f.source, "switch", "-c", "task/3-fix")
	newHead := workspaceCommit(t, f.source, "progress", "published")
	fixtureGit(t, f.source, "push", "origin", "task/3-fix")
	resumed, err := CreateTaskWorkspace(f.project, 3, "fix", "trunk", true, true)
	workOK(t, err)
	workEqual(t, resumed, tree)
	workEqual(t, fixtureGit(t, resumed, "branch", "--show-current"), "task/3-fix")
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), newHead)
	// A concurrent second attempt cannot remove the active worktree.
	_, err = CreateTaskWorkspace(f.project, 3, "fix", "trunk", true, true)
	requireGitExit(t, err, 128)
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), newHead)
}

func TestWorkspaceRetainedBranchDiverged(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(f.project, 3, "fix", "trunk", true, false)
	workOK(t, err)
	local := workspaceCommit(t, tree, "local", "local change")
	fixtureGit(t, f.source, "switch", "-c", "task/3-fix")
	published := workspaceCommit(t, f.source, "published", "another change")
	fixtureGit(t, f.source, "push", "origin", "task/3-fix")
	fixtureGit(t, f.repo, "fetch", "origin")
	// Compare the diagnostic to the installed Git, including its advice text.
	cmd := exec.Command("git", "merge", "--ff-only", "refs/remotes/origin/task/3-fix")
	cmd.Dir = tree
	output, err := cmd.CombinedOutput()
	requireGitExit(t, err, 128)
	workOK(t, RemoveTaskWorkspace(f.project, 3, "fix", false))
	_, err = CreateTaskWorkspace(f.project, 3, "fix", "trunk", true, true)
	requireGitExit(t, err, 128)
	workEqual(t, err.Error(), "git: exit status 128: "+strings.TrimSpace(string(output)))
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "task/3-fix"), local)
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "origin/task/3-fix"), published)
	_, err = os.Stat(tree)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
}

func TestWorkspaceReviewRounds(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.source, "checkout", "-b", "task/1-change")
	first := workspaceCommit(t, f.source, "patch", "first\n")
	second := workspaceCommit(t, f.source, "patch", "second\n")
	// Advance base independently: review diffs start at the merge base.
	fixtureGit(t, f.source, "checkout", "trunk")
	workspaceCommit(t, f.source, "upstream", "unrelated\n")
	fixtureGit(t, f.source, "push", "origin", "--all")
	for _, head := range []string{first, second} {
		review, err := PrepareReviewWorkspace(f.project, 1, head, "trunk", true)
		workOK(t, err)
		workEqual(t, review, filepath.Join(f.project, "review", "1"))
		workEqual(t, fixtureGit(t, review, "rev-parse", "HEAD"), head)
		workEqual(t, fixtureGit(t, review, "branch", "--show-current"), "")
		diff, err := os.ReadFile(filepath.Join(review, reviewDiff))
		workOK(t, err)
		workEqual(t, string(diff), fixtureGit(t, f.source, "diff", "trunk..."+head)+"\n")
	}
	workOK(t, RemoveReviewWorkspace(f.project, 1))
	_, err := os.Stat(filepath.Join(f.project, "review", "1"))
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	workEqual(t, fixtureGit(t, f.repo, "worktree", "list", "--porcelain"), "worktree "+filepath.ToSlash(f.repo)+"\nHEAD "+fixtureGit(t, f.repo, "rev-parse", "HEAD")+"\nbranch refs/heads/trunk\n")
}

func requireGitExit(t *testing.T, err error, code int) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected git exit %d, got %v", code, err)
	}
	workEqual(t, exit.ExitCode(), code)
}

func TestWorkspaceClonesTheBaseBranch(t *testing.T) {
	f := workspaceSetup(t)
	upstream := filepath.Join(filepath.Dir(f.source), "origin.git")
	fixtureGit(t, f.source, "switch", "-c", "default")
	other := workspaceCommit(t, f.source, "default", "default branch\n")
	fixtureGit(t, f.source, "push", "origin", "default")
	fixtureGit(t, upstream, "symbolic-ref", "HEAD", "refs/heads/default")
	project := filepath.Join(t.TempDir(), "group")
	repo, err := EnsureGroupWorkspace(project, upstream, "trunk")
	workOK(t, err)
	workEqual(t, fixtureGit(t, repo, "branch", "--show-current"), "trunk")
	workEqual(t, fixtureGit(t, repo, "rev-parse", "HEAD"), fixtureGit(t, f.repo, "rev-parse", "HEAD"))
	workEqual(t, fixtureGit(t, repo, "rev-parse", "origin/default"), other)
}

func TestWorkspaceGitFailures(t *testing.T) {
	f := workspaceSetup(t)
	head := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	_, err := EnsureGroupWorkspace(filepath.Join(t.TempDir(), "group"), filepath.Join(f.project, "absent"), "trunk")
	requireGitExit(t, err, 128)
	review, err := PrepareReviewWorkspace(f.project, 1, head, "trunk", false)
	workOK(t, err)
	_, err = PrepareReviewWorkspace(f.project, 1, "absent", "trunk", false)
	requireGitExit(t, err, 128)
	workEqual(t, fixtureGit(t, review, "rev-parse", "HEAD"), head)
	_, err = PrepareReviewWorkspace(f.project, 1, head, "missing", false)
	requireGitExit(t, err, 128)
	err = RemoveReviewWorkspace(f.project, 2)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	workOK(t, os.Rename(filepath.Join(review, ".git"), filepath.Join(review, "saved-git")))
	requireGitExit(t, RemoveReviewWorkspace(f.project, 1), 128)
	_, err = CreateTaskWorkspace(f.project, 1, "bad\x00slug", "trunk", true, true)
	workEqual(t, errors.Is(err, os.ErrInvalid) || errors.Is(err, syscall.EINVAL), true)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	requireGitExit(t, RefreshGroupWorkspace(f.project, "trunk", true), 128)
	_, err = CreateTaskWorkspace(f.project, 1, "fix", "trunk", true, false)
	requireGitExit(t, err, 128)
	_, err = PrepareReviewWorkspace(f.project, 1, head, "trunk", true)
	requireGitExit(t, err, 128)
}
