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
| `review.failed` | failed reviews since the task was created or a person last resumed or reassigned it from `failed` (R-63) |

- The card shows six positions: branch, local check, pull request, hosted
  checks, review, merge. "Pull request" is done when `pr` is set. With
  `forge: none`, "pull request" and "hosted checks" are drawn as skipped.
- A paused group, and an assignee that is `limited` or `offline`, are not
  written into tasks. The client shows them from the group and member rows.

## 2. Rules that hold for every row

- **Refusal.** A command that arrives when its row does not allow it is
  answered `conflict` and changes nothing; it is answered `paused` instead
  when the row would allow it for an `active` task and the task is
  `paused` (rows 7, 10, 12, 16, 17).
- **Roles.** Assignee and reviewer are working AIs of the group, never the
  secretary, and never the same AI.
- **Work items go to active tasks only.** The work items are `assign`,
  `checks`, `reviewed`, `review`, `resume` and `merge`. A queued, paused or
  failed task gets none. It gets the work item for its current step when it
  becomes active (rows 8, 11, 13, 13b, 22): at step `review` that is
  `review` for the current reviewer and commit, unless one is already
  queued for them; with no reviewer the wait becomes `reviewer` again.
  Every other item (`mention`, `reply`, `decided`, `notice`, `confirmed`)
  is queued whatever the task's state; hosts hold those for the sessions of
  a paused or failed task until it is active again (`flows/agent-turn.md`
  B).
- **Host facts while paused.** Rows 3 to 6 are also accepted while the task
  is `paused`. They change the step, `commit`, checks and counters as
  written, row 14 included, and leave the wait as it is; they queue no work
  item, since activation gives it.
- **Superseded items are retired.** When a row makes a queued inbox item
  pointless (another assignee, another reviewer, another commit, a cancelled
  task), the home deletes that item in the same transaction.
- **"Wait cleared" means recomputed.** The wait becomes the task's oldest
  pending approval card, else its pending decision card, else `null`. So a
  card that is still open is never forgotten by a pause and a resume. A
  card sets or clears the wait only when the wait is `null` or already a
  card wait (`approval`, `decision`): a `reviewer` or `conflict` wait stays
  until the row that ends it (5, 6, 11). This happens only for an `active`
  task: deciding a card of a `paused` task changes the card and leaves the
  paused wait as it is; the card waits are recomputed when the task becomes
  active.
- **Every pause keeps the wait it replaced.** Rows 12, 16 and 21 write
  into the paused wait (`manual`, `preempted`, `rounds`) one field, `kept`:
  the kind of the wait it replaced, when that was `reviewer` or `conflict`
  (card waits need no keeping: they are recomputed). The row that ends the
  pause (13, 13b or 22) restores a kept wait instead of clearing it. A row
  that ends such a wait (5, 11) drops `kept` while the task is paused.
- **Step `merge` is the point of no return.** From the moment a review
  passes until the host reports, the only things that change the task's
  state, step, wait, commit, assignee or reviewer are rows 8 and 9 for the
  same commit, and row 15 by an admin. This rule comes before every row
  below; rows 17 to 22 are no exception. The thread summary still follows
  the thread.

## 3. Transitions

"Host" means the node of the task's assignee. `SEC` is the secretary. "Admin"
is a person with role `owner` or `admin`. Items are inbox items
(`contracts/datachannel.md` section 6); the recipient is in brackets.

| # | Command, by | Allowed when | Task becomes | Items | Counters |
| --- | --- | --- | --- | --- | --- |
| 1 | `act confirm` on a brief, by its requester or an admin | card pending; the checks of a `brief` post (`contracts/commands.md` 1.3) still hold | per entry: `queued`/`branch`/`depends` if a dependency is not merged, else `active`/`branch`/`null` | `assign` [assignee] per active task; `confirmed` [SEC]; both with this `act` as cause | — |
| 2 | `task start`, by a working AI | the AI is a member | new task `active`/`branch`/`null`, assignee = that AI, no reviewer, `origin` and `plan` as given, `foundation` false, `pass` = `group.pass`; a `task` message | `assign` [assignee] with message `re`, except for `origin` `takeover`; `notice` [SEC] | — |
| 3 | `task branch`, by host | `active` or `paused`, step `branch` | step `local_check` | — | — |
| 4 | `task checked`, by host | `active` or `paused`; step `local_check`, `cloud_check` or `review` | `checks.local` recorded; a `checks` message | — | — |
| 5 | `task submitted`, by host | `active` or `paused`; step `local_check`, `cloud_check` or `review`; `checks.local.ok` and `checks.local.commit` = the submitted `commit` | `commit` set; a queued `review` item for the old commit is retired; a `reviewer` or `conflict` wait is cleared, or dropped from `kept`. With a remote: step `cloud_check`. Without: as row 6, passed | without a remote: as row 6, passed | without a remote: as row 6, passed |
| 6 | `task cloud`, by host | `active` or `paused`, `cloud_check`, same `commit` | `checks.cloud` recorded. Passed: step `review`; without a reviewer, wait `reviewer`. Not passed: step `local_check`, a `checks` message, and row 14 if the limit is reached | passed: `review` [reviewer], or `notice` [SEC] if none. Not passed, below the limit: `checks` [assignee] | passed: `failures` = 0. Not passed: `failures` + 1 |
| 7 | `post` kind `review`, by the reviewer | `active`, `review`, wait not `reviewer`; `body.commit` = the task's `commit` | passed: step `merge`. Not passed, `review.failed` ≤ `limits.review_rounds`: step `local_check`. Not passed, above it: row 14 | passed: `merge` [the host], naming the commit and the pull request. Not passed, within the limit: `reviewed` [assignee] | `review.round` + 1; not passed: `review.failed` + 1 |
| 8 | `task merged`, by host | `active`, `merge`, same `commit` | `merged`, wait `null`; pending cards of the task become `expired`; a `system` message `task_merged`; every queued task whose dependencies are now all merged becomes `active`/`branch`/`null` | `assign` [assignee] per task made active, with the cause of row 1; `notice` [SEC] | — |
| 9 | `task conflict`, by host | `active`, `merge`, same `commit` | step `local_check`, wait `conflict`; a `checks` message with `stage: merge` | `notice` [SEC] | not counted |
| 10 | `task fail` (reason `crash` or `timeout`), by host | `active`, not at step `merge` | unchanged, or row 14 if the limit is reached | below the limit: `notice` [SEC] with the detail | `failures` + 1 |
| 11 | `task assign`, by SEC or an admin; from `failed`, by an admin only | not final; not at step `merge` | fields set. Assignee changed: every pending approval card of the task becomes `reassigned`; the old assignee's queued items for the task are retired. Reviewer changed: the old reviewer's queued `review` item is retired. From `failed`: `active`, whichever field changed. Then, if the task is `active`: wait `conflict` cleared; at step `review`, wait `reviewer` cleared when the task now has a reviewer. If it is `paused`, the same applies to `kept` | if `active`: `assign` [assignee] with the latest `checks` or `review` message, when the assignee changed or the wait was `conflict`; `review` [reviewer], when the reviewer changed at step `review` | from `failed`: `failures` = 0, `review.failed` = 0 |
| 12 | `task pause`, by SEC or an admin | `active`, not at step `merge` | `paused`, wait `manual`, with `kept` (section 2) | — | — |
| 13 | `task resume`, by SEC or an admin; from `failed` or from wait `rounds`, by an admin only | `paused`, or `failed` | `active`, with the wait that `kept` names if there is one, else wait cleared; from wait `rounds`: `thread.rounds` = 0 | `resume` [assignee]; at step `review`, what section 2 says about activation | from `failed`: `failures` = 0, `review.failed` = 0 |
| 13b | `task resume`, by host | `paused` with wait `preempted` | `active`, with the wait that `kept` names if there is one, else wait cleared | — (the host continues the session itself); at step `review`, what section 2 says about activation | — |
| 14 | (inside rows 6, 7 and 10) | `failures` reaches `limits.failures`, or `review.failed` goes above `limits.review_rounds` | `failed`, wait `null`; a `system` message `task_failed` mentioning the brief's requester and the assignee's owner | `notice` [SEC], once; no other item | — |
| 15 | `task cancel`, by SEC or an admin; by host if `origin` is `takeover`; at step `merge`, by an admin only | not final | `cancelled`, wait `null`; pending cards of the task become `expired`; every queued item of the task is retired | `notice` [SEC] for each queued task that depends on it | — |
| 16 | `task preempted`, by host | `active`, not at step `merge` | `paused`, wait `preempted`, with `kept` (section 2); a `system` message `preempted` | — | — |
| 17 | `post` kind `approval`, by the assignee | `active` | wait `approval` with the card, when the wait is `null` or a card wait | — | — |
| 18 | `act approve` or `deny`, by an approver | card pending | the card is decided and keeps the signed `act`; wait cleared | `decided` [the asking AI] with that `act` as cause | — |
| 19 | `post` kind `decision` naming the task, by an AI | not final | if `active` with wait `null`: wait `decision` with the card | — | — |
| 20 | `act choose`, by an addressee or an admin | card pending | the card is decided and keeps the signed `act`; wait cleared | `decided` [the asking AI; the assignee if another] | — |
| 21 | any `post` by an AI in the thread that mentions an AI | — | `thread.rounds` + 1; at or above `limits.rounds`, if `active` and not at step `merge`: `paused`, wait `rounds`, with `kept` (section 2), a `system` message `rounds_limit` in the main line | at the limit: `notice` [SEC] | — |
| 22 | `post` kind `text` by a person in the thread | — | `thread.rounds` = 0; if wait was `rounds`: `active`, with the wait that `kept` names if there is one, else wait cleared | — | — |
| 23 | timer in the home | `active`, wait `null`, not at step `merge`, no thread message for `limits.idle_notice_minutes` | unchanged | `notice` [SEC], once per quiet period | — |
| 24 | `group resume` | the group is paused; a resume of a group that is not paused is accepted and changes nothing | unchanged | `resume` [assignee] for each task that is `active` with wait `null` and not at step `merge`; `review` [reviewer] for each `active` task at step `review` that has a reviewer, unless one is queued | — |

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
- **A member with unfinished work cannot be removed or made secretary.**
  `member remove` and `group set` of `secretary` are refused while the AI is
  assignee or reviewer of a task that is not final; reassign or cancel
  first (`contracts/commands.md` 1.1, 1.2).
- **Six failed reviews stop a task.** With `limits.review_rounds` = 5, the
  first five failed reviews send the work back; the sixth is row 14. A
  review that passed before a conflict does not count.
- **A stopped task waits for a person.** After row 14 only an admin can
  resume or reassign it (R-63), which resets `failures` and
  `review.failed`; the secretary may cancel it.
- **Failures are consecutive.** The counter grows on a crashed or timed-out
  turn and on failed hosted checks, and returns to zero only when hosted
  checks pass (or, without a remote, when a submission is accepted) and
  when a stopped task is resumed or reassigned.
- **Held by the round limit.** While a task's wait is `rounds`, `mention`
  items of its thread are not sent. However the wait ends (row 13 or 22),
  they go out again. Only a person ends it (R-60): an admin's resume or a
  person's message in the thread.
- **Pre-emption is the host's fact.** A host that interrupts a turn to run a
  queue jump in **another** session reports row 16, runs the jump turn,
  reports row 13b when it has ended, and gives the interrupted session a
  turn of its own. A jump aimed at the very session that is running just
  interrupts it and goes first; the task is not paused. If the host never
  reports 13b, row 13 does.
- **Paused or failed means no turns.** A host gives no turn to the sessions
  of a task that is `paused` or `failed`, and interrupts one that is running
  when the row arrives.
- **Final means gone.** When a task is `merged` or `cancelled`, the hosts
  end its sessions, remove its folders and forget its pending approvals. A
  task that depends on a cancelled task stays queued until it is cancelled
  too; the secretary is told (row 15).

## 5. What the secretary is told

Every `notice` names its case in `code` (`contracts/datachannel.md` section
6). `task` is the task the case is about, `msg` the message that records
it, and `detail` the host's text, each where the table gives one.

| `code` | Case | `msg` | `detail` |
| --- | --- | --- | --- |
| `task_started` | a direct or takeover task (row 2) | the `task` message | — |
| `no_reviewer` | a submission passed hosted checks, or was accepted without a remote, and the task has no reviewer (rows 5, 6) | — | — |
| `task_merged` | a merge (row 8) | the `system` message `task_merged` | — |
| `conflict` | the merge did not apply (row 9) | the `checks` message, stage `merge` | — |
| `crash`, `timeout` | a turn of the task session crashed or timed out, below the limit (row 10) | — | the `detail` of `task fail` |
| `task_failed` | a stopped task (row 14) | the `system` message `task_failed` | — |
| `dependency_cancelled` | a cancel left this queued task waiting for a dependency that will never merge (row 15) | — | — |
| `rounds_limit` | the round limit (row 21) | the `system` message `rounds_limit` | — |
| `quiet` | a quiet task (row 23) | — | — |
| `handback` | a hand-back summary (`contracts/commands.md` 1.3) | the `handback` message | — |
| `login_invalid`, `cap_reached`, `rate_limited` | an AI is limited (`contracts/commands.md` 1.9) | the `system` message of that event | — |

What the secretary does next (ask, reassign, cancel, bring it to a person)
is its judgement, not the platform's (R-22).
