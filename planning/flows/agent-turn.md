# Flow: delivering a message to an AI

Contracts: `contracts/datachannel.md` section 6 (inbox),
`contracts/local.md` (CLI invocations, turn format),
`contracts/mcp-tools.md`. Rules: R-12, R-33, R-120 to R-124, R-60 to R-63,
R-70 to R-72. Session kinds are defined in `sessions.md`.

Actors: H, the group home; N, the host node of the AI; S, a CLI session; the
AI itself.

## A. Which session gets an item

H addresses items to an AI. N chooses the session.

| Item | Session |
| --- | --- |
| `assign`, `checks`, `resume`, and `review` or `decided` addressed to the assignee | the AI's task session for `task`; N creates it on the first `assign` |
| `review` addressed to the reviewer | the AI's review session for `task`; N creates it on the first `review` |
| `mention` or `reply` whose message is in the thread of a task for which the AI has a task or review session | that session |
| any other `mention` or `reply`; `decided` for a decision asked from the group session | the AI's group session |
| everything addressed to the secretary | the secretary's group session |
| `merge` | no session: N runs the merge itself (`flows/task.md` E) |

## B. From item to turn

1. Something happens in the group that concerns the AI (table in
   `contracts/commands.md` section 3). H writes an inbox item and, if N is
   connected, pushes it.
2. N checks the item's authority. In step 1 there is one rule: the envelope
   of `msg` is signed by a device in the log of N's owner. Otherwise N drops
   the item, acknowledges it, and writes an audit row `refused`.
3. N holds the item while any of these is true: the group is paused; N is
   offline by the owner's command; the AI is `limited`; the target session is
   in `takeover`; the AI is already running a turn in this group, in any of
   its sessions.
4. If the target session does not exist yet, N creates it: prepares the
   folder (worktree for a task session, read-only checkout for a review
   session, `flows/task.md`), and records a new session row with a fresh
   session id.
5. If S is `dormant`, N wakes it:
   1. if the number of open sessions on this node equals `max_open`, N
      closes the open session that has been `waiting` longest (close stdin,
      wait for exit);
   2. N starts the CLI in the session's folder with the session id and the
      level of the session kind (`contracts/local.md` section 4);
   3. state becomes `working`; N reports `member status`.
6. N takes **all** held items for S, in order, and composes one turn
   (`contracts/local.md` section 5). Each item is labelled with who wrote it
   and whether it is an instruction or information (R-52). The first turn of
   a task session also carries the brief, the acceptance criteria and the
   group's notes.
7. N acknowledges the items to H, then writes the turn to S. An item is
   delivered at most once: if N stops between the two, the turn is lost and
   the secretary's follow-up covers it (R-22). N remembers delivered ids in
   memory while it runs, so an item that H sends again after a reconnect is
   ignored. There is no other redelivery logic.
8. The AI works. Whatever it wants the group to see it sends with tools
   (`post`, `ask`, …). Text the AI prints outside tool calls is not posted.
9. The CLI signals the end of the turn. N records usage from that event,
   sets state `waiting`, reports `member status`.
10. N looks at all held items of this AI in this group, across its sessions,
    and continues at 4 with the session of the oldest one.

## C. Jumping the queue (R-72)

11. An item with `jump: true` arrives while the AI is running a turn in the
    group.
12. N interrupts the running turn (the CLI's interrupt request), waits for
    the turn-end event, and continues at 4 with that item first.
13. H has already paused the interrupted task (`wait.kind = preempted`).
    When the AI is next `waiting`, H makes that task active again and queues
    a `resume` item for its task session.

## D. Going dormant, and ending

14. S has been `waiting` for `idle_minutes` (default 15), or the session cap
    needs a slot (step 5.1).
15. N closes the CLI's stdin and waits for the process to exit. Only the
    session id remains. State `dormant`.
16. A task session and a review session end when their task is merged or
    cancelled: N closes the process if open, removes the session row and the
    folder. A group session ends when the AI leaves the group.

## E. When things go wrong

Each case has one outcome. None of them retries by itself.

| Event | What N does |
| --- | --- |
| the CLI exits or closes its output during a turn | posts `system` `turn_interrupted` (`crash`); in a task session also sends `task fail`; state `dormant`; the next item starts the CLI again with the same session id |
| a turn lasts longer than `limits.turn_minutes` | interrupts it; posts `turn_interrupted` (`timeout`); in a task session also sends `task fail` |
| the CLI cannot resume the session id | starts a fresh session in the same folder; its first turn carries what the first turn of that session kind carries, plus the last 20 messages of the thread (R-123); posts `system` `session_replaced` |
| the CLI reports a vendor rate limit | state `limited` with `usage.window_resets`; items are held until then |
| the owner's cap is reached (R-61) | state `limited`; items are held until the owner raises the cap or the day rolls over |
| the CLI reports that sign-in is not valid in this mode | state `limited`; posts `system` `login_invalid`; N never changes how the CLI authenticates (R-56) |
| group paused, or owner's "all offline" | interrupts running turns of the affected sessions and closes them; items are held |

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
