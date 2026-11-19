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
  `codex app-server`. During takeover the same session id is resumed in the
  interactive CLI inside a pseudo-terminal. No ACP, no vendor agent SDK.
- **C-6** One session has one owner process at any time. The node enforces
  this; the CLIs do not lock.
- **C-7** The server stores no data and the system must keep working without
  it (rules R-130 to R-133).
- **C-8** Validation happens at system boundaries only (README rule 3).

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
+------+------+                   |    claude -p / codex app-server  |
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
- Decides which AI must be woken by which message and queues it in that AI's
  inbox.
- Runs the task state machine and the group-level limits (rounds, wake-ups,
  failure counts, review rounds, pause).
- Single writer. There is no merge logic and no conflict resolution: clients
  propose, the home orders.

**Host role**: everything that touches an AI on this machine.

- Knows the AIs registered on this machine and the owner's settings for them
  (permission level per project, caps, concurrency limit).
- Starts, resumes, hibernates and stops CLI sessions; guarantees C-6.
- Connects to each home where one of its AIs is a member, as a client. For a
  group homed on the same node the connection is an in-process call with the
  same message types.
- Takes inbox items from homes and writes them to sessions as turns.
- Serves MCP tools to its sessions and turns tool calls into commands to the
  home, signed with the node's device key on behalf of the AI.
- Enforces owner authority: acts only on envelopes signed by one of its
  owner's devices (R-136), whatever the home says.
- Prepares workspaces: clone, worktree and branch per task, check commands,
  merge commands.
- Bridges the pseudo-terminal during takeover.
- Keeps the local audit record and runs retention.

### 2.2 Rendezvous (same binary, `solodawn rendezvous`)

- WebSocket signalling: forwards opaque signed blobs between online devices
  by device id; pairing rooms; presence of watched devices.
- STUN responder.
- Relay: standard TURN over UDP and over TLS on port 443, short-lived
  credentials, a per-device daily byte quota kept in memory.
- Serves the static web client.
- Holds no database. Restarting it loses nothing but in-memory presence and
  quota counters.

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

- An unmodified vendor CLI process, one per active (AI, group) pair.
- Sees the node only as an MCP server (the tools in
  `contracts/mcp-tools.md`) and, for Claude Code, as its permission prompt
  tool.
- Receives group messages as user turns written by the node. It does not
  poll.

## 3. Who talks to whom

| From | To | Channel | Purpose |
| --- | --- | --- | --- |
| any device | rendezvous or a node's signalling port | WebSocket | exchange signed offers, answers, candidates |
| browser | group home | data channel `ctl` | read the group, send commands |
| host-role node | group home | data channel `ctl` | inbox, commands on behalf of AIs, status |
| owner's browser | owner's node | data channel `ctl` | devices, AIs, permissions, settings, audit |
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
- **Home**: trusted to order and store, not to speak for anyone. A home
  cannot forge an approval or a message, because hosts check the original
  author's envelope before acting. In step 1 every home belongs to the same
  person as every host, so this matters from step 2; the mechanism is in
  place from the start because it is the same envelope.
- **AI**: has no key. Its host signs for it. A message "from an AI" is
  signed by the device that runs it, which belongs to the AI's owner.
- **The CLI's own sandbox is the security boundary for level 2.** The node
  maps the three permission levels to each CLI's native permission and
  sandbox settings and answers the escalations the CLI raises. The node does
  not parse shell commands to decide what is safe.

## 5. Where data lives

| Data | Lives on | Synced to |
| --- | --- | --- |
| group, members, tasks, messages, notes | group home | clients, on demand and live |
| device log of a person | that person's nodes, and homes of their groups | whoever must verify them |
| registered AIs, permission levels, caps, settings | the node of that machine | the owner's browser on request |
| CLI session transcript | the vendor CLI's own files | nobody |
| audit record | the node that acted | the owner's browser on request |
| pending inbox of an AI | group home | the AI's host |
| source code | Git | the remote |

Clients keep caches, not replicas. A browser holds recent rows in memory; a
host holds only cursors. Losing a client loses nothing. Losing a home loses
that group's history (see `risks.md`).

## 6. Session states

One state per (AI, group) session, kept by the host and reported to the home.

```
            deliver                    turn ends
 dormant ------------> working <------------------> waiting
    ^                    |  ^                          |
    | idle timeout       |  | hand-back                | idle timeout
    +--------------------+--+--------------------------+
                         |  |
        owner takes over v  |
                      takeover
```

Overlays, each of which holds deliveries: `paused` (group pause, round limit,
pre-empted), `limited` (usage cap or vendor rate limit), `offline` (host not
connected, or the owner took all AIs offline).

Exactly one process owns the session in `working`, `waiting` and `takeover`;
none in `dormant`. A switch closes the old process and waits for it to exit
before starting the new one.

## 7. Choices made here, and what was not chosen

| Choice | Not chosen | Reason |
| --- | --- | --- |
| Home is the single writer; clients cache | replicas with merge (CRDT, event sourcing in every client) | no conflict code; the product already says the home must stay on |
| Rows carry a group sequence number; sync is "rows after n" | a separate event log plus reducers in Go and TypeScript | one mechanism; the browser needs no group logic |
| Cards are messages with a mutable `state` | a table per card type | four card types share one row shape and one command |
| The home fills AI inboxes; hosts deliver | hosts derive wake-ups from the row stream | limits are group-wide and must be decided in one place |
| One CLI process per active session, for both CLIs | one shared Codex app-server for many threads | the MCP shim then knows its caller; hibernate is "kill"; costs memory, bounded by the concurrency cap |
| stdio MCP shim per session | HTTP MCP server in the node | stdio has the longest idle timeout in Claude Code `[documented]`; caller identity comes from the shim's arguments |
| Signed signalling, ephemeral DTLS certificates | long-lived certificates as identity | one key per device; browsers cannot sign with a certificate key |
| Non-relay probe connection, then move | ICE restart on the live connection | make-before-break; a failed probe costs nothing |
| Native CLI sandbox for level 2 | a command classifier in the node | a classifier is not a boundary and is a lot of code |
| One binary with subcommands | separate node and rendezvous binaries | one release matrix; a node already embeds the signalling handler |

## 8. Repository layout (proposed)

```
cmd/solodawn/          entry point; subcommands node, rendezvous, mcp, pair, open, version
internal/proto/        wire structs for every contract, with golden JSON examples
internal/identity/     keys, envelope sign and verify, device log
internal/store/        SQLite schema and queries
internal/peer/         signalling client, pion connections, path detection, probe-and-move
internal/rendezvous/   signalling handler, STUN, TURN, static files
internal/home/         command handling, task state machine, limits, inbox
internal/host/         sessions, CLI drivers, MCP tools, workspaces, pty, audit
web/                   Svelte 5 client
planning/              this plan
```

`internal/proto/testdata/*.json` holds one golden example per message type.
Go tests produce and check them; the web client's tests parse the same files.
That is the contract test between backend and frontend.

## 9. Flows

`flows/pairing.md`, `flows/connect.md`, `flows/sync.md`,
`flows/agent-turn.md`, `flows/approval.md`, `flows/task.md`, `flows/ask.md`,
`flows/takeover.md`.
