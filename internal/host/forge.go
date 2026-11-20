package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
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

// runIn runs the git or gh command name in dir and returns what it wrote to
// standard output and to standard error. Forge and merge share it.
func runIn(ctx context.Context, dir, name string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	return string(out), errOut.String(), err
}

func (f TaskForge) command(ctx context.Context, name string, args ...string) (string, error) {
	out, stderr, err := runIn(ctx, f.Worktree, name, args...)
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr), err)
	}
	return strings.TrimSpace(out), err
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
// listed, and one from another repository (a fork with a branch of the same
// name, which --head also matches) is skipped, so none of them is reused.
func (f TaskForge) openPR(ctx context.Context, base string) (string, error) {
	out, err := f.command(ctx, "gh", "pr", "list", "--head", f.Branch, "--base", base, "--state", "open",
		"--json", "number,headRefOid,url,state,baseRefName,isCrossRepository")
	if err != nil {
		return "", err
	}
	var prs []struct {
		URL               string
		IsCrossRepository bool
	}
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return "", err
	}
	for _, pr := range prs {
		if !pr.IsCrossRepository {
			return pr.URL, nil
		}
	}
	return "", nil
}

// pullRequest takes the number from the last path element of the URL.
func pullRequest(url string) (*ForgePullRequest, error) {
	n, err := strconv.Atoi(url[strings.LastIndexByte(url, '/')+1:])
	if err != nil {
		return nil, err
	}
	return &ForgePullRequest{Number: n, URL: url}, nil
}

// WaitHostedChecks reads the head and checks together every 30 s and returns
// the result for commit once every check has finished. A read that fails or
// shows another head counts as unfinished. No checks pass only on a second
// consecutive empty read, because a forge may register its checks a few
// seconds late. The first unfinished read at or after sixty minutes from since,
// checks.local.at of the submitted commit, fails the checks as timed out with
// the items of the last read at that commit. The error is ctx's: cancel ctx
// on supersession.
func (f TaskForge) WaitHostedChecks(ctx context.Context, pr int, commit string, since time.Time) (*proto.ChecksBody, error) {
	w := hostedWait{commit: commit}
	deadline := since.Add(60 * time.Minute)
	for {
		if result := w.next(f.readHostedChecks(ctx, pr, commit)); result != nil {
			return result, nil
		}
		if !time.Now().Before(deadline) {
			return w.timedOut(), nil
		}
		nextPoll := time.After(30 * time.Second)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-nextPoll:
		}
	}
}

// hostedRead is one read at the submitted head: an item per check, and
// whether every check has finished.
type hostedRead struct {
	items []proto.CheckItem
	done  bool
}

// readHostedChecks is nil when the read failed or showed another head.
func (f TaskForge) readHostedChecks(ctx context.Context, pr int, commit string) *hostedRead {
	out, err := f.command(ctx, "gh", "pr", "view", strconv.Itoa(pr), "--json", "headRefOid,statusCheckRollup")
	var view struct {
		HeadRefOid        string
		StatusCheckRollup []forgeCheck
	}
	if err != nil || json.Unmarshal([]byte(out), &view) != nil || view.HeadRefOid != commit {
		return nil
	}
	read := hostedRead{items: make([]proto.CheckItem, 0, len(view.StatusCheckRollup)), done: true}
	for _, check := range view.StatusCheckRollup {
		item, done := check.item()
		read.items = append(read.items, item)
		read.done = read.done && done
	}
	return &read
}

// hostedWait keeps what the reads so far decide: the items of the last read
// at the submitted head, and whether that read was empty and finished.
type hostedWait struct {
	commit string
	last   []proto.CheckItem
	empty  bool
}

// next takes one read and returns the result to report, or nil to read again.
func (w *hostedWait) next(read *hostedRead) *proto.ChecksBody {
	if read == nil {
		w.empty = false
		return nil
	}
	empty := len(read.items) == 0
	if read.done && (!empty || w.empty) {
		return w.body(read.items)
	}
	w.last, w.empty = read.items, empty
	return nil
}

// timedOut fails the checks with the last read's items and the reason.
func (w *hostedWait) timedOut() *proto.ChecksBody {
	return w.body(append(w.last, proto.CheckItem{Name: "hosted checks", Detail: new("hosted checks timed out")}))
}

func (w *hostedWait) body(items []proto.CheckItem) *proto.ChecksBody {
	result := proto.ChecksBody{Stage: "cloud", OK: true, Commit: w.commit, Items: items}
	for _, item := range items {
		result.OK = result.OK && item.OK
	}
	return &result
}

// HostedCheck is a task whose hosted checks a restarted node polls again.
type HostedCheck struct {
	Task, Branch, Commit string
	PR                   int
	Since                time.Time // checks.local.at, where the 60 minutes count from
}

// HostedChecksToResume selects, from the task rows, the tasks a restarted
// node polls again: at step cloud_check, assigned to one of ais, and active
// or paused, the states in which task-machine row 6 takes task cloud. A row
// without checks.local has its 60 minutes spent: it ends with the timed-out
// result unless the first read has finished.
func HostedChecksToResume(tasks []proto.TaskRow, ais []string) []HostedCheck {
	var resume []HostedCheck
	for _, t := range tasks {
		if t.Step == "cloud_check" && slices.Contains(ais, t.Assignee) && slices.Contains(cloudStates, t.State) {
			resume = append(resume, HostedCheck{Task: t.ID, Branch: t.Branch, Commit: t.Commit, PR: t.PR, Since: localCheckTime(t.Checks.Local)})
		}
	}
	return resume
}

var cloudStates = []string{"active", "paused"}

func localCheckTime(local *proto.Check) time.Time {
	if local == nil {
		return time.Time{}
	}
	return time.UnixMilli(local.At)
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

// item is the check as reported, and whether it has finished. An unfinished
// check is not passed; its detail is its status or state.
func (c forgeCheck) item() (proto.CheckItem, bool) {
	name, state, done := c.Name, c.Conclusion, c.Status == "COMPLETED"
	if !done {
		state = c.Status
	}
	if c.TypeName == "StatusContext" {
		name, state, done = c.Context, c.State, c.State != "EXPECTED" && c.State != "PENDING"
	}
	return proto.CheckItem{Name: name, OK: passed[state], Detail: &state}, done
}
