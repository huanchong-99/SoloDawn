//go:build acceptance

package features

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/home"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/node"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["m3"] = m3Steps
}

const (
	m3SecretToken = "sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"
	m3SecretError = "secret: this looks like a secret; remove it and send again"
)

// TestM3MCPShim runs host.RunShim when invoked as a child MCP server process
// with the socket path and --token arguments after "--". During a regular go
// test run flag.Args() is empty and the test returns at once.
func TestM3MCPShim(_ *testing.T) {
	args := flag.Args()
	if len(args) == 0 {
		return
	}
	dial := func() (net.Conn, error) {
		return net.Dial("unix", args[0])
	}
	os.Exit(host.RunShim(args[1:], dial, os.Stdin, os.Stdout, os.Stderr))
}

type m3Event struct {
	kind   string
	member string
	state  string
	text   string
}

type m3ToolCheck struct {
	name             string
	want             []proto.Tool
	codex            []proto.Tool
	claudeStructured []proto.Tool
}

type m3World struct {
	f           fixtures
	cleanup     func(func())
	tb          scenarioTB
	root        string
	repoSrc     string
	cluster     *nodetest.Cluster
	activeHost  *host.Host
	sockPath    string
	closeMCP    func()
	browser     homeFrameClient
	browserConn *home.Conn
	nextReq     int64
	ids         seqIDGen
	scriptSeq   int
	groups      map[string]string
	aiByName    map[string]proto.AIMemberRow
	msgs        []proto.MsgRow
	events      []m3Event
	pendingCard string
	holdLink    *m3HoldLink
	restartLink *m3HoldLink
	toolChecks  []m3ToolCheck
}

func m3Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m3World{
		f:        f,
		groups:   map[string]string{},
		aiByName: map[string]proto.AIMemberRow{},
	}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.activeHost != nil {
			w.waitAllDormant(w.activeHost)
		}
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.tb = scenarioTB{TB: f.t, cleanup: w.cleanup}
	registerM3SetupSteps(sc, w)
	registerM3ActionSteps(sc, w)
	registerM3AssertSteps(sc, w)
}

func registerM3SetupSteps(sc *godog.ScenarioContext, w *m3World) {
	sc.Step(`^an in-process M3 node with a local MCP socket$`, w.givenBareNode)
	sc.Step(`^an in-process M3 node with group "([^"]+)" and configured AI "([^"]+)" on "([^"]+)"$`, w.givenSingleAINode)
	sc.Step(`^an in-process M3 node with idle_minutes (\d+), group "([^"]+)", and configured AI "([^"]+)" on "([^"]+)"$`, w.givenIdleAINode)
	sc.Step(`^an in-process M3 node with max_open (\d+), group "([^"]+)", configured AI "([^"]+)" on "([^"]+)", and configured AI "([^"]+)" on "([^"]+)"$`, w.givenMaxOpenTwoAINode)
	sc.Step(`^an in-process M3 node with group "([^"]+)", configured AI "([^"]+)" on "([^"]+)", and acknowledgements held on the link$`, w.givenHeldAckNode)
	sc.Step(`^"([^"]+)" is scripted for two turns that post "([^"]+)" and "([^"]+)"$`, w.scriptClaudeTwoTurns)
	sc.Step(`^"([^"]+)" is scripted for two turns on thread "([^"]+)" that post "([^"]+)" and "([^"]+)"$`, w.scriptCodexTwoTurns)
	sc.Step(`^"([^"]+)" is scripted to post "([^"]+)" and wait for stdin close$`, w.scriptClaudePostAndEOF)
	sc.Step(`^"([^"]+)" is scripted for a resumed turn that posts "([^"]+)"$`, w.scriptClaudeSinglePost)
	sc.Step(`^"([^"]+)" is scripted on thread "([^"]+)" for a first turn that posts "([^"]+)" and a second turn that waits on command approval before posting "([^"]+)"$`, w.scriptCodexApprovalGate)
	sc.Step(`^"([^"]+)" is scripted to exit mid-turn after reporting its session id$`, w.scriptClaudeMidTurnExit)
	sc.Step(`^"([^"]+)" is scripted for two turns that print "([^"]+)" with post "([^"]+)" and then print "([^"]+)" without calling post$`, w.scriptClaudeOutsideText)
	sc.Step(`^"([^"]+)" is scripted to post a test secret and expect tool error "secret"$`, w.scriptClaudeSecretPost)
	sc.Step(`^"([^"]+)" is scripted for one turn that posts "([^"]+)"$`, w.scriptClaudeSinglePost)
}

func registerM3ActionSteps(sc *godog.ScenarioContext, w *m3World) {
	sc.Step(`^MCP clients list tools through the shim for each session kind$`, w.whenListAllSessionTools)
	sc.Step(`^the owner posts "([^"]+)" in "([^"]+)"$`, w.whenOwnerPosts)
	sc.Step(`^the 1-minute idle period elapses$`, w.whenIdleMinuteElapses)
	sc.Step(`^the owner waits for the approval card from "([^"]+)" in "([^"]+)"$`, w.whenWaitApprovalCard)
	sc.Step(`^the owner approves that card in "([^"]+)"$`, w.whenOwnerApprovesCard)
	sc.Step(`^the home sends its due inbox items again on a new connection$`, w.whenHomeResendsInbox)
	sc.Step(`^the host restarts on the node's store and attaches to the home$`, w.whenHostRestarts)
}

func registerM3AssertSteps(sc *godog.ScenarioContext, w *m3World) {
	sc.Step(`^every Codex session lists the main-line tools and schemas of tools\.json$`, w.thenCodexToolsMatch)
	sc.Step(`^structured Claude Code sessions add only approve$`, w.thenClaudeApproveToolsMatch)
	sc.Step(`^"([^"]+)" posts "([^"]+)" in "([^"]+)" and enters state "([^"]+)"$`, w.thenAIPostsAndEntersState)
	sc.Step(`^"([^"]+)" posts "([^"]+)" in "([^"]+)"$`, w.thenAIPosts)
	sc.Step(`^the CLI lifecycle log records 1 "([^"]+)" process for the whole conversation$`, w.thenOneProcessLogged)
	sc.Step(`^the CLI lifecycle log records 1 "([^"]+)" process on session "([^"]+)"$`, w.thenOneSessionLogged)
	sc.Step(`^the first "([^"]+)" process has exited and "([^"]+)" has state "([^"]+)" in "([^"]+)"$`, w.thenFirstExitedAndState)
	sc.Step(`^the CLI lifecycle log records 2 "([^"]+)" starts with the same session id$`, w.thenTwoStartsSameSession)
	sc.Step(`^"([^"]+)" enters state "([^"]+)" before "([^"]+)" posts "([^"]+)" in "([^"]+)"$`, w.thenStateBeforePost)
	sc.Step(`^"([^"]+)" has not started a second process while "([^"]+)" is working$`, w.thenSecondProcessHeld)
	sc.Step(`^"([^"]+)" posts "([^"]+)" before "([^"]+)" posts "([^"]+)" in "([^"]+)"$`, w.thenPostBeforePost)
	sc.Step(`^a "turn_interrupted" system row for "([^"]+)" with reason "([^"]+)" appears in "([^"]+)"$`, w.thenTurnInterruptedRow)
	sc.Step(`^"([^"]+)" has state "([^"]+)" in "([^"]+)"$`, w.thenAIHasState)
	sc.Step(`^"([^"]+)" completes the second turn without posting a second message$`, w.thenSilentTurnCompletes)
	sc.Step(`^no message in "([^"]+)" contains "([^"]+)" or "([^"]+)"$`, w.thenNoMsgContains)
	sc.Step(`^"([^"]+)" finishes the turn after receiving the "secret" tool error$`, w.thenSecretTurnFinishes)
	sc.Step(`^no message from "([^"]+)" exists on the home for "([^"]+)"$`, w.thenNoMsgFromAIOnHome)
	sc.Step(`^"([^"]+)" posts "([^"]+)" in "([^"]+)" and queues 1 acknowledgement$`, w.thenPostAndOneHeldAck)
	sc.Step(`^the host queues a 2nd acknowledgement without starting another CLI process$`, w.thenSecondHeldAckOnly)
	sc.Step(`^the restarted host acknowledges the resent item and "([^"]+)" still ran 1 CLI process and posted 1 message$`, w.thenRestartedAckOnce)
}

func (w *m3World) node() *nodetest.Node {
	return w.cluster.Nodes[0]
}

func (w *m3World) startCluster() error {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sdm3")
	if err != nil {
		return err
	}
	w.root = root
	w.repoSrc = filepath.Join(w.root, "src")
	if err := initLocalGitRepo(w.repoSrc); err != nil {
		return err
	}
	w.cluster = nodetest.Start(w.tb, nodetest.Config{Root: w.root, BinDir: w.f.fakeCLIs, Nodes: 1})
	n := w.node()
	w.activeHost = n.Host
	if err := w.attachMCPSocket(n.Host, filepath.Join(n.Dir, "mcp")); err != nil {
		return err
	}
	w.attachBrowser()
	return nil
}

func (w *m3World) attachMCPSocket(h *host.Host, dir string) error {
	sockPath, stop, err := listenMCPSocket(h, dir)
	if err != nil {
		return err
	}
	registerBothCleanups(w.f.t, w.cleanup, stop)
	w.sockPath, w.closeMCP = sockPath, stop
	return nil
}

// listenMCPSocket serves the host's MCP shim socket in dir and points h.Shim at
// the test binary's shim entry; stop closes the socket and waits for its
// connections.
func listenMCPSocket(h *host.Host, dir string) (string, func(), error) {
	exe, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	local, err := node.ListenLocal(dir)
	if err != nil {
		return "", nil, err
	}
	sockPath := filepath.Join(dir, "sock")
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go serveM3MCP(ctx, local, h, &wg)
	stop := sync.OnceFunc(func() {
		cancel()
		local.Close()
		wg.Wait()
	})
	h.Shim = []string{exe, "-test.run=^TestM3MCPShim$", "--", sockPath}
	return sockPath, stop, nil
}

func serveM3MCP(ctx context.Context, local *node.Local, h *host.Host, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		conn, err := local.Accept()
		if err != nil {
			return
		}
		wg.Add(1)
		go handleM3MCPConn(ctx, conn, h, wg)
	}
}

func handleM3MCPConn(ctx context.Context, conn net.Conn, h *host.Host, wg *sync.WaitGroup) {
	defer wg.Done()
	open, _, r, err := node.AcceptOpening(conn)
	if err != nil {
		return
	}
	if open.MCP == "" {
		conn.Close()
		return
	}
	h.ServeMCP(ctx, open.MCP, r, conn)
}

func (w *m3World) attachBrowser() {
	// Single reader: drainBrowserUntil in m3World.
	w.browser = newHomeFrameClient(256)
	n := w.node()
	w.browserConn = n.Home.Attach(w.cluster.Owner.ID(), w.browser)
	w.cleanup(func() { n.Home.Detach(w.browserConn) })
}

func (w *m3World) writeSetting(key, jsonVal string) error {
	write := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES(?, ?)`, key, jsonVal)
		return err
	}
	return w.node().Store.Write(write)
}

func (w *m3World) createGroup(name string) error {
	gid := w.ids.next()
	w.groups[name] = string(gid)
	payload := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: time.Now().UnixMilli(), Group: gid},
			Op:          "create",
		},
		Name:       name,
		Kind:       "project",
		Repo:       w.repoSrc,
		Base:       "main",
		Forge:      "none",
		MemberName: "Owner",
		Checks:     []string{},
	}
	if err := w.sendBrowserCmd(payload); err != nil {
		return err
	}
	return w.syncBrowserGroup(string(gid))
}

func (w *m3World) sendBrowserCmd(v any) error {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return err
	}
	w.nextReq++
	reqID := w.nextReq
	w.node().Home.Cmd(w.browserConn, reqID, w.cluster.Owner.Sign(string(raw)))
	return w.awaitBrowserReply(reqID)
}

func (w *m3World) syncBrowserGroup(gid string) error {
	w.nextReq++
	reqID := w.nextReq
	w.node().Home.Sync(w.browserConn, proto.Sync{
		Request: proto.Request{Type: "sync", ID: reqID},
		Group:   gid,
	})
	return w.awaitBrowserReply(reqID)
}

func (w *m3World) awaitBrowserReply(reqID int64) error {
	var done bool
	var replyErr error
	check := func(raw json.RawMessage) {
		matched, err := decodeReplyMatch(raw, reqID)
		if matched {
			done, replyErr = true, err
		}
	}
	isDone := func() bool { return done }
	if err := w.drainBrowserUntil(5*time.Second, isDone, check); err != nil {
		return err
	}
	return replyErr
}

func decodeReplyMatch(raw json.RawMessage, reqID int64) (bool, error) {
	var hdr struct {
		Re *int64 `json:"re,omitempty"`
	}
	if proto.Decode(raw, &hdr) != nil || hdr.Re == nil || *hdr.Re != reqID {
		return false, nil
	}
	var wireErr proto.ErrorResponse
	if proto.Decode(raw, &wireErr) == nil {
		return true, fmt.Errorf("%s: %s", wireErr.Err.Code, wireErr.Err.Message)
	}
	return true, nil
}

// drainBrowserUntil is the single reader of w.browser.ch.
func (w *m3World) drainBrowserUntil(timeout time.Duration, stop func() bool, onFrame func(json.RawMessage)) error {
	return drainFramesUntil(w.browser, timeout, stop, w.applyBrowserFrame, onFrame)
}

// drainFramesUntil reads client until stop reports true, passing each frame to
// apply and then to onFrame (when set); it fails after timeout.
func drainFramesUntil(client homeFrameClient, timeout time.Duration, stop func() bool, apply, onFrame func(json.RawMessage)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for !stop() {
		select {
		case raw := <-client.ch:
			apply(raw)
			if onFrame != nil {
				onFrame(raw)
			}
		case <-deadline.C:
			return errors.New("timed out waiting on home pushes")
		}
	}
	return nil
}

func (w *m3World) applyBrowserFrame(raw json.RawMessage) {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Type != "rows" {
		return
	}
	for _, elem := range batch.Rows {
		w.recordRow(elem)
	}
}

func (w *m3World) recordRow(elem json.RawMessage) {
	var hdr proto.RowHeader
	if proto.Decode(elem, &hdr) != nil {
		return
	}
	if hdr.Type == "member" {
		w.recordMemberRow(elem)
	}
	if hdr.Type == "msg" {
		w.recordMsgRow(elem)
	}
}

func (w *m3World) recordMemberRow(elem json.RawMessage) {
	var ai proto.AIMemberRow
	if proto.Decode(elem, &ai) != nil || ai.Kind != "ai" {
		return
	}
	w.aiByName[ai.Name] = ai
	w.events = append(w.events, m3Event{kind: "member", member: ai.Name, state: ai.State})
}

func (w *m3World) recordMsgRow(elem json.RawMessage) {
	var msg proto.MsgRow
	if proto.Decode(elem, &msg) != nil {
		return
	}
	if slices.ContainsFunc(w.msgs, func(cur proto.MsgRow) bool { return cur.ID == msg.ID }) {
		return
	}
	w.msgs = append(w.msgs, msg)
	w.events = append(w.events, m3Event{kind: "msg", member: w.authorName(msg.By), text: msg.Text})
}

func (w *m3World) authorName(id string) string {
	for name, ai := range w.aiByName {
		if ai.ID == id {
			return name
		}
	}
	return id
}

func (w *m3World) configureAI(groupName, aiName, agent string) error {
	ctx := context.Background()
	n := w.node()
	if _, err := n.Host.Agents(ctx); err != nil {
		return err
	}
	by := w.cluster.Owner.ID()
	setRaw, err := marshalNoEscape(proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: n.Key.ID()},
		Agent:       agent,
		Account:     "personal",
	})
	if err != nil {
		return err
	}
	if _, err := host.Surface["agent.set"](ctx, n.Host, n.Link, by, setRaw); err != nil {
		return err
	}
	joinRaw, err := marshalNoEscape(proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: n.Key.ID()},
		Group:       proto.ID(w.groups[groupName]),
		Home:        n.Key.ID(),
		Agent:       agent,
		Name:        aiName,
	})
	if err != nil {
		return err
	}
	if _, err := host.Surface["host.join"](ctx, n.Host, n.Link, by, joinRaw); err != nil {
		return err
	}
	return w.drainBrowserUntil(5*time.Second, func() bool {
		_, ok := w.aiByName[aiName]
		return ok
	}, nil)
}

func (w *m3World) givenBareNode() error {
	return w.startCluster()
}

func (w *m3World) givenSingleAINode(groupName, aiName, agent string) error {
	if err := w.startCluster(); err != nil {
		return err
	}
	if err := w.createGroup(groupName); err != nil {
		return err
	}
	return w.configureAI(groupName, aiName, agent)
}

func (w *m3World) givenIdleAINode(idleMin int, groupName, aiName, agent string) error {
	if err := w.startCluster(); err != nil {
		return err
	}
	if err := w.writeSetting("idle_minutes", fmt.Sprint(idleMin)); err != nil {
		return err
	}
	if err := w.createGroup(groupName); err != nil {
		return err
	}
	return w.configureAI(groupName, aiName, agent)
}

func (w *m3World) givenMaxOpenTwoAINode(maxOpen int, groupName, ai1, agent1, ai2, agent2 string) error {
	if err := w.startCluster(); err != nil {
		return err
	}
	if err := w.writeSetting("max_open", fmt.Sprint(maxOpen)); err != nil {
		return err
	}
	if err := w.createGroup(groupName); err != nil {
		return err
	}
	if err := w.configureAI(groupName, ai1, agent1); err != nil {
		return err
	}
	return w.configureAI(groupName, ai2, agent2)
}

func (w *m3World) givenHeldAckNode(groupName, aiName, agent string) error {
	if err := w.givenSingleAINode(groupName, aiName, agent); err != nil {
		return err
	}
	n := w.node()
	n.Link.Close()
	w.holdLink = wrapM3HoldLink(n.Host, n.Home, true)
	w.cleanup(func() { w.holdLink.Close() })
	return w.awaitHostSynced(n.Host, w.groups[groupName])
}

func (w *m3World) awaitHostSynced(h *host.Host, gid string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := h.Rows.Get(gid, "group", gid); ok {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return errors.New("timed out waiting for host group sync")
}

func (w *m3World) writeScript(vendor string, doc map[string]any) error {
	w.scriptSeq++
	path := filepath.Join(w.root, fmt.Sprintf("%s-%d.json", vendor, w.scriptSeq))
	data, err := marshalNoEscape(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	w.cluster.SetScript(w.tb, vendor, path)
	return nil
}

func (w *m3World) scriptClaudeTwoTurns(_, reply1, reply2 string) error {
	steps := slices.Concat(claudeTurnSteps(1, true, "", reply1, ""), claudeTurnSteps(2, false, "", reply2, ""))
	return w.writeScript("claude", map[string]any{"steps": steps})
}

func (w *m3World) scriptClaudePostAndEOF(_, reply string) error {
	steps := append(claudeTurnSteps(1, true, "", reply, ""), map[string]any{"dir": "eof"})
	return w.writeScript("claude", map[string]any{"steps": steps})
}

func (w *m3World) scriptClaudeSinglePost(_, reply string) error {
	return w.writeScript("claude", map[string]any{"steps": claudeTurnSteps(1, true, "", reply, "")})
}

func (w *m3World) scriptClaudeMidTurnExit(_ string) error {
	steps := []map[string]any{
		{"dir": "stdin", "msg": map[string]any{"type": "user", "message": map[string]any{"role": "user"}}},
		{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
	}
	return w.writeScript("claude", map[string]any{"steps": steps, "exit": 1})
}

func (w *m3World) scriptClaudeOutsideText(_, out1, post1, out2 string) error {
	steps := slices.Concat(claudeTurnSteps(1, true, "", post1, out1), claudeTurnSteps(2, false, "", "", out2))
	return w.writeScript("claude", map[string]any{"steps": steps})
}

func (w *m3World) scriptClaudeSecretPost(_ string) error {
	steps := []map[string]any{claudeUserStdinStep("")}
	steps = append(steps, claudeInitMCP(0)...)
	steps = append(steps,
		map[string]any{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
		map[string]any{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": "post", "arguments": map[string]any{"text": "leak " + m3SecretToken}},
		}},
		map[string]any{"dir": "mcp->cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": m3SecretError}},
				"isError": true,
			},
		}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": "$session"}},
	)
	return w.writeScript("claude", map[string]any{"steps": steps})
}

func (w *m3World) scriptCodexTwoTurns(_, threadID, reply1, reply2 string) error {
	steps := slices.Concat(codexStartupSteps(), codexTurnSteps("turn-1", 1, "", reply1, false), codexTurnSteps("turn-2", 2, "", reply2, false))
	return w.writeScript("codex", map[string]any{"session": threadID, "steps": steps})
}

func (w *m3World) scriptCodexApprovalGate(_, threadID, reply1, reply2 string) error {
	steps := slices.Concat(codexStartupSteps(), codexTurnSteps("turn-1", 1, "", reply1, false), codexTurnSteps("turn-2", 2, "", reply2, true))
	steps = append(steps, map[string]any{"dir": "eof"})
	return w.writeScript("codex", map[string]any{"session": threadID, "steps": steps})
}

func (w *m3World) whenOwnerPosts(text, groupName string) error {
	gid := proto.ID(w.groups[groupName])
	mentions := []proto.ID{}
	for name, ai := range w.aiByName {
		if strings.Contains(text, "@"+name) {
			mentions = append(mentions, proto.ID(ai.ID))
		}
	}
	cmd := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: gid},
		Kind:        "text",
		Text:        text,
		Mentions:    mentions,
		Body:        proto.EmptyBody{},
	}
	return w.sendBrowserCmd(cmd)
}

func (w *m3World) whenIdleMinuteElapses() error {
	return w.drainBrowserUntil(75*time.Second, func() bool {
		ai, ok := w.aiByName["Claude Code"]
		return ok && ai.State == "dormant" && len(w.logLinesMatching("exit claude ")) >= 1
	}, nil)
}

func (w *m3World) whenWaitApprovalCard(aiName, _ string) error {
	aiID := w.aiByName[aiName].ID
	findCard := func() bool {
		for _, m := range w.msgs {
			if m.Kind == "approval" && m.By == aiID {
				w.pendingCard = m.ID
				return true
			}
		}
		return false
	}
	return w.drainBrowserUntil(10*time.Second, findCard, nil)
}

func (w *m3World) whenOwnerApprovesCard(groupName string) error {
	gid := proto.ID(w.groups[groupName])
	cmd := proto.Act{
		HomeCommand: proto.HomeCommand{Type: "act", Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: gid},
		Action:      "approve",
		Msg:         proto.ID(w.pendingCard),
	}
	return w.sendBrowserCmd(cmd)
}

func (w *m3World) whenHomeResendsInbox() error {
	w.holdLink.Close()
	w.holdLink = wrapM3HoldLink(w.activeHost, w.node().Home, true)
	w.cleanup(func() { w.holdLink.Close() })
	return nil
}

func (w *m3World) whenHostRestarts() error {
	n := w.node()
	if err := w.settleAllDormant(); err != nil {
		return err
	}
	w.holdLink.Close()
	w.closeMCP()
	h2, _, err := host.New(n.Store, n.Key, time.Now)
	if err != nil {
		return err
	}
	w.activeHost = h2
	if err := w.attachMCPSocket(h2, filepath.Join(n.Dir, "mcp2")); err != nil {
		return err
	}
	w.restartLink = wrapM3HoldLink(h2, n.Home, false)
	w.cleanup(func() { w.restartLink.Close() })
	return nil
}

func (w *m3World) thenAIPostsAndEntersState(aiName, wantText, _, wantState string) error {
	aiID := w.aiByName[aiName].ID
	ready := func() bool {
		return w.hasMsgFrom(aiID, wantText) && w.aiByName[aiName].State == wantState
	}
	return w.drainBrowserUntil(10*time.Second, ready, nil)
}

func (w *m3World) thenAIPosts(aiName, wantText, _ string) error {
	aiID := w.aiByName[aiName].ID
	return w.drainBrowserUntil(10*time.Second, func() bool {
		return w.hasMsgFrom(aiID, wantText)
	}, nil)
}

func (w *m3World) hasMsgFrom(by, text string) bool {
	return slices.ContainsFunc(w.msgs, func(m proto.MsgRow) bool {
		return m.By == by && m.Text == text
	})
}

func (w *m3World) waitAllDormant(h *host.Host) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !hasNonDormant(h) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func hasNonDormant(h *host.Host) bool {
	return slices.ContainsFunc(h.Sessions.All(), func(s host.Session) bool {
		return s.State != host.Dormant
	})
}

func (w *m3World) settleAllDormant() error {
	w.waitAllDormant(w.activeHost)
	return w.drainBrowserUntil(5*time.Second, w.allMembersDormant, nil)
}

func isSettledMemberState(state string) bool {
	return state == "dormant" || state == "offline"
}

func (w *m3World) allMembersDormant() bool {
	for _, ai := range w.aiByName {
		if !isSettledMemberState(ai.State) {
			return false
		}
	}
	return !hasNonDormant(w.activeHost)
}

func (w *m3World) logLines() []string {
	data, err := os.ReadFile(w.cluster.LogPath)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func (w *m3World) logLinesMatching(prefix string) []string {
	var out []string
	for _, line := range w.logLines() {
		if strings.HasPrefix(line, prefix) {
			out = append(out, line)
		}
	}
	return out
}

func (w *m3World) waitLogCount(prefix string, want int) ([]string, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := w.logLinesMatching(prefix); len(got) >= want {
			return got, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for %d %q lines; got %v", want, prefix, w.logLines())
}

func (w *m3World) thenOneProcessLogged(vendor string) error {
	starts, err := w.waitLogCount("start "+vendor+" ", 1)
	if err != nil {
		return err
	}
	exits, err := w.waitLogCount("exit "+vendor+" ", 1)
	if err != nil {
		return err
	}
	if err := verifySingleProcessPair(vendor, starts, exits); err != nil {
		return err
	}
	return w.settleAllDormant()
}

func verifySingleProcessPair(vendor string, starts, exits []string) error {
	if len(starts) != 1 || len(exits) != 1 {
		return fmt.Errorf("expected 1 start and 1 exit, got %v / %v", starts, exits)
	}
	id := strings.TrimPrefix(starts[0], "start "+vendor+" ")
	if id == "" || exits[0] != "exit "+vendor+" "+id {
		return fmt.Errorf("mismatched start/exit pair: %q vs %q", starts[0], exits[0])
	}
	return nil
}

func (w *m3World) thenOneSessionLogged(vendor, sessionID string) error {
	if err := w.thenOneProcessLogged(vendor); err != nil {
		return err
	}
	want := "start " + vendor + " " + sessionID
	if got := w.logLinesMatching("start " + vendor + " "); got[0] != want {
		return fmt.Errorf("got %q, want %q", got[0], want)
	}
	return nil
}

func (w *m3World) thenFirstExitedAndState(vendor, aiName, wantState, _ string) error {
	if len(w.logLinesMatching("exit "+vendor+" ")) != 1 {
		return fmt.Errorf("expected 1 %s exit, got %v", vendor, w.logLines())
	}
	if got := w.aiByName[aiName].State; got != wantState {
		return fmt.Errorf("%s state = %q, want %q", aiName, got, wantState)
	}
	return nil
}

func (w *m3World) thenTwoStartsSameSession(vendor string) error {
	starts, err := w.waitLogCount("start "+vendor+" ", 2)
	if err != nil {
		return err
	}
	if _, err := w.waitLogCount("exit "+vendor+" ", 2); err != nil {
		return err
	}
	if len(starts) != 2 || starts[0] != starts[1] {
		return fmt.Errorf("starts = %v, want 2 identical session starts", starts)
	}
	return w.settleAllDormant()
}

func (w *m3World) eventIndex(match func(m3Event) bool) int {
	return slices.IndexFunc(w.events, match)
}

func checkOrderedIndices(first, second int, detail string) error {
	if first < 0 || second < 0 || first >= second {
		return fmt.Errorf("order check failed (%d, %d): %s", first, second, detail)
	}
	return nil
}

func (w *m3World) thenStateBeforePost(ai1, wantState, ai2, wantText, groupName string) error {
	if err := w.thenAIPosts(ai2, wantText, groupName); err != nil {
		return err
	}
	dormantIdx := w.eventIndex(func(e m3Event) bool { return e.kind == "member" && e.member == ai1 && e.state == wantState })
	postIdx := w.eventIndex(func(e m3Event) bool { return e.kind == "msg" && e.member == ai2 && e.text == wantText })
	if err := checkOrderedIndices(dormantIdx, postIdx, fmt.Sprintf("%+v", w.events)); err != nil {
		return err
	}
	lines := w.logLines()
	exitIdx := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "exit claude ") })
	startIdx := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "start codex ") })
	return checkOrderedIndices(exitIdx, startIdx, strings.Join(lines, "; "))
}

func (w *m3World) thenSecondProcessHeld(aiName, workingAI string) error {
	agent := w.aiByName[aiName].CLI
	if err := w.awaitDeliveredCount(agent, 2); err != nil {
		return err
	}
	if starts := w.logLinesMatching("start " + agent + " "); len(starts) != 1 {
		return fmt.Errorf("%s started early while %s was working: %v", aiName, workingAI, starts)
	}
	return nil
}

func (w *m3World) awaitDeliveredCount(agent string, want int) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := w.deliveredCount(agent)
		if err != nil {
			return err
		}
		if n >= want {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %d delivered items for %s", want, agent)
}

func (w *m3World) deliveredCount(agent string) (int, error) {
	var count int
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM audit WHERE agent = ? AND what = 'delivered'`, agent).Scan(&count)
	}
	err := w.node().Store.Write(read)
	return count, err
}

func (w *m3World) thenPostBeforePost(ai1, text1, ai2, text2, groupName string) error {
	if err := w.thenAIPosts(ai1, text1, groupName); err != nil {
		return err
	}
	if err := w.thenAIPosts(ai2, text2, groupName); err != nil {
		return err
	}
	idx1 := w.eventIndex(func(e m3Event) bool { return e.kind == "msg" && e.member == ai1 && e.text == text1 })
	idx2 := w.eventIndex(func(e m3Event) bool { return e.kind == "msg" && e.member == ai2 && e.text == text2 })
	if err := checkOrderedIndices(idx1, idx2, fmt.Sprintf("%+v", w.events)); err != nil {
		return err
	}
	return w.settleAllDormant()
}

func (w *m3World) thenTurnInterruptedRow(aiName, wantReason, _ string) error {
	aiID := w.aiByName[aiName].ID
	found := func() bool {
		return slices.ContainsFunc(w.msgs, func(m proto.MsgRow) bool {
			return isInterruptedMsg(m, aiID, wantReason)
		})
	}
	return w.drainBrowserUntil(10*time.Second, found, nil)
}

func isInterruptedMsg(m proto.MsgRow, aiID, wantReason string) bool {
	if m.Kind != "system" {
		return false
	}
	var body proto.TurnInterruptedBody
	if proto.Decode(m.Body, &body) != nil {
		return false
	}
	return body.Code == "turn_interrupted" && body.AI == aiID && body.Reason == wantReason
}

func (w *m3World) thenAIHasState(aiName, wantState, _ string) error {
	return w.drainBrowserUntil(10*time.Second, func() bool {
		return w.aiByName[aiName].State == wantState
	}, nil)
}

func (w *m3World) countMessagesBy(authorID string) int {
	count := 0
	for _, m := range w.msgs {
		if m.By == authorID {
			count++
		}
	}
	return count
}

func (w *m3World) thenSilentTurnCompletes(aiName string) error {
	agent := w.aiByName[aiName].CLI
	if _, err := w.waitLogCount("exit "+agent+" ", 1); err != nil {
		return err
	}
	if err := w.settleAllDormant(); err != nil {
		return err
	}
	if got := w.countMessagesBy(w.aiByName[aiName].ID); got != 1 {
		return fmt.Errorf("got %d messages from %s, want 1", got, aiName)
	}
	return nil
}

func containsEither(text, sub1, sub2 string) bool {
	return strings.Contains(text, sub1) || strings.Contains(text, sub2)
}

func (w *m3World) thenNoMsgContains(groupName, sub1, sub2 string) error {
	for _, m := range w.msgs {
		if containsEither(m.Text, sub1, sub2) {
			return fmt.Errorf("unexpected outside text in group %s message: %q", groupName, m.Text)
		}
	}
	stored, err := w.homeMsgRows(w.groups[groupName])
	if err != nil {
		return err
	}
	for _, raw := range stored {
		if containsEither(raw, sub1, sub2) {
			return fmt.Errorf("unexpected outside text stored on home: %s", raw)
		}
	}
	return nil
}

func (w *m3World) homeMsgRows(gid string) ([]string, error) {
	var out []string
	read := func(tx *store.Tx) error {
		rows, err := tx.Query(`SELECT data FROM row WHERE g = ? AND r = 'msg' ORDER BY seq`, gid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			out = append(out, raw)
		}
		return rows.Err()
	}
	return out, w.node().Store.Write(read)
}

func (w *m3World) thenSecretTurnFinishes(aiName string) error {
	agent := w.aiByName[aiName].CLI
	if _, err := w.waitLogCount("exit "+agent+" ", 1); err != nil {
		return err
	}
	if err := w.settleAllDormant(); err != nil {
		return err
	}
	for _, m := range w.msgs {
		if m.Kind == "system" {
			return fmt.Errorf("unexpected system row on secret turn: %s", m.Body)
		}
	}
	return nil
}

func (w *m3World) thenNoMsgFromAIOnHome(aiName, groupName string) error {
	aiID := w.aiByName[aiName].ID
	stored, err := w.homeMsgRows(w.groups[groupName])
	if err != nil {
		return err
	}
	for _, raw := range stored {
		var msg proto.MsgRow
		if proto.Decode([]byte(raw), &msg) == nil && msg.By == aiID {
			return fmt.Errorf("unexpected row on home from %s: %s", aiName, raw)
		}
	}
	return nil
}

func (w *m3World) thenPostAndOneHeldAck(aiName, wantText, groupName string) error {
	if err := w.thenAIPosts(aiName, wantText, groupName); err != nil {
		return err
	}
	return w.holdLink.awaitAcks(1)
}

func (w *m3World) thenSecondHeldAckOnly() error {
	if err := w.holdLink.awaitAcks(1); err != nil {
		return err
	}
	if starts := w.logLinesMatching("start claude "); len(starts) != 1 {
		return fmt.Errorf("expected 1 claude start, got %v", starts)
	}
	return nil
}

func (w *m3World) thenRestartedAckOnce(aiName string) error {
	if err := w.restartLink.awaitAcks(1); err != nil {
		return err
	}
	if err := w.awaitHomeInboxEmpty(); err != nil {
		return err
	}
	if got := w.countMessagesBy(w.aiByName[aiName].ID); got != 1 {
		return fmt.Errorf("got %d AI messages, want 1", got)
	}
	return w.thenOneProcessLogged("claude")
}

func (w *m3World) awaitHomeInboxEmpty() error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := w.homeInboxCount()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	n, _ := w.homeInboxCount()
	return fmt.Errorf("home still has %d queued inbox items", n)
}

func (w *m3World) homeInboxCount() (int, error) {
	var count int
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM inbox`).Scan(&count)
	}
	err := w.node().Store.Write(read)
	return count, err
}

func (w *m3World) whenListAllSessionTools() error {
	lists, err := loadM3GoldenTools()
	if err != nil {
		return err
	}
	specs := []struct {
		name      string
		kind      host.Kind
		secretary bool
		task      string
		want      proto.ToolList
	}{
		{"secretary", host.GroupSession, true, "", lists.Secretary},
		{"group", host.GroupSession, false, "", lists.Group},
		{"review", host.ReviewSession, false, "t-rev", lists.Review},
		{"task_level_3", host.TaskSession, false, "t-3", lists.TaskLevel23},
	}
	for _, sp := range specs {
		chk, err := w.collectToolCheck(sp.name, sp.kind, sp.secretary, sp.task, sp.want)
		if err != nil {
			return err
		}
		w.toolChecks = append(w.toolChecks, chk)
	}
	return nil
}

func (w *m3World) collectToolCheck(label string, kind host.Kind, sec bool, task string, want proto.ToolList) (m3ToolCheck, error) {
	codexTools, err := w.listSessionTools("g-tools", "m-codex-"+label, task, "codex", kind, sec)
	if err != nil {
		return m3ToolCheck{}, err
	}
	claudeStruct, err := w.listSessionTools("g-tools", "m-claude-"+label, task, "claude", kind, sec)
	if err != nil {
		return m3ToolCheck{}, err
	}
	return m3ToolCheck{
		name:             label,
		want:             slices.SortedFunc(slices.Values(want.Tools), func(a, b proto.Tool) int { return cmp.Compare(a.Name, b.Name) }),
		codex:            codexTools,
		claudeStructured: claudeStruct,
	}, nil
}

func (w *m3World) listSessionTools(group, member, task, agent string, kind host.Kind, sec bool) ([]proto.Tool, error) {
	h := w.node().Host
	key := host.SessionKey{Group: group, Member: member, Task: task}
	err := h.Sessions.Create(host.Session{
		SessionKey: key,
		Kind:       kind,
		Folder:     "repo",
		Agent:      agent,
		Home:       w.node().Key.ID(),
		Name:       member,
		Secretary:  sec,
	})
	if err != nil {
		return nil, err
	}
	token := h.Sessions.Admit(key, 3, false)
	defer h.Sessions.Forget(token)
	raw, err := listToolsViaShim(w.sockPath, token)
	if err != nil {
		return nil, err
	}
	return decodeListedTools(raw, agent == "claude")
}

func listToolsViaShim(sockPath, token string) (json.RawMessage, error) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	// Single reader: wait at the end of listToolsViaShim.
	done := make(chan int, 1)
	dial := func() (net.Conn, error) { return net.Dial("unix", sockPath) }
	go func() {
		var stderr bytes.Buffer
		code := host.RunShim([]string{"--token", token}, dial, inR, outW, &stderr)
		outW.Close()
		done <- code
	}()
	raw, err := exchangeToolsList(inW, outR)
	inW.Close()
	outR.Close()
	<-done
	return raw, err
}

func exchangeToolsList(inW io.Writer, outR io.Reader) (json.RawMessage, error) {
	dec := json.NewDecoder(outR)
	if err := initMCPSession(inW, dec); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintln(inW, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); err != nil {
		return nil, err
	}
	return readM3RPC(dec)
}

// initMCPSession performs the initialize handshake on a shim stream.
func initMCPSession(w io.Writer, dec *json.Decoder) error {
	if _, err := fmt.Fprintln(w, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"m3","version":"1"}}}`); err != nil {
		return err
	}
	if _, err := readM3RPC(dec); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return err
}

func readM3RPC(dec *json.Decoder) (json.RawMessage, error) {
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := dec.Decode(&resp); err != nil || resp.Error != nil {
		return nil, fmt.Errorf("rpc decode: %v (%v)", err, resp.Error)
	}
	return resp.Result, nil
}

func decodeListedTools(raw json.RawMessage, allowApprove bool) ([]proto.Tool, error) {
	var wire struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	out := make([]proto.Tool, 0, len(wire.Tools))
	for _, entry := range wire.Tools {
		tool, err := decodeOneListedTool(entry.Name, entry.Description, entry.InputSchema, allowApprove)
		if err != nil {
			return nil, err
		}
		out = append(out, tool)
	}
	slices.SortFunc(out, func(a, b proto.Tool) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

func decodeOneListedTool(name, desc string, schema json.RawMessage, allowApprove bool) (proto.Tool, error) {
	if desc == "" {
		return proto.Tool{}, fmt.Errorf("empty description on tool %s", name)
	}
	if name == "approve" && allowApprove {
		return proto.Tool{Name: "approve"}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(schema))
	dec.DisallowUnknownFields()
	var in proto.ToolInput
	if err := dec.Decode(&in); err != nil {
		return proto.Tool{}, fmt.Errorf("strict schema decode %s: %w (%s)", name, err, schema)
	}
	return proto.Tool{Name: name, InputSchema: in}, nil
}

func loadM3GoldenTools() (proto.ToolLists, error) {
	data, err := os.ReadFile(filepath.Join("..", "internal", "proto", "testdata", "tools.json"))
	if err != nil {
		return proto.ToolLists{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var lists proto.ToolLists
	return lists, dec.Decode(&lists)
}

func (w *m3World) thenCodexToolsMatch() error {
	for _, chk := range w.toolChecks {
		if !reflect.DeepEqual(chk.codex, chk.want) {
			return fmt.Errorf("%s codex tools:\ngot  %+v\nwant %+v", chk.name, chk.codex, chk.want)
		}
	}
	return nil
}

func (w *m3World) thenClaudeApproveToolsMatch() error {
	for _, chk := range w.toolChecks {
		want := append(slices.Clone(chk.want), proto.Tool{Name: "approve"})
		slices.SortFunc(want, func(a, b proto.Tool) int { return cmp.Compare(a.Name, b.Name) })
		if !reflect.DeepEqual(chk.claudeStructured, want) {
			return fmt.Errorf("%s structured claude tools:\ngot  %+v\nwant %+v", chk.name, chk.claudeStructured, want)
		}
	}
	return nil
}

// m3HoldLink wraps node.LocalLink while optionally withholding ack frames from
// the home so A3.10 can observe the home resending unacknowledged inbox items.
type m3HoldLink struct {
	*node.LocalLink
	holdAck bool
	// Single reader: awaitAcks in m3HoldLink.
	ackCh chan proto.Ack
	acks  []proto.Ack
}

func wrapM3HoldLink(h *host.Host, loop *home.Loop, holdAck bool) *m3HoldLink {
	l := &m3HoldLink{
		LocalLink: node.AttachLocal(h, loop),
		holdAck:   holdAck,
		// Single reader: awaitAcks in m3HoldLink.
		ackCh: make(chan proto.Ack, 32),
	}
	h.Attach(h.Key.ID(), l)
	return l
}

func (l *m3HoldLink) Send(frame any) error {
	if ack, ok := frame.(proto.Ack); ok {
		l.ackCh <- ack
		if l.holdAck {
			return nil
		}
	}
	return l.LocalLink.Send(frame)
}

func (l *m3HoldLink) awaitAcks(want int) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for len(l.acks) < want {
		select {
		case ack := <-l.ackCh:
			l.acks = append(l.acks, ack)
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for %d acks, got %d", want, len(l.acks))
		}
	}
	return nil
}
