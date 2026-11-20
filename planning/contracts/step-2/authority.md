# Contract (step 2): authority across owners

Whose word a host accepts when a group's home belongs to someone else: the
host's rule and the list its owner signs (D-2), the level of a task that
another person caused (D-1), approver delegates (D-13), and the fork rule of
the device log with its vectors (D-4). The design points are in
`steps/parallel-plan-step-2-3.md` section 1. Rules: R-42, R-44, R-51, R-53,
R-115, R-136, R-138.

This file adds to the step 1 contracts; `authority-record.md` lists what is
folded into which file and what changed in other files. Presence follows
`contracts/conventions.md` section 2.

Not here, all in `contracts/step-2/`: when a home asks an owner, the digest
and the labels (`confirm.md`); the watch grant (`watch-team.md`); accepting
commands (`checks-forge.md`); what a browser checks (`browser-trust.md`).

## 1. Terms

A home orders and stores. From step 2 its signature on an item is not the
owner's instruction (R-136): a host also needs a signature that leads to
its owner. Below, X is an AI, N its host and O the owner of N.

- **The person of a signer**: the person in whose device log the signing
  device is valid when N judges. O's log, which N reads from its own store,
  decides first: a device valid there is O's, whatever another log holds.
  The logs of the other people of a group come from its `device` rows, each
  of which carries its signed entry: a log proves itself from entry 1
  (`contracts/step-2/browser-trust.md` section 2). A signer valid in no
  log, or in two of those other logs, has no person. No `member` row is
  read: names and roles are the home's.
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
| `merge` | the same, and with a remote: `pr` is the pull request N opened or reused for that task at `submit_task` (`contracts/local.md` section 7), whose number N keeps, and N has read it first (`gh pr view <pr> --json baseRefName,headRefName`): its base is the `base` O accepted (`contracts/step-2/checks-forge.md` section 2), its head the task's branch. If not, N reports `task conflict`. With `forge: custom` N hands its own request to the accepted template and can read no base |
| `decided` on an approval | section 5 |
| `watch` | `contracts/step-2/watch-team.md` section 2 |
| `reply` | the call it answers is one X posted and N still holds open: the `re` of `msg`, a reply or the `confirm` card of a denial, names it. N keeps each call it posts until its answer is delivered |
| `decided` on a decision | `msg` is a card X posted (N's own signature on its `env`, with `as` = X) that N still holds open, or its signed `body.task` is a task whose task session X has here; and `cause` is an `act choose` on that card. N keeps each decision card it posts, as it keeps a call, until its `decided` is delivered |
| `confirmed`, `notice` | X is the secretary by O's consent (below). For `confirmed` also: `msg` is a brief and `cause` is an `act` on it. A brief that X did not post (not N's signature with `as` = X) is one an earlier secretary left: the item is then `information` |

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

- **Proofs 1 and 2 need the role.** The one entry of R's `body.tasks` with
  the task row's `title` names X as `assignee` for `assign`, as `reviewer`
  for `review`. Or the item is signed by a device valid in O's log: O's
  own home, which queues such an item without a card only when a device of
  O signed the `task assign` that gave the role (section 6). Any other
  role the brief does not give X needs proof 5.
- **The secretary by consent.** N treats X as a group's secretary (its
  tools and instructions, `notice`, `confirmed`) only when the group row
  names X and O consented: O joined X as the secretary (the flag of
  `host.join`), or the row's `secretary_by` is a `group set` naming X that
  O signed as a person.
- Anything else is refused as in step 1: audit `refused`, then `ack`.
  Nothing tells the home. An honest home queues nothing a host refuses,
  unless a list or a log changed in between; such a task stays quiet, and
  `contracts/task-machine.md` row 23 brings the secretary.
- **Labels** (`contracts/step-2/labels.md`). The header of an item N acts
  on has the label of its kind. What the item carries beside its request
  (`msg`, the `cause` of a `decided` or `confirmed`) is shown under a line
  of its own, labelled by its own signer and by nothing else.
- The owner's word covers the request. Which task of it is the home's row.
  What a first turn says of the request (the brief; the title and plan of
  a `task start`) is read from R.
- **`v` 1.** N speaks `v` 1 only to a home whose device is valid in O's
  log, and keeps the step 1 rule there. Answered `version` by any other
  home, it sends no `hello` with `v` 1 and takes no item from it
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
- **A role the brief does not give** asks the AI's owner by a card
  (`contracts/step-2/confirm.md` section 7), except on that owner's own
  home when a device of that owner signed the `task assign`, as a person
  or through one of their AIs. A `brief` whose `body.tasks` holds one
  `title` twice is `bad_request`: a task's title names one entry.
- **The secretary.** `group set` with `secretary` is by an admin, as in
  `contracts/commands.md` 1.1, and only when that admin is the owner of
  the AI or the AI's member row has `secretary_joined`; else `forbidden`.
  `secretary_joined` is one more optional field of an AI's `member` row:
  `true` from a `member add_ai` with `secretary: true`, never removed, so
  that it moves with the rows of a group. The group row gains
  `secretary_by`, optional: the signed `group set` that named the
  secretary, absent when `member add_ai` did.
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
   on leave the log and E is appended. H is dropped. The others that left
   are applied again in their order, by the same rules, up to the first
   that is refused; that one and all after it are dropped.

- The outcome depends only on the entries before v and on E and H. Two
  receivers that have applied each other's whole log hold the same log
  (modelled, `contracts/step-2/authority-record.md` section 3). Entries
  travel as in step 1; no frame is added.
- **After a replacement** a receiver forwards E like any accepted entry
  and, when the frame is applied, closes the connections of devices that
  are no longer valid (`flows/pairing.md` D). A home writes the `device`
  rows of the versions that changed again; those of versions the log no
  longer has become `gone` markers. `device_log` follows.
- **A dropped `add` is no revoke.** A device whose `add` was dropped is
  not valid and not revoked. It counts where a rule asks whether a device
  was one of a person's (`contracts/step-2/move.md` section 2, check 2),
  and not where a rule asks for a `revoke` entry (`copy.md` section 4). Any
  valid device can add it again, with the same `dev`, `kind` and `name`,
  or revoke it.
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

In `contracts/step-2/fork-vectors.md`: what the part `devlogs` of
`crypto.json` keeps and what changes in it, the new part `forks`, and the
files unit T2-4 changes.

## 9. Folding, decisions, what is not verified

In `contracts/step-2/authority-record.md`: what is folded into which step 1
file, what changed in other files, the points the coordinator settled with
the other answer for each, and what was modelled and not run.

## 10. Limits

| | Limit | Consequence |
| --- | --- | --- |
| 1 | The home still says which task of a request an AI has, its role in a task without a brief, what a review said and when a merge is due, and it supplies what a session reads: notes, threads, task rows | a session trusts that home with its context; labels are the only guard (`contracts/step-2/labels.md`). Where level 3 is chosen for a group on someone else's home, the interface says so. A merge goes only into the `repo` and `base` the owner accepted (`contracts/step-2/checks-forge.md` section 2), and only through the pull request N opened or reused |
| 2 | A home can deliver again what was authorised once: an owner's or a listed person's message, the tasks of a confirmed brief, the `decided` of a card another AI posted about a task of X, and an old `secretary_by` of O after O moved the role away | no new authority; a confirmed `mention`, a call and a decision X posted are acted on once. A secretary named again by an old `group set` serves until O takes the AI out of the group |
| 3 | The logs of other people are in memory and come through homes | a home that holds a `revoke` back keeps that device's word good at N until the entry comes from elsewhere |
| 4 | A signer must be valid when N judges | after the device that confirmed a brief is revoked (`risks.md` K-20), its tasks start no new session; the secretary posts a new brief |
| 5 | A delegate removed while their `act` is on the way: the home may accept what the host refuses | the card reads approved and the request stays open; the AI asks again and a new card appears |
