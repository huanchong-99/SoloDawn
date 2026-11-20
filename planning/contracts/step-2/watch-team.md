# Contract (step 2): watching a terminal; the team-wide group

Sections 1 to 6: a member who is not the owner watches the terminal of a
takeover, with the owner's consent (R-81, design point D-14). Sections 7 to
10: the group kind `team`, without repository, tasks or AI members (R-14,
design point D-15).
The design points are in `steps/parallel-plan-step-2-3.md` section 1.

The `confirm` card is in `contracts/step-2/confirm.md`; what a host checks
on an item from a home that is not its owner's, in
`contracts/step-2/authority.md`; people and roles, in
`contracts/step-2/people.md`. This file changes no step 1 contract; it is
folded into them when step 1 is built (section 11).

Actors: O, the owner of the AI; B, O's browser in the takeover; N, the AI's
host; W, the watching person, with browser WB; H, the home of the group.

## 1. What can be watched

The terminal of a takeover, while it lasts (`flows/takeover.md`): the bytes
the interactive CLI writes to its pseudo-terminal. Outside a takeover a
session has no terminal and there is nothing to watch.

- A watcher receives bytes and sends none. Consent is per person and per
  takeover, and ends at hand-back.
- The path limit is that of takeover: `direct` only, judged by each end from
  its own selected pair (`flows/connect.md` steps 8 and 9).
- N does not read, store or interpret what it forwards (R-85), so a watcher
  sees the terminal from the moment the channel opens, with no replay.
- The owner's list of people who may direct the AI (`member.allow`, R-53)
  does not cover watching: every watcher is confirmed by a card.

## 2. From the card to the grant

1. W asks with a `confirm` card whose `body.what` is `watch` and which names
   the AI (who may ask and the card's states: `contracts/step-2/confirm.md`).
2. O confirms on any of O's devices: the signed `act confirm`.
3. H queues an inbox item for N with `why: "watch"`. Like `merge`, it is for
   the host itself and not for a session. It is never held: not during the
   takeover, not while the group is paused.

```json
{"env":{"p":"{\"t\":\"item\",\"n\":\"…\",\"ts\":1763600000000,\"g\":\"Qm1…\",\"to\":\"<host device>\",\"item\":4420,\"ai\":\"<member id>\",\"why\":\"watch\",\"msg\":\"<card id>\"}",
        "k":"<home device>","s":"…"},
 "msg":{"r":"msg","kind":"confirm", …},
 "cause":{"p":"…","k":"<a device of O>","s":"…"}}
```

The item has no `task`; `msg` and `cause` are always present in it. N
checks the item as every item (`contracts/datachannel.md` section 6,
`contracts/step-2/authority.md`), and then, in this order:

| # | Check |
| --- | --- |
| 1 | `cause` passes checks 1, 3 and 5 of `contracts/envelope.md` section 3 as a command, its signer is a currently valid device of N's owner, and it is an `act` for the item's group with `action: confirm` whose `msg` is the item's `msg` and whose `arg` is the digest of `contracts/step-2/confirm.md` section 6 for the item's `ai` and `msg.env` |
| 2 | `msg.env` passes checks 1 and 3, and is a `post` of kind `confirm` whose `n` is that id, whose `g` is the item's group, whose `body.what` is `watch` and which names the item's `ai` |
| 3 | the signer of `msg.env` is a currently valid device in the log of a person, by the `device` rows of the group, each of which carries its signed entry (`contracts/rows.md` section 3). That person is the watcher |
| 4 | no audit row `watch` names this card: N acts on a card once |
| 5 | the AI is in `takeover` in this group now |

N reads the card from `msg.env`: the unsigned parts of `msg` are content,
and so is the watcher's `member` row. That W is a member is the home's
check when it stores the card, and what O confirms is the person.

- All pass: N keeps the **grant** (group, AI, watcher's user id) in memory,
  writes the audit row `watch` with the card id as `detail`, the device that
  signed `cause` as `by` and result `granted`, and acknowledges the item.
- Any fails: audit row `refused`, then `ack`, as for every refused item.
- The grant is for the takeover that is running when the item arrives. When
  only check 5 fails, N also writes the audit row `watch` for the card,
  with result `late`: the card cannot be used in a later takeover, and W
  asks again during one.

## 3. The watcher's connection

WB connects to N as to any node (`flows/connect.md`); the AI's member row
names N in `host`. Two step 1 lists grow by one entry each:

- `contracts/envelope.md` section 3, check 2: a node has a reason to listen
  to the devices of a person who holds a grant from it; a browser has one
  to listen to the host of an AI of a group it reads.
- `contracts/datachannel.md` section 8:

| Peer is | Accepted |
| --- | --- |
| a device of a person who holds a grant from this node | `hello`, `devlog`, `watch.start` and the `watch` channel of that grant, besides what another row gives the peer |

`welcome` is as for any peer: `owner` is false, and `groups` lists what the
person may read on N, often nothing. When N is also the group's home, WB
uses the connection it has. When a person's last grant ends, N closes the
connections of their devices that no other row accepts.

## 4. The request and the channel

On `ctl`, a request from WB:

```json
{"t":"watch.start","id":7,"g":"Qm1…","member":"<member id>"}
```

Answered `{"re":7,"ok":true}`, or with one of:

| Code | When |
| --- | --- |
| `forbidden` | N holds no grant for this peer's person and that AI: none arrived yet, it was refused, or it has ended |
| `relay` | N's own path to this peer is not `direct` |

It is a plain request, like `host.info`, with no receipt: the authority is
O's signature on the card, and the connection says which device asks. N
writes an audit row `watch` with WB's device as `by` and result `opened`.
After `ok`, WB opens the channel (`contracts/datachannel.md` section 1):

| Label | Created by | Type | Carries |
| --- | --- | --- | --- |
| `watch:<g>:<member>` | a watcher's browser, after `watch.start` succeeded | ordered, reliable, binary | raw terminal bytes, from the node only |

- N writes every chunk it reads from the pseudo-terminal to the `pty`
  channel and, unchanged, to every open `watch` channel of that AI.
- N closes a `watch` channel on which the browser sends a message, without
  writing it to the pseudo-terminal, and one it did not answer `ok` for on
  this connection.
- The owner's terminal is never slowed by a watcher: when the send buffer
  of a `watch` channel is above 1 MiB, N closes that channel. The limit of
  the `pty` channel stays what it is; while N has stopped reading for it,
  watchers receive nothing either.
- **Size.** When a `watch` channel opens, and whenever N applies a
  `pty.resize` from B, N pushes the size to the watcher on `ctl` in the
  shape B uses (`contracts/datachannel.md` section 7, "Terminal"). A
  `pty.resize` from a watcher is not read.
- One device has one `watch` channel per AI; the number of watchers of a
  terminal has no limit.

## 5. What the owner sees and can stop

Each entry of `agents[].groups` in `host.info`
(`contracts/datachannel.md` section 7) gains an always-present list,
`"watchers":["<user id>"]`: the people who hold a grant for that AI in that
group now, `[]` when nobody does. The terminal page shows them.

A host command, by O as every host command (`contracts/commands.md` 2):

| `t` | Fields | Effect |
| --- | --- | --- |
| `watch.end` | `g`, `member`, `user` | N drops that person's grant for that AI and closes their `watch` channels; audit row `watch`, result `ended`. `not_found` when there is no such grant |

## 6. How watching ends

| Event | Result |
| --- | --- |
| hand-back, for any of the reasons of `flows/takeover.md` D and E | before step 13 there, N closes every `watch` channel of the AI and drops every grant for it |
| `watch.end` | that person's channels close, the grant is dropped |
| the watcher's `member` row in the group becomes a `gone` marker (`contracts/step-2/people.md` 2.4) | N drops that person's grants in the group and closes their `watch` channels |
| WB closes the channel, N closes it for a full buffer, or the connection is lost | the grant stays; WB may send `watch.start` again while the takeover lasts |
| the path of WB or of N to the other becomes `relay` | that end closes the `watch` channel; the grant stays |
| N restarts | grants are in memory only and are gone; the session is treated as handed back (`flows/takeover.md` E) |

H takes no part after the item and learns nothing about watchers.

## 7. The team-wide group

A group of kind `team` has people, a main line and notes: no repository,
no clone on any node, no AI member, no secretary, no task, no thread. R-14
allows a team one such group; nothing counts them, since the only "team"
on the wire is the member list of this group.

`group create` (`contracts/commands.md` 1.1) with `kind: "team"`: `repo`
is optional in that command and is left out; `base` is `""`, `forge` is
`"none"` and `checks` is `[]`. Anything else in those four is
`bad_request`. With `kind: "project"`, `repo` stays required.

The row has the shape of `contracts/rows.md` section 1 with those values.
`repo` and `secretary` are absent for the life of the group; `pass`,
`pass_foundation` and `limits` hold the defaults and are used by nothing:
`group set` with `checks`, `pass`, `pass_foundation` or `limits` is
`bad_request`. `kind` never changes after `create`.

## 8. What is refused

All with `conflict`: the group is of a kind that does not allow this.

| Receiver | Refuses, for a group of kind `team` |
| --- | --- |
| the home | every `task` command |
| the home | `post` of any kind but `text` and `bulletin` (section 10), so also `brief` and `confirm`; a `post` whose `thread` is not `null`; a `post` with `as` |
| the home | `member add_ai` |
| a node | `host.join`: after it has connected to `home` and read the group row, before any clone, and without sending `member add_ai` |

The home of a `project` group refuses a `post` of kind `bulletin` in the
same way. Nothing else is refused by kind: what needs an AI member or a
card (`member status`, `act`, `group set` with `secretary`) already fails
by the step 1 rules. No inbox item is ever queued in a team group.

## 9. People and messages

- People join, get roles and are removed as in any group
  (`contracts/step-2/people.md`); the group is moved and copied as any
  group (`contracts/step-2/move.md`).
- A person writes `post` of kind `text` in the main line. Mentions name
  people, who are reminded as in any group (`contracts/commands.md` 1.7).
- `sync`, `fetch`, `note`, `forget`, `push`, `group set` with `name`,
  `group pause` and `group resume` work as in a project group.
- A client reads `kind` from the group row: a team group has a main line
  and a member list, and no task, AI or brief control.

## 10. A secretary's notice (R-14)

Accepted by the coordinator (section 12, point 1). D-15 says that a
secretary posts there "with the `post` tool and a group id"; this section
says how the node knows that group's home and as whom the home accepts the
message, since no AI is a member.

**As whom.** The secretary's node signs the `post` as a device of its
owner, with no `as`: the person principal of `contracts/commands.md`. The
owner is a member of the team group, so the home accepts it by the rule it
has, and the row's `by` is the owner's member id. Who wrote it is said in
the body of a new message kind:

| `kind` | Where | Signed by | `body` | `state` |
| --- | --- | --- | --- | --- |
| `bulletin` | main, in a team group only | a person; in practice that person's node | below | `null` |

```json
{"from":"Qm1…","group":"Website","ai":"Claude Code"}
```

`from` is the id of the project group, `group` its name and `ai` the display
name of its secretary, all three written by the node. Signed: every field.
Added by the home: nothing. The home cannot check them, because it may know
nothing of that project group: they are the statement of the signer's owner,
which is all that any message "from an AI" is (`architecture.md` section 4). A
client shows a bulletin as written by `ai` of `group`, on a computer of the
person in `by`. One row for the `post` table of `contracts/commands.md` 1.3:

| `kind` | By | Extra checks | Effect beyond storing the row |
| --- | --- | --- | --- |
| `bulletin` | a person who may post in the group, that is a member | `thread` and `re` are `null`, `main` is `false`, `body.from` is an id; in a project group, section 8 | as for `text`: each person in `mentions` is reminded (`contracts/commands.md` 1.7) |

**A person-level post path.** This gives a node a way to post as its owner, a
person, with no `as`. The home cannot tell such a post from one the person
sent from a browser; only its kind, `bulletin`, says a node wrote it.

**Which home.** The owner names the team groups to their node, in a new key of
`settings.set` (`contracts/commands.md` section 2) that `host.info` shows in
`settings`: `"teams":[{"g":"<team group id>","home":"<device id>"}]`, replaced
as a whole, `[]` by default. The node connects to each `home` as a device of a
member (`contracts/datachannel.md` section 8, third row) and reads the group
with `state_only`. Nothing is cloned and no AI joins.

**The tool.** `post` (`contracts/mcp-tools.md` section 1) gains one
argument, listed for the secretary only:

| Argument | Meaning |
| --- | --- |
| `group` | the id of a team group in `settings.teams`. With it, only `text` and `mentions` are read; `mentions` are names of people of that group. Returns `{"id":"…"}`. Maps to `post` kind `bulletin` on that group's home |

Tool errors: `not_found` when `group` is not in `settings.teams` (the text
lists the valid ids with the group names); `conflict` when that group is
not of kind `team`; `home_unreachable`; `secret` (R-137, as on every post).

A secretary writes there and reads nothing there (`read_thread` takes no
`group`); a person carries a matter from the team group into a project.

## 11. Where each part goes when it is folded

Unit P-4 freezes the new shapes (`contracts/conventions.md` section 5).

| Step 1 file | Gains |
| --- | --- |
| `contracts/datachannel.md` | section 1: the `watch` channel; section 7: `watch.start`, `pty.resize` to watchers, `watchers` in `host.info`, `teams` in its `settings`; section 8: the grant row |
| `contracts/envelope.md` | section 3, check 2: the two reasons of section 3 here, and a node's reason to listen to the home of each team group in its `settings.teams` (section 10) |
| `contracts/commands.md` | 1.1: `create` of kind `team`, and the `set` it refuses (section 7); 1.3: `bulletin`; section 2: `watch.end`, `teams` in `settings.set`; section 3: item `watch`; the refusals of section 8 here |
| `contracts/messages.md` | kind `bulletin` and its body |
| `contracts/rows.md` | section 1: the values of a team group |
| `contracts/mcp-tools.md` | `post` with `group`; the secretary's list in `tools.json` |
| `contracts/storage.md` | audit `what` gains `watch`; `setting` key `teams` |
| `flows/takeover.md`, `flows/agent-turn.md`, `architecture.md` | takeover section G becomes sections 1 to 6 here; agent-turn A and B step 3: the item `watch` goes to no session and is never held (section 2); architecture section 3: a watcher's browser to the AI's host (`ctl`, `watch:*`), a node to the home of a team group |

## 12. Settled by the coordinator (2026-11-20)

1. **How a secretary reaches the team group**: the `bulletin` of section 10,
   with the person-level post path it states. Joining the AI as a member
   without a clone would need a refusal on every path that wakes an AI.
2. **The confirmation binds the card's signer**: the digest of
   `contracts/step-2/confirm.md` section 6 covers the request's `k`, so
   check 1 of section 2 fails for the same `p` signed by another person.
3. **The item `why: watch`** is the one `confirm.md` section 5 names.
4. **Consent lasts one takeover and can be withdrawn** (sections 2, 5),
   with the list `watchers` and the command `watch.end`. A grant also ends
   when the watcher is removed from the group (section 6).

## 13. Not verified

- Nothing here was run. No spike covers a second data channel fed from the
  pseudo-terminal bridge.
- `[unverified]` That a browser terminal draws a usable screen when it joins a
  running full-screen CLI without the earlier bytes, and that the CLIs redraw
  on a resize. If not, the remedy is for O to resize, not for N to replay.
- `[unverified]` That every engine of the baseline sees promptly that a
  channel with a full send buffer was closed.
- `contracts/step-2/move.md` did not exist yet.
