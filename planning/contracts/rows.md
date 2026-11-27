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
{"r":"group","id":"Qm1…","seq":812,"name":"Website",
 "home":"<device id>","secretary":"<member id>",
 "pass":90,"pass_foundation":70,
 "limits":{"rounds":10,"wakes_per_hour":20,"turn_minutes":60,"failures":3,
           "review_rounds":5,"ask_depth":4,"ask_wait_s":45,"approval_wait_s":600,
           "idle_notice_minutes":30},
 "paused":null,"created":1763500000000}
```

| Field | Values |
| --- | --- |
| `paused` | `{"by":"<member id>","at":<time>}` after an emergency stop, or `null` |

Optional: `secretary` (absent while the group has none).

There is one kind of group (R-10): the row names no kind, no repository, no
branch, no forge and no check command. A group with people and no AI is an
ordinary group.

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
 "workspace":"/home/dev/website",
 "secretary":false,"account":"personal","allow":[],"approvers":["<user id>"],
 "state":"working","task":"<task id>","since":1763599000000,"path":"direct",
 "usage":{"window_pct":62,"window_resets":1763611200000,"day_cap_pct":80,
          "turns_today":14},
 "last_seen":1763600000000}
```

| Field | Values |
| --- | --- |
| `role` | `owner`, `admin`, `member` (people) |
| `cli` | `claude`, `codex`; more in step 3, among them `external`: an agent the node does not start, whose row may also have `tool` (`contracts/step-3/external.md` section 1) |
| `workspace` | the folder on `host` that every session of this AI in this group runs in (R-11): an absolute path, as the host resolved it when the AI was added, symbolic links resolved. Set by `member add_ai` and never changed afterwards: to move an AI to another folder its owner removes it and adds it again. Every member of the group reads it. One AI member has one workspace; two members may have the same one (`contracts/local.md` section 7) |
| `account` | `personal`, `team`, `api_key` (R-50) |
| `allow` | user ids that may direct this AI without confirmation (step 2, R-53) |
| `approvers` | user ids that may decide its approvals: the owner, plus delegates |
| `state` | `working`, `waiting`, `dormant`, `takeover`, `paused`, `limited`, `offline` |
| `path` | path between the AI's host and the home: `local` (same node), `direct`, `relay`, or `""` before a pair is selected (`flows/connect.md` step 8) |
| `usage` | what the CLI reports; for an external AI, what the agent itself reported. Always an object; `{}` when nothing was reported |

Optional, AI only: `task` (absent when its state names no task); every key
of `usage`: `window_pct`, `window_resets`, `day_cap_pct`, `spend_today`,
`spend_cap`, `turns_today`. A person's row has no optional field.

`state` and `task` describe the AI in this group as a whole, derived by the
host as `sessions.md` section 3 says. `since` is the home time at which the
AI entered its current `state`: set when the AI joins, when `member status`
changes `state`, and when a closed host connection sets `state = offline`
and clears `task`. `last_seen` on an AI member row is set when the AI joins
and on each accepted `member status`.

`[unverified]` Which usage figures each CLI exposes in structured mode.
Spikes S-2 and S-3 record them; the card shows what exists: the host sends
`window_pct` and `window_resets` from the CLI's latest rate-limit event and
`turns_today` from the local day's `usage` row (`{}` before either is
known). `day_cap_pct`, `spend_today` and `spend_cap` go with caps (the card
shows spend only against its cap), which are postponed after 1.0 with
`settings.set`.

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
 "origin":"brief","brief":"<msg id>",
 "state":"active","step":"review","report":"<msg id>",
 "assignee":"<member id>","reviewer":"<member id>",
 "foundation":false,"pass":90,"depends":[],
 "review":{"round":1,"failed":1,"score":86,"ok":false,"by":"<member id>","report":"<msg id>","at":1763598900000},
 "wait":null,"failures":0,
 "thread":{"count":7,"rounds":3,
           "last":{"by":"<member id>","kind":"progress","text":"Adding the event after switch-back","ts":1763599000000}},
 "stats":{"started":1763595000000,"ended":null},
 "created":1763595000000}
```

| Field | Values |
| --- | --- |
| `num` | per-group counter, shown as `T-12` |
| `origin` | `brief` (from a confirmed brief), `direct` (R-71) |
| `state` | `queued`, `active`, `paused`, `failed`, `done`, `cancelled`. The last two are final |
| `step` | `work`, `review`. The card draws three positions: work, review, done (`contracts/task-machine.md` section 1) |
| `depends` | list of **task ids**: the tasks that must be done before this one starts. (In a brief the same relation is written as indexes; the home turns them into ids when it creates the tasks.) |
| `wait` | `{"kind":…,"msg":"<msg id>","who":["<user id>"],"since":<time>}`, the latest reason the task is not moving, or `null` |
| `wait.kind` | while `queued`: `depends`, `busy` (the assignee's folder is in use by another task); while `active`: `approval`, `decision`, `reviewer`; while `paused`: `manual`, `rounds`, `preempted` |
| `failures` | consecutive failures (R-63) |
| `review.failed` | failed reviews since the task was created or a person last resumed or reassigned it from `failed`; the stop of R-63 counts it (`contracts/task-machine.md` row 7) |
| `thread.rounds` | AI-to-AI exchanges since the last human message in the thread (R-60) |
| `thread.last.text` | a preview: at most the first 200 characters of that message's text. A long message is cut here, never refused because of it |
| `stats.started` | the time the task became active: when it was created, or, for a task created `queued`, when it was started (`contracts/task-machine.md` section 2, "Starting a queued task") |
| `stats.ended` | the time the task finished, or `null` |

Optional:

| Field | Present |
| --- | --- |
| `brief` | when `origin` is `brief` |
| `plan` | when `origin` is `direct` and the `plan` of `task start` is not empty: that plan |
| `report` | from the first submission: the id of the `report` message that is under review (`contracts/task-machine.md` row 5) |
| `reviewer` | once one is named |
| `asked` | while it names any: those of the assignee and the reviewer whose role in this task needs their owner's `confirm` card (`contracts/step-2/confirm.md` section 6) |
| `review` | after the first review; `review.failed` counts from 0 before it. `review.report` is the report that review was about |
| `thread.last` | when the thread has a message |
| `wait.msg`, `wait.who` | when the wait is about a card, and about people |
| `wait.kept` | on a wait of a paused task (`manual`, `rounds`, `preempted`) that replaced a `reviewer` wait: that `kind`, restored when the pause ends (`contracts/task-machine.md` section 2) |
| `wait.by` | exactly on a wait of kind `manual`: the member id of whoever paused the task, the secretary or a person, written by the home from the sender of `task pause` (`contracts/task-machine.md` row 12). A wait that replaces it takes it away. The waits `rounds` and `preempted` have none: nobody chose them |

What the mock-up badges map to. A task shows one badge: the first row,
from the top, whose condition holds.

| Badge | Condition |
| --- | --- |
| done | `state = done` |
| cancelled | `state = cancelled` |
| stopped after failures | `state = failed` |
| owner is operating directly | the assignee's member row has `state = takeover` and `task` = this task |
| paused | `state = paused`; for `wait.kind = manual` the card says who paused it: `<name> paused the task` (in Chinese `<name> 暂停了任务`), the name being that of the member `wait.by`, kept after that member has left |
| queued | `state = queued`; `wait.kind` says for what: another task (`depends`), or the folder (`busy`) |
| waiting for your approval / for someone's approval | `wait.kind = approval`, viewer in `wait.who` or not |
| waiting for your decision / for someone's decision | `wait.kind = decision`, viewer in `wait.who` or not |
| waiting for a reviewer | `wait.kind = reviewer` |
| sent back by review | `state = active`, `review.ok = false` |
| in progress | `state = active` |

## 5. `msg`

```json
{"r":"msg","id":"Zk3…","seq":814,"pos":790,"g":"Qm1…","thread":"t12…","main":false,
 "by":"<member id>","kind":"progress","ts":1763599000000,
 "text":"Plan: add the event, emit on switch-back, write tests.",
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
order (`fetch` with `kind: note`). A context note has no optional field; style
presets and knowledge-base documents add optional `type` and `name`
(`contracts/step-3/knowledge.md` section 1).

Notes are the group's long-term memory (R-102). They are not part of
`sync`; a client reads them with `fetch`, and changes are pushed like any
row. The ledger of scored points is the set of `review` messages; it needs
no separate row. Notes and `review` messages are exempt from retention and
stay until a person deletes them (`forget` in `contracts/commands.md`).
