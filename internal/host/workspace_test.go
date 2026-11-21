package host

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The member ids of two AIs on one node.
const (
	aiOne = "ai-one"
	aiTwo = "ai-two"
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
	repo, err := EnsureGroupWorkspace(t.Context(), project, upstream, "trunk")
	workOK(t, err)
	workEqual(t, repo, filepath.Join(project, "repo"))
	return workspaceFixture{project, source, repo}
}

func (f workspaceFixture) upstream() string {
	return filepath.Join(filepath.Dir(f.source), "origin.git")
}

func workspaceCommit(t *testing.T, dir, file, content string) string {
	t.Helper()
	workOK(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0644))
	fixtureGit(t, dir, "add", "--", file)
	fixtureGit(t, dir, "commit", "-m", "update "+file)
	return fixtureGit(t, dir, "rev-parse", "HEAD")
}

func requireAbsent(t *testing.T, path string) {
	t.Helper()
	_, err := os.Lstat(path)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
}

func requireFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	workOK(t, err)
	workEqual(t, string(data), want)
}

func requireGitExit(t *testing.T, err error, code int) {
	t.Helper()
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		t.Fatalf("expected git exit %d, got %v", code, err)
	}
	workEqual(t, exit.ExitCode(), code)
}

// worktreeList is the clone's worktrees as `git worktree list --porcelain` prints them.
func worktreeList(t *testing.T, repo string) string {
	t.Helper()
	return fixtureGit(t, repo, "worktree", "list", "--porcelain")
}

func onlyTheClone(t *testing.T, f workspaceFixture) string {
	t.Helper()
	return "worktree " + filepath.ToSlash(f.repo) + "\nHEAD " + fixtureGit(t, f.repo, "rev-parse", "HEAD") + "\nbranch refs/heads/trunk\n"
}

// holderOf reads the holder record of the worktree at path.
func holderOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(fixtureGit(t, path, "rev-parse", "--path-format=absolute", "--git-path", holderFile))
	workOK(t, err)
	return string(data)
}

func TestWorkspaceCloneAndRemoteBase(t *testing.T) {
	f := workspaceSetup(t)
	old := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	newHead := workspaceCommit(t, f.source, "upstream", "new\n")
	fixtureGit(t, f.source, "push", "origin", "trunk")
	again, err := EnsureGroupWorkspace(t.Context(), f.project, "unused", "unused")
	workOK(t, err)
	workEqual(t, again, f.repo)
	workEqual(t, fixtureGit(t, again, "rev-parse", "HEAD"), old)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 12, "fix", aiOne, "trunk", true)
	workOK(t, err)
	workEqual(t, tree, filepath.Join(f.project, "wt", "12-fix"))
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/12-fix")
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), newHead)
	workEqual(t, holderOf(t, tree), aiOne)
	// The record is in the worktree's admin folder, not in the checkout.
	workEqual(t, fixtureGit(t, tree, "status", "--porcelain", "--untracked-files=all", "--ignored"), "")
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "HEAD"), old)
	workOK(t, RefreshGroupWorkspace(t.Context(), f.project, "trunk", true))
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "HEAD"), newHead)
	workOK(t, RefreshGroupWorkspace(t.Context(), filepath.Join(f.project, "absent"), "trunk", false))
}

func TestWorkspaceClonesTheBaseBranch(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.source, "switch", "-c", "default")
	other := workspaceCommit(t, f.source, "default", "default branch\n")
	fixtureGit(t, f.source, "push", "origin", "default")
	fixtureGit(t, f.upstream(), "symbolic-ref", "HEAD", "refs/heads/default")
	project := filepath.Join(t.TempDir(), "group")
	repo, err := EnsureGroupWorkspace(t.Context(), project, f.upstream(), "trunk")
	workOK(t, err)
	workEqual(t, fixtureGit(t, repo, "branch", "--show-current"), "trunk")
	workEqual(t, fixtureGit(t, repo, "rev-parse", "HEAD"), fixtureGit(t, f.repo, "rev-parse", "HEAD"))
	workEqual(t, fixtureGit(t, repo, "rev-parse", "origin/default"), other)
	requireAbsent(t, filepath.Join(project, cloneTemp))
}

// M3 (d): a stopped clone leaves its temporary folder, never a repo folder.
func TestWorkspaceCloneAfterAStoppedClone(t *testing.T) {
	f := workspaceSetup(t)
	project := filepath.Join(t.TempDir(), "group")
	temp := filepath.Join(project, cloneTemp)
	workOK(t, os.MkdirAll(filepath.Join(temp, "objects"), 0o755))
	workOK(t, os.WriteFile(filepath.Join(temp, "objects", "partial"), []byte("half a pack"), 0o644))
	repo, err := EnsureGroupWorkspace(t.Context(), project, f.upstream(), "trunk")
	workOK(t, err)
	workEqual(t, fixtureGit(t, repo, "branch", "--show-current"), "trunk")
	requireAbsent(t, temp)
	// A failed clone leaves no repo folder behind.
	failed := filepath.Join(t.TempDir(), "failed")
	_, err = EnsureGroupWorkspace(t.Context(), failed, filepath.Join(f.project, "absent"), "trunk")
	requireGitExit(t, err, 128)
	requireAbsent(t, filepath.Join(failed, "repo"))
}

// M3 (d): an existing repo that is not a Git repository is an error and is kept.
func TestWorkspaceRefusesARepoFolderThatIsNotAClone(t *testing.T) {
	project := t.TempDir()
	repo := filepath.Join(project, "repo")
	workOK(t, os.MkdirAll(repo, 0o755))
	got, err := EnsureGroupWorkspace(t.Context(), project, "unused", "trunk")
	workEqual(t, got, repo)
	workEqual(t, err.Error(), repo+" is not a Git repository")
	info, err := os.Stat(repo)
	workOK(t, err)
	workEqual(t, info.IsDir(), true)
}

func TestWorkspaceLocalReassignment(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	workOK(t, RefreshGroupWorkspace(t.Context(), f.project, "trunk", false))
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 2, "docs", aiOne, "trunk", false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "guide", "draft")
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 2, "docs", aiOne, TaskGivenAway))
	requireAbsent(t, tree)
	resumed, err := CreateTaskWorkspace(t.Context(), f.project, 2, "docs", aiTwo, "trunk", false)
	workOK(t, err)
	workEqual(t, resumed, tree)
	workEqual(t, fixtureGit(t, resumed, "branch", "--show-current"), "task/2-docs")
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), head)
	workEqual(t, holderOf(t, resumed), aiTwo)
}

func TestWorkspaceCancelDiscardsUncommittedChanges(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 5, "draft", aiOne, "trunk", false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "kept", "committed\n")
	workOK(t, os.WriteFile(filepath.Join(tree, "kept"), []byte("edited\n"), 0644))
	workOK(t, os.WriteFile(filepath.Join(tree, "loose"), []byte("untracked\n"), 0644))
	requireGitExit(t, RemoveTaskWorkspace(t.Context(), f.project, 5, "draft", aiOne, TaskMerged), 128)
	requireFile(t, filepath.Join(tree, "kept"), "edited\n")
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 5, "draft", aiOne, TaskCancelled))
	requireAbsent(t, tree)
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "task/5-draft"), head)
	// Removing what is already gone is not an error: the item may come again.
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 5, "draft", aiOne, TaskCancelled))
}

func TestWorkspaceRemoteReassignment(t *testing.T) {
	f := workspaceSetup(t)
	base := workspaceCommit(t, f.source, "base", "updated")
	fixtureGit(t, f.source, "checkout", "-b", "task/3-fix")
	published := workspaceCommit(t, f.source, "progress", "published")
	fixtureGit(t, f.source, "push", "origin", "--all")
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
	workOK(t, err)
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), published)
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/3-fix")
	tree, err = CreateTaskWorkspace(t.Context(), f.project, 4, "fix", aiOne, "trunk", true)
	workOK(t, err)
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), base)
}

// Without a remote, a stale origin/<br> of the clone is never followed.
func TestWorkspaceWithoutARemoteIgnoresOriginBranches(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.source, "checkout", "-b", "task/3-fix")
	workspaceCommit(t, f.source, "progress", "published")
	fixtureGit(t, f.source, "push", "origin", "task/3-fix")
	fixtureGit(t, f.repo, "fetch", "origin")
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), fixtureGit(t, f.repo, "rev-parse", "trunk"))
}

func TestWorkspaceRetainedBranchFastForward(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
	workOK(t, err)
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, TaskGivenAway))
	fixtureGit(t, f.source, "switch", "-c", "task/3-fix")
	newHead := workspaceCommit(t, f.source, "progress", "published")
	fixtureGit(t, f.source, "push", "origin", "task/3-fix")
	resumed, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
	workOK(t, err)
	workEqual(t, resumed, tree)
	workEqual(t, fixtureGit(t, resumed, "branch", "--show-current"), "task/3-fix")
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), newHead)
}

func TestWorkspaceRetainedBranchDiverged(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
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
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, TaskGivenAway))
	_, err = CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
	requireGitExit(t, err, 128)
	workEqual(t, err.Error(), "git: exit status 128: "+strings.TrimSpace(string(output)))
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "task/3-fix"), local)
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "origin/task/3-fix"), published)
	requireAbsent(t, tree)
	workEqual(t, worktreeList(t, f.repo), onlyTheClone(t, f))
}

func TestWorkspaceRetainedBranchWithoutPublishedBranch(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
	workOK(t, err)
	local := workspaceCommit(t, tree, "local", "never pushed\n")
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, TaskGivenAway))
	resumed, err := CreateTaskWorkspace(t.Context(), f.project, 3, "fix", aiOne, "trunk", true)
	workOK(t, err)
	workEqual(t, resumed, tree)
	workEqual(t, fixtureGit(t, resumed, "branch", "--show-current"), "task/3-fix")
	workEqual(t, fixtureGit(t, resumed, "rev-parse", "HEAD"), local)
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
		review, err := PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, head, "trunk", true)
		workOK(t, err)
		workEqual(t, review, filepath.Join(f.project, "review", "1"))
		workEqual(t, fixtureGit(t, review, "rev-parse", "HEAD"), head)
		workEqual(t, fixtureGit(t, review, "branch", "--show-current"), "")
		workEqual(t, holderOf(t, review), aiOne)
		requireFile(t, filepath.Join(review, reviewDiff), fixtureGit(t, f.source, "diff", "trunk..."+head)+"\n")
	}
	workOK(t, RemoveReviewWorkspace(t.Context(), f.project, 1, aiOne))
	requireAbsent(t, filepath.Join(f.project, "review", "1"))
	workEqual(t, worktreeList(t, f.repo), onlyTheClone(t, f))
}

// L2: the review diff is Git's own diff, whatever the owner's diff settings.
func TestWorkspaceReviewDiffIgnoresDiffSettings(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 1, "change", aiOne, "trunk", false)
	workOK(t, err)
	workOK(t, os.WriteFile(filepath.Join(tree, ".gitattributes"), []byte("* diff=shout\n"), 0644))
	commit := workspaceCommit(t, tree, "patch", "lower case\n")
	want := fixtureGit(t, f.repo, "-c", "core.attributesFile="+os.DevNull, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "trunk..."+commit) + "\n"
	fixtureGit(t, f.repo, "config", "diff.external", "echo external")
	fixtureGit(t, f.repo, "config", "diff.shout.textconv", "tr a-z A-Z <")
	fixtureGit(t, f.repo, "config", "color.ui", "always")
	review, err := PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, commit, "trunk", false)
	workOK(t, err)
	requireFile(t, filepath.Join(review, reviewDiff), want)
}

// L7: a commit that tracks the reserved diff name does not stop the next round.
func TestWorkspaceReviewOfACommitThatTracksTheDiffName(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 1, "change", aiOne, "trunk", false)
	workOK(t, err)
	first := workspaceCommit(t, tree, "patch", "first\n")
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 1, aiTwo, first, "trunk", false)
	workOK(t, err)
	second := workspaceCommit(t, tree, reviewDiff, "tracked by the project\n")
	review, err := PrepareReviewWorkspace(t.Context(), f.project, 1, aiTwo, second, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, review, "rev-parse", "HEAD"), second)
	requireFile(t, filepath.Join(review, reviewDiff), fixtureGit(t, f.repo, "diff", "trunk..."+second)+"\n")
}

// M10: a slug the home cannot produce, or a num below 1, builds no path.
func TestWorkspaceRefusesSlugsTheHomeCannotProduce(t *testing.T) {
	f := workspaceSetup(t)
	long := strings.Repeat("a", 41)
	for _, c := range []struct {
		num  int
		slug string
	}{
		{1, "x/../../../../outside"}, {1, ""}, {1, "Upper"}, {1, "-lead"}, {1, "trail-"}, {1, "two--dashes"},
		{1, "dot.ted"}, {1, long}, {0, "fix"}, {-1, "fix"},
	} {
		want := "invalid task " + strconv.Itoa(c.num) + `-"` + c.slug + `": a task number is at least 1 and a slug matches ^[a-z0-9]+(-[a-z0-9]+)*$ in at most 40 characters`
		_, err := CreateTaskWorkspace(t.Context(), f.project, c.num, c.slug, aiOne, "trunk", false)
		workEqual(t, err.Error(), want)
		workEqual(t, RemoveTaskWorkspace(t.Context(), f.project, c.num, c.slug, aiOne, TaskCancelled).Error(), want)
	}
	requireAbsent(t, filepath.Join(f.project, "wt"))
	requireAbsent(t, filepath.Join(filepath.Dir(f.project), "outside"))
	for _, num := range []int{0, -1} {
		want := "invalid task number " + strconv.Itoa(num)
		_, err := PrepareReviewWorkspace(t.Context(), f.project, num, aiOne, "HEAD", "trunk", false)
		workEqual(t, err.Error(), want)
		workEqual(t, RemoveReviewWorkspace(t.Context(), f.project, num, aiOne).Error(), want)
	}
	requireAbsent(t, filepath.Join(f.project, "review"))
	// The limits themselves are accepted.
	limit := strings.Repeat("b", 39) + "9"
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 1, limit, aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, tree, filepath.Join(f.project, "wt", "1-"+limit))
	_, err = CreateTaskWorkspace(t.Context(), f.project, 2, "a-1-b", aiOne, "trunk", false)
	workOK(t, err)
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, "HEAD", "trunk", false)
	workOK(t, err)
}

// M4: the next holder's create can run before the old holder's remove.
func TestWorkspaceTaskHandedToAnotherAIOnTheSameNode(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 6, "away", aiOne, "trunk", false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "kept", "committed\n")
	workOK(t, os.WriteFile(filepath.Join(tree, "unsaved"), []byte("first AI's work\n"), 0644))
	again, err := CreateTaskWorkspace(t.Context(), f.project, 6, "away", aiTwo, "trunk", false)
	workOK(t, err)
	workEqual(t, again, tree)
	requireAbsent(t, filepath.Join(tree, "unsaved"))
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), head)
	workEqual(t, holderOf(t, tree), aiTwo)
	workOK(t, os.WriteFile(filepath.Join(tree, "unsaved"), []byte("second AI's work\n"), 0644))
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 6, "away", aiOne, TaskGivenAway))
	requireFile(t, filepath.Join(tree, "unsaved"), "second AI's work\n")
	workEqual(t, holderOf(t, tree), aiTwo)
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 6, "away", aiTwo, TaskGivenAway))
	requireAbsent(t, tree)
	workEqual(t, fixtureGit(t, f.repo, "rev-parse", "task/6-away"), head)
}

// M4 for reviews: a new reviewer on the same node gets its own checkout.
func TestWorkspaceReviewerChangedOnTheSameNode(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 7, "both", aiOne, "trunk", false)
	workOK(t, err)
	commit := workspaceCommit(t, tree, "change", "submitted\n")
	review, err := PrepareReviewWorkspace(t.Context(), f.project, 7, aiOne, commit, "trunk", false)
	workOK(t, err)
	workOK(t, os.WriteFile(filepath.Join(review, "notes"), []byte("old reviewer\n"), 0644))
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 7, aiTwo, commit, "trunk", false)
	workOK(t, err)
	requireAbsent(t, filepath.Join(review, "notes"))
	workEqual(t, holderOf(t, review), aiTwo)
	workOK(t, RemoveReviewWorkspace(t.Context(), f.project, 7, aiOne))
	workEqual(t, holderOf(t, review), aiTwo)
	workOK(t, RemoveReviewWorkspace(t.Context(), f.project, 7, aiTwo))
	requireAbsent(t, review)
	// The task's own worktree is another role's and stays.
	workEqual(t, fixtureGit(t, tree, "rev-parse", "HEAD"), commit)
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/7-both")
}

// A worktree with no holder record (stopped before the record) is added
// again on create, and removed by any AI on remove.
func TestWorkspaceWithoutAHolderRecord(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 8, "bare", aiOne, "trunk", false)
	workOK(t, err)
	record := fixtureGit(t, tree, "rev-parse", "--path-format=absolute", "--git-path", holderFile)
	workOK(t, os.Remove(record))
	workOK(t, os.WriteFile(filepath.Join(tree, "stray"), []byte("left\n"), 0644))
	_, err = CreateTaskWorkspace(t.Context(), f.project, 8, "bare", aiOne, "trunk", false)
	workOK(t, err)
	requireAbsent(t, filepath.Join(tree, "stray"))
	workOK(t, os.Remove(record))
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 8, "bare", aiTwo, TaskCancelled))
	requireAbsent(t, tree)
}

// The same AI's worktree is reused as it is, with its uncommitted work.
func TestWorkspaceReusesTheSameAIsWorktree(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 4, "crash", aiOne, "trunk", true)
	workOK(t, err)
	head := workspaceCommit(t, tree, "saved", "committed\n")
	workOK(t, os.WriteFile(filepath.Join(tree, "unsaved"), []byte("in progress\n"), 0644))
	again, err := CreateTaskWorkspace(t.Context(), f.project, 4, "crash", aiOne, "trunk", true)
	workOK(t, err)
	workEqual(t, again, tree)
	workEqual(t, fixtureGit(t, again, "rev-parse", "HEAD"), head)
	requireFile(t, filepath.Join(again, "unsaved"), "in progress\n")
}

// M3 (c): a stop inside `worktree remove` leaves the record without the
// folder, or a half-deleted folder without its .git file.
func TestWorkspaceAfterAStoppedRemove(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 9, "gone", aiOne, "trunk", false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "kept", "committed\n")
	workOK(t, os.RemoveAll(tree))
	again, err := CreateTaskWorkspace(t.Context(), f.project, 9, "gone", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, again, "rev-parse", "HEAD"), head)
	workOK(t, os.Remove(filepath.Join(tree, ".git")))
	workOK(t, os.Remove(filepath.Join(tree, "kept")))
	again, err = CreateTaskWorkspace(t.Context(), f.project, 9, "gone", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, again, "rev-parse", "HEAD"), head)
	workEqual(t, fixtureGit(t, again, "status", "--porcelain", "--untracked-files=all"), "")
	workEqual(t, holderOf(t, again), aiOne)
	// The same leftovers on removal: the folder is deleted.
	workOK(t, os.Remove(filepath.Join(tree, ".git")))
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 9, "gone", aiOne, TaskMerged))
	requireAbsent(t, tree)
	workEqual(t, worktreeList(t, f.repo), onlyTheClone(t, f))
}

// M3 (a), as the stopped add leaves it: a worktree locked initializing, with
// only part of its files. The unix tests reproduce it with a real SIGKILL.
func TestWorkspaceAfterAStoppedAdd(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 2, "init", aiOne, "trunk", false)
	workOK(t, err)
	head := workspaceCommit(t, tree, "file", "content\n")
	fixtureGit(t, f.repo, "worktree", "lock", "--reason", "initializing", tree)
	workOK(t, os.Remove(filepath.Join(tree, "file")))
	again, err := CreateTaskWorkspace(t.Context(), f.project, 2, "init", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, again, "rev-parse", "HEAD"), head)
	workEqual(t, fixtureGit(t, again, "status", "--porcelain", "--untracked-files=all"), "")
	fixtureGit(t, f.repo, "worktree", "lock", "--reason", "initializing", tree)
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 2, "init", aiOne, TaskCancelled))
	requireAbsent(t, tree)
	// A worktree locked for another reason, with its .git file gone, is not
	// pruned; it has no readable record and is added again.
	_, err = CreateTaskWorkspace(t.Context(), f.project, 2, "init", aiOne, "trunk", false)
	workOK(t, err)
	fixtureGit(t, f.repo, "worktree", "lock", "--reason", "owner", tree)
	workOK(t, os.Remove(filepath.Join(tree, ".git")))
	again, err = CreateTaskWorkspace(t.Context(), f.project, 2, "init", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, holderOf(t, again), aiOne)
}

// M3 (b): an add that failed after it created the branch.
func TestWorkspaceAfterAFailedAddThatKeptTheBranch(t *testing.T) {
	f := workspaceSetup(t)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	fixtureGit(t, f.repo, "branch", "task/1-x", "trunk")
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 1, "x", aiOne, "trunk", false)
	workOK(t, err)
	workEqual(t, fixtureGit(t, tree, "branch", "--show-current"), "task/1-x")
}

func TestWorkspaceGitFailures(t *testing.T) {
	f := workspaceSetup(t)
	head := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	review, err := PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, head, "trunk", false)
	workOK(t, err)
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, "absent", "trunk", false)
	requireGitExit(t, err, 128)
	workEqual(t, fixtureGit(t, review, "rev-parse", "HEAD"), head)
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 2, aiOne, "absent", "trunk", false)
	requireGitExit(t, err, 128)
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, head, "missing", false)
	requireGitExit(t, err, 128)
	workOK(t, RemoveReviewWorkspace(t.Context(), f.project, 2, aiOne))
	// A folder whose clone is gone: every command reports Git's error.
	missing := filepath.Join(t.TempDir(), "group")
	_, err = CreateTaskWorkspace(t.Context(), missing, 1, "fix", aiOne, "trunk", false)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	workEqual(t, errors.Is(RemoveTaskWorkspace(t.Context(), missing, 1, "fix", aiOne, TaskCancelled), fs.ErrNotExist), true)
	_, err = PrepareReviewWorkspace(t.Context(), missing, 1, aiOne, head, "trunk", false)
	workEqual(t, errors.Is(err, fs.ErrNotExist), true)
	fixtureGit(t, f.repo, "remote", "remove", "origin")
	requireGitExit(t, RefreshGroupWorkspace(t.Context(), f.project, "trunk", true), 128)
	_, err = CreateTaskWorkspace(t.Context(), f.project, 1, "fix", aiOne, "trunk", true)
	requireGitExit(t, err, 128)
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 1, aiOne, head, "trunk", true)
	requireGitExit(t, err, 128)
}

func TestWorkspaceGitCommandsWaitForTheCloneLock(t *testing.T) {
	f := workspaceSetup(t)
	head := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	_, err := CreateTaskWorkspace(t.Context(), f.project, 8, "held", aiOne, "trunk", false)
	workOK(t, err)
	_, err = PrepareReviewWorkspace(t.Context(), f.project, 8, aiOne, head, "trunk", false)
	workOK(t, err)
	for _, c := range []struct {
		name string
		op   func() error
	}{
		{"clone", func() error { _, err := EnsureGroupWorkspace(t.Context(), f.project, "unused", "unused"); return err }},
		{"fetch and merge", func() error { return RefreshGroupWorkspace(t.Context(), f.project, "trunk", true) }},
		{"worktree add", func() error {
			_, err := CreateTaskWorkspace(t.Context(), f.project, 9, "next", aiOne, "trunk", false)
			return err
		}},
		{"review checkout", func() error {
			_, err := PrepareReviewWorkspace(t.Context(), f.project, 8, aiOne, head, "trunk", false)
			return err
		}},
		{"worktree remove", func() error { return RemoveTaskWorkspace(t.Context(), f.project, 8, "held", aiOne, TaskMerged) }},
		{"review remove", func() error { return RemoveReviewWorkspace(t.Context(), f.project, 8, aiOne) }},
	} {
		requireCloneLockWait(t, f.repo, c.name, c.op)
	}
}

// requireCloneLockWait holds the clone's lock, under another spelling of
// its path, and checks that op starts only after it is released. op runs on
// another goroutine, so it returns its error and asserts nothing itself.
func requireCloneLockWait(t *testing.T, repo, name string, op func() error) {
	t.Helper()
	lock := cloneLock(repo + string(filepath.Separator))
	lock.Lock()
	done := make(chan error, 1)
	go func() { done <- op() }()
	select {
	case err := <-done:
		lock.Unlock()
		t.Fatalf("%s ran while the clone was locked: %v", name, err)
	case <-time.After(100 * time.Millisecond):
	}
	lock.Unlock()
	workOK(t, <-done)
}

// L12: a path through a symbolic link names the same clone and its lock.
func TestCloneLockFollowsSymbolicLinks(t *testing.T) {
	f := workspaceSetup(t)
	link := filepath.Join(t.TempDir(), "link")
	workOK(t, os.Symlink(filepath.Dir(f.project), link))
	linked := filepath.Join(link, filepath.Base(f.project))
	requireCloneLockWait(t, filepath.Join(linked, "repo"), "a create through the real path", func() error {
		_, err := CreateTaskWorkspace(t.Context(), f.project, 1, "fix", aiOne, "trunk", false)
		return err
	})
	merge := ReviewedMerge{Repo: filepath.Join(linked, "repo"), Base: "trunk", Branch: "task/1-fix", Task: "t1", Commit: "absent"}
	var result MergeResult
	requireCloneLockWait(t, f.repo, "a merge through the link", func() (err error) {
		result, err = merge.Inspect(t.Context())
		return err
	})
	workEqual(t, result.Conflict, "no commit on trunk records Merge task t1 at absent")
	// A clone that does not exist yet keys the same as it will once it does.
	workEqual(t, cloneLock(filepath.Join(link, "new", "repo")), cloneLock(filepath.Join(filepath.Dir(f.project), "new", "repo")))
}

func TestCloneLocksAreSeparatePerClone(t *testing.T) {
	f := workspaceSetup(t)
	lock := cloneLock(f.repo)
	lock.Lock()
	defer lock.Unlock()
	other := filepath.Join(t.TempDir(), "other")
	repo, err := EnsureGroupWorkspace(t.Context(), other, f.upstream(), "trunk")
	workOK(t, err)
	workEqual(t, fixtureGit(t, repo, "branch", "--show-current"), "trunk")
}

func TestTaskMergeWaitsForTheCloneLock(t *testing.T) {
	f := workspaceSetup(t)
	tree, err := CreateTaskWorkspace(t.Context(), f.project, 10, "merge", aiOne, "trunk", false)
	workOK(t, err)
	commit := workspaceCommit(t, tree, "merged", "content\n")
	merge := ReviewedMerge{Repo: f.repo, Base: "trunk", Branch: "task/10-merge", Task: "t10", Commit: commit}
	var applied, inspected MergeResult
	requireCloneLockWait(t, f.repo, "merge", func() (err error) {
		applied, err = merge.Apply(t.Context())
		return err
	})
	head := fixtureGit(t, f.repo, "rev-parse", "HEAD")
	workEqual(t, applied, MergeResult{MergeCommit: head})
	requireCloneLockWait(t, f.repo, "merge inspection", func() (err error) {
		inspected, err = merge.Inspect(t.Context())
		return err
	})
	workEqual(t, inspected, MergeResult{MergeCommit: head})
	workOK(t, RemoveTaskWorkspace(t.Context(), f.project, 10, "merge", aiOne, TaskMerged))
	var deleted bool
	requireCloneLockWait(t, f.repo, "branch deletion", func() (err error) {
		deleted, err = merge.DeleteBranch(t.Context())
		return err
	})
	workEqual(t, deleted, true)
	workEqual(t, fixtureGit(t, f.repo, "for-each-ref", "--format=%(refname)", "refs/heads/task"), "")
}
