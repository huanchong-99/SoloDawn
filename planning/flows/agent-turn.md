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
| `assign`, `checks`, `reviewed`, `resume` | the AI's task session for `task`; N creates it on the first `assign` |
| `review` | the AI's review session for `task`; N creates it on the first `review` |
| `decided` about a task | the AI's task session for `task` if it has one; else its review session for `task` if one is open; else its group session |
| `mention` or `reply` whose message is in the thread of a task for which the AI has a task or review session | that session |
| any other `mention`, `reply` or `decided` | the AI's group session |
| everything addressed to the secretary | the secretary's group session |
| `merge` | no session: N runs the merge itself (`flows/task.md` E). Held only while the group is paused or N is offline by the owner's command |

**An answer that a tool call is waiting for is not a turn.** A `reply` to a
call whose `ask` is still open, and a `decided` for an approval the CLI is
still waiting on, go straight to that waiting call (`flows/ask.md`,
`flows/approval.md`). They are never held and never queued behind the turn
that is waiting for them.

## B. From item to turn

1. Something happens in the group that concerns the AI
   (`contracts/commands.md` section 3). H writes an inbox item and, if N is
   connected, pushes it.
2. N checks the item when it arrives (`contracts/datachannel.md` section 6):
   the frame comes from the home of that group and `ai` runs on this host
   (owner's decision of 2026-11-21: no attack-and-defence code, so no item
   signature or `cause`). Otherwise N refuses the item: audit row `refused`,
   then `ack`. Rows inside the item that carry no signature are content for
   the turn, not authority.
3. N holds the item, unacknowledged, while any of these is true: the group
   is paused; the item's task is `paused` or `failed`; N is offline by the
   owner's command; the AI is `limited` by a cap or a rate limit; the
   target session is in `takeover`; the AI is already running a turn in
   this group, in any of its sessions. The home queues `decided` and
   `mention` items for a paused or failed task too; N holds every item of
   such a task and delivers them when the task is `active` again. N starts
   no turn in the session of a `failed` task.
4. If the target session does not exist yet, N creates it: prepares the
   folder (worktree for a task session, read-only checkout for a review
   session, `flows/task.md`), and records a new session row. The CLI's
   session id is stored at its first start (`contracts/drivers.md`
   section 1).
5. If S is `dormant`, N wakes it:
   1. if the number of open sessions on this node, those in `takeover`
      included, equals `max_open`, N closes the open session that has been
      `waiting` longest. If every open session is `working` or in
      `takeover`, the item waits until one is free;
   2. N starts the CLI in the session's folder with the session id, when
      it has one, and the level of the session kind
      (`contracts/drivers.md`);
   3. state becomes `working`; N reports `member status`.
6. N takes **all** held items for S, in order. Each item that asks for work
   on a task is checked now against the task as N holds it after reading
   the group's state: still active, the AI still in that role, and for
   `review` and `merge` the same step and commit; one that no longer fits
   is refused (`contracts/datachannel.md` section 6). A `notice` is always
   delivered. From the rest N composes one turn
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
    the item again under its number with `jump: true`. N replaces the one it
    holds; one it has delivered it acknowledges only.
12. If the AI is running a turn in the group, N interrupts it (the CLI's
    interrupt request) and waits for the turn-end event.
    - The jump item is for the **same** session: nothing is reported; the
      item simply goes first, and the task is not paused.
    - It is for another session, and the interrupted one is a task session:
      N reports `task preempted`. If the home refuses it (a task at step
      `merge`), nothing more is reported for that task.
13. N delivers the jump item as in steps 4 to 9.
14. When the jump turn has ended, N gives the interrupted session, whatever
    its kind, one turn of its own: "You were interrupted by a direct request
    from your owner. Continue the task." for a task session, and the same
    first sentence followed by "Continue." for the others. If its task was
    pre-empted, N first reports `task resume` for it. No inbox item is
    involved.

## D. Going dormant, and ending

15. S has been `waiting` for `idle_minutes` (default 15), or the session cap
    needs a slot (step 5.1).
16. N closes the CLI's stdin and waits for the process to exit. Only the
    session id remains. State `dormant`.
17. A task session ends when its task is merged or cancelled, or another AI
    becomes its assignee; a review session when its task is merged or
    cancelled, or another AI becomes its reviewer. N closes the process if
    open, removes the session row and the folder of that role
    (`contracts/local.md` section 7). A group session ends when the AI
    leaves the group.

## E. When things go wrong

Each case has one outcome. None of them retries by itself. "Reports" means
an `event` in `member status`, which H turns into a `system` row
(`contracts/commands.md` 1.9).

| Event | What N does |
| --- | --- |
| the CLI exits or closes its output during a turn | reports `turn_interrupted` (`crash`); in a task session also sends `task fail`; state `dormant`; the next item starts the CLI again with the same session id |
| a turn lasts longer than `limits.turn_minutes`, or a Codex turn is silent (`contracts/drivers.md` 1.2 step 6) | interrupts it; reports `turn_interrupted` (`timeout`); in a task session also sends `task fail` |
| the CLI cannot resume the session id | starts a fresh session in the same folder; its first turn carries what the first turn of that session kind carries, plus the last 20 messages of the thread (R-123); reports `session_replaced` |
| the CLI reports a vendor rate limit | state `limited`; reports `rate_limited` with the reset time; items are held until then |
| the owner's cap is reached (R-61) | state `limited`; reports `cap_reached`; items are held until the cap is released (`contracts/commands.md` section 2) |
| the CLI reports that sign-in is not valid in this mode | state `limited`; reports `login_invalid` only when the AI enters `limited`, at the first failed start after a success; later failed tries are not reported again. N never changes how the CLI authenticates (R-56). Sending `agent.set` for that CLI sets `login_ok` back to `true` in `host.info`, while the member's `limited` state ends at its next completed turn (the start for the next item is still tried, and a completed turn also restores `login_ok`) |
| group paused | interrupts running turns of the group's sessions (`turn_interrupted`, `stop`); the sessions stay open; items are held |
| task paused (`contracts/task-machine.md` row 12) | interrupts a running turn of that task's sessions (`turn_interrupted`, `stop`); the sessions stay open; items are held |
| owner's "all offline" | interrupts running turns (`turn_interrupted`, `stop`) and closes every session; items are held |
| the node starts and finds session rows saved as `working` or `waiting` | they become `dormant`; one that was `working` is reported as a crash: `turn_interrupted` (`crash`), and `task fail` in a task session. A row saved as `takeover` is handed back (`flows/takeover.md` E) |

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
  before anything else starts (C-6). A structured CLI, Claude Code or
  Codex, is closed by closing its stdin; an interactive CLI by closing its
  pseudo-terminal.
- A CLI that does not answer an interrupt or a close within 10 s gets its
  process tree killed. During a turn that is a crash (section E, first
  row). Outside a turn (an idle close) N writes an audit row and nothing
  else: no `turn_interrupted` and no `task fail`.
- N cannot stop a person from resuming the same session id in their own
  terminal outside SoloDawn. The owner is told not to; see `risks.md` K-6.
