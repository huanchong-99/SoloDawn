package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
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
	node.Subcommands["mcp"] = func(args []string, stdout, stderr io.Writer) int {
		return node.MCP(args, os.Stdin, stdout, stderr)
	}
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

// pair reads "pair <secret> [--name <label>] [--rendezvous <url>]". The
// label defaults to the computer's host name (local.md 1).
func pair(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		return usage(stderr)
	}
	host, _ := os.Hostname() // the supported systems always report one
	flags := flag.NewFlagSet("pair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", host, "")
	rendezvous := flags.String("rendezvous", "", "")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return usage(stderr)
	}
	return result(node.Command{Cmd: "pair", Secret: args[0], Name: name, Rendezvous: *rendezvous}.Run(stdout), stderr)
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
	flags.StringVar(&o.ACMEDirectory, "acme-directory", "", "ACME server directory URL (empty: Let's Encrypt)")
	flags.StringVar(&o.ACMECA, "acme-ca", "", "PEM roots for the ACME server's HTTPS only")
	flags.IntVar(&o.UDPPort, "udp-port", 3478, "STUN and TURN UDP port")
	flags.Var(&o.RelayPorts, "relay-ports", "relay allocation UDP ports, `first-last` (default 20000-29999)")
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
