//go:build windows

package host

import "os"

func signalQwenStop(*os.Process) {}
