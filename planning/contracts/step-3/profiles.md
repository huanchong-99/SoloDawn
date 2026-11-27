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

- `tier` and `skills` are the **profile**. The tier says how much work
  the owner wants the AI given: `strong` more, `standard` a normal share,
  `light` fewer (2026-11-27, the owner: it is the share of work, not a
  thinking level). Who gets the hard and special tasks comes from
  `skills`, and by the higher tier when skills say nothing (2026-11-27,
  R-165 updated). For escalation (section 4), the order is
  `light` < `standard` < `strong`; an AI is **stronger** than another when
  its tier is higher (`issues.md`, open limitation). Nothing in the product
  reads a tier but section 4, and nothing reads `skills` but the turn text
  of section 3.
- The home stores `model` and `effort` and checks only the bounds of the
  table: it knows no CLI and no model. The host passes them to the CLI at
  each start and checks nothing either: a value the CLI refuses is handled
  as a crash, not as a lost session (`contracts/step-3/drivers.md` 10.4).
- **Several members from one CLI.** A host may add the same CLI to a
  group more than once, each with its own name, model and level. They
  share the CLI's account. Each is `limited` only when its own CLI
  reports it.
- **An external AI** (`contracts/step-3/external.md`) has the four fields
  too. Its `model` and `effort` are text for people to read: no listing
  offers them and nothing passes them to the agent. Its `tier` and
  `skills` count like any other.
- A member with all four defaults behaves as in step 1, with one
  difference: the secretary's turns carry the member list of section 3
  whatever the values. A group whose AIs
  all keep the default tier has no stronger AI, so section 4 never asks
  for anything.
- The web client shows the four on the AI member panel and card, and lets
  the AI's owner edit them there (section 2a), except the model and level
  of an external AI, which are text set when it joins
  (`contracts/step-3/external.md` section 11).

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

- **Adding.** The dialog of `host.join`, and each AI ticked in the
  create-group dialog, offers the models of that CLI from `host.info`
  (`contracts/step-3/drivers.md` section 10), for the chosen model its
  levels, and "the CLI's default" for both; what is chosen goes in that
  AI's `host.join`. Choosing a
  model fills the tier with that model's suggestion; the owner may change
  it before sending, and, in the add-AI dialog only, write skills.
- **Editing.** The AI member panel shows model, level, tier and skills,
  the tier in one block with the usage line of section 3 (`window_pct`
  and when it resets, or the turns of today for a CLI that reports no
  window). The owner changes any of the four in one editor
  with `member set`; its level is a choice of its own, offered from the
  chosen model's `efforts` and "the CLI's default". Beside the tier it
  shows the suggestion for the chosen model when the two differ; taking
  it is the owner's click.
- **The secretary.** No view shows or offers the secretary's tier (its
  card, its editor, the add-AI and create-group dialogs once it is chosen
  as secretary), because it takes no tasks; its stored tier stays as set,
  and only orders it among the candidates of section 4. Its card shows the
  usage line alone, headed "Quota".
- **Refresh.** A button on the panel sends `host.info` with `refresh`.
  A discovery changes `host.info` only. It never changes a member row,
  so what the owner set stays, whatever is found later.
- **No list.** When `models` is empty, model and level are text fields.
  A listed model whose `efforts` is empty, and no model chosen, give a level choice that holds only "the CLI's default", disabled, with the reason (the model has no levels; choose a model first). A stored model that the list no longer holds is shown as "not
  listed" and kept.

## 3. Turn text (adds to `contracts/drivers.md` sections 3 and 4)

**Every turn of the secretary session** carries, between the session
header and the first item, the AI members of the group, the secretary
included, in the order of their names:

```
[SoloDawn] group "Website" · you are "Secretary" · secretary session
AI members (CLI on computer · folder · tier · state · skills):
- "Opus" · claude on workstation · /home/dev/website · strong · working on T-12 · window 40% used, resets in 3 h · Go backend, protocol design
- "Haiku" · claude on workstation · /home/dev/website · light · dormant · window 40% used, resets in 3 h · small edits, documentation
- "Codex" · codex on laptop · /srv/work/website · standard · limited · window 100% used
- "Secretary" · claude on workstation · /home/dev/website · standard · working
== 1 of 1 · mention · from "owner" (person) in main · instruction
…
```

- The folder is the member row's `workspace`, in full
  (`contracts/rows.md` section 2). The secretary reads from it and the
  computer which AIs share a folder, which takes one task at a time, and
  which reviewer can read an assignee's work without a push
  (`contracts/drivers.md` section 4).
- `state` is the member row's `state`, with `on T-<num>` when the row has
  `task`. `limited` covers a vendor rate limit and a sign-in
  that is not valid; the `notice` that came when it happened says which
  (`contracts/task-machine.md` section 5). No new fact is collected.
- Empty `skills` leaves out the last part.
- An external AI has `external` where the CLI stands, with its `tool` in
  brackets when the row has one, and one of the states of
  `contracts/step-3/external.md` section 6:
  `- "Scout" · external (ZCode) on workstation · /home/dev/website · standard · offline · Rust, reviews`
- An AI whose member row's `usage` has `window_pct` gets
  `window <pct>% used, resets in <n> h` after its state, `n` being the
  hours until `window_resets`, rounded up; without `window_resets` the
  "resets" part is left out. When `window_resets` is present and not in
  the future (`<= now`), the whole `usage` part is left out because the
  window has ended. There is no clock time and no date. A
  managed AI's `usage` comes from its CLI's events, an external AI's from
  `report_usage`. No caps.
- The host reads the rows it holds when it composes the turn.

**The secretary's instructions** gain the five rules of assignment,
applied task by task (not by the request the task came from). When
choosing assignee and reviewer, in order:

1. never choose an AI that is `limited` or `offline`;
2. the window first: give an AI with little of its window left for the
   hours until the reset only the tasks that need that AI, and the
   others to AIs with room; do not use up one subscription while another
   has room (members with the same CLI on the same computer share one
   subscription, not two external AIs);
3. the tier is the share of tasks its owner wants each AI given
   (`strong` more, `standard` a normal share, `light` fewer);
4. `skills` say which tasks: the hard and the special ones go to the AIs
   whose skills name them, and when skills say nothing, to the higher
   tier;
5. spread independent tasks over different AIs, so several subscriptions
   work at the same time.

The secretary decides; `propose_brief` and `set_task` accept any working
AI, as before.

**A `reviewed` item** of a task whose row has `stuck.ask`
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
| `kind` | the kind of the last counted failure. One value since the decision of 2026-11-26: `review` (a review did not pass, row 7) |
| `n` | how many failures of that kind came in a row |
| `ask` | `true` while the home refuses a submission of this task |

What the rows of `contracts/task-machine.md` section 3 do beside what they
do today. Nothing else touches `stuck`: not a crash or a timeout (row 10).

| Row | When | `stuck` becomes |
| --- | --- | --- |
| 7, not passed | — | `kind` `review`; `n` + 1 if the row had `stuck`, else 1; `ask` as below |
| 7, passed | — | removed |
| 11 | the assignee changed, or the task was `failed` | removed |
| 13 | the task was `failed` | removed |
| `post` kind `reply` | `ask` is `true`; the call is in the task's thread and by its assignee; the replier is an AI stronger than the assignee | `ask` `false` |
| 5 | `ask` is `true` | unchanged: the command is refused with `conflict`, and `msg` says to ask a stronger AI first |

- **`ask` after a failure** is `true` when `n` is 2 or more and the group
  has an AI stronger than the assignee, whatever its state; else `false`. The
  threshold 2 is fixed: it is no field of `limits`. Each further failure
  of the same kind asks again.
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
  `stuck.ask` from the task row before it sends anything and fails with
  `conflict`, its text naming the candidates. So a refused submission
  leaves no `report`. The home's refusal of row 5 is the rule; this is its
  order on the host.
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

## 7. What grows in the step 1 files

| File | Addition |
| --- | --- |
| `contracts/rows.md` section 2 | `model`, `effort`, `tier`, `skills` on an AI's row |
| `contracts/rows.md` section 4 | the optional `stuck`; one badge |
| `contracts/commands.md` 1.2 | the four fields in `member add_ai` and `member set` |
| `contracts/commands.md` section 2 | the four fields in `host.join`; `member` in `host.leave` |
| `contracts/datachannel.md` section 7 | `refresh`, `models` in `host.info`; more than one entry of `groups` may name one `g` |
| `contracts/drivers.md` section 1 | a model and a level are passed at each start (it said: no model) |
| `contracts/task-machine.md` section 3 | the effects of section 4 on rows 5, 7, 11, 13 and on a `reply` |
| `contracts/drivers.md` sections 3, 4 | the member list, the escalation line, the instruction sentences |
| `contracts/mcp-tools.md` section 1 | one more reason for `submit_task` to fail with `conflict` |

## 8. Decided, and not verified

Decided by the coordinator on 2026-11-22; the choices made in writing
this file and their answers are in `steps/parallel-plan-step-2-3.md`
section 5, and only there.

`[unverified]` That a model follows the added line and the rules of assignment,
the boundary and the announcements; the
home's refusal holds whether it does or not. What was run of the CLIs is
marked in `contracts/step-3/drivers.md` section 10.
