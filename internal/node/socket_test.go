package node

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestListenLocal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node")
	openSocket(t, dir)
	path := filepath.Join(dir, "sock")
	checkSocketModes(t, dir, path)
	if _, err := ListenLocal(dir); err == nil {
		t.Fatal("second listener claimed the live socket")
	}
	if _, err := ListenLocal(filepath.Join(path, "child")); err == nil {
		t.Fatal("created data directory beneath a socket")
	}
}

func TestSocketPermissionFailureClosesListener(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sock")
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
	for _, line := range []string{"", "bad\n", "{}\n", "{\"CMD\":\"status\"}\n",
		"{\"cmd\":\"status\",\"mcp\":\"token\"}\n", "{\"mcp\":\"\"}\n"} {
		if got, raw, err := ReadOpening(bufio.NewReader(strings.NewReader(line))); err == nil || raw != nil || got != (Opening{}) {
			t.Errorf("%q: got %+v, %q, %v", line, got, raw, err)
		}
	}
}

func TestCannotSendCommand(t *testing.T) {
	c := Command{Cmd: "status"}
	if err := c.send(t.TempDir(), io.Discard); err == nil {
		t.Fatal("sent to missing socket")
	}
	conn := struct {
		io.Reader
		io.Writer
	}{new(bytes.Buffer), failingWriter{}}
	if err := c.exchange(conn, io.Discard); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("send error = %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTerminalReplies(t *testing.T) {
	for _, tc := range []struct{ lines, wantErr string }{
		{"{\"done\":true,\"ok\":true}\n", ""},
		{"{\"done\":true,\"error\":\"denied\"}\n", "denied"},
		{"{\"done\":true,\"ok\":true,\"error\":\"denied\"}\n", "expected ok or error in the final socket reply"},
		{"{\"done\":true}\n", "expected ok or error in the final socket reply"},
		{"{\"done\":true,\"ok\":null}\n", "proto: null where the contract has no null"},
		{"{\"done\":null}\n", "proto: null where the contract has no null"},
		{"{\"done\":false}\n", "expected text in the socket reply"},
		{"bad\n", "invalid character 'b' looking for beginning of value"},
		{"{\"done\":true,\"ok\":true}", "EOF"},
	} {
		var out bytes.Buffer
		err := readReplies(bufio.NewReader(strings.NewReader(tc.lines)), &out)
		if errText(err) != tc.wantErr || out.Len() != 0 {
			t.Errorf("%q: stdout %q, error %v", tc.lines, out.String(), err)
		}
	}
}

func TestCommandRoundTrip(t *testing.T) {
	dir := nodeTestDir(t)
	l := openSocket(t, dir)
	received := make(chan string, 1)
	const data = "{\"text\":\"version dev\"}\n{\"text\":\"3F9A 0C21 77BE\"}\n"
	go func() {
		l.SetDeadline(time.Now().Add(2 * time.Second))
		conn, err := l.Accept()
		if err != nil {
			received <- err.Error()
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		line, _ := bufio.NewReader(conn).ReadString('\n')
		received <- line
		io.WriteString(conn, data+"{\"done\":true,\"ok\":true}\n")
	}()
	cmd := Command{Cmd: "status"}
	var stdout bytes.Buffer
	if err := cmd.Run(&stdout); err != nil || stdout.String() != "version dev\n3F9A 0C21 77BE\n" {
		t.Fatalf("status: %q, %v", stdout.String(), err)
	}
	if got := <-received; got != "{\"cmd\":\"status\"}\n" {
		t.Fatalf("request = %q", got)
	}

}

func TestForegroundRuntime(t *testing.T) {
	dir := nodeTestDir(t)
	saved := startNode
	t.Cleanup(func() { startNode = saved })
	startNode = nil
	if err := Foreground(); errText(err) != "node runtime is unavailable" {
		t.Fatalf("foreground returned %v", err)
	}
	startNode = func(path string, l *net.UnixListener) error {
		if path != dir || l.Addr().String() != filepath.Join(dir, "sock") {
			t.Fatalf("unexpected runtime paths %q and %q", path, l.Addr())
		}
		return io.ErrClosedPipe
	}
	if err := Foreground(); err != io.ErrClosedPipe {
		t.Fatalf("foreground returned %v", err)
	}
}

func TestForegroundErrors(t *testing.T) {
	dir := nodeTestDir(t)
	openSocket(t, dir)
	if err := Foreground(); err == nil {
		t.Fatal("foreground claimed an active socket")
	}
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "LocalAppData", "USERPROFILE"} {
		t.Setenv(name, "")
	}
	_, want := DataDir()
	if err := Foreground(); errText(err) != errText(want) {
		t.Fatalf("foreground returned %v, want %v", err, want)
	}
	if err := (Command{Cmd: "status"}).Run(io.Discard); errText(err) != errText(want) {
		t.Fatalf("status returned %v, want %v", err, want)
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

func TestOutputWriteError(t *testing.T) {
	if err := readReplies(bufio.NewReader(strings.NewReader("{\"text\":\"output\"}\n")), failingWriter{}); err != io.ErrClosedPipe {
		t.Fatalf("write error = %v", err)
	}
}

func nodeTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
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
