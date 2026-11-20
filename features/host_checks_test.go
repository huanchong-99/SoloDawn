//go:build acceptance

package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	stepSets["host_checks"] = hostCheckSteps
}

func hostCheckSteps(sc *godog.ScenarioContext, f fixtures) {
	var dir, head string
	probe, line := hostCheckEnvProbe()
	var result proto.ChecksBody
	var runErr error
	// git runs each command in dir, stops at the first that fails and returns
	// the output of the last. A step returns the error: Fatal on the suite's
	// test would end the whole suite.
	git := func(commands ...[]string) (string, error) {
		var out string
		for _, args := range commands {
			cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=fixture"}, args...)...)
			cmd.Dir = dir
			b, err := cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("git %v: %s (%v)", args, b, err)
			}
			out = strings.TrimSpace(string(b))
		}
		return out, nil
	}
	const marker = "-----BEGIN " + "PRIVATE KEY-----"
	var runner host.CheckRunner
	run := func(commands ...string) {
		group := proto.GroupRow{Checks: commands, Limits: proto.Limits{TurnMinutes: 1}}
		result, runErr = runner.Run(f.t.Context(), dir, "base", group)
	}
	sc.Step(`^a clean task branch for the host check runner$`, func() (err error) {
		dir = f.t.TempDir()
		head, err = git([]string{"init", "-b", "base"}, []string{"commit", "--allow-empty", "-m", "base"},
			[]string{"switch", "-c", "task"}, []string{"rev-parse", "HEAD"})
		return err
	})
	sc.Step(`^the task worktree contains an uncommitted file$`, func() error {
		return os.WriteFile(filepath.Join(dir, "notes"), []byte("unfinished"), 0600)
	})
	sc.Step(`^the host is asked to run a check that writes a marker$`, func() { run("echo ran > marker") })
	sc.Step(`^the host refuses the dirty worktree and the marker is absent$`, func() error {
		if runErr != host.ErrDirtyCheckTree {
			return fmt.Errorf("dirty tree: got %v", runErr)
		}
		if _, err := os.Stat(filepath.Join(dir, "marker")); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("marker: got %v, want file absent", err)
		}
		return nil
	})
	sc.Step(`^the task commit adds a line matching the secret checker$`, func() (err error) {
		if err = os.WriteFile(filepath.Join(dir, "notes"), []byte(marker+"\n"), 0600); err != nil {
			return err
		}
		head, err = git([]string{"add", "notes"}, []string{"commit", "-m", "add synthetic marker"}, []string{"rev-parse", "HEAD"})
		return err
	})
	sc.Step(`^the host runs checks with an empty project command list$`, func() { run() })
	sc.Step(`^the secret check fails for the task commit and the tests check passes$`, func() error {
		want := proto.ChecksBody{Stage: "local", Commit: head, Items: []proto.CheckItem{
			{Name: "secrets", Detail: new("secret: this looks like a secret; remove it and send again")}, {Name: "tests", OK: true},
		}}
		return compareHostChecks(result, runErr, want)
	})
	// The runner reads the process environment, so the variables are set for
	// this scenario only and restored after it; f.t.Setenv would keep them for
	// the rest of the suite.
	var restore []func()
	setenv := func(key, value string) {
		old, had := os.LookupEnv(key)
		restore = append(restore, func() {
			if had {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
		os.Setenv(key, value)
	}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		for _, r := range restore {
			r()
		}
		return ctx, nil
	})
	sc.Step(`^the host environment contains a test token$`, func() {
		setenv("SOLODAWN_CHECK_TEST_TOKEN", "node-only")
		setenv("HOME", dir)
	})
	sc.Step(`^the host runs a check that inspects PATH, HOME and the test token$`, func() {
		run(probe)
	})
	sc.Step(`^the command confirms PATH and HOME are set and the token is absent$`, func() error {
		want := proto.ChecksBody{Stage: "local", Commit: head, OK: true, Items: []proto.CheckItem{
			{Name: probe, OK: true, Detail: &line}, {Name: "secrets", OK: true}, {Name: "tests", OK: true},
		}}
		return compareHostChecks(result, runErr, want)
	})
}

func hostCheckEnvProbe() (command, output string) {
	if runtime.GOOS == "windows" {
		return `if defined PATH if defined HOME if not defined SOLODAWN_CHECK_TEST_TOKEN echo clean`, "clean\r\n"
	}
	return `test -n "$PATH" && test -n "$HOME" && test -z "${SOLODAWN_CHECK_TEST_TOKEN+x}" && echo clean`, "clean\n"
}

func compareHostChecks(got proto.ChecksBody, err error, want proto.ChecksBody) error {
	if err != nil || !reflect.DeepEqual(got, want) {
		return fmt.Errorf("host checks: %#v (%v), want %#v", got, err, want)
	}
	return nil
}
