//go:build acceptance

package features

import (
	"errors"
	"net"
	"sync"
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

// An Accept error of the M5 bridge fails the scenario, and the next shim is
// still taken.
func TestM5AcceptErrorIsReported(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	w, broken := &m5World{}, errors.New("too many open files")
	var wg sync.WaitGroup
	wg.Add(1)
	go w.acceptMCPLoop(t.Context(), &failOnce{ln, broken}, nil, &wg)
	t.Cleanup(func() {
		ln.Close()
		wg.Wait()
	})
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	conn.Write([]byte("{}\n"))
	if n, err := conn.Read(make([]byte, 1)); n != 1 {
		t.Fatalf("no answer to a bad opening line: %v", err)
	}
	if err := w.checkHookErr(); !errors.Is(err, broken) {
		t.Fatalf("hook error %v, want %v", err, broken)
	}
}
