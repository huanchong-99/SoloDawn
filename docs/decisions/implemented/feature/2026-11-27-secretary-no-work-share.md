# Agent Note: The secretary is offered no work share

Status: implemented

## Problem

Under R-21, a group's secretary coordinates work and takes no tasks. Yet the member card, profile
editor, create-group dialog, and add-AI dialog still displayed and offered the work-share `tier`
control (`Less` / `Normal` / `More`) and tier suggestions for a secretary AI, confusing users into
believing the secretary's assignment share mattered.

## Decision

The owner decided on 2026-11-27 that the secretary takes no work share in the UI:

1. **Card and profile editor:** A secretary member card displays its usage block headed `Quota`
   with no work share and no model-based share suggestion (`Profile.svelte`, `MemberPanel.svelte`);
   its profile editor shows model, thinking level, and `skills` only (`ProfileFields.svelte` with
   `share={false}`).
2. **Group creation and add-AI dialogs:** Selecting an AI as secretary in `CreateGroup.svelte` or
   `AddAI.svelte` hides the work-share chooser immediately; unchecking secretary restores the draft
   value cleanly.
3. **Wire compatibility preserved:** The stored `tier` field on the member row and wire commands
   remains unchanged (`standard` by default) and continues to order escalation candidates if a
   stuck assignee ever escalates to the secretary (`profiles.md` section 2a).

## Alternatives considered

- **Dropping `tier` from the stored secretary row or removing escalation ordering:** rejected
  because keeping the wire shape identical avoids protocol churn and preserves tier ordering for
  section 4 escalation (`contracts/step-3/profiles.md` section 4).
- **Keeping the work-share control visible but disabled:** rejected because showing an inactive
  work-share selector still implies the secretary can be throttled for tasks it never receives.

## Consequences

- UI and acceptance specifications (`features/web_member.feature`, `features/web_profile.feature`,
  `contracts/step-3/profiles.md` section 2a) reflect the hidden control cleanly.
- If a group relies on a secretary as an escalation candidate (`stuck.ask`), its stored default
  tier (`standard`, or whichever value was set before it became secretary) remains active without
  appearing in the UI.

## Verification

- Implemented in commit `92b5ccefe` (`Web: the secretary is offered no work share`) with updated
  Vitest (`member.spec.ts`, `profile.spec.ts`, `model.spec.ts`, `fields.svelte.test.ts`) and
  contract text in [profiles.md](../../../../planning/contracts/step-3/profiles.md) section 2a.

## Related

- [profiles.md](../../../../planning/contracts/step-3/profiles.md) (section 2a)
- [product-rules.md](../../../../planning/product-rules.md) (`R-21`, `R-165`)
