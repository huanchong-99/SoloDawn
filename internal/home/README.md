---
description: 'The home role of a node: how a signed command to a group is checked, answered once and turned into rows and inbox items; what a file adds to the tables of commands, timers and events, and what such a function may read and return; how the run loop applies them, one goroutine per group, and what it tells connected clients. Read it before adding a file to internal/home or calling the package from internal/node.'
kind: reference
---

# Home

## Summary

`internal/home` holds the rules of the groups a node is home to
([architecture](../../planning/architecture.md), section 2.1). This page covers its seam, `home.go`
and `check.go`: the state of a group in memory, the tables of commands, timers and events, the
checks every command passes before its function runs, and the receipt that answers a repeated
command. Every other file of the package adds to a table or builds on these types. The seam opens
no connection, keeps no clock and starts no transaction: it is given a device id, an envelope, a
time, and the reads and receipts of a transaction, and returns what to write.

The run loop, in the files whose names start with `run`, is "whoever runs the home": it owns the
states, the transactions, the clock and the connections ("The run loop" below).

## Use this capability

| Need | Entry |
| --- | --- |
| add a command | a function `func(s *State, c Command) (Result, *Refusal)` in a file of its own, and `Commands.Add(key, f)` in that file's `init` |
| read the payload in a function | `c.Decode(&p)`, with `p` the wire type of the command; the refusal it returns is the function's answer |
| ask who sent it | `c.Who`, `c.Who.Member()`, `s.Admin(c.Who)`, `s.Secretary(c.Who)`, `s.Hosts(c.Who, ai)` |
| change rows | `Result.Rows`, each made with `RowOf(row)` from a row type of `internal/proto` |
| queue or withdraw inbox items | `Result.Items`, with `Item.NotBefore` to hold one back; `Result.Retire` |
| change something that is no row | `Result.Writes`, values with `Put(tx)` |
| have every peer pushed its `groups` again | `Result.GroupsChanged` |
| show the group an accepted device-log entry | a `proto.DeviceRow` in `Result.Rows`; `Commit` applies its entry to the state's log |
| do after a commit what a pure function cannot, such as a Web Push | a function `func(s *State, res Result)` and `After.Add(name, f)` in its file's `init` |
| know in a hook which devices are connected | `s.Online[device]` |
| answer with result fields | `Result.Answer.Num`, `.Passed`, `.Round` |
| refuse | `&Refusal{Code: "conflict", Msg: "…"}`, with a code of [datachannel](../../planning/contracts/datachannel.md) section 9 |
| read a message, a note, a thread, the pending cards, the queued items | `c.Disk` in a command, the `Disk` argument of a timer or a handler |
| add a rule that the clock sets off | a function `func(s *State, d Disk, now int64) Result` and `Timers.Add(name, f)` in its file's `init` |
| add a rule for an event outside the commands | a function `func(s *State, d Disk, now int64, event any) Result` and `Events.Add(name, f)` in its file's `init` |
| take a `cmd` from a connection | `Commands.Apply(s, disk, tx, device, env, now)` inside the command's transaction, then `s.Commit(result)` once it is committed, and `result.Frame(re)` for the answer |
| run a timer or a handler | `s.Seal(f(s, disk, now, …))` inside a transaction, then write and `s.Commit(result)` as for a command |
| finish any result | after `s.Commit(result)`, every hook of `After`, on the goroutine of the group |
| know whether a device may use the home surface of a group | `s.Serves(device)` |
| run the home of a node | `StartLoop(LoopConfig{…})` with `NodeTables()`; `Close` before the store is closed |
| connect a client | `loop.Attach(device, client)` returns its `*Conn`; `loop.Detach(conn)`; `conn.Gone()` |
| hand in a `cmd` frame | `loop.Cmd(conn, re, env)`; the answer is sent to the connection |
| answer a `hello` | `loop.Welcome(conn, build)`: `build` makes the `welcome` frame from the groups of the peer |
| read a group and subscribe (`sync`) | `loop.Sync(conn, req)`; queues `reset` when needed, `rows` and `synced`, then pushes changes after the cut |
| send a host its inbox items, on every `hello` | `loop.Inbox(conn)` |
| run the timers by hand, or tell the home of an event | `loop.Tick(now)`, `loop.Event(name, event)` |
| move a task | `Step(task, event)`: one task row and one `Event` in, the row and its `Effects` out ("The task machine" below) |
| push to a client | the `Client` interface |

## Contract

**Keys.** The table holds one function per key: the payload's `t`, followed by `/` and its `op` for
`group`, `member` and `task`, its `kind` for `post`, its `action` for `act`. `note`, `forget`,
`jump` and `push` are keys by themselves. The `/` of a key is made by the seam and by nothing in
a payload: a `t`, or an `op`, `kind` or `action`, that holds a `/` names no command, and neither
does one of those five types without its field. `Add` panics when a key is given twice.

**Order of checks.** `Apply` answers with the first check that fails
([datachannel](../../planning/contracts/datachannel.md), section 5):

1. the signer is the device of the connection, else `forbidden`;
2. the envelope checks of [envelope](../../planning/contracts/envelope.md) section 3, with their
   own codes; a device is known when a log in `State.Logs` holds it as valid, and a `to` must be
   `State.Home`;
3. `g` is `State.ID`, else `bad_request`;
4. the receipt: a command answered before gets that outcome and nothing runs;
5. the payload names a key, and the key is in the table, else `bad_request`;
6. the group exists, else `not_found`; only `group/create` reaches its function without one;
7. `as`, when there, is a text that is not empty, else `bad_request`, and names an AI whose
   `host` is the signer, else `forbidden`;
8. the signer's person is a member of the group, or, for `group/create` on a group that does not
   exist, this node's owner, else `forbidden`;
9. the function;
10. no row of its result is above the size limit of a row, else `bad_request`.

Checks 1 to 3 leave no receipt. From check 5 on, the outcome is written as the receipt, a refusal
included, with the command's own `ts`. A repeated refusal is answered with its code and a fixed
`msg`, because a receipt holds no sentence.

**Who.** `Who` carries the signer, the person whose log holds it, that person's member id and the
AI of `as`. A signer without `as` is a person and, where a command's table names a host, also that
host; the function asks for the one its table names. `Admin` and `Hosts` are false for a command
that carries `as`; `Secretary` is true only for one that does. `Who.Human` is empty only in
`group/create`.

**Functions are pure.** A function reads `s` and `c` and calls nothing but `c.Disk`. It does not
change the state, read a clock (the home's is `c.Now`, the signer's `c.Timestamp`) or touch the
store, and checks its own table row, including who may send the command. A timer and a handler are
pure in the same way.

**Rows.** `Apply` numbers the rows of a result in order, from `State.Seq + 1`, by `State.Seal`. A function leaves
`seq` as it is. In a message or a note it creates it leaves `pos` at 0, and `Apply` sets it to the
row's first `seq`; a row that has a `pos` keeps it. `Row.Value` returns the row as its `proto`
type; `Row.Put` writes it through `store.Put`. The caller takes `NextSeq` from the store once per
row: the two counters move together.

**Sizes.** A result that holds a row above 16 KiB, or a card above 12 KiB that it creates, is
refused as a whole by `State.Seal` with `bad_request`, and the `msg` names the size and the limit
([conventions](../../planning/contracts/conventions.md), section 4). The size is that of the row's
JSON without HTML escaping, with its `seq` and `pos` set. A card is a message whose `state` is not
`null`; it is being created when its `pos` is its `seq`.

**State.** A `State` holds the group row, the member rows of people and of AIs, the task rows, the
last sequence number and the last task number, and beside them this node's device id, its owner's
user id and the device logs by user id. `Wakes` is the place for the count of the wake-up cap:
by AI, when each item of the last hour was sent. `Online` is the set of devices that have a
connection to this home now, for a hook to read. The seam only keeps these two places: whoever
sends the items keeps `Wakes`, whoever runs the home keeps `Online`, and the goroutine of the
group owns both, like the rest of the state. `Commit` takes the group, member and task rows of
a result, drops a member whose row became a `gone` marker, raises `Num` to the highest task number
and moves `Seq` past the rows. It does not hold messages or notes. `Commit` is called once per
result, after its transaction is committed and before the next `Apply` or `Seal`: a `State` is
used by one goroutine, the group's.

**Device logs.** The logs of a `State` are its own values: no other state and no other goroutine
holds them. `Commit` applies the entry of each `device` row of a result to the log of its person,
and starts a log with a person's first row, so a device is known to the group, or no longer known,
from the row that says so. The same entry in a row again changes nothing. An entry that the log
refuses is skipped: it makes no device known, leaves the log as it was and starts no log. Its row
is still a row of the result, written and numbered like the others.

**Disk.** `Disk` reads what a home keeps on disk only: one message, one note, a page of a thread
newest first, the pending cards oldest first, the unacknowledged inbox items oldest first. A `Disk`
belongs to one transaction of the store and to one call: it is an argument of `Apply`, which puts
it into the `Command`, and of a timer or a handler. `Apply` runs on the store's writer, where a
read outside that transaction would wait for it. Its methods return no error. An implementation
that fails to read must make the call fail.

**Timers and events.** `Timers` and `Events` hold the rules that no command selects, under names
their files choose. A `Timer` gets the state, a `Disk` and the home's time; a `Handler` also gets
the event, a value that the unit of that kind of event defines. Both return a `Result`, which
`State.Seal` numbers and measures as `Apply` does for a command; a refused result has no rows.
The seam runs neither table: whoever runs the home does, in a transaction per result.

**After a commit.** `After` holds hooks, `func(s *State, res Result)`, for what a pure function
cannot do. Whoever runs the home calls every hook after each `Commit`, on the goroutine of the
group, with the result of a command, a timer or an event and the state that now holds it, which
includes `Online`. The seam calls none.

**Other writes.** A `Write` in `Result.Writes` is a change that is no row. The function only
returns it; the caller of `Apply` calls its `Put` in the command's transaction, after the rows.
`Result.GroupsChanged` asks the caller to push every connected peer its `groups` again once the
result is committed.

**Items.** An `Item` is an inbox item before the home signs it: the AI, the `why`, and the fields
[datachannel](../../planning/contracts/datachannel.md) section 6 gives that `why`. `NotBefore`
holds an item back until that time. `Retire` lists queued items, by id, that the change made
pointless.

**Errors.** `Apply` returns an error only when a receipt could not be read or written. The caller
drops the transaction and answers `internal`.

**Client.** A `Client` is what a home answers and pushes to: `Send(frame)` and `Done()`. A
`peer.Connection` has both. The device a client was authenticated for is given beside it.

**One package, many files.** Every unit of the home adds files to this package, so an unexported
name is taken for all of them. The seam takes `wireRow`, `typedRow`, `rowLimit`, `cardLimit`,
`rowFits`, `sizeLimit`, `refused`, `repeated`, `addOnce`, `keyCreate` and `keySelector`. Its tests
are in package `home_test` and use only what is exported. The task machine takes the names that
start with `task`, `step`, `wait`, `item` and `states`, and `workItems`, `byAssignee` and
`byReviewer`; its tests take the names that start with `mc` and `machine`. The run loop takes the
names that start with `run`, and its tests those that start with `run` and `Run`; it exports
`Loop` with its entries, `LoopConfig`, `StartLoop`, `Tables`, `NodeTables`, `Conn`, `QueuedItem` and
`ItemsPush`. Groups and members take the names that start with `grp` and `mem`, their tests too,
and their test functions start with `TestGroup` and `TestMember`. The briefs take the
names that start with `brief`, their tests too, and their test functions start with `TestBrief`.
The task commands (`cmd_task.go`) take the names that start with `tsk`, their tests too, and their
test functions start with `TestTaskCmd`. The posts of HM-6 take the names that start with `post`,
their tests too, and their test functions start with `TestPost`; the notes and `forget` take the
names that start with `note`, their tests too, and their test functions start with `TestNote`.
Device-log events (`ev_device.go`) take the names that start with `dev`, and their tests start with
`TestDevice` and `dev`. Sync takes the names that start with `syn`, its tests too, and its test
functions start with `TestSync`; it exports `Loop.Sync`.

## The run loop

`StartLoop` reads every group from the store and starts one goroutine per group, which owns that
group's `State`. Everything that changes a group is a job of its goroutine, done strictly in the
order it was handed in; two groups run side by side.

**Loading.** A group is one that has a `group` row. Its state is the owner's device log as
`LoopConfig.Logs` gives it, then the group's state rows in the order of their sequence numbers,
then its device rows in the order of their versions, then the two counters of `grp`. The device
rows go by version because that is the order in which `Commit` applied their entries: a row that
was written again has a later sequence number than the entry after it, and by sequence number a
revoked device would be valid again after a restart. `Seq` is the stored counter, `Num` the higher of the stored counter and the task rows. A
store that cannot be read, or a state row that does not decode, starts no loop.

**One result, one transaction.** A command, a timer and an event go the same way:

1. In one `Write` of the store: `Commands.Apply`, or `State.Seal` of what a timer or a handler
   returned, with a `Disk` of that transaction; then the rows, each under the next `NextSeq`,
   which must be the number `Seal` gave it; the task counter (`RaiseNum`, to the highest `num`
   among the rows); the items; the retirements; the other writes.
2. If anything failed, a read of the `Disk` included, the transaction is dropped, the failure
   goes to `LoopConfig.Report`, a command is answered `internal`, and the state is as before.
3. Otherwise, for a result that holds something: `State.Commit`, the count of the wake-up cap,
   every hook of `After` in the order of their names, the answer, the pushes.

A result that holds nothing only gets its answer: a refusal (whose receipt is committed), a
command answered from its receipt, a timer with nothing to do. It reaches no hook. A timer's or a
handler's result that `Seal` refuses is reported and changes nothing.

**Groups that do not exist.** A command for a group without a goroutine starts one, with a state
that knows the owner's log only, so that `group create` can run there. If the group still does
not exist when that goroutine has no job left, it ends and the loop forgets it. A command whose
payload has no `g` is answered where it arrives, by the checks that leave no receipt, in their
order; only the owner's devices are known to that answer.

**Timers and events.** `Tick(now)` runs every timer for every group that exists, in the order of
their names, each in a transaction of its own; with `LoopConfig.Every` the loop calls it by
itself. `Event(name, event)` runs that handler for every group that exists; the handler returns
an empty result for a group the event does not concern.

**Pushes** ([datachannel](../../planning/contracts/datachannel.md), sections 3, 4 and 6). After
the answer, in this order:

1. `rows`, to each connection that watches the group (`Sync`): all rows of the result, or with
   `state_only` the `group`, `member`, `device` and `task` rows, markers of those types included.
   A frame takes rows up to 61,376 bytes; the rest follows in further frames. A connection that
   would get no row gets no frame. A connection whose device the group no longer serves
   (`State.Serves`, on the state that holds the result) stops watching with that result and is
   not pushed its rows: a revoked device and a member who left read nothing more on a connection
   that is still open ([datachannel](../../planning/contracts/datachannel.md), section 8). It
   watches again only through `Sync`.
2. `groups`, to every connection that has had its welcome, when the result has `GroupsChanged`:
   the groups that exist and whose `State.Serves` holds for the connection's device, in the
   order of their ids, `[]` when there are none.
3. The inbox items the result queued, to the connections that take them and whose device the
   group serves. So a row change reaches a host before the items it caused.

**Welcome.** `Welcome(conn, build)` reads the list of groups for the connection's device, has
`build` make the `welcome` frame from it and queues that frame, in one step under the lock that
also orders the `groups` pushes; and a connection is pushed `groups` only once it has had a
welcome. So no `groups` push comes before the welcome, and none that is older than the welcome
comes after it. A node calls it for every `hello`. `Groups(conn)` reads the same list without
queuing anything; a frame built from it can be older than a push that is already queued.

**Inbox items.** An item is stored in the transaction of its result, for the host of its AI as
that result leaves the member rows; an item for a member that is no AI fails the transaction.
`Inbox(conn)` makes a connection the taker of the items of its device: it is sent every item
that is due now, also those it was sent before, and from then on each item when it is queued or
when its `NotBefore` has passed, which the loop looks at on a tick. Each delivery reads the queue
from the store, so an item that was acknowledged or retired is not sent again. A queue that
cannot be read is reported and read again on the next tick. An item that cannot be made into a
frame is reported and left out, once for each connection until its next `Inbox`, and the other
items are sent. Until the inbox unit signs items, an item is
kept and sent in the seam's form: the store's `env` holds the `Item` as JSON, and the frame is an
`ItemsPush` with one `QueuedItem`. That unit replaces `runSeal` and `runItemFrame` in
`run_push.go` and nothing else.

**The wake-up cap.** `State.Wakes[ai]` holds one time for each item queued for that AI: when the
item is due, which is the home's time when its result was committed, or its `NotBefore` when that
is later. An item counts once, when it is queued. Sending it again, on a hello after a reconnect
or to a second connection of its host, counts nothing, and so does a host that was not connected
when the item became due. Retiring or acknowledging an item does not take its time back. Times
that are an hour old or older are dropped from an AI's list when its next item is counted, so a
rule that reads `Wakes` counts only the times after `c.Now` minus one hour, and can count on a
held item's time being in the future. The count is in memory and starts again with the loop, as
the contract says; items sent again after a restart count nothing.

**Online.** `State.Online[device]` is true from `Attach` until the last connection of that device
is gone, in every group, also for a device that the group does not serve. `Gone()` is closed
after every group has been given the job of forgetting the connection, so a command handed in
after it finds the device offline.

**Connections.** `Attach(device, client)` gives a `Client` a queue of its own and one goroutine
that sends from it, so the goroutine of a group never waits for a client. No frame is ever left
out, because a client's rows must have no gap. Instead, a connection is given up when its client
returns an error from `Send`, or when 1,024 frames wait for it: nothing more is sent, every group
forgets it, `Gone()` is closed, and the client is closed if it has a `Close` method, as a
`peer.Connection` has. The client then connects and reads again. A client that is done
(`Done()`) and one that the node detaches are forgotten in the same way and not closed. A
client's `Send` must return once the client is done or closed.

**For the node and the local link.** A loop is started with a `LoopConfig`: the store, the node's
device id, its owner's user id, the tables, and `Logs`, which returns the node's verified log of a
person as a value of its own (a second `identity.Replay` of the entries, not the log the node
keeps), or nil. `Now`, `Report` and `Every` may be left out: the machine's clock, the standard
logger, and no ticks but `Tick`. A connection goes through four entries, the same for a
`peer.Connection` and for the local link of a host on the same node:

1. `conn := loop.Attach(device, client)`, once the device is authenticated.
2. For each frame the client sends, the entry of its type: for `hello`,
   `loop.Welcome(conn, build)`, where `build` gets the `groups` of the welcome and returns the
   whole frame from values the node has ready, and never a `Send` of the node's own, and after it `loop.Inbox(conn)` for a host;
   `loop.Cmd(conn, re, env)` for `cmd`, with the `id` of the request as `re`;
   `loop.Sync(conn, req)` for `sync`; `fetch` and `ack` come with their units.
3. The loop calls `client.Send(frame)` for every answer and push, one at a time, in order. The
   answer to a command is a frame like the others and comes after the frames queued before it.
   So a `Send` must not wait for something that the sender of a command holds while it waits for
   its answer.
4. `loop.Detach(conn)`, or the client's `Done()`. `conn.Gone()` tells the node that the loop
   gave a connection up; the node need not detach it then.

**Who may call what.** `Cmd`, `Detach`, `Welcome`, `Inbox`, `Tick`, `Event` and `Groups` never
wait for a group and may be called from anywhere. The `build` of a `Welcome` runs under the lock
of the loop: it only puts the frame together from the list of groups and from values the node
worked out before it called `Welcome`, such as `time` and `addrs`. It takes no lock, waits for
nothing and calls nothing, the loop least of all; a `build` that waits for a lock of the node
which another goroutine holds while it calls `Cmd` waits for ever. The loop calls `LoopConfig.Logs`, `Now`
and `Report` on the goroutines of its groups, and `Logs` and `Now` also on the caller of `Cmd`:
none of the three may wait for a lock that is held around a call of `Attach`, `Sync`, `Close` or
`Cmd`, and none may call those. `Attach`, `Sync` and `Close` wait for the groups: they must not be
called on the goroutine of a group, which is where a hook runs, or on the store's writer, which is
where the function of a command, a timer, a handler, a `Disk` and a `Write.Put` run. A client's
`Send` runs on the goroutine of its connection and may call every entry. A hook may start
transactions of the store; what runs on the store's writer may not. The goroutine of a group waits
for the store's writer and for nothing else, and what runs on the writer takes no lock of the loop:
so no lock of a client, of a host's session or of the node can close a circle through the loop, as
long as that code keeps to this paragraph.

**For the files that build on the loop.** `loop.ask(group, f)` runs `f` on the goroutine of a
group and waits for it; there `runReader(g.state, conn.device)` checks group existence and access,
`runErr(re, code, msg)` builds an error answer, `g.watch(conn, stateOnly)` subscribes a seated
connection, `g.transact` and `g.settle` write and finish a result, `g.deliver` sends items, and
`conn.send(frame)` queues a frame behind the ones before it. Both `Loop.Sync` and `Loop.Fetch`
take a typed request on `(conn, req)` and read their `store.Snapshot` on the goroutine of the
group through `loop.ask`. `runDisk` is the `Disk` of a transaction.

## The task machine

`machine.go` holds the table of [the task machine](../../planning/contracts/task-machine.md) as one
function, `Step(task, event)`: one `proto.TaskRow` and one `Event` in, the row as the event leaves
it and its `Effects` out. It is pure and reads no other row, no `State` and no `Disk`. Every
function that moves a task calls it and adds nothing to its rules.

- **The event.** `Kind` names the row of the table (`OnPause`, `OnSubmitted`, …). `By` is the
  sender as the function has established it from `c.Who`: `ByHost` for the host of the task's
  assignee, `BySecretary`, `ByAdmin`, `ByPerson`, `ByAI` for a working AI and never for the
  secretary, `ByHome` for a timer or for what follows from another task's row. `Member` is the
  `as` of the command; the machine itself sees whether that AI is the task's assignee or reviewer.
  `Now`, `Limits` and `Remote` come from the command and the group row. `Pending` is the task's
  pending cards, oldest first: every row that sets or clears a card wait reads it.
- **The effects**, in the order to apply them. `Retire` selects queued items of the task to
  withdraw; an empty field selects every value. `Items` are the items to queue. Each is a
  `TaskItem`, which the function completes into an `Item`: the task, the message and the cause its
  row names, the pull request, and the secretary as the AI of a `notice`. A `review` item with
  `IfNone` is queued only if none is queued for that AI and commit. `Cards` says what the task's
  pending cards become. `System` is the code of the system message to write, in the main line when
  `Main` is set and in the task's thread otherwise.
- **A refusal** is a `TaskRefusal`; its text is the code of the answer: `conflict`, `paused` or
  `forbidden`. The row comes back unchanged.
- **Other tasks.** One call moves one row. After `OnMerged` the function sends `OnReleased` to each
  queued task that depends on the merged one, with `Blocked` set while another of its dependencies
  is not merged. After `OnCancel` it sends `OnOrphaned` to each queued task that depends on the
  cancelled one. For `group resume` it sends `OnGroupResume` to every task.
- **Left to the function.** The checks its row makes on a card or on the group; the fields a
  command only copies into the row (`branch`, `pr`, `pr_url`, `stats.added`, `stats.removed`,
  `thread.count`, `thread.last`); the bodies of the messages; for row 23, the timer and telling the
  secretary once per quiet period. The machine sets `stats.ended`.

## Groups and members

`cmd_group.go` holds `group` `create` and `set` ([commands](../../planning/contracts/commands.md)
1.1; [rows](../../planning/contracts/rows.md) 1 and 2), under `group/create` and `group/set`;
`group` `pause` and `resume` belong to `cmd_pause.go` (unit HM-14). `cmd_member.go` holds `member`
`add_ai`, `set` and `remove` ([commands](../../planning/contracts/commands.md) 1.2), under
`member/add_ai`, `member/set` and `member/remove`; `member` `status` belongs to `cmd_hostev.go`
(unit HM-16). Where the contracts are silent, they do this:

- **`group/create`.** Only a person of this node's owner (`c.Who.AI` empty, `c.Who.User ==
  s.Owner`) may create a group, else `forbidden`. A `g` that differs from `n`, a `kind` other than
  `project`, a `forge` other than `github` or `none`, or an empty `name`, `repo`, `base` or
  `member_name` is `bad_request`. A group that already exists (`s.Group.ID` not empty) is answered
  `conflict`. It writes the `group` row (`pass` 90, `pass_foundation` 70, default `limits`,
  `created` `c.Now`) and the owner's `human` `member` row (`role` `owner`, `online` true,
  `last_seen` `c.Now`), with `GroupsChanged` true. The owner's member id is the first 16 bytes of
  the SHA-256 of `SoloDawn group owner <n>` in base64url, so the function stays pure
  ([issues](../../issues.md)).
- **`group/set`.** By an admin, else `forbidden`. Sending none of `name`, `checks`, `pass`,
  `pass_foundation`, `limits` and `secretary`, or an empty `name`, is `bad_request`. `secretary`,
  when sent, must name an AI in `State.AIs` that is neither assignee nor reviewer of an open task
  (`queued`, `active`, `paused`, `failed`), else `conflict`; any previous secretary's member row
  loses `secretary`, and the named AI gains it when not already set. `GroupsChanged` is set when
  `name` changes, because `groups` carries each group's name.
- **`member/add_ai`.** By a host of a member: `c.Who.AI` must be empty, else `forbidden`; the
  `name` of the latest `add` entry for `c.Who.Device` in `State.Logs[c.Who.User]` becomes
  `host_name`. An empty `name`, a `cli` outside `claude` and `codex`, or an `account` outside
  `personal`, `team` and `api_key` is `bad_request`. `secretary: true` when the group already has a
  secretary requires an admin, else `forbidden`. An `id` or `name` already held by a human or AI
  member of the group is `conflict`. The new `ai` `member` row starts with `state` `dormant`,
  `since` and `last_seen` `c.Now`, `path` `local` when `c.Who.Device == s.Home` and `direct`
  otherwise, `allow` `[]` and `usage` `{}`; when `secretary` is true, any previous secretary loses
  the flag and `group.secretary` becomes `id`.
- **`member/set`.** Sending none of `name`, `account`, `allow` and `approvers`, an empty `name`, or
  an unknown `account` is `bad_request`. An unknown `id` is `not_found`; a human member row is
  `forbidden` in step 1. The signer must be the AI's owner (`c.Who.AI` empty, `c.Who.User ==
  ai.User`) or, when `approvers` is the only field sent, the AI's host (`s.Hosts`), else
  `forbidden`. A `name` held by another member in the group is `conflict`; keeping the AI's own
  `name` is accepted.
- **`member/remove`.** An unknown `id` is `not_found`; a human member row is `forbidden` in step 1.
  The signer must be the AI's owner or an admin, else `forbidden`. Removing an AI that is assignee
  or reviewer of an open task is `conflict`. Otherwise the member row becomes a `gone` marker and
  any queued inbox items for that AI (`c.Disk.Queued`) are retired in `Result.Retire`. Removing the
  secretary also clears `group.secretary` and writes `system` `no_secretary` in the main line with
  id `n`.

## Briefs

`cmd_brief.go` holds `post` of kind `brief` and `act` `confirm` and `revise` on it
([commands](../../planning/contracts/commands.md) 1.3 and 1.4;
[messages](../../planning/contracts/messages.md) 2.1 and 3.1), under the keys `post/brief`,
`act/confirm` and `act/revise`. Where the contracts are silent, it does this:

- **Post.** Only the secretary posts a brief, else `forbidden`; a group without one has nobody who
  can. What the payload says by itself is `bad_request`: a `thread` that is not `null`, no
  criterion, no task, a dependency that is not an earlier entry or that is named twice, an entry
  whose assignee is its reviewer, and a member named twice in `mentions`
  ([commands](../../planning/contracts/commands.md) 1.3, shared with `cmd_post.go`).
  What the group says is `conflict`: a mentioned id or requester who is no member,
  an assignee or reviewer who is no working AI (an AI member other than the secretary), and a
  `replaces` that names no brief that is `pending` or `revise`.
  The row is a main-line card with `main` false, `ts` as signed, `state` `pending`, and a body
  encoded again from the checked fields. A post creates no task and queues no item (A5.2). The
  replaced brief keeps its state with `status` `superseded`.
- **Act.** `confirm` has neither `arg` nor `text`, `revise` no `arg` and at most a `text` that is
  not empty and is at most 1,024 bytes as a JSON string, its quotes included; else `bad_request`.
  Then an act names no message: `not_found`; a message that is no brief: `bad_request`; a signer
  that is not a person who is the requester or an admin, which an `as` never is: `forbidden`; a
  brief that is not `pending`: `conflict`. The brief's `state` gets `by` (the person's member id),
  `at` (the home's time) and the signed `act`; a `revise` with a `text` also gets `note`.
- **Confirm.** The checks of the post are made again against the group as it is now, and a brief
  that fails one is answered `conflict`. Two are left: the brief it replaces has been superseded
  since the post, and the task machine refuses an entry whose assignee is its reviewer
  ([issues](../../issues.md)). Then each entry becomes a task through `Step` with `OnConfirm`,
  `By` `ByPerson` (row 1 takes a person and an admin alike), and `Blocked` when the entry has a dependency: a dependency is an
  earlier entry, created by the same confirm, so it is not merged. The row has `num` from
  `State.Num + 1` in the order of the entries, `origin` `brief`, `brief` the card's id, `slug`
  from `proto.MakeSlug`, `pass` the brief's or, for a foundation task, the group's
  `pass_foundation`, `depends` as task ids, and `created` and `stats.started` the home's time.
  Each task has a `task` message in the main line, with the task's own id.
- **Ids.** The id of the task of entry `i` is the first 16 bytes of the SHA-256 of
  `SoloDawn brief task <n of the act> <i>`, in base64url: a function has no randomness, and
  nobody can know the id before the act arrives. An id that a task or a message already has is
  answered `conflict`.
- **Items.** Each item of row 1 (`assign` for a task that starts) names the task and its task
  message, with the act as `cause`. Then the secretary gets `confirmed`, naming the brief, with
  the act as `cause`, after a `confirm` and after a `revise`; a group without a secretary gets
  none. A queued task gets its `assign` when the merge of its last dependency releases it (unit
  HM-12): its cause is the act in `state.act` of the brief that `brief` names, which
  `c.Disk.Msg` reads.

## Task commands

`cmd_task.go` holds `task` `start`, `branch`, `assign`, `pause`, `resume`, `cancel`, `fail` and
`preempted` ([commands](../../planning/contracts/commands.md) 1.5;
[task-machine](../../planning/contracts/task-machine.md) rows 2, 3 and 10 to 16), under the keys
`task/start`, `task/branch`, `task/assign`, `task/pause`, `task/resume`, `task/cancel`, `task/fail`
and `task/preempted`. Checks, submissions and reviews live in `cmd_review.go` (unit HM-11), and
merges in `cmd_merge.go` (unit HM-12). Where the contracts are silent, it does this:

- **Start.** Only a working AI starts a task, else `forbidden`. An `origin` other than `direct` or
  `takeover` is `bad_request`. An `id` already held by a task or a message is `conflict`. The row
  gets `num` `State.Num + 1`, `slug` from `proto.MakeSlug`, `pass` `group.pass`, `depends` `[]`,
  and `created` and `stats.started` the home's time; its `task` message in the main line has the
  task's own `id`. The answer carries `num`.
- **Branch.** After `Step` checks the task and sender (`not_found`, `forbidden`, `conflict`), a
  `branch` other than `task/<num>-<slug>` is answered `bad_request`.
- **Sender.** For an existing task, `s.Secretary` maps to `BySecretary`, any other `as` to `ByAI`,
  `s.Hosts` for the task's assignee to `ByHost`, `s.Admin` to `ByAdmin`, and anything else to
  `ByPerson`. Checking `s.Hosts` before `s.Admin` gives the assignee's host row 13b on `task resume`
  and the host rules on `pause`, `assign` and `cancel`.
- **Assign.** Sending neither `assignee` nor `reviewer` is `bad_request`. After `Step` checks the
  sender, the state and that the two roles differ, each role sent must name a working AI of the
  group, else `conflict`. When `Step` queues an `assign` item (an active task whose assignee
  changed, or that was `failed` or waiting on `conflict`), its `msg` is the newest `checks` or
  `review` message in the task's thread, paged through `c.Disk.Thread`, or absent when there is
  none. On a `queued` or `paused` task `Step` queues no work item.
- **Cards and queue.** Every step over an existing task passes the task's pending `approval` and
  `decision` cards from `c.Disk.Pending()`, oldest first. `Retire` withdraws matching items of that
  task from `c.Disk.Queued()` before `IfNone` checks what remains. `Cards` `reassigned` updates the
  task's pending `approval` cards; `expired` updates its pending `approval` and `decision` cards.
- **Cancel.** After `OnCancel`, each `queued` task whose `depends` names the cancelled task is
  stepped with `OnOrphaned`, in sorted task id order, queuing `dependency_cancelled` notices when
  the group has a secretary.
- **System messages.** `task fail` refuses a `reason` other than `crash` or `timeout` with
  `bad_request`. At `limits.failures` (row 14) it writes `system` `task_failed` under id `n` in the
  task's thread (or answers `conflict` if `n` is taken), mentioning the brief's requester when still
  a member and the assignee's owner without duplicates, and sets the `task_failed` notice's `msg` to
  that message. `task preempted` reads the jump message `msg` (`not_found` if absent) and writes
  `system` `preempted` under id `n` in the task's thread. Both thread messages advance
  `task.thread.count` and `task.thread.last`. A group without a secretary queues no `notice`.

## Posts, notes and forget

`cmd_post.go` holds `post` of kinds `text`, `progress`, `question` and `report`
([commands](../../planning/contracts/commands.md) 1.3;
[messages](../../planning/contracts/messages.md) 1 and 2.7;
[rows](../../planning/contracts/rows.md) sections 4 and 5), under `post/<kind>`. `cmd_note.go`
holds `note` and `forget` ([commands](../../planning/contracts/commands.md) 1.6;
[rows](../../planning/contracts/rows.md) sections 5 and 6). Where the contracts are silent, they do
this:

- **Who and placement.** `text` is by a person or an AI; `progress`, `question` and `report` are by
  an AI, else `forbidden`. `progress` and `report` belong in a thread (`thread` not `null`), else
  `bad_request`. None of the four is raised from a thread, and the row is written with `main`
  `false`.
- **Thread and mentions.** A non-null `thread` must name a task in `State.Tasks`, else `not_found`;
  for `report`, `as` must be that task's `assignee`, else `forbidden`. `(*State).postMentions`,
  shared with `cmd_brief.go`, checks the `mentions` rule of
  [commands](../../planning/contracts/commands.md) 1.3. `re` is kept as signed without looking up
  the answered message, which retention or `forget` may have removed.
- **Rows and items.** Every post writes its `MsgRow` with `ts` as signed and `body` encoded without
  HTML escaping; a post in a thread follows it with the updated `TaskRow` (`thread.count` + 1, and
  `thread.last` with `text` cut to at most 200 UTF-8 characters). A post never queues an item for
  its own author ([commands](../../planning/contracts/commands.md) 1.3): `text`, `progress` and
  `question` queue `mention` for each other mentioned AI in `mentions` order, and a main-line `text`
  from a person that mentions no AI queues `mention` for `Group.Secretary` when set.
- **`note` and `forget`.** `note` is by a person or an AI; a non-null `task` that names no task in
  `State.Tasks` is `not_found`, and the `NoteRow` gets `ts` as signed (`c.Timestamp`). `forget` is
  by an admin only (`forbidden`); `r` must be `note` or `msg` (`bad_request`); a missing or already
  forgotten row is `not_found`; a `msg` whose `kind` is not `review` is `conflict`; otherwise a
  `GoneRow` (`gone: true`) replaces it.

## Device-log events

`ev_device.go` registers `Events["device"]`, which receives a `[]proto.DeviceRow` slice accepted by
the node for one person ([envelope](../../planning/contracts/envelope.md), section 4;
[rows](../../planning/contracts/rows.md), section 2). In step 1 it ignores users other than
`State.Owner`, appends rows whose `ver` exceeds the entries already held in `State.Logs[State.Owner]`,
and sets `Result.GroupsChanged` when an `Online` device's access (`State.Serves`) changes between
served and unserved.

## Sync

`sync.go` answers `sync` requests on `Loop.Sync(conn, req)` with a typed `proto.Sync`
([datachannel](../../planning/contracts/datachannel.md) section 4;
[sync](../../planning/flows/sync.md) sections A, C and E).

- **Checks.** A group that has no goroutine or whose `group` row does not exist is answered
  `not_found`; a connection whose device `State.Serves` refuses is answered `forbidden`
  (`runReader`); a connection that has already detached receives nothing.
- **The cut.** Like `Loop.Fetch`, `Loop.Sync` runs its read on the goroutine of the group through
  `loop.ask`. There it opens a `store.Snapshot`, reads at `S = snap.Seq`, closes the snapshot,
  queues `reset` (when resetting), `rows` batched to 61,376 bytes by `runBatches` (no frame when
  there are zero rows) and `synced {re, ok: true, g, seq: S}`, and then calls
  `g.watch(conn, req.StateOnly)` before returning. Every change committed after `S` runs as a later
  job of the group's goroutine, so its `rows` push is queued strictly after `synced` and is neither
  missing nor sent twice. Any store error (`Snapshot`, `DeltaCount`, delta or cold read) is passed
  to `LoopConfig.Report` and answered `internal` without subscribing the connection.
- **Delta or reset.** Where the contract does not state what happens when `since > S` (for example
  a client cursor from before a store rebuild), `sync` resets so stale higher-numbered rows are
  dropped. A delta is sent when `0 < since <= S`, `since >= purged` and `DeltaCount(since, S) <= 1000`:
  all rows in `(since, S]` in increasing `seq` order, `gone` markers included, or those whose `r`
  is in `runStateRows` (`group`, `member`, `device`, `task`, markers included) when `state_only` is
  true, sharing one rule with live pushes. Otherwise `sync` first queues `reset` (including for
  `since <= 0`), then for `state_only` sends `StateRows`, and for a full reader merges `StateRows`,
  `Pending` and the newest 50 main-line messages (`PageThread(nil, 0, 50)`) without duplicates in
  increasing `seq` order.

## Known limitations

- The table of commands holds the groups, members, briefs, the task commands of HM-9, the four
  kinds of `post` of HM-6, `note` and `forget`, and no node starts a loop yet: the remaining command
  files (including thread round counting in HM-13 and the wake-up cap in HM-7) and the wiring of the
  node are separate units
  ([units](../../planning/steps/parallel-plan-units.md), section 2).
- The jobs of a group have no bound: a client that sends commands faster than its group applies
  them makes the list grow ([issues](../../issues.md)).
- A `peer.Connection` takes every frame at once and keeps it in its own buffer, so the limit of
  1,024 waiting frames never meets a slow browser ([issues](../../issues.md)).
- Inbox items are stored and sent unsigned, in the seam's form, until the inbox unit is built.
- A device of a member of another group on the same home is answered `unknown_device`, not
  `forbidden` ([issues](../../issues.md)).
- A device held by the logs of two people is read as the device of the one with the lower user id
  ([issues](../../issues.md)).
- `Item` is the home's own type. The signed `item` payload on the wire is not frozen yet
  ([conventions](../../planning/contracts/conventions.md), section 5).

## Verification and sources

`go test ./internal/home` covers every statement of the seam through its exported entries, with
fakes of `Receipts`, `Disk` and `Write` in the test files and one test on an in-memory store.
`features/home.feature` holds acceptance tests A2.3 and A2.4
([acceptance](../../planning/steps/step-1-acceptance.md)); its steps apply a command of their own
to a store in memory, because no command file exists yet. The choices behind the seam are in
[the decision record](../../docs/decisions/implemented/architecture/2026-11-20-home-command-seam.md).

The run loop is covered through its entries by `run_test.go`, `run_push_test.go` and
`run_disk_test.go`, on a store in memory with clients that keep, hold or refuse their frames, and
`run_internal_test.go` pins its arithmetic: where a frame is cut, when an item is due, how many
values a queue takes. `features/home_run.feature` holds the home half of acceptance test A2.1 and
a restart, with commands of its own. The choices behind the loop are in
[its decision record](../../docs/decisions/implemented/architecture/2026-11-21-home-run-loop.md).

`machine_test.go` has a case for every row of the task machine, for every rule of its section 2
and for the refusals, and covers every statement of `machine.go`. `features/home_machine.feature`
holds acceptance test A5.18 at step `merge` and the two points of the third review, a card decided
while a task is paused and a reviewer named while it is paused.

`cmd_group_test.go` and `cmd_member_test.go` send `group/create`, `group/set`, `member/add_ai`,
`member/set` and `member/remove` through `Commands.Apply` with fakes of `Receipts` and `Disk`,
covering every refusal, boundary and secretary transition. `features/home_group.feature` holds the
home command part of A2.1 and the member-removal part of A5.18.

`cmd_brief_test.go` sends every command of the briefs through `Commands.Apply` with fakes of
`Receipts` and `Disk`, with a case for every refusal and branch. `features/home_brief.feature`
holds acceptance tests A5.2, A5.6 and A5.15 for the home: its steps send the commands through
`Commands.Apply` with a `Disk` in memory, and move the created tasks through `Step` where the
commands of `task assign` and `task merged` are other units' (HM-9, HM-12).

`cmd_task_test.go` sends `task` `start`, `branch`, `assign`, `pause`, `resume`, `cancel`, `fail`
and `preempted` through `Commands.Apply` with fakes of `Receipts` and `Disk`, pinning every refusal,
card transition, retirement filter, thread-feedback page boundary and system message, and
`cmd_task_internal_test.go` pins `tskSender`. `features/home_task.feature` holds acceptance tests
A5.5, A5.7 (home half), A5.15 (`task fail` twice) and A5.18 (home commands) through `Commands.Apply`.

`cmd_post_test.go` and `cmd_note_test.go` cover every branch and boundary of the four `post` kinds,
`note` and `forget` through `Commands.Apply`, including the 200-character UTF-8 thread preview, the
shared `mentions` rule and `UnexpectedNulls` on the rows written. `features/home_post.feature` holds
acceptance tests A2.1 (message and item push across connected browsers and a host) and A5.14
(retention keeping notes and review messages, and `forget` replacing them with `gone` markers) on
`StartLoop` over an in-memory store.

`ev_device_test.go` covers every branch and boundary of `Events["device"]`, including ignored event
types, empty and non-owner slices, version boundaries and online/offline `GroupsChanged` transitions.

`sync_test.go` covers `Loop.Sync` against an in-memory and file-backed store: wire frame shapes,
cold content deduplication and `seq` order, exact boundaries of `since`, `purged` and the 1,000-row
delta limit, 60 KiB batching, `state_only`, concurrent writes while the first read is in flight,
refusals, detached connections and store failures; `run_internal_test.go` covers `runReader` and
`runErr`. `features/home_sync.feature` holds acceptance test A2.2 (delta, 1,500-row reset with
newest 50 and an older pending card, purge-mark reset, `state_only`) and the first-read cut of A2.7.
