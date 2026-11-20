package node

import (
	"bufio"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// ListenLocal creates the node's private directory and listens on its socket,
// which only the current user can open (local.md 3).
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
	if err := ownerOnly(l.Addr().String()); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
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

// startNode runs the node on its data directory and local listener. The
// runtime assembly in node.go replaces it.
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
