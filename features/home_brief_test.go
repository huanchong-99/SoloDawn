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
	stepSets["home_brief"] = func(sc *godog.ScenarioContext, _ fixtures) { homeBriefSteps(sc) }
}

// homeBriefWorld is a group home driven through the command table of the
// node, home.Commands, which holds the functions of the brief. What lives on
// disk is a fake: the messages the commands wrote, and the receipts.
type homeBriefWorld struct {
	now      time.Time
	state    *home.State
	disk     homeBriefDisk
	receipts homeBriefReceipts
	members  map[string]string       // member ids by the names of the scenarios
	keys     map[string]identity.Key // by who owns the device
	brief    string                  // the id of the brief the scenario is about
	sent     proto.Envelope          // the last command sent
	answers  []home.Result
	items    []home.Item // every item queued, in order
	nonce    int
}

// homeBriefDisk is a home.Disk over the messages in memory.
type homeBriefDisk map[string]proto.MsgRow

func (d homeBriefDisk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d[id]; return m, ok }
func (d homeBriefDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d homeBriefDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d homeBriefDisk) Pending() []proto.MsgRow                   { return nil }
func (d homeBriefDisk) Queued() []store.Item                      { return nil }

// homeBriefReceipts is a home.Receipts in memory.
type homeBriefReceipts map[string]store.Outcome

func (r homeBriefReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	held, ok := r[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return held, nil
}

func (r homeBriefReceipts) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	r[signer+" "+nonce] = result
	return nil
}

func homeBriefSteps(sc *godog.ScenarioContext) {
	w := &homeBriefWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^a group home with a secretary and three working AIs$`, w.start)
	sc.Step(`^the secretary (?:posts|has posted) the brief$`, func() error { return w.post(nil) })
	sc.Step(`^the secretary posts a brief that replaces it$`, func() error {
		replaced := w.brief
		return w.post(&replaced)
	})
	sc.Step(`^the owner (?:confirms|has confirmed) the brief$`, func() error {
		return w.act(w.keys["the owner"], "confirm", "")
	})
	sc.Step(`^the owner sends the same confirm again$`, func() error { return w.send(w.keys["the owner"], w.sent) })
	sc.Step(`^the owner sends the brief back with the note "([^"]*)"$`, func(note string) error {
		return w.act(w.keys["the owner"], "revise", fmt.Sprintf(`,"text":%q`, note))
	})
	sc.Step(`^(the secretary, for itself|the owner's laptop, for "ai-one"|the owner, with an arg) sends "(confirm|revise)" on the brief$`, w.actAs)
	sc.Step(`^the home answers ok$`, func() error { return w.answered(1, "") })
	sc.Step(`^the home answers ok to both$`, func() error { return w.answered(2, "") })
	sc.Step(`^the home answers "([^"]*)"$`, func(code string) error { return w.answered(1, code) })
	sc.Step(`^the brief is "([^"]*)"$`, func(status string) error { return w.briefIs(status, nil) })
	sc.Step(`^the brief is "([^"]*)" with the note "([^"]*)"$`, func(status, note string) error { return w.briefIs(status, &note) })
	sc.Step(`^the home holds no task and has queued no item$`, func() error {
		if err := w.holds(0); err != nil {
			return err
		}
		return w.queued(nil, proto.Envelope{})
	})
	sc.Step(`^the home holds no task$`, func() error { return w.holds(0) })
	sc.Step(`^the home holds (\d+) tasks$`, w.holds)
	sc.Step(`^task 1 is "active" and task 2 is "queued" until task 1 is merged$`, w.dependent)
	sc.Step(`^the home has queued "([^"]*)", each caused by the (confirm|send-back)$`, func(items, _ string) error {
		return w.queued(strings.Split(items, ", "), w.cause())
	})
	sc.Step(`^the secretary gives task 2 to "([^"]*)" while it is queued$`, w.reassign)
	sc.Step(`^task 2 is "queued" and the home has queued nothing that makes a worktree$`, w.stillQueued)
	sc.Step(`^task 1 is merged$`, w.merge)
	sc.Step(`^task 2 is "active" and the home has queued "([^"]*)", caused by the confirm$`, func(item string) error {
		if task := w.task(2); task.State != "active" || task.Wait != nil {
			return fmt.Errorf("task 2 is %s, waiting for %+v", task.State, task.Wait)
		}
		return w.queued([]string{item}, w.cause())
	})
}

// start makes a group whose owner has a browser, the node that is the home,
// and a laptop that hosts the secretary and the three working AIs.
func (w *homeBriefWorld) start() error {
	browser, node, laptop := exampleKey("brief browser"), exampleKey("brief node"), exampleKey("brief laptop")
	w.keys = map[string]identity.Key{"the owner": browser, "the owner's laptop": laptop}
	w.members = map[string]string{}
	for _, name := range []string{"owner", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("brief " + name))
	}
	group := string(exampleID("brief group"))
	w.state = home.NewState(group, node.ID(), browser.ID())
	log, err := homeLog([]identity.Key{browser, node, laptop})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	// The group, its owner and its AIs, as group create and member add_ai leave them.
	w.state.Group = proto.GroupRow{RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: node.ID(), Secretary: w.members["secretary"],
		Forge: "none", Pass: 90, PassFoundation: 70, Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5}}
	owner := w.members["owner"]
	w.state.People[owner] = proto.HumanMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: group, Kind: "human", User: browser.ID()}, Role: "owner"}
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		id := w.members[name]
		w.state.AIs[id] = proto.AIMemberRow{MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: group, Kind: "ai", User: browser.ID(), Name: name}, Host: laptop.ID()}
	}
	w.state.Seq = 6
	w.disk, w.receipts, w.answers, w.items = homeBriefDisk{}, homeBriefReceipts{}, nil, nil
	return nil
}

// id is a fresh nonce.
func (w *homeBriefWorld) id() proto.ID {
	w.nonce++
	return exampleID("brief nonce " + strconv.Itoa(w.nonce))
}

// post sends the secretary's brief, which replaces another when replaces is set.
func (w *homeBriefWorld) post(replaces *string) error {
	m := w.members
	post := proto.Post[proto.BriefBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.id(), Timestamp: w.now.UnixMilli() - 500, Group: proto.ID(w.state.ID), As: proto.ID(m["secretary"])},
		Kind:        "brief", Mentions: []proto.ID{},
		Body: proto.BriefBody{
			Title: "Show the connection path", Goal: "People see how a peer is connected.",
			Criteria: []string{"A path change shows within 1 s."}, Scope: proto.BriefScope{In: []string{"node events", "web indicator"}, Out: []string{}},
			Tasks: []proto.BriefTask{
				{Title: "Report path changes", Assignee: m["ai-one"], Reviewer: m["ai-two"], Depends: []int{}},
				{Title: "Show the path", Assignee: m["ai-two"], Reviewer: m["ai-one"], Depends: []int{0}},
			},
			Pass: 90, Requester: m["owner"], Replaces: replaces,
		},
	}
	payload, err := json.Marshal(post)
	if err != nil {
		return err
	}
	if replaces == nil {
		w.brief = string(post.Nonce)
	}
	laptop := w.keys["the owner's laptop"]
	return w.send(laptop, laptop.Sign(string(payload)))
}

// act sends an act on the brief, signed by a device; more holds further members.
func (w *homeBriefWorld) act(by identity.Key, action, more string) error {
	payload := fmt.Sprintf(`{"t":"act","n":%q,"ts":%d,"g":%q,"msg":%q,"action":%q%s}`, w.id(), w.now.UnixMilli()-1000, w.state.ID, w.brief, action, more)
	return w.send(by, by.Sign(payload))
}

func (w *homeBriefWorld) actAs(who, action string) error {
	more := map[string]string{
		"the secretary, for itself":        `,"as":"` + w.members["secretary"] + `"`,
		`the owner's laptop, for "ai-one"`: `,"as":"` + w.members["ai-one"] + `"`,
		"the owner, with an arg":           `,"arg":"A"`,
	}[who]
	by := w.keys["the owner"]
	if strings.Contains(more, `"as"`) {
		by = w.keys["the owner's laptop"]
	}
	return w.act(by, action, more)
}

// send applies a command as the run loop of a home does: in a transaction,
// whose messages land on disk, and whose result the state then commits.
func (w *homeBriefWorld) send(by identity.Key, env proto.Envelope) error {
	res, err := home.Commands.Apply(w.state, w.disk, w.receipts, by.ID(), env, w.now)
	if err != nil {
		return err
	}
	for _, row := range res.Rows {
		if msg, ok := row.Value().(proto.MsgRow); ok {
			w.disk[msg.ID] = msg
		}
	}
	w.state.Commit(res)
	w.sent, w.answers, w.items = env, append(w.answers, res), append(w.items, res.Items...)
	return nil
}

// answered checks the last n answers: ok, or refused with that code.
func (w *homeBriefWorld) answered(n int, code string) error {
	if len(w.answers) < n {
		return fmt.Errorf("%d answers", len(w.answers))
	}
	for _, res := range w.answers[len(w.answers)-n:] {
		if res.Answer.Code != code || res.Answer.OK != (code == "") {
			return fmt.Errorf("answered %+v: %s", res.Answer, res.Msg)
		}
	}
	return nil
}

// briefIs checks the state of the brief on disk.
func (w *homeBriefWorld) briefIs(status string, note *string) error {
	card := w.disk[w.brief]
	if card.State == nil || card.State.Status != status || !reflect.DeepEqual(card.State.Note, note) {
		got, _ := json.Marshal(card.State)
		return fmt.Errorf("the brief's state is %s", got)
	}
	return nil
}

func (w *homeBriefWorld) holds(n int) error {
	if len(w.state.Tasks) != n {
		return fmt.Errorf("the home holds %d tasks", len(w.state.Tasks))
	}
	return nil
}

// task is a task of the brief by its entry, counted from 1.
func (w *homeBriefWorld) task(entry int) proto.TaskRow {
	for _, task := range w.state.Tasks {
		if task.Brief == w.brief && task.Num == entry {
			return task
		}
	}
	return proto.TaskRow{}
}

// dependent checks that task 2 depends on task 1 and waits for it.
func (w *homeBriefWorld) dependent() error {
	one, two := w.task(1), w.task(2)
	if one.State != "active" || one.Wait != nil || len(one.Depends) != 0 {
		return fmt.Errorf("task 1 is %s, waits for %+v, depends on %v", one.State, one.Wait, one.Depends)
	}
	if two.State != "queued" || two.Wait == nil || two.Wait.Kind != "depends" || !slices.Equal(two.Depends, []string{one.ID}) {
		return fmt.Errorf("task 2 is %s, waits for %+v, depends on %v", two.State, two.Wait, two.Depends)
	}
	if msg, ok := w.disk[two.ID]; !ok || msg.Kind != "task" || msg.By != "system" {
		return fmt.Errorf("task 2 has no task message: %+v", msg)
	}
	return nil
}

// cause is the act that decided the brief, as its state keeps it.
func (w *homeBriefWorld) cause() proto.Envelope {
	if state := w.disk[w.brief].State; state != nil && state.Act != nil {
		return *state.Act
	}
	return proto.Envelope{}
}

// queued checks the items queued since the last check, written as "why
// member [task n]", and that each has the cause.
func (w *homeBriefWorld) queued(want []string, cause proto.Envelope) error {
	var got []string
	for _, it := range w.items {
		words := []string{it.Why, w.name(it.AI)}
		if it.Task != "" {
			words = append(words, "task", strconv.Itoa(w.state.Tasks[it.Task].Num))
		}
		got = append(got, strings.Join(words, " "))
		if it.Cause == nil || *it.Cause != cause {
			return fmt.Errorf("the item %q has the cause %+v", got[len(got)-1], it.Cause)
		}
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %q", got)
	}
	w.items = nil
	return nil
}

// name is the name of a member in the scenarios.
func (w *homeBriefWorld) name(member string) string {
	for name, id := range w.members {
		if id == member {
			return name
		}
	}
	return member
}

// reassign is task assign by the secretary, through the task machine: the
// command is unit HM-9's, the row it moves is one this unit created.
func (w *homeBriefWorld) reassign(name string) error {
	w.items = nil
	task := w.task(2)
	moved, fx, err := home.Step(task, home.Event{Kind: home.OnAssign, By: home.BySecretary, Member: w.members["secretary"], Now: w.now.UnixMilli(), Assignee: w.members[name]})
	if err != nil {
		return err
	}
	w.state.Tasks[moved.ID] = moved
	for _, it := range fx.Items {
		w.items = append(w.items, home.Item{AI: it.AI, Why: it.Why, Task: moved.ID})
	}
	return nil
}

// stillQueued checks that task 2 is queued and that nothing was queued that
// a host answers with a worktree: no work item at all.
func (w *homeBriefWorld) stillQueued() error {
	if task := w.task(2); task.State != "queued" || task.Assignee != w.members["ai-three"] {
		return fmt.Errorf("task 2 is %s for %s", task.State, w.name(task.Assignee))
	}
	for _, it := range w.items {
		if slices.Contains([]string{"assign", "checks", "reviewed", "review", "resume", "merge"}, it.Why) {
			return fmt.Errorf("queued %+v", it)
		}
	}
	return nil
}

// merge brings task 1 to its merge, as rows 3 to 7 would, and reports it
// merged, as unit HM-12 will: row 8 for task 1, then the release of each
// queued task that depends on it, with Blocked while another dependency is
// not merged. The cause of an assign item of a release is the act that
// confirmed the brief, which the brief's state keeps.
func (w *homeBriefWorld) merge() error {
	one := w.task(1)
	one.Step, one.Commit = "merge", "c1"
	merged, _, err := home.Step(one, home.Event{Kind: home.OnMerged, By: home.ByHost, Now: w.now.UnixMilli(), Commit: "c1"})
	if err != nil {
		return err
	}
	w.state.Tasks[merged.ID] = merged
	w.items = nil
	for _, task := range w.state.Tasks {
		if task.State != "queued" || !slices.Contains(task.Depends, merged.ID) {
			continue
		}
		blocked := slices.ContainsFunc(task.Depends, func(dep string) bool { return w.state.Tasks[dep].State != "merged" })
		released, fx, err := home.Step(task, home.Event{Kind: home.OnReleased, By: home.ByHome, Now: w.now.UnixMilli(), Blocked: blocked})
		if err != nil {
			return err
		}
		w.state.Tasks[released.ID] = released
		cause := w.cause()
		for _, it := range fx.Items {
			w.items = append(w.items, home.Item{AI: it.AI, Why: it.Why, Task: released.ID, Msg: released.ID, Cause: &cause})
		}
	}
	return nil
}
