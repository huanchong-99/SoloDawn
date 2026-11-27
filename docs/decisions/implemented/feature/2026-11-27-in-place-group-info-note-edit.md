# Agent Note: In-place editing in group info and the `note_edit` command

Status: implemented

## Problem

Editing group details (name, secretary) and project memory entries (notes, knowledge-base
documents, style presets) previously required separate forms or deleting and recreating notes,
losing the original note's position, task link, and id. Deleting and reposting a note was also
error-prone when correcting wording in an active project.

## Decision

The owner decided and approved unit M2 on 2026-11-27: **every item in the group info panel is
edited in place where it is displayed, backed by a new `note_edit` home command**:

1. **In-place group info UI:** Group admins edit the group name and secretary inline inside
   group info (`GroupInfoPanel.svelte` / `EditGroup.svelte`). Any human member can add or edit
   project-memory notes, knowledge-base documents, and style presets in place (`Ctrl`/`Cmd+Enter`
   to save, `Esc` to cancel, conflict reporting if edited after deletion).
2. **`note_edit` home command (`internal/home/cmd_note.go`, `internal/proto/command.go`):** Updates
   a project-memory note's text while preserving its `id`, thread/list position, associated `task`,
   and `type`.
3. **Boundaries:** Editing a note that has already been deleted answers `not_found`; AIs are
   forbidden from invoking `note_edit` (`internal/home/cmd_note.go`, keeping note creation/KB
   tools separate).

## Alternatives considered

- **Delete-and-recreate notes from the browser:** destroys the original note id, reordering
  entries and detaching task context; rejected.
- **Allowing AIs to invoke `note_edit`:** rejected for M2 so human curation in project memory stays
  unambiguous and audited under the existing command permission rules.

## Consequences

- Wire protocol (`internal/proto/testdata/note_edit.json`, `wire.json`) and web schemas (`proto.ts`)
  gain `note_edit`, verified against existing frozen contract conventions.
- Race cases where an admin forgets/deletes an item while another member edits it return `not_found`
  with clear inline UI feedback.

## Verification

- Implemented in commit `3ff4cd33e` (`feat: everything in group info is changed where it is shown; new note_edit command (M2)`), verified by Go home unit & acceptance tests (`internal/home/cmd_note_test.go`, `features/home_post.feature`), web feature tests (`features/web_groupinfo.feature`, `web_kb.feature`, `web_preset.feature`), and Playwright e2e (`web/e2e/m2.spec.ts`).

## Related

- [commands.md](../../../../planning/contracts/commands.md)
- [knowledge.md](../../../../planning/contracts/step-3/knowledge.md)
