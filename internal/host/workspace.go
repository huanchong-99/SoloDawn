package host

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Concurrent joins must not see a clone that git is still writing.
var groupCloneMu sync.Mutex

// EnsureGroupWorkspace returns the shared clone under projectDir (projects/<group>).
func EnsureGroupWorkspace(projectDir, source string) (string, error) {
	groupCloneMu.Lock()
	defer groupCloneMu.Unlock()
	repo := filepath.Join(projectDir, "repo")
	exists, err := workspaceDirExists(repo)
	if err != nil {
		return "", err
	}
	if exists {
		return repo, workspaceGit(repo, nil, "--git-dir=.git", "rev-parse", "--git-dir")
	}
	return repo, workspaceGit("", nil, "clone", "--", source, repo)
}

func workspaceDirExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("expected a workspace directory at %s", path)
	}
	return true, nil
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

func taskWorkspaceName(num int, slug string) (string, error) {
	if slug == "" || strings.ContainsAny(slug, `/\`) {
		return "", fmt.Errorf("invalid task workspace slug %q", slug)
	}
	return strconv.Itoa(num) + "-" + slug, nil
}

// CreateTaskWorkspace starts from the current base, or resumes the assignee's
// published branch (the retained local branch for projects without a remote).
func CreateTaskWorkspace(projectDir string, num int, slug, base string, remote, takeover bool) (string, error) {
	name, err := taskWorkspaceName(num, slug)
	if err != nil {
		return "", err
	}
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
		return workspaceGit(repo, nil, "worktree", "add", "--", path, branch)
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
	return workspaceGit(repo, nil, "worktree", "add", "-b", branch, "--", path, base)
}

func resumeTaskWorktree(repo, path, branch, published string) error {
	if err := workspaceGit(repo, nil, "worktree", "add", "--", path, branch); err != nil {
		return err
	}
	if err := workspaceGit(filepath.Join(repo, path), nil, "merge", "--ff-only", published); err != nil {
		return errors.Join(err, workspaceGit(repo, nil, "worktree", "remove", "--", path))
	}
	return nil
}

const reviewDiff = ".solodawn-review.diff"

// PrepareReviewWorkspace checks out the submitted commit detached from any
// branch. The review driver opens this directory at permission level 1.
func PrepareReviewWorkspace(projectDir string, num int, commit, base string, remote bool) (string, error) {
	repo := filepath.Join(projectDir, "repo")
	ref, err := workspaceBase(repo, base, remote)
	if err != nil {
		return "", err
	}
	// Build the next round's diff before moving the reviewer's checkout.
	stage, err := os.MkdirTemp(projectDir, ".review-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := stageReviewDiff(repo, stage, commit, ref); err != nil {
		return "", err
	}
	review := filepath.Join(projectDir, "review", strconv.Itoa(num))
	if err := checkoutReview(repo, review, num, commit); err != nil {
		return "", err
	}
	return review, os.Rename(filepath.Join(stage, reviewDiff), filepath.Join(review, reviewDiff))
}

func stageReviewDiff(repo, stage, commit, ref string) error {
	// Never replace a tracked file (including a symlink) with review metadata.
	var tracked bytes.Buffer
	if err := workspaceGit(repo, &tracked, "ls-tree", "-z", "--name-only", "--end-of-options", commit); err != nil {
		return err
	}
	for path := range strings.SplitSeq(tracked.String(), "\x00") {
		if strings.EqualFold(path, reviewDiff) {
			return fmt.Errorf("review diff path %s: %w", path, fs.ErrExist)
		}
	}
	output := filepath.Join("..", filepath.Base(stage), reviewDiff)
	return workspaceGit(repo, nil, "diff", "--no-color", "--output="+output, ref+"..."+commit, "--")
}

func checkoutReview(repo, review string, num int, commit string) error {
	exists, err := workspaceDirExists(review)
	if err != nil {
		return err
	}
	if !exists {
		return workspaceGit(repo, nil, "worktree", "add", "--detach", "--", filepath.Join("..", "review", strconv.Itoa(num)), commit)
	}
	return workspaceGit(review, nil, "checkout", "--detach", "--end-of-options", commit, "--")
}

// RemoveTaskWorkspace releases the working tree, keeping its branch for local reassignment.
func RemoveTaskWorkspace(projectDir string, num int, slug string) error {
	name, err := taskWorkspaceName(num, slug)
	if err != nil {
		return err
	}
	return workspaceGit(filepath.Join(projectDir, "repo"), nil, "worktree", "remove", "--", filepath.Join("..", "wt", name))
}

// RemoveReviewWorkspace removes the generated diff before releasing the checkout.
func RemoveReviewWorkspace(projectDir string, num int) error {
	name := strconv.Itoa(num)
	if err := workspaceGit(filepath.Join(projectDir, "review", name), nil, "clean", "-fx", "--", reviewDiff); err != nil {
		return err
	}
	return workspaceGit(filepath.Join(projectDir, "repo"), nil, "worktree", "remove", "--", filepath.Join("..", "review", name))
}
