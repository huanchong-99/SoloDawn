//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
)

type m6NodeProc struct {
	name string
	root string
	data string
	key  identity.Key
	cmd  *exec.Cmd
	out  syncBuffer
}

type m6World struct {
	f          fixtures
	cleanup    func(func())
	ctx        context.Context
	root       string
	repoDir    string
	cliLogPath string
	rvURL      string
	ownerKey   identity.Key
	browser    *peerBrowser
	homeNode   *m6NodeProc
	aiNode     *m6NodeProc
	homeLink   *peerBrowserConn
	aiLink     *peerBrowserConn
	groupID    proto.ID
	aiMemberID string
	ids        seqIDGen
}

func m6Steps(sc *godog.ScenarioContext, f fixtures) {
	w := &m6World{
		f:       f,
		cleanup: afterScenario(sc),
	}
	sc.Step(`^two owned nodes over ctl and a project group on the first node$`, w.givenTwoNodesAndGroup)
	sc.Step(`^"([^"]+)" on the second node is scripted to answer "([^"]+)" with "([^"]+)" and "([^"]+)" with "([^"]+)" as "([^"]+)"$`, w.givenTwoTurnScript)
	sc.Step(`^the owner joins "([^"]+)" on the second node to the group as "([^"]+)"$`, w.givenJoinedRemoteAI)
	sc.Step(`^the owner mentions "([^"]+)" in the group with "([^"]+)"$`, w.whenOwnerMentionsAI)
	sc.Step(`^"([^"]+)" posts "([^"]+)" from the second node and enters waiting$`, w.thenAIPostsAndWaits)
	sc.Step(`^one "([^"]+)" CLI process existed throughout on the second node$`, w.thenOneCLIProcessRan)
}

func (w *m6World) givenTwoNodesAndGroup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	if err := w.prepareAndStartNodes(); err != nil {
		return err
	}
	if err := w.connectOwnerToNodes(); err != nil {
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
	w.cliLogPath = filepath.Join(realRoot, "fakecli.log")
	w.repoDir = filepath.Join(realRoot, "repo")
	return initLocalGitRepo(w.repoDir)
}

func (w *m6World) prepareAndStartNodes() error {
	w.ownerKey = m6NewKey()
	homeNode, err := w.initNodeDir("home-node")
	if err != nil {
		return err
	}
	aiNode, err := w.initNodeDir("ai-node")
	if err != nil {
		return err
	}
	w.homeNode, w.aiNode = homeNode, aiNode
	entries := w.buildDeviceLog(homeNode, aiNode)
	if err := w.seedNodeDB(homeNode, entries); err != nil {
		return err
	}
	if err := w.seedNodeDB(aiNode, entries); err != nil {
		return err
	}
	if err := w.startNodeProc(homeNode, nil); err != nil {
		return err
	}
	extraEnv := []string{
		"SOLODAWN_FAKECLI_LOG=" + w.cliLogPath,
		"SOLODAWN_FAKE_CLAUDE_SCRIPT=" + filepath.Join(w.root, "claude.json"),
		"SOLODAWN_FAKE_CODEX_SCRIPT=" + filepath.Join(w.root, "codex.json"),
	}
	return w.startNodeProc(aiNode, extraEnv)
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

func (w *m6World) buildDeviceLog(n1, n2 *m6NodeProc) []proto.Envelope {
	ts := time.Now().UnixMilli()
	ownerID := w.ownerKey.ID()
	return []proto.Envelope{
		nodetest.SignDeviceAdd(w.ownerKey, ownerID, 1, "browser", "owner", ownerID, ts),
		nodetest.SignDeviceAdd(w.ownerKey, ownerID, 2, "node", n1.name, n1.key.ID(), ts),
		nodetest.SignDeviceAdd(w.ownerKey, ownerID, 3, "node", n2.name, n2.key.ID(), ts),
	}
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

func (w *m6World) startNodeProc(np *m6NodeProc, extraEnv []string) error {
	env := append(os.Environ(), "HOME="+np.root, "XDG_DATA_HOME="+np.root, "LocalAppData="+np.root)
	env = append(env, extraEnv...)
	cmd := exec.Command(w.f.bin, "node")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = &np.out, &np.out
	if _, err := startManagedCmd(w.f.t, w.cleanup, cmd); err != nil {
		return err
	}
	np.cmd = cmd
	sock := filepath.Join(np.data, "sock")
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return fmt.Errorf("%s socket %s: %w (%s)", np.name, sock, err, np.out.String())
	}
	return nil
}

func (w *m6World) connectOwnerToNodes() error {
	b, err := newPeerBrowser(w.ctx, w.cleanup, w.ownerKey, w.rvURL, nil)
	if err != nil {
		return err
	}
	w.browser = b
	if err := b.waitOnline(w.ctx, w.homeNode.key.ID(), w.aiNode.key.ID()); err != nil {
		return err
	}
	homeLink, err := w.dialAndWelcome(w.homeNode.key.ID())
	if err != nil {
		return err
	}
	aiLink, err := w.dialAndWelcome(w.aiNode.key.ID())
	if err != nil {
		return err
	}
	w.homeLink, w.aiLink = homeLink, aiLink
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
			HomeCommand: proto.HomeCommand{
				Type:      "group",
				Nonce:     w.groupID,
				Timestamp: time.Now().UnixMilli(),
				Group:     w.groupID,
			},
			Op: "create",
		},
		Name:       m6GroupName,
		Kind:       "project",
		Repo:       w.repoDir,
		Base:       "main",
		Forge:      "none",
		Checks:     []string{},
		MemberName: m6OwnerName,
	}
	if err := w.sendSignedCmd(w.homeLink.conn, create); err != nil {
		return err
	}
	raw, err := w.homeLink.conn.Request(w.ctx, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(w.groupID), Since: 0, StateOnly: false}
	})
	if err != nil {
		return err
	}
	var synced proto.Synced
	return proto.Decode(raw, &synced)
}

func (w *m6World) sendSignedCmd(conn *peer.Connection, payload any) error {
	env := w.ownerKey.Sign(string(mustMarshalNoEscape(payload)))
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
	var doc map[string]any
	if agent == "claude" {
		steps := slices.Concat(claudeTurnSteps(1, true, turn1, a1, ""), claudeTurnSteps(2, false, turn2, a2, ""))
		steps = append(steps, map[string]any{"dir": "eof"})
		doc = map[string]any{"steps": steps}
	} else {
		steps := slices.Concat(codexStartupSteps(), codexTurnSteps("turn-1", 1, turn1, a1, false), codexTurnSteps("turn-2", 2, turn2, a2, false))
		steps = append(steps, map[string]any{"dir": "eof"})
		doc = map[string]any{"session": "thr-m6-codex-1", "steps": steps}
	}
	return os.WriteFile(filepath.Join(w.root, agent+".json"), mustMarshalNoEscape(doc), 0o600)
}

func formatM6MentionTurn(member, body string) string {
	header := fmt.Sprintf("[SoloDawn] group %q · you are %q · group session", m6GroupName, member)
	block := fmt.Sprintf("== 1 of 1 · mention · from %q in main · instruction", m6OwnerName)
	return header + "\n" + block + "\n" + body
}

func (w *m6World) givenJoinedRemoteAI(agent, memberName string) error {
	if _, err := requestHostInfo(w.ctx, w.aiLink.conn); err != nil {
		return err
	}
	setCmd := proto.AgentSet{
		HostCommand: proto.HostCommand{
			Type:      "agent.set",
			Nonce:     w.ids.next(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.aiNode.key.ID(),
		},
		Agent:   agent,
		Account: "personal",
	}
	if err := w.sendSignedCmd(w.aiLink.conn, setCmd); err != nil {
		return err
	}
	joinCmd := proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type:      "host.join",
			Nonce:     w.ids.next(),
			Timestamp: time.Now().UnixMilli(),
			To:        w.aiNode.key.ID(),
		},
		Group:     w.groupID,
		Home:      w.homeNode.key.ID(),
		Agent:     agent,
		Name:      memberName,
		Secretary: false,
	}
	if err := w.sendSignedCmd(w.aiLink.conn, joinCmd); err != nil {
		return err
	}
	return w.awaitJoinedMember(agent, memberName)
}

func (w *m6World) awaitJoinedMember(agent, memberName string) error {
	match := func(raw json.RawMessage) (bool, error) {
		var ai proto.AIMemberRow
		if proto.Decode(raw, &ai) != nil || ai.Kind != "ai" || ai.Name != memberName {
			return false, nil
		}
		return w.verifyJoinedRow(ai, agent)
	}
	return w.readHomeRowsUntil(match)
}

func (w *m6World) verifyJoinedRow(ai proto.AIMemberRow, agent string) (bool, error) {
	if ai.CLI != agent || ai.Host != w.aiNode.key.ID() {
		return false, fmt.Errorf("unexpected AI member row: %+v", ai)
	}
	w.aiMemberID = ai.ID
	return true, nil
}

func (w *m6World) whenOwnerMentionsAI(_ string, text string) error {
	post := proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     w.ids.next(),
			Timestamp: time.Now().UnixMilli(),
			Group:     w.groupID,
		},
		Kind:     "text",
		Text:     text,
		Mentions: []proto.ID{proto.ID(w.aiMemberID)},
		Body:     proto.EmptyBody{},
	}
	return w.sendSignedCmd(w.homeLink.conn, post)
}

func (w *m6World) thenAIPostsAndWaits(_ string, wantText string) error {
	var sawAnswer bool
	match := func(raw json.RawMessage) (bool, error) {
		if !sawAnswer {
			sawAnswer = w.matchesAIPost(raw, wantText)
			return false, nil
		}
		return w.matchesWaitingAI(raw), nil
	}
	return w.readHomeRowsUntil(match)
}

func (w *m6World) matchesAIPost(raw json.RawMessage, wantText string) bool {
	var msg proto.MsgRow
	if proto.Decode(raw, &msg) != nil {
		return false
	}
	return msg.By == w.aiMemberID && msg.Kind == "text" && msg.Text == wantText
}

func (w *m6World) matchesWaitingAI(raw json.RawMessage) bool {
	var ai proto.AIMemberRow
	if proto.Decode(raw, &ai) != nil || ai.ID != w.aiMemberID {
		return false
	}
	return ai.Host == w.aiNode.key.ID() && ai.State == "waiting"
}

func (w *m6World) readHomeRowsUntil(match func(json.RawMessage) (bool, error)) error {
	for {
		rawFrame, err := w.homeLink.nextPush(w.ctx)
		if err != nil {
			return fmt.Errorf("waiting for home push: %w\nhome=%s\nai=%s", err, w.homeNode.out.String(), w.aiNode.out.String())
		}
		done, err := w.scanRowsFrame(rawFrame, match)
		if err != nil || done {
			return err
		}
	}
}

func (w *m6World) scanRowsFrame(rawFrame json.RawMessage, match func(json.RawMessage) (bool, error)) (bool, error) {
	var batch proto.Rows
	if proto.Decode(rawFrame, &batch) != nil || batch.Group != string(w.groupID) {
		return false, nil
	}
	for _, row := range batch.Rows {
		done, err := match(row)
		if err != nil || done {
			return done, err
		}
	}
	return false, nil
}

func (w *m6World) thenOneCLIProcessRan(agent string) error {
	return readSingleStartLine(w.cliLogPath, agent)
}

func readSingleStartLine(path, agent string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		return fmt.Errorf("expected one start line after both turns, got %q", string(raw))
	}
	prefix := "start " + agent + " "
	sessionID, ok := strings.CutPrefix(lines[0], prefix)
	if !ok || sessionID == "" {
		return fmt.Errorf("unexpected start line %q for %s", lines[0], agent)
	}
	return nil
}

func m6NewKey() identity.Key {
	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	return identity.Key(ed25519.NewKeyFromSeed(seed))
}
