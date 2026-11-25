package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// cloneLocks holds one lock per clone. Every Git command that changes a clone
// or one of its worktrees runs under it: clone, fetch, push, merge (the task
// merge of merge.go included), worktree add and remove (local.md section 7).
var cloneLocks sync.Map

// cloneLock returns the lock of the clone at repo. It is keyed by the clone's
// real path, so two spellings of one clone share one lock.
func cloneLock(repo string) *sync.Mutex {
	lock, _ := cloneLocks.LoadOrStore(realPath(repo), new(sync.Mutex))
	return lock.(*sync.Mutex)
}

// realPath resolves the symbolic links of the longest existing prefix of
// path, so a folder has the same key before and after it exists.
func realPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Join(realPath(filepath.Dir(path)), filepath.Base(path))
}

// lockClone locks the clone of projectDir and returns its path.
func lockClone(projectDir string) (string, func()) {
	repo := filepath.Join(projectDir, "repo")
	lock := cloneLock(repo)
	lock.Lock()
	return repo, lock.Unlock
}

// cloneTemp is the sibling folder a clone is made in before it becomes repo.
const cloneTemp = ".repo-clone"

// EnsureGroupWorkspace returns the group's clone under projectDir
// (projects/<group>), cloning the base branch only if repo does not exist
// yet. The clone is made in a temporary sibling folder and renamed to repo
// once complete; a folder left by a stopped clone is removed first. An
// existing repo that is not a Git repository is an error, never deleted.
func EnsureGroupWorkspace(ctx context.Context, projectDir, source, base string) (string, error) {
	repo, unlock := lockClone(projectDir)
	defer unlock()
	if _, err := os.Stat(repo); err == nil {
		return repo, isClone(repo)
	}
	temp := filepath.Join(projectDir, cloneTemp)
	if err := os.RemoveAll(temp); err != nil {
		return "", err
	}
	if _, err := workspaceGit(ctx, "", "clone", "--branch", base, "--", source, temp); err != nil {
		return "", err
	}
	return repo, os.Rename(temp, repo)
}

func isClone(repo string) error {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return fmt.Errorf("%s is not a Git repository", repo)
	}
	return nil
}

// workspaceGit runs git through the host's command runner and returns its
// standard output, or an error that carries its standard error.
func workspaceGit(ctx context.Context, dir string, args ...string) (string, error) {
	out, stderr, err := runIn(ctx, dir, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git: %w: %s", err, strings.TrimSpace(stderr))
	}
	return out, nil
}

// workspaceBase returns <base-ref>, fetching first when the group has a remote.
func workspaceBase(ctx context.Context, repo, base string, remote bool) (string, error) {
	if !remote {
		return "refs/heads/" + base, nil
	}
	_, err := workspaceGit(ctx, repo, "fetch", "origin")
	return "refs/remotes/origin/" + base, err
}

// RefreshGroupWorkspace fast-forwards the group session's clone on a remote project.
func RefreshGroupWorkspace(ctx context.Context, projectDir, base string, remote bool) error {
	if !remote {
		return nil
	}
	repo, unlock := lockClone(projectDir)
	defer unlock()
	ref, err := workspaceBase(ctx, repo, base, remote)
	if err != nil {
		return err
	}
	_, err = workspaceGit(ctx, repo, "merge", "--ff-only", ref)
	return err
}

// slugRule is the form of a slug the home produces (local.md section 7, "Slug").
var slugRule = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// taskName is <num>-<slug>. A slug the home cannot produce, or a num below 1,
// is refused before any path or branch is built from it.
func taskName(num int, slug string) (string, error) {
	if num < 1 || len(slug) > 40 || !slugRule.MatchString(slug) {
		return "", fmt.Errorf("invalid task %d-%q: a task number is at least 1 and a slug matches %s in at most 40 characters", num, slug, slugRule)
	}
	return strconv.Itoa(num) + "-" + slug, nil
}

// CreateTaskWorkspace gives the AI ai the folder wt/<num>-<slug> on branch
// task/<num>-<slug> (local.md section 7). A worktree the same AI holds is
// reused as it is. Otherwise the folder is added again: on the branch if
// this node has it, fast-forwarded to origin/<br> when that exists; else as
// a new branch from origin/<br> if that exists, or from the base.
func CreateTaskWorkspace(ctx context.Context, projectDir string, num int, slug, ai, base string, remote bool) (string, error) {
	name, err := taskName(num, slug)
	if err != nil {
		return "", err
	}
	_, unlock := lockClone(projectDir)
	defer unlock()
	s := newSlot(projectDir, "wt", name, "refs/heads/task/"+name, ai)
	reuse, force, err := s.claim(ctx)
	if reuse || err != nil {
		return s.path, err
	}
	start, err := workspaceBase(ctx, s.repo, base, remote)
	if err != nil {
		return s.path, err
	}
	if err := s.addTask(ctx, start, remote, force); err != nil {
		return s.path, err
	}
	return s.path, s.record(ctx)
}

// reviewDiff is a reserved name in a review checkout (local.md section 7).
const reviewDiff = ".solodawn-review.diff"

// PrepareReviewWorkspace gives the reviewer ai review/<num>, checked out
// detached at the submitted commit, with the diff against the base written
// to .solodawn-review.diff. The review session opens this folder at
// permission level 1. A checkout the same reviewer holds is moved to the
// commit; any other is added again.
func PrepareReviewWorkspace(ctx context.Context, projectDir string, num int, ai, commit, base string, remote bool) (string, error) {
	s, err := reviewSlot(projectDir, num, ai)
	if err != nil {
		return "", err
	}
	_, unlock := lockClone(projectDir)
	defer unlock()
	ref, err := workspaceBase(ctx, s.repo, base, remote)
	if err != nil {
		return s.path, err
	}
	if err := s.checkoutReview(ctx, commit); err != nil {
		return s.path, err
	}
	_, err = workspaceGit(ctx, s.repo, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--output="+filepath.Join(s.path, reviewDiff), ref+"..."+commit)
	return s.path, err
}

// TaskRelease is why a node removes a task's worktree (local.md section 7).
type TaskRelease int

const (
	TaskMerged TaskRelease = iota
	TaskCancelled
	TaskGivenAway
)

// RemoveTaskWorkspace removes the worktree that ai holds and keeps its
// branch, which a later takeover on this node continues. A folder another
// AI holds is left untouched. A cancelled or given-away task's worktree is
// removed even when it has changes or a lock: only commits carry over.
func RemoveTaskWorkspace(ctx context.Context, projectDir string, num int, slug, ai string, why TaskRelease) error {
	name, err := taskName(num, slug)
	if err != nil {
		return err
	}
	_, unlock := lockClone(projectDir)
	defer unlock()
	return newSlot(projectDir, "wt", name, "", ai).release(ctx, why)
}

func (s slot) release(ctx context.Context, why TaskRelease) error {
	if why == TaskMerged {
		return s.remove(ctx)
	}
	return s.remove(ctx, "--force", "--force")
}

// RemoveReviewWorkspace removes review/<num> that the reviewer ai holds, on a
// merge, a cancel, a task given away or a change of reviewer alone; a
// checkout another AI holds is left untouched. It holds no one's work, so it
// is removed with --force --force.
func RemoveReviewWorkspace(ctx context.Context, projectDir string, num int, ai string) error {
	s, err := reviewSlot(projectDir, num, ai)
	if err != nil {
		return err
	}
	_, unlock := lockClone(projectDir)
	defer unlock()
	return s.remove(ctx, "--force", "--force")
}

// reviewSlot is review/<num>; a num below 1 is refused.
func reviewSlot(projectDir string, num int, ai string) (slot, error) {
	if num < 1 {
		return slot{}, fmt.Errorf("invalid task number %d", num)
	}
	return newSlot(projectDir, "review", strconv.Itoa(num), "", ai), nil
}
