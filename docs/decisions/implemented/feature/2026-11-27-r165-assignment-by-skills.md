# Agent Note: Route hard and special tasks by skills before work-share tier (R-165)

Status: implemented

## Problem

Under R-165 as written earlier on 2026-11-27 ([product-rules.md](../../../../planning/product-rules.md) and [profiles.md](../../../../planning/contracts/step-3/profiles.md) section 3), `tier` carried two meanings at once: the owner's desired share of work (`strong` more, `light` fewer) and who should receive hard tasks ("more tasks and the hard ones" to `strong`, "fewer and simple ones" to `light`). When a fast or scarce model has the expertise for a hard bug and its owner lowers its work share (`light`) to save subscription quota while setting a roomier model to `strong` for routine implementation, routing hard tasks by `tier` sends the hard bug to the higher-quota model and conflicts with using quota headroom and `skills` sensibly. The first change of the same report, putting the quota window before the tier, made the old wording a conflict: a roomy AI set to more would still be given the hard tasks.

## Decision

The coordinator proposed this on 2026-11-27 (point 4 of proposal 1 of the lessons report written that day, after a subscription was used up while a roomier one did documentation); the owner assented the same day: "同意改成这个按擅长拍" (`user-approved`, the trade-off was shown: while skills are empty the hard tasks still go to the higher tier). Source: the decision log, kept outside the repository. Updates R-165 in [product-rules.md](../../../../planning/product-rules.md) and [profiles.md](../../../../planning/contracts/step-3/profiles.md).

The secretary chooses assignee and reviewer task by task (rather than by the whole request), in this order:

1. Never choose an AI that is `limited` or `offline`.
2. Respect the quota window and shared subscriptions first (members with the same CLI on the same computer share one subscription, not two external AIs).
3. Follow `tier` as the work share (`strong` more, `standard` a normal share, `light` fewer).
4. Match `skills` for which tasks: hard and special tasks go to the AIs whose skills name them, and fall back to the higher `tier` only when skills say nothing.
5. Spread independent tasks over different AIs.

The UI hint (`profile.tier.hint` in `web/src/messages/profile.{en,zh-CN}.json`) and the prompt bullet (`internal/host/instructions/secretary.md`) state this rule directly.

## Alternatives considered

- **Leave R-165 as written:** keeps hard tasks tied to `strong`, leaving the owner unable to set a roomy AI to `strong` for routine work without also sending it the hard tasks.
- **Put the quota window first without updating R-165:** can ship without changing a product rule because window order alone does not alter R-165, but leaves the owner unable to set a roomy AI to `strong` without sending it the hard tasks.
- **Separate stored strength field ([profiles.md](../../../../planning/contracts/step-3/profiles.md), strength apart from share):** adds a new field to the member row, commands, home rules, and frozen protocol shapes. Deferred until the owner has used work-share tiers in a real group.

## Consequences

- While an AI's `skills` are empty, the hard tasks still go to the higher `tier` as before; the window-first order and the task-by-task choice change the assignment for every user.
- Owners can now set a scarce specialist AI to `light` with explicit skills (such as hard bugs or protocol design) and a roomy AI to `strong` for routine tasks.
- Escalation on two failed reviews (`stuck.ask`, [profiles.md](../../../../planning/contracts/step-3/profiles.md) section 4) still orders candidates by `tier` (`light < standard < strong`); recorded as an open limitation in [issues.md](../../../../issues.md).

## Verification

- `features/host_profile.feature` ("The secretary sees the assignment roster and rules (A10.9 host)" via `checkAssignmentRules` in `features/host_profile_test.go`) and `features/a10_profiles.feature` (A10.9) pin the updated assignment wording in `internal/host/instructions/secretary.md`.
- Real CLI probing of model adherence (four probes covering window-first split, coordinate-only role boundary, keeping temporary conditions out of project memory, and announcing decisions in the turn made) is in the owner's pre-rc.2 test sheet (kept outside the repository).

## Related

- [product-rules.md](../../../../planning/product-rules.md) (`R-165`)
- [profiles.md](../../../../planning/contracts/step-3/profiles.md) (section 3)
