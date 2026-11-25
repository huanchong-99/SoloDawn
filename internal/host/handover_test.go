package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// blockHook makes the Git hook name of the clone wait, once it has started,
// until go is called.
func blockHook(t *testing.T, repo, name string) (waitStarted, release func()) {
	t.Helper()
	dir := filepath.ToSlash(t.TempDir())
	script := "#!/bin/sh\n: > '" + dir + "/started'\nwhile [ ! -e '" + dir + "/go' ]; do :; done\n"
	mustApproveNoErr(t, os.WriteFile(filepath.Join(repo, ".git", "hooks", name), []byte(script), 0o700))
	release = func() { os.WriteFile(dir+"/go", nil, 0o600) }
	t.Cleanup(release)
	return func() {
		poll(t, 10*time.Second, "the hook "+name, func() string {
			if !pathExists(dir + "/started") {
				return "not started"
			}
			return ""
		})
	}, release
}

// wantCloneLocked checks that a Git command that is running holds the clone's
// lock.
func wantCloneLocked(t *testing.T, repo string) {
	t.Helper()
	if lock := cloneLock(repo); lock.TryLock() {
		lock.Unlock()
		t.Fatal("the clone is not locked")
	}
}

type handOverRig struct {
	*deliverRig
	pDir, remote, wt, head string
}

// newHandOverRig makes a host whose first AI has a commit in its worktree that
// it has not pushed.
func newHandOverRig(t *testing.T) *handOverRig {
	r, pDir, remote, _ := setupStep17Rig(t)
	wt, err := CreateTaskWorkspace(t.Context(), pDir, 1, "alpha", testAI1, "main", true)
	mustApproveNoErr(t, err)
	mustApproveNoErr(t, os.WriteFile(filepath.Join(wt, "a.txt"), []byte("v1\n"), 0o600))
	runTestGit(t, wt, "add", "a.txt")
	runTestGit(t, wt, "commit", "-m", "alpha")
	head := strings.TrimSpace(runTestGit(t, wt, "rev-parse", "HEAD"))
	r.h.Sessions.Create(Session{SessionKey: SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}, Kind: TaskSession, Folder: "wt/1-alpha", Agent: "claude", Home: r.homeKey.ID(), Name: "Claude"})
	return &handOverRig{deliverRig: r, pDir: pDir, remote: remote, wt: wt, head: head}
}

func (r *handOverRig) row(assignee string, seq int64) proto.TaskRow {
	return proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: testTask1, Seq: seq}, Group: testGroup1, Num: 1, Title: "Alpha", Slug: "alpha",
		Origin: "brief", State: "active", Step: "local_check", Assignee: assignee, Depends: []string{},
	}
}

func (r *handOverRig) assign(item int64) {
	r.pushItem(r.homeKey, r.homeKey.ID(), testGroup1, proto.Item{Item: item, AI: testAI2, Why: "assign", Task: testTask1}, nil)
}

func (r *handOverRig) pushed() string {
	out, _, _ := runIn(r.t.Context(), r.remote, "git", "rev-parse", "--verify", "refs/heads/task/1-alpha")
	return strings.TrimSpace(out)
}

// wantTakenUp waits for the new assignee's turn and checks that its worktree is
// the branch with the other's commit, and that this was published.
func (r *handOverRig) wantTakenUp() {
	r.t.Helper()
	r.waitUntil("the new assignee's turn", func() bool { return len(r.runtime.recorded()) == 1 })
	r.waitAIUnbusy(testGroup1, testAI2)
	settleDelivery(r.h)
	joinAssertEqual(r.t, "published", r.pushed(), r.head)
	joinAssertEqual(r.t, "head of the new worktree", strings.TrimSpace(runTestGit(r.t, r.wt, "rev-parse", "HEAD")), r.head)
	joinAssertEqual(r.t, "holder", newSlot(r.pDir, "wt", "1-alpha", "", "").holder(r.t.Context()), testAI2)
	joinAssertEqual(r.t, "session of the old assignee", hasSession(r.h, SessionKey{Group: testGroup1, Member: testAI1, Task: testTask1}), false)
}

// The assign item of the new assignee is delivered before the host has cleaned
// up after the old one: the old worktree is published before it goes.
func TestTaskIsPublishedBeforeTheNewAssigneesWorktree(t *testing.T) {
	r := newHandOverRig(t)
	r.putTask(r.row(testAI2, 10))
	r.assign(1)
	r.wantTakenUp()
}

// The clean-up is pushing when the assign item is delivered: the new worktree
// waits for it.
func TestNewAssigneesWorktreeWaitsForThePush(t *testing.T) {
	r := newHandOverRig(t)
	started, release := blockHook(t, filepath.Join(r.pDir, "repo"), "pre-push")
	pushTaskState(r.deliverRig, r.row(testAI2, 10))
	started()
	wantCloneLocked(t, filepath.Join(r.pDir, "repo"))
	r.assign(1)
	release()
	r.wantTakenUp()
}

// A push that fails is told once, in the task's thread. The new assignee goes on
// from the branch of this node.
func TestFailedPushOfAHandOverIsTold(t *testing.T) {
	logs := captureLogs(t)
	r := newHandOverRig(t)
	hook := filepath.Join(r.pDir, "repo", ".git", "hooks", "pre-push")
	mustApproveNoErr(t, os.WriteFile(hook, []byte("#!/bin/sh\necho not today >&2\nexit 1\n"), 0o700))
	pushTaskState(r.deliverRig, r.row(testAI2, 10))
	r.assign(1)
	r.waitUntil("the new assignee's turn", func() bool { return len(r.runtime.recorded()) == 1 })
	settleDelivery(r.h)
	joinAssertEqual(t, "published", r.pushed(), "")
	joinAssertEqual(t, "head of the new worktree", strings.TrimSpace(runTestGit(t, r.wt, "rev-parse", "HEAD")), r.head)
	told := 0
	for _, p := range r.home.cmdPayloads() {
		if strings.Contains(p, `"turn_interrupted"`) && strings.Contains(p, "not today") && strings.Contains(p, `"id":"`+testAI1+`"`) {
			told++
		}
	}
	joinAssertEqual(t, "told", told, 1)
	joinAssertEqual(t, "logged", strings.Contains(logs.String(), "not today"), true)
}

// A clean-up that waited for the clone's lock while the task went on to another
// AI, or while the rows were read anew, leaves the worktree alone.
func TestHandOverGoesByTheAssigneeOfNow(t *testing.T) {
	r := newHandOverRig(t)
	r.putTask(r.row(testAI1, 10))
	grp, _ := lookupRow[proto.GroupRow](r.h, testGroup1, "group", testGroup1)
	handOver(t.Context(), r.h, r.pDir, grp, r.row(testAI2, 9), "1-alpha")
	r.h.Rows.Reset(testGroup1)
	handOver(t.Context(), r.h, r.pDir, grp, r.row(testAI2, 9), "1-alpha")
	joinAssertEqual(t, "worktree kept", pathExists(r.wt), true)
	joinAssertEqual(t, "published", r.pushed(), "")
}

// A push and a fetch write the same refs of the clone: each holds its lock.
// So does the fetch after a custom forge's merge.
func TestPushAndFetchOfTheCloneHoldItsLock(t *testing.T) {
	r := newHandOverRig(t)
	repo := filepath.Join(r.pDir, "repo")
	started, release := blockHook(t, repo, "pre-push")
	errs := make(chan error, 2)
	go func() {
		errs <- (TaskForge{Worktree: r.wt, Branch: "task/1-alpha", Base: "main"}).PushBranch(t.Context())
	}()
	started()
	wantCloneLocked(t, repo)
	go func() { errs <- RefreshGroupWorkspace(t.Context(), r.pDir, "main", true) }()
	release()
	mustApproveNoErr(t, <-errs)
	mustApproveNoErr(t, <-errs)
	joinAssertEqual(t, "published", r.pushed(), r.head)

	runTestGit(t, r.remote, "update-ref", "refs/heads/main", r.head)
	started, release = blockHook(t, repo, "reference-transaction")
	fakeForgeGH(t, forgeAnswer{})
	var res MergeResult
	go func() {
		var err error
		res, err = (ReviewedMerge{Repo: repo, Base: "main", Branch: "task/1-alpha", Commit: r.head, Templates: forgeTestTemplates()}).Apply(t.Context())
		errs <- err
	}()
	started()
	wantCloneLocked(t, repo)
	release()
	mustApproveNoErr(t, <-errs)
	joinAssertEqual(t, "merge commit", res.MergeCommit, r.head)
}
