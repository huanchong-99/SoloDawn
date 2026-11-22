//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
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
	h          *host.Host
	pUser      string
	qUser      string
	homeKey    identity.Key
	text       string
	singleDec  string
	singleConf string
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
	sc.Step(`^the host composes a checks turn for "Q-team" on task 12 from a checks row written by the home$`, w.composeChecks)
	sc.Step(`^"Q-team" posted a decision card to "Q" on task 12 and "(P|Q)" chose option "([^"]+)"(?: as admin)?$`, w.composeTaskDecision)
	sc.Step(`^"Q" confirms a brief posted by "Secretary" while "P" has not listed "Q"$`, func() error {
		return w.composeSecretaryConfirmed(nil, tiQMemID)
	})
	sc.Step(`^"P" lists "Q" on "Secretary" and "Q" confirms a brief posted by "Secretary"$`, func() error {
		return w.composeSecretaryConfirmed([]string{w.qUser}, tiQMemID)
	})
	sc.Step(`^"P" confirms a brief and chooses on a decision card in a one-person group on "P1"$`, w.composeOnePersonChoices)
	sc.Step(`^the turninfo item header is:$`, w.checkHeader)
	sc.Step(`^the turninfo composed text is:$`, w.checkFullText)
	sc.Step(`^the turninfo second line is:$`, w.checkSecondLine)
	sc.Step(`^the turninfo header and second line are:$`, w.checkTopTwoLines)
	sc.Step(`^both one-person turns read "instruction" in their header and on their second line$`, w.checkOnePersonBothInstruct)
}

func (w *turnInfoWorld) setup(cleanup func(func()), node string) error {
	db, err := store.Open(":memory:")
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.pUser, w.qUser, w.homeKey = turnSeedKey(11).ID(), turnSeedKey(22).ID(), turnSeedKey(33)
	owner := w.qUser
	if node == "P1" {
		owner = w.pUser
	}
	w.h, _, err = host.New(db, w.homeKey, func() time.Time { return time.UnixMilli(turnTimeMS) })
	if err != nil {
		return err
	}
	write := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", string(turnMustJSON(owner)))
		return err
	}
	if err := db.Write(write); err != nil {
		return err
	}
	w.seedGroupAndMembers()
	return nil
}

func (w *turnInfoWorld) seedGroupAndMembers() {
	w.h.Rows.Put(turnGroupID, "group", turnGroupID, turnMustJSON(proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: turnGroupID, Seq: 1},
		Name:      "Website", Kind: "project", Base: "main", Forge: "github", Home: w.homeKey.ID(),
		Secretary: turnSecID, Checks: []string{}, Pass: 90, PassFoundation: 70, Created: turnTimeMS,
	}))
	w.putHuman(tiPMemID, w.pUser, "P", "owner")
	w.putHuman(tiQMemID, w.qUser, "Q", "admin")
	w.putAI(tiQPersonalID, w.qUser, "Q-personal", "personal", []string{}, false)
	w.putAI(tiQTeamID, w.qUser, "Q-team", "team", []string{}, false)
	w.putAI(tiPAIID, w.pUser, "P-AI", "team", []string{}, false)
	w.putAI(turnSecID, w.pUser, "Secretary", "team", []string{}, true)
	w.h.Rows.Put(turnGroupID, "task", turnTaskID, turnMustJSON(proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: turnTaskID, Seq: 6},
		Group:     turnGroupID, Num: 12, Title: "Node: report connection path changes",
		Slug: "node-report-connection-path-changes", Origin: "brief", Brief: turnBriefID,
		State: "active", Step: "branch", Assignee: tiQTeamID, Reviewer: tiPAIID, Pass: 90, Depends: []string{}, Created: turnTimeMS,
	}))
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
	return host.Session{
		SessionKey: host.SessionKey{Group: turnGroupID, Member: aiID, Task: turnTaskID},
		Kind:       host.TaskSession, Home: w.homeKey.ID(), Name: name, CLISession: "c1",
	}
}

func (w *turnInfoWorld) secSession() host.Session {
	return host.Session{
		SessionKey: host.SessionKey{Group: turnGroupID, Member: turnSecID},
		Kind:       host.GroupSession, Home: w.homeKey.ID(), Name: "Secretary", Secretary: true, CLISession: "c1",
	}
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
			Task: turnTaskID, Num: 12, Title: "Paths", Commit: "9f2c1ab", Round: 1, Score: 86, Pass: 90, OK: false,
			Items: []proto.ReviewItem{{Text: "No event after switching back to direct", File: &file, Line: &line}},
		},
	})
	raw := turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 2, AI: proto.ID(tiQTeamID), Why: "reviewed", Task: proto.ID(turnTaskID), Msg: proto.ID(turnCallID)}, Msg: msg})
	w.text, _, err = host.Compose(w.h, w.taskSession(tiQTeamID, "Q-team"), host.ItemsTurn, []json.RawMessage{raw})
	return err
}

func (w *turnInfoWorld) composeChecks() (err error) {
	th := turnTaskID
	msg := turnMustJSON(proto.MessageRow[proto.ChecksBody, proto.NoCard]{
		RowHeader: proto.RowHeader{Type: "msg", ID: turnCallID, Seq: 13},
		Pos:       13, Group: turnGroupID, Thread: &th, By: "system", Kind: "checks", Timestamp: turnTimeMS, Mentions: []string{},
		Body: proto.ChecksBody{Stage: "cloud", OK: false, Commit: "9f2c1ab", Items: []proto.CheckItem{{Name: "test", OK: false}}},
	})
	raw := turnMustJSON(proto.InboxItem{Item: proto.Item{Item: 3, AI: proto.ID(tiQTeamID), Why: "checks", Task: proto.ID(turnTaskID), Msg: proto.ID(turnCallID)}, Msg: msg})
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

func (w *turnInfoWorld) composeOnePersonChoices() error {
	w.h.Rows.Put(turnGroupID, "member", tiQMemID, json.RawMessage(`{"r":"member","id":"`+tiQMemID+`","seq":99,"gone":true}`))
	if err := w.composeSecretaryConfirmed(nil, tiPMemID); err != nil {
		return err
	}
	w.singleConf = w.text
	decRaw := w.decisionInbox(turnSecID, nil, []string{w.pUser}, tiPMemID, "A")
	var err error
	w.singleDec, _, err = host.Compose(w.h, w.secSession(), host.ItemsTurn, []json.RawMessage{decRaw})
	return err
}

func (w *turnInfoWorld) checkHeader(doc *godog.DocString) error {
	lines := strings.Split(w.text, "\n")
	if len(lines) < 2 || lines[1] != doc.Content {
		return fmt.Errorf("got:\n%s\nwant header:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *turnInfoWorld) checkFullText(doc *godog.DocString) error {
	if w.text != doc.Content {
		return fmt.Errorf("got:\n%s\nwant:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *turnInfoWorld) checkSecondLine(doc *godog.DocString) error {
	lines := strings.Split(w.text, "\n")
	if len(lines) < 3 || lines[2] != doc.Content {
		return fmt.Errorf("got:\n%s\nwant second line:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *turnInfoWorld) checkTopTwoLines(doc *godog.DocString) error {
	lines := strings.Split(w.text, "\n")
	if len(lines) < 3 || lines[1]+"\n"+lines[2] != doc.Content {
		return fmt.Errorf("got:\n%s\nwant:\n%s", w.text, doc.Content)
	}
	return nil
}

func (w *turnInfoWorld) checkOnePersonBothInstruct() error {
	if !strings.Contains(w.singleConf, "== 1 of 1 · confirmed · instruction\n-- from \"P\" in main · instruction\n") {
		return fmt.Errorf("unexpected one-person confirmed:\n%s", w.singleConf)
	}
	if !strings.Contains(w.singleDec, "== 1 of 1 · decided · instruction\n-- from \"P\" in main · instruction\n") {
		return fmt.Errorf("unexpected one-person decided:\n%s", w.singleDec)
	}
	return nil
}
