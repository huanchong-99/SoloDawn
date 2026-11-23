//go:build !windows

package host

// These tests need a POSIX system: a fake git written as a shell script,
// shell pipelines and background jobs, process groups, Unix error texts. A
// build constraint keeps them out of Windows builds instead of skipping them
// at run time.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func TestChecksRequireBothGitDiffReads(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$2\" in --name-only) echo missing >&2; exit 2;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := checkTestRunner().hardChecks(t.Context(), dir, "base...head")
	if err == nil || err.Error() != "git diff: exit status 2: missing" {
		t.Fatalf("name-only: %v", err)
	}
}

func checkTestFIFO(t *testing.T, dir string) string {
	t.Helper()
	fifo := filepath.Join(dir, "ready")
	if out, err := exec.Command("mkfifo", fifo).CombinedOutput(); err != nil {
		t.Fatalf("mkfifo: %s (%v)", out, err)
	}
	return fifo
}

func TestCheckCancellationStopsForegroundPipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := t.TempDir()
	fifo := checkTestFIFO(t, dir)
	// Ready is signalled after the last fork, so the group kill finds every
	// process; stderr is closed so a shell's "Killed" notice, printed while
	// the kill reaches the group, is not part of the result.
	check := "exec 2>/dev/null; (sleep 30 & echo ready > ready; wait) | cat"
	done := make(chan proto.CheckItem, 1)
	go func() { done <- runProjectCheck(ctx, dir, check) }()
	data, err := os.ReadFile(fifo)
	if err != nil || string(data) != "ready\n" {
		t.Fatalf("ready: %q (%v)", data, err)
	}
	start := time.Now()
	cancel()
	want := proto.CheckItem{Name: check, Detail: new("signal: killed")}
	if got := <-done; !reflect.DeepEqual(got, want) {
		t.Fatalf("cancelled check: %q %v %q, want %q", got.Name, got.OK, *got.Detail, *want.Detail)
	}
	if time.Since(start) >= time.Second {
		t.Fatal("pipeline kept the output pipe open after cancellation")
	}
}

func TestTurnDeadlineKeepsHardChecks(t *testing.T) {
	dir := checkTestRepo(t)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	result, err := checkTestRunner().Run(ctx, dir, "base", checkTestGroup("sleep 2"))
	want := proto.ChecksBody{Stage: "local", Commit: checkTestGit(t, dir, "rev-parse", "HEAD"), Items: []proto.CheckItem{
		{Name: "sleep 2", Detail: new("signal: killed")},
		{Name: "secrets", OK: true}, {Name: "tests", OK: true},
	}}
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("turn deadline: %#v (%v), want %#v", result, err, want)
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("the check ran %v, past the turn's deadline", elapsed)
	}
}

func TestBackgroundChildDiesWithCheck(t *testing.T) {
	got := runProjectCheck(t.Context(), t.TempDir(), "sleep 30 & echo $!")
	pidText := strings.TrimPrefix(*got.Detail, exec.ErrWaitDelay.Error()+"\n")
	pid, err := strconv.Atoi(strings.TrimSpace(pidText))
	if got.OK || err != nil || pidText != strconv.Itoa(pid)+"\n" {
		t.Fatalf("background check: ok=%v detail %q", got.OK, *got.Detail)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("background child %d still runs after the check returned", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCheckThatCannotStart(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	want := proto.CheckItem{Name: "true", Detail: new("fork/exec /bin/sh: no such file or directory")}
	if got := runProjectCheck(t.Context(), missing, "true"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v (%q), want %q", got, *got.Detail, *want.Detail)
	}
}

func TestTaskkillArgs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	t.Setenv("TASKKILL_ARGS", out)
	if err := os.WriteFile(filepath.Join(dir, "taskkill.exe"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TASKKILL_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := taskkill(12345); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(out)
	if err != nil || string(args) != "/f\n/t\n/pid\n12345\n" {
		t.Fatalf("taskkill arguments: %q (%v)", args, err)
	}
}

// fakeCheckGit puts a git on PATH that runs script, a POSIX shell case on
// the subcommand, and answers anything else with nothing.
func fakeCheckGit(t *testing.T, cases string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n" + cases + "\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestCleanTreeSurfacesGitFailures(t *testing.T) {
	for _, tc := range []struct{ cases, want string }{
		{"status) echo broken >&2; exit 3;;", "git status: exit status 3: broken"},
		{"ls-files) printf 'h a\\000';;\nupdate-index) echo locked >&2; exit 4;;", "git update-index: exit status 4: locked"},
	} {
		dir := fakeCheckGit(t, tc.cases)
		if _, err := cleanCheckHead(t.Context(), dir); err == nil || err.Error() != tc.want {
			t.Errorf("%q: %v, want %q", tc.cases, err, tc.want)
		}
	}
}

// A pause, an interrupt or a shutdown cancels ctx: Run stops and reports
// nothing, in the group's checks and in the hard checks alike.
func TestCancelStopsRun(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fake    string
		command string
	}{
		{"during a project check", "", "touch '%s'; sleep 5"},
		{"during the hard checks", "rev-parse) echo abc;;\ndiff) touch \"$STARTED\"; exec sleep 5;;", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := checkTestRepo(t)
			if tc.fake != "" {
				dir = fakeCheckGit(t, tc.fake)
			}
			started := filepath.Join(t.TempDir(), "started")
			t.Setenv("STARTED", started)
			var group proto.GroupRow
			if tc.command != "" {
				// A check gets only the allow-listed environment, so the path is in the command.
				group = checkTestGroup(fmt.Sprintf(tc.command, started))
			}
			ctx, cancel := context.WithCancel(t.Context())
			go cancelWhenExists(ctx, cancel, started)
			result, err := checkTestRunner().Run(ctx, dir, "base", group)
			if err != context.Canceled || !reflect.DeepEqual(result, proto.ChecksBody{}) {
				t.Fatalf("cancelled run: %#v, %v", result, err)
			}
			if _, err := os.Stat(started); err != nil {
				t.Fatalf("the cancel came before the step: %v", err)
			}
		})
	}
}

// cancelWhenExists cancels once the step under test has created path.
func cancelWhenExists(ctx context.Context, cancel func(), path string) {
	for ctx.Err() == nil {
		if _, err := os.Stat(path); err == nil {
			cancel()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHardChecksTimeOutOnTheirOwnLimit(t *testing.T) {
	saved := hardCheckLimit
	t.Cleanup(func() { hardCheckLimit = saved })
	hardCheckLimit = 200 * time.Millisecond
	dir := fakeCheckGit(t, "rev-parse) echo abc;;\ndiff) exec sleep 5;;")
	result, err := checkTestRunner().Run(t.Context(), dir, "base", proto.GroupRow{})
	want := proto.ChecksBody{Stage: "local", Commit: "abc", Items: []proto.CheckItem{
		{Name: "secrets", Detail: new("timed out")}, {Name: "tests", Detail: new("timed out")},
	}}
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("hard checks past their limit: %#v (%v), want %#v", result, err, want)
	}
}
