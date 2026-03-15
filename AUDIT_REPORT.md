# GitCortex Full Codebase Audit Report

**Date:** 2026-03-15
**Scope:** Full codebase (~215k LOC across 12 Rust crates + React/TypeScript frontend)
**Focus:** Silent bugs - functionality failures without error messages
**Method:** 32 parallel audit agents covering non-overlapping code regions

---

## Executive Summary

| Severity | Count |
|----------|-------|
| HIGH     | 14    |
| MEDIUM   | 49    |
| LOW      | 18    |
| **TOTAL**| **81**|

---

## HIGH Severity Bugs

### H01. Orchestrator passes task_id where workflow_id expected - handoff context always empty
- **File:** `crates/services/src/services/orchestrator/agent.rs:5282`
- **Bug:** `GitEvent::find_by_workflow()` receives `prev_terminal.workflow_task_id` (a task ID) instead of a workflow ID. Query returns empty results.
- **Impact:** Handoff context between sequential terminals never includes git event/commit data.
- **Fix:** Pass `workflow_id` parameter instead of `workflow_task_id`.

### H02. `resume_workflow` skips terminal re-preparation - orchestrator operates on dead terminals
- **File:** `crates/server/src/routes/workflows.rs:1843`
- **Bug:** Unlike `start_workflow`, `resume_workflow` does not check if terminals need re-preparation after being paused (PTY processes killed).
- **Impact:** Orchestrator starts successfully but sends commands to dead terminal sessions. Workflow appears "running" but no work happens.
- **Fix:** Add `needs_reprepare` check matching `start_workflow` logic.

### H03. Failed parallel setup scripts never finalize the task
- **File:** `crates/services/src/services/container.rs:143`
- **Bug:** `should_finalize` checks for parallel setup scripts before checking failed/killed status, returning `false` for failed scripts.
- **Impact:** Task status never transitions to InReview, no user notification sent. Task appears stuck.
- **Fix:** Move failed/killed check above the parallel setup script check.

### H04. Gemini orchestrator base URL discarded - API key sent to wrong endpoint
- **File:** `crates/services/src/services/cc_switch.rs:936,986`
- **Bug:** Orchestrator fallback base_url is bound to `_base_url` (discarded). Only `terminal.custom_base_url` is used.
- **Impact:** Gemini CLI launches with correct API key but wrong endpoint. Silent auth failures or wrong service.
- **Fix:** Capture and use `fallback_base_url` like Codex does.

### H05. Chat connector feature enabled by default when env var unset
- **File:** `crates/server/src/routes/chat_integrations.rs:97`
- **Bug:** `is_chat_connector_feature_enabled()` uses `map_or(true, ...)`, opposite of all other feature flags. Also `"1"` disables it.
- **Impact:** All deployments without explicit config have chat connector endpoint open. Setting `=1` disables instead of enables.
- **Fix:** Change to `is_some_and(|v| v.eq_ignore_ascii_case("true") || v == "1")`.

### H06. Config migration chain broken for versions older than v6
- **File:** `crates/services/src/services/config/versions/v7.rs:58`
- **Bug:** v3-v7 use direct deserialization instead of chained `From<String>` impls. v5 configs fail to parse as v6 (different field names).
- **Impact:** Users upgrading from v5 or older silently lose ALL config settings, replaced with defaults.
- **Fix:** Use `From<String>` chain like v8 does, or try each older version.

### H07. Rebase conflict detection broken - errors lose typed data
- **File:** `frontend/src/hooks/useRebase.ts:40`
- **Bug:** `mutationFn` throws `new Error(errorMessage)` instead of the typed `Result<void, GitOperationError>`. `onError` handler checks `err.error.type` which is always `undefined` on plain `Error`.
- **Impact:** Conflict errors never recognized as conflicts. Users see generic error instead of conflict UI.
- **Fix:** Throw the full `res` object instead of wrapping in `new Error`.

### H08. `useHasDevServerScript` returns true for null/undefined scripts
- **File:** `frontend/src/hooks/useHasDevServerScript.ts:12`
- **Bug:** `repo.devServerScript?.trim() !== ''` returns `true` when `devServerScript` is `null` (`undefined !== ''` is `true`).
- **Impact:** Dev server UI elements shown for projects without scripts. Start button does nothing.
- **Fix:** Change to `!!repo.devServerScript?.trim()`.

### H09. `usePreviousPath` navigates to current path instead of previous
- **File:** `frontend/src/hooks/usePreviousPath.ts:74`
- **Bug:** Current pathname is already pushed to `scopedVisited` before the callback runs. `reverse().find()` finds current path first.
- **Impact:** "Go back" feature silently does nothing (navigates to same page).
- **Fix:** Skip the last entry (current path) before searching.

### H10. `lastProcessFailedOrKilled` never reset - shows failure after successful retry
- **File:** `frontend/src/hooks/useConversationHistory.ts:280`
- **Bug:** `mergeCodingAgentResult` sets flag to `true` but never resets to `false` for subsequent successful processes.
- **Impact:** Next-action card shows failure state even after successful retry. User sees "failed" for working tasks.
- **Fix:** Reset flag when a non-failed, non-running process is encountered.

### H11. ConfirmDialog never hides after confirm/cancel
- **File:** `frontend/src/components/dialogs/shared/ConfirmDialog.tsx:34`
- **Bug:** Both handlers call `modal.resolve()` but never `modal.hide()`.
- **Impact:** Dialog stays permanently visible on screen after user clicks. Completely blocks UI.
- **Fix:** Add `modal.hide()` after `modal.resolve()`.

### H12. FeatureShowcaseDialog "Finish" doesn't hide uncloseable dialog
- **File:** `frontend/src/components/dialogs/global/FeatureShowcaseDialog.tsx:43`
- **Bug:** `handleNext` calls `modal.resolve()` on last stage but never `modal.hide()`. Dialog is marked `uncloseable`.
- **Impact:** Dialog permanently stuck on screen with no way to dismiss.
- **Fix:** Call `handleClose()` on last stage.

### H13. BetaWorkspacesDialog never calls modal.hide()
- **File:** `frontend/src/components/dialogs/global/BetaWorkspacesDialog.tsx:17`
- **Bug:** Both handlers resolve but never hide. Dialog is `uncloseable`.
- **Impact:** Dialog permanently stuck on screen.
- **Fix:** Add `modal.hide()` after `modal.resolve()`.

### H14. Push button disappears during push flow
- **File:** `frontend/src/components/ui-new/containers/GitPanelContainer.tsx:187`
- **Bug:** `showPushButton: hasUnpushedCommits && !isInPushFlow` hides button during pending/success/error states.
- **Impact:** User clicks push, button immediately disappears. No feedback on success/failure.
- **Fix:** Change `&&` to `||`: `hasUnpushedCommits || isInPushFlow`.

---

## MEDIUM Severity Bugs

### M01. `planning_drafts.rs:183` - `update_spec` allows setting status to "materialized" without creating a workflow
### M02. `planning_drafts.rs:422` - API key encryption errors silently discarded with `let _ =`
### M03. `models.rs:333` - Anthropic model verification fails for paginated results
### M04. `git/cli.rs:795` - Swapped stderr/stdout labels in error messages
### M05. `git_watcher.rs:663` - Checkpoint commits get TerminalCompleted event instead of GitEvent
### M06. `azure/cli.rs:322` - Hardcoded dev.azure.com URL fails for legacy visualstudio.com PRs
### M07. `git.rs:170` - `ensure_cli_commit_identity` overwrites existing git config when only one field missing
### M08. `mcp/task_server.rs:291` - API token cached at construction, breaks after runtime rotation
### M09. `subscription_hub.rs:66` - Race condition in `subscribe` causes out-of-order event replay
### M10. `terminals.rs:630` - `stop_terminal` doesn't unregister TerminalBridge (resource leak)
### M11. `workflows.rs:1964` - `cleanup_workflow_terminals` doesn't unregister terminal bridges
### M12. `workflows.rs:1817` - `pause_workflow` doesn't clear `pty_session_id` (stale references)
### M13. `workflows.rs:2896` - Inconsistent default pagination between persisted and runtime messages
### M14. `state.rs:276` - Conversation trimming duplicates system messages
### M15. `agent.rs:3303` - Cancelled terminals counted as successful completions
### M16. `agent.rs:817` - Out-of-order completion events permanently discarded
### M17. `prompt_watcher.rs:2232` - Dead code path: EnterConfirm custom API key fallback unreachable
### M18. `process.rs:928` - `list_running` returns exited processes
### M19. `prompt_watcher.rs:311` - Operator precedence error in scope gap marker detection
### M20. `container.rs:1111` - Live log normalization skipped when executor profile not found
### M21. `container.rs:750` - ReviewRequest log normalization from DB uses wrong working directory
### M22. `cc_switch.rs:816` - `create_claude_settings` failure silently swallowed
### M23. `chat_connector.rs:83` - Telegram API errors return `Ok("")` instead of `Err`
### M24. `v2.rs:94` - v1-to-v2 migration drops `analytics_enabled` preference
### M25. `profile.rs:390` - `get_coding_agent` doesn't canonicalize variant key
### M26. `stdout_dup.rs:148` - Injected lines not forwarded to duplicate stream
### M27. `codex.rs:382` - `auto_approve` not set for `AskForApproval::Never`
### M28. `codex/normalize_logs.rs:563` - `ExecCommandBegin` overwrites CommandState from approval
### M29. `codex/normalize_logs.rs:767` - Stale patch entries survive when file count changes
### M30. `cursor.rs:457` - Cursor completed ToolCall always marked as Success even on failure
### M31. `opencode/normalize_logs.rs:361` - Missing `return` after empty call_id check
### M32. `opencode/sdk.rs:518` - `parse_model` swaps provider and model when no separator
### M33. `droid/normalize_logs.rs:884` - `session_id()` ignores System variant's session ID
### M34. `file_search.rs:529` - Incorrect match_type assigned at build time, destroys ranking
### M35. `diff_stream.rs:491` - Omitted diffs for previously-sent paths silently dropped
### M36. `main.rs:357` - Feishu manual reconnect unreachable due to `std::future::ready`
### M37. `main.rs:351` - Feishu `connected` flag set before connection established
### M38. `tasks.rs:309` - `parent_workspace_id` can never be cleared on task update
### M39. `useWorkflows.ts:422` - `submitPromptResponse` silently drops fields
### M40. `useWorkspaceMutations.ts:38` - `toggleArchive` negates the `archived` param
### M41. `useWorkspaceMutations.ts:52` - `togglePin` negates the `pinned` param
### M42. `useExecutionProcesses.ts:54` - Missing `useMemo` on derived arrays
### M43. `useForcePush.ts:24` - Silently succeeds when `attemptId` is undefined
### M44. `useChangeTargetBranch.ts:53` - Cache invalidation uses wrong `repoId`
### M45. `useDebouncedCallback.ts:34` - Stale `delay` closure
### M46. `useDevserverPreview.ts:101` - Competing useEffect hooks cause state reset race
### M47. `useAgentAvailability.ts:58` - Unstable `notifyError` dep can cause infinite loop
### M48. `useTodos.ts:44` - Newer todos can be overwritten by older entries
### M49. `useAttemptExecution.ts:54` - useMemo never memoizes due to unstable deps
### M50. `useCurrentUser.ts:8` - Missing `enabled` guard, stale user data after sign-out
### M51. `useConversationHistory.ts:806` - Reset and initial-load effects race
### M52. `DiffsPanel.tsx:112` - `allCollapsed` breaks with stale IDs
### M53. `DiffsPanel.tsx:55` - collapsedIds/processedIds never reset on attempt change
### M54. `PreviewPanel.tsx:105` - startTimer leaks timeouts
### M55. `Step2Tasks.tsx:188` - Progress indicator uses count-based comparison
### M56. `Step6Advanced.tsx:154` - useEffect silently clears error terminal model
### M57. `Step3Models.tsx:95` - handleOpenAddDialog doesn't reset isFormVerified
### M58. `CreatePRDialog.tsx:186` - Base branch defaults to current branch
### M59. `ProjectFormDialog.tsx:78` - Dialog hides on error
### M60. `Board.tsx:28` - Potential infinite useEffect loop from searchParams
### M61. `wsStore.ts:243` - normalizeTerminalCompletedStatus maps cancelled to unknown
### M62. `api.ts:1099` - profilesApi.save sends raw string without JSON.stringify
### M63. `wsStore.ts:230` - Missing checkpoint case in status normalization
### M64. `wsStore.ts:1280` - connectToWorkflow doesn't reset reconnectAttempts
### M65. `fileTypeIcon.ts:102` - Rust file icons swapped (dark/light)
### M66. `extToLanguage.ts:63` - Leading dot not stripped
### M67. `carousel.tsx:107` - reInit event listener leaks
### M68. `ApprovalFeedbackContext.tsx:56` - isTimedOut never updates after render
### M69. `ToolStatusDot.tsx:22` - Missing `relative` positioning
### M70. `ToolStatusDot.tsx:28` - Invalid Tailwind class `bg-text-low`
### M71. `PreviewBrowser.tsx:309` - hasDevScript check matches null scripts
### M72. `PreviewControlsContainer.tsx:60` - Same hasDevScript bug
### M73. `ConversationListContainer.tsx:136` - Stale closure in debounced callback

---

## LOW Severity Bugs

### L01. `slash_commands.rs:173` - Cannot clear prompt_template to None via update
### L02. `middleware/auth.rs:110` - constant_time_eq leaks token length
### L03. `v3.rs:43` - v3 migration resets telemetry_acknowledged
### L04. `codex/client.rs:304` - Denial decision depends on whether user typed a reason
### L05. `notification.rs:110` - Insufficient AppleScript escaping
### L06. `opencode.rs:230` - default_mcp_config_path returns None when file doesn't exist
### L07. `events/streams.rs:342` - Overly broad path prefix match
### L08. `file_search.rs:644` - Watching .git/HEAD fails after atomic replacement
### L09. `cleanup-quality-data.sh:27` - Quality issue records orphaned
### L10. `TerminalDebugView.tsx:121` - `??` doesn't catch empty string for terminal labels
### L11. `Step4Terminals.tsx:226` - Non-OK HTTP responses silently swallowed
### L12. `ClickedElementsBanner.tsx:58` - isExpanded state has no setter
### L13. `TaskFollowUpSection.tsx:1013` - Editor not disabled during send
### L14. `DevServerLogsView.tsx:27` - Stale activeProcessId
### L15. `SlashCommands.tsx:53` - Delete dialog uses error i18n key as title
### L16. `TerminalActivityPanel.tsx:153` - Collapse/expand caret icons swapped
### L17. `Workflows.tsx:688` - Inconsistent lowercase comparison on roles
### L18. `executor.ts:17` - null vs undefined profile comparison

---

*Report generated by 32 parallel audit agents analyzing ~215k lines of code.*
