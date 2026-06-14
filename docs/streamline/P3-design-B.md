# P3 Design B — G2 (mandatory gate-confirm before materialize) + G3 (standalone quality-gate settings)

Branch: `refactor/streamline-and-quality-gate-rules`
Author angle: **Mechanism B** (write resolved per-project `QualityGateConfig` to a project-managed `quality-gate.yaml` at gate-confirm; zero engine change) — but with a DB row as the source of truth, and the YAML treated as a derived cache. See "Rule delivery mechanism" for the final verdict (hybrid B+A).

All file:line anchors below were re-verified on this branch.

---

## 0. Verified current-state anchors

| Claim | Verified location |
|---|---|
| `QualityGateConfig` shape (mode/terminal/branch/repo/providers/sonar), all serde round-trippable, NONE derive `TS` | `crates/quality/src/config.rs:20-36`, `55-73`, `84-114` |
| `ConditionConfig { metric: MetricKey, operator: String, threshold: String }` + `to_condition()` validates operator via `Operator::from_db_value` | `crates/quality/src/config.rs:65-81` |
| `MetricKey` is a CLOSED enum with `#[serde(rename=…)]` snake_case variants (cargo_check_errors, clippy_warnings, tsc_errors, …) | `crates/quality/src/metrics.rs:11-120` |
| `load_from_project` order: repo `quality/quality-gate.yaml\|.yml\|.quality-gate.yaml` → `BUNDLED_CENTRAL_POLICY` → `default_config()`. **NO DB lookup.** | `crates/quality/src/config.rs:181-217` |
| `QualityEngine::from_project(project_root: &Path)` is the ONLY constructor; `new(config, providers)` exists but is not used by orchestrator | `crates/quality/src/engine.rs:74`, `79-80` |
| Terminal gate calls `from_project(wd)` where `wd` = worktree dir OR project repo fallback | `agent.rs:2815`, working-dir resolution `agent.rs:2686-2746` |
| Branch + Repo gate call `from_project(project_root)` inside `run_gate` | `agent.rs:7880` |
| `resolve_project_working_dir` = `project.default_agent_working_dir` else first project repo path | `agent.rs:4964-4985` |
| `confirm_draft` generates System-B AuditPlan, `set_confirmed`, status `spec_ready\|gathering → confirmed` | `planning_drafts.rs:286-389` |
| `materialize_draft` guard = ONLY `status != "confirmed"` (line 915); creates Workflow + `tokio::spawn auto_prepare_and_start` | `planning_drafts.rs:904-1034`, guard `915-920`, spawn `1016-1027` |
| `DraftResponse` DTO | `planning_drafts.rs:53-99` |
| Route registration | `planning_drafts.rs:123-141`; `mod.rs:166,170-175` |
| `planning_draft` migration | `20260307200000_add_planning_draft.sql`; audit cols `20260508000000_add_audit_plan.sql` |
| `PlanningDraft` model + `set_confirmed`/`set_materialized` | `crates/db/src/models/planning_draft.rs:223-258` |
| ts-rs pipeline: `decl()` list in `generate_types.rs:13-242`; `--check` mode `362-417`; quality crate has NO ts-rs dep | `generate_types.rs` |
| `generate_types.rs:128-129` = `OpenEditorRequest/Response` decls (CL-IDE removes; keep our adds disjoint) | `generate_types.rs:128-129` |
| FE confirm/materialize: no dialog between (CreateChatBoxContainer `handleConfirm`/`handleMaterialize` 320-349; PlanningChatContainer 125-143) | both containers |
| `planningDraftsApi` + `PlanningDraftResponse` | `api.ts:505-524`, `540-625` |
| **FE-01 bug**: `uploadAuditDoc` sends `formData.append('audit_doc', file)` (api.ts:607); backend rejects anything `!= "file"` (planning_drafts.rs:1072) → upload 100% broken | confirmed |
| Settings routes in `App.tsx:160-177`; nav list in `SettingsLayoutContainer.tsx:24-74`; `SettingsNavItem` in `SettingsLayout.tsx:6-8` | confirmed |
| `pages/settings/index.ts` barrel re-exports `ui-new/settings` (slated for P4-FINAL deletion) | `pages/settings/index.ts:1-11` |

---

## 1. Rule delivery mechanism — FINAL DECISION: **Hybrid B (write) + A (load priority), DB = source of truth**

The brief leans toward Mechanism B (serialize to the managed `quality-gate.yaml` the loader already reads → zero engine change). I adopt **B as the primary write path, but back it with a DB row and a thin priority-0 load branch (A) as a fallback**, because pure B has two fatal staleness/pollution failure modes that A closes for almost no cost.

### Why not pure B
The `from_project` working dir is **not stable** and can be a **git worktree, not the project repo** (`agent.rs:2686-2713`): the terminal gate runs in `WorktreeManager::get_worktree_base_dir().join(task.branch)`. A `quality-gate.yaml` written into the project repo root at confirm time is:

1. **Worktree-invisible at the terminal gate** unless the file was committed before the worktree branch was created. The worktree is created from `target_branch` at materialize; if we write the YAML *after* `git worktree add`, the terminal gate worktree won't see it. If we write it *before* and the worktree is created from a branch that has the commit, it's visible — but now we've **polluted git history** of the user's repo with a SoloDawn config commit (unacceptable for an output repo).
2. **Multi-repo**: a project can have N repos (`ProjectRepo::find_repos_for_project`). `resolve_project_working_dir` picks the *first*. Writing one YAML into one repo root silently mis-covers the others.

So a repo-root committed YAML is wrong. An **un-committed** YAML in the repo root is *also* wrong: `.gitignore` noise, and worktrees created via `git worktree add` do NOT inherit un-tracked working-tree files from the main checkout — they start clean from the branch tip, so the un-tracked YAML is invisible in the worktree anyway.

### The hybrid that actually works
- **Source of truth = DB.** New table `project_quality_policy` (one row per project) stores the full `QualityGateConfig` as JSON. The settings page (G3) and the confirm-popup (G2) both read/write this row. This is what makes G3 "per-project DIY override with built-in defaults" coherent and what survives worktree churn.
- **Write path (B), but to a SoloDawn-managed cache dir, NOT the repo.** At gate-confirm AND at every settings save, serialize the resolved config to `utils::cache_dir().join("quality_policies").join("{project_id}.yaml")`. Zero git pollution, no multi-repo ambiguity, no worktree visibility problem.
- **Load path (A), priority 0.** Add a `load_from_project_with_policy(project_root, policy_yaml: Option<&str>)` that prefers an explicitly-passed resolved YAML, falling through to the existing repo→bundled→default chain. The orchestrator passes the DB-resolved YAML (read from the cache file, or directly from DB) at the three call sites.

Because the orchestrator (`agent.rs`) holds `self.db.pool` but the `quality` crate has no pool, we resolve the policy **in the orchestrator** (which knows `workflow.project_id`) and hand the engine a ready string. The quality crate stays pool-free. This is mechanism A's "from_config injection" but fed by mechanism B's serialized artifact — the two are the same bytes, so we get B's "engine reads a YAML it already understands" with A's correctness.

**Net engine change is minimal and additive** (one new constructor + one new `config` fn), preserving the existing `from_project` for all non-orchestrated callers (e.g. `container.rs:246`).

### Staleness
DB row is read fresh at each of the three gate sites per run (the orchestrator already does per-run DB reads). The cache YAML is rewritten on every settings save and at confirm, and is only a fallback if the DB read fails — staleness window is one save, self-healing on next save. We also stamp `project_quality_policy.updated_at`; the orchestrator logs it for debuggability.

---

## 2. DB schema

New migration `crates/db/migrations/20260615000000_add_quality_policy_and_gates_confirm.sql`:

```sql
-- G2/G3: per-project quality-gate policy (System A) + gate-confirm checkpoint.

-- One DIY policy row per project. NULL/absent row => use built-in bundled defaults.
CREATE TABLE IF NOT EXISTS project_quality_policy (
    project_id   BLOB PRIMARY KEY REFERENCES project(id) ON DELETE CASCADE,
    -- Full QualityGateConfig serialized as JSON (serde round-trips this struct).
    config_json  TEXT NOT NULL,
    -- Cache of the YAML form written to disk; advisory, may be regenerated.
    config_yaml  TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- G2: hard checkpoint. materialize is blocked until this is set.
ALTER TABLE planning_draft ADD COLUMN gates_confirmed_at TEXT;
```

No down migration is required to match repo convention for the audit-plan migration (`20260508000000_add_audit_plan.sql` ships up-only); follow the same convention. (The feishu migration has up/down — if CI requires symmetry, add a `.down.sql` dropping the table + column.)

---

## 3. Backend changes

### 3.1 quality crate — additive engine/config API (`crates/quality`)

`Cargo.toml`: add `ts-rs = { workspace = true }` (match how other crates pull it; verify the workspace key name before editing).

`config.rs` — add `#[derive(TS)]` + `#[ts(export)]`-free decls (we register manually in generate_types) to: `QualityGateConfig`, `QualityGateMode`, `GateDefinition`, `ConditionConfig`, `ProvidersConfig`, `SonarConfig`. Add `#[derive(TS)]` to `MetricKey` in `metrics.rs` so the FE picker can enumerate the closed set. Keep existing serde renames; `ts-rs` honors `#[serde(rename_all)]`/`#[serde(rename)]`, so the generated TS union for `MetricKey` will be the snake_case string literals (`"cargo_check_errors" | "clippy_warnings" | …`) — exactly what the picker needs.

Add to `QualityGateConfig`:
```rust
/// Like load_from_project, but a caller-supplied resolved policy (DB-backed)
/// wins over repo file / bundled. Used by the orchestrator which holds the pool.
pub fn load_resolved(project_root: &Path, policy_yaml: Option<&str>) -> anyhow::Result<Self> {
    if let Some(yaml) = policy_yaml {
        match Self::from_yaml(yaml) {
            Ok(cfg) => return Ok(cfg),
            Err(e) => tracing::warn!(error=%e, "DB quality policy failed to parse; falling back to project/bundled"),
        }
    }
    Self::load_from_project(project_root)
}

/// Validate every condition operator + that thresholds parse where numeric.
/// Returns the list of human-readable errors (empty = valid). Used by the API
/// before persisting a DIY policy so the editor can show inline errors.
pub fn validate(&self) -> Vec<String> { /* iterate gates → to_condition(); collect Err strings */ }
```

`engine.rs` — add a sibling constructor (keep `from_project` untouched):
```rust
/// Build an engine from an already-resolved config (DB-backed policy).
pub fn from_config(config: QualityGateConfig, _project_root: &Path) -> Self {
    // same provider-assembly block as from_project, factored into a private
    // `build_providers(&config)` helper that both constructors call.
}
```
Refactor the provider-assembly block (`engine.rs:83-…`) into `fn build_providers(config: &QualityGateConfig) -> Vec<Arc<dyn QualityProvider>>` and call it from both `from_project` and `from_config`. Smallest-effective change: `from_project` becomes `Ok(Self::new(QualityGateConfig::load_from_project(root)?, Self::build_providers(&cfg)))`.

### 3.2 db model (`crates/db/src/models/`)

New `project_quality_policy.rs`:
```rust
pub struct ProjectQualityPolicy { project_id: Uuid, config_json: String, config_yaml: Option<String>, created_at, updated_at }
impl ProjectQualityPolicy {
    async fn find_by_project(pool, project_id) -> sqlx::Result<Option<Self>>;
    async fn upsert(pool, project_id, config_json: &str, config_yaml: Option<&str>) -> sqlx::Result<()>; // INSERT … ON CONFLICT(project_id) DO UPDATE
    async fn delete(pool, project_id) -> sqlx::Result<()>; // reset to built-in default
}
```
Register module in `crates/db/src/models/mod.rs`.

Add to `planning_draft.rs`:
- field `pub gates_confirmed_at: Option<DateTime<Utc>>` (after `confirmed_at`); add to `FromRow`, `new()` (=None), `insert` column list + bind (becomes `?25`).
- `async fn set_gates_confirmed(pool, id) -> sqlx::Result<()>` (`UPDATE … SET gates_confirmed_at = datetime('now'), updated_at = datetime('now')`).

### 3.3 New REST routes

**Quality policy (G3, project-scoped)** — add to `crates/server/src/routes/quality.rs`, registered as a new nest `mod.rs` under `/projects` is awkward (projects router is built differently); instead add a dedicated nest:

`mod.rs`: `.nest("/quality-policy", quality::quality_policy_routes())`

```
GET    /api/quality-policy/defaults                 -> QualityPolicyDto   (BUNDLED_CENTRAL_POLICY parsed; no project needed)
GET    /api/quality-policy/{project_id}             -> QualityPolicyResolvedDto { policy, isCustom, source: "custom"|"bundled" }
PUT    /api/quality-policy/{project_id}             body QualityPolicyDto -> 200 | 422 {errors:[…]}  (validates via QualityGateConfig::validate, upserts row, writes cache yaml)
DELETE /api/quality-policy/{project_id}             -> resets to built-in (delete row + cache file) -> returns defaults
GET    /api/quality-policy/metrics                  -> Vec<MetricInfoDto> { key, label, group }  (drives the closed-enum picker)
```

`QualityPolicyDto` wraps `quality::config::QualityGateConfig` directly (it's `Serialize/Deserialize`; add `TS`). The PUT handler:
1. Deserialize body → `QualityGateConfig`.
2. `cfg.validate()`; if non-empty → `ApiError::UnprocessableEntity` (or `BadRequest`) with the errors.
3. `serde_json::to_string(&cfg)` + `serde_yaml::to_string(&cfg)`.
4. `ProjectQualityPolicy::upsert(pool, project_id, json, Some(yaml))`.
5. Write cache file `utils::cache_dir().join("quality_policies").join(format!("{project_id}.yaml"))`.

`MetricInfoDto` list is generated from a static table in `metrics.rs` (new `pub const ALL_METRICS: &[(MetricKey, &str, &str)]` or a `MetricKey::all()` + `label()`/`group()` impls). Surfaces the **closed-enum constraint**: the FE can only pick from this list; it can never POST a metric string not in the enum (serde would 400).

**Gate-confirm (G2)** — add to `planning_drafts.rs` route table:
```
PUT /api/planning-drafts/{draft_id}/quality-policy   body QualityPolicyDto -> DraftResponse (per-project DIY edit from the popup; same upsert as G3, keyed by draft.project_id)
POST /api/planning-drafts/{draft_id}/confirm-gates    -> DraftResponse (sets planning_draft.gates_confirmed_at; the user's explicit approval)
```
`confirm-gates` is the G2 approval action. It does NOT mutate the policy (the popup's optional edits go through the PUT above first); it only stamps `gates_confirmed_at`. Guard: draft must be `status == "confirmed"` (spec already confirmed) and `gates_confirmed_at IS NULL`.

### 3.4 materialize hard-gate (G2 — the load-bearing block)

In `materialize_draft` (`planning_drafts.rs:904`), **after** the existing `status != "confirmed"` check (line 915), add:

```rust
if draft.gates_confirmed_at.is_none() {
    return Err(ApiError::BadRequest(
        "Quality gates must be reviewed and approved before materialization. \
         Call POST /planning-drafts/{id}/confirm-gates first.".to_string()
    ));
}
```

This is the backend hard block: even a direct API call to `/materialize` returns 400 until `confirm-gates` has stamped the column. The FE cannot bypass it. (Use `BadRequest` to match the sibling guard's existing 400 convention; switch to a 409/412 only if the API style guide prefers it.)

### 3.5 FE-01 audit-doc bug

Two valid fixes; pick the FE side (smaller, matches the `images` upload convention which uses a dedicated field name):
- **Fix in `api.ts:607`**: `formData.append('audit_doc', file)` → `formData.append('file', file)`.

(Backend already accepts only `"file"` at `planning_drafts.rs:1072`; do NOT change the backend or you break any already-correct caller.) Re-verify no other caller posts `audit_doc`.

### 3.6 ts-rs registration (`generate_types.rs`)

Append (keep DISJOINT from the CL-IDE removal at lines 128-129 — our adds go at the end of the `decls` vec, after line 242):
```rust
quality::config::QualityGateConfig::decl(),
quality::config::QualityGateMode::decl(),
quality::config::GateDefinition::decl(),
quality::config::ConditionConfig::decl(),
quality::config::ProvidersConfig::decl(),
quality::config::SonarConfig::decl(),
quality::metrics::MetricKey::decl(),
server::routes::quality::QualityPolicyResolvedDto::decl(),
server::routes::quality::MetricInfoDto::decl(),
```
Add `quality` to the bin's dependency closure (server already depends on quality transitively via services; confirm `quality` is a direct dep of the `server` crate or add it to `crates/server/Cargo.toml`). Then `npm run generate-types`; CI `--check` (generate_types.rs:382) will enforce `shared/types.ts` is committed.

---

## 4. Frontend changes

### 4.1 Shared editor component (serves BOTH G2 popup and G3 page)

Create `frontend/src/components/quality/QualityPolicyEditor.tsx` — a **stateless/controlled** view (per frontend/CLAUDE.md architecture rules: views receive data via props):

```tsx
interface QualityPolicyEditorProps {
  policy: QualityGateConfig;          // from shared/types (ts-rs generated)
  metrics: MetricInfoDto[];           // closed-enum picker source
  onChange: (next: QualityGateConfig) => void;
  readOnly?: boolean;
  errors?: string[];                  // from PUT 422
}
```
It renders: mode `<select>` (off/shadow/warn/enforce), three collapsible gate sections (terminal/branch/repo), each with a condition list where every row is `[ MetricKey picker ] [ GT|LT select ] [ threshold input ] [ delete ]` + an "add condition" button, and a providers section (13 toggles from `ProvidersConfig`) + sonar host/key fields. The metric picker is a `<select>`/combobox over `metrics` — **it cannot invent metrics** (the closed-enum constraint), surfaced in UI as a fixed dropdown grouped by `group`. Reuse `components/quality/` styling tokens (QualityReportPanel/QualityIssueList) and the new-design CSS vars (`bg-secondary`, `text-normal`, `ring-brand`).

State lives in the two parents (popup container, settings container); the editor is pure. This is the single source of UI truth shared by G2 and G3.

New API client + hooks:
- `api.ts`: `qualityPolicyApi = { getDefaults, get(projectId), put(projectId, dto), reset(projectId), getMetrics() }`. Add `confirmGates(draftId)` + `putDraftQualityPolicy(draftId, dto)` to `planningDraftsApi`. Import `QualityGateConfig`, `MetricKey`, `MetricInfoDto` from `shared/types`.
- `frontend/src/hooks/useQualityPolicy.ts`: `useQualityPolicy(projectId)`, `useQualityDefaults()`, `useQualityMetrics()`, `useSaveQualityPolicy()`, `useResetQualityPolicy()`, `useConfirmGates()`.

### 4.2 G2 — mandatory confirm popup between confirm and materialize

The current `Confirmed` state button calls `handleMaterialize` directly (CreateChatBoxContainer `PlanningStatusBar` line 239-248; PlanningChatContainer 137-143). Change:

- The `Materialize` button no longer calls materialize directly. When `isConfirmed && gatesConfirmedAt == null`, it opens a **GateConfirmDialog** (new `frontend/src/components/quality/GateConfirmDialog.tsx`) that:
  1. Loads the resolved policy for `draft.projectId` (`useQualityPolicy`).
  2. Renders `QualityPolicyEditor` (editable; optional DIY tweaks → `putDraftQualityPolicy`).
  3. Has an explicit "Approve quality gates" primary action → `useConfirmGates(draftId)` → on success closes dialog and THEN runs `handleMaterialize`.
- After `confirm-gates` succeeds (`gates_confirmed_at` set), the existing `handleMaterialize` runs and the backend block passes. If the user closes the dialog without approving, materialize never fires and (defense in depth) the backend 400s.
- Add `auditDocPath`/`gatesConfirmedAt` to `PlanningDraftResponse` (api.ts:505) — `gatesConfirmedAt: string | null` — and to `DraftResponse` (planning_drafts.rs:53 + `From` impl) reading the new column.

`gates_confirmed_at` must be plumbed: `DraftResponse` field `gates_confirmed_at: Option<String>` (rfc3339), `From<PlanningDraft>` maps `d.gates_confirmed_at.map(|t| t.to_rfc3339())`.

### 4.3 G3 — standalone settings page

- New `frontend/src/pages/ui-new/settings/QualitySettingsNew.tsx`: project selector (reuse existing project context) → loads `useQualityPolicy(projectId)` → renders `QualityPolicyEditor` with Save (`useSaveQualityPolicy`) + "Reset to built-in defaults" (`useResetQualityPolicy`). Shows `source: bundled|custom` badge.
- Register WITHOUT the doomed barrel: import directly in `App.tsx` (`import { QualitySettingsNew } from '@/pages/ui-new/settings/QualitySettingsNew'`), add `<Route path="quality" element={<QualitySettingsNew />} />` after `App.tsx:176`. Also export from `pages/ui-new/settings/index.ts` for symmetry, but the App import must be direct so it survives the `pages/settings/index.ts` deletion (P4-FINAL).
- Add nav item in `SettingsLayoutContainer.tsx:24-74`: `{ path: 'quality', label: t('settings:newDesign.nav.quality', 'Quality Gates'), icon: ShieldCheckIcon }` (import from `@phosphor-icons/react`). Add the i18n key.

---

## 5. API contracts (request/response summary)

```
GET    /api/quality-policy/defaults              -> ApiResponse<QualityGateConfig>
GET    /api/quality-policy/metrics               -> ApiResponse<MetricInfoDto[]>
GET    /api/quality-policy/{projectId}           -> ApiResponse<{ policy: QualityGateConfig, isCustom: bool, source: "custom"|"bundled" }>
PUT    /api/quality-policy/{projectId}           body QualityGateConfig -> ApiResponse<…> | 422 ApiResponse<{errors:string[]}>
DELETE /api/quality-policy/{projectId}           -> ApiResponse<QualityGateConfig> (defaults)
PUT    /api/planning-drafts/{draftId}/quality-policy   body QualityGateConfig -> ApiResponse<DraftResponse>
POST   /api/planning-drafts/{draftId}/confirm-gates    -> ApiResponse<DraftResponse>
POST   /api/planning-drafts/{draftId}/materialize      -> 400 if gates_confirmed_at IS NULL  (NEW guard)
POST   /api/planning-drafts/{draftId}/audit-doc        multipart field "file"  (FE now sends "file" — FE-01 fix)
```

---

## 6. Orchestrator wiring (mechanism A read side)

At the three gate sites, replace `QualityEngine::from_project(wd)` with a DB-resolved variant. The orchestrator holds `self.db.pool` and `workflow.project_id`:

- New helper on the agent: `async fn resolve_quality_policy_yaml(&self) -> Option<String>` → `ProjectQualityPolicy::find_by_project(&self.db.pool, project_id)` → `.config_yaml` (or re-serialize `.config_json`). Falls back to reading the cache file if DB row missing; returns `None` if neither → engine then uses `from_project`'s existing repo→bundled chain.
- Terminal gate (`agent.rs:2815`): `QualityEngine::from_config(QualityGateConfig::load_resolved(wd, policy.as_deref())?, wd)`. Note `wd` here is inside a free function block (around 2686) that has `db` available (`db.pool` used at 2689) — resolve `project_id` from the already-fetched workflow (`agent.rs:2717-2720`).
- Branch/Repo gate (`run_gate`, `agent.rs:7880`): same, using `self.db.pool` + `self.load_workflow().await?.project_id`.

Because `load_resolved` falls through to `load_from_project`, **all non-orchestrated callers (`container.rs:246`) keep working unchanged.**

---

## 7. Files to create / edit

**Create**
- `crates/db/migrations/20260615000000_add_quality_policy_and_gates_confirm.sql`
- `crates/db/src/models/project_quality_policy.rs`
- `frontend/src/components/quality/QualityPolicyEditor.tsx`
- `frontend/src/components/quality/GateConfirmDialog.tsx`
- `frontend/src/pages/ui-new/settings/QualitySettingsNew.tsx`
- `frontend/src/hooks/useQualityPolicy.ts`

**Edit (backend)**
- `crates/quality/Cargo.toml` (add ts-rs)
- `crates/quality/src/config.rs` (derive TS; `load_resolved`; `validate`)
- `crates/quality/src/metrics.rs` (derive TS on MetricKey; `ALL_METRICS`/`label`/`group`)
- `crates/quality/src/engine.rs` (`from_config` + `build_providers` refactor)
- `crates/db/src/models/mod.rs` (register module)
- `crates/db/src/models/planning_draft.rs` (gates_confirmed_at field + insert + `set_gates_confirmed`)
- `crates/server/src/routes/quality.rs` (`quality_policy_routes`, DTOs, handlers)
- `crates/server/src/routes/planning_drafts.rs` (materialize guard line ~920; `confirm-gates` + draft `quality-policy` routes; DraftResponse field)
- `crates/server/src/routes/mod.rs` (nest `/quality-policy`)
- `crates/server/Cargo.toml` (ensure direct `quality` dep for generate_types)
- `crates/server/src/bin/generate_types.rs` (append decls AFTER line 242 — disjoint from 128-129)

**Edit (frontend)**
- `frontend/src/lib/api.ts` (FE-01 fix line 607; `qualityPolicyApi`; `confirmGates`+`putDraftQualityPolicy`; `gatesConfirmedAt` on PlanningDraftResponse)
- `frontend/src/hooks/usePlanningDraft.ts` (`useConfirmGates`)
- `frontend/src/components/ui-new/containers/CreateChatBoxContainer.tsx` (materialize → open GateConfirmDialog)
- `frontend/src/components/ui-new/containers/PlanningChatContainer.tsx` (same)
- `frontend/src/App.tsx` (direct import + `<Route path="quality">`)
- `frontend/src/components/ui-new/containers/SettingsLayoutContainer.tsx` (nav item)
- `frontend/src/pages/ui-new/settings/index.ts` (export QualitySettingsNew)
- i18n: add `settings:newDesign.nav.quality` + planning confirm-gates strings
- `shared/types.ts` (regenerated, committed)

---

## 8. Coordination with P4-FINAL deletion
- `planning_drafts.rs` (RB-49 feishu refactor): our edits are in `confirm_draft`/`materialize_draft`/route table/DraftResponse — keep changes localized; rebase-merge carefully.
- `generate_types.rs`: CL-IDE removes lines 128-129 (OpenEditor); our adds are appended after line 242 → disjoint, no conflict.
- `api.ts`: CL-IDE removes `openEditor`; our adds are new exports (`qualityPolicyApi`) + the FE-01 one-liner + new `planningDraftsApi` methods → disjoint.
- `pages/settings/index.ts` barrel DELETED → QualitySettings is imported directly in App.tsx; never via that barrel.
- `quality` routes `mod.rs`: we add one `.nest("/quality-policy", …)` line — coordinate ordering with any other mod.rs edits.

---

## 9. Risks / open items
- **ts-rs on quality crate**: first time the crate gets ts-rs; verify workspace dep key and that `MetricKey`'s serde renames produce the expected TS string union. Run `npm run generate-types` and inspect before committing.
- **Worktree visibility (mechanism B pure)**: explicitly avoided by using DB + cache dir, not repo YAML. If a future reviewer insists on a repo-committed YAML, re-open this decision — it reintroduces git pollution + worktree staleness.
- **Multi-repo**: policy is per-project, not per-repo; one policy covers all repos of a project (matches current `from_project` which only ever sees one working dir). Acceptable; document it.
- **422 vs 400**: confirm the codebase's `ApiError` has an UnprocessableEntity variant; if not, return `BadRequest` with the errors array.
- **`gates_confirmed_at` reset**: if a draft is edited back to `spec_ready` (not currently possible post-confirm) we'd need to clear `gates_confirmed_at`. Current state machine forbids backward transitions, so no reset path needed now.
- **Auto-start race**: materialize spawns `auto_prepare_and_start` immediately (planning_drafts.rs:1016); the gate block is BEFORE workflow creation, so a blocked materialize never spawns. Correct.
- **fast-context unavailable**: the MCP `fast_context_search` returned `resource_exhausted`; all anchors were instead verified via Grep/Read (file:line cited in §0).
