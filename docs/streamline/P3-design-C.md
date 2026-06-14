# P3 Design C — Mandatory Quality-Gate Confirmation (G2) + Standalone Quality-Gate Settings (G3)

**Branch:** `refactor/streamline-and-quality-gate-rules`
**Angle:** Mechanism **C** (resolve config at the orchestrator layer where `self.db.pool` exists; pass it down via `QualityEngine::from_config`). No file writes by the engine, minimal quality-crate surface change.
**Status of anchors:** all file:line anchors below re-verified against the working tree on 2026-06-14.

---

## 0. Verified current-state anchors

| Claim | Verified location |
|---|---|
| `QualityGateConfig` shape (mode/terminal_gate/branch_gate/repo_gate/providers/sonar), all serde round-trippable, NONE derive `TS` | `crates/quality/src/config.rs:20-159` |
| `QualityGateMode` = off/shadow/warn/enforce, `#[serde(rename_all="lowercase")]`, default Shadow | `config.rs:41-53` |
| `ConditionConfig { metric: MetricKey, operator: String, threshold: String }` + `to_condition()` | `config.rs:65-81` |
| `Operator` closed set = **GT / LT only** (`from_db_value` bails on anything else) | `crates/quality/src/gate/condition.rs:16-32` |
| `MetricKey` CLOSED enum, **44 variants** (incl. internal sentinel `QualityGateEmptyScan`), `as_str()`/`display_name()` exhaustive matches | `crates/quality/src/metrics.rs:11-219` |
| `load_from_project` order: repo-local yaml/yml/.quality-gate.yaml → `BUNDLED_CENTRAL_POLICY` → `default_config()`. **No DB lookup.** | `config.rs:181-217` |
| `QualityEngine::from_project(&Path)` is the ONLY constructor; builds providers from `config.providers`; no DB pool | `crates/quality/src/engine.rs:79-141` |
| `QualityEngine::new(config, providers)` exists (pub) | `engine.rs:72-76` |
| Terminal gate site — inside `handle_checkpoint_quality_gate(&self,…)` (2451) but the `from_project(wd)` call runs inside `tokio::spawn(async move {…})` that **deliberately does not capture `&self`** | `agent.rs:2451`, spawn `agent.rs:2603`, call `agent.rs:2815` |
| Branch/Repo gate site — `run_final_quality_gate(&self, …, project_root, scope)` calls `from_project(project_root)` directly (has `&self`) | `agent.rs:7848`, call `agent.rs:7880` |
| Orchestrator holds `db: Arc<DBService>` and `config: OrchestratorConfig` | `agent.rs:67-81` |
| `confirm_draft` generates System-B AuditPlan + `set_confirmed`; status guard accepts `gathering`/`spec_ready` | `crates/server/src/routes/planning_drafts.rs:286-390` |
| `materialize_draft` guard is ONLY `status != "confirmed"` (line 915) → creates Workflow → `set_materialized` → `tokio::spawn(auto_prepare_and_start)` | `planning_drafts.rs:904-1034` |
| `DraftResponse` DTO (camelCase, Serialize, NOT TS) | `planning_drafts.rs:53-99` |
| `PlanningDraft` model + `set_confirmed`/`set_materialized` | `crates/db/models/planning_draft.rs:11,223,243` |
| ts-rs pipeline: `decls: Vec<String>` in `generate_types.rs:13-242`; CL-IDE removes lines **128-129** (`OpenEditorRequest/Response`) | `crates/server/src/bin/generate_types.rs` |
| quality crate has **no ts-rs dep** | `crates/quality/Cargo.toml:12-28` |
| **FE-01 bug**: FE sends multipart field `audit_doc`; backend requires `file` → upload 100% broken | FE `frontend/src/lib/api.ts:607`, BE `planning_drafts.rs:1072` |
| Route registration: `.nest("/planning-drafts", …)`, `.nest("/quality", quality::quality_routes())` | `crates/server/src/routes/mod.rs:166,171` |
| FE planning flow: `CreateChatBoxContainer.tsx` calls `useConfirmDraft`→`useMaterializeDraft` with NO dialog between | `frontend/src/components/ui-new/containers/CreateChatBoxContainer.tsx:320-349` |
| `PlanningDraftResponse` TS interface ~505; `planningDraftsApi.confirm`/`.materialize`/`.uploadAuditDoc` ~589-617 | `frontend/src/lib/api.ts` |
| Settings pages live in `frontend/src/pages/ui-new/settings/*New.tsx`; barrel `pages/settings/index.ts` slated for deletion (P4-FINAL) | dir listing |
| Existing reusable quality components | `frontend/src/components/quality/` |

---

## 1. Approach summary

**One canonical artifact: `QualityGateConfig` (System A).** It is what G2 confirms and what G3 edits. We expose it to TypeScript via ts-rs and persist per-project overrides in a new `project_quality_policy` row (one per project, latest-wins). The bundled central YAML stays the built-in default.

**G2 (mandatory gate confirm):** add a `gates_confirmed_at` timestamp column to `planning_draft`. `materialize_draft` HARD-BLOCKS (`400`) when it is NULL. A new endpoint `POST /planning-drafts/:id/confirm-gates` accepts the (possibly edited) `QualityGateConfig`, validates it, upserts it into `project_quality_policy` for the draft's project, and stamps `gates_confirmed_at`. The frontend inserts a blocking popup (the shared editor) between confirm and materialize.

**G3 (standalone settings):** a new `QualitySettingsNew.tsx` page renders the SAME editor component bound to `project_quality_policy` via `GET/PUT /projects/:project_id/quality-policy` (+ `GET /quality/metrics` catalog + `GET /quality/policy/defaults`). Built-in defaults are offered as a "reset to defaults" action.

**Mechanism C delivery:** add `QualityEngine::from_config(config, project_root)`. The orchestrator resolves the effective config **DB → repo-file → bundled** at the layer where `self.db.pool` and the workflow's `project_id` are available, then constructs the engine with the resolved config. All three gate sites switch from `from_project` to `from_config`.

---

## 2. DB schema (migration SQL)

New migration `crates/db/migrations/20260614120000_add_quality_policy_and_gate_confirm.sql`:

```sql
-- G2: mandatory quality-gate confirmation timestamp on the planning draft.
-- NULL = gates not yet confirmed → materialize is hard-blocked (400).
ALTER TABLE planning_draft ADD COLUMN gates_confirmed_at DATETIME;

-- G3: per-project DIY override of the System-A quality gate policy.
-- One latest-wins row per project (UNIQUE project_id). Absence = use bundled
-- central policy. config_yaml is the canonical serde-serialized QualityGateConfig.
CREATE TABLE IF NOT EXISTS project_quality_policy (
    id            TEXT PRIMARY KEY NOT NULL,
    project_id    TEXT NOT NULL,
    config_yaml   TEXT NOT NULL,            -- serde_yaml of QualityGateConfig (canonical)
    mode          TEXT NOT NULL DEFAULT 'enforce',  -- denormalized for quick reads/filtering
    updated_by    TEXT,                     -- optional: which surface wrote it (settings|confirm)
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id)
);

CREATE INDEX IF NOT EXISTS idx_project_quality_policy_project
    ON project_quality_policy(project_id);
```

Notes:
- `config_yaml` (not JSON) keeps it byte-compatible with `QualityGateConfig::from_yaml` / `serde_yaml::to_string`, the same format as `BUNDLED_CENTRAL_POLICY` and repo-local files — zero new parsing path.
- `mode` is denormalized only as a convenience column; `config_yaml` is authoritative.
- `project_id` stored as TEXT (sqlite) consistent with `planning_draft.project_id` handling elsewhere; bind via `.to_string()`.
- No `.down.sql` is strictly required by this repo's pattern (most migrations are forward-only), but add one dropping the table + column if the pair convention is followed in the touched range.

---

## 3. Backend changes

### 3.1 quality crate (`crates/quality`)

**`Cargo.toml`** — add ts-rs:
```toml
ts-rs = { workspace = true }
```

**`src/config.rs`** — derive `TS` on the wire structs and add a validator + constructors-from-DB:
- Add `#[derive(TS)]` (alongside existing derives) to: `QualityGateConfig`, `QualityGateMode`, `GateDefinition`, `ConditionConfig`, `ProvidersConfig`, `SonarConfig`.
  - `SonarConfig.token: Option<String>` is a secret-ish field; keep it serialized for round-trip but the API layer (3.2) must strip it on read responses (return `null`) and preserve-on-write if the client omits it.
- Add `impl QualityGateConfig { pub fn validate(&self) -> anyhow::Result<()> }`:
  - every `ConditionConfig.operator` parses via `Operator::from_db_value` (GT/LT only);
  - every `threshold` parses as f64 (reuse `Condition::parse_threshold_f64` semantics);
  - `metric` is implicitly validated by serde (closed enum) — invalid metric fails deserialization before reaching `validate`.
- Add `pub fn bundled_default() -> Self` returning `Self::from_yaml(BUNDLED_CENTRAL_POLICY).unwrap_or_else(|_| Self::default_config())` so the API/defaults endpoint reuses the exact built-in.

**`src/engine.rs`** — Mechanism C constructor (minimal change; refactor `from_project` body to delegate):
```rust
/// Construct an engine from an already-resolved config (Mechanism C).
/// Lets the orchestrator layer resolve DB → repo-file → bundled before
/// instantiation, since the engine itself has no DB pool.
pub fn from_config(config: QualityGateConfig, _project_root: &Path) -> anyhow::Result<Self> {
    // (identical provider-construction block currently in from_project,
    //  moved here; from_project becomes: load_from_project → from_config)
    Ok(Self::new(config, build_providers(&config)))
}

pub fn from_project(project_root: &Path) -> anyhow::Result<Self> {
    let config = QualityGateConfig::load_from_project(project_root)?;
    Self::from_config(config, project_root)
}
```
`build_providers(&config)` is the existing provider-toggle block (engine.rs:83-138) extracted to a private fn. `from_project` is preserved for all non-orchestrator callers and tests.

**`src/metrics.rs`** — add a catalog helper for the picker (no enum change):
```rust
impl MetricKey {
    pub const ALL: &'static [MetricKey] = &[ /* all 44, EXCLUDING QualityGateEmptyScan */ ];
}
```
Exclude `QualityGateEmptyScan` (internal sentinel) from the picker list. The catalog API (3.2) maps `ALL` → `{ key: as_str(), label: display_name() }`.

### 3.2 server crate (`crates/server`)

**New module `crates/server/src/routes/quality_policy.rs`** (or extend `routes/quality.rs`):

DTOs (derive `Serialize, Deserialize, TS`, camelCase):
- `QualityPolicyResponse { config: QualityGateConfig, source: String /* "project" | "bundled" */, updatedAt: Option<String> }`
- `UpdateQualityPolicyRequest { config: QualityGateConfig }`
- `MetricCatalogEntry { key: String, label: String }`
- `ConfirmGatesRequest { config: QualityGateConfig }`

Endpoints:
| Method + path | Handler | Purpose |
|---|---|---|
| `GET /quality/metrics` | `list_metrics` | returns `Vec<MetricCatalogEntry>` from `MetricKey::ALL` (G3 picker; static, no DB) |
| `GET /quality/policy/defaults` | `get_policy_defaults` | returns `QualityGateConfig::bundled_default()` (G3 "reset to defaults") |
| `GET /projects/{project_id}/quality-policy` | `get_project_policy` | DB row → `{config, source:"project"}`; if none → `{bundled_default(), source:"bundled"}`. Strips `sonar.token`. |
| `PUT /projects/{project_id}/quality-policy` | `put_project_policy` | `validate()` then upsert `project_quality_policy`; preserve existing `sonar.token` if request omits it; 400 on validation error |

Registration in `mod.rs`:
- `.nest("/quality", quality::quality_routes())` already exists → add the two new `/quality/...` routes inside `quality_routes()` (or merge a new `quality_policy::routes()`).
- Project-scoped routes go on the existing `projects::router(&deployment)` merge (add `/{project_id}/quality-policy` GET+PUT there to share the `/projects` prefix), OR `.nest("/projects", quality_policy::project_routes())`. Prefer adding to `projects` router to match prefix conventions.

**New DB model `crates/db/src/models/project_quality_policy.rs`** (mirror `quality_policy_snapshot.rs` style; register in `models/mod.rs`):
```rust
pub struct ProjectQualityPolicy { id, project_id, config_yaml, mode, updated_by, created_at, updated_at }
impl ProjectQualityPolicy {
    pub async fn find_by_project(pool, project_id: &str) -> sqlx::Result<Option<Self>>;
    pub async fn upsert(pool, project_id: &str, config_yaml: &str, mode: &str, updated_by: Option<&str>) -> sqlx::Result<()>;
    // upsert = INSERT ... ON CONFLICT(project_id) DO UPDATE SET config_yaml=…, mode=…, updated_at=datetime('now')
}
```

**`crates/db/src/models/planning_draft.rs`** — extend:
- add `pub gates_confirmed_at: Option<DateTime<Utc>>` to the struct + `new()` (None) + the INSERT column list/binds (config.rs pattern at lines 112-135).
- add `pub async fn set_gates_confirmed(pool, id: &str) -> sqlx::Result<()>` → `UPDATE planning_draft SET gates_confirmed_at = datetime('now'), updated_at = datetime('now') WHERE id = ?1`.

**`crates/server/src/routes/planning_drafts.rs`** changes:

1. `DraftResponse` (53-99): add field `pub gates_confirmed_at: Option<String>` + map `d.gates_confirmed_at.map(|t| t.to_rfc3339())` in `From`.

2. New handler + route `POST /{draft_id}/confirm-gates` (G2 write path):
```rust
async fn confirm_gates(State(deployment), Path(draft_id), Json(req): Json<ConfirmGatesRequest>)
  -> Result<Json<ApiResponse<DraftResponse>>, ApiError> {
    let draft = PlanningDraft::find_by_id(...).await?.ok_or(NotFound)?;
    // gates may only be confirmed after spec confirm, before materialize
    if draft.status != "confirmed" {
        return Err(ApiError::BadRequest("gates can only be confirmed for 'confirmed' drafts".into()));
    }
    req.config.validate().map_err(|e| ApiError::BadRequest(format!("invalid quality policy: {e}")))?;
    let yaml = serde_yaml::to_string(&req.config).map_err(...)?;
    let mode = serde-derived lowercase mode string;
    ProjectQualityPolicy::upsert(&pool, &draft.project_id.to_string(), &yaml, &mode, Some("confirm")).await?;
    PlanningDraft::set_gates_confirmed(&pool, &draft_id).await?;
    // return refreshed DraftResponse
}
```
Add `.route("/{draft_id}/confirm-gates", post(confirm_gates))` to `planning_draft_routes()` (mod at planning_drafts.rs:123-141).

3. **Materialize hard-gate** (`materialize_draft`, the block at lines 915-920): after the existing `status != "confirmed"` check, add:
```rust
if draft.gates_confirmed_at.is_none() {
    return Err(ApiError::BadRequest(
        "Quality gates must be confirmed before materializing. \
         Call POST /planning-drafts/{id}/confirm-gates first.".into()));
}
```
This is the backend HARD BLOCK: even if the FE is bypassed, materialize returns 400 until `gates_confirmed_at` is set.

4. **FE-01 fix (backend side is already correct — it expects `file`)**: only the FE changes (see 4.4). No backend change needed; note it here so the FE fix is not lost.

### 3.3 Orchestrator (Mechanism C wiring) — `crates/services/src/services/orchestrator/agent.rs`

Add a private resolver method on `OrchestratorAgent`:
```rust
async fn resolve_quality_config(&self, project_root: &Path) -> QualityGateConfig {
    // 1. DB project override (NEW — highest priority)
    if let Some(pid) = self.current_project_id().await {   // from workflow.project_id
        if let Ok(Some(row)) = db::models::ProjectQualityPolicy::find_by_project(&self.db.pool, &pid).await {
            if let Ok(cfg) = QualityGateConfig::from_yaml(&row.config_yaml) { return cfg; }
            tracing::warn!("project_quality_policy row failed to parse; falling back to file/bundled");
        }
    }
    // 2/3. repo-file → bundled → default (existing loader)
    QualityGateConfig::load_from_project(project_root).unwrap_or_else(|_| QualityGateConfig::default_config())
}
```
`current_project_id()` reads the workflow via `Workflow::find_by_id(&self.db.pool, &state.workflow_id)` (pattern at agent.rs:393) → `.project_id`.

**Branch/Repo site** (`run_final_quality_gate`, 7848-7880) — has `&self`:
```rust
let config = self.resolve_quality_config(project_root).await;
let report = match quality::engine::QualityEngine::from_config(config, project_root) { ... };
```

**Terminal site** (2451 `handle_checkpoint_quality_gate`, call at 2815 inside `tokio::spawn(async move {…})`):
The spawn deliberately does NOT capture `&self` (agent.rs:2606 comment). So resolve **before** the spawn and move the value in:
```rust
// BEFORE tokio::spawn (where &self/self.db.pool are available, ~2600):
let resolved_quality_config = self.resolve_quality_config(wd).await; // wd resolved earlier
// ... add `resolved_quality_config` to the captured set of the `async move` ...
// INSIDE the spawn, replace line 2815:
match quality::engine::QualityEngine::from_config(resolved_quality_config, wd) { ... }
```
`resolved_quality_config` is `Send` (plain serde struct) so it moves cleanly into the task. This is the one structural subtlety — call out in PR description.

`from_project` remains untouched for all other callers/tests.

### 3.4 ts-rs registration — `crates/server/src/bin/generate_types.rs`

Add to the `decls` vec (append near the existing quality decls at lines 228-234, keep DISJOINT from the CL-IDE removal at lines 128-129):
```rust
quality::config::QualityGateConfig::decl(),
quality::config::QualityGateMode::decl(),
quality::config::GateDefinition::decl(),
quality::config::ConditionConfig::decl(),
quality::config::ProvidersConfig::decl(),
quality::config::SonarConfig::decl(),
quality::metrics::MetricKey::decl(),
server::routes::quality_policy::QualityPolicyResponse::decl(),
server::routes::quality_policy::UpdateQualityPolicyRequest::decl(),
server::routes::quality_policy::MetricCatalogEntry::decl(),
server::routes::planning_drafts::ConfirmGatesRequest::decl(),
```
- `MetricKey` derives `TS` → emits a TS string-literal union (because of `#[serde(rename=...)]` on each variant) → the picker gets compile-time-checked metric keys. This is exactly how we surface the **closed-enum constraint** to the FE.
- `quality` crate must be a dependency of the `generate_types` bin's crate graph — `server` already (will) depend on `quality` transitively via routes; ensure `quality` is in `crates/server/Cargo.toml` (add if absent). `services` already depends on `quality`.
- Run `npm run generate-types`; CI `--check` enforces commit.

---

## 4. Frontend changes

### 4.1 Shared editor component (serves BOTH G2 popup and G3 page)

**New `frontend/src/components/quality/QualityPolicyEditor.tsx`** — the single source of truth. Controlled component:
```ts
interface QualityPolicyEditorProps {
  value: QualityGateConfig;              // from shared/types (ts-rs)
  onChange: (next: QualityGateConfig) => void;
  metrics: MetricCatalogEntry[];         // from GET /quality/metrics
  readOnly?: boolean;
}
```
Renders:
- a `mode` selector (off/shadow/warn/enforce — values come from the `QualityGateMode` TS union);
- three collapsible gate sections (terminal/branch/repo), each editing `GateDefinition.conditions[]`:
  - **MetricKey is a PICKER** (`<select>`) populated from `metrics` — the union type makes invalid keys impossible; surface a note "metrics are fixed; choose from the list";
  - operator picker limited to **GT / LT** only;
  - threshold free-text (validated numeric);
  - add/edit/delete condition rows;
- provider toggles (13 booleans from `ProvidersConfig`);
- sonar host/projectKey/token (token write-only; shows "unchanged" placeholder on read).

Reuse styling from existing `components/quality/QualityReportPanel`/`QualityIssueList`.

### 4.2 G2 — confirmation popup between confirm and materialize

**New `frontend/src/components/quality/QualityGateConfirmDialog.tsx`** — a modal wrapping `QualityPolicyEditor`:
- opens after `confirm` succeeds (draft.status === 'confirmed'), before `materialize` is allowed;
- on mount: `GET /projects/:projectId/quality-policy` to seed the editor (project override or bundled);
- primary action "Confirm gates & start": `POST /planning-drafts/:id/confirm-gates {config}` → on success closes dialog and enables/triggers materialize;
- cannot be dismissed into materialize without confirming (the materialize button stays gated on `draft.gatesConfirmedAt`).

**`CreateChatBoxContainer.tsx`** (320-349): change `handleConfirm` to open the dialog after confirm; change the materialize button enablement to require `planningDraft?.gatesConfirmedAt`. `handleMaterialize` stays, now reached only post-gate-confirm. Apply the same to `PlanningChatContainer.tsx`/`PlanningChat.tsx`.

### 4.3 G3 — standalone settings page

**New `frontend/src/pages/ui-new/settings/QualitySettingsNew.tsx`**:
- project selector (or use current project context, matching `ProjectSettingsNew.tsx`);
- loads `GET /quality/metrics` + `GET /projects/:projectId/quality-policy`;
- renders `QualityPolicyEditor` (same component);
- "Save" → `PUT /projects/:projectId/quality-policy {config}`;
- "Reset to defaults" → `GET /quality/policy/defaults` → load into editor (unsaved until Save).
- **Import directly** — do NOT go through `pages/settings/index.ts` (barrel deleted in P4-FINAL). Register the route wherever the other `*New` settings pages are wired (sidebar/router), importing `QualitySettingsNew` by its direct path.

### 4.4 api.ts changes (`frontend/src/lib/api.ts`)

Keep DISJOINT from CL-IDE (which removes `openEditor`). Additions:
- `qualityPolicyApi`:
  - `getMetrics(): Promise<MetricCatalogEntry[]>` → `GET /api/quality/metrics`
  - `getDefaults(): Promise<QualityGateConfig>` → `GET /api/quality/policy/defaults`
  - `getProjectPolicy(projectId): Promise<QualityPolicyResponse>` → `GET /api/projects/{projectId}/quality-policy`
  - `putProjectPolicy(projectId, config): Promise<QualityPolicyResponse>` → `PUT …`
- `planningDraftsApi.confirmGates(draftId, config): Promise<PlanningDraftResponse>` → `POST /api/planning-drafts/{draftId}/confirm-gates`
- `PlanningDraftResponse` interface (~505): add `gatesConfirmedAt: string | null`.
- **FE-01 fix** (line 607): change `formData.append('audit_doc', file)` → `formData.append('file', file)` to match backend field name (`planning_drafts.rs:1072`).

Import `QualityGateConfig`, `QualityGateMode`, `GateDefinition`, `ConditionConfig`, `ProvidersConfig`, `SonarConfig`, `MetricKey`, `MetricCatalogEntry`, `QualityPolicyResponse` from `shared/types`.

### 4.5 hooks (`frontend/src/hooks/`)

- extend `usePlanningDraft.ts`: add `useConfirmGates()` mutation (invalidates the draft query so `gatesConfirmedAt` refreshes and unblocks materialize).
- new `useQualityPolicy.ts`: `useQualityMetrics()`, `useProjectQualityPolicy(projectId)`, `useUpdateProjectQualityPolicy()`, `useQualityPolicyDefaults()`.

---

## 5. The materialize hard-gate (exact mechanism)

1. New column `planning_draft.gates_confirmed_at DATETIME` (NULL by default).
2. `materialize_draft` (planning_drafts.rs ~915), immediately after the `status != "confirmed"` guard, returns `ApiError::BadRequest` (HTTP 400) when `draft.gates_confirmed_at.is_none()`.
3. Only `POST /planning-drafts/:id/confirm-gates` sets it (via `PlanningDraft::set_gates_confirmed`), and only after a successful `QualityGateConfig::validate()` + `project_quality_policy` upsert.
4. Result: code-writing (`auto_prepare_and_start` spawn at planning_drafts.rs:1016) is unreachable until the user approves the System-A gates. Backend-enforced; FE popup is convenience, not the gate.

---

## 6. Rule delivery (Mechanism C) — why, and how the 3 sites get it

**Why C:** the engine has no DB pool (`from_project` is filesystem-only, engine.rs:79) and we explicitly do not want the engine to write files. The orchestrator already owns `self.db.pool` + `self.config` and can resolve the workflow's `project_id`. Resolving DB→file→bundled once at that layer and injecting via `from_config` is the smallest quality-crate change (one new constructor + extracted `build_providers`), keeps `load_from_project` semantics intact for every other caller/test, and makes the DB override authoritative without touching the loader's file-precedence logic.

**Three sites:**
- Branch + Repo (`run_final_quality_gate`, has `&self`): `self.resolve_quality_config(project_root).await` → `from_config`.
- Terminal (inside `tokio::spawn(async move)` with no `&self`): resolve BEFORE the spawn and move the `QualityGateConfig` into the task; call `from_config` inside.

This is a **hybrid of C with a DB-priority resolver** — DB lookup is layered ABOVE the existing `load_from_project` rather than inside it, so the closed-enum/file fallback chain is preserved verbatim.

---

## 7. MetricKey closed-enum picker constraint (must surface in UI)

`MetricKey` is a closed 44-variant Rust enum (metrics.rs:11-169) and `Operator` is closed to GT/LT (condition.rs:16-23). The editor **cannot invent metrics at runtime**: it is a PICKER over `GET /quality/metrics` (driven by `MetricKey::ALL`, excluding the internal `QualityGateEmptyScan` sentinel). ts-rs emits `MetricKey` as a TS string-literal union, so the FE is compile-time-constrained too. Adding a new metric requires a Rust code change + regenerated types — document this in the settings UI ("metric set is fixed by the engine").

---

## 8. Coordination with P4-FINAL deletion

| File | P4-FINAL (deletion) | This work (G2/G3) | Resolution |
|---|---|---|---|
| `generate_types.rs` | removes lines 128-129 (`OpenEditorRequest/Response`) | ADDS quality-config + policy decls near 228-234 | disjoint line regions; no conflict |
| `api.ts` | removes `openEditor` | adds `qualityPolicyApi` + `confirmGates` + FE-01 fix | disjoint symbols |
| `planning_drafts.rs` | RB-49 feishu-push refactor | adds confirm-gates route + materialize guard + DTO field | different functions; coordinate merge order |
| `pages/settings/index.ts` barrel | DELETED | new `QualitySettingsNew` imports DIRECTLY (not via barrel) | comply by direct import |
| quality routes `mod.rs` | (touched) | adds nested quality-policy routes | coordinate the nest list edit |

---

## 9. Risks

1. **Terminal-gate spawn capture (Mechanism C):** resolved config must be computed before `tokio::spawn` (agent.rs:2603) and moved in; getting this wrong reintroduces a `&self`-across-await borrow error. Verified the spawn does not capture `&self` (2606 comment).
2. **`project_id` type:** `planning_draft.project_id`/`workflow.project_id` is `Uuid`; `project_quality_policy.project_id` is TEXT — bind consistently with `.to_string()`.
3. **Sonar token leakage:** `SonarConfig.token` must be stripped on GET responses and preserved-on-omit on PUT, else the editor round-trip could blank or expose a secret.
4. **ts-rs dep wiring:** adding `ts-rs` to `crates/quality` + ensuring `server` depends on `quality` for `generate_types`; CI `--check` will fail loudly until types are regenerated and committed.
5. **`MetricKey` union breaking change:** deriving `TS` changes nothing at runtime but the generated union must match `as_str()` renames exactly; `QualityGateEmptyScan` should be excluded from the picker but WILL appear in the union unless `#[ts(skip)]` is applied — apply `#[ts(skip)]` to the sentinel variant or keep it in the union but filter in `MetricKey::ALL`.
6. **Existing-draft backfill:** existing `confirmed` drafts have NULL `gates_confirmed_at` and will be blocked from materialize until they call confirm-gates — acceptable (intended), but note for any in-flight drafts.
7. **Two FE planning entry points** (`CreateChatBoxContainer` + `PlanningChatContainer`/`PlanningChat`) both need the popup; missing one leaves a bypass of the convenience layer (backend still blocks).
