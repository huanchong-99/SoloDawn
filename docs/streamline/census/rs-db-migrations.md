# rs-db-migrations Census

Unit: rs-db-migrations  
Scope: `crates/db/migrations/` (105 SQL files) + `crates/db/benches/workflow_bench.rs`  
Branch: refactor/streamline-quality-gates  
Date: 2026-06-14

---

## Migration History Overview

The migrations evolve the SQLite schema from the initial kanban-style task tracker through a major renaming (task_attempts → workspaces/sessions) to the full multi-terminal workflow/orchestrator system.

---

## File Map

| File | Timestamp | Purpose | Tables Affected | Notes |
|---|---|---|---|---|
| `20250617183714_init.sql` | 2025-06-17 | Foundation: projects, tasks, task_attempts, task_attempt_activities | CREATE 4 tables | Superseded columns (stdout/stderr on task_attempts) removed by later migration |
| `20250620212427_execution_processes.sql` | 2025-06-20 | Add execution_processes table | CREATE execution_processes | Initial version with command/args/working_dir/stdout/stderr; all later dropped |
| `20250620214100_remove_stdout_stderr_from_task_attempts.sql` | 2025-06-20 | Remove stdout/stderr from task_attempts | ALTER task_attempts (rebuild) | Supercedes init.sql columns |
| `20250621120000_relate_activities_to_execution_processes.sql` | 2025-06-21 | Rewire activities to execution_processes | DROP+CREATE task_attempt_activities | Intermediate table; entire table dropped in 20250717 |
| `20250623120000_executor_sessions.sql` | 2025-06-23 | Create executor_sessions table | CREATE executor_sessions | Renamed to coding_agent_turns in 20251216 big refactor |
| `20250623130000_add_executor_type_to_execution_processes.sql` | 2025-06-23 | Add executor_type to execution_processes | ALTER execution_processes | Column later dropped in 20250805 |
| `20250625000000_add_dev_script_to_projects.sql` | 2025-06-25 | Add dev_script to projects | ALTER projects | Migrated to repos table in 20260107 |
| `20250701000000_add_branch_to_task_attempts.sql` | 2025-07-01 | Add branch column | ALTER task_attempts | Made nullable in 20250726, non-null again in 20250923 |
| `20250701000001_add_pr_tracking_to_task_attempts.sql` | 2025-07-01 | PR tracking columns on task_attempts | ALTER task_attempts (4 cols) | All moved to merges table in 20250819 |
| `20250701120000_add_assistant_message_to_executor_sessions.sql` | 2025-07-01 | Add summary to executor_sessions | ALTER executor_sessions | Misnamed (adds summary, not assistant_message) |
| `20250708000000_add_base_branch_to_task_attempts.sql` | 2025-07-08 | Add base_branch (later renamed target_branch) | ALTER task_attempts | Renamed to target_branch in 20250923, dropped in 20251209 |
| `20250709000000_add_worktree_deleted_flag.sql` | 2025-07-09 | Add worktree_deleted flag | ALTER task_attempts | Dropped in 20251215 |
| `20250710000000_add_setup_completion.sql` | 2025-07-10 | Add setup_completed_at | ALTER task_attempts | Still present in workspaces model |
| `20250715154859_add_task_templates.sql` | 2025-07-15 | Create task_templates table | CREATE task_templates | Entire table replaced by tags in 20251020 |
| `20250716143725_add_default_templates.sql` | 2025-07-16 | Seed 3 default templates | INSERT task_templates | Content migrated to tags in 20251020 |
| `20250716161432_update_executor_names_to_kebab_case.sql` | 2025-07-16 | Data migration: snake_case/camelCase → kebab-case executor names | UPDATE task_attempts, execution_processes | One-time data fix |
| `20250716170000_add_parent_task_to_tasks.sql` | 2025-07-16 | Add parent_task_attempt FK to tasks | ALTER tasks | Renamed to parent_workspace_id in 20251216 |
| `20250717000000_drop_task_attempt_activities.sql` | 2025-07-17 | Drop task_attempt_activities | DROP task_attempt_activities | Final removal of activity tracking |
| `20250719000000_add_cleanup_script_to_projects.sql` | 2025-07-19 | Add cleanup_script to projects | ALTER projects | Migrated to repos table in 20260107 |
| `20250720000000_add_cleanupscript_to_process_type_constraint.sql` | 2025-07-20 | Add 'cleanupscript' to CHECK constraint | ALTER execution_processes (rebuild via new column) | Expands process_type enum |
| `20250726182144_update_worktree_path_to_container_ref.sql` | 2025-07-26 | Rename worktree_path → container_ref | ALTER task_attempts | Semantic rename |
| `20250726210910_make_branch_optional.sql` | 2025-07-26 | Make branch nullable | ALTER task_attempts (rebuild) | Reverted to non-null in 20250923 |
| `20250727124142_remove_command_from_execution_process.sql` | 2025-07-27 | Drop command/args columns | ALTER execution_processes | Schema cleanup |
| `20250727150349_remove_working_directory.sql` | 2025-07-27 | Drop working_directory column | ALTER execution_processes | Schema cleanup |
| `20250729162941_create_execution_process_logs.sql` | 2025-07-29 | Create execution_process_logs (JSONL) | CREATE execution_process_logs | PK dropped in 20251101 |
| `20250729165913_remove_stdout_and_stderr_from_execution_processes.sql` | 2025-07-29 | Drop stdout/stderr from execution_processes | ALTER execution_processes | Replaced by logs table |
| `20250730000000_add_executor_action_to_execution_processes.sql` | 2025-07-30 | Add executor_action JSON column + backfill | ALTER execution_processes | Complex backfill of legacy rows |
| `20250730000001_rename_process_type_to_run_reason.sql` | 2025-07-30 | Rename process_type → run_reason | ALTER execution_processes | Semantic rename |
| `20250730124500_add_execution_process_task_attempt_index.sql` | 2025-07-30 | Add virtual column executor_action_type + index | ALTER execution_processes | Fixed by next migration (wrong JSON path) |
| `20250805112332_add_executor_action_type_to_task_attempts.sql` | 2025-08-05 | Drop executor_type; rename executor → base_coding_agent | ALTER execution_processes, task_attempts | Semantic rename |
| `20250805122100_fix_executor_action_type_virtual_column.sql` | 2025-08-05 | Fix virtual column JSON path ($.type → $.typ.type) | DROP+recreate index/column on execution_processes | Bug fix |
| `20250811000000_add_copy_files_to_projects.sql` | 2025-08-11 | Add copy_files to projects | ALTER projects | Migrated to repos table in 20260107 |
| `20250813000001_rename_base_coding_agent_to_profile.sql` | 2025-08-13 | Rename base_coding_agent → profile; map enum values | ALTER task_attempts | Further renamed to executor in 20250902 |
| `20250815100344_migrate_old_executor_actions.sql` | 2025-08-15 | Fix executor_action JSON format for old rows | UPDATE execution_processes | Data fix |
| `20250818150000_refactor_images_to_junction_tables.sql` | 2025-08-18 | Create images + task_images junction tables | CREATE images, task_images | Image subsystem; actively used |
| `20250819000000_move_merge_commit_to_merges_table.sql` | 2025-08-19 | Create merges table; migrate PR + commit data from task_attempts | CREATE merges; ALTER task_attempts (drop 5 cols) | Major restructuring |
| `20250902120000_add_masked_by_restore_to_execution_processes.sql` | 2025-09-02 | Add 'dropped' boolean flag to execution_processes | ALTER execution_processes | Used for timeline masking |
| `20250902184501_rename-profile-to-executor.sql` | 2025-09-02 | Rename profile → executor on task_attempts | ALTER task_attempts | |
| `20250903091032_executors_to_screaming_snake.sql` | 2025-09-03 | Convert executor values to SCREAMING_SNAKE via recursive CTE | UPDATE task_attempts | One-time data transform |
| `20250905090000_add_after_head_commit_to_execution_processes.sql` | 2025-09-05 | Add after_head_commit | ALTER execution_processes | Moved to execution_process_repo_states in 20251209 |
| `20250906120000_add_follow_up_drafts.sql` | 2025-09-06 | Create follow_up_drafts table | CREATE follow_up_drafts | Migrated into unified drafts in 20250921; then dropped in 20251129 |
| `20250910120000_add_before_head_commit_to_execution_processes.sql` | 2025-09-10 | Add before_head_commit + backfill | ALTER execution_processes | Moved to execution_process_repo_states in 20251209 |
| `20250917123000_optimize_selects_and_cleanup_indexes.sql` | 2025-09-17 | Performance indexes for task_attempts, execution_processes, tasks | CREATE 3 indexes, DROP 1 | Maintenance |
| `20250921222241_unify_drafts_tables.sql` | 2025-09-21 | Unify follow_up_drafts + retry_drafts → drafts | CREATE drafts; DROP follow_up_drafts, retry_drafts | Entire drafts table dropped in 20251129 |
| `20250923000000_make_branch_non_null.sql` | 2025-09-23 | Make branch NOT NULL; rename base_branch → target_branch | ALTER task_attempts (rebuild) | |
| `20251020120000_convert_templates_to_tags.sql` | 2025-10-20 | Replace task_templates → tags table | CREATE tags; DROP task_templates | Major redesign |
| `20251101090000_drop_execution_process_logs_pk.sql` | 2025-11-01 | Drop PK from execution_process_logs (allow multiple rows per execution) | Rebuild execution_process_logs | Complex 12-step SQLite ALTER workaround |
| `20251114000000_create_shared_tasks.sql` | 2025-11-14 | Create shared_tasks, shared_activity_cursors; link to tasks/projects | CREATE 2 tables; ALTER tasks, projects | Electric-sync tables; shared_tasks dropped in 20251202 |
| `20251120000001_refactor_to_scratch.sql` | 2025-11-20 | Create scratch key-value store | CREATE scratch | Actively used (sessions queue, etc.) |
| `20251129155145_drop_drafts_table.sql` | 2025-11-29 | Drop drafts table | DROP drafts | No down.sql; W2-38-08 data loss warning noted |
| `20251202000000_migrate_to_electric.sql` | 2025-12-02 | Drop shared_activity_cursors, shared_tasks; migrate shared_task_id | DROP 2 tables; ALTER tasks | W2-38-02 breaking change noted |
| `20251206000000_add_parallel_setup_script_to_projects.sql` | 2025-12-06 | Add parallel_setup_script to projects | ALTER projects | Migrated to repos table in 20251209 |
| `20251209000000_add_project_repositories.sql` | 2025-12-09 | Major: Create repos, project_repos, attempt_repos, execution_process_repo_states; migrate data | CREATE 4 tables; ALTER merges, execution_processes, task_attempts; REBUILD projects | Multi-repo architecture |
| `20251215145026_drop_worktree_deleted.sql` | 2025-12-15 | Drop worktree_deleted column | ALTER task_attempts | W2-38-09 data loss warning noted |
| `20251216000000_add_dev_script_working_dir_to_projects.sql` | 2025-12-16 | Add dev_script_working_dir to projects | ALTER projects | |
| `20251216142123_refactor_task_attempts_to_workspaces_sessions.sql` | 2025-12-16 | Rename task_attempts → workspaces; create sessions; migrate execution_processes; rename executor_sessions → coding_agent_turns; rename attempt_repos → workspace_repos | Massive rebuild | Central naming refactor; W2-38-05 UUID risk noted |
| `20251219000000_add_agent_working_dir_to_projects.sql` | 2025-12-19 | Add default_agent_working_dir to projects; add agent_working_dir to workspaces | ALTER projects, workspaces | |
| `20251219164205_add_missing_indexes_for_slow_queries.sql` | 2025-12-19 | Add 4 missing performance indexes; PRAGMA optimize | CREATE 4 indexes | |
| `20251220134608_fix_session_executor_format.sql` | 2025-12-20 | Strip variant suffix from sessions.executor (data fix) | UPDATE sessions | W2-38-06 data coercion risk noted |
| `20251221000000_add_workspace_flags.sql` | 2025-12-21 | Add archived/pinned/name to workspaces | ALTER workspaces | |
| `20260107000000_move_scripts_to_repos.sql` | 2026-01-07 | Migrate scripts from project_repos/projects → repos | ALTER repos; UPDATE | Consolidation |
| `20260107115155_add_seen_to_coding_agent_turns.sql` | 2026-01-07 | Add 'seen' flag to coding_agent_turns | ALTER coding_agent_turns | |
| `20260112160045_add_composite_indexes_for_performance.sql` | 2026-01-12 | Add 2 composite indexes for workspace_repos and merges | CREATE 2 indexes | |
| `20260117000001_create_workflow_tables.sql` | 2026-01-17 | Create workflow system: cli_type, model_config, slash_command_preset, workflow, workflow_command, workflow_task, terminal, terminal_log, git_event | CREATE 9 tables + seed data | Workflow feature entry point |
| `20260119000000_encrypt_api_keys.sql` | 2026-01-19 | Add orchestrator_api_key_encrypted column (old key left) | ALTER workflow | Incomplete: old column not dropped |
| `20260119000001_add_performance_indexes.sql` | 2026-01-19 | Add ~12 partial/composite indexes for workflow tables | CREATE ~12 indexes | |
| `20260119000002_add_workflow_project_created_index.sql` | 2026-01-19 | Add idx_workflow_project_created | CREATE 1 index | |
| `20260125000000_add_orchestrator_state.sql` | 2026-01-25 | Add orchestrator_state TEXT to workflow | ALTER workflow | For crash recovery |
| `20260125000001_add_terminal_session_binding.sql` | 2026-01-25 | Add session_id, execution_process_id to terminal; add terminal_id to sessions | ALTER terminal, sessions | |
| `20260202090000_fix_workflow_project_id_type.sql` | 2026-02-02 | Fix workflow.project_id TEXT → BLOB; rebuild table | Rebuild workflow | W2-38-04 partial rollback risk noted |
| `20260206000000_add_auto_confirm_to_terminal.sql` | 2026-02-06 | Add auto_confirm column (default 0) | ALTER terminal | Default flipped to 1 in next migration |
| `20260208010000_backfill_terminal_auto_confirm.sql` | 2026-02-08 | Backfill auto_confirm=1 for all; clean orphan FKs; rebuild terminal table | Rebuild terminal | Default changed to 1 |
| `20260208020000_fix_terminal_old_foreign_keys.sql` | 2026-02-08 | Rebuild terminal_log and git_event with proper CASCADE FKs | Rebuild terminal_log, git_event | FK fix |
| `20260224000000_add_git_watcher_enabled.sql` | 2026-02-24 | Add git_watcher_enabled to workflow | ALTER workflow | |
| `20260224001000_backfill_workflow_api_key_encrypted.sql` | 2026-02-24 | Add encrypted column again + triggers to mirror keys | ALTER workflow; CREATE 2 triggers | Duplicate of 20260119000000 column; triggers keep both in sync |
| `20260306100000_add_workflow_execution_mode_and_goal.sql` | 2026-03-06 | Add execution_mode, initial_goal to workflow | ALTER workflow | Dual-mode support |
| `20260307120000_add_orchestrator_chat_persistence.sql` | 2026-03-07 | Create orchestrator persistence: workflow_orchestrator_message, workflow_orchestrator_command, external_conversation_binding | CREATE 3 tables | Orchestrator chat replay/audit |
| `20260307200000_add_planning_draft.sql` | 2026-03-07 | Create planning_draft + planning_draft_message | CREATE 2 tables | Confirm→materialize flow (in-flight: System B) |
| `20260311120000_add_feishu_connector.sql` | 2026-03-11 | Create feishu_app_config table | CREATE 1 table | Feishu integration |
| `20260312130000_create_quality_gates.sql` | 2026-03-12 | **Intentionally empty** | None | Superseded by 20260312140000 due to wrong FK table names |
| `20260312140000_create_quality_tables.sql` | 2026-03-12 | Create quality_run + quality_issue | CREATE 2 tables | Quality Gate System A persistence |
| `20260313100000_create_quality_policy_snapshot.sql` | 2026-03-13 | Create quality_policy_snapshot | CREATE 1 table | Config snapshot at run time |
| `20260315000001_create_cli_install_history.sql` | 2026-03-15 | Create cli_install_history + cli_detection_cache | CREATE 2 tables | CLI management tracking |
| `20260315064003_feishu_app_config_unique_app_id.sql` | 2026-03-15 | Add UNIQUE index on feishu_app_config.app_id | CREATE 1 index | |
| `20260315120000_add_merges_unique_constraint.sql` | 2026-03-15 | Add unique index preventing duplicate PR records per workspace | CREATE 1 index | |
| `20260316100000_create_system_settings.sql` | 2026-03-16 | Create system_settings key-value store; seed feishu_enabled + setup_complete | CREATE 1 table | Feature flag store |
| `20260316100000_create_system_settings.down.sql` | 2026-03-16 | Drop system_settings | DOWN migration | One of few .down.sql files |
| `20260319100000_add_execution_process_indexes.sql` | 2026-03-19 | Add idx_ep_session_created composite index | CREATE 1 index | |
| `20260319110000_add_workflow_pause_reason.sql` | 2026-03-19 | Add pause_reason to workflow | ALTER workflow | |
| `20260320100000_add_model_config_credentials.sql` | 2026-03-20 | Add encrypted_api_key, base_url, api_type to model_config | ALTER model_config | Workspace-mode CLI auth |
| `20260321120000_add_session_model_config_id.sql` | 2026-03-21 | Add model_config_id to sessions | ALTER sessions | |
| `20260322200000_create_concierge_tables.sql` | 2026-03-22 | Create concierge_session, concierge_session_channel, concierge_message | CREATE 3 tables | Concierge agent (Feishu+Web chat) |
| `20260322210000_fix_concierge_session_fk.sql` | 2026-03-22 | Rebuild concierge_session to remove FK type mismatch on active_project_id | Rebuild concierge_session | FK type bug fix |
| `20260324112512_create_workflow_event.up.sql` | 2026-03-24 | Create workflow_event table | CREATE 1 table | Event audit log for orchestrator |
| `20260324112512_create_workflow_event.down.sql` | 2026-03-24 | Drop workflow_event | DOWN migration | |
| `20260324150000_add_planning_draft_feishu_sync.up.sql` | 2026-03-24 | Add feishu_sync + feishu_chat_id to planning_draft | ALTER planning_draft | |
| `20260324150000_add_planning_draft_feishu_sync.down.sql` | 2026-03-24 | Remove feishu_sync + feishu_chat_id from planning_draft | DOWN via table rebuild | |
| `20260324160000_add_sync_toggles.up.sql` | 2026-03-24 | Add sync_tools, sync_terminal, sync_progress, notify_on_completion to concierge_session + planning_draft | ALTER concierge_session, planning_draft | Feishu notification toggles |
| `20260324160000_add_sync_toggles.down.sql` | 2026-03-24 | No-op comment (columns remain, SQLite no DROP COLUMN older) | No change | Incomplete down migration |
| `20260325100000_add_concierge_feishu_chat_id.up.sql` | 2026-03-25 | Add feishu_chat_id to concierge_session | ALTER concierge_session | |
| `20260325100000_add_concierge_feishu_chat_id.down.sql` | 2026-03-25 | Drop feishu_chat_id from concierge_session | ALTER concierge_session DROP COLUMN | |
| `20260417010000_add_perf_indexes.sql` | 2026-04-17 | Add 3 performance indexes (execution_processes, tasks.shared_task_id, concierge_session) | CREATE 3 indexes | |
| `20260417020000_cascade_merges_repo_fk.sql` | 2026-04-17 | Rebuild merges with ON DELETE CASCADE on repo_id | Rebuild merges | FK enforcement fix |
| `20260417020002_set_null_git_event_terminal_fk.sql` | 2026-04-17 | Rebuild git_event with ON DELETE SET NULL on terminal_id | Rebuild git_event | FK behavior fix |
| `20260508000000_add_audit_plan.sql` | 2026-05-08 | Add audit_plan, audit_mode, audit_doc_path to planning_draft; add audit_plan to workflow | ALTER planning_draft, workflow | AuditPlan System B |

---

## Bench File

| File | Purpose | Public Surface | Notes |
|---|---|---|---|
| `workflow_bench.rs` | Criterion benchmarks for Workflow SQLite queries | `bench_find_by_id`, `bench_find_by_project`, `bench_find_by_project_with_status` (registered in criterion_group) | `_unused_keep_old_find_by_id_setup` is orphaned (not in criterion_group, decorated with `#[allow(dead_code)]`). 5 open TODOs (W2-05-04..08) for schema drift and data distribution. |

---

## Key Candidates for Deletion / Investigation

| Path | Kind | Evidence | Disposition |
|---|---|---|---|
| `benches/workflow_bench.rs:231-342` `_unused_keep_old_find_by_id_setup` | dead | `#[allow(dead_code)]`; not in `criterion_group!`; only reference is its own definition | delete |
| `20260312130000_create_quality_gates.sql` | stub | Intentionally empty; comment states original had wrong FK refs | keep (must not be removed — sqlx checksums) |
| `20260119000000_encrypt_api_keys.sql` | legacy | Adds `orchestrator_api_key_encrypted` but leaves old `orchestrator_api_key` with commented-out DROP | investigate |
| `20260224001000_backfill_workflow_api_key_encrypted.sql` | redundant | Adds same `orchestrator_api_key_encrypted` column again (idempotent via IF NOT EXISTS equivalent logic); mirrors with triggers | investigate |
| `20260324160000_add_sync_toggles.down.sql` | stub | Explicitly does nothing; columns remain permanently | keep (noted limitation) |
