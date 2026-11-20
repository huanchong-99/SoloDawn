//go:build !windows

package host

// These tests need a POSIX system: a fake git written as a shell script,
// shell pipelines and background jobs, process groups, Unix error texts. A
// build constraint keeps them out of Windows builds instead of skipping them
// at run time.

import (
	"context"
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

func TestChecksSurfaceLsFilesFailure(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in ls-files) echo broken >&2; exit 3;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := cleanCheckHead(t.Context(), dir)
	if err == nil || err.Error() != "git ls-files: exit status 3: broken" {
		t.Fatalf("ls-files: %v", err)
	}
}
