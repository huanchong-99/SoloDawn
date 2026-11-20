package node

// TEMPORARY diagnostic for the stale-socket test on Windows; removed before merge.

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestZZDiagStaleSocket(t *testing.T) {
	var log []string
	note := func(format string, args ...any) { log = append(log, fmt.Sprintf(format, args...)) }
	sd := func(p string) string {
		d, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			return "sd error: " + err.Error()
		}
		return d.String()
	}
	dir := socketRoot(t)
	path := filepath.Join(dir, "sock")
	note("dir %s: %s", dir, sd(dir))
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	note("listen stale: %v", err)
	note("stale sock sd: %s", sd(path))
	note("ownerOnly on live stale: %v", ownerOnly(path))
	stale.SetUnlinkOnClose(false)
	note("close stale: %v", stale.Close())
	_, err = os.Lstat(path)
	note("lstat after close: %v", err)
	c, err := net.Dial("unix", path)
	note("dial stale: %v, %v", c != nil, err)
	note("remove: %v", os.Remove(path))
	_, err = os.Lstat(path)
	note("lstat after remove: %v", err)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	note("listen again: %v", err)
	if l != nil {
		note("new sock sd: %s", sd(path))
		note("ownerOnly on new: %v", ownerOnly(path))
		l.Close()
	}
	fresh := filepath.Join(socketRoot(t), "sock")
	l2, err := net.ListenUnix("unix", &net.UnixAddr{Name: fresh, Net: "unix"})
	note("listen fresh: %v", err)
	if l2 != nil {
		note("fresh sd: %s", sd(fresh))
		note("ownerOnly on fresh: %v", ownerOnly(fresh))
		l2.Close()
	}
	other := filepath.Join(socketRoot(t), "sub", "solodawn")
	note("privateDir: %v", privateDir(other))
	l3, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(other, "sock"), Net: "unix"})
	note("listen in private dir: %v", err)
	if l3 != nil {
		note("ownerOnly in private dir: %v", ownerOnly(filepath.Join(other, "sock")))
		l3.Close()
	}
	t.Errorf("DIAG\n%s", strings.Join(log, "\n"))
}
