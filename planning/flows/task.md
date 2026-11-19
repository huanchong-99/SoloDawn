# Flow: a task from request to merge

Contracts: `contracts/commands.md`, `contracts/rows.md`,
`contracts/mcp-tools.md`, `contracts/local.md` (folders, check, push and
merge commands). Rules: R-12, R-20 to R-24, R-41, R-90 to R-92, R-100 to
R-103, R-63.

Actors: P, the person asking; SEC, the secretary AI; H, the home; A, the
assignee AI on host NA; R, the reviewer AI on host NR.

The six steps shown on a task card are `branch`, `local_check`, `pr`,
`cloud_check`, `review`, `merge`. The home moves a task from step to step
only on facts reported by hosts. No AI can set a step, and no AI performs
the merge.

## A. Brief

1. P writes in the main line. A main-line message from a person that
   mentions no AI goes to the secretary.
2. SEC asks only if something is unclear (`post`, kind `question` or
   `text`); P answers.
3. SEC calls `propose_brief`: title, goal, acceptance criteria, scope in and
   out, the split into tasks with assignee and reviewer for each, pass
   score. H stores it as a `brief` card, pending.
4. P taps "confirm" (`act confirm`). Nothing has started before this (R-23).
   "Change it" (`act revise`, with text) sends it back to SEC, who posts a
   new brief with `replaces`; H marks the old one `superseded`.
5. On confirm H creates the tasks exactly as listed, writes one `task`
   message per task, and queues `assign` for each task with no unmet
   dependency.

## B. Work

6. NA takes the `assign` item and creates A's **task session**: fetch,
   create the worktree and branch for the task from the base branch, record
   the session with that worktree as its folder. NA reports `task branch`.
   The step becomes `local_check`.
7. NA delivers the first turn: the brief, the criteria, the folder, the
   branch, and the group's notes.
8. A works in the worktree and reports with `post` (kind `progress`). To
   push while working it calls `push_branch`: NA pushes the task's own
   branch, never forced. No approval is involved (R-41). A `git push` that A
   runs by itself is like any other command that needs the network: at
   level 2 the CLI asks, and a card appears (`flows/approval.md`).
9. A calls `run_checks`. NA runs, in the worktree, every command in
   `group.checks` and then the universal hard checks (`contracts/local.md`
   section 7). NA reports `task checked` with the result and the commit it
   ran on; H writes a `checks` message. The tool returns pass, or the tail
   of the failing output. A fixes and repeats.
10. A calls `submit_task` with a summary. NA refuses unless the last
    `run_checks` passed on the current head commit. With a remote, NA then
    pushes the branch and opens the pull request if the branch has none
    (title: the task title; body: the summary). NA sends `post` (kind
    `report`) and `task submitted` with the pull-request number.

## C. Hosted checks

11. With `forge: github`, the step is `cloud_check`. NA polls the pull
    request's checks every 30 seconds until all have finished and reports
    `task cloud`. A repository without hosted checks passes at once.
12. With `forge: none` this step and the pull request are skipped.

## D. Review

13. The step is `review`. H queues `review` for R. If the task has no
    reviewer, H sets `wait.kind = reviewer` and gives SEC a `notice`; SEC
    assigns one.
14. NR creates R's **review session** for the task, or reuses it in a later
    round: a read-only checkout of the submitted commit, with the diff
    against the base branch written to a file in that folder. NR delivers
    the turn: brief, criteria, pass score, the name of the diff file.
15. R calls `submit_review` with a score from 0 to 100 and a list of
    remarks, each optionally tied to a file and line, each optionally a
    veto. H computes the outcome (`contracts/commands.md` 1.8).
16. Not passed: the round counter increases, the step returns to
    `local_check`, A gets the review in its task session. Continue at 8.
17. Passed: the step is `merge`, and H queues a `merge` item for NA.

## E. Merge and summary

18. NA merges: with a remote, through the forge (squash, delete the branch);
    without, a squash merge into the base branch of the local clone. NA
    reports `task merged`.
19. H marks the task `merged`, writes `system` `task_merged` in the thread
    (the web client shows it under the secretary's name, R-24), activates
    tasks that depended on it, and gives SEC a `notice`.
20. NA and NR end the sessions of this task and remove the worktree and the
    review checkout.
21. SEC calls `post_summary` with one or two sentences. H adds score,
    rounds, duration, lines changed and the pull-request number, and the
    summary card appears in the main line.

### Conflict

22. If the merge does not apply, NA reports `task conflict` with the detail.
    H writes a `checks` message (stage `merge`, failed), sets
    `step = local_check` and `wait.kind = conflict`, and gives SEC a
    `notice`.
23. SEC assigns an AI to resolve it (`set_task`): the same assignee or
    another one. H clears the wait and queues `assign`, carrying that
    `checks` message. The AI brings the branch up to date with the base
    branch, resolves, and the flow continues at 9: checks, submit, hosted
    checks, review, merge.

### Reassignment

When a task's assignee changes, the previous assignee's node ends its task
session and removes its worktree. The new assignee's node creates its own
from the task branch. What was committed (and, with a remote, pushed) carries
over; uncommitted changes do not.

## F. Failures

| What happens | Result |
| --- | --- |
| `run_checks` fails | returned to A in the same turn; not counted |
| hosted checks fail | `task fail`; `failures` + 1; A gets the detail |
| a turn crashes or times out in the task session | `task fail`; `failures` + 1 |
| merge conflict | section E, "Conflict"; not counted as a failure |
| `failures` reaches 3 | task `failed`; `system` message mentions P and A's owner (R-63) |
| review round would exceed 5 | task `failed`, reason `review_rounds` |
| approval not answered | task waits; SEC may reassign (R-42) |

A `failed` task moves again only when a person or SEC sends `task resume`
(counter back to zero), `task assign`, or `task cancel`.

## G. Direct work (R-70, R-71)

- The owner mentions their own AI in the main line. The item reaches the
  AI's group session without the secretary.
- A question that changes no code is answered there with `post`; nothing
  else happens. The group session is read only.
- If the request needs a code change, the AI calls `start_task` with a
  title and a short plan. NA sends `task start`; H creates the task card,
  gives SEC a `notice`, and queues `assign` carrying the owner's message.
  NA creates the task session as in step 6; its first turn holds the
  owner's message and the plan. The flow continues at 8. SEC assigns a
  reviewer when the task reaches `review`.

## H. Follow-up by the secretary (R-22)

The home does not chase anyone. SEC is woken by the items listed in
`contracts/commands.md` section 3. To notice silence, SEC calls
`list_tasks`, which shows for each task the time since its last thread
message, and asks in the thread when that is too long. How often SEC looks
is part of its instructions, not of the platform.

`[unverified]` Whether a secretary that is only woken by events notices a
silent assignee soon enough. If not, the smallest addition is one timer in
the home that queues a `notice` for SEC when an active task has had no
thread message for N minutes. See `risks.md` T-12.

## I. What a new task session knows

Only what the group holds: the brief and criteria, the thread, the notes,
and the ledger (earlier `review` messages). The secretary is instructed to
turn lessons from reviews into notes, because notes are what every new task
session is given (R-102).
