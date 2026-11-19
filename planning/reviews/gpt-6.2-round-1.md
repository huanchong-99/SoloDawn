# Round 1 — SoloDawn rebuild

Reviewed `2e1488c73`; [Q-1–Q-18](../risks.md#q-settled-by-the-coordinator-2026-11-20) stand. **Start the scaffold and spikes; don't freeze the wire fixtures.** Implementers can disagree on who authorized work, which rows arrived and which commit passed review. Evidence: **[verified]** against plan text, **[documented]** in linked upstream references, **[unverified]** where runtime evidence is missing.

## Findings

### F-1 Give a host the signed cause of the action
Severity: blocker

Where: [Inbox](../contracts/datachannel.md#6-inbox-home-to-host), [delivery B](../flows/agent-turn.md#b-from-item-to-turn), [inbox triggers](../contracts/commands.md#3-inbox-items-one-table)

Problem: `assign` can carry an unsigned system task card; after reassignment it may carry a checks row. Conversely, a signature on a post doesn't sign an item's action, assignee or revision. A host either drops valid work or acts without the required authorization. `member status` reports state, not an order to resume a particular task (R-136, A5.1/A5.12).

Proposal: Specify the signed cause for `assign`, `review`, `decided`, `resume` and `merge`, binding action, group, task, recipient and applicable revision at the host boundary. Carry the original instruction/decision unchanged. For derived actions in S1 the home is an owner device: have it sign the specific directive with the existing envelope. System rows are for display. An S2 home needs the actual owner's task authorization.

Confidence: verified (compared the required `msg.env` with the task/checks rows and `member status` effects).

### F-2 Use the same catch-up on a move as on a reconnect
Severity: blocker

Where: [Sync A–C](../flows/sync.md), [move steps 11–12](../flows/connect.md#b-while-relayed), [read protocol](../contracts/datachannel.md#4-reading-a-group-home-surface)

Problem: The old channel has a queued row when a later row arrives on the new one. Advancing the cursor loses the earlier change. `reset` likewise arrives *after* replacement rows, and writes between snapshot and subscription have no defined destination (A1.3/A2.2).

Proposal: Freeze the last applied cursor, retire the old channel (including its close callback), and `sync` on the new one. In writer order take a cut S and subscribe to changes > S. Send snapshot/delta through S, `synced(S)`, then live changes. Announce reset first or stage until `synced`. A host restarting without its cached state starts at zero. Delete subscription migration.

Confidence: reasoned

### F-3 A nonce is not an idempotency record for an update
Severity: major

Where: [Envelope §3](../contracts/envelope.md#3-envelope), [storage §1](../contracts/storage.md#1-home-tables), [sync D](../flows/sync.md#d-sending-a-command)

Problem: `task fail` increments a counter; `act confirm` updates an existing card and creates several tasks. Neither has a required result row keyed by the command nonce. Losing the response on a move can count a failure twice or return `conflict` for a successful confirm. Host commands such as `takeover.start` also have effects (A1.3/A2.3).

Proposal: Record the successful `(signer, nonce)` and required result with the mutation; reuse the existing created row where it suffices. Specify the resend lifetime and retain receipts for it. Repeat a mutation, not just `post`, in A2.3. Distinguish successful duplicates from envelope failures; `replay` is not a §9 wire error.

Confidence: verified (compared nonce rule, command effects and schema keys).

### F-4 Review and merge one immutable head
Severity: blocker

Where: [Task operations](../contracts/commands.md#15-task), [review outcome](../contracts/commands.md#18-review-outcome), [Git steps](../contracts/local.md#7-git-and-check-commands)

Problem: `submitted` requires a matching head but has no head field. Cloud checks and review carry no revision either. If the branch advances after review, a merge by PR number or branch name need not merge the reviewed tree. The reviewer is not required to differ from the assignee (R-90). Checking a dirty worktree also isn't checking its recorded commit (A5.3).

Proposal: Require a clean checked tree and one submitted commit shared by checks, review and merge. Enforce distinct AI member IDs at assignment/review. For a remote, the [gh merge manual](https://cli.github.com/manual/gh_pr_merge) documents `--match-head-commit`; without a remote squash the reviewed commit, not the branch tip. Use the existing conflict/state outcome on a mismatch. Distinguish the base branch **name** for a PR from the tracking ref for a diff.

Confidence: verified (payload fields and documented merge option; no merge run).

### F-5 Close the pairing and device-log protocol
Severity: blocker

Where: [Pairing A–D](../flows/pairing.md), [device log](../contracts/envelope.md#4-person-and-device-log), [command transport](../contracts/datachannel.md#5-commands)

Problem: A new browser is not an owner yet. Its historical log can contain several signers, but `cmd` requires the signer's own connection, and a host `device` nominally needs `to`. “Send the whole log” has no frame. Concurrent legitimate enrolments can fork too (A1.1/1.2/1.6).

Proposal: Give pairing, forwarding and catch-up one log-transfer frame containing the original entries. Limit a paired peer to bootstrap until enrolled. Pin the secret's canonical bytes in a shared MAC/room test vector. Serialize enrolments in S1 usage guidance; document ordinary forks under K-4.

Confidence: verified (compared pairing steps with `cmd`, the entry shape and the first-accepted rule).

### F-6 Account for a lost turn without redelivering it
Severity: major

Where: [Delivery B.7](../flows/agent-turn.md#b-from-item-to-turn), [secretary follow-up](../flows/task.md#h-follow-up-by-the-secretary-r-22), [approval D](../flows/approval.md#d-not-decided-in-time)

Problem: A host stops between ack and write. No event wakes a silent task's secretary. Conversely the CLI may receive the turn while the home misses the ack; host restart forgets delivery. A late approval can clear the visible wait before its notification or grant reaches the session.

Proposal: Keep K-3. Record `(home, item)` in the required delivery audit before ack/write; one high-water ID is unsafe across held sessions. Use T-12's idle notice for R-22. For A4.3 bind a one-shot grant to card, task/session, tool and original-input digest (display text isn't the input); persist it with the late decision, consume once, clear on reassignment/end. Don't redeliver a turn.

Confidence: reasoned

### F-7 Give the task machine one transition table
Severity: major

Where: [Commands §1](../contracts/commands.md#1-home-commands), [task states](../contracts/rows.md#4-task), [task flow](../flows/task.md)

Problem: The following cases yield different results depending on which document is implemented:

| Case | Missing transition or disagreement |
| --- | --- |
| Checks / submission | No entry into `pr`; `cloud(ok=false)` acts as `fail` while flow F also reports `fail`. Successful intermediate steps reset failures, so repeated hosted failures need a defined counting point. |
| Assignment in review | Filling `wait=reviewer` sends `assign` to the worker, not `review` to the newly named reviewer. An unsuccessful review also uses `why=review`, this time for the worker. Define routing by recipient/role. |
| Waits / terminal states | Define how `depends`, `rounds`, `cap`, `rate_limit`, `group` and manual pause enter/clear; do not clear a pending approval on group resume. Define failed-task resume, counter reset, removal/reassignment and session cleanup. The sixth *failed review*, not entry into round six, is the A5.4 stop. |
| Queue jump | `act` generally needs a pending card; `jump` targets ordinary text with no state. The interrupted turn's `working → waiting` precedes the jump turn's completion; it is not evidence that the preempted task can resume. |
| Busy session | The generic busy rule holds `decided`/`reply` behind the very turn waiting for the answer. Handle an answer to an open call first. `offline` both refuses and holds; leave held items unacked for A4.8. With every slot busy, wait for a free slot rather than evicting a working session. |

Proposal: For B-13 specify allowed prior `(state, step, wait)`, actor, new tuple, recipient/`why` and counter effect in one table; use it for A4.7–8, A5.4–6, A5.8–12 and A6.1. No new state-machine framework.

Confidence: verified (compared command, row and delivery tables).

### F-8 Make the read contracts work on a cold cache
Severity: major

Where: [Fetch and sync](../contracts/datachannel.md#4-reading-a-group-home-surface), [storage §1/4](../contracts/storage.md), [screens](../steps/step-1-frontend.md#what-each-screen-reads)

Problem: Pending cards outside the latest 50 disappear from “waiting for you.” Edits move old cards across page boundaries (`seq` is mutable). After a deletion marker expires an old cursor can yield an empty delta forever. Final tasks expire from creation, not finalization. A card plus its envelope can exceed 60 KiB; `fetched` has no continuation frame.

Proposal: Include pending cards on cold sync; page by creation cursor, sync by change cursor. Reset cursors older than the purge watermark, expire visible history at retention, and age final tasks from finalization. Bound fetch pages by encoded bytes and cap individual encoded rows to fit one frame (A2.2/2.5, R-34/135). Apply `state_only` to live pushes too.

Confidence: reasoned

### F-9 Specify events, principals and results that are currently implied
Severity: major

Where: [Commands](../contracts/commands.md), [tool mappings](../contracts/mcp-tools.md), [host surface](../contracts/datachannel.md#7-host-surface)

Problem: The following have no interoperable wire outcome:

| Consumer | Needed contract |
| --- | --- |
| Host status → timeline | The home writes `system`. Give signed status a task/reason for interruption, replacement, invalid sign-in, cap or rate limit; derive the corresponding row. `handback` has host-authored facts, even when rendered as system. |
| Group list / push | An already-connected second browser sees no new group after `welcome` (A2.1); send list changes. `brief` has no subscription value. An always-on node must not suppress a closed browser's push (A8.3); specify eligibility per subscribed browser. |
| Membership | Specify `(g, agent)` lookup, member-id allocation and unique display names. Group creation has no secretary; setup/replacement/removal must establish one current secretary before work (R-20). |
| MCP / cards | `start_task` needs a task number, `submit_review` an outcome, not just `ok`. Correlate rows or name result fields (`pair.browser` has one). Align `read_thread.before` id vs sequence, main-line clarification/replies, and `brief.replaces`. |
| Limits/checks | Name cap subject, units and release transition. Define hourly wake counts after ack deletion. Local §7 lets a setting disable the missing-test check; R-103 calls it universal and `group set` has no such setting. Drop the override. |
| Receiver boundary | Separate human, AI (`as` bound to host), secretary and reviewer rights. Bind `ack` to the authenticated item's host; its `id` identifies an item, not a request. Map boundary errors to §9; CLI failure needs a code and detail. |

Proposal: Add these cases to the existing command/row fixtures; no generic event bus. Required by R-20/34/60/61, A2.1, A3.6, A5.7 and A8.3.

Confidence: verified (compared wire examples, required effects and screen inputs).

### F-10 Exercise both sides of the local/hosted worktree contract
Severity: major

Where: [Local Git steps](../contracts/local.md#7-git-and-check-commands), [reassignment](../flows/task.md#reassignment), [takeover](../flows/takeover.md)

Problem: Review creation always fetches `origin`, even for `forge:none`. A second AI clones to the first AI's folder. Reassignment assumes a local branch on the new host. Secretary takeover lists clone edits but skips R-83's task flow (K-10).

Proposal: Specify one clone per node/group, a local source with no fetch for `none`, and the published ref on reassignment. Preserve the secretary's read-only level on interactive resume; A7.7's new task is for a **working** AI (Q-8/Q-15). If secretary editing is intended, **needs coordinator**: a summary is not a task branch.

Confidence: verified (compared folder paths, `forge:none` steps and R-83; no worktree experiment).

### F-11 Keep the CLI permission boundary on every execution path
Severity: major

Where: [Local §§4, 7](../contracts/local.md), [K-8/K-10](../risks.md#k-known-limits), [level mapping](../flows/approval.md#g-levels-at-a-glance)

Problem: Interactive resume lists only a session ID. It does not say how R-40/R-45 persist there. Also, a project check is owner-chosen, but the code it runs is the current task tree; K-8 places that execution outside the CLI sandbox. Read-only task sessions are still offered mutating host tools.

Proposal: Specify the same effective project/level policy on resume and which host-side operations the owner authorizes at each level, including running project checks. Explicitly settle the check execution boundary before claiming task code is confined; do not inspect commands. Test a benign outside-workspace marker and repo-config opt-in at the real boundary (A3.8/A4.4).

Confidence: reasoned

## Answers to the round-one questions

### A. Architecture

- **A1.** F-2/F-8 give stale-cache cases on a move, reset and deletion.
- **A2.** F-1 separates a signed instruction from an unsigned task card, checks row or status-derived resume.
- **A3.** Keep the small signed log. A current-device list signed by any current device cannot establish the signer's authority for a new home without prior history or a permanently available root. F-5 fixes distribution, not consensus; K-4 remains.
- **A4.** Yes, including a late approval: F-6 gives both the lost-turn window and the missing wake-up.
- **A5.** Keep the one-use permission. Asking again fails A4.3; permanently allowing the tool changes what the owner approved. A denial needs only the notification.
- **A6.** Keep one process per open session. T-6 reports ~240 MB for Claude Code; four such sessions imply ~960 MB **before** the node, not a measured total. Measure Codex and peak RSS on the smallest target (S-3/A8.6). A shared server only saves the shim design if each MCP call has a trustworthy thread identity or a per-thread endpoint; that is not established.

### B. Connectivity

| Question | Answer |
| --- | --- |
| **B1** | [RFC 8445 §8.1.1](https://www.rfc-editor.org/rfc/rfc8445#section-8.1.1) does not promise a second nomination without restart. [Pion v4.2.27](https://pkg.go.dev/github.com/pion/webrtc/v4@v4.2.27#SettingEngine.SetICERenomination) **does** offer opt-in automatic renomination. Neither proves discovery/upgrade on an already-relayed browser connection after a network change. S-1: restore the direct route on a still-live pair; measure against A1.3. Keep the relay-free probe on both ends unless all target pairs meet it natively. It costs signalling, ICE/DTLS setup, sockets and temporarily two channels, even on failure. |
| **B2** | F-2: resume from the last applied cursor with one live source. Use the actual selected pair, not a `hello.path` assertion, for the direct-only terminal rule, including after a pair change. |
| **B3** | [Pion v4.2.27 API](https://pkg.go.dev/github.com/pion/webrtc/v4@v4.2.27#SettingEngine): `SetICEUDPMux` shares a UDP port; `SetICETCPMux` with TCP network types supplies passive TCP; `SetICEMulticastDNSMode` supports querying; `SetICEAddressRewriteRules` can publish an external IP **and port**. The last consumes a mapping, it does not obtain one. S-1/S-8 must still show usable pairs with each browser and an actual router. |
| **B4** | Q-1 stands: reachable/open/mapped TCP yes; both ends behind NAT with no reachable port and UDP blocked: no promised direct path. S-9 is not on the S1 critical path. |
| **B5** | Deduplicate the offer by `(peer, conn)`, not every candidate by `conn`. The first path can carry the offer but lose the only answer; the offerer does not know the selected path for early trickle. Try gathering complete signed descriptions and returning the same answer on paths carrying duplicate offers. Removes candidate sequencing; adopt only if latency and worst signed SDP fit the 16 KiB signalling limit in S-1. Also test reciprocal host/home roles on two groups: “homes never offer” doesn't prevent both nodes offering each other. |
| **B6** | For a previously paired browser with the page available, a remembered, reachable **WSS** URL with a trusted certificate can replace public signalling; a self-hoster can supply it. It doesn't solve first discovery or reachability and isn't needed for K-2. Keep local-node signalling. A1.4 must say *direct* connections survive a rendezvous shutdown: a TURN allocation hosted there cannot. |
| **B7** | SNI routing is plausible, not guaranteed ([RFC 6066 §3](https://www.rfc-editor.org/rfc/rfc6066#section-3)); S-6 must exercise actual `turns:` clients. [pion/turn v5.1.3](https://pkg.go.dev/github.com/pion/turn/v5@v5.1.3#QuotaHandler) admits/refuses **allocations**, not bytes on an existing one. Count both directions at relay I/O against the authenticated device, across allocations, for A1.7. Its [built-in time-limited credential pair](https://github.com/pion/turn/blob/v5.1.3/lt_cred.go) uses HMAC-SHA1, not the plan's SHA256. Pick the matching issuer/verifier pair or explicitly specify the custom verifier; the browser receives an opaque password. Document restart-reset quota as a resource limit, not a durable budget. |

### C. Group home

- **C1.** Keep the 1000-row cutoff, thread fan-out and host state filter in S1; F-2/F-8 fix meaning, not throughput. Initial state includes *all* long-lived notes; 1000 is not a memory or wire-size cap.
- **C2.** Keep JSON. The first likely scan is `fetch(kind=review)` over retained chat: no matching kind index. Use the existing `keep` discriminator for retained messages and a partial ledger index only if a realistic query plan/measurement warrants it.
- **C3.** Q-13 keeps both notes and review-ledger entries until explicit deletion; intentional deletes **do** have markers. Ordinary expiry and old cursors are the separate F-8 problem.
- **C4.** Q-12: no copy, move or replication in S1. The nearly free measure is the R-134 creation warning identifying the sole home and consequence of losing it; it is not a backup.
- **C5.** Read-only cached view and `home_unreachable` suffice; no offline outbox. Local owner stop must still work with the home down. Define how a local turn ending during the outage is reflected on reconnection (current signed status), not by replaying the turn.

### D. Backend contracts

**D1.** F-1–F-11 identify the wire, storage and state disagreements. Also pin a golden example for synchronous `decided` (signed `act`, card id, original request), `confirmed` after revision, and `notice` for secretary-only events. `why` is a routing reason, never an authorization.

**D2.** Rename/change secretary already have `group set`; `host.leave`, `member remove`, `task cancel` and `forget` cover the named S1 lifecycles. No S1 rule requires deleting a whole group, deleting a task's history, or a human leave operation. Missing are signed host-event reporting, log transfer and new-group discovery (F-5/F-9), not generic CRUD.

**D3.** Testability by acceptance ID below. “Ready” means the plan supplies a determinate assertion with suitable fixtures, **not** that it passed. “Gate” names what an implementer still needs; live/platform tests are not ordinary fake-CLI CI.

| Milestone | Ready as specified | Gate or missing fixture |
| --- | --- | --- |
| M1 | A1.5; A1.8 (six build targets; measure on named runner) | A1.1–2/1.6: F-5. A1.3: F-2/F-3 plus controllable routes. A1.4: stop the server on a direct pair. A1.7: byte cutoff, not allocation admission. |
| M2 | A2.4–5; A2.6 with ordinary reconnect | A2.1 already-connected second browser: F-9. A2.2 reset: F-2. A2.3 is ready for `post` only; F-3 for commands generally. |
| M3 | A3.1–4, A3.7, A3.9 (scripted drivers) | A3.5: specify first AI waiting at eviction. A3.6: signed crash report (F-9). A3.8: actual hook in both opt-in states per CLI/OS, not a fake. |
| M4 | A4.6 (refusal and audit) | A4.1–3 need task fixtures before M5 and open-call routing (F-7); late decisions need F-6. A4.1/4.4–5 need real level mapping. A4.7–8: signed stop/resume, hold vs refuse. A4.9: signed timeout event. |
| M5 | A5.2, A5.5 (direct `fail` reports), A5.9, A5.14 | A5.1/5.6/5.10/5.12: signed delivery, revision and no-remote/hosted fixtures. A5.3: F-4. A5.4/5.8/5.11: F-7. A5.7: task-start result and real read-only scope. A5.13 applies only with a remote; S-2/3/5 must show the CLI approval boundary. |
| M6 | A6.3 (receipt plus later `re`), A6.4 | A6.1–2 need open-call routing ahead of the busy queue (F-7). |
| M7 | A7.3–4 (A7.4's samples cannot prove exclusivity; assert exit-before-start in the fake too), A7.5 given a forced relay | A7.1–2: real sign-in/PTY, trusted folder, same id (S-4/5). A7.6: distinguish orderly tab/channel close from transport failure; no specified five-second failure detector. A7.7: no-change vs commit fixture, fresh interactive id, working AI (F-10). |
| M8 | A8.6 once the machine, CLI versions and idle/open workload are named | A8.1/8.4: paired clean OS/reboot and DNS/TLS server. A8.2: define release source, artifact names and safe restart including takeover. A8.3: F-9 plus real push/mobile subscriptions. A8.5: define supported **OS/browser pairs**, not every browser on every OS; it inherits the earlier gates. |

**D4.** Keep explicit publication: `post` for speech and typed tools for visible workflow effects; never auto-post printed final text (A3.7). `post_summary` can be a secretary-only `post(kind=summary,task)`; `handback_summary` can be `post(kind=handback)` in a hand-back turn, with facts still supplied by the host. Don't fold `ask` into `post`: a waiting call has a distinct result and receipt.

**D5.** The labels are useful context, not enforcement of R-52 (S2). Derive the label from signed authority at ingress; keep author/text distinct from host headers. Explicitly require a new owner decision for changed scope. No prompt delimiter can establish a principal.

**D6.** [Documented] inputs below; not a live certification of the effective levels. Published versions at review: [Claude Code 2.1.328](https://www.npmjs.com/package/@anthropic-ai/claude-code/v/2.1.328), [Codex 0.170.0](https://www.npmjs.com/package/@openai/codex/v/0.170.0).

| CLI | Level 1 | Level 2 | Level 3 |
| --- | --- | --- | --- |
| Claude Code | `--permission-mode default --tools Read,Glob,Grep` (read built-ins only), session-scoped MCP; deny escalation. `--allowedTools` alone is not a tool restriction. | `--permission-mode acceptEdits`, `--settings` with `sandbox.enabled=true`, `sandbox.autoAllowBashIfSandboxed=true`, `sandbox.failIfUnavailable=true`; use the plan's `--permission-prompt-tool` and task worktree. S-2 must show that the effective filesystem/network policy and owner prompt meet R-41, not just that the flags parse. | `--permission-mode bypassPermissions` for an explicitly unrestricted task. |
| Codex `app-server` | `sandbox:"read-only"`, `approvalPolicy:"never"` | `sandbox:"workspace-write"`, `approvalPolicy:"on-request"` | `sandbox:"danger-full-access"`, `approvalPolicy:"never"` |

[Claude CLI flags](https://code.claude.com/docs/en/cli-reference), [sandbox settings and OS support](https://code.claude.com/docs/en/sandboxing); [Codex 0.170.0 start schema](https://github.com/openai/codex/blob/rust-v0.170.0/codex-rs/app-server-protocol/schema/json/v2/ThreadStartParams.json) and [resume schema](https://github.com/openai/codex/blob/rust-v0.170.0/codex-rs/app-server-protocol/schema/json/v2/ThreadResumeParams.json). On Codex these are **thread parameters**, with `cwd`, not app-server launch flags. On Claude Code use the plan's explicit MCP and settings sources; prove repository opt-in/out separately for each CLI. Native Windows is not listed as providing Claude's shell sandbox; `acceptEdits` alone is not level 2. Q-4 stands: return S-5 evidence to the coordinator; no silent substitution.

### E. Implementation order

- **E1.** B-00 and both spike tracks can start. Freeze B-01 fixtures in slices: pairing/log transfer, sync/move, then one confirmed task through signed review/merge. Move a minimal task fixture before B-12; M4 can't prove a task-session approval using only read-only group sessions. S-9 need not block B-06/07.
- **E2.** Split B-06/07 at signalling vs relay accounting vs direct-path handoff; B-09 at reads vs mutations; B-11 at driver lifecycle vs MCP/inbox; B-12 at permissions vs limits; B-13/14 at brief/assignment, local checks/review, then hosted checks/merge. Split B-17's push delivery from release/update. Each slice has a wire example and an observable test.
- **E3.** Script turn IDs across resume, pending MCP while busy, exact allow/deny timing, late decisions after process exit, receipt vs live reply, crash before turn end, usage/rate-limit/login events, PTY hand-back and exit-before-start. Match captured real driver messages, not fixtures derived solely from the fake. Fakes test the node, not vendor sandboxing or transcript continuity.
- **E4.** Extend S-1/S-6 with late path recovery, signed SDP size, active relay byte cutoff and renewal; S-2/3/5 with effective levels, repo-config isolation and MCP caller identity through resume. Add a small no-remote and two-host fixture for the exact checked/reviewed commit. No new spike framework.

### F. Simpler alternatives

**F1.** In [architecture §7](../architecture.md#7-choices-made-here-and-what-was-not-chosen), in order:

| Choice | Simpler sufficient alternative? |
| --- | --- |
| Single writer; sequenced rows; mutable cards | Keep all three. No client merge, event reducers or per-card tables. Separate change cursor from display order (F-8). |
| Home inboxes; process per session; session per task | Keep all three. Group-wide limits need the home; the process binds MCP identity; the task binds worktree and scope (Q-8/Q-15). |
| Home triggers merge; node pushes/opens PR | Keep both (Q-7/Q-10/Q-16). Bind them to the checked head, not an AI merge tool. |
| stdio shim; signed SDP + ephemeral DTLS | Keep both. Existing per-process token identifies the caller; no new HTTP identity or certificate lifecycle. |
| Probe-and-move | Reuse reconnect sync; delete probing only if S-1 proves native upgrade on all supported pairs. |
| Native sandbox; one binary | Keep both. No command classifier, no second distribution path. |

**F2.** No. An authenticated channel establishes its immediate peer, not the author of a later forwarded approval, task directive or device-log entry. R-136/A4.6 apply already; removing envelopes moves verification to the wrong machine.

**F3.** Q-7/Q-8/Q-15 already choose both: the home decides when, the assignee's host performs the deterministic merge; conflicts go back to the secretary. Task sessions isolate worktrees but lose cross-task memory; keep the secretary/group session, ledger and notes, not a per-message process.

**F4.** Delete subscription migration (F-2), separate summary tool spellings (D4), `note del` in single-owner S1 (`forget` suffices), the missing-test override and S2-only `confirm/list` authorization branches in S1. Store one effective level default, not two if the wildcard and `all_level3` mean the same thing. Defer simultaneous-open implementation, not Q-1's measurement.

## G. Three changes first

1. Write one confirmed-task wire trace with signed causes and one commit from checks through merge. Both implementers can then agree what a host is allowed to do.
2. Use one cursor-based sync for initial read, reconnect and relay handoff. This deletes a mechanism and makes lost rows testable.
3. Run the real CLI permission and resume matrix before building the scheduler. Fake drivers cannot establish those boundaries.

## Unverified

- **Evidence scope:** verified findings above compare planning documents, not running software. External links establish documented APIs/schema, not interoperability. No SoloDawn build, CLI login, capability test, benchmark or live acceptance test was run.
- **Transport:** actual browser/Pion renomination, ICE-TCP and mDNS across networks, router mappings/ports, WSS reachability, SDP/frame limits, end-to-end move timing, SNI, credential renewal and daily byte cutoff on active allocations. The proposed quota counting point and full-description signalling are designs, not tested implementations.
- **Drivers:** exact effective policies and tool sets on the installed Claude Code/Codex versions and OSes; Windows level 2, hook/plugin isolation, interactive permission continuity, PTY close latency, approval timing/digest stability, per-thread MCP identity, usage and sign-in events. The RAM estimate extrapolates T-6; no Codex comparison.
- **Storage/workflow:** proposed concurrency, crash and late-decision outcomes are reasoned from ordering; no crash injection or database workload. Ledger scan cost, cache lifetime, frame sizes, expected group sizes, secretary follow-up timing and achievable check isolation were not measured.
- **Git/shipping/UI:** no live worktree, forge, pending-check or conflict run; checksum/update, OS service lifecycle, browser-keys matrix and real mobile push remain untested. External mock-ups and quality gates were outside the review scope.
