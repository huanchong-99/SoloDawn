# Flow: an AI asks for permission

Contracts: `contracts/drivers.md` (how levels map to CLI settings),
`contracts/commands.md` (`post` kind `approval`, `act`),
`contracts/messages.md` (approval body), `contracts/storage.md` (`approval`).
Rules: R-40 to R-43, R-136, R-137, R-139.

Actors: the AI; S, its CLI session; N, its host node; H, the group home;
B, a browser of the AI's owner.

## A. Where requests come from

The node does not decide what is dangerous and does not inspect commands
(C-10). Each CLI session runs with the permission and sandbox settings of
its level: the level the owner set for this AI in this project for a task
session, level 1 for group and review sessions (`sessions.md`). The CLI
itself stops when an action exceeds them and asks through its permission
interface:

- Claude Code calls the node's MCP tool `approve` (its permission prompt
  tool) with the tool name and input.
- Codex sends an approval request on the app-server connection.

Both arrive at N as the same internal request: session, tool, input, and the
justification the CLI attached, if any. Which request of which CLI gives
which fields of the card is in `contracts/messages.md` 2.3.

## B. The request

1. N receives the request.
2. Level 1 (read only): N answers "deny" at once with the text "Denied:
   this session is read only." No card.
3. N looks in its approval records for one of this AI and task with the same
   tool and the same input digest, in state `granted`. If there is one, N
   deletes it, answers "allow", writes an audit row, done.
4. N runs the outbound secret check on the request text and on the
   justification (R-137). On a match N answers "deny" with the text "This
   request contains a secret and cannot be shown for approval", writes an
   audit row, and stops here. No card.
5. N writes an approval record: the id of the card it is about to post,
   the task, the tool, and the SHA-256 of the input exactly as the CLI
   sent it. For Claude Code that is `input`; for a Codex command its
   `command` and `cwd`; for a Codex file change the `changes` of its item.
   The request's own id is left out: it is new on every attempt. Then N
   sends `post` with `kind: approval` and that id to H, signed for the AI,
   with the tool, the request text, the justification, the kind of request,
   and the list of approvers. Audit row `requested`. A request too long
   for a card is not refused: the card carries its beginning and says how
   long the whole is (`contracts/messages.md` 2.3). The record, and so the
   owner's decision, is about the whole request, of which N keeps the
   digest and not the text (`risks.md` K-21). If H refuses the `post`, N
   deletes the record and answers "deny" with the text "The approval
   request was refused: <code>: <reason>", the error H answered.
6. H stores the card with `state.status = pending`, sets the task's wait to
   `approval`, and pushes the rows. The card appears in the main line under
   the secretary's name and mentions the approvers; the thread shows the
   request. H writes it; no AI turn is spent (R-42). Browsers of approvers
   list it under "waiting for you"; others see it greyed out with the name
   of who can decide.
7. N keeps the CLI's request open for up to `limits.approval_wait_s`
   (default 600 seconds). It sends no progress notifications (section H).

## C. Decided while the CLI is still waiting

8. The owner taps approve or deny. B signs `act` (`approve` or `deny`) and
   sends it to H.
9. H checks that the signer is in `body.approvers` and the card is pending,
   updates `state`, clears the wait, pushes rows, and queues a `decided`
   item for the AI whose `cause` is B's envelope, unchanged.
10. N checks the item like any item, and then the cause itself: signed by a
    device of its owner or of an approver the owner named, and about the
    card of an approval record N holds. If not, N refuses the item (audit
    row `refused`).
11. The item is not a turn: it goes straight to the waiting request. N
    answers the CLI "allow" or "deny", deletes the record, and writes an
    audit row with the decision and the signer. The AI continues in the
    same turn.

## D. Not decided in time

12. The wait runs out. N answers the CLI "deny" with the text: "No decision
    yet. The request is still open. Stop this action and end your turn; you
    will be told when it is decided."
13. The turn ends. The task keeps its wait; the card stays pending; the
    record stays. The session may go dormant, and N may restart.
14. When the owner approves later, steps 8 to 10 happen. N, in one database
    transaction, writes the audit row `delivered` for the item and marks the
    record `granted`. Then it delivers the item as a turn: "Approved. Run
    exactly the same request again." The retry is allowed by step 3.
15. When the owner denies later, N deletes the record and delivers the item
    as a turn: "Denied."

A granted record allows one request, the one whose input has the recorded
digest. Records of a task are deleted when the task is given to another AI
or ends.

## E. Reassigned or cancelled

16. The secretary may give the task to another AI while the card is pending
    (R-42). H sets the card's `state.status = reassigned`; the first AI's
    task session ends (`flows/task.md`, "Reassignment").
17. If the task is cancelled, H sets the card to `expired`.

## F. What is recorded on the owner's node (R-139)

One audit row per request and per decision: time, group, AI, tool, request
text as the card shows it, who decided (device id), result.

## G. Levels at a glance

| Level | The CLI is started with | Requests |
| --- | --- | --- |
| 1 read only | settings that allow no change | denied by N without a card |
| 2 workspace | the CLI's sandbox and permission rules for the task's worktree; what the CLI asks for becomes a card | card, owner decides |
| 3 unrestricted | the CLI's own "do not ask" mode | none |

What each of these settings was measured to confine on each CLI, and what
is not shown yet, is in `contracts/drivers.md` 1.1 and 1.2; that is the one
place for it.

- The same settings apply when the session is taken over in the interactive
  CLI, where the CLI asks the owner in the terminal instead of through a
  card (`flows/takeover.md`).
- Exact flags per CLI, and what the node itself does for a session at each
  level (checks, push, pull request, merge), are in `contracts/drivers.md`
  sections 1 and 2.
- The project's check commands are run by the node, outside the CLI's
  sandbox, on code the AI wrote, with a minimal environment. Level 2
  confines what the AI does; it does not confine what the project's own
  checks execute, and the web client says so next to the level control
  (decided, `risks.md` Q-19).
- Work that someone other than the owner triggered never runs above level 2
  (R-44, step 2).

## H. What the spikes showed, and what is still open

- `[verified]` Both CLIs let a request stay open for 630 seconds and then
  honoured the answer (S-2, S-3), Claude Code with and without progress
  notifications; longer was not tried. Claude Code's 120-second rule for
  MCP tools does not apply to `approve`. `approval_wait_s` stays at 600.
- `[verified]` The input of a retried request was byte-identical, in two
  of two tries on each CLI. The model writes it again each time, so this is
  likely and not certain; when it differs, step 3 does not recognise the
  retry and the request produces a new card.
- `[unverified]` That a blocked action at level 2 becomes a request when
  the AI asks to run it outside the sandbox (`contracts/drivers.md` 1.1;
  `risks.md` Q-25).
- Decided after spike S-5 (`risks.md` Q-4): on native Windows, where
  Claude Code has no sandbox, the node refuses level 2 for Claude Code and
  offers only the levels it can enforce.
