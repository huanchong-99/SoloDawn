//go:build !windows

package node

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrivateDirRefusesAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(file); errText(err) != file+" is not a directory" {
		t.Fatalf("privateDir on a file = %v", err)
	}
}

func TestPrivateDirFixesTheMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(dir); err != nil {
		t.Fatal(err)
	}
	checkMode(t, dir, 0o700)
}

func TestPrivateDirRefusesAnotherUsersFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	saved := currentUID
	t.Cleanup(func() { currentUID = saved })
	currentUID = func() int { return os.Getuid() + 1 }
	if err := privateDir(dir); errText(err) != dir+" is not owned by the current user" {
		t.Fatalf("privateDir on another user's folder = %v", err)
	}
	checkMode(t, dir, 0o755)
}

func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
		t.Fatalf("%s: %v, %v; want mode %o", path, info, err, want)
	}
}

// The socket is created for its owner only, before ownerOnly runs, and the
// process's umask is put back.
func TestListenPrivateMode(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	path := filepath.Join(socketRoot(t), "sock")
	l, err := listenPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	checkMode(t, path, 0o700)
	if got := syscall.Umask(0o022); got != 0o022 {
		t.Fatalf("umask after the bind = %o, want 022", got)
	}
}
