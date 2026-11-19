# Round 1: answers

Answers to `reviews/gpt-6.2-round-1.md` (reviewed commit `2e1488c`).
Decided points Q-1 to Q-18 were not reopened by the review and are
unchanged.

Summary: eleven findings; eight accepted, three accepted with changes, none
rejected. No proposal was turned down for adding defence inside the system,
because the review made none: every accepted change closes a gap at a
boundary (the network, the CLI, Git) or removes a mechanism. Where a
proposal was only partly taken, the part left out and the reason are stated.

New files from this round: `contracts/task-machine.md`,
`contracts/drivers.md` (sections 4 to 6 of the old `contracts/local.md`),
`flows/trace-task.md`. Backend task ids were renumbered when the list was
cut into smaller slices.

## Findings

### F-1
Decision: accepted with changes
Change: Every inbox item is now an envelope signed by the home, naming the
host, the AI, the reason, the task, the message and, for `review` and
`merge`, the commit. Rows inside an item are content only. The decision
that led to an item (`act confirm`, `act approve`) travels unchanged as
`cause`. In step 1 the home is one of the owner's devices, so the host's
rule is "signed by the home of that group, and that device is in my owner's
log", plus the existing end-to-end check of the approver's signature. The
step 2 rule (the owner's own signature per task when the home is someone
else's) is stated as a requirement, not designed. `why` is routing only.
Pre-emption no longer derives a resume from `member status`: the host
reports it (see F-7). Files: `contracts/datachannel.md` 6;
`contracts/commands.md` 3; `flows/agent-turn.md` B.2; `architecture.md` 4;
`contracts/storage.md` (`inbox.cause`); `flows/trace-task.md`; acceptance
A3.10.

### F-2
Decision: accepted
Change: Subscription migration is deleted. A move is a reconnect: the client
stops applying from the old connection, closes it, and sends `sync` with the
last sequence number it applied. The home takes a cut S in writer order,
subscribes the connection to changes after S, sends what is missing up to S,
answers `synced(S)`, then sends live changes. `reset` is its own push and
comes before the rows. A row is applied only if its `seq` is higher than the
cached one. A restarted host starts from zero. Files: `flows/sync.md` A and
C; `flows/connect.md` steps 11, 12, 17, 18; `contracts/datachannel.md` 3
and 4; acceptance A1.3, A2.2.

### F-3
Decision: accepted
Change: Every accepted command gets a receipt keyed by signer and nonce,
holding the answer, written in the transaction of the change; a repeat gets
the stored answer. Commands whose `ts` is more than 24 hours from the
receiver's clock are refused, and receipts are kept that long. The same
table serves host commands. A duplicate is not an error and `replay` is not
an error code. One point was not taken: reusing the created row as the
record where that would suffice. One receipt for every command is a single
mechanism; two would be more code. Files: `contracts/commands.md` ("Once
only"); `contracts/envelope.md` 3; `contracts/storage.md` 2 (`receipt`) and
4; `contracts/datachannel.md` 5; `flows/sync.md` D; acceptance A2.3, A5.15.

### F-4
Decision: accepted
Change: One commit runs through checks, review and merge. `run_checks`
refuses a dirty worktree and records the head commit; `submitted` carries
`commit`, which becomes the task's `commit`; `cloud`, the review body and
the `merge` item carry it, and the home refuses anything about another
commit. The merge uses `--match-head-commit`, or squashes exactly that
commit without a remote; a moved head is reported as a conflict. A reviewer
equal to the assignee is refused at confirm and at assignment. The base
branch name is now distinct from the ref used for branching and diffs.
Files: `contracts/task-machine.md` rows 4 to 9 and section 3;
`contracts/commands.md` 1.5; `contracts/rows.md` 4 and review body;
`contracts/local.md` 7; `flows/task.md` B to E; `flows/trace-task.md`;
`contracts/mcp-tools.md`; acceptance A5.3, A5.4.

### F-5
Decision: accepted
Change: Log entries travel only in a new frame, `devlog`, as the original
signed entries, in pairing, forwarding and catch-up; the `device` commands
are gone, so the "signer must be the connection's device" rule no longer
collides with forwarded entries. A paired device that is not yet in the log
may send `hello` and `devlog` and nothing else. A device that belongs to no
person adopts the person of a valid log that arrives on a connection it
paired on and that adds it. The canonical bytes of the pairing secret are
fixed and covered by a shared test vector. Forks from concurrent enrolment
are documented with the guidance "one device at a time". Files:
`contracts/datachannel.md` 3.1 and 8; `contracts/envelope.md` 4 and 6;
`flows/pairing.md`; `contracts/commands.md` (removed); `risks.md` K-4;
acceptance A1.1.

### F-6
Decision: accepted
Change: K-3 stays. Before acknowledging an item the host writes the audit
row `delivered` with the home and the item id; an item already recorded is
acknowledged and skipped, also after a restart. The in-memory list is gone.
The quiet-task timer of the former T-12 is adopted: one `notice` to the
secretary for an active task with no thread message for
`limits.idle_notice_minutes`. Approvals have a durable record on the host,
written when the request is posted: card, task, tool, digest of the input as
the CLI sent it. A late approval marks it granted in the same transaction
as the delivery record; it allows one request with that digest and is
removed on use, on denial, on reassignment and at the end of the task.
Files: `flows/agent-turn.md` B.7; `flows/approval.md` B to D;
`contracts/storage.md` 3 (`approval`, `audit`); `contracts/task-machine.md`
row 25; `flows/task.md` H; `risks.md` K-3, T-3, T-12; acceptance A3.10,
A4.3, A5.16.

### F-7
Decision: accepted
Change: `contracts/task-machine.md` is the single table: for every command,
the allowed prior state, the new state, step and wait, the items and the
counters. Decisions taken while writing it: `pr` is a position on the card,
not a resting step; a failed hosted check is reported once, by `task cloud`;
failures are consecutive and reset only when hosted checks pass (or a
submission is accepted without a remote); `review` goes to the reviewer and
the new `reviewed` to the assignee; naming a reviewer at step `review` sends
`review` to that reviewer; the sixth failed review stops the task;
`jump` is its own command, and pre-emption is reported by the host
(`task preempted`, `task continued`) instead of being derived; an answer to
an open tool call is never queued behind the turn that waits for it;
"offline" holds items unacknowledged; a full node waits for a free slot
rather than closing a working session; group resume skips tasks that wait
for a card. Three wait kinds were removed (`cap`, `rate_limit`, `group`):
they describe the AI or the group, which the client already reads from
their rows. Files: `contracts/task-machine.md`; `contracts/commands.md`
1.4, 1.5, 3; `contracts/rows.md` 4; `flows/agent-turn.md` A to C;
`flows/task.md` F; acceptance A4.7, A4.8, A5.4, A5.5, A5.11, A6.1.

### F-8
Decision: accepted
Change: A cold read includes every pending card. Messages have `pos`, the
sequence number at creation, used for order and paging; `seq` is only the
change cursor. A cursor below the group's purge mark is answered with a
reset. A finished task is aged from when it finished. A `fetch` page is
bounded by bytes; an envelope payload is at most 24 KiB so a row always
fits one message. `state_only` applies to live pushes too. Files:
`flows/sync.md` A to C and G; `contracts/datachannel.md` 2 and 4;
`contracts/rows.md` 5; `contracts/storage.md` 1 and 4; acceptance A2.2,
A2.5.

### F-9
Decision: accepted with changes
Change, by row of the finding:
- Host status to timeline: `member status` carries an `event`; the home
  writes the `system` row from it. `handback` is authored by the AI with
  facts added by its host. (`contracts/commands.md` 1.2, 1.9;
  `contracts/rows.md` 5.)
- Group list and push: a `groups` push when a peer's list changes; `brief`
  added to the push events; a reminder goes to each subscribed browser that
  is itself not connected, whatever else of that person is.
  (`contracts/datachannel.md` 3; `contracts/commands.md` 1.7; A2.1, A8.3.)
- Membership: unique names enforced on `add_ai`; one secretary, replaced by
  `group set`, with a `no_secretary` notice when removed.
  (`contracts/commands.md` 1.1, 1.2.)
- Tools and cards: commands may answer with named result fields
  (`task start`: `num`; review: `passed`, `round`); `read_thread.before` is
  a `pos`; `question` is allowed in the main line; `replaces` supersedes
  the old brief. (`contracts/commands.md`; `contracts/mcp-tools.md`.)
- Limits and checks: caps are named (day and task; unit `turns` always,
  other units where the CLI reports them; when each is released); the
  hourly wake count is kept in memory; the missing-test check can no longer
  be switched off. (`contracts/commands.md` 1.3, 2; `contracts/local.md` 7.)
- Receiver boundary: the three kinds of signer are defined; `ack` names its
  item in `item` and is honoured only from that item's host; a CLI failure
  has the code `cli`. (`contracts/commands.md` intro;
  `contracts/datachannel.md` 6, 9.)
The change from the proposal: cap units are kept to what a node can always
count, with the others optional, because which figures each CLI reports is
still a spike question.

### F-10
Decision: accepted
Change: One clone per node and group. Nothing fetches without a remote. On
reassignment the previous node pushes the branch first, and the new node
builds its worktree from the published branch, or from the base branch if
nothing was published. The interactive CLI is started with the session's
own level, so the secretary's terminal is read only and the former K-10
disappears; this is listed as Q-20 for the coordinator because it narrows
"open terminal" for the secretary. Files: `contracts/local.md` 2 and 7;
`flows/task.md` "Reassignment"; `flows/takeover.md` A, B, D;
`contracts/drivers.md` 1.3; `risks.md` K-10, Q-20; acceptance A7.7, A7.8.

### F-11
Decision: accepted with changes; one part needs coordinator
Change: Interactive resume uses the same level, setting sources and MCP
configuration as the structured start. A table says which operations the
node itself performs for a session at each level; a level-1 task session is
not offered `push_branch`, `run_checks` or `submit_task`. A live test writes
a marker outside the worktree at level 2. Not solved, and not hidden: the
project's check commands run as the owner, outside the CLI's sandbox, on
code the AI wrote. The plan now says so wherever level 2 is described and
no longer implies that task code is confined. What to do about it is Q-19.
Files: `contracts/drivers.md` 1.3 and 2; `flows/approval.md` G;
`contracts/mcp-tools.md`; `risks.md` Q-19, K-8; acceptance A3.8, A4.10.

## Answers to the review's sections A to G

| Item | Decision | What changed, or why not |
| --- | --- | --- |
| A3, A5, A6 | agreed | nothing to change; memory is measured in A8.6 |
| B1 | accepted | S-1 now tries pion's opt-in renomination; the probe stays unless every supported pair upgrades by itself |
| B2 | accepted | F-2; the direct-only rule for the terminal uses the pair the host's own stack selected, and `hello` no longer carries a path |
| B3 | accepted | setting names recorded as documented by the review, to be shown working in S-1 and S-8 |
| B5 | accepted with changes | offers, answers and candidates all go on every path and are told apart by sender and `conn`; two nodes may be clients of each other. Complete descriptions are **not** adopted now: S-1 measures them, and `ice` is removed if they are good enough |
| B6 | accepted with changes | A1.4 says "direct"; a `wss://` address in front of a node is noted as a rendezvous a browser can use. No new mechanism |
| B7 | accepted | quota counted on relayed data across allocations; credentials in the usual HMAC-SHA1 form; the quota is a resource limit that restarts with the service; S-6 uses real `turns:` clients |
| C1 | accepted with changes | notes are no longer part of `sync`; they are read with `fetch`, so the first read does not grow with the memory |
| C2 | accepted | one partial index on the `keep` rows serves the ledger and the notes |
| C3, C4 | agreed | already as decided (Q-13, Q-12) |
| C5 | accepted | "all my AIs offline" works with the home down; a host sends its current status on reconnect; nothing is replayed |
| D1 | accepted | the golden examples are frozen in three slices, the third being `flows/trace-task.md` |
| D2 | agreed | no new CRUD |
| D3 | accepted | acceptance tests reworded: A1.4, A1.7, A2.1, A3.5, A3.6, A4.8, A7.4, A7.6, A8.2, A8.3, A8.5, A8.6; supported system and browser pairs are named |
| D4 | accepted | `post_summary` and `handback_summary` are gone; both are kinds of `post` |
| D5 | accepted | the label is derived from signatures and stated to enforce nothing |
| D6 | accepted | flags and thread parameters recorded as documented by the review; S-2 and S-3 must show the effective limits |
| E1, E2 | accepted | backend list cut into smaller tasks; a minimal task session comes before permissions, since cards only arise in task sessions |
| E3 | accepted | the fake CLIs' script list, and shapes taken from captured real messages |
| E4 | accepted | spike questions added to S-1 to S-4 and S-6 |
| F1 to F3 | agreed | no change |
| F4 | accepted with changes | removed: subscription migration, the two summary tools, `note del`, the missing-test override, the step 2 `authz` branches, the wildcard level. Kept: `default_level` together with `all_level3`, because "all projects unrestricted" must be switchable off without losing the per-project levels |
| G1 | accepted | `flows/trace-task.md` |
| G2 | accepted | F-2 |
| G3 | accepted | the permission task (B-17) waits for the answers of S-2, S-3 and S-5 |

## Open points raised by this round

- **Q-19** (from F-11): where the project's checks run at level 2.
- **Q-20** (from F-10): the secretary's terminal is read only.

Both were decided by the coordinator after this file was first written:
Q-19 as "keep it in step 1, with a minimal environment and a plain
statement; an operating-system sandbox in step 2", Q-20 as accepted. See
`risks.md`.

## For round 2

In order of how much a mistake would cost:

1. `contracts/task-machine.md`: is every command covered for every state it
   can meet, and do rows 5 to 7 (resubmission), 11 (assignment), 16 and 17
   (pre-emption), 22 and 23 (rounds) leave no state without an exit?
2. `contracts/datachannel.md` 6 and `flows/trace-task.md`: is "signed by the
   home, which is in my owner's log" enough for every item in step 1, and is
   `cause` carried where it should be?
3. `flows/sync.md` A to C with `flows/connect.md` step 11: the cut, the
   reset, `pos` against `seq`, the purge mark, and the 24-hour receipt.
4. `contracts/envelope.md` 6 and `flows/pairing.md`: the adoption rule. Can
   a device be made to adopt the wrong person?
5. `contracts/drivers.md` 1.3 and 2: the interactive start and the level-1
   tool set.
6. The environment allow list for check commands (`contracts/local.md`
   section 7), now that Q-19 is decided.
7. `steps/step-1-backend.md`: the new slicing and its order.
