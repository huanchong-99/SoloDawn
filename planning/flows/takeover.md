# Flow: the owner takes the terminal and hands it back

Contracts: `contracts/commands.md` (`takeover.start`, `takeover.end`),
`contracts/datachannel.md` (terminal channel), `contracts/drivers.md` 1.3
(interactive commands). Rules: R-80 to R-85, R-124, R-12.

Actors: O, the owner; B, O's browser; N, the host node; S, the session that
is taken over; H, the home.

## A. Which session

Every session of an AI in a group runs in the member's workspace, so a
takeover is always in that folder (R-83). Which session's history the
terminal continues:

| The AI is | S is |
| --- | --- |
| a working AI, and `takeover.start` names a task it is assigned | that task session |
| a working AI, and `takeover.start` names a task it reviews | that review session, at the same level as its structured session (default 3 in 1.0, told by its instructions not to edit files; `contracts/drivers.md` sections 1 and 4). The hand-back turn leaves out the sentence about the task, and no task command is sent |
| a working AI, no task named, with an unfinished task that is not queued | the task session it used last |
| a working AI with no such task | its group session. No task is created (the owner's decision of 2026-11-26): the node reads no facts from the folder, so it could not tell afterwards whether anything changed and whether a task was needed. A group session that has never run is started in the terminal by a CLI that can start a new session there, with the new session id. For one that cannot (`contracts/drivers.md` 1.3 says which) N first delivers one turn in structured mode, which says only that the owner is about to work with the AI in the terminal, and takes over when it has ended |
| the secretary | its group session, at the same level as its structured session (default 3 in 1.0); its instructions say not to edit files (R-21, `contracts/drivers.md` sections 1 and 4) |

A queued task is left untouched: a takeover gives it no session and does
not start it, and a `takeover.start` that names one is answered `conflict`.

## B. Taking over

1. O taps "open terminal" on the card of one of their own AIs. The button is
   disabled unless B's path to N is `direct` (`flows/connect.md` step 8).
2. B sends the signed host command `takeover.start` to N.
3. N checks the signer is its owner and picks S (section A). It answers
   `busy` while another takeover of this AI is open, or N is giving the AI a
   turn that has not begun. If the AI is running a turn in this group, N
   interrupts it and waits for its end.
4. N closes the structured CLI process of S, if one is open, and waits for
   it to exit (C-6).
5. N checks that S's folder exists (`contracts/local.md` section 7); a
   folder that is gone is the first row of section E. N notes nothing about
   the folder: it reads no Git facts, before or after.
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
   count: the node pushes no `host.info` for it (`issues.md`).
10. Whatever O and the AI do is done in the member's workspace, like any
    work of that AI.
11. N does not read, store or interpret terminal bytes (R-85).

## D. Handing back

12. O taps "hand back" (`takeover.end`), or closes the terminal, or the
    channel closes for any reason. All three are the same event. A closed
    tab is seen at once when the browser closes the channel in order, and
    otherwise when N's own stack reports the connection lost.
13. N ends the interactive CLI by closing its pseudo-terminal and waits for
    it to exit, at most 10 s (`flows/agent-turn.md` G).
14. N starts the structured CLI again with the same session id and delivers
    one `handback` turn: "Your owner worked with you directly in the
    terminal. Post a short account of what was asked and what was done
    (`post`, kind `handback`). Then continue the task: if the work is
    complete, submit; otherwise post what remains."
15. The AI posts it: the account is the AI's own, and its body is `{}`
    (`contracts/messages.md` 2.10). H stores the message in the thread and
    notifies the secretary. If the AI has not posted it by the end of the
    hand-back turn, N posts the message itself: `post` kind `handback`,
    signed for the AI, with an empty `text`.
16. State `waiting`. Held items are delivered next.

For a group session, the secretary's or a working AI's, and for a review
session, steps 14 to 16 apply without the sentence about the task. The
account of a group session goes to the main line, and no item follows it.

## E. Failure cases

| Event | Result |
| --- | --- |
| the interactive CLI cannot start or resume the session, or S's folder is gone | N answers `takeover.start` with `cli` and the CLI's error text, for a missing folder a text that names it; S becomes `dormant` |
| the pseudo-terminal process exits by itself (O typed the CLI's exit command) | same as step 12 |
| the path of B or of N becomes `relay` during takeover | that end closes the `pty` channel (`flows/connect.md` step 9): same as step 12 |
| N restarts during takeover | on start, a session recorded as `takeover` is treated as handed back from step 14 |

## F. What spike S-4 showed, and what is still open

- `[verified]` On Claude Code and Codex, at levels 1, 2 and 3: structured, then
  the terminal, then structured again on one session id, history intact both
  ways, with the level holding in the terminal and the MCP token naming
  the right session in each phase. The commands, and what each CLI does
  and cannot do in the terminal, are in `contracts/drivers.md` 1.3. OpenCode,
  Qwen Code and external AIs declare `takeover: "none"`
  (`contracts/step-3/drivers.md` section 4, `contracts/step-3/external.md`).
- `[unverified]` Pseudo-terminal behaviour on Windows (ConPTY) with real
  CLIs. Spike S-5 measured ConPTY with another program, and `HS-19` wired
  `go-pty` in `internal/host/pty_*.go`; real-machine checks are tracked in
  `docs/cookbook/i11-real-machine-checklist.md`.

## G. Watching (step 2)

Someone other than the owner may ask to watch. The owner confirms on their
own device; the watcher's browser then opens a receive-only terminal channel
to N (`contracts/step-2/watch-team.md`, `internal/host/watch.go`,
`web/src/components/watch/`).
