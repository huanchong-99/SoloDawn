# Agent Note: The command seam of the home

Status: implemented

## Problem

Seventeen units add commands, reads and timers to `internal/home` at the same time, and two units of
`internal/node` connect it to clients ([parallel plan](../../../../planning/steps/parallel-plan.md),
section 1; [units](../../../../planning/steps/parallel-plan-units.md), section 2). They need one
shape for a command before any of them exists: what the function of a command is given, what it
returns, which checks are made for it, and who writes what. The plan fixed three things: a table
that each file adds to from its own `init`, pure functions from (state, command) to (rows, items,
answer, error), and interfaces that tests can fake. The contracts left several points open, listed
under Decision.

## Decision

Unit HM-0 built the seam in `internal/home/home.go` and `internal/home/check.go`; its current
contract is the [module README](../../../../internal/home/README.md). The choices were made by the
session that built HM-0, under the coordinator's instruction for that unit to decide where a
contract is silent and to write the rule into the contract. The owner did not decide them in
person.

1. **The table is a value.** `Table` is a map from key to function with the methods `Add` and
   `Apply`; `Commands` is the table of a node. A test or an acceptance step makes a table of its
   own.
2. **`Apply` runs inside the command's transaction and writes nothing but the receipt.** It takes
   the receipts as an interface that `*store.Tx` satisfies. The caller writes rows and items, and
   calls `State.Commit` after the commit, so memory never holds a row that the store refused.
3. **`Apply` numbers the rows.** A function returns rows without `seq`; `Apply` gives them
   `State.Seq + 1` onwards, and `pos` where a row has one that is 0.
4. **A refusal is a receipt.** Every command that arrived on its signer's connection and passed
   the envelope checks gets a receipt with its outcome, success or error code. A repeat of a
   refused command is answered with the same code and a fixed sentence. The connection check and
   the envelope checks leave no receipt. Written into
   [commands](../../../../planning/contracts/commands.md), "Once only", and
   [datachannel](../../../../planning/contracts/datachannel.md), section 5.
5. **One `Who` for the three principals.** The seam resolves the signer to its person, that
   person's member row and the AI of `as`, and refuses a signer that is no member's device and an
   `as` that its signer does not host. A signer without `as` is a person and may also be a host;
   the function asks for the one its table names. Written into
   [commands](../../../../planning/contracts/commands.md), "Who".
6. **What is on disk is read through `Disk`, without errors.** Messages, notes and queued items
   are not in the state; a function reads them through five methods that return values only.
7. **A row is a sealed value.** `RowOf` wraps one of the eight row types of `internal/proto` and
   keeps its type, so that the caller can write it with `store.Put` and `Commit` can hold it,
   without a switch over row types in every caller.

## Alternatives considered

**A package-level map that files write to directly**, as `internal/peer` does for offer kinds. It
is less code, but an acceptance step in another package could not use a table of its own, and no
command file exists yet for the scenarios of A2.3 and A2.4 to send. It would also let two files
take one key silently.

**The run loop keeps the receipts.** `Apply` would then be free of the store, but the rule "once
only" would be split between two units, and the acceptance scenarios of this unit could not show
it. Reading and writing the receipt through an interface keeps the rule in one function.

**The store numbers the rows, after the function.** The run loop would set `seq` from `NextSeq`
on rows of eight types, which needs a switch or reflection in the run loop, and a function's
result would not be complete until the transaction ran. Numbering in `Apply` makes the result of a
command a value that a test can compare whole; the run loop checks it against `NextSeq`.

**No receipt for a refusal.** A refused command would be judged again on every repeat and could
succeed later under the same nonce, so one nonce would have two answers. The contract's "the
outcome (ok and its result fields, or the error code)" reads as written here.

**`Disk` methods that return errors.** Every function would then carry a branch for a failed read
that no rule is about, and each branch needs a test under the mutation gate. An implementation
can remember a failed read and fail the command after the function returns.

**Functions return rows as `[]any`.** Callers would need a type switch to write a row and to hold
it, above the complexity limit for eight types.

## Consequences

- A command file is one function and one `Add`; its tests need a `State`, a `Command` and, where
  it reads messages, a fake `Disk`.
- The run loop must call `NextSeq` once per row and `Commit` once per result, in that order, on
  one goroutine per group. A second writer of the same `State` breaks the numbering.
- A refused command occupies a receipt for 24 hours, like an accepted one.
- `Apply` sets `seq` and `pos` by reflection on the row. A row type whose sequence field is not
  named `Seq`, or whose creation position is not named `Pos`, panics in the first test that
  numbers it.
- The seam does not measure rows against the 16 KiB limit and does not settle to which person a
  device belongs when two logs hold it; both are open in [issues](../../../../issues.md).

## Verification

`go test ./internal/home` covers every statement of both files. Each `==`, `!=` and arithmetic
operator of the two files was flipped by hand, one at a time, and the package's tests failed for
all 38. `features/home.feature` holds A2.3 and A2.4; its step file type-checks under
`go vet -tags acceptance ./features/`. The acceptance suite and the mutation gate were not run in
the building session: the coordinator ran the long checks from one place at that time.
