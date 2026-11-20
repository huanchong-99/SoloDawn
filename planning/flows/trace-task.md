# Trace: one confirmed task on the wire

One task from request to merge, message by message, with who signs what and
which commit each step is about. It follows `contracts/task-machine.md`
rows 1, 3 to 8.

This file is documentation: payloads are abridged. The golden vector is
slice (c) of `steps/step-1-backend.md` B-01: complete payloads with group,
nonce and time on each, real ids and signatures, the exact bytes of the
decisions, every response with its request id, and the variant without a
remote. Once it exists, this file links to it instead of repeating
payloads.

Cast: the owner's browser, device `OB`. Node A, device `HA`: home of group
`G`; hosts the secretary `mS` and the AI `mX`. Node B, device `HB`: hosts
the AI `mY`. All three devices are in the owner's log. The group has a
GitHub remote and base branch `main`. Payloads are shown without `n` and
`ts`, and envelopes as `signed by`.

## The steps

| # | From → to | Signed by | Payload (abridged) | The home then |
| --- | --- | --- | --- | --- |
| 1 | browser → home, `cmd` | `OB` | `{"t":"post","g":"G","kind":"text","text":"Show the connection path…"}` (id `M1`) | stores `M1`; item 1 for `mS` |
| 2 | secretary → home, `cmd` | `HA`, `as: mS` | `{"t":"post","kind":"brief","body":{"title":"…","criteria":[…],"tasks":[{"title":"Node: report path changes","assignee":"mX","reviewer":"mY"}],"pass":90,"requester":"<owner>"}}` (id `M2`) | stores the card, pending |
| 3 | browser → home, `cmd` | `OB` | `{"t":"act","g":"G","msg":"M2","action":"confirm"}` (nonce `C1`) | card confirmed; task `t1` (T-12) `active`/`branch`; `task` message `M3`; receipt for (`OB`, `C1`); item 2 for `mX`; item 3 for `mS` |
| 4 | host A → home, `cmd` | `HA` | `{"t":"task","op":"branch","id":"t1","branch":"task/12-node-report-path-changes"}` | step `local_check` |
| 5 | AI `mX` → home, `cmd` | `HA`, `as: mX` | `{"t":"post","kind":"progress","thread":"t1","text":"Branch created…"}` | stores it |
| 6 | host A → home, `cmd` | `HA` | `{"t":"task","op":"checked","id":"t1","ok":true,"commit":"c1","items":[…]}` | `checks.local`; a `checks` message |
| 7 | AI `mX`, then host A → home | `HA` | `{"t":"post","kind":"report","thread":"t1",…}` (`as: mX`), then `{"t":"task","op":"submitted","id":"t1","commit":"c1","pr":31,"pr_url":"…","stats":{…}}` | `commit` = `c1`; step `cloud_check` |
| 8 | host A → home, `cmd` | `HA` | `{"t":"task","op":"cloud","id":"t1","commit":"c1","ok":true,"items":[…]}` | `checks.cloud`; step `review`; `failures` = 0; item 4 for `mY` |
| 9 | AI `mY` → home, `cmd` | `HB`, `as: mY` | `{"t":"post","kind":"review","thread":"t1","body":{"task":"t1","commit":"c1","score":93,"items":[…]}}`; answered `{"ok":true,"passed":true,"round":1}` | step `merge`; item 5 for host A |
| 10 | host A → home, `cmd` | `HA` | `{"t":"task","op":"merged","id":"t1","commit":"c1","merge_commit":"m1","stats":{…}}` | task `merged`; `system` `task_merged`; item 6 for `mS` |
| 11 | secretary → home, `cmd` | `HA`, `as: mS` | `{"t":"post","kind":"summary","thread":"t1","text":"Path changes are reported…"}` | fills the figures; summary card |

## The items, in full

Each is an envelope signed by the home `HA` and addressed to one host.

Item 2, the assignment (after step 3). `cause` is the owner's confirmation
from step 3, unchanged:

```json
{"env":{"p":"{\"t\":\"item\",\"g\":\"G\",\"to\":\"HA\",\"item\":2,\"ai\":\"mX\",\"why\":\"assign\",\"task\":\"t1\",\"msg\":\"M3\"}","k":"HA","s":"…"},
 "msg":{"r":"msg","id":"M3","kind":"task","by":"system","body":{"task":"t1"}},
 "task":{"r":"task","id":"t1","num":12,"state":"active","step":"branch","assignee":"mX","reviewer":"mY","brief":"M2"},
 "cause":{"p":"{\"t\":\"act\",\"g\":\"G\",\"msg\":\"M2\",\"action\":\"confirm\"}","k":"OB","s":"…"}}
```

Item 4, the review request (after step 8), to host B, naming the commit:

```json
{"env":{"p":"{\"t\":\"item\",\"g\":\"G\",\"to\":\"HB\",\"item\":4,\"ai\":\"mY\",\"why\":\"review\",\"task\":\"t1\",\"commit\":\"c1\"}","k":"HA","s":"…"},
 "task":{"r":"task","id":"t1","num":12,"step":"review","commit":"c1","pr":31}}
```

Item 5, the merge (after step 9), to host A itself, naming the commit:

```json
{"env":{"p":"{\"t\":\"item\",\"g\":\"G\",\"to\":\"HA\",\"item\":5,\"ai\":\"mX\",\"why\":\"merge\",\"task\":\"t1\",\"commit\":\"c1\",\"pr\":31}","k":"HA","s":"…"},
 "task":{"r":"task","id":"t1","num":12,"step":"merge","commit":"c1","pr":31}}
```

Items 1, 3 and 6 are `mention`, `confirmed` and `notice` for the secretary,
with the same envelope shape and the message that caused them: `M1`, `M2`,
and the `system` message `task_merged`. Item 3 also carries the
confirmation of step 3 as its `cause`; item 6 has `code` `task_merged` and
`task` `t1` (`contracts/datachannel.md` section 6). Every envelope is built
and signed when its item is queued, and sent as it was signed.

## What each host checks before acting

| Item | The host checks | Then |
| --- | --- | --- |
| 2 | the envelope is signed by `HA`, the home of `G`; `HA` is in the owner's log; `to` is this host; `mX` runs here | creates the worktree and the task session; step 4 |
| 4 | the same, for `HB` and `mY` | checks out exactly `c1` for the review session |
| 5 | the same; and the task, as the host sees it now, is still at `merge` for `c1` | runs `gh pr merge 31 --squash --delete-branch --match-head-commit c1`, with the pull request number from the signed item |

The `task` and `msg` rows inside an item are what the turn is written from.
They carry no authority: a host never acts because a row says so.

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

Steps 7 and 8 collapse: `submitted` carries no `pr`, there is no `cloud`
step, and the home goes straight to `review`. Host B is then necessarily
host A, the review checkout is taken from the local clone, and step 10 is a
squash merge of `c1` into `main` in that clone, done only if the task
branch's head is still `c1`.
