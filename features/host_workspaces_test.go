//go:build acceptance

package features

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	stepSets["host_workspaces"] = hostWorkspacesSteps
}

type workspacesWorld struct {
	root   string
	h      *host.Host
	listed map[string][]proto.Workspace
}

func hostWorkspacesSteps(sc *godog.ScenarioContext, f fixtures) {
	cleanup := afterScenario(sc)
	w := &workspacesWorld{}
	sc.Step(`^a host on disk whose Claude Code sessions were in "([^"]+)", then in "([^"]+)", then in a deleted folder and under the node's data directory$`, func(first, second string) error {
		return w.setup(f, cleanup, first, second)
	})
	sc.Step(`^a scripted Codex whose threads, over two pages, were in "([^"]+)", then in "([^"]+)", then in a deleted folder$`, w.scriptCodex)
	sc.Step(`^a scripted Codex that exits without listing$`, func() error { return w.writeCodexScript(`{"session":"threads","exit":1}`) })
	sc.Step(`^the home folder's "([^"]+)", "([^"]+)" and "([^"]+)" cannot be read$`, w.lockFiles)
	sc.Step(`^the owner asks the host for the workspaces of "([^"]+)", "([^"]+)" and "([^"]+)"$`, w.ask)
	sc.Step(`^the host lists "([^"]*)" for "([^"]+)", "([^"]*)" for "([^"]+)" and nothing for "([^"]+)"$`, w.check)
}

// setup opens a host whose data directory is data/ under a scenario folder,
// and a home folder whose Claude Code sessions are, oldest first, in first,
// in second, in a folder that is then deleted, and under the data directory.
func (w *workspacesWorld) setup(f fixtures, cleanup func(func()), first, second string) error {
	root, err := filepath.EvalSymlinks(f.t.TempDir())
	if err != nil {
		return err
	}
	w.root = root
	data, inData := filepath.Join(root, "data"), filepath.Join(root, "data", "projects", "g", "repo")
	if err := os.MkdirAll(inData, 0o700); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(data, "node.db"))
	if err != nil {
		return err
	}
	cleanup(func() { db.Close() })
	w.h, _, err = host.New(db, identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))), time.Now)
	if err != nil {
		return err
	}
	home := filepath.Join(root, "home")
	if err := w.setEnv(cleanup, home); err != nil {
		return err
	}
	cleanup(func() { w.unlockFiles() })
	return w.writeClaudeSessions(home, w.folder(first), w.folder(second), w.folder("deleted"), inData)
}

// setEnv points the home folder, Claude Code's folder and the fake Codex at
// the scenario folder for this scenario.
func (w *workspacesWorld) setEnv(cleanup func(func()), home string) error {
	for _, kv := range [][2]string{{"HOME", home}, {"USERPROFILE", home}, {"CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")}, {"SOLODAWN_FAKE_CODEX_SCRIPT", filepath.Join(w.root, "codex.json")}, {"SOLODAWN_FAKECLI_LOG", filepath.Join(w.root, "cli.log")}} {
		if err := setScenarioEnv(cleanup, kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// folder is a folder under the scenario folder, made if it is not there.
func (w *workspacesWorld) folder(name string) string {
	dir := filepath.Join(w.root, "work", name)
	os.MkdirAll(dir, 0o700)
	return dir
}

// writeClaudeSessions writes one session file per folder, each newer than the
// one before, whose third record names the folder, and deletes the third.
func (w *workspacesWorld) writeClaudeSessions(home string, folders ...string) error {
	start := time.Now().Add(-time.Hour)
	for i, dir := range folders {
		file := filepath.Join(home, ".claude", "projects", fmt.Sprintf("p%d", i), "s.jsonl")
		lines := fmt.Sprintf("{\"type\":\"summary\"}\n{\"type\":\"file-history-snapshot\"}\n%s\n", accJSON(map[string]string{"type": "user", "cwd": dir}))
		err := errors.Join(os.MkdirAll(filepath.Dir(file), 0o700), os.WriteFile(file, []byte(lines), 0o600))
		if err = cmp.Or(err, os.Chtimes(file, start, start.Add(time.Duration(i)*time.Minute))); err != nil {
			return err
		}
	}
	return os.RemoveAll(folders[2])
}

// scriptCodex answers thread/list in two pages, as Codex CLI 0.164.0 does.
func (w *workspacesWorld) scriptCodex(first, second string) error {
	thread := func(dir string, minutes int) map[string]any {
		return map[string]any{"cwd": dir, "updatedAt": time.Now().Add(time.Duration(minutes-60) * time.Minute).Unix()}
	}
	steps := []any{
		map[string]any{"dir": "stdin", "msg": map[string]any{"id": 1, "method": "initialize"}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"id": 1, "result": map[string]any{}}},
		map[string]any{"dir": "stdin", "msg": map[string]any{"method": "initialized"}},
		map[string]any{"dir": "stdin", "msg": map[string]any{"id": 2, "method": "thread/list", "params": map[string]any{"sortKey": "updated_at", "modelProviders": []string{}}}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"id": 2, "result": map[string]any{"data": []any{thread(w.folder(second), 2), thread(w.folder(first), 1)}, "nextCursor": "page-2"}}},
		map[string]any{"dir": "stdin", "msg": map[string]any{"id": 3, "method": "thread/list", "params": map[string]any{"cursor": "page-2"}}},
		map[string]any{"dir": "stdout", "msg": map[string]any{"id": 3, "result": map[string]any{"data": []any{thread(filepath.Join(w.root, "work", "deleted"), 3)}, "nextCursor": nil}}},
		map[string]any{"dir": "eof"},
	}
	return w.writeCodexScript(accJSON(map[string]any{"session": "threads", "steps": steps}))
}

func (w *workspacesWorld) writeCodexScript(script string) error {
	return os.WriteFile(filepath.Join(w.root, "codex.json"), []byte(script), 0o600)
}

// lockFiles writes the files with no permission to read them. Where the
// system does not enforce that (Windows, root), the lists must still be the same.
func (w *workspacesWorld) lockFiles(first, second, third string) error {
	for _, name := range []string{first, second, third} {
		file := filepath.Join(w.root, "home", name)
		if err := errors.Join(os.MkdirAll(filepath.Dir(file), 0o700), os.WriteFile(file, []byte(`{"projects":{"/elsewhere":{}}}`), 0o000)); err != nil {
			return err
		}
	}
	return nil
}

// unlockFiles lets the scenario folder be removed.
func (w *workspacesWorld) unlockFiles() {
	for _, name := range []string{".claude.json", ".codex/auth.json", ".codex/config.toml"} {
		os.Chmod(filepath.Join(w.root, "home", name), 0o600)
	}
}

func (w *workspacesWorld) ask(first, second, third string) error {
	w.listed = map[string][]proto.Workspace{}
	for _, agent := range []string{first, second, third} {
		answer, err := w.h.Workspaces(context.Background(), json.RawMessage(accJSON(proto.HostWorkspacesRequest{Request: proto.Request{Type: "host.workspaces", ID: 9}, Agent: agent})))
		if err != nil {
			return err
		}
		w.listed[agent] = answer.(proto.HostWorkspaces).Workspaces
	}
	return nil
}

// check compares each list with the folders named, newest first, and checks
// that each time is the one of the folder's newest session.
func (w *workspacesWorld) check(claude, agent1, codex, agent2, none string) error {
	return errors.Join(w.checkList(agent1, claude), w.checkList(agent2, codex), w.checkList(none, ""))
}

func (w *workspacesWorld) checkList(agent, want string) error {
	var got, wantPaths []string
	for _, ws := range w.listed[agent] {
		got = append(got, ws.Path)
	}
	for name := range strings.SplitSeq(want, ", ") {
		if name != "" {
			wantPaths = append(wantPaths, w.folder(name))
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(wantPaths) || !newestFirst(w.listed[agent]) {
		return fmt.Errorf("%s lists %+v, want %v newest first", agent, w.listed[agent], wantPaths)
	}
	return nil
}

func newestFirst(list []proto.Workspace) bool {
	for i := 1; i < len(list); i++ {
		if list[i].Used > list[i-1].Used {
			return false
		}
	}
	return true
}
