# Triage Results — AUDIT_REPORT.md Verification (2026-04-14)

## Method
8 parallel agents verified the 81 bugs in AUDIT_REPORT.md against current tree.

## Still-Present Bugs (fix candidates)

### HIGH
- **H06** `crates/services/src/services/config/versions/v7.rs:59` — Broken migration chain; direct v6 deserialization only.

### MEDIUM (Rust)
- **M04** `crates/services/src/services/git/cli.rs:797` — Swapped stderr/stdout labels in error message.
- **M06** `crates/services/src/services/git_host/azure/cli.rs:325` — Hardcoded dev.azure.com fallback.
- **M07** `crates/services/src/services/git.rs:176` — Unconditional cfg creation overwrites partial config.
- **M09** `crates/server/src/routes/subscription_hub.rs:77` — Race in subscribe between lock release and replay.
- **M13** `crates/server/src/routes/workflows.rs:2891` — submit_prompt_response missing pagination.
- **M16** `crates/services/src/services/orchestrator/agent.rs:817-825` — Out-of-order completion events discarded.
- **M18** `crates/services/src/services/terminal/process.rs:928-937` — list_running returns exited/Err processes.
- **M21** `crates/services/src/services/container.rs:750` — ReviewRequest wrong working dir.
- **M24** `crates/services/src/services/config/versions/v2.rs:94` — analytics_enabled handling.
- **M30** `crates/executors/src/executors/cursor.rs:459` — ToolCall error always marked Success.
- **M32** `crates/executors/src/executors/opencode/sdk.rs:521` — parse_model swaps provider/model when no sep.
- **M38** `crates/server/src/routes/tasks.rs:309` — parent_workspace_id can't be cleared.

### MEDIUM (Frontend)
- **M42** `frontend/src/hooks/useExecutionProcesses.ts:54` — missing useMemo (verify again).
- **M46** `frontend/src/hooks/useDevserverPreview.ts:101` — competing useEffect race.
- **M47** `frontend/src/hooks/useAgentAvailability.ts:58` — unstable notifyError dep.
- **M48** `frontend/src/hooks/useTodos.ts:44` — newer todos overwritten.
- **M49** `frontend/src/hooks/useAttemptExecution.ts:54` — unstable useMemo deps.
- **M51** `frontend/src/hooks/useConversationHistory.ts:816-901` — reset/initial-load race.
- **M52** `frontend/src/components/panels/DiffsPanel.tsx:112` — allCollapsed with stale IDs.
- **M55** `frontend/src/components/workflow/steps/Step2Tasks.tsx:170-172` — count-based progress.
- **M56** `frontend/src/components/workflow/steps/Step6Advanced.tsx:154-190` — clears error terminal model.
- **M58** `frontend/src/components/dialogs/tasks/CreatePRDialog.tsx:178-192` — base branch defaults wrong.
- **M59** `frontend/src/components/dialogs/projects/ProjectFormDialog.tsx:78` — dialog shown on error.
- **M60** `frontend/src/components/board/WorkflowKanbanBoard.tsx:79-81` — searchParams useEffect loop.
- **M61** `frontend/src/stores/wsStore.ts:243-245` — cancelled mapped to failed.
- **M64** `frontend/src/stores/wsStore.ts:1296` — connectToWorkflow reconnectAttempts.
- **M65** `frontend/src/utils/fileTypeIcon.ts:102` — Rust icons swapped.
- **M71** `frontend/src/components/ui-new/views/PreviewBrowser.tsx:309-311` — hasDevScript null match.
- **M72** `frontend/src/components/ui-new/containers/PreviewControlsContainer.tsx:60-62` — same.
- **M73** `frontend/src/components/ui-new/containers/ConversationListContainer.tsx:132-150` — stale closure.

### LOW
- **L01** slash_commands.rs:173 · **L02** auth.rs:122 · **L03** v3.rs:43 · **L04** codex/client.rs:311 · **L05** notification.rs:113 · **L06** opencode.rs:230 · **L07** events/streams.rs:342 · **L08** file_search.rs:638
- **L11** Step4Terminals.tsx:226 · **L12** ClickedElementsBanner.tsx:58 · **L13** TaskFollowUpSection.tsx:1013 · **L14** DevServerLogsView.tsx:31 · **L15** SlashCommands.tsx:53 · **L17** Workflows.tsx:678 · **L18** executor.ts:17

## Uncertain / needs re-verification during fix
M05 (deliberate?), M17, M25, M27, M28, M33, M43, M44, M45, M53
