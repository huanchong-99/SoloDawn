# Trace: one confirmed task on the wire

One task from request to merge, message by message, with who signs what and
which commit each step is about. It follows `contracts/task-machine.md`
rows 1, 3 to 8.

This file is documentation: payloads are abridged. The complete signed golden
transcripts are `internal/proto/testdata/trace-remote.json` (with a GitHub
remote) and `internal/proto/testdata/trace-local.json` (`forge: none`, no
remote). `internal/home/trace_test.go` produces them by sending their signed
commands through the home and fails when they differ from its output;
`web/src/lib/trace.test.ts` verifies them on their own. Each pins the device
keys, owner device log, initial group and member rows, sequence and task
counters, every command envelope and `re`-paired response, the resulting
`rows` pushes, and all six inbox items with their host `ack`s.

Cast: the owner's browser, device `OB`. Node A, device `HA`: home of group
`G`; hosts the secretary `mS` and the AI `mX`. Node B, device `HB`: hosts
the AI `mY`. All three devices are in the owner's log. The group has a
GitHub remote and base branch `main`. Payloads below are shown without `n`
and `ts`, and envelopes as `signed by`.

## The steps

| # | From → to | Signed by | Payload (abridged) | The home then |
| --- | --- | --- | --- | --- |
| 1 | browser → home, `cmd` | `OB` | `{"t":"post","g":"G","kind":"text","text":"Show the connection path…"}` (id `M1`) | stores `M1`; item 1 for `mS` |
| 2 | secretary → home, `cmd` | `HA`, `as: mS` | `{"t":"post","kind":"brief","body":{"title":"…","criteria":[…],"tasks":[{"title":"Node: report path changes","assignee":"mX","reviewer":"mY"}],"pass":90,"requester":"<owner>"}}` (id `M2`) | stores the card, pending |
| 3 | browser → home, `cmd` | `OB` | `{"t":"act","g":"G","msg":"M2","action":"confirm"}` (nonce `C1`) | card confirmed (`state.act` keeps step 3's envelope); task `t1` (T-12) `active`/`branch`; the `task` message, whose id is `t1`; receipt for (`OB`, `C1`); item 2 for `mX`; item 3 for `mS` |
| 4 | host A → home, `cmd` | `HA` | `{"t":"task","op":"branch","id":"t1","branch":"task/12-node-report-path-changes"}` | step `local_check` |
| 5 | AI `mX` → home, `cmd` | `HA`, `as: mX` | `{"t":"post","kind":"progress","thread":"t1","text":"Branch created…"}` | stores it |
| 6 | host A → home, `cmd` | `HA` | `{"t":"task","op":"checked","id":"t1","ok":true,"commit":"c1","items":[…]}` | `checks.local`; a `checks` message |
| 7 | AI `mX`, then host A → home | `HA` | `{"t":"post","kind":"report","thread":"t1",…}` (`as: mX`), then `{"t":"task","op":"submitted","id":"t1","commit":"c1","pr":31,"pr_url":"…","stats":{…}}` | `commit` = `c1`; step `cloud_check` |
| 8 | host A → home, `cmd` | `HA` | `{"t":"task","op":"cloud","id":"t1","commit":"c1","ok":true,"items":[…]}` | `checks.cloud`; a `checks` message; step `review`; `failures` = 0; item 4 for `mY` |
| 9 | AI `mY` → home, `cmd` | `HB`, `as: mY` | `{"t":"post","kind":"review","thread":"t1","body":{"task":"t1","commit":"c1","score":93,"items":[…]}}`; answered `{"ok":true,"passed":true,"round":1}` | step `merge`; item 5 for host A |
| 10 | host A → home, `cmd` | `HA` | `{"t":"task","op":"merged","id":"t1","commit":"c1","merge_commit":"m1","stats":{…}}` | task `merged`; `system` `task_merged`; item 6 for `mS` |
| 11 | secretary → home, `cmd` | `HA`, `as: mS` | `{"t":"post","kind":"summary","thread":"t1","text":"Path changes are reported…"}` | fills the figures; summary card |

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
- Item 4 (`review` for `mY`, to `HB`): names `t1` and `commit` `c1`, with a
  copy of `t1`.
- Item 5 (`merge` for `mX`'s host `HA`): names `t1`, `commit` `c1` and `pr`
  `31`, with a copy of `t1`.
- Item 6 (`notice` for `mS`, to `HA`): names `t1`, `code` `task_merged` and
  the `task_merged` `system` message, with copies of that message and `t1`.

Exact wire JSON for all six items and their `ack` frames is in
`internal/proto/testdata/trace-remote.json` and
`internal/proto/testdata/trace-local.json`.

## What each host checks before acting

| Item | The host checks | Then |
| --- | --- | --- |
| 2 | the frame comes from `HA`, the home of `G`; `mX` runs here; when taken, `t1` is active for `mX` | creates the worktree and the task session; step 4 |
| 4 | the frame comes from `HA`; `mY` runs here; when taken, `t1` is active at `review` for `mY` and `c1` | checks out exactly `c1` for the review session |
| 5 | the frame comes from `HA`; `mX` runs here; when taken, `t1` is active at `merge` for `mX` and `c1` | runs `gh pr merge 31 --squash --delete-branch --match-head-commit c1`, with the pull request number from the item |

The `task` and `msg` rows inside an item are what the turn is written from.

## Where one commit is enforced

| Step | Check | By |
| --- | --- | --- |
| 6 | the worktree is clean; the commit is its head | host A |
| 7 | the head is still the checked commit `c1` | host A, then the home (`checks.local.commit` = `commit`) |
| 8 | head and checks are read in one answer, and the head is `c1` | host A; the home refuses a result for another commit |
| 9 | the review checkout is `c1`; `body.commit` = the task's `commit` | host B, then the home |
| 10 | the forge merges only if the head is `c1`; nothing else may change the task between steps 9 and 10 | the forge, asked by host A; the home |

If the assignee submits again before the merge, steps 7 to 10 repeat with
the new commit, and anything still about `c1` is refused.

## The same without a remote

Steps 7 and 8 collapse (`internal/proto/testdata/trace-local.json`):
`submitted` carries no `pr` or `pr_url`, there is no `cloud` step, and the
home goes straight to `review`. Reviewer `mY` is a distinct AI on host A,
the review checkout is taken from the local clone, item 5 carries no `pr`,
step 10 is a squash merge of `c1` into `main` in that clone done only if the
task branch's head is still `c1`, and the summary card carries no `pr`.
