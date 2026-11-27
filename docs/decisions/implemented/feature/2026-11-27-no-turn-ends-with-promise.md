# Agent Note: No AI ends a turn with an unexecuted promise

Status: implemented

## Problem

An AI turn ends immediately after it finishes responding and nothing wakes the session again until
a new inbox item arrives (`drivers.md` section 4). In simulation and live tests, AIs sometimes
posted messages such as promising to start a task, check something with another AI, or arrange a
merge later—without calling `start_task`, `set_task`, `propose_brief`, `ask`, `request_decision`,
or mentioning a peer—causing the conversation or workflow to stall silently.

## Decision

The owner established the rule on 2026-11-27 (shipped as unit M7): **no AI ends a turn with an
unfulfilled promise**:

1. **Act or delegate in the same turn:** Anything an AI says it will do must be executed in that
   same turn or handed directly to something that runs it (`start_task` in a group session,
   `set_task` or `propose_brief` for the secretary, `ask` or a `post` mentioning the AI or person
   to act, `request_decision` for human choice), naming in its post which action was triggered.
2. **Reports stay factual:** Reports and messages announce only work already done or already
   started, never future work nobody will trigger.
3. **Secretary closing requests included:** On seeing a passing review, the secretary initiates the
   closing step in that same turn rather than promising to handle it afterwards.
4. **Enforced via instructions, not text filters:** Added across `group.md`, `task.md`,
   `review.md`, and `secretary.md`, plus `contracts/drivers.md` and `contracts/mcp-tools.md`; no
   outbound regex/NLP code inspects post text for promises.

## Alternatives considered

- **Runtime text classifier rejecting "I will …" messages:** fragile across languages (Chinese and
  English), high false-positive rate on legitimate explanations, and violates `R-2` (keep judgement
  out of product runtime code).
- **Waking AIs automatically after plain posts:** risks infinite self-prompting loops and wastes
  tokens when an AI legitimately finishes a turn.

## Consequences

- Prevents silent workflow stalls caused by promissory posts across all four AI session roles.
- Accepted limit (`issues.md`): because no code checks message prose, compliance relies on the
  model following its embedded system instructions.

## Verification

- Implemented in commit `e11dea209` (`Host: no AI ends a turn with a promise (M7)`), pinned by
  unit tests in `internal/host/turn_test.go` and acceptance scenarios in
  `features/host_turn.feature`, and documented in [drivers.md](../../../../planning/contracts/drivers.md),
  [mcp-tools.md](../../../../planning/contracts/mcp-tools.md), and [issues.md](../../../../issues.md).

## Related

- [drivers.md](../../../../planning/contracts/drivers.md) (section 4)
- [mcp-tools.md](../../../../planning/contracts/mcp-tools.md)
- [2026-11-27-secretary-closing-step](./2026-11-27-secretary-closing-step.md)
