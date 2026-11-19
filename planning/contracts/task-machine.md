# Contract: the task machine

The one table of how a task row changes. `contracts/commands.md` names the
commands; this file says when each is allowed and what it does to a task.
Field shapes are in `contracts/rows.md` section 4. Rules: R-22 to R-24,
R-60, R-63, R-72, R-90.

## 1. The fields that change

| Field | Values |
| --- | --- |
| `state` | `queued`, `active`, `paused`, `failed`, `merged`, `cancelled`. The last two are final. |
| `step` | `branch`, `local_check`, `cloud_check`, `review`, `merge` |
| `wait` | `null`, or one waiting reason with its card and people |
| `wait.kind` | while `queued`: `depends`. While `active`: `approval`, `decision`, `reviewer`, `conflict`. While `paused`: `manual`, `rounds`, `preempted`. |
| `commit` | the submitted commit: the one hosted checks, review and merge are about |
| `failures` | consecutive failures (R-63) |
| `review.round` | number of reviews posted so far |

- The card shows six positions: branch, local check, pull request, hosted
  checks, review, merge. "Pull request" is done when `pr` is set. With
  `forge: none`, "pull request" and "hosted checks" are drawn as skipped.
- A paused group, and an assignee that is `limited` or `offline`, are not
  written into tasks. The client shows them from the group and member rows.

## 2. Rules that hold for every row

- **Refusal.** A command that arrives when its row does not allow it is
  answered `conflict` and changes nothing.
- **Roles.** Assignee and reviewer are working AIs of the group, never the
  secretary, and never the same AI.
- **Items go to active tasks only.** A queued or paused task gets no item.
  It gets the item for its current step when it becomes active.
- **Superseded items are retired.** When a row makes a queued inbox item
  pointless (another assignee, another reviewer, another commit, a cancelled
  task), the home deletes that item in the same transaction.
- **"Wait cleared" means recomputed.** The wait becomes the task's oldest
  pending approval card, else its pending decision card, else `null`. So a
  card that is still open is never forgotten by a pause and a resume.
- **Step `merge` is the point of no return.** From the moment a review
  passes until the host reports, the only things accepted for the task are
  rows 8 and 9 for the same commit, and row 15 by an admin.

## 3. Transitions

"Host" means the node of the task's assignee. `SEC` is the secretary. "Admin"
is a person with role `owner` or `admin`. Items are inbox items
(`contracts/datachannel.md` section 6); the recipient is in brackets.

| # | Command, by | Allowed when | Task becomes | Items | Counters |
| --- | --- | --- | --- | --- | --- |
| 1 | `act confirm` on a brief, by its requester or an admin | card pending | per entry: `queued`/`branch`/`depends` if a dependency is not merged, else `active`/`branch`/`null` | `assign` [assignee] per active task; `confirmed` [SEC]; both with this `act` as cause | — |
| 2 | `task start`, by a working AI | the AI is a member | new task `active`/`branch`/`null`, assignee = that AI, no reviewer, `origin` as given | `assign` [assignee] with message `re`; `notice` [SEC] | — |
| 3 | `task branch`, by host | `active`, step `branch` | step `local_check` | — | — |
| 4 | `task checked`, by host | `active`; step `local_check`, `cloud_check` or `review` | `checks.local` recorded; a `checks` message | — | — |
| 5 | `task submitted`, by host | `active`; step `local_check`, `cloud_check` or `review`; `checks.local.ok` and `checks.local.commit` = the submitted `commit` | `commit` set; a queued `review` item for the old commit is retired. With a remote: step `cloud_check`. Without: as row 6, passed | without a remote: as row 6, passed | — |
| 6 | `task cloud`, by host | `active`, `cloud_check`, same `commit` | passed: step `review`; without a reviewer, wait `reviewer`. Not passed: step `local_check`, a `checks` message, and row 14 if the limit is reached | passed: `review` [reviewer], or `notice` [SEC] if none. Not passed, below the limit: `checks` [assignee] | passed: `failures` = 0. Not passed: `failures` + 1 |
| 7 | `post` kind `review`, by the reviewer | `active`, `review`, wait not `reviewer`; `body.commit` = the task's `commit` | passed: step `merge`. Not passed, round ≤ `limits.review_rounds`: step `local_check`. Not passed, round above it: row 14 | passed: `merge` [the host], naming the commit and the pull request. Not passed, within the limit: `reviewed` [assignee] | `review.round` + 1 |
| 8 | `task merged`, by host | `active`, `merge`, same `commit` | `merged`; a `system` message `task_merged`; every queued task whose dependencies are now all merged becomes `active`/`branch`/`null` | `assign` [assignee] per task made active, with the cause of row 1; `notice` [SEC] | — |
| 9 | `task conflict`, by host | `active`, `merge`, same `commit` | step `local_check`, wait `conflict`; a `checks` message with `stage: merge` | `notice` [SEC] | not counted |
| 10 | `task fail` (reason `crash` or `timeout`), by host | `active`, not at step `merge` | unchanged, or row 14 if the limit is reached | below the limit: `notice` [SEC] with the detail | `failures` + 1 |
| 11 | `task assign`, by SEC or an admin; from `failed`, by an admin only | not final; not at step `merge` | fields set. Assignee changed: every pending approval card of the task becomes `reassigned`; the old assignee's queued items for the task are retired; `failed` becomes `active`. Reviewer changed: the old reviewer's queued `review` item is retired. Then, if the task is `active`: wait `conflict` cleared; at step `review`, wait `reviewer` cleared | if `active`: `assign` [assignee] with the latest `checks` or `review` message, when the assignee changed or the wait was `conflict`; `review` [reviewer], when the reviewer changed at step `review` | from `failed`: `failures` = 0 |
| 12 | `task pause`, by SEC or an admin | `active`, not at step `merge` | `paused`, wait `manual` | — | — |
| 13 | `task resume`, by SEC or an admin; from `failed`, by an admin only | `paused`, or `failed` | `active`, wait cleared | `resume` [assignee] | from `failed`: `failures` = 0 |
| 13b | `task resume`, by host | `paused` with wait `preempted` | `active`, wait cleared | — (the host continues the session itself) | — |
| 14 | (inside rows 6, 7 and 10) | `failures` reaches `limits.failures`, or a review fails above `limits.review_rounds` | `failed`; a `system` message `task_failed` mentioning the brief's requester and the assignee's owner | `notice` [SEC], once; no other item | — |
| 15 | `task cancel`, by SEC or an admin; by host if `origin` is `takeover`; at step `merge`, by an admin only | not final | `cancelled`; pending cards of the task become `expired`; every queued item of the task is retired | — | — |
| 16 | `task preempted`, by host | `active`, not at step `merge` | `paused`, wait `preempted`; a `system` message `preempted` | — | — |
| 17 | `post` kind `approval`, by the assignee | `active` | wait `approval` with the card | — | — |
| 18 | `act approve` or `deny`, by an approver | card pending | the card is decided and keeps the signed `act`; wait cleared | `decided` [the asking AI] with that `act` as cause | — |
| 19 | `post` kind `decision` naming the task, by an AI | not final | if `active` with wait `null`: wait `decision` with the card | — | — |
| 20 | `act choose`, by an addressee or an admin | card pending | the card is decided and keeps the signed `act`; wait cleared | `decided` [the asking AI; the assignee if another] | — |
| 21 | any `post` by an AI in the thread that mentions an AI | — | `thread.rounds` + 1; at `limits.rounds`, if `active` and not at step `merge`: `paused`, wait `rounds`, a `system` message `rounds_limit` in the main line | at the limit: `notice` [SEC] | — |
| 22 | `post` kind `text` by a person in the thread | — | `thread.rounds` = 0; if wait was `rounds`: `active`, wait cleared | — | — |
| 23 | timer in the home | `active`, wait `null`, not at step `merge`, no thread message for `limits.idle_notice_minutes` | unchanged | `notice` [SEC], once per quiet period | — |
| 24 | `group resume` | — | unchanged | `resume` [assignee] for each task that is `active` with wait `null` | — |

## 4. What follows from the table

- **One commit.** `submitted`, `cloud`, the review, the `merge` item and its
  outcome all name `commit`. A new submission replaces it and sends the task
  back through hosted checks and review. Anything about another commit is
  refused. The host merges exactly that commit (`contracts/local.md`
  section 7); if the branch has moved, that is row 9.
- **No return from `merge`.** Between row 7 and rows 8 or 9 nothing else
  changes the task: not a pause, not a reassignment, not a new submission,
  not the round limit. The one exception is a person's cancel (row 15), the
  way out when the host will never come back; a host that takes a `merge`
  item first looks at the task as it is now and drops the item if the task
  is no longer at `merge` for that commit (`contracts/datachannel.md`
  section 6). See `risks.md` K-14.
- **A member with unfinished work cannot be removed.** `member remove` is
  refused while the AI is assignee or reviewer of a task that is not final;
  reassign or cancel first (`contracts/commands.md` 1.2).
- **Six failed reviews stop a task.** With `limits.review_rounds` = 5, the
  first five failed reviews send the work back; the sixth is row 14.
- **A stopped task waits for a person.** After row 14 only an admin can
  resume or reassign it (R-63); the secretary may cancel it.
- **Failures are consecutive.** The counter grows on a crashed or timed-out
  turn and on failed hosted checks, and returns to zero only when hosted
  checks pass (or, without a remote, when a submission is accepted) and
  when a stopped task is resumed or reassigned.
- **Held by the round limit.** While a task's wait is `rounds`, `mention`
  items of its thread are not sent. However the wait ends (row 13 or 22),
  they go out again.
- **Pre-emption is the host's fact.** A host that interrupts a turn to run a
  queue jump in **another** session reports row 16, runs the jump turn,
  reports row 13b when it has ended, and gives the interrupted session a
  turn of its own. A jump aimed at the very session that is running just
  interrupts it and goes first; the task is not paused. If the host never
  reports 13b, row 13 does.
- **Paused means no turns.** A host gives no turn to the session of a task
  that is `paused`, and interrupts one that is running when the row arrives.
- **Final means gone.** When a task is `merged` or `cancelled`, the hosts
  end its sessions, remove its folders and forget its pending approvals. A
  task that depends on a cancelled task stays queued until it is cancelled
  too.

## 5. What the secretary is told

`notice` reaches the secretary for: a direct task (row 2); no reviewer (6);
a merge (8); a conflict (9); a crashed or timed-out turn (10); a stopped
task (14); the round limit (21); a quiet task (23). What it does next (ask,
reassign, cancel, bring it to a person) is its judgement, not the
platform's (R-22).
