//go:build acceptance

package features

import (
	"errors"
	"fmt"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

type a10ChoiceWorld struct {
	*choiceWorld
	f       fixtures
	cleanup func(func())
	joined  map[string]proto.Profile
	refused int
}

func a10ChoiceSteps(sc *godog.ScenarioContext, f fixtures, cleanup func(func())) {
	w := &a10ChoiceWorld{f: f, cleanup: cleanup, joined: map[string]proto.Profile{}}
	sc.Step(`^host.join adds the agent "([^"]*)" as "([^"]*)" with model "([^"]*)", effort "([^"]*)" and tier "([^"]*)"$`, w.join)
	sc.Step(`^the member rows of "Opus" and "Haiku" have the model, effort and tier they joined with$`, w.profilesForwarded)
	sc.Step(`^host.info lists both members under the agent "claude"$`, func() error {
		w.members["One"], w.members["Two"] = w.members["Opus"], w.members["Haiku"]
		return w.hostInfo()
	})
	sc.Step(`^host.leave for the agent "claude" without member is "bad_request"$`, w.ambiguousLeave)
	sc.Step(`^host.leave for the agent "claude" with member "Haiku" removes only "Haiku"$`, w.leaveHaiku)
	sc.Step(`^each member's sessions start the fake CLI with its own model and effort as in section 10.4$`, w.launches)
	sc.Step(`^a member with model "" and effort "" passes neither$`, w.empty)
	sc.Step(`^the owner sends member set for "Opus" with model "sonnet"$`, func() error { return w.publish("sonnet", "high", "Opus") })
	sc.Step(`^the next start of a session of "Opus", a resume, passes model "sonnet"$`, w.resume)
	sc.Step(`^the fake claude exits and the fake codex answers thread/start with an error on a refused model$`, w.refusals)
	sc.Step(`^each emits turn_interrupted with "crash", a task session ends in task fail, no session_replaced is sent and the session keeps its id$`, func() error { return hostModelsEqual(w.refused, 2) })
}

func a10NewChoice(f fixtures, cleanup func(func()), first, second string) (*choiceWorld, error) {
	w := &choiceWorld{profiles: map[string]proto.Profile{}, members: map[string]string{}, drivers: map[string]*choiceDriver{}}
	if err := w.setup(f, cleanup, first, second); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *a10ChoiceWorld) join(agent, name, model, effort, tier string) error {
	if w.choiceWorld == nil {
		var err error
		w.choiceWorld, err = a10NewChoice(w.f, w.cleanup, "claude", "codex")
		if err != nil {
			return err
		}
	}
	if err := w.joinProfile(agent, name, model, effort, tier); err != nil {
		return err
	}
	w.joined[name] = w.profiles[name]
	if err := w.run(name, false, false); err != nil {
		return err
	}
	return w.startedWith(model, effort)
}

func (w *a10ChoiceWorld) profilesForwarded() error {
	if err := w.forwarded(); err != nil {
		return err
	}
	for _, name := range []string{"Opus", "Haiku"} {
		raw, _ := w.h.Rows.Get(string(joinGroupIDs["Website"]), "member", w.members[name])
		var ai proto.AIMemberRow
		if err := proto.Decode(raw, &ai); err != nil {
			return err
		}
		if err := hostModelsEqual(ai.Profile, w.joined[name]); err != nil {
			return err
		}
	}
	return nil
}

func (w *a10ChoiceWorld) ambiguousLeave() error {
	var coded *host.Error
	if !errors.As(w.leave("claude", "Website"), &coded) {
		return fmt.Errorf("ambiguous leave did not return a host error")
	}
	return hostModelsEqual(coded.Code, "bad_request")
}

func (w *a10ChoiceWorld) leaveHaiku() error {
	if err := w.leaveMember("Haiku"); err != nil {
		return err
	}
	if err := w.status("dormant"); err != nil {
		return err
	}
	all := w.h.Sessions.All()
	if len(all) != 1 {
		return fmt.Errorf("remaining sessions: %+v", all)
	}
	return hostModelsEqual(all[0].Member, w.members["Opus"])
}

func (w *a10ChoiceWorld) launches() error {
	got := [][2]string{}
	for _, launch := range w.drivers["claude"].snapshot() {
		got = append(got, [2]string{launch.Model, launch.Effort})
	}
	want := [][2]string{{"opus", "high"}, {"haiku", ""}}
	return hostModelsEqual(got, want)
}

func (w *a10ChoiceWorld) empty() error {
	if err := w.joinProfile("codex", "Default", "", "", "standard"); err != nil {
		return err
	}
	if err := w.run("Default", false, false); err != nil {
		return err
	}
	return w.startedWith("", "")
}

func (w *a10ChoiceWorld) resume() error {
	w.key = host.SessionKey{Group: string(joinGroupIDs["Website"]), Member: w.members["Opus"]}
	s, _ := w.current()
	w.cliID = s.CLISession
	if err := w.run("Opus", true, false); err != nil {
		return err
	}
	if err := w.keptID(); err != nil {
		return err
	}
	return w.startedWith("sonnet", "high")
}

func (w *a10ChoiceWorld) refusals() error {
	for _, agent := range []string{"claude", "codex"} {
		x, err := a10NewChoice(w.f, w.cleanup, "claude", "codex")
		if err != nil {
			return err
		}
		if err := x.joinProfile(agent, "Refused", "refused", "high", "strong"); err != nil {
			return err
		}
		if err := x.run("Refused", false, true); err != nil {
			return err
		}
		if err := x.crashed(); err != nil {
			return err
		}
		w.refused++
	}
	return nil
}
