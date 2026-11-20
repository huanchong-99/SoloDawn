//go:build !windows

package host

// These tests need a POSIX shell: a fake git written as a shell script, and a
// shell pipeline with a FIFO. A build constraint keeps them out of Windows
// builds instead of skipping them at run time.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
