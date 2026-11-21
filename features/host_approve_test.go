//go:build acceptance

package features

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_approve"] = hostApproveSteps
}

type approveAccHome struct {
	mu            sync.Mutex
	done          chan struct{}
	approvalPosts []proto.Post[proto.ApprovalBody]
	decisionPosts []proto.Post[proto.DecisionBody]
	cards         map[string]proto.MsgRow
	onPost        func(card proto.ID)
}

func newApproveAccHome() *approveAccHome {
	return &approveAccHome{
		// Closed on disconnect; read via Done().
		done:  make(chan struct{}),
		cards: map[string]proto.MsgRow{},
	}
}

func (f *approveAccHome) Send(any) error               { return nil }
func (f *approveAccHome) Path() string                 { return "local" }
func (f *approveAccHome) PathChanged() <-chan struct{} { return nil }
func (f *approveAccHome) Done() <-chan struct{}        { return f.done }

func (f *approveAccHome) Request(_ context.Context, build func(int64) any) (json.RawMessage, error) {
	f.mu.Lock()
	id := int64(len(f.approvalPosts) + len(f.decisionPosts) + 1)
	req := build(id)
	if fetch, ok := req.(proto.FetchRow); ok {
		ans := f.buildFetchResponse(id, fetch.RowID)
		f.mu.Unlock()
		return ans, nil
	}
	postedID := f.recordCmdLocked(req)
	hook := f.onPost
	f.mu.Unlock()
	if hook != nil && postedID != "" {
		hook(postedID)
	}
	return json.RawMessage(fmt.Sprintf(`{"re":%d,"ok":true}`, id)), nil
}

func (f *approveAccHome) buildFetchResponse(id int64, rowID string) json.RawMessage {
	rows := []json.RawMessage{}
	if row, exists := f.cards[rowID]; exists {
		rows = []json.RawMessage{json.RawMessage(accMarshal(row))}
	}
	return json.RawMessage(accMarshal(proto.Fetched{
		Type:    "fetched",
		Success: proto.Success{Re: id, OK: true},
		Rows:    rows,
	}))
}

func (f *approveAccHome) recordCmdLocked(req any) proto.ID {
	cmd, ok := req.(proto.Cmd)
	if !ok {
		return ""
	}
	var hdr struct {
		Kind string   `json:"kind"`
		ID   proto.ID `json:"n"`
	}
	json.Unmarshal([]byte(cmd.Env.Payload), &hdr)
	if hdr.Kind != "approval" && hdr.Kind != "decision" {
		return ""
	}
	f.appendPostLocked(hdr.Kind, []byte(cmd.Env.Payload))
	return hdr.ID
}

func (f *approveAccHome) appendPostLocked(kind string, payload []byte) {
	switch kind {
	case "approval":
		var post proto.Post[proto.ApprovalBody]
		if proto.Decode(payload, &post) == nil {
			f.approvalPosts = append(f.approvalPosts, post)
		}
	case "decision":
		var post proto.Post[proto.DecisionBody]
		if proto.Decode(payload, &post) == nil {
			f.decisionPosts = append(f.decisionPosts, post)
		}
	}
}

func (f *approveAccHome) totalPostCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.approvalPosts) + len(f.decisionPosts)
}

func (f *approveAccHome) lastApprovalPost() (proto.Post[proto.ApprovalBody], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.approvalPosts) == 0 {
		return proto.Post[proto.ApprovalBody]{}, false
	}
	return f.approvalPosts[len(f.approvalPosts)-1], true
}

func (f *approveAccHome) lastDecisionPost() (proto.Post[proto.DecisionBody], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.decisionPosts) == 0 {
		return proto.Post[proto.DecisionBody]{}, false
	}
	return f.decisionPosts[len(f.decisionPosts)-1], true
}

func (f *approveAccHome) putCard(card proto.MsgRow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cards[card.ID] = card
}

type approveAccRunner struct {
	ch chan bool
}

func (r *approveAccRunner) Turn(context.Context, *host.Host, host.SessionKey, string, host.Turn) error {
	return nil
}

func (r *approveAccRunner) Answer(_ *host.Host, _ host.SessionKey, _ string, allow bool) error {
	r.ch <- allow
	return nil
}

func accSeedKey(b byte) identity.Key {
	return identity.Key(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize)))
}

func accMakeID(b byte) proto.ID {
	return proto.ID(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{b}, 16)))
}

func accMarshal(v any) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

func parseColonPairs(a, b string) []map[string]string {
	var out []map[string]string
	for _, raw := range []string{a, b} {
		label, desc, _ := strings.Cut(raw, ":")
		out = append(out, map[string]string{"label": label, "description": desc})
	}
	return out
}

type approveAccWorld struct {
	cleanup       func(func())
	owner1        identity.Key
	owner2        identity.Key
	homeDev       identity.Key
	hostDev       identity.Key
	groupID       string
	memberID      string
	taskID        string
	humanID       string
	db            *store.Store
	h             *host.Host
	fakeHome      *approveAccHome
	runner        *approveAccRunner
	nowMS         atomic.Int64
	nextItemID    int64
	sessKey       host.SessionKey
	cliName       string
	waitOutcome   chan host.ApprovalResult
	codexReplyCh  chan map[string][]string
	lastResult    host.ApprovalResult
	lastCodexAns  map[string][]string
	postsAtBefore int
}

func newApproveAccWorld(cleanup func(func())) *approveAccWorld {
	w := &approveAccWorld{
		cleanup:    cleanup,
		owner1:     accSeedKey(11),
		owner2:     accSeedKey(12),
		homeDev:    accSeedKey(13),
		hostDev:    accSeedKey(14),
		groupID:    string(accMakeID(21)),
		memberID:   string(accMakeID(22)),
		taskID:     string(accMakeID(23)),
		humanID:    string(accMakeID(24)),
		nextItemID: 500,
	}
	w.nowMS.Store(1_763_600_000_000)
	return w
}

func (w *approveAccWorld) clock() time.Time {
	return time.UnixMilli(w.nowMS.Add(500))
}

func (w *approveAccWorld) seedGroupRows(target *host.Host, waitS int, secretary bool) {
	grp := proto.GroupRow{
		RowHeader: proto.RowHeader{Type: "group", ID: w.groupID, Seq: 1},
		Name:      "Website", Kind: "project", Base: "main", Forge: "none",
		Home: w.homeDev.ID(), Checks: []string{}, Limits: proto.Limits{ApprovalWaitS: waitS},
	}
	hum := proto.HumanMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.humanID, Seq: 2}, Group: w.groupID, Kind: "human", User: w.owner1.ID(), Name: "Owner"},
		Role:      "owner",
	}
	ai := proto.AIMemberRow{
		MemberRow: proto.MemberRow{RowHeader: proto.RowHeader{Type: "member", ID: w.memberID, Seq: 3}, Group: w.groupID, Kind: "ai", User: w.owner1.ID(), Name: "Worker"},
		CLI:       w.cliName, Host: w.hostDev.ID(), Secretary: secretary, Account: "personal",
		Allow: []string{}, Approvers: []string{w.owner1.ID()}, State: "working",
	}
	target.Rows.Put(w.groupID, "group", w.groupID, json.RawMessage(accMarshal(grp)))
	target.Rows.Put(w.groupID, "member", w.humanID, json.RawMessage(accMarshal(hum)))
	target.Rows.Put(w.groupID, "member", w.memberID, json.RawMessage(accMarshal(ai)))
}

func (w *approveAccWorld) buildCardRow(card proto.ID, kind string, state proto.CardState) proto.MsgRow {
	var thread *string
	if w.sessKey.Task != "" {
		t := w.sessKey.Task
		thread = &t
	}
	return proto.MsgRow{
		RowHeader: proto.RowHeader{Type: "msg", ID: string(card), Seq: 10},
		Pos:       10, Group: w.groupID, Thread: thread, Main: true, By: w.memberID,
		Kind: kind, Timestamp: w.nowMS.Load(), Mentions: []string{},
		Body: json.RawMessage(`{}`), State: &state,
	}
}

func (w *approveAccWorld) buildDecidedInbox(card proto.ID, kind string, state proto.CardState) json.RawMessage {
	w.nextItemID++
	msgRow := w.buildCardRow(card, kind, state)
	return json.RawMessage(accMarshal(proto.InboxItem{
		Item: proto.Item{
			Item: w.nextItemID,
			AI:   proto.ID(w.memberID),
			Why:  "decided",
			Task: proto.ID(w.sessKey.Task),
			Msg:  card,
		},
		Msg: json.RawMessage(accMarshal(msgRow)),
	}))
}

func (w *approveAccWorld) buildApprovalDecidedRaw(signer identity.Key, card proto.ID, status string) json.RawMessage {
	actEnv := signer.Sign(`{"t":"act"}`)
	by := w.humanID
	at := w.nowMS.Load()
	return w.buildDecidedInbox(card, "approval", proto.CardState{Status: status, By: &by, At: &at, Act: &actEnv})
}

func (w *approveAccWorld) buildDecisionDecidedRaw(signer identity.Key, card proto.ID, choice, text *string) json.RawMessage {
	actEnv := signer.Sign(`{"t":"act"}`)
	by := w.humanID
	at := w.nowMS.Load()
	return w.buildDecidedInbox(card, "decision", proto.CardState{
		Status: "decided", By: &by, At: &at, Choice: choice, Text: text, Act: &actEnv,
	})
}

func parseSessionKind(name string) (host.Kind, string, bool) {
	switch name {
	case "group":
		return host.GroupSession, "repo", true
	case "review":
		return host.ReviewSession, "review/1", false
	default:
		return host.TaskSession, "wt/1-test", false
	}
}

func (w *approveAccWorld) setupHost(cli, sessionName string, waitS int) error {
	var err error
	w.cliName = cli
	w.db, err = store.Open(":memory:")
	if err != nil {
		return err
	}
	opened := w.db
	w.cleanup(func() { opened.Close() })
	seedOwner := func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT INTO setting(key, value) VALUES('owner', ?)", accMarshal(w.owner1.ID()))
		return err
	}
	if err := w.db.Write(seedOwner); err != nil {
		return err
	}
	w.installFakeRunner()
	return w.initHostAndSession(cli, sessionName, waitS)
}

func (w *approveAccWorld) installFakeRunner() {
	prevRunner := host.Runner
	// Single reader: startCodexApprovalAsk goroutine awaiting Runner.Answer.
	w.runner = &approveAccRunner{ch: make(chan bool, 4)}
	host.Runner = w.runner
	w.cleanup(func() { host.Runner = prevRunner })
}

func (w *approveAccWorld) initHostAndSession(cli, sessionName string, waitS int) error {
	var err error
	w.h, _, err = host.New(w.db, w.hostDev, w.clock)
	if err != nil {
		return err
	}
	w.fakeHome = newApproveAccHome()
	w.h.Attach(w.homeDev.ID(), w.fakeHome)
	kind, folder, secretary := parseSessionKind(sessionName)
	w.seedGroupRows(w.h, waitS, secretary)
	task := w.taskID
	if kind == host.GroupSession {
		task = ""
	}
	w.sessKey = host.SessionKey{Group: w.groupID, Member: w.memberID, Task: task}
	return w.h.Sessions.Create(host.Session{
		SessionKey: w.sessKey, Kind: kind, Folder: folder, Agent: cli,
		Home: w.homeDev.ID(), Name: "Worker", Secretary: secretary,
	})
}

func (w *approveAccWorld) armPostSignal() <-chan proto.ID {
	// Single reader: awaitPostedCard waiting for Home.Request post notification.
	posted := make(chan proto.ID, 1)
	w.fakeHome.mu.Lock()
	w.fakeHome.onPost = func(card proto.ID) {
		select {
		case posted <- card:
		default:
		}
	}
	w.fakeHome.mu.Unlock()
	return posted
}

func awaitPostedCard(posted <-chan proto.ID) error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-posted:
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out waiting for card post")
	}
}

func (w *approveAccWorld) startAsyncAsk(cli, cmd, why string, level int) error {
	posted := w.armPostSignal()
	// Single reader: collectResult waiting for outcome.
	w.waitOutcome = make(chan host.ApprovalResult, 1)
	if cli == "claude" {
		w.startClaudeApprovalAsk(cmd, why, level)
	} else {
		w.startCodexApprovalAsk(cmd, why)
	}
	return awaitPostedCard(posted)
}

func (w *approveAccWorld) startClaudeApprovalAsk(cmd, why string, level int) {
	s, _ := w.h.Sessions.Get(w.sessKey)
	caller := host.Caller{Session: s, Level: level}
	args := accMarshal(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": cmd, "description": why},
		"tool_use_id": "toolu_acc",
	})
	go func() {
		res, _ := host.Tools["approve"].Call(context.Background(), w.h, caller, json.RawMessage(args))
		w.waitOutcome <- res.(host.ApprovalResult)
	}()
}

func (w *approveAccWorld) startCodexApprovalAsk(cmd, why string) {
	input := accMarshal(map[string]string{"command": cmd, "cwd": "/repo"})
	w.h.Report(w.sessKey, host.Event{
		Kind: host.Asked,
		Ask:  &host.Ask{ID: "ask-1", Tool: "commandExecution", Category: "command", Command: cmd, Why: why, Input: json.RawMessage(input)},
	})
	go func() {
		if <-w.runner.ch {
			w.waitOutcome <- host.ApprovalResult{Behavior: "allow"}
			return
		}
		w.waitOutcome <- host.ApprovalResult{Behavior: "deny", Message: "Denied by the owner."}
	}()
}

func (w *approveAccWorld) collectResult() error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case w.lastResult = <-w.waitOutcome:
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out waiting for approval result")
	}
}

func (w *approveAccWorld) callSyncBash(cmd, why string, level int) error {
	s, _ := w.h.Sessions.Get(w.sessKey)
	args := accMarshal(map[string]any{
		"tool_name":   "Bash",
		"input":       map[string]string{"command": cmd, "description": why},
		"tool_use_id": "toolu_sync",
	})
	res, err := host.Tools["approve"].Call(context.Background(), w.h, host.Caller{Session: s, Level: level}, json.RawMessage(args))
	if err != nil {
		return err
	}
	w.lastResult = res.(host.ApprovalResult)
	return nil
}

func (w *approveAccWorld) stepAskRun(cli, cmd, why string) error {
	if strings.Contains(cmd, "ghp_") {
		return w.callSyncBash(cmd, why, 3)
	}
	return w.startAsyncAsk(cli, cmd, why, 3)
}

func (w *approveAccWorld) stepBrowserApproval(verb string) error {
	post, ok := w.fakeHome.lastApprovalPost()
	if !ok {
		return fmt.Errorf("no posted approval card")
	}
	status := "approved"
	if verb == "denies" {
		status = "denied"
	}
	waited, err := w.h.Hand(string(post.Nonce), w.buildApprovalDecidedRaw(w.owner2, post.Nonce, status))
	if !waited || err != nil {
		return fmt.Errorf("Hand waited=%v err=%v", waited, err)
	}
	return w.collectResult()
}

func (w *approveAccWorld) stepAssertOutcome(want string) error {
	if want == "allow" {
		return w.assertAllowResult()
	}
	return w.assertDenyResult("Denied by the owner.")
}

func (w *approveAccWorld) assertAllowResult() error {
	if w.lastResult.Behavior != "allow" || w.lastResult.Message != "" || w.lastResult.UpdatedInput != nil {
		return fmt.Errorf("got %+v, want allow", w.lastResult)
	}
	return nil
}

func (w *approveAccWorld) assertDenyResult(wantMsg string) error {
	if w.lastResult.Behavior != "deny" || w.lastResult.Message != wantMsg {
		return fmt.Errorf("got %+v, want deny %q", w.lastResult, wantMsg)
	}
	return nil
}

func (w *approveAccWorld) stepAssertApprovalCard(sessionName, wantCmd, wantWhy string) error {
	post, ok := w.fakeHome.lastApprovalPost()
	if !ok {
		return fmt.Errorf("no posted approval card")
	}
	if err := w.assertApprovalThread(sessionName, post); err != nil {
		return err
	}
	if post.Body.Command != wantCmd || post.Body.Why == nil || *post.Body.Why != wantWhy {
		return fmt.Errorf("got body %+v", post.Body)
	}
	return nil
}

func (w *approveAccWorld) assertApprovalThread(sessionName string, post proto.Post[proto.ApprovalBody]) error {
	gotThread := "<nil>"
	if post.Thread != nil {
		gotThread = string(*post.Thread)
	}
	wantThread, wantTask := w.taskID, w.taskID
	if sessionName == "group" {
		wantThread, wantTask = "<nil>", ""
	}
	if gotThread != wantThread || post.Body.Task != wantTask {
		return fmt.Errorf("%s approval had thread=%s task=%q", sessionName, gotThread, post.Body.Task)
	}
	return nil
}

func (w *approveAccWorld) stepClaudeAskQuestion(header, question, optA, optB string) error {
	posted := w.armPostSignal()
	// Single reader: collectResult waiting for outcome.
	w.waitOutcome = make(chan host.ApprovalResult, 1)
	s, _ := w.h.Sessions.Get(w.sessKey)
	args := accMarshal(map[string]any{
		"tool_name": "AskUserQuestion",
		"input": map[string]any{
			"questions": []map[string]any{
				{"header": header, "question": question, "options": parseColonPairs(optA, optB), "multiSelect": false},
			},
		},
		"tool_use_id": "toolu_ask",
	})
	go func() {
		res, _ := host.Tools["approve"].Call(context.Background(), w.h, host.Caller{Session: s, Level: 3}, json.RawMessage(args))
		w.waitOutcome <- res.(host.ApprovalResult)
	}()
	return awaitPostedCard(posted)
}

func (w *approveAccWorld) stepCodexAskQuestion(qID, header, question, optA, optB string) error {
	posted := w.armPostSignal()
	// Single reader: handDecisionAnswer reading Codex Reply result.
	w.codexReplyCh = make(chan map[string][]string, 1)
	input := accMarshal(map[string]any{
		"questions": []map[string]any{
			{"id": qID, "header": header, "question": question, "isSecret": false, "options": parseColonPairs(optA, optB)},
		},
	})
	replyFn := func(ans map[string][]string) error {
		w.codexReplyCh <- ans
		return nil
	}
	w.h.Report(w.sessKey, host.Event{
		Kind: host.Asked,
		Ask:  &host.Ask{ID: "req-" + qID, Tool: "requestUserInput", Input: json.RawMessage(input), Reply: replyFn},
	})
	return awaitPostedCard(posted)
}

func (w *approveAccWorld) handDecisionAnswer(choice, text *string) error {
	post, ok := w.fakeHome.lastDecisionPost()
	if !ok {
		return fmt.Errorf("no posted decision card")
	}
	waited, err := w.h.Hand(string(post.Nonce), w.buildDecisionDecidedRaw(w.owner2, post.Nonce, choice, text))
	if !waited || err != nil {
		return fmt.Errorf("Hand waited=%v err=%v", waited, err)
	}
	if w.cliName == "claude" {
		return w.collectResult()
	}
	w.lastCodexAns = <-w.codexReplyCh
	return nil
}

func (w *approveAccWorld) stepChooseOption(optID, note string) error {
	choice := optID
	var textPtr *string
	if note != "" {
		n := note
		textPtr = &n
	}
	return w.handDecisionAnswer(&choice, textPtr)
}

func (w *approveAccWorld) stepCustomText(custom string) error {
	txt := custom
	return w.handDecisionAnswer(nil, &txt)
}

func assertTwoOptions(opts []proto.DecisionOption, wantA, wantB string) error {
	if len(opts) != 2 {
		return fmt.Errorf("got %d options, want 2", len(opts))
	}
	gotA := opts[0].ID + ":" + opts[0].Label
	gotB := opts[1].ID + ":" + opts[1].Label
	if gotA != wantA || gotB != wantB {
		return fmt.Errorf("got options %q and %q, want %q and %q", gotA, gotB, wantA, wantB)
	}
	return nil
}

func (w *approveAccWorld) stepAssertDecisionWithContext(wantA, wantB, wantCtx string) error {
	post, ok := w.fakeHome.lastDecisionPost()
	if !ok {
		return fmt.Errorf("no posted decision card")
	}
	if post.Body.Context == nil || *post.Body.Context != wantCtx {
		return fmt.Errorf("unexpected context %v", post.Body.Context)
	}
	return assertTwoOptions(post.Body.Options, wantA, wantB)
}

func (w *approveAccWorld) stepAssertDecisionNoThread(wantA, wantB string) error {
	post, ok := w.fakeHome.lastDecisionPost()
	if !ok {
		return fmt.Errorf("no posted decision card")
	}
	if post.Thread != nil || post.Body.Task != nil {
		return fmt.Errorf("expected nil thread/task, got thread=%v task=%v", post.Thread, post.Body.Task)
	}
	return assertTwoOptions(post.Body.Options, wantA, wantB)
}

func (w *approveAccWorld) stepAssertClaudeAnswer(wantAns, question, wantNote string) error {
	if w.lastResult.Behavior != "allow" || w.lastResult.UpdatedInput == nil {
		return fmt.Errorf("got %+v, want allow with updatedInput", w.lastResult)
	}
	ansMap, _ := w.lastResult.UpdatedInput["answers"].(map[string]string)
	if ansMap[question] != wantAns {
		return fmt.Errorf("answers[%q] = %q, want %q", question, ansMap[question], wantAns)
	}
	return assertClaudeNote(w.lastResult.UpdatedInput, question, wantNote)
}

func assertClaudeNote(updated map[string]any, question, wantNote string) error {
	ann, hasAnn := updated["annotations"].(map[string]map[string]string)
	if wantNote == "" && hasAnn {
		return fmt.Errorf("expected no annotations, got %+v", ann)
	}
	if wantNote != "" && (!hasAnn || ann[question]["notes"] != wantNote) {
		return fmt.Errorf("annotations = %+v, want notes=%q", ann, wantNote)
	}
	return nil
}

func (w *approveAccWorld) stepRestartAndMarkApproved() error {
	post, ok := w.fakeHome.lastApprovalPost()
	if !ok {
		return fmt.Errorf("no posted card")
	}
	var err error
	w.h, _, err = host.New(w.db, w.hostDev, w.clock)
	if err != nil {
		return err
	}
	w.h.Attach(w.homeDev.ID(), w.fakeHome)
	w.seedGroupRows(w.h, 1, false)
	actEnv := w.owner2.Sign(`{"t":"act"}`)
	by := w.humanID
	at := w.nowMS.Load()
	w.fakeHome.putCard(w.buildCardRow(post.Nonce, "approval", proto.CardState{
		Status: "approved", By: &by, At: &at, Act: &actEnv,
	}))
	return nil
}

func hostApproveSteps(sc *godog.ScenarioContext, _ fixtures) {
	w := newApproveAccWorld(afterScenario(sc))

	sc.Step(`^a paired host with a second browser device and a default level 3 "([^"]+)" "([^"]+)" session$`, func(cli, kind string) error {
		return w.setupHost(cli, kind, 60)
	})
	sc.Step(`^a paired host with a second browser device and a default level 3 "([^"]+)" "([^"]+)" session with a 1-second approval wait$`, func(cli, kind string) error {
		return w.setupHost(cli, kind, 1)
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)"$`, w.stepAskRun)
	sc.Step(`^the second browser device "(approves|denies)" the posted approval card while the session waits$`, w.stepBrowserApproval)
	sc.Step(`^the "([^"]+)" session receives "(allow|deny)"$`, func(_, want string) error {
		return w.stepAssertOutcome(want)
	})
	sc.Step(`^the "([^"]+)" session receives deny "([^"]+)"$`, func(_, want string) error {
		return w.assertDenyResult(want)
	})
	sc.Step(`^the posted approval card has "([^"]+)" thread and command "([^"]+)" and why "([^"]+)"$`, w.stepAssertApprovalCard)
	sc.Step(`^Claude Code calls AskUserQuestion with header "([^"]*)" and question "([^"]+)" and options "([^"]+)" and "([^"]+)"$`, w.stepClaudeAskQuestion)
	sc.Step(`^the Codex session asks requestUserInput id "([^"]+)" header "([^"]*)" question "([^"]+)" with options "([^"]+)" and "([^"]+)"$`, w.stepCodexAskQuestion)
	sc.Step(`^the second browser device chooses option "([^"]+)" with note "([^"]*)" on the posted decision card$`, w.stepChooseOption)
	sc.Step(`^the second browser device answers the posted decision card with custom text "([^"]+)"$`, w.stepCustomText)
	sc.Step(`^the posted decision card has options "([^"]+)" and "([^"]+)" and context "([^"]+)"$`, w.stepAssertDecisionWithContext)
	sc.Step(`^the posted decision card has no task thread and options "([^"]+)" and "([^"]+)"$`, w.stepAssertDecisionNoThread)
	sc.Step(`^Claude Code receives allow with answer "([^"]+)" for "([^"]+)" and note "([^"]*)"$`, w.stepAssertClaudeAnswer)
	sc.Step(`^Codex Reply receives "([^"]+)" for question "([^"]+)"$`, func(wantVal, qID string) error {
		if !reflect.DeepEqual(w.lastCodexAns[qID], []string{wantVal}) {
			return fmt.Errorf("Codex answers[%q] = %v, want [%s]", qID, w.lastCodexAns[qID], wantVal)
		}
		return nil
	})
	sc.Step(`^the approval table holds (\d+) rows$`, func(want int) error {
		var got int
		read := func(tx *store.Tx) error { return tx.QueryRow("SELECT count(*) FROM approval").Scan(&got) }
		if err := w.db.Write(read); err != nil || got != want {
			return fmt.Errorf("approval count = %d (%v), want %d", got, err, want)
		}
		return nil
	})
	sc.Step(`^the approval table holds 1 row in state "([^"]+)"$`, func(want string) error {
		var state string
		read := func(tx *store.Tx) error { return tx.QueryRow("SELECT state FROM approval").Scan(&state) }
		if err := w.db.Write(read); err != nil || state != want {
			return fmt.Errorf("approval state = %q (%v), want %q", state, err, want)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session asks to run "([^"]+)" with why "([^"]+)" and no decision arrives$`, func(_, cmd, why string) error {
		return w.callSyncBash(cmd, why, 3)
	})
	sc.Step(`^the host restarts on the same store and the home marks the pending card approved by the second browser device$`, w.stepRestartAndMarkApproved)
	sc.Step(`^the "([^"]+)" session retries "([^"]+)" with why "([^"]+)"$`, func(_, cmd, why string) error {
		w.postsAtBefore = w.fakeHome.totalPostCount()
		return w.callSyncBash(cmd, why, 3)
	})
	sc.Step(`^the "([^"]+)" session receives allow without posting another card$`, func(_ string) error {
		if w.lastResult.Behavior != "allow" || w.fakeHome.totalPostCount() != w.postsAtBefore {
			return fmt.Errorf("result=%+v posts=%d (was %d)", w.lastResult, w.fakeHome.totalPostCount(), w.postsAtBefore)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" session asks at level (\d+) to run "([^"]+)"$`, func(_ string, level int, cmd string) error {
		return w.callSyncBash(cmd, "", level)
	})
	sc.Step(`^no card was posted to the home$`, func() error {
		if w.fakeHome.totalPostCount() != 0 {
			return fmt.Errorf("posted %d cards, want 0", w.fakeHome.totalPostCount())
		}
		return nil
	})
}
