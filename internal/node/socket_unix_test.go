//go:build !windows

package node

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrivateDirChecksAnExistingOne(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(file); errText(err) != file+" is not a directory" {
		t.Fatalf("privateDir on a file = %v", err)
	}
	dir := filepath.Join(root, "dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(dir); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("mode %o, want 700", info.Mode().Perm())
	}
	saved := currentUID
	t.Cleanup(func() { currentUID = saved })
	currentUID = func() int { return os.Getuid() + 1 }
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(dir); errText(err) != dir+" is not owned by the current user" {
		t.Fatalf("privateDir on another user's folder = %v", err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o755 {
		t.Fatalf("another user's folder changed to %o", info.Mode().Perm())
	}
}

// The socket is created mode 0600, before ownerOnly runs, and the process's
// umask is put back.
func TestListenPrivateMode(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	path := filepath.Join(socketRoot(t), "sock")
	l, err := listenPrivate(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v, %v; want 600", info, err)
	}
	if got := syscall.Umask(0o022); got != 0o022 {
		t.Fatalf("umask after the bind = %o, want 022", got)
	}
}
