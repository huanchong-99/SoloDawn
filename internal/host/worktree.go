package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

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
