package host

import (
	"context"
	"os"
	"path/filepath"
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
	ghMerge  = []string{"pr", "merge", "32", "--repo", "team/website", "--squash", "--delete-branch", "--match-head-commit", "abc"}
	ghView   = []string{"pr", "view", "32", "--json", "state,mergeCommit,headRefOid"}
	prOpen   = forgeAnswer{Stdout: `{"state":"OPEN","mergeCommit":null,"headRefOid":"abc"}`}
	prMerged = forgeAnswer{Stdout: `{"state":"MERGED","mergeCommit":{"oid":"def"},"headRefOid":"abc"}`}
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
	calls := fakeForgeGH(t, forgeAnswer{}, prOpen, prOpen, prMerged)
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

func TestPendingMergeStops(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, prOpen, prOpen, forgeAnswer{Stderr: "forge down", Code: 1})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		result, err := m.Inspect(ctx)
		assertForge(t, result, MergeResult{})
		assertForge(t, err, context.DeadlineExceeded)
	})
	synctest.Test(t, func(t *testing.T) {
		result, err := m.Inspect(t.Context())
		assertForge(t, result, MergeResult{})
		assertForge(t, err.Error(), "gh pr view 32 --json state,mergeCommit,headRefOid: exit status 1: forge down")
	})
	calls(ghView, ghView, ghView)
}

// The merge command and the read after it both fail: the outcome is unknown.
func TestMergeAndReadFail(t *testing.T) {
	m := pendingMerge(t)
	calls := fakeForgeGH(t, forgeAnswer{Stderr: "merge refused", Code: 1}, forgeAnswer{Stderr: "forge down", Code: 1})
	result, err := m.Apply(t.Context())
	assertForge(t, result, MergeResult{})
	assertForge(t, err.Error(), "gh pr merge 32 --repo team/website --squash --delete-branch --match-head-commit abc: exit status 1: merge refused\n"+
		"gh pr view 32 --json state,mergeCommit,headRefOid: exit status 1: forge down")
	calls(ghMerge, ghView)
}
