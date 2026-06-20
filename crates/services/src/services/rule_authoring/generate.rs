//! Step 1 — GENERATE (Proposer) and its revision variant (PRD §7.3).
//!
//! `draft_rule` turns the NL request + current rules context into a
//! [`GeneratedRule`] (scoped-regex body + description + 2-3 positive / 2-3
//! negative examples). `revise_rule` re-drafts after a failed round, feeding back
//! the adversary critique + empirical failures + round-trip delta. Both parse the
//! LLM output with the module-local depth-aware JSON extractor and surface a clear
//! error on parse failure (the caller decides the fail-closed re-loop, mirroring
//! the `audit_plan.rs` fail-closed template).

use crate::services::rule_authoring::{
    AuthoringLlm, json::parse_llm_json, prompts, types::GeneratedRule,
};

/// Draft the initial candidate rule (PRD §7.3 step 1).
pub async fn draft_rule(
    llm: &dyn AuthoringLlm,
    nl_request: &str,
    current_rules_context: Option<&str>,
) -> anyhow::Result<GeneratedRule> {
    let user = prompts::proposer_user(nl_request, current_rules_context);
    let response = llm.complete(prompts::PROPOSER_SYSTEM, &user).await?;
    parse_llm_json::<GeneratedRule>(&response)
        .map_err(|e| anyhow::anyhow!("proposer output was not a valid rule: {e}"))
}

/// Revise the candidate after a failed round (PRD §7.3 step 7), feeding the
/// folded fix instructions back to the proposer.
pub async fn revise_rule(
    llm: &dyn AuthoringLlm,
    nl_request: &str,
    previous: &GeneratedRule,
    fix_instructions: &str,
) -> anyhow::Result<GeneratedRule> {
    let user = prompts::revise_user(nl_request, previous, fix_instructions);
    let response = llm.complete(prompts::PROPOSER_SYSTEM, &user).await?;
    parse_llm_json::<GeneratedRule>(&response)
        .map_err(|e| anyhow::anyhow!("proposer revision output was not a valid rule: {e}"))
}
