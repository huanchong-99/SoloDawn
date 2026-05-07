# Wave-1 Exploration Findings (40 agents)

**Date:** 2026-04-14
**Total agents:** 40
**Total findings:** ~400+ bugs across all regions

## HIGH-Severity Bugs (for fixer priority)

### Frontend
- **E01-01** `frontend/src/pages/Board.tsx:29` — Infinite useEffect loop from searchParams dep
- **E01-02** `frontend/src/pages/Workflows.tsx:1119` — null projectId stringified as "null" in URL
- **E01-03** `frontend/src/pages/Board.tsx:48-49` — Stale state init from searchParams
- **E02-01** `frontend/src/pages/ui-new/settings/ProjectSettingsNew.tsx:327` — mutate() not awaited, setSaving never reset
- **E02-02** `frontend/src/pages/ui-new/settings/ProjectSettingsNew.tsx:314-336` — Missing name validation before mutate
- **E02-05** `frontend/src/pages/ui-new/settings/FeishuSettingsNew.tsx:100-103` — No appId format validation
- **E02-07** `frontend/src/pages/ui-new/settings/OrganizationSettingsNew.tsx:532,550,557,575,601` — Multiple sync-called mutations with no pending UI
- **E03-01** `frontend/src/components/NormalizedConversation/PendingApprovalEntry.tsx:286` — Missing `pendingStatus` dep, stale closure
- **E03-04** `frontend/src/components/NormalizedConversation/PendingApprovalEntry.tsx:248-253` — Missing deps cause scope leak
- **E03-06** `frontend/src/components/NormalizedConversation/NextActionCard.tsx:278-288` — handleCopy setTimeout leak
- **E03-09** `frontend/src/components/NormalizedConversation/PendingApprovalEntry.tsx:306-313` — Circular closure in deny flow
- **E04-01..03** `frontend/src/components/diff/DiffCard.tsx:196-233` — Multiple unmemoized callbacks causing re-render storms
- **E05-01** `frontend/src/components/ui/dialog.tsx:112-115` — Backdrop button missing aria
- **E05-02** `frontend/src/components/ui/checkbox.tsx:20-38` — Missing role/aria-checked
- **E05-03** `frontend/src/components/ui/multi-file-search-textarea.tsx:376-388` — Dropdown items missing a11y
- **E06-03** `frontend/src/components/ui-new/primitives/ProcessListItem.tsx:65-95` — Missing keyboard handler
- **E06-09** `frontend/src/components/ui-new/primitives/SearchableDropdown.tsx:108` — Missing Esc/Tab handling
- **E07-01** `frontend/src/components/ui-new/containers/SessionChatBoxContainer.tsx:229` — Missing `lastSessionProcesses` dep
- **E07-02** `frontend/src/components/ui-new/containers/SessionChatBoxContainer.tsx:512,533` — Wrong cache invalidation key
- **E08-01** `frontend/src/components/ui-new/views/ChangesPanel.tsx:83-99` — Empty state layout issue
- **E08-02** `frontend/src/components/ui-new/views/WorkspacesMain.tsx:114` — Unchecked optional callback
- **E08-06** `frontend/src/components/ui-new/views/ConciergeChatView.tsx:291` — Missing whitespace between elements
- **E09-01** `frontend/src/components/board/WorkflowKanbanBoard.tsx:86-109` — Drag-end race with stale tasks
- **E09-02** `frontend/src/hooks/useWorkflows.ts:913-954` — Missing onSuccess in useUpdateTaskStatus
- **E10-01** `frontend/src/components/pipeline/TerminalNode.tsx:76-84` — No backdrop click to close panel
- **E10-02** `frontend/src/components/pipeline/TerminalNode.tsx:49` — No cleanup on unmount
- **E11-01..03** `frontend/src/components/workflow/validators/step*.ts` — Multiple validation bypasses
- **E12-01** `frontend/src/components/dialogs/shared/ConfirmDialog.tsx:66` — onOpenChange always cancels
- **E12-02** `frontend/src/components/dialogs/shared/FolderPickerDialog.tsx:56-66` — Error persists on reopen
- **E12-03** `frontend/src/components/dialogs/tasks/ShareDialog.tsx:145,149` — modal.hide() without modal.resolve()
- **E13-01** `frontend/src/pages/ui-new/settings/GeneralSettingsNew.tsx:419` — Toggle state swapped
- **E13-04** `frontend/src/pages/ui-new/settings/AgentSettingsNew.tsx:931-932` — Form editor toggle inverted
- **E13-10** `frontend/src/pages/ui-new/settings/FeishuSettingsNew.tsx:111-121` — Toggle state not reverted on save failure
- **E14-01** `frontend/src/components/tasks/ClickedElementsBanner.tsx:58` — isExpanded has no setter (dup L12)
- **E14-02** `frontend/src/components/tasks/TaskCard.tsx:67` — Missing dep `isNavigatingToParent`
- **E14-06** `frontend/src/components/panels/TaskAttemptPanel.tsx:29-36` — Missing key on RetryUiProvider
- **E15-02** `frontend/src/components/terminal/TerminalEmulator.tsx:178-217` — FitAddon not unloaded
- **E15-07** `frontend/src/components/terminal/TerminalDebugView.tsx:654` — Terminal key change forces ungraceful WS close
- **E16-01** `frontend/src/components/logs/VirtualizedList.tsx:136` — Loading overlay blocks interaction
- **E16-02** `frontend/src/components/quality/QualityIssueList.tsx:62` — Missing aria-expanded
- **E16-05** `frontend/src/components/setup/SetupWizardStep2Model.tsx:383-387` — Missing key on empty option
- **E17-01** `frontend/src/hooks/useAssigneeUserNames.ts:30` — refetch() stale dep infinite loop
- **E17-02** `frontend/src/hooks/useAttemptExecution.ts:57` — useMemo never memoizes
- **E17-03** `frontend/src/hooks/useDevserverPreview.ts:112` — Missing reset on attemptId change
- **E18-01** `frontend/src/hooks/useImageMetadata.ts:48` — Silent fetch failure returns undefined
- **E18-05** `frontend/src/hooks/useOrganizationSelection.ts:31` — Potential infinite loop
- **E18-09** `frontend/src/hooks/useImageUpload.ts:6-8` — Empty deps on upload callbacks
- **E19-01** `frontend/src/hooks/auth/useAuthStatus.ts:25` — refetch race with observer
- **E19-02** `frontend/src/hooks/auth/useCurrentUser.ts:8` — Missing cleanup on sign-out
- **E19-05** `frontend/src/hooks/useSessionQueueInteraction.ts:73-89` — Missing `sessionId` in deps
- **E19-10** `frontend/src/hooks/useWorkspaceSessions.ts:51-71` — Workspace ID race
- **E20-01..10** `frontend/src/lib/api.ts` + `conciergeApi.ts` — 5 POST endpoints with missing bodies

### Rust
- **E21-01** `crates/services/src/services/orchestrator/agent.rs:1413-1429` — State write-lock held across publish
- **E21-02** `crates/services/src/services/orchestrator/agent.rs:2667-2710` — Deferred completion state race
- **E21-03** `crates/services/src/services/orchestrator/agent.rs:7005-7009` — Handoff note extraction from pre-truncated message
- **E22-01** `crates/services/src/services/config/versions/v4.rs:104` — Wrong log message "v3" instead of "v4"
- **E22-02** `crates/services/src/services/config/versions/v8.rs:71-96` — Three user prefs silently dropped
- **E23-01** `crates/services/src/services/git_watcher.rs:105-106` — Byte slice on trimmed line
- **E23-02** `crates/services/src/services/git_watcher.rs:85` — UTF-8 non-boundary slice
- **E24-01** `crates/services/src/services/git_host/github/cli.rs:310-320` — Unpaginated PR comments (>30 lost)
- **E24-02** `crates/services/src/services/git_host/types.rs:56-65` — Overly aggressive retry storms
- **E25-01** `crates/services/src/services/approvals.rs:147-149` — Race respond() vs timeout_watcher()
- **E25-02** `crates/services/src/services/approvals.rs:225-226` — Orphaned oneshot in timeout
- **E25-05** `crates/server/src/routes/concierge_ws.rs:65,144` — No subscription cleanup on disconnect
- **E25-11** `crates/services/src/services/approvals.rs:224-254` — Timeout skips DB update on store miss
- **E26-01** `crates/services/src/services/terminal/process.rs:835-836` — SIGTERM returns before child dead
- **E26-02** `crates/services/src/services/terminal/process.rs:363-371` — Poisoned mutex recovered unsafely
- **E26-03** `crates/services/src/services/terminal/bridge.rs:443-450` — Writer task completion race drops messages
- **E26-06** `crates/services/src/services/terminal/process.rs:915-925` — Dead processes not removed from HashMap
- **E26-09** `crates/services/src/services/terminal/prompt_watcher.rs:364-369` — Hardcoded menu index "2\r"
- **E26-15** `crates/services/src/services/terminal/process.rs:1476-1477` — Buffer counter race via Ordering::Relaxed
- **E27-01** `crates/services/src/services/chat_connector.rs:105` — Unsafe parse fallback to 0
- **E27-03** `crates/services/src/services/diff_stream.rs:212,226,265,270,485,492,517` — RwLock unwrap panics
- **E27-05** `crates/services/src/services/file_search.rs:637-644` — Watcher task never aborted
- **E27-07** `crates/services/src/services/container.rs:193-197` — Unsafe repo path resolution
- **E27-09** `crates/server/src/routes/subscription_hub.rs:77-82` — Race in replay_pending check (dup M09)
- **E27-13** `crates/services/src/services/file_search.rs:573-579` — Worker creates dummy DashMap, loses watchers
- **E27-15** `crates/server/src/routes/subscription_hub.rs:173-178` — Pending events accumulate indefinitely
- **E28-02** `crates/services/src/services/error_handler.rs:114` — Race on pty_session_id init
- **E28-04** `crates/services/src/services/filesystem.rs:32,49,62,79` — unsafe env mutation in tests
- **E28-08** `crates/services/src/services/feishu.rs:87` — Decrypt closure only processes first secret
- **E28-10** `crates/services/src/services/diff_stream.rs:220,285,331` — Silent channel drops
- **E28-13** `crates/services/src/services/runner_client.rs:100-101` — u16 truncation on dims
- **E29-03,10** `crates/server/src/routes/workflows.rs:3272-3304` — Pagination logic errors
- **E29-05** `crates/server/src/routes/task_attempts/pr.rs:203-211` — Empty title after trim accepted
- **E29-14** `crates/server/src/routes/task_attempts/workspace_summary.rs:78` — Unbounded query
- **E30-01,02** `crates/server/src/routes/models.rs:245,321` — Double response.text() calls return empty
- **E31-01** `crates/server/src/routes/terminals.rs:200` — Silent empty string in find_executable
- **E31-02,03** `crates/server/src/routes/chat_integrations.rs:372,378` — expect() in prod header parsing
- **E32-01** `crates/server/src/mcp/task_server.rs:337-338` — Redundant `.ok()?.ok()?` chain loses errors
- **E32-02** `crates/server/src/mcp/task_server.rs:817` — Typo `repsonse`
- **E33-01** `crates/executors/src/executors/claude/protocol.rs:123` — unwrap on serde
- **E33-02** `crates/executors/src/executors/codex/jsonrpc.rs:97` — Response clone before callback error
- **E33-03** `crates/executors/src/executors/codex/client.rs:487-501` — Fallback RequestId::Integer(0) collides
- **E34-01** `crates/executors/src/executors/cursor/mcp.rs:140` — Unwrap on regex
- **E34-04** `crates/executors/src/executors/opencode/sdk.rs:411-420` — fork_session URL injection
- **E34-08** `crates/executors/src/executors/acp/session.rs:37-47` — Session ID path traversal
- **E34-13** `crates/executors/src/executors/cursor/mcp.rs:166` — Hex substring panic
- **E35-01** `crates/executors/src/executors/droid/normalize_logs.rs:146,171,...` — Many unwrap() after insert
- **E35-02** `crates/executors/src/logs/utils/patch.rs:52,...` — Unwrap on JSON patch serialization
- **E35-05** `crates/executors/src/executors/acp/normalize_logs.rs:444` — Silent JSON parse of title
- **E35-09** `crates/executors/src/profile.rs:197` — RwLock().unwrap() panic on poison
- **E36-01** `crates/quality/src/rules/react_hooks.rs:85` — Scope depth comparison off-by-one
- **E36-04** `crates/quality/src/rules/type_assertion.rs:26` — `async as` false positive
- **E36-06** `crates/quality/src/rules/function_length.rs:127` — Brace depth underflow
- **E36-10** `crates/quality/src/rules/console_usage.rs:74` — is_test_file too broad
- **E36-14** `crates/quality/src/rules/secret_detection.rs:148-165` — Bad regex syntax
- **E37-01** `crates/quality/src/provider/builtin_frontend.rs:116` — Severity comparison ambiguity
- **E37-02** `crates/quality/src/discovery/mod.rs:270` — starts_with path prefix missing trailing slash
- **E37-04** `crates/quality/src/discovery/mod.rs:328-330` — Glob Windows backslash handling
- **E37-07** `crates/quality/src/provider/builtin_frontend.rs:148-152` — Path normalization cache miss
- **E38-01** `crates/db/migrations/20260117000001_create_workflow_tables.sql:107` — workflow.project_id TEXT vs BLOB
- **E38-02** `crates/db/migrations/20260117000001_create_workflow_tables.sql:154` — Missing ON DELETE CASCADE
- **E38-11** `crates/db/migrations/20251209000000_add_project_repositories.sql` — No foreign_key_check before re-enable
- **E39-01,02,03** `crates/local-deployment/src/container.rs:949,954,606-607` — Unwraps/expects on critical paths
- **E40-01** `crates/services/src/services/cli_installer.rs:193` — Spawned task holds mutex indefinitely

## MED / LOW
See individual agent reports (~300 additional findings). Full reports preserved in session.
