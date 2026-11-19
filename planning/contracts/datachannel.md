# Contract: data-channel protocol

What travels on a WebRTC connection between a client and a node. This file
covers channels, framing, the session, reading, the command transport, the
inbox for hosts, and the terminal channel. Row shapes are in
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
- A message is at most 60 KiB. Larger results are split into several
  messages by the sender (`rows` batches, `fetch` pages).
- **Request**: has `id` (integer, unique per connection, chosen by the
  sender). **Response**: has `re` equal to that `id`, and either `ok: true`
  plus result fields or `err: {code, msg}`. `msg` is an English sentence for
  logs; clients show their own text for `code`.
- **Push**: has no `id` and no `re`.

`[unverified]` 60 KiB assumes both stacks accept SCTP messages of that size
(pion's default limit and current browsers). Spike S-1 measures it; the
number is one constant.

## 3. Session

Client to node, first message:

```json
{"t":"hello","id":1,"v":1,"kind":"browser","name":"phone browser",
 "addrs":[],"path":"direct"}
```

| Field | Meaning |
| --- | --- |
| `v` | protocol version; a node answers `err.code = "version"` if it cannot speak it |
| `kind` | `browser` or `host` |
| `addrs` | hosts only: signalling URLs at which this node may be reachable (local address, mapped ports) |
| `path` | the sender's classification of the path: `lan`, `direct`, `relay` |

Node to client:

```json
{"t":"welcome","re":1,"ok":true,"v":1,"device":"<node device id>",
 "name":"workstation","version":"0.3.1","time":1763600000000,
 "groups":[{"g":"Qm1…","name":"Website","seq":812}],
 "owner":true}
```

| Field | Meaning |
| --- | --- |
| `groups` | groups homed on this node that the peer's person may read |
| `owner` | true if the peer is a device of this node's owner; the host surface is then available |
| `time` | the node's clock, for display of relative times |

A `hello` from a device that already has a connection to this node replaces
the older connection (`flows/connect.md` step 11).

## 4. Reading a group (home surface)

### `sync` (request)

```json
{"t":"sync","id":2,"g":"Qm1…","since":0,"state_only":false}
```

The node sends zero or more `rows` pushes, then the response:

```json
{"t":"synced","re":2,"ok":true,"g":"Qm1…","seq":812,"reset":false}
```

After `synced`, the node pushes every later change of that group to this
connection until it closes. Semantics in `flows/sync.md`.

### `rows` (push)

```json
{"t":"rows","g":"Qm1…","rows":[ {"r":"task", "seq":813, …}, {"r":"msg", "seq":814, …} ]}
```

Rows are in increasing `seq`. A row replaces any cached row with the same
`r` and `id`; a row with `gone: true` removes it.

### `fetch` (request)

```json
{"t":"fetch","id":3,"g":"Qm1…","thread":"<task id>","before":700,"limit":50}
```

`thread` is a task id, or `null` for the main line. `limit` is at most 100.
With `"kind":"review"` in place of `thread`, the answer holds the `review`
messages of all threads: the ledger (R-102).

```json
{"t":"fetched","re":3,"ok":true,"rows":[ … newest first … ],"more":true}
```

## 5. Commands

```json
{"t":"cmd","id":4,"env":{"p":"…","k":"…","s":"…"}}
```

```json
{"re":4,"ok":true}
{"re":4,"err":{"code":"forbidden","msg":"only the AI's owner may decide this approval"}}
```

- The payload inside `env` names the command in its own `t`
  (`contracts/commands.md`).
- The signer `env.k` must be the device that this connection was
  authenticated for. Nobody can submit another device's envelope, so a
  repeated command can only come from its own signer.
- A home command has `g`. A host command has `to` equal to the node's device
  id and no `g`, or a `g` where noted.
- Effects are observed through `rows` (home surface) or `host.info` (host
  surface), never through the response.

## 6. Inbox (home to host)

The home decides which AI must see which message and queues an item per AI.

```json
{"t":"inbox","g":"Qm1…","items":[
  {"id":4411,"ai":"<member id>","why":"mention",
   "msg":{"r":"msg", …},"task":{"r":"task", …},
   "authz":{"by":"author"}}
]}
```

| Field | Meaning |
| --- | --- |
| `id` | increasing; names the item in `ack` |
| `why` | `mention`, `assign`, `review`, `reply`, `decided`, `confirmed`, `checks`, `resume`, `notice`, `merge` (table in `contracts/commands.md` section 3) |
| `jump` | optional, true when the owner chose to jump the queue (R-72) |
| `msg` | the message row that caused the item, with its original envelope |
| `task` | the task row, when the item is about a task |
| `authz` | why the host may act on it: `author` (the message is signed by a device of the AI's owner), `task` (it belongs to a task the owner authorised), `confirm` and `list` (step 2) |

- The host answers each item with `{"t":"ack","g":"…","id":4411}` when it
  delivers the item to the session or refuses it. Until then the item stays
  on the home; the host keeps undelivered items in memory only. The home
  deletes acknowledged items.
- On every `hello` from a host the home re-sends unacknowledged items for
  that host's AIs.
- An item with `why: "merge"` is for the host itself, not for the session:
  the host runs the merge for `task` and reports with a `task` command.
- What the host does with an item is in `flows/agent-turn.md`.

## 7. Host surface

Available when `welcome.owner` is true. The name describes the wire. In the
code, node-level requests (`device`, `pair.browser`, node settings, putting
`host.info` together) are handled by `internal/node`, and everything about
AIs and sessions by `internal/host` (`architecture.md` section 8).

### `host.info` (request, then pushes)

```json
{"t":"host.info","id":5}
```

The response and every later push with `"t":"host.info"` carry the whole
object:

```json
{"t":"host.info","re":5,"ok":true,
 "device":"…","name":"workstation","os":"macOS 15.2","version":"0.3.1",
 "open":2,"max_open":4,"offline":false,
 "homes":[{"g":"Qm1…","name":"Website"}],
 "relay":{"used":3250000,"limit":52428800},
 "agents":[
   {"agent":"claude","cli_version":"2.1.311","account":"personal",
    "login_ok":true,"caps":{"day_pct":80},
    "groups":[{"g":"Qm1…","member":"<member id>","level":2,
               "repo_config":false,"state":"working","task":"<task id>",
               "queued":0}]}],
 "settings":{"retention_days":90,"idle_minutes":15,"default_level":2,
             "all_level3":false,"approvers":[]}}
```

`agents` lists every supported CLI found on the machine; `account` is `null`
until the owner has declared it. In `groups`, `state` and `task` are the
derived state of the AI in that group (`contracts/rows.md` section 2),
`level` is the level of its task sessions, and `queued` counts held inbox
items.

### `audit` (request)

```json
{"t":"audit","id":6,"before":1763600000000,"limit":50}
```

```json
{"re":6,"ok":true,"rows":[
  {"ts":1763599000000,"g":"Qm1…","ai":"claude","what":"approval",
   "detail":"git push origin task/11-docs","by":"<device id>","result":"approved"}]}
```

### `pty.resize` (push, browser to node)

```json
{"t":"pty.resize","g":"Qm1…","ai":"<member id>","cols":120,"rows":32}
```

### Terminal channel

After the host accepted `takeover.start` for (g, member), the browser opens
a data channel labelled `pty:<g>:<member>`. Each message is a chunk of raw
bytes: keystrokes one way, terminal output the other. The host stops reading
from the pseudo-terminal while the channel's send buffer is above 1 MiB.
Closing the channel is hand-back. The host refuses `takeover.start` with
`err.code = "relay"` when the path to this browser is `relay`.

## 8. What a node accepts from whom

| Peer is | Accepted |
| --- | --- |
| a device of the node's owner | everything: host surface, and home surface for groups homed here |
| a device of another member of a group homed here (step 2) | home surface for that group only |
| the host of an AI member of a group homed here | home surface for that group, `ack` |
| anything else | the connection is not accepted (signalling was dropped) |

## 9. Error codes

| Code | Meaning |
| --- | --- |
| `version` | protocol version not supported |
| `bad_request` | payload does not match the contract |
| `bad_sig`, `unknown_device`, `not_for_me`, `stale` | envelope checks (`contracts/envelope.md`) |
| `forbidden` | this member may not do this |
| `not_found` | the group, task or message does not exist |
| `conflict` | the target is not in a state that allows this (already decided, already merged) |
| `paused` | the group or task is paused |
| `limit` | a runaway limit refused it; `msg` names the limit |
| `relay` | not available on a relayed path |
| `secret` | host only: text from an AI matched the outbound secret check and was not sent (R-137) |
| `busy` | the session cannot do this now (for example takeover during a switch) |
| `home_unreachable` | host only: the home of that group is not connected |

Every boundary failure maps to exactly one of these. Internal failures are
not mapped: the node logs them and the request fails with `internal`.
