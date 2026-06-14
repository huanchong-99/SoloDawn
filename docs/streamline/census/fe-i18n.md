# fe-i18n Census — frontend/src/i18n/

Unit: fe-i18n | Branch: refactor/streamline-quality-gates | Date: 2026-06-14

## Structure

6 locales: `en`, `ja`, `es`, `ko`, `zh-Hans`, `zh-Hant`  
9 namespaces per locale: `common`, `settings`, `projects`, `tasks`, `organization`, `workflow`, `quality`, `setup`, `slashCommands`  
4 infrastructure files: `config.ts`, `index.ts`, `languages.ts`, `__tests__/config.test.ts`, `__tests__/workflow.test.ts`

---

## Infrastructure Files

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `config.ts` | i18next initialization; imports all locale JSON, registers resources, sets zh-Hans as default | `updateLanguageFromConfig(configLanguage)`, default export `i18n` | Consumed by `index.ts` (re-export), `App.tsx` (`I18nextProvider`), all components via react-i18next | fallback chain: zh-TW/HK/MO→zh-Hant→en; generic zh→zh-Hans→en |
| `index.ts` | Re-exports `config.ts` | `default` i18n instance | Entry point imported as `@/i18n` | Thin barrel |
| `languages.ts` | Centralized language map; converts `UiLanguage` enum to i18next codes; provides dropdown options | `UI_TO_I18N`, `SUPPORTED_I18N_CODES`, `uiLanguageToI18nCode()`, `getLanguageOptions()`, `UiLanguage` (re-exported from `shared/types`) | Used by `config.ts` (`SUPPORTED_I18N_CODES`, `uiLanguageToI18nCode`); by settings components for language dropdowns | `getEndonym()` uses `Intl.DisplayNames` with fallback map |
| `__tests__/config.test.ts` | Verifies no debug console.log in test mode | — | Imports `config.ts` | Passes in test env due to `isTestEnv` guard |
| `__tests__/workflow.test.ts` | Smoke test: loads workflow namespace and checks `wizard.title` | — | Imports `@/i18n` | Ensures bundle integrity |

---

## Locale Namespace Map

| Namespace | Content Summary | Primary Consumers | Key Sections |
|-----------|----------------|-------------------|-------------|
| `common` | Shared UI primitives: buttons, states, nav, workspaces, file tree, actions, onboarding, concierge, approval, feishu channel, statusBar | All components | `actions`, `workspaces`, `concierge`, `navbar`, `onboarding` (dead — see candidates), `statusBar`, `workspace`, `approval`, `feishuChannel` |
| `settings` | All settings page strings: general, editor, agents, MCP, projects, repos, feishu, runtime, newDesign nav | GeneralSettingsNew, AgentSettingsNew, McpSettingsNew, ProjectSettingsNew, ReposSettingsNew, FeishuSettingsNew, RuntimeSettingsNew, GhCliSetupDialog, EditorAvailabilityIndicator | `settings.settings.layout.*` (dead), `settings.settings.github.*` (dead duplicate), `newDesign.common.*` (dead), `runtime.*` (missing from 4 of 6 locales) |
| `projects` | Project list page: create, link, unlink, openInIDE | ProjectsPage, LinkProjectDialog | `openInIDE` key dead — no t() caller found |
| `tasks` | Task board, attempt, diffs, preview, git actions, PR creation, review, restore | TaskPanel, ActionsDropdown, NextActionCard, PreviewBrowser, CreatePRDialog, etc. | `attempt.actions.openInIde` dead, `attempt.actions.openMenu` dead, `attempt.actions.stopDevServer` dead |
| `organization` | Org create/invite, member list, invitations, shared projects | OrganizationSettingsNew, CreateOrganizationDialog, InviteMemberDialog | Fully used |
| `workflow` | Workflow wizard (6 steps), board, kanban, pipeline, terminal debug, management | Workflows page, all workflow step components | Fully used; `workflow:wizard.title` is smoke-tested |
| `quality` | Quality Gate Report panel: status badges, metrics, mode labels | QualityReportPanel, QualityBadge | Fully used |
| `setup` | Setup wizard steps (welcome/model/project/integrations/done) + first-run wizard | SetupWizardShell, FirstRunWizard | Fully used |
| `slashCommands` | Slash command list page: CRUD dialogs, form, empty states | SlashCommands page | Fully used |

---

## Locale Coverage Gaps

| Section | EN | zh-Hans | ja | ko | es | zh-Hant |
|---------|----|---------|----|----|----|---------|
| `common.select` | yes | no | no | no | no | no |
| `common.projectSelector` | yes | no | no | no | no | no |
| `common.createConfiguration` | yes | no | no | no | no | no |
| `common.signIn/signOut` | yes | no | no | no | no | no |
| `common.disclaimer` | yes | no | no | no | no | no |
| `common.onboarding` | yes | zh-Hans only | no | no | no | no |
| `common.releaseNotes` | yes | no | no | no | no | no |
| `common.workspace` | yes | no | no | no | no | no |
| `common.approval` | yes | no | no | no | no | no |
| `common.feishuChannel` | yes | no | no | no | no | no |
| `common.statusBar` | yes | no | no | no | no | no |
| `common.actions.selectAssignee` | yes | no | no | no | no | no |
| `settings.runtime` | yes | zh-Hans only | no | no | no | no |

---

## Dead / Candidate Keys

| Key Path (EN) | All Locales? | Evidence | Recommendation |
|---------------|-------------|----------|----------------|
| `projects.openInIDE` | yes (all 6) | No `t(...)` call references this key anywhere in .ts/.tsx | delete |
| `tasks.attempt.actions.openInIde` | yes (all 6) | Only `attempt.actions.startDevServer` is used in PreviewBrowser.tsx; `openInIde`/`openMenu`/`stopDevServer` in this sub-object have no callers | delete |
| `tasks.attempt.actions.openMenu` | yes (all 6) | No caller found | delete |
| `tasks.attempt.actions.stopDevServer` | yes (all 6) | `stopDevServer` callers use `preview.toolbar.stopDevServer` not this path | delete |
| `common.onboarding.*` (chooseEditor, preferredEditor, etc.) | en + zh-Hans | OnboardingDialog.tsx is exported but never `.show()`'d; App.tsx comment says "New setup wizard replaces old onboarding dialog" | delete (entire onboarding sub-object in common) |
| `settings.settings.layout.nav.*` | all 6 | No t() call uses `settings.layout.*` path | delete |
| `settings.settings.github.cliSetup.*` | all 6 | Duplicate of `settings.integrations.github.cliSetup.*`; only the integrations path is used in GhCliSetupDialog.tsx | delete |
| `settings.newDesign.common.*` | all 6 | No t() caller references `newDesign.common.*` path | delete |
| `settings.newDesign.layout.rerunSetupDescription` | all 6 | `rerunSetup` is used but `rerunSetupDescription` has no caller | delete |

---

## In-Flight Relevance

- **G1 IDE/Editor deletion candidate**: `actions.openInIde` (common + tasks namespaces) is referenced by `RepoCard.tsx` and `actions-dropdown.tsx` — these are **live** callers for the "Open in IDE" button. The `useOpenInEditor` hook is also live. However `projects.openInIDE`, `tasks.attempt.actions.openInIde`, `tasks.attempt.actions.openMenu`, `tasks.attempt.actions.stopDevServer` are dead.
- **Quality Gate System (quality namespace)**: Fully used by QualityReportPanel and QualityBadge. The `quality.mode.*` keys (off/shadow/warn/enforce) map to QualityGateConfig modes.
- **AuditPlan System (planning.auditDoc.*)**: Active — all keys in `tasks.conversation.planning.auditDoc.*` are consumed by `AuditDocPanel.tsx` and the `CreateChatBoxContainer.tsx`.
- **Feishu sync (invisible feature)**: `common.concierge.feishuSync`, `feishuSyncEnabled`, `feishuSyncDisabled`, `syncHistory`, etc. exist and are used — Feishu is a live integration.
- **VS Code webview bridge**: No specific i18n keys identified for webview bridge.
