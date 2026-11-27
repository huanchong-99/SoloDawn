# Architecture

Product behaviour is in `product-rules.md`. This file says which component
does what and why. Step-by-step sequences are in `flows/`.

## 1. Fixed constraints (technical)

- **C-1** Node and rendezvous are written in Go, in one repository, sharing
  wire structs. Builds use `CGO_ENABLED=0` and cross-compile to six targets
  (Windows, macOS, Linux on amd64 and arm64).
- **C-2** Peer-to-peer transport is WebRTC data channels (pion on nodes, the
  browser's own stack in the web client). One transport for node-to-node and
  browser-to-node.
- **C-3** Storage is SQLite through `modernc.org/sqlite` (pure Go).
- **C-4** The web client is Svelte 5, built to static files.
- **C-5** AI CLIs are driven in their documented structured modes: Claude
  Code with `-p` and stream-json in both directions, approvals through
  `--permission-prompt-tool` pointing at an MCP tool of the node; Codex with
  `codex app-server`; OpenCode with `opencode serve`; Qwen Code with
  `qwen serve` (`contracts/step-3/drivers.md`). An agent without a driver
  joins as an external AI through MCP (`contracts/step-3/external.md`).
  During takeover (on CLIs whose capability table supports it,
  `contracts/step-3/drivers.md` section 4) the same session id is resumed in
  the interactive CLI inside a pseudo-terminal. No ACP, no vendor agent SDK.
- **C-6** One session has one owner process at any time. The node enforces
  this; the CLIs do not lock.
- **C-7** The server stores no data and the system must keep working without
  it (rules R-130 to R-133).
- **C-8** Validation happens at system boundaries only (README rule 3).
- **C-9** The web client runs on browsers with native Ed25519 in WebCrypto.
  Spike S-7 fixed the lowest versions (`risks.md` Q-3). No signature
  library is bundled (R-164).
- **C-10** The node never inspects shell commands to decide whether they are
  safe (R-41). Permission questions come from the CLI; the node's only
  content check is the outbound secret check of R-137.
- **C-11** Removed on 2026-11-26: the node runs no `git`, no `gh` and no
  forge program (R-137). An AI works in a folder of its owner and uses the
  project's own tools through its CLI.

## 2. Components

```
                    +---------------------------+
                    |  rendezvous (public)      |
                    |  signalling WS, STUN,     |
                    |  relay (TURN), static web |
                    +-----+---------------+-----+
            signalling    |               |   signalling
        (signed blobs)    |               |   (signed blobs)
                          |               |
+-------------+   data channels   +-------+--------------------------+
| web client  +===================+ node A                           |
| (browser)   |    "ctl", "pty"   |  home role: group G store+rules  |
| device key  |                   |  host role: sessions of A's AIs  |
+------+------+                   |    claude, codex, opencode, qwen |
       |                          |    MCP shim per session          |
       |  data channels           +-------+--------------------------+
       |  "ctl", "pty"                    ‖ data channel "ctl"
       |                                  ‖ (host of B is a client of home A)
       |                          +-------+--------------------------+
       +==========================+ node B                           |
                                  |  host role: sessions of B's AIs  |
                                  +----------------------------------+
```

Double lines are end-to-end encrypted WebRTC connections. They are direct
whenever a direct path exists and pass through the relay only as ciphertext.

### 2.1 Node (one binary, `solodawn node`)

A node has two roles. Every node has the host role; a node has the home role
for the groups it is home to.

**Home role**: the platform logic of one group.

- Stores the group: members, device lists, tasks, messages, notes.
- Accepts signed commands from clients, checks who may do what, applies them,
  and assigns each changed row the next sequence number of the group.
- Pushes changed rows to connected clients and answers history queries.
- Decides which AI must be woken by which message and queues an item in
  that AI's inbox.
- Runs the task state machine and the group-level limits (rounds, wake-ups,
  failure counts, review rounds, pause).
- When a review passes, marks the task done and starts the tasks that
  waited for it or for its folder. No AI decides that, and nothing is
  merged (R-24).
- Single writer. There is no merge logic and no conflict resolution: clients
  propose, the home orders.

**Host role**: everything that touches an AI on this machine.

- Knows the AIs registered on this machine and the owner's settings for them
  (concurrency limit; per-group permission level and usage caps are postponed
  after 1.0, `contracts/commands.md` section 2).
- Starts, resumes, hibernates and stops CLI sessions; guarantees C-6.
- Connects to each home where one of its AIs is a member, as a client. For a
  group homed on the same node the connection is an in-process call with the
  same message types, through an interface: `host` does not import `home`
  (section 8).
- Takes inbox items from homes and writes them to sessions as turns.
- Serves MCP tools to its sessions and turns tool calls into commands to the
  home, signed with the node's device key on behalf of the AI.
- Enforces owner authority: acts only on envelopes signed by one of its
  owner's devices (R-136), whatever the home says.
- Refuses to send to the group anything from an AI that looks like a secret
  (R-137).
- Runs every session of an AI in the folder its owner gave it for that
  group, its workspace; lists the folders a CLI was used in; and keeps a
  folder to one group (`contracts/local.md` section 7). It prepares nothing
  in a folder and runs no command of a project.
- Bridges the pseudo-terminal during takeover.
- Keeps the local audit record and runs retention.

### 2.2 Rendezvous (same binary, `solodawn rendezvous`)

- WebSocket signalling: forwards opaque signed blobs between online devices
  by device id; pairing rooms; presence of watched devices.
- STUN responder.
- Relay: standard TURN over UDP and over TLS on port 443, short-lived
  credentials, a per-device daily byte quota kept in memory.
- Serves the static web client.
- Holds no database. Restarting it loses only what it holds in memory:
  presence, pairing rooms, relay allocations and credentials, quota counters.

Every node also exposes the signalling handler on its own port, so a peer
that knows a node's address can signal to it without any server (R-133).

### 2.3 Web client

- Static Svelte 5 application, served by the rendezvous host or by a node on
  `localhost`.
- Holds a device key. Connects to the homes of its groups and to its owner's
  nodes.
- Renders what homes push. It contains no group logic: it sends commands and
  displays rows.

### 2.4 AI session

- An unmodified vendor CLI process, one per open session. Three kinds of
  session exist (`sessions.md`): the group session of an AI, a task session, a
  review session.
- Sees the node only as an MCP server (the tools in
  `contracts/mcp-tools.md`) and, for Claude Code, as its permission prompt
  tool.
- Receives group messages as user turns written by the node. It does not
  poll.
- From step 3 an agent without a driver joins as an external AI. The node
  does not start it: its own runner waits with a command of the node and
  starts it once per item (`contracts/step-3/external.md`).

## 3. Who talks to whom

| From | To | Channel | Purpose |
| --- | --- | --- | --- |
| any device | rendezvous or a node's signalling port | WebSocket | exchange signed offers, answers, candidates |
| browser | group home | data channel `ctl` | read the group, send commands |
| host-role node | group home | data channel `ctl` | inbox, commands on behalf of AIs, status |
| owner's browser | owner's node | data channel `ctl` | devices, AIs, workspaces (`perm.set` and `settings.set` postponed after 1.0, `contracts/commands.md` section 2) |
| owner's browser | owner's node | data channel `pty:*` | terminal bytes during takeover, direct only |
| CLI session | its node | stdio MCP through a shim, local socket | tools, approvals |

Topology is a star around each group home. Hosts never talk to each other; an
AI calling an AI on another machine is two ordinary messages through the home
(`flows/ask.md`).

## 4. Trust model

- **Device**: an Ed25519 key pair. The public key is the device id.
- **Person**: a set of devices, defined by a signed device log
  (`contracts/envelope.md`). The id of a person is the id of the first
  device in the log.
- **Signed envelope**: the one authentication primitive. A payload string, the
  signer's device id, and a signature over the payload bytes. Used for
  signalling, for every command to a home, and for anything a host acts on.
- **Channel**: a WebRTC connection is accepted only if the signalling that
  set it up was signed by the expected device. The DTLS fingerprint inside
  the signed description ties the encrypted channel to that device, so the
  rendezvous server cannot sit in the middle.
- **Home**: trusted to order and store. Whatever it tells a host to do is an
  inbox item (plain unsigned frames on the authenticated home channel since
  `P-1b`, `contracts/datachannel.md` section 6). In step 1 the home is one of
  the owner's own devices, so that channel carries the owner's instruction
  (R-136). The decision of an approval is checked end to end as well: the host
  verifies the approver's own signature. From step 2 a home may belong to
  someone else; the host still takes its items, because members of a group
  trust each other (owner's rules of 2026-11-21 and 2026-11-22;
  `contracts/step-2/authority.md`).
- **AI**: has no key. Its host signs for it. A message "from an AI" is
  signed by the device that runs it, which belongs to the AI's owner.
- **The CLI's own sandbox is the security boundary for level 2.** The node
  maps permission levels to each CLI's native permission and sandbox settings
  and answers the escalations the CLI raises. The node does not parse shell
  commands to decide what is safe. Every 1.0 session defaults to level 3 while
  `perm.set` and `settings.set` are postponed (`contracts/drivers.md` section
  1, `contracts/commands.md` section 2).

## 5. Where data lives

| Data | Lives on | Synced to |
| --- | --- | --- |
| group, members, tasks, messages, notes | group home | clients, on demand and live |
| device log of a person | that person's nodes, and homes of their groups | whoever must verify them |
| registered AIs, concurrency limit (levels, caps, settings postponed after 1.0) | the node of that machine | the owner's browser on request |
| CLI session transcript | the vendor CLI's own files | nobody |
| audit record | the node that acted | nobody in version 1 (R-139) |
| pending inbox of an AI | group home | the AI's host |
| source code | the workspace of each AI, a folder of its owner on that AI's computer | nowhere by the platform: the project shares it its own way (R-111) |

Clients keep caches, not replicas. A browser holds recent rows in memory; a
host holds only cursors. Losing a client loses nothing. Losing a home loses
that group's history (see `risks.md`).

## 6. Sessions

In `sessions.md`: the kinds of session, their folder and levels, and the
states. In short: the secretary has one session per group; a working AI has
one session per task and one group session (default level 3 in 1.0, told by its
instructions not to edit files; `contracts/drivers.md` sections 1 and 4); one
process owns a session at any time (C-6); an AI runs one turn at a time per
group.

## 7. Choices made here, and what was not chosen

| Choice | Not chosen | Reason |
| --- | --- | --- |
| Home is the single writer; clients cache | replicas with merge (CRDT, event sourcing in every client) | no conflict code; the product already says the home must stay on |
| Rows carry a group sequence number; sync is "rows after n" | a separate event log plus reducers in Go and TypeScript | one mechanism; the browser needs no group logic |
| Cards are messages with a mutable `state` | a table per card type | four card types share one row shape and one command |
| The home fills AI inboxes; hosts deliver | hosts derive wake-ups from the row stream | limits are group-wide and must be decided in one place |
| One CLI process per open session, for every driven CLI | one shared server across unrelated sessions | the MCP shim then knows its caller; hibernate closes the process; costs memory, bounded by the concurrency cap (`contracts/step-3/drivers.md`) |
| A working AI has one session per task | one session per AI per group | context is clean; decided by the coordinator (Q-8). Until 2026-11-26 each task also had a worktree of its own; now every session runs in the member's workspace, and a folder takes one task at a time |
| The AI works in a folder of its owner, and the platform runs no Git | a clone and a worktree per task, checks, push, pull request and merge run by the node | the owner's decision of 2026-11-26: a group is people and AIs, not a repository ([record](../docs/decisions/implemented/feature/2026-11-26-groups-workspaces.md)) |
| A task is done when its review passes | a merge step after the review | nothing can be merged by a platform that runs no Git; the secretary arranges the closing step (R-24) |
| stdio MCP shim per session | HTTP MCP server in the node | stdio has the longest idle timeout in Claude Code `[documented]`; caller identity comes from the shim's arguments |
| Signed signalling, ephemeral DTLS certificates | long-lived certificates as identity | one key per device; browsers cannot sign with a certificate key |
| Non-relay probe connection, then move (cut 2026-11-21) | ICE restart on the live connection | make-before-break; a failed probe costs nothing |
| Native CLI sandbox for level 2 | a command classifier in the node | a classifier is not a boundary and is a lot of code (`perm.set` postponed after 1.0, `contracts/commands.md` section 2) |
| One binary with subcommands | separate node and rendezvous binaries | one release matrix; a node already embeds the signalling handler |

## 8. Repository layout and import rules

The local quality gates enforce this section. The backend implementer cannot
see the gate configuration, so this section is the statement of record.

### 8.1 Layout

```
cmd/solodawn/           argument parsing only; subcommands as in contracts/local.md section 1
internal/node/          assembly: starts and wires everything below; pairing, local socket,
                        device-log forwarding, retention schedule, self-update (postponed
                        after 1.0), start at login; embeds the built web client from webdist/
                        (ignored by Git)
internal/host/          sessions, CLI drivers, MCP tools, pty, audit
  workspaces/           the folders each CLI was used in; imports nothing from host
  fakecli/{claude,codex,opencode,qwen}/   the fake CLIs for tests (main packages)
  instructions/         instruction texts per session kind
internal/home/          command handling, task state machine, limits, inbox
internal/rendezvous/    signalling handler, STUN, TURN, static file handler
internal/peer/          signalling client, pion connections, path detection
internal/store/         SQLite schema and queries
internal/identity/      keys, envelope sign and verify, device log
internal/proto/         wire structs for every contract; testdata/ holds the golden JSON
web/                    Svelte 5 client; no Go files
planning/               this plan
```

- `internal/proto/testdata/*.json` holds one golden example per message
  type. Go tests produce and check them; the web client's tests parse the
  same files. That is the contract test between backend and frontend.
- Vite builds the web client straight into `internal/node/webdist/`.
  `internal/node` embeds it and hands the file system to the static file
  handler of `internal/rendezvous`, for a node's local page and for
  `solodawn rendezvous` alike. A committed placeholder keeps the Go build
  working when the web client has not been built.
- Spike code (M0) is never merged into `rebuild`. A spike is a pull request
  whose description holds the answers; the planning lead records them here.

### 8.2 Layers and allowed imports

Top to bottom: `cmd` > `node` > `host` > `home` > `rendezvous` > `peer` >
`store` > `identity` > `proto`. A package imports only packages below it,
and of those only the ones in this table:

| Package | May import, of the internal packages |
| --- | --- |
| `proto` | none |
| `identity` | `proto` |
| `store` | `proto` |
| `peer` | `proto`, `identity`, `store` |
| `rendezvous` | `proto`, `identity` |
| `home` | `proto`, `identity`, `store` |
| `host` | `proto`, `identity`, `store` |
| `node` | every package above |
| `cmd` | `node` |

A subpackage belongs to the layer of its parent: `internal/host/workspaces`
is in the layer of `host` and imports no internal package.

So these are forbidden although the order alone would allow them:
`host → home`, `host → rendezvous`, `host → peer`, `home → rendezvous`,
`home → peer`, `rendezvous → peer`, `rendezvous → store`.

### 8.3 How the packages meet without importing each other

- `home` and `host` each declare the few small interfaces through which they
  see the outside (a client to push to; a connection to a home; the owner's
  connection; a byte stream for the terminal), using only `proto` and
  standard-library types. `peer`'s types have exactly those methods, so
  `node` passes them in as they are: no adapter code.
- For a group homed on the same node, `node` gives `host` the local `home`
  behind the same interface. `host` is a client of `home` in both cases and
  never imports it.
- `rendezvous` is the signalling server, `peer` the signalling client. A
  node runs both; `node` connects the handler's local delivery to `peer`.

## 9. Flows

`flows/pairing.md`, `flows/connect.md`, `flows/sync.md`,
`flows/agent-turn.md`, `flows/approval.md`, `flows/task.md`, `flows/ask.md`,
`flows/takeover.md`.
