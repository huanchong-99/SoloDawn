//go:build acceptance

package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	stepSets["s3_kb"] = s3KBSteps
}

type s3KBPutOut struct {
	ID       string `json:"id"`
	Replaced bool   `json:"replaced"`
}

type s3KBDoc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
	By   string `json:"by"`
	At   int64  `json:"at"`
}

type s3KBSearchOut struct {
	Documents []s3KBDoc `json:"documents"`
	More      bool      `json:"more"`
}

type s3KBWorld struct {
	f          fixtures
	cleanup    func(func())
	tb         scenarioTB
	clock      atomic.Int64
	today      time.Time
	root       string
	repoSrc    string
	cluster    *nodetest.Cluster
	browser2   identity.Key
	b1         homeFrameClient
	b1Conn     *home.Conn
	b2         homeFrameClient
	b2Conn     *home.Conn
	nextReq    int64
	ids        seqIDGen
	scriptSeq  int
	groups     map[string]string
	aiByName   map[string]proto.AIMemberRow
	putCreated s3KBPutOut
	alphaHits  s3KBSearchOut
	alphaMiss  s3KBSearchOut
	betaHits   s3KBSearchOut
	restartOut s3KBSearchOut
	oldMsgID   string
	secretErrs []string
}

func s3KBSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &s3KBWorld{
		f:        f,
		today:    time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC),
		groups:   map[string]string{},
		aiByName: map[string]proto.AIMemberRow{},
	}
	w.clock.Store(w.today.UnixMilli())
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.cluster != nil {
			w.waitDormant()
		}
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.tb = scenarioTB{TB: f.t, cleanup: w.cleanup}

	sc.Step(`^an S3 knowledge-base node with groups "([^"]+)" and "([^"]+)" and configured AIs "([^"]+)" and "([^"]+)"$`, w.givenTwoGroups)
	sc.Step(`^"([^"]+)" stores the knowledge-base document "([^"]+)" in "([^"]+)" and searches for "([^"]+)" and "([^"]+)"$`, w.whenAIStoreAndSearch)
	sc.Step(`^kb_put created the document in "([^"]+)", kb_search for "([^"]+)" returned it, and kb_search for "([^"]+)" returned an empty list$`, w.thenCreatedAndSearched)
	sc.Step(`^"([^"]+)" searches the knowledge base in "([^"]+)" for "([^"]+)"$`, w.whenOtherGroupSearch)
	sc.Step(`^kb_search in "([^"]+)" returned an empty list without "([^"]+)"'s document$`, w.thenOtherGroupEmpty)
	sc.Step(`^the S3 home restarts and "([^"]+)" searches "([^"]+)" for "([^"]+)" again$`, w.whenRestartAndSearch)
	sc.Step(`^kb_search after the restart answers with the same document$`, w.thenRestartMatches)

	sc.Step(`^an S3 knowledge-base node with retention set to (\d+) days, group "([^"]+)", two browsers and configured AI "([^"]+)"$`, w.givenRetentionNode)
	sc.Step(`^(\d+) days ago "([^"]+)" received an old text message and "([^"]+)" stored the document "([^"]+)"$`, w.givenOldMsgAndDoc)
	sc.Step(`^daily retention runs on the S3 node today$`, w.whenRetentionRunsToday)
	sc.Step(`^the old text message in "([^"]+)" has expired while "([^"]+)" remains searchable by "([^"]+)"$`, w.thenOldMsgGoneDocRemains)
	sc.Step(`^the first browser forgets "([^"]+)" in "([^"]+)"$`, w.whenFirstBrowserForgets)
	sc.Step(`^the second browser receives a gone marker for "([^"]+)" within 1 second and kb_search for "([^"]+)" returns an empty list$`, w.thenSecondGetsGoneAndSearchEmpty)
	sc.Step(`^"([^"]+)" calls kb_put with a test secret in text and in name$`, w.whenAICallsSecretKBPut)
	sc.Step(`^both kb_put calls fail with "secret" and the home has no row for either secret document$`, w.thenSecretKBPutRefusedWithNoRow)
}

func (w *s3KBWorld) now() time.Time { return time.UnixMilli(w.clock.Load()) }

func (w *s3KBWorld) bootNode(withSecondBrowser bool) error {
	root, err := shortTempRoot(w.f.t, w.cleanup, "sds3kb")
	if err != nil {
		return err
	}
	w.root = root
	w.repoSrc = filepath.Join(w.root, "src")
	if err := initLocalGitRepo(w.repoSrc); err != nil {
		return err
	}
	var extras []identity.Key
	if withSecondBrowser {
		w.browser2 = exampleKey("s3kb second browser")
		extras = []identity.Key{w.browser2}
	}
	w.cluster = nodetest.Start(w.tb, nodetest.Config{
		Root:          w.root,
		BinDir:        w.f.fakeCLIs,
		Nodes:         1,
		ExtraBrowsers: extras,
		Now:           w.now,
	})
	n := w.cluster.Nodes[0]
	if err := w.attachMCP(n.Host, filepath.Join(n.Dir, "mcp")); err != nil {
		return err
	}
	w.attachBrowsers(withSecondBrowser)
	return w.declareClaudeAccount()
}

func (w *s3KBWorld) attachMCP(h *host.Host, dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	local, err := node.ListenLocal(dir)
	if err != nil {
		return err
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
	registerBothCleanups(w.f.t, w.cleanup, stop)
	h.Shim = []string{exe, "-test.run=^TestM3MCPShim$", "--", sockPath}
	return nil
}

func (w *s3KBWorld) attachBrowsers(withSecond bool) {
	n := w.cluster.Nodes[0]
	w.b1 = newHomeFrameClient(256)
	w.b1Conn = n.Home.Attach(w.cluster.Owner.ID(), w.b1)
	w.cleanup(func() { n.Home.Detach(w.b1Conn) })
	if withSecond {
		w.b2 = newHomeFrameClient(256)
		w.b2Conn = n.Home.Attach(w.browser2.ID(), w.b2)
		w.cleanup(func() { n.Home.Detach(w.b2Conn) })
	}
}

func (w *s3KBWorld) declareClaudeAccount() error {
	ctx := context.Background()
	n := w.cluster.Nodes[0]
	if _, err := n.Host.Agents(ctx); err != nil {
		return err
	}
	setRaw, err := marshalNoEscape(proto.AgentSet{
		HostCommand: proto.HostCommand{Type: "agent.set", Nonce: w.ids.next(), Timestamp: w.clock.Load(), To: n.Key.ID()},
		Agent:       "claude",
		Account:     "personal",
	})
	if err != nil {
		return err
	}
	_, err = host.Surface["agent.set"](ctx, n.Host, n.Link, w.cluster.Owner.ID(), setRaw)
	return err
}

func (w *s3KBWorld) createGroupAndAI(groupName, aiName string, syncSecond bool) error {
	gid := w.ids.next()
	w.groups[groupName] = string(gid)
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{Type: "group", Nonce: gid, Timestamp: w.clock.Load(), Group: gid},
			Op:          "create",
		},
		Name: groupName, Kind: "project", Repo: w.repoSrc, Base: "main", Forge: "none",
		MemberName: "Owner", Checks: []string{},
	}
	if err := w.sendCmd(create); err != nil {
		return err
	}
	if err := w.syncPeer(w.b1Conn, w.b1, string(gid)); err != nil {
		return err
	}
	if syncSecond {
		if err := w.syncPeer(w.b2Conn, w.b2, string(gid)); err != nil {
			return err
		}
	}
	return w.joinAI(groupName, aiName)
}

func (w *s3KBWorld) joinAI(groupName, aiName string) error {
	n := w.cluster.Nodes[0]
	joinRaw, err := marshalNoEscape(proto.HostJoin{
		HostCommand: proto.HostCommand{Type: "host.join", Nonce: w.ids.next(), Timestamp: w.clock.Load(), To: n.Key.ID()},
		Group:       proto.ID(w.groups[groupName]),
		Home:        n.Key.ID(),
		Agent:       "claude",
		Name:        aiName,
	})
	if err != nil {
		return err
	}
	if _, err := host.Surface["host.join"](context.Background(), n.Host, n.Link, w.cluster.Owner.ID(), joinRaw); err != nil {
		return err
	}
	return w.drainB1Until(5*time.Second, func() bool {
		_, ok := w.aiByName[aiName]
		return ok
	}, nil)
}

func (w *s3KBWorld) sendCmd(v any) error {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return err
	}
	w.nextReq++
	reqID := w.nextReq
	w.cluster.Nodes[0].Home.Cmd(w.b1Conn, reqID, w.cluster.Owner.Sign(string(raw)))
	return w.awaitReply(w.b1, reqID, w.recordB1Frame)
}

func (w *s3KBWorld) syncPeer(conn *home.Conn, client homeFrameClient, gid string) error {
	w.nextReq++
	reqID := w.nextReq
	w.cluster.Nodes[0].Home.Sync(conn, proto.Sync{
		Request: proto.Request{Type: "sync", ID: reqID},
		Group:   gid,
	})
	var onFrame func(json.RawMessage)
	if client.ch == w.b1.ch {
		onFrame = w.recordB1Frame
	}
	return w.awaitReply(client, reqID, onFrame)
}

func (w *s3KBWorld) awaitReply(client homeFrameClient, reqID int64, onFrame func(json.RawMessage)) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case raw := <-client.ch:
			if onFrame != nil {
				onFrame(raw)
			}
			matched, err := decodeReplyMatch(raw, reqID)
			if matched {
				return err
			}
		case <-deadline.C:
			return errors.New("timed out waiting for home reply")
		}
	}
}

func (w *s3KBWorld) drainB1Until(timeout time.Duration, stop func() bool, onFrame func(json.RawMessage)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for !stop() {
		select {
		case raw := <-w.b1.ch:
			w.recordB1Frame(raw)
			if onFrame != nil {
				onFrame(raw)
			}
		case <-deadline.C:
			return errors.New("timed out waiting on home pushes")
		}
	}
	return nil
}

func (w *s3KBWorld) recordB1Frame(raw json.RawMessage) {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Type != "rows" {
		return
	}
	for _, elem := range batch.Rows {
		var ai proto.AIMemberRow
		if proto.Decode(elem, &ai) == nil && ai.Type == "member" && ai.Kind == "ai" {
			w.aiByName[ai.Name] = ai
		}
	}
}

func (w *s3KBWorld) waitDormant() {
	h := w.cluster.Nodes[0].Host
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !hasNonDormant(h) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *s3KBWorld) callToolAsAI(aiName, groupName, tool string, args any, dst any) error {
	n := w.cluster.Nodes[0]
	ai := w.aiByName[aiName]
	key := host.SessionKey{Group: w.groups[groupName], Member: ai.ID}
	token := n.Host.Sessions.Admit(key, 3, false)
	defer n.Host.Sessions.Forget(token)
	sockPath := n.Host.Shim[len(n.Host.Shim)-1]
	raw, isErr, err := callMCPToolViaShim(sockPath, token, tool, args)
	if err != nil {
		return err
	}
	if isErr {
		return errors.New(string(raw))
	}
	if dst != nil {
		return json.Unmarshal(raw, dst)
	}
	return nil
}

func callMCPToolViaShim(sockPath, token, tool string, args any) ([]byte, bool, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	openLine, _ := marshalNoEscape(map[string]string{"mcp": token})
	if _, err := fmt.Fprintln(conn, string(openLine)); err != nil {
		return nil, false, err
	}
	dec := json.NewDecoder(conn)
	if err := initShimSession(conn, dec); err != nil {
		return nil, false, err
	}
	callReq, _ := marshalNoEscape(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if _, err := fmt.Fprintln(conn, string(callReq)); err != nil {
		return nil, false, err
	}
	resRaw, err := readM3RPC(dec)
	if err != nil {
		return nil, false, err
	}
	var toolRes struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resRaw, &toolRes); err != nil || len(toolRes.Content) == 0 {
		return nil, false, fmt.Errorf("bad tool result: %s (%v)", resRaw, err)
	}
	return []byte(toolRes.Content[0].Text), toolRes.IsError, nil
}

func initShimSession(conn net.Conn, dec *json.Decoder) error {
	if _, err := fmt.Fprintln(conn, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"s3","version":"1"}}}`); err != nil {
		return err
	}
	if _, err := readM3RPC(dec); err != nil {
		return err
	}
	_, err := fmt.Fprintln(conn, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return err
}

func (w *s3KBWorld) givenTwoGroups(g1, g2, ai1, ai2 string) error {
	if err := w.bootNode(false); err != nil {
		return err
	}
	if err := w.createGroupAndAI(g1, ai1, false); err != nil {
		return err
	}
	return w.createGroupAndAI(g2, ai2, false)
}

func (w *s3KBWorld) whenAIStoreAndSearch(aiName, docName, groupName, hitWord, missWord string) error {
	// Run kb_put through a real scripted fake Claude CLI turn, then verify search.
	w.scriptSeq++
	scriptPath := filepath.Join(w.root, fmt.Sprintf("claude-kb-%d.json", w.scriptSeq))
	steps := []map[string]any{claudeUserStdinStep("")}
	steps = append(steps, claudeInitMCP(0)...)
	steps = append(steps,
		map[string]any{"dir": "stdout", "msg": map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}},
		map[string]any{"dir": "mcp<-cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{
				"name":      "kb_put",
				"arguments": map[string]any{"name": docName, "text": "Every group write goes through its single writer home."},
			},
		}},
		map[string]any{"dir": "mcp->cli", "msg": map[string]any{
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{"isError": false},
		}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": "$session"}},
		map[string]any{"dir": "eof"},
	)
	rawScript, _ := marshalNoEscape(map[string]any{"steps": steps})
	if err := os.WriteFile(scriptPath, rawScript, 0o600); err != nil {
		return err
	}
	w.cluster.SetScript(w.tb, "claude", scriptPath)

	ai := w.aiByName[aiName]
	gid := proto.ID(w.groups[groupName])
	mention := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: w.ids.next(), Timestamp: w.clock.Load(), Group: gid},
		Kind:        "text",
		Text:        "@" + aiName + " record decision",
		Mentions:    []proto.ID{proto.ID(ai.ID)},
		Body:        proto.EmptyBody{},
	}
	if err := w.sendCmd(mention); err != nil {
		return err
	}
	if err := w.waitFirstTurnCompleted(aiName); err != nil {
		return err
	}
	if err := w.callToolAsAI(aiName, groupName, "kb_search", map[string]any{"query": hitWord}, &w.alphaHits); err != nil {
		return err
	}
	if len(w.alphaHits.Documents) == 1 {
		w.putCreated = s3KBPutOut{ID: w.alphaHits.Documents[0].ID, Replaced: false}
	}
	return w.callToolAsAI(aiName, groupName, "kb_search", map[string]any{"query": missWord}, &w.alphaMiss)
}

func (w *s3KBWorld) waitFirstTurnCompleted(aiName string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(w.cluster.LogPath)
		if strings.Contains(string(data), "start claude ") && !hasWorkingSession(w.cluster.Nodes[0].Host, w.aiByName[aiName].ID) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("timed out waiting for claude kb_put turn to finish")
}

func hasWorkingSession(h *host.Host, memberID string) bool {
	return slices.ContainsFunc(h.Sessions.All(), func(s host.Session) bool {
		return s.Member == memberID && s.State == host.Working
	})
}

func (w *s3KBWorld) thenCreatedAndSearched(_, _, _ string) error {
	if len(w.alphaHits.Documents) != 1 || w.alphaHits.More {
		return fmt.Errorf("got hits %+v, want 1 document with more=false", w.alphaHits)
	}
	doc := w.alphaHits.Documents[0]
	if doc.Name != "Decision: single writer" || !strings.Contains(doc.Text, "single writer") || doc.By != "Claude Alpha" {
		return fmt.Errorf("unexpected document: %+v", doc)
	}
	if len(w.alphaMiss.Documents) != 0 || w.alphaMiss.More {
		return fmt.Errorf("expected empty miss list, got %+v", w.alphaMiss)
	}
	return nil
}

func (w *s3KBWorld) whenOtherGroupSearch(aiName, groupName, word string) error {
	return w.callToolAsAI(aiName, groupName, "kb_search", map[string]any{"query": word}, &w.betaHits)
}

func (w *s3KBWorld) thenOtherGroupEmpty(_, _ string) error {
	if len(w.betaHits.Documents) != 0 || w.betaHits.More {
		return fmt.Errorf("expected empty search in other group, got %+v", w.betaHits)
	}
	return nil
}

func (w *s3KBWorld) whenRestartAndSearch(aiName, groupName, word string) error {
	n := w.cluster.Nodes[0]
	n.Home.Detach(w.b1Conn)
	n.RestartHome()
	w.b1 = newHomeFrameClient(256)
	w.b1Conn = n.Home.Attach(w.cluster.Owner.ID(), w.b1)
	w.cleanup(func() { n.Home.Detach(w.b1Conn) })
	return w.callToolAsAI(aiName, groupName, "kb_search", map[string]any{"query": word}, &w.restartOut)
}

func (w *s3KBWorld) thenRestartMatches() error {
	if len(w.restartOut.Documents) != 1 || w.restartOut.Documents[0] != w.alphaHits.Documents[0] {
		return fmt.Errorf("after restart got %+v, want %+v", w.restartOut, w.alphaHits)
	}
	return nil
}

func (w *s3KBWorld) givenRetentionNode(days int, groupName, aiName string) error {
	w.clock.Store(w.today.Add(-10 * 24 * time.Hour).UnixMilli())
	if err := w.bootNode(true); err != nil {
		return err
	}
	err := w.cluster.Nodes[0].Store.Write(func(tx *store.Tx) error {
		_, err := tx.Exec("INSERT OR REPLACE INTO setting(key, value) VALUES('retention_days', ?)", strconv.Itoa(days))
		return err
	})
	if err != nil {
		return err
	}
	return w.createGroupAndAI(groupName, aiName, true)
}

func (w *s3KBWorld) givenOldMsgAndDoc(daysAgo int, groupName, aiName, docName string) error {
	w.clock.Store(w.today.Add(-time.Duration(daysAgo) * 24 * time.Hour).UnixMilli())
	msgID := w.ids.next()
	w.oldMsgID = string(msgID)
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{Type: "post", Nonce: msgID, Timestamp: w.clock.Load(), Group: proto.ID(w.groups[groupName])},
		Kind:        "text",
		Text:        "ephemeral old chat",
		Mentions:    []proto.ID{},
		Body:        proto.EmptyBody{},
	}
	if err := w.sendCmd(post); err != nil {
		return err
	}
	return w.callToolAsAI(aiName, groupName, "kb_put", map[string]any{
		"name": docName,
		"text": "连接路径事件 survives daily retention.",
	}, &w.putCreated)
}

func (w *s3KBWorld) whenRetentionRunsToday() error {
	w.clock.Store(w.today.UnixMilli())
	return w.cluster.Nodes[0].Store.Write(func(tx *store.Tx) error {
		return home.Retain(tx, w.today.UnixMilli())
	})
}

func (w *s3KBWorld) thenOldMsgGoneDocRemains(groupName, docName, query string) error {
	var msgCount int
	err := w.cluster.Nodes[0].Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND r='msg' AND id=?", w.groups[groupName], w.oldMsgID).Scan(&msgCount)
	})
	if err != nil {
		return err
	}
	if msgCount != 0 {
		return fmt.Errorf("old text message still present after retention (count=%d)", msgCount)
	}
	var out s3KBSearchOut
	if err := w.callToolAsAI("Claude Alpha", groupName, "kb_search", map[string]any{"query": query}, &out); err != nil {
		return err
	}
	if len(out.Documents) != 1 || out.Documents[0].ID != w.putCreated.ID || out.Documents[0].Name != docName {
		return fmt.Errorf("expected retained document %s, got %+v", docName, out)
	}
	return nil
}

func (w *s3KBWorld) whenFirstBrowserForgets(_, groupName string) error {
	forget := proto.Forget{
		HomeCommand: proto.HomeCommand{Type: "forget", Nonce: w.ids.next(), Timestamp: w.clock.Load(), Group: proto.ID(w.groups[groupName])},
		Row:         "note",
		ID:          proto.ID(w.putCreated.ID),
	}
	return w.sendCmd(forget)
}

func (w *s3KBWorld) thenSecondGetsGoneAndSearchEmpty(_, query string) error {
	if err := w.awaitGoneOnB2("note", w.putCreated.ID); err != nil {
		return err
	}
	var out s3KBSearchOut
	if err := w.callToolAsAI("Claude Alpha", "Alpha", "kb_search", map[string]any{"query": query}, &out); err != nil {
		return err
	}
	if len(out.Documents) != 0 || out.More {
		return fmt.Errorf("expected empty search after forget, got %+v", out)
	}
	return nil
}

func (w *s3KBWorld) awaitGoneOnB2(rType, rowID string) error {
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case raw := <-w.b2.ch:
			if hasGoneMarker(raw, rType, rowID) {
				return nil
			}
		case <-deadline.C:
			return fmt.Errorf("second browser did not receive gone marker for %s/%s within 1 second", rType, rowID)
		}
	}
}

func hasGoneMarker(raw json.RawMessage, rType, rowID string) bool {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Type != "rows" {
		return false
	}
	for _, elem := range batch.Rows {
		var g proto.GoneRow
		if proto.Decode(elem, &g) == nil && g.Type == rType && g.ID == rowID && bool(g.Gone) {
			return true
		}
	}
	return false
}

func (w *s3KBWorld) whenAICallsSecretKBPut(aiName string) error {
	errText := w.callToolAsAI(aiName, "Alpha", "kb_put", map[string]any{
		"name": "SecretBodyDoc",
		"text": "token " + m3SecretToken,
	}, nil)
	errName := w.callToolAsAI(aiName, "Alpha", "kb_put", map[string]any{
		"name": m3SecretToken,
		"text": "clean body",
	}, nil)
	for _, err := range []error{errText, errName} {
		if err != nil {
			w.secretErrs = append(w.secretErrs, err.Error())
		}
	}
	return nil
}

func (w *s3KBWorld) thenSecretKBPutRefusedWithNoRow() error {
	if len(w.secretErrs) != 2 {
		return fmt.Errorf("got %d errors (%v), want 2 secret errors", len(w.secretErrs), w.secretErrs)
	}
	for _, msg := range w.secretErrs {
		if msg != m3SecretError {
			return fmt.Errorf("got tool error %q, want %q", msg, m3SecretError)
		}
	}
	var liveKBCount int
	err := w.cluster.Nodes[0].Store.Write(func(tx *store.Tx) error {
		return tx.QueryRow("SELECT count(*) FROM row WHERE g=? AND r='note' AND gone=0", w.groups["Alpha"]).Scan(&liveKBCount)
	})
	if err != nil {
		return err
	}
	if liveKBCount != 0 {
		return fmt.Errorf("expected 0 live note rows on home after secret refusals, got %d", liveKBCount)
	}
	return nil
}
