# Census: rs-server-routes-planning-quality

Unit scope (server REST route modules):
- `crates/server/src/routes/planning_drafts.rs` (1188 lines)
- `crates/server/src/routes/quality.rs` (170 lines)
- `crates/server/src/routes/system_settings.rs` (74 lines)
- `crates/server/src/routes/setup.rs` (66 lines)

All four `Router`-producing functions are mounted in `crates/server/src/routes/mod.rs`
(authed group L161/L166/L170-175; `setup::router()` is in the **unauthed** group L204).

## Module Map

| File | Purpose | Public surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `planning_drafts.rs` | Planning-draft API for orchestrated workspace mode: chat-driven spec gathering → confirm (audit-plan generation, **System B**) → materialize into a `Workflow`. Holds the **G2 confirm→materialize gate** and the audit-doc upload/delete feature. | `planning_draft_routes() -> Router`; DTOs `CreateDraftRequest`, `SendMessageRequest`, `UpdateSpecRequest`, `ConfirmDraftRequest`, `DraftResponse`, `MessageResponse`, `MaterializeResponse`. Routes (nested at `/planning-drafts`): `POST /` create, `GET /` list, `GET /{id}`, `PUT /{id}/spec`, `POST /{id}/confirm`, `POST /{id}/materialize`, `POST /{id}/feishu-sync`, `POST+DELETE /{id}/audit-doc` (10 MB cap), `GET+POST /{id}/messages`. | Consumes `db::models::planning_draft::{PlanningDraft, PlanningDraftMessage, PLANNING_DRAFT_STATUSES}`; orchestrator `generate_audit_plan` / `default_audit_plan` / `create_llm_client` / `create_claude_code_native_client` / `system_prompt_for_profile(WorkspacePlanning)` / `AuditMode`; `db::models::workflow::Workflow`; `db::models::ModelConfig::first_user_configured_ids`; `crate::routes::workflows::auto_prepare_and_start` (spawned post-materialize, L1017); Feishu via `SharedFeishuHandle`. Frontend: `frontend/src/lib/api.ts` `planningDraftsApi`, `usePlanningDraft.ts`, `PlanningChat*`, `AuditDocPanel.tsx`, `WorkspacesSidebarContainer.tsx`, `CreateChatBoxContainer.tsx`. | Confirm (L286) is where audit-plan is generated and stored; materialize (L904) requires status `confirmed`. **G2 secondary-confirmation popup injects here.** Audit-doc upload writes to `utils::cache_dir()/audit_docs/{draft_id}/{filename}`, ext-allowlist md/txt/pdf/docx (but only `read_to_string` at confirm — pdf/docx aren't parsed). Status state machine enforced in `update_spec`. |
| `quality.rs` | **Read-only** quality-run / quality-issue display API (System A *results*, not config). | `quality_workflow_routes()`, `quality_routes()`, `quality_terminal_routes()` (all `Router`); handlers `list_quality_runs`, `get_quality_run`, `get_quality_issues`, `get_terminal_latest_quality`; DTOs `QualityRunSummary`, `QualityRunDetail` (both `#[derive(TS)]`). Routes: `GET /workflows/{id}/quality/runs`, `GET /quality/runs/{run_id}`, `GET /quality/runs/{run_id}/issues`, `GET /terminals/{id}/quality/latest`. | Reads `db::models::{QualityRun, QualityIssueRecord}`. TS types flow to `shared/types.ts` via ts-rs. Frontend: `useQualityGate.ts` + tests, `frontend/src/hooks/index.ts`. **No** link to `crates/quality` `QualityGateConfig`/`quality-gate.yaml` (those are file-based, never exposed here). | **Zero mutation routes** (no post/put/delete). The **G3 per-project quality-rule CRUD does not exist** in any route module — this file is the natural home but currently display-only. |
| `system_settings.rs` | Global system settings get/update (currently only `feishu_enabled`). | `router() -> Router`; route `GET+PUT /system-settings`. Private DTO `UpdateSettings`. | `db::models::system_settings::SystemSetting::{find_all,set}`; `middleware::auth::{RequestContext, check_admin}`. PUT is admin-gated via `check_admin` (opt-in `SOLODAWN_ADMIN_TOKEN` + `X-Admin-Token`). | `UpdateSettings` has a single field; G24 TODO in comments to move to RBAC on `RequestContext`. `check_admin` is a real cross-module guard (used here + likely elsewhere) — not dead. |
| `setup.rs` | First-run onboarding status + completion flag (mounted **unauthed**). | `router() -> Router`; routes `GET /setup/status`, `POST /setup/complete`. | `db::models::{project::Project::count, system_settings::SystemSetting::{get_bool,set}}`; `deployment.config().workflow_model_library`. | `mark_complete` is idempotent (W2-18-06 guard). Unauthed by design (no token exists pre-setup). |

## In-flight work relevance

- **G2 (confirm→materialize gate):** entirely in `planning_drafts.rs`. `confirm_draft` (L286-390) generates/stores the audit plan and sets status `confirmed`; `materialize_draft` (L904-1034) gates on `status == "confirmed"`, creates the `Workflow`, copies `draft.audit_plan` → `workflow.audit_plan`, then spawns `auto_prepare_and_start`. A mandatory secondary confirmation popup would sit between these two endpoints (matches research/R2 + reports/R6).
- **G3 (per-project quality rules CRUD):** **absent.** `quality.rs` is read-only. The editable rule systems are A=`crates/quality` YAML (`QualityGateConfig`, file-based, no API/DB/UI) and B=audit-plan JSON on the draft/workflow (LLM-generated, opaque). Adding CRUD requires new routes (no existing surface to refactor).
- **System A vs System B:** quality.rs surfaces System A *run results*; planning_drafts.rs confirm flow drives System B *scoring plan*. They are distinct (per docs/research/R4).
- **G1 (open in external IDE) / VS Code webview bridge:** not present in any scope file (grep clean).

## Invisible features (in scope)

- **Feishu sync on planning drafts** (`toggle_feishu_sync` L748, push in `send_message` L611): background `tokio::spawn` pushes chat history/new messages to a Feishu chat; chat-id resolution has a 4-tier fallback (explicit → last_chat_id → concierge session → latest active binding → first bot chat). Used when `feishu_sync` enabled.
- **Claude Code native-credential LLM fallback** (`create_claude_code_native_client("claude-sonnet-4-6")`, L337 + L499): when no planner model is configured, both confirm and send_message silently fall back to native Claude Code subscription credentials.
- **Auto spec extraction / auto status transition** (L530-581): when the assistant reply contains a fenced `productGoal` JSON block, the draft auto-transitions `gathering → spec_ready` and extracts the spec — not an explicit UI action.
- **Auto-prepare-and-start after materialize** (L1010-1027): spawned background task immediately prepares + starts the new workflow so the orchestrator begins without a manual prepare/start click.
- **Admin token gate** (`check_admin` in system_settings PUT): opt-in `SOLODAWN_ADMIN_TOKEN` / `X-Admin-Token` defense-in-depth, invisible no-op when env unset.

## Candidates for keep/cut review

No dead/duplicate/deprecated code found in scope — every handler is mounted and has a verified frontend consumer. Two low-confidence items flagged for investigation only (see JSON): audit-doc pdf/docx accepted-but-not-parsed, and the duplicate Feishu 4000-char truncation block.

## Tooling note

fast-context (`mcp__fast-context__fast_context_search`) succeeded on the first router-mount query, then returned `resource_exhausted` (Windsurf backend quota) on all subsequent calls. Remaining cross-file questions (frontend consumers, CRUD existence, callee resolution) were answered via Grep fallback as permitted.
