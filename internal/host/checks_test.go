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
	checkTestFile(t, dir, "guide", "old "+checkTestSecret+"\n")
	checkTestGit(t, dir, "add", ".")
	checkTestGit(t, dir, "commit", "-m", "base")
	checkTestGit(t, dir, "switch", "-c", "task")
	return dir
}

// checkTestSecret matches the PEM pattern of CheckOutboundSecret; split so
// this file does not hold the header itself.
const checkTestSecret = "-----BEGIN " + "PRIVATE KEY-----"

func checkTestRunner() CheckRunner {
	return CheckRunner{}
}

func checkTestGroup(checks ...string) proto.GroupRow {
	return proto.GroupRow{Checks: checks, Limits: proto.Limits{TurnMinutes: 1}}
}

func TestProjectChecksAndBothHardChecks(t *testing.T) {
	dir := checkTestRepo(t)
	checkTestFile(t, dir, "source.go", "a "+checkTestSecret+"\n")
	checkTestGit(t, dir, "add", ".")
	checkTestGit(t, dir, "commit", "-m", "task")
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
		{Name: "secrets", Detail: new(errSecret.Error())},
		{Name: "tests", Detail: new("source changed without a test file")},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v (%v), want %#v", got, err, want)
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
	for _, tc := range []struct {
		patch string
		want  error
	}{
		{"+++ b/file\n@@ -1 +1,3 @@\n>A=1\n>B=2\n>MY_KEY=3\n", errSecret},
		{"+++ b/file\n@@ -1,2 +1,3 @@\n>A=1\n>B=2\n-old\n>MY_KEY=3", nil},
		{"+++ b/file\n@@ -1 +1,2 @@\n>A=1\n>B=2\n@@ -9,0 +12 @@\n>MY_KEY=3", nil},
		{"+++ b/file\n@@ -1 +1,3 @@\n A=1\n>B=2\n>MY_KEY=3", nil},
	} {
		if err := checkAddedLines(tc.patch); err != tc.want {
			t.Errorf("%q: got %v, want %v", tc.patch, err, tc.want)
		}
	}
}

func TestChecksRecognizeTestFilenamesAndDirectories(t *testing.T) {
	for _, name := range []string{
		"dir/foo_test.go", "ui/view.test.ts", "view.spec.jsx", "scripts/test_words.py",
		"tests/fixture", "dir/tests/deep/case.txt", "__tests__/case", "ui/__tests__/a b\nc",
	} {
		if err := checkTestFiles("src/a.go\x00" + name + "\x00"); err != nil {
			t.Errorf("test file %q: %v", name, err)
		}
	}
	for _, name := range []string{
		"mytests/view.ts", "testsuffix/foo_tested.go", "test_dir/source.rs", "view.test.ts/entry.js",
		"tests", "foo.spec", "test.py", "Tests/a.py", "test-fixture.py", "before.go\nafter.txt",
	} {
		err := checkTestFiles("ui/a.TS\x00" + name + "\x00")
		if err == nil || err.Error() != "source changed without a test file" {
			t.Errorf("non-test %q: got %v", name, err)
		}
	}
}

func TestChecksOnlyCountTheContractExtensionsAsSource(t *testing.T) {
	sources := strings.Fields(".cjs .cts .dart .ex .go .js .jsx .mjs .mts .php .py .ts .tsx")
	if !slices.Equal(checkCodeExtensions, sources) {
		t.Fatalf("extensions %q, want %q", checkCodeExtensions, sources)
	}
	for _, name := range []string{"lib/a.cjs", "a.CTS", "app/main.dart", "lib/x.ex", "src/a b\nc.py", "a.js", "v.jsx",
		"m.mjs", "m.mts", "index.php", "web/a.tsx", "web/a.ts", "src/thing.txt.go"} {
		err := checkTestFiles(name + "\x00notes.md\x00")
		if err == nil || err.Error() != "source changed without a test file" {
			t.Errorf("source %q: got %v", name, err)
		}
	}
	others := "docs/a.md\x00icon.png\x00package.json\x00Makefile\x00before.go\nafter.txt\x00ui/a.svelte\x00" +
		"native/one.cpp\x00lib.rs\x00App.java\x00run.sh\x00style.css\x00page.html\x00x.rb\x00a.c\x00a.h\x00" +
		"Main.kt\x00View.swift\x00C.vue\x00exs.exs\x00types.pyi\x00src/thing.go.txt\x00"
	if err := checkTestFiles(others); err != nil {
		t.Errorf("other file types: %v", err)
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
