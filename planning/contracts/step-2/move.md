# Contract (step 2): moving and copying a group

How a group moves to another home, and how a node keeps a copy of it for
safekeeping (R-134, R-135, `risks.md` Q-12). It settles design point D-11
of `steps/parallel-plan-step-2-3.md` and what its review left open:
receipts travel in the snapshot, and clients follow a signed redirect. The
home stays the single writer (`architecture.md` section 7); nothing is
replicated live. A group moves only to another node of its owner, so the
role `owner` never changes (`contracts/step-2/people.md` section 1). This
file changes no step 1 contract; it is folded into them later (section 10).
Actors: P, the owner of the group; B, a browser of P; H1, the home; H2,
another node of P; a client is a browser or a host of the group.

## 1. What travels

A **snapshot** is the group as its home stores it at one cut S, read from
the view that `sync` takes (`contracts/storage.md` section 1).

| Travels in the snapshot | Stays behind |
| --- | --- |
| every `row` of the group, deletion markers included, with its columns | push subscriptions: a browser sends `push` again (7.1) |
| the counters of `grp`: `seq`, `num`, `purged` | open invitations: they end when the move starts |
| the group's inbox items | the hourly wake-up counts, which start again as after a restart |
| the receipts of the group's commands (`contracts/commands.md`, "Once only") | all a host keeps: clones, sessions, audit, accepted hashes, watch grants |
| the acceptance marks of `contracts/step-2/checks-forge.md` section 4; in a move, the copy schedule (section 8) | device logs: the new home reads them from the `device` rows |

## 2. The redirect

The owner's statement that a group has a new home, which clients follow
(section 7). B signs it and carries it in the command of a move (section 4)
or of a restore (section 9).

```json
{"t":"redirect","n":"…","ts":1763600000000,"g":"Qm1…","from":"<device id>","home":"<device id>"}
```

`from` is the home the group leaves, `home` the new one; `seq`, optional,
is in a restore only: the copy's `seq`. A receiver accepts a redirect R
for group g when all of this holds:

1. R passes check 1 of `contracts/envelope.md` section 3 and no other: like
   a log entry, it is addressed to nobody and read long after it was
   signed. `R.p` is a JSON object, read by `contracts/conventions.md`
   section 2, with `t` `"redirect"`, `g` = g, and two different device ids
   as `from` and `home`.
2. In the log of the person in whose log `from` is or was a device, R's
   signer is a currently valid device of kind `browser`, `home` one of kind
   `node`. A device's kind is that of its latest `add` entry.
3. A client received it in one of the two ways of section 7.1.

One that fails changes nothing: the client goes on reading the group where
it did (A9.29). No node signs one, so no home can send clients elsewhere.

## 3. The snapshot

JSON lines: one record per line, a JSON object with `t` (its name) and the
fields below, then `\n`, in the order of the table.

| `t` | Fields | How many |
| --- | --- | --- |
| `head` | `g`; `kind` (`move`, `copy`); `from` (the home); `at` (time of the cut); `seq`, `num`, `purged` (the counters at S); `copy`, in a move the schedule `{"node":"…","minutes":60}` of section 8 or `null`, in a copy `null` | one |
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
- `seal.env` is signed by `from`, with the payload
  `{"t":"snapshot","n":"…","ts":…,"g":"Qm1…","seq":812,"hash":"…"}`;
  `hash` is the SHA-256 of the bytes of every line before the seal, each
  with its `\n`, in base64url (`contracts/envelope.md` section 1).

**Verifying**, when a snapshot is received and again before a restore:

1. The lines before the seal are records of the table, in its order, read
   by `contracts/conventions.md` section 2: else `bad_request`.
2. The seal passes check 1 of `contracts/envelope.md` section 3; its payload
   has `t` `"snapshot"`, the head's `g` and `seq`, and the `hash` of what
   was read; its signer is `head.from`: else `bad_sig`. One byte changed
   anywhere fails here (A9.29, A9.32).
3. On receipt only, as that device is revoked at a restore: `head.from` is
   the sender (section 5) and a currently valid node of the receiver's owner.

## 4. Moving

A home command (`contracts/commands.md` 1.1), sent by B to H1:

```json
{"t":"group","op":"move","n":"…","ts":…,"g":"Qm1…","redirect":{…},"addrs":["wss://rv.example/rv"]}
```

`addrs` holds the signalling URLs at which B reaches H2, `[]` when it knows
none (`contracts/storage.md` section 5). H1 accepts it when:

1. the signer is a device of the person with role `owner`: else
   `forbidden`, also for an admin (A9.29);
2. `redirect` passes checks 1 and 2 of section 2, has the command's signer
   and `g`, names H1 as `from` and has no `seq`: else `bad_request`;
3. the group is not moving: else `moving`.

| # | H1 | H2 |
| --- | --- | --- |
| 1 | writes a move record, state `sending`. From now it writes nothing for the group: commands for it are answered `moving`, no `ack` is applied, no inbox item is sent, retention and the home's timers skip it. `sync` and `fetch` are answered as before | — |
| 2 | takes the cut S, reaches H2 (`addrs`, then the rendezvous) and sends the snapshot of kind `move` with `snap` (section 5) | verifies it (section 3) and stages it |
| 3 | — | answers `snap` with `ok` |
| 4 | sets the record to `moved`. **Commit**: from here H1 is not the home | — |
| 5 | sends `move.commit` | installs (section 6) and answers `ok` |
| 6 | answers the `group move` `ok`; pushes `groups` with the redirect to every connected client of the group (7.1); closes connections that no row of `contracts/datachannel.md` section 8 keeps; deletes the group's rows, counters, inbox, marks, schedule and move record, and `push_sub` rows as `people.md` 2.4 step 3. Receipts stay until retention, so a command H1 accepted gets its answer again | — |

| Event | Result |
| --- | --- |
| the `group move` arrives again before the move ends | `moving`: it is answered, and its receipt written, when the move ends |
| H2 is not reached within 2 minutes, or the connection closes before step 4 | H1 drops the record and writes again; the answer is `home_unreachable` |
| H1's own path to H2 is not `direct` | as above; the answer is `relay` |
| H2 refuses the snapshot | as above; the answer is H2's code (section 5) |
| H1 restarts in state `sending` | H1 drops the record at start and writes again; the receipt holds `internal` |
| H1 restarts in state `moved`, or the connection closes after step 4 | H1 stays no home, reaches H2 again with the backoff of `flows/connect.md` C and sends `move.commit`; to `not_found` it sends the same rows again from step 2, since it wrote nothing |
| H2 restarts, or its connection to H1 closes, before step 5 | H2 drops the staged snapshot: one lives only as long as the connection it came on |

So H2 is never the home without H1's commit, and H1 never writes after it:
at most one of the two is the home, exactly one once both are reachable,
and after a failure before step 4 H2 holds no part of the group (A9.30).

## 5. From node to node

H1 uses its connection to H2, or opens one with `kind` `host`, which H2
accepts from its owner's device (`contracts/datachannel.md` section 8). Two
`ctl` requests, taken only from a node in the receiver's owner's log, with
no receipt, like `host.info`: the authority is the redirect and the seal.

`{"t":"snap","id":9,"g":"Qm1…","kind":"move","redirect":{…}}`, `redirect`
for `move` only; then the sender opens (`contracts/datachannel.md` 1):

| Label | Created by | Type | Carries |
| --- | --- | --- | --- |
| `snap:<g>` | the sender of a `snap` | ordered, reliable, binary | the snapshot's bytes in chunks of at most 16 KiB, from the sender only |

The receiver answers `snap` once it has verified the seal, or at once when
it refuses, then closes the channel; it closes one whose request it did not
get on this connection. `ok` stores a copy (section 8) or stages a move.

| Code | When |
| --- | --- |
| `relay` | the receiver's own path to the sender is not `direct`: the relay carries small data only (R-132) |
| `forbidden` | the sender is not a node of the receiver's owner |
| `conflict` | the receiver is the home of g; for `move`, another move of g is staged here |
| `bad_request` | `redirect` fails checks 1 and 2 of section 2, or its `from` is not the sender or its `home` not the receiver; check 1 of section 3 |
| `bad_sig` | check 2 of section 3; the channel closed before the seal |

`{"t":"move.commit","id":10,"g":"Qm1…","n":"<the redirect's n>"}`: the
receiver installs what it staged for that redirect, or is the home by it
already, and answers `ok`; `not_found` when it has nothing staged for it.

## 6. Installing a snapshot

At `move.commit` and at a restore (section 9), H2 in one transaction:
stores the rows as recorded and the head's counters; applies the `env` of
each `device` row to `device_log` (`contracts/envelope.md` section 4,
`contracts/step-2/authority.md`); adds the receipts, keeping one it has for
the same `k` and `n`; stores the marks; queues each item again, signed by
itself, with the payload of its `env` but a new `n`, `ts` and `item`, and
`first`, the rest of the record unchanged; keeps the redirect for its
`groups` entries (7.1) and, in a move, the schedule of `head.copy` unless
its `node` is H2; deletes its own copy of the group (section 8); writes the
group row with `home` = H2, and a `system` message `home_moved` in the main
line. Then it is the home: it pushes, accepts the devices of members and
runs the group's timers from the rows; its own AIs reach it in process
(`flows/sync.md` F).

**`first`**, optional in an item's payload (`contracts/datachannel.md`
section 6): `{"home":"<device id>","item":4411}`, the home and number under
which the item was first queued, set at its first move and kept after. A
host writes its audit row `delivered` or `refused` under `first` when the
item has one, so an item it delivered from H1 is acknowledged, not delivered
again, when H2 sends it (`contracts/storage.md` section 3; A9.27). `ack`
names the new number. **`home_moved`**, a row for `contracts/messages.md`
2.11, from the home: always present `from` and `home` (device ids), `by`
(the owner's member id) and `restored` (`true` after a restore).

## 7. What clients do

### 7.1 Following

An entry of `groups` and `welcome.groups` (`contracts/datachannel.md`
section 3) gains two optional fields, `redirect` and `addrs`. A client takes
a redirect from the node it reads the group from, whose entry names another
`home` (H1 at step 6, with the `addrs` of the `group move`), or from the
node it names while the client looks for the group (7.2): a home keeps the
redirect by which it got a group in every entry for it. A client that
accepts one (section 2) stops reading the group from `from`, records `home`
as its home (a browser in its list of group homes, a node in `homes` and
its `session` rows), connects to it (`addrs`, then the rendezvous) and
sends `sync`: with its cursor after a move, whose rows are those it read
(A9.28); from 0 after a restore, a redirect with `seq`.

- A host sends `hello`, gets the items again (section 6) and sends its AIs'
  status as when a home comes back (`flows/sync.md` H). It takes items for
  the group from the new home only: one signed by `from` is refused (A9.29).
- A browser subscribed for reminders sends its `push` to the new home, and
  `home.add` with it to each of its owner's nodes it is connected to, which
  replace their entry for the group (`people.md` section 7, point 2).
- A command answered `moving` was not applied: the client may send the same
  envelope to the new home after it follows, or to H1 once H1 writes again.
  An MCP tool returns the code as its error text.

### 7.2 Looking for a group

A client looks for a group when its home does not accept the connection, or
does not list the group in `welcome.groups`. It tries the other devices of
kind `node` in the log of the group's owner, in log order, once per round,
rounds with the backoff of `flows/connect.md` C, and follows the first whose
entry carries a redirect it accepts naming that node. Check 2 of
`contracts/envelope.md` section 3 gains the reason: a node of the owner of
the home of a group the receiver reads. So a node keeps in `device_log` the
log of the owner of each home it is a client of; a browser keeps, with each
group home it lists, the owner's user id and the ids of that person's nodes
as their log last showed them, and judges the redirect against the log the
node sends after `welcome`.

### 7.3 In the browser

Move: for the owner, in the group info, a choice of P's other nodes,
"moving", then the result. Copy: node, interval, last copy's time and any
error (`copies`). Restore: on a node holding a copy, only when the copy's
home is revoked and is still the group's home in the browser's list.

## 8. Copies

Host commands (`contracts/commands.md` section 2), by P as every one:

| `t` | Fields | Effect |
| --- | --- | --- |
| `copy.set` | `g`, `node`, `minutes` | on the home of g, else `not_found`. `node` is another valid node of P and `minutes` at least 1, or `node` is `""` to stop; else `bad_request`. Replaces the group's schedule |
| `copy.drop` | `g` | on a node that holds a copy of g, else `not_found`: deletes it |
| `copy.restore` | `g`, `redirect` | section 9 |

- The home keeps the schedules in the `setting` key `copies` and copies at
  once, then every `minutes`: a cut as `sync` takes it, writing on, sent as
  a snapshot of kind `copy` with `snap`. A run is skipped while another runs
  or the group moves, and when `seq` is unchanged since the last copy.
- `host.info` (`contracts/datachannel.md` section 7) gains a list `copies`:
  `{"g":"Qm1…","node":"<device id>","minutes":1,"last":1763600000000}`,
  `last` the `at` of the last copy stored or `null`, and `error`, optional,
  the code of the last run when it failed.
- The holder keeps one copy per group, the file `copies/<g>.snap` in its
  data directory, replaced only by a newer one that verified. The group is
  not homed there: no `welcome.groups` lists it, and `sync` and commands
  for it are refused (A9.31). Its `host.info` gains a list `held` of
  `{"g":"…","name":"Website","from":"…","seq":812,"at":…}`, one per file.

## 9. Restoring a copy

`copy.restore`, sent by B to H2, the holder, is checked in this order:

1. H2 holds a copy of g: else `not_found`.
2. `redirect` passes checks 1 and 2 of section 2, has the command's signer,
   and names the copy's `head.from`, H2 and the copy's `seq` as `from`,
   `home` and `seq`: else `bad_request`.
3. `from` is revoked in P's log: else `conflict`. A home that may still run
   is moved, not restored; revoking it makes every client leave it
   (`flows/pairing.md` D), so two homes never write the group.
4. H2 is not the home of g: else `conflict`.
5. The copy verifies (section 3): else `bad_sig` (A9.32).

H2 installs it (section 6) with `restored: true` and deletes the file;
clients find it by 7.2. What the old home wrote after the cut is lost; items
it acknowledged after the cut come again, and hosts drop them by `first`.

## 10. Where this goes after step 1

| Step 1 file | Gains |
| --- | --- |
| `contracts/commands.md` | 1.1: `move`; section 2: `copy.set`, `copy.drop`, `copy.restore`; "Once only": receipts travel with a group |
| `contracts/datachannel.md` | section 1: `snap`; 3: `redirect`, `addrs` in a `groups` entry; 6: `first`; 7: `snap`, `move.commit`, `copies`, `held`; 9: `moving` |
| `contracts/envelope.md` | section 3, check 2: the reasons of 7.1 and 7.2; the redirect, checked like a log entry |
| `contracts/storage.md` | `receipt.g`; `grp` keeps the redirect; a home table `move` (group, redirect, `addrs`, the command's `k` and `n`, state); the marks stored; `setting` key `copies`; `device_log` of home owners; audit under `first`; section 5: owner and nodes per group home |
| `contracts/local.md` | section 2: `copies/<g>.snap` and a staged snapshot |
| `contracts/messages.md`, `rows.md` | 2.11: `home_moved`; section 1: `home` changes with a move |
| `contracts/mcp-tools.md` | the tool error `moving` |
| `flows/`, `architecture.md`, `risks.md` | a new `flows/moving.md` (sections 4, 7); `flows/sync.md` H: 7.2; architecture 3 and 5: node to node of one owner, copies; K-1, Q-12 |

P-4 freezes: `group move`; the `redirect` payload; `redirect` and `addrs`
in a `groups` entry; `snap` and `move.commit` with their answers; the
records `head`, `row`, `item`, `receipt`, `mark`, `seal` and the `snapshot`
payload; `copy.set`, `copy.drop`, `copy.restore`; `copies` and `held` in
`host.info`; `first` in an item; the `home_moved` body; the code `moving`.

## 11. Not verified

Nothing here was run; `contracts/step-2/authority.md` did not exist yet.
`[unverified]` How long a large group takes to move on a direct path, and
that a home holds a read view open that long without harm. Stated limits:
if H2 is lost between H1's commit and its install, no node is the home and
P restores a copy; a browser that never saw a node P added later does not
look for the group there (7.2).
