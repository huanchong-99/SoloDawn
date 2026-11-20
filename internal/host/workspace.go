package host

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Two AIs of this node may join the same group at once; one of them clones.
var groupCloneMu sync.Mutex

// EnsureGroupWorkspace returns the group's clone under projectDir
// (projects/<group>), cloning the base branch only if the folder does not
// exist yet.
func EnsureGroupWorkspace(projectDir, source, base string) (string, error) {
	groupCloneMu.Lock()
	defer groupCloneMu.Unlock()
	repo := filepath.Join(projectDir, "repo")
	if _, err := os.Stat(repo); err == nil {
		return repo, nil
	}
	return repo, workspaceGit("", nil, "clone", "--branch", base, "--", source, repo)
}

func workspaceGit(dir string, stdout io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// workspaceBase returns <base-ref>, fetching first when the group has a remote.
func workspaceBase(repo, base string, remote bool) (string, error) {
	if !remote {
		return "refs/heads/" + base, nil
	}
	return "refs/remotes/origin/" + base, workspaceGit(repo, nil, "fetch", "origin")
}

// RefreshGroupWorkspace fast-forwards the group session's clone on a remote project.
func RefreshGroupWorkspace(projectDir, base string, remote bool) error {
	if !remote {
		return nil
	}
	repo := filepath.Join(projectDir, "repo")
	ref, err := workspaceBase(repo, base, remote)
	if err != nil {
		return err
	}
	return workspaceGit(repo, nil, "merge", "--ff-only", ref)
}

// CreateTaskWorkspace adds wt/<num>-<slug> on branch task/<num>-<slug>. A new
// task starts from the base; a task taken over from another AI continues its
// branch (local.md section 7).
func CreateTaskWorkspace(projectDir string, num int, slug, base string, remote, takeover bool) (string, error) {
	name := strconv.Itoa(num) + "-" + slug
	repo := filepath.Join(projectDir, "repo")
	start, err := workspaceBase(repo, base, remote)
	if err != nil {
		return "", err
	}
	path := filepath.Join("..", "wt", name)
	branch := "task/" + name
	return filepath.Join(projectDir, "wt", name), addTaskWorktree(repo, path, branch, start, remote, takeover)
}

func addTaskWorktree(repo, path, branch, base string, remote, takeover bool) error {
	if !takeover {
		return newTaskWorktree(repo, path, branch, base)
	}
	if !remote {
		return workspaceGit(repo, nil, "worktree", "add", path, branch)
	}
	local, published := "refs/heads/"+branch, "refs/remotes/origin/"+branch
	var out bytes.Buffer
	if err := workspaceGit(repo, &out, "for-each-ref", "--format=%(refname)", local, published); err != nil {
		return err
	}
	refs := strings.Split(out.String(), "\n")
	if slices.Contains(refs, local) {
		return resumeTaskWorktree(repo, path, branch, published)
	}
	if slices.Contains(refs, published) {
		base = published
	}
	return newTaskWorktree(repo, path, branch, base)
}

func newTaskWorktree(repo, path, branch, base string) error {
	return workspaceGit(repo, nil, "worktree", "add", "-b", branch, path, base)
}

// resumeTaskWorktree reuses the branch this node kept and fast-forwards it to
// the published one; a diverged branch fails with Git's output and leaves no
// worktree behind.
func resumeTaskWorktree(repo, path, branch, published string) error {
	if err := workspaceGit(repo, nil, "worktree", "add", path, branch); err != nil {
		return err
	}
	if err := workspaceGit(filepath.Join(repo, path), nil, "merge", "--ff-only", published); err != nil {
		return errors.Join(err, workspaceGit(repo, nil, "worktree", "remove", path))
	}
	return nil
}

const reviewDiff = ".solodawn-review.diff"

// PrepareReviewWorkspace checks out the submitted commit detached in
// review/<num> and writes the diff against the base next to it. The review
// session opens this folder at permission level 1.
func PrepareReviewWorkspace(projectDir string, num int, commit, base string, remote bool) (string, error) {
	repo := filepath.Join(projectDir, "repo")
	ref, err := workspaceBase(repo, base, remote)
	if err != nil {
		return "", err
	}
	review := filepath.Join(projectDir, "review", strconv.Itoa(num))
	if err := checkoutReview(repo, review, num, commit); err != nil {
		return "", err
	}
	return review, workspaceGit(repo, nil, "diff", "--no-color", "--output="+filepath.Join(review, reviewDiff), ref+"..."+commit)
}

func checkoutReview(repo, review string, num int, commit string) error {
	if _, err := os.Stat(review); err != nil {
		return workspaceGit(repo, nil, "worktree", "add", "--detach", filepath.Join("..", "review", strconv.Itoa(num)), commit)
	}
	return workspaceGit(review, nil, "checkout", "--detach", commit)
}

// RemoveTaskWorkspace removes the task's worktree and keeps its branch, which
// a later takeover on this node continues.
func RemoveTaskWorkspace(projectDir string, num int, slug string) error {
	name := strconv.Itoa(num) + "-" + slug
	return workspaceGit(filepath.Join(projectDir, "repo"), nil, "worktree", "remove", filepath.Join("..", "wt", name))
}

// RemoveReviewWorkspace deletes the diff, the only untracked file, so that
// Git removes the checkout without a force option.
func RemoveReviewWorkspace(projectDir string, num int) error {
	review := filepath.Join(projectDir, "review", strconv.Itoa(num))
	if err := os.Remove(filepath.Join(review, reviewDiff)); err != nil {
		return err
	}
	return workspaceGit(filepath.Join(projectDir, "repo"), nil, "worktree", "remove", filepath.Join("..", "review", strconv.Itoa(num)))
}
