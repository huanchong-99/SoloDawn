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
| `wait.kind` | while `queued`: `depends`. While `active`: `approval`, `decision`, `reviewer`, `assignee`, `conflict`. While `paused`: `manual`, `rounds`, `preempted`. |
| `commit` | the submitted commit: the one hosted checks, review and merge are about |
| `failures` | consecutive failures (R-63) |
| `review.round` | number of reviews posted so far |

- The card shows six positions: branch, local check, pull request, hosted
  checks, review, merge. "Pull request" is done when `pr` is set. With
  `forge: none`, "pull request" and "hosted checks" are drawn as skipped.
- `wait` holds one reason, the latest. What a person must act on is always
  read from pending cards, never from `wait`.
- A paused group, and an assignee that is `limited` or `offline`, are not
  written into tasks. The client shows them from the group and member rows.

## 2. Transitions

"Host" means the node of the task's assignee unless stated. `SEC` is the
secretary's host. "Admin" is a person with role `owner` or `admin`. Items
are inbox items (`contracts/datachannel.md` section 6); the recipient is in
brackets. A command that arrives when its row does not allow it is answered
`conflict` and changes nothing.

| # | Command, by | Allowed when | Task becomes | Items | Counters |
| --- | --- | --- | --- | --- | --- |
| 1 | `act confirm` on a brief, by its requester or an admin | card pending; every listed assignee and reviewer is an AI member; reviewer differs from assignee | per entry: `queued`/`branch`/`depends` if a dependency is not merged, else `active`/`branch`/`null` | `assign` [assignee] per active task; `confirmed` [secretary] | — |
| 2 | `task start`, by the host of an AI | the AI is a member and not the secretary | new task `active`/`branch`/`null`, assignee = the AI, no reviewer, `origin` as given | `assign` [assignee] with message `re`; `notice` [secretary] | — |
| 3 | `task branch`, by host | `active`, `branch` | step `local_check` | — | — |
| 4 | `task checked`, by host | `active`; step `local_check`, `cloud_check` or `review` | `checks.local` recorded; a `checks` message | — | — |
| 5 | `task submitted`, by host | `active`; step `local_check`, `cloud_check` or `review`; `checks.local.ok` and `checks.local.commit` = the submitted `commit` | `commit` set. With a remote: step `cloud_check`. Without: as row 6, passed | without a remote: as row 6, passed | — |
| 6 | `task cloud`, by host | `active`, `cloud_check`, same `commit` as the task | passed: step `review`; if there is no reviewer, wait `reviewer`. Not passed: step `local_check`; a `checks` message | passed: `review` [reviewer], or `notice` [secretary] if none. Not passed: `checks` [assignee] | passed: `failures` = 0. Not passed: `failures` + 1 (row 15) |
| 7 | `post` kind `review`, by the reviewer's host | `active`, `review`, wait not `reviewer`; `as` is the reviewer; `body.commit` = the task's `commit` | passed: step `merge`. Not passed, round ≤ `limits.review_rounds`: step `local_check`. Not passed, round above it: `failed` | passed: `merge` [assignee's host]. Not passed: `reviewed` [assignee]. Failed: `notice` [secretary] | `review.round` + 1 |
| 8 | `task merged`, by host | `active`, `merge`, same `commit` | `merged`; a `system` message `task_merged`; every queued task whose dependencies are now all merged becomes `active`/`branch`/`null` | `assign` [assignee] per task made active; `notice` [secretary] | — |
| 9 | `task conflict`, by host | `active`, `merge` | step `local_check`, wait `conflict`; a `checks` message with `stage: merge` | `notice` [secretary] | not counted |
| 10 | `task fail` (reason `crash` or `timeout`), by host | `active` | unchanged | `notice` [secretary] with the detail | `failures` + 1 (row 15) |
| 11 | `task assign`, by SEC or an admin | not final; reviewer differs from assignee | fields set. New assignee: a pending approval card becomes `reassigned`; wait cleared if `assignee`, `conflict` or `approval`; `failed` becomes `active`. New reviewer at step `review`: wait cleared if `reviewer` | new assignee, or the same one named while wait was `conflict`: `assign` [assignee] with the latest `checks` or `review` message. New reviewer at step `review`: `review` [reviewer] | from `failed`: `failures` = 0 |
| 12 | `task pause`, by SEC or an admin | `active` | `paused`, wait `manual` | — | — |
| 13 | `task resume`, by SEC or an admin | `paused`, or `failed` | `active`, wait `null` | `resume` [assignee] | from `failed`: `failures` = 0 |
| 14 | `task cancel`, by SEC or an admin; by host if `origin` is `takeover` | not final | `cancelled`; pending cards of the task become `expired` | — | — |
| 15 | (inside rows 6 and 10) | `failures` reaches `limits.failures` | `failed`; a `system` message `task_failed` mentioning the brief's requester and the assignee's owner | `notice` [secretary] | — |
| 16 | `task preempted`, by host | `active` | `paused`, wait `preempted`; a `system` message `preempted` | — | — |
| 17 | `task continued`, by host | `paused` with wait `preempted` | `active`, wait `null` | — (the host continues the session itself) | — |
| 18 | `post` kind `approval`, by host | `active` | wait `approval` with the card | — | — |
| 19 | `act approve` or `deny`, by an approver | card pending | wait `null` if it was this card | `decided` [the asking AI] with the signed `act` | — |
| 20 | `post` kind `decision` naming the task, by an AI's host | not final | if `active` with wait `null`: wait `decision` with the card | — | — |
| 21 | `act choose`, by an addressee or an admin | card pending | wait `null` if it was this card | `decided` [the asking AI; the assignee if another] | — |
| 22 | any `post` by an AI in the thread that mentions an AI | — | `thread.rounds` + 1; at `limits.rounds`, if `active`: `paused`, wait `rounds`, a `system` message `rounds_limit` in the main line | at the limit: `notice` [secretary]; `mention` items of this thread are held | — |
| 23 | `post` kind `text` by a person in the thread | — | `thread.rounds` = 0; if wait was `rounds`: `active`, wait `null`, held items released | — | — |
| 24 | `member remove` of the assignee or reviewer | not final | assignee removed: wait `assignee`. Reviewer removed at step `review`: wait `reviewer` | `notice` [secretary] | — |
| 25 | timer in the home | `active`, wait `null`, no thread message for `limits.idle_notice_minutes` | unchanged | `notice` [secretary], once per quiet period | — |
| 26 | `group resume` | — | unchanged | `resume` [assignee] for each task that is `active` with wait `null` | — |

## 3. What follows from the table

- **One commit.** `submitted`, `cloud`, the review and the merge all name
  `commit`. A new submission replaces it and sends the task back through
  hosted checks and review. A review or a hosted result for another commit
  is refused. The host merges exactly that commit (`contracts/local.md`
  section 7); if the branch has moved, the merge is refused by Git or the
  forge and reported as row 9.
- **A different AI reviews.** Rows 1 and 11 refuse a reviewer equal to the
  assignee (R-90).
- **Six failed reviews stop a task.** With `limits.review_rounds` = 5, the
  first five failed reviews send the work back; the sixth makes it `failed`.
- **Failures are consecutive.** The counter grows on a crashed or timed-out
  turn and on failed hosted checks, and returns to zero only when hosted
  checks pass (or, without a remote, when a submission is accepted) and
  when a failed task is resumed or reassigned.
- **Pre-emption is the host's fact.** The host that interrupts a turn for a
  queue jump reports row 16, runs the jump turn, reports row 17 when that
  turn has ended, and then gives the interrupted session a turn of its own.
  The home derives nothing from `member status` here.
- **Paused means no turns.** A host gives no turn to the session of a task
  that is `paused`, and interrupts one that is running when the row arrives.
- **Final means gone.** When a task is `merged` or `cancelled`, the hosts
  end its sessions, remove its folders and forget its pending approvals.
- **Resume after a group pause** skips tasks that wait for a card.

## 4. What the secretary is told

`notice` reaches the secretary for: a direct task (row 2); no reviewer (6);
a failed sixth review (7); a merge (8); a conflict (9); a crashed or
timed-out turn (10); a failed task (15); the round limit (22); a removed
member (24); a quiet task (25). What it does next (ask, reassign, cancel,
bring it to a person) is its judgement, not the platform's (R-22).
