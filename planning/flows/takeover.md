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
| a working AI, and `takeover.start` names a task it reviews | that review session, still at level 1. The hand-back turn leaves out the sentence about the task, and no task command is sent |
| a working AI, no task named, with an unfinished task that is not queued | the task session it used last |
| a working AI with no such task | a new task session: N first sends `task start` with `origin: takeover`, the title "Owner takeover" and an empty plan, and creates the session as in `flows/task.md` step 6. A CLI that can start a new session in the terminal is started there with the new session id. For one that cannot (`contracts/drivers.md` 1.3 says which) N first delivers the task's `assign` turn in structured mode (for a task of origin `takeover` it says only that the owner is about to work with the AI in the terminal) and takes over when it has ended |
| the secretary | its group session, in the project clone, still read only: the secretary changes no file, in the terminal either (R-21) |

A queued task is left untouched: a takeover gives it no worktree and no
session, and a `takeover.start` that names one is answered `conflict`.

## B. Taking over

1. O taps "open terminal" on the card of one of their own AIs. The button is
   disabled unless B's path to N is `direct` (`flows/connect.md` step 8).
2. B sends the signed host command `takeover.start` to N.
3. N checks the signer is its owner and picks S (section A). It answers
   `busy` while another takeover of this AI is open or a merge of one of
   its tasks is running. If the AI is running a turn in this group, N
   interrupts it and waits for its end.
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
   `pty.resize`. From here bytes flow both ways untouched. If B has not
   opened the channel 60 s after `ok`, N hands back as in step 12.
8. If the folder has never been trusted in the interactive CLI, the CLI asks
   O in the terminal. N does not answer for O. When each CLI asks is in
   `contracts/drivers.md` 1.3.

## C. During takeover

9. Inbox items for this AI in this group are held. The terminal page and
   the AI's card say that messages to the AI wait until hand-back, without a
   count: the node pushes no `host.info` (`issues.md`).
10. Whatever O and the AI do stays in the task's worktree and branch.
11. N does not read, store or interpret terminal bytes (R-85).

## D. Handing back

12. O taps "hand back" (`takeover.end`), or closes the terminal, or the
    channel closes for any reason. All three are the same event. A closed
    tab is seen at once when the browser closes the channel in order, and
    otherwise when N's own stack reports the connection lost.
13. N ends the interactive CLI by closing its pseudo-terminal and waits for
    it to exit, at most 10 s (`flows/agent-turn.md` G).
14. N computes the facts: commits made since step 5, files changed, lines
    added and removed (`contracts/local.md` section 7, hand-back facts).
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
    If the AI has not posted it by the end of the hand-back turn, N posts
    the facts itself: `post` kind `handback`, signed for the AI, with an
    empty `text`.
18. State `waiting`. Held items are delivered next.

For the secretary and for a review session, steps 16 to 18 apply without
the sentence about the task; the secretary's summary goes to the main line.
Both sessions are read only in the terminal as well, so there are no
changed files to report.

## E. Failure cases

| Event | Result |
| --- | --- |
| the interactive CLI cannot start or resume the session | N answers `takeover.start` with `cli` and the CLI's error text; S becomes `dormant`, and a task this takeover created is cancelled (`task cancel` by the host) |
| the pseudo-terminal process exits by itself (O typed the CLI's exit command) | same as step 12 |
| the path of B or of N becomes `relay` during takeover | that end closes the `pty` channel (`flows/connect.md` step 9): same as step 12 |
| N restarts during takeover | on start, a session recorded as `takeover` is treated as handed back from step 14 |

## F. What spike S-4 showed, and what is still open

- `[verified]` On both CLIs, at levels 1, 2 and 3: structured, then the
  terminal, then structured again on one session id, history intact both
  ways, with the level holding in the terminal and the MCP token naming
  the right session in each phase. The commands, and what each CLI does
  and cannot do in the terminal, are in `contracts/drivers.md` 1.3.
- `[unverified]` Whether trusting a parent folder covers Claude Code's
  worktrees below it.
- `[unverified]` Pseudo-terminal behaviour on Windows (ConPTY) with both
  CLIs. Spike S-5 measured ConPTY with another program; go-pty is measured
  in HS-19 (`risks.md` T-4).

## G. Watching (step 2)

Someone other than the owner may ask to watch. The owner confirms on their
own device; the watcher's browser then opens a receive-only terminal channel
to N. Not part of step 1.
