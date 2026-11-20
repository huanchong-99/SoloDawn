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
	if takeover && !remote {
		return workspaceGit(repo, nil, "worktree", "add", "--", path, branch)
	}
	if takeover {
		published := "refs/remotes/origin/" + branch
		var refs bytes.Buffer
		if err := workspaceGit(repo, &refs, "for-each-ref", "--format=%(refname)", published); err != nil {
			return err
		}
		if strings.TrimSuffix(refs.String(), "\n") == published {
			base = published
		}
	}
	return workspaceGit(repo, nil, "worktree", "add", "-b", branch, "--", path, base)
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
	review := filepath.Join(projectDir, "review", strconv.Itoa(num))
	if err := checkoutReview(repo, review, num, commit); err != nil {
		return "", err
	}
	// Never overwrite a file (including a symlink) from the submitted tree.
	diff, err := os.OpenFile(filepath.Join(review, reviewDiff), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	err = workspaceGit(review, diff, "diff", "--no-color", ref+"...HEAD", "--")
	return review, errors.Join(err, diff.Close())
}

func checkoutReview(repo, review string, num int, commit string) error {
	exists, err := workspaceDirExists(review)
	if err != nil {
		return err
	}
	if !exists {
		return workspaceGit(repo, nil, "worktree", "add", "--detach", "--", filepath.Join("..", "review", strconv.Itoa(num)), commit)
	}
	if err := cleanReviewDiff(review); err != nil {
		return err
	}
	// Resolve the revision as an operand before handing it to checkout.
	var oid bytes.Buffer
	if err := workspaceGit(repo, &oid, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}"); err != nil {
		return err
	}
	return workspaceGit(review, nil, "checkout", "--detach", strings.TrimSpace(oid.String()), "--")
}

// RemoveTaskWorkspace releases the working tree, keeping its branch for local reassignment.
func RemoveTaskWorkspace(projectDir string, num int, slug string) error {
	name, err := taskWorkspaceName(num, slug)
	if err != nil {
		return err
	}
	return workspaceGit(filepath.Join(projectDir, "repo"), nil, "worktree", "remove", "--", filepath.Join("..", "wt", name))
}

func cleanReviewDiff(review string) error {
	return workspaceGit(review, nil, "clean", "-fx", "--", reviewDiff)
}

// RemoveReviewWorkspace removes the generated diff before releasing the checkout.
func RemoveReviewWorkspace(projectDir string, num int) error {
	name := strconv.Itoa(num)
	if err := cleanReviewDiff(filepath.Join(projectDir, "review", name)); err != nil {
		return err
	}
	return workspaceGit(filepath.Join(projectDir, "repo"), nil, "worktree", "remove", "--", filepath.Join("..", "review", name))
}
