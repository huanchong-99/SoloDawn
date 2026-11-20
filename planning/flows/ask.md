# Flow: one AI asks another

Contracts: `contracts/mcp-tools.md` (`ask`, `reply`),
`contracts/commands.md` (`post` kinds `call` and `reply`). Rules: R-110 to
R-115, R-60.

Actors: A, the calling AI on host NA; B, the called AI on host NB; H, the
home. NA and NB may be the same node; nothing below changes.

## A. A small request, answered while A waits

1. During a turn, A calls `ask` with the callee, the text, and `wait: true`.
2. NA sends `post` with `kind: call` in the thread of A's current task (the
   main line if A has no task), mentioning B, with `body.depth`. Depth is 1,
   or one more than the depth of the call A is answering in this turn.
3. H checks depth ≤ `limits.ask_depth` (4), counts the call as a round of
   the thread (`contracts/task-machine.md` row 21), stores the call with
   `state.status = waiting`, and queues a `mention` item for B, held while
   the task waits for `rounds` (`contracts/task-machine.md` section 4).
   Everyone watching the thread sees the call.
4. NB delivers it to B as a turn, after B's current turn if B is working.
   The turn goes to B's session for that task if B is its assignee or
   reviewer, otherwise to B's group session, which is read only
   (`flows/agent-turn.md` A).
5. B calls `reply` with the call's id and the answer. NB sends `post` with
   `kind: reply`.
6. H stores the reply, sets the call's `state` to `answered` with the
   elapsed milliseconds, and queues a `reply` item for A.
7. NA receives the item while A's `ask` call is still open. The item is
   not held behind A's running turn: it is the answer that turn is waiting
   for. NA returns it as the tool result and acknowledges the item. A
   continues its turn with the answer.

## B. No answer within the wait

8. After `limits.ask_wait_s` (45 seconds) NA returns to A: "No answer yet.
   Receipt <call id>. The answer will arrive as a message." A goes on or
   ends its turn.
9. When B replies, steps 5 and 6 happen. NA now has no open call for that
   id, so it delivers the `reply` item to A as a turn.

## C. A large request

10. A calls `ask` with `wait: false`. NA returns the receipt at once after
    step 3. The rest is B.

## D. Limits

- Every call and every reply that mentions an AI counts as one round of the
  thread. At 10 rounds without a person's message the task pauses (R-60).
- A call whose depth would exceed 4 is refused with `limit`; the tool
  returns that text to A.
- The 45-second wait is below the shortest documented default tool timeout
  of the two CLIs. `[documented]` Codex: 60 seconds; Claude Code: tools move
  to the background after 120 seconds.

## E. Authority

- The call is a message signed by NA for A. The `mention` item that carries
  it to B is signed by the home, like every item. NB accepts the item
  because the home is a device of its owner (`contracts/datachannel.md`
  section 6); the call's own signature tells NB who is asking.
- Step 1: every AI in the group belongs to the same person, so nothing more
  is needed.
- Step 2: a call to someone else's personal-subscription AI waits for that
  owner's confirmation; team and API-key AIs follow their allow lists
  (R-115).

## F. What is not built

- No direct host-to-host connection. Two messages through the home are
  enough, and the home must see them anyway to show them (R-113) and count
  them.
- No file or diff transfer in a call. Text only; code goes through Git
  (R-111).
- No streaming of B's partial output to A.
