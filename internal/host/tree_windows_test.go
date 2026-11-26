package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The test binary, copied as taskkill.exe, writes its arguments and exits.
func init() {
	out := os.Getenv("SOLODAWN_TEST_TASKKILL_ARGS")
	if out == "" || !strings.EqualFold(filepath.Base(os.Args[0]), "taskkill.exe") {
		return
	}
	if err := os.WriteFile(out, []byte(strings.Join(os.Args[1:], " ")), 0600); err != nil {
		panic(err)
	}
	os.Exit(0)
}

func TestWindowsStopKillsTheProcessTree(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := agtLinkOrCopy(exe, filepath.Join(dir, "taskkill.exe")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "args")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_TEST_TASKKILL_ARGS", out)
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	cmd := exec.CommandContext(t.Context(), "cmd.exe")
	detachTerminal(cmd)
	cmd.Process = &os.Process{Pid: 12345}
	if err := cmd.Cancel(); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(out)
	if err != nil || string(args) != "/f /t /pid 12345" {
		t.Fatalf("taskkill arguments: %q (%v)", args, err)
	}
}

func TestWindowsTreeJobErrors(t *testing.T) {
	fail := func(*windows.SecurityAttributes, *uint16) (windows.Handle, error) {
		return 0, windows.ERROR_ACCESS_DENIED
	}
	if job, err := newTreeJob(0, fail); job != 0 || err != windows.ERROR_ACCESS_DENIED {
		t.Fatalf("create job: %v, %v", job, err)
	}
	if err := setupTreeJob(0, uint32(os.Getpid())); err != windows.ERROR_INVALID_HANDLE {
		t.Fatalf("configure invalid job: %v", err)
	}
	treeKiller(&exec.Cmd{Process: &os.Process{Pid: 0}})()
}
