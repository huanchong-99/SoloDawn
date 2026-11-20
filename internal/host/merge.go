package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ReviewedMerge identifies a merge item taken by the host after checking
// that the task is still active at the merge step for this commit.
type ReviewedMerge struct {
	Repo, Base, Branch string
	Task, Commit       string
	PR                 int // zero for a local-only group
}

// MergeResult supplies the squash commit for task merged, or the detail for
// task conflict. An inspection error leaves the outcome unknown.
type MergeResult struct {
	MergeCommit string
	Conflict    string
}

// Apply makes one merge attempt. For an item already taken before restart,
// the caller must use Inspect instead.
func (m ReviewedMerge) Apply(ctx context.Context) (MergeResult, error) {
	if m.PR == 0 {
		// Local tasks share the clone's base checkout and index: the squash
		// and its commit (or reset) run under the clone's lock.
		lock := cloneLock(m.Repo)
		lock.Lock()
		defer lock.Unlock()
		if err := m.squashLocal(ctx); err != nil {
			return MergeResult{Conflict: err.Error()}, nil
		}
		return m.inspectLocal(ctx)
	}
	// gh can exit unsuccessfully deleting the branch after the server has
	// already merged the PR. Inspect its state even on a command error.
	_, mergeErr := m.run(ctx, "gh", "pr", "merge", strconv.Itoa(m.PR),
		"--squash", "--delete-branch", "--match-head-commit", m.Commit)
	result, err := m.inspectPR(ctx)
	if err != nil {
		return MergeResult{}, errors.Join(mergeErr, err)
	}
	if result.Conflict != "" && mergeErr != nil {
		result.Conflict = mergeErr.Error()
	}
	return result, nil
}

func (m ReviewedMerge) message() string {
	return fmt.Sprintf("Merge task %s at %s", m.Task, m.Commit)
}

func (m ReviewedMerge) squashLocal(ctx context.Context) error {
	if err := m.checkLocal(ctx); err != nil {
		return err
	}
	_, err := m.run(ctx, "git", "merge", "--squash", "--", m.Commit)
	if err == nil {
		_, err = m.run(ctx, "git", "commit", "--allow-empty", "-m", m.message())
	}
	if err != nil {
		_, resetErr := m.run(context.WithoutCancel(ctx), "git", "reset", "--merge")
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

// Inspect reads the last commit on the local base or the state of the PR.
// It never attempts a merge. Without a remote, a base that does not record
// the merge gets the reset of a failed merge, which undoes a squash
// interrupted before its commit.
func (m ReviewedMerge) Inspect(ctx context.Context) (MergeResult, error) {
	if m.PR != 0 {
		return m.inspectPR(ctx)
	}
	lock := cloneLock(m.Repo)
	lock.Lock()
	defer lock.Unlock()
	result, err := m.inspectLocal(ctx)
	if result.Conflict == "" {
		return result, err
	}
	if _, err := m.run(ctx, "git", "reset", "--merge"); err != nil {
		return MergeResult{}, err
	}
	return result, nil
}

func (m ReviewedMerge) inspectLocal(ctx context.Context) (MergeResult, error) {
	last, err := m.run(ctx, "git", "log", "-1", "--no-show-signature", "--format=%H%n%B", "refs/heads/"+m.Base, "--")
	if err != nil {
		return MergeResult{}, err
	}
	oid, message, _ := strings.Cut(last, "\n")
	if message != m.message() {
		return MergeResult{Conflict: fmt.Sprintf("last commit on %s does not record %s", m.Base, m.message())}, nil
	}
	return MergeResult{MergeCommit: oid}, nil
}

func (m ReviewedMerge) inspectPR(ctx context.Context) (MergeResult, error) {
	out, err := m.run(ctx, "gh", "pr", "view", strconv.Itoa(m.PR), "--json", "state,mergeCommit,headRefOid")
	if err != nil {
		return MergeResult{}, err
	}
	var pr struct {
		State       string
		HeadRefOID  string
		MergeCommit struct{ OID string }
	}
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return MergeResult{}, fmt.Errorf("gh pr view: %w", err)
	}
	if pr.State != "MERGED" {
		return MergeResult{Conflict: fmt.Sprintf("pull request %d is %s", m.PR, pr.State)}, nil
	}
	if pr.HeadRefOID != m.Commit {
		return MergeResult{Conflict: fmt.Sprintf("pull request %d merged head %s, expected %s", m.PR, pr.HeadRefOID, m.Commit)}, nil
	}
	return MergeResult{MergeCommit: pr.MergeCommit.OID}, nil
}

func (m ReviewedMerge) run(ctx context.Context, name string, args ...string) (string, error) {
	out, stderr, err := runIn(ctx, m.Repo, name, args...)
	if err != nil {
		detail := strings.TrimSpace(out + stderr)
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
	}
	return strings.TrimRight(out, "\r\n"), nil
}
