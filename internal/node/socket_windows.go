package node

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const connRefused = windows.WSAECONNREFUSED

// ownerOnly replaces the socket's inherited DACL with a protected one that
// grants the current user, and nobody else, full access. An AF_UNIX socket
// file is a reparse point and opens only with FILE_FLAG_OPEN_REPARSE_POINT.
// The ignored errors cannot occur: the process can always read its own token,
// the descriptor is a constant form around a valid SID and has a DACL, and a
// bound socket path holds no NUL.
func ownerOnly(path string) error {
	user, _ := windows.GetCurrentProcessToken().GetTokenUser()
	sd, _ := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;FA;;;%s)", user.User.Sid))
	dacl, _, _ := sd.DACL()
	name, _ := windows.UTF16PtrFromString(path)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	h, err := windows.CreateFile(name, windows.WRITE_DAC, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
