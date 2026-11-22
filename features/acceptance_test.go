//go:build acceptance

package features

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

type fixtures struct {
	// t is the test of the whole suite, not of one scenario. What a step
	// registers on it (Cleanup, Setenv, TempDir, Context) lasts until the last
	// scenario has run, and FailNow on it from a step ends the suite there,
	// without the After hooks. A step returns its error instead, and undoes
	// what it changed with afterScenario.
	t        *testing.T
	bin      string
	fakeCLIs string
	stamped  map[string]string // solodawn built with a release tag, by tag, for this run of the suite
}

// Each step file registers its scenario initializer under its own name in init.
var stepSets = map[string]func(*godog.ScenarioContext, fixtures){}

func init() {
	stepSets["cmd"] = cmdSteps
}

func cmdSteps(sc *godog.ScenarioContext, f fixtures) {
	var stdout, stderr bytes.Buffer
	var code int
	sc.Step(`^I run "solodawn ?(.*)"$`, func(args string) {
		cmd := exec.Command(f.bin, strings.Fields(args)...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		_ = cmd.Run() // a non-zero exit is an outcome under test; ProcessState holds it
		code = cmd.ProcessState.ExitCode()
	})
	sc.Step(`^it prints "([^"]*)" on (stdout|stderr) and exits with code (\d+)$`, func(text, stream string, want int) error {
		got, other := stdout.String(), stderr.String()
		if stream == "stderr" {
			got, other = other, got
		}
		if got != text+"\n" || other != "" || code != want {
			return fmt.Errorf("got stdout %q, stderr %q, exit code %d", stdout.String(), stderr.String(), code)
		}
		return nil
	})
}

// TestFeatures runs every scenario in this directory except @web; the vitest-cucumber suite under web/ runs those.
// On Windows it also leaves out @not-windows: a scenario whose fixture Windows cannot provide, as its feature file says.
func TestFeatures(t *testing.T) {
	stopDeadlineTimer := armDeadlineCleanups(t)
	defer stopDeadlineTimer()
	bin := filepath.Join(t.TempDir(), "solodawn.exe")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/solodawn").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	fakes := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", fakes+string(os.PathSeparator), "../internal/host/fakecli/claude", "../internal/host/fakecli/codex", "../internal/host/fakecli/opencode").CombinedOutput(); err != nil {
		t.Fatalf("build fake CLIs: %v\n%s", err, out)
	}
	t.Setenv("PATH", fakes+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Git reads no user or system configuration, so a developer's settings
	// (commit signing, hooks) cannot change what the scenarios see.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := fixtures{t: t, bin: bin, fakeCLIs: fakes, stamped: map[string]string{}}
	names := slices.Sorted(maps.Keys(stepSets))
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	suite := godog.TestSuite{
		// godog sets its own variables (GODOG_SEED) before the suite starts.
		TestSuiteInitializer: func(ts *godog.TestSuiteContext) {
			ts.BeforeSuite(func() { env = environ() })
		},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, name := range names {
				stepSets[name](sc, f)
			}
			// Runs after every step set's own After hooks. Every scenario and the
			// go builds of the suite share the environment and the working
			// directory (Go keeps its module cache under HOME), so a scenario
			// that leaves either changed fails, and the next starts as the first.
			sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				if changed := restoreProcess(env, dir); len(changed) != 0 {
					return ctx, fmt.Errorf("the scenario left %s changed", strings.Join(changed, ", "))
				}
				return ctx, nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"."}, Tags: tags(runtime.GOOS), Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("acceptance scenarios failed")
	}
}

// tags is the suite's tag filter on one operating system.
func tags(goos string) string {
	if goos == "windows" {
		return "~@web && ~@not-windows"
	}
	return "~@web"
}

// afterScenario returns a function that defers undo to the end of the current
// scenario, after its last step, whether the steps passed or not. The undos
// run in reverse order.
func afterScenario(sc *godog.ScenarioContext) func(undo func()) {
	var undos []func()
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		for _, undo := range slices.Backward(undos) {
			undo()
		}
		return ctx, nil
	})
	return func(undo func()) { undos = append(undos, undo) }
}

func environ() map[string]string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	return env
}

// restoreProcess puts back the environment and the working directory the
// suite started with, and names what was changed. It names no value: a
// variable may hold a secret.
func restoreProcess(env map[string]string, dir string) []string {
	now := environ()
	keys := maps.Clone(env)
	maps.Copy(keys, now)
	var changed []string
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		value, set := env[key]
		if current, ok := now[key]; current != value || ok != set {
			changed = append(changed, "$"+key)
			restoreVar(key, value, set)
		}
	}
	if current, _ := os.Getwd(); current != dir {
		changed = append(changed, "the working directory")
		os.Chdir(dir)
	}
	return changed
}

func restoreVar(key, value string, set bool) {
	if set {
		os.Setenv(key, value)
	} else {
		os.Unsetenv(key)
	}
}
