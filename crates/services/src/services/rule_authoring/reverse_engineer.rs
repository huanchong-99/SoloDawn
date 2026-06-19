//! Steps 5 + 6 — CONTEXT-FREE REVERSE-ENGINEER (Interpreter) and JUDGE-COMPARE
//! (Matcher) (PRD §7.3).
//!
//! `interpret` is a FRESH LLM call given ONLY the rule body + description — no NL
//! request, no debate, no examples — so the reconstruction is a genuinely
//! independent reading (design principle 5). `compare` then judges the
//! reconstruction against the ORIGINAL request and returns a [`RoundTripVerdict`]
//! (`AuditScoreResult`-style score + match verdict). A mismatch forces a re-loop
//! within the cap. Both fail-closed to a non-matching verdict so a flaky/parse
//! failure never silently passes a rule.

use serde::Deserialize;

use crate::services::rule_authoring::{
    AuthoringLlm, json::parse_llm_json, prompts, types::RoundTripVerdict,
};

/// The interpreter's parsed output (PRD §7.3 step 5).
#[derive(Debug, Deserialize)]
struct Reconstruction {
    #[serde(rename = "reconstructedRequest")]
    reconstructed_request: String,
}

/// The matcher's parsed output (PRD §7.3 step 6), before folding into a
/// [`RoundTripVerdict`] (which also carries the reconstruction).
#[derive(Debug, Deserialize)]
struct MatchVerdict {
    matches: bool,
    #[serde(rename = "judgeScore", default)]
    judge_score: f64,
    #[serde(default)]
    reason: String,
}

/// Step 5: context-free reconstruction of what the rule forbids. The caller
/// passes ONLY the rule body + description.
pub async fn interpret(
    llm: &dyn AuthoringLlm,
    pattern: &str,
    description: &str,
) -> anyhow::Result<String> {
    let user = prompts::interpreter_user(pattern, description);
    let response = llm.complete(prompts::INTERPRETER_SYSTEM, &user).await?;
    let parsed = parse_llm_json::<Reconstruction>(&response)
        .map_err(|e| anyhow::anyhow!("interpreter output was not parseable: {e}"))?;
    Ok(parsed.reconstructed_request)
}

/// Step 6: judge-compare the reconstruction to the original request. Fail-closes
/// to a non-matching verdict (`matches=false`, score 0) on LLM/parse error so a
/// failure never passes the round-trip silently.
pub async fn compare(
    llm: &dyn AuthoringLlm,
    original_request: &str,
    reconstructed_request: &str,
) -> RoundTripVerdict {
    let user = prompts::matcher_user(original_request, reconstructed_request);
    let response = match llm.complete(prompts::MATCHER_SYSTEM, &user).await {
        Ok(r) => r,
        Err(e) => {
            tracing::warn!(error = %e, "matcher LLM call failed; failing the round-trip closed");
            return RoundTripVerdict {
                matches: false,
                reason: format!("round-trip judge unavailable: {e}"),
                reconstructed_request: reconstructed_request.to_string(),
                judge_score: 0.0,
            };
        }
    };

    match parse_llm_json::<MatchVerdict>(&response) {
        Ok(v) => RoundTripVerdict {
            matches: v.matches,
            reason: v.reason,
            reconstructed_request: reconstructed_request.to_string(),
            judge_score: v.judge_score,
        },
        Err(e) => {
            tracing::warn!(error = %e, "matcher output unparseable; failing the round-trip closed");
            RoundTripVerdict {
                matches: false,
                reason: format!("round-trip judge output unparseable: {e}"),
                reconstructed_request: reconstructed_request.to_string(),
                judge_score: 0.0,
            }
        }
    }
}

/// Run the full round-trip (interpret then compare) for a candidate. A failure in
/// the context-free interpretation itself yields a non-matching verdict (the rule
/// body could not even be read back), keeping the pipeline fail-closed.
pub async fn round_trip(
    interpreter: &dyn AuthoringLlm,
    matcher: &dyn AuthoringLlm,
    original_request: &str,
    pattern: &str,
    description: &str,
) -> RoundTripVerdict {
    let reconstructed = match interpret(interpreter, pattern, description).await {
        Ok(r) => r,
        Err(e) => {
            tracing::warn!(error = %e, "context-free interpretation failed; failing round-trip closed");
            return RoundTripVerdict {
                matches: false,
                reason: format!("context-free interpretation failed: {e}"),
                reconstructed_request: String::new(),
                judge_score: 0.0,
            };
        }
    };
    compare(matcher, original_request, &reconstructed).await
}
