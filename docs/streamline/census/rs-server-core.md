# Census: rs-server-core

Unit scope: `crates/server/src/` root files + `middleware/` + `mcp/` + `bin/`.
EXCLUDES: `routes/`, `self_test/`, `tests/`.

Branch: refactor/streamline-and-quality-gates. Tool note: fast-context confirmed the
auth-module call sites; subsequent fast-context calls returned `resource_exhausted`
(quota), so cross-file usage for editor/feishu/mcp/generate-types was confirmed via Grep.

## Module map

| File | Purpose | Public surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `lib.rs` | Crate root; declares modules, re-exports `Deployment` trait and the `DeploymentImpl` type alias. | `pub mod error/feishu_handle/mcp/middleware/routes/self_test`; `pub use deployment::Deployment`; `pub type DeploymentImpl = local_deployment::LocalDeployment`. | Aliased everywhere as backend state type. | Lines 50-53: commented-out `#[cfg(feature="cloud")] CloudDeployment` branch — cloud deployment is permanently disabled (dead comment / reserved feature). |
| `error.rs` | Central `ApiError` enum + `IntoResponse` mapping to HTTP status/JSON; `From` impls for service/db errors. | `pub enum ApiError` (ts-rs `TS`, serialized as `string`); `From<...>` for ProjectService/RepoService/ProjectRepo/Git2/&str. | Used by every route handler; `EditorOpen(EditorOpenError)` variant feeds the open-in-editor feature. | `[G35-003/004]` internal-error redaction is implemented. All variants appear reachable. |
| `main.rs` | Binary entrypoint (`solodawn-server`). CLI parse, release-mode secret guards, tracing/sentry init, deployment bootstrap, startup migrations/backfills, WS event bridge, Concierge agent, conditional Feishu connector, model-library→DB sync, axum serve + graceful shutdown. | `SoloDawnError`, `shutdown_signal()`, `perform_cleanup_actions()`; private `run_server`, `start_feishu_connector`, `ensure_*` guards, `decrypt_feishu_secret`. | Calls `routes::router(...)`, `EventBridge`, `SubscriptionHub`, `ConciergeAgent`, `FeishuService`. | `CliType` self-test subcommand `SelfTest` delegates to excluded `self_test::run`. `DEV_DEFAULT_ENCRYPTION_KEY` dev fallback only when debug. |
| `feishu_handle.rs` | Shared handle struct for the running Feishu connector, accessible from route handlers. | `pub struct FeishuHandle`, `pub type SharedFeishuHandle`, `pub fn new_shared_handle()`. | Created in main.rs `start_feishu_connector`; consumed by routes feishu.rs/concierge.rs/health.rs/mod.rs/planning_drafts.rs. | Live. Invisible feature (Feishu/Lark bot bridge), gated by `SystemSetting::is_feishu_enabled`. |
| `middleware/mod.rs` | Re-export hub for middleware submodules. | `pub mod auth; pub mod model_loaders; pub use auth::*; pub use model_loaders::*`. | — | Trivial glob re-export. |
| `middleware/auth.rs` | Bearer-token auth middleware + opt-in defense-in-depth helpers. | `RequestContext`, `assert_authorized()`, `check_admin()`, `require_api_token()` (middleware), private `constant_time_eq`. | `require_api_token` layered in routes/mod.rs; `assert_authorized` used in model_loaders + routes/tasks.rs; `check_admin` used in routes/config.rs (update_config) + system_settings.rs (update_settings). | All public fns confirmed used. Dev-mode passthrough when `SOLODAWN_API_TOKEN` unset; strict modes gated by `SOLODAWN_REQUIRE_AUTH` / `SOLODAWN_ADMIN_TOKEN`. |
| `middleware/model_loaders.rs` | Path-param loader middleware: fetch DB model by UUID and inject as request extension; each gated by `assert_authorized`. | `load_project_middleware`, `load_task_middleware`, `load_workspace_middleware`, `load_execution_process_middleware`, `load_tag_middleware`, `load_session_middleware`. | Layered per-route in routes/. | `load_task_middleware` comment "validate it belongs to the project" is aspirational (no project-ownership check) — noted, not a bug per se. |
| `mcp/mod.rs` | Module decl for MCP. | `pub mod task_server`. | — | Trivial. |
| `mcp/task_server.rs` | rmcp stdio MCP server: exposes task/project management tools (create/list/update/delete tasks, list projects/repos, start workspace session, get_context). Talks to the backend HTTP API as a reqwest client. | `TaskServer` (new/init), many `#[schemars]` request/response DTOs, 9 `#[tool]` methods, `ServerHandler` impl. | Only production consumer is `bin/mcp_task_server.rs`. Calls backend `/api/*` endpoints over HTTP. | `TODO(G35-008)` unauth alignment. `get_context` tool is conditionally removed at runtime if context fetch fails. Reuses route DTOs (`ContainerQuery`, `CreateTaskAttemptBody`, `WorkspaceRepoInput`). |
| `bin/mcp_task_server.rs` | Binary `mcp_task_server`: stdio MCP server entrypoint. Resolves backend URL via env/port-file, builds `TaskServer`, serves over stdio. | `fn main`. | Wraps `mcp::task_server::TaskServer`. | NOT referenced by any build script / npm script / executor config. `default_mcp.json` wires the `solodawn` MCP entry as `npx -y solodawn@latest --mcp` (npm wrapper), not this Rust bin. Invocation path is indirect/unverified — see candidates. |
| `bin/generate_types.rs` | Binary `generate_types`: the ts-rs + JSON-schema codegen pipeline. Emits `shared/types.ts` and `shared/schemas/*.json`; `--check` mode for CI drift detection. | `fn main`, `generate_types_content`, `generate_schemas`, `write_schemas`, `schemas_up_to_date`, `generate_json_schema`. | Invoked by `package.json` `generate-types` / `generate-types:check`; CI `ci-basic.yml` L46 runs `--check`. G1/G3 critical. | Stale banner: HEADER (L9) and emitted `shared/types.ts` say `crates/core/src/bin/generate_types.rs` but the real path is `crates/server/src/bin/...`. Wipes & recreates `shared/` on non-check runs (`fs::remove_dir_all`). |

## Invisible features

- **MCP task server (stdio)** — `bin/mcp_task_server.rs` + `mcp/task_server.rs`. A full task/project management MCP server that coding agents can connect to. Not user-visible; launched out-of-band. Wiring to the Rust bin is unconfirmed (default config uses an npm wrapper).
- **Feishu/Lark connector bridge** — `feishu_handle.rs` + main.rs `start_feishu_connector`. Background WebSocket bot connector with auto-reconnect; gated by DB `SystemSetting::is_feishu_enabled`. Routes consume the shared handle for test-send/test-receive.
- **Cloud deployment (reserved/disabled)** — lib.rs L50-53 commented `CloudDeployment` branch. Permanently off; `DeploymentImpl` hardcoded to `LocalDeployment`.
- **Opt-in security gates** — `SOLODAWN_REQUIRE_AUTH` (strict auth) and `SOLODAWN_ADMIN_TOKEN`/`X-Admin-Token` (admin gate). No-ops by default; not surfaced in UI.
- **Concierge agent + WS event bridge** — initialized in main.rs; backend-side runtime plumbing.
- **Startup model-library → DB sync** — main.rs L256-306 re-pushes `config.json` workflow_model_library into DB each boot so models survive DB resets.

## In-flight-work relevance

- **(a) Open in external IDE/editor [G1 deletion candidate]:** `error.rs` `ApiError::EditorOpen(EditorOpenError)` is live; the feature is wired in routes (`task_attempts.rs`, `repo.rs`, `projects.rs`) and services config editor. If G1 deletes the feature, the `EditorOpen` variant + its status mapping in error.rs (L66, L164-169) become removable.
- **(c) Quality Gate System A / (d) planning-draft + AuditPlan System B:** No QualityGateConfig / planning-draft / AuditPlan logic lives in this unit (it's all in routes/, out of scope). main.rs only wires generic routing.
