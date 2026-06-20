# PRD — AI-Assisted, Multi-Agent-Validated, User-Editable Quality-Gate Rules

| Field | Value |
|---|---|
| **Title** | AI-Assisted, Multi-Agent-Validated, User-Editable Quality-Gate Rules |
| **Status** | /goal-ready draft |
| **Date** | 2026-06-20 |
| **Revision** | rev2 (2026-06-20) |
| **Model** | opus 4.8 / ultracode |
| **Owner** | Quality platform |
| **Scope** | `crates/quality`, `crates/services`, `crates/server`, `crates/db`, `crates/local-deployment`, `crates/executors`, `frontend` |

> **rev2 changes (summary):** Decisions D1–D9 from the owner are now applied and the corresponding Open Questions are marked RESOLVED (Section 17). The authoring engine is no longer "one metered key" — it is **all globally-configured LLM sources, user-selectable**, behind a single **authoring-model invoker** abstraction with two backends that both reuse existing infrastructure: the metered `LLMClient` (`create_llm_client`) and the **subscription native-OAuth interactive transport** (the genuine `claude` binary, off the credit pool). A new top-level **Reuse Map** section (between §8 and §9) makes the reuse-over-rewrite posture explicit. Rule format is phased reuse-first: **P1 = scoped-regex only (zero new deps)**, **P2 = ast-grep AST format**. The secondary-verification **confirm dialog is mandatory**, never optional.

---

## 1. Executive Summary

SoloDawn's quality gate today can only enforce ~43 hard-coded numeric metrics (`MetricKey`, `crates/quality/src/metrics.rs:14-172`) thresholded by `GT`/`LT` conditions. The rules that *produce* those metrics are Rust structs compiled into the binary (`crates/quality/src/rules/`). A project owner cannot add a project-specific rule ("prohibit `X` in my code") without a Rust PR, and cannot even *understand* what a metric like `test_file_absence` or `runtime_security_smells` means — the editor renders the bare enum token with no description (`QualityGateRulesEditor.tsx:184-188`).

This feature delivers three things, in the order the user asked for them:

1. **Self-documenting tooltips** — a circled "!" on every indicator in the rule editor, showing what it measures *and its current value in the project* (read from the latest persisted run, never recomputed on hover — D7).
2. **Natural-language → rule generation** — the user types "I want to prohibit X", and **any LLM source they have already configured globally** (their own metered API key *or* their official Claude subscription) drafts a declarative rule plus a plain-language description that feeds the same "!" tooltip. The user *picks which configured source* to author with, via the existing model-selection UI (D1).
3. **Multi-agent adversarial validation with an empirical ground-truth test and a context-free round-trip check** — because a generated rule may not meet standards, agents *adversarially* refine it, the candidate is *executed* against must-flag / must-not-flag snippets, **the user confirms in a mandatory dialog** (D2), and then a **fresh, context-starved** agent reverse-engineers the rule body alone and a judge compares its reconstruction to the original request. Match → usable; mismatch → loop, with a hard cap.

The linchpin design choice is a **declarative, sandboxed rule format** (P1: scoped-regex; P2: ast-grep YAML): a rule is *data*, never executable code. This enables the central principle **"generate with AI, enforce without it"** — the LLM appears only during one-time authoring; once human-confirmed, the rule is fixed data that a deterministic, LLM-free engine runs identically every gate run. That is precisely what resolves the paradox of using AI to build the guardrail meant to catch AI mistakes.

**rev2 posture — reuse over rewrite (D9).** Almost everything this feature needs already exists in the codebase and is verified live: the `LLMClient` trait and `create_llm_client`; an already-built, trait-compatible **subscription interactive transport** (the no-`-p` native-OAuth path, `crates/local-deployment/src/container.rs:1468`); the severity-cap machinery (`cap_for_advisory`); the metric/gate machinery; the model-picker hook + dropdown UI; the G2 confirm dialog with its `gates_confirmed_at` hard-block. The net-new surface is a handful of glue modules (`rule_authoring/`, `provider/declarative.rs`), four DB tables, a few enum variants, and frontend wiring. Section "Reuse Map" enumerates this two-column existing-vs-glue split.

---

## 2. Problem & Motivation

**Gate rules are Rust-only and not data-driven.** The rule abstraction (`crates/quality/src/rules/mod.rs:18-51`) is the `Rule` + `CommonRule`/`RustRule`/`TsRule` trait family; each rule is a hand-written struct registered in `all_rust_rules()` (`rules/rust/mod.rs:22`), `all_ts_rules`, or `all_common_rules`. There is **no struct that loads a pattern/severity/metric from data** at runtime. Adding a rule means writing and compiling Rust.

**The Rust↔TS asymmetry.** Rust rules use a real `syn` AST (e.g. `rules/rust/error_handling.rs` implements ".unwrap()/.expect()/panic! outside `#[cfg(test)]`" via `syn::visit::Visit`); TypeScript rules are regex/line-based only (`rules/typescript/console_usage.rs`) — there is no TS AST in the crate. The P1 scoped-regex format works for both languages line-by-line; the P2 ast-grep format closes the structural gap for AST-shaped rules (D5).

**~43 opaque metrics, zero documentation.** `MetricKey` is a closed enum (`metrics.rs:14-172`) whose `as_str()` emits tokens like `clippy_warnings`, `tsc_errors`, `runtime_security_smells`. `selectable_metric_keys()` (`crates/server/src/routes/quality.rs:207-254`) feeds these straight to the editor, which shows the raw token with no explanation (`QualityGateRulesEditor.tsx:174-194`). Users literally cannot tell what to pick — the "I can't understand it / don't know what to add" complaint.

**A real behavioral-bug class motivates the empirical test.** The codebase already shipped over-broad rules — e.g. `test_file_absence` flagging idiomatic inline `#[cfg(test)]` modules. An LLM generating regexes will reproduce exactly this over-reach. A textual round-trip ("does the reconstructed request match?") cannot catch a *behavioral* false-positive; only *executing* the rule against curated snippets can.

**Persistence exists but is opaque.** Per-project policy is the SQLite table `project_quality_policy` (migration `20260614120000`) holding an opaque `config_yaml` of `QualityGateConfig` plus a denormalized `mode` (`crates/db/src/models/project_quality_policy.rs`). Rules are *not* first-class rows — there is nowhere to store a pattern body, its NL provenance, or its validation artifacts.

---

## 3. Goals & Non-Goals

### Goals
- G1. Every selectable indicator in the rule editor has a hover "!" tooltip with name, description, example, and the project's **current value** for that metric (read from the latest persisted `quality_run`, labelled "as of `<timestamp>`" — D7).
- G2. A user can submit a natural-language request and receive a generated, declarative, sandboxed rule + plain-language description, using **any of their already-configured LLM sources, which they select** (their metered API key *or* their official Claude subscription — D1).
- G3. Generated rules are **adversarially** validated, **empirically executed** against generated positive/negative snippets, **human-confirmed in a mandatory dialog** (D2), then **round-trip-verified** by a context-free agent + judge; failures loop with a hard cap (`MAX_AUTHORING_ROUNDS = 4`, D6) and otherwise hand back to the user.
- G4. Confirmed rules are enforced **deterministically and identically** every run by an LLM-free engine provider.
- G5. The whole feature is **additive and non-breaking** (R4 constraint): existing builtin providers and the policy YAML shape are untouched; everything ships dark (default-off) and rolls out per-project shadow→warn→enforce.
- G6. Custom rules can be displayed, edited, added, and deleted per project (the R4 "secondary popup" goal). Project-scoped first (`project_id NOT NULL` in the v1 UI) with the column kept nullable for a later additive global/org step (D4).

### Non-Goals
- N1. No execution of arbitrary user/LLM code. Rules are pure data.
- N2. No LLM in the scan/enforcement path — ever.
- N3. No new global LLM-settings store; reuse the existing per-entity encrypted config **and** the existing model-source enumeration/picker (D1, D9).
- N4. No change to the `quality_run`/`quality_issue` audit tables — authoring-time validation is a *separate* artifact store.
- N5. Org/global (project-less) rules are schema-supported (`project_id` stays nullable) but the v1 UI/feature is project-scoped (`project_id NOT NULL`); global scope is a later additive step (D4).
- N6. No new confirm dialog — the existing G2 `QualityGateConfirmDialog` is extended (preserving the `gates_confirmed_at` hard-block). Human confirmation is **mandatory**, never optional (D2).
- N7. No new LLM transport. The subscription authoring backend reuses the already-built, already-wired no-`-p` interactive native-OAuth transport (D1, D9); **no PTY is introduced** for the authoring turn (see §7.2 correction).

---

## 4. Current State (grounded)

| Concern | Reality today | File |
|---|---|---|
| Engine flow | `QualityProvider::analyze` → `ProviderReport{metrics, issues}` → aggregate → gate conditions read **only** `metrics` (not `issues`) | `provider/mod.rs:131-170`, `engine.rs` |
| Severity cap | `cap_for_advisory` caps `SeverityOrigin::ProjectConfig` analyzers to `Major`; `severity_origin()` is an **exhaustive** match (`_` would not compile) | `rule.rs:124-126`, `rule.rs:205-219` |
| Analyzer sources | Closed enum + `Other(String)`; `Other(_) => Tool` (keeps full severity) | `rule.rs:158-178`, `rule.rs:219` |
| Metric catalog | Closed enum, 3 coupled sites (variant+rename, `as_str`, `display_name`); sentinel `QualityGateEmptyScan` is `#[ts(skip)]` | `metrics.rs:14-273` |
| Provider registration | Single site `build_providers`, each gated on a `ProvidersConfig` bool | `engine.rs:81-141`, `config.rs:91-120` |
| Provider toggles | All default `true` via `default_true()` | `config.rs:122-124` |
| Policy storage | `project_quality_policy` (BLOB `project_id` PK, opaque `config_yaml`, denormalized `mode`); resolver is DB-priority-0, re-read every run | `models/project_quality_policy.rs`, `quality_policy.rs:26` |
| Editor | One shared `QualityGateRulesEditor` consumed by `RulesDialog`, `ConfirmDialog`, `SettingsNew`; `metricOptions` already threaded to all three; uses raw `text-slate-*`/`bg-white` Tailwind (NOT `.new-design`) | `QualityGateRulesEditor.tsx` |
| Metric catalog API | `GET /api/quality/policy/metrics` → `MetricCatalogResponse { metrics, operators }` | `quality.rs:198-203`, `:277` |
| Metered LLM client | `create_llm_client(&OrchestratorConfig)` → `Box<dyn LLMClient>`; one method `chat(Vec<LLMMessage>) -> LLMResponse` | `orchestrator/llm.rs:922`, `:23` |
| **Subscription LLM client (already built)** | `InteractiveClaudeClient` impl `LLMClient` against the genuine `claude` binary (subscription/native-OAuth surface, OFF the `-p`/Agent-SDK pool); `create_interactive_claude_client(model)` returns `None` if no native creds | `llm.rs:628-703`, `:706`, `:868` |
| **Interactive transport (already wired)** | `LocalContainerService::try_spawn_interactive_native_oauth` auto-routes every ClaudeCode run to the no-`-p` transport BEFORE the `-p` path; one-turn piped-null-stdin spawn + transcript tailer (NOT a PTY) | `container.rs:1468`, `:1044`, `:2108` |
| Model enumeration | `GET /api/cli_types/:cli/models` → `Vec<ModelConfig>` (`hasApiKey`/`isOfficial`/`displayName`/`apiType`); Rust `ModelConfig::find_user_configured`/`find_all` | `cli_types.rs:156-162`, `cli_type.rs:309/345` |
| Model picker (FE) | `useModelConfigForExecutor` merges custom+official, auto-selects, exposes `selectedModelConfigId`/`setSelectedModelConfigId`; rendered as a `ToolbarDropdown` with Custom/Official sections | `useModelConfigForExecutor.ts:56`, `CreateChatBox.tsx:143-201` |
| Metered/subscription discriminator | `InteractiveAuthMode::resolve(api_key, base_url)` → `NativeOauth` (None,_) / `OfficialKey` (Some,None) / `Relay` (Some,Some) | `cc_switch.rs:594-604` |
| Stage template | `generate_audit_plan` — free `async fn(&dyn LLMClient,...)` that fail-closes to a default on LLM/parse error | `audit_plan.rs:22-70` |
| Judge shape | `AuditScoreResult::parse` (dimensions → total vs `PASS_THRESHOLD`), parse-failure = failing score | `types.rs:631`, `:624` |
| Loop cap idiom | `FINAL_REPAIR_MAX_ROUNDS = 4` + guard `bail!` | `agent.rs:115`, `:8203` |
| G2 confirm hard-block | Materialize rejected if `gates_confirmed_at` is null (server-side) | `planning_drafts.rs:976`, handler `:436`, mount `:162` |

**The declarative gap:** there is no data-driven rule type and no DB table for editable rules. This feature builds both, additively. **Everything else in the authoring + enforcement pipeline is reuse** (see Reuse Map).

---

## 5. Design Principles

1. **Generate with AI, enforce without it.** The LLM is confined to authoring (one-time, multi-validated, human-confirmed). After confirmation a rule is fixed data the deterministic engine runs identically forever. A correct rule stays correct regardless of model drift; the enforcement path never calls an LLM and never feeds scanned source into a prompt.
2. **Rules are data, never code.** A rule is a scoped-regex JSON object (P1) or an ast-grep YAML document (P2). The matcher cannot open files, sockets, or spawn processes — the safety property is *structural*.
3. **Empirical ground truth over LLM consensus.** Two agreeable LLMs converge on wrong answers; an executed positive/negative fixture cannot be argued out of a behavioral failure. The empirical test is authoritative over any judge opinion.
4. **Adversarial, not agreeable.** The second agent's job is to *break* the rule (false-positives, evasions, ambiguity), not to nod along.
5. **Context-starved independence.** The round-trip interpreter sees *only* the rule body — it cannot launder the original intent back in, making the round-trip a genuine independent check.
6. **Advisory by default, opt-in to gate (D3).** Custom-rule severity is capped to `Major` (`AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig`, via `cap_for_advisory`); gating happens only through the explicit opt-in `CustomRuleCritical` count metric. A generated rule cannot self-escalate to `Blocker`.
7. **Additive & reversible.** New provider, new tables, new routes, new MetricKeys — nothing existing changes shape. Default-off; per-project shadow→enforce.
8. **Fail-closed, bounded.** Every authoring stage fail-closes to a safe default; the loop has a hard cap of 4 (D6); enforcement timeouts emit a Blocker in Enforce mode.
9. **Reuse over rewrite (D9).** Prefer an existing component to a new one in every layer (LLM invocation, severity machinery, persistence patterns, picker UI, confirm dialog). Anything v1 proposed building that already exists is flagged in the Reuse Map and removed from the net-new list.

---

## 6. The Declarative Rule Format (the linchpin)

### 6.1 Choice (phased — D5)
Every custom rule carries a `rule_format` discriminant so both formats coexist and simple token/header bans never pull the AST path:
- **`regex` (P1, ships first, zero new dependencies)** — a scoped-regex declarative JSON schema built on the crate's existing `regex` 1.x (already a dependency, `crates/quality/Cargo.toml:25`), modeled directly on `rules/typescript/console_usage.rs`. Covers ban-token / forbid-substring / required-header style rules across Rust and TS.
- **`ast_grep` (P2, additive expressiveness upgrade)** — ast-grep YAML, embedded in-process via the MIT crates **`ast-grep-core` + `ast-grep-config` + `ast-grep-language`** (bundled Rust + TypeScript tree-sitter grammars), for structural rules like "forbid `.unwrap()` in handlers". Added **only after** the unverified ast-grep linter-envelope field names are confirmed (see §17, the one remaining technical open item).

The `rule_format` discriminant is present in the schema and DB from P1 so P2 is a pure additive step, not a migration.

### 6.2 Rationale
The P1 scoped-regex format is the reuse-first minimum: it adds **zero** dependencies (`regex = "1"` is already in `crates/quality/Cargo.toml:25`), reuses the proven `console_usage.rs` compiled-`Regex` + per-line `find_iter` + `is_test_file` skip pattern, and is linear-time-safe (no backtracking/lookaround/backrefs), so it covers the majority of "prohibit token X" requests immediately.

For the P2 structural tier, ast-grep is the **only** candidate that is simultaneously: (a) pure declarative **data**, never executed as code; (b) **one schema across Rust AND TS**, closing today's `syn`-vs-regex split where TS has no AST; (c) embeddable as **license-clean Rust crates with no external binary/subprocess** (unlike semgrep's OCaml process + restrictive rule license); (d) **compact and code-shaped** so an LLM generates it reliably (unlike tree-sitter `.scm` S-expressions, which are LLM-hostile and force a DIY loader); and (e) **testable in-process** against snippets by asserting `NodeMatch` count/locations.

**Avoid:** semgrep/opengrep (subprocess + restrictive rule license + largest sandbox surface) and raw tree-sitter queries (LLM-hostile + DIY loader).

### 6.3 Examples

**Scoped-regex (P1) — forbidden token / file-header presence (zero new deps):**
```json
{
  "kind": "forbidden_token",
  "languages": ["rust", "typescript"],
  "pattern": "\\bdbg!\\s*\\(",
  "message": "dbg! macro left in committed code",
  "exclude_globs": ["**/tests/**"]
}
```

**Scoped-regex (P1) — ban `console.log` in committed TS (mirrors `console_usage.rs`):**
```json
{
  "kind": "forbidden_token",
  "languages": ["typescript"],
  "pattern": "console\\.log\\s*\\(",
  "message": "console.log left in committed TS",
  "exclude_globs": ["**/*.test.ts", "**/*.spec.ts"]
}
```

**ast-grep (P2) — forbid `.unwrap()` outside tests (structural; the exact `error_handling.rs` behavior, declaratively):**
```yaml
id: no-unwrap-outside-tests
language: rust
severity: warning
message: ".unwrap() outside test code can panic in production"
rule:
  pattern: $E.unwrap()
  not:
    inside:
      any:
        - matches: inside-cfg-test   # util sub-rule for #[cfg(test)] scope
        - matches: inside-test-fn
```

> **[unverified]** Exact ast-grep linter-envelope field names (`id`/`message`/`severity`/`note`/`constraints`/`utils`) must be confirmed against the pinned ast-grep crate version **before coding the P2 loader** (this is the one remaining technical open item, §17). The matching categories (atomic `pattern`/`kind`/`regex`; relational `inside`/`has`/`follows`/`precedes`; composite `all`/`any`/`not`/`matches`) are confirmed. P1 does not depend on this.

### 6.4 Sandbox model
- **No FS / no network / no subprocess** — a pattern-rule spawns no child at all (unlike the node-subprocess providers at `provider/mod.rs:120`), so it inherits no-FS/no-network trivially. (Note: this is also why the authoring turn does **not** use a PTY — the scan path is pure in-process matching; the PTY discussion in §7.2 concerns only how the *subscription authoring* LLM is invoked, not enforcement.)
- **Regex sandbox** — Rust `regex` 1.x is linear-time (no backtracking/lookaround/backrefs), so ReDoS-by-backtracking is impossible. The remaining vector is DFA memory blowup (e.g. `a{0,1000000}`): compile **once at load** via `RegexBuilder::new(p).size_limit(1<<20).dfa_size_limit(1<<20).build()`; reject failures as a user-facing 400, never at scan time.
- **Input bounds** — cap per-file bytes; skip/truncate pathological minified lines; reuse `analysis::is_excluded` (`analysis/mod.rs:21`) so `node_modules`/`target`/`dist`/`.git`/`vendor`/`.next`/`build` are never walked.
- **AST parse-error skip (P2)** — preserve the `syn`/tree-sitter parse-error skip pattern (`builtin_rust.rs:77`) so hostile scanned source can't panic an AST rule.

### 6.5 How it feeds the engine
A new provider (Section 8) walks files, runs each compiled rule, and for every match builds:
```rust
QualityIssue::new_capped(def.rule_id, def.rule_type, def.severity,
    AnalyzerSource::CustomRule, msg).with_location(file, line)
```
(Note: `new_capped` + `CustomRule`, **not** the `console_usage.rs` `::new` + `Other("built-in")` pattern — the cap is the whole point of D3.) It then aggregates counts into two new `MetricKey`s. Per the engine's decoupling, **issues alone never gate — only metrics referenced by a condition do** (`engine.rs` aggregation; gate conditions read only the `metrics` map). So enforcement requires publishing a numeric count metric, mirroring `builtin_rust_critical` (`builtin_rust.rs:117-147`).

---

## 7. Feature Spec

### 7.1 Metric/rule tooltips ("!" + current value — D7)
- Enrich `MetricCatalogResponse` (`quality.rs:198`) with `info: MetricInfo[]`, where `MetricInfo { key, displayName, description, example, higherIsWorse }` is a **compiled static table**, one entry per selectable `MetricKey`. (Static between deployments → `staleTime` 1h is fine.)
- Add `GET /api/projects/{id}/quality-metrics/latest` → `ProjectMetricSnapshot { values: Record<MetricKey, MeasureValue>, runId, ranAt }`, sourced from the **latest persisted `quality_run.report_json`** (already persisted; reuse the existing runs). **D7: do NOT trigger a fresh costly recompute on hover** — the tooltip is labelled "as of `<ranAt>`" and degrades to "no run yet" when nullable.
- In `QualityGateRulesEditor.tsx`, add an optional `metricInfo?: MetricInfo[]` prop threaded exactly like `metricOptions`. Render a circled-"!" button next to the Metric `<select>` (lines 174-194). On hover/focus show a popover: displayName, description, example, current project value + its `ranAt` timestamp.
- For **custom** rules, the same "!" shows the LLM-generated `description` stored on `custom_rule` — so generated rules remain self-documenting **after** application (user plan step 2).

### 7.2 NL→rule generation — dual-source, all-global, user-selectable engine (D1)

**The authoring engine is ALL globally-configured LLM sources, USER-SELECTABLE.** The user picks which configured source authors the rule, via the existing model-selection UI; there is **one "authoring-model invoker" abstraction** (§8.6) with **two backends, both reusing existing infrastructure**.

**CRITICAL CORRECTION to the prior draft and to the "PTY" premise.** The existing interactive transport does **NOT** drive a PTY for a single turn, and the authoring turn must not either. It uses `tokio::process::Command` with stdin/stdout/stderr = `null` (a piped one-shot, `container.rs:1064-1073`): closed stdin makes the genuine `claude` run exactly ONE turn and exit RC=0 after writing its on-disk transcript JSONL; the assistant reply is read by **tailing that transcript file**, never from stdout. The separate workflow-terminal `-p` PTY mechanism (`terminal/process.rs ProcessManager::spawn_pty_with_config`) is NOT what the native-OAuth transport reuses and is NOT used here. The authoring pipeline reuses the **piped-one-shot + transcript-tailer** seam.

**Entry point — per-gate header "Generate rule with AI".** The gate header (`QualityGateRulesEditor.tsx:142-154`, next to "Add condition") opens the new `RuleAuthoringDialog.tsx`. A textarea captures the NL request; the dialog POSTs `nlRequest` + `currentRulesContext` (the live `value` conditions) **plus the user-selected `modelConfigId` (+ `cliTypeId`)** to `POST /api/projects/{id}/custom-rules/author`.

**Model selection — reuse the existing picker (D1, D9).** The dialog lifts the `CreateChatBox.tsx` `ToolbarDropdown` picker (lines 143-201) fed by `useModelConfigForExecutor(executor, workflowModelLibrary)` (`useModelConfigForExecutor.ts:56`). That hook already enumerates **every** globally-configured model per CLI (via `useModelsForCli` → `GET /api/cli_types/:cli/models`), MERGES custom (from `workflow_model_library`) + official (DB rows where `isOfficial && hasApiKey`), marks each `ModelOption{ id, displayName, subtitle, isCustom, hasApiKey }`, auto-selects, and exposes `selectedModelConfigId`/`setSelectedModelConfigId`. The dialog binds `selectedId`/`onChange` and submits the chosen `model_config_id`. **No new enumeration or picker code.**

**Backend dispatch — the two reused invoker backends (server resolves by the chosen source):**

1. **Metered API-key sources → existing `LLMClient`.** Resolve the user's explicitly-chosen config via `ModelConfig::resolve_preferred_or_default(pool, Some(chosen_id), cli_type_id)` (`cli_type.rs:387`). If the resolved row has a key, decrypt via `ModelConfig::get_api_key()` (`cli_type.rs:186`) — **never** read `encrypted_api_key`/`orchestrator_api_key` directly (decryption is automatic, AES-256-GCM, `crates/db/src/encryption.rs`) — build an `OrchestratorConfig` via `OrchestratorConfig::from_workflow(api_type, base_url, api_key, model)` (`config.rs:290`), then `create_llm_client(&config)` (`llm.rs:922`). This HTTP path is metered against the user's own key and **exempt** from the subscription credit pool. **Constraint:** `OrchestratorConfig::validate()` (`config.rs:320-331`) whitelists `api_type ∈ {openai, anthropic, openai-compatible, anthropic-compatible}` — a `google` `ModelConfig` (offered by `Step3Models`) is **rejected** here and cannot author via this backend; surface this to the user in the picker (disable/annotate google rows).

2. **Official-subscription / native-OAuth sources → the existing interactive transport (off the credit pool).** When the chosen source has **no api key** (the native/subscription model), the invoker drives the genuine `claude` binary through the **already-built, already-wired no-`-p` interactive native-OAuth transport** — NOT the metered API, NOT a PTY, and NOT the `-p`/Agent-SDK pool. Two reuse layers, pick the highest the pipeline can use:

   - **Highest (preferred when a Workspace+Session is cheap):** call the trait method `ContainerService::start_execution(workspace, session, &ExecutorAction, &run_reason)` (impl `container.rs` → `start_execution_inner`, `container.rs:2108`) with a `CodingAgentInitialRequest { prompt = the rule-authoring instruction, executor_profile_id (ClaudeCode), working_dir, allow_user_questions: false }` (`coding_agent_initial.rs:18`). The router auto-selects native-OAuth via `try_spawn_interactive_native_oauth` (`container.rs:1468`) BEFORE the `-p` path; the services-layer `start_execution` (`services/container.rs:1046`) runs the single `normalize_logs` pass + `spawn_stream_raw_logs_to_db`; the assistant reply is captured into `coding_agent_turn.summary` by `extract_last_assistant_message` (`container.rs:1201`). The pipeline reads the reply from the summary (or subscribes to the `MsgStore` for streaming).
   - **Lowest (when a Workspace is too heavy):** replicate the canonical recipe verbatim from `crates/local-deployment/tests/interactive_transport_smoke.rs`: `cc_switch::create_interactive_isolated_home(None, &wd)` (`cc_switch.rs:544`) → `cc_switch::setup_interactive_auth(&home, /*api_key*/ None, /*base_url*/ None, &model, /*native_src*/ ~/.claude)` (`cc_switch.rs:707`, which copies `~/.claude/.credentials.json` and scrubs `ANTHROPIC_API_KEY/AUTH_TOKEN/BASE_URL/CLAUDE_CODE_OAUTH_TOKEN` so the run stays on the subscription plan and OFF the metered API) → build `ClaudeCode { interactive: Some(true), interactive_session_id: Some(home.session_uuid), model, .. }.build_interactive_command_parts()` (`claude.rs:204`, emits `--session-id <uuid>`, NO `-p`) → `.into_resolved().await` → `LocalContainerService::spawn_interactive_claude(...)` (`container.rs:1044`, the piped one-shot, NOT a PTY) → `add_child_to_store` (`container.rs:182`) → await `LogMsg::Finished` → `ClaudeCode::normalize_logs` (`claude.rs:336`) + scan history for `assistant_message` (smoke-test `assistant_texts()` helper, or reuse `extract_last_assistant_message`) → **CRUCIALLY** call `ProcessManager::cleanup_logical_session_home` (`process.rs:434`) at session end to remove the credentials-bearing interactive home (RB-37), since the low-level seam has no Workspace to drive `cleanup_workspace`.

**Billing guarantee for the subscription backend.** To force the free subscription path, the invoker passes `api_key = None`/`base_url = None` to `InteractiveAuthMode::resolve` (`cc_switch.rs:597` → `NativeOauth`), ensures `~/.claude/.credentials.json` exists, and **never sets `SOLODAWN_NO_POOL`** (which would force the metered `-p` fallback). Because the *user explicitly selected* the source, the precedence ambiguity at `container.rs:1539-1564` (a fallthrough `config_id=None` could mis-bill a subscription user who also has a saved key) does not arise — the picker always submits an explicit id, and the no-key sentinel is what selects native.

**Cost posture.** Metered sources draw the user's own key (pool-exempt HTTP); the subscription source runs on the user's official plan with **zero** metered-API/credit-pool cost. If the user selects a source with no usable credential → `ApiError::BadRequest` with an actionable message. There is no silent fallback between backends — the chosen source's mode (key vs native) deterministically picks the backend.

### 7.3 The validation pipeline (named steps)
Module `crates/services/src/services/rule_authoring/`. `const MAX_AUTHORING_ROUNDS: usize = 4;` (mirrors `FINAL_REPAIR_MAX_ROUNDS`, `agent.rs:115`; D6 — fixed, configurability deferred). Every stage is a free `async fn(&dyn LLMClient, ...) -> StageResult` that fail-closes (the `audit_plan.rs:22-70` template); JSON parsed via `extract_json_from_mixed_response` (`agent.rs:5511`) + `normalize_instruction_payload` (`agent.rs:5496`). **Because every stage takes `&dyn LLMClient`, the same pipeline runs unchanged against BOTH invoker backends** — the metered `create_llm_client` box and the subscription `InteractiveClaudeClient` box are the same trait object (this is the key reuse that makes D1 nearly free; the only call-site decision is which box to build).

| Step | Agent | Module fn | What it does |
|---|---|---|---|
| 1 GENERATE | **Proposer** | `generate::draft_rule` | From NL + currently-edited rules, emit `CandidateRule {rule_format, rule_body, description, rule_type, severity, mapped_metric}` **plus 2-3 positive (MUST trip) + 2-3 negative (MUST NOT trip) examples** — emitting examples is required. Fail-closed default = a minimal no-op rule flagged invalid. |
| 2 ADVERSARIAL REVIEW | **Adversary** | `adversary::attack` | Distinct system prompt to *break* the rule: (a) false-positive snippets (over-reach, e.g. inline `#[cfg(test)]` — the real `test_file_absence` bug), (b) evasion snippets that satisfy the intent but slip the pattern, (c) ambiguity/scope complaints. Its snippets are **appended as permanent fixtures**. |
| 3 EMPIRICAL TEST | *(deterministic, no LLM)* | `empirical_test::evaluate` | Compiles the candidate and **executes** it via the shared `quality::run_candidate(compiled_rule, snippet, virtual_path)` over every positive/negative/false-positive/evasion snippet. Pass = all positives flagged, all negatives + false-positives clean, all evasions flagged. **Authoritative.** |
| 4 JUDGE | **Judge** | `judge::score` | `AuditScoreResult`-shaped (intent-fidelity, precision/no-overreach, recall/no-evasion, clarity) vs `PASS_THRESHOLD` (`types.rs:624`). A failing empirical report **forces** `passed=false` regardless of LLM opinion. If `!passed` → `generate::revise(...)` and loop. |
| — CAP | — | — | If the loop exhausts `MAX_AUTHORING_ROUNDS` (=4) → return `outcome=capped_out` with the best candidate + all transcripts (does **not** panic — hands back to the user; D6). |
| 5 USER CONFIRM (MANDATORY) | *(human)* | UI | The full `AuthorRuleResult` is shown in `RuleAuthoringDialog` and **must** be confirmed — confirmation is never optional (D2). **Nothing is persisted until Confirm.** On confirm → a `custom_rule` row at `status='shadow'`. |
| 6 CONTEXT-FREE REVERSE-ENGINEER | **Interpreter** | `reverse_engineer::interpret` | A **fresh** `LLMClient` invocation whose entire input is the confirmed `rule_body` + `description` — **no** nl_request, **no** debate, **no** examples. Output: `ReconstructedRequest`. |
| 7 JUDGE-COMPARE | **Matcher** | `reverse_engineer::compare` | Explicit judge verdict: does the reconstruction semantically match the original request? `RoundTripVerdict {judgePassed, judgeScore, reconstructedRequest, rationale}`. **Match** → persist `custom_rule_validation{verdict='pass', roundtrip_ok=1}`; rule stays shadow. **Mismatch** → re-enter Step 1/2 with the reconstructed-vs-original delta as extra fix instructions, counting against the cap; after the cap, persist `{verdict='fail', roundtrip_ok=0}` and hand back to the user (fall-to-manual). |

Agent roster (all one `&dyn LLMClient`, distinct prompts; Step 6 uses a fresh message vector with no history): Proposer → Adversary → [deterministic 3] → Judge → [human 5, mandatory] → Interpreter (context-free) → Matcher. `MockLLMClient` (`llm.rs:100-155`) unit-tests each stage and the loop/cap deterministically.

### 7.4 Deterministic enforcement (AI-free)
Every gate run, `resolve_quality_config` (`quality_policy.rs:26`) additionally loads the project's enabled `custom_rule` rows, compiles them (P1: size-limited regex; P2: parsed ast-grep YAML), and constructs the `DeclarativeRuleProvider` with them. `QualityEngine::run` executes them deterministically and identically. `custom_rule_critical > 0` (only if the project opted into enforce) gates exactly like any builtin metric. The provider never spawns a subprocess and never calls an LLM.

### 7.5 Self-documenting tooltips
The Proposer emits the plain-language `description` that powers the "!" tooltip, so every custom rule is self-documenting; the tooltip also shows the metric's current project value as of the latest run (7.1, D7). Built-in metric descriptions are the static compiled catalog; custom-rule descriptions live in the DB.

### 7.6 Edit-revalidation policy (D8)
- Editing a rule **body** (the matching logic: `rule_body`, `rule_format`, `severity`, `mapped_metric`) **re-runs the full authoring validation pipeline** (§7.3 Steps 1-7 via `POST .../{ruleId}/revalidate`) and **drops the rule back to `status='shadow'`** until it passes again. Same pipeline, no new code path.
- **Metadata-only edits** (`name`, `description` text) **skip re-validation** and do not change status — they only bump `version` and write a `custom_rule_audit` row.
- The server decides which path by diffing the submitted `CustomRuleInput` against the persisted row: any change to a body field triggers revalidation; a change confined to name/description does not.

---

## 8. Architecture & Data Flow

### 8.1 Components by crate
- **`crates/quality`** — new `provider/declarative.rs` (`DeclarativeRuleProvider` impl `QualityProvider`); shared `run_candidate(compiled_rule, snippet, virtual_path) -> Vec<QualityIssue>`; new `AnalyzerSource::CustomRule`; new `MetricKey::{CustomRuleViolations, CustomRuleCritical}`; `ProvidersConfig.declarative_rules` toggle; `build_providers` registration. **P1 adds no Cargo deps** (regex already present, `Cargo.toml:25`); **P2 adds the ast-grep crates.**
- **`crates/services`** — new sibling module `services/rule_authoring/` (`mod.rs`, `generate.rs`, `adversary.rs`, `empirical_test.rs`, `judge.rs`, `reverse_engineer.rs`), registered in `services/mod.rs`; plus the thin **authoring-model invoker** (§8.6). `quality_policy.rs` extended to load + compile custom rules.
- **`crates/server`** — extend `routes/quality.rs` with the tooltip catalog, current-value endpoint, custom-rule CRUD, and the authoring/revalidate routes (the author route accepts the selected `model_config_id` + `cli_type_id`).
- **`crates/local-deployment` / `crates/executors`** — **no changes**; the subscription invoker backend *reuses* the existing `ContainerService::start_execution` / `try_spawn_interactive_native_oauth` / `spawn_interactive_claude` and the `ClaudeCode` interactive argv builders as-is.
- **`crates/db`** — migration `20260620120000_create_custom_rules.sql` + 4 models + `mod.rs` registration.
- **`frontend`** — `metricInfo` prop + "!" popover + custom-rules section in `QualityGateRulesEditor.tsx`; new `RuleAuthoringDialog.tsx` (lifts the `CreateChatBox` model picker + `useModelConfigForExecutor`); extend G2 `QualityGateConfirmDialog.tsx`; `useQualityPolicy.ts` hooks.

### 8.2 Provider contract (new `DeclarativeRuleProvider`)
- Name `"declarative-rules"`. Receives a `Vec` of compiled custom-rule definitions at construction (the quality crate stays **DB-free** — the verified G3 boundary).
- Walks files via `analysis::collect_files` (`analysis/mod.rs:33`) + `analysis::is_excluded` (`analysis/mod.rs:21`) (the `builtin_common.rs:90-135` template).
- For `regex` rules (P1): runs a size-limited compiled `Regex` per line (the `console_usage.rs` template, but with `new_capped` + `CustomRule`). For `ast_grep` rules (P2): runs `ast-grep-core` over the bundled Rust/TS grammar selected by file extension.
- Maps every match to `QualityIssue::new_capped(...)` (`issue.rs:105`) and aggregates counts into the two new MetricKeys (the `builtin_rust.rs:117-150` publish template).
- `applicable_metrics()` returns `Vec::new()` when zero rules are loaded (the `rust_analyzer.rs` empty-when-inapplicable pattern), so an empty rule set never fail-closes or false-positives unrelated repos.
- `analyze()` is wrapped in `tokio::time::timeout`; on timeout in Enforce mode it emits a Blocker (fail-closed, matching the empty-scan branch `engine.rs:317`). A provider `Err` degrades to a metric-less failure report → fail-closed; return `Ok` + sentinel when "no rules ran" should be benign.

### 8.3 Severity authority (D3)
New `AnalyzerSource::CustomRule` (`rule.rs:158-178`), mapped in the **exhaustive** `severity_origin()` match (`rule.rs:205`) to `SeverityOrigin::ProjectConfig` — so LLM/user-authored severities are capped at `Major` (non-blocking) by `cap_for_advisory` (`rule.rs:124`), exactly the model-taste argument already encoded for `EsLint` (`rule.rs:208`). The pinned `severity_origin_classification_is_pinned` test (`rule.rs:300-358`) and the `cap_routes_through_severity_origin` test (`rule.rs:332`) must be extended for the new variant. Gating happens **only** through the explicit opt-in `CustomRuleCritical` count metric the operator selects — **never** through a self-declared `Blocker`. Reuse the existing cap + metric-condition machinery; no new escalation path is introduced.

### 8.4 Text sequence — authoring (dual-source)
```
UI NL request + user-selected model (useModelConfigForExecutor picker)
  -> POST /api/projects/{id}/custom-rules/author { nlRequest, modelConfigId, cliTypeId, currentRulesContext? }
  -> authoring-model invoker resolves the chosen source:
       if it has a key  -> resolve_preferred_or_default(Some(id),cli) -> get_api_key
                           -> OrchestratorConfig::from_workflow -> create_llm_client      (METERED, pool-exempt)
       if it has NO key -> subscription native-OAuth interactive transport (NOT a PTY, OFF the pool):
                           ContainerService::start_execution(ClaudeCode CodingAgentInitialRequest)
                           [or low-level: create_interactive_isolated_home -> setup_interactive_auth(None,None,..)
                            -> build_interactive_command_parts -> spawn_interactive_claude -> tail transcript
                            -> normalize_logs -> extract assistant reply -> cleanup_logical_session_home]
  -> both yield a Box<dyn LLMClient>; rule_authoring::author_rules runs unchanged:
       Proposer -> [loop: Adversary -> empirical(run_candidate) -> Judge -> revise] (cap = MAX_AUTHORING_ROUNDS = 4)
  -> AuthorRuleResult {candidate, examples, empirical, debate, roundTrip, outcome, roundsUsed}
  -> UI shows two-agent transcript + empirical evidence
  -> user Confirm (MANDATORY)
  -> POST .../custom-rules (persist) at status=shadow
  -> Interpreter (fresh, context-free) -> Matcher (judge-compare)
  -> persist custom_rule + _example + _validation + _audit
```

### 8.5 Text sequence — enforcement (AI-free)
```
gate run
  -> resolve_quality_config (DB priority-0) loads enabled custom_rule rows
  -> compile (P1 size-limited regex / P2 parsed ast-grep YAML) + construct DeclarativeRuleProvider
  -> QualityEngine::run executes providers concurrently (deterministic)
  -> CustomRuleViolations / CustomRuleCritical published into metrics map
  -> if project opted into enforce: condition `custom_rule_critical GT 0` gates like any builtin metric
```

### 8.6 The authoring-model invoker (the one new abstraction over both backends — D1)
A thin server-side seam — `rule_authoring::invoker` — that turns the user's selected source into a `Box<dyn LLMClient>`, so the entire `rule_authoring` pipeline stays backend-agnostic (`&dyn LLMClient`). It does **not** wrap a new transport; it *chooses between two existing ones*:

```
fn build_authoring_client(pool, project_id, model_config_id, cli_type_id)
    -> Result<Box<dyn LLMClient>, ApiError>
  let cfg = ModelConfig::resolve_preferred_or_default(pool, Some(model_config_id), cli_type_id)?; // cli_type.rs:387
  match InteractiveAuthMode::resolve(cfg.get_api_key()?.as_deref(), cfg.base_url.as_deref()) {     // cc_switch.rs:597
    OfficialKey | Relay  => {                 // metered, pool-exempt HTTP
        let oc = OrchestratorConfig::from_workflow(cfg.api_type, cfg.base_url, key, cfg.api_model_id); // config.rs:290
        create_llm_client(&oc)                // llm.rs:922  (validate() rejects api_type=google)
    }
    NativeOauth          => {                 // subscription, OFF the credit pool, NOT a PTY
        // either drive ContainerService::start_execution (preferred) and adapt its summary as the reply,
        // or use the already-built create_interactive_claude_client(model) (llm.rs:868) which itself
        // returns an InteractiveClaudeClient (llm.rs:628) impl LLMClient against the genuine claude binary.
        create_interactive_claude_client(&cfg.api_model_id)
            .ok_or(ApiError::BadRequest("no native subscription credentials"))?
    }
  }
```

Two backends, both reuse: (a) the metered `LLMClient` via `create_llm_client`; (b) the subscription native-OAuth interactive transport via the existing `create_interactive_claude_client` (`llm.rs:868`, which constructs the already-built `InteractiveClaudeClient`, `llm.rs:628`) — or, for full lifecycle, `ContainerService::start_execution`. The discriminator is `InteractiveAuthMode::resolve` (`cc_switch.rs:597`); the user's explicit `model_config_id` avoids the `container.rs:1539-1564` mis-billing fallthrough.

> **rev2 reuse flag (D9):** The prior draft (§7.2) treated the interactive subscription path as a "fallback to avoid". That **under-reused** an existing, trait-compatible runner: `InteractiveClaudeClient` (`llm.rs:628`) already implements the same `LLMClient` trait as `create_llm_client`, is already consumed at `planning_drafts.rs:375/587` and `agent.rs:199`, and runs on the user's official subscription **off** the credit pool. rev2 promotes it to a first-class, user-selectable authoring backend. **There is no new transport to build.**

---

## Reuse Map (existing-reuse vs new-glue — D9)

Two columns: what already exists and is reused as-is (file:line verified), versus the minimal net-new glue. The guiding rule is "prefer the existing component; flag anything previously proposed-to-build that already exists."

| Concern | REUSE (existing, file:line) | NEW (minimal glue) |
|---|---|---|
| **Metered LLM call** | `LLMClient` trait `llm.rs:23`; `create_llm_client` `llm.rs:922`; `OrchestratorConfig::from_workflow` `config.rs:290`; `validate()` whitelist `config.rs:320` | call-site only (build `OrchestratorConfig` from the chosen `ModelConfig`) |
| **Subscription LLM call (off pool)** — *was flagged "build a PTY runner"; ALREADY EXISTS* | `InteractiveClaudeClient` impl `LLMClient` `llm.rs:628-703/:706`; `create_interactive_claude_client` `llm.rs:868`; `ContainerService::start_execution` → `start_execution_inner` `container.rs:2108`; `try_spawn_interactive_native_oauth` `container.rs:1468`; `spawn_interactive_claude` `container.rs:1044`; `create_interactive_isolated_home` `cc_switch.rs:544`; `setup_interactive_auth` `cc_switch.rs:707`; `InteractiveAuthMode::resolve` `cc_switch.rs:597`; `build_interactive_command_parts` `claude.rs:204`; `normalize_logs` `claude.rs:336`; `extract_last_assistant_message` `container.rs:1201`; smoke recipe `interactive_transport_smoke.rs`; cleanup `cleanup_logical_session_home` `process.rs:434` | the `build_authoring_client` dispatch (§8.6) + (if low-level) one explicit `cleanup_logical_session_home` call |
| **Authoring-model invoker** | both backends above; discriminator `cc_switch.rs:597` | one thin `invoker` fn (§8.6) selecting backend by source |
| **Model enumeration** | `GET /api/cli_types/:cli/models` `cli_types.rs:156`; `ModelConfig::find_by_cli_type` `cli_type.rs:205`; `find_user_configured`/`find_all` `cli_type.rs:309/345`; FE `useModelsForCli` `useCliTypes.ts:155` | none |
| **Model picker UI** | `useModelConfigForExecutor` `useModelConfigForExecutor.ts:56`; `ToolbarDropdown` Custom/Official sections `CreateChatBox.tsx:143-201`; native sentinel `NATIVE_MODEL_ID` `workflow/types.ts:137` | bind the lifted dropdown into `RuleAuthoringDialog` |
| **Credential resolution** | `ModelConfig::get_api_key` `cli_type.rs:186`; `resolve_preferred_or_default` `cli_type.rs:387`; `find_with_credentials_for_cli` `cli_type.rs:401`; `Workflow::get_api_key` `workflow.rs:206`; AES-256-GCM `db/src/encryption.rs` | none (always pass explicit `model_config_id`) |
| **Authoring stages** | fail-closed template `audit_plan.rs:22-70`; `extract_json_from_mixed_response` `agent.rs:5511`; `normalize_instruction_payload` `agent.rs:5496`; `AuditScoreResult`/`::parse`/`PASS_THRESHOLD` `types.rs:557/631/624`; `MockLLMClient` `llm.rs:100-155` | `rule_authoring/{mod,generate,adversary,empirical_test,judge,reverse_engineer}.rs` + prompts + structs |
| **Loop cap** | `FINAL_REPAIR_MAX_ROUNDS=4` `agent.rs:115`; guard `bail!` `agent.rs:8203` | `const MAX_AUTHORING_ROUNDS=4` clone (D6) |
| **Rule trait/types** | `Rule`/`RustRule`/`TsRule`/`CommonRule` + `RuleConfig` `rules/mod.rs:18-144` | `DeclarativeRuleProvider` + `run_candidate` |
| **Issue construction** | `QualityIssue::new_capped` `issue.rs:105`; `.with_location` `issue.rs:117` | use `new_capped`+`CustomRule` (NOT `console_usage.rs` `::new`+`Other`) |
| **Severity cap (D3)** | `cap_for_advisory` `rule.rs:124`; `SeverityOrigin` `rule.rs:146`; `AnalyzerSource` `rule.rs:158`; exhaustive `severity_origin()` `rule.rs:205`; pinned tests `rule.rs:300-358` | 1 `CustomRule` variant + 1 match arm + extend pinned tests |
| **Metrics/gate** | `MetricKey` `metrics.rs:14`; `as_str`/`display_name` `metrics.rs:176/226`; gate `Operator(Gt/Lt)`/`Condition` `gate/condition.rs:18`; `selectable_metric_keys` `quality.rs:207`; `MetricCatalogResponse` `quality.rs:198`; `get_metric_catalog` `quality.rs:277` | 2 `MetricKey` variants (3 sites + selectable + ts regen) + `info` field |
| **Regex format (P1)** | `regex="1"` already a dep `Cargo.toml:25`; template `rules/typescript/console_usage.rs` | scoped-regex JSON schema + size-limited compile |
| **AST format (P2)** | (none yet) | ast-grep crates + loader (after schema confirm, §17) |
| **File walk** | `analysis::collect_files` `analysis/mod.rs:33`; `is_excluded` `analysis/mod.rs:21` | none |
| **Metric-publish** | `builtin_rust.rs:117-150` template | publish 2 new metrics |
| **DB pattern** | Uuid/BLOB model+migration `project_quality_policy.rs:15-67`; `insert_batch` `quality_issue.rs:103-138`; migration template `20260614120000_create_project_quality_policy.sql`; `models/mod.rs:25-39`; `SCHEMA_EXPECTATIONS` `lib.rs:31` | migration `20260620120000` (4 tables) + 4 models + registration |
| **Shared editor** | `QualityGateRulesEditor.tsx` (props 14-23; `addCondition`/`updateCondition` 73-90; gate header 142-154; metric select 174-194; raw `text-slate-*` classes) | `metricInfo` prop + "!" popover + custom-rules section |
| **Confirm dialog (D2)** | G2 `QualityGateConfirmDialog.tsx` (editor render 177-184; Save&Confirm 111-141); hard-block `planning_drafts.rs:976` (handler `:436`, mount `:162`) | append read-only rule/examples/empirical/round-trip panel; keep mandatory |
| **Query/mutation** | `useQualityPolicy.ts` keys 9-14 + hooks 39-58; `lib/api.ts` `qualityPolicyApi` 674-704 + `makeRequest`/`handleApiResponse` 119/248 | `useGenerateRule` + `useCustomRules` + `customRules` key |
| **ts-rs export** | `MetricKey` `#[derive(TS)]` `metrics.rs:12`; workspace dep `Cargo.toml:23` | `#[derive(TS)]` on new DTOs + regen |

**Explicit "already exists, do NOT build" flags (D9):**
1. **Subscription/native interactive runner** — ALREADY built and trait-compatible (`InteractiveClaudeClient` `llm.rs:628`/`:868`); the prior §7.2 under-reused it. Do not build a PTY runner or any new transport.
2. **regex 1.x** — ALREADY a dep (`Cargo.toml:25`); do not add. (ast-grep is genuinely new, P2 only.)
3. **Model enumeration + picker** — ALREADY exist (`useModelConfigForExecutor` + `CreateChatBox` dropdown + `GET /api/cli_types/:cli/models`); lift, do not rebuild.
4. **ts-rs export harness** — ALREADY wired (`MetricKey`); only new DTOs need `#[derive(TS)]` + regen.
5. **G2 confirm dialog + hard-block** — ALREADY exist; extend, never replace (`gates_confirmed_at` block at `planning_drafts.rs:976`).
6. **No pre-existing `custom_rule`/`rule_authoring`/declarative scaffolding** — confirmed absent in `crates/**/*.rs`, so the remaining NEW items are genuinely net-new, not duplicates.

**Minor cite-drift corrections carried into rev2:**
- §11 styling: `QualityGateRulesEditor.tsx` uses raw `text-slate-*`/`bg-white` Tailwind, **not** `.new-design` tokens; only the `ConfirmDialog` wrapper uses ui-new tokens. New UI inside the shared editor should match its existing slate classes.
- `api_type` whitelist (`config.rs:320-331`) excludes `google`; a Google `ModelConfig` (offered by `Step3Models`) cannot flow through `create_llm_client` for authoring — annotate/disable it in the picker.

---

## 9. Data Model & Schema

**One additive migration** `crates/db/migrations/20260620120000_create_custom_rules.sql` — pure `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX`, **no rebuild, no `PRAGMA foreign_keys` toggle** (the load-bearing safety constraint: sqlx 0.8.6 wraps each migration in an implicit txn where a bare `PRAGMA foreign_keys=OFF` is a silent no-op). **Every project-scoped FK is BLOB** to match `projects.id` BLOB PK (the `project_quality_policy.rs` Uuid pattern — a TEXT child FK against a BLOB parent silently fails, the bug fixed by `20260202090000`).

**Scope note (D4):** `project_id` is kept **nullable** in the schema (`NULL` = global/org rule) so global scope is a later additive step, but the v1 UI/feature treats it as required (`project_id NOT NULL` enforced at the route/handler layer, not the column). No schema change is needed when global scope ships.

### custom_rule
```
id BLOB PRIMARY KEY NOT NULL
project_id BLOB REFERENCES projects(id) ON DELETE CASCADE   -- NULL = global/org rule (schema-allowed; v1 UI requires non-null, D4)
name TEXT NOT NULL
nl_request TEXT NOT NULL                  -- original NL ask (round-trip compare + reproducibility)
rule_format TEXT NOT NULL CHECK (rule_format IN ('ast_grep','regex'))   -- P1 emits 'regex'; 'ast_grep' is P2 (D5)
rule_body TEXT NOT NULL                   -- regex+scope JSON (P1) or ast-grep YAML (P2)
description TEXT                           -- LLM-generated text powering the '!' tooltip
rule_type TEXT NOT NULL DEFAULT 'CodeSmell' CHECK (rule_type IN ('Bug','Vulnerability','CodeSmell','SecurityHotspot'))
severity TEXT NOT NULL DEFAULT 'MAJOR' CHECK (severity IN ('INFO','MINOR','MAJOR','CRITICAL','BLOCKER'))
mapped_metric TEXT                         -- MetricKey::as_str() token; free text, NOT an FK
enabled INTEGER NOT NULL DEFAULT 1
status TEXT NOT NULL DEFAULT 'shadow' CHECK (status IN ('draft','shadow','warn','enforce','disabled'))
created_by TEXT
version INTEGER NOT NULL DEFAULT 1
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
updated_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
UNIQUE(project_id, name)
indexes: idx_custom_rule_project(project_id); idx_custom_rule_enabled(project_id, enabled); idx_custom_rule_metric(mapped_metric)
```

### custom_rule_example (correctness oracle)
```
id BLOB PK NOT NULL
rule_id BLOB NOT NULL REFERENCES custom_rule(id) ON DELETE CASCADE
kind TEXT NOT NULL CHECK (kind IN ('positive','negative'))   -- positive SHOULD flag; negative MUST NOT
language TEXT                                                 -- 'rust','typescript', NULL = agnostic
snippet TEXT NOT NULL
expected_match INTEGER NOT NULL                              -- 1 = rule expected to fire
note TEXT
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
index: idx_custom_rule_example_rule(rule_id, kind)
```

### custom_rule_validation (authoring-time artifact ONLY — do NOT conflate with quality_run/quality_issue)
```
id BLOB PK NOT NULL
rule_id BLOB NOT NULL REFERENCES custom_rule(id) ON DELETE CASCADE
rule_version INTEGER NOT NULL
verdict TEXT NOT NULL CHECK (verdict IN ('pass','fail','error','pending'))
roundtrip_ok INTEGER                                         -- judge verdict on reconstructed-NL vs original (NULL until run)
judge_score REAL                                             -- AuditScoreResult-style total
examples_total INTEGER NOT NULL DEFAULT 0
examples_passed INTEGER NOT NULL DEFAULT 0
rounds_used INTEGER NOT NULL DEFAULT 0
results_json TEXT                                            -- per-example {example_id, expected, actual, matched_spans}; + adversary transcript
error_message TEXT
validated_by TEXT
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
index: idx_custom_rule_validation_rule(rule_id, created_at DESC)
```

### custom_rule_audit (append-only, NEVER UPDATEd → never needs a rebuild; intentionally FK-LESS so history survives rule deletion)
```
id BLOB PK NOT NULL
rule_id BLOB NOT NULL
project_id BLOB
action TEXT NOT NULL CHECK (action IN ('create','update','enable','disable','delete','revalidate','promote'))
actor TEXT
from_version INTEGER
to_version INTEGER
diff_json TEXT
created_at TEXT NOT NULL DEFAULT (datetime('now','subsec'))
index: idx_custom_rule_audit_rule(rule_id, created_at DESC)
```

**Rust models:** new files `crates/db/src/models/custom_rule.rs` (+ `_example`/`_validation`/`_audit`), `#[derive(Debug,Clone,FromRow,Serialize,Deserialize,TS)]` `#[serde(rename_all="camelCase")]`, `Uuid` PK + `DateTime<Utc>` (the `project_quality_policy.rs:15-67` BLOB/Uuid pattern, **not** the `quality_run.rs` String pattern). CRUD: `find_by_project` / `find_enabled_by_project` / `upsert` / `set_enabled` / `delete` + insert helpers for children (the `quality_issue.rs:103-138` `insert_batch` pattern for examples). Register in `models/mod.rs:25-39` (`pub mod` + `pub use`). Optionally add the new NOT-NULL-with-default columns to `SCHEMA_EXPECTATIONS` (`lib.rs:31-86`) for the Windows startup self-heal.

**Future-rebuild note:** if `custom_rule` ever needs a column dropped, use the verified sqlx-escape sandwich (`PRAGMA foreign_keys=OFF; COMMIT; BEGIN; ...12-step rebuild...; PRAGMA foreign_key_check; COMMIT; PRAGMA foreign_keys=ON; BEGIN;`) — never a bare pragma. The audit table being append-only avoids this entirely.

**Unchanged:** `QualityGateConfig` YAML in `project_quality_policy` is a parallel store; the gate condition referencing `custom_rule_critical` still lives in the YAML, but rule bodies/examples/descriptions live in `custom_rule`. Built-in metric tooltip descriptions are static (compiled catalog), not DB.

---

## 10. API Surface

All wrapped in `ApiResponse<T>`, unwrapped by `handleApiResponse`; all DTOs `#[derive(TS)]` → regenerate `shared/types` (the `generate_types --check` gate, `ci-basic.yml:112`). Routes extend `crates/server/src/routes/quality.rs` (mounted `/api/quality` + `/api/projects`, `mod.rs:169-171`).

**Existing (unchanged), reused:**
- `GET /api/quality/policy/default`, `GET /api/quality/policy/metrics` (`quality.rs:277`), `GET/PUT/DELETE /api/projects/{id}/quality-policy`, `POST /api/planning-drafts/{draftId}/confirm-gates` (`planning_drafts.rs:162`).
- **Model-source enumeration (reused for the authoring picker — D1):** `GET /api/cli_types` (`cli_types.rs:131`) and `GET /api/cli_types/{cliTypeId}/models` → `Vec<ModelConfig>` (`cli_types.rs:156`), each row carrying `hasApiKey`/`isOfficial`/`displayName`/`apiType`. The FE picker consumes these via `useModelConfigForExecutor`; **no new enumeration endpoint is added** — the author route simply *accepts the chosen `model_config_id` + `cli_type_id`*.

**Extended — tooltip catalog:**
```
GET /api/quality/policy/metrics
  -> MetricCatalogResponse { metrics: MetricKey[], operators: string[], info: MetricInfo[] }
  MetricInfo { key: MetricKey, displayName: string, description: string, example: string, higherIsWorse: boolean }
```

**New — current value for tooltip (latest persisted run only — D7):**
```
GET /api/projects/{id}/quality-metrics/latest
  -> ProjectMetricSnapshot { values: Record<MetricKey, MeasureValue>, runId: string|null, ranAt: string|null }
  (reads the latest quality_run.report_json; never recomputes on hover)
```

**New — custom-rule CRUD:**
```
GET    /api/projects/{id}/custom-rules                 -> CustomRule[]
POST   /api/projects/{id}/custom-rules                 (CustomRuleInput) -> CustomRule
PUT    /api/projects/{id}/custom-rules/{ruleId}        (CustomRuleInput) -> CustomRule   (D8: body edit -> revalidate+shadow; metadata-only -> bump version + audit)
DELETE /api/projects/{id}/custom-rules/{ruleId}        -> ApiResponse<()>
PATCH  /api/projects/{id}/custom-rules/{ruleId}/status ({status}) -> CustomRule          (shadow->warn->enforce promotion)
GET    /api/projects/{id}/custom-rules/{ruleId}/validations -> CustomRuleValidation[]
```

**New — AI authoring (the heart; dual-source — D1):**
```
POST /api/projects/{id}/custom-rules/author
  Request  AuthorRuleRequest {
    nlRequest,
    modelConfigId: string,                  // user-selected source (from the reused picker)
    cliTypeId: string,                      // maps the source to its CLI roster
    ruleFormatPreference?: 'regex' | 'ast_grep',   // P1 honours 'regex'; 'ast_grep' available in P2 (D5)
    currentRulesContext?: ConditionConfig[]
  }
  Response AuthorRuleResult {
    candidate: CustomRuleDraft { ruleFormat, ruleBody, description, ruleType, severity, mappedMetric },
    examples: RuleExample[],                       // 2-3 positive + 2-3 negative
    empirical: EmpiricalReport { total, passed, perExample: ExampleResult[] },
    debate: AdversaryTranscript { proposerNotes, attackerFindings, revisions },
    roundTrip: RoundTripVerdict { reconstructedRequest, judgePassed, judgeScore, rationale },
    outcome: 'passed' | 'capped_out',
    roundsUsed: number,
    engine: { modelConfigId, backend: 'metered' | 'subscription' }   // echoes which invoker backend ran (D1)
  }
POST /api/projects/{id}/custom-rules/{ruleId}/revalidate -> CustomRuleValidation   (D8: full pipeline; drops to shadow)
```

**Server-side source resolution (the author/revalidate routes — D1):** resolve via `ModelConfig::resolve_preferred_or_default(pool, Some(modelConfigId), cliTypeId)` (`cli_type.rs:387`); pick the invoker backend from `InteractiveAuthMode::resolve(get_api_key()?, base_url)` (`cc_switch.rs:597`): keyed → `create_llm_client` (metered, pool-exempt; `validate()` rejects `google`); no-key/native → the subscription interactive transport (off the credit pool; never sets `SOLODAWN_NO_POOL`). Always pass the **explicit** `modelConfigId` to avoid the `container.rs:1539-1564` mis-billing fallthrough.

**Validation invariants (server-side, before any persist):**
- Any gate **condition** flows through `QualityGateConfig.validate()` (`config.rs:354`): operator ∈ {GT,LT}, parseable threshold, `MetricKey` is a compiled enum variant (serde rejects unknowns at deserialize — a bad metric 400s before validate).
- A custom rule is **refused** persistence (400) unless: `rule_format`/`severity`/`rule_type` pass the CHECK enums; (P1) the regex compiles within `RegexBuilder.size_limit(1<<20).dfa_size_limit(1<<20)`; **and** the empirical fixture run passes (every positive flags ≥1, every negative flags 0). This is the **admission gate**.
- `mapped_metric`, if present, must be a known `MetricKey::as_str()` token.

**ts-rs types to add/regenerate:** `MetricInfo`, `ProjectMetricSnapshot`, `CustomRule`, `CustomRuleInput`, `CustomRuleDraft`, `RuleExample`, `EmpiricalReport`, `ExampleResult`, `AuthorRuleRequest`, `AuthorRuleResult`, `AdversaryTranscript`, `RoundTripVerdict`, `CustomRuleValidation`. **Caveat:** `shared/types` is ts-rs-generated — regenerate, never hand-edit.

---

## 11. UI/UX

Anchor on the **one** shared `QualityGateRulesEditor.tsx` (props `value`/`defaults`/`metricOptions`/`onChange`/`readOnly`/`errors`) consumed by `RulesDialog`, `ConfirmDialog`, `SettingsNew`. **Styling correction (rev2):** the shared editor uses raw `text-slate-*`/`bg-white` Tailwind, **not** `.new-design` tokens — new UI *inside the editor* should match its existing slate classes; only the `ConfirmDialog` wrapper uses ui-new/.new-design tokens.

1. **Tooltip (D7)** — add optional `metricInfo?: MetricInfo[]` threaded like `metricOptions`. Render a circled-"!" button (reuse the imported lucide `Info`/`AlertTriangle`) next to the Metric `<select>` (lines 174-194). Popover shows displayName, description, example, and the current project value **as of `<ranAt>`** (from `GET /api/projects/{id}/quality-metrics/latest`, nullable → "no run yet"); it never triggers a recompute. For custom rules, show the stored `description`.
2. **Generate-rule affordance with the reused model picker (D1)** — per-gate header "Generate rule with AI" button (lines 142-154) opens `RuleAuthoringDialog.tsx`:
   - **Model picker:** lift the `CreateChatBox.tsx` `ToolbarDropdown` (lines 143-201) fed by `useModelConfigForExecutor` output (`useModelConfigForExecutor.ts:56`) — Custom/Official sections, CheckIcon on the selected id, `displayName` + `subtitle`. Bind `selectedModelConfigId`/`setSelectedModelConfigId`; submit the chosen `model_config_id` + `cli_type_id`. Annotate/disable `google`-`apiType` rows (rejected by `validate()`); the no-key native row routes to the subscription backend.
   - NL textarea; sends `nlRequest` + `currentRulesContext` (live `value` conditions) + the selected source to the author route.
   - Running stepper: Proposer → Adversary → Empirical → Judge (and a "subscription" vs "metered" badge from `AuthorRuleResult.engine.backend`).
   - Result renders three panels: (a) candidate rule + plain-language description + "!" tooltip; (b) empirical evidence table (positive/negative/false-positive snippets, flag/no-flag actual vs expected, red on mismatch); (c) adversarial debate transcript. If `outcome=capped_out`, a banner "couldn't converge — edit manually" with the best candidate prefilled (D6 hands back to the user).
   - Primary action "Confirm & add" → persists the `custom_rule` (status shadow) **and**, when mapped to a gating metric, splices a `ConditionConfig` into the editor via the existing `addCondition`/`updateCondition` helpers (lines 73-90). Then Steps 6/7 run; a "Round-trip check" badge shows pass/fail with the reconstructed request. On fail, the dialog returns to the edit step (the loop) rather than closing.
   - New `useGenerateRule` mutation + `useCustomRules` query in `useQualityPolicy.ts` (`qualityPolicyKeys` gains a `customRules:(projectId)` key), following the existing `makeRequest`+`handleApiResponse` pattern (`lib/api.ts` `qualityPolicyApi` lines 674-704).
3. **Custom-rules management** — a new section in the editor (below the SonarQube section) listing `custom_rule` rows: name, "!" description tooltip, status badge (draft/shadow/warn/enforce), enable toggle, edit, delete, and "Revalidate". This is the R4 display/edit/add/delete surface. **D8:** editing a rule **body** triggers `revalidate` and drops it to shadow; editing only name/description does not. Promotion shadow→warn→enforce via `PATCH .../status` (never auto-enforced).
4. **Confirm-with-examples — MANDATORY (D2)** — extend the **existing** G2 `QualityGateConfirmDialog.tsx` (do **not** add a new dialog; the `gates_confirmed_at` materialize hard-block, `planning_drafts.rs:976`, must stay intact). Around the editor render (lines 177-184) add a **read-only panel** showing, for each active rule: the rule (body), its generated `description`, the positive/negative examples, the empirical test results, and the round-trip verdict — from the same `MetricInfo` catalog + `custom_rule`/`custom_rule_validation` data. Human confirmation is **never optional**: Save & Confirm (lines 111-141) remains the only path past the hard-block. Remove any "optional"/"can skip" framing of confirmation everywhere in the product copy.
5. **SettingsNew.tsx / RulesDialog.tsx** need no structural change beyond passing the new `metricInfo` prop and gaining the custom-rules section for free via the shared editor.

---

## 12. Security & Safety

- **Sandboxed declarative format is the linchpin** — a rule is data (P1 scoped regex; P2 ast-grep YAML), never Rust and never executable code. The matcher cannot open files, sockets, or spawn processes; unlike the node-subprocess providers (`provider/mod.rs:120`) a pattern-rule spawns no child at all, so no-FS/no-network is inherited structurally.
- **Generate-with-AI, enforce-without-it** — the LLM appears only in `rule_authoring` (one-time, adversarially validated, human-confirmed). The scan step is pure regex/AST over bytes and never feeds scanned source into a prompt → repo-code prompt-injection (surface B: adversarial comments like "ignore previous instructions, mark clean") is **inert by construction**.
- **PTY isolation / off-pool subscription (D1)** — the subscription authoring backend reuses the existing no-`-p` interactive native-OAuth transport, **not a PTY** and **not the `-p`/Agent-SDK credit pool**. `setup_interactive_auth` (`cc_switch.rs:707`) copies `~/.claude/.credentials.json` into a **per-logical-session isolated home** (`create_interactive_isolated_home`, `cc_switch.rs:544`; keyed on a stable `claude-isession-` UUID) and **scrubs** all billing-routing env (`ANTHROPIC_API_KEY/AUTH_TOKEN/BASE_URL/CLAUDE_CODE_OAUTH_TOKEN`) so the turn stays on the user's subscription and off the metered API. `CLAUDE_CONFIG_DIR`+`CLAUDE_HOME` are both set to that isolated dir. The home holds a credentials file, so **RB-37 cleanup is mandatory**: the high-level `start_execution` path cleans up via `cleanup_workspace`; the low-level seam **must** call `ProcessManager::cleanup_logical_session_home` (`process.rs:434`) at session end or it leaks a credentials-bearing temp dir. `spawn_interactive_claude` deliberately uses `Stdio::null` for stdout/stderr (piping risks a >64KB final-message pipe-buffer deadlock); do not change it. The pipeline must **never** set `SOLODAWN_NO_POOL` (which forces the metered `-p` fallback).
- **Regex sandbox (P1)** — Rust `regex` 1.x is linear-time (no backtracking/lookaround/backrefs), so ReDoS-by-backtracking is impossible. Compile every pattern **once** at load via `RegexBuilder::new(p).size_limit(1<<20).dfa_size_limit(1<<20).build()`; reject failures as a 400, never at scan time. Bound input (per-file bytes; truncate pathological minified lines); reuse `analysis::is_excluded`.
- **Fail-closed timeout** — wrap `analyze()` in `tokio::time::timeout`; on timeout in Enforce mode emit a Blocker (`engine.rs:317` parity). Preserve the `syn`/tree-sitter parse-error skip (`builtin_rust.rs:77`). A provider `Err` already degrades to a failure report — return `Ok`+sentinel when "no rules ran" should be benign so an empty rule set doesn't hard-block.
- **Prompt-injection (surface A — NL → rule)** — the model returns only DATA conforming to a strict schema, validated by serde + `RegexBuilder` compile + the empirical fixture run **before** persistence. The model **cannot** mint a self-escalating Blocker: `AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig` (`rule.rs:205`) caps custom severities at `Major` (D3); gating is opt-in via the explicit `CustomRuleCritical` count metric only. Persist the rule (data), not the freeform prompt, so re-runs are reproducible. **Surface A applies to BOTH invoker backends** — a metered key and the subscription transport both feed the same strict-schema + admission-gate, so neither backend widens the injection surface.
- **Key handling (D1)** — reuse the user's already-configured credentials via the model `get_api_key()` helpers (`cli_type.rs:186` / `workflow.rs:206`); never read `encrypted_api_key`/`orchestrator_api_key` directly (decryption is automatic, AES-256-GCM). For metered sources build `OrchestratorConfig` (validate() whitelists `api_type`) and go through `create_llm_client` (metered, pool-exempt). For the subscription source no key env is set at all (native-OAuth). Always pass the **explicit** `model_config_id` so the `container.rs:1539-1564` precedence fallthrough cannot mis-bill a subscription user. Never store any key in `system_settings` (plaintext K/V).
- **Mandatory human confirmation (D2)** — no AI-authored rule is persisted or promoted without the user passing the mandatory G2 confirm dialog; the `gates_confirmed_at` hard-block (`planning_drafts.rs:976`) remains the only path to materialize. There is no "optional confirm" code path.
- **Admission gate** — refuse to persist any rule that fails its own positive/negative fixtures **or** that flags a curated known-clean corpus (catches the dominant failure mode — over-broad LLM regexes that would false-positive the whole repo).

---

## 13. Testing Strategy

- **Inline unit tests gate CI.** The quality crate has **no `tests/` directory** — every quality test is inline `#[cfg(test)]`, so `cargo nextest run --workspace ... --lib` (the `ci-basic.yml:109` backend gate) runs them. Put **inline** in `crates/quality`: rule-compile, regex `size_limit` rejection, timeout-fail-closed, `severity_origin`/`cap_for_advisory` for `CustomRule`, the updated pinned `severity_origin` tests (`rule.rs:300-358`), and the **empirical positive/negative harness** (mirroring `secret_detection.rs:209` `analyze_text` — in-memory, deterministic, no IO/clock/network).
- **The empirical harness as admission gate.** `run_candidate(compiled_rule, snippet, virtual_path)` asserts each positive yields an issue and each negative yields none. This both gates acceptance of a new rule and regression-locks it. Catches the over-matching failure mode that is the #1 risk of LLM-generated regexes (e.g. flagging inline `#[cfg(test)]`).
- **Authoring-pipeline tests.** Use `MockLLMClient` (`llm.rs:100-155`) to unit-test each stage (`generate`/`adversary`/`empirical_test`/`judge`/`reverse_engineer`) and the loop/cap deterministically — verify fail-closed defaults, that a failing empirical report forces `passed=false`, and that the cap returns `capped_out` at 4 rounds (not panic). **Because both invoker backends resolve to `&dyn LLMClient`, the pipeline tests cover both backends with one mock** — no transport mock is needed (the metered/subscription split is exercised separately at the `build_authoring_client` dispatch boundary).
- **Invoker dispatch test.** Unit-test `build_authoring_client` (§8.6): a keyed `ModelConfig` selects the metered backend; a no-key/native config selects the subscription backend; a `google`-`apiType` config is rejected (`validate()`); an explicit id never falls through to `find_with_credentials_for_cli` mis-billing. The subscription transport itself is already covered end-to-end by `crates/local-deployment/tests/interactive_transport_smoke.rs` (reuse, do not duplicate).
- **Integration tests run under nextest, NOT `cargo test`.** DB persistence (custom-rule CRUD via the new tables), resolver loading enabled rules, edit-revalidation drop-to-shadow (D8), and full-engine runs go in `crates/services/tests` / `crates/server/tests`, mirroring `quality_policy_resolver_test.rs` (in-memory `SqlitePool` + `sqlx::migrate!("../db/migrations")`) and `quality_gates_test.rs` (axum `oneshot`). Tests touching `DeploymentImpl` **must** run under nextest (per-process isolation) + `#[serial]` — never `cargo test` (the migration race fixed in `6facb481a`: every `DeploymentImpl::new()` opens the same `db.sqlite`).
- **CI coverage gap (flagged).** The `--lib` gate runs inline tests but **not** `crates/*/tests` integration tests. Put the empirical harness + compile/timeout/severity-cap tests **inline** (gates automatically); for DB/end-to-end coverage either add inline where feasible or add a separate nextest step **without** `--lib`.
- **CI hygiene.** Every new `#[derive(TS)]` DTO and the new `MetricKey` variants need regenerated `shared/types` committed or `generate_types --check` fails (`ci-basic.yml:112`); clippy runs `--all-targets --all-features` (`:90`) so test fixtures must be clippy-clean. Verify CASCADE on `custom_rule_example`/`_validation` actually fires with an in-memory test (runtime connections set `PRAGMA foreign_keys=ON`, `lib.rs:244/291`).

---

## 14. Rollout & Phasing

**P0 — Tooltips (D7)** (pure frontend + static catalog, zero engine risk, ships first):
- Enrich `MetricCatalogResponse` with `MetricInfo[]` (static description/example table per selectable `MetricKey`, `quality.rs:207`); add `GET /api/projects/{id}/quality-metrics/latest` (reads the **latest** existing `quality_run.report_json`; never recomputes — D7).
- Add `metricInfo` prop + circled-"!" popover; thread through `useQualityMetricKeys`. Regenerate ts-rs (`MetricInfo`, `ProjectMetricSnapshot`). No DB migration. Directly solves "I can't understand it."

**P1 — Scoped-regex declarative format + deterministic enforcement (D5: regex ONLY, zero new deps)** (the engine half; no AI yet):
- **No new Cargo deps** — reuse `regex = "1"` (`Cargo.toml:25`). Implement `provider/declarative.rs` (the `console_usage.rs` template, but `new_capped`+`CustomRule`) + shared `run_candidate` + the inline empirical harness. **ast-grep is NOT added in P1.**
- Ship the **scoped-regex** `rule_body` format and emit `rule_format='regex'` from the authoring path; keep the `rule_format` discriminant so `ast_grep` can be added in P2 without migration.
- Add `AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig` (update exhaustive match + pinned tests `rule.rs:300-358`; D3); add `MetricKey::{CustomRuleViolations, CustomRuleCritical}` (3 coupled sites + `selectable_metric_keys` + ts-rs regen). Add `ProvidersConfig.declarative_rules=false` + `build_providers` registration with `applicable_metrics` empty-when-no-rules.
- Add the 4 `custom_rule` tables (migration `20260620120000`) + db models + CRUD routes + the custom-rules management section. `resolve_quality_config` loads enabled rules and constructs the provider with size-limited compile + tokio timeout. **Scope is project-only in the UI (D4)** though the column stays nullable.
- **Rollout via `QualityGateMode`/per-rule `status`:** rules land at `shadow` (run + record, never block — the proven engine shadow path), promote per-project shadow→warn→enforce; enforce allowed **only** after fixtures + clean-corpus pass; severity stays capped. Per-project policy is re-read every run, so promotion needs no redeploy.

**P2 — NL generation + adversarial validation + ast-grep structural rules (D1, D5)** (the AI half + the structural-format upgrade, last, behind the now-stable enforcement):
- Build `crates/services/src/services/rule_authoring/` + the **dual-source authoring-model invoker** (§8.6) reusing `create_llm_client` (metered) **and** the already-built subscription interactive transport (`create_interactive_claude_client`/`ContainerService::start_execution`) — **no new transport, no PTY**. Reuse `AuditScoreResult` + the bounded-loop cap (`MAX_AUTHORING_ROUNDS=4`, D6); unit-test every stage + the loop + the dispatch with `MockLLMClient`.
- Add `POST .../custom-rules/author` (accepts the selected `modelConfigId`+`cliTypeId`) + `/revalidate` (D8: full pipeline → shadow). Add `RuleAuthoringDialog.tsx` lifting the `CreateChatBox` model picker + `useModelConfigForExecutor` (D1). Wire the context-free reverse-engineer (Step 6) + judge-compare (Step 7).
- Extend the G2 `QualityGateConfirmDialog` with the **mandatory** read-only rule/examples/empirical/round-trip panel (D2).
- **ast-grep structural format:** after **confirming the unverified ast-grep linter-envelope field names** (§17), add the ast-grep MIT crates and the `ast_grep` loader behind the existing `rule_format` discriminant — additive, no migration. This is the P2 expressiveness upgrade (e.g. `.unwrap()` in handlers), not a P1 dependency.

Each phase is independently shippable: P0 delivers immediate UX value; P1 is usable manually (regex rules) without any AI and without ast-grep; P2 layers AI authoring (dual-source) and structural rules on a proven deterministic substrate. A `ProvidersConfig.declarative_rules=false` default + optional `SOLODAWN_NO_*` env kill-switch keeps the feature dark until ready. (Note: never set `SOLODAWN_NO_POOL` from the authoring path — it forces the metered `-p` fallback for the subscription backend.)

---

## 15. Risks & Mitigations

**RISK 1 — "AI builds the AI guardrail" (the central tension).** Using an LLM to author the rule meant to catch LLM mistakes risks inheriting the same blind spots. **Resolution (designed-in):** (a) generate-with-AI / enforce-without-it — AI touches authoring only; the confirmed rule is deterministic data run identically forever; (b) the empirical test is **deterministic ground truth** — it cannot be talked out of a behavioral failure; (c) the reverse-engineer agent is deliberately **context-free**, a genuine independent check; (d) `CustomRule` severity is capped to `Major` (D3) so a bad rule cannot self-escalate to Blocker; (e) **mandatory** human confirm (D2) + per-project shadow→enforce means no AI-authored rule blocks delivery until a person promotes it.

**RISK 2 — Two agreeable LLMs converge on a wrong answer.** **Resolution:** Step 2 is explicitly **adversarial** (distinct attacker prompt producing false-positives/evasions/ambiguity); its snippets become permanent fixtures feeding the deterministic Step 3 — disagreement is forced and checked against ground truth.

**RISK 3 — Over-broad generated regex false-positives the whole repo** (the real `test_file_absence`/inline-`#[cfg(test)]` bug class). **Resolution:** the admission gate refuses persistence unless negative + adversary false-positive fixtures pass **and** a curated clean-corpus stays clean; default Shadow mode collects the real false-positive rate before any gating.

**RISK 4 — DFA memory blowup / pathological input.** **Resolution:** `size_limit`+`dfa_size_limit` at one-time compile, per-file/line input caps, tokio timeout fail-closed.

**RISK 5 — ast-grep footprint + unverified schema fields (P2 only — D5).** **Resolution:** P1 ships the zero-new-dep scoped regex (`rule_format='regex'`), so the whole feature is usable without ast-grep; the `rule_format` discriminant lets `ast_grep` arrive additively in P2. **Before coding the P2 loader, confirm the ast-grep YAML linter-envelope field names** against the pinned crate version (the one remaining technical open item, §17). The fallback regex schema is fully viable if AST deps are ever rejected.

**RISK 6 — CI coverage gap** (`ci-basic.yml` backend gate is nextest `--lib`, which runs inline tests but not `crates/*/tests`). **Resolution:** put the empirical harness + compile/timeout/severity-cap tests **inline**; add inline DB coverage where feasible or a separate nextest step without `--lib`. `DeploymentImpl` integration tests must run under nextest + `#[serial]`, never `cargo test`.

**RISK 7 — ts-rs drift.** Every new `#[derive(TS)]` DTO and the new `MetricKey` variants need regenerated `shared/types` committed or `generate_types --check` fails; clippy `--all-targets` means test fixtures must be clippy-clean.

**RISK 8 — Cost surprise (metered backend only — D1).** Authoring on a **metered** source runs up to `MAX_AUTHORING_ROUNDS × several LLM calls` on the user's key. **Resolution:** cap rounds at 4 (D6), show token usage (`LLMResponse.usage`), and surface the per-run backend (`AuthorRuleResult.engine.backend`). The **subscription** backend (native-OAuth) has **zero** metered/credit-pool cost; selecting it is the free path. Surface "no usable credential for the selected source" rather than silently switching backends.

**RISK 9 — Subscription-backend mis-billing / credential leak (D1).** A fallthrough resolve (`config_id=None`) into `find_with_credentials_for_cli` could route a subscription user who also has a saved key onto metered billing (`container.rs:1539-1564`); and the low-level interactive seam holds a copied `.credentials.json`. **Resolution:** always submit the user's **explicit** `model_config_id`; never set `SOLODAWN_NO_POOL`; and on the low-level path **always** call `ProcessManager::cleanup_logical_session_home` (`process.rs:434`) at session end (the high-level `start_execution` path does this via `cleanup_workspace`).

**RISK 10 — Subscription-PTY/interactive concurrency (open — §17).** The interactive transport spawns a real `claude` child per turn against a per-logical-session isolated home; the safe ceiling on **concurrent** authoring turns through the subscription backend is not yet set. **Resolution (to decide):** cap concurrent subscription-authoring runs (e.g. a small semaphore) so parallel authoring requests don't spawn unbounded `claude` children; metered runs are HTTP and not subject to this.

---

## 16. Success Metrics

- **Comprehension:** ≥90% of selectable metrics show a non-empty description + example in the editor; "!" popover renders the current project value as-of-`<ranAt>` (or "no run yet") for every metric — without any recompute (D7).
- **Authoring yield:** ≥70% of NL requests produce a candidate that passes the empirical fixture run within `MAX_AUTHORING_ROUNDS` (=4); `capped_out` results always hand back a prefilled best candidate (never a dead end). Holds across **both** invoker backends (D1).
- **Round-trip integrity:** the context-free Matcher rejects ≥X% of deliberately mismatched candidates in test fixtures (calibrate during P2).
- **Enforcement safety:** zero AI-authored rule reaches Enforce without passing its fixtures + clean-corpus check **and** the mandatory human confirm (D2); default Shadow false-positive rate is observable per project before promotion.
- **Determinism:** the same confirmed rule produces byte-identical findings across repeated gate runs (regression-locked by the inline harness).
- **Cost integrity:** subscription-backed authoring runs draw **zero** metered/credit-pool cost; metered runs report token usage; no silent backend switch (D1).
- **Non-breakage:** with `declarative_rules=false` (default), existing gate behavior is bit-identical to pre-feature (verified by the existing engine tests staying green).

---

## 17. Open Questions for the User

The owner's decisions D1–D9 are now applied. The items below record each decision as RESOLVED, then list the only items that remain genuinely open.

### Resolved (decisions applied in rev2)
- **D1 — Authoring engine (was OQ-3): RESOLVED.** The engine is **ALL globally-configured LLM sources, user-selectable**. One **authoring-model invoker** (§8.6) with two backends, both reusing existing infra: (a) metered API-key sources → existing `create_llm_client` (`llm.rs:922`); (b) official-subscription / native-OAuth sources → the existing no-`-p` interactive transport driving the genuine `claude` binary off the credit pool (`create_interactive_claude_client` `llm.rs:868` / `ContainerService::start_execution` `container.rs:2108`; **not a PTY**). The user picks the source via the reused `useModelConfigForExecutor` + `CreateChatBox` dropdown. No new transport, no new picker, no new enumeration.
- **D2 — Confirm dialog (was OQ-7): RESOLVED — MANDATORY.** The secondary-verification confirm is **never optional**. Extend the existing G2 `QualityGateConfirmDialog` with a read-only panel (rule, generated description, positive/negative examples, empirical results, round-trip verdict); preserve the `gates_confirmed_at` hard-block (`planning_drafts.rs:976`). All "optional confirm" framing removed.
- **D3 — Severity (was OQ-1): RESOLVED — advisory by default.** `AnalyzerSource::CustomRule → SeverityOrigin::ProjectConfig`, capped to `Major` by `cap_for_advisory` (`rule.rs:124`); gating **only** via the opt-in `CustomRuleCritical` count metric, never a self-declared `Blocker`. Reuse the existing cap + metric-condition machinery.
- **D4 — Scope (was OQ-2): RESOLVED — project-scoped first.** `project_id` is required in the v1 UI/feature (enforced at the route layer) while the column stays **nullable** so global/org scope is a later additive step with no migration.
- **D5 — Rule format phasing (was OQ-4): RESOLVED — regex-first, ast-grep later.** P1 ships the **scoped-regex format only** (zero new deps; `regex = "1"` `Cargo.toml:25`; `console_usage.rs` template). P2 **adds** the ast-grep AST format (MIT crates) behind the same `rule_format` discriminant, **after** confirming the unverified ast-grep schema field names (see still-open below).
- **D6 — Authoring round cap (was OQ-5): RESOLVED — fixed 4.** `const MAX_AUTHORING_ROUNDS = 4` (mirrors `FINAL_REPAIR_MAX_ROUNDS` `agent.rs:115`); `capped_out` hands back to the user (no panic). Configurability deferred.
- **D7 — Tooltip current value (was OQ-6): RESOLVED — latest persisted run.** Read the latest `quality_run.report_json`, labelled "as of `<ranAt>`"; **no** fresh recompute on hover.
- **D8 — Edit-revalidation (was OQ-8): RESOLVED.** Editing a rule **body** re-runs the full authoring validation and drops the rule back to **shadow**; metadata-only edits (name/description) skip re-validation (bump version + audit only). Same pipeline reused.
- **D9 — Reuse over rewrite (cross-cutting): RESOLVED.** Added the **Reuse Map** section (existing-reuse vs new-glue) and flagged everything the prior draft proposed building that already exists (chiefly the subscription interactive runner, the model enumeration/picker, the regex dep, the ts-rs harness, and the G2 confirm dialog). Net-new code is minimised to: `rule_authoring/` + invoker, `provider/declarative.rs` + `run_candidate`, four DB tables + models, three enum-edit sites, and the frontend wiring.

### Still genuinely open
1. **ast-grep linter-envelope field names (P2 blocker — D5/RISK 5).** Confirm the exact ast-grep YAML field names (`id`/`message`/`severity`/`note`/`constraints`/`utils`, and the pinned crate version) **before** coding the P2 `ast_grep` loader. The matching categories are confirmed; only the envelope is `[unverified]`. P1 (regex) does not depend on this.
2. **Subscription-backend concurrency limit (D1/RISK 10).** Set the safe ceiling on **concurrent** authoring turns through the subscription native-OAuth transport (each spawns a real `claude` child against an isolated home). Proposed default: a small semaphore (e.g. 1–2 concurrent subscription-authoring runs); metered HTTP runs are exempt. Confirm the number and whether it is global or per-user/per-project.
