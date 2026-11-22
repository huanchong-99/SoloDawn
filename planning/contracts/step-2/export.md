# Contract (step 2): exporting and importing a group

How a group moves to another home, and how a copy is kept for safekeeping
(R-134, R-135, `risks.md` Q-12). Design point D-11 as the coordinator
decided it on 2026-11-22: one export on the old node and one import on the
new; a copy is an export. There is no live hand-off. This file adds to the
step 1 contracts and changes the meaning of no step 1 shape; section 6 says
where it is folded.

Actors: P, the group's owner; P1, the home; P2, another node of P; Q, a
member, with node Q1.

## 1. Commands

Two commands of the local command line (`contracts/local.md` section 1),
which talk to the running node through the local socket. Whoever can run
them is at the owner's machine, so they need no role check of their own.

| Command | Does |
| --- | --- |
| `solodawn group export <group id> <file> [--to <device id>]` | on the home of the group: writes the group to `<file>` (section 2). With `--to`, a move (section 3): from then on this node is no longer the home. Without it, a copy: this node stays the home. Fails when this node is not the group's home |
| `solodawn group import <file>` | installs the group of `<file>` and makes this node its home. Fails when this node's owner is not the group's owner, or when the node already holds rows of that group |

Both print one line and exit non-zero on failure, as the other commands do.
A group moves only between nodes of its owner, so the role `owner` never
changes (`contracts/step-2/people.md` section 1).

## 2. The file

One SQLite file with the schema of the home tables (`contracts/storage.md`
section 1) and `device_log`, holding only that group: its `row` rows,
`gone` markers included; its `grp` counters; its unacknowledged `inbox`
items; the device logs and `push_sub` rows of its people. The node writes it
in one read transaction, so the file is one consistent state of the group.
Receipts are not in the file (section 5, limit 2).

## 3. Moving

1. P runs `group export <g> <file> --to <P2>` on P1. In one transaction P1
   writes the file, deletes the group's rows, and keeps a `setting` entry
   `moved`: the group id and the device id of P2.
2. From then on P1 answers every request for that group, `cmd`, `sync` and
   `fetch`, with the new error `moved` and the field `home`, the device id
   of P2. `welcome` and `groups` no longer list the group.
3. P copies the file to P2 by any means and runs `group import <file>`
   there. P2 is the home; it pushes `groups` to the peers that connect.
4. **Clients follow.** A browser that gets `moved` replaces the group's home
   in its list with `home`, keeping the signalling URLs it had, and
   connects. A node with an AI in the group does the same for that AI's
   connection; its sessions stay. Until P2 has imported, the new home is
   unreachable or answers `not_found`, and clients retry as for any home
   that is down.
5. Items that P1 held unacknowledged are delivered by P2, once: a host acks
   on the connection to P2, which deletes the item.

If the import fails, the file can be imported on P1 again.

## 4. A copy for safekeeping

- A copy is `group export` without `--to`. When it runs is the owner's
  choice: an operating-system timer (cron, launchd, Task Scheduler), which
  the user documentation shows. The product schedules nothing.
- **Restore.** When P1 is lost, P imports the last copy on P2. P1 cannot
  answer `moved`, so the others learn the new home by a link: an admin
  sends `invite` on P2, and a person who is already a member and opens it
  gets `ok` from `join` with no new row (`contracts/step-2/people.md` 2.2,
  point 3). Their browser replaces the group's home and sends `home.add` to
  the person's nodes, which replace the home they connect to for an AI in
  that group.

## 5. Limits

| | Limit | Consequence |
| --- | --- | --- |
| 1 | No live hand-off | the group is unavailable from the export until the import |
| 2 | Receipts stay on P1 | a command sent to P1, unanswered, and sent again to P2 is applied again |
| 3 | A restore holds the rows of the last copy | rows written after it are lost; a task whose row went back reports to the secretary as its host acts on it, and a person settles it |
| 4 | `moved` carries no signalling URLs | clients reach P2 through the signalling URLs they had for P1, which is the same rendezvous for one owner's nodes; otherwise they need the link of section 4 |

## 6. Where it goes

| Part | Folded into |
| --- | --- |
| section 1 | `contracts/local.md` section 1 |
| section 2 | `contracts/storage.md`, a section for the export file; `setting` key `moved` |
| section 3 | `contracts/datachannel.md` section 9, error `moved` with `home`; `flows/sync.md`, following a moved group |
| section 4 | `contracts/step-2/people.md` 2.2 and section 7, point 2; `install.md`, scheduling a copy |

Unit P-4b freezes the shape: the error `moved` with `home`. Unit T2-10
builds sections 1 to 3; T2-1a the `join` of a member and T2-1b `home.add`
for a group with an AI (section 4); U2-5 the browser's part.

`[unverified]` Nothing here was run.
