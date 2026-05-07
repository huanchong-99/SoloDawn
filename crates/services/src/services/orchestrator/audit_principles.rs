//! Built-in audit scoring principles.
//!
//! Contains the complete 100-point scoring rubric used as the default
//! audit plan when no user-uploaded document is provided.

use chrono::Utc;

use super::types::{AuditDimensionSpec, AuditMode, AuditPlan};

/// Complete built-in scoring rubric (100-point scale, pass threshold: 90).
///
/// This text is embedded verbatim into LLM prompts during acceptance review.
/// It is derived from V1.0-Acceptance-Criteria.md and covers all five
/// evaluation dimensions plus three veto rules.
pub const BUILTIN_AUDIT_PRINCIPLES: &str = r#"# Built-in Audit Scoring Principles (100-point scale, pass threshold: 90)

## Veto Rules (any trigger = 0 points for entire delivery)
1. Repository is empty (only initial commit) → 0 points
2. Over 50% of source files are stubs/TODO/placeholder → 0 points
3. Code is a verbatim copy of an existing open-source project (not built per requirements) → 0 points

## Dimension 1: Buildability (20 points)

Evaluation steps:
  1. Identify project type (check package.json / Cargo.toml / go.mod / requirements.txt)
  2. Install dependencies (npm install / cargo check / pip install / go build)
  3. Execute build (npm run build / cargo build / pnpm build)
  4. Count errors and warnings in output

Scoring rubric:
  20: Dependencies install + build commands produce zero errors
  15: Warnings present but no errors
  10: A few errors but core modules still build
   5: Many build errors, only partial files compile
   0: Completely unbuildable, or project files incomplete

## Dimension 2: Functional Completeness (25 points)

Evaluation steps:
  1. Read the task requirements (from the confirmed spec)
  2. Check each listed feature for corresponding code implementation
     - Search for key files and directories
     - Read core files, confirm they are not stubs
     - Judge whether implementation is complete (real logic, not TODO/placeholder)
  3. Calculate: implemented features / total required features = completion rate

Scoring rubric:
  25: 100% completion rate, all features have substantive implementation
  20: Completion rate ≥ 80%
  15: Completion rate ≥ 60%
  10: Completion rate ≥ 40%
   5: Completion rate < 40%
   0: Essentially no implementation

## Dimension 3: Code Quality (30 points)

### 3a. Architecture (10 points)
  - Is the directory structure clear with proper separation of concerns?
  - Is there reasonable layering (MVC, Clean Architecture, etc.)?
  - Is inter-module coupling reasonable?
  - No duplicate implementation of existing mechanisms

### 3b. Standards (10 points)
  - Naming is consistent and meaningful (variables, functions, files)
  - Proper error handling (try/catch, Result types, etc.)
  - No obvious code smells (overly long functions, deep nesting, duplicate code, hardcoding)
  - Follows idiomatic patterns for the language in use
  - Types used properly (no untyped escape hatches like excessive `any`)

### 3c. Security (10 points)
  - No obvious vulnerabilities (hardcoded secrets, SQL injection, XSS, unvalidated input)
  - No high-severity dependency vulnerabilities
  - No hardcoded passwords or API keys in source (except test fixtures)
  - Input validation on all external interfaces
  - Environment variables documented in .env.example, not leaked in code

## Dimension 4: Test Quality (15 points)

Evaluation steps:
  1. Search for test files (*.test.*, *.spec.*, *_test.*, test_*)
  2. Count test files
  3. Read at least 2 test files, evaluate test quality
  4. Execute tests if possible
  5. Check coverage reports if available

Scoring rubric:
  15: Comprehensive test suite (unit + integration), tests run and pass, coverage ≥ 60%
  12: Tests exist and run, coverage ≥ 40% or reasonable test count
   9: Test files exist but some cannot run, or insufficient coverage
   5: Few tests of poor quality (empty tests, smoke tests only)
   2: Test files exist but all are stubs or cannot run
   0: No tests at all

## Dimension 5: Engineering & Documentation (10 points)

Evaluation steps:
  1. Check README.md (project description, setup steps, API docs)
  2. Check configuration completeness (eslint, prettier, tsconfig, Cargo.toml, Docker, .github/)
  3. Check deployment config (Dockerfile, docker-compose)
  4. Check CI config (.github/workflows/)
  5. Check .gitignore is reasonable

Scoring rubric:
  10: Complete README (project description + setup + API docs), CI config, Docker config, code style config
   8: README exists with setup steps, has Docker OR CI
   5: README exists but thin, missing deployment/CI config
   2: Only default README
   0: No README"#;

/// Construct the default `AuditPlan` from the built-in principles.
///
/// This is used as the fallback when LLM-based plan generation fails, and
/// as the base for Mode A (built-in only) audit plans.
pub fn default_audit_plan() -> AuditPlan {
    AuditPlan {
        mode: AuditMode::Builtin,
        dimensions: builtin_dimension_specs(),
        pass_threshold: 90.0,
        generated_at: Utc::now().to_rfc3339(),
        raw_principles: BUILTIN_AUDIT_PRINCIPLES.to_string(),
    }
}

/// Build the structured dimension specs corresponding to `BUILTIN_AUDIT_PRINCIPLES`.
fn builtin_dimension_specs() -> Vec<AuditDimensionSpec> {
    vec![
        AuditDimensionSpec {
            name: "buildability".to_string(),
            name_zh: "可构建性".to_string(),
            max_score: 20.0,
            criteria: vec![
                "Dependencies install without errors".to_string(),
                "Build commands produce zero errors".to_string(),
                "Project files are complete".to_string(),
            ],
            sub_dimensions: None,
        },
        AuditDimensionSpec {
            name: "functional_completeness".to_string(),
            name_zh: "功能完整性".to_string(),
            max_score: 25.0,
            criteria: vec![
                "All required features have corresponding code".to_string(),
                "Implementations are substantive, not stubs".to_string(),
                "Feature completion rate matches requirements".to_string(),
            ],
            sub_dimensions: None,
        },
        AuditDimensionSpec {
            name: "code_quality".to_string(),
            name_zh: "代码质量".to_string(),
            max_score: 30.0,
            criteria: vec![
                "Architecture: clear directory structure and separation of concerns".to_string(),
                "Standards: consistent naming, proper error handling, idiomatic patterns".to_string(),
                "Security: no hardcoded secrets, input validation, no obvious vulnerabilities"
                    .to_string(),
            ],
            sub_dimensions: Some(vec![
                AuditDimensionSpec {
                    name: "architecture".to_string(),
                    name_zh: "架构".to_string(),
                    max_score: 10.0,
                    criteria: vec![
                        "Clear directory structure with separation of concerns".to_string(),
                        "Reasonable layering (MVC, Clean Architecture, etc.)".to_string(),
                        "Reasonable inter-module coupling".to_string(),
                    ],
                    sub_dimensions: None,
                },
                AuditDimensionSpec {
                    name: "standards".to_string(),
                    name_zh: "规范".to_string(),
                    max_score: 10.0,
                    criteria: vec![
                        "Consistent and meaningful naming".to_string(),
                        "Proper error handling".to_string(),
                        "No obvious code smells".to_string(),
                        "Idiomatic patterns for the language".to_string(),
                    ],
                    sub_dimensions: None,
                },
                AuditDimensionSpec {
                    name: "security".to_string(),
                    name_zh: "安全".to_string(),
                    max_score: 10.0,
                    criteria: vec![
                        "No hardcoded secrets or API keys".to_string(),
                        "Input validation on external interfaces".to_string(),
                        "No obvious vulnerabilities (SQLi, XSS, etc.)".to_string(),
                        "Environment variables documented in .env.example".to_string(),
                    ],
                    sub_dimensions: None,
                },
            ]),
        },
        AuditDimensionSpec {
            name: "test_quality".to_string(),
            name_zh: "测试质量".to_string(),
            max_score: 15.0,
            criteria: vec![
                "Test files exist and are not stubs".to_string(),
                "Tests run and pass".to_string(),
                "Reasonable coverage (unit + integration)".to_string(),
            ],
            sub_dimensions: None,
        },
        AuditDimensionSpec {
            name: "engineering_docs".to_string(),
            name_zh: "工程化文档".to_string(),
            max_score: 10.0,
            criteria: vec![
                "README with project description, setup, and API docs".to_string(),
                "Deployment config (Dockerfile, docker-compose)".to_string(),
                "CI config (.github/workflows/)".to_string(),
                "Proper .gitignore".to_string(),
            ],
            sub_dimensions: None,
        },
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn builtin_principles_contains_all_dimensions() {
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Dimension 1: Buildability"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Dimension 2: Functional Completeness"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Dimension 3: Code Quality"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Dimension 4: Test Quality"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Dimension 5: Engineering & Documentation"));
    }

    #[test]
    fn builtin_principles_contains_veto_rules() {
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Veto Rules"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("Repository is empty"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("50% of source files are stubs"));
        assert!(BUILTIN_AUDIT_PRINCIPLES.contains("verbatim copy"));
    }

    #[test]
    fn default_plan_has_correct_structure() {
        let plan = default_audit_plan();
        assert_eq!(plan.mode, AuditMode::Builtin);
        assert!((plan.pass_threshold - 90.0).abs() < f64::EPSILON);
        assert_eq!(plan.dimensions.len(), 5);
        assert!(!plan.raw_principles.is_empty());
    }

    #[test]
    fn default_plan_dimension_max_scores_sum_to_100() {
        let plan = default_audit_plan();
        let total: f64 = plan.dimensions.iter().map(|d| d.max_score).sum();
        assert!(
            (total - 100.0).abs() < f64::EPSILON,
            "total max scores should be 100, got {total}"
        );
    }

    #[test]
    fn code_quality_has_three_sub_dimensions() {
        let plan = default_audit_plan();
        let cq = plan
            .dimensions
            .iter()
            .find(|d| d.name == "code_quality")
            .expect("code_quality dimension must exist");
        let subs = cq.sub_dimensions.as_ref().expect("must have sub_dimensions");
        assert_eq!(subs.len(), 3);

        let sub_total: f64 = subs.iter().map(|s| s.max_score).sum();
        assert!(
            (sub_total - 30.0).abs() < f64::EPSILON,
            "code_quality sub-dimension max scores should sum to 30, got {sub_total}"
        );
    }
}
