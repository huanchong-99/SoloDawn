package main

import (
	"io"
	"os"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func init() {
	node.Subcommands["service"] = service(node.InstallService, node.RemoveService)
}

// service reads "service install" and "service remove", which write or
// delete the start-at-login entry themselves, without a running node
// (local.md 1). install registers this executable.
func service(install func(executable string) error, remove func() error) node.Subcommand {
	return func(args []string, _, stderr io.Writer) int {
		if len(args) != 1 {
			return usage(stderr)
		}
		switch args[0] {
		case "install":
			executable, err := os.Executable()
			if err == nil {
				err = install(executable)
			}
			return result(err, stderr)
		case "remove":
			return result(remove(), stderr)
		}
		return usage(stderr)
	}
}
