//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_hostev"] = func(sc *godog.ScenarioContext, _ fixtures) { homeHostEvSteps(sc) }
}

type homeHostEvWorld struct {
	now         time.Time
	state       *home.State
	disk        homeHostEvDisk
	receipts    homeHostEvReceipts
	keys        map[string]identity.Key
	members     map[string]string
	taskID      string
	lastMsgID   string
	lastRes     home.Result
	items       []home.Item
	savedSince  int64
	laptopConns int
	nonce       int
}

type homeHostEvDisk map[string]proto.MsgRow

func (d homeHostEvDisk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d[id]; return m, ok }
func (d homeHostEvDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d homeHostEvDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d homeHostEvDisk) Pending() []proto.MsgRow                   { return nil }
func (d homeHostEvDisk) Queued() []store.Item                      { return nil }

type homeHostEvReceipts map[string]store.Outcome

func (r homeHostEvReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	held, ok := r[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return held, nil
}

func (r homeHostEvReceipts) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	r[signer+" "+nonce] = result
	return nil
}

func homeHostEvSteps(sc *godog.ScenarioContext) {
	w := &homeHostEvWorld{}
	sc.Step(`^a group home for host events with (a secretary|no secretary), "codex" on the laptop, "claude" on the home node, and task 1$`, w.setup)
	sc.Step(`^the laptop (?:reports|has reported) "codex" as "([^"]*)" on task 1 over "([^"]*)" with (\d+) turns today$`, func(state, path string, turns int) error {
		w.savedSince = w.state.AIs[w.members["codex"]].Since
		extra := fmt.Sprintf(`,"task":%q,"usage":{"turns_today":%d}`, w.taskID, turns)
		return w.sendStatus(w.keys["laptop"], w.members["codex"], state, path, extra)
	})
	sc.Step(`^time advances and the laptop reports "codex" as "([^"]*)" on task 1 over "([^"]*)" with (\d+) turns today$`, func(state, path string, turns int) error {
		w.now = w.now.Add(5 * time.Second)
		w.savedSince = w.state.AIs[w.members["codex"]].Since
		extra := fmt.Sprintf(`,"task":%q,"usage":{"turns_today":%d}`, w.taskID, turns)
		return w.sendStatus(w.keys["laptop"], w.members["codex"], state, path, extra)
	})
	sc.Step(`^time advances and the laptop reports "codex" as "([^"]*)" with event "turn_interrupted" for reason "([^"]*)" on task 1$`, func(state, reason string) error {
		w.now = w.now.Add(5 * time.Second)
		extra := fmt.Sprintf(`,"event":{"code":"turn_interrupted","reason":%q,"task":%q}`, reason, w.taskID)
		return w.sendStatus(w.keys["laptop"], w.members["codex"], state, "direct", extra)
	})
	sc.Step(`^the laptop reports "codex" as "([^"]*)" with host event "([^"]*)" on "(task 1|main)"$`, w.sendEventCase)
	sc.Step(`^the home answers ok to the status$`, w.checkOK)
	sc.Step(`^"codex" is "([^"]*)" on task 1 over "([^"]*)" with (\d+) turns today$`, func(state, path string, turns int) error {
		return w.checkAI("codex", state, w.taskID, path, turns)
	})
	sc.Step(`^"codex" is "([^"]*)" on task 1 over "([^"]*)" with (\d+) turns today and kept its state time on the path change$`, func(state, path string, turns int) error {
		if err := w.checkAI("codex", state, w.taskID, path, turns); err != nil {
			return err
		}
		if got := w.state.AIs[w.members["codex"]].Since; got != w.savedSince {
			return fmt.Errorf("since changed to %d; want %d", got, w.savedSince)
		}
		return nil
	})
	sc.Step(`^"codex" is "([^"]*)" on no task$`, func(state string) error {
		ai := w.state.AIs[w.members["codex"]]
		if ai.State != state || ai.Task != "" {
			return fmt.Errorf("codex is %s on task %q", ai.State, ai.Task)
		}
		return nil
	})
	sc.Step(`^"(codex|claude)" is still "([^"]*)" on task 1$`, func(name, state string) error {
		ai := w.state.AIs[w.members[name]]
		if ai.State != state || ai.Task != w.taskID {
			return fmt.Errorf("%s is %s on task %q", name, ai.State, ai.Task)
		}
		return nil
	})
	sc.Step(`^task 1 has (\d+) thread message, a "system" message "([^"]*)" for "codex"$`, w.checkThreadEvent)
	sc.Step(`^no inbox item was queued by the status$`, func() error { return w.checkNotice("none") })
	sc.Step(`^the "([^"]*)" system message for "codex" is in "(task 1|main)"$`, w.checkEventPlacement)
	sc.Step(`^the status queued "([^"]*)"$`, w.checkNotice)
	sc.Step(`^the laptop has (\d+) open connections to the home$`, func(n int) {
		w.laptopConns = n
		w.state.Online[w.keys["laptop"].ID()] = n > 0
	})
	sc.Step(`^(?:one|the last) laptop connection closes$`, w.closeLaptopConn)
	sc.Step(`^(the owner's browser reports "codex" as "working"|the laptop, signed as "codex", reports "codex" as "working"|the laptop reports the human owner as "working"|the laptop reports an unknown member as "working"|the laptop reports "codex" as "working" on an unknown task|the laptop reports "codex" with an unknown state|the laptop reports "codex" with an unknown host event code)$`, w.sendBadCase)
	sc.Step(`^the status is refused with "([^"]*)"$`, w.checkRefused)
}

func (w *homeHostEvWorld) setup(secMode string) error {
	w.now = time.UnixMilli(1763600000000)
	browser, node, laptop := exampleKey("hev browser"), exampleKey("hev node"), exampleKey("hev laptop")
	w.keys = map[string]identity.Key{"browser": browser, "node": node, "laptop": laptop}
	w.members = map[string]string{
		"owner":     string(exampleID("hev owner")),
		"secretary": string(exampleID("hev secretary")),
		"codex":     string(exampleID("hev codex")),
		"claude":    string(exampleID("hev claude")),
	}
	w.taskID = string(exampleID("hev task 1"))
	group := string(exampleID("hev group"))
	w.state = home.NewState(group, node.ID(), browser.ID())
	log, err := homeLog([]identity.Key{browser, node, laptop})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	secID := w.members["secretary"]
	if secMode == "no secretary" {
		secID = ""
	}
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: node.ID(), Secretary: secID, Forge: "none",
	}
	owner := w.members["owner"]
	w.state.People[owner] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: group, Kind: "human", User: browser.ID()},
		Role:      "owner",
	}
	w.state.AIs[w.members["secretary"]] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["secretary"]}, Group: group, Kind: "ai", User: browser.ID(), Name: "secretary"},
		Host:      laptop.ID(), State: "dormant", Since: w.now.UnixMilli() - 10000, Path: "direct",
	}
	w.state.AIs[w.members["codex"]] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["codex"]}, Group: group, Kind: "ai", User: browser.ID(), Name: "codex"},
		Host:      laptop.ID(), State: "dormant", Since: w.now.UnixMilli() - 10000, Path: "direct",
	}
	w.state.AIs[w.members["claude"]] = proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.members["claude"]}, Group: group, Kind: "ai", User: browser.ID(), Name: "claude"},
		Host:      node.ID(), State: "working", Task: w.taskID, Since: w.now.UnixMilli() - 5000, Path: "local",
	}
	w.state.Tasks[w.taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID}, Group: group, Num: 1,
		Title: "Handle host events", Slug: "handle-host-events", Origin: "brief",
		State: "active", Step: "local_check", Assignee: w.members["codex"], Reviewer: w.members["claude"], Depends: []string{},
	}
	w.state.Seq = 10
	w.disk, w.receipts, w.items = homeHostEvDisk{}, homeHostEvReceipts{}, nil
	return nil
}

func (w *homeHostEvWorld) nextID() proto.ID {
	w.nonce++
	return exampleID("hev nonce " + strconv.Itoa(w.nonce))
}

func (w *homeHostEvWorld) sendStatus(by identity.Key, member, state, path, extra string) error {
	id := w.nextID()
	w.lastMsgID = string(id)
	payload := fmt.Sprintf(
		`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"status","id":%q,"state":%q,"path":%q,"usage":{}%s}`,
		id, w.now.UnixMilli(), w.state.ID, member, state, path, extra,
	)
	res, err := home.Commands.Apply(w.state, w.disk, w.receipts, by.ID(), by.Sign(payload), w.now)
	if err != nil {
		return err
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			w.disk[msg.ID] = msg
		}
	}
	w.state.Commit(res)
	w.lastRes = res
	w.items = res.Items
	return nil
}

func (w *homeHostEvWorld) sendEventCase(state, code, where string) error {
	taskPart := ""
	if where == "task 1" {
		taskPart = fmt.Sprintf(`,"task":%q`, w.taskID)
	}
	events := map[string]string{
		"turn_interrupted": fmt.Sprintf(`{"code":"turn_interrupted","reason":"timeout"%s,"detail":"Deadline reached."}`, taskPart),
		"cap_reached":      fmt.Sprintf(`{"code":"cap_reached","cap":"day"%s}`, taskPart),
		"rate_limited":     `{"code":"rate_limited","resets":1763603600000}`,
		"login_invalid":    `{"code":"login_invalid","detail":"Signed out."}`,
		"session_replaced": fmt.Sprintf(`{"code":"session_replaced"%s}`, taskPart),
	}
	return w.sendStatus(w.keys["laptop"], w.members["codex"], state, "direct", `,"event":`+events[code])
}

func (w *homeHostEvWorld) sendBadCase(attempt string) error {
	laptop := w.keys["laptop"]
	codex := w.members["codex"]
	cases := map[string]func() error{
		`the owner's browser reports "codex" as "working"`:            func() error { return w.sendStatus(w.keys["browser"], codex, "working", "direct", "") },
		`the laptop, signed as "codex", reports "codex" as "working"`: func() error { return w.sendStatus(laptop, codex, "working", "direct", fmt.Sprintf(`,"as":%q`, codex)) },
		`the laptop reports the human owner as "working"`:             func() error { return w.sendStatus(laptop, w.members["owner"], "working", "direct", "") },
		`the laptop reports an unknown member as "working"`:           func() error { return w.sendStatus(laptop, string(exampleID("hev nobody")), "working", "direct", "") },
		`the laptop reports "codex" as "working" on an unknown task`: func() error {
			return w.sendStatus(laptop, codex, "working", "direct", fmt.Sprintf(`,"task":%q`, exampleID("hev missing task")))
		},
		`the laptop reports "codex" with an unknown state`: func() error { return w.sendStatus(laptop, codex, "busy", "direct", "") },
		`the laptop reports "codex" with an unknown host event code`: func() error {
			return w.sendStatus(laptop, codex, "limited", "direct", `,"event":{"code":"no_secretary"}`)
		},
	}
	return cases[attempt]()
}

func (w *homeHostEvWorld) checkOK() error {
	if !w.lastRes.Answer.OK || w.lastRes.Answer.Code != "" {
		return fmt.Errorf("answered %+v: %s", w.lastRes.Answer, w.lastRes.Msg)
	}
	return nil
}

func (w *homeHostEvWorld) checkRefused(code string) error {
	if w.lastRes.Answer.Code != code || len(w.lastRes.Rows) != 0 {
		return fmt.Errorf("answered %+v (%s) with %d rows; want %s", w.lastRes.Answer, w.lastRes.Msg, len(w.lastRes.Rows), code)
	}
	return nil
}

func (w *homeHostEvWorld) checkAI(name, state, task, path string, turns int) error {
	ai := w.state.AIs[w.members[name]]
	if ai.State != state || ai.Task != task || ai.Path != path || ai.Usage.TurnsToday == nil || *ai.Usage.TurnsToday != turns {
		return fmt.Errorf("got ai %+v", ai)
	}
	return nil
}

func (w *homeHostEvWorld) checkThreadEvent(count int, code string) error {
	task := w.state.Tasks[w.taskID]
	if task.Thread.Count != count || task.Thread.Last == nil || task.Thread.Last.Kind != "system" || task.Thread.Last.By != "system" {
		return fmt.Errorf("task thread summary is %+v", task.Thread)
	}
	return w.checkEventPlacement(code, "task 1")
}

func (w *homeHostEvWorld) checkEventPlacement(code, where string) error {
	msg, ok := w.disk[w.lastMsgID]
	if !ok || msg.Kind != "system" || msg.By != "system" {
		return fmt.Errorf("missing system message: %+v", msg)
	}
	if err := w.checkEventBody(msg.Body, code); err != nil {
		return err
	}
	return w.checkMsgWhere(msg.Thread, where)
}

func (w *homeHostEvWorld) checkEventBody(raw json.RawMessage, code string) error {
	var body struct {
		Code string `json:"code"`
		AI   string `json:"ai"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if body.Code != code || body.AI != w.members["codex"] {
		return fmt.Errorf("body is %s", raw)
	}
	return nil
}

func (w *homeHostEvWorld) checkMsgWhere(thread *string, where string) error {
	got := ""
	if thread != nil {
		got = *thread
	}
	want := ""
	if where == "task 1" {
		want = w.taskID
	}
	if got != want {
		return fmt.Errorf("expected thread %q, got %q", want, got)
	}
	return nil
}

func (w *homeHostEvWorld) checkNotice(want string) error {
	if want == "none" {
		return w.checkNoItems()
	}
	if len(w.items) != 1 {
		return fmt.Errorf("queued %+v; want %s", w.items, want)
	}
	return w.checkOneNotice(w.items[0], want)
}

func (w *homeHostEvWorld) checkNoItems() error {
	if len(w.items) != 0 {
		return fmt.Errorf("queued %+v; want none", w.items)
	}
	return nil
}

func (w *homeHostEvWorld) checkOneNotice(it home.Item, want string) error {
	got := it.Why + " " + it.Code
	if got != want || it.AI != w.state.Group.Secretary || it.Msg != w.lastMsgID {
		return fmt.Errorf("queued item %+v; want %s for secretary on msg %s", it, want, w.lastMsgID)
	}
	return nil
}

func (w *homeHostEvWorld) closeLaptopConn() error {
	w.laptopConns--
	device := w.keys["laptop"].ID()
	if w.laptopConns <= 0 {
		delete(w.state.Online, device)
	}
	w.now = w.now.Add(5 * time.Second)
	res := w.state.Seal(home.Events[home.HostOfflineEvent](w.state, w.disk, w.now.UnixMilli(), device))
	w.state.Commit(res)
	return nil
}
