//go:build acceptance

package features

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_forge"] = hostForgeSteps
}

func hostForgeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func hostForgeSteps(sc *godog.ScenarioContext, f fixtures) {
	var task host.TaskForge
	var remote, commit, newer string
	var pushErr error
	git := func(args ...string) string { return hostForgeGit(f.t, task.Worktree, args...) }
	makeCommit := func(msg string) {
		git("-c", "user.name=test", "-c", "user.email=test.invalid", "commit", "--allow-empty", "-m", msg)
	}
	remoteBranch := func() string {
		return hostForgeGit(f.t, remote, "for-each-ref", "--format=%(refname) %(objectname)")
	}
	sc.Step(`^a task worktree with a bare origin and another unpublished branch$`, func() {
		task = host.TaskForge{Worktree: f.t.TempDir(), Branch: "task/12-fix"}
		remote = f.t.TempDir()
		hostForgeGit(f.t, remote, "init", "--bare", "-b", "base")
		git("init", "-b", task.Branch)
		makeCommit("task")
		git("branch", "sibling")
		git("remote", "add", "origin", remote)
		commit = git("rev-parse", "HEAD")
	})
	sc.Step(`^origin has a newer commit on the assigned branch$`, func() {
		git("switch", "sibling")
		makeCommit("newer")
		newer = git("rev-parse", "HEAD")
		git("push", "origin", "HEAD:refs/heads/"+task.Branch)
		git("switch", task.Branch)
	})
	sc.Step(`^the host pushes the assigned task branch$`, func() {
		pushErr = task.PushBranch(f.t.Context())
	})
	sc.Step(`^origin contains only that branch at the task commit$`, func() error {
		if pushErr != nil {
			return pushErr
		}
		if got := remoteBranch(); got != "refs/heads/"+task.Branch+" "+commit {
			return fmt.Errorf("unexpected remote refs: %s", got)
		}
		return nil
	})
	sc.Step(`^Git rejects the push and origin keeps the newer commit$`, func() error {
		if err := hostForgeRejected(task, pushErr); err != nil {
			return err
		}
		if got := remoteBranch(); got != "refs/heads/"+task.Branch+" "+newer {
			return fmt.Errorf("unexpected remote refs after rejection: %s", got)
		}
		return nil
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
