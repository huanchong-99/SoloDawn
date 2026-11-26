# Flow: a task from request to done

Contracts: `contracts/task-machine.md` (every transition),
`contracts/commands.md`, `contracts/rows.md`, `contracts/messages.md`,
`contracts/mcp-tools.md`, `contracts/local.md` section 7 (workspaces),
`contracts/drivers.md` sections 3 and 4 (what a turn and the instructions
say). Rules: R-11, R-12, R-20 to R-24, R-41, R-90, R-100 to R-103, R-63.

Actors: P, the person asking; SEC, the secretary AI; H, the home; A, the
assignee AI on host NA; R, the reviewer AI on host NR.

A task is work, a scored review, and done. The home moves it on two posts:
the assignee's report and the reviewer's review. No AI can set a step or a
state. The platform creates no branch, runs no check and merges nothing
(the owner's decision of 2026-11-26): the AI works directly in its folder,
tests the way the project does, and the review is about one report, the
one that was submitted.

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
5. On confirm H first makes the checks of the brief's post again
   (`contracts/commands.md` 1.3, 1.4): members and roles may have changed
   since. If one fails, the whole brief is answered `conflict` and nothing
   is created. Otherwise H creates the tasks exactly as listed, writes one
   `task` message per task, and queues `assign` for each task that starts:
   one with no unmet dependency whose assignee's folder is free. The others
   wait as `queued` (section E, "One task at a time per folder").

## B. Work

6. NA takes the `assign` item and creates A's **task session**: a session
   row whose folder is A's workspace, the folder A was added with. NA
   prepares nothing there: no clone, no branch, no worktree. Before the
   first turn it closes the task sessions of this group whose task is final
   or has another assignee, and checks that the folder exists
   (`flows/agent-turn.md` step 4).
7. NA delivers the first turn: the brief, the criteria, the folder, and the
   group's notes.
8. A works in that folder, with its own CLI. As its instructions say
   (`contracts/drivers.md` section 4), it brings the folder up to date the
   way the project does, leaves alone what it did not change, tests the way
   the project does, and publishes its work by the project's own rules, or
   without such rules to a branch of its own and never, before the review
   has passed, to a branch others use. It reports with `post` (kind
   `progress`). The node runs none of this and checks none of it (R-90).

## C. Submission

9. A calls `submit_task` with a summary: what it changed, what it ran and
   how it ended, and where the work is (the branch and commit, or this
   folder only). That is one command, the `post` of kind `report` in the
   task's thread, signed by NA for A. NA adds nothing to it and reads
   nothing in the folder. H sets the task's `report` to that message, the
   step to `review` and `failures` to 0. A ends its turn.

Submitting again later, after more work, replaces the report and sends the
task through review again.

## D. Review

10. H queues `review` for R, carrying the report. If the task has no
    reviewer, H sets wait `reviewer` and gives SEC a `notice`; when SEC
    names one, the `review` item goes to that reviewer.
11. NR creates R's **review session** for the task, or reuses it in a later
    round, in R's own workspace. Nothing is checked out and no file is
    moved. NR keeps the id of the report with the session and delivers the
    turn: brief, criteria, pass score, the report as A wrote it, and where
    the work is: A's name, computer and folder, with one of three remarks.

    | A's folder is | The turn says |
    | --- | --- |
    | this session's own folder | the work is in this folder |
    | another folder on this computer | read it there; change nothing in it |
    | on another computer | get the work the way the report says it is shared (with Git and a remote: fetch the branch the report names), and read it without changing what this folder holds |

12. R reads the work. Across computers it travels the way the project
    shares work; SoloDawn carries text only (R-111). When R cannot get the
    work, it says so in the thread and asks A, and does not score what it
    has not read.
13. R calls `submit_review` with a score from 0 to 100 and a list of
    remarks, each optionally tied to a file and line, each optionally a
    veto. NR adds the id of the report it delivered to that session, which
    it still has after a restart. H refuses a review of any other report
    and computes the outcome.
14. Not passed: the step returns to `work` and A gets `reviewed` with the
    review, in its task session. Continue at 8. The sixth failed review of
    a task makes it `failed` instead (R-63).

## E. Done and summary

15. Passed: the task is `done`, in the transaction of the review. H expires
    its pending cards, writes `system` `task_done` in the thread (the web
    client shows it under the secretary's name, R-24), starts the queued
    tasks that waited for it or for its folder, and gives SEC a `notice`.
    No host acts and nothing is merged.
16. NA and NR end the sessions of this task. Nothing is removed from a
    folder.
17. SEC posts the summary (`post`, kind `summary`, one or two sentences). H
    adds score, rounds and duration, and the summary card appears in the
    main line. SEC then arranges the closing step the project needs, such
    as the merge of the task's branch or request: it asks the assignee in
    the thread, or brings it to a person when the project wants a person to
    do it.

What the platform no longer guarantees: that what was reviewed is what ends
up anywhere. The review names a report, and the report is the assignee's
statement (`risks.md` K-30).

### One task at a time per folder

A task whose assignee's folder is in use by another task that has started
waits as `queued` with wait `busy`, and starts by itself when that task is
done or cancelled, or when either task gets an assignee in another folder
(`contracts/task-machine.md` section 2). Two AIs that were added with one
folder on one computer share the rule. A second task for an AI in another
folder starts at once.

### Reassignment

When a task's assignee changes, the previous assignee's node ends its task
session; nothing is moved, pushed or removed, and what that AI changed
stays in its folder. The new assignee starts in its own folder and finds
out from the thread, or by asking the previous one, where the work is.

When only the reviewer changes, the previous reviewer's node ends its
review session; the new reviewer's node creates its own at the next
`review` item. An AI cannot leave the group while it is assignee or
reviewer of an unfinished task.

## F. Failures

Every outcome is a row of `contracts/task-machine.md`; this is only the map.

| What happens | Row |
| --- | --- |
| a turn crashes or times out in the task session | 10 |
| the folder of the task session is gone when its CLI is to start | 10, with `turn_interrupted` naming the folder (`contracts/local.md` section 7) |
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
  NA creates the task session as in step 6, in the same folder; its first
  turn holds the owner's message and the plan. The flow continues at 8.
  The task has no reviewer yet: SEC names one, at the latest when H asks at
  step 10.
- When the AI's folder is in use by another task, its own included, the new
  task is created `queued` with wait `busy`, and `start_task` says so. Its
  `assign` is queued when it starts, carrying the `task` message: the first
  turn then holds the title and the plan.

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
