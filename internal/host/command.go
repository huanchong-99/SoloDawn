package host

import (
	"os/exec"
	"strconv"
	"time"
)

func taskkill(pid int) error {
	return exec.Command("taskkill.exe", "/f", "/t", "/pid", strconv.Itoa(pid)).Run()
}

// commandWaitDelay is how long the output of a stopped command may stay
// open, held by a child that left its session: 10 s. Tests shorten it.
var commandWaitDelay = time.Duration(10e9)
