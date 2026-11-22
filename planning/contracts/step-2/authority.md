# Contract (step 2): authority across owners

Who decides what an AI does when a group's home belongs to someone else:
the allow list and approver delegates (D-13), the secretary, and the level
of a task that another person caused (D-1, not in 1.0). The design points are in
`steps/parallel-plan-step-2-3.md` section 1. Rules: R-42, R-44, R-51, R-53,
R-115, R-136.

This file adds to the step 1 contracts. Presence follows
`contracts/conventions.md` section 2.

Not here, all in `contracts/step-2/`: when a home asks an owner, and the
labels (`confirm.md`, `labels.md`); watching (`watch-team.md`); forge
templates (`checks-forge.md`).

## 1. The home decides

By the owner's rules of 2026-11-21 and 2026-11-22 (no defence in code;
members of a group trust each other), a host takes an item from the home of
its group as in step 1 (`contracts/datachannel.md` section 6): the home is
trusted with who may direct an AI, and a host checks no signature of its
owner on an item. Roles, the allow list, delegates and the secretary are
enforced by the home, from its rows. Security between people is outside the
product: join only groups of people you trust.

Below, X is an AI, N its host and O the owner of N.

- **Listed for X**: named in the `allow` of X's `member` row, while X's
  account (`agent.set`, R-50) is `team` or `api_key`. For a `personal`
  account nobody is listed (R-51).

## 2. The allow list and delegates

No new shape. O sets both with the `member set` of step 1
(`contracts/commands.md` 1.2): `allow` for the people whose word is enough
for X (R-53), `approvers` for the people who may decide its approval cards
besides O (R-42). Only O, the AI's owner, sends them; the home keeps O in
`approvers` as in step 1. `allow` is kept whatever the account type, with
effect only while it is `team` or `api_key`. The lists give nothing else:
`confirm` cards and watching stay with the owner.

## 3. Approvals

For `contracts/commands.md` 1.4 the home takes `approve` and `deny` on an
approval card from a person in `body.approvers` who is still in the
`approvers` of the member row of `body.ai` when the command is handled;
anyone else is `forbidden`. That takes a removed delegate off the cards
posted before (A9.19). A delegate decides approval cards of X only. The
host takes the home's `decided` as in step 1.

## 4. The secretary

`group set` with `secretary` is by an admin, as in `contracts/commands.md`
1.1, and the group row says which AI is the secretary. A host treats X as
the secretary (its tools and instructions, `notice`, `confirmed`) when the
group row names X.

## 5. The level of a task session (D-1)

No cap in 1.0 (coordinator, 2026-11-22): R-44 is not applied, and a task
session runs at the level of step 1 (`contracts/commands.md` section 2,
"Levels"), whoever caused the task. R-44 returns with level control.

## 6. Limits

| | Limit | Consequence |
| --- | --- | --- |
| 1 | The home says who may direct an AI and who is the secretary | a member who runs a home can put another member's AI to work; the deployment guide says to join only groups of people you trust |
| 2 | A delegate removed while their `act` is on the way | the home handles it by the row at that moment |
