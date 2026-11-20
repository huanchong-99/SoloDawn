package host

import (
	"reflect"
	"runtime"
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
	checkTestGit(t, dir, "commit", "-m", "base")
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
	key := checkTestSecret + "\nMIIEvQIBADANBg\n-----END PRIVATE KEY-----\n"
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

func TestChecksRefuseHiddenTrackedChanges(t *testing.T) {
	for _, bit := range []string{"--skip-worktree", "--assume-unchanged"} {
		t.Run(bit, func(t *testing.T) {
			dir := checkReviewRepo(t, map[string]string{"config.txt": "checked\n"}, func(string) {})
			checkTestGit(t, dir, "update-index", bit, "config.txt")
			checkTestFile(t, dir, "config.txt", "not what was committed\n")
			if status := checkTestGit(t, dir, "status", "--porcelain"); status != "" {
				t.Fatalf("status shows %q; the bit should hide the change", status)
			}
			result, err := checkTestRunner().Run(t.Context(), dir, "base", checkTestGroup("echo ran > marker"))
			if err != ErrDirtyCheckTree || !reflect.DeepEqual(result, proto.ChecksBody{}) {
				t.Fatalf("hidden change: %#v, %v", result, err)
			}
		})
	}
}

func TestCheckOutputRedactsKeyLongerThanTail(t *testing.T) {
	dir := t.TempDir()
	key := checkTestSecret + "\n" + strings.Repeat("MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7\n", 85) + "-----END PRIVATE KEY-----\n"
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
