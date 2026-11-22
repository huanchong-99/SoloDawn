package proto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
)

// TaskTrace is a complete signed transcript of one confirmed task from the
// owner's request to the secretary's summary (planning/flows/trace-task.md).
type TaskTrace struct {
	Keys   []TraceKey     `json:"keys"`
	Devlog Devlog         `json:"devlog"`
	Group  GroupRow       `json:"group"`
	Owner  HumanMemberRow `json:"owner"`
	AIs    []AIMemberRow  `json:"ais"`
	Seq    int64          `json:"seq"`
	Num    int            `json:"num"`
	Steps  []TraceStep    `json:"steps"`
}

// TraceKey pins the Ed25519 seed and public device id of one device in the cast.
type TraceKey struct {
	Name   string `json:"name"`
	Seed   string `json:"seed"`
	Device string `json:"device"`
}

// TraceStep is one command on ctl, its response, the rows the home writes and
// pushes, and the inbox deliveries and host acknowledgements that follow.
type TraceStep struct {
	From    string       `json:"from"`
	Cmd     Cmd          `json:"cmd"`
	Answer  CmdResult    `json:"answer"`
	Rows    Rows         `json:"rows"`
	Inboxes []TraceInbox `json:"inboxes"`
}

// TraceInbox is an inbox push to one host device and that host's acknowledgements.
type TraceInbox struct {
	To    string `json:"to"`
	Inbox Inbox  `json:"inbox"`
	Acks  []Ack  `json:"acks"`
}

const (
	traceCommit      = "9f2c1abf43f3224eb6826ef971a943dbab4795ce"
	traceMergeCommit = "c1d9e7a2f0b34455aa8f0e6d7c2b1a9988776655"
	traceTitle       = "Node: report path changes"
	tracePR          = 31
	tracePRURL       = "https://git.example/team/website/pull/31"
)

func traceSeedKey(name, label string) TraceKey {
	priv := signer(label)
	seed := base64.RawURLEncoding.EncodeToString(priv.Seed())
	return TraceKey{Name: name, Seed: seed, Device: device(priv)}
}

func traceDerivedID(input string) string {
	sum := sha256.Sum256([]byte(input))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

type traceBuilder struct {
	t      *testing.T
	remote bool
	obKey  ed25519.PrivateKey
	haKey  ed25519.PrivateKey
	hbKey  ed25519.PrivateKey
	ob     string
	ha     string
	hb     string
	g      string
	mO     string
	mS     string
	mX     string
	mY     string
	m1     string
	m2     string
	c1     string
	t1     string
	seq    int64
	reqID  int64
	itemID int64
	brief  MessageRow[BriefBody, StatusOnly]
	task   TaskRow
	steps  []TraceStep
}

func newTraceBuilder(t *testing.T, remote bool) *traceBuilder {
	t.Helper()
	obKey := signer("trace browser")
	haKey := signer("trace host a")
	hbKey := signer("trace host b")
	g := id("trace group")
	c1 := id("trace c1")
	return &traceBuilder{
		t: t, remote: remote,
		obKey: obKey, haKey: haKey, hbKey: hbKey,
		ob: device(obKey), ha: device(haKey), hb: device(hbKey),
		g:   g,
		mO:  traceDerivedID("SoloDawn group owner " + g),
		mS:  id("trace secretary"),
		mX:  id("trace ai x"),
		mY:  id("trace ai y"),
		m1:  id("trace m1"),
		m2:  id("trace m2"),
		c1:  c1,
		t1:  traceDerivedID("SoloDawn brief task " + c1 + " 0"),
		seq: 800,
	}
}

func (b *traceBuilder) keys() []TraceKey {
	ob := traceSeedKey("OB", "trace browser")
	ha := traceSeedKey("HA", "trace host a")
	if !b.remote {
		return []TraceKey{ob, ha}
	}
	return []TraceKey{ob, ha, traceSeedKey("HB", "trace host b")}
}

func (b *traceBuilder) devlog() Devlog {
	entry := func(ver int, dev, kind, name, nonce string, at int64) Envelope {
		e := DeviceEntry{
			PayloadHeader{Type: "device", Nonce: ID(id(nonce)), Timestamp: at},
			DeviceChange{User: b.ob, Version: ver, Op: "add", Device: dev, Kind: kind, Name: name},
		}
		return signed(b.obKey, string(wireJSON(b.t, e)))
	}
	entries := []Envelope{
		entry(1, b.ob, "browser", "phone browser", "trace dev 1", ts-70000),
		entry(2, b.ha, "node", "workstation", "trace dev 2", ts-69000),
	}
	if b.remote {
		entries = append(entries, entry(3, b.hb, "node", "laptop", "trace dev 3", ts-68000))
	}
	return Devlog{Type: "devlog", User: b.ob, Entries: entries}
}

func (b *traceBuilder) groupRow() GroupRow {
	repo, forge := "/srv/repos/website", "none"
	if b.remote {
		repo, forge = "https://git.example/team/website.git", "github"
	}
	return GroupRow{
		RowHeader: RowHeader{"group", b.g, 796}, Name: "Website", Kind: "project",
		Repo: repo, Base: "main", Forge: forge, Home: b.ha, Secretary: b.mS,
		Checks: []string{"go vet ./...", "go test ./..."},
		Pass:   90, PassFoundation: 70, Limits: limits, Created: ts - 60000,
	}
}

func (b *traceBuilder) ais() []AIMemberRow {
	ai := func(memberID, name, cli, hostDev, hostName, path string, sec bool, seq int64) AIMemberRow {
		return AIMemberRow{
			MemberRow: MemberRow{RowHeader{"member", memberID, seq}, b.g, "ai", b.ob, name, ts - 10000},
			CLI:       cli, Host: hostDev, HostName: hostName, Secretary: sec, Account: "personal",
			Allow: []string{}, Approvers: []string{b.ob}, State: "waiting", Since: ts - 10000,
			Path: path, Usage: Usage{},
		}
	}
	revHost, revName, revPath := b.ha, "workstation", "local"
	if b.remote {
		revHost, revName, revPath = b.hb, "laptop", "direct"
	}
	return []AIMemberRow{
		ai(b.mS, "Secretary", "claude", b.ha, "workstation", "local", true, 798),
		ai(b.mX, "Claude", "claude", b.ha, "workstation", "local", false, 799),
		ai(b.mY, "Codex", "codex", revHost, revName, revPath, false, 800),
	}
}

func (b *traceBuilder) nextSeq() int64 {
	b.seq++
	return b.seq
}

func (b *traceBuilder) nextItem(ai, why string) Item {
	b.itemID++
	return Item{Item: b.itemID, AI: ID(ai), Why: why}
}

func (b *traceBuilder) deliver(to string, items ...InboxItem) TraceInbox {
	acks := make([]Ack, len(items))
	for i, it := range items {
		acks[i] = Ack{Type: "ack", Group: ID(b.g), Item: it.Item.Item}
	}
	return TraceInbox{
		To:    to,
		Inbox: Inbox{Type: "inbox", Group: ID(b.g), Items: items},
		Acks:  acks,
	}
}

func (b *traceBuilder) send(key ed25519.PrivateKey, payload any, answer CmdResult, rows []any, inboxes ...TraceInbox) {
	b.reqID++
	env := signed(key, string(wireJSON(b.t, payload)))
	answer.Re, answer.OK = b.reqID, true
	rawRows := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		rawRows[i] = wireJSON(b.t, r)
	}
	if inboxes == nil {
		inboxes = []TraceInbox{}
	}
	b.steps = append(b.steps, TraceStep{
		From:    device(key),
		Cmd:     Cmd{Request{"cmd", b.reqID}, env},
		Answer:  answer,
		Rows:    Rows{Type: "rows", Group: b.g, Rows: rawRows},
		Inboxes: inboxes,
	})
}

func (b *traceBuilder) advanceThread(by, kind, text string, at int64) TaskRow {
	b.task.Seq = b.nextSeq()
	b.task.Thread.Count++
	b.task.Thread.Last = &ThreadMessage{By: by, Kind: kind, Text: text, Timestamp: at}
	return b.task
}

func (b *traceBuilder) stepText() {
	at := ts + 10000
	post := Post[EmptyBody]{
		HomeCommand: HomeCommand{Type: "post", Nonce: ID(b.m1), Timestamp: at, Group: ID(b.g)},
		Kind:        "text", Text: "Show the connection path and warn when relayed.",
		Mentions: []ID{}, Body: EmptyBody{},
	}
	seq := b.nextSeq()
	env := signed(b.obKey, string(wireJSON(b.t, post)))
	row := MessageRow[EmptyBody, NoCard]{
		RowHeader: RowHeader{"msg", b.m1, seq}, Pos: seq, Group: b.g, By: b.mO, Kind: "text",
		Timestamp: at, Text: post.Text, Mentions: []string{}, Body: EmptyBody{}, Env: &env,
	}
	it := b.nextItem(b.mS, "mention")
	it.Msg = ID(b.m1)
	b.send(b.obKey, post, CmdResult{}, []any{row}, b.deliver(b.ha, InboxItem{Item: it, Msg: wireJSON(b.t, row)}))
}

func (b *traceBuilder) stepBrief() {
	at := ts + 20000
	body := BriefBody{
		Title:    "Show the connection path",
		Goal:     "People can see how a peer is connected.",
		Criteria: []string{"Report every connection path change within 1 s."},
		Scope:    BriefScope{In: []string{"node path events"}, Out: []string{"relay server"}},
		Tasks:    []BriefTask{{Title: traceTitle, Assignee: b.mX, Reviewer: b.mY, Depends: []int{}}},
		Pass:     90, Requester: b.mO,
	}
	post := Post[BriefBody]{
		HomeCommand: HomeCommand{Type: "post", Nonce: ID(b.m2), Timestamp: at, Group: ID(b.g), As: ID(b.mS)},
		Kind:        "brief", Text: "", Mentions: []ID{}, Body: body,
	}
	seq := b.nextSeq()
	env := signed(b.haKey, string(wireJSON(b.t, post)))
	b.brief = MessageRow[BriefBody, StatusOnly]{
		RowHeader: RowHeader{"msg", b.m2, seq}, Pos: seq, Group: b.g, By: b.mS, Kind: "brief",
		Timestamp: at, Mentions: []string{}, Body: body, State: StatusOnly{"pending"}, Env: &env,
	}
	b.send(b.haKey, post, CmdResult{}, []any{b.brief})
}

func (b *traceBuilder) stepConfirm() {
	at := ts + 30000
	act := Act{
		HomeCommand: HomeCommand{Type: "act", Nonce: ID(b.c1), Timestamp: at, Group: ID(b.g)},
		Msg:         ID(b.m2), Action: "confirm",
	}
	actEnv := signed(b.obKey, string(wireJSON(b.t, act)))
	confirmed := MessageRow[BriefBody, ActedState]{
		RowHeader: RowHeader{"msg", b.m2, b.nextSeq()}, Pos: b.brief.Pos, Group: b.g, By: b.mS,
		Kind: "brief", Timestamp: b.brief.Timestamp, Mentions: []string{}, Body: b.brief.Body,
		State: ActedState{Status: "confirmed", By: b.mO, At: at, Act: actEnv}, Env: b.brief.Env,
	}
	b.task = TaskRow{
		RowHeader: RowHeader{"task", b.t1, b.nextSeq()}, Group: b.g, Num: 12, Title: traceTitle,
		Slug: MakeSlug(traceTitle), Origin: "brief", Brief: b.m2, State: "active", Step: "branch",
		Assignee: b.mX, Reviewer: b.mY, Pass: 90, Depends: []string{},
		Stats: TaskStats{Started: at}, Created: at,
	}
	msgSeq := b.nextSeq()
	taskMsg := MessageRow[TaskBody, NoCard]{
		RowHeader: RowHeader{"msg", b.t1, msgSeq}, Pos: msgSeq, Group: b.g, By: "system",
		Kind: "task", Timestamp: at, Mentions: []string{}, Body: TaskBody{Task: b.t1},
	}
	assign := b.nextItem(b.mX, "assign")
	assign.Task, assign.Msg = ID(b.t1), ID(b.t1)
	told := b.nextItem(b.mS, "confirmed")
	told.Msg = ID(b.m2)
	taskCopy := b.task
	delivery := b.deliver(b.ha,
		InboxItem{Item: assign, Msg: wireJSON(b.t, taskMsg), Task: &taskCopy},
		InboxItem{Item: told, Msg: wireJSON(b.t, confirmed)},
	)
	b.send(b.obKey, act, CmdResult{}, []any{confirmed, b.task, taskMsg}, delivery)
}

func (b *traceBuilder) stepBranch() {
	at := ts + 40000
	branch := "task/12-" + string(b.task.Slug)
	cmd := TaskBranch{
		TaskCommand: TaskCommand{HomeCommand{"task", ID(id("trace branch")), at, ID(b.g), ""}, "branch", ID(b.t1)},
		Branch:      branch,
	}
	b.task.Seq = b.nextSeq()
	b.task.Step, b.task.Branch = "local_check", branch
	b.send(b.haKey, cmd, CmdResult{}, []any{b.task})
}

func (b *traceBuilder) stepProgress() {
	at := ts + 90000
	msgID := id("trace progress")
	tid, tidStr := ID(b.t1), b.t1
	text := "Branch created. Reporting path changes from the peer loop."
	post := Post[EmptyBody]{
		HomeCommand: HomeCommand{Type: "post", Nonce: ID(msgID), Timestamp: at, Group: ID(b.g), As: ID(b.mX)},
		Kind:        "progress", Thread: &tid, Text: text, Mentions: []ID{}, Body: EmptyBody{},
	}
	seq := b.nextSeq()
	env := signed(b.haKey, string(wireJSON(b.t, post)))
	msg := MessageRow[EmptyBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, seq}, Pos: seq, Group: b.g, Thread: &tidStr,
		By: b.mX, Kind: "progress", Timestamp: at, Text: text, Mentions: []string{}, Body: EmptyBody{}, Env: &env,
	}
	b.send(b.haKey, post, CmdResult{}, []any{msg, b.advanceThread(b.mX, "progress", text, at)})
}

func (b *traceBuilder) stepChecked() {
	at := ts + 140000
	msgID := id("trace checked")
	tidStr := b.t1
	items := []CheckItem{{Name: "go vet ./...", OK: true}, {Name: "go test ./...", OK: true, Detail: new("18 tests")}}
	cmd := TaskChecked{
		TaskCommand: TaskCommand{HomeCommand{"task", ID(msgID), at, ID(b.g), ""}, "checked", ID(b.t1)},
		OK:          true, Items: items, Commit: traceCommit,
	}
	seq := b.nextSeq()
	msg := MessageRow[ChecksBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, seq}, Pos: seq, Group: b.g, Thread: &tidStr,
		By: "system", Kind: "checks", Timestamp: at, Mentions: []string{},
		Body: ChecksBody{Stage: "local", OK: true, Commit: traceCommit, Items: items},
	}
	b.task.Checks.Local = &Check{OK: true, At: at, Commit: traceCommit}
	b.send(b.haKey, cmd, CmdResult{}, []any{msg, b.advanceThread("system", "checks", "", at)})
}

func (b *traceBuilder) stepReport() {
	at := ts + 150000
	msgID := id("trace report")
	tid, tidStr := ID(b.t1), b.t1
	text := "Path changes are emitted with transport kind and round-trip time."
	body := ReportBody{Commit: traceCommit, Files: 3, Added: 84, Removed: 12}
	if b.remote {
		body.PR = new(tracePR)
	}
	post := Post[ReportBody]{
		HomeCommand: HomeCommand{Type: "post", Nonce: ID(msgID), Timestamp: at, Group: ID(b.g), As: ID(b.mX)},
		Kind:        "report", Thread: &tid, Text: text, Mentions: []ID{}, Body: body,
	}
	seq := b.nextSeq()
	env := signed(b.haKey, string(wireJSON(b.t, post)))
	msg := MessageRow[ReportBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, seq}, Pos: seq, Group: b.g, Thread: &tidStr,
		By: b.mX, Kind: "report", Timestamp: at, Text: text, Mentions: []string{}, Body: body, Env: &env,
	}
	b.send(b.haKey, post, CmdResult{}, []any{msg, b.advanceThread(b.mX, "report", text, at)})
}

func (b *traceBuilder) stepSubmitted() {
	at := ts + 160000
	cmd := TaskSubmitted{
		TaskCommand: TaskCommand{HomeCommand{"task", ID(id("trace submitted")), at, ID(b.g), ""}, "submitted", ID(b.t1)},
		Commit:      traceCommit, Stats: DiffStats{Added: 84, Removed: 12},
	}
	b.task.Seq = b.nextSeq()
	b.task.Commit = traceCommit
	b.task.Stats.Added, b.task.Stats.Removed = 84, 12
	if b.remote {
		cmd.PR, cmd.PRURL = tracePR, tracePRURL
		b.task.Step, b.task.PR, b.task.PRURL = "cloud_check", tracePR, tracePRURL
		b.send(b.haKey, cmd, CmdResult{}, []any{b.task})
		return
	}
	b.task.Step = "review"
	rev := b.nextItem(b.mY, "review")
	rev.Task, rev.Commit = ID(b.t1), traceCommit
	taskCopy := b.task
	b.send(b.haKey, cmd, CmdResult{}, []any{b.task}, b.deliver(b.ha, InboxItem{Item: rev, Task: &taskCopy}))
}

func (b *traceBuilder) stepCloud() {
	at := ts + 190000
	msgID := id("trace cloud")
	tidStr := b.t1
	items := []CheckItem{{Name: "ci / test", OK: true}}
	cmd := TaskChecked{
		TaskCommand: TaskCommand{HomeCommand{"task", ID(msgID), at, ID(b.g), ""}, "cloud", ID(b.t1)},
		OK:          true, Items: items, Commit: traceCommit,
	}
	seq := b.nextSeq()
	msg := MessageRow[ChecksBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, seq}, Pos: seq, Group: b.g, Thread: &tidStr,
		By: "system", Kind: "checks", Timestamp: at, Mentions: []string{},
		Body: ChecksBody{Stage: "cloud", OK: true, Commit: traceCommit, Items: items},
	}
	b.task.Step = "review"
	b.task.Checks.Cloud = &Check{OK: true, At: at, Commit: traceCommit}
	moved := b.advanceThread("system", "checks", "", at)
	rev := b.nextItem(b.mY, "review")
	rev.Task, rev.Commit = ID(b.t1), traceCommit
	taskCopy := moved
	b.send(b.haKey, cmd, CmdResult{}, []any{msg, moved}, b.deliver(b.hb, InboxItem{Item: rev, Task: &taskCopy}))
}

func (b *traceBuilder) stepReview() {
	at := ts + 230000
	msgID := id("trace review")
	tid, tidStr := ID(b.t1), b.t1
	text := "Path transitions and round-trip fields are covered cleanly."
	items := []ReviewItem{{Text: "Covers direct-to-relay and relay-to-direct transitions.", Veto: false, File: new("internal/peer/path.go"), Line: new(42)}}
	post := Post[ReviewSigned]{
		HomeCommand: HomeCommand{Type: "post", Nonce: ID(msgID), Timestamp: at, Group: ID(b.g), As: ID(b.mY)},
		Kind:        "review", Thread: &tid, Text: text, Mentions: []ID{},
		Body: ReviewSigned{Task: ID(b.t1), Commit: traceCommit, Score: 93, Items: items},
	}
	revKey := b.haKey
	if b.remote {
		revKey = b.hbKey
	}
	seq := b.nextSeq()
	env := signed(revKey, string(wireJSON(b.t, post)))
	msg := MessageRow[ReviewBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, seq}, Pos: seq, Group: b.g, Thread: &tidStr,
		By: b.mY, Kind: "review", Timestamp: at, Text: text, Mentions: []string{},
		Body: ReviewBody{Task: b.t1, Num: 12, Title: traceTitle, Commit: traceCommit, Round: 1, Score: 93, Pass: 90, OK: true, Items: items},
		Env:  &env,
	}
	b.task.Step = "merge"
	b.task.Review = &Review{Round: 1, Failed: 0, Score: 93, OK: true, By: b.mY, Commit: traceCommit, At: at}
	moved := b.advanceThread(b.mY, "review", text, at)
	mrg := b.nextItem(b.mX, "merge")
	mrg.Task, mrg.Commit, mrg.PR = ID(b.t1), traceCommit, b.task.PR
	taskCopy := moved
	b.send(revKey, post, CmdResult{Passed: new(true), Round: 1}, []any{msg, moved}, b.deliver(b.ha, InboxItem{Item: mrg, Task: &taskCopy}))
}

func (b *traceBuilder) stepMerged() {
	at := ts + 270000
	msgID := id("trace merged")
	tidStr := b.t1
	cmd := TaskMerged{
		TaskCommand: TaskCommand{HomeCommand{"task", ID(msgID), at, ID(b.g), ""}, "merged", ID(b.t1)},
		Commit:      traceCommit, MergeCommit: traceMergeCommit, Stats: DiffStats{Added: 84, Removed: 12},
	}
	b.task.State, b.task.Stats.Ended = "merged", &at
	moved := b.advanceThread("system", "system", "", at)
	msgSeq := b.nextSeq()
	sysMsg := MessageRow[TaskMergedBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, msgSeq}, Pos: msgSeq, Group: b.g, Thread: &tidStr,
		By: "system", Kind: "system", Timestamp: at, Mentions: []string{},
		Body: TaskMergedBody{SystemCode: SystemCode{"task_merged"}, Task: b.t1, Commit: traceCommit, MergeCommit: traceMergeCommit},
	}
	notice := b.nextItem(b.mS, "notice")
	notice.Task, notice.Msg, notice.Code = ID(b.t1), ID(msgID), "task_merged"
	taskCopy := moved
	b.send(b.haKey, cmd, CmdResult{}, []any{moved, sysMsg}, b.deliver(b.ha, InboxItem{Item: notice, Msg: wireJSON(b.t, sysMsg), Task: &taskCopy}))
}

func (b *traceBuilder) stepSummary() {
	at := ts + 280000
	msgID := id("trace summary")
	tid, tidStr := ID(b.t1), b.t1
	text := "Path changes are reported with the active transport kind and round-trip time."
	post := Post[EmptyBody]{
		HomeCommand: HomeCommand{Type: "post", Nonce: ID(msgID), Timestamp: at, Group: ID(b.g), As: ID(b.mS)},
		Kind:        "summary", Thread: &tid, Main: true, Text: text, Mentions: []ID{}, Body: EmptyBody{},
	}
	var pr *int
	if b.remote {
		pr = new(tracePR)
	}
	seq := b.nextSeq()
	env := signed(b.haKey, string(wireJSON(b.t, post)))
	msg := MessageRow[SummaryBody, NoCard]{
		RowHeader: RowHeader{"msg", msgID, seq}, Pos: seq, Group: b.g, Thread: &tidStr, Main: true,
		By: b.mS, Kind: "summary", Timestamp: at, Text: text, Mentions: []string{},
		Body: SummaryBody{Task: b.t1, Score: 93, Rounds: 1, Minutes: 4, Added: 84, Removed: 12, PR: pr},
		Env:  &env,
	}
	b.send(b.haKey, post, CmdResult{}, []any{msg, b.advanceThread(b.mS, "summary", text, at)})
}

func (b *traceBuilder) build() TaskTrace {
	b.stepText()
	b.stepBrief()
	b.stepConfirm()
	b.stepBranch()
	b.stepProgress()
	b.stepChecked()
	b.stepReport()
	b.stepSubmitted()
	if b.remote {
		b.stepCloud()
	}
	b.stepReview()
	b.stepMerged()
	b.stepSummary()
	return TaskTrace{
		Keys:   b.keys(),
		Devlog: b.devlog(),
		Group:  b.groupRow(),
		Owner: HumanMemberRow{
			MemberRow: MemberRow{RowHeader{"member", b.mO, 797}, b.g, "human", b.ob, "owner", ts - 10000},
			Role:      "owner", Online: true,
		},
		AIs:   b.ais(),
		Seq:   800,
		Num:   11,
		Steps: b.steps,
	}
}

func TestTaskTraceGoldens(t *testing.T) {
	golden(t, "trace-remote", newTraceBuilder(t, true).build())
	golden(t, "trace-local", newTraceBuilder(t, false).build())
}

var traceCmdShapes = map[string]reflect.Type{
	"post/text":      of[Post[EmptyBody]](),
	"post/brief":     of[Post[BriefBody]](),
	"post/progress":  of[Post[EmptyBody]](),
	"post/report":    of[Post[ReportBody]](),
	"post/review":    of[Post[ReviewSigned]](),
	"post/summary":   of[Post[EmptyBody]](),
	"act/confirm":    of[Act](),
	"task/branch":    of[TaskBranch](),
	"task/checked":   of[TaskChecked](),
	"task/submitted": of[TaskSubmitted](),
	"task/cloud":     of[TaskChecked](),
	"task/merged":    of[TaskMerged](),
}

var traceMsgShapes = map[string]reflect.Type{
	"text":            plain[EmptyBody](),
	"brief/pending":   card[BriefBody, StatusOnly](),
	"brief/confirmed": card[BriefBody, ActedState](),
	"task":            plain[TaskBody](),
	"progress":        plain[EmptyBody](),
	"checks":          plain[ChecksBody](),
	"report":          plain[ReportBody](),
	"review":          plain[ReviewBody](),
	"system":          plain[TaskMergedBody](),
	"summary":         plain[SummaryBody](),
}

func verifyEnvSig(t *testing.T, pubs map[string]ed25519.PublicKey, env Envelope) {
	t.Helper()
	pub := pubs[env.Key]
	sig, err := base64.RawURLEncoding.DecodeString(env.Signature)
	if err != nil || len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, []byte("solodawn/1\n"+env.Payload), sig) {
		t.Fatalf("signature check failed for signer %s", env.Key)
	}
}

func verifyTraceKeys(t *testing.T, keys []TraceKey) map[string]ed25519.PublicKey {
	t.Helper()
	pubs := map[string]ed25519.PublicKey{}
	for _, k := range keys {
		seed, err := base64.RawURLEncoding.DecodeString(k.Seed)
		if err != nil || len(seed) != ed25519.SeedSize {
			t.Fatalf("invalid seed for %s", k.Name)
		}
		priv := ed25519.NewKeyFromSeed(seed)
		if device(priv) != k.Device {
			t.Fatalf("key %s derived %s, want %s", k.Name, device(priv), k.Device)
		}
		pubs[k.Device] = priv.Public().(ed25519.PublicKey)
	}
	return pubs
}

func verifyTraceDevlog(t *testing.T, log Devlog, pubs map[string]ed25519.PublicKey) map[string]bool {
	t.Helper()
	valid := map[string]bool{}
	for i, env := range log.Entries {
		valid[verifyOneLogEntry(t, env, log.User, i+1, pubs, valid)] = true
	}
	return valid
}

func verifyOneLogEntry(t *testing.T, env Envelope, user string, ver int, pubs map[string]ed25519.PublicKey, valid map[string]bool) string {
	t.Helper()
	verifyEnvSig(t, pubs, env)
	var entry DeviceEntry
	if err := Decode([]byte(env.Payload), &entry); err != nil {
		t.Fatal(err)
	}
	if [3]any{entry.Version, entry.User, entry.Op} != [3]any{ver, user, "add"} {
		t.Fatalf("unexpected devlog entry %d: %+v", ver, entry)
	}
	checkLogEntrySigner(t, ver, user, env.Key, entry.Device, valid)
	return entry.Device
}

func checkLogEntrySigner(t *testing.T, ver int, user, signer, added string, valid map[string]bool) {
	t.Helper()
	if ver == 1 {
		if [2]string{signer, added} != [2]string{user, user} {
			t.Fatalf("genesis entry mismatch: signer=%s added=%s", signer, added)
		}
		return
	}
	if !valid[signer] {
		t.Fatalf("entry %d signed by unenrolled device %s", ver, signer)
	}
}

func verifyTraceCast(t *testing.T, tr TaskTrace, valid map[string]bool, remote bool) map[string]AIMemberRow {
	t.Helper()
	wantForge, wantKeys := "none", 2
	if remote {
		wantForge, wantKeys = "github", 3
	}
	if [3]any{tr.Group.Forge, len(tr.Keys), len(valid)} != [3]any{wantForge, wantKeys, wantKeys} {
		t.Fatalf("unexpected cast for remote=%v: forge=%s keys=%d", remote, tr.Group.Forge, len(tr.Keys))
	}
	if tr.Owner.User != tr.Devlog.User || !valid[tr.Group.Home] {
		t.Fatalf("owner or home not enrolled in devlog")
	}
	return indexTraceAIs(t, tr.AIs, valid)
}

func indexTraceAIs(t *testing.T, list []AIMemberRow, valid map[string]bool) map[string]AIMemberRow {
	t.Helper()
	ais := map[string]AIMemberRow{}
	for _, ai := range list {
		if !valid[ai.Host] {
			t.Fatalf("AI %s hosted on unknown device %s", ai.ID, ai.Host)
		}
		ais[ai.ID] = ai
	}
	return ais
}

func traceCmdKey(raw string) (string, HomeCommand) {
	var probe struct {
		HomeCommand
		Op     string `json:"op,omitempty"`
		Kind   string `json:"kind,omitempty"`
		Action string `json:"action,omitempty"`
	}
	if err := Decode([]byte(raw), &probe); err != nil {
		return "", HomeCommand{}
	}
	return probe.Type + "/" + probe.Op + probe.Kind + probe.Action, probe.HomeCommand
}

func verifyStepCommand(t *testing.T, step TraceStep, tr TaskTrace, pubs map[string]ed25519.PublicKey, valid map[string]bool, ais map[string]AIMemberRow) string {
	t.Helper()
	if [3]any{valid[step.From], step.Cmd.Env.Key, step.Answer.Re} != [3]any{true, step.From, step.Cmd.ID} {
		t.Fatalf("command framing mismatch at request %d", step.Cmd.ID)
	}
	verifyEnvSig(t, pubs, step.Cmd.Env)
	key, hdr := traceCmdKey(step.Cmd.Env.Payload)
	shape := traceCmdShapes[key]
	if shape == nil || string(hdr.Group) != tr.Group.ID {
		t.Fatalf("unknown or misrouted command %q", key)
	}
	if err := Decode([]byte(step.Cmd.Env.Payload), reflect.New(shape).Interface()); err != nil {
		t.Fatalf("command %s decode failed: %v", key, err)
	}
	verifyCommandPrincipal(t, hdr, step.From, tr.Owner.User, tr.Group.Home, ais)
	return key
}

func verifyCommandPrincipal(t *testing.T, hdr HomeCommand, signer, ownerDev, homeDev string, ais map[string]AIMemberRow) {
	t.Helper()
	if hdr.As != "" {
		if ais[string(hdr.As)].Host != signer {
			t.Fatalf("AI %s may not be signed by %s", hdr.As, signer)
		}
		return
	}
	if signer != ownerDev && signer != homeDev {
		t.Fatalf("unexpected bare signer %s", signer)
	}
}

type traceAuditState struct {
	seq        int64
	confirmEnv Envelope
	msgs       map[string]json.RawMessage
	tasks      map[string]TaskRow
	whys       []string
	nextItem   int64
}

func verifyStepRows(t *testing.T, step TraceStep, key string, pubs map[string]ed25519.PublicKey, st *traceAuditState) {
	t.Helper()
	if key == "act/confirm" {
		st.confirmEnv = step.Cmd.Env
	}
	for _, raw := range step.Rows.Rows {
		st.seq++
		var hdr RowHeader
		if err := Decode(raw, &hdr); err != nil || hdr.Seq != st.seq {
			t.Fatalf("row seq = %d (%v), want %d", hdr.Seq, err, st.seq)
		}
		verifyOneTraceRow(t, hdr.Type, raw, step.Cmd.Env, pubs, st)
	}
}

func verifyOneTraceRow(t *testing.T, rowType string, raw json.RawMessage, cmdEnv Envelope, pubs map[string]ed25519.PublicKey, st *traceAuditState) {
	t.Helper()
	if rowType == "task" {
		var task TaskRow
		if err := Decode(raw, &task); err != nil {
			t.Fatal(err)
		}
		st.tasks[task.ID] = task
		return
	}
	var msg MsgRow
	if err := Decode(raw, &msg); err != nil {
		t.Fatal(err)
	}
	verifyTraceMsgRow(t, msg, raw, cmdEnv, pubs, st.confirmEnv)
	st.msgs[msg.ID] = raw
}

func verifyTraceMsgRow(t *testing.T, msg MsgRow, raw json.RawMessage, cmdEnv Envelope, pubs map[string]ed25519.PublicKey, confirmEnv Envelope) {
	t.Helper()
	key := msg.Kind
	if msg.State != nil {
		key += "/" + msg.State.Status
	}
	shape := traceMsgShapes[key]
	if shape == nil || Decode(raw, reflect.New(shape).Interface()) != nil {
		t.Fatalf("message row %s failed strict shape check", key)
	}
	verifyAuthorEnvelope(t, msg, cmdEnv, pubs)
	verifyConfirmedBriefAct(t, key, msg, confirmEnv)
}

func verifyAuthorEnvelope(t *testing.T, msg MsgRow, cmdEnv Envelope, pubs map[string]ed25519.PublicKey) {
	t.Helper()
	if msg.Env != nil {
		verifyEnvSig(t, pubs, *msg.Env)
	}
	if [2]bool{msg.By != "system", msg.State == nil} != [2]bool{true, true} {
		return
	}
	if msg.Env == nil || *msg.Env != cmdEnv {
		t.Fatalf("signed message %s lost author envelope", msg.ID)
	}
}

func verifyConfirmedBriefAct(t *testing.T, key string, msg MsgRow, confirmEnv Envelope) {
	t.Helper()
	if key != "brief/confirmed" {
		return
	}
	if msg.State.Act == nil || *msg.State.Act != confirmEnv {
		t.Fatalf("confirmed brief did not retain exact confirm envelope")
	}
}

func verifyStepInboxes(t *testing.T, inboxes []TraceInbox, group string, ais map[string]AIMemberRow, st *traceAuditState) {
	t.Helper()
	for _, delivery := range inboxes {
		if string(delivery.Inbox.Group) != group || len(delivery.Acks) != len(delivery.Inbox.Items) {
			t.Fatalf("inbox delivery framing mismatch for %s", delivery.To)
		}
		for i, entry := range delivery.Inbox.Items {
			st.nextItem++
			verifyOneInboxItem(t, delivery.To, entry, delivery.Acks[i], group, ais, st)
			st.whys = append(st.whys, entry.Item.Why)
		}
	}
}

func verifyOneInboxItem(t *testing.T, to string, entry InboxItem, ack Ack, group string, ais map[string]AIMemberRow, st *traceAuditState) {
	t.Helper()
	wantAck := Ack{Type: "ack", Group: ID(group), Item: st.nextItem}
	if [3]any{ais[string(entry.Item.AI)].Host, entry.Item.Item, ack} != [3]any{to, st.nextItem, wantAck} {
		t.Fatalf("inbox item %d routing or ack mismatch", st.nextItem)
	}
	verifyInboxRowCopies(t, entry, st)
}

func verifyInboxRowCopies(t *testing.T, entry InboxItem, st *traceAuditState) {
	t.Helper()
	if entry.Item.Task != "" {
		if entry.Task == nil || !reflect.DeepEqual(*entry.Task, st.tasks[string(entry.Item.Task)]) {
			t.Fatalf("inbox item %d task copy mismatch", st.nextItem)
		}
	}
	if entry.Item.Msg != "" {
		if !reflect.DeepEqual(jsonValue(t, entry.Msg), jsonValue(t, st.msgs[string(entry.Item.Msg)])) {
			t.Fatalf("inbox item %d msg copy mismatch", st.nextItem)
		}
	}
}

func verifyFinalTraceOutcome(t *testing.T, tr TaskTrace, st *traceAuditState, remote bool) {
	t.Helper()
	wantWhys := []string{"mention", "assign", "confirmed", "review", "merge", "notice"}
	if !reflect.DeepEqual(st.whys, wantWhys) || len(st.tasks) != 1 {
		t.Fatalf("items = %v, tasks = %d", st.whys, len(st.tasks))
	}
	for _, task := range st.tasks {
		verifyFinalTask(t, task, remote)
	}
	if len(tr.Steps) != map[bool]int{true: 12, false: 11}[remote] {
		t.Fatalf("unexpected step count %d for remote=%v", len(tr.Steps), remote)
	}
}

func verifyFinalTask(t *testing.T, task TaskRow, remote bool) {
	t.Helper()
	if [3]string{task.State, task.Step, task.Commit} != [3]string{"merged", "merge", traceCommit} || task.Stats.Ended == nil {
		t.Fatalf("final task not merged cleanly: %+v", task)
	}
	if [2]bool{task.PR != 0, task.Checks.Cloud != nil} != [2]bool{remote, remote} {
		t.Fatalf("remote fields mismatch on final task (remote=%v): %+v", remote, task)
	}
}

func verifyTaskTrace(t *testing.T, name string, remote bool) {
	t.Helper()
	var tr TaskTrace
	if err := Decode(example(t, name), &tr); err != nil {
		t.Fatal(err)
	}
	pubs := verifyTraceKeys(t, tr.Keys)
	valid := verifyTraceDevlog(t, tr.Devlog, pubs)
	ais := verifyTraceCast(t, tr, valid, remote)
	st := &traceAuditState{
		seq:   tr.Seq,
		msgs:  map[string]json.RawMessage{},
		tasks: map[string]TaskRow{},
	}
	for _, step := range tr.Steps {
		key := verifyStepCommand(t, step, tr, pubs, valid, ais)
		verifyStepRows(t, step, key, pubs, st)
		verifyStepInboxes(t, step.Inboxes, tr.Group.ID, ais, st)
	}
	verifyFinalTraceOutcome(t, tr, st, remote)
}

func TestTaskTracesVerifyAlone(t *testing.T) {
	t.Run("remote", func(t *testing.T) { verifyTaskTrace(t, "trace-remote", true) })
	t.Run("local", func(t *testing.T) { verifyTaskTrace(t, "trace-local", false) })
}
