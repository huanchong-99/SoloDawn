# Contract: storage (SQLite schema draft)

One database file per node, `node.db`, through `modernc.org/sqlite`. WAL
mode. One writer goroutine. Schema version in `PRAGMA user_version`;
migrations are an ordered list of SQL strings applied at start.

The node has two roles, so the file has two groups of tables. A node that is
home to no group simply has empty home tables.

Draft: column lists may change during implementation. The shape (which
tables exist and what each is for) is the contract.

## 1. Home tables

### `row`: every row of every group homed here

The wire rows of `contracts/rows.md` are stored as they are sent.

```sql
CREATE TABLE row (
  g       TEXT    NOT NULL,   -- group id
  r       TEXT    NOT NULL,   -- group | member | device | task | msg | note
  id      TEXT    NOT NULL,
  seq     INTEGER NOT NULL,   -- group sequence at last change: the sync cursor
  pos     INTEGER NOT NULL,   -- group sequence at creation: display and paging order
  ts      INTEGER NOT NULL,   -- creation time
  mt      INTEGER NOT NULL,   -- time of last change
  thread  TEXT,               -- msg: task id, NULL for main line
  in_main INTEGER NOT NULL DEFAULT 0,  -- msg: 1 if shown in the main line
  pending INTEGER NOT NULL DEFAULT 0,  -- msg: 1 while its card state is pending
  final   INTEGER NOT NULL DEFAULT 0,  -- task: 1 if merged or cancelled
  keep    INTEGER NOT NULL DEFAULT 0,  -- 1 for notes and review messages: exempt from retention
  gone    INTEGER NOT NULL DEFAULT 0,  -- 1 for the marker of a row removed on purpose
  data    TEXT    NOT NULL,   -- the row as JSON, exactly as pushed
  PRIMARY KEY (g, r, id)
) STRICT;
CREATE UNIQUE INDEX row_by_seq    ON row (g, seq);
CREATE INDEX        row_by_thread ON row (g, thread, pos) WHERE r = 'msg';
CREATE INDEX        row_main      ON row (g, pos) WHERE r = 'msg' AND in_main = 1;
CREATE INDEX        row_pending   ON row (g) WHERE pending = 1;
CREATE INDEX        row_keep      ON row (g, r, pos) WHERE keep = 1;
```

- "Rows after n, up to m": `SELECT data FROM row WHERE g = ? AND seq > ?
  AND seq <= ? ORDER BY seq`, markers included, run in a read transaction
  opened by the writer at the moment it takes S. The snapshot of that
  transaction is the cut: later writes do not appear in it and cannot
  replace a row it contains. The upper bound lets the home read the rows
  up to S in batches.
- A thread page: `… WHERE g = ? AND r = 'msg' AND thread = ? AND pos < ?
  ORDER BY pos DESC LIMIT ?`. The ledger and the notes are read through
  `row_keep`.
- State rows (`group`, `member`, `device`, `task`) of a group are also held
  in memory by the home while the node runs; they are small. The rule code
  reads memory and writes through to this table. Messages and notes are read
  from disk.

### `grp`: counters

```sql
CREATE TABLE grp (
  g      TEXT PRIMARY KEY,
  seq    INTEGER NOT NULL,     -- last sequence number given out
  num    INTEGER NOT NULL,     -- last task number given out (T-n)
  purged INTEGER NOT NULL DEFAULT 0   -- highest seq of a gone marker that retention removed
) STRICT;
```

Each command runs in one transaction: bump `seq` once per changed row, write
the rows, the inbox items and the receipt, commit, then push. A `sync` whose
`since` is below `purged` is answered with a reset.

### `inbox`: items waiting for AIs

```sql
CREATE TABLE inbox (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  g          TEXT    NOT NULL,
  ai         TEXT    NOT NULL,   -- member id
  host       TEXT    NOT NULL,   -- device id of the AI's host
  why        TEXT    NOT NULL,
  msg        TEXT,               -- message id
  task       TEXT,               -- task id
  commit_id  TEXT,               -- review, merge: the commit the item is about
  env        TEXT    NOT NULL,   -- JSON: the item's envelope, signed when the item was queued
  cause      TEXT,               -- JSON: the signed act that led to the item
  not_before INTEGER NOT NULL DEFAULT 0,  -- wake-up cap, round limit hold
  created    INTEGER NOT NULL
) STRICT;
CREATE INDEX inbox_by_host ON inbox (host, id);
```

The item's envelope is built and signed in the transaction that queues the
item, so it says what was decided then. A row is deleted when its host
acknowledges it, or when a later change of the task makes it pointless
(`contracts/task-machine.md` section 2).

### `push_sub`: reminders for browsers

```sql
CREATE TABLE push_sub (
  device TEXT PRIMARY KEY,     -- the browser's device id
  user   TEXT NOT NULL,
  sub    TEXT NOT NULL,        -- JSON: endpoint, p256dh, auth
  vapid  TEXT NOT NULL,        -- JSON: pub, priv (created by the browser)
  events TEXT NOT NULL         -- JSON list
) STRICT;
```

## 2. Tables used by both roles

```sql
CREATE TABLE device_log (       -- verified device logs
  user TEXT    NOT NULL,        -- user id
  ver  INTEGER NOT NULL,
  env  TEXT    NOT NULL,        -- the entry's envelope, JSON
  PRIMARY KEY (user, ver)
) STRICT;

CREATE TABLE receipt (          -- one per accepted command (commands.md, "Once only")
  k      TEXT    NOT NULL,      -- signer device id
  n      TEXT    NOT NULL,      -- the command's nonce
  ts     INTEGER NOT NULL,      -- the command's own ts
  answer TEXT    NOT NULL,      -- JSON: the outcome (ok and result fields, or the error code); no request id, no secret
  PRIMARY KEY (k, n)
) STRICT;

CREATE TABLE processed_offer (
  signer TEXT NOT NULL,
  conn TEXT NOT NULL,
  expires_ms INTEGER NOT NULL,
  PRIMARY KEY (signer, conn)
) STRICT;

CREATE TABLE setting (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL           -- JSON
) STRICT;
```

- `device_log` holds the log of the node's owner and of every person in a
  group homed here. The valid device set of each person is rebuilt from it
  at start and kept in memory. A `device` row in `row` is the same entry,
  written once per group so that it gets a group sequence number.
- `receipt` is written in the transaction of the command it belongs to, for
  home commands and host commands alike. For `pair.browser` and
  `takeover.start` it records only that the command succeeded
  (`contracts/commands.md`, "Once only").
- `processed_offer` remembers each applied offer by (`signer`, `conn`) across
  restarts ([rendezvous §2.3](rendezvous.md#23-signalling-payloads-inside-env)). Its
  expiry is the offer's `ts + 120000`; delete it only when the node's clock is strictly later.
- `setting` keys: `name`, `owner` (user id), `rendezvous` (list of URLs),
  `retention_days` (90), `idle_minutes` (15), `max_open` (4),
  `default_level` (2), `all_level3` (false), `approvers` (list of user ids),
  `offline` (false), `homes` (the list `host.info` shows,
  `contracts/datachannel.md` section 7).

## 3. Host tables

```sql
CREATE TABLE agent (              -- AI CLIs registered on this machine
  agent       TEXT PRIMARY KEY,   -- claude | codex
  cli_version TEXT NOT NULL,
  account     TEXT,               -- personal | team | api_key; NULL until declared
  caps        TEXT NOT NULL DEFAULT '{}'   -- JSON, as in commands.md section 2
) STRICT;

CREATE TABLE perm (               -- R-40, R-45
  agent       TEXT    NOT NULL,
  g           TEXT    NOT NULL,   -- group id
  level       INTEGER NOT NULL,   -- 1 | 2 | 3
  repo_config INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (agent, g)
) STRICT;

CREATE TABLE session (            -- one per session (sessions.md)
  g           TEXT    NOT NULL,
  member      TEXT    NOT NULL,   -- member id of the AI in that group
  task        TEXT    NOT NULL DEFAULT '',  -- '' for the group session
  kind        TEXT    NOT NULL,   -- group | task | review
  folder      TEXT    NOT NULL,   -- working folder, relative to projects/<group>/
  agent       TEXT    NOT NULL,
  home        TEXT    NOT NULL,   -- device id of the group home
  name        TEXT    NOT NULL,
  secretary   INTEGER NOT NULL DEFAULT 0,
  cli_session TEXT,               -- the CLI's session or thread id
  state       TEXT    NOT NULL,   -- dormant | working | waiting | takeover
  last_active INTEGER NOT NULL DEFAULT 0,
  takeover_at TEXT,               -- commit noted at takeover start
  PRIMARY KEY (g, member, task)
) STRICT;

CREATE TABLE approval (           -- one per approval request (flows/approval.md)
  card   TEXT PRIMARY KEY,        -- id of the approval card
  g      TEXT NOT NULL,
  member TEXT NOT NULL,
  task   TEXT NOT NULL,
  tool   TEXT NOT NULL,
  input  TEXT NOT NULL,           -- SHA-256 of the request input as the CLI sent it
  state  TEXT NOT NULL            -- pending | granted
) STRICT;

CREATE TABLE audit (              -- R-139
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  ts     INTEGER NOT NULL,
  g      TEXT,
  agent  TEXT,
  what   TEXT NOT NULL,           -- delivered | refused | requested | approval | takeover | handback | merge | setting | killed
  item   INTEGER,                 -- delivered, refused: the inbox item id
  home   TEXT,                    -- delivered, refused: device id of the home
  detail TEXT NOT NULL,           -- merge: the task and the commit (contracts/local.md section 7)
  by     TEXT,                    -- device id that caused it
  result TEXT
) STRICT;
CREATE INDEX audit_by_ts ON audit (ts);
CREATE UNIQUE INDEX audit_item ON audit (home, item) WHERE item IS NOT NULL;

CREATE TABLE usage (              -- R-61
  agent TEXT NOT NULL,
  day   TEXT NOT NULL,            -- YYYY-MM-DD, local time
  data  TEXT NOT NULL,            -- JSON: turns, tokens, cost, per task
  PRIMARY KEY (agent, day)
) STRICT;

CREATE TABLE peer (               -- remembered signalling addresses (R-133)
  device TEXT    NOT NULL,
  url    TEXT    NOT NULL,
  ok_at  INTEGER NOT NULL,
  PRIMARY KEY (device, url)
) STRICT;
```

- **Delivered once.** Before a host acknowledges an inbox item it writes the
  audit row `delivered` (or `refused`) with the home and the item id. An
  item that is already there is acknowledged again and not delivered again.
  The audit the owner reads (R-139) is the same record.
- **Approvals.** A row is written when the request is posted and removed
  when the request is denied, when the grant is used, and when the task is
  reassigned or ends.

Not stored: session tokens, relay credentials, the pairing secret, the
hourly wake-up counts, and anything from a terminal.

## 4. Retention (R-135)

Once a day, with `cutoff = now − retention_days`:

```sql
UPDATE grp SET purged = MAX(purged, (SELECT COALESCE(MAX(seq), 0) FROM row
                                     WHERE row.g = grp.g AND gone = 1 AND mt < :cutoff));
DELETE FROM row   WHERE gone = 1 AND mt < :cutoff;
DELETE FROM row   WHERE r = 'msg'  AND gone = 0 AND keep = 0 AND pending = 0
                    AND ts < :cutoff
                    AND NOT (data->>'kind' = 'brief'
                             AND data->>'$.state.status' = 'confirmed'
                             AND EXISTS (SELECT 1 FROM row AS t
                                         WHERE t.g = row.g AND t.r = 'task'
                                           AND t.data->>'state' = 'queued'
                                           AND t.data->>'brief' = row.id));
DELETE FROM row   WHERE r = 'task' AND final = 1 AND mt < :cutoff;
DELETE FROM inbox WHERE created < :cutoff;
DELETE FROM audit WHERE ts < :cutoff;
DELETE FROM usage WHERE day < :cutoff_day;
DELETE FROM peer  WHERE ok_at < :cutoff;
DELETE FROM receipt WHERE ts < :now - 86400000;
```

- A deletion marker leaves only through the second statement, after the
  first has moved `purged` past it. That is why the third has `gone = 0`:
  without it, an old message that was deleted recently would lose its
  marker while `purged` stayed, and a client would never learn of the
  deletion.
- A finished task is aged from the day it finished (`mt`), not from the day
  it was created.
- A card that is still pending is never deleted, however old: someone still
  has to act on it. It goes with the next run after it is decided.
- A confirmed brief is not deleted while a task made from it is still
  queued: that task's first `assign` item must carry the confirmation the
  brief holds. The statement reads this from the rows' JSON, which is
  enough at this size; with much more data it becomes an indexed column.
- Notes and `review` messages have `keep = 1` and are never deleted by this
  run (R-102). A person removes one with `forget`; the row then becomes a
  `gone` marker.
- Lowering `retention_days` takes effect at the next daily run, after one
  confirmation in the web client.

## 5. What is on a browser

- IndexedDB: the device key (non-extractable), the owner's device log, the
  list of the owner's nodes and group homes with their signalling URLs, the
  VAPID key pair, interface preferences.
- Memory only: group rows.

## 6. Sizes to expect

A message row is about 0.5 to 2 KiB, at most 16 KiB. A busy project
group may write a few thousand rows a day; ninety days stay well under a
gigabyte. No sharding, no archive tables, no compaction beyond SQLite's own.
