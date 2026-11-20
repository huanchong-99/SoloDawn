package proto

import (
	"fmt"
	"reflect"
	"testing"
)

var (
	taskID    = id("task")
	agent     = id("agent")
	reviewer  = id("reviewer")
	secretary = id("secretary")
	person    = id("member")
)

type messageExample struct {
	kind, author, text string
	thread, replyTo    *string
	main               bool
	signedBody         any // when the home adds fields to the posted body
}

func exampleRow[B, S any](t *testing.T, m messageExample, body B, state *S, seq int64) MessageRow[B, S] {
	t.Helper()
	r := MessageRow[B, S]{
		RowHeader: RowHeader{"msg", id(m.kind), seq}, Pos: 790, Group: group,
		Thread: m.thread, Main: m.main, By: m.author, Kind: m.kind,
		Timestamp: ts - 500, Text: m.text, Mentions: []string{}, Re: m.replyTo, Body: body, State: state,
	}
	if m.author != "system" {
		if m.signedBody == nil {
			m.signedBody = body
		}
		post := fmt.Sprintf(`{"t":"post","n":%q,"ts":%d,"g":%q,"as":%q,"kind":%q,"thread":%s,"main":%t,"text":%s,"mentions":[],"re":%s,"body":%s}`,
			r.ID, r.Timestamp, group, r.By, r.Kind, wireJSON(t, r.Thread), r.Main, wireJSON(t, r.Text), wireJSON(t, r.Re), wireJSON(t, m.signedBody))
		r.Env = new(signed(node, post))
	}
	return r
}

func actExample(kind, action, arg, text string) Envelope {
	nonce := id("act " + kind + " " + action)
	// Preserve the nonce of the original decided-decision vector.
	if action == "choose" {
		nonce = id("act")
	}
	payload := fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":%q,"arg":%q,"text":%q}`, nonce, ts, group, id(kind), action, arg, text)
	return signed(browser, payload)
}

func decisionExample() (messageExample, DecisionBody) {
	return messageExample{kind: "decision", author: agent, thread: &taskID, main: true, text: "Choose a location."},
		DecisionBody{&taskID, "Where should the path appear?", new("Only space for one label."),
			[]DecisionOption{{"A", "Header", "Always visible."}}, new("A"), []string{user}}
}

func decidedExample(t *testing.T) MessageRow[DecisionBody, DecisionDecidedState] {
	m, body := decisionExample()
	state := DecisionDecidedState{"decided", person, ts, new("A"), new("Header works."), actExample("decision", "choose", "A", "Header works.")}
	return exampleRow(t, m, body, &state, 814)
}

func TestDecisionGoldens(t *testing.T) {
	m, body := decisionExample()
	golden(t, "row-msg-decision-pending", exampleRow(t, m, body, &StatusOnly{"pending"}, 790))
	golden(t, "row-msg-decision-decided", decidedExample(t))
}

func TestBriefGoldens(t *testing.T) {
	m := messageExample{kind: "brief", author: secretary}
	body := BriefBody{
		"Show connection paths", "People can see how a peer is connected.", []string{"Show a path change within 1 s."},
		BriefScope{[]string{"node events", "web indicator"}, []string{"relay service"}},
		[]BriefTask{{"Report path changes", agent, reviewer, true, []int{}}, {"Show the path", reviewer, agent, false, []int{0}}},
		90, person, nil,
	}
	golden(t, "row-msg-brief-pending", exampleRow(t, m, body, &StatusOnly{"pending"}, 790))
	confirmed := ActedState{"confirmed", person, ts, actExample("brief", "confirm", "", "")}
	golden(t, "row-msg-brief-confirmed", exampleRow(t, m, body, &confirmed, 814))
	note := "Include the relay path."
	revise := BriefReviseState{"revise", person, ts, actExample("brief", "revise", "", note), &note}
	golden(t, "row-msg-brief-revise", exampleRow(t, m, body, &revise, 814))
	superseded := CardState{Status: "superseded", By: &revise.By, At: &revise.At, Act: &revise.Act, Note: &note}
	golden(t, "row-msg-brief-superseded", exampleRow(t, m, body, &superseded, 815))
}

func TestApprovalGoldens(t *testing.T) {
	m := messageExample{kind: "approval", author: agent, thread: &taskID, main: true}
	body := ApprovalBody{taskID, agent, "Bash", "pnpm exec playwright install chromium", new("Time the path change in a browser."), "command", []string{user}}
	golden(t, "row-msg-approval-pending", exampleRow(t, m, body, &StatusOnly{"pending"}, 790))
	for _, a := range []struct{ status, action string }{{"approved", "approve"}, {"denied", "deny"}} {
		state := ActedState{a.status, person, ts, actExample("approval", a.action, "", "")}
		golden(t, "row-msg-approval-"+a.status, exampleRow(t, m, body, &state, 814))
	}
	for _, status := range []string{"expired", "reassigned"} {
		golden(t, "row-msg-approval-"+status, exampleRow(t, m, body, &TimedState{status, ts}, 814))
	}
}

func TestCallGoldens(t *testing.T) {
	m := messageExample{kind: "call", author: agent, thread: &taskID, text: "Which event reports a path change?"}
	body := CallBody{reviewer, 1, true}
	golden(t, "row-msg-call-waiting", exampleRow(t, m, body, &StatusOnly{"waiting"}, 790))
	golden(t, "row-msg-call-answered", exampleRow(t, m, body, &CallAnsweredState{"answered", id("reply"), 500}, 814))
}

func nonCard[B any](t *testing.T, m messageExample, body B) {
	t.Helper()
	golden(t, "row-msg-"+m.kind, exampleRow[B, StatusOnly](t, m, body, nil, 790))
}

// CardState reads every state, a superseded brief's included, and keeps what is there.
func TestCardStateRetainsPresentZeroValues(t *testing.T) {
	for _, wire := range []string{
		`{"status":"superseded"}`,
		`{"status":"expired","at":0}`,
		`{"status":"answered","reply":"m","ms":0}`,
		`{"status":"decided","by":"m","at":0,"text":"","act":{"p":"act","k":"key","s":"signature"}}`,
	} {
		var state CardState
		if err := Decode([]byte(wire), &state); err != nil {
			t.Fatal(err)
		}
		if got := wireJSON(t, state); !reflect.DeepEqual(jsonValue(t, got), jsonValue(t, []byte(wire))) {
			t.Errorf("state = %s; want %s", got, wire)
		}
	}
}

func TestNonCardGoldens(t *testing.T) {
	nonCard(t, messageExample{kind: "text", author: agent, text: "The path is direct."}, EmptyBody{})
	nonCard(t, messageExample{kind: "progress", author: agent, thread: &taskID, text: "Added the path indicator."}, EmptyBody{})
	nonCard(t, messageExample{kind: "question", author: agent, text: "Should the path include latency?"}, EmptyBody{})
	nonCard(t, messageExample{kind: "reply", author: reviewer, thread: &taskID, replyTo: new(id("call")), text: "Use the path change event."}, EmptyBody{})
	nonCard(t, messageExample{kind: "task", author: "system"}, TaskBody{taskID})
	nonCard(t, messageExample{kind: "summary", author: secretary, thread: &taskID, main: true, signedBody: EmptyBody{}},
		SummaryBody{taskID, 94, 2, 72, 318, 42, new(28)})
	nonCard(t, messageExample{kind: "report", author: agent, thread: &taskID}, ReportBody{"9f2c1ab", 4, 212, 18, new(31)})
	nonCard(t, messageExample{kind: "checks", author: "system", thread: &taskID}, ChecksBody{"local", true, "9f2c1ab", []CheckItem{{"go test ./...", true, new("42 tests")}}})
	items := []ReviewItem{{"Missing relay path", false, new("path.go"), new(88)}}
	review := ReviewBody{taskID, 12, "Show connection paths", "9f2c1ab", 1, 86, 90, false, items}
	nonCard(t, messageExample{kind: "review", author: reviewer, thread: &taskID, signedBody: map[string]any{
		"task": review.Task, "commit": review.Commit, "score": review.Score, "items": review.Items}}, review)
	nonCard(t, messageExample{kind: "handback", author: agent, thread: &taskID}, HandbackBody{[]string{"path.go"}, []CommitSubject{{"9f2c1ab", "Emit path changes"}}, 40, 3})
	nonCard(t, messageExample{kind: "system", author: "system", thread: &taskID}, TurnInterruptedBody{SystemCode{"turn_interrupted"}, agent, "timeout", &taskID, new("Turn deadline reached.")})
}
