//go:build acceptance

package features

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_confirm_task"] = func(sc *godog.ScenarioContext, _ fixtures) { homeRoleSteps(sc) }
}

func TestHomeRoleFeature(t *testing.T) {
	suite := godog.TestSuite{ScenarioInitializer: homeRoleSteps,
		Options: &godog.Options{Format: "pretty", Paths: []string{"home_confirm_task.feature"}, Strict: true, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("role-confirmation scenarios failed")
	}
}

func homeRoleSteps(sc *godog.ScenarioContext) {
	w := &homeConfirmWorld{}
	var active, dependent, roleCard string
	other := func() string { return w.members["ai-three"] }
	sc.Step(`^a role-confirmation home$`, func() error {
		w.now = time.UnixMilli(1763600000000)
		if err := w.setup("ai-one"); err != nil {
			return err
		}
		a := w.state.AIs[other()]
		a.User, a.Host, a.Account = w.devices["guest"].ID(), w.devices["guest"].ID(), "personal"
		w.state.AIs[other()] = a
		p := w.state.People[w.members["guest"]]
		p.Role = "admin"
		w.state.People[p.ID] = p
		w.disk[w.taskID] = proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: w.taskID}, Kind: "task"}
		return nil
	})
	brief := func(reviewer bool) error {
		a, r := other(), w.members["ai-two"]
		if reviewer {
			a, r = w.members["ai-one"], other()
		}
		body := proto.BriefBody{Title: "Check ports", Goal: "Find the port", Criteria: []string{"Port is documented"},
			Scope: proto.BriefScope{In: []string{"socket"}, Out: []string{}}, Pass: 90, Requester: w.members["owner"],
			Tasks: []proto.BriefTask{{Title: "Read ports", Assignee: a, Reviewer: r, Depends: []int{}}, {Title: "Write ports", Assignee: a, Reviewer: r, Depends: []int{0}}},
		}
		p := proto.Post[proto.BriefBody]{HomeCommand: w.command("post", w.members["secretary"]), Kind: "brief", Mentions: []proto.ID{}, Body: body}
		if err := w.send(w.devices["host"], p); err != nil {
			return err
		}
		if err := w.checkAnswer(""); err != nil {
			return err
		}
		act := proto.Act{HomeCommand: w.command("act", ""), Msg: p.Nonce, Action: "confirm"}
		if err := w.send(w.devices["first"], act); err != nil {
			return err
		}
		if err := w.checkAnswer(""); err != nil {
			return err
		}
		for _, row := range w.answers[len(w.answers)-1].Rows {
			if task, ok := row.Value().(proto.TaskRow); ok {
				if task.State == "active" {
					active = task.ID
				} else {
					dependent = task.ID
				}
			}
		}
		roleCard = w.card
		return nil
	}
	sc.Step(`^the first owner confirms a brief assigning two tasks to the other AI$`, func() error { return brief(false) })
	sc.Step(`^the first owner confirms another brief for the other AI$`, func() error { return brief(false) })
	sc.Step(`^the first owner confirms a brief naming the other AI as reviewer$`, func() error { return brief(true) })
	sc.Step(`^the active role waits for confirmation and the dependent role waits for dependencies$`, func() error {
		if w.state.Tasks[dependent].Wait.Kind != "depends" {
			return fmt.Errorf("dependent task %+v", w.state.Tasks[dependent])
		}
		wait := w.state.Tasks[active].Wait
		if wait == nil || wait.Kind != "confirm" || wait.Msg != roleCard {
			return fmt.Errorf("active wait %+v", wait)
		}
		return nil
	})
	sc.Step(`^the active role waits for confirmation$`, func() error {
		wait := w.state.Tasks[active].Wait
		if wait == nil || wait.Kind != "confirm" || wait.Msg != roleCard {
			return fmt.Errorf("active wait %+v", wait)
		}
		return nil
	})
	sc.Step(`^the active role has no confirmation wait$`, func() error {
		if wait := w.state.Tasks[active].Wait; wait != nil && wait.Kind == "confirm" {
			return fmt.Errorf("held by %+v", wait)
		}
		return nil
	})
	sc.Step(`^the held role does not exempt a thread mention$`, func() error {
		taskID := proto.ID(active)
		if err := w.send(w.devices["first"], proto.Post[proto.EmptyBody]{HomeCommand: w.command("post", ""), Kind: "text", Thread: &taskID, Mentions: []proto.ID{proto.ID(other())}, Body: proto.EmptyBody{}}); err != nil {
			return err
		}
		if err := w.checkAnswer(""); err != nil {
			return err
		}
		if w.card == roleCard || len(w.queued) != 0 {
			return fmt.Errorf("mention card %q, items %+v", w.card, w.queued)
		}
		return nil
	})
	for _, action := range []string{"confirm", "deny"} {
		verb := "confirms"
		if action == "deny" {
			verb = "denies"
		}
		sc.Step("^the other owner "+verb+" the role card$", func() error {
			if err := w.send(w.devices["guest"], proto.Act{HomeCommand: w.command("act", ""), Msg: proto.ID(roleCard), Action: action}); err != nil {
				return err
			}
			return w.checkAnswer("")
		})
	}
	sc.Step(`^the first owner cannot confirm the role card$`, func() error {
		if err := w.send(w.devices["first"], proto.Act{HomeCommand: w.command("act", ""), Msg: proto.ID(roleCard), Action: "confirm"}); err != nil {
			return err
		}
		return w.checkAnswer("forbidden")
	})
	sc.Step(`^the role delivery is "([^"]*)"$`, func(want string) error {
		var kinds []string
		for _, it := range w.queued {
			kinds = append(kinds, it.Why)
		}
		if strings.Join(kinds, ",") != want {
			return fmt.Errorf("delivery %+v, want %q", w.queued, want)
		}
		return nil
	})
	taskCmd := func(op string) proto.TaskCommand {
		return proto.TaskCommand{HomeCommand: w.command("task", ""), Op: op, ID: proto.ID(active)}
	}
	sc.Step(`^the first owner pauses and resumes the role task$`, func() error {
		for _, op := range []string{"pause", "resume"} {
			if err := w.send(w.devices["first"], taskCmd(op)); err != nil {
				return err
			}
			if err := w.checkAnswer(""); err != nil {
				return err
			}
		}
		return nil
	})
	assign := func(by identity.Key) error {
		active = w.taskID
		if err := w.send(by, proto.TaskAssign{TaskCommand: taskCmd("assign"), Assignee: proto.ID(other())}); err != nil {
			return err
		}
		roleCard = w.card
		return w.checkAnswer("")
	}
	sc.Step(`^the first owner assigns the existing task to the other AI$`, func() error { return assign(w.devices["first"]) })
	sc.Step(`^the other owner assigns the existing task to their AI$`, func() error { return assign(w.devices["guest"]) })
	sc.Step(`^no role-confirmation card was created$`, func() error {
		if w.card != "" {
			return fmt.Errorf("unexpected card %q", w.card)
		}
		return nil
	})
	sc.Step(`^the assignee submits passing local checks$`, func() error {
		task := w.state.Tasks[active]
		p := taskCmd("branch")
		if err := w.send(w.devices["host"], proto.TaskBranch{TaskCommand: p, Branch: fmt.Sprintf("task/%d-%s", task.Num, task.Slug)}); err != nil {
			return err
		}
		if err := w.checkAnswer(""); err != nil {
			return err
		}
		if err := w.send(w.devices["host"], proto.TaskChecked{TaskCommand: taskCmd("checked"), Commit: "commit", OK: true, Items: []proto.CheckItem{}}); err != nil {
			return err
		}
		if err := w.checkAnswer(""); err != nil {
			return err
		}
		if err := w.send(w.devices["host"], proto.TaskSubmitted{TaskCommand: taskCmd("submitted"), Commit: "commit"}); err != nil {
			return err
		}
		return w.checkAnswer("")
	})
}
