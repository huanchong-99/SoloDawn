# Parallel plan: units of step 1

The units of step 1, by lane. Rules, lanes, waves and the critical path are
in `steps/parallel-plan.md`; steps 2 and 3 are in
`steps/parallel-plan-step-2-3.md`.

Columns: **W** is the wave in which the unit starts. **Does** gives the goal
and the contract sections. **Tests** names acceptance tests of
`steps/step-1-acceptance.md`; "unit" means the unit's own scenarios only.
**Owns** lists files; every unit also owns its
`features/<module>_<slice>.feature` and step file. **Needs** lists units
that must be merged first. **M** is the builder's model, F for Fable 5.2 and
A for GPT-6.2 Astra; the reviewer is the other. **Sz** is S, M or L.

Paths are short: `home/` is `internal/home/`, `host/` is `internal/host/`,
`node/` is `internal/node/`, `peer/` is `internal/peer/`, `lib/` is
`web/src/lib/`, `cmp/` is `web/src/components/`, `routes/` is
`web/src/routes/`.

## 1. Seams, wire shapes, test infrastructure

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| X-1 | 1 | Step registry for godog: each step file registers itself from `init`; the four allowed dependencies not yet in `go.mod` (MCP SDK, pseudo-terminal with ConPTY, UPnP/NAT-PMP, Web Push), held by a root test file | existing suites still pass | `features/acceptance_test.go`, `go.mod`, `go.sum`, `deps_test.go` | — | A | S |
| P-1 | 1 | Freeze payloads of `group`, `member`, `post` (all kinds), `act`, `jump`, `task`, `note`, `forget`, `push`, the review outcome and host events; `inbox`, signed `item`, `ack`; `host.join`, `host.leave` (`commands.md` 1, 3; `datachannel.md` 5, 6) | golden round trip in Go and web | `internal/proto/**`, `lib/proto.ts`, `lib/proto.test.ts`, `lib/mock.ts` | — | F | L |
| P-2 | 2 | Freeze `groups`, `host.info`, `audit`, `pty.resize`, `beacon`; payloads of `settings.set`, `pair.browser`, `agent.set`, `perm.set`, `host.offline`, `host.online`, `takeover.start`, `takeover.end` (`commands.md` 2; `datachannel.md` 3, 7; `rendezvous.md` 4); leave `trace-*` files out of the generic file map | golden round trip | as P-1 | P-1, B-07 | F | M |
| P-3 | 3 | B-01 slice (c): two complete signed transcripts of one task, with and without a remote (`flows/trace-task.md`; backend list, "Fix in the task", B-01) | both transcripts parse alone and verify | `internal/proto/trace_test.go`, `testdata/trace-*.json`, `lib/trace.test.ts` | P-2 | F | L |
| NET-1 | 1 | Network simulation on Linux: namespaces with NAT of each type, blocked UDP, blocked ICE-TCP port, a UPnP gateway; started from Go tests | unit; used by A1.3, A1.4, A1.7, A7.9 | `features/netsim/` | — | A | M |
| E-0 | 2 | Playwright project file for Chromium, Firefox and WebKit that starts real nodes and a rendezvous with fake CLIs; one smoke scenario | unit | `web/e2e/config.ts`, `web/e2e/nodes.ts`, `web/e2e/smoke.spec.ts` | F-01c, B-13 | A | M |
| C-0 | 1 | Planning: `conventions.md` section 5 names P-1 and P-2 as the freezing units; the feature-file convention of the parallel plan | — | `planning/contracts/conventions.md` | — | F | S |

## 2. Lane HOME

Every command file registers its function in the table of HM-0. The command
functions are pure: (state, command) to (rows, items, answer, error).

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| HM-0 | 1 | Group state in memory, the command table, `Apply`; the `Client` interface (`architecture.md` 8.3); principal checks and receipts (`commands.md` head; `datachannel.md` 5, 8) | A2.3, A2.4 | `home/home.go`, `home/check.go`, `features/home.feature` | — | F | M |
| HM-M | 1 | The task machine: rows 1 to 24 and the rules of section 2 as one pure step function over a task row (`task-machine.md`); R3-1 and R3-2 | unit tests for every row; A5.18 at step `merge` | `home/machine.go` | — | F | L |
| HM-1 | 2 | Run loop: load, write-through to the store, pushes to clients with `state_only`, row change before its items (`datachannel.md` 4, 6) | A2.1 | `home/run.go` | HM-0 | F | M |
| HM-2 | 2 | `sync`: the cut, `reset`, delta or cold content, pending cards (`datachannel.md` 4; `flows/sync.md`) | A2.2 | `home/sync.go` | HM-0 | F | M |
| HM-3 | 2 | `fetch` by `pos` and by `kind`, bounded by bytes | A2.2 (scroll back), A2.7 (last sentence) | `home/fetch.go` | HM-0 | A | S |
| HM-4 | 2 | Retention: the daily run, purge mark, pending cards kept, confirmed brief of a queued task kept (R3-4) (`storage.md` 4) | A2.5, A5.14 | `home/retention.go` | HM-0 | A | M |
| HM-5 | 2 | `group`, `member`: unique names, secretary rules, `add_ai` (`commands.md` 1.1, 1.2) | A2.1 | `home/cmd_group.go`, `home/cmd_member.go` | HM-0, P-1 | A | M |
| HM-6 | 2 | `post` kind `text`; `note`; `forget` with `gone` markers (`commands.md` 1.3, 1.6) | A2.1, A5.14 | `home/cmd_post.go`, `home/cmd_note.go` | HM-0, P-1 | A | M |
| HM-7 | 2 | Inbox: which AI a row wakes, signed items, the wake-up cap, removal of an item that is no longer true (`commands.md` 3; `flows/agent-turn.md`) | A3.10 (home half), A5.19 (home half) | `home/inbox.go` | HM-0, P-1 | F | M |
| HM-8 | 2 | `post` kind `brief`; `act confirm` and `revise` with the `cause`; dependencies and queued tasks (`messages.md` 2.1, 3.1; machine row 1) | A5.2, A5.6, A5.15 | `home/cmd_brief.go` | HM-0, HM-M, P-1 | F | M |
| HM-9 | 2 | `task`: `start`, `branch`, `assign`, `pause`, `resume`, `cancel`, `fail`, `preempted` (`commands.md` 1.5; rows 2, 3, 10 to 16) | A5.5, A5.7 (home half), A5.18 | `home/cmd_task.go` | HM-0, HM-M, P-1 | F | M |
| HM-10 | 2 | Cards: `post` kind `approval` and `decision`; `act approve`, `deny`, `choose`; expiry and reassignment (`messages.md` 2.3, 2.4, 3.2, 3.3; rows 17 to 20) | A4.1, A4.2 (home half) | `home/cmd_card.go` | HM-0, HM-M, P-1 | F | M |
| HM-11 | 2 | Checks and review: `task checked`, `submitted`, `cloud`; `post` kind `review`; the outcome; the six-review stop (`commands.md` 1.8; rows 4 to 7, 14) | A5.3, A5.4 | `home/cmd_review.go` | HM-0, HM-M, P-1 | F | M |
| HM-12 | 2 | Merge: the `merge` item when a review passes, `task merged`, `task conflict`, `task_merged`, `post` kind `summary` (rows 8, 9) | A5.10, A5.12 | `home/cmd_merge.go` | HM-0, HM-M, P-1 | F | M |
| HM-13 | 2 | Rounds in threads, held mentions, `jump`, the quiet-task timer (rows 21 to 23; `commands.md` 1.4) | A5.8, A5.11, A5.16 | `home/rounds.go` | HM-0, HM-M, P-1 | A | M |
| HM-14 | 2 | `group pause` and `resume` (row 24; backend list, B-18 note) | A4.7 (home half) | `home/cmd_pause.go` | HM-0, HM-M, P-1 | A | S |
| HM-15 | 2 | Calls: `post` kind `call`, the answer, depth, receipt (`messages.md` 2.6, 3.4; `flows/ask.md`) | A6.3, A6.4 (home half) | `home/cmd_call.go` | HM-0, P-1 | A | M |
| HM-16 | 2 | Host events: member status, usage, `turn_interrupted` (`commands.md` 1.9) | A3.6 (home half) | `home/cmd_hostev.go` | HM-0, P-1 | A | S |
| HM-17 | 2 | Web Push: `push`, eligibility per browser, the short summary (`commands.md` 1.7; `storage.md` `push_sub`) | A8.3 (backend half) | `home/webpush.go` | HM-0, P-1, X-1 | A | M |

## 3. Lane HOST

Every tool file registers its tools in the table of HS-0.

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| HS-0 | 1 | Interfaces to a home, the owner's connection and the terminal stream (`architecture.md` 8.3); the driver interface and its events; the session table and state machine (`sessions.md`); the tool table; the audit write | unit: every state change of `sessions.md` 2 | `host/host.go`, `host/session.go`, `host/audit.go`, `features/host.feature` | — | F | M |
| HS-1 | 2 | Claude Code driver (`drivers.md` 1.1) against the fake CLI | A3.1 (driver half), A3.6 | `host/claude.go` | HS-0, B-13 | A | M |
| HS-2 | 2 | Codex driver (`drivers.md` 1.2) against the fake CLI | A3.2 (driver half) | `host/codex.go` | HS-0, B-13 | A | M |
| HS-3 | 3 | CLI detection and sign-in (`drivers.md` 1.4); `agent.set` | A3.4 as seen in `host.info` | `host/agents.go` | HS-0, P-2 | A | S |
| HS-4 | 1 | Workspaces: one clone per group, worktree and branch per task, read-only checkout for a review, removal (`local.md` 2, 7) | A5.1 (folders), A5.2, A5.6 | `host/workspace.go` | — | A | M |
| HS-5 | 1 | Check runner: exactly the strings of `group.checks`, clean tree, minimal environment, both hard checks (`local.md` 7) | A5.3 (first sentence), A5.9, A5.17 | `host/checks.go` | — | A | M |
| HS-6 | 1 | Outbound secret check (`local.md` 8) | A3.9 (the check) | `host/secret.go` | — | A | S |
| HS-7a | 1 | Forge, outward: push the task branch, open the pull request, read hosted checks with the head in one answer (`local.md` 7) | A5.13 (first sentence) | `host/forge.go` | — | A | M |
| HS-7b | 1 | Merge of the reviewed commit with and without a remote, conflict, a restart in the middle (`local.md` 7; `flows/task.md`) | A5.3 (last sentence), A5.10 (host half) | `host/merge.go` | — | A | M |
| HS-8 | 2 | MCP server and the stdio shim over the local socket; the caller from the shim's arguments; the tool list test (`mcp-tools.md` head, 5) | unit: tool list equals the contract | `host/mcp.go`, `host/shim.go` | HS-0, X-1 | A | M |
| HS-9 | 2 | Tools `post`, `read_thread`, `list_members`, `list_tasks`, `note_put`, `note_list` (`mcp-tools.md` 1) | A3.7, A3.9 | `host/tools_talk.go` | HS-0, HS-6, P-1 | A | M |
| HS-10 | 2 | Tools `start_task`, `run_checks`, `submit_task`, `push_branch`, `submit_review` (`mcp-tools.md` 1) | A5.3, A5.7 (host half) | `host/tools_task.go` | HS-0, HS-4, HS-5, HS-7a, P-1 | A | M |
| HS-11 | 2 | Secretary tools `propose_brief`, `set_task`, `request_decision` (`mcp-tools.md` 2) | unit | `host/tools_sec.go` | HS-0, P-1 | A | S |
| HS-12 | 2 | `ask` and `reply`: the open-call table, depth per turn, the answer ahead of the queue, wait and receipt (`flows/ask.md`) | A6.1, A6.3, A6.4 (host half) | `host/tools_ask.go` | HS-0, P-1 | F | M |
| HS-13 | 2 | Delivery: check an item when it arrives and when it is taken (R3-3), audit first, `ack`, once across a restart, the item-to-session table, one turn per AI per group, the cursor on `reset` (R3-5) (`flows/agent-turn.md`; `datachannel.md` 6) | A3.10 | `host/deliver.go` | HS-0, P-1 | F | M |
| HS-14 | 2 | Turn text and the instructions per session kind (`drivers.md` 3, 4) | unit: exact text per item kind | `host/turn.go`, `host/instructions/` | HS-0, P-1 | F | M |
| HS-15 | 2 | Session runtime: start and resume, dormancy, `max_open`, crash and failed resume, the switch under C-6 | A3.4, A3.5, A3.6, A7.4 (ordering) | `host/run.go` | HS-0 | F | M |
| HS-16a | 2 | Approvals: the `approve` tool and Codex requests behind one handler, records, the wait, the late decision, the check of `cause` (`mcp-tools.md` 3; `flows/approval.md`) | A4.1 to A4.3, A4.6 (host half) | `host/approve.go` | HS-0, P-1 | F | M |
| HS-16b | 3 | Levels by session kind for both CLIs, what the node offers at each level, `perm.set` (`drivers.md` 2) | A4.4, A4.5 | `host/level.go` | HS-1, HS-2, P-2 | F | M |
| HS-17 | 3 | The `audit` request (`datachannel.md` 7) | A4.1 (audit sentence) | `host/audit_read.go` | HS-0, P-2 | A | S |
| HS-18 | 3 | Usage and caps, the turn time limit, `host.offline` and `host.online` | A4.8, A4.9 | `host/limits.go` | HS-15, P-2 | A | M |
| HS-19 | 2 | Pseudo-terminal: interactive start (`drivers.md` 1.3), bridge with back-pressure, resize; Unix and ConPTY | unit on Linux and on a Windows machine | `host/pty.go`, `host/pty_unix.go`, `host/pty_windows.go` | HS-0, X-1 | A | M |
| HS-20 | 3 | Takeover: choice of session, a task for a working AI without one, the state switch, hand-back facts and turn, `direct` only, a repeated start (`flows/takeover.md`) | A7.3 to A7.7 (host half) | `host/takeover.go` | HS-15, HS-19, P-2 | F | M |
| HS-21 | 2 | `host.join`, `host.leave`; one connection per home of the host's AIs | A3.3 (host half) | `host/join.go` | HS-0, HS-4, P-1 | A | S |

## 4. Lanes NODE and PEER

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| N-0 | 1 | Data directory, local socket, and `node`, `pair`, `pair-browser`, `open`, `status` as argument parsing (`local.md` 1 to 3) | unit; A1.1 (`status` prints the fingerprint) | `node/dir.go`, `node/socket.go`, `cmd/solodawn/`, `features/cmd.feature` | — | A | M |
| N-1 | 2 | The local link: a host and a home on one node through the interfaces of HM-0 and HS-0; a test helper that starts several nodes with fake CLIs | unit: a command and a push cross the link | `node/local.go`, `node/nodetest/` | HM-0, HS-0 | A | M |
| N-2 | 2 | `ctl` session: `hello`, `welcome`, the bootstrap rule, `devlog` both ways and forwarding, the request table, receipts for host commands (`datachannel.md` 3, 3.1, 8) | A1.1 (second half), A1.6 | `node/session.go`, `node/devlog.go` | HM-0, HS-0, B-08 | F | M |
| N-3 | 3 | Pairing over a room; `pair.browser` with its nonce; `settings.set`, `host.info`, `groups` (`flows/pairing.md`; `commands.md` 2) | A1.1, A1.9 | `node/pair.go`, `node/surface.go` | N-2, P-2 | A | M |
| N-4 | 3 | Wiring: start-up of store, identity, peer, the signalling handler's local delivery, home and host; `solodawn node` | A1.2, A1.8 | `node/node.go`, `features/node.feature` | N-1, N-2, HM-1, HS-15, B-08 | F | M |
| N-5 | 1 | `service install` and `remove` on three systems (`local.md` 1) | A8.1 (service half), on Devin machines of each system | `node/service_*.go` | — | A | M |
| N-6 | 1 | Self-update: sum check, replace, never during a turn or takeover (`local.md` 9) | A8.2 | `node/update.go` | — | A | M |
| N-7 | 1 | Release workflow: six binaries, `SHA256SUMS`, `install.md` | A1.8 (builds) | `.github/workflows/release.yml`, `install.md` | — | A | S |
| N-8 | 3 | `net_status`; the retention schedule | unit | `node/netstatus.go`, `node/schedule.go` | N-4 | A | S |
| PR-1 | 2 | The relay-free probe and the move as a reconnect; classifying again (`flows/connect.md` B) | A1.3, A1.13 | `peer/probe.go` | B-08, B-07 | F | M |
| PR-2 | 3 | Local-network beacon and remembered addresses (`rendezvous.md` 4) | A1.4 | `peer/beacon.go`, `peer/addrs.go` | B-08, P-2 | A | M |
| PR-3 | 2 | Port mapping by UPnP and NAT-PMP, publishing the granted port (`rendezvous.md` 5.1) | unit against the gateway of NET-1 | `peer/portmap.go` | B-08, X-1, NET-1 | A | M |
| RV-1 | 2 | HTTPS and TURN over TLS on one port by server name; certificates by flags (`rendezvous.md` 3, 6) | A8.4 against a local certificate authority | `internal/rendezvous/tls.go` | B-07, S-6 | A | M |

## 5. Lanes WEB-LIB, UI-SHELL and UI

W units hold logic and types only. U units hold every visual decision and
are built by Fable against the golden files through `mock.ts`.

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| W-1 | 1 | `rendezvous.ts`: handshake, `to`/`from`, `watch`, rooms, the `ice` request, over every signalling path (`rendezvous.md` 2) | A1.5 (browser half) | `lib/rendezvous.ts` | — | A | M |
| W-2 | 1 | `peer.ts`: signed offer and candidates, early candidates held, `ctl` requests and pushes, path from the browser's statistics, reconnect with back-off, unanswered commands sent again | A1.11 (browser half) | `lib/peer.ts` | — | F | M |
| W-3 | 2 | The probe and the move as a reconnect (`flows/connect.md` B) | A1.3, A1.13 (browser half) | `lib/move.ts` | W-2 | F | M |
| W-4 | 1 | One active tab: the lock, losing it, "Use here" as logic (`flows/connect.md` F) | A1.12 (logic) | `lib/tablock.ts` | — | A | S |
| U-0 | 1 | Shell seams: message files per area with one loader, the route table with all seven pages, the connection handed to pages (mock or real), the per-slice feature convention for the web | existing `web.feature` passes | `web/src/App.svelte`, `lib/route.svelte.ts`, `web/src/messages/`, `web/features/` | F-06/F-07 | F | M |
| U-1 | 2 | F-04 onboarding page | A1.1 (browser half) | `routes/Onboarding.svelte`, `cmp/pairing/` | U-0 | F | M |
| U-2 | 3 | F-05: indicator with four states; computers page part 1; `nodes.svelte.ts` | A1.2, A1.6 | `routes/Nodes.svelte`, `cmp/nodes/`, `lib/nodes.svelte.ts` | U-0, P-2 | F | M |
| U-3 | 2 | F-08: AI member panel and cards, add an AI | A3.1, A3.4 as seen | `cmp/member/` | U-0 | F | M |
| U-4 | 2 | F-09a: approval card in all states; "waiting for you" bar | A4.1, A4.2 as seen | `cmp/approval/` | U-0 | F | M |
| U-5 | 3 | F-09b: permission control, emergency stop dialog, paused banner | A4.7, A4.8 as seen | `cmp/permission/` | U-0, P-2 | F | M |
| U-6 | 3 | F-09c: audit list, in-page notifications | A4.1 (audit) as seen | `cmp/audit/`, `cmp/notify/` | U-0, P-2 | F | S |
| U-7 | 2 | F-10a: brief, decision and summary cards | A5.1 as seen | `cmp/brief/`, `cmp/decision/`, `cmp/summary/` | U-0 | F | M |
| U-8 | 2 | F-10b: task card with the six-step bar, badges, checks, review lines, the waits, "jump the queue" | A5.1, A5.11 as seen | `cmp/task/` | U-0 | F | M |
| U-9 | 2 | F-10c: thread with every message kind, review block, `task_merged` | every golden state reachable | `cmp/thread/` | U-0 | F | M |
| U-10 | 2 | F-11: call block | A6.1 as seen | `cmp/call/` | U-0 | F | S |
| U-11 | 3 | F-12: terminal page | A7.5, A7.6, A7.7, A7.9 (browser half) | `routes/Terminal.svelte`, `cmp/terminal/` | U-0, P-2, W-2 | F | M |
| U-12 | 3 | F-13a: settings page | unit | `routes/Settings.svelte`, `cmp/settings/` | U-0, P-2 | F | M |
| U-13 | 3 | F-13b: service worker, Web Push subscription, the reminder, the iOS hint | A8.3 (browser half) | `web/src/sw.ts`, `lib/push.ts`, `cmp/reminder/` | U-0, P-1 | F | M |
| U-14 | 2 | F-15: group info panel with project memory | A5.14 as seen | `cmp/groupinfo/` | U-0 | F | S |
| U-15 | 2 | Other-tab notice with "Use here"; "cannot reach the group home" and read-only | A1.12, A2.6 as seen | `cmp/status/` | U-0, W-4 | F | S |
| U-16 | 4 | Accessibility pass at the design's widths (F-14, second half) | keyboard, focus, contrast, reduced motion, names | fixes in `cmp/`, after every U unit merged | U-1 to U-15 | F | M |

## 6. Integration units

They own test files and no product logic. A failure becomes a fix unit for
the owner of the failing file.

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| I-1 | 3 | M3 on the local link with fake CLIs | A3.1, A3.2, A3.4 to A3.7, A3.9, A3.10 | `features/m3*` | N-1, HM-1, HM-5 to HM-7, HM-16, HS-1, HS-2, HS-8, HS-9, HS-13 to HS-15 | A | M |
| I-3 | 3 | M2 on the local link with a test client | A2.1 to A2.6 | `features/m2*` | N-1, HM-1 to HM-6 | A | M |
| I-2 | 4 | M1 on real transport and NET-1; A2.7 during a move | A1.1 to A1.7, A1.9 to A1.11, A1.13, A2.7 | `features/m1*` | N-3, N-4, PR-1, PR-2, NET-1 | A | M |
| I-4 | 4 | M4 without the `live` parts | A4.1 to A4.9 | `features/m4*` | I-1, HM-10, HM-14, HS-16a, HS-16b, HS-17, HS-18 | A | M |
| I-5 | 4 | M5 with `forge: none` and with a bare remote and a fake `gh`; the transcripts of P-3 replayed | A5.1 to A5.19 | `features/m5*` | I-1, P-3, HM-8 to HM-13, HS-4 to HS-7b, HS-10, HS-11 | F | L |
| I-6 | 4 | M6 on one node and on two | A3.3, A6.1 to A6.4 | `features/m6*` | I-1, HM-15, HS-12, HS-21, N-4 | A | S |
| I-7 | 4 | M7 with the fake CLIs in a pseudo-terminal | A7.3 to A7.7, A7.9 | `features/m7*` | I-2, HS-20 | A | M |
| I-8 | 4 | The web client against a real node in three browsers | A1.3, A1.11 to A1.13 (browser side) | `web/e2e/connect.spec.ts` | E-0, N-4, W-1 to W-4 | F | M |
| I-9 | 5 | F-14: the browser-side checks, one unit per milestone (E-M1 to E-M8) | every "as seen" above | `web/e2e/m<n>.spec.ts` | I-2 to I-8, U-1 to U-15 | A | 8 × M |
| I-10 | 5 | M8: install, pair, service, update, rendezvous by one command | A8.1 to A8.4 | `features/m8*` | N-4 to N-7, RV-1, HM-17 | A | M |
| I-11 | 6 | B-26: the full pass on Windows, macOS and Linux, one session per system; the measurements; the printed list for the owner's live run | A8.5, A8.6 | `docs/decisions/` report, CI matrix | all | A | 3 × M |

## 7. Spikes left

| Id | W | Questions | Machine | Needs the owner | M |
| --- | --- | --- | --- | --- | --- |
| S-5 | 1 | ConPTY, Unix socket, interrupt without signals; level 2 for Claude Code on Windows from documentation (Q-4) | Devin Windows | the real CLIs for the last confirmation | A |
| S-6 | 1 | One port for HTTPS and TURN over TLS by server name, `turns:` clients, certificates for two names, cutting an open allocation | Devin Linux with a local certificate authority | a public server for the final run | A |
| S-8 | 2 | Port mapping and hole punching in the simulation of NET-1 | Devin Linux | a real router for the final run | A |
| S-9 | — | TCP simultaneous open; not scheduled, see step 3.4 | — | real NATs | — |
