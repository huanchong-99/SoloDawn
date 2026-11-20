package node

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// Subcommands is the CLI dispatch table. Each command registers its argument
// parser from init under its first command-line word.
var Subcommands = map[string]Subcommand{}

type Subcommand func(args []string, stdout, stderr io.Writer) int

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
	conn, err := net.Dial("unix", filepath.Join(dir, "sock"))
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.exchange(conn, stdout)
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

func (r localReply) failure() error {
	if r.Error == "" {
		return nil
	}
	return errors.New(r.Error)
}
