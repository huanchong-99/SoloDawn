package node

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Subcommands is the CLI dispatch table. Each command registers its argument
// parser from init under its first command-line word.
var Subcommands = map[string]Subcommand{}

type Subcommand func(args []string, stdout, stderr io.Writer) int

// ListenLocal creates the node's private directory and listens on its socket.
func ListenLocal(dir string) (*net.UnixListener, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	addr := &net.UnixAddr{Net: "unix", Name: filepath.Join(dir, "sock")}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	return secureSocket(l)
}

func secureSocket(l *net.UnixListener) (*net.UnixListener, error) {
	if err := os.Chmod(l.Addr().String(), 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Command opens a local command connection. Pair always sends the name
// field, even when the optional CLI label was omitted.
type Command struct {
	Cmd    string  `json:"cmd"`
	Secret string  `json:"secret,omitempty"`
	Name   *string `json:"name,omitempty"`
}

// Opening selects a command handler or an MCP session. The handler reads
// its own command arguments from the complete opening line.
type Opening struct {
	Cmd string `json:"cmd,omitempty"`
	MCP string `json:"mcp,omitempty"`
}

func ReadOpening(r *bufio.Reader) (Opening, []byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Opening{}, nil, err
	}
	var open Opening
	if err := proto.Decode(line, &open); err != nil {
		return Opening{}, nil, err
	}
	if (open.Cmd == "") == (open.MCP == "") {
		return Opening{}, nil, errors.New("expected a command or an MCP token")
	}
	// The caller continues the MCP stream from r, including any buffered bytes.
	return open, line, nil
}

// Run submits a command to the user's running node.
func (c Command) Run(stdout io.Writer) error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	return c.send(dir, stdout)
}

func (c Command) send(dir string, stdout io.Writer) error {
	conn, err := net.Dial("unix", filepath.Join(dir, "sock"))
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.exchange(conn, stdout)
}

// The runtime assembly in node.go supplies the foreground entry point.
var startNode func(string, *net.UnixListener) error

// Foreground gives the node runtime its data directory and local listener.
func Foreground() error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	l, err := ListenLocal(dir)
	if err != nil {
		return err
	}
	defer l.Close()
	if startNode == nil {
		return errors.New("node runtime is unavailable")
	}
	return startNode(dir, l)
}

func (c Command) exchange(conn io.ReadWriter, stdout io.Writer) error {
	if err := json.NewEncoder(conn).Encode(c); err != nil {
		return err
	}
	return readReplies(bufio.NewReader(conn), stdout)
}

type localReply struct {
	Done  bool    `json:"done,omitempty"`
	OK    bool    `json:"ok,omitempty"`
	Error *string `json:"error,omitempty"`
	Text  *string `json:"text,omitempty"`
}

func readReplies(r *bufio.Reader, stdout io.Writer) error {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return err
		}
		var reply localReply
		if err := proto.Decode(line, &reply); err != nil {
			return err
		}
		if reply.Done {
			return reply.completion()
		}
		if err := reply.print(stdout); err != nil {
			return err
		}
	}
}

func (r localReply) print(stdout io.Writer) error {
	if r.Text == nil {
		return errors.New("expected text in the socket reply")
	}
	_, err := fmt.Fprintln(stdout, *r.Text)
	return err
}

func (r localReply) completion() error {
	if r.OK == (r.Error != nil) {
		return errors.New("expected ok or error in the final socket reply")
	}
	if r.Error != nil {
		return errors.New(*r.Error)
	}
	return nil
}
