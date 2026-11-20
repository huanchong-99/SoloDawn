# Contract: MCP tools offered to AI sessions

Each CLI session reaches its node through one MCP server named `solodawn`
(stdio, through the shim described in `contracts/local.md`). The node knows
which AI is calling from the shim's session token, so no tool takes a "from"
argument and a model cannot speak as someone else. The token is the only
identity Claude Code gives a server; it still names the right session after
a resume and in a takeover `[verified]`. Codex also sends its thread id
with each call (`_meta`), which the node may compare.

Conventions:

- Members are named by their display name, unique in the group (`"Codex"`).
  Tasks are named `"T-12"`. An unknown name returns an error that lists the
  valid ones.
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
  last column of each table says, and for a task session on its level
  (the notes of section 1).

## 1. Tools for every AI

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `post` | `text`; `kind` = `text` (default), `progress`, `question`; also `handback` in a hand-back turn (the node adds the Git facts), and `summary` with `task` for the secretary after a merge; `mentions` (names); `where` = `thread` or `main`, by default the place of the last item of the turn: its thread, or `main` when that item is in no thread | `{"id":"…"}` | `post` |
| `ask` | `to` (an AI's name); `text`; `wait` = true (default) or false | `{"answer":"…"}`, or `{"pending":true,"receipt":"…"}` | `post` kind `call`; `flows/ask.md` |
| `reply` | `call` (the id shown in the turn); `text` | `{"ok":true}` | `post` kind `reply` |
| `read_thread` | `task` or `"main"`; `limit` (default 30, at most 100); `before` (the `pos` of a message from an earlier result; left out for the newest messages) | `{"messages":[{"id","pos","by","kind","at","text"}],"more":true}` | `fetch` |
| `list_members` | — | `{"members":[{"name","kind","state","host","account","task","usage"}]}` | state rows |
| `list_tasks` | `state` (optional filter) | `{"tasks":[{"task":"T-12","title","state","step","assignee","reviewer","wait","idle_minutes"}]}` | state rows |
| `note_put` | `text`; `task` (optional) | `{"id":"…"}` | `note` |
| `note_list` | `task` (optional) | `{"notes":[{"id","task","text","by","at"}]}` | `fetch` with `kind: note` |
| `start_task` | `title`; `plan` (what is to be done, for the task session) | `{"task":"T-14","moved":true}`, the number being the command's result: the work continues in a new task session; end this turn | `task start`. Group sessions of working AIs only |
| `push_branch` | — | `{"ok":true,"commit":"9f2c1ab"}` | none on the home: the node pushes the task's own branch, never forced. Task sessions only; listed with or without a remote, and without one it fails with `conflict` |
| `run_checks` | — | `{"ok":false,"commit":"9f2c1ab","items":[{"name":"go test ./...","ok":false,"detail":"…last 4 KiB of output…"}]}`, `detail` as on the wire (`contracts/messages.md` 2.8) | runs locally on the head commit, then `task checked`. Fails with `conflict` if the worktree has uncommitted changes. Task sessions only |
| `submit_task` | `summary` | `{"ok":true,"commit":"9f2c1ab","pr":31}` | the node pushes the branch and opens the pull request if there is none (with a remote), then `post` kind `report` and `task submitted` with the commit. Task sessions only |
| `submit_review` | `score` (0 to 100); `items`: list of `{text, file, line, veto}` | `{"passed":false,"round":1}`, the command's result | `post` kind `review`; the node adds the commit of the review checkout. Review sessions only |
| `request_decision` | `question`; `options`: list of `{label, effect}`; `recommended` (index); `context`; `to` (names of people, default the requester of the task's brief) | `{"id":"…"}`; the answer arrives later as a turn | `post` kind `decision` |
| `net_status` | — | `{"paths":[{"peer":"…","path":"relay","rtt_ms":210}],"nat":{"udp":true,"mapped_ports":false,"ipv6":false},"rendezvous":[{"url":"…","ok":true}]}` | local |

Notes:

- `post` is the only way an AI speaks in the group. What the model prints
  outside tool calls stays in its own session.
- An unknown member name or task name fails with `not_found`, listing the
  group's valid names.
- `list_members` orders entries by `name`: AI rows include `state`, `host`
  (`host_name`, or `host` when empty), `account`, `task` (`"T-<num>"`,
  omitted when unset) and `usage` (`{}` when empty); human rows include
  `state` (`online` or `offline` from `online`) and omit the AI-only fields.
- `list_tasks` orders entries by `num` (`T-1`, `T-2`, …), omits `reviewer`
  when unset, sets `wait` to `wait.kind` or `null`, and sets `idle_minutes`
  to `0` for finished tasks (`merged`, `cancelled`) and to whole minutes
  elapsed since `thread.last.ts` (or `stats.started`) otherwise.
- `note_put` defaults an omitted `task` to the session's task in task and
  review sessions and to `null` in group sessions. `note_list` returns all
  notes of the group when `task` is omitted and filters to that task when
  given, formatting each note's `task` as `"T-<num>"` or `null`.
- `ask` returns within `limits.ask_wait_s` (45 seconds) in every case.
- `run_checks` may run for a long time, and the call stays open until the
  checks end. Claude Code kept a call of 600 seconds in the foreground,
  with or without progress notifications. Codex ended a call at 300
  seconds whether or not progress was sent, and let one of 600 seconds
  complete with `tool_timeout_sec` set to 900 `[verified]`. So the node
  sets that (`contracts/drivers.md` 1.2), sends no progress notifications,
  and has no "pending" result. Calls longer than 600 seconds are
  `[unverified]` on both; the turn time limit ends them.
- `submit_task` fails with `conflict` if the worktree has uncommitted
  changes or the last `run_checks` did not pass on the head commit.
- A task session at level 1 is not offered `push_branch`, `run_checks` or
  `submit_task` (`contracts/drivers.md` section 2).
- `push_branch` and `submit_task` are the only way a task's branch is
  pushed and its pull request opened without a card. The node does both by
  running the owner's signed-in `git` and `gh`; it never reads a token or a
  credential (`contracts/local.md` section 7).

## 2. Tools for the secretary only

Listed by the server only when the calling session is the group's secretary.

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `propose_brief` | `title`; `goal`; `criteria` (list); `scope_in`, `scope_out` (lists); `tasks`: list of `{title, assignee, reviewer, foundation, depends}`; `requester` (name, default the author of the request); `replaces` (id of an earlier brief) | `{"id":"…"}`; confirmation arrives later as a turn | `post` kind `brief` |
| `set_task` | `task`; any of `assignee`, `reviewer`; `action` = `pause`, `resume`, `cancel` | `{"ok":true}` | `task assign`, `task pause`, `task resume`, `task cancel` |

`propose_brief` has no `pass` argument: the host fills the brief's `pass`
from `group.pass`.

The secretary has the tools of section 1 too, except `start_task`,
`push_branch`, `run_checks`, `submit_task` and `submit_review` (R-21).

There is no merge tool. When checks and review have passed, the home has the
assignee's node merge, and tells the secretary afterwards (R-24). After a
merge conflict the secretary uses `set_task` to say which AI resolves it.

## 3. The permission prompt tool (Claude Code only)

`approve` is named in `--permission-prompt-tool`. The model does not call
it; the CLI does, when an action needs permission.

Input, as sent by the CLI `[verified]`:

```json
{"tool_name":"Bash","input":{"command":"git push origin task/11-docs","description":"Push the branch"},"tool_use_id":"toolu_…"}
```

Result, one text block containing exactly one of:

```json
{"behavior":"allow"}
{"behavior":"deny","message":"…"}
```

The message of a denial is the sentence `flows/approval.md` gives for the
case: step 2 at level 1; step 4 for a request that contains a secret; step
5 when the home refuses the card; `Denied by the owner.` when a person
denied the card while the CLI waited; step 12 when it was not decided in
time. Codex's `decline` carries no text (`contracts/drivers.md` 1.2).

The input is never echoed back in the result `[verified: large echoed inputs
fail]`. Handling is in `flows/approval.md`. `tool_use_id` is new on every
attempt, so a retry is recognised by `tool_name` and `input` alone.

The node lists `approve` only to a structured Claude Code session, where
the CLI hides it from the model `[verified]`. Codex has no such tool: its
approval requests arrive on the app-server connection and are handled by
the same code in the node. A session taken over in the terminal is not
given it either (`contracts/drivers.md` 1.3).

## 4. What the tools deliberately do not do

- No tool reads or writes files or runs arbitrary commands on the AI's
  behalf. The only commands the node runs are the fixed steps of the task
  flow: the project's check commands (`run_checks`), pushing the task's own
  branch and opening its pull request (`push_branch`, `submit_task`), and
  the merge the home requests. The AI uses its own CLI tools for everything
  else, under its session's permission level.
- No tool waits for a person. Decisions, confirmations and late approvals
  arrive as new turns.
- No tool returns another AI's private session content.

## 5. Tool list as the contract test sees it

`internal/proto/testdata/tools.json` holds the MCP `tools/list` result for
each kind of session: the secretary's, a working AI's group session, a
review session, and a task session twice, at level 1 and at level 2 or 3,
because the two differ. `approve` is not in the file: the node adds it for
a structured Claude Code session only (section 3). The names and argument
schemas in that file are the contract; this document explains them.

The file is one object with a member per kind of session: `secretary`,
`group` (a working AI's group session), `review`, `task_level_1` and
`task_level_2_3`. Each holds `{"tools":[…]}`, the tools in the order of the
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
- **Kinds of `post`.** `text`, `progress` and `question` everywhere;
  `handback` in the sessions a takeover can reach (the secretary's group
  session, review and task sessions; `flows/takeover.md` A), and `summary`
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
