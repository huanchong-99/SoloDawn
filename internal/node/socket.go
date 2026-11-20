package node

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
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
// that connects means a node is running; any other failure leaves the bind to
// report. No file is looked for first: Windows refuses a dial to a path where
// no file is, as it refuses one to a stale socket.
func clearStale(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
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

// AcceptOpening reads the first line of a local connection. A line that is
// neither a command nor an MCP token gets one error line, and the connection
// is closed (local.md 3). The caller continues from the returned reader.
func AcceptOpening(conn io.ReadWriteCloser) (Opening, []byte, *bufio.Reader, error) {
	r := bufio.NewReader(conn)
	open, line, err := ReadOpening(r)
	if err != nil {
		json.NewEncoder(conn).Encode(localReply{Done: true, Error: err.Error()}) // the client may be gone already
		conn.Close()
	}
	return open, line, r, err
}

// ErrOwned refuses pair on a node that already has an owner (local.md 1).
var ErrOwned = errors.New("this node already has an owner; to add a browser, run solodawn pair-browser")

// PairRequest is a pair command as the node reads it.
type PairRequest struct {
	Secret     string
	Name       string
	Rendezvous []string
}

// ReadPair reads the opening line of a pair command. owner is the "owner"
// setting, empty before the first pairing; configured is the "rendezvous"
// setting. A rendezvous given with the command replaces the configured ones
// for this pairing (local.md 1, 3; storage.md 2).
func ReadPair(line []byte, owner string, configured []string) (PairRequest, error) {
	if owner != "" {
		return PairRequest{}, ErrOwned
	}
	var c struct {
		Secret     string `json:"secret"`
		Name       string `json:"name"`
		Rendezvous string `json:"rendezvous,omitempty"`
	}
	if err := proto.Decode(line, &c); err != nil {
		return PairRequest{}, err
	}
	req := PairRequest{Secret: c.Secret, Name: c.Name, Rendezvous: configured}
	if c.Rendezvous != "" {
		req.Rendezvous = []string{c.Rendezvous}
	}
	return req, nil
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
