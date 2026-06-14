# Census: rs-db-models-A
## Unit: crates/db/src/models/ — alphabetical first half (files 1–15 + workflow.rs, workspace.rs)

Generated: 2026-06-14

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `mod.rs` | Re-exports all sub-modules; glob-re-exports key types | `pub use cli_type::*`, `concierge::*`, `git_event::*`, `orchestrator_message::*`, `quality_*::*`, `system_settings::SystemSetting`, `terminal::*`, `workflow::*` | Root of all model sub-mods | First-class vs. second-class modules reflected by glob-export choices |
| `cli_install_history.rs` | Tracks CLI install/uninstall operations and caches detection results | `CliInstallHistory` (CRUD), `CliDetectionCache` (upsert/get/delete); 7 async fns total | Used by `crates/server/src/routes/cli_types.rs` | Table: `cli_install_history`, `cli_detection_cache` |
| `cli_type.rs` | Stores AI coding agent CLI types (Claude Code, Codex, etc.) and model configs with encrypted API keys | `CliType` (find_all/by_id/by_name), `ModelConfig` (14 fns incl. create_custom, update_credentials, resolve_preferred_or_default), `CliDetectionStatus` (DTO) | 28 callers across server routes, services, cc-switch | Most heavily used type-config module; `ModelConfig.encrypted_api_key` uses AES-GCM via `crate::encryption` |
| `coding_agent_turn.rs` | Records prompt+summary for each coding agent session attached to an execution process; tracks `seen` state for UI badges | `CodingAgentTurn` (7 fns), `CreateCodingAgentTurn` | Used by `container.rs`, `task_attempts.rs`, `workspace_summary.rs`, `local-deployment/container.rs` | Drives "unseen turns" notification badges in frontend |
| `concierge.rs` | Multi-channel AI concierge sessions: session state, channel bindings (Feishu/Web), message history | `ConciergeSession` (15 fns), `ConciergeSessionChannel` (7 fns), `ConciergeMessage` (8 fns) | 11 callers across concierge routes/services, orchestrator agent, feishu service | Feishu-sync flags (`sync_tools`, `sync_terminal`, `sync_progress`) control invisible background push events |
| `execution_process.rs` | Core lifecycle record for every PTY process spawned (coding agent, setup/cleanup scripts, dev server) | `ExecutionProcess` (20+ fns), `ExecutionProcessStatus` enum, `ExecutionProcessRunReason` enum, `UpdateExecutionProcess` (DEAD), `ExecutionContext`, `LatestProcessInfo`, `MissingBeforeContext` | 27+ callers across all server/services/local-deployment | `UpdateExecutionProcess` struct (#[allow(dead_code)]) has 0 production callers — delete candidate |
| `execution_process_logs.rs` | Stores JSONL log chunks for execution processes; paginated read | `ExecutionProcessLogs` (find_by_execution_id, find_by_execution_id_page, parse_logs, append_log_line) | `execution_processes.rs` route, `generate_types.rs`, `container.rs` | W2-15-06: capped at 5000 rows per query to prevent OOM |
| `execution_process_repo_state.rs` | Per-repo git head commit state (before/after/merge) for each execution process | `ExecutionProcessRepoState` (5 fns), `CreateExecutionProcessRepoState` | Called from `execution_process.rs::create`, `container.rs`, `util.rs` | Backfill path `list_missing_before_context` uses a window function (W2-15-05) |
| `feishu_config.rs` | Feishu (Lark) app config with encrypted app secret | `FeishuAppConfig` (insert, find_by_id, find_enabled, find_first, find_all, update_enabled, update_credentials, delete, encrypt_secret) | Used by `feishu.rs` service/route, `health.rs` | Invisible feature: Feishu integration is an optional enterprise channel; E38-15 note on duplicate-prevention unique index |
| `git_event.rs` | Persists commit events detected by GitWatcher | `GitEvent` (insert, update_status, update_metadata, find_by_workflow, new_pending) | Used by `git_watcher.rs`, `event_bridge.rs`, `workflow_events.rs`, `orchestrator::runtime.rs` | E38-13 migration note: terminal FK is SET NULL on delete |
| `image.rs` | Image file metadata and task-to-image association for coding context | `Image` (6 fns incl. find_orphaned_images), `TaskImage` (associate_many_dedup, delete_by_task_id, is_associated), `CreateImage`, `CreateTaskImage` | Used by `image.rs` service, `tasks.rs` route, `images.rs` route | W2-15-02: batch insert uses ON CONFLICT DO NOTHING to avoid N+1 |
| `merge.rs` | Direct and PR merge records for workspace branches | `Merge` (enum: Direct/Pr), `DirectMerge`, `PrMerge`, `PullRequestInfo`, `MergeStatus`, `MergeType` (7 fns including tx variants) | Used by `pr_monitor.rs`, `git_host/*`, `task_attempts/pr.rs`, `workspace_summary.rs` | E38-04: repo FK cascades on delete. Tx-compatible variants for atomic PR create+update (E29-06) |
| `mod.rs` (see above) | — | — | — | — |
| `orchestrator_message.rs` | Workflow orchestrator chat message persistence (messages, commands, external conversation bindings) | `WorkflowOrchestratorMessage` (insert, list_by_workflow_paginated), `WorkflowOrchestratorCommand` (6 fns), `ExternalConversationBinding` (upsert, find_active, find_latest_active, deactivate) | Used by orchestrator runtime, feishu service, chat_integrations route, local-deployment | `recover_incomplete_commands` provides startup crash-recovery for queued/running commands |
| `planning_draft.rs` | G2-CRITICAL: Planning draft lifecycle (gathering→spec_ready→confirmed→materialized). Stores AI-generated spec, audit plan, and Feishu sync config | `PlanningDraft` (13 fns), `PlanningDraftMessage` (insert, list_by_draft), `PLANNING_DRAFT_STATUSES` const | Used by `planning_drafts.rs` route, `orchestrator/llm.rs`, `cc_switch.rs`, `concierge/tools.rs`, `self_test/tests.rs` | Key fields: `audit_plan` (JSON AuditPlan), `audit_mode` (builtin/merged/custom), `audit_doc_path`; confirm→materialize flow creates a Workflow |
| `project.rs` | Project entity (name, default_agent_working_dir) | `Project`, `CreateProject`, `ProjectError` | 30+ callers across all layers | Boundary file (#15) — included for completeness but in second-half territory |
| `workflow.rs` | Multi-terminal orchestrated workflow: status FSM (9 states), tasks, slash commands, API key encryption | `Workflow` (10+ fns incl. CAS transitions), `WorkflowTask` (6 fns), `SlashCommandPreset` (5 fns), `WorkflowCommand` (2 fns), `WorkflowStatus/WorkflowTaskStatus` enums, request DTOs | 35 callers — most widely used model in codebase | CAS guards on terminal states prevent race conditions; `pause_reason` records Quality Gate pause cause (G2/R8); `audit_plan` copied from planning_draft at materialize time |
| `workspace.rs` | Workspace (branch isolation + container ref) with rich status queries and auto-generated names | `Workspace` (20+ fns), `WorkspaceWithStatus`, `WorkspaceContext`, `WorkspaceError`, `ContainerInfo`, `CreatePrParams`, `AttemptResumeContext` | 27 callers across all layers | `find_expired_for_cleanup` drives container lifecycle cleanup; `resolve_container_ref_by_prefix` enables VS Code extension to resolve path→workspace |

---

## Candidates

### 1. `UpdateExecutionProcess` struct (execution_process.rs, lines 101-108)
- **Kind:** dead
- **Evidence:** `#[allow(dead_code)]` suppressor present; Grep across entire repo confirms 0 production callers — only reference is in `docs/R5-dead-code-inventory.md` (already flagged)
- **Why:** The struct was intended to support a PATCH endpoint for execution processes but was never wired up. The `update_completion` fn (line 567) does the actual update directly.
- **Disposition:** delete
- **Confidence:** high
- **Blast radius:** None — no callers exist

### 2. `ExecutionProcessRunReason::QualityScan` variant (execution_process.rs, lines 68-71)
- **Kind:** stub / reserved
- **Evidence:** Grep finds 0 usages of `QualityScan` outside its own definition. Comment says "Reserved for future quality-scan process integration."
- **Why:** Intentionally reserved per P29-G04 design comment. Quality scans use `quality_run` table instead.
- **Disposition:** keep
- **Confidence:** high
- **Blast radius:** If removed, future quality-scan process integration loses taxonomy slot; low immediate impact

### 3. `default_auto_confirm_true` fn (workflow.rs, line 356) — private fn
- **Kind:** dubious-feature / hidden default
- **Evidence:** Only used as `#[serde(default = "default_auto_confirm_true")]` on `CreateTerminalRequest.auto_confirm`; NOT used on `TerminalConfig` (which has `auto_confirm` absent)
- **Why:** If `TerminalConfig` also needs `auto_confirm`, there's a feature gap. Otherwise consistent. The fn itself is rightly private.
- **Disposition:** investigate
- **Confidence:** low
- **Blast radius:** Changing the default would affect all terminals created without explicit `autoConfirm`

---

## Invisible Features

| Name | What It Does | User Visible | Seems Used | Note |
|------|-------------|-------------|-----------|------|
| Feishu sync push | `ConciergeSession.sync_tools/terminal/progress` and `PlanningDraft.feishu_sync` push live events to Feishu/Lark channels | No (enterprise add-on) | Yes — feishu service and concierge notifications wire these | Flag columns; users must explicitly enable in session config |
| VS Code workspace resolver | `Workspace.resolve_container_ref_by_prefix` tries exact path then parent path to handle single-repo subfolders | No (extension API) | Yes — used by container routes | Enables VS Code extension to open correct workspace context from any subfolder |
| Concierge channel binding | `ConciergeSessionChannel` maps external provider+conversation_id to a concierge session; `switch_active_session` for session switching | No (Feishu webhook) | Yes — feishu route and concierge routes | Multi-channel AI assistant sessions across Feishu bots and web UI |
| Workspace auto-name | `Workspace.list_generated_names + persist_generated_names` extracts first user prompt to auto-name workspaces | Indirectly (UI shows name) | Yes — called from `find_all_with_status` | Lazy; writes name back to DB on first list; truncates to 60 chars |
| Container lifetime cleanup | `find_expired_for_cleanup` uses complex HAVING clause to expire containers after 1h (archived/inactive tasks) or 72h (active) | No (background) | Yes — workspace_manager.rs | Invisible TTL-based cleanup of Docker containers |
| `ExternalConversationBinding` | Binds a Feishu/external chat conversation_id to a workflow for orchestrator message bridging | No (webhook plumbing) | Yes — chat_integrations route, feishu route | Enables cross-platform message routing without user awareness |
| `WorkflowOrchestratorCommand.recover_incomplete_commands` | At startup, marks queued/running commands as failed+retryable after crash | No (startup hook) | Yes — called in server startup | Crash-recovery shim for orchestrator command queue |
| `QualityScan` enum variant (reserved) | Reserved slot in `ExecutionProcessRunReason` for future quality-scan PTY process | No | No (not wired) | P29-G04 design: quality scans currently use separate `quality_run` table |
| `planning_draft.audit_plan` + `audit_mode` | Stores AuditPlan JSON generated at confirm time; copied to Workflow.audit_plan at materialization | Indirectly (shown in UI) | Yes — orchestrator/audit_plan.rs, planning_drafts route | G2-critical: bridges planning→execution quality gates; audit_mode controls whether builtin/user-custom/merged policy applies |
