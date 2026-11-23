//go:build acceptance

package features

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	path := os.Getenv("SOLODAWN_EXTERNAL_TEST_TURNS")
	if path == "" || m5ProgName(os.Args[0]) != "claude" {
		return
	}
	os.Exit(externalFakeClaude(path))
}

func externalFakeClaude(path string) int {
	turns, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer turns.Close()
	return fakecli.Run("claude", "2.1.142", os.Args[1:], io.TeeReader(os.Stdin, turns), os.Stdout, os.Stderr)
}

func externalManagedSteps(sc *godog.ScenarioContext, w *externalWorld) {
	sc.Step(`^X asks the managed member and gets its scripted answer$`, w.askManaged)
	sc.Step(`^a managed member is not offered report_usage$`, w.managedTools)
	sc.Step(`^the external secretary's next turn lists X with "([^"]+)"$`, w.secretaryUsage)
	sc.Step(`^the external secretary's next turn lists X as offline$`, func() error { return w.secretaryTurn("offline", "") })
}

func (w *externalWorld) prepareClaude() error {
	bin := filepath.Join(w.root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := m5CopyExec(exe, filepath.Join(bin, "claude"+m5ExeExt())); err != nil {
		return err
	}
	return s3Run(func() error {
		return setScenarioEnv(w.cleanup, "PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	},
		func() error { return setScenarioEnv(w.cleanup, "SOLODAWN_EXTERNAL_TEST_TURNS", w.turnsPath()) })
}

func (w *externalWorld) turnsPath() string { return filepath.Join(w.root, "managed-input.jsonl") }

func (w *externalWorld) joinSecretary() error { return w.joinManaged("Secretary", true) }

func (w *externalWorld) joinManaged(name string, secretary bool) error {
	agent := "claude"
	if name == "M" && w.aiByName["Secretary"].ID != "" {
		agent = "codex"
	}
	set := proto.AgentSet{HostCommand: w.header("agent.set"), Agent: agent, Account: "personal"}
	join := proto.HostJoin{HostCommand: w.header("host.join"), Group: w.groupID, Home: w.homeNode.key.ID(),
		Agent: agent, Name: name, Secretary: secretary}
	return s3Run(func() error { return w.hostCommand(set) }, func() error { return w.hostCommand(join) },
		func() error { return w.waitState(func() bool { return w.aiByName[name].ID != "" }) })
}

func (w *externalWorld) scriptClaude(body []map[string]any) error {
	steps := append(claudeToolTurn(true, "", "", body), map[string]any{"dir": "eof"})
	return writeM6Script(w.homeNode, "claude", map[string]any{"steps": steps})
}

func (w *externalWorld) scriptNotice() error {
	return w.scriptClaude(mcpPostSteps(1, "Interruption seen"))
}

func (w *externalWorld) askManaged() error {
	return s3Run(func() error { return w.joinManaged("M", false) },
		func() error { return w.scriptClaude(nil) }, w.askAndAnswer)
}

func (w *externalWorld) askAndAnswer() error {
	const question, answer = "External question", "Managed answer"
	result := make(chan error, 1)
	go func() { result <- w.askTool(question, answer) }()
	var msg proto.MsgRow
	if err := w.waitState(func() bool { msg = w.findManagedQuestion(question); return msg.ID != "" }); err != nil {
		return err
	}
	if err := w.answerAsManaged(msg, answer); err != nil {
		return err
	}
	return <-result
}

func (w *externalWorld) askTool(question, answer string) error {
	res, err := w.call("ask", map[string]string{"to": "M", "text": question})
	if err != nil {
		return err
	}
	raw, err := externalResult(res)
	if err != nil {
		return err
	}
	var out struct{ Answer string }
	if json.Unmarshal(raw, &out) != nil || out.Answer != answer || res.IsError {
		return fmt.Errorf("ask result = %s, want answer %q", raw, answer)
	}
	return nil
}

func (w *externalWorld) findManagedQuestion(question string) proto.MsgRow {
	for _, m := range w.msgs {
		if m.Kind == "call" && m.By == w.aiByName["X"].ID && m.Text == question {
			return m
		}
	}
	return proto.MsgRow{}
}

func (w *externalWorld) answerAsManaged(call proto.MsgRow, answer string) error {
	h := w.homeHeader("post")
	h.As = proto.ID(w.aiByName["M"].ID)
	cmd := proto.Post[proto.EmptyBody]{HomeCommand: h, Kind: "reply", Text: answer, Re: new(proto.ID(call.ID)), Mentions: []proto.ID{}}
	return w.sendSignedCmd(w.homeLink.conn, w.homeNode.key, cmd)
}

func (w *externalWorld) managedTools() error {
	if err := w.joinSecretary(); err != nil {
		return err
	}
	return w.scriptManagedToolCheck()
}

func (w *externalWorld) scriptManagedToolCheck() error {
	body := mcpCallSteps(1, "report_usage", map[string]any{"window_pct": 40}, map[string]any{"isError": true})
	body = append(body, mcpPostSteps(2, "Managed tool check completed")...)
	steps := slices.Concat(claudeToolTurn(true, "", "", body),
		claudeToolTurn(false, "", "", mcpPostSteps(3, "Roster seen")),
		[]map[string]any{{"dir": "eof"}})
	return s3Run(func() error { return writeM6Script(w.homeNode, "claude", map[string]any{"steps": steps}) },
		func() error { return w.whenOwnerMentionsAI("Secretary", "Check managed tools") },
		func() error {
			return w.waitState(func() bool { return w.hasTextPost("Secretary", "Managed tool check completed", "") })
		})
}

func (w *externalWorld) secretaryUsage(usage string) error {
	cmd := proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: w.homeHeader("member"),
		Op: "set", ID: proto.ID(w.aiByName["X"].ID)}, ProfileSet: proto.ProfileSet{Skills: new("Rust, reviews")}}
	return s3Run(func() error { return w.hostCommand(cmd) }, func() error { return w.secretaryTurn("waiting", usage) })
}

func (w *externalWorld) secretaryTurn(state, usage string) error {
	return s3Run(w.ensureSecretary, w.mentionSecretary,
		func() error { return w.checkSecretaryInput(state, usage) })
}

func (w *externalWorld) ensureSecretary() error {
	if w.aiByName["Secretary"].ID != "" {
		return nil
	}
	return s3Run(w.joinSecretary, func() error { return w.scriptClaude(mcpPostSteps(1, "Roster seen")) })
}

func (w *externalWorld) mentionSecretary() error {
	return s3Run(func() error { return w.whenOwnerMentionsAI("Secretary", "Read the roster") },
		func() error { return w.waitState(func() bool { return w.hasTextPost("Secretary", "Roster seen", "") }) })
}

func (w *externalWorld) checkSecretaryInput(state, usage string) error {
	f, err := os.Open(w.turnsPath())
	if err != nil {
		return err
	}
	defer f.Close()
	prefix := fmt.Sprintf(`- "X" · external (ZCode) on %s · standard · %s`, w.aiByName["X"].HostName, state)
	suffix := ""
	if usage != "" {
		suffix = " · Rust, reviews"
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if externalRosterLine(scanner.Bytes(), prefix, usage, suffix) {
			return nil
		}
	}
	return fmt.Errorf("no secretary turn contained X's roster line %q including %q (%v)", prefix, usage, scanner.Err())
}

func externalRosterLine(raw []byte, prefix, usage, suffix string) bool {
	var input struct {
		Message struct{ Content []struct{ Text string } }
	}
	if json.Unmarshal(raw, &input) != nil {
		return false
	}
	for _, content := range input.Message.Content {
		for _, line := range strings.Split(content.Text, "\n") {
			if externalRosterMatches(line, prefix, usage, suffix) {
				return true
			}
		}
	}
	return false
}

func externalRosterMatches(line, prefix, usage, suffix string) bool {
	return strings.HasPrefix(line, prefix) && strings.Contains(line, usage) && strings.HasSuffix(line, suffix)
}

func (w *externalWorld) runBrief(role string) error {
	return s3Run(w.joinSecretary, func() error { return w.joinManaged("M", false) },
		func() error { return w.prepareBrief(role) }, func() error { return w.handleBriefRun(role) })
}

func (w *externalWorld) prepareBrief(role string) error {
	assignee, reviewer := "X", "M"
	if role == "reviewer" {
		assignee, reviewer = reviewer, assignee
	}
	if err := w.scriptManagedBriefWork(role); err != nil {
		return err
	}
	cmd := w.briefCommand(assignee, reviewer)
	return s3Run(func() error { return w.sendSignedCmd(w.homeLink.conn, w.homeNode.key, cmd) },
		func() error { return w.confirmBrief(cmd.Nonce) })
}

func (w *externalWorld) briefCommand(assignee, reviewer string) proto.Post[proto.BriefBody] {
	h := w.homeHeader("post")
	h.As = proto.ID(w.aiByName["Secretary"].ID)
	body := proto.BriefBody{Title: "External brief", Goal: "Exercise external task work", Criteria: []string{externalCriteria},
		Scope: proto.BriefScope{In: []string{"project"}, Out: []string{}}, Pass: 90, Requester: w.ownerKey.ID(),
		Tasks: []proto.BriefTask{{Title: "External work", Assignee: w.aiByName[assignee].ID, Reviewer: w.aiByName[reviewer].ID, Depends: []int{}}}}
	return proto.Post[proto.BriefBody]{HomeCommand: h, Kind: "brief", Text: "External brief", Mentions: []proto.ID{}, Body: body}
}

func (w *externalWorld) confirmBrief(id proto.ID) error {
	if err := w.hostCommand(proto.Act{HomeCommand: w.homeHeader("act"), Msg: id, Action: "confirm"}); err != nil {
		return err
	}
	return w.waitState(func() bool {
		task := w.tasksByNum[1]
		w.taskID = task.ID
		return task.ID != "" && task.Branch != ""
	})
}

func (w *externalWorld) scriptManagedBriefWork(role string) error {
	script := m5CodexScript("external-managed-review", m5PassingReview(100))
	if role == "reviewer" {
		script = m5CodexScript("external-managed-task", []m5ToolCallSpec{{Name: "run_checks", Args: map[string]any{}},
			{Name: "submit_task", Args: map[string]any{"summary": "Managed task complete"}}})
	}
	return os.WriteFile(w.homeNode.scriptPath("codex"), []byte(script), 0o600)
}

func (w *externalWorld) handleBriefRun(role string) error {
	if role == "reviewer" {
		return w.reviewBrief()
	}
	return s3Run(func() error { return w.look(0) }, func() error { return w.handout("task", "External brief") },
		w.commitExternalWork, func() error { return w.tool("run_checks", "{}", "ok") },
		func() error { return w.tool("submit_task", `{"summary":"External task complete"}`, "ok") },
		func() error { return w.end(w.run.Run) })
}

func (w *externalWorld) commitExternalWork() error {
	w.folder = w.run.Folder
	branch, err := m5GitOut(w.folder, "branch", "--show-current")
	if err != nil {
		return err
	}
	if branch != w.currentTask().Branch || !strings.Contains(w.run.Context, externalCriteria) {
		return fmt.Errorf("wrong task branch or missing brief criteria: branch %q, handout %+v", branch, w.run)
	}
	_, err = m5GitCommitFiles(w.folder, map[string]string{"external.txt": "External result\n"}, "External task work")
	return err
}

func (w *externalWorld) reviewBrief() error {
	return s3Run(func() error { w.startWait(); return w.finishWait(0) },
		func() error { return w.handout("review", "External work") }, w.submitExternalReview)
}

func (w *externalWorld) submitExternalReview() error {
	w.folder = w.run.Folder
	if _, err := os.Stat(filepath.Join(w.folder, ".solodawn-review.diff")); err != nil {
		return err
	}
	return s3Run(func() error { return w.tool("submit_review", `{"score":100,"items":[]}`, "ok") },
		func() error { return w.end(w.run.Run) })
}

func (w *externalWorld) waitForManagedNotice(reason string) error {
	_, err := waitFakeCLILogs(w.homeNode.cliLog(), "start claude ", 1, 5*time.Second)
	if err != nil {
		return err
	}
	f, err := os.ReadFile(w.turnsPath())
	if err != nil {
		return err
	}
	if !strings.Contains(string(f), "notice · information") || !strings.Contains(string(f), reason) {
		return fmt.Errorf("secretary's input did not include the %q notice: %s", reason, f)
	}
	return nil
}
