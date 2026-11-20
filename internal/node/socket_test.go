package node

import (
	"bufio"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
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
	if l, err := ListenLocal(dir); l != nil || err != errRunning {
		t.Fatalf("second listener = %v, %v; want %v", l, err, errRunning)
	}
	if l, err := ListenLocal(filepath.Join(path, "child")); l != nil || !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("listener beneath a socket = %v, %v; want not a directory", l, err)
	}
}

// A node that holds the lock and has not bound yet is running: a second
// node neither removes nor binds the socket.
func TestLockedDataDirIsRunning(t *testing.T) {
	dir := socketRoot(t)
	lock, err := lockData(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := ListenLocal(dir); l != nil || err != errRunning {
		t.Fatalf("listener beside a held lock = %v, %v; want %v", l, err, errRunning)
	}
	if _, err := os.Lstat(filepath.Join(dir, "sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket beside a held lock: %v", err)
	}
	lock.Close()
	openSocket(t, dir)
}

// Nodes that start together: exactly one listens, and its socket stays.
func TestNodesStartingTogether(t *testing.T) {
	for round := range 20 {
		dir := socketRoot(t)
		winners := startTogether(t, dir, 8)
		if len(winners) != 1 {
			t.Fatalf("round %d: %d listeners", round, len(winners))
		}
		conn, err := net.Dial("unix", filepath.Join(dir, "sock"))
		if err != nil {
			t.Fatalf("round %d: dial the winner: %v", round, err)
		}
		conn.Close()
		winners[0].Close()
	}
}

// startTogether starts n listeners on dir at once and returns those that listen.
func startTogether(t *testing.T, dir string, n int) []*Local {
	results := make(chan *Local, n)
	for range n {
		go func() {
			l, err := ListenLocal(dir)
			if err != nil && err != errRunning {
				t.Error(err)
			}
			results <- l
		}()
	}
	var winners []*Local
	for range n {
		if l := <-results; l != nil {
			winners = append(winners, l)
		}
	}
	return winners
}

func TestLockPathError(t *testing.T) {
	dir := socketRoot(t)
	if err := os.Mkdir(filepath.Join(dir, "lock"), 0o700); err != nil {
		t.Fatal(err)
	}
	var open *os.PathError
	if l, err := ListenLocal(dir); l != nil || !errors.As(err, &open) || open.Op != "open" {
		t.Fatalf("listener with a folder as its lock = %v, %v", l, err)
	}
}

func TestSocketPathError(t *testing.T) {
	dir := socketRoot(t)
	if err := os.MkdirAll(filepath.Join(dir, "sock", "file"), 0o700); err != nil {
		t.Fatal(err)
	}
	var remove *os.PathError
	if l, err := ListenLocal(dir); l != nil || !errors.As(err, &remove) || remove.Op != "remove" {
		t.Fatalf("listener with a folder as its socket = %v, %v", l, err)
	}
	openLock(t, dir) // the failed listener released the lock
}

func openLock(t *testing.T, dir string) {
	t.Helper()
	lock, err := lockData(dir)
	if err != nil {
		t.Fatalf("lock after a failed listener: %v", err)
	}
	lock.Close()
}

func TestStaleSocketIsReplaced(t *testing.T) {
	dir := socketRoot(t)
	path := filepath.Join(dir, "sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()
	l := openSocket(t, dir)
	if l.Addr().String() != path {
		t.Fatalf("listener on %q, want %q", l.Addr(), path)
	}
	checkSocketModes(t, dir, path)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial the new socket: %v", err)
	}
	conn.Close()
}

func TestBindFailure(t *testing.T) {
	dir := filepath.Join(socketRoot(t), strings.Repeat("d", 110))
	var bind *net.OpError
	if l, err := ListenLocal(dir); l != nil || !errors.As(err, &bind) || bind.Op != "listen" {
		t.Fatalf("listener on a path beyond the socket limit = %v, %v; want a listen error", l, err)
	}
	openLock(t, dir)
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
	startNode = func(path string, l *Local) error {
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
	if err := Foreground(); err != errRunning {
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

func openSocket(t *testing.T, dir string) *Local {
	t.Helper()
	l, err := ListenLocal(dir)
	if l == nil || err != nil {
		t.Fatalf("listener = %v, error = %v", l, err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestMalformedOpeningGetsOneErrorLine(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan error, 1)
	go func() {
		_, _, _, err := AcceptOpening(server)
		done <- err
	}()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	io.WriteString(client, "{}\n")
	reply, err := io.ReadAll(client)
	if string(reply) != "{\"done\":true,\"error\":\"expected a command or an MCP token\"}\n" || err != nil {
		t.Fatalf("reply %q, %v", reply, err)
	}
	if err := <-done; errText(err) != "expected a command or an MCP token" {
		t.Fatalf("AcceptOpening returned %v", err)
	}
}

func TestOpeningKeepsTheConnection(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go io.WriteString(client, "{\"mcp\":\"token\"}\nstream")
	server.SetDeadline(time.Now().Add(2 * time.Second))
	open, line, r, err := AcceptOpening(server)
	if open != (Opening{MCP: "token"}) || string(line) != "{\"mcp\":\"token\"}\n" || err != nil {
		t.Fatalf("opening %+v, %q, %v", open, line, err)
	}
	if rest := make([]byte, 6); func() bool { _, err := io.ReadFull(r, rest); return err != nil || string(rest) != "stream" }() {
		t.Fatalf("stream after the opening = %q", rest)
	}
}

func TestReadPair(t *testing.T) {
	configured := []string{"https://configured.example"}
	for _, tc := range []struct {
		line, owner string
		want        PairRequest
		err         string
	}{
		{`{"cmd":"pair","secret":"s","name":"laptop"}`, "", PairRequest{Secret: "s", Name: "laptop", Rendezvous: configured}, ""},
		{`{"cmd":"pair","secret":"s","name":"laptop","rendezvous":"https://rv.example"}`, "", PairRequest{Secret: "s", Name: "laptop", Rendezvous: []string{"https://rv.example"}}, ""},
		{`{"cmd":"pair","secret":"s","name":"laptop"}`, "user-1", PairRequest{}, "this node already has an owner; to add a browser, run solodawn pair-browser"},
		{`{"cmd":"pair","secret":"s"}`, "", PairRequest{}, "proto: a field that is always present is missing"},
	} {
		got, err := ReadPair([]byte(tc.line), tc.owner, configured)
		if !reflect.DeepEqual(got, tc.want) || errText(err) != tc.err {
			t.Errorf("%s with owner %q: got %+v, %v; want %+v, %q", tc.line, tc.owner, got, err, tc.want, tc.err)
		}
	}
}

func TestLongOpeningIsRefused(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		client.Write([]byte(strings.Repeat("x", maxOpening)))
	}()
	server.SetDeadline(time.Now().Add(2 * time.Second))
	go io.Copy(io.Discard, client)
	if _, _, _, err := AcceptOpening(server); err != bufio.ErrBufferFull {
		t.Fatalf("AcceptOpening on %d bytes without a line end = %v", maxOpening, err)
	}
}

func TestOpeningLimits(t *testing.T) {
	saved := openingTimeout
	t.Cleanup(func() { openingTimeout = saved })
	openingTimeout = 50 * time.Millisecond
	// A silent client gets the error line once the wait is over.
	client, server := net.Pipe()
	go AcceptOpening(server)
	client.SetDeadline(time.Now().Add(2 * time.Second))
	reply, err := io.ReadAll(client)
	client.Close()
	if string(reply) != "{\"done\":true,\"error\":\"read pipe: i/o timeout\"}\n" || err != nil {
		t.Fatalf("reply to a silent client %q, %v", reply, err)
	}
	// After a good opening the stream has no deadline.
	client, server = net.Pipe()
	defer client.Close()
	defer server.Close()
	go io.WriteString(client, "{\"cmd\":\"status\"}\n")
	_, _, r, err := AcceptOpening(server)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * openingTimeout)
	go io.WriteString(client, "later\n")
	if rest, err := r.ReadString('\n'); rest != "later\n" || err != nil {
		t.Fatalf("stream after the opening = %q, %v", rest, err)
	}
}
