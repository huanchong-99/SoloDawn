//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["m6"] = m6Steps
}

const (
	m6GroupName = "Project"
	m6OwnerName = "Alice"
	m6TaskTitle = "Path event"
	m6TaskPlan  = "Ask Codex"
)

type m6NodeProc struct {
	name string
	root string
	data string
	key  identity.Key
	cmd  *exec.Cmd
	out  syncBuffer
}

func (np *m6NodeProc) cliLog() string {
	return filepath.Join(np.root, "fakecli.log")
}

func (np *m6NodeProc) scriptPath(agent string) string {
	return filepath.Join(np.root, agent+".json")
}

type m6World struct {
	f           fixtures
	cleanup     func(func())
	ctx         context.Context
	root        string
	repoDir     string
	rvURL       string
	ownerKey    identity.Key
	browser     *peerBrowser
	homeNode    *m6NodeProc
	aiNode      *m6NodeProc
	homeLink    *peerBrowserConn
	aiLink      *peerBrowserConn
	groupID     proto.ID
	group       proto.GroupRow
	aiByName    map[string]proto.AIMemberRow
	nodeByAI    map[string]*m6NodeProc
	tasksByNum  map[int]proto.TaskRow
	msgs        map[string]proto.MsgRow
	msgOrder    []string
	setupCardID string
	holdCardID  string
	taskID      string
	callID      string
	ids         seqIDGen
}

func m6Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m6World{
		f:          f,
		cleanup:    afterScenario(sc),
		aiByName:   map[string]proto.AIMemberRow{},
		nodeByAI:   map[string]*m6NodeProc{},
		tasksByNum: map[int]proto.TaskRow{},
		msgs:       map[string]proto.MsgRow{},
	}
	registerM6RemoteSessionSteps(sc, w)
	registerM6CallSteps(sc, w)
}

func registerM6RemoteSessionSteps(sc *godog.ScenarioContext, w *m6World) {
	sc.Step(`^two owned nodes over ctl and a project group on the first node$`, w.givenTwoNodesAndGroup)
	sc.Step(`^"([^"]+)" on the second node is scripted to answer "([^"]+)" with "([^"]+)" and "([^"]+)" with "([^"]+)" as "([^"]+)"$`, w.givenTwoTurnScript)
	sc.Step(`^the owner joins "([^"]+)" on the second node to the group as "([^"]+)"$`, w.givenJoinedRemoteAI)
	sc.Step(`^the owner mentions "([^"]+)" in the group with "([^"]+)"$`, w.whenOwnerMentionsAI)
	sc.Step(`^"([^"]+)" posts "([^"]+)" from the second node and enters waiting$`, w.thenAIPostsAndWaits)
	sc.Step(`^one "([^"]+)" CLI process existed throughout on the second node$`, w.thenOneCLIProcessRan)
}

func registerM6CallSteps(sc *godog.ScenarioContext, w *m6World) {
	sc.Step(`^one M6 node over ctl with a project group and joined AIs "([^"]+)" and "([^"]+)"$`, func(caller, callee string) error {
		return w.givenOneNodeWithTwoAIs(0, caller, callee)
	})
	sc.Step(`^one M6 node over ctl with a project group, ask_wait_s (\d+), and joined AIs "([^"]+)" and "([^"]+)"$`, func(waitS int, caller, callee string) error {
		return w.givenOneNodeWithTwoAIs(waitS, caller, callee)
	})
	sc.Step(`^the owner joins "([^"]+)" on the first node as "([^"]+)" and "([^"]+)" on the second node as "([^"]+)"$`, w.givenSplitAIs)
	sc.Step(`^"([^"]+)" on a task asks "([^"]+)" "([^"]+)" and "([^"]+)" replies "([^"]+)" while the call is open$`, w.whenTaskAsksAndCalleeRepliesOpen)
	sc.Step(`^"([^"]+)" receives "([^"]+)" in its running turn, posts "([^"]+)", and started only the group session and one task session process$`, w.thenCallerAnsweredInTurn)
	sc.Step(`^the M6 task thread holds the answered call and reply with elapsed milliseconds$`, func() error {
		return w.thenThreadHoldsAnsweredCall(0)
	})
	sc.Step(`^"([^"]+)" is busy on another task and "([^"]+)" on a task asks "([^"]+)" "([^"]+)"$`, w.whenCalleeBusyAndCallerAsks)
	sc.Step(`^after the wait "([^"]+)" receives a pending receipt, posts "([^"]+)", and enters waiting while the call stays waiting$`, w.thenCallerGetsReceiptWhileWaiting)
	sc.Step(`^busy "([^"]+)" finishes its turn and replies "([^"]+)" to the call$`, w.whenBusyCalleeFinishesAndReplies)
	sc.Step(`^"([^"]+)" receives the reply as a later turn, posts "([^"]+)", and started only the group session and one task session process$`, w.thenCallerGetsLateReplyTurn)
	sc.Step(`^the M6 task thread holds the answered call and reply with elapsed milliseconds at least (\d+)$`, w.thenThreadHoldsAnsweredCall)
	sc.Step(`^"([^"]+)" and "([^"]+)" chain 4 calls in the M6 group and "([^"]+)" attempts a 5th call$`, w.whenFiveHopCallChainRuns)
	sc.Step(`^the 5th ask call fails with "([^"]+)" and "([^"]+)" posts "([^"]+)"$`, w.thenFifthCallRefusedAndPosted)
	sc.Step(`^the M6 group holds 4 call messages at depths 1 to 4 and none at depth 5$`, w.thenFourCallsStoredAtDepthsOneToFour)
}

func (w *m6World) givenTwoNodesAndGroup() error {
	return w.setupNodesAndGroup(true)
}

func (w *m6World) givenOneNodeWithTwoAIs(waitS int, caller, callee string) error {
	if err := w.setupNodesAndGroup(false); err != nil {
		return err
	}
	if waitS > 0 {
		if err := w.setGroupAskWait(waitS); err != nil {
			return err
		}
	}
	if err := w.joinAIOnNode(w.homeNode, w.homeLink, "claude", caller); err != nil {
		return err
	}
	return w.joinAIOnNode(w.homeNode, w.homeLink, "codex", callee)
}

func (w *m6World) givenSplitAIs(a1, m1, a2, m2 string) error {
	if err := w.joinAIOnNode(w.homeNode, w.homeLink, a1, m1); err != nil {
		return err
	}
	return w.joinAIOnNode(w.aiNode, w.aiLink, a2, m2)
}

func (w *m6World) setupNodesAndGroup(twoNodes bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	w.ctx = ctx
	w.cleanup(cancel)
	if err := w.prepareRootAndRepo(); err != nil {
		return err
	}
	rvURL, err := startAcceptanceRelayRV(w.cleanup)
	if err != nil {
		return err
	}
	w.rvURL = rvURL
	if err := w.prepareAndStartNodes(twoNodes); err != nil {
		return err
	}
	if err := w.connectOwnerToNodes(twoNodes); err != nil {
		return err
	}
	return w.createAndSyncGroup()
}

func (w *m6World) prepareRootAndRepo() error {
	rawRoot, err := shortTempRoot(w.f.t, w.cleanup, "sdm6")
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(rawRoot)
	if err != nil {
		return err
	}
	w.root = realRoot
	w.repoDir = filepath.Join(realRoot, "repo")
	return initLocalGitRepo(w.repoDir)
}

func (w *m6World) prepareAndStartNodes(twoNodes bool) error {
	w.ownerKey = newRandomPeerKey()
	homeNode, err := w.initNodeDir("home-node")
	if err != nil {
		return err
	}
	w.homeNode = homeNode
	nodes := []*m6NodeProc{homeNode}
	if twoNodes {
		aiNode, err := w.initNodeDir("ai-node")
		if err != nil {
			return err
		}
		w.aiNode = aiNode
		nodes = append(nodes, aiNode)
	}
	return w.seedAndStartNodeList(nodes)
}

func (w *m6World) seedAndStartNodeList(nodes []*m6NodeProc) error {
	entries := w.buildDeviceLog(nodes)
	for _, np := range nodes {
		if err := w.seedNodeDB(np, entries); err != nil {
			return err
		}
		if err := w.startNodeProc(np); err != nil {
			return err
		}
	}
	return nil
}

func (w *m6World) initNodeDir(name string) (*m6NodeProc, error) {
	nodeRoot := filepath.Join(w.root, name)
	data := filepath.Join(nodeRoot, "solodawn")
	if runtime.GOOS == "darwin" {
		data = filepath.Join(nodeRoot, "Library", "Application Support", "solodawn")
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return nil, err
	}
	key, err := identity.LoadKey(data)
	if err != nil {
		return nil, err
	}
	return &m6NodeProc{name: name, root: nodeRoot, data: data, key: key}, nil
}

func (w *m6World) buildDeviceLog(nodes []*m6NodeProc) []proto.Envelope {
	ts := time.Now().UnixMilli()
	ownerID := w.ownerKey.ID()
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(w.ownerKey, ownerID, 1, "browser", "owner", ownerID, ts),
	}
	for i, np := range nodes {
		entries = append(entries, nodetest.SignDeviceAdd(w.ownerKey, ownerID, i+2, "node", np.name, np.key.ID(), ts))
	}
	return entries
}

func (w *m6World) seedNodeDB(np *m6NodeProc, entries []proto.Envelope) error {
	db, err := store.Open(filepath.Join(np.data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	ownerJSON := string(mustMarshalNoEscape(w.ownerKey.ID()))
	nameJSON := string(mustMarshalNoEscape(np.name))
	rvJSON := string(mustMarshalNoEscape([]string{w.rvURL}))
	write := func(tx *store.Tx) error {
		if _, err := tx.Exec(`INSERT INTO setting(key, value) VALUES('owner', ?), ('name', ?), ('rendezvous', ?)`, ownerJSON, nameJSON, rvJSON); err != nil {
			return err
		}
		return seedM6LogEntries(tx, w.ownerKey.ID(), entries)
	}
	return db.Write(write)
}

func seedM6LogEntries(tx *store.Tx, user string, entries []proto.Envelope) error {
	for i, env := range entries {
		if _, err := tx.Exec(`INSERT INTO device_log(user, ver, env) VALUES(?, ?, ?)`, user, i+1, string(mustMarshalNoEscape(env))); err != nil {
			return err
		}
	}
	return nil
}

func (w *m6World) startNodeProc(np *m6NodeProc) error {
	env := append(os.Environ(),
		"HOME="+np.root,
		"XDG_DATA_HOME="+np.root,
		"LocalAppData="+np.root,
		"SOLODAWN_FAKECLI_LOG="+np.cliLog(),
		"SOLODAWN_FAKE_CLAUDE_SCRIPT="+np.scriptPath("claude"),
		"SOLODAWN_FAKE_CODEX_SCRIPT="+np.scriptPath("codex"),
	)
	cmd := exec.Command(w.f.bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.out, &np.out
	if _, err := startNodeCmd(w.f.t, w.cleanup, cmd, np.name, np.data, &np.out); err != nil {
		return err
	}
	np.cmd = cmd
	sock := filepath.Join(np.data, "sock")
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return fmt.Errorf("%s socket %s: %w (%s)", np.name, sock, err, np.out.String())
	}
	return nil
}

func (w *m6World) connectOwnerToNodes(twoNodes bool) error {
	b, err := newPeerBrowser(w.ctx, w.cleanup, w.ownerKey, w.rvURL, nil)
	if err != nil {
		return err
	}
	w.browser = b
	devices := []string{w.homeNode.key.ID()}
	if twoNodes {
		devices = append(devices, w.aiNode.key.ID())
	}
	if err := b.waitOnline(w.ctx, devices...); err != nil {
		return err
	}
	return w.openNodeLinks(twoNodes)
}

func (w *m6World) openNodeLinks(twoNodes bool) error {
	homeLink, err := w.dialAndWelcome(w.homeNode.key.ID())
	if err != nil {
		return err
	}
	w.homeLink = homeLink
	if !twoNodes {
		return nil
	}
	aiLink, err := w.dialAndWelcome(w.aiNode.key.ID())
	if err != nil {
		return err
	}
	w.aiLink = aiLink
	return nil
}

func (w *m6World) dialAndWelcome(device string) (*peerBrowserConn, error) {
	bc, err := w.browser.openCtl(w.ctx, w.cleanup, device)
	if err != nil {
		return nil, err
	}
	if err := checkOwnerWelcome(w.ctx, bc.conn, device); err != nil {
		return nil, err
	}
	return bc, nil
}

func (w *m6World) createAndSyncGroup() error {
	w.groupID = w.ids.next()
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.groupID, Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Op:          "create",
		},
		Name: m6GroupName, Kind: "project", Repo: w.repoDir, Base: "main",
		Forge: "none", Checks: []string{}, MemberName: m6OwnerName,
	}
	if err := w.sendSignedCmd(w.homeLink.conn, w.ownerKey, create); err != nil {
		return err
	}
	if err := w.requestGroupSync(); err != nil {
		return err
	}
	return w.waitState(func() bool { return w.group.ID == string(w.groupID) })
}

func (w *m6World) requestGroupSync() error {
	raw, err := w.homeLink.conn.Request(w.ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.groupID), Since: 0, StateOnly: false}
	})
	if err != nil {
		return err
	}
	var synced proto.Synced
	return proto.Decode(raw, &synced)
}

func (w *m6World) setGroupAskWait(waitS int) error {
	limits := w.group.Limits
	limits.AskWaitS = waitS
	setCmd := proto.GroupSet{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Op:          "set",
		},
		Limits: &limits,
	}
	if err := w.sendSignedCmd(w.homeLink.conn, w.ownerKey, setCmd); err != nil {
		return err
	}
	return w.waitState(func() bool { return w.group.Limits.AskWaitS == waitS })
}

func (w *m6World) sendSignedCmd(conn *peer.Connection, signer identity.Key, payload any) error {
	env := signer.Sign(string(mustMarshalNoEscape(payload)))
	raw, err := conn.Request(w.ctx, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
	if err != nil {
		return err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return fmt.Errorf("cmd failed: %s (%v)", raw, err)
	}
	return nil
}

func (w *m6World) givenTwoTurnScript(agent, m1, a1, m2, a2, member string) error {
	turn1 := formatM6MentionTurn(member, m1)
	turn2 := formatM6MentionTurn(member, m2)
	doc := buildM6TwoTurnDoc(agent, turn1, a1, turn2, a2)
	return writeM6Script(w.aiNode, agent, doc)
}

func buildM6TwoTurnDoc(agent, turn1, a1, turn2, a2 string) map[string]any {
	if agent == "claude" {
		steps := slices.Concat(claudeTurnSteps(1, true, turn1, a1, ""), claudeTurnSteps(2, false, turn2, a2, ""))
		return map[string]any{"steps": append(steps, map[string]any{"dir": "eof"})}
	}
	steps := slices.Concat(codexStartupSteps(), codexTurnSteps("turn-1", 1, turn1, a1, false), codexTurnSteps("turn-2", 2, turn2, a2, false))
	return map[string]any{"session": "thr-m6-codex-1", "steps": append(steps, map[string]any{"dir": "eof"})}
}

func writeM6Script(np *m6NodeProc, agent string, doc map[string]any) error {
	return os.WriteFile(np.scriptPath(agent), mustMarshalNoEscape(doc), 0o600)
}

func formatM6MentionTurn(member, body string) string {
	header := fmt.Sprintf("[SoloDawn] group %q · you are %q · group session", m6GroupName, member)
	block := fmt.Sprintf("== 1 of 1 · mention · from %q in main · instruction", m6OwnerName)
	return header + "\n" + block + "\n" + body
}

func (w *m6World) givenJoinedRemoteAI(agent, memberName string) error {
	return w.joinAIOnNode(w.aiNode, w.aiLink, agent, memberName)
}

func (w *m6World) joinAIOnNode(np *m6NodeProc, link *peerBrowserConn, agent, memberName string) error {
	if _, err := requestHostInfo(w.ctx, link.conn); err != nil {
		return err
	}
	setCmd := proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: np.key.ID()},
		Agent:       agent,
		Account:     "personal",
	}
	if err := w.sendSignedCmd(link.conn, w.ownerKey, setCmd); err != nil {
		return err
	}
	joinCmd := proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: np.key.ID()},
		Group:       w.groupID, Home: w.homeNode.key.ID(), Agent: agent, Name: memberName, Secretary: false,
	}
	if err := w.sendSignedCmd(link.conn, w.ownerKey, joinCmd); err != nil {
		return err
	}
	w.nodeByAI[memberName] = np
	return w.awaitJoinedMember(np, agent, memberName)
}

func (w *m6World) awaitJoinedMember(np *m6NodeProc, agent, memberName string) error {
	check := func() (bool, error) {
		ai, ok := w.aiByName[memberName]
		if !ok {
			return false, nil
		}
		return w.verifyJoinedRow(np, ai, agent)
	}
	return w.waitCheck(check)
}

func (w *m6World) verifyJoinedRow(np *m6NodeProc, ai proto.AIMemberRow, agent string) (bool, error) {
	if ai.CLI != agent || ai.Host != np.key.ID() {
		return false, fmt.Errorf("unexpected AI member row: %+v", ai)
	}
	return true, nil
}

func (w *m6World) whenOwnerMentionsAI(member, text string) error {
	ai := w.aiByName[member]
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Kind:        "text",
		Text:        text,
		Mentions:    []proto.ID{proto.ID(ai.ID)},
		Body:        proto.EmptyBody{},
	}
	return w.sendSignedCmd(w.homeLink.conn, w.ownerKey, post)
}

func (w *m6World) thenAIPostsAndWaits(member, wantText string) error {
	return w.awaitPostAndWaiting(member, wantText, "")
}

func (w *m6World) awaitPostAndWaiting(member, wantText, wantThread string) error {
	return w.waitState(func() bool {
		return w.hasTextPost(member, wantText, wantThread) && w.aiByName[member].State == "waiting"
	})
}

func (w *m6World) hasTextPost(member, wantText, wantThread string) bool {
	aiID := w.aiByName[member].ID
	for _, id := range w.msgOrder {
		if m6MatchesTextMsg(w.msgs[id], aiID, wantText, wantThread) {
			return true
		}
	}
	return false
}

func m6MatchesTextMsg(m proto.MsgRow, byID, wantText, wantThread string) bool {
	if m.By != byID || m.Kind != "text" || m.Text != wantText {
		return false
	}
	return wantThread == "" || (m.Thread != nil && *m.Thread == wantThread)
}

func (w *m6World) whenTaskAsksAndCalleeRepliesOpen(caller, callee, question, replier, answer string) error {
	if err := w.prepareTasksAndHoldCallee(caller, callee); err != nil {
		return err
	}
	postText := "Confirmed in turn: " + answer
	if err := w.scriptOpenAskTurn(caller, callee, question, answer, postText); err != nil {
		return err
	}
	if err := w.releaseCallerTaskAndAwaitCall(question); err != nil {
		return err
	}
	return w.releaseCalleeWithReply(replier, caller, question, answer)
}

func (w *m6World) thenCallerAnsweredInTurn(caller, _, postText string) error {
	if err := w.awaitPostAndWaiting(caller, postText, w.taskID); err != nil {
		return err
	}
	return checkStartLineCount(w.nodeByAI[caller].cliLog(), "claude", 2)
}

func (w *m6World) whenCalleeBusyAndCallerAsks(callee, caller, target, question string) error {
	if err := w.prepareTasksAndHoldCallee(caller, callee); err != nil {
		return err
	}
	if err := w.scriptBusyAskAndLateReply(caller, target, question); err != nil {
		return err
	}
	return w.releaseCallerTaskAndAwaitCall(question)
}

func (w *m6World) thenCallerGetsReceiptWhileWaiting(caller, postText string) error {
	if err := w.awaitPostAndWaiting(caller, postText, w.taskID); err != nil {
		return err
	}
	call := w.msgs[w.callID]
	if call.State == nil || call.State.Status != "waiting" {
		return fmt.Errorf("expected call %s to remain waiting, got %+v", w.callID, call.State)
	}
	return nil
}

func (w *m6World) whenBusyCalleeFinishesAndReplies(callee, answer string) error {
	call := w.msgs[w.callID]
	return w.releaseCalleeWithReply(callee, "Claude", call.Text, answer)
}

func (w *m6World) thenCallerGetsLateReplyTurn(caller, postText string) error {
	if err := w.awaitPostAndWaiting(caller, postText, w.taskID); err != nil {
		return err
	}
	return checkStartLineCount(w.nodeByAI[caller].cliLog(), "claude", 2)
}

func (w *m6World) prepareTasksAndHoldCallee(caller, callee string) error {
	if err := writeM6Script(w.nodeByAI[caller], "claude", m6ClaudeSetupDoc(caller)); err != nil {
		return err
	}
	if err := w.whenOwnerMentionsAI(caller, "Set up M6 tasks."); err != nil {
		return err
	}
	if err := w.waitState(w.hasSetupTasksAndCard); err != nil {
		return err
	}
	w.taskID = w.tasksByNum[2].ID
	return w.assignHoldTaskToCallee(callee)
}

func (w *m6World) hasSetupTasksAndCard() bool {
	return w.tasksByNum[1].ID != "" && w.tasksByNum[2].ID != "" && w.setupCardID != ""
}

func (w *m6World) assignHoldTaskToCallee(callee string) error {
	if err := writeM6Script(w.nodeByAI[callee], "codex", m6CodexHoldDoc()); err != nil {
		return err
	}
	assign := proto.TaskAssign{
		TaskCommand: proto.TaskCommand{
			HomeCommand: proto.HomeCommand{Type: "task", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
			Op:          "assign",
			ID:          proto.ID(w.tasksByNum[1].ID),
		},
		Assignee: proto.ID(w.aiByName[callee].ID),
	}
	if err := w.sendSignedCmd(w.homeLink.conn, w.ownerKey, assign); err != nil {
		return err
	}
	return w.waitState(func() bool { return w.holdCardID != "" })
}

func (w *m6World) releaseCallerTaskAndAwaitCall(question string) error {
	act := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Msg:         proto.ID(w.setupCardID),
		Action:      "choose",
		Arg:         "A",
	}
	if err := w.sendSignedCmd(w.homeLink.conn, w.ownerKey, act); err != nil {
		return err
	}
	return w.waitState(func() bool {
		w.callID = w.findCallInThread(w.taskID, question)
		return w.callID != ""
	})
}

func (w *m6World) findCallInThread(taskID, question string) string {
	for _, id := range w.msgOrder {
		m := w.msgs[id]
		if m.Kind == "call" && m.Text == question && m.Thread != nil && *m.Thread == taskID {
			return m.ID
		}
	}
	return ""
}

func (w *m6World) releaseCalleeWithReply(callee, caller, question, answer string) error {
	np := w.nodeByAI[callee]
	turnText := formatM6CallMentionTurn(callee, caller, question, w.callID)
	if err := writeM6Script(np, "codex", m6CodexReplyDoc(turnText, w.callID, answer)); err != nil {
		return err
	}
	act := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID},
		Msg:         proto.ID(w.holdCardID),
		Action:      "approve",
	}
	return w.sendSignedCmd(w.homeLink.conn, w.ownerKey, act)
}

func m6ClaudeSetupDoc(caller string) map[string]any {
	stdinText := formatM6MentionTurn(caller, "Set up M6 tasks.")
	okRes := map[string]any{"isError": false}
	askQ := map[string]any{
		"tool_name":   "AskUserQuestion",
		"tool_use_id": "toolu_m6_setup",
		"input": map[string]any{
			"questions": []map[string]any{{
				"question":    "Proceed to task?",
				"header":      "Setup",
				"options":     []map[string]string{{"label": "Proceed", "description": "Start"}, {"label": "Wait", "description": "Hold"}},
				"multiSelect": false,
			}},
		},
	}
	body := slices.Concat(
		mcpCallSteps(1, "start_task", map[string]any{"title": "Hold slot", "plan": "Wait on approval"}, okRes),
		mcpCallSteps(2, "start_task", map[string]any{"title": m6TaskTitle, "plan": m6TaskPlan}, okRes),
		mcpCallSteps(3, "approve", askQ, okRes),
	)
	return map[string]any{"steps": append(claudeToolTurn(true, stdinText, "", body), map[string]any{"dir": "eof"})}
}

func (w *m6World) scriptOpenAskTurn(caller, callee, question, answer, postText string) error {
	assignTurn := formatM6TaskAssignTurn(caller)
	wantRes := m6ToolTextResult(false, string(mustMarshalNoEscape(map[string]string{"answer": answer})))
	askSteps := mcpCallSteps(1, "ask", map[string]any{"to": callee, "text": question}, wantRes)
	steps := claudeToolTurn(true, assignTurn, "", append(askSteps, mcpPostSteps(2, postText)...))
	steps = append(steps, map[string]any{"dir": "eof"})
	return writeM6Script(w.nodeByAI[caller], "claude", map[string]any{"steps": steps})
}

func (w *m6World) scriptBusyAskAndLateReply(caller, callee, question string) error {
	assignTurn := formatM6TaskAssignTurn(caller)
	replyTurn := formatM6ReplyTurn(caller, callee, "Fixtures are ready.")
	askSteps := mcpCallSteps(1, "ask", map[string]any{"to": callee, "text": question}, map[string]any{"isError": false})
	turn1 := claudeToolTurn(true, assignTurn, "", append(askSteps, mcpPostSteps(2, "Waiting on receipt.")...))
	turn2 := claudeTurnSteps(3, false, replyTurn, "Late answer: Fixtures are ready.", "")
	steps := append(slices.Concat(turn1, turn2), map[string]any{"dir": "eof"})
	return writeM6Script(w.nodeByAI[caller], "claude", map[string]any{"steps": steps})
}

func (w *m6World) thenThreadHoldsAnsweredCall(minMS int64) error {
	return w.waitCheck(func() (bool, error) {
		return w.verifyAnsweredCall(minMS)
	})
}

func (w *m6World) verifyAnsweredCall(minMS int64) (bool, error) {
	call := w.msgs[w.callID]
	if call.State == nil || call.State.Status != "answered" {
		return false, nil
	}
	reply, ok := w.msgs[m6StrVal(call.State.Reply)]
	if !ok {
		return false, nil
	}
	return true, checkM6CallAndReply(call, reply, w.taskID, minMS)
}

func checkM6CallAndReply(call, reply proto.MsgRow, taskID string, minMS int64) error {
	if call.State.MS == nil || *call.State.MS < minMS {
		return fmt.Errorf("call elapsed ms = %v, want >= %d", call.State.MS, minMS)
	}
	if !m6InSameThread(call, reply, taskID) || m6StrVal(reply.Re) != call.ID {
		return fmt.Errorf("mismatched thread or re on call=%+v reply=%+v", call, reply)
	}
	return nil
}

func m6InSameThread(call, reply proto.MsgRow, taskID string) bool {
	return m6StrVal(call.Thread) == taskID && m6StrVal(reply.Thread) == taskID && reply.Kind == "reply"
}

func m6StrVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (w *m6World) whenFiveHopCallChainRuns(caller, callee, fifthCaller string) error {
	const limitErr = "limit: call depth 5 exceeds the ask_depth limit of 4"
	claudeDoc := buildM6ClaudeChainDoc(caller, callee, limitErr, "Depth 5 refused with limit.")
	if err := writeM6Script(w.nodeByAI[fifthCaller], "claude", claudeDoc); err != nil {
		return err
	}
	codexDoc := buildM6CodexChainDoc(caller)
	if err := writeM6Script(w.nodeByAI[callee], "codex", codexDoc); err != nil {
		return err
	}
	return w.whenOwnerMentionsAI(caller, "Run five-hop call chain.")
}

func (w *m6World) thenFifthCallRefusedAndPosted(_, caller, postText string) error {
	return w.awaitPostAndWaiting(caller, postText, "")
}

func (w *m6World) thenFourCallsStoredAtDepthsOneToFour() error {
	var depths []int
	for _, id := range w.msgOrder {
		if d, ok := m6DecodeCallDepth(w.msgs[id]); ok {
			depths = append(depths, d)
		}
	}
	if !slices.Equal(depths, []int{1, 2, 3, 4}) {
		return fmt.Errorf("stored call depths = %v, want [1 2 3 4]", depths)
	}
	return nil
}

func m6DecodeCallDepth(m proto.MsgRow) (int, bool) {
	if m.Kind != "call" {
		return 0, false
	}
	var body proto.CallBody
	if proto.Decode(m.Body, &body) != nil {
		return 0, false
	}
	return body.Depth, true
}

func buildM6ClaudeChainDoc(caller, callee, wantErr, finalPost string) map[string]any {
	t1In := formatM6MentionTurn(caller, "Run five-hop call chain.")
	okRes := map[string]any{"isError": false}
	errRes := m6ToolTextResult(true, wantErr)
	t1 := claudeToolTurn(true, t1In, "", mcpCallSteps(1, "ask", map[string]any{"to": callee, "text": "Hop 1", "wait": false}, okRes))
	t2 := claudeToolTurn(false, "", "", mcpCallSteps(2, "ask", map[string]any{"to": callee, "text": "Hop 3", "wait": false}, okRes))
	lastCall := mcpCallSteps(3, "ask", map[string]any{"to": callee, "text": "Hop 5"}, errRes)
	t3 := claudeToolTurn(false, "", "", append(lastCall, mcpPostSteps(4, finalPost)...))
	steps := append(slices.Concat(t1, t2, t3), map[string]any{"dir": "eof"})
	return map[string]any{"steps": steps}
}

func buildM6CodexChainDoc(caller string) map[string]any {
	okRes := map[string]any{"isError": false}
	steps := slices.Concat(
		codexStartupSteps(),
		codexToolTurn("turn-1", "", mcpCallSteps(1, "ask", map[string]any{"to": caller, "text": "Hop 2", "wait": false}, okRes)),
		codexToolTurn("turn-2", "", mcpCallSteps(2, "ask", map[string]any{"to": caller, "text": "Hop 4", "wait": false}, okRes)),
		[]map[string]any{{"dir": "eof"}},
	)
	return map[string]any{"session": "thr-m6-codex-chain", "steps": steps}
}

func m6CodexHoldDoc() map[string]any {
	steps := slices.Concat(codexStartupSteps(), codexToolTurn("turn-hold", "", codexApprovalSteps()), []map[string]any{{"dir": "eof"}})
	return map[string]any{"session": "thr-m6-codex-hold", "steps": steps}
}

func m6CodexReplyDoc(stdinText, callID, answer string) map[string]any {
	okRes := m6ToolTextResult(false, `{"ok":true}`)
	call := mcpCallSteps(1, "reply", map[string]any{"call": callID, "text": answer}, okRes)
	steps := slices.Concat(codexStartupSteps(), codexToolTurn("turn-reply", stdinText, call), []map[string]any{{"dir": "eof"}})
	return map[string]any{"session": "thr-m6-codex-reply", "steps": steps}
}

func m6ToolTextResult(isErr bool, text string) map[string]any {
	return map[string]any{
		"isError": isErr,
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

func formatM6TaskAssignTurn(caller string) string {
	return strings.Join([]string{
		fmt.Sprintf("[SoloDawn] group %q · you are %q · task session T-2", m6GroupName, caller),
		"== 1 of 1 · assign · instruction",
		fmt.Sprintf("Task T-2 %q", m6TaskTitle),
		"Branch: task/2-path-event (this folder is its worktree)",
		"Plan: " + m6TaskPlan,
		"Reviewer: none   Pass score: 90",
	}, "\n")
}

func formatM6CallMentionTurn(callee, caller, body, callID string) string {
	return strings.Join([]string{
		fmt.Sprintf("[SoloDawn] group %q · you are %q · group session", m6GroupName, callee),
		fmt.Sprintf("== 1 of 1 · mention · from %q (AI) in T-2 · information · call id %s", caller, callID),
		body,
	}, "\n")
}

func formatM6ReplyTurn(caller, callee, body string) string {
	return strings.Join([]string{
		fmt.Sprintf("[SoloDawn] group %q · you are %q · task session T-2", m6GroupName, caller),
		fmt.Sprintf("== 1 of 1 · reply · from %q (AI) in T-2 · information", callee),
		body,
	}, "\n")
}

func (w *m6World) waitState(ready func() bool) error {
	return w.waitCheck(func() (bool, error) { return ready(), nil })
}

func (w *m6World) waitCheck(check func() (bool, error)) error {
	for {
		done, err := check()
		if err != nil || done {
			return err
		}
		if err := w.readNextPush(); err != nil {
			return err
		}
	}
}

func (w *m6World) readNextPush() error {
	rawFrame, err := w.homeLink.nextPush(w.ctx)
	if err != nil {
		return fmt.Errorf("waiting for home push: %w\n%s", err, w.nodeLogs())
	}
	w.recordRowsFrame(rawFrame)
	return nil
}

func (w *m6World) nodeLogs() string {
	out := "home=" + w.homeNode.out.String()
	if w.aiNode != nil {
		out += "\nai=" + w.aiNode.out.String()
	}
	return out
}

func (w *m6World) recordRowsFrame(rawFrame json.RawMessage) {
	var batch proto.Rows
	if proto.Decode(rawFrame, &batch) != nil || batch.Group != string(w.groupID) {
		return
	}
	for _, raw := range batch.Rows {
		w.recordRow(raw)
	}
}

func (w *m6World) recordRow(raw json.RawMessage) {
	var hdr proto.RowHeader
	if proto.Decode(raw, &hdr) != nil {
		return
	}
	switch hdr.Type {
	case "group":
		_ = proto.Decode(raw, &w.group)
	case "member":
		w.recordMemberRow(raw)
	case "task":
		w.recordTaskRow(raw)
	case "msg":
		w.recordMsgRow(raw)
	}
}

func (w *m6World) recordTaskRow(raw json.RawMessage) {
	var t proto.TaskRow
	if proto.Decode(raw, &t) == nil {
		w.tasksByNum[t.Num] = t
	}
}

func (w *m6World) recordMemberRow(raw json.RawMessage) {
	var ai proto.AIMemberRow
	if proto.Decode(raw, &ai) == nil && ai.Kind == "ai" {
		w.aiByName[ai.Name] = ai
	}
}

func (w *m6World) recordMsgRow(raw json.RawMessage) {
	var m proto.MsgRow
	if proto.Decode(raw, &m) != nil {
		return
	}
	if _, seen := w.msgs[m.ID]; !seen {
		w.msgOrder = append(w.msgOrder, m.ID)
	}
	w.msgs[m.ID] = m
	w.notePendingCard(m)
}

func (w *m6World) notePendingCard(m proto.MsgRow) {
	if m.State == nil || m.State.Status != "pending" {
		return
	}
	if m.Kind == "decision" {
		w.setupCardID = m.ID
	}
	if m.Kind == "approval" {
		w.holdCardID = m.ID
	}
}

func (w *m6World) thenOneCLIProcessRan(agent string) error {
	return checkStartLineCount(w.aiNode.cliLog(), agent, 1)
}

func checkStartLineCount(path, agent string, want int) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := m6AgentLogLines(string(raw), agent)
	if len(lines) != want {
		return fmt.Errorf("expected %d %s start line(s), got %q", want, agent, string(raw))
	}
	return verifyStartLines(lines, agent)
}

func verifyStartLines(lines []string, agent string) error {
	prefix := "start " + agent + " "
	for _, line := range lines {
		if id, ok := strings.CutPrefix(line, prefix); !ok || id == "" {
			return fmt.Errorf("unexpected lifecycle line %q for %s", line, agent)
		}
	}
	return nil
}

func m6AgentLogLines(content, agent string) []string {
	var out []string
	needle := " " + agent + " "
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if strings.Contains(line, needle) {
			out = append(out, line)
		}
	}
	return out
}
