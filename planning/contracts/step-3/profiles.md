# Contract, step 3: AI profiles, assignment and escalation

Which model an AI member runs, what its owner says about it, how the
secretary is given that, and when a stuck assignee must ask a stronger
AI. Rule: R-165. Decided by
the coordinator on 2026-11-22; lane PROFILES of
`steps/parallel-plan-step-2-3.md` section 4.1 builds it.

This file adds to the step 1 contracts and changes one sentence of them:
a model is now passed at each start (section 7).

A **working AI** is an AI member whose row has `secretary` `false`, as in
step 1. The product decides nothing here that needs judgement (R-2):
it lists what a CLI offers, suggests, stores four fields, prints two of
them, counts failures and refuses one command.

Not in it: cost or usage caps (postponed with `settings.set`), switching
models while a CLI process runs, permission levels, audit rows, and any check
against a peer that lies.

## 1. The member (adds to `contracts/rows.md` section 2)

An AI member is a CLI on a computer under an account, with a chosen model
and thinking level, and its profile. Four fields on the member row of an
AI, always present. A person's row has none of them.

| Field | Values | When never set |
| --- | --- | --- |
| `model` | the model's id as the CLI names it, at most 100 code points, no control characters; `""`: the CLI's own default | `""` |
| `effort` | the thinking level as the CLI names it, not the permission level of `sessions.md`, which it does not change; at most 32 code points, no control characters; `""`: the CLI's own default | `""` |
| `tier` | `strong`, `standard`, `light` | `standard` |
| `skills` | what the AI is good at: one line of at most 200 Unicode code points, without `\r` or `\n` | `""` |

```json
{"r":"member", …, "kind":"ai", "cli":"claude", "model":"opus",
 "effort":"high", "tier":"strong",
 "skills":"Go backend, protocol design, hard debugging", …}
```

- `tier` and `skills` are the **profile**. The order is `light` <
  `standard` < `strong`; an AI is **stronger** than another when its tier
  is higher. Nothing in the product reads a tier but section 4, and
  nothing reads `skills` but the turn text of section 3.
- The home stores `model` and `effort` and checks only the bounds of the
  table: it knows no CLI and no model. The host passes them to the CLI at
  each start and checks nothing either: a value the CLI refuses is handled
  as a crash, not as a lost session (`contracts/step-3/drivers.md` 10.4).
- **Several members from one CLI.** A host may add the same CLI to a
  group more than once, each with its own name, model and level. They
  share the CLI's account. Each is `limited` only when its own CLI
  reports it.
- A member with all four defaults behaves as in step 1, with one
  difference: the secretary's turns carry the member list of section 3
  whatever the values. A group whose AIs
  all keep the default tier has no stronger AI, so section 4 never asks
  for anything.
- The web client shows the four on the AI member panel and card, and lets
  the AI's owner edit them there (section 2a).

## 2. Setting it (adds to `contracts/commands.md` 1.2 and section 2)

| Command | Addition |
| --- | --- |
| `host.join` | optional `model`, `effort`, `tier`, `skills`, passed on in `member add_ai`. Joining with an `agent` that already has a member in `g` adds another member |
| `member add_ai` | the same four, optional; what is absent gets the default of section 1 |
| `member set` | the same four in its "any of" |
| `host.leave` | optional `member`, a member id: required when the `agent` has more than one member in `g`, else `bad_request` |

- `member set` with any of the four is by the AI's owner only. The AI's
  host may not send them.
- A `tier` that is not one of the three values, and a text that breaks a
  bound of section 1, are `bad_request`.
- A new `tier` or `skills` takes effect for what happens next: the next
  turn of the secretary, the next failure of a task. A task that is
  already waiting for an answer (section 4) keeps waiting.
- A new `model` or `effort` takes effect when the host next starts a CLI
  process for a session of that member, a resume included. The host does
  not stop a running process for it.

## 2a. What the owner sees

The node lists and suggests; the owner chooses; rows change only by the
owner's `member set`.

- **Adding.** The dialog of `host.join` offers the models of that CLI
  from `host.info` (`contracts/step-3/drivers.md` section 10), for the
  chosen model its levels, and "the CLI's default" for both. Choosing a
  model fills the tier with that model's suggestion; the owner may change
  it, and write skills, before sending.
- **Editing.** The AI member panel shows model, level, tier and skills.
  The owner changes any of them with `member set`. Beside the tier it
  shows the suggestion for the chosen model when the two differ; taking
  it is the owner's click.
- **Refresh.** A button on the panel sends `host.info` with `refresh`.
  A discovery changes `host.info` only. It never changes a member row,
  so what the owner set stays, whatever is found later.
- **No list.** When `models` is empty, model and level are text fields.
  A listed model whose `efforts` is empty is offered no level. A stored model that the list no longer holds is shown as "not
  listed" and kept.

## 3. Turn text (adds to `contracts/drivers.md` sections 3 and 4)

**Every turn of the secretary session** carries, between the session
header and the first item, the AI members of the group, the secretary
included, in the order of their names:

```
[SoloDawn] group "Website" · you are "Secretary" · secretary session
AI members (CLI on computer · tier · state · skills):
- "Opus" · claude on workstation · strong · working on T-12 · Go backend, protocol design
- "Haiku" · claude on workstation · light · dormant · small edits, documentation
- "Codex" · codex on laptop · standard · limited
- "Secretary" · claude on workstation · standard · working
== 1 of 1 · mention · from "owner" (person) in main · instruction
…
```

- `state` is the member row's `state`, with `on T-<num>` when the row has
  `task`. `limited` covers a vendor rate limit and a sign-in
  that is not valid; the `notice` that came when it happened says which
  (`contracts/task-machine.md` section 5). No new fact is collected.
- Empty `skills` leaves out the last part.
- The host reads the rows it holds when it composes the turn.

**The secretary's instructions** gain the four rules of assignment. When
choosing assignee and reviewer:

1. give hard work to `strong` AIs and simple work to `light` ones;
2. match the work to `skills`;
3. spread tasks that do not depend on each other over different AIs, so
   that several subscriptions work at the same time; members with the
   same CLI on the same computer share one subscription;
4. do not choose an AI that is `limited` or `offline`.

The secretary decides; `propose_brief` and `set_task` accept any working
AI, as before.

**A `checks` or `reviewed` item** of a task whose row has `stuck.ask`
`true` (section 4) ends with one more line, the candidates being the AI
members of the group that are stronger than the assignee, the secretary
among them, by name:

```
This is failure 2 in a row of the same kind (review). Before you submit again, call ask to one of: "Claude Code" (strong). Then continue with the answer.
```

**The task session's instructions** gain one sentence: when a turn says to
ask a stronger AI, do it with `ask` before `submit_task`.

## 4. Escalation (adds to `contracts/task-machine.md`)

A new optional field of the task row, `stuck` (adds to `contracts/rows.md`
section 4), present from a task's first counted failure until it is reset:

```json
"stuck":{"kind":"review","n":2,"ask":true}
```

| Field | Values |
| --- | --- |
| `kind` | the kind of the last counted failure: `checks` (hosted checks did not pass, row 6) or `review` (a review did not pass, row 7) |
| `n` | how many failures of that kind came in a row |
| `ask` | `true` while the home refuses a submission of this task |

What the rows of `contracts/task-machine.md` section 3 do beside what they
do today. Nothing else touches `stuck`: not a crash or a timeout (row 10),
not a failed `run_checks`, not a conflict (row 9).

| Row | When | `stuck` becomes |
| --- | --- | --- |
| 6, not passed | — | `kind` `checks`; `n` + 1 if `kind` was `checks`, else 1; `ask` as below |
| 7, not passed | — | `kind` `review`; `n` + 1 if `kind` was `review`, else 1; `ask` as below |
| 6, passed (and row 5 without a remote) | `kind` is `checks` | removed |
| 7, passed | — | removed |
| 11 | the assignee changed, or the task was `failed` | removed |
| 13 | the task was `failed` | removed |
| `post` kind `reply` | `ask` is `true`; the call is in the task's thread and by its assignee; the replier is an AI stronger than the assignee | `ask` `false` |
| 5 | `ask` is `true` | unchanged: the command is refused with `conflict`, and `msg` says to ask a stronger AI first |

- **`ask` after a failure** is `true` when `n` is 2 or more and the group
  has an AI stronger than the assignee, whatever its state; else `false`. The
  threshold 2 is fixed: it is no field of `limits`. Each further failure
  of the same kind asks again.
- **Without a remote repository** there is no hosted check, so only
  failed reviews count.
- **The secretary is a candidate** when its tier is higher: R-21 keeps
  it from writing code, not from answering.
- **Without a stronger AI nothing changes**: `ask` stays `false`, no line
  is added to the turn and no submission is refused.
- **Row 14 comes first.** A failure that stops the task (R-63) still
  counts in `stuck`, and the task is `failed` as today.
- **The answer may be to a call made before the failure**: what counts is
  that it arrives while `ask` is `true`. A reply to another member's
  call, or to the assignee's call in another thread, changes nothing. A call that is never answered, or
  is denied (step 2), leaves the task waiting; the quiet-task notice
  (row 23) brings the secretary, who reassigns (row 11).
- **`submit_task`** (adds to `contracts/mcp-tools.md` section 1) reads
  `stuck.ask` from the task row before it pushes anything and fails with
  `conflict`, its text naming the candidates. So a refused submission
  leaves no push, no pull request and no `report`. The home's refusal of
  row 5 is the rule; this is its order on the host.
- The web client adds one badge to the table of `contracts/rows.md`
  section 4, above "sent back by review": "must ask a stronger AI", when
  `state` is `active` and `stuck.ask` is `true`.

## 5. What is not built

- No assignment algorithm, no score, no automatic reassignment.
- No `tier` or `skills` in `list_members`: the turn text carries them to
  the two readers that need them. No model or level in any turn text.
- No check of a model or level against the list, no stored list, no
  automatic change of any member field.
- No new inbox item, message kind, card, command, error code, limit or
  storage table. `stuck` is part of the task row.
- No check that the answer helped, or that the stronger AI is reachable.
- No list of models or levels kept by the host.

## 6. Shapes to freeze (unit P-6)

Changed golden examples, with the schema in `web/src/lib/proto.ts` and
`internal/proto/testdata/wire.json`:

- `row-member-ai.json`: `model`, `effort`, `tier` and `skills`, always
  present.
- `member-set.json` and `member-add_ai.json` stay; new
  `member-set-profile.json` and `member-add_ai-profile.json` with the
  four fields.
- `host.join` with the four fields, `host.leave` with `member`, and
  `host.info` with `refresh` and `models`
  (`contracts/step-3/drivers.md` section 10).
- `row-task.json` stays, without `stuck`; new `row-task-stuck.json` with
  `stuck` and `ask` `true`.

`tools.json` does not change.

Decided in building P-6 (2026-11-23):

- `host-join.json` and `host-leave.json` change in place: the first with
  the four fields, the second with `member`. `host-info-request.json`
  carries `"refresh":true`.
- In all three commands each of the four fields is optional on its own.
  In `member set` a present `""` for `model` or `effort` returns to the
  CLI's default; in `member add_ai` and `host.join` a present `""` is the
  same as absent.
- `refresh` is a boolean; `false` is the same as absent. `fast` is `true`
  or absent, and a reader refuses `false`, as for `jump`.
- The wire readers read `tier` and `stuck.kind` as text; the home checks
  the values of a command (section 2), and the client shows what a row
  holds.

## 7. What grows in the step 1 files

| File | Addition |
| --- | --- |
| `contracts/rows.md` section 2 | `model`, `effort`, `tier`, `skills` on an AI's row |
| `contracts/rows.md` section 4 | the optional `stuck`; one badge |
| `contracts/commands.md` 1.2 | the four fields in `member add_ai` and `member set` |
| `contracts/commands.md` section 2 | the four fields in `host.join`; `member` in `host.leave` |
| `contracts/datachannel.md` section 7 | `refresh`, `models` in `host.info`; more than one entry of `groups` may name one `g` |
| `contracts/drivers.md` section 1 | a model and a level are passed at each start (it said: no model) |
| `contracts/task-machine.md` section 3 | the effects of section 4 on rows 5, 6, 7, 11, 13 and on a `reply` |
| `contracts/drivers.md` sections 3, 4 | the member list, the escalation line, the instruction sentences |
| `contracts/mcp-tools.md` section 1 | one more reason for `submit_task` to fail with `conflict` |

## 8. Decided, and not verified

Decided by the coordinator on 2026-11-22; the choices made in writing
this file and their answers are in `steps/parallel-plan-step-2-3.md`
section 5, and only there.

`[unverified]` That a model follows the added line and the four rules; the
home's refusal holds whether it does or not. What was run of the CLIs is
marked in `contracts/step-3/drivers.md` section 10.
