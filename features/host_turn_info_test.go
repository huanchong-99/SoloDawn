//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_turn_info"] = hostTurnInfoSteps
}

const (
	tiPMemID      = "aB1cD2eF3gH4iJ5kL6mN7A"
	tiQMemID      = "bC2dE3fG4hI5jK6lM7nO8Q"
	tiQPersonalID = "cD3eF4gH5iJ6kL7mN8oP9g"
	tiQTeamID     = "dE4fG5hI6jK7lM8nO9pQ0w"
	tiPAIID       = "eF5gH6iJ7kL8mN9oP0qR1A"
)

type turnInfoWorld struct {
	turnStepWorld
	pUser string
	qUser string
}

func hostTurnInfoSteps(sc *godog.ScenarioContext, _ fixtures) {
	w := &turnInfoWorld{}
	cleanup := afterScenario(sc)
	sc.Step(`^a two-owner host turn fixture on "(P1|Q1)"$`, func(node string) error { return w.setup(cleanup, node) })
	sc.Step(`^"(P|Q)" mentions "(Q-personal|Q-team)" in the thread of task 12 saying "([^"]+)"$`, w.mentionInThread)
	sc.Step(`^"Q" lists "P" on "Q-team" and "P" mentions "Q-team" in the thread of task 12 saying "([^"]+)"$`, w.listAndMentionInThread)
	sc.Step(`^"Q" posts in the thread of task 12 to "Q-personal" saying "([^"]+)"$`, func(text string) error {
		return w.mentionInThread("Q", "Q-personal", text)
	})
	sc.Step(`^the host composes a reviewed turn for "Q-team" on task 12 from a review written by "P-AI"$`, w.composeReviewed)
	sc.Step(`^the host composes a checks turn for "Q-team" on task 12 from a checks row written by the home$`, rewritten("AC-2", "the checks message and the item checks are removed (MODEL-1126 plan 3.1)"))
	sc.Step(`^"Q-team" posted a decision card to "Q" on task 12 and "(P|Q)" chose option "([^"]+)"(?: as admin)?$`, w.composeTaskDecision)
	sc.Step(`^"Q" confirms a brief posted by "Secretary" while "P" has not listed "Q"$`, func() error {
		return w.composeSecretaryConfirmed(nil, tiQMemID)
	})
	sc.Step(`^"P" lists "Q" on "Secretary" and "Q" confirms a brief posted by "Secretary"$`, func() error {
		return w.composeSecretaryConfirmed([]string{w.qUser}, tiQMemID)
	})
	sc.Step(`^the turninfo item header is:$`, w.checkItemHeader)
	sc.Step(`^the turninfo composed text is:$`, w.checkTurnText)
	sc.Step(`^the turninfo second line is:$`, func(doc *godog.DocString) error { return w.checkLines(doc, 2, 1) })
	sc.Step(`^the turninfo header and second line are:$`, w.checkFirstItem)
}

func (w *turnInfoWorld) setup(cleanup func(func()), node string) error {
	if err := w.turnStepWorld.setup(cleanup); err != nil {
		return err
	}
	w.pUser, w.qUser = w.ownerID, turnSeedKey(22).ID()
	owner := w.qUser
	if node == "P1" {
		owner = w.pUser
	}
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("UPDATE setting SET value = ? WHERE key = 'owner'", string(turnMustJSON(owner)))
		return err
	}
	if err := w.h.Store.Write(write); err != nil {
		return err
	}
	w.seedGroupAndMembers()
	return nil
}

func (w *turnInfoWorld) seedGroupAndMembers() {
	w.h.Rows.Put(turnGroupID, "group", turnGroupID, turnMustJSON(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: turnGroupID, Seq: 1},
		Name:      "Website", Home: w.homeKey.ID(),
		Secretary: turnSecID, Pass: 90, PassFoundation: 70, Created: turnTimeMS,
	}))
	w.putHuman(tiPMemID, w.pUser, "P", "owner")
	w.putHuman(tiQMemID, w.qUser, "Q", "admin")
	w.putAI(tiQPersonalID, w.qUser, "Q-personal", "personal", []string{}, false)
	w.putAI(tiQTeamID, w.qUser, "Q-team", "team", []string{}, false)
	w.putAI(tiPAIID, w.pUser, "P-AI", "team", []string{}, false)
	w.putAI(turnSecID, w.pUser, "Secretary", "team", []string{}, true)
	task := w.taskRow()
	task.Assignee, task.Reviewer = tiQTeamID, tiPAIID
	w.h.Rows.Put(turnGroupID, "task", turnTaskID, turnMustJSON(task))
}

func (w *turnInfoWorld) putHuman(id, user, name, role string) {
	w.h.Rows.Put(turnGroupID, "member", id, turnMustJSON(proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 2}, Group: turnGroupID, Kind: "human", User: user, Name: name, LastSeen: turnTimeMS},
		Role:      role, Online: true,
	}))
}

func (w *turnInfoWorld) putAI(id, user, name, account string, allow []string, sec bool) {
	w.h.Rows.Put(turnGroupID, "member", id, turnMustJSON(proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id, Seq: 3}, Group: turnGroupID, Kind: "ai", User: user, Name: name, LastSeen: turnTimeMS},
		CLI:       "claude", Host: w.homeKey.ID(), HostName: "node", Secretary: sec, Account: account,
		Allow: allow, Approvers: []string{user}, State: "waiting", Path: "direct",
	}))
}

func (w *turnInfoWorld) taskSession(aiID, name string) host.Session {
	s := w.sessionFor("task")
	s.Member, s.Name, s.CLISession = aiID, name, "c1"
	return s
}

func (w *turnInfoWorld) secSession() host.Session {
	s := w.sessionFor("secretary")
	s.CLISession = "c1"
	return s
}

func (w *turnInfoWorld) mentionInThread(who, aiName, text string) (err error) {
	by := tiPMemID
	if who == "Q" {
		by = tiQMemID
	}
	aiID := tiQPersonalID
	if aiName == "Q-team" {
		aiID = tiQTeamID
	}
	rawTask, _ := w.h.Rows.Get(turnGroupID, "task", turnTaskID)
	var task proto.TaskRow
	proto.Decode(rawTask, &task)
	task.Assignee = aiID
	w.h.Rows.Put(turnGroupID, "task", turnTaskID, turnMustJSON(task))
	th := turnTaskID
	msg := turnMustJSON(proto.MessageRow[proto.EmptyBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: turnCallID, Seq: 10},
		Pos:       10, Group: turnGroupID, Thread: &th, By: by, Kind: "text",
		Timestamp: turnTimeMS, Text: text, Mentions: []string{aiID}, Body: proto.EmptyBody{},
	})
	raw := turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 1, AI: proto.ID(aiID), Why: "mention", Task: proto.ID(turnTaskID), Msg: proto.ID(turnCallID)}, Msg: msg})
	w.text, _, err = host.Compose(w.h, w.taskSession(aiID, aiName), host.ItemsTurn, []json.RawMessage{raw})
	return err
}

func (w *turnInfoWorld) listAndMentionInThread(text string) error {
	w.putAI(tiQTeamID, w.qUser, "Q-team", "team", []string{w.pUser}, false)
	return w.mentionInThread("P", "Q-team", text)
}

func (w *turnInfoWorld) composeReviewed() (err error) {
	th, file, line := turnTaskID, "conn/switch.go", 88
	msg := turnMustJSON(proto.MessageRow[proto.ReviewBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: turnCallID, Seq: 12},
		Pos:       12, Group: turnGroupID, Thread: &th, By: tiPAIID, Kind: "review", Timestamp: turnTimeMS, Mentions: []string{},
		Body: proto.ReviewBody{
			Task: turnTaskID, Num: 12, Title: "Paths", Report: "9f2c1ab", Round: 1, Score: 86, Pass: 90, OK: false,
			Items: []proto.ReviewItem{{Text: "No event after switching back to direct", File: &file, Line: &line}},
		},
	})
	raw := turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 2, AI: proto.ID(tiQTeamID), Why: "reviewed", Task: proto.ID(turnTaskID), Msg: proto.ID(turnCallID)}, Msg: msg})
	w.text, _, err = host.Compose(w.h, w.taskSession(tiQTeamID, "Q-team"), host.ItemsTurn, []json.RawMessage{raw})
	return err
}

func (w *turnInfoWorld) composeTaskDecision(actor, choice string) (err error) {
	by := tiPMemID
	if actor == "Q" {
		by = tiQMemID
	}
	th := turnTaskID
	raw := w.decisionInbox(tiQTeamID, &th, []string{w.qUser}, by, choice)
	w.text, _, err = host.Compose(w.h, w.taskSession(tiQTeamID, "Q-team"), host.ItemsTurn, []json.RawMessage{raw})
	return err
}

func (w *turnInfoWorld) decisionInbox(aiID string, thread *string, to []string, by, choice string) json.RawMessage {
	var taskID proto.ID
	if thread != nil {
		taskID = proto.ID(*thread)
	}
	card := turnMustJSON(proto.MessageRow[proto.DecisionBody, *proto.CardState]{
		RowHeader: proto.RowHeader{Type: "msg", ID: turnCallID, Seq: 14},
		Pos:       14, Group: turnGroupID, Thread: thread, Main: true, By: aiID, Kind: "decision", Timestamp: turnTimeMS, Mentions: []string{},
		Body:  proto.DecisionBody{Task: thread, Question: "Where should the notice go?", Options: []proto.DecisionOption{{ID: "A", Label: "Top bar", Effect: "Visible"}}, To: to},
		State: &proto.CardState{Status: "decided", By: &by, Choice: &choice},
	})
	return turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 4, AI: proto.ID(aiID), Why: "decided", Task: taskID, Msg: proto.ID(turnCallID)}, Msg: card})
}

func (w *turnInfoWorld) composeSecretaryConfirmed(allow []string, confirmer string) (err error) {
	if allow == nil {
		allow = []string{}
	}
	w.putAI(turnSecID, w.pUser, "Secretary", "team", allow, true)
	body := proto.BriefBody{
		Title: "Connection path", Goal: "Show path.", Criteria: []string{"1 s"},
		Scope: proto.BriefScope{In: []string{"node"}, Out: []string{}},
		Tasks: []proto.BriefTask{{Title: "Node", Assignee: tiQTeamID, Reviewer: tiPAIID, Depends: []int{}}},
		Pass:  90, Requester: tiPMemID,
	}
	msg := turnMustJSON(proto.MessageRow[proto.BriefBody, *proto.CardState]{
		RowHeader: proto.RowHeader{Type: "msg", ID: turnBriefID, Seq: 15},
		Pos:       15, Group: turnGroupID, Main: true, By: turnSecID, Kind: "brief", Timestamp: turnTimeMS, Mentions: []string{},
		Body: body, State: &proto.CardState{Status: "confirmed", By: &confirmer},
	})
	raw := turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 5, AI: proto.ID(turnSecID), Why: "confirmed", Msg: proto.ID(turnBriefID)}, Msg: msg})
	w.text, _, err = host.Compose(w.h, w.secSession(), host.ItemsTurn, []json.RawMessage{raw})
	return err
}

func (w *turnInfoWorld) checkFirstItem(doc *godog.DocString) error {
	for i, line := range strings.Split(w.text, "\n") {
		if strings.HasPrefix(line, "== 1 of ") {
			return w.checkLines(doc, i, 2)
		}
	}
	return fmt.Errorf("first item missing from:\n%s", w.text)
}

func (w *turnInfoWorld) checkLines(doc *godog.DocString, start, count int) error {
	lines := strings.Split(w.text, "\n")
	if len(lines) < start+count || strings.Join(lines[start:start+count], "\n") != doc.Content {
		return fmt.Errorf("got:\n%s\nwant:\n%s", w.text, doc.Content)
	}
	return nil
}
