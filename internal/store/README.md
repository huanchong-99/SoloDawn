# SQLite store

`Open(path)` migrates the node database and enables WAL for files. Use
`:memory:` for isolated tests. Each `Write` callback runs on the one writer;
return its first error to roll back counters, rows, inbox items and receipts
together. Queue signing happens inside that callback, after allocating the id.

Allocate a sequence with `NextSeq` for each changed row. Use it as the new
row's `seq` and, for a new message, its `pos`; keep the original `pos` and
creation time on updates. Pass the resulting proto value to `Put`.

A `Snapshot` pins the group's sequence and purge mark in the writer's order.
Read deltas or pages from it, then close it. A page holds the rows
below `before` in `pos` order, newest first; `before` 0 asks for the newest
page. A file-backed WAL reader does not
block new writes; the single-connection in-memory database must release its
snapshot before another transaction can run.

`Retain` is the daily transaction's cleanup operation, not a scheduler.
See [storage](../../planning/contracts/storage.md) and
[rows](../../planning/contracts/rows.md) for the schema and wire formats.
