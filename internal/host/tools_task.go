package host

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

var (
	tskStartSchema  = json.RawMessage(`{"type":"object","properties":{"plan":{"type":"string"},"title":{"type":"string"}},"required":["title","plan"],"additionalProperties":false}`)
	tskEmptySchema  = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	tskSubmitSchema = json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"],"additionalProperties":false}`)
	tskReviewSchema = json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"line":{"type":"integer","minimum":1},"text":{"type":"string"},"veto":{"type":"boolean","default":false}},"required":["text"],"additionalProperties":false}},"score":{"type":"integer","minimum":0,"maximum":100}},"required":["score","items"],"additionalProperties":false}`)

	tskFilesChangedRe = regexp.MustCompile(`(\d+) files? changed`)
	tskInsertionsRe   = regexp.MustCompile(`(\d+) insertions?\(\+\)`)
	tskDeletionsRe    = regexp.MustCompile(`(\d+) deletions?\(-\)`)
)

const tskMainDBSQL = `SELECT coalesce((SELECT file FROM pragma_database_list WHERE name = 'main'), '')`

func init() {
	Register(Tools, "start_task", Tool{
		Description: "Create a direct task and move the work to a new task session.",
		Schema:      tskStartSchema,
		For:         tskForWorkingGroup,
		Call:        tskStartTool,
	})
	Register(Tools, "push_branch", Tool{
		Description: "Push the task's assigned branch to origin without force.",
		Schema:      tskEmptySchema,
		For:         tskForTaskAboveLevel1,
		Call:        tskPushTool,
	})
	Register(Tools, "run_checks", Tool{
		Description: "Run the group's checks and both hard checks on the clean task worktree.",
		Schema:      tskEmptySchema,
		For:         tskForTaskAboveLevel1,
		Call:        tskChecksTool,
	})
	Register(Tools, "submit_task", Tool{
		Description: "Submit the checked commit, opening a pull request when the group has a remote.",
		Schema:      tskSubmitSchema,
		For:         tskForTaskAboveLevel1,
		Call:        tskSubmitTool,
	})
	Register(Tools, "submit_review", Tool{
		Description: "Submit a scored review of the checked-out task commit.",
		Schema:      tskReviewSchema,
		For:         tskForReview,
		Call:        tskReviewTool,
	})
}

func tskForWorkingGroup(c Caller) bool    { return c.Kind == GroupSession && !c.Secretary }
func tskForTaskAboveLevel1(c Caller) bool { return c.Kind == TaskSession && c.Level > 1 }
func tskForReview(c Caller) bool          { return c.Kind == ReviewSession }

type tskStartArgs struct {
	Title string `json:"title"`
	Plan  string `json:"plan"`
}

type tskStartResult struct {
	Task  string `json:"task"`
	Moved bool   `json:"moved"`
}

type tskPushResult struct {
	OK     bool   `json:"ok"`
	Commit string `json:"commit"`
}

type tskChecksResult struct {
	OK     bool              `json:"ok"`
	Commit string            `json:"commit"`
	Items  []proto.CheckItem `json:"items"`
}

type tskSubmitArgs struct {
	Summary string `json:"summary"`
}

type tskSubmitResult struct {
	OK     bool   `json:"ok"`
	Commit string `json:"commit"`
	PR     int    `json:"pr,omitempty"`
}

type tskTaskEnv struct {
	grp      proto.GroupRow
	task     proto.TaskRow
	branch   string
	worktree string
}

type tskSubmitReady struct {
	tskTaskEnv
	head    string
	files   int
	added   int
	removed int
}

type tskReviewItemIn struct {
	Text string  `json:"text"`
	File *string `json:"file,omitempty"`
	Line *int    `json:"line,omitempty"`
	Veto *bool   `json:"veto,omitempty"`
}

type tskReviewArgs struct {
	Score int               `json:"score"`
	Items []json.RawMessage `json:"items"`
}

type tskReviewResult struct {
	Passed bool `json:"passed"`
	Round  int  `json:"round"`
}

type tskCandidateMsg struct {
	ID       proto.ID `json:"id"`
	Pos      int64    `json:"pos"`
	Thread   *string  `json:"thread"`
	By       string   `json:"by"`
	Mentions []string `json:"mentions"`
}

type tskCloudKey struct {
	group string
	task  string
}

type tskCloudRun struct {
	cancel context.CancelFunc
}

// tskPart tracks active hosted-check polling goroutines per host.
// Lock order: mu is a leaf lock; nothing called under mu acquires Host,
// Sessions, Rows or Store locks.
type tskPart struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	cancels map[tskCloudKey]*tskCloudRun
}

func tskDecodeStrict(raw json.RawMessage, dst any, known ...string) error {
	if proto.Decode(raw, dst) != nil {
		return &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	var obj map[string]json.RawMessage
	json.Unmarshal(raw, &obj)
	for key := range obj {
		if !slices.Contains(known, key) {
			return &Error{Code: "bad_request", Message: "unknown tool argument: " + key}
		}
	}
	return nil
}

func tskCheckSecrets(texts ...string) error {
	for _, s := range texts {
		if err := CheckOutboundSecret(s); err != nil {
			return &Error{Code: "secret", Message: "this looks like a secret; remove it and send again"}
		}
	}
	return nil
}

func tskNewID() proto.ID {
	var b [16]byte
	rand.Read(b[:])
	return proto.ID(base64.RawURLEncoding.EncodeToString(b[:]))
}

func tskEncode(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

func tskCLIError(err error) error {
	return &Error{Code: "cli", Message: RedactSecrets(err.Error())}
}

func tskTreeError(err error) error {
	if errors.Is(err, ErrDirtyCheckTree) || errors.Is(err, ErrSkipWorktree) {
		return &Error{Code: "conflict", Message: strings.TrimPrefix(err.Error(), "conflict: ")}
	}
	return tskCLIError(err)
}

func tskHomeHeader(h *Host, group, as, cmdType string) proto.HomeCommand {
	return proto.HomeCommand{
		Type:      cmdType,
		Nonce:     tskNewID(),
		Timestamp: h.Now().UnixMilli(),
		Group:     proto.ID(group),
		As:        proto.ID(as),
	}
}

func tskCommand(h *Host, group, as, op string, taskID proto.ID) proto.TaskCommand {
	return proto.TaskCommand{
		HomeCommand: tskHomeHeader(h, group, as, "task"),
		Op:          op,
		ID:          taskID,
	}
}

func tskGroupRow(h *Host, group string) (proto.GroupRow, error) {
	raw, ok := h.Rows.Get(group, "group", group)
	if !ok {
		return proto.GroupRow{}, &Error{Code: "not_found", Message: "group state is not loaded"}
	}
	var grp proto.GroupRow
	if proto.Decode(raw, &grp) != nil {
		return proto.GroupRow{}, &Error{Code: "bad_request", Message: "invalid group row"}
	}
	return grp, nil
}

func tskTaskRow(h *Host, group, taskID string) (proto.TaskRow, string, error) {
	raw, ok := h.Rows.Get(group, "task", taskID)
	if !ok {
		return proto.TaskRow{}, "", &Error{Code: "not_found", Message: "task state is not loaded"}
	}
	var task proto.TaskRow
	if proto.Decode(raw, &task) != nil {
		return proto.TaskRow{}, "", &Error{Code: "bad_request", Message: "invalid task row"}
	}
	name, err := taskName(task.Num, string(task.Slug))
	if err != nil {
		return proto.TaskRow{}, "", &Error{Code: "bad_request", Message: err.Error()}
	}
	return task, "task/" + name, nil
}

func tskWorkDir(h *Host, c Caller) (string, error) {
	if c.Folder == "" {
		return "", &Error{Code: "bad_request", Message: "session has no working folder"}
	}
	if filepath.IsAbs(c.Folder) {
		return c.Folder, nil
	}
	var dbFile string
	readDB := func(tx *store.Tx) error { return tx.QueryRow(tskMainDBSQL).Scan(&dbFile) }
	h.Store.Write(readDB)
	if dbFile == "" {
		return "", &Error{Code: "bad_request", Message: "project directory is unavailable"}
	}
	return filepath.Join(filepath.Dir(dbFile), "projects", c.Group, c.Folder), nil
}

func tskLoadTaskEnv(h *Host, c Caller) (tskTaskEnv, error) {
	grp, err := tskGroupRow(h, c.Group)
	if err != nil {
		return tskTaskEnv{}, err
	}
	task, branch, err := tskTaskRow(h, c.Group, c.Task)
	if err != nil {
		return tskTaskEnv{}, err
	}
	worktree, err := tskWorkDir(h, c)
	if err != nil {
		return tskTaskEnv{}, err
	}
	return tskTaskEnv{grp: grp, task: task, branch: branch, worktree: worktree}, nil
}

func tskBaseRef(grp proto.GroupRow) string {
	if grp.Forge == "github" {
		return "origin/" + grp.Base
	}
	return grp.Base
}

func tskStartTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskStartArgs
	if err := tskDecodeStrict(raw, &in, "title", "plan"); err != nil {
		return nil, err
	}
	if err := tskCheckSecrets(in.Title, in.Plan); err != nil {
		return nil, err
	}
	cmd := proto.TaskStart{
		TaskCommand: tskCommand(h, c.Group, c.Member, "start", tskNewID()),
		Title:       in.Title,
		Plan:        in.Plan,
		Re:          tskTurnLeadMsg(ctx, h, c),
		Origin:      "direct",
	}
	ans, err := h.Command(ctx, c.Home, tskEncode(cmd))
	if err != nil {
		return nil, err
	}
	return tskDecodeStartResult(ans)
}

func tskDecodeStartResult(raw json.RawMessage) (tskStartResult, error) {
	var res proto.CmdResult
	if proto.Decode(raw, &res) != nil || res.Num < 1 {
		return tskStartResult{}, &Error{Code: "bad_request", Message: "invalid task start answer"}
	}
	return tskStartResult{Task: "T-" + strconv.Itoa(res.Num), Moved: true}, nil
}

func tskTurnLeadMsg(ctx context.Context, h *Host, c Caller) *proto.ID {
	if c.Turn.Kind != ItemsTurn && c.Turn.Kind != ReplacedTurn {
		return nil
	}
	if id := tskPickLeadMsg(tskCachedMsgs(h.Rows, c.Group, c.Turn.Thread), c.Member); id != nil {
		return id
	}
	return tskPickLeadMsg(tskFetchedMsgs(ctx, h, c), c.Member)
}

func tskCachedMsgs(rows *Rows, group, thread string) []tskCandidateMsg {
	return tskDecodeCandidateMsgs(rows.Of(group, "msg"), thread)
}

func tskFetchedMsgs(ctx context.Context, h *Host, c Caller) []tskCandidateMsg {
	home, ok := h.Home(c.Home)
	if !ok {
		return nil
	}
	var thread *string
	if c.Turn.Thread != "" {
		t := c.Turn.Thread
		thread = &t
	}
	build := func(id int64) any {
		return proto.FetchThread{
			FetchPage: proto.FetchPage{Request: proto.Request{Type: "fetch", ID: id}, Group: c.Group, Before: 0, Limit: 20},
			Thread:    thread,
		}
	}
	reply, err := home.Request(ctx, build)
	var page proto.Fetched
	if err != nil || proto.Decode(reply, &page) != nil {
		return nil
	}
	return tskDecodeCandidateMsgs(page.Rows, c.Turn.Thread)
}

func tskDecodeCandidateMsgs(rawRows []json.RawMessage, thread string) []tskCandidateMsg {
	var msgs []tskCandidateMsg
	for _, raw := range rawRows {
		var m tskCandidateMsg
		if proto.Decode(raw, &m) == nil && tskMsgThread(m.Thread) == thread {
			msgs = append(msgs, m)
		}
	}
	slices.SortFunc(msgs, tskCompareMsgPos)
	return msgs
}

func tskCompareMsgPos(a, b tskCandidateMsg) int {
	return cmp.Compare(b.Pos, a.Pos)
}

func tskMsgThread(thread *string) string {
	if thread == nil {
		return ""
	}
	return *thread
}

func tskPickLeadMsg(msgs []tskCandidateMsg, self string) *proto.ID {
	if id := tskFindLead(msgs, self, true); id != nil {
		return id
	}
	return tskFindLead(msgs, self, false)
}

func tskFindLead(msgs []tskCandidateMsg, self string, requireMention bool) *proto.ID {
	for _, m := range msgs {
		if tskEligibleLead(m, self, requireMention) {
			id := m.ID
			return &id
		}
	}
	return nil
}

func tskEligibleLead(m tskCandidateMsg, self string, requireMention bool) bool {
	if m.By == self || m.By == "system" {
		return false
	}
	return !requireMention || slices.Contains(m.Mentions, self)
}

func tskPushTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	if err := tskDecodeStrict(raw, &struct{}{}); err != nil {
		return nil, err
	}
	grp, err := tskGroupRow(h, c.Group)
	if err != nil {
		return nil, err
	}
	if grp.Forge != "github" {
		return nil, &Error{Code: "conflict", Message: "group has no remote repository"}
	}
	_, branch, err := tskTaskRow(h, c.Group, c.Task)
	if err != nil {
		return nil, err
	}
	worktree, err := tskWorkDir(h, c)
	if err != nil {
		return nil, err
	}
	return tskPushBranch(ctx, worktree, branch)
}

func tskPushBranch(ctx context.Context, worktree, branch string) (tskPushResult, error) {
	out, err := checkGit(ctx, worktree, "rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		return tskPushResult{}, tskCLIError(err)
	}
	if err := (TaskForge{Worktree: worktree, Branch: branch}).PushBranch(ctx); err != nil {
		return tskPushResult{}, tskCLIError(err)
	}
	return tskPushResult{OK: true, Commit: strings.TrimSpace(out)}, nil
}

func tskChecksTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	if err := tskDecodeStrict(raw, &struct{}{}); err != nil {
		return nil, err
	}
	grp, err := tskGroupRow(h, c.Group)
	if err != nil {
		return nil, err
	}
	worktree, err := tskWorkDir(h, c)
	if err != nil {
		return nil, err
	}
	body, err := CheckRunner{}.Run(ctx, worktree, tskBaseRef(grp), grp)
	if err != nil {
		return nil, tskTreeError(err)
	}
	return tskReportChecks(ctx, h, c, body)
}

func tskReportChecks(ctx context.Context, h *Host, c Caller, body proto.ChecksBody) (tskChecksResult, error) {
	items := slices.Concat([]proto.CheckItem{}, body.Items)
	cmd := proto.TaskChecked{
		TaskCommand: tskCommand(h, c.Group, "", "checked", proto.ID(c.Task)),
		OK:          body.OK,
		Items:       items,
		Commit:      body.Commit,
	}
	if _, err := h.Command(ctx, c.Home, tskEncode(cmd)); err != nil {
		return tskChecksResult{}, err
	}
	return tskChecksResult{OK: body.OK, Commit: body.Commit, Items: items}, nil
}

func tskSubmitTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskSubmitArgs
	if err := tskDecodeStrict(raw, &in, "summary"); err != nil {
		return nil, err
	}
	if err := tskCheckSecrets(in.Summary); err != nil {
		return nil, err
	}
	ready, err := tskPrepareSubmit(ctx, h, c)
	if err != nil {
		return nil, err
	}
	pr, err := tskForgeSubmitIfRemote(ctx, ready, in.Summary)
	if err != nil {
		return nil, err
	}
	return tskSendSubmission(ctx, h, c, ready, in.Summary, pr)
}

func tskPrepareSubmit(ctx context.Context, h *Host, c Caller) (tskSubmitReady, error) {
	env, err := tskLoadTaskEnv(h, c)
	if err != nil {
		return tskSubmitReady{}, err
	}
	if err := tskRequireLocalChecks(env.task.Checks.Local); err != nil {
		return tskSubmitReady{}, err
	}
	head, err := tskVerifyCheckedCommit(ctx, env.worktree, env.branch, env.task.Checks.Local.Commit)
	if err != nil {
		return tskSubmitReady{}, err
	}
	files, added, removed, err := tskShortstat(ctx, env.worktree, tskBaseRef(env.grp), head)
	if err != nil {
		return tskSubmitReady{}, err
	}
	return tskSubmitReady{tskTaskEnv: env, head: head, files: files, added: added, removed: removed}, nil
}

func tskRequireLocalChecks(local *proto.Check) error {
	if local == nil || !local.OK {
		return &Error{Code: "conflict", Message: "local checks have not passed"}
	}
	return nil
}

func tskVerifyCheckedCommit(ctx context.Context, worktree, branch, checked string) (string, error) {
	head, err := cleanCheckHead(ctx, worktree)
	if err != nil {
		return "", tskTreeError(err)
	}
	branchOut, err := checkGit(ctx, worktree, "rev-parse", "--verify", "refs/heads/"+branch)
	if err != nil {
		return "", tskCLIError(err)
	}
	if head != checked || strings.TrimSpace(branchOut) != checked {
		return "", &Error{Code: "conflict", Message: "task head differs from the checked commit"}
	}
	return head, nil
}

func tskShortstat(ctx context.Context, worktree, baseRef, commit string) (files, added, removed int, err error) {
	out, err := checkGit(ctx, worktree, "diff", "--no-ext-diff", "--shortstat", "--end-of-options", baseRef+"..."+commit, "--")
	if err != nil {
		return 0, 0, 0, tskCLIError(err)
	}
	files, added, removed = tskParseShortstat(out)
	return files, added, removed, nil
}

func tskStatCount(re *regexp.Regexp, text string) int {
	if m := re.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func tskParseShortstat(text string) (files, added, removed int) {
	return tskStatCount(tskFilesChangedRe, text), tskStatCount(tskInsertionsRe, text), tskStatCount(tskDeletionsRe, text)
}

func tskForgeSubmitIfRemote(ctx context.Context, r tskSubmitReady, summary string) (*ForgePullRequest, error) {
	if r.grp.Forge != "github" {
		return nil, nil
	}
	pr, err := (TaskForge{Worktree: r.worktree, Branch: r.branch}).Submit(ctx, r.head, r.grp.Base, r.task.Title, summary)
	if err != nil {
		return nil, tskCLIError(err)
	}
	return pr, nil
}

func tskSendSubmission(ctx context.Context, h *Host, c Caller, r tskSubmitReady, summary string, pr *ForgePullRequest) (tskSubmitResult, error) {
	if _, err := h.Command(ctx, c.Home, tskReportPost(h, c, r, summary, pr)); err != nil {
		return tskSubmitResult{}, err
	}
	if _, err := h.Command(ctx, c.Home, tskSubmittedCmd(h, c, r, pr)); err != nil {
		return tskSubmitResult{}, err
	}
	if pr == nil {
		return tskSubmitResult{OK: true, Commit: r.head}, nil
	}
	tskStartHostedChecks(h, c, r, pr.Number)
	return tskSubmitResult{OK: true, Commit: r.head, PR: pr.Number}, nil
}

func tskReportPost(h *Host, c Caller, r tskSubmitReady, summary string, pr *ForgePullRequest) string {
	var prNum *int
	if pr != nil {
		n := pr.Number
		prNum = &n
	}
	taskID := proto.ID(c.Task)
	return tskEncode(proto.Post[proto.ReportBody]{
		HomeCommand: tskHomeHeader(h, c.Group, c.Member, "post"),
		Kind:        "report",
		Thread:      &taskID,
		Main:        false,
		Text:        summary,
		Mentions:    []proto.ID{},
		Re:          nil,
		Body: proto.ReportBody{
			Commit:  r.head,
			Files:   r.files,
			Added:   r.added,
			Removed: r.removed,
			PR:      prNum,
		},
	})
}

func tskSubmittedCmd(h *Host, c Caller, r tskSubmitReady, pr *ForgePullRequest) string {
	cmd := proto.TaskSubmitted{
		TaskCommand: tskCommand(h, c.Group, "", "submitted", proto.ID(c.Task)),
		Commit:      r.head,
		Stats:       proto.DiffStats{Added: r.added, Removed: r.removed},
	}
	if pr != nil {
		cmd.PR, cmd.PRURL = pr.Number, pr.URL
	}
	return tskEncode(cmd)
}

func tskStartHostedChecks(h *Host, c Caller, r tskSubmitReady, prNum int) {
	p := Part[tskPart](h)
	key := tskCloudKey{group: c.Group, task: c.Task}
	pollCtx, run := tskBeginCloud(p, key)
	forge := TaskForge{Worktree: r.worktree, Branch: r.branch}
	since := time.UnixMilli(r.task.Checks.Local.At)
	go tskPollHostedChecks(pollCtx, p, h, c.Home, key, run, forge, prNum, r.head, since)
}

func tskBeginCloud(p *tskPart, key tskCloudKey) (context.Context, *tskCloudRun) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancels == nil {
		p.cancels = map[tskCloudKey]*tskCloudRun{}
	}
	if prev := p.cancels[key]; prev != nil {
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &tskCloudRun{cancel: cancel}
	p.cancels[key] = run
	p.wg.Add(1)
	return ctx, run
}

func tskFinishCloud(p *tskPart, key tskCloudKey, run *tskCloudRun) {
	p.mu.Lock()
	defer p.mu.Unlock()
	run.cancel()
	if p.cancels[key] == run {
		delete(p.cancels, key)
	}
}

func tskPollHostedChecks(ctx context.Context, p *tskPart, h *Host, home string, key tskCloudKey, run *tskCloudRun, forge TaskForge, pr int, commit string, since time.Time) {
	defer p.wg.Done()
	defer tskFinishCloud(p, key, run)
	body, err := forge.WaitHostedChecks(ctx, pr, commit, since)
	if err != nil {
		return
	}
	cmd := proto.TaskChecked{
		TaskCommand: tskCommand(h, key.group, "", "cloud", proto.ID(key.task)),
		OK:          body.OK,
		Items:       slices.Concat([]proto.CheckItem{}, body.Items),
		Commit:      body.Commit,
	}
	h.Command(ctx, home, tskEncode(cmd))
}

func tskReviewTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	score, items, err := tskPrepareReview(raw)
	if err != nil {
		return nil, err
	}
	reviewDir, err := tskWorkDir(h, c)
	if err != nil {
		return nil, err
	}
	headOut, err := checkGit(ctx, reviewDir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, tskCLIError(err)
	}
	return tskSendReview(ctx, h, c, strings.TrimSpace(headOut), score, items)
}

func tskPrepareReview(raw json.RawMessage) (int, []proto.ReviewItem, error) {
	var in tskReviewArgs
	if err := tskDecodeStrict(raw, &in, "score", "items"); err != nil {
		return 0, nil, err
	}
	if in.Score < 0 || in.Score > 100 {
		return 0, nil, &Error{Code: "bad_request", Message: "score must be between 0 and 100"}
	}
	items, err := tskDecodeReviewItems(in.Items)
	if err != nil {
		return 0, nil, err
	}
	if err := tskCheckReviewSecrets(items); err != nil {
		return 0, nil, err
	}
	return in.Score, items, nil
}

func tskDecodeReviewItems(rawItems []json.RawMessage) ([]proto.ReviewItem, error) {
	items := make([]proto.ReviewItem, len(rawItems))
	for i, raw := range rawItems {
		item, err := tskDecodeReviewItem(raw)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

func tskDecodeReviewItem(raw json.RawMessage) (proto.ReviewItem, error) {
	var in tskReviewItemIn
	if tskDecodeStrict(raw, &in, "text", "file", "line", "veto") != nil || !tskValidReviewItem(in) {
		return proto.ReviewItem{}, &Error{Code: "bad_request", Message: "invalid review item"}
	}
	var veto bool
	if in.Veto != nil {
		veto = *in.Veto
	}
	return proto.ReviewItem{Text: in.Text, Veto: veto, File: in.File, Line: in.Line}, nil
}

func tskValidReviewItem(in tskReviewItemIn) bool {
	if in.Text == "" || (in.File != nil && *in.File == "") {
		return false
	}
	return in.Line == nil || *in.Line >= 1
}

func tskCheckReviewSecrets(items []proto.ReviewItem) error {
	for _, it := range items {
		if err := tskCheckSecrets(it.Text); err != nil {
			return err
		}
		if it.File != nil {
			if err := tskCheckSecrets(*it.File); err != nil {
				return err
			}
		}
	}
	return nil
}

func tskSendReview(ctx context.Context, h *Host, c Caller, commit string, score int, items []proto.ReviewItem) (tskReviewResult, error) {
	taskID := proto.ID(c.Task)
	post := proto.Post[proto.ReviewSigned]{
		HomeCommand: tskHomeHeader(h, c.Group, c.Member, "post"),
		Kind:        "review",
		Thread:      &taskID,
		Main:        false,
		Text:        "",
		Mentions:    []proto.ID{},
		Re:          nil,
		Body: proto.ReviewSigned{
			Task:   taskID,
			Commit: commit,
			Score:  score,
			Items:  items,
		},
	}
	ans, err := h.Command(ctx, c.Home, tskEncode(post))
	if err != nil {
		return tskReviewResult{}, err
	}
	return tskDecodeReviewResult(ans)
}

func tskDecodeReviewResult(raw json.RawMessage) (tskReviewResult, error) {
	var res proto.CmdResult
	if proto.Decode(raw, &res) != nil || res.Passed == nil || res.Round < 1 {
		return tskReviewResult{}, &Error{Code: "bad_request", Message: "invalid submit_review answer"}
	}
	return tskReviewResult{Passed: *res.Passed, Round: res.Round}, nil
}
