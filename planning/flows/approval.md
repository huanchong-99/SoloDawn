# Flow: an AI asks for permission

Contracts: `contracts/local.md` (how levels map to CLI settings),
`contracts/commands.md` (`post` kind `approval`, `act`),
`contracts/rows.md` (approval body). Rules: R-40 to R-43, R-136, R-139.

Actors: the AI; S, its CLI session; N, its host node; H, the group home;
B, a browser of the AI's owner.

## A. Where requests come from

The node does not decide what is dangerous and does not inspect commands
(C-10). Each CLI session runs with the permission and sandbox settings of
its level: the level the owner set for this AI in this project for a task
session, level 1 for group and review sessions (`architecture.md` section
6). The CLI itself stops when an action exceeds them and asks through its
permission interface:

- Claude Code calls the node's MCP tool `approve` (its permission prompt
  tool) with the tool name and input.
- Codex sends an approval request on the app-server connection.

Both arrive at N as the same internal request: session, tool, input, and the
justification the CLI attached, if any.

## B. The request

1. N receives the request.
2. Level 1 (read only): N answers "deny" at once. No card.
3. N looks for a one-shot grant for (AI, group, tool, exact input). If there
   is one, N deletes it, answers "allow", writes an audit row, done.
4. N runs the outbound secret check on the request text and on the
   justification (R-137). On a match
   N answers "deny" with the text "This request contains a secret and
   cannot be shown for approval", writes an audit row, and stops here. No
   card.
5. N sends `post` with `kind: approval` to H, signed for the AI, with the
   tool, the request text exactly as received, the justification, and the
   list of approvers. N writes an audit row `requested`.
6. H stores the card with `state.status = pending`, sets the task's
   `wait.kind = approval`, and pushes the rows. The card appears in the main
   line under the secretary's name and mentions the approvers; the thread
   shows the request. H writes it; no AI turn is spent (R-42). Browsers of
   approvers list it under "waiting for you"; others see it greyed out with
   the name of who can decide.
7. N keeps the CLI's request open for up to `limits.approval_wait_s`
   (default 600 seconds). For Claude Code it sends an MCP progress
   notification every 30 seconds so the call is not considered idle.

## C. Decided while the CLI is still waiting

8. The owner taps approve or deny. B signs `act` (`approve` or `deny`) and
   sends it to H.
9. H checks that the signer is in `body.approvers` and the card is pending,
   updates `state`, clears `task.wait`, pushes rows, and queues an inbox item
   `decided` for the AI carrying B's envelope.
10. N checks that envelope itself: signed by a device of its owner (or of a
    delegate the owner named), and `msg` is the request N is holding. If the
    check fails N ignores the item and writes an audit row `refused`.
11. N answers the CLI "allow" or "deny". The AI continues in the same turn.
    N writes an audit row with the decision and the signer.

## D. Not decided in time

12. The wait runs out. N answers the CLI "deny" with the text: "No decision
    yet. The request is still open. Stop this action and end your turn; you
    will be told when it is decided."
13. The turn ends. The task keeps `wait.kind = approval`; the card stays
    pending. The session may go dormant.
14. When the owner approves later, steps 8 to 10 happen. N then stores a
    one-shot grant for (AI, group, tool, exact input) and delivers the
    `decided` item as a turn: "Approved. Run exactly the same request
    again." The retry is allowed by step 3.
15. When the owner denies later, N delivers the `decided` item as a turn:
    "Denied."

A grant is used once and is deleted when the task ends.

## E. Reassigned or cancelled

16. The secretary may give the task to another AI while the card is pending
    (R-42). H sets the card's `state.status = reassigned`; the first AI's
    task session ends (`flows/task.md`, "Reassignment").
17. If the task is cancelled or fails, H sets the card to `expired`.

## F. What is recorded on the owner's node (R-139)

One audit row per request and per decision: time, group, AI, tool, request
text, who decided (device id), result.

## G. Levels at a glance

| Level | The CLI is started with | Requests |
| --- | --- | --- |
| 1 read only | no write or execute tools | denied by N without a card |
| 2 workspace | edits inside the task's worktree accepted; commands run in the CLI's sandbox; anything beyond asks. Pushing the task's own branch is done by the node (`push_branch`, `submit_task`) and never asks | card, owner decides |
| 3 unrestricted | the CLI's own "do not ask" mode | none |

Exact flags per CLI are in `contracts/local.md`. Work that someone other
than the owner triggered never runs above level 2 (R-44, step 2).

## H. Not verified yet

- `[unverified]` How long Claude Code lets a permission prompt tool call
  stay open, and whether the 120-second "move to background" rule for MCP
  tools applies to it. Spike S-2. If the real limit is shorter than 600
  seconds, `approval_wait_s` is lowered; the flow does not change.
- `[unverified]` Whether level 2 can be expressed on Claude Code for native
  Windows, where its sandbox is not available. The coordinator decides after
  spike S-5 (`risks.md` Q-4).
