# Flow: reading a group and sending commands

Contract: `contracts/datachannel.md`. Rule: R-134.

The home is the only writer of a group. Every row it stores carries `seq`,
the group's sequence number at the row's last change. Clients ask for "rows
after n" and then receive changes as they happen. Clients keep caches, never
authoritative copies. There is one way to read, used for the first read,
for a reconnect and for a move to a better path.

Actors: C, a client (browser or host); H, the group home.

## A. Reading

1. `ctl` is open and `hello`/`welcome` are done. `welcome` lists the groups C
   may read on this home.
2. C sends `sync {g, since}`, where `since` is the last sequence number C
   **applied** for that group, or 0 if it holds nothing.
3. H, in the order in which it writes, takes the group's current sequence
   number S, opens a read view of the database as it is at S, and
   subscribes this connection to every change after S.
4. H decides between a delta and a reset (section C). For a reset it first
   pushes `reset`, and C drops its cache for that group.
5. H pushes `rows`, read from that view:
   - delta: every row with `seq` above `since`, in order;
   - reset, or `since` = 0: every **state row** (the group row, members,
     device log entries, tasks), every message whose card is still pending,
     and the newest 50 main-line messages.
6. H answers `synced {g, seq: S}`.
7. From now on H pushes a `rows` frame to C for every change after S, in
   `seq` order, thread messages included.

Because the rows of step 5 come from the view at S and the subscription
starts after S, nothing written while H was sending is lost, hidden or sent
twice. C applies a row only if its `seq` is higher than the one it holds
for that row.

**C's cursor**, the `since` of its next `sync`: 0 at the start, and 0 again
from the moment a `reset` makes C drop its cache; S at the `synced` that
follows; after that the `seq` of each live row C applies. Pages from
`fetch` never move it. So a reset that breaks off half-way starts again
from nothing, and a delta that breaks off keeps the cursor it had.

A task row carries a summary of its thread (count, last message, AI round
counter), so the collapsed thread line needs no thread messages. Pending
cards are always sent, however old, so "waiting for you" is complete on a
cold start.

## B. Opening a thread or scrolling back

8. C sends `fetch {g, thread: <task id or null>, before: <pos>, limit}`.
   For the newest page `before` is 0; for the page before one it has, it is
   the lowest `pos` C received.
9. H answers with messages of that scope whose `pos` is lower, newest first,
   as many as fit one message up to `limit`, and `more: true|false`.

Messages are ordered by `pos`, the sequence number they were created with.
`seq` moves when a card is decided; `pos` does not, so a page boundary never
shifts. Notes and the ledger are read the same way (`kind`).

## C. Delta or reset

H sends a delta when all of these hold, otherwise a reset:

- `since` is not 0;
- at most 1000 rows have `seq` above `since`;
- `since` is not below the group's purge mark, the highest sequence number
  of a deletion marker that retention has already removed. Below it, C may
  hold a row whose deletion it can no longer be told about.

## D. Sending a command

10. C builds the payload, signs it into an envelope, and sends
    `cmd {id, env}`.
11. H checks the envelope, checks that this principal may do this
    (`contracts/commands.md`), and applies the command in one database
    transaction, together with its receipt. Each row it changes gets the
    next `seq`.
12. H answers `{re: id, ok: true}`, with result fields where the command
    has them, or `{re: id, err: {code, msg}}`.
13. H pushes the changed rows to every connected client of the group,
    including C. C renders from rows, not from its own command.
14. A command sent again with the same nonce gets the same answer from the
    receipt and changes nothing. This holds for every command, not only for
    those that create a row.

## E. What a host does differently

- A host reads with `state_only: true`: it receives state rows only, in the
  first read and afterwards.
- H pushes `inbox` items for the host's AIs and expects `ack`
  (`flows/agent-turn.md`).
- Tool calls from an AI that need group data (`read_thread`, `note_list`)
  become `fetch`; `list_members` and `list_tasks` are answered from the
  state rows the host already holds.

## F. Home on the same node

When the host role and the home role are in one process, the host reaches
the home through the same interface it uses for a remote home, with the same
message structs. The assembly package `internal/node` plugs the local home
in; `host` does not import `home` (`architecture.md` section 8). Nothing is
signed twice and nothing goes through WebRTC, but envelopes are still built
and stored, because other hosts and, later, other people's devices will
verify them.

## G. Retention and deletion

- Once a day H deletes messages older than the retention period, and tasks
  that finished before it (R-135). These deletions are not announced; caches
  are short-lived.
- Exempt (R-102): notes, and `review` messages, which are the ledger. They
  stay until a person deletes them.
- A row removed on purpose (a member leaves, a person deletes a note or a
  ledger entry) is announced: H pushes the row with the same `r` and `id`, a
  new `seq`, and `gone: true`. A client keeps the marker in place of the
  row and shows nothing for it; an older copy of the row that arrives
  afterwards, in a `fetch` page asked for earlier, has a lower `seq` and is
  ignored. H keeps the marker so that "rows after n" includes it; the daily
  run removes markers older than the retention period and raises the purge
  mark.
- A card that is still pending is not deleted by the daily run.
- A browser cache lives in memory and is rebuilt on reload.

## H. When the home is unreachable

- Browsers show the last rows they hold, read-only, with "cannot reach the
  group home".
- Hosts keep running turns already started. Tool calls that need the home
  fail with `home_unreachable`, which the AI sees as the tool's error text.
- The owner can still stop their AIs: "all my AIs offline" goes from the
  browser straight to each node and needs no home.
- Nothing is queued for later on the client side except the commands that
  were already in flight. When the home is back, a host sends the current
  status of its AIs; what happened in between is not replayed.
