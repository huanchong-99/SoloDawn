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

func run(args []string, stdout, stderr io.Writer, serve func(node.RendezvousOptions) error) int {
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
