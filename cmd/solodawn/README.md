---
description: 'The solodawn command: which subcommands the binary registers itself, how it parses their arguments and what it returns, and which subcommands live in internal/node.'
kind: reference
---

# solodawn command

## Summary

`cmd/solodawn` is the program's `main` package and holds no logic of its own. It fills the dispatch
table `node.Subcommands` (`internal/node/cmd.go`), parses the arguments of the commands it
registers, and turns the result into an exit code. The command line is specified in
[local](../../planning/contracts/local.md), section 1.

## Files

| File | Holds |
| --- | --- |
| `main.go` | `main`, `run` (looks the first word up in `node.Subcommands`), and the registration of `rendezvous`, `pair`, `version`, `node`, `mcp`, `status`, `pair-browser` and `open` |
| `service.go` | `service install` and `service remove` |

`group` (`internal/node/export.go`), `wait` and `done` (`internal/node/external.go`) register
themselves from `internal/node`; this package does not name them.

## Invariants

- An empty command line, an unknown first word and any wrong argument print
  `usage: solodawn <command> [arguments]` to stderr and exit 2. Success exits 0 and a failed command
  exits 1; its error is printed to stderr on one line (CR and LF become spaces).
- `version`, `node`, `status`, `pair-browser` and `open` take no arguments. `status`,
  `pair-browser` and `open` talk to the running node through its local socket; `version` and
  `service` need no running node.
- `pair <secret> [--name <label>] [--rendezvous <url>]`: the secret is the first word and does not
  start with `-`. Without `--name` the label is the computer's host name.
- `service install` registers this executable (`os.Executable`) and the `PATH` of the shell that
  runs it, because a login service starts with a smaller `PATH` and would not find the AI CLIs.
  `service remove` deletes the entry and does not stop a running node.
- `rendezvous` serves until it fails, prints the error and exits 1; with no error to report it
  never returns. Its flags are `node.RendezvousOptions` (`internal/node`, served by
  `internal/rendezvous`).
