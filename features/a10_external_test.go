//go:build acceptance

package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// externalNoClone is what the steps that MODEL-1126 took away say when they fail.
const externalNoClone = "every hand-out names the folder X was added with, not a project clone (MODEL-1126 plan 3.3, A10.18)"

func init() { stepSets["a10_external"] = externalSteps }

type externalWorld struct {
	m6World
	mcp             *mcp.ClientSession
	run             proto.Handout
	oldRun          int64
	wait            <-chan externalCLIResult
	usageTime       time.Time
	usagePct        int
	approval        <-chan error
	folder          string
	heldMsg         proto.ID
	interruptDetail string
}

func externalSteps(sc *godog.ScenarioContext, f fixtures) {
	w := &externalWorld{m6World: m6World{
		f: f, cleanup: afterScenario(sc), aiByName: map[string]proto.AIMemberRow{},
		nodeByAI: map[string]*m6NodeProc{}, tasksByNum: map[int]proto.TaskRow{}, msgs: map[string]proto.MsgRow{},
	}}
	sc.Step(`^a running external acceptance project$`, w.setup)
	sc.Step(`^an external acceptance project with X$`, w.setupX)
	sc.Step(`^the owner joins external member "([^"]+)" with account "([^"]*)" and tool "([^"]*)"$`, w.join)
	sc.Step(`^the node rejects invalid external joins and agent settings$`, w.invalidJoins)
	sc.Step(`^external member "([^"]+)" has state "([^"]+)"$`, w.state)
	sc.Step(`^the owner relabels X to "([^"]+)"$`, w.relabel)
	sc.Step(`^host info advertises both external members and the node executable$`, w.info)
	externalRunnerSteps(sc, w)
	externalMCPSteps(sc, w)
	externalTaskSteps(sc, w)
	externalPairSteps(sc, w)
}

func (w *externalWorld) setupX() error {
	return s3Run(w.setup, func() error { return w.join("X", "api_key", "ZCode") })
}

func (w *externalWorld) setup() error {
	return w.setupProject()
}

// prepareContext bounds the scenario.
func (w *externalWorld) prepareContext() error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	w.ctx = ctx
	w.cleanup(cancel)
	return nil
}

func (w *externalWorld) setupProject() error {
	return s3Run(w.prepareContext, w.prepareRoot, w.prepareClaude, w.startNode, w.createAndSyncGroup)
}

func (w *externalWorld) startNode() error {
	rv, err := startAcceptanceRelayRV(w.cleanup)
	if err != nil {
		return err
	}
	w.rvURL = rv
	return s3Run(func() error { return w.prepareAndStartNodes(false) },
		func() error { return w.connectOwnerToNodes(false) })
}

func (w *externalWorld) header(typ string) proto.HostCommand {
	return proto.HostCommand{Type: typ, Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), To: w.homeNode.key.ID()}
}

func (w *externalWorld) homeHeader(typ string) proto.HomeCommand {
	return proto.HomeCommand{Type: typ, Nonce: w.ids.next(), Timestamp: time.Now().UnixMilli(), Group: w.groupID}
}

func (w *externalWorld) joinCmd(name, account string, tool *string) (proto.HostJoin, error) {
	workspace, err := aiWorkspace(w.root, w.groupID, name)
	return proto.HostJoin{HostCommand: w.header("host.join"), Group: w.groupID, Home: w.homeNode.key.ID(), Agent: "external",
		Name: name, Account: account, Tool: tool, Workspace: workspace}, err
}

func (w *externalWorld) hostCommand(cmd any) error {
	return w.sendSignedCmd(w.homeLink.conn, w.ownerKey, cmd)
}

func (w *externalWorld) join(name, account, tool string) error {
	join := func() error {
		cmd, err := w.joinCmd(name, account, &tool)
		if err != nil {
			return err
		}
		return w.hostCommand(cmd)
	}
	return s3Run(join,
		func() error { return w.state(name, "offline") }, func() error { return w.checkJoin(name, account, tool) })
}

func (w *externalWorld) checkJoin(name, account, tool string) error {
	ai := w.aiByName[name]
	want := proto.Profile{Tier: "standard"}
	if ai.CLI != "external" || ai.Account != account || ai.Tool != tool || ai.Secretary || ai.Profile != want {
		return fmt.Errorf("unexpected external member after join: %+v", ai)
	}
	return nil
}

func (w *externalWorld) invalidJoins() error {
	noAccount, err1 := w.joinCmd("No account", "", nil)
	secretary, err2 := w.joinCmd("Bad secretary", "personal", nil)
	long, err3 := w.joinCmd("Long label", "team", new(strings.Repeat("文", 33)))
	if err := errors.Join(err1, err2, err3); err != nil {
		return err
	}
	secretary.Secretary = true
	for _, cmd := range []any{noAccount, secretary, long,
		proto.AgentSet{HostCommand: w.header("agent.set"), Agent: "external", Account: "personal"}} {
		if err := externalRefusal(w.hostCommand(cmd), "bad_request"); err != nil {
			return err
		}
	}
	return nil
}

func externalRefusal(err error, code string) error {
	if err == nil || !strings.Contains(err.Error(), `"code":"`+code+`"`) {
		return fmt.Errorf("got %v, want a %s refusal", err, code)
	}
	return nil
}

func (w *externalWorld) state(name, state string) error {
	return w.waitState(func() bool { return w.aiByName[name].State == state })
}

func (w *externalWorld) waitExternalPush(what string, ready func() bool) error {
	wait := newWait(5*time.Second, what, nil)
	defer wait.stop()
	for !ready() {
		raw, err := recvWait(wait, w.homeLink.pushes)
		if err != nil {
			return err
		}
		if raw == nil {
			return errors.New("ctl connection closed before push")
		}
		var header struct {
			Type string `json:"t"`
		}
		_ = json.Unmarshal(raw, &header)
		wait.saw.see(header.Type)
		w.recordRowsFrame(raw)
	}
	return nil
}

func (w *externalWorld) relabel(tool string) error {
	cmd := proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: w.homeHeader("member"),
		Op: "set", ID: proto.ID(w.aiByName["X"].ID)}, Tool: &tool}
	return s3Run(func() error { return w.hostCommand(cmd) },
		func() error { return w.waitState(func() bool { return w.aiByName["X"].Tool == tool }) })
}

func (w *externalWorld) info() error {
	info, err := requestHostInfo(w.ctx, w.homeLink.conn)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(info.Exe) || filepath.Base(info.Exe) != filepath.Base(w.f.bin) {
		return fmt.Errorf("host.info exe = %q, want the absolute node executable %q", info.Exe, w.f.bin)
	}
	return w.checkExternalAgents(info.Agents)
}

func (w *externalWorld) checkExternalAgents(agents []proto.HostAgent) error {
	entries := slices.DeleteFunc(slices.Clone(agents), func(a proto.HostAgent) bool { return a.Agent != "external" })
	if len(entries) != 1 || entries[0].Driver.Takeover != "none" || len(entries[0].Models) != 0 {
		return fmt.Errorf("unexpected external host.info entries: %+v", entries)
	}
	return w.checkAgentGroups(entries[0].Groups)
}

func (w *externalWorld) checkAgentGroups(groups []proto.AgentGroup) error {
	var ids []string
	for _, g := range groups {
		for _, id := range []string{w.aiByName["X"].ID, w.aiByName["Y"].ID} {
			if g.Group == w.groupID && string(g.Member) == id {
				ids = append(ids, id)
			}
		}
	}
	want := []string{w.aiByName["X"].ID, w.aiByName["Y"].ID}
	if !slices.Equal(sortedStrings(ids), sortedStrings(want)) {
		return fmt.Errorf("external host.info advertised %v, want %v", ids, want)
	}
	return nil
}

func sortedStrings(values []string) []string {
	slices.Sort(values)
	return values
}

func (w *externalWorld) command(args ...string) *exec.Cmd {
	cmd := exec.CommandContext(w.ctx, w.f.bin, args...)
	root := w.homeNode.root
	cmd.Env = append(os.Environ(), "HOME="+root, "XDG_DATA_HOME="+root, "LocalAppData="+root)
	return cmd
}

type externalCLIResult struct {
	out, stderr string
	code        int
	err         error
}

func runExternalCLI(cmd *exec.Cmd) externalCLIResult {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return externalCLIResult{stdout.String(), stderr.String(), exit.ExitCode(), nil}
	}
	return externalCLIResult{stdout.String(), stderr.String(), 0, err}
}

func (r externalCLIResult) expect(code int) error {
	if r.err != nil || r.code != code {
		return fmt.Errorf("exit %d (%v), stdout %q, stderr %q; want exit %d", r.code, r.err, r.out, r.stderr, code)
	}
	if code == 3 && (r.out != "" || r.stderr != "") {
		return fmt.Errorf("empty wait printed stdout %q and stderr %q", r.out, r.stderr)
	}
	return nil
}

func (w *externalWorld) waitCommand(timeout string) *exec.Cmd {
	args := []string{"wait", "--member", w.aiByName["X"].ID}
	if timeout != "" {
		args = append(args, "--timeout", timeout)
	}
	return w.command(args...)
}

func (w *externalWorld) look(code int) error {
	if code == 0 {
		return waitUntil(5*time.Second, "external wait handout", w.pollLook)
	}
	return runExternalCLI(w.waitCommand("0")).expect(code)
}

// pollLook retries while the item is not staged yet (exit 3); any other
// failure ends the wait.
func (w *externalWorld) pollLook() error {
	r := runExternalCLI(w.waitCommand("0"))
	if r.expect(3) == nil {
		return errors.New("wait exit 3")
	}
	if err := w.readHandout(r); err != nil {
		return abort(err)
	}
	return nil
}

func (w *externalWorld) readHandout(r externalCLIResult) error {
	if err := r.expect(0); err != nil {
		return err
	}
	line, ok := strings.CutSuffix(r.out, "\n")
	if !ok || strings.Contains(line, "\n") || r.stderr != "" {
		return fmt.Errorf("wait did not print one JSON line: %+v", r)
	}
	return w.decodeHandout([]byte(line))
}

func (w *externalWorld) decodeHandout(raw []byte) error {
	var handout proto.Handout
	if err := proto.Decode(raw, &handout); err != nil {
		return err
	}
	w.oldRun, w.run = w.run.Run, handout
	return nil
}

func (w *externalWorld) handout(kind, text string) error {
	run := w.run
	if !externalHandoutIdentity(run, kind, w.oldRun) {
		return fmt.Errorf("unexpected handout: %+v", run)
	}
	if !strings.Contains(run.Text, text) || !strings.Contains(run.Context, "report_usage") {
		return fmt.Errorf("handout did not include %q in text and external instructions in context: %+v", text, run)
	}
	wantTask := ""
	if kind != "group" {
		wantTask = fmt.Sprintf("T-%d", w.currentTask().Num)
	}
	if run.Task != wantTask {
		return fmt.Errorf("handout task = %q, want %q", run.Task, wantTask)
	}
	return nil
}

func externalHandoutIdentity(run proto.Handout, kind string, old int64) bool {
	return run.Kind == kind && run.Run > 0 && run.Run != old && filepath.IsAbs(run.Folder)
}

func (w *externalWorld) end(run int64, extra ...string) error {
	args := []string{"done", "--member", w.aiByName["X"].ID, "--run", strconv.FormatInt(run, 10)}
	return runExternalCLI(w.command(append(args, extra...)...)).expect(0)
}

func (w *externalWorld) startWait() {
	result := make(chan externalCLIResult, 1)
	cmd := w.waitCommand("")
	go func() { result <- runExternalCLI(cmd) }()
	w.wait = result
}

func (w *externalWorld) finishWait(code int) error {
	got, err := recvCtx(w.ctx, w.wait, "the blocking wait to exit", nil)
	if err != nil {
		return err
	}
	if code == 0 {
		return w.readHandout(got)
	}
	return got.expect(code)
}

func (w *externalWorld) keepWaiting() error {
	select {
	case got := <-w.wait:
		return fmt.Errorf("blocking wait exited early: %+v", got)
	default:
		return nil
	}
}

func (w *externalWorld) timedWait() error {
	start := time.Now()
	got := runExternalCLI(w.waitCommand("2"))
	if elapsed := time.Since(start); elapsed < 2*time.Second || elapsed > 5*time.Second {
		return fmt.Errorf("two-second wait took %v", elapsed)
	}
	return got.expect(3)
}

func (w *externalWorld) remove() error {
	return w.hostCommand(proto.MemberCommand{HomeCommand: w.homeHeader("member"), Op: "remove", ID: proto.ID(w.aiByName["X"].ID)})
}

func (w *externalWorld) missingWait(member string) error {
	r := runExternalCLI(w.command("wait", "--member", member, "--timeout", "0"))
	if err := r.expect(1); err != nil {
		return err
	}
	line, terminated := strings.CutSuffix(r.stderr, "\n")
	if r.out != "" || !terminated || strings.TrimSpace(line) == "" || strings.Contains(line, "\n") {
		return fmt.Errorf("missing member wait: stdout %q, stderr %q; want one stderr line and no stdout", r.out, r.stderr)
	}
	return nil
}

func (w *externalWorld) inbox(want int) error {
	db, err := store.Open(filepath.Join(w.homeNode.data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	return awaitHomeInboxCount(db, want)
}

func externalRunnerSteps(sc *godog.ScenarioContext, w *externalWorld) {
	sc.Step(`^the external owner mentions X with "([^"]+)"$`, func(text string) error { return w.whenOwnerMentionsAI("X", text) })
	sc.Step(`^an external look exits (\d+)$`, w.look)
	sc.Step(`^the external handout is a "([^"]+)" run containing "([^"]+)"$`, w.handout)
	sc.Step(`^the external home has (\d+) inbox items$`, w.inbox)
	sc.Step(`^the external runner ends its current run$`, func() error { return w.end(w.run.Run) })
	sc.Step(`^the external runner repeats done for the previous run$`, func() error { return w.end(w.oldRun) })
	sc.Step(`^an external blocking wait starts$`, func() error { w.startWait(); return nil })
	sc.Step(`^the external blocking wait has not exited$`, w.keepWaiting)
	sc.Step(`^the external blocking wait exits (\d+)$`, w.finishWait)
	sc.Step(`^an empty external wait times out after two seconds$`, w.timedWait)
	sc.Step(`^the external owner removes X$`, w.remove)
	sc.Step(`^an external look rejects an unknown member$`, func() error { return w.missingWait(string(exampleID("unknown external"))) })
	sc.Step(`^an external look rejects the removed member$`, func() error { return w.missingWait(w.aiByName["X"].ID) })
	sc.Step(`^the external handout folder is the project clone$`, rewritten("AC-3", externalNoClone))
	sc.Step(`^the "([^"]+)" runs (\d+) queued external items(?:, with the second mention arriving while X is working on the first)?$`, w.runQueued)
}

func (w *externalWorld) openMCP() error {
	session, err := w.connect(w.aiByName["X"].ID)
	if err != nil {
		return err
	}
	w.mcp = session
	w.cleanup(func() { _ = session.Close() })
	return nil
}

func (w *externalWorld) connect(member string) (*mcp.ClientSession, error) {
	start := time.Now()
	client := mcp.NewClient(&mcp.Implementation{Name: "external-acceptance", Version: "1"}, nil)
	session, err := client.Connect(w.ctx, &mcp.CommandTransport{Command: w.command("mcp", "--member", member)}, nil)
	if err != nil {
		return nil, ctxCallErr(w.ctx, start, "external MCP connect", nil, err)
	}
	return session, nil
}

func (w *externalWorld) call(name string, args any) (*mcp.CallToolResult, error) {
	start := time.Now()
	res, err := w.mcp.CallTool(w.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, ctxCallErr(w.ctx, start, "external MCP tool "+name, nil, err)
	}
	return res, nil
}

func externalResult(result *mcp.CallToolResult) ([]byte, error) {
	if result.StructuredContent != nil {
		return json.Marshal(result.StructuredContent)
	}
	if len(result.Content) == 1 {
		if text, ok := result.Content[0].(*mcp.TextContent); ok {
			return []byte(text.Text), nil
		}
	}
	return nil, fmt.Errorf("no JSON tool result: %+v", result)
}

func (w *externalWorld) tool(name, args, want string) error {
	result, err := w.call(name, json.RawMessage(args))
	if err != nil {
		return err
	}
	if want == "ok" && !result.IsError {
		return nil
	}
	return externalToolFailure(name, result, want)
}

func externalToolFailure(name string, result *mcp.CallToolResult, want string) error {
	raw, err := externalResult(result)
	if err != nil {
		return err
	}
	if !result.IsError || !strings.Contains(string(raw), want) {
		return fmt.Errorf("%s returned %s (isError %t), want %s", name, raw, result.IsError, want)
	}
	return nil
}

func (w *externalWorld) listedTools() error {
	var golden proto.ToolLists
	if err := readWireExample("tools", &golden); err != nil {
		return err
	}
	want := []string{"approve"}
	for _, t := range golden.External.Tools {
		want = append(want, t.Name)
	}
	start := time.Now()
	var got []string
	for tool, err := range w.mcp.Tools(w.ctx, nil) {
		if err != nil {
			return ctxCallErr(w.ctx, start, "external MCP Tools", nil, err)
		}
		got = append(got, tool.Name)
	}
	if !slices.Equal(sortedStrings(got), sortedStrings(want)) {
		return fmt.Errorf("external tools = %v, want %v", got, want)
	}
	return nil
}

func (w *externalWorld) callNextItem() ([]byte, error) {
	result, err := w.call("next_item", map[string]any{})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("next_item returned an error: %+v", result)
	}
	return externalResult(result)
}

func (w *externalWorld) nextItem(none bool) error {
	if none {
		raw, err := w.callNextItem()
		if err != nil {
			return err
		}
		return expectExternalNone(raw)
	}
	return waitUntil(5*time.Second, "external MCP next_item handout", w.pollNextItem)
}

func (w *externalWorld) pollNextItem() error {
	raw, err := w.callNextItem()
	if err != nil {
		return abort(err)
	}
	if expectExternalNone(raw) == nil {
		return errors.New("none")
	}
	if err := w.decodeHandout(raw); err != nil {
		return abort(err)
	}
	return nil
}

func expectExternalNone(raw []byte) error {
	var got map[string]bool
	if json.Unmarshal(raw, &got) != nil || len(got) != 1 || !got["none"] {
		return fmt.Errorf("next_item = %s, want {\"none\":true}", raw)
	}
	return nil
}

func (w *externalWorld) rejectsMCP(member string) error {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	client := mcp.NewClient(&mcp.Implementation{Name: "external-acceptance", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: w.command("mcp", "--member", member)}, nil)
	if err == nil {
		session.Close()
		return fmt.Errorf("MCP connected for invalid member %q", member)
	}
	if ctx.Err() != nil {
		return ctxEnded(ctx, start, "external MCP connect rejection", nil)
	}
	return nil
}

func externalMCPSteps(sc *godog.ScenarioContext, w *externalWorld) {
	sc.Step(`^a scripted stdio MCP client connects as X$`, w.openMCP)
	sc.Step(`^external MCP lists exactly the frozen external tools and approve$`, w.listedTools)
	sc.Step(`^external MCP calls "([^"]+)" with (\{.*\}) and gets "([^"]+)"$`, w.tool)
	sc.Step(`^X's note "([^"]+)" and document "([^"]+)" with "([^"]+)" are on the home as X's$`, w.storedNotes)
	sc.Step(`^X's search for "([^"]+)" returns document "([^"]+)" with "([^"]+)" by X$`, w.searchedNote)
	sc.Step(`^external MCP next_item returns none$`, func() error { return w.nextItem(true) })
	sc.Step(`^external MCP next_item returns a handout$`, func() error { return w.nextItem(false) })
	sc.Step(`^external MCP ends the run$`, func() error { return w.tool("done", fmt.Sprintf(`{"run":%d}`, w.run.Run), "ok") })
	sc.Step(`^external MCP rejects an unknown member$`, func() error { return w.rejectsMCP(string(exampleID("unknown external"))) })
	sc.Step(`^external MCP rejects the removed member$`, func() error { return w.rejectsMCP(w.aiByName["X"].ID) })
	sc.Step(`^X posts "([^"]+)" in the external project$`, w.externalPost)
	sc.Step(`^an external post with a test secret is refused$`, func() error {
		return w.tool("post", fmt.Sprintf(`{"text":%q}`, m3SecretToken), "secret")
	})
	sc.Step(`^X reports (\d+) percent used with a reset in (\d+) minutes$`, w.reportUsage)
	sc.Step(`^X's member row and list_members contain that usage$`, w.checkUsage)
}

func (w *externalWorld) externalPost(text string) error {
	if err := w.waitState(func() bool { return w.hasTextPost("X", text, "") }); err != nil {
		return err
	}
	count := 0
	member := w.aiByName["X"].ID
	for _, msg := range w.msgs {
		if msg.Thread == nil && m6MatchesTextMsg(msg, member, text, "") {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("X posted %q %d times, want once", text, count)
	}
	return nil
}

func (w *externalWorld) reportUsage(pct, minutes int) error {
	w.usageTime = time.Now().Add(time.Duration(minutes) * time.Minute)
	w.usagePct = pct
	return w.tool("report_usage", fmt.Sprintf(`{"window_pct":%d,"window_resets":%d}`, pct, w.usageTime.UnixMilli()), "ok")
}

func (w *externalWorld) checkUsage() error {
	rowMatches := func() bool {
		return externalUsageMatches(w.aiByName["X"].Usage, w.usagePct, w.usageTime.UnixMilli())
	}
	if err := w.waitState(rowMatches); err != nil {
		return err
	}
	result, err := w.call("list_members", map[string]any{})
	if err != nil {
		return err
	}
	return w.checkListedUsage(result, w.usagePct)
}

func externalUsageMatches(usage proto.Usage, pct int, resets int64) bool {
	return usage.WindowPct != nil && *usage.WindowPct == float64(pct) &&
		usage.WindowResets != nil && *usage.WindowResets == resets
}

func (w *externalWorld) checkListedUsage(result *mcp.CallToolResult, pct int) error {
	raw, err := externalResult(result)
	if err != nil {
		return err
	}
	var got struct {
		Members []proto.AIMemberRow `json:"members"`
	}
	_ = json.Unmarshal(raw, &got)
	for _, member := range got.Members {
		if member.Name == "X" && externalUsageMatches(member.Usage, pct, w.usageTime.UnixMilli()) {
			return nil
		}
	}
	return fmt.Errorf("list_members did not include X's reported usage: %s", raw)
}

// storedNotes compares what note_put and kb_put wrote, with their author.
func (w *externalWorld) storedNotes(note, name, text string) error {
	db, err := store.Open(filepath.Join(w.homeNode.data, "node.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	var stored string
	err = db.Write(func(tx *store.Tx) error {
		return tx.QueryRow(`SELECT json_group_array(json_array(coalesce(data->>'type', ''), coalesce(data->>'name', ''), data->>'text', data->>'by'))
			FROM (SELECT data FROM row WHERE g = ? AND r = 'note' AND gone = 0 ORDER BY seq)`, string(w.groupID)).Scan(&stored)
	})
	if err != nil {
		return err
	}
	var got [][4]string
	if err := json.Unmarshal([]byte(stored), &got); err != nil {
		return err
	}
	x := w.aiByName["X"].ID
	want := [][4]string{{"", "", note, x}, {"kb", name, text, x}}
	if !slices.Equal(got, want) {
		return fmt.Errorf("notes on the home = %q, want %q", got, want)
	}
	return nil
}

func (w *externalWorld) searchedNote(query, name, text string) error {
	result, err := w.call("kb_search", map[string]string{"query": query})
	if err != nil {
		return err
	}
	raw, err := externalResult(result)
	if err != nil {
		return err
	}
	var got struct {
		Documents []struct{ Name, Text, By string }
		More      bool
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		return err
	}
	want := []struct{ Name, Text, By string }{{name, text, "X"}}
	if !slices.Equal(got.Documents, want) || got.More {
		return fmt.Errorf("kb_search = %s, want only %v", raw, want)
	}
	return nil
}
