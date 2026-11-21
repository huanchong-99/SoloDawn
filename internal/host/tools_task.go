package host

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
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

type tskReviewArgs struct {
	Score int                `json:"score"`
	Items []proto.ReviewItem `json:"items"`
}

type tskReviewResult struct {
	Passed bool `json:"passed"`
	Round  int  `json:"round"`
}

type tskCloudKey struct {
	group string
	task  string
}

type tskCloudRun struct {
	commit string
	cancel context.CancelFunc
}

// tskPart tracks active hosted-check polling goroutines per host.
// Lock order: mu is a leaf lock; nothing called under mu acquires Host,
// Sessions, Rows or Store locks.
type tskPart struct {
	mu      sync.Mutex
	cancels map[tskCloudKey]*tskCloudRun
}

func tskDecodeArgs(raw json.RawMessage, dst any) error {
	if proto.Decode(raw, dst) != nil {
		return &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	return nil
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
		Nonce:     freshNonce(),
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
	proto.Decode(raw, &grp)
	return grp, nil
}

func tskTaskRow(h *Host, group, taskID string) (proto.TaskRow, string, error) {
	raw, ok := h.Rows.Get(group, "task", taskID)
	if !ok {
		return proto.TaskRow{}, "", &Error{Code: "not_found", Message: "task state is not loaded"}
	}
	var task proto.TaskRow
	proto.Decode(raw, &task)
	name, _ := taskName(task.Num, string(task.Slug))
	return task, "task/" + name, nil
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
	return tskTaskEnv{grp: grp, task: task, branch: branch, worktree: sessionDir(h, c.Session)}, nil
}

func tskBaseRef(grp proto.GroupRow) string {
	if grp.Forge == "github" {
		return "origin/" + grp.Base
	}
	return grp.Base
}

func tskStartTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskStartArgs
	if err := tskDecodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Title); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Plan); err != nil {
		return nil, err
	}
	cmd := proto.TaskStart{
		TaskCommand: tskCommand(h, c.Group, c.Member, "start", freshNonce()),
		Title:       in.Title,
		Plan:        in.Plan,
		Re:          nil,
		Origin:      "direct",
	}
	ans, err := h.Command(ctx, c.Home, marshalWire(cmd))
	if err != nil {
		return nil, err
	}
	var res proto.CmdResult
	proto.Decode(ans, &res)
	return tskStartResult{Task: taskLabel(res.Num), Moved: true}, nil
}

func tskPushTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	if err := tskDecodeArgs(raw, &struct{}{}); err != nil {
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
	return tskPushBranch(ctx, sessionDir(h, c.Session), branch)
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
	if err := tskDecodeArgs(raw, &struct{}{}); err != nil {
		return nil, err
	}
	grp, err := tskGroupRow(h, c.Group)
	if err != nil {
		return nil, err
	}
	body, err := CheckRunner{}.Run(ctx, sessionDir(h, c.Session), tskBaseRef(grp), grp)
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
	if _, err := h.Command(ctx, c.Home, marshalWire(cmd)); err != nil {
		return tskChecksResult{}, err
	}
	return tskChecksResult{OK: body.OK, Commit: body.Commit, Items: items}, nil
}

func tskSubmitTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	var in tskSubmitArgs
	if err := tskDecodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if err := CheckOutboundSecret(in.Summary); err != nil {
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
	hc := HostedCheck{Task: c.Task, Branch: r.branch, Commit: r.head, PR: pr.Number, Since: time.UnixMilli(r.task.Checks.Local.At)}
	startHostedCheck(h, c.Home, c.Group, hc)
	return tskSubmitResult{OK: true, Commit: r.head, PR: pr.Number}, nil
}

func tskReportPost(h *Host, c Caller, r tskSubmitReady, summary string, pr *ForgePullRequest) string {
	var prNum *int
	if pr != nil {
		n := pr.Number
		prNum = &n
	}
	taskID := proto.ID(c.Task)
	return marshalWire(proto.Post[proto.ReportBody]{
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
	return marshalWire(cmd)
}

func startHostedCheck(h *Host, home, group string, hc HostedCheck) {
	p := Part[tskPart](h)
	key := tskCloudKey{group: group, task: hc.Task}
	pollCtx, run := tskBeginCloud(p, key, hc.Commit)
	if run == nil {
		return
	}
	wt := sessionDir(h, Session{SessionKey: SessionKey{Group: group}, Folder: "wt/" + strings.TrimPrefix(hc.Branch, "task/")})
	forge := TaskForge{Worktree: wt, Branch: hc.Branch}
	go tskPollHostedChecks(pollCtx, p, h, home, key, run, forge, hc.PR, hc.Commit, hc.Since)
}

func tskBeginCloud(p *tskPart, key tskCloudKey, commit string) (context.Context, *tskCloudRun) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancels == nil {
		p.cancels = map[tskCloudKey]*tskCloudRun{}
	}
	if prev := p.cancels[key]; prev != nil {
		if prev.commit == commit {
			return nil, nil
		}
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &tskCloudRun{commit: commit, cancel: cancel}
	p.cancels[key] = run
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
	h.Command(ctx, home, marshalWire(cmd))
}

func tskReviewTool(ctx context.Context, h *Host, c Caller, raw json.RawMessage) (any, error) {
	score, items, err := tskPrepareReview(raw)
	if err != nil {
		return nil, err
	}
	headOut, err := checkGit(ctx, sessionDir(h, c.Session), "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, tskCLIError(err)
	}
	return tskSendReview(ctx, h, c, strings.TrimSpace(headOut), score, items)
}

func tskPrepareReview(raw json.RawMessage) (int, []proto.ReviewItem, error) {
	var in tskReviewArgs
	if json.Unmarshal(raw, &in) != nil {
		return 0, nil, &Error{Code: "bad_request", Message: "invalid tool arguments"}
	}
	if err := tskCheckReviewSecrets(in.Items); err != nil {
		return 0, nil, err
	}
	return in.Score, in.Items, nil
}

func tskCheckReviewSecrets(items []proto.ReviewItem) error {
	for _, it := range items {
		if err := CheckOutboundSecret(it.Text); err != nil {
			return err
		}
		if it.File != nil {
			if err := CheckOutboundSecret(*it.File); err != nil {
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
	ans, err := h.Command(ctx, c.Home, marshalWire(post))
	if err != nil {
		return tskReviewResult{}, err
	}
	var res tskReviewResult
	json.Unmarshal(ans, &res)
	return res, nil
}
