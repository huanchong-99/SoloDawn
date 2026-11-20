package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, node.Rendezvous))
}

func run(args []string, stdout, stderr io.Writer, serve func(string) error) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, node.Version)
		return 0
	}
	if len(args) != 0 && args[0] == "rendezvous" {
		return rendezvous(args[1:], stderr, serve)
	}
	fmt.Fprintln(stderr, "usage: solodawn version | rendezvous [--listen address]")
	return 2
}

func rendezvous(args []string, stderr io.Writer, serve func(string) error) int {
	flags := flag.NewFlagSet("rendezvous", flag.ContinueOnError)
	flags.SetOutput(stderr)
	addr := flags.String("listen", "127.0.0.1:8080", "HTTP signalling and static file address")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	fmt.Fprintln(stderr, serve(*addr))
	return 1
}
