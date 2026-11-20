package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func checkTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=fixture"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s (%v)", args, out, err)
	}
	return strings.TrimSpace(string(out))
}

func checkTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func checkTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	checkTestGit(t, dir, "init", "-b", "base")
	checkTestFile(t, dir, "guide", "old marked secret\n")
	checkTestGit(t, dir, "add", ".")
	checkTestGit(t, dir, "commit", "-m", "base")
	checkTestGit(t, dir, "switch", "-c", "task")
	return dir
}

func checkTestRunner() CheckRunner {
	return CheckRunner{
		CheckSecret: func(text string) error {
			if strings.Contains(text, "marked secret") {
				return errors.New("secret detected")
			}
			return nil
		},
		ClassifyPath: func(path string) (bool, bool) {
			return path == "source.go", path == "source_test.go"
		},
	}
}

func checkTestGroup(checks ...string) proto.GroupRow {
	return proto.GroupRow{Checks: checks, Limits: proto.Limits{TurnMinutes: 1}}
}

func TestProjectChecksAndBothHardChecks(t *testing.T) {
	dir := checkTestRepo(t)
	checkTestFile(t, dir, "source.go", "a marked secret\n")
	checkTestGit(t, dir, "add", ".")
	checkTestGit(t, dir, "commit", "-m", "task")
	checkTestFile(t, dir, ".git/info/exclude", "first\n")
	commands := []string{"echo one > first", "exit 12", "git hash-object first"}

	// Use Git's own hash on the platform's shell output (LF or CRLF).
	first := "one\n"
	if runtime.GOOS == "windows" {
		commands[0] = ">first echo one"
		first = "one\r\n"
	}

	sample := t.TempDir()
	checkTestFile(t, sample, "first", first)
	digest := checkTestGit(t, sample, "hash-object", "first")
	head := checkTestGit(t, dir, "rev-parse", "HEAD")
	got, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup(commands...))
	want := proto.ChecksBody{Stage: "local", Commit: head, Items: []proto.CheckItem{
		{Name: commands[0], OK: true},
		{Name: commands[1], Detail: new("exit status 12")},
		{Name: commands[2], OK: true, Detail: new(digest + "\n")},
		{Name: "secrets", Detail: new("secret detected")},
		{Name: "tests", Detail: new("source changed without a test file")},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v (%v), want %#v", got, err, want)
	}
}

func TestChecksStopOnTreeOrCommitChange(t *testing.T) {
	for _, tc := range []struct{ command, detail string }{
		{"echo changed > guide", ErrDirtyCheckTree.Error()},
		{"git switch -q base", "conflict: HEAD changed during checks"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			dir := checkTestRepo(t)
			checkTestGit(t, dir, "commit", "--allow-empty", "-m", "task head")
			head := checkTestGit(t, dir, "rev-parse", "HEAD")
			got, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup(tc.command, "echo ran > marker"))
			want := proto.ChecksBody{Stage: "local", Commit: head, Items: []proto.CheckItem{
				{Name: tc.command, Detail: &tc.detail}, {Name: "secrets", OK: true}, {Name: "tests", OK: true},
			}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("tree changed: %#v (%v), want %#v", got, err, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "marker")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("later check ran: %v", err)
			}
		})
	}
}

func TestTurnDeadlineStillRunsBothHardChecks(t *testing.T) {
	dir := checkTestRepo(t)
	group := checkTestGroup("echo late")
	group.Limits.TurnMinutes = 0
	result, err := checkTestRunner().Run(t.Context(), dir, "base", group)
	want := proto.ChecksBody{Stage: "local", Commit: checkTestGit(t, dir, "rev-parse", "HEAD"), Items: []proto.CheckItem{
		{Name: "echo late", Detail: new(context.DeadlineExceeded.Error())},
		{Name: "secrets", OK: true}, {Name: "tests", OK: true},
	}}
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("deadline: %#v (%v), want %#v", result, err, want)
	}
}

func TestChecksPinCommitAndIgnoreRemovedSecrets(t *testing.T) {
	dir := checkTestRepo(t)
	checkTestFile(t, dir, "guide", "replacement\n")
	checkTestFile(t, dir, "source.go", "code\n")
	checkTestFile(t, dir, "source_test.go", "test\n")
	checkTestGit(t, dir, "add", ".")
	checkTestGit(t, dir, "commit", "-m", "task")
	head := checkTestGit(t, dir, "rev-parse", "HEAD")
	got, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup())
	want := proto.ChecksBody{Stage: "local", OK: true, Commit: head, Items: []proto.CheckItem{
		{Name: "secrets", OK: true}, {Name: "tests", OK: true},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v (%v), want %#v", got, err, want)
	}
}

func TestChecksRefuseDirtyTrees(t *testing.T) {
	dir := checkTestRepo(t)
	checkTestGit(t, dir, "config", "status.showUntrackedFiles", "no")
	checkTestFile(t, dir, "untracked", "staged later\n")
	for _, stage := range []bool{false, true} {
		if stage {
			checkTestGit(t, dir, "add", "untracked")
		}
		result, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup("echo ran > marker"))
		if err != ErrDirtyCheckTree || !reflect.DeepEqual(result, proto.ChecksBody{}) {
			t.Fatalf("dirty: %#v, %v", result, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("check marker: %v", err)
	}
}

func TestChecksSurfaceGitFailures(t *testing.T) {
	dir := checkTestRepo(t)
	_, err := checkTestRunner().Run(t.Context(), dir, "missing", checkTestGroup())
	if err == nil || !strings.HasPrefix(err.Error(), "git diff: exit status 128: fatal: ") {
		t.Fatalf("diff: %v", err)
	}
	_, err = checkTestRunner().Run(t.Context(), t.TempDir(), "base", checkTestGroup())
	if err == nil || !strings.HasPrefix(err.Error(), "git status: exit status 128: fatal: ") {
		t.Fatalf("status: %v", err)
	}
}

func TestChecksRequireBothGitDiffReads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX git fixture")
	}
	dir := t.TempDir()
	t.Setenv("ComSpec", "windows-only")
	script := "#!/bin/sh\ncase \"$2\" in --name-only) echo \"missing${ComSpec}\" >&2; exit 2;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := checkTestRunner().hardChecks(t.Context(), dir, "base...head")
	if err == nil || err.Error() != "git diff: exit status 2: missing" {
		t.Fatalf("name-only: %v", err)
	}
}

func TestProjectCheckPlatformEnvironment(t *testing.T) {
	command := `test -z "${ComSpec+x}" && echo ok`
	line := "ok\n"
	if runtime.GOOS == "windows" {
		command, line = "if defined ComSpec echo ok", "ok\r\n"
	} else {
		t.Setenv("ComSpec", "windows-only")
	}
	want := proto.CheckItem{Name: command, OK: true, Detail: &line}
	if got := runProjectCheck(t.Context(), t.TempDir(), command); !reflect.DeepEqual(got, want) {
		t.Fatalf("environment probe: %#v, want %#v", got, want)
	}
}

func TestChecksOnlyConsecutiveAddedLinesReachSecretMatcher(t *testing.T) {
	var got []string
	runner := CheckRunner{CheckSecret: func(text string) error { got = append(got, text); return nil }}
	patch := "+++ b/file\n@@ -1 +1,3 @@\n-old\n>++literal\n>>text\n>\n@@ -9,0 +12 @@\n>last"
	if err := runner.checkAddedLines(patch); err != nil {
		t.Fatal(err)
	}
	if want := []string{"++literal\n>text\n\n", "last"}; !slices.Equal(got, want) {
		t.Fatalf("matcher inputs %q, want %q", got, want)
	}
}

func TestChecksNULDelimitedNames(t *testing.T) {
	var got []string
	runner := CheckRunner{ClassifyPath: func(name string) (bool, bool) {
		got = append(got, name)
		return false, false
	}}
	err := runner.checkTestFiles("one file\x00line\nbreak\x00")
	if err != nil || !slices.Equal(got, []string{"one file", "line\nbreak"}) {
		t.Fatalf("names %q, err %v", got, err)
	}
}

func TestCheckEnvironment(t *testing.T) {
	common := strings.Fields("PATH=/bin HOME=home USER=u LOGNAME=u SHELL=sh LANG=C LC_ALL=C LC_FOO=bar TZ=UTC TMPDIR=a TEMP=b TMP=c")
	windows := strings.Fields("SystemRoot=root ComSpec=cmd PATHEXT=.EXE USERPROFILE=p HOMEDRIVE=d HOMEPATH=h APPDATA=a LOCALAPPDATA=l lc_Mixed=value Path=Mixed")
	foreign := strings.Fields("GIT_TOKEN=set NODE_OPTIONS=flag PATH_EXTRA=bad LCALL=x =C:=drive")
	all := slices.Concat(common, windows, foreign)
	if got := checkEnvironment(all, false); !slices.Equal(got, common) {
		t.Fatalf("Unix env %q, want %q", got, common)
	}
	if got, want := checkEnvironment(all, true), slices.Concat(common, windows); !slices.Equal(got, want) {
		t.Fatalf("Windows env %q, want %q", got, want)
	}
	got := checkEnvironment(foreign, false)
	if got == nil || len(got) != 0 {
		t.Fatalf("empty allow list must not inherit the parent environment: %#v", got)
	}
}

func TestCheckOutputKeepsLastFourKiB(t *testing.T) {
	const limit = 4 * 1024
	var tail checkTail
	var output string
	for _, chunk := range []string{"", "first", strings.Repeat("a", 5000), strings.Repeat("b", 4095), "c", "d"} {
		n, err := tail.Write([]byte(chunk))
		output += chunk
		want := output[max(0, len(output)-limit):]
		if err != nil || n != len(chunk) || string(tail) != want {
			t.Fatalf("write %d bytes: n=%d, err=%v, tail matches=%v", len(chunk), n, err, string(tail) == want)
		}
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
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell pipeline")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := t.TempDir()
	fifo := checkTestFIFO(t, dir)
	check := "(echo ready > ready; sleep 30; echo late) | cat"
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
		t.Fatalf("cancelled check: %#v, want %#v", got, want)
	}
	if time.Since(start) >= time.Second {
		t.Fatal("pipeline kept the output pipe open after cancellation")
	}
}

func TestWindowsCheckShellCommandLine(t *testing.T) {
	t.Setenv("ComSpec", "cmd.exe")
	var attributes struct{ CmdLine string }
	command := `echo "two words" && echo %PATH%`
	cmd := checkShell(t.Context(), command, true, reflect.ValueOf(&attributes).Elem())
	if !slices.Equal(cmd.Args, []string{"cmd.exe"}) || attributes.CmdLine != `cmd.exe /d /s /c "echo "two words" && echo %PATH%"` {
		t.Fatalf("cmd arguments %q and command line %q", cmd.Args, attributes.CmdLine)
	}
	if runtime.GOOS == "windows" {
		return
	}
	checkTaskkillArgs(t, cmd)
}

func checkTaskkillArgs(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	t.Setenv("TASKKILL_ARGS", out)
	if err := os.WriteFile(filepath.Join(dir, "taskkill.exe"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TASKKILL_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cmd.Process = &os.Process{Pid: 12345}
	if err := cmd.Cancel(); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(out)
	if err != nil || string(args) != "/f\n/t\n/pid\n12345\n" {
		t.Fatalf("taskkill arguments: %q (%v)", args, err)
	}
}
