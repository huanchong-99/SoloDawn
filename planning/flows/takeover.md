# Flow: the owner takes the terminal and hands it back

Contracts: `contracts/commands.md` (`takeover.start`, `takeover.end`),
`contracts/datachannel.md` (terminal channel), `contracts/local.md`
(interactive commands). Rules: R-80 to R-85, R-124, R-12.

Actors: O, the owner; B, O's browser; N, the host node; S, the session that
is taken over; H, the home.

## A. Which session

Code changed during takeover must be on a task branch (R-83). Task sessions
already live in a task's worktree, so the rule is:

| The AI is | S is |
| --- | --- |
| a working AI, and `takeover.start` names a task it is assigned | that task session |
| a working AI, no task named, with an unfinished task | the task session it used last |
| a working AI with no unfinished task | a new task session: N first sends `task start` with `origin: takeover`, and creates the session as in `flows/task.md` step 6 |
| the secretary | its group session, in the project clone |

## B. Taking over

1. O taps "open terminal" on the card of one of their own AIs. The button is
   disabled when the path from B to N is `relay`.
2. B sends the signed host command `takeover.start` to N.
3. N checks the signer is its owner and picks S (section A). If the AI is
   running a turn in this group, N interrupts it and waits for its end.
4. N closes the structured CLI process of S, if one is open, and waits for
   it to exit (C-6).
5. N notes, for the hand-back summary, the current commit and the list of
   uncommitted files in S's folder.
6. N starts the vendor's interactive CLI in a pseudo-terminal, in S's
   folder, with S's session id. State `takeover`; N reports
   `member status`; the task card shows "owner is operating directly".
7. N answers `ok`. B opens the data channel `pty:<g>:<member>` and sends
   `pty.resize`. From here bytes flow both ways untouched.
8. If the folder has never been trusted in the interactive CLI, the CLI asks
   O in the terminal. N does not answer for O.

## C. During takeover

9. Inbox items for this AI in this group are held. `host.info` shows how
   many are waiting; the terminal page shows that count.
10. Whatever O and the AI do stays in the task's worktree and branch.
11. N does not read, store or interpret terminal bytes (R-85).

## D. Handing back

12. O taps "hand back" (`takeover.end`), or closes the terminal, or the
    channel closes for any reason. All three are the same event.
13. N ends the interactive CLI and waits for it to exit.
14. N computes the facts: commits made since step 5, files changed, lines
    added and removed.
15. If the task was created by this takeover and nothing changed (no new
    commit, no modified file), N sends `task cancel`, the session ends, and
    nothing is posted. The flow stops here.
16. N starts the structured CLI again with the same session id and delivers
    one `handback` turn: "Your owner worked with you directly in the
    terminal. Call `handback_summary` with a short account of what was
    asked and what was done. Then continue the task: if the work is
    complete, run the checks and submit; otherwise post what remains."
17. The AI calls `handback_summary`. N sends `post` with `kind: handback`,
    the AI's text, and the facts from step 14 in `body`. H stores it in the
    thread and notifies the secretary.
18. State `waiting`. Held items are delivered next.

For the secretary, steps 16 to 18 apply without the sentence about the task,
and the summary goes to the main line. If the facts show changed files in
the project clone, they are listed in the summary and nothing else happens;
see `risks.md` K-10.

## E. Failure cases

| Event | Result |
| --- | --- |
| the interactive CLI cannot start or resume the session | N answers `takeover.start` with the CLI's error text and returns to the structured state |
| the pseudo-terminal process exits by itself (O typed the CLI's exit command) | same as step 12 |
| N restarts during takeover | on start, a session recorded as `takeover` is treated as handed back from step 14 |

## F. Not verified yet

- `[verified]` for Codex: structured to interactive and back on one session
  id, history intact both ways.
- `[documented]`, not tested: the same for Claude Code. The test was
  stopped by the workspace trust prompt. Spike S-4.
- `[unverified]` Starting a brand-new session directly in interactive mode
  with an id chosen by the node, for both CLIs (section A, third row).
  Spike S-4. If a CLI cannot, N creates the session with one empty
  structured start first.
- `[unverified]` Pseudo-terminal behaviour on Windows (ConPTY) with both
  CLIs. Spike S-5.

## G. Watching (step 2)

Someone other than the owner may ask to watch. The owner confirms on their
own device; the watcher's browser then opens a receive-only terminal channel
to N. Not part of step 1.
