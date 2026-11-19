# Round 3: final check before backend work

Reviewed the [round-two answers](fable-round-2.md) against `rebuild` at `5994ba988`; its planning tree is identical to the requested `bf4dae9af`. No finding requires a new wire message or a coordinator decision. The cases below belong in the tasks that implement them.

## 1. Task machine

There is no unspecified default: an operation outside its allowed row returns `conflict` without a write. At `merge`, §2 takes precedence over the row-local guards: **only** a host report for the same commit (8 or 9) or an owner/admin person's cancel (15) can change the task. In B-19/B-21, exercise A5.18 against *all* task writes: card waits (17–20) and thread counters (21–22) do not get an exception. Row 24 is a group command, not a task transition; it must not dispatch an assignee turn to perform a merge.

### F-1 Deciding a card can erase the reason a task is paused
Severity: major

Status: fix in the task PR

Where: [task machine §3](../contracts/task-machine.md#3-transitions), rows 12, 16, 18, 20 and 22.

Problem: A task paused for `preempted` can still have a pending card. Deciding it recomputes the task wait as another card or null. Neither is a paused wait; the host can no longer resume via 13b. The same write can lose the `rounds` wait that a person's message should release.

Proposal: In B-19 decide the card and retain its signed act, but leave a paused task's wait alone. Recompute card waits on activation. Test deciding the last card during pre-emption, then resuming via 13b.

Confidence: verified (compared rows 18/20's “wait cleared” to §2 and the guard of 13b).

### F-2 A reviewer named while paused never gets the review
Severity: major

Status: fix in the task PR

Where: [task machine §3](../contracts/task-machine.md#3-transitions), rows 6, 11–13.

Problem: Pause a task waiting for a reviewer, then name one. Row 11 correctly sends nothing while paused. On resume, row 13 sends only to the assignee. There is no previous review item for the new reviewer to take.

Proposal: Make the §2 activation rule concrete in B-19: on resuming at review, dispatch the current commit to the current reviewer if not already dispatched, or restore the reviewer wait if none is named. Test this sequence without a second reassignment.

Confidence: verified (followed the recipients in rows 6, 11 and 13).

## 2. Items

The [inbox schema](../contracts/storage.md#inbox-items-waiting-for-ais) now fixes the signed envelope in the enqueue transaction. Changing a reviewer or commit retires superseded items in that transaction; cancellation retires all queued task items. Delivery does not rebuild the instruction from the latest task row. Approval authority remains the matching signed decision, not a row copy.

### F-3 Check held work when it is taken, not when it first arrives
Severity: major

Status: fix in the task PR

Where: [inbox §6](../contracts/datachannel.md#6-inbox-home-to-host); [turn delivery B.2–7, B.10](../flows/agent-turn.md#b-from-item-to-turn).

Problem: A host can accept a review for C1, hold it while busy, receive the task row for C2, then resume directly at B.4. Deleting the item at home doesn't remove that held copy. On reconnect, inbox replay on `hello` can also precede the host's state sync. Conversely, applying “not final” to *every* task-related item would discard row 8's secretary notice precisely when the task is merged.

Proposal: In B-15, keep authentication at receipt; make the existing current-task decision at use, after the group's state sync, with task updates ordered before the items they produce. Require active work and the current recipient; for review/merge also require the matching step and commit. Route a completion notice as information to the secretary's group session, not as work on the finished task. Test a held C1 after C2 (A5.19), a cold host with a current item, and delivery of the notice in A5.1. No extra round trip.

Confidence: verified (compared the replay point and B.10's restart point with A5.1/A5.19).

### F-4 Retention can delete the cause before a dependent task starts
Severity: major

Status: fix in the task PR

Where: [card decisions §1.4](../contracts/commands.md#14-act-and-jump); [retention §4](../contracts/storage.md#4-retention-r-135); task-machine rows 1 and 8.

Problem: A queued dependent has no inbox item yet. Its original signed confirmation lives on the confirmed brief, which is neither pending nor exempt from the daily message sweep. If its dependency merges after that sweep, row 8 cannot attach the promised cause.

Proposal: In B-19 keep the confirmed brief while a queued dependent still needs it. Advance the retention clock before releasing that dependent and assert its assignment carries the original confirmation bytes.

Confidence: verified (compared the message deletion predicate with the two enqueue points).

## 3. Reading

The home takes the snapshot and subscription cut in writer order. A row replaced after S is sent as it was at S, then as a live change. Catch-up rows and fetch pages do not advance the cursor; `synced(S)` does, then applied live rows do. A cached `gone` marker retains its sequence, so an older page cannot resurrect the row. The daily sweep now spares pending cards.

### F-5 Clear the old cursor when a reset discards its cache
Severity: minor

Status: fix in the task PR

Where: [sync A–C](../flows/sync.md#a-reading); [wire read contract §4](../contracts/datachannel.md#4-reading-a-group-home-surface).

Problem: “0 until the first synced” covers cold start, not a reset after a previous successful sync. If that reset breaks partway through, the old nonzero cursor describes rows the client has already discarded. A later delta is not a replacement for the missing baseline.

Proposal: In F-06, clear the cursor to zero with the cache on every reset; set it to S only on the new connection's matching `synced`. Test disconnect after one reset batch: next `sync.since` is zero. An interrupted *delta* keeps the prior completed cursor.

Confidence: verified (compared A.4 with the cursor definition and reconnect rule).

## 4. Adoption

[Envelope §6](../contracts/envelope.md#6-pairing-two-devices-that-have-never-met) works in [all three flows](../flows/pairing.md): in A the browser creates genesis and signs the node's add; in B the existing browser signs the new node's add; in C the existing node signs the new browser's add. In each, only an unowned recipient adopts, on the authenticated pairing connection, from a valid log containing its own add signed by that peer, with both devices valid at the tip. Owner and log are stored together; the renewed hello observes enrollment. Already-owned devices do not switch owners, and cross-person pairing changes neither log. No finding.

## 5. Receipts

[Once only](../contracts/commands.md) and [receipt storage](../contracts/storage.md#2-tables-used-by-both-roles) agree: persist the outcome under signer and nonce, never the transport id; answer using the new request's `re`. For an otherwise valid repeat, `pair.browser` returns the same still-valid link; `takeover.start` returns success only for the original terminal still open on that connection. Otherwise return `conflict`, including after loss of the live result. Neither secret nor terminal is reconstructed. A1.9/A7.6 cover expiry/use and hand-back. No finding.

## 6. Golden examples

**(a) and (b): yes, freeze their wire shapes.** (a) covers the envelope, signalling, pairing, log entries and hello/welcome/devlog; (b) covers sync/reset/rows/synced, paging, command answers and all six row types, including a deletion marker. F-5 changes cursor bookkeeping, not the frames.

**(c): freeze two complete transcripts, not the [abridged trace](../flows/trace-task.md).** For each, pin the initial group, members, device keys, task counter and sequence so expected rows are reproducible. Supply:

1. Each command in order through the summary: full payload string, real-format ids/nonces/times, `g`, signer, AI `as` where applicable, and verifiable signature over the exact prefixed bytes. Pair every transport `id` with its response `re`, including the review result.
2. The resulting rows with their sequence/position where applicable; all six home-signed inbox items with recipient, row copies and host acknowledgements. Items 2 and 3 must carry the *same original signed confirm*; item 6 must still reach the secretary after merge.
3. Remote: one submitted SHA through local/hosted checks, review, signed merge instruction (with PR number), and merged report (with separate resulting SHA). No remote: distinct reviewer AI on the same host, no PR fields or hosted-check command/result, review on submission, local merge of the reviewed SHA. Make the no-remote variant parseable on its own.

## 7. Decided choices

No conflict with a fixed rule or acceptance test. A5.18 exercises both decisions; K-14 remains the stated limit, not a reason to add a merge handshake.

## Unverified

This is a contract review. The interleavings above were traced on paper, not run against a backend or browser. No golden payloads, signatures, CLI behavior or external merge outcomes were generated or tested; the fixtures and acceptance tests are implementation work.

Ready to start
