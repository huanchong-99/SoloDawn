//! Integration tests for the authoring state-machine (PRD §7.3) with a
//! deterministic scripted [`AuthoringLlm`] and an in-memory SQLite DB. These run
//! under nextest with no network and no `claude` binary.
//!
//! The scripted mock dispatches on the agent's DISTINCT system prompt (proposer /
//! adversary / interpreter / matcher) and on a shared call counter, so a single
//! `&dyn AuthoringLlm` reproduces the multi-turn behavior `MockLLMClient` cannot
//! (it returns one fixed string). Coverage:
//!   (a) full loop reaching a usable rule;
//!   (b) an empirical failure forces a re-loop then succeeds;
//!   (c) cap-out path returns `CappedOut` without panic;
//!   (d) round-trip mismatch loops.

use std::sync::atomic::{AtomicUsize, Ordering};

use async_trait::async_trait;
use sqlx::sqlite::SqlitePoolOptions;
use sqlx::SqlitePool;
use uuid::Uuid;

use crate::services::rule_authoring::{
    AuthoringAgents, AuthoringBackend, AuthoringLlm, AuthorOutcome, MAX_AUTHORING_ROUNDS,
    persist_run, prompts, revalidate_rule_body, run_authoring,
};

// --- scripted mock ---------------------------------------------------------

/// Which agent role a system prompt belongs to (matched by a stable prefix of the
/// real prompts so the mock stays in lockstep with `prompts.rs`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Role {
    Proposer,
    Adversary,
    Interpreter,
    Matcher,
}

fn role_of(system: &str) -> Role {
    if system == prompts::PROPOSER_SYSTEM {
        Role::Proposer
    } else if system == prompts::ADVERSARY_SYSTEM {
        Role::Adversary
    } else if system == prompts::INTERPRETER_SYSTEM {
        Role::Interpreter
    } else if system == prompts::MATCHER_SYSTEM {
        Role::Matcher
    } else {
        panic!("unexpected system prompt routed to scripted mock: {system:.40}");
    }
}

/// A scripted authoring backend. `script` is a closure that, given the role and
/// the 0-based number of prior calls to THAT role, returns the canned reply.
struct ScriptedLlm<F: Fn(Role, usize) -> String + Send + Sync> {
    script: F,
    proposer: AtomicUsize,
    adversary: AtomicUsize,
    interpreter: AtomicUsize,
    matcher: AtomicUsize,
}

impl<F: Fn(Role, usize) -> String + Send + Sync> ScriptedLlm<F> {
    fn new(script: F) -> Self {
        Self {
            script,
            proposer: AtomicUsize::new(0),
            adversary: AtomicUsize::new(0),
            interpreter: AtomicUsize::new(0),
            matcher: AtomicUsize::new(0),
        }
    }

    fn counter(&self, role: Role) -> &AtomicUsize {
        match role {
            Role::Proposer => &self.proposer,
            Role::Adversary => &self.adversary,
            Role::Interpreter => &self.interpreter,
            Role::Matcher => &self.matcher,
        }
    }
}

#[async_trait]
impl<F: Fn(Role, usize) -> String + Send + Sync> AuthoringLlm for ScriptedLlm<F> {
    async fn complete(&self, system: &str, _user: &str) -> anyhow::Result<String> {
        let role = role_of(system);
        let n = self.counter(role).fetch_add(1, Ordering::SeqCst);
        Ok((self.script)(role, n))
    }
}

// --- canned JSON payloads --------------------------------------------------

/// A proposer rule whose pattern correctly flags `dbg!(` and not idiomatic code,
/// with passing positive/negative examples.
fn good_rule_json() -> String {
    r#"{
        "ruleFormat":"regex",
        "pattern":"\\bdbg!\\s*\\(",
        "name":"no-dbg-macro",
        "description":"Forbids the dbg! debugging macro in committed Rust code.",
        "message":"dbg! left in committed code",
        "ruleType":"CodeSmell",
        "severity":"Major",
        "languages":["rust"],
        "extensions":["rs"],
        "examples":[
            {"kind":"positive","language":"rust","snippet":"let x = dbg!(value);"},
            {"kind":"positive","language":"rust","snippet":"  dbg!(a, b);"},
            {"kind":"negative","language":"rust","snippet":"let x = compute(value);"},
            {"kind":"negative","language":"rust","snippet":"let dbgr = Debugger::new();"}
        ]
    }"#
    .to_string()
}

/// An over-broad proposer rule: pattern `dbg` matches the negative `dbgr` too, so
/// the empirical test must fail it.
fn overbroad_rule_json() -> String {
    r#"{
        "ruleFormat":"regex",
        "pattern":"dbg",
        "name":"no-dbg-macro",
        "description":"Forbids dbg.",
        "message":"dbg present",
        "ruleType":"CodeSmell",
        "severity":"Major",
        "languages":["rust"],
        "extensions":["rs"],
        "examples":[
            {"kind":"positive","language":"rust","snippet":"let x = dbg!(value);"},
            {"kind":"negative","language":"rust","snippet":"let dbgr = make();"}
        ]
    }"#
    .to_string()
}

fn adversary_sound_json() -> String {
    r#"{"looksSound":true,"critique":"no weaknesses found","examples":[]}"#.to_string()
}

fn interpreter_json() -> String {
    r#"{"reconstructedRequest":"Disallow the dbg! macro in committed Rust source."}"#.to_string()
}

fn matcher_match_json() -> String {
    r#"{"matches":true,"judgeScore":96.0,"reason":"reconstruction matches the request"}"#
        .to_string()
}

fn matcher_mismatch_json() -> String {
    r#"{"matches":false,"judgeScore":40.0,"reason":"reconstruction is broader than the request"}"#
        .to_string()
}

// --- test DB helpers -------------------------------------------------------

async fn setup_db() -> SqlitePool {
    let pool = SqlitePoolOptions::new()
        .max_connections(1)
        .connect("sqlite::memory:")
        .await
        .expect("in-memory pool");
    let migrations =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../db/migrations");
    let migrator = sqlx::migrate::Migrator::new(migrations)
        .await
        .expect("load migrations");
    migrator.run(&pool).await.expect("run migrations");
    pool
}

async fn insert_project(pool: &SqlitePool) -> Uuid {
    let id = Uuid::new_v4();
    sqlx::query("INSERT INTO projects (id, name) VALUES (?1, ?2)")
        .bind(id)
        .bind("rule-authoring-test")
        .execute(pool)
        .await
        .expect("insert project");
    id
}

// --- (a) full loop reaches a usable rule -----------------------------------

#[tokio::test]
async fn full_loop_reaches_usable_rule() {
    let llm = ScriptedLlm::new(|role, _n| match role {
        Role::Proposer => good_rule_json(),
        Role::Adversary => adversary_sound_json(),
        Role::Interpreter => interpreter_json(),
        Role::Matcher => matcher_match_json(),
    });

    let result = run_authoring(&llm, "prohibit dbg! in Rust", None, AuthoringBackend::Metered)
        .await
        .expect("authoring run");

    assert_eq!(result.outcome, AuthorOutcome::Passed);
    assert!(result.is_usable());
    assert_eq!(result.rounds_used, 1, "a clean rule converges in one round");
    assert!(result.candidate.empirical.all_passed());
    assert!(result.candidate.round_trip.matches);
    // The candidate carries the positive/negative examples for the UI.
    assert!(result.candidate.rule.positives().count() >= 2);
    assert!(result.candidate.rule.negatives().count() >= 2);
}

// --- (b) empirical failure forces a re-loop, then succeeds ------------------

#[tokio::test]
async fn empirical_failure_forces_reloop_then_succeeds() {
    // Round 1 proposer emits the over-broad rule (negative `dbgr` wrongly flagged
    // → empirical FAIL). The revision (proposer call #1) emits the good rule.
    let llm = ScriptedLlm::new(|role, n| match role {
        Role::Proposer => {
            if n == 0 {
                overbroad_rule_json()
            } else {
                good_rule_json()
            }
        }
        Role::Adversary => adversary_sound_json(),
        Role::Interpreter => interpreter_json(),
        Role::Matcher => matcher_match_json(),
    });

    let result = run_authoring(&llm, "prohibit dbg! in Rust", None, AuthoringBackend::Metered)
        .await
        .expect("authoring run");

    assert_eq!(
        result.outcome,
        AuthorOutcome::Passed,
        "should recover after the empirical failure"
    );
    assert_eq!(result.rounds_used, 2, "one failed round + one passing round");
    assert!(result.candidate.empirical.all_passed());
}

// --- (c) cap-out path returns CappedOut without panic ----------------------

#[tokio::test]
async fn cap_out_returns_capped_without_panic() {
    // The proposer NEVER fixes the over-broad rule, so the empirical test fails
    // every round; the loop must exhaust the cap and return CappedOut (no panic).
    let llm = ScriptedLlm::new(|role, _n| match role {
        Role::Proposer => overbroad_rule_json(),
        Role::Adversary => adversary_sound_json(),
        Role::Interpreter => interpreter_json(),
        Role::Matcher => matcher_match_json(),
    });

    let result = run_authoring(&llm, "prohibit dbg! in Rust", None, AuthoringBackend::Metered)
        .await
        .expect("authoring run must not error on cap-out");

    assert_eq!(result.outcome, AuthorOutcome::CappedOut);
    assert!(!result.is_usable());
    assert_eq!(result.rounds_used, MAX_AUTHORING_ROUNDS);
    // The best candidate is still handed back for "edit manually".
    assert!(!result.candidate.empirical.all_passed());
    assert!(!result.candidate.rule.pattern.is_empty());

    // Persisting a capped-out run is still allowed (shadow, verdict=fail) so the
    // user can see the artifact; it must not panic and must write the rows.
    let pool = setup_db().await;
    let project_id = insert_project(&pool).await;
    let rule = persist_run(
        &pool,
        Some(project_id),
        "prohibit dbg! in Rust",
        Some("tester"),
        &result,
    )
    .await
    .expect("persist capped-out run");

    assert_eq!(rule.status, "shadow");
    let validations = db::models::CustomRuleValidation::find_by_rule(&pool, rule.id)
        .await
        .expect("validations");
    assert_eq!(validations.len(), 1);
    assert_eq!(validations[0].verdict, "fail");
    assert_eq!(validations[0].roundtrip_ok, Some(true));
}

// --- (d) round-trip mismatch loops -----------------------------------------

#[tokio::test]
async fn round_trip_mismatch_loops() {
    // Empirical always passes (good rule), but the matcher says "mismatch" on the
    // first round and "match" on the second → the loop must re-run on the
    // round-trip verdict alone, then converge.
    let llm = ScriptedLlm::new(|role, n| match role {
        Role::Proposer => good_rule_json(),
        Role::Adversary => adversary_sound_json(),
        Role::Interpreter => interpreter_json(),
        Role::Matcher => {
            if n == 0 {
                matcher_mismatch_json()
            } else {
                matcher_match_json()
            }
        }
    });

    let result = run_authoring(&llm, "prohibit dbg! in Rust", None, AuthoringBackend::Metered)
        .await
        .expect("authoring run");

    assert_eq!(result.outcome, AuthorOutcome::Passed);
    assert_eq!(
        result.rounds_used, 2,
        "round-trip mismatch forces a second round even with empirical pass"
    );
    assert!(result.candidate.round_trip.matches);
}

// --- persistence of a passing run + revalidation (D8) ----------------------

#[tokio::test]
async fn persist_passing_run_then_revalidate_body() {
    let pool = setup_db().await;
    let project_id = insert_project(&pool).await;

    let llm = ScriptedLlm::new(|role, _n| match role {
        Role::Proposer => good_rule_json(),
        Role::Adversary => adversary_sound_json(),
        Role::Interpreter => interpreter_json(),
        Role::Matcher => matcher_match_json(),
    });

    let result = run_authoring(&llm, "prohibit dbg! in Rust", None, AuthoringBackend::Subscription)
        .await
        .expect("authoring run");
    assert_eq!(result.outcome, AuthorOutcome::Passed);

    let rule = persist_run(
        &pool,
        Some(project_id),
        "prohibit dbg! in Rust",
        Some("tester"),
        &result,
    )
    .await
    .expect("persist");

    // Rule row + examples + validation + audit all persisted.
    assert_eq!(rule.status, "shadow");
    assert_eq!(rule.rule_format, "regex");
    assert_eq!(rule.severity, "MAJOR");
    assert_eq!(rule.rule_type, "CodeSmell");

    let examples = db::models::CustomRuleExample::find_by_rule(&pool, rule.id)
        .await
        .expect("examples");
    assert!(examples.len() >= 4, "candidate examples persisted");

    let validations = db::models::CustomRuleValidation::find_by_rule(&pool, rule.id)
        .await
        .expect("validations");
    assert_eq!(validations.len(), 1);
    assert_eq!(validations[0].verdict, "pass");

    let audits = db::models::CustomRuleAudit::find_by_rule(&pool, rule.id)
        .await
        .expect("audits");
    assert_eq!(audits.len(), 1);
    assert_eq!(audits[0].action, "create");

    // D8: revalidate the body → re-run pipeline, drop to shadow, fresh artifacts.
    let agents = AuthoringAgents {
        generator: &llm,
        adversary: &llm,
        interpreter: &llm,
        matcher: &llm,
    };
    let reval = revalidate_rule_body(
        &pool,
        &agents,
        rule.id,
        Some("tester"),
        AuthoringBackend::Subscription,
    )
    .await
    .expect("revalidate");
    assert_eq!(reval.outcome, AuthorOutcome::Passed);

    let after = db::models::CustomRule::find_by_id(&pool, rule.id)
        .await
        .expect("find")
        .expect("rule still present");
    assert_eq!(after.status, "shadow", "body edit drops to shadow (D8)");

    let validations = db::models::CustomRuleValidation::find_by_rule(&pool, rule.id)
        .await
        .expect("validations");
    assert_eq!(validations.len(), 2, "a second validation artifact was written");

    let audits = db::models::CustomRuleAudit::find_by_rule(&pool, rule.id)
        .await
        .expect("audits");
    assert!(
        audits.iter().any(|a| a.action == "revalidate"),
        "revalidate audit row appended"
    );
}
