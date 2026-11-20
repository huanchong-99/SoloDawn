//go:build acceptance

package features

import (
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
	git := func(args ...string) string {
		f.t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=fixture"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			f.t.Fatalf("git %v: %s (%v)", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	const marker = "-----BEGIN " + "PRIVATE KEY-----"
	runner := host.CheckRunner{
		CheckSecret: func(text string) error {
			if strings.Contains(text, marker) {
				return errors.New("secret detected")
			}
			return nil
		},
	}
	run := func(commands ...string) {
		group := proto.GroupRow{Checks: commands, Limits: proto.Limits{TurnMinutes: 1}}
		result, runErr = runner.Run(f.t.Context(), dir, "base", group)
	}
	sc.Step(`^a clean task branch for the host check runner$`, func() {
		dir = f.t.TempDir()
		git("init", "-b", "base")
		git("commit", "--allow-empty", "-m", "base")
		git("switch", "-c", "task")
		head = git("rev-parse", "HEAD")
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
	sc.Step(`^the task commit adds a line matching the secret checker$`, func() error {
		if err := os.WriteFile(filepath.Join(dir, "notes"), []byte(marker+"\n"), 0600); err != nil {
			return err
		}
		git("add", "notes")
		git("commit", "-m", "add synthetic marker")
		head = git("rev-parse", "HEAD")
		return nil
	})
	sc.Step(`^the host runs checks with an empty project command list$`, func() { run() })
	sc.Step(`^the secret check fails for the task commit and the tests check passes$`, func() error {
		want := proto.ChecksBody{Stage: "local", Commit: head, Items: []proto.CheckItem{
			{Name: "secrets", Detail: new("secret detected")}, {Name: "tests", OK: true},
		}}
		return compareHostChecks(result, runErr, want)
	})
	sc.Step(`^the host environment contains a test token$`, func() {
		f.t.Setenv("SOLODAWN_CHECK_TEST_TOKEN", "node-only")
		f.t.Setenv("HOME", dir)
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
