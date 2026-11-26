# Trace: one confirmed task on the wire

One task from request to done, message by message, with who signs what and
which report each step is about. It follows `contracts/task-machine.md`
rows 1, 5 and 7.

This file is documentation: payloads are abridged. The complete signed golden
transcript is `internal/proto/testdata/trace-local.json`.
`internal/home/trace_test.go` produces it by sending its signed commands
through the home and fails when it differs from its output;
`web/src/lib/trace.test.ts` verifies it on its own. It pins the device
keys, owner device log, initial group and member rows, sequence and task
counters, every command envelope and `re`-paired response, the resulting
`rows` pushes, and all five inbox items with their host `ack`s. The second
transcript, `trace-remote.json`, went with the forge: there is one flow,
with or without a remote, because the platform runs no Git (the owner's
decision of 2026-11-26).

Cast: the owner's browser, device `OB`. Node A, device `HA`: home of group
`G`; hosts the secretary `mS` and the AIs `mX` and `mY`, each added with a
folder on that computer (`workspace` on its member row). Both devices are
in the owner's log. Payloads below are shown without `n` and `ts`, and
envelopes as `signed by`.

## The steps

| # | From → to | Signed by | Payload (abridged) | The home then |
| --- | --- | --- | --- | --- |
| 1 | browser → home, `cmd` | `OB` | `{"t":"post","g":"G","kind":"text","text":"Show the connection path…"}` (id `M1`) | stores `M1`; item 1 for `mS` |
| 2 | secretary → home, `cmd` | `HA`, `as: mS` | `{"t":"post","kind":"brief","body":{"title":"…","criteria":[…],"tasks":[{"title":"Node: report path changes","assignee":"mX","reviewer":"mY"}],"pass":90,"requester":"<owner>"}}` (id `M2`) | stores the card, pending |
| 3 | browser → home, `cmd` | `OB` | `{"t":"act","g":"G","msg":"M2","action":"confirm"}` (nonce `C1`) | card confirmed (`state.act` keeps step 3's envelope); task `t1` (T-12) `active`/`work`, its folder being free; the `task` message, whose id is `t1`; receipt for (`OB`, `C1`); item 2 for `mX`; item 3 for `mS` |
| 4 | AI `mX` → home, `cmd` | `HA`, `as: mX` | `{"t":"post","kind":"progress","thread":"t1","text":"Plan: add the event…"}` | stores it |
| 5 | AI `mX` → home, `cmd` | `HA`, `as: mX` | `{"t":"post","kind":"report","thread":"t1","text":"Path events are sent on every switch. Ran the tests: 42 passed. Commit 9f2c1ab on branch t12-path-events.","body":{}}` (id `M3`) | `report` = `M3`; step `review`; `failures` = 0; item 4 for `mY` |
| 6 | AI `mY` → home, `cmd` | `HA`, `as: mY` | `{"t":"post","kind":"review","thread":"t1","body":{"task":"t1","report":"M3","score":93,"items":[…]}}`; answered `{"ok":true,"passed":true,"round":1}` | task `done`; `system` `task_done`; item 5 for `mS` |
| 7 | secretary → home, `cmd` | `HA`, `as: mS` | `{"t":"post","kind":"summary","thread":"t1","text":"Path changes are reported…"}` | fills the figures; summary card |

No host command moves the task: there is no branch, no check and no merge
to report. What `mX` did in its folder between steps 3 and 5 is its own
CLI's work and is on no wire.

## The items

By the owner's decision of 2026-11-21 (no attack-and-defence code,
`contracts/datachannel.md` section 6), each inbox entry carries a plain
unsigned `item` object (`item`, `ai`, `why` and the fields of its `why`),
with no `env` wrapper and no `cause`, `g` or `to`, alongside copies of the
`msg` and `task` rows it names as of delivery:

- Item 1 (`mention` for `mS`, to `HA`): names `M1`, with a copy of `M1`.
- Item 2 (`assign` for `mX`, to `HA`): names task `t1` and its `task`
  message, whose id is also `t1`, with copies of both rows; the owner's
  signed confirmation lives on the confirmed brief row (`M2.state.act`).
- Item 3 (`confirmed` for `mS`, to `HA`): names `M2`, with a copy of the
  confirmed brief row.
- Item 4 (`review` for `mY`, to `HA`): names `t1` and the report `M3`, with
  copies of both rows.
- Item 5 (`notice` for `mS`, to `HA`): names `t1`, `code` `task_done` and
  the `task_done` `system` message, with copies of that message and `t1`.

Exact wire JSON for all five items and their `ack` frames is in
`internal/proto/testdata/trace-local.json`.

## What each host checks before acting

| Item | The host checks | Then |
| --- | --- | --- |
| 2 | the frame comes from `HA`, the home of `G`; `mX` runs here; when taken, `t1` is active for `mX` | closes the task sessions of `G` whose task is final or has another assignee; checks that `mX`'s folder exists; creates the task session there |
| 4 | the frame comes from `HA`; `mY` runs here; when taken, `t1` is active at `review` for `mY` and its `report` is `M3` | creates the review session in `mY`'s folder and keeps `M3` with it (`contracts/storage.md` section 3) |

The `task` and `msg` rows inside an item are what the turn is written from:
for item 4, the brief, the report `M3` as `mX` wrote it, and where the work
is, read from `mX`'s member row (`contracts/drivers.md` section 3).

## Where one report is enforced

| Step | Check | By |
| --- | --- | --- |
| 5 | `thread` is `t1`, and `as` is its assignee | the home |
| 6 | the review names the report the session was given when item 4 was delivered | host A, which adds `body.report` from the session's row |
| 6 | `body.report` = the task's `report` | the home |

If the assignee submits again before the review, step 5 repeats with a new
report: the home retires item 4, queues a `review` item for the new report,
and refuses a review that still names `M3`.

The home checks nothing about a folder, a branch or a commit: it cannot.
What the report says about them is the assignee's statement
(`risks.md` K-30).

## Across two computers

With `mY` on a node B, device `HB`, the same commands travel, step 6 signed
by `HB`. Item 4 goes to `HB`, and its turn says that the work is on another
computer: the reviewer gets it the way the report says it is shared, for a
project with Git and a remote by fetching the branch the report names. The
platform moves no file between the two (R-111).
