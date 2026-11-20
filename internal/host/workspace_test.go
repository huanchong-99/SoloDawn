package host

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	project := filepath.Join(dir, "projects", "group")
	fixtureGit(t, dir, "init", "-b", "trunk", source)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "base")
	upstream := filepath.Join(dir, "origin.git")
	fixtureGit(t, dir, "clone", "--bare", source, upstream)
	fixtureGit(t, source, "remote", "add", "origin", upstream)
	repo, err := EnsureGroupWorkspace(project, upstream)
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
	again, err := EnsureGroupWorkspace(f.project, "unused")
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
	workOK(t, RemoveTaskWorkspace(f.project, 2, "docs"))
	_, err = os.Stat(tree)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	resumed, err := CreateTaskWorkspace(f.project, 2, "docs", "trunk", false, true)
	workOK(t, err)
	workEqual(t, resumed, tree)
	workEqual(t, fixtureGit(t, resumed, "branch", "--show-current"), "task/2-docs")
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), head)
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

func TestWorkspaceProtectsSubmittedDiffPath(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	head := workspaceCommit(t, f.repo, strings.ToUpper(reviewDiff), "tracked content")
	_, err := PrepareReviewWorkspace(f.project, 5, head, "trunk", false)
	workEqual(t, errors.Is(err, fs.ErrExist), true)
	data, err := os.ReadFile(filepath.Join(f.repo, strings.ToUpper(reviewDiff)))
	workOK(t, err)
	workEqual(t, string(data), "tracked content")
	_, err = os.Stat(filepath.Join(f.project, "review"))
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
}

func TestWorkspacePathErrors(t *testing.T) {
	project := t.TempDir()
	repo := filepath.Join(project, "repo")
	workOK(t, os.WriteFile(repo, nil, 0600))
	_, err := EnsureGroupWorkspace(project, "unused")
	workEqual(t, err.Error(), "expected a workspace directory at "+repo)
	for _, slug := range []string{"", "../other", `x\y`} {
		_, err := CreateTaskWorkspace(project, 1, slug, "trunk", false, false)
		workEqual(t, err.Error(), "invalid task workspace slug "+strconv.Quote(slug))
		workEqual(t, RemoveTaskWorkspace(project, 1, slug).Error(), err.Error())
	}
	badDir := filepath.Join(project, "bad\x00dir")
	_, err = EnsureGroupWorkspace(badDir, "unused")
	var pathErr *os.PathError
	workEqual(t, errors.As(err, &pathErr), true)
	workEqual(t, pathErr.Path, filepath.Join(badDir, "repo"))
	_, err = PrepareReviewWorkspace(filepath.Join(project, "missing"), 2, "HEAD", "trunk", false)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
}

func requireGitExit(t *testing.T, err error, code int) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected git exit %d, got %v", code, err)
	}
	workEqual(t, exit.ExitCode(), code)
}

func TestWorkspaceGitFailures(t *testing.T) {
	f := workspaceSetup(t)
	head := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	review, err := PrepareReviewWorkspace(f.project, 1, head, "trunk", false)
	workOK(t, err)
	_, err = PrepareReviewWorkspace(f.project, 1, head, "missing", false)
	requireGitExit(t, err, 128)
	_, err = PrepareReviewWorkspace(f.project, 1, "absent", "trunk", false)
	requireGitExit(t, err, 128)
	workEqual(t, fixtureGit(t, review, "rev-parse", "HEAD"), head)
	diff, err := os.ReadFile(filepath.Join(review, reviewDiff))
	workOK(t, err)
	workEqual(t, string(diff), "")
	workOK(t, os.Rename(filepath.Join(review, ".git"), filepath.Join(review, "saved-git")))
	_, err = PrepareReviewWorkspace(f.project, 1, head, "trunk", false)
	requireGitExit(t, err, 128)
	requireGitExit(t, RemoveReviewWorkspace(f.project, 1), 128)
	workOK(t, os.WriteFile(filepath.Join(f.project, "review", "2"), nil, 0600))
	_, err = PrepareReviewWorkspace(f.project, 2, head, "trunk", false)
	workEqual(t, err.Error(), "expected a workspace directory at "+filepath.Join(f.project, "review", "2"))
	_, err = CreateTaskWorkspace(f.project, 1, "bad\x00slug", "trunk", true, true)
	workEqual(t, errors.Is(err, os.ErrInvalid) || errors.Is(err, syscall.EINVAL), true)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	requireGitExit(t, RefreshGroupWorkspace(f.project, "trunk", true), 128)
	_, err = CreateTaskWorkspace(f.project, 1, "fix", "trunk", true, false)
	requireGitExit(t, err, 128)
	_, err = PrepareReviewWorkspace(f.project, 1, head, "trunk", true)
	requireGitExit(t, err, 128)
}
