//go:build acceptance

package features

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

type fixtures struct {
	t        *testing.T
	bin      string
	fakeCLIs string
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
func TestFeatures(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "solodawn.exe")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/solodawn").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	fakes := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", fakes+string(os.PathSeparator), "../internal/host/fakecli/claude", "../internal/host/fakecli/codex").CombinedOutput(); err != nil {
		t.Fatalf("build fake CLIs: %v\n%s", err, out)
	}
	t.Setenv("PATH", fakes+string(os.PathListSeparator)+os.Getenv("PATH"))
	f := fixtures{t: t, bin: bin, fakeCLIs: fakes}
	names := slices.Sorted(maps.Keys(stepSets))
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, name := range names {
				stepSets[name](sc, f)
			}
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"."}, Tags: "~@web", Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("acceptance scenarios failed")
	}
}
