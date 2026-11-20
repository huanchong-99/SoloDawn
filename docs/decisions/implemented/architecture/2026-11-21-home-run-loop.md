# Agent Note: The run loop of the home

Status: implemented

## Problem

The command seam of the home ([decision](2026-11-20-home-command-seam.md)) opens no connection,
keeps no clock and starts no transaction: "whoever runs the home" does. Unit HM-1 is that code. The
plan gave it one line ([units](../../../../planning/steps/parallel-plan-units.md), section 2): load,
write through to the store, push to clients with `state_only`, a row change before its items. The
contracts say what is pushed and in which order
([datachannel](../../../../planning/contracts/datachannel.md), sections 3 to 6;
[sync](../../../../planning/flows/sync.md), D). They do not say how a slow client is kept from
holding a group, how the wake-up cap counts an item that is sent twice, where a `group create`
runs before its group exists, or which goroutine may wait for which. The review of the host seam
had found deadlocks of exactly that last kind, between a session lock and the store's writer.

## Decision

The session that built HM-1 made these choices, under the coordinator's instruction for that unit
to decide where a contract is silent and to write the rule down; the owner did not decide them in
person. The current contract is the [module README](../../../../internal/home/README.md), "The run
loop".

1. **One goroutine per group, with a list of jobs that has no bound.** Every change of a group is
   a job of its goroutine. Handing a job in never waits, so no caller, no other group and no hook
   can wait for a group by sending it work. Only `Attach`, `Watch` and `Close` wait for a group,
   and they hold no lock while they do.
2. **The goroutine of a group waits for the store's writer and for nothing else.** What runs on
   the writer (the function of a command, a `Disk`, a `Write`) takes no lock of the loop. The
   state is changed after the transaction is committed, on the goroutine of the group, so memory
   never holds what the store refused.
3. **A connection has a queue and a goroutine of its own.** The group puts a frame into the queue
   and goes on. A connection whose client refuses a frame, or for which 1,024 frames wait, is
   given up and closed; no frame is ever left out, because a client that missed one row would
   move its cursor past it. The client reads again with `sync`, which is what the contract gives
   a client after any reconnect.
4. **A group that does not exist gets a goroutine for as long as it has jobs.** `group create`
   runs there like any command, in order with whatever else arrives for that id. The goroutine
   ends when it has no job left and the group still does not exist.
5. **A result that holds nothing reaches no hook.** A refusal, a command answered from its
   receipt and a timer with nothing to do are not given to `Commit` or to `After`. The seam
   cannot tell a repeated command from a first one by its result; this rule makes the difference
   not matter.
6. **The order after a commit is `Commit`, the count of the wake-up cap, the hooks, the answer,
   the pushes.** The pushes are the rows, then the list of groups, then the items.
7. **Items are delivered from the store, in a transaction of their own.** After a result that
   queued items, on a host's hello and on a tick when a hold has ended, the loop reads the queue
   and sends each connection the due items of its device that it was not sent yet. What a
   connection was sent is kept per connection and group, and cut down to the items still queued
   at every delivery.
8. **The wake-up cap counts an item once, when it is queued, at the time it is due.**
   `State.Wakes` gets the home's time of the commit, or the item's `NotBefore` when that is
   later. Sending an item again counts nothing.
9. **The task counter is written through.** `grp.num` is raised to the highest task number of a
   result, and a loaded state takes the higher of that counter and its task rows, so a number is
   not given out again after retention has removed the task that had it.
10. **The owner's log comes from the node, every other log from the group's device rows.** A
    state starts with the log that `LoopConfig.Logs` returns for the owner, and loading replays
    the group's device rows on top, as `Commit` applied them.
11. **The tables are part of the configuration.** `NodeTables()` returns those of a node; a test
    and an acceptance step pass their own.

## Alternatives considered

**A bounded channel of jobs per group.** A caller would wait when a group is busy, which gives
back-pressure for free. But a hook that raises an event, a timer tick and a retiring group all
hand jobs in from places that must not wait, and a job handed in under the registry lock could
then hold that lock. An unbounded list has no such case; what it costs is recorded in
[issues](../../../../issues.md).

**`Cmd` that returns the answer.** The caller would wait for the group, and the answer would no
longer be ordered with the pushes of the same connection. With the answer as a frame in the
connection's queue, a client sees its answer before the rows of its command, and a local link is
treated like a remote connection.

**Leaving frames out for a slow client, or telling it to reset.** The contract has `reset` only
inside the answer to `sync`, and a client that keeps its cursor after a gap would never ask for
the missing rows. Closing the connection uses the one path every client already has.

**One goroutine for all groups that do not exist yet.** It would hand a group over once it is
created, and a command that arrives in between would have to be passed on to the new goroutine
in order. A goroutine per id from the first command has no hand-over.

**Counting an item for the cap when it is sent.** The count would then depend on whether the host
was connected, a rule could not know the time a held item will take in the hour, and an item sent
again after a reconnect would need its own rule. Counting at the time the item is due gives a
pure function everything it needs in `State.Wakes`.

**Keeping the queued items of a group in memory.** Delivery would need no read, but the inbox
unit deletes acknowledged items in the store, and the two lists could differ. The queue is small
and is read only when items are sent.

**Building the item frames in the transaction of the result.** That saves a transaction, but the
copies of rows that an item carries are to be taken "when the item is sent", and the state holds
the result only after the commit. A delivery of its own is also the one path for a hello and for
a hold that ends.

**The node's log for every person of a group at load.** It would make a device known to a group
that no device row has shown it, which the seam's rule for device rows excludes. The owner is the
exception because `group create` needs the owner's devices before the group has any row.

## Consequences

- A hook, a timer, a handler and the function of a command must not call `Attach`, `Watch` or
  `Close`; everything else of the loop may be called from anywhere.
- The list of jobs of a group can grow without bound while a client sends faster than the group
  applies. A `peer.Connection` buffers by itself, so the limit of 1,024 frames applies to clients
  whose `Send` waits, such as a local link.
- Every tick starts one transaction per timer and group, also when the timer has nothing to do,
  because a timer reads through a `Disk`. An event starts one per group.
- A hook is not called for a command that changes nothing, also not the first time.
- An item that is queued while its host is away takes its place in the hour then, not when the
  host returns.
- The inbox unit replaces two functions, `runSeal` and `runItemFrame`, and adds `ack`.
- `sync` builds on `Loop.ask` and `runGroup.watch`; until it exists, `Watch` is the only way a
  connection subscribes.

## Verification

`go test ./internal/home` covers every statement of the loop: load after a restart, order within
a group, two groups side by side, a refused command, each step of a write that fails, a client
that holds, refuses or ends, timers, events and hook order. `go test -race` passes on
`internal/home` and `internal/store`. `features/home_run.feature` holds the home half of A2.1.
The mutation gate and the checks on Linux and Windows were not run in the building session: a
separate runner and CI do that.
