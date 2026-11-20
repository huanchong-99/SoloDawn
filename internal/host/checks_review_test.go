package host

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// checkReviewRepo commits files on base, applies change on the task branch
// and commits it there.
func checkReviewRepo(t *testing.T, base map[string]string, change func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	checkTestGit(t, dir, "init", "-b", "base")
	checkTestGit(t, dir, "commit", "--allow-empty", "-m", "empty")
	for name, content := range base {
		checkTestFile(t, dir, name, content)
	}
	checkTestGit(t, dir, "add", "-A")
	checkTestGit(t, dir, "commit", "--allow-empty", "-m", "base")
	checkTestGit(t, dir, "switch", "-c", "task")
	change(dir)
	checkTestGit(t, dir, "add", "-A")
	checkTestGit(t, dir, "commit", "--allow-empty", "-m", "task")
	return dir
}

func checkHardItems(t *testing.T, dir string) []proto.CheckItem {
	t.Helper()
	result, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup())
	if err != nil {
		t.Fatal(err)
	}
	return result.Items
}

func TestTestsCheckIgnoresDeletedFiles(t *testing.T) {
	failed := proto.CheckItem{Name: "tests", Detail: new("source changed without a test file")}
	passed := proto.CheckItem{Name: "tests", OK: true}
	for _, tc := range []struct {
		name   string
		base   map[string]string
		change func(dir string)
		want   proto.CheckItem
	}{
		{"source changed, its test deleted", map[string]string{"a.go": "a\n", "a_test.go": "t\n"}, func(dir string) {
			checkTestFile(t, dir, "a.go", "a changed\n")
			checkTestGit(t, dir, "rm", "-q", "a_test.go")
		}, failed},
		{"only a source deleted", map[string]string{"old.go": "old\n"}, func(dir string) {
			checkTestGit(t, dir, "rm", "-q", "old.go")
		}, passed},
		{"test renamed to a source", map[string]string{"x_test.go": "package x\n\nfunc Example() {}\n"}, func(dir string) {
			checkTestGit(t, dir, "mv", "x_test.go", "y.go")
		}, failed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := checkHardItems(t, checkReviewRepo(t, tc.base, tc.change))
			if !reflect.DeepEqual(items[1], tc.want) {
				t.Fatalf("tests item %+v, want %+v", items[1], tc.want)
			}
		})
	}
}

func TestSecretsCheckReadsFilesGitTakesAsBinary(t *testing.T) {
	key := checkTestSecret + "\nMIIEvQIBADANBg\n" + keyEnd + "PRIVATE KEY-----\n"
	for name, change := range map[string]func(dir string){
		"binary attribute": func(dir string) { checkTestFile(t, dir, "server.pem", key) },
		"NUL byte":         func(dir string) { checkTestFile(t, dir, "blob", "\x00\n"+key) },
	} {
		t.Run(name, func(t *testing.T) {
			dir := checkReviewRepo(t, map[string]string{".gitattributes": "*.pem binary\n"}, change)
			want := proto.CheckItem{Name: "secrets", Detail: new(errSecret.Error())}
			if items := checkHardItems(t, dir); !reflect.DeepEqual(items[0], want) {
				t.Fatalf("secrets item %+v, want %+v", items[0], want)
			}
		})
	}
}

func TestSkipWorktreeIsRefusedByName(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  string
	}{
		{1, "f00"},
		{10, "f00, f01, f02, f03, f04, f05, f06, f07, f08, f09"},
		{11, "f00, f01, f02, f03, f04, f05, f06, f07, f08, f09, and 1 more"},
	} {
		t.Run(strconv.Itoa(tc.count), func(t *testing.T) {
			files := map[string]string{}
			args := []string{"update-index", "--skip-worktree", "--"}
			for i := range tc.count {
				files[fmt.Sprintf("f%02d", i)] = "checked\n"
				args = append(args, fmt.Sprintf("f%02d", i))
			}
			dir := checkReviewRepo(t, files, func(string) {})
			checkTestGit(t, dir, args...)
			result, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup("echo ran > marker"))
			if !errors.Is(err, ErrSkipWorktree) || err.Error() != ErrSkipWorktree.Error()+": "+tc.want || !reflect.DeepEqual(result, proto.ChecksBody{}) {
				t.Fatalf("skip-worktree: %#v, %v", result, err)
			}
		})
	}
}

// core.ignoreStat sets the assume-unchanged bit on every path Git adds; the
// bit is cleared before the status, so a change shows and a clean tree runs.
func TestAssumeUnchangedDoesNotHideChanges(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ignoreStat bool
		change     bool
	}{
		{"ignoreStat, clean", true, false},
		{"ignoreStat, changed", true, true},
		{"bit set by hand, changed", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ignoreStat {
				t.Setenv("GIT_CONFIG_COUNT", "1")
				t.Setenv("GIT_CONFIG_KEY_0", "core.ignoreStat")
				t.Setenv("GIT_CONFIG_VALUE_0", "true")
			}
			dir := checkReviewRepo(t, map[string]string{"config.txt": "checked\n"}, func(string) {})
			if !tc.ignoreStat {
				checkTestGit(t, dir, "update-index", "--assume-unchanged", "config.txt")
			}
			if tags := checkTestGit(t, dir, "ls-files", "-v"); tags != "h config.txt" {
				t.Fatalf("ls-files -v = %q; the bit should be set", tags)
			}
			if tc.change {
				checkTestFile(t, dir, "config.txt", "not what was committed\n")
			}
			result, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup())
			want := proto.ChecksBody{Stage: "local", OK: true, Commit: checkTestGit(t, dir, "rev-parse", "HEAD"), Items: []proto.CheckItem{
				{Name: "secrets", OK: true}, {Name: "tests", OK: true},
			}}
			var wantErr error
			if tc.change {
				want, wantErr = proto.ChecksBody{}, ErrDirtyCheckTree
			}
			if err != wantErr || !reflect.DeepEqual(result, want) {
				t.Fatalf("got %#v, %v; want %#v, %v", result, err, want, wantErr)
			}
		})
	}
}

func TestAddedLinesOverTheLimit(t *testing.T) {
	saved := addedLineLimit
	t.Cleanup(func() { addedLineLimit = saved })
	addedLineLimit = 10
	for _, tc := range []struct {
		content string
		want    proto.CheckItem
	}{
		{"01234567\n", proto.CheckItem{Name: "secrets", OK: true}},
		{"012345678\n", proto.CheckItem{Name: "secrets", Detail: new("too large to check: 11 bytes of added lines, over the limit of 10")}},
	} {
		dir := checkReviewRepo(t, nil, func(dir string) { checkTestFile(t, dir, "notes", tc.content) })
		if items := checkHardItems(t, dir); !reflect.DeepEqual(items[0], tc.want) {
			t.Errorf("%q: secrets item %+v, want %+v", tc.content, items[0], tc.want)
		}
	}
}

func TestScanStopsAtTheLimitOfTheHardChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := checkAddedLines(ctx, ">x\n"); err != errHardTimeout {
		t.Fatalf("scan after the limit: %v", err)
	}
}

func TestCheckOutputRedactsKeyLongerThanTail(t *testing.T) {
	dir := t.TempDir()
	key := checkTestSecret + "\n" + strings.Repeat("MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7\n", 85) + keyEnd + "PRIVATE KEY-----\n"
	checkTestFile(t, dir, "key.pem", key)
	command := "cat key.pem"
	if runtime.GOOS == "windows" {
		command = "type key.pem"
	}
	want := proto.CheckItem{Name: command, OK: true, Detail: new("[redacted]\n")}
	if got := runProjectCheck(t.Context(), dir, command); len(key) <= checkTailLimit || !reflect.DeepEqual(got, want) {
		t.Fatalf("%d-byte key: %q %v %q, want detail %q", len(key), got.Name, got.OK, *got.Detail, *want.Detail)
	}
}
