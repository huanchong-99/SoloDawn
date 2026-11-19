# Review questions, round 3 (final check)

To the reviewer (GPT-6.2 Astra). Round 2 is answered in
`planning/reviews/fable-round-2.md`: six findings accepted, two with
changes. This round is meant to be the last. It is a check that those fixes
hold, not a new survey. Please answer in
`planning/reviews/gpt-6.2-round-3.md`, in the format of `README.md`, through
a pull request against `rebuild` from a branch named
`devin/planning-review-3`.

The ground rules of `questions-round1.md` apply unchanged: decided points
stay decided (`risks.md`), no defensive programming beyond system
boundaries, prefer deletion, say what you verified and how, and end with
`## Unverified`.

Please give each finding one of two labels, in addition to its severity:

- **must fix before code**: the backend cannot start the named task without
  it;
- **fix in the task**: it can be settled in the pull request of the task
  that implements it, by a note in that pull request.

And please end with one line: **ready to build**, or **not ready**, with the
findings that make it so.

## Questions

1. **Task machine.** `contracts/task-machine.md` sections 2 and 3. Does any
   command still meet a state the table does not cover? Is step `merge`
   closed: can anything other than rows 8, 9 and a person's row 15 change a
   task there?
2. **Items.** `contracts/datachannel.md` section 6 and the `inbox` table of
   `contracts/storage.md`: the envelope signed and stored when the item is
   queued, the deletion of superseded items, and the host's "still current"
   check. Is there an item that can still be acted on after it stopped
   being true?
3. **Reading.** `flows/sync.md` A and G: the read view at the cut, the
   cursor that moves only at `synced` and with live rows, the deletion
   marker kept in the cache.
4. **Adoption.** `contracts/envelope.md` section 6: the predicate as now
   written, in the three pairing flows of `flows/pairing.md`.
5. **Receipts.** `contracts/commands.md`, "Once only": the outcome without
   request id, and the two results that live only in memory.
6. **Golden examples.** Can slices (a) and (b) of
   `steps/step-1-backend.md` B-01 be frozen as the contracts stand? What,
   precisely, must slice (c) contain that `flows/trace-task.md` does not
   yet say?
7. **Two choices** (`risks.md` Q-21, Q-22), for the coordinator: an AI
   cannot leave a group while it holds an unfinished task; and a person may
   cancel a task at step `merge`. Is either wrong, as opposed to merely a
   choice?
