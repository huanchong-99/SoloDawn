# Contract: the task machine

The one table of how a task row changes. `contracts/commands.md` names the
commands; this file says when each is allowed and what it does to a task.
Field shapes are in `contracts/rows.md` section 4. Rules: R-12, R-22 to
R-24, R-60, R-63, R-72, R-90. In code the table is `Step` in
`internal/home/machine.go`: one task row and one event in, the row and what
the home does beside writing it out.

The platform creates no branch, runs no check and merges nothing: a task is
work, then a scored review, then done (the owner's decision of 2026-11-26).
Rows keep the numbers they had before that decision; 3, 4, 6, 8 and 9 are
not used.

## 1. The fields that change

| Field | Values |
| --- | --- |
| `state` | `queued`, `active`, `paused`, `failed`, `done`, `cancelled`. The last two are final. |
| `step` | `work`, `review` |
| `wait` | `null`, or one waiting reason with its card and people |
| `wait.kind` | while `queued`: `depends`, `busy`. While `active`: `approval`, `decision`, `reviewer`. While `paused`: `manual`, `rounds`, `preempted`. |
| `report` | the id of the `report` message that is under review: the latest submission. Absent before the first |
| `failures` | consecutive failures (R-63) |
| `review.round` | number of reviews posted so far |
| `review.failed` | failed reviews since the task was created or a person last resumed or reassigned it from `failed` (R-63) |

- The card shows three positions: work, review, done.
- A paused group, and an assignee that is `limited` or `offline`, are not
  written into tasks. The client shows them from the group and member rows.

## 2. Rules that hold for every row

- **Refusal.** A command that arrives when its row does not allow it is
  answered `conflict` and changes nothing; it is answered `paused` instead
  when the row would allow it for an `active` task and the task is
  `paused` (rows 7, 10, 12, 16, 17). The sender is checked first: a command
  from a sender its row never names is answered `forbidden`, and so is one
  that the task, as it is, allows from another sender only (the secretary's
  resume or reassignment of a `failed` task, a host's resume of a task it
  did not pre-empt).
- **Roles.** Assignee and reviewer are working AIs of the group, never the
  secretary, and never the same AI. A command that would make them the same
  AI is answered `conflict`.
- **One task at a time per folder** (R-12). The folder of a task is its
  assignee's: the pair `host` and `workspace` of that member row
  (`contracts/rows.md` section 2). A task is **started** when it is
  `active`, `paused` or `failed`. A folder is **busy** while a started task
  has it. Two AIs of one group that were added with one folder on one
  computer share it, and so share this rule. The rule is the home's and
  holds inside a group; that a folder serves one group only is the host's
  rule (`contracts/local.md` section 7).
- **Starting a queued task.** A `queued` task starts when every task in its
  `depends` is `done` and its folder is not busy with another task. Until
  then its wait says why: `depends` while a dependency is not done, else
  `busy`. The home looks at the queued tasks of the group whenever a row
  may have changed that: a task became `done` or `cancelled` (rows 7, 15),
  or a task got another assignee (row 11). It goes through them in the
  order of `num`, lowest first, and judges each against the tasks as the
  ones before it left them: a task that it starts makes its folder busy for
  those after it. So two queued tasks of one folder never start together.
  A task that starts becomes `active` at step `work`, its `stats.started`
  is that moment, its wait is recomputed (below) and `assign` is queued for
  its assignee. A task that stays `queued` gets the wait that holds now.
  Rows 1 and 2 create their tasks by the same judgement, row 1 in the order
  of the brief's entries.
- **Work items go to active tasks only.** The work items are `assign`,
  `reviewed`, `review` and `resume`. A queued, paused or failed task gets
  none. It gets the work item for its current step when it becomes active
  (a start, rows 11, 13, 13b, 22): at step `review` that is `review` for
  the current reviewer and report, unless one is already queued for them;
  with no reviewer the wait becomes `reviewer` again. Every other item
  (`mention`, `reply`, `decided`, `notice`, `confirmed`) is queued whatever
  the task's state; hosts hold those for the sessions of a paused or failed
  task until it is active again (`flows/agent-turn.md` B).
- **A submission while paused.** Row 5 is also accepted while the task is
  `paused`: a pause can arrive while the assignee's `submit_task` is on its
  way. It changes `report`, the step and the counter as written and leaves
  the wait as it is; it queues no work item, since activation gives it.
- **Superseded items are retired.** When a row makes a queued inbox item
  pointless (another assignee, another reviewer, another report, a cancelled
  task), the home deletes that item in the same transaction.
- **"Wait cleared" means recomputed.** The wait becomes the task's oldest
  pending approval card, else its oldest pending decision card, else `null`.
  So a card that is still open is never forgotten by a pause and a resume. A
  card sets or clears the wait only when the wait is `null` or already a
  card wait (`approval`, `decision`): a `reviewer` wait stays until the row
  that ends it (11). This happens only for an `active` task: deciding a
  card of a `paused` task changes the card and leaves the paused wait as it
  is; the card waits are recomputed when the task becomes active, by
  whichever row (a start, 11, 13, 13b, 22). The `since` of a card wait is
  the card's `ts`; the `since` of any other wait is the time of the row
  that set it, a restored `kept` wait included.
- **Every pause keeps the wait it replaced.** Rows 12, 16 and 21 write
  into the paused wait (`manual`, `preempted`, `rounds`) one field, `kept`:
  the kind of the wait it replaced, when that was `reviewer` (card waits
  need no keeping: they are recomputed). The row that ends the pause (13,
  13b or 22) restores a kept wait instead of clearing it. Row 11, when it
  ends such a wait, drops `kept` while the task is paused.
- **Who paused.** Row 12 also writes `by` into its `manual` wait: the
  member id of its sender, the secretary's or the person's. The wait that
  replaces it has none. Rows 16 and 21 write none: nobody chose them.

## 3. Transitions

"Host" means the node of the task's assignee. `SEC` is the secretary. "Admin"
is a person with role `owner` or `admin`. Items are inbox items
(`contracts/datachannel.md` section 6); the recipient is in brackets.
"Queued tasks looked at" is the rule "Starting a queued task" of section 2.

| # | Command, by | Allowed when | Task becomes | Items | Counters |
| --- | --- | --- | --- | --- | --- |
| 1 | `act confirm` on a brief, by its requester or an admin | card pending; the checks of a `brief` post (`contracts/commands.md` 1.3) still hold | per entry, in the order of the brief: `active`/`work`/`null`; or `queued`/`work` with wait `depends` if a dependency is not done, else with wait `busy` if its folder is busy | `assign` [assignee] per active task; `confirmed` [SEC] | — |
| 2 | `task start`, by a working AI | the AI is a member | new task, assignee = that AI, no reviewer, `origin` `direct`, `plan` as given, `foundation` false, `pass` = `group.pass`: `active`/`work`/`null`, or `queued`/`work`/`busy` if the AI's folder is busy; a `task` message | if `active`: `assign` [assignee] with message `re`. Always: `notice` [SEC] | — |
| 5 | `post` kind `report`, by the assignee | `active` or `paused`; step `work` or `review` | `report` = that message; step `review`; a queued `review` item for the earlier report is retired. Without a reviewer: wait `reviewer` | `review` [reviewer] with the report, or `notice` [SEC] if the task has no reviewer | `failures` = 0 |
| 7 | `post` kind `review`, by the reviewer | `active`, `review`, wait not `reviewer`; `body.report` = the task's `report` | passed: `done`, wait `null`, `stats.ended` set; pending cards of the task become `expired`; a `system` message `task_done`; queued tasks looked at. Not passed, `review.failed` ≤ `limits.review_rounds`: step `work`. Not passed, above it: row 14 | passed: `assign` [assignee] per task started; `notice` [SEC]. Not passed, within the limit: `reviewed` [assignee] | `review.round` + 1; not passed: `review.failed` + 1 |
| 10 | `task fail` (reason `crash` or `timeout`), by host | `active` | unchanged, or row 14 if the limit is reached | below the limit: `notice` [SEC] with the detail | `failures` + 1 |
| 11 | `task assign`, by SEC or an admin; from `failed`, by an admin only | not final. A new assignee for a started task whose folder is busy with another task is refused with `conflict`: a started task is never sent back to `queued` | fields set. Assignee changed: every pending approval card of the task becomes `reassigned`, with `by` = the member id of the sender, and an `approval` wait is cleared; the old assignee's queued items for the task are retired; queued tasks looked at, this one too if it is `queued`. Reviewer changed: the old reviewer's queued `review` item is retired. From `failed`: `active`, whichever field changed. Then, if the task is `active`: at step `review`, wait `reviewer` cleared when the task now has a reviewer. If it is `paused`, the same applies to `kept` | if `active`: `assign` [assignee] with the latest `review` message, when the assignee changed or the task was `failed`; `review` [reviewer], when the reviewer changed at step `review`; from `failed` at step `review`, what section 2 says about activation. `assign` [assignee] per task started | from `failed`: `failures` = 0, `review.failed` = 0 |
| 12 | `task pause`, by SEC or an admin | `active` | `paused`, wait `manual`, with `kept` and `by` (section 2) | — | — |
| 13 | `task resume`, by SEC or an admin; from `failed` or from wait `rounds`, by an admin only | `paused`, or `failed` | `active`, with the wait that `kept` names if there is one, else wait cleared; from wait `rounds`: `thread.rounds` = 0 | `resume` [assignee]; at step `review`, what section 2 says about activation | from `failed`: `failures` = 0, `review.failed` = 0 |
| 13b | `task resume`, by host | `paused` with wait `preempted` | `active`, with the wait that `kept` names if there is one, else wait cleared | — (the host continues the session itself); at step `review`, what section 2 says about activation | — |
| 14 | (inside rows 7 and 10) | `failures` reaches `limits.failures`, or `review.failed` goes above `limits.review_rounds` | `failed`, wait `null`; a `system` message `task_failed` mentioning the brief's requester and the assignee's owner | `notice` [SEC], once; no other item | — |
| 15 | `task cancel`, by SEC or an admin | not final | `cancelled`, wait `null`; pending cards of the task become `expired`; every queued item of the task is retired; queued tasks looked at | `assign` [assignee] per task started; `notice` [SEC] for each queued task that depends on it | — |
| 16 | `task preempted`, by host | `active` | `paused`, wait `preempted`, with `kept` (section 2); a `system` message `preempted` | — | — |
| 17 | `post` kind `approval`, by the assignee or the reviewer | `active` | when the wait is `null` or a card wait, it is recomputed with this card among the pending ones (section 2): wait `approval` with the task's oldest pending approval card, which is this card only if no older one is pending | — | — |
| 18 | `act approve` or `deny`, by an approver | card pending | the card is decided and keeps the signed `act`; wait cleared | `decided` [the asking AI] | — |
| 19 | `post` kind `decision` naming the task, by an AI | not final | if `active` with wait `null`: wait `decision` with the card | — | — |
| 20 | `act choose`, by an addressee or an admin | card pending | the card is decided and keeps the signed `act`; wait cleared | `decided` [the asking AI; the assignee if another] | — |
| 21 | any `post` by an AI in the thread that mentions an AI | — | `thread.rounds` + 1; at or above `limits.rounds`, if `active`: `paused`, wait `rounds`, with `kept` (section 2), a `system` message `rounds_limit` in the main line | at the limit: `notice` [SEC] | — |
| 22 | `post` kind `text` by a person in the thread | — | `thread.rounds` = 0; if wait was `rounds`: `active`, with the wait that `kept` names if there is one, else wait cleared | — | — |
| 23 | timer in the home | the group is not paused; `active`, wait `null`, no thread message for `limits.idle_notice_minutes` | unchanged | `notice` [SEC], once per quiet period | — |
| 24 | `group resume` | the group is paused; a resume of a group that is not paused is accepted and changes nothing | unchanged | `resume` [assignee] for each task that is `active` with wait `null`; `review` [reviewer] for each `active` task at step `review` that has a reviewer, unless one is queued | — |

## 4. What follows from the table

- **One submission.** The task's `report`, the `review` item and the review
  all name one `report` message. A new submission replaces it and sends the
  task through review again; a review of any other report is refused. The
  report is the assignee's own statement: the home reads no folder, and
  nothing ties the review to a commit (`risks.md` K-30).
- **Done at review.** A passed review ends the task in the transaction of
  the review: no step follows, no host acts, and nothing is merged (R-24).
  What the project still needs, such as a merge, the secretary arranges
  after its `notice`.
- **Queued behind a folder.** A task waits with `busy` only before it
  starts. A review, a question and a call are reading and are not held by
  the rule; an AI still runs one turn at a time per group (`sessions.md`).
- **A member with unfinished work cannot be removed or made secretary.**
  `member remove` and `group set` of `secretary` are refused while the AI is
  assignee or reviewer of a task that is not final; reassign or cancel
  first (`contracts/commands.md` 1.1, 1.2).
- **Six failed reviews stop a task.** With `limits.review_rounds` = 5, the
  first five failed reviews send the work back; the sixth is row 14.
- **A stopped task waits for a person.** After row 14 only an admin can
  resume or reassign it (R-63), which resets `failures` and
  `review.failed`; the secretary may cancel it. Until then it keeps its
  folder busy.
- **Failures are consecutive.** The counter grows on a crashed or timed-out
  turn, and returns to zero at a submission (row 5) and when a stopped task
  is resumed or reassigned.
- **Held by the round limit.** While a task's wait is `rounds`, `mention`
  items of its thread are not delivered: the host holds them, as for any
  paused task (section 2). However the wait ends (row 13 or 22), they go
  out again. Only a person ends it (R-60): an admin's resume or a person's
  message in the thread.
- **Pre-emption is the host's fact.** A host that interrupts a turn to run a
  queue jump in **another** session reports row 16, runs the jump turn,
  reports row 13b when it has ended, and gives the interrupted session a
  turn of its own. A jump aimed at the very session that is running just
  interrupts it and goes first; the task is not paused, and the session gets
  its turn of its own after it. If the host never reports 13b, row 13 does.
- **Paused or failed means no turns.** A host gives no turn to the sessions
  of a task that is `paused` or `failed`, and interrupts one that is running
  when the row arrives.
- **Final means gone.** When a task is `done` or `cancelled`, the hosts end
  its sessions and forget its pending approvals. Nothing is removed from a
  folder: what the task changed stays in its assignee's workspace. A task
  that depends on a cancelled task stays queued until it is cancelled too;
  the secretary is told (row 15).
- **The freed folder and the turn before.** The home frees a folder when
  the row changes, not when the old assignee's turn ends. The host closes
  the gap: the task instructions end the turn after `submit_task`
  (`contracts/drivers.md` section 4), and the first turn of a task session
  starts only after the host has closed the task sessions of this group
  whose task is final or has another assignee (`flows/agent-turn.md` step
  4). Tasks of one folder are tasks of one host, so that host sees both.

## 5. What the secretary is told

Every `notice` names its case in `code` (`contracts/datachannel.md` section
6). `task` is the task the case is about, `msg` the message that records
it, and `detail` the host's text, each where the table gives one.

| `code` | Case | `msg` | `detail` |
| --- | --- | --- | --- |
| `task_started` | a direct task, started or queued (row 2) | the `task` message | — |
| `no_reviewer` | a submission for a task that has no reviewer (row 5) | — | — |
| `task_done` | a passed review (row 7) | the `system` message `task_done` | — |
| `crash`, `timeout` | a turn of the task session crashed or timed out, below the limit (row 10) | — | the `detail` of `task fail` |
| `task_failed` | a stopped task (row 14) | the `system` message `task_failed` | — |
| `dependency_cancelled` | a cancel left this queued task waiting for a dependency that will never be done (row 15) | — | — |
| `rounds_limit` | the round limit (row 21) | the `system` message `rounds_limit` | — |
| `quiet` | a quiet task (row 23) | — | — |
| `handback` | a hand-back account posted in a thread (`contracts/commands.md` 1.3) | the `handback` message | — |
| `confirm_denied` | an owner denied a `confirm` card (`contracts/step-2/confirm.md` sections 5 and 6) | the `confirm` card | — |
| `login_invalid`, `cap_reached` (postponed after 1.0 with `settings.set`), `rate_limited` | an AI is limited (`contracts/commands.md` 1.9) | the `system` message of that event | — |

What the secretary does next (ask, reassign, cancel, arrange the closing
step, bring it to a person) is its judgement, not the platform's (R-22).
