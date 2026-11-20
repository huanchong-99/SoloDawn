package node

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const connRefused = windows.WSAECONNREFUSED

// ownerDACL is a protected DACL with one entry: full access for the current
// user, with the given inheritance flags. The ignored errors cannot occur:
// the process can always read its own token, and the descriptor is a
// constant form around a valid SID that has a DACL.
func ownerDACL(inherit string) *windows.ACL {
	user, _ := windows.GetCurrentProcessToken().GetTokenUser()
	sd, _ := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;%s;FA;;;%s)", inherit, user.User.Sid))
	dacl, _, _ := sd.DACL()
	return dacl
}

// privateDir creates dir with a protected DACL that only the current user
// is granted, inherited by every file and folder created in it. A mode does
// not limit access on Windows. An existing directory keeps its DACL: setting
// one there would rewrite the security of every file below it, the clones
// included.
func privateDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, ownerDACL("OICI"), nil)
}

// ownerOnly replaces the socket's inherited DACL with a protected one that
// grants the current user, and nobody else, full access. An AF_UNIX socket
// file is a reparse point and opens only with FILE_FLAG_OPEN_REPARSE_POINT.
// A bound socket path holds no NUL, so its conversion cannot fail.
func ownerOnly(path string) error {
	name, _ := windows.UTF16PtrFromString(path)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	h, err := windows.CreateFile(name, windows.WRITE_DAC, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, ownerDACL(""), nil)
}
