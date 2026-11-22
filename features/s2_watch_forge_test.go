//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	s2ForgeTitle   = "A title with a \"quote\"; $(touch marker)"
	s2ForgeSummary = "Checked and ready"
	s2ForgeURL     = "https://forge.example.invalid/requests/"
)

type s2ForgeTemplates struct {
	Push, Open, Checks, Merge []string
}

type s2ForgeConfig struct {
	Repo, Base string
	Templates  s2ForgeTemplates
}

type s2ForgeWorld struct {
	*s2WatchWorld
	cfg            s2ForgeConfig
	group          proto.ID
	sec            proto.AIMemberRow
	workers        [2]proto.AIMemberRow
	tasks          map[int]proto.TaskRow
	summaries      map[string]proto.SummaryBody
	short, live    bool
	conflictCommit string
}

func s2RegisterForge(sc *godog.ScenarioContext, w *s2WatchWorld) {
	f := &s2ForgeWorld{s2WatchWorld: w}
	sc.Step("^a project uses the fake forge templates$", f.fakeProject)
	sc.Step("^a signed-in non-GitHub forge and its scratch repository$", f.liveProject)
	sc.Step("^the secretary proposes two tasks for cross-review and P confirms the brief$", f.confirmTwo)
	sc.Step("^the stale reviewed head is refused and the secretary reassigns the task$", f.waitConflict)
	sc.Step("^both tasks merge after checks and cross-review and receive summaries$", f.waitTwo)
	sc.Step("^the forge records the filled template arguments and gh never runs$", f.verifyRecorded)
	sc.Step("^the project refuses a push template containing the title place$", f.rejectPushTitle)
	sc.Step("^P confirms a brief with a title containing spaces, quotes and shell punctuation$", f.confirmTitle)
	sc.Step("^the forge receives the title as one argument and the tag as a literal without creating a marker$", f.verifyTitle)
	sc.Step("^both tasks land on the scratch remote and the forge reports both requests$", f.checkRemote)
}

func s2FakeConfig(c *s2Cast) s2ForgeConfig {
	return s2ForgeConfig{Repo: c.BareDir, Base: "main", Templates: s2ForgeTemplates{
		Push:   []string{s2ForgeProg, "watch-push", "{branch}", "{base}", "{commit}"},
		Open:   []string{s2ForgeProg, "watch-open", "{branch}", "{base}", "{commit}", "--title={title}", "--summary={summary}", "{tag}"},
		Checks: []string{s2ForgeProg, "watch-checks", "{branch}", "{base}", "{commit}", "{request}"},
		Merge:  []string{s2ForgeProg, "watch-merge", "{branch}", "{base}", "{commit}", "{request}"},
	}}
}

func (f *s2ForgeWorld) fakeProject() error {
	if err := f.start(8 * time.Minute); err != nil {
		return err
	}
	f.cfg = s2FakeConfig(f.c)
	return f.createProject()
}

func (f *s2ForgeWorld) liveProject() error {
	f.live = true
	path := os.Getenv("SOLODAWN_S2_LIVE_FORGE")
	if path == "" {
		return fmt.Errorf("set SOLODAWN_S2_LIVE_FORGE to the scratch forge configuration file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &f.cfg); err != nil {
		return err
	}
	if err := f.start(8 * time.Minute); err != nil {
		return err
	}
	return f.createProject()
}

func (f *s2ForgeWorld) groupCreate(id proto.ID) map[string]any {
	c := f.c
	t := f.cfg.Templates
	return map[string]any{
		"t": "group", "op": "create", "g": id, "n": id, "kind": "project", "repo": f.cfg.Repo, "base": f.cfg.Base,
		"name": "Custom forge", "forge": "custom", "checks": []string{}, "member_name": c.P.Name,
		"templates": map[string][]string{"push": t.Push, "open": t.Open, "checks": t.Checks, "merge": t.Merge},
	}
}

func (f *s2ForgeWorld) createProject() error {
	c := f.c
	f.group = c.NextID()
	bc := c.P.Conns[c.P1.ID()]
	if err := s2WatchCmd(c, c.P, bc.conn, f.groupCreate(f.group), ""); err != nil {
		return err
	}
	if err := c.syncGroup(bc.conn, f.group); err != nil {
		return err
	}
	return f.joinProjectAIs()
}

func (f *s2ForgeWorld) joinProjectAIs() error {
	c := f.c
	var err error
	f.sec, err = c.JoinAI(c.P, c.P1, c.P1, f.group, "claude", s2SecretaryName, true)
	if err != nil {
		return err
	}
	f.workers[0], err = c.JoinAI(c.P, c.P2, c.P1, f.group, "claude", "Claude", false)
	if err != nil {
		return err
	}
	if err := c.configureNodeAgent(c.P, c.P1, "codex", "team"); err != nil {
		return err
	}
	f.workers[1], err = c.JoinAI(c.P, c.P1, c.P1, f.group, "codex", "Codex", false)
	return err
}

func (f *s2ForgeWorld) rejectPushTitle() error {
	c := f.c
	conn := c.P.Conns[c.P1.ID()].conn
	create := f.groupCreate(c.NextID())
	templates := create["templates"].(map[string][]string)
	templates["push"] = append(slices.Clone(templates["push"]), "--title={title}")
	if err := s2WatchCmd(c, c.P, conn, create, "bad_request"); err != nil {
		return err
	}
	set := map[string]any{"t": "group", "op": "set", "g": f.group, "templates": templates}
	return s2WatchCmd(c, c.P, conn, set, "bad_request")
}

func (f *s2ForgeWorld) confirmTwo() error {
	if err := f.prepareTasks(2); err != nil {
		return err
	}
	return f.confirmBrief(m5TwoTaskBriefArgs(), 2)
}

func (f *s2ForgeWorld) confirmTitle() error {
	f.short = true
	tasks := []map[string]any{{"title": s2ForgeTitle, "assignee": "Claude", "reviewer": "Codex", "depends": []int{}}}
	if err := f.prepareTasks(1); err != nil {
		return err
	}
	return f.confirmBrief(m5BriefArgs(tasks), 1)
}

func (f *s2ForgeWorld) prepareTasks(count int) error {
	for num := 1; num <= count; num++ {
		if err := f.prepareTask(num); err != nil {
			return err
		}
	}
	if !f.live {
		return f.prepareFakeForge(count)
	}
	return nil
}

func (f *s2ForgeWorld) prepareTask(num int) error {
	work, review := f.c.P2, f.c.P1
	vendor, other := "claude", "codex"
	if num == 2 {
		work, review, vendor, other = review, work, other, vendor
	}
	if err := s2ForgeTurnFixture(work, vendor, num, 1, "first edit", m5WorkerCalls(true, s2ForgeSummary)); err != nil {
		return err
	}
	if f.short {
		return nil
	}
	reviewCalls := m5PassingReview(95)
	if err := s2ForgeTurnFixture(review, other, num, 1, "", reviewCalls); err != nil {
		return err
	}
	if num == 1 {
		return f.prepareRetry(work, review, vendor, other, reviewCalls)
	}
	return nil
}

func (f *s2ForgeWorld) prepareRetry(work, review *s2Node, vendor, other string, reviewCalls []m5ToolCallSpec) error {
	if f.live || f.short {
		return nil
	}
	if err := s2ForgeTurnFixture(work, vendor, 1, 2, "resolved edit", m5WorkerCalls(true, s2ForgeSummary)); err != nil {
		return err
	}
	return s2ForgeTurnFixture(review, other, 1, 2, "", reviewCalls)
}

func (f *s2ForgeWorld) prepareFakeForge(count int) error {
	var replies []s2ForgeReply
	for num := 1; num <= count; num++ {
		replies = append(replies, s2ForgeReply{Stdout: s2ForgeURL + strconv.Itoa(num) + "\n"})
	}
	if err := f.c.SetForgeReplies("watch-open", replies...); err != nil {
		return err
	}
	if count == 2 {
		if err := os.WriteFile(filepath.Join(f.c.ForgeDir, "advance-reviewed-head"), nil, 0600); err != nil {
			return err
		}
	}
	return nil
}

func (f *s2ForgeWorld) confirmBrief(args map[string]any, count int) error {
	c := f.c
	if err := c.WriteScript(c.P1, "claude", m5ClaudeScript(f.secretaryTurns(args, count)...)); err != nil {
		return err
	}
	conn := c.P.Conns[c.P1.ID()]
	post := s2WatchPost(f.group, "text", "Please build the helpers.", proto.EmptyBody{})
	if err := s2WatchCmd(c, c.P, conn.conn, post, ""); err != nil {
		return err
	}
	brief, err := f.waitBrief()
	if err != nil {
		return err
	}
	return s2WatchCmd(c, c.P, conn.conn, map[string]any{"t": "act", "g": f.group, "msg": brief, "action": "confirm"}, "")
}

func (f *s2ForgeWorld) secretaryTurns(args map[string]any, count int) [][]m5ToolCallSpec {
	turns := m5BriefTurns(args)
	if f.short {
		return turns
	}
	if !f.live {
		turns = append(turns, []m5ToolCallSpec{{Name: "set_task", Args: map[string]any{"task": "T-1", "assignee": "Claude"}}})
	}
	var summaries []string
	for num := 1; num <= count; num++ {
		summaries = append(summaries, fmt.Sprintf("Task %d merged.", num))
	}
	return append(turns, m5SummaryTurns(summaries...)...)
}

func (f *s2ForgeWorld) waitBrief() (string, error) {
	var brief string
	err := f.rows(func(raw json.RawMessage) (bool, error) {
		var row proto.MsgRow
		if proto.Decode(raw, &row) != nil || row.Kind != "brief" || row.By != f.sec.ID {
			return false, nil
		}
		brief = row.ID
		return true, nil
	})
	return brief, err
}

func (f *s2ForgeWorld) rows(match func(json.RawMessage) (bool, error)) error {
	return f.c.readGroupRowsUntil(f.c.P.Conns[f.c.P1.ID()], f.group, match)
}

func (f *s2ForgeWorld) waitConflict() error {
	return f.rows(func(raw json.RawMessage) (bool, error) {
		var row proto.TaskRow
		if proto.Decode(raw, &row) != nil || row.Num != 1 || row.Wait == nil {
			return false, nil
		}
		if row.Wait.Kind != "conflict" || row.Step != "local_check" {
			return false, nil
		}
		f.conflictCommit = row.Commit
		return true, nil
	})
}

func (f *s2ForgeWorld) waitTwo() error {
	return f.waitForSummaries(2)
}

func (f *s2ForgeWorld) waitForSummaries(count int) error {
	f.tasks = make(map[int]proto.TaskRow)
	f.summaries = make(map[string]proto.SummaryBody)
	if err := f.rows(func(raw json.RawMessage) (bool, error) {
		if err := f.observeTaskRow(raw); err != nil {
			return false, err
		}
		return len(f.summaries) == count, nil
	}); err != nil {
		return err
	}
	if err := f.checkTaskResults(count); err != nil {
		return err
	}
	return f.verifyZeroApprovalCards()
}

func (f *s2ForgeWorld) observeTaskRow(raw json.RawMessage) error {
	var task proto.TaskRow
	if proto.Decode(raw, &task) == nil && task.State == "merged" {
		f.tasks[task.Num] = task
		return nil
	}
	var row proto.MsgRow
	if proto.Decode(raw, &row) != nil {
		return nil
	}
	switch row.Kind {
	case "approval":
		return fmt.Errorf("unexpected approval %s", row.ID)
	case "summary":
		return f.recordSummary(row)
	}
	return nil
}

func (f *s2ForgeWorld) recordSummary(row proto.MsgRow) error {
	var body proto.SummaryBody
	if err := proto.Decode(row.Body, &body); err != nil {
		return err
	}
	if row.By != f.sec.ID || !row.Main || body.PR == nil {
		return fmt.Errorf("unexpected summary row %+v", row)
	}
	f.summaries[body.Task] = body
	return nil
}

func (f *s2ForgeWorld) checkTaskResults(count int) error {
	for num := 1; num <= count; num++ {
		task := f.tasks[num]
		if err := f.checkTaskResult(task); err != nil {
			return err
		}
	}
	if count == 2 {
		return checkS2CrossReview(f.tasks[1], f.tasks[2], [2]string{f.workers[0].ID, f.workers[1].ID})
	}
	return nil
}

func checkS2CrossReview(first, second proto.TaskRow, workers [2]string) error {
	for i, task := range []proto.TaskRow{first, second} {
		if task.Assignee != workers[i] || task.Reviewer != workers[1-i] || task.Review.By != workers[1-i] {
			return fmt.Errorf("task %d assignee %q, reviewer %q, review by %q; want %q and %q", task.Num, task.Assignee, task.Reviewer, task.Review.By, workers[i], workers[1-i])
		}
	}
	return nil
}

func (f *s2ForgeWorld) checkTaskResult(task proto.TaskRow) error {
	if task.Review == nil || !task.Review.OK || task.PR == 0 || task.Review.Commit != task.Commit {
		return fmt.Errorf("task did not merge after a passing review with a request: %+v", task)
	}
	if err := checkS2FinalChecks(task); err != nil {
		return err
	}
	return f.checkTaskSummary(task)
}

func checkS2FinalChecks(task proto.TaskRow) error {
	for _, check := range []*proto.Check{task.Checks.Local, task.Checks.Cloud} {
		if check == nil || !check.OK || check.Commit != task.Commit {
			return fmt.Errorf("task %d lacks passing checks for final commit %q", task.Num, task.Commit)
		}
	}
	return nil
}

func (f *s2ForgeWorld) checkTaskSummary(task proto.TaskRow) error {
	summary := f.summaries[task.ID]
	if summary.PR == nil || *summary.PR != task.PR {
		return fmt.Errorf("task %d summary request does not match %d", task.Num, task.PR)
	}
	return f.checkTaskCleanup(task)
}

func (f *s2ForgeWorld) verifyZeroApprovalCards() error {
	db, err := store.Open(filepath.Join(f.c.P1.Data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	var messages []proto.MsgRow
	err = db.Write(func(tx *store.Tx) error {
		rows, err := tx.Query("SELECT data FROM row WHERE g = ? AND r = 'msg' ORDER BY seq", f.group)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var message proto.MsgRow
			if err := proto.Decode(raw, &message); err != nil {
				return err
			}
			messages = append(messages, message)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	return m5CheckZeroApprovalCards(messages)
}

func (f *s2ForgeWorld) checkTaskCleanup(task proto.TaskRow) error {
	folder := fmt.Sprintf("%d-%s", task.Num, task.Slug)
	for _, node := range []*s2Node{f.c.P1, f.c.P2} {
		project := filepath.Join(node.Data, "projects", string(f.group))
		dirs := []string{filepath.Join(project, "wt", folder), filepath.Join(project, "review", strconv.Itoa(task.Num))}
		if err := f.waitTaskCleanup(node, task.ID, dirs); err != nil {
			return fmt.Errorf("task %d cleanup on %s: %w", task.Num, node.Name, err)
		}
	}
	return nil
}

func (f *s2ForgeWorld) waitTaskCleanup(node *s2Node, id string, dirs []string) error {
	db, err := store.Open(filepath.Join(node.Data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	return s2ForgeWaitCleanup(f.c.Ctx, db, f.group, id, dirs)
}

func s2ForgeWaitCleanup(ctx context.Context, db *store.Store, group proto.ID, id string, dirs []string) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		gone, err := s2ForgeSessionsGone(db, group, id, dirs)
		if err != nil || gone {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func s2ForgeSessionsGone(db *store.Store, group proto.ID, id string, dirs []string) (bool, error) {
	var count int
	err := db.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM session WHERE g = ? AND task = ?", group, id).Scan(&count)
	})
	return count == 0 && m5DirsGone(dirs), err
}

func (f *s2ForgeWorld) checkRemote() error {
	repo := f.c.RepoDir
	if err := runGitIn(repo, "fetch", f.cfg.Repo, f.cfg.Base); err != nil {
		return err
	}
	paths, err := m5GitOut(repo, "ls-tree", "--name-only", "FETCH_HEAD")
	if err != nil {
		return err
	}
	for num := 1; num <= len(f.tasks); num++ {
		if !slices.Contains(strings.Split(paths, "\n"), s2ForgeFile(num)) {
			return fmt.Errorf("scratch remote is missing task %d in %s", num, f.cfg.Base)
		}
	}
	return nil
}

func (f *s2ForgeWorld) verifyRecorded() error {
	if err := f.checkRemote(); err != nil {
		return err
	}
	calls, err := f.c.ForgeCalls()
	if err != nil {
		return err
	}
	if err := f.checkCalls(calls); err != nil {
		return err
	}
	gh, err := f.c.GHCalls()
	if err == nil && len(gh) != 0 {
		return fmt.Errorf("gh ran: %+v", gh)
	}
	return err
}

func (f *s2ForgeWorld) verifyTitle() error {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		calls, err := f.c.ForgeCalls()
		if err != nil {
			return err
		}
		i := slices.IndexFunc(calls, func(call s2ForgeCall) bool { return len(call.Args) > 0 && call.Args[0] == "watch-open" })
		if i >= 0 {
			return f.checkTitleOpen(calls, calls[i])
		}
		select {
		case <-f.c.Ctx.Done():
			return f.c.Ctx.Err()
		case <-tick.C:
		}
	}
}

func (f *s2ForgeWorld) checkTitleOpen(calls []s2ForgeCall, open s2ForgeCall) error {
	if len(open.Args) != 7 {
		return fmt.Errorf("unexpected open arguments %q", open.Args)
	}
	if _, err := os.Stat(filepath.Join(open.Dir, "marker")); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unexpected worktree marker: %v", err)
	}
	want := make(map[string]int)
	f.expectedCalls(want, proto.TaskRow{Branch: open.Args[1], Commit: open.Args[3], Title: s2ForgeTitle})
	return checkS2ForgeCallCounts(calls, want)
}

func (f *s2ForgeWorld) checkCalls(calls []s2ForgeCall) error {
	want := make(map[string]int)
	for _, task := range f.tasks {
		f.expectedCalls(want, task)
	}
	if f.conflictCommit != "" {
		task := f.tasks[1]
		task.Commit = f.conflictCommit
		f.expectedCalls(want, task)
	}
	return checkS2ForgeCallCounts(calls, want)
}

func checkS2ForgeCallCounts(calls []s2ForgeCall, want map[string]int) error {
	for _, call := range calls {
		key := string(mustMarshalNoEscape(call.Args))
		if want[key] == 0 {
			return fmt.Errorf("unexpected forge arguments %q", call.Args)
		}
		want[key]--
	}
	for args, n := range want {
		if n != 0 {
			return fmt.Errorf("missing %d forge call(s) with %s", n, args)
		}
	}
	return nil
}

func (f *s2ForgeWorld) expectedCalls(want map[string]int, task proto.TaskRow) {
	branch, base, commit, req := task.Branch, f.cfg.Base, task.Commit, strconv.Itoa(task.PR)
	s2WantForge(want, 2, "watch-push", branch, base, commit)
	if task.Num != 1 || f.conflictCommit == "" || commit == f.conflictCommit {
		s2WantForge(want, 1, "watch-open", branch, base, commit, "--title="+task.Title, "--summary="+s2ForgeSummary, "{tag}")
	}
	if f.short {
		return
	}
	s2WantForge(want, 1, "watch-checks", branch, base, commit, req)
	s2WantForge(want, 1, "watch-merge", branch, base, commit, req)
}

func s2WantForge(want map[string]int, n int, args ...string) {
	want[string(mustMarshalNoEscape(args))] = n
}

func TestS2ForgeCrossReview(t *testing.T) {
	workers := [2]string{"claude", "codex"}
	first := proto.TaskRow{Num: 1, Assignee: "claude", Reviewer: "codex", Review: &proto.Review{By: "codex"}}
	second := proto.TaskRow{Num: 2, Assignee: "codex", Reviewer: "claude", Review: &proto.Review{By: "claude"}}
	if err := checkS2CrossReview(first, second, workers); err != nil {
		t.Fatal(err)
	}
	for _, task := range []proto.TaskRow{
		{Num: 1, Review: &proto.Review{}},
		{Num: 1, Assignee: "codex", Reviewer: "codex", Review: &proto.Review{By: "codex"}},
		{Num: 1, Assignee: "claude", Reviewer: "claude", Review: &proto.Review{By: "codex"}},
		{Num: 1, Assignee: "claude", Reviewer: "codex", Review: &proto.Review{By: "claude"}},
	} {
		want := fmt.Sprintf("task 1 assignee %q, reviewer %q, review by %q; want %q and %q", task.Assignee, task.Reviewer, task.Review.By, "claude", "codex")
		if err := checkS2CrossReview(task, second, workers); err == nil || err.Error() != want {
			t.Fatalf("got %v, want %s", err, want)
		}
	}
}

func TestS2ForgeFinalChecks(t *testing.T) {
	passing := &proto.Check{OK: true, Commit: "final"}
	task := proto.TaskRow{Num: 1, Commit: "final", Checks: proto.TaskChecks{Local: passing, Cloud: passing}}
	if err := checkS2FinalChecks(task); err != nil {
		t.Fatal(err)
	}
	for _, check := range []*proto.Check{nil, {Commit: "final"}, {OK: true, Commit: "old"}} {
		for _, checks := range []proto.TaskChecks{{Local: check, Cloud: passing}, {Local: passing, Cloud: check}} {
			task.Checks = checks
			want := `task 1 lacks passing checks for final commit "final"`
			if err := checkS2FinalChecks(task); err == nil || err.Error() != want {
				t.Fatalf("got %v, want %s", err, want)
			}
		}
	}
}

func TestS2ForgeStaleReview(t *testing.T) {
	f := &s2ForgeWorld{}
	for _, review := range []*proto.Review{nil, {OK: true, Commit: "old"}} {
		task := proto.TaskRow{Commit: "final", PR: 1, Review: review}
		want := fmt.Sprintf("task did not merge after a passing review with a request: %+v", task)
		if err := f.checkTaskResult(task); err == nil || err.Error() != want {
			t.Fatalf("got %v, want %s", err, want)
		}
	}
}

func TestS2ForgeApprovalCards(t *testing.T) {
	for _, messages := range [][]proto.MsgRow{nil, {{Kind: "summary"}}} {
		if err := m5CheckZeroApprovalCards(messages); err != nil {
			t.Fatal(err)
		}
	}
	messages := []proto.MsgRow{{RowHeader: proto.RowHeader{ID: "first-push"}, Kind: "approval"}, {Kind: "summary"}}
	if err := m5CheckZeroApprovalCards(messages); err == nil || err.Error() != "unexpected approval card first-push" {
		t.Fatalf("got %v, want unexpected approval card first-push", err)
	}
}

func TestS2ForgeApprovalHistory(t *testing.T) {
	summary := proto.MsgRow{RowHeader: proto.RowHeader{Type: "msg", ID: "summary", Seq: 2}, Pos: 2,
		Group: "group", Kind: "summary", Mentions: []string{}, Body: json.RawMessage(`{}`)}
	approval := summary
	approval.ID, approval.Seq, approval.Pos, approval.Kind = "first-push", 1, 1, "approval"
	approval.State = &proto.CardState{Status: "accepted"}
	for _, tc := range []struct {
		name     string
		messages []proto.MsgRow
		want     string
	}{
		{name: "empty", want: "<nil>"},
		{name: "summary", messages: []proto.MsgRow{summary}, want: "<nil>"},
		{name: "earlier resolved approval", messages: []proto.MsgRow{approval, summary}, want: "unexpected approval card first-push"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := t.TempDir()
			f := &s2ForgeWorld{s2WatchWorld: &s2WatchWorld{c: &s2Cast{P1: &s2Node{Data: data}}}, group: proto.ID("group")}
			db, err := store.Open(filepath.Join(data, "node.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, message := range tc.messages {
				if err := db.Write(func(tx *store.Tx) error { return store.Put(tx, "group", message.Seq, message) }); err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprint(f.verifyZeroApprovalCards()); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
