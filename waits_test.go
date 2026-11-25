package solodawn_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// planning/contracts/diagnostics.md section 4: a wait that runs out says what
// it saw. A string of a test file that starts like a timeout names it.
const (
	timeoutStart = "timed out "
	timeoutSaw   = "; saw "
)

// TestTimeoutsSayWhatTheySaw checks the test files of features and internal.
func TestTimeoutsSayWhatTheySaw(t *testing.T) {
	for _, bad := range timeoutsWithoutSaw(os.DirFS("."), "features", "internal") {
		t.Error(bad)
	}
}

func TestTimeoutsWithoutSawAreFound(t *testing.T) {
	fsys := fstest.MapFS{
		"internal/a/a_test.go": {Data: []byte("package a\nvar _ = []string{\n\t\"timed out after 1s waiting for x; saw 0: \",\n\t\"timed out waiting for x\",\n\t\"timed out\",\n\t`timed out %d`,\n}\n")},
		"internal/a/a.go":      {Data: []byte("package a\nvar _ = \"timed out waiting\"\n")},
		"other/b_test.go":      {Data: []byte("package b\nvar _ = \"timed out waiting\"\n")},
		"internal/a/c_test.go": {Data: []byte("package a\nvar _ = 1 +\n")},
	}
	got := strings.Join(timeoutsWithoutSaw(fsys, "internal"), "\n")
	want := "internal/a/a_test.go:4: \"timed out waiting for x\" has no \"; saw \"\n" +
		"internal/a/a_test.go:6: \"timed out %d\" has no \"; saw \"\n" +
		"internal/a/c_test.go: " + parseErrorOf(fsys, "internal/a/c_test.go")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func parseErrorOf(fsys fs.FS, file string) string {
	src, _ := fs.ReadFile(fsys, file)
	_, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
	return err.Error()
}

// timeoutsWithoutSaw lists the strings of the test files under roots that
// start with the words of a timeout and do not hold the list of what was seen.
func timeoutsWithoutSaw(fsys fs.FS, roots ...string) []string {
	var bad []string
	for _, root := range roots {
		fs.WalkDir(fsys, root, func(file string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(file, "_test.go") {
				bad = append(bad, timeoutsInFile(fsys, file)...)
			}
			return nil
		})
	}
	return bad
}

func timeoutsInFile(fsys fs.FS, file string) []string {
	src, err := fs.ReadFile(fsys, file)
	if err != nil {
		return []string{file + ": " + err.Error()}
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		return []string{file + ": " + err.Error()}
	}
	var bad []string
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil && strings.HasPrefix(s, timeoutStart) && !strings.Contains(s, timeoutSaw) {
			bad = append(bad, fmt.Sprintf("%s:%d: %q has no %q", file, fset.Position(lit.Pos()).Line, s, timeoutSaw))
		}
		return true
	})
	return bad
}
