//go:build acceptance

package features

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// A5.5: three consecutive failures of a task stop it for a person
// (planning/contracts/task-machine.md rows 6, 10 and 14).

const (
	m5Crash      = "a crash"
	m5HostedPass = "passed hosted checks"
	m5HostedFail = "failed hosted checks"
	m5TakenUp    = "Continuing T-1."
)

type m5Failures struct {
	w       *m5World
	commits int
	last    string // the last commit on the task's branch
}

func m5RegisterFailureSteps(sc *godog.ScenarioContext, w *m5World) {
	x := &m5Failures{w: w}
	sc.Step(`^the work of "Claude" on the task ends in turn in "([^"]*)"$`, x.stepEndings)
	sc.Step(`^the third failure in a row made the task failed, and its thread mentions the owner$`, x.stepVerifyFailed)
	sc.Step(`^the secretary tries to resume the failed task and to reassign it to "Codex"$`, x.stepSecretaryTries)
	sc.Step(`^the home refuses both with "forbidden" and the task stays failed$`, x.stepVerifyForbidden)
	sc.Step(`^the owner resumes the failed task$`, x.stepOwnerResumes)
	sc.Step(`^the owner reassigns the failed task to "Codex"$`, x.stepOwnerReassigns)
	sc.Step(`^the task is active without failures, and "(Claude|Codex)" continues from the last commit of "Claude"$`, func(name string) error {
		return m5FinalStep(w, func() error { return x.verifyTakenUp(name) })()
	})
}

// stepEndings gives Claude one turn for each ending, and checks after each
// the number of consecutive failures.
func (x *m5Failures) stepEndings(list string) error {
	x.w.setMemberHeld(x.w.codexM, true)
	failures := 0
	for i, ending := range strings.Split(list, ", ") {
		failures++
		if ending == m5HostedPass {
			failures = 0
		}
		if err := x.endOnce(ending, i, failures); err != nil {
			return fmt.Errorf("ending %d, %s: %w", i+1, ending, err)
		}
	}
	return m5Same("failures at the end", failures, 3)
}

func (x *m5Failures) task() proto.TaskRow {
	t, _ := x.w.taskByNum(1)
	return t
}

func (x *m5Failures) endOnce(ending string, i, failures int) error {
	w := x.w
	if err := x.wakeClaude(i); err != nil {
		return err
	}
	if ending == m5Crash {
		x.scriptCrash()
	} else {
		x.scriptSubmission(ending == m5HostedFail)
	}
	x.release(w.claudeM)
	return w.waitSaw(fmt.Sprintf("%d failures after %s", failures, ending), func() error {
		if t := x.task(); t.Failures == failures && x.ending() == ending {
			return nil
		}
		return errors.New(w.tasksBrief())
	})
}

// ending is how the work on the last commit ended, as the task row shows it.
func (x *m5Failures) ending() string {
	cloud := x.task().Checks.Cloud
	if cloud == nil || cloud.Commit != x.lastCommit() {
		return m5Crash
	}
	if cloud.OK {
		return m5HostedPass
	}
	return m5HostedFail
}

func (x *m5Failures) lastCommit() string {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	return x.last
}

// wakeClaude has exactly one item for Claude wait on the host: the first
// assignment and failed hosted checks give one, else the owner writes.
func (x *m5Failures) wakeClaude(i int) error {
	w := x.w
	if err := m5WaitAllDormant(w.cluster.Nodes[0].Host); err != nil {
		return err
	}
	if i > 0 && x.ending() != m5HostedFail {
		if err := w.ownerPost("@Claude go on.", x.task().ID, []string{w.claudeM}); err != nil {
			return err
		}
	}
	return w.waitSaw("one item for Claude on the host", func() error {
		return m5Same("queued items", w.cluster.Nodes[0].Host.QueuedCount(w.groupID, w.claudeM), 1)
	})
}

func (x *m5Failures) release(member string) {
	x.w.setMemberHeld(member, false)
	host.Deliver(x.w.cluster.Nodes[0].Host, x.w.groupID, member)
}

// commit adds a commit that passes the local checks to the task's branch, and
// holds Claude again until the next ending.
func (x *m5Failures) commit(c host.Caller) (string, error) {
	x.w.setMemberHeld(x.w.claudeM, true)
	x.commits++
	sha, err := m5GitCommitFiles(x.w.sessionFolder(c), map[string]string{"check.txt": fmt.Sprintf("PASS_MARKER %d\n", x.commits)}, "Work")
	x.w.setSHA(&x.last, sha)
	return sha, err
}

// scriptCrash is a turn whose CLI exits before its end, after a commit that
// is never pushed.
func (x *m5Failures) scriptCrash() {
	x.w.addBeforeHook("post", func(c host.Caller) error {
		_, err := x.commit(c)
		return err
	})
	calls, _ := m5BuildMCPCallSteps(m5PostCall("About to crash."), 2)
	x.w.writeScriptFile("claude", `{"exit":1,"steps":[`+strings.Join(append(m5ClaudeTurnStart(0), calls...), ",")+`]}`)
}

func (x *m5Failures) scriptSubmission(failed bool) {
	x.w.addBeforeHook("run_checks", func(c host.Caller) error {
		sha, err := x.commit(c)
		if err != nil || !failed {
			return err
		}
		return x.w.failHostedChecks(sha)
	})
	x.w.writeScriptFile("claude", m5ClaudeScript(m5WorkerCalls(false, "Ready.")))
}

func (x *m5Failures) stepVerifyFailed() error {
	w := x.w
	return w.waitSaw("the failed task to mention the owner", func() error {
		rows := (&m5Jump{w: w}).system("task_failed")
		if len(rows) != 1 || x.task().State != "failed" {
			return fmt.Errorf("task_failed rows %+v, tasks %s", rows, w.tasksBrief())
		}
		thread, owner := rows[0].Thread, w.findMemberByName("Owner")
		if thread == nil || *thread != x.task().ID || !slices.Contains(rows[0].Mentions, owner) {
			return fmt.Errorf("task_failed in thread %v mentions %v, not the owner %s in the task's thread", thread, rows[0].Mentions, owner)
		}
		return nil
	})
}

func (x *m5Failures) stepSecretaryTries() error {
	w := x.w
	if err := m5WaitAllDormant(w.cluster.Nodes[0].Host); err != nil {
		return err
	}
	w.resetTools()
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "set_task", Args: map[string]any{"task": "T-1", "action": "resume"}, WantErr: true},
		{Name: "set_task", Args: map[string]any{"task": "T-1", "assignee": "Codex", "reviewer": "Claude"}, WantErr: true},
	}))
	x.release(w.secID)
	return w.waitFor("the secretary's two set_task calls", func() bool { return w.toolCallCount() >= 2 })
}

func (x *m5Failures) stepVerifyForbidden() error {
	w := x.w
	w.setMemberHeld(w.secID, true)
	w.mu.Lock()
	recs := slices.Clone(w.tools)
	w.mu.Unlock()
	for _, rec := range recs {
		if !rec.IsError || !strings.Contains(rec.Text, "forbidden") {
			return fmt.Errorf("the secretary's %s answered %q, want forbidden", rec.Tool, rec.Text)
		}
	}
	return m5Same("state of the task", x.task().State, "failed")
}

func (x *m5Failures) ownerTaskCmd(fields map[string]any) error {
	fields["t"], fields["g"], fields["id"] = "task", x.w.groupID, x.task().ID
	fields["n"], fields["ts"] = x.w.nextID(), x.w.cluster.Nodes[0].Host.Now().UnixMilli()
	_, err := x.w.ownerCmd(fields)
	return err
}

func (x *m5Failures) stepOwnerResumes() error {
	w := x.w
	if err := m5WaitAllDormant(w.cluster.Nodes[0].Host); err != nil {
		return err
	}
	w.writeScriptFile("claude", m5ClaudeScript(m5PostCall(m5TakenUp)))
	if err := x.ownerTaskCmd(map[string]any{"op": "resume"}); err != nil {
		return err
	}
	x.release(w.claudeM)
	return nil
}

func (x *m5Failures) stepOwnerReassigns() error {
	w := x.w
	w.writeScriptFile("codex", m5CodexScript("codex-takes-over", m5PostCall(m5TakenUp)))
	if err := x.ownerTaskCmd(map[string]any{"op": "assign", "assignee": w.codexM, "reviewer": w.claudeM}); err != nil {
		return err
	}
	x.release(w.codexM)
	return nil
}

func (x *m5Failures) verifyTakenUp(name string) error {
	w := x.w
	if err := w.waitFor(name+" to continue the task", func() bool { return w.hasMsgWithText(m5TakenUp) }); err != nil {
		return err
	}
	t := x.task()
	w.mu.Lock()
	member, dir := w.sawTaskSess[1].Member, w.sawTaskDir[1]
	w.mu.Unlock()
	head, err := m5GitOut(dir, "rev-parse", "--verify", "HEAD")
	return errors.Join(err,
		m5Same("assignee", t.Assignee, w.findMemberByName(name)),
		m5Same("member of the task session", member, t.Assignee),
		m5Same("head of the task's worktree", head, x.lastCommit()),
		m5Same("the task", fmt.Sprint(t.State, t.Failures), fmt.Sprint("active", 0)),
		x.verifyHandedOver(name))
}

// verifyHandedOver checks that the commits Claude never pushed are on the
// remote once the task is Codex's.
func (x *m5Failures) verifyHandedOver(name string) error {
	if name == "Claude" || !x.w.hasRemote() {
		return nil
	}
	return x.w.waitPullRequest("OPEN", x.lastCommit())
}
