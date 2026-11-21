//go:build acceptance

package features

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
)

func init() {
	stepSets["host_claude"] = hostClaudeSteps
}

func TestAcceptanceClaudeMCPShim(t *testing.T) {
	addr := os.Getenv("SOLODAWN_ACCEPTANCE_CLAUDE_MCP")
	if addr == "" {
		return
	}
	idx := slices.Index(os.Args, "--")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprintln(conn, strings.Join(os.Args[idx+1:], " "))
	go io.Copy(conn, os.Stdin)
	io.Copy(os.Stdout, conn)
	os.Exit(0)
}

type claudeAcceptanceState struct {
	t        *testing.T
	cleanup  func(func())
	dir      string
	logPath  string
	session  string
	proc     host.Process
	events   []host.Event
	mcpCalls []string
	mcpErr   <-chan error
}

func hostClaudeSteps(sc *godog.ScenarioContext, f fixtures) {
	st := &claudeAcceptanceState{t: f.t, cleanup: afterScenario(sc)}
	sc.Step(`^a scripted Claude Code session with two turns that call "post"$`, st.givenTwoTurnPostScript)
	sc.Step(`^the Claude driver starts a fresh session and sends both turns$`, st.whenStartAndSendBothTurns)
	sc.Step(`^the Claude driver reports Ready once per turn and two completed TurnEnded events$`, st.thenTwoCompletedTurns)
	sc.Step(`^the MCP shim received both "post" calls with the start's token$`, st.thenMCPReceivedBothPosts)
	sc.Step(`^closing the Claude process records one start-exit pair in the lifecycle log$`, st.thenOneStartExitPair)
	sc.Step(`^a scripted Claude Code session that exits mid-turn before a result$`, st.givenMidTurnExitScript)
	sc.Step(`^the Claude driver starts a fresh session and sends a turn$`, st.whenStartAndSendOneTurn)
	sc.Step(`^the Claude driver reports Ready and closes Events without TurnEnded$`, st.thenClosedWithoutTurnEnded)
	sc.Step(`^the Claude driver resumes that session id with a new token and sends the next turn$`, st.whenResumeAndSendTurn)
	sc.Step(`^the resumed Claude session completes the turn and calls the MCP shim with the new token$`, st.thenResumedCompletedWithNewToken)
}

func (s *claudeAcceptanceState) setEnv(key, value string) {
	prev, had := os.LookupEnv(key)
	os.Setenv(key, value)
	s.cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

func (s *claudeAcceptanceState) prepareDir() {
	if s.dir != "" {
		return
	}
	s.dir = s.t.TempDir()
	s.logPath = filepath.Join(s.dir, "lifecycle.log")
	s.setEnv("SOLODAWN_FAKECLI_LOG", s.logPath)
}

func (s *claudeAcceptanceState) writeScript(name, steps string) error {
	s.prepareDir()
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, []byte(`{"steps":[`+steps+`]}`), 0600); err != nil {
		return err
	}
	s.setEnv("SOLODAWN_FAKE_CLAUDE_SCRIPT", path)
	return nil
}

func (s *claudeAcceptanceState) startMCP(token string, calls int) ([]string, error) {
	peer, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	s.cleanup(func() { peer.Close() })
	s.setEnv("SOLODAWN_ACCEPTANCE_CLAUDE_MCP", peer.Addr().String())
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	s.mcpCalls = nil
	// Single reader: scenario verification step reading <-s.mcpErr.
	done := make(chan error, 1)
	s.mcpErr = done
	go s.serveMCP(peer, calls, done)
	return []string{exe, "-test.run=^TestAcceptanceClaudeMCPShim$", "--", "mcp", "--token", token}, nil
}

func (s *claudeAcceptanceState) serveMCP(peer *net.TCPListener, calls int, done chan<- error) {
	peer.SetDeadline(time.Now().Add(10 * time.Second))
	conn, err := peer.Accept()
	if err != nil {
		done <- err
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := json.NewDecoder(conn)
	header, err := readAcceptanceLine(conn)
	if err != nil {
		done <- err
		return
	}
	for i := range calls {
		tool, err := answerAcceptanceMCPCall(conn, reader, i+1)
		if err != nil {
			done <- err
			return
		}
		s.mcpCalls = append(s.mcpCalls, header+"|"+tool)
	}
	done <- nil
}

func readAcceptanceLine(r io.Reader) (string, error) {
	var buf []byte
	var b [1]byte
	for {
		if _, err := r.Read(b[:]); err != nil {
			return "", err
		}
		if b[0] == '\n' {
			return string(buf), nil
		}
		buf = append(buf, b[0])
	}
}

func answerAcceptanceMCPCall(conn net.Conn, dec *json.Decoder, id int) (string, error) {
	var msg struct {
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := dec.Decode(&msg); err != nil {
		return "", err
	}
	fmt.Fprintf(conn, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"{\"id\":\"m1\"}"}],"isError":false}}`+"\n", id)
	return msg.Params.Name, nil
}

func (s *claudeAcceptanceState) givenTwoTurnPostScript() error {
	const steps = `
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"first mention"}]}}},
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},
		{"dir":"mcp<-cli","msg":{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post","arguments":{"text":"answer 1"}}}},
		{"dir":"mcp->cli","msg":{"jsonrpc":"2.0","id":1,"result":{}}},
		{"dir":"stdout","msg":{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1795615200,"rateLimitType":"seven_day","unifiedWindows":{"seven_day":{"utilization":0.25}}}}},
		{"dir":"stdout","msg":{"type":"result","subtype":"success","usage":{"input_tokens":10,"output_tokens":5},"model_usage_since_last_result":{"haiku":{"costUSD":0.001}}}},
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"second mention"}]}}},
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},
		{"dir":"mcp<-cli","msg":{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"post","arguments":{"text":"remembers first"}}}},
		{"dir":"mcp->cli","msg":{"jsonrpc":"2.0","id":2,"result":{}}},
		{"dir":"stdout","msg":{"type":"result","subtype":"success","usage":{"input_tokens":20,"output_tokens":8},"model_usage_since_last_result":{"haiku":{"costUSD":0.002}}}},
		{"dir":"eof"}`
	return s.writeScript("two-turns.json", steps)
}

func (s *claudeAcceptanceState) whenStartAndSendBothTurns() error {
	shim, err := s.startMCP("tok-start", 2)
	if err != nil {
		return err
	}
	proc, err := host.Drivers["claude"].Start(s.t.Context(), host.Launch{
		Folder: s.dir,
		Level:  1,
		Shim:   shim,
	})
	if err != nil {
		return err
	}
	s.proc = proc
	s.cleanup(func() { proc.Close() })
	for _, prompt := range []string{"first mention", "second mention"} {
		if err := proc.Turn(prompt); err != nil {
			return err
		}
		if err := s.collectUntilTurnEnd(); err != nil {
			return err
		}
	}
	return nil
}

func (s *claudeAcceptanceState) collectUntilTurnEnd() error {
	for ev := range s.proc.Events() {
		s.events = append(s.events, ev)
		if ev.Kind == host.Ready {
			s.session = ev.Session
		}
		if ev.Kind == host.TurnEnded {
			return nil
		}
	}
	return fmt.Errorf("Events closed before TurnEnded: %+v", s.events)
}

func isCompletedTurn(ev host.Event) bool {
	return ev.Kind == host.TurnEnded && ev.Status == "completed"
}

func (s *claudeAcceptanceState) thenTwoCompletedTurns() error {
	var readyCount, endedCount int
	for _, ev := range s.events {
		if ev.Kind == host.Ready {
			readyCount++
		}
		if isCompletedTurn(ev) {
			endedCount++
		}
	}
	got := [2]int{readyCount, endedCount}
	if s.session == "" || got != [2]int{2, 2} {
		return fmt.Errorf("session=%q counts=%v events=%+v", s.session, got, s.events)
	}
	return nil
}

func (s *claudeAcceptanceState) thenMCPReceivedBothPosts() error {
	if err := <-s.mcpErr; err != nil {
		return err
	}
	want := []string{"mcp --token tok-start|post", "mcp --token tok-start|post"}
	if !slices.Equal(s.mcpCalls, want) {
		return fmt.Errorf("MCP calls %q, want %q", s.mcpCalls, want)
	}
	return nil
}

func (s *claudeAcceptanceState) thenOneStartExitPair() error {
	if err := s.proc.Close(); err != nil {
		return err
	}
	for range s.proc.Events() {
	}
	got, err := os.ReadFile(s.logPath)
	want := fmt.Sprintf("start claude %s\nexit claude %s\n", s.session, s.session)
	if err != nil || string(got) != want {
		return fmt.Errorf("lifecycle log %q (%v), want %q", got, err, want)
	}
	return nil
}

func (s *claudeAcceptanceState) givenMidTurnExitScript() error {
	const steps = `
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"first turn"}]}}},
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},
		{"dir":"stdout","msg":{"type":"assistant"}}`
	return s.writeScript("crash.json", steps)
}

func (s *claudeAcceptanceState) whenStartAndSendOneTurn() error {
	proc, err := host.Drivers["claude"].Start(s.t.Context(), host.Launch{
		Folder: s.dir,
		Level:  1,
		Shim:   []string{"solodawn", "mcp", "--token", "tok-crash"},
	})
	if err != nil {
		return err
	}
	s.proc = proc
	s.cleanup(func() { proc.Close() })
	if err := proc.Turn("first turn"); err != nil {
		return err
	}
	for ev := range proc.Events() {
		s.events = append(s.events, ev)
		if ev.Kind == host.Ready {
			s.session = ev.Session
		}
	}
	return nil
}

func (s *claudeAcceptanceState) thenClosedWithoutTurnEnded() error {
	for _, ev := range s.events {
		if ev.Kind == host.TurnEnded {
			return fmt.Errorf("unexpected TurnEnded on mid-turn exit: %+v", s.events)
		}
	}
	if s.session == "" {
		return fmt.Errorf("missing Ready session before exit: %+v", s.events)
	}
	return nil
}

func (s *claudeAcceptanceState) whenResumeAndSendTurn() error {
	const steps = `
		{"dir":"stdin","msg":{"type":"user","message":{"role":"user","content":[{"type":"text","text":"next mention"}]}}},
		{"dir":"stdout","msg":{"type":"system","subtype":"init","session_id":"$session"}},
		{"dir":"mcp<-cli","msg":{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post","arguments":{"text":"resumed ok"}}}},
		{"dir":"mcp->cli","msg":{"jsonrpc":"2.0","id":1,"result":{}}},
		{"dir":"stdout","msg":{"type":"result","subtype":"success","usage":{"input_tokens":8,"output_tokens":4},"model_usage_since_last_result":{"haiku":{"costUSD":0.001}}}},
		{"dir":"eof"}`
	if err := s.writeScript("resumed.json", steps); err != nil {
		return err
	}
	shim, err := s.startMCP("tok-resumed", 1)
	if err != nil {
		return err
	}
	proc, err := host.Drivers["claude"].Start(s.t.Context(), host.Launch{
		Folder:  s.dir,
		Session: s.session,
		Level:   1,
		Shim:    shim,
	})
	if err != nil {
		return err
	}
	s.proc = proc
	s.cleanup(func() { proc.Close() })
	s.events = nil
	if err := proc.Turn("next mention"); err != nil {
		return err
	}
	return s.collectUntilTurnEnd()
}

func (s *claudeAcceptanceState) thenResumedCompletedWithNewToken() error {
	if err := <-s.mcpErr; err != nil {
		return err
	}
	if err := s.proc.Close(); err != nil {
		return err
	}
	for range s.proc.Events() {
	}
	last := s.events[len(s.events)-1]
	if !isCompletedTurn(last) {
		return fmt.Errorf("unexpected final event: %+v", last)
	}
	wantCalls := []string{"mcp --token tok-resumed|post"}
	if !slices.Equal(s.mcpCalls, wantCalls) {
		return fmt.Errorf("calls=%q, want %q", s.mcpCalls, wantCalls)
	}
	return nil
}
