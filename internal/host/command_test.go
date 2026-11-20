package host

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

func TestCommandLimits(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
		want time.Duration
	}{
		{"git", []string{"clone", "--branch", "main", "--", "src", "dst"}, 60 * time.Minute},
		{"git", []string{"push", "origin", "refs/heads/x:refs/heads/x"}, 60 * time.Minute},
		{"git", []string{"fetch", "origin"}, 10 * time.Minute},
		{"git", []string{"pull"}, 10 * time.Minute},
		{"git", nil, 10 * time.Minute},
		{"gh", []string{"push"}, 10 * time.Minute},
		{"gh", []string{"pr", "merge", "1"}, 10 * time.Minute},
	} {
		if got := limitOf(c.name, c.args); got != c.want {
			t.Errorf("%s %q: limit %v, want %v", c.name, c.args, got, c.want)
		}
	}
}

// The default stays pinned here: a test that shortens the delay cannot see a
// changed initializer.
func TestCommandWaitDelayDefault(t *testing.T) {
	if commandWaitDelay != 10*time.Second {
		t.Fatalf("wait delay %v, want 10s", commandWaitDelay)
	}
}

// A missing folder is reported as such on every system. Once SysProcAttr is
// set, os/exec skips its own check of the folder, and Windows would say only
// "The directory name is invalid", which is not fs.ErrNotExist.
func TestCommandInAMissingFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	_, _, err := runIn(t.Context(), dir, "git", "status")
	pathErr, ok := errors.AsType[*fs.PathError](err)
	if !ok || pathErr.Op != "stat" || pathErr.Path != dir || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("got %#v, want stat %s: not exist", err, dir)
	}
}
