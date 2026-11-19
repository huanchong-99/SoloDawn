# Contract: rows

Rows are what a home pushes in `rows` and returns from `fetch`
(`contracts/datachannel.md`). The web client renders rows and nothing else.
Every row has:

| Field | Meaning |
| --- | --- |
| `r` | row type: `group`, `member`, `device`, `task`, `msg`, `note` |
| `id` | unique within its type and group |
| `seq` | group sequence number at the row's last change |
| `gone` | present and `true` only on a row that was removed on purpose; it then carries nothing else but `r`, `id`, `seq` (`flows/sync.md` G) |

Optional fields are omitted when empty. Ids of members, tasks and messages
are random ids unless stated.

## 1. `group`

```json
{"r":"group","id":"Qm1…","seq":812,"name":"Website","kind":"project",
 "repo":"https://git.example/team/website.git","base":"main","forge":"github",
 "home":"<device id>","secretary":"<member id>",
 "checks":["go vet ./...","go test ./..."],"pass":90,"pass_foundation":70,
 "limits":{"rounds":10,"wakes_per_hour":20,"turn_minutes":60,"failures":3,
           "review_rounds":5,"ask_depth":4,"ask_wait_s":45,"approval_wait_s":600},
 "paused":null,"created":1763500000000}
```

| Field | Values |
| --- | --- |
| `kind` | `project`; `team` from step 2 (no repository, no tasks) |
| `forge` | `github` (through the `gh` CLI) or `none` (single machine, no remote) |
| `checks` | the project's own check commands, run in the task worktree (R-103) |
| `paused` | `null`, or `{"by":"<member id>","at":<time>}` after an emergency stop |

## 2. `member`

Person:

```json
{"r":"member","id":"m1…","seq":3,"g":"Qm1…","kind":"human","user":"<user id>",
 "name":"owner","role":"owner","online":true,"last_seen":1763600000000}
```

AI:

```json
{"r":"member","id":"m7…","seq":790,"g":"Qm1…","kind":"ai","user":"<owner's user id>",
 "name":"Claude Code","cli":"claude","host":"<device id>","host_name":"workstation",
 "secretary":false,"account":"personal","allow":[],"approvers":["<user id>"],
 "state":"working","task":"<task id>","since":1763599000000,"path":"direct",
 "usage":{"window_pct":62,"window_resets":1763611200000,"day_cap_pct":80,
          "turns_today":14},
 "last_seen":1763600000000}
```

| Field | Values |
| --- | --- |
| `role` | `owner`, `admin`, `member` (people) |
| `cli` | `claude`, `codex`; more in step 3 |
| `account` | `personal`, `team`, `api_key` (R-50) |
| `allow` | user ids that may direct this AI without confirmation (step 2, R-53) |
| `approvers` | user ids that may decide its approvals: the owner, plus delegates |
| `state` | `working`, `waiting`, `dormant`, `takeover`, `paused`, `limited`, `offline` |
| `path` | path between the AI's host and the home: `local` (same node), `lan`, `direct`, `relay` |
| `usage` | whatever the CLI reports; every key optional: `window_pct`, `window_resets`, `day_cap_pct`, `spend_today`, `spend_cap`, `turns_today` |

`state` and `task` describe the AI in this group as a whole. The host
derives them from the AI's sessions: the session that is running a turn or
is taken over, otherwise `waiting` if any session is open, otherwise
`dormant` (`architecture.md` section 6).

`[unverified]` Which usage figures each CLI exposes in structured mode.
Spikes S-2 and S-3 record them; the card shows what exists.

## 3. `device`

One row per accepted device-log entry of a person in the group.

```json
{"r":"device","id":"<user id>:3","seq":41,"user":"<user id>","ver":3,
 "op":"add","dev":"<device id>","kind":"node","name":"workstation",
 "env":{"p":"…","k":"…","s":"…"}}
```

## 4. `task`

```json
{"r":"task","id":"t12…","seq":805,"g":"Qm1…","num":12,
 "title":"Node: report connection path changes",
 "origin":"brief","brief":"<msg id>",
 "state":"active","step":"review",
 "assignee":"<member id>","reviewer":"<member id>",
 "foundation":false,"pass":90,"depends":[],
 "branch":"task/12-conn-events","pr":31,"pr_url":"https://…/pull/31",
 "checks":{"local":{"ok":true,"at":1763598000000,"commit":"9f2c1ab"},
           "cloud":{"ok":true,"at":1763598300000}},
 "review":{"round":1,"score":86,"ok":false,"by":"<member id>","at":1763598900000},
 "wait":null,"failures":0,
 "thread":{"count":7,"rounds":3,
           "last":{"by":"<member id>","kind":"progress","text":"Adding the event after switch-back","ts":1763599000000}},
 "stats":{"added":212,"removed":18,"started":1763595000000,"ended":null},
 "created":1763595000000}
```

| Field | Values |
| --- | --- |
| `num` | per-group counter, shown as `T-12` |
| `origin` | `brief` (from a confirmed brief), `direct` (R-71), `takeover` (R-83) |
| `state` | `queued`, `active`, `paused`, `failed`, `merged`, `cancelled` |
| `step` | `branch`, `local_check`, `pr`, `cloud_check`, `review`, `merge`; with `forge: none` the steps `pr` and `cloud_check` are skipped |
| `wait` | `null`, or `{"kind":…,"msg":"<msg id>","who":["<user id>"],"since":<time>}` |
| `wait.kind` | while `active`: `approval`, `decision`, `reviewer`, `assignee`, `conflict`; while `paused`: `rounds`, `preempted`, `cap`, `rate_limit`, `group`; while `queued`: `depends` |
| `failures` | consecutive failures (R-63) |
| `thread.rounds` | AI-to-AI exchanges since the last human message in the thread (R-60) |

What the mock-up badges map to:

| Badge | Condition |
| --- | --- |
| queued | `state = queued` |
| in progress | `state = active`, `wait = null` |
| waiting for your approval / for someone's approval | `wait.kind = approval`, viewer in `wait.who` or not |
| sent back by review | `state = active`, `review.ok = false` |
| stopped after failures | `state = failed` |
| merged | `state = merged` |
| owner is operating directly | the assignee's member row has `state = takeover` and `task` = this task |
| paused | `state = paused` |

## 5. `msg`

```json
{"r":"msg","id":"Zk3…","seq":814,"g":"Qm1…","thread":"t12…","main":false,
 "by":"<member id>","kind":"progress","ts":1763599000000,
 "text":"Branch created. Plan: add the event, emit on switch-back, write tests.",
 "mentions":[],"re":null,"body":{},"state":null,
 "env":{"p":"…","k":"…","s":"…"}}
```

| Field | Meaning |
| --- | --- |
| `thread` | task id, or `null` for a main-line message |
| `main` | `true` if it also appears in the main line (cards raised from a thread) |
| `by` | member id, or `"system"` for rows written by the home |
| `text` | at most 16 KiB of UTF-8; plain text with Markdown code spans |
| `mentions` | member ids |
| `re` | the message this one answers |
| `body` | fixed after creation; shape depends on `kind` |
| `state` | the only mutable part; changed by `act` commands and by the home |
| `env` | the author's envelope; absent on `system` rows |

### Kinds

| `kind` | Where | Who writes | `body` | `state` |
| --- | --- | --- | --- | --- |
| `text` | main, thread | anyone | — | — |
| `brief` | main | secretary | brief body | `{status, by, at, note}`; status `pending`, `confirmed`, `revise`, `superseded` |
| `task` | main | system | `{"task":"<task id>"}` (the card is drawn from the task row) | — |
| `approval` | thread + main | the AI's host | approval body | `{status, by, at}`; status `pending`, `approved`, `denied`, `expired`, `reassigned` |
| `decision` | main, or thread + main | an AI | decision body | `{status, choice, text, by, at}`; status `pending`, `decided` |
| `summary` | main | secretary | summary body | — |
| `progress` | thread | AI | — | — |
| `question` | thread | AI | — | — |
| `call` | thread, or main when the caller has no task | AI | `{"to":"<member id>","depth":1,"wait":true}` | `{status, reply, ms}`; status `waiting`, `answered` |
| `reply` | thread | AI | — (`re` is the call) | — |
| `report` | thread | AI | `{"pr":31,"files":4,"added":212,"removed":18}` | — |
| `checks` | thread | system | `{"stage":"local","ok":true,"items":[{"name":"go test ./...","ok":true,"detail":"42 tests"}]}`; `stage` is `local`, `cloud` or `merge` | — |
| `review` | thread | reviewer AI | review body | — |
| `handback` | thread | system, with the AI's text | `{"files":["a.go"],"commits":[{"sha":"9f2c1ab","subject":"…"}],"added":40,"removed":3}` | — |
| `system` | main, thread | system | `{"code":"…", …}` | — |

Approval cards are written by the AI's host and presented in the main line
under the secretary's name with a mention of the approvers (R-42). No AI
turn is spent on presenting them.

### Bodies

Brief:

```json
{"title":"Show the connection path; warn when relayed",
 "goal":"…","criteria":["Indicator follows a path change within 1 s","…"],
 "scope":{"in":["node events","web indicator"],"out":["the relay itself"]},
 "tasks":[{"title":"Node: report path changes","assignee":"<member id>",
           "reviewer":"<member id>","foundation":false,"depends":[]},
          {"title":"Web: indicator and notice","assignee":"<member id>",
           "reviewer":"<member id>","foundation":false,"depends":[]}],
 "pass":90,"requester":"<member id>","replaces":null}
```

`depends` holds indexes into `tasks`. Confirming the brief creates exactly
these tasks.

Approval:

```json
{"task":"t13…","ai":"<member id>","tool":"Bash",
 "command":"pnpm add -D @playwright/test\npnpm exec playwright install chromium",
 "why":"Criteria 1 and 3 must be timed in a real browser.",
 "category":"install","approvers":["<user id>"]}
```

`command` is the request exactly as the CLI presented it. `why` is the
justification the CLI attached, if any. `category` is a display hint
(`install`, `push`, `outside`, `network`, `secret`, `delete`, `other`) and
is never used to decide anything.

Decision:

```json
{"task":"t13…","question":"Where does the relay notice go?",
 "context":"Both are about the same work.",
 "options":[{"id":"A","label":"A line under the top bar","effect":"Most visible; always shown while relayed."},
            {"id":"B","label":"Only on the indicator","effect":"Quiet; easy to miss."}],
 "recommended":"B","to":["<user id>"]}
```

A decision may be answered with an option, with free text, or both.

Review:

```json
{"task":"t12…","num":12,"title":"Node: report connection path changes",
 "round":1,"score":86,"pass":90,"ok":false,
 "items":[{"text":"No event after switching back to direct","file":"conn/switch.go","line":88,"veto":false}]}
```

A `veto` item forces `score` 0 (R-101). The home adds `num` and `title`, so
that a ledger entry still reads well after its task row has been removed by
retention.

Summary:

```json
{"task":"t9…","score":94,"rounds":2,"minutes":72,"added":318,"removed":42,"pr":28}
```

System codes (the client localises them):

| `code` | Other fields |
| --- | --- |
| `rounds_limit` | `task`, `n` |
| `turn_interrupted` | `ai`, `reason` (`crash`, `timeout`, `stop`) |
| `group_paused`, `group_resumed` | `by` |
| `preempted` | `task`, `by` |
| `cap_reached` | `ai`, `cap` |
| `rate_limited` | `ai`, `resets` |
| `login_invalid` | `ai` (R-56) |
| `task_failed` | `task`, `reason` |
| `task_merged` | `task`, `commit`; shown under the secretary's name (R-24) |
| `session_replaced` | `ai` (a new session took over from notes, R-123) |

## 6. `note`

```json
{"r":"note","id":"n4…","seq":640,"g":"Qm1…","task":null,
 "text":"Path events use fields path, rtt_ms, since, limited.",
 "by":"<member id>","ts":1763590000000}
```

Notes are the group's long-term memory (R-102). The ledger of scored points
is the set of `review` messages; it needs no separate row. Notes and
`review` messages are exempt from retention and stay until a person deletes
them (`forget` in `contracts/commands.md`).
