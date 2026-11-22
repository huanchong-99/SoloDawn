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
	"time"

	"github.com/huanchong-99/SoloDawn/internal/host"
	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Subcommands is the CLI dispatch table. Each command registers its argument
// parser from init under its first command-line word.
var Subcommands = map[string]Subcommand{}

// Subcommand parses and runs one CLI subcommand, returning its exit code.
type Subcommand func(args []string, stdout, stderr io.Writer) int

var (
	executablePath = os.Executable
	spawnNode      = startBackgroundNode
	pairStartWait  = 5 * time.Second
)

// MCP runs the stdio MCP shim ("solodawn mcp --token <t>") against the running
// node's local socket (local.md 1, 3).
func MCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return host.RunShim(args, dialLocal, stdin, stdout, stderr)
}

func dialLocal() (net.Conn, error) {
	dir, err := DataDir()
	if err != nil {
		return nil, err
	}
	return net.Dial("unix", filepath.Join(dir, "sock"))
}

func startBackgroundNode() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	return startDetached(exe, "node")
}

// Command opens a local command connection. Pair always sends the name
// field, and the rendezvous field only when the option was given.
type Command struct {
	Cmd        string  `json:"cmd"`
	Secret     string  `json:"secret,omitempty"`
	Name       *string `json:"name,omitempty"`
	Rendezvous string  `json:"rendezvous,omitempty"`
}

// Run submits a command to the user's running node and prints each text line
// of the answer (local.md 3).
func (c Command) Run(stdout io.Writer) error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	return c.send(dir, stdout)
}

func (c Command) send(dir string, stdout io.Writer) error {
	conn, err := c.dialOrStart(dir)
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.exchange(conn, stdout)
}

func (c Command) dialOrStart(dir string) (net.Conn, error) {
	sock := filepath.Join(dir, "sock")
	conn, err := net.Dial("unix", sock)
	if err == nil || c.Cmd != "pair" {
		return conn, err
	}
	if err := spawnNode(); err != nil {
		return nil, err
	}
	return waitSocket(sock, pairStartWait)
}

func waitSocket(sock string, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	for {
		conn, err := net.Dial("unix", sock)
		if err == nil || !time.Now().Before(deadline) {
			return conn, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (c Command) exchange(conn io.ReadWriter, stdout io.Writer) error {
	if err := json.NewEncoder(conn).Encode(c); err != nil {
		return err
	}
	return readReplies(bufio.NewReader(conn), stdout)
}

// localReply is one answer line: {"text":"…"} before the last line, then
// {"done":true,"ok":true} or {"done":true,"error":"…"}.
type localReply struct {
	Done  bool   `json:"done,omitempty"`
	OK    bool   `json:"ok,omitempty"`
	Error string `json:"error,omitempty"`
	Text  string `json:"text,omitempty"`
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
			return reply.failure()
		}
		if _, err := fmt.Fprintln(stdout, reply.Text); err != nil {
			return err
		}
	}
}

// errNoOK answers a last line with neither "ok":true nor an error.
var errNoOK = errors.New("the node ended its answer without ok")

func (r localReply) failure() error {
	if r.Error != "" {
		return errors.New(r.Error)
	}
	if !r.OK {
		return errNoOK
	}
	return nil
}

func writeLocalReplies(w io.Writer, lines []string, err error) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, text := range lines {
		enc.Encode(localReply{Text: text})
	}
	if err != nil {
		enc.Encode(localReply{Done: true, Error: err.Error()})
		return
	}
	enc.Encode(localReply{Done: true, OK: true})
}
