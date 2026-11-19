# Contract: MCP tools offered to AI sessions

Each CLI session reaches its node through one MCP server named `solodawn`
(stdio, through the shim described in `contracts/local.md`). The node knows
which AI is calling from the shim's session token, so no tool takes a "from"
argument and a model cannot speak as someone else.

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
- Text arguments are at most 16 KiB.
- Each tool maps to one command or one read on the home. The node adds the
  signature and one check: text that an AI sends to the group is refused
  with `secret` when it matches the outbound secret patterns
  (`contracts/local.md` section 8, R-137).
- Which tools a session sees depends on its kind (`architecture.md` section
  6); the last column of each table says so.

## 1. Tools for every AI

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `post` | `text`; `kind` = `text` (default), `progress`, `question`; `mentions` (names); `where` = `thread` (default when the turn has a task) or `main` | `{"id":"…"}` | `post` |
| `ask` | `to` (an AI's name); `text`; `wait` = true (default) or false | `{"answer":"…"}`, or `{"pending":true,"receipt":"…"}` | `post` kind `call`; `flows/ask.md` |
| `reply` | `call` (the id shown in the turn); `text` | `{"ok":true}` | `post` kind `reply` |
| `read_thread` | `task` or `"main"`; `limit` (default 30, at most 100); `before` (id) | `{"messages":[{"id","by","kind","at","text"}],"more":true}` | `fetch` |
| `list_members` | — | `{"members":[{"name","kind","state","host","account","task","usage"}]}` | state rows |
| `list_tasks` | `state` (optional filter) | `{"tasks":[{"task":"T-12","title","state","step","assignee","reviewer","wait","idle_minutes"}]}` | state rows |
| `note_put` | `text`; `task` (optional) | `{"id":"…"}` | `note put` |
| `note_list` | `task` (optional) | `{"notes":[{"id","task","text","by","at"}]}` | state rows |
| `start_task` | `title`; `plan` (what is to be done, for the task session) | `{"task":"T-14","moved":true}`: the work continues in a new task session; end this turn | `task start`. Group sessions of working AIs only |
| `push_branch` | — | `{"ok":true,"commit":"9f2c1ab"}` | none on the home: the node pushes the task's own branch, never forced. Task sessions only, with a remote |
| `run_checks` | — | `{"ok":false,"items":[{"name":"go test ./...","ok":false,"tail":"…last 4 KiB of output…"}]}` | runs locally, then `task checked`. Task sessions only |
| `submit_task` | `summary` | `{"ok":true,"pr":31}` | the node pushes the branch and opens the pull request if there is none (with a remote), then `post` kind `report` and `task submitted`. Task sessions only |
| `submit_review` | `score` (0 to 100); `items`: list of `{text, file, line, veto}` | `{"passed":false,"round":1}` | `post` kind `review`. Review sessions only |
| `request_decision` | `question`; `options`: list of `{label, effect}`; `recommended` (index); `context`; `to` (names of people, default the requester of the task's brief) | `{"id":"…"}`; the answer arrives later as a turn | `post` kind `decision` |
| `handback_summary` | `text` | `{"ok":true}` | `post` kind `handback`; only meaningful in a hand-back turn |
| `net_status` | — | `{"paths":[{"peer":"…","path":"relay","rtt_ms":210}],"nat":{"udp":true,"mapped_ports":false,"ipv6":false},"rendezvous":[{"url":"…","ok":true}]}` | local |

Notes:

- `post` is the only way an AI speaks in the group. What the model prints
  outside tool calls stays in its own session.
- `ask` returns within `limits.ask_wait_s` (45 seconds) in every case.
- `run_checks` may run for a long time. The node sends an MCP progress
  notification every 30 seconds while it runs. `[unverified]` whether that
  keeps a tool call in the foreground past each CLI's default limit; spike
  S-2 and S-3. If it does not, `run_checks` returns `{"pending":true}` and
  the result arrives as a `checks` turn.
- `submit_task` fails with `conflict` if the last `run_checks` did not pass
  on the current head commit.
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
| `post_summary` | `task`; `text` | `{"id":"…"}` | `post` kind `summary` |

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
{"behavior":"deny","message":"Denied by the owner."}
```

The input is never echoed back in the result `[verified: large echoed inputs
fail]`. Handling is in `flows/approval.md`.

Codex has no such tool; its approval requests arrive on the app-server
connection and are handled by the same code in the node.

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
each kind of session: the secretary's, a working AI's group session, a task
session, a review session. The names and argument schemas in that file are
the contract; this document explains them.
