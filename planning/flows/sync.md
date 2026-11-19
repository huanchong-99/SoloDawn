# Flow: reading a group and sending commands

Contract: `contracts/datachannel.md`. Rule: R-134.

The home is the only writer of a group. Every row it stores carries `seq`,
the group's sequence number at the row's last change. Clients ask for "rows
after n" and then receive changes as they happen. Clients keep caches, never
authoritative copies.

Actors: C, a client (browser or host); H, the group home.

## A. First read

1. `ctl` is open and `hello`/`welcome` are done. `welcome` lists the groups C
   may read on this home and the current `seq` of each.
2. C sends `sync {g, since: 0}`.
3. H sends `rows` frames containing every **state row** of the group: the
   group row, members, device log entries of its people, tasks, notes.
4. H then sends the newest 50 main-line messages, and finishes with
   `synced {g, seq}`.
5. From now on H pushes a `rows` frame to C for every change in the group,
   in `seq` order, thread messages included.

A task row carries a summary of its thread (count, last message, AI round
counter), so the collapsed thread line needs no thread messages.

## B. Opening a thread or scrolling back

6. C sends `fetch {g, thread: <task id or null>, before: <seq>, limit}`.
7. H answers with up to `limit` messages of that scope with `seq` below
   `before`, newest first, and `more: true|false`.

## C. Reconnecting

8. C sends `sync {g, since: <last seq it saw>}`.
9. If at most 1000 rows changed since then, H sends exactly those rows in
   order, then `synced`.
10. Otherwise H answers `synced {g, seq, reset: true}` after sending what
    step 3 and 4 would send. C discards its cache for that group first.

## D. Sending a command

11. C builds the payload, signs it into an envelope, and sends
    `cmd {id, env}`.
12. H checks the envelope, checks that this member may do this
    (`contracts/datachannel.md`, permission table), and applies the command
    in one database transaction. Each row it changes gets the next `seq`.
13. H answers `{re: id, ok: true}` or `{re: id, err: {code, msg}}`.
14. H pushes the changed rows to every connected client of the group,
    including C. C renders from rows, not from its own command.
15. A command re-sent with the same nonce is answered `ok` again and changes
    nothing.

## E. What a host does differently

- A host syncs only state rows (`sync {g, since, state_only: true}`); it
  does not keep messages.
- H pushes `inbox` items for the host's AIs and expects `ack`
  (`flows/agent-turn.md`).
- Tool calls from an AI that need group data (`read_thread`, `list_tasks`)
  become `fetch` or are answered from the state rows the host already holds.

## F. Home on the same node

When the host role and the home role are in one process, the host calls the
home's handler directly with the same message structs. Nothing is signed
twice and nothing goes through WebRTC, but the envelope is still built and
stored, because other hosts and, later, other people's devices will verify
it.

## G. Retention

- Once a day H deletes messages older than the retention period, and tasks
  that reached a final state before it (R-135). Deletions are not announced;
  caches are short-lived.
- A browser cache lives in memory and is rebuilt on reload.

## H. When the home is unreachable

- Browsers show the last rows they hold, read-only, with "cannot reach the
  group home".
- Hosts keep running turns already started. Tool calls that need the home
  fail with `home_unreachable`, which the AI sees as the tool's error text.
- Nothing is queued for later on the client side except the commands that
  were already in flight (`flows/connect.md` step 18).
