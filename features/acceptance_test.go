//go:build acceptance

package features

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// TestFeatures runs every scenario in this directory except @web; the vitest-cucumber suite under web/ runs those.
func TestFeatures(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "solodawn.exe")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/solodawn").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			protocolSteps(sc)
			identitySteps(sc)
			storeSteps(sc)
			rendezvousSteps(sc, bin)
			var stdout, stderr bytes.Buffer
			var code int
			sc.Step(`^I run "solodawn ?(.*)"$`, func(args string) {
				cmd := exec.Command(bin, strings.Fields(args)...)
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
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"."}, Tags: "~@web", Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("acceptance scenarios failed")
	}
}
