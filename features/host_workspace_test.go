//go:build acceptance

package features

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_workspace"] = workspaceSteps
}

func workspaceCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func workspaceGitEquals(dir, want string, args ...string) error {
	got, err := workspaceCommand(dir, args...)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("git %v: got %q, want %q", args, got, want)
	}
	return nil
}

func workspaceAbsent(path string) error {
	_, err := os.Lstat(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("expected %s to be absent, stat returned %v", path, err)
	}
	return nil
}

// The member ids of the AIs in the workspace scenarios, all on one node.
const (
	assignee  = "assignee"
	successor = "successor"
	reviewer  = "reviewer"
)

func workspaceSteps(sc *godog.ScenarioContext, f fixtures) {
	var project, source, repo, commit, merged string
	taskName := func(n int) string { return strconv.Itoa(n) + "-change" }
	tree := func(n int) string { return filepath.Join(project, "wt", taskName(n)) }
	review := func(n int) string { return filepath.Join(project, "review", strconv.Itoa(n)) }
	sc.Step(`^a member joins a workspace group with a local repository$`, func() error {
		// Git reports real paths; the temporary folder may be behind a symlink (macOS).
		root, err := filepath.EvalSymlinks(f.t.TempDir())
		if err != nil {
			return err
		}
		project, source = filepath.Join(root, "projects", "group"), filepath.Join(root, "source")
		if _, err := workspaceCommand(root, "init", "-b", "trunk", source); err != nil {
			return err
		}
		if _, err := workspaceCommand(source, "commit", "--allow-empty", "-m", "initial"); err != nil {
			return err
		}
		repo, err = host.EnsureGroupWorkspace(f.t.Context(), project, source, "trunk")
		return err
	})
	sc.Step(`^another member joins the workspace group$`, func() error {
		_, err := host.EnsureGroupWorkspace(f.t.Context(), project, source, "trunk")
		return err
	})
	sc.Step(`^the group clone is the only registered worktree$`, func() error {
		head, err := workspaceCommand(repo, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		want := "worktree " + filepath.ToSlash(filepath.Join(project, "repo")) + "\nHEAD " + head + "\nbranch refs/heads/trunk\n"
		return workspaceGitEquals(repo, want, "worktree", "list", "--porcelain")
	})
	sc.Step(`^task (\d+) has no workspace or branch$`, func(n int) error {
		if err := workspaceAbsent(tree(n)); err != nil {
			return err
		}
		return workspaceGitEquals(repo, "", "branch", "--list", "task/"+taskName(n))
	})
	sc.Step(`^workspace task (\d+) starts$`, func(n int) error {
		_, err := host.CreateTaskWorkspace(f.t.Context(), project, n, "change", assignee, "trunk", false)
		return err
	})
	sc.Step(`^workspace task (\d+) is on its own branch$`, func(n int) error {
		return workspaceGitEquals(tree(n), "task/"+taskName(n), "branch", "--show-current")
	})
	sc.Step(`^task (\d+) commits a change for review$`, func(n int) error {
		dir := tree(n)
		if err := os.WriteFile(filepath.Join(dir, "change.txt"), []byte("submitted\n"), 0644); err != nil {
			return err
		}
		if _, err := workspaceCommand(dir, "add", "change.txt"); err != nil {
			return err
		}
		if _, err := workspaceCommand(dir, "commit", "-m", "change"); err != nil {
			return err
		}
		var err error
		commit, err = workspaceCommand(dir, "rev-parse", "HEAD")
		return err
	})
	sc.Step(`^the reviewer opens the submitted commit of task (\d+)$`, func(n int) error {
		_, err := host.PrepareReviewWorkspace(f.t.Context(), project, n, reviewer, commit, "trunk", false)
		return err
	})
	sc.Step(`^review (\d+) is detached at that commit and contains its diff$`, func(n int) error {
		dir := review(n)
		if err := workspaceGitEquals(dir, "", "branch", "--show-current"); err != nil {
			return err
		}
		if err := workspaceGitEquals(dir, commit, "rev-parse", "HEAD"); err != nil {
			return err
		}
		diff, err := os.ReadFile(filepath.Join(dir, ".solodawn-review.diff"))
		if err != nil {
			return err
		}
		return workspaceGitEquals(repo, strings.TrimSuffix(string(diff), "\n"), "diff", "trunk..."+commit)
	})
	sc.Step(`^the host releases the task and review folders for task (\d+)$`, func(n int) error {
		if err := host.RemoveTaskWorkspace(f.t.Context(), project, n, "change", assignee, host.TaskMerged); err != nil {
			return err
		}
		return host.RemoveReviewWorkspace(f.t.Context(), project, n, reviewer)
	})
	sc.Step(`^task (\d+) leaves an uncommitted change$`, func(n int) error {
		return os.WriteFile(filepath.Join(tree(n), "change.txt"), []byte("unsaved\n"), 0644)
	})
	sc.Step(`^the host removes the worktree of cancelled task (\d+)$`, func(n int) error {
		return host.RemoveTaskWorkspace(f.t.Context(), project, n, "change", assignee, host.TaskCancelled)
	})
	sc.Step(`^task (\d+) is given to another AI of this node before the first AI's worktree is released$`, func(n int) error {
		_, err := host.CreateTaskWorkspace(f.t.Context(), project, n, "change", successor, "trunk", false)
		return err
	})
	sc.Step(`^the host releases the first AI's worktree of task (\d+)$`, func(n int) error {
		return host.RemoveTaskWorkspace(f.t.Context(), project, n, "change", assignee, host.TaskGivenAway)
	})
	sc.Step(`^the worktree of task (\d+) has that commit and none of the first AI's uncommitted change$`, func(n int) error {
		if err := workspaceGitEquals(tree(n), commit, "rev-parse", "HEAD"); err != nil {
			return err
		}
		return workspaceGitEquals(tree(n), "", "status", "--porcelain", "--untracked-files=all")
	})
	sc.Step(`^the branch of task (\d+) is at that commit$`, func(n int) error {
		return workspaceGitEquals(repo, commit, "rev-parse", "task/"+taskName(n))
	})
	sc.Step(`^task (\d+) has no task or review folder$`, func(n int) error {
		if err := workspaceAbsent(tree(n)); err != nil {
			return err
		}
		return workspaceAbsent(review(n))
	})
	sc.Step(`^the prerequisite task (\d+) is merged into the group base$`, func(n int) error {
		if _, err := workspaceCommand(repo, "merge", "--no-ff", "-m", "merge prerequisite", "task/"+taskName(n)); err != nil {
			return err
		}
		var err error
		merged, err = workspaceCommand(repo, "rev-parse", "HEAD")
		return err
	})
	sc.Step(`^workspace task (\d+) starts at the merged prerequisite$`, func(n int) error {
		return workspaceGitEquals(tree(n), merged, "rev-parse", "HEAD")
	})
}
