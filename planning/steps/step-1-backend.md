# Step 1: backend task list

For the backend implementer. Read first, in this order: `README.md`,
`product-rules.md`, `architecture.md`, `sessions.md`, every file in
`contracts/`, then the flow each task names. Milestones and acceptance tests
are in `steps/step-1.md`.

## Working rules

- One pull request per task, from a branch `devin/<task id>-<slug>` into
  `rebuild`. Never push to `rebuild` or `main` directly; never force-push.
- A pull request contains the code, its tests, and nothing under
  `planning/`. If a contract is wrong or incomplete, say so in the pull
  request description; the planning lead changes the contract.
- **No defensive programming beyond system boundaries** (README rule 3).
  Boundaries are: bytes from a data channel or WebSocket, the local socket,
  output of a CLI or of Git, command-line arguments. Inside, no nil checks
  for values that cannot be nil, no retries, no fallbacks, no wrapping of
  errors layer by layer. An impossible state is a panic with a clear
  message.
- Least code. No interface with one implementation, except the CLI driver
  (two), the store used by tests, and the few small interfaces through
  which `home` and `host` see connections (`architecture.md` 8.3). No
  configuration option that no rule asks for.
- **Imports follow `architecture.md` section 8.** A local gate enforces the
  table and the forbidden list there. A pull request that seems to need an
  import outside the table says so in its description instead of adding
  it; the table changes only through the planning lead and the
  coordinator.
- Dependencies allowed without asking: `pion/webrtc`, `pion/turn`,
  `modernc.org/sqlite`, `modelcontextprotocol/go-sdk`, a pseudo-terminal
  library with ConPTY support, a WebSocket library, `golang.org/x/crypto`,
  a UPnP/NAT-PMP library, a Web Push library. Anything else: ask in the pull
  request.
- Every task ships **one wire example and one observable test**: a golden
  JSON file under `internal/proto/testdata/` for what it puts on the wire,
  and a test that fails without the task.
- Every pull request states what was **not** verified.
- If something blocks you, say so at once in the pull request or its
  description. Do not work around a blocker silently.

## Tasks

`→` means "depends on". "Done when" names the acceptance tests of
`steps/step-1.md`, or the artefact that must exist.

### Foundation

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-00 | Scaffold: Go module; the layout of `architecture.md` section 8 with `cmd/solodawn` parsing arguments only and `internal/node` as the assembly package; a test that fails on an import outside the table of 8.2; `go vet` and tests in CI; cross-compile job for six targets with cgo off; Apache-2.0 licence file | — | CI green on an empty `solodawn version` |
| B-01 | `internal/proto`, **frozen in three slices**, each with its golden files: (a) envelope, signalling payloads, `pair`, device-log entries, `hello`, `welcome`, `devlog`; (b) `sync`, `reset`, `rows`, `fetch`, `cmd` and its answers, every row type; (c) one confirmed task from brief to merge: the signed inbox items with their causes, the task commands with one commit, review, merge | B-00 | round-trip tests pass; a test fails if a struct and its golden file disagree. Slice (a) before B-06, (b) before B-09, (c) before B-12 |
| B-02 | `internal/identity`: key creation and loading, envelope sign and verify, device-log apply and replay, fingerprint, the canonical bytes of the pairing secret, `mac` and room id; `testdata/crypto.json` with fixed vectors for the frontend | B-01a | unit tests cover every rule of `contracts/envelope.md` sections 3, 4, 6 |
| B-03 | Spikes S-1, S-6, S-8, S-9 | B-00 | answers written in the pull request, which is not merged |
| B-04 | Spikes S-2, S-3, S-4 on macOS; S-5 on Windows | B-00 | answers written in the pull request, which is not merged; the messages the real CLIs sent are kept as files for B-10 |
| B-05 | `internal/store`: open, migrations, the tables of `contracts/storage.md`, row put / after-seq-up-to / page by `pos`, counters, inbox, receipts, retention with the purge mark | B-01b | unit tests with an in-memory database |

B-03 and B-04 run in parallel with B-01, B-02, B-05. Their answers come
before the code that depends on them: B-03 before B-07, B-04 before B-10
and B-11, and S-2, S-3, S-5 before B-13.

### M1 connectivity

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-06 | `internal/rendezvous`, signalling: handshake, `to`/`from`, `watch`, rooms; static file handler; `solodawn rendezvous` | B-02, B-03 | A1.5 against a local instance |
| B-07 | `internal/rendezvous`, relay: STUN and TURN, credentials, the daily byte quota counted on relayed data, `quota`; TLS by flags | B-06 | A1.7 |
| B-08 | `internal/peer`, connections: signalling client with fan-out of every payload over every path; pion setup per `contracts/rendezvous.md` section 5; acceptance hook; `ctl` framing with requests, responses and pushes; one inbound connection per client device; path classification from the selected pair; reconnect | B-06 | A1.3 up to "the path is `relay`" |
| B-09 | `internal/peer`, paths: the relay-free probe and the move as a reconnect (or renomination, if S-1 says so); local-network beacon; remembered addresses; port mapping | B-08 | A1.3, A1.4 |
| B-10 | `internal/node`, the assembly package: start-up and wiring; local socket; pairing with the bootstrap rule; `devlog` in both directions and forwarding; `hello`/`welcome`, `groups`; routing of `ctl` messages to the home and host surfaces; receipts for host commands; the node-level part of the host surface (`settings.set`, `pair.browser`, `host.info`). `cmd/solodawn`: `node`, `pair`, `pair-browser`, `open`, `status` as argument parsing | B-05, B-08 | A1.1, A1.2, A1.6, A1.8 |

### M2 home

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-11 | `internal/home`, reads: in-memory state per group with write-through; `sync` with the cut, `reset`, delta or cold content, pending cards; live pushes, `state_only`; `fetch` by `pos` and by `kind`, bounded by bytes; retention | B-10 | A2.2, A2.5, A2.6 |
| B-12 | `internal/home`, first mutations: `group`, `member` (with unique names and the secretary rules), `post` kind `text`, `note`, `forget` with `gone` markers; the principal checks of `contracts/commands.md`; receipts | B-11 | A2.1, A2.3, A2.4 |

### M3 sessions

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-13 | Fake CLIs in `internal/host/fakecli/claude` and `.../codex`, scripted from a file: turn events with ids that survive a resume; MCP calls while a turn runs; permission requests with exact allow and deny timing; a decision that arrives after the process exited; a reply to an open call and a late one; exit before the end of a turn; usage, rate-limit and sign-in events; start and exit written to a log with the session id | B-04 | used by every later integration test; shapes equal to the captured real ones |
| B-14 | `internal/host`, drivers: driver interface; Claude Code and Codex drivers per `contracts/drivers.md` section 1; CLI detection, `agent.set`; session table and states (`sessions.md`); dormancy and `max_open`; crash and resume-failure handling; host events in `member status` | B-12, B-13 | A3.4, A3.5, A3.6 |
| B-15 | `internal/host`, talking: MCP server and shim with `post`, `read_thread`, `list_members`, `list_tasks`, `note_put`, `note_list`; the outbound secret check; signed inbox items on the home and their check on the host; audit-first delivery, `ack`; routing of items to sessions, one turn at a time per AI per group; turn composition; instructions; `host.join`, `host.leave`, one clone per group | B-14 | A3.1 to A3.3, A3.7 to A3.10 |

### First slice of M5, then M4

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-16 | A task session, minimal: `task start` and `task branch` on the home with the task row and `task` message; `start_task`; worktree and branch; the session created on `assign`, ended on `task cancel` | B-15 | A5.7 |
| B-17 | Permissions: level by session kind for both CLIs; what the node itself offers at each level; `perm.set`; the `approve` tool and Codex approval requests behind one handler; approval records; `post` kind `approval` and `act approve`/`deny`; the wait and the late decision; the check of `cause` on the host; audit and the `audit` request | B-16, answers of S-2, S-3, S-5 | A4.1 to A4.6, A4.10 |
| B-18 | Limits and stops: usage recording and caps; turn time limit; wake-up cap; `group pause`/`resume`; `host.offline`/`online` | B-17 | A4.7 to A4.9 |

### M5 tasks

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-19 | Brief and assignment: `brief`, `act confirm`/`revise` with the `cause` on `assign` items; dependencies; `propose_brief`, `set_task`, `request_decision`; `decision` and `act choose`; `task assign`, `pause`, `resume`, `cancel`; secretary instructions | B-18 | A5.2, A5.6, A5.15; unit tests for task-machine rows 1, 11 to 14, 18 to 21 |
| B-20 | Checks and review: `run_checks` on a clean tree with the project's commands, in the minimal environment, and both hard checks; `submit_task` without a remote; review sessions with the diff file; `submit_review`, the outcome and the six-review stop; failures; `reviewed`; the round limit; `jump`, `task preempted` and `continued`; the quiet-task timer | B-19 | A5.3 (first two sentences), A5.4, A5.5, A5.8, A5.9, A5.11, A5.16, A5.17; `forge: none` half of A5.1 |
| B-21 | Remote and merge: `push_branch`; pull request on submit; hosted checks by commit; the merge of the reviewed commit, with and without a remote; `task conflict`; reassignment; `task_merged`; `post` kind `summary` | B-20 | A5.1, A5.3, A5.10, A5.12 to A5.14 |

### M6, M7

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-22 | `ask` and `reply`: call depth per turn, open-call table on the host, the answer to an open call ahead of the queue, wait and receipt | B-20 | A6.1 to A6.4 |
| B-23 | Takeover: choice of session, a task created for a working AI without one and cancelled when nothing changed; interactive start with the session's level; state switch under C-6; `pty` channel bridge with back-pressure, `pty.resize`; hand-back facts and turn; refusal on a relayed path | B-17 | A7.1 to A7.8 |

### M8 shipping

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-24 | In `internal/node`: `service install`/`remove` on three systems; self-update per `contracts/local.md` section 9; embedding the web client from `internal/node/webdist/`. Release workflow producing six binaries, `SHA256SUMS` and `install.md`. `net_status` | B-21 | A8.1, A8.2, A8.4 |
| B-25 | Web Push in `home`: `push`, eligibility per browser, the short summary | B-21 | A8.3 (backend half) |
| B-26 | Full pass on Windows and Linux; measurements of A8.6 | all | A8.5, A8.6 |

## Notes on the hard parts

- **A move is a reconnect (B-09).** No subscription is moved and `ctl` has
  no sequence numbers of its own. The client reads again from the last row
  it applied.
- **Drivers (B-14).** Each driver is a struct that owns one process and
  exposes: send a turn, a channel of events (turn ended with usage, approval
  request, exited), interrupt, close. Parse only the events the node uses;
  ignore every other event type without logging it as an error.
- **Sessions (B-14 to B-16).** The home knows nothing about sessions. The
  host maps an item to a session by the table in `flows/agent-turn.md` A.
- **Home rules (B-12, B-19 to B-21).** One function per command, from
  (state, command) to (changed rows, inbox items, answer, error); pure, and
  tested without a database or a network. The table in
  `contracts/task-machine.md` is the specification; no state-machine
  framework.
- **Layering (all tasks).** `home` and `host` never import `peer`,
  `rendezvous` or each other. Each declares the small interfaces it needs;
  `peer`'s types satisfy them as they are, and `internal/node` passes them
  in.
- **Checks (B-20).** The node runs exactly the strings in `group.checks`. It
  does not discover checks from the repository.
- **What not to build.** No plugin system, no generic "agent adapter"
  layer beyond the two drivers, no message queue, no cache layer, no
  metrics, no telemetry, no admin interface, no event bus.

## Where to start

B-00 first. Then B-01 slice (a), B-02, and both spike tracks at the same
time. The wire examples are not frozen all at once: each slice of B-01 is
agreed just before the code that needs it.
