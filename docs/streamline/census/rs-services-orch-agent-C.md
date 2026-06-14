# Census: rs-services-orch-agent-C

Scope: `crates/services/src/services/orchestrator/agent.rs` lines **8200-12172** (file ends 12173).
This is the tail of the `OrchestratorAgent` impl, plus free helper functions, the
R8 quality-loop progress classifier types, and the full `#[cfg(test)] mod tests`.

Tool note: fast-context MCP returned `resource_exhausted` on two consecutive
attempts; cross-file caller verification fell back to repo-wide Grep.

## Module map

| Symbol (line) | Purpose | Public surface | Relations | Notes |
|---|---|---|---|---|
| `ensure_final_repair_task` body tail (8200-8307) | Reuse/create a "final-integration-repair" terminal and dispatch the repair instruction; idempotent over active/startable terminals. | private method on `OrchestratorAgent` | calls `build_final_repair_instruction`, `dispatch_terminal_when_ready_or_queue`, `select_final_repair_runtime`, `runtime_actions().create_terminal`, `dispatch_queued_terminals` | Final Repair (System B adjacent). Driver of the "make deliverable pass final gates" loop. |
| `select_final_repair_runtime` (8309-8327) | Pick cli_type/model from a non-repair existing terminal to reuse runtime for repair. | private | reads `WorkflowTask`/`Terminal` models | — |
| `build_final_repair_instruction` (8329-8387) | Compose the LLM instruction text for the final-repair terminal (goal, task summary, failing gate, blocking evidence, metadata block). | private | uses `GIT_COMMIT_METADATA_SEPARATOR`, `is_final_repair_task` | — |
| `publish_repairing_final_issues_status` (8389-8407) | Emit a `StatusUpdate(running)` bus event when repairing final issues. | private | `message_bus.publish_workflow_event` | Verify caller (likely the final-gate path < L8200). |
| `is_final_repair_task` (8409-8413) | Identify the synthetic final-repair task by name. | `Self::` assoc fn | `FINAL_REPAIR_TASK_NAME` | used in 8309/8341 etc. |
| `final_repair_fingerprint` / `final_repair_marker` / `normalized_final_repair_reason` / `stable_short_hash` / `final_repair_reason_with_escalation` (8415-8457) | Deterministic fingerprint + escalation copy for repeated final blockers (FNV-1a hash). | `Self::` assoc fns | tested at 10648-10668 | — |
| `artifact_readiness_issues` + `has_non_empty_file` / `has_non_empty_ci_workflow` / `has_test_artifact` / `scan_for_test_artifact` (8459-8550) | Greenfield (full-delivery) artifact gate: require README, Dockerfile, CI workflow, a test artifact. | `Self::` assoc fns | gated by `WorkflowStrategy::is_full_delivery`; tested 10613-10646 | Part of final readiness gates (System A adjacent). |
| `reconcile_workflow_completion_from_runtime` (8561-8566) | Public-crate entry that delegates to `auto_sync_workflow_completion`. | `pub(crate)` | called by `runtime.rs:858,878` | Live. TOCTOU note documented inline (G15-011). |
| `auto_sync_workflow_completion` (8568-8750) | Core auto-completion state machine: only when running, no runnable terminals, planning complete, LLM idle, all tasks completed, no unresolved enforce blockers, final readiness gates pass → CAS running→completed, then optional auto-merge. | private | `workflow_has_unresolved_enforce_blockers`, `ensure_final_repair_task`, `run_final_readiness_gates`, `execute_auto_merge`, `mark_workflow_failed` | Central completion logic. Gate-system relevant. |
| `execute_auto_merge` (8754-8779) | Collect completed task branches and delegate to `trigger_merge`. | private | `trigger_merge`, `resolve_project_working_dir` | gated by `config.auto_merge_on_completion`. |
| `trigger_merge` (8803-9006) | Merge all completed task branches into target via `MergeCoordinator`; idempotency, no-worktree skip, partial-failure status, worktree cleanup. | `pub` | called by `auto_sync_workflow_completion` (8773) and self at 6607 (out of scope); uses `MergeCoordinator`, `GitService`, `WorktreeManager` | Live. G06-003..009 fixes. |
| `refresh_pending_task_branches` (9011-9099) | After merge, delete stale pending-task branches with no unique commits so they re-fork from updated target. | private | called at agent.rs:6620 (out of scope) | Live (in-file caller). |
| `handle_terminal_failure` (9105-9119) | Public wrapper that resolves workflow_id then calls `error_handler.handle_terminal_failure`. | `pub` | inner `ErrorHandler::handle_terminal_failure` | **Zero external callers** repo-wide — dead public wrapper. Candidate. |
| `handle_user_prompt_response` (9125-9176) | Resolve terminal/session, validate workflow ownership, delegate to `PromptHandler`. | `pub` | called by `runtime.rs:576`; `prompt_handler.handle_user_prompt_response` | Live. |
| `submit_orchestrator_chat_message` (9182-9210) | User chat → call_llm → execute_instruction; toggles run_state. | `pub` | called by `runtime.rs:650` | Live. |
| `get_conversation_history` (9213-9216) | Snapshot conversation history. | `pub` | `runtime.rs:696` | Live. |
| `get_provider_status` (9219-9221) | Live provider status from LLM client. | `pub` | `runtime.rs:907` → route `provider_health.rs:58` | Live (HTTP `/{workflow_id}/providers/status`). |
| `reset_provider` (9224-9226) | Reset provider circuit breaker. | `pub` | `runtime.rs:926` → route `provider_health.rs:114` | Live (HTTP POST). |
| `execute_slash_commands` (9236-9346) | Render + send configured workflow slash commands to LLM at agent start. | `pub` | called at agent.rs:985 (out of scope); models: WorkflowCommand/SlashCommandPreset; `TemplateRenderer`, `WorkflowContext` | Live. Invisible feature (slash-command pipeline, gated by `workflow.use_slash_commands`). |
| `fetch_diff_for_review` (9349-9366) | Truncated `git diff --stat` for a commit. | private | called at agent.rs:6449 (out of scope) | Live. |
| `truncate_with_marker` (9370-9377) | Truncate string with `[...truncated]` marker (UTF-8 boundary safe). | free fn (module) | used by completion-context helpers | Live. |
| `TerminalGateChangedFiles` enum + impl (9379-9421) | Scoped/Unscoped changed-file set for terminal quality gate. | module-private | `from_collection_result`/`as_deref` are `#[cfg(test)]`; `scope_label`/`count` used by gate code | Two methods test-only. |
| `collect_changed_files_for_quality_gate` (9423-9453) | `git diff --name-only base..head`. | free fn | tested 10519-10551 | Live. |
| JS bootstrap block: `JS_BOOTSTRAP_LOCKS`/`JS_BOOTSTRAP_CACHE`/`get_bootstrap_lock`/`JsPackageManager`/`ensure_js_deps_installed_for_gate`/`reset_js_bootstrap_cache_for_test` (9469-9642) | R4 Fix A: pre-gate `npm/pnpm/yarn/bun install` when package.json present and node_modules missing; per-worktree serialized, cached, Windows-aware exe resolution. | free fns + statics | `quality::discovery::resolve_node_exe`; tested 11064-11126 | `reset_js_bootstrap_cache_for_test` is `#[cfg(test)]`. Invisible infra feature. |
| `is_quality_run_only_infra_blockers` (9650-9674) | R4 Fix B: classify a run as environment-only iff every blocking issue rule_id ends `::unavailable`. | free fn | `quality::provider::frontend::UNAVAILABLE_RULE_SUFFIX`; tested 11703-11762 | Live (call site < L8200). |
| `terminal_has_unresolved_enforce_blockers` / `workflow_has_unresolved_enforce_blockers` (9687-9717) | R8-B3: block premature completion when latest enforce run still has unresolved blockers. | free fns | called by `auto_sync_workflow_completion` (8660); tested 12007 | Live. |
| `BlockerKey` / `BlockerMultiset` (9737-9806) | R8 per-blocker identity + canonical multiset with progress relations (`same_keys`, `is_strict_shrink_of`, `total`, `unique_count`, `fingerprint_string`). | `pub(crate)` | used by classifier; tested 12144 | Live. |
| `compute_blocker_multiset` (9809-9830) | Build multiset from a run's blocking issues. | free fn | tested broadly | Live. |
| `LoopProgressClass` + `LOOP_MIN_HISTORY`/`LOOP_PAUSE_PLATEAU_ROUNDS`/`LOOP_PAUSE_REGRESSION_ROUNDS` + `classify_loop_progress` + `loop_progress_hint` (9834-10033) | R8 progress classification (FirstFew/MakingProgress/Plateau/Regression) replacing R4 Fix C N-failure escalation; never auto-fails, only pauses at thresholds. | `pub(crate)` + free fns | consumed by `handle_quality_gate_result` (< L8200); tested 11766-11951 | Live. System A quality-loop core. |
| `fetch_terminal_completion_context` (10039-10154) | Build completion context (log summary, diff stat, commit body, changed-file content) for LLM completion/acceptance prompts. | free fn | `compute_merge_base`, `collect_changed_files_content`; called at 5009/5115 | Live. Feeds acceptance review (System B). |
| `compute_merge_base` (10157-10177) | `git merge-base target task`. | free fn | used above | Live. |
| `current_git_head` (10179-10195) | `git rev-parse HEAD`. | free fn | used by acceptance-review invalidation (< L8200) | Live. |
| `same_commit_ref` (10197-10204) | Compare two commit refs incl. abbreviated prefixes. | free fn | acceptance-review invalidation | Live. |
| `collect_changed_files_content` (10209-10337) | Read content of changed source/config files for review, prioritized + byte-capped. | free fn | used by `fetch_terminal_completion_context` | Live. |
| `fetch_previous_terminal_context` (10341-10403) | Pull prior terminal's role/status/commit/handoff for the next terminal. | free fn | called at agent.rs:4266; `extract_handoff_notes` | Live. |
| `extract_handoff_notes` (10409-10434) | Parse HANDOFF markers / strip metadata block from a commit message. | free fn | tested indirectly | Live. E21-03/E21-12 fixes. |
| `#[cfg(test)] mod tests` (10436-12172) | Unit/integration tests: changed-file collection, archetype detect, artifact readiness, final-repair fingerprint, instruction builders, stall recovery, JS bootstrap, R4 Fix B, R8 classifier + multiset, acceptance-review gate (System B), queued dispatch. | test-only | exercises System A + System B paths | Large test block. |

## Invisible features in scope
- **Final Integration Repair loop** (8200-8457): synthetic `final-integration-repair` terminal/task auto-created to fix branch/repo/artifact gate failures, with FNV-1a fingerprint dedupe + escalation copy. Not surfaced as a distinct UI feature.
- **Greenfield artifact readiness gate** (8459-8550): for full-delivery strategy, hard-requires README + Dockerfile + CI workflow + a test artifact before auto-completion.
- **Auto-merge on completion** (8716-8779, 8803-9006): when `config.auto_merge_on_completion`, completed task branches are squash-merged into target via MergeCoordinator with partial-failure tracking and worktree cleanup. Background capability gated by config.
- **JS dep pre-bootstrap (R4 Fix A)** (9469-9642): silent `npm/pnpm/yarn/bun install` before the quality gate fires to avoid spurious `*::unavailable`.
- **R8 quality-loop progress classifier** (9719-10033): per-terminal multiset fingerprinting that distinguishes real progress from plateau/regression and only pauses (never auto-fails) at thresholds. Drives the fix-prompt feedback the user sees.
- **Slash-command execution pipeline** (9236-9346): renders DB-configured workflow slash commands into LLM messages at startup, gated by `workflow.use_slash_commands`.

## Quality Gate system relevance
- **System A (three-layer quality gate)**: `auto_sync_workflow_completion` consumes `run_final_readiness_gates` + `workflow_has_unresolved_enforce_blockers`; R8 classifier (`classify_loop_progress`, `LoopProgressClass`, multisets) is the in-loop enforce-mode feedback engine; `is_quality_run_only_infra_blockers` separates env from code blockers.
- **System B (acceptance review / scoring, ~L5300-5450, out of scope)**: in scope we see its support code — `fetch_terminal_completion_context` / `collect_changed_files_content` feed the review prompt, and tests `run_acceptance_review_gate_*` (11320-11621) exercise it (rejection → review_pending, LLM failure → review_pending, new-commit-during-review invalidation).
