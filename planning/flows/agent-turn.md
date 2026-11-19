# Flow: delivering a message to an AI

Contracts: `contracts/datachannel.md` section 6 (inbox),
`contracts/drivers.md` (CLI invocations, turn format),
`contracts/mcp-tools.md`. Rules: R-12, R-33, R-120 to R-124, R-60 to R-63,
R-70 to R-72, R-136. Session kinds are defined in `sessions.md`.

Actors: H, the group home; N, the host node of the AI; S, a CLI session; the
AI itself.

## A. Which session gets an item

H addresses items to an AI. N chooses the session.

| Item | Session |
| --- | --- |
| `assign`, `checks`, `reviewed`, `resume`; `decided` about a task the AI is assigned | the AI's task session for `task`; N creates it on the first `assign` |
| `review` | the AI's review session for `task`; N creates it on the first `review` |
| `mention` or `reply` whose message is in the thread of a task for which the AI has a task or review session | that session |
| any other `mention`, `reply` or `decided` | the AI's group session |
| everything addressed to the secretary | the secretary's group session |
| `merge` | no session: N runs the merge itself (`flows/task.md` E) |

**An answer that a tool call is waiting for is not a turn.** A `reply` to a
call whose `ask` is still open, and a `decided` for an approval the CLI is
still waiting on, go straight to that waiting call (`flows/ask.md`,
`flows/approval.md`). They are never held and never queued behind the turn
that is waiting for them.

## B. From item to turn

1. Something happens in the group that concerns the AI
   (`contracts/commands.md` section 3). H writes an inbox item, signs it,
   and, if N is connected, pushes it.
2. N checks the item at its boundary (R-136): the item's envelope is signed
   by the home of that group, that device is in the log of N's owner, the
   item is addressed to N and names an AI that runs here. For an item
   about a task, the task as N sees it now must still fit the item: not
   final, the AI still in that role, and for `review` and `merge` the same
   commit (`contracts/datachannel.md` section 6). Otherwise N refuses the
   item: audit row `refused`, then `ack`. Rows inside the item that carry
   no signature are content for the turn, not authority.
3. N holds the item, unacknowledged, while any of these is true: the group
   is paused; the task of the target session is `paused`; N is offline by
   the owner's command; the AI is `limited`; the target session is in
   `takeover`; the AI is already running a turn in this group, in any of
   its sessions.
4. If the target session does not exist yet, N creates it: prepares the
   folder (worktree for a task session, read-only checkout for a review
   session, `flows/task.md`), and records a new session row with a fresh
   session id.
5. If S is `dormant`, N wakes it:
   1. if the number of open sessions on this node equals `max_open`, N
      closes the open session that has been `waiting` longest. If every
      open session is `working`, the item waits until one is free;
   2. N starts the CLI in the session's folder with the session id and the
      level of the session kind (`contracts/drivers.md`);
   3. state becomes `working`; N reports `member status`.
6. N takes **all** held items for S, in order, and composes one turn
   (`contracts/drivers.md` section 3). Each item is labelled with who wrote
   it and whether it is an instruction or information (R-52). The first turn
   of a task session also carries the brief, the acceptance criteria and the
   group's notes.
7. For each item N writes the audit row `delivered`, with the home and the
   item id, then acknowledges the items to H, then writes the turn to S. An
   item whose audit row already exists is acknowledged and skipped: that is
   how an item sent twice by H is delivered once. If N stops between the
   audit row and the write, the turn is lost (K-3); the home's quiet-task
   notice brings the secretary to it (`flows/task.md` H). A turn is never
   delivered a second time.
8. The AI works. Whatever it wants the group to see it sends with tools
   (`post`, `ask`, …). Text the AI prints outside tool calls is not posted.
9. The CLI signals the end of the turn. N records usage from that event,
   sets state `waiting`, reports `member status`.
10. N looks at all held items of this AI in this group, across its sessions,
    and continues at 4 with the session of the oldest one.

## C. Jumping the queue (R-72)

11. The owner chooses "jump the queue" on their message (`jump`). H sends
    the item again with `jump: true`.
12. If the AI is running a turn in the group, N interrupts it (the CLI's
    interrupt request) and waits for the turn-end event.
    - The jump item is for the **same** session: nothing is reported; the
      item simply goes first, and the task is not paused.
    - It is for another session, and the interrupted one is a task session:
      N reports `task preempted`.
13. N delivers the jump item as in steps 4 to 9.
14. If a task was pre-empted: when the jump turn has ended, N reports
    `task resume` for it and gives the interrupted session one turn of its
    own: "You were interrupted by a direct request from your owner.
    Continue the task." No inbox item is involved.

## D. Going dormant, and ending

15. S has been `waiting` for `idle_minutes` (default 15), or the session cap
    needs a slot (step 5.1).
16. N closes the CLI's stdin and waits for the process to exit. Only the
    session id remains. State `dormant`.
17. A task session and a review session end when their task is merged,
    cancelled, or given to another AI: N closes the process if open, removes
    the session row and the folder. A group session ends when the AI leaves
    the group.

## E. When things go wrong

Each case has one outcome. None of them retries by itself. "Reports" means
an `event` in `member status`, which H turns into a `system` row
(`contracts/commands.md` 1.9).

| Event | What N does |
| --- | --- |
| the CLI exits or closes its output during a turn | reports `turn_interrupted` (`crash`); in a task session also sends `task fail`; state `dormant`; the next item starts the CLI again with the same session id |
| a turn lasts longer than `limits.turn_minutes` | interrupts it; reports `turn_interrupted` (`timeout`); in a task session also sends `task fail` |
| the CLI cannot resume the session id | starts a fresh session in the same folder; its first turn carries what the first turn of that session kind carries, plus the last 20 messages of the thread (R-123); reports `session_replaced` |
| the CLI reports a vendor rate limit | state `limited`; reports `rate_limited` with the reset time; items are held until then |
| the owner's cap is reached (R-61) | state `limited`; reports `cap_reached`; items are held until the cap is released (`contracts/commands.md` section 2) |
| the CLI reports that sign-in is not valid in this mode | state `limited`; reports `login_invalid`; N never changes how the CLI authenticates (R-56) |
| group paused, or owner's "all offline" | interrupts running turns of the affected sessions (`turn_interrupted`, `stop`) and closes them; items are held |

## F. Who the AI is talking to

- The AI never addresses the home or other nodes. It calls tools on its own
  node. N turns each call into a signed command to H and returns H's answer
  as the tool result.
- N signs with its device key and sets `as` to the AI's member id.
- Before sending anything an AI wrote, N runs the outbound secret check
  (`contracts/local.md` section 8). On a match the tool fails with `secret`
  and nothing leaves the machine (R-137).
- If H is not connected, the tool returns the error text
  `home_unreachable`.

## G. One process, one session

- N keeps one row per session with the CLI's session id, the folder and the
  state.
- Exactly one process is attached to a session id while the state is
  `working`, `waiting` or `takeover`. Every switch (dormant, takeover,
  hand-back, restart) closes the current process and waits for its exit
  before anything else starts (C-6).
- N cannot stop a person from resuming the same session id in their own
  terminal outside SoloDawn. The owner is told not to; see `risks.md` K-6.
