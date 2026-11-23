package node

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/huanchong-99/SoloDawn/internal/proto"
)

func init() {
	Subcommands["wait"] = Wait
	Subcommands["done"] = Done
}

// Wait runs "solodawn wait --member <id> [--timeout <s>]": exit 0 with the
// hand-out on stdout, 3 with nothing to hand out, 1 on an error and 2 on
// wrong arguments (contracts/step-3/external.md 3.1).
func Wait(args []string, stdout, stderr io.Writer) int {
	flags, member := extFlags("wait")
	timeout := flags.Int("timeout", -1, "")
	if !extParse(flags, args) || *timeout < -1 {
		return extUsage(stderr)
	}
	c := Command{Cmd: "wait", Member: *member}
	if *timeout >= 0 {
		c.Timeout = timeout
	}
	var out bytes.Buffer
	if err := c.Run(&out); err != nil {
		return extFailed(stderr, err)
	}
	if out.Len() == 0 {
		return 3
	}
	stdout.Write(out.Bytes())
	return 0
}

// Done runs "solodawn done --member <id> --run <n> [--failed <text>]".
func Done(args []string, stdout, stderr io.Writer) int {
	flags, member := extFlags("done")
	run := flags.Int64("run", 0, "")
	failed := flags.String("failed", "", "")
	if !extParse(flags, args) || *run <= 0 {
		return extUsage(stderr)
	}
	if err := (Command{Cmd: "done", Member: *member, RunID: *run, Failed: *failed}).Run(stdout); err != nil {
		return extFailed(stderr, err)
	}
	return 0
}

func extFlags(name string) (*flag.FlagSet, *string) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags, flags.String("member", "", "")
}

func extParse(flags *flag.FlagSet, args []string) bool {
	return flags.Parse(args) == nil && flags.NArg() == 0 && flags.Lookup("member").Value.String() != ""
}

func extUsage(stderr io.Writer) int {
	fmt.Fprintln(stderr, "usage: solodawn <command> [arguments]")
	return 2
}

func extFailed(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, strings.NewReplacer("\r", " ", "\n", " ").Replace(err.Error()))
	return 1
}

func (n *nodeRuntime) cmdWait(ctx context.Context, line []byte) ([]string, error) {
	var c Command
	if err := proto.Decode(line, &c); err != nil {
		return nil, err
	}
	var timeout *time.Duration
	if c.Timeout != nil {
		timeout = new(time.Duration(*c.Timeout) * time.Second)
	}
	return n.host.ExternalWaitLines(ctx, c.Member, timeout)
}

func (n *nodeRuntime) cmdDone(line []byte) error {
	var c Command
	if err := proto.Decode(line, &c); err != nil {
		return err
	}
	return n.host.ExternalEnd(c.Member, c.RunID, c.Failed)
}
