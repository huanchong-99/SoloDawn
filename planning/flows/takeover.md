# Flow: the owner takes the terminal and hands it back

Contracts: `contracts/commands.md` (`takeover.start`, `takeover.end`),
`contracts/datachannel.md` (terminal channel), `contracts/local.md`
(interactive commands). Rules: R-80 to R-85, R-124.

Actors: O, the owner; B, O's browser; N, the host node; S, the session of
(AI, group); H, the home.

## A. Taking over

1. O taps "open terminal" on the card of one of their own AIs. The button is
   disabled when the path from B to N is `relay`.
2. B sends the signed host command `takeover.start` to N.
3. N checks the signer is its owner. If S is `working`, N interrupts the
   turn and waits for its end.
4. N closes the structured CLI process and waits for it to exit (C-6).
5. N notes, for the hand-back summary, the current commit and the list of
   uncommitted files in the AI's current worktree (the project folder if
   there is no task).
6. N starts the vendor's interactive CLI in a pseudo-terminal, in the same
   folder, resuming the same session id. State `takeover`; N reports
   `member status`; the task card shows "owner is operating directly".
7. N answers `ok`. B opens the data channel `pty:<g>:<member>` and sends
   `pty.resize`. From here bytes flow both ways untouched.
8. If the folder has never been trusted in the interactive CLI, the CLI asks
   O in the terminal. N does not answer for O.

## B. During takeover

9. Inbox items for S are held. `host.info` shows how many are waiting; the
   terminal page shows that count.
10. Whatever O and the AI do stays in the task's worktree and branch.
11. N does not read, store or interpret terminal bytes (R-85).

## C. Handing back

12. O taps "hand back" (`takeover.end`), or closes the terminal, or the
    channel closes for any reason. All three are the same event.
13. N ends the interactive CLI and waits for it to exit.
14. N computes the facts: commits made since step 5, files changed, lines
    added and removed.
15. If code changed and the AI had no task, N sends `task start` with
    `origin: takeover` (R-83).
16. N starts the structured CLI again with the same session id and delivers
    one `handback` turn: "Your owner worked with you directly in the
    terminal. Call `handback_summary` with a short account of what was
    asked and what was done."
17. The AI calls `handback_summary`. N sends `post` with `kind: handback`,
    the AI's text, and the facts from step 14 in `body`. H stores it in the
    thread and notifies the secretary.
18. State `waiting`. Held items are delivered as the next turn.

## D. Failure cases

| Event | Result |
| --- | --- |
| the interactive CLI cannot resume the session | N answers `takeover.start` with the CLI's error text and returns to the structured state |
| the pseudo-terminal process exits by itself (O typed the CLI's exit command) | same as step 12 |
| N restarts during takeover | on start, a session recorded as `takeover` is treated as handed back from step 14 |

## E. Not verified yet

- `[verified]` for Codex: structured to interactive and back on one session
  id, history intact both ways.
- `[documented]`, not tested: the same for Claude Code. The test was
  stopped by the workspace trust prompt. Spike S-4.
- `[unverified]` Pseudo-terminal behaviour on Windows (ConPTY) with both
  CLIs. Spike S-5.

## F. Watching (step 2)

Someone other than the owner may ask to watch. The owner confirms on their
own device; the watcher's browser then opens a receive-only terminal channel
to N. Not part of step 1.
