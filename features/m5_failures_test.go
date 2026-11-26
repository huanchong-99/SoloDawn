//go:build acceptance

package features

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// m5NoCommit is what the steps that MODEL-1126 took away say when they fail.
const m5NoCommit = "the task row holds no commit or hosted checks: failures count crashed turns between submissions by report (MODEL-1126 plan 1.6)"

// A5.5: three consecutive failures of a task stop it for a person
// (planning/contracts/task-machine.md rows 6, 10 and 14).

const m5TakenUp = "Continuing T-1."

type m5Failures struct {
	w *m5World
}

func m5RegisterFailureSteps(sc *godog.ScenarioContext, w *m5World) {
	x := &m5Failures{w: w}
	sc.Step(`^the work of "Claude" on the task ends in turn in "([^"]*)"$`, rewritten("AC-1", m5NoCommit))
	sc.Step(`^the third failure in a row made the task failed, and its thread mentions the owner$`, x.stepVerifyFailed)
	sc.Step(`^the secretary tries to resume the failed task and to reassign it to "Codex"$`, x.stepSecretaryTries)
	sc.Step(`^the home refuses both with "forbidden" and the task stays failed$`, x.stepVerifyForbidden)
	sc.Step(`^the owner resumes the failed task$`, x.stepOwnerResumes)
	sc.Step(`^the owner reassigns the failed task to "Codex"$`, x.stepOwnerReassigns)
	sc.Step(`^the task is active without failures, and "(Claude|Codex)" continues from the last commit of "Claude"$`, rewritten("AC-1", m5NoCommit))
}

func (x *m5Failures) task() proto.TaskRow {
	t, _ := x.w.taskByNum(1)
	return t
}

func (x *m5Failures) release(member string) {
	x.w.setMemberHeld(member, false)
	host.Deliver(x.w.cluster.Nodes[0].Host, x.w.groupID, member)
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
