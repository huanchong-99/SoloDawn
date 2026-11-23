package host

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
	"golang.org/x/sys/windows"
)

func init() {
	name := os.Getenv("SOLODAWN_TEST_CHECK_COMMAND")
	if name == "" || !strings.EqualFold(strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"), strings.TrimSuffix(name, ".exe")) {
		return
	}
	switch name {
	case "taskkill.exe":
		forgeOK(os.WriteFile(os.Getenv("SOLODAWN_TEST_CHECK_ARGS"), []byte(strings.Join(os.Args[1:], " ")), 0600))
		os.Exit(0)
	case "git.exe":
		os.Exit(checkGitResponse())
	}
}

func checkGitResponse() int {
	switch os.Args[1] {
	case os.Getenv("SOLODAWN_TEST_CHECK_FAIL"):
		fmt.Fprintln(os.Stderr, "broken")
		return 3
	case "ls-files":
		if os.Getenv("SOLODAWN_TEST_CHECK_FAIL") == "update-index" {
			fmt.Print("h guide\x00")
		}
	case "diff":
		if os.Args[2] == "--name-only" {
			fmt.Fprintln(os.Stderr, "missing")
			return 2
		}
	}
	return 0
}

func fakeWindowsCheckCommand(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	exe := mustForge(os.Executable())
	forgeOK(os.WriteFile(filepath.Join(dir, name), mustForge(os.ReadFile(exe)), 0700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SOLODAWN_TEST_CHECK_COMMAND", name)
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
}

func TestWindowsCheckShellCommandLine(t *testing.T) {
	t.Setenv("ComSpec", "cmd.exe")
	command := `echo "two words" && echo %PATH%`
	cmd := checkShell(t.Context(), command)
	if !slices.Equal(cmd.Args, []string{"cmd.exe"}) || cmd.SysProcAttr.CmdLine != `cmd.exe /d /s /c "echo "two words" && echo %PATH%"` {
		t.Fatalf("cmd arguments %q and command line %q", cmd.Args, cmd.SysProcAttr.CmdLine)
	}
	checkTaskkillArgs(t, cmd)
}

func checkTaskkillArgs(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	fakeWindowsCheckCommand(t, "taskkill.exe")
	out := filepath.Join(t.TempDir(), "args")
	t.Setenv("SOLODAWN_TEST_CHECK_ARGS", out)
	cmd.Process = &os.Process{Pid: 12345}
	if err := cmd.Cancel(); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(out)
	if err != nil || string(args) != "/f /t /pid 12345" {
		t.Fatalf("taskkill arguments: %q (%v)", args, err)
	}
}

func TestWindowsCleanTreeSurfacesGitFailures(t *testing.T) {
	fakeWindowsCheckCommand(t, "git.exe")
	for _, name := range []string{"status", "update-index"} {
		t.Setenv("SOLODAWN_TEST_CHECK_FAIL", name)
		_, err := cleanCheckHead(t.Context(), t.TempDir())
		if want := "git " + name + ": exit status 3: broken"; err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %q", name, err, want)
		}
	}
}

func TestWindowsChecksRequireBothGitDiffReads(t *testing.T) {
	fakeWindowsCheckCommand(t, "git.exe")
	_, err := checkTestRunner().hardChecks(t.Context(), t.TempDir(), "base...head")
	if err == nil || err.Error() != "git diff: exit status 2: missing" {
		t.Fatalf("name-only: %v", err)
	}
}

func TestWindowsCanceledCheckRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := checkTestRunner().Run(ctx, t.TempDir(), "base", proto.GroupRow{})
	if err != context.Canceled || !reflect.DeepEqual(result, proto.ChecksBody{}) {
		t.Fatalf("cancelled run: %#v, %v", result, err)
	}
	hard, stop := hardCheckContext(ctx)
	defer stop()
	<-hard.Done()
	if hard.Err() != context.Canceled {
		t.Fatalf("hard check context: %v", hard.Err())
	}
}

func TestWindowsHardCheckDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()
	item, err := hardItem(ctx, "tests", ctx.Err(), func() error { panic("verdict after timeout") })
	want := proto.CheckItem{Name: "tests", Detail: new("timed out")}
	if err != nil || !reflect.DeepEqual(item, want) {
		t.Fatalf("hard check timeout: %#v, %v", item, err)
	}
}

func TestWindowsCheckJobErrors(t *testing.T) {
	fail := func(*windows.SecurityAttributes, *uint16) (windows.Handle, error) {
		return 0, windows.ERROR_ACCESS_DENIED
	}
	if job, err := newCheckJob(0, fail); job != 0 || err != windows.ERROR_ACCESS_DENIED {
		t.Fatalf("create job: %v, %v", job, err)
	}
	if err := setupCheckJob(0, uint32(os.Getpid())); err != windows.ERROR_INVALID_HANDLE {
		t.Fatalf("configure invalid job: %v", err)
	}
	killCheckTree(&exec.Cmd{Process: &os.Process{Pid: 0}})()
}
