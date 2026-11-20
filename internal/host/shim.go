package host

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
)

const shimUsage = "usage: solodawn <command> [arguments]"

type shimOpening struct {
	MCP string `json:"mcp"`
}

// RunShim runs "solodawn mcp --token <t>": it parses args, connects to the
// node's local socket through dial, sends the {"mcp":"<t>"} opening line, and
// copies stdin to the socket and the socket to stdout (contracts/local.md
// sections 1 and 3). It writes a usage or error line to stderr on failure and
// returns the exit code.
func RunShim(args []string, dial func() (net.Conn, error), stdin io.Reader, stdout, stderr io.Writer) int {
	token, ok := parseShimToken(args)
	if !ok {
		fmt.Fprintln(stderr, shimUsage)
		return 2
	}
	if err := bridgeShim(token, dial, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, oneLineError(err.Error()))
		return 1
	}
	return 0
}

func parseShimToken(args []string) (string, bool) {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	token := flags.String("token", "", "")
	if flags.Parse(args) != nil {
		return "", false
	}
	if flags.NArg() != 0 {
		return "", false
	}
	return *token, *token != ""
}

func bridgeShim(token string, dial func() (net.Conn, error), stdin io.Reader, stdout io.Writer) error {
	conn, err := dial()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := writeOpeningLine(conn, token); err != nil {
		return err
	}
	var closedAtEOF atomic.Bool
	go copyShimInput(conn, stdin, &closedAtEOF)
	_, err = io.Copy(stdout, conn)
	if closedAtEOF.Load() {
		return nil
	}
	return err
}

func writeOpeningLine(w io.Writer, token string) error {
	var line bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	enc.Encode(shimOpening{MCP: token})
	_, err := w.Write(line.Bytes())
	return err
}

func copyShimInput(conn net.Conn, stdin io.Reader, closedAtEOF *atomic.Bool) {
	io.Copy(conn, stdin)
	if half, ok := conn.(interface{ CloseWrite() error }); ok {
		half.CloseWrite()
		return
	}
	closedAtEOF.Store(true)
	conn.Close()
}

func oneLineError(text string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(text)
}
