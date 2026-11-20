package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path"
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
func (f TaskForge) PushBranch(ctx context.Context) error {
	_, err := f.command(ctx, "git", "push", "origin", f.Branch)
	return err
}

// Submit pushes a clean, checked task commit and returns its pull request number.
func (f TaskForge) Submit(ctx context.Context, checkedCommit, base, title, summary string) (int, error) {
	if err := f.checkSubmission(ctx, checkedCommit); err != nil {
		return 0, err
	}
	if err := f.PushBranch(ctx); err != nil {
		return 0, err
	}
	view, err := f.command(ctx, "gh", "pr", "view", f.Branch, "--json", "number,headRefOid")
	if err == nil {
		return existingPR(view, checkedCommit)
	}
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok || strings.TrimSpace(string(exit.Stderr)) != fmt.Sprintf("no pull requests found for branch %q", f.Branch) {
		return 0, err
	}
	return f.createPR(ctx, base, title, summary)
}

func (f TaskForge) checkSubmission(ctx context.Context, commit string) error {
	status, err := f.command(ctx, "git", "status", "--porcelain")
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

func existingPR(view, commit string) (int, error) {
	var pr struct {
		Number     int
		HeadRefOid string
	}
	if err := json.Unmarshal([]byte(view), &pr); err != nil {
		return 0, err
	}
	if pr.Number < 1 || pr.HeadRefOid != commit {
		return 0, errors.New("expected a pull request for the checked commit")
	}
	return pr.Number, nil
}

func (f TaskForge) createPR(ctx context.Context, base, title, summary string) (int, error) {
	out, err := f.command(ctx, "gh", "pr", "create", "--head", f.Branch, "--base", base, "--title", title, "--body", summary)
	if err != nil {
		return 0, err
	}
	return createdPRNumber(out)
}

func createdPRNumber(out string) (int, error) {
	u, err := url.ParseRequestURI(out)
	if err != nil {
		return 0, err
	}
	dir, number := path.Split(u.Path)
	n, err := strconv.Atoi(number)
	if u.Host == "" || !strings.HasSuffix(dir, "/pull/") || err != nil || n < 1 {
		return 0, fmt.Errorf("expected a pull request URL from gh, got %q", out)
	}
	return n, nil
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
	return hostedResult(out, commit)
}

type forgeCheck struct {
	TypeName                 string `json:"__typename"`
	Name, Status, Conclusion string // CheckRun
	Context, State           string // StatusContext
}

func hostedResult(out, commit string) (*proto.ChecksBody, error) {
	var view struct {
		HeadRefOid        string
		StatusCheckRollup []forgeCheck
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		return nil, err
	}
	if view.HeadRefOid == "" || view.StatusCheckRollup == nil {
		return nil, errors.New("expected headRefOid and statusCheckRollup from gh")
	}
	if view.HeadRefOid != commit {
		return nil, nil
	}
	return completedChecks(commit, view.StatusCheckRollup)
}

func completedChecks(commit string, checks []forgeCheck) (*proto.ChecksBody, error) {
	result := proto.ChecksBody{Stage: "cloud", OK: true, Commit: commit, Items: make([]proto.CheckItem, 0, len(checks))}
	for _, check := range checks {
		item, err := check.item()
		if item == nil {
			return nil, err
		}
		result.Items = append(result.Items, *item)
		result.OK = result.OK && item.OK
	}
	return &result, nil
}

func (c forgeCheck) item() (*proto.CheckItem, error) {
	switch c.TypeName {
	case "CheckRun":
		return c.runItem()
	case "StatusContext":
		switch c.State {
		case "EXPECTED", "PENDING":
			return nil, nil
		case "SUCCESS", "FAILURE", "ERROR":
			return finishedCheck(c.Context, c.State, c.State == "SUCCESS")
		}
	}
	return nil, fmt.Errorf("unknown hosted check type %q or state %q", c.TypeName, c.State)
}

func (c forgeCheck) runItem() (*proto.CheckItem, error) {
	switch c.Status {
	case "EXPECTED", "IN_PROGRESS", "PENDING", "QUEUED", "REQUESTED", "WAITING":
		return nil, nil
	case "COMPLETED":
		switch c.Conclusion {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			return finishedCheck(c.Name, c.Conclusion, true)
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE":
			return finishedCheck(c.Name, c.Conclusion, false)
		}
	}
	return nil, fmt.Errorf("unknown check run status %q or conclusion %q", c.Status, c.Conclusion)
}

func finishedCheck(name, state string, ok bool) (*proto.CheckItem, error) {
	if name == "" {
		return nil, errors.New("missing hosted check name")
	}
	return &proto.CheckItem{Name: name, Detail: &state, OK: ok}, nil
}
