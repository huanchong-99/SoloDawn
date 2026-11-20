//go:build acceptance

package features

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_machine"] = homeMachineSteps
}

// homeMachine is one task row, moved through a scenario by the task machine.
type homeMachine struct {
	task    proto.TaskRow
	pending []home.PendingCard
	fx      home.Effects
	err     error
}

var homeMachineSenders = map[string]home.Sender{
	"host": home.ByHost, "secretary": home.BySecretary, "admin": home.ByAdmin, "person": home.ByPerson,
	"assignee": home.ByAI, "reviewer": home.ByAI,
}

// The member an AI's command is signed for.
var homeMachineMembers = map[string]string{"assignee": "ai-assignee", "reviewer": "ai-reviewer"}

func (m *homeMachine) send(sender, kind string, e home.Event) {
	e.Kind, e.By, e.Member = home.EventKind(kind), homeMachineSenders[sender], homeMachineMembers[sender]
	e.Now, e.Pending = 1763600000000, m.pending
	e.Limits = proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5}
	m.task, m.fx, m.err = home.Step(m.task, e)
}

// homeMachineItems writes inbox items as the scenarios do: why, then the AI or the notice code, then the commit.
func homeMachineItems(items []home.TaskItem) string {
	if len(items) == 0 {
		return "nothing"
	}
	var out []string
	for _, it := range items {
		words := []string{it.Why, it.AI + it.Code}
		if it.Commit != "" {
			words = append(words, it.Commit)
		}
		if it.IfNone {
			words = append(words, "unless queued")
		}
		out = append(out, strings.Join(words, " "))
	}
	return strings.Join(out, ", ")
}

func homeMachineSteps(sc *godog.ScenarioContext, _ fixtures) {
	var m homeMachine
	sc.Step(`^a task row that is "([^"]*)" at step "([^"]*)"$`, func(state, step string) {
		m = homeMachine{task: proto.TaskRow{
			Origin: "brief", State: state, Step: step, Commit: "c1", Assignee: "ai-assignee", Reviewer: "ai-reviewer",
			Checks: proto.TaskChecks{Local: &proto.Check{OK: true, At: 1763599997000, Commit: "c1"}},
		}}
	})
	sc.Step(`^the task row has no reviewer and waits for one$`, func() {
		m.task.Reviewer, m.task.Wait = "", &proto.TaskWait{Kind: "reviewer", Since: 1763599998000}
	})
	sc.Step(`^the task row waits for the approval card "([^"]*)"$`, func(card string) {
		m.pending = []home.PendingCard{{Kind: "approval", Msg: card, Who: []string{"owner"}, Since: 1763599999000}}
		m.task.Wait = &proto.TaskWait{Kind: "approval", Msg: card, Who: []string{"owner"}, Since: 1763599999000}
	})
	sc.Step(`^the (host|secretary|admin|person|assignee|reviewer) sends "([^"]*)" to the task machine$`, func(sender, kind string) {
		m.send(sender, kind, home.Event{Commit: "c1", OK: true})
	})
	sc.Step(`^the host sends "([^"]*)" for commit "([^"]*)" to the task machine$`, func(kind, commit string) {
		m.send("host", kind, home.Event{Commit: commit, OK: true})
	})
	sc.Step(`^the secretary names "([^"]*)" as the task's (assignee|reviewer)$`, func(ai, role string) {
		e := home.Event{Assignee: ai}
		if role == "reviewer" {
			e = home.Event{Reviewer: ai}
		}
		m.send("secretary", "assign", e)
	})
	sc.Step(`^a person decides the approval card "([^"]*)"$`, func(card string) {
		m.send("person", "approve", home.Event{Card: home.PendingCard{Msg: card}, Asker: "ai-assignee"})
		m.pending = slices.DeleteFunc(m.pending, func(c home.PendingCard) bool { return c.Msg == card })
	})
	sc.Step(`^the task machine answers "([^"]*)"$`, func(want string) error {
		got := "ok"
		if m.err != nil {
			got = m.err.Error()
		}
		if got != want {
			return fmt.Errorf("the task machine answered %q, want %q", got, want)
		}
		return nil
	})
	sc.Step(`^the task row is "([^"]*)" at step "([^"]*)" and waits for "([^"]*)"$`, func(state, step, wait string) error {
		kind := "nothing"
		if m.task.Wait != nil {
			kind = m.task.Wait.Kind
		}
		if m.err != nil || m.task.State != state || m.task.Step != step || kind != wait {
			return fmt.Errorf("the task row is %q at step %q and waits for %q, the answer was %v", m.task.State, m.task.Step, kind, m.err)
		}
		return nil
	})
	sc.Step(`^the task machine queues "([^"]*)"$`, func(want string) error {
		if got := homeMachineItems(m.fx.Items); got != want {
			return fmt.Errorf("the task machine queued %q, want %q", got, want)
		}
		return nil
	})
	sc.Step(`^every queued item of the task is retired and its pending cards expire$`, func() error {
		if !reflect.DeepEqual(m.fx.Retire, []home.ItemFilter{{}}) || m.fx.Cards != "expired" {
			return fmt.Errorf("the task machine retires %+v and makes the pending cards %q", m.fx.Retire, m.fx.Cards)
		}
		return nil
	})
}
