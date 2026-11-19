# Step 1: backend task list

For the backend implementer. Read first, in this order: `README.md`,
`product-rules.md`, `architecture.md`, every file in `contracts/`, then the
flow each task names. Milestones and acceptance tests are in
`steps/step-1.md`.

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
- Every pull request states what was **not** verified.
- If something blocks you, say so at once in the pull request or its
  description. Do not work around a blocker silently.

## Tasks

`→` means "depends on". "Done when" lists the acceptance tests of
`steps/step-1.md` that must pass, or the artefact that must exist.

### Foundation

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-00 | Scaffold: Go module, the layout of `architecture.md` section 8 with `cmd/solodawn` parsing arguments only and `internal/node` as the assembly package, a test that fails on an import outside the table of section 8.2, `go vet` and tests in CI, cross-compile job for six targets with cgo off, Apache-2.0 licence file | — | CI green on an empty `solodawn version` |
| B-01 | `internal/proto`: structs for the envelope, signalling payloads, every `ctl` message, every row, every command; one golden JSON file per example in the contracts under `testdata/` | B-00 | round-trip tests pass; a test fails if a struct and its golden file disagree |
| B-02 | `internal/identity`: key creation and loading, envelope sign and verify, device-log apply and replay, fingerprint, pairing `mac` and room id; `testdata/crypto.json` with fixed vectors for the frontend | B-01 | unit tests cover every rule of `contracts/envelope.md` sections 3, 4, 6 |
| B-03 | Spikes S-1, S-6, S-8, S-9 | B-00 | answers written in the pull request, which is not merged |
| B-04 | Spikes S-2, S-3, S-4 on macOS; S-5 on Windows | B-00 | answers written in the pull request, which is not merged; `contracts/local.md` markers can be updated from them |
| B-05 | `internal/store`: open, migrations, the tables of `contracts/storage.md`, row put / after-seq / thread page, sequence counter, inbox, retention | B-01 | unit tests with an in-memory database |

B-03 and B-04 run in parallel with B-01, B-02, B-05. Their answers come
before the code that depends on them: B-03 before B-07, B-04 before B-11.

### M1 connectivity

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-06 | `internal/rendezvous`: handshake, `to`/`from`, `watch`, rooms, `quota`; STUN and TURN with the daily quota; static files; TLS by flags; `solodawn rendezvous` | B-02, B-03 | A1.5, A1.7 against a local instance |
| B-07 | `internal/peer`: signalling client with fan-out over all paths; pion setup per `contracts/rendezvous.md` section 5; acceptance hook; `ctl` framing with requests, responses and pushes; path classification; reconnect; probe-and-move; local-network beacon; remembered addresses | B-06 | A1.3, A1.4 |
| B-08 | `internal/node`, the assembly package: start-up and wiring of store, identity, peer, the local signalling handler and (later) home and host; local socket; pairing flow; device-log storage and forwarding; `hello`/`welcome` and routing of `ctl` messages to the home and host surfaces. `cmd/solodawn`: the subcommands `node`, `pair`, `pair-browser`, `open`, `status`, as argument parsing over `internal/node`. Also in `internal/node`: the node-level part of the host surface, that is `device`, `settings.set`, `pair.browser`, and `host.info` assembled from what the other packages report | B-05, B-07 | A1.1, A1.2, A1.6, A1.8 |

### M2 home

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-09 | `internal/home`, part 1: in-memory state per group with write-through; `group`, `member`, `post` (kind `text`), `note`, `device`; the permission checks of `contracts/commands.md`; `sync`, `rows`, `fetch`; `gone` markers and `forget`; the retention purge with the `keep` exemption, scheduled by `internal/node` | B-08 | A2.1 to A2.6 |

### M3 sessions

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-10 | Fake CLIs for tests, in `internal/host/fakecli/claude` and `internal/host/fakecli/codex`: `claude` and `codex` stand-ins that speak the structured protocols from a script file (emit events, call MCP tools, ask permission, exit mid-turn, sleep) | B-04 | used by the B-11 tests |
| B-11 | `internal/host`: driver interface with Claude Code and Codex drivers per `contracts/local.md` section 4; CLI detection, `agent.set`; `host.join`, `host.leave`, clone; the session table, the group session kind and the state machine of `sessions.md`, one turn at a time per AI per group; MCP server and shim with `post`, `read_thread`, `list_members`, `list_tasks`, `note_put`, `note_list`; the outbound secret check of `contracts/local.md` section 8; inbox handling, routing of items to sessions, turn composition, `ack`; `member status`; dormancy and `max_open`; crash and resume-failure handling; instructions text | B-09, B-10 | A3.1 to A3.9 |

### M4 permissions

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-12 | Level mapping for both CLIs by session kind; `perm.set`; the `approve` tool and Codex approval requests behind one handler; `post` kind `approval`, `act approve`/`deny` on the home; wait, one-shot grant, `decided` handling with envelope check on the host; audit table and `audit` request; usage recording and caps; `group pause`/`resume`; `host.offline`/`online`; turn time limit | B-11 | A4.1 to A4.9 |

### M5 tasks

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-13 | Home: `brief` and `act confirm`/`revise`; task rows and `task` messages; every `task` operation, `start` and `conflict` included; review outcome, with the `merge` item queued by the home itself; `task_merged`; failure and review-round counters; round limit and wake-up cap; `jump` and pre-emption; `decision` and `act choose`; `summary` | B-12 | unit tests for every transition of `contracts/commands.md` section 1 |
| B-14 | Host: task sessions (worktree and branch created with the session, notes in the first turn, session and folder removed on merge, cancel or reassignment); review sessions with the diff file; `push_branch`; `run_checks` with the project's commands and the two hard checks; `submit_task` with the head-commit rule, the push and the pull request; hosted-check polling; the merge on a `merge` item, with and without a remote, and `task conflict`; tools `start_task`, `push_branch`, `run_checks`, `submit_task`, `submit_review`, `request_decision`, and the three secretary tools; instructions per session kind | B-13 | A5.1 to A5.14 |

### M6, M7

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-15 | `ask` and `reply`: call depth per turn, open-call table on the host, wait and receipt, `reply` item handling | B-14 | A6.1 to A6.4 |
| B-16 | Takeover: choice of session (`flows/takeover.md` A), a task created for an AI without one and cancelled when nothing changed; pseudo-terminal start for both CLIs, state switch under C-6, `pty` channel bridge with back-pressure, `pty.resize`, hand-back facts from Git, `handback` turn and `handback_summary`, refusal on a relayed path | B-14 | A7.1 to A7.7 |

The pseudo-terminal bridge of B-16 needs only B-11 and can be built and
tested on a group session alongside B-12 to B-14; the session choice and
A7.7 need task sessions (B-14).

### M8 shipping

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| B-17 | In `internal/node`: `service install`/`remove` on three systems, self-update with checksum, embedding the web client that Vite builds into `internal/node/webdist/`. Release workflow producing six binaries, `SHA256SUMS` and `install.md`. `net_status` (`host`, fed by `node`). Web Push sender in `home` with the short summary of `contracts/commands.md` 1.7 | B-14 | A8.1, A8.2, A8.3 (backend half), A8.4 |
| B-18 | Full pass on Windows and Linux; measurements of A8.6 | all | A8.5, A8.6 |

## Notes on the hard parts

- **Probe-and-move (B-07).** The move is "a second `hello` from the same
  device replaces the first connection". Keep it that simple: no session
  migration object, no sequence numbers on `ctl`.
- **Drivers (B-11).** Each driver is a struct that owns one process and
  exposes: send a turn, a channel of events (turn ended with usage, approval
  request, exited), interrupt, close. Parse only the events the node uses;
  ignore every other event type without logging it as an error.
- **Sessions (B-11, B-14).** The home knows nothing about sessions. The
  host maps an item to a session by the table in `flows/agent-turn.md` A
  and never runs two turns of one AI in one group at the same time.
- **Home rules (B-13).** Write each command handler as a function from
  (state, command) to (changed rows, inbox items, error). Pure, and tested
  without a database or a network.
- **Layering (all tasks).** `home` and `host` never import `peer`,
  `rendezvous` or each other. Each declares the small interfaces it needs;
  `peer`'s types satisfy them as they are, and `internal/node` passes them
  in. The same-node home reaches `host` through that same interface.
- **Checks (B-14).** The node runs exactly the strings in `group.checks`. It
  does not discover checks from the repository.
- **What not to build.** No plugin system, no generic "agent adapter"
  layer beyond the two drivers, no message queue, no cache layer, no
  metrics, no telemetry, no admin interface.

## Where to start

B-00 first. Then B-01, B-02, B-05 and the spikes B-03, B-04 at the same
time. B-01's golden files unblock the frontend, so B-01 is merged first
among them.
