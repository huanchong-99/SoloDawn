package node

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestProbeCommand(t *testing.T) {
	if Subcommands["probe"] == nil {
		t.Fatal("probe subcommand not registered")
	}
	const usage = "usage: solodawn <command> [arguments]\n"
	for _, args := range [][]string{nil, {""}, {"a", "b"}} {
		var stdout, stderr bytes.Buffer
		if code := probeCommand(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("probe(%q) = %d, stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}

	emptyDir := t.TempDir()
	nonEmptyDir := t.TempDir()
	filePath := filepath.Join(nonEmptyDir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{emptyDir, nonEmptyDir} {
		var stdout, stderr bytes.Buffer
		if code := probeCommand([]string{dir}, &stdout, &stderr); code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("probe(%q) = %d, stdout=%q stderr=%q", dir, code, stdout.String(), stderr.String())
		}
	}

	for _, bad := range []string{filepath.Join(emptyDir, "missing"), filePath} {
		var stdout, stderr bytes.Buffer
		if code := probeCommand([]string{bad}, &stdout, &stderr); code != 1 || stderr.Len() == 0 {
			t.Fatalf("probe(%q) = %d, stderr=%q; want 1", bad, code, stderr.String())
		}
	}

	var stderr bytes.Buffer
	if code := probeExit(fmt.Errorf("open /x: %w", syscall.EPERM), &stderr); code != 3 || !strings.Contains(stderr.String(), "operation not permitted") {
		t.Fatalf("probeExit(EPERM) = %d, stderr=%q; want 3", code, stderr.String())
	}
}
