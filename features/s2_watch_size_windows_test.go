//go:build acceptance && windows

package features

import (
	"os"

	"golang.org/x/sys/windows"
)

func s2WatchTerminalSize() (int, int, error) {
	var info windows.ConsoleScreenBufferInfo
	err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info)
	return int(info.Size.X), int(info.Size.Y), err
}
