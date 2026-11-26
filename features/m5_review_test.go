//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// Submission by report and the review of a report (A5.4, A5.18, A5.19,
// A5.22; planning/contracts/task-machine.md rows 5 and 7).

func m5RegisterReviewSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^an active brief task assigned to "Claude" and reviewed by "Codex"$`, func() error {
		return w.givenOneBriefTask(false)
	})
	sc.Step(`^a submitted brief task assigned to "Claude" and reviewed by "Codex"$`, w.givenSubmittedBriefTask)
	sc.Step(`^a brief task assigned to "Claude" and reviewed by "Codex" that "Claude" submitted twice$`, w.givenSubmittedBriefTask)
	sc.Step(`^"Claude" reviews the report under review and "Codex" reviews the earlier report$`, w.stepAttemptInvalidReviews)
	sc.Step(`^the review by the assignee is refused with "forbidden" and the review of the earlier report with "conflict"$`, w.stepVerifyInvalidReviews)
	sc.Step(`^"Codex" reviews with score 86, then with a veto at score 95, and then four more failing reviews on resubmissions$`, w.stepRunSixFailedReviews)
	sc.Step(`^the 86 review returned the task to work, the veto recorded score 0, and the sixth failed review made the task failed$`, m5FinalStep(w, w.stepVerifySixFailedReviews))

	sc.Step(`^an active brief task assigned to "Claude" with reviewer "Codex" held on the host$`, func() error {
		return w.givenOneBriefTask(true)
	})
	sc.Step(`^"Claude" submits report 1 and then report 2 before "Codex" is released$`, w.stepSubmitTwoReportsWhileReviewerHeld)
	sc.Step(`^the home has retired the review item for report 1 and keeps only report 2 queued$`, w.stepVerifyHomeRetiredReport1Review)
	sc.Step(`^the host hold on "Codex" is released and "Codex" runs its review turn$`, w.stepReleaseReviewerForReport2)
	sc.Step(`^the host refused the held review item for report 1, and "Codex" reviewed only report 2 and the task is done$`, m5FinalStep(w, w.stepVerifyHostRefusedStaleReview))

	sc.Step(`^"Claude" calls submit_task with a summary that holds a test secret$`, w.stepSubmitSecretSummary)
	sc.Step(`^submit_task failed with "secret", the task is still at step work, and the thread holds no report$`, m5FinalStep(w, w.stepVerifySecretRefused))
}

// givenSubmittedBriefTask has Claude submit task 1 twice while Codex, its
// reviewer, is held on the host: report 2 is under review, report 1 is not.
func (w *m5World) givenSubmittedBriefTask() error {
	if err := w.givenOneBriefTask(true); err != nil {
		return err
	}
	return w.stepSubmitTwoReportsWhileReviewerHeld()
}

func (w *m5World) task1AtReview() bool {
	t, ok := w.taskByNum(1)
	return ok && t.Step == "review" && t.Report != ""
}

// reviewAs posts a review of task 1 through the host, signed for member.
func (w *m5World) reviewAs(member, report string) error {
	n0 := w.cluster.Nodes[0]
	t, _ := w.taskByNum(1)
	review := map[string]any{
		"t": "post", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(), "g": w.groupID, "as": member,
		"kind": "review", "text": "", "thread": t.ID, "main": false, "mentions": []string{}, "re": nil,
		"body": map[string]any{"task": t.ID, "report": report, "score": 95, "items": []any{}},
	}
	_, err := n0.Host.Command(context.Background(), n0.Key.ID(), m5MustJSON(review))
	return err
}

func (w *m5World) stepAttemptInvalidReviews() error {
	t, _ := w.taskByNum(1)
	w.mu.Lock()
	r1 := w.report1
	w.mu.Unlock()
	w.refuseCode1 = m5ErrCode(w.reviewAs(w.claudeM, t.Report))
	w.refuseCode2 = m5ErrCode(w.reviewAs(w.codexM, r1))
	return nil
}

func (w *m5World) stepVerifyInvalidReviews() error {
	if w.refuseCode1 != "forbidden" || w.refuseCode2 != "conflict" {
		return fmt.Errorf("got codes (%q, %q), want (forbidden, conflict)", w.refuseCode1, w.refuseCode2)
	}
	return nil
}

func (w *m5World) stepRunSixFailedReviews() error {
	specs := []struct {
		score int
		veto  bool
	}{
		{86, false}, {95, true}, {70, false}, {70, false}, {70, false}, {70, false},
	}
	for i, sp := range specs {
		if err := w.runOneFailedReviewRound(i+1, sp.score, sp.veto); err != nil {
			return err
		}
	}
	return nil
}

func (w *m5World) runOneFailedReviewRound(round, score int, veto bool) error {
	if round > 1 {
		if err := w.resubmit(round); err != nil {
			return err
		}
	}
	w.setMemberHeld(w.claudeM, true)
	items := []any{map[string]any{"text": fmt.Sprintf("issue %d", round), "veto": veto}}
	w.writeScriptFile("codex", m5CodexScript(fmt.Sprintf("codex-r-%d", round), []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": score, "items": items},
	}}))
	w.release(w.codexM)
	return w.waitFor(fmt.Sprintf("review round %d", round), func() bool {
		t, ok := w.taskByNum(1)
		return m5RoundSettled(t, ok, round)
	})
}

// m5RoundSettled holds once the review of a round is on the row: the task is
// back at step work, or after the sixth it has failed.
func m5RoundSettled(t proto.TaskRow, ok bool, round int) bool {
	if !ok || t.Review == nil || t.Review.Round != round {
		return false
	}
	return round == 6 || t.Step == "work"
}

// resubmit has Claude submit a new report after the reviewed item.
func (w *m5World) resubmit(round int) error {
	before, _ := w.taskByNum(1)
	w.setMemberHeld(w.codexM, true)
	w.writeScriptFile("claude", m5ClaudeScript(m5WorkerCalls(fmt.Sprintf("Resubmit %d.", round))))
	w.release(w.claudeM)
	return w.waitFor(fmt.Sprintf("resubmit %d at review", round), func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "review" && t.Report != before.Report
	})
}

func (w *m5World) collectReviewScores() []int {
	var scores []int
	for _, m := range w.browserMessages() {
		var b proto.ReviewBody
		if m.Kind == "review" && json.Unmarshal(m.Body, &b) == nil {
			scores = append(scores, b.Score)
		}
	}
	return scores
}

func (w *m5World) stepVerifySixFailedReviews() error {
	scores := w.collectReviewScores()
	t, ok := w.taskByNum(1)
	if !ok || !slices.Equal(scores, []int{86, 0, 70, 70, 70, 70}) || t.State != "failed" || t.Review.Failed != 6 {
		return fmt.Errorf("unexpected review scores=%v task=%+v", scores, t)
	}
	return nil
}

// stepSubmitTwoReportsWhileReviewerHeld has Claude submit twice in one turn;
// before the second submission the review item for report 1 is queued.
func (w *m5World) stepSubmitTwoReportsWhileReviewerHeld() error {
	w.writeScriptFile("claude", m5ClaudeScript(append(m5WorkerCalls("Report 1."), m5WorkerCalls("Report 2.")...)))
	w.addBeforeHook("submit_task", func(host.Caller) error { return nil })
	w.addBeforeHook("submit_task", func(host.Caller) error {
		if err := w.waitFor("report 1 at step review", w.task1AtReview); err != nil {
			return err
		}
		t, _ := w.taskByNum(1)
		w.setReport(&w.report1, t.Report)
		id, err := w.findQueuedReviewItem(t.Report)
		w.mu.Lock()
		w.refusedItem = id
		w.mu.Unlock()
		return err
	})
	w.release(w.claudeM)
	return w.waitFor("task 1 at step review with report 2", func() bool {
		t, _ := w.taskByNum(1)
		w.mu.Lock()
		r1 := w.report1
		w.mu.Unlock()
		return w.task1AtReview() && r1 != "" && t.Report != r1
	})
}

func (w *m5World) setReport(dst *string, id string) {
	w.mu.Lock()
	*dst = id
	w.mu.Unlock()
}

func (w *m5World) findQueuedReviewItem(report string) (int64, error) {
	items, err := w.homeQueuedItems()
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		if it.Why == "review" && it.Msg != nil && *it.Msg == report {
			return it.ID, nil
		}
	}
	return 0, fmt.Errorf("no queued review item for report %s in %+v", report, items)
}

func (w *m5World) stepVerifyHomeRetiredReport1Review() error {
	items, err := w.homeQueuedItems()
	if err != nil {
		return err
	}
	var reports []string
	for _, it := range items {
		if it.Why == "review" && it.Msg != nil {
			reports = append(reports, *it.Msg)
		}
	}
	t, _ := w.taskByNum(1)
	w.setReport(&w.report2, t.Report)
	if !slices.Equal(reports, []string{t.Report}) {
		return fmt.Errorf("the home queues reviews of %v, want only [%s]", reports, t.Report)
	}
	return nil
}

func (w *m5World) stepReleaseReviewerForReport2() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("codex", m5CodexScript("codex-a519-rev", m5PassingReview(95)))
	w.release(w.codexM)
	return w.waitFor("task 1 done", func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.State == "done"
	})
}

func (w *m5World) hasRefusedItemAudit(itemID int64, agent, detail string) (bool, error) {
	var count int
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM audit WHERE g = ? AND what = 'refused' AND item = ? AND agent = ? AND detail = ?`,
			w.groupID, itemID, agent, detail).Scan(&count)
	}
	err := w.cluster.Nodes[0].Store.Write(read)
	return count == 1, err
}

func (w *m5World) stepVerifyHostRefusedStaleReview() error {
	w.mu.Lock()
	itemID, r2 := w.refusedItem, w.report2
	w.mu.Unlock()
	refused, err := w.hasRefusedItemAudit(itemID, "codex", "stale_task")
	if err != nil || !refused {
		return fmt.Errorf("expected the host to refuse review item %d with stale_task, got %v (err=%v)", itemID, refused, err)
	}
	if reviewed := w.reviewedReports(); !slices.Equal(reviewed, []string{r2}) {
		return fmt.Errorf("reviews of %v, want one of report 2 %s", reviewed, r2)
	}
	return nil
}

func m5FailedWith(rec m5ToolRecord, tool, code string) bool {
	return rec.Tool == tool && rec.IsError && strings.HasPrefix(rec.Text, code)
}

// reviewedReports is the report each review message names, in order.
func (w *m5World) reviewedReports() []string {
	var reports []string
	for _, m := range w.browserMessages() {
		var b proto.ReviewBody
		if m.Kind == "review" && json.Unmarshal(m.Body, &b) == nil {
			reports = append(reports, b.Report)
		}
	}
	return reports
}

func (w *m5World) stepSubmitSecretSummary() error {
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "submit_task", Args: map[string]any{"summary": "Done; the key is " + m3SecretToken}, WantErr: true},
	}))
	w.release(w.claudeM)
	return w.waitFor("Claude's submit_task", func() bool { return w.toolCallCount() >= 1 })
}

func (w *m5World) stepVerifySecretRefused() error {
	w.mu.Lock()
	recs := slices.Clone(w.tools)
	w.mu.Unlock()
	if len(recs) != 1 || !m5FailedWith(recs[0], "submit_task", "secret") {
		return fmt.Errorf("tool calls %+v, want one submit_task that failed with secret", recs)
	}
	if t, _ := w.taskByNum(1); fmt.Sprint(t.State, t.Step, t.Report) != fmt.Sprint("active", "work", "") {
		return fmt.Errorf("task 1 is %s at %s with report %q, want active at work without one", t.State, t.Step, t.Report)
	}
	if slices.ContainsFunc(w.browserMessages(), func(m proto.MsgRow) bool { return m.Kind == "report" }) {
		return fmt.Errorf("the group holds a report")
	}
	return nil
}
