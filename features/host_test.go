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
	"time"

	"github.com/cucumber/godog"
)

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
		for i := 0; i <= switches; i++ {
			args := []string{"app-server"}
			switch {
			case vendor == "claude" && i == 0:
				args = []string{"-p", "--session-id", id}
			case vendor == "claude" && i%2 == 0:
				args = []string{"-p", "--resume", id}
			case vendor == "claude":
				args = []string{"--resume", id}
			case i%2 == 1:
				args = []string{"resume", id}
			}
			cmd := exec.CommandContext(ctx, vendor, args...)
			cmd.Env = append(os.Environ(), "SOLODAWN_FAKE_"+strings.ToUpper(vendor)+"_SCRIPT="+script, "SOLODAWN_FAKECLI_LOG="+log)
			if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
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
