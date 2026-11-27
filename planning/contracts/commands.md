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
`contracts/datachannel.md` section 5 puts before the receipt: it passed the
envelope checks and, as a home command, has its `g`. Whatever the receiver answers after that, a refusal
included, is the outcome its receipt keeps; a command that one of those
checks refused leaves no receipt. The answer to a repeated refusal has the
same code. Its `msg` is not kept, and need not be the same.

Three results name something that lives only in memory: the link of
`pair.browser`, the link of `invite` (`contracts/step-2/people.md` 2.1)
and the terminal of `takeover.start`. Their receipts record that the
command succeeded, not the link or the terminal. A repeat of
`pair.browser` or `invite` is answered `ok` with the original link (and
expiry for `invite`) while that link is still open, and `takeover.start`
is answered `ok` while that terminal is still open on this connection;
otherwise `conflict`. A repeat never creates a new link or a new terminal.

The code of `code new` and `code show` is stored by the home and is in no
receipt: a repeat is answered `ok` with no field
(`contracts/step-2/people.md` 2.6).

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
- in `push`: either `sub`, `vapid` and `on`, or `off` alone (1.7);
- in `takeover.start`: `task`;
- in `note_edit`: `name`, there exactly when the note has a `type`;
- in `code`: `hours`, there with `op` `new` alone;
- in `member add_ai`, `host.join` and as one of the "any of" of `member set`: `follow`, which is
  only ever `true` (`contracts/step-3/profiles.md` section 1a).

A `post` always carries `kind`, `text`, `thread` (or `null`), `main`,
`mentions`, `re` (or `null`) and `body`; what `body` must hold for each
kind is in `contracts/messages.md`. In `task start`, `re` is the message
that led to the task, or `null`; in `note`, `task` is a task id or `null`.

## 1. Home commands

### 1.1 `group`

| `op` | Fields | By | Effect |
| --- | --- | --- | --- |
| `create` | `name`, `member_name` | a person: a device of the receiving node's owner | the node becomes home of group `g` = `n`: the payload carries `g` like every home command, and a `create` whose `g` is not its `n` is `bad_request`; group row with `pass` 90, `pass_foundation` 70 (R-101) and the `limits` of the example in `contracts/rows.md` section 1; a `member` row for that person with role `owner` and `member_name` as its `name`; a `groups` push to that person's connected devices |
| `set` | any of `name`, `pass`, `pass_foundation`, `limits`, `secretary` | an admin | group row updated. `limits`, when sent, is the whole object of `contracts/rows.md` section 1. `secretary` must name an AI member that is not assignee or reviewer of a task that is not final, else `conflict`; the previous secretary's member row loses the flag |
| `pause` | — | an admin | `group.paused` set; `system` `group_paused`; hosts interrupt running turns of the group and hold items. A `pause` of a paused group is accepted and changes nothing |
| `resume` | — | an admin | `group.paused` cleared; `system` `group_resumed`; task-machine row 24. A `resume` of a group that is not paused is accepted and changes nothing |

A group has at most one secretary (R-20), and needs none to exist: a group
may hold people only. While it has none, main-line messages that mention
nobody reach no AI and no brief can be posted.

**Creating a group with people and AIs** (R-10) is not one command, and
nothing in it is atomic. The client sends `group create`; then one
`member add_person` per person chosen (1.2); then one `host.join` per AI
chosen, each to the computer of that AI, with its folder (section 2); then
`code new` (`contracts/step-2/people.md` 2.6) when an invitation code was
asked for. The first step that fails stops the rest and is shown beside its
person or AI, or under the code; the group exists, and the same commands
add them again later. Adding a person or an AI to an existing group is
those same commands.

### 1.2 `member`

| `op` | Fields | By | Effect |
| --- | --- | --- | --- |
| `add_ai` | `id`, `cli` (one of `[a-z0-9_]+`, an agent the host runs), `name`, `workspace`, `account`, `secretary`, `approvers`, `profile`, `follow`, `tool` | a host whose owner is a member | member row with `host` = the signer and `workspace` as sent: the absolute path of the AI's folder on that host, which the host checked and resolved (`host.join`, section 2). The home stores it and never changes it. `name` must be unused in the group, else `conflict`. `secretary: true` is accepted when the group has none or the owner is an admin, and sets `group.secretary`; a previous secretary's member row loses the flag |
| `add_person` | `user`, `name` | an admin | a person who is already a member of another group homed on this node becomes a member of this one, with no invitation (`contracts/step-2/people.md` section 2.5) |
| `status` | `id`, `state`, `task`, `path`, `usage`, `event` | the member's host | member row updated; `event`, when present, becomes a `system` row (1.9) |
| `set` | `id`, any of `name`, `account`, `allow`, `approvers`, `profile`, `follow`, `tool` | the AI's owner; for `approvers` alone, also the AI's host; for `follow` with the four profile fields, also the AI's host (`contracts/step-3/profiles.md` section 2) | member row updated. A new `name` must be unused in the group, else `conflict`. On a person's row, `set` takes `role` alone, by the owner (`contracts/step-2/people.md` section 2.3) |
| `remove` | `id` | the AI's owner, or an admin; for a person's row, `contracts/step-2/people.md` section 2.4 | refused with `conflict` while the AI is assignee or reviewer of a task that is not final: reassign or cancel first. Otherwise the member row is replaced by a `gone` marker. Removing the secretary clears `group.secretary` and writes `system` `no_secretary` |

On `add_ai` and on `set` with `approvers`, the home adds the owner's user id
to `approvers` when it is missing (`contracts/rows.md` section 2); the owner
is the person whose host signed `add_ai`.

Step 2 adds people: `invite`, `join`, `code` and `add_person`
(`contracts/step-2/people.md` section 2), and `set` and `remove` on a
person's row. `member set` never takes `workspace`: to move an AI to another
folder its owner removes it and adds it again.

Step 3 adds the member's profile to `add_ai` and `set`, and `follow`
(`contracts/step-3/profiles.md` sections 1a and 2), and `tool` for an external AI
(`contracts/step-3/external.md` section 2).

### 1.3 `post`

Fields: `kind`, `text`, `thread`, `main`, `mentions`, `re`, `body`, `as`.
The message id is `n`.

| `kind` | By | Extra checks | Effect beyond storing the row |
| --- | --- | --- | --- |
| `text` | a person, or an AI | — | item `mention` for each mentioned AI; a main-line message from a person that mentions no AI produces one for the secretary |
| `progress`, `question` | an AI | `progress` only in a thread | item `mention` for each mentioned AI |
| `report` | an AI | `thread` is a task whose assignee is `as`; task-machine row 5 | the submission of the task: task-machine row 5 |
| `confirm` | a person, for `body.what` `watch`; otherwise the home alone | `contracts/step-2/confirm.md` section 3 | `state.status = pending`; `main = true` |
| `brief` | the secretary | `body.requester` is a member; every assignee and reviewer is a working AI of the group, the two differ, and `depends` names only earlier entries of the same brief; `body.replaces`, when not `null`, is a brief whose `state.status` is `pending` or `revise`, else `conflict` | `state.status = pending`; the brief named in `body.replaces` becomes `superseded` |
| `summary` | the secretary | `thread` is a `done` task | the home adds the whole `body` from the task row; `main = true` |
| `approval` | an AI | `thread` = `body.task`, or `null` when `body.task` is `""`; `as` = `body.ai`; for a task, `as` is its assignee or reviewer | `state.status = pending`; `main = true`; with a task, task-machine row 17 |
| `decision` | an AI | `thread` = `body.task`, or `null` when the body names no task | `state.status = pending`; `main = true`; task-machine row 19 |
| `call` | an AI | the callee is another AI member; `body.depth` ≤ `limits.ask_depth` | `state.status = waiting`; item `mention` for the callee |
| `reply` | an AI | `re` is a `call` addressed to `as` whose `state.status` is still `waiting`, else `conflict`: a call is answered once | the call's `state` becomes `answered` with `reply` and `ms`; item `reply` for the caller |
| `review` | an AI | `thread` = `body.task`; task-machine row 7, which refuses a review whose `body.report` is not the task's `report` | Result: `passed`, `round` |
| `handback` | an AI | in a thread, or in the main line for a session without a task | item `notice` (`code: handback`) for the secretary when posted in a thread |

Rules for every `post`:

- A member named twice in `mentions` is `bad_request`, and a mentioned id
  that is no member of the group is `conflict`.
- A post never queues an item for its own author.
- The wake-up cap (R-60): if an AI has been sent `limits.wakes_per_hour`
  items in the last hour, a new item for it is held until the hour allows
  it. An item queued by a person's command, that is, a command of a device
  that hosts no AI of the group, is neither held nor counted, and no more are
  `reply` and `watch` items. The count is kept in memory by the home
  and starts again when the home restarts.

Rules for every `post` by an AI:

- A message in a thread that mentions an AI counts a round (task-machine
  rows 21 and 22).
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
| `confirm` | `confirm`, `deny` | the person in `body.to` | `contracts/step-2/confirm.md` section 5 |

On a decision with options, `choose` takes `arg` (the `id` of one of
`body.options`), `text`, or both, and `text` alone is also accepted on a
card with options (2026-11-21 owner decision); on a decision whose `options`
is empty it takes `text` and no `arg`. `revise` may have `text`. `confirm`,
`approve` and `deny` have neither. Anything else is `bad_request`.

Acting on a card whose `state.status` is not `pending` returns `conflict`.
A decided card keeps the signed `act` in `state.act`.

`jump` (R-72), fields: `msg`. By the person who wrote message `msg`. Every
unacknowledged `mention` item that `msg` produced gets `jump: true` under
its number, is no longer held back by the wake-up cap, and is sent again;
when there is none, `conflict`.
The host does the rest (`flows/agent-turn.md` C).

### 1.5 `task`

When each is allowed, and what it does, is `contracts/task-machine.md`.

| `op` | Fields | By | Result |
| --- | --- | --- | --- |
| `start` | `id`, `title`, `plan`, `re`, `origin` (`direct`: the one value; anything else is `bad_request`) | a working AI, not the secretary | `num` |
| `fail` | `id`, `reason` (`crash`, `timeout`), `detail` | host of the assignee | — |
| `preempted` | `id`, `msg` (the message whose jump interrupted the task) | host of the assignee | — |
| `assign` | `id`, `assignee`, `reviewer` | the secretary, or an admin; for a `failed` task an admin only | — |
| `pause` | `id` | the secretary, or an admin | — |
| `resume` | `id` | the secretary, or an admin (for a `failed` task, or one paused by the round limit, an admin only); the host of the assignee, for its own pre-empted task | — |
| `cancel` | `id` | the secretary, or an admin | — |

A host reports two facts about a task, `fail` and `preempted`, and resumes
what it pre-empted. It reports nothing about a branch, a check, a pull
request or a merge: the node runs none. A submission is the assignee's
`post` of kind `report` (1.3), and a task is done when its review passes
(task-machine rows 5 and 7).

### 1.6 `note` and `forget`

| `t` | Fields | By | Effect |
| --- | --- | --- | --- |
| `note` | `id`, `task`, `text` | a person, or an AI | note row written |
| `note_edit` | `id`, `name`, `text` | a person | the live note `id` gets `text`, and `name` when it has a `type`; `by` becomes the sender and `ts` the command's. Its `id`, `pos`, `task` and `type` stay, and it gets a new `seq`. A note that does not exist or was forgotten is `not_found`; one signed for an AI, `forbidden`. A `name` on a context note, or on a typed note none or one outside the rule of `contracts/step-3/knowledge.md` section 1, is `bad_request`; a name that another note of the same `type` has is `conflict` |
| `forget` | `r` (`note` or `msg`), `id` | an admin | a note, or a `review` message, is replaced by a `gone` marker; any other message is refused with `conflict` |

`note_edit` is how a person corrects an entry of the project memory, a
document of the knowledge base or a preset where it is shown: by its `id`,
so an entry forgotten meanwhile is not written again. No history: the old
text is gone.

Notes and `review` messages are not removed by retention (R-102). `forget`
is how a person deletes one by hand, and it deletes nothing else: R-102
names only the project memory; every other message leaves by retention.

### 1.7 `push`

By a browser, for itself: `{"sub":{"endpoint":"…","p256dh":"…","auth":"…"},
"vapid":{"pub":"…","priv":"…"},"on":["mention","approval","decision",
"brief","done","confirm"]}`, or `{"off":true}`. A `push` whose `off` is `true` is the
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
{"g":"<group id>","msg":"<what a click opens>","group":"Website",
 "kind":"approval","who":"Codex","title":"Install the browser test dependencies"}
```

| Field | Content |
| --- | --- |
| `msg` | the id of the card or message; for a message that is in a task's thread alone, the task's id: that of the task's message in the main line, which opens the thread |
| `kind` | `approval`, `decision`, `brief`, `mention`, `done`, `confirm` |
| `who` | display name of the member it is about: the AI asking, the secretary, the author of the mention; for a `system` row that mentions people, the AI its body names, or the assignee of the task its body names |
| `title` | one line, at most 80 characters. `approval`: the first line of the justification the CLI attached (`body.why`), absent if there is none. `decision`: the question. `brief`, `done`: the brief or task title. `mention`: the task title for a `system` row about a task, else absent |
| `code` | for a `system` row its `code`, such as `task_failed` or `login_invalid`, else absent |

`title` and `code` are optional; every other field is always present.

Never included: message text, the command of an approval, code, file
names. The browser shows `group · kind · who: title` in the user's language,
with words for `code` in place of `kind` when it has some, and opens `msg`.

Each created message row produces at most one reminder kind: `approval`
(`body.approvers`), `decision` (`body.to`), `brief` (human `body.requester`)
and `summary` (`done`, every human member of the group) do not also send
`mention` for their `mentions`; any other created message row sends `mention`
to the people named in `mentions`. `title` is cut at the first `\r` or `\n` and
then at 80 Unicode code points, and left out when empty. A `404` or `410` from
the push service drops that subscription.

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

The row of `login_invalid` mentions the AI's owner (R-56). For
`login_invalid`, `cap_reached` and `rate_limited` the home also gives the
secretary a `notice`. `cap_reached` is postponed after 1.0 with
`settings.set`, which alone sets caps: a 1.0 host never sends it, and a 1.0
home refuses it as an unknown code. A closed host connection sets `state = offline`
for its AIs. Nothing else is derived from a status.

## 2. Host commands

Sent by the owner's browser to one of the owner's nodes. The node accepts
them only from a device in its owner's device log, with `to` equal to its own
device id. Receipts apply as for home commands.

| `t` | Fields | Effect |
| --- | --- | --- |
| `agent.set` | `agent`, `account` | declares the account type (`personal`, `team` or `api_key`, else `bad_request`) of a CLI found on this machine (one of the agents of `contracts/step-3/drivers.md`, else `bad_request`; `conflict` when not found here; R-50); sets `login_ok` back to `true` in `host.info` (`flows/agent-turn.md` E) and writes audit `setting` (`account <account>`); required before the AI can join a group |
| `agent.defaults` | `agent`, `model`, `effort`, `tier`, `skills` | sets the defaults of that CLI on this node (`contracts/step-3/profiles.md` section 1a). `agent` is one of the agents of `contracts/step-3/drivers.md`, else `bad_request` (`external` included); the four are all sent and within the bounds of `profiles.md` section 1, with `tier` one of the three values, else `bad_request`. The node keeps them in its setting `defaults`, shows them in `host.info`, and brings every member of that CLI here that follows them up to date. A CLI that is not found here may have defaults: they show once it is found |
| `perm.set` | `agent`, `g`, `level` (1, 2, 3) | postponed after 1.0 (no level control in 1.0): permission level for that AI's sessions in that group (R-40); takes effect the next time a session's process starts |
| `settings.set` | any of `retention_days`, `idle_minutes`, `max_open`, `default_level`, `all_level3`, `approvers`, `caps` | postponed after 1.0 (with `U-12`): node settings; for `approvers` the node sends `member set` with them for each of its AIs to that AI's home |
| `host.join` | `g`, `home`, `agent`, `name`, `secretary`, `workspace`, `follow` | adds that AI to the group with `workspace` as its folder (R-11; the rules are below the table): the node checks and resolves the folder, connects to `home`, sends `member add_ai` with the resolved path, and records the folder with the AI's group session. It clones nothing and creates no folder |
| `host.leave` | `g`, `agent`, `member` (needed when the node has several members of that CLI in `g`) | the node sends `member remove`; if the home accepts it, the AI's sessions in that group are closed (it is refused while the AI has unfinished tasks) |
| `home.add` | `g`, `home`, `name`, `addrs` | records the home of a group on this node before any AI of its owner joins it (`contracts/step-2/people.md` section 7, point 2) |
| `watch.end` | `g`, `member`, `user` | drops a person's watch grant (`contracts/step-2/watch-team.md` section 5) |
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
one is the host event `cap_reached`. Caps and `cap_reached` are postponed
after 1.0 with `settings.set`.

**`host.join` and the folder.** `workspace` is a path on this node, from
the list of `host.workspaces` (`contracts/datachannel.md` section 7) or
typed by the person. The node answers, in this order:

| Code | When |
| --- | --- |
| `bad_request` | `workspace` is not an absolute path |
| `not_found` | no folder is there now |
| `conflict` | the folder, with its symbolic links resolved, is the workspace of an AI of **another** group on this node, whichever home that group has; `msg` names that group. A folder of an AI of this same group is accepted |

Then it sends `member add_ai`, and an error of the home is the answer. The
check for another group and the writing of the group session's row happen
under one lock, and joins on one node run one at a time, so two joins for
one folder and two groups cannot both pass. After the AI has left its group
(`host.leave`), the folder is free again (`contracts/local.md` section 7).

`pair.browser`, `host.offline` and `host.online` carry `t`, `n`, `ts` and
`to` only. In `takeover.start` and `takeover.end`, `member` is the member id
of the AI in group `g`, as in the label `pty:<g>:<member>`
(`contracts/datachannel.md` section 1); `task`, when there, is a task id.

Except where a Result is named, effects are observed through `host.info`.

Step 3: `host.join` with `agent` `external` adds an agent that the node
does not start; it carries `account` and may carry `tool`, and needs no
`agent.set` (`contracts/step-3/external.md` section 2). Its `workspace` is
judged as for every AI.

## 3. Inbox items

`why` says what the item is for; the wire shape and host checks are in
`contracts/datachannel.md` section 6.

| `why` | For | When |
| --- | --- | --- |
| `mention` | an AI | a message mentions it; a `call` names it |
| `assign` | assignee | a task becomes active for it, or is given to it |
| `review` | reviewer | a submission is ready for review; the item carries its `report` message |
| `reviewed` | assignee | a review did not pass |
| `reply` | caller | its call was answered |
| `decided` | the asking AI; for a decision about a task, also its assignee | an approval or a decision was decided |
| `confirmed` | secretary | a brief was confirmed or sent back |
| `resume` | assignee | a paused task continues; a group resumes |
| `notice` | secretary | the cases of task-machine section 5, each named by its `code`: among them a task that is done, a hand-back account and an AI limited |

While a group is paused, items are still queued; hosts do not deliver them
until the group resumes. An item that a later change has made pointless is
deleted by the home (`contracts/task-machine.md` section 2).
