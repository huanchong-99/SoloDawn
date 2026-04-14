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
}

#[derive(Debug, Serialize)]
struct VerifyModelResponse {
    verified: bool,
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
        "openai" | "openai-compatible" => {
            list_openai_models(&client, &base_url, &api_key).await?
        }
        "anthropic" | "anthropic-compatible" => {
            list_anthropic_models(&client, &base_url, &api_key).await?
        }
        "google" => {
            list_google_models(&client, &base_url, &api_key).await?
        }
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

    let verified = match payload.api_type.as_str() {
        "openai" | "openai-compatible" => {
            let base_url = trim_trailing_slash(&payload.base_url);
            verify_openai_model(&client, &base_url, &payload.api_key, &payload.model_id).await
        }
        "anthropic" | "anthropic-compatible" => {
            let base_url = trim_trailing_slash(&payload.base_url);
            verify_anthropic_model(&client, &base_url, &payload.api_key, &payload.model_id).await
        }
        "google" => {
            let base_url = trim_trailing_slash(&payload.base_url);
            verify_google_model(&client, &base_url, &payload.api_key, &payload.model_id).await
        }
        other => {
            return Err(ApiError::BadRequest(format!(
                "Unsupported apiType: {other}"
            )));
        }
    };

    Ok(ResponseJson(VerifyModelResponse {
        verified: verified.unwrap_or(false),
    }))
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

// ============================================================================
// OpenAI / OpenAI-Compatible
// ============================================================================

async fn list_openai_models(
    client: &Client,
    base_url: &str,
    api_key: &str,
) -> Result<Vec<String>, ApiError> {
    let url = join_url(base_url, "models");
    tracing::debug!("Fetching models from: {}", url);

    let response = client
        .get(&url)
        .header("Authorization", format!("Bearer {api_key}"))
        .send()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to fetch models: {e}")))?;

    let status = response.status();
    let body = response.text().await.unwrap_or_default();

    if !status.is_success() {
        tracing::warn!("Model list request failed: {} - {}", status, body);
        return Err(ApiError::BadRequest(format!(
            "Model list request failed: {status}"
        )));
    }

    let json: Value = serde_json::from_str(&body)
        .map_err(|e| ApiError::Internal(format!("Invalid model list response: {e}")))?;

    Ok(extract_model_ids(&json))
}

async fn verify_openai_model(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
) -> Result<bool, ApiError> {
    let url = join_url(base_url, "chat/completions");
    tracing::debug!("Verifying model {} at: {}", model_id, url);

    let payload = serde_json::json!({
        "model": model_id,
        "messages": [{"role": "user", "content": "ping"}],
        "max_tokens": 1,
        "temperature": 0.0
    });

    let response = client
        .post(&url)
        .header("Authorization", format!("Bearer {api_key}"))
        .header("Content-Type", "application/json")
        .json(&payload)
        .send()
        .await
        .map_err(|e| {
            tracing::warn!("Failed to verify model: {}", e);
            ApiError::Internal(format!("Failed to verify model: {e}"))
        })?;

    let status = response.status();
    if !status.is_success() {
        let body = response.text().await.unwrap_or_default();
        tracing::warn!("Model verification failed: {} - {}", status, body);
        return Ok(false);
    }

    if !verify_response_body_ok(&response.text().await.unwrap_or_default(), &["choices", "content"], "OpenAI") {
        return Ok(false);
    }

    tracing::info!("Model {} verified successfully", model_id);
    Ok(true)
}

// ============================================================================
// Anthropic
// ============================================================================

async fn list_anthropic_models(
    client: &Client,
    base_url: &str,
    api_key: &str,
) -> Result<Vec<String>, ApiError> {
    let url = join_url(base_url, "models");
    tracing::debug!("Fetching Anthropic models from: {}", url);

    let response = client
        .get(&url)
        .header("x-api-key", api_key)
        .header("anthropic-version", ANTHROPIC_VERSION)
        .send()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to fetch models: {e}")))?;

    let status = response.status();
    let body = response.text().await.unwrap_or_default();

    if !status.is_success() {
        tracing::warn!("Anthropic model list request failed: {} - {}", status, body);
        return Err(ApiError::BadRequest(format!(
            "Model list request failed: {status}"
        )));
    }

    let json: Value = serde_json::from_str(&body)
        .map_err(|e| ApiError::Internal(format!("Invalid model list response: {e}")))?;

    Ok(extract_model_ids(&json))
}

async fn verify_anthropic_model(
    client: &Client,
    base_url: &str,
    api_key: &str,
    model_id: &str,
) -> Result<bool, ApiError> {
    let url = join_url(base_url, "messages");
    tracing::debug!("Verifying Anthropic model {} at: {}", model_id, url);

    let payload = serde_json::json!({
        "model": model_id,
        "messages": [{"role": "user", "content": "ping"}],
        "max_tokens": 32
    });

    let response = client
        .post(&url)
        .header("x-api-key", api_key)
        .header("anthropic-version", ANTHROPIC_VERSION)
        .header("Content-Type", "application/json")
        .json(&payload)
        .send()
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to verify model: {e}")))?;

    let status = response.status();
    if !status.is_success() {
        let body = response.text().await.unwrap_or_default();
        tracing::warn!("Anthropic model verification failed: {} - {}", status, body);
        return Ok(false);
    }

    if !verify_response_body_ok(&response.text().await.unwrap_or_default(), &["content", "id"], "Anthropic") {
        return Ok(false);
    }

    tracing::info!("Anthropic model {} verified successfully", model_id);
    Ok(true)
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
) -> Result<bool, ApiError> {
    let models = list_google_models(client, base_url, api_key).await?;
    let target = model_id.trim();
    Ok(models.iter().any(|model| model == target))
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
                tracing::warn!("{label} verification returned 200 but body has no {expected_keys:?}: {body}");
                return false;
            }
            true
        }
        Err(_) => {
            tracing::warn!(
                "{label} verification returned 200 but body is not valid JSON (likely wrong URL): {}",
                &body[..body.len().min(200)]
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
