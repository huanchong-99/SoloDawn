# Parallel plan: steps 2 and 3

The design points with their answers, the units of step 2 as lanes that
can be built at the same time, the units of step 3, the questions left for
the coordinator, and the cuts of 2026-11-22. The outline being refined is
`steps/step-2-3.md`; lane PROTO and the file-ownership rules are in
`steps/parallel-plan.md`.

The owner's rules of 2026-11-21 and 2026-11-22 override everything written
before them here: least code; no attack-and-defence code (security comes
from tunnels and firewalls, in the deployment guide); members of a group
trust each other, and the home of a group decides; identity the features
need stays (signed commands say who did what; roles are enforced by the
home); every AI session of 1.0 runs at the highest level, with no level
control; the project's own AI configuration is loaded in task and review
sessions, never in group sessions.

## 1. Design points and answers

| # | Point | Answer |
| --- | --- | --- |
| D-1 | Who caused a task, for the level cap of R-44 | Not in 1.0 (decided 2026-11-22): R-44 is not applied; every session runs at the highest level (`contracts/step-2/authority.md` section 5) |
| D-2 | What a host requires before it acts for a home that is not its owner's | Cut: the host takes its home's items as in step 1 |
| D-3 | Check commands on other people's machines | Cut: no acceptance and no gate. The client shows a group's commands before an AI joins it (`contracts/step-2/checks-forge.md` section 1) |
| D-4 | Stolen-device race in the device log | Cut: the step 1 rule stays, `risks.md` K-4 a stated limit |
| D-5 | What a browser trusts | Cut: a browser shows what the home pushes, as in step 1 |
| D-6 | A secretary on a personal subscription | No mechanism. The interface warns; the documentation recommends a team plan or key (`contracts/step-2/browser-trust.md` section 1) |
| D-7 | Invitation | The link carries group id, home device id and a one-time secret; joining reuses the pairing room and `mac` unchanged, and the home treats the peer as joining. Roles: `owner` (one, the home's owner), `admin`, `member` (`contracts/step-2/people.md`) |
| D-8 | The `confirm` card | One card kind for three requests: directing someone's personal-subscription AI, calling it with `ask`, and watching its terminal. `act confirm` names the card; the home queues the item (`contracts/step-2/confirm.md`) |
| D-9 | Messages from others after a task started (R-52) | Delivered with the label `information`, read from the rows the home sent (`contracts/step-2/labels.md`) |
| D-10 | Other forges | Per group: four templates (push, open request, read checks, merge) as argument lists with named places, never a shell string; the home checks them (`contracts/step-2/checks-forge.md`) |
| D-11 | Moving a group; a copy for safekeeping | One export on the old node and one import on the new; a copy is an export, scheduled by the owner; clients follow the error `moved` (decided 2026-11-22; `contracts/step-2/export.md`) |
| D-12 | Checks in a sandbox | Cut: none in 1.0; `risks.md` K-8 stays a stated limit |
| D-13 | Approver delegates (R-42) | The `approvers` of the member row, set by the owner with `member set` of step 1; the home checks the row when a decision comes (`contracts/step-2/authority.md` section 3) |
| D-14 | Watching a terminal (R-81) | A receive-only channel `watch:*` served by the AI's host on a `direct` path, opened after a `confirm` card with `what: watch`; the home's `watch` item gives the grant (`contracts/step-2/watch-team.md`) |
| D-15 | Team-wide group | Group kind `team`: tasks, briefs and AIs refused; a secretary posts a `bulletin` there to a team group its node knows (`contracts/step-2/watch-team.md`) |
| D-16 | Which CLIs first (step 3.1) | OpenCode and Qwen Code, API keys only; Amp deferred (`contracts/step-3/drivers.md` section 8) |
| D-17 | Style presets (step 3.2) | A note of type `preset` per group; a brief names one; its text is put into the `assign` turn |
| D-18 | Knowledge base (step 3.3) | Notes of type `kb` with a full-text index on the home; tools `kb_put` and `kb_search`; exempt from retention |
| D-19 | TCP simultaneous open (step 3.4) | Not in version 1 |
| D-20 | Agents without a driver (owner, 2026-11-19 and 2026-11-23) | An external AI: the node's MCP server under a member id, `solodawn wait` and `solodawn done` for the agent's own runner, a setup prompt the browser shows; the node names the folder and never starts the agent. No further driver (`contracts/step-3/external.md`) |

## 2. Contracts

The contract units C2-0 to C2-5, C2-7, C3-1 and C3-2 are merged and cut
to the rules above by this file's pull request. C2-6 (moving and copying,
pull request #166) is replaced by `contracts/step-2/export.md`, written in
this pull request; the coordinator closes #166. Spikes S-10 and T3-0 have reported; S-10 is not used, since
D-12 is cut.

## 3. Step 2 in lanes

Each lane is split by layer so that its units run at the same time:

- **Backend** units implement the contract in Go.
- **Web** units build against the contract with the mock transport
  (`web/src/lib/mock.ts`) and the golden wire examples; they do not wait
  for the backend.
- **Acceptance** units write their scenarios and steps against the
  contracts now; the scenarios turn green as the parts merge.
- A shape two units share is fixed first: wire shapes by lane PROTO (P-4a,
  P-4b), a Go seam by a types-only pull request (X2-1). Those merge before
  their lane fans out.

Paths are short as in `steps/parallel-plan-units.md` (`home/`, `host/`,
`node/`, `lib/`, `cmp/`, `routes/`). Every unit also owns its own
`features/<module>_<slice>.feature` and step file. Where a unit must change
a step 1 file, it is named in "Owns" and the unit is its only writer while
it runs. Sizes: S about 50 to 160 lines of product code, M 220 to 400, L
about 700. "Start" says when a unit can begin once this plan is merged.

### 3.1 Lane PROTO: shapes first

Strictly serial (`steps/parallel-plan.md`), so P-4a, P-4b and step 3's
P-5 run one at a time. If Gemini's step 3 units are waiting for P-5, P-5
goes first.

| Id | Does | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- |
| P-4a | Freeze people and confirm: `invite` and its result, `join`, `member set` with `role`, the `system` bodies `member_joined` and `member_removed`, `home.add` and `home` in `host.info.homes` (`people.md`); kind `confirm` with its body and four states, `act confirm` and `act deny` on it, the call status `denied`, `ask` returning `{"denied":true}`, `push` with `confirm`, `wait.kind` `confirm` (`confirm.md` sections 3 to 6); `hello` with `v` 2 (`contracts/step-3/knowledge.md` section 8). Golden files and mock answers | lane PROTO files | — | M | now |
| P-4b | Freeze watch, team, forge and export: item `watch`, `watch.start` and its answer, the label `watch:<g>:<member>`, `watchers` in `host.info`, `watch.end` (`watch-team.md` sections 2 to 5); group kind `team`, kind `bulletin` and its body (sections 7 to 10); `forge: custom` and `templates` in the group row, `group create` and `group set` (`checks-forge.md` section 2); the error `moved` with `home` (`export.md` section 3) | lane PROTO files | P-4a | S | after P-4a |

### 3.2 Lane PEOPLE: invitations, joining, roles, removal

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| X2-1 | Types-only seam for joining: the field of a command's context that says a connection is joining, for which group, with which presented log; the hook the home calls for `invite` | build passes | `home/people_seam.go` | — | S | now |
| T2-1a | Home: `join`, also by a member after a restore (`export.md` section 4), `member set` with `role`, `member remove` of a person with its `device` and `push_sub` rows (`people.md` sections 1, 2, 6) | A9.3, A9.4 (home half) | `home/cmd_people.go` | P-4a, X2-1 | M | after P-4a |
| T2-1b | Node: `invite` (the secret, the room, the link, in memory), joining connections (`people.md` sections 3 to 5), device logs of several people, `home.add`, also replacing the home of a group with an AI here, `hello` `v` 2 with its fallback | A9.1, A9.2, A9.6 (node half) | `node/admit.go`, `node/version.go`; names `node/pair.go`, `node/devlog.go` | P-4a, X2-1 | M | after P-4a |
| W2-1 | Browser: open a `#join=` link, join the room as in a pairing, present the log, send `join`, then `home.add` to the person's own nodes; `hello` `v` 2 with its fallback | unit | `lib/join.ts`, `lib/version.ts` | P-4a | M | after P-4a |
| U2-1 | Members and roles, the invitation dialog with its countdown, the join page, removal | A9.1 to A9.4 as seen | `cmp/people/`, `routes/Join.svelte` | P-4a; the join page also W2-1 | M | after P-4a |

### 3.3 Lane INSTRUCTIONS: the `confirm` card, allow lists, delegates, labels

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| T2-2a | Home: when to ask (`confirm.md` section 2) for mentions and calls; the card, `act confirm` and `act deny`, expiry, reminders; the call's denial; approvals by the current `approvers` of the row (`authority.md` section 3) | A9.8, A9.9, A9.14, A9.18, A9.19 (home half) | `home/cmd_confirm.go`; names `home/cmd_post.go`, `home/cmd_card.go` | P-4a | M | after P-4a |
| T2-2b | Home: a role needs its owner's word (`confirm.md` section 6): the card at task-machine rows 1 and 11, the wait `confirm`, items after a confirmation, a kept request | A9.11, A9.13, A9.43 | `home/cmd_confirm_task.go`; names `home/machine.go`, `home/cmd_task.go` | T2-2a | M | after T2-2a |
| T2-13 | Host: the labels in turn text, read from the rows the home sent (`labels.md`) | A9.12, A9.42 | `host/turn_info.go` | — | S | now |
| U2-2 | The `confirm` card in its three kinds and four states, actions for `body.to` only; the allow list and delegates editor on the AI member panel (`member set` with `allow`, `approvers`) | A9.8, A9.9, A9.11, A9.14 as seen | `cmp/confirm/`, `cmp/allow/` | P-4a | M | after P-4a |

### 3.4 Lane WATCH AND TEAM

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| T2-6 | Host and node: the `watch` item and the grant, `watch.start`, the `watch` channel fed from the terminal bridge, `watchers`, `watch.end`, how watching ends (`watch-team.md` sections 1 to 6) | A9.20, A9.21 | `host/watch.go`; names `host/pty.go`, `node/surface.go` | P-4b, HS-19, HS-20 | M | after P-4b and the terminal units |
| T2-7 | Team group: the home's refusals and `bulletin`; the node's connections to the team groups it knows; the `group` argument of the `post` tool (sections 7 to 10) | A9.22, A9.23 | `home/team.go`, `node/teams.go`, `host/tools_bulletin.go` | P-4b | M | after P-4b |
| U2-3 | The receive-only terminal view, the watch request, who is watching and stopping them; team group screens and the bulletin | A9.20 to A9.23 as seen | `cmp/watch/`, `cmp/team/` | P-4b, U-11 | M | after P-4b and U-11 |

### 3.5 Lane FORGE AND GROUP INFO

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| T2-8 | Forge templates: the home's rules on `group create` and `group set`; running them on the host (`checks-forge.md` sections 2, 3) | A9.24, A9.25 | `home/templates.go`, `host/forge_tmpl.go` | P-4b | M | after P-4b |
| U2-4 | The forge choice and template editor; a group's commands shown in the dialog that joins an AI and in the group info (`checks-forge.md` section 1); whose computer keeps the group and the secretary warning (`browser-trust.md`) | A9.24, A9.39 as seen | `cmp/forge/`, `cmp/groupinfo/` (named: U-14's folder) | P-4b | M | after P-4b |

### 3.6 Lane HOME CHANGES

The contract is `contracts/step-2/export.md`, in this pull request; its one
shape is in P-4b.

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| T2-10 | Node and store: `group export` and `group import`, the file, the `moved` setting and answers, a node with an AI following `moved` (`export.md` sections 1 to 3) | A9.27, A9.28 (node half) | `node/export.go`, `internal/store/export.go`; names `node/local.go` | P-4b | M | after P-4b |
| U2-5 | Browser: follow `moved`; the group info shows the owner the two commands with the group id and the owner's other nodes | A9.27 as seen | `cmp/move/`; names `lib/groups.svelte.ts` | P-4b | S | after P-4b |

### 3.7 Lane ACCEPTANCE

| Id | Does | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- |
| I2-0 | The cast of `step-2-3-acceptance.md`: two persons, three nodes, Q-personal and Q-team fake CLIs, the test forge program first on `PATH` | `features/s2_cast_test.go` | — | S | now |
| I2-1a | Scenarios and steps of A9.1 to A9.6 | `features/s2_people*` | I2-0 to run | S | now |
| I2-1b | A9.8 to A9.19, A9.42 to A9.44 | `features/s2_confirm*` | I2-0 to run | M | now |
| I2-1c | A9.20 to A9.26 | `features/s2_watch*` | I2-0 to run | M | now |
| I2-1e | A9.27, A9.28 | `features/s2_export*` | I2-0 to run | S | now |
| I2-1d | A9.40, A9.41 | `features/s2_whole*` | the other I2-1 | S | after the lanes merge |
| I2-2 | Step 2 in the browsers and on three systems | `web/e2e/s2.spec.ts` | the U2 units | M | after the U2 units |

The scenarios of I2-1a, I2-1b, I2-1c and I2-1e are written now; each turns green when the
units it names merge.

### 3.8 What can start at once

- **Now**, once this plan is merged: P-4a (or P-5 first, 3.1), X2-1, T2-13,
  I2-0 and the scenarios of I2-1a, I2-1b, I2-1c and I2-1e.
- **After P-4a**: T2-1a, T2-1b, W2-1, U2-1, T2-2a, U2-2, and P-4b.
- **After P-4b**: T2-7, T2-8, U2-4, T2-10, U2-5; T2-6 and U2-3 also need
  the terminal units of step 1 (HS-19, HS-20, U-11).

## 4. Code units of step 3

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T3-0 | 1 | Desk research: terms and capabilities of each candidate CLI, each claim with its source (no code; the answer goes into C3-1) | — | pull request description | — | A | S |
| P-5 | 5 | Wire shapes: note types `preset` and `kb`, the brief's preset field, `kb_put`, `kb_search`; the `driver` object of `host.info` (`contracts/step-3/drivers.md` section 6) | golden round trip | lane PROTO | C3-1, C3-2; not at the same time as P-4a or P-4b | F | S |
| T3-1 | 4 | The capability table and the switches of `agent.set`, the tool list and `takeover.start` only (decided 2026-11-22), and nothing on the wire: `driver` in `host.info` comes with T3-2, after P-5 | A10 capabilities | `host/caps.go` | C3-1, HS-1, HS-2 | A | S |
| T3-2, T3-3 | 5+ | One driver and one fake CLI each for OpenCode and Qwen Code, the CLIs that passed T3-0, one at a time. T3-2 also puts the declaration into `host.info` as `driver`, in the shape P-5 froze | M3, M4, M7 tests for that CLI | `host/<cli>.go`, `host/fakecli/<cli>/` | T3-1, captures; T3-2 also P-5 | A | 2 × M |
| T3-5 | 6 | Presets: the note type, the brief field, the turn text | A10 presets | `home/preset.go`, `host/turn_preset.go` | P-5 | A | S |
| T3-6 | 6 | Knowledge base: index, `kb_put`, `kb_search`, exempt from retention | A10 knowledge | `internal/store/kb.go`, `home/kb.go`, `host/tools_kb.go` | P-5 | A | M |
| U3-1 | 6 | Preset choice in the brief card; the preset editor | A10 as seen | `cmp/preset/` | U-7, P-5 | F | S |
| U3-2 | 6 | Knowledge base panel in the group info | A10 as seen | `cmp/kb/` | U-14, P-5 | F | S |
| U3-3 | 6 | What `driver` offers: the account choice and the terminal button only (decided 2026-11-22; `contracts/step-3/drivers.md` section 6) | A10.1, A10.2 as seen | `cmp/driver/` | U-3, U-11, P-5 | F | S |
| I3-1 | 7 | Step 3 acceptance, for the CLIs that passed T3-0 only | A10.x | `features/s3*` | T3-1 to T3-3, T3-5, T3-6, U3-1 to U3-3 | A | M |

Columns as in `steps/parallel-plan-units.md`. Amp is deferred
(`contracts/step-3/drivers.md` section 8) and has no unit.

### 4.1 Lane PROFILES: AI models, profiles, assignment and escalation

Added 2026-11-22 (R-165; `contracts/step-3/profiles.md`,
`contracts/step-3/drivers.md` section 10). It extends what T3-1 gives the
product to know about an AI: T3-1 says what a CLI can do, this lane which
models it offers, which one a member runs and what its owner says the
member is good for. Its units change files that units still in flight
change too, so each waits for those pull requests ("Needs").

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| C3-3 | 4 | The contracts and this lane | — | `contracts/step-3/profiles.md`, `contracts/step-3/drivers.md` section 10 | — | — | M |
| P-6 | 5 | Wire shapes: `model`, `effort`, `tier`, `skills` on the AI member row, in `member add_ai`, `member set` and `host.join`; `member` in `host.leave`; `refresh` and `models` in `host.info`; `stuck` on the task row (`profiles.md` section 6) | golden round trip | lane PROTO: `proto/host.go`, the member and task shapes of `internal/proto`, `internal/proto/testdata/` with `wire.json`, `web/src/lib/proto.ts`, `proto.test.ts` | C3-3; #279 (P-4a), #289 (T3-2) and #296 (T3-3) merged | F | S |
| I3-2a | 5 | The scenarios of A10.8 to A10.16, written now and tagged `@pending`, so the units below build against them | — | `features/a10_profiles*.feature` | C3-3 | A | S |
| T3-7 | 6 | Home: the four fields in `add_ai` and `member set`, owner only, with defaults and bounds; `stuck` counted, cleared and answered; row 5 refused (`profiles.md` sections 1, 2, 4) | A10.8, A10.10 to A10.12, A10.16 at the home | `home/profile.go`, `features/home_profile.feature`; its calls in `home/cmd_member.go`, `home/machine.go` | P-6; #289 and #296 merged | A | M |
| T3-8 | 6 | Host, turns: the member list in every secretary turn, the line on `checks` and `reviewed` items, the instruction sentences, `submit_task` refused before the push (`profiles.md` sections 3, 4) | A10.9; A10.10 at the host | `host/turn_profile.go`, `features/host_profile.feature`; its calls in `host/turn.go`, `host/tools_task.go`, `host/instructions/` | P-6; #273 (HS-20), #280 (T3-5), #295 (T2-13) and #297 (T3-6) merged | A | S |
| T3-9 | 6 | Host, discovery: the operation "list models" for Claude Code and Codex with their fake CLIs, on the probe path and on `refresh`; the tier table; `models` in `host.info` (`drivers.md` 10.1 to 10.3) | A10.13, A10.14 at the host | `host/models.go`, `host/tiers.go`, `features/host_models.feature`; its calls in `host/claude.go`, `host/codex.go`, and in `host/agents.go`, where `host.info` is built | P-6; #275 (T3-1), #289 and #296 merged | A | M |
| T3-11 | 7 | Host, choice: `host.join` and `host.leave` for several members of one CLI; model and level passed at every start and in the terminal, for Claude Code and Codex (`profiles.md` section 2, `drivers.md` 10.4) | A10.15 at the host | `host/start_model.go`, `features/host_start_model.feature`; its calls in `host/claude.go`, `host/codex.go`, `host/join.go`, `host/takeover.go` | T3-9; #273 merged | A | S |
| T3-10 | 8 | Listing and start settings for OpenCode; start settings and the empty list for Qwen Code, in their drivers | A10.13, A10.15 for those CLIs | the files of T3-2 and T3-3 | T3-11, T3-2, T3-3 | A | S |
| U3-4 | 6 | Web, profile: tier and skills on the AI member panel and card, the owner's editor, the suggestion beside the tier, the badge. It alone edits `cmp/member/`, and leaves there the two places where U3-5 mounts its parts | A10.8, A10.10, A10.16 as seen | `cmp/profile/`, the changes in `cmp/member/` | U-3, P-6 | F | S |
| U3-5 | 7 | Web, models: model and level in the dialog for adding an AI and on the panel, the text fields for an empty list, "not listed", the refresh button, several members of one CLI on the computers page | A10.13 to A10.15 as seen | `cmp/model/` | U3-4 | F | M |
| I3-2b | 9 | The steps of I3-2a's scenarios with fake CLIs, at the nodes and in the browsers, `@pending` removed; the `live` parts once per real CLI | A10.8 to A10.16 | the step files of `features/a10_profiles*`, `web/e2e/profiles.spec.ts` | I3-2a, T3-7 to T3-11, U3-4, U3-5 | A | M |

After P-6, T3-7, T3-8, T3-9 and U3-4 start together, each once the pull
requests in its "Needs" are merged; no two of them write one file. T3-11
follows T3-9, since both write `host/claude.go` and `host/codex.go`, and
T3-10 follows T3-11. A unit keeps its logic and its scenarios in its own
files and adds only calls to the step 1 files in its "Owns"; it is their
only writer while it runs (section 1). No file of this lane begins with
`features/s3`, which is I3-1's; I3-1 tests A10.1 to A10.7 and this lane
the rest.

**What depends on a CLI exposing its list**
(`contracts/step-3/drivers.md` 10.5): only the choice from a list and the
suggested tier, so of the acceptance items A10.13 and A10.14. The
listings of Claude Code and Codex were run by the reviewer; OpenCode's
is documented; Qwen Code has none, so its list is always empty. Where a
listing does not hold, that driver declares the empty list and the owner
types the model.

### 4.2 Lane EXTERNAL: agents that join through MCP

Added 2026-11-23 (R-166; `contracts/step-3/external.md`). It is the first
tier of the owner's description of 2026-11-19, which the drivers did not
cover (`issues.md`, 2026-11-23): an agent without a driver joins through
the node's MCP server and its own runner. No driver follows T3-2 and T3-3.

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| C3-4 | 4 | The contract and this lane | — | `contracts/step-3/external.md`; the pointers in the files its section 13 names | — | — | M |
| P-7 | 5 | Wire shapes: `tool` on the AI member row, in `member add_ai` and `member set`; `host.join` for `external` with `account` and `tool`; `exe` and the `external` agent in `host.info`; the tool list `external` with `next_item`, `done`, `report_usage`; the hand-out (`external.md` section 12). Mock answers for `host.join` and `host.info` | golden round trip | lane PROTO: `proto/host.go`, `proto/tools.go`, the member shapes of `internal/proto`, a new `proto/handout.go`, `internal/proto/testdata/` with `wire.json` and `tools.json`, `lib/proto.ts`, `lib/proto.test.ts`, `lib/mock.ts` | C3-4; not at the same time as another PROTO unit | F | S |
| T3-12 | 6 | Home: `tool` in `member add_ai` and `member set`, by the AI's owner only, with its bound (`external.md` sections 1, 2) | A10.17 at the home | `home/external.go`, `features/home_external.feature`; its calls in `home/cmd_member.go`, `home/profile.go` | P-7 | A | S |
| T3-13 | 6 | Host, runs: `host.join` and `host.leave` for the agent `external`, with `account` and `tool` and no `agent.set`; its entry in `agents`; the hand-out, built on delivery's batch, folder and turn text, with `context`; a run and its four ends; presence and the reported state; the member list line (`external.md` sections 2 to 4, 6, 9, 10) | A10.17, A10.22 to A10.25, A10.27 at the host | `host/external.go`, `host/instructions/external.md`, `features/host_external.feature`; its calls in `host/deliver.go`, `host/join.go`, `host/agents.go`, `host/turn_profile.go`; `host/README.md` | P-7; #347 and #348 merged (`host/README.md`, `host/deliver.go`); T3-15 (`host/turn_profile.go`) | F | M |
| T3-14 | 7 | Host and node, the two surfaces: a member id where a session token stands, and `mcp --member`; the list `external`, each call judged by the open run; `next_item`, `done`, `report_usage`; `approve` for an external connection; `solodawn wait` and `solodawn done` with their socket commands and exit codes; `exe` in `host.info` (`external.md` sections 3, 5, 7) | A10.18 to A10.21, A10.25, A10.26 at the node | `host/tools_external.go`, `node/external.go`, `features/host_tools_external.feature`, `features/cmd_external.feature`; its calls in `host/shim.go`, `host/mcp.go`, `host/approve.go`, `node/socket.go` and where `host.info` is built | T3-13 | A | M |
| T3-15 | 6 | Host, the member list of the secretary's turn: every AI whose `usage` is known gets `window <pct>% used, resets in <n> h` (n rounded up; the "resets" part left out without `window_resets`, nothing without `window_pct`), a managed AI from its CLI's events and an external one from `report_usage`; assignment rule 3 gains "prefer AIs with more of their window left". No caps (owner, 2026-11-23) | A10.9, A10.25 at the host | `host/turn_profile.go`, `features/host_profile.feature`; the member list text of A10.9 in `steps/step-3-acceptance.md` and of `contracts/step-3/profiles.md` section 3 | I3-2b (#343) merged: it changes the steps of A10.9 | A | S |
| U3-6 | 6 | Web: `external` in the dialog that adds an AI, with account, tool label and text fields; the setup panel with the prompt, the server entry, the setup check and its four marks; the tool label on the member card and panel (`external.md` sections 8, 10). The prompt text lives in its one file | A10.17, A10.20, A10.25, A10.27 as seen | `cmp/external/` with its `mounts.ts` and `prompt.md`; a mount point in `cmp/member/` only where none fits | P-7 | F | M |
| I3-3 | 8 | The scenarios and steps of A10.17 to A10.28, at the nodes and in the browsers. Its test agent is a stdio MCP client behind `solodawn mcp --member` and a runner around `solodawn wait` and `solodawn done`. The `live` test once, with a real agent given the prompt | A10.17 to A10.28 | `features/a10_external*`, `web/e2e/external.spec.ts` | T3-12 to T3-14, U3-6 | A | M |

After P-7, T3-12, T3-13 and U3-6 start together; no two of them write one
file. T3-15 needs no shape of P-7 and starts when #343 is merged; it and
T3-13 both write `host/turn_profile.go`, so T3-13 waits for it. T3-14 follows T3-13 and is given to the same builder, since it calls
what T3-13 makes. As in lane PROFILES, a unit keeps its logic and its
scenarios in its own files and adds only calls to the files named in its
"Owns".

Checked against the open pull requests on 2026-11-23: #348 changes
`host/deliver.go` and `host/README.md`, and #347 `host/README.md`, so
T3-13 waits for both; #346
changes `host/turn.go` and #343 `features/a10_profiles*`,
`web/e2e/profiles.spec.ts` and `testdata/trace-*.json`, which no unit of
this lane writes. #349 adds `planning/spikes/zcode.md`, a check of ZCode as
an external AI: what it finds goes into I3-3's `live` test.

### 4.3 Lane DIAGNOSTICS: logs and tests that explain a failure

Added 2026-11-23 (the owner's rule of that day, `issues.md`;
`contracts/diagnostics.md`). Several tests called flaky were product races,
and each took hours of stress runs to find. The lane makes one occurrence
enough: the node logs its decisions, and a failing test says what it saw
and leaves the logs behind. It adds no command and no package.

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| DG-1 | Harness: `log` into a buffer emptied per scenario, at `debug`; the folder of a failed scenario; the one wait helper of `features/` with the message form; the upload in CI (`diagnostics.md` section 4, points 1, 2 and 4) | a scenario made to fail leaves the folder; unit of the helper | `features/diag_test.go`; one hook in `features/acceptance_test.go`; one line where a child node starts (`features/node_test.go`, `m6_test.go`, `m8_test.go`, `s2_cast_test.go`) and the `SIGQUIT` in `features/proc_unix_test.go`; `.github/workflows/backend.yml` | — | M | now |
| DG-2 | `features/`: every wait that can time out goes through the helper of DG-1. It changes waits only, in every step file | the scenarios still pass | the waits of `features/*_test.go`; `helpers_test.go`, `m3_test.go`, `s3_driver_limits_test.go` and `s3_drivers_test.go` last, after #360; `host_run_test.go` after #363 | DG-1 | M | after DG-1 |
| DG-3 | `internal/`: the same for the waits in the test files of `home`, `node`, `peer` and `host`, with each package's own helpers; the root test that holds the form in `features/` and `internal/` (section 4, point 1) | the root test | `waits_test.go`; the waits of `internal/**/*_test.go`; `home/cmd_hostev_test.go`, `node/node_test.go` and `peer/ctl_test.go` after #360; `host/pty_test.go` and `host/run_test.go` after #363 | DG-2 | M | after DG-2 |
| DG-4 | Node: the flags of `log`, the level file, the logger with `node`; `node.start`, `node.stop`, `home.dial`, `bg.*` for the runtime's own work; the section "After a failure" at the end of `install.md` (sections 1, 3, 5) | unit: each line once, with its fields; the level file | `node/diag.go`; its calls in `node/node.go`, `node/teams.go`; `install.md`; `node/README.md` | #360 merged or closed | S | after #360 |
| DG-5 | Home: `home.attach`, `home.detach`, `cmd.reply`, `rows.send`, `inbox.send`, `ack.recv`, `bg.*` (section 2) | unit: each line once, with its fields; `rows.send` before the `cmd.reply` of one command | `home/diag.go`; its calls in `home/run.go`, `run_conn.go`, `run_group.go`, `run_push.go`, `inbox.go`; `home/README.md` | — | S | now |
| DG-6 | Host: `host.attach` to `settle.end` of section 2, and `bg.*` through one wrapper for the work its wait groups start | unit: each line once, with its fields; `inbox.take` with the pause reading and what it left | `host/diag.go`; its calls in `host/host.go`, `deliver.go`, `run.go`, `external.go`, `mcp.go`, `tools_task.go`, `limits.go`, `takeover.go`, `home_moved.go`; `host/README.md` | #363 merged (it holds the FLAKY-28 and FLAKY-30 fixes) | M | after those |
| DG-7 | Peer: `sig.send`, `sig.recv`, `ice.state`, `ice.pair`, `ctl.open`, `conn.close` (section 2) | unit with two peers in one process: each line once, with its fields | `peer/diag.go`; its calls in `peer/signal.go`, `peer/ctl.go`, `peer/sockets.go` | #360 merged or closed | S | after #360 |

- **Order.** DG-1, DG-2 and DG-3 are one chain for one builder, bottom up;
  it starts now and needs no product unit. DG-5 starts now beside it. DG-4
  and DG-7 start when #360 is merged or closed, DG-6 when `internal/host`
  is free. The four product units write no common file and merge in any
  order: each package makes its own logger from the key or id it already
  has.
- **Checked against the open pull requests on 2026-11-23.** #360 changes
  `features/helpers_test.go`, `m3_test.go`, `s3_driver_limits_test.go`,
  `s3_drivers_test.go`, `home/cmd_hostev_test.go`, `node/node.go`,
  `node/node_test.go`, `peer/ctl.go`, `peer/ctl_test.go` and
  `peer/path.go`; #363 changes `features/host_run_test.go`,
  `host/pty_test.go` and `host/run_test.go`; #365 changes
  `features/a10_external.feature`, which no unit here writes; #361 is a lab
  branch that is not merged. DG-2 and DG-3 check the list again when they
  start, and take a file another open pull request holds last.
- A test that reads what `log` wrote (`host/deliver_test.go`,
  `node/update_test.go`, `node/schedule_test.go`, `home/run_test.go`) now
  sees the event lines too; the unit of that package adjusts it.
- Removing the hooks of #360 once the lines exist is in the follow-up list
  of section 5, not a unit here.

## 5. Open questions for the coordinator

1. **R-44 against "highest level, no control" in 1.0.** (a) Drop the cap
   for 1.0: a colleague's task runs on my machine at level 3, gated only by
   the `confirm` card or my allow list; saves T2-3 (S), one item field and
   A9.16, A9.17. (b) Keep it as an automatic cap with no control: T2-3 and
   one item field; a colleague's task stops for my approval on anything
   outside its worktree (network, installs), and Claude Code on native
   Windows falls to level 1, read only. **Recommended: (a).** A level
   guards against AI mistakes, which do not depend on who asked, and
   members trust each other; R-44 stays written for after 1.0.
2. **Moving and copying a group (R-134, R-135; SLIMMING D-3).** (a) Merge
   #166 without its signatures (A-15) and build move and copy: 2 × M and
   six tests. (b) One export on the old node and one import on the new; a
   copy is a periodic export: a short contract, one M unit, one S web unit,
   two tests; no live hand-off, and a failed move is redone by hand.
   **Recommended: (b).** Note: the decision log says #166 merged, but it is
   still open and `move.md` is not on `rebuild`.
3. **The `v` 1 fallback of `hello` (SLIMMING D-4).** Cutting it saves about
   60 lines in T2-1b and W2-1; all nodes then run one version (they update
   themselves). `contracts/step-3/knowledge.md` section 8 builds on it, and
   Gemini's step 3 units read that file. **Recommended:** keep it now; cut
   it in a small pull request after those units merge.
4. **Step 3 assumes level control.** `contracts/step-3/drivers.md` section 5
   (`perm.set`, the caps of `settings.set`, the R-44 cap), U3-3 (level
   control, cap units) and A10.1 (`perm.set` on Windows) act on commands
   that are postponed after 1.0. Not changed here, because Gemini builds
   against that file. **Recommended:** T3-1 builds the switches of
   `agent.set`, the tool list and `takeover.start` only; U3-3 shows the
   account choice and the terminal button only; the rest waits for
   `perm.set`.

5. **Lane PROFILES (R-165): the choices made in writing
   `contracts/step-3/profiles.md` and `contracts/step-3/drivers.md`
   section 10.** (a) "Checks not passed" counts hosted checks only; a
   failed `run_checks` reaches no row of the task machine. (b) The
   refusal of `submit` reuses `conflict`, with no new error code. (c)
   Whether a stronger AI exists is judged by tier alone, whatever its
   state: if the only stronger AI is limited or offline the task waits,
   and the quiet-task notice brings the secretary. (d) After the answer,
   a third failure of the same kind asks again. (e) Nothing was added to
   `list_members` or to the driver declaration. (g) A changed model
   applies at the next start of a CLI process, a resume included. (h)
   Fast mode is listed and cannot be chosen. (i) Discovery runs with
   detection, so the Claude Code listing starts a process (no model
   call) at most once in 60 s per `host.info` request. (j) Qwen Code's
   list read from its settings file. (k) Two members of one CLI share
   one subscription; the secretary is told so, nothing stops it. (l) The
   secretary is no candidate for an escalation.

6. **Lane EXTERNAL (R-166): the choices made in writing
   `contracts/step-3/external.md`.** (a) The member id is the token, and
   no secret: the local socket admits only the owner's user, so a secret
   would protect nothing and would sit in configuration files. (b) The
   node names the folder on its own computer; the remote is not the
   exchange point. (c) An item is acknowledged when it is handed out and
   never handed out again; a run that died is a crash or a timeout, as
   for a managed AI. (d) A run is ended by its number, so a late `done`
   ends nothing else; the runner ends it, and the tool `done` is for a
   session a person started. (e) `next_item` stays as a tool and never
   waits. (f) The setup check is a message the client posts as the owner,
   which the home turns into a `mention`: no new item of the home. If the
   home must send it itself, that is one more small home unit and one
   more shape in P-7. (g) Presence lasts 180 seconds after the last
   command or connection. (h) One tool list for an external connection,
   each call judged by the open run. (i) An external AI is never the
   secretary; the client does not offer it and the home does not check.
   (j) The member list of the secretary's turn carries no usage for any
   AI; the secretary reads it with `list_members`. Adding it to that line
   is a change for every AI and is not made here.

### Decided (2026-11-22)

1. R-44: dropped for 1.0, answer (a). T2-3 is cancelled, with A9.16 and
   A9.17; `contracts/step-2/authority.md` section 5 and R-44 say so.
2. Moving and copying: export and import, answer (b).
   `contracts/step-2/export.md` is the short contract; T2-10 (M) and U2-5
   (S) are its units; A9.27 and A9.28 its two tests. Pull request #166 is
   left to the coordinator, who closes it.
3. The `hello` `v` 1 fallback stays for now.
4. Step 3 and level control: as recommended. T3-1 builds the switches of
   `agent.set`, the tool list and `takeover.start` only; U3-3 shows the
   account choice and the terminal button only; the postponed parts of
   `contracts/step-3/drivers.md` section 5 and of A10.1 are dropped.
5. Lane PROFILES: (a) to (e), (h), (i) and (k) as written; for (a),
   a task without a remote has no hosted check, so only its failed
   reviews count; for (h),
   Codex's fast mode is read from `serviceTiers`; for (k), each member
   is `limited` only when its own CLI reports it. (g) as written, with
   no fallback to a new session. (j) rejected: the settings file can hold
   a credential, so Qwen Code lists nothing and the owner types the
   model. (l) rejected: the secretary is a candidate; R-21 forbids it
   writing code, not answering. No host-kept list of levels: without a
   list from the CLI the level is a text field. The A10 items of the
   lane are in `steps/step-3-acceptance.md`, with the rest of A10.

### Decided (2026-11-23)

6. Lane EXTERNAL: (a) to (i) as written. For (f), the setup check stays
   the client's message. (j) changed: the owner wants each AI's quota on
   the member list line, for scheduling, so the line changes for every
   AI. Unit T3-15 of section 4.2 does it and owns the text.

7. Lane DIAGNOSTICS: (a) the lines go through the default logger of
   `log/slog`; no package or handler of our own. (b) Every event is on by
   default, so a machine that was not prepared still explains its failure;
   the cost is about ten lines per turn. (c) Only ICE candidates are
   `debug`; `bg.start` and `bg.stop` are `info`, because the owner wants a
   failure on his machines explained with no preparation and accepts the
   extra lines. (d) The switch is the file `logs/level`, read when the node
   starts; the price is a restart. (e) No `solodawn logs` command: one
   shell line per system sends the folder. An installed node has no
   goroutine dump, for now. (f) The web client and the rendezvous service
   log nothing new. (g) A root test holds the message form of the waits.
   The Windows `tar` line is checked in I-11.

### Follow-up

- Cut the `hello` `v` 1 fallback (SLIMMING D-4: about 60 lines in T2-1b and
  W2-1, and `contracts/step-3/knowledge.md` section 8) in a small pull
  request after Gemini's step 3 units merge.
- Remove the one-off hooks that #360 added to look into one failure
  (`Connection.NegotiationState`, the signal receiver of `node/node.go`)
  once the DIAGNOSTICS events are merged.

## 6. Not verified

- The lanes were read against the contracts and the file tree of `rebuild`;
  no unit was sized by trying it.
- The capabilities of OpenCode and Qwen Code: T3-0 read them in the
  vendors' documentation and ran nothing (`contracts/step-3/drivers.md`
  section 8).
- Lane EXTERNAL: no agent was given the setup prompt, and no unit was
  sized by trying it (`contracts/step-3/external.md` section 14).
- Lane DIAGNOSTICS: the events were chosen by reading `issues.md`, none
  was tried against a failing run, and no unit was sized by trying it
  (`contracts/diagnostics.md` section 6).

## Cuts of 2026-11-22

Why, for every line: the owner's rules of 2026-11-21 (least code; no
defence in code against attackers, a dishonest home, host or member, forged
or replayed messages, or a stolen device; members of a group trust each
other) and 2026-11-22. Items are those of the slimming list SLIMMING.md;
the external measure for each is in the deployment guide. Section numbers
are those of the files before the cut.

| # | What was cut | Why | SLIMMING.md |
| --- | --- | --- | --- |
| 1 | `authority.md` sections 1 and 3: a host's proof that its owner signed for each item (forwarded envelopes, proofs 1 to 5, the table per `why`) | a dishonest home; the home decides | A-8 |
| 2 | `authority.md` section 2: the host command `allow.set`, the host table `allow`, the lists in `host.info`; the lists stay on the member row of step 1 | a dishonest home | A-8 |
| 3 | `authority.md` section 6: `req`, `body.start`, a brief's `arg`, unique titles in a brief | a home that swaps a request | A-8, A-11 |
| 4 | The secretary by consent: `secretary_by`, `secretary_joined`, the host's check; `group set` names the secretary | a dishonest admin or home | A-8 |
| 5 | `authority-record.md`, deleted | its rules are cut | A-8 |
| 6 | Unit T2-3 as planned (owner-signature rule, signed list, merge reading the pull request's base and head) | a dishonest home | A-8, A-16 |
| 7 | `authority.md` section 7, the fork rule of the device log; `fork-vectors.md` deleted; its decision record moved to `docs/decisions/rejected/`; `risks.md` K-4 back to the step 1 rule | a stolen device | A-9 |
| 8 | Unit T2-4 (the fork rule and its vectors); the device-list notices of U2-4 | a stolen device | A-9, overview |
| 9 | `browser-trust.md` sections 1 to 6, 8.1, the fingerprints of 8.2, 9 to 12: the browser's check of every envelope and the "unverified" mark | a dishonest home | A-10 |
| 10 | Unit T2-5 (`lib/verify.ts`); the mark in U2-4 | a dishonest home | A-10, overview |
| 11 | `confirm.md` section 6: the digest of a request, `arg` on `act confirm`, the `act` as `cause` of an item, the host refusing an item without its card | a home that moves a confirmation | A-11 |
| 12 | `people.md`: `fp` in the link and its confirmation, the role `join`, acceptance by purpose, the new pairing vectors, the admission table; R-140 rewritten | a rendezvous that stands in for a home | A-12 |
| 13 | `contracts/step-3/knowledge.md` section 8 and P-4: a `v` 2 home takes `v` 1 only from its owner's devices; a host speaks `v` 1 only to its owner's home | a dishonest home on an old node | A-13 |
| 14 | `watch-team.md` section 2: checks 1 to 4 of the grant, the `cause` of the item; settled point 2 | a dishonest home | A-14 |
| 15 | `checks-forge.md`: the host checking a template again before it runs | a dishonest home | A-16 |
| 16 | `labels.md`: labels from verified signatures; they are read from the rows | a dishonest home | A-17 |
| 17 | `checks-forge.md` sections 1 to 4: acceptance per machine, the hash, `checks.accept`, the host's gate, the error `accept`, `task accept` with rows CF-1 and CF-2, the wait `accept`, the vector part `accept`; the client shows the commands instead | the owner's decision on C-4 | C-4 |
| 18 | Unit T2-9 | as above | C-4, overview |
| 19 | `checks-forge.md` section 7 and D-12: checks in bubblewrap or `sandbox-exec`; `risks.md` K-8, Q-19 and T-16 updated | the owner's decision on C-5; K-8 stays a stated limit | C-5 |
| 20 | Unit T2-12 | as above | C-5, overview |
| 21 | Audit rows written in step 2: `setting` for the lists, `watch` with `granted`, `opened`, `late`, `ended`; `refused` of items a host no longer refuses | audit for after the fact | A-4 |
| 22 | `checks-forge.md`: after a restart, `git merge-tree` to learn whether an unreported custom-forge merge landed; the task reports a conflict and the owner looks | least code; rare, fixed by hand | D-5 |
| 23 | Acceptance A9.7, A9.10, A9.15, A9.37, A9.38, A9.45 | they test cut defences | A-8 to A-10, A-16 |
| 24 | Acceptance A9.33 to A9.36 | they test the sandbox and the acceptance gate | C-4, C-5 |
| 25 | The test hook on a home, and the parts of A9.1, A9.2, A9.6, A9.8, A9.19, A9.29, A9.32, A9.40, A9.43, A9.44 that tested fingerprints, signatures, audit, injected envelopes, a changed byte or `checks.accept` | as the items named | A-4, A-8, A-12, A-15, C-4 |
| 26 | `steps/step-2-3.md`: known points 2 to 4, the sandbox row, the signed-item paragraph; R-136, R-137 and C-11 (`architecture.md`) reworded | follow from the cuts above | A-8, C-4, C-5 |
| 27 | P-4 as planned: `allow.set`, `req`, `body.start`, `secretary_by`, `secretary_joined`, `arg`, `checks.accept`, the `pair` role `join`, the snapshot; it is now P-4a and P-4b | their rules are cut | A-8, A-11, A-12, C-4 |
| 28 | Unit T3-4 (Amp) removed from the unit table; it was never scheduled | no approval channel (`contracts/step-3/drivers.md` section 8) | — |
| 29 | The level cap of R-44: `authority.md` section 5's rule, the `capped` item field, unit T2-3 (cancelled), A9.16 and A9.17 | R-44 is not in 1.0 (decision 1 above) | — |
| 30 | The live move and the scheduled copy of D-11 (PR #166: snapshot, redirect, epoch, stop-and-transfer), unit T2-11, A9.29 to A9.32 | export and import instead (decision 2 above); least code | D-3, A-15 |
| 31 | `contracts/step-3/drivers.md` section 5: the switches of `perm.set`, `settings.set` caps and the task session's level; U3-3's level control and cap units; the `perm.set` sentence of A10.1 | no level control or caps in 1.0 (decision 4 above) | — |

Changed, not cut: `settings.teams` became "the team groups a node knows"
(homed there, or added with `home.add`), because `settings.set` is postponed
after 1.0 (`contracts/step-2/watch-team.md` section 10).
