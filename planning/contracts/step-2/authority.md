# Contract (step 2): authority across owners

Whose word a host accepts when a group's home belongs to someone else: the
host's rule and the list its owner signs (D-2), the level of a task that
another person caused (D-1), approver delegates (D-13), and the fork rule of
the device log with its vectors (D-4). The design points are in
`steps/parallel-plan-step-2-3.md` section 1. Rules: R-42, R-44, R-51, R-53,
R-115, R-136, R-138.

This file adds to the step 1 contracts; section 9 lists what is folded into
which file and what changed in other step 2 files. Presence follows
`contracts/conventions.md` section 2.

Not here, all in `contracts/step-2/`: when a home asks an owner, the digest
and the labels (`confirm.md`); the watch grant (`watch-team.md`); accepting
commands (`checks-forge.md`); what a browser checks (`browser-trust.md`).

## 1. Terms

A home orders and stores. From step 2 its signature on an item is not the
owner's instruction (R-136): a host also needs a signature that leads to
its owner. Below, X is an AI, N its host and O the owner of N.

- **The person of a signer**: the person in whose device log the signing
  device is valid when N judges. N reads O's log from its own store, and
  the logs of the other people of a group from its `device` rows, each of
  which carries its signed entry: a log proves itself from entry 1
  (`contracts/step-2/browser-trust.md` section 2). A signer in no such log
  has no person. No `member` row is read: names and roles are the home's.
- **As a person**: the payload has no `as`. **Through an AI**: it has one.
- **Listed for X**: named in the `allow` list of section 2 for X, while
  X's account (`agent.set`, R-50) is `team` or `api_key`. For a `personal`
  account nobody is listed (R-51).
- **A forwarded envelope** (`cause`, the `env` of `msg` and of `req`, a
  `body.start`) passes checks 1 and 3 of `contracts/envelope.md` section 3
  and names the item's group in `g`. Checks 2, 4 and 5 are not made.
- **The digest for X** of a request is that of
  `contracts/step-2/confirm.md` section 6. **The digest of a brief** is the
  same with an empty text in place of `body.ai`: a digest for no AI.

## 2. The list the owner signs

One host command (`contracts/commands.md` section 2), accepted as every
host command: from a device in the node's owner's log, with `to` the node.

```json
{"t":"allow.set","n":"…","ts":1763700000000,"to":"<device id>","g":"Qm1…",
 "agent":"claude","allow":["<user id>"],"delegates":["<user id>"]}
```

| Field | Value |
| --- | --- |
| `g`, `agent` | the AI: that CLI of this node in that group; `not_found` when it is no member there |
| `allow` | the people whose word is enough for this AI (R-53) |
| `delegates` | the people who may decide its approval cards besides the owner (R-42) |

- Both lists are always sent and replace what the node held; before the
  first command both are `[]`. An entry that is no user id: `bad_request`.
- The node keeps them in one more host table, `allow` (`agent`, `g`, the
  two lists; the row goes when the AI leaves the group), writes the audit
  row `setting`, and shows them in `host.info`: each entry of
  `agents[].groups` gains `allow` and `delegates`, always present. The
  owner's client reads and edits the lists there.
- The node then sends `member set` for that AI with `allow`, and with
  `approvers`: O, the node's `settings.approvers` and `delegates`. The row
  is a copy: the home reads it to decide whether to ask
  (`contracts/step-2/confirm.md` section 2) and who may decide a card
  (section 5), and other people's clients show it. No host reads it
  (A9.15), and a step 2 client sends neither field itself.
- `allow` is kept whatever the account type, with effect only while it is
  `team` or `api_key`. The lists give nothing else: `confirm` cards,
  `checks.accept`, levels and watching stay with the owner.

## 3. What a host accepts

N applies the checks of step 1 to every item (`contracts/datachannel.md`
section 6: the home's signature, `to`, `ai`), whoever owns the home. Then,
when it uses the item, after it has read the group's state on this
connection ("Still current" there), the row of its `why`:

| `why` | Acted on when |
| --- | --- |
| `mention` | `msg.env` is a `post`, and one of: its signer's person is O; that person is listed for X; `cause` is O's confirmation of `msg.env` for X, not acted on before (`contracts/step-2/confirm.md` section 6); its `thread` is a task for which X has a task or review session here (R-52) |
| `assign`, `review` | X has the session of that role for the task here, which makes the item a continuation; otherwise, the proof of a role below |
| `reviewed`, `checks`, `resume` | X has the task session of that task here |
| `merge` | the same, and the item's `commit` is on the task's branch in this node's clone; when it is not, N reports `task conflict`, as for a branch that has moved |
| `decided` on an approval | section 5 |
| `watch` | `contracts/step-2/watch-team.md` section 2 |
| `reply`, `decided` on a decision, `confirmed`, `notice` | as in step 1: they answer what X asked, or state the home's facts |

**O's confirmation** of an envelope for X: an `act confirm` signed by O as
a person, whose `arg` is the digest for X of that envelope.

**The proof of a role.** `req` (section 6) is the request of the item's
task: its brief, or the `task` message whose `body.start` is a `task start`
with that task as `id`. R is the brief's `env`, or that `body.start`.

| | Holds | Level (section 4) |
| --- | --- | --- |
| 1 | R is a brief; `cause` is an `act confirm` whose `msg` is its `n` and whose `arg` is the digest of the brief, signed as a person by O | O's |
| 2 | the same, signed by a person listed for X | capped |
| 3 | R is a `task start` whose signer's person is O | O's, or capped |
| 4 | R is a `task start` whose signer's person is listed for X | capped |
| 5 | `cause` is O's confirmation of R for X | capped |

- Anything else is refused as in step 1: audit `refused`, then `ack`.
  Nothing tells the home. An honest home queues nothing a host refuses,
  unless a list or a log changed in between; such a task stays quiet, and
  `contracts/task-machine.md` row 23 brings the secretary.
- The owner's word covers the request. Which task of it and which role
  are the home's rows, as in step 1. What a first turn says of the request
  (the brief; the title and plan of a `task start`) is read from R.
- One rule for every home, N's own included. Only a connection that speaks
  `v` 1 keeps the step 1 rule: such a home has one person
  (`contracts/step-3/knowledge.md` section 8).

## 4. The level of a task session (D-1)

Set when N creates the task session, from the proof its first `assign`
passed, and kept for the life of that session:

- **O caused the task** with proof 1; with proof 3 when the item's `msg`
  is the message that the `re` of that `task start` names (by its `n`) and
  O signed it as a person; and when N registered the task for a takeover
  (`flows/takeover.md` A). The session has the level of step 1
  (`contracts/commands.md` section 2, "Levels").
- **Otherwise it is capped** (R-44): the lower of that level and 2, and 1
  where the node cannot enforce level 2 for that CLI
  (`contracts/drivers.md` 1.1). `all_level3` is part of "that level", so it
  lifts nothing. The session row gains one column, `capped`; the cap holds
  at every start of the session's process, a takeover included.
- **`re` of `task start`.** N chooses it when `start_task` is called: the
  first message of that turn that O did not sign as a person; when O signed
  them all, the last one; `null` for a turn without a message.
- One signer of one envelope decides. An owner who confirmed a `confirm`
  card let the AI take part and caused nothing (proof 5). A message
  through an AI never carries O's level, one of O's own included: a capped
  task would otherwise reach it by a call between two AIs of O. A later
  message of O in the thread lifts no cap.

## 5. Approver delegates (D-13)

The delegates of X are the `delegates` of its list and, as in step 1, the
node's `settings.approvers`. A delegate decides approval cards of X only.

- N writes `body.approvers` of each approval card it posts: O, then the
  delegates of X at that moment.
- **The home**, for `contracts/commands.md` 1.4: `approve` and `deny` are
  by a person in `body.approvers` who is in the `approvers` of the member
  row of `body.ai` when the command is handled; anyone else is `forbidden`.
  That takes a removed delegate off the cards posted before (A9.19).
- **The host**, for `decided` on an approval: the check of step 1
  (`flows/approval.md` step 10), with "an approver the owner named" read as
  a delegate of X now: `cause` is signed as a person by O or by such a
  delegate, and names the card of an approval record N holds. The audit
  row names the signing device.

## 6. What the home does for it

- **`req`**: one more optional member beside the `env` of an inbox item, a
  copy of the row of the task's request, taken as `msg` is. It is on every
  `assign` and `review` item. `cause` is chosen by
  `contracts/step-2/confirm.md` section 7.
- **Every task has a request.** For a task from a brief, the brief. The
  `task` message of any other task (`contracts/task-machine.md` row 2)
  gains `body.start`, the signed `task start` that made it; that message is
  its request, and the word behind such a task is the owner of the AI that
  started it. So an owner can be asked about any task.
- **A brief is confirmed with its digest.** `act confirm` on a brief
  always has `arg`, the digest of the brief, which the browser computes
  from the brief it shows. Without it, or with another value: `bad_request`.
- **A request is kept** while a task from it is not final: the exemption
  of `contracts/storage.md` section 4 for a confirmed brief with a queued
  task covers the brief and the `task` message of every such task.
- `member set` with `allow` is accepted from the AI's host, as `approvers`.

## 7. The fork rule of the device log (D-4)

It replaces "Any other entry for that version is refused. First accepted
wins." in `contracts/envelope.md` section 4. For an entry E whose `ver` v
the log holds, and which is not the entry H held there:

1. E is judged by rules 1, 2 and 4 to 6 of that section against the first
   v − 1 entries. If it fails it is refused, and nothing changes.
2. E **wins** when its signer is earlier in the log than the signer of H:
   the device whose first `add` has the lower `ver`. When one device signed
   both, the entry whose `s` is the lower text wins, compared byte by byte.
3. E loses: it is refused, and nothing changes. E wins: the entries from v
   on leave the log, E is appended, and those that left are applied again
   in their order, by the same rules, up to the first that is refused. That
   one and all after it are dropped.

- The outcome depends only on the entries before v and on E and H. Two
  receivers that have applied each other's whole log hold the same log
  (modelled, section 12). Entries travel as in step 1; no frame is added.
- **After a replacement** a receiver forwards E like any accepted entry
  and, when the frame is applied, closes the connections of devices that
  are no longer valid (`flows/pairing.md` D). A home writes the `device`
  rows of the versions that changed again; those of versions the log no
  longer has become `gone` markers. `device_log` follows.
- **A dropped `add` is no revoke.** The device can be added again by any
  valid device, with the same `dev`, `kind` and `name`.
- **Where a revoke is written.** By rule 2 a device D replaces any entry
  above its own first `add` that it signed itself or that a later device
  signed, and what follows is judged again on a log to which D may have
  added devices. A `revoke` of D above such an entry does not hold. So a
  client that revokes D with a device earlier than D signs the `revoke`
  for the lowest version above D's first `add` whose entry D or a later
  device signed, and for the next version when there is none. It wins
  there, and D wins nowhere. The client then signs an `add` again for the
  devices this dropped that the person keeps; for those D added, directly
  or through a device it added, only when the person says so.
- **Limits.** Who holds the oldest device wins: a first device cannot be
  taken back. A `revoke` signed by a device later than D holds only while D
  signs nothing against it. Of two devices that add at one moment, the
  later one's entry is dropped (`contracts/step-2/browser-trust.md` 8.1).

## 8. Vectors

Unit T2-4 changes `internal/proto/testdata/crypto.json`, its generator and
Go reader `internal/identity/vectors_test.go`, its web reader
`web/src/lib/vectors.test.ts`, and both `devlog` files. For `vectors.md`:

**`devlogs`.** `accept` true also means: the entry replaced the one held at
its version. The case "for a version already held, a different entry and
the same entry again" keeps its entries, its flags (true, true, false,
false, true, true) and its `valid`; only its name changes, to "an entry
that loses to the one held". Its third entry is now refused by the
tie-break (one signer; its `s`, `CQDZ…`, is above the held `6z6X…`), its
fourth by rule 5 (its signer is not in the log before version 2). No other
case names a held version with another entry `[verified]` (from the file).

**`forks`**, a new part:

```json
{"name":"two devices revoke each other","user":"<user id>",
 "entries":[{"p":"…","k":"…","s":"…"}],
 "orders":[[0,1,2,3],[0,1,3,2]],"log":[0,1,2],"valid":["<device id>"]}
```

Each list of `orders` names entries by index and is applied to an empty log
of `user`; each must end with exactly the entries of `log`, in that order,
and with `valid` as in `devlogs`. Cases:

- two devices revoke each other at one version (A9.7); two entries of one
  device for one version; two first entries;
- the loser first, followed by an entry that still fits and one that does
  not, and the same with the winner first;
- an entry by an earlier device that fails rule 5 or 6: nothing changes;
- the first device, revoked at a version, signs for that version and wins;
- a `revoke` written where section 7 says, then entries of the revoked
  device for that version and for lower ones: all refused.

## 9. Where this goes, and what changed elsewhere

| File | Gains |
| --- | --- |
| `contracts/envelope.md`, `contracts/vectors.md` | section 4: the fork rule; the vectors of section 8 |
| `contracts/commands.md` | 1.2: `allow` from the host; 1.4: `arg` on a brief's `confirm`, who may `approve` and `deny`; section 2: `allow.set`, the cap under "Levels" |
| `contracts/datachannel.md`, `contracts/conventions.md` | section 6: `req`, and section 3 here in place of "What a host accepts (step 1)"; section 7: the lists in `host.info`. Section 4: an item with `req` stays under 54 KiB |
| `contracts/messages.md`, `contracts/storage.md` | 2.2: `start` in the `task` body. Host table `allow`, `session.capped`, the exemption of section 6 |
| `flows/approval.md`, `flows/agent-turn.md`, `flows/task.md` | steps 9 and 10; step 2 and the level of step 5; G: the `re` of section 4 |

Changed with this file: `confirm.md` (the request and the word behind a task
without a brief; a brief's `arg`), `browser-trust.md` (a `task` request; 8.1)
and `watch-team.md` here; `steps/step-2-3-acceptance.md`; `risks.md` K-4.
P-4 freezes `allow.set`, `req`, `body.start`, a brief's `arg` and the lists
in `host.info`. T2-3 builds sections 2 to 5 on the host, T2-2 the home's
side (sections 5 and 6), T2-4 sections 7 and 8.

## 10. Limits

| | Limit | Consequence |
| --- | --- | --- |
| 1 | The home still says which task and role an AI has, what a review said and when a merge is due, and it supplies what a session reads: notes, threads, task rows | a session trusts that home with its context; labels are the only guard (`contracts/step-2/confirm.md` section 8). Where level 3 is chosen for a group on someone else's home, the interface says so |
| 2 | A home can deliver again what was authorised once: an owner's or a listed person's message, the tasks of a confirmed brief | no new authority; a confirmed `mention` is acted on once |
| 3 | The logs of other people are in memory and come through homes | a home that holds a `revoke` back keeps that device's word good at N until the entry comes from elsewhere |
| 4 | A signer must be valid when N judges | after the device that confirmed a brief is revoked (`risks.md` K-20), its tasks start no new session; the secretary posts a new brief |
| 5 | A delegate removed while their `act` is on the way: the home may accept what the host refuses | the card reads approved and the request stays open; the AI asks again and a new card appears |
| 6 | The repository and base branch N merges into are those of the group row it was given, and the owner's own sign-in merges | section 3 limits a merge to the task's own branch; that a review passed is the home's word |

## 11. Decided here, for the coordinator to confirm

| | Written here | The other answer |
| --- | --- | --- |
| 1 | The signed list is a host command; the member row is a copy | a list signed into the row: a home could show an old one, so the host would need versions |
| 2 | A brief's `confirm` carries the digest of the brief, a change to a step 1 command under `v` 2 | `msg` alone: a home can put another brief under the same id, and with it its own work at the owner's level |
| 3 | The request of a task without a brief is its signed `task start`, shown in the `task` message | the message that led to it: a home could hang any plan on an old message of the owner |
| 4 | D-4 refined: after a replacement the entries that followed are applied again, not all dropped; one signer: the lower `s` | all dropped: a receiver that saw the winner first keeps an entry that another has dropped |
| 5 | Where a `revoke` is written, and adding again what it dropped | always the next version: the revoked device replaces an older entry and is valid again |
| 6 | Only O's signature as a person carries O's level | "O's word", also through O's AIs: a capped task could raise itself by a call |

## 12. Not verified

- Nothing here was run in the product. A throwaway model of section 7 (no
  keys, random logs and orders) ended with one log on every receiver in
  10,000 runs; after a `revoke` written as that section says, no entry of
  the revoked device changed a log in 2,896.
- `[unverified]` Read against `contracts/`, not the host code of step 1;
  the branch test of `merge` with every forge template.
