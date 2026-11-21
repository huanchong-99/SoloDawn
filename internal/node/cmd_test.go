package node

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCannotSendCommand(t *testing.T) {
	c := Command{Cmd: "status"}
	// No socket: the dial fails (not found; Windows says refused).
	var dial *net.OpError
	if err := c.send(t.TempDir(), io.Discard); !errors.As(err, &dial) || dial.Op != "dial" {
		t.Fatalf("send to a missing socket = %v", err)
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

func TestReplies(t *testing.T) {
	for _, tc := range []struct{ lines, stdout, wantErr string }{
		{"{\"done\":true,\"ok\":true}\n", "", ""},
		{"{\"done\":true}\n", "", "the node ended its answer without ok"},
		{"{\"text\":\"one\"}\n{\"done\":true,\"ok\":false}\n", "one\n", "the node ended its answer without ok"},
		{"{\"done\":true,\"error\":\"denied\"}\n", "", "denied"},
		{"{\"text\":\"one\"}\n{\"text\":\"\"}\n{\"done\":true,\"ok\":true}\n", "one\n\n", ""},
		{"{\"text\":\"partial\"}\n{\"done\":true,\"error\":\"denied\"}\n", "partial\n", "denied"},
		{"{\"done\":null}\n", "", "proto: null where the contract has no null"},
		{"bad\n", "", "invalid character 'b' looking for beginning of value"},
		{"{\"done\":true,\"ok\":true}", "", "EOF"},
	} {
		var out bytes.Buffer
		err := readReplies(bufio.NewReader(strings.NewReader(tc.lines)), &out)
		if errText(err) != tc.wantErr || out.String() != tc.stdout {
			t.Errorf("%q: stdout %q, error %v; want %q, %q", tc.lines, out.String(), err, tc.stdout, tc.wantErr)
		}
	}
}

func TestOutputWriteError(t *testing.T) {
	if err := readReplies(bufio.NewReader(strings.NewReader("{\"text\":\"output\"}\n")), failingWriter{}); err != io.ErrClosedPipe {
		t.Fatalf("write error = %v", err)
	}
}

func TestCommandRoundTrip(t *testing.T) {
	l := openSocket(t, nodeTestDir(t))
	received := make(chan string, 1)
	go func() {
		l.SetDeadline(time.Now().Add(2 * time.Second))
		conn, err := l.Accept()
		if err != nil {
			received <- err.Error()
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		received <- line
		io.WriteString(conn, "{\"text\":\"version dev\"}\n{\"text\":\"3F9A 0C21 77BE\"}\n{\"done\":true,\"ok\":true}\n")
	}()
	var stdout bytes.Buffer
	if err := (Command{Cmd: "status"}).Run(&stdout); err != nil || stdout.String() != "version dev\n3F9A 0C21 77BE\n" {
		t.Fatalf("status: %q, %v", stdout.String(), err)
	}
	if got := <-received; got != "{\"cmd\":\"status\"}\n" {
		t.Fatalf("request = %q", got)
	}
}

func TestCommandWithoutDataDir(t *testing.T) {
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "LocalAppData", "USERPROFILE"} {
		t.Setenv(name, "")
	}
	_, want := DataDir()
	if err := (Command{Cmd: "status"}).Run(io.Discard); errText(err) != errText(want) || want == nil {
		t.Fatalf("status returned %v, want %v", err, want)
	}
}

func TestMCPShim(t *testing.T) {
	checkMCPShimErrors(t)
	checkMCPShimRoundTrip(t)
}

func checkMCPShimErrors(t *testing.T) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := MCP(nil, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("MCP(nil) = %d, want 2", code)
	}
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "LocalAppData", "USERPROFILE"} {
		t.Setenv(name, "")
	}
	assertMCPExitOne(t, "without DataDir")
	nodeTestDir(t)
	assertMCPExitOne(t, "without socket")
}

func assertMCPExitOne(t *testing.T, label string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := MCP([]string{"--token", "tok"}, strings.NewReader(""), &stdout, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("MCP %s = %d, stderr %q", label, code, stderr.String())
	}
}

func checkMCPShimRoundTrip(t *testing.T) {
	t.Helper()
	l := openSocket(t, nodeTestDir(t))
	opened := make(chan string, 1) // single reader: checkMCPShimRoundTrip
	go serveMCPTestEcho(l, opened)
	var stdout, stderr bytes.Buffer
	if code := MCP([]string{"--token", "tok-1"}, strings.NewReader("ping\n"), &stdout, &stderr); code != 0 || stdout.String() != "echo:ping\n" {
		t.Fatalf("MCP round-trip = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if got := <-opened; got != "{\"mcp\":\"tok-1\"}\n" {
		t.Fatalf("opening = %q", got)
	}
}

func serveMCPTestEcho(l *Local, opened chan<- string) {
	conn, err := l.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	line, _ := r.ReadString('\n')
	opened <- line
	req, _ := r.ReadString('\n')
	io.WriteString(conn, "echo:"+req)
}

func TestPairAutoStartsNode(t *testing.T) {
	savedSpawn, savedWait, savedExe := spawnNode, pairStartWait, executablePath
	t.Cleanup(func() {
		spawnNode = savedSpawn
		pairStartWait = savedWait
		executablePath = savedExe
	})
	dir := nodeTestDir(t)
	name := "laptop"
	pairCmd := Command{Cmd: "pair", Secret: "s", Name: &name}

	checkPairSpawnFailures(t, dir, pairCmd)
	checkPairAutoStartSuccess(t, dir, pairCmd)
	checkStartBackgroundNode(t, dir)
}

func checkPairSpawnFailures(t *testing.T, dir string, pairCmd Command) {
	t.Helper()
	spawns := 0
	spawnNode = func() error { spawns++; return nil }
	if err := (Command{Cmd: "status"}).send(dir, io.Discard); err == nil || spawns != 0 {
		t.Fatalf("status missing socket: err=%v spawns=%d", err, spawns)
	}
	spawnNode = func() error { spawns++; return io.ErrUnexpectedEOF }
	if err := pairCmd.send(dir, io.Discard); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("pair spawn error = %v", err)
	}
	pairStartWait = 0
	spawnNode = func() error { spawns++; return nil }
	var dialErr *net.OpError
	if err := pairCmd.send(dir, io.Discard); !errors.As(err, &dialErr) {
		t.Fatalf("pair wait timeout = %v", err)
	}
}

func checkPairAutoStartSuccess(t *testing.T, dir string, pairCmd Command) {
	t.Helper()
	pairStartWait = 2 * time.Second
	var bg *Local
	t.Cleanup(func() {
		if bg != nil {
			bg.Close()
		}
	})
	spawns := 0
	spawnNode = func() error {
		spawns++
		go func() {
			time.Sleep(30 * time.Millisecond)
			var err error
			if bg, err = ListenLocal(dir); err == nil {
				servePairedOnce(bg)
			}
		}()
		return nil
	}
	assertPairOutput(t, dir, pairCmd, &spawns, 1)
	go servePairedOnce(bg)
	assertPairOutput(t, dir, pairCmd, &spawns, 1)
}

func servePairedOnce(l *Local) {
	conn, err := l.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	bufio.NewReader(conn).ReadString('\n')
	writeLocalReplies(conn, []string{"paired"}, nil)
}

func assertPairOutput(t *testing.T, dir string, pairCmd Command, spawns *int, wantSpawns int) {
	t.Helper()
	var out bytes.Buffer
	if err := pairCmd.send(dir, &out); err != nil || out.String() != "paired\n" || *spawns != wantSpawns {
		t.Fatalf("pair: out=%q err=%v spawns=%d", out.String(), err, *spawns)
	}
}

func checkStartBackgroundNode(t *testing.T, dir string) {
	t.Helper()
	executablePath = func() (string, error) { return "", io.ErrNoProgress }
	if err := startBackgroundNode(); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("startBackgroundNode exe error = %v", err)
	}
	executablePath = func() (string, error) { return filepath.Join(dir, "missing-exe"), nil }
	if err := startBackgroundNode(); err == nil {
		t.Fatal("expected startBackgroundNode error for missing executable")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := startDetached(self, "-test.run=^$"); err != nil {
		t.Fatalf("startDetached(self) = %v", err)
	}
}
