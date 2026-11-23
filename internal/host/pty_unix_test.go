//go:build !windows

package host

import (
	"errors"
	"os"
	"testing"

	gopty "github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/unix"
)

type ptyBadMaster struct{ gopty.UnixPty }

func (ptyBadMaster) Control(f func(uintptr)) error {
	f(^uintptr(0))
	return nil
}

func TestPTYUnixMasterErrors(t *testing.T) {
	f, err := ptyNonblockFile(-1, "invalid")
	assertEqual(t, f, (*os.File)(nil))
	assertEqual(t, errors.Is(err, unix.EBADF), true)
	ptmx, err := gopty.New()
	if err != nil {
		t.Fatal(err)
	}
	h, err := ptyWrapStarted(ptyBadMaster{ptmx.(gopty.UnixPty)})
	assertEqual(t, h, nil)
	assertEqual(t, errors.Is(err, unix.EBADF), true)
	if err := ptmx.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = ptyWrapStarted(ptmx)
	assertEqual(t, h, nil)
	assertEqual(t, err.Error(), "use of closed file")
}

func TestPTYUnixResizeDescriptorErrors(t *testing.T) {
	if err := (ptyUnixHandle{}).Resize(80, 24); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("resize nil file = %v, want ErrInvalid", err)
	}
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	err = (ptyUnixHandle{File: f}).Resize(80, 24)
	assertEqual(t, err.Error(), "use of closed file")
}
