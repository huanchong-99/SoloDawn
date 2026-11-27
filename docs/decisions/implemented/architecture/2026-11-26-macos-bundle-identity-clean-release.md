# Agent Note: macOS bundle identity (`org.solodawn.node`) and clean-tree release verification

Status: implemented

## Problem

Cross-compiling macOS binaries without an embedded `Info.plist` caused every release binary to
share Go's default `a.out` identity, so the macOS Application Firewall could not distinguish or
remember firewall permissions specifically for SoloDawn. Additionally, release builds had no hard
check refusing binaries built from a modified or dirty source tree (`vcs.modified=true`).

## Decision

Updated [the release workflow](../../../../.github/workflows/release.yml) and added
[cmd/solodawn/Info.plist](../../../../cmd/solodawn/Info.plist):

1. **macOS identity `org.solodawn.node`:** Release builds run on macOS (`macos-26`), compile
   `darwin/amd64` and `darwin/arm64` with `CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET=13.0` and
   `-linkmode=external -extldflags '-sectcreate __TEXT __info_plist ...'`, then run `codesign -s -
   -f` and verify `codesign -dv` reports `Identifier=org.solodawn.node`.
2. **Clean-tree check:** Every built binary (macOS, Linux, Windows) must pass `go version -m`
   asserting `\tbuild\tvcs.modified=false` before computing `SHA256SUMS` and uploading artifacts.

## Alternatives considered

- **Cross-compiling darwin binaries on Linux without Apple's linker:** cannot embed `__TEXT,__info_plist`
  or run `codesign`, leaving binaries identified as `a.out`.
- **Full Developer ID notarization:** out of scope for 1.0 under R-151 (`no installers, no code
  signing`); ad-hoc signing (`codesign -s -`) gives a stable bundle identifier with zero paid
  certificates while `install.md` documents clearing `com.apple.quarantine`.

## Consequences

- Release workflow builds run on macOS runners (`macos-26`) so both darwin architectures receive
  valid bundle identifiers; Linux and Windows artifacts remain cross-compiled (`CGO_ENABLED=0`) in
  the same job.
- Manual ad-hoc local builds that omit `Info.plist` linker flags still report `a.out`, whereas
  release binaries and local builds replicating `release.yml` carry `org.solodawn.node`.

## Verification

- Implemented in commit `168700524` (`Release: macOS binaries carry their own identity, org.solodawn.node; no binary from a modified tree`), updating `.github/workflows/release.yml`, `cmd/solodawn/Info.plist`, `cmd/solodawn/README.md`, `install.md`, and `issues.md`.

## Related

- [release workflow](../../../../.github/workflows/release.yml)
- [install.md](../../../../install.md)
- [product-rules.md](../../../../planning/product-rules.md) (`R-151`)
