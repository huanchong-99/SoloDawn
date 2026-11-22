//go:build acceptance && !windows

package features

import (
	"os"

	"golang.org/x/sys/unix"
)

func s2WatchTerminalSize() (int, int, error) {
	size, err := unix.IoctlGetWinsize(int(os.Stdin.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}
	return int(size.Col), int(size.Row), nil
}
