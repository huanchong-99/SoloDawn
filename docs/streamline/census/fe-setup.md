# fe-setup Census — frontend/src/components/setup/

Unit: fe-setup  
Branch: refactor/streamline-quality-gates  
Date: 2026-06-14

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `index.ts` | Barrel re-export for the setup module | `SetupWizardShell` (re-export) | Consumed by `App.tsx` via `@/components/setup` | Only exports SetupWizardShell; other components are imported directly by peers within the module |
| `SetupWizardShell.tsx` | Orchestrator: full-page 5-step wizard host with step state machine and step router | `SetupWizardShell()` component | Mounted at route `/setup` in `App.tsx`; calls `updateAndSaveConfig({setup_wizard_completed:true})` and navigates to `/board` on completion; also re-enterable from Settings via `SettingsLayoutContainer.handleRerunSetup` | Step rendering is a switch on integer `currentStep`; `TOTAL_STEPS = 5` constant is local-only |
| `SetupWizardStepIndicator.tsx` | Visual progress bar: numbered dots with checkmarks and connector lines | `SetupWizardStepIndicator({ steps, currentStep })` component | Used by `SetupWizardShell.tsx` AND by `pages/ui-new/FirstRunWizard.tsx` (the only cross-module consumer) | Standalone presentational component; no state |
| `SetupWizardStep1Welcome.tsx` | View: language picker + continue/skip buttons | `SetupWizardStep1Welcome(props)`, `SetupWizardStep1WelcomeProps` interface | Rendered by `SetupWizardStep1WelcomeContainer`; consumes `getLanguageOptions()` labels passed in | Pure view; no hooks |
| `SetupWizardStep1WelcomeContainer.tsx` | Container: reads current `config.language`, applies `i18n.changeLanguage` and persists via `updateAndSaveConfig` | `SetupWizardStep1WelcomeContainer({ onNext, onSkip })` | Uses `useUserSystem`, `getLanguageOptions`, `uiLanguageToI18nCode`; renders `SetupWizardStep1Welcome` | Language change is immediately persisted to user config |
| `SetupWizardStep2Model.tsx` | View: model configuration — native subscription tab + manual API key tab with verify flow | `SetupWizardStep2Model(props)`, `SetupWizardStep2ModelProps` interface | Rendered by `SetupWizardStep2ModelContainer`; consumes `CLI_TYPES` from `workflow/constants` and `SetupModelMode` type from Container | Largest view file (505 lines); dual-mode UI |
| `SetupWizardStep2ModelContainer.tsx` | Container: native credential detection, API model list fetching, model verification, persists `ModelConfig` to `workflow_model_library` | `SetupWizardStep2ModelContainer({ onNext, onBack, onSkip })`, `SetupModelMode` type | Uses `useNativeCredentials`, `useModelVerification`, `useUserSystem`, `createNativeModelConfig`, `NATIVE_MODEL_ID` | `canProceed` for native mode requires `nativeAvailable`; for manual requires non-empty modelId + apiKey |
| `SetupWizardStep3Project.tsx` | View: git repo directory picker with validation status indicator | `SetupWizardStep3Project(props)`, `SetupWizardStep3ProjectProps` interface | Rendered by `SetupWizardStep3ProjectContainer` | Derives `canContinue` from props; step labeled "Optional" |
| `SetupWizardStep3ProjectContainer.tsx` | Container: debounced git-repo validation via `/api/git/status`, folder picker dialog, project creation via mutation | `SetupWizardStep3ProjectContainer({ onNext, onSkip })` | Uses `FolderPickerDialog`, `useProjectMutations`; inline `checkGitRepo(path)` fetch helper; `deriveProjectName()` pure function | 500ms debounce on directory typing; project name derived from last path segment |
| `SetupWizardStep4Integrations.tsx` | View: Feishu integration toggle + animated credential fields (appId / appSecret) | `SetupWizardStep4Integrations(props)`, `SetupWizardStep4IntegrationsProps` interface | Rendered by `SetupWizardStep4IntegrationsContainer` | Secret visibility toggle is local component state (not lifted) |
| `SetupWizardStep4IntegrationsContainer.tsx` | Container: Feishu enable/disable flow — calls `/api/system-settings PUT` then `feishuApi.updateConfig` | `SetupWizardStep4IntegrationsContainer({ onNext, onSkip })` | Uses `feishuApi`, `makeRequest`, `handleApiResponse` from `@/lib/api`; `handleSkip` is a trivial `onSkip()` wrapper | `saving` flag silently disables onNext during in-flight save; skip proceeds even while saving; errors are swallowed intentionally |
| `SetupWizardStep5Done.tsx` | View: completion screen with Get Started button | `SetupWizardStep5Done({ onGetStarted })` | Rendered by `SetupWizardShell` directly (no container); `onGetStarted` calls `completeWizard()` which sets `setup_wizard_completed: true` and navigates to `/board` | Purely decorative; SparkleIcon + hint about re-running wizard |

## Cross-Module Observations

- **Route gate**: `App.tsx` redirects to `/setup` when `!config.setup_wizard_completed` (after disclaimer). Re-entry from Settings resets the flag to `false` via `SettingsLayoutContainer.handleRerunSetup`.
- **Parallel wizard**: `pages/ui-new/FirstRunWizard.tsx` is a separate 4-step wizard at `/first-run` (installer mode for standalone Windows). It borrows `SetupWizardStepIndicator` but is otherwise independent; it uses `first_run_completed` config key (not `setup_wizard_completed`).
- **No in-flight G1/IDE/quality-gate/AuditPlan connections** — none of the setup wizard files touch the external-editor feature, VS Code webview bridge, quality-gate YAML, or planning-draft/AuditPlan flows.

## Candidates

| Path | Lines | Kind | Evidence | Why | Disposition | Confidence | Blast Radius |
|------|-------|------|---------|-----|-------------|-----------|--------------|
| `SetupWizardStep4IntegrationsContainer.tsx` | 50–51 | redundant | `handleSkip` is `useCallback(() => { onSkip(); }, [onSkip])` — a pass-through wrapper with no side effects | Adds complexity; `onSkip` could be passed directly | refactor | high | Only used internally; renaming or removing the wrapper is safe |
| `SetupWizardStep4IntegrationsContainer.tsx` | 18, 62 | dubious-feature | `saving` state silently disables the Next button (`onNext={saving ? () => {} : handleNext}`) without any loading indicator or disabled styling surfaced to the UI view layer | User gets no feedback during save; clicks are silently dropped | refactor | high | Only this container; fix requires passing `saving` prop to view or using `disabled` button style |
