package main

import (
	"fmt"
	"io"
	"os"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "version" {
		fmt.Fprintln(stderr, "usage: solodawn version")
		return 2
	}
	fmt.Fprintln(stdout, node.Version)
	return 0
}
