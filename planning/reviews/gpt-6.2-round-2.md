# Round 2: fix the handoffs before freezing the wire

Reviewed `39be4751b` against the [round-one answers](fable-round-1.md). Q-1–Q-20 stand. The scaffold and spikes can proceed; the confirmed-task fixture is **not ready to freeze**. The single writer, one commit per submission, and one receipt per accepted mutation are worth keeping.

## 1. Task machine

### F-1 Assignment can start work too early or leave it without a worker
Severity: major

Where: [task machine, rows 1–3, 8, 9, 11, 24](../contracts/task-machine.md#2-transitions)

Problem: Row 11 emits `assign` even for a queued task; the host creates its worktree before its dependency merges (A5.6). Naming the **same** assignee after a conflict emits an item but only a **new** assignee clears the wait. Row 24 can replace a queued or paused wait with `assignee`, which is invalid in either state. Rows 1 and 11 also allow the secretary as reviewer, but its tool list excludes `submit_review`.

Proposal: While queued/paused, edit the assignment without dispatch; send the right item for the current step on activation. Clear the conflict wait for either assignment. Require eligible working AIs for both roles, distinct when both are set. Reject cyclic or nonexistent dependencies at confirm; a dependent of a cancelled task can be explicitly cancelled. Do not report a fresh `branch` transition when continuing a task at `local_check`.

Confidence: verified (compared the table with task flow B/E, session kinds, and MCP §2).

### F-2 Merge dispatch needs a point of no return
Severity: blocker

Where: [task machine, rows 4–17](../contracts/task-machine.md#2-transitions); [local operations §7](../contracts/local.md#7-git-and-check-commands)

Problem: Rows 4–7 handle a new submission during checks/review: a result for the old commit conflicts. But **after** a review passes, a merge instruction can be in flight while cancel, pause or reassignment still succeeds. The external merge can finish and row 8 then reject the fact. Separately, reading the PR head and reading its checks are two observations; if the head changes between them the reported result need not belong to the named commit.

Proposal: Make entry to `merge` the dispatch decision. Until the outcome, refuse task changes that would invalidate its recipient or commit, including member removal. Accept only the result or conflict from that dispatch; carry the commit on both. Obtain check results attributed to the submitted SHA. Without a remote, local §7 names the commit but never compares the task branch head as A5.3 expects. Make that comparison explicit. No rollback or merge retry (R-24).

Confidence: reasoned

### F-3 Resume does not preserve the reason a task stopped
Severity: major

Where: [task machine, rows 10–26](../contracts/task-machine.md#2-transitions); [queue jump](../flows/agent-turn.md#c-jumping-the-queue-r-72)

Problem: At the round limit, row 13 clears `rounds` but only row 23 releases held mentions. A later human message no longer releases them. A jump targeting the interrupted task itself needs a turn in the very session row 16 just paused. And after the sixth failed review, row 13 lets the secretary restart the task although R-63 requires a person to decide.

Proposal: Use the wait reason to choose the exit. A human thread message clears `rounds` and releases its mentions. A host continuing a pre-empted task supplies its own turn, not a second generic wake. A same-session jump can interrupt and take priority without pausing its own task. If the host stopped, allow explicit manual resume. After the sixth failed review, require a person's decision. Restore outstanding card waits on resume (A4.7, A5.8, A5.11).

Confidence: verified (compared the row guards, releases and recipients with R-60/R-63/R-72).

**Other rows.** Creation, branch, check, submission, review and merge have explicit wrong-state rejection; a direct start still needs one principal (`as` the working AI, as in commands §1.5, not an unspecified host in row 2). Rows 6/10 crossing the failure threshold should emit the failed outcome once, not also send a work item. Rows 18–21 resolve a pending card by identity; expire *all* old assignee approvals on reassignment. Rows 25–26 need no new task state. No general transition framework.

## 2. Signed items

### F-4 The queued instruction must retain the decision made at enqueue
Severity: blocker

Where: [inbox contract §6](../contracts/datachannel.md#6-inbox-home-to-host); [inbox storage](../contracts/storage.md#inbox-items-waiting-for-ais); [trace](../flows/trace-task.md)

Problem: The signed home instruction suffices in step 1 for ordinary routing and workflow, **not** instead of the person's signed approval. The inbox stores no commit and builds the envelope at send time. A queued review for C1 can therefore be reconstructed from a task now at C2. The trace's merge also uses a PR number supplied only in a row expressly labelled content, not authority. Delivery after reassignment or cancellation is unspecified.

Proposal: Store the instruction fields fixed by the transition (or its signed payload) in the existing inbox row. Bind the merge target as well as the commit, or resolve it from the assignee's recorded submission. Deliver only current assignments/reviews; retire superseded queued work in the transition and use current task state before acting on held work (R-90). Keep ordinary message text as content.

Confidence: verified (compared stored columns and send-time signing with trace items 4/5).

**Cause and trace.** `decided` needs the original signed approve/deny for the matching request; a signed choice is not a tool grant. Carry confirm/revise on `confirmed`; keep the signed confirm with the task for a dependent first assigned at row 8. Direct work starts from the owner's message, not a fictitious confirm. No extra person signature per ordinary step-1 item. Step 2 needs the owner's task authorization, not just a foreign home's signature. The [ask flow §E](../flows/ask.md#e-authority) misidentifies the item signer: the caller's host signs the *call*; the home signs the *item*.

## 3. Reading

### F-5 A cut is a read view, not just an upper bound
Severity: major

Where: [sync A–C](../flows/sync.md#a-reading); [row storage](../contracts/storage.md#1-home-tables)

Problem: At cut 10, a row has `seq=10`. A later write replaces it with 11. A fresh query for `seq <= 10` returns nothing: live 11 can eventually repair the view, but the answer at `synced(10)` was incomplete. Separately, dropping a deleted row also drops its version; an older fetch response applied afterwards can restore it.

Proposal: Select the catch-up set at the cut in writer order or from one read snapshot; then stream it. Order fetch results against deletion pushes, or retain the deleted row's version while older pages are outstanding. Use `pos` only for display/paging. Resume from the **completed subscription** cursor: zero during an incomplete reset, S at `synced`, then the last applied live change; never advance it from a page. Keep the current old-connection discard and purge-mark rule (A1.3, A2.2).

Confidence: verified (the cut query over the draft SQLite schema omitted the replaced row; the fetch case follows from the specified cache rule).

### F-6 Retention removes cards a live task still needs
Severity: major

Where: [retention SQL](../contracts/storage.md#4-retention-r-135); [sync G](../flows/sync.md#g-retention-and-deletion)

Problem: The message sweep deletes a pending approval older than the cutoff even if its task still waits for it. A cold client cannot show it and a live client may still offer a decision the home can no longer accept.

Proposal: Keep a pending card until resolved, or expire it visibly and clear its task wait at the retention cutoff. Do not silently delete an actionable card. Ordinary aged-out history may still be stale as documented (A2.2, A4.3).

Confidence: verified (ran the draft retention SQL against an in-memory pending message; zero pending rows remained).

## 4. Receipts

### F-7 A stored result is not a live resource
Severity: major

Where: [host commands §2](../contracts/commands.md#2-host-commands); [terminal hand-back](../flows/takeover.md#d-handing-back); [receipts](../contracts/storage.md#2-tables-used-by-both-roles)

Problem: Replaying `pair.browser` can return an expired/used link, and persisting its answer stores the pairing secret, contrary to storage §3. Replaying a successful `takeover.start` after disconnect can say a terminal is available after hand-back. A literal saved wire response also has the old connection's `re`.

Proposal: Store the outcome, not the header; echo the **current** request id. Task number, review result and ordinary mutation acknowledgments are historical and safe. For the two live results, keep the receipt but not the secret. Return the original link only while live in memory, terminal success only for its still-active direct connection, otherwise `conflict`. Never recreate either on replay. Purging by command `ts` matches the freshness window (A2.3, A7.5–6).

Confidence: verified (compared the receipt schema, ten-minute/single-use link and terminal lifecycle).

## 5. Pairing

### F-8 Bind first adoption to the authenticated enrolling peer
Severity: blocker

Where: [envelope §§4, 6](../contracts/envelope.md#6-pairing-two-devices-that-have-never-met); [pairing A–C](../flows/pairing.md)

Problem: A valid log containing a historical add is not proof that the peer on this pairing connection enrolled the newcomer for *its* person, or that the newcomer is still valid. The stated predicate leaves wrong-person adoption unspecified. Re-adoption by an already-owned device is explicitly forbidden; do not weaken that rule.

Proposal: On the live pairing connection, require an add for the local key signed by the authenticated peer, with both keys valid at the log tip. Persist owner and log together only while unowned; pairing devices already bound to different owners changes neither log. This trusts the selected peer, not an independently verified human identity. Specify when the initial `welcome.owner:false` becomes true on the client (A1.1–2).

Confidence: verified (compared the adoption predicate with the signer in all three legitimate flows; no protocol execution).

## 6. Levels

**S-4 needs outcomes, not just flags.** For each CLI, compare structured → interactive → structured on one history and record the version. At level 1, reads and permitted MCP tools work; project files stay unchanged and check/push/submit are neither offered nor accepted. Check the promised absence of execute tools separately: read-only files alone do not establish it. Cover the secretary (Q-20), reviewer and level-1 task; fixtures by kind alone cannot express both task tool sets. At level 2 an in-worktree edit succeeds; going beyond it requires the owner's terminal decision. Check project config stays off by default, level 3 survives resume, and MCP still identifies the right session. These are spike criteria, not vendor results.

## 7. Project checks

No universal addition identified. One ordinary exception is the Windows C/C++ toolchain: [documented](https://learn.microsoft.com/en-us/cpp/build/building-on-the-command-line?view=msvc-170), its developer environment supplies `INCLUDE`, `LIB` and `LIBPATH` as well as `PATH`. Let that project's check initialize its toolchain; do not inherit the whole developer environment. Keep Q-19. For step 2, first measure whether one real offline build/test **and its subprocesses** can use the checkout, toolchain and scratch space while unrelated user data stays inaccessible and outside writes/network stay denied. Measure compatibility before timing overhead; report Windows separately.

## 8. Order and the golden trace

**Order.** Keep B-16 before B-17. Test relay movement at B-09 with B-07 available; the command/row part of A1.3 needs B-11/12. B-16 ends a session on cancellation but B-19 first implements cancel: move minimal cancel to B-16 or defer that assertion. Split scheduling/limits from B-20 checks/review. Move its full no-remote A5.1 assertion to B-21, which supplies merge. B-04's captured CLI messages feed B-13, not B-10.

**Slice (c): not yet.** Use the trace to guide one concrete signed fixture after F-2/F-4. Include real group/nonce/timestamp fields on every required payload, valid ids, exact decision bytes, the task/PR/commit binding, and responses with their request ids. Include the no-remote omission of PR/cloud fields. Resolve the stale caller/home signer text. An abridged transcript with ellipses is useful documentation, not an independent golden vector.

## 9. Deletion

Delete the separate `continued` wire verb: restrict host-originated `resume` to its own pre-empted task, with row 17's existing effect (no inbox wake). R-72 still shows the pause and continuation. Keep the transition table; replace the duplicate outcome table in task flow F with row references. Once the golden fixture exists, link to it from the trace instead of maintaining two payload copies.

## Unverified

- Review of contracts, not an implementation run. Interleavings, delivery after state changes, crash timing, and the proposed transition/receipt rules are reasoned, not observed on two nodes. The SQLite checks tested the quoted queries only.
- No live pairing, WebRTC switch, browser paging, forge/check run or merge; no assertion that a particular forge query returns SHA-bound check results. No security bypass or sandbox testing performed.
- Interactive flags, effective vendor permissions/MCP tools, config isolation, history and process-token binding on both CLIs remain spike results. No OS/build matrix, MSVC build, step-2 confinement or performance measurement; the allow-list conclusion is not a portability guarantee.
