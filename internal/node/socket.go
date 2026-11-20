package node

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Subcommands is the CLI dispatch table. Each command registers its argument
// parser from init under its first command-line word.
var Subcommands = map[string]Subcommand{}

type Subcommand struct {
	Usage string
	Run   func(args []string, stdout, stderr io.Writer) int
}

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

// ReadOpening consumes just the first JSON line. An MCP caller continues
// reading its stream from the same buffered reader.
func ReadOpening(r *bufio.Reader) (Command, string, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Command{}, "", err
	}
	var opener struct {
		Cmd *string `json:"cmd,omitempty"`
		MCP *string `json:"mcp,omitempty"`
	}
	if err := proto.Decode(line, &opener); err != nil {
		return Command{}, "", err
	}
	if (opener.Cmd == nil) == (opener.MCP == nil) {
		return Command{}, "", errors.New("expected one of cmd or mcp")
	}
	if opener.MCP != nil {
		if *opener.MCP == "" {
			return Command{}, "", errors.New("empty mcp token")
		}
		return Command{}, *opener.MCP, nil
	}
	return readCommand(*opener.Cmd, line)
}

func readCommand(cmd string, line []byte) (Command, string, error) {
	c := Command{Cmd: cmd}
	switch cmd {
	case "pair":
		var fields struct {
			Secret string `json:"secret"`
			Name   string `json:"name"`
		}
		if err := proto.Decode(line, &fields); err != nil {
			return Command{}, "", err
		}
		c.Secret, c.Name = fields.Secret, &fields.Name
	case "status", "open", "pair-browser":
	default:
		return Command{}, "", errors.New("unknown local command")
	}
	return c, "", nil
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
var startNode = func(string, *net.UnixListener) error {
	return errors.New("node runtime is unavailable")
}

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
	return startNode(dir, l)
}

func (c Command) exchange(conn io.ReadWriter, stdout io.Writer) error {
	if err := json.NewEncoder(conn).Encode(c); err != nil {
		return err
	}
	return readReplies(bufio.NewReader(conn), stdout)
}

func readReplies(r *bufio.Reader, stdout io.Writer) error {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return err
		}
		var reply any
		if err := proto.Decode(line, &reply); err != nil {
			return err
		}
		object, _ := reply.(map[string]any)
		if _, terminal := object["done"]; terminal {
			return completion(line)
		}
		if _, err := stdout.Write(line); err != nil {
			return err
		}
	}
}

func completion(line []byte) error {
	var result struct {
		Done  bool    `json:"done"`
		OK    bool    `json:"ok,omitempty"`
		Error *string `json:"error,omitempty"`
	}
	if err := proto.Decode(line, &result); err != nil {
		return err
	}
	if !result.Done {
		return errors.New("expected done to be true in the final socket reply")
	}
	if result.Error != nil {
		return errors.New(*result.Error)
	}
	if !result.OK {
		return errors.New("expected ok or error in the final socket reply")
	}
	return nil
}
