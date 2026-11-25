package node

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// failOnce is a listener whose first Accept fails.
type failOnce struct {
	net.Listener
	err error
}

func (l *failOnce) Accept() (net.Conn, error) {
	if err := l.err; err != nil {
		l.err = nil
		return nil, err
	}
	return l.Listener.Accept()
}

func acceptFailed(t *testing.T, dir, name string) bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "logs", "node.log"))
	mustNoErr(t, err)
	return strings.Contains(string(raw), "ERROR accept.fail node=") && strings.Contains(string(raw), " name="+name+" err=")
}

// The local socket still takes connections after an Accept error.
func TestLocalSocketOutlivesAnAcceptError(t *testing.T) {
	dir := socketRoot(t)
	l := openSocket(t, dir)
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	mustNoErr(t, l.SetDeadline(time.Now()))
	served := make(chan struct{}) // single reader: this test
	go func() { n.serve(l); close(served) }()
	waitCond(t, func() bool { return acceptFailed(t, dir, "local") })
	mustNoErr(t, l.SetDeadline(time.Time{}))
	var out bytes.Buffer
	mustNoErr(t, (Command{Cmd: "status"}).send(dir, &out))
	if !strings.Contains(out.String(), n.key.Fingerprint()) {
		t.Fatalf("status %q", out.String())
	}
	n.Close()
	<-served
}

// The node's own web server still takes connections after an Accept error.
func TestLocalHTTPOutlivesAnAcceptError(t *testing.T) {
	saved := tcpListen
	t.Cleanup(func() { tcpListen = saved })
	tcpListen = func(network, addr string) (net.Listener, error) {
		ln, err := saved(network, addr)
		return &failOnce{ln, errors.New("connection reset by the system")}, err
	}
	dir := socketRoot(t)
	n, err := openNode(t.Context(), dir)
	mustNoErr(t, err)
	t.Cleanup(n.Close)
	mustEqual(t, acceptFailed(t, dir, "http"), true)
}
