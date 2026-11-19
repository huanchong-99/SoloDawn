# Contract: data-channel protocol

What travels on a WebRTC connection between a client and a node. This file
covers channels, framing, the session, device logs, reading, the command
transport, the inbox for hosts, and the terminal channel. Row shapes are in
`contracts/rows.md`. The commands themselves are in `contracts/commands.md`.

Two surfaces share the same channel and the same framing:

- **home surface**: any client talking to a group home;
- **host surface**: the owner's browser talking to one of the owner's nodes.

A node answers both. Which messages it accepts from a peer depends on who the
peer is (section 8).

## 1. Channels

| Label | Created by | Type | Carries |
| --- | --- | --- | --- |
| `ctl` | the offering side, in the offer | ordered, reliable, text | everything in this file except terminal bytes |
| `pty:<g>:<member>` | the owner's browser, after `takeover.start` succeeded | ordered, reliable, binary | raw terminal bytes, both directions |

## 2. Framing on `ctl`

- One JSON object per message. Field `t` is the type.
- A message is at most 60 KiB. The payload `p` of an envelope is at most
  24 KiB, so a row with its envelope always fits one message. Larger results
  are split by the sender (`rows` batches, `fetch` pages).
- **Request**: has `id` (integer, unique per connection, chosen by the
  sender). **Response**: has `re` equal to that `id`, and either `ok: true`
  plus result fields or `err: {code, msg}`. `msg` is an English sentence for
  logs; clients show their own text for `code`.
- **Push**: has no `id` and no `re`.

`[unverified]` 60 KiB assumes both stacks accept SCTP messages of that size.
Spike S-1 measures it; the two numbers are constants.

## 3. Session

Client to node, first message:

```json
{"t":"hello","id":1,"v":1,"kind":"browser","name":"phone browser","addrs":[]}
```

| Field | Meaning |
| --- | --- |
| `v` | protocol version; a node answers `err.code = "version"` if it cannot speak it |
| `kind` | `browser` or `host` |
| `addrs` | hosts only: signalling URLs at which this node may be reachable |

Node to client:

```json
{"t":"welcome","re":1,"ok":true,"v":1,"device":"<node device id>",
 "name":"workstation","version":"0.3.1","time":1763600000000,
 "groups":[{"g":"Qm1…","name":"Website","seq":812}],"owner":true}
```

| Field | Meaning |
| --- | --- |
| `groups` | groups homed on this node that the peer's person may read |
| `owner` | true if the peer is a device of this node's owner; the host surface is then available |

- When the list of groups for a connected peer changes, the node pushes
  `{"t":"groups","groups":[…]}` with the whole list.
- A node keeps one inbound connection per client device. A `hello` on a new
  inbound connection from a device that already has one closes the older
  one. Nothing is carried over: the client reads again with `sync`
  (`flows/connect.md` B). A connection the node itself opened as a client
  of that device's home is separate and unaffected.
- A `hello` on a connection that has already had one is answered with a
  fresh `welcome`. A client sends it again when pairing has put it in the
  owner's log, and reads `owner: true` from the answer.
- Each side decides what its path is from the candidate pair its own stack
  selected. Nothing a peer says about the path is used for a rule.

### 3.1 Device logs

```json
{"t":"devlog","user":"<user id>","entries":[{"p":"…","k":"…","s":"…"}]}
```

A push, in either direction, with entries of one person's device log in
order (`contracts/envelope.md` section 4). The receiver applies each entry
by the log's own rules; who sent the frame does not matter, because every
entry carries its own signature. It is the only way log entries travel:
during pairing, when a device forwards a new entry, and after `welcome`,
when each side sends the logs the other may be missing.

## 4. Reading a group (home surface)

### `sync` (request)

```json
{"t":"sync","id":2,"g":"Qm1…","since":0,"state_only":false}
```

The home takes the group's current sequence number S together with a read
view of the database as it is at S, subscribes this connection to every
change after S, and then sends, in this order:

1. `{"t":"reset","g":"Qm1…"}`, only if the client must drop its cache
   (`flows/sync.md` C);
2. `rows` pushes with what the client is missing, read from that view, so
   that a write made while they are being sent neither hides nor changes
   any of them;
3. the response `{"t":"synced","re":2,"ok":true,"g":"Qm1…","seq":S}`;
4. from then on, every change after S, as `rows` pushes.

With `state_only: true`, steps 2 and 4 carry only `group`, `member`,
`device` and `task` rows.

### `rows` (push)

```json
{"t":"rows","g":"Qm1…","rows":[ {"r":"task", "seq":813, …}, {"r":"msg", "seq":814, …} ]}
```

Rows are in increasing `seq`. A row replaces any cached row with the same
`r` and `id`, unless the cached one has a higher `seq`. A row with
`gone: true` replaces it too and stays in the cache as a marker that is
not shown, so that an older copy arriving later (a `fetch` page asked for
earlier) cannot bring the row back.

The client's cursor for a group, the `since` of its next `sync`, is 0 until
the first `synced`; it becomes S at `synced`, and then the `seq` of each
live row it applies. Rows received before `synced` and rows from `fetch`
never move it.

### `fetch` (request)

```json
{"t":"fetch","id":3,"g":"Qm1…","thread":"<task id>","before":700,"limit":50}
```

- `thread` is a task id, or `null` for the main line. In its place,
  `"kind":"review"` asks for the review messages of all threads (the
  ledger) and `"kind":"note"` for the notes (R-102).
- `before` is a `pos` (`contracts/rows.md`): the answer holds rows with a
  lower `pos`, newest first. `limit` is at most 100.

```json
{"t":"fetched","re":3,"ok":true,"rows":[ … ],"more":true}
```

The home stops filling the answer when the next row would exceed the message
limit; `more` says whether older rows exist.

## 5. Commands

```json
{"t":"cmd","id":4,"env":{"p":"…","k":"…","s":"…"}}
```

```json
{"re":4,"ok":true}
{"re":4,"ok":true,"num":14}
{"re":4,"err":{"code":"forbidden","msg":"only the AI's owner may decide this approval"}}
```

- The payload inside `env` names the command in its own `t`
  (`contracts/commands.md`).
- The signer `env.k` must be the device that this connection was
  authenticated for.
- A command that the receiver has already accepted gets the same outcome
  again, under the `re` of the new request, and has no second effect
  (`contracts/commands.md`, "Once only").
- A home command has `g`. A host command has `to` equal to the node's device
  id.

## 6. Inbox (home to host)

The home decides which AI must see what and queues an item per AI. Every
item is an instruction to a host, so every item is signed by the home.

```json
{"t":"inbox","g":"Qm1…","items":[
  {"env":{"p":"{\"t\":\"item\",\"n\":\"…\",\"ts\":1763600000000,\"g\":\"Qm1…\",\"to\":\"<host device>\",\"item\":4411,\"ai\":\"<member id>\",\"why\":\"review\",\"task\":\"t12…\",\"msg\":\"Zk3…\",\"commit\":\"9f2c1ab\"}",
          "k":"<home device>","s":"…"},
   "msg":{"r":"msg", …},"task":{"r":"task", …},"cause":{"p":"…","k":"…","s":"…"}}
]}
```

| Part | Meaning |
| --- | --- |
| `env` | the item itself: which AI, why, which task and message; for `review` and `merge` which commit, and for `merge` which pull request (`pr`); `jump: true` when the owner jumped the queue. Built and signed by the home in the transaction that queued the item, and stored: what is sent later is what was decided then, whatever the task has become |
| `msg`, `task` | copies, taken when the item is sent, of the rows the turn is built from. `msg.env`, when present, is the author's own envelope |
| `cause` | the signed decision that led to the item, unchanged: the `act` of the person who decided an approval or a decision, or who confirmed or sent back the brief. Absent where there is none: direct work starts from the owner's own message, which is `msg` |

- **What a host accepts (step 1).** The item's envelope is signed by the
  home of that group, that device is in the log of the host's owner, `to`
  is this host and `ai` runs here. For `decided` on an approval the host
  also checks `cause` itself: signed by its owner or a named approver, about
  the request it holds (`flows/approval.md`). Rows without a signature are
  content, never authority. From step 2 a home may belong to someone else,
  and an item will also need the owner's own signature for that task.
- **Still current.** An item may wait a long time before it is used. For an
  item about a task the host looks at the task row it holds now: the task
  is not final, the AI still has the role the item is for, and for
  `review` and `merge` the task is at that step with the item's commit.
  If not, the host refuses the item (audit `refused`, then `ack`). The
  home, for its part, deletes items that a later change made pointless.
- The host answers `{"t":"ack","g":"…","item":4411}` when it delivers the
  item to the session or refuses it. The home deletes an acknowledged item
  only if the acknowledging connection is the item's host. On every `hello`
  from a host the home sends its unacknowledged items again.
- An item with `why: "merge"` is for the host itself, not for a session.
- What the host does with an item is in `flows/agent-turn.md`.

## 7. Host surface

Available when `welcome.owner` is true. The name describes the wire. In the
code, node-level requests (`pair.browser`, node settings, putting
`host.info` together) are handled by `internal/node`, and everything about
AIs and sessions by `internal/host` (`architecture.md` section 8).

### `host.info` (request, then pushes)

The response to `{"t":"host.info","id":5}` and every later push of that type
carry the whole object:

```json
{"t":"host.info","re":5,"ok":true,
 "device":"…","name":"workstation","os":"macOS 15.2","version":"0.3.1",
 "open":2,"max_open":4,"offline":false,"update":null,
 "homes":[{"g":"Qm1…","name":"Website"}],
 "relay":{"used":3250000,"limit":52428800},
 "agents":[
   {"agent":"claude","cli_version":"2.1.311","account":"personal",
    "login_ok":true,"caps":{"day":{"unit":"turns","max":200}},
    "groups":[{"g":"Qm1…","member":"<member id>","level":2,
               "repo_config":false,"state":"working","task":"<task id>",
               "queued":0}]}],
 "settings":{"retention_days":90,"idle_minutes":15,"default_level":2,
             "all_level3":false,"approvers":[]}}
```

`agents` lists every supported CLI found on the machine; `account` is `null`
until the owner has declared it. In `groups`, `state` and `task` are the
derived state of the AI in that group (`contracts/rows.md` section 2),
`level` is the level of its task sessions, and `queued` counts held items.

### `audit` (request)

`{"t":"audit","id":6,"before":1763600000000,"limit":50}` is answered with

```json
{"re":6,"ok":true,"rows":[
  {"ts":1763599000000,"g":"Qm1…","ai":"claude","what":"approval",
   "detail":"git push origin other-branch","by":"<device id>","result":"approved"}]}
```

### Terminal

- `{"t":"pty.resize","g":"Qm1…","ai":"<member id>","cols":120,"rows":32}`
  is a push from the browser.
- After the host accepted `takeover.start`, the browser opens a data channel
  labelled `pty:<g>:<member>`. Each message is a chunk of raw bytes. The
  host stops reading from the pseudo-terminal while the channel's send
  buffer is above 1 MiB. The channel closing, for any reason, is hand-back.
- The host refuses `takeover.start` with `relay` when the pair its own
  stack selected for this connection is relayed.

## 8. What a node accepts from whom

| Peer is | Accepted |
| --- | --- |
| a device of the node's owner | everything: host surface, and home surface for groups homed here |
| a device that passed the pairing exchange and is not yet in the owner's log | `hello` and `devlog` only, until a log entry makes it an owner device (`flows/pairing.md`) |
| a device of another member of a group homed here (step 2) | home surface for that group only |
| the host of an AI member of a group homed here | home surface for that group, `ack` for its own items |
| anything else | the connection is not accepted (signalling was dropped) |

## 9. Error codes

| Code | Meaning |
| --- | --- |
| `version` | protocol version not supported |
| `bad_request` | payload does not match the contract |
| `bad_sig`, `unknown_device`, `not_for_me`, `stale` | envelope checks (`contracts/envelope.md`) |
| `forbidden` | this principal may not do this |
| `not_found` | the group, task or message does not exist |
| `conflict` | the target is not in a state that allows this |
| `paused` | the group or task is paused |
| `limit` | a runaway limit refused it; `msg` names the limit |
| `relay` | not available on a relayed path |
| `busy` | the session cannot do this now |
| `cli` | the CLI program failed; `msg` carries its error text |
| `secret` | host only: text from an AI matched the outbound secret check and was not sent (R-137) |
| `home_unreachable` | host only: the home of that group is not connected |

Every boundary failure maps to exactly one of these. Internal failures are
not mapped: the node logs them and the request fails with `internal`.
