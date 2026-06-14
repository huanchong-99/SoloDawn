# Gap-fill Census: crates/services — build.rs and 11 service modules

Unit: gapfill  
Date: 2026-06-14

## build.rs

Single-line build script that compiles `proto/runner.proto` via `tonic_build`.
The proto file exists at `E:\SoloDawn\proto\runner.proto` and is also compiled
by `crates/runner/build.rs`. Output feeds `services/src/services/runner_client.rs`.

## analytics.rs

PostHog telemetry wrapper. Exports `AnalyticsConfig`, `AnalyticsService`,
`AnalyticsContext`, `generate_user_id`. Callers: `crates/local-deployment/src/lib.rs`,
`crates/deployment/src/lib.rs`, `crates/services/src/services/pr_monitor.rs`.
`AnalyticsContext` is a thin holder struct combining `user_id + AnalyticsService`;
no additional logic.

**Candidate**: `AnalyticsContext` struct is a trivial pair with no methods —
callers that need both fields could hold them directly. Low-confidence (may be
a stable public API surface).

## approvals.rs

Tool-call approval gating. Exports `Approvals`, `ApprovalError`, `ToolContext`,
`ensure_task_in_review` (pub(crate)), `executor_approvals` sub-module.
Consumed by `crates/server/src/routes/approvals.rs` (HTTP respond endpoint),
`crates/local-deployment/src/lib.rs`, and `executor_approvals.rs` (bridge for
executor-side approval requests).

`get_pending_execution_process_ids` is used by
`crates/server/src/routes/task_attempts/workspace_summary.rs`.

## auth.rs

Thin wrapper around `OAuthCredentials` + a cached profile `RwLock`. Exports
`AuthContext`. Used only by `crates/local-deployment/src/lib.rs` (and referenced
as a trait in `crates/deployment/src/lib.rs`).

**Candidate**: `refresh_guard`, `save_credentials`, `get_credentials`,
`clear_credentials`, `set_profile` — none of these appear to be called outside
`auth.rs` itself in the current grep. Only `cached_profile` and `clear_profile`
are invoked from `local-deployment/src/lib.rs`. The other five methods are dead
unless called by a non-Rust layer (impossible here). Confidence: medium —
the deployment crate trait bound may be only the struct, not all methods.

## cli_health_monitor.rs

Background periodic CLI detection service. Exports `CliHealthMonitor`,
`SharedCliHealthMonitor`, `CliStatusChange`, `CachedCliStatus`.
Consumed by `crates/server/src/routes/cli_status_sse.rs` (SSE stream) and
`crates/local-deployment/src/lib.rs`.

**Note**: Contains a `TODO` about persisting to `cli_detection_cache` DB table
(which does not exist in migrations — only `cli_install_history` exists).
The persistence path is unimplemented dead code inside the TODO comment.

## cli_installer.rs

Spawns per-platform install scripts and streams output via mpsc channel. Exports
`CliInstaller`, `InstallOutputLine`, `InstallOutputStream`. Consumed only by
`crates/server/src/routes/cli_types.rs`.

## diff_stream.rs

Real-time git-diff streaming via filesystem + git HEAD watchers. Exports
`DiffStreamHandle`, `DiffStreamArgs`, `create`, `apply_stream_omit_policy`,
`MAX_CUMULATIVE_DIFF_BYTES`. `apply_stream_omit_policy` is only used internally
(both call-sites are within this file). `create` is called from
`crates/local-deployment/src/container.rs`.

## error_handler.rs

Handles workflow terminal failure: updates workflow status, optionally activates
or creates an error terminal. Called by `crates/services/src/services/orchestrator/agent.rs`.

**Candidate (bug)**: `activate_error_terminal` ignores the workflow's
`error_terminal_cli_id` and `error_terminal_model_id` fields that are set at
workflow creation time; instead it falls back to `CliType::find_all` + `first()`
and `ModelConfig::find_all` + `first()`. The workflow schema clearly intends for
specific cli/model to be pinned per-workflow. This is a silent logic bug — the
error terminal will use a random first CLI/model rather than the configured one.
Confidence: high.

## events.rs

SQLite update-hook bridge that converts DB row changes into JSON-patch messages
pushed to `MsgStore`. Exports `EventService`, re-exports patch modules.
Consumed by `crates/local-deployment/src/lib.rs` and `crates/deployment/src/lib.rs`.

`entry_count` field on `EventService` is marked `#[allow(dead_code)]` but IS
used in the fallback path at line ~471 (incremented and embedded in `/entries/{n}`
patch path). The `#[allow(dead_code)]` annotation is misleading — the field is
read indirectly through the closure capture, not through the struct field accessor.

## feishu.rs

Feishu (Lark) WebSocket integration. Exports `FeishuService`, `FeishuConnector`.
Only wired in `crates/server/src/main.rs`. Contains full session management
(`/new`, `/list`, `/switch`, `/current`), model selection flow, and Concierge
Agent routing alongside legacy `/bind`/`/unbind` orchestrator forwarding.

**Note**: `BusMessage::TerminalMessage` is explicitly documented (G32-016) as
a semantic misuse for carrying external chat messages — a dedicated variant would
be cleaner but is deferred.

## file_ranker.rs

Git-history-based file scoring with a `Lazy<DashMap>` global cache. Exports
`FileRanker`, `FileStat`, `FileStats`. Used by `file_search.rs` and
`crates/services/src/services/project.rs`.

**Note**: Global `FILE_STATS_CACHE` (static `DashMap`) is never evicted and
grows unboundedly across the process lifetime — no TTL, no capacity limit.
Compare with `FileSearchCache`'s moka cache (50-repo / 1h TTL). If many repos
are opened, this leaks indefinitely. Confidence: medium (might be intentional
for a single-repo dev scenario).

## file_search.rs

FST-style linear substring search with moka cache (50-repo, 1h TTL) and `notify`
file-system watchers. Exports `FileSearchCache`, `SearchMode`, `SearchQuery`,
`IndexedFile`, `CachedRepo`, `FileIndexError`, `CacheError`. Used by
`crates/services/src/services/project.rs` and `crates/local-deployment/src/lib.rs`.

**Note (W2-37-05/06)**: Module header explicitly documents that case-insensitive
matching is ASCII-only (`str::to_lowercase`) — known limitation.

`build_file_index` walks the entire repo twice (superset walker + ignore-aware
walker) to determine `is_ignored` per file. This is O(2n) in file count and
may be slow on large repos. Confidence: medium (it's a correctness vs. perf
trade-off).
