//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["home_profile"] = func(sc *godog.ScenarioContext, _ fixtures) {
		w := &homeProfileWorld{homeReviewWorld: homeReviewWorld{now: time.UnixMilli(1763600000000)}}
		sc.Step(`^an AI has joined the profile home without profile fields$`, w.join)
		sc.Step(`^an AI has joined the profile home following defaults "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, w.joinFollowing)
		sc.Step(`^an AI with its own profile "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)" at the profile home$`, w.joinOwn)
		sc.Step(`^the AI profile is model "([^"]*)", effort "([^"]*)", tier "([^"]*)" and skills "([^"]*)"$`, w.check)
		sc.Step(`^the AI row (follows the defaults|has its own profile)$`, w.checkFollow)
		sc.Step(`^the profile owner sets model "([^"]*)", effort "([^"]*)", tier "([^"]*)" and skills "([^"]*)"$`, w.edit)
		sc.Step(`^the profile owner sets tier "([^"]*)"$`, w.setTier)
		sc.Step(`^the profile owner sets follow$`, w.ownerFollow)
		sc.Step(`^the AI's host sets follow with "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, w.hostSetFollowOK)
		sc.Step(`^the AI's host setting follow with "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)" is refused with "([^"]*)"$`, w.hostSetFollow)
		sc.Step(`^the AI's host setting tier "([^"]*)" without follow is refused with "([^"]*)"$`, w.hostSetTier)
		sc.Step(`^the AI's host setting follow and name "([^"]*)" is refused with "([^"]*)"$`, w.hostSetFollowName)
		sc.Step(`^another person setting follow is refused with "([^"]*)"$`, w.otherPersonFollow)
		sc.Step(`^a member set with follow false is refused with "([^"]*)"$`, w.followFalse)
		sc.Step(`^the AI's host is forbidden to set its tier$`, w.hostEdit)
		sc.Step(`^the profile owner cannot set tier "([^"]*)"$`, w.invalidTier)
		sc.Step(`^a profile task with secretary tier "([^"]*)"$`, w.taskWithSecretary)
		sc.Step(`^the profile assignee has called the secretary in the task thread$`, w.callSecretary)
		sc.Step(`^the profile secretary answers the call$`, w.answerCall)
		sc.Step(`^the profile task (fails|passes) review on "([^"]*)"$`, w.review)
		sc.Step(`^the profile task is stuck on "([^"]*)" at (\d+) with ask (true|false)$`, w.stuck)
		sc.Step(`^the profile task has no stuck counter$`, w.notStuck)
		sc.Step(`^the profile home refuses submission of "([^"]*)" until the assignee asks$`, w.refuseSubmit)
		sc.Step(`^the profile owner gives the task to the reviewer$`, w.reassign)
	}
}

type homeProfileWorld struct {
	homeReviewWorld
	ai   proto.ID
	call proto.ID
}

func (w *homeProfileWorld) command(op string) proto.MemberCommand {
	return proto.MemberCommand{
		HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
		Op:          op,
		ID:          w.ai,
	}
}

func (w *homeProfileWorld) join() error {
	return w.joinProfile(false, proto.ProfileSet{})
}

func (w *homeProfileWorld) joinFollowing(model, effort, tier, skills string) error {
	return w.joinProfile(true, proto.ProfileSet{Model: &model, Effort: &effort, Tier: &tier, Skills: &skills})
}

func (w *homeProfileWorld) joinOwn(model, effort, tier, skills string) error {
	return w.joinProfile(false, proto.ProfileSet{Model: &model, Effort: &effort, Tier: &tier, Skills: &skills})
}

func (w *homeProfileWorld) joinProfile(follow proto.True, profile proto.ProfileSet) error {
	if err := w.setup(); err != nil {
		return err
	}
	w.ai = exampleID("profile new AI")
	p := proto.MemberAddAI{
		MemberCommand: w.command("add_ai"), CLI: "claude", Name: "Opus", Workspace: "/work/Opus",
		Account: "personal", Approvers: []string{}, ProfileSet: profile, Follow: follow,
	}
	if err := w.apply(w.hosts["assignee"], p); err != nil {
		return err
	}
	return w.checkAnswer("")
}

func (w *homeProfileWorld) check(model, effort, tier, skills string) error {
	got := w.state.AIs[string(w.ai)].Profile
	want := proto.Profile{Model: model, Effort: effort, Tier: tier, Skills: skills}
	if got != want {
		return fmt.Errorf("profile is %+v, want %+v", got, want)
	}
	return nil
}

func (w *homeProfileWorld) checkFollow(which string) error {
	want := which == "follows the defaults"
	if got := bool(w.state.AIs[string(w.ai)].Follow); got != want {
		return fmt.Errorf("follow is %v, want %v", got, want)
	}
	return nil
}

func (w *homeProfileWorld) set(signer identity.Key, p proto.ProfileSet, code string) error {
	return w.sendSet(signer, proto.MemberSet{MemberCommand: w.command("set"), ProfileSet: p}, code)
}

func (w *homeProfileWorld) sendSet(signer identity.Key, msg proto.MemberSet, code string) error {
	if err := w.apply(signer, msg); err != nil {
		return err
	}
	return w.checkAnswer(code)
}

func (w *homeProfileWorld) edit(model, effort, tier, skills string) error {
	return w.set(w.ownerKey, proto.ProfileSet{Model: &model, Effort: &effort, Tier: &tier, Skills: &skills}, "")
}

func (w *homeProfileWorld) setTier(tier string) error {
	return w.set(w.ownerKey, proto.ProfileSet{Tier: &tier}, "")
}

func (w *homeProfileWorld) ownerFollow() error {
	return w.sendSet(w.ownerKey, proto.MemberSet{MemberCommand: w.command("set"), Follow: true}, "")
}

func (w *homeProfileWorld) hostSetFollowOK(model, effort, tier, skills string) error {
	return w.hostSetFollow(model, effort, tier, skills, "")
}

func (w *homeProfileWorld) hostSetFollow(model, effort, tier, skills, code string) error {
	msg := proto.MemberSet{
		MemberCommand: w.command("set"),
		ProfileSet:    proto.ProfileSet{Model: &model, Effort: &effort, Tier: &tier, Skills: &skills},
		Follow:        true,
	}
	return w.sendSet(w.hosts["assignee"], msg, code)
}

func (w *homeProfileWorld) hostSetTier(tier, code string) error {
	return w.set(w.hosts["assignee"], proto.ProfileSet{Tier: &tier}, code)
}

func (w *homeProfileWorld) hostSetFollowName(name, code string) error {
	return w.sendSet(w.hosts["assignee"], proto.MemberSet{MemberCommand: w.command("set"), Name: &name, Follow: true}, code)
}

func (w *homeProfileWorld) otherPersonFollow(code string) error {
	other := exampleKey("profile other person")
	log, err := homeLog([]identity.Key{other})
	if err != nil {
		return err
	}
	w.state.Logs[other.ID()] = log
	id := string(exampleID("profile other member"))
	w.state.People[id] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: w.state.ID, Kind: "human", User: other.ID(), Name: "other"},
		Role:      "member",
	}
	return w.sendSet(other, proto.MemberSet{MemberCommand: w.command("set"), Follow: true}, code)
}

func (w *homeProfileWorld) followFalse(code string) error {
	raw := fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"set","id":%q,"follow":false}`,
		w.nextID(), w.now.UnixMilli(), w.state.ID, w.ai)
	if err := w.apply(w.ownerKey, json.RawMessage(raw)); err != nil {
		return err
	}
	return w.checkAnswer(code)
}

func (w *homeProfileWorld) hostEdit() error {
	return w.set(w.hosts["assignee"], proto.ProfileSet{Tier: new("light")}, "forbidden")
}

func (w *homeProfileWorld) invalidTier(tier string) error {
	return w.set(w.ownerKey, proto.ProfileSet{Tier: &tier}, "bad_request")
}

func (w *homeProfileWorld) taskWithSecretary(tier string) error {
	if err := w.setup(); err != nil {
		return err
	}
	w.state.Group.Limits.AskDepth = 4
	w.ai = proto.ID(w.members["secretary"])
	return w.set(w.ownerKey, proto.ProfileSet{Tier: &tier}, "")
}

func (w *homeProfileWorld) postCommand(as string) proto.HomeCommand {
	return proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
		Group: proto.ID(w.state.ID), As: proto.ID(w.members[as])}
}

func (w *homeProfileWorld) callSecretary() error {
	cmd := w.postCommand("assignee")
	w.call = cmd.Nonce
	post := proto.Post[proto.CallBody]{HomeCommand: cmd, Kind: "call", Thread: new(proto.ID(w.taskID)),
		Text: "What should I change?", Mentions: []proto.ID{},
		Body: proto.CallBody{To: w.members["secretary"], Depth: 1, Wait: true}}
	if err := w.apply(w.hosts["assignee"], post); err != nil {
		return err
	}
	return w.checkAnswer("")
}

func (w *homeProfileWorld) answerCall() error {
	post := proto.Post[proto.EmptyBody]{HomeCommand: w.postCommand("secretary"), Kind: "reply", Thread: new(proto.ID(w.taskID)),
		Text: "Check the response shape.", Mentions: []proto.ID{}, Re: &w.call}
	if err := w.apply(w.hosts["secretary"], post); err != nil {
		return err
	}
	return w.checkAnswer("")
}

func (w *homeProfileWorld) review(verdict, commit string) error {
	if err := w.reachReview(commit); err != nil {
		return err
	}
	round := 1
	if last := w.state.Tasks[w.taskID].Review; last != nil {
		round += last.Round
	}
	score := 85
	if verdict == "passes" {
		score = 95
	}
	if err := w.postReview("reviewer", commit, score, false); err != nil {
		return err
	}
	return w.checkReviewAnswer(fmt.Sprint(verdict == "passes"), round)
}

func (w *homeProfileWorld) stuck(kind string, n int, ask string) error {
	got := w.state.Tasks[w.taskID].Stuck
	want := proto.Stuck{Kind: kind, N: n, Ask: ask == "true"}
	if got == nil || *got != want {
		return fmt.Errorf("stuck is %+v, want %+v", got, want)
	}
	return nil
}

func (w *homeProfileWorld) notStuck() error {
	if got := w.state.Tasks[w.taskID].Stuck; got != nil {
		return fmt.Errorf("stuck is %+v, want none", got)
	}
	return nil
}

func (w *homeProfileWorld) refuseSubmit(commit string) error {
	if err := w.reachReview(commit); err != nil {
		return err
	}
	if err := w.checkAnswer("conflict"); err != nil {
		return err
	}
	if w.lastRes.Msg != "ask a stronger AI before submitting again" || len(w.lastRes.Rows) != 0 {
		return fmt.Errorf("submission wrote %d rows and said %q", len(w.lastRes.Rows), w.lastRes.Msg)
	}
	return nil
}

func (w *homeProfileWorld) reassign() error {
	p := proto.TaskAssign{TaskCommand: proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
		Op:          "assign", ID: proto.ID(w.taskID),
	}, Assignee: proto.ID(w.members["reviewer"]), Reviewer: proto.ID(w.members["assignee"])}
	if err := w.apply(w.ownerKey, p); err != nil {
		return err
	}
	return w.checkAnswer("")
}
