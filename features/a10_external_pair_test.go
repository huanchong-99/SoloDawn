//go:build acceptance

package features

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host/fakecli"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// With this variable the test binary, started as codex, is the managed AI's
// CLI. Every session of the managed AI runs in its workspace, so it picks its
// script by the order of its sessions there: the first is its task, the
// second its review. The scenario makes that order.
const externalPairDir = "SOLODAWN_EXTERNAL_TEST_PAIR"

func init() {
	dir := os.Getenv(externalPairDir)
	if dir == "" || m5ProgName(os.Args[0]) != "codex" {
		return
	}
	if err := externalPairRole(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(fakecli.Run("codex", "codex-cli 0.164.0", os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// externalPairRole notes the folder of a session of the managed AI, selects
// its script, and leaves the work of its task in the folder. A start in any
// other folder is the host probing the CLI.
func externalPairRole(dir string) error {
	cwd, err := externalPairFolder(dir)
	if err != nil || cwd == "" {
		return err
	}
	role := externalPairNext(dir)
	if err := externalPairStart(dir, role, cwd); err != nil || role == "review" {
		return err
	}
	return os.WriteFile("managed.txt", []byte("Managed result\n"), 0o600)
}

// externalPairFolder is the folder the CLI runs in when that is the managed
// AI's workspace, else "". Before the scenario has noted the workspace, every
// start is a probe.
func externalPairFolder(dir string) (string, error) {
	workspace, err := os.ReadFile(filepath.Join(dir, "workspace"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	cwd, err := os.Getwd()
	if err != nil || cwd != string(workspace) {
		return "", err
	}
	return cwd, nil
}

// externalPairNext is the role of a session of the managed AI: its first
// session is its task, its second its review.
func externalPairNext(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "folders")); err == nil {
		return "review"
	}
	return "task"
}

func externalPairStart(dir, role, cwd string) error {
	if err := os.Setenv("SOLODAWN_FAKE_CODEX_SCRIPT", filepath.Join(dir, role+".json")); err != nil {
		return err
	}
	folders, err := os.OpenFile(filepath.Join(dir, "folders"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	fmt.Fprintln(folders, cwd)
	return folders.Close()
}

func externalPairSteps(sc *godog.ScenarioContext, w *externalWorld) {
	p := &externalPair{externalWorld: w}
	sc.Step(`^a project with external X and a managed AI that works and reviews$`, p.setup)
	sc.Step(`^the owner confirms a brief of two tasks that X and the managed AI do and review for each other$`, p.confirm)
	sc.Step(`^X works on its task while the review of the other waits, and X's next run is that review$`, p.twoRuns)
	sc.Step(`^both external tasks are done with two summary cards, each assignee's work is in its own folder, and the owner saw no card but the brief$`, p.done)
	sc.Step(`^every session ran in the folder of its AI, the task sessions are gone and the node's data directory holds no project folder$`, p.cleaned)
}

type externalPair struct {
	*externalWorld
	dir     string
	folders []string // of X's runs
}

func (p *externalPair) setup() error {
	return s3Run(p.prepareContext, p.prepareRoot, p.prepareClaude, p.prepareCodex, p.startNode, p.createAndSyncGroup,
		func() error { return p.join("X", "api_key", "ZCode") },
		p.joinSecretary, func() error { return p.joinManaged("M", false) }, p.noteManagedWorkspace)
}

func (p *externalPair) prepareCodex() error {
	p.dir = filepath.Join(p.root, "pair")
	if err := os.Mkdir(p.dir, 0o700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := m5CopyExec(exe, filepath.Join(p.dir, "codex"+m5ExeExt())); err != nil {
		return err
	}
	return s3Run(func() error { return setScenarioEnv(p.cleanup, externalPairDir, p.dir) },
		func() error {
			return setScenarioEnv(p.cleanup, "PATH", p.dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		})
}

func (p *externalPair) noteManagedWorkspace() error {
	return os.WriteFile(filepath.Join(p.dir, "workspace"), []byte(p.aiByName["M"].Workspace), 0o600)
}

func (p *externalPair) confirm() error {
	return s3Run(p.writeScripts,
		func() error { return p.whenOwnerMentionsAI("Secretary", "Propose the external brief") }, p.confirmBrief,
		func() error {
			return p.waitState(func() bool { return p.hasTextPost("Secretary", "Confirmation seen", "") })
		})
}

func (p *externalPair) writeScripts() error {
	work := m5CodexScript("external-pair-task", m5WorkerCalls("Managed task complete"))
	review := m5CodexScript("external-pair-review", m5PassingReview(95))
	return s3Run(func() error { return os.WriteFile(filepath.Join(p.dir, "task.json"), []byte(work), 0o600) },
		func() error { return os.WriteFile(filepath.Join(p.dir, "review.json"), []byte(review), 0o600) },
		func() error {
			return writeM6Script(p.homeNode, "claude", map[string]any{"steps": externalPairSecretary()})
		})
}

// externalPairSecretary proposes the brief, then takes one notice a turn: the
// confirmation and the two done tasks.
func externalPairSecretary() []map[string]any {
	args := map[string]any{
		"title": "External pair", "goal": "Exercise external task work", "criteria": []string{externalCriteria},
		"scope_in": []string{"project"}, "scope_out": []string{},
		"tasks": []map[string]string{
			{"title": "External work", "assignee": "X", "reviewer": "M"},
			{"title": "Managed work", "assignee": "M", "reviewer": "X"},
		},
	}
	ok := map[string]any{"isError": false}
	summary := func(id int, task string) []map[string]any {
		return mcpCallSteps(id, "post", map[string]any{"kind": "summary", "task": task, "text": task + " done"}, ok)
	}
	return slices.Concat(claudeToolTurn(true, "", "", mcpCallSteps(1, "propose_brief", args, ok)),
		claudeToolTurn(false, "", "", mcpPostSteps(2, "Confirmation seen")),
		claudeToolTurn(false, "", "", summary(3, "T-1")), claudeToolTurn(false, "", "", summary(4, "T-2")),
		[]map[string]any{{"dir": "eof"}})
}

// twoRuns: the managed AI submits its task first, so that its review of X's
// task is its second session.
func (p *externalPair) twoRuns() error {
	return s3Run(func() error { return p.take(1, "task", "External work") },
		func() error { return p.waitTask(2, func(t proto.TaskRow) bool { return t.Step == "review" }) },
		func() error { return p.inbox(1) }, func() error { return p.look(3) },
		p.workInFolder,
		func() error { return p.tool("submit_task", `{"summary":"External task complete"}`, "ok") },
		func() error { return p.end(p.run.Run) },
		func() error { return p.awaitSummary(1) },
		func() error { return p.take(2, "review", "Managed work") }, p.reviewElsewhere,
		func() error { return p.awaitSummary(2) })
}

func (p *externalPair) take(num int, kind, title string) error {
	p.taskID = p.tasksByNum[num].ID
	err := s3Run(p.awaitBriefHandout, func() error { return p.handout(kind, title) })
	p.folders = append(p.folders, p.run.Folder)
	return err
}

func (p *externalPair) waitTask(num int, ready func(proto.TaskRow) bool) error {
	return p.waitState(func() bool { return ready(p.tasksByNum[num]) })
}

func (p *externalPair) awaitSummary(num int) error {
	return p.waitState(func() bool { return len(p.summaries()) == num && p.tasksByNum[num].State == "done" })
}

func (p *externalPair) summaries() []proto.MsgRow {
	var cards []proto.MsgRow
	for _, id := range p.msgOrder {
		if msg := p.msgs[id]; msg.Kind == "summary" && msg.Main {
			cards = append(cards, msg)
		}
	}
	return cards
}

func (p *externalPair) done() error {
	return s3Run(p.onlyBriefCard, p.secretarySummaries, p.workInFolders)
}

func (p *externalPair) onlyBriefCard() error {
	for _, msg := range p.msgs {
		if slices.Contains([]string{"approval", "confirm", "decision"}, msg.Kind) {
			return fmt.Errorf("unexpected %s card %+v", msg.Kind, msg)
		}
	}
	return nil
}

func (p *externalPair) secretarySummaries() error {
	var by []string
	for _, card := range p.summaries() {
		by = append(by, card.By)
	}
	if sec := p.aiByName["Secretary"].ID; !slices.Equal(by, []string{sec, sec}) {
		return fmt.Errorf("summary cards by %v, want two by the secretary %s", by, sec)
	}
	return nil
}

// workInFolders checks that each task left its work in its assignee's
// workspace and nowhere else.
func (p *externalPair) workInFolders() error {
	for _, f := range []struct{ ai, file, other string }{{"X", "external.txt", "M"}, {"M", "managed.txt", "X"}} {
		if _, err := os.Stat(filepath.Join(p.aiByName[f.ai].Workspace, f.file)); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(p.aiByName[f.other].Workspace, f.file)); !os.IsNotExist(err) {
			return fmt.Errorf("%s is in the folder of %s too (%v)", f.file, f.other, err)
		}
	}
	return nil
}

func (p *externalPair) cleaned() error {
	raw, err := os.ReadFile(filepath.Join(p.dir, "folders"))
	if err != nil {
		return err
	}
	managed := strings.Split(string(bytes.TrimSpace(raw)), "\n")
	x, m := p.aiByName["X"].Workspace, p.aiByName["M"].Workspace
	if !slices.Equal(p.folders, []string{x, x}) || !slices.Equal(managed, []string{m, m}) {
		return fmt.Errorf("folders of X's runs %q and of the managed AI's sessions %q, want X's %q twice and the managed AI's %q twice", p.folders, managed, x, m)
	}
	err = waitUntil(5*time.Second, "the task sessions to go", func() error {
		sessions, err := p.taskSessions()
		if err != nil || len(sessions) == 0 {
			return err
		}
		return fmt.Errorf("left: %q", sessions)
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(p.homeNode.data, "projects")); !os.IsNotExist(err) {
		return fmt.Errorf("the node's data directory holds a project folder (%v)", err)
	}
	return nil
}

// taskSessions reads the session rows of the group's tasks from the home's
// database.
func (p *externalPair) taskSessions() ([]string, error) {
	db, err := store.Open(filepath.Join(p.homeNode.data, "node.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var sessions []string
	err = db.Write(func(tx *store.Tx) error {
		rows, err := tx.Query("SELECT member, task FROM session WHERE g = ? AND task <> ''", string(p.groupID))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var member, task string
			if err := rows.Scan(&member, &task); err != nil {
				return err
			}
			sessions = append(sessions, "session of "+member+" on task "+task)
		}
		return rows.Err()
	})
	return sessions, err
}
