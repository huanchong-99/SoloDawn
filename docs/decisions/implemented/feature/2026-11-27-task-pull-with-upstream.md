# Agent Note: Task AI pulls only when the current branch has an upstream

Status: implemented

## Problem

After groups moved to arbitrary local workspace folders (`2026-11-26-groups-workspaces.md`), users
started testing SoloDawn in local-only Git repositories that have no configured remote or upstream
tracking branch. Because task instructions previously told the assignee to run `git pull
--ff-only` unconditionally, compliant CLIs hit Git errors on upstream-less branches and stopped
before starting work.

## Decision

Updated task instructions (`internal/host/instructions/task.md`), the drivers contract
(`planning/contracts/drivers.md` section 4), and instruction-pinning tests so the pull directive
is explicitly conditional: run `git pull --ff-only` **when the current branch has an upstream**
(`without one there is nothing to bring in`).

Origin: fix proposed and shipped directly by the engineering/coordinator workflow after real-world
local testing exposed the failure; aligns driver instructions with local-only folders.

## Alternatives considered

- **Tell AIs to ignore `git pull` errors:** risky because genuine divergence or a dirty tree on
  tracked repos should still halt the task and notify the thread instead of continuing on stale
  history.
- **Have the node check Git remotes automatically:** violates `R-137` (`the node runs no git`).

## Consequences

- Local-only Git projects and detached/initial branches without an upstream proceed straight to
  task work after inspecting `git status`.
- Repositories that do have an upstream tracking branch keep the strict fast-forward-only check
  before starting work.

## Verification

- Implemented in commit `c4f57e4d8` (`Host: the task AI pulls only when the branch has an
  upstream`), updating `internal/host/instructions/task.md`, [drivers.md](../../../../planning/contracts/drivers.md),
  `features/host_turn.feature`, and `internal/host/turn_test.go`.

## Related

- [drivers.md](../../../../planning/contracts/drivers.md) (section 4)
- [product-rules.md](../../../../planning/product-rules.md) (`R-90`, `R-137`)
