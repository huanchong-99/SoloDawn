//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["m4"] = m4Steps
}

const (
	m4GroupName     = "Website"
	m4OwnerDenyText = "Denied by the owner."
	m4TimeoutText   = "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."
	m4Q1Text        = "Which storage engine?"
	m4Q2Text        = "How should migrations run?"
	m4CodexThread   = "01a1f63f-701c-78c1-bbf3-f1ac61c56c40"
)

type m4Client struct {
	key     identity.Key
	frames  homeFrameClient
	conn    *home.Conn
	nextReq int64
	homeView
}

func attachM4Client(loop *home.Loop, cleanup func(func()), key identity.Key) *m4Client {
	c := &m4Client{
		key: key,
		// Single reader: drainUntil in m4Client.
		frames:   newHomeFrameClient(256),
		homeView: newHomeView(),
	}
	c.conn = loop.Attach(key.ID(), c.frames)
	cleanup(func() { loop.Detach(c.conn) })
	return c
}

// drainUntil is the single reader of c.frames.ch.
func (c *m4Client) drainUntil(timeout time.Duration, stop func() bool, onFrame func(json.RawMessage)) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for !stop() {
		select {
		case raw := <-c.frames.ch:
			applyRowsFrame(&c.homeView, raw)
			if onFrame != nil {
				onFrame(raw)
			}
		case <-timer.C:
			return errors.New("timed out waiting on M4 home frames")
		}
	}
	return nil
}

func (c *m4Client) syncGroup(loop *home.Loop, gid string) error {
	c.nextReq++
	reqID := c.nextReq
	loop.Sync(c.conn, proto.Sync{
		Request: proto.Request{Type: "sync", ID: reqID},
		Group:   gid,
	})
	code, err := c.awaitReply(reqID)
	if err != nil {
		return err
	}
	if code != "" {
		return fmt.Errorf("sync refused with %s", code)
	}
	return nil
}

func (c *m4Client) sendCmd(loop *home.Loop, v any) (string, error) {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return "", err
	}
	return c.sendEnv(loop, c.key.Sign(string(raw)))
}

func (c *m4Client) sendEnv(loop *home.Loop, env proto.Envelope) (string, error) {
	c.nextReq++
	reqID := c.nextReq
	loop.Cmd(c.conn, reqID, env)
	return c.awaitReply(reqID)
}

func (c *m4Client) awaitReply(reqID int64) (string, error) {
	var done bool
	var code string
	onFrame := func(raw json.RawMessage) {
		matched, errCode := decodeReplyCode(raw, reqID)
		if matched {
			done, code = true, errCode
		}
	}
	isDone := func() bool { return done }
	if err := c.drainUntil(5*time.Second, isDone, onFrame); err != nil {
		return "", err
	}
	return code, nil
}

type m4World struct {
	f             fixtures
	cleanup       func(func())
	tb            scenarioTB
	root          string
	repoDir       string
	cluster       *nodetest.Cluster
	activeHost    *host.Host
	activeLink    *node.LocalLink
	closeMCP      func()
	browser2Key   identity.Key
	b1            *m4Client
	b2            *m4Client
	ids           seqIDGen
	scriptSeq     int
	mcpSeq        int
	groupID       proto.ID
	currentTaskID string
	secondTaskID  string
	currentCard   proto.MsgRow
	firstCardID   string
	task2DecID    string
	task2AppID    string
	seenCards     map[string]bool
	waitElapsed   time.Duration
	pauseStarted  time.Time
}

func m4Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m4World{
		f:         f,
		seenCards: map[string]bool{},
	}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.activeHost != nil {
			waitHostDormant(w.activeHost)
		}
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.tb = scenarioTB{TB: f.t, cleanup: w.cleanup}
	registerM4SetupSteps(sc, w)
	registerM4ApprovalSteps(sc, w)
	registerM4DecisionSteps(sc, w)
	registerM4LifecycleSteps(sc, w)
	registerM4PauseSteps(sc, w)
}

func registerM4SetupSteps(sc *godog.ScenarioContext, w *m4World) {
	sc.Step(`^an M4 project group with a second browser and configured AIs "([^"]+)" and "([^"]+)"$`, func(ai1, ai2 string) error {
		return w.givenTwoAIGroup(0, ai1, ai2)
	})
	sc.Step(`^an M4 project group with a (\d+)-second approval wait, a second browser and configured AI "([^"]+)"$`, w.givenSingleAIWaitGroup)
	sc.Step(`^an M4 project group with a (\d+)-second approval wait, a second browser and configured AIs "([^"]+)" and "([^"]+)"$`, w.givenTwoAIGroup)
}

func registerM4ApprovalSteps(sc *godog.ScenarioContext, w *m4World) {
	sc.Step(`^an M4 task for "([^"]+)" asks permission to run "([^"]+)" with why "([^"]+)" before posting "([^"]+)"$`, w.whenTaskAsksAllow)
	sc.Step(`^an M4 task for "([^"]+)" asks command approval for "([^"]+)" with why "([^"]+)" before posting "([^"]+)"$`, w.whenCodexTaskAsksAllow)
	sc.Step(`^an M4 task for "([^"]+)" asks permission to run "([^"]+)" with why "([^"]+)" and expects owner denial$`, w.whenTaskAsksDeny)
	sc.Step(`^an M4 task for "([^"]+)" asks file approval on "([^"]+)" diff "([^"]+)" with why "([^"]+)" and expects owner denial$`, w.whenCodexFileAsksDeny)
	sc.Step(`^the M4 (first|second) browser sees a pending approval card on "([^"]+)"'s task for "([^"]+)" command "([^"]+)" with why "([^"]+)"$`, w.thenBrowserSeesApprovalCard)
	sc.Step(`^the M4 second browser approves that pending card and "([^"]+)" posts "([^"]+)"$`, w.whenSecondApprovesAndPosts)
	sc.Step(`^the M4 (first|second) browser denies that pending card and "([^"]+)" posts "([^"]+)"$`, w.whenBrowserDeniesAndPosts)
	sc.Step(`^the M4 approval card is approved by the owner from the second browser with 0 pending host records$`, w.thenApprovedFromSecondBrowser)
	sc.Step(`^the M4 approval card is denied by the owner with no task wait and 0 pending host records$`, w.thenDeniedAndCleared)
}

func registerM4DecisionSteps(sc *godog.ScenarioContext, w *m4World) {
	sc.Step(`^an M4 task for "([^"]+)" asks two four-option AskUserQuestion prompts and posts "([^"]+)"$`, w.whenClaudeAsksTwoQuestions)
	sc.Step(`^the M4 browsers answer "([^"]+)"'s first decision card with option "([^"]+)" and note "([^"]+)" and second with free text "([^"]+)"$`, w.thenAnswerClaudeQuestions)
	sc.Step(`^"([^"]+)" posts "([^"]+)" in the M4 group$`, w.thenAIPostsAndSettles)
	sc.Step(`^the M4 owner mentions "([^"]+)" in the main line and it asks two four-option requestUserInput prompts$`, w.whenCodexMainLineQuestions)
	sc.Step(`^the M4 browsers answer "([^"]+)"'s main-line decision cards with option "([^"]+)" and with free text "([^"]+)"$`, w.thenAnswerCodexQuestions)
	sc.Step(`^a second M4 task for "([^"]+)" raises a four-option request_decision card and ends its turn$`, w.whenTaskRaisesRequestDecision)
	sc.Step(`^the M4 second browser chooses option "([^"]+)" with note "([^"]+)" and wakes "([^"]+)" to post "([^"]+)"$`, w.thenChooseAndWakeRequestDecision)
}

func registerM4LifecycleSteps(sc *godog.ScenarioContext, w *m4World) {
	sc.Step(`^an M4 task for "([^"]+)" asks permission to run "([^"]+)" with why "([^"]+)" and nobody answers for 20 seconds$`, w.whenTaskWaitsTwentySeconds)
	sc.Step(`^"([^"]+)"'s turn ends after the 20-second wait while the card stays pending, the task waits on approval, and the host keeps 1 pending record$`, w.thenTwentySecondWaitEnded)
	sc.Step(`^the M4 host restarts on the node store$`, w.restartHost)
	sc.Step(`^"([^"]+)" is scripted on resume to retry "([^"]+)" and then ask to run "([^"]+)" before posting "([^"]+)"$`, w.scriptResumeRetryAndDifferentCmd)
	sc.Step(`^the M4 second browser approves the first card after the restart$`, w.whenApproveFirstAfterRestart)
	sc.Step(`^the M4 browsers see no second card for "([^"]+)" and see a new pending approval card from "([^"]+)" for "([^"]+)"$`, w.thenNoRetryCardAndNewCard)
	sc.Step(`^the M4 second browser approves that new pending card$`, func() error {
		return w.actOnCard(w.b2, w.currentCard.ID, "approve", "", "", "")
	})
	sc.Step(`^"([^"]+)" posts "([^"]+)" in the M4 group after 2 resumed starts of the same CLI session and 2 total approval cards$`, w.thenResumedRetryCompletes)
	sc.Step(`^an M4 task for "([^"]+)" times out waiting for approval on "([^"]+)"$`, w.whenTaskTimesOutShort)
	sc.Step(`^the M4 owner reassigns that task to "([^"]+)" scripted to post "([^"]+)"$`, w.whenOwnerReassignsTask)
	sc.Step(`^the M4 approval card has status "([^"]+)", "([^"]+)" posts "([^"]+)", and approving the reassigned card fails with "([^"]+)"$`, w.thenReassignedCardAndRefusal)
	sc.Step(`^a second M4 task for "([^"]+)" leaves a pending decision card and a timed-out pending approval card on "([^"]+)"$`, w.whenSecondTaskLeavesTwoPendingCards)
	sc.Step(`^the M4 owner cancels that second task$`, w.whenOwnerCancelsSecondTask)
	sc.Step(`^both pending M4 cards on the second task have status "([^"]+)" and late actions on them fail with "([^"]+)"$`, w.thenCancelledCardsExpiredAndRefused)
}

func registerM4PauseSteps(sc *godog.ScenarioContext, w *m4World) {
	sc.Step(`^"([^"]+)" starts a main-line turn posting "([^"]+)" and is scripted to post "([^"]+)" on its next turn$`, w.whenAIStartsPausableTurn)
	sc.Step(`^the M4 owner pauses the group$`, w.whenOwnerPausesGroup)
	sc.Step(`^"([^"]+)"'s running turn is interrupted within 2 seconds with reason "([^"]+)" and system message "([^"]+)"$`, w.thenTurnInterruptedWithinTwoSeconds)
	sc.Step(`^the M4 owner mentions "([^"]+)" with "([^"]+)"$`, w.postOwnerMention)
	sc.Step(`^the (\d+) M4 mention items stay on the home while the group is paused$`, func(want int) error {
		return awaitHomeInboxCount(w.node().Store, want)
	})
	sc.Step(`^the M4 owner resumes the group$`, w.whenOwnerResumesGroup)
	sc.Step(`^"([^"]+)" posts "([^"]+)" after system message "([^"]+)" and the M4 home inbox is empty$`, w.thenResumedGroupDeliversMentions)
	sc.Step(`^"([^"]+)"'s approval-waiting task was not woken by the group resume$`, w.thenApprovalTaskStayedAsleep)
}

func (w *m4World) node() *nodetest.Node {
	return w.cluster.Nodes[0]
}

func (w *m4World) givenSingleAIWaitGroup(waitS int, aiName string) error {
	if err := w.startNode(); err != nil {
		return err
	}
	if err := w.joinAI(aiName, m4VendorOf(aiName)); err != nil {
		return err
	}
	return w.applyOptionalWait(waitS)
}

func (w *m4World) givenTwoAIGroup(waitS int, ai1, ai2 string) error {
	if err := w.startNode(); err != nil {
		return err
	}
	if err := w.joinAI(ai1, m4VendorOf(ai1)); err != nil {
		return err
	}
	if err := w.joinAI(ai2, m4VendorOf(ai2)); err != nil {
		return err
	}
	return w.applyOptionalWait(waitS)
}

func (w *m4World) applyOptionalWait(waitS int) error {
	if waitS <= 0 {
		return nil
	}
	return w.setApprovalWait(waitS)
}

func m4VendorOf(aiName string) string {
	if strings.EqualFold(aiName, "codex") {
		return "codex"
	}
	return "claude"
}

func (w *m4World) startNode() error {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sdm4")
	if err != nil {
		return err
	}
	w.root = root
	w.repoDir = filepath.Join(root, "src")
	if err := initLocalGitRepo(w.repoDir); err != nil {
		return err
	}
	w.browser2Key = newRandomPeerKey()
	w.cluster = nodetest.Start(w.tb, nodetest.Config{
		Root:          w.root,
		BinDir:        w.f.fakeCLIs,
		Nodes:         1,
		ExtraBrowsers: []identity.Key{w.browser2Key},
	})
	n := w.node()
	w.activeHost, w.activeLink = n.Host, n.Link
	if err := w.attachMCP(n.Host); err != nil {
		return err
	}
	w.b1 = attachM4Client(n.Home, w.cleanup, w.cluster.Owner)
	w.b2 = attachM4Client(n.Home, w.cleanup, w.browser2Key)
	return w.createGroupAndSync()
}

func (w *m4World) attachMCP(h *host.Host) error {
	w.mcpSeq++
	dir := filepath.Join(w.node().Dir, fmt.Sprintf("mcp-%d", w.mcpSeq))
	_, stop, err := attachMCPSocket(w.f.t, w.cleanup, h, dir)
	w.closeMCP = stop
	return err
}

func (w *m4World) createGroupAndSync() error {
	w.groupID = w.ids.next()
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.groupID, Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Op:          "create",
		},
		Name: m4GroupName, Kind: "project", Repo: w.repoDir, Base: "main",
		Forge: "none", MemberName: "Owner", Checks: []string{},
	}
	if err := w.sendOK(w.b1, create); err != nil {
		return err
	}
	if err := w.b1.syncGroup(w.node().Home, string(w.groupID)); err != nil {
		return err
	}
	return w.b2.syncGroup(w.node().Home, string(w.groupID))
}

func (w *m4World) setApprovalWait(waitS int) error {
	limits := w.b1.group.Limits
	limits.ApprovalWaitS = waitS
	setCmd := proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Op:          "set",
		},
		Limits: &limits,
	}
	if err := w.sendOK(w.b1, setCmd); err != nil {
		return err
	}
	return w.awaitHostWaitLimit(w.activeHost, waitS)
}

func (w *m4World) awaitHostWaitLimit(h *host.Host, want int) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m4HostHasWaitLimit(h, string(w.groupID), want) {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for host approval_wait_s %d", want)
}

func m4HostHasWaitLimit(h *host.Host, gid string, want int) bool {
	raw, ok := h.Rows.Get(gid, "group", gid)
	if !ok {
		return false
	}
	var grp proto.GroupRow
	return proto.Decode(raw, &grp) == nil && grp.Limits.ApprovalWaitS == want
}

func (w *m4World) joinAI(aiName, agent string) error {
	n := w.node()
	if err := configureHostAI(w.activeHost, w.activeLink, &w.ids, w.cluster.Owner.ID(), n.Key.ID(), w.groupID, aiName, agent); err != nil {
		return err
	}
	return w.awaitAISynced(aiName)
}

func (w *m4World) awaitAISynced(aiName string) error {
	hasB1 := func() bool { _, ok := w.b1.aiByName[aiName]; return ok }
	if err := w.b1.drainUntil(5*time.Second, hasB1, nil); err != nil {
		return err
	}
	hasB2 := func() bool { _, ok := w.b2.aiByName[aiName]; return ok }
	return w.b2.drainUntil(5*time.Second, hasB2, nil)
}

func (w *m4World) sendOK(b *m4Client, payload any) error {
	code, err := b.sendCmd(w.node().Home, payload)
	if err != nil {
		return err
	}
	if code != "" {
		return fmt.Errorf("unexpected home refusal %q", code)
	}
	return nil
}

func (w *m4World) setScript(vendor string, doc map[string]any) error {
	return writeClusterScript(w.tb, w.cluster, w.root, &w.scriptSeq, vendor, doc)
}

func (w *m4World) waitLogs(prefix string, want int) ([]string, error) {
	return waitFakeCLILogs(w.cluster.LogPath, prefix, want, 30*time.Second)
}

func (w *m4World) settleDormant() error {
	waitHostDormant(w.activeHost)
	settled := func() bool { return allMembersSettled(w.activeHost, w.b1.aiByName) }
	return w.b1.drainUntil(5*time.Second, settled, nil)
}

func (w *m4World) startTaskFor(aiName, title, plan string) (string, error) {
	aiID := w.b1.aiByName[aiName].ID
	taskID := w.ids.next()
	cmd := proto.TaskStart{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{
				Type: "task", Nonce: taskID, Timestamp: time.Now().UnixMilli(), Group: w.groupID, As: proto.ID(aiID),
			},
			Op: "start", ID: taskID,
		},
		Title: title, Plan: plan, Origin: "direct",
	}
	raw, err := marshalNoEscape(cmd)
	if err != nil {
		return "", err
	}
	if _, err := w.activeHost.Command(context.Background(), w.node().Key.ID(), string(raw)); err != nil {
		return "", err
	}
	return string(taskID), nil
}

func (w *m4World) browserOf(which string) *m4Client {
	if which == "first" {
		return w.b1
	}
	return w.b2
}

func m4ApproveCallSteps(id int, toolName, toolUseID string, input any, wantRes host.ApprovalResult) []map[string]any {
	wantText := string(mustMarshalNoEscape(wantRes))
	return []map[string]any{
		{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{
				"name":      "approve",
				"arguments": map[string]any{"tool_name": toolName, "input": input, "tool_use_id": toolUseID},
			},
		}},
		{"dir": "mcp->cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": id,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": wantText}},
				"isError": false,
			},
		}},
	}
}

func m4ClaudeBashAskSteps(id int, toolUseID, cmd, why string, res host.ApprovalResult) []map[string]any {
	in := map[string]string{"command": cmd, "description": why}
	return m4ApproveCallSteps(id, "Bash", toolUseID, in, res)
}

func m4ClaudeTurnDoc(body []map[string]any, postID int, postText string) map[string]any {
	steps := []map[string]any{claudeUserStdinStep("")}
	steps = append(steps, claudeInitMCP(0)...)
	steps = append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}})
	steps = append(steps, body...)
	if postText != "" {
		steps = append(steps, mcpPostSteps(postID, postText)...)
	}
	steps = append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": "$session",
	}})
	return map[string]any{"steps": steps}
}

func m4CodexTurnDoc(session, turnID string, body []map[string]any, postID int, postText string) map[string]any {
	steps := codexStartupSteps()
	steps = append(steps,
		map[string]any{"dir": "stdin", "msg": map[string]any{"method": "turn/start", "params": map[string]any{"threadId": "$session"}}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}}},
	)
	steps = append(steps, body...)
	if postText != "" {
		steps = append(steps, mcpPostSteps(postID, postText)...)
	}
	steps = append(steps, map[string]any{"dir": "stdout", "msg": map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": "$session", "turn": map[string]any{"id": turnID, "status": "completed", "error": nil}},
	}})
	return map[string]any{"session": session, "steps": steps}
}

func m4CodexInterruptTurnSteps(turnID string, postID int, postText string) []map[string]any {
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"method": "turn/start", "params": map[string]any{"threadId": "$session"}}},
		{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}}},
	}
	steps = append(steps, mcpPostSteps(postID, postText)...)
	return append(steps,
		map[string]any{"dir": "stdin", "msg": map[string]any{"method": "turn/interrupt", "params": map[string]any{"threadId": "$session", "turnId": turnID}}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"id": "$id", "result": map[string]any{}}},
		map[string]any{"dir": "stdout", "msg": map[string]any{
			"method": "turn/completed",
			"params": map[string]any{"threadId": "$session", "turn": map[string]any{"id": turnID, "status": "interrupted", "error": nil}},
		}},
	)
}

func m4CodexCmdAskSteps(rpcID int, cmd, reason, decision string) []map[string]any {
	return []map[string]any{
		{"dir": "stdout", "msg": map[string]any{
			"id": rpcID, "method": "item/commandExecution/requestApproval",
			"params": map[string]any{"command": cmd, "cwd": "/tmp", "reason": reason},
		}},
		{"dir": "stdin", "msg": map[string]any{"id": rpcID, "result": map[string]any{"decision": decision}}},
	}
}

func m4CodexFileAskSteps(rpcID int, itemID, path, diff, reason, decision string) []map[string]any {
	changes := []map[string]any{{"path": path, "kind": map[string]any{"type": "update"}, "diff": diff}}
	return []map[string]any{
		{"dir": "stdout", "msg": map[string]any{
			"method": "item/started",
			"params": map[string]any{"item": map[string]any{"type": "fileChange", "id": itemID, "changes": changes}},
		}},
		{"dir": "stdout", "msg": map[string]any{
			"id": rpcID, "method": "item/fileChange/requestApproval",
			"params": map[string]any{"itemId": itemID, "reason": reason},
		}},
		{"dir": "stdin", "msg": map[string]any{"id": rpcID, "result": map[string]any{"decision": decision}}},
	}
}

func m4ColonOptions(pairs ...string) []map[string]string {
	out := make([]map[string]string, len(pairs))
	for i, p := range pairs {
		label, desc, _ := strings.Cut(p, ":")
		out[i] = map[string]string{"label": label, "description": desc}
	}
	return out
}

func m4AskUserSteps(id int, toolUseID, header, question string, opts []map[string]string, ans, note string) []map[string]any {
	in := map[string]any{
		"questions": []map[string]any{{"header": header, "question": question, "options": opts, "multiSelect": false}},
	}
	updated := map[string]any{}
	if err := json.Unmarshal(mustMarshalNoEscape(in), &updated); err != nil {
		panic(err)
	}
	updated["answers"] = map[string]string{question: ans}
	if note != "" {
		updated["annotations"] = map[string]map[string]string{question: {"notes": note}}
	}
	return m4ApproveCallSteps(id, "AskUserQuestion", toolUseID, in, host.ApprovalResult{Behavior: "allow", UpdatedInput: updated})
}

func m4CodexQuestionSteps(rpcID int, qID, header, question string, opts []map[string]string, wantAns []string) []map[string]any {
	q := map[string]any{"id": qID, "header": header, "question": question, "isSecret": false, "options": opts}
	return []map[string]any{
		{"dir": "stdout", "msg": map[string]any{
			"id": rpcID, "method": "item/tool/requestUserInput",
			"params": map[string]any{"questions": []map[string]any{q}},
		}},
		{"dir": "stdin", "msg": map[string]any{
			"id": rpcID, "result": map[string]any{"answers": map[string]any{qID: map[string]any{"answers": wantAns}}},
		}},
	}
}

func (w *m4World) whenTaskAsksAllow(aiName, cmd, why, postText string) error {
	body := m4ClaudeBashAskSteps(1, "toolu_allow", cmd, why, host.ApprovalResult{Behavior: "allow"})
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 2, postText)); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Install dependency", "Add test dependency")
	w.currentTaskID = tid
	return err
}

func (w *m4World) whenCodexTaskAsksAllow(aiName, cmd, why, postText string) error {
	body := m4CodexCmdAskSteps(91, cmd, why, "accept")
	if err := w.setScript(m4VendorOf(aiName), m4CodexTurnDoc(m4CodexThread, "turn-1", body, 1, postText)); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Install browser", "Add browser binary")
	w.currentTaskID = tid
	return err
}

func (w *m4World) whenTaskAsksDeny(aiName, cmd, why string) error {
	deny := host.ApprovalResult{Behavior: "deny", Message: m4OwnerDenyText}
	body := m4ClaudeBashAskSteps(1, "toolu_deny", cmd, why, deny)
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 2, "Stopped without publishing.")); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Publish release", "Run npm publish")
	w.currentTaskID = tid
	if err != nil {
		return err
	}
	card, err := w.waitPendingCard(w.b1, "approval", w.b1.aiByName[aiName].ID)
	w.currentCard = card
	return err
}

func (w *m4World) whenCodexFileAsksDeny(aiName, path, diff, why string) error {
	body := m4CodexFileAskSteps(92, "fc-prod", path, diff, why, "decline")
	if err := w.setScript(m4VendorOf(aiName), m4CodexTurnDoc(m4CodexThread, "turn-1", body, 1, "Stopped without editing config.")); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Edit config", "Touch config file")
	w.currentTaskID = tid
	return err
}

func (w *m4World) waitPendingCard(b *m4Client, kind, byAI string) (proto.MsgRow, error) {
	var found proto.MsgRow
	match := func() bool {
		m, ok := findUnseenCard(b, kind, byAI, w.seenCards)
		if ok {
			found = m
		}
		return ok
	}
	if err := b.drainUntil(10*time.Second, match, nil); err != nil {
		return proto.MsgRow{}, err
	}
	w.seenCards[found.ID] = true
	return found, nil
}

func findUnseenCard(b *m4Client, kind, byAI string, seen map[string]bool) (proto.MsgRow, bool) {
	for _, id := range b.msgOrder {
		m := b.msgs[id]
		if !seen[id] && m4IsPendingBy(m, kind, byAI) {
			return m, true
		}
	}
	return proto.MsgRow{}, false
}

func m4IsPendingBy(m proto.MsgRow, kind, byAI string) bool {
	return m.Kind == kind && m.By == byAI && m.State != nil && m.State.Status == "pending"
}

func (w *m4World) thenBrowserSeesApprovalCard(which, aiName, wantTool, wantCmd, wantWhy string) error {
	b := w.browserOf(which)
	card, err := w.waitPendingCard(b, "approval", b.aiByName[aiName].ID)
	if err != nil {
		return err
	}
	w.currentCard = card
	if err := checkApprovalCardBody(card, w.currentTaskID, wantTool, wantCmd, wantWhy, b.ownerMem); err != nil {
		return err
	}
	if err := w.awaitTaskWait(b, w.currentTaskID, "approval", card.ID); err != nil {
		return err
	}
	return assertHostApprovalCount(w.node().Store, 1)
}

func checkApprovalCardBody(card proto.MsgRow, wantTask, wantTool, wantCmd, wantWhy, ownerMem string) error {
	if card.Thread == nil || *card.Thread != wantTask || !slices.Equal(card.Mentions, []string{ownerMem}) {
		return fmt.Errorf("unexpected card thread/mentions: thread=%v mentions=%v", card.Thread, card.Mentions)
	}
	var body proto.ApprovalBody
	if err := proto.Decode(card.Body, &body); err != nil {
		return err
	}
	return checkApprovalFields(body, wantTask, wantTool, wantCmd, wantWhy)
}

func checkApprovalFields(body proto.ApprovalBody, wantTask, wantTool, wantCmd, wantWhy string) error {
	wantCat := "command"
	if wantTool == "fileChange" {
		wantCat = "file"
	}
	if [3]string{body.Task, body.Tool, body.Category} != [3]string{wantTask, wantTool, wantCat} {
		return fmt.Errorf("unexpected approval metadata: %+v", body)
	}
	return checkApprovalText(body, wantCmd, wantWhy)
}

func checkApprovalText(body proto.ApprovalBody, wantCmd, wantWhy string) error {
	unescaped := strings.ReplaceAll(wantCmd, `\n`, "\n")
	gotWhy := ""
	if body.Why != nil {
		gotWhy = *body.Why
	}
	if body.Command != unescaped || gotWhy != wantWhy {
		return fmt.Errorf("unexpected approval command/why: %+v", body)
	}
	return nil
}

func (w *m4World) awaitTaskWait(b *m4Client, taskID, wantKind, wantMsg string) error {
	hasWait := func() bool {
		t, ok := b.tasks[taskID]
		return ok && t.Wait != nil && t.Wait.Kind == wantKind && t.Wait.Msg == wantMsg
	}
	return b.drainUntil(5*time.Second, hasWait, nil)
}

func (w *m4World) awaitTaskCleared(b *m4Client, taskID string) error {
	cleared := func() bool {
		t, ok := b.tasks[taskID]
		return ok && t.Wait == nil
	}
	return b.drainUntil(5*time.Second, cleared, nil)
}

func (w *m4World) actOnCard(b *m4Client, cardID, action, arg, text, wantCode string) error {
	cmd := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Msg:         proto.ID(cardID),
		Action:      action,
		Arg:         arg,
		Text:        text,
	}
	code, err := b.sendCmd(w.node().Home, cmd)
	if err != nil {
		return err
	}
	if code != wantCode {
		return fmt.Errorf("act %s returned %q, want %q", action, code, wantCode)
	}
	return nil
}

func (w *m4World) whenSecondApprovesAndPosts(aiName, wantPost string) error {
	if err := w.actOnCard(w.b2, w.currentCard.ID, "approve", "", "", ""); err != nil {
		return err
	}
	return w.thenAIPostsAndSettles(aiName, wantPost)
}

func (w *m4World) whenBrowserDeniesAndPosts(which, aiName, wantPost string) error {
	if err := w.actOnCard(w.browserOf(which), w.currentCard.ID, "deny", "", "", ""); err != nil {
		return err
	}
	return w.thenAIPostsAndSettles(aiName, wantPost)
}

func (w *m4World) thenApprovedFromSecondBrowser() error {
	if err := w.awaitCardDecision(w.b1, w.currentCard.ID, "approved", w.b1.ownerMem, w.browser2Key.ID()); err != nil {
		return err
	}
	if err := w.awaitTaskCleared(w.b1, w.currentTaskID); err != nil {
		return err
	}
	if err := assertDeliveredDecided(w.node().Store); err != nil {
		return err
	}
	return assertHostApprovalCount(w.node().Store, 0)
}

func (w *m4World) thenDeniedAndCleared() error {
	if err := w.awaitCardStatus(w.b1, w.currentCard.ID, "denied"); err != nil {
		return err
	}
	if err := w.awaitTaskCleared(w.b1, w.currentTaskID); err != nil {
		return err
	}
	return assertHostApprovalCount(w.node().Store, 0)
}

func (w *m4World) awaitCardDecision(b *m4Client, cardID, wantStatus, wantBy, wantSigner string) error {
	ready := func() bool {
		m, ok := b.msgs[cardID]
		return ok && m.State != nil && m.State.Status == wantStatus
	}
	if err := b.drainUntil(5*time.Second, ready, nil); err != nil {
		return err
	}
	st := b.msgs[cardID].State
	if st.By == nil || *st.By != wantBy || st.Act == nil || st.Act.Key != wantSigner {
		return fmt.Errorf("unexpected card state: %+v", st)
	}
	return nil
}

func (w *m4World) awaitCardStatus(b *m4Client, cardID, wantStatus string) error {
	ready := func() bool {
		m, ok := b.msgs[cardID]
		return ok && m.State != nil && m.State.Status == wantStatus
	}
	return b.drainUntil(5*time.Second, ready, nil)
}

func assertHostApprovalCount(db *store.Store, want int) error {
	var got int
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM approval WHERE state = 'pending'").Scan(&got)
	}
	if err := db.Write(read); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("pending host approval rows = %d, want %d", got, want)
	}
	return nil
}

func assertDeliveredDecided(db *store.Store) error {
	var got int
	read := func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM audit WHERE what = 'delivered' AND detail = 'decided'").Scan(&got)
	}
	if err := db.Write(read); err != nil {
		return err
	}
	if got < 1 {
		return errors.New("expected at least 1 delivered decided audit row")
	}
	return nil
}

func (w *m4World) whenClaudeAsksTwoQuestions(aiName, postText string) error {
	opts1 := m4ColonOptions("Postgres:Server", "MySQL:Relational", "SQLite:Embedded", "DuckDB:Analytical")
	opts2 := m4ColonOptions("Atlas:CLI", "Goose:Go", "Flyway:JVM", "Sqitch:Perl")
	body := slices.Concat(
		m4AskUserSteps(1, "toolu_q1", "Database", m4Q1Text, opts1, "SQLite", "Enable WAL mode"),
		m4AskUserSteps(2, "toolu_q2", "Migrations", m4Q2Text, opts2, "Embed numbered SQL files", ""),
	)
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 3, postText)); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Choose storage", "Pick storage and migrations")
	w.currentTaskID = tid
	return err
}

func (w *m4World) thenAnswerClaudeQuestions(aiName, optID, note, customText string) error {
	aiID := w.b2.aiByName[aiName].ID
	if err := w.answerTaskDecision(w.b2, aiID, "Database", optID, note); err != nil {
		return err
	}
	return w.answerTaskDecision(w.b1, aiID, "Migrations", "", customText)
}

func (w *m4World) answerTaskDecision(b *m4Client, aiID, wantCtx, optID, text string) error {
	card, err := w.waitPendingCard(b, "decision", aiID)
	if err != nil {
		return err
	}
	if err := checkDecisionCardBody(card, w.currentTaskID, wantCtx, 4, ""); err != nil {
		return err
	}
	if err := w.awaitTaskWait(b, w.currentTaskID, "decision", card.ID); err != nil {
		return err
	}
	return w.actOnCard(b, card.ID, "choose", optID, text, "")
}

func checkDecisionCardBody(card proto.MsgRow, wantTask, wantCtx string, wantOpts int, wantRec string) error {
	if err := checkDecisionThread(card, wantTask); err != nil {
		return err
	}
	var body proto.DecisionBody
	if err := proto.Decode(card.Body, &body); err != nil {
		return err
	}
	if len(body.Options) != wantOpts || body.Options[0].ID != "A" {
		return fmt.Errorf("unexpected decision options: %+v", body.Options)
	}
	return checkDecisionMeta(body, wantCtx, wantRec)
}

func checkDecisionThread(card proto.MsgRow, wantTask string) error {
	gotThread := ""
	if card.Thread != nil {
		gotThread = *card.Thread
	}
	if gotThread != wantTask {
		return fmt.Errorf("decision card thread = %q, want %q", gotThread, wantTask)
	}
	return nil
}

func checkDecisionMeta(body proto.DecisionBody, wantCtx, wantRec string) error {
	gotCtx := ""
	if body.Context != nil {
		gotCtx = *body.Context
	}
	gotRec := ""
	if body.Recommended != nil {
		gotRec = *body.Recommended
	}
	if gotCtx != wantCtx || gotRec != wantRec {
		return fmt.Errorf("decision context=%q rec=%q, want %q / %q", gotCtx, gotRec, wantCtx, wantRec)
	}
	return nil
}

func (w *m4World) whenCodexMainLineQuestions(aiName string) error {
	opts1 := m4ColonOptions("JSONL:Lines", "CBOR:Binary", "Protobuf:Typed", "MsgPack:Compact")
	opts2 := m4ColonOptions("none:Raw", "gzip:Standard", "lz4:Fast", "snappy:Framed")
	body := slices.Concat(
		m4CodexQuestionSteps(101, "q_wire", "Framing", "Which framing?", opts1, []string{"MsgPack"}),
		m4CodexQuestionSteps(102, "q_dict", "", "Custom compression?", opts2, []string{"zstd dictionary"}),
	)
	if err := w.setScript(m4VendorOf(aiName), m4CodexTurnDoc(m4CodexThread, "turn-q", body, 1, "Codex recorded MsgPack and zstd.")); err != nil {
		return err
	}
	return w.postOwnerMention(aiName, "@"+aiName+" check framing")
}

func (w *m4World) postOwnerMention(aiName, text string) error {
	aiID := w.b1.aiByName[aiName].ID
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Kind:        "text", Text: text, Mentions: []proto.ID{proto.ID(aiID)}, Body: proto.EmptyBody{},
	}
	return w.sendOK(w.b1, post)
}

func (w *m4World) thenAnswerCodexQuestions(aiName, optID, customText string) error {
	aiID := w.b2.aiByName[aiName].ID
	card1, err := w.waitPendingCard(w.b2, "decision", aiID)
	if err != nil {
		return err
	}
	if err := checkDecisionCardBody(card1, "", "Framing", 4, ""); err != nil {
		return err
	}
	if err := w.actOnCard(w.b2, card1.ID, "choose", optID, "", ""); err != nil {
		return err
	}
	card2, err := w.waitPendingCard(w.b1, "decision", aiID)
	if err != nil {
		return err
	}
	if err := checkDecisionCardBody(card2, "", "", 4, ""); err != nil {
		return err
	}
	return w.actOnCard(w.b1, card2.ID, "choose", "", customText, "")
}

func (w *m4World) whenTaskRaisesRequestDecision(aiName string) error {
	reqArgs := map[string]any{
		"question": "Where should the status badge sit?", "context": "Placement", "recommended": 1,
		"options": []map[string]string{
			{"label": "Sidebar", "effect": "Compact"},
			{"label": "Top bar", "effect": "Always visible"},
			{"label": "Footer", "effect": "Quiet"},
			{"label": "Modal", "effect": "Blocking"},
		},
	}
	body := m4GenericToolCall(1, "request_decision", reqArgs)
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 0, "")); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Badge layout", "Choose badge placement")
	w.currentTaskID = tid
	if err != nil {
		return err
	}
	return w.awaitDecisionAndTurnExit(aiName, tid, "Placement", "B", 2)
}

func m4GenericToolCall(id int, name string, args any) []map[string]any {
	return []map[string]any{
		{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": id, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		}},
		{"dir": "mcp->cli", "msg": map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"isError": false}}},
	}
}

func (w *m4World) awaitDecisionAndTurnExit(aiName, taskID, wantCtx, wantRec string, wantExits int) error {
	card, err := w.waitPendingCard(w.b2, "decision", w.b2.aiByName[aiName].ID)
	if err != nil {
		return err
	}
	w.currentCard = card
	if err := checkDecisionCardBody(card, taskID, wantCtx, 4, wantRec); err != nil {
		return err
	}
	if _, err := w.waitLogs("exit "+m4VendorOf(aiName)+" ", wantExits); err != nil {
		return err
	}
	return w.settleDormant()
}

func (w *m4World) thenChooseAndWakeRequestDecision(optID, note, aiName, wantPost string) error {
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(nil, 1, wantPost)); err != nil {
		return err
	}
	if err := w.actOnCard(w.b2, w.currentCard.ID, "choose", optID, note, ""); err != nil {
		return err
	}
	if err := w.thenAIPostsAndSettles(aiName, wantPost); err != nil {
		return err
	}
	return w.awaitTaskCleared(w.b1, w.currentTaskID)
}

func (w *m4World) awaitAIPost(aiName, wantPost string, timeout time.Duration) error {
	aiID := w.b1.aiByName[aiName].ID
	hasPost := func() bool { return viewHasMsgFrom(&w.b1.homeView, aiID, wantPost) }
	return w.b1.drainUntil(timeout, hasPost, nil)
}

func (w *m4World) thenAIPostsAndSettles(aiName, wantPost string) error {
	if err := w.awaitAIPost(aiName, wantPost, 10*time.Second); err != nil {
		return err
	}
	return w.settleDormant()
}

func (w *m4World) whenTaskWaitsTwentySeconds(aiName, cmd, why string) error {
	timeoutDeny := host.ApprovalResult{Behavior: "deny", Message: m4TimeoutText}
	body := m4ClaudeBashAskSteps(1, "toolu_wait20", cmd, why, timeoutDeny)
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 0, "")); err != nil {
		return err
	}
	started := time.Now()
	tid, err := w.startTaskFor(aiName, "Add chalk", "Install chalk dependency")
	w.currentTaskID = tid
	if err != nil {
		return err
	}
	card, err := w.waitPendingCard(w.b1, "approval", w.b1.aiByName[aiName].ID)
	if err != nil {
		return err
	}
	w.currentCard, w.firstCardID = card, card.ID
	if _, err := w.waitLogs("exit "+m4VendorOf(aiName)+" ", 1); err != nil {
		return err
	}
	w.waitElapsed = time.Since(started)
	return w.settleDormant()
}

func (w *m4World) thenTwentySecondWaitEnded(aiName string) error {
	if w.waitElapsed < 19*time.Second {
		return fmt.Errorf("approval wait ended in %s, want ~20s", w.waitElapsed)
	}
	if got := w.b1.aiByName[aiName].State; got != "dormant" {
		return fmt.Errorf("%s state = %q, want dormant", aiName, got)
	}
	if err := w.awaitCardStatus(w.b1, w.firstCardID, "pending"); err != nil {
		return err
	}
	if err := w.awaitTaskWait(w.b1, w.currentTaskID, "approval", w.firstCardID); err != nil {
		return err
	}
	return assertHostApprovalCount(w.node().Store, 1)
}

func (w *m4World) restartHost() error {
	if err := w.settleDormant(); err != nil {
		return err
	}
	w.activeLink.Close()
	w.closeMCP()
	w.activeHost.Settle()
	n := w.node()
	h2, _, err := host.New(n.Store, n.Key, time.Now)
	if err != nil {
		return err
	}
	w.activeHost = h2
	if err := w.attachMCP(h2); err != nil {
		return err
	}
	link2 := node.AttachLocal(h2, n.Home)
	w.activeLink = link2
	w.cleanup(func() {
		link2.Close()
		h2.Settle()
	})
	return w.awaitHostWaitLimit(h2, 20)
}

func (w *m4World) scriptResumeRetryAndDifferentCmd(aiName, retryCmd, diffCmd, postText string) error {
	allow := host.ApprovalResult{Behavior: "allow"}
	body := slices.Concat(
		m4ClaudeBashAskSteps(1, "toolu_retry2", retryCmd, "Install chalk", allow),
		m4ClaudeBashAskSteps(2, "toolu_diff1", diffCmd, "Install typescript", allow),
	)
	return w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 3, postText))
}

func (w *m4World) whenApproveFirstAfterRestart() error {
	return w.actOnCard(w.b2, w.firstCardID, "approve", "", "", "")
}

func (w *m4World) thenNoRetryCardAndNewCard(retryCmd, aiName, newCmd string) error {
	card2, err := w.waitPendingCard(w.b2, "approval", w.b2.aiByName[aiName].ID)
	if err != nil {
		return err
	}
	w.currentCard = card2
	if err := checkApprovalCardBody(card2, w.currentTaskID, "Bash", newCmd, "Install typescript", w.b2.ownerMem); err != nil {
		return err
	}
	if got := countApprovalCards(w.b2); got != 2 {
		return fmt.Errorf("approval card count = %d, want 2 (no duplicate for %q)", got, retryCmd)
	}
	return nil
}

func countApprovalCards(b *m4Client) int {
	n := 0
	for _, m := range b.msgs {
		if m.Kind == "approval" {
			n++
		}
	}
	return n
}

func (w *m4World) thenResumedRetryCompletes(aiName, wantPost string) error {
	if err := w.thenAIPostsAndSettles(aiName, wantPost); err != nil {
		return err
	}
	starts, err := w.waitLogs("start "+m4VendorOf(aiName)+" ", 2)
	if err != nil {
		return err
	}
	if len(starts) != 2 || starts[0] != starts[1] {
		return fmt.Errorf("%s starts = %v, want 2 identical session starts", aiName, starts)
	}
	if got := countApprovalCards(w.b1); got != 2 {
		return fmt.Errorf("total approval cards = %d, want 2", got)
	}
	return assertHostApprovalCount(w.node().Store, 0)
}

func (w *m4World) whenTaskTimesOutShort(aiName, cmd string) error {
	timeoutDeny := host.ApprovalResult{Behavior: "deny", Message: m4TimeoutText}
	body := m4ClaudeBashAskSteps(1, "toolu_short1", cmd, "Short wait", timeoutDeny)
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 0, "")); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Short wait task", "Test reassignment")
	w.currentTaskID = tid
	if err != nil {
		return err
	}
	card, err := w.waitPendingCard(w.b1, "approval", w.b1.aiByName[aiName].ID)
	if err != nil {
		return err
	}
	w.currentCard, w.firstCardID = card, card.ID
	if _, err := w.waitLogs("exit "+m4VendorOf(aiName)+" ", 1); err != nil {
		return err
	}
	return w.settleDormant()
}

func (w *m4World) whenOwnerReassignsTask(newAI, postText string) error {
	if err := w.setScript(m4VendorOf(newAI), m4CodexTurnDoc(m4CodexThread, "turn-r", nil, 1, postText)); err != nil {
		return err
	}
	assign := proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Op:          "assign", ID: proto.ID(w.currentTaskID),
		},
		Assignee: proto.ID(w.b1.aiByName[newAI].ID),
	}
	return w.sendOK(w.b1, assign)
}

func (w *m4World) thenReassignedCardAndRefusal(wantStatus, newAI, wantPost, wantCode string) error {
	if err := w.awaitCardStatus(w.b1, w.currentCard.ID, wantStatus); err != nil {
		return err
	}
	if err := w.thenAIPostsAndSettles(newAI, wantPost); err != nil {
		return err
	}
	return w.actOnCard(w.b1, w.currentCard.ID, "approve", "", "", wantCode)
}

func (w *m4World) whenSecondTaskLeavesTwoPendingCards(aiName, cmd string) error {
	reqArgs := map[string]any{
		"question": "Drop legacy build script?",
		"options":  []map[string]string{{"label": "Keep", "effect": "Safe"}, {"label": "Drop", "effect": "Lean"}},
	}
	timeoutDeny := host.ApprovalResult{Behavior: "deny", Message: m4TimeoutText}
	body := slices.Concat(
		m4GenericToolCall(1, "request_decision", reqArgs),
		m4ClaudeBashAskSteps(2, "toolu_short2", cmd, "Clean dist", timeoutDeny),
	)
	if err := w.setScript(m4VendorOf(aiName), m4ClaudeTurnDoc(body, 0, "")); err != nil {
		return err
	}
	tid, err := w.startTaskFor(aiName, "Second task", "Test cancellation expiry")
	w.secondTaskID = tid
	if err != nil {
		return err
	}
	return w.collectTaskTwoPendingCards(aiName)
}

func (w *m4World) collectTaskTwoPendingCards(aiName string) error {
	aiID := w.b1.aiByName[aiName].ID
	decCard, err := w.waitPendingCard(w.b1, "decision", aiID)
	if err != nil {
		return err
	}
	appCard, err := w.waitPendingCard(w.b1, "approval", aiID)
	if err != nil {
		return err
	}
	w.task2DecID, w.task2AppID = decCard.ID, appCard.ID
	if _, err := w.waitLogs("exit "+m4VendorOf(aiName)+" ", 2); err != nil {
		return err
	}
	return w.settleDormant()
}

func (w *m4World) whenOwnerCancelsSecondTask() error {
	cancel := proto.TaskCommand{
		HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Op:          "cancel", ID: proto.ID(w.secondTaskID),
	}
	if err := w.sendOK(w.b1, cancel); err != nil {
		return err
	}
	return w.awaitTaskSessionRemoved(w.secondTaskID)
}

func (w *m4World) awaitTaskSessionRemoved(taskID string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		hasTask := func(s host.Session) bool { return s.Task == taskID }
		if !slices.ContainsFunc(w.activeHost.Sessions.All(), hasTask) {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for cancelled task session %s cleanup", taskID)
}

func (w *m4World) thenCancelledCardsExpiredAndRefused(wantStatus, wantCode string) error {
	if err := w.awaitCardStatus(w.b1, w.task2AppID, wantStatus); err != nil {
		return err
	}
	if err := w.awaitCardStatus(w.b1, w.task2DecID, wantStatus); err != nil {
		return err
	}
	if err := w.actOnCard(w.b2, w.task2AppID, "approve", "", "", wantCode); err != nil {
		return err
	}
	return w.actOnCard(w.b2, w.task2DecID, "choose", "A", "", wantCode)
}

func (w *m4World) whenAIStartsPausableTurn(aiName, startPost, nextPost string) error {
	steps := slices.Concat(
		codexStartupSteps(),
		m4CodexInterruptTurnSteps("turn-1", 1, startPost),
		codexTurnSteps("turn-2", 2, "", nextPost, false),
	)
	if err := w.setScript(m4VendorOf(aiName), map[string]any{"session": m4CodexThread, "steps": steps}); err != nil {
		return err
	}
	if err := w.postOwnerMention(aiName, "@"+aiName+" begin analysis"); err != nil {
		return err
	}
	return w.awaitAIPost(aiName, startPost, 10*time.Second)
}

func (w *m4World) sendGroupOp(op string) error {
	cmd := proto.GroupCommand{
		HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Op:          op,
	}
	return w.sendOK(w.b1, cmd)
}

func (w *m4World) whenOwnerPausesGroup() error {
	w.pauseStarted = time.Now()
	return w.sendGroupOp("pause")
}

func (w *m4World) thenTurnInterruptedWithinTwoSeconds(aiName, wantReason, wantCode string) error {
	aiID := w.b1.aiByName[aiName].ID
	ready := func() bool {
		_, paused := m4SystemMsg(w.b1, wantCode)
		return paused && w.b1.group.Paused != nil && viewHasInterruptedMsg(&w.b1.homeView, aiID, wantReason)
	}
	return w.b1.drainUntil(time.Until(w.pauseStarted.Add(2*time.Second)), ready, nil)
}

func m4SystemMsg(b *m4Client, wantCode string) (proto.MsgRow, bool) {
	for _, m := range b.msgs {
		var sc proto.SystemCode
		if m.Kind == "system" && proto.Decode(m.Body, &sc) == nil && sc.Code == wantCode {
			return m, true
		}
	}
	return proto.MsgRow{}, false
}

func (w *m4World) whenOwnerResumesGroup() error {
	return w.sendGroupOp("resume")
}

func (w *m4World) thenResumedGroupDeliversMentions(aiName, wantPost, wantCode string) error {
	if err := w.thenAIPostsAndSettles(aiName, wantPost); err != nil {
		return err
	}
	resumed, ok := m4SystemMsg(w.b1, wantCode)
	isAnswer := func(id string) bool { return w.b1.msgs[id].Text == wantPost }
	if !ok || w.b1.group.Paused != nil || slices.IndexFunc(w.b1.msgOrder, isAnswer) < slices.Index(w.b1.msgOrder, resumed.ID) {
		return fmt.Errorf("%q was posted before system message %q (found %v), or the group is still paused: %v", wantPost, wantCode, ok, w.b1.msgOrder)
	}
	return awaitHomeInboxCount(w.node().Store, 0)
}

func (w *m4World) thenApprovalTaskStayedAsleep(aiName string) error {
	if err := w.awaitCardStatus(w.b1, w.firstCardID, "pending"); err != nil {
		return err
	}
	if err := w.awaitTaskWait(w.b1, w.currentTaskID, "approval", w.firstCardID); err != nil {
		return err
	}
	starts := matchingFakeCLILogs(w.cluster.LogPath, "start "+m4VendorOf(aiName)+" ")
	if len(starts) != 1 {
		return fmt.Errorf("%s CLI starts = %v, want 1 (task waiting on approval must stay asleep)", aiName, starts)
	}
	return nil
}
