//go:build acceptance

package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() { stepSets["m4_readonly"] = m4ReadOnlySteps }

// editWorld has one AI whose CLI is features/editcli: on every turn it edits
// README.md in its working directory unless the settings that SoloDawn
// started it with forbid that. What the real CLIs do with those settings is
// theirs, and is on the real-machine list.
type editWorld struct {
	m      *m3World
	m4     *m4World
	agent  string
	folder string // of the task session that tried last
}

func m4ReadOnlySteps(sc *godog.ScenarioContext, f fixtures) {
	w := &editWorld{}
	cleanup := afterScenario(sc)
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.m != nil && w.m.activeHost != nil {
			waitHostDormant(w.m.activeHost)
		}
		return ctx, nil
	})
	sc.Step(`^an M4 project with a "([^"]+)" AI whose CLI edits a file on every turn$`, func(agent string) error {
		m := &m3World{f: f, cleanup: cleanup, tb: scenarioTB{TB: f.t, cleanup: cleanup}, groups: map[string]string{}, homeView: newHomeView()}
		if err := m.givenSingleAINode("Website", "Worker", agent); err != nil {
			return err
		}
		*w = *newEditWorld(m, agent)
		if err := w.seedReadme(); err != nil {
			return err
		}
		return useEditCLIs(f, cleanup)
	})
	sc.Step(`^a task of that AI runs at level 1$`, w.tryReadOnly)
	sc.Step(`^a task of that AI runs at level 1 and its CLI asks the host to edit the file$`, w.askReadOnly)
	sc.Step(`^the CLI refused its own edit, the file and the worktree are unchanged and there is no card$`, w.unchanged)
	sc.Step(`^a task of that AI runs at level 3$`, w.tryLevel3)
	sc.Step(`^the same CLI edited the file and there is no card$`, w.changed)
}

// seedReadme puts the file the CLI edits into the AI's workspace.
func (w *editWorld) seedReadme() error {
	folder, err := aiWorkspace(w.m.root, proto.ID(w.m.groups["Website"]), "Worker")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(folder, "README.md"), []byte("hello\n"), 0o600)
}

func newEditWorld(m *m3World, agent string) *editWorld {
	caps := &s3DriverWorld{caps: &hostCapsWorld{m3: m}}
	return &editWorld{m: m, m4: caps.m4(), agent: agent}
}

// editCLIDirs has the folder with the built CLIs of each run of the suite.
var editCLIDirs = map[*testing.T]string{}

func buildEditCLIs(t *testing.T) (string, error) {
	if dir := editCLIDirs[t]; dir != "" {
		return dir, nil
	}
	dir := t.TempDir()
	for _, vendor := range []string{"claude", "codex", "opencode", "qwen"} {
		cmd := exec.Command("go", "build", "-tags", "acceptance", "-o", filepath.Join(dir, hostAgentsExe(vendor)), "./editcli")
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("build editcli: %s: %w", out, err)
		}
	}
	editCLIDirs[t] = dir
	return dir, nil
}

// useEditCLIs makes every CLI that starts from here on the editing one.
func useEditCLIs(f fixtures, cleanup func(func())) error {
	dir, err := buildEditCLIs(f.t)
	if err != nil {
		return err
	}
	return setScenarioEnv(cleanup, "PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func (w *editWorld) tryReadOnly() error {
	return w.try("1", "Read only.", nil)
}

// askReadOnly posts only after the host refused the request.
func (w *editWorld) askReadOnly() error {
	return w.try("1", "Refused.", w.askSteps())
}

func (w *editWorld) tryLevel3() error {
	return w.try("3", "Edited.", nil)
}

// try runs one task turn that makes the body calls and then posts text.
func (w *editWorld) try(level, text string, body []map[string]any) error {
	if err := w.m.writeSetting("default_level", level); err != nil {
		return err
	}
	if err := w.script(body, text); err != nil {
		return err
	}
	task, err := w.m4.startTaskFor("Worker", "Edit at level "+level, "Edit README.md")
	if err != nil {
		return err
	}
	if err := w.m.thenAIPosts("Worker", text, "Website"); err != nil {
		return err
	}
	sess, ok := w.m.activeHost.Sessions.Get(host.SessionKey{Group: w.m.groups["Website"], Member: w.m.aiByName["Worker"].ID, Task: task})
	w.folder = sess.Folder
	if !ok {
		return fmt.Errorf("task %s has no session: %v", task, w.m.activeHost.Sessions.All())
	}
	return nil
}

func (w *editWorld) script(body []map[string]any, text string) error {
	switch w.agent {
	case "claude":
		return w.m.setScript("claude", m4ClaudeTurnDoc(body, 2, text))
	case "codex":
		return w.m.setScript("codex", m4CodexTurnDoc("edit-thread", "turn-1", body, 2, text))
	}
	s3 := &s3DriverWorld{agent: w.agent, caps: &hostCapsWorld{m3: w.m}}
	return s3.script(false, false, s3Turn{body: append(body, mcpPostSteps(2, text)...)})
}

// askSteps ask the host for permission to edit, as the CLI would if it asked
// at all, and expect its refusal: SoloDawn's half of the level.
func (w *editWorld) askSteps() []map[string]any {
	switch w.agent {
	case "claude":
		deny := host.ApprovalResult{Behavior: "deny", Message: "Denied: this session is read only."}
		return m4ApproveCallSteps(1, "Write", "toolu_1", map[string]any{"file_path": "README.md", "content": "edited"}, deny)
	case "codex":
		return m4CodexFileAskSteps(99, "item-1", "README.md", "+edited", "edit", "decline")
	case "opencode":
		return editServedAsk(map[string]any{"type": "permission.asked", "properties": map[string]any{"id": "ask-1", "sessionID": "s3-session", "permission": "edit", "patterns": []string{"README.md"}}},
			serverHTTP("POST /permission/ask-1/reply", map[string]string{"reply": "reject"}, true, 200))
	}
	return editServedAsk(map[string]any{"type": "permission_request", "data": map[string]any{"requestId": "ask-1", "toolCall": map[string]any{"kind": "edit"}}},
		serverHTTP("POST /session/s3-session/permission/ask-1", map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": "cancel"}}, map[string]any{}, 200))
}

func editServedAsk(event map[string]any, answer any) []map[string]any {
	return []map[string]any{{"dir": "sse", "msg": event}, answer.(map[string]any)}
}

func (w *editWorld) unchanged() error {
	entries, err := os.ReadDir(w.folder)
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("the read-only session left files in %s: %v", w.folder, entries)
	}
	return w.check("refused", "hello\n")
}

func (w *editWorld) changed() error {
	return w.check("wrote", "hello\nedited by "+w.agent+"\n")
}

// check compares the one attempt of the last turn, the file and the cards.
func (w *editWorld) check(outcome, want string) error {
	lines, err := w.m.waitLogs("edit "+w.agent+" "+outcome+" ", 1)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(filepath.Join(w.folder, "README.md"))
	if err != nil {
		return err
	}
	if len(lines) != 1 || string(got) != want {
		return fmt.Errorf("attempts %q, README.md %q, want one attempt and %q", lines, got, want)
	}
	return w.noCard()
}

func (w *editWorld) noCard() error {
	var cards []string
	for id, row := range w.m.msgs {
		if strings.HasPrefix(row.Kind, "approval") || row.Kind == "decision" || row.Kind == "confirm" {
			cards = append(cards, fmt.Sprintf("%s %s", row.Kind, id))
		}
	}
	if len(cards) != 0 {
		return fmt.Errorf("cards: %v", cards)
	}
	return nil
}
