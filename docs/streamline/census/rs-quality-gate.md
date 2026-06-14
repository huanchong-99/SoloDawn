# Census: rs-quality-gate (`crates/quality/src/gate/`)

Unit: the three-gate evaluation model (System A). Ported from SonarQube's qualitygate module.
Branch: refactor/streamline-and-quality-gate-rules. Files covered: 5/5.

## Module map

| File | Purpose | Public surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `gate/mod.rs` | Quality-gate definition + aggregate decision + the three-gate level enum. Aggregates per-condition `EvaluationResult`s into one `QualityGateStatus`. | `QualityGate{id,name,conditions}` + `new`, `with_id`, `evaluate`; `QualityGateDecision{gate_id,gate_name,status,condition_results}` + `is_passed`, `is_blocked`, `failed_conditions`; `enum QualityGateLevel{Terminal,Branch,Repo}` (+Display) | `QualityGate::new` called by `config.rs:327` (`to_gate`). `evaluate()` called by `engine.rs:389`. `QualityGateDecision` stored in `report.rs` (`with_decision`), serialized to DB (`db/models/quality_run.rs`). `is_passed()` consumed by `report.rs:76`, `container.rs:255`, `agent.rs:2835/7981`. `QualityGateLevel::{Terminal,Branch,Repo}` all live (agent.rs:2820/7810/7826, container.rs:249). | `with_id` has ZERO callers (prod builds via `new`). `is_blocked`/`failed_conditions` used only by this file's own tests. `QualityGateStatus::Warn` branch (l.63-64) is unreachable — see below. |
| `gate/condition.rs` | Condition model (Metric+Operator+Threshold) and `Operator` enum with DB string mapping. | `enum Operator{GreaterThan,LessThan}` + `from_db_value`, `to_db_value`, `is_triggered`, Display; `Condition{metric,operator,error_threshold,use_variation}` + `new`, `parse_threshold_f64`, `parse_threshold_i64`, `description`; `PartialEq/Eq/Hash` by metric only. | `Operator::from_db_value` called by `config.rs:78` (`ConditionConfig::to_condition`). `Condition::new` called by `config.rs:79`. `is_triggered` called by `evaluator.rs:87`. `Operator`/`Condition` Display used by evaluator error messages. | `to_db_value`, `description`, `parse_threshold_f64`, `parse_threshold_i64` have NO production callers (the latter two only in this file's tests; evaluator uses its own private `parse_threshold`). |
| `gate/evaluator.rs` | Stateless condition evaluator: compares a measure against a threshold and produces an `EvaluationResult`. Implements fail-closed for missing metric and the `-1` sentinel (G33-001). | `struct ConditionEvaluator` + `evaluate`, `evaluate_all`; private `parse_threshold`. | `evaluate_all` called by `engine.rs:283` (sole production caller). Uses `Condition`, `EvaluationResult`, `MeasureValue`, `MetricKey`. | Core live logic. `parse_threshold` is a private duplicate of `Condition::parse_threshold_*` — the public condition variants are the dead twins, not this one. |
| `gate/result.rs` | Unified comparable value type `MeasureValue` + per-condition `EvaluationResult`. | `enum MeasureValue{Int,Float,String,None}` + `compare`, `From<i64/i32/f64/String>`, Display; `EvaluationResult{level,metric,value,message}` + `ok`, `warn`, `error`, `error_with_message`. | `EvaluationResult::{ok,error_with_message}` heavily used by `evaluator.rs` + `engine.rs`. `MeasureValue` built by all providers (e.g. `sonar.rs:290` String, providers Int/Float). `compare` used by evaluator. | `EvaluationResult::warn` is the ONLY constructor of `Level::Warn` and has ZERO callers — making the entire Warn path dead. `EvaluationResult::error` (no-message) only used in mod.rs tests. `MeasureValue::None` variant only printed in Display, never constructed (providers pass `Option<MeasureValue>` and use `None` the Option, not this variant). |
| `gate/status.rs` | Two small status enums: aggregate `QualityGateStatus{Ok,Warn,Error}` and per-condition `Level{Ok,Warn,Error}`. | `enum QualityGateStatus` (+Display), `enum Level` (+Display). | `QualityGateStatus` matched in `report.rs:116`, `agent.rs:2829/7936`. `Level::Error`/`Ok` drive `evaluate` aggregation and `failed_conditions`. | `Warn` variant of both enums is structurally reserved but never produced in prod (see invisible-features). DB layer (`quality_run.rs`, `quality_policy_snapshot.rs`) has its own `shadow|warn|enforce|off` mode string — distinct from this `Level::Warn`. |

## Key relationship summary

- Construction path: `quality-gate.yaml` -> `config.rs` (`ConditionConfig::to_condition` -> `Operator::from_db_value`, `Condition::new`; `to_gate` -> `QualityGate::new`).
- Evaluation path: `engine.rs::run` -> `ConditionEvaluator::evaluate_all` -> `QualityGate::evaluate` -> `QualityGateDecision` -> `report.with_decision` -> serialized to DB + consumed by orchestrator `agent.rs` (promote/block).
- The three-gate System A (`QualityGateLevel::{Terminal,Branch,Repo}`) is fully wired: Terminal at checkpoint (agent.rs:2820, container.rs:249), Branch (agent.rs:7810), Repo (agent.rs:7826).

## Candidates (see JSON for structured form)

- `mod.rs:46-56` `QualityGate::with_id` — dead (0 callers; prod uses `new`).
- `condition.rs:36-41` `Operator::to_db_value` — dead (only `from_db_value` is used).
- `condition.rs:97-121` `parse_threshold_f64`/`parse_threshold_i64` — redundant with evaluator's private `parse_threshold`; only test callers.
- `condition.rs:124-126` `Condition::description` — dead (0 callers).
- `result.rs:101-113` `EvaluationResult::warn` — dead; sole producer of `Level::Warn`, never called.
- `mod.rs:91-110` `is_blocked`/`failed_conditions` — only self-test callers.
- `result.rs` `MeasureValue::None` variant — never constructed.

## Invisible features

- **Reserved Warn / shadow path (gate-level)**: `QualityGateStatus::Warn` + `Level::Warn` + `EvaluationResult::warn` form a non-blocking warning mechanism described in `status.rs:4` as "for shadow/warn mode". In practice shadow/warn behavior is implemented at the ORCHESTRATOR level (mode string `off|shadow|warn|enforce` in `config.rs` / `agent.rs`), NOT via gate-level `Level::Warn`. So this gate-level Warn machinery is a parallel, currently-unused mechanism. Removing `EvaluationResult::warn` would make the Warn enum variants formally unreachable but they are still matched for display robustness.
- **Fail-closed sentinels** (`evaluator.rs:39-70`): missing metric -> ERROR, and `MeasureValue::Int(-1)` "not collected" sentinel (G33-001) -> ERROR. Not user-visible but load-bearing safety behavior; KEEP.
- **`use_variation` flag** (`condition.rs:81/92`): auto-enabled when metric key starts with `new_` (SonarQube new-code semantics). Set but never READ by the evaluator — the "only evaluate changed code" behavior is actually handled by `QualityScope` in `engine.rs`. Reserved/stub field.
