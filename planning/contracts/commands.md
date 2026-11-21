# Contract: commands

A command is the payload of a signed envelope (`contracts/envelope.md`) sent
in a `cmd` message (`contracts/datachannel.md` section 5). This file lists
every command, who may send it, and what the receiver does. How task
commands move a task is in `contracts/task-machine.md`.

Every payload has `t`, `n`, `ts`. Home commands have `g`. Host commands have
`to`. When a node signs for one of its AIs the payload has `as`, the AI's
member id.

**Who.** A signer is one of three principals, and the tables say which:

- a **person**: a device in that person's log, with no `as`;
- an **AI**: the device that is the AI's `host`, with `as` = that AI;
- a **host**: a node speaking for itself (facts about tasks, status).

"Admin" is a person with role `owner` or `admin`. "Secretary" is the AI that
`group.secretary` names.

A signer with no `as` is the person whose log holds it and, when it is a
node, also a host: a row of the tables is met by whichever of the two it
names. A home takes a command only from a device of a member of the group,
and `group create` only from a device of its own owner; any other signer
that passed the envelope checks is answered `forbidden`. An `as` that is no
text, or an empty one, is `bad_request`; one that does not name an AI whose
`host` is the signer is `forbidden` (`contracts/envelope.md` section 5).
Should the logs of two people hold one device, a home reads it as a device
of the person with the lower user id; step 2, which brings the second
person, settles what such a device may do.

**Once only.** The receiver stores a receipt for every command it accepts,
keyed by signer and `n`, with the outcome (ok and its result fields, or the
error code), in the same transaction as the change. A command that arrives
again gets that outcome, under the request id of the new request, and has
no second effect. A command whose `ts` is more than 24 hours from the
receiver's clock is refused with `stale`; receipts are kept for that long
(`contracts/storage.md`).

A command is accepted, in this sense, once it passed the checks that
`contracts/datachannel.md` section 5 puts before the receipt: it arrived on
the connection of its signer, passed the envelope checks and, as a home
command, has its `g`. Whatever the receiver answers after that, a refusal
included, is the outcome its receipt keeps; a command that one of those
checks refused leaves no receipt. The answer to a repeated refusal has the
same code. Its `msg` is not kept, and need not be the same.

Two results name something that lives only in memory: the link of
`pair.browser` and the terminal of `takeover.start`. Their receipts record
that the command succeeded, not the link or the terminal. A repeat is
answered `ok` with the original link while that link is still valid, or
`ok` while that terminal is still open on this connection; otherwise
`conflict`. A repeat never creates a new link or a new terminal.

**Results.** Effects are read from rows. A few commands also answer with
named fields, listed in the tables as "Result"; an answer has only the
result fields of its command.

**Presence** follows `contracts/conventions.md` section 2. The fields a
table lists for a command are all in its payload, except these optional
ones:

- where a table says "any of", only the fields being changed are sent, and
  a command that sends none of them is `bad_request`;
- `as`, there only when a host signs for one of its AIs;
- in `act`: `arg` and `text` (which action has which: 1.4);
- in `task assign`: `assignee` and `reviewer` (at least one of the two; with
  neither it is `bad_request`);
- in `member status`: `task` (the AI is on no task) and `event` (there is
  no fact to report);
- in `task submitted`: `pr` and `pr_url` (the group has no remote);
- in `push`: either `sub`, `vapid` and `on`, or `off` alone (1.7);
- in `takeover.start`: `task`.

A `post` always carries `kind`, `text`, `thread` (or `null`), `main`,
`mentions`, `re` (or `null`) and `body`; what `body` must hold for each
kind is in `contracts/messages.md`. In `task start`, `re` is the message
that led to the task, or `null`; in `note`, `task` is a task id or `null`.

## 1. Home commands

### 1.1 `group`

| `op` | Fields | By | Effect |
| --- | --- | --- | --- |
| `create` | `name`, `kind`, `repo`, `base`, `forge`, `checks`, `member_name` | a person: a device of the receiving node's owner | the node becomes home of group `g` = `n`: the payload carries `g` like every home command, and a `create` whose `g` is not its `n` is `bad_request`; group row with `pass` 90, `pass_foundation` 70 (R-101) and the `limits` of the example in `contracts/rows.md` section 1; a `member` row for that person with role `owner` and `member_name` as its `name`; a `groups` push to that person's connected devices |
| `set` | any of `name`, `checks`, `pass`, `pass_foundation`, `limits`, `secretary` | an admin | group row updated. `limits`, when sent, is the whole object of `contracts/rows.md` section 1. `secretary` must name an AI member that is not assignee or reviewer of a task that is not final, else `conflict`; the previous secretary's member row loses the flag |
| `pause` | — | an admin | `group.paused` set; `system` `group_paused`; hosts interrupt running turns of the group and hold items. A `pause` of a paused group is accepted and changes nothing |
| `resume` | — | an admin | `group.paused` cleared; `system` `group_resumed`; task-machine row 24. A `resume` of a group that is not paused is accepted and changes nothing |

A group has one secretary (R-20). While it has none, main-line messages
that mention nobody reach no AI and no brief can be posted; the client asks
for one when the group is created.

### 1.2 `member`

| `op` | Fields | By | Effect |
| --- | --- | --- | --- |
| `add_ai` | `id`, `cli`, `name`, `account`, `secretary`, `approvers` | a host whose owner is a member | member row with `host` = the signer. `name` must be unused in the group, else `conflict`. `secretary: true` is accepted when the group has none or the owner is an admin, and sets `group.secretary`; a previous secretary's member row loses the flag |
| `status` | `id`, `state`, `task`, `path`, `usage`, `event` | the member's host | member row updated; `event`, when present, becomes a `system` row (1.9) |
| `set` | `id`, any of `name`, `account`, `allow`, `approvers` | the AI's owner; for `approvers` alone, also the AI's host | member row updated. A new `name` must be unused in the group, else `conflict` |
| `remove` | `id` | the AI's owner, or an admin | refused with `conflict` while the AI is assignee or reviewer of a task that is not final: reassign or cancel first. Otherwise the member row is replaced by a `gone` marker. Removing the secretary clears `group.secretary` and writes `system` `no_secretary` |

On `add_ai` and on `set` with `approvers`, the home adds the owner's user id
to `approvers` when it is missing (`contracts/rows.md` section 2); the owner
is the person whose host signed `add_ai`. Adding people is step 2.

### 1.3 `post`

Fields: `kind`, `text`, `thread`, `main`, `mentions`, `re`, `body`, `as`.
The message id is `n`.

| `kind` | By | Extra checks | Effect beyond storing the row |
| --- | --- | --- | --- |
| `text` | a person, or an AI | — | item `mention` for each mentioned AI; a main-line message from a person that mentions no AI produces one for the secretary |
| `progress`, `question` | an AI | `progress` only in a thread | item `mention` for each mentioned AI |
| `report` | an AI | `thread` is a task whose assignee is `as` | — |
| `brief` | the secretary | `body.requester` is a member; every assignee and reviewer is a working AI of the group, the two differ, and `depends` names only earlier entries of the same brief; `body.replaces`, when not `null`, is a brief whose `state.status` is `pending` or `revise`, else `conflict` | `state.status = pending`; the brief named in `body.replaces` becomes `superseded` |
| `summary` | the secretary | `thread` is a `merged` task | the home adds the whole `body` from the task row; `main = true` |
| `approval` | an AI | `thread` = `body.task`, or `null` when `body.task` is `""`; `as` = `body.ai`; for a task, `as` is its assignee or reviewer | `state.status = pending`; `main = true`; with a task, task-machine row 17 |
| `decision` | an AI | `thread` = `body.task`, or `null` when the body names no task | `state.status = pending`; `main = true`; task-machine row 19 |
| `call` | an AI | the callee is another AI member; `body.depth` ≤ `limits.ask_depth` | `state.status = waiting`; item `mention` for the callee |
| `reply` | an AI | `re` is a `call` addressed to `as` whose `state.status` is still `waiting`, else `conflict`: a call is answered once | the call's `state` becomes `answered` with `reply` and `ms`; item `reply` for the caller |
| `review` | an AI | `thread` = `body.task`; task-machine row 7 | Result: `passed`, `round` |

`handback` is postponed with takeover.

Rules for every `post`:

- A member named twice in `mentions` is `bad_request`, and a mentioned id
  that is no member of the group is `conflict`.
- A post never queues an item for its own author.

Rules for every `post` by an AI:

- A message in a thread that mentions an AI counts a round (task-machine
  rows 21 and 22).
- If an AI has been sent `limits.wakes_per_hour` items in the last hour, a
  new item for it is held until the hour allows it. The count is kept in
  memory by the home and starts again when the home restarts.
- The outbound secret check (R-137) runs on the AI's host before signing. A
  refused message never reaches the home (`contracts/local.md` section 8).

### 1.4 `act` and `jump`

`act` fields: `msg` (a card), `action`, `arg`, `text`.

| Card | `action` | By | Effect |
| --- | --- | --- | --- |
| `brief` | `confirm` | its requester, or an admin | the checks of a `brief` post (1.3) are made again, and a brief that fails one is answered `conflict`. Then `state.status = confirmed`; one task row and one `task` message per entry of `body.tasks`; task-machine row 1 |
| `brief` | `revise` | same | `state.status = revise`, `state.note = text`; item `confirmed` for the secretary |
| `approval` | `approve`, `deny` | a person in `body.approvers` | `state` updated; task-machine row 18 |
| `decision` | `choose` | a person in `body.to`, or an admin | `state.status = decided` with `choice = arg` and `text`; task-machine row 20 |

On a decision with options, `choose` takes `arg` (the `id` of one of
`body.options`), `text`, or both, and `text` alone is also accepted on a
card with options (2026-11-21 owner decision); on a decision whose `options`
is empty it takes `text` and no `arg`. `revise` may have `text`. `confirm`,
`approve` and `deny` have neither. Anything else is `bad_request`.

Acting on a card whose `state.status` is not `pending` returns `conflict`.
A decided card keeps the signed `act` in `state.act`.

`jump` (R-72), fields: `msg`. By the person who wrote message `msg`. The
newest unacknowledged item that a message of that person in the same
thread (or the main line, when `msg` has no thread) produced gets
`jump: true` and is sent again; when there is none, `conflict`.
The host does the rest (`flows/agent-turn.md` C).

### 1.5 `task`

When each is allowed, and what it does, is `contracts/task-machine.md`.

| `op` | Fields | By | Result |
| --- | --- | --- | --- |
| `start` | `id`, `title`, `plan`, `re`, `origin` (`direct`, `takeover`) | a working AI, not the secretary | `num` |
| `branch` | `id`, `branch` | host of the assignee | — |
| `checked` | `id`, `ok`, `items`, `commit` | host of the assignee | — |
| `submitted` | `id`, `commit`, `pr`, `pr_url`, `stats` | host of the assignee | — |
| `cloud` | `id`, `commit`, `ok`, `items` | host of the assignee | — |
| `merged` | `id`, `commit` (the reviewed commit that was merged), `merge_commit`, `stats` | host of the assignee | — |
| `conflict` | `id`, `commit`, `detail` | host of the assignee | — |
| `fail` | `id`, `reason` (`crash`, `timeout`), `detail` | host of the assignee | — |
| `preempted` | `id`, `msg` (the message whose jump interrupted the task) | host of the assignee | — |
| `assign` | `id`, `assignee`, `reviewer` | the secretary, or an admin; for a `failed` task an admin only | — |
| `pause` | `id` | the secretary, or an admin | — |
| `resume` | `id` | the secretary, or an admin (for a `failed` task, or one paused by the round limit, an admin only); the host of the assignee, for its own pre-empted task | — |
| `cancel` | `id` | the secretary, or an admin (at step `merge` an admin only); the host of the assignee when `origin` is `takeover` | — |

`branch` of `task branch` is `task/<num>-<slug>` (`contracts/local.md`
section 7); any other value is `bad_request`. `items` of `checked` and
`cloud` has the shape of `items` in a `checks` body
(`contracts/messages.md` 2.8), which the home writes from it.
`stats` of `submitted` and `merged` is `{"added":…,"removed":…}`, the line
counts of `git diff --shortstat <base-ref>...<commit>` (`contracts/local.md`
section 7), written into the task row's `stats`.

### 1.6 `note` and `forget`

| `t` | Fields | By | Effect |
| --- | --- | --- | --- |
| `note` | `id`, `task`, `text` | a person, or an AI | note row written |
| `forget` | `r` (`note` or `msg`), `id` | an admin | a note, or a `review` message, is replaced by a `gone` marker; any other message is refused with `conflict` |

Notes and `review` messages are not removed by retention (R-102). `forget`
is how a person deletes one by hand, and it deletes nothing else: R-102
names only the project memory; every other message leaves by retention.

### 1.7 `push`

By a browser, for itself: `{"sub":{"endpoint":"…","p256dh":"…","auth":"…"},
"vapid":{"pub":"…","priv":"…"},"on":["mention","approval","decision",
"brief","done"]}`, or `{"off":true}`. A `push` whose `off` is `true` is the
second form, whatever else it holds; any other `push` is read as the first,
and its `off` is not read. The browser creates the VAPID key pair
and hands it to each home it wants reminders from. `g` routes the command,
as in every home command; the subscription covers all of that browser's
groups on that home.

A reminder is sent to a subscribed browser when an event in its `on`
concerns its person and **that browser** is not connected to this home at
that moment. Other devices of the person, a node included, do not count.

A reminder goes through the browser vendor's push service, encrypted end to
end for that browser (RFC 8291); the vendor sees ciphertext only (R-163).
Its content is a short summary:

```json
{"g":"<group id>","msg":"<id of the card or message>","group":"Website",
 "kind":"approval","who":"Codex","title":"Install the browser test dependencies"}
```

| Field | Content |
| --- | --- |
| `kind` | `approval`, `decision`, `brief`, `mention`, `done` |
| `who` | display name of the member it is about: the AI asking, the secretary, the author of the mention |
| `title` | one line, at most 80 characters. `approval`: the first line of the justification the CLI attached (`body.why`), absent if there is none. `decision`: the question. `brief`, `done`: the brief or task title. `mention`: absent |

`title` is optional; every other field is always present.

Never included: message text, the command of an approval, code, file
names. The browser shows `group · kind · who: title` in the user's language
and opens the card `msg`.

### 1.8 Review outcome

On `post` with `kind = review` the home computes `ok`: `score` ≥
`task.pass` and no item has `veto`. A veto sets `score` to 0. What follows
is task-machine row 7.

### 1.9 Host events

`member status` carries `event` when the host has a fact to report. An
event is the body of a `system` message without `ai`
(`contracts/messages.md` 2.11, which lists the codes a host reports and the
fields of each). The home adds `ai`, the `id` of that `member status`, and
writes the row in the thread of the event's `task` when it has one, else in
the main line.

For `login_invalid`, `cap_reached` and `rate_limited` the home also gives
the secretary a `notice`. A closed host connection sets `state = offline`
for its AIs. Nothing else is derived from a status.

## 2. Host commands

Sent by the owner's browser to one of the owner's nodes. The node accepts
them only from a device in its owner's device log, with `to` equal to its own
device id. Receipts apply as for home commands.

| `t` | Fields | Effect |
| --- | --- | --- |
| `agent.set` | `agent`, `account` | declares the account type (`personal`, `team` or `api_key`, else `bad_request`) of a CLI found on this machine (`claude` or `codex`, else `bad_request`; `conflict` when not found here; R-50); sets `login_ok` back to `true` in `host.info` (`flows/agent-turn.md` E) and writes audit `setting` (`account <account>`); required before the AI can join a group |
| `perm.set` | `agent`, `g`, `level` (1, 2, 3), `repo_config` (bool) | permission level for that AI's sessions in that project (R-40), and whether repository-supplied AI configuration is loaded (R-45; for which sessions: `contracts/drivers.md` 1.1, 1.2); records an audit row of `what = setting`; takes effect the next time a session's process starts |
| `settings.set` | any of `retention_days`, `idle_minutes`, `max_open`, `default_level`, `all_level3`, `approvers`, `caps` | postponed after 1.0 (with `U-12`): node settings; for `approvers` the node sends `member set` with them for each of its AIs to that AI's home |
| `host.join` | `g`, `home`, `agent`, `name`, `secretary` | the node connects to `home`, clones the project if this node has no clone of it yet (R-11), and sends `member add_ai` |
| `host.leave` | `g`, `agent` | the node sends `member remove`; if the home accepts it, the AI's sessions in that group are closed (it is refused while the AI has unfinished tasks) |
| `takeover.start` | `g`, `member`, `task` (optional) | `flows/takeover.md`; refused with `relay`, `busy` or `cli` |
| `takeover.end` | `g`, `member` | hand-back |
| `host.offline`, `host.online` | — | R-64: `offline` interrupts and closes every session and tells every home; items stay on the homes, unacknowledged, until `online` |
| `pair.browser` | — | Result: `link`, `expires` (`flows/pairing.md` C) |

Levels: by the owner's decision of 2026-11-21, `default_level` defaults to 3,
and group, review and task sessions all use the same effective level: 3
whenever `all_level3` is on, else the level set by `perm.set` for `(agent,
group)` (1, 2 or 3), else `default_level` (3 when unset). Where a CLI cannot
enforce level 2 (Claude Code on native Windows), `perm.set` asking for level 2
is refused with `conflict` (`contracts/drivers.md` 1.1).

`caps`, per agent: `{"day":{"unit":"turns","max":200},"task":{"unit":
"turns","max":60}}`. `unit` is `turns` always, and `tokens`, `usd` or
`window_pct` where the CLI reports that figure; `max` may be a fraction. In
`settings.set`, `caps` has a member for each agent whose caps change,
`claude` or `codex`, holding that agent's caps whole: `day` and `task` are
optional, a cap that is not there is no longer set, and `{}` sets none. An
agent that is not there keeps its caps. `host.info` shows each agent's caps
in its entry of `agents`. A day cap is released at the next local midnight
or when the owner raises it; a task cap when the owner raises it. Reaching
one is the host event `cap_reached`.

`pair.browser`, `host.offline` and `host.online` carry `t`, `n`, `ts` and
`to` only. In `takeover.start` and `takeover.end`, `member` is the member id
of the AI in group `g`, as in the label `pty:<g>:<member>`
(`contracts/datachannel.md` section 1); `task`, when there, is a task id.

Except where a Result is named, effects are observed through `host.info`.

## 3. Inbox items

`why` says what the item is for; the wire shape and host checks are in
`contracts/datachannel.md` section 6.

| `why` | For | When |
| --- | --- | --- |
| `mention` | an AI | a message mentions it; a `call` names it |
| `assign` | assignee | a task becomes active for it, or is given to it |
| `review` | reviewer | a commit is ready for review |
| `reviewed` | assignee | a review did not pass |
| `checks` | assignee | hosted checks failed |
| `reply` | caller | its call was answered |
| `decided` | the asking AI; for a decision about a task, also its assignee | an approval or a decision was decided |
| `confirmed` | secretary | a brief was confirmed or sent back |
| `resume` | assignee | a paused task continues; a group resumes |
| `notice` | secretary | the cases of task-machine section 5, each named by its `code`: among them a hand-back summary and an AI limited |
| `merge` | the assignee's host, not a session | a review passed; the item names the commit and the pull request |

While a group is paused, items are still queued; hosts do not deliver them
until the group resumes. An item that a later change has made pointless is
deleted by the home (`contracts/task-machine.md` section 2).
