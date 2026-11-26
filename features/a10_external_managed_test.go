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
	workspace, err := aiWorkspace(w.root, w.groupID, name)
	if err != nil {
		return err
	}
	set := proto.AgentSet{HostCommand: w.header("agent.set"), Agent: agent, Account: "personal"}
	join := proto.HostJoin{HostCommand: w.header("host.join"), Group: w.groupID, Home: w.homeNode.key.ID(),
		Agent: agent, Name: name, Secretary: secretary, Workspace: workspace}
	return s3Run(func() error { return w.hostCommand(set) }, func() error { return w.hostCommand(join) },
		func() error { return w.waitState(func() bool { return w.aiByName[name].ID != "" }) })
}

func (w *externalWorld) scriptClaude(body []map[string]any) error {
	steps := append(claudeToolTurn(true, "", "", body), map[string]any{"dir": "eof"})
	return writeM6Script(w.homeNode, "claude", map[string]any{"steps": steps})
}

func (w *externalWorld) scriptNotice() error {
	steps := slices.Concat(claudeToolTurn(true, "", "", mcpPostSteps(1, "Interruption seen")),
		claudeToolTurn(false, "", "", nil), []map[string]any{{"dir": "eof"}})
	return writeM6Script(w.homeNode, "claude", map[string]any{"steps": steps})
}

func (w *externalWorld) askManaged() error {
	return s3Run(func() error { return w.joinManaged("M", false) },
		func() error { return w.setHold("group", "pause") }, w.askAndAnswer)
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
	steps := mcpCallSteps(1, "reply", map[string]any{"call": call.ID, "text": answer}, map[string]any{"isError": false})
	return s3Run(func() error { return w.scriptClaude(steps) }, func() error { return w.setHold("group", "resume") })
}

func (w *externalWorld) managedTools() error {
	if err := w.joinSecretary(); err != nil {
		return err
	}
	return w.scriptManagedToolCheck()
}

func (w *externalWorld) scriptManagedToolCheck() error {
	body := mcpCallSteps(1, "report_usage", map[string]any{"window_pct": 40}, nil)
	body[1]["msg"] = map[string]any{"jsonrpc": "2.0", "id": 1,
		"error": map[string]any{"code": -32602, "message": `unknown tool "report_usage"`}}
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
	return w.secretaryTurn("waiting", usage)
}

const externalRosterSkills = "Rust, reviews"

func (w *externalWorld) setRosterSkills() error {
	cmd := proto.MemberSet{MemberCommand: proto.MemberCommand{HomeCommand: w.homeHeader("member"),
		Op: "set", ID: proto.ID(w.aiByName["X"].ID)}, ProfileSet: proto.ProfileSet{Skills: new(externalRosterSkills)}}
	return s3Run(func() error { return w.hostCommand(cmd) },
		func() error {
			return w.waitState(func() bool { return w.aiByName["X"].Skills == externalRosterSkills })
		})
}

func (w *externalWorld) secretaryTurn(state, usage string) error {
	return s3Run(w.ensureSecretary, w.setRosterSkills, w.mentionSecretary,
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
	x := w.aiByName["X"]
	prefix := fmt.Sprintf(`- "X" · external (ZCode) on %s · %s · standard · %s`, x.HostName, x.Workspace, state)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if externalRosterLine(scanner.Bytes(), prefix, usage) {
			return nil
		}
	}
	return fmt.Errorf("no secretary turn contained X's roster line %q including %q (%v)", prefix, usage, scanner.Err())
}

func externalRosterLine(raw []byte, prefix, usage string) bool {
	var input struct {
		Message struct{ Content []struct{ Text string } }
	}
	if json.Unmarshal(raw, &input) != nil {
		return false
	}
	for _, content := range input.Message.Content {
		for _, line := range strings.Split(content.Text, "\n") {
			if externalRosterMatches(line, prefix, usage) {
				return true
			}
		}
	}
	return false
}

func externalRosterMatches(line, prefix, usage string) bool {
	want := prefix
	if usage != "" {
		want += " · " + usage
	}
	return line == want+" · "+externalRosterSkills
}

func (w *externalWorld) runBrief(role string) error {
	return s3Run(func() error { return w.startBrief(role) }, func() error { return w.handleBriefRun(role) })
}

func (w *externalWorld) handleBriefRun(role string) error {
	if role == "reviewer" {
		return s3Run(w.awaitBriefHandout, func() error { return w.handout("review", "External work") }, w.reviewElsewhere)
	}
	return s3Run(w.awaitBriefHandout, func() error { return w.handout("task", "External work") }, w.workInFolder,
		func() error { return w.tool("submit_task", `{"summary":"External task complete"}`, "ok") },
		func() error { return w.end(w.run.Run) })
}

// workInFolder checks that X's task run is in the folder X was added with
// and carries the brief, and leaves X's work there.
func (w *externalWorld) workInFolder() error {
	if w.run.Folder != w.aiByName["X"].Workspace || !strings.Contains(w.run.Context, externalCriteria) {
		return fmt.Errorf("task run in %q with context %q, want X's folder %q and the brief's criteria", w.run.Folder, w.run.Context, w.aiByName["X"].Workspace)
	}
	return os.WriteFile(filepath.Join(w.run.Folder, "external.txt"), []byte("External result\n"), 0o600)
}

// reviewElsewhere checks that X's review run is in X's own folder and says
// where the managed AI's work is, then posts a passing review.
func (w *externalWorld) reviewElsewhere() error {
	m := w.aiByName["M"]
	where := fmt.Sprintf("Work: by %q on %s, in %s\nThe work is in another folder on this computer.", m.Name, m.HostName, m.Workspace)
	if w.run.Folder != w.aiByName["X"].Workspace || !strings.Contains(w.run.Text, where) {
		return fmt.Errorf("review run in %q with text %q, want X's folder %q and %q", w.run.Folder, w.run.Text, w.aiByName["X"].Workspace, where)
	}
	return s3Run(func() error { return w.tool("submit_review", `{"score":100,"items":[]}`, "ok") },
		func() error { return w.end(w.run.Run) })
}

func (w *externalWorld) taskDone() error {
	return w.waitExternalPush("the external task to be done", func() bool { return w.currentTask().State == "done" })
}

func (w *externalWorld) startBrief(role string) error {
	return s3Run(w.joinSecretary, func() error { return w.joinManaged("M", false) },
		func() error { return w.prepareBrief(role) })
}

func (w *externalWorld) takeBrief() error {
	return s3Run(func() error { return w.startBrief("assignee") }, w.awaitBriefHandout,
		func() error { return w.handout("task", "External work") })
}

func (w *externalWorld) inTaskThread(kind, text string) error {
	return w.waitState(func() bool {
		for _, msg := range w.msgs {
			if msg.By == w.aiByName["X"].ID && msg.Kind == kind && msg.Text == text {
				return msg.Thread != nil && *msg.Thread == w.taskID
			}
		}
		return false
	})
}

func (w *externalWorld) prepareBrief(role string) error {
	assignee, reviewer := "X", "M"
	if role == "reviewer" {
		assignee, reviewer = reviewer, assignee
	}
	if err := w.scriptManagedBriefWork(role); err != nil {
		return err
	}
	return s3Run(func() error { return w.scriptBrief(assignee, reviewer) },
		func() error { return w.whenOwnerMentionsAI("Secretary", "Propose the external brief") },
		w.confirmBrief)
}

func (w *externalWorld) scriptBrief(assignee, reviewer string) error {
	args := map[string]any{
		"title": "External brief", "goal": "Exercise external task work", "criteria": []string{externalCriteria},
		"scope_in": []string{"project"}, "scope_out": []string{},
		"tasks": []map[string]string{{"title": "External work", "assignee": assignee, "reviewer": reviewer}},
	}
	return w.scriptClaude(mcpCallSteps(1, "propose_brief", args, map[string]any{"isError": false}))
}

func (w *externalWorld) confirmBrief() error {
	var id proto.ID
	err := w.waitExternalPush("secretary's brief", func() bool {
		for _, msg := range w.msgs {
			if msg.Kind == "brief" && msg.By == w.aiByName["Secretary"].ID {
				id = proto.ID(msg.ID)
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	if err := w.hostCommand(proto.Act{HomeCommand: w.homeHeader("act"), Msg: id, Action: "confirm"}); err != nil {
		return err
	}
	return w.waitExternalPush("confirmed external task", func() bool {
		task := w.tasksByNum[1]
		w.taskID = task.ID
		return task.ID != ""
	})
}

func (w *externalWorld) scriptManagedBriefWork(role string) error {
	script := m5CodexScript("external-managed-review", m5PassingReview(100))
	if role == "reviewer" {
		script = m5CodexScript("external-managed-task", m5WorkerCalls("Managed task complete"))
	}
	return os.WriteFile(w.homeNode.scriptPath("codex"), []byte(script), 0o600)
}

func (w *externalWorld) awaitBriefHandout() error {
	w.startWait()
	wait := newWait(10*time.Second, "external brief handout", nil)
	defer wait.stop()
	got, err := recvWait(wait, w.wait)
	if err != nil {
		return err
	}
	return w.readHandout(got)
}

func (w *externalWorld) waitForManagedNotice(reason string) error {
	return waitUntil(5*time.Second, fmt.Sprintf("secretary's %q notice", reason), func() error {
		f, err := os.ReadFile(w.turnsPath())
		if err != nil {
			return abort(err)
		}
		if !strings.Contains(string(f), fmt.Sprintf("notice · information\\nNotice (%s)", reason)) {
			return fmt.Errorf("secretary's input: %s", f)
		}
		return nil
	})
}
