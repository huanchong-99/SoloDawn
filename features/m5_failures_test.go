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

// A5.5: three consecutive crashed turns with no submission in between stop
// a task for a person (planning/contracts/task-machine.md rows 5, 10 and 14).

const (
	m5Crash      = "a crash"
	m5Submission = "a submission"
	m5TakenUp    = "Continuing T-1."
)

type m5Failures struct {
	w *m5World
}

func m5RegisterFailureSteps(sc *godog.ScenarioContext, w *m5World) {
	x := &m5Failures{w: w}
	sc.Step(`^the work of "Claude" on the task ends in turn in "([^"]*)"$`, x.stepEndings)
	sc.Step(`^the third failure in a row made the task failed, and its thread mentions the owner$`, x.stepVerifyFailed)
	sc.Step(`^the secretary tries to resume the failed task and to reassign it to "Codex"$`, x.stepSecretaryTries)
	sc.Step(`^the home refuses both with "forbidden" and the task stays failed$`, x.stepVerifyForbidden)
	sc.Step(`^the owner resumes the failed task$`, x.stepOwnerResumes)
	sc.Step(`^the owner reassigns the failed task to "Codex"$`, x.stepOwnerReassigns)
	sc.Step(`^the task is active without failures, and "(Claude|Codex)" continues it in its own workspace$`, func(name string) error {
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
		if ending == m5Submission {
			failures = 0
		}
		if err := x.endOnce(ending, i, failures); err != nil {
			return fmt.Errorf("ending %d, %s: %w", i+1, ending, err)
		}
	}
	return m5Same("failures at the end", failures, 3)
}

func (x *m5Failures) endOnce(ending string, i, failures int) error {
	w := x.w
	if err := x.wakeClaude(i); err != nil {
		return err
	}
	report := x.task().Report
	x.scriptEnding(ending)
	w.release(w.claudeM)
	return w.waitSaw(fmt.Sprintf("%d failures after %s", failures, ending), func() error {
		t := x.task()
		if t.Failures == failures && (t.Report != report) == (ending == m5Submission) {
			return nil
		}
		return errors.New(w.tasksBrief())
	})
}

// wakeClaude has exactly one item for Claude wait on the host: the first
// assignment, else a message of the owner in the task's thread.
func (x *m5Failures) wakeClaude(i int) error {
	w := x.w
	if err := m5WaitAllDormant(w.cluster.Nodes[0].Host); err != nil {
		return err
	}
	if i > 0 {
		if err := w.ownerPost("@Claude go on.", x.task().ID, []string{w.claudeM}); err != nil {
			return err
		}
	}
	return w.waitSaw("one item for Claude on the host", func() error {
		return m5Same("queued items", w.cluster.Nodes[0].Host.QueuedCount(w.groupID, w.claudeM), 1)
	})
}

// scriptEnding is a turn whose CLI exits before its end, or one that
// submits the task by report. Its tool call holds Claude again until the
// next ending.
func (x *m5Failures) scriptEnding(ending string) {
	w := x.w
	hold := func(host.Caller) error {
		w.setMemberHeld(w.claudeM, true)
		return nil
	}
	if ending == m5Submission {
		w.addBeforeHook("submit_task", hold)
		w.writeScriptFile("claude", m5ClaudeScript(m5WorkerCalls("Ready in my folder.")))
		return
	}
	w.addBeforeHook("post", hold)
	calls, _ := m5BuildMCPCallSteps(m5PostCall("About to crash."), 2)
	w.writeScriptFile("claude", `{"exit":1,"steps":[`+strings.Join(append(m5ClaudeTurnStart(0), calls...), ",")+`]}`)
}

func (x *m5Failures) task() proto.TaskRow {
	t, _ := x.w.taskByNum(1)
	return t
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
	x.w.release(w.secID)
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
	x.w.release(w.claudeM)
	return nil
}

func (x *m5Failures) stepOwnerReassigns() error {
	w := x.w
	w.writeScriptFile("codex", m5CodexScript("codex-takes-over", m5PostCall(m5TakenUp)))
	if err := x.ownerTaskCmd(map[string]any{"op": "assign", "assignee": w.codexM, "reviewer": w.claudeM}); err != nil {
		return err
	}
	x.w.release(w.codexM)
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
	return errors.Join(
		m5Same("assignee", t.Assignee, w.findMemberByName(name)),
		m5Same("member of the task session", member, t.Assignee),
		m5Same("folder of the task session", dir, w.workspaces[name]),
		m5Same("the task", fmt.Sprint(t.State, t.Failures), fmt.Sprint("active", 0)))
}
