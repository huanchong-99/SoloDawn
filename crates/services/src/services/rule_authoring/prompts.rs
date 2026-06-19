//! System + user prompts for each authoring agent (PRD §7.3).
//!
//! Each agent has a DISTINCT system prompt — the adversary genuinely hunts for
//! breakage rather than "discussing", and the context-free interpreter is handed
//! ONLY the rule body + description (no NL request, no debate, no examples) so the
//! round-trip is a real independent check (PRD design principle 5). Every prompt
//! instructs the model to reply with a single JSON object so the stage can parse
//! it deterministically with [`crate::services::rule_authoring::json`].

use crate::services::rule_authoring::types::{AdversaryFindings, GeneratedRule};

/// Proposer system prompt (PRD §7.3 step 1). Drafts a scoped-regex rule + plain
/// description + 2-3 positive (MUST match) + 2-3 negative (MUST NOT match)
/// examples.
pub const PROPOSER_SYSTEM: &str = "\
You are a precise static-analysis rule author for a code quality gate. \
You translate a natural-language request into a DECLARATIVE, sandboxed rule that \
is pure data, never executable code. The rule format is `regex`: a Rust `regex` \
crate pattern (linear-time; NO backreferences, NO lookaround) matched per line \
against source files.

Output requirements (reply with ONE JSON object, no prose, no markdown fence):
{
  \"ruleFormat\": \"regex\",
  \"pattern\": \"<a Rust regex matching exactly the forbidden construct>\",
  \"name\": \"<short rule name>\",
  \"description\": \"<plain-language explanation of what this rule forbids and why>\",
  \"message\": \"<message shown on each match>\",
  \"ruleType\": \"CodeSmell\",
  \"severity\": \"Major\",
  \"languages\": [\"rust\"],
  \"extensions\": [\"rs\"],
  \"excludeGlobs\": [\"**/tests/**\"],
  \"mappedMetric\": null,
  \"examples\": [
    {\"kind\":\"positive\",\"language\":\"rust\",\"snippet\":\"<code that MUST trip the rule>\"},
    {\"kind\":\"negative\",\"language\":\"rust\",\"snippet\":\"<idiomatic code that MUST NOT trip>\"}
  ]
}

Rules for a good pattern:
- Match ONLY the targeted construct; avoid over-reach (do not flag idiomatic code).
- `ruleType` is one of Bug | Vulnerability | CodeSmell | SecurityHotspot.
- `severity` is one of Info | Minor | Major | Critical | Blocker (it will be \
capped to Major at enforcement regardless).
- You MUST provide 2-3 positive and 2-3 negative examples. Positives MUST match \
the pattern; negatives MUST NOT. The examples are EXECUTED against your pattern; \
if any positive fails to match or any negative matches, the rule is rejected.";

/// Adversary system prompt (PRD §7.3 step 2). Hunts false-positives / over-reach
/// / ambiguity / evasions; it does NOT agree or summarize.
pub const ADVERSARY_SYSTEM: &str = "\
You are a RED-TEAM adversary auditing a proposed code-quality regex rule. Your \
job is to BREAK it, not to agree with it. Find:
- FALSE POSITIVES: idiomatic, correct code that the pattern would wrongly flag \
(e.g. an inline `#[cfg(test)]` module wrongly flagged as a missing test file).
- EVASIONS: code that satisfies the bad intent but slips past the pattern \
(spacing, aliasing, alternate spelling).
- OVER-REACH / AMBIGUITY: scopes that are too broad or unclear.

Reply with ONE JSON object, no prose, no markdown fence:
{
  \"looksSound\": false,
  \"critique\": \"<concrete description of the weaknesses you found>\",
  \"examples\": [
    {\"kind\":\"negative\",\"language\":\"rust\",\"snippet\":\"<idiomatic code the rule must NOT flag (a false positive you found)>\",\"note\":\"false positive\"},
    {\"kind\":\"positive\",\"language\":\"rust\",\"snippet\":\"<evasive code that SHOULD be flagged but the pattern misses>\",\"note\":\"evasion\"}
  ]
}

Set `looksSound` to true ONLY if you genuinely cannot find any false positive, \
evasion, or ambiguity. When you set it true, return an empty `examples` array. \
Your `examples` become PERMANENT fixtures the rule must satisfy.";

/// Interpreter system prompt (PRD §7.3 step 5). CONTEXT-FREE: given ONLY the rule
/// body + description, reconstruct what the rule forbids — no original request,
/// no debate, no examples.
pub const INTERPRETER_SYSTEM: &str = "\
You are reverse-engineering a code-quality rule. You are given ONLY the rule's \
matcher body and its description — you do NOT have the original request that \
produced it. In one or two sentences, state plainly WHAT this rule forbids or \
requires in source code, as if explaining it to a developer who will be subject \
to it.

Reply with ONE JSON object, no prose, no markdown fence:
{ \"reconstructedRequest\": \"<what the rule forbids/requires, in plain language>\" }";

/// Matcher/judge system prompt (PRD §7.3 step 6). Compares the context-free
/// reconstruction to the ORIGINAL request and returns an `AuditScoreResult`-style
/// score plus a match verdict.
pub const MATCHER_SYSTEM: &str = "\
You are an impartial judge comparing two descriptions of a code-quality rule: \
(1) the user's ORIGINAL natural-language request, and (2) an independent \
reconstruction produced by reading ONLY the generated rule body. Decide whether \
the rule, as built, faithfully captures the original intent — neither broader \
nor narrower.

Reply with ONE JSON object, no prose, no markdown fence:
{
  \"matches\": true,
  \"judgeScore\": 95.0,
  \"reason\": \"<why the reconstruction does or does not match the original intent>\"
}

`matches` is true ONLY when the reconstruction semantically matches the original \
request. `judgeScore` is 0-100; a faithful match should score >= 90.";

/// Build the proposer's user prompt from the NL request and the current rules
/// context (PRD §7.3 step 1).
pub fn proposer_user(nl_request: &str, current_rules_context: Option<&str>) -> String {
    let context = current_rules_context
        .filter(|c| !c.trim().is_empty())
        .map(|c| format!("\n\nThe gate currently has these conditions (for context, do not duplicate):\n{c}"))
        .unwrap_or_default();
    format!("Author a quality-gate rule for this request:\n\n{nl_request}{context}")
}

/// Build the proposer's revision user prompt for a re-loop (PRD §7.3 step 7).
/// Feeds back the previous candidate, the adversary's critique, the empirical
/// failures, and the round-trip delta as concrete fix instructions.
pub fn revise_user(
    nl_request: &str,
    previous: &GeneratedRule,
    fix_instructions: &str,
) -> String {
    let prev_json = serde_json::to_string_pretty(previous)
        .unwrap_or_else(|_| "<unserializable previous candidate>".to_string());
    format!(
        "Revise the quality-gate rule for this request:\n\n{nl_request}\n\n\
         Your PREVIOUS candidate was:\n{prev_json}\n\n\
         It FAILED for these reasons — fix ALL of them and re-emit the full JSON \
         object (keep the examples that still apply, fix or replace the ones that \
         broke, and ensure every positive matches and every negative does not):\n\n{fix_instructions}"
    )
}

/// Build the adversary's user prompt (PRD §7.3 step 2).
pub fn adversary_user(nl_request: &str, candidate: &GeneratedRule) -> String {
    let candidate_json = serde_json::to_string_pretty(candidate)
        .unwrap_or_else(|_| "<unserializable candidate>".to_string());
    format!(
        "Original request:\n{nl_request}\n\n\
         Proposed rule to break:\n{candidate_json}"
    )
}

/// Build the context-free interpreter's user prompt (PRD §7.3 step 5). ONLY the
/// rule body + description — deliberately no NL request, debate, or examples.
pub fn interpreter_user(pattern: &str, description: &str) -> String {
    format!(
        "Rule matcher body (Rust regex):\n{pattern}\n\nRule description:\n{description}"
    )
}

/// Build the matcher/judge user prompt (PRD §7.3 step 6).
pub fn matcher_user(original_request: &str, reconstructed: &str) -> String {
    format!(
        "ORIGINAL request:\n{original_request}\n\n\
         Independent reconstruction (from the rule body alone):\n{reconstructed}"
    )
}

/// Fold an adversary's findings + empirical failures + round-trip delta into a
/// single fix-instruction blob for the proposer's revision prompt.
pub fn build_fix_instructions(
    adversary: Option<&AdversaryFindings>,
    empirical_failures: &[String],
    round_trip_delta: Option<&str>,
) -> String {
    let mut parts: Vec<String> = Vec::new();
    if let Some(adv) = adversary {
        if !adv.critique.trim().is_empty() {
            parts.push(format!("Adversary critique: {}", adv.critique));
        }
    }
    if !empirical_failures.is_empty() {
        parts.push(format!(
            "Empirical failures (these are AUTHORITATIVE — the rule was executed):\n- {}",
            empirical_failures.join("\n- ")
        ));
    }
    if let Some(delta) = round_trip_delta {
        if !delta.trim().is_empty() {
            parts.push(format!(
                "Round-trip mismatch — a fresh reader of the rule body understood it differently than your request: {delta}"
            ));
        }
    }
    if parts.is_empty() {
        "The candidate did not converge; tighten the pattern so positives match and negatives do not.".to_string()
    } else {
        parts.join("\n\n")
    }
}
