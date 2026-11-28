# Contract: MCP tools offered to AI sessions

Each CLI session reaches its node through one MCP server named `solodawn`
(stdio, through the shim described in `contracts/local.md`). The node knows
which AI is calling from the shim's session token, so no tool takes a "from"
argument and a model cannot speak as someone else. The token is the only
identity Claude Code gives a server; it still names the right session after
a resume and in a takeover `[verified]`. Codex also sends its thread id
with each call (`_meta`), which the node may compare.

An external AI (step 3) is not started by the node and has no session
token: its member id names it. Its tool list, its three own tools
(`next_item`, `done`, `report_usage`) and how its calls are judged are in
`contracts/step-3/external.md` section 5.

Conventions:

- Members are named by their display name, unique in the group (`"Codex"`).
  Tasks are named `"T-12"`. An unknown name returns a `not_found` error that
  lists the valid ones.
- In a task or review session the task is the session's own and is not an
  argument. Where a tool takes `task` it is optional and defaults to that
  task; in a group session it must be given.
- A result is one text block containing JSON. A failure is a tool error
  whose text is `<code>: <sentence>`, with the codes of
  `contracts/datachannel.md` section 9.
- A text argument that would make a row too large fails with
  `bad_request`, and the error text names the limit and the size the row
  would have had (`contracts/conventions.md` section 4).
- Each tool maps to what the last column of its table names: commands or a
  read on the home, or work the node does locally. The node adds the
  signature and one check: text that an AI sends to the group is refused
  with `secret` when it matches the outbound secret patterns
  (`contracts/local.md` section 8, R-137).
- Which tools a session sees depends on its kind (`sessions.md`), as the
  last column of each table says. The level of a session changes no list.

## 1. Tools for every AI

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `post` | `text`; `kind` = `text` (default), `progress`, `question`; also `handback` in a hand-back turn, and `summary` with `task` for the secretary after a task is done; `mentions` (names); `where` = `thread` or `main`, by default the place of the last item of the turn: its thread, or `main` when that item is in no thread | `{"id":"…"}` | `post` |
| `ask` | `to` (an AI's name); `text`; `wait` = true (default) or false | `{"answer":"…"}`, or `{"pending":true,"receipt":"…"}`; a denied call: `contracts/step-2/confirm.md` section 5 | `post` kind `call`; `flows/ask.md` |
| `reply` | `call` (the id shown in the turn); `text` | `{"ok":true}` | `post` kind `reply` |
| `read_thread` | `task` or `"main"`; `limit` (default 30, at most 100); `before` (the `pos` of a message from an earlier result; left out for the newest messages) | `{"messages":[{"id","pos","by","kind","at","text"}],"more":true}` | `fetch` |
| `list_members` | — | `{"members":[{"name","kind","state","host","workspace","account","task","usage"}]}` | state rows |
| `list_tasks` | `state` (optional filter) | `{"tasks":[{"task":"T-12","title","state","step","assignee","reviewer","wait","idle_minutes"}]}` | state rows |
| `note_put` | `text`; `task` (optional) | `{"id":"…"}` | `note` |
| `note_list` | `task` (optional) | `{"notes":[{"id","task","text","by","at"}]}` | `fetch` with `kind: note` |
| `start_task` | `title`; `plan` (what is to be done, for the task session) | `{"task":"T-14","moved":true}`, the number being the command's result: the work continues in a new task session; end this turn. Or `{"task":"T-14","queued":true}`, when the task row the command made is `queued`: the folder is in use by another task, and this one starts by itself when that one is done; end this turn | `task start`. Group sessions of working AIs only |
| `submit_task` | `summary`: what was changed, what was run and how it ended, and where the work is | `{"ok":true}` | `post` kind `report` in the task's thread, with `summary` as its text: the submission (`contracts/task-machine.md` row 5). The node adds nothing to it. Task sessions only |
| `submit_review` | `score` (0 to 100); `items`: list of `{text, file, line, veto}` | `{"passed":false,"round":1}`, the command's result | `post` kind `review`; the node adds the id of the report that this review session was given. Review sessions only |
| `request_decision` | `question`; `options`: list of `{label, effect}`; `recommended` (index); `context`; `to` (names of people, default the group's people) | `{"id":"…"}`; the answer arrives later as a turn | `post` kind `decision` |
| `net_status` | — | `{"paths":[{"peer":"…","path":"relay","rtt_ms":210}],"nat":{"udp":true,"mapped_ports":false,"ipv6":false},"rendezvous":[{"url":"…","ok":true}]}` | local |

Notes:

- `post` is the only way an AI speaks in the group. What the model prints
  outside tool calls stays in its own session.
- The `text` of a `read_thread` message is the text a turn gives for its kind
  (`contracts/drivers.md` section 3): what its `body` and `state` hold, then
  its own text. An `approval` is `Approval (<status>) of <tool>:` and the
  request on the next line, because its turn text tells only the AI that asked
  what to do next.
- A person in `list_members` has `name` and `kind` only. `workspace` is an
  AI's folder on its `host` (`contracts/rows.md` section 2): the secretary
  and a reviewer need it to know where an AI's work is.
- `ask` returns within `limits.ask_wait_s` (45 seconds) in every case.
- A tool call may stay open for a long time. Claude Code kept a call of 600
  seconds in the foreground, with or without progress notifications. Codex
  ended a call at 300 seconds whether or not progress was sent, and let one
  of 600 seconds complete with `tool_timeout_sec` set to 900 `[verified]`.
  So the node sets that (`contracts/drivers.md` 1.2) and sends no progress
  notifications. Calls longer than 600 seconds are `[unverified]` on both;
  the turn time limit ends them.
- `submit_task` checks nothing in the folder: the node reads no working
  tree, runs no test and pushes nothing. The AI has tested and published
  its work itself, the way the project does, and says so in `summary`
  (`contracts/drivers.md` section 4). The call fails with what the home
  answers to the `post`: `conflict` when the task is not the caller's to
  submit now, `secret` before anything is sent.
- `submit_review` names the report the session was given when its `review`
  item was delivered, also after the node restarted; the home refuses a
  review of any other report (`contracts/task-machine.md` row 7).
- `net_status` reads the node's current state without probing the network:
  `paths` lists welcomed `ctl` connections that have selected a `direct` or
  `relay` path (`rtt_ms` is omitted in step 1), `nat` reports that the node's
  UDP ICE listener is bound (`udp`, always `true` in a running node), that
  router port mappings are active (`mapped_ports`, `false` in step 1) and
  whether a usable local IPv6 address is present (`ipv6`), and `rendezvous`
  lists each configured rendezvous URL with `ok: true` while its
  authenticated signalling WebSocket is open.

## 2. Tools for the secretary only

Listed by the server only when the calling session is the group's secretary.

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `propose_brief` | `title`; `goal`; `criteria` (list); `scope_in`, `scope_out` (lists); `tasks`: list of `{title, assignee, reviewer, foundation, depends}`; `requester` (name, default the author of the request); `replaces` (id of an earlier brief) | `{"id":"…"}`; confirmation arrives later as a turn (or local `conflict` if `< 2` working AIs besides the secretary) | `post` kind `brief` |
| `set_task` | `task`; any of `assignee`, `reviewer`; `action` = `pause`, `resume`, `cancel` | `{"ok":true}` | `task assign`, `task pause`, `task resume`, `task cancel` |

`propose_brief` has no `pass` argument: the host fills the brief's `pass`
from `group.pass`. When fewer than two working AIs exist besides the
secretary, the host refuses `propose_brief` locally with `conflict` before
sending any home command. It sets `mentions` to `[requester]`, and when `requester`
is omitted it defaults to the group's only person (refusing with
`bad_request` when the group has no person or more than one, naming them).
`set_task` requires at least one of `assignee`, `reviewer` or `action` (else
`bad_request`), and sends `task assign` before `task <action>` when both a
role and an action are given (noting that `task assign` succeeded if
`task <action>` fails). `request_decision` numbers options `"A"`, `"B"`, …
in list order, maps `recommended` from its 0-based index to that option's
`id` (else `bad_request`), sets `mentions` to the member ids of the
addressed people (refusing an empty `to` with `bad_request` and a non-person
or unknown name with `not_found` listing the group's people), and defaults
an omitted `to` to the group's people.

The secretary has the tools of section 1 too, except `start_task`,
`submit_task` and `submit_review` (R-21).

There is no merge tool, and nothing is merged by the platform. When a
review passes the task is done, and the home tells the secretary (R-24),
who requests the closing step the project needs in that same turn: it
asks the assignee with `ask`, or brings it to a person with
`request_decision` (`contracts/drivers.md` section 4).

## 3. The permission prompt tool (Claude Code only)

`approve` is named in `--permission-prompt-tool`. The model does not call
it directly; the CLI calls it when an action needs permission, and whenever
the model invokes `AskUserQuestion` (including at level 3,
`bypassPermissions`, per the 2026-11-21 owner override `[unverified]`).

Input, as sent by the CLI `[verified]`:

```json
{"tool_name":"Bash","input":{"command":"git push origin task/11-docs","description":"Push the branch"},"tool_use_id":"toolu_…"}
```

Result, one text block containing one of:

```json
{"behavior":"allow"}
{"behavior":"allow","updatedInput":{"questions":[…],"answers":{…}}}
{"behavior":"deny","message":"…"}
```

The message of a denial is the sentence `flows/approval.md` gives for the
case: step 2 at level 1; step 4 for a request that contains a secret; step
5 when the home refuses the card; `Denied by the owner.` when a person
denied the card while the CLI waited; step 12 when it was not decided in
time. Codex's `decline` carries no text (`contracts/drivers.md` 1.2).

For ordinary tool approvals, `updatedInput` is never returned `[verified:
large echoed inputs fail]`. Handling is in `flows/approval.md`.
`tool_use_id` is new on every attempt, so a retry is recognised by
`tool_name` and `input` alone.

When `tool_name` is `AskUserQuestion`, `input.questions` must be a non-empty
list of `{question, header, options: [{label, description}], multiSelect}`
entries with non-empty `question` texts (otherwise `bad_request`). After the
outbound secret check on every `question`, `header`, `label` and
`description`, each question is posted one at a time as a `decision` card
(`options` numbered `"A"` to `"Z"`; `context` from a non-empty `header`;
`thread` and `body.task` set for a task or review session and omitted in a
group session) and the node waits up to `limits.approval_wait_s` for its
`decided` item before posting the next question. A question with
`multiSelect: true` still uses a single-choice `decision` card; multiple
selections are given in the free-text field (`issues.md`). When all
questions are decided, `approve` returns `{"behavior":"allow",
"updatedInput":{…}}` with the original `input` fields, `answers[question]`
set to the chosen option's `label` (plus `annotations[question] =
{"notes": text}` when free text accompanied that choice) or to the custom
free-text answer when no option was chosen.

The node lists `approve` only to a structured Claude Code session, where
the CLI hides it from the model `[verified]`. Codex has no such tool: its
approval and `item/tool/requestUserInput` requests arrive on the app-server
connection and are handled by the same code in the node. A session taken
over in the terminal is not given it either (`contracts/drivers.md` 1.3).

## 4. What the tools deliberately do not do

- No tool reads or writes files or runs a command on the AI's behalf. The
  node runs no command of a project, no `git` and no `gh`
  (`contracts/local.md` section 7). The AI uses its own CLI tools for
  everything in its folder, under its session's permission level.
- No tool waits for a person. Decisions, confirmations and late approvals
  arrive as new turns.
- No tool returns another AI's private session content.

## 5. Tool list as the contract test sees it

`internal/proto/testdata/tools.json` holds the MCP `tools/list` result for
each kind of session: the secretary's, a working AI's group session, a
review session and a task session. `approve` is not in the file: the node adds it for
a structured Claude Code session only (section 3). The names and argument
schemas in that file are the contract; this document explains them.
Step 3 adds the member `external` (`contracts/step-3/external.md`
section 12).

The file is one object with a member per kind of session: `secretary`,
`group` (a working AI's group session), `review` and `task`, and
`external` from step 3. Each holds `{"tools":[…]}`, the tools in the order of the
tables above, and each tool has `name` and `inputSchema` only: a tool's
`description` is prose the node writes for the model and is not part of the
contract. An `inputSchema` is a JSON Schema of `"type":"object"` with
`properties` (`{}` for a tool without arguments), `required` when an
argument must be given, and `"additionalProperties":false`, so that an
argument the tool does not know is refused. Inside it the keywords are
`type`, `enum`, `default`, `minimum`, `maximum`, `items`, `properties`,
`required` and `additionalProperties`, and no others. The Go type is
`proto.ToolLists`; `internal/proto` and the web client both read the file,
refusing a member it does not name.

What the file settles where the tables above leave it open:

- **Where the task and the people come from.** In a group session (the
  secretary's, a working AI's) `read_thread` requires `task` and
  `request_decision` requires `to`; in a task or review session both are
  optional and default to the session's own task and to the requester of
  its brief. `note_put` and `note_list` take an optional `task` everywhere:
  in a group session a note may be about no task.
- **Kinds of `post`.** `text`, `progress`, `question` and `handback` in
  every list but `external`: a takeover can reach every kind of session,
  a working AI's group session included (`flows/takeover.md` A). `summary`
  for the secretary, whose `post` also takes `task`. A list is fixed for a
  session, so the node refuses `handback` outside a hand-back turn with
  `bad_request` (`issues.md`). `kind` defaults to
  `text`; `where` has no default, because it depends on the turn.
- **Defaults and bounds.** `ask.wait` defaults to `true`; `read_thread.limit`
  is 1 to 100, default 30, and `before` at least 1 (no `pos` is 0);
  `submit_review.score` is 0 to 100, each item requires `text`, `line` is at
  least 1 and `veto` defaults to `false`; `request_decision.options` is
  required (it may be `[]`) and each option requires `label` and `effect`;
  `recommended` is an index from 0; in `propose_brief` each task requires
  `title`, `assignee` and `reviewer`, `foundation` defaults to `false` and
  `depends` to `[]`, indexes from 0; `list_tasks.state` is one of the task
  states of `contracts/rows.md` section 4; `set_task` requires `task` only.
