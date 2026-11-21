//go:build acceptance

package features

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "codex-mcp-shim" {
		os.Exit(bridgeCodexAcceptanceMCP())
	}
	stepSets["host_codex"] = hostCodexSteps
}

func bridgeCodexAcceptanceMCP() int {
	conn, err := net.Dial("tcp", os.Getenv("SOLODAWN_ACCEPTANCE_MCP_PEER"))
	if err != nil {
		return 1
	}
	defer conn.Close()
	fmt.Fprintln(conn, strings.Join(os.Args[1:], " "))
	go io.Copy(conn, os.Stdin)
	io.Copy(os.Stdout, conn)
	return 0
}

type codexAcceptanceState struct {
	f           fixtures
	undo        func(func())
	logPath     string
	proc        host.Process
	threadID    string
	firstAsk    *host.Ask
	turn2Tokens int64
	seenToken   string
	interArgv   []string
	interSess   string
	interErr    error
}

func hostCodexSteps(sc *godog.ScenarioContext, f fixtures) {
	st := &codexAcceptanceState{f: f, undo: afterScenario(sc)}
	sc.Step(`^the scripted Codex approval session is configured at level 2$`, func() error {
		return st.configureScript("codex-approval.json")
	})
	sc.Step(`^the scripted Codex mid-turn exit session is configured at level 2$`, func() error {
		return st.configureScript("codex-exit-mid-turn.json")
	})
	sc.Step(`^the host starts Codex and sends the first turn$`, func() error {
		return st.startAndBeginTurn("", "tok-initial", "Try the write.")
	})
	sc.Step(`^Codex requests command approval and ends the declined turn with (\d+) tokens$`, st.declineFirstTurn)
	sc.Step(`^the host sends the follow-up turn on the same Codex process and accepts the retry$`, st.acceptSecondTurn)
	sc.Step(`^Codex ends the second turn with (\d+) tokens and logs one process start and exit$`, st.verifySingleProcessLifecycle)
	sc.Step(`^the host starts Codex and reads until the process exits mid-turn$`, st.runUntilMidTurnExit)
	sc.Step(`^the host resumes that Codex thread id with a new MCP token$`, st.resumeWithNewToken)
	sc.Step(`^the resumed Codex turn observes the new MCP token and ends with (\d+) tokens$`, st.verifyResumedTurn)
	sc.Step(`^the host asks the Codex driver for an interactive command without a thread id$`, func() {
		st.interArgv, st.interSess, st.interErr = host.Drivers["codex"].Interactive(st.baseLaunch("", "tok-1"))
	})
	sc.Step(`^the Codex driver refuses the interactive start$`, func() error {
		if st.interErr == nil || st.interArgv != nil || st.interSess != "" {
			return fmt.Errorf("got (%v, %q, %v), want error", st.interArgv, st.interSess, st.interErr)
		}
		return nil
	})
	sc.Step(`^the host asks the Codex driver for a level 2 interactive command on thread "([^"]*)"$`, func(id string) {
		st.interArgv, st.interSess, st.interErr = host.Drivers["codex"].Interactive(st.baseLaunch(id, "tok-2"))
	})
	sc.Step(`^the interactive Codex command resumes "([^"]*)" with workspace-write and project docs disabled$`, st.verifyInteractive)
	sc.Step(`^a scripted Codex session with user-input and permission requests$`, st.configureUserInputAndPermissionsScript)
	sc.Step(`^the host starts Codex and begins the user-input and permissions turn$`, func() error {
		return st.startAndBeginTurn("", "tok-ui-perm", "Ask and request permissions.")
	})
	sc.Step(`^Codex asks user-input questions that the host answers and leaves unanswered$`, st.handleUserInputQuestions)
	sc.Step(`^Codex requests network and file permissions that the host allows then declines$`, st.handlePermissionProfileRequests)
}

func (st *codexAcceptanceState) setEnv(key, value string) {
	old, had := os.LookupEnv(key)
	os.Setenv(key, value)
	st.undo(func() { restoreVar(key, old, had) })
}

func (st *codexAcceptanceState) configureScript(name string) error {
	script, err := filepath.Abs(filepath.Join("..", "internal", "host", "fakecli", "testdata", name))
	if err != nil {
		return err
	}
	if st.logPath == "" {
		st.logPath = filepath.Join(st.f.t.TempDir(), "codex-lifecycle.log")
		st.setEnv("SOLODAWN_FAKECLI_LOG", st.logPath)
	}
	st.setEnv("SOLODAWN_FAKE_CODEX_SCRIPT", script)
	return nil
}

func (st *codexAcceptanceState) baseLaunch(session, token string) host.Launch {
	exe, _ := os.Executable()
	return host.Launch{
		Folder:       st.f.t.TempDir(),
		Session:      session,
		Level:        2,
		Shim:         []string{exe, "codex-mcp-shim", "--token", token},
		Instructions: "Task instructions.",
		ToolTimeout:  300 * time.Second,
	}
}

func (st *codexAcceptanceState) startMCPResponder() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	st.undo(func() { ln.Close() })
	st.setEnv("SOLODAWN_ACCEPTANCE_MCP_PEER", ln.Addr().String())
	go st.serveOneMCP(ln)
	return nil
}

func (st *codexAcceptanceState) serveOneMCP(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	fields := strings.Fields(header)
	token := fields[len(fields)-1]
	st.seenToken = token
	serveMCPLines(reader, conn, token)
}

func serveMCPLines(reader *bufio.Reader, out io.Writer, token string) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(line, &req) != nil {
			return
		}
		replyMCPMethod(out, req.ID, req.Method, token)
	}
}

func replyMCPMethod(out io.Writer, id int, method, token string) {
	switch method {
	case "initialize":
		fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"solodawn","version":"1"}}}`+"\n", id)
	case "tools/list":
		fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[]}}`+"\n", id)
	case "tools/call":
		quoted, _ := json.Marshal(fmt.Sprintf(`{"token": %q}`, token))
		fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":%s}],"isError":false}}`+"\n", id, quoted)
	}
}

func (st *codexAcceptanceState) startAndBeginTurn(session, token, prompt string) error {
	if err := st.startMCPResponder(); err != nil {
		return err
	}
	proc, err := host.Drivers["codex"].Start(st.f.t.Context(), st.baseLaunch(session, token))
	if err != nil {
		return err
	}
	st.proc = proc
	st.undo(func() { proc.Close() })
	ready, err := readNextHostEvent(proc.Events())
	if err != nil || ready.Kind != host.Ready || ready.Session == "" {
		return fmt.Errorf("ready event = %+v (%v)", ready, err)
	}
	st.threadID = ready.Session
	return proc.Turn(prompt)
}

func (st *codexAcceptanceState) declineFirstTurn(wantTokens int64) error {
	ask, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	st.firstAsk = ask
	if err := st.proc.Answer(ask.ID, false); err != nil {
		return err
	}
	tokens, err := readCompletedTurnTokens(st.proc.Events())
	if err != nil || tokens != wantTokens {
		return fmt.Errorf("turn 1 tokens = %d, want %d (%v)", tokens, wantTokens, err)
	}
	return nil
}

func (st *codexAcceptanceState) acceptSecondTurn() error {
	if err := st.proc.Turn("Approved. Retry."); err != nil {
		return err
	}
	ask, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	if ask.ID == st.firstAsk.ID || string(ask.Input) != string(st.firstAsk.Input) {
		return fmt.Errorf("second ask = %+v", ask)
	}
	if err := st.proc.Answer(ask.ID, true); err != nil {
		return err
	}
	st.turn2Tokens, err = readCompletedTurnTokens(st.proc.Events())
	return err
}

func (st *codexAcceptanceState) verifySingleProcessLifecycle(wantTokens int64) error {
	if st.turn2Tokens != wantTokens {
		return fmt.Errorf("turn 2 tokens = %d, want %d", st.turn2Tokens, wantTokens)
	}
	if err := st.closeAndDrain(); err != nil {
		return err
	}
	want := fmt.Sprintf("start codex %s\nexit codex %s\n", st.threadID, st.threadID)
	return checkLifecycleLog(st.logPath, want)
}

func (st *codexAcceptanceState) runUntilMidTurnExit() error {
	if err := st.startAndBeginTurn("", "tok-before", "Try the write."); err != nil {
		return err
	}
	if _, err := readAskedEvent(st.proc.Events()); err != nil {
		return err
	}
	return waitChannelClosed(st.proc.Events())
}

func (st *codexAcceptanceState) resumeWithNewToken() error {
	if err := st.configureScript("codex-resume.json"); err != nil {
		return err
	}
	return st.startAndBeginTurn(st.threadID, "tok-resumed", "Check the token.")
}

func (st *codexAcceptanceState) verifyResumedTurn(wantTokens int64) error {
	tokens, err := readCompletedTurnTokens(st.proc.Events())
	if err != nil || tokens != wantTokens {
		return fmt.Errorf("resumed tokens = %d, want %d (%v)", tokens, wantTokens, err)
	}
	if st.seenToken != "tok-resumed" {
		return fmt.Errorf("seen MCP token = %q, want tok-resumed", st.seenToken)
	}
	if err := st.closeAndDrain(); err != nil {
		return err
	}
	want := strings.Join([]string{
		"start codex " + st.threadID,
		"exit codex " + st.threadID,
		"start codex " + st.threadID,
		"exit codex " + st.threadID,
	}, "\n") + "\n"
	return checkLifecycleLog(st.logPath, want)
}

func (st *codexAcceptanceState) closeAndDrain() error {
	if err := st.proc.Close(); err != nil {
		return err
	}
	return waitChannelClosed(st.proc.Events())
}

func (st *codexAcceptanceState) verifyInteractive(wantID string) error {
	if st.interErr != nil || st.interSess != wantID {
		return fmt.Errorf("interactive = (%q, %v)", st.interSess, st.interErr)
	}
	for _, item := range []string{wantID, "--no-alt-screen", "--no-daemon", "workspace-write", "on-request", "project_doc_max_bytes=0"} {
		if !slices.Contains(st.interArgv, item) {
			return fmt.Errorf("interactive argv %q missing %q", st.interArgv, item)
		}
	}
	return nil
}

func readAskedEvent(ch <-chan host.Event) (*host.Ask, error) {
	ev, err := readUntilSignificant(ch)
	if err != nil || ev.Kind != host.Asked || ev.Ask == nil {
		return nil, fmt.Errorf("expected Asked, got %+v (%v)", ev, err)
	}
	return ev.Ask, nil
}

func readCompletedTurnTokens(ch <-chan host.Event) (int64, error) {
	ev, err := readUntilTurnEnded(ch)
	if err != nil {
		return 0, err
	}
	if ev.Status != "completed" || ev.Usage.Tokens == nil {
		return 0, fmt.Errorf("unexpected turn end %+v", ev)
	}
	return *ev.Usage.Tokens, nil
}

func readNextHostEvent(ch <-chan host.Event) (host.Event, error) {
	select {
	case ev, ok := <-ch:
		if !ok {
			return host.Event{}, errors.New("events channel closed")
		}
		return ev, nil
	case <-time.After(10 * time.Second):
		return host.Event{}, errors.New("timed out reading event")
	}
}

func readUntilSignificant(ch <-chan host.Event) (host.Event, error) {
	for {
		ev, err := readNextHostEvent(ch)
		if err != nil || ev.Kind != host.Active {
			return ev, err
		}
	}
}

func readUntilTurnEnded(ch <-chan host.Event) (host.Event, error) {
	for {
		ev, err := readUntilSignificant(ch)
		if err != nil || ev.Kind == host.TurnEnded {
			return ev, err
		}
		if ev.Kind != host.Used {
			return host.Event{}, fmt.Errorf("unexpected event %+v", ev)
		}
	}
}

func waitChannelClosed(ch <-chan host.Event) error {
	timeout := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return nil
			}
		case <-timeout:
			return errors.New("timed out waiting for events channel to close")
		}
	}
}

func checkLifecycleLog(path, want string) error {
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		return fmt.Errorf("lifecycle log = %q, want %q (%v)", got, want, err)
	}
	return nil
}

func (st *codexAcceptanceState) configureUserInputAndPermissionsScript() error {
	const rawScript = `{
  "session": "thr-ui-perm",
  "steps": [
    {"dir":"stdin","msg":{"method":"initialize"}},
    {"dir":"stdout","msg":{"id":"$id","result":{}}},
    {"dir":"stdin","msg":{"method":"initialized"}},
    {"dir":"stdin","msg":{"method":"thread/start"}},
    {"dir":"stdout","msg":{"id":"$id","result":{"thread":{"id":"$session"}}}},
    {"dir":"stdin","msg":{"method":"turn/start","params":{"threadId":"$session"}}},
    {"dir":"stdout","msg":{"id":"$id","result":{"turn":{"id":"turn-ui-perm"}}}},
    {"dir":"stdout","msg":{"id":10,"method":"item/tool/requestUserInput","params":{"questions":[]}}},
    {"dir":"stdin","msg":{"id":10,"result":{"answers":{}}}},
    {"dir":"stdout","msg":{"id":11,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q1","header":"DB","question":"Which database?","isSecret":false,"options":[{"label":"SQLite","description":"Local file"}]}]}}},
    {"dir":"stdin","msg":{"id":11,"result":{"answers":{"q1":{"answers":["SQLite"]}}}}},
    {"dir":"stdout","msg":{"id":12,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q2","question":"Extra note?"}]}}},
    {"dir":"stdin","msg":{"id":12,"result":{"answers":{}}}},
    {"dir":"stdout","msg":{"id":13,"method":"item/permissions/requestApproval","params":{"permissions":[]}}},
    {"dir":"stdin","msg":{"id":13,"result":{"permissions":{}}}},
    {"dir":"stdout","msg":{"id":14,"method":"item/permissions/requestApproval","params":{"permissions":{"network":{"enabled":true}},"reason":"Fetch dependency"}}},
    {"dir":"stdin","msg":{"id":14,"result":{"permissions":{"network":{"enabled":true}}}}},
    {"dir":"stdout","msg":{"id":15,"method":"item/permissions/requestApproval","params":{"permissions":{"fileSystem":{"write":["/outside"]},"network":null},"reason":"Write outside"}}},
    {"dir":"stdin","msg":{"id":15,"result":{"permissions":{}}}},
    {"dir":"stdout","msg":{"method":"thread/tokenUsage/updated","params":{"turnId":"turn-ui-perm","tokenUsage":{"last":{"totalTokens":1234}}}}},
    {"dir":"stdout","msg":{"method":"turn/completed","params":{"turn":{"id":"turn-ui-perm","status":"completed","error":null}}}},
    {"dir":"eof"}
  ]
}`
	path := filepath.Join(st.f.t.TempDir(), "codex-ui-perm.json")
	if err := os.WriteFile(path, []byte(rawScript), 0600); err != nil {
		return err
	}
	if st.logPath == "" {
		st.logPath = filepath.Join(st.f.t.TempDir(), "codex-lifecycle.log")
		st.setEnv("SOLODAWN_FAKECLI_LOG", st.logPath)
	}
	st.setEnv("SOLODAWN_FAKE_CODEX_SCRIPT", path)
	return nil
}

func (st *codexAcceptanceState) handleUserInputQuestions() error {
	ask1, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	if ask1.Tool != "requestUserInput" || ask1.Reply == nil {
		return fmt.Errorf("unexpected first user-input ask: %+v", ask1)
	}
	if err := ask1.Reply(map[string][]string{"q1": {"SQLite"}}); err != nil {
		return err
	}
	if ask1.Reply(map[string][]string{"q1": {"SQLite"}}) == nil {
		return errors.New("duplicate user-input Reply succeeded")
	}
	ask2, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	return ask2.Reply(nil)
}

func (st *codexAcceptanceState) handlePermissionProfileRequests() error {
	netAsk, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	if netAsk.Tool != "permissions" || netAsk.Category != "network" || netAsk.Command != `{"network":{"enabled":true}}` || netAsk.Why != "Fetch dependency" {
		return fmt.Errorf("unexpected network permissions ask: %+v", netAsk)
	}
	if err := st.proc.Answer(netAsk.ID, true); err != nil {
		return err
	}
	fileAsk, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	if fileAsk.Tool != "permissions" || fileAsk.Category != "file" || fileAsk.Command != `{"fileSystem":{"write":["/outside"]},"network":null}` || fileAsk.Why != "Write outside" {
		return fmt.Errorf("unexpected file permissions ask: %+v", fileAsk)
	}
	if err := st.proc.Answer(fileAsk.ID, false); err != nil {
		return err
	}
	tokens, err := readCompletedTurnTokens(st.proc.Events())
	if err != nil || tokens != 1234 {
		return fmt.Errorf("turn tokens = %d, want 1234 (%v)", tokens, err)
	}
	if err := st.closeAndDrain(); err != nil {
		return err
	}
	want := fmt.Sprintf("start codex %s\nexit codex %s\n", st.threadID, st.threadID)
	return checkLifecycleLog(st.logPath, want)
}
