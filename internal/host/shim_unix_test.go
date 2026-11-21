//go:build !windows

package host

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
)

// TestRunShimUnixHalfClose checks RunShim's CloseWrite half-close over a real
// Unix domain socket. Windows AF_UNIX half-close does not always deliver EOF
// when CloseWrite races with a concurrent peer Read (issues.md), so this test
// builds only on non-Windows systems.
func TestRunShimUnixHalfClose(t *testing.T) {
	sock := shortSocketPath(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// serverOpening is read once after RunShim returns.
	serverOpening := make(chan string, 1)
	go serveUnixHalfCloseEcho(ln, serverOpening)

	var out, errBuf bytes.Buffer
	code := RunShim([]string{"--token", "tok<&>"}, func() (net.Conn, error) {
		return net.Dial("unix", sock)
	}, strings.NewReader("ping\n"), &out, &errBuf)
	if code != 0 || errBuf.Len() != 0 || out.String() != "echo:ping\n" {
		t.Fatalf("pipelined unix shim: code %d, out %q, err %q", code, out.String(), errBuf.String())
	}
	if gotOpen := <-serverOpening; gotOpen != "{\"mcp\":\"tok<&>\"}\n" {
		t.Fatalf("opening line = %q", gotOpen)
	}
}

func serveUnixHalfCloseEcho(ln net.Listener, serverOpening chan<- string) {
	srv, err := ln.Accept()
	if err != nil {
		return
	}
	defer srv.Close()
	r := bufio.NewReader(srv)
	first, _ := r.ReadString('\n')
	serverOpening <- first
	body, _ := io.ReadAll(r)
	srv.Write([]byte("echo:" + string(body)))
}
