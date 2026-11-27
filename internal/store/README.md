# SQLite store

`export.go` writes a standalone SQLite snapshot containing one group's rows
(including markers), counters, unacknowledged inbox and its people's device
logs and push subscriptions; no `setting`, so no invitation code. A move removes the group's
data, deletes its entry of the `invites` setting (its invitation code ends) and records its new
device in the `moved` setting. Import binds the group to the receiving node,
rebuilds the knowledge-base index and assigns destination-local inbox IDs.
Receipts and session rows are not copied. The file's `PRAGMA user_version` is the node's schema
version; `ExportedGroup` and `ImportGroup` refuse a file of another version in one line (a file of
`v3.0.0-rc.1` has 0).

`Open(path)` migrates the node database and enables WAL for files. Four migrations run in order,
saved as `PRAGMA user_version`: `schema.sql` (the tables of
[storage](../../planning/contracts/storage.md)), `processed_offer` (`offers.go`), the
knowledge-base index, and `forget.go`, which deletes every group of the model before 2026-11-26
with its sessions, approvals, levels and the devices and push subscriptions of everyone but the
node's owner, and replaces `session.takeover_at` by `session.report` (storage section 7). A
database with a higher version is refused. Use
`:memory:` for isolated tests. Each `Write` callback runs on the one writer;
return its first error to roll back counters, rows, inbox items and receipts
together. Item numbering happens inside that callback, after allocating the id.

Allocate a sequence with `NextSeq` for each changed row. Use it as the new
row's `seq` and, for a new message, its `pos`; keep the original `pos` and
creation time on updates. Pass the resulting proto value to `Put`.

A `Snapshot` (`read.go`, `kb.go`) pins the group's sequence and purge mark in the
writer's order. Read deltas (`AfterSeqUpTo`, `DeltaCount`), cold-sync sets
(`StateRows` and `Pending`, delegating to `Tx`), pages (`PageThread`,
`PageKind`), knowledge-base full-text search (`SearchKB`), or one live row
(`Row`, delegating to `Tx`) from it, then close
it. A page holds the rows below `before` in `pos` order, newest first; `before`
0 asks for the newest page. `PageKind` filters `"note"` to context notes without
a `type` and `"preset"` / `"kb"` to notes of that type. `Put` maintains the FTS5
`kb` table (with CJK character spreading) in the same transaction as `row`;
migration 3 creates it and fills it from `row`. `Row` returns `sql.ErrNoRows` when the row does not
exist or is a `gone` marker. A file-backed WAL reader does not block new
writes; the single-connection in-memory database must release its snapshot
before another transaction can run.

`home_reads.go` holds what the run loop of a home needs inside a `Write`: `Groups`,
`StateRows` and `Counters` to load a group, `LackingEntries` for an import, `Row`, `Thread`, `Pending` and `Queued` for
what a rule reads from disk, and `RaiseNum`, `Retire` and `Jump`. A read there sees what its
own transaction wrote, and never waits for the writer, which it runs on. `Invites(now)` returns
the open invitation codes by group from the `invites` setting
([people](../../planning/contracts/step-2/people.md) 2.6), each an `Invite` (`code`, `by`,
`expires`); an entry whose `expires` is not after `now` counts as none. The home reads it in every
command's transaction and the node at its start. `SetInvite(group, code, now)` stores a group's
code, or ends it with the zero `Invite`, and deletes every entry that has ended by `now`.

`offers.go` remembers applied signalling offers across restarts: `RememberOffer(signer, conn, ts, now)`
claims one offer and reports false when it was already claimed, and `HasOffer` asks. A claim
expires 120 seconds after the offer's `ts`. `receipt.go` keeps the replayable `Outcome` of each
accepted command by signer and nonce: `PutReceipt` writes it inside the command's transaction and
`Receipt` returns `sql.ErrNoRows` for a command that was not accepted. `TypedNotes` (`home_reads.go`)
lists the live notes of one `type` of a group by name.

`Retain(now, days)` is the daily transaction's cleanup operation, not a scheduler. It deletes
what is older than `days` by the statements of [storage](../../planning/contracts/storage.md)
section 4, which includes the rows of `processed_offer` that expired and the `receipt` rows older
than 24 hours, whatever `days` is. A pending card, a note, a `review` message, and a confirmed
brief, its tasks and their `direct` confirm cards while a task is not final, stay.
See [storage](../../planning/contracts/storage.md) and
[rows](../../planning/contracts/rows.md) for the schema and wire formats.
