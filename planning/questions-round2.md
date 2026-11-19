# Review questions, round 2

To the reviewer (GPT-6.2 Astra). Round 1 is answered in
`planning/reviews/fable-round-1.md`; every finding was accepted, three with
changes. Please review what changed and answer in
`planning/reviews/gpt-6.2-round-2.md`, in the format of `README.md`, through
a pull request against `rebuild` from a branch named
`devin/planning-review-2`.

The ground rules of `questions-round1.md` apply unchanged: decided points
stay decided (Q-1 to Q-20 in `risks.md`), no defensive programming beyond
system boundaries, prefer deletion, say what you verified and how, and end
with `## Unverified`.

## What is new since the commit you reviewed

- `contracts/task-machine.md`: the single transition table.
- `contracts/drivers.md`: CLI start, levels, turn format, instructions
  (moved out of `contracts/local.md`), with a table of what the node does
  itself at each level.
- `flows/trace-task.md`: one confirmed task on the wire.
- Signed inbox items, receipts, `devlog`, the read protocol with a cut,
  `pos` against `seq`, one commit through checks, review and merge.
- `steps/step-1-backend.md`: smaller tasks in a new order.

## Questions

1. **Task machine.** For each command in `contracts/task-machine.md`, is
   there a state in which it can arrive that the table does not cover? Do
   rows 5 to 7 (a second submission during hosted checks or review), 11
   (assignment), 16 and 17 (pre-emption) and 22 and 23 (rounds) leave any
   state without a way out? Is anything in the table more than step 1
   needs?
2. **Signed items.** `contracts/datachannel.md` section 6 and
   `flows/trace-task.md`. In step 1, is "signed by the home of the group,
   which is a device in my owner's log" enough for every `why`? Is `cause`
   carried exactly where a host or a later step needs it? Does the trace
   disagree with any contract?
3. **Reading.** `flows/sync.md` A to C and `flows/connect.md` step 11. With
   the cut, the separate `reset` push, `pos` and `seq`, the purge mark and
   "apply only a higher `seq`": find a sequence that still loses, repeats
   or misorders a row.
4. **Receipts.** One receipt per accepted command, 24 hours, purged by the
   command's own `ts`. Any command for which the stored answer is wrong to
   return a second time?
5. **Pairing.** `contracts/envelope.md` section 6 and `flows/pairing.md`:
   the bootstrap limit and the adoption rule. Can a device be made to adopt
   the wrong person, or an already paired device be re-adopted?
6. **Levels.** `contracts/drivers.md` 1.3 and 2: starting the interactive
   CLI with the session's level, and the level-1 tool set. What is missing
   to make this testable in spike S-4?
7. **Checks at level 2.** Decided (`risks.md` Q-19): in step 1 they run
   outside the sandbox with a minimal environment, and the product says
   so; in step 2 they move into an existing operating-system sandbox. The
   read-only terminal of the secretary is decided too (Q-20). Not open for
   redesign. Two narrow questions remain: is the environment allow list in
   `contracts/local.md` section 7 missing a variable that ordinary build
   and test tools cannot work without, and what should the step 2
   experiment measure first?
8. **Order.** `steps/step-1-backend.md`: is any task still too large, or in
   the wrong place? Is slice (c) of B-01 specified well enough by
   `flows/trace-task.md` to be frozen?
9. **Deletion.** After this round the plan has more files. Name what can be
   removed again without breaking an `[S1]` rule.
