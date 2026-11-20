# Contract: rows

Rows are what a home pushes in `rows` and returns from `fetch`
(`contracts/datachannel.md`). The web client renders rows and nothing else.
Every row has:

| Field | Meaning |
| --- | --- |
| `r` | row type: `group`, `member`, `device`, `task`, `msg`, `note` |
| `id` | unique within its type and group |
| `seq` | group sequence number at the row's last change |
| `gone` | optional: `true` on the marker of a row that was removed on purpose. A marker has `r`, `id`, `seq`, `gone` and nothing else. A client keeps it and shows nothing for it (`flows/sync.md` G) |

**Presence** follows `contracts/conventions.md` section 2.

Ids of members, tasks and messages are random ids unless stated.

## 1. `group`

```json
{"r":"group","id":"Qm1…","seq":812,"name":"Website","kind":"project",
 "repo":"https://git.example/team/website.git","base":"main","forge":"github",
 "home":"<device id>","secretary":"<member id>",
 "checks":["go vet ./...","go test ./..."],"pass":90,"pass_foundation":70,
 "limits":{"rounds":10,"wakes_per_hour":20,"turn_minutes":60,"failures":3,
           "review_rounds":5,"ask_depth":4,"ask_wait_s":45,"approval_wait_s":600,
           "idle_notice_minutes":30},
 "paused":null,"created":1763500000000}
```

| Field | Values |
| --- | --- |
| `kind` | `project`; `team` from step 2 (no repository, no tasks) |
| `forge` | `github` (through the `gh` CLI) or `none` (single machine, no remote) |
| `checks` | the project's own check commands, run in the task worktree (R-103) |
| `paused` | `{"by":"<member id>","at":<time>}` after an emergency stop, or `null` |

Optional: `secretary` (absent while the group has none); `repo` (absent only
for a group without a repository, step 2).

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
| `path` | path between the AI's host and the home: `local` (same node), `direct`, `relay` (`flows/connect.md` step 8) |
| `usage` | what the CLI reports. Always an object; `{}` when nothing was reported |

Optional, AI only: `task` (absent when its state names no task); every key
of `usage`: `window_pct`, `window_resets`, `day_cap_pct`, `spend_today`,
`spend_cap`, `turns_today`. A person's row has no optional field.

`state` and `task` describe the AI in this group as a whole, derived by the
host as `sessions.md` section 3 says.

`[unverified]` Which usage figures each CLI exposes in structured mode.
Spikes S-2 and S-3 record them; the card shows what exists.

## 3. `device`

One row per accepted device-log entry of a person in the group.

```json
{"r":"device","id":"<user id>:3","seq":41,"user":"<user id>","ver":3,
 "op":"add","dev":"<device id>","kind":"node","name":"workstation",
 "env":{"p":"…","k":"…","s":"…"}}
```

Optional: `kind` and `name`, which an `add` entry has and a `revoke` entry
does not.

## 4. `task`

```json
{"r":"task","id":"t12…","seq":805,"g":"Qm1…","num":12,
 "title":"Node: report connection path changes",
 "slug":"node-report-connection-path-changes",
 "origin":"brief","brief":"<msg id>",
 "state":"active","step":"review","commit":"9f2c1ab",
 "assignee":"<member id>","reviewer":"<member id>",
 "foundation":false,"pass":90,"depends":[],
 "branch":"task/12-node-report-connection-path-changes","pr":31,"pr_url":"https://…/pull/31",
 "checks":{"local":{"ok":true,"at":1763598000000,"commit":"9f2c1ab"},
           "cloud":{"ok":true,"at":1763598300000,"commit":"9f2c1ab"}},
 "review":{"round":1,"failed":1,"score":86,"ok":false,"by":"<member id>","commit":"9f2c1ab","at":1763598900000},
 "wait":null,"failures":0,
 "thread":{"count":7,"rounds":3,
           "last":{"by":"<member id>","kind":"progress","text":"Adding the event after switch-back","ts":1763599000000}},
 "stats":{"added":212,"removed":18,"started":1763595000000,"ended":null},
 "created":1763595000000}
```

| Field | Values |
| --- | --- |
| `num` | per-group counter, shown as `T-12` |
| `slug` | the task's name in its branch `task/<num>-<slug>` and its worktree: 1 to 40 characters of `a` to `z`, `0` to `9` and `-`, made from `title` by the rule of `contracts/local.md` section 7. The home writes it once, when it creates the task, and never changes it; hosts read it from here. A reader refuses a task row whose `slug` is any other text, a copy of the row beside an inbox item included, so that none reaches a path or a branch name. In code: `proto.MakeSlug` and the type `proto.Slug`, and `Slug` in `web/src/lib/proto.ts` |
| `origin` | `brief` (from a confirmed brief), `direct` (R-71), `takeover` (R-83) |
| `state` | `queued`, `active`, `paused`, `failed`, `merged`, `cancelled` |
| `step` | `branch`, `local_check`, `cloud_check`, `review`, `merge`. The card draws six positions; "pull request" is done when `pr` is set (`contracts/task-machine.md` section 1) |
| `depends` | list of **task ids**: the tasks that must be merged before this one starts. (In a brief the same relation is written as indexes; the home turns them into ids when it creates the tasks.) |
| `checks` | always an object; `{}` before the first check |
| `wait` | `{"kind":…,"msg":"<msg id>","who":["<user id>"],"since":<time>}`, the latest reason the task is not moving, or `null` |
| `wait.kind` | while `queued`: `depends`; while `active`: `approval`, `decision`, `reviewer`, `conflict`; while `paused`: `manual`, `rounds`, `preempted` |
| `failures` | consecutive failures (R-63) |
| `review.failed` | failed reviews since the task was created or a person last resumed or reassigned it from `failed`; the stop of R-63 counts it (`contracts/task-machine.md` row 7) |
| `thread.rounds` | AI-to-AI exchanges since the last human message in the thread (R-60) |
| `thread.last.text` | a preview: at most the first 200 characters of that message's text. A long message is cut here, never refused because of it |
| `stats.ended` | the time the task finished, or `null` |

Optional:

| Field | Present |
| --- | --- |
| `brief` | when `origin` is `brief` |
| `plan` | when `origin` is `direct` or `takeover` and the `plan` of `task start` is not empty: that plan |
| `commit` | from the first submission: what hosted checks, the review and the merge are about |
| `reviewer` | once one is named |
| `branch` | once the branch exists |
| `pr`, `pr_url` | after a submission, when the group has a remote |
| `review` | after the first review; `review.failed` counts from 0 before it |
| `checks.local`, `checks.cloud` | after that check has run |
| `thread.last` | when the thread has a message |
| `wait.msg`, `wait.who` | when the wait is about a card, and about people |
| `wait.kept` | on a wait of a paused task (`manual`, `rounds`, `preempted`) that replaced a `reviewer` or `conflict` wait: that `kind`, restored when the pause ends (`contracts/task-machine.md` section 2) |

What the mock-up badges map to. A task shows one badge: the first row,
from the top, whose condition holds.

| Badge | Condition |
| --- | --- |
| merged | `state = merged` |
| cancelled | `state = cancelled` |
| stopped after failures | `state = failed` |
| owner is operating directly | the assignee's member row has `state = takeover` and `task` = this task |
| paused | `state = paused` |
| queued | `state = queued` |
| waiting for your approval / for someone's approval | `wait.kind = approval`, viewer in `wait.who` or not |
| waiting for your decision / for someone's decision | `wait.kind = decision`, viewer in `wait.who` or not |
| waiting for a reviewer | `wait.kind = reviewer` |
| merge conflict | `wait.kind = conflict` |
| sent back by review | `state = active`, `review.ok = false` |
| in progress | `state = active` |

## 5. `msg`

```json
{"r":"msg","id":"Zk3…","seq":814,"pos":790,"g":"Qm1…","thread":"t12…","main":false,
 "by":"<member id>","kind":"progress","ts":1763599000000,
 "text":"Branch created. Plan: add the event, emit on switch-back, write tests.",
 "mentions":[],"re":null,"body":{},"state":null,
 "env":{"p":"…","k":"…","s":"…"}}
```

| Field | Meaning |
| --- | --- |
| `pos` | the group sequence number when the message was created. It never changes. Messages are shown and paged in `pos` order; `seq` moves when a card's `state` changes and is only for sync |
| `thread` | task id, or `null` for a main-line message |
| `main` | `true` if it also appears in the main line (cards raised from a thread) |
| `by` | member id, or `"system"` for rows written by the home |
| `text` | plain text with Markdown code spans; `""` when the message is only a card. How long it can be is below |
| `mentions` | member ids |
| `re` | the message this one answers, or `null` |
| `body` | always an object, fixed after creation; its shape depends on `kind`, and it is `{}` for kinds without one |
| `state` | the only mutable part, changed by `act` commands and by the home; `null` for a message that is not a card. A decided card keeps the signed `act` that decided it in `state.act` |
| `env` | the author's envelope |

Optional: `env`, absent on rows written by the home.

The kinds, the body of each, and the states of cards are in
`contracts/messages.md`.

**How long a message can be.** The limit is that of the row
(`contracts/conventions.md` section 4), and the row holds the text twice:
as `text`, and inside `env.p`, where it is escaped once more. Counted in
the row, an ASCII character costs 2 bytes, a line break 5, a quote or a
backslash 6, a Chinese character 6; the rest of the row takes about 0.8
KiB. That is about 7,700 characters of plain English, about 2,500 Chinese
characters, and 5.5 to 7 KiB of source code, less the more quotes it has
(2.5 KiB if it were nothing but quotes). A client estimates the row by
this rule and tells the writer before sending. Go's `encoding/json`
always writes U+2028 and U+2029 as six-byte escapes, also with HTML
escaping off, and a browser writes them as they are: an estimate made in
a browser undercounts those two characters, and the size the node
measures decides. This limit is `risks.md` K-22.

## 6. `note`

```json
{"r":"note","id":"n4…","seq":640,"pos":640,"g":"Qm1…","task":null,
 "text":"Path events use fields path, rtt_ms, since, limited.",
 "by":"<member id>","ts":1763590000000}
```

`task` is a task id or `null`. `pos` is the group sequence number when the
note was created, as on a message: notes are listed and paged in `pos`
order (`fetch` with `kind: note`). A note has no optional field.

Notes are the group's long-term memory (R-102). They are not part of
`sync`; a client reads them with `fetch`, and changes are pushed like any
row. The ledger of scored points is the set of `review` messages; it needs
no separate row. Notes and `review` messages are exempt from retention and
stay until a person deletes them (`forget` in `contracts/commands.md`).
