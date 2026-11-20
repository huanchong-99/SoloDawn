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

func rendezvous(args []string, stderr io.Writer, serve func(node.RendezvousOptions) error) int {
	flags := flag.NewFlagSet("rendezvous", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var o node.RendezvousOptions
	flags.StringVar(&o.Listen, "listen", "", "signalling address (TLS :443 or local HTTP 127.0.0.1:8080)")
	flags.StringVar(&o.Domain, "domain", "", "signalling and STUN hostname")
	flags.StringVar(&o.TurnDomain, "turn-domain", "", "TURN-over-TLS hostname")
	flags.StringVar(&o.Cert, "cert", "", "PEM certificate chain for both hostnames")
	flags.StringVar(&o.Key, "key", "", "PEM certificate private key")
	flags.IntVar(&o.UDPPort, "udp-port", 3478, "STUN and TURN UDP port")
	flags.Int64Var(&o.QuotaMB, "relay-quota-mb", 50, "per-device UTC daily limit in MiB")
	flags.BoolVar(&o.NoRelay, "no-relay", false, "offer STUN but no relay")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	fmt.Fprintln(stderr, serve(o))
	return 1
}
