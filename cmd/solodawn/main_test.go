package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func TestRun(t *testing.T) {
	const usage = "usage: solodawn <command> [arguments]\n"
	const flags = "Usage of rendezvous:\n  -listen string\n    \tHTTP signalling and static file address (default \"127.0.0.1:8080\")\n"
	for _, tc := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"version"}, 0, "dev\n", ""},
		{nil, 2, "", usage},
		{[]string{"rendezvous", "--listen", "bad", "extra"}, 2, "", flags},
		{[]string{"rendezvous", "--listen", "bad", "--unknown"}, 2, "", "flag provided but not defined: -unknown\n" + flags},
		{[]string{"rendezvous", "--listen", "bad"}, 1, "", "listen tcp: address bad: missing port in address\n"},
		{[]string{"unknown"}, 2, "", usage},
		{[]string{"version", "extra"}, 2, "", usage},
		{[]string{"node", "extra"}, 2, "", usage},
		{[]string{"status", "extra"}, 2, "", usage},
		{[]string{"pair"}, 2, "", usage},
		{[]string{"pair", ""}, 2, "", usage},
		{[]string{"pair", "--name"}, 2, "", usage},
		{[]string{"pair", "secret", "--name"}, 2, "", usage},
		{[]string{"pair", "secret", "--other", "name"}, 2, "", usage},
	} {
		var stdout, stderr bytes.Buffer
		code := run(tc.args, &stdout, &stderr)
		if code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want %d, %q, %q", tc.args, code, stdout.String(), stderr.String(), tc.code, tc.stdout, tc.stderr)
		}
	}
}

func TestLocalArgumentsReachTheSocket(t *testing.T) {
	dir := testDataDir(t)
	l := testListener(t, dir)
	for _, tc := range []struct {
		args []string
		line string
	}{
		{[]string{"status"}, `{"cmd":"status"}`},
		{[]string{"open"}, `{"cmd":"open"}`},
		{[]string{"pair-browser"}, `{"cmd":"pair-browser"}`},
		{[]string{"pair", "secret"}, `{"cmd":"pair","secret":"secret","name":""}`},
		{[]string{"pair", "secret", "--name", "label with spaces"}, `{"cmd":"pair","secret":"secret","name":"label with spaces"}`},
	} {
		received := replyToCommand(l)
		var out, stderr bytes.Buffer
		if code := run(tc.args, &out, &stderr); code != 0 || out.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", tc.args, code, out.String(), stderr.String())
		}
		if got := <-received; got != tc.line+"\n" {
			t.Errorf("%v: got %q", tc.args, got)
		}
	}
}

func TestNodeAlreadyRunning(t *testing.T) {
	testListener(t, testDataDir(t))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"node"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.String() != "a node is already running\n" {
		t.Fatalf("node: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

// testDataDir points the data directory at a short temporary root: macOS
// limits a socket path to 103 bytes, and its temporary directory is deep.
func testDataDir(t *testing.T) string {
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
	for _, name := range []string{"LocalAppData", "HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, root)
	}
	dir, err := node.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func testListener(t *testing.T, dir string) *net.UnixListener {
	t.Helper()
	l, err := node.ListenLocal(dir)
	if l == nil || err != nil {
		t.Fatalf("listener = %v, error = %v", l, err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestNodeErrorIsOneLine(t *testing.T) {
	l := testListener(t, testDataDir(t))
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		bufio.NewReader(conn).ReadString('\n')
		fmt.Fprintln(conn, `{"text":"partial"}`)
		fmt.Fprintln(conn, `{"done":true,"error":"first\r\nsecond\nthird"}`)
	}()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"status"}, &stdout, &stderr); code != 1 || stdout.String() != "partial\n" || stderr.String() != "first  second third\n" {
		t.Fatalf("status: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func replyToCommand(l *net.UnixListener) <-chan string {
	received := make(chan string, 1)
	go func() {
		l.SetDeadline(time.Now().Add(2 * time.Second))
		conn, err := l.Accept()
		if err != nil {
			received <- err.Error()
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			received <- err.Error()
			return
		}
		received <- line
		fmt.Fprintln(conn, `{"done":true,"ok":true}`)
	}()
	return received
}

func TestDefaultRendezvousAddress(t *testing.T) {
	var stderr bytes.Buffer
	code := rendezvous(nil, &stderr, func(addr string) error { return fmt.Errorf("stopped %s", addr) })
	if code != 1 || stderr.String() != "stopped 127.0.0.1:8080\n" {
		t.Errorf("rendezvous returned %d, %q", code, stderr.String())
	}
}
