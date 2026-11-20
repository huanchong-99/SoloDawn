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
| `post` | `text`; `kind` = `text` (default), `progress`, `question`; also `handback` in a hand-back turn (the node adds the Git facts), and `summary` with `task` for the secretary after a merge; `mentions` (names); `where` = `thread` (default when the turn has a task) or `main` | `{"id":"…"}` | `post` |
| `ask` | `to` (an AI's name); `text`; `wait` = true (default) or false | `{"answer":"…"}`, or `{"pending":true,"receipt":"…"}` | `post` kind `call`; `flows/ask.md` |
| `reply` | `call` (the id shown in the turn); `text` | `{"ok":true}` | `post` kind `reply` |
| `read_thread` | `task` or `"main"`; `limit` (default 30, at most 100); `before` (the `pos` of a message from an earlier result; left out for the newest messages) | `{"messages":[{"id","pos","by","kind","at","text"}],"more":true}` | `fetch` |
| `list_members` | — | `{"members":[{"name","kind","state","host","account","task","usage"}]}` | state rows |
| `list_tasks` | `state` (optional filter) | `{"tasks":[{"task":"T-12","title","state","step","assignee","reviewer","wait","idle_minutes"}]}` | state rows |
| `note_put` | `text`; `task` (optional) | `{"id":"…"}` | `note` |
| `note_list` | `task` (optional) | `{"notes":[{"id","task","text","by","at"}]}` | `fetch` with `kind: note` |
| `start_task` | `title`; `plan` (what is to be done, for the task session) | `{"task":"T-14","moved":true}`, the number being the command's result: the work continues in a new task session; end this turn | `task start`. Group sessions of working AIs only |
| `push_branch` | — | `{"ok":true,"commit":"9f2c1ab"}` | none on the home: the node pushes the task's own branch, never forced. Task sessions only, with a remote |
| `run_checks` | — | `{"ok":false,"commit":"9f2c1ab","items":[{"name":"go test ./...","ok":false,"tail":"…last 4 KiB of output…"}]}` | runs locally on the head commit, then `task checked`. Fails with `conflict` if the worktree has uncommitted changes. Task sessions only |
| `submit_task` | `summary` | `{"ok":true,"commit":"9f2c1ab","pr":31}` | the node pushes the branch and opens the pull request if there is none (with a remote), then `post` kind `report` and `task submitted` with the commit. Task sessions only |
| `submit_review` | `score` (0 to 100); `items`: list of `{text, file, line, veto}` | `{"passed":false,"round":1}`, the command's result | `post` kind `review`; the node adds the commit of the review checkout. Review sessions only |
| `request_decision` | `question`; `options`: list of `{label, effect}`; `recommended` (index); `context`; `to` (names of people, default the requester of the task's brief) | `{"id":"…"}`; the answer arrives later as a turn | `post` kind `decision` |
| `net_status` | — | `{"paths":[{"peer":"…","path":"relay","rtt_ms":210}],"nat":{"udp":true,"mapped_ports":false,"ipv6":false},"rendezvous":[{"url":"…","ok":true}]}` | local |

Notes:

- `post` is the only way an AI speaks in the group. What the model prints
  outside tool calls stays in its own session.
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

The message of a denial is `Denied by the owner.` when a person denied the
card while the CLI waited, and the sentence of `flows/approval.md` step 4
or step 12 for a request that contains a secret or that was not decided in
time.

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
because the two differ. The names and argument schemas in that file are
the contract; this document explains them.
