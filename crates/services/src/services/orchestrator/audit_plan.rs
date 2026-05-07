//! Audit plan generation pipeline.
//!
//! Given a confirmed spec and an optional user-uploaded audit document,
//! produces an `AuditPlan` by calling the LLM to tailor scoring criteria
//! to the specific project.

use super::{
    audit_principles::{BUILTIN_AUDIT_PRINCIPLES, default_audit_plan},
    llm::LLMClient,
    types::{AuditMode, AuditPlan, LLMMessage},
};

/// Generate an audit plan by calling the LLM to tailor scoring criteria.
///
/// # Modes
/// - **Builtin** (A): LLM tailors `BUILTIN_AUDIT_PRINCIPLES` to the project spec.
/// - **Merged** (B): LLM merges user doc + built-in principles; user doc takes precedence.
/// - **Custom** (C): LLM structures user doc into the 5-dimension scoring format.
///
/// # Fail-closed
/// Any LLM or parse failure returns `default_audit_plan()`.
pub async fn generate_audit_plan(
    llm_client: &dyn LLMClient,
    requirement_summary: &str,
    technical_spec: &str,
    user_audit_doc: Option<&str>,
    mode: AuditMode,
) -> AuditPlan {
    let prompt = build_generation_prompt(requirement_summary, technical_spec, user_audit_doc, mode);

    let messages = vec![
        LLMMessage {
            role: "system".to_string(),
            content: "You are an expert software quality auditor. \
                      You generate structured audit plans for code review. \
                      Respond ONLY with a valid JSON object — no markdown fences, no explanation."
                .to_string(),
        },
        LLMMessage {
            role: "user".to_string(),
            content: prompt,
        },
    ];

    let response = match llm_client.chat(messages).await {
        Ok(r) => r,
        Err(e) => {
            tracing::warn!(error = %e, "LLM call failed during audit plan generation; using default");
            return default_audit_plan();
        }
    };

    match parse_audit_plan_response(&response.content, mode, user_audit_doc) {
        Ok(plan) => {
            tracing::info!(
                mode = ?mode,
                dimensions = plan.dimensions.len(),
                "Audit plan generated successfully"
            );
            plan
        }
        Err(e) => {
            tracing::warn!(
                error = %e,
                "Failed to parse LLM audit plan response; using default"
            );
            default_audit_plan()
        }
    }
}

/// Build the LLM prompt based on the audit mode.
fn build_generation_prompt(
    requirement_summary: &str,
    technical_spec: &str,
    user_audit_doc: Option<&str>,
    mode: AuditMode,
) -> String {
    match mode {
        AuditMode::Builtin => {
            format!(
                r#"Below are the BUILT-IN audit scoring principles that will be used to evaluate code deliveries:

<BUILTIN_PRINCIPLES>
{BUILTIN_AUDIT_PRINCIPLES}
</BUILTIN_PRINCIPLES>

Below is the project specification:

<REQUIREMENT_SUMMARY>
{requirement_summary}
</REQUIREMENT_SUMMARY>

<TECHNICAL_SPEC>
{technical_spec}
</TECHNICAL_SPEC>

Your task: Tailor the built-in audit principles to this specific project. Add project-specific criteria based on the spec (e.g., if the spec mentions JWT auth, add "must implement JWT authentication" to functional completeness criteria). Keep the same 5-dimension structure and scoring rubric.

Return a JSON object with this exact schema:
{{
  "mode": "builtin",
  "dimensions": [
    {{
      "name": "buildability",
      "name_zh": "可构建性",
      "max_score": 20.0,
      "criteria": ["criterion 1", "criterion 2", ...],
      "sub_dimensions": null
    }},
    {{
      "name": "functional_completeness",
      "name_zh": "功能完整性",
      "max_score": 25.0,
      "criteria": ["criterion 1", ...],
      "sub_dimensions": null
    }},
    {{
      "name": "code_quality",
      "name_zh": "代码质量",
      "max_score": 30.0,
      "criteria": ["overall criterion 1", ...],
      "sub_dimensions": [
        {{"name": "architecture", "name_zh": "架构", "max_score": 10.0, "criteria": [...], "sub_dimensions": null}},
        {{"name": "standards", "name_zh": "规范", "max_score": 10.0, "criteria": [...], "sub_dimensions": null}},
        {{"name": "security", "name_zh": "安全", "max_score": 10.0, "criteria": [...], "sub_dimensions": null}}
      ]
    }},
    {{
      "name": "test_quality",
      "name_zh": "测试质量",
      "max_score": 15.0,
      "criteria": ["criterion 1", ...],
      "sub_dimensions": null
    }},
    {{
      "name": "engineering_docs",
      "name_zh": "工程化文档",
      "max_score": 10.0,
      "criteria": ["criterion 1", ...],
      "sub_dimensions": null
    }}
  ],
  "pass_threshold": 90.0,
  "generated_at": "<ISO 8601 timestamp>",
  "raw_principles": "<the complete scoring rubric text, including both built-in principles and any project-specific additions>"
}}"#
            )
        }
        AuditMode::Merged => {
            let user_doc = user_audit_doc.unwrap_or("");
            format!(
                r#"Below are TWO audit scoring documents that need to be intelligently MERGED.

Document 1 — BUILT-IN audit scoring principles:
<BUILTIN_PRINCIPLES>
{BUILTIN_AUDIT_PRINCIPLES}
</BUILTIN_PRINCIPLES>

Document 2 — USER-PROVIDED audit document (takes precedence on conflicts):
<USER_DOC>
{user_doc}
</USER_DOC>

Below is the project specification:
<REQUIREMENT_SUMMARY>
{requirement_summary}
</REQUIREMENT_SUMMARY>

<TECHNICAL_SPEC>
{technical_spec}
</TECHNICAL_SPEC>

Your task: Merge both documents into a single audit plan. Rules:
1. Keep the 5-dimension structure (buildability, functional_completeness, code_quality, test_quality, engineering_docs)
2. When both documents address the same topic, the USER document takes precedence
3. Include criteria from both sources, removing duplicates
4. Add project-specific criteria from the spec
5. The raw_principles field should contain the full merged scoring rubric text

Return a JSON object with the same schema as above, but with "mode": "merged"."#
            )
        }
        AuditMode::Custom => {
            let user_doc = user_audit_doc.unwrap_or("");
            format!(
                r#"Below is a USER-PROVIDED audit document that should be used as the SOLE basis for the audit plan. Ignore any built-in scoring principles.

<USER_DOC>
{user_doc}
</USER_DOC>

Below is the project specification:
<REQUIREMENT_SUMMARY>
{requirement_summary}
</REQUIREMENT_SUMMARY>

<TECHNICAL_SPEC>
{technical_spec}
</TECHNICAL_SPEC>

Your task: Structure the user's audit document into the standard 5-dimension scoring format. If the user's document uses different dimensions, map them as closely as possible to:
  - buildability (max 20 pts)
  - functional_completeness (max 25 pts)
  - code_quality (max 30 pts, with sub-dimensions: architecture 10, standards 10, security 10)
  - test_quality (max 15 pts)
  - engineering_docs (max 10 pts)

If the user's document specifies different point allocations, respect those instead. The raw_principles field should contain the user's document content.

Return a JSON object with the same schema as above, but with "mode": "custom"."#
            )
        }
    }
}

/// Parse the LLM response into an `AuditPlan`.
///
/// Extracts JSON from the response, deserializes, and sets `raw_principles`
/// based on the mode if the LLM didn't provide it.
fn parse_audit_plan_response(
    response: &str,
    mode: AuditMode,
    user_audit_doc: Option<&str>,
) -> Result<AuditPlan, String> {
    let trimmed = response.trim();

    // Extract JSON block
    let json_str = if let Some(start) = trimmed.find('{') {
        if let Some(end) = trimmed.rfind('}') {
            &trimmed[start..=end]
        } else {
            return Err("No closing brace found in response".to_string());
        }
    } else {
        return Err("No JSON object found in response".to_string());
    };

    let mut plan: AuditPlan =
        serde_json::from_str(json_str).map_err(|e| format!("JSON parse error: {e}"))?;

    // Override the mode to match what was requested (LLM may get it wrong)
    plan.mode = mode;

    // Ensure raw_principles is populated
    if plan.raw_principles.is_empty() {
        plan.raw_principles = match mode {
            AuditMode::Builtin => BUILTIN_AUDIT_PRINCIPLES.to_string(),
            AuditMode::Merged => BUILTIN_AUDIT_PRINCIPLES.to_string(),
            AuditMode::Custom => user_audit_doc.unwrap_or("").to_string(),
        };
    }

    // Validate dimension structure
    if plan.dimensions.is_empty() {
        return Err("No dimensions found in audit plan".to_string());
    }

    Ok(plan)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::services::orchestrator::llm::MockLLMClient;

    fn make_valid_plan_json(mode: &str) -> String {
        format!(
            r#"{{
            "mode": "{mode}",
            "dimensions": [
                {{"name": "buildability", "name_zh": "可构建性", "max_score": 20.0, "criteria": ["builds cleanly"], "sub_dimensions": null}},
                {{"name": "functional_completeness", "name_zh": "功能完整性", "max_score": 25.0, "criteria": ["all features"], "sub_dimensions": null}},
                {{"name": "code_quality", "name_zh": "代码质量", "max_score": 30.0, "criteria": ["good quality"], "sub_dimensions": [
                    {{"name": "architecture", "name_zh": "架构", "max_score": 10.0, "criteria": ["clean arch"], "sub_dimensions": null}},
                    {{"name": "standards", "name_zh": "规范", "max_score": 10.0, "criteria": ["consistent naming"], "sub_dimensions": null}},
                    {{"name": "security", "name_zh": "安全", "max_score": 10.0, "criteria": ["no secrets"], "sub_dimensions": null}}
                ]}},
                {{"name": "test_quality", "name_zh": "测试质量", "max_score": 15.0, "criteria": ["tests pass"], "sub_dimensions": null}},
                {{"name": "engineering_docs", "name_zh": "工程化文档", "max_score": 10.0, "criteria": ["readme exists"], "sub_dimensions": null}}
            ],
            "pass_threshold": 90.0,
            "generated_at": "2026-05-07T00:00:00Z",
            "raw_principles": "test principles"
        }}"#
        )
    }

    #[tokio::test]
    async fn generate_plan_builtin_mode_with_valid_response() {
        let json = make_valid_plan_json("builtin");
        let client = MockLLMClient::with_response(&json);

        let plan = generate_audit_plan(
            &client,
            "Build a todo app",
            "{ \"productGoal\": \"todo app\" }",
            None,
            AuditMode::Builtin,
        )
        .await;

        assert_eq!(plan.mode, AuditMode::Builtin);
        assert_eq!(plan.dimensions.len(), 5);
        assert!((plan.pass_threshold - 90.0).abs() < f64::EPSILON);
    }

    #[tokio::test]
    async fn generate_plan_falls_back_on_llm_failure() {
        let client = MockLLMClient::that_fails();

        let plan = generate_audit_plan(
            &client,
            "Build a todo app",
            "{}",
            None,
            AuditMode::Builtin,
        )
        .await;

        // Should return default plan
        assert_eq!(plan.mode, AuditMode::Builtin);
        assert_eq!(plan.dimensions.len(), 5);
        assert!(plan.raw_principles.contains("Dimension 1: Buildability"));
    }

    #[tokio::test]
    async fn generate_plan_falls_back_on_invalid_json() {
        let client = MockLLMClient::with_response("This is not valid JSON at all.");

        let plan = generate_audit_plan(
            &client,
            "Build a todo app",
            "{}",
            None,
            AuditMode::Builtin,
        )
        .await;

        // Should return default plan
        assert_eq!(plan.mode, AuditMode::Builtin);
        assert!(plan.raw_principles.contains("Veto Rules"));
    }

    #[tokio::test]
    async fn generate_plan_merged_mode() {
        let json = make_valid_plan_json("merged");
        let client = MockLLMClient::with_response(&json);

        let plan = generate_audit_plan(
            &client,
            "Build an auth service",
            "{ \"productGoal\": \"auth\" }",
            Some("Custom: JWT must use RS256"),
            AuditMode::Merged,
        )
        .await;

        assert_eq!(plan.mode, AuditMode::Merged);
        assert_eq!(plan.dimensions.len(), 5);
    }

    #[tokio::test]
    async fn generate_plan_custom_mode() {
        let json = make_valid_plan_json("custom");
        let client = MockLLMClient::with_response(&json);

        let plan = generate_audit_plan(
            &client,
            "Build a payment system",
            "{}",
            Some("My custom audit criteria..."),
            AuditMode::Custom,
        )
        .await;

        assert_eq!(plan.mode, AuditMode::Custom);
        assert_eq!(plan.dimensions.len(), 5);
    }

    #[test]
    fn parse_response_extracts_json_from_surrounding_text() {
        let json = make_valid_plan_json("builtin");
        let response = format!("Here is the plan:\n\n{json}\n\nDone.");

        let plan = parse_audit_plan_response(&response, AuditMode::Builtin, None);
        assert!(plan.is_ok());
        assert_eq!(plan.unwrap().dimensions.len(), 5);
    }

    #[test]
    fn parse_response_rejects_empty_dimensions() {
        let json = r#"{"mode":"builtin","dimensions":[],"pass_threshold":90.0,"generated_at":"now","raw_principles":"x"}"#;
        let result = parse_audit_plan_response(json, AuditMode::Builtin, None);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("No dimensions"));
    }

    #[test]
    fn parse_response_fills_empty_raw_principles_builtin() {
        let json = r#"{"mode":"builtin","dimensions":[{"name":"buildability","name_zh":"可构建性","max_score":20.0,"criteria":["ok"],"sub_dimensions":null}],"pass_threshold":90.0,"generated_at":"now","raw_principles":""}"#;
        let plan = parse_audit_plan_response(json, AuditMode::Builtin, None).unwrap();
        assert!(plan.raw_principles.contains("Dimension 1: Buildability"));
    }

    #[test]
    fn parse_response_fills_empty_raw_principles_custom() {
        let json = r#"{"mode":"custom","dimensions":[{"name":"buildability","name_zh":"可构建性","max_score":20.0,"criteria":["ok"],"sub_dimensions":null}],"pass_threshold":90.0,"generated_at":"now","raw_principles":""}"#;
        let user_doc = "My custom audit rules";
        let plan = parse_audit_plan_response(json, AuditMode::Custom, Some(user_doc)).unwrap();
        assert_eq!(plan.raw_principles, user_doc);
    }

    #[test]
    fn parse_response_overrides_mode_from_llm() {
        // LLM says "builtin" but we requested "merged" — request wins
        let json = make_valid_plan_json("builtin");
        let plan = parse_audit_plan_response(&json, AuditMode::Merged, None).unwrap();
        assert_eq!(plan.mode, AuditMode::Merged);
    }
}
