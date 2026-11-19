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
  seq     INTEGER NOT NULL,   -- group sequence at last change
  ts      INTEGER NOT NULL,   -- creation time, for retention
  thread  TEXT,               -- msg: task id, NULL for main line
  in_main INTEGER NOT NULL DEFAULT 0,  -- msg: 1 if shown in the main line
  final   INTEGER NOT NULL DEFAULT 0,  -- task: 1 if merged, cancelled or failed
  keep    INTEGER NOT NULL DEFAULT 0,  -- 1 for notes and review messages: exempt from retention
  gone    INTEGER NOT NULL DEFAULT 0,  -- 1 for the marker of a row removed on purpose
  data    TEXT    NOT NULL,   -- the row as JSON, exactly as pushed
  PRIMARY KEY (g, r, id)
) STRICT;
CREATE UNIQUE INDEX row_by_seq    ON row (g, seq);
CREATE INDEX        row_by_thread ON row (g, thread, seq) WHERE r = 'msg';
CREATE INDEX        row_main      ON row (g, seq) WHERE r = 'msg' AND in_main = 1;
```

- "Rows after n": `SELECT data FROM row WHERE g = ? AND seq > ? ORDER BY seq`.
- A thread page: `… WHERE g = ? AND r = 'msg' AND thread = ? AND seq < ?
  ORDER BY seq DESC LIMIT ?`.
- State rows (`group`, `member`, `device`, `task`, `note`) of a group are
  also held in memory by the home while the node runs; they are small. The
  rule code reads memory and writes through to this table. Messages are read
  from disk.
- The message id is the nonce of the command that created it, so a repeated
  command hits the primary key and is answered `ok` without effect.

### `grp`: sequence counter

```sql
CREATE TABLE grp (
  g    TEXT PRIMARY KEY,
  seq  INTEGER NOT NULL,      -- last sequence number given out
  num  INTEGER NOT NULL       -- last task number given out (T-n)
) STRICT;
```

Each command runs in one transaction: bump `seq` once per changed row, write
the rows, write inbox items, commit, then push.

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
  jump       INTEGER NOT NULL DEFAULT 0,
  not_before INTEGER NOT NULL DEFAULT 0,  -- wake-up cap, round limit hold
  extra      TEXT,               -- JSON, e.g. the approver's envelope
  created    INTEGER NOT NULL
) STRICT;
CREATE INDEX inbox_by_host ON inbox (host, id);
```

A row is deleted when the host acknowledges it.

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

### `device_log`: verified device logs

```sql
CREATE TABLE device_log (
  user TEXT    NOT NULL,       -- user id
  ver  INTEGER NOT NULL,
  env  TEXT    NOT NULL,       -- the entry's envelope, JSON
  PRIMARY KEY (user, ver)
) STRICT;
```

Holds the log of the node's owner and of every person in a group homed here.
The valid device set of each person is rebuilt from it at start and kept in
memory. A `device` row in `row` is the same entry, written once per group so
that it gets a group sequence number.

### `setting`: key and value

```sql
CREATE TABLE setting (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL          -- JSON
) STRICT;
```

Keys: `name`, `owner` (user id), `rendezvous` (list of URLs),
`retention_days` (90), `idle_minutes` (15), `max_open` (4),
`default_level` (2), `all_level3` (false), `approvers` (list of user ids),
`offline` (false).

## 3. Host tables

```sql
CREATE TABLE agent (              -- AI CLIs registered on this machine
  agent       TEXT PRIMARY KEY,   -- claude | codex
  cli_version TEXT NOT NULL,
  account     TEXT,               -- personal | team | api_key; NULL until declared
  model       TEXT,
  caps        TEXT NOT NULL DEFAULT '{}'   -- JSON: day_pct, per-task limits
) STRICT;

CREATE TABLE perm (               -- R-40, R-45
  agent       TEXT    NOT NULL,
  g           TEXT    NOT NULL,   -- group id, or '*' for the default
  level       INTEGER NOT NULL,   -- 1 | 2 | 3
  repo_config INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (agent, g)
) STRICT;

CREATE TABLE session (            -- one per session (architecture.md section 6)
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

CREATE TABLE grant_once (         -- late approvals (flows/approval.md D)
  g      TEXT NOT NULL,
  member TEXT NOT NULL,
  tool   TEXT NOT NULL,
  input  TEXT NOT NULL,           -- SHA-256 of the exact request input
  task   TEXT,
  PRIMARY KEY (g, member, tool, input)
) STRICT;

CREATE TABLE audit (              -- R-139
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  ts     INTEGER NOT NULL,
  g      TEXT,
  agent  TEXT,
  what   TEXT NOT NULL,           -- delivered | refused | requested | approval | takeover | handback | merge | setting
  detail TEXT NOT NULL,
  by     TEXT,                    -- device id that caused it
  result TEXT
) STRICT;
CREATE INDEX audit_by_ts ON audit (ts);

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

Not stored: which inbox items were delivered (the host acknowledges an item
just before writing it to a session and remembers delivered ids in memory
for the life of the process); pending approval requests (they live in the
waiting CLI process and in the card on the home); session tokens; relay
credentials; the pairing secret; anything from a terminal.

## 4. Retention (R-135)

Once a day, with `cutoff = now − retention_days`:

```sql
DELETE FROM row   WHERE r = 'msg'  AND keep = 0 AND ts < :cutoff;
DELETE FROM row   WHERE r = 'task' AND final = 1 AND ts < :cutoff;
DELETE FROM row   WHERE gone = 1 AND ts < :cutoff;
DELETE FROM inbox WHERE created < :cutoff;
DELETE FROM audit WHERE ts < :cutoff;
DELETE FROM usage WHERE day < :cutoff_day;
DELETE FROM peer  WHERE ok_at < :cutoff;
```

Lowering `retention_days` takes effect at the next daily run, after one
confirmation in the web client.

Notes and `review` messages have `keep = 1` and are never deleted by this
run (R-102). A person removes one with `forget`; the row then becomes a
`gone` marker whose `ts` is the time of removal.

## 5. What is on a browser

- IndexedDB: the device key (non-extractable), the owner's device log, the
  list of the owner's nodes and group homes with their signalling URLs, the
  VAPID key pair, interface preferences.
- Memory only: group rows.

## 6. Sizes to expect

A message row is about 0.5 to 2 KiB. A busy project group may write a few
thousand rows a day; ninety days stay well under a gigabyte. No sharding, no
archive tables, no compaction beyond SQLite's own.
