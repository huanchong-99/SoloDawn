package solodawn_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Section 8.2 of planning/architecture.md. Subpackages belong to their layer.
var allowed = map[string][]string{
	"cmd":                 {"internal/node"},
	"internal/node":       {"internal/host", "internal/home", "internal/rendezvous", "internal/peer", "internal/store", "internal/identity", "internal/proto"},
	"internal/host":       {"internal/proto", "internal/identity", "internal/store"},
	"internal/home":       {"internal/proto", "internal/identity", "internal/store"},
	"internal/rendezvous": {"internal/proto", "internal/identity"},
	"internal/peer":       {"internal/proto", "internal/identity", "internal/store"},
	"internal/store":      {"internal/proto"},
	"internal/identity":   {"internal/proto"},
	"internal/proto":      {},
}

func layer(dir string) string {
	if rest, ok := strings.CutPrefix(dir, "internal/"); ok {
		pkg, _, _ := strings.Cut(rest, "/")
		return "internal/" + pkg
	}
	root, _, _ := strings.Cut(dir, "/")
	return root
}

func TestPackageImports(t *testing.T) {
	const module = "github.com/huanchong-99/SoloDawn"
	for _, root := range []string{"cmd", "internal"} {
		err := fs.WalkDir(os.DirFS("."), root, func(file string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || file == "internal/node/webdist") {
				return fs.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(file, ".go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			from := layer(path.Dir(file))
			for _, imp := range f.Imports {
				name, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return err
				}
				local, ok := strings.CutPrefix(name, module+"/")
				if !ok && name != module {
					continue
				}
				to := layer(local)
				deps, known := allowed[from]
				if !known || (from != to && !slices.Contains(deps, to)) {
					t.Errorf("%s: %s may not import %s (architecture §8.2)", file, from, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
