//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["home_group"] = func(sc *godog.ScenarioContext, _ fixtures) { homeGroupSteps(sc) }
}

type homeGroupStore struct {
	msgs     map[string]proto.MsgRow
	queued   []store.Item
	outcomes map[string]store.Outcome
}

func newHomeGroupStore() *homeGroupStore {
	return &homeGroupStore{msgs: map[string]proto.MsgRow{}, outcomes: map[string]store.Outcome{}}
}

func (s *homeGroupStore) Msg(id string) (proto.MsgRow, bool)        { m, ok := s.msgs[id]; return m, ok }
func (s *homeGroupStore) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (s *homeGroupStore) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (s *homeGroupStore) Pending() []proto.MsgRow                   { return nil }
func (s *homeGroupStore) Queued() []store.Item                      { return s.queued }

func (s *homeGroupStore) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := s.outcomes[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (s *homeGroupStore) PutReceipt(signer, nonce string, _ int64, out store.Outcome) error {
	s.outcomes[signer+" "+nonce] = out
	return nil
}

type homeGroupWorld struct {
	now     time.Time
	state   *home.State
	store   *homeGroupStore
	browser identity.Key
	laptop  identity.Key
	guest   identity.Key
	aiIDs   map[string]string
	taskID  string
	last    home.Result
	nonce   int
}

func homeGroupSteps(sc *godog.ScenarioContext) {
	w := &homeGroupWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^an uncreated group home on the owner's node$`, w.initEmpty)
	sc.Step(`^a created group "([^"]*)" with owner "([^"]*)"$`, w.initCreated)
	sc.Step(`^the owner creates project group "([^"]*)" as member "([^"]*)"$`, w.createGroup)
	sc.Step(`^the owner renames the group to "([^"]*)"$`, w.renameGroup)
	sc.Step(`^the owner's laptop (?:adds|has added) AI "([^"]*)" on "([^"]*)" as (secretary|worker)$`, w.addAI)
	sc.Step(`^the owner sets the group secretary to "([^"]*)"$`, w.setSecretary)
	sc.Step(`^inbox item (\d+) is queued for "([^"]*)"$`, w.queueItem)
	sc.Step(`^the owner removes AI "([^"]*)"$`, w.removeAI)
	sc.Step(`^"([^"]*)" is the assignee of an "([^"]*)" task$`, w.addTask)
	sc.Step(`^that task becomes "([^"]*)"$`, w.setTaskState)
	sc.Step(`^(the owner sends group create with g not n|the owner sends group set with no fields|the owner sends member set with no fields|a non-admin member sends group set)$`, w.invalidAttempt)
	sc.Step(`^the group command answers ok$`, w.answerOK)
	sc.Step(`^the group command answers ok and marks groups changed$`, w.answerGroupsChanged)
	sc.Step(`^the group command answers "([^"]*)"$`, w.answerCode)
	sc.Step(`^the group row is "([^"]*)" with pass (\d+) and foundation pass (\d+) and owner "([^"]*)"$`, w.checkCreated)
	sc.Step(`^the group row is named "([^"]*)"$`, w.checkGroupName)
	sc.Step(`^the group secretary is "([^"]*)" and "([^"]*)" is a working AI$`, w.checkRoles)
	sc.Step(`^the group has no secretary and holds a "([^"]*)" system message$`, w.checkNoSecretary)
	sc.Step(`^AI "([^"]*)" is gone and inbox item (\d+) is retired$`, w.checkGone)
}

func (w *homeGroupWorld) nextID(tag string) proto.ID {
	w.nonce++
	return exampleID("home_group " + tag + " " + strconv.Itoa(w.nonce))
}

func (w *homeGroupWorld) initEmpty() error {
	node := exampleKey("hg node")
	w.browser, w.laptop, w.guest = exampleKey("hg browser"), exampleKey("hg laptop"), exampleKey("hg guest")
	group := string(exampleID("hg group"))
	w.state = home.NewState(group, node.ID(), w.browser.ID())
	ownerLog, err := homeLog([]identity.Key{w.browser, node, w.laptop})
	if err != nil {
		return err
	}
	guestLog, err := homeLog([]identity.Key{w.guest})
	if err != nil {
		return err
	}
	w.state.Logs[ownerLog.User] = ownerLog
	w.state.Logs[guestLog.User] = guestLog
	w.store, w.aiIDs = newHomeGroupStore(), map[string]string{}
	return nil
}

func (w *homeGroupWorld) initCreated(name, member string) error {
	if err := w.initEmpty(); err != nil {
		return err
	}
	if err := w.createGroup(name, member); err != nil {
		return err
	}
	guestID := string(exampleID("hg member guest"))
	w.state.People[guestID] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: guestID}, Group: w.state.ID, Kind: "human", User: w.guest.ID(), Name: "guest"},
		Role:      "member", Online: true,
	}
	return w.answerOK()
}

func (w *homeGroupWorld) apply(by identity.Key, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return w.applyRaw(by, string(raw))
}

func (w *homeGroupWorld) applyRaw(by identity.Key, payload string) error {
	res, err := home.Commands.Apply(w.state, w.store, w.store, by.ID(), by.Sign(payload), w.now)
	if err != nil {
		return err
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			w.store.msgs[msg.ID] = msg
		}
	}
	w.state.Commit(res)
	w.last = res
	return nil
}

func (w *homeGroupWorld) createGroup(name, member string) error {
	g := proto.ID(w.state.ID)
	return w.apply(w.browser, proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: g, Timestamp: w.now.UnixMilli(), Group: g},
			Op:          "create",
		},
		Name: name, Kind: "project", Repo: "https://git.example/team/repo.git",
		Base: "main", Forge: "github", Checks: []string{"go test ./..."}, MemberName: member,
	})
}

func (w *homeGroupWorld) renameGroup(name string) error {
	return w.apply(w.browser, proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.nextID("set"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "set",
		},
		Name: &name,
	})
}

func (w *homeGroupWorld) addAI(name, cli, role string) error {
	id, ok := w.aiIDs[name]
	if !ok {
		id = string(exampleID("hg ai " + name))
		w.aiIDs[name] = id
	}
	return w.apply(w.laptop, proto.MemberAddAI{
		MemberCommand: proto.MemberCommand{
			HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("add"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "add_ai",
			ID:          proto.ID(id),
		},
		CLI: cli, Name: name, Account: "personal", Secretary: role == "secretary", Approvers: []string{w.browser.ID()},
	})
}

func (w *homeGroupWorld) setSecretary(name string) error {
	return w.apply(w.browser, proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.nextID("sec"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "set",
		},
		Secretary: proto.ID(w.aiIDs[name]),
	})
}

func (w *homeGroupWorld) queueItem(id int, name string) {
	w.store.queued = append(w.store.queued, store.Item{ID: int64(id), AI: w.aiIDs[name], Why: "notice"})
}

func (w *homeGroupWorld) removeAI(name string) error {
	return w.apply(w.browser, proto.MemberCommand{
		HomeCommand: proto.HomeCommand{Type: "member", Nonce: w.nextID("rm"), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
		Op:          "remove",
		ID:          proto.ID(w.aiIDs[name]),
	})
}

func (w *homeGroupWorld) addTask(name, state string) {
	w.taskID = string(exampleID("hg task"))
	w.state.Tasks[w.taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID}, State: state, Assignee: w.aiIDs[name],
	}
}

func (w *homeGroupWorld) setTaskState(state string) {
	task := w.state.Tasks[w.taskID]
	task.State = state
	w.state.Tasks[w.taskID] = task
}

func (w *homeGroupWorld) invalidAttempt(attempt string) error {
	g, ts, n := w.state.ID, w.now.UnixMilli(), w.nextID("bad")
	switch attempt {
	case "the owner sends group create with g not n":
		p := proto.GroupCreate{
			GroupCommand: proto.GroupCommand{HomeCommand: proto.HomeCommand{Type: "group", Nonce: n, Timestamp: ts, Group: proto.ID(g)}, Op: "create"},
			Name:         "Other", Kind: "project", Repo: "https://git.example/r.git", Base: "main", Forge: "none", Checks: []string{}, MemberName: "alex",
		}
		return w.apply(w.browser, p)
	case "the owner sends group set with no fields":
		return w.applyRaw(w.browser, fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":"set"}`, n, ts, g))
	case "the owner sends member set with no fields":
		return w.applyRaw(w.browser, fmt.Sprintf(`{"t":"member","n":%q,"ts":%d,"g":%q,"op":"set","id":%q}`, n, ts, g, exampleID("hg any")))
	default:
		name := "Blocked"
		return w.apply(w.guest, proto.GroupSet{
			GroupCommand: proto.GroupCommand{HomeCommand: proto.HomeCommand{Type: "group", Nonce: n, Timestamp: ts, Group: proto.ID(g)}, Op: "set"},
			Name:         &name,
		})
	}
}

func (w *homeGroupWorld) answerOK() error {
	if !w.last.Answer.OK || w.last.Answer.Code != "" {
		return fmt.Errorf("answered %+v (%s)", w.last.Answer, w.last.Msg)
	}
	return nil
}

func (w *homeGroupWorld) answerGroupsChanged() error {
	if !w.last.GroupsChanged {
		return fmt.Errorf("GroupsChanged is false")
	}
	return w.answerOK()
}

func (w *homeGroupWorld) answerCode(code string) error {
	if w.last.Answer.Code != code || len(w.last.Rows) != 0 {
		return fmt.Errorf("answered %+v with %d rows, want %s", w.last.Answer, len(w.last.Rows), code)
	}
	return nil
}

func (w *homeGroupWorld) checkCreated(name string, pass, found int, owner string) error {
	g := w.state.Group
	if g.Name != name || g.Pass != pass || g.PassFoundation != found {
		return fmt.Errorf("group row is %+v", g)
	}
	if !w.hasOwner(owner) {
		return fmt.Errorf("owner %q not found in %+v", owner, w.state.People)
	}
	return nil
}

func (w *homeGroupWorld) hasOwner(owner string) bool {
	for _, m := range w.state.People {
		if m.Role == "owner" && m.Name == owner {
			return true
		}
	}
	return false
}

func (w *homeGroupWorld) checkGroupName(name string) error {
	if w.state.Group.Name != name {
		return fmt.Errorf("group name is %q, want %q", w.state.Group.Name, name)
	}
	return nil
}

func (w *homeGroupWorld) checkRoles(sec, worker string) error {
	secRow, workRow := w.state.AIs[w.aiIDs[sec]], w.state.AIs[w.aiIDs[worker]]
	if w.state.Group.Secretary != secRow.ID || !secRow.Secretary || workRow.Secretary {
		return fmt.Errorf("group secretary=%q, sec=%+v, worker=%+v", w.state.Group.Secretary, secRow, workRow)
	}
	return nil
}

func (w *homeGroupWorld) checkNoSecretary(code string) error {
	if w.state.Group.Secretary != "" {
		return fmt.Errorf("secretary is still %q", w.state.Group.Secretary)
	}
	want := `{"code":"` + code + `"}`
	for _, m := range w.store.msgs {
		if m.Kind == "system" && string(m.Body) == want {
			return nil
		}
	}
	return fmt.Errorf("system message %s not found in %+v", want, w.store.msgs)
}

func (w *homeGroupWorld) checkGone(name string, retired int) error {
	if _, ok := w.state.AIs[w.aiIDs[name]]; ok {
		return fmt.Errorf("AI %q is still in state", name)
	}
	var want []int64
	if retired != 0 {
		want = []int64{int64(retired)}
	}
	if !slices.Equal(w.last.Retire, want) {
		return fmt.Errorf("retired %v, want %v", w.last.Retire, want)
	}
	return nil
}
