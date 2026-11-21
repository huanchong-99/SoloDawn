# Flow: an AI asks for permission or a decision

Contracts: `contracts/drivers.md` (how levels map to CLI settings),
`contracts/commands.md` (`post` kinds `approval` and `decision`, `act`),
`contracts/messages.md` (approval and decision bodies and card states),
`contracts/mcp-tools.md` (`approve`), `contracts/storage.md` (`approval`).
Rules: R-40 to R-43, R-136, R-137, R-139, and the 2026-11-21 owner
overrides (default level 3 across sessions; questions and approvals go
through cards at level 2 and level 3; slimming A-2 and A-3).

Actors: the AI; S, its CLI session; N, its host node; H, the group home;
B, a browser of the AI's owner.

## A. Where requests come from

The node does not decide what is dangerous and does not inspect commands
(C-10). Every CLI session — group, review or task — defaults to level 3
(the highest permission level, owner override 2 of 2026-11-21), unless
`perm` or `default_level` sets another level and `all_level3` is off. Even
at level 3, the CLIs stop and ask for multiple-choice questions and for
actions that still require confirmation (owner override 1 of 2026-11-21):

- Claude Code calls the node's MCP tool `approve` (its permission prompt
  tool) for tool permission prompts and for `AskUserQuestion`.
- Codex sends `item/commandExecution/requestApproval` and
  `item/fileChange/requestApproval` permission requests (answered through
  `Runner.Answer`) and `item/tool/requestUserInput` multiple-choice
  questions (answered through `Ask.Reply`) on the app-server connection.

Permission requests arrive at N with session, tool, input, and any
justification the CLI attached (`contracts/messages.md` 2.3). CLI
multiple-choice questions arrive with a list of questions, optional headers
and options (`label`, `description`), and accept either a chosen option
(with or without notes) or a custom free-text answer (`contracts/mcp-tools.md`
section 3).

## B. Permission requests (`approval` cards)

1. N receives the permission request.
2. Level 1 (read only): N answers "deny" at once with the text "Denied:
   this session is read only." No card. Requests raised by the CLI at level
   2 or level 3 continue to step 3.
3. N looks in its approval records for one of this AI and task with the same
   tool and the same input digest, in state `granted`. If there is one, N
   deletes it, answers "allow", writes an audit row, done. Otherwise, if N
   holds a `pending` record for that same `(g, member, task, tool, input)`
   and H is connected, N fetches that card's `msg` row from H (`fetch` with
   `r: "msg"`): when `state.status` is `approved`, N deletes the record,
   writes an `approved` audit row, and answers "allow"; when `state.status`
   is `denied`, `expired` or `reassigned`, N deletes the stale record and
   checks any remaining `pending` record.
4. N runs the outbound secret check on the request text and on the
   justification (R-137). On a match N answers "deny" with the text "This
   request contains a secret and cannot be shown for approval", writes an
   audit row, and stops here. No card.
5. N writes an approval record: the id of the card it is about to post,
   the task, the tool, and the SHA-256 of the input as canonical JSON. For
   Claude Code that is `input`; for a Codex command its `command` and `cwd`;
   for a Codex file change the `changes` of its item. The request's own id
   is left out: it is new on every attempt. N opens its wait before sending
   `post` with `kind: approval` and that id to H, signed for the AI, with
   the tool, the request text, the justification, the kind of request, and
   the list of approvers. Audit row `requested`. A request too long for a
   card is not refused: the card carries its beginning and says how long
   the whole is (`contracts/messages.md` 2.3). If H refuses the `post`, N
   deletes the record and answers "deny" with the text "The approval
   request was refused: <code>: <reason>", the error H answered.
6. H stores the card with `state.status = pending`, sets the task's wait to
   `approval`, and pushes the rows. The card appears in the main line under
   the secretary's name and mentions the approvers; the thread shows the
   request. H writes it; no AI turn is spent (R-42).
7. N keeps the CLI's request open for up to `limits.approval_wait_s`
   (default 600 seconds). It sends no progress notifications (section I).

## C. CLI multiple-choice questions (`decision` cards)

For Claude Code `AskUserQuestion` and Codex `item/tool/requestUserInput`:

1. N validates the question list and runs the outbound secret check on every
   question, header, option label and option description (and refuses any
   Codex question marked `isSecret`). On a secret match N writes an audit
   row (`result = secret`) and denies `AskUserQuestion` with the step-4
   secret message or answers `Ask.Reply(nil)` for Codex. No card.
2. For each question in order, N maps the CLI options to `decision` options
   `"A"` through `"Z"` (then `"27"`, …), opens its wait, and sends `post`
   with `kind: decision` (`main: true`, `thread` and `body.task` set when
   the session belongs to a task and omitted in a group session, `context`
   from a non-empty header, `to` set to the approvers, `mentions` set to
   their member ids). Audit row `requested`.
3. N waits up to `limits.approval_wait_s` for the `decided` item of that
   card. From `msg.state` (`status = decided`) N reads the chosen option's
   label (`choice` matched to `options`) and any free text (`text`), writes
   audit row `approval` (`result = decided`), and passes the answer back to
   the CLI (`updatedInput.answers` and optional `annotations` for Claude
   Code; `answers[id]` slice for Codex). If H refuses a card or the wait
   runs out, Claude Code receives the corresponding denial message and Codex
   receives `Ask.Reply(nil)`.

## D. Decided while the CLI is still waiting

8. The owner acts on the card (`approve` or `deny` on an `approval` card;
   `choose` with an option id and/or free text on a `decision` card). B
   signs `act` and sends it to H.
9. H checks the actor and pending state, updates `msg.state`, clears the
   wait, pushes rows, and queues a `decided` item for the AI carrying the
   updated `msg` row and no `cause` (slimming A-3).
10. N decodes the `decided` item and reads `msg.state` directly without
    re-verifying approver device signatures (slimming A-2). For an open
    approval wait, N also checks that the `pending` approval row still
    exists.
11. The item is not a turn: it goes straight to the waiting call. For an
    approval card, N answers the CLI "allow" or "deny" (`Denied by the
    owner.`), deletes the record, and writes an audit row with the decision
    and decider (`state.act.k` when present, else `state.by`, else H). The
    AI continues in the same turn.

## E. Approval not decided in time

12. The wait runs out. N answers the CLI "deny" with the text: "No decision
    yet. The request is still open. Stop this action and end your turn; you
    will be told when it is decided."
13. The turn ends. The task keeps its wait; the card stays pending; the
    record stays. The session may go dormant, and N may restart.
14. When the owner approves later, H sends the `decided` item. If N records
    it through `RecordApprovalDecision`, N writes `delivered` and marks the
    record `granted` in one transaction; if delivery hands the `decided`
    item straight to a turn, the record remains `pending` until the AI
    retries the same request and step 3 confirms `state.status = approved`
    on H via `fetch`. Either way, the one retry is allowed without posting a
    second card.
15. When the owner denies later, `RecordApprovalDecision` deletes the record
    in one transaction with `delivered` (or step 3 deletes the stale record
    on retry).

A granted or fetched-approved record allows one request, the one whose input
has the recorded digest. Records of a task are deleted when the task is
given to another AI or ends.

## F. Reassigned or cancelled

16. The secretary may give the task to another AI while the card is pending
    (R-42). H sets the card's `state.status = reassigned`; the first AI's
    task session ends (`flows/task.md`, "Reassignment").
17. If the task is cancelled or merged while the card is pending, H sets
    the card to `expired` (`contracts/task-machine.md` rows 8 and 15).

## G. What is recorded on the owner's node (R-139)

One audit row per request and per decision: time, group, AI, request text or
question as shown on the card, who decided, result (`approved`, `denied`,
`decided` or `secret`).

## H. Levels at a glance

| Level | The CLI is started with | Requests |
| --- | --- | --- |
| 1 read only | settings that allow no change | permission requests denied by N without a card |
| 2 workspace | the CLI's sandbox and permission rules for the task's worktree | card, owner decides |
| 3 unrestricted (default) | the CLI's highest-autonomy mode (2026-11-21 owner override 2) | any permission request or multiple-choice question the CLI still raises becomes a card |

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

## I. What the spikes showed, and what is still open

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
