# Contract, step 3: external AIs

How an agent that speaks MCP joins a group without a driver: how it is
added, how it takes an item, where it works, its tools, its states, the
prompt that sets it up, and what the browser shows. Rule: R-166. It is the
first tier of the owner's description of 2026-11-19, as the owner refined
it on 2026-11-23: the node opens one interface and the agent adapts
itself. Lane EXTERNAL of `steps/parallel-plan-step-2-3.md` section 4.2
builds it.

This file adds to the step 1 contracts and changes the meaning of none of
their shapes; section 13 lists what grows where.

- An **external AI** is an AI member whose row has `cli` `external`. The
  node never starts it. A **managed AI** is one of the four with a driver
  (`contracts/step-3/drivers.md` section 8).
- Its **runner** is a script or scheduled task that the agent wrote for
  itself. It waits for items with a command of the node and starts the
  agent once per item.
- **Waiting never keeps an AI session running** (owner, 2026-11-23). The
  runner waits; the agent runs only for an item.

Members of a group trust each other, so nothing here defends against an
agent or a runner that misbehaves.

## 1. The member (adds to `contracts/rows.md` section 2)

An AI member row like any other, with these differences:

| Field | For an external AI |
| --- | --- |
| `cli` | `external` |
| `tool` | new, optional: the owner's label for the program that runs it, such as `Cursor` or `ZCode`; 1 to 32 code points, no control characters. No managed AI has it |
| `host` | the node that added it, as for every AI. The agent runs on that computer |
| `workspace` | as for every AI: the folder it was added with, where every run works (section 4) |
| `account` | declared for this member when it is added (R-50), not for a CLI |
| `secretary` | always `false` |
| `model`, `effort` | text for people to read, within the bounds of `contracts/step-3/profiles.md` section 1. Nothing passes them to the agent |
| `tier`, `skills` | as for every AI |
| `state` | `working`, `waiting`, `paused` or `offline` (section 6) |
| `usage` | what `report_usage` last said (section 5); `{}` before that and after the node restarts |

```json
{"r":"member","id":"1PC8WineBrUQ-apCjx7tug","seq":812,"g":"Qm1…","kind":"ai",
 "user":"<owner's user id>","name":"Scout","cli":"external","tool":"ZCode",
 "host":"<device id>","host_name":"workstation","workspace":"/home/dev/website",
 "secretary":false,
 "account":"api_key","allow":[],"approvers":["<user id>"],
 "state":"waiting","since":1763599000000,"path":"local",
 "usage":{"window_pct":40,"window_resets":1763611200000},
 "model":"","effort":"","tier":"standard","skills":"Rust, reviews",
 "last_seen":1763600000000}
```

**The member token is the member's id.** The node makes the id when it
adds the member (`member add_ai`), as for every AI. It is no secret and
needs no storage of its own: an agent reaches the node only through the
local socket, which admits the owner's user and nobody else
(`contracts/local.md` section 3). So the token may sit in a configuration
file or a script. Removing the member revokes it (section 2). An agent in
two groups is two members, each with its own id and its own runner.

## 2. Adding and removing (adds to `contracts/commands.md` 1.2 and section 2)

| Command | For an external AI |
| --- | --- |
| `host.join` | `agent` `external`; also `account`, required, one of the three types, and `tool`, optional. `secretary` `true` is `bad_request`. No `agent.set` comes first. `workspace` is required and judged as for any AI (R-11; `contracts/commands.md` section 2): `host.workspaces` lists nothing for `external`, so the owner types the folder. The node sends `member add_ai` with `cli` `external`, then reports the state `offline`. The four profile fields as in `contracts/step-3/profiles.md` section 2 |
| `member add_ai` | optional `tool`; outside its bound, `bad_request` |
| `member set` | `tool` in its "any of", by the AI's owner only; `""` removes it |
| `agent.set` | `agent` `external` is `bad_request`: the account belongs to a member |
| `host.leave`, `member remove` | unchanged |
| `host.info` | new `exe`, always present: the absolute path of the node's own program. `agents` always holds one entry `external`: `cli_version` `""`, `account` `null`, `login_ok` `true`, `caps` `{}`, `driver` `{"accounts":["personal","team","api_key"],"takeover":"none"}`, `models` `[]`, and its members under `groups` |

- A group may hold several external AIs of one node. `host.leave` then
  needs `member`, as for several members of one CLI.
- **Removing is revoking.** Once the member row is gone, the node closes
  that member's MCP connections and waits, and a command or connection
  that names the id fails (section 3). Nothing else revokes.
- The home treats an external AI as any working AI: it is mentioned,
  called, assigned and made reviewer, and its items wait in the inbox.
- It is never the secretary: a secretary must be woken. The client does
  not offer it; the home has no check of its own.

## 3. Taking an item (adds to `contracts/local.md` sections 1 and 3)

One implementation in the node, behind two surfaces:

| | Command line, for a runner | MCP tool, for a session a person started |
| --- | --- | --- |
| take | `solodawn wait --member <id> [--timeout <s>]` | `next_item` |
| end | `solodawn done --member <id> --run <n> [--failed <text>]` | `done` with `run` |

### 3.1 `wait`

| Call | Does |
| --- | --- |
| without `--timeout` | blocks until an item can be handed out: for a loop |
| `--timeout 0` | looks once and returns at once: for a scheduler |
| `--timeout <s>` | blocks for at most that many seconds |

| Exit | Meaning |
| --- | --- |
| 0 | one item was handed out: one line of JSON on standard output (3.2) |
| 3 | nothing to hand out now. Nothing is printed |
| 1 | an error, in one line on standard error: no node is running, the id names no external AI of this node, or the member was removed while the command waited |
| 2 | wrong arguments |

`next_item` takes no argument and never waits: it is `--timeout 0`. Its
result is the hand-out, or `{"none":true}`.

`solodawn done` exits 0 when the node took it, also for a run that is no
longer open, and 1 or 2 as `wait` does.

On the local socket `wait` and `done` are commands like `pair`:
`{"cmd":"wait","member":"<id>","timeout":0}`, `timeout` left out to block,
and `{"cmd":"done","member":"<id>","run":4412,"failed":"exit 1"}`, `failed`
optional. The hand-out is the one `{"text":"…"}` line of the answer; an
answer without one is exit 3.

### 3.2 The hand-out

```json
{"run":4412,"kind":"task","task":"T-12",
 "folder":"/home/dev/website",
 "context":"…","text":"…"}
```

| Field | Presence | Meaning |
| --- | --- | --- |
| `run` | always | the number that ends this run: the id of its first inbox item |
| `kind` | always | `group`, `task` or `review`: the kind of session a managed AI would get these items in (`flows/agent-turn.md` A) |
| `task` | optional | `T-<num>`, for `task` and `review` |
| `folder` | always | the absolute path to work in: the member's workspace, whatever the `kind` (section 4) |
| `context` | always | what a session that knows nothing needs: the instructions of that kind of session, the lines of section 9, then what a replaced session is told (`contracts/drivers.md` sections 4 and 3) |
| `text` | always | the turn text for these items, as a managed AI gets it (`contracts/drivers.md` section 3) |

- **What is handed out** is what `flows/agent-turn.md` steps 4 and 6 give
  a managed AI in one turn: the held items of the session of the oldest
  one, checked as there, the folder check included.
- **Acknowledged when handed out**, as its step 7: the audit row
  `delivered`, then `ack`, then the hand-out is written. An item is handed
  out once and never again.
- Nothing is handed out while a run of this member is open, the group is
  paused, the item's task is `paused` or `failed`, or the owner took all
  AIs offline. A `jump` item goes first and interrupts nothing.
- A `reply` to an `ask` that is still open, and a `decided` for an
  `approve` that is still open, go to that call, as today.

### 3.3 A run

A run begins with a hand-out, and the member has one at a time. It ends by
the first of these:

| End | The node reports |
| --- | --- |
| `done` with its `run` | nothing: the turn ended |
| `done` with its `run` and `--failed` | `turn_interrupted` with `crash` and the text as `detail`; `task fail` in a task run |
| `limits.turn_minutes` after the hand-out | `turn_interrupted` with `timeout`; `task fail` in a task run. The node stops no process |
| the node restarts | as a managed turn that was `working` (`flows/agent-turn.md` E, last row) |

- A `done` whose `run` is not the open run changes nothing and succeeds.
  So a late `done` of an old run never ends a newer one, and it does not
  matter that a runner and its agent both end a run.
- **A run that died** is one of the last three rows. Its items are not
  handed out again: the failure reaches the secretary and counts as for a
  managed AI (`contracts/task-machine.md` rows 10 and 14).
- The tool `done` takes `run` only. A session reports its own failure in
  the group, with `post`.

## 4. Where it works

Decided: **the node tells the agent the folder, on the same computer.** It
is the member's workspace, the folder the owner gave when adding it, for a
run of every `kind`: a group run reads and answers and changes nothing
there, a task run works there, a review run reads the work where its turn
says it is (`contracts/drivers.md` section 3).

- The node makes and removes no folder, as for a managed AI
  (`contracts/local.md` section 7). The runner starts the agent in
  `folder`.
- An external AI has session rows as a managed AI (`session` table,
  `agent` `external`, no `cli_session`): a hand-out sets the row of its
  session `working`, the end of the run sets it `waiting`; so the folder,
  `flows/agent-turn.md` step 17 and the restart row apply unchanged.
- So `submit_task` and `submit_review` are the code of the managed AIs,
  unchanged, and the busy rule holds for an external AI's folder as for any
  (`contracts/task-machine.md` section 2).
- The node sets no level and no sandbox, and cannot keep the repository's
  own AI configuration out of a group run (R-45): the agent's own settings
  apply.

## 5. Tools (adds to `contracts/mcp-tools.md`)

`solodawn mcp --member <id>` is the shim of `contracts/local.md` section 3
under a clearer name: its opening line is `{"mcp":"<id>"}`. The node takes
a member id of one of its external AIs where it takes a session token.

**One list.** An external connection is listed the tools of every kind of
session of a working AI, and three of its own: `tools.json` gains the
member `external`. The schemas are those of a task session, where `task`
and `to` are optional.

**A call is judged by the open run.** With a run open, a call is that of a
managed session of the run's kind: its task, its folder, the place of
`post`, what must be given. With no run open, it is that of the member's
group session. A tool that kind of session is not listed fails with
`conflict`, and the text names the kind it needs.

| Tool | Needs |
| --- | --- |
| `submit_task` | a task run |
| `submit_review` | a review run |
| `start_task` | a group run, or no run |
| `post` with kind `handback` or `summary`, `propose_brief`, `set_task` | not listed: takeover and the secretary |
| every other tool of `contracts/mcp-tools.md` section 1, `kb_put`, `kb_search` | nothing |

New tools, for an external connection only:

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `next_item` | — | the hand-out (3.2), or `{"none":true}` | local: takes an item; never waits |
| `done` | `run` | `{"ok":true}` | local: ends that run (3.3) |
| `report_usage` | `window_pct` (0 to 100, may be a fraction); `window_resets` (a time, optional) | `{"ok":true}` | `member status` with that `usage` |

- Identity comes from the member id, so no tool takes a "from". The
  outbound secret check (`contracts/local.md` section 8) applies as to
  every AI.
- `report_usage` replaces what was reported before. A value outside its
  bounds is `bad_request`. Managed AIs are not listed it: their usage is
  read from their CLI's events.
- The secretary reads every AI's `usage` with `list_members`, as today;
  unit T3-15 also puts it on the member list of its turn
  (`contracts/step-3/profiles.md` section 3).

## 6. States (adds to `sessions.md` section 3)

A member is **present** while one of its commands or MCP connections is
open, and for 180 seconds after the last one ended. The first line that
holds gives the state the node reports in `member status`:

| State | When |
| --- | --- |
| `working` | a run is open; `task` is the run's task when it has one |
| `offline` | it is not present, or the owner took all AIs offline |
| `paused` | the group is paused |
| `waiting` | otherwise |

- 180 seconds covers a scheduler that looks once a minute. It is fixed,
  not a setting.
- Never `dormant`, `takeover` or `limited`. The home sets `offline` itself
  when the node's connection closes, as for every AI.
- An `offline` member keeps its items: they are handed out when its
  runner returns. The secretary does not choose an `offline` AI
  (`contracts/step-3/profiles.md` section 3).

## 7. Approvals

`approve` is listed to an external connection, with the input and result
of `contracts/mcp-tools.md` section 3 and the flow of `flows/approval.md`
at level 3. The request belongs to the open run, or to the group session
without one.

- An agent whose CLI can send its permission prompts to an MCP tool in
  that shape names `approve`; the setup prompt says so. `[verified]` only
  for Claude Code's `--permission-prompt-tool`.
- Any other agent keeps its own permission settings. The node sees none of
  its prompts.

## 8. The setup prompt

The browser shows this text for a member, with four places filled:
`{group}` and `{name}` from the rows, `{member}` the member id, `{exe}`
from `host.info`. The owner pastes it into the agent on the computer of
that node. Unit U3-6 puts it in one file of the web client; from then on
that file holds the text and is reviewed like code, as the instruction
texts are.

````text
You are joining the SoloDawn group "{group}" as the AI member "{name}".
SoloDawn runs on this computer. Its program is: {exe}
Your member id is: {member}

Set yourself up in the five steps below, then tell your owner what you
made and where the files are, and end this session. Do not run wait
without --timeout 0 here, and do not wait for the check yourself: your
runner starts you for it. Do it yourself; ask only if a step fails.

THE RULE: never wait or poll for work inside an AI session. A script or a
scheduled task waits. You are started only when there is an item, and you
exit when it is done.

1. MCP server. Add this stdio server to your own configuration, under the
   name solodawn:
     command: {exe}
     args:    mcp --member {member}
   If your CLI can send its permission prompts to an MCP tool, point it at
   the tool "approve" of this server. Otherwise your own settings apply.

2. Runner. Write a small script that does this and nothing else:
   a. item = output of:  {exe} wait --member {member}
      Exit 0: one line of JSON with run, kind, task, folder, context, text.
      Exit 3: nothing to do now. Any other exit: an error; try again in a
      minute.
   b. Start yourself non-interactively in the folder item.folder, with the
      solodawn MCP server configured. The prompt is item.context, a blank
      line, then item.text. If you resume an earlier session of yours for
      the same kind and task, item.text alone is enough.
   c. When that process has exited:
        {exe} done --member {member} --run <item.run>
      and add  --failed "<why>"  when it exited with an error.
   One process per item, never two at once. Read the JSON with a real
   parser.

3. Keep it alive, in one of two ways:
   - a loop around step 2, kept running as a user service or a background
     process (wait without --timeout blocks until there is an item);
   - a scheduled task every minute that runs step 2 once, with
     wait --timeout 0.
   Examples to adapt are below.

4. In each run: do what the item says, in that folder. Speak to the group
   only through the solodawn tools. Before you finish, if your CLI shows
   how much of your quota is used, call report_usage. Then exit.

5. Proof. Your owner's page has sent you a setup check; it is your first
   item. When your runner starts you with it, answer with the tool post in
   one line. The page shows that you were started once and answered.

Examples. YOUR_CLI stands for your own non-interactive command, reading
the prompt from standard input. A scheduled task gets a short PATH: write
absolute paths for jq and your CLI, or set PATH in the script.

POSIX shell, one item (runner.sh; it uses jq, or another JSON parser):
  S="{exe}"; M="{member}"
  item=$("$S" wait --member "$M" --timeout 0) || exit $?
  run=$(printf '%s' "$item" | jq -r .run)
  dir=$(printf '%s' "$item" | jq -r .folder)
  if (cd "$dir" && printf '%s' "$item" |
        jq -r '.context + "\n\n" + .text' | YOUR_CLI); then
    "$S" done --member "$M" --run "$run"
  else
    "$S" done --member "$M" --run "$run" --failed "exit $?"
  fi

As a loop: drop "--timeout 0", and run it as
  while :; do sh runner.sh || sleep 60; done

crontab, every minute:
  * * * * * flock -n /tmp/solodawn-{member}.lock /bin/sh /path/to/runner.sh
cron starts a new copy even while the last one runs; the lock stops that
(launchd and Task Scheduler never overlap). On macOS use launchd.

launchd (macOS), ~/Library/LaunchAgents/solodawn.{member}.plist, in outline:
  Label             solodawn.{member}
  ProgramArguments  /bin/sh, /path/to/runner.sh
  StartInterval     60
  then: launchctl bootstrap gui/$(id -u) <that file>

Windows, PowerShell (runner.ps1):
  [Console]::OutputEncoding = $OutputEncoding = [Text.UTF8Encoding]::new($false)
  $S = '{exe}'; $M = '{member}'
  $j = & $S wait --member $M --timeout 0; if ($LASTEXITCODE -ne 0) { exit }
  $i = $j | ConvertFrom-Json
  Push-Location $i.folder
  ($i.context + "`n`n" + $i.text) | YOUR_CLI; $ok = ($LASTEXITCODE -eq 0)
  Pop-Location
  if ($ok) { & $S done --member $M --run $i.run }
  else { & $S done --member $M --run $i.run --failed 'run failed' }
Task Scheduler, every minute:
  schtasks /Create /TN "SoloDawn {member}" /SC MINUTE /MO 1 /TR "powershell -NoProfile -ExecutionPolicy Bypass -File C:\path\to\runner.ps1"
````

- The prompt is English, like the instruction texts: its reader is an AI.
- Below it the page shows the server entry of step 1 alone, for an agent
  whose owner configures it by hand.

**The setup check** is an ordinary message. When `host.join` has succeeded
for an external AI, the client posts in the main line, as the owner, a
`text` message that mentions the new member:
`Setup check: answer here in one line, then finish.` The home queues its
`mention` item like any other, so it waits until the runner first asks.
One reply from the member, and not two, shows the whole path: the runner
started the agent once for the item, the agent reached the node through
MCP, and the run ended.

## 9. What every run is told

`internal/host/instructions/external.md`, a few lines that `context`
carries after the instructions of the session kind. Contents:

- You were started for this one item. Do it, then stop. Never wait or
  poll for more work in this session.
- If a runner started you, exit when you are done; the runner ends the
  run. If a person started you, call `done` with the item's `run`.
- Work in the folder you were started in.
- The node neither sandboxes nor stops you. If a tool answers `paused`,
  stop and exit.
- Before you finish, if your CLI shows how much of your quota is used,
  call `report_usage`.

The description of `next_item` says: take one item when a person asks;
never call it in a loop.

## 10. What the browser shows

- **Adding.** Under each of the owner's nodes, the dialog that adds an AI
  offers the agent `external` beside the CLIs found there: name, account
  type, tool label, the folder (typed), tier, skills, and model and level as
  text. It sends
  `host.join`, then the setup check (section 8).
- **Setup panel**, for the owner of that node, after adding and from the
  member panel at any time: which computer to paste into; the prompt with
  a copy button; the server entry alone; a button that sends the setup
  check again; and four marks read from the rows: the runner was seen
  (`state` is not `offline`), the item was taken (`working`), the member
  answered (its post after the check), the run ended (`waiting`).
- **Member card and panel.** `external` with the tool label, the state,
  the reported usage, tier and skills. No terminal button and no model
  list: `driver.takeover` is `none` and `models` is `[]`, which the
  client already follows (`contracts/step-3/drivers.md` section 6,
  `contracts/step-3/profiles.md` section 2a). Removing is the step 1
  action.
- **The secretary's member list** (`contracts/step-3/profiles.md` section
  3) shows it like any AI, with `external` where the CLI stands and the
  tool label when there is one:
  `- "Scout" · external (ZCode) on workstation · standard · waiting · Rust, reviews`

## 11. Not built

- A driver, or any start of the agent by the node. No further driver is
  built for any CLI (`contracts/step-3/drivers.md` section 1).
- A long-lived AI session that polls: `next_item` never waits.
- Terminal takeover, and watching.
- Permission levels, a sandbox, or the repository-configuration switch of
  R-45, enforced by the node.
- The owner switching its model or level: both are text.
- Interrupting it: a pause, a jump and the time limit stop no process.
- The secretary role.
- Idle close, the cap on open sessions, resuming a session by id,
  `session_replaced`, `limited`, `login_invalid`, a turn count.
- A second hand-out of an item, or a retry of a failed run.
- A secret token, a token table, or a revocation of its own.
- A new inbox item, message kind, card, error code, limit or storage
  table. The setup check is a `mention`.
- Cloud-hosted AIs through a public interface of the home
  (`steps/step-2-3.md`, "Not in the first version").

## 12. Shapes to freeze (unit P-7)

In `internal/proto`, with golden files, `wire.json` and the web schema
(`contracts/conventions.md` section 5):

- new `row-member-ai-external.json`: the row of section 1.
  `row-member-ai.json` stays, without `tool`.
- new `member-add_ai-external.json` and `member-set-tool.json`.
- new `host-join-external.json`, with `account`, `tool` and, since unit
  P-8, `workspace`.
- `host-info-answer.json` and `host-info-push.json`: `exe`, and the
  `external` entry of `agents`.
- `tools.json`: the member `external`, with `next_item`, `done` and
  `report_usage`. `approve` is not in the file, as before.
- new `handout.json`: the hand-out of 3.2 (`proto.Handout`). The web
  client only maps the file.

`member-status.json` does not change.

## 13. What grows in the other files

| File | Addition |
| --- | --- |
| `contracts/rows.md` section 2 | `cli` `external`; the optional `tool`; `usage` by report |
| `contracts/commands.md` 1.2 | `tool` in `member add_ai` and `member set` |
| `contracts/commands.md` section 2 | `host.join` with `agent` `external`, `account` and `tool`; `agent.set` refuses `external` |
| `contracts/datachannel.md` section 7 | `exe` and the `external` entry in `host.info` |
| `contracts/local.md` sections 1, 3 | `solodawn wait`, `solodawn done`, `mcp --member`; a member id where a session token stands |
| `contracts/mcp-tools.md` sections 1, 3, 5 | the list `external`; `next_item`, `done`, `report_usage`; `approve` for an external connection |
| `contracts/drivers.md` section 4 | the fifth instruction text (section 9) |
| `sessions.md` section 3 | the states of section 6 |
| `contracts/step-3/profiles.md` sections 1, 3 | `model` and `effort` as text; the member list line |
| `contracts/step-3/drivers.md` sections 1, 8 | no further driver |
| `contracts/storage.md` | nothing |

## 14. Decided, and not verified

The choices made in writing this file, and the coordinator's decisions of
2026-11-23 on them, are in `steps/parallel-plan-step-2-3.md` section 5,
question 6, and only there.

`[unverified]`, all of it until unit I3-3 has run:

- That an agent given the prompt writes a runner that works. No agent was
  tried: not DeepSeek Harness, not ZCode, not Claude Code as an external.
- The examples of section 8. Only the POSIX shell body was run, against
  stand-ins for `solodawn` and the agent; the crontab line, the launchd
  outline, the PowerShell script and the `schtasks` line were not run.
- That each agent's CLI can be started in a folder it did not open, and
  can resume a session per kind and task.
- Which CLIs besides Claude Code can route permission prompts to
  `approve`, and whether their tool timeout outlasts
  `limits.approval_wait_s`.
- That agents report their quota, and report it correctly.
- A scheduler slower than once in 180 seconds shows the member `offline`
  between two looks.
