# Flow: a task from request to merge

Contracts: `contracts/commands.md`, `contracts/rows.md`,
`contracts/mcp-tools.md`, `contracts/local.md` (workspace layout, check and
merge commands). Rules: R-20 to R-24, R-90 to R-92, R-100 to R-103, R-63.

Actors: P, the person asking; SEC, the secretary AI; H, the home; A, the
assignee AI on host NA; R, the reviewer AI on host NR.

The six steps shown on a task card are `branch`, `local_check`, `pr`,
`cloud_check`, `review`, `merge`. The home moves a task from step to step
only on facts reported by hosts. No AI can set a step.

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

6. NA takes the `assign` item. Before delivering it, NA prepares the
   workspace: fetch, create the worktree and branch for the task from the
   base branch, and report `task branch`. The step becomes `local_check`.
7. NA delivers the turn: the brief, the criteria, the worktree folder, the
   branch.
8. A works in the worktree and reports with `post` (kind `progress`). With a
   remote, A pushes as it goes; at level 2 a push asks for approval
   (`flows/approval.md`).
9. A calls `run_checks`. NA runs, in the worktree, every command in
   `group.checks` and then the universal hard checks (`contracts/local.md`
   section 7). NA reports `task checked` with the result and the commit it
   ran on; H writes a `checks` message. The tool returns pass, or the tail
   of the failing output. A fixes and repeats.
10. With a remote, A opens a pull request.
11. A calls `submit_task` with a summary and the pull-request number. NA
    refuses unless the last `run_checks` passed on the current head commit
    and, with a remote, the pull request's head is that commit. Then NA
    sends `post` (kind `report`) and `task submitted`.

## C. Hosted checks

12. With `forge: github`, the step is `cloud_check`. NA polls the pull
    request's checks every 30 seconds until all have finished and reports
    `task cloud`. A repository without hosted checks passes at once.
13. With `forge: none` this step and the pull request are skipped.

## D. Review

14. The step is `review`. H queues `review` for R. If the task has no
    reviewer, H sets `wait.kind = reviewer` and gives SEC a `notice`; SEC
    assigns one.
15. NR prepares a read-only checkout of the submitted commit and delivers
    the turn: brief, criteria, pass score, how to see the diff.
16. R calls `submit_review` with a score from 0 to 100 and a list of
    remarks, each optionally tied to a file and line, each optionally a
    veto. H computes the outcome (`contracts/commands.md` 1.8).
17. Not passed: the round counter increases, the step returns to
    `local_check`, A gets the review. Continue at 8.
18. Passed: the step is `merge`. SEC gets the review.

## E. Merge and summary

19. SEC calls `merge_task`. H accepts only at step `merge` and queues a
    `merge` item for NA.
20. NA merges: with a remote, through the forge (squash, delete the branch);
    without, a squash merge into the base branch of the local clone. NA
    removes the worktree and reports `task merged`.
21. H marks the task `merged`, activates tasks that depended on it, and
    notifies SEC.
22. SEC calls `post_summary` with one or two sentences. H adds score,
    rounds, duration, lines changed and the pull-request number, and the
    summary card appears in the main line.

## F. Failures

| What happens | Result |
| --- | --- |
| `run_checks` fails | returned to A in the same turn; not counted |
| hosted checks fail | `task fail`; `failures` + 1; A gets the detail |
| a turn crashes or times out during the task | `task fail`; `failures` + 1 |
| merge conflict | `task fail` with reason `conflict`; step returns to `local_check`; A gets the detail and SEC a `notice` |
| `failures` reaches 3 | task `failed`; `system` message mentions P and A's owner (R-63) |
| review round would exceed 5 | task `failed`, reason `review_rounds` |
| approval not answered | task waits; SEC may reassign (R-42) |

A `failed` task moves again only when a person or SEC sends `task resume`
(counter back to zero), `task assign`, or `task cancel`.

## G. Direct work (R-70, R-71)

- The owner mentions their own AI in the main line. The item reaches the AI
  without the secretary.
- A question that changes no code is answered with `post`; nothing else
  happens.
- If the AI is about to change code and has no task for it, it calls
  `start_task`. NA sends `task start`; H creates the task card and notifies
  SEC; NA prepares the worktree as in step 6. The flow continues at 8. SEC
  assigns a reviewer when the task reaches `review`.

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
