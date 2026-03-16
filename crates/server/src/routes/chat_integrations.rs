//! Chat connector integration routes.
//!
//! Provides a provider-agnostic connector entrypoint and binding management:
//! - POST /api/integrations/chat/{provider}/events
//! - PUT  /api/integrations/chat/{provider}/bindings
//! - DELETE /api/integrations/chat/{provider}/bindings/{conversation_id}

use std::{
    collections::HashMap,
    time::{Duration, Instant},
};

use axum::{
    Json, Router,
    extract::{Path, State},
    http::HeaderMap,
    response::Json as ResponseJson,
    routing::{delete, post, put},
};
use chrono::Utc;
use db::models::{ExternalConversationBinding, Workflow};
use deployment::Deployment;
use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{
    DeploymentImpl,
    error::ApiError,
    routes::workflows::{
        OrchestratorChatRequestMetadata, SubmitOrchestratorChatRequest,
        submit_orchestrator_chat,
    },
};

const CHAT_EVENT_ALLOWED_PROVIDER: &str = "telegram";
const CHAT_EVENT_REPLAY_WINDOW: Duration = Duration::from_secs(15 * 60);
const CHAT_EVENT_TIMESTAMP_TOLERANCE_SECS: i64 = 300;

static CHAT_REPLAY_CACHE: Lazy<tokio::sync::Mutex<HashMap<String, Instant>>> =
    Lazy::new(|| tokio::sync::Mutex::new(HashMap::new()));

pub fn router() -> Router<DeploymentImpl> {
    Router::new()
        .route("/chat/{provider}/events", post(handle_chat_event))
        .route("/chat/{provider}/bindings", put(bind_conversation))
        .route(
            "/chat/{provider}/bindings/{conversation_id}",
            delete(unbind_conversation),
        )
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ChatEventRequest {
    provider_message_id: String,
    conversation_id: String,
    sender_id: String,
    text: String,
    signature: String,
    timestamp: i64,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BindConversationRequest {
    workflow_id: String,
    conversation_id: String,
    operator_id: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ChatConnectorResponse {
    status: String,
    message: String,
    template: String,
    workflow_id: Option<String>,
    command_id: Option<String>,
}

fn ensure_supported_provider(provider: &str) -> Result<(), ApiError> {
    if provider.eq_ignore_ascii_case(CHAT_EVENT_ALLOWED_PROVIDER) {
        return Ok(());
    }

    Err(ApiError::BadRequest(format!(
        "Unsupported chat provider '{provider}'. Supported provider: {CHAT_EVENT_ALLOWED_PROVIDER}"
    )))
}

fn is_chat_connector_feature_enabled() -> bool {
    std::env::var("GITCORTEX_CHAT_CONNECTOR_ENABLED")
        .ok()
        .is_some_and(|v| v.trim().eq_ignore_ascii_case("true") || v.trim() == "1")
}

fn read_chat_webhook_secret() -> Result<String, ApiError> {
    std::env::var("GITCORTEX_CHAT_WEBHOOK_SECRET")
        .map_err(|_| ApiError::Conflict("GITCORTEX_CHAT_WEBHOOK_SECRET is not configured".to_string()))
}

fn compute_chat_signature(secret: &str, provider: &str, payload: &ChatEventRequest) -> String {
    let canonical = format!(
        "{}:{}:{}:{}:{}:{}",
        provider,
        payload.conversation_id,
        payload.provider_message_id,
        payload.sender_id,
        payload.timestamp,
        payload.text
    );
    let mut hasher = Sha256::new();
    hasher.update(secret.as_bytes());
    hasher.update(b":");
    hasher.update(canonical.as_bytes());
    let digest = hasher.finalize();
    format!("sha256={digest:x}")
}

fn verify_event_timestamp(timestamp: i64) -> Result<(), ApiError> {
    let now = Utc::now().timestamp();
    let delta = (now - timestamp).abs();
    if delta > CHAT_EVENT_TIMESTAMP_TOLERANCE_SECS {
        return Err(ApiError::Forbidden(
            "Chat event timestamp is outside the allowed time window".to_string(),
        ));
    }
    Ok(())
}

async fn ensure_not_replayed(provider: &str, provider_message_id: &str) -> Result<(), ApiError> {
    let now = Instant::now();
    let cache_key = format!("{provider}:{provider_message_id}");
    let mut cache = CHAT_REPLAY_CACHE.lock().await;

    cache.retain(|_, seen_at| now.duration_since(*seen_at) <= CHAT_EVENT_REPLAY_WINDOW);

    if cache.contains_key(&cache_key) {
        return Err(ApiError::Conflict(
            "Duplicate chat event detected (replay blocked)".to_string(),
        ));
    }

    cache.insert(cache_key, now);
    Ok(())
}

fn build_external_template(status: &str, retryable: bool, error: Option<&str>) -> (&'static str, String) {
    match status {
        "succeeded" => (
            "success",
            "任务已接收并执行完成".to_string(),
        ),
        "failed" => (
            if retryable { "need_confirmation" } else { "unexecutable" },
            match error {
                Some(err) => format!("执行失败：{err}"),
                None => "执行失败，请稍后重试".to_string(),
            },
        ),
        "cancelled" => ("need_confirmation", "执行已取消，请确认是否重试".to_string()),
        _ => ("unexecutable", "当前状态不可执行，请稍后重试".to_string()),
    }
}

async fn bind_conversation(
    State(deployment): State<DeploymentImpl>,
    Path(provider): Path<String>,
    Json(payload): Json<BindConversationRequest>,
) -> Result<ResponseJson<ApiResponse<ChatConnectorResponse>>, ApiError> {
    if !is_chat_connector_feature_enabled() {
        return Err(ApiError::Conflict(
            "Chat connector feature is disabled by rollout flag".to_string(),
        ));
    }

    ensure_supported_provider(&provider)?;

    let workflow = Workflow::find_by_id(&deployment.db().pool, &payload.workflow_id)
        .await?
        .ok_or_else(|| ApiError::NotFound("Workflow not found".to_string()))?;
    if workflow.execution_mode != "agent_planned" {
        return Err(ApiError::Conflict(
            "Only agent_planned workflows can be bound to chat connectors".to_string(),
        ));
    }

    ExternalConversationBinding::upsert(
        &deployment.db().pool,
        &provider,
        &payload.conversation_id,
        &payload.workflow_id,
        payload.operator_id.as_deref(),
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to bind conversation: {e}")))?;

    let response = ChatConnectorResponse {
        status: "bound".to_string(),
        message: format!(
            "Conversation {} is now bound to workflow {}",
            payload.conversation_id, payload.workflow_id
        ),
        template: "success".to_string(),
        workflow_id: Some(payload.workflow_id),
        command_id: None,
    };

    Ok(ResponseJson(ApiResponse::success(response)))
}

async fn unbind_conversation(
    State(deployment): State<DeploymentImpl>,
    Path((provider, conversation_id)): Path<(String, String)>,
) -> Result<ResponseJson<ApiResponse<ChatConnectorResponse>>, ApiError> {
    if !is_chat_connector_feature_enabled() {
        return Err(ApiError::Conflict(
            "Chat connector feature is disabled by rollout flag".to_string(),
        ));
    }

    ensure_supported_provider(&provider)?;

    let affected = ExternalConversationBinding::deactivate(
        &deployment.db().pool,
        &provider,
        &conversation_id,
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to unbind conversation: {e}")))?;

    if affected == 0 {
        return Err(ApiError::NotFound(
            "Conversation binding not found".to_string(),
        ));
    }

    let response = ChatConnectorResponse {
        status: "unbound".to_string(),
        message: format!("Conversation {conversation_id} has been unbound"),
        template: "success".to_string(),
        workflow_id: None,
        command_id: None,
    };

    Ok(ResponseJson(ApiResponse::success(response)))
}

async fn handle_chat_event(
    State(deployment): State<DeploymentImpl>,
    Path(provider): Path<String>,
    Json(payload): Json<ChatEventRequest>,
) -> Result<ResponseJson<ApiResponse<ChatConnectorResponse>>, ApiError> {
    if !is_chat_connector_feature_enabled() {
        return Err(ApiError::Conflict(
            "Chat connector feature is disabled by rollout flag".to_string(),
        ));
    }

    ensure_supported_provider(&provider)?;
    verify_event_timestamp(payload.timestamp)?;
    ensure_not_replayed(&provider, &payload.provider_message_id).await?;

    let secret = read_chat_webhook_secret()?;
    let expected_signature = compute_chat_signature(&secret, &provider, &payload);
    if !constant_time_eq(payload.signature.as_bytes(), expected_signature.as_bytes()) {
        return Err(ApiError::Forbidden(
            "Invalid chat event signature".to_string(),
        ));
    }

    let command_text = payload.text.trim();
    if command_text.is_empty() {
        return Err(ApiError::BadRequest(
            "Chat event text must not be empty".to_string(),
        ));
    }

    if let Some(workflow_id) = command_text.strip_prefix("/bind ").map(str::trim) {
        if workflow_id.is_empty() {
            return Err(ApiError::BadRequest(
                "Usage: /bind <workflow_id>".to_string(),
            ));
        }

        ExternalConversationBinding::upsert(
            &deployment.db().pool,
            &provider,
            &payload.conversation_id,
            workflow_id,
            Some(payload.sender_id.as_str()),
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to bind conversation: {e}")))?;

        let response = ChatConnectorResponse {
            status: "bound".to_string(),
            message: format!("Bound to workflow {workflow_id}"),
            template: "success".to_string(),
            workflow_id: Some(workflow_id.to_string()),
            command_id: None,
        };
        return Ok(ResponseJson(ApiResponse::success(response)));
    }

    if command_text.eq_ignore_ascii_case("/unbind") {
        ExternalConversationBinding::deactivate(
            &deployment.db().pool,
            &provider,
            &payload.conversation_id,
        )
        .await
        .map_err(|e| ApiError::Internal(format!("Failed to unbind conversation: {e}")))?;

        let response = ChatConnectorResponse {
            status: "unbound".to_string(),
            message: "Conversation unbound".to_string(),
            template: "success".to_string(),
            workflow_id: None,
            command_id: None,
        };
        return Ok(ResponseJson(ApiResponse::success(response)));
    }

    let binding = ExternalConversationBinding::find_active(
        &deployment.db().pool,
        &provider,
        &payload.conversation_id,
    )
    .await
    .map_err(|e| ApiError::Internal(format!("Failed to query conversation binding: {e}")))?;

    let Some(binding) = binding else {
        let response = ChatConnectorResponse {
            status: "unbound".to_string(),
            message: "Conversation is not bound. Use /bind <workflow_id> first.".to_string(),
            template: "need_confirmation".to_string(),
            workflow_id: None,
            command_id: None,
        };
        return Ok(ResponseJson(ApiResponse::success(response)));
    };

    let mut headers = HeaderMap::new();
    headers.insert("x-orchestrator-role", "operator".parse().expect("valid header value"));
    headers.insert(
        "x-orchestrator-operator-id",
        payload
            .sender_id
            .parse()
            .unwrap_or_else(|_| "external-user".parse().expect("valid fallback header")),
    );

    let submit_payload = SubmitOrchestratorChatRequest {
        message: command_text.to_string(),
        source: Some("social".to_string()),
        external_message_id: Some(payload.provider_message_id.clone()),
        metadata: OrchestratorChatRequestMetadata {
            operator_id: Some(payload.sender_id.clone()),
            client_ts: Some(payload.timestamp.to_string()),
            conversation_id: Some(payload.conversation_id.clone()),
        },
    };

    let ResponseJson(api_response) = submit_orchestrator_chat(
        State(deployment),
        headers,
        Path(Uuid::parse_str(&binding.workflow_id).map_err(|e| ApiError::BadRequest(format!("Invalid workflow ID: {e}")))?),
        Json(submit_payload),
    )
    .await?;

    let submit_response = api_response.into_data().ok_or_else(|| {
        ApiError::Internal("Orchestrator chat response did not include command data".to_string())
    })?;

    let (template, message) = build_external_template(
        submit_response.status.as_str(),
        submit_response.retryable,
        submit_response.error.as_deref(),
    );

    let response = ChatConnectorResponse {
        status: submit_response.status,
        message,
        template: template.to_string(),
        workflow_id: Some(binding.workflow_id),
        command_id: Some(submit_response.command_id),
    };

    Ok(ResponseJson(ApiResponse::success(response)))
}

#[cfg(test)]
mod tests {
    use axum::{
        body::{Body, to_bytes},
        http::{Request, StatusCode},
    };
    use chrono::Utc;
    use db::models::{
        Workflow,
        project::{CreateProject, Project},
    };
    use serial_test::serial;
    use tower::ServiceExt;
    use uuid::Uuid;

    use super::*;

    fn build_agent_planned_workflow(
        id: String,
        project_id: Uuid,
        merge_cli_id: &str,
        merge_model_id: &str,
    ) -> Workflow {
        Workflow {
            id,
            project_id,
            name: "Chat Bound Workflow".to_string(),
            description: None,
            status: "running".to_string(),
            execution_mode: "agent_planned".to_string(),
            initial_goal: Some("Handle external chat commands".to_string()),
            use_slash_commands: false,
            orchestrator_enabled: true,
            orchestrator_api_type: Some("openai-compatible".to_string()),
            orchestrator_base_url: Some("https://api.example.com".to_string()),
            orchestrator_api_key: None,
            orchestrator_model: Some("gpt-4.1".to_string()),
            error_terminal_enabled: false,
            error_terminal_cli_id: None,
            error_terminal_model_id: None,
            merge_terminal_cli_id: merge_cli_id.to_string(),
            merge_terminal_model_id: merge_model_id.to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: Some(Utc::now()),
            started_at: Some(Utc::now()),
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    #[tokio::test]
    #[serial]
    async fn bind_and_unbind_conversation_succeeds_for_agent_planned_workflow() {
        unsafe { std::env::set_var("GITCORTEX_CHAT_CONNECTOR_ENABLED", "true"); }
        let deployment = DeploymentImpl::new().await.expect("deployment should start");
        let app = router().with_state(deployment.clone());

        let project_id = Uuid::new_v4();
        Project::create(
            &deployment.db().pool,
            &CreateProject {
                name: "Chat Integration Project".to_string(),
                repositories: vec![],
            },
            project_id,
        )
        .await
        .expect("project should be inserted");

        let cli_id = format!("test-cli-{}", Uuid::new_v4().simple());
        let model_id = format!("test-model-{}", Uuid::new_v4().simple());
        let workflow_id = Uuid::new_v4().to_string();
        sqlx::query(
            r"
            INSERT INTO cli_type (
                id, name, display_name, detect_command, install_command,
                install_guide_url, config_file_path, is_system, created_at
            ) VALUES (?1, ?2, ?3, ?4, NULL, NULL, NULL, 0, ?5)
            ",
        )
        .bind(&cli_id)
        .bind(&cli_id)
        .bind("Test CLI")
        .bind("echo --version")
        .bind(Utc::now())
        .execute(&deployment.db().pool)
        .await
        .expect("cli type should be inserted");
        sqlx::query(
            r"
            INSERT INTO model_config (
                id, cli_type_id, name, display_name, api_model_id,
                is_default, is_official, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, 1, 1, ?6, ?7)
            ",
        )
        .bind(&model_id)
        .bind(&cli_id)
        .bind(&model_id)
        .bind("Test Model")
        .bind(&model_id)
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&deployment.db().pool)
        .await
        .expect("model config should be inserted");

        Workflow::create(
            &deployment.db().pool,
            &build_agent_planned_workflow(
                workflow_id.clone(),
                project_id,
                &cli_id,
                &model_id,
            ),
        )
        .await
        .expect("workflow should be inserted");

        let bind_payload = serde_json::json!({
            "workflowId": workflow_id,
            "conversationId": "conv-1",
            "operatorId": "tester"
        })
        .to_string();

        let bind_request = Request::builder()
            .method("PUT")
            .uri("/chat/telegram/bindings")
            .header("content-type", "application/json")
            .body(Body::from(bind_payload))
            .expect("request should build");
        let bind_response = app
            .clone()
            .oneshot(bind_request)
            .await
            .expect("bind request should execute");
        assert_eq!(bind_response.status(), StatusCode::OK);

        let binding = ExternalConversationBinding::find_active(
            &deployment.db().pool,
            "telegram",
            "conv-1",
        )
        .await
        .expect("binding lookup should succeed");
        assert!(binding.is_some());

        let unbind_request = Request::builder()
            .method("DELETE")
            .uri("/chat/telegram/bindings/conv-1")
            .body(Body::empty())
            .expect("request should build");
        let unbind_response = app
            .oneshot(unbind_request)
            .await
            .expect("unbind request should execute");
        assert_eq!(unbind_response.status(), StatusCode::OK);
    }

    #[tokio::test]
    #[serial]
    async fn handle_chat_event_rejects_invalid_signature() {
        unsafe {
            std::env::set_var("GITCORTEX_CHAT_CONNECTOR_ENABLED", "true");
            std::env::set_var("GITCORTEX_CHAT_WEBHOOK_SECRET", "secret-1");
        }

        let deployment = DeploymentImpl::new().await.expect("deployment should start");
        let app = router().with_state(deployment);

        let payload = serde_json::json!({
            "providerMessageId": "msg-1",
            "conversationId": "conv-1",
            "senderId": "user-1",
            "text": "hello",
            "signature": "sha256=deadbeef",
            "timestamp": Utc::now().timestamp()
        })
        .to_string();

        let request = Request::builder()
            .method("POST")
            .uri("/chat/telegram/events")
            .header("content-type", "application/json")
            .body(Body::from(payload))
            .expect("request should build");
        let response = app
            .oneshot(request)
            .await
            .expect("request should execute");
        assert_eq!(response.status(), StatusCode::FORBIDDEN);

        let body = to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body read should succeed");
        let body_json: serde_json::Value =
            serde_json::from_slice(&body).expect("body should be json");
        assert_eq!(
            body_json.get("message").and_then(serde_json::Value::as_str),
            Some("Invalid chat event signature")
        );

        unsafe {
            std::env::remove_var("GITCORTEX_CHAT_WEBHOOK_SECRET");
        }
    }
}
