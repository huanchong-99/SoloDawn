# Contract, step 3: AI profiles, assignment and escalation

What an owner says about each of their AIs, how the secretary is given it,
and when a stuck assignee must ask a stronger AI. Rule: R-165. Decided by
the coordinator on 2026-11-22; lane PROFILES of
`steps/parallel-plan-step-2-3.md` section 4.1 builds it.

This file adds to the step 1 contracts and changes nothing in them
(section 7). The product decides nothing here that needs judgement (R-2):
it stores two fields, prints them, counts failures and refuses one command.

Not in it: cost or usage caps (postponed with `settings.set`), switching
models inside a session, permission levels, audit rows, and any check
against a peer that lies.

## 1. The profile (adds to `contracts/rows.md` section 2)

Two fields on the member row of an AI, always present. A person's row has
neither.

| Field | Values | When never set |
| --- | --- | --- |
| `tier` | `strong`, `standard`, `light` | `standard` |
| `skills` | what the AI is good at: one line of at most 200 Unicode code points, without `\r` or `\n` | `""` |

```json
{"r":"member", …, "kind":"ai", "tier":"strong",
 "skills":"Go backend, protocol design, hard debugging", …}
```

- The order is `light` < `standard` < `strong`. An AI is **stronger** than
  another when its tier is higher. Nothing else in the product reads a
  tier, and nothing reads `skills` but the turn text of section 3.
- `member add_ai` carries neither field; the home writes the defaults. A
  group whose AIs all keep the defaults behaves as in step 1: no AI is
  stronger than another, so section 4 never asks for anything.
- The web client shows both on the AI member panel and card, and lets the
  AI's owner edit them there.

## 2. Setting it (adds to `contracts/commands.md` 1.2)

`member set` takes two more fields in its "any of": `tier` and `skills`.

- By the AI's owner only. The AI's host may not send them.
- A `tier` that is not one of the three values, and a `skills` that breaks
  the bound of section 1, are `bad_request`.
- A change takes effect for what happens next: the next turn of the
  secretary, the next failure of a task. A task that is already waiting
  for an answer (section 4) keeps waiting.

## 3. Turn text (adds to `contracts/drivers.md` sections 3 and 4)

**Every turn of the secretary session** carries, between the session
header and the first item, the AI members of the group, the secretary
included, in the order of their names:

```
[SoloDawn] group "Website" · you are "Secretary" · secretary session
AI members (tier · state · skills):
- "Claude Code" · strong · working on T-12 · Go backend, protocol design
- "Codex" · standard · limited
- "Qwen Code" · light · dormant · small edits, documentation
- "Secretary" · standard · working
== 1 of 1 · mention · from "owner" (person) in main · instruction
…
```

- `state` is the member row's `state`, with `on T-<num>` when the row has
  `task`. `limited` covers a usage cap, a vendor rate limit and a sign-in
  that is not valid; the `notice` that came when it happened says which
  (`contracts/task-machine.md` section 5). No new fact is collected.
- Empty `skills` leaves out the last part.
- The host reads the rows it holds when it composes the turn.

**The secretary's instructions** gain the four rules of assignment. When
choosing assignee and reviewer:

1. give hard work to `strong` AIs and simple work to `light` ones;
2. match the work to `skills`;
3. spread tasks that do not depend on each other over different AIs, so
   that several subscriptions work at the same time;
4. do not choose an AI that is `limited` or `offline`.

The secretary decides; `propose_brief` and `set_task` accept any working
AI, as before.

**A `checks` or `reviewed` item** of a task whose row has `stuck.ask`
`true` (section 4) ends with one more line, the candidates being the
working AIs of the group that are stronger than the assignee, by name:

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
| `post` kind `reply` | `ask` is `true`; the call is in the task's thread and by its assignee; the replier is a working AI stronger than the assignee | `ask` `false` |
| 5 | `ask` is `true` | unchanged: the command is refused with `conflict`, and `msg` says to ask a stronger AI first |

- **`ask` after a failure** is `true` when `n` is 2 or more and the group
  has a working AI stronger than the assignee; else `false`. The
  threshold 2 is fixed: it is no field of `limits`. Each further failure
  of the same kind asks again.
- **Without a stronger AI nothing changes**: `ask` stays `false`, no line
  is added to the turn and no submission is refused.
- **Row 14 comes first.** A failure that stops the task (R-63) still
  counts in `stuck`, and the task is `failed` as today.
- **The answer may be to a call made before the failure**: what counts is
  that it arrives while `ask` is `true`. A call that is never answered, or
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
- No `tier` or `skills` in `list_members`, in `host.info` or in the driver
  declaration (`contracts/step-3/drivers.md` section 4): the turn text
  carries them to the two readers that need them.
- No new inbox item, message kind, card, command, error code, limit or
  storage table. `stuck` is part of the task row.
- No check that the answer helped, or that the stronger AI is reachable.

## 6. Shapes to freeze (unit P-6)

Changed golden examples, with the schema in `web/src/lib/proto.ts` and
`internal/proto/testdata/wire.json`:

- `row-member-ai.json`: `tier` and `skills`, always present.
- `member-set.json` stays; new `member-set-profile.json` with `tier` and
  `skills`.
- `row-task.json` stays, without `stuck`; new `row-task-stuck.json` with
  `stuck` and `ask` `true`.

`tools.json` does not change.

## 7. What grows in the step 1 files

| File | Addition |
| --- | --- |
| `contracts/rows.md` section 2 | `tier`, `skills` on an AI's row |
| `contracts/rows.md` section 4 | the optional `stuck`; one badge |
| `contracts/commands.md` 1.2 | `tier`, `skills` in `member set` |
| `contracts/task-machine.md` section 3 | the effects of section 4 on rows 5, 6, 7, 11, 13 and on a `reply` |
| `contracts/drivers.md` sections 3, 4 | the member list, the escalation line, the instruction sentences |
| `contracts/mcp-tools.md` section 1 | one more reason for `submit_task` to fail with `conflict` |

## 8. Decided, and not verified

Decided by the coordinator (2026-11-22): the two fields and their
defaults; who sets them; that the secretary decides from turn text; the
threshold 2; the refusal of the next submission; nothing without a
stronger AI.

Chosen here, open to the coordinator
(`steps/parallel-plan-step-2-3.md` section 5):

- "Checks" means hosted checks: a failed `run_checks` reaches no row
  (`flows/task.md` F).
- A candidate is a working AI, not the secretary (R-21).
- Whether a stronger AI exists is decided at the failure, from tiers
  alone, whatever the state of that AI.
- The refusal reuses `conflict`.

`[unverified]` That a model follows the added line and the four rules; the
home's refusal holds whether it does or not. Nothing here was run.
