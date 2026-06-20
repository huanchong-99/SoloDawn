//! Step 2 — ADVERSARIAL REVIEW (Adversary) (PRD §7.3).
//!
//! A SECOND LLM call with a distinct ATTACKER prompt that hunts false-positives /
//! over-reach / ambiguity / evasions (it does not "discuss"). Its snippets are
//! appended as permanent fixtures and its critique feeds the generator on a
//! re-loop. Fail-closed: on LLM/parse error we return an empty, NOT-sound finding
//! so the loop continues (the empirical test, step 3, is the authoritative gate
//! either way) rather than aborting the run.

use crate::services::rule_authoring::{
    AuthoringLlm, json::parse_llm_json, prompts, types::AdversaryFindings,
};

/// Attack the candidate rule (PRD §7.3 step 2). Fail-closes to "not sound, no
/// extra fixtures" on any LLM/parse error.
pub async fn attack(
    llm: &dyn AuthoringLlm,
    nl_request: &str,
    candidate: &crate::services::rule_authoring::types::GeneratedRule,
) -> AdversaryFindings {
    let user = prompts::adversary_user(nl_request, candidate);
    let response = match llm.complete(prompts::ADVERSARY_SYSTEM, &user).await {
        Ok(r) => r,
        Err(e) => {
            tracing::warn!(error = %e, "adversary LLM call failed; treating as no findings");
            return AdversaryFindings {
                looks_sound: false,
                critique: format!("adversary unavailable: {e}"),
                examples: Vec::new(),
            };
        }
    };

    match parse_llm_json::<AdversaryFindings>(&response) {
        Ok(findings) => findings,
        Err(e) => {
            tracing::warn!(error = %e, "adversary output unparseable; treating as no findings");
            AdversaryFindings {
                looks_sound: false,
                critique: format!("adversary output unparseable: {e}"),
                examples: Vec::new(),
            }
        }
    }
}
