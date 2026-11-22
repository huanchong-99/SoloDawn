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
| D-1 | Who caused a task, for the level cap of R-44 | Open: question 1 below. If R-44 is kept: the person who confirmed the brief, or who made the direct request, caused the task; the home marks the item, the host caps the session (`contracts/step-2/authority.md` section 5) |
| D-2 | What a host requires before it acts for a home that is not its owner's | Cut: the host takes its home's items as in step 1 |
| D-3 | Check commands on other people's machines | Cut: no acceptance and no gate. The client shows a group's commands before an AI joins it (`contracts/step-2/checks-forge.md` section 1) |
| D-4 | Stolen-device race in the device log | Cut: the step 1 rule stays, `risks.md` K-4 a stated limit |
| D-5 | What a browser trusts | Cut: a browser shows what the home pushes, as in step 1 |
| D-6 | A secretary on a personal subscription | No mechanism. The interface warns; the documentation recommends a team plan or key (`contracts/step-2/browser-trust.md` section 1) |
| D-7 | Invitation | The link carries group id, home device id and a one-time secret; joining reuses the pairing room and `mac` unchanged, and the home treats the peer as joining. Roles: `owner` (one, the home's owner), `admin`, `member` (`contracts/step-2/people.md`) |
| D-8 | The `confirm` card | One card kind for three requests: directing someone's personal-subscription AI, calling it with `ask`, and watching its terminal. `act confirm` names the card; the home queues the item (`contracts/step-2/confirm.md`) |
| D-9 | Messages from others after a task started (R-52) | Delivered with the label `information`, read from the rows the home sent (`contracts/step-2/labels.md`) |
| D-10 | Other forges | Per group: four templates (push, open request, read checks, merge) as argument lists with named places, never a shell string; the home checks them (`contracts/step-2/checks-forge.md`) |
| D-11 | Moving a group; a copy for safekeeping | Open: question 2 below |
| D-12 | Checks in a sandbox | Cut: none in 1.0; `risks.md` K-8 stays a stated limit |
| D-13 | Approver delegates (R-42) | The `approvers` of the member row, set by the owner with `member set` of step 1; the home checks the row when a decision comes (`contracts/step-2/authority.md` section 3) |
| D-14 | Watching a terminal (R-81) | A receive-only channel `watch:*` served by the AI's host on a `direct` path, opened after a `confirm` card with `what: watch`; the home's `watch` item gives the grant (`contracts/step-2/watch-team.md`) |
| D-15 | Team-wide group | Group kind `team`: tasks, briefs and AIs refused; a secretary posts a `bulletin` there to a team group its node knows (`contracts/step-2/watch-team.md`) |
| D-16 | Which CLIs first (step 3.1) | OpenCode and Qwen Code, API keys only; Amp deferred (`contracts/step-3/drivers.md` section 8) |
| D-17 | Style presets (step 3.2) | A note of type `preset` per group; a brief names one; its text is put into the `assign` turn |
| D-18 | Knowledge base (step 3.3) | Notes of type `kb` with a full-text index on the home; tools `kb_put` and `kb_search`; exempt from retention |
| D-19 | TCP simultaneous open (step 3.4) | Not in version 1 |

## 2. Contracts

The contract units C2-0 to C2-5, C2-7, C3-1 and C3-2 are merged and cut
to the rules above by this file's pull request. C2-6 (moving and copying,
`move.md` and `copy.md`) is still open as pull request #166 and waits for
question 2. Spikes S-10 and T3-0 have reported; S-10 is not used, since
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
| P-4b | Freeze watch, team and forge: item `watch`, `watch.start` and its answer, the label `watch:<g>:<member>`, `watchers` in `host.info`, `watch.end` (`watch-team.md` sections 2 to 5); group kind `team`, kind `bulletin` and its body (sections 7 to 10); `forge: custom` and `templates` in the group row, `group create` and `group set` (`checks-forge.md` section 2) | lane PROTO files | P-4a | S | after P-4a |

### 3.2 Lane PEOPLE: invitations, joining, roles, removal

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| X2-1 | Types-only seam for joining: the field of a command's context that says a connection is joining, for which group, with which presented log; the hook the home calls for `invite` | build passes | `home/people_seam.go` | — | S | now |
| T2-1a | Home: `join`, `member set` with `role`, `member remove` of a person with its `device` and `push_sub` rows (`people.md` sections 1, 2, 6) | A9.3, A9.4 (home half) | `home/cmd_people.go` | P-4a, X2-1 | M | after P-4a |
| T2-1b | Node: `invite` (the secret, the room, the link, in memory), joining connections (`people.md` sections 3 to 5), device logs of several people, `home.add`, `hello` `v` 2 with its fallback | A9.1, A9.2, A9.6 (node half) | `node/admit.go`, `node/version.go`; names `node/pair.go`, `node/devlog.go` | P-4a, X2-1 | M | after P-4a |
| W2-1 | Browser: open a `#join=` link, join the room as in a pairing, present the log, send `join`, then `home.add` to the person's own nodes; `hello` `v` 2 with its fallback | unit | `lib/join.ts`, `lib/version.ts` | P-4a | M | after P-4a |
| U2-1 | Members and roles, the invitation dialog with its countdown, the join page, removal | A9.1 to A9.4 as seen | `cmp/people/`, `routes/Join.svelte` | P-4a; the join page also W2-1 | M | after P-4a |

### 3.3 Lane INSTRUCTIONS: the `confirm` card, allow lists, delegates, labels

| Id | Does | Tests | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- | --- |
| T2-2a | Home: when to ask (`confirm.md` section 2) for mentions and calls; the card, `act confirm` and `act deny`, expiry, reminders; the call's denial; approvals by the current `approvers` of the row (`authority.md` section 3) | A9.8, A9.9, A9.14, A9.18, A9.19 (home half) | `home/cmd_confirm.go`; names `home/cmd_post.go`, `home/cmd_card.go` | P-4a | M | after P-4a |
| T2-2b | Home: a role needs its owner's word (`confirm.md` section 6): the card at task-machine rows 1 and 11, the wait `confirm`, items after a confirmation, a kept request | A9.11, A9.13, A9.43 | `home/cmd_confirm_task.go`; names `home/machine.go`, `home/cmd_task.go` | T2-2a | M | after T2-2a |
| T2-13 | Host: the labels in turn text, read from the rows the home sent (`labels.md`) | A9.12, A9.42 | `host/turn_info.go` | — | S | now |
| T2-3 | Level cap, only if question 1 keeps R-44: the home marks the item, the host caps the session (`authority.md` section 5); its one item field goes into whichever of P-4a, P-4b is not merged yet | A9.16, A9.17 | `home/cap.go`, `host/cap.go` | question 1, P-4a | S | after question 1 |
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

Waits for question 2. With the recommended answer: a short contract
(C2-6b, S: the export file, the import command, how clients learn the new
home), its shapes in lane PROTO (S), one backend unit T2-10 (M: export and
import on the node and home), one web unit U2-5 (S: export, import, the
last export's time), and A9.27 to A9.32 rewritten as two tests. With the
other answer: pull request #166 cut by A-15 and merged, then T2-10 (move)
and T2-11 (copy) as first planned, M each.

### 3.7 Lane ACCEPTANCE

| Id | Does | Owns | Needs | Sz | Start |
| --- | --- | --- | --- | --- | --- |
| I2-0 | The cast of `step-2-3-acceptance.md`: two persons, three nodes, Q-personal and Q-team fake CLIs, the test forge program first on `PATH` | `features/s2_cast_test.go` | — | S | now |
| I2-1a | Scenarios and steps of A9.1 to A9.6 | `features/s2_people*` | I2-0 to run | S | now |
| I2-1b | A9.8 to A9.19, A9.42 to A9.44 | `features/s2_confirm*` | I2-0 to run | M | now |
| I2-1c | A9.20 to A9.26 | `features/s2_watch*` | I2-0 to run | M | now |
| I2-1d | A9.40, A9.41 | `features/s2_whole*` | the other I2-1 | S | after the lanes merge |
| I2-2 | Step 2 in the browsers and on three systems | `web/e2e/s2.spec.ts` | the U2 units | M | after the U2 units |

The scenarios of I2-1a to I2-1c are written now; each turns green when the
units it names merge.

### 3.8 What can start at once

- **Now**, once this plan is merged: P-4a (or P-5 first, 3.1), X2-1, T2-13,
  I2-0 and the scenarios of I2-1a to I2-1c.
- **After P-4a**: T2-1a, T2-1b, W2-1, U2-1, T2-2a, U2-2, and P-4b.
- **After P-4b**: T2-7, T2-8, U2-4; T2-6 and U2-3 also need the terminal
  units of step 1 (HS-19, HS-20, U-11).
- **After the coordinator's answers**: T2-3 (question 1), lane HOME CHANGES
  (question 2).

## 4. Code units of step 3

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T3-0 | 1 | Desk research: terms and capabilities of each candidate CLI, each claim with its source (no code; the answer goes into C3-1) | — | pull request description | — | A | S |
| P-5 | 5 | Wire shapes: note types `preset` and `kb`, the brief's preset field, `kb_put`, `kb_search`; the `driver` object of `host.info` (`contracts/step-3/drivers.md` section 6) | golden round trip | lane PROTO | C3-1, C3-2; not at the same time as P-4a or P-4b | F | S |
| T3-1 | 4 | The capability table and the switches it drives, and nothing on the wire: `driver` in `host.info` comes with T3-2, after P-5 | A10 capabilities | `host/caps.go` | C3-1, HS-1, HS-2 | A | S |
| T3-2, T3-3 | 5+ | One driver and one fake CLI each for OpenCode and Qwen Code, the CLIs that passed T3-0, one at a time. T3-2 also puts the declaration into `host.info` as `driver`, in the shape P-5 froze | M3, M4, M7 tests for that CLI | `host/<cli>.go`, `host/fakecli/<cli>/` | T3-1, captures; T3-2 also P-5 | A | 2 × M |
| T3-5 | 6 | Presets: the note type, the brief field, the turn text | A10 presets | `home/preset.go`, `host/turn_preset.go` | P-5 | A | S |
| T3-6 | 6 | Knowledge base: index, `kb_put`, `kb_search`, exempt from retention | A10 knowledge | `internal/store/kb.go`, `home/kb.go`, `host/tools_kb.go` | P-5 | A | M |
| U3-1 | 6 | Preset choice in the brief card; the preset editor | A10 as seen | `cmp/preset/` | U-7, P-5 | F | S |
| U3-2 | 6 | Knowledge base panel in the group info | A10 as seen | `cmp/kb/` | U-14, P-5 | F | S |
| U3-3 | 6 | What `driver` offers: the account choice, the level control, the cap units, the terminal button (`contracts/step-3/drivers.md` section 6) | A10.1, A10.2 as seen | `cmp/driver/` | U-3, U-5, U-11, U-12, P-5 | F | S |
| I3-1 | 7 | Step 3 acceptance, for the CLIs that passed T3-0 only | A10.x | `features/s3*` | T3-1 to T3-3, T3-5, T3-6, U3-1 to U3-3 | A | M |

Columns as in `steps/parallel-plan-units.md`. Amp is deferred
(`contracts/step-3/drivers.md` section 8) and has no unit.

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

## 6. Not verified

- The lanes were read against the contracts and the file tree of `rebuild`;
  no unit was sized by trying it.
- The capabilities of OpenCode and Qwen Code: T3-0 read them in the
  vendors' documentation and ran nothing (`contracts/step-3/drivers.md`
  section 8).

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
| 6 | Unit T2-3 as planned (owner-signature rule, signed list, merge reading the pull request's base and head); a cap-only T2-3 waits for question 1 | a dishonest home | A-8, A-16 |
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

Changed, not cut: `settings.teams` became "the team groups a node knows"
(homed there, or added with `home.add`), because `settings.set` is postponed
after 1.0 (`contracts/step-2/watch-team.md` section 10).
