# P3 Design A — G2 (mandatory gate-confirm before materialize) + G3 (standalone quality-gate settings)

> Architect angle: **Mechanism A (DB-priority-0 resolution)** — but in the **resolver-injection** variant, because the crate graph forbids the literal form. Verified anchors below.

## 0. Decisive constraint discovered during verification

`quality` crate **does NOT depend on `db`**, and `db` **does NOT depend on `quality`** (verified: `crates/quality/Cargo.toml` has no `db`; `crates/db/Cargo.toml` has no `quality`). `services` depends on both.

Therefore the literal Mechanism A ("DB lookup *inside* `QualityGateConfig::load_from_project`") is **not viable** — it would force `quality → db`, a layering inversion (and risks a cycle once `db` ever needs a metric type). The correct, least-invasive realization of the same intent is:

- Keep `quality` DB-free. Add a pure **`QualityEngine::from_project_with_config(project_root, config)`** constructor.
- Resolve the effective config **DB-first** in the **`services`** layer (which holds `self.db.pool` + `project_id`) and pass it in at the 3 gate sites.
- `from_project(project_root)` stays as a thin fallback: `from_project_with_config(project_root, QualityGateConfig::load_from_project(project_root)?)`.

This is **Mechanism A in spirit** (DB is priority-0), threaded with minimal blast radius and zero new crate deps. Mechanism B (write resolved YAML at confirm) and C (config injection at orchestrator construction) are rejected for reasons in §7.

## 1. Verified anchors (file:line — re-verified this pass)

| Fact | Location | Status |
|---|---|---|
| `QualityGateConfig` shape (mode/3 gates/providers/sonar) | `crates/quality/src/config.rs:20-36` | ✅ |
| `QualityGateMode` off/shadow/warn/enforce, serde lowercase | `config.rs:41-53` | ✅ |
| `GateDefinition{name, conditions}` / `ConditionConfig{metric,operator,threshold}` | `config.rs:56-73` | ✅ |
| `ProvidersConfig` = **11** bool toggles (not 13) | `config.rs:84-114` | ⚠️ Corrected: prompt said 13; actual = rust, frontend, repo, security, sonar, builtin_rust, builtin_frontend, builtin_common, coverage, completeness, delivery_readiness |
| `load_from_project` order: repo yaml → BUNDLED_CENTRAL_POLICY → default_config; **no DB** | `config.rs:181-217` | ✅ |
| `MetricKey` closed enum, `#[serde(rename=...)]` snake_case | `crates/quality/src/metrics.rs:11-12` (44 variants) | ✅ |
| `Operator::from_db_value("GT"\|"LT")` | `crates/quality/src/gate/condition.rs:16,27` | ✅ |
| `QualityEngine::from_project(&Path)` sole constructor, no pool | `crates/quality/src/engine.rs:79` | ✅ |
| Gate call sites: terminal `agent.rs:2815`; final (branch+repo) via `run_final_quality_gate` `agent.rs:7880`; workspace `container.rs:246` | — | ✅ (4 sites total) |
| Agent holds `db: Arc<DBService>` (`self.db.pool`) + `state.workflow_id` | `agent.rs:72`, resolves project via `resolve_project_working_dir`→`Project::find_by_id(workflow.project_id)` `agent.rs:4964-4968` | ✅ |
| Materialize guard = **only** `status=="confirmed"` | `planning_drafts.rs:915-920` | ✅ |
| `confirm_draft` generates System-B AuditPlan + `set_confirmed` | `planning_drafts.rs:286-389` | ✅ |
| `PlanningDraft.confirmed_at` column **already exists** | `crates/db/src/models/planning_draft.rs:24` | ✅ (reuse pattern, but gates need their OWN column) |
| `DraftResponse` DTO (camelCase serde, **not** TS-derived) | `planning_drafts.rs:53-99` | ✅ |
| `upload_audit_doc` expects multipart field **`"file"`** | `planning_drafts.rs:1071-1072` | ✅ |
| FE sends **`"audit_doc"`** (FE-01 bug) | `frontend/src/lib/api.ts:607` | ✅ confirmed broken |
| generate_types decls list | `crates/server/src/bin/generate_types.rs:13-242` | ✅ |
| Routes: `/quality` + `/workflows/.../quality` nested | `routes/mod.rs:170-171`, `quality.rs:153-170` | ✅ |
| Latest migration | `crates/db/migrations/20260508000000_add_audit_plan.sql` | ✅ (new ones must sort after) |
| Settings nav defined in container; routes in App.tsx; barrel `pages/settings/index.ts` re-exports `ui-new/settings` | `SettingsLayoutContainer.tsx:24-74`, `App.tsx:167-176`, `pages/settings/index.ts` | ✅ (barrel slated for deletion P4-FINAL) |

## 2. Data model — new DB tables/columns

### 2a. `project_quality_policy` (per-project DIY override; the DB priority-0 source)
### 2b. `planning_draft.gates_confirmed_at` (G2 hard-gate flag)

Two new migrations (timestamps after `20260508000000`). SQLite (project uses SQLite — `SqlitePool`).

```sql
-- crates/db/migrations/20260615100000_create_project_quality_policy.sql
CREATE TABLE IF NOT EXISTS project_quality_policy (
    project_id   TEXT PRIMARY KEY NOT NULL
                 REFERENCES projects(id) ON DELETE CASCADE,
    -- Full QualityGateConfig serialized as YAML (same schema as quality-gate.yaml).
    -- YAML (not JSON) so it is byte-identical to the file form and round-trips through
    -- QualityGateConfig::from_yaml without a second codec.
    config_yaml  TEXT NOT NULL,
    -- Denormalized for cheap listing/badges without parsing YAML.
    mode         TEXT NOT NULL DEFAULT 'enforce',
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

```sql
-- crates/db/migrations/20260615100001_add_gates_confirmed_at.sql
ALTER TABLE planning_draft ADD COLUMN gates_confirmed_at DATETIME;
```

Notes:
- `project_id` PK = one policy per project (upsert on save).
- `mode` denormalized column lets the settings list show a badge; source of truth is `config_yaml`.
- `gates_confirmed_at` is **separate** from the existing `confirmed_at` (which marks System-B audit-plan confirm). G2 confirms System-A *gates*; the two confirmations are semantically distinct and must not alias.

## 3. Backend — Rust changes

### 3a. `quality` crate — add ts-rs + new constructor (DB-free)

`crates/quality/Cargo.toml` — add:
```toml
ts-rs = { workspace = true }
```
(use workspace pin to match the rest of the repo; if no workspace entry, add `ts-rs = "..."` matching `crates/server`'s pin.)

`crates/quality/src/config.rs`:
- Add `#[derive(TS)] #[ts(export)]` (alongside existing serde) to: `QualityGateConfig`, `QualityGateMode`, `GateDefinition`, `ConditionConfig`, `ProvidersConfig`, `SonarConfig`.
- Add `#[derive(TS)]` to `MetricKey` (`metrics.rs`) and `Operator` (`gate/condition.rs`) — so the FE picker gets a generated string-union of the **closed** metric set (the MetricKey-closed-enum constraint is then enforced *at the type level* in TS, not just by convention).
- Add a **validation** method used by the API on save:
  ```rust
  impl QualityGateConfig {
      /// Validate every condition round-trips (operator parses, metric is in-enum).
      pub fn validate(&self) -> anyhow::Result<()> {
          for g in [&self.terminal_gate, &self.branch_gate, &self.repo_gate] {
              for c in &g.conditions { c.to_condition()?; }
          }
          Ok(())
      }
  }
  ```

`crates/quality/src/engine.rs` — split the constructor:
```rust
impl QualityEngine {
    /// Pure: build providers from an already-resolved config. No file/DB access.
    pub fn from_project_with_config(_project_root: &Path, config: QualityGateConfig) -> anyhow::Result<Self> {
        // (move the existing provider-construction body of from_project here verbatim)
    }

    /// Backward-compatible: resolve via file/bundled and delegate. Unchanged behavior.
    pub fn from_project(project_root: &Path) -> anyhow::Result<Self> {
        let config = QualityGateConfig::load_from_project(project_root)?;
        Self::from_project_with_config(project_root, config)
    }
}
```
This is the minimal change: existing callers keep working; new callers can pass a DB-resolved config.

### 3b. `db` crate — new model `ProjectQualityPolicy`

`crates/db/src/models/project_quality_policy.rs` (new), exported in `crates/db/src/models/mod.rs`:
```rust
#[derive(Debug, Clone, FromRow)]
pub struct ProjectQualityPolicy {
    pub project_id: String,
    pub config_yaml: String,
    pub mode: String,
    pub updated_at: DateTime<Utc>,
    pub created_at: DateTime<Utc>,
}
impl ProjectQualityPolicy {
    pub async fn find_by_project(pool: &SqlitePool, project_id: &str) -> sqlx::Result<Option<Self>>;
    pub async fn upsert(pool: &SqlitePool, project_id: &str, config_yaml: &str, mode: &str) -> sqlx::Result<()>; // INSERT .. ON CONFLICT(project_id) DO UPDATE
    pub async fn delete(pool: &SqlitePool, project_id: &str) -> sqlx::Result<()>;
}
```
`db` stays free of `quality` (stores opaque YAML text only — no `QualityGateConfig` type leakage). ✅ no new crate dep.

`crates/db/src/models/planning_draft.rs`:
- Add field `pub gates_confirmed_at: Option<DateTime<Utc>>;` to the struct.
- Add `set_gates_confirmed(pool, id)` (`UPDATE planning_draft SET gates_confirmed_at = datetime('now'), updated_at = datetime('now') WHERE id = ?1`).
- Update any explicit column SELECT lists for `planning_draft` to include the new column (the struct is `FromRow`).

### 3c. `services` crate — DB-first resolver, threaded at the 3 functional gate sites

New helper in `services` (e.g. `crates/services/src/services/orchestrator/quality_policy.rs`):
```rust
/// Mechanism A: DB priority-0, then file/bundled fallback.
pub async fn resolve_quality_config(
    pool: &SqlitePool,
    project_id: &Uuid,
    project_root: &Path,
) -> quality::config::QualityGateConfig {
    if let Ok(Some(row)) = db::models::ProjectQualityPolicy::find_by_project(pool, &project_id.to_string()).await {
        match quality::config::QualityGateConfig::from_yaml(&row.config_yaml) {
            Ok(cfg) => return cfg,
            Err(e) => tracing::error!(error=%e, %project_id, "project_quality_policy YAML invalid; falling back to file/bundled"),
        }
    }
    quality::config::QualityGateConfig::load_from_project(project_root)
        .unwrap_or_else(|_| quality::config::QualityGateConfig::default_config())
}
```
Call-site edits (replace `QualityEngine::from_project(p)` with resolve→`from_project_with_config`):
- `agent.rs:2815` (terminal): `self` has `self.db.pool` + workflow→`project_id`. Resolve, then `from_project_with_config(wd, cfg)`.
- `agent.rs:7880` (`run_final_quality_gate`, covers **branch** 7810 and repo): same.
- `container.rs:246` (workspace gate): this `tokio::spawn` already has `pool`; it resolves `repo_path` from `WorkspaceRepo`. Add a `project_id` lookup (workspace→project) or, if not readily available, fall back to `from_project` here (workspace gate is advisory `Terminal`-level). **Decision:** thread it in `agent.rs` (the two enforce-mode sites) for certain; `container.rs` may keep `from_project` if project_id is awkward there — document as acceptable because that path is shadow/advisory. Re-verify `container.rs` workspace→project reachability before finalizing.

`project_id` at agent sites: `self.resolve_project_working_dir()` already loads the `Workflow` then `Project`; capture `workflow.project_id` alongside `wd` (the function at `agent.rs:4964` already fetches the workflow — return/stash the id, or do a second cheap `Workflow::find_by_id` which the agent does routinely).

### 3d. `server` crate — new REST endpoints

New file `crates/server/src/routes/quality_policy.rs`, nested under existing `/quality` in `routes/mod.rs:171` (extend `quality::quality_routes()` or add a sibling `quality_policy_routes()`):

| Method | Path | Handler | Body / Resp |
|---|---|---|---|
| GET | `/api/quality/policy/default` | `get_default_policy` | → `QualityGateConfig` (parsed `BUNDLED_CENTRAL_POLICY`) |
| GET | `/api/quality/policy/metrics` | `list_metric_keys` | → `MetricKey[]` (all variants) for the picker; plus operators `["GT","LT"]` |
| GET | `/api/projects/{project_id}/quality-policy` | `get_project_policy` | → `{ source: "project"\|"file"\|"bundled", config: QualityGateConfig }` (DB-first resolve, same order as §3c) |
| PUT | `/api/projects/{project_id}/quality-policy` | `put_project_policy` | body `QualityGateConfig` → `validate()` → serialize YAML → `upsert`; 400 on invalid |
| DELETE | `/api/projects/{project_id}/quality-policy` | `delete_project_policy` | reset to default (delete row) |

Handler sketch for PUT (the validation + closed-enum enforcement point):
```rust
async fn put_project_policy(State(dep), Path(pid): Path<String>, Json(cfg): Json<QualityGateConfig>)
  -> Result<Json<ApiResponse<()>>, ApiError> {
    cfg.validate().map_err(|e| ApiError::BadRequest(format!("invalid policy: {e}")))?;
    let yaml = serde_yaml::to_string(&cfg).map_err(|e| ApiError::Internal(e.to_string()))?;
    let mode = serde_yaml::to_value(&cfg.mode).ok()
        .and_then(|v| v.as_str().map(String::from)).unwrap_or("enforce".into());
    db::models::ProjectQualityPolicy::upsert(&dep.db().pool, &pid, &yaml, &mode).await
        .map_err(|e| ApiError::Internal(e.to_string()))?;
    Ok(Json(ApiResponse::success(())))
}
```
`server` already depends on `quality`? — **verify**: if not, add `quality = { path = "../quality" }` to `crates/server/Cargo.toml` (server depends on `db`, `services`, etc.; adding `quality` is acceptable — `services` already pulls it). The `QualityGateConfig` must deserialize from JSON (FE sends JSON) — it already derives `Deserialize`, and YAML serialize for storage. ✅

### 3e. `server` — G2 materialize hard-gate

`crates/server/src/routes/planning_drafts.rs`:
1. **`materialize_draft` (L915)** — add gate check immediately after the status guard:
```rust
if draft.status != "confirmed" { /* existing 400 */ }

// G2: hard-block until System-A quality gates are explicitly confirmed.
if draft.gates_confirmed_at.is_none() {
    return Err(ApiError::BadRequest(
        "Quality gates must be confirmed before materialization. \
         POST /api/planning-drafts/{id}/confirm-gates first.".to_string(),
    ));
}
```
This is the **exact hard-block**: a 400 from the backend whenever `gates_confirmed_at IS NULL`. The FE cannot bypass it (the existing `confirm`→`materialize` two-call sequence now physically fails at materialize unless gates were confirmed).

2. New endpoint `POST /api/planning-drafts/{draft_id}/confirm-gates` (`confirm_gates` handler), registered in `planning_draft_routes()` (`planning_drafts.rs:123-141`):
```rust
.route("/{draft_id}/confirm-gates", post(confirm_gates))
```
Handler: load draft → require `status in ("spec_ready","gathering","confirmed")` (must be confirmable but not yet materialized) → `PlanningDraft::set_gates_confirmed(pool, id)` → return `DraftResponse`. It does **not** itself write to `project_quality_policy`; the editor's PUT (§3d) already persisted the DIY override (or the user accepted defaults). `confirm-gates` only records the human approval timestamp that unlocks materialize.
   - Optional convenience: accept an optional body `{ config?: QualityGateConfig }` so the popup can persist+confirm in one call (PUT-then-confirm). Keep it optional; the shared editor can also call PUT separately.

3. **`DraftResponse` DTO** — add `pub gates_confirmed_at: Option<String>` (rfc3339) + map in `From<PlanningDraft>` (`planning_drafts.rs:76-98`). DraftResponse is **not** TS-derived (manual serde camelCase), so update the FE interface by hand (§5).

### 3f. FE-01 bug fix (backend side is correct; fix is FE-only)
Backend already expects `"file"` (`planning_drafts.rs:1072`). Fix is in `api.ts` (§5). No backend change.

## 4. ts-rs pipeline

`crates/server/src/bin/generate_types.rs` — add to the `decls` vec (keep **disjoint** from the CL-IDE removal of lines 128-129 per P4-FINAL coordination; append at the end of the quality block near line 234):
```rust
quality::config::QualityGateConfig::decl(),
quality::config::QualityGateMode::decl(),
quality::config::GateDefinition::decl(),
quality::config::ConditionConfig::decl(),
quality::config::ProvidersConfig::decl(),
quality::config::SonarConfig::decl(),
quality::metrics::MetricKey::decl(),
quality::gate::condition::Operator::decl(),
```
Then `npm run generate-types` regenerates `shared/types.ts`; CI `--check` validates. (Requires `generate_types` bin's crate to depend on `quality` — it depends on `server`/`services`/`db`/`executors`/`utils` today; add `quality` to that bin crate's deps if absent.)

## 5. Frontend changes

### 5a. FE-01 fix (one line)
`frontend/src/lib/api.ts:607` — `formData.append('audit_doc', file)` → `formData.append('file', file)`.

### 5b. API client additions (`api.ts`)
- Extend `PlanningDraftResponse` (L505): add `gatesConfirmedAt: string | null;`.
- Add to `planningDraftsApi` (L540): `confirmGates(draftId, config?)` → `POST .../confirm-gates`.
- New `qualityPolicyApi` object:
  - `getDefault(): Promise<QualityGateConfig>` → `GET /api/quality/policy/default`
  - `getMetrics(): Promise<MetricKey[]>` → `GET /api/quality/policy/metrics`
  - `getProject(projectId): Promise<{source; config: QualityGateConfig}>`
  - `putProject(projectId, config): Promise<void>`
  - `deleteProject(projectId): Promise<void>`
- (CL-IDE removes `openEditor` from api.ts in P4-FINAL — keep these additions in a separate region to avoid merge collision.)

Types `QualityGateConfig`, `MetricKey`, `Operator`, etc. import from `shared/types` (generated in §4).

### 5c. The ONE shared editor component (serves G2 popup + G3 settings)

New presentational component:
`frontend/src/components/quality/QualityGateEditor.tsx`
```tsx
interface QualityGateEditorProps {
  value: QualityGateConfig;
  defaults: QualityGateConfig;          // for "reset to default"
  metricOptions: MetricKey[];           // closed picker set from getMetrics()
  onChange: (next: QualityGateConfig) => void;
  readOnly?: boolean;
}
```
- **Stateless/controlled** (matches frontend CLAUDE.md "view components are stateless"). Parent owns state + persistence.
- Renders: mode `<select>` (off/shadow/warn/enforce); three gate sections (terminal/branch/repo) each a condition table with rows = `{ metric <picker>, operator <GT|LT select>, threshold <input> }` + add/delete row; provider toggles (11 checkboxes); Sonar host/key/token fields.
- **MetricKey closed-enum constraint:** the metric column is a `<select>` populated **only** from `metricOptions` (the generated `MetricKey` union). No free-text — users cannot invent metrics at runtime. Surface a helptext: "Metrics are fixed; pick from the supported set."
- Reuse styling tokens from `components/quality/QualityReportPanel`/`QualityIssueList` (bg-secondary, rounded, text-normal etc. per `frontend/CLAUDE.md`).

**G2 popup wrapper** — `frontend/src/components/quality/QualityGateConfirmDialog.tsx` (container):
- Opened from the planning flow before materialize. Fetches `getProject(projectId)` (DB-first; falls back to file/bundled) + `getDefault()` + `getMetrics()`.
- Holds local draft state, renders `<QualityGateEditor>`, has "Save & Confirm" → `qualityPolicyApi.putProject(projectId, draft)` then `planningDraftsApi.confirmGates(draftId)`; "Use defaults & Confirm" → `confirmGates` only.
- On success, enables the existing Materialize button.

**G3 settings page** — `frontend/src/pages/ui-new/settings/QualityGateSettingsNew.tsx` (container+view, or a thin page that reuses the same editor):
- Needs a project context (per-project DIY). Use the active project from `ProjectContext`/route. Fetches the same three queries, renders `<QualityGateEditor>`, "Save" → `putProject`, "Reset to default" → `deleteProject`.
- **Import directly** (NOT via the deleted `pages/settings/index.ts` barrel). Add export to `pages/ui-new/settings/index.ts` only if that barrel survives P4-FINAL; otherwise import the file path directly in App.tsx.

### 5d. Wire the G2 popup into both planning containers
`CreateChatBoxContainer.tsx` (`handleMaterialize` L334) and `PlanningChatContainer.tsx` (`handleMaterialize`):
- Intercept: if `draft.gatesConfirmedAt == null`, open `<QualityGateConfirmDialog>` instead of calling materialize. Materialize proceeds only after `confirmGates` succeeds (and the backend 400 is the hard backstop if FE is bypassed).
- This is the FE half of the "mandatory secondary confirmation between plan-confirm and materialize."

### 5e. New settings nav entry + route
- `SettingsLayoutContainer.tsx:24` — add nav item `{ path: 'quality-gates', label: t('settings:newDesign.nav.qualityGates','Quality Gates'), icon: ShieldCheckIcon }`.
- `App.tsx:176` — add `<Route path="quality-gates" element={<QualityGateSettingsNew />} />` and import the page **directly** from `@/pages/ui-new/settings/QualityGateSettingsNew` (avoid the doomed barrel).
- i18n: add `newDesign.nav.qualityGates` key to `settings` locale files (en + zh at minimum).

### 5f. New hooks
`frontend/src/hooks/useQualityPolicy.ts`: `useProjectQualityPolicy(projectId)`, `useDefaultQualityPolicy()`, `useQualityMetricKeys()`, `useSaveQualityPolicy()`, `useResetQualityPolicy()`, `useConfirmGates()` (react-query, mirrors `usePlanningDraft.ts` patterns).

## 6. End-to-end flow (G2)

1. User plans → `confirm_draft` runs (System-B audit plan, `status=confirmed`, `confirmed_at` set). **No gates confirmed yet** → `gates_confirmed_at` still NULL.
2. User clicks Materialize → FE sees `gatesConfirmedAt==null` → opens `QualityGateConfirmDialog` (DB-first config or defaults, editable via shared editor).
3. User reviews/edits System-A gates → "Save & Confirm" → `PUT project quality-policy` (persists DIY) + `POST confirm-gates` (`gates_confirmed_at = now`).
4. FE calls `materialize` → backend guard passes (status=confirmed AND gates_confirmed_at not null) → workflow created + auto-start.
5. If FE ever skips step 3, backend returns **400** at materialize. Hard-blocked. ✅
6. Orchestrator gates (terminal/branch/repo) now resolve config **DB-first** (§3c) → the user's DIY policy is what actually enforces during code-writing.

## 7. Why Mechanism A (resolver-injection), not B or C

- **A (chosen):** DB priority-0 read at the gate sites where `pool`+`project_id` already exist; `quality` stays DB-free; runtime always sees the freshest policy (no staleness). One new pure constructor + a 3-site swap. **Least invasive that still honors live edits.**
- **B (write resolved YAML to repo at confirm):** would let `load_from_project` pick it up file-first with zero engine change — but (1) pollutes the user's output repo with a generated file, (2) goes **stale** if the user edits the policy in settings after materialize, (3) races with repo-local `quality/quality-gate.yaml` precedence. Rejected.
- **C (inject config into OrchestratorAgent at construction):** config captured once at workflow start → cannot reflect mid-run settings edits, and the agent constructs the engine per-gate from a `&Path` today, so it is no less invasive than A. Rejected.
- **Hybrid note:** A + the existing file/bundled fallback chain *is* the hybrid — DB → repo yaml → bundled → default. That ordering is implemented in `resolve_quality_config` (§3c), not inside `quality`.

## 8. Risks / coordination

- **Crate-dep additions:** `quality` gains `ts-rs`; `server` (and the `generate_types` bin) may need `quality` dep. Verify before edit; both are downstream of `services` which already depends on `quality`, so no cycle.
- **P4-FINAL collisions:** `planning_drafts.rs` (RB-49 feishu refactor), `generate_types.rs` (CL-IDE removes 128-129 — our adds go at ~234, disjoint), `api.ts` (CL-IDE removes `openEditor` — our adds in separate region), `pages/settings/index.ts` barrel **deletion** — new page imports directly.
- **`container.rs:246` project_id reachability** unverified for the workspace gate; acceptable to leave `from_project` there (advisory path) — confirm before committing.
- **`ProvidersConfig` is 11 toggles, not 13** (prompt drift) — editor must render the actual 11.
- **DraftResponse is hand-serded**, not ts-rs — FE interface edit is manual; easy to forget the `gatesConfirmedAt` field.
- **MetricKey closed enum** — picker only; document for users that new metrics require a code change (new enum variant + provider), not a settings edit.
- **Migration ordering** — both new migrations must sort after `20260508000000`; SQLite `ALTER TABLE ADD COLUMN` is safe (nullable, no default backfill needed).
