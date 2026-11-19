# Round 2: answers

Answers to `reviews/gpt-6.2-round-2.md` (reviewed commit `39be475`).
Decided points Q-1 to Q-20 were not reopened and are unchanged.

Summary: eight findings; six accepted, two accepted with changes, none
rejected. Again the review proposed nothing that adds defence inside the
system, so nothing was turned down on that ground. Where a different fix
was chosen than the one proposed, it is the one that removes more.

The review's verdict is taken as it stands: the scaffold and the spikes can
start; the confirmed-task vector is frozen only after round 3.

## Findings

### F-1
Decision: accepted with changes
Change: A queued or paused task gets no item; it gets the item for its
current step when it becomes active. Naming the same assignee after a
conflict clears the wait. Assignee and reviewer must be working AIs, never
the secretary, and differ; `depends` may name only earlier entries of the
same brief, so a missing or cyclic dependency cannot be written. A node
reports `task branch` only for a task still at step `branch`. The change
from the proposal is in the last case of the finding: instead of repairing
what a removed member leaves behind, `member remove` is refused while the
AI is assignee or reviewer of an unfinished task. That deleted the
transition and the wait kind `assignee`. It is a choice, so it is listed as
Q-21. A task that depends on a cancelled task stays queued until cancelled.
Files: `contracts/task-machine.md` 2, rows 1, 11, and section 4;
`contracts/commands.md` 1.2, 1.3; `contracts/rows.md` 4 and brief body;
`flows/task.md` "Reassignment"; acceptance A5.6, A5.18.

### F-2
Decision: accepted with changes
Change: Entry to step `merge` is the point of no return, as proposed: until
the host reports, the task accepts only `merged` and `conflict` for the same
commit. Pause, reassignment, a new submission, the round limit and removal
of the member are refused. Both reports carry the commit. Hosted checks are
read together with the head in one answer, and only an answer whose head is
the submitted commit is reported. Without a remote the node compares the
branch head with the commit before merging. No retry and no rollback; a node
that restarts in the middle of a merge looks at what happened and reports
it. One addition to the proposal: a person, and only a person, may still
cancel at step `merge`, because otherwise a host that never returns leaves
the task stuck for good. The host checks that the task is still at `merge`
for that commit before it acts, so the cancel is safe except in the seconds
of the merge itself; that window is stated (K-14) and the choice is listed
as Q-22. Files: `contracts/task-machine.md` 2, rows 5 to 12, 15, 16, 21,
23, and section 4; `contracts/commands.md` 1.5; `contracts/local.md` 7;
`flows/task.md` steps 11, 17, 18; `flows/trace-task.md`; `risks.md` K-14;
acceptance A5.3, A5.18.

### F-3
Decision: accepted
Change: The wait reason chooses the exit. "Wait cleared" now means
recomputed from the task's pending cards, so a pause and a resume cannot
lose an open approval. While the wait is `rounds`, mentions of the thread
are simply not sent; whichever way the wait ends, they go out. A jump aimed
at the session that is running interrupts it and goes first without pausing
its task; only a jump for another session pre-empts. The separate
`continued` verb is gone: the host sends `task resume` for its own
pre-empted task, with no inbox item, and an ordinary resume still works if
the host never does. After three failures or a sixth failed review only a
person may resume or reassign the task (R-63); the secretary may cancel it.
At the failure limit one `notice` is sent and no work item. Files:
`contracts/task-machine.md` 2, rows 6, 10, 11, 13, 13b, 14, 16, 21, 22;
`contracts/commands.md` 1.5; `flows/agent-turn.md` C; acceptance A5.5, A5.8,
A5.11.

### F-4
Decision: accepted
Change: An item's envelope is built and signed in the transaction that
queues it, and stored; what is sent later is what was decided then. The
`merge` item names the pull request as well as the commit. The home deletes
queued items that a later change made pointless (another assignee, reviewer
or commit; a cancelled task). A host, before acting on an item about a task,
checks the task as it holds it now: not final, the AI still in that role,
and for `review` and `merge` the same commit; otherwise it refuses the item.
A decided card keeps its signed `act`, which is the `cause` of the items
that follow, including the `assign` of a dependent task that becomes active
later, and `confirmed` for both confirm and revise. Direct work has no
cause: it starts from the owner's own message. A signed choice on a
decision card is information, never a tool grant. The signer text in the
ask flow is corrected. Files: `contracts/datachannel.md` 6;
`contracts/storage.md` (`inbox.env`, `inbox.commit_id`);
`contracts/commands.md` 1.4, 3; `contracts/task-machine.md` 2;
`flows/agent-turn.md` B.2; `flows/ask.md` E; `flows/trace-task.md`;
acceptance A5.19.

### F-5
Decision: accepted
Change: The cut is a read view: the home takes S together with a snapshot of
the database at S and sends the catch-up from that snapshot, so a later
write can neither hide nor replace a row in it. A deletion marker stays in
the client's cache in place of the row, so an older page that arrives later
cannot bring the row back. The client's cursor is defined: 0 until the first
`synced`, S at `synced`, then the `seq` of each live row; pages never move
it. Files: `flows/sync.md` A and G; `contracts/datachannel.md` 4;
`contracts/storage.md` 1; `contracts/rows.md` (`gone`); acceptance A2.7.

### F-6
Decision: accepted
Change: The daily run never deletes a card that is still pending. Files:
`contracts/storage.md` 4; `flows/sync.md` G; acceptance A2.5.

### F-7
Decision: accepted
Change: A receipt stores the outcome, without the request id and without
any secret; a repeat is answered under the id of the new request. For
`pair.browser` and `takeover.start` the receipt records only success: a
repeat gets the original link while it is valid, or `ok` while that
terminal is open on this connection, otherwise `conflict`; nothing is
created by a repeat. Files: `contracts/commands.md` ("Once only");
`contracts/datachannel.md` 5; `contracts/storage.md` 2; acceptance A1.9,
A7.6.

### F-8
Decision: accepted
Change: Adoption requires, on the pairing connection, an `add` for the local
key signed by the device at the other end, with both devices valid at the
end of the log; owner and log are stored together, and only by a device
that belongs to nobody. Pairing between devices of two people changes
neither log. The text says that pairing trusts the holder of the secret,
not a verified person. A client sends `hello` again once it is enrolled and
reads `owner: true` from the new `welcome`. Files: `contracts/envelope.md`
6; `contracts/datachannel.md` 3; `flows/pairing.md` A.10, C.4; acceptance
A1.1.

## The review's other sections

| Section | Decision | What changed |
| --- | --- | --- |
| 1, "Other rows" | accepted | row 2 names one principal, the working AI; one outcome at the failure limit; every pending approval of the old assignee becomes `reassigned` |
| 2, "Cause and trace" | accepted | see F-4 |
| 6, levels | accepted | spike S-4 lists outcomes to observe per CLI and per level, not only flags; the tool list fixture has a level-1 and a level-2 task session |
| 7, project checks | accepted | Q-19 unchanged; a toolchain that needs a prepared environment is initialised by the project's own check command; the step 2 experiment measures compatibility before overhead and reports Windows separately |
| 8, order | accepted | captured CLI messages feed B-13; B-09 depends on B-07; the exactly-once part of A1.3 became A2.7 under B-12; the smallest `task cancel` moved into B-16; scheduling split from checks and review (new B-27); the no-remote half of A5.1 moved to B-21 |
| 8, slice (c) | accepted | `flows/trace-task.md` is documentation; the golden vector is produced in B-01 slice (c), complete, with and without a remote, after round 3 |
| 9, deletion | accepted | `continued` deleted; the outcome table in `flows/task.md` F replaced by row references; the trace will link to the golden vector once it exists |

## Open points raised by this round

- **Q-21** (from F-1): an AI cannot leave a group while it holds an
  unfinished task.
- **Q-22** (from F-2): nothing changes a task between a passed review and
  the host's report, except a person's cancel.

Both are written into the plan as described. The coordinator accepted both
after this file was first written; see `risks.md`.

## For round 3

Round 3 is meant to be the last: a check that the fixes above hold, not a
new survey. In order:

1. `contracts/task-machine.md` sections 2 and 3: does any command still meet
   a state the table does not cover, and is step `merge` now closed?
2. `contracts/datachannel.md` 6 with `contracts/storage.md` (`inbox`): the
   stored, signed item and the host's "still current" check.
3. `flows/sync.md` A: the read view, the cursor, the deletion marker.
4. `contracts/envelope.md` 6: the adoption predicate.
5. `contracts/commands.md` "Once only": receipts for the two live results.
6. Whether anything stands in the way of freezing slices (a) and (b) of the
   golden examples now, and slice (c) when the backend produces it.
