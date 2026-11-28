package main

import (
	"io"
	"os"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

var serviceExecutable = node.Executable

func init() {
	node.Subcommands["service"] = service(node.InstallService, node.RemoveService)
}

// service reads "service install" and "service remove", which write or
// delete the start-at-login entry themselves, without a running node
// (local.md 1). install registers this executable, with the PATH of the
// shell that runs it.
func service(install func(executable, path string) error, remove func() error) node.Subcommand {
	return func(args []string, _, stderr io.Writer) int {
		if len(args) != 1 {
			return usage(stderr)
		}
		switch args[0] {
		case "install":
			executable, err := serviceExecutable()
			if err == nil {
				err = install(executable, os.Getenv("PATH"))
			}
			return result(err, stderr)
		case "remove":
			return result(remove(), stderr)
		}
		return usage(stderr)
	}
}
