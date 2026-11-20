package node

import (
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

const addrInUse = windows.WSAEADDRINUSE

func TestSocketDACLIsOwnerOnly(t *testing.T) {
	dir := socketRoot(t)
	openSocket(t, dir)
	name, err := windows.UTF16PtrFromString(filepath.Join(dir, "sock"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sd.String(), fmt.Sprintf("D:P(A;;FA;;;%s)", user.User.Sid); got != want {
		t.Errorf("socket DACL = %s, want %s", got, want)
	}
}
