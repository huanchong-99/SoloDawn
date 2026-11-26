//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_review"] = func(sc *godog.ScenarioContext, _ fixtures) { homeReviewSteps(sc) }
}

type homeReviewWorld struct {
	now       time.Time
	state     *home.State
	msgs      map[string]proto.MsgRow
	receipts  map[string]store.Outcome
	members   map[string]string
	hosts     map[string]identity.Key
	ownerKey  identity.Key
	taskID    string
	lastMsg   proto.MsgRow
	lastRes   home.Result
	queuedOut []home.Item
	seqNonce  int
	reports   map[string]string // the name a scenario gives a report, by its message id
}

func (w *homeReviewWorld) Msg(id string) (proto.MsgRow, bool) {
	m, ok := w.msgs[id]
	return m, ok
}

func (w *homeReviewWorld) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (w *homeReviewWorld) TypedNotes(string) []proto.NoteRow         { return nil }
func (w *homeReviewWorld) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (w *homeReviewWorld) Pending() []proto.MsgRow                   { return nil }
func (w *homeReviewWorld) Queued() []store.Item                      { return nil }
func (w *homeReviewWorld) OtherMember(string) bool                   { return false }
func (w *homeReviewWorld) DeviceLog(string) []proto.Envelope         { return nil }

func (w *homeReviewWorld) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := w.receipts[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (w *homeReviewWorld) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	w.receipts[signer+" "+nonce] = result
	return nil
}

func homeReviewSteps(sc *godog.ScenarioContext) {
	w := &homeReviewWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^a review home$`, w.setup)
	sc.Step(`^the (assignee|reviewer) submits report "([^"]*)"$`, w.submitAs)
	sc.Step(`^report "([^"]*)" (?:has reached step "review"|is submitted for review)$`, w.reachReview)
	sc.Step(`^(\d+) minutes pass$`, func(n int) { w.now = w.now.Add(time.Duration(n) * time.Minute) })
	sc.Step(`^"([^"]*)" posts a review of report "([^"]*)" with score (\d+)$`, func(who, r string, score int) error {
		return w.postReview(who, r, score, false)
	})
	sc.Step(`^"([^"]*)" posts a veto review of report "([^"]*)" with score (\d+)$`, func(who, r string, score int) error {
		return w.postReview(who, r, score, true)
	})
	sc.Step(`^(\d+) more reviews fail on new reports$`, w.failMoreReviews)
	sc.Step(`^the thread of the review task ends with system "([^"]*)"$`, w.checkLastSystem)
	sc.Step(`^the secretary posts summary "([^"]*)" for the review task$`, w.postSummary)
	sc.Step(`^the summary appears in the thread and the main line with score (\d+), (\d+) round and (\d+) minutes$`, w.checkSummary)
	sc.Step(`^the review home answers ok$`, func() error { return w.checkAnswer("") })
	sc.Step(`^the review home answers "([^"]*)"$`, w.checkAnswer)
	sc.Step(`^the review home answers review passed (true|false) in round (\d+)$`, w.checkReviewAnswer)
	sc.Step(`^the review task is "([^"]*)" at step "([^"]*)" for report "([^"]*)"$`, w.checkTask)
	sc.Step(`^the latest review row has score (\d+) and ok (true|false)$`, w.checkReviewRow)
	sc.Step(`^the review home has queued "([^"]*)"$`, w.checkQueued)
}

func (w *homeReviewWorld) setup() error {
	browser, node, hostA, hostB := exampleKey("rev browser"), exampleKey("rev node"), exampleKey("rev hostA"), exampleKey("rev hostB")
	w.ownerKey = browser
	w.members = map[string]string{
		"owner":     string(exampleID("rev owner")),
		"secretary": string(exampleID("rev secretary")),
		"assignee":  string(exampleID("rev assignee")),
		"reviewer":  string(exampleID("rev reviewer")),
	}
	w.hosts = map[string]identity.Key{
		"secretary": hostA,
		"assignee":  hostA,
		"reviewer":  hostB,
	}
	group := string(exampleID("rev group"))
	w.state = home.NewState(group, node.ID(), browser.ID())
	log, err := homeLog([]identity.Key{browser, node, hostA, hostB})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: node.ID(),
		Secretary: w.members["secretary"], Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	owner := w.members["owner"]
	w.state.People[owner] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: group, Kind: "human", User: browser.ID()},
		Role:      "owner",
	}
	for _, role := range []string{"secretary", "assignee", "reviewer"} {
		mid := w.members[role]
		host := w.hosts[role]
		w.state.AIs[mid] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: mid}, Group: group, Kind: "ai", User: browser.ID(), Name: role},
			Host:      host.ID(), Workspace: "/work/" + role, Secretary: role == "secretary",
		}
	}
	w.taskID = string(exampleID("rev task"))
	w.state.Tasks[w.taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID, Seq: 5},
		Group:     group, Num: 1, Title: "Verify checks and review",
		Origin: "direct", State: "active", Step: "work",
		Assignee: w.members["assignee"], Reviewer: w.members["reviewer"],
		Pass: 90, Depends: []string{},
		Stats: proto.TaskStats{Started: w.now.UnixMilli() - 1000}, Created: w.now.UnixMilli() - 1000,
	}
	w.state.Seq, w.state.Num = 5, 1
	w.msgs = map[string]proto.MsgRow{}
	w.receipts = map[string]store.Outcome{}
	w.reports = map[string]string{}
	w.queuedOut = nil
	return nil
}

func (w *homeReviewWorld) nextID() proto.ID {
	w.seqNonce++
	return exampleID("rev nonce " + strconv.Itoa(w.seqNonce))
}

func (w *homeReviewWorld) apply(by identity.Key, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	res, err := home.Commands.Apply(w.state, w, w, by.Sign(string(raw)), w.now)
	if err != nil {
		return err
	}
	for _, r := range res.Rows {
		if m, ok := r.Value().(proto.MsgRow); ok {
			w.msgs[m.ID] = m
			if m.Kind == "review" {
				w.lastMsg = m
			}
		}
	}
	w.state.Commit(res)
	w.lastRes = res
	w.queuedOut = append(w.queuedOut, res.Items...)
	return nil
}

// submit posts the assignee's report under the name a scenario gives it.
func (w *homeReviewWorld) submit(name string) error { return w.submitAs("assignee", name) }

func (w *homeReviewWorld) submitAs(role, name string) error {
	w.queuedOut = nil
	tid, id := proto.ID(w.taskID), w.reportID(name)
	w.reports[string(id)] = name
	p := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: id, Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.state.ID), As: proto.ID(w.members[role]),
		},
		Kind: "report", Thread: &tid, Text: "Ready for review.", Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
	return w.apply(w.hosts[role], p)
}

func (w *homeReviewWorld) reportID(name string) proto.ID { return exampleID("rev report " + name) }

func (w *homeReviewWorld) reachReview(name string) error { return w.submit(name) }

func (w *homeReviewWorld) postReview(role, commit string, score int, veto bool) error {
	w.queuedOut = nil
	tid := proto.ID(w.taskID)
	items := []proto.ReviewItem{{Text: "Remark", Veto: veto}}
	p := proto.Post[proto.ReviewSigned]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.state.ID), As: proto.ID(w.members[role]),
		},
		Kind: "review", Thread: &tid, Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: tid, Report: w.reportID(commit), Score: score, Items: items},
	}
	return w.apply(w.hosts[role], p)
}

func (w *homeReviewWorld) failMoreReviews(count int) error {
	for i := range count {
		commit := fmt.Sprintf("r%d", i+3)
		if err := w.reachReview(commit); err != nil {
			return err
		}
		if err := w.postReview("reviewer", commit, 86, false); err != nil {
			return err
		}
		if w.lastRes.Answer.Code != "" {
			return fmt.Errorf("unexpected refusal on round: %+v", w.lastRes.Answer)
		}
	}
	w.queuedOut = nil
	return nil
}

func (w *homeReviewWorld) checkAnswer(code string) error {
	if w.lastRes.Answer.Code != code || w.lastRes.Answer.OK != (code == "") {
		return fmt.Errorf("got answer %+v (%s); want code %q", w.lastRes.Answer, w.lastRes.Msg, code)
	}
	return nil
}

func (w *homeReviewWorld) checkReviewAnswer(passedText string, round int) error {
	wantPassed := passedText == "true"
	ans := w.lastRes.Answer
	if !ans.OK || ans.Passed == nil || *ans.Passed != wantPassed || ans.Round != round {
		return fmt.Errorf("got review answer %+v; want passed=%v round=%d", ans, wantPassed, round)
	}
	return nil
}

func (w *homeReviewWorld) checkTask(state, step, commit string) error {
	t := w.state.Tasks[w.taskID]
	if t.State != state || t.Step != step || t.Report != string(w.reportID(commit)) {
		return fmt.Errorf("task is state=%q step=%q report=%q", t.State, t.Step, w.reports[t.Report])
	}
	return nil
}

func (w *homeReviewWorld) checkLastSystem(code string) error {
	last := w.state.Tasks[w.taskID].Thread.Last
	if last == nil || last.Kind != "system" {
		return fmt.Errorf("the thread ends with %+v", last)
	}
	for _, m := range w.msgs {
		if w.taskSystem(m, last.Timestamp) == code {
			return nil
		}
	}
	return fmt.Errorf("the thread of the task holds no system message %q", code)
}

// taskSystem is the code of a system message in the task's thread written at
// that time, else "".
func (w *homeReviewWorld) taskSystem(m proto.MsgRow, ts int64) string {
	var body proto.SystemCode
	if m.Kind != "system" || m.Thread == nil || *m.Thread != w.taskID || m.Timestamp != ts || json.Unmarshal(m.Body, &body) != nil {
		return ""
	}
	return body.Code
}

func (w *homeReviewWorld) postSummary(text string) error {
	tid := proto.ID(w.taskID)
	p := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.state.ID), As: proto.ID(w.members["secretary"]),
		},
		Kind: "summary", Thread: &tid, Main: true, Text: text, Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
	if err := w.apply(w.hosts["secretary"], p); err != nil {
		return err
	}
	w.lastMsg = w.msgs[string(p.Nonce)]
	return nil
}

func (w *homeReviewWorld) checkSummary(score, rounds, minutes int) error {
	if err := w.checkSummaryRow(proto.SummaryBody{Task: w.taskID, Score: score, Rounds: rounds, Minutes: minutes}); err != nil {
		return err
	}
	last := w.state.Tasks[w.taskID].Thread.Last
	if last == nil || [2]string{last.Kind, last.By} != [2]string{"summary", w.members["secretary"]} {
		return fmt.Errorf("the thread ends with %+v", last)
	}
	return nil
}

// checkSummaryRow checks that the summary is in the task's thread and in the
// main line, with that body.
func (w *homeReviewWorld) checkSummaryRow(want proto.SummaryBody) error {
	var body proto.SummaryBody
	if err := json.Unmarshal(w.lastMsg.Body, &body); err != nil {
		return err
	}
	if body != want || !w.lastMsg.Main || !reflect.DeepEqual(w.lastMsg.Thread, &w.taskID) {
		return fmt.Errorf("summary row main=%v thread=%v body=%+v; want %+v", w.lastMsg.Main, w.lastMsg.Thread, body, want)
	}
	return nil
}

func (w *homeReviewWorld) checkReviewRow(score int, okText string) error {
	var body proto.ReviewBody
	if err := json.Unmarshal(w.lastMsg.Body, &body); err != nil {
		return err
	}
	wantOK := okText == "true"
	if body.Score != score || body.OK != wantOK {
		return fmt.Errorf("review row body has score=%d ok=%v; want score=%d ok=%v", body.Score, body.OK, score, wantOK)
	}
	return nil
}

func (w *homeReviewWorld) checkQueued(spec string) error {
	var got []string
	for _, it := range w.queuedOut {
		parts := []string{it.Why, w.roleName(it.AI)}
		if name := w.reports[it.Msg]; name != "" {
			parts = append(parts, name)
		}
		if it.Code != "" {
			parts = append(parts, it.Code)
		}
		got = append(got, strings.Join(parts, " "))
	}
	want := strings.Split(spec, ", ")
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %v; want %v", got, want)
	}
	w.queuedOut = nil
	return nil
}

func (w *homeReviewWorld) roleName(memberID string) string {
	for role, id := range w.members {
		if id == memberID {
			return role
		}
	}
	return memberID
}
