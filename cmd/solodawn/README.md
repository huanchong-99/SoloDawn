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
| `Info.plist` | The identity `org.solodawn.node` that the release build embeds in the macOS binaries and signs them with (`.github/workflows/release.yml`), so the macOS firewall tells `solodawn` from other Go programs |

`group` (`internal/node/export.go`), `wait` and `done` (`internal/node/external.go`) register
themselves from `internal/node`; this package does not name them.

## Invariants

- An empty command line and an unknown first word print `usage: solodawn <command> [arguments]` to
  stderr and exit 2; so does a wrong argument of any command registered in `main.go` except
  `rendezvous` (and `service` in `service.go`). `rendezvous` prints the flag package's own message
  and usage for a bad flag or an extra argument and exits 2. `mcp` (`internal/node`), `group`,
  `wait` and `done` print their own usage. Success exits 0 and a failed command exits 1; its error
  is printed to stderr on one line (CR and LF become spaces).
- `version`, `node`, `status`, `pair-browser` and `open` take no arguments. `status`,
  `pair-browser` and `open` talk to the running node through its local socket and fail when none
  runs; `version` and `service` need no running node. `pair` is the only command that starts a node
  when none runs (`dialOrStart`, `internal/node/cmd.go`).
- `pair <secret> [--name <label>] [--rendezvous <url>]`: the secret is the first word and does not
  start with `-`. Without `--name` the label is the computer's host name.
- `service install` registers this executable (`os.Executable`) and the `PATH` of the shell that
  runs it on Linux and macOS (Windows ignores it), because a login service starts with a smaller
  `PATH` and would not find the AI CLIs.
  `service remove` deletes the entry and does not stop a running node.
- `rendezvous` serves until it fails, prints the error and exits 1; the server always ends with an
  error. Its flags are `node.RendezvousOptions` (`internal/node`, served by
  `internal/rendezvous`).
