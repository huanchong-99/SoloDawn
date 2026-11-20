# Flow: a task from request to merge

Contracts: `contracts/task-machine.md` (every transition),
`contracts/commands.md`, `contracts/rows.md`, `contracts/messages.md`,
`contracts/mcp-tools.md`, `contracts/local.md` section 7 (Git commands).
Rules: R-12, R-20 to R-24, R-41, R-90 to R-92, R-100 to R-103, R-63.

Actors: P, the person asking; SEC, the secretary AI; H, the home; A, the
assignee AI on host NA; R, the reviewer AI on host NR.

The home moves a task only on facts reported by hosts. No AI can set a step,
and no AI performs the merge. Hosted checks, the review and the merge are
always about one commit: the one that was submitted.

## A. Brief

1. P writes in the main line. A main-line message from a person that
   mentions no AI goes to the secretary.
2. SEC asks only if something is unclear (`post`, kind `question`, in the
   main line); P answers.
3. SEC calls `propose_brief`: title, goal, acceptance criteria, scope in and
   out, the split into tasks with assignee and reviewer for each, pass
   score. H stores it as a `brief` card, pending.
4. P taps "confirm" (`act confirm`). Nothing has started before this (R-23).
   "Change it" (`act revise`, with text) sends it back to SEC, who posts a
   new brief with `replaces`; H marks the old one `superseded`.
5. On confirm H creates the tasks exactly as listed, refusing a task whose
   reviewer is its assignee, writes one `task` message per task, and queues
   `assign` for each task with no unmet dependency. The item carries P's
   signed confirmation as its `cause`.

## B. Work

6. NA takes the `assign` item and creates A's **task session**: fetch (with
   a remote), create the worktree and branch for the task from the base
   branch, record the session with that worktree as its folder. NA reports
   `task branch`. The step becomes `local_check`.
7. NA delivers the first turn: the brief, the criteria, the folder, the
   branch, and the group's notes.
8. A works in the worktree and reports with `post` (kind `progress`). To
   push while working it calls `push_branch`: NA pushes the task's own
   branch, never forced. No approval is involved (R-41). A `git push` that A
   runs by itself is like any other command that needs the network: at
   level 2 the CLI asks, and a card appears (`flows/approval.md`).
9. A calls `run_checks`. NA refuses if the worktree has uncommitted changes:
   checks are about a commit. Otherwise NA runs, in the worktree, every
   command in `group.checks` and then the universal hard checks, and reports
   `task checked` with the result and the head commit; H writes a `checks`
   message. The tool returns pass, or the tail of the failing output. A
   fixes, commits and repeats.
10. A calls `submit_task` with a summary. NA refuses unless the worktree is
    clean and the last `run_checks` passed on the head commit. With a
    remote, NA then pushes the branch and opens the pull request if the
    branch has none (title: the task title; body: the summary). NA sends
    `post` (kind `report`) and `task submitted` with that commit and the
    pull-request number. That commit is now the task's `commit`.

Submitting again later, after more work, replaces the commit and sends the
task through hosted checks and review again.

## C. Hosted checks

11. With `forge: github`, the step is `cloud_check`. Every 30 seconds NA
    reads, in one request, the pull request's head commit and the state of
    its checks. It reports `task cloud` only from an answer whose head is
    the submitted commit and whose checks have all finished. A repository
    without hosted checks passes at once. A failure returns the task to
    `local_check`, counts as one failure, and A gets the detail.
12. With `forge: none` this step and the pull request are skipped.

## D. Review

13. The step is `review`. H queues `review` for R, naming the commit. If the
    task has no reviewer, H sets wait `reviewer` and gives SEC a `notice`;
    when SEC names one, the `review` item goes to that reviewer.
14. NR creates R's **review session** for the task, or reuses it in a later
    round: a read-only checkout of exactly that commit, with the diff
    against the base branch written to a file in that folder. NR delivers
    the turn: brief, criteria, pass score, the name of the diff file.
15. R calls `submit_review` with a score from 0 to 100 and a list of
    remarks, each optionally tied to a file and line, each optionally a
    veto. NR adds the commit it checked out. H refuses a review of any
    other commit and computes the outcome.
16. Not passed: the step returns to `local_check` and A gets `reviewed` with
    the review, in its task session. Continue at 8. The sixth failed review
    of a task makes it `failed` instead (R-63).
17. Passed: the step is `merge`, and H queues a `merge` item for NA, naming
    the commit and the pull request. From here until NA reports, nothing
    else changes the task: no pause, no reassignment, no new submission
    (`contracts/task-machine.md` section 4).

## E. Merge and summary

18. NA takes the item only if the task, as NA sees it now, is still at
    `merge` for that commit. It merges exactly that commit: with a remote,
    through the forge (squash, delete the branch), told to refuse if the
    pull request's head is not that commit; without, only if the task
    branch's head is that commit, by a squash merge into the base branch of
    the local clone. NA reports `task merged`. There is no retry and no
    rollback.
19. H marks the task `merged`, writes `system` `task_merged` in the thread
    (the web client shows it under the secretary's name, R-24), activates
    tasks that depended on it, and gives SEC a `notice`.
20. NA and NR end the sessions of this task and remove the worktree and the
    review checkout.
21. SEC posts the summary (`post`, kind `summary`, one or two sentences). H
    adds score, rounds, duration, lines changed and the pull-request number,
    and the summary card appears in the main line.

### Conflict

22. If the merge does not apply, or the branch has moved past the reviewed
    commit, NA reports `task conflict` with the detail. H writes a `checks`
    message (stage `merge`, failed), sets step `local_check` and wait
    `conflict`, and gives SEC a `notice`.
23. SEC says which AI resolves it (`set_task`): the same assignee or another
    one. H clears the wait and queues `assign`, carrying that `checks`
    message. The AI brings the branch up to date with the base branch,
    resolves, and the flow continues at 9.

### Reassignment

When a task's assignee changes, the previous assignee's node, with a remote,
first pushes the task branch, then ends its task session and removes its
worktree. The new assignee's node creates its worktree from the published
branch, or from the base branch if nothing was ever published; without a
remote both AIs are on the same node and use the same local branch.
Committed work carries over; uncommitted changes do not. The new node
reports `task branch` only if the task is still at step `branch`; a task
that is further along keeps its step.

A task cannot be reassigned, paused or resubmitted while it is at step
`merge`, and an AI cannot leave the group while it is assignee or reviewer
of an unfinished task.

## F. Failures

Every outcome is a row of `contracts/task-machine.md`; this is only the map.

| What happens | Row |
| --- | --- |
| `run_checks` fails | none: returned to A in the same turn, not counted |
| hosted checks fail | 6 |
| a turn crashes or times out in the task session | 10 |
| the merge does not apply, or the branch has moved | 9 |
| three consecutive failures; a sixth failed review | 14: only a person resumes or reassigns the task (R-63) |
| approval not answered | 17: the task waits; SEC may reassign (row 11, R-42) |

## G. Direct work (R-70, R-71)

- The owner mentions their own AI in the main line. The item reaches the
  AI's group session without the secretary.
- A question that changes no code is answered there with `post`; nothing
  else happens. The group session is read only.
- If the request needs a code change, the AI calls `start_task` with a
  title and a short plan. NA sends `task start`; H creates the task card,
  gives SEC a `notice`, and queues `assign` carrying the owner's message.
  NA creates the task session as in step 6; its first turn holds the
  owner's message and the plan. The flow continues at 8. The task has no
  reviewer yet: SEC names one, at the latest when H asks at step 13.

## H. Follow-up by the secretary (R-22)

SEC is woken by events (`contracts/task-machine.md` section 5), and by one
timer in the home: an active task that waits for nothing and has had no
thread message for `limits.idle_notice_minutes` (default 30) produces one
`notice` for SEC. That is also what recovers a turn lost on the way to a
session (K-3). What SEC then does is its judgement: ask in the thread,
reassign, or bring it to a person.

## I. What a new task session knows

Only what the group holds: the brief and criteria, the thread, the notes,
and the ledger (earlier `review` messages). The secretary is instructed to
turn lessons from reviews into notes, because notes are what every new task
session is given (R-102).
