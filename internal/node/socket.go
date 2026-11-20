package node

import (
	"bufio"
	"errors"
	"net"
	"os"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// errRunning means another node answers on the socket.
var errRunning = errors.New("a node is already running")

// ListenLocal creates the node's private directory and listens on its socket,
// which only the current user can open (local.md 3). A socket file left by a
// node that stopped is removed first.
func ListenLocal(dir string) (*net.UnixListener, error) {
	if err := privateDir(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "sock")
	if err := clearStale(path); err != nil {
		return nil, err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
	if err != nil {
		return nil, err
	}
	return secureSocket(l)
}

// clearStale removes the socket file when a dial to it is refused. A dial
// that connects means a node is running; any other failure, such as no file,
// leaves the bind to report.
func clearStale(path string) error {
	conn, err := net.Dial("unix", path)
	if err == nil {
		conn.Close()
		return errRunning
	}
	if errors.Is(err, connRefused) {
		return os.Remove(path)
	}
	return nil
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
