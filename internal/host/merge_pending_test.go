package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func pendingMerge(t *testing.T) ReviewedMerge {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	forgeOK(os.Mkdir(repo, 0700))
	return ReviewedMerge{Repo: repo, Task: "t7", Commit: "abc", PR: 32, GitHubRepo: "team/website"}
}

var (
	ghMerge = []string{"pr", "merge", "32", "--repo", "team/website", "--squash", "--delete-branch", "--match-head-commit", "abc"}
	ghView  = []string{"api", "graphql", "-f", "query=" + prQuery, "-f", "owner=team", "-f", "name=website", "-F", "number=32",
		"--jq", ".data.repository.pullRequest"}
	prOpen   = forgeAnswer{Stdout: `{"state":"OPEN","headRefOid":"abc","mergeCommit":null,"autoMergeRequest":{"enabledAt":"2026-11-20T12:00:00Z"},"mergeQueueEntry":null}`}
	prQueued = forgeAnswer{Stdout: `{"state":"OPEN","headRefOid":"abc","mergeCommit":null,"autoMergeRequest":null,"mergeQueueEntry":{"id":"MQE_1"}}`}
	prIdle   = forgeAnswer{Stdout: `{"state":"OPEN","headRefOid":"abc","mergeCommit":null,"autoMergeRequest":null,"mergeQueueEntry":null}`}
	prMerged = forgeAnswer{Stdout: `{"state":"MERGED","headRefOid":"abc","mergeCommit":{"oid":"def"},"autoMergeRequest":null,"mergeQueueEntry":null}`}
)

// runPending runs op on a virtual clock and checks its result and how long
// it took.
func runPending(t *testing.T, op func(context.Context) (MergeResult, error), want MergeResult, took time.Duration) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		result, err := op(t.Context())
		assertForge(t, err, nil)
		assertForge(t, result, want)
		assertForge(t, time.Since(start), took)
	})
}

// A merge queue or auto-merge keeps the PR OPEN after gh pr merge exits 0:
// the merge is pending, and the PR is read every 30 s until it is merged.
func TestMergeWaitsWhilePending(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, forgeAnswer{}, prOpen, prQueued, prMerged)
	runPending(t, m.Apply, MergeResult{MergeCommit: "def"}, time.Minute)
	calls(ghMerge, ghView, ghView, ghView)
}

func TestPendingMergeEnds(t *testing.T) {
	for _, tc := range []struct {
		end  forgeAnswer
		want MergeResult
	}{
		{forgeAnswer{Stdout: `{"state":"CLOSED","mergeCommit":null,"headRefOid":"abc"}`}, MergeResult{Conflict: "pull request 32 is CLOSED"}},
		{forgeAnswer{Stdout: `{"state":"MERGED","mergeCommit":{"oid":"def"},"headRefOid":"xyz"}`}, MergeResult{Conflict: "pull request 32 merged head xyz, expected abc"}},
	} {
		m := pendingMerge(t)
		calls := fakeForgeGH(t, forgeAnswer{}, prOpen, tc.end)
		runPending(t, m.Apply, tc.want, 30*time.Second)
		calls(ghMerge, ghView, ghView)
	}
}

// After a restart the wait resumes: from Taken, or else from the restart.
func TestInspectResumesPendingMerge(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, prOpen, prMerged)
	runPending(t, m.Inspect, MergeResult{MergeCommit: "def"}, 30*time.Second)
	calls(ghView, ghView)
	calls = fakeForgeGH(t, prOpen, prOpen)
	synctest.Test(t, func(t *testing.T) {
		m.Taken = time.Now().Add(-59*time.Minute - 30*time.Second)
		start := time.Now()
		result, err := m.Inspect(t.Context())
		assertForge(t, err, nil)
		assertForge(t, result, MergeResult{Conflict: "merge still pending after 60 minutes (merge queue or auto-merge)"})
		assertForge(t, time.Since(start), 30*time.Second)
	})
	calls(ghView, ghView)
}

func TestPendingMergeStopsOnCancel(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, prOpen)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		result, err := m.Inspect(ctx)
		assertForge(t, result, MergeResult{})
		assertForge(t, err, context.DeadlineExceeded)
	})
	calls(ghView)
}

// A read that fails, or whose answer cannot be read, during the wait counts
// as unfinished: the PR is read again 30 s later, and the merge the queue
// makes later is still reported.
func TestPendingMergeOutlastsFailedReads(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, forgeAnswer{}, forgeAnswer{Stderr: "forge down", Code: 1}, prOpen, forgeAnswer{Stdout: "{"}, prMerged)
	runPending(t, m.Apply, MergeResult{MergeCommit: "def"}, 90*time.Second)
	calls(ghMerge, ghView, ghView, ghView, ghView)
}

// The merge command and the read after it both fail: the outcome is unknown.
func TestMergeAndReadFail(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, forgeAnswer{Stderr: "merge refused", Code: 1}, forgeAnswer{Stderr: "forge down", Code: 1})
	result, err := m.Apply(t.Context())
	assertForge(t, result, MergeResult{})
	assertForge(t, err.Error(), "gh pr merge 32 --repo team/website --squash --delete-branch --match-head-commit abc: exit status 1: merge refused\n"+
		"gh "+strings.Join(ghView, " ")+": exit status 1: forge down")
	calls(ghMerge, ghView)
}

// An OPEN PR with no merge queue entry and no auto-merge has nothing pending:
// the merge never ran, failed, or left the queue. It is a conflict at once.
func TestOpenPRWithNothingPending(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, prIdle)
	runPending(t, m.Inspect, MergeResult{Conflict: "pull request 32 is OPEN, with no merge queue entry and no auto-merge"}, 0)
	calls(ghView)
}

// gh pr merge fails after it has put the PR in the queue: the merge is still
// pending, and the wait reports the queue's merge.
func TestFailedMergeCommandLeftInQueue(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, forgeAnswer{Stderr: "timeout", Code: 1}, prQueued, prQueued, prMerged)
	runPending(t, m.Apply, MergeResult{MergeCommit: "def"}, 30*time.Second)
	calls(ghMerge, ghView, ghView, ghView)
}
