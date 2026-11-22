# Contract (step 2): the `confirm` card and the `information` label

How a home asks the owner of an AI before that AI is put to work for someone
else, and how a message that is not an order is marked for the AI. Design
points D-8 and D-9 of `steps/parallel-plan-step-2-3.md`. Rules: R-51, R-52,
R-53, R-81, R-115.

This file adds to the step 1 contracts and changes no meaning in them;
section 8 lists what is folded into which file when step 1 is built.
Presence follows `contracts/conventions.md` section 2.

Not here: the allow list, delegates and the level of a task session
(`contracts/step-2/authority.md`); the terminal channel a watcher opens
(`contracts/step-2/watch-team.md`).

## 1. One card kind, three requests

A `confirm` card is about one AI and one request; the AI's owner decides
it.

| `body.what` | The owner is asked to | The request is | Rules |
| --- | --- | --- | --- |
| `direct` | let the AI act on a message that mentions it, or take a role in a task | that message; for a role, the task's request: the brief, or the `task` message of a task without one | R-51, R-53 |
| `ask` | let the AI answer a call | the `call` message | R-115 |
| `watch` | let a person watch the AI's terminal | the card itself, which that person posts | R-81 |

## 2. When the home asks

- **The word behind a request** is the person who wrote it; for a message
  an AI wrote, that AI's owner (R-115). Behind a role in a task it is the
  person who confirmed the task's brief; for a task without a brief, the
  owner of the AI that started it.
- That word **is enough** for an AI when the person is the AI's owner, or
  when the AI's `account` is `team` or `api_key` and the person is on its
  allow list (`contracts/step-2/authority.md` section 2). For a `personal`
  account only the owner's word is enough, whatever the list says (R-51).
- An AI **takes part in** a task when it is the task's assignee or reviewer
  and an `assign` or `review` item of that task has been queued for it.

Where step 1 queues an item, or gives a role, and the word behind it is not
enough for that AI, the home does this instead:

| The home is about to | Step 2, when the word is not enough |
| --- | --- |
| queue `mention` for a message that mentions the AI, or for a main-line message of a person that goes to the secretary (`contracts/commands.md` 1.3) | writes a `direct` card; no item |
| queue `mention` for a `call` | writes an `ask` card; no item. The call stays `waiting` until the card is decided (section 5), and the caller's `ask` returns its receipt as in `flows/ask.md` B |
| do either in the thread of a task the AI takes part in | no card. The item is queued as in step 1 and is labelled `information` (`contracts/step-2/labels.md`, R-52) |
| give the AI a role in a task (`contracts/task-machine.md` rows 1 and 11) | writes a `direct` card unless the AI already has one for that request; section 7 |
| queue `reply`, `decided`, `reviewed`, `checks`, `resume`, `confirmed`, `notice` or `merge` | nothing new: these follow from what the AI asked itself or already takes part in |

- The card is written in the transaction of the command that made the
  request. One card per AI and request: two such AIs, two cards.
- No switch confirms for the owner (R-51). A secretary on a personal
  subscription thus needs a card for every main-line request of another
  person; the client warns about it (D-6).

## 3. Kind and body

One row for the table of `contracts/messages.md` section 1:

| `kind` | Where | Written by | `body` | `state` |
| --- | --- | --- | --- | --- |
| `confirm` | main, or thread + main | the home; for `watch`, the person who asks | below | section 4 |

```json
{"what":"direct","ai":"<member id>","to":["<user id>"]}
```

| Field | Value |
| --- | --- |
| `what` | `direct`, `ask`, `watch` |
| `ai` | the AI member the request is for |
| `to` | a list with exactly one user id, the owner of `ai`. A list, so that a client finds the cards that wait for a person by one rule for `approvers` and `to` |

The body has no optional field. The rest is in the message row
(`contracts/rows.md` section 5):

| Row field | `direct`, `ask` | `watch` |
| --- | --- | --- |
| `by` | `"system"` | the person who asks |
| `env` | absent | that person's `post` |
| `re` | the request | `null` |
| `thread` | that of the request; `null` for a brief | `null` |
| `main` | `true` | `true` |
| `text` | `""` | `""` |
| `mentions` | `[]`: `body.to` names who is asked | `[]` |

Who asked is not in the body: it is the author of the request; for a role,
the person in the brief's `state.by`, or the owner of the AI that started
a task without a brief.

`watch`, one row for the `post` table of `contracts/commands.md` 1.3:

| `kind` | By | Extra checks | Effect beyond storing the row |
| --- | --- | --- | --- |
| `confirm` | a person | `body.what` is `watch`; `body.ai` is an AI member whose owner is another person; `mentions` is `[]` | `state.status = pending`; `main = true` |

Sent by the person: `what` and `ai`. Added by the home: `to`.

## 4. States

For `contracts/messages.md` section 3:

| `status` | Other keys |
| --- | --- |
| `pending` | none |
| `confirmed`, `denied` | `by`, `at`, `act` |
| `expired` | `at` |

A pending card becomes `expired`, by the home, when what it asks for can no
longer happen:

- the AI, or the person who posted a `watch` card, is no longer a member;
- for a card written for a role: no task from the request that is not final
  still names the AI;
- retention has removed the request (`contracts/storage.md` section 4). The
  card itself is kept while it is pending, like every card.

## 5. `act confirm` and `act deny`

Two rows for the table of `contracts/commands.md` 1.4. No new action name:
`confirm` is the action of a brief, `deny` that of an approval.

| Card | `action` | By | Effect |
| --- | --- | --- | --- |
| `confirm` | `confirm` | the person in `body.to` | `state.status = confirmed`; then the table below |
| `confirm` | `deny` | the same | `state.status = denied`; then the table below |

```json
{"t":"act","n":"…","ts":1763700000000,"g":"Qm1…","msg":"<card id>",
 "action":"confirm"}
```

- Only the owner decides (R-51): no admin, no approval delegate (R-42).
- `arg` and `text` are not used. A card that is not `pending` answers
  `conflict`.

| `what` | Confirmed | Denied |
| --- | --- | --- |
| `direct` | the home queues what the request gives this AI: the `mention`, when the request mentions the AI or is a main-line message that goes to it as secretary; and the items of section 7, when a task from the request names it | section 7 for a role. `notice` for the secretary when the request was written by an AI or gave a role |
| `ask` | `mention` for the callee with the call as `msg`. From there `flows/ask.md` step 4 | the home answers the call with the denial: the call's `state` becomes `denied` with `at`, and a `reply` item for the caller carries the card as `msg`. The caller's open `ask` returns `{"denied":true}`; after the wait the denial is its turn. `notice` for the secretary |
| `watch` | one item `watch` for the AI's host, with the card as `msg`; the host keeps the grant (`contracts/step-2/watch-team.md` section 2) | nothing |

- A confirmation delivers its message or call once; for a role it holds
  for the tasks of that request.

**Reminders.** The card waits for a person (R-34): `push`
(`contracts/commands.md` 1.7) gains the value `confirm` in `on` and in
`kind`. `who` is the display name of `body.ai`; `title` is absent.

## 6. Roles in tasks

One rule beside those of `contracts/task-machine.md` section 2, **a role
needs its owner's word**:

- An `assign` or `review` item for an AI is queued when the word behind the
  role is enough for the AI and the brief gives the AI that role, or the
  exception below holds, or the AI's `direct` card for that request is
  confirmed. Otherwise the item is not queued.
- A brief gives a role to the AIs it names, each in the role it names. A
  `task assign` that gives a role to another AI asks that AI's owner by a
  card, whoever confirmed the brief. One exception: the owner of that AI
  sent the `task assign`, as a person or through one of their AIs.
- Row 2 asks nothing: the task's AI started it through its own host. Its
  `assign` is queued as in step 1.
- The home writes the missing card when the role is given: at row 1 for
  every AI the brief names, tasks that start `queued` included; at row 11
  for the AI newly named.
- While the item of an `active` task is held back and the card is
  `pending`, the task's wait is `confirm`, with the card in `wait.msg` and
  `body.to` in `wait.who`: for the assignee at every step, for the reviewer
  at step `review`. `wait.kind` gains this value for `active` tasks.
- "Wait cleared" (`contracts/task-machine.md` section 2) recomputes in this
  order: such a pending `confirm` card, else the oldest pending approval
  card, else the pending decision card, else `null`.
- Confirmed: each `active` task from the request gets the item for its
  current step and the AI's role, as on activation. Tasks that become
  active later, and later reviews, get theirs too.
- Denied: no item, now or later; the wait is recomputed; the secretary gets
  a `notice` and reassigns or cancels. Naming another AI is row 11.
- **A request is kept** while a task from it is not final: the exemption of
  `contracts/storage.md` section 4 for a confirmed brief with a queued task
  covers the brief and the `task` message of every such task.

## 7. The `information` label

In `contracts/step-2/labels.md`, split from this file for its length: the
label of an item, the label of a message or a decision it carries, where a
turn shows each, and the sentence every session is given.

## 8. What grows in the step 1 files

Folded in when step 1 is built; until then this file is the reference.

| File | Addition |
| --- | --- |
| `contracts/messages.md` | kind `confirm` (section 3), its states (section 4); the call status `denied`, with `at` (section 5) |
| `contracts/commands.md` | 1.3: a card in place of a `mention` item when the word behind it is not enough (section 2); `post` kind `confirm`; `act confirm` and `deny` on the card (section 5); in `push`, the value `confirm`; `notice` after a denial |
| `contracts/task-machine.md`, `contracts/rows.md` | `wait.kind` `confirm`; the rule of section 6 |
| `contracts/drivers.md` | what `contracts/step-2/labels.md` section 5 lists |
| `contracts/mcp-tools.md` | section 1: `ask` may return `{"denied":true}` (section 5) |
| `contracts/datachannel.md` | section 6: the item `watch` |

Unit P-4a of the parallel plan freezes the shapes: one golden message row
per status, `row-msg-confirm-<status>.json`, `row-msg-call-denied.json`,
and the `act` on the card.

## 9. Settled by the coordinator (2026-11-20)

1. A person not on the allow list of a team or API-key AI gets the same
   card as for a personal AI (section 2), not a refusal.
2. A change of direction after a task started: no card for a message in
   the thread; a mention outside it is a new card
   (`contracts/step-2/labels.md` section 4).
3. Only a message a person wrote is `instruction` (section 3 there).
4. A listed person's message in the thread of a running task is
   `information`: R-52 governs there, R-53 outside it
   (`contracts/step-2/labels.md` section 3).
5. Every task has a request (the brief, or its `task` message), so its
   owner can be asked about any task.

## 10. Not verified

Nothing here was run.
