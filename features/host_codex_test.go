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
	f         fixtures
	undo      func(func())
	logPath   string
	proc      host.Process
	threadID  string
	firstAsk  *host.Ask
	seenToken string
	interArgv []string
	interSess string
	interErr  error
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
	sc.Step(`^Codex requests command approval and completes the declined turn$`, st.declineFirstTurn)
	sc.Step(`^the host sends the follow-up turn on the same Codex process and accepts the retry$`, st.acceptSecondTurn)
	sc.Step(`^Codex completes the second turn and logs one process start and exit$`, st.verifySingleProcessLifecycle)
	sc.Step(`^the host starts Codex and reads until the process exits mid-turn$`, st.runUntilMidTurnExit)
	sc.Step(`^the host resumes that Codex thread id with a new MCP token$`, st.resumeWithNewToken)
	sc.Step(`^the resumed Codex turn observes the new MCP token and completes$`, st.verifyResumedTurn)
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
	sc.Step(`^a scripted Codex session with user-input requests$`, st.configureUserInputScript)
	sc.Step(`^the host starts Codex and begins the user-input turn$`, func() error {
		return st.startAndBeginTurn("", "tok-ui", "Ask the owner.")
	})
	sc.Step(`^Codex asks user-input questions that the host answers and leaves unanswered$`, st.handleUserInputQuestions)
}

func (st *codexAcceptanceState) configureScript(name string) error {
	script, err := filepath.Abs(filepath.Join("..", "internal", "host", "fakecli", "testdata", name))
	if err != nil {
		return err
	}
	return st.configureScriptPath(script)
}

func (st *codexAcceptanceState) configureScriptPath(script string) error {
	if st.logPath == "" {
		st.logPath = filepath.Join(st.f.t.TempDir(), "codex-lifecycle.log")
		if err := setScenarioEnv(st.undo, "SOLODAWN_FAKECLI_LOG", st.logPath); err != nil {
			return err
		}
	}
	return setScenarioEnv(st.undo, "SOLODAWN_FAKE_CODEX_SCRIPT", script)
}

func (st *codexAcceptanceState) baseLaunch(session, token string) host.Launch {
	exe, _ := os.Executable()
	return host.Launch{
		Folder:       st.f.t.TempDir(),
		Session:      session,
		Level:        2,
		Shim:         []string{exe, "codex-mcp-shim", "--token", token},
		Instructions: "Group instructions.",
		ToolTimeout:  300 * time.Second,
	}
}

func (st *codexAcceptanceState) startMCPResponder() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	st.undo(func() { ln.Close() })
	if err := setScenarioEnv(st.undo, "SOLODAWN_ACCEPTANCE_MCP_PEER", ln.Addr().String()); err != nil {
		return err
	}
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

func (st *codexAcceptanceState) declineFirstTurn() error {
	ask, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	st.firstAsk = ask
	if err := st.proc.Answer(ask.ID, false); err != nil {
		return err
	}
	return readCompletedTurn(st.proc.Events())
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
	return readCompletedTurn(st.proc.Events())
}

func (st *codexAcceptanceState) verifySingleProcessLifecycle() error {
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

func (st *codexAcceptanceState) verifyResumedTurn() error {
	if err := readCompletedTurn(st.proc.Events()); err != nil {
		return err
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
	for _, item := range []string{wantID, "--no-alt-screen", "--no-daemon", "features.default_mode_request_user_input=true", "workspace-write", "on-request", "project_doc_max_bytes=0"} {
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

func readCompletedTurn(ch <-chan host.Event) error {
	ev, err := readUntilSignificant(ch)
	if err != nil {
		return err
	}
	if ev.Kind != host.TurnEnded || ev.Status != "completed" {
		return fmt.Errorf("unexpected turn end %+v", ev)
	}
	return nil
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
		if err != nil || (ev.Kind != host.Active && ev.Kind != host.Used) {
			return ev, err
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

func (st *codexAcceptanceState) configureUserInputScript() error {
	const rawScript = `{
  "session": "thr-ui",
  "steps": [
    {"dir":"stdin","msg":{"method":"initialize"}},
    {"dir":"stdout","msg":{"id":"$id","result":{}}},
    {"dir":"stdin","msg":{"method":"initialized"}},
    {"dir":"stdin","msg":{"method":"thread/start"}},
    {"dir":"stdout","msg":{"id":"$id","result":{"thread":{"id":"$session"}}}},
    {"dir":"stdin","msg":{"method":"turn/start","params":{"threadId":"$session"}}},
    {"dir":"stdout","msg":{"id":"$id","result":{"turn":{"id":"turn-ui"}}}},
    {"dir":"stdout","msg":{"id":10,"method":"item/tool/requestUserInput","params":{"questions":[]}}},
    {"dir":"stdin","msg":{"id":10,"error":{"code":-32601}}},
    {"dir":"stdout","msg":{"id":11,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q1","header":"DB","question":"Which database?","isSecret":false,"options":[{"label":"SQLite","description":"Local file"}]}]}}},
    {"dir":"stdin","msg":{"id":11,"result":{"answers":{"q1":{"answers":["SQLite"]}}}}},
    {"dir":"stdout","msg":{"id":12,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"q2","question":"Extra note?"}]}}},
    {"dir":"stdin","msg":{"id":12,"result":{"answers":{}}}},
    {"dir":"stdout","msg":{"method":"turn/completed","params":{"turn":{"id":"turn-ui","status":"completed","error":null}}}},
    {"dir":"eof"}
  ]
}`
	path := filepath.Join(st.f.t.TempDir(), "codex-ui.json")
	if err := os.WriteFile(path, []byte(rawScript), 0600); err != nil {
		return err
	}
	return st.configureScriptPath(path)
}

func (st *codexAcceptanceState) handleUserInputQuestions() error {
	if err := st.answerUserInputQuestions(); err != nil {
		return err
	}
	if err := readCompletedTurn(st.proc.Events()); err != nil {
		return err
	}
	return st.verifySingleProcessLifecycle()
}

func (st *codexAcceptanceState) answerUserInputQuestions() error {
	ask1, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	if err := replyFirstUserInputAsk(ask1); err != nil {
		return err
	}
	ask2, err := readAskedEvent(st.proc.Events())
	if err != nil {
		return err
	}
	return ask2.Reply(nil)
}

func replyFirstUserInputAsk(ask *host.Ask) error {
	if ask.Tool != "requestUserInput" || ask.Reply == nil {
		return fmt.Errorf("unexpected first user-input ask: %+v", ask)
	}
	if err := ask.Reply(map[string][]string{"q1": {"SQLite"}}); err != nil {
		return err
	}
	if ask.Reply(map[string][]string{"q1": {"SQLite"}}) == nil {
		return errors.New("duplicate user-input Reply succeeded")
	}
	return nil
}
