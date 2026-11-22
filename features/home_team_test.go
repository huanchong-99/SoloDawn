//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_team"] = func(sc *godog.ScenarioContext, _ fixtures) {
		w := &homeGroupWorld{now: time.UnixMilli(1763600000000)}
		sc.Step(`^a team home named "([^"]*)"$`, func(name string) error { return initTeamWorld(w, name) })
		sc.Step(`^a project bulletin home named "([^"]*)"$`, func(name string) error { return w.initCreated(name, "owner") })
		sc.Step(`^its owner posts a bulletin from project "([^"]*)" and secretary "([^"]*)"$`, func(project, ai string) error {
			return w.apply(w.browser, proto.Post[proto.BulletinSigned]{
				HomeCommand: teamHeader(w, "post"), Kind: "bulletin", Text: "Ready", Mentions: []proto.ID{},
				Body: proto.BulletinSigned{From: exampleID("team source"), Group: project, AI: ai},
			})
		})
		sc.Step(`^the team stores that signed bulletin as its owner without inbox items$`, func() error {
			if err := w.answerOK(); err != nil {
				return err
			}
			msg := w.last.Rows[0].Value().(proto.MsgRow)
			var body proto.BulletinBody
			if err := json.Unmarshal(msg.Body, &body); err != nil {
				return err
			}
			if len(w.last.Items) != 0 || msg.Kind != "bulletin" || msg.By != w.state.People[msg.By].ID || w.state.People[msg.By].Role != "owner" || body.Group != "Website" || body.AI != "Secretary" || msg.Env == nil {
				return fmt.Errorf("unexpected team bulletin: %+v", msg)
			}
			return nil
		})
		sc.Step(`^its owner posts empty team text$`, func() error {
			return w.apply(w.browser, proto.Post[proto.EmptyBody]{HomeCommand: teamHeader(w, "post"), Kind: "text", Mentions: []proto.ID{}, Body: proto.EmptyBody{}})
		})
		sc.Step(`^the team accepts the post without inbox items$`, func() error {
			if len(w.last.Items) != 0 {
				return fmt.Errorf("queued %d team items", len(w.last.Items))
			}
			return w.answerOK()
		})
		sc.Step(`^its owner sends team command '([^']*)'$`, func(fields string) error {
			return w.applyRaw(w.browser, fmt.Sprintf(`{"g":%q,"n":%q,"ts":%d,%s}`, w.state.ID, w.nextID("team"), w.now.UnixMilli(), fields))
		})
		sc.Step(`^the team command is refused with "([^"]*)"$`, w.answerCode)
	}
}

func initTeamWorld(w *homeGroupWorld, name string) error {
	if err := w.initEmpty(); err != nil {
		return err
	}
	g := proto.ID(w.state.ID)
	return w.apply(w.browser, proto.GroupCreate{
		GroupCommand: proto.GroupCommand{HomeCommand: proto.HomeCommand{Type: "group", Nonce: g, Timestamp: w.now.UnixMilli(), Group: g}, Op: "create"},
		Name:         name, Kind: "team", Forge: "none", Checks: []string{}, MemberName: "owner",
	})
}

func teamHeader(w *homeGroupWorld, typ string) proto.HomeCommand {
	return proto.HomeCommand{Type: typ, Nonce: w.nextID(typ), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}
}
