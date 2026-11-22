# Contract: more CLIs (step 3)

What a driver for a further CLI must have, the capability table it
declares, and what the node switches off where a capability is missing.
It refines `steps/step-2-3.md` 3.1. The two step 1 drivers are in
`contracts/drivers.md`; sessions in `sessions.md`; takeover in
`flows/takeover.md`; approvals in `flows/approval.md`.

This is a new file so that no step 1 contract has two writers while step 1
is built. It is folded into `contracts/drivers.md` afterwards; section 5
also into `contracts/commands.md` section 2 and `contracts/mcp-tools.md`
section 3, and section 6 into `contracts/datachannel.md` section 7 and
`contracts/rows.md` section 2. Sections 1 to 9 change the meaning of no
step 1 shape and of no step 1 driver; section 10 adds fields to step 1
shapes and passes a model at each start.

Markers: `[verified]`, `[documented]`, `[unverified]` as in
`contracts/drivers.md`.

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
| an approval channel that is not a terminal prompt: the CLI's own permission request reaches the node and waits for its answer (section 4, `approvals`) | R-42, R-43 | A4.1, A4.2 |
| level 1: settings under which the session changes no file | every group session, review session and secretary is level 1 (`sessions.md`) | A4.4 |
| repository configuration loaded in task and review sessions and kept out in group sessions (owner's decision, 2026-11-22) | R-45, Q-24 | A3.8 |

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
| `approvals` | `tool` or `conn` | how the required approval channel (section 3, R-43) reaches the node. `tool`: the CLI calls the node's MCP tool `approve`. `conn`: requests arrive on the driver's own connection | never missing: a CLI without a channel is not built |
| `takeover` | `new`, `resume` or `none` | an interactive command that resumes the same session in a terminal, with the level, the repository setting and the node's MCP server holding there as in structured mode. `new`: it can also start a session there with an id the node chose | `none`: no "open terminal" for this AI, and nothing to watch (step 2). `resume`: a working AI without an unfinished task gets its `assign` turn in structured mode first (`flows/takeover.md` A) |
| `usage` | list of `tokens`, `usd`, `window_pct`; may be empty | the CLI reports that figure in structured mode | a cap in a unit not listed cannot be set. `turns` always can: the node counts them |

Rules for a declaration:

- `levels` holds `2` only with a sandbox; permission rules without one do
  not stand for level 2. Decided for Claude Code on native Windows, where
  the vendor provides no sandbox (`risks.md` Q-4): the node refuses level 2
  there and says why. WSL2 is a later option.
- `takeover` is not `none` only if all of it holds. A terminal that drops
  the level or loads the repository's configuration in a group session is no
  takeover.
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
| the tool list of a session | `approve` is listed only to a structured session of a driver with `approvals` `tool` (`contracts/mcp-tools.md` section 3) |
| `takeover.start` | refused with `conflict` when `takeover` is `none`, before the checks that answer `relay`, `busy` or `cli` |
| `takeover.start` for a working AI without an unfinished task | `new`: the session starts in the terminal. `resume`: the structured `assign` turn comes first |

- Each refusal names the field of the declaration in `msg`.
- No switch reads `levels` or `usage` in 1.0: `perm.set` and the caps of
  `settings.set` are postponed, and R-44 is not applied (coordinator,
  2026-11-22). They are declared all the same, for when those return.
- Nothing else in the node asks which CLI a session runs. A difference
  between CLIs that is not in this table lives inside the driver, behind
  the five operations.

## 6. On the wire

One addition: each entry of `agents` in `host.info`
(`contracts/datachannel.md` section 7) carries the declaration as the
always-present object `driver`, with the `accounts` and `takeover` of section 4.
The name is not `caps`: that field holds the owner's usage caps.

```json
{"agent":"codex","cli_version":"0.164.0","account":"team","login_ok":true,
 "caps":{},
 "driver":{"accounts":["personal","team","api_key"],"takeover":"resume"},
 "groups":[]}
```

| Field | Presence | The web client uses it for |
| --- | --- | --- |
| `accounts` | always, never `[]` | the account types offered when an AI is registered |
| `takeover` | always | "open terminal" is absent when it is `none` |

- The other fields of section 4 stay in the host's own table and go on the
  wire when level control or caps return.
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
  the coordinator decided on 2026-11-20. The interface unit U3-3 shows it:
  the account choice and the terminal button (2026-11-22). The parallel plan lists both; T3-1 puts nothing on the
  wire, and T3-2 puts `driver` into `host.info` after P-5.

## 7. Acceptance

For each CLI that was built, the tests of M3, M4 and M7
(`steps/step-1-acceptance.md`) run with its fake CLI, and their `live`
parts once with the real one. A missing capability takes tests out, and
the test of the switch takes their place:

| Declaration | Step 1 tests that do not run for this CLI |
| --- | --- |
| no level 2 | A4.1, A4.2, A4.3, A4.6, A4.10, and the last sentence of A4.7: without level 2 no approval card can arise (`contracts/drivers.md` section 2) |
| no level 3 | A4.5 |
| `takeover` `none` | A7.1 to A7.9 |
| `takeover` `resume` | A7.7 runs with the structured `assign` turn first |

- Containment at level 2 is tested on every driver that declares it,
  through refusals: A4.2 and A4.10, where a refused request leaves no
  effect on disk.
- The switches of section 5 are tested once, with declarations made up in
  the test: no shipped CLI is needed to show a refusal. Those are the
  capability tests among A10.x of `steps/step-3-acceptance.md`.
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
| `opencode` (T3-2) | `api_key` | `1`, `3` `[unverified]` | `conn` `[unverified]` | `none` | none |

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
3. Decided: every driver has an approval channel (section 3);
   `approvals` `none` does not exist.
4. `[unverified]` The values on Windows, for the step 1 drivers too
   (spike S-5).
5. The capability tests are A10.1 to A10.4 of
   `steps/step-3-acceptance.md` (unit C2-0).
6. The answers of `steps/parallel-plan-step-2-3.md` section 1 that this
   file rests on: D-16 only. R-44 is not applied in 1.0.
7. Decided on 2026-11-20: the approval channel is required (point 3).

## 10. Models: discovery, suggestion and start

Added 2026-11-22 (R-165). The member fields and what the owner does with
them are in `contracts/step-3/profiles.md`; this section owns what the
host finds, what it suggests and what it passes to the CLI. It changes one
sentence of `contracts/drivers.md` section 1: a model is passed.

`[verified]` here means run by the reviewer of this contract on
2026-11-22, with Claude Code 2.1.311 and Codex 0.164.0.

### 10.1 A sixth operation: list models

Each driver lists the models its CLI offers under the signed-in account,
by the CLI's own listing and nothing else. The host adds no model, drops
none and keeps no list of its own, neither of models nor of levels, so a
new model appears without a release of SoloDawn.

| `agent` | How the list is read | Model id, name | Levels | Fast mode |
| --- | --- | --- | --- | --- |
| `claude` | start `claude -p --verbose --input-format stream-json --output-format stream-json --no-session-persistence --strict-mcp-config --settings '{"disableAllHooks":true}'`, write one `initialize` control request, read `models` of the `control_response`, close stdin. `[verified]`: it lists the models (14) and calls no model. The field names are `[documented]`, from `ModelInfo` of the Agent SDK | `value`, `displayName` | `supportedEffortLevels`, when `supportsEffort` | `supportsFastMode` |
| `codex` | `model/list` on `codex app-server`, after `initialize` `[verified]` (8 models); asked again with `cursor` = the answer's `nextCursor` until there is none; hidden entries are not asked for `[documented]` | `model` (not `id`, the name of the catalog entry), `displayName` | `supportedReasoningEfforts[].reasoningEffort`, in the order given | `serviceTiers` is not empty. `[unverified]` that every such tier is a faster one |
| `opencode` | `GET /api/model` of `opencode serve`; the same list as `opencode models --verbose` `[documented]`; the fields are `[unverified]` | `provider/model`, its name | the model's variants | — |
| `qwen` | not read. No listing is documented for headless mode, and its settings file can hold a credential, which the node never reads (`contracts/drivers.md` 1.4, R-50) | — | — | — |

- **A CLI without a listing**, a listing that fails, and a list in a shape
  the driver does not know all give the empty list. The host guesses
  nothing: the owner types the model's id and the level
  (`contracts/step-3/profiles.md` section 2a).
- A claim not marked `[verified]` is captured from the real CLI by the
  unit that builds it; a driver whose listing does not work as written
  here declares the empty list until it does (section 4, last rule).
- **When.** With detection (`contracts/drivers.md` 1.4): at node start and
  on a `host.info` request, kept for the same 60 s. A `host.info` request
  with `"refresh":true` skips what was kept; that is the owner's refresh.
  Nothing is stored on disk.

### 10.2 The suggested tier

The host ships one table and suggests a tier per listed model. The
model's id, in lower case, is cut into words at every character that is
not a letter or a digit; the first row that holds one of those words
gives the tier; no row, `standard`. So `gemini-3-pro` has the words
`gemini`, `3`, `pro` and is not `mini`.

| A word of the id | Tier |
| --- | --- |
| `haiku`, `mini`, `nano`, `flash`, `lite` | `light` |
| `opus`, `fable` | `strong` |

- Claude Code's entries are matched on `resolvedModel`, not on `value`:
  its entry `default` then gets the tier of the model it stands for.
- The table is data in one file of the host. T3-9 completes it for the
  models the CLIs list on the day it is built, each row with its source;
  a wrong row costs one click, since the owner sees the suggestion before
  anything is stored.
- No `skills` are suggested.

### 10.3 On the wire (adds to section 6)

Each entry of `agents` in `host.info` carries `models`, always present.
`driver` does not change.

```json
{"agent":"claude", …,
 "models":[{"model":"opus","name":"Opus","efforts":["low","medium","high","xhigh","max"],
            "fast":true,"tier":"strong"},
           {"model":"haiku","name":"Haiku","efforts":[],"tier":"light"}]}
```

| Field | Presence | Meaning |
| --- | --- | --- |
| `models` | always; `[]` when nothing was listed | what 10.1 found, in the CLI's order |
| `models[].model` | always | what `member` `model` takes |
| `models[].name` | always | the CLI's display name; the id when it gives none |
| `models[].efforts` | always, may be `[]` | the levels of that model, in the CLI's order |
| `models[].fast` | only when `true` | the CLI says the model has a fast mode. Shown; it cannot be chosen |
| `models[].tier` | always | the suggestion of 10.2 |

`groups` of an agent may hold several entries of one `g`, one per member.

### 10.4 Starting with the choice

At every start of a CLI process the driver passes the member's `model`
and `effort`; an empty value passes nothing, as in step 1.

| `agent` | Model | Level | In the terminal (`contracts/drivers.md` 1.3) |
| --- | --- | --- | --- |
| `claude` | `--model <model>` `[verified]` | `--effort <effort>` `[verified]` | the same two flags |
| `codex` | `model` of `thread/start` and `thread/resume` `[verified]` against the generated schema of 0.164.0 | `config.model_reasoning_effort` of the same `[documented]` | `-m <model>` and `-c model_reasoning_effort="<effort>"` on `codex resume` `[documented]` |
| `opencode` | `POST /api/session/{id}/model` before the first prompt of a process `[documented]` | the variant in the same selection, `provider/model#variant` `[unverified]`: version 1 documents `--variant` instead | no takeover (section 8) |
| `qwen` | `startupConfig.modelServiceId` of `POST /session`, behind the capability `session_startup_config`; `--model` in stream-json mode `[documented]` | `startupConfig.reasoningEffort` `[documented]` | no takeover (section 8) |

- A changed model or level applies at the next start, a resume included:
  the session and its history stay. There is no fallback to a new
  session.
- **A value the CLI refuses.** Whether the CLI exits (Claude Code) or
  answers the start with an error and stays up (Codex's `thread/start`
  and `thread/resume` `[verified]` against the schema of 0.164.0;
  OpenCode's HTTP error `[documented]`), the driver handles it as row 1
  of `flows/agent-turn.md` E: `turn_interrupted` with `crash`, `task
  fail` in a task session, state `dormant`, and a CLI process that
  stayed up is closed. It is not "cannot resume the session id": the
  session is not replaced and keeps its history. The owner reads the
  notice and corrects the member.

### 10.5 What depends on a CLI exposing its list

Only the choice from a list and the suggested tier. With the empty list
(Qwen Code, always) the owner types model and level, sets tier and
skills, and everything of `contracts/step-3/profiles.md` sections 3 and
4 works.
