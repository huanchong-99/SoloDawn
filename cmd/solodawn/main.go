package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/huanchong-99/SoloDawn/internal/node"
)

var (
	exit          = os.Exit
	runForeground = node.Foreground
)

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
	node.Subcommands["node"] = nodeCommand
	node.Subcommands["help"] = noArgs(func(stdout io.Writer) error {
		_, err := fmt.Fprint(stdout, mainHelp)
		return err
	})
	node.Subcommands["mcp"] = func(args []string, stdout, stderr io.Writer) int {
		return node.MCP(args, os.Stdin, stdout, stderr)
	}
	for _, cmd := range []string{"status", "pair-browser", "open"} {
		node.Subcommands[cmd] = noArgs(node.Command{Cmd: cmd}.Run)
	}
}

const mainHelp = `usage: solodawn <command> [arguments]

Start the local node and open http://localhost:47800 in your browser (falls back
to another free port if 47800 is in use):
  solodawn node

Commands:
  node                                       Run the node in the foreground (local UI at http://localhost:47800)
  pair <secret> [--name <label>] [--rendezvous <url>]
                                             Pair this node with a browser (starts the node if needed)
  pair-browser                               Print a pairing link for a new browser
  open                                       Open the local web client with a new browser pairing link
  status                                     Print version, fingerprint, connections, and open sessions
  service install | remove                   Register or remove start-at-login for the current user
  group export <id> <file> [--to <device>]   Export a group to a file
  group import <file>                        Import a group from a file
  rendezvous [flags]                         Run the rendezvous service ("rendezvous -h" for flags)
  version                                    Print the binary version
  mcp --token <t> | --member <id>            Run the stdio MCP shim
  wait --member <id> [--timeout <s>]         Wait for an external AI item
  done --member <id> --run <n> [--failed <text>]
                                             End an external AI run
  help                                       Show this help

Options:
  -h, --help                                 Show this help

Run "solodawn node --help" for node startup details.
`

const nodeHelp = `usage: solodawn node

Runs the SoloDawn node in the foreground, keeping its local socket open and
serving the web client and built-in /rv signalling endpoint at
http://localhost:47800 (falling back to another free port if 47800 is in use).

First run:
  1. Run "solodawn node" in a terminal and keep it running.
  2. Open http://localhost:47800 in your browser to begin onboarding.
  3. Run "solodawn pair <secret> --name <label>" in another terminal.
     Do not run "solodawn open" before pairing: it requires an owner first.

Options:
  -h, --help    Show this help
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usage(stderr)
	}
	if len(args) == 1 && isHelpFlag(args[0]) {
		return node.Subcommands["help"](nil, stdout, stderr)
	}
	cmd, ok := node.Subcommands[args[0]]
	if !ok {
		return usage(stderr)
	}
	return cmd(args[1:], stdout, stderr)
}

func nodeCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && isHelpFlag(args[0]) {
		fmt.Fprint(stdout, nodeHelp)
		return 0
	}
	if len(args) != 0 {
		return usage(stderr)
	}
	return result(runForeground(stdout), stderr)
}

func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h"
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
