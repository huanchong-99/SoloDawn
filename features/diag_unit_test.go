//go:build acceptance

package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	ossignal "os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestTrailCutsToTheLastTwenty(t *testing.T) {
	var tr trail
	for i := 1; i <= 25; i++ {
		tr.see(fmt.Sprintf("v%d", i))
	}
	got := tr.timedOut(10*time.Second, "the thing").Error()
	want := "timed out after 10s waiting for the thing; saw 25: v6, v7, v8, v9, v10, v11, v12, v13, v14, v15, v16, v17, v18, v19, v20, v21, v22, v23, v24, v25"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWaitUntilReturnsWhenTheCheckPasses(t *testing.T) {
	if err := waitUntil(time.Second, "now", func() error { return nil }); err != nil {
		t.Fatalf("a passing check failed the wait: %v", err)
	}
}

func TestWaitUntilListsAReadingOnce(t *testing.T) {
	err := waitUntil(time.Millisecond, "a value", func() error { return errors.New("x=1") })
	if want := "timed out after 1ms waiting for a value; saw 1: x=1"; err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestWaitUntilListsEachNewReading(t *testing.T) {
	calls := 0
	err := waitUntil(time.Millisecond, "a count", func() error {
		calls++
		return fmt.Errorf("%d", calls)
	})
	list := make([]string, calls)
	for i := range list {
		list[i] = fmt.Sprint(i + 1)
	}
	want := fmt.Sprintf("timed out after 1ms waiting for a count; saw %d: %s", calls, strings.Join(list, ", "))
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestWaitUntilEndsAtOnceOnAbort(t *testing.T) {
	boom := errors.New("boom")
	if err := waitUntil(time.Hour, "never", func() error { return abort(boom) }); err != boom {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

func TestRecvWait(t *testing.T) {
	ch := make(chan string, 1)
	ch <- "x"
	w := newWait(time.Second, "a string", nil)
	defer w.stop()
	if got, err := recvWait(w, ch); got != "x" || err != nil {
		t.Fatalf("got %q, %v", got, err)
	}
	var seen trail
	seen.see("t=rows g=g1 seq=1-2")
	late := newWait(time.Millisecond, "a frame", &seen)
	defer late.stop()
	_, err := recvWait(late, ch)
	if want := "timed out after 1ms waiting for a frame; saw 1: t=rows g=g1 seq=1-2"; err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestFrameBrief(t *testing.T) {
	for raw, want := range map[string]string{
		`{"t":"synced","re":3,"ok":true,"g":"g1","seq":9}`:   "t=synced re=3 g=g1 seq=9",
		`{"t":"rows","g":"g1","rows":[{"seq":4},{"seq":6}]}`: "t=rows g=g1 seq=4-6",
		`{"t":"welcome","re":0}`:                             "t=welcome re=0",
		`not json`:                                           `"not json"`,
		strings.Repeat("x", 100):                             `"` + strings.Repeat("x", 60) + `"`,
	} {
		if got := frameBrief([]byte(raw)); got != want {
			t.Errorf("frameBrief(%.30q) = %q, want %q", raw, got, want)
		}
	}
}

// TestDiagChildNode is the node that TestFailedScenarioLeavesDiagnostics
// starts as a child process; run by itself it does nothing.
func TestDiagChildNode(t *testing.T) {
	data := os.Getenv("SOLODAWN_DIAG_CHILD")
	if data == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(data, "logs", "node.log"), []byte("node line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan os.Signal, 1)
	ossignal.Notify(stop, os.Interrupt)
	fmt.Fprintln(os.Stderr, "child up")
	<-stop
}

// startDiagChild starts TestDiagChildNode as a node of the scenario.
func startDiagChild(t *testing.T, scratch string, stderr *syncBuffer) error {
	cmd := exec.Command(os.Args[0], "-test.run=^TestDiagChildNode$")
	cmd.Env = append(os.Environ(), "SOLODAWN_DIAG_CHILD="+scratch, fakeCLILog+"="+filepath.Join(scratch, "child.log"))
	cmd.Stderr = stderr
	if _, err := startNodeCmd(t, nil, cmd, "node", scratch, stderr); err != nil {
		return err
	}
	return waitUntil(10*time.Second, "the child node to start", func() error {
		if !strings.Contains(stderr.String(), "child up") {
			return fmt.Errorf("stderr = %q", stderr.String())
		}
		return nil
	})
}

// runDiagSuite runs one scenario that passes and one that fails with a child
// node, and returns what godog printed.
func runDiagSuite(t *testing.T, scratch string) string {
	var out bytes.Buffer
	var stderr syncBuffer
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			diagHooks(sc)
			sc.Step(`^the process logs "([^"]*)"$`, func(line string) { log.Print(line) })
			sc.Step(`^a node runs$`, func() error { return startDiagChild(t, scratch, &stderr) })
			sc.Step(`^the scenario fails$`, func() error { return errors.New("boom") })
		},
		Options: &godog.Options{
			Format: "pretty", NoColors: true, Output: &out, Strict: true, Concurrency: 1,
			FeatureContents: []godog.Feature{{Name: "dump.feature", Contents: []byte(`Feature: dump
  Scenario: passes
    Given the process logs "first line"

  Scenario: fails with a node
    Given the process logs "second line"
    And a node runs
    When the scenario fails
`)}},
		},
	}
	if suite.Run() == 0 {
		t.Fatal("the failing scenario passed")
	}
	return out.String()
}

// diagScratch sets up the folders and the fake CLI logs of the child node.
func diagScratch(t *testing.T) (artifacts, scratch string) {
	artifacts, scratch = t.TempDir(), t.TempDir()
	t.Setenv(artifactsIn, artifacts)
	t.Setenv(fakeCLILog, filepath.Join(scratch, "process.log"))
	for path, text := range map[string]string{"process.log": "start claude 1\n", "child.log": "start codex 2\n"} {
		if err := os.WriteFile(filepath.Join(scratch, path), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(scratch, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	return artifacts, scratch
}

func TestFailedScenarioLeavesDiagnostics(t *testing.T) {
	artifacts, scratch := diagScratch(t)
	defer diagCapture()()

	printed := runDiagSuite(t, scratch)

	entries, err := os.ReadDir(artifacts)
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "scenario-fails-with-a-node-") {
		t.Fatalf("folders = %v, %v; want the one of the failed scenario", entries, err)
	}
	if !strings.Contains(printed, "diagnostics are in "+artifacts) {
		t.Errorf("the failure does not name the folder:\n%s", printed)
	}
	checkDiagFolder(t, filepath.Join(artifacts, entries[0].Name()))
}

// childDump is what the node's standard error holds: on Windows only what it
// wrote, elsewhere the goroutine dump of the SIGQUIT.
func childDump() string {
	if runtime.GOOS == "windows" {
		return "child up"
	}
	return "SIGQUIT: quit"
}

func checkDiagFolder(t *testing.T, dir string) {
	t.Helper()
	nodes := readFile(t, dir, "nodes.log")
	if !strings.Contains(nodes, "second line") || strings.Contains(nodes, "first line") || !strings.Contains(nodes, "/") {
		t.Errorf("nodes.log = %q, want the lines of the failed scenario with their time", nodes)
	}
	for file, want := range map[string]string{
		"goroutines.txt":       "goroutine ",
		"fakecli.log":          "start claude 1\n",
		"node-0/logs/node.log": "node line\n",
		"node-0/fakecli.log":   "start codex 2\n",
		"node-0/stderr.txt":    childDump(),
	} {
		if got := readFile(t, dir, file); !strings.Contains(got, want) {
			t.Errorf("%s = %q, want it to hold %q", file, got, want)
		}
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Error(err)
	}
	return string(raw)
}

func TestRecvCtx(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 7
	if got, err := recvCtx(t.Context(), ch, "a number", nil); got != 7 || err != nil {
		t.Fatalf("got %d, %v", got, err)
	}
	var seen trail
	seen.see("t=rows seq=1")
	late, cancelLate := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancelLate()
	_, err := recvCtx(late, ch, "a number", &seen)
	if want := regexp.MustCompile(`^timed out after \d+ms waiting for a number; saw 1: t=rows seq=1$`); err == nil || !want.MatchString(err.Error()) {
		t.Fatalf("got %v", err)
	}
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := recvCtx(gone, ch, "a number", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the cancellation", err)
	}
}

func TestRecvFrameListsTheFramesTaken(t *testing.T) {
	ch := make(chan any, 2)
	ch <- map[string]any{"t": "rows", "g": "g1"}
	var seen trail
	if _, err := recvFrame(ch, time.Second, &seen, briefAny); err != nil {
		t.Fatal(err)
	}
	_, err := recvFrame(ch, time.Millisecond, &seen, briefAny)
	if want := "timed out after 1ms waiting for a frame; saw 1: t=rows g=g1"; err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}
