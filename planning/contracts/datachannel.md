# Contract: data-channel protocol

What travels on a WebRTC connection between a client and a node. Row
shapes are in `contracts/rows.md`, the commands in `contracts/commands.md`.
Two surfaces share the channel and its framing: the **home surface**, any
client talking to a group home, and the **host surface**, the owner's
browser talking to one of the owner's nodes. A node answers both; which
messages it accepts from a peer depends on who the peer is (section 8).

## 1. Channels

| Label | Created by | Type | Carries |
| --- | --- | --- | --- |
| `ctl` | the offering side, in the offer | ordered, reliable, text | everything in this file except terminal bytes |
| `pty:<g>:<member>` | the owner's browser, after `takeover.start` succeeded | ordered, reliable, binary | raw terminal bytes, both directions |

## 2. Framing on `ctl`

- One JSON object per message. Field `t` is the type.
- The rules for every frame are in `contracts/conventions.md`: presence
  (section 2); requests, answers and pushes (section 3); sizes (section 4).
- A result too large for one message is split (`rows` batches, `fetch` pages).
- A message that is not a JSON object, or whose `t` the receiver does not
  know, is answered `bad_request` when it carries a request `id`, and is
  dropped otherwise. The connection stays open.

## 3. Session

The client's first message, `hello`, and the node's answer, `welcome`:

```json
{"t":"hello","id":1,"v":1,"kind":"browser","name":"phone browser","addrs":[]}
{"t":"welcome","re":1,"ok":true,"v":1,"device":"<node device id>",
 "name":"workstation","version":"v0.3.1","time":1763600000000,
 "addrs":["ws://192.0.2.10:47800/rv","wss://rv.example/rv"],
 "groups":[{"g":"Qm1…","name":"Website","seq":812}],"owner":true}
```

| Field | Meaning |
| --- | --- |
| `v` | protocol version. A node that cannot speak it answers `version` and keeps the connection open, so that the client can send `hello` again with a lower `v` |
| `kind` | `browser` or `host` |
| `addrs` | where the sender can be signalled. In `hello`, URLs at which this node may be reachable, `[]` from a browser. In `welcome`, what reaches the node without the rendezvous: its own `ws://<address>:<port>/rv` URLs, with the mapped external port when it has one, then its rendezvous URLs (`contracts/rendezvous.md` section 4) |
| `time` | the node's clock, in Unix milliseconds, when it sends `welcome` |
| `groups` | groups homed on this node that the peer's person may read |
| `owner` | true if the peer is a device of this node's owner; the host surface is then available |

- When the list of groups for a connected peer changes, the node pushes
  the whole list again, entries as in `welcome`, `[]` when the peer may
  read no group here: `{"t":"groups","groups":[{"g":"Qm1…","name":"Website","seq":812}]}`.
- A node keeps one inbound connection per client device. On first application
  of a valid ordinary offer it closes that signer's previous inbound
  connection and any pending inbound probe before it starts the new checks; a
  copy or a replay closes nothing, and a probe's first valid `hello` promotes
  it (`contracts/rendezvous.md` 2.3; hello timeout: `flows/connect.md` step
  11). Nothing is carried over: the client reads again with `sync`.
- A `hello` on a connection that has already had one is answered with a
  fresh `welcome`. A client sends it again when pairing has enrolled the
  newcomer, whichever side that is, and reads `owner: true` from the answer.
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

- during pairing (`flows/pairing.md`);
- after every `welcome`, each side sends every log the peer has a reason to
  know, in full from version 1;
- a device that accepts a new entry forwards it to every connected peer
  that keeps that log.

A log split over several `devlog` frames is adopted once it is complete.

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

Rows are in increasing `seq`. A row is applied only if no row with the same
`r` and `id` is cached or its `seq` is higher than the cached one's; an
equal or lower `seq` changes nothing and does not move the cursor. A
`gone: true` row stays cached as a marker that is not shown, so that an
older copy arriving later (a `fetch` page) cannot bring the row back.

The client's cursor for a group, the `since` of its next `sync`, is 0 at the
start and again from every `reset`; it becomes S at the `synced` that
follows, and then the `seq` of each live row it applies. Rows received
before `synced` and rows from `fetch` never move it.

### `fetch` (request)

```json
{"t":"fetch","id":3,"g":"Qm1…","thread":"<task id>","before":700,"limit":50}
```

- `thread` is a task id, or `null` for the main line. In its place,
  `"kind":"review"` asks for the review messages of all threads (the
  ledger), `"kind":"note"` for the notes (R-102), and `"r":"<row type>",
  "row":"<id>"` for that one row of the group, with `before` 0 and `limit`
  1 (unit HM-3).
- `before` is a `pos` (`contracts/rows.md`): the answer holds rows with a
  lower `pos`, newest first. `before: 0` asks for the newest page; no `pos`
  is 0. For an older page the client sends the lowest `pos` it received.
  `limit` is at most 100.

`{"t":"fetched","re":3,"ok":true,"rows":[ … ],"more":true}`: the home stops
filling `rows` when the next row would exceed the message limit; `more`
says whether older rows exist. `rows` is `[]` when there is nothing and
never a `gone` marker, which has no `pos` (`contracts/rows.md`).

## 5. Commands

`{"t":"cmd","id":4,"env":{"p":"…","k":"…","s":"…"}}` is answered
`{"re":4,"ok":true}`, with the command's result fields where it has some
(`{"re":4,"ok":true,"num":14}`), or with an error
(`{"re":4,"err":{"code":"forbidden","msg":"…"}}`).

- The payload inside `env` names the command in its own `t`.
- The signer `env.k` must be the device that this connection was
  authenticated for; a command signed by another device is `forbidden`.
- A command that the receiver has already accepted gets the same outcome
  again, under the `re` of the new request, and has no second effect
  (`contracts/commands.md`, "Once only").
- A home command has `g`; a host command has `to`, the node's device id.
- A home answers with the first of these that fails, in this order: the
  signer is the connection's device; the envelope checks
  (`contracts/envelope.md` section 3); `g` is there, else `bad_request`.
  These leave no receipt. Then the receipt answers a command that was
  answered before. Then, with the outcome kept as the receipt: the type
  and, for `group`, `member`, `task`, `post` and `act`, the `op`, `kind`
  or `action` name a command, else `bad_request`; the node is home to the
  group, else `not_found`, for every command but `group create`; `as` and
  the signer (`contracts/commands.md`, "Who"); and what the command's own
  table says.

## 6. Inbox (home to host)

The home decides which AI must see what and queues an item per AI. Every
item is an instruction to a host, so every item is signed by the home.

```json
{"t":"inbox","g":"Qm1…","items":[
  {"env":{"p":"{\"t\":\"item\",\"n\":\"…\",\"ts\":1763600000000,\"g\":\"Qm1…\",\"to\":\"<host device>\",\"item\":4411,\"ai\":\"<member id>\",\"why\":\"review\",\"task\":\"t12…\",\"commit\":\"9f2c1ab\"}",
          "k":"<home device>","s":"…"},
   "task":{"r":"task", …}}
]}
```

`env` is the item, built and signed by the home in the transaction that
queues it, and stored: what is sent later is what was decided then,
whatever the task has become. Its payload always has `t` (`item`), `n`,
`ts`, `g`, `to`, `item`, `ai` and `why`, and the fields the table gives its
`why`, each there exactly when the table says. Beside `env`, `msg` and
`task` are copies of the rows `env` names, taken when the item is sent, to
build the turn from (`msg.env` is the author's own envelope), and `cause`
is the signed `act` that led to the item, unchanged.

| `why` | `task` | `msg` | Other |
| --- | --- | --- | --- |
| `mention` | when the message is in a thread | the message | `jump`: `true` after the author's `jump` (R-72), else absent |
| `reply` | when the reply is in a thread | the reply | — |
| `assign` | always | the `task` message (task-machine rows 1, 8); `re` of `task start` (row 2); the latest `checks` or `review` message (row 11); absent when there is none | `cause` in rows 1 and 8: the `act` that confirmed the brief |
| `review`, `merge` | always | — | `commit`; for `merge`, `pr` when the group has a remote |
| `reviewed`, `checks` | always | the `review`, or the `checks`, message | — |
| `resume` | always | — | — |
| `decided` | when the card names a task | the card | `cause`: the `act` that decided it |
| `confirmed` | — | the brief | `cause`: the `act` that confirmed it or sent it back |
| `notice` | when the case is about a task | the message that records the case, when there is one | `code`; `detail` where the case has one (`contracts/task-machine.md` section 5) |

- **What a host accepts (step 1).** The item's envelope is signed by the
  home of that group, that device is in the log of the host's owner, `to`
  is this host and `ai` runs here. For `decided` on an approval the host
  also checks `cause` itself: signed by its owner or a named approver, about
  the request it holds (`flows/approval.md`). Rows without a signature are
  content, never authority. From step 2 a home may belong to someone else,
  and an item will also need the owner's own signature for that task.
- **Still current.** The signature is checked when an item arrives; whether
  it is still true is checked when the host **uses** it, after it has read
  the group's state on this connection. For a work item (`assign`,
  `checks`, `reviewed`, `review`, `resume`, `merge`): the task is active,
  the AI still has that role, and for `review` and `merge` the task is at
  that step with the item's commit. If not, the host refuses it (audit
  `refused`, then `ack`). A `notice` is delivered whatever the task has
  become. The home pushes a row change before the items it produces, and
  deletes items that a later change made pointless.
- The host answers `{"t":"ack","g":"…","item":4411}` when it delivers the
  item to the session or refuses it. The home deletes an acknowledged item
  only if the acknowledging connection is the item's host. On every `hello`
  from a host the home sends its unacknowledged items again.
- A `merge` item is for the host itself, not for a session. What the host
  does with an item is in `flows/agent-turn.md`.

## 7. Host surface

Available when `welcome.owner` is true. In the code, node-level requests are
handled by `internal/node`, AIs and sessions by `internal/host`
(`architecture.md` section 8).

### `host.info` (request, then pushes)

The response to `{"t":"host.info","id":5}`, and a push whenever one of its
fields changes, carry the whole object:

```json
{"t":"host.info","re":5,"ok":true,"device":"…","name":"workstation",
 "os":"macOS 15.2","version":"v0.3.1","open":2,"max_open":4,"offline":false,
 "update":null,"homes":[{"g":"Qm1…","home":"<device id>","name":"Website"}],
 "relay":{"used":3250000,"limit":52428800},
 "agents":[{"agent":"claude","cli_version":"2.1.311","account":"personal",
   "login_ok":true,"caps":{"day":{"unit":"turns","max":200}},
   "groups":[{"g":"Qm1…","member":"<member id>","level":2,"repo_config":false,
              "state":"working","task":"<task id>","queued":0}]}],
 "settings":{"retention_days":90,"idle_minutes":15,"default_level":2,"all_level3":false,"approvers":[]}}
```

- `homes`: the groups whose home this node knows, one entry per group: `g`,
  `home` (its home's device id), `name`. Step 2's `home.add` adds entries;
  the node keeps them in its settings under `homes` (`contracts/storage.md`
  section 2), so they survive a restart.
- `relay`: the `quota` answer of the rendezvous the node is connected to
  (`contracts/rendezvous.md` section 2). `account` is `null` until the
  owner has declared it. `agents` lists every supported CLI found here.
- `update`: `null`, or `{"version":"v0.3.2","status":"pending"}` after an
  update check (`contracts/local.md` section 9): `pending` when that version
  is installed and waits for a restart, `mismatch` when its file failed the
  checksum and nothing was replaced.
- In `groups`: `state` and `task` are the AI's derived state
  (`contracts/rows.md` section 2; `task` is optional), `level` is the level
  of its task sessions, and `queued` counts held items.

### `audit` (request)

`{"t":"audit","id":6,"before":1763600000000,"limit":50}` is answered with
`{"re":6,"ok":true,"rows":[…]}`, each row shaped as
`{"ts":1763599000000,"g":"Qm1…","ai":"claude","what":"approval","detail":"git push origin other-branch","by":"<device id>","result":"approved"}`.
`g`, `ai`, `by` and `result` are optional.

### Terminal

- The browser pushes `{"t":"pty.resize","g":"Qm1…","ai":"<member id>","cols":120,"rows":32}`.
- After the host accepted `takeover.start`, the browser opens a data channel
  labelled `pty:<g>:<member>`. Each message is a chunk of raw bytes. The
  host stops reading from the pseudo-terminal while the channel's send
  buffer is above 1 MiB. The channel closing, for any reason, is hand-back.
- The host refuses `takeover.start` with `relay` and closes an open `pty`
  channel unless its own path is `direct` (`flows/connect.md` steps 8, 9).

## 8. What a node accepts from whom

| Peer is | Accepted |
| --- | --- |
| a device of the node's owner | everything: host surface, and home surface for groups homed here |
| a device that passed the pairing exchange and is not yet in the owner's log | `hello` and `devlog` only, until a log entry makes it an owner device (`flows/pairing.md`); any other request is answered `forbidden` |
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
| `paused` | the task is paused (`contracts/task-machine.md` section 2) |
| `limit` | a runaway limit refused it; `msg` names the limit |
| `relay` | not available on a relayed path |
| `busy` | the session cannot do this now |
| `cli` | the CLI program failed; `msg` carries its error text |
| `secret` | host only: text from an AI matched the outbound secret check and was not sent (R-137) |
| `home_unreachable` | host only: the home of that group is not connected |
| `internal` | the node itself failed; the cause is in its log, not in `msg` |

Every boundary failure maps to exactly one of the codes above `internal`;
any other failure is logged by the node and answered with `internal`.
