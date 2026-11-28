package node

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

func init() {
	Subcommands["probe"] = probeCommand
}

func probeCommand(args []string, _ io.Writer, stderr io.Writer) int {
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(stderr, "usage: solodawn <command> [arguments]")
		return 2
	}
	return probeExit(probeDir(args[0]), stderr)
}

func probeDir(folder string) error {
	f, err := os.Open(folder)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func probeExit(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(stderr, err)
	if errors.Is(err, syscall.EPERM) {
		return 3
	}
	return 1
}
