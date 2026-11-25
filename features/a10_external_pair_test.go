//go:build acceptance

package features

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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
// CLI: it picks its script by the folder it runs in, so that its task and its
// review may start in any order.
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

// externalPairRole selects the script of a task or review folder, notes the
// folder, and commits the work of a task.
func externalPairRole(dir string) error {
	// A worktree's .git is a file; elsewhere the host only probes the CLI.
	if git, err := os.Stat(".git"); err != nil || git.IsDir() {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(".solodawn-review.diff"); err == nil {
		return externalPairStart(dir, "review", cwd)
	}
	if err := externalPairStart(dir, "task", cwd); err != nil {
		return err
	}
	_, err = m5GitCommitFiles(cwd, map[string]string{"managed.txt": "Managed result\n"}, "Managed task work")
	return err
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
	sc.Step(`^a project with external X, a managed AI that works and reviews, and forge "([^"]+)"$`, p.setup)
	sc.Step(`^the owner confirms a brief of two tasks that X and the managed AI do and review for each other$`, p.confirm)
	sc.Step(`^X works on its task while the review of the other waits, and X's next run is that review$`, p.twoRuns)
	sc.Step(`^both external tasks are on the base with two summary cards, and the owner saw no card but the brief$`, p.onBase)
	sc.Step(`^each task and review had a folder of its own, and the folders and branches are gone$`, p.cleaned)
}

type externalPair struct {
	*externalWorld
	dir     string
	folders []string // of X's runs
}

func (p *externalPair) setup(forge string) error {
	return s3Run(p.prepareContext, p.prepareRootAndRepo, p.prepareClaude,
		func() error { return p.prepareForge(forge) }, p.prepareCodex, p.startNode,
		func() error { return p.createExternalGroup(forge) },
		func() error { return p.join("X", "api_key", "ZCode") },
		p.joinSecretary, func() error { return p.joinManaged("M", false) }, p.setBriefGitIdentity)
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

func (p *externalPair) confirm() error {
	return s3Run(p.writeScripts,
		func() error { return p.whenOwnerMentionsAI("Secretary", "Propose the external brief") }, p.confirmBrief,
		func() error {
			return p.waitState(func() bool { return p.hasTextPost("Secretary", "Confirmation seen", "") })
		})
}

func (p *externalPair) writeScripts() error {
	work := m5CodexScript("external-pair-task", m5WorkerCalls(p.group.Forge != "none", "Managed task complete"))
	review := m5CodexScript("external-pair-review", m5PassingReview(95))
	return s3Run(func() error { return os.WriteFile(filepath.Join(p.dir, "task.json"), []byte(work), 0o600) },
		func() error { return os.WriteFile(filepath.Join(p.dir, "review.json"), []byte(review), 0o600) },
		func() error {
			return writeM6Script(p.homeNode, "claude", map[string]any{"steps": externalPairSecretary()})
		})
}

// externalPairSecretary proposes the brief, then takes one notice a turn: the
// confirmation and the two merges.
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

func (p *externalPair) twoRuns() error {
	return s3Run(func() error { return p.take(1, "task", "External work") },
		func() error { return p.waitTask(2, func(t proto.TaskRow) bool { return t.Step == "review" }) },
		func() error { return p.inbox(1) }, func() error { return p.look(3) },
		p.commitExternalWork, p.pushExternalWork,
		func() error { return p.tool("run_checks", "{}", "ok") },
		func() error { return p.tool("submit_task", `{"summary":"External task complete"}`, "ok") },
		func() error { return p.end(p.run.Run) },
		func() error { return p.awaitSummary(1) },
		func() error { return p.take(2, "review", "Managed work") }, p.submitExternalReview,
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
	return p.waitState(func() bool { return len(p.summaries()) == num && p.tasksByNum[num].State == "merged" })
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

func (p *externalPair) onBase() error {
	return s3Run(p.onlyBriefCard, p.secretarySummaries, p.baseFiles)
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

func (p *externalPair) baseFiles() error {
	base := p.clone()
	if p.group.Forge != "none" {
		base = p.repoDir
	}
	tree, err := m5GitOut(base, "ls-tree", "--name-only", "refs/heads/main")
	if err != nil {
		return err
	}
	if files := strings.Fields(tree); !slices.Contains(files, "external.txt") || !slices.Contains(files, "managed.txt") {
		return fmt.Errorf("base of %s has %v, want the files of both tasks", base, files)
	}
	return nil
}

func (p *externalPair) clone() string {
	return filepath.Join(p.homeNode.data, "projects", string(p.groupID), "repo")
}

func (p *externalPair) cleaned() error {
	raw, err := os.ReadFile(filepath.Join(p.dir, "folders"))
	if err != nil {
		return err
	}
	all := slices.Concat(p.folders, strings.Split(string(bytes.TrimSpace(raw)), "\n"))
	if distinct := slices.Compact(sortedStrings(slices.Clone(all))); len(all) != 4 || len(distinct) != 4 {
		return fmt.Errorf("folders of X's runs, then of the managed AI's: %q, want four", all)
	}
	return waitUntil(5*time.Second, "task and review folders, branches and sessions to go", func() error {
		left, err := p.left(all)
		if err != nil || len(left) == 0 {
			return err
		}
		return fmt.Errorf("left: %q", left)
	})
}

// left names the folders, task branches and task sessions that are still
// there.
func (p *externalPair) left(folders []string) ([]string, error) {
	var left []string
	for _, d := range folders {
		if _, err := os.Stat(d); !errors.Is(err, fs.ErrNotExist) {
			left = append(left, "folder "+d)
		}
	}
	refs, err := m5GitOut(p.clone(), "for-each-ref", "--format=%(refname)", "refs/heads/task")
	if err != nil {
		return nil, err
	}
	for _, ref := range strings.Fields(refs) {
		left = append(left, "branch "+ref)
	}
	sessions, err := p.taskSessions()
	return slices.Concat(left, sessions), err
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
