package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ReviewedMerge identifies a merge item taken by the host after checking
// that the task is still active at the merge step for this commit.
type ReviewedMerge struct {
	Repo, Base, Branch string
	Task, Commit       string
	PR                 int    // zero for a local-only group
	GitHubRepo         string // <owner>/<repo> of the group's GitHub repository
	// Taken is when the node took the merge, the time of its audit row
	// merge. After a restart a merge still pending on the forge waits until
	// 60 minutes after it; zero counts from the restart.
	Taken time.Time
}

// MergeResult supplies the squash commit for task merged, or the detail for
// task conflict. An inspection error leaves the outcome unknown.
type MergeResult struct {
	MergeCommit string
	Conflict    string
}

// Apply makes one merge attempt. For an item already taken before restart,
// the caller must use Inspect instead. The command output in a conflict goes
// to the group, so a secret in it is redacted (local.md 8).
func (m ReviewedMerge) Apply(ctx context.Context) (MergeResult, error) {
	if m.PR == 0 {
		// Local tasks share the clone's base checkout and index: the squash
		// and its commit (or reset) run under the clone's lock.
		lock := cloneLock(m.Repo)
		lock.Lock()
		defer lock.Unlock()
		if err := m.squashLocal(ctx); err != nil {
			return MergeResult{Conflict: redactSecrets(err.Error())}, nil
		}
		return m.inspectLocal(ctx)
	}
	return m.applyPR(ctx)
}

// applyPR merges through the forge. A merge command that succeeds may leave
// the PR pending in a merge queue or auto-merge (awaitPR).
func (m ReviewedMerge) applyPR(ctx context.Context) (MergeResult, error) {
	taken := time.Now()
	_, mergeErr := m.mergePR(ctx)
	if mergeErr == nil {
		// GitHub's read can lag its own write: right after the merge an OPEN
		// PR with nothing pending is final only from the second read on.
		return m.awaitPR(ctx, taken, false)
	}
	// gh can exit unsuccessfully deleting the remote branch after the server
	// has already merged the PR. Inspect its state even on a command error.
	settled, result, err := m.readPR(ctx, true)
	if err != nil {
		return MergeResult{}, errors.Join(mergeErr, err)
	}
	if !settled {
		return m.awaitPR(ctx, taken, true)
	}
	if result.Conflict != "" {
		result.Conflict = redactSecrets(mergeErr.Error())
	}
	return result, nil
}

// DeleteBranch deletes the local task branch after a successful merge and
// the removal of the task's worktree, only while the branch is still the
// merged commit. A branch that has moved since is kept, and deleted is false,
// so no unreviewed commit is lost. The branch can move only from a worktree
// that has it checked out, and branch -D refuses exactly that case, so a
// worktree whose removal failed is never left on a deleted branch. It runs
// under the clone's lock.
func (m ReviewedMerge) DeleteBranch(ctx context.Context) (deleted bool, err error) {
	lock := cloneLock(m.Repo)
	lock.Lock()
	defer lock.Unlock()
	ref := "refs/heads/" + m.Branch
	head, err := m.run(ctx, "git", "rev-parse", "--verify", ref)
	if err != nil || head != m.Commit {
		return false, err
	}
	_, err = m.run(ctx, "git", "branch", "-D", m.Branch)
	return err == nil, err
}

func (m ReviewedMerge) message() string {
	return fmt.Sprintf("Merge task %s at %s", m.Task, m.Commit)
}

// noHooks makes the empty folder that the local merge's Git commands take as
// core.hooksPath, so that no hook of the project runs in them (local.md 7),
// and returns that setting for -c.
func (m ReviewedMerge) noHooks() (string, error) {
	dir := filepath.Join(m.Repo, ".git", "solodawn-no-hooks")
	return "core.hooksPath=" + dir, os.MkdirAll(dir, 0o700)
}

// squashLocal squashes and commits without the project's hooks, and without
// signing, which could wait for a person; a failure is reset the same way.
func (m ReviewedMerge) squashLocal(ctx context.Context) error {
	if err := m.checkLocal(ctx); err != nil {
		return err
	}
	hooks, err := m.noHooks()
	if err != nil {
		return err
	}
	_, err = m.run(ctx, "git", "-c", hooks, "merge", "--squash", "--", m.Commit)
	if err == nil {
		_, err = m.run(ctx, "git", "-c", hooks, "commit", "--allow-empty", "--no-verify", "--no-gpg-sign", "-m", m.message())
	}
	if err != nil {
		_, resetErr := m.run(context.WithoutCancel(ctx), "git", "-c", hooks, "reset", "--merge")
		return errors.Join(err, resetErr)
	}
	return nil
}

func (m ReviewedMerge) checkLocal(ctx context.Context) error {
	if err := m.expectGit(ctx, m.Commit, "rev-parse", "--verify", "refs/heads/"+m.Branch); err != nil {
		return err
	}
	return m.expectGit(ctx, "refs/heads/"+m.Base, "symbolic-ref", "HEAD")
}

func (m ReviewedMerge) expectGit(ctx context.Context, want string, args ...string) error {
	got, err := m.run(ctx, "git", args...)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("git %s: got %q, expected %q", strings.Join(args, " "), got, want)
	}
	return nil
}

// Inspect looks for the merge on the local base, or reads the PR and waits
// while it is still pending (awaitPR), from Taken or else from now. It never
// attempts a merge. Without a remote, a base that does not record the merge
// gets the reset of a failed merge, which undoes a squash interrupted before
// its commit.
func (m ReviewedMerge) Inspect(ctx context.Context) (MergeResult, error) {
	if m.PR != 0 {
		since := m.Taken
		if since.IsZero() {
			since = time.Now()
		}
		return m.awaitPR(ctx, since, true)
	}
	lock := cloneLock(m.Repo)
	lock.Lock()
	defer lock.Unlock()
	result, err := m.inspectLocal(ctx)
	if result.Conflict == "" {
		return result, err
	}
	if err := m.resetLocal(ctx); err != nil {
		return MergeResult{}, err
	}
	return result, nil
}

func (m ReviewedMerge) resetLocal(ctx context.Context) error {
	hooks, err := m.noHooks()
	if err == nil {
		_, err = m.run(ctx, "git", "-c", hooks, "reset", "--merge")
	}
	return err
}

// inspectLocal finds the merge commit on the first-parent history of the
// base: the newest commit whose message is exactly the merge message.
func (m ReviewedMerge) inspectLocal(ctx context.Context) (MergeResult, error) {
	log, err := m.run(ctx, "git", "log", "-z", "--first-parent", "--no-show-signature", "--fixed-strings",
		"--grep="+m.message(), "--format=%H%n%B", "refs/heads/"+m.Base, "--")
	if err != nil {
		return MergeResult{}, err
	}
	for entry := range strings.SplitSeq(log, "\x00") {
		oid, message, _ := strings.Cut(entry, "\n")
		if strings.TrimRight(message, "\n") == m.message() {
			return MergeResult{MergeCommit: oid}, nil
		}
	}
	return MergeResult{Conflict: fmt.Sprintf("no commit on %s records %s", m.Base, m.message())}, nil
}

// awaitPR reads the PR every 30 s while its merge is pending: OPEN with a
// merge queue entry or auto-merge, which merges it later. A read that fails
// counts as unfinished too: only an outcome (merged, closed, or OPEN with
// nothing pending, which counts from the first read only when idleFinal) or
// the limit, 60 minutes after since, ends the wait.
func (m ReviewedMerge) awaitPR(ctx context.Context, since time.Time, idleFinal bool) (MergeResult, error) {
	deadline := since.Add(60 * time.Minute)
	for {
		settled, result, err := m.readPR(ctx, idleFinal)
		if err == nil && settled {
			return result, nil
		}
		idleFinal = true
		if !time.Now().Before(deadline) {
			return stillPending(err)
		}
		if err := pollWait(ctx); err != nil {
			return MergeResult{}, err
		}
	}
}

// stillPending is the outcome at the limit. When the last read failed, the
// outcome is unknown: its error is returned and the node reads again later,
// since a merged PR must not become a conflict because the forge was out of
// reach. Only a read that showed the merge still pending is a conflict.
func stillPending(lastRead error) (MergeResult, error) {
	if lastRead != nil {
		return MergeResult{}, lastRead
	}
	return MergeResult{Conflict: "merge still pending after 60 minutes (merge queue or auto-merge)"}, nil
}

// pollWait waits the 30 s between two reads, or returns ctx's error.
func pollWait(ctx context.Context) error {
	nextPoll := time.After(30 * time.Second)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-nextPoll:
		return nil
	}
}

// prQuery reads, in one call, what decides a merge through the forge.
const prQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { pullRequest(number: $number) { state headRefOid mergeCommit { oid } autoMergeRequest { enabledAt } mergeQueueEntry { id } } } }`

// readPR reads the PR once, from the folder that holds the clone, and returns
// whether it has settled, with the outcome; idleFinal settles an OPEN PR with
// nothing pending.
func (m ReviewedMerge) readPR(ctx context.Context, idleFinal bool) (bool, MergeResult, error) {
	owner, name, _ := strings.Cut(m.GitHubRepo, "/")
	out, err := commandText(ctx, filepath.Dir(m.Repo), "gh", "api", "graphql", "-f", "query="+prQuery,
		"-f", "owner="+owner, "-f", "name="+name, "-F", "number="+strconv.Itoa(m.PR), "--jq", ".data.repository.pullRequest")
	if err != nil {
		return false, MergeResult{}, err
	}
	var pr forgeMergeState
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return false, MergeResult{}, fmt.Errorf("gh api graphql: %w", err)
	}
	settled, result := pr.outcome(m, idleFinal)
	return settled, result, nil
}

// forgeMergeState is the PR as prQuery reads it; a null field is nil.
type forgeMergeState struct {
	State, HeadRefOid string
	MergeCommit       struct{ Oid string }
	AutoMergeRequest  *struct{}
	MergeQueueEntry   *struct{}
}

func (pr forgeMergeState) outcome(m ReviewedMerge, idleFinal bool) (bool, MergeResult) {
	if pr.State == "MERGED" {
		return true, pr.merged(m)
	}
	if pr.State == "OPEN" && (pr.AutoMergeRequest != nil || pr.MergeQueueEntry != nil) {
		return false, MergeResult{}
	}
	detail, settled := fmt.Sprintf("pull request %d is %s", m.PR, pr.State), true
	if pr.State == "OPEN" {
		detail, settled = detail+", with no merge queue entry and no auto-merge", idleFinal
	}
	return settled, MergeResult{Conflict: detail}
}

func (pr forgeMergeState) merged(m ReviewedMerge) MergeResult {
	if pr.HeadRefOid != m.Commit {
		return MergeResult{Conflict: fmt.Sprintf("pull request %d merged head %s, expected %s", m.PR, pr.HeadRefOid, m.Commit)}
	}
	return MergeResult{MergeCommit: pr.MergeCommit.Oid}
}

// mergePR runs gh with --repo from the folder that holds the clone, not
// inside it: gh then merges and deletes the remote branch only, whatever its
// version, and leaves the worktree and the local branch to the node, which
// removes them itself (RemoveTaskWorkspace, DeleteBranch).
func (m ReviewedMerge) mergePR(ctx context.Context) (string, error) {
	return commandText(ctx, filepath.Dir(m.Repo), "gh", "pr", "merge", strconv.Itoa(m.PR), "--repo", m.GitHubRepo,
		"--squash", "--delete-branch", "--match-head-commit", m.Commit)
}

func (m ReviewedMerge) run(ctx context.Context, name string, args ...string) (string, error) {
	return commandText(ctx, m.Repo, name, args...)
}

// commandText runs name in dir and returns its output without the final
// line break; an error names the command and carries its whole output.
func commandText(ctx context.Context, dir, name string, args ...string) (string, error) {
	out, stderr, err := runIn(ctx, dir, name, args...)
	if err != nil {
		detail := strings.TrimSpace(out + stderr)
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
	}
	return strings.TrimRight(out, "\r\n"), nil
}
