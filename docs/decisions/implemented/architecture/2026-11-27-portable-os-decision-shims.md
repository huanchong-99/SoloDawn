# Agent Note: OS-specific node and rendezvous functions are thin shims over portable logic

Status: implemented

## Problem

Local verification on macOS (`gate.py` CRAP and coverage checks) only compiles files matching the
host `GOOS`. Platform-specific files such as `internal/node/service_windows.go`, `service_linux.go`,
`socket_windows.go`, and `internal/rendezvous/truncated_windows.go` contained non-trivial decision
branches whose test coverage on macOS reported `0%`, failing complexity/coverage thresholds even
though the behavior itself was platform-independent logic (path checks, file permissions/cleanup,
and UDP buffer truncation detection).

## Decision

Moved all branching decisions out of OS-tagged files into portable, build-on-every-OS helpers
(`internal/node/service.go`, `internal/node/socket.go`, `internal/rendezvous/truncated.go`),
leaving OS files (`_darwin`, `_linux`, `_windows`) as minimal thin syscalls/process shims:

- `InstallService` / `RemoveService` delegate branching to `installServiceFile` /
  `removeServiceFile` (`internal/node/service.go`).
- `privateDir` delegates directory creation/branching to `createDir` (`internal/node/socket.go`).
- `readDatagram` delegates buffer sizing and truncation branching to `readSkipping`
  (`internal/rendezvous/truncated.go`).
- Updated `.github/workflows/changes.yml` filter to watch `internal/node/service*` so changes to
  portable `service.go` still trigger the service smoke job.

## Alternatives considered

- **Excluding OS-tagged files from CRAP/coverage checks:** weakens quality enforcement by hiding
  complex branches inside Windows/Linux files instead of testing them on every developer machine
  and CI host.

## Consequences

- Decision branches for service file installation/removal, private directory creation, and UDP
  truncation run under `-race -cover` on macOS, Linux, and Windows alike with 100% statement
  coverage (`service_test.go`, `socket_test.go`, `truncated_test.go`).
- OS-specific syscalls (`SetNamedSecurityInfo`, `CreateFile`, `systemctl`, `powershell`) remain
  tiny single-purpose wrappers verified by `GOOS=... go vet` and hosted OS test runners.

## Verification

- Implemented in commit `0ad7f9397` (`Node, rendezvous: OS-specific functions are shims; their decisions are portable and tested everywhere`), verified by local unit/coverage suites (`internal/node`, `internal/rendezvous`) and `.github/workflows/service-smoke.yml`.

## Related

- [internal/node/README.md](../../../../internal/node/README.md)
- [internal/rendezvous/README.md](../../../../internal/rendezvous/README.md)
