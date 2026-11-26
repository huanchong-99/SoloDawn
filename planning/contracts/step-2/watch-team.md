# Contract (step 2): watching a terminal

A member who is not the owner watches the terminal of a takeover, with the
owner's consent (R-81, design point D-14 of
`steps/parallel-plan-step-2-3.md` section 1). The file keeps its name; the
team-wide group it also described ended with the decision of 2026-11-26:
there is one kind of group, and a group without AIs is what the team group
was (R-10).

The `confirm` card is in `contracts/step-2/confirm.md`; people and roles, in
`contracts/step-2/people.md`. This file changes no step 1 contract; it is
folded into them when step 1 is built (section 7).

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
2. O confirms on any of O's devices with `act confirm`.
3. H queues an inbox item for N with `why: "watch"`. It is for the host
   itself and not for a session, the one item that is. It is never held:
   not during the takeover, not while the group is paused.

```json
{"item":{"item":4420,"ai":"<member id>","why":"watch","msg":"<card id>"},
 "msg":{"r":"msg","kind":"confirm", …}}
```

The item has no `task`; `msg` is always present in it. The watcher is the
person whose member row is the card's `by`. N takes the item from its home
as every item (`contracts/datachannel.md` section 6) and checks only that
the AI is in `takeover` in this group now.

- It is: N keeps the **grant** (group, AI, watcher's user id) in memory
  and acknowledges the item.
- It is not: N acknowledges the item and keeps nothing. The grant is for
  the takeover that is running when the item arrives; W asks again during
  a later one.

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

It is a plain request, like `host.info`, with no receipt: the grant is the
authority, and the connection says which device asks. After `ok`, WB opens
the channel (`contracts/datachannel.md` section 1):

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
| `watch.end` | `g`, `member`, `user` | N drops that person's grant for that AI and closes their `watch` channels. `not_found` when there is no such grant |

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

## 7. Where each part goes when it is folded

Unit P-4b freezes the new shapes (`contracts/conventions.md` section 5).

| Step 1 file | Gains |
| --- | --- |
| `contracts/datachannel.md` | section 1: the `watch` channel; section 7: `watch.start`, `pty.resize` to watchers, `watchers` in `host.info`; section 8: the grant row |
| `contracts/envelope.md` | section 3, check 2: the two reasons of section 3 here |
| `contracts/commands.md` | section 2: `watch.end`; section 3: item `watch` |
| `flows/takeover.md`, `flows/agent-turn.md`, `architecture.md` | takeover section G becomes sections 1 to 6 here; agent-turn A and B step 3: the item `watch` goes to no session and is never held (section 2); architecture section 3: a watcher's browser to the AI's host (`ctl`, `watch:*`) |

## 8. Settled by the coordinator (2026-11-20)

1. **The item `why: watch`** is the one `confirm.md` section 5 names.
2. **Consent lasts one takeover and can be withdrawn** (sections 2, 5),
   with the list `watchers` and the command `watch.end`. A grant also ends
   when the watcher is removed from the group (section 6).

## 9. Not verified

- Nothing here was run. No spike covers a second data channel fed from the
  pseudo-terminal bridge.
- `[unverified]` That a browser terminal draws a usable screen when it joins a
  running full-screen CLI without the earlier bytes, and that the CLIs redraw
  on a resize. If not, the remedy is for O to resize, not for N to replay.
- `[unverified]` That every engine of the baseline sees promptly that a
  channel with a full send buffer was closed.
