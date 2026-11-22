CREATE TABLE row (
  g       TEXT    NOT NULL,
  r       TEXT    NOT NULL,
  id      TEXT    NOT NULL,
  seq     INTEGER NOT NULL,
  pos     INTEGER NOT NULL,
  ts      INTEGER NOT NULL,
  mt      INTEGER NOT NULL,
  thread  TEXT,
  in_main INTEGER NOT NULL DEFAULT 0,
  pending INTEGER NOT NULL DEFAULT 0,
  final   INTEGER NOT NULL DEFAULT 0,
  keep    INTEGER NOT NULL DEFAULT 0,
  gone    INTEGER NOT NULL DEFAULT 0,
  data    TEXT    NOT NULL,
  PRIMARY KEY (g, r, id)
) STRICT;
CREATE UNIQUE INDEX row_by_seq    ON row (g, seq);
CREATE INDEX        row_by_thread ON row (g, thread, pos) WHERE r = 'msg';
CREATE INDEX        row_main      ON row (g, pos) WHERE r = 'msg' AND in_main = 1;
CREATE INDEX        row_pending   ON row (g) WHERE pending = 1;
CREATE INDEX        row_keep      ON row (g, r, pos) WHERE keep = 1;
CREATE UNIQUE INDEX row_named     ON row (g, data->>'type', data->>'name') WHERE r = 'note' AND gone = 0 AND data->>'type' IS NOT NULL;

CREATE TABLE grp (
  g      TEXT PRIMARY KEY,
  seq    INTEGER NOT NULL,
  num    INTEGER NOT NULL,
  purged INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE inbox (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  g          TEXT    NOT NULL,
  ai         TEXT    NOT NULL,
  host       TEXT    NOT NULL,
  why        TEXT    NOT NULL,
  msg        TEXT,
  task       TEXT,
  commit_id  TEXT,
  env        TEXT    NOT NULL,
  not_before INTEGER NOT NULL DEFAULT 0,
  created    INTEGER NOT NULL
) STRICT;
CREATE INDEX inbox_by_host ON inbox (host, id);

CREATE TABLE push_sub (
  device TEXT PRIMARY KEY,
  user   TEXT NOT NULL,
  sub    TEXT NOT NULL,
  vapid  TEXT NOT NULL,
  events TEXT NOT NULL
) STRICT;

CREATE TABLE device_log (
  user TEXT    NOT NULL,
  ver  INTEGER NOT NULL,
  env  TEXT    NOT NULL,
  PRIMARY KEY (user, ver)
) STRICT;

CREATE TABLE receipt (
  k      TEXT    NOT NULL,
  n      TEXT    NOT NULL,
  ts     INTEGER NOT NULL,
  answer TEXT    NOT NULL,
  PRIMARY KEY (k, n)
) STRICT;

CREATE TABLE setting (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
) STRICT;

CREATE TABLE agent (
  agent       TEXT PRIMARY KEY,
  cli_version TEXT NOT NULL,
  account     TEXT,
  model       TEXT,
  caps        TEXT NOT NULL DEFAULT '{}'
) STRICT;

CREATE TABLE perm (
  agent TEXT    NOT NULL,
  g     TEXT    NOT NULL,
  level INTEGER NOT NULL,
  PRIMARY KEY (agent, g)
) STRICT;

CREATE TABLE session (
  g           TEXT    NOT NULL,
  member      TEXT    NOT NULL,
  task        TEXT    NOT NULL DEFAULT '',
  kind        TEXT    NOT NULL,
  folder      TEXT    NOT NULL,
  agent       TEXT    NOT NULL,
  home        TEXT    NOT NULL,
  name        TEXT    NOT NULL,
  secretary   INTEGER NOT NULL DEFAULT 0,
  cli_session TEXT,
  state       TEXT    NOT NULL,
  last_active INTEGER NOT NULL DEFAULT 0,
  takeover_at TEXT,
  PRIMARY KEY (g, member, task)
) STRICT;

CREATE TABLE approval (
  card   TEXT PRIMARY KEY,
  g      TEXT NOT NULL,
  member TEXT NOT NULL,
  task   TEXT NOT NULL,
  tool   TEXT NOT NULL,
  input  TEXT NOT NULL,
  state  TEXT NOT NULL
) STRICT;

CREATE TABLE audit (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  ts     INTEGER NOT NULL,
  g      TEXT,
  agent  TEXT,
  what   TEXT NOT NULL,
  item   INTEGER,
  home   TEXT,
  detail TEXT NOT NULL,
  by     TEXT,
  result TEXT
) STRICT;
CREATE INDEX audit_by_ts ON audit (ts);
CREATE UNIQUE INDEX audit_item ON audit (home, item) WHERE item IS NOT NULL;

CREATE TABLE usage (
  agent TEXT NOT NULL,
  day   TEXT NOT NULL,
  data  TEXT NOT NULL,
  PRIMARY KEY (agent, day)
) STRICT;

CREATE TABLE peer (
  device TEXT    NOT NULL,
  url    TEXT    NOT NULL,
  ok_at  INTEGER NOT NULL,
  PRIMARY KEY (device, url)
) STRICT;
