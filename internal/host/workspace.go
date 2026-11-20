package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// cloneLocks holds one lock per clone. Every Git command that changes a clone
// or one of its worktrees runs under it: clone, fetch, merge (the task merge
// of merge.go included), worktree add and remove (local.md section 7).
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

// slot is one folder for a worktree of the clone, wt/<num>-<slug> or
// review/<num>, and the AI that is to hold it.
type slot struct {
	repo   string // the clone
	path   string // the folder
	rel    string // the folder relative to repo, as worktree commands take it
	branch string // refs/heads/<br>, or "" for a detached review checkout
	ai     string // the member id of the AI that holds the folder
}

func newSlot(projectDir, kind, name, branch, ai string) slot {
	return slot{
		repo:   filepath.Join(projectDir, "repo"),
		path:   filepath.Join(projectDir, kind, name),
		rel:    filepath.Join("..", kind, name),
		branch: branch,
		ai:     ai,
	}
}

// worktreeEntry is what `git worktree list` records for a slot's folder.
type worktreeEntry struct {
	found, initializing bool
	branch              string
}

// entry prunes the records of worktrees whose folder is gone, then reads the
// record of s's folder.
func (s slot) entry(ctx context.Context) (worktreeEntry, error) {
	_, pruneErr := workspaceGit(ctx, s.repo, "worktree", "prune")
	out, listErr := workspaceGit(ctx, s.repo, "worktree", "list", "--porcelain", "-z")
	if err := errors.Join(pruneErr, listErr); err != nil {
		return worktreeEntry{}, err
	}
	want := "worktree " + filepath.ToSlash(realPath(s.path))
	for block := range strings.SplitSeq(out, "\x00\x00") {
		if lines := strings.Split(block, "\x00"); lines[0] == want {
			return parseEntry(lines[1:]), nil
		}
	}
	return worktreeEntry{}, nil
}

func parseEntry(lines []string) worktreeEntry {
	entry := worktreeEntry{found: true}
	for _, line := range lines {
		if branch, ok := strings.CutPrefix(line, "branch "); ok {
			entry.branch = branch
		}
		entry.initializing = entry.initializing || line == "locked initializing"
	}
	return entry
}

// holderFile is kept in the worktree's own admin folder, never in the
// checkout, so it goes away with the worktree.
const holderFile = "solodawn-holder"

// holder is the member id recorded for s's folder, or "" without a record.
func (s slot) holder(ctx context.Context) string {
	file, _ := workspaceGit(ctx, s.path, "rev-parse", "--path-format=absolute", "--git-path", holderFile)
	held, _ := os.ReadFile(strings.TrimSpace(file))
	return string(held)
}

// record writes s.ai as the holder of the worktree just added at s.path.
func (s slot) record(ctx context.Context) error {
	file, err := workspaceGit(ctx, s.path, "rev-parse", "--path-format=absolute", "--git-path", holderFile)
	if err == nil {
		err = os.WriteFile(strings.TrimSpace(file), []byte(s.ai), 0o600)
	}
	return err
}

// held is whether s's folder can be reused as it is: a worktree on s's
// branch (or detached, for a review), not left initializing by a stopped
// add, and recorded for the same AI.
func (s slot) held(ctx context.Context, entry worktreeEntry) bool {
	return entry.found && !entry.initializing && entry.branch == s.branch && s.holder(ctx) == s.ai
}

// claim reports whether s's folder is reused as it is (local.md section 7,
// "A worktree left by a crash"). Anything else there is deleted, so that the
// caller adds the worktree again; only commits carry over. For a folder
// that is still registered, the add needs the returned --force --force,
// which replaces the record even when it is locked or its .git file is gone.
func (s slot) claim(ctx context.Context) (reuse bool, force []string, err error) {
	entry, err := s.entry(ctx)
	if err != nil {
		return false, nil, err
	}
	if s.held(ctx, entry) {
		return true, nil, nil
	}
	if entry.found {
		force = []string{"--force", "--force"}
	}
	return false, force, os.RemoveAll(s.path)
}

// remove removes s's folder unless another AI holds it. A folder that is no
// longer a worktree is deleted.
func (s slot) remove(ctx context.Context, force ...string) error {
	entry, err := s.entry(ctx)
	if err != nil {
		return err
	}
	if !entry.found {
		return os.RemoveAll(s.path)
	}
	if held := s.holder(ctx); held != "" && held != s.ai {
		return nil
	}
	_, err = workspaceGit(ctx, s.repo, append(append([]string{"worktree", "remove"}, force...), s.rel)...)
	return err
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

func (s slot) addTask(ctx context.Context, start string, remote bool, force []string) error {
	name := strings.TrimPrefix(s.branch, "refs/heads/")
	published := "refs/remotes/origin/" + name
	// An unreadable list adds a new branch, and that add reports the failure.
	out, _ := workspaceGit(ctx, s.repo, "for-each-ref", "--format=%(refname)", s.branch, published)
	refs := strings.Split(out, "\n")
	follow := remote && slices.Contains(refs, published)
	if slices.Contains(refs, s.branch) {
		return s.resumeTask(ctx, published, follow, worktreeAdd(force, s.rel, name))
	}
	if follow {
		start = published
	}
	_, err := workspaceGit(ctx, s.repo, worktreeAdd(force, "-b", name, s.rel, start)...)
	return err
}

// worktreeAdd is `git worktree add` with force, if any, and then args.
func worktreeAdd(force []string, args ...string) []string {
	return append(append([]string{"worktree", "add"}, force...), args...)
}

// resumeTask adds the branch this node already has and fast-forwards it to
// the published one when follow is set; a diverged branch fails with Git's
// output and leaves no worktree behind.
func (s slot) resumeTask(ctx context.Context, published string, follow bool, add []string) error {
	if _, err := workspaceGit(ctx, s.repo, add...); err != nil || !follow {
		return err
	}
	if _, err := workspaceGit(ctx, s.path, "merge", "--ff-only", published); err != nil {
		_, removeErr := workspaceGit(ctx, s.repo, "worktree", "remove", "--force", s.rel)
		return errors.Join(err, removeErr)
	}
	return nil
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

// checkoutReview forces the checkout: a review checkout holds no one's work,
// and the untracked diff must not stop it.
func (s slot) checkoutReview(ctx context.Context, commit string) error {
	reuse, force, err := s.claim(ctx)
	if err != nil {
		return err
	}
	if reuse {
		_, err = workspaceGit(ctx, s.path, "checkout", "--force", "--detach", commit)
		return err
	}
	if _, err := workspaceGit(ctx, s.repo, worktreeAdd(force, "--detach", s.rel, commit)...); err != nil {
		return err
	}
	return s.record(ctx)
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
	var force []string
	if why != TaskMerged {
		force = []string{"--force", "--force"}
	}
	return newSlot(projectDir, "wt", name, "", ai).remove(ctx, force...)
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
