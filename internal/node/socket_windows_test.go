package node

import (
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

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
	if got, want := sd.String(), sddl(t, "D:PAI(A;;FA;;;%s)", user.User.Sid); got != want {
		t.Errorf("socket DACL = %s, want %s", got, want)
	}
}

func TestDataDirectoryDACL(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(socketRoot(t), "new", "solodawn")
	openSocket(t, fresh)
	if got, want := dirDACL(t, fresh), sddl(t, "D:PAI(A;OICI;FA;;;%s)", user.User.Sid); got != want {
		t.Errorf("new data directory DACL = %s, want %s", got, want)
	}
	existing := socketRoot(t)
	before := dirDACL(t, existing)
	openSocket(t, existing)
	if got := dirDACL(t, existing); got != before {
		t.Errorf("existing data directory DACL = %s, want it kept as %s", got, before)
	}
}

// sddl is the descriptor written in SDDL, as Windows writes it back: one
// protected DACL for the user's SID, which Windows names by its alias where it
// has one (LA for the built-in Administrator a CI runner uses). Setting a DACL
// also marks it auto-inherited (AI).
func sddl(t *testing.T, format string, sid *windows.SID) string {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf(format, sid))
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

func dirDACL(t *testing.T, dir string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}
