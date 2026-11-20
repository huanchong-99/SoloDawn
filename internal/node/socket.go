package node

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

// errRunning means another node holds the data directory's lock.
var errRunning = errors.New("a node is already running")

// Local is the node's local socket and the lock that makes it the only node
// of its data directory. Both are held until Close.
type Local struct {
	*net.UnixListener
	lock *os.File
}

// Close closes the listener, which removes the socket, then releases the lock.
func (l *Local) Close() error {
	return errors.Join(l.UnixListener.Close(), l.lock.Close())
}

// ListenLocal creates the node's private directory, takes its lock and
// listens on its socket, which only the current user can open (local.md 3).
// Without the lock another node is running. With it, a socket file that is
// there was left by a node that stopped, and is removed.
func ListenLocal(dir string) (*Local, error) {
	if err := privateDir(dir); err != nil {
		return nil, err
	}
	lock, err := lockData(dir)
	if err != nil {
		return nil, err
	}
	l, err := bindLocal(filepath.Join(dir, "sock"))
	if err != nil {
		lock.Close()
		return nil, err
	}
	return &Local{l, lock}, nil
}

// lockData takes an exclusive lock on the lock file of the data directory;
// the system releases it when the process ends.
func lockData(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if lockFile(f) != nil {
		f.Close()
		return nil, errRunning
	}
	return f, nil
}

func bindLocal(path string) (*net.UnixListener, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	l, err := listenPrivate(path)
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

// maxOpening limits the first line of a local connection.
const maxOpening = 64 << 10

// openingTimeout limits the wait for the first line, and for the error line
// that answers a bad one.
var openingTimeout = 10 * time.Second

// ReadOpening reads the first line, which must fit in r's buffer.
func ReadOpening(r *bufio.Reader) (Opening, []byte, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		return Opening{}, nil, err
	}
	line = bytes.Clone(line)
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

// AcceptOpening reads the first line of a local connection, at most
// maxOpening bytes within openingTimeout. A line that is neither a command
// nor an MCP token gets one error line, and the connection is closed
// (local.md 3). The caller continues from the returned reader, without a
// deadline.
func AcceptOpening(conn net.Conn) (Opening, []byte, *bufio.Reader, error) {
	conn.SetReadDeadline(time.Now().Add(openingTimeout))
	r := bufio.NewReaderSize(conn, maxOpening)
	open, line, err := ReadOpening(r)
	if err != nil {
		conn.SetWriteDeadline(time.Now().Add(openingTimeout))
		json.NewEncoder(conn).Encode(localReply{Done: true, Error: err.Error()}) // the client may be gone already
		conn.Close()
		return open, line, r, err
	}
	conn.SetReadDeadline(time.Time{})
	return open, line, r, nil
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
var startNode = func(string, *Local) error {
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
