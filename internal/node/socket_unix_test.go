//go:build !windows

package node

import "syscall"

const addrInUse = syscall.EADDRINUSE
