//go:build acceptance

package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_merge"] = hostMergeSteps
}

func hostMergeSteps(sc *godog.ScenarioContext, f fixtures) {
	var m host.ReviewedMerge
	var taskTree, base, moved, squash string
	var result host.MergeResult
	sc.Step(`^a reviewed task commit in a local task worktree$`, func() error {
		root := f.t.TempDir()
		taskTree = filepath.Join(root, "worktree")
		m = host.ReviewedMerge{Repo: filepath.Join(root, "repo"), Base: "base", Branch: "task/7", Task: "t7"}
		var err error
		base, err = hostMergeSetup(&m, taskTree)
		return err
	})
	sc.Step(`^the assignee has advanced the task branch after review$`, func() error {
		var err error
		moved, err = hostMergeEdit(taskTree, "unreviewed")
		return err
	})
	sc.Step(`^the base has independently changed the same line$`, func() error {
		var err error
		base, err = hostMergeEdit(m.Repo, "base change")
		return err
	})
	sc.Step(`^the host takes the merge item for the reviewed commit$`, func(ctx context.Context) error {
		var err error
		result, err = m.Apply(ctx)
		return err
	})
	sc.Step(`^the host reports a conflict naming the moved head$`, func() error {
		want := fmt.Sprintf("git rev-parse --verify refs/heads/task/7: got %q, expected %q", moved, m.Commit)
		return hostMergeWant(result, host.MergeResult{Conflict: want})
	})
	sc.Step(`^the host reports the Git merge conflict$`, func() error {
		if !strings.Contains(result.Conflict, "CONFLICT (content): Merge conflict in item") {
			return fmt.Errorf("missing git conflict detail in %q", result.Conflict)
		}
		return hostMergeWant(result.MergeCommit, "")
	})
	sc.Step(`^the base commit and index are unchanged$`, func() error {
		if err := hostMergeVerify(m.Repo, "", "status", "--porcelain"); err != nil {
			return err
		}
		return hostMergeVerify(m.Repo, base, "rev-parse", "base")
	})
	sc.Step(`^the base contains the reviewed change in a single squash commit$`, func() error {
		squash = result.MergeCommit
		if err := hostMergeVerify(m.Repo, squash+" "+base, "rev-list", "-1", "--parents", "base"); err != nil {
			return err
		}
		return hostMergeVerify(m.Repo, "reviewed", "show", "base:item")
	})
	sc.Step(`^the restarted host inspects the taken merge item$`, func(ctx context.Context) error {
		var err error
		result, err = m.Inspect(ctx)
		return err
	})
	sc.Step(`^the host reports the existing squash commit$`, func() error {
		if err := hostMergeWant(result, host.MergeResult{MergeCommit: squash}); err != nil {
			return err
		}
		return hostMergeVerify(m.Repo, squash, "rev-parse", "base")
	})
	sc.Step(`^the host reports that the base does not record this merge$`, func() error {
		return hostMergeWant(result, host.MergeResult{Conflict: "last commit on base does not record Merge task t7 at " + m.Commit})
	})
}

func hostMergeSetup(m *host.ReviewedMerge, taskTree string) (string, error) {
	for _, args := range [][]string{
		{"init", "-q", "-b", m.Base, m.Repo},
		{"-C", m.Repo, "config", "user.name", "fixture"},
		{"-C", m.Repo, "config", "user.email", "fixture"},
		{"-C", m.Repo, "config", "commit.gpgsign", "false"},
	} {
		if _, err := hostMergeGit(filepath.Dir(m.Repo), args...); err != nil {
			return "", err
		}
	}
	base, err := hostMergeEdit(m.Repo, "initial")
	if err != nil {
		return "", err
	}
	if _, err := hostMergeGit(m.Repo, "worktree", "add", "-qb", m.Branch, taskTree); err != nil {
		return "", err
	}
	m.Commit, err = hostMergeEdit(taskTree, "reviewed")
	return base, err
}

func hostMergeGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func hostMergeEdit(dir, text string) (string, error) {
	if err := os.WriteFile(filepath.Join(dir, "item"), []byte(text+"\n"), 0600); err != nil {
		return "", err
	}
	if _, err := hostMergeGit(dir, "add", "item"); err != nil {
		return "", err
	}
	if _, err := hostMergeGit(dir, "commit", "-qm", text); err != nil {
		return "", err
	}
	return hostMergeGit(dir, "rev-parse", "HEAD")
}

func hostMergeVerify(dir, want string, args ...string) error {
	got, err := hostMergeGit(dir, args...)
	if err != nil {
		return err
	}
	return hostMergeWant(got, want)
}

func hostMergeWant[T comparable](got, want T) error {
	if got != want {
		return fmt.Errorf("got %v, want %v", got, want)
	}
	return nil
}
