# Contract (step 2): moving a group

How a group moves to another home (R-134, `risks.md` Q-12). Copies and
restoring one are in `contracts/step-2/copy.md`, which rests on this file.
Together they settle design point D-11 of `steps/parallel-plan-step-2-3.md`
and what its review left open: receipts travel in the snapshot, and clients
follow a signed redirect. The home stays the single writer
(`architecture.md` section 7); nothing is replicated live. A group moves
only to another node of its owner, so the role `owner` never changes
(`contracts/step-2/people.md` section 1). No step 1 contract changes here;
section 8 lists what is folded where. Actors: P, the owner of the group;
B, a browser of P; H1, the home; H2, another node of P.

## 1. What travels

A **snapshot** is the group as its home stores it at one cut S, read from
the view that `sync` takes (`contracts/storage.md` section 1).

| Travels in the snapshot | Stays behind |
| --- | --- |
| every `row` of the group, deletion markers included, with its columns | push subscriptions: a browser sends `push` again (7.1) |
| the counters of `grp`: `seq`, `num`, `purged` | open invitations: they end when the move starts |
| the group's inbox items | the hourly wake-up counts, which start again as after a restart |
| the receipts of the group's commands (`contracts/commands.md`, "Once only") | all a host keeps: clones, sessions, audit, accepted hashes, watch grants |
| the acceptance marks of `contracts/step-2/checks-forge.md` section 4; the copy schedule (`copy.md` section 2) | device logs: the new home reads them from the `device` rows |

## 2. The redirect and the epoch

The group row gains `epoch`, an integer: 0 at `group create`, and the
`epoch` of the redirect by which a home installed the group (section 6).
A **redirect** is the owner's statement that the group has a new home. B
signs it, with an `epoch` above the group row's and above every epoch B
holds for the group, and carries it in the command of a move (section 4)
or of a restore (`copy.md` section 4).

```json
{"t":"redirect","n":"…","ts":1763600000000,"g":"Qm1…","from":"<device id>","home":"<device id>","epoch":3}
```

`from` is the home the group leaves, `home` the new one; `seq`, optional,
is in a restore only: the copy's `seq`. A receiver accepts a redirect R for
group g when all of this holds:

1. R passes check 1 of `contracts/envelope.md` section 3 and no other: like
   a log entry it is addressed to nobody and read long after it was signed.
   `R.p`, read by `contracts/conventions.md` section 2, has `t`
   `"redirect"`, `g` = g, two different device ids as `from` and `home`,
   and an integer `epoch` of at least 1.
2. In the log of the group's owner, R's signer is a currently valid device
   of kind `browser` (the kind of its latest `add`), `home` a currently
   valid device of kind `node`, and `from` is or was a device: valid,
   revoked, or with its `add` dropped by the fork rule (`authority.md` 7).
   That log is H1's and H2's own owner's; for a client, that of the person
   in whose log the node it reads the group from is valid, or of the owner
   it searches in 7.2.
3. `epoch` is above every epoch the receiver holds for g: from its group
   row, from redirects it accepted or installed, from a copy it holds.
4. For a client: R came from the node it reads g from, with that node as
   `from`; or, in 7.2, from a node of the owner's log, with it as `home`.

Checks 2 and 4 pin the person: the client picks the log from what it holds,
never from R, and takes R only from a device of it that is the group's home
before or after. So only a browser of the owner redirects, never a home. A
redirect that fails changes nothing: the client stays where it is (A9.29).

## 3. The snapshot

JSON lines: one record per line, a JSON object with `t` (its name) and the
fields below, then `\n`, in the order of the table, and nothing after.

| `t` | Fields | How many |
| --- | --- | --- |
| `head` | `g`; `kind` (`move`, `copy`); `from` (the home); `at` (time of the cut); `epoch`, `seq`, `num`, `purged` (at S); `copy`: the schedule `{"node":"…","minutes":60}` (`copy.md` section 2), or `null` | one |
| `row` | the columns of the `row` table but `g`: `r`, `id`, `seq`, `pos`, `ts`, `mt`, `thread` (or `null`), `in_main`, `pending`, `final`, `keep`, `gone`, `data` | one per row, in `seq` order |
| `item` | the columns of `inbox` but `g`: `item` (its `id`), `ai`, `host`, `why`, `msg`, `task`, `commit` (each of these three or `null`), `env`, `cause` (or `null`), `not_before`, `created` | one per item, in `item` order |
| `receipt` | `k`, `n`, `ts`, `answer` | one per receipt of the group |
| `mark` | `task` | one per marked task |
| `seal` | `env` | one, last |

- `data`, `env`, `cause` and `answer` are the stored JSON, embedded as a
  value, not a string; a reader keeps its bytes as they are in the line
  (in Go, `json.RawMessage`), so the new home stores what the old one held.
- `receipt` gains a column `g`, the group of a home command (`NULL` for a
  host command), so that a home can pick a group's receipts.
- `seal.env` is signed by `from`. Its payload has `t` `"snapshot"`, `n`,
  `ts`, the head's `g`, `kind`, `epoch` and `seq`, `hash`, and in a move
  `redirect`, the redirect's envelope. `hash` is the SHA-256 of the bytes
  of every line before the seal, each with its `\n`, in base64url.
- The seal line is written as Go's `encoding/json` writes the record, HTML
  escaping off: `{"t":"seal","env":{"p":…,"k":…,"s":…}}`, then `\n`.

**Verifying**, by the receiver, and again before a restore:

1. The lines before the seal are records of the table, in its order, read
   by `contracts/conventions.md` section 2; `head.kind` is the `kind` of
   the `snap` (section 5); no byte follows the seal's `\n`: else
   `bad_request`.
2. The seal line is the bytes that writing its record again gives; the
   seal passes check 1 of `contracts/envelope.md` section 3; its signer is
   `head.from`; its payload has the head's values and the `hash` of what
   was read: else `bad_sig`.
3. In a move, the seal's `redirect` passes checks 1 to 3 of section 2 for
   the receiver, and has `head.from` as `from`, the receiver as `home` and
   an `epoch` above `head.epoch`: else `bad_request`.
4. On receipt only (at a restore that device is revoked): `head.from` is
   the sender (section 5) and a currently valid node of the receiver's
   owner.

So a changed byte is refused wherever it is: in a record by the hash, in
the seal line by check 2, after it by check 1 (A9.29, A9.32).

## 4. Moving

A home command (`contracts/commands.md` 1.1), sent by B to H1:

```json
{"t":"group","op":"move","n":"…","ts":…,"g":"Qm1…","redirect":{…},"addrs":["wss://rv.example/rv"]}
```

`addrs` holds the signalling URLs at which B reaches H2, `[]` when it knows
none (`contracts/storage.md` section 5). H1 accepts it when the signer is a
device of the person with role `owner`, else `forbidden`, also for an admin
(A9.29); and when `redirect` passes checks 1 to 3 of section 2, has the
command's signer and `g`, names H1 as `from` and has no `seq`, else
`bad_request`.

**`moving`.** From step 1 until the move ends, a command for the group that
passed the checks before the receipt (`contracts/datachannel.md` section 5)
and has none is answered `moving`, and no receipt keeps it: H1 writes
nothing for the group. That holds for the `group move` sent again too; its
answer and receipt come when the move ends.

| # | H1 | H2 |
| --- | --- | --- |
| 1 | writes a move record, state `sending`. No `ack` is applied, no inbox item sent; retention and the home's timers skip the group. `sync` and `fetch` are answered as before | — |
| 2 | takes the cut S, reaches H2 (`addrs`, then the rendezvous) and sends the snapshot of kind `move` with `snap` (section 5) | verifies it (section 3) and stages it |
| 3 | — | answers `snap` with `ok` |
| 4 | sets the record to `moved`. **Commit**: from here H1 is not the home; it answers as in `sending` | — |
| 5 | sends `move.commit` | installs (section 6) and answers `ok` |
| 6 | answers the `group move` `ok`; pushes `groups` with the redirect to every connected client of the group, its own host role included (7.1); closes connections that no row of `contracts/datachannel.md` section 8 keeps; deletes the group's rows, counters, inbox, marks, schedule and move record, and `push_sub` rows as `people.md` 2.4 step 3. Receipts stay until retention, so a command H1 accepted gets its answer again | — |

**Abort**: H1 sends `move.abort` if it is connected to H2, drops the record,
writes again, and sends its unacknowledged items to the connected hosts of
the group, as at their `hello`.

| Event | Result |
| --- | --- |
| H2 is not reached within 2 minutes, the connection closes before step 4, or step 4 is not reached 60 minutes after step 1 | abort; the answer is `unreachable` |
| H1's own path to H2 is not `direct` | abort; the answer is `relay` |
| H2 refuses the snapshot | abort; the answer is H2's code (section 5) |
| H1 restarts in state `sending` | abort at start; the receipt holds `internal` |
| H1 restarts in state `moved`, or the connection closes after step 4 | H1 reaches H2 again (`flows/connect.md` C) and sends `move.commit`: `ok` leads to step 6, `not_found` to step 2 again, of the same rows. If H2 is revoked in P's log meanwhile, abort with the group row's `epoch` raised to the redirect's, so that the next redirect is above it; the answer is `unreachable` |
| H2 restarts, or its connection to H1 closes, before step 5 | H2 drops the staged snapshot: one lives only as long as the connection it came on |

So H2 is never the home without H1's commit, H1 never writes after it
unless H2 is revoked, and H2 installs one redirect once: at most one of the
two is the home, exactly one once both are reachable, and after a failure
before step 4 H2 holds no part of the group (A9.30).

## 5. From node to node

H1 uses its connection to H2, or opens one with `kind` `host`, which H2
accepts from its owner's device (`contracts/datachannel.md` section 8).
These `ctl` requests are taken only from a node in the receiver's owner's
log and have no receipt, like `host.info`: the redirect and the seal are
the authority.

`{"t":"snap","id":9,"g":"Qm1…","kind":"move"}`; the sender then opens a
channel (`contracts/datachannel.md` section 1) and writes the snapshot:

| Label | Created by | Type | Carries |
| --- | --- | --- | --- |
| `snap:<g>` | the sender of a `snap` | ordered, reliable, binary | the snapshot's bytes in order, in messages of at most 16 KiB whose bounds mean nothing; it ends with the seal's `\n`. From the sender only |

The receiver answers `snap` once it has verified the seal, or at once when
it refuses, then closes the channel; it closes one whose request it did not
get on this connection. `ok` stages a move, or stores a copy (`copy.md`).

| Code | When |
| --- | --- |
| `relay` | the receiver's own path to the sender is not `direct`: the relay carries small data only (R-132) |
| `forbidden` | the sender is not a node of the receiver's owner |
| `conflict` | the receiver is the home of g; for `move`, another move of g is staged here, or the seal's redirect was installed here before; for `copy`, `copy.md` section 3 |
| `bad_request` | checks 1 and 3 of section 3 |
| `bad_sig` | check 2 of section 3; the channel closed before the seal |

`{"t":"move.commit","id":10,"g":"Qm1…","n":"<the redirect's n>"}`: the
receiver installs what it staged for that redirect and answers `ok`; to a
redirect it installed before, home or not any more, `ok` with no effect (it
keeps their `n`); `not_found` only when it has neither staged nor installed
it. `move.abort`, the same fields, drops what it staged for it; always `ok`.

## 6. Installing a snapshot

At `move.commit`, and at a restore (`copy.md` section 4), H2 in one
transaction: stores the rows as recorded and the head's counters; applies
the entry of each `device` row (a marker has none) to `device_log`, per
person in the order of `ver`, as any entry it receives
(`contracts/envelope.md` section 4, and the fork rule of
`contracts/step-2/authority.md` section 7: a replaced entry is dropped, and
those after it are applied again up to the first that is refused, so the
rows of a version the log no longer has are markers); adds the receipts,
keeping one it has for the same `k` and `n`; stores the marks; queues each
item again, signed by itself, with the payload of its `env` but a new `n`,
`ts` and `item`, and `first`, the rest of the record unchanged; keeps the
redirect for its `groups` entries (7.1) and its `n`; in a move, keeps the
schedule of `head.copy` unless its `node` is H2; deletes its own copy of the
group; writes the group row with `home` = H2 and `epoch` = the redirect's,
and a `system` message `home_moved` in the main line. Then it is the home:
it pushes, accepts the devices of members and runs the group's timers.

**`first`**, optional in an item's payload (`contracts/datachannel.md`
section 6): `{"home":"<device id>","item":4411}`, the home and number under
which the item was first queued, set at its first move and kept after. A
host checks a re-signed item as any (`contracts/step-2/authority.md` section
3: `cause` is unchanged, `req` is taken when sent), and writes its audit row
`delivered` or `refused` under `first`, so an item it took from H1 is
acknowledged, not delivered again, when H2 sends it (A9.27). `ack` names
the new number. **`home_moved`**, a row for `contracts/messages.md` 2.11,
from the home: always present `from` and `home` (device ids), `by` (the
owner's member id) and `restored` (`true` after a restore).

## 7. What clients do

### 7.1 Following

An entry of `groups` and `welcome.groups` (`contracts/datachannel.md`
section 3) gains two optional fields, `redirect` and `addrs`. H1 pushes the
redirect at step 6 with the `addrs` of the `group move`; a home keeps the
redirect by which it got a group in every entry for it, for 7.2. A client
that accepts one (section 2) stops reading the group from `from`, records
the `epoch` and `home` as the group's home (a browser in its list of group
homes; a node in `homes`, its `session` rows and, for a team group, its
entry in `settings.teams`), connects to it (`addrs`, then the rendezvous)
and sends `sync`: with its cursor after a move, whose rows are those it
read (A9.28); from 0 after a restore, a redirect with `seq`.

- A host sends `hello`, gets the items again (section 6) and sends its AIs'
  status as when a home comes back (`flows/sync.md` H). It takes items for
  the group from the new home only: one signed by `from` is refused (A9.29).
  H1's own host role, which reached the home in process (`flows/sync.md`
  F), now reaches H2 as a remote home.
- A browser subscribed for reminders sends its `push` to the new home, and
  `home.add` with it to each of its owner's nodes it is connected to, which
  replaces their entry for the group (`people.md` section 7, point 2).
- A command answered `moving` was not applied: the client may send the same
  envelope to the new home after it follows, or to H1 once H1 writes again.
  An MCP tool returns the code as its error text.

### 7.2 Looking for a group

A client looks for a group when its home does not accept the connection, or
does not list the group in `welcome.groups`. It tries the other devices of
kind `node` in the log of the group's owner, in log order, once per round,
rounds with the backoff of `flows/connect.md` C, and follows the first
whose entry carries a redirect it accepts naming that node. Check 2 of
`contracts/envelope.md` section 3 gains the reason: a node of the owner of
the home of a group the receiver reads. So a node keeps in `device_log` the
log of the owner of each home it is a client of; a browser keeps, with each
group home it lists, the owner's user id and the ids of that person's nodes
as their log last showed them, and judges against the log the node sends
after `welcome`. In the browser, the owner moves a group from the group
info, to one of P's other nodes; copies and restoring: `copy.md` section 5.

## 8. Where this goes after step 1

| File | Gains |
| --- | --- |
| `contracts/commands.md` | 1.1: `move`; "Once only": `moving` leaves no receipt, receipts travel with a group |
| `contracts/datachannel.md` | 1: `snap`; 3: `redirect`, `addrs` in an entry of `groups` and `welcome`; 5: `moving` in the order of checks; 6: `first`; 7: `snap`, `move.commit`, `move.abort`; 9: `moving`, `unreachable` |
| `contracts/envelope.md`, `contracts/vectors.md` | section 3, check 2: the reasons of 7.1 and 7.2; the redirect, checked like a log entry; vectors of redirect cases for checks 1 to 3 of section 2 |
| `contracts/storage.md` | `receipt.g`; `grp` keeps the redirect and the `n` of those installed; a home table `move` (group, redirect, `addrs`, the command's `k` and `n`, state); the marks stored; `device_log` of home owners; audit under `first`; section 5: owner and nodes per group home |
| `contracts/rows.md`, `messages.md`, `local.md` | group `epoch`, `home` changes with a move; 2.11: `home_moved`; section 2: the staged snapshot |
| `contracts/mcp-tools.md` | the tool error `moving` |
| step 2 files | `people.md` section 7, point 2: `home.add` replaces the entry of its `g`; `watch-team.md` section 10: a node follows a team group in `settings.teams` |
| `flows/`, `architecture.md`, `risks.md` | a new `flows/moving.md` (sections 4, 7); `flows/sync.md` H: 7.2; architecture 3 and 5: node to node of one owner, copies; K-1, Q-12 |

P-4 freezes: `group move`; the `redirect` payload with `epoch`; `epoch` in
the group row; `redirect` and `addrs` in a `groups` entry; `snap`,
`move.commit`, `move.abort` and their answers, with the rule that
`snap.kind` is `head.kind`; the label `snap:<g>` and its framing; the
records `head`, `row`, `item`, `receipt`, `mark`, `seal`, the written form
of the seal line and the `snapshot` payload; `first`; the `home_moved`
body; the codes `moving` and `unreachable`; and the shapes of `copy.md`.

## 9. Limits; not verified

- If H2 is lost between H1's commit and its install, no node is the home
  until P revokes H2; a client that found H2 by 7.2 before that returns
  only with the next move or restore.
- A client and a revoked home that both missed the `revoke` meet and go
  on; what is written there stays on that node, stopped, once it gets the
  entry (`copy.md` section 6). A browser never shown a node P added later
  does not look there (7.2).
- Nothing here was run. `[unverified]` That 60 minutes moves a large group
  on a direct path, and that a home holds a read view open that long.
