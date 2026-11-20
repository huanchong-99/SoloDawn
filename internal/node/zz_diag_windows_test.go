package node

// TEMPORARY diagnostic for the socket DACL on Windows; removed before merge.

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestZZDiagSocketDACL(t *testing.T) {
	var log []string
	note := func(format string, args ...any) { log = append(log, fmt.Sprintf(format, args...)) }
	v := windows.RtlGetVersion()
	note("windows %d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	tok := windows.GetCurrentProcessToken()
	user, err := tok.GetTokenUser()
	note("user %v (%v), elevated %v", user.User.Sid, err, tok.IsElevated())
	path := filepath.Join(socketRoot(t), "sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	note("listen: %v", err)
	if l == nil {
		t.Fatal(strings.Join(log, "\n"))
	}
	name, _ := windows.UTF16PtrFromString(path)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	open := func(label string, access, flags uint32) {
		h, err := windows.CreateFile(name, access, share, nil, windows.OPEN_EXISTING, flags, 0)
		note("%s: %v", label, err)
		if err == nil {
			if strings.HasPrefix(label, "write_dac") {
				note("  SetSecurityInfo: %v", windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, ownerDACL(""), nil))
			}
			windows.CloseHandle(h)
		}
	}
	open("no access", 0, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	open("read_control", windows.READ_CONTROL, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	open("read_attributes", windows.FILE_READ_ATTRIBUTES, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	open("write_dac+backup", windows.WRITE_DAC, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS)
	open("write_dac", windows.WRITE_DAC, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	open("write_dac", windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	note("ownerOnly (fixed): %v", ownerOnly(path))
	if h, err := windows.CreateFile(name, windows.READ_CONTROL, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0); err == nil {
		d, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		note("handle readback after fixed ownerOnly: %v %v", d, err)
		windows.CloseHandle(h)
	}
	note("SetNamedSecurityInfo: %v", windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, ownerDACL(""), nil))
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	note("after: %v %v", sd, err)
	c, err := net.Dial("unix", path)
	note("dial after: %v", err)
	if c != nil {
		c.Close()
	}
	l.SetUnlinkOnClose(false)
	l.Close()
	open("write_dac after close", windows.WRITE_DAC, windows.FILE_FLAG_OPEN_REPARSE_POINT)
	t.Errorf("DIAG\n%s", strings.Join(log, "\n"))
}
