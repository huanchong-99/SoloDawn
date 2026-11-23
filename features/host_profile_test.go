//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["host_profile"] = hostProfileSteps
}

func hostProfileSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	w := &profileTurnWorld{}
	st := &profileSubmitWorld{tskScenario{t: f.t, cleanup: cleanup}}
	sc.Step(`^the host has the secretary's synced AI profiles$`, func() error { return w.setupRoster(cleanup) })
	sc.Step(`^the secretary takes its next turn$`, w.composeConfirmedBrief)
	sc.Step(`^the turn begins with:$`, w.checkRoster)
	sc.Step(`^the secretary instructions contain the assignment rules$`, w.checkAssignmentRules)
	sc.Step(`^list_members still returns:$`, w.checkListMembers)
	sc.Step(`^the host has a standard assignee and an offline strong secretary$`, func() error { return w.setupEscalation(cleanup) })
	sc.Step(`^the host composes a turn after the (\d+)(?:st|nd|rd) failed (review|hosted check)$`, w.composeFailure)
	sc.Step(`^the turn has no escalation line$`, w.checkNoEscalation)
	sc.Step(`^the turn ends with the escalation line:$`, w.checkEscalation)
	sc.Step(`^the task instructions say to ask before submitting$`, w.checkAskRule)
	sc.Step(`^a remote task with a checked commit and an unanswered escalation$`, st.setupBlocked)
	sc.Step(`^the assignee submits the task before the answer$`, st.submitEarly)
	sc.Step(`^the submission returns conflict naming the offline strong secretary with no push, pull request or report$`, st.checkBlocked)
	sc.Step(`^the host receives the task row with the escalation answered and the assignee retries$`, st.submitAfterAnswer)
	sc.Step(`^the retried submission pushes the commit and returns pull request 31$`, st.checkSubmitted)
}

type profileTurnWorld struct {
	turnInfoWorld
}

func (w *profileTurnWorld) setupRoster(cleanup func(func())) error {
	if err := w.turnStepWorld.setup(cleanup); err != nil {
		return err
	}
	w.putProfile(turnAIID, "strong", "offline", "", "Go backend")
	w.putProfile(turnPeerID, "light", "limited", "", "docs")
	w.putProfile(turnSecID, "standard", "working", turnTaskID, "")
	w.putUsage(turnAIID, proto.Usage{WindowPct: new(40.0), WindowResets: new(int64(turnTimeMS + 9_000_000))})
	w.putUsage(turnPeerID, proto.Usage{WindowPct: new(100.0)})
	return nil
}

func (w *profileTurnWorld) putUsage(id string, usage proto.Usage) {
	raw, _ := w.h.Rows.Get(turnGroupID, "member", id)
	var ai proto.AIMemberRow
	if err := proto.Decode(raw, &ai); err != nil {
		panic(err)
	}
	ai.Usage = usage
	w.h.Rows.Put(turnGroupID, "member", id, turnMustJSON(ai))
}

func (w *profileTurnWorld) putProfile(id, tier, state, task, skills string) {
	raw, _ := w.h.Rows.Get(turnGroupID, "member", id)
	var ai proto.AIMemberRow
	if err := proto.Decode(raw, &ai); err != nil {
		panic(err)
	}
	ai.Tier, ai.State, ai.Task, ai.Skills = tier, state, task, skills
	w.h.Rows.Put(turnGroupID, "member", id, turnMustJSON(ai))
}

func (w *profileTurnWorld) checkRoster(doc *godog.DocString) error {
	if !strings.HasPrefix(w.text, doc.Content+"\n") {
		return fmt.Errorf("got:\n%s\nwant prefix:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *profileTurnWorld) checkAssignmentRules() error {
	return checkProfileInstructions(w.sessionFor("secretary"),
		"give hard work to `strong` AIs and simple work to `light` AIs",
		"Match the work to their skills",
		"Spread independent tasks over different AIs",
		"members with the same CLI on the same computer share one subscription",
		"Prefer AIs with more of their window left",
		"Do not choose an AI that is `limited` or `offline`")
}

func (w *profileTurnWorld) checkListMembers(ctx context.Context, doc *godog.DocString) error {
	caller := host.Caller{Session: w.sessionFor("secretary")}
	result, err := host.Tools["list_members"].Call(ctx, w.h, caller, json.RawMessage(`{}`))
	if err != nil {
		return err
	}
	got := turnMustJSON(result)
	var want bytes.Buffer
	if err := json.Compact(&want, []byte(doc.Content)); err != nil {
		return err
	}
	if !bytes.Equal(got, want.Bytes()) {
		return fmt.Errorf("list_members = %s; want %s", got, want.Bytes())
	}
	return nil
}

func (w *profileTurnWorld) setupEscalation(cleanup func(func())) error {
	if err := w.setup(cleanup, "P1"); err != nil {
		return err
	}
	w.putProfile(turnSecID, "strong", "offline", "", "")
	w.putProfile(tiPAIID, "light", "waiting", "", "")
	return nil
}

func (w *profileTurnWorld) composeFailure(n int, kind string) error {
	if kind == "hosted check" {
		kind = "checks"
	}
	task := w.taskRow()
	task.Assignee = tiQTeamID
	task.Stuck = &proto.Stuck{Kind: kind, N: n, Ask: n >= 2}
	w.h.Rows.Put(turnGroupID, "task", turnTaskID, turnMustJSON(task))
	if kind == "checks" {
		return w.composeChecks()
	}
	return w.composeReviewed()
}

func (w *profileTurnWorld) checkNoEscalation() error {
	if strings.Contains(w.text, "Before you submit again") {
		return fmt.Errorf("unexpected escalation:\n%s", w.text)
	}
	return nil
}

func (w *profileTurnWorld) checkEscalation(doc *godog.DocString) error {
	if !strings.HasSuffix(w.text, "\n"+doc.Content) {
		return fmt.Errorf("got:\n%s\nwant suffix:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *profileTurnWorld) checkAskRule() error {
	return checkProfileInstructions(w.sessionFor("task"), "When a turn says to ask a stronger AI, use `ask` before calling `submit_task`.")
}

func checkProfileInstructions(s host.Session, want ...string) error {
	for _, text := range want {
		if !strings.Contains(host.Instructions(s), text) {
			return fmt.Errorf("instructions missing %q", text)
		}
	}
	return nil
}

type profileSubmitWorld struct {
	tskScenario
}

func (w *profileSubmitWorld) setupBlocked() error {
	if err := w.givenTaskSession(true); err != nil {
		return err
	}
	if err := w.commitWorktreeFiles(map[string]string{"pass.ok": "ok\n"}, "Checked change"); err != nil {
		return err
	}
	w.putSubmitMember(tskAccWorkerID, "Assignee", "standard", "working", false)
	w.putSubmitMember(tskAccReviewID, "Secretary", "strong", "offline", true)
	task := w.home.currentTask()
	task.Checks.Local = &proto.Check{OK: true, Commit: w.head, At: task.Created}
	task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	w.home.putTask(task)
	return w.queueGHReplies()
}

func (w *profileSubmitWorld) putSubmitMember(id, name, tier, state string, secretary bool) {
	ai := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 3}, Group: tskAccGroupID, Kind: "ai", Name: name},
		CLI:       "claude", Host: "home-dev", HostName: "node", Account: "personal", Secretary: secretary, State: state,
		Allow: []string{}, Approvers: []string{}, Profile: proto.Profile{Tier: tier},
	}
	w.h.Rows.Put(tskAccGroupID, "member", id, turnMustJSON(ai))
}

func (w *profileSubmitWorld) submitEarly() {
	w.callTool("submit_task", map[string]string{"summary": "Ready to submit."})
}

func (w *profileSubmitWorld) checkBlocked() error {
	err, ok := errors.AsType[*host.Error](w.lastErr)
	if !ok || err.Code != "conflict" || err.Message != "Ask a stronger AI before submitting again. Candidates: \"Secretary\" (strong)." {
		return fmt.Errorf("unexpected refusal: %v", w.lastErr)
	}
	if len(w.home.allCmds()) != 0 {
		return errors.New("refused submission sent a home command")
	}
	return w.checkNoRemoteWrites()
}

func (w *profileSubmitWorld) checkNoRemoteWrites() error {
	calls, err := os.ReadFile(filepath.Join(w.ghDir, "calls"))
	if err != nil || len(calls) != 0 {
		return fmt.Errorf("forge calls = %q (%v)", calls, err)
	}
	refs, err := hostForgeGit(w.remoteDir, []string{"for-each-ref", "--format=%(refname)", "refs/heads/task"})
	if err != nil || refs != "" {
		return fmt.Errorf("remote task refs = %q (%v)", refs, err)
	}
	return nil
}

func (w *profileSubmitWorld) submitAfterAnswer() error {
	task := w.home.currentTask()
	task.Stuck.Ask = false
	w.home.putTask(task)
	return w.whenRemoteChecksAndSubmit("Ready to submit after the answer.")
}

func (w *profileSubmitWorld) checkSubmitted() error {
	if err := w.thenRemoteSubmitAndCloudSucceeded(); err != nil {
		return err
	}
	refs, err := hostForgeGit(w.remoteDir, []string{"for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/task"})
	if err != nil || refs != "refs/heads/task/1-fix-path "+w.head {
		return fmt.Errorf("remote task refs = %q (%v)", refs, err)
	}
	return nil
}
