# Agent Note: The secretary arranges each task's closing step without platform merges

Status: implemented

## Problem

Prior to 2026-11-26, SoloDawn merged task branches itself after a passing review. Once groups moved
to direct workspace folders and platform Git operations were removed (`2026-11-26-groups-workspaces.md`),
a passing review immediately marks a task `done`, leaving branch merges, pull-request actions, or
workspace cleanup with no owner unless the secretary explicitly coordinates them.

## Decision

The coordinator decided (question 2 of the 2026-11-26 workspace model and refined in M5/M7 on
2026-11-27) that **after a task's review passes, the secretary arranges the project-appropriate
closing step in the same turn it sees the review pass**:

1. **Platform boundary (`R-90`, `R-137`):** The home marks the task `done` upon passing review and
   creates no merge or branch step of its own.
2. **Same-turn secretary action:** In the turn where the review passes, the secretary either calls
   `ask` targeting the task's assignee (in that task's thread, one task at a time) when the project
   lets an AI perform the closing step (e.g. merging/cleaning up), or calls `request_decision` when
   a human should perform or approve the step.
3. **Explicit `summary` disclosure:** The secretary's `summary` post states which closing step was
   requested (`ask` or `request_decision`), never leaving an unowned implied merge behind.

## Alternatives considered

- **Leave closing steps entirely implicit in task instructions:** led to tasks stopping after
  passing review with unmerged local branches or unnotified human sign-offs (as observed in the M5
  demo walkthrough before per-task thread closing requests were added).
- **Restore node-run `git merge` after review:** incompatible with arbitrary non-Git or shared
  local folders (`2026-11-26-groups-workspaces.md`).

## Consequences

- Each task has clear ownership after passing review without coupling the node to Git workflows.
- Because the closing step runs via model instructions (`secretary.md`) rather than a home state
  machine transition, reliability depends on prompt adherence (tracked in tests and real-CLI
  checks).

## Verification

- Established and refined across commits `b051eafef` (contracts/rules), `c035aaef6` (home marks
  task done at review), `2f4c0eb70` (`internal/host/instructions/secretary.md`), `e11dea209`
  (same-turn closing step rule + instruction pins), and `9cf60fbb8` (per-task thread closing
  requests in the demo simulation).

## Related

- [2026-11-26-groups-workspaces.md](./2026-11-26-groups-workspaces.md)
- [drivers.md](../../../../planning/contracts/drivers.md) (section 4)
- [no-turn-ends-with-promise](./2026-11-27-no-turn-ends-with-promise.md)
