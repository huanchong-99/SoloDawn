# Round 3: answers, and the plan as it now stands

Answers to `reviews/gpt-6.2-round-3.md` (reviewed commit `5994ba9`, whose
planning tree is that of `bf4dae9`). Its verdict is **ready to start**: five
findings, all marked "fix in the task PR", none that must be fixed before
code, and none that needs a new wire message or a coordinator decision.

All five are accepted. For each, the contract now states the right
behaviour in one or two sentences, and the task that implements it carries
the instruction and the test (`steps/step-1-backend.md`, "Fix in the task
(round 3)"). Nothing was redesigned.

## Findings, and the task each belongs to

### F-1 Deciding a card can erase the reason a task is paused
Decision: accepted, fix in the task
Tasks: **B-17** (approval cards), **B-19** (decision cards, pause and
resume); tested in **B-27**, where pre-emption exists.
What to do: deciding a card of a paused task changes the card and keeps its
signed `act`; the task's paused wait is left alone; card waits are
recomputed when the task becomes active.
Contract: `contracts/task-machine.md` section 2, "Wait cleared means
recomputed".
Test: decide the last pending card of a pre-empted task; the host then
resumes it by row 13b and the task is active with no wait.

### F-2 A reviewer named while paused never gets the review
Decision: accepted, fix in the task
Tasks: **B-19** (the activation rule, with `task assign` and `task
resume`); tested in **B-20**, where review items exist.
What to do: when a task becomes active at step `review`, queue `review` for
the current reviewer and commit unless one is already queued or was
delivered; with no reviewer, set wait `reviewer` again.
Contract: `contracts/task-machine.md` section 2, "Items go to active tasks
only", and row 13.
Test: pause a task that waits for a reviewer, name one, resume; the
reviewer gets the item without a second reassignment.

### F-3 Check held work when it is taken, not when it first arrives
Decision: accepted, fix in the task
Task: **B-15** (inbox handling on the host and item order on the home);
two of its tests land in **B-21**.
What to do: verify the signature on arrival; decide whether an item is
still true at the moment it is used, after the state read on that
connection: active task, same role, and for `review` and `merge` the same
step and commit. A `notice` is information for the secretary and is always
delivered. The home pushes a row change before the items it produces. No
extra round trip.
Contract: `contracts/datachannel.md` section 6, "Still current";
`flows/agent-turn.md` B.2 and B.6.
Tests: a `review` held for one commit is refused after a resubmission
(A5.19); a host that starts cold takes an item that is current; the
secretary receives the `notice` of a merged task (A5.1).

### F-4 Retention can delete the cause before a dependent task starts
Decision: accepted, fix in the task
Task: **B-19**.
What to do: the daily run keeps a confirmed brief while a task made from it
is still queued, so that the task's first `assign` can carry the
confirmation.
Contract: `contracts/storage.md` section 4; `contracts/commands.md` 1.4.
Test: advance the clock past the retention period, run the daily job, merge
the dependency; the dependent's `assign` carries the original bytes of the
confirmation.

### F-5 Clear the old cursor when a reset discards its cache
Decision: accepted, fix in the task
Tasks: **F-06** (web client); **B-15** for the host's own reads.
What to do: on every `reset` the cursor goes to 0 together with the cache,
and becomes S only at the `synced` that follows. An interrupted delta keeps
the cursor it had.
Contract: `flows/sync.md` A; `contracts/datachannel.md` section 4.
Test: lose the connection after the first batch of a reset; the next `sync`
has `since` 0.

## The review's remarks without a finding

| Remark | Where it went |
| --- | --- |
| At step `merge` the general rule comes before every row; rows 17 to 22 are no exception; test A5.18 against every write | `contracts/task-machine.md` section 2; B-19 and B-21 |
| `group resume` is not a task transition and must not cause a turn to perform a merge | `contracts/task-machine.md` row 24; B-18 |
| Slices (a) and (b) of the golden examples can be frozen | B-01 |
| Slice (c) is two complete transcripts with the listed contents, not the abridged trace | B-01; `flows/trace-task.md` already says it is documentation |
| Adoption, receipts, reading: no finding | nothing to change |
| Q-21 and Q-22 conflict with no rule or test; K-14 stays the stated limit | nothing to change |

## Three rounds in one table

| Round | Reviewed | Findings | Accepted | With changes | Rejected | Verdict |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `2e1488c` | 11 (4 blockers) | 8 | 3 | 0 | start the scaffold and spikes; do not freeze the wire |
| 2 | `39be475` | 8 (3 blockers) | 6 | 2 | 0 | scaffold and spikes can proceed; the task vector is not ready |
| 3 | `5994ba9` | 5 (0 blockers) | 5 | 0 | 0 | ready to start |

No proposal in any round was rejected for adding defence inside the system;
none was made. The coordinator settled 22 open points (Q-1 to Q-22 in
`risks.md`); the reviews reopened none of them.

## What remains open, by design

- What the spikes must answer before the code that depends on them
  (`steps/step-1.md`, M0): transport behaviour with each browser, the
  effective permission levels of both CLIs, takeover on one session id,
  Windows, the relay on port 443, browser keys, port mapping.
- Q-4, level 2 for Claude Code on native Windows, waits for spike S-5.
- Slice (c) of the golden examples is frozen when the backend produces it.

The plan is converged. Implementation proceeds by `steps/step-1.md`.
