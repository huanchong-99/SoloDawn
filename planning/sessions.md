# Sessions

Which CLI sessions exist, where each one works, what it may do, and the
states it goes through. Rules: R-12, R-70 to R-72, R-120 to R-124; settled
by the coordinator in `risks.md` Q-8 and Q-15. Delivery of items to sessions
is in `flows/agent-turn.md`; CLI flags are in `contracts/drivers.md`,
folders in `contracts/local.md`.

## 1. Kinds

An AI in a group has several sessions over time:

| Session | Exists | Working folder | Level | Receives |
| --- | --- | --- | --- | --- |
| group session | one per AI per group, long-lived. For the secretary this is its only session. | the project clone | the level the owner set for this AI in this project (default 3; owner's decision of 2026-11-21) | main-line mentions, direct questions (R-70), calls from threads where the AI has no role; for the secretary, everything addressed to it |
| task session | one per task the AI is assigned, from first delivery until the task is merged or cancelled, or another AI becomes its assignee (`flows/agent-turn.md` step 17) | the task's worktree | the level the owner set for this AI in this project (default 3; owner's decision of 2026-11-21) | everything about that task |
| review session | one per task the AI reviews, until that task is merged or cancelled, or another AI becomes its reviewer (`flows/agent-turn.md` step 17) | a detached checkout of the submitted commit | the level the owner set for this AI in this project (default 3; owner's decision of 2026-11-21) | review requests for that task |

- A working AI does not change code in its group session: its instructions
  tell it not to, and at level 1 the CLI enforces read-only access (by the
  owner's decision of 2026-11-21 every session defaults to level 3 so that
  group and review sessions keep the CLI's ask-the-user tool). To change code
  it calls `start_task`, which creates a task and moves the work to a new task
  session (R-71).
- An AI runs **one turn at a time per group**, whichever session it is in.
  Items for its other sessions wait. That is what "busy" means in R-72.
- Nothing carries from one task session to the next except what is in the
  group: the brief, the thread, the ledger and the notes.

## 2. States

Each session has one state, kept by the host: `dormant`, `working`,
`waiting` or `takeover`. These are all the changes:

| From | To | When | Where |
| --- | --- | --- | --- |
| `dormant` | `working` | N starts the CLI for an item, or for a turn of its own (a continue turn) | `flows/agent-turn.md` steps 5, 14 |
| `waiting` | `working` | N writes a turn to the open process | `flows/agent-turn.md` step 6 |
| `working` | `waiting` | the turn ends, also after an interrupt | `flows/agent-turn.md` steps 9, 12; E |
| `waiting` | `dormant` | `idle_minutes` passed, `max_open` needs the slot, or the owner took all AIs offline | `flows/agent-turn.md` steps 5.1, 15; E |
| `working` | `dormant` | the CLI exits during a turn or is killed; the owner took all AIs offline | `flows/agent-turn.md` E, G |
| `dormant`, `waiting`, `working` | `takeover` | `takeover.start` is accepted | `flows/takeover.md` steps 3 to 6 |
| `takeover` | `working` | hand-back: the structured CLI starts with the hand-back turn | `flows/takeover.md` step 16 |
| `takeover` | `dormant` | the interactive CLI cannot start | `flows/takeover.md` E |
| `working`, `waiting` (saved) | `dormant` | the node starts | `flows/agent-turn.md` E |
| `takeover` (saved) | `working` | the node starts: hand-back from step 14 | `flows/takeover.md` E |

A session ends, and its row is removed, as section 1 says.

- The host makes a change only from the state it takes the session to be
  in. A change the table does not list, or one from another state than the
  session is in, is refused and changes nothing.
- While one session of an AI in a group is `working` or in `takeover`, no
  other session of that AI in that group becomes `working` or goes to
  `takeover`: section 1 gives the AI one turn at a time per group, and a
  takeover holds its other sessions (`flows/takeover.md` step 9).
- When the node starts, rows saved as `working` or `waiting` become
  `dormant` before anything else happens. A row saved as `takeover` keeps
  that state, and so holds its deliveries, until its hand-back starts the
  structured CLI (`working`) or ends the session (`flows/takeover.md` E).

Overlays, each of which holds deliveries: `paused` (group pause; the
session's task paused by hand, by the round limit, or pre-empted),
`limited` (usage cap or vendor rate limit; also sign-in not valid, which
holds nothing: the next item tries the start again), `offline`
(host not connected, or the owner took all AIs offline). Items of a task
that is `failed` are held too (`flows/agent-turn.md` step 3).

Exactly one process owns the session in `working`, `waiting` and `takeover`;
none in `dormant`. A switch closes the old process and waits for it to exit
before starting the new one.

## 3. What others see

What the home and the web client see is one state per AI per group, derived
by the host in this order: `takeover` if one of its sessions is taken over;
`working` if one is running a turn; `offline` if the owner took all AIs
offline; `limited` if the AI is limited; `paused` if the group is paused;
otherwise `waiting` if any session is open, otherwise `dormant`. The task
is that of the session the state comes from, when it has one. The home
itself sets `offline` when the host's connection closes
(`contracts/commands.md` 1.9).

An external AI (step 3) has no session that the host starts or closes. Its
state is `working`, `waiting`, `paused` or `offline`, as
`contracts/step-3/external.md` section 6 says.
