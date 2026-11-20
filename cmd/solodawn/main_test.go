package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func TestRun(t *testing.T) {
	const usage = "usage: solodawn node | open | pair <secret> [--name <label>] | pair-browser | rendezvous [--listen address] | status | version\n"
	const flags = "Usage of rendezvous:\n  -listen string\n    \tHTTP signalling and static file address (default \"127.0.0.1:8080\")\n"
	for _, tc := range []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"version"}, 0, "dev\n", ""},
		{nil, 2, "", usage},
		{[]string{"rendezvous", "extra"}, 2, "", flags},
		{[]string{"rendezvous", "--unknown"}, 2, "", "flag provided but not defined: -unknown\n" + flags},
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
	root := t.TempDir()
	for _, name := range []string{"LocalAppData", "HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, root)
	}
	dir, err := node.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	l, err := node.ListenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
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
		received := make(chan string, 1)
		go func() {
			conn, err := l.Accept()
			if err != nil {
				received <- err.Error()
				return
			}
			defer conn.Close()
			line, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil {
				received <- err.Error()
				return
			}
			received <- line
			fmt.Fprintln(conn, `{"done":true,"ok":true}`)
		}()
		var out, stderr bytes.Buffer
		if code := run(tc.args, &out, &stderr); code != 0 || out.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("%v: code %d, stdout %q, stderr %q", tc.args, code, out.String(), stderr.String())
		}
		if got := <-received; got != tc.line+"\n" {
			t.Errorf("%v: got %q", tc.args, got)
		}
	}
}

func TestForegroundFailure(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"LocalAppData", "HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, root)
	}
	dir, err := node.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	_, want := net.Listen("unix", path)
	if code := run([]string{"node"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.String() != want.Error()+"\n" {
		t.Fatalf("node: exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}
