# Contract: more CLIs (step 3)

What a driver for a further CLI must have, the capability table it
declares, and what the node switches off where a capability is missing.
It refines `steps/step-2-3.md` 3.1. The two step 1 drivers are in
`contracts/drivers.md`; sessions in `sessions.md`; takeover in
`flows/takeover.md`; approvals in `flows/approval.md`.

This is a new file so that no step 1 contract has two writers while step 1
is built. It is folded into `contracts/drivers.md` afterwards. Nothing here
changes the meaning of a step 1 shape or of a step 1 driver.

Markers: `[verified]`, `[documented]`, `[unverified]` as in
`contracts/drivers.md`. **Gap** marks what waits for research unit T3-0.

## 1. When a driver is built

- Drivers are built one at a time, and only for a CLI that passed both
  checks of research unit T3-0: the terms (section 2) and the required
  capabilities (section 3). A CLI that fails one of them waits; nothing is
  built on a claim that was not checked.
- Each claim is recorded with its source and the version of the CLI it was
  read for. CLIs change monthly, so a claim is checked again when the work
  on that driver starts.
- Acceptance (section 7) covers only the CLIs that passed and were built.
- Among the CLIs that passed, one that passed for API keys comes first.

## 2. Terms

The question, per account type of R-50: do the vendor's terms allow its
official program, under the user's own sign-in or key, to be started by
another program?

- The answer is the list `accounts` of the declaration (section 4). A type
  is listed only when the terms allow it for every model the CLI can reach
  under that type. "API key only" is the list `["api_key"]`; R-54 is this
  case.
- An empty list means the CLI is not offered, and no driver is built.
- The node does not verify the account type and reads no credential (R-50).
  The list limits what the owner can declare, nothing else.

## 3. What every driver has

None of these can be switched off: the product does not work without them,
and a step 1 acceptance test shows each. A CLI that lacks one waits.

| Required | Why | Shown by |
| --- | --- | --- |
| a documented structured mode in which one process takes turn after turn and reports the end of each | R-1, R-120, R-124 | A3.1 |
| a session resumed by its id in that mode, with its history | R-121 | A3.4 |
| an MCP client that takes, at each start, one stdio server with its arguments, and calls that server's tools without a request for each call | the session token names the caller; `post` is the only way an AI speaks (`contracts/mcp-tools.md`) | A3.1, A3.7 |
| level 1: settings under which the session changes no file | every group session, review session and secretary is level 1 (`sessions.md`) | A4.4 |
| repository configuration kept out unless the owner enabled it | R-45 | A3.8 |

The driver itself is what `steps/step-2-3.md` 3.1 lists, as for the two
step 1 drivers:

- five operations: start or resume, send a turn, events (turn end with
  usage, approval request, exit), interrupt, close. After an interrupt the
  session can take its next turn or be resumed;
- the instructions of `contracts/drivers.md` section 4 passed at session
  start, and the turn text of its section 3, unchanged but for the one
  sentence that section 5 of this file replaces;
- model, permission and sandbox settings passed explicitly at every start;
- detection by the program's own version command and sign-in failure
  reported as `login_invalid`, as in `contracts/drivers.md` 1.4;
- the level mapping: the settings for each level the driver declares, with
  what was measured for each, written like the level tables of
  `contracts/drivers.md` 1.1 and 1.2;
- a fake CLI scripted from captures of the real one, as the two of step 1
  (`internal/host/fakecli/`).

## 4. The capability table

Each driver declares one value, fixed in its code. A field may differ by
operating system; the driver then declares the value of the system the
node was built for. The table holds only what a switch of section 5 reads.

| Field | Value | The capability | Without it |
| --- | --- | --- | --- |
| `accounts` | list of `personal`, `team`, `api_key`; at least one | the terms allow that account type (section 2) | a type not listed cannot be declared for this CLI |
| `levels` | list of `1`, `2`, `3`; always holds `1` | `2`: a sandbox of the CLI's own that lets it write inside the session's folder and stops writes outside it. `3`: a mode in which the CLI asks nothing | a level not listed is not offered. Without `2` and `3` the AI can be a secretary, a reviewer and answer questions, and cannot produce a submission (`contracts/drivers.md` section 2) |
| `approvals` | `tool`, `conn` or `none` | an approval channel that is not a terminal prompt (R-43). `tool`: the CLI calls the node's MCP tool `approve`. `conn`: requests arrive on the driver's own connection | `none`: this AI produces no approval card. At level 2 what its sandbox stops stays stopped; the owner raises the level or takes over the terminal |
| `takeover` | `new`, `resume` or `none` | an interactive command that resumes the same session in a terminal, with the level, the repository setting and the node's MCP server holding there as in structured mode. `new`: it can also start a session there with an id the node chose | `none`: no "open terminal" for this AI, and nothing to watch (step 2). `resume`: a working AI without an unfinished task gets its `assign` turn in structured mode first (`flows/takeover.md` A) |
| `usage` | list of `tokens`, `usd`, `window_pct`; may be empty | the CLI reports that figure in structured mode | a cap in a unit not listed cannot be set. `turns` always can: the node counts them |

Rules for a declaration:

- `levels` holds `2` only with a sandbox; permission rules without one do
  not stand for level 2. Decided for Claude Code on native Windows, where
  the vendor provides no sandbox (`risks.md` Q-4): the node refuses level 2
  there and says why. WSL2 is a later option.
- With `approvals` `none`, `levels` holds `2` only if the CLI can be
  started so that an action its sandbox stops fails by itself: no prompt,
  and no wait for one.
- `takeover` is not `none` only if all of it holds. A terminal that drops
  the level or loads the repository's configuration is no takeover.
- A value is declared from what was run with the real CLI. A capability
  known only from documentation is declared as missing until a capture
  shows it.

What needs no field, because no switch reads it: a CLI that reports no
vendor rate limit never causes `rate_limited` or the state `limited` for
that reason; its turn fails as the CLI fails it (`flows/agent-turn.md` E).

## 5. The switches

The host enforces the declaration; the web client only stops offering what
would be refused. The home knows nothing of it, and no row changes.

| Where | What the declaration switches |
| --- | --- |
| `agent.set` | an `account` not in `accounts` is refused with `bad_request` |
| `perm.set` | a `level` not in `levels` is refused with `bad_request` |
| the level of a task session | the highest level in `levels` that is not above the level the other rules give: `contracts/commands.md` section 2, and from step 2 the cap of R-44 |
| `settings.set` | a cap for this CLI whose `unit` is neither `turns` nor in `usage` is refused with `bad_request` |
| the tool list of a session | `approve` is listed only to a structured session of a driver with `approvals` `tool` (`contracts/mcp-tools.md` section 3) |
| the instructions of a task session | with `approvals` `none`, the sentence on asking through the CLI's own permission request is replaced by: "When the sandbox blocks something the task needs, say so with `post` and end your turn; do not work around it." |
| `takeover.start` | refused with `conflict` when `takeover` is `none`, before the checks that answer `relay`, `busy` or `cli` |
| `takeover.start` for a working AI without an unfinished task | `new`: the session starts in the terminal. `resume`: the structured `assign` turn comes first |

- Each refusal names the field of the declaration in `msg`.
- `approvals` `none` needs no refusal: no request ever reaches the node.
  The owner reads what was stopped in the group, and raises the level with
  `perm.set` or takes over, as the owner decides.
- From step 2, a task that someone other than the owner caused runs at
  level 2 at most (R-44). On a CLI whose `levels` are `1` and `3` that is
  level 1: such a task can be read and reported on, and not submitted.
- Nothing else in the node asks which CLI a session runs. A difference
  between CLIs that is not in this table lives inside the driver, behind
  the five operations.

## 6. On the wire

One addition: each entry of `agents` in `host.info`
(`contracts/datachannel.md` section 7) carries the declaration as the
always-present object `driver`, field for field as section 4 gives it.
The name is not `caps`: that field holds the owner's usage caps.

```json
{"agent":"codex","cli_version":"0.164.0","account":"team","login_ok":true,
 "caps":{},
 "driver":{"accounts":["personal","team","api_key"],"levels":[1,2,3],
           "approvals":"conn","takeover":"resume","usage":[]},
 "groups":[]}
```

| Field | Presence | The web client uses it for |
| --- | --- | --- |
| `accounts` | always, never `[]` | the account types offered when an AI is registered |
| `levels` | always, never `[]` | the levels offered where a level is chosen |
| `approvals` | always | the sentence beside level 2 when it is `none`: nothing beyond the workspace can be approved for this AI |
| `takeover` | always | "open terminal" is absent when it is `none` |
| `usage` | always; `[]` when the CLI reports none | the cap units offered beside `turns` |

- `agent` in `host.info` and `cli` in a member row (`contracts/rows.md`
  section 2) take one new value per driver: the name the driver declares,
  made of `a` to `z`, `0` to `9` and `_` (`contracts/conventions.md`
  section 2), because it is a key of `caps` in `settings.set`.
- The web client shows no CLI-specific text beyond that name. What it
  offers follows `driver`, so a new driver needs no change in the client.
- Other members see nothing of the declaration. An AI that cannot do
  something says so in the group, like any member.
- `driver` is a wire shape, so lane PROTO freezes it in both languages in
  one pull request (`contracts/conventions.md` section 5): unit P-5, as
  the coordinator decided on 2026-11-20. The Fable interface unit U3-3
  shows it: the account choice, the level control, the cap units and the
  terminal button. The tables of the parallel plan do not list either yet.

## 7. Acceptance

For each CLI that was built, the tests of M3, M4 and M7
(`steps/step-1-acceptance.md`) run with its fake CLI, and their `live`
parts once with the real one. A missing capability takes tests out, and
the test of the switch takes their place:

| Declaration | Step 1 tests that do not run for this CLI |
| --- | --- |
| no level 2 | A4.1, A4.2, A4.3, A4.6, A4.10 |
| level 2 with `approvals` `none` | A4.1, A4.2, A4.3, A4.6. A4.10 runs with the outcome of section 4: no card, the write fails within the turn, and no file exists |
| no level 3 | A4.5 |
| `takeover` `none` | A7.1 to A7.9 |
| `takeover` `resume` | A7.7 runs with the structured `assign` turn first |

- The switches of section 5 are tested once, with declarations made up in
  the test: no shipped CLI is needed to show a refusal. Those are the
  capability tests among A10.x of `steps/step-2-3-acceptance.md`.
- A `live` part needs the owner's key for that CLI. Until it has run, the
  driver's level tables carry `[unverified]` where step 1 carries
  `[verified]`.

## 8. The CLIs

The two step 1 drivers, from `contracts/drivers.md`
(`[verified]` on macOS with the versions named there):

| `agent` | `accounts` | `levels` | `approvals` | `takeover` | `usage` |
| --- | --- | --- | --- | --- | --- |
| `claude` | all three | `1`, `2`, `3`; on native Windows `1`, `3` (`risks.md` Q-4) | `tool` | `new` | `[unverified]` |
| `codex` | all three | `1`, `2`, `3` | `conn` | `resume` | `[unverified]` |

`usage` of both waits for what the usage unit of step 1 reads from the
events (`contracts/rows.md` section 2 marks the figures `[unverified]`).

**The step 3 candidates**, from research unit T3-0 (pull request 91; its
sources were read and spot-checked on 2026-11-20). The source of each claim
is in that description. `[documented]` means read there and not run.

| CLI, version read | Headless mode | Resume by session id | Takeover | MCP client | Approval request and answer | API key |
| --- | --- | --- | --- | --- | --- | --- |
| OpenCode 1.19.6 | `[documented]` `opencode serve`: a session HTTP API and an event stream | `[documented]` | documented, not measured: off | `[documented]` | `[documented]` a pending request on the event stream, answered by its id over HTTP | a direct provider key |
| Qwen Code 0.28.0 | `[documented]` `--output-format stream-json`; the daemon protocol | `[documented]` `--resume <id>` | documented, not measured: off | `[documented]` | `[documented]` the daemon's `permission_request` event, answered by a POST | a QwenCloud pay-as-you-go key |
| Amp | deferred | | | | none for an outside program, so it cannot meet R-43 | |

- Units: T3-2 builds the OpenCode driver and T3-3 the Qwen Code driver, one
  at a time. T3-4 (Amp) is not scheduled.
- For both, `accounts` is `["api_key"]`: their other account types did not
  pass T3-0's terms check. `takeover` is `none` until a capture shows the
  same-id handoff. Every other value is declared from that unit's captures
  (section 4), so neither has a row in the table above yet.

## 9. Open points and what is not verified

1. Decided: P-5 freezes `driver` and U3-3 shows it (section 6).
2. `[unverified]` Every capability of OpenCode and Qwen Code: T3-0 read
   them, and nothing was run.
3. `[unverified]` That any CLI without an approval channel can be started
   so that a stopped action fails by itself (section 4). If none can,
   `approvals` `none` always comes with no level 2.
4. `[unverified]` The values on Windows, for the step 1 drivers too
   (spike S-5).
5. The capability tests are A10.1 to A10.4 of
   `steps/step-2-3-acceptance.md` (unit C2-0).
6. The answers of `steps/parallel-plan-step-2-3.md` section 1 that this
   file rests on: D-16 only. The level cap of D-1 is used as R-44 states
   it.
7. **Open: is an approval channel required?** Amp is deferred because it has
   no approval channel, yet section 4 lets a driver declare `approvals`
   `none`. Needs the coordinator's word. Recommended: move the channel to
   the required list of section 3 and drop `none`, with its instruction
   sentence (section 5), its A4.10 row (section 7) and point 3 above.
