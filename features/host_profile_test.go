//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// profileNoPush is what the steps that MODEL-1126 took away say when they fail.
const profileNoPush = "a submission is a report: the node pushes nothing and opens no pull request; the ask-first rule is rewritten on submit_task by report (MODEL-1126 plan 1.6)"

func init() {
	stepSets["host_profile"] = hostProfileSteps
}

func hostProfileSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	w := &profileTurnWorld{}
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
	sc.Step(`^a remote task with a checked commit and an unanswered escalation$`, rewritten("AC-2", profileNoPush))
	sc.Step(`^the assignee submits the task before the answer$`, rewritten("AC-2", profileNoPush))
	sc.Step(`^the submission returns conflict naming the offline strong secretary with no push, pull request or report$`, rewritten("AC-2", profileNoPush))
	sc.Step(`^the host receives the task row with the escalation answered and the assignee retries$`, rewritten("AC-2", profileNoPush))
	sc.Step(`^the retried submission pushes the commit and returns pull request 31$`, rewritten("AC-2", profileNoPush))
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
		return rewritten("AC-2", "hosted checks are removed: the halves of A10.11 and A10.12 on two failed hosted checks go (MODEL-1126 plan 3.2)")()
	}
	task := w.taskRow()
	task.Assignee = tiQTeamID
	task.Stuck = &proto.Stuck{Kind: kind, N: n, Ask: n >= 2}
	w.h.Rows.Put(turnGroupID, "task", turnTaskID, turnMustJSON(task))
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
