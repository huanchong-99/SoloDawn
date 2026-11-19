# Contract: commands

A command is the payload of a signed envelope (`contracts/envelope.md`) sent
in a `cmd` message (`contracts/datachannel.md` section 5). This file lists
every command, who may send it, and what the receiver does.

Every payload has `t`, `n`, `ts`. Home commands have `g`. Host commands have
`to`. When a node signs for one of its AIs the payload has `as`, the AI's
member id.

"Signer" below means the person whose device log contains the signing
device, unless a device is named.

## 1. Home commands

### 1.1 `group`

| `op` | Fields | Signer | Effect |
| --- | --- | --- | --- |
| `create` | `name`, `kind`, `repo`, `base`, `forge`, `checks` | a device of the receiving node's owner | the node becomes home of group `g` = `n`; group row; a `member` row for the signer's person with role `owner` |
| `set` | any of `name`, `checks`, `pass`, `pass_foundation`, `limits`, `secretary` | role `owner` or `admin` | group row updated |
| `pause` | — | role `owner` or `admin` | `group.paused` set; a `system` message; hosts interrupt running turns of the group and hold deliveries |
| `resume` | — | role `owner` or `admin` | `group.paused` cleared; an inbox item `resume` for the assignee of every active task |

### 1.2 `member`

| `op` | Fields | Signer | Effect |
| --- | --- | --- | --- |
| `add_ai` | `id`, `cli`, `name`, `account`, `secretary` | a node of a person who is a member; that node becomes the AI's `host` | member row; `secretary: true` is accepted when the group has no secretary or the person is `owner`/`admin`, and sets `group.secretary` |
| `status` | `id`, `state`, `task`, `path`, `usage` | the member's `host` | member row updated; see 1.9 for what the home derives |
| `set` | `id`, any of `name`, `account`, `allow`, `approvers` | the AI's owner | member row updated |
| `remove` | `id` | the AI's owner, or role `owner`/`admin` | member row removed; its active tasks become `queued` with `wait.kind = busy` and the secretary gets an inbox item |

Adding people is step 2.

### 1.3 `post`

Fields: `kind`, `text`, `thread`, `main`, `mentions`, `re`, `body`, `as`.
The message id is `n`.

| `kind` | Signer | Extra checks | Effect beyond storing the row |
| --- | --- | --- | --- |
| `text` | any member | — | inbox item `mention` for each mentioned AI; a main-line message from a person that mentions no AI produces one for the secretary |
| `progress`, `question`, `report` | an AI's host | `thread` is a task of the group | inbox item `mention` for each mentioned AI |
| `brief` | the secretary's host | `body.requester` is a member | `state.status = pending` |
| `summary` | the secretary's host | the task is `merged` | home fills `score`, `rounds`, `minutes`, `added`, `removed`, `pr` in `body` from the task row |
| `approval` | an AI's host | `as` is the task's assignee or reviewer | `state.status = pending`; `task.wait = approval`; `main = true` |
| `decision` | an AI's host | — | `state.status = pending`; if `body.task`, `task.wait = decision` |
| `call` | an AI's host | callee is an AI member; `body.depth` ≤ `limits.ask_depth` | `state.status = waiting`; inbox item `mention` for the callee |
| `reply` | an AI's host | `re` is a `call` addressed to `as` | call `state` becomes `answered` with `reply` and `ms`; inbox item `reply` for the caller (its host returns it from the open tool call or delivers it as a turn, `flows/ask.md`) |
| `review` | the reviewer's host | task `step = review`, `as` is `task.reviewer` | see 1.8 |
| `handback` | an AI's host | — | inbox item `notice` for the secretary |

Rules applied to every `post` by an AI in a thread (R-60):

- If the message mentions an AI, `thread.rounds` increases by one. A `text`
  message by a person in the thread sets it to zero.
- When `thread.rounds` reaches `limits.rounds`: the task becomes `paused`
  with `wait.kind = rounds`; a `system` message `rounds_limit` is written in
  the main line mentioning the brief's requester; the secretary gets an
  inbox item. Inbox items for that thread are held until a person posts in
  the thread or the task is resumed.
- If an AI already received `limits.wakes_per_hour` inbox items in the last
  hour, a new item is held until the window allows it.

### 1.4 `act`

Fields: `msg` (the card), `action`, `arg`, `text`.

| Card kind | `action` | Signer | Effect |
| --- | --- | --- | --- |
| `brief` | `confirm` | `body.requester`, or role `owner`/`admin` | `state.status = confirmed`; one task row and one `task` message per entry of `body.tasks`; tasks with unmet `depends` are `queued`, the others `active`; inbox item `assign` per active task; inbox item `confirmed` for the secretary |
| `brief` | `revise` | same | `state.status = revise`, `state.note = text`; inbox item for the secretary |
| `approval` | `approve`, `deny` | a person in `body.approvers` | `state` updated; `task.wait` cleared; inbox item `decided` for the AI, carrying this envelope |
| `decision` | `choose` | a person in `body.to`, or role `owner`/`admin` | `state.status = decided` with `choice = arg` and `text`; `task.wait` cleared; inbox item `decided` for the author and for the task's assignee |
| `text` (the signer's own message, mentioning an AI that is `working`) | `jump` | the author | the AI's pending item for that message is sent again with `jump: true`; see 1.9 |

Acting on a card whose `state.status` is not `pending` returns `conflict`.

### 1.5 `task`

| `op` | Fields | Signer | Effect |
| --- | --- | --- | --- |
| `start` | `id`, `title`, `as` | an AI's host | direct work (R-71): task row with `origin = direct`, `state = active`, `step = branch`; a `task` message; inbox item `notice` for the secretary |
| `branch` | `id`, `branch` | assignee's host | `step = local_check` |
| `checked` | `id`, `ok`, `items`, `commit` | assignee's host | `checks.local` set; a `checks` message in the thread |
| `submitted` | `id`, `pr`, `pr_url`, `stats` | assignee's host | accepted only if `checks.local.ok` and `checks.local.commit` equals the submitted head; `step = cloud_check` (forge `github`) or `review` (forge `none`, with an inbox item `review` for the reviewer) |
| `cloud` | `id`, `ok`, `items` | assignee's host | `checks.cloud` set; a `checks` message; if ok, `step = review` and an inbox item `review` for the reviewer; if not, as `fail` |
| `fail` | `id`, `reason` (`cloud`, `crash`, `timeout`, `conflict`), `detail` | assignee's host | `failures` + 1; inbox item `checks` for the assignee with the detail; reason `conflict` also sets `step = local_check` and gives the secretary a `notice`; at `limits.failures` the task becomes `failed`, with a `system` message `task_failed` mentioning the requester and the AI's owner |
| `assign` | `id`, `assignee`, `reviewer` | the secretary's host, or role `owner`/`admin` | fields updated; a pending approval card of the task gets `state.status = reassigned`; inbox item `assign` |
| `pause`, `resume`, `cancel` | `id` | the secretary's host, or role `owner`/`admin` | state updated; `resume` creates an inbox item `resume` for the assignee |
| `merge` | `id` | the secretary's host | accepted only if `step = merge`; inbox item `merge` for the assignee's host |
| `merged` | `id`, `commit`, `stats` | assignee's host | `state = merged`; dependants that become unblocked turn `active` with an inbox item `assign`; inbox item `notice` for the secretary |

`failures` returns to zero whenever the task advances a step. A task that
reaches `review` without a reviewer gets `wait.kind = reviewer` and the
secretary a `notice`; `assign` with a reviewer clears it.

### 1.6 `note`

| `op` | Fields | Signer | Effect |
| --- | --- | --- | --- |
| `put` | `id`, `task`, `text` | any member (a host for its AI) | note row written |
| `del` | `id` | the note's author, or role `owner`/`admin` | note row removed |

### 1.7 `device` and `push`

- `device`: a device-log entry (`contracts/envelope.md` section 4). No `g`.
  A home accepts it for people in its groups and writes a `device` row in
  each of those groups. A revocation closes the revoked device's connection.
- `push` (a browser, for itself): `{"sub":{"endpoint":"…","p256dh":"…",
  "auth":"…"},"vapid":{"pub":"…","priv":"…"},"on":["mention","approval",
  "decision","done"]}`, or `{"off":true}`. The browser creates the VAPID key
  pair and hands it to each home it wants reminders from; the home stores it
  and sends an encrypted reminder when an event in `on` concerns that
  person and none of their devices is connected. See `risks.md` Q-5.

### 1.8 Review outcome

On `post` with `kind = review` the home computes `ok`:
`score` ≥ `task.pass` and no item has `veto`. A veto sets `score` to 0.

- ok: `task.step = merge`; inbox item `review` for the secretary.
- not ok: `task.review.round` + 1 and `step = local_check`; inbox item
  `review` for the assignee. When the round exceeds `limits.review_rounds`
  the task becomes `failed` with `task_failed` and reason `review_rounds`.

### 1.9 What the home derives from `member status`

- A host connection closing sets `state = offline` for its AIs.
- `state` changing from `working` to `waiting`:
  if the AI has a task paused with `wait.kind = preempted`, that task
  becomes `active` and gets an inbox item `resume`.
- An item marked `jump` (1.4): the home pauses the AI's current task with
  `wait.kind = preempted`, writes `system` `preempted`, and the host
  interrupts the running turn before delivering the item.
- `state = limited`: the home writes `system` `cap_reached` or
  `rate_limited` once per occurrence and gives the secretary an inbox item.

## 2. Host commands

Sent by the owner's browser to one of the owner's nodes. The node accepts
them only from a device in its owner's device log, with `to` equal to its own
device id.

| `t` | Fields | Effect |
| --- | --- | --- |
| `agent.set` | `agent`, `account` | declares the account type of a CLI found on this machine (R-50); required before the AI can join a group |
| `perm.set` | `agent`, `g` (a group id or `"*"`), `level` (1, 2, 3), `repo_config` (bool) | permission level for that AI in that project (R-40), and whether repository-supplied AI configuration is loaded (R-45); takes effect at the next session start |
| `settings.set` | any of `retention_days`, `idle_minutes`, `max_open`, `default_level`, `approvers`, `caps` | node settings; `approvers` is copied to the member rows of this node's AIs |
| `host.join` | `g`, `home`, `agent`, `name`, `secretary` | the node connects to `home`, clones the project (R-11), and sends `member add_ai` |
| `host.leave` | `g`, `agent` | the session is closed; the node sends `member remove` |
| `takeover.start` | `g`, `member` | `flows/takeover.md`; refused with `relay` or `busy` |
| `takeover.end` | `g`, `member` | hand-back |
| `host.offline`, `host.online` | — | R-64: every session is interrupted and closed, every home is told `offline`, and deliveries are refused until `host.online` |
| `pair.browser` | — | the response carries `link` and `expires` (`flows/pairing.md` C) |
| `device` | a device-log entry | stored if accepted; forwarded to connected homes |

Except for `pair.browser`, results are observed through `host.info`.

## 3. Inbox items: one table

| Trigger | Item for | `why` |
| --- | --- | --- |
| a message mentions an AI | that AI | `mention` |
| a task becomes active for its assignee | assignee | `assign` |
| a task reaches `review` | reviewer | `review` |
| a review is posted | secretary (ok) or assignee (not ok) | `review` |
| a call is answered | caller | `reply` |
| an approval or decision is decided | the asking AI; the assignee | `decided` |
| a brief is confirmed or sent back | secretary | `confirmed` |
| checks failed | assignee | `checks` |
| a paused or pre-empted task continues | assignee | `resume` |
| the secretary asked for a merge | assignee's host (not the session) | `merge` |
| a direct task started, a task merged, a hand-back summary posted, an AI limited or removed | secretary | `notice` |

While a group is paused, items are still queued; hosts do not deliver them
until the group resumes.
