//go:build acceptance

package features

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/node/nodetest"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

const (
	s2ScenarioTimeout = 3 * time.Minute
	s2PName           = "P"
	s2QName           = "Q"
	s2SecretaryName   = "Secretary"
	s2PWorkName       = "P-work"
	s2QPersonalName   = "Q-personal"
	s2QTeamName       = "Q-team"
	s2SecretaryCLI    = "claude"
	s2PWorkCLI        = "claude"
	s2QPersonalCLI    = "codex"
	s2QTeamCLI        = "claude"
)

var (
	s2FakeCLIVersions = map[string]string{
		"claude": "2.1.311 (Claude Code)",
		"codex":  "codex-cli 0.164.0",
	}
)

func init() {
	s2DispatchSubproc()
}

func s2DispatchSubproc() {
	prog := m5ProgName(os.Args[0])
	if logPath := os.Getenv("SOLODAWN_S2_CLI_LOG"); logPath != "" {
		s2MaybeExitFakeCLI(prog, logPath, os.Args[1:])
	}
}

func s2MaybeExitFakeCLI(prog, logPath string, args []string) {
	ver, ok := s2FakeCLIVersions[prog]
	if !ok {
		return
	}
	os.Exit(s2RunFakeCLI(prog, ver, logPath, args))
}

type s2CLILogEntry struct {
	Kind    string `json:"kind"`
	Node    string `json:"node"`
	Vendor  string `json:"vendor"`
	Session string `json:"session"`
	Text    string `json:"text,omitempty"`
}

type s2CLIObserver struct {
	logPath string
	node    string
	vendor  string
	session string
	turnSeq int
}

func s2RunFakeCLI(vendor, version, logPath string, args []string) int {
	if slices.Equal(args, []string{"--version"}) {
		fmt.Fprintln(os.Stdout, version)
		return 0
	}
	if vendor == s2QTeamCLI && s2WatchInteractive(args) {
		return s2RunWatchInteractive()
	}
	obs := newS2CLIObserver(vendor, logPath, args)
	if s2ScriptExists(vendor) {
		return s2RunScriptedCLI(obs, version, args)
	}
	return s2RunDefaultCLI(obs)
}

func newS2CLIObserver(vendor, logPath string, args []string) *s2CLIObserver {
	obs := &s2CLIObserver{
		logPath: logPath,
		node:    os.Getenv("SOLODAWN_S2_NODE"),
		vendor:  vendor,
	}
	if vendor == "claude" {
		obs.session = s2ArgAfter(args, "--session-id", "--resume")
		obs.logEntry("start", "")
	}
	return obs
}

func s2ArgAfter(args []string, names ...string) string {
	i := slices.IndexFunc(args, func(a string) bool { return slices.Contains(names, a) })
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

func s2ScriptExists(vendor string) bool {
	path := os.Getenv("SOLODAWN_FAKE_" + strings.ToUpper(vendor) + "_SCRIPT")
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func s2RunScriptedCLI(obs *s2CLIObserver, version string, args []string) int {
	pipeR, pipeW := io.Pipe()
	defer pipeR.Close()
	go obs.pumpStdin(os.Stdin, pipeW)
	return fakecli.Run(obs.vendor, version, args, pipeR, os.Stdout, os.Stderr)
}

func (o *s2CLIObserver) pumpStdin(src io.Reader, dst *io.PipeWriter) {
	defer dst.Close()
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			o.observeLine(bytes.TrimSpace(line))
			if _, werr := dst.Write(line); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (o *s2CLIObserver) observeLine(line []byte) {
	if o.vendor == "claude" {
		o.observeClaudeLine(line)
		return
	}
	o.observeCodexLine(line)
}

func (o *s2CLIObserver) observeClaudeLine(line []byte) {
	var msg struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &msg) == nil && msg.Type == "user" {
		var text string
		if len(msg.Message.Content) > 0 {
			text = msg.Message.Content[0].Text
		}
		o.logEntry("turn", text)
	}
}

func (o *s2CLIObserver) observeCodexLine(line []byte) {
	var msg struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	if msg.Method == "thread/start" || msg.Method == "thread/resume" {
		o.observeCodexStart(msg.Params)
	}
	if msg.Method == "turn/start" {
		o.observeCodexTurn(msg.Params)
	}
}

func (o *s2CLIObserver) observeCodexStart(raw json.RawMessage) {
	var p struct {
		ThreadID string `json:"threadId"`
	}
	_ = json.Unmarshal(raw, &p)
	o.session = cmp.Or(p.ThreadID, "thr-"+cmp.Or(o.node, "s2"))
	o.logEntry("start", "")
}

func (o *s2CLIObserver) observeCodexTurn(raw json.RawMessage) {
	var p struct {
		Input []struct {
			Text string `json:"text"`
		} `json:"input"`
	}
	_ = json.Unmarshal(raw, &p)
	var text string
	if len(p.Input) > 0 {
		text = p.Input[0].Text
	}
	o.logEntry("turn", text)
}

func (o *s2CLIObserver) logEntry(kind, text string) {
	_ = s2AppendJSONLine(o.logPath, s2CLILogEntry{
		Kind:    kind,
		Node:    o.node,
		Vendor:  o.vendor,
		Session: o.session,
		Text:    text,
	})
}

func s2RunDefaultCLI(obs *s2CLIObserver) int {
	if obs.vendor == "claude" {
		return obs.defaultClaude(os.Stdin, os.Stdout)
	}
	return obs.defaultCodex(os.Stdin, os.Stdout)
}

func (o *s2CLIObserver) defaultClaude(in io.Reader, out io.Writer) int {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	first := true
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		o.observeClaudeLine(line)
		o.replyDefaultClaude(out, &first)
	}
	return 0
}

func (o *s2CLIObserver) replyDefaultClaude(out io.Writer, first *bool) {
	if *first {
		*first = false
		fmt.Fprintln(out, string(mustMarshalNoEscape(map[string]any{
			"type": "system", "subtype": "init", "session_id": o.session,
		})))
	}
	fmt.Fprintln(out, string(mustMarshalNoEscape(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "session_id": o.session, "result": "",
	})))
}

func (o *s2CLIObserver) defaultCodex(in io.Reader, out io.Writer) int {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		o.observeCodexLine(line)
		o.replyDefaultCodex(out, line)
	}
	return 0
}

func (o *s2CLIObserver) replyDefaultCodex(out io.Writer, line []byte) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if json.Unmarshal(line, &req) != nil || len(req.ID) == 0 {
		return
	}
	switch req.Method {
	case "initialize":
		fmt.Fprintf(out, `{"id":%s,"result":{}}`+"\n", req.ID)
	case "thread/start", "thread/resume":
		fmt.Fprintf(out, `{"id":%s,"result":{"thread":{"id":%q}}}`+"\n", req.ID, o.session)
	case "model/list":
		fmt.Fprintf(out, `{"id":%s,"result":{"data":[]}}`+"\n", req.ID)
	case "turn/start":
		o.writeCodexTurnDone(out, req.ID)
	}
}

func (o *s2CLIObserver) writeCodexTurnDone(out io.Writer, rawID json.RawMessage) {
	o.turnSeq++
	tid := fmt.Sprintf("turn-%d", o.turnSeq)
	fmt.Fprintf(out, `{"id":%s,"result":{"turn":{"id":%q,"status":"inProgress"}}}`+"\n", rawID, tid)
	fmt.Fprintf(out, `{"method":"turn/completed","params":{"threadId":%q,"turn":{"id":%q,"status":"completed","error":null}}}`+"\n", o.session, tid)
}

func s2AppendJSONLine(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(mustMarshalNoEscape(v), '\n'))
	return err
}

func s2ReadJSONLines[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s2DecodeLines[T](raw)
}

func s2DecodeLines[T any](raw []byte) ([]T, error) {
	var out []T
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type s2Person struct {
	Name     string
	Key      identity.Key
	Browser  *peerBrowser
	Browsers []*peerBrowser
	Entries  []proto.Envelope
	Conns    map[string]*peerBrowserConn
}

func (p *s2Person) ID() string { return p.Key.ID() }

type s2Node struct {
	Name  string
	Owner *s2Person
	Root  string
	Data  string
	Key   identity.Key
	Env   []string
	Cmd   *exec.Cmd
	Stop  func()
	Out   syncBuffer
}

func (n *s2Node) ID() string { return n.Key.ID() }

type s2CastConfig struct {
	Rendezvous string
	Timeout    time.Duration
}

type s2Cast struct {
	F          fixtures
	Ctx        context.Context
	Root       string
	BinDir     string
	CLILogPath string
	RVURL      string
	P          *s2Person
	Q          *s2Person
	P1         *s2Node
	P2         *s2Node
	Q1         *s2Node

	ids       seqIDGen
	groups    map[proto.ID]string
	mu        sync.Mutex
	undos     []func()
	closeOnce sync.Once
}

func startS2Cast(t testing.TB, cleanup func(func()), f fixtures, cfg s2CastConfig) (*s2Cast, error) {
	c := &s2Cast{F: f, groups: map[proto.ID]string{}}
	registerBothCleanups(t, cleanup, c.Close)
	cleanWindowsLoginTaskIfAbsent(t, c.addUndo)
	ctx, cancel := context.WithTimeout(context.Background(), cmp.Or(cfg.Timeout, s2ScenarioTimeout))
	c.Ctx = ctx
	c.addUndo(cancel)
	for _, step := range []func() error{
		func() error { return c.prepareDirsAndPath(t) },
		func() error { return c.prepareRendezvous(cfg.Rendezvous) },
		func() error { return c.prepareAndStartNodes(t) },
		c.connectBrowsersAndRegisterCLIs,
	} {
		if err := step(); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, nil
}

func (c *s2Cast) addUndo(fn func()) {
	c.mu.Lock()
	c.undos = append(c.undos, fn)
	c.mu.Unlock()
}

func (c *s2Cast) Close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		pending := slices.Clone(c.undos)
		c.undos = nil
		c.mu.Unlock()
		for _, fn := range slices.Backward(pending) {
			fn()
		}
	})
}

func (c *s2Cast) NextID() proto.ID { return c.ids.next() }

func (c *s2Cast) prepareDirsAndPath(t testing.TB) error {
	rawRoot, err := shortTempRoot(t, c.addUndo, "sds2")
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(rawRoot)
	if err != nil {
		return err
	}
	c.Root = realRoot
	c.BinDir = filepath.Join(realRoot, "bin")
	c.CLILogPath = filepath.Join(realRoot, "s2-cli.jsonl")
	if err := os.MkdirAll(c.BinDir, 0o700); err != nil {
		return err
	}
	if err := c.linkBinHelpers(); err != nil {
		return err
	}
	tb := scenarioTB{TB: t, cleanup: c.addUndo}
	tb.Setenv("PATH", c.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return nil
}

func (c *s2Cast) linkBinHelpers() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ext := m5ExeExt()
	for _, name := range []string{"claude", "codex"} {
		if err := m5CopyExec(exe, filepath.Join(c.BinDir, name+ext)); err != nil {
			return err
		}
	}
	return nil
}

func (c *s2Cast) prepareRendezvous(sharedURL string) error {
	rvURL, err := startAcceptanceRelayRVWithBack(c.addUndo, sharedURL)
	if err != nil {
		return err
	}
	c.RVURL = rvURL
	return nil
}

func (c *s2Cast) prepareAndStartNodes(t testing.TB) error {
	c.P = newS2Person(s2PName)
	c.Q = newS2Person(s2QName)
	var err error
	if c.P1, err = c.initCastNode("P1", c.P); err != nil {
		return err
	}
	if c.P2, err = c.initCastNode("P2", c.P); err != nil {
		return err
	}
	if c.Q1, err = c.initCastNode("Q1", c.Q); err != nil {
		return err
	}
	c.P.Entries = buildS2PersonLog(c.P, c.P1, c.P2)
	c.Q.Entries = buildS2PersonLog(c.Q, c.Q1)
	for _, n := range []*s2Node{c.P1, c.P2, c.Q1} {
		if err := c.seedAndStartNode(t, n); err != nil {
			return err
		}
	}
	return nil
}

func newS2Person(name string) *s2Person {
	return &s2Person{
		Name:  name,
		Key:   newRandomPeerKey(),
		Conns: map[string]*peerBrowserConn{},
	}
}

func (c *s2Cast) initCastNode(name string, owner *s2Person) (*s2Node, error) {
	nodeRoot := filepath.Join(c.Root, name)
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
	return &s2Node{Name: name, Owner: owner, Root: nodeRoot, Data: data, Key: key}, nil
}

func buildS2PersonLog(p *s2Person, nodes ...*s2Node) []proto.Envelope {
	ts := time.Now().UnixMilli()
	uid := p.Key.ID()
	entries := []proto.Envelope{
		nodetest.SignDeviceAdd(p.Key, uid, 1, "browser", p.Name+"-browser-1", uid, ts),
	}
	for i, n := range nodes {
		entries = append(entries, nodetest.SignDeviceAdd(p.Key, uid, i+2, "node", n.Name, n.ID(), ts))
	}
	return entries
}

func (c *s2Cast) seedAndStartNode(t testing.TB, n *s2Node) error {
	if err := c.seedCastNodeDB(n); err != nil {
		return err
	}
	n.Env = c.buildNodeEnv(n)
	return c.startNode(t, n)
}

// startNode starts the process of n on the data it has: a node that was stopped starts again so.
func (c *s2Cast) startNode(t testing.TB, n *s2Node) error {
	cmd := exec.Command(c.F.bin, "node")
	cmd.Env = n.Env
	cmd.Stdout, cmd.Stderr = &n.Out, &n.Out
	stop, err := startNodeCmd(t, c.addUndo, cmd, n.Name, n.Data, &n.Out)
	if err != nil {
		return err
	}
	n.Cmd, n.Stop = cmd, stop
	sock := filepath.Join(n.Data, "sock")
	if err := waitAcceptanceSocket(sock, 10*time.Second); err != nil {
		return fmt.Errorf("%s socket %s: %w (%s)", n.Name, sock, err, n.Out.String())
	}
	return nil
}

func (c *s2Cast) seedCastNodeDB(n *s2Node) error {
	db, err := store.Open(filepath.Join(n.Data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	rvJSON := string(mustMarshalNoEscape([]string{c.RVURL}))
	writeRV := func(tx *store.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO setting(key, value) VALUES('rendezvous', ?)`, rvJSON)
		return err
	}
	if err := db.Write(writeRV); err != nil {
		return err
	}
	return seedPeerOwnerEntries(db, n.Owner.ID(), n.Name, n.Owner.Entries)
}

func (c *s2Cast) buildNodeEnv(n *s2Node) []string {
	env := append(os.Environ(),
		"HOME="+n.Root,
		"XDG_DATA_HOME="+n.Root,
		"XDG_CONFIG_HOME="+n.Root,
		"LocalAppData="+n.Root,
		"SOLODAWN_S2_NODE="+n.Name,
		"SOLODAWN_S2_CLI_LOG="+c.CLILogPath,
		"SOLODAWN_FAKECLI_LOG="+filepath.Join(n.Root, "fakecli.log"),
		"SOLODAWN_FAKE_CLAUDE_SCRIPT="+filepath.Join(n.Root, "claude.json"),
		"SOLODAWN_FAKE_CODEX_SCRIPT="+filepath.Join(n.Root, "codex.json"),
	)
	return env
}

func (c *s2Cast) connectBrowsersAndRegisterCLIs() error {
	if err := c.connectOwnerBrowser(c.P, c.P1, c.P2); err != nil {
		return err
	}
	if err := c.connectOwnerBrowser(c.Q, c.Q1); err != nil {
		return err
	}
	if err := c.configureNodeAgent(c.P, c.P1, s2SecretaryCLI, "team"); err != nil {
		return err
	}
	if err := c.configureNodeAgent(c.P, c.P2, s2PWorkCLI, "team"); err != nil {
		return err
	}
	if err := c.configureNodeAgent(c.Q, c.Q1, s2QPersonalCLI, "personal"); err != nil {
		return err
	}
	return c.configureNodeAgent(c.Q, c.Q1, s2QTeamCLI, "team")
}

func (c *s2Cast) connectOwnerBrowser(p *s2Person, nodes ...*s2Node) error {
	b, err := newPeerBrowser(c.Ctx, c.addUndo, p.Key, c.RVURL, nil)
	if err != nil {
		return err
	}
	p.Browser = b
	p.Browsers = []*peerBrowser{b}
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID()
	}
	if err := b.waitOnline(c.Ctx, ids...); err != nil {
		return err
	}
	for _, n := range nodes {
		if err := c.openOwnerCtl(p, b, n.ID()); err != nil {
			return err
		}
	}
	return nil
}

func (c *s2Cast) openOwnerCtl(p *s2Person, b *peerBrowser, nodeID string) error {
	bc, err := b.openCtl(c.Ctx, c.addUndo, nodeID)
	if err != nil {
		return err
	}
	if err := checkOwnerWelcome(c.Ctx, bc.conn, nodeID); err != nil {
		return err
	}
	p.Conns[nodeID] = bc
	return nil
}

func (c *s2Cast) configureNodeAgent(p *s2Person, n *s2Node, agent, account string) error {
	bc := p.Conns[n.ID()]
	if _, err := requestHostInfo(c.Ctx, bc.conn); err != nil {
		return err
	}
	_, err := c.SendCmd(p, bc.conn, proto.AgentSet{
		HostCommand: proto.HostCommand{
			Type:      "agent.set",
			Nonce:     c.NextID(),
			Timestamp: time.Now().UnixMilli(),
			To:        n.ID(),
		},
		Agent:   agent,
		Account: account,
	})
	return err
}

func (c *s2Cast) AddBrowser(p *s2Person, label string) (*peerBrowser, error) {
	key := newRandomPeerKey()
	b, err := newPeerBrowser(c.Ctx, c.addUndo, key, c.RVURL, nil)
	if err != nil {
		return nil, err
	}
	p.Browsers = append(p.Browsers, b)
	if err := c.EnrollBrowser(p, key, label); err != nil {
		return nil, err
	}
	return b, nil
}

func (c *s2Cast) EnrollBrowser(p *s2Person, key identity.Key, label string) error {
	ver := len(p.Entries) + 1
	entry := nodetest.SignDeviceAdd(p.Key, p.ID(), ver, "browser", label, key.ID(), time.Now().UnixMilli())
	p.Entries = append(p.Entries, entry)
	dl := proto.Devlog{Type: "devlog", User: p.ID(), Entries: []proto.Envelope{entry}}
	for _, bc := range p.Conns {
		if err := bc.conn.Send(dl); err != nil {
			return err
		}
	}
	return nil
}

func (c *s2Cast) SendCmd(p *s2Person, conn *peer.Connection, payload any) (proto.CmdResult, error) {
	raw, err := sendSignedCmdRaw(c.Ctx, conn, p.Key, payload)
	if err != nil {
		return proto.CmdResult{}, err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return proto.CmdResult{}, fmt.Errorf("cmd failed: %s (%w)", raw, err)
	}
	return res, nil
}

func sendSignedCmdRaw(ctx context.Context, conn *peer.Connection, signer identity.Key, payload any) (json.RawMessage, error) {
	env := signer.Sign(string(mustMarshalNoEscape(payload)))
	return requestWait(ctx, conn, nil, func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	})
}

func (c *s2Cast) CreateGroup(p *s2Person, home *s2Node, name string) (proto.ID, error) {
	gid := c.NextID()
	bc := p.Conns[home.ID()]
	create := proto.GroupCreate{
		GroupCommand: proto.GroupCommand{
			HomeCommand: proto.HomeCommand{
				Type:      "group",
				Nonce:     gid,
				Timestamp: time.Now().UnixMilli(),
				Group:     gid,
			},
			Op: "create",
		},
		Name:       name,
		MemberName: p.Name,
	}
	if _, err := c.SendCmd(p, bc.conn, create); err != nil {
		return "", err
	}
	c.groups[gid] = name
	return gid, c.syncGroup(bc.conn, gid)
}

func (c *s2Cast) JoinGroup(p *s2Person, link s2JoinLink, name string) (*peerBrowserConn, proto.ID, error) {
	bc, member, err := c.sendJoin(p, link, name)
	if err != nil {
		return nil, "", err
	}
	return bc, member, c.followJoin(p, link)
}

func (c *s2Cast) sendJoin(p *s2Person, link s2JoinLink, name string) (*peerBrowserConn, proto.ID, error) {
	member := c.NextID()
	bc, raw, err := joinGroupWithLink(c.Ctx, c.addUndo, p.Browser, p.Key, p.Entries, link, name, member, time.Now())
	if err != nil {
		return nil, "", err
	}
	var res proto.CmdResult
	if err := proto.Decode(raw, &res); err != nil {
		return nil, "", fmt.Errorf("join: %s (%w)", raw, err)
	}
	p.Conns[link.Home] = bc
	return bc, member, nil
}

// followJoin is what the web client does after an accepted join (joinGroup in web/src/lib/join.ts):
// it sends home.add to the person's own nodes and syncs the group.
func (c *s2Cast) followJoin(p *s2Person, link s2JoinLink) error {
	for _, n := range []*s2Node{c.P1, c.P2, c.Q1} {
		if n.Owner != p {
			continue
		}
		_, err := c.SendCmd(p, p.Conns[n.ID()].conn, proto.HomeAdd{
			HostCommand: proto.HostCommand{Type: "home.add", Nonce: c.NextID(), Timestamp: time.Now().UnixMilli(), To: n.ID()},
			Group:       proto.ID(link.Group), Home: link.Home, Name: c.groups[proto.ID(link.Group)], Addrs: []string{c.RVURL},
		})
		if err != nil {
			return err
		}
	}
	return c.syncGroup(p.Conns[link.Home].conn, proto.ID(link.Group))
}

func (c *s2Cast) syncGroup(conn *peer.Connection, gid proto.ID) error {
	return syncS2Group(c.Ctx, conn, gid)
}

func syncS2Group(ctx context.Context, conn *peer.Connection, gid proto.ID) error {
	raw, err := requestWait(ctx, conn, nil, func(id int64) any {
		return proto.Sync{Request: proto.Request{Type: "sync", ID: id}, Group: string(gid), Since: 0, StateOnly: false}
	})
	if err != nil {
		return err
	}
	var synced proto.Synced
	return proto.Decode(raw, &synced)
}

func (c *s2Cast) JoinAI(p *s2Person, hostNode, homeNode *s2Node, gid proto.ID, agent, memberName string, secretary bool) (proto.AIMemberRow, error) {
	hostBC := p.Conns[hostNode.ID()]
	workspace, err := c.Workspace(hostNode, gid, memberName)
	if err != nil {
		return proto.AIMemberRow{}, err
	}
	joinCmd := proto.HostJoin{
		HostCommand: proto.HostCommand{
			Type:      "host.join",
			Nonce:     c.NextID(),
			Timestamp: time.Now().UnixMilli(),
			To:        hostNode.ID(),
		},
		Group:     gid,
		Home:      homeNode.ID(),
		Agent:     agent,
		Name:      memberName,
		Secretary: secretary,
		Workspace: workspace,
	}
	if _, err := c.SendCmd(p, hostBC.conn, joinCmd); err != nil {
		return proto.AIMemberRow{}, err
	}
	return c.waitAIMember(p.Conns[homeNode.ID()], gid, hostNode.ID(), agent, memberName)
}

// Workspace makes the folder an AI of the cast works in: its own, on its node and in its group.
func (c *s2Cast) Workspace(n *s2Node, gid proto.ID, ai string) (string, error) {
	return aiWorkspace(n.Root, gid, ai)
}

func (c *s2Cast) waitAIMember(homeBC *peerBrowserConn, gid proto.ID, wantHost, wantCLI, wantName string) (proto.AIMemberRow, error) {
	var found proto.AIMemberRow
	err := c.readGroupRowsUntil(homeBC, gid, func(row json.RawMessage) (bool, error) {
		var ai proto.AIMemberRow
		if proto.Decode(row, &ai) != nil || ai.Kind != "ai" || ai.Name != wantName {
			return false, nil
		}
		return verifyS2AIRow(ai, wantHost, wantCLI, &found)
	})
	return found, err
}

func verifyS2AIRow(ai proto.AIMemberRow, wantHost, wantCLI string, out *proto.AIMemberRow) (bool, error) {
	if ai.Host != wantHost || ai.CLI != wantCLI {
		return false, fmt.Errorf("unexpected AI member row: %+v", ai)
	}
	*out = ai
	return true, nil
}

func (c *s2Cast) readGroupRowsUntil(bc *peerBrowserConn, gid proto.ID, match func(json.RawMessage) (bool, error)) error {
	for {
		raw, err := bc.nextPush(c.Ctx)
		if err != nil {
			return err
		}
		done, err := scanS2RowsBatch(raw, string(gid), match)
		if err != nil || done {
			return err
		}
	}
}

func scanS2RowsBatch(raw json.RawMessage, wantGroup string, match func(json.RawMessage) (bool, error)) (bool, error) {
	var batch proto.Rows
	if proto.Decode(raw, &batch) != nil || batch.Group != wantGroup {
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

func (c *s2Cast) WriteScript(n *s2Node, vendor, content string) error {
	return os.WriteFile(filepath.Join(n.Root, vendor+".json"), []byte(content), 0o600)
}

func (c *s2Cast) ClearScript(n *s2Node, vendor string) error {
	err := os.Remove(filepath.Join(n.Root, vendor+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (c *s2Cast) CLILogs() ([]s2CLILogEntry, error) {
	return s2ReadJSONLines[s2CLILogEntry](c.CLILogPath)
}

func TestS2Cast(t *testing.T) {
	stopTimer := armDeadlineCleanups(t)
	defer stopTimer()
	f := buildS2TestFixtures(t)
	http.DefaultClient.CloseIdleConnections()

	cast, err := startS2Cast(t, nil, f, s2CastConfig{})
	if err != nil {
		t.Fatalf("startS2Cast: %v", err)
	}
	verifyS2NodesAndAccounts(t, cast)
	verifyS2SecondBrowser(t, cast)
	verifyS2PGroupAndCrossNodeAI(t, cast)
	verifyS2QAIsAndCLILogs(t, cast)
	verifyS2CleanTeardown(t, cast)
}

func buildS2TestFixtures(t *testing.T) fixtures {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "solodawn.exe")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/solodawn").CombinedOutput(); err != nil {
		t.Fatalf("go build solodawn: %v\n%s", err, out)
	}
	return fixtures{t: t, bin: bin, stamped: map[string]string{}}
}

func verifyS2NodesAndAccounts(t *testing.T, cast *s2Cast) {
	t.Helper()
	assertS2NodeAgentAccount(t, cast, cast.P, cast.P1, s2SecretaryCLI, "team")
	assertS2NodeAgentAccount(t, cast, cast.P, cast.P2, s2PWorkCLI, "team")
	assertS2NodeAgentAccount(t, cast, cast.Q, cast.Q1, s2QPersonalCLI, "personal")
	assertS2NodeAgentAccount(t, cast, cast.Q, cast.Q1, s2QTeamCLI, "team")
}

func assertS2NodeAgentAccount(t *testing.T, cast *s2Cast, p *s2Person, n *s2Node, agent, wantAccount string) {
	t.Helper()
	info, err := requestHostInfo(cast.Ctx, p.Conns[n.ID()].conn)
	if err != nil {
		t.Fatalf("host.info %s: %v", n.Name, err)
	}
	idx := slices.IndexFunc(info.Agents, func(a proto.HostAgent) bool { return a.Agent == agent })
	if idx < 0 || info.Agents[idx].Account == nil || *info.Agents[idx].Account != wantAccount {
		t.Fatalf("%s agent %s = %+v, want account %q", n.Name, agent, info.Agents, wantAccount)
	}
}

func verifyS2SecondBrowser(t *testing.T, cast *s2Cast) {
	t.Helper()
	b2, err := cast.AddBrowser(cast.Q, "Q-browser-2")
	if err != nil {
		t.Fatalf("AddBrowser: %v", err)
	}
	if err := waitS2NodeAcceptsBrowser(cast, b2, cast.Q1.ID()); err != nil {
		t.Fatalf("second browser ctl to Q1: %v", err)
	}
}

func waitS2NodeAcceptsBrowser(cast *s2Cast, b *peerBrowser, nodeID string) error {
	return waitUntilEvery(500*time.Millisecond, s2ScenarioTimeout, "the node to welcome the second browser", func() error {
		ok, err := tryS2OwnerWelcome(cast, b, nodeID)
		if err != nil {
			return abort(err)
		}
		if !ok {
			return errors.New("no welcome")
		}
		return nil
	})
}

func tryS2OwnerWelcome(cast *s2Cast, b *peerBrowser, nodeID string) (bool, error) {
	bc, ok, err := b.tryOpenCtl(cast.Ctx, cast.addUndo, nodeID, s2ScenarioTimeout, new(trail))
	if err != nil || !ok {
		return false, err
	}
	if err := checkOwnerWelcome(cast.Ctx, bc.conn, nodeID); err != nil {
		bc.conn.Close()
		return false, nil
	}
	return true, nil
}

func verifyS2PGroupAndCrossNodeAI(t *testing.T, cast *s2Cast) {
	t.Helper()
	gid, err := cast.CreateGroup(cast.P, cast.P1, "Alpha")
	if err != nil {
		t.Fatalf("CreateGroup P1: %v", err)
	}
	if _, err := cast.JoinAI(cast.P, cast.P1, cast.P1, gid, s2SecretaryCLI, s2SecretaryName, true); err != nil {
		t.Fatalf("JoinAI Secretary on P1: %v", err)
	}
	pWork, err := cast.JoinAI(cast.P, cast.P2, cast.P1, gid, s2PWorkCLI, s2PWorkName, false)
	if err != nil {
		t.Fatalf("JoinAI P-work on P2: %v", err)
	}
	sendS2MentionAndWaitIdle(t, cast, cast.P, cast.P1, gid, pWork.ID, "ping P2 worker")
}

func verifyS2QAIsAndCLILogs(t *testing.T, cast *s2Cast) {
	t.Helper()
	gid, err := cast.CreateGroup(cast.Q, cast.Q1, "Q-Smoke")
	if err != nil {
		t.Fatalf("CreateGroup Q1: %v", err)
	}
	qTeam, err := cast.JoinAI(cast.Q, cast.Q1, cast.Q1, gid, s2QTeamCLI, s2QTeamName, false)
	if err != nil {
		t.Fatalf("JoinAI Q-team: %v", err)
	}
	qPers, err := cast.JoinAI(cast.Q, cast.Q1, cast.Q1, gid, s2QPersonalCLI, s2QPersonalName, false)
	if err != nil {
		t.Fatalf("JoinAI Q-personal: %v", err)
	}
	script := m5ClaudeScript([]m5ToolCallSpec{{Name: "post", Args: map[string]any{"text": "ack from Q-team"}}})
	if err := cast.WriteScript(cast.Q1, s2QTeamCLI, script); err != nil {
		t.Fatal(err)
	}
	sendS2MentionAndWaitPost(t, cast, cast.Q, cast.Q1, gid, qTeam.ID, "hello Q-team", "ack from Q-team")
	if err := cast.ClearScript(cast.Q1, s2QTeamCLI); err != nil {
		t.Fatal(err)
	}
	sendS2MentionAndWaitIdle(t, cast, cast.Q, cast.Q1, gid, qPers.ID, "hello Q-personal")
	assertS2CLILogsComplete(t, cast)
}

func sendS2MentionAndWaitIdle(t *testing.T, cast *s2Cast, p *s2Person, home *s2Node, gid proto.ID, aiID, text string) {
	t.Helper()
	if err := postS2Mention(cast, p, home, gid, aiID, text); err != nil {
		t.Fatal(err)
	}
	var sawWorking bool
	err := cast.readGroupRowsUntil(p.Conns[home.ID()], gid, func(row json.RawMessage) (bool, error) {
		return matchS2WorkingThenWaiting(row, aiID, &sawWorking), nil
	})
	if err != nil {
		t.Fatalf("wait AI %s waiting: %v", aiID, err)
	}
}

func matchS2WorkingThenWaiting(row json.RawMessage, aiID string, sawWorking *bool) bool {
	var ai proto.AIMemberRow
	if proto.Decode(row, &ai) != nil || ai.ID != aiID {
		return false
	}
	if ai.State == "working" {
		*sawWorking = true
		return false
	}
	return *sawWorking && ai.State == "waiting"
}

func sendS2MentionAndWaitPost(t *testing.T, cast *s2Cast, p *s2Person, home *s2Node, gid proto.ID, aiID, text, wantReply string) {
	t.Helper()
	if err := postS2Mention(cast, p, home, gid, aiID, text); err != nil {
		t.Fatal(err)
	}
	var sawReply bool
	err := cast.readGroupRowsUntil(p.Conns[home.ID()], gid, func(row json.RawMessage) (bool, error) {
		return matchS2PostThenWaiting(row, aiID, wantReply, &sawReply), nil
	})
	if err != nil {
		t.Fatalf("wait AI %s reply %q: %v", aiID, wantReply, err)
	}
}

func matchS2PostThenWaiting(row json.RawMessage, aiID, wantReply string, sawReply *bool) bool {
	if !*sawReply {
		var msg proto.MsgRow
		*sawReply = proto.Decode(row, &msg) == nil && msg.By == aiID && msg.Text == wantReply
		return false
	}
	var ai proto.AIMemberRow
	return proto.Decode(row, &ai) == nil && ai.ID == aiID && ai.State == "waiting"
}

func postS2Mention(cast *s2Cast, p *s2Person, home *s2Node, gid proto.ID, aiID, text string) error {
	mentions := []proto.ID{}
	if aiID != "" {
		mentions = append(mentions, proto.ID(aiID))
	}
	_, err := cast.SendCmd(p, p.Conns[home.ID()].conn, proto.Post[proto.EmptyBody]{
		HomeCommand: proto.HomeCommand{
			Type:      "post",
			Nonce:     cast.NextID(),
			Timestamp: time.Now().UnixMilli(),
			Group:     gid,
		},
		Kind:     "text",
		Text:     text,
		Main:     true,
		Mentions: mentions,
		Body:     proto.EmptyBody{},
	})
	return err
}

func assertS2CLILogsComplete(t *testing.T, cast *s2Cast) {
	t.Helper()
	entries, err := cast.CLILogs()
	if err != nil {
		t.Fatalf("CLILogs: %v", err)
	}
	assertHasS2CLITurn(t, entries, "P2", s2PWorkCLI, "ping P2 worker")
	assertHasS2CLITurn(t, entries, "Q1", s2QTeamCLI, "hello Q-team")
	assertHasS2CLITurn(t, entries, "Q1", s2QPersonalCLI, "hello Q-personal")
}

func assertHasS2CLITurn(t *testing.T, entries []s2CLILogEntry, node, vendor, sub string) {
	t.Helper()
	hasStart := slices.ContainsFunc(entries, func(e s2CLILogEntry) bool {
		return e.Kind == "start" && e.Node == node && e.Vendor == vendor
	})
	hasTurn := slices.ContainsFunc(entries, func(e s2CLILogEntry) bool {
		return e.Kind == "turn" && e.Node == node && e.Vendor == vendor && strings.Contains(e.Text, sub)
	})
	if !hasStart || !hasTurn {
		t.Fatalf("missing start/turn for (%s, %s, %q) in %+v", node, vendor, sub, entries)
	}
}

func verifyS2CleanTeardown(t *testing.T, cast *s2Cast) {
	t.Helper()
	cast.Close()
	for _, n := range []*s2Node{cast.P1, cast.P2, cast.Q1} {
		if n.Cmd == nil || n.Cmd.ProcessState == nil {
			t.Fatalf("node %s process did not exit cleanly", n.Name)
		}
	}
	http.DefaultClient.CloseIdleConnections()
}
