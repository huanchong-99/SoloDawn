package node

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCannotSendCommand(t *testing.T) {
	c := Command{Cmd: "status"}
	if err := c.send(t.TempDir(), io.Discard); !errors.Is(err, os.ErrNotExist) {
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
