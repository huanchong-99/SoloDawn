//go:build acceptance

package features

import (
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_external"] = func(sc *godog.ScenarioContext, _ fixtures) {
		w := &homeExternalWorld{homeReviewWorld: homeReviewWorld{now: time.UnixMilli(1763600000000)}}
		sc.Step(`^an external AI joins its home with tool "([^"]*)"$`, w.join)
		sc.Step(`^the external member row has tool "([^"]*)" and the default profile$`, w.checkRow)
		sc.Step(`^another member tries to set the external tool to "([^"]*)"$`, w.otherEdit)
		sc.Step(`^the owner sets the external tool to "([^"]*)"$`, w.ownerEdit)
		sc.Step(`^the home refuses the external member change as "([^"]*)"$`, w.checkAnswer)
	}
}

type homeExternalWorld struct {
	homeReviewWorld
	other identity.Key
}

func (w *homeExternalWorld) command(op string) proto.MemberCommand {
	return proto.MemberCommand{
		HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
		Op:          op, ID: exampleID("external member"),
	}
}

func (w *homeExternalWorld) join(tool string) error {
	if err := w.setup(); err != nil {
		return err
	}
	w.other = exampleKey("external group member")
	log, err := homeLog([]identity.Key{w.other})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	id := string(exampleID("other human member"))
	w.state.People[id] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: w.state.ID, Kind: "human", User: log.User},
		Role:      "member",
	}
	cmd := proto.MemberAddAI{MemberCommand: w.command("add_ai"), CLI: "external", Name: "Scout", Workspace: "/work/Scout", Account: "api_key", Tool: &tool, Approvers: []string{}}
	return w.apply(w.hosts["assignee"], cmd)
}

func (w *homeExternalWorld) checkRow(tool string) error {
	ai := w.state.AIs[string(exampleID("external member"))]
	if ai.CLI != "external" || ai.Account != "api_key" || ai.Secretary || ai.Tool != tool || ai.Profile != (proto.Profile{Tier: "standard"}) {
		return fmt.Errorf("external member has cli %q, account %q, secretary %t, tool %q and profile %+v; want external, api_key, false, %q and defaults",
			ai.CLI, ai.Account, ai.Secretary, ai.Tool, ai.Profile, tool)
	}
	return nil
}

func (w *homeExternalWorld) otherEdit(tool string) error {
	return w.apply(w.other, proto.MemberSet{MemberCommand: w.command("set"), Tool: &tool})
}

func (w *homeExternalWorld) ownerEdit(tool string) error {
	if err := w.apply(w.ownerKey, proto.MemberSet{MemberCommand: w.command("set"), Tool: &tool}); err != nil {
		return err
	}
	return w.checkAnswer("")
}
