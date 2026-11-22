//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_confirm"] = func(sc *godog.ScenarioContext, _ fixtures) { homeConfirmSteps(sc) }
}

func TestHomeConfirmFeature(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: homeConfirmSteps,
		Options:             &godog.Options{Format: "pretty", Paths: []string{"home_confirm.feature"}, Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("home confirmation scenarios failed")
	}
}

type homeConfirmWorld struct {
	homeCardWorld
	card, call string
}

func (w *homeConfirmWorld) send(by identity.Key, p any) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	w.queued = nil
	if err := w.apply(by.Sign(string(raw))); err != nil {
		return err
	}
	for _, row := range w.answers[len(w.answers)-1].Rows {
		if m, ok := row.Value().(proto.MsgRow); ok && m.Kind == "confirm" {
			w.card = m.ID
		}
	}
	return nil
}

func (w *homeConfirmWorld) command(kind, as string) proto.HomeCommand {
	return proto.HomeCommand{Type: kind, Nonce: w.freshNonce(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID), As: proto.ID(as)}
}

func homeConfirmSteps(sc *godog.ScenarioContext) {
	w := &homeConfirmWorld{}
	other := func() string { return w.members["ai-three"] }
	sc.Step(`^a confirmation home with two owners$`, func() error {
		w.now = time.UnixMilli(1763600000000)
		if err := w.setup("ai-one"); err != nil {
			return err
		}
		guest, second := w.devices["guest"], exampleKey("confirm other second device")
		log, err := homeLog([]identity.Key{guest, second})
		if err != nil {
			return err
		}
		w.state.Logs[guest.ID()] = log
		w.devices["other-second"] = second
		a := w.state.AIs[other()]
		a.User, a.Host, a.Account = guest.ID(), guest.ID(), "personal"
		a.Approvers = []string{guest.ID()}
		w.state.AIs[other()] = a
		w.state.Group.Limits.AskDepth = 4
		return nil
	})
	mention := func(by identity.Key) error {
		return w.send(by, proto.Post[proto.EmptyBody]{HomeCommand: w.command("post", ""), Kind: "text", Text: "Check the port", Mentions: []proto.ID{proto.ID(other())}, Body: proto.EmptyBody{}})
	}
	sc.Step(`^the first owner mentions the other AI$`, func() error { return mention(w.devices["first"]) })
	sc.Step(`^the first owner asks to watch the other AI$`, func() error {
		return w.send(w.devices["first"], proto.Post[proto.ConfirmSigned]{HomeCommand: w.command("post", ""), Kind: "confirm", Mentions: []proto.ID{}, Body: proto.ConfirmSigned{What: "watch", AI: proto.ID(other())}})
	})
	sc.Step(`^the first owner asks to watch with thread, reply and text fields$`, func() error {
		context := exampleID("watch context")
		return w.send(w.devices["first"], proto.Post[proto.ConfirmSigned]{HomeCommand: w.command("post", ""), Kind: "confirm", Thread: &context, Re: &context, Text: "Watch context", Mentions: []proto.ID{}, Body: proto.ConfirmSigned{What: "watch", AI: proto.ID(other())}})
	})
	sc.Step(`^the second owner mentions their own AI$`, func() error { return mention(w.devices["guest"]) })
	sc.Step(`^a pending "([^"]*)" card exists with no delivery$`, func(what string) error {
		if err := w.checkAnswer(""); err != nil {
			return err
		}
		var body proto.ConfirmBody
		card := w.disk[w.card]
		json.Unmarshal(card.Body, &body)
		want := proto.ConfirmBody{What: what, AI: other(), To: []string{w.devices["guest"].ID()}}
		if !reflect.DeepEqual(body, want) || card.State.Status != "pending" || len(w.queued) != 0 {
			return fmt.Errorf("card %+v, body %+v, delivery %+v", card, body, w.queued)
		}
		return nil
	})
	act := func(by identity.Key, action string) error {
		return w.send(by, proto.Act{HomeCommand: w.command("act", ""), Msg: proto.ID(w.card), Action: action})
	}
	sc.Step(`^the first owner tries to confirm it$`, func() error { return act(w.devices["first"], "confirm") })
	sc.Step(`^the second owner confirms it$`, func() error { return act(w.devices["guest"], "confirm") })
	sc.Step(`^the second owner denies it$`, func() error { return act(w.devices["guest"], "deny") })
	sc.Step(`^the second owner's second device confirms it twice with one nonce$`, func() error {
		p := proto.Act{HomeCommand: w.command("act", ""), Msg: proto.ID(w.card), Action: "confirm"}
		by := w.devices["other-second"]
		if err := w.send(by, p); err != nil {
			return err
		}
		queued := w.queued
		if err := w.send(by, p); err != nil {
			return err
		}
		if len(w.queued) != 0 {
			return fmt.Errorf("replay delivered %+v", w.queued)
		}
		w.queued = queued
		return nil
	})
	sc.Step(`^the home confirmation command answers "([^"]*)"$`, w.checkAnswer)
	sc.Step(`^the confirmation card is "([^"]*)"$`, func(status string) error { return w.checkCardStatus(w.card, status) })
	sc.Step(`^the latest delivery is "([^"]*)"$`, func(why string) error {
		var got []string
		for _, it := range w.queued {
			got = append(got, it.Why)
		}
		if strings.Join(got, ",") != why {
			return fmt.Errorf("delivery %+v, want %q", w.queued, why)
		}
		return nil
	})
	set := func(account string, allow []string, approvers []string) error {
		p := proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: w.command("member", ""), Op: "set", ID: proto.ID(other())}}
		if account != "" {
			p.Account = &account
		}
		if allow != nil {
			p.Allow = &allow
		}
		if approvers != nil {
			p.Approvers = &approvers
		}
		return w.send(w.devices["guest"], p)
	}
	sc.Step(`^the second owner configures the other AI as "([^"]*)" allowing the first owner$`, func(account string) error {
		return set(account, []string{w.devices["first"].ID()}, nil)
	})
	sc.Step(`^the second owner removes the first owner from the allow list$`, func() error { return set("", []string{}, nil) })
	sc.Step(`^the first owner's AI calls the other AI$`, func() error {
		p := proto.Post[proto.CallBody]{HomeCommand: w.command("post", w.members["ai-one"]), Kind: "call", Text: "Which port?", Mentions: []proto.ID{}, Body: proto.CallBody{To: other(), Depth: 1, Wait: true}}
		w.call = string(p.Nonce)
		return w.send(w.devices["host"], p)
	})
	sc.Step(`^the call is denied with a reply and secretary notice$`, func() error {
		if err := w.checkCardStatus(w.call, "denied"); err != nil {
			return err
		}
		want := []home.Item{{AI: w.members["ai-one"], Why: "reply", Msg: w.card}, {AI: w.members["secretary"], Why: "notice", Msg: w.card, Code: "confirm_denied"}}
		if !reflect.DeepEqual(w.queued, want) {
			return fmt.Errorf("delivery %+v, want %+v", w.queued, want)
		}
		return nil
	})
	sc.Step(`^the second owner adds the first owner as an approval delegate$`, func() error {
		return set("", nil, []string{w.devices["first"].ID()})
	})
	sc.Step(`^the second owner removes the first owner as an approval delegate$`, func() error { return set("", nil, []string{}) })
	sc.Step(`^the other AI posts an approval card for both owners$`, func() error {
		p := proto.Post[proto.ApprovalBody]{HomeCommand: w.command("post", other()), Kind: "approval", Main: true, Mentions: []proto.ID{},
			Body: proto.ApprovalBody{AI: other(), Tool: "Bash", Command: "ls", Category: "command", Approvers: []string{w.devices["first"].ID(), w.devices["guest"].ID()}},
		}
		w.approval = string(p.Nonce)
		return w.send(w.devices["guest"], p)
	})
	sc.Step(`^the first owner approves that approval$`, func() error {
		return w.send(w.devices["first"], proto.Act{HomeCommand: w.command("act", ""), Msg: proto.ID(w.approval), Action: "approve"})
	})
	sc.Step(`^retention removes the request$`, func() error {
		delete(w.disk, *w.disk[w.card].Re)
		res := home.Timers["confirm"](w.state, w.disk, w.now.UnixMilli())
		for _, r := range res.Rows {
			m := r.Value().(proto.MsgRow)
			w.disk[m.ID] = m
		}
		w.state.Commit(res)
		return nil
	})
}
