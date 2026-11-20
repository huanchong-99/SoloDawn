# Flow: the owner takes the terminal and hands it back

Contracts: `contracts/commands.md` (`takeover.start`, `takeover.end`),
`contracts/datachannel.md` (terminal channel), `contracts/drivers.md` 1.3
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
| a working AI with no unfinished task | a new task session: N first sends `task start` with `origin: takeover`, and creates the session as in `flows/task.md` step 6. Claude Code is then started in the terminal with the new session id. Codex cannot be: it takes no id and does not keep a thread that has had no turn `[verified]`, so N first delivers the task's `assign` turn in structured mode (for a task of origin `takeover` it says only that the owner is about to work with the AI in the terminal) and takes over when it has ended |
| the secretary | its group session, in the project clone, still read only: the secretary changes no file, in the terminal either (R-21) |

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
   folder, with S's session id and with the same level, the same setting
   sources and the same MCP configuration as the structured session
   (`contracts/drivers.md` 1.3). What the CLI would have asked through a
   card it now asks O in the terminal. State `takeover`; N reports
   `member status`; the task card shows "owner is operating directly".
7. N answers `ok`. B opens the data channel `pty:<g>:<member>` and sends
   `pty.resize`. From here bytes flow both ways untouched.
8. If the folder has never been trusted in the interactive CLI, the CLI asks
   O in the terminal. N does not answer for O. Claude Code asks on its first
   interactive start in each folder, so on the first takeover of every
   worktree; Codex does not ask `[verified]`.

## C. During takeover

9. Inbox items for this AI in this group are held. `host.info` shows how
   many are waiting; the terminal page shows that count.
10. Whatever O and the AI do stays in the task's worktree and branch.
11. N does not read, store or interpret terminal bytes (R-85).

## D. Handing back

12. O taps "hand back" (`takeover.end`), or closes the terminal, or the
    channel closes for any reason. All three are the same event. A closed
    tab is seen at once when the browser closes the channel in order, and
    otherwise when N's own stack reports the connection lost.
13. N ends the interactive CLI and waits for it to exit.
14. N computes the facts: commits made since step 5, files changed, lines
    added and removed.
15. If the task was created by this takeover and nothing changed (no new
    commit, no modified file), N sends `task cancel`, the session ends, and
    nothing is posted. The flow stops here.
16. N starts the structured CLI again with the same session id and delivers
    one `handback` turn: "Your owner worked with you directly in the
    terminal. Post a short account of what was asked and what was done
    (`post`, kind `handback`). Then continue the task: if the work is
    complete, run the checks and submit; otherwise post what remains."
17. The AI posts it. N adds the facts from step 14 to `body` before
    signing. H stores the message in the thread and notifies the secretary.
18. State `waiting`. Held items are delivered next.

For the secretary, steps 16 to 18 apply without the sentence about the task,
and the summary goes to the main line. Its session is read only in the
terminal as well, so there are no changed files to report.

## E. Failure cases

| Event | Result |
| --- | --- |
| the interactive CLI cannot start or resume the session | N answers `takeover.start` with the CLI's error text and returns to the structured state |
| the pseudo-terminal process exits by itself (O typed the CLI's exit command) | same as step 12 |
| N restarts during takeover | on start, a session recorded as `takeover` is treated as handed back from step 14 |

## F. What spike S-4 showed, and what is still open

- `[verified]` On both CLIs, at levels 1, 2 and 3: structured, then the
  terminal, then structured again on one session id, history intact both
  ways; the level holds in the terminal; project hooks stay off for Claude
  Code; the MCP token names the right session in each phase. The commands
  are in `contracts/drivers.md` 1.3.
- `[verified]` Level 1 in the terminal: reading and the node's tools work
  and no file changes. Codex keeps its shell there too (it can read and
  cannot write), which the secretary's terminal therefore has.
- `[verified]` A session that does not exist yet can be started in the
  terminal by Claude Code and not by Codex (section A, third row).
- `[unverified]` Whether trusting a parent folder covers Claude Code's
  worktrees below it; `project_doc_max_bytes=0` on Codex's interactive
  CLI.
- `[unverified]` Pseudo-terminal behaviour on Windows (ConPTY) with both
  CLIs. Spike S-5.

## G. Watching (step 2)

Someone other than the owner may ask to watch. The owner confirms on their
own device; the watcher's browser then opens a receive-only terminal channel
to N. Not part of step 1.
