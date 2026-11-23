package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWorkspaceCloneTempThatCannotBeRemoved(t *testing.T) {
	project := t.TempDir()
	temp := filepath.Join(project, cloneTemp)
	workOK(t, os.Mkdir(temp, 0o755))
	f, err := os.Create(filepath.Join(temp, "partial"))
	workOK(t, err)
	defer f.Close()
	repo, err := EnsureGroupWorkspace(t.Context(), project, "unused", "trunk")
	workEqual(t, repo, "")
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("removing locked clone: %v", err)
	}
	requireAbsent(t, filepath.Join(project, "repo"))
}
