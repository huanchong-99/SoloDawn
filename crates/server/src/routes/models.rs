//! Model verification and listing API endpoints
//!
//! Provides `/api/models/list` and `/api/models/verify` for frontend model configuration.

use std::time::Duration;

use axum::{
    Router,
    extract::{Json, Query},
    http::HeaderMap,
    response::Json as ResponseJson,
    routing::{get, post},
};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::{DeploymentImpl, error::ApiError};

const DEFAULT_OPENAI_BASE_URL: &str = "https://api.openai.com";
const DEFAULT_ANTHROPIC_BASE_URL: &str = "https://api.anthropic.com";
const DEFAULT_GOOGLE_BASE_URL: &str = "https://generativelanguage.googleapis.com";
const ANTHROPIC_VERSION: &str = "2023-06-01";

pub fn router() -> Router<DeploymentImpl> {
    Router::new()
        .route("/list", get(list_models))
        .route("/verify", post(verify_model))
}

#[derive(Debug, Deserialize)]
struct ModelsListQuery {
    #[serde(rename = "apiType")]
    api_type: String,
    #[serde(rename = "baseUrl")]
    base_url: Option<String>,
}

#[derive(Debug, Serialize)]
struct ModelsListResponse {
    models: Vec<String>,
}

#[derive(Debug, Deserialize)]
struct VerifyModelRequest {
    #[serde(rename = "apiType")]
    api_type: String,
    #[serde(rename = "baseUrl")]
    base_url: String,
    #[serde(rename = "apiKey")]
    api_key: String,
    #[serde(rename = "modelId")]
    model_id: String,
    /// CLI this model is bound to, when the caller knows it.
    ///
    /// Verification must exercise the API surface the CLI will really call.
    /// Codex speaks the OpenAI **Responses** API, so a model that only answers
    /// on `/chat/completions` used to verify green and then be completely
    /// unusable inside a Codex terminal.
    #[serde(rename = "cliTypeId", default)]
    cli_type_id: Option<String>,
}

#[derive(Debug, Serialize)]
struct VerifyModelResponse {
    verified: bool,
    /// Why verification failed, when it did. `None` on success.
    #[serde(skip_serializing_if = "Option::is_none")]
    detail: Option<String>,
}

/// Whether a CLI type id refers to Codex.
///
/// Accepts both the DB id (`cli-codex`) and the bare CLI name (`codex`).
fn is_codex_cli(cli_type_id: Option<&str>) -> bool {
    cli_type_id
        .map(str::trim)
        .map(str::to_ascii_lowercase)
        .is_some_and(|value| value == "codex" || value == "cli-codex")
}

/// GET /api/models/list
/// Lists available models for the given API type
async fn list_models(
    Query(query): Query<ModelsListQuery>,
    headers: HeaderMap,
) -> Result<ResponseJson<ModelsListResponse>, ApiError> {
    let api_key = api_key_from_headers(&headers)?;
    let base_url = normalized_base_url(&query.api_type, query.base_url.as_deref())?;
    let client = http_client()?;

    let models = match query.api_type.as_str() {
        "openai" | "openai-compatible" => list_openai_models(&client, &base_url, &api_key).await?,
        "anthropic" | "anthropic-compatible" => {
            list_anthropic_models(&client, &base_url, &api_key).await?
        }
        "google" => list_google_models(&client, &base_url, &api_key).await?,
        other => {
            return Err(ApiError::BadRequest(format!(
                "Unsupported apiType: {other}"
            )));
        }
    };

    Ok(ResponseJson(ModelsListResponse { models }))
}

/// POST /api/models/verify
/// Verifies that a model configuration is valid and can connect
async fn verify_model(
    Json(payload): Json<VerifyModelRequest>,
) -> Result<ResponseJson<VerifyModelResponse>, ApiError> {
    let client = http_client()?;
    let base_url = trim_trailing_slash(&payload.base_url);

    // Codex only speaks the Responses API. Verifying it through Chat
    // Completions would report a green connection for an endpoint the Codex
    // terminal cannot use at all.
    if is_codex_cli(payload.cli_type_id.as_deref()) {
        let outcome =
            verify_openai_responses_model(&client, &base_url, &payload.api_key, &payload.model_id)
                .await;
        return Ok(ResponseJson(verify_outcome_to_response(outcome)));
    }

    let outcome = match payload.api_type.as_str() {
        "openai" | "openai-compatible" => {
            verify_openai_model(&client, &base_url, &payload.api_key, &payload.model_id).await
        }
        "anthropic" | "anthropic-compatible" => {
            verify_anthropic_model(&client, &base_url, &payload.api_key, &payload.model_id).await
        }
        "google" => {
            verify_google_model(&client, &base_url, &payload.api_key, &payload.model_id).await
        }
        other => {
            return Err(ApiError::BadRequest(format!(
                "Unsupported apiType: {other}"
            )));
        }
    };

    Ok(ResponseJson(verify_outcome_to_response(outcome)))
}

/// Result of one verification attempt: verified, or a user-facing reason.
type VerifyOutcome = Result<bool, String>;

fn verify_outcome_to_response(outcome: VerifyOutcome) -> VerifyModelResponse {
    match outcome {
        Ok(true) => VerifyModelResponse {
            verified: true,
            detail: None,
        },
        Ok(false) => VerifyModelResponse {
            verified: false,
            detail: None,
        },
        Err(detail) => VerifyModelResponse {
            verified: false,
            detail: Some(detail),
        },
    }
}

fn api_key_from_headers(headers: &HeaderMap) -> Result<String, ApiError> {
    headers
        .get("X-API-Key")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(std::string::ToString::to_string)
        .ok_or_else(|| ApiError::BadRequest("X-API-Key header is required".to_string()))
}

fn http_client() -> Result<Client, ApiError> {
    Client::builder()
        .timeout(Duration::from_secs(90))
        .build()
        .map_err(|e| ApiError::Internal(format!("Failed to create HTTP client: {e}")))
}

fn normalized_base_url(api_type: &str, base_url: Option<&str>) -> Result<String, ApiError> {
    let fallback = match api_type {
        "openai" => Some(DEFAULT_OPENAI_BASE_URL),
        "openai-compatible" | "anthropic-compatible" => None,
        "anthropic" => Some(DEFAULT_ANTHROPIC_BASE_URL),
        "google" => Some(DEFAULT_GOOGLE_BASE_URL),
        _ => None,
    };

    let base_url = base_url
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .or(fallback)
        .ok_or_else(|| ApiError::BadRequest("baseUrl is required".to_string()))?;

    Ok(trim_trailing_slash(base_url))
}

fn trim_trailing_slash(value: &str) -> String {
    value.trim_end_matches('/').to_string()
}

fn join_url(base: &str, path: &str) -> String {
    format!(
        "{}/{}",
        base.trim_end_matches('/'),
        path.trim_start_matches('/')
    )
}

/// Candidate URLs to try for `path` under a user-supplied base URL.
///
/// Providers document their base URL inconsistently: some publish
/// `https://host` and expect the client to append `/v1`, others publish
/// `https://host/v1` already. Guessing one convention makes the other fail with
/// a bare 404 — which is what "the fetch-models button returns nothing" looked
/// like for OpenAI-compatible relays, where only `{base}/models` was ever tried.
///
/// Both spellings are attempted, most-likely first, with duplicates removed so a
/// base that already ends in `/v1` is never turned into `/v1/v1`.
fn endpoint_candidates(base: &str, path: &str) -> Vec<String> {
    let trimmed = base.trim_end_matches('/');
    let leaf = path.trim_start_matches('/');

    let mut candidates = Vec::with_capacity(2);
    let mut push = |url: String| {
        if !candidates.contains(&url) {
            candidates.push(url);
        }
    };

    if trimmed.ends_with("/v1") || trimmed.contains("/v1/") {
        // Base already carries the version segment — use it verbatim first, and
        // fall back to the base with that segment removed rather than appending
        // a second one (which would produce `/v1/v1/...`).
        push(format!("{trimmed}/{leaf}"));
        if let Some(without_version) = trimmed.strip_suffix("/v1") {
            push(format!("{without_version}/{leaf}"));
        }
    } else {
        push(format!("{trimmed}/v1/{leaf}"));
        push(format!("{trimmed}/{leaf}"));
    }

    candidates
}

// ============================================================================
// OpenAI / OpenAI-Compatible
// ============================================================================

async fn list_openai_models(
    client: &Client,
    base_url: &str,
    api_key: &str,
) -> Result<Vec<String>, ApiError> {
    let mut attempts = Vec::new();

    for url in endpoint_candidates(base_url, "models") {
        tracing::debug!("Fetching models from: {}", url);

        let response = client
            .get(&url)
            .header("Authorization", format!("Bearer {api_key}"))
            .send()
            .await;

        match model_list_attempt(response, &url).await {
            Ok(models) => return Ok(models),
            Err(detail) => attempts.push(detail),
        }
    }

    Err(model_list_error(&attempts))
}

/// Evaluate one model-list HTTP attempt.
///
/// `Ok` carries the parsed model ids; `Err` carries a human-readable reason
/// suitable for showing the user, so a failure explains itself instead of
/// collapsing into an empty dropdown.
async fn model_list_attempt(
    response: Result<reqwest::Response, reqwest::Error>,
    url: &str,
) -> Result<Vec<String>, String> {
    let response = match response {
        Ok(response) => response,
        Err(e) => return Err(format!("{url} — request failed: {e}")),
    };

    let status = response.status();
    let body = response.text().await.unwrap_or_default();

    if !status.is_success() {
        tracing::warn!("Model list request failed: {} {} - {}", url, status, body);
        return Err(format!("{url} — HTTP {status}"));
    }

    let json: Value = match serde_json::from_str(&body) {
        Ok(json) => json,
        Err(e) => {
            tracing::warn!("Model list response was not JSON: {} - {}", url, e);
            return Err(format!("{url} — response was not JSON ({e})"));
        }
    };

    let models = extract_model_ids(&json);
    if models.is_empty() {
        // A 200 with no recognisable model array means we hit something that is
        // not the models endpoint (an SPA index page, a gateway landing route).
        return Err(format!("{url} — response contained no model list"));
    }

    Ok(models)
}

/// Collapse every failed endpoint attempt into one actionable error.
fn model_list_error(attempts: &[String]) -> ApiError {
    if attempts.is_empty() {
        return ApiError::BadRequest("Model list request failed".to_string());
    }
    ApiError::BadRequest(format!(
        "Could not fetch the model list. Tried: {}. Check the base URL and API key, \
         or type the model ID manually.",
        attempts.join("; ")
    ))
}

async fn verify_openai_model(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
) -> VerifyOutcome {
    let payload = serde_json::json!({
        "model": model_id,
        "messages": [{"role": "user", "content": "ping"}],
        "max_tokens": 1,
        "temperature": 0.0
    });

    verify_openai_style(
        client,
        base_url,
        api_key,
        model_id,
        "chat/completions",
        &payload,
        &["choices", "content"],
        "OpenAI",
    )
    .await
}

/// Verify against the OpenAI **Responses** API — the only wire protocol Codex
/// supports (`wire_api = "chat"` was removed from Codex; see
/// `create_codex_config`). An endpoint that answers Chat Completions but not
/// Responses cannot drive a Codex terminal, and must not verify green.
async fn verify_openai_responses_model(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
) -> VerifyOutcome {
    let payload = serde_json::json!({
        "model": model_id,
        "input": "ping",
        "max_output_tokens": 16,
        "stream": false
    });

    verify_openai_style(
        client,
        base_url,
        api_key,
        model_id,
        "responses",
        &payload,
        &["output", "output_text", "content", "id"],
        "OpenAI Responses",
    )
    .await
    .map_err(|detail| {
        format!(
            "{detail}\nCodex requires the OpenAI Responses API. This endpoint did not answer it, \
             so the model will not work in a Codex terminal even if Chat Completions succeeds."
        )
    })
}

/// Shared POST-and-validate loop for the OpenAI-shaped endpoints, trying both
/// `{base}/{path}` and `{base}/v1/{path}`.
#[allow(clippy::too_many_arguments)]
async fn verify_openai_style(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
    path: &str,
    payload: &Value,
    expected_keys: &[&str],
    label: &str,
) -> VerifyOutcome {
    let mut attempts = Vec::new();

    for url in endpoint_candidates(base_url, path) {
        tracing::debug!("Verifying model {} at: {}", model_id, url);

        let response = client
            .post(&url)
            .header("Authorization", format!("Bearer {api_key}"))
            .header("Content-Type", "application/json")
            .json(payload)
            .send()
            .await;

        let response = match response {
            Ok(response) => response,
            Err(e) => {
                tracing::warn!("Failed to verify model at {}: {}", url, e);
                attempts.push(format!("{url} — request failed: {e}"));
                continue;
            }
        };

        let status = response.status();
        let body = response.text().await.unwrap_or_default();

        if !status.is_success() {
            tracing::warn!("{label} verification failed: {} {} - {}", url, status, body);
            attempts.push(format!("{url} — HTTP {status}"));
            continue;
        }

        if !verify_response_body_ok(&body, expected_keys, label) {
            attempts.push(format!("{url} — HTTP 200 but the body was not a {label} reply"));
            continue;
        }

        tracing::info!("Model {} verified successfully at {}", model_id, url);
        return Ok(true);
    }

    Err(format!("Tried: {}", attempts.join("; ")))
}

// ============================================================================
// Anthropic
// ============================================================================

async fn list_anthropic_models(
    client: &Client,
    base_url: &str,
    api_key: &str,
) -> Result<Vec<String>, ApiError> {
    let mut attempts = Vec::new();

    for url in endpoint_candidates(base_url, "models") {
        tracing::debug!("Fetching Anthropic models from: {}", url);

        // Anthropic-compatible relays are frequently multi-protocol gateways
        // that authenticate with either header, so send both rather than
        // failing on a gateway that only understands `Authorization`.
        let response = client
            .get(&url)
            .header("x-api-key", api_key)
            .header("Authorization", format!("Bearer {api_key}"))
            .header("anthropic-version", ANTHROPIC_VERSION)
            .send()
            .await;

        match model_list_attempt(response, &url).await {
            Ok(models) => return Ok(models),
            Err(detail) => attempts.push(detail),
        }
    }

    Err(model_list_error(&attempts))
}

async fn verify_anthropic_model(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
) -> VerifyOutcome {
    let payload = serde_json::json!({
        "model": model_id,
        "messages": [{"role": "user", "content": "ping"}],
        "max_tokens": 32
    });

    let mut attempts = Vec::new();

    for url in endpoint_candidates(base_url, "messages") {
        tracing::debug!("Verifying Anthropic model {} at: {}", model_id, url);

        let response = client
            .post(&url)
            .header("x-api-key", api_key)
            .header("Authorization", format!("Bearer {api_key}"))
            .header("anthropic-version", ANTHROPIC_VERSION)
            .header("Content-Type", "application/json")
            .json(&payload)
            .send()
            .await;

        let response = match response {
            Ok(response) => response,
            Err(e) => {
                attempts.push(format!("{url} — request failed: {e}"));
                continue;
            }
        };

        let status = response.status();
        let body = response.text().await.unwrap_or_default();

        if !status.is_success() {
            tracing::warn!(
                "Anthropic model verification failed: {} {} - {}",
                url,
                status,
                body
            );
            attempts.push(format!("{url} — HTTP {status}"));
            continue;
        }

        if !verify_response_body_ok(&body, &["content", "id"], "Anthropic") {
            attempts.push(format!(
                "{url} — HTTP 200 but the body was not an Anthropic reply"
            ));
            continue;
        }

        tracing::info!("Anthropic model {} verified successfully", model_id);
        return Ok(true);
    }

    Err(format!("Tried: {}", attempts.join("; ")))
}

// ============================================================================
// Google
// ============================================================================

async fn list_google_models(
    client: &Client,
    base_url: &str,
    api_key: &str,
) -> Result<Vec<String>, ApiError> {
    let root = if base_url.ends_with("/v1") || base_url.ends_with("/v1beta") {
        base_url.to_string()
    } else {
        format!("{}/v1beta", base_url.trim_end_matches('/'))
    };
    let url = join_url(&root, "models");
    tracing::debug!("Fetching Google models from: {}", url);

    let response = client
        .get(&url)
        .query(&[("key", api_key)])
        .send()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to fetch models: {e}")))?;

    let status = response.status();
    let body = response.text().await.unwrap_or_default();

    if !status.is_success() {
        tracing::warn!("Google model list request failed: {} - {}", status, body);
        return Err(ApiError::BadRequest(format!(
            "Model list request failed: {status}"
        )));
    }

    let json: Value = serde_json::from_str(&body)
        .map_err(|e| ApiError::Internal(format!("Invalid model list response: {e}")))?;

    Ok(extract_google_model_ids(&json))
}

async fn verify_google_model(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
) -> VerifyOutcome {
    let models = list_google_models(client, base_url, api_key)
        .await
        .map_err(|e| format!("Could not list Google models: {e}"))?;
    let target = model_id.trim();
    if models.iter().any(|model| model == target) {
        return Ok(true);
    }
    Err(format!(
        "Model '{target}' is not in the endpoint's model list"
    ))
}

// ============================================================================
// Shared verification helpers
// ============================================================================

/// Validates that a 200 response body does not contain a top-level "error" key
/// and (optionally) contains at least one of the `expected_keys`.
/// Some providers (e.g., BigModel.cn) return HTTP 200 with error payloads.
fn verify_response_body_ok(body: &str, expected_keys: &[&str], label: &str) -> bool {
    match serde_json::from_str::<Value>(body) {
        Ok(json) => {
            if json.get("error").is_some() {
                tracing::warn!("{label} verification returned 200 but body contains error: {body}");
                return false;
            }
            if !expected_keys.is_empty() && expected_keys.iter().all(|k| json.get(*k).is_none()) {
                tracing::warn!(
                    "{label} verification returned 200 but body has no {expected_keys:?}: {body}"
                );
                return false;
            }
            true
        }
        Err(_) => {
            tracing::warn!(
                "{label} verification returned 200 but body is not valid JSON (likely wrong URL): {}",
                body.chars().take(200).collect::<String>()
            );
            false
        }
    }
}

// ============================================================================
// Helpers
// ============================================================================

fn extract_model_ids(json: &Value) -> Vec<String> {
    let mut models = Vec::new();

    // OpenAI format: { "data": [{ "id": "model-id" }, ...] }
    if let Some(items) = json.get("data").and_then(|v| v.as_array()) {
        for item in items {
            if let Some(id) = item.get("id").and_then(|v| v.as_str()) {
                models.push(id.to_string());
            }
        }
    }

    // Alternative format: { "models": [{ "id": "model-id" }, ...] }
    if models.is_empty() {
        if let Some(items) = json.get("models").and_then(|v| v.as_array()) {
            for item in items {
                if let Some(id) = item.get("id").and_then(|v| v.as_str()) {
                    models.push(id.to_string());
                }
            }
        }
    }

    models
}

fn extract_google_model_ids(json: &Value) -> Vec<String> {
    let mut models = Vec::new();

    // Google format: { "models": [{ "name": "models/gemini-pro" }, ...] }
    if let Some(items) = json.get("models").and_then(|v| v.as_array()) {
        for item in items {
            if let Some(name) = item.get("name").and_then(|v| v.as_str()) {
                // Extract model ID from "models/gemini-pro" -> "gemini-pro"
                let id = name.split('/').next_back().unwrap_or(name).to_string();
                models.push(id);
            }
        }
    }

    models
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn endpoint_candidates_tries_versioned_path_first_for_bare_base() {
        // Providers commonly document `https://host` as the base URL and expect
        // the client to append `/v1`. Only trying `{base}/models` produced a 404
        // and an empty model dropdown.
        assert_eq!(
            endpoint_candidates("https://api.example.com", "models"),
            vec![
                "https://api.example.com/v1/models".to_string(),
                "https://api.example.com/models".to_string(),
            ]
        );
    }

    #[test]
    fn endpoint_candidates_never_doubles_an_existing_version_segment() {
        let candidates = endpoint_candidates("https://api.example.com/v1", "models");
        assert_eq!(candidates[0], "https://api.example.com/v1/models");
        assert!(
            !candidates.iter().any(|url| url.contains("/v1/v1/")),
            "a base URL that already ends in /v1 must not become /v1/v1: {candidates:?}"
        );
    }

    #[test]
    fn endpoint_candidates_trims_trailing_slashes() {
        assert_eq!(
            endpoint_candidates("https://api.example.com/", "models")[0],
            "https://api.example.com/v1/models"
        );
    }

    #[test]
    fn endpoint_candidates_respects_a_mid_path_version_segment() {
        let candidates = endpoint_candidates("https://gateway.example.com/v1/openai", "models");
        assert_eq!(candidates[0], "https://gateway.example.com/v1/openai/models");
    }

    #[test]
    fn endpoint_candidates_deduplicates() {
        // A base whose only difference between the two spellings is ordering
        // must not produce the same URL twice.
        let candidates = endpoint_candidates("https://api.example.com/v1", "models");
        let mut sorted = candidates.clone();
        sorted.sort();
        sorted.dedup();
        assert_eq!(sorted.len(), candidates.len());
    }

    #[test]
    fn codex_cli_is_recognised_by_id_and_by_name() {
        assert!(is_codex_cli(Some("cli-codex")));
        assert!(is_codex_cli(Some("codex")));
        assert!(is_codex_cli(Some("  Codex  ")));
        assert!(!is_codex_cli(Some("cli-claude-code")));
        assert!(!is_codex_cli(None));
    }

    #[test]
    fn model_list_error_names_every_attempted_endpoint() {
        let error = model_list_error(&[
            "https://a/v1/models — HTTP 404 Not Found".to_string(),
            "https://a/models — HTTP 404 Not Found".to_string(),
        ]);
        let message = format!("{error:?}");
        assert!(message.contains("https://a/v1/models"), "{message}");
        assert!(message.contains("https://a/models"), "{message}");
    }

    #[test]
    fn verify_outcome_carries_failure_detail_to_the_client() {
        let response = verify_outcome_to_response(Err("Tried: https://a/v1/responses — HTTP 404".to_string()));
        assert!(!response.verified);
        assert!(
            response.detail.is_some_and(|d| d.contains("responses")),
            "a failed verification must explain which endpoint was tried"
        );

        let ok = verify_outcome_to_response(Ok(true));
        assert!(ok.verified);
        assert!(ok.detail.is_none());
    }

    #[test]
    fn extract_model_ids_supports_both_documented_shapes() {
        let openai = serde_json::json!({"data": [{"id": "gpt-x"}, {"id": "gpt-y"}]});
        assert_eq!(extract_model_ids(&openai), vec!["gpt-x", "gpt-y"]);

        let alternative = serde_json::json!({"models": [{"id": "m-1"}]});
        assert_eq!(extract_model_ids(&alternative), vec!["m-1"]);
    }
}
