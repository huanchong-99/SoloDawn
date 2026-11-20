package node

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

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

// lockHeld is LockFileEx's answer when another process holds the lock.
const lockHeld = windows.ERROR_LOCK_VIOLATION

func lockExclusive(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
}

// unlockFile releases the lock at once: Windows releases a lock left on a
// closed handle only when its resources allow.
func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}

// listenPrivate binds the socket; privateDir's inherited DACL and ownerOnly
// limit who can open it.
func listenPrivate(path string) (*net.UnixListener, error) {
	return net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
}

// ownerOnly replaces the socket's inherited DACL with a protected one that
// grants the current user, and nobody else, full access. An AF_UNIX socket
// file is a reparse point and opens only with FILE_FLAG_OPEN_REPARSE_POINT.
// SetSecurityInfo needs READ_CONTROL besides WRITE_DAC: with WRITE_DAC alone
// it answers "Access is denied" (Windows Server 2022 and 2025).
// A bound socket path holds no NUL, so its conversion cannot fail.
func ownerOnly(path string) error {
	name, _ := windows.UTF16PtrFromString(path)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	h, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, ownerDACL(""), nil)
}
