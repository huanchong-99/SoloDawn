# Issues

The issue record of this project: bugs, deviations from a requirement or from the owner's expectation, and known limitations, open and resolved. Read it before implementing or fixing anything. Add an entry for every such finding, not for every step; update the entry when it is resolved, and keep resolved entries.

Entry format: `### YYYY-MM-DD · type · title`, where type is `bug`, `deviation` (a feature does not do what was required or expected) or `limitation` (known, accepted for now). Then the symptom, the cause with its location (`file:line` where it is code), and the status (open, or resolved with the version or commit). Link a lesson, decision or postmortem instead of repeating it.

## Open

### 2026-11-21 · limitation · Windows `AF_UNIX` half-close does not always deliver EOF; a shim may outlive its CLI until the session closes

- Symptom: on Windows, `solodawn mcp --token <t>` can stay blocked in `bridgeShim`'s socket-to-stdout copy (`internal/host/shim.go:60`) after its CLI has closed `stdin`, and `TestRunShimArgumentsErrorsAndHalfClose` hung on Windows CI (`internal/host/mcp_test.go:827,908,914`) when `verifyShimUnixHalfClose` waited for end-of-input after the shim half-closed the socket.
- Cause: when `stdin` reaches EOF, `copyShimInput` calls `half.CloseWrite()` on `*net.UnixConn` (`internal/host/shim.go:74-76`, `shutdown` with `SD_SEND`) without closing the connection, and Windows `afunix.sys` occasionally fails to deliver `io.EOF` when `CloseWrite` races with a concurrent `WSARecv` on the peer (`golang/go#73140`).
- Status: open, accepted on 2026-11-21 with no product code changed per the owner's rule on rare cases. When the node closes the session (`internal/host/run.go`), closing the session kills the CLI's process tree (`detachTerminal`), which includes the shim, and `ServeMCP` closes its socket end when the session ends.

### 2026-11-21 · limitation · `AskUserQuestion` reaching `approve` under each permission mode is not yet verified by a live CLI capture

- Symptom: no spike capture or scripted fake-CLI test shows Claude Code invoking `--permission-prompt-tool mcp__solodawn__approve` when the model calls `AskUserQuestion` under `--permission-mode bypassPermissions` (level 3, the default), or honouring the `updatedInput.answers` reply (`planning/contracts/mcp-tools.md` section 3).
- Cause: the macOS spike captures (`spikes/b-04-macos/captures/claude/*.jsonl`) mention `AskUserQuestion` only in the `system/init` tool list, and `internal/host/approve_test.go` calls `Tools["approve"]` directly.
- Status: open; marked `[unverified]` in `planning/contracts/mcp-tools.md` section 3 and added to the `live` checks of `A4.1` (`B-17`) and `I-11`.

### 2026-11-21 · limitation · Claude Code `AskUserQuestion` with `multiSelect` uses a single-choice `decision` card

- Symptom: when Claude Code calls `approve` for an `AskUserQuestion` entry with `multiSelect: true`, the host posts a single-choice `decision` card (`internal/host/approve.go`, `postAndWaitDecision`) whose `act choose` carries at most one option id, so `updatedInput.answers[question]` holds one option label unless the owner enters multiple picks in the free-text field.
- Cause: `decision` cards (`planning/contracts/messages.md` section 2.4, `planning/contracts/commands.md` section 1.4) take a single `arg` option id plus optional `text`, and owner override 1 of 2026-11-21 does not add multi-select cards.
- Status: open, accepted for 1.0 (`planning/contracts/mcp-tools.md` section 3); multiple picks are given in the free-text field.

### 2026-11-21 · limitation · An immediate `submit_task` after `run_checks` can read `checks.local` before the home's row push

- Symptom: a scripted CLI that calls `submit_task` immediately after `run_checks` returns can read a stale `task` row in `Host.Rows` and fail with `conflict` (`local checks have not passed`), whereas with a real model the gap between the two tool calls is seconds.
- Cause: the home answers `task checked` before pushing the `rows` frame that carries the updated `checks.local` (`internal/home/README.md`), and `tskPrepareSubmit` (`internal/host/tools_task.go:318`) reads `checks.local` from `Host.Rows`.
- Status: open, accepted by the coordinator's decision of 2026-11-21 (#206).

### 2026-11-21 · deviation · Host delivery derives the data folder from SQLite's `pragma_database_list`

- Symptom: for an in-memory store (`:memory:`), `hostConfig.projectDir` returns `""` and delivery skips disk clone and worktree creation; for a disk store it derives `projects/<group>` from the directory of the `main` database file (`filepath.Join(filepath.Dir(dbFile), "projects", group)`), tying delivery to the store's file layout.
- Cause: `host.Host` (`internal/host/host.go:75`) has no `Dir` field yet, so `readHostConfigSQL` and `hostConfig.projectDir` (`internal/host/deliver.go`) read `file` from `pragma_database_list WHERE name = 'main'`.
- Status: open. `node` setting a `Dir` field on `Host` (HS-0 / N-4) can replace the `pragma_database_list` lookup.

### 2026-11-21 · limitation · `reportWorkspaceFailure` duplicates `member status` assembly from the session runtime

- Symptom: on a task or review workspace Git failure, `reportWorkspaceFailure` (`internal/host/deliver.go:934`) builds `member status` (`turn_interrupted`) by hand and infers `state` (`waiting` when any session of the AI is `waiting`, else `dormant`) instead of sharing the session runtime's status builder.
- Cause: HS-13 (#186) and HS-15 (#187) were built on separate branches without a shared status helper on `host.Runtime`.
- Status: open, until N-4 consolidates `member status` assembly into one helper.

### 2026-11-21 · limitation · `max_open` is enforced in the session runtime rather than in delivery

- Symptom: `deliver.go` composes and hands an eligible turn to `Runner.Turn` without counting open sessions against `max_open` itself.
- Cause: `planning/flows/agent-turn.md` step 5.1 places `max_open` (closing the longest-`waiting` session, or waiting when every open session is `working` or `takeover`) in the session runtime (`HS-15`, #187, `tryReserveSlot`), so delivery does not duplicate it; `Deliver` (`deliverNow` in `internal/host/deliver.go`) walks all held AI keys on each wake so a freed slot reaches waiting AIs.
- Status: open, coordinated between HS-13 (#186) and HS-15 (#187).

### 2026-11-21 · limitation · `member/status` refuses `cap_reached` and `rate_limited` while `HS-18` is postponed

- Symptom: a `member status` carrying host event code `cap_reached` or `rate_limited` (`planning/contracts/commands.md` section 1.9; `planning/contracts/messages.md` section 2.11) is answered `bad_request` ("unknown host event code").
- Cause: `internal/home/cmd_hostev.go` (`hevEventReaders`) handles only the three step-1 host codes that `HS-15` sends (`turn_interrupted`, `login_invalid`, `session_replaced`), per the coordinator's decision of 2026-11-21 removing caps and rate limits from the main line.
- Status: open until `HS-18` brings usage caps and rate-limit events back.

### 2026-11-21 · limitation · Reassigning a task's assignee also marks a reviewer's pending approval card `reassigned`

- Symptom: if a task's assignee changes while the task's reviewer has a pending approval card on that task, the reviewer's card becomes `reassigned` along with the old assignee's cards, so it can no longer be approved or denied and the reviewer's waiting CLI receives no decision.
- Cause: row 17 allows both the assignee and the reviewer to post approval cards on the task (`internal/home/machine.go:224`), while row 11 sets `Effects.Cards = "reassigned"` for all pending approval cards of the task whenever `Assignee` changes (`internal/home/machine.go:617`; `planning/contracts/task-machine.md` row 11) without distinguishing who asked. Hitting this requires a review session asking for approval and an assignee reassignment while that request is open.
- Status: open, accepted as a rare limitation by the coordinator's decision of 2026-11-21 (#184).

### 2026-11-21 · limitation · A member's `limited` state from `login_invalid` ends at its next completed turn, not at `agent.set`

- Symptom: after `login_invalid` puts an AI member into `limited`, fixing sign-in and sending `agent.set` sets `login_ok` back to `true` in `host.info`, while the group still shows the member as `limited` until the next item arrives and its turn completes.
- Cause: `agent.set` clears `agtPart.invalid` (`internal/host/agents.go`) and is not a session `Event`, while the member status overlay `runtimePart.limited` (`internal/host/run.go:69`, set at `:664-680`) is cleared in `finishActiveTurn` (`internal/host/run.go:473-482`) when a turn finishes. `runtimePart.isLimited` (`:682-686`) feeds member status only and holds no delivery.
- Status: open, accepted by the coordinator's decision of 2026-11-21 (`HS-3`, `HS-15`); `planning/contracts/commands.md` section 2 and `planning/flows/agent-turn.md` section E match this behaviour.

### 2026-11-21 · limitation · Claude driver matches English error text for `LoginInvalid` and `NoSession`

- Symptom: `claudeDiagEvent` (`internal/host/claude.go`) recognises an unresumable session or invalid sign-in from HTTP status `401` and English error substrings (`no conversation found`, `session not found`, `/login`, `not logged in`, `invalid api key`, `authentication`) in `result` or non-zero-exit `stderr`, so a wording change in a newer Claude Code release can report a failed turn or crash instead.
- Cause: Claude Code has no dedicated stream-json event type for sign-in or missing-session errors (see also the open entry on missing invalid-login captures).
- Status: open. Revisit error patterns when live failure captures are added.

### 2026-11-21 · limitation · Codex `default_mode_request_user_input` and `item/tool/requestUserInput` are unverified against a live session

- Symptom: Codex CLI 0.164.0 hides its `request_user_input` tool in Default mode (`codex features list` reports `default_mode_request_user_input` as `under development`, default `false`), and no spike capture covers `item/tool/requestUserInput` (`planning/contracts/drivers.md` section 1.2, step 4).
- Cause: `codexBaseFlags` (`internal/host/codex.go`) passes `-c features.default_mode_request_user_input=true` on `codex app-server` and `codex resume`, and `onUserInputRequest` maps `item/tool/requestUserInput` from the generated schema (`[documented]`), without a live run on the owner's sign-in yet.
- Status: open for the live check in B-17 / I-11 (`planning/steps/step-1-acceptance.md` A4.11). If the flag does not offer a working `request_user_input` tool in Default mode, record in `planning/contracts/drivers.md` section 1.2 that a Codex AI asks through `request_decision` (`HS-11`) instead.

### 2026-11-21 · limitation · The host has no shutdown hook for open CLI processes

- Symptom: when `solodawn node` stops on `SIGINT` or `SIGTERM`, open AI CLI processes are not closed explicitly before `node.db` closes; they end on stdin EOF when the node process exits.
- Cause: `host.Host` exposes no shutdown method, and CLI processes are started with `context.Background()` (`internal/host/run.go:380`).
- Status: open, accepted for 1.0 (`N-4` review).

### 2026-11-21 · limitation · `host.info` has no live push yet and reports `queued` as `0`

- Symptom: `Surface` (`internal/node/surface.go`) answers `host.info` requests with `HostState` and sends no `host.info` pushes (no `host.info` push yet), and every `AgentGroup.Queued` entry in `host.info` is `0` (`internal/node/surface.go:154`).
- Cause: `settings.set` is postponed with `U-12`, the 1.0 web client requests `host.info` on connect and after `agent.set`, and held items are held on the home (`not_before`, `HM-7`) rather than counted on the host.
- Status: open, accepted for 1.0 (`N-3`).

### 2026-11-20 · limitation · Self-update leaves three things to the node runtime (N-4)

- Symptom, from the independent review of `internal/node`:
  1. `Apply` checks `Busy` and then calls `Restart` (`internal/node/update.go:98`). A turn or a takeover admitted between the two is restarted.
  2. `Status` (`internal/node/update.go:40`) is written by `Check` and read for `host.info`; read from another goroutine, it races.
  3. `Restart` (`:39`) must replace the process in place (exec on Unix). launchd and systemd stop the whole process group when the node's process ends, so a child started before the exit would be stopped with it.
- Cause: the `Updater` holds no lock and does not restart by itself; the owner of the runtime is to call `Check` and `Apply` under the lock that admits turns and takeovers, copy `Status` under it, and supply a `Restart` that execs.
- Status: open, postponed off the step-1 main line (`N-6`).

### 2026-11-21 · limitation · `Sessions` handlers and runtime pairing/surface wiring across `HM-2`, `HM-3`, `N-3` and `N-4`

- Symptom: before runtime assembly, inbound `ctl` requests for `sync`, `fetch` and `host.info` were answered `bad_request` on a bare `Sessions` hub (`internal/node/session.go`), and `Pairer.Serve`, `Pairer.Known`/`Pairer.Routes`, `Pairer.HandleEvent`/`PairConfig.Paths`, `Pairer.Peer`, local socket `pair`/`pair-browser`/`open`, and `SurfaceConfig.Agents` (`(*Host).Agents`) awaited assembly in `internal/node/node.go`.
- Cause: `Sessions`, `HM-2`, `HM-3`, `HM-7`, `N-3` and `N-4` were built as separate units (`settings.set` is postponed with `U-12`).
- Status: resolved in `HM-7` (#221), `HM-2` (#205), `HM-3` (#207), `N-3` (#223) and `N-4` (#226).

### 2026-11-21 · limitation · Postponed step-2 contracts still assume an item-level `cause`

- Symptom: the step-2 planning documents (`planning/contracts/step-2/authority.md:36, 85, 92-93, 104, 108`, `planning/contracts/step-2/confirm.md:145-147, 196-223, 244`, `planning/contracts/step-2/watch-team.md:46-67` and `planning/steps/parallel-plan-step-2-3.md:20-21, 27`) require an inbox item to carry a signed `cause` when a group home belongs to another person, whereas `P-1b` removed `cause` from step-1 items.
- Cause: step 2 is postponed while step 1 ships with plain unsigned items under the owner's 2026-11-21 decision.
- Status: open, without effect in step 1. When step 2 resumes, items will need an authority field again.

### 2026-11-21 · limitation · Group and review sessions default to permission level 3 and rely on instructions not to edit files

- Symptom: at the default permission level of 3 (owner's decision of 2026-11-21), a group session or a review session starts its CLI with full tool access rather than a CLI-enforced read-only sandbox, so a model that ignores its prompt could edit files in `repo/` or `review/<num>/`.
- Cause: `readLaunchConfig` (`internal/host/run.go`) applies `perm.level` or `default_level` (default 3) to every session kind so that group and review sessions keep the CLI's ask-the-user tool and their multiple-choice questions reach the owner as decision cards (`planning/sessions.md` section 1). At level 1 Claude Code runs with `--tools Read,Glob,Grep` (`planning/contracts/drivers.md` section 1.1), which strips that tool.
- Status: open, accepted by the coordinator's decision of 2026-11-21 (#187). Each session's instructions (`planning/contracts/drivers.md` section 4) tell group and review sessions not to change code.

### 2026-11-21 · limitation · A late duplicate `TurnEnded` from a CLI can end the next turn

- Symptom: if a CLI process emits a duplicate `TurnEnded` for a finished turn and `runProcessLoop` handles that event after `Runner.Turn` has started the next turn on the same `waiting` process, the runtime ends the new turn prematurely, moves the session back to `waiting`, reports `waiting` status, and calls `Deliver` early.
- Cause: `liveProc` tracks an open turn with a boolean `turnActive` rather than a per-turn identifier (`internal/host/run.go:107`). Once `claimWaitingProc` sets `lp.turnActive = true` for the next turn (`internal/host/run.go:285`), `onProcessTurnEnded` (`internal/host/run.go:458`) passes `finishActiveTurn` (`internal/host/run.go:473`) for the stale event, clears `lp.turnActive` (`internal/host/run.go:479`), and moves the session from `working` to `waiting` (`internal/host/run.go:466`).
- Status: open, accepted on 2026-11-21 with no product code added per the owner's rule on rare cases. In production the next turn starts only after `Deliver` (`internal/host/run.go:470`) fetches held items from the home, leaving a small window for a duplicate event from the previous turn to arrive that late.

### 2026-11-21 · limitation · The jobs of a group in the home's run loop have no bound

- Symptom: not observed. A client that sends `cmd` frames faster than its group applies them makes the list of that group's jobs grow, and with it the memory of the node, until the client stops. A group applies a command in one transaction of the store, so this needs a client that sends on purpose or a store that has become slow. Ticks and events are jobs as well: while a group does not keep up, each tick of `LoopConfig.Every` and each `Event` adds one job to its list.
- Cause: `Loop.Cmd` hands a command to the goroutine of its group and returns (`internal/home/run.go`, `route`), and `Tick` and `Event` hand theirs to every group (`all`); the list (`runQueue` in `internal/home/run_conn.go`) takes every job. That is on purpose: no caller, hook or timer can then wait for a group, which is what rules out a deadlock ([decision](docs/decisions/implemented/architecture/2026-11-21-home-run-loop.md)).
- Status: open, without effect in step 1, where every client of a home is a device of its owner. The request table of the `ctl` session (N-2) is the place to limit how many requests of one connection are unanswered; a tick that finds the tick before it still waiting could be left out.

### 2026-11-21 · limitation · The limit of waiting frames does not meet a slow browser

- Symptom: not observed. The run loop gives a connection up when 1,024 frames wait for it. A `peer.Connection` never lets a frame wait: its `Send` hands the text to the data channel, which keeps it in its own send buffer without a limit. A browser that stops reading while its connection stays open so makes that buffer grow, and the loop does not see it.
- Cause: `(*Connection).Send` calls `SendText` and returns (`internal/peer/ctl.go:174`); `home.Client` has `Send` and `Done` only (`internal/home/home.go:27`), so the loop cannot ask how much is buffered. The limit (`runBacklog`, `internal/home/run_conn.go`) applies to a client whose `Send` waits, such as a local link.
- Status: open. Either `peer.Connection.Send` waits, or fails, above a buffered amount, or the node closes a connection whose buffer stays above a limit; owner N-2 or the owner of `internal/peer`.

### 2026-11-21 · limitation · A command without `g` knows only the devices of the node's owner

- Symptom: a `cmd` whose payload has no `g` is answered `bad_request` when a device of the node's owner signed it, and `unknown_device` when a device of another member of a group homed here did, although that device is known to its group.
- Cause: such a command belongs to no group, so the run loop answers it with a state that holds the owner's log only (`internal/home/run.go`, `stray`). It is the case of "A member of another group on the same home is answered `unknown_device`" below, for a payload that names no group.
- Status: open, without effect in step 1. Settled with that entry in step 2.

### 2026-11-21 · deviation · `act confirm` does not make the `replaces` check of the post again

- Symptom: `planning/contracts/commands.md` 1.4 says that a `confirm` makes "the checks of a `brief` post (1.3)" again. One of them is that `body.replaces`, when not `null`, names a brief that is `pending` or `revise`. The post itself made that brief `superseded`, so the check, made again, would refuse every confirm of a brief that replaces another.
- Cause: the contract lists the post's checks without saying which of them are about the group when the brief is confirmed. `(*State).briefStill` (`internal/home/cmd_brief.go:243`) makes the others again: the shape of the body, the requester a member, every assignee and reviewer a working AI. That an entry's assignee is not its reviewer is made again by the task machine's role rule, which `Step` answers `conflict` (`internal/home/machine.go:253`).
- Status: open, decided by unit HM-8 on 2026-11-21. The contract should say that the `replaces` check is the post's only.

### 2026-11-21 · deviation · Derived member and task ids where the home creates rows without a client id

- Symptom: `planning/contracts/rows.md` says that the ids of members, tasks and messages are random ids. The home gives the owner's `member` row in `group create` the first 16 bytes of the SHA-256 of `SoloDawn group owner <n>`, and the task of entry `i` of a confirmed brief (with its `task` message) the first 16 bytes of the SHA-256 of `SoloDawn brief task <n of the act> <i>`.
- Cause: the function of a command is pure and has no randomness (`internal/home/README.md`, "Functions are pure"); `grpOwnerID` (`internal/home/cmd_group.go:89`) and `briefTaskID` (`internal/home/cmd_brief.go:286`). Each id has the form of a random id, and nobody knows it before the command arrives, because `n` is random. An id that a task or a message already has is answered `conflict` (`internal/home/cmd_brief.go:272`).
- Status: open, decided by units HM-8 and HM-5 on 2026-11-21. Should the seam ever give functions a source of random ids, the derivations can go.

### 2026-11-21 · deviation · A `task_failed` row written beside a `checks` or `review` row has an id derived from the command's nonce

- Symptom: when `task cloud` reaches `limits.failures` (row 6 into row 14) or `post` of kind `review` exceeds `limits.review_rounds` (row 7 into row 14), the command writes two message rows in the task's thread: the `checks` or `review` row under `n`, and the `task_failed` `system` row under the first 16 bytes of the SHA-256 of `SoloDawn task failed <n>` in base64url.
- Cause: `n` already names the `checks` or `review` message row, and a command function is pure and has no randomness (`revFailedID` in `internal/home/cmd_review.go:210`).
- Status: open, decided in unit HM-11 on 2026-11-21, for the same reason as `briefTaskID` above.

### 2026-11-21 · deviation · Passed hosted checks write a `checks` message in the task's thread

- Symptom: `task cloud` with `ok: true` writes a `checks` message (`stage: "cloud"`, `ok: true`) in the task's thread, where `planning/contracts/task-machine.md` row 6 and `planning/flows/trace-task.md` step 8 mention a `checks` message only when hosted checks do not pass.
- Cause: `planning/contracts/commands.md` 1.5 and `planning/contracts/messages.md` 2.8 state that the home writes a `checks` message from `task checked` (`local`) and `task cloud` (`cloud`), and `internal/proto/testdata/task-cloud.json` carries `items` when `ok` is `true`, which `TaskRow.Checks.Cloud` does not store (`revCloud` in `internal/home/cmd_review.go:44`).
- Status: open, decided in unit HM-11 on 2026-11-21.

### 2026-11-20 · limitation · `tools.json` freezes JSON Schema keywords that the MCP SDK may write otherwise

- Symptom: `internal/proto/testdata/tools.json` (#165, P-2) freezes each tool's `inputSchema` with a fixed set of keywords: `properties` always there, `{}` for a tool without arguments, `required` only when an argument must be given, and `"additionalProperties": false` (`planning/contracts/mcp-tools.md` section 5). The node's MCP server builds its tools with `github.com/modelcontextprotocol/go-sdk` v1.8.0 and `github.com/google/jsonschema-go`, and it is not shown that they write exactly these keywords: for example `properties: {}` for a struct without fields, or a `$schema` or `title` of their own.
- Cause: the file was written from the contract, before any server code (`internal/proto/tools_test.go`, `toolLists`).
- Status: open until I-1 checks the live registered tools together; the MCP server half is settled in HS-8 (`internal/host/mcp.go`). Every tool registers a hand-written `json.RawMessage` rather than calling `jsonschema.For`, and the server takes the tools' schemas as they are (`emptyToolSchema` when empty), projecting only the per-caller differences of `mcp-tools.md` section 5 (`projectCallerSchema`); each tool's own golden test pins its schema, and `TestToolListsMatchGoldenSchemas` (`internal/host/mcp_test.go`) checks all five session lists of `tools.json` by tool name.

### 2026-11-20 · limitation · The `post` tool lists `handback` for a whole session, not for a hand-back turn

- Symptom: `tools.json` lists `handback` among the kinds of `post` in every session a takeover can reach (the secretary's, review and task sessions), because a tool list is fixed per session, not per turn. `planning/contracts/mcp-tools.md` section 1 allows it only in a hand-back turn, so a model can call it in any turn of those sessions.
- Cause: `tools/list` is read once per session (`internal/proto/tools_test.go`, `toolLists`; `planning/contracts/mcp-tools.md` section 5).
- Status: open. Terminal takeover is postponed off the main line, so `post` with kind `handback` answers `bad_request` (`internal/host/tools_talk.go`, `checkPostKind`); the hand-back path comes back with takeover.

### 2026-11-20 · limitation · A task stopped by its sixth failed review is reviewed again when it is resumed

- Symptom: after the sixth failed review a task is `failed` at step `review`. When an admin resumes or reassigns it, the reviewer is asked to review the same commit again, although that commit has just failed.
- Cause: row 7 of `contracts/task-machine.md` above the limit is row 14 alone, which leaves the step (`internal/home/machine.go:560`); section 2 then gives a task that becomes active at step `review` its `review` item (`internal/home/machine.go:428`). Row 6 is different: failed hosted checks set step `local_check` before they stop the task.
- Status: open. No change, by the coordinator's decision of 2026-11-20: the contract says so explicitly, and it lets a person resume the task with another reviewer for the same commit.

### 2026-11-20 · limitation · A child of a Windows check can start before the job holds the shell

- Symptom: on Windows a check's `cmd.exe` joins its job object only after it has started, so a process it starts in that moment is outside the job and survives the check.
- Cause: Go starts the process before `killCheckTree` assigns it (`internal/host/checks_tree_windows.go`, `newCheckJob`).
- Status: open. The fix is to start the shell with `CREATE_SUSPENDED`, assign it to the job, then resume it.

### 2026-11-20 · limitation · A key in a UTF-16 file passes the secrets check

- Symptom: a UTF-16LE file holding a private key or a token passes the secrets hard check: with `--text`, Git shows its bytes, and every second byte is NUL, so no pattern matches.
- Cause: the patterns of `local.md` §8 match ASCII text (`internal/host/secret.go`); `checkAddedLines` reads the diff as it comes.
- Status: open.

### 2026-11-20 · limitation · Files hidden by ignore rules can change what a check runs

- Symptom: a file ignored through `.git/info/exclude` or `core.excludesFile`, not through the project's `.gitignore`, is shown by neither `git status` nor `git ls-files`; it can change what a project check does, so a passing check may not describe the commit alone (L5).
- Cause: `run_checks` runs in the task's worktree (`internal/host/checks.go`, `cleanCheckHead`), where such files can exist.
- Status: open. Step 2 closes it by running the checks on a clean export of the commit.

### 2026-11-20 · bug · Two nodes starting together could each remove the other's socket

- Symptom: two nodes that started together on one data directory both ran over a stale `sock`: one removed the other's live socket, and the first one's close then removed the second's (89 of 3000 rounds in the independent review of `internal/node`).
- Cause: the dial of `sock` and its removal were not one step (`internal/node/socket.go`, before this fix).
- Status: resolved. `local.md` section 3, "One node per data directory", now holds the rule: an exclusive lock on `lock` in the data directory, taken by `ListenLocal` (`internal/node/socket.go:38`, `lockData` at `:61`) before the socket is touched.

### 2026-11-20 · limitation · The start-at-login entries are not run by a real service manager

- Symptom: the tests check the files and the task definition that `solodawn service install` writes, not a service manager starting the node from them. Not verified:
  1. a systemd user unit started by a real user manager, with the escaping of `ExecStart` (`$$`, `%%`) and the `Environment="PATH=…"` line (`internal/node/service_linux.go`);
  2. launchd loading the agent with its `EnvironmentVariables` (`internal/node/service_darwin.go`);
  3. the scheduled task running the node headless through `conhost.exe --headless` (`internal/node/service_windows.go`); Windows CI only builds the action.
- Cause: nothing is installed on the development machine, by the coordinator's decision of 2026-11-20.
- Status: open. Candidate for a CI smoke job on GitHub's throwaway runners (Linux, macOS, Windows) after the quota reset: install, log in or start the manager, check that the node runs with the expected PATH, remove.

### 2026-11-20 · limitation · `SHA256SUMS` proves the download intact, not where it came from

- Symptom: the node checks the binary against `SHA256SUMS` from the same release (`local.md` section 9). Whoever can replace the binary there can replace `SHA256SUMS` too, so the check catches a damaged or cut download, not a forged release.
- Cause: the contract has no signature over releases; binaries are not signed (decided). The download itself is https only, follows at most five redirects, each to https, is limited in size, and is given up after 60 s without data, whatever its total time (`internal/node/update.go`, `get`).
- Status: accepted trade-off of the contract.

### 2026-11-20 · bug · go-nat v0.2.0 gets four things wrong in port mapping

- Symptom: against NET-1's gateway (miniupnpd 2.3.9) in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), go-nat failed four ways. (1) NAT-PMP returns the external port it asked for, not the one the router granted: it reported 55459 where the router mapped 30000, so a node would publish an unreachable port. (2) NAT-PMP `DeletePortMapping` leaves the mapping in place. (3) NAT-PMP calls ignore the context: with the router silent, a call with a 2 s context had not returned after 10 s. (4) UPnP asks only for random external ports and fails with 606 where the router restricts ports. Discovery also always takes 5 s.
- Cause: in go-nat v0.2.0, `natpmp.go:106` and `:115` discard the mapping result; `natpmp.go:125-127` only forgets the port; `natpmp.go:43` uses `natpmp.NewClient`, which retries for about 128 s (go-nat-pmp v1.0.2 `natpmp.go:34-35`); `nat.go:123-125` draws random ports and `upnp.go:250-275` never tries `AddAnyPortMapping`; `upnp.go:152` waits 5 s. Details in [`spikes/S-8/results.md`](https://github.com/huanchong-99/SoloDawn/blob/spike/S-8-portmap/spikes/S-8/results.md), section 1.
- Status: open, external. Avoided by the coordinator's decision of 2026-11-20: PR-3 uses go-nat-pmp (`NewClientWithTimeout`) and goupnp's `internetgateway2` (IGDv2 first, then IGDv1) directly, not go-nat, and replaces the go-nat pin in `deps_test.go` with these two.

### 2026-11-20 · limitation · pion drops `tcptype` from server-reflexive candidates

- Symptom: in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), a node's mapped ICE-TCP listener published as `typ srflx ... tcptype passive` was never dialled: two symmetric NATs with UDP between them blocked and no TURN did not connect. The same endpoint published as `typ host ... tcptype passive` connected in 206 ms.
- Cause: pion ice v4.4.8 `UnmarshalCandidate` gives `tcptype` to host candidates only (`candidate_base.go:962-993`), so a srflx candidate has no TCP type and the agent creates no active candidate for it (`agent.go:1313-1323`).
- Workaround: publish a mapped TCP endpoint as `typ host ... tcptype passive` with the external address and the granted port (PR-3).
- Status: open, external; not reported upstream for now.

### 2026-11-20 · limitation · pion can select the relay while a direct pair is valid

- Symptom: in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), 5 of 63 runs with TURN, a port-mapped answerer and a restricted-cone offerer selected the relay after 2.0 s, although direct pairs were valid from about 630 ms. None of 160 runs without a mapping, none of 160 with a mapped offerer, and none with another offerer NAT type did so.
- Cause, from the ICE trace of [run 44704969215](https://github.com/huanchong-99/SoloDawn/actions/runs/44704969215): a check relayed through the answerer's TURN allocation reaches the offerer before that relay candidate is trickled, so the offerer files it as peer-reflexive and validates the pair. When the relay candidate arrives, pion ice v4.4.8 replaces the peer-reflexive candidate but keeps the pair's old priority (`agent.go:1247-1262`, `setPriorityOverride`). That pair stays the best valid one, and the controlling agent nominates only the best valid pair once its type's wait has passed, 2 s for relay (`selection.go:74-84`). Details in [`spikes/S-8/results.md`](https://github.com/huanchong-99/SoloDawn/blob/spike/S-8-portmap/spikes/S-8/results.md), section 2.
- Workaround: the connection works over the relay, and the probe of `planning/flows/connect.md` section B moves it to a direct path.
- Status: open, external; not reported upstream for now.

### 2026-11-20 · limitation · Three peer scenarios time out on a Mac with its firewall on

- Symptom: "Both nodes can offer over the same two signalling paths", "A replayed offer is not processed" and "Early and repeated candidates" fail with `context deadline exceeded` after 20 s on macOS with the application firewall enabled, on `rebuild` as merged (815c52aa6) and with the audit fixes; the other 77 scenarios pass. They pass on Linux in CI, and on GitHub's `macos-latest` runner (#145).
- Cause, not verified: `features/peer_test.go` makes each node's sockets with `peer.Listen(0, 0)`, which listens on every interface (`internal/peer/sockets.go:22-23`), and the firewall holds incoming traffic to the unsigned test binary on addresses other than loopback. The package's own tests keep the sockets on loopback (`internal/peer/sockets.go:26-27`).
- Status: open. Bind the acceptance nodes to loopback as the unit tests do, or exclude the scenarios on macOS by tag.

### 2026-11-20 · limitation · Rendezvous steps reserve a port by closing it

- Symptom: not observed. The steps listen on 127.0.0.1:0, close the listener and give its port to `solodawn rendezvous`; another process can bind the port in between, and `rvListening` would take that process for the rendezvous.
- Cause: `features/rendezvous_test.go:162-169` and `features/relay_test.go:89-94`; the command cannot take a listener or report the port it chose.
- Status: open, until the command can listen on port 0 and print its address.

### 2026-11-20 · limitation · Three web unit tests time out on a fully busy machine

- Symptom: with 14 busy loops on 12 cores, `web/src/main.test.ts:4`, `web/src/lib/motion.test.ts:29` and `web/src/lib/prefs.test.ts:26` exceeded Vitest's 5 s test limit; one idle run right after `pnpm install` also failed two tests in two files (output not kept). CI passed.
- Cause: each is the first test of its file to import the client's modules afresh inside the test, so their transformation counts against the test's limit.
- Status: open; the gate can fail spuriously while other work loads the machine.

### 2026-11-20 · limitation · A node's socket does not fit under a long HOME on macOS

- Symptom: with a HOME of more than about 60 bytes, `solodawn node` exits with `listen unix <HOME>/Library/Application Support/solodawn/sock: bind: invalid argument`. Seen on macOS with HOME under a session's temporary directory of over 100 bytes.
- Cause: a Unix socket path holds at most 103 bytes on macOS (107 on Linux), and on macOS the socket is 41 bytes below HOME (`internal/node/dir.go:23-24`, `internal/node/socket.go:46`). The system's temporary directory there (`/var/folders/…/T/`, 49 bytes) leaves room for a short name only, so `t.TempDir()` as HOME fails. `internal/node/socket_test.go:289-298`, `features/cmd_local_test.go:29-34` and `web/e2e/nodes.ts` create their directories under `/tmp` on macOS for this reason.
- Effect: none for an ordinary home directory such as `/Users/<name>`.
- Status: open, accepted for tests; a test that sets HOME for a node on macOS keeps it under `/tmp`.

### 2026-11-20 · deviation · End-to-end steps are not in a `.spec.ts` file

- Symptom: the feature-file convention (`planning/steps/parallel-plan.md` section 2, `planning/contracts/conventions.md` section 5) puts a web slice's steps in `web/features/<slice>.spec.ts`. The steps of `features/web_e2e.feature` are in `web/features/e2e.ts`, and the acceptance gate's `vitest run features` does not run them; the `Browser end-to-end` workflow does.
- Cause: `pnpm test` and the gates use `web/vite.config.ts`, whose Vitest default pattern would run a `web/features/*.spec.ts` without the global setup, Go and the browsers that end-to-end tests need. F-01c keeps `web/features/browser-crypto.ts` out of that run the same way.
- Status: open, accepted by the coordinator on 2026-11-20. `web/vite.config.ts` excludes `e2e/**` ([#132](https://github.com/huanchong-99/SoloDawn/pull/132)), so `web/e2e/*.spec.ts`, I-8's `connect.spec.ts` included, run only under `web/e2e/config.ts`.

### 2026-11-20 · limitation · Two workflows download the same pinned browsers

- Symptom: `.github/workflows/e2e.yml` repeats the download steps of `.github/workflows/browser-vectors.yml`: Chrome for Testing 137, Edge 137 and Firefox 136 with their URLs and SHA-256 sums. A change of version has to be made in both files.
- Cause: E-0 added the end-to-end workflow beside F-01c's without changing it.
- Status: open. A composite action that both workflows use can replace the steps later (coordinator, 2026-11-20).

### 2026-11-20 · limitation · The end-to-end harness runs on macOS and Linux only

- Symptom: the global setup of `web/e2e/config.ts` throws on Windows.
- Cause: `web/e2e/nodes.ts` starts every process as the leader of its own process group, stops it with `process.kill(-pgid)`, and finds what a dead test process left with `ps`; Windows has neither.
- Status: open. The browser workflows run on Linux and macOS; I-11 decides whether end-to-end tests run on Windows.

### 2026-11-20 · limitation · Fake CLIs lack invalid-login and exhausted-limit captures

- Symptom: neither scripted CLI covers an invalid-login startup error or an exhausted rate limit.
- Cause: the [macOS driver spike](https://github.com/huanchong-99/SoloDawn/pull/57) captured signed-in sessions and rate-limit updates, not these failures.
- Status: open. Add scripts from real captures before B-14 handles either failure.

### 2026-11-20 · limitation · Web client can paint one frame in the wrong theme

- Symptom: when the theme chosen in the browser (`sd-theme` = `light` or `dark`) is the opposite of the system setting, the page can paint one frame in the system theme while it loads, then switches.
- Cause: `data-theme` is set on `<html>` by the module script after the bundle has loaded (`web/src/lib/prefs.svelte.ts:40-47`); until then `tokens.css` follows the system (`web/src/styles/tokens.css:6`). `web/index.html` has no inline script that sets it earlier.
- Status: open. No inline script for now, by the coordinator's decision of 2026-11-20 (one inline line is the usual fix; no gate measures it and a later Content-Security-Policy may forbid it).

### 2026-11-20 · limitation · Older browsers get an unstyled notice or a blank page

- Symptom: a browser below the supported versions (Chrome and Edge 137, Firefox 136, Safari 17.5; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md)) sees the upgrade notice. Chrome and Edge 123 to 136 and Firefox 120 to 135 show it styled. A browser without `light-dark()` (Chrome below 123, Firefox below 120, Safari below 17.5) shows it with default colours, and one without nested rules that start with a type selector (Chrome below 120, Firefox below 117, Safari below 17.2) also loses the layout those rules give; a browser that cannot run the bundle shows a blank page.
- Cause: the notice is mounted by the same bundle (`web/src/lib/boot.ts:32-34`) and styled by the same tokens (`web/src/styles/tokens.css`, every colour a `light-dark()` pair) and nested rules (`web/src/styles/base.css`) as the application, and the build targets the supported versions (`web/vite.config.ts:21-25`), so the CSS ships as written.
- History: until the baseline was set, `web/vite.config.ts` had no `build.target`, so Vite 8.3.8's default (`chrome111`, `edge111`, `firefox114`, `safari16.4`, `ios16.4`) applied, and Lightning CSS, which minifies the CSS for the build target, rewrote every `light-dark()` pair into two custom properties and flattened the nesting. The built notice was then styled down to those versions; only the development server served the CSS as written, which is what the first version of this entry described.
- Status: open, accepted by the coordinator on 2026-11-20 as a known limitation; restated the same day when the coordinator chose to ship the CSS as written.

### 2026-11-20 · limitation · Safari deletes the device key after seven days without a visit

- Symptom: in Safari on macOS and iOS, when the person does not interact with the SoloDawn page during seven days of Safari use, Safari deletes the origin's IndexedDB, and with it the device key once the identity module keeps it there (F-01b). The browser then starts as a new device and has to be paired again. A web app added to the home screen keeps its own count and is not affected; Lockdown Mode disables IndexedDB altogether.
- Cause: WebKit's tracking prevention deletes all script-writable storage of a site after seven days of browser use without interaction with it ([WebKit blog](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/)). Not a SoloDawn defect; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md), Consequences.
- Mitigation: F-13 shows the iOS add-to-home-screen hint. F-04 asks for persistent storage (`navigator.storage.persist()`) once the device is enrolled, after pairing; Safari grants it by heuristics such as being opened as a home screen web app ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)). `loadDevice` does not ask on first run, because Firefox answers the call with a permission prompt (`dom/quota/StorageManager.cpp`, `PersistentStoragePermissionRequest::Start`, asks through `nsContentPermissionUtils::AskPermission`; MDN, [Storage quotas and eviction criteria](https://developer.mozilla.org/en-US/docs/Web/API/Storage_API/Storage_quotas_and_eviction_criteria)).
- Status: open, accepted as a limitation of the platform.

### 2026-11-20 · limitation · Safari's verify() fails every signature over an empty message

- Symptom: WebKit 27.2 (Playwright build, macOS) returns false from `crypto.subtle.verify` for a valid Ed25519 signature over an empty message, RFC 8032 test 1 included; its own signature of the empty message is valid (OpenSSL verifies it). Chromium 159 and Firefox 159 verify both.
- Cause: in WebKit's `Source/WebCore/PAL/pal/crypto/CryptoKitShim.swift`, `isValidSignature` returns false when the data is empty. No bug report found; the first affected Safari version is unknown.
- Effect: none on the protocol; envelopes and challenges have non-empty context prefixes. F-01c asserts WebKit's false result on RFC 8032 test 1 explicitly, separately from the protocol vectors.
- Status: open, external.

### 2026-11-20 · limitation · Random ids are not checked in rows, most message bodies and the frames that read a group

- Symptom: `contracts/conventions.md` section 2 makes any text that is no random id a wrong type in every field that holds one. `proto.Decode` and `decode` check it for `n` and `g` of a signed payload, which is what the envelope check reads (`contracts/envelope.md` section 3, check 3); for `conn` of a signalling payload, which a node drops otherwise (`internal/peer/signal.go:104-109`) and so does the browser (`web/src/lib/rendezvous.ts:175-180`); and, since unit P-1, for every id in the payload of a home or host command, in the payload of an inbox item, and for `g` of `inbox` and `ack`. Of the bodies a `post` carries, a review's is checked too: its `task` is refused when it is no id, because `ReviewSigned` is a type of P-1. The other bodies are not yet: `brief`, `approval`, `decision` and `call` take any text for the ids they hold (`requester`, `assignee`, `reviewer`, `replaces`, `task`, `ai`, `to`), although the `thread`, `re` and `mentions` around them are checked. Nor is a row (`g` of a row, `thread`, `re`, `task`, `mentions`, `assignee`, a row's own `id`), nor `g` of `sync`, `reset`, `rows` and `fetch`.
- Cause: only those fields have the type (`ID` in `internal/proto/envelope.go`, `command.go` and `inbox.go`; `Id` in `web/src/lib/proto.ts`). The row types and the body types a row shares with its `post` were frozen before the type existed, and code outside `internal/proto` builds them from plain strings (`internal/store`, `internal/host`), so a change of their field types is a change in those packages too. For rows, `contracts/rows.md` says "random ids unless stated" without saying per field which values stand beside the id (the `by` of a message row may be `"system"`, a device row's `id` is `<user id>:<ver>`).
- Status: open, by the coordinator's decision of 2026-11-20: they stay as they are in P-1. A home looks up every id that a body names (`contracts/commands.md` 1.3), so a text that is no id names nothing there. The rows and bodies get the type in one change with `internal/store` and `internal/host`.

### 2026-11-20 · limitation · The shared wire test does not show where one reader is stricter about a text

- Symptom: `internal/proto/testdata/wire.json` lists, per example, the texts that are random ids, and both test suites check that such a text refuses a value that is no id. The reverse is not checked: that a text which is not listed takes any text on both sides. So a value rule that only one reader has passes unnoticed. Two are known, both in the web client alone: `conn` of a signalling payload is an `Id` (`web/src/lib/proto.ts:41`) where Go has a plain string (`internal/proto/envelope.go:27`), and a pairing room is 32 lower-case hex digits (`web/src/lib/proto.ts:130`) where Go has a plain string (`internal/proto/rendezvous.go:67`). The independent review of P-1 probed for more and found none.
- Cause: the shared changes are those the Go tags can state (left out, `null`, the type of a value, a fraction, an id, a slug); a rule a schema adds to a text (a pattern, a literal) is in no tag. The names a schema gives `t`, `op`, a post's `kind` and an event's `code` are outside the agreement on purpose (`contracts/conventions.md` section 5).
- Status: open, accepted by the coordinator on 2026-11-20 with no code change. Both known cases behave alike in the end, because the Go code that uses the field checks it itself: `internal/peer` refuses a `conn` that is no id (`internal/peer/signal.go:104-109`), and the rendezvous a room that is not 32 lower-case hex digits (`internal/rendezvous/room.go:10-19`).

### 2026-11-20 · limitation · A message cut short by a closing transport is forwarded as if it were complete

- Symptom: when a client's transport closes in the middle of a WebSocket message, the rendezvous can read the bytes received so far as one complete message. This happens between two fragments of a fragmented message, and also inside one unfragmented frame when the transport closes at the boundary of a read. If those bytes happen to form a valid frame, it is handled like any other: a `to` is forwarded as `from`. `planning/contracts/rendezvous.md` section 2 says that a frame the server cannot read closes its sender.
- Cause: `github.com/coder/websocket` v1.8.15 takes a plain `io.EOF` for the end of the message (`read.go:447-449`, `msgReader.Read`). Between fragments, the header of the next fragment fails with `io.EOF` when the transport has closed (`readFrameHeader`, `frame.go:52-58`). Inside one frame, `Conn.Read` collects the payload with `io.ReadAll`, and when no byte arrives for its next read, `io.ReadFull` returns `io.EOF`, not `io.ErrUnexpectedEOF` (`readFramePayload`, `read.go:282`). `Conn.Read` then returns the bytes so far without an error, to `internal/rendezvous/server.go:179`.
- Why it is harmless: what is forwarded is a complete, valid frame that the same authenticated sender could have sent on purpose; the sender's connection is already gone; and the receiver checks the signed envelope itself (`planning/contracts/envelope.md` section 3).
- Status: open, accepted by the coordinator on 2026-11-20 with no code change; the in-frame case was added after the rendezvous audit of the same day. Revisit if the library changes, or if the server ever acts on a frame beyond forwarding it.

### 2026-11-20 · limitation · A frame dropped for a slow receiver can leave it with stale state

- Symptom: when a receiver's queue of 128 frames is full, the rendezvous drops the next frame for it and closes neither connection (`planning/contracts/rendezvous.md` section 2). A dropped `from` or `room` is covered by its sender's offer timeout and back-off. A frame the server itself generates has no sender to send it again: after a dropped `presence` a watcher shows the wrong online state until that device's next change or the next `watch`; after a dropped `joined` a pairing member waits until it joins again; a dropped `offline`, `ice` or `quota` leaves that request unanswered.
- Cause: `client.sendFrom` drops a frame when the queue is full, holds 16 frames of its sender, or holds 32 from senders off the receiver's watch list (`internal/rendezvous/server.go:129-167`, limits at `:81-82`, queue size at `:92`).
- Status: open. Coalescing the server's own frames is not built, by the coordinator's decision of 2026-11-20.

### 2026-11-20 · limitation · The rendezvous challenge is not bound to a server

- Symptom: a malicious rendezvous, or any node that serves `/rv`, can hand a device that connects to it the nonce of another server's `challenge`, and answer that server with the device's signature. It is then signed in there as the device: the device's own connection there is replaced and closed, and the attacker receives the frames sent to the device, which shows who signals it and when, and can watch other devices under its id. It also gets the device's TURN credentials in `ready` and can spend the device's daily relay quota, cutting off the device's own relayed connections.
- Cause: the answer signs `"solodawn/rv/1\n"` and the nonce only, nothing that names the server (`planning/contracts/rendezvous.md` section 2.1; `internal/identity/key.go:75-82`, checked at `internal/rendezvous/server.go:200-210`).
- Status: open design issue, owner the coordinator, to be designed after the quota reset. The challenge format is unchanged until then.

### 2026-11-20 · limitation · A late `offline` can retire a newer unanswered offer

- Symptom: a node that offers again soon after an offer to an absent device can retire the new offer at once, as if every path had reported the target offline, and wait out a back-off before the next one.
- Cause: the rendezvous answers each `to` it cannot deliver with `{"t":"offline","k":…}`, which names the target but not the `conn` (`planning/contracts/rendezvous.md` section 2.2). `internal/peer/path.go:140-145` passes every such report to `Peer.Offline`, which takes it for each unanswered offer to that target and removes the reporting path (`internal/peer/unanswered.go:38-43`). A report for an earlier frame (the earlier offer or one of its candidates) still on its way when the next offer leaves therefore counts against the new one. The back-off of at least one second makes this rare; a presence report can end the back-off sooner (`planning/flows/connect.md` step 16).
- Status: open, accepted by the coordinator on 2026-11-20 with no contract change.

### 2026-11-20 · deviation · No receiver check or shared vector for a beacon yet

- Symptom: `planning/contracts/envelope.md` section 3, checks 4 and 5, now gives a beacon (`planning/contracts/rendezvous.md` section 4) a third set of receiver checks: no `to`, and a two-minute window. The code and the vectors know two kinds only. `OpenSignal` requires `to`, and `OpenCommand` allows 24 hours. So neither reads a beacon as the contract says.
- Cause: the rule was settled after B-02c and F-01d (#136). The identity code has the two kinds only: `internal/identity/envelope.go:30` and `:37`, and `web/src/lib/identity.ts:185-195`. `planning/contracts/vectors.md` section 3 lets `received` be `command` or `signal` only, and `internal/proto/testdata/crypto.json` has no beacon case.
- Status: open. PR-2 adds a beacon case to `received` in `vectors.md` and `crypto.json`, and a third envelope kind in both identity implementations, before it reads beacons.

### 2026-11-20 · deviation · The web client does not handle `full` yet

- Symptom: a browser that joins a pairing room that already has two members is answered `{"t":"full","room":"<room id>"}` (`planning/contracts/rendezvous.md` section 2.2), but the web client drops the frame without telling the page, so a third browser keeps waiting for `joined`.
- Cause: `full` is not among the frames the web rendezvous client reads (`web/src/lib/rendezvous.ts:21`); a frame that does not parse is ignored (`web/src/lib/rendezvous.ts:165-168`). The server sends it from `internal/rendezvous/room.go` (`join`).
- Status: open, owner W-2.

### 2026-11-20 · limitation · A8.4 does not run on Windows

- Symptom: on Windows the A8.4 scenario (`features/rendezvous_acme.feature`) cannot start its rendezvous: the binary exits with `lookup rv.localhost: no such host` (CI run 44716573698, `test (windows-latest)`). The scenario is tagged `@not-windows`, which the acceptance runner leaves out on Windows (`features/acceptance_test.go`, `tags`); Linux and macOS run it.
- Cause: the Windows resolver answers no name under `.localhost` (RFC 6761), and the rendezvous looks up its `--domain` for the relay address at startup (`internal/rendezvous/relay.go`, `relay.generator`, `net.LookupIP`). The certificate library refuses `localhost` itself, a name without a dot, so the test cannot use it instead.
- Status: open. A hosts file entry for `rv.localhost` and `turn.rv.localhost` on the Windows runner would let it run; that changes the machine, not the test.

### 2026-11-20 · limitation · A TURN alias links one device's addresses for the life of the process

- Symptom: the alias in a TURN username stays the same for a device as long as the rendezvous process runs, so whoever sees its TURN traffic (the network path to the server, or a relay peer) can tell that allocations from different IP addresses belong to one pseudonym. The alias does not reveal the device id.
- Cause: the alias is an HMAC-SHA256 of the device id under a secret drawn once per process start (`internal/rendezvous/relay.go:79-86`, secret at `:94`), and the daily quota is kept per alias, so it cannot change within a run.
- Status: open, accepted by the coordinator on 2026-11-20 after the rendezvous security review (F8).

### 2026-11-20 · limitation · Limits by address let one NAT crowd its users and a spoofer fill a victim's places

- Symptom: the relay's 128 allocations and the shared TLS port's 256 connections are counted per IPv4 address or IPv6 /64 (`planning/contracts/rendezvous.md` sections 3 and 6). Devices behind one NAT or carrier-grade NAT share one count, so busy neighbours can leave a device without relay. A sender that can forge UDP source addresses can, with TURN credentials of its own (any device key it makes gets them), create allocations from a victim's address without seeing the answers; each holds one of the victim's places for its lifetime, up to an hour, and blind refreshes can keep it.
- Cause: the server can tell sources apart only by their address (`internal/rendezvous/limits.go:41-50`, `source`; `internal/rendezvous/tls.go:192-205`, `admit`), and pion's nonces are not tied to the client's address (pion/turn v5.1.3 `internal/server/util.go:59`, `NonceHash.Generate`), so a nonce fetched from the sender's own address works for forged ones.
- Status: open, accepted by the coordinator on 2026-11-20 after the rendezvous security review (F9).

### 2026-11-20 · limitation · The host gives no notice when something `host.info` shows has changed

- Symptom: `planning/contracts/datachannel.md` section 7 pushes `host.info` whenever one of its fields changes. `open`, and `state`, `task` and `queued` of each AI in a group, come from the host's sessions, and the host tells nobody when one of them changes: whoever builds `host.info` can only read the session table again (`Sessions.All`, `internal/host/session.go:341`).
- Cause: the seam of the host (HS-0, #159) has no hook on a change of the session table (`internal/host/session.go:115`); which changes matter depends on what `host.info` is built from, which is N-3's.
- Status: open. N-3 decides whether it needs a notice or reads the table again, and asks the owner of `internal/host/session.go` for the hook if it does (coordinator, 2026-11-20).

### 2026-11-20 · limitation · A commit that fails leaves the session table ahead of the store

- Symptom: not observed. If the store fails to commit a transaction whose function succeeded (an I/O error at `COMMIT`), the session table in memory keeps the changes the store lost, until the node restarts and reads the table again.
- Cause: `Sessions.Write` puts a change into memory inside the transaction and takes it out again only when the transaction's function returns an error (`internal/host/session.go:180`, `Write`). The store reports a failed commit only after the next transaction may have started (`internal/store/store.go:76-86`, `:88-92`), and has no hook that runs before it.
- Status: open, accepted for step 1. A store hook that runs on the writer after a failed commit would close it.

### 2026-11-20 · limitation · Cards and home-written rows are one line each in the chat main line

- Symptom: in the main line of the web client a brief, an approval, a decision, a summary and a call are one outlined line that names the kind and the card's subject, with the dawn edge and "Waiting on you" when the card waits for the reader. It has no buttons: the reader cannot confirm, approve or choose there. A row the home wrote (`task`, `system`) is a quiet centred line; a `system` row shows its `code` as it is on the wire, not a sentence.
- Cause: F-07 is the row layout and text messages (`planning/steps/step-1-frontend.md`). The line is drawn in `web/src/components/chat/Message.svelte`; `subject` in `web/src/lib/groups.svelte.ts` gives the subject.
- Status: open. The card units replace the line (U-4 approvals, U-7 brief, decision and summary, U-8 task, U-9 the wording of `system` rows, U-10 calls).

### 2026-11-20 · limitation · In Safari the main line moves when a message arrives while the reader is scrolled back

- Symptom: not observed, expected from the engine. The main line is a reversed scroller, whose offset is counted from its bottom. A message that arrives at the bottom while the reader is scrolled back pushes what they read up by its height, unless the browser anchors the scroll position. Chrome and Firefox do; Safari has no scroll anchoring.
- Cause: `web/src/components/chat/Chat.svelte`, `.scroller` (`flex-direction: column-reverse`). The reversed scroller is what keeps a page added above from moving anything, in every browser.
- Status: open. No correction in script was added, because it could not be tried in Safari here.

### 2026-11-20 · limitation · A device in the logs of two people is read as the device of the lower user id

- Symptom: nothing stops a person from adding to their own log a device that another person's log already holds. A home then reads that device's commands as coming from whichever of the two people has the lower user id (`planning/contracts/commands.md`, "Who"). A member could so turn the commands of another member's device into their own, which that device's person did not ask for.
- Cause: `(*State).user` walks the logs in the order of their user ids and returns the first that holds the device as valid (`internal/home/check.go:92`). `planning/contracts/envelope.md` section 4 says nothing about one device in two logs.
- Status: open, without effect in step 1, where a group has one person. Step 2, which adds people, has to settle it.

### 2026-11-20 · limitation · A member of another group on the same home is answered `unknown_device`, not `forbidden`

- Symptom: check 2 of `planning/contracts/envelope.md` section 3 knows a device of "a member of a group it homes". A command for group A from a device whose person is a member only of group B, homed on the same node, is answered `unknown_device`, where `forbidden` is the code for a known signer that may not do this.
- Cause: `(*State).known` asks the logs of one group's `State` (`internal/home/check.go:88`), which hold the node owner's log and those of that group's people.
- Status: open, without effect in step 1, where every group on a node has the same one person. For step 2, by the coordinator's decision of 2026-11-20 after the review of #160.

### 2026-11-21 · deviation · From 768 px the top bar of an open chat names the page, not the group

- Symptom: at 768 px and wider, the shell's top bar over an open chat says "Chats", and the chat's own head with the group's name, its members and its home is below it. The design has one bar with the group's name (`web/design/chat.html`).
- Cause: the top bar's title is the page's (`page.<page>` in `web/src/App.svelte`), and no mounting point lets a page put its own head into the shell's bar (`web/src/lib/mounts.svelte.ts`). Below 768 px the chat's route has the whole screen, so its head is the only bar there.
- Status: open, for the lane UI-SHELL (U-15, U-16) or a follow-up of U-0.

### 2026-11-21 · limitation · The texts of the development-only states sheet are in the build

- Symptom: the build carries the texts of the area `states` (about 0.5 KiB), although only a development server opens `#/states`.
- Cause: `web/src/messages/files.ts` takes every message file eagerly, and the loader merges them all (`web/src/messages/texts.ts`).
- Status: open, accepted with U-0 (#170).

## Resolved

### 2026-11-21 · deviation · Step 17 session and workspace cleanup awaits a close hook from the session runtime

- Symptom: when a `rows` push marked a task `merged`, `cancelled` or reassigned, changed a task's reviewer, or marked a member `gone`, `deliver.go` did not end the session row or remove the task or review worktree.
- Cause: `planning/flows/agent-turn.md` D17 and G require closing any open CLI process and waiting for its exit (C-6) before ending the session row or deleting its working folder, and `host.Runtime` exposed `Turn` and `Answer` only.
- Status: resolved in unit `HS-23`: `Runtime.Close` (`internal/host/host.go`, `internal/host/run.go`) closes any open CLI process for a session and waits for its exit, and `cleanupGroupState` (`internal/host/deliver.go`) closes and ends obsolete task, review and gone-member sessions and removes obsolete `wt/<num>-<slug>` (plus `DeleteBranch` on `merged` and `PushBranch` on `github` handoff) and `review/<num>` worktrees idempotently across pushes and restarts.

### 2026-11-21 · limitation · Ending a session through `Sessions.End` does not close an open CLI process

- Symptom: when a session ended (`planning/flows/agent-turn.md` section D, step 17) and its caller removed the row with `Sessions.End`, an open CLI process for that session stayed in `runtimePart.procs` until its `idle_minutes` timer expired or `max_open` evicted a slot.
- Cause: `Runtime` exposed `Turn` and `Answer` only, and `Sessions.End` (`internal/host/session.go`) runs inside a `Store.Write` transaction without calling into the session runtime.
- Status: resolved in unit `HS-23`: `Runtime.Close` closes and waits for any open process outside `Store.Write`, and both step 17 cleanup (`internal/host/deliver.go`) and `host.leave` (`internal/host/join.go`) call `Runner.Close` before `Sessions.End`.

### 2026-11-21 · limitation · Background hosted-check polling sends `task cloud` once

- Symptom: if the home connection was unreachable at the moment `TaskForge.WaitHostedChecks` settled, the `task cloud` command failed and the task stayed in `cloud_check`.
- Cause: `tskPollHostedChecks` (`internal/host/tools_task.go`) calls `Host.Command` once without a separate retry loop on `home_unreachable`.
- Status: resolved in unit `HS-22`: once `tskFinishCloud` releases the claim after the failed command, the group sync on reconnect (`Attach` -> `syncOneGroup`) and any `rows` push run `afterGroupState` -> `resumeGroupHostedChecks`, which polls the `cloud_check` task again and sends `task cloud`. Test: `TestResumeHostedChecksAfterRestart`.

### 2026-11-21 · limitation · `Sessions` implements only the server side of a `ctl` connection

- Symptom: a host on one node had no outgoing `ctl` client session yet to a home on another node (sending `hello` with `kind: "host"`, reading `welcome`, exchanging `devlog` both ways, and passing pushes to `Host.Push`), so cross-node AI sessions (`A3.3`, unit `I-6`) had no transport path.
- Cause: `internal/node/session.go` (`N-2`) served inbound `ctl` connections (`Serve`); the client side of a `ctl` session was split out by the coordinator's decision of 2026-11-21.
- Status: resolved in unit `N-2b` (#222, `internal/node/client.go`: `(*Sessions).serveClient` and `(*Sessions).connectHome`, plus `attachRuntime` in `internal/host/run.go`) and wired in `N-4` (`internal/node/node.go`, `Host.Connect` -> `connectHome` -> `peer.Peer.Connect`).

### 2026-11-21 · deviation · The local link routes `cmd`, `sync` and `ack` until `fetch` lands

- Symptom: on `LocalLink`, a `fetch` request from the host was answered `bad_request`.
- Cause: `LocalLink.toHome` (`internal/node/local.go`) dispatched `cmd` to `Loop.Cmd`, `sync` to `Loop.Sync` and `ack` to `Loop.Ack`, while `Fetch` (`HM-3`, #207) landed in a separate unit.
- Status: resolved in branch `gemini/N-4` after `HM-3` (#207): `LocalLink.toHome` (`internal/node/local.go`) and `NewSessions` (`internal/node/session.go`) both route `fetch` requests to `Loop.Fetch` (`TestLocalLinkCommandAndRowPush`, `TestHomeLoopIntegrationAndDevlogForwarding`).

### 2026-11-21 · limitation · `home.Loop` keeps the `Owner` it was started with across adoption

- Symptom: when an unowned node starts `home.Loop` with `Owner: ""` and is later adopted over a pairing `ctl` session, `Logs.Owner()` becomes the adopter's user id (`internal/node/devlog.go`, `dvlAdopt`), while the running `home.Loop` still has `LoopConfig.Owner == ""` (`internal/home/run.go:65`) and refuses `group create` until the loop is restarted.
- Cause: `LoopConfig.Owner` was read as a static string when `StartLoop` constructed group states (`internal/home/run.go:175`, `:198`).
- Status: resolved in branch `gemini/N-4`: `LoopConfig.OwnerFn` (`internal/home/run.go`) reads the node's owner dynamically (`logs.Owner`), so an unowned node's `home.Loop` sees the adopted owner immediately without restarting (`TestRunDynamicOwnerFn`, `TestAdoptionRendezvousAndForegroundStop`).

### 2026-11-20 · deviation · Nothing resumes hosted checks after a node restart yet

- Symptom: after a restart, a task at step `cloud_check` is not polled again, so it stays there until something else moves it (decision D3 of the host-contract audit).
- Cause: `HostedChecksToResume` (`internal/host/forge.go`) selects the tasks to poll again and gives each its PR, commit, branch and `checks.local.at`, where the 60-minute limit counts from, but no code ran on group-state sync or `rows` pushes to call it and start `WaitHostedChecks` for each.
- Status: resolved in unit `HS-22`: `afterGroupState` (`internal/host/deliver.go`) calls `resumeGroupHostedChecks`, passing the group's task rows and local AI member ids to `HostedChecksToResume`, and `startHostedCheck` (`internal/host/tools_task.go`) claims `(group, task)` idempotently by commit in `Part[tskPart](h)` so a second `rows` push or a duplicate `submit_task` leaves a running check alone. Test: `TestResumeHostedChecksAfterRestart`.

### 2026-11-20 · deviation · `solodawn pair` does not start a node that is not running

- Symptom: `local.md` section 1 says `solodawn pair` "starts the node if needed". With no node running, `solodawn pair <secret>` fails on the dial of the local socket and pairs nothing.
- Cause: `pair` (`cmd/solodawn/main.go:61` before the fix) only sent the command through `Command.send` (`internal/node/cmd.go:40` before the fix).
- Status: resolved in branch `gemini/N-4`: `(Command).dialOrStart` (`internal/node/cmd.go`) spawns a detached `solodawn node` (`startDetached` in `proc_unix.go` / `proc_windows.go`) when `c.Cmd == "pair"` and `sock` is absent, then polls until `sock` accepts (`TestPairAutoStartsNode`).

### 2026-11-20 · limitation · End-to-end tests cannot run a node until the node runtime is wired

- Symptom: `startNet` of `web/e2e/nodes.ts` waits until each node answers on its local socket. Every node exits with status 1 and `node runtime is unavailable` right after it makes its data directory and binds the socket, so a start that awaits nodes fails with that message. The harness's own scenarios start nodes with `awaitNodes: false`. A node is started with no flags, because `solodawn node` takes none yet: it has no signalling or web port and no rendezvous address, and the harness hands over only its HOME, data directory, socket and environment.
- Cause: `startNode` was a stub that returned that error until N-4 assembled the runtime (`internal/node/socket.go:181-183` before the fix).
- Status: resolved in branch `gemini/N-4`: `startNode` points to `runForegroundNode` (`internal/node/node.go`), starting the full node runtime (`node.db`, `device.key`, `logs/node.log`, `home.Loop`, `host.Host` + `ServeMCP`, `AttachLocal`, `Logs` + `Sessions`, `peer.Peer`, embedded web UI + `/rv` with `:47800` to `:0` multi-node fallback, and local socket commands).

### 2026-11-21 · deviation · `peer.Connection` delivers `re` responses without waiting for earlier queued pushes

- Symptom: on a WebRTC `*peer.Connection`, when a remote home sends `rows` pushes immediately followed by a `synced` response (`planning/contracts/datachannel.md` section 4), `(*peer.Connection).Request` could return `synced` before the `serveClient` reader goroutine had read `rows` from `(*peer.Connection).Read` and applied them to `Host.Rows`. A caller such as `host.join` (`HS-21`) that reads `Host.Rows` immediately after `sync` returns could see `not_found` if scheduled ahead of the reader goroutine.
- Cause: `(*peer.Connection).deliver` (`internal/peer/ctl.go`) previously wrote non-`re` frames to the buffered `incoming` channel (`Read`) and wrote `re` frames directly to `waiting[re]` (`Request`) on Pion's callback goroutine, with no ordering barrier between the two channels.
- Status: resolved in #222: `(*peer.Connection).deliver` queues valid `re` responses in `incoming` alongside requests and pushes, and `(*peer.Connection).Read` hands any queued `re` frame to `response` before reading on; `(*Sessions).serveClient` (`internal/node/client.go`) starts its read loop before sending `hello`, applies `devlog` pushes at once, and holds other pushes until `Host.Attach` has returned. Test: `TestPushesDeliveredBeforeAnswer`.

### 2026-11-20 · limitation · The gate measures CRAP 6.01 for `deliver`, which Go covers fully

- Symptom: `crap.py` gave `deliver` in `internal/peer/ctl.go` a CRAP score of 6.01, above the limit of 6, while `go test -cover` reported `internal/peer` at 100% of statements.
- Cause: the empty `case <-c.Done():` of the `select` previously in `deliver` was a cover block with no statements and count 0, counted as an uncovered line at complexity 6.
- Status: resolved in #222: `queue` holds the `select`, `validFrame` checks incoming `ctl` framing, and `deliver` has complexity 2 (`lizard -C 6 -w` clean, 100.0% statement coverage).

### 2026-11-21 · deviation · Dead `cause` path in `internal/home` and `internal/store`

- Symptom: the home and store still carried, wrote and read an item `cause`, although `P-1b` re-froze `InboxItem` and `Item` without `cause` under the owner's 2026-11-21 decision (no attack-and-defence code). Nothing on the wire or on the host read it.
- Cause: `internal/home/home.go` (`Item.Cause`), `internal/home/run_group.go` (`runCause`), `internal/home/cmd_brief.go`, `internal/home/machine.go` and `internal/store/inbox.go` together with the `inbox.cause` column were built before `P-1b` dropped `cause`.
- Status: resolved in branch `gemini/HM-7`: removed `Item.Cause`, `runCause`, the dead `cause` assignments and assertions across `home`, `store` and `features/home_brief*`, and the `inbox.cause` column in `internal/store/schema.sql`.

### 2026-11-21 · deviation · Inbox items are not yet pushed in the wire `inbox` frame

- Symptom: the run loop stored an item with the seam's `Item` as JSON in the `env` column and pushed it as `home.ItemsPush`, one `home.QueuedItem` per frame, where `planning/contracts/datachannel.md` section 6 (re-frozen in P-1b as a plain unsigned `item` object) has an `inbox` frame of `InboxItem`s with optional `msg` and `task` row copies.
- Cause: the wire `item` and `inbox` frame belonged to unit HM-7; the loop had `runSeal` and `runItemFrame` (`internal/home/run_push.go`) in their place.
- Status: resolved in branch `gemini/HM-7`: `internal/home/inbox.go` encodes plain `proto.Item` JSON in `inbox.env`, copies live `msg` and `task` rows into `proto.InboxItem` when sent, batches due items per connection into `proto.Inbox` frames up to `runRoom` (61,376 bytes), implements `(*Loop).Ack` (routed from `LocalLink.toHome` and `Sessions.sesPush`), and paces excess wakes under `limits.wakes_per_hour` (`TestInboxGoldensAndRowCopies`, `TestInboxBatchesItemsToTheCtlLimit`, `TestInboxAckDeletesOnlyTheHostsOwnItem`, `TestInboxWakeUpCapHoldsAndReleasesItems`, `TestLocalLinkAckDeletesQueuedItem`).

### 2026-11-21 · limitation · A queued task's `stats.started` is the time its brief was confirmed

- Symptom: a task created queued by a confirm has `stats.started` set when it is created, and it keeps that time when the merge of its dependency makes it active. The minutes a summary counts for it include the time it waited.
- Cause: `briefCreate` sets `created` and `stats.started` to the home's time (`internal/home/cmd_brief.go:307`); row 8 of `planning/contracts/task-machine.md` does not name `stats.started`, and `Step` does not set it on a release (`internal/home/machine.go:577`). The contracts did not say when a queued task starts.
- Status: resolved in unit HM-12: when `task merged` releases a `queued` task into `active`, `mrgReleaseOne` (`internal/home/cmd_merge.go`) sets `stats.started` to the home's time (`c.Now`), and `planning/contracts/rows.md` section 4 states the rule.

### 2026-11-21 · limitation · A secretary's own hand-back summary queues a `notice` for the secretary

- Symptom: when the owner hands the secretary's terminal back and a `handback` message is posted in the main line signed for the secretary, the home queued a `notice` (`code: handback`) for `group.secretary`, waking the secretary again for the summary it or its host just posted.
- Cause: `planning/contracts/commands.md` 1.3 specified `item notice for the secretary` on `handback` without an exception when `as` is the secretary.
- Status: resolved on branch `gemini/HM-6` (#181): `post` of kind `handback` is removed from `internal/home/cmd_post.go` and marked postponed with takeover in `planning/contracts/commands.md` 1.3, and `planning/contracts/commands.md` 1.3 now states the general rule that a post never queues an item for its own author (`postWakes` skips `msg.By`; tested in `internal/home/cmd_post_test.go`).

### 2026-11-21 · deviation · `web/src/lib/peer.ts` had no method to send a `ctl` push

- Symptom: onboarding (`flows/pairing.md` step 10) and device management must push `devlog` frames (`{"t":"devlog","user":"…","entries":[…]}`) without a request `id` (`planning/contracts/conventions.md` section 3). `Connection` and `Peer` in `web/src/lib/peer.ts` exposed `request(req)` only, which always attaches an `id` and awaits a response, while `#ctl` and `#send` were private.
- Cause: unit W-2 implemented `request` and incoming `onpush`, omitting outgoing pushes (`internal/peer/ctl.go` has `Send` beside `Request`).
- Status: resolved in #212: `Connection.send` and `Peer.send` send a `Devlog` frame without `id` and enforce the 60 KiB `ctl` frame limit (`web/src/lib/peer.ts`). Tests in `web/src/lib/peer.test.ts`.

### 2026-11-20 · limitation · A task's `branch` is text that no reader checks

- Symptom: `proto.Decode` and the web client refuse a task row whose `slug` the home's rule cannot make (unit P-1), but take any text as its `branch`, and as the `branch` of `task branch`. `contracts/local.md` section 7 derives the branch from `num` and `slug` (`task/<num>-<slug>`); `HostedChecksToResume` previously copied `branch` from the row (`internal/host/forge.go:220` before `HOST-BRANCH`), from where it would reach `git` and `gh` arguments.
- Cause: `TaskRow.Branch` and `TaskBranch.Branch` are plain strings (`internal/proto/task.go`, `internal/proto/command.go`). Giving the field a type of its own changes `internal/host`, which unit P-1 does not own; and whether the home must refuse a `task branch` whose `branch` is not `task/<num>-<slug>` was not in `contracts/commands.md` 1.5.
- Status: resolved on the host in #203 (`HostedChecksToResume` in `internal/host/forge.go` derives `task/<num>-<slug>` from `Num` and `Slug` through `taskName`, skips rows that fail `taskName`, and ignores `TaskRow.Branch`) and on the home in #183 (`tskBranch` in `internal/home/cmd_task.go:76` answers `bad_request` to any `branch` other than `task/<num>-<slug>`, and `planning/contracts/commands.md` 1.5 states the rule).

### 2026-11-20 · deviation · A `set` that changes nothing and a `task assign` without a role were not refused

- Symptom: `contracts/commands.md` gives `group set` and `member set` "any of" their fields, and `task assign` at least one of `assignee` and `reviewer`. A `group set` with no field, a `member set` with nothing but `id`, and a `task assign` with neither role were read by `proto.Decode` and by the web client's schemas without refusal.
- Cause: each of those fields is optional by itself (`GroupSet`, `MemberSet`, `TaskAssign` in `internal/proto/command.go`; the schemas of the same names in `web/src/lib/proto.ts`), and "at least one" is a rule across fields, which the wire types do not state.
- Status: resolved in #179 (`grpSetValid` in `internal/home/cmd_group.go:110` and `memSetValid` in `internal/home/cmd_member.go:144`) and #183 (`tskAssign` in `internal/home/cmd_task.go:94`), which answer `bad_request` as `contracts/commands.md` specifies.

### 2026-11-21 · bug · `Loop.Inbox` marked only the groups that existed when it was called

- Symptom: after `Loop.Inbox(conn)` had made a host connection the inbox taker (`internal/home/run.go:366`), a group created later on that home spawned its `runGroup` with `seat.inbox` false (`internal/home/run_group.go:52`) and did not deliver queued items to `conn` until `Loop.Inbox(conn)` was called again (which `LocalLink` never does, because it sends no `hello`).
- Cause: `Conn` recorded `welcomed` across groups (`internal/home/run_conn.go:34`), whereas `inbox` was kept per group seat (`internal/home/run_group.go:31`) and `spawn` called `seat` with `inbox` false (`internal/home/run.go:162`).
- Status: resolved in #209: `Conn.taker` (`internal/home/run_conn.go:36`) is set under `Loop.mu` in `Loop.Inbox` (`internal/home/run.go:366`), and `runGroup.seat` initialises `runSeat{inbox: c.taker}` (`internal/home/run_group.go:53`), covering both local and remote hosts. Test: `TestLocalLinkInboxForGroupCreatedAfterAttach`.

### 2026-11-20 · limitation · `ReviewedMerge.GitHubRepo` is not filled from the group's repository

- Symptom: the merge with a remote runs `gh pr merge <pr> --repo <owner>/<repo>` (`planning/contracts/local.md` section 7, row "merge, with a remote"), but no code sets `<owner>/<repo>` from the group's repository; an empty `GitHubRepo` would make the merge fail.
- Cause: `ReviewedMerge.GitHubRepo` (`internal/host/merge.go`) is set by its caller, and no unit called the merge before HS-13. The claim that `--repo` makes gh skip every local step (worktree removal, local branch deletion) rests on the gh 2.102.0 source (`pkg/cmd/pr/merge/merge.go`: `CanDeleteLocalBranch = !cmd.Flags().Changed("repo")`), not on a run against GitHub or on older gh versions.
- Status: resolved in HS-13: `githubRepo` (`internal/host/deliver.go`, `buildReviewedMerge`) extracts `<owner>/<repo>` from the group row's `repo` field when `forge` is `github`.

### 2026-11-20 · deviation · The audit record has no `what` for a CLI killed on an idle close

- Symptom: `planning/flows/agent-turn.md` section G has the node write an audit row, and nothing else, when it kills a CLI that did not answer a close within 10 s outside a turn. `planning/contracts/storage.md` section 3 lists eight values of `what` (`delivered`, `refused`, `requested`, `approval`, `takeover`, `handback`, `merge`, `setting`), and none of them says that.
- Cause: the list of `what` was written before that rule. The audit write takes any text as `what` and has a constant for each of the eight only (`internal/host/audit.go:6-15`); nothing kills a CLI yet.
- Status: resolved in #187: `planning/contracts/storage.md` section 3 adds `killed` to `audit.what`, `internal/host/audit.go` defines `AuditKilled`, and `internal/host/run.go` writes the row naming the member and task when an idle close does not finish within 10 s (`TestCloseTimeoutOnAllPathsAuditAndC6SwitchOrdering`).

### 2026-11-21 · bug · A missing clone folder was reported as a failed start of git

- Symptom: on Windows CI, `TestWorkspaceGitFailures` failed at `internal/host/workspace_test.go:552`: the error of a command in a missing clone folder was not `fs.ErrNotExist`. On macOS the same error read `fork/exec /usr/bin/git: no such file or directory`, naming git instead of the folder.
- Cause: the new runner sets `SysProcAttr` (`internal/host/command_unix.go:13`, `internal/host/command_windows.go:16`), and `os.StartProcess` checks the working folder only when `SysProcAttr` is nil (Go `src/os/exec_posix.go:30`). The start then failed in the OS: on Unix with ENOENT from the child's chdir, on Windows with `ERROR_DIRECTORY` ("The directory name is invalid"), which `syscall.Errno.Is` does not map to `fs.ErrNotExist`.
- Status: resolved in branch `fix/host-workspace-review`: `runInput` checks the folder itself before it starts the command (`internal/host/command.go:45`), so the error is `chdir <folder>: …`, as os/exec gives it without `SysProcAttr`, with `fs.ErrNotExist` on every system. Test: `TestCommandInAMissingFolder`.

### 2026-11-20 · bug · Crash recovery of the host's worktrees decided on the folder alone

- Symptom: the independent review of `internal/host` reproduced four states. (a) A SIGKILL of `worktree add -b` during checkout, held there by a slow smudge filter, left a folder with only `.git` and `.gitattributes`, locked `initializing`; `CreateTaskWorkspace` reused it, `git status` there showed every file staged as deleted, and `RemoveTaskWorkspace(TaskCancelled)` failed with "cannot remove a locked working tree". (b) After `add -b` failed past the branch's creation, every retry failed with "a branch named 'task/1-x' already exists". (c) A kill inside `worktree remove` left "missing but already registered worktree" for good, and a half-deleted folder without `.git` was returned as the task's folder. (d) After a killed clone, `EnsureGroupWorkspace` accepted an empty `repo/`, and every later command failed with "not a git repository".
- Cause: `internal/host/workspace.go:41`, `:89`, `:122` and `:157-161` before the fix took an existing folder as a good worktree or clone, and `add -b` assumed the branch did not exist.
- Status: resolved in branch `fix/host-workspace-review`. Under the clone's lock the host prunes, reuses a folder only when `git worktree list` shows it on its branch (or detached, for a review), not `locked initializing`, and recorded for the same AI, and otherwise deletes it and adds it again, with `--force --force` while it is still registered and without `-b` when the branch exists (`internal/host/worktree.go:40-112`, `:129-180`). The clone is made in `.repo-clone` and renamed to `repo`; a `repo` that is not a Git repository is an error that names it (`internal/host/workspace.go:51-72`). Tests: `TestWorkspaceAfterAKilledAdd` (a real SIGKILL through the runner during a slow smudge filter), `TestWorkspaceAfterAnAddThatFailedInCheckout`, `TestWorkspaceAfterAStoppedRemove`, `TestWorkspaceCloneAfterAStoppedClone`, `TestWorkspaceRefusesARepoFolderThatIsNotAClone`. `planning/contracts/local.md` section 7, "A worktree left by a crash".

### 2026-11-20 · bug · Two AIs on one node shared a task's worktree and a review checkout

- Symptom: when the new holder's create ran before the old holder's removal, the new AI got the old AI's uncommitted file, and the removal then force-deleted the new session's folder.
- Cause: `CreateTaskWorkspace` and the removals did not know which AI held the folder (`internal/host/workspace.go:84-97` and `:176-192` before the fix); the host relied on N-4 ordering them.
- Status: resolved in branch `fix/host-workspace-review`. Create and Remove, for tasks and reviews, take the member id of the AI that holds the folder; the host records it after the add, in the worktree's own admin folder (`internal/host/worktree.go:68-85`). A create adds a folder recorded for another AI, or with no record, again; a removal leaves a folder recorded for another AI untouched (`internal/host/worktree.go:114-126`). Tests: `TestWorkspaceTaskHandedToAnotherAIOnTheSameNode`, `TestWorkspaceReviewerChangedOnTheSameNode`, `TestWorkspaceWithoutAHolderRecord`, and the scenario "A task given to another AI of the same node carries over only its commits".

### 2026-11-20 · bug · Host Git commands could wait for ever

- Symptom: workspace commands had no context and no limit, so a fetch that hung held the clone lock until a restart, and ssh could prompt on `/dev/tty`. The host had four runners for its Git and gh commands.
- Cause: `workspaceGit` (`internal/host/workspace.go:47`), `checkGit` (`internal/host/checks.go:59`), `runIn` (`internal/host/forge.go:26`) and `commandText` (`internal/host/merge.go:176`) before the fix; only the last two took a context.
- Status: resolved in branch `fix/host-workspace-review`. One runner, `runIn` (`internal/host/command.go:27`), runs every git and gh command of the host within the caller's context and its own limit of 60 minutes for `git clone` and `git push` and 10 minutes for any other, with `GIT_TERMINAL_PROMPT=0`, `GH_PROMPT_DISABLED=1`, `SSH_ASKPASS_REQUIRE=never` and `GCM_INTERACTIVE=never` added to the node's environment, in a new session on Unix and without a console window on Windows, killing the whole session or process tree when it stops (`command_unix.go`, `command_windows.go`). The other three call it; every exported workspace function takes a context. Tests: `TestCommandLimits`, `TestCommandsNeverWaitForAPerson`, `TestCommandStopsAtItsLimit`. `planning/contracts/local.md` section 7, "Commands never wait for a person".

### 2026-11-20 · bug · A task slug went into paths unchecked

- Symptom: the slug `x/../../../../outside` gave a folder outside the project, with a nil error.
- Cause: `internal/host/workspace.go:87-90` before the fix joined `<num>-<slug>` into paths and the branch name as given.
- Status: resolved in branch `fix/host-workspace-review`: `taskName` refuses a slug that does not match `^[a-z0-9]+(-[a-z0-9]+)*$` or is longer than 40 characters, and a `num` below 1, before any path or branch is built (`internal/host/workspace.go:113`); `reviewSlot` refuses a `num` below 1 (`internal/host/workspace.go:215`). Test: `TestWorkspaceRefusesSlugsTheHomeCannotProduce`. The open entry for P-1 above covers the decoder.

### 2026-11-20 · bug · The review diff followed the owner's diff settings

- Symptom: with `diff.external` set, the review diff was the external program's output; `textconv` and `color.ui=always` changed it too.
- Cause: `internal/host/workspace.go:154` before the fix ran `git diff --no-color` only.
- Status: resolved in branch `fix/host-workspace-review` with `--no-ext-diff --no-textconv --no-color` (`internal/host/workspace.go:155`). Test: `TestWorkspaceReviewDiffIgnoresDiffSettings`.

### 2026-11-20 · bug · A tracked `.solodawn-review.diff` stopped the next review round

- Symptom: when the reviewed commit tracks `.solodawn-review.diff`, the untracked diff of the previous round made `checkout --detach` fail.
- Cause: `internal/host/workspace.go:161` before the fix.
- Status: resolved in branch `fix/host-workspace-review` with `git -C review/<num> checkout --force --detach <commit>` (`internal/host/worktree.go:167`); `planning/contracts/local.md` section 7 marks the name as reserved. Test: `TestWorkspaceReviewOfACommitThatTracksTheDiffName`.

### 2026-11-20 · bug · Two spellings of one clone path took two locks

- Symptom: a clone reached through a symbolic link and through its real path had two locks.
- Cause: `cloneLock` keyed by `filepath.Clean` only; `internal/host/merge.go:36`, `:62` and `:114` lock `m.Repo` and `internal/host/workspace.go:29` locked `filepath.Join(projectDir, "repo")`.
- Status: resolved in branch `fix/host-workspace-review`: `cloneLock`, the one helper both use, keys by the real path of the longest existing prefix (`internal/host/workspace.go:21-33`), so a clone has one key before and after it exists. A relative and an absolute spelling still differ; the node passes absolute paths. Test: `TestCloneLockFollowsSymbolicLinks`.

### 2026-11-20 · deviation · P-1's task row decoder must refuse a slug the home cannot produce

- Symptom: the host refuses a `slug` that does not match `^[a-z0-9]+(-[a-z0-9]+)*$` or is longer than 40 characters, and a `num` below 1 (`internal/host/workspace.go:113`, `taskName`), because both go into paths and branch names; a task row with such a slug was accepted on the wire.
- Cause: the decoder of the task row (P-1, `internal/proto`) checked no form of `slug`; `planning/contracts/local.md` section 7, "Slug", gives the only form the home produces.
- Status: resolved by P-1 on `rebuild`: `proto.Slug` (`internal/proto/slug.go`) and `proto.Decode` refuse such a row (`internal/proto/decode.go:163`). The host keeps its own refusal before it builds a path.

### 2026-11-20 · deviation · An open chat hid the shell's top bar on a phone from its own styles

- Symptom: below 768 px an open chat showed no top bar and no bottom navigation, because `web/src/routes/Groups.svelte` hid them with `.app:has(.groups.open)` in its own styles, and it read the group id from the address itself, because `route` carried only the page.
- Cause: the shell had no place where a page says that it takes the whole screen, and the route table had no parameters.
- Status: resolved in U-0 (#170): the route `chat` (`#/groups/<g>`) asks the shell for the whole screen on a phone (`full: 'phone'` in `web/src/lib/route.svelte.ts`, drawn by `.app[data-full='phone']` in `web/src/styles/base.css`), and `Groups.svelte` reads `params.g`. The title of the top bar from 768 px is the open entry of 2026-11-21.

### 2026-11-20 · deviation · Web mock does not answer `host.info` or push `groups`

- Symptom: F-02 in `planning/steps/step-1-frontend.md` asks for a mock that answers `hello`, `sync`, `fetch` and `host.info`; the settled plan (PR #58) adds that it pushes `groups`. `mockCtl` answers `hello`, `sync`, `fetch` and `cmd`, and pushes only `reset` and `rows`. `proto.ts` has no `host.info` type; it has a `Groups` type, written from the contract.
- Cause: neither frame has a Go type or a wire example yet, and the mock answers from wire examples only (`web/src/lib/mock.ts:63-75`, `web/src/lib/proto.ts:94-96`, `web/src/lib/proto.ts:372-377`). Both are frozen by unit P-2 (`planning/contracts/conventions.md` section 5; `planning/steps/parallel-plan-units.md` section 1), which also adds the mock answers for `host.info` and `groups`; B-10 named them before the parallel plan.
- Status: resolved in #165 (P-2): `host-info-answer.json` and `groups.json` exist with their Go types and schemas; `mockCtl` answers `host.info` and pushes `groups` before it answers a `group create` (`web/src/lib/mock.ts`, `mockCtl`).

### 2026-11-20 · bug · The local merge ran the project's Git hooks (review M5)

- Symptom: once a check's `npm install` had let Husky set `core.hooksPath` in the clone's config, a `prepare-commit-msg` hook that adds a trailer made `Apply` return a conflict ("last commit on base does not record …") although the base held the change, and the next round added a second squash commit; a commitlint-style `commit-msg` hook made every local merge a conflict. A configured signing key could make the commit wait for a person.
- Cause: `git merge --squash`, `git commit` and `git reset --merge` in `squashLocal` (`internal/host/merge.go`) ran with the clone's own hooks and signing settings, and the inspection compared the exact message.
- Status: resolved on branch `fix/host-merge-review`: the three commands, and the reset of an interrupted squash after a restart, run with `-c core.hooksPath=<repo>/.git/solodawn-no-hooks`, an empty folder the host makes; the commit with `--no-verify --no-gpg-sign` (`noHooks`, `squashLocal`, `resetLocal`). Pushes and fetches keep the project's hooks (Git LFS needs `pre-push`). `local.md` section 7, row "merge, no remote". Tests `TestLocalMergeRunsNoProjectHooks`, `TestLocalMergeWithoutHooksFolder`.

### 2026-11-20 · bug · `Submit` could reuse a pull request from a fork (review M6)

- Symptom: an open pull request from a fork whose branch has the task branch's name, into the same base, was taken as the task's pull request.
- Cause: in gh 2.102.0 `gh pr list --head` matches the branch name only; `openPR` (`internal/host/forge.go`) took the first entry.
- Status: resolved on branch `fix/host-merge-review`: `--json` asks for `isCrossRepository` and such entries are skipped; `local.md` section 7, row `submit_task`. Test `TestSubmitForgeSkipsForkPullRequests`, with a fake `gh`; not verified against GitHub.

### 2026-11-20 · bug · A merge queue or auto-merge was reported as a conflict (review M7)

- Symptom: when `gh pr merge` exits 0 and the pull request stays `OPEN` (a merge queue or auto-merge merges it later), `Apply` reported `task conflict` "pull request N is OPEN", and so did `Inspect` after a restart.
- Cause: `Apply` and `Inspect` (`internal/host/merge.go`) read the pull request once.
- Status: resolved on branch `fix/host-merge-review`: the pull request is read with one `gh api graphql` call (`state`, `headRefOid`, `mergeCommit`, `autoMergeRequest`, `mergeQueueEntry`). The merge is pending only while it is `OPEN` with a merge queue entry or auto-merge; `awaitPR` then reads every 30 s until an outcome, or 60 minutes after the merge command (after a restart, after `ReviewedMerge.Taken`, the audit row's time, or the restart), then `task conflict` "merge still pending after 60 minutes (merge queue or auto-merge)". An `OPEN` pull request with neither (the merge never ran, failed, or left the queue) is `task conflict` at once (re-check N2; `TestOpenPRWithNothingPending`). Right after a merge command that succeeded, that holds only from the second read, as GitHub's read can lag its write; at the limit, a last read that failed returns its error, so a merged PR out of reach is not reported as a conflict (focused review D1, D2; `TestReadFailureAtTheLimitAfterARestart`, `TestReadFailureAtTheLimitAfterAnOutage`, `TestFirstReadAfterTheMergeMayLag`). `local.md` section 7, rows "merge, with a remote" and "node restarts with a merge". Tests `TestMergeWaitsWhilePending`, `TestPendingMergeEnds`, `TestInspectResumesPendingMerge`, `TestPendingMergeStops`, with a fake `gh`; not verified against GitHub. A read that fails during the wait counts as unfinished and is repeated 30 s later, as for hosted checks (`TestPendingMergeOutlastsFailedReads`).

### 2026-11-20 · bug · `HostedChecksToResume` panicked on a row without `checks.local` (review M8)

- Symptom: a task row at `cloud_check` with no `checks.local` made the selection dereference a nil pointer.
- Cause: `time.UnixMilli(t.Checks.Local.At)` in `HostedChecksToResume` (`internal/host/forge.go`).
- Status: resolved on branch `fix/host-merge-review`: such a task resumes with its 60 minutes spent (`Since` is the zero time), so it ends with the timed-out result unless its first read has finished. Tests `TestHostedChecksToResumePausedAndWithoutLocalCheck`, `TestHostedChecksWithoutLocalCheckTimeOut`.

### 2026-11-20 · bug · Restart inspection without a remote looked at the last commit only (review L6)

- Symptom: after merging task t1 and then t2, `Inspect` of t1 reported a conflict.
- Cause: `inspectLocal` (`internal/host/merge.go`) compared the message of `git log -1` on the base only.
- Status: resolved on branch `fix/host-merge-review`: merged means a commit on the first-parent history of the base whose message is exactly the merge message; `local.md` section 7, row "node restarts with a merge". Test `TestInspectFindsAnEarlierLocalMerge`.

### 2026-11-20 · deviation · Paused tasks were not resumed at `cloud_check` (review L8)

- Symptom: after a restart, a paused task at `cloud_check` was not polled again, though task-machine row 6 takes `task cloud` while paused and `local.md` section 7 says every task at that step.
- Cause: `HostedChecksToResume` (`internal/host/forge.go`) kept `active` tasks only, and its test asserted the exclusion.
- Status: resolved on branch `fix/host-merge-review`: tasks at `cloud_check` that are active or paused resume, the two states row 6 accepts; the test now expects the paused task.

### 2026-11-20 · bug · `DeleteBranch` could drop unreviewed commits (review L9)

- Symptom: a local task branch that had moved past the merged commit was deleted with `git branch -D`, and its later commits with it.
- Cause: `DeleteBranch` (`internal/host/merge.go`) deleted the branch whatever it pointed at.
- Status: resolved on branch `fix/host-merge-review`: the branch is deleted with `git branch -D` only while `rev-parse` shows it is the merged commit; a moved branch is kept and `DeleteBranch` returns `deleted` false. `branch -D` refuses a branch that a worktree still has checked out, the only place it could move from; an earlier version used `update-ref -d`, which deleted it under a worktree whose removal had failed (re-check N1). `local.md` section 7, row "task merged". Tests `TestDeleteBranchOnlyAtTheMergedCommit`, `TestDeleteBranchRefusedWhileCheckedOut`, `TestDeleteBranchFailures`.

### 2026-11-20 · limitation · The task machine held `review.failed` and `wait.kept` beside the task row

- Symptom: on the branch of HM-M, `home.Step` took and returned `home.Task`, a `proto.TaskRow` with two more fields, `ReviewFailed` and `Kept`. A caller that kept only the `proto.TaskRow` would have lost the count of failed reviews and the wait a pause replaced.
- Cause: `contracts/rows.md` section 4 has `review.failed` and `wait.kept`; `proto.TaskRow` did not have them until P-1 froze them, and HM-M was built in the same wave.
- Status: resolved in PR #161, before it was merged: after P-1 (PR #162) `home.Step` takes and returns `proto.TaskRow`, and the two fields are `review.failed` and `wait.kept` of the row.

### 2026-11-20 · limitation · `group create` was sent as the contract describes it, before its wire example

- Symptom: the create-group dialog of the web client was written before P-1 froze the payload of `group`, with `g` equal to `n` as `planning/contracts/commands.md` 1.1 reads.
- Cause: the order of the units; `Home.create` in `web/src/lib/groups.svelte.ts`.
- Status: resolved in #164. P-1 (#162) froze `group-create.json` with `g` equal to `n`; `create` sends the fields of that example in its order, and its test reads the payload with the `GroupCreate` schema. The dialog still asks for the repository address with `forge: none`, since `repo` is absent only for a group without a repository (`planning/contracts/rows.md` section 1).

### 2026-11-20 · deviation · No unit measured a row against its size limit

- Symptom: `planning/contracts/conventions.md` section 4 has the home answer `bad_request`, naming the limit and the size, to a command that would write a row above 16 KiB, or a card above 12 KiB when it is created. No row of `planning/steps/parallel-plan-units.md` names the rule, and the first version of the seam handed the rows of a command's function to its caller unmeasured.
- Status: resolved in #160 by the coordinator's decision of 2026-11-20: `Table.Apply` measures every row of a result after numbering it and refuses the command for one above its limit (`State.Seal`, `internal/home/home.go:344`; `rowFits` at `:143`).

### 2026-11-20 · bug · The tests check counted deleted files

- Symptom: changing `a.go` and deleting `a_test.go` passed; only deleting `old.go` failed; renaming `x_test.go` to `y.go` passed. `local.md` §7 says a deleted file counts as neither a test file nor a source file. Found in the independent review of `internal/host` (M1).
- Cause: the names diff of the hard checks listed deleted paths (`internal/host/checks.go:163` before the fix); with `--no-renames` a rename is a deletion and an addition.
- Status: resolved on branch `fix/host-checks-review`: `--diff-filter=d` on the names diff. Test `TestTestsCheckIgnoresDeletedFiles`.

### 2026-11-20 · bug · The secrets check did not read files Git takes as binary

- Symptom: a `server.pem` holding a PKCS #8 key passed the secrets hard check when `.gitattributes` marked `*.pem binary` or the file held a NUL byte; Git printed "Binary files differ" instead of the added lines (M2).
- Cause: the patch diff ran without `--text` (`internal/host/checks.go:159` before the fix).
- Status: resolved on branch `fix/host-checks-review`: `--text` on the patch diff. A large binary then put tens of MiB through the scan: the check now matches in place with a literal prefilter and no copies, fails above 64 MiB of added lines with "too large to check", and the hard checks stop at their own 2-minute limit with "timed out". Tests `TestSecretsCheckReadsFilesGitTakesAsBinary`, `TestAddedLinesOverTheLimit`, `TestHardChecksTimeOutOnTheirOwnLimit`.

### 2026-11-20 · bug · A check's output tail could hold the end of a private key

- Symptom: a check that printed a 4.4 KiB key left a tail that began inside the key and ended with its END line, which redaction did not match (L1).
- Cause: the output was cut to 4 KiB before redaction, and redaction needed the header (`internal/host/checks.go:118-125` before the fix).
- Status: resolved on branch `fix/host-checks-review`: the last 64 KiB of output are redacted, then cut to the 4 KiB tail (`checkDetail`); redaction, and not the check, also replaces a private-key footer without its header from the start of the text. Tests `TestCheckOutputRedactsKeyLongerThanTail`, `TestCheckDetailIsErrorThenRedactedTail`, `TestRedactSecrets`.

### 2026-11-20 · bug · A turn that ran out took the hard checks with it

- Symptom: with a context of 300 ms and the check `sleep 2`, `Run` returned "context deadline exceeded" and no items, so neither hard check ran (L3; R-103 says both always run).
- Cause: the hard checks used the caller's context after the group's checks had used up its deadline (`internal/host/checks.go:38` before the fix).
- Status: resolved on branch `fix/host-checks-review`: the turn's deadline does not stop the hard checks, which have their own limit; an explicit cancel (pause, interrupt, shutdown) stops the run, which then reports nothing and returns the error. The group's checks share the turn's deadline, with `turn_minutes` bounding a context without one. Tests `TestTurnDeadlineKeepsHardChecks`, `TestCancelStopsRun`.

### 2026-11-20 · bug · A background child of a check outlived it and hid the error

- Symptom: `sleep 3 & echo done` gave `ok=false` with the detail "done\n": the `WaitDelay` error was dropped, and `sleep` kept running (L4).
- Cause: the detail was replaced by the output tail, and nothing killed the process group after `Wait` (`internal/host/checks.go:115-125` before the fix).
- Status: resolved on branch `fix/host-checks-review`: after `Wait` the process group is killed (Unix, `checks_tree_unix.go`); on Windows the shell runs in a job object closed after `Wait` (`checks_tree_windows.go`); without a job nothing is killed after `Wait`, as Go has released the process. The detail is the error, then the redacted tail. Test `TestBackgroundChildDiesWithCheck`.

### 2026-11-20 · bug · A clean status did not prove the checked commit

- Symptom: a tracked file with the skip-worktree or assume-unchanged bit could differ from the commit while `git status --porcelain` stayed empty, so `run_checks` checked content that was not the commit (L5).
- Cause: the clean-tree test read the status only (`internal/host/checks.go:70-80` before the fix).
- Status: resolved on branch `fix/host-checks-review`: a skip-worktree path (`S` or `s` in `git ls-files -v`) is refused with its own error naming up to 10 paths; the assume-unchanged bit, a speed hint that `core.ignoreStat` sets on every path, is cleared before the status, so a change shows and a clean tree runs. `local.md` §7 `run_checks` says so. Tests `TestSkipWorktreeIsRefusedByName`, `TestAssumeUnchangedDoesNotHideChanges`.

### 2026-11-20 · deviation · Redaction was not available to the node

- Symptom: the node must apply the one pattern list to the details of `task fail`, `login_invalid` and `turn_interrupted` (`local.md` §7), but the helper was unexported (L10).
- Cause: `redactSecrets` in `internal/host/secret.go:53` before the fix.
- Status: resolved on branch `fix/host-checks-review`: exported as `RedactSecrets`. Test `TestRedactSecretsFromOutsideHost`.

### 2026-11-20 · bug · OpenPGP private keys passed the secret check

- Symptom: `-----BEGIN PGP PRIVATE KEY BLOCK-----` matched no pattern (L11).
- Cause: the PEM pattern required `PRIVATE KEY-----` (`internal/host/secret.go:19` before the fix).
- Status: resolved on branch `fix/host-checks-review`: the pattern takes an OpenPGP block through its END line; `local.md` §8 reads "a PEM or OpenPGP private-key header". Tests in `TestRedactSecrets` and `TestCheckOutboundSecretRejects`.

### 2026-11-20 · bug · The signalling client could set off the rendezvous's own limits

- Symptom: a node's rendezvous connection could be closed by the server for two things the node did itself. A backlog of signalling left in bursts of 90 frames exactly 10 seconds apart, so ordinary latency between two bursts could bring 121 frames within the server's 10 seconds. A `to` frame of 16,383 or 16,384 bytes passed the node's own check, but the `from` the server forwards is 2 bytes longer than the 16 KiB limit, and the server closes the sender.
- Cause: the pacer allowed 90 frames in any 10 seconds (`internal/peer/path.go`, `pacer.turn`), with no margin in time against the server's count (`internal/rendezvous/server.go`, `client.admit`). `Path.Write` held every frame to 16,384 bytes, the `to` frames of `Path.send` included, while the server refuses a `to` whose `from` would exceed that (`internal/rendezvous/server.go`, `client.sendFrom`).
- Status: resolved on branch `fix/peer-pacer-margin`. The pacer allows 90 frames in any 11 seconds, and a `to` frame is refused locally over 16,382 bytes; `planning/contracts/rendezvous.md` section 2 states both. The browser's signalling client gets the same rules in #155.

### 2026-11-20 · bug · The independent review of `web/src/lib` found ten faults in signalling, the tab lock, the device log and the wire types, and its re-check five more

- Symptom: none showed in use, as no module has a caller outside tests yet (W-2, U-15). F1: a path sent every frame at once and any size; 150 `send` calls in one tick reached the socket, and the server closes a connection at its 121st frame in 10 seconds or at a frame over 16 KiB. F2: one URL the browser refuses (`ws://` from an HTTPS page, a malformed URL) made `rendezvous()` throw and left the sockets already opened. F3: `DeviceLog.apply` called for entries 1, 2, 3 at once refused a later one as `gap` when its signature was checked first. F4: a path closed before `ready` kept every frame sent to it, and `receive` was still called after `close()`. F5: a frame of a path could reach `receive` before a `from` that arrived ahead of it. F6: an `AbortError` of the session itself was read as a stolen lock. F7: `usage` and `checks` took `[]`, which Go refuses. F8: a `from` whose `conn` is no random id was delivered; Go drops it. F9: a `ready` with `ttl` below 1 was taken, and ICE entries pion and the browser refuse were passed on. F10: `{"re":1,"ok":true,"err":{…}}` was read as a success. The re-check of the first fix found: N1, one exception in `receive` or in `known` silenced a path for good; N2, a backlog left in bursts exactly at the server's 10-second boundary, so jitter closes the path; N3, a `to` of 16,383 or 16,384 bytes passed, whose `from` the server refuses; N4, the pace followed the wall clock, and setting it back an hour held a frame for 61 minutes; N5, TURN entries with an empty username or credential and ports outside 1 to 65535 were kept, on which Chromium throws.
- Cause: `web/src/lib/rendezvous.ts:33`, `:35`, `:45-52`, `:63-72`, `:88-93`, `web/src/lib/devlog.ts:50-51`, `web/src/lib/tablock.ts:13-17`, `web/src/lib/proto.ts:27`, `:77-78`, `:235-242`, `:275`, `:569-582` before the fix.
- Status: resolved in branch `fix/web-lib-review`. A path keeps 90 frames in any 11 seconds, a second inside the server's 10, by `performance.now()`, and queues the rest in order behind one timer; a frame over 16,384 UTF-8 bytes, or a `to` over 16,382 (its `from` is 2 bytes longer), throws `RangeError` before any path sends it (`web/src/lib/rendezvous.ts:26-34`, `:107-145`, `:219-224`); `internal/peer/path.go:53-78`, `:135-148` pace at 10 seconds and is fixed apart. A refused URL is a path that sends nothing and reports `closed` after the call (`:93-105`). Each path delivers through one promise chain, its `closed` last, and nothing after `close()` (`:77-91`, `:142-145`, `:182-198`); a check or a `receive` that throws is passed to `reportError`, its frame counts as dropped, and the frames after it still come (`:63`, `:83-91`). `DeviceLog.apply` checks entries one after another in the order of the calls (`web/src/lib/devlog.ts:46-57`). Only a rejection of `request()` is a steal; a session's error is thrown after `request()` resolves (`web/src/lib/tablock.ts:3-31`). `usage`, `checks` and empty bodies refuse an array through `record` (`web/src/lib/proto.ts:17-25`); `conn` is an `Id` (`:36`) and the browser drops a `from` whose `conn` is not (`web/src/lib/rendezvous.ts:175-180`); a success frame has no `err` (`web/src/lib/proto.ts:139-140`). A `ready` with `ttl` below 1 closes its path, such an `ice` answer is dropped, and only usable ICE entries go on (`web/src/lib/rendezvous.ts:37-61`): the entries Go drops (`internal/peer/path.go:245-270`), and also an empty URL list, TURN without a username or credential or with an empty one, and a port outside 1 to 65535, on which a browser's `RTCPeerConnection` throws (WebRTC, "validate an ICE server"; Chromium for the empty values and the port). Upper-case schemes and ports with leading zeros are dropped, which pion takes. Tests: `web/src/lib/rendezvous.test.ts`, `devlog.test.ts`, `tablock.test.ts`, `proto.test.ts`, `web/features/rendezvous.spec.ts`.

### 2026-11-20 · limitation · The own-address rule knows the interface addresses of the start only

- Symptom: the relay sends a datagram to one of the server's own addresses only when it is for one of its relay allocations (`planning/contracts/rendezvous.md` section 3). The server reads its interface addresses once, when it starts; an address that an interface gets later (a new DHCP lease, an added IPv6 address) is not one of its own addresses for this rule until the service restarts, so a client can relay to a service on the server through that address.
- Cause: `ownAddrs` read `net.InterfaceAddrs` once, from `relay.generator` at startup (`internal/rendezvous/relay.go:111-129` before the fix), with the addresses its name resolves to.
- Status: resolved in PR #151: `generator.ownAddr` reads them again once they are a minute old and keeps the previous set if the read fails (`internal/rendezvous/relay.go:133-160`); a new address counts as the server's own within a minute.

### 2026-11-20 · deviation · The plan had host files register their subcommands and channels

- Symptom: `planning/steps/parallel-plan-units.md` had HS-19 register the terminal channel in the table of PR-0 from `host/pty.go`, and every host unit register its subcommands and channels in the tables of N-0 and PR-0 from its own file. `host` may import neither `peer` nor `node` (`planning/architecture.md` 8.2, `imports_test.go`).
- Status: resolved in #135 by the coordinator's decision of 2026-11-20: `node` registers the subcommands of host features in the table of N-0 and gives each `Peer` the handlers of their channels when it creates it (`peer.New`, `peer.Channels`); a handler gets a `peer.Stream`. The entries of HS-19, N-4, PR-0 and PR-1 and the note of lane HOST say so.

### 2026-11-20 · deviation · The probe needs `peer/signal.go`, which PR-1 did not own

- Symptom: PR-1 owned `peer/probe.go` only. PR-0 hands a probe offer to its handler, but a probe also needs an answer without TURN (`create` takes its ICE servers from `paths`), an ordinary offer that closes a pending probe (`ordinaryOffer`), and a probe that does not take the device's slot before its first `hello` (`answer`), all in `internal/peer/signal.go`.
- Status: resolved in #135 by the coordinator's decision of 2026-11-20: PR-1 also owns those three changes in `peer/signal.go`.

### 2026-11-20 · bug · Go acceptance steps acted beyond their own scenario

Found in the audit of the harness; each was reproduced before the fix (lines are those before it).
- A failing step could end the whole suite: later scenarios and their After hooks did not run, godog printed no summary, go test said "subtest may have called FailNow on a parent test". Steps called `Fatal` on the suite's own test from a scenario's subtest (`features/node_update_test.go:98`, `:104`, `features/host_forge_test.go:26`, and `features/host_checks_test.go:36`, fixed on branch `ci/go127-windows-macos`).
- `go test -tags acceptance -count=2 ./features/` failed with `open …/TestFeatures…/017/v0.3.1: no such file or directory`: the cache of release builds was a package variable that outlived the TempDir of the first run (`features/node_update_test.go:89`).
- The fake node's socket stayed listening through every later scenario, and each node-update scenario left an HTTP server listening (five by the end): cleanups were registered with `f.t.Cleanup` (`features/cmd_local_test.go:38`, `:44`, `features/node_update_test.go:54`).
- Under `go test -run` naming the two login-service scenarios, "Register" failed and "Remove" passed without doing anything: HOME was set in a subtest of the suite's test that `-run` skipped (`features/node_service_test.go:30`).
- A scenario that changed a variable other than HOME, or the working directory, changed every later scenario: the guard of PR #121 checked HOME only and did not restore it (`features/acceptance_test.go:67-80`).
- With `commit.gpgsign = true` in the global Git configuration, a host-check scenario failed with "gpg failed to sign the data": git ran with the user's and the system's configuration.
- A step error X was printed as "after scenario hook failed: X, step error: X, X, step error: X": After hooks returned the step's error as their own (`features/identity_test.go:72`, `features/rendezvous_test.go:101`, `features/peer_test.go:61`).
- Status: resolved in PR #142: steps return errors (the comment on `fixtures.t` states the rule); `afterScenario` undoes per scenario; the build cache is `fixtures.stamped`; the login steps set and restore the variables around each operation; after each scenario the suite compares every variable and the working directory with their state at its start, names what changed (never values) and restores it; `TestFeatures` sets `GIT_CONFIG_GLOBAL` to the null device and `GIT_CONFIG_NOSYSTEM`; the hooks return nil.

### 2026-11-20 · bug · The web acceptance file failed on a busy machine although its tests passed

- Symptom: with every core busy, `features/web.spec.ts` failed 3 of 3 runs with "HookTimeoutError: Hook timed out in 10000ms" at `web/features/motion.ts:22`; all 213 tests passed.
- Cause: `browser.close()` took 10.0 s under that load (20 to 60 ms idle), Vitest's default hook limit.
- Status: resolved in PR #142: both browser hooks have 60 s; 3 of 3 loaded runs passed.

### 2026-11-20 · bug · The node could not start on Windows

- Symptom: on the first Windows runs of CI (Windows Server 2025, and 2022 in a trial), every `ListenLocal` failed, so `solodawn node` could not start: 13 tests of `internal/node` and `cmd/solodawn` failed, first with "remove …\sock: The system cannot find the file specified", then with "Access is denied".
- Cause: Windows refuses a dial to a path where no file is, as it refuses one to a stale socket, so `clearStale` removed a file that was not there (`internal/node/socket.go:45-46` before the fix). Then `ownerOnly` opened the socket with `WRITE_DAC` alone, and `SetSecurityInfo` answers "Access is denied" to such a handle on both versions (`internal/node/socket_windows.go:51` before the fix; found with a diagnostic test in CI).
- Also: five tests encoded Unix answers (the error of a dial to a missing socket, the text of exec on a non-executable, address in use, a binary's mode, the `.old` image the Windows install keeps), and the two DACL tests expected `D:P(…)` with the full SID where Windows writes `D:PAI(…)` and the alias `LA` of the runner's built-in Administrator.
- Status: resolved on branch `ci/go127-windows-macos`: a refused dial removes the file only if one is there, the handle also has `READ_CONTROL`, and the tests take the system's own answers.

### 2026-11-20 · deviation · CI gaps found in the audit of the workflows

- Actions were referenced by movable tags (`pnpm/action-setup` is third party); `release.yml` restored pnpm and Go caches into the build of released files; `sudo ./netsim.test` had no time limit; CI never vetted code behind the `acceptance` tag nor compiled the macOS and Windows tests. Resolved in PR #142: pins to the full commit, no cache in the release job, `-test.timeout=10m`, `go vet` with the tag and for every cross-compile target.
- Every job built with Go 1.26.0, eleven patch releases behind 1.26.11 (Backend run 44697108086), because setup-go v5.6.0 reads `go 1.26.0` in `go.mod:3` as an exact version. Resolved on branch `ci/go127-windows-macos` by the coordinator's decision of 2026-11-20: every setup-go takes `go-version: '1.27.x'` with `check-latest: true`, the newest patch of the line development uses (1.27.4); `go.mod` keeps `go 1.26.0` as the minimum.
- No job had a time limit, so a hung step could hold a runner for six hours (a `pnpm install` held a macOS runner for 30 minutes on 2026-11-20); and no Go code ran on Windows or macOS. Resolved on branch `ci/go127-windows-macos`: `timeout-minutes` on every job, and a Backend job on `windows-latest` and `macos-latest` (vet, `go test -race`, the acceptance suite, coverage of the files built only there).

### 2026-11-20 · deviation · Two host check tests skipped themselves on Windows

- Symptom: `TestChecksRequireBothGitDiffReads` and `TestCheckCancellationStopsForegroundPipeline` in `internal/host/checks_test.go` called `t.Skip` on Windows. `t.Skip(` is a listed directive under `[tamper]` in `.project-standard/gates.toml` with no exception, yet both merged with HS-5 (#94).
- Cause: both tests need a POSIX shell (a fake `git` written as a shell script; a pipeline with a FIFO), and the author guarded them with a runtime skip. Neither CI nor the reviews of #94 ran the directive check.
- Status: resolved. Both tests and their FIFO helper moved to `internal/host/checks_unix_test.go` under `//go:build !windows`, so they are not compiled where they cannot run; no listed directive remains in `cmd/`, `internal/` or `features/`.

### 2026-11-20 · bug · Wire types read seven things differently from the contracts

- Symptom: an audit of the merged wire layer found (1) `proto.ts` typed some 50 integer fields (`seq`, `re`, `id`, every time and count outside a message row) as any number, so the browser read `"seq":805.5` where `proto.Decode` refuses it; (2) `decode(Frame, …)` read a `welcome`, `synced` or `fetched` answer that lacks a field as a `cmd` answer `{re, ok}` instead of refusing it; (3) neither language had the optional `full` of an `approval` body (`contracts/messages.md` 2.3, added in PR #71 after the B-01d freeze), so it was dropped; (4) Go's `MessageRow[ApprovalBody, …]` took `"state":null` for a card, and `MessageRow[EmptyBody, StatusOnly]` took a state for a text message, both of which `proto.ts` refuses (`contracts/rows.md` section 5); (5) `testdata/welcome.json` gave the node version as `0.3.1`, where `contracts/local.md` section 9 and the `welcome` example (PR #112) make it the release tag, `v0.3.1`; (6) Go's `Success` took `"ok":false`, which `proto.ts` refuses (`contracts/conventions.md` section 3); (7) the web read a number beyond the largest double in a fraction field (`"window_pct":1e400`) as `Infinity`, which `proto.Decode` refuses.
- Cause: (1) `num` in place of `int` (`web/src/lib/proto.ts`); (2) the `Frame` union tried `CmdResult`, which has no `t` and drops unknown fields, after the typed answers; (3) the structs were frozen before the field existed; (4) `MessageRow` held its state as `*S` for every kind; (5) the example predates PR #112; (6) `Success.OK` was a plain `bool`; (7) `v.number()` accepts `Infinity`.
- Status: resolved in branch `fix/audit-proto`: integers are `int` except the four `usage` fractions (`web/src/lib/proto.ts:10-12`); the `cmd` answer in `Frame` has no `t` (`web/src/lib/proto.ts:562`); `full` is in both types and the approval examples (`internal/proto/messages.go:41`, `web/src/lib/proto.ts:317`); a card's state is a struct and a non-card's is `NoCard`, which is `null` only (`internal/proto/rows.go:98-115`, `internal/proto/messages.go:198-207`); the `welcome` example has `v0.3.1`; `Success.OK` has the type `True`, which takes `true` only (`internal/proto/session.go`); every number in `proto.ts` is finite.

### 2026-11-20 · bug · The language button was not named by its visible text

- Symptom: the top-bar language button shows "EN" (or "中") but was named "Switch language / 切换语言" by its `aria-label`, so a speech-input user saying "click EN" reached nothing (WCAG 2.2 SC 2.5.3), and the half of the name in the other language had no language of its own (SC 3.1.2). Found in the audit of the web shell.
- Cause: `aria-label={t('bar.lang')}` replaced the button's content as its name (`web/src/App.svelte:49` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: no `aria-label`; the name is the visible text, marked with its language, then a visually hidden "Switch language / 切换语言" whose halves carry `en` and `zh-CN` (`web/src/App.svelte`, `textIn` and `otherLang` in `web/src/lib/prefs.svelte.ts`). The look is unchanged. Scenario "The language button is named by its visible text, each part in its own language".

### 2026-11-20 · deviation · The browser bar kept the system colour after a theme was chosen

- Symptom: with a light system and the dark theme chosen, the browser's bar (Android Chrome, Safari) stayed white: `<meta name="theme-color">` in `web/index.html` follows the system scheme only.
- Status: resolved on branch `fix/audit-web-shell-2`: an effect in `web/src/lib/prefs.svelte.ts` gives both tags the colour `index.html` gives the theme shown. The manifest colours stay fixed. Unit tests "the colour of the browser bar".

### 2026-11-20 · bug · Under forced colours a choice was told by the weight of its text alone

- Symptom: in a Windows contrast theme (forced colours), the sliding mark of the navigation and the thumb of each segmented control disappear, because the system drops backgrounds and shadows; the current page and the chosen theme and language were shown only in bold.
- Status: resolved on branch `fix/audit-web-shell-2`: `web/src/styles/base.css` section 8 draws them in `Highlight` and `HighlightText` with `forced-color-adjust: none` (otherwise the system's text backplate hides the text) and hides the hover layer over the current entry. Scenario "Under forced colours the chosen options and the current page are marked in system colours".

### 2026-11-20 · deviation · Defaults were stored on the first visit

- Symptom: the first load wrote the default theme and the language guessed from the browser to `localStorage`, so a person who never chose a language kept it after changing the browser's language.
- Cause: one effect wrote `<html>` and storage on every run, the first included (`web/src/lib/prefs.svelte.ts:40-47` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: storage is written only when the person changes a setting (`set` in `web/src/lib/prefs.svelte.ts`). Unit test "writes nothing to this browser until the person changes a setting".

### 2026-11-20 · bug · Web device log refused the entry it held when `p` carried a lone surrogate

- Symptom: a browser holding a device-log entry whose `p` has U+FFFD refused with `fork` the same entry arriving with the escape `\ud800` in that place, and the reverse; a Go node, for the same bytes, does not refuse it (`contracts/envelope.md` section 4: the entry held at a version, the same `p`, `k` and `s`, is not refused). Both spellings are one signed text: the signature covers `p` as `TextEncoder` encodes it, a lone surrogate as U+FFFD, and `encoding/json` reads the escape as U+FFFD. A node passes entries on with U+FFFD, so a browser that wrote the escape and later sends its log again after a `welcome` has its own entry refused by a browser that got it through a node. Found in the audit of the identity code.
- Cause: `same` compared `p` as JavaScript strings (`web/src/lib/devlog.ts:111` before the fix).
- Status: resolved in PR #115: `same` compares `p.toWellFormed()` (`web/src/lib/devlog.ts:110-113`). `crypto.json` holds the case in `devlogs` (its entries are JSON text, so that `p` can carry the escape), and both vector runners pass it; `web/src/lib/devlog.test.ts` and `internal/identity/devlog_test.go` (`TestLogTakesTheSameEntryWithALoneSurrogateAgain`) test it too.

### 2026-11-20 · deviation · A device-log entry of more than 2 KiB is refused before the contract says so

- Symptom: `Log.Apply` and `DeviceLog.apply` refuse an entry whose `p` is more than 2,048 bytes as UTF-8. `planning/contracts/envelope.md` on `rebuild` has no such rule yet.
- Cause: the rule was settled by the coordinator (`planning/risks.md` Q-23) and is written in planning pull request #71, which is not merged: `contracts/envelope.md` section 4, rule 2, and the two cases of `contracts/vectors.md`. The code follows that text (`internal/identity/devlog.go:27-29`, `:89`; `web/src/lib/devlog.ts:118-128`), and `crypto.json` holds the cases.
- Status: resolved: PR #71 (4ac3e10f5) merged the rule into `planning/contracts/envelope.md` section 4, rule 2, so the code now follows the contract.

### 2026-11-20 · bug · Language options were spoken in the language of the page

- Symptom: on the settings page of the English interface, the option "简体中文" had no language of its own, so a screen reader spoke it with the English voice (eSpeak in NVDA reads no Chinese); in the Chinese interface "English" was given the Chinese voice. WCAG 2.2 SC 3.1.2 (Language of Parts, AA) asks that each passage's language be programmatically determined. Found in the audit of the web shell.
- Cause: the radio labels inherited `lang` from `<html>`, which follows the interface language (`web/src/routes/Settings.svelte:39-40` before the fix).
- Status: resolved on branch `fix/audit-web-shell`: each option's `<label>` carries the `lang` of the language it names (`web/src/routes/Settings.svelte:41`); the scenario "Each language option is marked with the language it is written in" of `features/web.feature` covers both interfaces.

### 2026-11-20 · deviation · Identity code was behind the contract settled in PR #69

- Symptom: after PR #69 the contract and both implementations differed: the pairing `mac` covered the role and the device id only, so the rendezvous service could change `kind` and `name` of a `pair` message; a `pair` message of any `kind` was accepted; `n` and `g` of a signed payload took any text, an empty one and the escape of a lone surrogate included, which a node and a browser read as different nonces; `crypto.json` had no `pair_messages` and no `kind` and `name` in `pairing`.
- Cause: the rules were settled after B-02b and F-01b merged (`internal/identity/pairing.go:60-64`, `internal/identity/envelope.go:45-51`, `web/src/lib/pairing.ts:46-56`, `web/src/lib/payload.ts:32-38`, all before the change).
- Status: resolved in PR #72 (B-02c with F-01d): the `mac` covers `role`, `k`, `kind` and `name` as `TextEncoder` encodes them (`internal/identity/pairing.go:57-75`, `web/src/lib/pairing.ts:46-72`); a random id is a type that `proto.Decode` and `proto.ts` check (`internal/proto/decode.go:152-157`, `web/src/lib/proto.ts:17-22`); `crypto.json` holds every case `contracts/vectors.md` names and both test suites pass on it.

### 2026-11-20 · deviation · Go identity and the web client answer differently until B-02b

- Symptom: for the same bytes, `internal/identity` and `web/src/lib/identity.ts` can give different answers. The web client follows the contract as settled in PR #58 and the coordinator's rules of 2026-11-20; the Go package follows the contract as it was. Known differences: weak keys as `k`, `dev`, `user`, pair `k` or signature R (web refuses, Go accepts); an empty `to` (web: a value, so `not_for_me`; Go: no `to`); signalling without `to` (web: `not_for_me`; Go accepts); a payload without `t`, `n` or `ts`, or with `null` in a field read (web: `bad_request`; Go: zero values); a key in another case, such as `TO` (web ignores it; Go reads it); an integer written `2.0` (web: 2; Go refuses) and one above 2^53 (web refuses; Go reads it); an `add` without `kind` `node` or `browser` or without a string `name` (web refuses; Go accepts); a `revoke` of the last valid device (web refuses; Go accepts); a long s in a pairing secret (web refuses; Go reads it as S); a pair message whose `k` is not a device id (web ignores it; Go checks only the mac); a signature whose S is not below L (both refuse, the web client before WebCrypto).
- Cause: the rules changed after B-02 merged; the Go package keeps the old behaviour (`internal/identity/key.go:87-91` signature check, `internal/identity/envelope.go:51-62` payload fields, `internal/identity/devlog.go:63-78` and `:93-104` log rules, `internal/identity/pairing.go:36-37` and `:66-68` secret and pair message). The web side: `web/src/lib/payload.ts`, `web/src/lib/identity.ts:126-136`, `:213-219`, `web/src/lib/devlog.ts:98-101`, `web/src/lib/pairing.ts:22-29`.
- Status: resolved in PR #67: `internal/identity` follows the settled contract, `internal/proto/testdata/crypto.json` holds the matching vectors, and `web/src/lib/vectors.test.ts` passes on them. The line references above are those before the change.

### 2026-11-20 · limitation · An empty `to` in a signed payload is read as no `to`

- Symptom: an envelope whose payload carries `"to":""` passes the `not_for_me` check on every receiver, although `contracts/envelope.md` section 3 says a `to` that is present must equal the receiver's device id.
- Cause: `proto.PayloadHeader.To` is a plain string (`internal/proto/envelope.go:17`), so `addressed` cannot tell an empty `to` from a missing one (`internal/identity/envelope.go:56`). Only a device the receiver already listens to can sign such a payload.
- Status: resolved in PR #67, by the rule the contract settled on (`contracts/envelope.md` section 3, check 4): an empty `to` is a value like any other and is nobody's device id, so it is `not_for_me` for signalling and commands alike, and `null` is `bad_request`. The payload is read into a pointer field, which tells an empty `to` from a missing one (`internal/identity/envelope.go:49`, `addressed` at `internal/identity/envelope.go:82`).

### 2026-11-20 · limitation · Go and browser JSON parsers can disagree on a signed payload

- Symptom: not observed. A log entry written by hand by one of the person's own valid devices could be accepted by a node and refused by a browser, or the reverse, which leaves their logs at different versions. Examples: `"ver":2.0` (refused by Go, read as 2 by `JSON.parse`), a key spelled `"VER"` (matched by Go, ignored by a browser). The common fields that `OpenSignal` and `OpenCommand` read disagree the same way: for `{"to":"A","TO":"B"}` Go reads `to` as B and `JSON.parse` as A, and Go also matches Unicode case folds, so `"tſ"` (long s) sets `ts`. Go and `JSON.parse` both keep the last of two equal keys.
- Cause: `entry` reads the payload with `encoding/json` into `proto.DeviceEntry` (`internal/identity/devlog.go:68`), and `addressed` into `proto.PayloadHeader` (`internal/identity/envelope.go:53`); `encoding/json` matches keys without regard to case and refuses a fraction for an integer, and the contract has no canonical JSON. The shared vectors (`internal/proto/testdata/crypto.json`) hold no such payload.
- Status: resolved in PR #67. `internal/identity` reads every payload with `proto.Decode` (`internal/proto/decode.go:40`), as every later task that reads the wire does; it reads as `contracts/conventions.md` section 2 says: names are exact, a field that is always present must be there, `null` is taken only by a field marked "or `null`", and an integer is a number whose nearest double is a whole number of at most 2^53 − 1, so `2.0` is 2. `internal/proto/testdata/crypto.json` holds such payloads in `received` and `devlogs`. What remains is the depth of nesting each parser accepts (`planning/risks.md` K-16).

### 2026-11-20 · limitation · Go verifies signatures of small-order public keys that OpenSSL refuses

- Symptom: for the device id `AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` (the identity point), the signature made of the identity point and a zero scalar verifies over any text in `crypto/ed25519`, so `VerifyChallenge` and `OpenCommand` (with a `known` that admits the id) accept it without any private key. The other small-order points verify such forgeries for part of all texts. Node 24 (OpenSSL) refuses the same signature, so a Go receiver and another Ed25519 implementation can disagree on whether an envelope or log entry verifies.
- Cause: `ed25519.Verify` (`internal/identity/key.go:90`) follows RFC 8032 without refusing small-order or non-canonical public keys, and `contracts/envelope.md` section 2 does not say whether such a key is a device id.
- Status: resolved in PR #67. The fourteen encodings of small-order points are no device id and no first half of a signature (`contracts/envelope.md` sections 2 and 3); `verify` compares the bytes itself before it asks the library (`internal/identity/key.go:91-137`). As `k` such a value is `bad_sig`, as `dev` it makes a log entry refused, and `Secret.Verify` refuses a `pair` message that carries it. `crypto.json` holds a forged signature under each of the fourteen and one by a real key that begins with the identity point. A real key with a small-order point added to it still passes (`planning/risks.md` K-15).

### 2026-11-20 · deviation · Web note rows refuse `pos` until the Go type has it

- Symptom: the settled row contract (PR #58) gives note rows a `pos`; `decode` refuses a note row that carries one.
- Cause: `NoteRow` mirrors the merged Go `NoteRow` and `internal/proto/testdata/row-note.json`, which have no `pos` (`web/src/lib/proto.ts:261-270`).
- Status: resolved in B-01 slice (d). Both wire types require the creation position and the shared note example includes it.

### 2026-11-20 · bug · Web `verify` threw for a `k` or `s` of some hundred thousand characters

- Symptom: `verify`, `isDevice`, `verifyPair` and `DeviceLog.apply` threw `RangeError: Maximum call stack size exceeded` for a `k`, `s`, `dev` or `mac` of 400 000 base64url characters, where the contract wants `bad_sig`, a refused entry or an ignored `pair` message. Under Node 24 a text of 133 000 characters still passed and one of 267 000 threw; the limit differs by engine and was not measured in the browsers. Nothing was accepted that should have been refused. Found in the review of PR #66, before the code was merged.
- Cause: `decode` encodes the bytes it decoded to compare the spelling, and `encode` spread them all into the arguments of one `String.fromCharCode` call (`web/src/lib/identity.ts:18` before the fix).
- Status: resolved in PR #66: `encode` makes the text byte by byte (`web/src/lib/identity.ts:18-19`). Checked under vitest with 400 000 characters.

### 2026-11-20 · limitation · Browsers accept small-order Ed25519 public keys

- Symptom: `crypto.subtle.verify` returns true for the signature R = identity, S = 0 under the identity point as public key, over any message, in Chromium 159, Firefox 159 and WebKit 27.2 (spike S-7, 2026-11-20). WebCrypto requires false for small-order keys; web-platform-tests `eddsa_small_order_points` passes 8, 9 and 9 of 14 in Chrome 158, Firefox 160 and Safari 27.2 (wpt.fyi, 2026-11-19).
- Cause: the engines' Ed25519 libraries do not make the small-order checks ([Firefox bug 1929022](https://bugzilla.mozilla.org/show_bug.cgi?id=1929022), [WebKit bug 280673](https://bugs.webkit.org/show_bug.cgi?id=280673), both open).
- Effect: relying on WebCrypto alone would admit invalid signers. The envelope contract requires rejection before WebCrypto.
- Status: resolved for the web client in F-01b: the settled contract (PR #58, `contracts/envelope.md` section 2) makes the fourteen weak values no device id, and the client compares the bytes itself before WebCrypto is asked, for `k`, `dev`, `user`, a pair message's `k` and the first half of a signature (`web/src/lib/identity.ts:104-136`). The Go side follows in B-02b.

F-01c measurements. The first two results are native `subtle.verify`; the last is SoloDawn's receiver result for **each** of the fourteen weak-key vectors. CI runs four builds on every pull request; the others ran by hand on macOS 26.6.2.

| Browser | Run by | Identity-point vector | Valid empty message (RFC 8032 §7.1 test 1) | Receiver |
| --- | --- | --- | --- | --- |
| Chrome for Testing 137.0.7151.119 | CI, Ubuntu 24.04; by hand, macOS | true | true | `bad_sig` |
| Edge 137.0.3296.93 | CI, Ubuntu 24.04 | true | true | `bad_sig` |
| Firefox 136.0 (Playwright build 1478) | CI, Ubuntu 24.04; by hand, macOS | true | true | `bad_sig` |
| WebKit 27.2 (Playwright build 2370) | CI, macOS 26; by hand, macOS | true | false | `bad_sig` |
| Chromium 156.0.8078.4 (Playwright) | by hand, macOS | true | true | `bad_sig` |
| Firefox 157.0 (Playwright) | by hand, macOS | true | true | `bad_sig` |
| Safari 26.6.2 | by hand, macOS, once, with Vitest's preview provider, which is not in the repository | true | false | `bad_sig` |

No Safari release runs in CI, and Safari 17.5, the baseline, was not run at all. Playwright's WebKit is a current build of Apple's port, so its row shows that the vectors pass on Apple's Ed25519 implementation of today, not on that of Safari 17.5. For Safari 17.5 the baseline still rests on the sources cited in the browser-baseline decision, not on a run.

### 2026-11-20 · limitation · Firefox 136 cannot export the JWK of a seed imported as PKCS #8

- Symptom: on Firefox 136.0, importing an Ed25519 seed succeeds and it signs, but exporting the imported private key as JWK rejects with `OperationError`. The other builds above export the expected public coordinate.
- Effect: the original shared test used that export to construct a key pair. SoloDawn does not export its private device keys.
- Status: handled in F-01c. The suite does not export: it imports the public key of the vector and proves that **each** seed signs for it. No signature bytes are compared.

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16` before the fix), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: resolved in 815f0547d (PR #61). Storage is read and written behind a guard (`web/src/lib/prefs.svelte.ts:17-35`): a failed read, or a value the client does not know, gives the default; a failed write is ignored. Checked under vitest with a `localStorage` getter that throws a `SecurityError`; still not tried in a real browser.

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.

### 2026-11-20 · bug · The store wrote rows with HTML escapes

- Symptom: a note with 16,000 `<` characters was stored as 96,124 bytes, though `planning/contracts/conventions.md` section 4 counts a row's size on its JSON without HTML escaping and the home accepts rows up to 16 KiB on that count. A row the home accepts could be stored above the 60 KiB limit of a `ctl` message, so a `rows` or `fetched` frame built from it could not be sent. No node wrote rows yet, so nothing was broken.
- Cause: `Put` encoded the row with `json.Marshal` (`internal/store/put.go:17` before the fix), which escapes `<`, `>` and `&` as six-byte `<`, `>`, `&`. `internal/peer` encodes frames without escaping and leaves stored escapes as they are.
- Status: resolved in PR #163. `Put` and `PutReceipt` encode through `encode` (`internal/store/put.go:29`, a `json.Encoder` with `SetEscapeHTML(false)`, newline trimmed); `internal/store/put.go:18` and `internal/store/receipt.go:16` call it. Raw JSON the store returns (read paths, signed envelopes in the inbox) is never re-encoded. Test: `TestPutStoresWithoutHTMLEscapes` in `internal/store/read_test.go`. `internal/proto/decode.go:64` has the same `json.Marshal` pattern but is an intermediate that is decoded again at once; not changed (owned by P-1).

### 2026-11-20 · bug · A refused read left the top of the main line loading forever

- Symptom: when the home refused the `sync` of a group with `forbidden` while the group was still listed, the chat top showed the idle loading dots for good instead of "Earlier messages cannot be loaded right now."
- Cause: `Group.sync` returned on `answer.err` without touching the paging (`web/src/lib/groups.svelte.ts:160-170` before the fix), so `more=true` and `failed=false` stayed while the connection was up, and `stuck` in `web/src/components/chat/Chat.svelte:19` stayed false. No page is asked for after a refused read (the group is not live), which is intended.
- Status: resolved in PR #169: a refused read sets `paging().failed` (`web/src/lib/groups.svelte.ts:163`), a read that comes too late does not. The next successful read starts a fresh paging. Tests in `web/src/lib/groups.test.ts` (refused read, late refusal).

### 2026-11-20 · bug · `plain` copied its list at every mention

- Symptom: 2300 mentions in one text took about 69 ms to cut into parts, and it runs on every key press; 6000 overflowed the stack.
- Cause: `plain` (`web/src/lib/compose.ts` before the fix) was recursive and spread the result of the rest of the text into a new list at each mention, so the cost grew with the square of the mentions and the depth with their number.
- Status: resolved in PR #169: `plain` builds the list with a loop (`web/src/lib/compose.ts:53`), same output. Test in `web/src/lib/compose.test.ts` (2300 mentions give the same parts; 6000 take under ten times what 1500 do).

### 2026-11-21 · deviation · Seven error paths in `internal/store` had no unit tests and two `Scan` checks were unreachable

- Symptom: `go test -cover ./internal/store/` reported 93.7% of statements, leaving nine error-return blocks uncovered across `inbox.go`, `read.go` and `store.go`.
- Cause: unit tests did not exercise locked or closed database paths in `Enqueue` (`internal/store/inbox.go:20-22`), `Inbox` (`internal/store/inbox.go:35-37`), `Snapshot` (`internal/store/read.go:20-22`, `:26-28`, `:31-33`), `Snapshot.rows` (`internal/store/read.go:72-74`) and `transaction` (`internal/store/store.go:78-80`); and `Inbox` (`internal/store/inbox.go:44-46` before the fix) and `Snapshot.rows` (`internal/store/read.go:79-81` before the fix) checked `rows.Scan` errors that cannot occur. Both `inbox` and `row` are SQLite `STRICT` tables (`internal/store/schema.sql:1-17`, `:31-44`; `planning/contracts/storage.md` section 1) whose selected columns match the non-nil Go destination types (`INTEGER NOT NULL` and `INTEGER PRIMARY KEY` to `*int64`, `TEXT NOT NULL` to `*string` or `*[]byte`, nullable `TEXT` to `**string` or `*[]byte`), while `modernc.org/sqlite` steps and reads column values inside `rows.Next()`, surfacing any SQLite or I/O failure through `rows.Err()`.
- Status: resolved in #174: removed the two unreachable `rows.Scan` error branches in `internal/store/inbox.go` and `internal/store/read.go`, and added unit tests covering all seven reachable error paths (`TestInboxFailures` in `internal/store/delivery_test.go`, `TestSnapshotAndQueryFailures` in `internal/store/read_test.go`, `TestWriteBeginFailure` in `internal/store/store_test.go`), bringing `internal/store` to 100.0% statement coverage.
