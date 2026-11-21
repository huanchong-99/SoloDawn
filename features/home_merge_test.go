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
	stepSets["home_merge"] = func(sc *godog.ScenarioContext, _ fixtures) { homeMergeSteps(sc) }
}

type homeMergeWorld struct {
	now         time.Time
	releaseTime int64
	state       *home.State
	table       home.Table
	msgs        map[string]proto.MsgRow
	order       []string
	receipts    map[string]store.Outcome
	members     map[string]string
	hosts       map[string]identity.Key
	ownerKey    identity.Key
	briefID     string
	taskID      string
	items       []home.Item
	lastSummary proto.MsgRow
	seqNonce    int
}

func (w *homeMergeWorld) Msg(id string) (proto.MsgRow, bool) {
	m, ok := w.msgs[id]
	return m, ok
}

func (*homeMergeWorld) Note(string) (proto.NoteRow, bool) { return proto.NoteRow{}, false }

func (w *homeMergeWorld) Thread(task *string, before int64, limit int) []proto.MsgRow {
	var out []proto.MsgRow
	for i := len(w.order) - 1; i >= 0 && len(out) < limit; i-- {
		m := w.msgs[w.order[i]]
		if before > 0 && m.Pos >= before {
			continue
		}
		if homeMergeSameThread(m.Thread, task) {
			out = append(out, m)
		}
	}
	return out
}

func (w *homeMergeWorld) Pending() []proto.MsgRow {
	var out []proto.MsgRow
	for _, id := range w.order {
		m := w.msgs[id]
		if m.State != nil && m.State.Status == "pending" {
			out = append(out, m)
		}
	}
	return out
}

func (*homeMergeWorld) Queued() []store.Item { return nil }

func (w *homeMergeWorld) Receipt(signer, nonce string) (store.Outcome, error) {
	out, ok := w.receipts[signer+" "+nonce]
	if !ok {
		return store.Outcome{}, sql.ErrNoRows
	}
	return out, nil
}

func (w *homeMergeWorld) PutReceipt(signer, nonce string, _ int64, result store.Outcome) error {
	w.receipts[signer+" "+nonce] = result
	return nil
}

func homeMergeSameThread(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func homeMergeSteps(sc *godog.ScenarioContext) {
	w := &homeMergeWorld{now: time.UnixMilli(1763600000000)}
	sc.Step(`^a merge home with a GitHub remote and an active task at step "review"$`, func() error {
		return w.initSingleTask("github", "review", 28)
	})
	sc.Step(`^a merge home without a remote and an active task at step "merge" for commit "([^"]*)"$`, func(commit string) error {
		if err := w.initSingleTask("none", "merge", 0); err != nil {
			return err
		}
		t := w.state.Tasks[w.taskID]
		t.Commit = commit
		t.Review = &proto.Review{Round: 1, Score: 93, OK: true, By: w.members["reviewer"], Commit: commit, At: w.now.UnixMilli() - 1000}
		w.state.Tasks[w.taskID] = t
		return nil
	})
	sc.Step(`^a merge home where confirming a two-task brief started task 1 and queued task 2$`, w.initTwoTaskBrief)
	sc.Step(`^the reviewer passes commit "([^"]*)" with score (\d+) on pull request (\d+)$`, func(commit string, score, pr int) error {
		t := w.state.Tasks[w.taskID]
		t.Commit, t.PR, t.PRURL = commit, pr, fmt.Sprintf("https://git.example/pull/%d", pr)
		w.state.Tasks[w.taskID] = t
		return w.sendReview(w.taskID, commit, score)
	})
	sc.Step(`^the merge home queues "([^"]*)" and no session turn item$`, w.assertMergeItemOnly)
	sc.Step(`^the assignee's host reports commit "([^"]*)" merged as "([^"]*)" with (\d+) added and (\d+) removed$`, func(commit, mergeCommit string, added, removed int) error {
		return w.sendMerged(w.taskID, commit, mergeCommit, added, removed)
	})
	sc.Step(`^"([^"]*)"'s host reports commit "([^"]*)" merged as "([^"]*)" with (\d+) added and (\d+) removed$`, func(_, commit, mergeCommit string, added, removed int) error {
		return w.sendMerged(w.taskID, commit, mergeCommit, added, removed)
	})
	sc.Step(`^the task is "([^"]*)" and its thread ends with system "([^"]*)"$`, w.assertMergedAndSystem)
	sc.Step(`^the merge home queues "([^"]*)"$`, w.assertQueued)
	sc.Step(`^the secretary posts summary "([^"]*)" for the task$`, func(text string) error {
		return w.sendSummary(w.taskID, text)
	})
	sc.Step(`^the secretary posts summary "([^"]*)" for task (\d+)$`, func(text string, num int) error {
		return w.sendSummary(w.taskByNum(num).ID, text)
	})
	sc.Step(`^the summary appears in the thread and the main line with score (\d+), (\d+) round, (\d+) minutes, (\d+) added, (\d+) removed and pull request (\d+)$`, w.assertFullSummary)
	sc.Step(`^the assignee's host reports a merge conflict on commit "([^"]*)" with detail "([^"]*)"$`, w.sendConflict)
	sc.Step(`^the task is "([^"]*)" at step "([^"]*)" waiting on "([^"]*)"$`, w.assertStateStepWait)
	sc.Step(`^the thread ends with a failed "([^"]*)" checks message carrying that detail$`, w.assertConflictChecks)
	sc.Step(`^the secretary assigns the conflicted task to "([^"]*)"$`, w.sendAssign)
	sc.Step(`^"([^"]*)"'s host checks and submits commit "([^"]*)" and the reviewer passes it with score (\d+)$`, w.checkSubmitAndReview)
	sc.Step(`^(\d+) minutes pass and task (\d+) reaches step "merge" and merges commit "([^"]*)" as "([^"]*)"$`, w.advanceAndMergeTask)
	sc.Step(`^task 2 becomes "active" at step "branch" with stats\.started at the release time$`, w.assertTask2Released)
	sc.Step(`^the merge home queues "([^"]*)" with task 2 caused by the confirm$`, w.assertQueuedWithCause)
	sc.Step(`^task 2's summary records (\d+) minutes of active work$`, w.assertSummaryMinutes)
}

func (w *homeMergeWorld) initBase(forge string) error {
	browser, node, hostA, hostB := exampleKey("mrg browser"), exampleKey("mrg node"), exampleKey("mrg hostA"), exampleKey("mrg hostB")
	w.ownerKey = browser
	w.members = map[string]string{
		"owner":     string(exampleID("mrg owner")),
		"secretary": string(exampleID("mrg secretary")),
		"assignee":  string(exampleID("mrg assignee")),
		"reviewer":  string(exampleID("mrg reviewer")),
		"ai-two":    string(exampleID("mrg ai-two")),
		"ai-three":  string(exampleID("mrg ai-three")),
	}
	w.hosts = map[string]identity.Key{
		"secretary": hostA,
		"assignee":  hostA,
		"reviewer":  hostB,
		"ai-two":    hostB,
		"ai-three":  hostA,
	}
	group := string(exampleID("mrg group"))
	w.state = home.NewState(group, node.ID(), browser.ID())
	log, err := homeLog([]identity.Key{browser, node, hostA, hostB})
	if err != nil {
		return err
	}
	w.state.Logs[log.User] = log
	w.state.Group = proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: group}, Home: node.ID(),
		Forge: forge, Secretary: w.members["secretary"], Pass: 90, PassFoundation: 70,
		Limits: proto.Limits{Rounds: 10, Failures: 3, ReviewRounds: 5},
	}
	owner := w.members["owner"]
	w.state.People[owner] = proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: owner}, Group: group, Kind: "human", User: browser.ID()},
		Role:      "owner",
	}
	for _, role := range []string{"secretary", "assignee", "reviewer", "ai-two", "ai-three"} {
		mid := w.members[role]
		host := w.hosts[role]
		w.state.AIs[mid] = proto.AIMemberRow{
			MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: mid}, Group: group, Kind: "ai", User: browser.ID(), Name: role},
			Host:      host.ID(), Secretary: role == "secretary",
		}
	}
	w.state.Seq = 10
	w.msgs = map[string]proto.MsgRow{}
	w.order = nil
	w.receipts = map[string]store.Outcome{}
	w.items = nil
	w.table = homeMergeBuildTable()
	return nil
}

// homeMergeBuildTable clones home.Commands and attaches minimal stand-ins only
// when sibling units (HM-9, HM-11) have not yet merged into home.Commands.
func homeMergeBuildTable() home.Table {
	tbl := maps.Clone(home.Commands)
	if tbl["post/review"] == nil {
		tbl["post/review"] = homeMergeStandInReview
	}
	if tbl["task/checked"] == nil {
		tbl["task/checked"] = homeMergeStandInChecked
	}
	if tbl["task/submitted"] == nil {
		tbl["task/submitted"] = homeMergeStandInSubmitted
	}
	if tbl["task/assign"] == nil {
		tbl["task/assign"] = homeMergeStandInAssign
	}
	return tbl
}

func homeMergeStandInReview(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.Post[proto.ReviewSigned]
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	task := s.Tasks[string(p.Body.Task)]
	ok := p.Body.Score >= task.Pass
	moved, fx, err := home.Step(task, home.Event{
		Kind: home.OnReview, By: home.ByAI, Member: c.Who.AI, Now: c.Now,
		Limits: s.Group.Limits, Remote: s.Group.Forge != "none", Commit: p.Body.Commit, OK: ok, Score: p.Body.Score,
	})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "review refused"}
	}
	tid := moved.ID
	body, _ := json.Marshal(proto.ReviewBody{
		Task: moved.ID, Num: moved.Num, Title: moved.Title, Commit: p.Body.Commit,
		Round: moved.Review.Round, Score: p.Body.Score, Pass: moved.Pass, OK: ok, Items: p.Body.Items,
	})
	msg := proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(c.Nonce)}, Group: s.ID, Thread: &tid,
		By: c.Who.Member(), Kind: "review", Timestamp: c.Timestamp, Text: p.Text, Mentions: []string{}, Body: body, Env: &c.Env,
	}
	moved.Thread.Count++
	moved.Thread.Last = &proto.ThreadMessage{By: msg.By, Kind: "review", Text: p.Text, Timestamp: c.Timestamp}
	var items []home.Item
	for _, raw := range fx.Items {
		it := home.Item{AI: raw.AI, Why: raw.Why, Task: moved.ID, Commit: raw.Commit}
		if raw.Why == "merge" && s.Group.Forge != "none" {
			it.PR = moved.PR
		}
		items = append(items, it)
	}
	return home.Result{Rows: []home.Row{home.RowOf(msg), home.RowOf(moved)}, Items: items, Answer: store.Outcome{Passed: &ok, Round: moved.Review.Round}}, nil
}

func homeMergeStandInChecked(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.TaskChecked
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	moved, _, err := home.Step(s.Tasks[string(p.ID)], home.Event{Kind: home.OnChecked, By: home.ByHost, Now: c.Now, Commit: p.Commit, OK: p.OK})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "checked refused"}
	}
	return home.Result{Rows: []home.Row{home.RowOf(moved)}}, nil
}

func homeMergeStandInSubmitted(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.TaskSubmitted
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	moved, fx, err := home.Step(s.Tasks[string(p.ID)], home.Event{
		Kind: home.OnSubmitted, By: home.ByHost, Now: c.Now, Commit: p.Commit, Remote: s.Group.Forge != "none",
	})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "submitted refused"}
	}
	moved.PR, moved.PRURL = p.PR, p.PRURL
	moved.Stats.Added, moved.Stats.Removed = p.Stats.Added, p.Stats.Removed
	var items []home.Item
	for _, raw := range fx.Items {
		items = append(items, home.Item{AI: raw.AI, Why: raw.Why, Task: moved.ID, Commit: raw.Commit})
	}
	return home.Result{Rows: []home.Row{home.RowOf(moved)}, Items: items}, nil
}

func homeMergeStandInAssign(s *home.State, c home.Command) (home.Result, *home.Refusal) {
	var p proto.TaskAssign
	if refusal := c.Decode(&p); refusal != nil {
		return home.Result{}, refusal
	}
	moved, fx, err := home.Step(s.Tasks[string(p.ID)], home.Event{
		Kind: home.OnAssign, By: home.BySecretary, Member: c.Who.AI, Now: c.Now,
		Assignee: string(p.Assignee), Reviewer: string(p.Reviewer),
	})
	if err != nil {
		return home.Result{}, &home.Refusal{Code: err.Error(), Msg: "assign refused"}
	}
	feedback := homeMergeFeedbackID(c.Disk, moved.ID)
	var items []home.Item
	for _, raw := range fx.Items {
		items = append(items, home.Item{AI: raw.AI, Why: raw.Why, Task: moved.ID, Msg: feedback})
	}
	return home.Result{Rows: []home.Row{home.RowOf(moved)}, Items: items}, nil
}

func homeMergeFeedbackID(d home.Disk, taskID string) string {
	for _, m := range d.Thread(&taskID, 0, 50) {
		if m.Kind == "checks" || m.Kind == "review" {
			return m.ID
		}
	}
	return ""
}

func (w *homeMergeWorld) initSingleTask(forge, step string, pr int) error {
	if err := w.initBase(forge); err != nil {
		return err
	}
	w.taskID = string(exampleID("mrg single task"))
	started := w.now.UnixMilli() - 72*60_000
	w.state.Tasks[w.taskID] = proto.TaskRow{
		RowHeader: proto.RowHeader{Type: "task", ID: w.taskID, Seq: 10},
		Group:     w.state.ID, Num: 1, Title: "Show connection path", Slug: "show-connection-path",
		Origin: "direct", State: "active", Step: step, Commit: "c1",
		Assignee: w.members["assignee"], Reviewer: w.members["reviewer"],
		Pass: 90, Depends: []string{}, Branch: "task/1-show-connection-path", PR: pr,
		Stats: proto.TaskStats{Started: started}, Created: started,
	}
	w.state.Num = 1
	return nil
}

func (w *homeMergeWorld) initTwoTaskBrief() error {
	if err := w.initBase("none"); err != nil {
		return err
	}
	post := proto.Post[proto.BriefBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli() - 1000,
			Group: proto.ID(w.state.ID), As: proto.ID(w.members["secretary"]),
		},
		Kind: "brief", Mentions: []proto.ID{},
		Body: proto.BriefBody{
			Title: "Two-step change", Goal: "Ship in order.", Criteria: []string{"Both merged."},
			Scope: proto.BriefScope{In: []string{"home"}, Out: []string{}},
			Tasks: []proto.BriefTask{
				{Title: "First step", Assignee: w.members["assignee"], Reviewer: w.members["reviewer"], Depends: []int{}},
				{Title: "Second step", Assignee: w.members["ai-two"], Reviewer: w.members["assignee"], Depends: []int{0}},
			},
			Pass: 90, Requester: w.members["owner"],
		},
	}
	w.briefID = string(post.Nonce)
	if err := w.apply(w.hosts["secretary"], post); err != nil {
		return err
	}
	act := proto.Act{
		HomeCommand: proto.HomeCommand{
			Type: "act", Nonce: w.nextID(), Timestamp: w.now.UnixMilli() - 500, Group: proto.ID(w.state.ID),
		},
		Msg: proto.ID(w.briefID), Action: "confirm",
	}
	if err := w.apply(w.ownerKey, act); err != nil {
		return err
	}
	w.items = nil
	return nil
}

func (w *homeMergeWorld) nextID() proto.ID {
	w.seqNonce++
	return exampleID("mrg nonce " + strconv.Itoa(w.seqNonce))
}

func (w *homeMergeWorld) apply(by identity.Key, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	res, err := w.table.Apply(w.state, w, w, by.ID(), by.Sign(string(raw)), w.now)
	if err != nil {
		return err
	}
	if res.Answer.Code != "" {
		return fmt.Errorf("command refused with %s: %s", res.Answer.Code, res.Msg)
	}
	w.recordRows(res.Rows)
	w.state.Commit(res)
	w.items = append(w.items, res.Items...)
	return nil
}

func (w *homeMergeWorld) recordRows(rows []home.Row) {
	for _, r := range rows {
		m, ok := r.Value().(proto.MsgRow)
		if !ok {
			continue
		}
		if _, seen := w.msgs[m.ID]; !seen {
			w.order = append(w.order, m.ID)
		}
		w.msgs[m.ID] = m
		if m.Kind == "summary" {
			w.lastSummary = m
		}
	}
}

func (w *homeMergeWorld) sendReview(taskID, commit string, score int) error {
	w.items = nil
	task := w.state.Tasks[taskID]
	tid := proto.ID(taskID)
	p := proto.Post[proto.ReviewSigned]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.state.ID), As: proto.ID(task.Reviewer),
		},
		Kind: "review", Thread: &tid, Mentions: []proto.ID{},
		Body: proto.ReviewSigned{Task: tid, Commit: commit, Score: score, Items: []proto.ReviewItem{{Text: "Ready.", Veto: false}}},
	}
	return w.apply(w.hostOf(task.Reviewer), p)
}

func (w *homeMergeWorld) sendMerged(taskID, commit, mergeCommit string, added, removed int) error {
	w.items = nil
	task := w.state.Tasks[taskID]
	p := proto.TaskMerged{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "merged", ID: proto.ID(taskID),
		},
		Commit: commit, MergeCommit: mergeCommit, Stats: proto.DiffStats{Added: added, Removed: removed},
	}
	return w.apply(w.hostOf(task.Assignee), p)
}

func (w *homeMergeWorld) sendConflict(commit, detail string) error {
	w.items = nil
	task := w.state.Tasks[w.taskID]
	p := proto.TaskConflict{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "conflict", ID: proto.ID(w.taskID),
		},
		Commit: commit, Detail: detail,
	}
	return w.apply(w.hostOf(task.Assignee), p)
}

func (w *homeMergeWorld) sendSummary(taskID, text string) error {
	w.items = nil
	tid := proto.ID(taskID)
	p := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type: "post", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
			Group: proto.ID(w.state.ID), As: proto.ID(w.members["secretary"]),
		},
		Kind: "summary", Thread: &tid, Main: true, Text: text, Mentions: []proto.ID{}, Body: proto.EmptyBody{},
	}
	return w.apply(w.hosts["secretary"], p)
}

func (w *homeMergeWorld) sendAssign(role string) error {
	w.items = nil
	p := proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(),
				Group: proto.ID(w.state.ID), As: proto.ID(w.members["secretary"]),
			},
			Op: "assign", ID: proto.ID(w.taskID),
		},
		Assignee: proto.ID(w.members[role]),
	}
	return w.apply(w.hosts["secretary"], p)
}

func (w *homeMergeWorld) checkSubmitAndReview(role, commit string, score int) error {
	host := w.hosts[role]
	checked := proto.TaskChecked{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "checked", ID: proto.ID(w.taskID),
		},
		OK: true, Commit: commit, Items: []proto.CheckItem{{Name: "go test ./...", OK: true}},
	}
	if err := w.apply(host, checked); err != nil {
		return err
	}
	submitted := proto.TaskSubmitted{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.nextID(), Timestamp: w.now.UnixMilli(), Group: proto.ID(w.state.ID)},
			Op:          "submitted", ID: proto.ID(w.taskID),
		},
		Commit: commit, Stats: proto.DiffStats{Added: 40, Removed: 6},
	}
	if err := w.apply(host, submitted); err != nil {
		return err
	}
	return w.sendReview(w.taskID, commit, score)
}

func (w *homeMergeWorld) advanceAndMergeTask(minutes, num int, commit, mergeCommit string) error {
	w.now = w.now.Add(time.Duration(minutes) * time.Minute)
	if num == 1 {
		w.releaseTime = w.now.UnixMilli()
	}
	t := w.taskByNum(num)
	t.Step = "merge"
	t.Commit = commit
	t.Review = &proto.Review{Round: 1, Score: 95, OK: true, By: t.Reviewer, Commit: commit, At: w.now.UnixMilli() - 1000}
	w.state.Tasks[t.ID] = t
	return w.sendMerged(t.ID, commit, mergeCommit, 25, 3)
}

func (w *homeMergeWorld) assertMergeItemOnly(want string) error {
	for _, it := range w.items {
		if it.Why != "merge" {
			return fmt.Errorf("unexpected non-merge item queued: %+v", it)
		}
	}
	return w.assertQueued(want)
}

func (w *homeMergeWorld) assertMergedAndSystem(state, code string) error {
	t := w.state.Tasks[w.taskID]
	if !homeMergeTaskFinal(t, state) {
		return fmt.Errorf("task state=%q wait=%+v ended=%v", t.State, t.Wait, t.Stats.Ended)
	}
	last := w.msgs[w.order[len(w.order)-1]]
	var body proto.TaskMergedBody
	if err := json.Unmarshal(last.Body, &body); err != nil {
		return err
	}
	if !homeMergeSystemRowOK(last, body, code) || t.Thread.Last == nil || t.Thread.Last.Kind != "system" {
		return fmt.Errorf("thread ended with %+v (body %+v)", last, body)
	}
	return nil
}

func homeMergeTaskFinal(t proto.TaskRow, state string) bool {
	return t.State == state && t.Wait == nil && t.Stats.Ended != nil
}

func homeMergeSystemRowOK(last proto.MsgRow, body proto.TaskMergedBody, code string) bool {
	return last.Kind == "system" && last.By == "system" && body.Code == code
}

func (w *homeMergeWorld) assertFullSummary(score, rounds, minutes, added, removed, pr int) error {
	var body proto.SummaryBody
	if err := json.Unmarshal(w.lastSummary.Body, &body); err != nil {
		return err
	}
	want := proto.SummaryBody{
		Task: w.taskID, Score: score, Rounds: rounds, Minutes: minutes,
		Added: added, Removed: removed, PR: &pr,
	}
	if !homeMergeSummaryRowOK(w.lastSummary, w.taskID) || !reflect.DeepEqual(body, want) {
		return fmt.Errorf("summary row main=%v thread=%v body=%+v; want %+v", w.lastSummary.Main, w.lastSummary.Thread, body, want)
	}
	if !homeMergeSummaryLastOK(w.state.Tasks[w.taskID], w.members["secretary"]) {
		return fmt.Errorf("task thread last=%+v", w.state.Tasks[w.taskID].Thread.Last)
	}
	return nil
}

func homeMergeSummaryRowOK(m proto.MsgRow, taskID string) bool {
	return m.Main && m.Thread != nil && *m.Thread == taskID
}

func homeMergeSummaryLastOK(t proto.TaskRow, secretary string) bool {
	return t.Thread.Last != nil && t.Thread.Last.Kind == "summary" && t.Thread.Last.By == secretary
}

func (w *homeMergeWorld) assertStateStepWait(state, step, wait string) error {
	t := w.state.Tasks[w.taskID]
	if t.State != state || t.Step != step || t.Wait == nil || t.Wait.Kind != wait {
		return fmt.Errorf("task state=%q step=%q wait=%+v", t.State, t.Step, t.Wait)
	}
	return nil
}

func (w *homeMergeWorld) assertConflictChecks(stage string) error {
	last := w.msgs[w.order[len(w.order)-1]]
	var body proto.ChecksBody
	if err := json.Unmarshal(last.Body, &body); err != nil {
		return err
	}
	if !homeMergeChecksHeaderOK(last, body, stage) || !homeMergeChecksItemOK(body.Items) {
		return fmt.Errorf("conflict checks row=%+v body=%+v", last, body)
	}
	return nil
}

func homeMergeChecksHeaderOK(m proto.MsgRow, body proto.ChecksBody, stage string) bool {
	return m.Kind == "checks" && body.Stage == stage && !body.OK
}

func homeMergeChecksItemOK(items []proto.CheckItem) bool {
	return len(items) == 1 && items[0].Detail != nil && *items[0].Detail != ""
}

func (w *homeMergeWorld) assertTask2Released() error {
	t2 := w.taskByNum(2)
	if t2.State != "active" || t2.Step != "branch" || t2.Wait != nil || t2.Stats.Started != w.releaseTime {
		return fmt.Errorf("task 2 state=%q step=%q wait=%+v started=%d (want %d)", t2.State, t2.Step, t2.Wait, t2.Stats.Started, w.releaseTime)
	}
	return nil
}

func (w *homeMergeWorld) assertQueuedWithCause(spec string) error {
	brief := w.msgs[w.briefID]
	if brief.State == nil || brief.State.Act == nil {
		return fmt.Errorf("brief %s has no confirmation act", w.briefID)
	}
	t2 := w.taskByNum(2)
	it, ok := w.findAssign(t2.ID)
	if !ok || !homeMergeCauseMatches(it, t2.ID, *brief.State.Act) {
		return fmt.Errorf("assign item=%+v; want msg=%q cause=%+v", it, t2.ID, brief.State.Act)
	}
	return w.assertQueued(spec)
}

func (w *homeMergeWorld) findAssign(taskID string) (home.Item, bool) {
	for _, it := range w.items {
		if it.Why == "assign" && it.Task == taskID {
			return it, true
		}
	}
	return home.Item{}, false
}

func homeMergeCauseMatches(it home.Item, msg string, act proto.Envelope) bool {
	return it.Msg == msg && it.Cause != nil && *it.Cause == act
}

func (w *homeMergeWorld) assertSummaryMinutes(want int) error {
	var body proto.SummaryBody
	if err := json.Unmarshal(w.lastSummary.Body, &body); err != nil {
		return err
	}
	if body.Minutes != want {
		return fmt.Errorf("summary minutes=%d; want %d", body.Minutes, want)
	}
	return nil
}

func (w *homeMergeWorld) assertQueued(spec string) error {
	got := make([]string, len(w.items))
	for i, it := range w.items {
		got[i] = w.formatQueuedItem(it)
	}
	want := strings.Split(spec, ", ")
	if !slices.Equal(got, want) {
		return fmt.Errorf("queued %v; want %v", got, want)
	}
	w.items = nil
	return nil
}

func (w *homeMergeWorld) formatQueuedItem(it home.Item) string {
	parts := []string{it.Why, w.roleOf(it.AI)}
	if it.Why == "assign" && it.Task != "" {
		parts = append(parts, fmt.Sprintf("task %d", w.state.Tasks[it.Task].Num))
	}
	return strings.Join(homeMergeAppendTail(parts, it), " ")
}

func homeMergeAppendTail(parts []string, it home.Item) []string {
	if it.Commit != "" {
		parts = append(parts, it.Commit)
	}
	if it.PR > 0 {
		parts = append(parts, fmt.Sprintf("pr=%d", it.PR))
	}
	if it.Code != "" {
		parts = append(parts, it.Code)
	}
	return parts
}

func (w *homeMergeWorld) taskByNum(num int) proto.TaskRow {
	for _, t := range w.state.Tasks {
		if t.Num == num {
			return t
		}
	}
	return proto.TaskRow{}
}

func (w *homeMergeWorld) hostOf(memberID string) identity.Key {
	deviceID := w.state.AIs[memberID].Host
	for _, k := range w.hosts {
		if k.ID() == deviceID {
			return k
		}
	}
	return w.ownerKey
}

func (w *homeMergeWorld) roleOf(memberID string) string {
	for role, id := range w.members {
		if id == memberID {
			return role
		}
	}
	return memberID
}
