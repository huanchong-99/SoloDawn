# Contract: message kinds, bodies and card states

A message is a `msg` row (`contracts/rows.md` section 5). Its `kind` selects
the shape of `body`, which never changes after creation, and of `state`,
which is the only part that changes. Presence follows
`contracts/conventions.md` section 2: every field below is always present
unless marked optional. Section 5 of that file says which task freezes
these shapes.

## 1. Kinds

| `kind` | Where | Signed by | `body` | `state` |
| --- | --- | --- | --- | --- |
| `text` | main, thread | a person, or an AI | `{}` | `null` |
| `progress` | thread | an AI | `{}` | `null` |
| `question` | main, thread | an AI | `{}` | `null` |
| `reply` | where its call is: thread, or main | an AI (`re` is the call) | `{}` | `null` |
| `brief` | main | the secretary | 2.1 | 3.1 |
| `task` | main | nobody: the home writes it | 2.2 | `null` |
| `approval` | main, or thread + main | an AI | 2.3 | 3.2 |
| `decision` | main, or thread + main | an AI | 2.4 | 3.3 |
| `summary` | thread + main | the secretary | 2.5 | `null` |
| `call` | thread, or main when the caller has no task | an AI | 2.6 | 3.4 |
| `report` | thread | an AI | 2.7 | `null` |
| `checks` | thread | nobody: the home writes it | 2.8 | `null` |
| `review` | thread | an AI, the reviewer | 2.9 | `null` |
| `handback` | thread (main for the secretary) | an AI | 2.10 | `null` |
| `system` | main, thread | nobody: the home writes it | 2.11 | `null` |

A signed message reaches the home as a `post` command
(`contracts/commands.md` 1.3). "An AI" signs through its host
(`contracts/envelope.md` section 5): whatever the model gave to a tool, the
host completes the body before it signs. For each kind, section 2 says:

- **signed**: the fields that `body` holds in the `post`. An optional
  field may be missing there as in the row; the home answers `bad_request`
  to a `post` that lacks any other;
- **added by the home**: the fields the home adds when it writes the row. A
  `post` does not carry them, and the home does not read them if it does.

The home builds the row's `body` from the signed fields it has checked and
the fields it adds. The author's own bytes stay in `env`. A message the
home writes itself has no `post` behind it: its `by` is `"system"` and it
has no `env`.

An `approval` is posted by the AI's host on its own, when the CLI asks for
permission; no AI turn is spent on it. It is presented in the main line
under the secretary's name, and the host sets its `mentions` to the member
ids of the people in `body.approvers` (R-42).

## 2. Bodies

### 2.1 `brief`

```json
{"title":"Show the connection path; warn when relayed",
 "goal":"…","criteria":["Indicator follows a path change within 1 s"],
 "scope":{"in":["node events","web indicator"],"out":["the relay itself"]},
 "tasks":[{"title":"Node: report path changes","assignee":"<member id>",
           "reviewer":"<member id>","foundation":false,"depends":[]},
          {"title":"Web: indicator and notice","assignee":"<member id>",
           "reviewer":"<member id>","foundation":false,"depends":[0]}],
 "pass":90,"requester":"<member id>","replaces":null}
```

| Field | Value |
| --- | --- |
| `criteria` | list of strings, at least one |
| `scope` | `in` and `out`, both lists |
| `tasks` | at least one entry; every field of an entry is always present |
| `tasks[].depends` | list of **indexes** into `tasks`, each lower than the entry's own, so a cycle cannot be written |
| `pass` | pass score of ordinary tasks, which the secretary's host fills from `group.pass` (it is no tool argument); foundation tasks use the group's `pass_foundation` |
| `replaces` | id of the brief this one replaces, or `null` |

Assignee and reviewer are working AIs, not the secretary, and differ.
Confirming the brief creates exactly these tasks; the task rows then carry
`depends` as task ids (`contracts/rows.md` section 4).

Signed: every field. Added by the home: nothing.

### 2.2 `task`

`{"task":"<task id>"}`. Written by the home when it creates the task. The
card is drawn from the task row.

### 2.3 `approval`

```json
{"task":"<task id>","ai":"<member id>","tool":"Bash",
 "command":"pnpm add -D @playwright/test\npnpm exec playwright install chromium",
 "why":"Criteria 1 and 3 must be timed in a real browser.",
 "category":"command","approvers":["<user id>"]}
```

| Field | Value |
| --- | --- |
| `task` | the task it is about, or `""` for a card in the main line (`thread` is then `null`) |
| `tool` | what the CLI names: Claude Code's tool (`Bash`, `Write`, …), or the type of Codex's item (`commandExecution`, `fileChange`) |
| `command` | the request as the CLI presented it (table below), or its beginning: at most 3 KiB as JSON text in the payload, cut at a character |
| `full` | optional: the size in bytes of the whole request, there when `command` holds only its beginning. The card then says that it shows a part, how long the whole is, and what `risks.md` K-21 recommends |
| `why` | optional: the justification the CLI attached; at most 256 bytes as JSON text, cut like `command` |
| `category` | `command`, `file`, `network`, `other`: the kind of request the CLI made, taken from the type of the request and never from the text of a command (C-10, `risks.md` Q-18). A display hint; it decides nothing |
| `approvers` | user ids, at least one |

The card's label is `category` plus `why`. The host fills the body from the
CLI's request (`contracts/drivers.md` 1.1 and 1.2); the first, second and
last two rows were seen in spikes S-2 and S-3:

| Request | `tool` | `category` | `command` | `why` |
| --- | --- | --- | --- | --- |
| Claude Code `approve`, `tool_name` `Bash` | `Bash` | `command` | `input.command` | `input.description` |
| the same with `Write`, `Edit`, `NotebookEdit`, `Read` | the tool name | `file` | `input` as JSON text | — |
| the same with `WebFetch`, `WebSearch` | the tool name | `network` | `input` as JSON text | — |
| the same with any other tool | the tool name | `other` | `input` as JSON text | — |
| Codex `item/commandExecution/requestApproval` | `commandExecution` | `command` | `command` | `reason` |
| Codex `item/fileChange/requestApproval` | `fileChange` | `file` | for each change of the item: `kind`, `path`, then its `diff` | `reason`, when it is not `null` |

Signed: every field. Added by the home: nothing.

A long request is shown in part and not refused: with these two limits the
card fits the row of a card (`contracts/conventions.md` section 4).
The owner's decision still covers the whole request, whose digest the host
recorded (`flows/approval.md` step 5). In the first version nobody can
read the rest of it (`risks.md` K-21).

### 2.4 `decision`

```json
{"task":"<task id>","question":"Where does the relay notice go?",
 "context":"Both are about the same work.",
 "options":[{"id":"A","label":"A line under the top bar","effect":"Most visible."},
            {"id":"B","label":"Only on the indicator","effect":"Quiet; easy to miss."}],
 "recommended":"B","to":["<user id>"]}
```

| Field | Value |
| --- | --- |
| `task` | optional: the task it is about |
| `context` | optional |
| `options` | list, empty when only free text is asked; every field of an option is always present |
| `recommended` | optional: the `id` of an option |
| `to` | user ids, at least one |

A decision with options is answered with one of them (with or without free
text) or with free text alone (2026-11-21 owner decision); one whose
`options` is empty, with free text (`contracts/commands.md` 1.4).

Signed: every field. Added by the home: nothing.

### 2.5 `summary`

`{"task":"<task id>","score":94,"rounds":2,"minutes":72,"added":318,"removed":42,"pr":28}`.
`pr` is optional (absent without a remote).

Signed: `{}`. Added by the home: every field, from the task row.

### 2.6 `call`

`{"to":"<member id>","depth":1,"wait":true}`.

Signed: every field; `depth` is counted by the host (`flows/ask.md`). Added
by the home: nothing.

### 2.7 `report`

`{"commit":"9f2c1ab","files":4,"added":212,"removed":18,"pr":31}`. `pr` is
optional.

Signed: every field; the host fills them at `submit_task`. `commit` is the
head of the task branch at that moment, the commit the `task submitted`
that follows names. It is in the body because a task is submitted again
after every failed review: each report says which commit it describes.
Added by the home: nothing.

### 2.8 `checks`

```json
{"stage":"local","ok":true,"commit":"9f2c1ab",
 "items":[{"name":"go test ./...","ok":true,"detail":"42 tests"}]}
```

`stage` is `local`, `cloud` or `merge`. `items[].detail` is optional. For
`merge` there is one item, named `merge`, whose `detail` is the output of
the failed merge.

Written by the home from the host's `task checked` (`local`), `task cloud`
(`cloud`) or `task conflict` (`merge`); `commit` is the one that command
names. It is in the body because a task is checked again after every
change, and what passes review and is merged is one commit
(`contracts/task-machine.md` section 4): each result says which commit it
is about.

### 2.9 `review`

```json
{"task":"<task id>","num":12,"title":"Node: report connection path changes",
 "commit":"9f2c1ab","round":1,"score":86,"pass":90,"ok":false,
 "items":[{"text":"No event after switching back to direct","veto":false,
           "file":"conn/switch.go","line":88}]}
```

`items[].file` and `items[].line` are optional.

Signed: `task`, `commit`, `score` and `items`. The reviewer gives `score`
and `items`; its host adds `task` and the commit of the review checkout
before it signs. Added by the home: `num`, `title`, `round`, `pass` and
`ok`, so that a ledger entry still reads well after its task row has been
removed by retention. The home also writes `score` as 0 when an item has
`veto` (R-101).

### 2.10 `handback`

`{"files":["a.go"],"commits":[{"sha":"9f2c1ab","subject":"…"}],"added":40,"removed":3}`.

Signed: every field; the host fills them from Git. Added by the home:
nothing.

### 2.11 `system`

`{"code":"…", …}`. Written by the home; the client localises the text from
`code` and the other fields. Some codes are the home's own. The others
start as a fact that a host reports in `member status`
(`contracts/commands.md` 1.9): the host sends the fields that the code's
row lists below, except `ai`, which the home adds.

| `code` | From | Always present | Optional |
| --- | --- | --- | --- |
| `rounds_limit` | home | `task`, `n` (the limit that was reached, `limits.rounds`) | — |
| `turn_interrupted` | host | `ai`, `reason` (`crash`, `timeout`, `stop`) | `task`, `detail` |
| `group_paused`, `group_resumed` | home | `by` | — |
| `preempted` | home | `task`, `by` (the author of the `msg` named by `task preempted`, whose jump interrupted the task) | — |
| `cap_reached` | host, postponed after 1.0 with `settings.set` | `ai`, `cap` (`day`, `task`) | `task` |
| `rate_limited` | host | `ai`, `resets` | — |
| `login_invalid` | host | `ai` (R-56) | `detail` |
| `task_failed` | home | `task`, `reason` (`failures`, `review_rounds`) | — |
| `task_merged` | home | `task`, `commit`, `merge_commit`; shown under the secretary's name (R-24) | — |
| `no_secretary` | home | — | — |
| `session_replaced` | host | `ai` (a new session took over from notes, R-123) | `task` |

`ai` and `by` are member ids.

## 3. Card states

Only four kinds have a `state`; for every other kind it is `null`. `status`
is always present; every other key is optional and appears as listed. `by`
is the member id of the person who acted, `at` the time, `act` the signed
`act` envelope that decided the card. The `text` of an `act` (the note of
a `revise`, the free text of a `choose`) is at most 1 KiB as JSON text in
its payload; the home answers `bad_request` to a longer one. A decision
therefore adds less than 4 KiB to a card's row.

### 3.1 `brief`

| `status` | Other keys |
| --- | --- |
| `pending` | none |
| `confirmed` | `by`, `at`, `act` |
| `revise` | `by`, `at`, `act`; `note` when the person wrote one |
| `superseded` | what the state held before it was replaced |

### 3.2 `approval`

| `status` | Other keys |
| --- | --- |
| `pending` | none |
| `approved`, `denied` | `by`, `at`, `act` |
| `expired`, `reassigned` | `at` |

### 3.3 `decision`

| `status` | Other keys |
| --- | --- |
| `pending` | none |
| `decided` | `by`, `at`, `act`; `choice` (an option id) and `text`, at least one of the two |
| `expired` | `at`: the task it names was merged or cancelled while it was pending |

### 3.4 `call`

| `status` | Other keys |
| --- | --- |
| `waiting` | none |
| `answered` | `reply` (id of the reply message), `ms` (elapsed milliseconds) |
