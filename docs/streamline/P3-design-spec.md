# P3 FINAL Implementation Spec — G2 (mandatory quality-gate confirm) + G3 (per-project rules CRUD)

> Status: implementation-ready. Mechanism: **Hybrid A (DB priority-0 via services-layer resolver-injection)**.
> All file:line anchors below were re-verified on branch `refactor/streamline-quality-gates` (working tree, 2026-06-14).
> Authoritative judge decision: Design A. Grafted ideas explicitly tagged `[GRAFT]`.

---

## 0. Verified facts that constrain the design (do NOT relitigate)

| Fact | Evidence | Consequence |
|---|---|---|
| `quality` crate has **no** `db` dep; `db` has **no** `quality` dep; `services` depends on both | `crates/quality/Cargo.toml` (no db), `crates/services/Cargo.toml:14-15` | The DB-priority resolver MUST live in `services`, NOT inside `QualityGateConfig::load_from_project`. Engine stays pool-free & file-write-free. |
| `ProvidersConfig` has **exactly 11** bool toggles | `crates/quality/src/config.rs:85-114` (`rust, frontend, repo, security, sonar, builtin_rust, builtin_frontend, builtin_common, coverage, completeness, delivery_readiness`) | Editor renders **11** checkboxes. NOT 13. |
| `MetricKey` is a closed enum, last variant is internal sentinel `QualityGateEmptyScan` | `crates/quality/src/metrics.rs:11-169` (sentinel at `:167-168`) | Editor metric column is a **picker** over the enum. `[GRAFT-C]` apply `#[ts(skip)]` to `QualityGateEmptyScan` so it does not leak into the FE picker. |
| `Operator` = `GT`\|`LT` only | `crates/quality/src/gate/condition.rs:16-39` | Operator column is a 2-value select. |
| Table is `projects` (plural); `planning_draft` (singular) | `crates/db/migrations/20250617183714_init.sql:5` | FK = `REFERENCES projects(id)`; `ALTER TABLE planning_draft`. (Design B's `project(id)` would fail.) |
| `planning_draft.confirmed_at` already exists (System-B audit confirm) | `crates/db/src/models/planning_draft.rs:24`, `set_confirmed():223-237` | New gate confirmation MUST be a **separate** `gates_confirmed_at` column — must NOT alias `confirmed_at`. |
| `materialize_draft` status guard at L915-920, `Workflow::create` at L996, auto-start `tokio::spawn` at L1016 | `crates/server/src/routes/planning_drafts.rs:904-1034` | 400 hard-block inserted right after L920, BEFORE workflow creation. |
| `DraftResponse` is hand-serded camelCase + manual `From<PlanningDraft>`, NOT ts-rs | `crates/server/src/routes/planning_drafts.rs:53-99` | `gatesConfirmedAt` is a **manual** struct field + From-map + FE-interface edit. ts-rs will NOT generate it. |
| Terminal gate runs inside `tokio::spawn(async move)` (L2603) that captures **no** `&self`, but moves in `db = Arc::clone(&self.db)` (L2594) and already fetches `wf.project_id` at L2717-2719 | `crates/services/src/services/orchestrator/agent.rs:2594-2746`, gate call at **L2815** | Resolver at terminal site must be a **free fn over the moved `db.pool`**, called inside the spawn. `[GRAFT-C]` mental model: resolve the `QualityGateConfig` (plain `Send` serde struct) where pool+project_id already exist. |
| Branch/repo gate inside `run_final_quality_gate(&self,…)` (L7848) — has `&self` | `agent.rs:7848-7880`, gate call at **L7880** | Resolver here can use `self.db.pool` directly. |
| `from_project` is the sole engine constructor that loads-from-fs | `crates/quality/src/engine.rs:79-141` | Split: extract `build_providers(&config)`, add `from_config(config, root)`, keep `from_project` delegating. |
| `quality.rs` route module already exists, already imports `db` + `ts_rs::TS`; nested under `/quality`, `/workflows`, `/terminals` | `crates/server/src/routes/quality.rs:1-60`, `mod.rs:170-175` | Add policy routes **into existing `quality.rs`** `[GRAFT]`, nest under `/quality` and `/projects`. |
| server crate has `db` (L15), `services` (L16), `ts-rs` (L30) but **no `quality`** dep | `crates/server/Cargo.toml` | MUST add `quality = { path = "../quality" }` for the new DTO/`decl()` calls (cycle-free: quality has no upward deps). |
| generate_types decls vec closes with `];` at **L242**; CL-IDE removes L128-129 (`OpenEditorRequest/Response`) | `crates/server/src/bin/generate_types.rs:118-242` | New `decl()` lines inserted **before L242** (after L241), disjoint from the 128-129 removal. |
| FE-01: `api.ts:607` appends multipart field `'audit_doc'`; backend only accepts `'file'` and 400s otherwise | `frontend/src/lib/api.ts:607`, `planning_drafts.rs:1072,1135-1137` | Change to `'file'`. |
| latest migration = `20260508000000_add_audit_plan.sql` | `crates/db/migrations/` | New migration timestamps sort after it. **Coordinate with P4-FINAL RB-49 feishu migration** to avoid timestamp collision (see §5). |

---

## 1. DB migrations (forward-only; new files only)

Two **new** single-file `.sql` migrations (matching the `20260508000000_add_audit_plan.sql` single-file convention; up-only is acceptable per that precedent). SQLite nullable `ADD COLUMN` needs no backfill.

### `crates/db/migrations/20260614120000_create_project_quality_policy.sql`
```sql
-- G3: per-project System-A quality-gate policy override (priority-0 source of truth).
-- config_yaml is serde_yaml of quality::config::QualityGateConfig — byte-identical to
-- quality/quality-gate.yaml form, round-trips via QualityGateConfig::from_yaml. Storing
-- opaque YAML keeps the db crate free of any quality types.
CREATE TABLE IF NOT EXISTS project_quality_policy (
    project_id   TEXT PRIMARY KEY NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    config_yaml  TEXT NOT NULL,
    mode         TEXT NOT NULL DEFAULT 'enforce',   -- [GRAFT-C] denormalized for list badges; config_yaml authoritative
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
```
> `project_id` PK ⇒ one policy per project ⇒ upsert `ON CONFLICT(project_id) DO UPDATE`. `projects(id)` is `TEXT` UUID (matches `planning_draft.project_id` storage). Mirrors `quality_policy_snapshot` (config_yaml + mode) convention verified at `20260313100000_create_quality_policy_snapshot.sql`.

### `crates/db/migrations/20260614120001_add_gates_confirmed_at.sql`
```sql
-- G2: System-A quality-gate confirmation timestamp on the planning draft.
-- DISTINCT from the pre-existing confirmed_at (System-B audit confirm). Nullable, no backfill.
ALTER TABLE planning_draft ADD COLUMN gates_confirmed_at DATETIME;
```

---

## 2. Backend (Rust)

### 2.1 `crates/quality/Cargo.toml` — add ts-rs
```toml
ts-rs = { workspace = true }
```
(workspace pin at root `Cargo.toml:32`.)

### 2.2 `crates/quality/src/config.rs` — ts-rs derives + `validate()`
Add `use ts_rs::TS;` and `#[derive(TS)]` (+ `#[ts(export)]` per repo convention) to: `QualityGateConfig`, `QualityGateMode`, `GateDefinition`, `ConditionConfig`, `ProvidersConfig`, `SonarConfig`.

`[GRAFT-B]` Add field-level validation returning per-condition errors (better editor UX than a bare bool):
```rust
impl QualityGateConfig {
    /// Validate every gate condition: operator parses (GT|LT) and threshold is well-formed.
    /// Returns one human-readable string per offending condition; empty Vec = valid.
    pub fn validate(&self) -> Vec<String> {
        let mut errs = Vec::new();
        for (gate_name, gate) in [
            ("terminal", &self.gates.terminal),
            ("branch", &self.gates.branch),
            ("repo", &self.gates.repo),
        ] {
            for (i, cond) in gate.conditions.iter().enumerate() {
                if let Err(e) = cond.to_condition() {
                    errs.push(format!("{gate_name}.conditions[{i}] ({}): {e}", cond.metric.as_str()));
                }
            }
        }
        errs
    }
}
```
> Confirm the exact `gates` accessor names (`terminal/branch/repo`) against the `QualityGateConfig` struct before wiring; adjust the loop if the field is a map. `MetricKey` is enforced by serde at deserialize time, so any non-enum metric already 400s before `validate()` runs.

### 2.3 `crates/quality/src/metrics.rs` — ts-rs derive + skip sentinel
```rust
use ts_rs::TS;
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, TS)]
#[ts(export)]
pub enum MetricKey { … }
```
`[GRAFT-C]` On the sentinel variant only:
```rust
    #[serde(rename = "quality_gate_empty_scan")]
    #[ts(skip)]
    QualityGateEmptyScan,
```
> `#[serde(rename=…)]` is honored by this ts-rs fork ⇒ `MetricKey` emits a TS string-literal union of the snake_case keys. `#[ts(skip)]` removes the internal sentinel from that union so the FE picker cannot select it. Verify the fork honors `#[ts(skip)]` on an enum variant; if not, post-process is unnecessary because the GET /metrics endpoint (§2.7) is the picker source and simply omits the sentinel.

### 2.4 `crates/quality/src/gate/condition.rs` — ts-rs derive on `Operator`
```rust
use ts_rs::TS;
#[derive(…, TS)]
#[ts(export)]
pub enum Operator { … }   // emits "GT" | "LT"
```

### 2.5 `crates/quality/src/engine.rs` — split constructor (`[GRAFT-B]` shared helper)
Replace `from_project` (L79-141) with:
```rust
/// Build the enabled provider set from a config. Single source of truth for the
/// 11-toggle block (was inlined in from_project); shared by both constructors.
fn build_providers(config: &QualityGateConfig) -> Vec<Arc<dyn QualityProvider>> {
    let mut providers: Vec<Arc<dyn QualityProvider>> = Vec::new();
    // … move the existing L85-138 toggle block here verbatim, reading `config.providers.*` …
    providers
}

/// Pure constructor: caller supplies the resolved config (DB-first resolution
/// happens in the services layer). project_root reserved for future per-root
/// provider tuning; unused today.
pub fn from_config(config: QualityGateConfig, _project_root: &Path) -> anyhow::Result<Self> {
    let providers = Self::build_providers(&config);
    Ok(Self::new(config, providers))
}

/// Filesystem fallback constructor (unchanged behavior for all non-orchestrator callers/tests).
pub fn from_project(project_root: &Path) -> anyhow::Result<Self> {
    let config = QualityGateConfig::load_from_project(project_root)?;
    Self::from_config(config, project_root)
}
```
> Engine gains NO db dep and writes NO files. All existing `from_project` callers unchanged.

### 2.6 `crates/db/src/models/project_quality_policy.rs` (NEW) + `mod.rs` export
```rust
use chrono::{DateTime, Utc};
use sqlx::{FromRow, SqlitePool};
use uuid::Uuid;

#[derive(Debug, Clone, FromRow)]
pub struct ProjectQualityPolicy {
    pub project_id: Uuid,
    pub config_yaml: String,
    pub mode: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl ProjectQualityPolicy {
    pub async fn find_by_project(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Self>("SELECT * FROM project_quality_policy WHERE project_id = ?1")
            .bind(project_id).fetch_optional(pool).await
    }
    pub async fn upsert(pool: &SqlitePool, project_id: Uuid, config_yaml: &str, mode: &str) -> sqlx::Result<()> {
        sqlx::query(
            r"INSERT INTO project_quality_policy (project_id, config_yaml, mode, created_at, updated_at)
              VALUES (?1, ?2, ?3, datetime('now'), datetime('now'))
              ON CONFLICT(project_id) DO UPDATE SET
                config_yaml = excluded.config_yaml,
                mode        = excluded.mode,
                updated_at  = datetime('now')",
        ).bind(project_id).bind(config_yaml).bind(mode).execute(pool).await?;
        Ok(())
    }
    pub async fn delete(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<()> {
        sqlx::query("DELETE FROM project_quality_policy WHERE project_id = ?1")
            .bind(project_id).execute(pool).await?;
        Ok(())
    }
}
```
`crates/db/src/models/mod.rs`: add `pub mod project_quality_policy;` and `pub use project_quality_policy::*;` (near the existing quality_* exports at L24-39).

### 2.7 `crates/db/src/models/planning_draft.rs` — `gates_confirmed_at` + setter
- Struct (after L24 `confirmed_at`): `pub gates_confirmed_at: Option<DateTime<Utc>>,`
- `new()` (L80-105): `gates_confirmed_at: None,`
- `insert()` (L108-148): `find_by_id` uses `SELECT *` so reads auto-map. The `insert` SQL has an explicit column list (?1..?24) — **add** `gates_confirmed_at` as `?25` to both the column list and the bind chain (`.bind(draft.gates_confirmed_at)` after `confirmed_at`), bumping the VALUES placeholder count. (New drafts always insert `None`, so order-only correctness matters.)
- New setter (mirror `set_confirmed` at L223-237, but a DISTINCT column — do NOT touch `confirmed_at`):
```rust
pub async fn set_gates_confirmed(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
    sqlx::query(
        "UPDATE planning_draft SET gates_confirmed_at = datetime('now'),
         updated_at = datetime('now') WHERE id = ?1",
    ).bind(id).execute(pool).await?;
    Ok(())
}
```

### 2.8 `crates/services/src/services/orchestrator/quality_policy.rs` (NEW) — the resolver
DB-first, then the existing file→bundled→default chain. **Free function** (callable from the no-`&self` terminal spawn over the moved `db.pool`).
```rust
use std::path::Path;
use sqlx::SqlitePool;
use uuid::Uuid;
use quality::config::QualityGateConfig;

/// Resolve the effective System-A quality config for a project + working dir.
/// Priority: (0) DB project_quality_policy  →  (1) repo-local quality-gate.yaml
///           →  (2) BUNDLED_CENTRAL_POLICY   →  (3) default_config().
/// Always re-read at gate run time so mid-run G3 edits are honored.
pub async fn resolve_quality_config(
    pool: &SqlitePool,
    project_id: Uuid,
    project_root: &Path,
) -> QualityGateConfig {
    if let Ok(Some(policy)) =
        db::models::project_quality_policy::ProjectQualityPolicy::find_by_project(pool, project_id).await
    {
        match QualityGateConfig::from_yaml(&policy.config_yaml) {
            Ok(cfg) => return cfg,
            Err(e) => tracing::warn!(%project_id, error = %e,
                "project_quality_policy YAML failed to parse — falling back to file/bundled chain"),
        }
    }
    // Fallback chain identical to non-orchestrated callers.
    QualityGateConfig::load_from_project(project_root)
        .unwrap_or_else(|_| QualityGateConfig::default_config())
}
```
> `default_config()` is the same final fallback `load_from_project` already uses; confirm its exact path/name in config.rs and call it directly here for the parse-failure case. Register `pub mod quality_policy;` in the orchestrator module's `mod.rs`.

### 2.9 `crates/services/src/services/orchestrator/agent.rs` — thread resolver at the 3 enforcing sites

**Site 1 — terminal gate, L2815** (inside `tokio::spawn(async move)`, no `&self`; `db` Arc moved in at L2594; `wf.project_id` fetched at L2717-2719). Resolve the config where pool+project_id are in scope, then call `from_config`:
```rust
// project_id is already available from the worktree-fallback Workflow fetch.
// If working_dir came from the worktree branch (the wf fetch was skipped),
// fetch wf.project_id here from the moved db.pool before this block.
let resolved_cfg = crate::services::orchestrator::quality_policy::resolve_quality_config(
    &db.pool, project_id, wd,
).await;
match quality::engine::QualityEngine::from_config(resolved_cfg, wd) {   // was: QualityEngine::from_project(wd)
    Ok(engine) => { /* unchanged: engine.run_with_scope(...).await */ }
    Err(error) => { /* unchanged */ }
}
```
> The struct is plain `Send`, so resolving inside the spawn is fine. Ensure `project_id` is in scope on the worktree path (the L2717 fetch only runs when there is no worktree); add a small `Workflow::find_by_id(&db.pool, &workflow_id)` to obtain `project_id` if the worktree branch was taken.

**Site 2 — branch/repo gate, L7880** (`run_final_quality_gate(&self,…)`):
```rust
let project_id = self.load_workflow().await?.project_id;   // or existing accessor for the workflow
let resolved_cfg = crate::services::orchestrator::quality_policy::resolve_quality_config(
    &self.db.pool, project_id, project_root,
).await;
let report = match quality::engine::QualityEngine::from_config(resolved_cfg, project_root) {  // was: from_project(project_root)
    Ok(engine) => match engine.run_with_scope(project_root, level, scope).await { … },
    Err(error) => { … }
};
```
> Confirm the `&self` workflow/project_id accessor name (this method receives `workflow_id: &str`; fetch `Workflow::find_by_id(&self.db.pool, workflow_id)` → `.project_id`).

**Site 3 — workspace/container gate, `crates/services/src/services/container.rs:246`**: this is the advisory/shadow path; `project_id` reachability is unverified. **Leave `from_project` unchanged** unless `project_id` is trivially reachable. Confirm before commit (see §5 risks).

### 2.10 `crates/server/Cargo.toml` — add quality dep
```toml
quality = { path = "../quality" }
```

### 2.11 `crates/server/src/routes/quality.rs` — policy DTOs + handlers (`[GRAFT]` extend existing module)
Add at the bottom of the existing module:
```rust
use quality::config::QualityGateConfig;
use quality::metrics::MetricKey;

#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct QualityPolicyResponse {
    pub source: String,            // "project" | "file" | "bundled"
    pub config: QualityGateConfig,
}

#[derive(Debug, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct MetricCatalogResponse {
    pub metrics: Vec<MetricKey>,   // closed-enum picker source (sentinel excluded)
    pub operators: Vec<String>,    // ["GT","LT"]
}

// GET /quality/policy/default  -> QualityGateConfig parsed from BUNDLED_CENTRAL_POLICY
// GET /quality/policy/metrics  -> MetricCatalogResponse
// GET    /projects/{project_id}/quality-policy  -> QualityPolicyResponse (DB-first resolve)
// PUT    /projects/{project_id}/quality-policy  body QualityGateConfig
//        -> cfg.validate(); if non-empty return 400 with the joined errors; else serde_yaml
//           serialize + ProjectQualityPolicy::upsert(project_id, yaml, cfg.mode_str())
// DELETE /projects/{project_id}/quality-policy  -> ProjectQualityPolicy::delete (reset to default)
```
- `quality_policy_routes()` → registered under `/quality` (default, metrics).
- `quality_policy_project_routes()` → registered under `/projects` using `/{project_id}/quality-policy`.
> Build the metrics list from an explicit `MetricKey` array excluding `QualityGateEmptyScan` (the sentinel), or derive from a const slice. Default endpoint parses `quality::config::BUNDLED_CENTRAL_POLICY` via `QualityGateConfig::from_yaml`.

### 2.12 `crates/server/src/routes/mod.rs` — register policy routes
After the existing quality nests (L170-175), add:
```rust
.nest("/quality", quality::quality_policy_routes())
.nest("/projects", quality::quality_policy_project_routes())
```
> The `/projects/{project_id}/quality-policy` nest sits as a sibling of the existing standalone `/projects/{project_id}/repositories/{repo_id}` route (projects.rs:752) which is OUTSIDE the `load_project_middleware` layer — so the new policy routes do NOT require that middleware. Alternatively register inside `projects::router`'s outer `projects_router` (projects.rs:749-757) as a sibling route; either is acceptable, keep it OUTSIDE `project_id_router`'s middleware.

### 2.13 `crates/server/src/routes/planning_drafts.rs` — G2 hard-block + confirm-gates + DTO
1. **Hard-block** — in `materialize_draft`, immediately AFTER the status guard (after L920):
```rust
if draft.gates_confirmed_at.is_none() {
    return Err(ApiError::BadRequest(
        "Quality gates must be confirmed before materialization. \
         Call POST /planning-drafts/{id}/confirm-gates first.".to_string(),
    ));
}
```
This is BEFORE `Workflow::create` (L996) and the auto-start spawn (L1016) ⇒ bypass-proof; a direct API call also 400s.

2. **DraftResponse** (manual, NOT ts-rs):
- struct (after L71 `audit_doc_path`): `pub gates_confirmed_at: Option<String>,`
- `From<PlanningDraft>` map (after L94): `gates_confirmed_at: d.gates_confirmed_at.map(|t| t.to_rfc3339()),`

3. **confirm-gates handler + route**:
```rust
async fn confirm_gates(
    State(deployment): State<DeploymentImpl>,
    Path(draft_id): Path<String>,
    // optional DIY edit body:
    body: Option<Json<QualityGateConfig>>,
) -> Result<ResponseJson<ApiResponse<DraftResponse>>, ApiError> {
    let draft = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id).await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::NotFound(format!("Planning draft {draft_id} not found")))?;
    // If a DIY config was supplied, validate + upsert it as the project policy first.
    if let Some(Json(cfg)) = body {
        let errs = cfg.validate();
        if !errs.is_empty() {
            return Err(ApiError::BadRequest(format!("Invalid quality policy: {}", errs.join("; "))));
        }
        let yaml = serde_yaml::to_string(&cfg)
            .map_err(|e| ApiError::Internal(format!("serialize policy: {e}")))?;
        ProjectQualityPolicy::upsert(&deployment.db().pool, draft.project_id, &yaml, cfg.mode_str()).await
            .map_err(|e| ApiError::Internal(format!("upsert policy: {e}")))?;
    }
    PlanningDraft::set_gates_confirmed(&deployment.db().pool, &draft_id).await
        .map_err(|e| ApiError::Internal(format!("set gates_confirmed: {e}")))?;
    let updated = PlanningDraft::find_by_id(&deployment.db().pool, &draft_id).await
        .map_err(|e| ApiError::Internal(format!("Database error: {e}")))?
        .ok_or_else(|| ApiError::Internal("Draft disappeared".into()))?;
    Ok(Json(ApiResponse::success(DraftResponse::from(updated))))
}
```
Route (in `planning_draft_routes()`, after L129 `materialize`):
```rust
.route("/{draft_id}/confirm-gates", post(confirm_gates))
```
> `mode_str()` = serialize `QualityGateMode` to its YAML string (`off|shadow|warn|enforce`); add a tiny helper or `serde_yaml::to_string(&cfg.mode)` trimmed. Add `use quality::config::QualityGateConfig;` and the `ProjectQualityPolicy` import.

### 2.14 `crates/server/src/bin/generate_types.rs` — additions (disjoint from CL-IDE 128-129)
Insert **before L242 `];`** (after L241), leaving L128-129 untouched for CL-IDE:
```rust
        quality::config::QualityGateConfig::decl(),
        quality::config::QualityGateMode::decl(),
        quality::config::GateDefinition::decl(),
        quality::config::ConditionConfig::decl(),
        quality::config::ProvidersConfig::decl(),
        quality::config::SonarConfig::decl(),
        quality::metrics::MetricKey::decl(),
        quality::gate::condition::Operator::decl(),
        server::routes::quality::QualityPolicyResponse::decl(),
        server::routes::quality::MetricCatalogResponse::decl(),
```
> Confirm `quality::gate::condition` path is `pub`. `DraftResponse` is intentionally absent (hand-serded; FE interface edited manually).

---

## 3. Frontend

### 3.1 `frontend/src/lib/api.ts`
- **FE-01 fix (L607):** `formData.append('audit_doc', file);` → `formData.append('file', file);`
- **PlanningDraftResponse (L505):** add `gatesConfirmedAt: string | null;`
- **planningDraftsApi:** add
```ts
confirmGates: async (draftId: string, config?: QualityGateConfig): Promise<PlanningDraftResponse> => {
  const response = await fetch(`/api/planning-drafts/${draftId}/confirm-gates`, {
    method: 'POST',
    headers: config ? { 'Content-Type': 'application/json' } : undefined,
    body: config ? JSON.stringify(config) : undefined,
  });
  return handleApiResponse<PlanningDraftResponse>(response);
},
```
- **qualityPolicyApi (new):** `getDefault()`, `getMetrics()`, `getProject(projectId)`, `putProject(projectId, config)`, `deleteProject(projectId)` — typed against `QualityGateConfig`, `MetricKey`, `QualityPolicyResponse`, `MetricCatalogResponse` imported from `shared/types`.
> Keep all additions in a region SEPARATE from the CL-IDE `openEditor` removal to avoid P4 merge collisions.

### 3.2 `frontend/src/components/quality/QualityGateRulesEditor.tsx` (NEW) — ONE shared stateless controlled editor
Props:
```ts
interface QualityGateRulesEditorProps {
  value: QualityGateConfig;
  defaults: QualityGateConfig;
  metricOptions: MetricKey[];   // from GET /quality/policy/metrics — picker source
  onChange: (next: QualityGateConfig) => void;
  readOnly?: boolean;
  errors?: string[];            // [GRAFT-B] field-level validation errors to surface
}
```
Renders:
- **Mode select**: `off | shadow | warn | enforce` (`QualityGateMode` union).
- **Three gate sections** (terminal / branch / repo), each a condition table; each row = `{ metric <select over metricOptions>, operator <GT|LT select>, threshold <input> }` with add/delete. The metric column is a **`<select>` populated ONLY from `metricOptions`** ⇒ enforces the closed enum; no free text. Sentinel excluded by the endpoint.
- **11 provider checkboxes** (exactly the 11 from config.rs:85-114): `rust, frontend, repo, security, sonar, builtin_rust, builtin_frontend, builtin_common, coverage, completeness, delivery_readiness`.
- **Sonar fields**: `host_url`, `project_key`, `token` (token write-only / masked).
Stateless & props-driven (parents own state + persistence, per `frontend/CLAUDE.md`). Reuse styling tokens from `components/quality/QualityReportPanel` + `QualityIssueList`.

### 3.3 `frontend/src/components/quality/QualityGateConfirmDialog.tsx` (NEW) — G2 popup container
Fetches `qualityPolicyApi.getProject(projectId)` + `getDefault()` + `getMetrics()`, holds editor state, wraps `QualityGateRulesEditor`. Primary action **"Save & Confirm"** → `qualityPolicyApi.putProject(projectId, edited)` (only if edited) then `planningDraftsApi.confirmGates(draftId, edited?)`, then closes and lets the parent call materialize. Cancel closes without confirming (materialize stays blocked).

### 3.4 `frontend/src/pages/ui-new/settings/QualityGateSettingsNew.tsx` (NEW) — G3 standalone page
Project context (selector or current-project), wraps the SAME `QualityGateRulesEditor`. **Save** → `useSaveQualityPolicy` (PUT). **Reset** → `useResetQualityPolicy` (DELETE → falls back to default). Shows the resolved `source` badge (project/file/bundled). Import target lives in `pages/ui-new/settings/`.

### 3.5 `frontend/src/hooks/useQualityPolicy.ts` (NEW)
react-query hooks: `useProjectQualityPolicy(projectId)`, `useDefaultQualityPolicy()`, `useQualityMetricKeys()`, `useSaveQualityPolicy()`, `useResetQualityPolicy()`.

### 3.6 `frontend/src/hooks/usePlanningDraft.ts` — confirm-gates hook
After `useMaterializeDraft` (L137-148):
```ts
export function useConfirmGates() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ draftId, config }: { draftId: string; config?: QualityGateConfig }) =>
      planningDraftsApi.confirmGates(draftId, config),
    onSuccess: (_d, { draftId }) => {
      qc.invalidateQueries({ queryKey: ['planning-draft', draftId] });
    },
  });
}
```

### 3.7 Wire the popup BETWEEN confirmed → materialize
- **`CreateChatBoxContainer.tsx`** (`PlanningStatusBar.handleMaterialize`, button at L241): if `draft.gatesConfirmedAt == null`, open `QualityGateConfirmDialog` instead of calling materialize. After dialog confirms (draft refetched with `gatesConfirmedAt` set), allow/auto-trigger materialize.
- **`PlanningChatContainer.tsx`** (`handleMaterialize`, L137-148): same interception — guard `gatesConfirmedAt == null` → open dialog; only call `materializeMutation.mutateAsync` once gates are confirmed.

### 3.8 Settings nav + route
- **`SettingsLayoutContainer.tsx`** (`navItems`, L24-74): add `{ path: 'quality-gates', label: t('settings:newDesign.nav.qualityGates'), icon: ShieldCheckIcon }` (import `ShieldCheckIcon` from the existing phosphor icon set used in this file).
- **`App.tsx`**: add `<Route path="quality-gates" element={<QualityGateSettingsNew />} />` after L174 (alongside the other settings routes). **Import the page DIRECTLY** — `import { QualityGateSettingsNew } from '@/pages/ui-new/settings/QualityGateSettingsNew';` — NOT via the `@/pages/ui-new/settings` barrel, per P4-FINAL barrel-deletion risk. Also add the export to `pages/ui-new/settings/index.ts` for symmetry (harmless if the barrel survives).
- **i18n**: add `newDesign.nav.qualityGates` to `frontend/src/i18n/locales/en/settings.json` and `zh-Hans/settings.json`.

---

## 4. End-to-end paths

### Happy path (gates confirmed → materialize succeeds)
1. User drives a planning draft to `status = "confirmed"` (System-B audit confirm sets `confirmed_at`).
2. User clicks **Materialize**. FE sees `draft.gatesConfirmedAt == null` → opens `QualityGateConfirmDialog` (does NOT call materialize yet).
3. Dialog loads effective policy via `GET /projects/{id}/quality-policy` (DB-first), metrics via `GET /quality/policy/metrics`, defaults via `GET /quality/policy/default`. User optionally edits rules in `QualityGateRulesEditor`.
4. User clicks **Save & Confirm** → (if edited) `PUT /projects/{id}/quality-policy` (validate→upsert), then `POST /planning-drafts/{id}/confirm-gates` → backend `set_gates_confirmed` stamps `gates_confirmed_at` and returns `DraftResponse{ gatesConfirmedAt: <ts> }`.
5. FE refetches the draft (`gatesConfirmedAt` now set) → calls `POST /planning-drafts/{id}/materialize`. Backend status guard passes, gates guard passes → `Workflow::create` → auto-start spawn → orchestrator runs.
6. At each gate run, the services resolver reads `project_quality_policy` (priority-0) and builds the engine via `from_config` ⇒ the just-saved DIY rules are enforced (terminal L2815, branch/repo L7880). Mid-run G3 edits are picked up because resolution happens per gate run, not at workflow start.

### Hard-block path (gates NOT confirmed → 400, no code written)
1. Caller (FE bug, script, or direct curl) `POST /planning-drafts/{id}/materialize` while `gates_confirmed_at IS NULL`.
2. After the `status != "confirmed"` guard, the new `gates_confirmed_at.is_none()` guard returns **HTTP 400** "Quality gates must be confirmed before materialization…".
3. Because this is BEFORE `Workflow::create` (L996) and the auto-start spawn (L1016), **no workflow is created and no code-writing begins**. The block is backend-enforced and cannot be bypassed by skipping the FE popup.

---

## 5. Implementation ORDER + P4-FINAL coordination

**Order (backend types/migration/ts-rs first so the FE has generated types):**
1. Migrations §1 (both files).
2. `crates/quality/Cargo.toml` ts-rs dep §2.1; ts-rs derives §2.2-2.4; `validate()`; engine split §2.5.
3. `crates/db` model §2.6-2.7 (`ProjectQualityPolicy`, `gates_confirmed_at`, `set_gates_confirmed`).
4. `crates/services` resolver §2.8; thread the 3 sites §2.9.
5. `crates/server` quality dep §2.10; route DTOs/handlers §2.11; mod registration §2.12; planning_drafts hard-block + confirm-gates + DraftResponse §2.13.
6. `generate_types.rs` §2.14 → run `npm run generate-types` → commit regenerated `shared/types.ts` (CI `--check` enforces). **Manually** add `gatesConfirmedAt` to FE `PlanningDraftResponse` (ts-rs does NOT generate it).
7. `cargo build` + `cargo test` backend; only THEN start FE §3 (now `QualityGateConfig`/`MetricKey`/`Operator` exist in `shared/types`).
8. FE api.ts §3.1 (incl. FE-01), shared editor §3.2, dialog §3.3, settings page §3.4, hooks §3.5-3.6, wiring §3.7-3.8.

**P4-FINAL coordination notes:**
- **`planning_drafts.rs`** also touched by RB-49 (feishu refactor): our edits are localized (status-guard insert, new route line, DraftResponse field + From map, new handler) — keep them in distinct hunks.
- **`generate_types.rs`**: our adds go BEFORE L242 (after L241); CL-IDE removes L128-129 (`OpenEditorRequest/Response`) — disjoint.
- **`api.ts`**: CL-IDE removes `openEditor`; keep our `qualityPolicyApi` + `confirmGates` + FE-01 additions in a SEPARATE region.
- **`pages/ui-new/settings/index.ts` / barrel**: import `QualityGateSettingsNew` DIRECTLY in App.tsx so a barrel deletion doesn't break the route.
- **Migration timestamps**: `20260614120000` / `…120001` sort after `20260508000000_add_audit_plan.sql`. **Dedupe against the RB-49 feishu migration timestamp before merge** — if RB-49 also picks a 2026-06-14/15 timestamp, bump ours to avoid an ambiguous apply order.
- **`container.rs:246`** workspace gate: confirm `project_id` reachability before commit; if awkward, leave `from_project` (advisory/shadow path) — the 3 enforcing sites already get DB-priority policy.

---

## 6. Test plan

**Unit (quality crate):**
- `engine::from_config` builds exactly the providers enabled by the supplied config (toggle a provider off → assert absent); confirm `from_project` still delegates and is unchanged.
- `QualityGateConfig::validate()` returns errors for a bad operator/threshold condition and `[]` for the bundled default; round-trips `from_yaml(serde_yaml::to_string(cfg))` byte-stably.
- `MetricKey` TS union excludes `QualityGateEmptyScan` (inspect generated `shared/types.ts` or the `/metrics` list).

**Unit (services):**
- `resolve_quality_config`: (a) DB row present → returns its parsed config; (b) DB row present but YAML corrupt → falls back to file/bundled; (c) no DB row → `load_from_project` path; (d) all-miss → `default_config()`.

**Integration (server, sqlx test pool):**
- Materialize 400-until-confirmed: create draft → `set_confirmed` (status=confirmed, `gates_confirmed_at` NULL) → `POST /materialize` ⇒ **400** and assert NO workflow row created. Then `POST /confirm-gates` → `POST /materialize` ⇒ **200**, workflow row exists, `gates_confirmed_at` set, `confirmed_at` unchanged (no aliasing).
- Confirm-gates with DIY body: invalid metric/operator ⇒ 400; valid ⇒ upserts `project_quality_policy` row + stamps `gates_confirmed_at`.
- CRUD round-trip: `PUT /projects/{id}/quality-policy` (valid config) → `GET` returns `source:"project"` + same config; `DELETE` → `GET` returns `source:"file"|"bundled"` (no row). `PUT` invalid config ⇒ 400 with error list.
- `GET /quality/policy/default` parses `BUNDLED_CENTRAL_POLICY`; `GET /quality/policy/metrics` returns the closed enum minus the sentinel + `["GT","LT"]`.

**FE (vitest/RTL):**
- `QualityGateRulesEditor` renders exactly **11** provider checkboxes and a metric `<select>` whose options equal `metricOptions` (no free-text input).
- Materialize interception: with `gatesConfirmedAt == null` the materialize click opens the dialog and does NOT call `materializeMutation`; after a mocked `confirmGates` success it proceeds.
- FE-01 regression: `uploadAuditDoc` appends multipart field name `file`.

---

## 7. FINAL_SCHEMA (return contract)

```
specPath:            docs/streamline/P3-design-spec.md
chosenMechanism:     Hybrid A — DB project_quality_policy priority-0 resolved by a
                     services-layer free-fn resolver (resolve_quality_config), engine
                     stays DB-free & file-write-free via from_config; G2 = separate
                     planning_draft.gates_confirmed_at column + 400 hard-block in
                     materialize before Workflow::create/auto-start; ONE shared
                     stateless QualityGateRulesEditor (closed-MetricKey picker).
migrationFiles:      20260614120000_create_project_quality_policy.sql
                     20260614120001_add_gates_confirmed_at.sql
```
