package node

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestListenLocal(t *testing.T) {
	dir := filepath.Join(socketRoot(t), "node")
	openSocket(t, dir)
	path := filepath.Join(dir, "sock")
	checkSocketModes(t, dir, path)
	if l, err := ListenLocal(dir); l != nil || !errors.Is(err, addrInUse) {
		t.Fatalf("second listener = %v, %v; want address in use", l, err)
	}
	if l, err := ListenLocal(filepath.Join(path, "child")); l != nil || !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("listener beneath a socket = %v, %v; want not a directory", l, err)
	}
}

func TestSocketPermissionFailureClosesListener(t *testing.T) {
	path := filepath.Join(socketRoot(t), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	l.SetDeadline(time.Now().Add(2 * time.Second))
	if got, err := secureSocket(l); got != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secureSocket = %v, %v", got, err)
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept = %v, want closed listener", err)
	}
}

func TestOpening(t *testing.T) {
	for _, tc := range []struct {
		line string
		want Opening
	}{
		{`{"cmd":"status"}`, Opening{Cmd: "status"}},
		{`{"cmd":"pair","secret":"secret","name":"label"}`, Opening{Cmd: "pair"}},
		{`{"cmd":"extension","argument":"value"}`, Opening{Cmd: "extension"}},
		{`{"mcp":"token"}`, Opening{MCP: "token"}},
	} {
		r := bufio.NewReader(strings.NewReader(tc.line + "\nstream"))
		got, line, err := ReadOpening(r)
		if err != nil || got != tc.want || string(line) != tc.line+"\n" {
			t.Errorf("%s: got %+v, %q, %v", tc.line, got, line, err)
		}
		if rest, _ := io.ReadAll(r); string(rest) != "stream" {
			t.Errorf("remaining stream = %q", rest)
		}
	}
}

func TestInvalidOpening(t *testing.T) {
	const neither = "expected a command or an MCP token"
	for _, tc := range []struct{ line, want string }{
		{"", "EOF"},
		{"bad\n", "invalid character 'b' looking for beginning of value"},
		{"{}\n", neither},
		{"{\"CMD\":\"status\"}\n", neither},
		{"{\"cmd\":\"status\",\"mcp\":\"token\"}\n", neither},
		{"{\"mcp\":\"\"}\n", neither},
	} {
		got, raw, err := ReadOpening(bufio.NewReader(strings.NewReader(tc.line)))
		if errText(err) != tc.want || raw != nil || got != (Opening{}) {
			t.Errorf("%q: got %+v, %q, %v; want error %q", tc.line, got, raw, err, tc.want)
		}
	}
}

func TestForegroundRuntime(t *testing.T) {
	dir := nodeTestDir(t)
	if err := Foreground(); errText(err) != "node runtime is unavailable" {
		t.Fatalf("foreground returned %v", err)
	}
	saved := startNode
	t.Cleanup(func() { startNode = saved })
	startNode = func(path string, l *net.UnixListener) error {
		if path != dir || l.Addr().String() != filepath.Join(dir, "sock") {
			t.Errorf("runtime got %q and %q, want %q", path, l.Addr(), dir)
		}
		return io.ErrClosedPipe
	}
	if err := Foreground(); err != io.ErrClosedPipe {
		t.Fatalf("foreground returned %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after the runtime returned: %v", err)
	}
}

func TestForegroundErrors(t *testing.T) {
	dir := nodeTestDir(t)
	openSocket(t, dir)
	if err := Foreground(); !errors.Is(err, addrInUse) {
		t.Fatalf("foreground on an active socket returned %v", err)
	}
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "LocalAppData", "USERPROFILE"} {
		t.Setenv(name, "")
	}
	_, want := DataDir()
	if err := Foreground(); errText(err) != errText(want) || want == nil {
		t.Fatalf("foreground returned %v, want %v", err, want)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func checkSocketModes(t *testing.T, dir, path string) {
	t.Helper()
	for name, want := range map[string]os.FileMode{dir: 0o700, path: 0o600} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != want {
			t.Errorf("%s: mode %o, want %o", name, info.Mode().Perm(), want)
		}
	}
}

// socketRoot returns a short temporary directory: macOS limits a socket path
// to 103 bytes, and its temporary directory is deep.
func socketRoot(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if runtime.GOOS == "darwin" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "sd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return root
}

func nodeTestDir(t *testing.T) string {
	t.Helper()
	root := socketRoot(t)
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "LocalAppData"} {
		t.Setenv(name, root)
	}
	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func openSocket(t *testing.T, dir string) *net.UnixListener {
	t.Helper()
	l, err := ListenLocal(dir)
	if l == nil || err != nil {
		t.Fatalf("listener = %v, error = %v", l, err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}
