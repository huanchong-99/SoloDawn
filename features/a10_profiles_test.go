//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["a10_profiles"] = a10ProfilesSteps }

type a10ProfileWorld struct {
	homeProfileWorld
	f        fixtures
	cleanup  func(func())
	turn     profileTurnWorld
	commit   int
	assignee string
	models   *a10ModelsWorld
}

func a10ProfilesSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &a10ProfileWorld{f: f, cleanup: afterScenario(sc)}
	w.now = time.UnixMilli(1763600000000)
	sc.Step(`^the profiles cast$`, func() error { return w.cast("M", "cast") })
	sc.Step(`^the profiles cast with (M|S) the assignee of task T$`, func(as string) error { return w.cast(as, "cast") })
	sc.Step(`^the profiles cast with every AI at the default tier and M the assignee of task T$`, func() error { return w.cast("M", "default") })
	sc.Step(`^the profiles cast with C strong, S standard and S the assignee of task T$`, func() error { return w.cast("S", "secretary") })
	sc.Step(`^a fresh profiles task flow with M the assignee of task T$`, func() error { return w.cast("M", "cast") })
	sc.Step(`^a new AI member "([^"]*)" joins$`, w.add)
	sc.Step(`^the member row of "([^"]*)" has tier "([^"]*)" and skills "([^"]*)"$`, w.fields)
	sc.Step(`^the owner sends member set for "([^"]*)" with tier "([^"]*)" and skills "([^"]*)"$`, w.editFields)
	sc.Step(`^member set for "([^"]*)" with tier "([^"]*)" is "([^"]*)" from its host, another person and an admin who is not its owner$`, w.forbidden)
	sc.Step(`^member set for "([^"]*)" is "([^"]*)" with tier "expert", with skills of 201 code points and with skills holding a line break$`, w.invalid)
	sc.Step(`^in the owner's browser the AI member panel of "([^"]*)" shows tier "([^"]*)" and skills "([^"]*)"$`, w.fields)
	sc.Step(`^the owner sets tier "([^"]*)" and skills "([^"]*)" for "([^"]*)" in the browser$`, func(tier, skills, name string) error { return w.editFields(name, tier, skills) })
	sc.Step(`^no other person's browser offers the editor of "([^"]*)"$`, func(name string) error { return w.forbidden(name, "strong", "forbidden") })
	sc.Step(`^L is limited and the host of S is offline$`, w.offlineRoster)
	sc.Step(`^C takes its next turn$`, w.secretaryTurn)
	sc.Step(`^the turn lists every AI member with its tier, state and skills as in section 3 of the profiles contract$`, w.roster)
	sc.Step(`^the turn lists L as "limited" and S as "offline"$`, w.roster)
	sc.Step(`^C's instructions hold the four rules of assignment$`, func() error { return w.turn.checkAssignmentRules() })
	sc.Step(`^list_members answers as in step 1$`, w.listMembers)
	sc.Step(`^(M|S) (fails|passes) a review$`, w.reviewTask)
	sc.Step(`^M fails hosted checks$`, func() error { return w.checkTask("fails") })
	sc.Step(`^M passes hosted checks on the next commit$`, func() error { return w.checkTask("passes") })
	sc.Step(`^task T has stuck kind "([^"]*)", n (\d+) and ask (true|false)$`, w.stuck)
	sc.Step(`^task T has no stuck$`, w.notStuck)
	sc.Step(`^the browser shows (no badge|the badge) "must ask a stronger AI" on task T$`, w.badge)
	sc.Step(`^(M|S)'s (reviewed|checks) item ends with the line naming (S|C) and not (L|M) or (C|L)$`, w.escalation)
	sc.Step(`^(M|S)'s reviewed item has no added line$`, w.noEscalation)
	sc.Step(`^submit_task by (M|S) fails with "conflict" with no push, no pull request and no report$`, w.submitBlocked)
	sc.Step(`^submit_task by (M|S) succeeds$`, w.submitOK)
	sc.Step(`^(L|S|C) replies to (M|L|S)'s call in the thread of (task T|another task)$`, w.reply)
	sc.Step(`^the host of S goes offline$`, func() { w.setState("S", "offline") })
	sc.Step(`^the host of S returns$`, func() { w.setState("S", "waiting") })
	sc.Step(`^task assign gives task T to L$`, w.assignL)
	w.models = a10ModelSteps(sc, f, w.cleanup)
	a10ChoiceSteps(sc, f, w.cleanup)
}

func (w *a10ProfileWorld) cast(assignee, mode string) error {
	if err := w.setup("github"); err != nil {
		return err
	}
	w.assignee = assignee
	w.commit = 0
	w.state.Group.Limits.AskDepth = 4
	w.members[assignee] = w.members["assignee"]
	other := map[string]string{"M": "S", "S": "M"}[assignee]
	w.members[other], w.members["C"] = w.members["reviewer"], w.members["secretary"]
	if err := w.add("L"); err != nil {
		return err
	}
	for _, name := range []string{"S", "M", "L", "C"} {
		ai := w.state.AIs[w.members[name]]
		ai.Name, ai.CLI, ai.State, ai.Account, ai.HostName = name, "claude", "waiting", "personal", "node"
		ai.Profile = proto.Profile{Tier: a10CastTier(name, mode)}
		w.state.AIs[ai.ID] = ai
	}
	return nil
}

func a10CastTier(name, mode string) string {
	if mode == "default" {
		return "standard"
	}
	if mode == "secretary" {
		return map[string]string{"S": "standard", "M": "standard", "L": "light", "C": "strong"}[name]
	}
	return map[string]string{"S": "strong", "M": "standard", "L": "light", "C": "standard"}[name]
}

func (w *a10ProfileWorld) add(name string) error {
	w.ai = exampleID("a10 " + name)
	p := proto.MemberAddAI{MemberCommand: w.command("add_ai"), CLI: "claude", Name: name, Account: "personal", Approvers: []string{}}
	if err := w.apply(w.hosts["assignee"], p); err != nil {
		return err
	}
	w.members[name] = string(w.ai)
	return w.checkAnswer("")
}

func (w *a10ProfileWorld) fields(name, tier, skills string) error {
	ai := w.state.AIs[w.members[name]]
	return hostModelsEqual([]string{ai.Tier, ai.Skills}, []string{tier, skills})
}

func (w *a10ProfileWorld) editFields(name, tier, skills string) error {
	if w.models.persist != nil {
		return w.models.ownerEdit(tier, skills)
	}
	w.ai = proto.ID(w.members[name])
	return w.set(w.ownerKey, proto.ProfileSet{Tier: &tier, Skills: &skills}, "")
}

func (w *a10ProfileWorld) forbidden(name, tier, code string) error {
	w.ai = proto.ID(w.members[name])
	before := w.state.AIs[string(w.ai)]
	for _, role := range []string{"member", "admin"} {
		key := exampleKey("a10 " + role)
		log, err := homeLog([]identity.Key{key})
		if err != nil {
			return err
		}
		w.state.Logs[log.User] = log
		id := string(exampleID("a10 " + role))
		w.state.People[id] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: w.state.ID, Kind: "human", User: key.ID()}, Role: role}
		if err := w.set(key, proto.ProfileSet{Tier: &tier}, code); err != nil {
			return err
		}
	}
	if err := w.set(w.hosts["assignee"], proto.ProfileSet{Tier: &tier}, code); err != nil {
		return err
	}
	return hostModelsEqual(w.state.AIs[string(w.ai)], before)
}

func (w *a10ProfileWorld) invalid(name, code string) error {
	w.ai = proto.ID(w.members[name])
	before := w.state.AIs[string(w.ai)]
	for _, p := range []proto.ProfileSet{{Tier: new("expert")}, {Skills: new(strings.Repeat("x", 201))}, {Skills: new("two\nlines")}} {
		if err := w.set(w.ownerKey, p, code); err != nil {
			return err
		}
	}
	return hostModelsEqual(w.state.AIs[string(w.ai)], before)
}

func (w *a10ProfileWorld) setState(name, state string) {
	ai := w.state.AIs[w.members[name]]
	ai.State = state
	w.state.AIs[ai.ID] = ai
}

func (w *a10ProfileWorld) offlineRoster() { w.setState("L", "limited"); w.setState("S", "offline") }

func (w *a10ProfileWorld) syncTurn() error {
	if w.turn.h == nil {
		if err := w.turn.turnStepWorld.setup(w.cleanup); err != nil {
			return err
		}
	}
	for name, id := range map[string]string{"S": turnAIID, "M": turnPeerID, "L": tiPAIID, "C": turnSecID} {
		ai := w.state.AIs[w.members[name]]
		ai.ID, ai.Group, ai.User, ai.Host = id, turnGroupID, w.turn.ownerID, w.turn.homeKey.ID()
		ai.Allow, ai.Approvers = []string{}, []string{w.turn.ownerID}
		w.turn.h.Rows.Put(turnGroupID, "member", id, turnMustJSON(ai))
	}
	task := w.turn.taskRow()
	task.Assignee = map[string]string{"M": turnPeerID, "S": turnAIID}[w.assignee]
	task.Stuck = w.state.Tasks[w.taskID].Stuck
	w.turn.h.Rows.Put(turnGroupID, "task", turnTaskID, turnMustJSON(task))
	return nil
}

func (w *a10ProfileWorld) secretaryTurn() error {
	if err := w.syncTurn(); err != nil {
		return err
	}
	return w.turn.composeConfirmedBrief()
}

func (w *a10ProfileWorld) roster() error {
	want := "[SoloDawn] group \"Website\" · you are \"Secretary\" · secretary session\nAI members (CLI on computer · tier · state · skills):\n- \"C\" · claude on node · standard · waiting\n- \"L\" · claude on node · light · limited · docs\n- \"M\" · claude on node · standard · waiting\n- \"S\" · claude on node · strong · offline · Go backend"
	if !strings.HasPrefix(w.turn.text, want+"\n") {
		return fmt.Errorf("turn = %q, want prefix %q", w.turn.text, want)
	}
	return nil
}

func (w *a10ProfileWorld) listMembers(ctx context.Context) error {
	got, err := host.Tools["list_members"].Call(ctx, w.turn.h, host.Caller{Session: w.turn.sessionFor("secretary")}, json.RawMessage(`{}`))
	if err != nil {
		return err
	}
	var result struct {
		Members []map[string]any `json:"members"`
	}
	if err := json.Unmarshal(turnMustJSON(got), &result); err != nil {
		return err
	}
	want := []map[string]any{
		{"name": "C", "kind": "ai", "host": "node", "state": "waiting", "account": "personal", "usage": map[string]any{}},
		{"name": "L", "kind": "ai", "host": "node", "state": "limited", "account": "personal", "usage": map[string]any{}},
		{"name": "M", "kind": "ai", "host": "node", "state": "waiting", "account": "personal", "usage": map[string]any{}},
		{"name": "S", "kind": "ai", "host": "node", "state": "offline", "account": "personal", "usage": map[string]any{}},
		{"name": "owner", "kind": "human"},
	}
	return hostModelsEqual(result.Members, want)
}

func (w *a10ProfileWorld) reviewTask(_, verdict string) error {
	w.commit++
	return w.review(verdict, fmt.Sprintf("a10-review-%d", w.commit))
}

func (w *a10ProfileWorld) checkTask(verdict string) error {
	w.commit++
	return w.checkCloud(verdict, fmt.Sprintf("a10-check-%d", w.commit))
}

func (w *a10ProfileWorld) badge(kind string) error {
	stuck := w.state.Tasks[w.taskID].Stuck
	return hostModelsEqual(stuck != nil && stuck.Ask, kind == "the badge")
}

func (w *a10ProfileWorld) compose(kind string) error {
	if err := w.syncTurn(); err != nil {
		return err
	}
	s := w.turn.sessionFor("task")
	s.Member = map[string]string{"M": turnPeerID, "S": turnAIID}[w.assignee]
	msg := w.lastMsg
	msg.ID, msg.Group, msg.Thread = turnCallID, turnGroupID, new(turnTaskID)
	msg.Kind = "review"
	if kind == "checks" {
		msg.Kind = "checks"
		msg.Body = turnMustJSON(proto.ChecksBody{Stage: "cloud", OK: false, Commit: "a10-check", Items: []proto.CheckItem{{Name: "test", OK: false}}})
	}
	raw := turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 1, AI: proto.ID(s.Member), Why: kind, Task: proto.ID(s.Task)}, Msg: turnMustJSON(msg)})
	var err error
	w.turn.text, _, err = host.Compose(w.turn.h, s, host.ItemsTurn, []json.RawMessage{raw})
	return err
}

func (w *a10ProfileWorld) escalation(_, kind, candidate, _, _ string) error {
	if err := w.compose(kind); err != nil {
		return err
	}
	stuck := w.state.Tasks[w.taskID].Stuck
	want := fmt.Sprintf("This is failure %d in a row of the same kind (%s). Before you submit again, call ask to one of: %q (strong). Then continue with the answer.", stuck.N, stuck.Kind, candidate)
	return w.turn.checkEscalation(&godog.DocString{Content: want})
}

func (w *a10ProfileWorld) noEscalation(_ string) error {
	if err := w.compose("reviewed"); err != nil {
		return err
	}
	return w.turn.checkNoEscalation()
}

func (w *a10ProfileWorld) reply(by, from, thread string) error {
	task := w.taskID
	if thread == "another task" {
		task = string(exampleID("a10 other task"))
		row := w.state.Tasks[w.taskID]
		row.ID, row.Num, row.Stuck = task, 2, nil
		w.state.Tasks[task] = row
	}
	cmd := w.actorCommand(from)
	call := cmd.Nonce
	p := proto.Post[proto.CallBody]{HomeCommand: cmd, Kind: "call", Thread: new(proto.ID(task)), Text: "Advice?", Mentions: []proto.ID{}, Body: proto.CallBody{To: w.members[by], Depth: 1, Wait: true}}
	if err := w.apply(w.actorHost(from), p); err != nil {
		return err
	}
	if err := w.checkAnswer(""); err != nil {
		return err
	}
	r := proto.Post[proto.EmptyBody]{HomeCommand: w.actorCommand(by), Kind: "reply", Thread: new(proto.ID(task)), Text: "Check the shape.", Mentions: []proto.ID{}, Re: &call}
	if err := w.apply(w.actorHost(by), r); err != nil {
		return err
	}
	return w.checkAnswer("")
}

func (w *a10ProfileWorld) actorHost(name string) identity.Key {
	if w.state.AIs[w.members[name]].Host == w.hosts["reviewer"].ID() {
		return w.hosts["reviewer"]
	}
	return w.hosts["assignee"]
}

func (w *a10ProfileWorld) actorCommand(name string) proto.HomeCommand {
	cmd := w.postCommand("assignee")
	cmd.As = proto.ID(w.members[name])
	return cmd
}

func (w *a10ProfileWorld) assignL() error {
	cmd := proto.TaskAssign{TaskCommand: proto.TaskCommand{HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)}, Op: "assign", ID: proto.ID(w.taskID)}, Assignee: proto.ID(w.members["L"]), Reviewer: proto.ID(w.members["reviewer"])}
	if err := w.apply(w.ownerKey, cmd); err != nil {
		return err
	}
	return w.checkAnswer("")
}

func (w *a10ProfileWorld) submitFixture() (*profileSubmitWorld, error) {
	st := &profileSubmitWorld{tskScenario{t: w.f.t, cleanup: w.cleanup}}
	if err := st.setupBlocked(); err != nil {
		return nil, err
	}
	ids := map[string]string{"S": tskAccReviewID, "M": tskAccWorkerID, "L": string(exampleID("a10 submit L")), "C": string(exampleID("a10 submit C"))}
	ids[w.assignee], ids[map[string]string{"M": "S", "S": "M"}[w.assignee]] = tskAccWorkerID, tskAccReviewID
	for name, id := range ids {
		ai := w.state.AIs[w.members[name]]
		st.putSubmitMember(id, name, ai.Tier, ai.State, ai.Secretary)
	}
	task := st.home.currentTask()
	task.Stuck = w.state.Tasks[w.taskID].Stuck
	st.home.putTask(task)
	return st, nil
}

func (w *a10ProfileWorld) submitBlocked(_ string) error {
	st, err := w.submitFixture()
	if err != nil {
		return err
	}
	st.submitEarly()
	coded, ok := errors.AsType[*host.Error](st.lastErr)
	if !ok {
		return fmt.Errorf("submission error = %v", st.lastErr)
	}
	if err := hostModelsEqual([]string{coded.Code, coded.Message}, []string{"conflict", `Ask a stronger AI before submitting again. Candidates: "S" (strong).`}); err != nil {
		return err
	}
	if err := hostModelsEqual(len(st.home.allCmds()), 0); err != nil {
		return err
	}
	return st.checkNoRemoteWrites()
}

func (w *a10ProfileWorld) submitOK(_ string) error {
	st, err := w.submitFixture()
	if err != nil {
		return err
	}
	if err := st.whenRemoteChecksAndSubmit("Ready after advice."); err != nil {
		return err
	}
	return st.checkSubmitted()
}
