# Contract, step 2: people

How a second person comes into a group, what the three roles may do, and
how a person leaves or is removed. It settles design point D-7 of
`steps/parallel-plan-step-2-3.md` and refines the row "People" of
`steps/step-2-3.md`. Rules: R-134, R-136, R-138, R-140.

This file adds to the step 1 contracts and changes nothing in them
(section 8). Whose signature a host requires is
`contracts/step-2/authority.md`.

## 1. Roles

A person's `member` row has `role` (`contracts/rows.md` section 2).

| Role | Who | May, beyond what the role above it in this table may |
| --- | --- | --- |
| `member` | every person who joined | read the group; `post`, `note`, `jump`; `act` on a card that names them; add and remove their own AIs (`host.join`, `host.leave`); remove themself |
| `admin` | named by the owner | everything `contracts/commands.md` gives "an admin"; `invite`; remove a person whose role is `member` |
| `owner` | the owner of the home node; the person of `group create` | set roles; remove an admin |

- A group has exactly one owner. The role is never given, taken or left:
  it follows the home and changes only when the group moves
  (`contracts/step-2/move.md`).
- "Admin" in every contract means role `owner` or `admin`, as in step 1.
- A role is read from the row when a command is handled. Nothing is
  cached and no command is judged again after a role changed.
- A role belongs to the person. The AIs of an admin get nothing from it.

## 2. Commands

Home commands; `g` names the group (`contracts/commands.md`).

### 2.1 `invite`

`{"t":"invite","n":"…","ts":…,"g":"…"}`. By an admin. Result: `link`,
`expires`. The home creates a pairing secret (`contracts/envelope.md`
section 6: 80 random bits, valid 10 minutes, used once), marks it as an
**invitation** for group `g` made by that admin, authenticates to its
rendezvous services and joins the room of the secret. It writes no row:
an invitation lives in memory and ends with the node process.

```
https://<web client>/#join=<secret>&g=<group id>&home=<device id>&fp=3F9A0C2177BE
```

| Part | Content |
| --- | --- |
| `<web client>` | the address the link of `pair.browser` uses |
| `join` | the secret, as section 6 shows it |
| `g` | the group id |
| `home` | the device id of the home |
| `fp` | the fingerprint of `home` (`contracts/envelope.md` section 2), its 12 hex digits without spaces |

`expires` is the time the secret ends. An invitation admits one person
and is open until a `join` succeeds on it or that time is reached; no
command withdraws one. The receipt records that the command succeeded,
not the link: a repeat is answered `ok` with the original link while the
invitation is open, otherwise `conflict`, as for `pair.browser`.

### 2.2 `join`

`{"t":"join","n":"…","ts":…,"g":"…","name":"…"}`. By the device of a
joining connection (section 4), for the group of its invitation. `name` is
the name the person takes in this group.

The home accepts it when all of this holds:

1. The connection is a joining connection whose invitation is open and is
   for `g`: else `forbidden`.
2. A device log was presented on this connection (section 4, step 7), it
   is valid, and at its end the signer `k` is a valid device: else
   `forbidden`. The person is the `user` of that log.
3. That person is not a member of `g`, and `name` is unused in the group:
   else `conflict`.

A refused `join` leaves the invitation open. An accepted one writes, in
one transaction:

- a `member` row: `id` = `n`, `kind: human`, `user`, `name`, role `member`;
- one `device` row per entry of the person's log, and the log in
  `device_log` (`contracts/storage.md` section 2);
- a `system` message `member_joined` in the main line (section 6);
- the invitation is used up.

Then the home pushes `groups` to the connection, which from now on is the
connection of a member's device (section 5).

### 2.3 `member set` with `role`

`{"t":"member","n":"…","ts":…,"g":"…","op":"set","id":"<member id>",
"role":"admin"}`. By the owner. `id` is a person's row, not the owner's
own (`forbidden`). `role` is `admin` or `member`; anything else is
`bad_request`. The member row is updated. On a person's row `set` reads
`role` and nothing else; on an AI's row it is the `set` of step 1.

### 2.4 `member remove` of a person

`{"t":"member","n":"…","ts":…,"g":"…","op":"remove","id":"<member id>"}`,
the shape of step 1, with a person's row as `id`.

| Target's role | By |
| --- | --- |
| `member` | that person; an admin |
| `admin` | that person; the owner |
| `owner` | nobody: `forbidden` |

Refused with `conflict` while an AI of that person is assignee or reviewer
of a task that is not final: reassign or cancel first, as for one AI.
Otherwise, in one transaction:

1. Each AI of that person in the group is removed as `member remove` of
   step 1 removes it: a `gone` marker, and for the secretary
   `group.secretary` cleared with `system` `no_secretary`.
2. The person's `member` row becomes a `gone` marker.
3. The person's `device` rows become `gone` markers. Their log leaves
   `device_log`, and the `push_sub` rows of their browsers are deleted,
   unless the person is the node's owner or a member of another group
   homed here.
4. A `system` message `member_removed` in the main line.

The home then pushes `groups`, without this group, to every connected
device of that person and pushes them nothing more about it. A connection
left with no reason of `contracts/datachannel.md` section 8 is closed;
new signalling from those devices fails `unknown_device`.

- Not touched: what the person and their AIs wrote, and lists that name
  the person's user id (`allow` and `approvers` of other AIs,
  `body.approvers`, `body.to`). Those lists have no effect: the home
  answers `forbidden` to every command of a person who is not a member.
- The host of a removed AI sees the `gone` marker of its row and closes
  that AI's sessions in the group as after `host.leave`.

A removed person may be invited again and gets a new member id. Stated
limit: removal ends access and takes nothing back. The person keeps the
clone on their machines and whatever their clients had read.

## 3. The invitation is not a device pairing

Section 6 of `contracts/envelope.md` enrols a device into a person: roles
`a` and `b`, an `add` entry, adoption. An invitation uses the same secret,
room and `mac`, and must never have that effect. Four rules:

1. **A secret has one purpose**, fixed by the device that created it:
   pairing or invitation. It is never used for the other.
2. **A third role.** In the `pair` message of the joiner, `role` is
   `join`. The home, which created the secret, sends `role: a` as in
   section 6. `mac` is computed as there, over the role as it stands in
   the message, so the rendezvous service cannot turn `join` into `b` or
   back.
3. **Acceptance by purpose.** The acceptance rule of section 6 stands,
   with "the other role" read as:

   | Side | Holds | Accepts only |
   | --- | --- | --- |
   | creator of a pairing secret | a pairing secret | `b` |
   | the other side of a pairing | a pairing secret | `a` |
   | home | an invitation | `join` |
   | joiner | a link with `join=` | `a`, with `k` equal to `home` of the link |

   Any other `pair` message is ignored. So the holder of an invitation
   who sends `b` is not enrolled, and a browser that opened a `#pair=`
   link never sends `join`.
4. **No log entry, no adoption.** A device accepted with `join` is a
   **joining peer**, not a pairing peer. The home writes no `add` entry
   for it and sends it no log before it is a member. No device adopts a
   person on a joining connection, whatever arrives on it: adoption
   needs a connection paired with roles `a` and `b`. Both logs stay as
   they were; the joiner's person becomes a member, with all its devices.

For check 2 of `contracts/envelope.md` section 3, the home listens to a
joining peer, and the joiner to the `home` of its link, like a pairing
peer: for 10 minutes from the creation of the secret, for signalling and,
at the home, for the one command of section 5.

The shared vectors gain (`contracts/vectors.md`): in `pairing`, the `mac`
of a side with role `join`; in `pair_messages`, a field `for` (`pair` or
`invite`) beside `me`, and the cases of the table above, each refused
case with the `mac` that follows from its role.

## 4. Flow: joining

Actors: A, an admin; H, the home; P, the person joining; B, P's browser;
RV, the rendezvous service.

1. A opens "invite" and A's browser sends `invite` to H. The dialog shows
   the link, its countdown and the fingerprint of H.
2. A gives the link to P by any means. Who holds the link can join.
3. B opens the link. B must belong to a person: a browser without one
   shows onboarding (`flows/pairing.md` A) and does not join.
4. B reads the four parts and contacts nobody unless `join` is a secret
   as section 6 reads a typed one, `g` is an id, `home` is a device id,
   and `fp` is the fingerprint of `home`. It shows the fingerprint and
   asks P to confirm. P can compare it with what A's dialog shows.
5. B authenticates to RV and joins the room. RV sends `joined` to both. H
   and B exchange `pair` messages and accept them by section 3.
6. B sends a signed `offer` to H; ICE runs (`flows/connect.md`); `ctl`
   opens. B sends `hello`; the `welcome` has `groups: []` and
   `owner: false`.
7. B sends its person's whole log in `devlog` frames. H judges the
   entries by the rules of the log and keeps the result in memory for
   this connection. H sends no log.
8. B sends `join` with the name P chose. H handles it (2.2).
9. On `ok`, B adds H to its list of group homes with the signalling URLs
   it reached H on (`contracts/storage.md` section 5), sends `home.add`
   (section 7) to each of P's nodes it is connected to, reads the `groups`
   push and sends `sync`. A new member reads the whole history H holds.
10. P joins AIs as in step 1: B sends `host.join` with `home` = H to one
    of P's nodes. That node connects to H, and H accepts its signalling
    because the node is a valid device of a member.

What each side does to admit the other:

| Side | Checks, in order |
| --- | --- |
| B, before it sends anything about P | the link agrees with itself (step 4); H's `pair` has role `a`, the `mac` of the secret, and `k` = `home`; H's `answer` is signed by `home`. The channel is then tied to that key (`architecture.md` section 4), and RV could not stand in for H |
| H, before P is a member | B's `pair` has role `join` and the `mac` of an open invitation; B's `offer` is signed by the `k` of that message; the log B presents is valid and holds that `k` as a valid device; `join` is signed by it |
| P's node, for `host.join` | the command is signed by a device of its owner and names `home`; the node accepts signalling only from that device id |
| H, for P's node | the node is a valid device in the log of a member |

So the nodes of two people verify each other by keys their own owners
signed for (R-140). Failure at any step is shown with the step and the
reason. B closes the connection; the invitation stays open until its
time is reached.

## 5. What a node accepts from a joining peer and from a member

Two rows of `contracts/datachannel.md` section 8 in step 2:

| Peer is | Accepted |
| --- | --- |
| a joining peer: a device that passed the exchange of section 3 and whose person is not a member | `hello`; `devlog` for one person's log; `cmd` with a `join` payload for the group of its invitation. Every other request is answered `forbidden`. No `sync`, no `fetch`, no host surface |
| a device of another member of a group homed here | home surface for that group; `devlog` |

- A joining connection whose person is no member when the invitation's
  time is reached is closed.
- **Device logs with several people.** After `welcome` a home sends a
  peer the logs of the people in the groups that peer may read, and no
  other. It applies a `devlog` frame only for a person whose log it
  stores (its owner, the members of its groups) or, on a joining
  connection, for the log being presented; other frames are dropped. A
  member's device sends its own person's new entries to every home it is
  connected to, as in step 1, and the home writes each accepted entry as
  a `device` row in every group of that person. Two entries for one
  version are judged by `contracts/step-2/authority.md`.
- A device that a member's log revokes is closed and refused as in
  `flows/pairing.md` D.

## 6. `system` codes

Two rows for the table of `contracts/messages.md` 2.11, both written by
the home:

| `code` | Always present | Optional |
| --- | --- | --- |
| `member_joined` | `member` (the new row), `by` (the admin who invited) | — |
| `member_removed` | `user`, `name` (of the removed row, which is gone) | `by`, the member who removed them; absent when the person left |

So a link that reached the wrong person is seen in the history. A role
change writes no message: the row shows it.

## 7. Points the owner settled (2026-11-20)

1. **Lifetime of an invitation**: 10 minutes, as a pairing secret, in a
   room that lives 15 (`contracts/rendezvous.md` 2.2). Nothing is stored.
2. **The person's other devices** learn the group from P's nodes before
   any AI of P joins. Host command `home.add` (`contracts/commands.md`
   section 2), fields `g`, `home` (device id), `name`, `addrs` (the
   signalling URLs B reached H on): the node adds the entry to
   `host.info.homes` (`contracts/datachannel.md` section 7), each entry of
   which carries `home`, and keeps `addrs` as addresses of `home`. It
   connects to nothing and clones nothing. A node B did not reach learns
   the group at `host.join`. Frozen in P-4, built in T2-1.
3. **One web client origin.** A stated limit: a browser is a device per
   origin, and the link opens the web client of the home's rendezvous. A
   person on another origin first adds a browser there
   (`flows/pairing.md` C). With the official service it does not arise.
4. **A person with no node cannot join.** A stated limit: step 3 of the
   flow refuses a browser that belongs to nobody. An identity in one
   browser profile alone would be lost with it (`risks.md` K-20).
5. **Who sets roles**: the owner alone (section 1), so that admins cannot
   promote or demote each other.

## 8. Where this goes after step 1

| Part | Folded into |
| --- | --- |
| sections 1 and 2 | `contracts/commands.md` 1.2, and a section for `invite` and `join` |
| section 3 | `contracts/envelope.md` section 6; `contracts/vectors.md` |
| section 4 | a new `flows/joining.md` |
| section 5 | `contracts/datachannel.md` sections 3.1 and 8 |
| section 6 | `contracts/messages.md` 2.11 |

Unit P-4 freezes the wire shapes: the payloads of `invite` (with its
result) and `join`, `member set` with `role`, the `pair` message with
role `join`, the two `system` bodies, `home.add` and `home` in
`host.info.homes` (section 7), and the vectors of section 3.

`[unverified]` Nothing here was run. That one rendezvous connection holds
several rooms was read in the code of the service, not tested.
