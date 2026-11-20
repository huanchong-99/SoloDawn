//go:build windows

package rendezvous

import (
	"errors"

	"golang.org/x/sys/windows"
)

// truncated reports a datagram longer than the read buffer. Windows fails
// that read with WSAEMSGSIZE after filling the buffer.
func truncated(err error) bool { return errors.Is(err, windows.WSAEMSGSIZE) }
