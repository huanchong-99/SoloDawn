//go:build acceptance

package features

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_forge"] = hostForgeSteps
}

// hostForgeGit runs each command in dir, stops at the first that fails and
// returns the output of the last.
func hostForgeGit(dir string, commands ...[]string) (string, error) {
	var out string
	for _, args := range commands {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		b, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %v: %s", args, err, b)
		}
		out = strings.TrimSpace(string(b))
	}
	return out, nil
}

func hostForgeCommit(msg string) []string {
	return []string{"-c", "user.name=test", "-c", "user.email=test.invalid", "commit", "--allow-empty", "-m", msg}
}

func hostForgeSteps(sc *godog.ScenarioContext, f fixtures) {
	var task host.TaskForge
	var remote, commit, newer string
	var pushErr error
	remoteHas := func(want string) error {
		got, err := hostForgeGit(remote, []string{"for-each-ref", "--format=%(refname) %(objectname)"})
		if err == nil && got != "refs/heads/"+task.Branch+" "+want {
			err = fmt.Errorf("unexpected remote refs: %s", got)
		}
		return err
	}
	sc.Step(`^a task worktree with a bare origin and another unpublished branch$`, func() (err error) {
		task = host.TaskForge{Worktree: f.t.TempDir(), Branch: "task/12-fix"}
		remote = f.t.TempDir()
		if err = initBareRepo(remote, "base"); err != nil {
			return err
		}
		commit, err = hostForgeGit(task.Worktree, []string{"init", "-b", task.Branch}, hostForgeCommit("task"),
			[]string{"branch", "sibling"}, []string{"remote", "add", "origin", remote}, []string{"rev-parse", "HEAD"})
		return err
	})
	sc.Step(`^origin has a newer commit on the assigned branch$`, func() (err error) {
		if newer, err = hostForgeGit(task.Worktree, []string{"switch", "sibling"}, hostForgeCommit("newer"), []string{"rev-parse", "HEAD"}); err != nil {
			return err
		}
		_, err = hostForgeGit(task.Worktree, []string{"push", "origin", "HEAD:refs/heads/" + task.Branch}, []string{"switch", task.Branch})
		return err
	})
	sc.Step(`^the host pushes the assigned task branch$`, func(ctx context.Context) {
		pushErr = task.PushBranch(ctx)
	})
	sc.Step(`^origin contains only that branch at the task commit$`, func() error {
		if pushErr != nil {
			return pushErr
		}
		return remoteHas(commit)
	})
	sc.Step(`^Git rejects the push and origin keeps the newer commit$`, func() error {
		if err := hostForgeRejected(task, pushErr); err != nil {
			return err
		}
		return remoteHas(newer)
	})
}

// hostForgeRejected compares the host's push error with Git's own refusal,
// whose text differs between Git versions, by repeating the push without force.
func hostForgeRejected(task host.TaskForge, pushErr error) error {
	ref := "refs/heads/" + task.Branch
	push := exec.Command("git", "push", "origin", ref+":"+ref)
	push.Dir = task.Worktree
	var stderr bytes.Buffer
	push.Stderr = &stderr
	err := push.Run()
	want := fmt.Sprintf("%s: %v", strings.TrimSpace(stderr.String()), err)
	if err == nil || pushErr == nil || pushErr.Error() != want {
		return fmt.Errorf("got push error %v, want %s", pushErr, want)
	}
	return nil
}
