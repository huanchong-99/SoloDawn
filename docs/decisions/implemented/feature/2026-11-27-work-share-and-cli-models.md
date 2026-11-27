# Agent Note: Work share beside quota, CLI model catalogs, and create-group model choices

Status: implemented

## Problem

Users interpreted the `tier` label (`light`, `standard`, `strong`) as a model-intelligence or
thinking-effort setting instead of a task-allocation preference, while quota usage windows and
thinking levels (`effort`) from each CLI were hard to inspect and adjust consistently across member
cards, editors, and group creation dialogs.

## Decision

The owner decided on 2026-11-27 (shipped as unit M1) to separate model/thinking choices cleanly
from task-allocation share without breaking wire compatibility:

1. **Tier presented as work share beside quota:** Reworded the tier control across UI (`Work share:
   Less / Normal / More` in English, with the Chinese message catalogue carrying the matching
   labels), secretary turn context, and contracts (`profiles.md`), while preserving stored `light`,
   `standard`, and `strong` values on the wire. The AI card leads with its work share and live
   quota usage bar (`window_pct` / resets or today's turn count).
2. **CLI-sourced model and thinking-level catalogs:** Each CLI reports available `models` and per-
   model `efforts` in `host.info`. The profile editor (`Edit profile`), `Add AI`, and
   `Create group` dialogs present dropdowns for model and thinking level (`CLI default` when
   unset), falling back to text fields when no model catalog exists (`external` or unlisted CLIs).
3. **Group creation includes model, level, and share:** Choosing an AI in `Create group` exposes
   its model, thinking level (pre-filling suggested work share when a model is picked), and work-
   share setting before sending `host.join`.

## Alternatives considered

- **Renaming the protocol field `tier` to `work_share`:** avoided unnecessary wire/schema churn
  across frozen v3 protocol types and SQLite rows when UI wording, hints, and card pairing solved
  the ambiguity.
- **Hardcoding model lists in the web client:** rejected because CLIs update model ids and thinking
  efforts independently; reading them from `host.info` keeps listings current without client
  releases.

## Consequences

- Stored `tier` values remain unchanged; existing clients and secretary assignments keep working
  seamlessly, with subsequent secretary-card adjustments in [secretary-no-work-share](./2026-11-27-secretary-no-work-share.md).
- If a CLI stops listing an already configured model, the UI renders it as "not listed" while
  retaining the saved setting (`contracts/step-3/profiles.md` section 2a).

## Verification

- Implemented in commit `6a456afcb` (`Web: the tier is the work share beside the quota; thinking level from the CLI; model and level when a group is made (M1)`), verified across Vitest (`member.spec.ts`, `model.spec.ts`, `profile.spec.ts`, `confirm.spec.ts`, `a11y.spec.ts`) and Playwright e2e (`web/e2e/profiles.spec.ts`).

## Related

- [profiles.md](../../../../planning/contracts/step-3/profiles.md) (sections 1 and 2a)
- [2026-11-27-secretary-no-work-share.md](./2026-11-27-secretary-no-work-share.md)
