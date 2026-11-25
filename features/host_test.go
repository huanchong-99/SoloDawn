//go:build acceptance

package features

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/peer"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

// The host's interfaces are method sets of peer's types, so that node hands
// a connection or a stream to the host as it is (planning/architecture.md 8.3).
var (
	_ host.Home     = (*peer.Connection)(nil)
	_ host.Owner    = (*peer.Connection)(nil)
	_ host.Terminal = (*peer.Stream)(nil)
)

func init() {
	stepSets["host"] = func(sc *godog.ScenarioContext, f fixtures) {
		hostSteps(sc, f.t, f.fakeCLIs)
		hostSeamSteps(sc)
	}
}

// hostSeamSteps drive the session table, the tool table, the audit record and
// the link to a home of one host on a store in memory.
func hostSeamSteps(sc *godog.ScenarioContext) {
	var (
		db      *store.Store
		h       *host.Host
		saved   []host.Session
		moved   error
		caller  host.Caller
		written []bool
		failed  error
	)
	cleanup := afterScenario(sc)
	keys := map[string]host.SessionKey{"task": {Group: "g1", Member: "m7", Task: "t12"}, "group": {Group: "g1", Member: "m7"}}
	// How a new session reaches each state through changes the contract lists.
	paths := map[string][]host.State{"dormant": {}, "working": {host.Working}, "waiting": {host.Working, host.Waiting}, "takeover": {host.Takeover}}
	start := func() (err error) {
		h, saved, err = host.New(db, identity.Key(ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))), time.Now)
		return err
	}
	open := func() (err error) {
		if db != nil {
			return nil
		}
		if db, err = store.Open(":memory:"); err != nil {
			return err
		}
		cleanup(func() { db.Close() })
		return start()
	}
	sc.Step(`^a host without sessions$`, open)
	sc.Step(`^a host whose AI has a "([^"]+)" (task|group) session$`, func(state, kind string) error {
		path, known := paths[state]
		if err := open(); err != nil || !known {
			return fmt.Errorf("no host with a %s session: %v", state, err)
		}
		key, at := keys[kind], host.Dormant
		session := host.Session{SessionKey: key, Kind: host.Kind(kind), Folder: "repo", Agent: "claude", Home: "home-device", Name: "Claude Code"}
		if err := h.Sessions.Create(session); err != nil {
			return err
		}
		for _, next := range path {
			if err := h.Sessions.Move(key, at, next); err != nil {
				return err
			}
			at = next
		}
		return nil
	})
	sc.Step(`^the host moves the (task|group) session from "([^"]+)" to "([^"]+)"$`, func(kind, from, to string) {
		moved = h.Sessions.Move(keys[kind], host.State(from), host.State(to))
	})
	sc.Step(`^the host records a takeover and moves the task session from "([^"]+)" to "([^"]+)" in one transaction$`, func(from, to string) {
		moved = h.Sessions.Write(func(tx *host.Tx) error {
			if _, err := h.Audit(tx.Tx, host.AuditRow{Group: "g1", Agent: "claude", What: host.AuditSetting}); err != nil {
				return err
			}
			return tx.Move(keys["task"], host.State(from), host.State(to))
		})
	})
	sc.Step(`^the audit record holds (\d+) rows$`, func(want int) error {
		var got int
		err := db.Write(func(tx *store.Tx) error { return tx.QueryRow("SELECT count(*) FROM audit").Scan(&got) })
		if err != nil || got != want {
			return fmt.Errorf("the audit record holds %d rows (%v), want %d", got, err, want)
		}
		return nil
	})
	sc.Step(`^the move ends with "([^"]*)"$`, func(want string) error {
		got := "ok"
		if moved != nil {
			got = moved.Error()
		}
		if got != want {
			return fmt.Errorf("got %q, want %q", got, want)
		}
		return nil
	})
	sc.Step(`^the host holds the (task|group) session as "([^"]+)"$`, func(kind, want string) error {
		if s, ok := h.Sessions.Get(keys[kind]); !ok || string(s.State) != want {
			return fmt.Errorf("got %q (found: %v), want %q", s.State, ok, want)
		}
		return nil
	})
	sc.Step(`^the host starts again on its store$`, start)
	sc.Step(`^the start reports the task session as it was left, "([^"]+)"$`, func(want string) error {
		if len(saved) != 1 || saved[0].SessionKey != keys["task"] || string(saved[0].State) != want {
			return fmt.Errorf("the start reported %+v, want the task session as %s", saved, want)
		}
		return nil
	})
	sc.Step(`^a tool "([^"]+)" that is for task sessions above level 1$`, func(name string) {
		host.Tools[name] = host.Tool{For: func(c host.Caller) bool { return c.Kind == host.TaskSession && c.Level > 1 }}
		cleanup(func() { delete(host.Tools, name) })
	})
	sc.Step(`^a CLI process is admitted for the task session at level (\d+)$`, func(level int) error {
		var named bool
		if caller, named = h.Sessions.Caller(h.Sessions.Admit(keys["task"], level, false)); !named {
			return errors.New("the token names nobody")
		}
		return nil
	})
	sc.Step(`^the tool "([^"]+)" is (listed|not listed) to that process$`, func(name, want string) error {
		if _, listed := host.ToolsFor(caller)[name]; listed != (want == "listed") {
			return fmt.Errorf("the tool is listed: %v, want it %s", listed, want)
		}
		return nil
	})
	sc.Step(`^the host records inbox item (\d+) of home "([^"]+)" as "([^"]+)"$`, func(item int, home, what string) error {
		return db.Write(func(tx *store.Tx) error {
			done, err := h.Audit(tx, host.AuditRow{Group: "g1", Agent: "claude", What: what, Item: int64(item), Home: home})
			written = append(written, done)
			return err
		})
	})
	sc.Step(`^the first record is written and the second is not$`, func() error {
		if !slices.Equal(written, []bool{true, false}) {
			return fmt.Errorf("written: %v", written)
		}
		return nil
	})
	sc.Step(`^the host sends a command to a home that is not connected$`, func() {
		_, failed = h.Command(context.Background(), "absent-home", `{"t":"post"}`)
	})
	sc.Step(`^the host's command fails with "([^"]*)"$`, func(want string) error {
		if failed == nil || failed.Error() != want {
			return fmt.Errorf("got %v, want %q", failed, want)
		}
		return nil
	})
}

func hostSteps(sc *godog.ScenarioContext, t *testing.T, bin string) {
	var vendor, id, log string
	sc.Step(`^"([^"]+)" is found first on PATH with version "([^"]+)"$`, func(name, version string) error {
		vendor = name
		path, err := exec.LookPath(name)
		if err != nil {
			return err
		}
		if filepath.Dir(path) != bin {
			return fmt.Errorf("%s resolved outside the test bin directory: %s", name, path)
		}
		out, err := exec.Command(path, "--version").Output()
		if err != nil || string(out) != version+"\n" {
			return fmt.Errorf("--version: %q (%v), want %q", out, err, version)
		}
		return nil
	})
	sc.Step(`^I switch between structured and terminal starts (\d+) times on session "([^"]+)"$`, func(switches int, session string) error {
		dir := t.TempDir()
		id, log = session, filepath.Join(dir, "lifecycle")
		script := filepath.Join(dir, "wait.json")
		if err := os.WriteFile(script, []byte(fmt.Sprintf(`{"session":%q,"steps":[{"dir":"eof"}]}`, id)), 0600); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		start := time.Now()
		for i := 0; i <= switches; i++ {
			cmd := exec.CommandContext(ctx, vendor, switchFakeArgs(vendor, id, i)...)
			cmd.Env = append(os.Environ(), "SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT="+script, "SOLODAWN_FAKECLI_LOG="+log)
			out, err := cmd.CombinedOutput()
			if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ctxEnded(ctx, start, fmt.Sprintf("CLI switch %d (%s)", i, vendor), nil)
			}
			if err != nil || len(out) != 0 {
				return fmt.Errorf("start %d: %q (%v)", i, out, err)
			}
		}
		return nil
	})
	sc.Step(`^the lifecycle log contains (\d+) start-exit pairs for that session in order$`, func(n int) error {
		got, err := os.ReadFile(log)
		if err != nil {
			return err
		}
		pair := fmt.Sprintf("start %s %s\nexit %s %s\n", vendor, id, vendor, id)
		if string(got) != strings.Repeat(pair, n) {
			return fmt.Errorf("lifecycle log: %s", got)
		}
		return nil
	})
}

func switchFakeArgs(vendor, id string, i int) []string {
	if vendor == "claude" {
		return switchClaudeArgs(id, i)
	}
	if i%2 == 1 {
		return []string{"resume", id}
	}
	return []string{"app-server"}
}

func switchClaudeArgs(id string, i int) []string {
	switch {
	case i == 0:
		return []string{"-p", "--session-id", id}
	case i%2 == 0:
		return []string{"-p", "--resume", id}
	default:
		return []string{"--resume", id}
	}
}
