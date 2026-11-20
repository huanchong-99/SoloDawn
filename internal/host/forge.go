package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// TaskForge is bound by the host to the task's worktree and assigned branch.
type TaskForge struct {
	Worktree string
	Branch   string
}

func (f TaskForge) command(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = f.Worktree
	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(exit.Stderr)), err)
	}
	return strings.TrimSpace(string(out)), err
}

// PushBranch pushes only the host-assigned branch, without requesting approval.
// The full refspec keeps the owner's push configuration from redirecting it.
func (f TaskForge) PushBranch(ctx context.Context) error {
	ref := "refs/heads/" + f.Branch
	_, err := f.command(ctx, "git", "push", "origin", ref+":"+ref)
	return err
}

// ForgePullRequest identifies the pull request reported with a submitted commit.
type ForgePullRequest struct {
	Number int
	URL    string
}

// Submit pushes a clean, checked task commit and returns its open pull request.
func (f TaskForge) Submit(ctx context.Context, checkedCommit, base, title, summary string) (*ForgePullRequest, error) {
	if err := f.checkSubmission(ctx, checkedCommit); err != nil {
		return nil, err
	}
	if err := f.PushBranch(ctx); err != nil {
		return nil, err
	}
	url, err := f.openPR(ctx, base)
	if url == "" && err == nil {
		url, err = f.command(ctx, "gh", "pr", "create", "--head", f.Branch, "--base", base, "--title", title, "--body", summary)
	}
	if err != nil {
		return nil, err
	}
	return pullRequest(url)
}

func (f TaskForge) checkSubmission(ctx context.Context, commit string) error {
	// Untracked files are forced to show: the owner's status.showUntrackedFiles cannot hide them.
	status, err := f.command(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return errors.New("task worktree has uncommitted changes")
	}
	heads, err := f.command(ctx, "git", "rev-parse", "HEAD", "refs/heads/"+f.Branch)
	if err != nil {
		return err
	}
	if heads != commit+"\n"+commit {
		return errors.New("task head differs from the checked commit")
	}
	return nil
}

// openPR returns the URL of the open pull request from the task branch into
// base, or "" if there is none. Closed and other-base pull requests are not
// listed, so they are never reused.
func (f TaskForge) openPR(ctx context.Context, base string) (string, error) {
	out, err := f.command(ctx, "gh", "pr", "list", "--head", f.Branch, "--base", base, "--state", "open", "--json", "number,headRefOid,url,state,baseRefName")
	if err != nil {
		return "", err
	}
	var prs []struct{ URL string }
	if err := json.Unmarshal([]byte(out), &prs); err != nil || len(prs) == 0 {
		return "", err
	}
	return prs[0].URL, nil
}

// pullRequest takes the number from the last path element of the URL.
func pullRequest(url string) (*ForgePullRequest, error) {
	n, err := strconv.Atoi(url[strings.LastIndexByte(url, '/')+1:])
	if err != nil {
		return nil, err
	}
	return &ForgePullRequest{Number: n, URL: url}, nil
}

// WaitHostedChecks reads the head and checks together. Only a completed
// result for commit can be reported to the home. Cancel ctx on supersession.
func (f TaskForge) WaitHostedChecks(ctx context.Context, pr int, commit string) (*proto.ChecksBody, error) {
	for {
		result, err := f.ReadHostedChecks(ctx, pr, commit)
		if result != nil || err != nil {
			return result, err
		}
		nextPoll := time.After(30 * time.Second)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-nextPoll:
		}
	}
}

// ReadHostedChecks takes one sample. A nil result without an error means
// the head has moved or a check is still pending.
func (f TaskForge) ReadHostedChecks(ctx context.Context, pr int, commit string) (*proto.ChecksBody, error) {
	out, err := f.command(ctx, "gh", "pr", "view", strconv.Itoa(pr), "--json", "headRefOid,statusCheckRollup")
	if err != nil {
		return nil, err
	}
	var view struct {
		HeadRefOid        string
		StatusCheckRollup []forgeCheck
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil || view.HeadRefOid != commit {
		return nil, err
	}
	return completedChecks(commit, view.StatusCheckRollup), nil
}

// forgeCheck is one entry of the rollup: a CheckRun (name, status,
// conclusion) or a StatusContext (context, state).
type forgeCheck struct {
	TypeName                 string `json:"__typename"`
	Name, Status, Conclusion string
	Context, State           string
}

// passed holds the conclusions and states that count as a passed check.
var passed = map[string]bool{"SUCCESS": true, "NEUTRAL": true, "SKIPPED": true}

// completedChecks is nil while any check is still pending. No checks pass.
func completedChecks(commit string, checks []forgeCheck) *proto.ChecksBody {
	result := proto.ChecksBody{Stage: "cloud", OK: true, Commit: commit, Items: make([]proto.CheckItem, 0, len(checks))}
	for _, check := range checks {
		item := check.item()
		if item == nil {
			return nil
		}
		result.Items = append(result.Items, *item)
		result.OK = result.OK && item.OK
	}
	return &result
}

func (c forgeCheck) item() *proto.CheckItem {
	name, state, done := c.Name, c.Conclusion, c.Status == "COMPLETED"
	if c.TypeName == "StatusContext" {
		name, state, done = c.Context, c.State, c.State != "EXPECTED" && c.State != "PENDING"
	}
	if !done {
		return nil
	}
	return &proto.CheckItem{Name: name, OK: passed[state], Detail: &state}
}
