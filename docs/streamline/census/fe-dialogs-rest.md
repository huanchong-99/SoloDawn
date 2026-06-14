# Census: fe-dialogs-rest

Unit covers `frontend/src/components/dialogs/` **excluding** `dialogs/tasks/`.

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `dialogs/index.ts` | Barrel re-export for all dialogs | Re-exports every named dialog, type, and prop interface | Used by `types/modals.ts`, `hooks/useMessageEditRetry`, `hooks/useRetryProcess`, `pages/ui-new/settings/OrganizationSettingsNew` | Also re-exports tasks/ dialogs (out of scope). `ConfirmDialog` re-export here is orphaned at runtime — all callers use `ui-new/dialogs/ConfirmDialog` |
| `shared/ConfirmDialog.tsx` | Generic confirm/cancel modal (old design) | `ConfirmDialog`, `ConfirmDialogProps` | Re-exported from barrel; only runtime use is `types/modals.ts` importing `ConfirmDialogProps` for type-only purpose | Duplicate of `ui-new/dialogs/ConfirmDialog`. The new version adds `showCancelButton` prop. All 12 call-sites import from `ui-new/dialogs/ConfirmDialog`. |
| `shared/FolderPickerDialog.tsx` | File-system folder browser (browse, navigate, select) | `FolderPickerDialog`, `FolderPickerDialogProps` | Used by `RepoPickerDialog`, `SetupWizardStep3ProjectContainer`, `BrowseRepoButtonContainer`, `workflow/steps/Step0Project`, `ui-new/dialogs/CreateRepoDialog` | Actively used in 5+ locations; resolves `string | null` |
| `shared/LoginRequiredPrompt.tsx` | Alert + login CTA shown when user is not authenticated | `LoginRequiredPrompt` (React component, not a modal) | Used by `LinkProjectDialog`, `tasks/ShareDialog`, `pages/ui-new/settings/OrganizationSettingsNew` | Not a NiceModal — renders inline within other dialogs |
| `shared/RepoPickerDialog.tsx` | Pick/create/register a git repo (3-stage wizard) | `RepoPickerDialog`, `RepoPickerDialogProps` | Used by `ProjectFormDialog`, `pages/ui-new/settings/ProjectSettingsNew`; internally uses `FolderPickerDialog` and `repoApi` | Resolves `Repo | null` |
| `global/DisclaimerDialog.tsx` | First-run safety disclaimer (uncloseable) | `DisclaimerDialog` | Called in `App.tsx` as part of first-run flow; resolves `'accepted'` | Uncloseable dialog; gated by `config.disclaimer_acknowledged` |
| `global/OnboardingDialog.tsx` | First-run onboarding: agent + editor picker | `OnboardingDialog`, `OnboardingResult` | Exported from barrel and declared in `types/modals.ts` but **never called** anywhere (App.tsx comment: "New setup wizard replaces old onboarding dialog") | [G1] Contains editor picker (EditorType enum + custom command). Dead — replaced by `/setup` wizard route |
| `global/OAuthDialog.tsx` | OAuth login popup flow (GitHub/Google) | `OAuthDialog` | Used by `LoginRequiredPrompt`, `tasks/ShareDialog`, `layout/Navbar` | Opens popup window; polls `useAuthStatus` until login completes |
| `global/ReleaseNotesDialog.tsx` | Iframe-embedded release notes viewer | `ReleaseNotesDialog` | Called in `App.tsx` when `config.show_release_notes` is true; also exported from barrel | Falls back to "Open in browser" if iframe fails to load |
| `projects/ProjectEditorSelectionDialog.tsx` | Fallback editor picker when `openEditor` API fails | `ProjectEditorSelectionDialog`, `ProjectEditorSelectionDialogProps` | Used only from `hooks/useOpenProjectInEditor` (called from `Navbar`); internally calls `useOpenProjectInEditor` recursively | [G1] Part of "open in external IDE" feature. Uses `EditorType` enum. The dialog only appears on `openEditor` failure with no explicit `editorType`. |
| `projects/ProjectFormDialog.tsx` | Project creation flow (delegates to RepoPickerDialog) | `ProjectFormDialog`, `ProjectFormDialogProps`, `ProjectFormDialogResult` | Exported from barrel; **no external caller** found — `ProjectFormDialog.show()` is never called outside the file itself | Dead — barrel-exported but never invoked. RepoPickerDialog is called directly in `ProjectSettingsNew`. |
| `projects/LinkProjectDialog.tsx` | Link local project to remote org project (or create new) | `LinkProjectDialog`, `LinkProjectResult` | Called from `tasks/ShareDialog` | Requires `remoteFeaturesEnabled`; shows `LoginRequiredPrompt` when not signed in |
| `org/CreateOrganizationDialog.tsx` | Create a new org (name + auto-slug) | `CreateOrganizationDialog`, `CreateOrganizationResult` | Used by `pages/ui-new/settings/OrganizationSettingsNew` | Auto-generates slug from name; 3-50 char constraints |
| `org/InviteMemberDialog.tsx` | Invite member to org by email + role | `InviteMemberDialog`, `InviteMemberDialogProps`, `InviteMemberResult` | Used by `pages/ui-new/settings/OrganizationSettingsNew` | Custom email validator (no regex backtracking, linear scan) |
| `auth/GhCliSetupDialog.tsx` | Install + authenticate `gh` CLI for a task attempt | `GhCliSetupDialog`, `GhCliHelpInstructions`, `mapGhCliErrorToUi`, `GhCliSupportContent`, `GhCliSupportVariant` | Called from `tasks/CreatePRDialog`; helper exports `GhCliHelpInstructions` and `mapGhCliErrorToUi` are re-used inside `CreatePRDialog` | Calls `attemptsApi.setupGhCli`; handles BREW_MISSING / SETUP_HELPER_NOT_SUPPORTED error variants |
| `settings/CreateConfigurationDialog.tsx` | Create a new executor configuration (name + optional clone) | `CreateConfigurationDialog`, `CreateConfigurationDialogProps`, `CreateConfigurationResult` | Used by `pages/ui-new/settings/AgentSettingsNew` | Pure UI confirmation — no async calls; returns name + cloneFrom to caller |
| `settings/DeleteConfigurationDialog.tsx` | Confirm deletion of an executor configuration | `DeleteConfigurationDialog`, `DeleteConfigurationDialogProps`, `DeleteConfigurationResult` | Used by `pages/ui-new/settings/AgentSettingsNew` | try/catch around `modal.resolve()` which never throws — dead error path |
| `git/ForcePushDialog.tsx` | Confirm + execute git force-push | `ForcePushDialog`, `ForcePushDialogProps` | Used by `hooks/useGitOperations` and `ui-new/containers/GitPanelContainer` | Calls `useForcePush` hook; resolves `'success'` or `'canceled'` |
| `scripts/ScriptFixerDialog.tsx` | Edit + save + test setup/cleanup/dev-server scripts | `ScriptFixerDialog`, `ScriptFixerDialogProps`, `ScriptFixerDialogResult`, `ScriptType` | Called from `NormalizedConversation/DisplayConversationEntry`, `ui-new/containers/NewDisplayConversationEntry`, `ui-new/containers/PreviewBrowserContainer` | Shows live logs via `useLogStream` + `useExecutionProcesses`; `VirtualizedProcessLogs` for output |
| `wysiwyg/ImagePreviewDialog.tsx` | Full-size image preview for WYSIWYG editor | `ImagePreviewDialog`, `ImagePreviewDialogProps` | Called from `ui/wysiwyg/nodes/image-node.tsx` | Resolves `void`; shows loading spinner until image loads; displays format + size metadata |

## Candidates Summary

| Candidate | Kind | Disposition |
|-----------|------|-------------|
| `global/OnboardingDialog.tsx` | dead | delete — replaced by `/setup` wizard, no callers |
| `projects/ProjectFormDialog.tsx` | dead | delete — no callers outside its own file |
| `shared/ConfirmDialog.tsx` | duplicate | refactor/delete — all runtime callers use `ui-new/dialogs/ConfirmDialog`; only type import via barrel |
| `settings/DeleteConfigurationDialog.tsx` (lines 34–42) | bug | refactor — try/catch around `modal.resolve()` which cannot throw; dead error path with misleading state |
| `projects/ProjectEditorSelectionDialog.tsx` | dubious-feature | investigate — part of [G1] "open in IDE" fallback; uses `useOpenProjectInEditor` recursively |
