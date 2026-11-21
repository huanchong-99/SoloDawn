# Spike B-04 (macOS): S-2 Claude Code driver, S-3 Codex driver, S-4 takeover

Throwaway spike. Not to be merged. Answers the S-2, S-3 and S-4 rows of
`planning/steps/step-1.md`. S-5 (Windows) is not part of this run.

Markers: **[observed]** run in this spike; **[documented]** from the CLI's own
`--help` or generated schema, not run; **[not verified]** could not be shown.

## Setup

| | |
| --- | --- |
| Claude Code | `2.1.311 (Claude Code)`, subscription sign-in, model `haiku` (`claude-haiku-5-5`) |
| Codex CLI | `codex-cli 0.164.0`, ChatGPT sign-in, model `gpt-6-luna`, effort `low`, `serviceTier: "default"` |
| System | macOS (Darwin 27.2, arm64) |
| MCP server | a 150-line Python stand-in for `solodawn mcp --token <t>` with tools `whoami` (returns its token), `post`, `approve`, and at level 2 or 3 `run_checks`, `push_branch`, `submit_task`. It logs every JSON-RPC line both ways. |
| Drivers | small Python drivers for `claude -p` stream-json and `codex app-server`; a PTY driver for the interactive CLIs |

All work ran in one temporary folder, called `$WORK` in this document and in
the captures. The spike was launched from inside a Claude Code desktop
session; its `CLAUDE*` environment variables were removed from every child
process so that the CLIs saw an ordinary login environment. The owner's own
user configuration (`~/.claude`, `~/.codex`) was **not** isolated, on purpose:
a node will run with it too. Its effects are listed where they matter.

## Captures for B-13

`captures/claude/*.jsonl` and `captures/codex/*.jsonl`. One JSON object per
line: `{"t_ms", "dir", "msg"}`. `dir` is `argv` (how the CLI was started),
`stdin` (node to CLI), `stdout` (CLI to node), `stderr`, `exit`, `mcp<-cli`
(CLI to the MCP server) or `mcp->cli` (MCP server to CLI). The first line of
each file is a `note` that says what the scenario shows. `msg` is the parsed
line exactly as sent, except for the privacy replacements below. Codex
`*/delta` notifications are removed (they are many and uniform; their shape is
in `codex/03`).

| Scenario | Claude Code | Codex |
| --- | --- | --- |
| initialisation, one turn, end of turn | `01`, `02` (project hooks running) | `01` |
| approval request and reply | `04`, `05`, `06`, `10`, `11` | `03`, `04`, `06` |
| denied | `04`, `05` turn 1 | `02` (MCP under `never`), `03` turn 1, `04` turn 1 |
| interrupt | `07` | `03` (end) |
| resume | `08`, `09`, `15` | `05`, `10` |
| usage and rate limit | every `result` and `rate_limit_event` (e.g. `01`) | `thread/tokenUsage/updated`, `account/rateLimits/updated` (e.g. `01`) |
| long calls | `10`, `11` (approve 630 s), `12` (MCP tool 600 s) | `06` (approval 630 s), `07`, `08`, `09` (MCP tool timeout) |
| level 1 tool set | `03` | `01` |

Privacy replacements: the work folder is `$WORK` (and `$WORK_ENCODED` for the
CLI's encoded project-folder name), the home folder `$HOME`, `$TMPDIR`, the
user name `<user>`, the host name `<hostname>`, the Codex installation id;
the owner's own skills, slash commands, plugins, agents and MCP servers are
replaced by numbered placeholders (shape kept); Claude's opaque thinking
`signature` values are `<signature>`. MCP tokens in the captures are the fake
`tok-…` values of the spike.

---

## S-2 Claude Code driver

### 2.1 Does `--setting-sources user` keep project hooks out? Yes [observed]

A test project had `.claude/settings.json` with `SessionStart` and
`UserPromptSubmit` hooks that touch a marker file, a `CLAUDE.md` with a
codeword, and a `.mcp.json` with a server that touches a marker file.

| Run (`claude -p … --strict-mcp-config`) | project hooks | project `CLAUDE.md` | project `.mcp.json` |
| --- | --- | --- | --- |
| without `--setting-sources` | ran (`system/hook_started`, `hook_response` in the stream; both markers) | loaded (answered the codeword) | not started |
| with `--setting-sources user` | did not run, no hook events | **not loaded** (did not know the codeword) | not started |

- `-p` skips the trust dialog, so without `--setting-sources user` a
  repository's hooks run on the first structured start [observed].
- `--setting-sources user` also drops the repository's `CLAUDE.md`. If the
  plan wants a project's own AI instructions read when repository
  configuration is off, it must pass them some other way.
- User-level configuration still loads with `user`: the owner's
  `~/.claude/CLAUDE.md` (replies came in the owner's language), user skills,
  plugins, agents and auto-memory (a level-1 session tried to `Write` into
  `~/.claude/projects/<folder>/memory/`, refused because Write was not in its
  tool set) [observed].

### 2.2 Does `--append-system-prompt` work with stream-json? Yes [observed]

"End every reply with the single word PINEAPPLE" was obeyed in a stream-json
session. Caveat [observed]: the system prompt is recorded on the first request
and reused on every resume (`--system-prompt-snapshot`, default `on`
[documented]). A later `--resume` with a different `--append-system-prompt`
("… BANANA") was ignored: the reply still ended in PINEAPPLE. Instructions are
fixed per session id. `--system-prompt-snapshot off` re-renders them
[documented, not run].

### 2.3 Effective file and network limits at levels 1 and 2

**Level 1** (`--permission-mode default --tools Read,Glob,Grep`, approve
answers deny) [observed]:

- `--permission-mode default` is accepted (init reports `"permissionMode":"default"`,
  the terminal calls it "manual mode"), although `--help` now lists the choices
  `acceptEdits, auto, bypassPermissions, manual, dontAsk, plan`.
- `init.tools` is exactly `Glob, Grep, Read` plus the MCP tools. A Write or Bash
  attempt fails with "No such tool available". No file changed.
- `Read` inside the folder works without asking; `Read` of a file outside the
  folder goes to `approve` (denied).
- **Every MCP tool call also goes to `approve`.** With approve answering deny,
  `whoami` and `post` were denied. Adding `--allowedTools mcp__solodawn`
  (server-wide) fixed it: both ran without reaching approve.
- The `approve` tool itself is hidden from the model when it is named in
  `--permission-prompt-tool`.

**Level 2** (`--permission-mode acceptEdits`, `--settings` with
`sandbox.enabled`, `autoAllowBashIfSandboxed`, `failIfUnavailable` all true,
`--allowedTools mcp__solodawn`, approve answers deny) [observed]:

| Action | Result |
| --- | --- |
| Write / Edit tool inside the folder | done, no approve call |
| Write tool or Read tool on a path outside the folder | approve call (denied) |
| Bash `echo > file` inside the folder | done in the sandbox, no approve call (`SANDBOX_RUNTIME=1`) |
| Bash `cat` of a file outside the folder, and of `/etc/hosts` | done: the sandbox does not confine reads to the folder |
| Bash write outside the folder but inside `/tmp/claude-<uid>` | done: that is the sandbox's own `TMPDIR`, always writable. The spike folder was under it, so a write denial outside the temp area could not be shown directly |
| Bash write to a folder listed in `sandbox.filesystem.denyWrite` | `operation not permitted` (enforcement shown) |
| Bash `curl https://example.com` | hangs until curl's own timeout (8 s, 20 s); no approve call, no event |
| same with `sandbox.network.allowedDomains: ["example.com"]` | goes through the proxy (curl then failed on TLS verification) |
| MCP tool | needs `--allowedTools mcp__solodawn`, as at level 1 |

The sandbox's debug line: `Applied restrictions - network: true, read:
denyAllExcept, write: denyAllExcept`. `allowUnsandboxedCommands` exists in the
binary [documented]; the models here obeyed "do not disable the sandbox", so an
unsandboxed retry reaching approve was not seen [not verified].

### 2.4 How long can an `approve` call stay open? At least 630 s [observed]

Two sessions each held one approve call 630 s and then answered allow:
one sent `notifications/progress` every 30 s (Claude passes
`_meta.progressToken` in every `tools/call`), one sent nothing. In both the
write happened and the turn ended normally. Nothing appears on stdout while
approve is open. Longer holds were not tried (spike limit 15 min).

### 2.5 A tool call that runs 10 minutes: completes [observed]

`run_checks` held 600 s, with and without progress notifications: the call
stayed in the foreground and returned its result. The CLI itself emits
`{"type":"tool_progress","heartbeat":true,"elapsed_time_seconds":30,…}` every
30 s while an MCP tool runs (capture `claude/12`). No 120-second move to the
background happened, for approve or for the tool. The CLI blocks a standalone
`sleep N` in Bash by itself ("Blocked: standalone sleep 45 …"); a 10-minute
Bash command was not tried.

### 2.6 Is the input of a retried request byte-identical? Yes, in 2 of 2 [observed]

Turn 1: approve answered `deny` with "No decision yet …". Turn 2: "Approved.
Run exactly the same request again." Raw bytes of `arguments.input` had equal
SHA-256 for a `Write` and for a `Bash` call (including the model-written
`description`). `tool_use_id` differs, so the digest must cover `tool_name` and
`input` only. The model regenerates the input; identity is likely, not
guaranteed.

### 2.7 Usage and rate-limit figures in events [observed]

- `result`: `usage` (input, output, cache read, cache creation by TTL, thinking
  tokens, `service_tier`, `speed`, `iterations`), `modelUsage` per model (tokens,
  `costUSD`, `contextWindow`, `maxOutputTokens`), `total_cost_usd`,
  `duration_api_ms`, `permission_denials`, `terminal_reason`.
- `rate_limit_event` once per turn: `status` (`allowed`), `rateLimitType`
  (`seven_day`), `resetsAt`, `unifiedWindows.seven_day.utilization`,
  `overageStatus`, `isUsingOverage`.
- `system/thinking_tokens` estimates during the turn.
- A limited or rejected status was not seen [not verified].

### 2.8 Can a session be resumed from another working folder? Yes [observed]

`--resume <id>` started in a different folder continued the session with its
history (2.1.311). The transcript stays in the original folder's project
directory under `~/.claude/projects/`.

### 2.9 Does the MCP token still identify the caller after a resume? Yes [observed]

Each start reads the `--mcp-config` it is given. After `--resume` with a new
file, `whoami` returned the new token; the same in the terminal takeover
(`tok-…-interactive`) and after it. Claude sends no session id to MCP servers
(only `_meta.claudecode/toolUseId` and `progressToken`), so the token is the
only identity.

### 2.10 Other observations

- Interrupt [observed, capture `claude/07`]: `control_request` `interrupt` →
  `control_response` `{"still_queued":[]}`, `system/task_notification`
  `stopped`, `result` `error_during_execution` with `terminal_reason`
  `aborted_tools`; the next turn works in the same process. `init.capabilities`
  has no plain `interrupt` entry; it lists `interrupt_receipt_v1`,
  `interrupt_cancel_queued_v1`, `interrupt_send_now_v1`,
  `interrupt_requeue_taken_v1` and others.
- Closing stdin ends the process with exit 0 in 0.6 to 1.2 s.
- Every run writes `~/.claude.json` (debug log) and keeps a transcript of
  250 to 500 KB even for a two-turn session.

---

## S-3 Codex driver

### 3.1 Does `-c mcp_servers.…` work on `app-server`? Yes, with two more keys [observed]

`-c mcp_servers.solodawn.command="…"` and `-c mcp_servers.solodawn.args=[…]`
start the server; it is `ready` and its tools are callable. The same flags work
on `codex resume` (interactive). Findings:

- **MCP tool calls need approval by default.** Under `approvalPolicy: never`
  the call fails: "MCP tool call requires approval, but approval policy is
  never" (capture `codex/02`). Fix: `-c
  mcp_servers.solodawn.default_tools_approval_mode="approve"`; accepted values
  are `auto`, `prompt`, `writes`, `approve` (from the CLI's own error).
- The owner's MCP servers from `~/.codex/config.toml` start too, plus built-in
  ones (`codex_apps`, `cua_repl`). `-c mcp_servers.<name>.enabled=false`
  switches a named one off.
- Each `tools/call` carries `_meta.x-codex-turn-metadata` with `thread_id`,
  `turn_id`, `model`, `sandbox_mode`, plus `threadId`, `itemId` and
  `progressToken`. The node can check the token against the thread.

### 3.2 Exact thread parameters [documented: schema; observed: those used]

`ThreadStartParams` (0.164.0): `cwd`, `model`, `modelProvider`, `serviceTier`,
`sandbox` (`read-only` | `workspace-write` | `danger-full-access`),
`approvalPolicy` (`untrusted` | `on-request` | `never` | `{granular:{…}}`),
`approvalsReviewer`, `baseInstructions`, `developerInstructions`, `config`
(any `config.toml` keys), `ephemeral`, `serviceName`, `sessionStartSource`,
`threadSource`, `personality` (deprecated). `thread/resume` takes the same
plus `threadId` and `excludeTurns`. Reasoning effort is not a thread field:
use `config.model_reasoning_effort` or `turn/start.effort`. `turn/start` can
also override `cwd`, `approvalPolicy`, `sandboxPolicy`, `model`,
`serviceTier`.

Used and observed: `cwd`, `model`, `serviceTier: "default"` (the owner's
config has `priority`), `sandbox`, `approvalPolicy`, `developerInstructions`
(obeyed), `config.model_reasoning_effort`, `config.sandbox_workspace_write`,
`config.project_doc_max_bytes`.

Limits seen per level [observed]:

| Level | Behaviour |
| --- | --- |
| 1 `read-only` + `never` | the shell tool exists and runs: `cat` inside and outside the folder worked; a write failed with `operation not permitted`. MCP works only with the approval mode above |
| 2 `workspace-write` + `on-request` | write inside the folder: no request. **Writes to `/tmp` and `$TMPDIR` are allowed by default**: a write to the `/tmp` test folder outside the workspace succeeded without a request. With `config.sandbox_workspace_write = {exclude_slash_tmp: true, exclude_tmpdir_env_var: true}` the same write failed and the model asked for escalation. Network: `curl` failed on DNS, no request |
| 3 `danger-full-access` + `never` | writes without requests |

### 3.3 Which approval request types exist?

[documented, schema] Server requests: `item/commandExecution/requestApproval`,
`item/fileChange/requestApproval`, `item/permissions/requestApproval`,
`mcpServer/elicitation/request`, `item/tool/requestUserInput`, `item/tool/call`
(dynamic tools), `account/chatgptAuthTokens/refresh`, `attestation/generate`,
and the legacy `applyPatchApproval`, `execCommandApproval`. Command decisions:
`accept`, `acceptForSession`, `acceptWithExecpolicyAmendment`,
`applyNetworkPolicyAmendment`, `decline`, `cancel`.

[observed] `item/commandExecution/requestApproval` (params: `command`, `cwd`,
`reason`, `commandActions`, `proposedExecpolicyAmendment`, ids) and
`item/fileChange/requestApproval`, whose params hold **only ids** (`reason` and
`grantRoot` null): the change itself is in the preceding `item/started`
`fileChange` item (`changes[].path`, `kind`, `diff`). `decline` and `accept`
both took effect.

### 3.4 How long can one stay open? At least 630 s [observed]

A command approval answered `accept` after 630 s: the command ran and the turn
completed (capture `codex/06`).

### 3.5 Is a retried request's input identical? Yes, in 2 of 2 [observed]

Declined, then "The owner has now approved. Run exactly the same request
again", then accepted: `command`, `cwd` and `reason` were identical; `itemId`
differs. For a file change, the request carries no content, so the digest has
to be taken over the item's `changes`; they were identical on retry.

### 3.6 Tool timeout setting [observed]

| MCP tool duration | setting | result |
| --- | --- | --- |
| 150 s | default | completed |
| 600 s | default | failed at **300 s**: "timed out awaiting tools/call after 300s" |
| 400 s, progress every 20 s | default | failed at 300 s: progress does not extend the limit |
| 600 s | `-c mcp_servers.solodawn.tool_timeout_sec=900` | completed |

MCP progress notifications are not forwarded as app-server notifications (no
`item/mcpToolCall/progress` seen).

### 3.7 Usage and rate-limit notifications [observed]

- `thread/tokenUsage/updated` after every model call: `total` and `last`, each
  with `inputTokens`, `cachedInputTokens`, `cacheWriteInputTokens`,
  `outputTokens`, `reasoningOutputTokens`, `totalTokens`; `modelContextWindow`.
- `account/rateLimits/updated` with each: `primary` (300-minute window) and
  `secondary` (10080-minute window), each `usedPercent`, `windowDurationMins`,
  `resetsAt`; `credits`, `planType`, `rateLimitReachedType`.
- A reached limit was not seen [not verified].

### 3.8 Memory of one process per session [observed]

`app-server` RSS 110 MB after `initialize`; 285 MB after two turns, plus child
processes `cua_repl` 18 MB, the MCP server 13 MB, `codex-code-mode-host`
24 MB: about 340 MB for one process with one thread.

### 3.9 Other observations

- `turn/interrupt` returns `{}` and `turn/completed` with status `interrupted`
  arrives within 10 ms; the process stays usable [observed, `codex/03`].
- **Two turns stalled** (in about 25): no notification for 170 s and more, once
  in the middle of the final answer, once before any model output. The node
  needs a watchdog that interrupts a silent turn [observed, cause unknown].
- User configuration that leaks in [observed]: the owner's `notify` program
  (set `-c notify=[]`), `service_tier = "priority"`, `~/.codex/AGENTS.md`
  (injected as a user message), user MCP servers. A project's `AGENTS.md` is
  loaded by default (the codeword was known without reading files);
  `config.project_doc_max_bytes = 0` turns it off.
- New thread ids are created by Codex; `thread.path` is the rollout file. A
  thread that never had a turn is not saved.

---

## S-4 Takeover

Each run: structured start, then the interactive CLI in a pseudo-terminal in
the same folder on the same id, then structured again. The test folders had
project hooks (Claude) or a project `AGENTS.md` (Codex).

### 4.1 Claude Code 2.1.311

Interactive command used [observed]:

```
claude --resume <id> --model <m> --mcp-config <file> --strict-mcp-config
  --setting-sources user --allowedTools mcp__solodawn  <level flags>
level 1: --permission-mode default --tools Read,Glob,Grep
level 2: --permission-mode acceptEdits --settings '{"sandbox":{…}}'
level 3: --permission-mode bypassPermissions --settings '{"skipDangerousModePermissionPrompt":true}'
```

no `--permission-prompt-tool`.

| Question | Answer |
| --- | --- |
| One session id, history intact, structured → terminal → structured | yes at levels 1, 2, 3 [observed]. The terminal showed the earlier structured turns; the structured resume listed the terminal turns |
| Trust dialog | appears on the first interactive start in each folder, also with `--setting-sources user` (default choice "No, exit"). Clicked once per folder: three entries |
| Level 1 | reading works; `whoami`, `post` work; no Write, no Bash; no file changed; `run_checks`, `push_branch` not listed (the server omits them at level 1). **But `approve` is listed to the model as an ordinary tool**, because nothing names it as the permission prompt tool in the terminal; the model reported it among its tools |
| Level 2 | Write inside the folder: done, no prompt. Write outside: terminal prompt "Do you want to create tk-out.txt? 1. Yes / 2. Yes, and always allow access to <dir> for this session / 3. No"; Esc rejected it and nothing was written |
| Level 3 survives the resume | yes: a.txt (structured), b.txt (terminal), c.txt (structured) written without prompts. Without `skipDangerousModePermissionPrompt` the terminal first shows "WARNING: Claude Code running in Bypass Permissions mode … Yes, I accept", which would store the acceptance in the user's configuration; it was declined, and passing the setting per process with `--settings` avoids it |
| Project configuration off | no project hook ran in any interactive or structured run with `--setting-sources user` |
| MCP token | the terminal's `whoami` returned the terminal's token; the structured resume returned its own |
| Brand-new session directly in the terminal with a chosen id | yes: `claude --session-id <uuid>` interactive, then `-p --resume <uuid>` remembered the turn [observed] |

On one start a second dialog appeared after the trust dialog ("No, keep
browser tools off / Yes, use my browser"). The driver's typing answered it;
`~/.claude.json` now has `claudeInChromeDefaultEnabled: false` (browser tools
off). Its earlier value is unknown.

### 4.2 Codex 0.164.0

Interactive command used [observed]:

```
codex resume <thread id> --no-alt-screen --no-daemon -C <folder> -m <model>
  -c model_reasoning_effort="low" -c service_tier="default"
  -c mcp_servers.solodawn.command=… -c mcp_servers.solodawn.args=[…]
  -c mcp_servers.solodawn.default_tools_approval_mode="approve"
  -c sandbox_workspace_write.exclude_slash_tmp=true
  -c sandbox_workspace_write.exclude_tmpdir_env_var=true
  <level 1: -s read-only -a never | level 2: -s workspace-write -a on-request | level 3: -s danger-full-access -a never>
```

| Question | Answer |
| --- | --- |
| One thread id, history intact, structured → terminal → structured | yes at levels 1, 2, 3 [observed]. No trust dialog |
| Level 1 | reading works; the MCP tool works; a write fails (`operation not permitted`); **an execute tool exists** (shell commands ran, e.g. `cat`, `whoami`). `run_checks` not listed |
| Level 2 | shell write inside the folder: no prompt. Write outside: terminal prompt "Would you like to run the following command? … 1. Yes, proceed / 2. Yes, and don't ask again for commands that start with … / 3. No (esc)"; Esc declined it, nothing written |
| Level 3 survives the resume | yes: a.txt, b.txt, c.txt without prompts |
| Project configuration off | **no by default**: the project `AGENTS.md` was used in structured and terminal runs. `project_doc_max_bytes=0` keeps it out (structured, observed); the same `-c` on the terminal was not run |
| MCP token | terminal `whoami` → terminal token; structured resume → its own token |
| Brand-new session directly in the terminal with a chosen id | **no**: there is no option to set an id, and a thread created with `thread/start` but no turn cannot be resumed ("No saved session found with ID …") [observed] |

---

## Assumptions in the contracts

### `contracts/drivers.md`

| Section | Assumption | Holds? | Proposed change |
| --- | --- | --- | --- |
| 1.1 flags | `--strict-mcp-config`, `--setting-sources user`, `--append-system-prompt`, `--permission-mode` `[documented]` | hold | mark `[verified]`. Note that `user` also drops the project `CLAUDE.md`, and that the system prompt is fixed at the first request of a session id |
| 1.1 levels | level 1 = `default` + `--tools Read,Glob,Grep`; approve answers deny | **fails for MCP**: every MCP call reaches approve and is denied | add `--allowedTools mcp__solodawn` at every level |
| 1.1 level 2 | sandbox settings give the limits | partly | file tools: folder only (else approve). Bash: reads anywhere, writes folder plus `/tmp/claude-<uid>`, network blocked with no approve and no event (a call hangs until its own timeout). Decide whether to add `sandbox.filesystem.denyRead` for secret folders and `sandbox.network.allowedDomains` |
| 1.1 level 3 | `bypassPermissions` | holds for `-p`; the terminal needs `--settings '{"skipDangerousModePermissionPrompt":true}'` | add it to the level-3 flags |
| 1.1 interrupt | "used only if `system/init` lists the capability" | the list has `interrupt_receipt_v1` and similar, no plain `interrupt` | name the capability string, or always send it (it worked) |
| 1.2 | `-c mcp_servers…` `[unverified]` | holds | mark `[verified]`; add `default_tools_approval_mode="approve"` and `tool_timeout_sec` (at least `approval_wait_s` and the check run time) |
| 1.2 thread parameters | `cwd`, model, `sandbox`, `approvalPolicy`, instructions | hold | instructions = `developerInstructions`; effort = `config.model_reasoning_effort` or `turn/start.effort`; pass `serviceTier` |
| 1.2 level 1 | read-only means no execution | **fails**: Codex always has a shell tool; reads are unrestricted | say so; level 1 on Codex is "no writes, no network", not "no execute tool" |
| 1.2 level 2 | `workspace-write` confines writes to the folder | **fails by default**: `/tmp` and `$TMPDIR` writable | add `config.sandbox_workspace_write = {exclude_slash_tmp: true, exclude_tmpdir_env_var: true}` |
| 1.2 approvals | command and file-change requests | hold | file-change params carry only ids; the digest and the card take the content from the `fileChange` item |
| 1.2 (new) | — | turns can stall silently | a watchdog: no notification for N seconds → `turn/interrupt` and report |
| 1.2 (new) | — | user config leaks in | pass `-c notify=[]`; decide about owner MCP servers (`enabled=false`) and `~/.codex/AGENTS.md`; keep project `AGENTS.md` off with `project_doc_max_bytes=0` unless repository configuration is on |
| 1.3 | exact interactive flags `[unverified]` | settled | use the two commands in 4.1 and 4.2 of this document; Codex adds `--no-daemon` |
| 1.3 | "without the permission prompt tool" | incomplete | the takeover's MCP server must not list `approve` (give the interactive start its own token kind) |
| 1.3 | trust | Claude asks once per folder, every new worktree included; Codex did not ask | state that the owner answers the trust dialog in the terminal on the first takeover of each worktree |

### `contracts/mcp-tools.md`

| Section | Assumption | Holds? | Proposed change |
| --- | --- | --- | --- |
| 1 notes, `run_checks` | progress every 30 s keeps the call in the foreground `[unverified]` | Claude: yes, up to at least 600 s (no limit hit, the CLI sends its own heartbeats). **Codex: no**, hard 300 s default, progress ignored | Codex: set `tool_timeout_sec`, or return `{"pending":true}` after less than 300 s |
| 3 approve input | `{tool_name, input, tool_use_id}` `[verified]` | holds; `tools/call` also has `_meta.claudecode/toolUseId` and `progressToken` | digest over `tool_name` + `input` only (the id changes on retry) |
| intro | the token identifies the session | holds after resume and in takeover | note that Codex additionally sends `thread_id` in `_meta` |

### `flows/approval.md` section H

- How long Claude Code lets approve stay open: at least 630 s, with or
  without progress; the 120-second background rule does not apply. The
  default `approval_wait_s` of 600 s can stay.
- Retried input byte-identical: observed for Claude (2/2) and Codex (2/2).

## Not verified

- Approve or approval holds longer than 630 s; MCP tools longer than 600 s on
  Claude Code.
- A Claude sandbox write denial outside its temp area without an explicit
  `denyWrite` (the spike folder had to be inside the sandbox's temp area).
- An unsandboxed retry (`dangerouslyDisableSandbox`) reaching approve.
- Rate-limit events with a non-`allowed` status, on both CLIs.
- `item/permissions/requestApproval` and MCP elicitation requests on Codex.
- Codex `project_doc_max_bytes=0` on the interactive CLI; Codex TUI without
  `--no-daemon`.
- Whether trusting a parent folder covers Claude worktrees below it.
- Windows (S-5).

## Leftovers on the spike machine

- `~/.claude.json`: three trust entries for subfolders of the deleted spike
  folder (`$WORK/tk`, `$WORK/tk1`, `$WORK/tk3`), and possibly
  `claudeInChromeDefaultEnabled: false` (see 4.1).
- Transcripts the CLIs wrote: `~/.claude/projects/$WORK_ENCODED-*` (deleted
  after the spike) and Codex rollouts under `~/.codex/sessions/2026/11/20/`.
