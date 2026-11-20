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
| group session | one per AI per group, long-lived. For the secretary this is its only session. | the project clone | 1, read only | main-line mentions, direct questions (R-70), calls from threads where the AI has no role; for the secretary, everything addressed to it |
| task session | one per task the AI is assigned, from first delivery until the task is merged, cancelled or given to another AI (`flows/agent-turn.md` step 17) | the task's worktree | the level the owner set for this AI in this project | everything about that task |
| review session | one per task the AI reviews, until that task is merged, cancelled or given to another AI (`flows/agent-turn.md` step 17) | a read-only checkout of the submitted commit | 1, read only | review requests for that task |

- A working AI cannot change code in its group session: the session is read
  only. To change code it calls `start_task`, which creates a task and moves
  the work to a new task session (R-71).
- An AI runs **one turn at a time per group**, whichever session it is in.
  Items for its other sessions wait. That is what "busy" means in R-72.
- Nothing carries from one task session to the next except what is in the
  group: the brief, the thread, the ledger and the notes.

## 2. States

Each session has one state, kept by the host.

```
            deliver                    turn ends
 dormant ------------> working <------------------> waiting
    ^                    |  ^                          |
    | idle timeout       |  | hand-back                | idle timeout
    +--------------------+--+--------------------------+
                         |  |
        owner takes over v  |
                      takeover
```

Overlays, each of which holds deliveries: `paused` (group pause, round limit,
pre-empted), `limited` (usage cap or vendor rate limit), `offline` (host not
connected, or the owner took all AIs offline).

Exactly one process owns the session in `working`, `waiting` and `takeover`;
none in `dormant`. A switch closes the old process and waits for it to exit
before starting the new one.

## 3. What others see

What the home and the web client see is one state per AI per group, derived
by the host: the state of the session that is running a turn or is taken
over, otherwise `waiting` if any session is open, otherwise `dormant`; and
the task of that session.
