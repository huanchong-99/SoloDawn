//go:build acceptance

package features

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// A dependent task (A5.6) and a direct task (A5.7).

const (
	m5Task2Started = "Started T-2."
	m5DirectFile   = "direct.txt"
	m5DirectDone   = "Changed direct.txt."
	m5SecNoted     = "Noted T-1."
)

func m5RegisterDepAndDirectSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a confirmed brief where task 2 depends on task 1$`, w.stepGivenConfirmedDepBrief)
	sc.Step(`^the secretary reassigns task 2 to "Codex" and reviewer "Claude" while task 2 is queued$`, w.stepReassignQueuedTask2)
	sc.Step(`^task 2 remains queued with no task session$`, w.stepVerifyTask2StillQueued)
	sc.Step(`^task 1 is submitted and passes its review$`, w.stepFinishTask1)
	sc.Step(`^task 1 is done, and task 2 becomes active and runs its task session for "Codex" in the workspace of "Codex"$`, m5FinalStep(w, w.stepVerifyTask2StartedForCodex))

	sc.Step(`^the owner mentions "Claude" directly for a change and "Claude" calls start_task$`, w.stepRunDirectTask)
	sc.Step(`^the group session listed start_task and no tool that submits, a task card exists, and the secretary was notified$`, w.stepVerifyDirectTaskCard)
	sc.Step(`^the change was made in a new task session in the folder of the group session of "Claude"$`, m5FinalStep(w, w.stepVerifyDirectTaskFolder))
}

func (w *m5World) stepGivenConfirmedDepBrief() error {
	if err := w.setupCluster(true); err != nil {
		return err
	}
	brief := m5BriefArgs([]map[string]any{
		{"title": "Base unit", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
		{"title": "Dependent unit", "assignee": "Claude", "reviewer": "Codex", "depends": []int{0}},
	})
	w.writeScriptFile("claude", m5ClaudeScript(
		[]m5ToolCallSpec{{Name: "propose_brief", Args: brief}},
		nil,
		[]m5ToolCallSpec{{Name: "set_task", Args: map[string]any{"task": "T-2", "assignee": "Codex", "reviewer": "Claude"}}},
	))
	if err := w.ownerPostMain("Build base and dependent units.", nil); err != nil {
		return err
	}
	if err := w.waitPendingBrief(); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, true)
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	return w.waitFor("2 tasks created and secretary confirmed turn settled", w.depBriefConfirmedReady)
}

func (w *m5World) depBriefConfirmedReady() bool {
	state, _ := w.cluster.Nodes[0].Host.MemberState(w.groupID, w.secID)
	return len(w.browserTasks()) == 2 && w.turnCount() >= 2 && state == string(host.Waiting)
}

func (w *m5World) stepReassignQueuedTask2() error {
	if err := w.ownerPostMain("Give T-2 to Codex.", nil); err != nil {
		return err
	}
	return w.waitFor("task 2 reassigned to Codex", func() bool {
		t2, ok := w.taskByNum(2)
		return ok && t2.Assignee == w.codexM && t2.Reviewer == w.claudeM
	})
}

func (w *m5World) stepVerifyTask2StillQueued() error {
	t2, ok := w.taskByNum(2)
	if !ok || t2.State != "queued" || t2.Wait == nil || t2.Wait.Kind != "depends" {
		return fmt.Errorf("expected task 2 queued on its dependency, got %s", w.tasksBrief())
	}
	if slices.ContainsFunc(w.cluster.Nodes[0].Host.Sessions.All(), func(s host.Session) bool { return s.Task == t2.ID }) {
		return fmt.Errorf("queued task 2 has a session")
	}
	return nil
}

// stepFinishTask1 has Claude submit task 1 and Codex pass it; Codex's task
// session of task 2 then posts once.
func (w *m5World) stepFinishTask1() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("claude", m5ClaudeScript(m5WorkerCalls("Base unit done in my folder.")))
	w.writeScriptFile("codex", m5CodexScript("codex-dep-rev1", m5PassingReview(95)))
	w.addBeforeHook("submit_review", func(host.Caller) error {
		w.writeScriptFile("codex", m5CodexScript("codex-dep-t2", m5PostCall(m5Task2Started)))
		return nil
	})
	w.release(w.claudeM)
	return w.waitFor("task 1 done and task 2 active", func() bool {
		t1, ok1 := w.taskByNum(1)
		t2, ok2 := w.taskByNum(2)
		return ok1 && ok2 && t1.State == "done" && t2.State == "active"
	})
}

func (w *m5World) stepVerifyTask2StartedForCodex() error {
	if err := w.waitFor("Codex to post in task 2", func() bool { return w.hasMsgWithText(m5Task2Started) }); err != nil {
		return err
	}
	w.mu.Lock()
	sess, dir := w.sawTaskSess[2], w.sawTaskDir[2]
	w.mu.Unlock()
	t2, _ := w.taskByNum(2)
	if sess.Member != w.codexM || sess.Task != t2.ID || dir != w.workspaces["Codex"] {
		return fmt.Errorf("task 2 ran in session %+v in %q, want Codex's in %q", sess, dir, w.workspaces["Codex"])
	}
	return nil
}

func (w *m5World) hasMsgWithText(text string) bool {
	return slices.ContainsFunc(w.browserMessages(), func(m proto.MsgRow) bool { return m.Text == text })
}

// stepRunDirectTask has Claude call start_task in its group session; its
// task session then writes a file in its folder and posts that it did.
func (w *m5World) stepRunDirectTask() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "start_task", Args: map[string]any{"title": "Direct helper", "plan": "Add direct helper and test."}},
	}))
	w.addBeforeHook("start_task", func(host.Caller) error {
		w.writeScriptFile("claude", m5ClaudeScript(m5PostCall(m5DirectDone)))
		return nil
	})
	w.addBeforeHook("post", func(c host.Caller) error {
		return os.WriteFile(filepath.Join(c.Folder, m5DirectFile), []byte("direct\n"), 0o600)
	})
	if err := w.ownerPostMain("@Claude add a direct helper.", []string{w.claudeM}); err != nil {
		return err
	}
	if err := w.waitFor("the change in the direct task", func() bool { return w.hasMsgWithText(m5DirectDone) }); err != nil {
		return err
	}
	return w.deliverDirectNoticeToSec()
}

func (w *m5World) deliverDirectNoticeToSec() error {
	w.writeScriptFile("claude", m5ClaudeScript(m5PostCall(m5SecNoted)))
	w.release(w.secID)
	return w.waitFor("the secretary's turn on the task_started notice", func() bool {
		return slices.ContainsFunc(w.browserMessages(), func(m proto.MsgRow) bool { return m.By == w.secID && m.Text == m5SecNoted })
	})
}

func (w *m5World) stepVerifyDirectTaskCard() error {
	t1, _ := w.taskByNum(1)
	w.mu.Lock()
	grpTools := slices.Clone(w.listedTools[host.SessionKey{Group: w.groupID, Member: w.claudeM}])
	w.mu.Unlock()
	if !slices.Contains(grpTools, "start_task") || slices.ContainsFunc(grpTools, func(t string) bool { return strings.HasPrefix(t, "submit_") }) {
		return fmt.Errorf("unexpected group session tools: %v", grpTools)
	}
	if t1.Origin != "direct" || !slices.ContainsFunc(w.browserMessages(), func(m proto.MsgRow) bool { return m.Kind == "task" && m.ID == t1.ID }) {
		return fmt.Errorf("no task card for the direct task %+v", t1)
	}
	if !w.secretaryNoticed(t1.ID) {
		return fmt.Errorf("the secretary had no notice of task %s", t1.ID)
	}
	return nil
}

// secretaryNoticed says whether a turn of the secretary held a notice of the task.
func (w *m5World) secretaryNoticed(task string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.ContainsFunc(w.turnTrace, func(tr m5TurnTrace) bool {
		return tr.Key.Member == w.secID && slices.ContainsFunc(tr.Items, func(it m5ItemTrace) bool {
			return it.Why == "notice" && string(it.Task) == task
		})
	})
}

func (w *m5World) stepVerifyDirectTaskFolder() error {
	t1, _ := w.taskByNum(1)
	group, _ := w.cluster.Nodes[0].Host.Sessions.Get(host.SessionKey{Group: w.groupID, Member: w.claudeM})
	w.mu.Lock()
	sess, dir := w.sawTaskSess[1], w.sawTaskDir[1]
	w.mu.Unlock()
	if sess.Task != t1.ID || sess.Member != w.claudeM || dir != group.Folder || dir != w.workspaces["Claude"] {
		return fmt.Errorf("the change ran in session %+v in %q, want Claude's task session in %q", sess, dir, group.Folder)
	}
	_, err := os.Stat(filepath.Join(dir, m5DirectFile))
	return err
}
