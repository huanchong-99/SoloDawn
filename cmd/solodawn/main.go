package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func init() {
	node.Subcommands["rendezvous"] = func(args []string, _ io.Writer, stderr io.Writer) int {
		return rendezvous(args, stderr, node.Rendezvous)
	}
	node.Subcommands["pair"] = pair
	node.Subcommands["version"] = noArgs(func(stdout io.Writer) error {
		_, err := fmt.Fprintln(stdout, node.Version)
		return err
	})
	node.Subcommands["node"] = noArgs(func(io.Writer) error {
		return node.Foreground()
	})
	for _, cmd := range []string{"status", "pair-browser", "open"} {
		node.Subcommands[cmd] = noArgs(node.Command{Cmd: cmd}.Run)
	}
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usage(stderr)
	}
	cmd, ok := node.Subcommands[args[0]]
	if !ok {
		return usage(stderr)
	}
	return cmd(args[1:], stdout, stderr)
}

func usage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: solodawn <command> [arguments]")
	return 2
}

func noArgs(invoke func(io.Writer) error) node.Subcommand {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			return usage(stderr)
		}
		return result(invoke(stdout), stderr)
	}
}

func pair(args []string, stdout, stderr io.Writer) int {
	var name string
	switch len(args) {
	case 3:
		if args[1] != "--name" {
			return usage(stderr)
		}
		name = args[2]
	case 1:
	default:
		return usage(stderr)
	}
	if args[0] == "" || strings.HasPrefix(args[0], "-") {
		return usage(stderr)
	}
	return result(node.Command{Cmd: "pair", Secret: args[0], Name: &name}.Run(stdout), stderr)
}

func result(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, strings.NewReplacer("\r", " ", "\n", " ").Replace(err.Error()))
		return 1
	}
	return 0
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
