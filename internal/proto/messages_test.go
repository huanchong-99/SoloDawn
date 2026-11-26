package proto

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	mentions           []string
	signedBody         any // when the home adds fields to the posted body
}

func ids(texts []string) []ID {
	list := []ID{}
	for _, text := range texts {
		list = append(list, ID(text))
	}
	return list
}

// exampleRow is a message row with the post that its author signed, which is
// also the example of a post of that kind.
func exampleRow[B, S any](t *testing.T, m messageExample, body B, state S, seq int64) MessageRow[B, S] {
	t.Helper()
	if m.mentions == nil {
		m.mentions = []string{}
	}
	r := MessageRow[B, S]{
		RowHeader: RowHeader{"msg", id(m.kind), seq}, Pos: 790, Group: group,
		Thread: m.thread, Main: m.main, By: m.author, Kind: m.kind,
		Timestamp: ts - 500, Text: m.text, Mentions: m.mentions, Re: m.replyTo, Body: body, State: state,
	}
	if m.author != "system" {
		if m.signedBody == nil {
			m.signedBody = body
		}
		post := wireJSON(t, Post[any]{
			HomeCommand{"post", ID(r.ID), r.Timestamp, ID(group), ID(r.By)},
			r.Kind, (*ID)(r.Thread), r.Main, r.Text, ids(r.Mentions), (*ID)(r.Re), m.signedBody,
		})
		goldenJSON(t, "post-"+m.kind, post)
		r.Env = new(signed(node, string(post)))
	}
	return r
}

// actExample is the act that decides the example card of that kind, signed by the person's browser.
func actExample(t *testing.T, kind, action, arg, text string) Envelope {
	t.Helper()
	nonce := id("act " + kind + " " + action)
	// Preserve the nonce of the original decided-decision vector.
	if action == "choose" {
		nonce = id("act")
	}
	act := Act{HomeCommand{Type: "act", Nonce: ID(nonce), Timestamp: ts, Group: ID(group)}, ID(id(kind)), action, arg, text}
	golden(t, "act-"+action, act)
	return signed(browser, string(wireJSON(t, act)))
}

func decisionExample() (messageExample, DecisionBody) {
	return messageExample{kind: "decision", author: agent, thread: &taskID, main: true, text: "Choose a location."},
		DecisionBody{&taskID, "Where should the path appear?", new("Only space for one label."),
			[]DecisionOption{{"A", "Header", "Always visible."}}, new("A"), []string{user}}
}

func decidedExample(t *testing.T) MessageRow[DecisionBody, DecisionDecidedState] {
	m, body := decisionExample()
	state := DecisionDecidedState{"decided", person, ts, new("A"), new("Header works."), actExample(t, "decision", "choose", "A", "Header works.")}
	return exampleRow(t, m, body, state, 814)
}

func TestDecisionGoldens(t *testing.T) {
	m, body := decisionExample()
	golden(t, "row-msg-decision-pending", exampleRow(t, m, body, StatusOnly{"pending"}, 790))
	golden(t, "row-msg-decision-decided", decidedExample(t))
	// The task it names was done or cancelled while it was pending (contracts/messages.md 3.3).
	golden(t, "row-msg-decision-expired", exampleRow(t, m, body, TimedState{"expired", ts}, 814))
}

func TestBriefGoldens(t *testing.T) {
	m := messageExample{kind: "brief", author: secretary}
	body := BriefBody{
		Title: "Show connection paths", Goal: "People can see how a peer is connected.", Criteria: []string{"Show a path change within 1 s."},
		Scope: BriefScope{[]string{"node events", "web indicator"}, []string{"relay service"}},
		Tasks: []BriefTask{{"Report path changes", agent, reviewer, true, []int{}}, {"Show the path", reviewer, agent, false, []int{0}}},
		Pass:  90, Requester: person, Replaces: nil,
	}
	golden(t, "row-msg-brief-pending", exampleRow(t, m, body, StatusOnly{"pending"}, 790))
	confirmed := ActedState{"confirmed", person, ts, actExample(t, "brief", "confirm", "", "")}
	golden(t, "row-msg-brief-confirmed", exampleRow(t, m, body, confirmed, 814))
	note := "Include the relay path."
	revise := BriefReviseState{"revise", person, ts, actExample(t, "brief", "revise", "", note), &note}
	golden(t, "row-msg-brief-revise", exampleRow(t, m, body, revise, 814))
	superseded := CardState{Status: "superseded", By: &revise.By, At: &revise.At, Act: &revise.Act, Note: &note}
	golden(t, "row-msg-brief-superseded", exampleRow(t, m, body, superseded, 815))

	withPreset := body
	withPreset.Preset = "Calm dashboard"
	presetID := id("brief preset")
	presetPost := wireJSON(t, Post[BriefBody]{
		HomeCommand{"post", ID(presetID), ts - 500, ID(group), ID(secretary)},
		"brief", nil, false, "", []ID{}, nil, withPreset,
	})
	golden(t, "row-msg-brief-preset", MessageRow[BriefBody, StatusOnly]{
		RowHeader: RowHeader{"msg", presetID, 903}, Pos: 903, Group: group,
		By: secretary, Kind: "brief", Timestamp: ts - 500, Mentions: []string{},
		Body: withPreset, State: StatusOnly{"pending"}, Env: new(signed(node, string(presetPost))),
	})
}

func TestApprovalGoldens(t *testing.T) {
	// The host sets the mentions of an approval to the members who may decide it (contracts/messages.md section 1).
	m := messageExample{kind: "approval", author: agent, thread: &taskID, main: true, mentions: []string{person}}
	// The request was longer than what the card shows (contracts/messages.md 2.3).
	body := ApprovalBody{taskID, agent, "Bash", "pnpm exec playwright install chromium", new(4096), new("Time the path change in a browser."), "command", []string{user}}
	golden(t, "row-msg-approval-pending", exampleRow(t, m, body, StatusOnly{"pending"}, 790))
	for _, a := range []struct{ status, action string }{{"approved", "approve"}, {"denied", "deny"}} {
		state := ActedState{a.status, person, ts, actExample(t, "approval", a.action, "", "")}
		golden(t, "row-msg-approval-"+a.status, exampleRow(t, m, body, state, 814))
	}
	golden(t, "row-msg-approval-expired", exampleRow(t, m, body, TimedState{"expired", ts}, 814))
	// The secretary gave the task to another AI (contracts/messages.md 3.2).
	golden(t, "row-msg-approval-reassigned", exampleRow(t, m, body, ReassignedState{"reassigned", secretary, ts}, 814))
}

func TestCallGoldens(t *testing.T) {
	m := messageExample{kind: "call", author: agent, thread: &taskID, text: "Which event reports a path change?"}
	body := CallBody{reviewer, 1, true}
	golden(t, "row-msg-call-waiting", exampleRow(t, m, body, StatusOnly{"waiting"}, 790))
	golden(t, "row-msg-call-answered", exampleRow(t, m, body, CallAnsweredState{"answered", id("reply"), 500}, 814))
	golden(t, "row-msg-call-denied", exampleRow(t, m, body, TimedState{"denied", ts}, 814))
}

func TestConfirmGoldens(t *testing.T) {
	confirmID := id("confirm")
	otherUser := device(signer("other browser"))
	otherAI := id("other ai")

	// A watch request is posted by a person (no as); the home adds to.
	watchPost := wireJSON(t, Post[ConfirmSigned]{
		HomeCommand{Type: "post", Nonce: ID(confirmID), Timestamp: ts - 500, Group: ID(group)},
		"confirm", nil, true, "", []ID{}, nil, ConfirmSigned{"watch", ID(otherAI)},
	})
	goldenJSON(t, "post-confirm", watchPost)
	golden(t, "row-msg-confirm-pending", MessageRow[ConfirmBody, StatusOnly]{
		RowHeader: RowHeader{"msg", confirmID, 790}, Pos: 790, Group: group,
		Main: true, By: person, Kind: "confirm", Timestamp: ts - 500, Mentions: []string{},
		Body: ConfirmBody{"watch", otherAI, []string{otherUser}}, State: StatusOnly{"pending"},
		Env: new(signed(browser, string(watchPost))),
	})

	confirmAct := func(action string) Envelope {
		act := Act{HomeCommand{Type: "act", Nonce: ID(id("act confirm " + action)), Timestamp: ts, Group: ID(group)}, ID(confirmID), action, "", ""}
		var decoded Act
		if err := Decode(wireJSON(t, act), &decoded); err != nil || decoded != act {
			t.Fatalf("confirm act %s: %+v, %v", action, decoded, err)
		}
		return signed(browser, string(wireJSON(t, act)))
	}

	// Direct and ask cards are written by the home (by "system", no env).
	golden(t, "row-msg-confirm-confirmed", MessageRow[ConfirmBody, ActedState]{
		RowHeader: RowHeader{"msg", confirmID, 814}, Pos: 790, Group: group,
		Main: true, By: "system", Kind: "confirm", Timestamp: ts - 500, Mentions: []string{},
		Re: new(id("text")), Body: ConfirmBody{"direct", agent, []string{user}},
		State: ActedState{"confirmed", person, ts, confirmAct("confirm")},
	})
	golden(t, "row-msg-confirm-denied", MessageRow[ConfirmBody, ActedState]{
		RowHeader: RowHeader{"msg", confirmID, 814}, Pos: 790, Group: group,
		Thread: &taskID, Main: true, By: "system", Kind: "confirm", Timestamp: ts - 500, Mentions: []string{},
		Re: new(id("call")), Body: ConfirmBody{"ask", agent, []string{user}},
		State: ActedState{"denied", person, ts, confirmAct("deny")},
	})
	golden(t, "row-msg-confirm-expired", MessageRow[ConfirmBody, TimedState]{
		RowHeader: RowHeader{"msg", confirmID, 814}, Pos: 790, Group: group,
		Main: true, By: "system", Kind: "confirm", Timestamp: ts - 500, Mentions: []string{},
		Re: new(id("brief")), Body: ConfirmBody{"direct", agent, []string{user}},
		State: TimedState{"expired", ts},
	})
}

func nonCard[B any](t *testing.T, m messageExample, body B) {
	t.Helper()
	golden(t, "row-msg-"+m.kind, exampleRow[B, NoCard](t, m, body, nil, 790))
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

// reportRow is the submission of the example task: its text is the account the
// assignee gave submit_task, and its body is empty (contracts/messages.md 2.7).
func reportRow(t *testing.T) MessageRow[EmptyBody, NoCard] {
	t.Helper()
	return exampleRow[EmptyBody, NoCard](t, messageExample{kind: "report", author: agent, thread: &taskID,
		text: "Added the path indicator and its test; go test ./... passed. The work is on branch path-indicator at 9f2c1ab, pushed."},
		EmptyBody{}, nil, 790)
}

func TestNonCardGoldens(t *testing.T) {
	nonCard(t, messageExample{kind: "text", author: agent, text: "The path is direct."}, EmptyBody{})
	nonCard(t, messageExample{kind: "progress", author: agent, thread: &taskID, text: "Added the path indicator."}, EmptyBody{})
	nonCard(t, messageExample{kind: "question", author: agent, text: "Should the path include latency?"}, EmptyBody{})
	nonCard(t, messageExample{kind: "reply", author: reviewer, thread: &taskID, replyTo: new(id("call")), text: "Use the path change event."}, EmptyBody{})
	nonCard(t, messageExample{kind: "task", author: "system"}, TaskBody{taskID})
	nonCard(t, messageExample{kind: "summary", author: secretary, thread: &taskID, main: true, signedBody: EmptyBody{}},
		SummaryBody{taskID, 94, 2, 72})
	golden(t, "row-msg-report", reportRow(t))
	items := []ReviewItem{{"Missing relay path", false, new("path.go"), new(88)}}
	review := ReviewBody{taskID, 12, "Show connection paths", report, 1, 86, 90, false, items}
	nonCard(t, messageExample{kind: "review", author: reviewer, thread: &taskID, signedBody: ReviewSigned{
		ID(review.Task), ID(review.Report), review.Score, review.Items}}, review)
	nonCard(t, messageExample{kind: "handback", author: agent, thread: &taskID, text: "Renamed the indicator in the terminal, as its owner asked."}, EmptyBody{})
	nonCard(t, messageExample{kind: "system", author: "system", thread: &taskID}, TurnInterruptedBody{SystemCode{"turn_interrupted"}, agent, "timeout", &taskID, new("Turn deadline reached.")})
}

// withState is a message example with its state replaced.
func withState(t *testing.T, name string, state any) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	row["state"] = state
	return wireJSON(t, row)
}

// A card's state is never null; it is null for every other kind (contracts/rows.md section 5).
func TestOnlyACardHasAState(t *testing.T) {
	var card MessageRow[ApprovalBody, StatusOnly]
	if err := Decode(withState(t, "row-msg-approval-pending", nil), &card); err != errNull {
		t.Errorf("a pending approval without a state: %v, want %v", err, errNull)
	}
	var text MessageRow[EmptyBody, NoCard]
	if err := Decode(withState(t, "row-msg-text", map[string]any{"status": "pending"}), &text); err != errCard {
		t.Errorf("a text message with a state: %v, want %v", err, errCard)
	}
	// Before its kind is known, a message may hold either.
	for name, state := range map[string]any{"row-msg-approval-pending": nil, "row-msg-text": map[string]any{"status": "pending"}} {
		var row MsgRow
		if err := Decode(withState(t, name, state), &row); err != nil || (row.State == nil) != (state == nil) {
			t.Errorf("%s with state %v: %+v, %v", name, state, row.State, err)
		}
	}
}
