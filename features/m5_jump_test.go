//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// A5.11: the owner jumps the queue at an AI that is running a task turn
// (planning/flows/agent-turn.md C).

const m5ContinueTaskTurn = "[SoloDawn] group \"Proj\" · you are \"Claude\" · task session T-1\n== 1 of 1 · continue · instruction\nYou were interrupted by a direct request from your owner. Continue the task."

type m5Jump struct {
	w         *m5World
	task      string        // the task Claude works on
	waiting   string        // an older message that waits in the same thread
	jumped    string        // the message the owner jumped
	meanwhile proto.TaskRow // the task as the browser showed it during the jump turn
}

func m5RegisterJumpSteps(sc *godog.ScenarioContext, w *m5World) {
	j := &m5Jump{w: w}
	sc.Step(`^"Claude" is running its task turn and the owner jumps the queue on a direct request to it$`, j.stepJumpDirectRequest)
	sc.Step(`^the running turn is interrupted and the task shows as pre-empted while "Claude" answers the request in its group session$`, j.stepVerifyPreemptedMeanwhile)
	sc.Step(`^the task continues by itself with one turn of its own, and nothing is reported as a failure or delivered twice$`, m5FinalStep(w, j.stepVerifyContinued))
	sc.Step(`^"Claude" is running its task turn and the owner jumps the queue on a message in the task thread$`, j.stepJumpThreadMessage)
	sc.Step(`^the running turn is interrupted and the jumped message goes first in the next turn of the same session$`, j.stepVerifyWentFirst)
	sc.Step(`^the task was never paused, and nothing is reported as a failure$`, m5FinalStep(w, j.stepVerifyNeverPaused))
}

// m5InterruptedClaudeScript is a Claude Code session whose first turn runs
// until it is interrupted, and whose next turn, which must be next, makes
// calls.
func m5InterruptedClaudeScript(next map[string]any, calls []m5ToolCallSpec) string {
	steps := append(m5ClaudeTurnStart(0),
		m5StepJSON("stdin", map[string]any{"type": "control_request", "request": map[string]any{"subtype": "interrupt"}}),
		m5StepJSON("stdout", map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true, "session_id": "$session"}),
		m5StepJSON("stdin", next),
		m5StepJSON("stdout", map[string]any{"type": "assistant", "session_id": "$session"}),
	)
	callSteps, _ := m5BuildMCPCallSteps(calls, 2)
	steps = append(steps, callSteps...)
	steps = append(steps, m5StepJSON("stdout", map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": "$session"}))
	return `{"steps":[` + strings.Join(steps, ",") + `]}`
}

func m5ClaudeUserTurn(content ...map[string]string) map[string]any {
	message := map[string]any{"role": "user"}
	if len(content) > 0 {
		message["content"] = content
	}
	return map[string]any{"type": "user", "message": message}
}

func m5PostCall(text string) []m5ToolCallSpec {
	return []m5ToolCallSpec{{Name: "post", Args: map[string]any{"text": text}}}
}

// startTaskTurn starts Claude's task session with script and waits until its
// CLI has read the turn, so that an interrupt reaches a running turn.
func (j *m5Jump) startTaskTurn(script string) error {
	w := j.w
	task, _ := w.taskByNum(1)
	j.task = task.ID
	w.writeScriptFile("claude", script)
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	key := host.SessionKey{Group: w.groupID, Member: w.claudeM, Task: j.task}
	return w.waitFor("Claude's task turn", func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.listedTools[key]) > 0
	})
}

// ownerMentionsClaude posts a message of the owner that mentions Claude, in
// the thread of a task or in the main line, waits until the host holds its
// item behind the running turn, and returns its id.
func (j *m5Jump) ownerMentionsClaude(text, thread string, held int) (string, error) {
	w := j.w
	n0 := w.cluster.Nodes[0]
	var in any
	if thread != "" {
		in = thread
	}
	_, err := w.ownerCmd(map[string]any{
		"t": "post", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(), "g": w.groupID,
		"kind": "text", "text": text, "thread": in, "main": thread == "",
		"mentions": []string{w.claudeM}, "re": nil, "body": map[string]any{},
	})
	if err != nil {
		return "", err
	}
	var id string
	err = w.waitFor("the host to hold the item of "+text, func() bool {
		id = j.message(func(m proto.MsgRow) bool { return m.Text == text }).ID
		return id != "" && n0.Host.QueuedCount(w.groupID, w.claudeM) == held
	})
	return id, err
}

func (j *m5Jump) ownerJumps(msg string) error {
	w := j.w
	j.jumped = msg
	_, err := w.ownerCmd(map[string]any{
		"t": "jump", "n": w.nextID(), "ts": w.cluster.Nodes[0].Host.Now().UnixMilli(), "g": w.groupID, "msg": msg,
	})
	return err
}

func (j *m5Jump) message(match func(proto.MsgRow) bool) proto.MsgRow {
	msgs := j.w.browserMessages()
	if i := slices.IndexFunc(msgs, match); i >= 0 {
		return msgs[i]
	}
	return proto.MsgRow{}
}

func (j *m5Jump) waitPost(text string) error {
	w := j.w
	return w.waitFor("Claude to post "+text, func() bool {
		return j.message(func(m proto.MsgRow) bool { return m.By == w.claudeM && m.Text == text }).ID != ""
	})
}

// system is the system rows with a code.
func (j *m5Jump) system(code string) []proto.MsgRow {
	var rows []proto.MsgRow
	for _, m := range j.w.browserMessages() {
		var body proto.SystemCode
		if m.Kind == "system" && json.Unmarshal(m.Body, &body) == nil && body.Code == code {
			rows = append(rows, m)
		}
	}
	return rows
}

// turns is the turns Claude was given, in order, each as its kind, its
// session ("task" or "group") and the messages of its items.
func (j *m5Jump) turns() []string {
	w := j.w
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, tr := range w.turnTrace {
		if tr.Key.Member != w.claudeM {
			continue
		}
		parts := []string{string(tr.Kind), map[bool]string{true: "task", false: "group"}[tr.Key.Task == j.task]}
		for _, item := range tr.Items {
			if item.Msg != "" && item.Why == "mention" {
				parts = append(parts, string(item.Msg))
			}
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func (j *m5Jump) stepJumpDirectRequest() error {
	w := j.w
	if err := j.startTaskTurn(m5InterruptedClaudeScript(
		m5ClaudeUserTurn(map[string]string{"type": "text", "text": m5ContinueTaskTurn}), m5PostCall("Continuing T-1."),
	)); err != nil {
		return err
	}
	w.writeScriptFile("claude", m5ClaudeScript(m5PostCall("Direct answer.")))
	w.addBeforeHook("post", j.noteTaskMeanwhile)
	msg, err := j.ownerMentionsClaude("@Claude what is the status?", "", 1)
	if err != nil {
		return err
	}
	return j.ownerJumps(msg)
}

// noteTaskMeanwhile runs before the post of the jump turn: that turn has not
// ended, so the task is still as the pre-emption left it.
func (j *m5Jump) noteTaskMeanwhile(host.Caller) error {
	w := j.w
	return w.waitFor("task 1 to show as pre-empted", func() bool {
		task, _ := w.taskByNum(1)
		w.mu.Lock()
		j.meanwhile = task
		w.mu.Unlock()
		return task.State == "paused"
	})
}

func (j *m5Jump) stepVerifyPreemptedMeanwhile() error {
	w := j.w
	if err := j.waitPost("Direct answer."); err != nil {
		return err
	}
	w.mu.Lock()
	meanwhile := j.meanwhile
	w.mu.Unlock()
	if meanwhile.Wait == nil || meanwhile.Wait.Kind != "preempted" {
		return fmt.Errorf("during the jump turn the task was %s with wait %+v", meanwhile.State, meanwhile.Wait)
	}
	return j.verifyPreemptedRow()
}

// verifyPreemptedRow checks the one system row of the pre-emption: in the
// task's thread, naming the task and the author of the jumped message.
func (j *m5Jump) verifyPreemptedRow() error {
	asked := j.message(func(m proto.MsgRow) bool { return m.ID == j.jumped })
	want := proto.PreemptedBody{SystemCode: proto.SystemCode{Code: "preempted"}, Task: j.task, By: asked.By}
	rows := j.system("preempted")
	if len(rows) != 1 {
		return fmt.Errorf("preempted rows = %+v, want one", rows)
	}
	var body proto.PreemptedBody
	if err := json.Unmarshal(rows[0].Body, &body); err != nil {
		return err
	}
	if thread := rows[0].Thread; body != want || thread == nil || *thread != j.task {
		return fmt.Errorf("preempted row = %+v in thread %v, want %+v in the task's thread", body, thread, want)
	}
	return nil
}

func (j *m5Jump) stepVerifyContinued() error {
	if err := j.waitPost("Continuing T-1."); err != nil {
		return err
	}
	want := []string{"items task", "items group " + j.jumped, "continue task"}
	if err := j.waitTaskActive(); err != nil {
		return err
	}
	return j.verifyTurns(want)
}

func (j *m5Jump) waitTaskActive() error {
	w := j.w
	return w.waitFor("task 1 to be active without a wait", func() bool {
		task, _ := w.taskByNum(1)
		return task.State == "active" && task.Wait == nil
	})
}

// verifyTurns checks the turns Claude was given, and that no turn was
// reported as interrupted or counted as a failure of the task.
func (j *m5Jump) verifyTurns(want []string) error {
	if got := j.turns(); !slices.Equal(got, want) {
		return fmt.Errorf("Claude's turns = %q, want %q", got, want)
	}
	if rows := j.system("turn_interrupted"); len(rows) != 0 {
		return fmt.Errorf("turn_interrupted rows = %+v, want none", rows)
	}
	if task, _ := j.w.taskByNum(1); task.Failures != 0 {
		return fmt.Errorf("the task has %d failures, want 0", task.Failures)
	}
	return nil
}

func (j *m5Jump) stepJumpThreadMessage() error {
	if err := j.startTaskTurn(m5InterruptedClaudeScript(m5ClaudeUserTurn(), m5PostCall("Answered first."))); err != nil {
		return err
	}
	waiting, err := j.ownerMentionsClaude("@Claude a note for later.", j.task, 1)
	if err != nil {
		return err
	}
	j.waiting = waiting
	msg, err := j.ownerMentionsClaude("@Claude answer this first.", j.task, 2)
	if err != nil {
		return err
	}
	return j.ownerJumps(msg)
}

func (j *m5Jump) stepVerifyWentFirst() error {
	if err := j.waitPost("Answered first."); err != nil {
		return err
	}
	return j.verifyTurns([]string{"items task", "items task " + j.jumped + " " + j.waiting})
}

func (j *m5Jump) stepVerifyNeverPaused() error {
	if rows := j.system("preempted"); len(rows) != 0 {
		return fmt.Errorf("preempted rows = %+v, want none", rows)
	}
	return j.waitTaskActive()
}
