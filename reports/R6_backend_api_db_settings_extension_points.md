# R6 — Backend API Surface, DB Schema & Settings Panel: Extension Points

**Date:** 2026-06-13  
**Scope:** What must be touched to add (a) a mandatory secondary confirmation step before `materialize` and (b) editable per-project quality-gate rules (audit-rules CRUD).

---

## 1. HTTP / WS Route Surface

### 1.1 Router Registration

File: `crates/server/src/routes/mod.rs` (line 166)

```rust
.nest("/planning-drafts", planning_drafts::planning_draft_routes())
.nest("/quality", quality::quality_routes())
.nest("/workflows", quality::quality_workflow_routes())
.nest("/terminals", quality::quality_terminal_routes())
```

All planning-draft and quality routes are authenticated (inside the `base_routes` block, which applies `require_api_token` middleware).

### 1.2 Planning-Draft Endpoints

File: `crates/server/src/routes/planning_drafts.rs`

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/api/planning-drafts` | `create_draft` | Create new draft (status=gathering) |
| GET | `/api/planning-drafts` | `list_drafts` | List all or filter by `?project_id=` |
| GET | `/api/planning-drafts/{draft_id}` | `get_draft` | Single draft by ID |
| PUT | `/api/planning-drafts/{draft_id}/spec` | `update_spec` | Update requirement_summary / technical_spec / workflow_seed / status |
| POST | `/api/planning-drafts/{draft_id}/confirm` | `confirm_draft` | Generates audit plan via LLM, sets status=confirmed |
| POST | `/api/planning-drafts/{draft_id}/materialize` | `materialize_draft` | Creates Workflow record, sets status=materialized. **Requires confirmed status** |
| POST | `/api/planning-drafts/{draft_id}/feishu-sync` | `toggle_feishu_sync` | Toggle Feishu notification sync |
| POST | `/api/planning-drafts/{draft_id}/audit-doc` | `upload_audit_doc` | Upload custom audit document (multipart, ≤10 MB) |
| DELETE | `/api/planning-drafts/{draft_id}/audit-doc` | `delete_audit_doc` | Remove uploaded audit document |
| GET | `/api/planning-drafts/{draft_id}/messages` | `list_messages` | List conversation history |
| POST | `/api/planning-drafts/{draft_id}/messages` | `send_message` | Send user message, get LLM reply |

**Key constraint (line 915 in `planning_drafts.rs`):**
```rust
if draft.status != "confirmed" {
    return Err(ApiError::BadRequest(format!("Only confirmed drafts can be materialized...")));
}
```
`materialize_draft` already blocks if status ≠ `confirmed`. The confirm step itself has no secondary human approval gate — it runs LLM audit generation and auto-sets status.

### 1.3 Quality / Audit Endpoints

File: `crates/server/src/routes/quality.rs`

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/workflows/{workflow_id}/quality/runs` | `list_quality_runs` | List quality gate runs |
| GET | `/api/quality/runs/{run_id}` | `get_quality_run` | Single run detail with report JSON |
| GET | `/api/quality/runs/{run_id}/issues` | `get_quality_issues` | Issues for a quality run |
| GET | `/api/terminals/{terminal_id}/quality/latest` | `get_terminal_latest_quality` | Latest terminal gate run |

**No CRUD endpoints exist for quality-gate rules.** There is no `/quality/rules` or `/projects/{id}/quality-config` endpoint.

### 1.4 System Settings Endpoints

File: `crates/server/src/routes/system_settings.rs`

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/system-settings` | `get_settings` |
| PUT | `/api/system-settings` | `update_settings` (admin-gated) |

Currently only handles `feishu_enabled`. No quality-gate rules stored here.

### 1.5 Setup / Onboarding Endpoints

File: `crates/server/src/routes/setup.rs`

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/setup/status` | Returns `{complete, checks:{hasModelConfig, hasProject}}` |
| POST | `/api/setup/complete` | Marks `setup_complete=true` in system_settings |

The "complete all required configurations" gate is: `hasModelConfig` (any model in `workflow_model_library` with non-empty model_id) AND `hasProject` (at least one project row). There is no per-project quality-rule gate.

Config endpoint: `PUT /api/config` in `crates/server/src/routes/config.rs` — stores `setup_wizard_completed` flag in the JSON config file (not DB).

---

## 2. DB Schema

### 2.1 `planning_draft` Table

Migration: `crates/db/migrations/20260307200000_add_planning_draft.sql`  
Columns added later: `20260324150000_add_planning_draft_feishu_sync.up.sql`, `20260508000000_add_audit_plan.sql`

Current columns:

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | UUID string |
| project_id | BLOB | References projects(id) |
| name | TEXT | |
| status | TEXT | `gathering|spec_ready|confirmed|materialized|cancelled` |
| requirement_summary | TEXT | User-facing requirement |
| technical_spec | TEXT | Structured JSON spec |
| workflow_seed | TEXT | Candidate workflow config |
| planner_model_id | TEXT | |
| planner_api_type | TEXT | |
| planner_base_url | TEXT | |
| planner_api_key | TEXT | Encrypted |
| confirmed_at | TEXT | ISO timestamp |
| materialized_workflow_id | TEXT | FK → workflow(id) ON DELETE SET NULL |
| feishu_sync | INTEGER | bool |
| feishu_chat_id | TEXT | |
| sync_tools | INTEGER | bool (migration: 20260324160000) |
| sync_terminal | INTEGER | bool |
| sync_progress | INTEGER | bool |
| notify_on_completion | INTEGER | bool |
| **audit_plan** | TEXT | JSON-serialized `AuditPlan` (generated at confirm) |
| **audit_mode** | TEXT | `builtin|merged|custom` |
| **audit_doc_path** | TEXT | Relative path on disk |
| created_at | TEXT | |
| updated_at | TEXT | |

**Missing for secondary confirmation:** No column tracks whether a human has reviewed/approved the generated audit plan. No `audit_reviewed_at`, `audit_approved_by`, or `secondary_confirmed` flag.

### 2.2 `planning_draft_message` Table

Migration: `crates/db/migrations/20260307200000_add_planning_draft.sql`

| Column | Type |
|--------|------|
| id | TEXT PK |
| draft_id | TEXT FK → planning_draft(id) ON DELETE CASCADE |
| role | TEXT | `user|assistant` |
| content | TEXT |
| created_at | TEXT |

### 2.3 `quality_run` Table

Migration: `crates/db/migrations/20260312140000_create_quality_tables.sql`

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | |
| workflow_id | TEXT | Not FK (workflow may not exist yet) |
| task_id | TEXT | Nullable |
| terminal_id | TEXT | Nullable |
| commit_hash | TEXT | |
| gate_level | TEXT | `terminal|branch|repo` |
| gate_status | TEXT | `pending|running|ok|warn|error|skipped` |
| mode | TEXT | `off|shadow|warn|enforce` |
| total_issues | INTEGER | |
| blocking_issues | INTEGER | |
| new_issues | INTEGER | |
| duration_ms | INTEGER | |
| providers_run | TEXT | JSON array |
| report_json | TEXT | Full `QualityReport` |
| decision_json | TEXT | `QualityGateDecision` |
| error_message | TEXT | |
| created_at | DATETIME | |
| completed_at | DATETIME | |

### 2.4 `quality_issue` Table

See `20260312140000_create_quality_tables.sql`. Columns: `id, quality_run_id, rule_id, rule_type, severity, source, message, file_path, line, end_line, column_start, column_end, is_new, is_blocking, effort_minutes, context, created_at`.

### 2.5 Per-Project Quality-Gate Configuration — Current State

**No DB table stores per-project quality-gate rules.** The quality gate config is loaded from YAML files on disk in this order (`crates/quality/src/config.rs:181`):
1. `{project_root}/quality/quality-gate.yaml`
2. `{project_root}/quality/quality-gate.yml`
3. `{project_root}/.quality-gate.yaml`
4. Fall back to `BUNDLED_CENTRAL_POLICY` (compile-time embedded `quality/quality-gate.yaml`)

There is no DB table for per-project overrides. If a project wants custom quality-gate rules, it must have a YAML file on disk. There is no HTTP endpoint to read or mutate these rules.

### 2.6 `system_settings` Table

Migration: `crates/db/migrations/20260316100000_create_system_settings.sql`

Simple key-value store: `{key, value, description, updated_at}`. Currently seeded with `feishu_enabled` and `setup_complete`. No quality-gate keys.

### 2.7 `workflow` Table (relevant columns)

Migration: `crates/db/migrations/20260117000001_create_workflow_tables.sql` + `20260508000000_add_audit_plan.sql`

Column `audit_plan TEXT` was added to `workflow` in `20260508000000_add_audit_plan.sql`. The `materialize_draft` handler copies `draft.audit_plan` into the created `Workflow` struct (line 984 of `planning_drafts.rs`).

---

## 3. Rust Service / Type Layer

### 3.1 `AuditPlan` and `AuditMode` Types

File: `crates/services/src/services/orchestrator/types.rs`

```rust
pub struct AuditPlan {
    pub mode: AuditMode,
    pub dimensions: Vec<AuditDimensionSpec>,
    pub pass_threshold: f64,
    pub generated_at: String,
    pub raw_principles: String,   // full rubric text for LLM prompts
}

pub enum AuditMode { Builtin, Merged, Custom }

pub struct AuditDimensionSpec {
    pub name: String,
    pub name_zh: String,
    pub max_score: f64,
    pub criteria: Vec<String>,
    pub sub_dimensions: Option<Vec<AuditDimensionSpec>>,
}
```

These are NOT `#[derive(TS)]` — they do not appear in `shared/types.ts`. The frontend only sees the JSON blob serialized into `planning_draft.audit_plan` (TEXT column), and the `auditMode` and `auditDocPath` string fields exposed by `DraftResponse`.

### 3.2 `DraftResponse` (server-side DTO)

File: `crates/server/src/routes/planning_drafts.rs:53-98`  
Not `#[derive(TS)]`. Mapped manually in `frontend/src/lib/api.ts:505` as `PlanningDraftResponse`.

### 3.3 Audit Plan Generation Pipeline

File: `crates/services/src/services/orchestrator/audit_plan.rs`  
Function: `generate_audit_plan(llm_client, requirement_summary, technical_spec, user_audit_doc, mode) -> AuditPlan`

Called from `confirm_draft` handler. Three modes: Builtin (LLM tailors built-in rubric), Merged (LLM merges built-in + user doc), Custom (LLM structures user doc only). Fail-closed — returns `default_audit_plan()` on LLM or parse error.

### 3.4 Built-in Audit Principles

File: `crates/services/src/services/orchestrator/audit_principles.rs`  
Constant `BUILTIN_AUDIT_PRINCIPLES`: 100-point rubric across 5 dimensions. Function `default_audit_plan()` returns a statically-constructed `AuditPlan` with mode=Builtin.

### 3.5 `QualityGateConfig`

File: `crates/quality/src/config.rs`  
Structs: `QualityGateConfig`, `GateDefinition`, `ConditionConfig`, `ProvidersConfig`, `SonarConfig`  
Loaded from YAML only (disk). Not in DB. Not exposed via HTTP API.

---

## 4. TypeScript / Frontend Types

### 4.1 `PlanningDraftResponse` (manually authored in api.ts)

File: `frontend/src/lib/api.ts:505-523`

```typescript
export interface PlanningDraftResponse {
  id: string;
  projectId: string;
  name: string;
  status: 'gathering' | 'spec_ready' | 'confirmed' | 'materialized' | 'cancelled';
  requirementSummary: string | null;
  technicalSpec: string | null;
  workflowSeed: string | null;
  materializedWorkflowId: string | null;
  feishuSync: boolean;
  syncTools: boolean;
  syncTerminal: boolean;
  syncProgress: boolean;
  notifyOnCompletion: boolean;
  auditPlan: string | null;          // JSON blob
  auditMode: 'builtin' | 'merged' | 'custom';
  auditDocPath: string | null;
  createdAt: string;
  updatedAt: string;
}
```

**Not generated via ts-rs.** Not in `shared/types.ts`. Manually kept in sync with `DraftResponse` in Rust.

### 4.2 `planningDraftsApi` (api.ts:540-640)

Functions: `list`, `create`, `get`, `sendMessage`, `listMessages`, `confirm(draftId, retainBuiltin?)`, `materialize(draftId)`, `uploadAuditDoc`, `deleteAuditDoc`, `toggleFeishuSync`.

No CRUD for audit rules exposed from this layer.

### 4.3 `shared/types.ts` (generated by ts-rs)

Planning-draft types (`PlanningDraft`, `AuditPlan`, `AuditMode`, `AuditDimensionSpec`) are **NOT** in `shared/types.ts`. They are absent from `crates/server/src/bin/generate_types.rs`. Quality types that ARE generated: `QualityRun`, `QualityIssueRecord`, `SeverityCount`, `QualityRunSummary`, `QualityRunDetail`.

---

## 5. Frontend Settings Panel

### 5.1 Settings Pages

Entry point: `frontend/src/pages/settings/index.ts` — re-exports from `frontend/src/pages/ui-new/settings/`:
- `GeneralSettingsNew` → `GeneralSettingsNew.tsx`
- `ModelsSettingsNew` → `ModelsSettingsNew.tsx`
- `ProjectSettingsNew` → `ProjectSettingsNew.tsx`
- `ReposSettingsNew` → `ReposSettingsNew.tsx`
- `AgentSettingsNew` → `AgentSettingsNew.tsx`
- `McpSettingsNew` → `McpSettingsNew.tsx`
- `FeishuSettingsNew` → `FeishuSettingsNew.tsx`
- `OrganizationSettingsNew` → `OrganizationSettingsNew.tsx`
- `RuntimeSettingsNew` → `RuntimeSettingsNew.tsx`

**No quality-gate settings page exists.**

### 5.2 Setup Wizard (first-run gate)

Files: `frontend/src/components/setup/SetupWizardShell.tsx`, `SetupWizardStep1Welcome.tsx` through `SetupWizardStep5Done.tsx`

The wizard gate (App.tsx:87): shows if `config.setup_wizard_completed` is falsy. Completion set by `SetupWizardStep5Done` which calls `updateAndSaveConfig({ setup_wizard_completed: true })` — this writes to the JSON config file, not the DB.

The "complete all required configurations" check from `GET /api/setup/status`:
- `checks.hasModelConfig`: any entry in `workflow_model_library` with non-empty `model_id`
- `checks.hasProject`: at least one project row in DB

No quality-gate configuration is required for setup to be "complete".

### 5.3 `wizardStore` (workflow creation wizard — not setup)

File: `frontend/src/stores/wizardStore.ts`  
Used for the workflow creation wizard (7 steps). Manages `WizardConfig` with project, basic, commands, advanced, tasks, models, terminals. No audit-rules or quality-gate state.

### 5.4 Planning Chat Frontend

- Container: `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx`
- View: `frontend/src/components/ui-new/primitives/PlanningChat.tsx`
- Hook: `frontend/src/hooks/usePlanningDraft.ts`

The planning chat UI shows three action buttons based on draft status:
1. `gathering` → "Send" (chat)
2. `spec_ready` → "Confirm" (calls `confirm_draft`) + optional "Refine"
3. `confirmed` → "Materialize" (calls `materialize_draft`)
4. `materialized` → disabled "Done" button

There is no "review audit plan" step between `confirmed` and the Materialize button. The UI directly enables Materialize as soon as status=`confirmed`.

---

## 6. ts-rs Type Generation Pipeline

### How it works end-to-end

1. Rust structs derive `#[derive(TS)]` from the `ts_rs` crate.
2. `crates/server/src/bin/generate_types.rs` calls `::decl()` on each type and assembles `shared/types.ts`.
3. Run via `npm run generate-types` (or `cargo run --bin generate_types`).
4. CI checks with `--check` flag that `shared/types.ts` is up-to-date.

### To add a new type end-to-end:

1. Add `#[derive(TS)]` (plus `#[ts(export)]` if needed) to the Rust struct.
2. Add `MyType::decl()` to the `decls` vec in `generate_types.rs`.
3. Run `npm run generate-types` to regenerate `shared/types.ts`.
4. Import from `shared/types.ts` in frontend code.

**Currently missing from this pipeline for planning/audit:**  
`AuditPlan`, `AuditDimensionSpec`, `AuditMode`, `DraftResponse` / `PlanningDraft` — none derive `TS`, none appear in `generate_types.rs`.

---

## 7. What Must Be Touched for the Two New Features

### Feature A: Mandatory Secondary Confirmation Step (block materialize until human approves audit plan)

#### DB changes
- Add column to `planning_draft`: `audit_confirmed_at TEXT` (nullable; non-null = human has reviewed and approved the generated audit plan)
- New migration: e.g., `20260614000000_add_audit_secondary_confirm.sql`
  ```sql
  ALTER TABLE planning_draft ADD COLUMN audit_confirmed_at TEXT;
  ```

#### Backend (Rust)
- `crates/db/src/models/planning_draft.rs` — add `audit_confirmed_at: Option<DateTime<Utc>>` field; add method `set_audit_confirmed(pool, id)`. **Extend.**
- `crates/server/src/routes/planning_drafts.rs`:
  - Add new route `POST /{draft_id}/audit-confirm` → handler that validates status=`confirmed`, sets `audit_confirmed_at=now()`. **Extend.**
  - Modify `materialize_draft` to check `draft.audit_confirmed_at.is_some()` before proceeding; return 400 if not set. **Extend.**
  - `DraftResponse` struct: add `audit_confirmed_at: Option<String>` field. **Extend.**
  - Update `From<PlanningDraft> for DraftResponse` impl.
- Router in `planning_draft_routes()`: add `.route("/{draft_id}/audit-confirm", post(audit_confirm_handler))`. **Extend.**

#### Frontend
- `frontend/src/lib/api.ts`: add `auditConfirmedAt: string | null` to `PlanningDraftResponse`; add `auditConfirm: async (draftId) => ...` to `planningDraftsApi`. **Extend.**
- `frontend/src/hooks/usePlanningDraft.ts`: add `useAuditConfirmDraft()` mutation hook. **Extend.**
- `frontend/src/components/ui-new/primitives/PlanningChat.tsx`: add a new UI state/step between `confirmed` and `materialized`. When `status=confirmed` and `auditConfirmedAt=null`, show an "Audit Plan Review" panel rendering the parsed `auditPlan` JSON with a "Approve Audit Plan" button. Only after approval (status=confirmed + auditConfirmedAt set) show the Materialize button. **Extend.**
- `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx`: wire up the new `handleAuditConfirm` callback. **Extend.**

### Feature B: Editable Per-Project Quality-Gate Rules (CRUD)

#### DB changes (new table)
```sql
CREATE TABLE IF NOT EXISTS project_quality_config (
    id          TEXT PRIMARY KEY,
    project_id  BLOB NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    gate_level  TEXT NOT NULL,    -- terminal | branch | repo | all
    config_json TEXT NOT NULL,    -- serialized QualityGateConfig or subset
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX idx_project_quality_project_level
    ON project_quality_config(project_id, gate_level);
```
New migration file needed.

#### Backend (Rust)
- New DB model: `crates/db/src/models/project_quality_config.rs` — struct `ProjectQualityConfig`; CRUD methods `find_by_project`, `upsert`, `delete`. **New file.**
- `crates/db/src/models/mod.rs` — expose new model. **Extend.**
- New route file: `crates/server/src/routes/project_quality.rs` — handlers:
  - `GET /projects/{project_id}/quality-config` → load DB row; fall back to bundled YAML if absent
  - `PUT /projects/{project_id}/quality-config` → upsert (validate YAML/JSON structure against `QualityGateConfig`)
  - `DELETE /projects/{project_id}/quality-config` → remove override (reverts to bundled policy)
  Also: `GET /projects/{project_id}/quality-config/rules` and `POST/PUT/DELETE /projects/{project_id}/quality-config/rules/{rule_id}` for granular condition CRUD. **New file.**
- `crates/server/src/routes/mod.rs`: declare `pub mod project_quality;` and add `.merge(project_quality::router())`. **Extend.**
- `crates/quality/src/config.rs`: add `QualityGateConfig::merge_with_project_override(project_override: &ProjectQualityConfig)` or similar. **Extend.**
- `crates/quality/src/engine.rs`: before running a quality gate, look up `ProjectQualityConfig` from DB (needs pool passed in). **Extend.**

#### Frontend
- New settings sub-page: `frontend/src/pages/ui-new/settings/QualitySettingsNew.tsx` — CRUD editor for conditions (metric, operator, threshold) per gate level (terminal/branch/repo). **New file.**
- `frontend/src/pages/settings/index.ts`: add export for `QualitySettingsNew`. **Extend.**
- `frontend/src/lib/api.ts`: add `projectQualityApi` with `get(projectId)`, `update(projectId, config)`, `delete(projectId)`. **Extend.**
- New TypeScript interfaces: `QualityGateCondition`, `ProjectQualityConfig`. Could be driven from ts-rs (requires adding `#[derive(TS)]` to Rust structs in `crates/quality/src/config.rs`). **Extend.**
- Router in `App.tsx` or settings router: add route `/settings/quality`. **Extend.**

#### ts-rs pipeline changes for Feature B
- `crates/quality/src/config.rs`: add `#[derive(TS)]` to `QualityGateConfig`, `GateDefinition`, `ConditionConfig`, `QualityGateMode`, `ProvidersConfig`. Add `use ts_rs::TS;`. Also add to `crates/quality/Cargo.toml`: `ts-rs` dep. **Extend.**
- `crates/server/src/bin/generate_types.rs`: add `quality::config::QualityGateConfig::decl()` etc. **Extend.**
- Run `npm run generate-types`.

---

## 8. Summary of All Extension Points

| File | Disposition | Required Change |
|------|------------|-----------------|
| `crates/db/migrations/20260614000000_add_audit_secondary_confirm.sql` | **New** | `ALTER TABLE planning_draft ADD COLUMN audit_confirmed_at TEXT` |
| `crates/db/migrations/2026061XXXXXXX_create_project_quality_config.sql` | **New** | New `project_quality_config` table |
| `crates/db/src/models/planning_draft.rs` | **Extend** | Add `audit_confirmed_at` field + `set_audit_confirmed()` method |
| `crates/db/src/models/project_quality_config.rs` | **New** | New model for per-project quality config |
| `crates/db/src/models/mod.rs` | **Extend** | Export new model |
| `crates/quality/src/config.rs` | **Extend** | Add `#[derive(TS)]`, add `merge_with_project_override()` |
| `crates/quality/src/engine.rs` | **Extend** | Load project override from DB before evaluating gate |
| `crates/server/src/routes/planning_drafts.rs` | **Extend** | New `audit-confirm` endpoint; block `materialize` until `audit_confirmed_at` set; add field to `DraftResponse` |
| `crates/server/src/routes/project_quality.rs` | **New** | CRUD endpoints for per-project quality-gate config |
| `crates/server/src/routes/mod.rs` | **Extend** | Register `project_quality` module + routes |
| `crates/server/src/bin/generate_types.rs` | **Extend** | Add quality config type declarations |
| `quality/quality-gate.yaml` | **Keep** | Central/bundled policy — unchanged; acts as default |
| `quality/profiles/enforce-mode.yaml` | **Keep** | Reference profile only |
| `frontend/src/lib/api.ts` | **Extend** | Add `auditConfirmedAt`, `auditConfirm()`, `projectQualityApi` |
| `frontend/src/hooks/usePlanningDraft.ts` | **Extend** | Add `useAuditConfirmDraft()` hook |
| `frontend/src/components/ui-new/primitives/PlanningChat.tsx` | **Extend** | Add audit-plan review step between confirmed→materialized |
| `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx` | **Extend** | Wire `handleAuditConfirm` callback |
| `frontend/src/pages/ui-new/settings/QualitySettingsNew.tsx` | **New** | Quality-gate rules CRUD settings page |
| `frontend/src/pages/settings/index.ts` | **Extend** | Export `QualitySettingsNew` |
| `shared/types.ts` | **Extend** | Auto-generated; will include new quality config types after pipeline update |

---

## 9. Open Questions / Risks

1. **Scope of secondary confirmation:** The current `confirm` endpoint already runs LLM generation and is idempotent. Should `audit-confirm` be a completely separate state machine step (new status like `audit_reviewed`), or just a flag column on the existing `confirmed` status? Adding a new status value would affect `PLANNING_DRAFT_STATUSES`, all status-check code, and frontend rendering.

2. **Audit plan editing:** If users can edit audit-plan dimensions/criteria in the secondary confirmation step (before clicking "Approve"), the `AuditPlan` struct needs to be exposed via a PUT endpoint (`/{draft_id}/audit-plan`) and must derive `TS` for the frontend to work with it properly.

3. **Per-project quality config storage:** Storing `QualityGateConfig` as a JSON blob vs. normalized rows in a relational table. Blob is simpler (matches the YAML structure), normalized is more queryable. Currently the engine reads YAML files from disk — the engine would need a DB pool reference added to its call path to load DB overrides.

4. **Quality crate `Cargo.toml` dependency on ts-rs:** The quality crate currently does not depend on `ts-rs`. Adding it pulls in a new dependency that the CI and `generate_types.rs` binary must handle.

5. **Auth/permission model:** Currently all authenticated users can call any planning-draft endpoint. If secondary confirmation should be gated to a specific role (e.g., project manager), the `require_api_token` middleware alone is insufficient — a role-check layer would be needed.

6. **`upload_audit_doc` field name mismatch:** The frontend (`api.ts:611`) appends the file under field name `"audit_doc"` but the backend (`planning_drafts.rs:1072`) expects field name `"file"`. This is a pre-existing bug that should be fixed alongside this work.
