//go:build acceptance

package features

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

var m5CurrentWorld atomic.Pointer[m5World]

func init() {
	prog := m5ProgName(os.Args[0])
	if prog == "gh" && os.Getenv("SOLODAWN_M5_GH_DIR") != "" {
		os.Exit(m5RunFakeGH(os.Getenv("SOLODAWN_M5_GH_DIR"), os.Args[1:]))
	}
	if prog == "m5envcheck" {
		os.Exit(m5RunEnvCheck())
	}
	if len(os.Args) > 2 && os.Args[1] == "m5-mcp-shim" {
		os.Exit(m5RunMCPShim(os.Args[2], os.Args[3:]))
	}
	host.Register(host.Listeners, "m5_turns", m5OnHostEvent)
	host.Register(host.Holds, "m5_hold", m5OnHostHold)
	stepSets["a_m5"] = m5Steps
}

func m5OnHostEvent(_ *host.Host, _ host.SessionKey, e host.Event) {
	w := m5CurrentWorld.Load()
	if w != nil && e.Kind == host.TurnBegan {
		w.mu.Lock()
		w.turns++
		w.mu.Unlock()
	}
}

func m5OnHostHold(_ *host.Host, key host.SessionKey) (string, bool) {
	w := m5CurrentWorld.Load()
	if w == nil {
		return "", false
	}
	w.mu.Lock()
	held := w.closing || w.heldMembers[key.Member]
	w.mu.Unlock()
	if held {
		return "offline", true
	}
	return "", false
}

func m5ProgName(arg0 string) string {
	return strings.TrimSuffix(filepath.Base(arg0), ".exe")
}

func m5RunEnvCheck() int {
	if os.Getenv("PATH") == "" || os.Getenv("HOME") == "" {
		fmt.Fprintln(os.Stderr, "missing PATH or HOME")
		return 1
	}
	if os.Getenv("SOLODAWN_M5_TEST_TOKEN") != "" {
		fmt.Fprintln(os.Stderr, "leaked SOLODAWN_M5_TEST_TOKEN")
		return 1
	}
	fmt.Println("env ok")
	return 0
}

func m5RunMCPShim(addr string, args []string) int {
	dial := func() (net.Conn, error) { return net.Dial("tcp", addr) }
	return host.RunShim(args, dial, os.Stdin, os.Stdout, os.Stderr)
}

type m5GHPR struct {
	Number   int    `json:"number"`
	Branch   string `json:"branch"`
	Base     string `json:"base"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`
	State    string `json:"state"`
	HeadOid  string `json:"head_oid"`
	MergeOid string `json:"merge_oid"`
}

type m5GHState struct {
	BareRepo string   `json:"bare_repo"`
	PRs      []m5GHPR `json:"prs"`
}

func m5RunFakeGH(dir string, args []string) int {
	st, err := m5LoadGHState(dir)
	if err != nil || len(args) < 2 {
		fmt.Fprintln(os.Stderr, "fake gh state error")
		return 1
	}
	code, out, errText := m5DispatchGH(dir, &st, args)
	if out != "" {
		fmt.Fprint(os.Stdout, out)
	}
	if errText != "" {
		fmt.Fprintln(os.Stderr, errText)
	}
	return code
}

func m5DispatchGH(dir string, st *m5GHState, args []string) (int, string, string) {
	key := args[0] + "/" + args[1]
	switch key {
	case "pr/list":
		return m5GHPRList(st, args[2:])
	case "pr/create":
		return m5GHPRCreate(dir, st, args[2:])
	case "pr/view":
		return m5GHPRView(st, args[2:])
	case "pr/merge":
		return m5GHPRMerge(dir, st, args[2:])
	case "api/graphql":
		return m5GHGraphQL(st, args[2:])
	}
	return 1, "", "unsupported gh subcommand: " + key
}

func m5LoadGHState(dir string) (m5GHState, error) {
	var st m5GHState
	raw, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		err = json.Unmarshal(raw, &st)
	}
	return st, err
}

func m5SaveGHState(dir string, st m5GHState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "state.json"), raw, 0o600)
}

func m5FlagVal(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func m5GHPRList(st *m5GHState, args []string) (int, string, string) {
	head, base := m5FlagVal(args, "--head"), m5FlagVal(args, "--base")
	type item struct {
		URL               string `json:"url"`
		IsCrossRepository bool   `json:"isCrossRepository"`
	}
	out := []item{}
	for _, pr := range st.PRs {
		if pr.State == "OPEN" && pr.Branch == head && pr.Base == base {
			out = append(out, item{URL: pr.URL})
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return 1, "", err.Error()
	}
	return 0, string(raw) + "\n", ""
}

func m5GHPRCreate(dir string, st *m5GHState, args []string) (int, string, string) {
	num := len(st.PRs) + 1
	url := fmt.Sprintf("https://github.example/acme/proj/pull/%d", num)
	pr := m5GHPR{
		Number: num,
		Branch: m5FlagVal(args, "--head"),
		Base:   m5FlagVal(args, "--base"),
		Title:  m5FlagVal(args, "--title"),
		Body:   m5FlagVal(args, "--body"),
		URL:    url,
		State:  "OPEN",
	}
	st.PRs = append(st.PRs, pr)
	if err := m5SaveGHState(dir, *st); err != nil {
		return 1, "", err.Error()
	}
	return 0, url + "\n", ""
}

func m5FindPR(st *m5GHState, numStr string) (*m5GHPR, error) {
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return nil, err
	}
	for i := range st.PRs {
		if st.PRs[i].Number == n {
			return &st.PRs[i], nil
		}
	}
	return nil, errors.New("pull request not found")
}

func m5GHPRView(st *m5GHState, args []string) (int, string, string) {
	if len(args) == 0 {
		return 1, "", "missing pr number"
	}
	pr, err := m5FindPR(st, args[0])
	if err != nil {
		return 1, "", err.Error()
	}
	head, err := m5GitOut(st.BareRepo, "rev-parse", "--verify", "refs/heads/"+pr.Branch)
	if err != nil {
		return 1, "", err.Error()
	}
	body := fmt.Sprintf(`{"headRefOid":%q,"statusCheckRollup":[{"__typename":"CheckRun","name":"ci","status":"COMPLETED","conclusion":"SUCCESS"}]}`, head)
	return 0, body + "\n", ""
}

func m5GHPRMerge(dir string, st *m5GHState, args []string) (int, string, string) {
	if len(args) == 0 {
		return 1, "", "missing pr number"
	}
	pr, err := m5FindPR(st, args[0])
	if err != nil {
		return 1, "", err.Error()
	}
	want := m5FlagVal(args, "--match-head-commit")
	head, err := m5GitOut(st.BareRepo, "rev-parse", "--verify", "refs/heads/"+pr.Branch)
	if err != nil || head != want {
		return 1, "", "head commit does not match reviewed commit"
	}
	return m5SquashRemotePR(dir, st, pr, head)
}

func m5SquashRemotePR(dir string, st *m5GHState, pr *m5GHPR, head string) (int, string, string) {
	tmp, err := os.MkdirTemp(dir, "gh-merge-*")
	if err != nil {
		return 1, "", err.Error()
	}
	defer os.RemoveAll(tmp)
	mergeOid, err := m5RunSquashGit(st.BareRepo, tmp, pr.Base, pr.Branch, pr.Number)
	if err != nil {
		return 1, "", err.Error()
	}
	pr.State, pr.HeadOid, pr.MergeOid = "MERGED", head, mergeOid
	if err := m5SaveGHState(dir, *st); err != nil {
		return 1, "", err.Error()
	}
	return 0, "merged\n", ""
}

func m5RunSquashGit(bareRepo, tmp, base, branch string, prNum int) (string, error) {
	cloneDir := filepath.Join(tmp, "work")
	steps := [][]string{
		{"clone", "--branch", base, bareRepo, cloneDir},
		{"-C", cloneDir, "fetch", "origin", "refs/heads/" + branch},
		{"-C", cloneDir, "merge", "--squash", "FETCH_HEAD"},
		{"-C", cloneDir, "-c", "user.name=gh", "-c", "user.email=gh@test.invalid", "commit", "--allow-empty", "-m", fmt.Sprintf("Merge PR %d", prNum)},
		{"-C", cloneDir, "push", "origin", "HEAD:refs/heads/" + base},
		{"-C", bareRepo, "update-ref", "-d", "refs/heads/" + branch},
	}
	for _, s := range steps {
		if _, err := m5GitOut("", s...); err != nil {
			return "", err
		}
	}
	return m5GitOut(bareRepo, "rev-parse", "--verify", "refs/heads/"+base)
}

func m5GHGraphQL(st *m5GHState, args []string) (int, string, string) {
	numStr := ""
	for _, a := range args {
		if rest, ok := strings.CutPrefix(a, "number="); ok {
			numStr = rest
		}
	}
	pr, err := m5FindPR(st, numStr)
	if err != nil {
		return 1, "", err.Error()
	}
	head, err := m5ResolvePRHead(st.BareRepo, pr)
	if err != nil {
		return 1, "", err.Error()
	}
	body := fmt.Sprintf(`{"state":%q,"headRefOid":%q,"mergeCommit":{"oid":%q},"autoMergeRequest":null,"mergeQueueEntry":null}`, pr.State, head, pr.MergeOid)
	return 0, body + "\n", ""
}

func m5ResolvePRHead(bareRepo string, pr *m5GHPR) (string, error) {
	if pr.HeadOid != "" {
		return pr.HeadOid, nil
	}
	return m5GitOut(bareRepo, "rev-parse", "--verify", "refs/heads/"+pr.Branch)
}

func m5GitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func m5GitCommitFile(dir, relPath, content, msg string) (string, error) {
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return "", err
	}
	if _, err := m5GitOut(dir, "add", "--", relPath); err != nil {
		return "", err
	}
	if _, err := m5GitOut(dir, "-c", "user.name=test", "-c", "user.email=test@test.invalid", "commit", "-m", msg); err != nil {
		return "", err
	}
	return m5GitOut(dir, "rev-parse", "--verify", "HEAD")
}

func m5GitCommitFiles(dir string, files map[string]string, msg string) (string, error) {
	keys := m5MapKeys(files)
	slices.Sort(keys)
	for _, rel := range keys {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, []byte(files[rel]), 0o600); err != nil {
			return "", err
		}
	}
	if _, err := m5GitOut(dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := m5GitOut(dir, "-c", "user.name=test", "-c", "user.email=test@test.invalid", "commit", "-m", msg); err != nil {
		return "", err
	}
	return m5GitOut(dir, "rev-parse", "--verify", "HEAD")
}

func m5MapKeys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type m5ToolCallSpec struct {
	Name    string
	Args    any
	WantErr bool
}

func m5MustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func m5StepJSON(dir string, msg any) string {
	return m5MustJSON(map[string]any{"dir": dir, "msg": msg})
}

func m5ClaudeScript(turns ...[]m5ToolCallSpec) string {
	var steps []string
	rpcID := 2
	for i, calls := range turns {
		steps = append(steps, m5ClaudeTurnStart(i)...)
		callSteps, nextID := m5BuildMCPCallSteps(calls, rpcID)
		rpcID = nextID
		steps = append(steps, callSteps...)
		steps = append(steps, m5StepJSON("stdout", map[string]any{
			"type": "result", "subtype": "success", "is_error": false, "session_id": "$session",
		}))
	}
	return `{"steps":[` + strings.Join(steps, ",") + `]}`
}

func m5ClaudeTurnStart(turnIndex int) []string {
	stdin := m5StepJSON("stdin", map[string]any{"type": "user", "message": map[string]any{"role": "user"}})
	if turnIndex > 0 {
		return []string{stdin, m5StepJSON("stdout", map[string]any{"type": "assistant", "session_id": "$session"})}
	}
	return []string{
		stdin,
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "claude-code", "version": "2.1.311"}}}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{}}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{}}),
		m5StepJSON("stdout", map[string]any{"type": "system", "subtype": "init", "session_id": "$session"}),
	}
}

func m5CodexScript(session string, turns ...[]m5ToolCallSpec) string {
	steps := m5CodexBootSteps()
	rpcID := 2
	for i, calls := range turns {
		turnID := fmt.Sprintf("turn-%d", i+1)
		steps = append(steps,
			m5StepJSON("stdin", map[string]any{"method": "turn/start", "params": map[string]any{"threadId": "$session"}}),
			m5StepJSON("stdout", map[string]any{"id": "$id", "result": map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress"}}}),
			m5StepJSON("stdout", map[string]any{"method": "item/started", "params": map[string]any{"threadId": "$session", "turnId": turnID}}),
		)
		callSteps, nextID := m5BuildMCPCallSteps(calls, rpcID)
		rpcID = nextID
		steps = append(steps, callSteps...)
		steps = append(steps, m5StepJSON("stdout", map[string]any{
			"method": "turn/completed",
			"params": map[string]any{"threadId": "$session", "turn": map[string]any{"id": turnID, "status": "completed"}},
		}))
	}
	return fmt.Sprintf(`{"session":%q,"steps":[%s]}`, session, strings.Join(steps, ","))
}

func m5CodexBootSteps() []string {
	return []string{
		m5StepJSON("stdin", map[string]any{"method": "initialize"}),
		m5StepJSON("stdout", map[string]any{"id": "$id", "result": map[string]any{}}),
		m5StepJSON("stdin", map[string]any{"method": "initialized"}),
		m5StepJSON("stdin", map[string]any{"id": 2}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "codex-mcp-client", "version": "0.164.0"}}}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 0, "result": map[string]any{}}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}),
		m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}),
		m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{}}),
		m5StepJSON("stdout", map[string]any{"id": "$id", "result": map[string]any{"thread": map[string]any{"id": "$session"}}}),
	}
}

func m5BuildMCPCallSteps(calls []m5ToolCallSpec, startID int) ([]string, int) {
	var out []string
	id := startID
	for _, c := range calls {
		args := c.Args
		if args == nil {
			args = map[string]any{}
		}
		out = append(out,
			m5StepJSON("mcp<-cli", map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": c.Name, "arguments": args}}),
			m5StepJSON("mcp->cli", map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"isError": c.WantErr}}),
		)
		id++
	}
	return out, id
}

type m5ToolRecord struct {
	Session host.SessionKey
	Kind    host.Kind
	Tool    string
	IsError bool
	Text    string
}

type m5HookFunc func(caller host.Caller) error

type m5Browser struct {
	mu          sync.Mutex
	conn        *home.Conn
	nextReq     int64
	waiting     map[int64]chan json.RawMessage
	rows        map[string]map[string]json.RawMessage
	onTaskMerge func(taskID string)
}

func newM5Browser(onMerge func(string)) *m5Browser {
	return &m5Browser{
		waiting:     map[int64]chan json.RawMessage{},
		rows:        map[string]map[string]json.RawMessage{},
		onTaskMerge: onMerge,
	}
}

func (b *m5Browser) Done() <-chan struct{} { return nil }

func (b *m5Browser) Send(frame any) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	var hdr struct {
		Re   *int64 `json:"re,omitempty"`
		Type string `json:"t,omitempty"`
	}
	if json.Unmarshal(raw, &hdr) != nil {
		return nil
	}
	if hdr.Re != nil {
		b.deliverReply(*hdr.Re, raw)
		return nil
	}
	if hdr.Type == "rows" {
		b.applyRows(raw)
	}
	return nil
}

func (b *m5Browser) deliverReply(re int64, raw json.RawMessage) {
	b.mu.Lock()
	ch := b.waiting[re]
	delete(b.waiting, re)
	b.mu.Unlock()
	if ch != nil {
		ch <- raw
	}
}

func (b *m5Browser) applyRows(raw json.RawMessage) {
	var batch proto.Rows
	if json.Unmarshal(raw, &batch) != nil {
		return
	}
	var merged []string
	b.mu.Lock()
	for _, elem := range batch.Rows {
		if id := b.storeRowLocked(elem); id != "" {
			merged = append(merged, id)
		}
	}
	b.mu.Unlock()
	for _, id := range merged {
		b.onTaskMerge(id)
	}
}

func (b *m5Browser) storeRowLocked(elem json.RawMessage) string {
	var rh proto.RowHeader
	if json.Unmarshal(elem, &rh) != nil || rh.Type == "" {
		return ""
	}
	if b.rows[rh.Type] == nil {
		b.rows[rh.Type] = map[string]json.RawMessage{}
	}
	prev := b.rows[rh.Type][rh.ID]
	b.rows[rh.Type][rh.ID] = elem
	if rh.Type != "task" {
		return ""
	}
	return m5JustMergedID(prev, elem)
}

func m5WasMerged(prev json.RawMessage) bool {
	var oldT proto.TaskRow
	return len(prev) > 0 && json.Unmarshal(prev, &oldT) == nil && oldT.State == "merged"
}

func m5JustMergedID(prev, cur json.RawMessage) string {
	var newT proto.TaskRow
	if json.Unmarshal(cur, &newT) == nil && newT.State == "merged" && !m5WasMerged(prev) {
		return newT.ID
	}
	return ""
}

func (b *m5Browser) openRequest() (int64, <-chan json.RawMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextReq++
	// Single reader: waitBrowserReply in the calling test step.
	ch := make(chan json.RawMessage, 1)
	b.waiting[b.nextReq] = ch
	return b.nextReq, ch
}

type m5World struct {
	f       fixtures
	cleanup func(func())
	cluster *nodetest.Cluster
	browser *m5Browser
	groupID string
	repoDir string
	bareDir string
	ghDir   string
	binDir  string
	secID   string
	claudeM string
	codexM  string
	seq     atomic.Int64
	scriptN atomic.Int64

	mu                sync.Mutex
	turns             int
	turnsAtReviewPass map[string]int
	turnsAtMerged     map[string]int
	sawTaskSess       map[int]host.SessionKey
	sawTaskDir        map[int]string
	sawRevSess        map[int]host.SessionKey
	sawRevDir         map[int]string
	tools             []m5ToolRecord
	listedTools       map[host.SessionKey][]string
	beforeHooks       map[string][]m5HookFunc
	heldMembers       map[string]bool
	closing           bool
	hookErr           error

	confirmAns1 proto.CmdResult
	confirmAns2 proto.CmdResult
	failAns1    proto.CmdResult
	failAns2    proto.CmdResult
	refuseCode1 string
	refuseCode2 string
	commit1     string
	commit2     string
	unreviewed  string
}

func newM5World(sc *godog.ScenarioContext, f fixtures) *m5World {
	w := &m5World{
		f:                 f,
		turnsAtReviewPass: map[string]int{},
		turnsAtMerged:     map[string]int{},
		sawTaskSess:       map[int]host.SessionKey{},
		sawTaskDir:        map[int]string{},
		sawRevSess:        map[int]host.SessionKey{},
		sawRevDir:         map[int]string{},
		listedTools:       map[host.SessionKey][]string{},
		beforeHooks:       map[string][]m5HookFunc{},
		heldMembers:       map[string]bool{},
	}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		w.settleBeforeAfterHooks()
		return ctx, nil
	})
	w.cleanup = afterScenario(sc)
	w.browser = newM5Browser(w.recordTaskMerged)
	return w
}

func (w *m5World) setEnv(key, val string) {
	old, had := os.LookupEnv(key)
	os.Setenv(key, val)
	w.cleanup(func() { restoreVar(key, old, had) })
}

func (w *m5World) nextID() string {
	n := w.seq.Add(1)
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[8:], uint64(n))
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func (w *m5World) writeScriptFile(vendor, content string) string {
	idx := w.scriptN.Add(1)
	path := filepath.Join(w.cluster.Root, fmt.Sprintf("%s-%d.json", vendor, idx))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		w.recordHookErr(err)
	}
	w.setEnv("SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT", path)
	return path
}

func (w *m5World) recordHookErr(err error) {
	if err == nil {
		return
	}
	w.mu.Lock()
	if w.hookErr == nil {
		w.hookErr = err
	}
	w.mu.Unlock()
}

func (w *m5World) checkHookErr() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hookErr
}

func (w *m5World) addBeforeHook(tool string, fn m5HookFunc) {
	w.mu.Lock()
	w.beforeHooks[tool] = append(w.beforeHooks[tool], fn)
	w.mu.Unlock()
}

func (w *m5World) popBeforeHook(tool string) m5HookFunc {
	w.mu.Lock()
	defer w.mu.Unlock()
	list := w.beforeHooks[tool]
	if len(list) == 0 {
		return nil
	}
	w.beforeHooks[tool] = list[1:]
	return list[0]
}

func (w *m5World) setMemberHeld(member string, held bool) {
	w.mu.Lock()
	if held {
		w.heldMembers[member] = true
	} else {
		delete(w.heldMembers, member)
	}
	w.mu.Unlock()
}

func (w *m5World) recordTaskMerged(taskID string) {
	w.mu.Lock()
	if _, ok := w.turnsAtMerged[taskID]; !ok {
		w.turnsAtMerged[taskID] = w.turns
	}
	w.mu.Unlock()
}

func (w *m5World) setupCluster(forge string, checks []string, level2 bool) error {
	if err := w.prepareHelperBinaries(forge == "github"); err != nil {
		return err
	}
	w.registerHostHooks()
	tb := scenarioTB{TB: w.f.t, cleanup: w.cleanup}
	w.cluster = nodetest.Start(tb, nodetest.Config{BinDir: w.binDir, Nodes: 1})
	if err := w.startMCPBridge(); err != nil {
		return err
	}
	if err := w.initSourceRepo(forge == "github"); err != nil {
		return err
	}
	return w.createGroupAndJoinAIs(forge, checks, level2)
}

func m5ExeExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func (w *m5World) prepareHelperBinaries(withGH bool) error {
	w.binDir = w.f.t.TempDir()
	ext := m5ExeExt()
	for _, cli := range []string{"claude", "codex"} {
		if err := m5CopyExec(filepath.Join(w.f.fakeCLIs, cli+ext), filepath.Join(w.binDir, cli+ext)); err != nil {
			return err
		}
	}
	return w.linkSelfHelpers(ext, withGH)
}

func (w *m5World) linkSelfHelpers(ext string, withGH bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	names := []string{"m5envcheck"}
	if withGH {
		names = append(names, "gh")
	}
	for _, name := range names {
		if err := m5CopyExec(exe, filepath.Join(w.binDir, name+ext)); err != nil {
			return err
		}
	}
	return nil
}

func m5CopyExec(src, dst string) error {
	if os.Link(src, dst) == nil {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o755)
}

func (w *m5World) registerHostHooks() {
	m5CurrentWorld.Store(w)
	w.cleanup(func() { m5CurrentWorld.Store(nil) })
}

func (w *m5World) settleBeforeAfterHooks() {
	if w.cluster == nil {
		return
	}
	w.mu.Lock()
	w.closing = true
	w.mu.Unlock()
	n0 := w.cluster.Nodes[0]
	n0.Link.Close()
	n0.Home.Close()
	m5WaitAllDormant(n0.Host)
	time.Sleep(30 * time.Millisecond)
}

func m5WaitAllDormant(h *host.Host) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m5HasNonDormant(h) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func m5HasNonDormant(h *host.Host) bool {
	return slices.ContainsFunc(h.Sessions.All(), func(s host.Session) bool {
		return s.State != host.Dormant
	})
}

func (w *m5World) startMCPBridge() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cleanup(func() {
		cancel()
		ln.Close()
	})
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	n0 := w.cluster.Nodes[0]
	n0.Host.Shim = []string{exe, "m5-mcp-shim", ln.Addr().String()}
	go w.acceptMCPLoop(ctx, ln, n0.Host)
	return nil
}

func (w *m5World) acceptMCPLoop(ctx context.Context, ln net.Listener, h *host.Host) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go w.serveOneMCPConn(ctx, conn, h)
	}
}

func (w *m5World) serveOneMCPConn(ctx context.Context, conn net.Conn, h *host.Host) {
	open, _, reader, err := node.AcceptOpening(conn)
	if err != nil {
		return
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	var reqMu sync.Mutex
	methods := map[string]string{}
	tools := map[string]string{}
	go w.pumpMCPRequests(h, open.MCP, reader, inW, &reqMu, methods, tools)
	go w.pumpMCPResponses(h, open.MCP, outR, conn, &reqMu, methods, tools)
	h.ServeMCP(ctx, open.MCP, inR, outW)
}

func (w *m5World) pumpMCPRequests(h *host.Host, token string, r *bufio.Reader, dst *io.PipeWriter, mu *sync.Mutex, methods, tools map[string]string) {
	defer dst.Close()
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			w.inspectMCPRequest(h, token, bytes.TrimSpace(line), mu, methods, tools)
			if _, werr := dst.Write(line); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (w *m5World) inspectMCPRequest(h *host.Host, token string, line []byte, mu *sync.Mutex, methods, tools map[string]string) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal(line, &req) != nil || len(req.ID) == 0 {
		return
	}
	idKey := string(req.ID)
	mu.Lock()
	methods[idKey] = req.Method
	tools[idKey] = req.Params.Name
	mu.Unlock()
	caller, ok := h.Sessions.Caller(token)
	if ok && req.Method == "tools/call" {
		w.onBeforeToolCall(h, caller, req.Params.Name)
	}
}

func (w *m5World) onBeforeToolCall(h *host.Host, caller host.Caller, toolName string) {
	w.noteSessionWorkspace(h, caller)
	if fn := w.popBeforeHook(toolName); fn != nil {
		w.recordHookErr(fn(caller))
	}
}

func (w *m5World) noteSessionWorkspace(h *host.Host, caller host.Caller) {
	if caller.Task == "" {
		return
	}
	raw, ok := h.Rows.Get(caller.Group, "task", caller.Task)
	if !ok {
		return
	}
	var t proto.TaskRow
	if json.Unmarshal(raw, &t) != nil {
		return
	}
	dir := filepath.Join(w.cluster.Nodes[0].Dir, "projects", caller.Group, caller.Folder)
	w.recordObservedSession(caller, t.Num, dir)
}

func (w *m5World) recordObservedSession(caller host.Caller, num int, dir string) {
	if _, err := os.Stat(dir); err != nil {
		w.recordHookErr(err)
		return
	}
	w.mu.Lock()
	if caller.Kind == host.TaskSession {
		w.sawTaskSess[num] = caller.SessionKey
		w.sawTaskDir[num] = dir
	}
	if caller.Kind == host.ReviewSession {
		w.sawRevSess[num] = caller.SessionKey
		w.sawRevDir[num] = dir
	}
	w.mu.Unlock()
}

func (w *m5World) pumpMCPResponses(h *host.Host, token string, src *io.PipeReader, dst net.Conn, mu *sync.Mutex, methods, tools map[string]string) {
	defer dst.Close()
	r := bufio.NewReader(src)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			w.inspectMCPResponse(h, token, bytes.TrimSpace(line), mu, methods, tools)
			if _, werr := dst.Write(line); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (w *m5World) inspectMCPResponse(h *host.Host, token string, line []byte, mu *sync.Mutex, methods, tools map[string]string) {
	var resp struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(line, &resp) != nil || len(resp.ID) == 0 {
		return
	}
	idKey := string(resp.ID)
	mu.Lock()
	method, tool := methods[idKey], tools[idKey]
	delete(methods, idKey)
	delete(tools, idKey)
	mu.Unlock()
	caller, ok := h.Sessions.Caller(token)
	if !ok {
		return
	}
	if method == "tools/list" {
		w.recordListedTools(caller.SessionKey, resp.Result)
	}
	if method == "tools/call" {
		w.onAfterToolCall(h, caller, tool, resp.Result)
	}
}

func (w *m5World) recordListedTools(key host.SessionKey, raw json.RawMessage) {
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	names := make([]string, len(list.Tools))
	for i, item := range list.Tools {
		names[i] = item.Name
	}
	w.mu.Lock()
	w.listedTools[key] = names
	w.mu.Unlock()
}

func m5ParseToolCallResult(raw json.RawMessage) (string, bool) {
	var callRes struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(raw, &callRes) != nil {
		return "", false
	}
	if len(callRes.Content) > 0 {
		return callRes.Content[0].Text, callRes.IsError
	}
	return "", callRes.IsError
}

func (w *m5World) onAfterToolCall(h *host.Host, caller host.Caller, tool string, raw json.RawMessage) {
	text, isErr := m5ParseToolCallResult(raw)
	if !isErr {
		w.syncAfterSuccessTool(h, caller, tool, text)
	}
	w.mu.Lock()
	w.tools = append(w.tools, m5ToolRecord{Session: caller.SessionKey, Kind: caller.Kind, Tool: tool, IsError: isErr, Text: text})
	w.mu.Unlock()
}

func (w *m5World) syncAfterSuccessTool(h *host.Host, caller host.Caller, tool, text string) {
	if tool == "run_checks" {
		w.waitHostLocalChecks(h, caller, text)
	}
	if tool == "submit_review" && strings.Contains(text, `"passed":true`) {
		w.mu.Lock()
		w.turnsAtReviewPass[caller.Task] = w.turns
		w.mu.Unlock()
	}
}

func (w *m5World) waitHostLocalChecks(h *host.Host, caller host.Caller, text string) {
	var out struct {
		Commit string `json:"commit"`
	}
	if json.Unmarshal([]byte(text), &out) != nil || out.Commit == "" {
		return
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m5TaskHasLocalCheck(h, caller.Group, caller.Task, out.Commit) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func m5TaskHasLocalCheck(h *host.Host, group, taskID, commit string) bool {
	raw, ok := h.Rows.Get(group, "task", taskID)
	if !ok {
		return false
	}
	var t proto.TaskRow
	return json.Unmarshal(raw, &t) == nil && t.Checks.Local != nil && t.Checks.Local.Commit == commit
}

func (w *m5World) initSourceRepo(withRemote bool) error {
	work := w.f.t.TempDir()
	if _, err := m5GitOut("", "init", "-b", "main", work); err != nil {
		return err
	}
	files := map[string]string{
		"shared.txt":   "initial\n",
		"init_test.go": "package proj\n",
	}
	if _, err := m5GitCommitFiles(work, files, "Initial commit"); err != nil {
		return err
	}
	if !withRemote {
		w.repoDir = work
		return nil
	}
	return w.initBareRemote(work)
}

func (w *m5World) initBareRemote(seedWork string) error {
	parent := w.f.t.TempDir()
	bare := filepath.Join(parent, "acme", "proj.git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o700); err != nil {
		return err
	}
	if _, err := m5GitOut("", "clone", "--bare", seedWork, bare); err != nil {
		return err
	}
	w.bareDir, w.repoDir = bare, bare
	w.ghDir = w.f.t.TempDir()
	if err := m5SaveGHState(w.ghDir, m5GHState{BareRepo: bare}); err != nil {
		return err
	}
	w.setEnv("SOLODAWN_M5_GH_DIR", w.ghDir)
	return nil
}

func (w *m5World) createGroupAndJoinAIs(forge string, checks []string, level2 bool) error {
	n0 := w.cluster.Nodes[0]
	w.browser.conn = n0.Home.Attach(w.cluster.Owner.ID(), w.browser)
	w.cleanup(func() { n0.Home.Detach(w.browser.conn) })
	w.groupID = w.nextID()
	createPayload := map[string]any{
		"t": "group", "op": "create", "n": w.groupID, "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "name": "Proj", "kind": "project", "repo": w.repoDir,
		"base": "main", "forge": forge, "checks": checks, "member_name": "Owner",
	}
	if _, err := w.ownerCmd(createPayload); err != nil {
		return err
	}
	if err := w.syncBrowser(); err != nil {
		return err
	}
	return w.joinGroupMembers(level2)
}

func (w *m5World) declareAndJoinWorkingAIs() error {
	n0 := w.cluster.Nodes[0]
	if _, err := n0.Host.Agents(context.Background()); err != nil {
		return err
	}
	specs := []struct{ agent, name string }{{"claude", "Claude"}, {"codex", "Codex"}}
	for _, sp := range specs {
		if err := w.callHostSurface("agent.set", map[string]any{"agent": sp.agent, "account": "personal"}); err != nil {
			return err
		}
		if err := w.callHostSurface("host.join", map[string]any{"g": w.groupID, "home": n0.Key.ID(), "agent": sp.agent, "name": sp.name, "secretary": false}); err != nil {
			return err
		}
	}
	return nil
}

func (w *m5World) joinGroupMembers(level2 bool) error {
	if err := w.declareAndJoinWorkingAIs(); err != nil {
		return err
	}
	if err := w.addSecretaryMember(); err != nil {
		return err
	}
	if err := w.configureLevels(level2); err != nil {
		return err
	}
	return w.waitMembersSynced()
}

func (w *m5World) callHostSurface(name string, fields map[string]any) error {
	n0 := w.cluster.Nodes[0]
	fields["t"] = name
	fields["n"] = w.nextID()
	fields["ts"] = n0.Host.Now().UnixMilli()
	fields["to"] = n0.Key.ID()
	raw := []byte(m5MustJSON(fields))
	_, err := host.Surface[name](context.Background(), n0.Host, n0.Link, w.cluster.Owner.ID(), raw)
	return err
}

func (w *m5World) addSecretaryMember() error {
	n0 := w.cluster.Nodes[0]
	w.secID = w.nextID()
	cmd := map[string]any{
		"t": "member", "op": "add_ai", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": w.secID, "cli": "claude", "name": "Secretary",
		"account": "personal", "secretary": true, "approvers": []string{w.cluster.Owner.ID()},
	}
	_, err := n0.Host.Command(context.Background(), n0.Key.ID(), m5MustJSON(cmd))
	return err
}

func (w *m5World) configureLevels(level2 bool) error {
	if !level2 {
		return nil
	}
	claudeLvl := 2
	if runtime.GOOS == "windows" {
		claudeLvl = 3
	}
	write := func(tx *store.Tx) error {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO perm(agent, g, level, repo_config) VALUES('codex', ?, 2, 0)`, w.groupID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO perm(agent, g, level, repo_config) VALUES('claude', ?, ?, 0)`, w.groupID, claudeLvl)
		return err
	}
	return w.cluster.Nodes[0].Store.Write(write)
}

func (w *m5World) waitMembersSynced() error {
	n0 := w.cluster.Nodes[0]
	check := func() bool {
		w.claudeM = w.findMemberByName("Claude")
		w.codexM = w.findMemberByName("Codex")
		_, hasSec := n0.Host.Rows.Get(w.groupID, "member", w.secID)
		_, hasC := n0.Host.Rows.Get(w.groupID, "member", w.claudeM)
		_, hasX := n0.Host.Rows.Get(w.groupID, "member", w.codexM)
		return w.claudeM != "" && w.codexM != "" && hasSec && hasC && hasX
	}
	return w.waitFor("group members synced to host", check)
}

func (w *m5World) findMemberByName(name string) string {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	for id, raw := range w.browser.rows["member"] {
		var m proto.MemberRow
		if json.Unmarshal(raw, &m) == nil && m.Name == name {
			return id
		}
	}
	return ""
}

func (w *m5World) ownerCmd(payload any) (proto.CmdResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return proto.CmdResult{}, err
	}
	return w.sendSignedCmd(w.cluster.Owner, string(raw))
}

func (w *m5World) sendSignedCmd(signer identity.Key, payload string) (proto.CmdResult, error) {
	return w.sendEnvelopeCmd(signer.Sign(payload))
}

func (w *m5World) sendEnvelopeCmd(env proto.Envelope) (proto.CmdResult, error) {
	reqID, ch := w.browser.openRequest()
	w.cluster.Nodes[0].Home.Cmd(w.browser.conn, reqID, env)
	return w.waitCmdReply(ch)
}

func (w *m5World) waitCmdReply(ch <-chan json.RawMessage) (proto.CmdResult, error) {
	select {
	case raw := <-ch:
		var errResp proto.ErrorResponse
		if proto.Decode(raw, &errResp) == nil {
			return proto.CmdResult{}, &host.Error{Code: errResp.Err.Code, Message: errResp.Err.Message}
		}
		var res proto.CmdResult
		return res, proto.Decode(raw, &res)
	case <-time.After(10 * time.Second):
		return proto.CmdResult{}, errors.New("timed out waiting for home command reply")
	}
}

func (w *m5World) syncBrowser() error {
	reqID, ch := w.browser.openRequest()
	w.cluster.Nodes[0].Home.Sync(w.browser.conn, proto.Sync{
		Request: proto.Request{Type: "sync", ID: reqID},
		Group:   w.groupID,
	})
	select {
	case raw := <-ch:
		var synced proto.Synced
		return proto.Decode(raw, &synced)
	case <-time.After(10 * time.Second):
		return errors.New("timed out waiting for browser sync")
	}
}

func (w *m5World) waitFor(desc string, check func() bool) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := w.checkHookErr(); err != nil {
			return err
		}
		if check() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", desc)
}

func (w *m5World) ownerPostMain(text string, mentions []string) error {
	n0 := w.cluster.Nodes[0]
	m := mentions
	if m == nil {
		m = []string{}
	}
	_, err := w.ownerCmd(map[string]any{
		"t": "post", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(), "g": w.groupID,
		"kind": "text", "text": text, "thread": nil, "main": true,
		"mentions": m, "re": nil, "body": map[string]any{},
	})
	return err
}

func (w *m5World) ownerConfirmBrief(briefID string) error {
	n0 := w.cluster.Nodes[0]
	_, err := w.ownerCmd(map[string]any{
		"t": "act", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "msg": briefID, "action": "confirm",
	})
	return err
}

func (w *m5World) pendingBriefID() string {
	for _, m := range w.browserMessages() {
		if m.Kind == "brief" && m.State != nil && m.State.Status == "pending" {
			return m.ID
		}
	}
	return ""
}

func (w *m5World) browserMessages() []proto.MsgRow {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	var list []proto.MsgRow
	for _, raw := range w.browser.rows["msg"] {
		var m proto.MsgRow
		if json.Unmarshal(raw, &m) == nil {
			list = append(list, m)
		}
	}
	slices.SortFunc(list, func(a, b proto.MsgRow) int { return int(a.Pos - b.Pos) })
	return list
}

func (w *m5World) browserTasks() []proto.TaskRow {
	w.browser.mu.Lock()
	defer w.browser.mu.Unlock()
	var list []proto.TaskRow
	for _, raw := range w.browser.rows["task"] {
		var t proto.TaskRow
		if json.Unmarshal(raw, &t) == nil {
			list = append(list, t)
		}
	}
	slices.SortFunc(list, func(a, b proto.TaskRow) int { return a.Num - b.Num })
	return list
}

func (w *m5World) taskByNum(num int) (proto.TaskRow, bool) {
	for _, t := range w.browserTasks() {
		if t.Num == num {
			return t, true
		}
	}
	return proto.TaskRow{}, false
}

func (w *m5World) homeQueuedItems() ([]store.Item, error) {
	var items []store.Item
	read := func(tx *store.Tx) error {
		var err error
		items, err = tx.Queued(w.groupID)
		return err
	}
	err := w.cluster.Nodes[0].Store.Write(read)
	return items, err
}

func (w *m5World) hostAuditCount(what, detailSub string) (int, error) {
	var count int
	read := func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM audit WHERE g = ? AND what = ? AND instr(detail, ?) > 0`, w.groupID, what, detailSub).Scan(&count)
	}
	err := w.cluster.Nodes[0].Store.Write(read)
	return count, err
}

func (w *m5World) projectDir() string {
	return filepath.Join(w.cluster.Nodes[0].Dir, "projects", w.groupID)
}

func (w *m5World) sessionFolder(c host.Caller) string {
	return filepath.Join(w.projectDir(), c.Folder)
}

func m5BriefArgs(tasks []map[string]any) map[string]any {
	return map[string]any{
		"title":     "Build helpers",
		"goal":      "Add helper functions with tests",
		"criteria":  []string{"All checks pass"},
		"scope_in":  []string{"helpers"},
		"scope_out": []string{"cli"},
		"tasks":     tasks,
	}
}

func m5TwoTaskBriefArgs() map[string]any {
	return m5BriefArgs([]map[string]any{
		{"title": "Add alpha helper", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
		{"title": "Add beta helper", "assignee": "Codex", "reviewer": "Claude", "depends": []int{0}},
	})
}

func m5OneTaskBriefArgs() map[string]any {
	return m5BriefArgs([]map[string]any{
		{"title": "Add gamma helper", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
	})
}

func m5Steps(sc *godog.ScenarioContext, f fixtures) {
	w := newM5World(sc, f)
	m5RegisterFullFlowSteps(sc, w)
	m5RegisterCheckAndReviewSteps(sc, w)
	m5RegisterDepAndDirectSteps(sc, w)
	m5RegisterConflictAndEdgeSteps(sc, w)
}

func m5RegisterFullFlowSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a project group with forge "none", a secretary and working AIs "Claude" and "Codex" at level 2$`, func() error {
		return w.setupCluster("none", []string{}, true)
	})
	sc.Step(`^a project group with forge "github", a bare remote, fake gh, a secretary and working AIs "Claude" and "Codex" at level 2$`, func() error {
		return w.setupCluster("github", []string{}, true)
	})
	sc.Step(`^the owner posts a feature request in the main line and the secretary proposes a two-task brief$`, w.stepRequestAndTwoTaskBrief)
	sc.Step(`^before confirm no task worktree exists and no assign item is queued on the home$`, w.stepVerifyBeforeConfirm)
	sc.Step(`^the owner confirms the brief and both tasks run through checks, cross-AI review, merge and summary$`, func() error {
		return w.stepConfirmAndRunTwoTasks(false)
	})
	sc.Step(`^the owner confirms the brief and both tasks push their branches, pass local and hosted checks, review each other, merge and summarize$`, func() error {
		return w.stepConfirmAndRunTwoTasks(true)
	})
	sc.Step(`^both tasks are merged onto base with two summary cards in the main line and no approval card$`, func() error {
		return w.stepVerifyTwoTasksMerged(false)
	})
	sc.Step(`^push_branch pushed the assigned branch to origin at level 2 with no approval card$`, w.stepVerifyPushBranchAtLevel2)
	sc.Step(`^both tasks are merged on origin with two summary cards carrying pull-request numbers and no approval card$`, func() error {
		return w.stepVerifyTwoTasksMerged(true)
	})
	sc.Step(`^each passing review was followed by its merge with no AI turn in between and a task_merged row in the thread$`, w.stepVerifyNoTurnBeforeMerge)
	sc.Step(`^each task ran in its own session and worktree, and after the merge those sessions and folders are gone$`, w.stepVerifySessionsAndFoldersGone)
}

func (w *m5World) stepRequestAndTwoTaskBrief() error {
	secScript := m5ClaudeScript(
		[]m5ToolCallSpec{{Name: "propose_brief", Args: m5TwoTaskBriefArgs()}},
		nil,
		[]m5ToolCallSpec{{Name: "post", Args: map[string]any{"kind": "summary", "task": "T-1", "text": "Alpha merged."}}},
		[]m5ToolCallSpec{{Name: "post", Args: map[string]any{"kind": "summary", "task": "T-2", "text": "Beta merged."}}},
	)
	w.writeScriptFile("claude", secScript)
	if err := w.ownerPostMain("Please add alpha and beta helpers.", nil); err != nil {
		return err
	}
	return w.waitFor("pending brief card", func() bool { return w.pendingBriefID() != "" })
}

func (w *m5World) stepVerifyBeforeConfirm() error {
	if _, err := os.Stat(filepath.Join(w.projectDir(), "wt")); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("expected no wt directory before confirm, got err=%v", err)
	}
	items, err := w.homeQueuedItems()
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.Why == "assign" {
			return fmt.Errorf("unexpected assign item %+v before confirm", it)
		}
	}
	return nil
}

func (w *m5World) stepConfirmAndRunTwoTasks(withPush bool) error {
	t1Calls := m5WorkerCalls(withPush, "Alpha ready.")
	t2Calls := m5WorkerCalls(withPush, "Beta ready.")
	w.writeScriptFile("claude", m5ClaudeScript(t1Calls))
	w.writeScriptFile("codex", m5CodexScript("codex-rev-1", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	commitHook1 := "run_checks"
	if withPush {
		commitHook1 = "push_branch"
	}
	w.addBeforeHook(commitHook1, func(c host.Caller) error {
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"alpha.go": "package proj\n", "alpha_test.go": "package proj\n"}, "Add alpha")
		return err
	})
	w.addBeforeHook("submit_review", func(_ host.Caller) error {
		w.writeScriptFile("codex", m5CodexScript("codex-task-2", t2Calls))
		w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{
			Name: "submit_review", Args: map[string]any{"score": 96, "items": []any{}},
		}}))
		return nil
	})
	w.addBeforeHook(commitHook1, func(c host.Caller) error {
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"beta.go": "package proj\n", "beta_test.go": "package proj\n"}, "Add beta")
		return err
	})
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	return w.waitFor("two summary cards", func() bool { return len(w.summaryCards()) == 2 })
}

func m5WorkerCalls(withPush bool, summary string) []m5ToolCallSpec {
	var calls []m5ToolCallSpec
	if withPush {
		calls = append(calls, m5ToolCallSpec{Name: "push_branch"})
	}
	return append(calls,
		m5ToolCallSpec{Name: "run_checks"},
		m5ToolCallSpec{Name: "submit_task", Args: map[string]any{"summary": summary}},
	)
}

func (w *m5World) summaryCards() []proto.MsgRow {
	var out []proto.MsgRow
	for _, m := range w.browserMessages() {
		if m.Kind == "summary" && m.Main {
			out = append(out, m)
		}
	}
	return out
}

func (w *m5World) stepVerifyPushBranchAtLevel2() error {
	w.mu.Lock()
	var pushes int
	for _, rec := range w.tools {
		if rec.Tool == "push_branch" && !rec.IsError && strings.Contains(rec.Text, `"ok":true`) {
			pushes++
		}
	}
	w.mu.Unlock()
	if pushes != 2 {
		return fmt.Errorf("expected 2 successful push_branch calls, got %d", pushes)
	}
	return w.verifyZeroApprovalCards()
}

func (w *m5World) verifyZeroApprovalCards() error {
	for _, m := range w.browserMessages() {
		if m.Kind == "approval" {
			return fmt.Errorf("unexpected approval card %s", m.ID)
		}
	}
	return nil
}

func (w *m5World) stepVerifyTwoTasksMerged(remote bool) error {
	if err := w.verifyZeroApprovalCards(); err != nil {
		return err
	}
	tasks := w.browserTasks()
	if len(tasks) != 2 || tasks[0].State != "merged" || tasks[1].State != "merged" {
		return fmt.Errorf("expected 2 merged tasks, got %+v", tasks)
	}
	if err := w.verifySummaries(remote); err != nil {
		return err
	}
	return w.verifyBaseHasFiles(remote, "alpha.go", "beta.go")
}

func (w *m5World) validSummaryCard(c proto.MsgRow, remote bool) bool {
	var body proto.SummaryBody
	if json.Unmarshal(c.Body, &body) != nil || c.By != w.secID {
		return false
	}
	return !remote || body.PR != nil
}

func (w *m5World) verifySummaries(remote bool) error {
	cards := w.summaryCards()
	if len(cards) != 2 {
		return fmt.Errorf("expected 2 summary cards, got %d", len(cards))
	}
	for _, c := range cards {
		if !w.validSummaryCard(c, remote) {
			return fmt.Errorf("unexpected summary card by=%q body=%s", c.By, string(c.Body))
		}
	}
	return nil
}

func (w *m5World) verifyBaseHasFiles(remote bool, names ...string) error {
	target := filepath.Join(w.projectDir(), "repo")
	if remote {
		target = w.bareDir
	}
	tree, err := m5GitOut(target, "ls-tree", "--name-only", "refs/heads/main")
	if err != nil {
		return err
	}
	present := strings.Split(tree, "\n")
	for _, name := range names {
		if !slices.Contains(present, name) {
			return fmt.Errorf("missing %q on main branch (%v)", name, present)
		}
	}
	return nil
}

func (w *m5World) stepVerifyNoTurnBeforeMerge() error {
	for _, t := range w.browserTasks() {
		if err := w.verifyTaskMergedOrder(t.ID); err != nil {
			return err
		}
	}
	return nil
}

func (w *m5World) verifyTaskMergedOrder(taskID string) error {
	w.mu.Lock()
	atPass, ok1 := w.turnsAtReviewPass[taskID]
	atMerge, ok2 := w.turnsAtMerged[taskID]
	w.mu.Unlock()
	if !ok1 || !ok2 || atPass != atMerge {
		return fmt.Errorf("task %s turns at review pass=%d (ok=%v), at merge=%d (ok=%v)", taskID, atPass, ok1, atMerge, ok2)
	}
	if !w.hasThreadTaskMergedMsg(taskID) {
		return fmt.Errorf("task %s missing system task_merged message in thread", taskID)
	}
	return nil
}

func m5IsTaskMergedMsg(m proto.MsgRow, taskID string) bool {
	if m.Thread == nil || *m.Thread != taskID || m.Kind != "system" {
		return false
	}
	return m.By == "system" && strings.Contains(string(m.Body), `"task_merged"`)
}

func (w *m5World) hasThreadTaskMergedMsg(taskID string) bool {
	for _, m := range w.browserMessages() {
		if m5IsTaskMergedMsg(m, taskID) {
			return true
		}
	}
	return false
}

func m5DistinctPair[T comparable](a, b, zero T) bool {
	return a != zero && b != zero && a != b
}

func (w *m5World) stepVerifySessionsAndFoldersGone() error {
	w.mu.Lock()
	s1, s2 := w.sawTaskSess[1], w.sawTaskSess[2]
	d1, d2 := w.sawTaskDir[1], w.sawTaskDir[2]
	r1, r2 := w.sawRevSess[1], w.sawRevSess[2]
	rd1, rd2 := w.sawRevDir[1], w.sawRevDir[2]
	w.mu.Unlock()
	if !m5DistinctPair(s1, s2, host.SessionKey{}) || !m5DistinctPair(d1, d2, "") {
		return fmt.Errorf("tasks did not run in distinct sessions and worktrees: %+v %+v %q %q", s1, s2, d1, d2)
	}
	return w.waitCleanupComplete([]host.SessionKey{s1, s2, r1, r2}, []string{d1, d2, rd1, rd2})
}

func (w *m5World) waitCleanupComplete(keys []host.SessionKey, dirs []string) error {
	h := w.cluster.Nodes[0].Host
	check := func() bool {
		for _, k := range keys {
			if _, ok := h.Sessions.Get(k); ok {
				return false
			}
		}
		for _, d := range dirs {
			if _, err := os.Stat(d); !errors.Is(err, fs.ErrNotExist) {
				return false
			}
		}
		return true
	}
	return w.waitFor("task and review sessions and folders removed after merge", check)
}

func m5RegisterCheckAndReviewSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^an active brief task on forge "none" assigned to "Claude" and reviewed by "Codex"$`, func() error {
		return w.givenOneBriefTask("none", []string{"git grep -q PASS_MARKER check.txt"}, false)
	})
	sc.Step(`^"Claude" runs checks on a dirty worktree, submits after failing checks, submits after passing checks, and submits again after an unchecked commit$`, w.stepRunA53TaskSequence)
	sc.Step(`^run_checks refused the dirty worktree, submit_task failed after failing checks, succeeded after passing checks, and failed after the extra commit$`, w.stepVerifyA53ToolResults)
	sc.Step(`^the review passes while merge is held and a new commit advances the task branch before merge runs$`, w.stepReviewAndMoveBranchBeforeMerge)
	sc.Step(`^the unreviewed commit is not merged onto base and the task returns to local_check waiting on conflict$`, w.stepVerifyMovedBranchConflict)

	sc.Step(`^a submitted brief task on forge "none" assigned to "Claude" and reviewed by "Codex"$`, func() error {
		return w.givenSubmittedBriefTask(false)
	})
	sc.Step(`^the assignee and a wrong-commit checkout attempt reviews on the submitted task$`, w.stepAttemptInvalidReviews)
	sc.Step(`^the assignee review is refused with "forbidden" and the wrong-commit review is refused with "conflict"$`, w.stepVerifyInvalidReviews)
	sc.Step(`^"Codex" reviews with score 86, then with a veto at score 95, and then four more failing reviews on resubmissions$`, w.stepRunSixFailedReviews)
	sc.Step(`^the 86 review returned the task to local_check, the veto recorded score 0, and the sixth failed review made the task failed$`, w.stepVerifySixFailedReviews)

	sc.Step(`^"Claude" commits a test secret block with a test file and calls run_checks$`, w.stepCommitSecretAndRunChecks)
	sc.Step(`^run_checks completes with ok false and a failing "secrets" hard-check item$`, w.stepVerifySecretHardCheck)

	sc.Step(`^a project group with check "m5envcheck" and a test token variable set in the node environment$`, w.stepSetupEnvCheckGroup)
	sc.Step(`^an active task on that group commits a change and calls run_checks$`, w.stepRunEnvCheckTask)
	sc.Step(`^run_checks passes with "m5envcheck" seeing PATH and HOME and not seeing the test token variable$`, w.stepVerifyEnvCheckPassed)
}

func (w *m5World) waitPendingBrief() error {
	hasBrief := func() bool { return w.pendingBriefID() != "" }
	return w.waitFor("pending brief", hasBrief)
}

func (w *m5World) givenOneBriefTask(forge string, checks []string, holdCodex bool) error {
	if err := w.setupCluster(forge, checks, true); err != nil {
		return err
	}
	if holdCodex {
		w.setMemberHeld(w.codexM, true)
	}
	w.writeScriptFile("claude", m5ClaudeScript(
		[]m5ToolCallSpec{{Name: "propose_brief", Args: m5OneTaskBriefArgs()}},
		nil,
	))
	if err := w.ownerPostMain("Add gamma helper.", nil); err != nil {
		return err
	}
	if err := w.waitPendingBrief(); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, true)
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	err := w.waitFor("active task 1 and dormant secretary", w.task1AndSecReady)
	w.setMemberHeld(w.secID, true)
	w.resetTools()
	return err
}

func (w *m5World) task1AndSecReady() bool {
	t, ok := w.taskByNum(1)
	state, _ := w.cluster.Nodes[0].Host.MemberState(w.groupID, w.secID)
	return ok && t.State == "active" && state == string(host.Dormant)
}

func (w *m5World) resetTools() {
	w.mu.Lock()
	w.tools = nil
	w.mu.Unlock()
}

func (w *m5World) stepRunA53TaskSequence() error {
	calls := []m5ToolCallSpec{
		{Name: "run_checks", WantErr: true},
		{Name: "run_checks", WantErr: false},
		{Name: "submit_task", Args: map[string]any{"summary": "After fail."}, WantErr: true},
		{Name: "run_checks", WantErr: false},
		{Name: "submit_task", Args: map[string]any{"summary": "Passing submit."}, WantErr: false},
		{Name: "submit_task", Args: map[string]any{"summary": "Extra commit."}, WantErr: true},
	}
	w.writeScriptFile("claude", m5ClaudeScript(calls))
	w.setMemberHeld(w.codexM, true)
	w.registerA53Hooks()
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return w.waitFor("all 6 A5.3 tool calls", func() bool { return w.toolCallCount() >= 6 })
}

func (w *m5World) registerA53Hooks() {
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		return os.WriteFile(filepath.Join(w.sessionFolder(c), "dirty.txt"), []byte("dirty\n"), 0o600)
	})
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		if err := os.Remove(filepath.Join(w.sessionFolder(c), "dirty.txt")); err != nil {
			return err
		}
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"check.txt": "FAIL_MARKER\n", "gamma_test.go": "package proj\n"}, "Failing check")
		return err
	})
	w.addBeforeHook("submit_task", func(_ host.Caller) error { return nil })
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		sha, err := m5GitCommitFile(w.sessionFolder(c), "check.txt", "PASS_MARKER\n", "Passing check")
		w.setCommit1(sha)
		return err
	})
	w.addBeforeHook("submit_task", func(_ host.Caller) error { return nil })
	w.addBeforeHook("submit_task", func(c host.Caller) error {
		sha, err := m5GitCommitFile(w.sessionFolder(c), "extra.txt", "extra\n", "Unchecked commit")
		w.mu.Lock()
		w.unreviewed = sha
		w.mu.Unlock()
		return err
	})
}

func (w *m5World) setCommit1(sha string) {
	w.mu.Lock()
	w.commit1 = sha
	w.mu.Unlock()
}

func (w *m5World) getCommit1() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.commit1
}

func (w *m5World) setCommit2(sha string) {
	w.mu.Lock()
	w.commit2 = sha
	w.mu.Unlock()
}

func (w *m5World) getCommit2() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.commit2
}

func (w *m5World) toolCallCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.tools)
}

func (w *m5World) stepVerifyA53ToolResults() error {
	w.mu.Lock()
	recs := slices.Clone(w.tools)
	w.mu.Unlock()
	wantErr := []bool{true, false, true, false, false, true}
	if len(recs) < len(wantErr) {
		return fmt.Errorf("expected %d tool records, got %d", len(wantErr), len(recs))
	}
	for i, want := range wantErr {
		if recs[i].IsError != want {
			return fmt.Errorf("tool[%d] %s isError=%v (%q), want %v", i, recs[i].Tool, recs[i].IsError, recs[i].Text, want)
		}
	}
	return nil
}

func (w *m5World) waitStepMerge() error {
	atMerge := func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "merge"
	}
	return w.waitFor("task at step merge", atMerge)
}

func (w *m5World) stepReviewAndMoveBranchBeforeMerge() error {
	wtDir := filepath.Join(w.projectDir(), "wt", "1-add-gamma-helper")
	if _, err := m5GitOut(wtDir, "reset", "--hard", w.getCommit1()); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, true)
	w.writeScriptFile("codex", m5CodexScript("codex-a53-rev", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	w.setMemberHeld(w.codexM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.codexM)
	if err := w.waitStepMerge(); err != nil {
		return err
	}
	if _, err := m5GitCommitFile(wtDir, "late.txt", "after-review\n", "Moved branch after review"); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	atConflict := func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "local_check" && t.Wait != nil && t.Wait.Kind == "conflict"
	}
	return w.waitFor("task conflict reported", atConflict)
}

func (w *m5World) stepVerifyMovedBranchConflict() error {
	tree, err := m5GitOut(filepath.Join(w.projectDir(), "repo"), "ls-tree", "--name-only", "refs/heads/main")
	if err != nil {
		return err
	}
	if strings.Contains(tree, "late.txt") || strings.Contains(tree, "check.txt") {
		return fmt.Errorf("expected unreviewed commits absent from main, got %q", tree)
	}
	return nil
}

func (w *m5World) givenSubmittedBriefTask(conflictingBase bool) error {
	if err := w.givenOneBriefTask("none", []string{}, true); err != nil {
		return err
	}
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "run_checks"},
		{Name: "submit_task", Args: map[string]any{"summary": "Initial submit."}},
	}))
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		sha, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"shared.txt": "from-task\n", "gamma_test.go": "package proj\n"}, "Task change")
		w.setCommit1(sha)
		return err
	})
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	atReview := func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "review" && t.Commit != ""
	}
	if err := w.waitFor("task 1 at step review", atReview); err != nil {
		return err
	}
	if !conflictingBase {
		return nil
	}
	_, err := m5GitCommitFile(filepath.Join(w.projectDir(), "repo"), "shared.txt", "from-base\n", "Conflicting base commit")
	return err
}

func (w *m5World) stepAttemptInvalidReviews() error {
	n0 := w.cluster.Nodes[0]
	t, _ := w.taskByNum(1)
	assigneeRev := map[string]any{
		"t": "post", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(), "g": w.groupID, "as": w.claudeM,
		"kind": "review", "text": "", "thread": t.ID, "main": false, "mentions": []string{}, "re": nil,
		"body": map[string]any{"task": t.ID, "commit": t.Commit, "score": 95, "items": []any{}},
	}
	_, err1 := n0.Host.Command(context.Background(), n0.Key.ID(), m5MustJSON(assigneeRev))
	w.refuseCode1 = m5ErrCode(err1)

	wrongRev := map[string]any{
		"t": "post", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(), "g": w.groupID, "as": w.codexM,
		"kind": "review", "text": "", "thread": t.ID, "main": false, "mentions": []string{}, "re": nil,
		"body": map[string]any{"task": t.ID, "commit": "0000000000000000000000000000000000000000", "score": 95, "items": []any{}},
	}
	_, err2 := n0.Host.Command(context.Background(), n0.Key.ID(), m5MustJSON(wrongRev))
	w.refuseCode2 = m5ErrCode(err2)
	return nil
}

func m5ErrCode(err error) string {
	var he *host.Error
	if errors.As(err, &he) {
		return he.Code
	}
	return ""
}

func (w *m5World) stepVerifyInvalidReviews() error {
	if w.refuseCode1 != "forbidden" || w.refuseCode2 != "conflict" {
		return fmt.Errorf("got codes (%q, %q), want (forbidden, conflict)", w.refuseCode1, w.refuseCode2)
	}
	return nil
}

func (w *m5World) stepRunSixFailedReviews() error {
	specs := []struct {
		score int
		veto  bool
	}{
		{86, false}, {95, true}, {70, false}, {70, false}, {70, false}, {70, false},
	}
	for i, sp := range specs {
		if err := w.runOneFailedReviewRound(i+1, sp.score, sp.veto); err != nil {
			return err
		}
	}
	return nil
}

func (w *m5World) runOneFailedReviewRound(round, score int, veto bool) error {
	if round > 1 {
		if err := w.resubmitSameCommit(round); err != nil {
			return err
		}
	}
	w.setMemberHeld(w.claudeM, true)
	items := []any{map[string]any{"text": fmt.Sprintf("issue %d", round), "veto": veto}}
	w.writeScriptFile("codex", m5CodexScript(fmt.Sprintf("codex-r-%d", round), []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": score, "items": items},
	}}))
	w.setMemberHeld(w.codexM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.codexM)
	return w.waitFor(fmt.Sprintf("review round %d", round), func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Review != nil && t.Review.Round == round
	})
}

func (w *m5World) resubmitSameCommit(round int) error {
	w.setMemberHeld(w.codexM, true)
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{
		Name: "submit_task", Args: map[string]any{"summary": fmt.Sprintf("Resubmit %d.", round)},
	}}))
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return w.waitFor(fmt.Sprintf("resubmit %d at review", round), func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "review"
	})
}

func (w *m5World) collectReviewScores() []int {
	var scores []int
	for _, m := range w.browserMessages() {
		var b proto.ReviewBody
		if m.Kind == "review" && json.Unmarshal(m.Body, &b) == nil {
			scores = append(scores, b.Score)
		}
	}
	return scores
}

func m5SixScoresMatch(scores []int) bool {
	return len(scores) == 6 && scores[0] == 86 && scores[1] == 0
}

func (w *m5World) stepVerifySixFailedReviews() error {
	revScores := w.collectReviewScores()
	t, ok := w.taskByNum(1)
	if !ok || !m5SixScoresMatch(revScores) || t.State != "failed" || t.Review.Failed != 6 {
		return fmt.Errorf("unexpected review scores=%v task=%+v", revScores, t)
	}
	return nil
}

func (w *m5World) stepCommitSecretAndRunChecks() error {
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{Name: "run_checks"}}))
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		secretEnv := "MODE=test\nSERVICE_TOKEN=placeholder\nPORT=8000\n"
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"app.env": secretEnv, "app_test.go": "package proj\n"}, "Add config")
		return err
	})
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return w.waitFor("secret run_checks finished", func() bool { return w.toolCallCount() >= 1 })
}

func (w *m5World) stepVerifySecretHardCheck() error {
	for _, m := range w.browserMessages() {
		if m.Kind == "checks" && m5HasFailedSecretsItem(m.Body) {
			return nil
		}
	}
	return errors.New("expected failing secrets hard-check item in checks message")
}

func m5HasFailedSecretsItem(raw json.RawMessage) bool {
	var body proto.ChecksBody
	if json.Unmarshal(raw, &body) != nil || body.OK {
		return false
	}
	for _, it := range body.Items {
		if it.Name == "secrets" && !it.OK {
			return true
		}
	}
	return false
}

func (w *m5World) stepSetupEnvCheckGroup() error {
	if os.Getenv("HOME") == "" {
		w.setEnv("HOME", w.f.t.TempDir())
	}
	w.setEnv("SOLODAWN_M5_TEST_TOKEN", "node-only-token-value")
	return w.givenOneBriefTask("none", []string{"m5envcheck"}, true)
}

func (w *m5World) stepRunEnvCheckTask() error {
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{Name: "run_checks"}}))
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"env.go": "package proj\n", "env_test.go": "package proj\n"}, "Test env")
		return err
	})
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return w.waitFor("m5envcheck run_checks finished", func() bool { return w.toolCallCount() >= 1 })
}

func (w *m5World) stepVerifyEnvCheckPassed() error {
	t, ok := w.taskByNum(1)
	if !ok || t.Checks.Local == nil || !t.Checks.Local.OK {
		return fmt.Errorf("expected passing local check, got task=%+v", t)
	}
	return nil
}

func m5RegisterDepAndDirectSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a confirmed brief where task 2 depends on task 1 on forge "none"$`, w.stepGivenConfirmedDepBrief)
	sc.Step(`^the secretary reassigns task 2 to "Codex" and reviewer "Claude" while task 2 is queued$`, w.stepReassignQueuedTask2)
	sc.Step(`^task 2 remains queued with no worktree and no task session$`, w.stepVerifyTask2StillQueued)
	sc.Step(`^task 1 completes checks, passes review and merges$`, w.stepFinishAndMergeTask1)
	sc.Step(`^task 2 becomes active, creates its worktree, and runs its task session for "Codex"$`, w.stepVerifyTask2ActivatedForCodex)

	sc.Step(`^the owner mentions "Claude" directly for a code change and "Claude" calls start_task$`, w.stepRunDirectTaskA57)
	sc.Step(`^the group session kept repo clean without task submission tools, created a task card, notified the secretary, and committed the change in a new task session$`, w.stepVerifyDirectTaskA57)
}

func (w *m5World) stepGivenConfirmedDepBrief() error {
	if err := w.setupCluster("none", []string{}, true); err != nil {
		return err
	}
	brief := m5BriefArgs([]map[string]any{
		{"title": "Base unit", "assignee": "Claude", "reviewer": "Codex", "depends": []int{}},
		{"title": "Dependent unit", "assignee": "Claude", "reviewer": "Codex", "depends": []int{0}},
	})
	w.writeScriptFile("claude", m5ClaudeScript(
		[]m5ToolCallSpec{{Name: "propose_brief", Args: brief}},
		nil,
		[]m5ToolCallSpec{{Name: "set_task", Args: map[string]any{"task": "T-2", "assignee": "Codex", "reviewer": "Claude"}}},
	))
	if err := w.ownerPostMain("Build base and dependent units.", nil); err != nil {
		return err
	}
	if err := w.waitPendingBrief(); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, true)
	if err := w.ownerConfirmBrief(w.pendingBriefID()); err != nil {
		return err
	}
	twoTasks := func() bool { return len(w.browserTasks()) == 2 }
	return w.waitFor("2 tasks created", twoTasks)
}

func (w *m5World) stepReassignQueuedTask2() error {
	if err := w.ownerPostMain("Give T-2 to Codex.", nil); err != nil {
		return err
	}
	return w.waitFor("task 2 reassigned to Codex", func() bool {
		t2, ok := w.taskByNum(2)
		return ok && t2.Assignee == w.codexM && t2.Reviewer == w.claudeM
	})
}

func (w *m5World) stepVerifyTask2StillQueued() error {
	t2, ok := w.taskByNum(2)
	if !ok || t2.State != "queued" {
		return fmt.Errorf("expected task 2 queued, got %+v", t2)
	}
	wt2 := filepath.Join(w.projectDir(), "wt", "2-dependent-unit")
	if _, err := os.Stat(wt2); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("expected no worktree for queued task 2, got err=%v", err)
	}
	h := w.cluster.Nodes[0].Host
	if _, exists := h.Sessions.Get(host.SessionKey{Group: w.groupID, Member: w.codexM, Task: t2.ID}); exists {
		return errors.New("unexpected task session for queued task 2")
	}
	return nil
}

func (w *m5World) stepFinishAndMergeTask1() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "run_checks"},
		{Name: "submit_task", Args: map[string]any{"summary": "Base unit done."}},
	}))
	w.writeScriptFile("codex", m5CodexScript("codex-dep-rev1", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"base.go": "package proj\n", "base_test.go": "package proj\n"}, "Base unit")
		return err
	})
	w.addBeforeHook("submit_review", func(_ host.Caller) error {
		w.writeScriptFile("codex", m5CodexScript("codex-dep-t2", []m5ToolCallSpec{
			{Name: "post", Args: map[string]any{"kind": "progress", "text": "Started T-2."}},
		}))
		return nil
	})
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return w.waitFor("task 1 merged and task 2 active", func() bool {
		t1, ok1 := w.taskByNum(1)
		t2, ok2 := w.taskByNum(2)
		return ok1 && ok2 && t1.State == "merged" && t2.State == "active"
	})
}

func (w *m5World) stepVerifyTask2ActivatedForCodex() error {
	return w.waitFor("Codex ran task session in task 2 worktree", w.codexCompletedT2Turn)
}

func (w *m5World) codexCompletedT2Turn() bool {
	w.mu.Lock()
	sess := w.sawTaskSess[2]
	dir := w.sawTaskDir[2]
	w.mu.Unlock()
	return sess.Member == w.codexM && strings.HasSuffix(dir, "2-dependent-unit") && w.hasMsgWithText("Started T-2.")
}

func (w *m5World) hasMsgWithText(text string) bool {
	for _, m := range w.browserMessages() {
		if m.Text == text {
			return true
		}
	}
	return false
}

func (w *m5World) stepRunDirectTaskA57() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "start_task", Args: map[string]any{"title": "Direct helper", "plan": "Add direct helper and test."}},
	}))
	w.addBeforeHook("start_task", func(_ host.Caller) error {
		w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{Name: "run_checks"}}))
		return nil
	})
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		_, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"direct.go": "package proj\n", "direct_test.go": "package proj\n"}, "Direct change")
		return err
	})
	if err := w.ownerPostMain("@Claude add a direct helper.", []string{w.claudeM}); err != nil {
		return err
	}
	directChecked := func() bool {
		t1, ok := w.taskByNum(1)
		return ok && t1.Origin == "direct" && t1.Checks.Local != nil && t1.Checks.Local.OK
	}
	if err := w.waitFor("direct task checked in new task session", directChecked); err != nil {
		return err
	}
	return w.deliverDirectNoticeToSec()
}

func (w *m5World) deliverDirectNoticeToSec() error {
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
		{Name: "post", Args: map[string]any{"text": "Noted T-1."}},
	}))
	w.setMemberHeld(w.secID, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.secID)
	secNoted := func() bool {
		for _, m := range w.browserMessages() {
			if m.By == w.secID && m.Text == "Noted T-1." {
				return true
			}
		}
		return false
	}
	return w.waitFor("secretary acknowledged task_started notice", secNoted)
}

func m5GroupToolsReadOnly(grpTools []string) bool {
	if !slices.Contains(grpTools, "start_task") {
		return false
	}
	return !slices.Contains(grpTools, "submit_task") && !slices.Contains(grpTools, "run_checks")
}

func (w *m5World) stepVerifyDirectTaskA57() error {
	status, err := m5GitOut(filepath.Join(w.projectDir(), "repo"), "status", "--porcelain", "--untracked-files=all")
	if err != nil || status != "" {
		return fmt.Errorf("expected clean repo folder, got status=%q err=%v", status, err)
	}
	w.mu.Lock()
	grpTools := slices.Clone(w.listedTools[host.SessionKey{Group: w.groupID, Member: w.claudeM}])
	sess := w.sawTaskSess[1]
	w.mu.Unlock()
	if !m5GroupToolsReadOnly(grpTools) {
		return fmt.Errorf("unexpected group session tools: %v", grpTools)
	}
	if sess.Task == "" || !w.hasTaskCardMsg(sess.Task) {
		return fmt.Errorf("expected task card and task session, got sess=%+v", sess)
	}
	return nil
}

func (w *m5World) hasTaskCardMsg(taskID string) bool {
	for _, m := range w.browserMessages() {
		if m.Kind == "task" && m.ID == taskID {
			return true
		}
	}
	return false
}

func m5RegisterConflictAndEdgeSteps(sc *godog.ScenarioContext, w *m5World) {
	sc.Step(`^a submitted brief task on forge "none" whose base branch gained a conflicting commit$`, func() error {
		return w.givenSubmittedBriefTask(true)
	})
	sc.Step(`^"Codex" passes the review and the host attempts the merge$`, w.stepPassReviewIntoConflict)
	sc.Step(`^the task returns to local_check with wait "conflict" and the secretary receives a conflict notice$`, w.stepVerifyMergeConflictAndNotice)
	sc.Step(`^the secretary names "Claude" on the conflicted task and "Claude" resolves, re-checks, resubmits, passes review and merges$`, w.stepResolveConflictAndMerge)
	sc.Step(`^the task reaches state "merged" and base holds the resolved content$`, w.stepVerifyConflictResolvedOnBase)

	sc.Step(`^a project group with forge "none", a secretary, working AIs and a pending one-task brief$`, w.stepGivenPendingOneTaskBrief)
	sc.Step(`^the owner sends the same signed confirm twice and the host sends the same signed task fail twice$`, w.stepSendDuplicateConfirmAndFail)
	sc.Step(`^both confirms answered ok creating one task, and both task fails answered ok counting one failure$`, w.stepVerifyDuplicateConfirmAndFail)

	sc.Step(`^removing "Claude" or "Codex" from the group is refused with "conflict"$`, w.stepVerifyBusyAICannotLeave)
	sc.Step(`^"Codex" passes the review while the host holds the merge item$`, w.stepPassReviewWithMergeHeld)
	sc.Step(`^reassigning the task and submitting a new commit at step merge are refused with "conflict"$`, w.stepVerifyMergeStepRefusals)
	sc.Step(`^the owner cancels the task at step merge and the host releases and processes the held merge item$`, w.stepOwnerCancelsAndHostDropsMerge)
	sc.Step(`^the task is cancelled, base is unchanged, and the host recorded the merge item as refused$`, w.stepVerifyCancelledMergeDropped)

	sc.Step(`^an active brief task on forge "none" assigned to "Claude" with reviewer "Codex" held on the host$`, func() error {
		return w.givenOneBriefTask("none", []string{}, true)
	})
	sc.Step(`^"Claude" submits commit 1 and then commits, checks and submits commit 2 before "Codex" is released$`, w.stepSubmitTwoCommitsWhileReviewerHeld)
	sc.Step(`^the home has retired the review item for commit 1 and keeps only commit 2 queued$`, w.stepVerifyHomeRetiredCommit1Review)
	sc.Step(`^the host hold on "Codex" is released and "Codex" runs its review turn$`, w.stepReleaseReviewerForCommit2)
	sc.Step(`^the host refused the held review item for commit 1 and delivered only commit 2 to "Codex"$`, w.stepVerifyHostRefusedStaleReview)
}

func (w *m5World) stepPassReviewIntoConflict() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("codex", m5CodexScript("codex-conf-rev1", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	w.setMemberHeld(w.codexM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.codexM)
	return w.waitFor("task 1 wait conflict", func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "local_check" && t.Wait != nil && t.Wait.Kind == "conflict"
	})
}

func m5IsSecConflictNotice(it store.Item, secID string) bool {
	var plain proto.Item
	if json.Unmarshal(it.Env, &plain) != nil {
		return false
	}
	return string(plain.AI) == secID && plain.Why == "notice" && plain.Code == "conflict"
}

func (w *m5World) stepVerifyMergeConflictAndNotice() error {
	items, err := w.homeQueuedItems()
	if err != nil {
		return err
	}
	for _, it := range items {
		if m5IsSecConflictNotice(it, w.secID) {
			return nil
		}
	}
	return fmt.Errorf("expected queued conflict notice for secretary, got %+v", items)
}

func (w *m5World) stepResolveConflictAndMerge() error {
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{
		Name: "set_task", Args: map[string]any{"task": "T-1", "assignee": "Claude"},
	}}))
	w.writeScriptFile("codex", m5CodexScript("codex-conf-rev2", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	w.addBeforeHook("set_task", func(_ host.Caller) error {
		w.setMemberHeld(w.secID, true)
		w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{
			{Name: "run_checks"},
			{Name: "submit_task", Args: map[string]any{"summary": "Resolved conflict."}},
		}))
		return nil
	})
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		dir := w.sessionFolder(c)
		if _, err := m5GitOut(dir, "-c", "user.name=test", "-c", "user.email=test@test.invalid", "merge", "-s", "ours", "-m", "Sync base", "main"); err != nil {
			return err
		}
		_, err := m5GitCommitFile(dir, "shared.txt", "resolved-both\n", "Resolve shared.txt")
		return err
	})
	w.setMemberHeld(w.secID, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.secID)
	return w.waitFor("task 1 merged after conflict resolution", func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.State == "merged"
	})
}

func (w *m5World) stepVerifyConflictResolvedOnBase() error {
	out, err := m5GitOut(filepath.Join(w.projectDir(), "repo"), "show", "refs/heads/main:shared.txt")
	if err != nil || out != "resolved-both" {
		return fmt.Errorf("got shared.txt=%q err=%v, want resolved-both", out, err)
	}
	return nil
}

func (w *m5World) stepGivenPendingOneTaskBrief() error {
	if err := w.setupCluster("none", []string{}, true); err != nil {
		return err
	}
	w.writeScriptFile("claude", m5ClaudeScript([]m5ToolCallSpec{{Name: "propose_brief", Args: m5OneTaskBriefArgs()}}))
	if err := w.ownerPostMain("Add gamma helper.", nil); err != nil {
		return err
	}
	return w.waitFor("pending brief", func() bool { return w.pendingBriefID() != "" })
}

func (w *m5World) stepSendDuplicateConfirmAndFail() error {
	n0 := w.cluster.Nodes[0]
	w.setMemberHeld(w.secID, true)
	w.setMemberHeld(w.claudeM, true)
	confirmRaw := m5MustJSON(map[string]any{
		"t": "act", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "msg": w.pendingBriefID(), "action": "confirm",
	})
	confirmEnv := w.cluster.Owner.Sign(confirmRaw)
	var err error
	if w.confirmAns1, err = w.sendEnvelopeCmd(confirmEnv); err != nil {
		return err
	}
	if w.confirmAns2, err = w.sendEnvelopeCmd(confirmEnv); err != nil {
		return err
	}
	oneTask := func() bool { return len(w.browserTasks()) == 1 }
	if err := w.waitFor("one task created", oneTask); err != nil {
		return err
	}
	return w.sendDuplicateTaskFail()
}

func (w *m5World) sendDuplicateTaskFail() error {
	n0 := w.cluster.Nodes[0]
	t1, _ := w.taskByNum(1)
	failPayload := m5MustJSON(map[string]any{
		"t": "task", "op": "fail", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": t1.ID, "reason": "crash", "detail": "simulated exit",
	})
	env := n0.Key.Sign(failPayload)
	var err error
	if w.failAns1, err = w.sendHostEnvelope(env); err != nil {
		return err
	}
	if w.failAns2, err = w.sendHostEnvelope(env); err != nil {
		return err
	}
	return w.waitFor("task failure recorded", func() bool {
		cur, ok := w.taskByNum(1)
		return ok && cur.Failures >= 1
	})
}

func (w *m5World) sendHostEnvelope(env proto.Envelope) (proto.CmdResult, error) {
	n0 := w.cluster.Nodes[0]
	build := func(id int64) any {
		return proto.Cmd{Request: proto.Request{Type: "cmd", ID: id}, Env: env}
	}
	raw, err := n0.Link.Request(context.Background(), build)
	if err != nil {
		return proto.CmdResult{}, err
	}
	var res proto.CmdResult
	return res, proto.Decode(raw, &res)
}

func m5BothOK(a, b proto.CmdResult) bool {
	return bool(a.OK && b.OK)
}

func (w *m5World) stepVerifyDuplicateConfirmAndFail() error {
	t1, ok := w.taskByNum(1)
	if !ok || !m5BothOK(w.confirmAns1, w.confirmAns2) || len(w.browserTasks()) != 1 {
		return fmt.Errorf("unexpected confirm results: %+v %+v tasks=%d", w.confirmAns1, w.confirmAns2, len(w.browserTasks()))
	}
	if !m5BothOK(w.failAns1, w.failAns2) || t1.Failures != 1 {
		return fmt.Errorf("unexpected fail results: %+v %+v failures=%d", w.failAns1, w.failAns2, t1.Failures)
	}
	return nil
}

func (w *m5World) stepVerifyBusyAICannotLeave() error {
	for _, memberID := range []string{w.claudeM, w.codexM} {
		if m5ErrCode(w.ownerRemoveMember(memberID)) != "conflict" {
			return fmt.Errorf("expected conflict removing member %s", memberID)
		}
	}
	err := w.callHostSurface("host.leave", map[string]any{"g": w.groupID, "agent": "codex"})
	if m5ErrCode(err) != "conflict" {
		return fmt.Errorf("expected conflict on host.leave codex, got %v", err)
	}
	return nil
}

func (w *m5World) ownerRemoveMember(memberID string) error {
	n0 := w.cluster.Nodes[0]
	_, err := w.ownerCmd(map[string]any{
		"t": "member", "op": "remove", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": memberID,
	})
	return err
}

func (w *m5World) stepPassReviewWithMergeHeld() error {
	w.setMemberHeld(w.claudeM, true)
	w.writeScriptFile("codex", m5CodexScript("codex-a518-rev", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	w.setMemberHeld(w.codexM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.codexM)
	return w.waitFor("task 1 at step merge", func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.Step == "merge"
	})
}

func (w *m5World) stepVerifyMergeStepRefusals() error {
	n0 := w.cluster.Nodes[0]
	t1, _ := w.taskByNum(1)
	_, errAssign := w.ownerCmd(map[string]any{
		"t": "task", "op": "assign", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": t1.ID, "assignee": w.codexM, "reviewer": w.claudeM,
	})
	subRaw := m5MustJSON(map[string]any{
		"t": "task", "op": "submitted", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": t1.ID, "commit": t1.Commit, "stats": map[string]any{"added": 1, "removed": 0},
	})
	_, errSub := n0.Host.Command(context.Background(), n0.Key.ID(), subRaw)
	if m5ErrCode(errAssign) != "conflict" || m5ErrCode(errSub) != "conflict" {
		return fmt.Errorf("expected conflict for assign (%v) and submitted (%v)", errAssign, errSub)
	}
	return nil
}

func (w *m5World) stepOwnerCancelsAndHostDropsMerge() error {
	n0 := w.cluster.Nodes[0]
	t1, _ := w.taskByNum(1)
	if _, err := w.ownerCmd(map[string]any{
		"t": "task", "op": "cancel", "n": w.nextID(), "ts": n0.Host.Now().UnixMilli(),
		"g": w.groupID, "id": t1.ID,
	}); err != nil {
		return err
	}
	hostCancelled := func() bool {
		raw, ok := n0.Host.Rows.Get(w.groupID, "task", t1.ID)
		return ok && strings.Contains(string(raw), `"state":"cancelled"`)
	}
	if err := w.waitFor("host sees cancelled task", hostCancelled); err != nil {
		return err
	}
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(n0.Host, w.groupID, w.claudeM)
	auditRefused := func() bool {
		n, err := w.hostAuditCount("refused", "stale_task")
		return err == nil && n >= 1
	}
	return w.waitFor("merge item refused in audit", auditRefused)
}

func (w *m5World) stepVerifyCancelledMergeDropped() error {
	t1, ok := w.taskByNum(1)
	if !ok || t1.State != "cancelled" {
		return fmt.Errorf("expected cancelled task, got %+v", t1)
	}
	out, err := m5GitOut(filepath.Join(w.projectDir(), "repo"), "show", "refs/heads/main:shared.txt")
	if err != nil || out != "initial" {
		return fmt.Errorf("expected base shared.txt unchanged (initial), got %q err=%v", out, err)
	}
	return nil
}

func (w *m5World) stepSubmitTwoCommitsWhileReviewerHeld() error {
	calls := []m5ToolCallSpec{
		{Name: "run_checks"},
		{Name: "submit_task", Args: map[string]any{"summary": "Commit 1."}},
		{Name: "run_checks"},
		{Name: "submit_task", Args: map[string]any{"summary": "Commit 2."}},
	}
	w.writeScriptFile("claude", m5ClaudeScript(calls))
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		sha, err := m5GitCommitFiles(w.sessionFolder(c), map[string]string{"v1.go": "package proj\n", "v1_test.go": "package proj\n"}, "Commit 1")
		w.setCommit1(sha)
		return err
	})
	w.addBeforeHook("run_checks", func(c host.Caller) error {
		sha, err := m5GitCommitFile(w.sessionFolder(c), "v1.go", "package proj\nvar V = 2\n", "Commit 2")
		w.setCommit2(sha)
		return err
	})
	w.setMemberHeld(w.claudeM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.claudeM)
	return w.waitFor("task 1 submitted commit 2", func() bool {
		t, ok := w.taskByNum(1)
		c2 := w.getCommit2()
		return ok && c2 != "" && t.Commit == c2
	})
}

func (w *m5World) stepVerifyHomeRetiredCommit1Review() error {
	items, err := w.homeQueuedItems()
	if err != nil {
		return err
	}
	var commits []string
	for _, it := range items {
		if it.Why == "review" && it.Commit != nil {
			commits = append(commits, *it.Commit)
		}
	}
	c2 := w.getCommit2()
	if !slices.Equal(commits, []string{c2}) {
		return fmt.Errorf("expected home queued reviews [%s], got %v", c2, commits)
	}
	return nil
}

func (w *m5World) stepReleaseReviewerForCommit2() error {
	w.setMemberHeld(w.secID, true)
	w.writeScriptFile("codex", m5CodexScript("codex-a519-rev", []m5ToolCallSpec{{
		Name: "submit_review", Args: map[string]any{"score": 95, "items": []any{}},
	}}))
	w.setMemberHeld(w.codexM, false)
	host.Deliver(w.cluster.Nodes[0].Host, w.groupID, w.codexM)
	return w.waitFor("task 1 merged at commit 2", func() bool {
		t, ok := w.taskByNum(1)
		return ok && t.State == "merged"
	})
}

func (w *m5World) stepVerifyHostRefusedStaleReview() error {
	refused, err := w.hostAuditCount("refused", "stale_task")
	if err != nil || refused < 1 {
		return fmt.Errorf("expected host audit refused stale_task >= 1, got %d (err=%v)", refused, err)
	}
	c2 := w.getCommit2()
	for _, m := range w.browserMessages() {
		if m.Kind == "review" && !strings.Contains(string(m.Body), c2) {
			return fmt.Errorf("unexpected review for stale commit: %s", string(m.Body))
		}
	}
	return nil
}
