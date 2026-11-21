//go:build acceptance

package features

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
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
	stepSets["home_pause"] = func(sc *godog.ScenarioContext, _ fixtures) { homePauseSteps(sc) }
}

type homePauseWorld struct {
	now      time.Time
	state    *home.State
	table    home.Table
	disk     *homePauseDisk
	receipts homePauseReceipts
	members  map[string]string
	tasks    map[string]string
	keys     map[string]identity.Key
	last     home.Result
	items    []home.Item
	nonce    int
}

type homePauseDisk struct {
	msgs   map[string]proto.MsgRow
	queued []store.Item
}

func (d *homePauseDisk) Msg(id string) (proto.MsgRow, bool)        { m, ok := d.msgs[id]; return m, ok }
func (d *homePauseDisk) Note(string) (proto.NoteRow, bool)         { return proto.NoteRow{}, false }
func (d *homePauseDisk) Thread(*string, int64, int) []proto.MsgRow { return nil }
func (d *homePauseDisk) Pending() []proto.MsgRow                   { return nil }
func (d *homePauseDisk) Queued() []store.Item                      { return d.queued }

type homePauseReceipts map[string]store.Outcome

func (r homePauseReceipts) Receipt(signer, nonce string) (store.Outcome, error) {
	held, ok := r[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return held, nil
}

func (r homePauseReceipts) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	r[signer+" "+nonce] = result
	return nil
}

func homePauseSteps(sc *godog.ScenarioContext) {
	w := &homePauseWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^a group for pause and resume with an owner, an admin, a member and three working AIs$`, w.start)
	sc.Step(`^"([^"]+)" has paused the group$`, func(who string) error {
		if err := w.sendGroup(who, "pause"); err != nil {
			return err
		}
		w.items = nil
		return w.answered("ok")
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)"$`, func(name, step, ai string) {
		w.putTask(name, "active", step, ai, "", "", nil)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)" waiting for an approval$`, func(name, step, ai string) {
		w.putTask(name, "active", step, ai, "", "", &proto.TaskWait{Kind: "approval", Since: w.now.UnixMilli() - 3000})
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "([^"]+)" assigned to "([^"]+)" for commit "([^"]+)"$`, func(name, step, ai, commit string) {
		w.putTask(name, "active", step, ai, "", commit, nil)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)"$`, func(name, ai, reviewer, commit string) {
		w.putTask(name, "active", "review", ai, reviewer, commit, nil)
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)" with its review already queued$`, func(name, ai, reviewer, commit string) {
		id := w.putTask(name, "active", "review", ai, reviewer, commit, nil)
		rev := w.members[reviewer]
		w.disk.queued = append(w.disk.queued, store.Item{Why: "review", AI: rev, Task: &id, Commit: &commit})
	})
	sc.Step(`^an active pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)" waiting for an approval$`, func(name, ai, reviewer, commit string) {
		w.putTask(name, "active", "review", ai, reviewer, commit, &proto.TaskWait{Kind: "approval", Since: w.now.UnixMilli() - 3000})
	})
	sc.Step(`^a paused pause-test task "([^"]+)" at step "review" assigned to "([^"]+)" and reviewed by "([^"]+)" for commit "([^"]+)"$`, func(name, ai, reviewer, commit string) {
		w.putTask(name, "paused", "review", ai, reviewer, commit, &proto.TaskWait{Kind: "manual", Since: w.now.UnixMilli() - 3000})
	})
	sc.Step(`^"([^"]+)" sends group "(pause|resume)"$`, w.sendGroup)
	sc.Step(`^"([^"]+)" posts a text mentioning "([^"]+)" while the group is paused$`, w.postMention)
	sc.Step(`^the pause home answers "([^"]+)"$`, w.answered)
	sc.Step(`^the pause home answers "([^"]+)" with no rows and no items$`, w.answeredIdle)
	sc.Step(`^the group is paused by "([^"]+)" with system message "([^"]+)"$`, w.pausedBy)
	sc.Step(`^the group is unpaused with system message "([^"]+)" by "([^"]+)"$`, w.unpausedBy)
	sc.Step(`^the pause home has queued "([^"]*)"$`, w.checkQueued)
}

func (w *homePauseWorld) start() error {
	ownerDev := exampleKey("pause owner")
	nodeDev := exampleKey("pause node")
	laptopDev := exampleKey("pause laptop")
	adminDev := exampleKey("pause admin")
	memberDev := exampleKey("pause member")
	w.keys = map[string]identity.Key{
		"the owner":  ownerDev,
		"the admin":  adminDev,
		"the member": memberDev,
		"laptop":     laptopDev,
	}
	w.members, w.tasks = map[string]string{}, map[string]string{}
	for _, name := range []string{"owner", "admin", "member", "secretary", "ai-one", "ai-two", "ai-three"} {
		w.members[name] = string(exampleID("pause " + name))
	}
	group := string(exampleID("pause group"))
	w.state = home.NewState(group, nodeDev.ID(), ownerDev.ID())
	for _, devs := range [][]identity.Key{{ownerDev, nodeDev, laptopDev}, {adminDev}, {memberDev}} {
		log, err := homeLog(devs)
		if err != nil {
			return err
		}
		w.state.Logs[log.User] = log
	}
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: nodeDev.ID(),
		Secretary: w.members["secretary"], Forge: "none", Pass: 90, PassFoundation: 70,
	}
	w.addHuman("owner", ownerDev, "owner")
	w.addHuman("admin", adminDev, "admin")
	w.addHuman("member", memberDev, "member")
	for _, name := range []string{"secretary", "ai-one", "ai-two", "ai-three"} {
		id := w.members[name]
		w.state.AIs[id] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: group, Kind: "ai", User: ownerDev.ID(), Name: name},
			Host:      laptopDev.ID(), Secretary: name == "secretary",
		}
	}
	w.state.Seq = 8
	w.table = maps.Clone(home.Commands)
	if w.table["post/text"] == nil {
		w.table["post/text"] = homePausePostText
	}
	w.disk = &homePauseDisk{msgs: map[string]proto.MsgRow{}}
	w.receipts, w.items, w.nonce = homePauseReceipts{}, nil, 0
	return nil
}

func (w *homePauseWorld) addHuman(name string, dev identity.Key, role string) {
	id := w.members[name]
	w.state.People[id] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: id}, Group: w.state.ID, Kind: "human", User: dev.ID(), Name: name},
		Role:      role,
	}
}

func (w *homePauseWorld) nextID() proto.ID {
	w.nonce++
	return exampleID("pause nonce " + strconv.Itoa(w.nonce))
}

func (w *homePauseWorld) putTask(name, state, step, ai, reviewer, commit string, wait *proto.TaskWait) string {
	num := len(w.tasks) + 1
	id := w.orderedTaskID(name)
	w.tasks[name] = id
	revID := ""
	if reviewer != "" {
		revID = w.members[reviewer]
	}
	w.state.Tasks[id] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: id}, Group: w.state.ID, Num: num,
		Title: name, Slug: proto.MakeSlug(name), State: state, Step: step,
		Assignee: w.members[ai], Reviewer: revID, Commit: commit, Wait: wait,
	}
	return id
}

func (w *homePauseWorld) orderedTaskID(name string) string {
	// Prefix with padded name characters in ASCII base64url so sorted ids follow name order.
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		return '-'
	}, name)
	for len(clean) < 21 {
		clean += "a"
	}
	return clean[:21] + "A"
}

func (w *homePauseWorld) sendGroup(who, op string) error {
	key, more := w.signer(who)
	payload := fmt.Sprintf(`{"t":"group","n":%q,"ts":%d,"g":%q,"op":%q%s}`, w.nextID(), w.now.UnixMilli()-500, w.state.ID, op, more)
	return w.apply(key, key.Sign(payload))
}

func (w *homePauseWorld) signer(who string) (identity.Key, string) {
	if key, ok := w.keys[who]; ok {
		return key, ""
	}
	ai := strings.TrimPrefix(who, "the ")
	return w.keys["laptop"], fmt.Sprintf(`,"as":%q`, w.members[ai])
}

func (w *homePauseWorld) postMention(who, ai string) error {
	key := w.keys[who]
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli() - 400, Group: proto.ID(w.state.ID)},
		Kind:        "text", Text: "Hold until resume.", Mentions: []proto.ID{proto.ID(w.members[ai])}, Body: proto.EmptyBody{},
	}
	raw, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return w.apply(key, key.Sign(string(raw)))
}

func (w *homePauseWorld) apply(by identity.Key, env proto.Envelope) error {
	res, err := w.table.Apply(w.state, w.disk, w.receipts, by.ID(), env, w.now)
	if err != nil {
		return err
	}
	for _, r := range res.Rows {
		if msg, ok := r.Value().(proto.MsgRow); ok {
			w.disk.msgs[msg.ID] = msg
		}
	}
	w.state.Commit(res)
	w.last = res
	w.items = append(w.items, res.Items...)
	return nil
}

func (w *homePauseWorld) answered(want string) error {
	if want == "ok" {
		if !w.last.Answer.OK || w.last.Answer.Code != "" {
			return fmt.Errorf("answered %+v (%s)", w.last.Answer, w.last.Msg)
		}
		return nil
	}
	if w.last.Answer.OK || w.last.Answer.Code != want {
		return fmt.Errorf("answered %+v (%s); want %s", w.last.Answer, w.last.Msg, want)
	}
	return nil
}

func (w *homePauseWorld) answeredIdle(want string) error {
	if err := w.answered(want); err != nil {
		return err
	}
	if len(w.last.Rows) != 0 || len(w.last.Items) != 0 {
		return fmt.Errorf("wrote %d rows and %d items", len(w.last.Rows), len(w.last.Items))
	}
	return nil
}

func (w *homePauseWorld) pausedBy(who, code string) error {
	by := w.members[who]
	if w.state.Group.Paused == nil || w.state.Group.Paused.By != by || w.state.Group.Paused.At != w.now.UnixMilli() {
		return fmt.Errorf("group.paused is %+v", w.state.Group.Paused)
	}
	return w.systemMsgIs(code, by)
}

func (w *homePauseWorld) unpausedBy(code, who string) error {
	if w.state.Group.Paused != nil {
		return fmt.Errorf("group.paused is %+v", w.state.Group.Paused)
	}
	return w.systemMsgIs(code, w.members[who])
}

func (w *homePauseWorld) systemMsgIs(code, by string) error {
	if len(w.last.Rows) != 2 {
		return fmt.Errorf("expected 2 rows, got %d", len(w.last.Rows))
	}
	msg, ok := w.last.Rows[1].Value().(proto.MsgRow)
	want := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: msg.ID, Seq: msg.Seq}, Pos: msg.Seq, Group: w.state.ID,
		By: "system", Kind: "system", Timestamp: w.now.UnixMilli(), Mentions: []string{},
		Body: json.RawMessage(fmt.Sprintf(`{"code":%q,"by":%q}`, code, by)),
	}
	if !ok || !reflect.DeepEqual(msg, want) {
		return fmt.Errorf("got message %+v; want %+v", msg, want)
	}
	return nil
}

func (w *homePauseWorld) checkQueued(spec string) error {
	var got []string
	for _, it := range w.items {
		parts := []string{it.Why, w.memberName(it.AI)}
		if it.Task != "" {
			parts = append(parts, "task", w.taskName(it.Task))
		}
		if it.Commit != "" {
			parts = append(parts, it.Commit)
		}
		got = append(got, strings.Join(parts, " "))
	}
	want := strings.Split(spec, ", ")
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %q; want %q", got, want)
	}
	w.items = nil
	return nil
}

func (w *homePauseWorld) memberName(id string) string {
	for name, mid := range w.members {
		if mid == id {
			return name
		}
	}
	return id
}

func (w *homePauseWorld) taskName(id string) string {
	for name, tid := range w.tasks {
		if tid == id {
			return name
		}
	}
	return id
}

// homePausePostText stands in for post/text until HM-6 merges into rebuild.
func homePausePostText(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.Post[proto.EmptyBody]
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	mentions := make([]string, len(p.Mentions))
	var items []home.Item
	for i, id := range p.Mentions {
		mentions[i] = string(id)
		items = append(items, home.Item{AI: string(id), Why: "mention", Msg: string(c.Nonce)})
	}
	row := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, By: c.Who.Member(),
		Kind: "text", Timestamp: c.Timestamp, Text: p.Text, Mentions: mentions, Body: json.RawMessage("{}"), Env: &c.Env,
	}
	return home.Result{Rows: []home.Row{home.RowOf(row)}, Items: items}, nil
}
