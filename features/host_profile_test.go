//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	sc.Step(`^the host has the secretary's synced AI profiles$`, func() error { return w.setupRoster(cleanup) })
	sc.Step(`^the secretary's group has a claude member that follows defaults with tier "([^"]*)" and skills "([^"]*)"$`, func(tier, skills string) error {
		return w.setupFollowingMember(cleanup, tier, skills)
	})
	sc.Step(`^the secretary takes its next turn$`, w.composeConfirmedBrief)
	sc.Step(`^the turn begins with:$`, w.checkRoster)
	sc.Step(`^its roster line for that member shows "([^"]*)" and "([^"]*)"$`, w.checkFollowerRosterLine)
	sc.Step(`^no turn text and no list_members answer contains "([^"]*)"$`, w.checkTurnAndListExclude)
	sc.Step(`^the secretary instructions contain the assignment rules$`, w.checkAssignmentRules)
	sc.Step(`^list_members still returns:$`, w.checkListMembers)
	sc.Step(`^the host has a standard assignee and an offline strong secretary$`, func() error { return w.setupEscalation(cleanup) })
	sc.Step(`^the host composes a turn after the (\d+)(?:st|nd|rd) failed (review)$`, w.composeFailure)
	sc.Step(`^the turn has no escalation line$`, w.checkNoEscalation)
	sc.Step(`^the turn ends with the escalation line:$`, w.checkEscalation)
	sc.Step(`^the task instructions say to ask before submitting$`, w.checkAskRule)
	sub := &profileSubmitWorld{tskScenario{t: f.t, cleanup: cleanup}}
	sc.Step(`^a task session whose task asks its assignee to ask a stronger AI first$`, sub.setupBlocked)
	sc.Step(`^the assignee submits the task before the answer$`, func() { sub.whenSubmit("Ready to submit.") })
	sc.Step(`^the submission returns conflict naming the offline strong secretary and no report was posted$`, sub.checkBlocked)
	sc.Step(`^the host receives the task row with the escalation answered and the assignee retries$`, sub.submitAfterAnswer)
	sc.Step(`^the retried submission posts its summary as the report in the task's thread$`, sub.thenReported)
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

func (w *profileTurnWorld) setupFollowingMember(cleanup func(func()), tier, skills string) error {
	if err := w.turnStepWorld.setup(cleanup); err != nil {
		return err
	}
	w.editMember(turnAIID, func(ai *proto.AIMemberRow) {
		ai.Follow = true
		ai.Tier, ai.Skills = tier, skills
	})
	return nil
}

func (w *profileTurnWorld) editMember(id string, edit func(*proto.AIMemberRow)) {
	raw, _ := w.h.Rows.Get(turnGroupID, "member", id)
	var ai proto.AIMemberRow
	if err := proto.Decode(raw, &ai); err != nil {
		panic(err)
	}
	edit(&ai)
	w.h.Rows.Put(turnGroupID, "member", id, turnMustJSON(ai))
}

func (w *profileTurnWorld) putUsage(id string, usage proto.Usage) {
	w.editMember(id, func(ai *proto.AIMemberRow) {
		ai.Usage = usage
	})
}

func (w *profileTurnWorld) putProfile(id, tier, state, task, skills string) {
	w.editMember(id, func(ai *proto.AIMemberRow) {
		ai.Tier, ai.State, ai.Task, ai.Skills = tier, state, task, skills
	})
}

func (w *profileTurnWorld) checkFollowerRosterLine(tier, skills string) error {
	for _, line := range strings.Split(w.text, "\n") {
		if strings.HasPrefix(line, `- "Claude Code" `) {
			if !strings.Contains(line, tier) || !strings.Contains(line, skills) {
				return fmt.Errorf("member roster line = %q, want %q and %q", line, tier, skills)
			}
			return nil
		}
	}
	return fmt.Errorf("member roster line missing from turn:\n%s", w.text)
}

func (w *profileTurnWorld) checkTurnAndListExclude(ctx context.Context, word string) error {
	if strings.Contains(w.text, word) {
		return fmt.Errorf("turn text contains %q:\n%s", word, w.text)
	}
	caller := host.Caller{Session: w.sessionFor("secretary")}
	result, err := host.Tools["list_members"].Call(ctx, w.h, caller, json.RawMessage(`{}`))
	if err != nil {
		return err
	}
	got := turnMustJSON(result)
	if strings.Contains(string(got), word) {
		return fmt.Errorf("list_members contains %q: %s", word, got)
	}
	return nil
}

func (w *profileTurnWorld) checkRoster(doc *godog.DocString) error {
	if !strings.HasPrefix(w.text, doc.Content+"\n") {
		return fmt.Errorf("got:\n%s\nwant prefix:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *profileTurnWorld) checkAssignmentRules() error {
	return checkProfileInstructions(w.sessionFor("secretary"),
		"Choose assignee and reviewer task by task",
		"Never an AI that is `limited` or `offline`.",
		"The window first.",
		"Members with the same CLI on the same computer share one subscription (not two external AIs).",
		"The tier is the share of tasks its owner wants it given: `strong` more, `standard` a normal share, `light` fewer.",
		"Skills say which tasks: the hard and the special ones go to the AIs whose skills name them; when skills say nothing, to the higher tier.",
		"Spread independent tasks over different AIs, so several subscriptions work at once.",
		"Each AI works in its own folder on its own computer; the member list in your turn names both.",
		"choose a reviewer on the assignee's computer when the project has no shared remote")
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

type profileSubmitWorld struct {
	tskScenario
}

// setupBlocked is a task session whose task failed two reviews in a row while
// the group has a stronger AI, the offline secretary: the home asks first.
func (w *profileSubmitWorld) setupBlocked() error {
	if err := w.givenTaskSession(); err != nil {
		return err
	}
	w.putSubmitMember(tskAccWorkerID, "Assignee", "standard", "working", false)
	w.putSubmitMember(tskAccReviewID, "Secretary", "strong", "offline", true)
	task := w.home.currentTask()
	task.Stuck = &proto.Stuck{Kind: "review", N: 2, Ask: true}
	w.home.putTask(task)
	return nil
}

func (w *profileSubmitWorld) putSubmitMember(id, name, tier, state string, secretary bool) {
	ai := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 3}, Group: tskAccGroupID, Kind: "ai", Name: name},
		CLI:       "claude", Host: "home-dev", HostName: "node", Workspace: w.workspace, Account: "personal", Secretary: secretary, State: state,
		Allow: []string{}, Approvers: []string{}, Profile: proto.Profile{Tier: tier},
	}
	w.h.Rows.Put(tskAccGroupID, "member", id, turnMustJSON(ai))
}

func (w *profileSubmitWorld) checkBlocked() error {
	err, ok := errors.AsType[*host.Error](w.lastErr)
	if !ok || err.Code != "conflict" || err.Message != "Ask a stronger AI before submitting again. Candidates: \"Secretary\" (strong)." {
		return fmt.Errorf("unexpected refusal: %v", w.lastErr)
	}
	if cmds := w.home.allCmds(); len(cmds) != 0 {
		return fmt.Errorf("the refused submission sent %d commands to the home", len(cmds))
	}
	return nil
}

func (w *profileSubmitWorld) submitAfterAnswer() {
	task := w.home.currentTask()
	task.Stuck.Ask = false
	w.home.putTask(task)
	w.whenSubmit("Ready to submit after the answer.")
}
