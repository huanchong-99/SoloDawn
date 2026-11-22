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
no connection, keeps no clock and starts no transaction: it is given an envelope, a time, and the
reads and receipts of a transaction, and returns what to write.

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
| answer with result fields | `Result.Answer.Num`, `.Passed`, `.Round`, `.ID`, `.Replaced`, and for `invite`, `Result.Link` and `Result.Expires` |
| create or replay an invitation | `Result.InviteBy` (the inviting admin's member id) with `LoopConfig.Invite` (`Inviter`) |
| handle a joining connection | `c.Joining`, attached with `conn.SetJoining(fn)` on `Loop`; `Result.InviteUsed` marks consumption |
| admit or remove a person, or change their role | `cmd_people.go`: `join`, human `member/set` and `member/remove` |
| check another group's membership in a transaction | `Disk.OtherMember(user)` excludes the current group |
| refuse | `&Refusal{Code: "conflict", Msg: "…"}`, with a code of [datachannel](../../planning/contracts/datachannel.md) section 9 |
| read a message, a note, a thread, the pending cards, the queued items | `c.Disk` in a command, the `Disk` argument of a timer or a handler |
| add a rule that the clock sets off | a function `func(s *State, d Disk, now int64) Result` and `Timers.Add(name, f)` in its file's `init` |
| add a rule for an event outside the commands | a function `func(s *State, d Disk, now int64, event any) Result` and `Events.Add(name, f)` in its file's `init` |
| take a `cmd` from a connection | `Commands.Apply(s, disk, tx, env, now)` inside the command's transaction, then `s.Commit(result)` once it is committed, and `result.Frame(re)` for the answer |
| run a timer or a handler | `s.Seal(f(s, disk, now, …))` inside a transaction, then write and `s.Commit(result)` as for a command |
| finish any result | after `s.Commit(result)`, every hook of `After`, on the goroutine of the group |
| know whether a device may use the home surface of a group | `s.Serves(device)` |
| run the home of a node | `StartLoop(LoopConfig{…})` with `NodeTables()`; `Close` before the store is closed |
| connect a client | `loop.Attach(device, client)` returns its `*Conn`; `loop.Detach(conn)`; `conn.Gone()` |
| hand in a `cmd` frame | `loop.Cmd(conn, re, env)`; the answer is sent to the connection |
| answer a `hello` | `loop.Welcome(conn, build)`: `build` makes the `welcome` frame from the groups of the peer |
| read a group and subscribe (`sync`) | `loop.Sync(conn, req)`; queues `reset` when needed, `rows` and `synced`, then pushes changes after the cut |
| answer a `fetch` frame | `loop.Fetch(conn, req)`; the answer is sent to the connection |
| send a host its inbox items, on every `hello` | `loop.Inbox(conn)` |
| record a host's `ack` | `loop.Ack(conn, req)` |
| run the timers by hand, or tell the home of an event | `loop.Tick(now)`, `loop.Event(name, event)` |
| expire old store data in a transaction | `Retain(tx, now)` |
| mark a disconnected host's AIs offline | `HostOfflineEvent`, fired by `Loop` when a device's last `Inbox` connection ends |
| move a task | `Step(task, event)`: one task row and one `Event` in, the row and its `Effects` out ("The task machine" below) |
| push to a client | the `Client` interface |

Joining an existing person writes no rows and does not consume the invitation. Removing a
person retires their AIs and group devices and drops the group's in-memory device log. Only the
last local membership deletes the node-wide device log and push subscriptions.

## Contract

**Keys.** The table holds one function per key: the payload's `t`, followed by `/` and its `op` for
`group`, `member` and `task`, its `kind` for `post`, its `action` for `act`. `note`, `forget`,
`jump` and `push` are keys by themselves. The `/` of a key is made by the seam and by nothing in
a payload: a `t`, or an `op`, `kind` or `action`, that holds a `/` names no command, and neither
does one of those five types without its field. `Add` panics when a key is given twice.

**Order of checks.** `Apply` answers with the first check that fails
([datachannel](../../planning/contracts/datachannel.md), section 5):

1. the envelope checks of [envelope](../../planning/contracts/envelope.md) section 3, with their
   own codes; a device is known when a log in `State.Logs` holds it as valid or when the connection
   carries a non-nil `Joining` ([people](../../planning/contracts/step-2/people.md), section 3),
   and a `to` must be `State.Home`;
2. `g` is `State.ID`, else `bad_request`;
3. the receipt: a command answered before gets that outcome and nothing runs, except a succeeded
   `invite`, which asks `Inviter.ReplayInvite` and answers with the original `link` and `expires`
   while the invitation is open, or `conflict` otherwise;
4. the payload names a key, and the key is in the table, else `bad_request`;
5. on a joining connection (`c.Joining != nil`), only `join` with `c.Joining.Group == State.ID` is
   accepted (`forbidden` otherwise), then `State.Group.ID` must exist (`not_found` otherwise), `as`
   must be absent (`bad_request` for non-text or empty, `forbidden` when present), and `c.Who` gets
   `Device` plus `User` and `Human` when the signer is valid in `c.Joining.Log`; on an ordinary
   connection, the group exists (`not_found` otherwise; only `group/create` reaches its function
   without one);
6. `as`, when there, is a text that is not empty, else `bad_request`, and names an AI whose
   `host` is the signer, else `forbidden`;
7. the signer's person is a member of the group, or, for `group/create` on a group that does not
   exist, this node's owner, else `forbidden`;
8. the function;
9. no row of its result is above the size limit of a row, else `bad_request`.

Checks 1 and 2 leave no receipt. From check 4 on, the outcome is written as the receipt, a refusal
included, with the command's own `ts`. For an accepted `invite` (`Result.InviteBy` not empty), the
home calls `Inviter.Invite(group, by, signer, nonce)` (on a `Loop`, on the group's goroutine outside
`Store.Write` before writing the `{ok: true}` receipt, so an `Invite` error leaves no receipt and
never deadlocks `Store.Write`), and carries `Link` and `Expires` on `Result` without storing them in
the receipt. On a joining connection, when an accepted command commits on `Loop`,
`Joining.Admit(res.InviteUsed)` runs before the `ok` answer frame is queued. A repeated refusal
is answered with its code and a fixed `msg`, because a receipt holds no sentence.

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

**Items.** An `Item` is an inbox item before the home numbers it: the AI, the `why`, and the fields
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
`Loop` with its entries, `LoopConfig`, `StartLoop`, `Tables`, `NodeTables` and `Conn`. The inbox
takes the names that start with `inbox`, its tests too, and its test functions start with
`TestInbox`. Groups and members take the names that start with `grp` and `mem`, their tests too,
and their test functions start with `TestGroup` and `TestMember`. The briefs take the
names that start with `brief`, their tests too, and their test functions start with `TestBrief`.
The task commands (`cmd_task.go`) take the names that start with `tsk`, their tests too, and their
test functions start with `TestTaskCmd`. The posts of HM-6 take the names that start with `post`,
their tests too, and their test functions start with `TestPost`; the notes and `forget` take the
names that start with `note`, their tests too, and their test functions start with `TestNote`;
the knowledge base (`kb.go`) takes the names that start with `kb`, and its test functions start
with `TestKB`.
The cards take the names that start with `card`, their tests too, and their test functions start
with `TestCard`. Device-log events (`ev_device.go`) take the names that start with `dev`, and their
tests start with `TestDevice` and `dev`. Sync takes the names that start with `syn`, its tests too,
and its test functions start with `TestSync`; it exports `Loop.Sync`. `fetch.go` exports `Fetch`
and `Loop.Fetch`, takes the names that start with `fet`, its tests too, and its test functions
start with `TestFetch`.
The checks and reviews (`cmd_review.go`) take the names that start with `rev`, their tests too, and
their test functions start with `TestReviewCmd`. The merges and summaries (`cmd_merge.go`) take the
names that start with `mrg`, their tests too, and their test functions start with `TestMerge`.
Host events (`cmd_hostev.go`) take the names that start with `hev`, their tests too, and their test
functions start with `TestHostEv`; the file exports `HostOfflineEvent`.
Group pause and resume (`cmd_pause.go`) take the names that start with `pau`, their tests too, and
their test functions start with `TestPause`. The calls (`cmd_call.go`) take the names that start
with `cal`, their tests too, and their test functions start with `TestCall`. Thread rounds, queue jumps and the quiet-task timer (`rounds.go`) take the names that
start with `rnd`, their tests too, and their test functions start with `TestRounds`. Retention
(`retention.go`) takes the names that start with `ret`, exports `Retain`, and its test functions
start with `TestRetention`.
The step-2 people seam (`people_seam.go`) takes the unexported names that start with `seam` and
`keyInvite` / `keyJoin` (leaving `ppl` for `cmd_people.go`), and exports `Joining`, `InviteLink`,
`Inviter` and `(*Conn).SetJoining`. Web Push (`webpush.go`) takes the names that start with
`wpush`, its tests too, and its test functions start with `TestWebPush`.
Style presets (`preset.go`) take the names that start with `preset`, their tests too, and their test
functions start with `TestPreset`.

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

**Inbox items.** An item is stored in the transaction of its result as a plain wire `proto.Item`
JSON in `inbox.env`, for the host of its AI as that result leaves the member rows; an item for a
member that is no AI fails the transaction. `Inbox(conn)` makes a connection the taker of the
items of its device: it is sent every item that is due now, also those it was sent before, and
from then on each item when it is queued or when its `NotBefore` has passed, which the loop looks
at on a tick. Due items for one connection are sent in `proto.Inbox` frames packed up to 61,376
bytes (`runRoom`), in queue order, each carrying live copies of the `msg` and `task` rows the item
names when sent ([datachannel](../../planning/contracts/datachannel.md), section 6). `Ack(conn, req)` deletes `req.Item` in `req.Group` when `conn`'s device is the item's host. Each delivery reads the queue from the store, so an
item that was acknowledged or retired is not sent again. A queue that cannot be read is reported
and read again on the next tick. An item that cannot be made into a frame is reported and left out,
once for each connection until its next `Inbox`, and the other items are sent.

**Fetch.** `Fetch(conn, req)` answers a `fetch` request
([datachannel](../../planning/contracts/datachannel.md), section 4): a page by `kind` (`note`,
`preset`, `kb` or `review`) when `Kind` is set, one live row when `Row` or `RowID` is set, or a page of a task's
thread or of the main line (`Thread` nil) otherwise. Paged reads require `Limit` in 1..100 and
`Before >= 0` (`bad_request` otherwise). With `kind: kb`, a non-nil `Query` turns the page into a search: it requires `Before` 0 and a non-blank query (`bad_request` otherwise) and returns the matching documents best first (`kbSearchOK`, `store.SearchKB`). On the goroutine of the group (`ask`), it checks that the
connection is still seated, that the group exists (`not_found` otherwise) and serves the
connection's device (`forbidden` otherwise), opens and closes a `store.Snapshot`, packs page rows
with `runBatches` within the 61,440-byte `ctl` limit
([conventions](../../planning/contracts/conventions.md), section 4) without `gone` markers, and
queues the `fetched` or error answer on `conn`.

**The wake-up cap.** `inboxPace` enforces `limits.wakes_per_hour` before items are written, and
`State.Wakes[ai]` holds one time for each non-`merge` item queued for that AI: when the item is
due, which is the home's time when its result was committed, or its `NotBefore` when that is
later. Once an AI has `limits.wakes_per_hour` due times after `now - 3600000`, each further item
gets `NotBefore` at least one hour after the oldest of the last `wakes_per_hour` times. A `merge`
item is for the assignee's host rather than an AI session, so it neither waits on nor counts
toward `wakes_per_hour`. Sending an item again, on a hello after a reconnect or to a second
connection of its host, counts nothing, and so does a host that was not connected when the item
became due. Retiring or acknowledging an item does not take its time back. The count is in memory
and starts again with the loop, as the contract says; items sent again after a restart count
nothing.

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
(`Done()`) and one that the node detaches are forgotten in the same way and not closed. When the
last connection that took inbox items (`Inbox`) for a device ends, the loop runs `HostOfflineEvent`
for that device. A client's `Send` must return once the client is done or closed.

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
   `loop.Sync(conn, req)` for `sync`; `loop.Fetch(conn, req)` for `fetch`; and `loop.Ack(conn, req)` for `ack`.
3. The loop calls `client.Send(frame)` for every answer and push, one at a time, in order. The
   answer to a command is a frame like the others and comes after the frames queued before it.
   So a `Send` must not wait for something that the sender of a command holds while it waits for
   its answer.
4. `loop.Detach(conn)`, or the client's `Done()`. `conn.Gone()` tells the node that the loop
   gave a connection up; the node need not detach it then.

**Who may call what.** `Cmd`, `Ack`, `Detach`, `Welcome`, `Inbox`, `Tick`, `Event` and `Groups` never
wait for a group and may be called from anywhere. The `build` of a `Welcome` runs under the lock
of the loop: it only puts the frame together from the list of groups and from values the node
worked out before it called `Welcome`, such as `time` and `addrs`. It takes no lock, waits for
nothing and calls nothing, the loop least of all; a `build` that waits for a lock of the node
which another goroutine holds while it calls `Cmd` waits for ever. The loop calls `LoopConfig.Logs`, `Now`
and `Report` on the goroutines of its groups, and `Logs` and `Now` also on the caller of `Cmd`:
none of the three may wait for a lock that is held around a call of `Attach`, `Sync`, `Fetch`,
`Close` or `Cmd`, and none may call those. `Attach`, `Sync`, `Fetch` and `Close` wait for the
groups: they must not be called on the goroutine of a group, which is where a hook runs, or on the
store's writer, which is where the function of a command, a timer, a handler, a `Disk` and a
`Write.Put` run. A client's `Send` runs on the goroutine of its connection and may call every
entry except `Close`. `Close` waits for every connection's `Send` in progress, and for `Close` of
a given-up client, so a `Send` must return once its connection is gone. A hook may start
transactions of the store; what runs on the store's writer may not. The
goroutine of a group waits for the store's writer and for nothing else, and what runs on the writer
takes no lock of the loop: so no lock of a client, of a host's session or of the node can close a
circle through the loop, as long as that code keeps to this paragraph.

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
  `TaskItem`, which the function completes into an `Item`: the task and the message its
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

`cmd_confirm.go` gates mention and call delivery by the AI owner's word
([step-2 confirmations](../../planning/contracts/step-2/confirm.md) sections 2–5).
Personal accounts require the owner; team and API-key accounts also accept their allow list.
Task participants receive thread messages without a new card. Otherwise each AI gets a pending
`direct` or `ask` card, on the main line and also in the request's thread. Only its owner may
confirm or deny it; a confirmation delivers once. A denied call becomes `denied` and gives its
caller a `reply` carrying the card, plus a secretary notice. Denied AI-authored direct requests
also notify the secretary. Human `post/confirm` requests create `watch` cards; confirming one
queues a host-only `watch` item, outside the wake cap. The confirmation timer expires cards when
the AI, watch requester or retained request disappears. Role confirmation and waits belong to
T2-2b. Approval decisions require both the card's approvers and the AI row's current approvers.

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
  message. Then the secretary gets `confirmed`, naming the brief, after a `confirm` and after a
  `revise`; a group without a secretary gets none. A queued task gets its `assign` when the merge
  of its last dependency releases it (unit HM-12).

## Merges and summaries

`cmd_merge.go` holds `task` `merged` and `conflict` and `post` of kind `summary`
([commands](../../planning/contracts/commands.md) 1.3 and 1.5;
[messages](../../planning/contracts/messages.md) 2.5, 2.8 and 2.11;
[task-machine](../../planning/contracts/task-machine.md) rows 8 and 9), under the keys
`task/merged`, `task/conflict` and `post/summary`.

When `task/merged` steps queued dependents through `OnReleased` (in sorted task id order), each
task that becomes `active` gets `stats.started` set to `c.Now`
([rows](../../planning/contracts/rows.md), section 4) so summary minutes count active work rather
than dependency wait, and its `assign` item names its `task` message (`msg` equal to the task id).

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

`cmd_post.go` holds `post` of kinds `text`, `progress`, `question`, `report` and `handback`
([commands](../../planning/contracts/commands.md) 1.3;
[messages](../../planning/contracts/messages.md) 1, 2.7 and 2.10;
[rows](../../planning/contracts/rows.md) sections 4 and 5), under `post/<kind>`. `cmd_note.go`
holds `note` and `forget` ([commands](../../planning/contracts/commands.md) 1.6;
[rows](../../planning/contracts/rows.md) sections 5 and 6). Where the contracts are silent, they do
this:

- **Who and placement.** `text` is by a person or an AI; `progress`, `question`, `report` and
  `handback` are by an AI, else `forbidden`. `progress` and `report` belong in a thread (`thread`
  not `null`), else `bad_request`; `handback` belongs in a thread unless `as` is the group's
  secretary, else `bad_request`. None of the five is
  raised from a thread, and the row is written with `main` `false`.
- **Thread and mentions.** A non-null `thread` must name a task in `State.Tasks`, else `not_found`;
  for `report`, `as` must be that task's `assignee`, else `forbidden`. `(*State).postMentions`,
  shared with `cmd_brief.go`, checks the `mentions` rule of
  [commands](../../planning/contracts/commands.md) 1.3. `re` is kept as signed without looking up
  the answered message, which retention or `forget` may have removed.
- **Rows and items.** Every post writes its `MsgRow` with `ts` as signed and `body` encoded without
  HTML escaping; a post in a thread follows it with the updated `TaskRow` (`thread.count` + 1, and
  `thread.last` with `text` cut to at most 200 UTF-8 characters). A post never queues an item for
  its own author ([commands](../../planning/contracts/commands.md) 1.3): `text`, `progress` and
  `question` queue `mention` for each other mentioned AI in `mentions` order, a main-line `text`
  from a person that mentions no AI queues `mention` for `Group.Secretary` when set, and a
  `handback` posted in a thread queues `notice` (`code: "handback"`) for `Group.Secretary` when
  set.
- **`note` and `forget`.** `note` is by a person or an AI; a non-null `task` that names no task in
  `State.Tasks` is `not_found`, and the `NoteRow` gets `ts` as signed (`c.Timestamp`). `forget` is
  by an admin only (`forbidden`); `r` must be `note` or `msg` (`bad_request`); a missing or already
  forgotten row is `not_found`; a `msg` whose `kind` is not `review` is `conflict`; otherwise a
  `GoneRow` (`gone: true`) replaces it.

## Cards

`cmd_card.go` holds `post` of kinds `approval` and `decision`, and `act` `approve`, `deny` and
`choose` on them ([commands](../../planning/contracts/commands.md) 1.3 and 1.4;
[messages](../../planning/contracts/messages.md) 2.3, 2.4, 3.2 and 3.3;
[task machine](../../planning/contracts/task-machine.md) rows 17 to 20), under `post/approval`,
`post/decision`, `act/approve`, `act/deny` and `act/choose`. It reads a task's pending cards with
HM-9's `tskPending`, and checks mentions with HM-6's `postMentions`. Where the contracts are silent:

- **Post.** Both card kinds may be posted in the main line (`thread` `null`, with `body.task` `""`
  for `approval` and omitted for `decision`) or in a task's thread (`thread` equal to `body.task`).
  Duplicate mentions are `bad_request`; mentioning a non-member is `conflict`. For a card on a
  task, a missing task is `not_found`, and `Step` (`OnApproval` or `OnDecision`, with
  `BySecretary` for the secretary and `ByAI` for any other AI) runs before the duplicate-id check,
  so `forbidden` and `paused` precede `conflict` on a taken `n`. A card in a thread advances
  `task.Thread` (`count` + 1, `last.text` cut at 200 runes) and writes `[card, task]`.
- **Act.** Empty `arg` or `text` strings are rejected as `bad_request` like unknown fields; a
  missing message is `not_found`, and an action on the wrong message kind is `bad_request`. Cards
  naming a task run `Step` (`OnApprove` or `OnChoose`) with `ByPerson`, writing `[card, task]`
  only when `Step` changed the task row and `[card]` otherwise. Main-line cards update the card row
  and queue `decided` for the asking AI directly. `decided` items are queued only for AIs still in
  `State.AIs`.

## Device-log events

`ev_device.go` registers `Events["device"]`, which receives a `[]proto.DeviceRow` slice accepted by
the node for one person ([envelope](../../planning/contracts/envelope.md), section 4;
[rows](../../planning/contracts/rows.md), section 2). In step 1 it ignores users other than
`State.Owner`, appends rows whose `ver` exceeds the entries already held in `State.Logs[State.Owner]`,
and sets `Result.GroupsChanged` when an `Online` device's access (`State.Serves`) changes between
served and unserved.

## Checks and reviews

`cmd_review.go` holds `task` `checked`, `submitted` and `cloud`, and `post` of kind `review`
([commands](../../planning/contracts/commands.md) 1.3, 1.5 and 1.8;
[messages](../../planning/contracts/messages.md) 2.8, 2.9 and 2.11;
[task-machine](../../planning/contracts/task-machine.md) rows 4 to 7 and 14), under the keys
`task/checked`, `task/submitted`, `task/cloud` and `post/review`. Where the contracts are silent,
it does this:

- **Passed hosted checks.** `task/cloud` writes a `checks` message (`by` `"system"`, `stage`
  `"cloud"`, `ts` `c.Now`) under id `n` in the task's thread on passed runs as well as failed ones,
  so the `items` of a passing `task cloud` payload are recorded ([issues](../../issues.md)).
- **Derived `task_failed` id.** When `task/cloud` or `post/review` triggers row 14, `n` already
  names the `checks` or `review` row, so the `task_failed` row gets the first 16 bytes of the
  SHA-256 of `SoloDawn task failed <n>` in base64url ([issues](../../issues.md)), and mentions the
  brief's requester when still a member and the assignee's owner without duplicates.

## Host events

`cmd_hostev.go` holds `member` `status` and the `HostOfflineEvent` (`"host.offline"`) handler
([commands](../../planning/contracts/commands.md) 1.2 and 1.9;
[messages](../../planning/contracts/messages.md) 2.11;
[rows](../../planning/contracts/rows.md) 2 and 4;
[task-machine](../../planning/contracts/task-machine.md) 5), under `Commands` key `member/status`
and `Events` key `HostOfflineEvent`. Where the contracts are silent, it does this:

- **Payload and target.** When `event` is present, `code` must be one of the host codes
  (`turn_interrupted`, `rate_limited`, `login_invalid`, `session_replaced`; `cap_reached` is postponed
  with `settings.set`), else `bad_request`; an omitted or `""`
  `detail` is left out of the row body, and a non-empty `event.task` must exist in `State.Tasks`,
  else `not_found`. An `id` not in `State.AIs` is `not_found`; any signer for which
  `State.Hosts(c.Who, id)` is false (another device, or a command carrying `as`) is `forbidden`.
- **Rows and notices.** The AI's `AIMemberRow` gets `State`, `Task`, `Path`, `Usage` and
  `LastSeen = c.Now`; `Since` becomes `c.Now` when `State` changes and stays as it was when `State`
  is unchanged. When `event` is present, the home writes a `system` `MsgRow` (`id` `n`, `by`
  `"system"`, `ts` `c.Now`, `mentions` `[]`, and `ai` set to `id`) in the thread of `event.task`
  when set, advancing `task.Thread.Count` and `task.Thread.Last` (`text` `""`, `ts` `c.Now`) on the
  task row, or in the main line otherwise. For `login_invalid` and `rate_limited`, when `State.Group.Secretary` is not
  empty, it queues `Item{AI: s.Group.Secretary, Why: "notice", Msg: n, Code: code}`.
- **Closed host connection.** When `Loop.Detach` ends a connection that took inbox items
  (`Conn.taker`, set by `Loop.Inbox`) and no other taker connection of `device` remains on the
  loop, `Loop` fires `HostOfflineEvent` with `device`. For every AI in `State.AIs` in id order with
  `Host == device` and `State != "offline"`, the handler sets `State = "offline"`, `Task = ""` and
  `Since = now`, leaving `LastSeen`, `Path` and `Usage` unchanged.

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

## Group pause and resume

`cmd_pause.go` holds `group` `pause` and `resume`
([commands](../../planning/contracts/commands.md) 1.1;
[messages](../../planning/contracts/messages.md) 2.11;
[the task machine](../../planning/contracts/task-machine.md) row 24), under the keys `group/pause`
and `group/resume`:

- **Who.** Both require `s.Admin(c.Who)` (`forbidden` for a non-admin person or any command carrying
  `as`); they carry no fields beyond the group command header checked by the seam.
- **Idempotence.** Pausing a group whose `group.paused` is already set, or resuming one whose
  `group.paused` is `null`, returns `ok` without writing rows or queuing items.
- **Rows.** Otherwise `group/pause` sets `group.paused` to `{"by": c.Who.Human, "at": c.Now}` and
  writes main-line `system` message `group_paused` (`id` `n`, `ts` `c.Now`, `by` `c.Who.Human`)
  through `tskThreadSystem`. `group/resume` clears `group.paused` to `null` and writes main-line
  `system` message `group_resumed` with the same fields.
- **Waking tasks.** On `group/resume` of a paused group, `Step` runs with `OnGroupResume` (`ByHome`)
  over `State.Tasks` in sorted task-id order and completes items through `tskItemCtx`: `resume` goes
  to each `active` task with `wait` `null` before step `merge`, and `review` goes to each `active`
  task at step `review` that has a reviewer unless `c.Disk.Queued` already holds an unacknowledged
  `review` item for that reviewer, task and commit.

## Rounds, jumps and quiet tasks

`rounds.go` holds the `jump` command ([commands](../../planning/contracts/commands.md) 1.4, R-72),
the `quiet` timer and its `After` hook ([task-machine](../../planning/contracts/task-machine.md)
row 23), and the thread round transitions (rows 21 and 22, and section 4 "Held by the round limit";
[messages](../../planning/contracts/messages.md) 2.11):

- **`jump`.** Registered under `jump`. Refuses an AI signer (a non-empty `c.Who.AI`) with
  `forbidden`, a missing `msg` with `not_found`, and any `msg` whose `by` differs from `c.Who.Human`
  with `forbidden`.
  Scans `c.Disk.Queued()` from newest to oldest for an unacknowledged `mention` item whose message
  was written by `c.Who.Human` in the same thread (or in the main line when `msg` has no thread),
  retires that item and queues its replacement with `Jump: true` and `NotBefore: 0`, or answers
  `conflict` when none exists.
- **`quiet` timer.** Registered under `Timers["quiet"]` and `After["quiet"]`. When the group has a
  secretary, is not paused and has `limits.idle_notice_minutes > 0`, checks each task in sorted id
  order: if the task has had no thread message for at least `limits.idle_notice_minutes`
  (measured from `thread.last.ts`, else `stats.started` when non-zero, else `created`), has not already notified
  the secretary at that last-activity timestamp, and passes `Step` with `OnQuiet` (`ByHome`:
  `active`, `wait` `null`, `step` not `merge`), it queues one `notice` (`code: "quiet"`) for
  `group.secretary`. `After["quiet"]` records the task's last-activity timestamp in the group's
  `State` (in memory) once the transaction commits, so any new thread message starts a new quiet
  period ([issues](../../issues.md)).
- **Rounds.** `Table.Add` wraps every `post/*` entry (`rndWrap`), without depending on file
  initialisation order. On a thread `post` by an AI that mentions another AI (in `mentions` with
  `id` other than `msg.By`, or via a queued `mention` item such as `call`), steps `OnMention` (row
  21); when the round limit pauses the task, writes the main-line `system` message `rounds_limit`
  (`tskThreadSystem`) under `revDerivedID("SoloDawn rounds limit ", c.Nonce)`
  ([issues](../../issues.md)) and queues `notice` (`code: "rounds_limit"`) naming it for the
  secretary when the group has one. Thread `mention` items are queued at once with the updated
  `paused` task row; the host holds them while the task is `paused` or `failed` and delivers them
  again once a person's thread `post/text` (row 22, passing `tskPending` and deduplicating `IfNone`
  `review` items through `tskItemCtx`) or an admin's `task/resume` (row 13) activates the task.

## Calls

`cmd_call.go` holds `post` of kinds `call` and `reply`
([commands](../../planning/contracts/commands.md) 1.3;
[messages](../../planning/contracts/messages.md) 1, 2.6 and 3.4;
[ask](../../planning/flows/ask.md)), under the keys `post/call` and `post/reply`, sharing
`postCheck`, `postRows` and `postTaskID` with `cmd_post.go` and `cardItems` with `cmd_card.go`.
Where the contracts are silent, it does this:

- **Call.** A self-call (`body.to` equal to `as`) is `bad_request`, a non-null `thread` absent from
  `State.Tasks` is `not_found`, a `body.to` absent from `State.AIs` is `conflict`, and `body.depth`
  above `Group.Limits.AskDepth` is `limit` (A6.4). One `mention` item is queued for `body.to`, with
  `task` set when in a thread, even when `mentions` also names `body.to` or other AIs. In a thread
  the call counts a round through that item, and a reply counts one when it mentions another AI
  (`rndWrap`, see "Rounds"; [ask](../../planning/flows/ask.md) A.3 and D).
- **Reply.** A `re` `null` or a `thread` that differs from the call's `thread` is `bad_request`; a
  missing `re` message on `Disk` or a deleted task in `State.Tasks` is `not_found`. Elapsed `ms` on
  the answered call is `max(0, c.Timestamp - call.Timestamp)` so inter-host clock skew never
  produces a negative duration, followed by the reply `MsgRow` and, in a thread, the updated
  `TaskRow`. When `call.By` is still in `State.AIs`, one `reply` item is queued for it (A6.3).

## Retention

`retention.go` exports `Retain(tx, now)`, which reads `retention_days` from SQLite `setting`
(defaulting to `90` when absent) and calls `tx.Retain(time.UnixMilli(now), days)`
([storage](../../planning/contracts/storage.md) sections 2 and 4). The daily node schedule in
`internal/node` calls `Retain` once a day per node.

- **What remains.** In one transaction the store raises `grp.purged` to the highest `seq` among
  aged-out `gone` markers before deleting those markers, keeps `pending` cards, keeps `note` and
  `review` rows (`keep = 1`), and deletes older messages and final (`merged` and `cancelled`) task
  rows whose `mt` is before the cutoff.
- **In-memory tasks.** After retention deletes rows from the store, finished tasks stay in the
  in-memory state (`State.Tasks`) until restart ([issues](../../issues.md)). `grp.num` keeps the
  highest task number given out, so task numbers are not reused after restart.

## Web Push

`webpush.go` holds the `push` command, the `"push"` hook of `After` and the loop's startup and
shutdown hooks ([commands](../../planning/contracts/commands.md) 1.7;
[storage](../../planning/contracts/storage.md) section 1, `push_sub`):

- **Command.** A payload that decodes as `proto.PushOff` (`off: true`) deletes the signing device's
  row from `push_sub`; any other payload decodes as `proto.PushOn` (`bad_request` otherwise) and
  writes `push_sub` through `Result.Writes`.
- **Memory, startup and shutdown.** Subscriptions and the active `*Loop` are held per `State.Home`
  under a leaf mutex (`wpushMu`, which never calls `Loop`, `Store` or HTTP). `StartLoop` loads
  `push_sub` from `LoopConfig.Store` for `LoopConfig.Home` before starting group goroutines,
  `After["push"]` updates memory only after transaction commit so a rolled-back write leaves memory
  unchanged, and `Loop.Close` drains in-flight Web Push deliveries on `Loop.wg` before detaching.
- **Eligibility and delivery.** For each newly created `proto.MsgRow` (`Pos == Seq`), `After["push"]`
  extracts one reminder kind (`approval`, `decision`, `brief`, `confirm`, `done` from `summary`, or `mention`
  from any other message kind), filters for offline subscribed devices of concerned group members
  that remain valid in `State.Logs`, encrypts `proto.Reminder` JSON (without HTML escaping) via
  RFC 8291 (`webpush-go`), and sends each request in its own goroutine with a 5 s context timeout.
  Confirmation reminders go to `body.to`, name the AI in `who`, and omit `title`.
  A `404` or `410` response drops that subscription in memory and from `push_sub` if not replaced
  since.

## Known limitations

- The table of commands holds the groups, group pause and resume, members, briefs, the task
  commands of HM-9, the four kinds of `post` of HM-6, `note` and `forget`, the cards, the checks and
  reviews, the merges and summaries, the host events, `push`, the calls, and `jump` and thread rounds
  of HM-13, and no node starts a loop yet: the remaining command files and the wiring of the node
  are separate units
  ([units](../../planning/steps/parallel-plan-units.md), section 2).
- After retention deletes rows, finished tasks stay in the in-memory `State.Tasks` until restart
  ([issues](../../issues.md)).
- The jobs of a group have no bound: a client that sends commands faster than its group applies
  them makes the list grow ([issues](../../issues.md)).
- A `peer.Connection` takes every frame at once and keeps it in its own buffer, so the limit of
  1,024 waiting frames never meets a slow browser ([issues](../../issues.md)).
- A device of a member of another group on the same home is answered `unknown_device`, not
  `forbidden` ([issues](../../issues.md)).
- A device held by the logs of two people is read as the device of the one with the lower user id
  ([issues](../../issues.md)).

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

`inbox_test.go` pins every frozen `item-*.json` wire shape, `msg` and `task` row copies, multi-item
`inbox` frame batching to the `ctl` byte limit, `Loop.Ack`, and the `wakes_per_hour` hold and
release. `features/home_inbox.feature` holds the home half of A3.10 and the hourly wake-up cap.

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

`cmd_card_test.go` exercises `post/approval`, `post/decision`, `act/approve`, `act/deny` and
`act/choose` through `Commands.Apply` with fakes of `Receipts` and `Disk`, covering main-line and
task cards, wait recomputation and refusals, and `cmd_card_internal_test.go` covers `tskPending` on card rows,
`cardChoiceOK`, `cardPoster` and `cardItems`. `features/home_card.feature` holds the home half of
acceptance tests A4.1 and A4.2, decision priority and free-text override.

`cmd_merge_test.go` sends `task/merged`, `task/conflict` and `post/summary` through
`Commands.Apply` with fakes of `Receipts` and `Disk`, covering every refusal, boundary, card
expiration and dependent task release. `features/home_merge.feature` holds the home half of
acceptance tests A5.10 and A5.12 and dependent task release with `stats.started`.

`ev_device_test.go` covers every branch and boundary of `Events["device"]`, including ignored event
types, empty and non-owner slices, version boundaries and online/offline `GroupsChanged` transitions.

`cmd_review_test.go` sends `task/checked`, `task/submitted`, `task/cloud` and `post/review` through
`Commands.Apply` with fakes of `Receipts` and `Disk`, pinning every refusal, boundary, retirement,
card-wait recomputation, veto and row-14 stop, and checking `proto.UnexpectedNulls` on the rows
written. `features/home_review.feature` holds the home half of acceptance tests A5.3 and A5.4.

`cmd_hostev_test.go` checks `member/status` against the wire golden files, state and `since`
transitions, the four host event codes, `login_invalid` and `rate_limited` secretary notices, refusals, and
`HostOfflineEvent` both directly and when taker connections end on `StartLoop`.
`features/home_hostev.feature` holds the home half of acceptance test A3.6 together with the four
host event codes, secretary notices and host disconnection.

`sync_test.go` covers `Loop.Sync` against an in-memory and file-backed store: wire frame shapes,
cold content deduplication and `seq` order, exact boundaries of `since`, `purged` and the 1,000-row
delta limit, 60 KiB batching, `state_only`, concurrent writes while the first read is in flight,
refusals, detached connections and store failures; `run_internal_test.go` covers `runReader` and
`runErr`. `features/home_sync.feature` holds acceptance test A2.2 (delta, 1,500-row reset with
newest 50 and an older pending card, purge-mark reset, `state_only`) and the first-read cut of A2.7.

`trace_test.go` produces the two signed task transcripts of
[trace-task.md](../../planning/flows/trace-task.md), `internal/proto/testdata/trace-remote.json` and
`trace-local.json`: it sends their commands through `Commands.Apply` at each command's `ts`, writes
each result to a store in memory, commits it, and records the answer, the `rows` frame and the
`inbox` frames that `runLotOf` makes, acknowledging each item. It fails when a file differs from
that; `go test ./internal/home -run TestTaskTraces -update` writes them.

`fetch_test.go` exercises `Loop.Fetch` across thread pages, `note` and `review` pages, single-row
reads, exact 61,440-byte boundaries, page bounds checks, group access checks, detached connections
and store failures. `features/home_fetch.feature` holds the scroll-back scenarios of A2.2 and the
deleted-row scenario of A2.7.

`cmd_pause_test.go` covers `group/pause` and `group/resume` through `Commands.Apply`: the admin
check, both repeats, the rows, and row 24 over tasks with and without waits, at step `merge`, and
with a review already queued. `features/home_pause.feature` holds the home half of acceptance test
A4.7 on `StartLoop` over an in-memory store.

`rounds_test.go` covers `jump`, the `quiet` timer, `After["quiet"]`, and rows 21 and 22 through
`Commands.Apply`, `Table.Add` and `StartLoop`, including delivery after a rounds-paused task fails
and is resumed and wake-up cap ageing. `features/home_rounds.feature` holds acceptance tests A5.8,
A5.11 (home half) and A5.16 over `StartLoop`.

`cmd_call_test.go` sends `post/call` and `post/reply` through `Commands.Apply` with fakes of
`Receipts` and `Disk`, checking the golden shapes, `UnexpectedNulls`, the 200-character preview,
elapsed-time clamping and every refusal boundary. `features/home_call.feature` holds the
home half of acceptance tests A6.3 and A6.4.

`retention_test.go` covers `Retain`: default and configured `retention_days`, store error
propagation, protected rows, and the purge mark with `Loop.Sync` reset.
`features/home_retention.feature` holds acceptance test A2.5 and the retention half of A5.14.

`people_seam_internal_test.go` covers joining dispatch. `people_seam_test.go` covers
`LoopConfig.Invite` (including `Store.Write` re-entrancy freedom on `Invite` and `ReplayInvite`),
and `(*Conn).SetJoining` admission callbacks.
`webpush_test.go` covers `push`, `After["push"]`, `StartLoop` subscription loading and `Close`
drain, RFC 8291 decryption, per-browser eligibility and revocation, and `404`/`410` eviction in
memory and `push_sub`. `features/home_webpush.feature` holds the backend half of acceptance test
A8.3 against a local HTTP push endpoint, verifying that wire payloads are ciphertext only and
decrypted reminders hold neither message text nor commands.
