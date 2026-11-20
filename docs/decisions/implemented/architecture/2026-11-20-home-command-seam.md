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
contract is silent and to write the rule into the contract. An independent review of the first
version found one defect and several gaps; the coordinator decided how each is closed, and points
6 and 8 to 13 are as those decisions left them, the skipped entry of point 12 and `State.Online`
included. The owner did not decide them in person.

1. **The table is a value.** `Table` is a map from key to function with the methods `Add` and
   `Apply`; `Commands` is the table of a node. A test or an acceptance step makes a table of its
   own.
2. **`Apply` runs inside the command's transaction and writes nothing but the receipt.** It takes
   the receipts as an interface that `*store.Tx` satisfies. The caller writes rows and items, and
   calls `State.Commit` after the commit, so memory never holds a row that the store refused.
3. **The seam numbers the rows.** A function returns rows without `seq`; `State.Seal`, which
   `Apply` calls, gives them `State.Seq + 1` onwards, and `pos` where a row has one that is 0.
4. **A refusal is a receipt.** Every command that arrived on its signer's connection, passed the
   envelope checks and names its group gets a receipt with its outcome, success or error code. A
   repeat of a refused command is answered with the same code and a fixed sentence. Those three
   checks leave no receipt. Written into
   [commands](../../../../planning/contracts/commands.md), "Once only", and
   [datachannel](../../../../planning/contracts/datachannel.md), section 5.
5. **One `Who` for the three principals.** The seam resolves the signer to its person, that
   person's member row and the AI of `as`, and refuses a signer that is no member's device and an
   `as` that its signer does not host. A signer without `as` is a person and may also be a host;
   the function asks for the one its table names. Written into
   [commands](../../../../planning/contracts/commands.md), "Who".
6. **What is on disk is read through `Disk`, without errors, inside the transaction.** Messages,
   notes and queued items are not in the state; a function reads them through five methods that
   return values only. A `Disk` is an argument of `Apply` and of every timer and handler, not a
   field of the state: it reads in the transaction that the call runs in.
7. **A row is a sealed value.** `RowOf` wraps one of the eight row types of `internal/proto` and
   keeps its type, so that the caller can write it with `store.Put` and `Commit` can hold it,
   without a switch over row types in every caller.
8. **Only the seam makes a key.** A key is `t`, or `t`, `/` and the field that the type selects.
   A `t` or a selector field that holds a `/` names no command, and a type with a selector needs
   its field. Written into [datachannel](../../../../planning/contracts/datachannel.md), section 5.
9. **Timers and events have tables of their own, and one path for their results.** `Timers` and
   `Events` hold pure functions from the state, a `Disk` and the time (and, for an event, a value
   its unit defines) to a `Result`. `State.Seal` numbers and measures any result; from there a
   result is written and committed the same way, whatever produced it.
10. **A result carries what later units write besides rows.** `Result.Writes` holds values with
    `Put(tx)` for what is no row; `Result.GroupsChanged` asks for the `groups` push;
    `Item.NotBefore` holds an item back. The seam passes them on and reads none of them.
11. **The count of the wake-up cap has its place in the state.** `State.Wakes` is a map the seam
    makes and never reads; the unit that sends items keeps it, on the goroutine of the group.
12. **A state keeps its own device logs and follows them through device rows.** `Commit` applies
    the entry of each `device` row to the log of its person, and skips an entry that the log
    refuses. The goroutine of the group is the only writer of those logs.
13. **What needs I/O after a commit is a hook.** `After` is a table of `func(s *State, res
    Result)` that the run loop calls after each `Commit`. The seam holds the table and calls
    nothing. `State.Online`, the set of connected devices, is the place for what a hook needs to
    know about connections; the run loop keeps it.

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

**`Disk` as a field of the state.** That was the first version. `Apply` runs on the store's one
writer, inside the command's transaction: a `Disk` that reads through a snapshot or through the
database waits for that writer, and on a database in memory, which has one connection, it waits
for the transaction it is called from. A `Disk` handed to each call can only be the one of that
transaction.

**The key of a payload without a selector is its `t` as it stands.** That was the first version,
and the defect the review found: `{"t":"post/text","kind":"brief"}` reached the function of
`post/text` with another kind, and `{"t":"group/create"}` reached group create.

**An event as a struct with fixed fields** (a kind, a device, an envelope). A unit whose event
needs another field could not add it without changing the seam. An event is a value of a type its
own unit defines, and its handler asserts that type.

**Device logs that the states share with the node.** In the first version whoever made a state
filled in its logs, and nothing updated them: a device added after the start stayed unknown to
the group until a restart. Handing every state the node's own `identity.Log` values would update
them, but a log has no lock, and the node and each group write and read on goroutines of their
own.

**`Commit` panics on an entry its log refuses.** That was the first version of point 12: whoever
writes a device row has had the entry accepted, so a refusal is a mistake in that code. But the
row is stored by then, and it is read again at every start: the node would stop each time it
loads the group. A skipped entry authorizes nothing, which is all that matters for it.

**A handler sends the Web Push.** A handler is pure and runs before the commit. The run loop
cannot call the push code either: it is built without it. A table of hooks lets that unit add
itself.

**The function of a command writes what is no row itself.** It would then need the transaction
and could not be tested without a database. It returns a value with `Put(tx)` and stays pure.

## Consequences

- A command file is one function and one `Add`; its tests need a `State`, a `Command` and, where
  it reads messages, a fake `Disk` in that `Command`.
- The run loop builds a `Disk` per transaction, which needs reads of the store that run in a
  transaction. It runs the timers and the handlers, and calls `Seal` for their results.
- A handler asserts the type of its event; a caller that passes another type makes it panic.
- A device row whose entry its log refuses is stored and shown to clients like any row, and
  changes nothing in the state: nothing reports the mistake of the code that wrote it.
- A hook gets the state and the result and nothing else. `State.Online` is right only as far as
  the run loop keeps it.
- The run loop must call `NextSeq` once per row and `Commit` once per result, in that order, on
  one goroutine per group. A second writer of the same `State` breaks the numbering, and so does
  a second `Seal` before the `Commit` of the first.
- A refused command occupies a receipt for 24 hours, like an accepted one.
- `Apply` sets `seq` and `pos` by reflection on the row. A row type whose sequence field is not
  named `Seq`, or whose creation position is not named `Pos`, panics in the first test that
  numbers it.
- `Seal` measures every row of a result against the size limit of a row, after numbering it,
  and refuses the whole result for one row above it. It measures the JSON without HTML escaping,
  as the contract counts and as the store writes it.
- The seam does not settle to which person a device belongs when two logs hold it, and it knows a
  device only by the logs of one group; both are open in [issues](../../../../issues.md).

## Verification

`go test ./internal/home` covers every statement of both files, the two payloads of the review's
defect among its cases. Each `==`, `!=`, `>` and arithmetic operator of the two files was flipped
by hand, one at a time, and the package's tests failed for all 45. `features/home.feature` holds A2.3 and A2.4; its step file type-checks under
`go vet -tags acceptance ./features/`. The acceptance suite and the mutation gate were not run in
the building session: the coordinator ran the long checks from one place at that time.
