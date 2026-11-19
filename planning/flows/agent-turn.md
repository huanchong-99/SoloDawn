# Flow: delivering a message to an AI

Contracts: `contracts/datachannel.md` section 6 (inbox),
`contracts/local.md` (CLI invocations, turn format),
`contracts/mcp-tools.md`. Rules: R-33, R-120 to R-124, R-60 to R-63.

Actors: H, the group home; N, the host node of the AI; S, the CLI session of
(AI, group); the AI itself.

## A. From message to turn

1. Something happens in the group that concerns the AI (table in
   `contracts/commands.md` section 3). H writes an inbox item and, if N is
   connected, pushes it.
2. N checks the item's authority. In step 1 there is one rule: the envelope
   of `msg` is signed by a device in the log of N's owner. Otherwise N drops
   the item, acknowledges it, and writes an audit row `refused`.
3. N holds the item while any of these is true: the group is paused; N is
   offline by the owner's command; the AI is `limited`; S is in `takeover`;
   S is `working`.
4. If S is `dormant`, N wakes it:
   1. if the number of open sessions equals `max_open`, N closes the open
      session that has been `waiting` longest (close stdin, wait for exit);
   2. N starts the CLI in the project folder of the group with the stored
      session id (`contracts/local.md`), or with a fresh id the first time;
   3. state becomes `working`; N reports `member status`.
5. N takes **all** held items for S, in order, and composes one turn
   (`contracts/local.md` section 5). Each item is labelled with who wrote it
   and whether it is an instruction or information (R-52).
6. N stores the highest item id as delivered, acknowledges the items to H,
   then writes the turn to S. An item is delivered at most once; if N stops
   between storing and writing, the turn is lost and the secretary's
   follow-up covers it (R-22). There is no redelivery logic.
7. The AI works. Whatever it wants the group to see it sends with tools
   (`post`, `ask`, …). Text the AI prints outside tool calls is not posted.
8. The CLI signals the end of the turn. N records usage from that event,
   sets state `waiting`, reports `member status`.
9. If items arrived meanwhile, continue at 5.

## B. Jumping the queue (R-72)

10. An item with `jump: true` arrives while S is `working`.
11. N interrupts the running turn (the CLI's interrupt request), waits for
    the turn-end event, and continues at 5 with that item first.
12. H has already paused the interrupted task (`wait.kind = preempted`).
    When S next becomes `waiting`, H makes it active again and queues a
    `resume` item.

## C. Going dormant

13. S has been `waiting` for `idle_minutes` (default 15), or the session cap
    needs a slot (step 4.1).
14. N closes the CLI's stdin and waits for the process to exit. Only the
    session id remains. State `dormant`; N reports it.

## D. When things go wrong

Each case has one outcome. None of them retries by itself.

| Event | What N does |
| --- | --- |
| the CLI exits or closes its output during a turn | posts `system` `turn_interrupted` (`crash`); if a task was in progress sends `task fail`; state `dormant`; the next item starts the CLI again with the same session id |
| a turn lasts longer than `limits.turn_minutes` | interrupts it; posts `turn_interrupted` (`timeout`); sends `task fail` |
| the CLI cannot resume the session id | starts a fresh session; the first turn carries the task card, the notes of the group and the last 20 thread messages (R-123); posts `system` `session_replaced` |
| the CLI reports a vendor rate limit | state `limited` with `usage.window_resets`; items are held until then |
| the owner's cap is reached (R-61) | state `limited`; items are held until the owner raises the cap or the day rolls over |
| the CLI reports that sign-in is not valid in this mode | state `limited`; posts `system` `login_invalid`; N never changes how the CLI authenticates (R-56) |
| group paused, or owner's "all offline" | interrupts running turns of the affected sessions and closes them; items are held |

## E. Who the AI is talking to

- The AI never addresses the home or other nodes. It calls tools on its own
  node. N turns each call into a signed command to H and returns H's answer
  as the tool result.
- N signs with its device key and sets `as` to the AI's member id.
- If H is not connected, the tool returns the error text
  `home_unreachable`.

## F. One process, one session

- N keeps one row per (AI, group) with the CLI's session id and the state.
- Exactly one process is attached to a session id while the state is
  `working`, `waiting` or `takeover`. Every switch (dormant, takeover,
  hand-back, restart) closes the current process and waits for its exit
  before anything else starts (C-6).
- N cannot stop a person from resuming the same session id in their own
  terminal outside SoloDawn. The owner is told not to; see `risks.md` K-6.
