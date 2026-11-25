package node

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/huanchong-99/SoloDawn/internal/identity"
	"github.com/huanchong-99/SoloDawn/internal/proto"
	"github.com/huanchong-99/SoloDawn/internal/store"
)

func init() {
	Subcommands["group"] = groupCommand
}

func groupCommand(args []string, stdout, stderr io.Writer) int {
	cmd, err := groupArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := cmd.Run(stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func groupArgs(args []string) (Command, error) {
	switch {
	case len(args) == 2 && args[0] == "import":
		file, err := groupFile(args[1])
		return Command{Cmd: "group.import", File: file}, err
	case len(args) >= 3 && args[0] == "export":
		return exportArgs(args[1:])
	}
	return Command{}, groupUsage
}

var groupUsage = errors.New("usage: solodawn group export <group id> <file> [--to <device id>] | solodawn group import <file>")

func exportArgs(args []string) (Command, error) {
	cmd := Command{Cmd: "group.export", Group: proto.ID(args[0])}
	switch len(args) {
	case 2:
	case 4:
		if args[2] != "--to" || !identity.IsDevice(args[3]) {
			return Command{}, groupUsage
		}
		cmd.To = args[3]
	default:
		return Command{}, groupUsage
	}
	file, err := groupFile(args[1])
	cmd.File = file
	return cmd, err
}

func groupFile(arg string) (string, error) {
	if arg == "" {
		return "", groupUsage
	}
	return filepath.Abs(arg)
}

func (n *nodeRuntime) cmdGroup(line []byte) error {
	var cmd Command
	if proto.Decode(line, &cmd) != nil || !filepath.IsAbs(cmd.File) {
		return errors.New("invalid group transfer arguments")
	}
	if cmd.Cmd == "group.import" {
		return n.importGroup(cmd.File)
	}
	return n.exportGroup(cmd)
}

func (n *nodeRuntime) exportGroup(cmd Command) error {
	if cmd.Group == "" || cmd.To != "" && !identity.IsDevice(cmd.To) {
		return errors.New("invalid group or destination id")
	}
	return n.loop.ExportGroup(string(cmd.Group), cmd.File, cmd.To)
}

func (n *nodeRuntime) importGroup(file string) error {
	group, err := store.ExportedGroup(file)
	if err != nil {
		return fmt.Errorf("read group export: %w", err)
	}
	loaded := &Logs{log: map[string]*identity.Log{}}
	return n.loop.ImportGroup(group, file, loaded.dvlLoadEntries, func() { n.keepImported(loaded.log) })
}

func (n *nodeRuntime) keepImported(logs map[string]*identity.Log) {
	for _, log := range logs {
		if err := n.logs.dvlKeep(log, nil); err != nil {
			n.sessions.cfg.Report(err)
		}
	}
}
