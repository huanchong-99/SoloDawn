//! Local JSON extraction for authoring-stage LLM output.
//!
//! Mirrors the depth-aware extractor used by the orchestrator
//! (`agent.rs::extract_json_from_mixed_response` / `normalize_instruction_payload`),
//! but kept module-local so this feature does not widen the orchestrator's public
//! surface (those helpers are private associated fns). The behavior is
//! intentionally identical: strip a `json` fence, else find the first balanced
//! top-level `{...}` / `[...]` block, tolerating prose around it.

use serde::de::DeserializeOwned;

/// Strip a single surrounding ```` ```json ```` / ```` ``` ```` fence, if the
/// whole payload is fenced (the `normalize_instruction_payload` behavior).
fn strip_code_fence(response: &str) -> &str {
    let trimmed = response.trim();
    if trimmed.starts_with("```") && trimmed.ends_with("```") {
        let without_opening = trimmed
            .trim_start_matches("```json")
            .trim_start_matches("```JSON")
            .trim_start_matches("```");
        return without_opening
            .strip_suffix("```")
            .map_or(without_opening.trim(), str::trim);
    }
    trimmed
}

/// Extract the first balanced top-level JSON object/array from a mixed text
/// response, tolerating prose and a surrounding code fence around it. Returns the
/// owned JSON substring, or `None` when no balanced block is found.
pub fn extract_json_block(response: &str) -> Option<String> {
    let response = strip_code_fence(response);

    let first_bracket = response.find('[').unwrap_or(usize::MAX);
    let first_brace = response.find('{').unwrap_or(usize::MAX);
    let start = first_bracket.min(first_brace);
    if start >= response.len() {
        return None;
    }

    let remaining = &response[start..];
    let (open, close) = if remaining.starts_with('[') {
        ('[', ']')
    } else {
        ('{', '}')
    };

    let mut depth = 0i32;
    let mut in_string = false;
    let mut escape_next = false;
    for (i, ch) in remaining.char_indices() {
        if escape_next {
            escape_next = false;
            continue;
        }
        if ch == '\\' && in_string {
            escape_next = true;
            continue;
        }
        if ch == '"' {
            in_string = !in_string;
            continue;
        }
        if in_string {
            continue;
        }
        if ch == open {
            depth += 1;
        } else if ch == close {
            depth -= 1;
            if depth == 0 {
                return Some(remaining[..=i].to_string());
            }
        }
    }
    None
}

/// Parse an LLM response into `T`: try a direct parse of the fence-stripped
/// payload first, then fall back to the first balanced JSON block. Returns a
/// descriptive error (including a snippet of the raw response) on failure, so the
/// fail-closed stages can log/feed it back.
pub fn parse_llm_json<T: DeserializeOwned>(response: &str) -> anyhow::Result<T> {
    let normalized = strip_code_fence(response);
    if let Ok(value) = serde_json::from_str::<T>(normalized) {
        return Ok(value);
    }
    if let Some(block) = extract_json_block(response) {
        return serde_json::from_str::<T>(&block).map_err(|e| {
            anyhow::anyhow!("failed to parse extracted JSON block: {e}; block: {block}")
        });
    }
    let preview: String = response.chars().take(280).collect();
    Err(anyhow::anyhow!(
        "no JSON object/array found in LLM response; got: {preview}"
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde::Deserialize;

    #[derive(Debug, Deserialize, PartialEq)]
    struct Sample {
        a: i32,
        b: String,
    }

    #[test]
    fn parses_bare_json() {
        let s: Sample = parse_llm_json(r#"{"a":1,"b":"x"}"#).unwrap();
        assert_eq!(s, Sample { a: 1, b: "x".into() });
    }

    #[test]
    fn parses_fenced_json() {
        let s: Sample = parse_llm_json("```json\n{\"a\":2,\"b\":\"y\"}\n```").unwrap();
        assert_eq!(s, Sample { a: 2, b: "y".into() });
    }

    #[test]
    fn parses_json_amid_prose() {
        let raw = "Here is the rule you asked for:\n{\"a\":3,\"b\":\"z\"}\nLet me know!";
        let s: Sample = parse_llm_json(raw).unwrap();
        assert_eq!(s, Sample { a: 3, b: "z".into() });
    }

    #[test]
    fn ignores_braces_inside_strings() {
        let raw = r#"{"a":4,"b":"a } brace { inside"}"#;
        let s: Sample = parse_llm_json(raw).unwrap();
        assert_eq!(
            s,
            Sample {
                a: 4,
                b: "a } brace { inside".into()
            }
        );
    }

    #[test]
    fn errors_when_no_json() {
        let err = parse_llm_json::<Sample>("no json here at all").unwrap_err();
        assert!(err.to_string().contains("no JSON"));
    }
}
