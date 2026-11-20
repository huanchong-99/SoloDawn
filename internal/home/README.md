---
description: 'The home role of a node: how a signed command to a group is checked, answered once and turned into rows and inbox items; what a command file adds to the table and what the function of a command may read. Read it before adding a file to internal/home or calling the package from internal/node.'
kind: reference
---

# Home

## Summary

`internal/home` holds the rules of the groups a node is home to
([architecture](../../planning/architecture.md), section 2.1). This page covers its seam, `home.go`
and `check.go`: the state of a group in memory, the table of commands, the checks every command
passes before its function runs, and the receipt that answers a repeated command. Every other file
of the package adds to the table or builds on these types. The seam opens no connection, keeps no
clock and starts no transaction: it is given a device id, an envelope, a time and a place for
receipts, and returns what to write.

## Use this capability

| Need | Entry |
| --- | --- |
| add a command | a function `func(s *State, c Command) (Result, *Refusal)` in a file of its own, and `Commands.Add(key, f)` in that file's `init` |
| read the payload in a function | `c.Decode(&p)`, with `p` the wire type of the command; the refusal it returns is the function's answer |
| ask who sent it | `c.Who`, `c.Who.Member()`, `s.Admin(c.Who)`, `s.Secretary(c.Who)`, `s.Hosts(c.Who, ai)` |
| change rows | `Result.Rows`, each made with `RowOf(row)` from a row type of `internal/proto` |
| queue or withdraw inbox items | `Result.Items`, `Result.Retire` |
| answer with result fields | `Result.Answer.Num`, `.Passed`, `.Round` |
| refuse | `&Refusal{Code: "conflict", Msg: "…"}`, with a code of [datachannel](../../planning/contracts/datachannel.md) section 9 |
| read a message, a note, a thread, the pending cards, the queued items | `s.Disk` |
| take a `cmd` from a connection | `Commands.Apply(s, tx, device, env, now)` inside the command's transaction, then `s.Commit(result)` once it is committed, and `result.Frame(re)` for the answer |
| know whether a device may use the home surface of a group | `s.Serves(device)` |
| push to a client | the `Client` interface |

## Contract

**Keys.** The table holds one function per key: the payload's `t`, followed by `/` and its `op` for
`group`, `member` and `task`, its `kind` for `post`, its `action` for `act`. `note`, `forget`,
`jump` and `push` are keys by themselves. `Add` panics when a key is given twice.

**Order of checks.** `Apply` answers with the first check that fails
([datachannel](../../planning/contracts/datachannel.md), section 5):

1. the signer is the device of the connection, else `forbidden`;
2. the envelope checks of [envelope](../../planning/contracts/envelope.md) section 3, with their
   own codes; a device is known when a log in `State.Logs` holds it as valid, and a `to` must be
   `State.Home`;
3. `g` is `State.ID`, else `bad_request`;
4. the receipt: a command answered before gets that outcome and nothing runs;
5. the key is in the table, else `bad_request`;
6. the group exists, else `not_found`; only `group/create` reaches its function without one;
7. `as`, when there, is a text that is not empty, else `bad_request`, and names an AI whose
   `host` is the signer, else `forbidden`;
8. the signer's person is a member of the group, or, for `group/create` on a group that does not
   exist, this node's owner, else `forbidden`;
9. the function.

Checks 1 to 3 leave no receipt. From check 5 on, the outcome is written as the receipt, a refusal
included, with the command's own `ts`. A repeated refusal is answered with its code and a fixed
`msg`, because a receipt holds no sentence.

**Who.** `Who` carries the signer, the person whose log holds it, that person's member id and the
AI of `as`. A signer without `as` is a person and, where a command's table names a host, also that
host; the function asks for the one its table names. `Admin` and `Hosts` are false for a command
that carries `as`; `Secretary` is true only for one that does. `Who.Human` is empty only in
`group/create`.

**Functions are pure.** A function reads `s` and `c` and calls nothing but `s.Disk`. It does not
change the state, read a clock (the home's is `c.Now`, the signer's `c.Timestamp`) or touch the
store. It checks its own table row: who may send the command, and that an id the payload chooses
(`n` as the id of what it creates, the `id` of `member add_ai` or of `note`) is not already taken
by another row.

**Rows.** `Apply` numbers the rows of a result in order, from `State.Seq + 1`. A function leaves
`seq` as it is. In a message or a note it creates it leaves `pos` at 0, and `Apply` sets it to the
row's first `seq`; a row that has a `pos` keeps it. `Row.Value` returns the row as its `proto`
type; `Row.Put` writes it through `store.Put`. The caller takes `NextSeq` from the store once per
row: the two counters move together.

**State.** A `State` holds the group row, the member rows of people and of AIs, the task rows, the
last sequence number and the last task number, and beside them this node's device id, its owner's
user id and the device logs by user id. `Commit` takes the group, member and task rows of a result,
drops a member whose row became a `gone` marker, raises `Num` to the highest task number and
moves `Seq` past the rows. It does not hold messages, notes or device rows. `Commit` is called
once per result, after its transaction is committed and before the next `Apply`: a `State` is used
by one goroutine.

**Disk.** `Disk` reads what a home keeps on disk only: one message, one note, a page of a thread
newest first, the pending cards oldest first, the unacknowledged inbox items oldest first. Its
methods return no error. An implementation that fails to read must make the command fail.

**Items.** An `Item` is an inbox item before the home signs it: the AI, the `why`, and the fields
[datachannel](../../planning/contracts/datachannel.md) section 6 gives that `why`. `Retire` lists
queued items, by id, that the change made pointless.

**Errors.** `Apply` returns an error only when a receipt could not be read or written. The caller
drops the transaction and answers `internal`.

**Client.** A `Client` is what a home answers and pushes to: `Send(frame)` and `Done()`. A
`peer.Connection` has both. The device a client was authenticated for is given beside it.

**One package, many files.** Every unit of the home adds files to this package, so an unexported
name is taken for all of them. The seam takes `wireRow`, `typedRow`, `refused`, `repeated`,
`keyCreate` and `keySelector`. Its tests are in package `home_test` and use only what is exported.

## Known limitations

- The table is empty and nothing in a node calls `Apply` yet: the command files and the run loop
  are separate units ([units](../../planning/steps/parallel-plan-units.md), section 2).
- No row is measured against the size limit of a row ([issues](../../issues.md)).
- A device held by the logs of two people is read as the device of the one with the lower user id
  ([issues](../../issues.md)).
- `Item` is the home's own type. The signed `item` payload on the wire is not frozen yet
  ([conventions](../../planning/contracts/conventions.md), section 5).

## Verification and sources

`go test ./internal/home` covers every statement of the seam through its exported entries, with
fakes of `Receipts` and `Disk` in the test files and one test on an in-memory store.
`features/home.feature` holds acceptance tests A2.3 and A2.4
([acceptance](../../planning/steps/step-1-acceptance.md)); its steps apply a command of their own
to a store in memory, because no command file exists yet. The choices behind the seam are in
[the decision record](../../docs/decisions/implemented/architecture/2026-11-20-home-command-seam.md).
