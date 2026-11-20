# Contract: what a browser trusts (step 2)

In step 1 a browser shows what the home pushes, because the home is one of
the viewer's own devices. From step 2 a home may belong to someone else.
This file says what a browser then checks before it shows who wrote a
message or decided a card, what it shows when the check fails, and what the
interface of step 2 must say to people. It settles design points D-5 and D-6
of `steps/parallel-plan-step-2-3.md` and states the limit of D-4 for the
interface.

It is a step 2 file: no step 1 contract changes, and nothing is added to
the wire. It rests on `contracts/envelope.md`, `contracts/rows.md`,
`contracts/messages.md` and `contracts/step-2/authority.md` (the fork rule
of the device log). It says what is shown and when; how it looks is
decided by the interface units.

## 1. When the check applies

A browser reads a group from one device, the home, over a connection whose
signalling that device signed (`architecture.md` section 4).

- **Own home**: that device is in the valid set of the viewer's own device
  log, as the browser holds it. Rows are shown as in step 1, with no check.
- **Home of another**: any other device. Sections 2 to 6 apply to every
  row of the group.

The browser decides this from its own copy of its person's log and the
device id it connected to, not from `welcome.owner` or the `home` of the
group row: both are statements of the device being judged.

## 2. The logs the browser checks against

- The browser keeps one device log per person, in memory, for every person
  it meets in any group. Entries come from the `env` of `device` rows and
  from `devlog` frames (`contracts/datachannel.md` 3.1), whichever home or
  node sent them: every entry carries its own signature. The other fields
  of a `device` row are not read.
- It applies them by `contracts/envelope.md` section 4 and by the fork rule
  of `contracts/step-2/authority.md`. A user id is the key of the first
  entry, so a log proves itself: no home can write a log for a person. A
  home can only hold entries back (section 10).
- A device is **valid** when the log holds it in its valid set now,
  **revoked** when the log has revoked it, and **unknown** to it otherwise.

## 3. A message

A valid signature alone says nothing about the name or the text on the
screen. A `msg` row whose `by` is a member id is shown as that member's
only when the envelope is good (3.1), what is displayed is what was signed
(3.2), and the signer is bound to the member (3.3). A row the home wrote
itself claims no author and is not checked (section 5).

### 3.1 The envelope

- The row has `env`, and `env` passes check 1 of `contracts/envelope.md`
  section 3 (`bad_sig`). No other check of that section applies: a row is
  read long after it was signed, and is addressed to nobody.
- `env.p` is a JSON object, read by `contracts/conventions.md` section 2,
  in which `t` is `"post"`.

### 3.2 What is displayed must be what was signed

| Row field | Must equal, in the payload |
| --- | --- |
| `id` | `n`: the envelope is not shown as another message |
| `g` | `g`: nor in another group |
| `kind`, `text`, `thread`, `re`, `mentions` | the field of the same name |
| each field of `body` that is **signed** for this kind | the same field of `body` |

- Which fields of `body` are signed is listed per kind in
  `contracts/messages.md` section 2, and for the `confirm` card in
  `contracts/step-2/confirm.md`. A field the home adds is not compared
  (section 5).
- One exception, from `contracts/messages.md` 2.9: the `score` of a
  `review` row equals the signed `score`, or is 0 when a signed item has
  `veto: true`.
- Equal means the same JSON value: the same type, number or text, lists in
  the same order, objects with the same members in any order. An optional
  field is absent on both sides or equal.

### 3.3 The signer must be bound to the member

Let M be the `member` row whose `id` is the row's `by`.

| Author | Must hold |
| --- | --- |
| a person | the payload has no `as`; M has `kind: human`; `env.k` is valid in the log of `M.user` |
| an AI | the payload's `as` equals `by`; M has `kind: ai`; `env.k` equals `M.host`; `env.k` is valid in the log of `M.user`, the AI's owner |

An AI has no key: its host signs for it (`contracts/envelope.md`
section 5). The last condition of the AI row ties the host to a person;
without it a home could name any device as a host. This establishes that
a device of the person with user id `M.user` signed the message as this
member; for an AI, which person's computer sent it, not what the model
wrote. That the user id goes with the name on the screen is not
established (section 10, limit 1).

## 4. Who decided a card

A decided card keeps the signed `act` in `state.act`
(`contracts/messages.md` section 3). Before the browser shows who
confirmed, sent back, approved, denied or chose, all of this holds:

| Part | Must hold |
| --- | --- |
| envelope | `state.act` passes check 1 of `contracts/envelope.md` section 3; its payload is a JSON object in which `t` is `"act"`, `g` is the group and `msg` is the row's `id` |
| action | `action` goes with `state.status`: `confirm` with `confirmed`, `revise` with `revise`, `approve` with `approved`, `deny` with `denied`, `choose` with `decided` |
| what the person wrote | `text` equals `state.note` of a `revise`; `arg` equals `state.choice` and `text` equals `state.text` of a `decided` card; each absent on both sides or equal. On a `confirm` card, `arg` of a `confirm` is the digest this browser computes (`contracts/step-2/confirm.md` section 6) |
| signer | the payload has no `as`; the member row of `state.by` has `kind: human`; the signer is valid in the log of that row's `user` |

A `superseded` brief that still holds an `act` is checked by that `act`'s
own `action`, and the decided states of the `confirm` card by the actions
of `contracts/step-2/confirm.md`. A status without an `act` is the home's
statement. This reads "the envelope of a row" in D-5 as both `env` and
`state.act` (section 11, point 4).

## 5. What is attributed to the home

Everything below has no signature of an author. It is what the home says,
and it is shown as in step 1, without a mark: the mark is for a claim
"this member wrote this" or "this person decided this" that did not pass.
The group info says once whose computer the home is (8.2).

| What | Where |
| --- | --- |
| order and time | `seq`, `pos` and `ts` of every row; `main` of a message, which the home may set |
| the state of a card apart from section 4 | `state.status` without an `act`; `state.at`; the `reply` and `ms` of a call |
| fields the home adds to a signed message | every field of a `summary` body; `num`, `title`, `round`, `pass` and `ok` of a `review`, and its `score` of 0 after a veto |
| messages the home generates | rows with `by: "system"`, which have no `env`: kinds `task`, `checks` and `system`. A `task_merged` row is still shown under the secretary's name (R-24) |
| rows that are not messages | `group`, `member` and `task` rows, so also names, roles, the state of an AI, who a task is assigned to |
| notes | a `note` row has `by` and no envelope (section 11, point 2) |
| previews and reminders | `thread.last` of a task row; the names in `welcome.groups`; the content of a reminder (`contracts/commands.md` 1.7) |

A row is the home's only when its `by` is `"system"` and its `kind` is
`task`, `checks` or `system`: these have no `state` and nothing to act on.
Every other row is checked by section 3. So a message that names a member
and has no `env` does not pass, nor does a card with `by: "system"`,
except a `confirm` card with `body.what` `direct` or `ask`: it is judged by
the request its `re` names, by section 3, and its `act` binds that request
(`contracts/step-2/confirm.md` section 6; section 11, point 6). An
`approval` card is presented under the secretary's name (R-42); the check
is about its `by`, the AI that asked.

## 6. Outcomes and the unverified mark

The check ends at the first line that applies:

| | If | Outcome |
| --- | --- | --- |
| 1 | 3.1 or 3.2 fails; for a decision, one of the first three parts of section 4 | does not match |
| 2 | the browser holds no live member row for `by` (so also for `"system"`), or no log for that member's person | cannot be checked |
| 3 | the member is of the other kind, `as` or `host` differ, or the signer is unknown to the log | does not match |
| 4 | the log has revoked the signer | signed by a removed device |
| 5 | none of these | verified |

- **When.** A row is drawn when its check has ended; the check needs no
  traffic. It is judged again when the row, the member row or the person's
  log changes: an outcome follows the log and is never stored. Rows of a
  first read are judged after its `synced`, because the state rows the
  check needs are part of that read.
- **Verified** rows are shown as in step 1, with nothing added. There is
  no "verified" badge, and no mark anywhere in a group on an own home.
- **The unverified mark** is shown for the three other outcomes, beside
  the name it is about: the author of a message, wherever that row is
  drawn with its author (main line, thread, the ledger, the last message
  in the group list, an in-page notification), and the person named as
  having decided a card. The text and the card are still shown, as the
  home sent them.
- **On request** (the interface units choose how) the mark names the
  home's computer and its person and gives the sentence of its outcome:

| Outcome | Sentence |
| --- | --- |
| does not match | this browser could not confirm that the member wrote, or decided, what is shown here |
| signed by a removed device | it was signed by a device that its person has removed since |
| cannot be checked | this browser does not have what it needs to check it |

- **A card can be acted on from this browser only when its author is
  verified**; a `direct` or `ask` card, when its request is. Otherwise its actions are replaced by the sentence of the
  mark: an `act` names only the id of a card, so acting on content that
  did not pass would sign for something not seen (section 11, point 3).

## 7. The secretary warning (D-6)

There is no mechanism: R-51 stays without an auto-accept switch. The
interface warns, without blocking any action and with nothing to switch
on or off, and the documentation advises (section 9).

**Condition**, read from rows: the group row names a secretary, that
member row has `account: "personal"`, and the group has a person who is
not the secretary's owner (a `human` member whose `user` differs from the
secretary's `user`). The account type is what the owner declared (R-50);
the warning follows the declaration.

| Where | Shown to | When |
| --- | --- | --- |
| group info, beside the secretary | every person of the group | while the condition holds |
| choosing the secretary (`group set`, or adding an AI as secretary) | the person choosing | the choice would make the condition true |
| the invitation dialog | the person inviting | the secretary is on a personal subscription and the group has no other person yet |

**What it says**: every request to this secretary from someone other than
its owner waits until the owner confirms it, each time (R-51); with a team
plan or an API key the owner can list who may direct it (R-53).

## 8. Interface notes of step 2

### 8.1 The first device (the limit of D-4)

The rule is in `contracts/step-2/authority.md`: when two entries claim one
version of a person's log, the one signed by the device that is earlier in
the log wins on every receiver, and the other entry with everything after
it is dropped.

The limit, to be said in these words or plainer ones: **whoever holds the
oldest device wins, so the first device must be kept safe.** The first
device is the person's id and nothing ranks above it. Someone who holds it
can sign an entry for the version at which the other devices removed it;
that entry wins, so they can remove every other device and add their own,
and no other device can undo it. SoloDawn cannot take a first device back.

| Where | When | What is shown |
| --- | --- | --- |
| onboarding, first run (`flows/pairing.md` A, step 10) | the browser writes entry 1 | this browser becomes the person's first device, and the limit above |
| the device list of "my computers and AIs" | always | devices in the order of the log, oldest first; the first one named as the first device, with the limit |
| the dialog that revokes a device | this browser is later in the log than the device to revoke | the revoke holds only while that older device signs nothing against it; revoke from an older device where the person has one. For the first device: it cannot be taken back at all |
| "add a computer", "add a browser" | always | add one device at a time (`flows/pairing.md` B) |
| a notice on the device list and in the page | the fork rule dropped entries of the viewer's own log that this browser held | which entries were dropped (device name, added or removed), which device signed the entry that won, and: if that was not the person's own doing, that device is in someone else's hands |

The notice stays until the person closes it. A browser whose own `add` was
dropped is no device of the person any more and behaves as a revoked
browser does in step 1 (`flows/pairing.md` C and D).

### 8.2 Whose computer keeps the group

The group info shows, for every group: the name and fingerprint
(`contracts/envelope.md` section 2) of the device this browser is
connected to as the home, and the person in whose log that device is
valid, or that this person is not known.

For a home of another it also says, once: that computer stores and orders
the group and must stay on (R-134); this browser checks who wrote each
message and who decided each card; everything else is as that computer
reports it (section 5). The project memory section says the same above the
notes, whose authors are not checked. A person's member card shows the
fingerprint of their user id, so that two people can compare it outside
SoloDawn (section 10, limit 1).

### 8.3 What the other step 2 contracts show

This file adds nothing to the other files of `contracts/step-2/`
(`people.md`, `confirm.md`, `authority.md`, `watch-team.md`,
`checks-forge.md`, `move.md`), which say what their own additions show.
Their acceptance tests are in `steps/step-2-3-acceptance.md`.

## 9. Notes for the user documentation

The documentation of step 2 has a place for each of these:

| Topic | What it says |
| --- | --- |
| accounts in a team | A secretary that several people talk to should run on a team plan or an API key, where the owner lists who may direct it. On a personal subscription every request from someone else waits for the owner (R-51), and that cannot be switched off. The same holds for any AI that colleagues direct often |
| the first device | which device is the first one (in a first run, the browser the person paired from), what holding it means (8.1), and how to keep it: a browser profile on a computer only that person uses, never a shared or borrowed one |
| the unverified mark | what each of its three sentences means and what to do: ask the member through another channel, compare fingerprints, do not act on the card |
| a group on someone else's computer | what that computer can do (hold back, delay, reorder or drop rows; write its own rows; say who is a member and under which name) and what it cannot (show text under a member's name, or a decision under a person's name, without the mark) |

## 10. Limits

| | Limit | Consequence |
| --- | --- | --- |
| 1 | Names are the home's. The check proves which user id signed; that this id belongs to the member shown, under that name, is in the `member` row, which has no signature | people compare the fingerprint of 8.2 |
| 2 | A home can hold back. It cannot forge an entry, a message or a decision, but it can leave out messages and log entries | a revoke that a home holds back keeps the removed device looking valid there until the browser gets the entry from another home or node |
| 3 | Removed devices are common: a browser that lost its key comes back as a new device and the old one is revoked (`risks.md` K-20) | in homes of others its earlier messages carry the mark (section 11, point 1) |
| 4 | Previews and reminders are the home's (section 5) | they may name an author that the thread, once opened, does not confirm |
| 5 | Nothing is stored: the cache and the logs live in memory (`flows/sync.md` G) | every check is made again after a reload |
| 6 | A node and a browser read the escape of a lone surrogate differently (`risks.md` K-17) | such a message does not match; only its signer can write one |
| 7 | An own home is not checked | if one of the viewer's own nodes is in the wrong hands, revoking it protects; this file does not |

## 11. Needs the coordinator's word

The contract is written on the first answer of each point.

| | Written here | The other answer |
| --- | --- | --- |
| 1 | A message signed by a device that was valid then and is revoked now is marked, with its own sentence: a stolen key and a dishonest home could otherwise write under its person's name for ever | it is shown as verified: the mark is rarer, revoking is weaker |
| 2 | Notes have no envelope: their author is the home's statement, said once above the list | `note` rows get an optional `env` in the wire shapes of step 2 and are checked like messages |
| 3 | A card whose author carries the mark cannot be acted on from this browser | the actions stay, with a warning |
| 4 | Section 4: `state.act` is checked before the browser names who decided a card | D-5 speaks only of "the envelope of a row": the author alone is checked |
| 5 | 8.2: the fingerprint of a person's user id on their member card, the only thing added to a step 1 screen beyond the mark and the texts | no fingerprint; limit 1 stays without a remedy |
| 6 | A card with `by: "system"` does not pass and cannot be acted on, except a `direct` or `ask` `confirm` card: `contracts/step-2/confirm.md` has the home write it, names the signed request in `re`, which this browser checks by section 3 before the owner can act, and binds the `act` to that request by its digest | a card the home wrote is acted on unchecked: the owner signs for a card id the home chose, whatever was shown |

## 12. Not verified

- The sibling files of `contracts/step-2/` are written at the same time
  and were not read. 8.1 follows D-4 as the plan states it; section 4
  assumes that a `confirm` card keeps its `act` in `state.act`.
- The signed fields were taken from `contracts/messages.md`, not compared
  with the golden files or with code.
- How long the checks of a first read take in each baseline browser.
- That a first device cannot be taken back follows from D-4 as written,
  not from test vectors or from every case of `risks.md` K-4.
