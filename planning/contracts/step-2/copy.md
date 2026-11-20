# Contract (step 2): copies of a group

A copy of a group kept on a second node for safekeeping, and restoring it
by hand when the home is lost (R-135, `risks.md` Q-12). With
`contracts/step-2/move.md` it settles design point D-11 of
`steps/parallel-plan-step-2-3.md`: the same snapshot, sent on a schedule,
read-only, and no live replication. The snapshot, the redirect and its
epoch, installing and finding a moved group are in `move.md`; this file
names their sections. No step 1 contract changes here (section 7).

Actors: P, the owner of the group; B, a browser of P; H1, the home; H2, the
node of P that holds the copy.

## 1. What a copy is

A snapshot of kind `copy` (`move.md` section 3) that H1 sent to H2, kept as
H2 received it. It is not a home: H2 serves nothing from it and no client
is told of it (A9.31). It becomes the group again only by a restore
(section 4), after the home is lost and revoked.

## 2. The schedule

Host commands (`contracts/commands.md` section 2), by P as every one:

| `t` | Fields | Effect |
| --- | --- | --- |
| `copy.set` | `g`, `node`, `minutes` | on the home of g, else `not_found`. `node` is another valid node of P and `minutes` at least 1, or `node` is `""` to stop; else `bad_request`. Replaces the group's schedule |
| `copy.drop` | `g` | on a node that holds a copy of g, else `not_found`: deletes it |
| `copy.restore` | `g`, `redirect` | section 4 |

- The home keeps the schedules in the `setting` key `copies`. It copies at
  once, then every `minutes`: a cut as `sync` takes it, while writing goes
  on, sent as a snapshot of kind `copy` with `snap` (`move.md` section 5).
  A run is skipped while another runs or the group moves, and when `seq`
  is unchanged since the last copy H2 stored.
- A move takes the schedule along in `head.copy`; the new home drops it
  when its `node` is that home itself (`move.md` section 6).
- `host.info` (`contracts/datachannel.md` section 7) gains a list `copies`,
  always present, one entry per schedule:
  `{"g":"Qm1…","node":"<device id>","minutes":1,"last":1763600000000}`.
  `last` is the `at` of the last copy stored, or `null`; `error`, optional,
  is the code of the last run when it failed, absent after a success.

## 3. The holder

- H2 keeps one copy per group, the file `copies/<g>.snap` in its data
  directory, written once the copy verified (`move.md` section 3).
- **Newer** means a higher `head.epoch`, or the same epoch and a higher
  `head.seq`. A copy that is not newer than the one held is refused with
  `conflict`, and the held one stays. So an old home that missed a move
  cannot replace the copy of the group's present home.
- The group is not homed on H2: no `welcome.groups` lists it, and `sync`
  and commands for it are refused as for any group not homed there.
- H2 deletes its copy of a group when it accepts or installs a redirect
  for that group whose `from` is the copy's `head.from`, and when it
  installs the group (`move.md` section 6). A copy is thus dropped once its
  home has moved on, wherever the holder learns it: as a client of the
  group, or as the node of a later move or restore.
- `host.info` of H2 gains a list `held`, always present, one entry per
  file, with `name` from the copy's group row and the rest from its head:

```json
{"g":"Qm1…","name":"Website","from":"<device id>","epoch":2,"seq":812,"at":1763600000000}
```

## 4. Restoring a copy

`copy.restore`, sent by B to H2, is checked in this order:

1. H2 holds a copy of g: else `not_found`.
2. `redirect` passes checks 1 to 3 of `move.md` section 2 for H2, so its
   `epoch` is above the copy's `head.epoch`; it has the command's signer,
   the copy's `head.from` as `from`, H2 as `home`, and the copy's `seq`:
   else `bad_request`.
3. `from` is revoked in P's log: else `conflict`. A home that may still run
   is moved, not restored; a revoked node stops being a home
   (`move.md` section 8), and its peers refuse it.
4. H2 is not the home of g: else `conflict`.
5. The copy verifies again, by checks 1 and 2 of `move.md` section 3 with
   `copy` as the kind: else `bad_request` or `bad_sig` (A9.32).

H2 then installs the copy (`move.md` section 6), with `restored: true`, and
deletes the file. Clients cannot reach `from`, look for the group among P's
nodes (`move.md` 7.2), find H2 with this redirect and sync from 0, since it
has `seq`. A client that holds a higher epoch, because it followed a later
move, refuses the redirect: a stale copy cannot pull it back.

## 5. In the browser

- **Copy**: in the group info, for the owner: the node and the interval,
  and from `copies` of the home's `host.info` the last copy's time and any
  error.
- **Restore** is normative for every browser of P, not only a screen rule.
  B sends `copy.restore` only when all of this holds: the copy's `from` is
  revoked in P's log; a look for the group by `move.md` 7.2, made just
  before, found no node of P that lists it; and B signs the redirect with
  an `epoch` above the copy's and above every epoch B holds for the group.
  Otherwise the page says why: the lost computer is not revoked yet, or
  the group has a home, which it names.

## 6. Limits

| | Limit | Consequence |
| --- | --- | --- |
| 1 | What the old home wrote after the cut is lost | items it acknowledged after the cut come again, and hosts drop those they took, by `first` (`move.md` section 6) |
| 2 | After a restore, the sequence numbers and task numbers that the lost home gave out after the cut are given out again | a task number may name two tasks over the group's life; a branch `task/<num>-<slug>` of a lost task may exist on the remote, and the push of the new task fails until a person removes it |
| 3 | Two holders, restored from two browsers while neither can reach the other's holder | each lookup of section 5 finds nothing; a client keeps the higher epoch it meets. The owner keeps one and revokes the other node, which drops the group (`move.md` section 8) |
| 4 | A holder that never learns of a move keeps its copy | the owner drops it with `copy.drop`. A restore of it is refused while its `from` is valid, and B's lookup finds the present home (section 5) |

## 7. Where this goes after step 1

| File | Gains |
| --- | --- |
| `contracts/commands.md` | section 2: `copy.set`, `copy.drop`, `copy.restore` |
| `contracts/datachannel.md` | section 7: `copies` and `held` in `host.info` |
| `contracts/storage.md`, `contracts/local.md` | `setting` key `copies`; data directory: `copies/<g>.snap` |
| `flows/` | `flows/moving.md` (of `move.md` section 9) gains copying and restoring |

P-4 freezes `copy.set`, `copy.drop`, `copy.restore`, and `copies` and
`held` in `host.info`, with the shapes of `move.md` section 9.

## 8. Not verified

- Nothing here was run.
- `[unverified]` What a copy every minute costs a busy home; the skip of
  section 2 bounds it for a quiet group only.
